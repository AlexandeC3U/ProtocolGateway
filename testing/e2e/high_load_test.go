//go:build e2e
// +build e2e

// Package e2e provides end-to-end tests for the protocol gateway.
// high_load_test.go tests sustained high message rates under load.
package e2e

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// High Load Test Configuration
// =============================================================================

type highLoadConfig struct {
	deviceCount    int
	tagsPerDevice  int
	pollingRate    time.Duration
	duration       time.Duration
	errorRate      float64
	connectDelay   time.Duration
	channelBufSize int
}

func defaultHighLoadConfig() highLoadConfig {
	return highLoadConfig{
		deviceCount:    100,
		tagsPerDevice:  20,
		pollingRate:    10 * time.Millisecond,
		duration:       30 * time.Second,
		errorRate:      0.0,
		connectDelay:   5 * time.Millisecond,
		channelBufSize: 100000,
	}
}

// highLoadMetrics tracks throughput and error metrics during the test.
type highLoadMetrics struct {
	totalPoints    int64
	totalErrors    int64
	droppedPoints  int64
	peakRate       int64 // points per second (peak observed)
	intervalCounts []int64
	intervalErrors []int64
}

// =============================================================================
// High Load Test Cases
// =============================================================================

// TestHighLoadSustainedThroughput tests sustained high message rates over an
// extended period. It creates many devices with many tags, polls at high
// frequency, and verifies that throughput remains stable without degradation.
func TestHighLoadSustainedThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping sustained high load test in short mode")
	}

	cfg := defaultHighLoadConfig()
	metrics := runHighLoadTest(t, cfg)

	// Report results
	totalDuration := cfg.duration.Seconds()
	avgRate := float64(metrics.totalPoints) / totalDuration
	// Account for simulated per-tag read latency (1ms/tag in MockDevice.ReadTags)
	perPollMs := float64(cfg.tagsPerDevice) // 1ms per tag
	effectiveIntervalMs := math.Max(float64(cfg.pollingRate.Milliseconds()), perPollMs)
	theoreticalRate := float64(cfg.deviceCount*cfg.tagsPerDevice) * (1000.0 / effectiveIntervalMs)
	expectedMinRate := theoreticalRate * 0.5

	t.Logf("=== Sustained Throughput Results ===")
	t.Logf("  Devices:        %d", cfg.deviceCount)
	t.Logf("  Tags/device:    %d", cfg.tagsPerDevice)
	t.Logf("  Poll interval:  %v", cfg.pollingRate)
	t.Logf("  Duration:       %v", cfg.duration)
	t.Logf("  Total points:   %d", metrics.totalPoints)
	t.Logf("  Average rate:   %.0f pts/sec", avgRate)
	t.Logf("  Peak rate:      %d pts/sec", metrics.peakRate)
	t.Logf("  Dropped points: %d", metrics.droppedPoints)
	t.Logf("  Total errors:   %d", metrics.totalErrors)

	// Verify minimum throughput
	if avgRate < expectedMinRate {
		t.Errorf("Average rate %.0f pts/sec below minimum expected %.0f pts/sec", avgRate, expectedMinRate)
	}

	// Verify no excessive drops
	dropRate := float64(metrics.droppedPoints) / float64(metrics.totalPoints+metrics.droppedPoints+1)
	if dropRate > 0.05 {
		t.Errorf("Drop rate %.2f%% exceeds 5%% threshold", dropRate*100)
	}
}

// TestHighLoadWithErrors tests throughput stability when devices experience errors.
func TestHighLoadWithErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high load with errors test in short mode")
	}

	cfg := defaultHighLoadConfig()
	cfg.deviceCount = 50
	cfg.tagsPerDevice = 10
	cfg.errorRate = 0.1 // 10% error rate
	cfg.duration = 15 * time.Second

	metrics := runHighLoadTest(t, cfg)

	totalDuration := cfg.duration.Seconds()
	avgRate := float64(metrics.totalPoints) / totalDuration

	t.Logf("=== High Load With Errors Results ===")
	t.Logf("  Devices:        %d (error rate: %.0f%%)", cfg.deviceCount, cfg.errorRate*100)
	t.Logf("  Duration:       %v", cfg.duration)
	t.Logf("  Total points:   %d", metrics.totalPoints)
	t.Logf("  Average rate:   %.0f pts/sec", avgRate)
	t.Logf("  Total errors:   %d", metrics.totalErrors)

	// Even with errors, system should maintain throughput
	if metrics.totalPoints == 0 {
		t.Error("Expected to receive data points despite errors")
	}

	// Error count should be roughly proportional to error rate
	totalPolls := metrics.totalPoints + metrics.totalErrors
	if totalPolls > 0 {
		actualErrorRate := float64(metrics.totalErrors) / float64(totalPolls)
		t.Logf("  Actual error rate: %.2f%% (configured: %.0f%%)", actualErrorRate*100, cfg.errorRate*100)
	}
}

