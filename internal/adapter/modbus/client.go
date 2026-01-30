// Package modbus provides a production-grade Modbus TCP/RTU client implementation
// with connection pooling, circuit breaker, retry logic, and comprehensive error handling.
package modbus

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"time"

	"github.com/goburrow/modbus"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/rs/zerolog"
)

// NewClient creates a new Modbus client with the given configuration.
func NewClient(deviceID string, config ClientConfig, logger zerolog.Logger) (*Client, error) {
	if config.Address == "" {
		return nil, fmt.Errorf("modbus address is required")
	}
	if config.SlaveID == 0 || config.SlaveID > 247 {
		return nil, domain.ErrInvalidSlaveID
	}
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}
	if config.IdleTimeout == 0 {
		config.IdleTimeout = 30 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 100 * time.Millisecond
	}

	c := &Client{
		config:   config,
		logger:   logger.With().Str("device_id", deviceID).Str("address", config.Address).Logger(),
		stats:    &ClientStats{},
		deviceID: deviceID,
		lastUsed: time.Now(),
	}

	return c, nil
}

// Connect establishes the connection to the Modbus device.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected.Load() {
		return nil
	}

	c.logger.Debug().Msg("Connecting to Modbus device")

	// Create TCP handler
	handler := modbus.NewTCPClientHandler(c.config.Address)
	handler.Timeout = c.config.Timeout
	handler.SlaveId = c.config.SlaveID
	handler.IdleTimeout = c.config.IdleTimeout

	// Use context for connection timeout
	connectDone := make(chan error, 1)
	go func() {
		connectDone <- handler.Connect()
	}()

	select {
	case err := <-connectDone:
		if err != nil {
			c.lastError = err
			return fmt.Errorf("%w: %v", domain.ErrConnectionFailed, err)
		}
	case <-ctx.Done():
		return fmt.Errorf("%w: %v", domain.ErrConnectionTimeout, ctx.Err())
	}

	c.handler = handler
	c.client = modbus.NewClient(handler)
	c.connected.Store(true)
	c.lastError = nil
	c.lastUsed = time.Now()

	c.logger.Info().Msg("Connected to Modbus device")
	return nil
}

// Disconnect closes the connection to the Modbus device.
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected.Load() {
		return nil
	}

	if c.handler != nil {
		if err := c.handler.Close(); err != nil {
			c.logger.Warn().Err(err).Msg("Error closing Modbus connection")
		}
	}

	c.connected.Store(false)
	c.handler = nil
	c.client = nil

	c.logger.Debug().Msg("Disconnected from Modbus device")
	return nil
}

// IsConnected returns true if the client is currently connected.
func (c *Client) IsConnected() bool {
	return c.connected.Load()
}

// ReadTag reads a single tag from the device.
func (c *Client) ReadTag(ctx context.Context, tag *domain.Tag) (*domain.DataPoint, error) {
	startTime := time.Now()
	defer func() {
		c.stats.TotalReadTime.Add(time.Since(startTime).Nanoseconds())
	}()

	c.mu.Lock()
	c.lastUsed = time.Now()
	c.mu.Unlock()

	if !c.connected.Load() {
		return nil, domain.ErrConnectionClosed
	}

	var rawBytes []byte
	var err error

	// Execute read with retry logic
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			c.stats.RetryCount.Add(1)
			delay := c.calculateBackoff(attempt)
			c.logger.Debug().
				Int("attempt", attempt).
				Dur("delay", delay).
				Msg("Retrying Modbus read")

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		rawBytes, err = c.readRegisters(tag)
		if err == nil {
			break
		}

		// Check if error is retryable
		if !c.isRetryableError(err) {
			c.stats.ErrorCount.Add(1)
			c.recordTagError(tag.ID, err)
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
		return c.createErrorDataPoint(tag, err), err
	}

	c.stats.ReadCount.Add(1)
	c.recordTagSuccess(tag.ID)

	// Parse the raw bytes into a typed value
	value, err := parseValue(rawBytes, tag)
	if err != nil {
		return c.createErrorDataPoint(tag, err), err
	}

	// Apply scaling and offset
	scaledValue := applyScaling(value, tag)

	// Create data point
	dp := domain.NewDataPoint(
		c.deviceID,
		tag.ID,
		"", // Topic will be set by the caller
		scaledValue,
		tag.Unit,
		domain.QualityGood,
	).WithRawValue(value)

	return dp, nil
}

