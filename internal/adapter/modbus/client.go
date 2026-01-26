// Package modbus provides a production-grade Modbus TCP/RTU client implementation
// with connection pooling, circuit breaker, retry logic, and comprehensive error handling.
package modbus

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goburrow/modbus"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/rs/zerolog"
)

// Client represents a Modbus client connection to a single device.
type Client struct {
	config     ClientConfig
	handler    *modbus.TCPClientHandler
	client     modbus.Client
	logger     zerolog.Logger
	mu         sync.RWMutex
	connected  atomic.Bool
	lastError  error
	lastUsed   time.Time
	stats      *ClientStats
	deviceID   string
}

// ClientConfig holds configuration for a Modbus client.
type ClientConfig struct {
	// Address is the host:port for TCP or serial port for RTU
	Address string

	// SlaveID is the Modbus slave/unit ID (1-247)
	SlaveID byte

	// Timeout is the connection and response timeout
	Timeout time.Duration

	// IdleTimeout is how long to keep idle connections open
	IdleTimeout time.Duration

	// MaxRetries is the number of retry attempts on transient failures
	MaxRetries int

	// RetryDelay is the base delay between retries (exponential backoff applied)
	RetryDelay time.Duration

	// Protocol specifies TCP or RTU
	Protocol domain.Protocol
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

	// Parse the raw bytes into a typed value
	value, err := c.parseValue(rawBytes, tag)
	if err != nil {
		return c.createErrorDataPoint(tag, err), err
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
func (c *Client) readRegisters(tag *domain.Tag) ([]byte, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, domain.ErrConnectionClosed
	}

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
		return nil, c.translateModbusError(err)
	}

	return result, nil
}

// parseValue converts raw bytes to a typed value based on the tag's data type.
func (c *Client) parseValue(data []byte, tag *domain.Tag) (interface{}, error) {
	if len(data) == 0 {
		return nil, domain.ErrInvalidDataLength
	}

	// Handle coil/discrete input (boolean) values
	if tag.RegisterType == domain.RegisterTypeCoil || 
	   tag.RegisterType == domain.RegisterTypeDiscreteInput {
		if tag.BitPosition != nil {
			return (data[0] & (1 << *tag.BitPosition)) != 0, nil
		}
		return data[0] != 0, nil
	}

	// Handle register values
	expectedLen := int(tag.RegisterCount) * 2
	if len(data) < expectedLen {
		return nil, domain.ErrInvalidDataLength
	}

	// Reorder bytes based on byte order
	orderedData := c.reorderBytes(data[:expectedLen], tag.ByteOrder)

	switch tag.DataType {
	case domain.DataTypeBool:
		if tag.BitPosition != nil {
			val := binary.BigEndian.Uint16(orderedData)
			return (val & (1 << *tag.BitPosition)) != 0, nil
		}
		return orderedData[0] != 0 || orderedData[1] != 0, nil

	case domain.DataTypeInt16:
		return int16(binary.BigEndian.Uint16(orderedData)), nil

	case domain.DataTypeUInt16:
		return binary.BigEndian.Uint16(orderedData), nil

	case domain.DataTypeInt32:
		return int32(binary.BigEndian.Uint32(orderedData)), nil

	case domain.DataTypeUInt32:
		return binary.BigEndian.Uint32(orderedData), nil

	case domain.DataTypeInt64:
		return int64(binary.BigEndian.Uint64(orderedData)), nil

	case domain.DataTypeUInt64:
		return binary.BigEndian.Uint64(orderedData), nil

	case domain.DataTypeFloat32:
		bits := binary.BigEndian.Uint32(orderedData)
		return math.Float32frombits(bits), nil

	case domain.DataTypeFloat64:
		bits := binary.BigEndian.Uint64(orderedData)
		return math.Float64frombits(bits), nil

	default:
		return nil, domain.ErrInvalidDataType
	}
}

// reorderBytes reorders bytes according to the specified byte order.
func (c *Client) reorderBytes(data []byte, order domain.ByteOrder) []byte {
	if len(data) <= 2 {
		if order == domain.ByteOrderLittleEndian {
			return []byte{data[1], data[0]}
		}
		return data
	}

	result := make([]byte, len(data))
	switch order {
	case domain.ByteOrderBigEndian: // ABCD
		copy(result, data)

	case domain.ByteOrderLittleEndian: // DCBA
		for i := 0; i < len(data); i++ {
			result[i] = data[len(data)-1-i]
		}

	case domain.ByteOrderMidBigEndian: // BADC (word swap)
		for i := 0; i < len(data); i += 2 {
			result[i] = data[i+1]
			result[i+1] = data[i]
		}

	case domain.ByteOrderMidLitEndian: // CDAB (byte swap)
		for i := 0; i < len(data); i += 4 {
			if i+3 < len(data) {
				result[i] = data[i+2]
				result[i+1] = data[i+3]
				result[i+2] = data[i]
				result[i+3] = data[i+1]
			}
		}

	default:
		copy(result, data)
	}

	return result
}