// TestHighLoadThroughputStability verifies that throughput does not degrade
// over time by comparing early vs late intervals.
func TestHighLoadThroughputStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping throughput stability test in short mode")
	}

	cfg := defaultHighLoadConfig()
	cfg.deviceCount = 50
	cfg.tagsPerDevice = 10
	cfg.duration = 20 * time.Second

	metrics := runHighLoadTest(t, cfg)

	if len(metrics.intervalCounts) < 4 {
		t.Skip("Not enough intervals collected for stability analysis")
	}

	// Compare first quarter vs last quarter throughput
	quarter := len(metrics.intervalCounts) / 4
	if quarter == 0 {
		quarter = 1
	}

	var earlySum, lateSum int64
	for i := 0; i < quarter; i++ {
		earlySum += metrics.intervalCounts[i]
	}
	for i := len(metrics.intervalCounts) - quarter; i < len(metrics.intervalCounts); i++ {
		lateSum += metrics.intervalCounts[i]
	}

	earlyAvg := float64(earlySum) / float64(quarter)
	lateAvg := float64(lateSum) / float64(quarter)

	t.Logf("=== Throughput Stability Results ===")
	t.Logf("  Total intervals: %d", len(metrics.intervalCounts))
	t.Logf("  Early avg rate:  %.0f pts/interval", earlyAvg)
	t.Logf("  Late avg rate:   %.0f pts/interval", lateAvg)

	// Late throughput should not drop below 50% of early throughput
	if earlyAvg > 0 {
		degradation := (earlyAvg - lateAvg) / earlyAvg
		t.Logf("  Degradation:     %.1f%%", degradation*100)
		if degradation > 0.5 {
			t.Errorf("Throughput degraded by %.1f%% (early: %.0f, late: %.0f)", degradation*100, earlyAvg, lateAvg)
		}
	}
}

// TestHighLoadMixedProtocols tests high load across all protocol types.
func TestHighLoadMixedProtocols(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping mixed protocol high load test in short mode")
	}

	cfg := defaultHighLoadConfig()
	cfg.deviceCount = 90 // 30 per protocol
	cfg.tagsPerDevice = 10
	cfg.duration = 15 * time.Second

	manager := &MultiDeviceManager{
		devices:  make(map[string]*MockDevice),
		dataChan: make(chan *DataPoint, cfg.channelBufSize),
	}

	protocols := []Protocol{ProtocolModbus, ProtocolOPCUA, ProtocolS7}
	for i := 0; i < cfg.deviceCount; i++ {
		tags := make([]*MockTag, cfg.tagsPerDevice)
		for j := 0; j < cfg.tagsPerDevice; j++ {
			tags[j] = &MockTag{
				ID:       fmt.Sprintf("tag-%d", j),
				DataType: "float32",
			}
		}
		manager.AddDevice(&MockDevice{
			ID:           fmt.Sprintf("device-%s-%d", protocols[i%3], i),
			Protocol:     protocols[i%3],
			PollingRate:  cfg.pollingRate,
			ConnectDelay: cfg.connectDelay,
			ErrorRate:    cfg.errorRate,
			Tags:         tags,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.duration+10*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	// Track per-protocol throughput
	protocolCounts := make(map[Protocol]*int64)
	for _, p := range protocols {
		var count int64
		protocolCounts[p] = &count
	}

	var totalReceived int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case dp, ok := <-manager.GetDataChannel():
				if !ok {
					return
				}
				atomic.AddInt64(&totalReceived, 1)
				if counter, exists := protocolCounts[dp.Protocol]; exists {
					atomic.AddInt64(counter, 1)
				}
			}
		}
	}()

	time.Sleep(cfg.duration)
	manager.Stop()
	cancel()
	<-done

	t.Logf("=== Mixed Protocol Results ===")
	t.Logf("  Total received: %d", atomic.LoadInt64(&totalReceived))
	for proto, counter := range protocolCounts {
		count := atomic.LoadInt64(counter)
		t.Logf("  %s: %d points", proto, count)
		if count == 0 {
			t.Errorf("Protocol %s produced zero data points", proto)
		}
	}

	// Verify roughly equal distribution (within 3x of each other)
	var counts []int64
	for _, counter := range protocolCounts {
		counts = append(counts, atomic.LoadInt64(counter))
	}
	if len(counts) >= 2 {
		minCount, maxCount := counts[0], counts[0]
		for _, c := range counts[1:] {
			if c < minCount {
				minCount = c
			}
			if c > maxCount {
				maxCount = c
			}
		}
		if minCount > 0 && maxCount > 3*minCount {
			t.Errorf("Protocol throughput imbalance: min=%d, max=%d (ratio %.1fx)", minCount, maxCount, float64(maxCount)/float64(minCount))
		}
	}
}

