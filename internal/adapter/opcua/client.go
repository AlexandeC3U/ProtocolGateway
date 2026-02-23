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
	"strings"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/nexus-edge/protocol-gateway/internal/metrics"
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
		namespaceMap: make(map[string]uint16),
		sessionState: SessionStateDisconnected,
	}

	return c, nil
}

// SetMetrics sets the metrics registry for clock drift tracking.
func (c *Client) SetMetrics(m *metrics.Registry) {
	c.metricsReg = m
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

	// Validate and load security configuration
	secConfig, err := ValidateSecurityConfig(c.config, c.logger)
	if err != nil {
		c.lastError = err
		c.sessionState = SessionStateError
		return fmt.Errorf("%w: security configuration error: %v", domain.ErrConnectionFailed, err)
	}

	var opts []opcua.Option

	// Auto-discover endpoint if enabled
	if c.config.AutoSelectEndpoint {
		discoveryCtx, cancel := context.WithTimeout(ctx, c.config.Timeout)
		endpoint, err := DiscoverAndSelectEndpoint(discoveryCtx, c.config.EndpointURL, c.config, c.logger)
		cancel()

		if err != nil {
			c.logger.Warn().Err(err).Msg("Endpoint discovery failed, using manual configuration")
			opts = secConfig.BuildClientOptions()
		} else {
			// Validate server certificate against trust store if configured
			if c.trustStore != nil && len(endpoint.ServerCertificate) > 0 {
				if err := c.trustStore.ValidateServerCertificate(endpoint.ServerCertificate, c.autoTrust); err != nil {
					c.lastError = err
					c.sessionState = SessionStateError
					return fmt.Errorf("%w: %v", domain.ErrConnectionFailed, err)
				}
			}
			opts = BuildOptionsFromEndpoint(endpoint, secConfig)
		}
	} else {
		opts = secConfig.BuildClientOptions()
	}

	// Add timeout options
	opts = append(opts,
		opcua.RequestTimeout(c.config.RequestTimeout),
		opcua.SessionTimeout(c.config.SessionTimeout),
	)

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

	// Fetch namespace array from server for URI-based NodeID resolution
	if err := c.updateNamespaceTable(ctx); err != nil {
		c.logger.Warn().Err(err).Msg("Failed to fetch namespace table, URI-based NodeIDs may not resolve")
	}

	c.logger.Info().
		Str("policy", c.config.SecurityPolicy).
		Str("mode", c.config.SecurityMode).
		Str("auth", c.config.AuthMode).
		Int("namespaces", len(c.namespaceArray)).
		Msg("Connected to OPC UA server")
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

	// Clear namespace cache (will be repopulated on reconnect)
	c.namespaceMu.Lock()
	c.namespaceMap = make(map[string]uint16)
	c.namespaceArray = nil
	c.namespaceMu.Unlock()

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

	// Parse node ID (supports both ns= and nsu= formats, plus OPCNamespaceURI field)
	nodeID, err := c.getNodeIDForTag(tag)
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
// Uses opMu to serialize batch operations for thread safety.
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

	// Serialize batch operations to prevent protocol corruption
	c.opMu.Lock()
	defer c.opMu.Unlock()

	// Build read requests
	nodesToRead := make([]*ua.ReadValueID, 0, len(tags))
	validTags := make([]*domain.Tag, 0, len(tags))

	for _, tag := range tags {
		// Parse node ID (supports both ns= and nsu= formats, plus OPCNamespaceURI field)
		nodeID, err := c.getNodeIDForTag(tag)
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

	// Parse node ID (supports both ns= and nsu= formats, plus OPCNamespaceURI field)
	nodeID, err := c.getNodeIDForTag(tag)
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
// Uses opMu to serialize batch operations for thread safety.
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

	// Serialize batch operations to prevent protocol corruption
	c.opMu.Lock()
	defer c.opMu.Unlock()

	// Build write requests
	nodesToWrite := make([]*ua.WriteValue, 0, len(writes))
	validIndices := make([]int, 0, len(writes))
	errors := make([]error, len(writes))

	for i, write := range writes {
		if !write.Tag.IsWritable() {
			errors[i] = fmt.Errorf("%w: tag %s is not writable", domain.ErrWriteFailed, write.Tag.ID)
			continue
		}

		// Parse node ID (supports both ns= and nsu= formats, plus OPCNamespaceURI field)
		nodeID, err := c.getNodeIDForTag(write.Tag)
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
		return domain.AcquireDataPoint(
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

	dp := domain.AcquireDataPoint(
		c.deviceID,
		tag.ID,
		"",
		scaledValue,
		tag.Unit,
		quality,
	).WithRawValue(value).WithPriority(tag.Priority)

	// Set source timestamp from OPC UA if available and record clock drift
	if !result.SourceTimestamp.IsZero() {
		dp.WithSourceTimestamp(result.SourceTimestamp)
		if c.metricsReg != nil {
			drift := time.Since(result.SourceTimestamp)
			c.metricsReg.RecordOPCUAClockDrift(c.deviceID, drift.Seconds())
		}
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

// maxNodeCacheSize limits the node cache to prevent unbounded memory growth.
// 50k entries is ~4MB assuming 80 bytes per entry (string key + NodeID pointer).
const maxNodeCacheSize = 50000

// getNodeID parses and caches a node ID, with support for namespace URI resolution.
// Supports both traditional "ns=2;s=Temperature" and URI-based "nsu=http://example.org/;s=Temperature" formats.
func (c *Client) getNodeID(nodeIDStr string) (*ua.NodeID, error) {
	// Check cache first
	c.nodeCacheMu.RLock()
	if nodeID, exists := c.nodeCache[nodeIDStr]; exists {
		c.nodeCacheMu.RUnlock()
		return nodeID, nil
	}
	c.nodeCacheMu.RUnlock()

	var nodeID *ua.NodeID
	var err error

	// Check if this is a namespace URI-based NodeID (nsu=...)
	if strings.HasPrefix(nodeIDStr, "nsu=") {
		nodeID, err = c.parseExpandedNodeID(nodeIDStr)
	} else {
		// Traditional ns= format
		nodeID, err = ua.ParseNodeID(nodeIDStr)
	}

	if err != nil {
		return nil, fmt.Errorf("%w: invalid node ID %s: %v", domain.ErrOPCUAInvalidNodeID, nodeIDStr, err)
	}

	// Cache it (with size limit)
	c.nodeCacheMu.Lock()
	if len(c.nodeCache) >= maxNodeCacheSize {
		// Evict ~10% of entries when full (simple random eviction)
		count := 0
		for key := range c.nodeCache {
			delete(c.nodeCache, key)
			count++
			if count >= maxNodeCacheSize/10 {
				break
			}
		}
		c.logger.Debug().Int("evicted", count).Msg("Evicted old node cache entries")
	}
	c.nodeCache[nodeIDStr] = nodeID
	c.nodeCacheMu.Unlock()

	return nodeID, nil
}

// getNodeIDForTag resolves a NodeID for a tag, considering both OPCNodeID and OPCNamespaceURI fields.
// If OPCNamespaceURI is specified, it takes precedence over any ns= in OPCNodeID.
func (c *Client) getNodeIDForTag(tag *domain.Tag) (*ua.NodeID, error) {
	nodeIDStr := tag.OPCNodeID

	// If OPCNamespaceURI is specified, we need to resolve it and potentially modify the NodeID
	if tag.OPCNamespaceURI != "" {
		nsIndex, err := c.resolveNamespaceURI(tag.OPCNamespaceURI)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve namespace URI %q: %w", tag.OPCNamespaceURI, err)
		}

		// Build a new NodeID string with the resolved namespace index
		nodeIDStr = c.buildNodeIDWithNamespace(tag.OPCNodeID, nsIndex)
	}

	return c.getNodeID(nodeIDStr)
}

// parseExpandedNodeID parses a NodeID with namespace URI (nsu=...) format.
// Example: "nsu=http://example.org/;s=Temperature"
func (c *Client) parseExpandedNodeID(nodeIDStr string) (*ua.NodeID, error) {
	c.namespaceMu.RLock()
	nsArray := c.namespaceArray
	c.namespaceMu.RUnlock()

	if len(nsArray) == 0 {
		return nil, fmt.Errorf("namespace table not available, cannot resolve URI-based NodeID")
	}

	// Use gopcua's ParseExpandedNodeID which handles nsu= format
	expandedNodeID, err := ua.ParseExpandedNodeID(nodeIDStr, nsArray)
	if err != nil {
		return nil, err
	}

	return expandedNodeID.NodeID, nil
}

// resolveNamespaceURI looks up a namespace URI in the server's namespace table
// and returns the corresponding namespace index.
func (c *Client) resolveNamespaceURI(nsURI string) (uint16, error) {
	c.namespaceMu.RLock()
	defer c.namespaceMu.RUnlock()

	if idx, ok := c.namespaceMap[nsURI]; ok {
		return idx, nil
	}

	return 0, fmt.Errorf("namespace URI %q not found in server's namespace table", nsURI)
}

// buildNodeIDWithNamespace constructs a NodeID string with the specified namespace index.
// It handles both cases: when OPCNodeID already has ns= prefix and when it doesn't.
func (c *Client) buildNodeIDWithNamespace(nodeIDStr string, nsIndex uint16) string {
	// Remove existing ns= prefix if present
	if strings.HasPrefix(nodeIDStr, "ns=") {
		// Find the semicolon after ns=N
		idx := strings.Index(nodeIDStr, ";")
		if idx != -1 {
			nodeIDStr = nodeIDStr[idx+1:]
		}
	} else if strings.HasPrefix(nodeIDStr, "nsu=") {
		// Remove nsu= prefix
		idx := strings.Index(nodeIDStr, ";")
		if idx != -1 {
			nodeIDStr = nodeIDStr[idx+1:]
		}
	}

	// Build new NodeID with resolved namespace index
	return fmt.Sprintf("ns=%d;%s", nsIndex, nodeIDStr)
}

// updateNamespaceTable fetches the namespace array from the server and builds the URI->index map.
func (c *Client) updateNamespaceTable(ctx context.Context) error {
	if c.client == nil {
		return fmt.Errorf("client not connected")
	}

	// Fetch namespace array from server
	nsArray, err := c.client.NamespaceArray(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch namespace array: %w", err)
	}

	c.namespaceMu.Lock()
	defer c.namespaceMu.Unlock()

	c.namespaceArray = nsArray
	c.namespaceMap = make(map[string]uint16, len(nsArray))

	for idx, uri := range nsArray {
		c.namespaceMap[uri] = uint16(idx)
	}

	c.logger.Debug().
		Int("count", len(nsArray)).
		Strs("namespaces", nsArray).
		Msg("Fetched namespace table from server")

	return nil
}

// GetNamespaceArray returns the current namespace array from the server.
// This can be useful for diagnostics or building UI selectors.
func (c *Client) GetNamespaceArray() []string {
	c.namespaceMu.RLock()
	defer c.namespaceMu.RUnlock()

	if c.namespaceArray == nil {
		return nil
	}

	// Return a copy to prevent external modification
	result := make([]string, len(c.namespaceArray))
	copy(result, c.namespaceArray)
	return result
}

// RefreshNamespaceTable forces a refresh of the namespace table from the server.
// Call this if you suspect the server's namespace table has changed.
func (c *Client) RefreshNamespaceTable(ctx context.Context) error {
	// Clear node cache since namespace indices may have changed
	c.nodeCacheMu.Lock()
	c.nodeCache = make(map[string]*ua.NodeID)
	c.nodeCacheMu.Unlock()

	return c.updateNamespaceTable(ctx)
}

// Browse explores the OPC UA address space starting from a given node.
// If nodeID is empty, browsing starts from the Objects folder (i=85).
// maxDepth controls recursion depth (1 = immediate children only).
func (c *Client) Browse(ctx context.Context, nodeID string, maxDepth int) (*BrowseResult, error) {
	if !c.connected.Load() {
		return nil, domain.ErrConnectionClosed
	}

	c.mu.Lock()
	c.lastUsed = time.Now()
	c.mu.Unlock()

	// Default to Objects folder if no nodeID specified
	var startNodeID *ua.NodeID
	var err error
	if nodeID == "" {
		startNodeID = ua.NewNumericNodeID(0, 85) // Objects folder
	} else {
		startNodeID, err = c.getNodeID(nodeID)
		if err != nil {
			return nil, fmt.Errorf("invalid node ID: %w", err)
		}
	}

	// Ensure maxDepth is at least 1
	if maxDepth < 1 {
		maxDepth = 1
	}

	return c.browseNode(ctx, startNodeID, maxDepth, 0)
}

// browseNode recursively browses a node and its children.
func (c *Client) browseNode(ctx context.Context, nodeID *ua.NodeID, maxDepth, currentDepth int) (*BrowseResult, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, domain.ErrConnectionClosed
	}

	// Read node attributes first
	result := &BrowseResult{
		NodeID: nodeID.String(),
	}

	// Read DisplayName, BrowseName, NodeClass in batch
	if err := c.readNodeAttributes(ctx, nodeID, result); err != nil {
		c.logger.Debug().Err(err).Str("node_id", nodeID.String()).Msg("Failed to read node attributes")
	}

	// For Variable nodes, read DataType and AccessLevel
	if result.NodeClass == ua.NodeClassVariable {
		c.readVariableAttributes(ctx, nodeID, result)
	}

	// Stop recursion if we've reached max depth
	if currentDepth >= maxDepth {
		// Check if node has children without fetching them
		result.HasChildren = c.checkHasChildren(ctx, nodeID)
		return result, nil
	}

	// Browse children with HierarchicalReferences
	children, err := c.browseChildren(ctx, nodeID)
	if err != nil {
		c.logger.Debug().Err(err).Str("node_id", nodeID.String()).Msg("Failed to browse children")
		return result, nil
	}

	result.HasChildren = len(children) > 0

	// Recursively browse children
	for _, childRef := range children {
		childResult, err := c.browseNode(ctx, childRef.NodeID.NodeID, maxDepth, currentDepth+1)
		if err != nil {
			c.logger.Debug().Err(err).Str("node_id", childRef.NodeID.NodeID.String()).Msg("Failed to browse child node")
			continue
		}
		result.Children = append(result.Children, childResult)
	}

	return result, nil
}

// readNodeAttributes reads DisplayName, BrowseName, and NodeClass for a node.
func (c *Client) readNodeAttributes(ctx context.Context, nodeID *ua.NodeID, result *BrowseResult) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()

	req := &ua.ReadRequest{
		MaxAge:             0,
		TimestampsToReturn: ua.TimestampsToReturnNeither,
		NodesToRead: []*ua.ReadValueID{
			{NodeID: nodeID, AttributeID: ua.AttributeIDDisplayName},
			{NodeID: nodeID, AttributeID: ua.AttributeIDBrowseName},
			{NodeID: nodeID, AttributeID: ua.AttributeIDNodeClass},
		},
	}

	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return domain.ErrConnectionClosed
	}

	resp, err := client.Read(ctx, req)
	if err != nil {
		return err
	}

	// Parse DisplayName
	if len(resp.Results) > 0 && resp.Results[0].Status == ua.StatusOK {
		if lt, ok := resp.Results[0].Value.Value().(*ua.LocalizedText); ok && lt != nil {
			result.DisplayName = lt.Text
		}
	}

	// Parse BrowseName
	if len(resp.Results) > 1 && resp.Results[1].Status == ua.StatusOK {
		if qn, ok := resp.Results[1].Value.Value().(*ua.QualifiedName); ok && qn != nil {
			result.BrowseName = qn.Name
		}
	}

	// Parse NodeClass
	if len(resp.Results) > 2 && resp.Results[2].Status == ua.StatusOK {
		if nc, ok := resp.Results[2].Value.Value().(int32); ok {
			result.NodeClass = ua.NodeClass(nc)
			result.NodeClassName = nodeClassToString(result.NodeClass)
		}
	}

	return nil
}

// readVariableAttributes reads DataType and AccessLevel for Variable nodes.
func (c *Client) readVariableAttributes(ctx context.Context, nodeID *ua.NodeID, result *BrowseResult) {
	c.opMu.Lock()
	defer c.opMu.Unlock()

	req := &ua.ReadRequest{
		MaxAge:             0,
		TimestampsToReturn: ua.TimestampsToReturnNeither,
		NodesToRead: []*ua.ReadValueID{
			{NodeID: nodeID, AttributeID: ua.AttributeIDDataType},
			{NodeID: nodeID, AttributeID: ua.AttributeIDAccessLevel},
		},
	}

	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return
	}

	resp, err := client.Read(ctx, req)
	if err != nil {
		return
	}

	// Parse DataType
	if len(resp.Results) > 0 && resp.Results[0].Status == ua.StatusOK {
		if dtNodeID, ok := resp.Results[0].Value.Value().(*ua.NodeID); ok && dtNodeID != nil {
			result.DataType = dataTypeNodeIDToString(dtNodeID)
		}
	}

	// Parse AccessLevel
	if len(resp.Results) > 1 && resp.Results[1].Status == ua.StatusOK {
		if al, ok := resp.Results[1].Value.Value().(uint8); ok {
			result.AccessLevel = accessLevelToString(ua.AccessLevelType(al))
		}
	}
}

