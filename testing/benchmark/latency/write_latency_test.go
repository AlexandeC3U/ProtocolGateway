//go:build benchmark
// +build benchmark

// Package latency provides benchmark tests for write operation latency measurement.
package latency

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Write Latency Benchmarks
// =============================================================================

// BenchmarkWriteLatency_Simulated benchmarks simulated write operations.
func BenchmarkWriteLatency_Simulated(b *testing.B) {
	scenarios := []struct {
		name        string
		baseLatency time.Duration
		jitter      time.Duration
	}{
		{"Fast_NoJitter", 100 * time.Microsecond, 0},
		{"Fast_LowJitter", 100 * time.Microsecond, 50 * time.Microsecond},
		{"Medium_NoJitter", 1 * time.Millisecond, 0},
		{"Medium_MediumJitter", 1 * time.Millisecond, 500 * time.Microsecond},
		{"Slow_NoJitter", 10 * time.Millisecond, 0},
		{"Slow_HighJitter", 10 * time.Millisecond, 5 * time.Millisecond},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				simulateWrite(sc.baseLatency, sc.jitter)
			}
		})
	}
}

// simulateWrite simulates a write operation with configurable latency.
func simulateWrite(baseLatency time.Duration, jitter time.Duration) time.Duration {
	start := time.Now()

	// Simulate write work (typically slightly slower than reads due to confirmation)
	sleepDuration := baseLatency
	if jitter > 0 {
		ns := time.Now().UnixNano() % int64(jitter)
		sleepDuration += time.Duration(ns)
	}
	time.Sleep(sleepDuration)

	return time.Since(start)
}

// =============================================================================
// Write Latency Stats Collection
// =============================================================================

// WriteLatencyStats holds write latency statistics.
type WriteLatencyStats struct {
	Count    int
	Min      time.Duration
	Max      time.Duration
	Mean     time.Duration
	Median   time.Duration
	P90      time.Duration
	P95      time.Duration
	P99      time.Duration
	StdDev   time.Duration
	Timeouts int
}

// calculateWriteStats computes write latency statistics from samples.
func calculateWriteStats(samples []time.Duration) WriteLatencyStats {
	if len(samples) == 0 {
		return WriteLatencyStats{}
	}

	sorted := make([]time.Duration, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	stats := WriteLatencyStats{
		Count:  len(samples),
		Min:    sorted[0],
		Max:    sorted[len(sorted)-1],
		Median: sorted[len(sorted)/2],
		P90:    sorted[int(float64(len(sorted))*0.9)],
		P95:    sorted[int(float64(len(sorted))*0.95)],
		P99:    sorted[int(float64(len(sorted))*0.99)],
	}

	var total time.Duration
	for _, s := range samples {
		total += s
	}
	stats.Mean = total / time.Duration(len(samples))

	var variance float64
	meanNs := float64(stats.Mean.Nanoseconds())
	for _, s := range samples {
		diff := float64(s.Nanoseconds()) - meanNs
		variance += diff * diff
	}
	variance /= float64(len(samples))
	stats.StdDev = time.Duration(math.Sqrt(variance))

	return stats
}

// =============================================================================
// Protocol Write Latency Tests
// =============================================================================

// TestWriteLatency_Modbus tests Modbus write latency characteristics.
func TestWriteLatency_Modbus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping latency test in short mode")
	}

	iterations := 100
	samples := make([]time.Duration, iterations)

	// Simulate Modbus write operations
	for i := 0; i < iterations; i++ {
		// Modbus writes are typically 5-20ms
		samples[i] = simulateWrite(8*time.Millisecond, 4*time.Millisecond)
	}

	stats := calculateWriteStats(samples)

	t.Logf("Modbus Write Latency Statistics (n=%d):", stats.Count)
	t.Logf("  Min:    %v", stats.Min)
	t.Logf("  Max:    %v", stats.Max)
	t.Logf("  Mean:   %v", stats.Mean)
	t.Logf("  Median: %v", stats.Median)
	t.Logf("  P90:    %v", stats.P90)
	t.Logf("  P95:    %v", stats.P95)
	t.Logf("  P99:    %v", stats.P99)
	t.Logf("  StdDev: %v", stats.StdDev)

	// Assertions for reasonable latency bounds
	if stats.P99 > 50*time.Millisecond {
		t.Errorf("P99 latency too high: %v (expected < 50ms)", stats.P99)
	}
}

