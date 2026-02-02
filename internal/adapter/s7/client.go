// Package s7 provides a production-grade Siemens S7 client implementation
// with connection management, bidirectional communication, and comprehensive error handling.
package s7

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/robinson/gos7"
	"github.com/rs/zerolog"
)

// NewClient creates a new S7 client with the given configuration.
func NewClient(deviceID string, config ClientConfig, logger zerolog.Logger) (*Client, error) {
	if config.Address == "" {
		return nil, fmt.Errorf("S7 address is required")
	}

	// Apply defaults
	if config.Port == 0 {
		config.Port = 102 // Standard ISO-on-TCP port
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.IdleTimeout == 0 {
		config.IdleTimeout = 60 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 500 * time.Millisecond
	}
	if config.PDUSize == 0 {
		config.PDUSize = 480
	}

	c := &Client{
		config:         config,
		logger:         logger.With().Str("device_id", deviceID).Str("address", config.Address).Logger(),
		stats:          &ClientStats{},
		deviceID:       deviceID,
		lastUsed:       time.Now(),
		tagDiagnostics: make(map[string]*TagDiagnostic),
	}

	return c, nil
}

// Connect establishes the connection to the S7 PLC.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected.Load() {
		return nil
	}

	c.logger.Debug().
		Int("rack", c.config.Rack).
		Int("slot", c.config.Slot).
		Msg("Connecting to S7 PLC")

	// Create TCP handler
	address := fmt.Sprintf("%s:%d", c.config.Address, c.config.Port)
	handler := gos7.NewTCPClientHandler(address, c.config.Rack, c.config.Slot)
	handler.Timeout = c.config.Timeout
	handler.IdleTimeout = c.config.IdleTimeout

	// Connect with context timeout
	connectDone := make(chan error, 1)
	go func() {
		connectDone <- handler.Connect()
	}()

	select {
	case err := <-connectDone:
		if err != nil {
			// Close handler to release resources on failure
			handler.Close()
			c.lastError = err
			return fmt.Errorf("%w: %v", domain.ErrS7ConnectionFailed, err)
		}
	case <-ctx.Done():
		// Context cancelled - close handler in background to prevent leak
		// The Connect() goroutine may still be running, but handler.Close()
		// will cause it to fail and return.
		go func() {
			<-connectDone // Wait for connect goroutine to finish
			handler.Close()
		}()
		return fmt.Errorf("%w: %v", domain.ErrConnectionTimeout, ctx.Err())
	}

	c.handler = handler
	c.client = gos7.NewClient(handler)
	c.connected.Store(true)
	c.lastError = nil
	c.lastUsed = time.Now()

	c.logger.Info().Msg("Connected to S7 PLC")
	return nil
}

// Disconnect closes the connection to the S7 PLC.
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected.Load() {
		return nil
	}

	if c.handler != nil {
		c.handler.Close()
	}

	c.connected.Store(false)
	c.handler = nil
	c.client = nil

	c.logger.Debug().Msg("Disconnected from S7 PLC")
	return nil
}

// IsConnected returns true if the client is currently connected.
func (c *Client) IsConnected() bool {
	return c.connected.Load()
}

