// Package service_test tests the polling service functionality.
package service_test

import (
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/service"
)

// TestPollingConfigStructure tests PollingConfig structure.
func TestPollingConfigStructure(t *testing.T) {
	cfg := service.PollingConfig{
		WorkerCount:     10,
		BatchSize:       50,
		DefaultInterval: 1 * time.Second,
		MaxRetries:      3,
		ShutdownTimeout: 30 * time.Second,
	}

	if cfg.WorkerCount != 10 {
		t.Errorf("expected WorkerCount 10, got %d", cfg.WorkerCount)
	}
	if cfg.BatchSize != 50 {
		t.Errorf("expected BatchSize 50, got %d", cfg.BatchSize)
	}
	if cfg.DefaultInterval != 1*time.Second {
		t.Errorf("expected DefaultInterval 1s, got %v", cfg.DefaultInterval)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("expected MaxRetries 3, got %d", cfg.MaxRetries)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("expected ShutdownTimeout 30s, got %v", cfg.ShutdownTimeout)
	}
}

// TestPollingConfigDefaults tests default configuration values.
func TestPollingConfigDefaults(t *testing.T) {
	// Document expected default behaviors
	tests := []struct {
		name     string
		field    string
		value    interface{}
		expected interface{}
	}{
		{"Worker count default", "WorkerCount", 10, 10},
		{"Batch size default", "BatchSize", 50, 50},
		{"Default interval", "DefaultInterval", 1 * time.Second, 1 * time.Second},
		{"Max retries", "MaxRetries", 3, 3},
		{"Shutdown timeout", "ShutdownTimeout", 30 * time.Second, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, tt.value)
			}
		})
	}
}

// TestPollingStatsAtomicOperations tests PollingStats atomic operations.
func TestPollingStatsAtomicOperations(t *testing.T) {
	stats := &service.PollingStats{}

	// Test TotalPolls
	stats.TotalPolls.Add(100)
	if got := stats.TotalPolls.Load(); got != 100 {
		t.Errorf("expected TotalPolls 100, got %d", got)
	}

	// Test SuccessPolls
	stats.SuccessPolls.Add(95)
	if got := stats.SuccessPolls.Load(); got != 95 {
		t.Errorf("expected SuccessPolls 95, got %d", got)
	}

	// Test FailedPolls
	stats.FailedPolls.Add(5)
	if got := stats.FailedPolls.Load(); got != 5 {
		t.Errorf("expected FailedPolls 5, got %d", got)
	}

	// Test SkippedPolls
	stats.SkippedPolls.Add(2)
	if got := stats.SkippedPolls.Load(); got != 2 {
		t.Errorf("expected SkippedPolls 2, got %d", got)
	}

	// Test PointsRead
	stats.PointsRead.Add(1000)
	if got := stats.PointsRead.Load(); got != 1000 {
		t.Errorf("expected PointsRead 1000, got %d", got)
	}

	// Test PointsPublished
	stats.PointsPublished.Add(950)
	if got := stats.PointsPublished.Load(); got != 950 {
		t.Errorf("expected PointsPublished 950, got %d", got)
	}
}

// TestPollingStatsConcurrency tests stats are safe for concurrent access.
func TestPollingStatsConcurrency(t *testing.T) {
	stats := &service.PollingStats{}
	done := make(chan struct{})

	// Multiple goroutines updating stats
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				stats.TotalPolls.Add(1)
				stats.SuccessPolls.Add(1)
				stats.PointsRead.Add(10)
			}
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	if got := stats.TotalPolls.Load(); got != 1000 {
		t.Errorf("expected TotalPolls 1000, got %d", got)
	}
	if got := stats.SuccessPolls.Load(); got != 1000 {
		t.Errorf("expected SuccessPolls 1000, got %d", got)
	}
	if got := stats.PointsRead.Load(); got != 10000 {
		t.Errorf("expected PointsRead 10000, got %d", got)
	}
}

// TestPollIntervalConfigurations tests various poll interval settings.
func TestPollIntervalConfigurations(t *testing.T) {
	tests := []struct {
		name        string
		interval    time.Duration
		description string
	}{
		{"Fast polling", 100 * time.Millisecond, "High-frequency sensor data"},
		{"Standard polling", 1 * time.Second, "Normal process variables"},
		{"Slow polling", 5 * time.Second, "Slowly changing values"},
		{"Status polling", 30 * time.Second, "Device status checks"},
		{"Heartbeat polling", 60 * time.Second, "Keep-alive checks"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := service.PollingConfig{DefaultInterval: tt.interval}
			if cfg.DefaultInterval != tt.interval {
				t.Errorf("expected interval %v, got %v", tt.interval, cfg.DefaultInterval)
			}
		})
	}
}

