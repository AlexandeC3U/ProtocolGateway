// Package opcua provides a production-grade OPC UA client implementation
// with connection management, subscriptions, bidirectional communication, and comprehensive error handling.
package opcua

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/rs/zerolog"
)

// NewClient creates a new OPC UA client with the given configuration.
func NewClient(deviceID string, config ClientConfig, logger zerolog.Logger) (*Client, error) {
	if config.EndpointURL == "" {
		return nil, fmt.Errorf("OPC UA endpoint URL is required")
	}

	// Apply defaults
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.KeepAlive == 0 {
		config.KeepAlive = 30 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 500 * time.Millisecond
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 5 * time.Second
	}
	if config.SessionTimeout == 0 {
		config.SessionTimeout = 30 * time.Minute
	}
	if config.SecurityPolicy == "" {
		config.SecurityPolicy = "None"
	}
	if config.SecurityMode == "" {
		config.SecurityMode = "None"
	}
	if config.AuthMode == "" {
		config.AuthMode = "Anonymous"
	}
	// Subscription defaults
	if config.DefaultPublishingInterval == 0 {
		config.DefaultPublishingInterval = 1 * time.Second
	}
	if config.DefaultSamplingInterval == 0 {
		config.DefaultSamplingInterval = 500 * time.Millisecond
	}
	if config.DefaultQueueSize == 0 {
		config.DefaultQueueSize = 10
	}

	c := &Client{
		config:       config,
		logger:       logger.With().Str("device_id", deviceID).Str("endpoint", config.EndpointURL).Logger(),
		stats:        &ClientStats{},
		deviceID:     deviceID,
		lastUsed:     time.Now(),
		nodeCache:    make(map[string]*ua.NodeID),
		sessionState: SessionStateDisconnected,
	}

	return c, nil
}

// Connect establishes the connection to the OPC UA server.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected.Load() {
		return nil
	}

	c.sessionState = SessionStateConnecting
	c.logger.Debug().Msg("Connecting to OPC UA server")

	// Build client options
	opts := []opcua.Option{
		opcua.RequestTimeout(c.config.RequestTimeout),
		opcua.SessionTimeout(c.config.SessionTimeout),
	}

	// Configure security
	secPolicy := c.getSecurityPolicy()

	if secPolicy != ua.SecurityPolicyURINone {
		opts = append(opts, opcua.SecurityPolicy(secPolicy))
		opts = append(opts, opcua.SecurityModeString(c.config.SecurityMode))
	}

	// Configure authentication
	switch c.config.AuthMode {
	case "Anonymous":
		opts = append(opts, opcua.AuthAnonymous())
	case "UserName":
		opts = append(opts, opcua.AuthUsername(c.config.Username, c.config.Password))
	case "Certificate":
		opts = append(opts, opcua.CertificateFile(c.config.CertificateFile))
		opts = append(opts, opcua.PrivateKeyFile(c.config.PrivateKeyFile))
	}

	// Create client
	client, err := opcua.NewClient(c.config.EndpointURL, opts...)
	if err != nil {
		c.lastError = err
		return fmt.Errorf("%w: failed to create client: %v", domain.ErrConnectionFailed, err)
	}

	// Connect with timeout
	connectCtx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()

	if err := client.Connect(connectCtx); err != nil {
		// Ensure resources are released on failed handshakes/session creation.
		// Without this, repeated retries can exhaust server session limits.
		_ = client.Close(context.Background())
		c.lastError = err
		c.sessionState = SessionStateError
		return fmt.Errorf("%w: %v", domain.ErrConnectionFailed, err)
	}

	c.client = client
	c.connected.Store(true)
	c.sessionState = SessionStateActive
	c.lastError = nil
	c.lastUsed = time.Now()
	c.consecutiveFailures.Store(0)

	c.logger.Info().Msg("Connected to OPC UA server")
	return nil
}

// Disconnect closes the connection to the OPC UA server.
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected.Load() {
		return nil
	}

	if c.client != nil {
		if err := c.client.Close(context.Background()); err != nil {
			c.logger.Warn().Err(err).Msg("Error closing OPC UA connection")
		}
	}

	c.connected.Store(false)
	c.sessionState = SessionStateDisconnected
	c.client = nil

	// Clear node cache
	c.nodeCacheMu.Lock()
	c.nodeCache = make(map[string]*ua.NodeID)
	c.nodeCacheMu.Unlock()

	c.logger.Debug().Msg("Disconnected from OPC UA server")
	return nil
}

