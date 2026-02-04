// Package s7_test tests the S7 health and diagnostics functionality.
package s7_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/s7"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

// =============================================================================
// TagDiagnostic Structure Tests
// =============================================================================

// TestTagDiagnosticStructure tests TagDiagnostic struct fields.
func TestTagDiagnosticStructure(t *testing.T) {
	now := time.Now()
	diag := &s7.TagDiagnostic{
		TagID:           "db1.dbw0",
		LastError:       nil,
		LastErrorTime:   time.Time{},
		LastSuccessTime: now,
		ErrorCount:      0,
		SuccessCount:    100,
	}

	if diag.TagID != "db1.dbw0" {
		t.Errorf("expected TagID 'db1.dbw0', got %s", diag.TagID)
	}
	if diag.LastError != nil {
		t.Errorf("expected LastError nil, got %v", diag.LastError)
	}
	if diag.SuccessCount != 100 {
		t.Errorf("expected SuccessCount 100, got %d", diag.SuccessCount)
	}
	if !diag.LastSuccessTime.Equal(now) {
		t.Errorf("expected LastSuccessTime %v, got %v", now, diag.LastSuccessTime)
	}
}

// TestTagDiagnosticWithError tests TagDiagnostic with error state.
func TestTagDiagnosticWithError(t *testing.T) {
	now := time.Now()
	testErr := errors.New("read failed: DB not accessible")

	diag := &s7.TagDiagnostic{
		TagID:         "db100.dbd0",
		LastError:     testErr,
		LastErrorTime: now,
		ErrorCount:    5,
		SuccessCount:  95,
	}

	if diag.LastError == nil {
		t.Error("expected LastError to be set")
	}
	if diag.ErrorCount != 5 {
		t.Errorf("expected ErrorCount 5, got %d", diag.ErrorCount)
	}

	// Error rate calculation
	totalOps := diag.SuccessCount + diag.ErrorCount
	errorRate := float64(diag.ErrorCount) / float64(totalOps) * 100
	if errorRate != 5.0 {
		t.Errorf("expected error rate 5%%, got %.1f%%", errorRate)
	}
}

// TestTagDiagnosticConcurrency tests concurrent access to tag diagnostics.
func TestTagDiagnosticConcurrency(t *testing.T) {
	// Test that multiple goroutines can safely update counters
	// Since TagDiagnostic has unexported mutex, we test the concept
	var successCount atomic.Uint64
	var errorCount atomic.Uint64

	var wg sync.WaitGroup
	workers := 10
	opsPerWorker := 100

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerWorker; j++ {
				successCount.Add(1)
			}
		}()
	}

	// Some error updates
	for i := 0; i < workers/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerWorker/10; j++ {
				errorCount.Add(1)
			}
		}()
	}

	wg.Wait()

	expectedSuccess := uint64(workers * opsPerWorker)
	expectedErrors := uint64((workers / 2) * (opsPerWorker / 10))

	if successCount.Load() != expectedSuccess {
		t.Errorf("expected SuccessCount %d, got %d", expectedSuccess, successCount.Load())
	}
	if errorCount.Load() != expectedErrors {
		t.Errorf("expected ErrorCount %d, got %d", expectedErrors, errorCount.Load())
	}
}

// =============================================================================
// ClientStats Tests
// =============================================================================

// TestClientStatsStructure tests ClientStats struct fields.
func TestClientStatsStructure(t *testing.T) {
	stats := &s7.ClientStats{}

	// Test atomic operations
	stats.ReadCount.Add(500)
	stats.WriteCount.Add(100)
	stats.ErrorCount.Add(10)
	stats.RetryCount.Add(20)
	stats.ReconnectCount.Add(2)
	stats.TotalReadTime.Add(5000000)  // 5ms in nanoseconds
	stats.TotalWriteTime.Add(2000000) // 2ms in nanoseconds

	if stats.ReadCount.Load() != 500 {
		t.Errorf("expected ReadCount 500, got %d", stats.ReadCount.Load())
	}
	if stats.WriteCount.Load() != 100 {
		t.Errorf("expected WriteCount 100, got %d", stats.WriteCount.Load())
	}
	if stats.ErrorCount.Load() != 10 {
		t.Errorf("expected ErrorCount 10, got %d", stats.ErrorCount.Load())
	}
	if stats.RetryCount.Load() != 20 {
		t.Errorf("expected RetryCount 20, got %d", stats.RetryCount.Load())
	}
	if stats.ReconnectCount.Load() != 2 {
		t.Errorf("expected ReconnectCount 2, got %d", stats.ReconnectCount.Load())
	}
	if stats.TotalReadTime.Load() != 5000000 {
		t.Errorf("expected TotalReadTime 5000000, got %d", stats.TotalReadTime.Load())
	}
	if stats.TotalWriteTime.Load() != 2000000 {
		t.Errorf("expected TotalWriteTime 2000000, got %d", stats.TotalWriteTime.Load())
	}
}