// ReadTag reads a single tag from the PLC.
func (c *Client) ReadTag(ctx context.Context, tag *domain.Tag) (*domain.DataPoint, error) {
	startTime := time.Now()
	defer func() {
		c.stats.TotalReadTime.Add(time.Since(startTime).Nanoseconds())
	}()

	c.mu.Lock()
	c.lastUsed = time.Now()
	c.mu.Unlock()

	if !c.connected.Load() {
		c.recordTagError(tag.ID, domain.ErrConnectionClosed)
		return nil, domain.ErrConnectionClosed
	}

	// Parse tag address if needed
	area, dbNumber, offset, bitOffset, err := c.parseTagAddress(tag)
	if err != nil {
		c.stats.ErrorCount.Add(1)
		c.recordTagError(tag.ID, err)
		c.consecutiveFailures.Add(1)
		return c.createErrorDataPoint(tag, err), err
	}

	var dp *domain.DataPoint

	// Execute read with retry logic
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			c.stats.RetryCount.Add(1)
			delay := c.calculateBackoff(attempt)
			c.logger.Debug().
				Int("attempt", attempt).
				Dur("delay", delay).
				Msg("Retrying S7 read")

			select {
			case <-ctx.Done():
				c.recordTagError(tag.ID, ctx.Err())
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		dp, err = c.readData(tag, area, dbNumber, offset, bitOffset)
		if err == nil {
			break
		}

		// Check if error is retryable
		if !c.isRetryableError(err) {
			c.stats.ErrorCount.Add(1)
			c.recordTagError(tag.ID, err)
			c.consecutiveFailures.Add(1)
			return c.createErrorDataPoint(tag, err), err
		}

		// Try to reconnect on connection errors
		if c.isConnectionError(err) {
			c.logger.Warn().Err(err).Msg("Connection error, attempting reconnect")
			c.reconnect(ctx)
		}
	}

	if err != nil {
		c.stats.ErrorCount.Add(1)
		c.recordTagError(tag.ID, err)
		c.consecutiveFailures.Add(1)
		return c.createErrorDataPoint(tag, err), err
	}

	// Success - reset consecutive failures and record success
	c.consecutiveFailures.Store(0)
	c.recordTagSuccess(tag.ID)
	c.stats.ReadCount.Add(1)
	return dp, nil
}

// ReadTags reads multiple tags efficiently using batch reads where possible.
func (c *Client) ReadTags(ctx context.Context, tags []*domain.Tag) ([]*domain.DataPoint, error) {
	if len(tags) == 0 {
		return nil, nil
	}

	c.mu.Lock()
	c.lastUsed = time.Now()
	c.mu.Unlock()

	if !c.connected.Load() {
		return nil, domain.ErrConnectionClosed
	}

	// Group tags by area for batch reading
	groups := c.groupTagsByArea(tags)
	results := make([]*domain.DataPoint, 0, len(tags))

	for _, group := range groups {
		for _, tag := range group {
			select {
			case <-ctx.Done():
				return results, ctx.Err()
			default:
			}

			dp, err := c.ReadTag(ctx, tag)
			if err != nil {
				c.logger.Warn().Err(err).Str("tag", tag.ID).Msg("Failed to read tag")
			}
			results = append(results, dp)
		}
	}

	return results, nil
}

// WriteTag writes a value to a tag on the PLC.
func (c *Client) WriteTag(ctx context.Context, tag *domain.Tag, value interface{}) error {
	startTime := time.Now()
	defer func() {
		c.stats.TotalWriteTime.Add(time.Since(startTime).Nanoseconds())
	}()

	c.mu.Lock()
	c.lastUsed = time.Now()
	c.mu.Unlock()

	if !c.connected.Load() {
		c.recordTagError(tag.ID, domain.ErrConnectionClosed)
		return domain.ErrConnectionClosed
	}

	// Check if tag is writable
	if !c.isTagWritable(tag) {
		err := fmt.Errorf("%w: tag %s", domain.ErrTagNotWritable, tag.ID)
		c.recordTagError(tag.ID, err)
		return err
	}

	// Parse tag address
	area, dbNumber, offset, bitOffset, err := c.parseTagAddress(tag)
	if err != nil {
		c.stats.ErrorCount.Add(1)
		c.recordTagError(tag.ID, err)
		c.consecutiveFailures.Add(1)
		return err
	}

	// Execute write with retry logic
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			c.stats.RetryCount.Add(1)
			delay := c.calculateBackoff(attempt)
			c.logger.Debug().
				Int("attempt", attempt).
				Dur("delay", delay).
				Msg("Retrying S7 write")

			select {
			case <-ctx.Done():
				c.recordTagError(tag.ID, ctx.Err())
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err = c.writeData(tag, area, dbNumber, offset, bitOffset, value)
		if err == nil {
			break
		}

		// Check if error is retryable
		if !c.isRetryableError(err) {
			c.stats.ErrorCount.Add(1)
			c.recordTagError(tag.ID, err)
			c.consecutiveFailures.Add(1)
			return err
		}

		// Try to reconnect on connection errors
		if c.isConnectionError(err) {
			c.logger.Warn().Err(err).Msg("Connection error, attempting reconnect")
			c.reconnect(ctx)
		}
	}

	if err != nil {
		c.stats.ErrorCount.Add(1)
		c.recordTagError(tag.ID, err)
		c.consecutiveFailures.Add(1)
		return err
	}

	// Success - reset consecutive failures and record success
	c.consecutiveFailures.Store(0)
	c.recordTagSuccess(tag.ID)
	c.stats.WriteCount.Add(1)
	c.logger.Debug().
		Str("tag", tag.ID).
		Interface("value", value).
		Msg("Successfully wrote to S7 PLC")

	return nil
}

