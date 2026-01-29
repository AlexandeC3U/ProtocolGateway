// Package opcua provides fleet-wide load shaping for OPC UA operations.
package opcua

import (
	"context"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/sony/gobreaker"
)

// =============================================================================
// Load Shaping Types
// =============================================================================

// opRequest represents a queued operation for priority scheduling.
type opRequest struct {
	ctx      context.Context
	priority int // 0=telemetry, 1=control, 2=safety
	fn       func() error
	resultCh chan error
	session  *pooledSession // For circuit breaker check before execute
}

// Priority constants for load shaping.
const (
	PriorityTelemetry = 0 // Bulk data, can be delayed
	PriorityControl   = 1 // Commands, higher priority
	PrioritySafety    = 2 // Safety-critical, highest priority
)

// =============================================================================
// Load Shaping Methods
// =============================================================================

// priorityQueueProcessor processes operations in priority order.
// Safety > Control > Telemetry
// Multiple workers run concurrently (fixes single-thread bottleneck).
func (p *ConnectionPool) priorityQueueProcessor() {
	defer p.wg.Done()

	for {
		p.mu.RLock()
		closed := p.closed
		p.mu.RUnlock()
		if closed {
			return
		}

		// Process in priority order: safety (2), control (1), telemetry (0)
		var req *opRequest
		select {
		case req = <-p.priorityQueues[PrioritySafety]:
		default:
			select {
			case req = <-p.priorityQueues[PrioritySafety]:
			case req = <-p.priorityQueues[PriorityControl]:
			default:
				select {
				case req = <-p.priorityQueues[PrioritySafety]:
				case req = <-p.priorityQueues[PriorityControl]:
				case req = <-p.priorityQueues[PriorityTelemetry]:
				case <-time.After(100 * time.Millisecond):
					continue
				}
			}
		}

		if req == nil {
			continue
		}

		// === DEADLINE PROPAGATION ===
		// Critical for control systems: don't execute if context already expired
		// This prevents executing operations after HTTP timeout, PLC command invalid, etc.
		if req.ctx.Err() != nil {
			if req.resultCh != nil {
				req.resultCh <- req.ctx.Err()
			}
			p.globalInFlight.Add(-1)
			// Also decrement endpoint counter if we have a session
			if req.session != nil {
				req.session.inFlight.Add(-1)
			}
			continue
		}

		// Check circuit breaker before executing queued operation
		// This prevents building infinite backlog for dead endpoints
		if req.session != nil && req.session.breaker.State() == gobreaker.StateOpen {
			if req.resultCh != nil {
				req.resultCh <- domain.ErrCircuitBreakerOpen
			}
			p.globalInFlight.Add(-1)
			req.session.inFlight.Add(-1)
			continue
		}

		// Execute the operation
		err := req.fn()
		if req.resultCh != nil {
			req.resultCh <- err
		}
		p.globalInFlight.Add(-1)
		if req.session != nil {
			req.session.inFlight.Add(-1)
		}
	}
}

// checkGlobalLoadAndQueue checks global load and potentially queues the operation.
// Returns error if operation is rejected or queued operation fails.
// IMPORTANT: session parameter is optional but enables circuit breaker checks for queued ops.
func (p *ConnectionPool) checkGlobalLoadAndQueue(ctx context.Context, priority int, fn func() error) error {
	return p.checkGlobalLoadAndQueueWithSession(ctx, priority, nil, fn)
}