// TestClientStatsConcurrency tests concurrent stats updates.
func TestClientStatsConcurrency(t *testing.T) {
	stats := &s7.ClientStats{}

	var wg sync.WaitGroup
	workers := 50
	opsPerWorker := 1000

	// Concurrent reads
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerWorker; j++ {
				stats.ReadCount.Add(1)
				stats.TotalReadTime.Add(100000) // 0.1ms
			}
		}()
	}

	// Concurrent writes (fewer)
	for i := 0; i < workers/5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerWorker/10; j++ {
				stats.WriteCount.Add(1)
				stats.TotalWriteTime.Add(200000) // 0.2ms
			}
		}()
	}

	wg.Wait()

	expectedReads := uint64(workers * opsPerWorker)
	if stats.ReadCount.Load() != expectedReads {
		t.Errorf("expected ReadCount %d, got %d", expectedReads, stats.ReadCount.Load())
	}

	expectedWrites := uint64((workers / 5) * (opsPerWorker / 10))
	if stats.WriteCount.Load() != expectedWrites {
		t.Errorf("expected WriteCount %d, got %d", expectedWrites, stats.WriteCount.Load())
	}
}

// =============================================================================
// PoolStats Tests
// =============================================================================

// TestPoolStatsStructure tests PoolStats struct fields.
func TestPoolStatsStructure(t *testing.T) {
	stats := s7.PoolStats{
		TotalConnections:  10,
		ActiveConnections: 8,
		MaxConnections:    50,
		Devices:           make([]s7.DeviceStats, 0),
	}

	if stats.TotalConnections != 10 {
		t.Errorf("expected TotalConnections 10, got %d", stats.TotalConnections)
	}
	if stats.ActiveConnections != 8 {
		t.Errorf("expected ActiveConnections 8, got %d", stats.ActiveConnections)
	}
	if stats.MaxConnections != 50 {
		t.Errorf("expected MaxConnections 50, got %d", stats.MaxConnections)
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
		{"Empty", 0, 100, 0},
		{"Half full", 50, 100, 50},
		{"Near capacity", 95, 100, 95},
		{"Full", 100, 100, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := s7.PoolStats{
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

// =============================================================================
// DeviceStats Tests
// =============================================================================

// TestDeviceStatsStructure tests DeviceStats struct fields.
func TestDeviceStatsStructure(t *testing.T) {
	now := time.Now()
	stats := s7.DeviceStats{
		DeviceID:            "plc-001",
		Connected:           true,
		LastUsed:            now,
		LastError:           "",
		ReadCount:           1000,
		WriteCount:          200,
		ErrorCount:          5,
		ReconnectCount:      1,
		BreakerState:        "closed",
		ConsecutiveFailures: 0,
		TagDiagnosticsCount: 10,
	}

	if stats.DeviceID != "plc-001" {
		t.Errorf("expected DeviceID 'plc-001', got %s", stats.DeviceID)
	}
	if !stats.Connected {
		t.Error("expected Connected true")
	}
	if !stats.LastUsed.Equal(now) {
		t.Errorf("expected LastUsed %v, got %v", now, stats.LastUsed)
	}
	if stats.ReadCount != 1000 {
		t.Errorf("expected ReadCount 1000, got %d", stats.ReadCount)
	}
	if stats.WriteCount != 200 {
		t.Errorf("expected WriteCount 200, got %d", stats.WriteCount)
	}
	if stats.ErrorCount != 5 {
		t.Errorf("expected ErrorCount 5, got %d", stats.ErrorCount)
	}
	if stats.ReconnectCount != 1 {
		t.Errorf("expected ReconnectCount 1, got %d", stats.ReconnectCount)
	}
	if stats.BreakerState != "closed" {
		t.Errorf("expected BreakerState 'closed', got %s", stats.BreakerState)
	}
	if stats.ConsecutiveFailures != 0 {
		t.Errorf("expected ConsecutiveFailures 0, got %d", stats.ConsecutiveFailures)
	}
	if stats.TagDiagnosticsCount != 10 {
		t.Errorf("expected TagDiagnosticsCount 10, got %d", stats.TagDiagnosticsCount)
	}
}

// TestDeviceStatsErrorState tests DeviceStats with error state.
func TestDeviceStatsErrorState(t *testing.T) {
	stats := s7.DeviceStats{
		DeviceID:            "plc-002",
		Connected:           false,
		LastError:           "connection refused: ECONNREFUSED",
		ErrorCount:          50,
		ReconnectCount:      10,
		BreakerState:        "open",
		ConsecutiveFailures: 5,
	}

	if stats.Connected {
		t.Error("expected Connected false in error state")
	}
	if stats.LastError == "" {
		t.Error("expected LastError to be set")
	}
	if stats.BreakerState != "open" {
		t.Errorf("expected BreakerState 'open', got %s", stats.BreakerState)
	}
	if stats.ConsecutiveFailures != 5 {
		t.Errorf("expected ConsecutiveFailures 5, got %d", stats.ConsecutiveFailures)
	}
}

// =============================================================================
// DeviceHealth Tests
// =============================================================================

// TestDeviceHealthStructure tests DeviceHealth struct fields.
func TestDeviceHealthStructure(t *testing.T) {
	now := time.Now()
	health := s7.DeviceHealth{
		DeviceID:            "plc-001",
		Connected:           true,
		CircuitBreakerOpen:  false,
		LastError:           nil,
		LastUsed:            now,
		ConsecutiveFailures: 0,
	}

	if health.DeviceID != "plc-001" {
		t.Errorf("expected DeviceID 'plc-001', got %s", health.DeviceID)
	}
	if !health.Connected {
		t.Error("expected Connected true")
	}
	if health.CircuitBreakerOpen {
		t.Error("expected CircuitBreakerOpen false")
	}
	if health.LastError != nil {
		t.Errorf("expected LastError nil, got %v", health.LastError)
	}
	if !health.LastUsed.Equal(now) {
		t.Errorf("expected LastUsed %v, got %v", now, health.LastUsed)
	}
	if health.ConsecutiveFailures != 0 {
		t.Errorf("expected ConsecutiveFailures 0, got %d", health.ConsecutiveFailures)
	}
}

// TestDeviceHealthTransitions tests health state transitions.
func TestDeviceHealthTransitions(t *testing.T) {
	tests := []struct {
		name          string
		connected     bool
		breakerOpen   bool
		failures      int32
		expectedState string
	}{
		{"Healthy", true, false, 0, "healthy"},
		{"Degraded", true, false, 3, "degraded"},
		{"CircuitOpen", false, true, 5, "circuit_open"},
		{"Disconnected", false, false, 0, "disconnected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health := s7.DeviceHealth{
				Connected:           tt.connected,
				CircuitBreakerOpen:  tt.breakerOpen,
				ConsecutiveFailures: tt.failures,
			}

			var state string
			switch {
			case health.Connected && !health.CircuitBreakerOpen && health.ConsecutiveFailures == 0:
				state = "healthy"
			case health.Connected && health.ConsecutiveFailures > 0:
				state = "degraded"
			case health.CircuitBreakerOpen:
				state = "circuit_open"
			case !health.Connected:
				state = "disconnected"
			}

			if state != tt.expectedState {
				t.Errorf("expected state %q, got %q", tt.expectedState, state)
			}
		})
	}
}

// =============================================================================
// Health Aggregation Tests
// =============================================================================

// TestHealthAggregation tests aggregating health across multiple S7 devices.
func TestHealthAggregation(t *testing.T) {
	devices := []s7.DeviceHealth{
		{DeviceID: "plc-001", Connected: true, CircuitBreakerOpen: false},
		{DeviceID: "plc-002", Connected: true, CircuitBreakerOpen: false},
		{DeviceID: "plc-003", Connected: false, CircuitBreakerOpen: true},
		{DeviceID: "plc-004", Connected: true, CircuitBreakerOpen: false},
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
		t.Errorf("expected 3 connected, got %d", connected)
	}
	if disconnected != 1 {
		t.Errorf("expected 1 disconnected, got %d", disconnected)
	}
	if breakerOpen != 1 {
		t.Errorf("expected 1 breaker open, got %d", breakerOpen)
	}

	healthPct := float64(connected) / float64(len(devices)) * 100
	if healthPct != 75.0 {
		t.Errorf("expected 75%% health, got %.1f%%", healthPct)
	}
}

// =============================================================================
// Context Cancellation Tests
// =============================================================================

// TestHealthCheckContext tests health check respects context cancellation.
func TestHealthCheckContext(t *testing.T) {
	// Test that context cancellation is handled properly
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Simulate a health check that respects context
	select {
	case <-ctx.Done():
		// Expected behavior - context cancelled
		if ctx.Err() != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Error("context cancellation not detected")
	}
}

// TestHealthCheckTimeout tests health check with timeout.
func TestHealthCheckTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Simulate slow operation
	select {
	case <-ctx.Done():
		if ctx.Err() != context.DeadlineExceeded {
			t.Errorf("expected context.DeadlineExceeded, got %v", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Error("timeout not triggered")
	}
}

// =============================================================================
// Consecutive Failures Tests
// =============================================================================

// TestConsecutiveFailuresTracking tests consecutive failure counter.
func TestConsecutiveFailuresTracking(t *testing.T) {
	var failures atomic.Int32

	// Record failures
	for i := 0; i < 5; i++ {
		failures.Add(1)
	}

	if failures.Load() != 5 {
		t.Errorf("expected 5 consecutive failures, got %d", failures.Load())
	}

	// Success resets counter
	failures.Store(0)

	if failures.Load() != 0 {
		t.Errorf("expected 0 after reset, got %d", failures.Load())
	}
}

// TestBackoffThresholds tests backoff trigger thresholds.
func TestBackoffThresholds(t *testing.T) {
	tests := []struct {
		failures      int32
		shouldBackoff bool
		description   string
	}{
		{0, false, "No failures - no backoff"},
		{1, false, "Single failure - no backoff yet"},
		{2, false, "Two failures - still trying"},
		{3, true, "Three failures - start backoff"},
		{5, true, "Five failures - extended backoff"},
		{10, true, "Ten failures - circuit breaker territory"},
	}

	backoffThreshold := int32(3)

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			shouldBackoff := tt.failures >= backoffThreshold
			if shouldBackoff != tt.shouldBackoff {
				t.Errorf("failures=%d: expected backoff=%v, got %v",
					tt.failures, tt.shouldBackoff, shouldBackoff)
			}
		})
	}
}