// readData performs the actual S7 read operation.
// Uses opMu to serialize operations - gos7 client is NOT thread-safe.
func (c *Client) readData(tag *domain.Tag, area domain.S7Area, dbNumber, offset, bitOffset int) (*domain.DataPoint, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, domain.ErrConnectionClosed
	}

	// Serialize S7 operations to prevent protocol corruption
	c.opMu.Lock()
	defer c.opMu.Unlock()

	// Calculate bytes to read based on data type
	byteCount := c.getByteCount(tag.DataType)
	buffer := BufferPool.Get(byteCount)
	defer BufferPool.Put(buffer)

	// Get the S7 area code
	areaCode, ok := S7AreaCode[area]
	if !ok {
		return nil, fmt.Errorf("%w: %s", domain.ErrS7InvalidArea, area)
	}

	// Read from PLC
	var err error
	switch area {
	case domain.S7AreaDB:
		err = client.AGReadDB(dbNumber, offset, byteCount, buffer)
	default:
		err = client.AGReadEB(offset, byteCount, buffer)
	}

	if err != nil {
		return nil, fmt.Errorf("%w: area=%s db=%d offset=%d: %v",
			domain.ErrS7ReadFailed, area, dbNumber, offset, err)
	}

	// Parse the raw bytes into a typed value
	value, err := c.parseValue(buffer, tag, bitOffset)
	if err != nil {
		return nil, err
	}

	// Apply scaling and offset
	scaledValue := c.applyScaling(value, tag)

	// Create data point
	dp := domain.NewDataPoint(
		c.deviceID,
		tag.ID,
		"", // Topic will be set by the caller
		scaledValue,
		tag.Unit,
		domain.QualityGood,
	).WithRawValue(value)

	_ = areaCode // Suppress unused variable warning
	return dp, nil
}

// writeData performs the actual S7 write operation.
// Uses opMu to serialize operations - gos7 client is NOT thread-safe.
func (c *Client) writeData(tag *domain.Tag, area domain.S7Area, dbNumber, offset, bitOffset int, value interface{}) error {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return domain.ErrConnectionClosed
	}

	// Serialize S7 operations to prevent protocol corruption
	c.opMu.Lock()
	defer c.opMu.Unlock()

	// For boolean writes, we need read-modify-write to preserve adjacent bits
	if tag.DataType == domain.DataTypeBool {
		return c.writeBoolWithRMW(client, tag, area, dbNumber, offset, bitOffset, value)
	}

	// Convert value to bytes
	buffer, err := c.valueToBytes(value, tag, bitOffset)
	if err != nil {
		return err
	}
	defer BufferPool.Put(buffer) // Return buffer to pool after use

	// Write to PLC
	switch area {
	case domain.S7AreaDB:
		err = client.AGWriteDB(dbNumber, offset, len(buffer), buffer)
	default:
		err = client.AGWriteEB(offset, len(buffer), buffer)
	}

	if err != nil {
		return fmt.Errorf("%w: area=%s db=%d offset=%d: %v",
			domain.ErrS7WriteFailed, area, dbNumber, offset, err)
	}

	return nil
}