// GetSessionState returns the current session state.
func (c *Client) GetSessionState() SessionState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionState
}

// IsConnected returns true if the client is currently connected.
func (c *Client) IsConnected() bool {
	return c.connected.Load()
}

// ReadTag reads a single tag from the OPC UA server.
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

	// Parse node ID
	nodeID, err := c.getNodeID(tag.OPCNodeID)
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
				Msg("Retrying OPC UA read")

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		dp, err = c.readNode(ctx, nodeID, tag)
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

// ReadTags reads multiple tags efficiently using batch reads.
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

	// Build read requests
	nodesToRead := make([]*ua.ReadValueID, 0, len(tags))
	validTags := make([]*domain.Tag, 0, len(tags))

	for _, tag := range tags {
		nodeID, err := c.getNodeID(tag.OPCNodeID)
		if err != nil {
			c.logger.Warn().Err(err).Str("tag", tag.ID).Str("node_id", tag.OPCNodeID).Msg("Invalid node ID")
			continue
		}
		nodesToRead = append(nodesToRead, &ua.ReadValueID{
			NodeID:       nodeID,
			AttributeID:  ua.AttributeIDValue,
			DataEncoding: &ua.QualifiedName{},
		})
		validTags = append(validTags, tag)
	}

	if len(nodesToRead) == 0 {
		return nil, nil
	}

	// Execute batch read
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, domain.ErrConnectionClosed
	}

	req := &ua.ReadRequest{
		MaxAge:             0,
		TimestampsToReturn: ua.TimestampsToReturnBoth,
		NodesToRead:        nodesToRead,
	}

	resp, err := client.Read(ctx, req)
	if err != nil {
		c.stats.ErrorCount.Add(1)
		return nil, fmt.Errorf("%w: %v", domain.ErrReadFailed, err)
	}

	// Process results
	results := make([]*domain.DataPoint, 0, len(tags))
	for i, result := range resp.Results {
		if i >= len(validTags) {
			break
		}
		tag := validTags[i]
		dp := c.processReadResult(result, tag)
		results = append(results, dp)
	}

	c.stats.ReadCount.Add(uint64(len(results)))
	return results, nil
}

// WriteTag writes a value to a tag on the OPC UA server.
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
		return fmt.Errorf("%w: tag %s is not writable", domain.ErrWriteFailed, tag.ID)
	}

	// Parse node ID
	nodeID, err := c.getNodeID(tag.OPCNodeID)
	if err != nil {
		c.stats.ErrorCount.Add(1)
		return err
	}

	// Convert value to OPC UA variant
	variant, err := c.valueToVariant(value, tag)
	if err != nil {
		c.stats.ErrorCount.Add(1)
		return fmt.Errorf("%w: %v", domain.ErrWriteFailed, err)
	}

	// Execute write with retry logic
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			c.stats.RetryCount.Add(1)
			delay := c.calculateBackoff(attempt)
			c.logger.Debug().
				Int("attempt", attempt).
				Dur("delay", delay).
				Msg("Retrying OPC UA write")

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err = c.writeNode(ctx, nodeID, variant)
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
		Msg("Successfully wrote to OPC UA node")

	return nil
}

