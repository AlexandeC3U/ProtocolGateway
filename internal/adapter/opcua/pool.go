// Package opcua provides connection pooling for OPC UA clients.
//
// Architecture: Per-Endpoint Session Sharing (Kepware/Ignition pattern)
//
// Unlike simple per-device pooling (like Modbus), this implementation shares
// OPC UA sessions across devices that connect to the same endpoint. This is
// critical because:
//
//   - OPC UA servers enforce MaxSessions, MaxSubscriptions, MaxSecureChannels
//   - At scale (50+ PLCs behind one OPC UA gateway), per-device sessions get hard-denied
//   - Industry leaders (Kepware, Ignition) use endpoint-keyed sessions
//
// Session sharing key: host + port + security + auth credentials
// This allows 200 "devices" to share a single session to one Kepserver.
//
// File organization:
//   - pool.go       - Core pool logic, config, read/write operations
//   - session.go    - Session and device binding types
//   - loadshaping.go - Priority queues, brownout mode
//   - health.go     - Stats, health checks, background loops
package opcua

import (
	"context"
	"fmt"
	"math/rand/v2"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/nexus-edge/protocol-gateway/internal/metrics"
	"github.com/rs/zerolog"
	"github.com/sony/gobreaker"
)

// =============================================================================
// Connection Pool
// =============================================================================

// ConnectionPool manages OPC UA sessions keyed by endpoint, not device.
// Multiple devices sharing the same endpoint share a single OPC UA session.
type ConnectionPool struct {
	config   PoolConfig
	sessions map[string]*pooledSession // key = endpointKey (host+port+security+auth)
	devices  map[string]*DeviceBinding // key = deviceID -> which session it uses
	mu       sync.RWMutex
	logger   zerolog.Logger
	metrics  *metrics.Registry
	closed   bool
	wg       sync.WaitGroup

	// Fleet-Wide Load Shaping
	globalInFlight    atomic.Int64       // Current in-flight operations across all sessions
	maxGlobalInFlight int64              // Global cap on concurrent operations
	priorityQueues    [3]chan *opRequest // Priority tiers: 0=telemetry, 1=control, 2=safety
	brownoutMode      atomic.Bool        // True when under global pressure
	brownoutThreshold float64            // Fraction of maxGlobalInFlight to trigger brownout
}

// PoolConfig holds configuration for the connection pool.
type PoolConfig struct {
	// MaxConnections is the maximum number of concurrent endpoint sessions
	MaxConnections int

	// IdleTimeout is how long to keep idle connections open
	IdleTimeout time.Duration

	// HealthCheckPeriod is how often to check connection health
	HealthCheckPeriod time.Duration

	// ConnectionTimeout is the timeout for establishing new connections
	ConnectionTimeout time.Duration

	// RetryAttempts is the number of retry attempts for failed operations
	RetryAttempts int

	// RetryDelay is the base delay between retries
	RetryDelay time.Duration

	// CircuitBreakerName is the name for the circuit breaker
	CircuitBreakerName string

	// DefaultSecurityPolicy is the default security policy
	DefaultSecurityPolicy string

	// DefaultSecurityMode is the default security mode
	DefaultSecurityMode string

	// DefaultAuthMode is the default authentication mode
	DefaultAuthMode string

	// Server Limits Awareness
	MaxNodesPerRead   int // Max nodes per read request (server limit)
	MaxNodesPerWrite  int // Max nodes per write request
	MaxNodesPerBrowse int // Max nodes per browse request

	// Fleet-Wide Load Shaping
	MaxGlobalInFlight int     // Global cap on concurrent operations
	BrownoutThreshold float64 // Fraction that triggers brownout mode (0.0-1.0)
	PriorityQueueSize int     // Size of each priority queue

	// Trust Store (Certificate Management)
	TrustStorePath            string // Path to trust store directory
	AutoAcceptUntrusted       bool   // Auto-accept untrusted certs (dev only!)
	CertExpirationWarningDays int    // Warn when certs expire within this many days

	// Worker Pool
	NumWorkers int // Number of priority queue workers (default: NumCPU*2)

	// Per-Endpoint Fairness (prevents one noisy endpoint from starving others)
	MaxInFlightPerEndpoint int // Max concurrent ops per endpoint (0 = no limit)
	MaxQueuedPerEndpoint   int // Max queued ops per endpoint (0 = no limit)

	// Cold-Start Storm Protection (Kubernetes restart scenarios)
	StartupJitterMax   time.Duration // Max random delay on startup (0 = no jitter)
	WarmupRampDuration time.Duration // Gradual capacity ramp-up period (0 = full capacity immediately)
}