// browseChildren returns the child references of a node using HierarchicalReferences.
func (c *Client) browseChildren(ctx context.Context, nodeID *ua.NodeID) ([]*ua.ReferenceDescription, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()

	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, domain.ErrConnectionClosed
	}

	browseReq := &ua.BrowseRequest{
		RequestedMaxReferencesPerNode: 1000,
		NodesToBrowse: []*ua.BrowseDescription{
			{
				NodeID:          nodeID,
				BrowseDirection: ua.BrowseDirectionForward,
				ReferenceTypeID: ua.NewNumericNodeID(0, 33), // HierarchicalReferences
				IncludeSubtypes: true,
				NodeClassMask:   uint32(ua.NodeClassObject | ua.NodeClassVariable | ua.NodeClassMethod),
				ResultMask:      uint32(ua.BrowseResultMaskAll),
			},
		},
	}

	browseResp, err := client.Browse(ctx, browseReq)
	if err != nil {
		return nil, err
	}

	if len(browseResp.Results) == 0 {
		return nil, nil
	}

	browseResult := browseResp.Results[0]
	if browseResult.StatusCode != ua.StatusOK {
		return nil, fmt.Errorf("browse failed: status code %d", browseResult.StatusCode)
	}

	refs := browseResult.References

	// Handle continuation point for large result sets
	for len(browseResult.ContinuationPoint) > 0 {
		nextReq := &ua.BrowseNextRequest{
			ReleaseContinuationPoints: false,
			ContinuationPoints:        [][]byte{browseResult.ContinuationPoint},
		}

		nextResp, err := client.BrowseNext(ctx, nextReq)
		if err != nil {
			c.logger.Warn().Err(err).Msg("BrowseNext failed, returning partial results")
			break
		}

		if len(nextResp.Results) > 0 && nextResp.Results[0].StatusCode == ua.StatusOK {
			refs = append(refs, nextResp.Results[0].References...)
			browseResult = nextResp.Results[0]
		} else {
			break
		}
	}

	return refs, nil
}

