// Package mocks provides mock implementations for testing.
package mocks

import (
	"sync"
)

// HealthStatus represents the health status of a component.
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusDegraded  HealthStatus = "degraded"
)

// ComponentHealth represents the health of a single component.
type ComponentHealth struct {
	Name    string
	Status  HealthStatus
	Message string
}

// MockHealthChecker is a mock implementation of the health checker.
type MockHealthChecker struct {
	mu sync.RWMutex

	// Component health status
	components map[string]*ComponentHealth

	// Function overrides
	CheckFunc func() map[string]*ComponentHealth
}

// NewMockHealthChecker creates a new mock health checker.
func NewMockHealthChecker() *MockHealthChecker {
	return &MockHealthChecker{
		components: make(map[string]*ComponentHealth),
	}
}

// SetComponentHealth sets the health status of a component.
func (m *MockHealthChecker) SetComponentHealth(name string, status HealthStatus, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.components[name] = &ComponentHealth{
		Name:    name,
		Status:  status,
		Message: message,
	}
}

// SetHealthy marks a component as healthy.
func (m *MockHealthChecker) SetHealthy(name string) {
	m.SetComponentHealth(name, HealthStatusHealthy, "")
}

// SetUnhealthy marks a component as unhealthy.
func (m *MockHealthChecker) SetUnhealthy(name string, message string) {
	m.SetComponentHealth(name, HealthStatusUnhealthy, message)
}

// SetDegraded marks a component as degraded.
func (m *MockHealthChecker) SetDegraded(name string, message string) {
	m.SetComponentHealth(name, HealthStatusDegraded, message)
}

// Check returns the current health status of all components.
func (m *MockHealthChecker) Check() map[string]*ComponentHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.CheckFunc != nil {
		return m.CheckFunc()
	}

	result := make(map[string]*ComponentHealth)
	for k, v := range m.components {
		result[k] = &ComponentHealth{
			Name:    v.Name,
			Status:  v.Status,
			Message: v.Message,
		}
	}
	return result
}

// IsHealthy returns true if all components are healthy.
func (m *MockHealthChecker) IsHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, c := range m.components {
		if c.Status != HealthStatusHealthy {
			return false
		}
	}
	return true
}

// Reset clears all component health status.
func (m *MockHealthChecker) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.components = make(map[string]*ComponentHealth)
}
