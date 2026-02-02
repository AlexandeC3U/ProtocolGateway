// Package s7 provides S7 connection pooling with circuit breaker protection.
package s7

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

// Pool manages a pool of S7 clients with circuit breaker protection.
type Pool struct {
	clients        map[string]*clientEntry
	mu             sync.RWMutex
	logger         zerolog.Logger
	metrics        *metrics.Registry
	maxConnections int
	idleTimeout    time.Duration
	healthCheck    time.Duration
	stopChan       chan struct{}
	wg             sync.WaitGroup
	closed         bool
}

// clientEntry represents a pooled client with its circuit breaker.
type clientEntry struct {
	client          *Client
	device          *domain.Device
	breaker         *gobreaker.CircuitBreaker
	lastUse         time.Time
	lastError       error
	connectFailures int
	nextReconnectAt time.Time
	mu              sync.Mutex
}

// PoolConfig holds configuration for the connection pool.
type PoolConfig struct {
	// MaxConnections is the maximum number of concurrent connections
	MaxConnections int

	// IdleTimeout is how long to keep idle connections open
	IdleTimeout time.Duration

	// HealthCheckInterval is how often to check connection health
	HealthCheckInterval time.Duration

	// RetryDelay is the base delay for reconnection attempts
	RetryDelay time.Duration

	// CircuitBreakerConfig holds circuit breaker settings
	CircuitBreaker CircuitBreakerConfig
}

// CircuitBreakerConfig holds circuit breaker configuration.
type CircuitBreakerConfig struct {
	// MaxRequests is the maximum number of requests allowed to pass when half-open
	MaxRequests uint32

	// Interval is the cyclic period of the closed state to clear internal counts
	Interval time.Duration

	// Timeout is the period of the open state before transitioning to half-open
	Timeout time.Duration

	// FailureThreshold is the number of failures before opening the circuit
	FailureThreshold uint32

	// SuccessThreshold is the number of successes required in half-open to close
	SuccessThreshold uint32
}

// NewPool creates a new S7 connection pool.
func NewPool(config PoolConfig, logger zerolog.Logger, metricsReg *metrics.Registry) *Pool {
	if config.MaxConnections == 0 {
		config.MaxConnections = 100
	}
	if config.IdleTimeout == 0 {
		config.IdleTimeout = 5 * time.Minute
	}
	if config.HealthCheckInterval == 0 {
		config.HealthCheckInterval = 30 * time.Second
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 5 * time.Second
	}

	pool := &Pool{
		clients:        make(map[string]*clientEntry),
		logger:         logger.With().Str("component", "s7-pool").Logger(),
		metrics:        metricsReg,
		maxConnections: config.MaxConnections,
		idleTimeout:    config.IdleTimeout,
		healthCheck:    config.HealthCheckInterval,
		stopChan:       make(chan struct{}),
	}

	// Start background health check
	pool.wg.Add(1)
	go pool.healthCheckLoop()

	// Start idle connection reaper
	pool.wg.Add(1)
	go pool.idleReaperLoop()

	return pool
}

// GetOrCreate gets an existing client or creates a new one for the device.
func (p *Pool) GetOrCreate(ctx context.Context, device *domain.Device) (*Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check for existing client
	if entry, exists := p.clients[device.ID]; exists {
		entry.lastUse = time.Now()
		return entry.client, nil
	}

	// Check pool capacity
	if len(p.clients) >= p.maxConnections {
		// Try to evict an idle connection
		if !p.evictIdleConnection() {
			return nil, domain.ErrPoolExhausted
		}
	}

	// Create new client
	client, err := p.createClient(ctx, device)
	if err != nil {
		return nil, err
	}

	// Create circuit breaker for this device
	breaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        fmt.Sprintf("s7-%s", device.ID),
		MaxRequests: 3,
		Interval:    10 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 5 && failureRatio >= 0.5
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			p.logger.Info().
				Str("device", name).
				Str("from", from.String()).
				Str("to", to.String()).
				Msg("S7 circuit breaker state changed")
		},
	})

	p.clients[device.ID] = &clientEntry{
		client:  client,
		device:  device,
		breaker: breaker,
		lastUse: time.Now(),
	}

	return client, nil
}