// writeBoolWithRMW performs a read-modify-write for boolean values to preserve adjacent bits.
// Must be called with opMu already held.
func (c *Client) writeBoolWithRMW(client gos7.Client, tag *domain.Tag, area domain.S7Area, dbNumber, offset, bitOffset int, value interface{}) error {
	// Convert value to bool
	actualValue := c.reverseScaling(value, tag)
	b, ok := toBool(actualValue)
	if !ok {
		return fmt.Errorf("%w: cannot convert %T to bool", domain.ErrInvalidWriteValue, value)
	}

	// Read current byte
	currentByte := BufferPool.Get(1)
	defer BufferPool.Put(currentByte)

	var err error
	switch area {
	case domain.S7AreaDB:
		err = client.AGReadDB(dbNumber, offset, 1, currentByte)
	default:
		err = client.AGReadEB(offset, 1, currentByte)
	}

	if err != nil {
		return fmt.Errorf("%w: failed to read byte for RMW: %v", domain.ErrS7ReadFailed, err)
	}

	// Modify the specific bit
	if b {
		currentByte[0] |= (1 << bitOffset) // Set bit
	} else {
		currentByte[0] &^= (1 << bitOffset) // Clear bit
	}

	// Write back
	switch area {
	case domain.S7AreaDB:
		err = client.AGWriteDB(dbNumber, offset, 1, currentByte)
	default:
		err = client.AGWriteEB(offset, 1, currentByte)
	}

	if err != nil {
		return fmt.Errorf("%w: area=%s db=%d offset=%d bit=%d: %v",
			domain.ErrS7WriteFailed, area, dbNumber, offset, bitOffset, err)
	}

	return nil
}

// parseTagAddress extracts S7 address components from a tag.
func (c *Client) parseTagAddress(tag *domain.Tag) (domain.S7Area, int, int, int, error) {
	// If symbolic address is provided, parse it
	if tag.S7Address != "" {
		return c.parseSymbolicAddress(tag.S7Address)
	}

	// Use direct address components
	if tag.S7Area == "" {
		return "", 0, 0, 0, domain.ErrS7InvalidArea
	}

	return tag.S7Area, tag.S7DBNumber, tag.S7Offset, tag.S7BitOffset, nil
}

// parseSymbolicAddress parses S7 symbolic addresses like "DB1.DBD0", "MW100", "I0.0"
func (c *Client) parseSymbolicAddress(address string) (domain.S7Area, int, int, int, error) {
	address = strings.ToUpper(strings.TrimSpace(address))

	// Pattern for Data Block addresses: DB<n>.DB<type><offset>[.<bit>]
	// Examples: DB1.DBD0, DB1.DBW4, DB1.DBB8, DB1.DBX10.3
	dbPattern := regexp.MustCompile(`^DB(\d+)\.DB([XBWD])(\d+)(?:\.(\d))?$`)
	if matches := dbPattern.FindStringSubmatch(address); matches != nil {
		dbNum, _ := strconv.Atoi(matches[1])
		offset, _ := strconv.Atoi(matches[3])
		bitOffset := 0
		if matches[4] != "" {
			bitOffset, _ = strconv.Atoi(matches[4])
		}
		return domain.S7AreaDB, dbNum, offset, bitOffset, nil
	}

	// Pattern for Merker (flags): M<type><offset>[.<bit>] or MB<offset>, MW<offset>, MD<offset>
	// Examples: M0.0, MB0, MW0, MD0
	merkerPattern := regexp.MustCompile(`^M([BWD])?(\d+)(?:\.(\d))?$`)
	if matches := merkerPattern.FindStringSubmatch(address); matches != nil {
		offset, _ := strconv.Atoi(matches[2])
		bitOffset := 0
		if matches[3] != "" {
			bitOffset, _ = strconv.Atoi(matches[3])
		}
		return domain.S7AreaM, 0, offset, bitOffset, nil
	}

	// Pattern for Inputs: I<offset>.<bit> or IB<offset>, IW<offset>, ID<offset>
	// Examples: I0.0, IB0, IW0, ID0
	inputPattern := regexp.MustCompile(`^I([BWD])?(\d+)(?:\.(\d))?$`)
	if matches := inputPattern.FindStringSubmatch(address); matches != nil {
		offset, _ := strconv.Atoi(matches[2])
		bitOffset := 0
		if matches[3] != "" {
			bitOffset, _ = strconv.Atoi(matches[3])
		}
		return domain.S7AreaI, 0, offset, bitOffset, nil
	}

	// Pattern for Outputs: Q<offset>.<bit> or QB<offset>, QW<offset>, QD<offset>
	// Examples: Q0.0, QB0, QW0, QD0
	outputPattern := regexp.MustCompile(`^Q([BWD])?(\d+)(?:\.(\d))?$`)
	if matches := outputPattern.FindStringSubmatch(address); matches != nil {
		offset, _ := strconv.Atoi(matches[2])
		bitOffset := 0
		if matches[3] != "" {
			bitOffset, _ = strconv.Atoi(matches[3])
		}
		return domain.S7AreaQ, 0, offset, bitOffset, nil
	}

	// Pattern for Timers: T<number>
	timerPattern := regexp.MustCompile(`^T(\d+)$`)
	if matches := timerPattern.FindStringSubmatch(address); matches != nil {
		offset, _ := strconv.Atoi(matches[1])
		return domain.S7AreaT, 0, offset, 0, nil
	}

	// Pattern for Counters: C<number>
	counterPattern := regexp.MustCompile(`^C(\d+)$`)
	if matches := counterPattern.FindStringSubmatch(address); matches != nil {
		offset, _ := strconv.Atoi(matches[1])
		return domain.S7AreaC, 0, offset, 0, nil
	}

	return "", 0, 0, 0, fmt.Errorf("%w: %s", domain.ErrS7InvalidAddress, address)
}

