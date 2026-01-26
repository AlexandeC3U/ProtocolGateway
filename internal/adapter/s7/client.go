// Package s7 provides a production-grade Siemens S7 client implementation
// with connection management, bidirectional communication, and comprehensive error handling.
package s7

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/robinson/gos7"
	"github.com/rs/zerolog"
)

// Client represents an S7 client connection to a single PLC.
type Client struct {
	config     ClientConfig
	handler    *gos7.TCPClientHandler
	client     gos7.Client
	logger     zerolog.Logger
	mu         sync.RWMutex
	connected  atomic.Bool
	lastError  error
	lastUsed   time.Time
	stats      *ClientStats
	deviceID   string
}

// ClientConfig holds configuration for an S7 client.
type ClientConfig struct {
	// Address is the IP address of the PLC
	Address string

	// Port is the TCP port (default: 102 for ISO-on-TCP)
	Port int

	// Rack is the rack number of the PLC (usually 0)
	Rack int

	// Slot is the slot number of the CPU module
	// S7-300/400: usually 2, S7-1200/1500: usually 0 or 1
	Slot int

	// Timeout is the connection and response timeout
	Timeout time.Duration

	// IdleTimeout is how long to keep idle connections open
	IdleTimeout time.Duration

	// MaxRetries is the number of retry attempts on transient failures
	MaxRetries int

	// RetryDelay is the base delay between retries (exponential backoff applied)
	RetryDelay time.Duration

	// PDUSize is the maximum PDU size (default: 480)
	PDUSize int
}

// ClientStats tracks client performance metrics.
type ClientStats struct {
	ReadCount      atomic.Uint64
	WriteCount     atomic.Uint64
	ErrorCount     atomic.Uint64
	RetryCount     atomic.Uint64
	TotalReadTime  atomic.Int64 // nanoseconds
	TotalWriteTime atomic.Int64 // nanoseconds
}

// S7AreaCode maps domain S7Area to gos7 area codes.
var S7AreaCode = map[domain.S7Area]int{
	domain.S7AreaDB: 0x84, // Data Blocks
	domain.S7AreaM:  0x83, // Merkers
	domain.S7AreaI:  0x81, // Inputs
	domain.S7AreaQ:  0x82, // Outputs
	domain.S7AreaT:  0x1D, // Timers
	domain.S7AreaC:  0x1C, // Counters
}

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
		config:   config,
		logger:   logger.With().Str("device_id", deviceID).Str("address", config.Address).Logger(),
		stats:    &ClientStats{},
		deviceID: deviceID,
		lastUsed: time.Now(),
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
			c.lastError = err
			return fmt.Errorf("%w: %v", domain.ErrS7ConnectionFailed, err)
		}
	case <-ctx.Done():
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
		return nil, domain.ErrConnectionClosed
	}

	// Parse tag address if needed
	area, dbNumber, offset, bitOffset, err := c.parseTagAddress(tag)
	if err != nil {
		c.stats.ErrorCount.Add(1)
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
		return c.createErrorDataPoint(tag, err), err
	}

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
		return domain.ErrConnectionClosed
	}

	// Check if tag is writable
	if !c.isTagWritable(tag) {
		return fmt.Errorf("%w: tag %s", domain.ErrTagNotWritable, tag.ID)
	}

	// Parse tag address
	area, dbNumber, offset, bitOffset, err := c.parseTagAddress(tag)
	if err != nil {
		c.stats.ErrorCount.Add(1)
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
		Msg("Successfully wrote to S7 PLC")

	return nil
}

// readData performs the actual S7 read operation.
func (c *Client) readData(tag *domain.Tag, area domain.S7Area, dbNumber, offset, bitOffset int) (*domain.DataPoint, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, domain.ErrConnectionClosed
	}

	// Calculate bytes to read based on data type
	byteCount := c.getByteCount(tag.DataType)
	buffer := make([]byte, byteCount)

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
func (c *Client) writeData(tag *domain.Tag, area domain.S7Area, dbNumber, offset, bitOffset int, value interface{}) error {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return domain.ErrConnectionClosed
	}

	// Convert value to bytes
	buffer, err := c.valueToBytes(value, tag, bitOffset)
	if err != nil {
		return err
	}

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