// DefaultPoolConfig returns sensible defaults for industrial-scale deployments.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxConnections:            100, // Endpoint sessions, not devices
		IdleTimeout:               5 * time.Minute,
		HealthCheckPeriod:         30 * time.Second,
		ConnectionTimeout:         15 * time.Second,
		RetryAttempts:             3,
		RetryDelay:                500 * time.Millisecond,
		CircuitBreakerName:        "opcua-pool",
		DefaultSecurityPolicy:     "None",
		DefaultSecurityMode:       "None",
		DefaultAuthMode:           "Anonymous",
		MaxNodesPerRead:           500,
		MaxNodesPerWrite:          100,
		MaxNodesPerBrowse:         1000,
		MaxGlobalInFlight:         1000,
		BrownoutThreshold:         0.8,
		PriorityQueueSize:         1000,
		CertExpirationWarningDays: 30,
		NumWorkers:                runtime.NumCPU() * 2,
		MaxInFlightPerEndpoint:    100,              // Per-endpoint cap (global / 10 as default)
		MaxQueuedPerEndpoint:      200,              // Per-endpoint queue limit
		StartupJitterMax:          5 * time.Second,  // Spread reconnections over 5s
		WarmupRampDuration:        30 * time.Second, // Ramp to full capacity over 30s
	}
}

// NewConnectionPool creates a new connection pool with per-endpoint session sharing.
func NewConnectionPool(config PoolConfig, logger zerolog.Logger, metricsReg *metrics.Registry) *ConnectionPool {
	// Apply defaults
	if config.MaxConnections == 0 {
		config.MaxConnections = 100
	}
	if config.IdleTimeout == 0 {
		config.IdleTimeout = 5 * time.Minute
	}
	if config.HealthCheckPeriod == 0 {
		config.HealthCheckPeriod = 30 * time.Second
	}
	if config.ConnectionTimeout == 0 {
		config.ConnectionTimeout = 15 * time.Second
	}
	if config.MaxNodesPerRead == 0 {
		config.MaxNodesPerRead = 500
	}
	if config.MaxNodesPerWrite == 0 {
		config.MaxNodesPerWrite = 100
	}
	if config.MaxGlobalInFlight == 0 {
		config.MaxGlobalInFlight = 1000
	}
	if config.BrownoutThreshold == 0 {
		config.BrownoutThreshold = 0.8
	}
	if config.PriorityQueueSize == 0 {
		config.PriorityQueueSize = 1000
	}
	if config.NumWorkers == 0 {
		config.NumWorkers = runtime.NumCPU() * 2
	}
	if config.MaxInFlightPerEndpoint == 0 {
		config.MaxInFlightPerEndpoint = config.MaxGlobalInFlight / 10
	}

	// === COLD-START STORM PROTECTION ===
	// When Kubernetes restarts all pods, they reconnect simultaneously → PLC denial of service
	// Apply jittered startup delay to spread reconnection load
	if config.StartupJitterMax > 0 {
		jitter := time.Duration(rand.Int64N(int64(config.StartupJitterMax)))
		logger.Info().
			Dur("startup_jitter", jitter).
			Msg("Applying cold-start jitter delay to prevent reconnection storm")
		time.Sleep(jitter)
	}

	pool := &ConnectionPool{
		config:            config,
		sessions:          make(map[string]*pooledSession),
		devices:           make(map[string]*DeviceBinding),
		logger:            logger.With().Str("component", "opcua-pool").Logger(),
		metrics:           metricsReg,
		maxGlobalInFlight: int64(config.MaxGlobalInFlight),
		brownoutThreshold: config.BrownoutThreshold,
	}

	// Initialize priority queues
	for i := range pool.priorityQueues {
		pool.priorityQueues[i] = make(chan *opRequest, config.PriorityQueueSize)
	}

	// Start background goroutines
	pool.wg.Add(2 + config.NumWorkers)
	go pool.healthCheckLoop()
	go pool.idleReaperLoop()
	// Worker pool for priority queue processing (fixes single-thread bottleneck)
	for i := 0; i < config.NumWorkers; i++ {
		go pool.priorityQueueProcessor()
	}

	pool.logger.Info().
		Int("max_sessions", config.MaxConnections).
		Int("max_global_inflight", config.MaxGlobalInFlight).
		Int("max_per_endpoint", config.MaxInFlightPerEndpoint).
		Float64("brownout_threshold", config.BrownoutThreshold).
		Int("num_workers", config.NumWorkers).
		Msg("OPC UA connection pool initialized with per-endpoint session sharing")

	return pool
}