// checkHasChildren checks if a node has children without fetching them.
func (c *Client) checkHasChildren(ctx context.Context, nodeID *ua.NodeID) bool {
	c.opMu.Lock()
	defer c.opMu.Unlock()

	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return false
	}

	browseReq := &ua.BrowseRequest{
		RequestedMaxReferencesPerNode: 1, // Only need to know if there's at least one
		NodesToBrowse: []*ua.BrowseDescription{
			{
				NodeID:          nodeID,
				BrowseDirection: ua.BrowseDirectionForward,
				ReferenceTypeID: ua.NewNumericNodeID(0, 33), // HierarchicalReferences
				IncludeSubtypes: true,
				NodeClassMask:   uint32(ua.NodeClassObject | ua.NodeClassVariable | ua.NodeClassMethod),
				ResultMask:      uint32(ua.BrowseResultMaskNodeClass),
			},
		},
	}

	browseResp, err := client.Browse(ctx, browseReq)
	if err != nil {
		return false
	}

	if len(browseResp.Results) > 0 && browseResp.Results[0].StatusCode == ua.StatusOK {
		return len(browseResp.Results[0].References) > 0
	}
	return false
}

// nodeClassToString converts NodeClass to human-readable string.
func nodeClassToString(nc ua.NodeClass) string {
	switch nc {
	case ua.NodeClassObject:
		return "Object"
	case ua.NodeClassVariable:
		return "Variable"
	case ua.NodeClassMethod:
		return "Method"
	case ua.NodeClassObjectType:
		return "ObjectType"
	case ua.NodeClassVariableType:
		return "VariableType"
	case ua.NodeClassReferenceType:
		return "ReferenceType"
	case ua.NodeClassDataType:
		return "DataType"
	case ua.NodeClassView:
		return "View"
	default:
		return "Unknown"
	}
}

