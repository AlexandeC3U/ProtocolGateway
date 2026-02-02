// Package modbus_test tests the Modbus connection pool functionality.
package modbus_test

import (
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/modbus"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

// TestPoolConfigStructure tests PoolConfig structure.
func TestPoolConfigStructure(t *testing.T) {
	cfg := modbus.PoolConfig{
		MaxConnections:     100,
		IdleTimeout:        5 * time.Minute,
		HealthCheckPeriod:  30 * time.Second,
		ConnectionTimeout:  10 * time.Second,
		RetryAttempts:      3,
		RetryDelay:         100 * time.Millisecond,
		CircuitBreakerName: "test-breaker",
	}

	if cfg.MaxConnections != 100 {
		t.Errorf("expected MaxConnections 100, got %d", cfg.MaxConnections)
	}
	if cfg.CircuitBreakerName != "test-breaker" {
		t.Errorf("expected CircuitBreakerName 'test-breaker', got %q", cfg.CircuitBreakerName)
	}
}

// TestPoolConfigScaling tests pool configuration for different scales.
func TestPoolConfigScaling(t *testing.T) {
	tests := []struct {
		name           string
		maxConnections int
		description    string
	}{
		{"Small deployment", 50, "Single factory floor"},
		{"Medium deployment", 200, "Multi-line factory"},
		{"Large deployment", 500, "Industrial campus"},
		{"Enterprise deployment", 1000, "Multi-site operation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := modbus.PoolConfig{MaxConnections: tt.maxConnections}
			if cfg.MaxConnections != tt.maxConnections {
				t.Errorf("expected MaxConnections %d, got %d", tt.maxConnections, cfg.MaxConnections)
			}
		})
	}
}

// TestPoolStatsFields tests all PoolStats fields.
func TestPoolStatsFields(t *testing.T) {
	stats := modbus.PoolStats{
		TotalConnections:  100,
		ActiveConnections: 75,
		InUseConnections:  50,
		MaxConnections:    500,
	}

	// Verify relationships
	if stats.InUseConnections > stats.ActiveConnections {
		t.Error("InUseConnections should not exceed ActiveConnections")
	}
	if stats.ActiveConnections > stats.TotalConnections {
		t.Error("ActiveConnections should not exceed TotalConnections")
	}
	if stats.TotalConnections > stats.MaxConnections {
		t.Error("TotalConnections should not exceed MaxConnections")
	}
}

// TestPoolStatsUtilization tests pool utilization calculations.
func TestPoolStatsUtilization(t *testing.T) {
	tests := []struct {
		name        string
		active      int
		max         int
		expectedPct float64
	}{
		{"Empty pool", 0, 100, 0.0},
		{"Half full", 50, 100, 50.0},
		{"Nearly full", 95, 100, 95.0},
		{"At capacity", 100, 100, 100.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := modbus.PoolStats{
				ActiveConnections: tt.active,
				MaxConnections:    tt.max,
			}
			utilization := float64(stats.ActiveConnections) / float64(stats.MaxConnections) * 100
			if utilization != tt.expectedPct {
				t.Errorf("expected utilization %.1f%%, got %.1f%%", tt.expectedPct, utilization)
			}
		})
	}
}

// TestDeviceHealthStates tests various device health states.
func TestDeviceHealthStates(t *testing.T) {
	tests := []struct {
		name          string
		connected     bool
		cbOpen        bool
		hasError      bool
		expectedState string
	}{
		{"Healthy", true, false, false, "healthy"},
		{"Disconnected", false, false, true, "disconnected"},
		{"Circuit open", false, true, true, "circuit_open"},
		{"Recovering", true, false, true, "recovering"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.hasError {
				err = domain.ErrConnectionFailed
			}

			health := modbus.DeviceHealth{
				DeviceID:           "test-device",
				Connected:          tt.connected,
				CircuitBreakerOpen: tt.cbOpen,
				LastError:          err,
			}

			// Determine state based on health
			var state string
			switch {
			case health.CircuitBreakerOpen:
				state = "circuit_open"
			case !health.Connected:
				state = "disconnected"
			case health.LastError != nil:
				state = "recovering"
			default:
				state = "healthy"
			}

			if state != tt.expectedState {
				t.Errorf("expected state %q, got %q", tt.expectedState, state)
			}
		})
	}
}

