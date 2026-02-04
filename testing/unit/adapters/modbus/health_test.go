// Package modbus_test tests the Modbus health and diagnostics functionality.
package modbus_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/modbus"
)

// =============================================================================
// Tag Health Tracking Tests
// =============================================================================

// TestTagHealthTracking tests per-tag health tracking.
func TestTagHealthTracking(t *testing.T) {
	diag := modbus.NewTagDiagnostic("health-tag-001")

	if diag.TagID != "health-tag-001" {
		t.Errorf("expected TagID 'health-tag-001', got %s", diag.TagID)
	}
}

// TestTagHealthCounters tests atomic counter operations.
func TestTagHealthCounters(t *testing.T) {
	diag := modbus.NewTagDiagnostic("counter-tag")

	// Simulate multiple successful reads
	for i := 0; i < 100; i++ {
		diag.ReadCount.Add(1)
	}

	if diag.ReadCount.Load() != 100 {
		t.Errorf("expected ReadCount 100, got %d", diag.ReadCount.Load())
	}

	// Simulate some errors
	for i := 0; i < 5; i++ {
		diag.ErrorCount.Add(1)
	}

	if diag.ErrorCount.Load() != 5 {
		t.Errorf("expected ErrorCount 5, got %d", diag.ErrorCount.Load())
	}
}

// TestTagHealthTimestamps tests timestamp tracking.
func TestTagHealthTimestamps(t *testing.T) {
	diag := modbus.NewTagDiagnostic("timestamp-tag")

	// Set last success time
	successTime := time.Now()
	diag.LastSuccessTime.Store(successTime)

	stored := diag.LastSuccessTime.Load()
	if stored == nil {
		t.Fatal("expected LastSuccessTime to be set")
	}
	if !stored.(time.Time).Equal(successTime) {
		t.Errorf("expected LastSuccessTime %v, got %v", successTime, stored)
	}

	// Set error time
	errorTime := time.Now().Add(time.Second)
	diag.LastErrorTime.Store(errorTime)

	storedError := diag.LastErrorTime.Load()
	if storedError == nil {
		t.Fatal("expected LastErrorTime to be set")
	}
	if !storedError.(time.Time).Equal(errorTime) {
		t.Errorf("expected LastErrorTime %v, got %v", errorTime, storedError)
	}
}

// TestTagHealthErrorTracking tests error storage with domain errors.
func TestTagHealthErrorTracking(t *testing.T) {
	diag := modbus.NewTagDiagnostic("error-tag")

	// Store a standard error
	testErr := errors.New("plc-001: connection timeout")
	diag.LastError.Store(testErr)
	diag.ErrorCount.Add(1)

	stored := diag.LastError.Load()
	if stored == nil {
		t.Fatal("expected LastError to be set")
	}

	storedErr, ok := stored.(error)
	if !ok {
		t.Fatalf("expected error, got %T", stored)
	}
	if storedErr.Error() != "plc-001: connection timeout" {
		t.Errorf("expected error message 'plc-001: connection timeout', got %s", storedErr.Error())
	}
}

// TestTagHealthConcurrency tests concurrent access to diagnostics.
func TestTagHealthConcurrency(t *testing.T) {
	diag := modbus.NewTagDiagnostic("concurrent-tag")

	var wg sync.WaitGroup
	workers := 10
	opsPerWorker := 1000

	// Concurrent reads
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerWorker; j++ {
				diag.ReadCount.Add(1)
				diag.LastSuccessTime.Store(time.Now())
			}
		}()
	}

	// Concurrent error tracking
	for i := 0; i < workers/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerWorker/10; j++ {
				diag.ErrorCount.Add(1)
				diag.LastErrorTime.Store(time.Now())
			}
		}()
	}

	wg.Wait()

	expectedReads := uint64(workers * opsPerWorker)
	if diag.ReadCount.Load() != expectedReads {
		t.Errorf("expected ReadCount %d, got %d", expectedReads, diag.ReadCount.Load())
	}

	expectedErrors := uint64((workers / 2) * (opsPerWorker / 10))
	if diag.ErrorCount.Load() != expectedErrors {
		t.Errorf("expected ErrorCount %d, got %d", expectedErrors, diag.ErrorCount.Load())
	}
}

// =============================================================================
// Stats Average Calculation Tests
// =============================================================================