// applyScaling applies scale factor and offset to the value.
func (c *Client) applyScaling(value interface{}, tag *domain.Tag) interface{} {
	if tag.ScaleFactor == 1.0 && tag.Offset == 0 {
		return value
	}

	var floatVal float64
	switch v := value.(type) {
	case int16:
		floatVal = float64(v)
	case uint16:
		floatVal = float64(v)
	case int32:
		floatVal = float64(v)
	case uint32:
		floatVal = float64(v)
	case int64:
		floatVal = float64(v)
	case uint64:
		floatVal = float64(v)
	case float32:
		floatVal = float64(v)
	case float64:
		floatVal = v
	case bool:
		return value // No scaling for booleans
	default:
		return value
	}

	return floatVal*tag.ScaleFactor + tag.Offset
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

// readTagGroup reads a group of tags of the same register type.
func (c *Client) readTagGroup(ctx context.Context, tags []*domain.Tag) ([]*domain.DataPoint, error) {
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
	// Retry on timeouts and connection errors
	return isTimeout(err) || c.isConnectionError(err)
}

// isConnectionError checks if the error is a connection-related error.
func (c *Client) isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	// Check for network errors
	if _, ok := err.(net.Error); ok {
		return true
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
func (c *Client) writeRegister(tag *domain.Tag, value interface{}) error {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return domain.ErrConnectionClosed
	}

	switch tag.RegisterType {
	case domain.RegisterTypeCoil:
		return c.writeSingleCoil(client, tag.Address, value)
	case domain.RegisterTypeHoldingRegister:
		return c.writeHoldingRegister(client, tag, value)
	default:
		return fmt.Errorf("%w: %s is read-only", domain.ErrTagNotWritable, tag.RegisterType)
	}
}

// writeSingleCoil writes a boolean value to a coil (function code 0x05).
func (c *Client) writeSingleCoil(client modbus.Client, address uint16, value interface{}) error {
	boolValue, ok := c.toBool(value)
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

// writeHoldingRegister writes a value to holding register(s).
func (c *Client) writeHoldingRegister(client modbus.Client, tag *domain.Tag, value interface{}) error {
	// Convert value to bytes based on data type
	bytes, err := c.valueToBytes(value, tag)
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

	_, err := client.WriteSingleCoil(address, coilValue)
	if err != nil {
		c.stats.ErrorCount.Add(1)
		return fmt.Errorf("%w: %v", domain.ErrWriteFailed, err)
	}

	c.stats.WriteCount.Add(1)
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

	_, err := client.WriteSingleRegister(address, value)
	if err != nil {
		c.stats.ErrorCount.Add(1)
		return fmt.Errorf("%w: %v", domain.ErrWriteFailed, err)
	}

	c.stats.WriteCount.Add(1)
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

	_, err := client.WriteMultipleRegisters(address, uint16(len(values)), bytes)
	if err != nil {
		c.stats.ErrorCount.Add(1)
		return fmt.Errorf("%w: %v", domain.ErrWriteFailed, err)
	}

	c.stats.WriteCount.Add(1)
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

	_, err := client.WriteMultipleCoils(address, uint16(len(values)), bytes)
	if err != nil {
		c.stats.ErrorCount.Add(1)
		return fmt.Errorf("%w: %v", domain.ErrWriteFailed, err)
	}

	c.stats.WriteCount.Add(1)
	return nil
}

// valueToBytes converts a value to bytes based on the tag's data type.
func (c *Client) valueToBytes(value interface{}, tag *domain.Tag) ([]byte, error) {
	// Reverse scaling if applied
	actualValue := c.reverseScaling(value, tag)

	var bytes []byte

	switch tag.DataType {
	case domain.DataTypeBool:
		bytes = make([]byte, 2)
		if b, ok := c.toBool(actualValue); ok && b {
			binary.BigEndian.PutUint16(bytes, 1)
		}

	case domain.DataTypeInt16:
		bytes = make([]byte, 2)
		if v, ok := c.toInt64(actualValue); ok {
			binary.BigEndian.PutUint16(bytes, uint16(int16(v)))
		} else {
			return nil, fmt.Errorf("%w: cannot convert %T to int16", domain.ErrInvalidWriteValue, value)
		}

	case domain.DataTypeUInt16:
		bytes = make([]byte, 2)
		if v, ok := c.toInt64(actualValue); ok {
			binary.BigEndian.PutUint16(bytes, uint16(v))
		} else {
			return nil, fmt.Errorf("%w: cannot convert %T to uint16", domain.ErrInvalidWriteValue, value)
		}

	case domain.DataTypeInt32:
		bytes = make([]byte, 4)
		if v, ok := c.toInt64(actualValue); ok {
			binary.BigEndian.PutUint32(bytes, uint32(int32(v)))
		} else {
			return nil, fmt.Errorf("%w: cannot convert %T to int32", domain.ErrInvalidWriteValue, value)
		}

	case domain.DataTypeUInt32:
		bytes = make([]byte, 4)
		if v, ok := c.toInt64(actualValue); ok {
			binary.BigEndian.PutUint32(bytes, uint32(v))
		} else {
			return nil, fmt.Errorf("%w: cannot convert %T to uint32", domain.ErrInvalidWriteValue, value)
		}

	case domain.DataTypeInt64:
		bytes = make([]byte, 8)
		if v, ok := c.toInt64(actualValue); ok {
			binary.BigEndian.PutUint64(bytes, uint64(v))
		} else {
			return nil, fmt.Errorf("%w: cannot convert %T to int64", domain.ErrInvalidWriteValue, value)
		}

	case domain.DataTypeUInt64:
		bytes = make([]byte, 8)
		if v, ok := c.toUint64(actualValue); ok {
			binary.BigEndian.PutUint64(bytes, v)
		} else {
			return nil, fmt.Errorf("%w: cannot convert %T to uint64", domain.ErrInvalidWriteValue, value)
		}

	case domain.DataTypeFloat32:
		bytes = make([]byte, 4)
		if v, ok := c.toFloat64(actualValue); ok {
			binary.BigEndian.PutUint32(bytes, math.Float32bits(float32(v)))
		} else {
			return nil, fmt.Errorf("%w: cannot convert %T to float32", domain.ErrInvalidWriteValue, value)
		}

	case domain.DataTypeFloat64:
		bytes = make([]byte, 8)
		if v, ok := c.toFloat64(actualValue); ok {
			binary.BigEndian.PutUint64(bytes, math.Float64bits(v))
		} else {
			return nil, fmt.Errorf("%w: cannot convert %T to float64", domain.ErrInvalidWriteValue, value)
		}

	default:
		return nil, fmt.Errorf("%w: unsupported data type %s", domain.ErrInvalidDataType, tag.DataType)
	}

	// Apply byte order transformation
	bytes = c.reorderBytesForWrite(bytes, tag.ByteOrder)

	return bytes, nil
}

// reverseScaling reverses the scaling for write operations.
func (c *Client) reverseScaling(value interface{}, tag *domain.Tag) interface{} {
	if tag.ScaleFactor == 1.0 && tag.Offset == 0 {
		return value
	}

	floatVal, ok := c.toFloat64(value)
	if !ok {
		return value
	}

	return (floatVal - tag.Offset) / tag.ScaleFactor
}

// reorderBytesForWrite reorders bytes for write operations (inverse of read).
func (c *Client) reorderBytesForWrite(data []byte, order domain.ByteOrder) []byte {
	// The byte reordering for writes is the same as reads - just apply the same transformation
	return c.reorderBytes(data, order)
}

// toBool converts a value to bool.
func (c *Client) toBool(v interface{}) (bool, bool) {
	switch val := v.(type) {
	case bool:
		return val, true
	case int:
		return val != 0, true
	case int8:
		return val != 0, true
	case int16:
		return val != 0, true
	case int32:
		return val != 0, true
	case int64:
		return val != 0, true
	case uint:
		return val != 0, true
	case uint8:
		return val != 0, true
	case uint16:
		return val != 0, true
	case uint32:
		return val != 0, true
	case uint64:
		return val != 0, true
	case float32:
		return val != 0, true
	case float64:
		return val != 0, true
	default:
		return false, false
	}
}

// toInt64 converts a value to int64.
func (c *Client) toInt64(v interface{}) (int64, bool) {
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

// toUint64 converts a value to uint64.
func (c *Client) toUint64(v interface{}) (uint64, bool) {
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

// toFloat64 converts a value to float64.
func (c *Client) toFloat64(v interface{}) (float64, bool) {
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