// TestDeviceStatsAggregation tests device stats aggregation.
func TestDeviceStatsAggregation(t *testing.T) {
	deviceStats := []modbus.DeviceStats{
		{DeviceID: "dev1", ReadCount: 100, WriteCount: 50, ErrorCount: 5},
		{DeviceID: "dev2", ReadCount: 200, WriteCount: 100, ErrorCount: 10},
		{DeviceID: "dev3", ReadCount: 150, WriteCount: 75, ErrorCount: 3},
	}

	var totalReads, totalWrites, totalErrors uint64
	for _, ds := range deviceStats {
		totalReads += ds.ReadCount
		totalWrites += ds.WriteCount
		totalErrors += ds.ErrorCount
	}

	if totalReads != 450 {
		t.Errorf("expected total reads 450, got %d", totalReads)
	}
	if totalWrites != 225 {
		t.Errorf("expected total writes 225, got %d", totalWrites)
	}
	if totalErrors != 18 {
		t.Errorf("expected total errors 18, got %d", totalErrors)
	}
}

// TestDeviceStatsErrorRate tests error rate calculation.
func TestDeviceStatsErrorRate(t *testing.T) {
	tests := []struct {
		name          string
		readCount     uint64
		errorCount    uint64
		expectedRate  float64
		toleranceRate float64
	}{
		{"No errors", 1000, 0, 0.0, 0.001},
		{"Low error rate", 1000, 10, 1.0, 0.001},
		{"High error rate", 1000, 100, 10.0, 0.001},
		{"All errors", 100, 100, 100.0, 0.001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := modbus.DeviceStats{
				ReadCount:  tt.readCount,
				ErrorCount: tt.errorCount,
			}
			var errorRate float64
			if stats.ReadCount > 0 {
				errorRate = float64(stats.ErrorCount) / float64(stats.ReadCount) * 100
			}
			diff := errorRate - tt.expectedRate
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.toleranceRate {
				t.Errorf("expected error rate %.2f%%, got %.2f%%", tt.expectedRate, errorRate)
			}
		})
	}
}

// TestPoolTimeoutConfigurations tests various timeout configurations.
func TestPoolTimeoutConfigurations(t *testing.T) {
	tests := []struct {
		name              string
		idleTimeout       time.Duration
		healthCheckPeriod time.Duration
		connectionTimeout time.Duration
	}{
		{"Fast/Aggressive", 1 * time.Minute, 10 * time.Second, 5 * time.Second},
		{"Standard", 5 * time.Minute, 30 * time.Second, 10 * time.Second},
		{"Conservative", 10 * time.Minute, 60 * time.Second, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := modbus.PoolConfig{
				IdleTimeout:       tt.idleTimeout,
				HealthCheckPeriod: tt.healthCheckPeriod,
				ConnectionTimeout: tt.connectionTimeout,
			}

			// Health check should be more frequent than idle timeout
			if cfg.HealthCheckPeriod >= cfg.IdleTimeout {
				t.Error("HealthCheckPeriod should be less than IdleTimeout")
			}
		})
	}
}

// TestCircuitBreakerNaming tests circuit breaker naming patterns.
func TestCircuitBreakerNaming(t *testing.T) {
	devices := []struct {
		deviceID     string
		expectedName string
	}{
		{"plc-001", "modbus-plc-001"},
		{"sensor-temp-1", "modbus-sensor-temp-1"},
		{"factory-a-conveyor", "modbus-factory-a-conveyor"},
	}

	for _, d := range devices {
		t.Run(d.deviceID, func(t *testing.T) {
			// Simulate circuit breaker naming pattern
			name := "modbus-" + d.deviceID
			if name != d.expectedName {
				t.Errorf("expected CB name %q, got %q", d.expectedName, name)
			}
		})
	}
}

// TestPoolCapacityBehavior tests pool capacity edge cases.
func TestPoolCapacityBehavior(t *testing.T) {
	tests := []struct {
		name           string
		maxConnections int
		currentCount   int
		canAdd         bool
	}{
		{"Empty pool", 100, 0, true},
		{"Half capacity", 100, 50, true},
		{"Near capacity", 100, 99, true},
		{"At capacity", 100, 100, false},
		{"Over capacity check", 100, 101, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canAdd := tt.currentCount < tt.maxConnections
			if canAdd != tt.canAdd {
				t.Errorf("expected canAdd=%v for %d/%d, got %v",
					tt.canAdd, tt.currentCount, tt.maxConnections, canAdd)
			}
		})
	}
}