// groupTagsByArea groups tags by S7 memory area for efficient batch reads.
func (c *Client) groupTagsByArea(tags []*domain.Tag) map[domain.S7Area][]*domain.Tag {
	groups := make(map[domain.S7Area][]*domain.Tag)
	for _, tag := range tags {
		area := tag.S7Area
		if tag.S7Address != "" {
			parsedArea, _, _, _, err := c.parseSymbolicAddress(tag.S7Address)
			if err == nil {
				area = parsedArea
			}
		}
		groups[area] = append(groups[area], tag)
	}
	return groups
}

// isTagWritable checks if a tag is writable based on its area and access mode.
func (c *Client) isTagWritable(tag *domain.Tag) bool {
	// Check explicit access mode first
	if tag.AccessMode != "" {
		return tag.AccessMode == domain.AccessModeWriteOnly || tag.AccessMode == domain.AccessModeReadWrite
	}

	// For S7, determine writability from area
	area := tag.S7Area
	if tag.S7Address != "" {
		parsedArea, _, _, _, err := c.parseSymbolicAddress(tag.S7Address)
		if err == nil {
			area = parsedArea
		}
	}

	switch area {
	case domain.S7AreaDB, domain.S7AreaM, domain.S7AreaQ:
		return true // DB, Merkers, and Outputs are writable
	case domain.S7AreaI:
		return false // Inputs are read-only
	case domain.S7AreaT, domain.S7AreaC:
		return true // Timers and Counters can be written
	default:
		return false
	}
}

// createErrorDataPoint creates a data point with error quality.
func (c *Client) createErrorDataPoint(tag *domain.Tag, err error) *domain.DataPoint {
	quality := domain.QualityBad
	if c.isConnectionError(err) {
		quality = domain.QualityNotConnected
	}

	return domain.NewDataPoint(
		c.deviceID,
		tag.ID,
		"",
		nil,
		tag.Unit,
		quality,
	)
}

// calculateBackoff calculates exponential backoff delay.
func (c *Client) calculateBackoff(attempt int) time.Duration {
	delay := c.config.RetryDelay * time.Duration(1<<uint(attempt))
	maxDelay := 10 * time.Second
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}

// isRetryableError determines if an error is transient and worth retrying.
func (c *Client) isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	return c.isConnectionError(err)
}

// isConnectionError checks if the error is a connection-related error.
func (c *Client) isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "closed") ||
		strings.Contains(errStr, "refused") ||
		strings.Contains(errStr, "reset")
}

// reconnect attempts to re-establish the connection.
func (c *Client) reconnect(ctx context.Context) {
	c.stats.ReconnectCount.Add(1)
	c.Disconnect()
	if err := c.Connect(ctx); err != nil {
		c.mu.Lock()
		c.lastError = err
		c.mu.Unlock()
		c.logger.Error().Err(err).Msg("Failed to reconnect to S7 PLC")
	} else {
		c.mu.Lock()
		c.lastError = nil
		c.mu.Unlock()
		c.logger.Info().Msg("Reconnected to S7 PLC")
	}
}