// checkGlobalLoadAndQueueWithSession is like checkGlobalLoadAndQueue but includes session for breaker checks.
// Implements three-tier load control: Global limit → Per-endpoint limit → Execute
func (p *ConnectionPool) checkGlobalLoadAndQueueWithSession(ctx context.Context, priority int, session *pooledSession, fn func() error) error {
	// === DEADLINE CHECK EARLY ===
	// Don't even start if context already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check circuit breaker BEFORE queueing - don't build backlog for dead endpoints
	if session != nil && session.breaker.State() == gobreaker.StateOpen {
		return domain.ErrCircuitBreakerOpen
	}

	// Check brownout mode for telemetry
	if p.brownoutMode.Load() && priority == PriorityTelemetry {
		return domain.ErrServiceOverloaded
	}

	// === PER-ENDPOINT FAIRNESS ===
	// Prevents one noisy endpoint from consuming all global capacity
	if session != nil && p.config.MaxInFlightPerEndpoint > 0 {
		endpointCurrent := session.inFlight.Load()
		if endpointCurrent >= int64(p.config.MaxInFlightPerEndpoint) {
			// This endpoint is at capacity - reject telemetry, allow control/safety
			if priority == PriorityTelemetry {
				p.logger.Debug().
					Str("endpoint", session.endpointKey[:min(len(session.endpointKey), 30)]).
					Int64("endpoint_inflight", endpointCurrent).
					Msg("Per-endpoint limit reached, rejecting telemetry")
				return domain.ErrServiceOverloaded
			}
			// Control/Safety still allowed but logged
			p.logger.Warn().
				Str("endpoint", session.endpointKey[:min(len(session.endpointKey), 30)]).
				Int64("endpoint_inflight", endpointCurrent).
				Int("priority", priority).
				Msg("Per-endpoint limit exceeded, allowing high-priority operation")
		}
	}

	// Check global in-flight limit
	current := p.globalInFlight.Load()
	threshold := int64(float64(p.maxGlobalInFlight) * p.brownoutThreshold)

	if current >= threshold && priority == PriorityTelemetry {
		// Enter brownout mode
		if !p.brownoutMode.Load() {
			p.brownoutMode.Store(true)
			p.logger.Warn().
				Int64("current_inflight", current).
				Int64("threshold", threshold).
				Msg("Entering brownout mode - telemetry operations will be rejected")
		}
		return domain.ErrServiceOverloaded
	}

	if current >= p.maxGlobalInFlight {
		// Hard limit reached - queue the operation
		// INCREMENT BEFORE QUEUEING - this fixes the accounting bug
		p.globalInFlight.Add(1)
		if session != nil {
			session.inFlight.Add(1)
		}

		req := &opRequest{
			ctx:      ctx,
			priority: priority,
			fn:       fn,
			resultCh: make(chan error, 1),
			session:  session, // For circuit breaker check in worker
		}

		select {
		case p.priorityQueues[priority] <- req:
			// Wait for result
			select {
			case err := <-req.resultCh:
				return err
			case <-ctx.Done():
				// Context cancelled while waiting - worker will handle decrement
				return ctx.Err()
			}
		default:
			// Queue full - decrement since we can't queue
			p.globalInFlight.Add(-1)
			if session != nil {
				session.inFlight.Add(-1)
			}
			return domain.ErrServiceOverloaded
		}
	}

	// Proceed immediately
	p.globalInFlight.Add(1)
	if session != nil {
		session.inFlight.Add(1)
	}
	defer func() {
		p.globalInFlight.Add(-1)
		if session != nil {
			session.inFlight.Add(-1)
		}
	}()

	// Exit brownout mode if load drops
	if p.brownoutMode.Load() && current < threshold/2 {
		p.brownoutMode.Store(false)
		p.logger.Info().Msg("Exiting brownout mode")
	}

	return fn()
}

// checkGlobalHealth monitors fleet-wide health and adjusts brownout mode.
func (p *ConnectionPool) checkGlobalHealth() {
	current := p.globalInFlight.Load()
	threshold := int64(float64(p.maxGlobalInFlight) * p.brownoutThreshold)

	if current >= threshold && !p.brownoutMode.Load() {
		p.brownoutMode.Store(true)
		p.logger.Warn().
			Int64("current_inflight", current).
			Int64("threshold", threshold).
			Msg("Entering brownout mode due to global load")
	} else if current < threshold/2 && p.brownoutMode.Load() {
		p.brownoutMode.Store(false)
		p.logger.Info().
			Int64("current_inflight", current).
			Msg("Exiting brownout mode")
	}
}

// IsBrownoutMode returns whether the pool is in brownout mode.
func (p *ConnectionPool) IsBrownoutMode() bool {
	return p.brownoutMode.Load()
}

// GetGlobalLoad returns current global load information.
func (p *ConnectionPool) GetGlobalLoad() (current, max int64, brownout bool) {
	return p.globalInFlight.Load(), p.maxGlobalInFlight, p.brownoutMode.Load()
}