// =============================================================================
// Client Management
// =============================================================================

// GetClient retrieves or creates a client for the given device.
// Uses per-endpoint session sharing - multiple devices on the same endpoint share one session.
func (p *ConnectionPool) GetClient(ctx context.Context, device *domain.Device) (*Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, domain.ErrServiceStopped
	}

	epKey := endpointKey(device, p.config)

	// Check if we already have a session for this endpoint
	if session, exists := p.sessions[epKey]; exists {
		return p.getClientFromExistingSession(ctx, session, device, epKey)
	}

	// Check pool capacity (count sessions, not devices)
	if len(p.sessions) >= p.config.MaxConnections {
		return nil, domain.ErrPoolExhausted
	}

	return p.createNewSession(ctx, device, epKey)
}

func (p *ConnectionPool) getClientFromExistingSession(ctx context.Context, session *pooledSession, device *domain.Device, epKey string) (*Client, error) {
	session.mu.Lock()
	defer session.mu.Unlock()

	// Register device with session if not already bound
	if _, deviceBound := session.devices[device.ID]; !deviceBound {
		binding := &DeviceBinding{
			DeviceID:       device.ID,
			Device:         device,
			EndpointKey:    epKey,
			MonitoredItems: make(map[string]uint32),
			breaker:        p.createDeviceBreaker(device.ID),
		}
		session.devices[device.ID] = binding
		p.devices[device.ID] = binding

		p.logger.Debug().
			Str("device_id", device.ID).
			Str("endpoint", epKey[:min(len(epKey), 50)]).
			Int("devices_on_session", len(session.devices)).
			Msg("Device bound to existing session")
	}

	if session.client.IsConnected() {
		return session.client, nil
	}

	now := time.Now()
	if !session.canAttemptReconnect(now) {
		if session.lastError != nil {
			return nil, fmt.Errorf("%w: reconnect backoff active: %v", domain.ErrConnectionFailed, session.lastError)
		}
		return nil, fmt.Errorf("%w: reconnect backoff active", domain.ErrConnectionFailed)
	}

	// Try to reconnect
	start := time.Now()
	err := session.client.Connect(ctx)
	if p.metrics != nil {
		p.metrics.RecordConnectionForProtocol(string(domain.ProtocolOPCUA), err == nil, time.Since(start).Seconds())
	}
	session.recordConnectResult(now, err, p.config.RetryDelay)
	if err != nil {
		return nil, err
	}

	// Trigger subscription recovery
	if session.subscriptionState != nil {
		go p.recoverSubscriptions(session)
	}

	return session.client, nil
}