// WriteTags writes multiple values to tags on the OPC UA server.
func (c *Client) WriteTags(ctx context.Context, writes []TagWrite) []error {
	if len(writes) == 0 {
		return nil
	}

	c.mu.Lock()
	c.lastUsed = time.Now()
	c.mu.Unlock()

	if !c.connected.Load() {
		errors := make([]error, len(writes))
		for i := range errors {
			errors[i] = domain.ErrConnectionClosed
		}
		return errors
	}

	// Build write requests
	nodesToWrite := make([]*ua.WriteValue, 0, len(writes))
	validIndices := make([]int, 0, len(writes))
	errors := make([]error, len(writes))

	for i, write := range writes {
		if !write.Tag.IsWritable() {
			errors[i] = fmt.Errorf("%w: tag %s is not writable", domain.ErrWriteFailed, write.Tag.ID)
			continue
		}

		nodeID, err := c.getNodeID(write.Tag.OPCNodeID)
		if err != nil {
			errors[i] = err
			continue
		}

		variant, err := c.valueToVariant(write.Value, write.Tag)
		if err != nil {
			errors[i] = err
			continue
		}

		nodesToWrite = append(nodesToWrite, &ua.WriteValue{
			NodeID:      nodeID,
			AttributeID: ua.AttributeIDValue,
			Value: &ua.DataValue{
				Value:           variant,
				SourceTimestamp: time.Now(),
			},
		})
		validIndices = append(validIndices, i)
	}

	if len(nodesToWrite) == 0 {
		return errors
	}

	// Execute batch write
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		for _, idx := range validIndices {
			errors[idx] = domain.ErrConnectionClosed
		}
		return errors
	}

	req := &ua.WriteRequest{
		NodesToWrite: nodesToWrite,
	}

	resp, err := client.Write(ctx, req)
	if err != nil {
		c.stats.ErrorCount.Add(1)
		for _, idx := range validIndices {
			errors[idx] = fmt.Errorf("%w: %v", domain.ErrWriteFailed, err)
		}
		return errors
	}

	// Process results
	for i, result := range resp.Results {
		if i >= len(validIndices) {
			break
		}
		idx := validIndices[i]
		if result != ua.StatusOK {
			errors[idx] = fmt.Errorf("%w: status code %d", domain.ErrWriteFailed, result)
			c.stats.ErrorCount.Add(1)
		} else {
			c.stats.WriteCount.Add(1)
		}
	}

	return errors
}

// TagWrite represents a single write operation.
type TagWrite struct {
	Tag   *domain.Tag
	Value interface{}
}

// readNode performs a single node read operation.
// Uses opMu to serialize operations for thread safety.
func (c *Client) readNode(ctx context.Context, nodeID *ua.NodeID, tag *domain.Tag) (*domain.DataPoint, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, domain.ErrConnectionClosed
	}

	// Serialize OPC UA operations
	c.opMu.Lock()
	defer c.opMu.Unlock()

	req := &ua.ReadRequest{
		MaxAge:             0,
		TimestampsToReturn: ua.TimestampsToReturnBoth,
		NodesToRead: []*ua.ReadValueID{
			{
				NodeID:       nodeID,
				AttributeID:  ua.AttributeIDValue,
				DataEncoding: &ua.QualifiedName{},
			},
		},
	}

	resp, err := client.Read(ctx, req)
	if err != nil {
		c.consecutiveFailures.Add(1)
		return nil, fmt.Errorf("%w: %v", domain.ErrReadFailed, err)
	}

	if len(resp.Results) == 0 {
		return nil, fmt.Errorf("%w: no results returned", domain.ErrReadFailed)
	}

	c.consecutiveFailures.Store(0)
	return c.processReadResult(resp.Results[0], tag), nil
}

// writeNode performs a single node write operation.
// Uses opMu to serialize operations for thread safety.
func (c *Client) writeNode(ctx context.Context, nodeID *ua.NodeID, variant *ua.Variant) error {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return domain.ErrConnectionClosed
	}

	// Serialize OPC UA operations
	c.opMu.Lock()
	defer c.opMu.Unlock()

	req := &ua.WriteRequest{
		NodesToWrite: []*ua.WriteValue{
			{
				NodeID:      nodeID,
				AttributeID: ua.AttributeIDValue,
				Value: &ua.DataValue{
					EncodingMask: ua.DataValueValue,
					Value:        variant,
				},
			},
		},
	}

	resp, err := client.Write(ctx, req)
	if err != nil {
		c.consecutiveFailures.Add(1)
		return fmt.Errorf("%w: %v", domain.ErrWriteFailed, err)
	}

	if len(resp.Results) == 0 {
		return fmt.Errorf("%w: no results returned", domain.ErrWriteFailed)
	}

	if resp.Results[0] != ua.StatusOK {
		c.consecutiveFailures.Add(1)
		return fmt.Errorf("%w: status code %d", domain.ErrWriteFailed, resp.Results[0])
	}

	c.consecutiveFailures.Store(0)
	return nil
}