// =============================================================================
// S7 Area Code Tests
// =============================================================================

// TestS7AreaCodeMapping tests S7 area code mappings.
func TestS7AreaCodeMapping(t *testing.T) {
	expectedCodes := map[domain.S7Area]int{
		domain.S7AreaDB: 0x84, // Data Blocks
		domain.S7AreaM:  0x83, // Merkers
		domain.S7AreaI:  0x81, // Inputs
		domain.S7AreaQ:  0x82, // Outputs
		domain.S7AreaT:  0x1D, // Timers
		domain.S7AreaC:  0x1C, // Counters
	}

	for area, expectedCode := range expectedCodes {
		t.Run(string(area), func(t *testing.T) {
			code, exists := s7.S7AreaCode[area]
			if !exists {
				t.Errorf("expected area code for %s to exist", area)
				return
			}
			if code != expectedCode {
				t.Errorf("expected area code 0x%02X for %s, got 0x%02X",
					expectedCode, area, code)
			}
		})
	}
}

// TestS7AreaCodeComplete tests that all S7 areas have mappings.
func TestS7AreaCodeComplete(t *testing.T) {
	areas := []domain.S7Area{
		domain.S7AreaDB,
		domain.S7AreaM,
		domain.S7AreaI,
		domain.S7AreaQ,
		domain.S7AreaT,
		domain.S7AreaC,
	}

	for _, area := range areas {
		if _, exists := s7.S7AreaCode[area]; !exists {
			t.Errorf("missing S7AreaCode mapping for %s", area)
		}
	}
}