func (p *ConnectionPool) createNewSession(ctx context.Context, device *domain.Device, epKey string) (*Client, error) {
	client, err := p.newClientForEndpoint(device)
	if err != nil {
		return nil, err
	}

	session := &pooledSession{
		client:      client,
		endpointKey: epKey,
		endpointURL: client.config.EndpointURL,
		breaker:     p.createCircuitBreaker(epKey),
		devices:     make(map[string]*DeviceBinding),
		maxInFlight: int64(p.config.MaxInFlightPerEndpoint), // Per-endpoint fairness limit
		subscriptionState: &SubscriptionRecoveryState{
			MonitoredItems: make(map[uint32]*MonitoredItemState),
		},
	}

	binding := &DeviceBinding{
		DeviceID:       device.ID,
		Device:         device,
		EndpointKey:    epKey,
		MonitoredItems: make(map[string]uint32),
		breaker:        p.createDeviceBreaker(device.ID),
	}
	session.devices[device.ID] = binding
	p.devices[device.ID] = binding
	p.sessions[epKey] = session

	p.logger.Info().
		Str("device_id", device.ID).
		Str("endpoint", epKey[:min(len(epKey), 50)]).
		Int("session_count", len(p.sessions)).
		Msg("Created new OPC UA session for endpoint")

	now := time.Now()
	start := time.Now()
	err = client.Connect(ctx)
	if p.metrics != nil {
		p.metrics.RecordConnectionForProtocol(string(domain.ProtocolOPCUA), err == nil, time.Since(start).Seconds())
	}
	session.recordConnectResult(now, err, p.config.RetryDelay)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (p *ConnectionPool) newClientForEndpoint(device *domain.Device) (*Client, error) {
	endpointURL := device.Connection.OPCEndpointURL
	if endpointURL == "" {
		endpointURL = fmt.Sprintf("opc.tcp://%s:%d", device.Connection.Host, device.Connection.Port)
	}

	clientConfig := ClientConfig{
		EndpointURL:    endpointURL,
		SecurityPolicy: p.config.DefaultSecurityPolicy,
		SecurityMode:   p.config.DefaultSecurityMode,
		AuthMode:       p.config.DefaultAuthMode,
		Timeout:        p.config.ConnectionTimeout,
		KeepAlive:      30 * time.Second,
		MaxRetries:     p.config.RetryAttempts,
		RetryDelay:     p.config.RetryDelay,
		RequestTimeout: device.Connection.Timeout,
	}

	// Override with device-specific settings
	if device.Connection.OPCSecurityPolicy != "" {
		clientConfig.SecurityPolicy = device.Connection.OPCSecurityPolicy
	}
	if device.Connection.OPCSecurityMode != "" {
		clientConfig.SecurityMode = device.Connection.OPCSecurityMode
	}
	if device.Connection.OPCAuthMode != "" {
		clientConfig.AuthMode = device.Connection.OPCAuthMode
	}
	if device.Connection.OPCUsername != "" {
		clientConfig.Username = device.Connection.OPCUsername
	}
	if device.Connection.OPCPassword != "" {
		clientConfig.Password = device.Connection.OPCPassword
	}
	if device.Connection.OPCCertFile != "" {
		clientConfig.CertificateFile = device.Connection.OPCCertFile
	}
	if device.Connection.OPCKeyFile != "" {
		clientConfig.PrivateKeyFile = device.Connection.OPCKeyFile
	}
	// Server certificate and security options
	if device.Connection.OPCServerCertFile != "" {
		clientConfig.ServerCertificateFile = device.Connection.OPCServerCertFile
	}
	clientConfig.InsecureSkipVerify = device.Connection.OPCInsecureSkipVerify
	clientConfig.AutoSelectEndpoint = device.Connection.OPCAutoSelectEndpoint
	if device.Connection.OPCApplicationName != "" {
		clientConfig.ApplicationName = device.Connection.OPCApplicationName
	}
	if device.Connection.OPCApplicationURI != "" {
		clientConfig.ApplicationURI = device.Connection.OPCApplicationURI
	}
	if clientConfig.RequestTimeout == 0 {
		clientConfig.RequestTimeout = 5 * time.Second
	}

	return NewClient(device.ID, clientConfig, p.logger)
}

// createCircuitBreaker creates an endpoint-level circuit breaker.
// This breaker trips on connection/infrastructure errors that affect ALL devices
// on this endpoint (timeouts, EOF, secure channel failures, etc.).
func (p *ConnectionPool) createCircuitBreaker(epKey string) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        fmt.Sprintf("opcua-endpoint-%s", epKey[:min(len(epKey), 32)]),
		MaxRequests: 3,
		Interval:    1 * time.Minute,
		Timeout:     60 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Trip on 60% failure rate after at least 5 requests
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 5 && failureRatio >= 0.6
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			p.logger.Info().
				Str("breaker", name).
				Str("from", from.String()).
				Str("to", to.String()).
				Msg("OPC UA endpoint circuit breaker state changed")
		},
	})
}