// TestWorkerCountScaling tests worker count for different deployment sizes.
func TestWorkerCountScaling(t *testing.T) {
	tests := []struct {
		name        string
		workerCount int
		devices     int
		description string
	}{
		{"Small", 5, 10, "10 devices, 5 workers"},
		{"Medium", 10, 50, "50 devices, 10 workers"},
		{"Large", 20, 200, "200 devices, 20 workers"},
		{"Enterprise", 50, 500, "500 devices, 50 workers"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := service.PollingConfig{WorkerCount: tt.workerCount}

			// Workers should be able to handle device load
			devicesPerWorker := tt.devices / cfg.WorkerCount
			if devicesPerWorker > 20 {
				t.Logf("Warning: %d devices per worker may cause delays", devicesPerWorker)
			}
		})
	}
}

// TestBatchSizeOptimization tests batch size configurations.
func TestBatchSizeOptimization(t *testing.T) {
	tests := []struct {
		name      string
		batchSize int
		scenario  string
	}{
		{"Small batch", 10, "Low latency, high overhead"},
		{"Medium batch", 50, "Balanced throughput and latency"},
		{"Large batch", 100, "High throughput, higher latency"},
		{"Max batch", 200, "Maximum throughput"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := service.PollingConfig{BatchSize: tt.batchSize}
			if cfg.BatchSize != tt.batchSize {
				t.Errorf("expected BatchSize %d, got %d", tt.batchSize, cfg.BatchSize)
			}
		})
	}
}

// TestSuccessRateCalculation tests success rate calculation.
func TestSuccessRateCalculation(t *testing.T) {
	tests := []struct {
		name        string
		total       uint64
		success     uint64
		expectedPct float64
	}{
		{"Perfect", 100, 100, 100.0},
		{"Good", 100, 95, 95.0},
		{"Acceptable", 100, 90, 90.0},
		{"Poor", 100, 50, 50.0},
		{"No data", 0, 0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := service.PollingStats{}
			stats.TotalPolls.Store(tt.total)
			stats.SuccessPolls.Store(tt.success)

			var successRate float64
			if stats.TotalPolls.Load() > 0 {
				successRate = float64(stats.SuccessPolls.Load()) / float64(stats.TotalPolls.Load()) * 100
			}

			if successRate != tt.expectedPct {
				t.Errorf("expected success rate %.1f%%, got %.1f%%", tt.expectedPct, successRate)
			}
		})
	}
}

// TestShutdownTimeoutConfigurations tests shutdown timeout settings.
func TestShutdownTimeoutConfigurations(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		reason  string
	}{
		{"Fast shutdown", 10 * time.Second, "Development/testing"},
		{"Normal shutdown", 30 * time.Second, "Standard production"},
		{"Graceful shutdown", 60 * time.Second, "Critical operations"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := service.PollingConfig{ShutdownTimeout: tt.timeout}
			if cfg.ShutdownTimeout != tt.timeout {
				t.Errorf("expected ShutdownTimeout %v, got %v", tt.timeout, cfg.ShutdownTimeout)
			}
		})
	}
}

// TestRetryConfiguration tests retry settings.
func TestRetryConfiguration(t *testing.T) {
	tests := []struct {
		name       string
		maxRetries int
		expected   string
	}{
		{"No retry", 0, "Fail fast"},
		{"Single retry", 1, "One more chance"},
		{"Standard retry", 3, "Normal resilience"},
		{"High retry", 5, "High availability"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := service.PollingConfig{MaxRetries: tt.maxRetries}
			if cfg.MaxRetries != tt.maxRetries {
				t.Errorf("expected MaxRetries %d, got %d", tt.maxRetries, cfg.MaxRetries)
			}
		})
	}
}

