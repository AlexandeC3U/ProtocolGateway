// Package opcua provides connection pooling for OPC UA clients.
package opcua

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/nexus-edge/protocol-gateway/internal/metrics"
	"github.com/rs/zerolog"
	"github.com/sony/gobreaker"
)

// ConnectionPool manages a pool of OPC UA client connections.
type ConnectionPool struct {
	config         PoolConfig
	clients        map[string]*pooledClient
	mu             sync.RWMutex
	logger         zerolog.Logger
	metrics        *metrics.Registry
	circuitBreaker *gobreaker.CircuitBreaker
	closed         bool
	wg             sync.WaitGroup
}

// pooledClient wraps a Client with pool-specific metadata.
type pooledClient struct {
	client          *Client
	device          *domain.Device
	inUse           bool
	lastError       error
	connectFailures int
	nextReconnectAt time.Time
	mu              sync.Mutex
}

func (pc *pooledClient) canAttemptReconnect(now time.Time) bool {
	return pc.nextReconnectAt.IsZero() || !now.Before(pc.nextReconnectAt)
}

func (pc *pooledClient) recordConnectResult(now time.Time, err error, baseDelay time.Duration) {
	if err == nil {
		pc.lastError = nil
		pc.connectFailures = 0
		pc.nextReconnectAt = time.Time{}
		return
	}

	pc.lastError = err
	pc.connectFailures++

	// Default backoff base. Avoid overly aggressive reconnects.
	//
	// Note: Poll intervals are often 1-10s, so sub-second backoffs still
	// translate into a reconnect attempt on every poll cycle, which can
	// hammer servers and flood logs.
	if baseDelay <= 0 {
		baseDelay = 1 * time.Second
	}
	if baseDelay < 1*time.Second {
		baseDelay = 1 * time.Second
	}

	// TooManySessions is rarely resolved quickly; use a larger base delay.
	if isTooManySessionsError(err) {
		baseDelay = 1 * time.Minute
	} else if contains(err.Error(), "EOF") {
		// EOF typically indicates server-side socket close (overload, ACL, firewall,
		// stale sessions, etc.). Back off to avoid repeated connect storms.
		baseDelay = 30 * time.Second
	} else if baseDelay < 5*time.Second {
		// For general connection failures, use a more conservative base.
		baseDelay = 5 * time.Second
	}

	shift := pc.connectFailures - 1
	if shift > 6 {
		shift = 6
	}

	delay := baseDelay * time.Duration(1<<uint(shift))
	maxDelay := 5 * time.Minute
	if delay > maxDelay {
		delay = maxDelay
	}

	pc.nextReconnectAt = now.Add(delay)
}

// PoolConfig holds configuration for the connection pool.
type PoolConfig struct {
	// MaxConnections is the maximum number of concurrent connections
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
}

// DefaultPoolConfig returns a PoolConfig with sensible defaults.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxConnections:        50,
		IdleTimeout:           5 * time.Minute,
		HealthCheckPeriod:     30 * time.Second,
		ConnectionTimeout:     15 * time.Second,
		RetryAttempts:         3,
		RetryDelay:            500 * time.Millisecond,
		CircuitBreakerName:    "opcua-pool",
		DefaultSecurityPolicy: "None",
		DefaultSecurityMode:   "None",
		DefaultAuthMode:       "Anonymous",
	}
}

// NewConnectionPool creates a new connection pool.
func NewConnectionPool(config PoolConfig, logger zerolog.Logger, metricsReg *metrics.Registry) *ConnectionPool {
	// Apply defaults
	if config.MaxConnections == 0 {
		config.MaxConnections = 50
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

	// Create circuit breaker
	cbSettings := gobreaker.Settings{
		Name:        config.CircuitBreakerName,
		MaxRequests: 3,
		// Keep a longer rolling window than typical poll intervals,
		// otherwise counts reset before the breaker can trip.
		Interval: 1 * time.Minute,
		Timeout:  60 * time.Second, // OPC UA connections may take longer
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 5 && failureRatio >= 0.6
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			logger.Info().
				Str("name", name).
				Str("from", from.String()).
				Str("to", to.String()).
				Msg("Circuit breaker state changed")
		},
	}

	pool := &ConnectionPool{
		config:         config,
		clients:        make(map[string]*pooledClient),
		logger:         logger.With().Str("component", "opcua-pool").Logger(),
		metrics:        metricsReg,
		circuitBreaker: gobreaker.NewCircuitBreaker(cbSettings),
	}

	// Start background health checker
	pool.wg.Add(1)
	go pool.healthCheckLoop()

	// Start idle connection reaper
	pool.wg.Add(1)
	go pool.idleReaperLoop()

	return pool
}