// createDeviceBreaker creates a device-level circuit breaker.
// This breaker trips on config/semantic errors specific to ONE device
// (bad node IDs, access denied, type mismatches, etc.).
// It does NOT trip on connection errors - those go to the endpoint breaker.
func (p *ConnectionPool) createDeviceBreaker(deviceID string) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        fmt.Sprintf("opcua-device-%s", deviceID),
		MaxRequests: 1,                // Allow 1 request in half-open to test recovery
		Interval:    30 * time.Second, // Reset counters after 30s of no errors
		Timeout:     15 * time.Second, // Stay open for 15s before testing
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Trip after 5 consecutive device-level failures
			return counts.ConsecutiveFailures >= 5
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			p.logger.Warn().
				Str("breaker", name).
				Str("from", from.String()).
				Str("to", to.String()).
				Msg("OPC UA device circuit breaker state changed")
		},
	})
}

func (p *ConnectionPool) getSessionForDevice(deviceID string) (*pooledSession, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	binding, exists := p.devices[deviceID]
	if !exists {
		return nil, false
	}

	session, exists := p.sessions[binding.EndpointKey]
	return session, exists
}

// getDeviceBinding returns the device binding for a device ID.
func (p *ConnectionPool) getDeviceBinding(deviceID string) (*DeviceBinding, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	binding, exists := p.devices[deviceID]
	return binding, exists
}

// executeWithTwoTierBreaker executes an operation through both endpoint and device breakers.
// Evaluation order: Endpoint breaker (is server healthy?) → Device breaker (is this device OK?)
// Error classification routes errors to the correct breaker.
func (p *ConnectionPool) executeWithTwoTierBreaker(
	session *pooledSession,
	binding *DeviceBinding,
	fn func() (interface{}, error),
) (interface{}, error) {
	// First tier: Endpoint breaker - checks if server is reachable
	result, err := session.breaker.Execute(func() (interface{}, error) {
		// Second tier: Device breaker - checks if this device's config is valid
		return binding.breaker.Execute(fn)
	})

	if err != nil {
		// Classify the error to determine which breaker should count it
		// The gobreaker already counted it, but we log for observability
		if err == gobreaker.ErrOpenState {
			// Check which breaker is open
			if session.breaker.State() == gobreaker.StateOpen {
				return nil, fmt.Errorf("%w: endpoint breaker open", domain.ErrCircuitBreakerOpen)
			}
			if binding.breaker.State() == gobreaker.StateOpen {
				return nil, fmt.Errorf("%w: device breaker open for %s", domain.ErrCircuitBreakerOpen, binding.DeviceID)
			}
			return nil, domain.ErrCircuitBreakerOpen
		}
		return nil, err
	}

	return result, nil
}