// ReadTags reads multiple tags efficiently using optimized register grouping.
func (c *Client) ReadTags(ctx context.Context, tags []*domain.Tag) ([]*domain.DataPoint, error) {
	if len(tags) == 0 {
		return nil, nil
	}

	// Group tags by register type for efficient batch reads
	groups := c.groupTagsByType(tags)
	results := make([]*domain.DataPoint, 0, len(tags))

	for _, group := range groups {
		groupResults, err := c.readTagGroup(ctx, group)
		if err != nil {
			c.logger.Error().Err(err).Msg("Error reading tag group")
			// Continue with other groups, add error points for failed tags
			for _, tag := range group {
				results = append(results, c.createErrorDataPoint(tag, err))
			}
			continue
		}
		results = append(results, groupResults...)
	}

	return results, nil
}

// readRegisters performs the actual Modbus read operation.
// Uses opMu to serialize operations - goburrow/modbus Client is NOT thread-safe.
func (c *Client) readRegisters(tag *domain.Tag) ([]byte, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, domain.ErrConnectionClosed
	}

	// Serialize all Modbus operations to prevent protocol corruption
	c.opMu.Lock()
	defer c.opMu.Unlock()

	var result []byte
	var err error

	switch tag.RegisterType {
	case domain.RegisterTypeCoil:
		result, err = client.ReadCoils(tag.Address, tag.RegisterCount)
	case domain.RegisterTypeDiscreteInput:
		result, err = client.ReadDiscreteInputs(tag.Address, tag.RegisterCount)
	case domain.RegisterTypeHoldingRegister:
		result, err = client.ReadHoldingRegisters(tag.Address, tag.RegisterCount)
	case domain.RegisterTypeInputRegister:
		result, err = client.ReadInputRegisters(tag.Address, tag.RegisterCount)
	default:
		return nil, domain.ErrInvalidRegisterType
	}

	if err != nil {
		c.consecutiveFailures.Add(1)
		return nil, c.translateModbusError(err)
	}

	c.consecutiveFailures.Store(0) // Reset on success
	return result, nil
}

// groupTagsByType groups tags by register type for efficient batch reads.
func (c *Client) groupTagsByType(tags []*domain.Tag) [][]*domain.Tag {
	groups := make(map[domain.RegisterType][]*domain.Tag)
	for _, tag := range tags {
		groups[tag.RegisterType] = append(groups[tag.RegisterType], tag)
	}

	result := make([][]*domain.Tag, 0, len(groups))
	for _, group := range groups {
		result = append(result, group)
	}
	return result
}