// TestWriteLatency_OPCUA tests OPC UA write latency characteristics.
func TestWriteLatency_OPCUA(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping latency test in short mode")
	}

	iterations := 100
	samples := make([]time.Duration, iterations)

	// Simulate OPC UA write operations (typically faster than Modbus)
	for i := 0; i < iterations; i++ {
		samples[i] = simulateWrite(3*time.Millisecond, 2*time.Millisecond)
	}

	stats := calculateWriteStats(samples)

	t.Logf("OPC UA Write Latency Statistics (n=%d):", stats.Count)
	t.Logf("  Min:    %v", stats.Min)
	t.Logf("  Max:    %v", stats.Max)
	t.Logf("  Mean:   %v", stats.Mean)
	t.Logf("  Median: %v", stats.Median)
	t.Logf("  P90:    %v", stats.P90)
	t.Logf("  P95:    %v", stats.P95)
	t.Logf("  P99:    %v", stats.P99)
	t.Logf("  StdDev: %v", stats.StdDev)

	if stats.P99 > 50*time.Millisecond {
		t.Errorf("P99 latency too high: %v (expected < 50ms)", stats.P99)
	}
}

// TestWriteLatency_S7 tests S7 write latency characteristics.
func TestWriteLatency_S7(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping latency test in short mode")
	}

	iterations := 100
	samples := make([]time.Duration, iterations)

	// Simulate S7 write operations
	for i := 0; i < iterations; i++ {
		samples[i] = simulateWrite(5*time.Millisecond, 3*time.Millisecond)
	}

	stats := calculateWriteStats(samples)

	t.Logf("S7 Write Latency Statistics (n=%d):", stats.Count)
	t.Logf("  Min:    %v", stats.Min)
	t.Logf("  Max:    %v", stats.Max)
	t.Logf("  Mean:   %v", stats.Mean)
	t.Logf("  Median: %v", stats.Median)
	t.Logf("  P90:    %v", stats.P90)
	t.Logf("  P95:    %v", stats.P95)
	t.Logf("  P99:    %v", stats.P99)
	t.Logf("  StdDev: %v", stats.StdDev)

	if stats.P99 > 40*time.Millisecond {
		t.Errorf("P99 latency too high: %v (expected < 40ms)", stats.P99)
	}
}

// =============================================================================
// Concurrent Write Latency Tests
// =============================================================================

// TestWriteLatency_Concurrent tests write latency under concurrent load.
func TestWriteLatency_Concurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent latency test in short mode")
	}

	concurrencyLevels := []int{1, 2, 4, 8, 16}
	iterationsPerWorker := 50

	for _, concurrency := range concurrencyLevels {
		t.Run(fmt.Sprintf("%d_workers", concurrency), func(t *testing.T) {
			testConcurrentWriteLatency(t, concurrency, iterationsPerWorker)
		})
	}
}

func testConcurrentWriteLatency(t *testing.T, concurrency, iterations int) {
	var mu sync.Mutex
	samples := make([]time.Duration, 0, concurrency*iterations)

	var wg sync.WaitGroup
	wg.Add(concurrency)

	for w := 0; w < concurrency; w++ {
		go func() {
			defer wg.Done()
			localSamples := make([]time.Duration, iterations)
			for i := 0; i < iterations; i++ {
				localSamples[i] = simulateWrite(5*time.Millisecond, 2*time.Millisecond)
			}
			mu.Lock()
			samples = append(samples, localSamples...)
			mu.Unlock()
		}()
	}

	wg.Wait()

	stats := calculateWriteStats(samples)

	t.Logf("Concurrent Write Latency (workers=%d, n=%d):", concurrency, stats.Count)
	t.Logf("  Mean:   %v", stats.Mean)
	t.Logf("  P95:    %v", stats.P95)
	t.Logf("  P99:    %v", stats.P99)
}