// processReadResult converts an OPC UA read result to a DataPoint.
func (c *Client) processReadResult(result *ua.DataValue, tag *domain.Tag) *domain.DataPoint {
	quality := c.statusCodeToQuality(result.Status)

	if quality != domain.QualityGood {
		return domain.NewDataPoint(
			c.deviceID,
			tag.ID,
			"",
			nil,
			tag.Unit,
			quality,
		)
	}

	// Extract value from variant
	value := c.variantToValue(result.Value, tag)

	// Apply scaling and offset
	scaledValue := applyScaling(value, tag)

	dp := domain.NewDataPoint(
		c.deviceID,
		tag.ID,
		"",
		scaledValue,
		tag.Unit,
		quality,
	).WithRawValue(value).WithPriority(tag.Priority)

	// Set source timestamp from OPC UA if available
	if !result.SourceTimestamp.IsZero() {
		dp.WithSourceTimestamp(result.SourceTimestamp)
	}

	return dp
}

// variantToValue converts an OPC UA variant to a Go value.
func (c *Client) variantToValue(v *ua.Variant, tag *domain.Tag) interface{} {
	if v == nil {
		return nil
	}

	// Return the value directly - the OPC UA library handles type conversion
	return v.Value()
}

// valueToVariant converts a Go value to an OPC UA variant.
func (c *Client) valueToVariant(value interface{}, tag *domain.Tag) (*ua.Variant, error) {
	// Reverse scaling if applied
	actualValue := reverseScaling(value, tag)

	// Convert based on target data type
	switch tag.DataType {
	case domain.DataTypeBool:
		b, ok := toBool(actualValue)
		if !ok {
			return nil, fmt.Errorf("cannot convert %T to bool", actualValue)
		}
		return ua.NewVariant(b)

	case domain.DataTypeInt16:
		i, ok := toInt64(actualValue)
		if !ok {
			return nil, fmt.Errorf("cannot convert %T to int16", actualValue)
		}
		return ua.NewVariant(int16(i))

	case domain.DataTypeUInt16:
		i, ok := toInt64(actualValue)
		if !ok {
			return nil, fmt.Errorf("cannot convert %T to uint16", actualValue)
		}
		return ua.NewVariant(uint16(i))

	case domain.DataTypeInt32:
		i, ok := toInt64(actualValue)
		if !ok {
			return nil, fmt.Errorf("cannot convert %T to int32", actualValue)
		}
		return ua.NewVariant(int32(i))

	case domain.DataTypeUInt32:
		i, ok := toInt64(actualValue)
		if !ok {
			return nil, fmt.Errorf("cannot convert %T to uint32", actualValue)
		}
		return ua.NewVariant(uint32(i))

	case domain.DataTypeInt64:
		i, ok := toInt64(actualValue)
		if !ok {
			return nil, fmt.Errorf("cannot convert %T to int64", actualValue)
		}
		return ua.NewVariant(i)

	case domain.DataTypeUInt64:
		i, ok := toUint64(actualValue)
		if !ok {
			return nil, fmt.Errorf("cannot convert %T to uint64", actualValue)
		}
		return ua.NewVariant(i)

	case domain.DataTypeFloat32:
		f, ok := toFloat64(actualValue)
		if !ok {
			return nil, fmt.Errorf("cannot convert %T to float32", actualValue)
		}
		return ua.NewVariant(float32(f))

	case domain.DataTypeFloat64:
		f, ok := toFloat64(actualValue)
		if !ok {
			return nil, fmt.Errorf("cannot convert %T to float64", actualValue)
		}
		return ua.NewVariant(f)

	case domain.DataTypeString:
		s, ok := actualValue.(string)
		if !ok {
			s = fmt.Sprintf("%v", actualValue)
		}
		return ua.NewVariant(s)

	default:
		return ua.NewVariant(actualValue)
	}
}