// dataTypeNodeIDToString converts a DataType NodeID to a human-readable type name.
func dataTypeNodeIDToString(nodeID *ua.NodeID) string {
	if nodeID.Namespace() != 0 {
		return nodeID.String()
	}

	switch nodeID.IntID() {
	case 1:
		return "Boolean"
	case 2:
		return "SByte"
	case 3:
		return "Byte"
	case 4:
		return "Int16"
	case 5:
		return "UInt16"
	case 6:
		return "Int32"
	case 7:
		return "UInt32"
	case 8:
		return "Int64"
	case 9:
		return "UInt64"
	case 10:
		return "Float"
	case 11:
		return "Double"
	case 12:
		return "String"
	case 13:
		return "DateTime"
	case 14:
		return "Guid"
	case 15:
		return "ByteString"
	case 16:
		return "XmlElement"
	case 17:
		return "NodeId"
	case 19:
		return "StatusCode"
	case 21:
		return "LocalizedText"
	case 22:
		return "ExtensionObject"
	case 24:
		return "BaseDataType"
	default:
		return nodeID.String()
	}
}

// accessLevelToString converts AccessLevelType to human-readable string.
func accessLevelToString(al ua.AccessLevelType) string {
	var parts []string
	if al&ua.AccessLevelTypeCurrentRead != 0 {
		parts = append(parts, "Read")
	}
	if al&ua.AccessLevelTypeCurrentWrite != 0 {
		parts = append(parts, "Write")
	}
	if al&ua.AccessLevelTypeHistoryRead != 0 {
		parts = append(parts, "HistoryRead")
	}
	if al&ua.AccessLevelTypeHistoryWrite != 0 {
		parts = append(parts, "HistoryWrite")
	}
	if len(parts) == 0 {
		return "None"
	}
	return strings.Join(parts, ", ")
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

	return domain.AcquireDataPoint(
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
// This is safe to call concurrently - only one reconnect attempt will proceed.
func (c *Client) reconnect(ctx context.Context) {
	// Use TryLock to avoid blocking if another goroutine is already reconnecting.
	// If we can't get the lock, someone else is already handling reconnection.
	if !c.mu.TryLock() {
		c.logger.Debug().Msg("Reconnect already in progress, skipping")
		return
	}

	// Check if we're still connected (another goroutine may have reconnected)
	if c.connected.Load() {
		c.mu.Unlock()
		return
	}

	c.sessionState = SessionStateConnecting
	c.mu.Unlock()

	// Perform disconnect (which acquires its own lock)
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