// =============================================================================
// Batch Write Latency Tests
// =============================================================================

// TestWriteLatency_BatchSizes tests write latency for different batch sizes.
func TestWriteLatency_BatchSizes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping batch latency test in short mode")
	}

	batchSizes := []int{1, 5, 10, 20, 50}
	iterations := 50

	for _, batchSize := range batchSizes {
		t.Run(fmt.Sprintf("batch_%d", batchSize), func(t *testing.T) {
			samples := make([]time.Duration, iterations)

			for i := 0; i < iterations; i++ {
				start := time.Now()
				// Simulate batch write (total time increases with batch size)
				for j := 0; j < batchSize; j++ {
					simulateWrite(1*time.Millisecond, 500*time.Microsecond)
				}
				samples[i] = time.Since(start)
			}

			stats := calculateWriteStats(samples)

			t.Logf("Batch Write Latency (size=%d, n=%d):", batchSize, stats.Count)
			t.Logf("  Mean:       %v", stats.Mean)
			t.Logf("  Per-item:   %v", stats.Mean/time.Duration(batchSize))
			t.Logf("  P95:        %v", stats.P95)
		})
	}
}

// =============================================================================
// Write vs Read Latency Comparison
// =============================================================================

// TestWriteLatency_CompareToRead compares write and read latencies.
func TestWriteLatency_CompareToRead(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping comparison test in short mode")
	}

	iterations := 100

	// Collect read latencies
	readSamples := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		readSamples[i] = simulateRead(3*time.Millisecond, 1*time.Millisecond)
	}
	readStats := calculateWriteStats(readSamples)

	// Collect write latencies (typically 10-20% slower due to confirmation)
	writeSamples := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		writeSamples[i] = simulateWrite(4*time.Millisecond, 1*time.Millisecond)
	}
	writeStats := calculateWriteStats(writeSamples)

	t.Logf("Read vs Write Latency Comparison (n=%d):", iterations)
	t.Logf("  Read  - Mean: %v, P95: %v", readStats.Mean, readStats.P95)
	t.Logf("  Write - Mean: %v, P95: %v", writeStats.Mean, writeStats.P95)

	writeOverhead := float64(writeStats.Mean) / float64(readStats.Mean)
	t.Logf("  Write overhead: %.1f%%", (writeOverhead-1)*100)

	// Writes should not be more than 2x slower than reads
	if writeOverhead > 2.0 {
		t.Errorf("Write overhead too high: %.1f%% (expected < 100%%)", (writeOverhead-1)*100)
	}
}

// =============================================================================
// Write Confirmation Latency
// =============================================================================

// TestWriteLatency_Confirmation tests write with confirmation latency.
func TestWriteLatency_Confirmation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping confirmation latency test in short mode")
	}

	iterations := 100

	// Write without waiting for confirmation
	noConfirmSamples := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		noConfirmSamples[i] = simulateWrite(2*time.Millisecond, 1*time.Millisecond)
	}
	noConfirmStats := calculateWriteStats(noConfirmSamples)

	// Write with confirmation (simulated by longer latency)
	withConfirmSamples := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		withConfirmSamples[i] = simulateWrite(5*time.Millisecond, 2*time.Millisecond)
	}
	withConfirmStats := calculateWriteStats(withConfirmSamples)

	t.Logf("Write Confirmation Latency (n=%d):", iterations)
	t.Logf("  No confirm  - Mean: %v, P95: %v", noConfirmStats.Mean, noConfirmStats.P95)
	t.Logf("  With confirm - Mean: %v, P95: %v", withConfirmStats.Mean, withConfirmStats.P95)

	confirmOverhead := withConfirmStats.Mean - noConfirmStats.Mean
	t.Logf("  Confirmation overhead: %v", confirmOverhead)
}

// Note: simulateRead is defined in read_latency_test.go in the same package