// buildContiguousRanges groups tags into contiguous address ranges for batched reads.
// This is the "performance crown jewel" - reduces N reads to 1-5 reads for 100+ tags.
func (c *Client) buildContiguousRanges(tags []*domain.Tag, config BatchConfig) []RegisterRange {
	if len(tags) == 0 {
		return nil
	}

	// Sort tags by address
	sorted := make([]*domain.Tag, len(tags))
	copy(sorted, tags)
	sortTagsByAddress(sorted)

	var ranges []RegisterRange
	currentRange := RegisterRange{
		StartAddress: sorted[0].Address,
		EndAddress:   sorted[0].Address + sorted[0].RegisterCount - 1,
		Tags:         []*domain.Tag{sorted[0]},
	}

	for i := 1; i < len(sorted); i++ {
		tag := sorted[i]
		tagEnd := tag.Address + tag.RegisterCount - 1

		// Calculate gap between current range end and this tag's start
		gap := int(tag.Address) - int(currentRange.EndAddress) - 1

		// Calculate new range size if we merge
		newRangeSize := tagEnd - currentRange.StartAddress + 1

		// Merge if: gap is acceptable AND total size is within limits
		if gap <= int(config.MaxGapSize) && newRangeSize <= config.MaxRegistersPerRead {
			// Extend current range
			if tagEnd > currentRange.EndAddress {
				currentRange.EndAddress = tagEnd
			}
			currentRange.Tags = append(currentRange.Tags, tag)
		} else {
			// Start new range
			ranges = append(ranges, currentRange)
			currentRange = RegisterRange{
				StartAddress: tag.Address,
				EndAddress:   tagEnd,
				Tags:         []*domain.Tag{tag},
			}
		}
	}
	// Don't forget the last range
	ranges = append(ranges, currentRange)

	return ranges
}

// sortTagsByAddress sorts tags by address in ascending order (insertion sort for small slices).
func sortTagsByAddress(tags []*domain.Tag) {
	for i := 1; i < len(tags); i++ {
		j := i
		for j > 0 && tags[j].Address < tags[j-1].Address {
			tags[j], tags[j-1] = tags[j-1], tags[j]
			j--
		}
	}
}

// readTagGroup reads a group of tags of the same register type using batched reads.
// Uses range-based batching to minimize Modbus operations.
func (c *Client) readTagGroup(ctx context.Context, tags []*domain.Tag) ([]*domain.DataPoint, error) {
	if len(tags) == 0 {
		return nil, nil
	}

	// For coils/discrete inputs, batch differently (they're packed in bits)
	if tags[0].RegisterType == domain.RegisterTypeCoil ||
		tags[0].RegisterType == domain.RegisterTypeDiscreteInput {
		return c.readTagGroupIndividually(ctx, tags)
	}

	// Build contiguous ranges for holding/input registers
	config := DefaultBatchConfig()
	ranges := c.buildContiguousRanges(tags, config)

	c.logger.Debug().
		Int("tags", len(tags)).
		Int("ranges", len(ranges)).
		Msg("Batched tags into register ranges")

	results := make([]*domain.DataPoint, 0, len(tags))

	for _, rng := range ranges {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		rangeResults, err := c.readRegisterRange(ctx, rng, tags[0].RegisterType)
		if err != nil {
			c.logger.Warn().Err(err).
				Uint16("start", rng.StartAddress).
				Uint16("end", rng.EndAddress).
				Msg("Failed to read register range")
			// Add error points for all tags in this range
			for _, tag := range rng.Tags {
				results = append(results, c.createErrorDataPoint(tag, err))
			}
			continue
		}
		results = append(results, rangeResults...)
	}

	return results, nil
}