// GetClient retrieves or creates a client for the given device.
func (p *ConnectionPool) GetClient(ctx context.Context, device *domain.Device) (*Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, domain.ErrServiceStopped
	}

	// Check if we already have a client for this device
	if pc, exists := p.clients[device.ID]; exists {
		pc.mu.Lock()
		defer pc.mu.Unlock()

		if pc.client.IsConnected() {
			return pc.client, nil
		}

		now := time.Now()
		if !pc.canAttemptReconnect(now) {
			// Avoid hammering the server when it is rejecting sessions.
			// Surface that backoff is active to make logs/diagnostics clearer.
			if pc.lastError != nil {
				return nil, fmt.Errorf("%w: reconnect backoff active: %v", domain.ErrConnectionFailed, pc.lastError)
			}
			return nil, fmt.Errorf("%w: reconnect backoff active", domain.ErrConnectionFailed)
		}

		// Try to reconnect
		err := pc.client.Connect(ctx)
		pc.recordConnectResult(now, err, p.config.RetryDelay)
		if err != nil {
			return nil, err
		}
		return pc.client, nil
	}

	// Check pool capacity
	if len(p.clients) >= p.config.MaxConnections {
		return nil, domain.ErrPoolExhausted
	}

	// Create new client (store it even if initial connect fails, so we can backoff)
	client, err := p.newClient(device)
	if err != nil {
		return nil, err
	}

	pc := &pooledClient{client: client, device: device}
	p.clients[device.ID] = pc

	p.logger.Info().
		Str("device_id", device.ID).
		Int("pool_size", len(p.clients)).
		Msg("Created new OPC UA client")

	now := time.Now()
	err = client.Connect(ctx)
	pc.recordConnectResult(now, err, p.config.RetryDelay)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// newClient creates a new OPC UA client for the device (does not connect).
func (p *ConnectionPool) newClient(device *domain.Device) (*Client, error) {
	// Build endpoint URL
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

	// Override with device-specific settings if provided
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

	// Apply defaults
	if clientConfig.RequestTimeout == 0 {
		clientConfig.RequestTimeout = 5 * time.Second
	}

	client, err := NewClient(device.ID, clientConfig, p.logger)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// ReadTags reads multiple tags from a device using the pooled connection.
func (p *ConnectionPool) ReadTags(ctx context.Context, device *domain.Device, tags []*domain.Tag) ([]*domain.DataPoint, error) {
	// Use circuit breaker
	result, err := p.circuitBreaker.Execute(func() (interface{}, error) {
		client, err := p.GetClient(ctx, device)
		if err != nil {
			return nil, err
		}
		return client.ReadTags(ctx, tags)
	})

	if err != nil {
		if err == gobreaker.ErrOpenState {
			return nil, domain.ErrCircuitBreakerOpen
		}
		return nil, err
	}

	return result.([]*domain.DataPoint), nil
}

// ReadTag reads a single tag from a device.
func (p *ConnectionPool) ReadTag(ctx context.Context, device *domain.Device, tag *domain.Tag) (*domain.DataPoint, error) {
	result, err := p.circuitBreaker.Execute(func() (interface{}, error) {
		client, err := p.GetClient(ctx, device)
		if err != nil {
			return nil, err
		}
		return client.ReadTag(ctx, tag)
	})

	if err != nil {
		if err == gobreaker.ErrOpenState {
			return nil, domain.ErrCircuitBreakerOpen
		}
		return nil, err
	}

	return result.(*domain.DataPoint), nil
}

// WriteTag writes a value to a tag on the device.
func (p *ConnectionPool) WriteTag(ctx context.Context, device *domain.Device, tag *domain.Tag, value interface{}) error {
	_, err := p.circuitBreaker.Execute(func() (interface{}, error) {
		client, err := p.GetClient(ctx, device)
		if err != nil {
			return nil, err
		}
		return nil, client.WriteTag(ctx, tag, value)
	})

	if err != nil {
		if err == gobreaker.ErrOpenState {
			return domain.ErrCircuitBreakerOpen
		}
		return err
	}

	return nil
}

// WriteTags writes multiple values to tags on the device.
func (p *ConnectionPool) WriteTags(ctx context.Context, device *domain.Device, writes []TagWrite) []error {
	result, err := p.circuitBreaker.Execute(func() (interface{}, error) {
		client, err := p.GetClient(ctx, device)
		if err != nil {
			// Return the same error for all writes
			errors := make([]error, len(writes))
			for i := range errors {
				errors[i] = err
			}
			return errors, nil
		}
		return client.WriteTags(ctx, writes), nil
	})

	if err != nil {
		errors := make([]error, len(writes))
		if err == gobreaker.ErrOpenState {
			for i := range errors {
				errors[i] = domain.ErrCircuitBreakerOpen
			}
		} else {
			for i := range errors {
				errors[i] = err
			}
		}
		return errors
	}

	return result.([]error)
}

// RemoveClient removes a client from the pool and closes its connection.
func (p *ConnectionPool) RemoveClient(deviceID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	pc, exists := p.clients[deviceID]
	if !exists {
		return domain.ErrDeviceNotFound
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	if err := pc.client.Disconnect(); err != nil {
		p.logger.Warn().Err(err).Str("device_id", deviceID).Msg("Error disconnecting client")
	}

	delete(p.clients, deviceID)
	p.logger.Info().Str("device_id", deviceID).Msg("Removed client from pool")

	return nil
}

// Close closes all connections and stops the pool.
func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()

	// Wait for background goroutines to stop
	p.wg.Wait()

	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for deviceID, pc := range p.clients {
		pc.mu.Lock()
		if err := pc.client.Disconnect(); err != nil {
			lastErr = err
			p.logger.Warn().Err(err).Str("device_id", deviceID).Msg("Error closing client")
		}
		pc.mu.Unlock()
	}

	p.clients = make(map[string]*pooledClient)
	p.logger.Info().Msg("Connection pool closed")

	return lastErr
}

// healthCheckLoop periodically checks the health of all connections.
func (p *ConnectionPool) healthCheckLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.HealthCheckPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.mu.RLock()
			if p.closed {
				p.mu.RUnlock()
				return
			}

			// Copy device IDs to avoid holding lock during health checks
			deviceIDs := make([]string, 0, len(p.clients))
			for id := range p.clients {
				deviceIDs = append(deviceIDs, id)
			}
			p.mu.RUnlock()

			for _, deviceID := range deviceIDs {
				p.checkClientHealth(deviceID)
			}
		}
	}
}