// TestStatsAverageCalculation tests average time calculation logic.
func TestStatsAverageCalculation(t *testing.T) {
	tests := []struct {
		name        string
		readCount   uint64
		totalTimeNs int64
		expectedMs  float64
	}{
		{"No operations", 0, 0, 0},
		{"Single fast read", 1, 100000, 0.1},      // 0.1ms
		{"Multiple reads", 100, 50000000, 0.5},    // 50ms total / 100 = 0.5ms avg
		{"Many reads", 10000, 100000000000, 10.0}, // 100s total / 10000 = 10ms avg
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := &modbus.ClientStats{}
			stats.ReadCount.Store(tt.readCount)
			stats.TotalReadTime.Store(tt.totalTimeNs)

			var avgMs float64
			if tt.readCount > 0 {
				avgMs = float64(stats.TotalReadTime.Load()) / float64(stats.ReadCount.Load()) / 1e6
			}

			// Allow small floating point tolerance
			diff := avgMs - tt.expectedMs
			if diff > 0.001 || diff < -0.001 {
				t.Errorf("expected average %.3fms, got %.3fms", tt.expectedMs, avgMs)
			}
		})
	}
}

// =============================================================================
// Health State Transition Tests
// =============================================================================

// TestHealthStateTransitions tests typical health state transitions.
func TestHealthStateTransitions(t *testing.T) {
	tests := []struct {
		name         string
		connected    bool
		breakerOpen  bool
		hasError     bool
		healthyState string
	}{
		{"Healthy", true, false, false, "operational"},
		{"Disconnected", false, false, true, "disconnected"},
		{"CircuitOpen", false, true, true, "circuit_open"},
		{"Recovering", true, false, true, "recovering"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var lastErr error
			if tt.hasError {
				lastErr = errors.New("test error")
			}

			health := modbus.DeviceHealth{
				DeviceID:           "test-device",
				Connected:          tt.connected,
				CircuitBreakerOpen: tt.breakerOpen,
				LastError:          lastErr,
			}

			// Derive health state
			var state string
			switch {
			case health.Connected && !health.CircuitBreakerOpen && health.LastError == nil:
				state = "operational"
			case !health.Connected && health.CircuitBreakerOpen:
				state = "circuit_open"
			case !health.Connected:
				state = "disconnected"
			case health.Connected && health.LastError != nil:
				state = "recovering"
			default:
				state = "unknown"
			}

			if state != tt.healthyState {
				t.Errorf("expected state %q, got %q", tt.healthyState, state)
			}
		})
	}
}

// =============================================================================
// Error Rate Calculation Tests
// =============================================================================

// TestErrorRateCalculation tests error rate calculation.
func TestErrorRateCalculation(t *testing.T) {
	tests := []struct {
		name         string
		readCount    uint64
		writeCount   uint64
		errorCount   uint64
		expectedRate float64
	}{
		{"No operations", 0, 0, 0, 0},
		{"No errors", 1000, 100, 0, 0},
		{"Low error rate", 1000, 100, 11, 0.01}, // 11/(1000+100) = 1%
		{"High error rate", 100, 10, 55, 0.5},   // 55/(100+10) = 50%
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := modbus.DeviceStats{
				ReadCount:  tt.readCount,
				WriteCount: tt.writeCount,
				ErrorCount: tt.errorCount,
			}

			totalOps := stats.ReadCount + stats.WriteCount
			var errorRate float64
			if totalOps > 0 {
				errorRate = float64(stats.ErrorCount) / float64(totalOps)
			}

			diff := errorRate - tt.expectedRate
			if diff > 0.001 || diff < -0.001 {
				t.Errorf("expected error rate %.3f, got %.3f", tt.expectedRate, errorRate)
			}
		})
	}
}

// =============================================================================
// Health Aggregation Tests
// =============================================================================

// TestHealthAggregation tests aggregating health across multiple devices.
func TestHealthAggregation(t *testing.T) {
	devices := []modbus.DeviceHealth{
		{DeviceID: "plc-001", Connected: true, CircuitBreakerOpen: false},
		{DeviceID: "plc-002", Connected: true, CircuitBreakerOpen: false},
		{DeviceID: "plc-003", Connected: false, CircuitBreakerOpen: true},
		{DeviceID: "plc-004", Connected: true, CircuitBreakerOpen: false},
		{DeviceID: "plc-005", Connected: false, CircuitBreakerOpen: false},
	}

	var connected, disconnected, breakerOpen int
	for _, d := range devices {
		if d.Connected {
			connected++
		} else {
			disconnected++
		}
		if d.CircuitBreakerOpen {
			breakerOpen++
		}
	}

	if connected != 3 {
		t.Errorf("expected 3 connected devices, got %d", connected)
	}
	if disconnected != 2 {
		t.Errorf("expected 2 disconnected devices, got %d", disconnected)
	}
	if breakerOpen != 1 {
		t.Errorf("expected 1 circuit breaker open, got %d", breakerOpen)
	}

	// Health percentage
	healthPct := float64(connected) / float64(len(devices)) * 100
	if healthPct != 60.0 {
		t.Errorf("expected 60%% health, got %.1f%%", healthPct)
	}
}