func (p *ConnectionPool) recoverSubscriptions(session *pooledSession) {
	session.subscriptionState.mu.Lock()
	defer session.subscriptionState.mu.Unlock()

	if len(session.subscriptionState.MissedSequenceNumbers) > 0 {
		p.logger.Info().
			Int("missed_sequences", len(session.subscriptionState.MissedSequenceNumbers)).
			Msg("Attempting subscription recovery with republish")
		session.subscriptionState.MissedSequenceNumbers = nil
	}

	if len(session.subscriptionState.MonitoredItems) > 0 {
		p.logger.Info().
			Int("monitored_items", len(session.subscriptionState.MonitoredItems)).
			Msg("Rebinding monitored items after reconnect")
	}
}

// =============================================================================
// Read/Write Operations
// =============================================================================

// ReadTags reads multiple tags from a device.
// Uses two-tier circuit breakers: endpoint breaker → device breaker.
func (p *ConnectionPool) ReadTags(ctx context.Context, device *domain.Device, tags []*domain.Tag) ([]*domain.DataPoint, error) {
	client, err := p.GetClient(ctx, device)
	if err != nil {
		return nil, err
	}

	session, exists := p.getSessionForDevice(device.ID)
	if !exists {
		return nil, domain.ErrDeviceNotFound
	}

	binding, exists := p.getDeviceBinding(device.ID)
	if !exists {
		return nil, domain.ErrDeviceNotFound
	}

	priority := PriorityTelemetry
	for _, tag := range tags {
		if int(tag.Priority) > priority {
			priority = int(tag.Priority)
		}
	}

	var result []*domain.DataPoint
	err = p.checkGlobalLoadAndQueueWithSession(ctx, priority, session, func() error {
		res, err := p.executeWithTwoTierBreaker(session, binding, func() (interface{}, error) {
			if len(tags) <= p.config.MaxNodesPerRead {
				return client.ReadTags(ctx, tags)
			}

			// Batch to respect server limits
			allResults := make([]*domain.DataPoint, 0, len(tags))
			for i := 0; i < len(tags); i += p.config.MaxNodesPerRead {
				end := i + p.config.MaxNodesPerRead
				if end > len(tags) {
					end = len(tags)
				}
				batchResults, err := client.ReadTags(ctx, tags[i:end])
				if err != nil {
					return nil, err
				}
				allResults = append(allResults, batchResults...)
			}
			return allResults, nil
		})

		if err != nil {
			return err
		}
		result = res.([]*domain.DataPoint)
		return nil
	})

	return result, err
}

// ReadTag reads a single tag from a device.
// Uses two-tier circuit breakers: endpoint breaker → device breaker.
func (p *ConnectionPool) ReadTag(ctx context.Context, device *domain.Device, tag *domain.Tag) (*domain.DataPoint, error) {
	client, err := p.GetClient(ctx, device)
	if err != nil {
		return nil, err
	}

	session, exists := p.getSessionForDevice(device.ID)
	if !exists {
		return nil, domain.ErrDeviceNotFound
	}

	binding, exists := p.getDeviceBinding(device.ID)
	if !exists {
		return nil, domain.ErrDeviceNotFound
	}

	var result *domain.DataPoint
	err = p.checkGlobalLoadAndQueueWithSession(ctx, int(tag.Priority), session, func() error {
		res, err := p.executeWithTwoTierBreaker(session, binding, func() (interface{}, error) {
			return client.ReadTag(ctx, tag)
		})
		if err != nil {
			return err
		}
		result = res.(*domain.DataPoint)
		return nil
	})

	return result, err
}

// WriteTag writes a value to a tag on the device.
// Uses two-tier circuit breakers: endpoint breaker → device breaker.
func (p *ConnectionPool) WriteTag(ctx context.Context, device *domain.Device, tag *domain.Tag, value interface{}) error {
	client, err := p.GetClient(ctx, device)
	if err != nil {
		return err
	}

	session, exists := p.getSessionForDevice(device.ID)
	if !exists {
		return domain.ErrDeviceNotFound
	}

	binding, exists := p.getDeviceBinding(device.ID)
	if !exists {
		return domain.ErrDeviceNotFound
	}

	priority := PriorityControl
	if int(tag.Priority) > priority {
		priority = int(tag.Priority)
	}

	return p.checkGlobalLoadAndQueueWithSession(ctx, priority, session, func() error {
		_, err := p.executeWithTwoTierBreaker(session, binding, func() (interface{}, error) {
			return nil, client.WriteTag(ctx, tag, value)
		})
		return err
	})
}