// checkClientHealth checks and potentially reconnects a client.
func (p *ConnectionPool) checkClientHealth(deviceID string) {
	p.mu.RLock()
	pc, exists := p.clients[deviceID]
	p.mu.RUnlock()

	if !exists {
		return
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	// If the pool-wide breaker is open, avoid background reconnect attempts.
	// Polling will observe the breaker state and report it; reconnecting here
	// just adds noise and can keep hammering an unhealthy endpoint.
	if p.circuitBreaker.State() == gobreaker.StateOpen {
		return
	}

	if !pc.client.IsConnected() {
		now := time.Now()
		if !pc.canAttemptReconnect(now) {
			return
		}
		p.logger.Debug().Str("device_id", deviceID).Msg("Client disconnected, attempting reconnect")

		ctx, cancel := context.WithTimeout(context.Background(), p.config.ConnectionTimeout)
		defer cancel()

		err := pc.client.Connect(ctx)
		pc.recordConnectResult(now, err, p.config.RetryDelay)
		if err != nil {
			p.logger.Warn().Err(err).Str("device_id", deviceID).Msg("Failed to reconnect client")
		} else {
			p.logger.Info().Str("device_id", deviceID).Msg("Client reconnected")
		}
	}
}

// idleReaperLoop removes idle connections that haven't been used.
func (p *ConnectionPool) idleReaperLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.IdleTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.mu.RLock()
			if p.closed {
				p.mu.RUnlock()
				return
			}
			p.mu.RUnlock()

			p.reapIdleConnections()
		}
	}
}

// reapIdleConnections closes connections that have been idle too long.
func (p *ConnectionPool) reapIdleConnections() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	for deviceID, pc := range p.clients {
		pc.mu.Lock()
		if now.Sub(pc.client.LastUsed()) > p.config.IdleTimeout {
			p.logger.Debug().Str("device_id", deviceID).Msg("Closing idle connection")
			pc.client.Disconnect()
			delete(p.clients, deviceID)
		}
		pc.mu.Unlock()
	}
}

// Stats returns pool statistics.
func (p *ConnectionPool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := PoolStats{
		TotalConnections: len(p.clients),
		MaxConnections:   p.config.MaxConnections,
	}

	for _, pc := range p.clients {
		pc.mu.Lock()
		if pc.client.IsConnected() {
			stats.ActiveConnections++
		}
		if pc.inUse {
			stats.InUseConnections++
		}
		pc.mu.Unlock()
	}

	return stats
}

// PoolStats contains pool statistics.
type PoolStats struct {
	TotalConnections  int
	ActiveConnections int
	InUseConnections  int
	MaxConnections    int
}

// HealthCheck implements the health.Checker interface.
func (p *ConnectionPool) HealthCheck(ctx context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return domain.ErrServiceStopped
	}

	// Check circuit breaker state
	state := p.circuitBreaker.State()
	if state == gobreaker.StateOpen {
		return domain.ErrCircuitBreakerOpen
	}

	return nil
}