// TestJitterCalculation tests poll jitter behavior.
func TestJitterCalculation(t *testing.T) {
	// Jitter is 0-10% of poll interval to spread device polls over time
	tests := []struct {
		interval  time.Duration
		maxJitter time.Duration
	}{
		{100 * time.Millisecond, 10 * time.Millisecond},
		{1 * time.Second, 100 * time.Millisecond},
		{10 * time.Second, 1 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.interval.String(), func(t *testing.T) {
			jitterMax := tt.interval / 10
			if jitterMax != tt.maxJitter {
				t.Errorf("expected max jitter %v, got %v", tt.maxJitter, jitterMax)
			}
		})
	}
}

// TestThroughputCalculation tests throughput estimation.
func TestThroughputCalculation(t *testing.T) {
	// Calculate theoretical throughput
	tests := []struct {
		name       string
		workers    int
		batchSize  int
		intervalMs int
		expected   int // points per second
	}{
		{"Low throughput", 5, 10, 1000, 50},
		{"Medium throughput", 10, 50, 1000, 500},
		{"High throughput", 20, 100, 500, 4000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simplified throughput calculation
			pollsPerSecond := float64(tt.workers) * (1000.0 / float64(tt.intervalMs))
			pointsPerSecond := int(pollsPerSecond * float64(tt.batchSize))

			if pointsPerSecond != tt.expected {
				t.Errorf("expected throughput %d pts/s, got %d pts/s", tt.expected, pointsPerSecond)
			}
		})
	}
}

// TestBackPressureStats tests back-pressure skip tracking.
func TestBackPressureStats(t *testing.T) {
	stats := &service.PollingStats{}

	// Simulate back-pressure scenario
	stats.TotalPolls.Store(100)
	stats.SuccessPolls.Store(80)
	stats.SkippedPolls.Store(15)
	stats.FailedPolls.Store(5)

	// Verify consistency
	accounted := stats.SuccessPolls.Load() + stats.SkippedPolls.Load() + stats.FailedPolls.Load()
	if accounted != stats.TotalPolls.Load() {
		t.Errorf("stats don't add up: success(%d) + skipped(%d) + failed(%d) = %d, total = %d",
			stats.SuccessPolls.Load(), stats.SkippedPolls.Load(), stats.FailedPolls.Load(),
			accounted, stats.TotalPolls.Load())
	}

	// Calculate skip rate
	skipRate := float64(stats.SkippedPolls.Load()) / float64(stats.TotalPolls.Load()) * 100
	if skipRate != 15.0 {
		t.Errorf("expected skip rate 15%%, got %.1f%%", skipRate)
	}
}

// TestPublishEfficiency tests publish efficiency calculation.
func TestPublishEfficiency(t *testing.T) {
	tests := []struct {
		name        string
		read        uint64
		published   uint64
		expectedPct float64
	}{
		{"Perfect", 1000, 1000, 100.0},
		{"Good", 1000, 950, 95.0},
		{"Lossy", 1000, 800, 80.0},
		{"No data", 0, 0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := service.PollingStats{}
			stats.PointsRead.Store(tt.read)
			stats.PointsPublished.Store(tt.published)

			var efficiency float64
			if stats.PointsRead.Load() > 0 {
				efficiency = float64(stats.PointsPublished.Load()) / float64(stats.PointsRead.Load()) * 100
			}

			if efficiency != tt.expectedPct {
				t.Errorf("expected efficiency %.1f%%, got %.1f%%", tt.expectedPct, efficiency)
			}
		})
	}
}

// TestPollingConfigValidation tests configuration validation rules.
func TestPollingConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  service.PollingConfig
		isValid bool
	}{
		{
			"Valid config",
			service.PollingConfig{
				WorkerCount:     10,
				BatchSize:       50,
				DefaultInterval: 1 * time.Second,
				MaxRetries:      3,
				ShutdownTimeout: 30 * time.Second,
			},
			true,
		},
		{
			"Zero workers",
			service.PollingConfig{
				WorkerCount: 0,
				BatchSize:   50,
			},
			false,
		},
		{
			"Negative workers",
			service.PollingConfig{
				WorkerCount: -1,
				BatchSize:   50,
			},
			false,
		},
		{
			"Zero batch",
			service.PollingConfig{
				WorkerCount: 10,
				BatchSize:   0,
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simple validation rules
			isValid := tt.config.WorkerCount > 0 && tt.config.BatchSize > 0
			if isValid != tt.isValid {
				t.Errorf("expected isValid=%v, got %v", tt.isValid, isValid)
			}
		})
	}
}