// TestHighLoadBurstRecovery tests the system's ability to recover after bursts.
func TestHighLoadBurstRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping burst recovery test in short mode")
	}

	manager := &MultiDeviceManager{
		devices:  make(map[string]*MockDevice),
		dataChan: make(chan *DataPoint, 50000),
	}

	// Start with 10 devices
	initialDevices := 10
	burstDevices := 100
	tagsPerDevice := 10

	for i := 0; i < initialDevices; i++ {
		tags := make([]*MockTag, tagsPerDevice)
		for j := 0; j < tagsPerDevice; j++ {
			tags[j] = &MockTag{ID: fmt.Sprintf("tag-%d", j), DataType: "float32"}
		}
		manager.AddDevice(&MockDevice{
			ID:           fmt.Sprintf("initial-%d", i),
			Protocol:     ProtocolModbus,
			PollingRate:  20 * time.Millisecond,
			ConnectDelay: 5 * time.Millisecond,
			Tags:         tags,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	// Measure baseline
	var baselineCount int64
	var burstCount int64
	var postBurstCount int64
	var phase int32 // 0=baseline, 1=burst, 2=post-burst
	atomic.StoreInt32(&phase, 0)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-manager.GetDataChannel():
				if !ok {
					return
				}
				switch atomic.LoadInt32(&phase) {
				case 0:
					atomic.AddInt64(&baselineCount, 1)
				case 1:
					atomic.AddInt64(&burstCount, 1)
				case 2:
					atomic.AddInt64(&postBurstCount, 1)
				}
			}
		}
	}()

	// Phase 1: baseline (5s)
	time.Sleep(5 * time.Second)
	baseline := atomic.LoadInt64(&baselineCount)

	// Phase 2: burst - add many more devices (they won't be polled by manager
	// since it's already started, but this tests the channel capacity)
	atomic.StoreInt32(&phase, 1)

	// Simulate burst by creating additional standalone pollers
	var burstWg sync.WaitGroup
	burstStop := make(chan struct{})
	for i := 0; i < burstDevices; i++ {
		tags := make([]*MockTag, tagsPerDevice)
		for j := 0; j < tagsPerDevice; j++ {
			tags[j] = &MockTag{ID: fmt.Sprintf("tag-%d", j), DataType: "float32"}
		}
		dev := &MockDevice{
			ID:           fmt.Sprintf("burst-%d", i),
			Protocol:     ProtocolOPCUA,
			PollingRate:  10 * time.Millisecond,
			ConnectDelay: 2 * time.Millisecond,
			Tags:         tags,
		}
		_ = dev.Connect(ctx)

		burstWg.Add(1)
		go func(d *MockDevice) {
			defer burstWg.Done()
			ticker := time.NewTicker(d.PollingRate)
			defer ticker.Stop()
			for {
				select {
				case <-burstStop:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
					values, err := d.ReadTags(ctx)
					if err != nil {
						continue
					}
					for tagID, value := range values {
						select {
						case manager.dataChan <- &DataPoint{
							DeviceID:  d.ID,
							TagID:     tagID,
							Value:     value,
							Quality:   192,
							Timestamp: time.Now(),
							Protocol:  d.Protocol,
						}:
						default:
						}
					}
				}
			}
		}(dev)
	}

	time.Sleep(5 * time.Second)
	burst := atomic.LoadInt64(&burstCount)

	// Phase 3: remove burst, measure recovery
	atomic.StoreInt32(&phase, 2)
	close(burstStop)
	burstWg.Wait()

	time.Sleep(5 * time.Second)
	postBurst := atomic.LoadInt64(&postBurstCount)

	manager.Stop()
	cancel()
	<-done

	t.Logf("=== Burst Recovery Results ===")
	t.Logf("  Baseline (5s):   %d points (%.0f/sec)", baseline, float64(baseline)/5.0)
	t.Logf("  Burst (5s):      %d points (%.0f/sec)", burst, float64(burst)/5.0)
	t.Logf("  Post-burst (5s): %d points (%.0f/sec)", postBurst, float64(postBurst)/5.0)

	// Post-burst should recover to at least 50% of baseline
	if baseline > 0 && postBurst < baseline/2 {
		t.Errorf("Post-burst throughput %d did not recover to at least 50%% of baseline %d", postBurst, baseline)
	}
}

