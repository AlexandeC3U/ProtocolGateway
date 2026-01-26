// Package health provides health check functionality for the service.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Checker interface defines a component that can be health checked.
type Checker interface {
	HealthCheck(ctx context.Context) error
}

// HealthChecker manages health checks for the service.
type HealthChecker struct {
	config   Config
	checks   map[string]Checker
	mu       sync.RWMutex
	statuses map[string]*CheckStatus
}

// Config holds health checker configuration.
type Config struct {
	ServiceName    string
	ServiceVersion string
	CheckTimeout   time.Duration
}

// CheckStatus represents the status of a single health check.
type CheckStatus struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"` // "healthy", "unhealthy", "degraded"
	Error     string    `json:"error,omitempty"`
	LastCheck time.Time `json:"last_check"`
}

// HealthResponse represents the full health response.
type HealthResponse struct {
	Status      string                  `json:"status"`
	Service     string                  `json:"service"`
	Version     string                  `json:"version"`
	Timestamp   time.Time               `json:"timestamp"`
	Checks      map[string]*CheckStatus `json:"checks,omitempty"`
	Uptime      string                  `json:"uptime,omitempty"`
}

// NewChecker creates a new health checker.
func NewChecker(config Config) *HealthChecker {
	if config.CheckTimeout == 0 {
		config.CheckTimeout = 5 * time.Second
	}

	return &HealthChecker{
		config:   config,
		checks:   make(map[string]Checker),
		statuses: make(map[string]*CheckStatus),
	}
}

// AddCheck registers a health check.
func (h *HealthChecker) AddCheck(name string, checker Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks[name] = checker
	h.statuses[name] = &CheckStatus{
		Name:   name,
		Status: "unknown",
	}
}

// RemoveCheck removes a health check.
func (h *HealthChecker) RemoveCheck(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.checks, name)
	delete(h.statuses, name)
}

// Check performs all health checks and returns the overall status.
func (h *HealthChecker) Check(ctx context.Context) *HealthResponse {
	h.mu.RLock()
	checks := make(map[string]Checker, len(h.checks))
	for name, checker := range h.checks {
		checks[name] = checker
	}
	h.mu.RUnlock()

	response := &HealthResponse{
		Status:    "healthy",
		Service:   h.config.ServiceName,
		Version:   h.config.ServiceVersion,
		Timestamp: time.Now(),
		Checks:    make(map[string]*CheckStatus),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, checker := range checks {
		wg.Add(1)
		go func(name string, checker Checker) {
			defer wg.Done()

			checkCtx, cancel := context.WithTimeout(ctx, h.config.CheckTimeout)
			defer cancel()

			status := &CheckStatus{
				Name:      name,
				LastCheck: time.Now(),
			}

			if err := checker.HealthCheck(checkCtx); err != nil {
				status.Status = "unhealthy"
				status.Error = err.Error()
			} else {
				status.Status = "healthy"
			}

			mu.Lock()
			response.Checks[name] = status
			if status.Status != "healthy" {
				response.Status = "unhealthy"
			}
			mu.Unlock()
		}(name, checker)
	}

	wg.Wait()

	// Update stored statuses
	h.mu.Lock()
	for name, status := range response.Checks {
		h.statuses[name] = status
	}
	h.mu.Unlock()

	return response
}

// HealthHandler handles HTTP health check requests.
func (h *HealthChecker) HealthHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	response := h.Check(ctx)

	w.Header().Set("Content-Type", "application/json")

	statusCode := http.StatusOK
	if response.Status != "healthy" {
		statusCode = http.StatusServiceUnavailable
	}

	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(response)
}

// LivenessHandler handles Kubernetes liveness probe.
// Returns 200 if the service is running.
func (h *HealthChecker) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	response := &HealthResponse{
		Status:    "healthy",
		Service:   h.config.ServiceName,
		Version:   h.config.ServiceVersion,
		Timestamp: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// ReadinessHandler handles Kubernetes readiness probe.
// Returns 200 if all dependencies are healthy.
func (h *HealthChecker) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	response := h.Check(ctx)

	w.Header().Set("Content-Type", "application/json")

	statusCode := http.StatusOK
	if response.Status != "healthy" {
		statusCode = http.StatusServiceUnavailable
	}

	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(response)
}

// GetStatus returns the current cached status of a check.
func (h *HealthChecker) GetStatus(name string) *CheckStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.statuses[name]
}

// IsHealthy returns true if all checks are healthy.
func (h *HealthChecker) IsHealthy(ctx context.Context) bool {
	response := h.Check(ctx)
	return response.Status == "healthy"
}