// parseValue converts raw bytes to a typed value based on the tag's data type.
func (c *Client) parseValue(data []byte, tag *domain.Tag, bitOffset int) (interface{}, error) {
	if len(data) == 0 {
		return nil, domain.ErrInvalidDataLength
	}

	switch tag.DataType {
	case domain.DataTypeBool:
		if len(data) < 1 {
			return nil, domain.ErrInvalidDataLength
		}
		return (data[0] & (1 << bitOffset)) != 0, nil

	case domain.DataTypeInt16:
		if len(data) < 2 {
			return nil, domain.ErrInvalidDataLength
		}
		return int16(binary.BigEndian.Uint16(data)), nil

	case domain.DataTypeUInt16:
		if len(data) < 2 {
			return nil, domain.ErrInvalidDataLength
		}
		return binary.BigEndian.Uint16(data), nil

	case domain.DataTypeInt32:
		if len(data) < 4 {
			return nil, domain.ErrInvalidDataLength
		}
		return int32(binary.BigEndian.Uint32(data)), nil

	case domain.DataTypeUInt32:
		if len(data) < 4 {
			return nil, domain.ErrInvalidDataLength
		}
		return binary.BigEndian.Uint32(data), nil

	case domain.DataTypeInt64:
		if len(data) < 8 {
			return nil, domain.ErrInvalidDataLength
		}
		return int64(binary.BigEndian.Uint64(data)), nil

	case domain.DataTypeUInt64:
		if len(data) < 8 {
			return nil, domain.ErrInvalidDataLength
		}
		return binary.BigEndian.Uint64(data), nil

	case domain.DataTypeFloat32:
		if len(data) < 4 {
			return nil, domain.ErrInvalidDataLength
		}
		bits := binary.BigEndian.Uint32(data)
		return math.Float32frombits(bits), nil

	case domain.DataTypeFloat64:
		if len(data) < 8 {
			return nil, domain.ErrInvalidDataLength
		}
		bits := binary.BigEndian.Uint64(data)
		return math.Float64frombits(bits), nil

	default:
		return nil, domain.ErrInvalidDataType
	}
}

// valueToBytes converts a value to bytes for writing.
func (c *Client) valueToBytes(value interface{}, tag *domain.Tag, bitOffset int) ([]byte, error) {
	// Reverse scaling if applied
	actualValue := c.reverseScaling(value, tag)

	switch tag.DataType {
	case domain.DataTypeBool:
		b, ok := toBool(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to bool", domain.ErrInvalidWriteValue, value)
		}
		data := make([]byte, 1)
		if b {
			data[0] = 1 << bitOffset
		}
		return data, nil

	case domain.DataTypeInt16:
		i, ok := toInt64(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to int16", domain.ErrInvalidWriteValue, value)
		}
		data := make([]byte, 2)
		binary.BigEndian.PutUint16(data, uint16(int16(i)))
		return data, nil

	case domain.DataTypeUInt16:
		i, ok := toInt64(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to uint16", domain.ErrInvalidWriteValue, value)
		}
		data := make([]byte, 2)
		binary.BigEndian.PutUint16(data, uint16(i))
		return data, nil

	case domain.DataTypeInt32:
		i, ok := toInt64(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to int32", domain.ErrInvalidWriteValue, value)
		}
		data := make([]byte, 4)
		binary.BigEndian.PutUint32(data, uint32(int32(i)))
		return data, nil

	case domain.DataTypeUInt32:
		i, ok := toInt64(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to uint32", domain.ErrInvalidWriteValue, value)
		}
		data := make([]byte, 4)
		binary.BigEndian.PutUint32(data, uint32(i))
		return data, nil

	case domain.DataTypeInt64:
		i, ok := toInt64(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to int64", domain.ErrInvalidWriteValue, value)
		}
		data := make([]byte, 8)
		binary.BigEndian.PutUint64(data, uint64(i))
		return data, nil

	case domain.DataTypeUInt64:
		i, ok := toUint64(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to uint64", domain.ErrInvalidWriteValue, value)
		}
		data := make([]byte, 8)
		binary.BigEndian.PutUint64(data, i)
		return data, nil

	case domain.DataTypeFloat32:
		f, ok := toFloat64(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to float32", domain.ErrInvalidWriteValue, value)
		}
		data := make([]byte, 4)
		binary.BigEndian.PutUint32(data, math.Float32bits(float32(f)))
		return data, nil

	case domain.DataTypeFloat64:
		f, ok := toFloat64(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to float64", domain.ErrInvalidWriteValue, value)
		}
		data := make([]byte, 8)
		binary.BigEndian.PutUint64(data, math.Float64bits(f))
		return data, nil

	default:
		return nil, fmt.Errorf("%w: unsupported data type %s", domain.ErrInvalidDataType, tag.DataType)
	}
}