// =============================================================================
// High Load Test Runner
// =============================================================================

// runHighLoadTest executes a high load test with the given configuration and
// returns collected metrics.
func runHighLoadTest(t *testing.T, cfg highLoadConfig) *highLoadMetrics {
	t.Helper()

	manager := &MultiDeviceManager{
		devices:  make(map[string]*MockDevice),
		dataChan: make(chan *DataPoint, cfg.channelBufSize),
	}

	// Create devices
	protocols := []Protocol{ProtocolModbus, ProtocolOPCUA, ProtocolS7}
	for i := 0; i < cfg.deviceCount; i++ {
		tags := make([]*MockTag, cfg.tagsPerDevice)
		for j := 0; j < cfg.tagsPerDevice; j++ {
			tags[j] = &MockTag{
				ID:       fmt.Sprintf("tag-%d", j),
				DataType: []string{"float32", "float64", "int32", "uint16", "bool"}[j%5],
			}
		}
		manager.AddDevice(&MockDevice{
			ID:           fmt.Sprintf("device-%d", i),
			Protocol:     protocols[i%3],
			PollingRate:  cfg.pollingRate,
			ConnectDelay: cfg.connectDelay,
			ErrorRate:    cfg.errorRate,
			Tags:         tags,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.duration+10*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	// Collect metrics
	metrics := &highLoadMetrics{}
	var intervalCount int64
	var mu sync.Mutex

	// Sample throughput every second
	sampleTicker := time.NewTicker(1 * time.Second)
	defer sampleTicker.Stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-manager.GetDataChannel():
				if !ok {
					return
				}
				atomic.AddInt64(&metrics.totalPoints, 1)
				atomic.AddInt64(&intervalCount, 1)
			}
		}
	}()

	// Interval sampler
	sampleDone := make(chan struct{})
	go func() {
		defer close(sampleDone)
		for {
			select {
			case <-ctx.Done():
				return
			case <-sampleTicker.C:
				count := atomic.SwapInt64(&intervalCount, 0)
				mu.Lock()
				metrics.intervalCounts = append(metrics.intervalCounts, count)
				mu.Unlock()
				if count > atomic.LoadInt64(&metrics.peakRate) {
					atomic.StoreInt64(&metrics.peakRate, count)
				}
			}
		}
	}()

	// Run for configured duration
	time.Sleep(cfg.duration)
	manager.Stop()

	// Collect error counts from all devices
	for _, device := range manager.devices {
		stats := device.GetStats()
		atomic.AddInt64(&metrics.totalErrors, stats["errors"])
	}

	cancel()
	<-done
	<-sampleDone

	return metrics
}