// createClient creates and connects a new S7 client.
func (p *Pool) createClient(ctx context.Context, device *domain.Device) (*Client, error) {
	config := ClientConfig{
		Address:     device.Connection.Host,
		Port:        device.Connection.Port,
		Rack:        device.Connection.S7Rack,
		Slot:        device.Connection.S7Slot,
		Timeout:     device.Connection.Timeout,
		IdleTimeout: p.idleTimeout,
		MaxRetries:  device.Connection.RetryCount,
		RetryDelay:  device.Connection.RetryDelay,
		PDUSize:     device.Connection.S7PDUSize,
	}

	// Apply defaults
	if config.Port == 0 {
		config.Port = 102
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	client, err := NewClient(device.ID, config, p.logger)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	err = client.Connect(ctx)
	if p.metrics != nil {
		p.metrics.RecordConnectionForProtocol(string(domain.ProtocolS7), err == nil, time.Since(start).Seconds())
	}
	if err != nil {
		return nil, err
	}

	return client, nil
}

// ReadTag reads a tag with circuit breaker protection.
func (p *Pool) ReadTag(ctx context.Context, device *domain.Device, tag *domain.Tag) (*domain.DataPoint, error) {
	p.mu.RLock()
	entry, exists := p.clients[device.ID]
	p.mu.RUnlock()

	if !exists {
		client, err := p.GetOrCreate(ctx, device)
		if err != nil {
			return nil, err
		}
		p.mu.RLock()
		entry = p.clients[device.ID]
		p.mu.RUnlock()
		_ = client // Used indirectly via entry
	}

	// Execute read through circuit breaker
	result, err := entry.breaker.Execute(func() (interface{}, error) {
		return entry.client.ReadTag(ctx, tag)
	})

	if err != nil {
		if err == gobreaker.ErrOpenState {
			return nil, domain.ErrCircuitBreakerOpen
		}
		return nil, err
	}

	return result.(*domain.DataPoint), nil
}

// ReadTags reads multiple tags with circuit breaker protection.
func (p *Pool) ReadTags(ctx context.Context, device *domain.Device, tags []*domain.Tag) ([]*domain.DataPoint, error) {
	p.mu.RLock()
	entry, exists := p.clients[device.ID]
	p.mu.RUnlock()

	if !exists {
		client, err := p.GetOrCreate(ctx, device)
		if err != nil {
			return nil, err
		}
		p.mu.RLock()
		entry = p.clients[device.ID]
		p.mu.RUnlock()
		_ = client
	}

	// Execute read through circuit breaker
	result, err := entry.breaker.Execute(func() (interface{}, error) {
		return entry.client.ReadTags(ctx, tags)
	})

	if err != nil {
		if err == gobreaker.ErrOpenState {
			return nil, domain.ErrCircuitBreakerOpen
		}
		return nil, err
	}

	return result.([]*domain.DataPoint), nil
}

// WriteTag writes to a tag with circuit breaker protection.
func (p *Pool) WriteTag(ctx context.Context, device *domain.Device, tag *domain.Tag, value interface{}) error {
	p.mu.RLock()
	entry, exists := p.clients[device.ID]
	p.mu.RUnlock()

	if !exists {
		client, err := p.GetOrCreate(ctx, device)
		if err != nil {
			return err
		}
		p.mu.RLock()
		entry = p.clients[device.ID]
		p.mu.RUnlock()
		_ = client
	}

	// Execute write through circuit breaker
	_, err := entry.breaker.Execute(func() (interface{}, error) {
		return nil, entry.client.WriteTag(ctx, tag, value)
	})

	if err != nil {
		if err == gobreaker.ErrOpenState {
			return domain.ErrCircuitBreakerOpen
		}
		return err
	}

	return nil
}

// WriteTags writes multiple values with circuit breaker protection.
func (p *Pool) WriteTags(ctx context.Context, device *domain.Device, writes []TagWrite) []error {
	p.mu.RLock()
	entry, exists := p.clients[device.ID]
	p.mu.RUnlock()

	if !exists {
		_, err := p.GetOrCreate(ctx, device)
		if err != nil {
			errors := make([]error, len(writes))
			for i := range errors {
				errors[i] = err
			}
			return errors
		}
		p.mu.RLock()
		entry = p.clients[device.ID]
		p.mu.RUnlock()
	}

	errors := make([]error, len(writes))
	for i, write := range writes {
		_, err := entry.breaker.Execute(func() (interface{}, error) {
			return nil, entry.client.WriteTag(ctx, write.Tag, write.Value)
		})
		if err != nil {
			if err == gobreaker.ErrOpenState {
				errors[i] = domain.ErrCircuitBreakerOpen
			} else {
				errors[i] = err
			}
		}
	}

	return errors
}

// TagWrite represents a single write operation.
type TagWrite struct {
	Tag   *domain.Tag
	Value interface{}
}

// Remove removes a client from the pool and closes its connection.
func (p *Pool) Remove(deviceID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, exists := p.clients[deviceID]
	if !exists {
		return nil
	}

	delete(p.clients, deviceID)
	return entry.client.Disconnect()
}

// Close closes all connections and stops the pool.
func (p *Pool) Close() error {
	close(p.stopChan)
	p.wg.Wait()

	p.mu.Lock()
	p.closed = true
	defer p.mu.Unlock()

	// Collect client IDs first to avoid deleting from map during iteration
	clientIDs := make([]string, 0, len(p.clients))
	for id := range p.clients {
		clientIDs = append(clientIDs, id)
	}

	var lastErr error
	for _, id := range clientIDs {
		entry := p.clients[id]
		if err := entry.client.Disconnect(); err != nil {
			p.logger.Error().Err(err).Str("device", id).Msg("Error closing S7 connection")
			lastErr = err
		}
		delete(p.clients, id)
	}

	return lastErr
}

// evictIdleConnection removes the oldest idle connection.
func (p *Pool) evictIdleConnection() bool {
	var oldestID string
	var oldestTime time.Time

	for id, entry := range p.clients {
		if oldestID == "" || entry.lastUse.Before(oldestTime) {
			oldestID = id
			oldestTime = entry.lastUse
		}
	}

	if oldestID == "" {
		return false
	}

	if time.Since(oldestTime) < p.idleTimeout {
		return false
	}

	entry := p.clients[oldestID]
	delete(p.clients, oldestID)
	entry.client.Disconnect()

	p.logger.Debug().Str("device", oldestID).Msg("Evicted idle S7 connection")
	return true
}

// healthCheckLoop periodically checks connection health and attempts reconnections.
func (p *Pool) healthCheckLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.healthCheck)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopChan:
			return
		case <-ticker.C:
			p.checkConnectionsAndReconnect()
			p.publishActiveConnectionMetrics()
		}
	}
}