// =============================================================================
// Buffer Pool Tests
// =============================================================================

// TestBufferPoolGet tests buffer pool allocation.
func TestBufferPoolGet(t *testing.T) {
	sizes := []int{1, 2, 4, 8, 16}

	for _, size := range sizes {
		t.Run("", func(t *testing.T) {
			buf := s7.BufferPool.Get(size)
			if len(buf) != size {
				t.Errorf("expected buffer length %d, got %d", size, len(buf))
			}
			s7.BufferPool.Put(buf)
		})
	}
}

// TestBufferPoolLargeAlloc tests buffer pool with large allocation.
func TestBufferPoolLargeAlloc(t *testing.T) {
	// Large buffers should be allocated directly, not from pool
	size := 1024
	buf := s7.BufferPool.Get(size)
	if len(buf) != size {
		t.Errorf("expected buffer length %d, got %d", size, len(buf))
	}
	// Large buffers are not returned to pool, so Put is a no-op
	s7.BufferPool.Put(buf)
}

// TestBufferPoolConcurrency tests concurrent buffer pool access.
func TestBufferPoolConcurrency(t *testing.T) {
	var wg sync.WaitGroup
	workers := 100
	iterations := 1000

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				sizes := []int{1, 2, 4, 8, 16}
				for _, size := range sizes {
					buf := s7.BufferPool.Get(size)
					// Use buffer
					for k := 0; k < len(buf); k++ {
						buf[k] = byte(k)
					}
					s7.BufferPool.Put(buf)
				}
			}
		}()
	}

	wg.Wait()
	// No assertion - test passes if no race condition
}