// =============================================================================
// Consecutive Failures Tests
// =============================================================================

// TestConsecutiveFailures tests consecutive failure counter behavior.
func TestConsecutiveFailures(t *testing.T) {
	var consecutiveFailures atomic.Int32

	// Simulate failures
	for i := 0; i < 5; i++ {
		consecutiveFailures.Add(1)
	}

	if consecutiveFailures.Load() != 5 {
		t.Errorf("expected 5 consecutive failures, got %d", consecutiveFailures.Load())
	}

	// Simulate success resets counter
	consecutiveFailures.Store(0)

	if consecutiveFailures.Load() != 0 {
		t.Errorf("expected 0 after reset, got %d", consecutiveFailures.Load())
	}
}

// =============================================================================
// Backoff Calculation Tests
// =============================================================================

// TestBackoff tests exponential backoff calculation.
func TestBackoff(t *testing.T) {
	tests := []struct {
		failures    int
		baseDelay   time.Duration
		maxDelay    time.Duration
		expectedMin time.Duration
		expectedMax time.Duration
	}{
		{0, time.Second, time.Minute, time.Second, time.Second},
		{1, time.Second, time.Minute, 2 * time.Second, 2 * time.Second},
		{2, time.Second, time.Minute, 4 * time.Second, 4 * time.Second},
		{3, time.Second, time.Minute, 8 * time.Second, 8 * time.Second},
		{10, time.Second, time.Minute, time.Minute, time.Minute}, // Capped
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			delay := tt.baseDelay * time.Duration(1<<uint(tt.failures))
			if delay > tt.maxDelay {
				delay = tt.maxDelay
			}

			if delay < tt.expectedMin || delay > tt.expectedMax {
				t.Errorf("failures=%d: expected delay [%v, %v], got %v",
					tt.failures, tt.expectedMin, tt.expectedMax, delay)
			}
		})
	}
}

// =============================================================================
// Health Check Interval Tests
// =============================================================================

// TestHealthCheckIntervals tests health check interval configurations.
func TestHealthCheckIntervals(t *testing.T) {
	tests := []struct {
		name        string
		interval    time.Duration
		description string
	}{
		{"Fast", 5 * time.Second, "Real-time monitoring"},
		{"Standard", 30 * time.Second, "Normal production"},
		{"Slow", 60 * time.Second, "Low-priority devices"},
		{"Minimal", 5 * time.Minute, "Archival monitoring"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := modbus.PoolConfig{
				HealthCheckPeriod: tt.interval,
			}

			if cfg.HealthCheckPeriod != tt.interval {
				t.Errorf("expected interval %v, got %v", tt.interval, cfg.HealthCheckPeriod)
			}
		})
	}
}

// =============================================================================
// Pool Health Tests
// =============================================================================

// TestPoolHealthMetrics tests pool-level health metrics.
func TestPoolHealthMetrics(t *testing.T) {
	stats := modbus.PoolStats{
		TotalConnections:  10,
		ActiveConnections: 8,
		InUseConnections:  5,
		MaxConnections:    100,
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

	// Calculate utilization
	utilization := float64(stats.ActiveConnections) / float64(stats.MaxConnections) * 100
	if utilization != 8.0 {
		t.Errorf("expected 8%% utilization, got %.1f%%", utilization)
	}
}

// TestIdleConnectionHealth tests idle connection handling.
func TestIdleConnectionHealth(t *testing.T) {
	idleTimeout := 5 * time.Minute

	tests := []struct {
		name     string
		lastUsed time.Duration
		isIdle   bool
	}{
		{"Recently used", 1 * time.Minute, false},
		{"Just idle", 5 * time.Minute, true},
		{"Long idle", 30 * time.Minute, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lastUsed := time.Now().Add(-tt.lastUsed)
			isIdle := time.Since(lastUsed) >= idleTimeout

			if isIdle != tt.isIdle {
				t.Errorf("expected idle=%v, got %v", tt.isIdle, isIdle)
			}
		})
	}
}
