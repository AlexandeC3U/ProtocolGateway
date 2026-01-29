// Package health provides health check functionality for the service.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// OperationalState represents the operational state of the service.
type OperationalState string

const (
	StateStarting     OperationalState = "starting"
	StateRunning      OperationalState = "running"
	StateDegraded     OperationalState = "degraded"
	StateRecovering   OperationalState = "recovering"
	StateShuttingDown OperationalState = "shutting_down"
	StateOffline      OperationalState = "offline"
)

// CheckSeverity indicates how a check failure affects overall health.
type CheckSeverity int

const (
	// SeverityInfo - failure is informational only, doesn't affect health status.
	SeverityInfo CheckSeverity = iota
	// SeverityWarning - failure marks system as degraded but still operational.
	SeverityWarning
	// SeverityCritical - failure marks system as unhealthy, fails readiness.
	SeverityCritical
)

func (s CheckSeverity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// Checker interface defines a component that can be health checked.
type Checker interface {
	HealthCheck(ctx context.Context) error
}

// CheckRegistration holds a checker and its configuration.
type CheckRegistration struct {
	Checker  Checker
	Severity CheckSeverity
}

// HealthChecker manages health checks for the service.
type HealthChecker struct {
	config Config
	checks map[string]*CheckRegistration
	mu     sync.RWMutex

	// Cached check results (updated by background loop)
	statuses     map[string]*CheckStatus
	statusesMu   sync.RWMutex
	lastCheckAt  time.Time
	cachedResult *HealthResponse

	// Operational state
	state   OperationalState
	stateMu sync.RWMutex

	// Flapping protection: track consecutive failures per check
	failureCounts   map[string]int
	failureCountsMu sync.Mutex

	// Background scheduling
	stopChan chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
	started  bool
}

// Config holds health checker configuration.
type Config struct {
	ServiceName    string
	ServiceVersion string
	CheckTimeout   time.Duration

	// CheckInterval is how often background checks run (default: 10s).
	CheckInterval time.Duration

	// FailureThreshold is how many consecutive failures before marking unhealthy (default: 3).
	FailureThreshold int

	// RecoveryThreshold is how many consecutive successes before marking recovered (default: 2).
	RecoveryThreshold int
}

// CheckStatus represents the status of a single health check.
type CheckStatus struct {
	Name             string    `json:"name"`
	Status           string    `json:"status"` // "healthy", "unhealthy", "degraded", "unknown"
	Severity         string    `json:"severity"`
	Error            string    `json:"error,omitempty"`
	LastCheck        time.Time `json:"last_check"`
	ConsecutiveFails int       `json:"consecutive_fails,omitempty"`
	LastStateChange  time.Time `json:"last_state_change,omitempty"`
}

// HealthResponse represents the full health response.
type HealthResponse struct {
	Status    string                  `json:"status"` // "healthy", "degraded", "unhealthy"
	State     OperationalState        `json:"state"`  // operational state
	Service   string                  `json:"service"`
	Version   string                  `json:"version"`
	Timestamp time.Time               `json:"timestamp"`
	Checks    map[string]*CheckStatus `json:"checks,omitempty"`
	Uptime    string                  `json:"uptime,omitempty"`
}

// NewChecker creates a new health checker.
func NewChecker(config Config) *HealthChecker {
	if config.CheckTimeout == 0 {
		config.CheckTimeout = 5 * time.Second
	}
	if config.CheckInterval == 0 {
		config.CheckInterval = 10 * time.Second
	}
	if config.FailureThreshold == 0 {
		config.FailureThreshold = 3
	}
	if config.RecoveryThreshold == 0 {
		config.RecoveryThreshold = 2
	}

	return &HealthChecker{
		config:        config,
		checks:        make(map[string]*CheckRegistration),
		statuses:      make(map[string]*CheckStatus),
		failureCounts: make(map[string]int),
		state:         StateStarting,
		stopChan:      make(chan struct{}),
	}
}

// Start begins the background health check loop.
func (h *HealthChecker) Start() {
	h.mu.Lock()
	if h.started {
		h.mu.Unlock()
		return
	}
	h.started = true
	h.mu.Unlock()

	h.wg.Add(1)
	go h.checkLoop()
}

// Stop stops the background health check loop.
// Safe to call multiple times.
func (h *HealthChecker) Stop() {
	h.mu.Lock()
	if !h.started {
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()

	h.stopOnce.Do(func() {
		h.SetState(StateShuttingDown)
		close(h.stopChan)
	})

	h.wg.Wait()
	h.SetState(StateOffline)
}

// checkLoop runs health checks on a schedule.
func (h *HealthChecker) checkLoop() {
	defer h.wg.Done()

	// Run initial check immediately
	h.runChecks()

	ticker := time.NewTicker(h.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopChan:
			return
		case <-ticker.C:
			h.runChecks()
		}
	}
}

// runChecks performs all health checks and updates cached results.
func (h *HealthChecker) runChecks() {
	ctx, cancel := context.WithTimeout(context.Background(), h.config.CheckTimeout*2)
	defer cancel()

	h.mu.RLock()
	checks := make(map[string]*CheckRegistration, len(h.checks))
	for name, reg := range h.checks {
		checks[name] = reg
	}
	h.mu.RUnlock()

	response := &HealthResponse{
		Status:    "healthy",
		State:     h.GetState(),
		Service:   h.config.ServiceName,
		Version:   h.config.ServiceVersion,
		Timestamp: time.Now(),
		Checks:    make(map[string]*CheckStatus),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	hasCriticalFailure := false
	hasWarningFailure := false

	for name, reg := range checks {
		wg.Add(1)
		go func(name string, reg *CheckRegistration) {
			defer wg.Done()

			checkCtx, checkCancel := context.WithTimeout(ctx, h.config.CheckTimeout)
			defer checkCancel()

			now := time.Now()
			err := reg.Checker.HealthCheck(checkCtx)

			status := h.updateCheckStatus(name, reg.Severity, err, now)

			mu.Lock()
			response.Checks[name] = status

			// Determine overall status based on severity
			if status.Status == "unhealthy" {
				switch reg.Severity {
				case SeverityCritical:
					hasCriticalFailure = true
				case SeverityWarning:
					hasWarningFailure = true
					// SeverityInfo doesn't affect overall status
				}
			}
			mu.Unlock()
		}(name, reg)
	}

	wg.Wait()

	// Set overall status based on failures
	if hasCriticalFailure {
		response.Status = "unhealthy"
	} else if hasWarningFailure {
		response.Status = "degraded"
	}

	// Update operational state based on health
	h.updateOperationalState(response.Status)
	response.State = h.GetState()

	// Cache the result
	h.statusesMu.Lock()
	h.cachedResult = response
	h.lastCheckAt = time.Now()
	h.statusesMu.Unlock()
}

// updateCheckStatus updates status with flapping protection.
func (h *HealthChecker) updateCheckStatus(name string, severity CheckSeverity, err error, checkTime time.Time) *CheckStatus {
	h.failureCountsMu.Lock()
	defer h.failureCountsMu.Unlock()

	h.statusesMu.Lock()
	oldStatus := h.statuses[name]
	h.statusesMu.Unlock()

	status := &CheckStatus{
		Name:      name,
		Severity:  severity.String(),
		LastCheck: checkTime,
	}

	if oldStatus != nil {
		status.LastStateChange = oldStatus.LastStateChange
		status.ConsecutiveFails = oldStatus.ConsecutiveFails
	}

	if err != nil {
		// Increment failure count
		h.failureCounts[name]++
		status.ConsecutiveFails = h.failureCounts[name]
		status.Error = err.Error()

		// Only mark unhealthy after threshold consecutive failures (flapping protection)
		if h.failureCounts[name] >= h.config.FailureThreshold {
			status.Status = "unhealthy"
			if oldStatus == nil || oldStatus.Status != "unhealthy" {
				status.LastStateChange = checkTime
			}
		} else {
			// Below threshold: keep previous status or mark as degraded
			if oldStatus != nil && oldStatus.Status == "healthy" {
				status.Status = "healthy" // Still within grace period
			} else {
				status.Status = "degraded" // Transitioning
			}
		}
	} else {
		// Success - check recovery threshold
		previousFails := h.failureCounts[name]
		h.failureCounts[name] = 0
		status.ConsecutiveFails = 0

		if previousFails >= h.config.FailureThreshold {
			// Was unhealthy, now recovering
			status.Status = "healthy"
			status.LastStateChange = checkTime
		} else {
			status.Status = "healthy"
			if oldStatus == nil || oldStatus.Status != "healthy" {
				status.LastStateChange = checkTime
			}
		}
	}

	// Update stored status
	h.statusesMu.Lock()
	h.statuses[name] = status
	h.statusesMu.Unlock()

	return status
}

// updateOperationalState transitions state based on health status.
func (h *HealthChecker) updateOperationalState(healthStatus string) {
	h.stateMu.Lock()
	defer h.stateMu.Unlock()

	currentState := h.state

	switch currentState {
	case StateStarting:
		// Transition out of starting once we've run checks
		if healthStatus == "healthy" {
			h.state = StateRunning
		} else if healthStatus == "degraded" {
			h.state = StateDegraded
		} else {
			h.state = StateDegraded // Don't go straight to offline from starting
		}

	case StateRunning:
		if healthStatus == "degraded" {
			h.state = StateDegraded
		} else if healthStatus == "unhealthy" {
			h.state = StateDegraded // Transition through degraded first
		}

	case StateDegraded:
		if healthStatus == "healthy" {
			h.state = StateRecovering
		}
		// Stay degraded if unhealthy; only external shutdown moves to offline

	case StateRecovering:
		if healthStatus == "healthy" {
			h.state = StateRunning
		} else {
			h.state = StateDegraded
		}

	case StateShuttingDown, StateOffline:
		// Terminal states, don't change
	}
}

// SetState manually sets the operational state.
func (h *HealthChecker) SetState(state OperationalState) {
	h.stateMu.Lock()
	defer h.stateMu.Unlock()
	h.state = state
}

// GetState returns the current operational state.
func (h *HealthChecker) GetState() OperationalState {
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	return h.state
}

// AddCheck registers a health check with default CRITICAL severity.
func (h *HealthChecker) AddCheck(name string, checker Checker) {
	h.AddCheckWithSeverity(name, checker, SeverityCritical)
}

// AddCheckWithSeverity registers a health check with specified severity.
func (h *HealthChecker) AddCheckWithSeverity(name string, checker Checker, severity CheckSeverity) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks[name] = &CheckRegistration{
		Checker:  checker,
		Severity: severity,
	}

	h.statusesMu.Lock()
	h.statuses[name] = &CheckStatus{
		Name:     name,
		Status:   "unknown",
		Severity: severity.String(),
	}
	h.statusesMu.Unlock()
}

// RemoveCheck removes a health check.
func (h *HealthChecker) RemoveCheck(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.checks, name)

	h.statusesMu.Lock()
	delete(h.statuses, name)
	h.statusesMu.Unlock()

	h.failureCountsMu.Lock()
	delete(h.failureCounts, name)
	h.failureCountsMu.Unlock()
}

