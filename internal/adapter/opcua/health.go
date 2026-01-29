// Package opcua provides health and statistics for OPC UA connection pool.
package opcua

import (
	"context"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/sony/gobreaker"
)

// =============================================================================
// Statistics Types
// =============================================================================

// PoolStats contains pool statistics.
type PoolStats struct {
	TotalSessions     int   // Number of endpoint sessions
	ActiveSessions    int   // Sessions with active connections
	TotalDevices      int   // Total devices bound to sessions
	MaxSessions       int   // Max allowed sessions
	GlobalInFlight    int64 // Current in-flight operations
	MaxGlobalInFlight int64 // Max allowed in-flight operations
	BrownoutMode      bool  // Whether brownout mode is active
}

// DeviceHealth contains health information for a single device.
// Includes both endpoint-level and device-level circuit breaker states.
type DeviceHealth struct {
	DeviceID            string
	EndpointKey         string
	Connected           bool
	EndpointBreakerOpen bool // Endpoint-level breaker (connection errors)
	DeviceBreakerOpen   bool // Device-level breaker (config errors)
	CircuitBreakerOpen  bool // Deprecated: use EndpointBreakerOpen
	LastError           error
	SessionState        SessionState
	DevicesOnSession    int // How many devices share this session
}

// SessionHealth contains health information for an endpoint session.
type SessionHealth struct {
	EndpointKey        string
	EndpointURL        string
	Connected          bool
	CircuitBreakerOpen bool
	LastError          error
	SessionState       SessionState
	DeviceCount        int      // Number of devices using this session
	DeviceIDs          []string // IDs of devices using this session
}

// =============================================================================
// Health Check Methods
// =============================================================================

// HealthCheck implements the health.Checker interface.
// With per-endpoint circuit breakers, the pool is considered healthy
// as long as the pool itself is operational.
func (p *ConnectionPool) HealthCheck(ctx context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return domain.ErrServiceStopped
	}

	return nil
}

// Stats returns pool statistics.
func (p *ConnectionPool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := PoolStats{
		TotalSessions:     len(p.sessions),
		TotalDevices:      len(p.devices),
		MaxSessions:       p.config.MaxConnections,
		GlobalInFlight:    p.globalInFlight.Load(),
		MaxGlobalInFlight: p.maxGlobalInFlight,
		BrownoutMode:      p.brownoutMode.Load(),
	}

	for _, session := range p.sessions {
		session.mu.Lock()
		if session.client.IsConnected() {
			stats.ActiveSessions++
		}
		session.mu.Unlock()
	}

	return stats
}

// GetDeviceHealth returns health information for a specific device.
// Includes both endpoint and device circuit breaker states.
func (p *ConnectionPool) GetDeviceHealth(deviceID string) (DeviceHealth, bool) {
	p.mu.RLock()
	binding, exists := p.devices[deviceID]
	if !exists {
		p.mu.RUnlock()
		return DeviceHealth{}, false
	}

	session, sessionExists := p.sessions[binding.EndpointKey]
	p.mu.RUnlock()

	if !sessionExists {
		return DeviceHealth{}, false
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	endpointBreakerOpen := session.breaker.State() == gobreaker.StateOpen
	deviceBreakerOpen := binding.breaker != nil && binding.breaker.State() == gobreaker.StateOpen

	return DeviceHealth{
		DeviceID:            deviceID,
		EndpointKey:         binding.EndpointKey[:min(len(binding.EndpointKey), 50)],
		Connected:           session.client.IsConnected(),
		EndpointBreakerOpen: endpointBreakerOpen,
		DeviceBreakerOpen:   deviceBreakerOpen,
		CircuitBreakerOpen:  endpointBreakerOpen || deviceBreakerOpen, // Backward compat
		LastError:           session.lastError,
		SessionState:        session.client.GetSessionState(),
		DevicesOnSession:    len(session.devices),
	}, true
}

// GetSessionHealth returns health information for a specific endpoint session.
func (p *ConnectionPool) GetSessionHealth(endpointKey string) (SessionHealth, bool) {
	p.mu.RLock()
	session, exists := p.sessions[endpointKey]
	p.mu.RUnlock()

	if !exists {
		return SessionHealth{}, false
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	deviceIDs := make([]string, 0, len(session.devices))
	for deviceID := range session.devices {
		deviceIDs = append(deviceIDs, deviceID)
	}

	return SessionHealth{
		EndpointKey:        endpointKey,
		EndpointURL:        session.endpointURL,
		Connected:          session.client.IsConnected(),
		CircuitBreakerOpen: session.breaker.State() == gobreaker.StateOpen,
		LastError:          session.lastError,
		SessionState:       session.client.GetSessionState(),
		DeviceCount:        len(session.devices),
		DeviceIDs:          deviceIDs,
	}, true
}

// GetAllSessionsHealth returns health information for all sessions.
func (p *ConnectionPool) GetAllSessionsHealth() []SessionHealth {
	p.mu.RLock()
	defer p.mu.RUnlock()

	healths := make([]SessionHealth, 0, len(p.sessions))
	for epKey, session := range p.sessions {
		session.mu.Lock()

		deviceIDs := make([]string, 0, len(session.devices))
		for deviceID := range session.devices {
			deviceIDs = append(deviceIDs, deviceID)
		}

		healths = append(healths, SessionHealth{
			EndpointKey:        epKey,
			EndpointURL:        session.endpointURL,
			Connected:          session.client.IsConnected(),
			CircuitBreakerOpen: session.breaker.State() == gobreaker.StateOpen,
			LastError:          session.lastError,
			SessionState:       session.client.GetSessionState(),
			DeviceCount:        len(session.devices),
			DeviceIDs:          deviceIDs,
		})

		session.mu.Unlock()
	}

	return healths
}

// =============================================================================
// Background Health Loops
// =============================================================================

// healthCheckLoop periodically checks the health of all sessions.
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

			// Copy endpoint keys to avoid holding lock during health checks
			endpointKeys := make([]string, 0, len(p.sessions))
			for epKey := range p.sessions {
				endpointKeys = append(endpointKeys, epKey)
			}
			p.mu.RUnlock()

			for _, epKey := range endpointKeys {
				p.checkSessionHealth(epKey)
			}

			p.publishActiveConnectionMetrics()
			p.checkGlobalHealth()
		}
	}
}