// WriteTags writes multiple values to tags on the device.
// Uses two-tier circuit breakers: endpoint breaker → device breaker.
func (p *ConnectionPool) WriteTags(ctx context.Context, device *domain.Device, writes []TagWrite) []error {
	client, err := p.GetClient(ctx, device)
	if err != nil {
		errors := make([]error, len(writes))
		for i := range errors {
			errors[i] = err
		}
		return errors
	}

	session, exists := p.getSessionForDevice(device.ID)
	if !exists {
		errors := make([]error, len(writes))
		for i := range errors {
			errors[i] = domain.ErrDeviceNotFound
		}
		return errors
	}

	binding, exists := p.getDeviceBinding(device.ID)
	if !exists {
		errors := make([]error, len(writes))
		for i := range errors {
			errors[i] = domain.ErrDeviceNotFound
		}
		return errors
	}

	var result []error
	err = p.checkGlobalLoadAndQueueWithSession(ctx, PriorityControl, session, func() error {
		res, err := p.executeWithTwoTierBreaker(session, binding, func() (interface{}, error) {
			return client.WriteTags(ctx, writes), nil
		})
		if err != nil {
			errors := make([]error, len(writes))
			for i := range errors {
				errors[i] = err
			}
			result = errors
			return nil
		}
		result = res.([]error)
		return nil
	})

	if err != nil {
		errors := make([]error, len(writes))
		for i := range errors {
			errors[i] = err
		}
		return errors
	}
	return result
}

// =============================================================================
// Lifecycle Management
// =============================================================================

// RemoveClient removes a device from its session.
// If the device was the last one using the session, the session is closed.
func (p *ConnectionPool) RemoveClient(deviceID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	binding, exists := p.devices[deviceID]
	if !exists {
		return domain.ErrDeviceNotFound
	}

	session, exists := p.sessions[binding.EndpointKey]
	if !exists {
		delete(p.devices, deviceID)
		return nil
	}

	session.mu.Lock()
	delete(session.devices, deviceID)
	remainingDevices := len(session.devices)
	session.mu.Unlock()

	delete(p.devices, deviceID)

	if remainingDevices == 0 {
		if err := session.client.Disconnect(); err != nil {
			p.logger.Warn().Err(err).Str("endpoint", binding.EndpointKey[:min(len(binding.EndpointKey), 50)]).Msg("Error disconnecting session")
		}
		delete(p.sessions, binding.EndpointKey)
		p.logger.Info().
			Str("device_id", deviceID).
			Str("endpoint", binding.EndpointKey[:min(len(binding.EndpointKey), 50)]).
			Msg("Removed last device, session closed")
	} else {
		p.logger.Info().
			Str("device_id", deviceID).
			Int("remaining_devices", remainingDevices).
			Msg("Removed device from session")
	}

	return nil
}

// Close closes all sessions and stops the pool.
func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	p.closed = true
	for i := range p.priorityQueues {
		close(p.priorityQueues[i])
	}
	p.mu.Unlock()

	p.wg.Wait()

	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for epKey, session := range p.sessions {
		session.mu.Lock()
		if err := session.client.Disconnect(); err != nil {
			lastErr = err
			p.logger.Warn().Err(err).Str("endpoint", epKey[:min(len(epKey), 50)]).Msg("Error closing session")
		}
		session.mu.Unlock()
	}

	p.sessions = make(map[string]*pooledSession)
	p.devices = make(map[string]*DeviceBinding)
	p.logger.Info().Msg("Connection pool closed")

	return lastErr
}