// getNodeID parses and caches a node ID.
func (c *Client) getNodeID(nodeIDStr string) (*ua.NodeID, error) {
	// Check cache
	c.nodeCacheMu.RLock()
	if nodeID, exists := c.nodeCache[nodeIDStr]; exists {
		c.nodeCacheMu.RUnlock()
		return nodeID, nil
	}
	c.nodeCacheMu.RUnlock()

	// Parse node ID
	nodeID, err := ua.ParseNodeID(nodeIDStr)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid node ID %s: %v", domain.ErrOPCUAInvalidNodeID, nodeIDStr, err)
	}

	// Cache it
	c.nodeCacheMu.Lock()
	c.nodeCache[nodeIDStr] = nodeID
	c.nodeCacheMu.Unlock()

	return nodeID, nil
}

// statusCodeToQuality converts OPC UA status code to domain quality.
func (c *Client) statusCodeToQuality(status ua.StatusCode) domain.Quality {
	if status == ua.StatusOK || status == ua.StatusGood {
		return domain.QualityGood
	}

	// Check for specific status codes
	switch {
	case status == ua.StatusBad:
		return domain.QualityBad
	case status == ua.StatusUncertain:
		return domain.QualityUncertain
	case status&0x80000000 != 0: // Bad status codes have bit 31 set
		return domain.QualityBad
	case status&0x40000000 != 0: // Uncertain status codes have bit 30 set
		return domain.QualityUncertain
	default:
		return domain.QualityGood
	}
}

// getSecurityPolicy returns the OPC UA security policy URI.
func (c *Client) getSecurityPolicy() string {
	switch c.config.SecurityPolicy {
	case "None":
		return ua.SecurityPolicyURINone
	case "Basic128Rsa15":
		return ua.SecurityPolicyURIBasic128Rsa15
	case "Basic256":
		return ua.SecurityPolicyURIBasic256
	case "Basic256Sha256":
		return ua.SecurityPolicyURIBasic256Sha256
	default:
		return ua.SecurityPolicyURINone
	}
}

// getSecurityMode returns the OPC UA security mode.
func (c *Client) getSecurityMode() ua.MessageSecurityMode {
	switch c.config.SecurityMode {
	case "None":
		return ua.MessageSecurityModeNone
	case "Sign":
		return ua.MessageSecurityModeSign
	case "SignAndEncrypt":
		return ua.MessageSecurityModeSignAndEncrypt
	default:
		return ua.MessageSecurityModeNone
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
	// If the server is refusing new sessions, retries usually just worsen the situation.
	if isTooManySessionsError(err) {
		return false
	}
	// Retry on timeouts and connection errors
	return c.isConnectionError(err)
}

// isConnectionError checks if the error is a connection-related error.
func (c *Client) isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	// Check for EOF errors (common on connection drops)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	// Check for common connection error patterns
	errStr := err.Error()
	return contains(errStr, "connection") ||
		contains(errStr, "timeout") ||
		contains(errStr, "closed") ||
		contains(errStr, "refused") ||
		contains(errStr, "reset") ||
		contains(errStr, "broken pipe") ||
		contains(errStr, "no route to host")
}

func isTooManySessionsError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "TooManySessions") ||
		contains(errStr, "maximum number of sessions") ||
		contains(errStr, "StatusBadTooManySessions")
}

// reconnect attempts to re-establish the connection.
func (c *Client) reconnect(ctx context.Context) {
	c.Disconnect()
	if err := c.Connect(ctx); err != nil {
		c.logger.Error().Err(err).Msg("Failed to reconnect")
	}
}

// GetStats returns the client statistics as a map.
func (c *Client) GetStats() map[string]uint64 {
	return map[string]uint64{
		"read_count":         c.stats.ReadCount.Load(),
		"write_count":        c.stats.WriteCount.Load(),
		"error_count":        c.stats.ErrorCount.Load(),
		"retry_count":        c.stats.RetryCount.Load(),
		"subscribe_count":    c.stats.SubscribeCount.Load(),
		"notification_count": c.stats.NotificationCount.Load(),
		"total_read_ns":      uint64(c.stats.TotalReadTime.Load()),
		"total_write_ns":     uint64(c.stats.TotalWriteTime.Load()),
	}
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

// Helper function for string contains
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsRune(s, substr))
}

func containsRune(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Unused imports guard - will be removed by compiler if not needed
var _ = binary.BigEndian
var _ = math.Float32frombits