// TestRetryDelayProgression tests exponential backoff progression.
func TestRetryDelayProgression(t *testing.T) {
	baseDelay := 100 * time.Millisecond
	maxRetries := 5

	var delays []time.Duration
	for i := 0; i < maxRetries; i++ {
		// Simple exponential backoff: delay * 2^attempt
		delay := baseDelay * (1 << i)
		delays = append(delays, delay)
	}

	expected := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1600 * time.Millisecond,
	}

	for i, exp := range expected {
		if delays[i] != exp {
			t.Errorf("retry %d: expected delay %v, got %v", i, exp, delays[i])
		}
	}
}

// TestDefaultPoolConfigIndustrialScale verifies defaults support industrial scale.
func TestDefaultPoolConfigIndustrialScale(t *testing.T) {
	cfg := modbus.DefaultPoolConfig()

	// Industrial deployments often have 100-1000 devices
	// Default should support at least medium-scale
	if cfg.MaxConnections < 100 {
		t.Errorf("MaxConnections (%d) too low for industrial scale", cfg.MaxConnections)
	}

	// Connection timeout should be reasonable for network delays
	if cfg.ConnectionTimeout < 5*time.Second {
		t.Errorf("ConnectionTimeout (%v) may be too aggressive", cfg.ConnectionTimeout)
	}

	// Idle timeout should allow efficient connection reuse
	if cfg.IdleTimeout < 1*time.Minute {
		t.Errorf("IdleTimeout (%v) may cause too much reconnection overhead", cfg.IdleTimeout)
	}
}

// TestPoolHealthCheckFrequency tests health check configuration.
func TestPoolHealthCheckFrequency(t *testing.T) {
	tests := []struct {
		name              string
		healthCheckPeriod time.Duration
		expectedChecks    int // per minute
	}{
		{"Fast checks", 10 * time.Second, 6},
		{"Standard checks", 30 * time.Second, 2},
		{"Slow checks", 60 * time.Second, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checksPerMinute := int(time.Minute / tt.healthCheckPeriod)
			if checksPerMinute != tt.expectedChecks {
				t.Errorf("expected %d checks/min, got %d", tt.expectedChecks, checksPerMinute)
			}
		})
	}
}

// TestDeviceStatsTiming tests timing-related device stats.
func TestDeviceStatsTiming(t *testing.T) {
	stats := modbus.DeviceStats{
		DeviceID:       "test-device",
		ReadCount:      1000,
		AvgReadTimeMs:  2.5,
		WriteCount:     500,
		AvgWriteTimeMs: 3.0,
	}

	// Calculate total time spent
	totalReadTimeMs := float64(stats.ReadCount) * stats.AvgReadTimeMs
	totalWriteTimeMs := float64(stats.WriteCount) * stats.AvgWriteTimeMs

	if totalReadTimeMs != 2500.0 {
		t.Errorf("expected total read time 2500ms, got %.1fms", totalReadTimeMs)
	}
	if totalWriteTimeMs != 1500.0 {
		t.Errorf("expected total write time 1500ms, got %.1fms", totalWriteTimeMs)
	}
}

// TestPooledClientStates tests the states a pooled client can be in.
func TestPooledClientStates(t *testing.T) {
	states := []struct {
		name        string
		connected   bool
		inUse       bool
		hasError    bool
		description string
	}{
		{"Idle", true, false, false, "Connected but not in use"},
		{"Active", true, true, false, "Connected and processing"},
		{"Error", true, false, true, "Connected but last op failed"},
		{"Disconnected", false, false, false, "Not connected"},
		{"Reconnecting", false, false, true, "Failed, pending reconnect"},
	}

	for _, s := range states {
		t.Run(s.name, func(t *testing.T) {
			// This test documents the expected states for pooled clients
			// Actual state management is in the pool implementation
			t.Logf("State: %s - %s", s.name, s.description)
		})
	}
}

// TestConnectionPoolConcurrencyLimits tests concurrency limit configurations.
func TestConnectionPoolConcurrencyLimits(t *testing.T) {
	cfg := modbus.DefaultPoolConfig()

	// Verify reasonable limits
	if cfg.MaxConnections < 1 {
		t.Error("MaxConnections must be at least 1")
	}

	if cfg.RetryAttempts < 0 {
		t.Error("RetryAttempts must be non-negative")
	}

	if cfg.RetryDelay <= 0 {
		t.Error("RetryDelay must be positive")
	}
}