// readRegisterRange reads a contiguous range of registers and extracts individual tag values.
func (c *Client) readRegisterRange(ctx context.Context, rng RegisterRange, regType domain.RegisterType) ([]*domain.DataPoint, error) {
	registerCount := rng.EndAddress - rng.StartAddress + 1

	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, domain.ErrConnectionClosed
	}

	// Read the entire range in one Modbus operation
	c.opMu.Lock()
	var rawData []byte
	var err error
	switch regType {
	case domain.RegisterTypeHoldingRegister:
		rawData, err = client.ReadHoldingRegisters(rng.StartAddress, registerCount)
	case domain.RegisterTypeInputRegister:
		rawData, err = client.ReadInputRegisters(rng.StartAddress, registerCount)
	default:
		c.opMu.Unlock()
		return nil, domain.ErrInvalidRegisterType
	}
	c.opMu.Unlock()

	if err != nil {
		c.consecutiveFailures.Add(1)
		return nil, c.translateModbusError(err)
	}
	c.consecutiveFailures.Store(0)
	c.stats.ReadCount.Add(1)

	// Extract individual tag values from the raw data
	results := make([]*domain.DataPoint, 0, len(rng.Tags))
	for _, tag := range rng.Tags {
		// Calculate offset within the raw data
		offset := int(tag.Address-rng.StartAddress) * 2 // 2 bytes per register
		tagByteLen := int(tag.RegisterCount) * 2

		if offset < 0 || offset+tagByteLen > len(rawData) {
			c.logger.Error().
				Str("tag", tag.ID).
				Uint16("address", tag.Address).
				Int("offset", offset).
				Int("len", len(rawData)).
				Msg("Tag data out of range")
			c.recordTagError(tag.ID, domain.ErrInvalidDataLength)
			results = append(results, c.createErrorDataPoint(tag, domain.ErrInvalidDataLength))
			continue
		}

		tagData := rawData[offset : offset+tagByteLen]

		value, err := parseValue(tagData, tag)
		if err != nil {
			c.recordTagError(tag.ID, err)
			results = append(results, c.createErrorDataPoint(tag, err))
			continue
		}

		scaledValue := applyScaling(value, tag)
		c.recordTagSuccess(tag.ID)

		dp := domain.NewDataPoint(
			c.deviceID,
			tag.ID,
			"",
			scaledValue,
			tag.Unit,
			domain.QualityGood,
		).WithRawValue(value).WithPriority(tag.Priority)

		results = append(results, dp)
	}

	return results, nil
}