// idleReaperLoop removes idle connections that haven't been used.
func (p *Pool) idleReaperLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.idleTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopChan:
			return
		case <-ticker.C:
			p.reapIdleConnections()
		}
	}
}

func (p *Pool) publishActiveConnectionMetrics() {
	if p.metrics == nil {
		return
	}

	active := 0
	p.mu.RLock()
	for _, entry := range p.clients {
		if entry.client.IsConnected() {
			active++
		}
	}
	p.mu.RUnlock()

	p.metrics.UpdateActiveConnectionsForProtocol(string(domain.ProtocolS7), active)
}

// checkConnectionsAndReconnect checks all connections and attempts to reconnect disconnected ones.
func (p *Pool) checkConnectionsAndReconnect() {
	// First pass: identify disconnected clients that need reconnection
	p.mu.RLock()
	type reconnectCandidate struct {
		id    string
		entry *clientEntry
	}
	candidates := make([]reconnectCandidate, 0)
	for id, entry := range p.clients {
		if !entry.client.IsConnected() {
			candidates = append(candidates, reconnectCandidate{id: id, entry: entry})
		}
	}
	p.mu.RUnlock()

	// Second pass: attempt reconnection for each candidate
	now := time.Now()
	for _, c := range candidates {
		c.entry.mu.Lock()

		// Skip if circuit breaker is open
		if c.entry.breaker.State() == gobreaker.StateOpen {
			c.entry.mu.Unlock()
			continue
		}

		// Check if we should attempt reconnection (with backoff)
		if !c.entry.canAttemptReconnect(now) {
			c.entry.mu.Unlock()
			continue
		}

		p.logger.Debug().Str("device", c.id).Msg("Attempting to reconnect S7 client")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		start := time.Now()
		err := c.entry.client.Connect(ctx)
		if p.metrics != nil {
			p.metrics.RecordConnectionForProtocol(string(domain.ProtocolS7), err == nil, time.Since(start).Seconds())
		}
		cancel()

		c.entry.recordConnectResult(now, err, 5*time.Second)

		if err != nil {
			p.logger.Warn().Err(err).Str("device", c.id).Msg("Failed to reconnect S7 client")
		} else {
			p.logger.Info().Str("device", c.id).Msg("Successfully reconnected S7 client")
		}

		c.entry.mu.Unlock()
	}
}

// reapIdleConnections removes connections that have been idle too long.
func (p *Pool) reapIdleConnections() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	now := time.Now()
	toRemove := make([]string, 0)

	for id, entry := range p.clients {
		if now.Sub(entry.lastUse) > p.idleTimeout {
			toRemove = append(toRemove, id)
		}
	}

	for _, id := range toRemove {
		entry := p.clients[id]
		delete(p.clients, id)
		entry.client.Disconnect()
		p.logger.Debug().Str("device", id).Msg("Reaped idle S7 connection")
	}

	if len(toRemove) > 0 {
		p.logger.Debug().
			Int("removed", len(toRemove)).
			Int("remaining", len(p.clients)).
			Msg("S7 idle reaper complete")
	}
}

// =============================================================================
// Client Entry Methods
// =============================================================================

func (ce *clientEntry) canAttemptReconnect(now time.Time) bool {
	return ce.nextReconnectAt.IsZero() || !now.Before(ce.nextReconnectAt)
}

func (ce *clientEntry) recordConnectResult(now time.Time, err error, baseDelay time.Duration) {
	if err == nil {
		ce.lastError = nil
		ce.connectFailures = 0
		ce.nextReconnectAt = time.Time{}
		return
	}

	ce.lastError = err
	ce.connectFailures++

	// Calculate exponential backoff
	shift := ce.connectFailures - 1
	if shift > 6 {
		shift = 6
	}

	delay := baseDelay * time.Duration(1<<uint(shift))
	maxDelay := 5 * time.Minute
	if delay > maxDelay {
		delay = maxDelay
	}

	ce.nextReconnectAt = now.Add(delay)
}