// Check returns the cached health response (does NOT run checks on-demand).
// Use RunChecksNow() if you need fresh results.
func (h *HealthChecker) Check(ctx context.Context) *HealthResponse {
	h.statusesMu.RLock()
	cached := h.cachedResult
	h.statusesMu.RUnlock()

	if cached != nil {
		return cached
	}

	// No cached result yet; return a starting response
	return &HealthResponse{
		Status:    "unknown",
		State:     h.GetState(),
		Service:   h.config.ServiceName,
		Version:   h.config.ServiceVersion,
		Timestamp: time.Now(),
		Checks:    make(map[string]*CheckStatus),
	}
}

// RunChecksNow forces an immediate check run (for testing or manual refresh).
func (h *HealthChecker) RunChecksNow() *HealthResponse {
	h.runChecks()
	return h.Check(context.Background())
}

// HealthHandler handles HTTP health check requests.
// Returns cached results from background checks.
func (h *HealthChecker) HealthHandler(w http.ResponseWriter, r *http.Request) {
	response := h.Check(r.Context())

	w.Header().Set("Content-Type", "application/json")

	statusCode := http.StatusOK
	if response.Status == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}

	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(response)
}

// LivenessHandler handles Kubernetes liveness probe.
// Returns 200 if the service process is running (not deadlocked).
// Does NOT check dependencies - that's readiness.
func (h *HealthChecker) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	state := h.GetState()

	response := &HealthResponse{
		Status:    "healthy",
		State:     state,
		Service:   h.config.ServiceName,
		Version:   h.config.ServiceVersion,
		Timestamp: time.Now(),
	}

	// Liveness fails only if we're offline or shutting down
	statusCode := http.StatusOK
	if state == StateOffline || state == StateShuttingDown {
		response.Status = "unhealthy"
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(response)
}

// ReadinessHandler handles Kubernetes readiness probe.
// Returns 200 only if all CRITICAL checks pass.
// Degraded (WARNING failures only) still returns 200 - traffic can be served.
func (h *HealthChecker) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	response := h.Check(r.Context())

	w.Header().Set("Content-Type", "application/json")

	// Ready if healthy OR degraded (only critical failures block readiness)
	statusCode := http.StatusOK
	if response.Status == "unhealthy" || response.State == StateStarting {
		statusCode = http.StatusServiceUnavailable
	}

	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(response)
}

// GetStatus returns the current cached status of a check.
func (h *HealthChecker) GetStatus(name string) *CheckStatus {
	h.statusesMu.RLock()
	defer h.statusesMu.RUnlock()
	return h.statuses[name]
}

// IsHealthy returns true if all critical checks are healthy.
func (h *HealthChecker) IsHealthy(ctx context.Context) bool {
	response := h.Check(ctx)
	return response.Status == "healthy"
}

// IsReady returns true if the service can accept traffic (healthy or degraded).
func (h *HealthChecker) IsReady(ctx context.Context) bool {
	response := h.Check(ctx)
	return response.Status != "unhealthy" && h.GetState() != StateStarting
}