// checkSessionHealth checks and potentially reconnects a session.
func (p *ConnectionPool) checkSessionHealth(endpointKey string) {
	p.mu.RLock()
	session, exists := p.sessions[endpointKey]
	p.mu.RUnlock()

	if !exists {
		return
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// If the breaker is open, avoid background reconnect attempts
	if session.breaker.State() == gobreaker.StateOpen {
		return
	}

	if !session.client.IsConnected() {
		now := time.Now()
		if !session.canAttemptReconnect(now) {
			return
		}
		p.logger.Debug().Str("endpoint", endpointKey[:min(len(endpointKey), 50)]).Msg("Session disconnected, attempting reconnect")

		ctx, cancel := context.WithTimeout(context.Background(), p.config.ConnectionTimeout)
		defer cancel()

		start := time.Now()
		err := session.client.Connect(ctx)
		if p.metrics != nil {
			p.metrics.RecordConnectionForProtocol(string(domain.ProtocolOPCUA), err == nil, time.Since(start).Seconds())
		}
		session.recordConnectResult(now, err, p.config.RetryDelay)
		if err != nil {
			p.logger.Warn().Err(err).Str("endpoint", endpointKey[:min(len(endpointKey), 50)]).Msg("Failed to reconnect session")
		} else {
			p.logger.Info().
				Str("endpoint", endpointKey[:min(len(endpointKey), 50)]).
				Int("devices", len(session.devices)).
				Msg("Session reconnected")

			// Trigger subscription recovery
			if session.subscriptionState != nil {
				go p.recoverSubscriptions(session)
			}
		}
	}
}

func (p *ConnectionPool) publishActiveConnectionMetrics() {
	if p.metrics == nil {
		return
	}

	// Copy session pointers under pool lock, then iterate without pool lock
	// This enforces correct lock ordering: pool.mu -> session.mu (never reverse)
	p.mu.RLock()
	sessions := make([]*pooledSession, 0, len(p.sessions))
	for _, session := range p.sessions {
		sessions = append(sessions, session)
	}
	p.mu.RUnlock()

	active := 0
	for _, session := range sessions {
		session.mu.Lock()
		if session.client.IsConnected() {
			active++
		}
		session.mu.Unlock()
	}

	p.metrics.UpdateActiveConnectionsForProtocol(string(domain.ProtocolOPCUA), active)
}

// idleReaperLoop removes idle sessions that haven't been used.
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

			p.reapIdleSessions()
		}
	}
}

// reapIdleSessions closes sessions that have been idle too long.
// IMPORTANT: Does NOT reap sessions with active subscriptions.
func (p *ConnectionPool) reapIdleSessions() {
	// First pass: identify sessions to reap (under pool.mu read lock)
	p.mu.RLock()
	type reapCandidate struct {
		epKey   string
		session *pooledSession
	}
	candidates := make([]reapCandidate, 0)
	for epKey, session := range p.sessions {
		candidates = append(candidates, reapCandidate{epKey: epKey, session: session})
	}
	p.mu.RUnlock()

	// Second pass: check each candidate (lock session.mu individually)
	toReap := make([]string, 0)
	for _, c := range candidates {
		c.session.mu.Lock()
		// Use hasRecentActivity which checks BOTH client.LastUsed AND subscription activity
		if !c.session.hasRecentActivity(p.config.IdleTimeout) {
			toReap = append(toReap, c.epKey)
		}
		c.session.mu.Unlock()
	}

	if len(toReap) == 0 {
		return
	}

	// Third pass: actually reap (under pool.mu write lock)
	// This enforces lock ordering: pool.mu -> session.mu
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, epKey := range toReap {
		session, exists := p.sessions[epKey]
		if !exists {
			continue
		}

		session.mu.Lock()
		// Re-check under lock in case activity happened
		if !session.hasRecentActivity(p.config.IdleTimeout) {
			p.logger.Debug().
				Str("endpoint", epKey[:min(len(epKey), 50)]).
				Bool("had_subscriptions", session.hasActiveSubscriptions).
				Msg("Closing idle session")
			session.client.Disconnect()

			// Remove all device bindings for this session
			for deviceID := range session.devices {
				delete(p.devices, deviceID)
			}
			delete(p.sessions, epKey)
		}
		session.mu.Unlock()
	}
}