// readTagGroupIndividually reads tags one by one (fallback for coils/discrete inputs).
func (c *Client) readTagGroupIndividually(ctx context.Context, tags []*domain.Tag) ([]*domain.DataPoint, error) {
	results := make([]*domain.DataPoint, 0, len(tags))
	for _, tag := range tags {
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
	return results, nil
}

// createErrorDataPoint creates a data point with error quality.
func (c *Client) createErrorDataPoint(tag *domain.Tag, err error) *domain.DataPoint {
	quality := domain.QualityBad
	switch {
	case isTimeout(err):
		quality = domain.QualityTimeout
	case c.isConnectionError(err):
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

// calculateBackoff calculates exponential backoff delay with jitter.
// Jitter prevents reconnection storms when multiple clients fail simultaneously.
func (c *Client) calculateBackoff(attempt int) time.Duration {
	delay := c.config.RetryDelay * time.Duration(1<<uint(attempt))
	maxDelay := 10 * time.Second
	if delay > maxDelay {
		delay = maxDelay
	}
	// Add ±25% jitter to prevent thundering herd
	jitter := time.Duration(rand.Int64N(int64(delay)/2)) - (delay / 4)
	return delay + jitter
}

// isRetryableError determines if an error is transient and worth retrying.
func (c *Client) isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// Retry on timeouts and connection errors
	return isTimeout(err) || c.isConnectionError(err)
}

// isConnectionError checks if the error is a connection-related error.
// Includes io.EOF which goburrow/modbus returns on connection drops.
func (c *Client) isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	// Check for network errors
	if _, ok := err.(net.Error); ok {
		return true
	}
	// goburrow/modbus often returns EOF on connection drops
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	// Check for common connection-related error strings
	errStr := err.Error()
	if contains(errStr, "connection reset", "broken pipe", "connection refused", "no route to host") {
		return true
	}
	return false
}

// contains checks if s contains any of the substrings.
func contains(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// isTimeout checks if the error is a timeout error.
func isTimeout(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}

// translateModbusError converts Modbus library errors to domain errors.
func (c *Client) translateModbusError(err error) error {
	if err == nil {
		return nil
	}
	// The goburrow/modbus library returns exception codes in error messages
	// We'll wrap the original error for now
	return fmt.Errorf("%w: %v", domain.ErrReadFailed, err)
}

// reconnect attempts to re-establish the connection.
func (c *Client) reconnect(ctx context.Context) {
	c.Disconnect()
	if err := c.Connect(ctx); err != nil {
		c.logger.Error().Err(err).Msg("Failed to reconnect")
	}
}

// WriteTag writes a value to a tag on the device.
func (c *Client) WriteTag(ctx context.Context, tag *domain.Tag, value interface{}) error {
	startTime := time.Now()
	defer func() {
		c.stats.TotalWriteTime.Add(time.Since(startTime).Nanoseconds())
	}()

	c.mu.Lock()
	c.lastUsed = time.Now()
	c.mu.Unlock()

	if !c.connected.Load() {
		return domain.ErrConnectionClosed
	}

	// Check if tag is writable
	if !tag.IsWritable() {
		return fmt.Errorf("%w: tag %s (register type: %s)", domain.ErrTagNotWritable, tag.ID, tag.RegisterType)
	}

	var err error

	// Execute write with retry logic
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			c.stats.RetryCount.Add(1)
			delay := c.calculateBackoff(attempt)
			c.logger.Debug().
				Int("attempt", attempt).
				Dur("delay", delay).
				Msg("Retrying Modbus write")

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err = c.writeRegister(tag, value)
		if err == nil {
			break
		}

		// Check if error is retryable
		if !c.isRetryableError(err) {
			c.stats.ErrorCount.Add(1)
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
		return err
	}

	c.stats.WriteCount.Add(1)
	c.logger.Debug().
		Str("tag", tag.ID).
		Interface("value", value).
		Msg("Successfully wrote to Modbus register")

	return nil
}

// writeRegister performs the actual Modbus write operation.
// Uses opMu to serialize operations - goburrow/modbus Client is NOT thread-safe.
func (c *Client) writeRegister(tag *domain.Tag, value interface{}) error {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return domain.ErrConnectionClosed
	}

	// Serialize all Modbus operations to prevent protocol corruption
	c.opMu.Lock()
	defer c.opMu.Unlock()

	var err error
	switch tag.RegisterType {
	case domain.RegisterTypeCoil:
		err = c.writeSingleCoilLocked(client, tag.Address, value)
	case domain.RegisterTypeHoldingRegister:
		err = c.writeHoldingRegisterLocked(client, tag, value)
	default:
		return fmt.Errorf("%w: %s is read-only", domain.ErrTagNotWritable, tag.RegisterType)
	}

	if err != nil {
		c.consecutiveFailures.Add(1)
		return err
	}
	c.consecutiveFailures.Store(0)
	return nil
}

// writeSingleCoilLocked writes a boolean value to a coil (function code 0x05).
// Caller must hold opMu.
func (c *Client) writeSingleCoilLocked(client modbus.Client, address uint16, value interface{}) error {
	boolValue, ok := toBool(value)
	if !ok {
		return fmt.Errorf("%w: cannot convert %T to bool for coil", domain.ErrInvalidWriteValue, value)
	}

	var coilValue uint16
	if boolValue {
		coilValue = 0xFF00 // ON
	} else {
		coilValue = 0x0000 // OFF
	}

	_, err := client.WriteSingleCoil(address, coilValue)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrWriteFailed, err)
	}

	return nil
}

// writeHoldingRegisterLocked writes a value to holding register(s).
// Caller must hold opMu.
func (c *Client) writeHoldingRegisterLocked(client modbus.Client, tag *domain.Tag, value interface{}) error {
	// Convert value to bytes based on data type
	bytes, err := valueToBytes(value, tag)
	if err != nil {
		return err
	}

	// Single register write (function code 0x06)
	if len(bytes) == 2 {
		regValue := binary.BigEndian.Uint16(bytes)
		_, err := client.WriteSingleRegister(tag.Address, regValue)
		if err != nil {
			return fmt.Errorf("%w: %v", domain.ErrWriteFailed, err)
		}
		return nil
	}

	// Multiple register write (function code 0x10)
	quantity := uint16(len(bytes) / 2)
	_, err = client.WriteMultipleRegisters(tag.Address, quantity, bytes)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrWriteFailed, err)
	}

	return nil
}