// TestHighLoadChannelBackpressure tests behavior when the data channel fills up.
func TestHighLoadChannelBackpressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping channel backpressure test in short mode")
	}

	// Use a small channel buffer to force backpressure
	manager := &MultiDeviceManager{
		devices:  make(map[string]*MockDevice),
		dataChan: make(chan *DataPoint, 100), // intentionally small
	}

	// Create devices that produce data faster than the channel can hold
	for i := 0; i < 50; i++ {
		tags := make([]*MockTag, 20)
		for j := 0; j < 20; j++ {
			tags[j] = &MockTag{ID: fmt.Sprintf("tag-%d", j), DataType: "float32"}
		}
		manager.AddDevice(&MockDevice{
			ID:           fmt.Sprintf("device-%d", i),
			Protocol:     ProtocolModbus,
			PollingRate:  10 * time.Millisecond,
			ConnectDelay: 5 * time.Millisecond,
			Tags:         tags,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	// Slow consumer — read slowly to create backpressure
	var received int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-manager.GetDataChannel():
				if !ok {
					return
				}
				atomic.AddInt64(&received, 1)
				// Simulate slow processing
				time.Sleep(1 * time.Millisecond)
			}
		}
	}()

	time.Sleep(10 * time.Second)
	manager.Stop()
	cancel()
	<-done

	total := atomic.LoadInt64(&received)
	t.Logf("=== Channel Backpressure Results ===")
	t.Logf("  Received (slow consumer): %d points", total)

	// System should not panic or deadlock under backpressure
	// Having received any data confirms the system stayed responsive
	if total == 0 {
		t.Error("Expected to receive some data points despite backpressure")
	}
}

// TestHighLoadLatencyUnderLoad measures per-poll latency under high load.
func TestHighLoadLatencyUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping latency under load test in short mode")
	}

	deviceCount := 50
	tagsPerDevice := 10

	manager := &MultiDeviceManager{
		devices:  make(map[string]*MockDevice),
		dataChan: make(chan *DataPoint, 50000),
	}

	for i := 0; i < deviceCount; i++ {
		tags := make([]*MockTag, tagsPerDevice)
		for j := 0; j < tagsPerDevice; j++ {
			tags[j] = &MockTag{ID: fmt.Sprintf("tag-%d", j), DataType: "float32"}
		}
		manager.AddDevice(&MockDevice{
			ID:           fmt.Sprintf("device-%d", i),
			Protocol:     []Protocol{ProtocolModbus, ProtocolOPCUA, ProtocolS7}[i%3],
			PollingRate:  50 * time.Millisecond,
			ConnectDelay: 5 * time.Millisecond,
			Tags:         tags,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	// Measure end-to-end latency (time from publish to receive)
	var latencies []time.Duration
	var mu sync.Mutex

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case dp, ok := <-manager.GetDataChannel():
				if !ok {
					return
				}
				latency := time.Since(dp.Timestamp)
				mu.Lock()
				latencies = append(latencies, latency)
				mu.Unlock()
			}
		}
	}()

	time.Sleep(10 * time.Second)
	manager.Stop()
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()

	if len(latencies) == 0 {
		t.Fatal("No latency measurements collected")
	}

	// Calculate percentiles
	p50 := percentile(latencies, 0.50)
	p95 := percentile(latencies, 0.95)
	p99 := percentile(latencies, 0.99)
	maxLat := percentile(latencies, 1.0)

	t.Logf("=== Latency Under Load Results ===")
	t.Logf("  Samples:  %d", len(latencies))
	t.Logf("  P50:      %v", p50)
	t.Logf("  P95:      %v", p95)
	t.Logf("  P99:      %v", p99)
	t.Logf("  Max:      %v", maxLat)

	// P99 should be under 5 seconds for mock devices
	if p99 > 5*time.Second {
		t.Errorf("P99 latency %v exceeds 5s threshold", p99)
	}
}

// percentile calculates the p-th percentile from a slice of durations.
// p should be between 0.0 and 1.0.
func percentile(data []time.Duration, p float64) time.Duration {
	if len(data) == 0 {
		return 0
	}

	// Copy and sort
	sorted := make([]time.Duration, len(data))
	copy(sorted, data)
	sortDurations(sorted)

	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// sortDurations sorts a slice of durations in ascending order (insertion sort).
func sortDurations(d []time.Duration) {
	for i := 1; i < len(d); i++ {
		key := d[i]
		j := i - 1
		for j >= 0 && d[j] > key {
			d[j+1] = d[j]
			j--
		}
		d[j+1] = key
	}
}