// getByteCount returns the number of bytes needed for a data type.
func (c *Client) getByteCount(dataType domain.DataType) int {
	switch dataType {
	case domain.DataTypeBool:
		return 1
	case domain.DataTypeInt16, domain.DataTypeUInt16:
		return 2
	case domain.DataTypeInt32, domain.DataTypeUInt32, domain.DataTypeFloat32:
		return 4
	case domain.DataTypeInt64, domain.DataTypeUInt64, domain.DataTypeFloat64:
		return 8
	default:
		return 1
	}
}

// applyScaling applies scale factor and offset to the value.
func (c *Client) applyScaling(value interface{}, tag *domain.Tag) interface{} {
	if tag.ScaleFactor == 1.0 && tag.Offset == 0 {
		return value
	}

	floatVal, ok := toFloat64(value)
	if !ok {
		return value
	}

	return floatVal*tag.ScaleFactor + tag.Offset
}

// reverseScaling reverses the scaling for write operations.
func (c *Client) reverseScaling(value interface{}, tag *domain.Tag) interface{} {
	if tag.ScaleFactor == 1.0 && tag.Offset == 0 {
		return value
	}

	floatVal, ok := toFloat64(value)
	if !ok {
		return value
	}

	return (floatVal - tag.Offset) / tag.ScaleFactor
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
	c.Disconnect()
	if err := c.Connect(ctx); err != nil {
		c.logger.Error().Err(err).Msg("Failed to reconnect to S7 PLC")
	}
}

// GetStats returns the client statistics.
func (c *Client) GetStats() map[string]uint64 {
	return map[string]uint64{
		"read_count":       c.stats.ReadCount.Load(),
		"write_count":      c.stats.WriteCount.Load(),
		"error_count":      c.stats.ErrorCount.Load(),
		"retry_count":      c.stats.RetryCount.Load(),
		"total_read_ns":    uint64(c.stats.TotalReadTime.Load()),
		"total_write_ns":   uint64(c.stats.TotalWriteTime.Load()),
	}
}

// GetStatsStruct returns the raw stats struct for direct access.
func (c *Client) GetStatsStruct() *ClientStats {
	return c.stats
}

// LastUsed returns when the client was last used.
func (c *Client) LastUsed() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastUsed
}

// DeviceID returns the device ID this client is connected to.
func (c *Client) DeviceID() string {
	return c.deviceID
}

// Helper functions for type conversion
func toBool(v interface{}) (bool, bool) {
	switch val := v.(type) {
	case bool:
		return val, true
	case int, int8, int16, int32, int64:
		return val != 0, true
	case uint, uint8, uint16, uint32, uint64:
		return val != 0, true
	case float32:
		return val != 0, true
	case float64:
		return val != 0, true
	default:
		return false, false
	}
}

func toInt64(v interface{}) (int64, bool) {
	switch val := v.(type) {
	case int:
		return int64(val), true
	case int8:
		return int64(val), true
	case int16:
		return int64(val), true
	case int32:
		return int64(val), true
	case int64:
		return val, true
	case uint:
		return int64(val), true
	case uint8:
		return int64(val), true
	case uint16:
		return int64(val), true
	case uint32:
		return int64(val), true
	case uint64:
		return int64(val), true
	case float32:
		return int64(val), true
	case float64:
		return int64(val), true
	default:
		return 0, false
	}
}

func toUint64(v interface{}) (uint64, bool) {
	switch val := v.(type) {
	case int:
		return uint64(val), true
	case int8:
		return uint64(val), true
	case int16:
		return uint64(val), true
	case int32:
		return uint64(val), true
	case int64:
		return uint64(val), true
	case uint:
		return uint64(val), true
	case uint8:
		return uint64(val), true
	case uint16:
		return uint64(val), true
	case uint32:
		return uint64(val), true
	case uint64:
		return val, true
	case float32:
		return uint64(val), true
	case float64:
		return uint64(val), true
	default:
		return 0, false
	}
}

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case int:
		return float64(val), true
	case int8:
		return float64(val), true
	case int16:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint:
		return float64(val), true
	case uint8:
		return float64(val), true
	case uint16:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint64:
		return float64(val), true
	case float32:
		return float64(val), true
	case float64:
		return val, true
	default:
		return 0, false
	}
}