// WriteSingleCoil writes a boolean value to a coil at the specified address.
func (c *Client) WriteSingleCoil(ctx context.Context, address uint16, value bool) error {
	c.mu.Lock()
	c.lastUsed = time.Now()
	c.mu.Unlock()

	if !c.connected.Load() {
		return domain.ErrConnectionClosed
	}

	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return domain.ErrConnectionClosed
	}

	var coilValue uint16
	if value {
		coilValue = 0xFF00
	}

	// Serialize all Modbus operations
	c.opMu.Lock()
	_, err := client.WriteSingleCoil(address, coilValue)
	c.opMu.Unlock()

	if err != nil {
		c.stats.ErrorCount.Add(1)
		c.consecutiveFailures.Add(1)
		return fmt.Errorf("%w: %v", domain.ErrWriteFailed, err)
	}

	c.stats.WriteCount.Add(1)
	c.consecutiveFailures.Store(0)
	return nil
}

// WriteSingleRegister writes a 16-bit value to a holding register.
func (c *Client) WriteSingleRegister(ctx context.Context, address uint16, value uint16) error {
	c.mu.Lock()
	c.lastUsed = time.Now()
	c.mu.Unlock()

	if !c.connected.Load() {
		return domain.ErrConnectionClosed
	}

	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return domain.ErrConnectionClosed
	}

	// Serialize all Modbus operations
	c.opMu.Lock()
	_, err := client.WriteSingleRegister(address, value)
	c.opMu.Unlock()

	if err != nil {
		c.stats.ErrorCount.Add(1)
		c.consecutiveFailures.Add(1)
		return fmt.Errorf("%w: %v", domain.ErrWriteFailed, err)
	}

	c.stats.WriteCount.Add(1)
	c.consecutiveFailures.Store(0)
	return nil
}

// WriteMultipleRegisters writes multiple 16-bit values to consecutive holding registers.
func (c *Client) WriteMultipleRegisters(ctx context.Context, address uint16, values []uint16) error {
	c.mu.Lock()
	c.lastUsed = time.Now()
	c.mu.Unlock()

	if !c.connected.Load() {
		return domain.ErrConnectionClosed
	}

	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return domain.ErrConnectionClosed
	}

	// Convert to bytes
	bytes := make([]byte, len(values)*2)
	for i, v := range values {
		binary.BigEndian.PutUint16(bytes[i*2:], v)
	}

	// Serialize all Modbus operations
	c.opMu.Lock()
	_, err := client.WriteMultipleRegisters(address, uint16(len(values)), bytes)
	c.opMu.Unlock()

	if err != nil {
		c.stats.ErrorCount.Add(1)
		c.consecutiveFailures.Add(1)
		return fmt.Errorf("%w: %v", domain.ErrWriteFailed, err)
	}

	c.stats.WriteCount.Add(1)
	c.consecutiveFailures.Store(0)
	return nil
}

// WriteMultipleCoils writes multiple boolean values to consecutive coils.
func (c *Client) WriteMultipleCoils(ctx context.Context, address uint16, values []bool) error {
	c.mu.Lock()
	c.lastUsed = time.Now()
	c.mu.Unlock()

	if !c.connected.Load() {
		return domain.ErrConnectionClosed
	}

	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return domain.ErrConnectionClosed
	}

	// Pack bools into bytes
	byteCount := (len(values) + 7) / 8
	bytes := make([]byte, byteCount)
	for i, v := range values {
		if v {
			bytes[i/8] |= 1 << (i % 8)
		}
	}

	// Serialize all Modbus operations
	c.opMu.Lock()
	_, err := client.WriteMultipleCoils(address, uint16(len(values)), bytes)
	c.opMu.Unlock()

	if err != nil {
		c.stats.ErrorCount.Add(1)
		c.consecutiveFailures.Add(1)
		return fmt.Errorf("%w: %v", domain.ErrWriteFailed, err)
	}

	c.stats.WriteCount.Add(1)
	c.consecutiveFailures.Store(0)
	return nil
}
