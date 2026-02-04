//go:build benchmark
// +build benchmark

// Package latency provides benchmark tests for read operation latency measurement.
package latency

import (
	"context"
	"math"
	"sort"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Latency Measurement Types
// =============================================================================

// LatencyStats holds latency statistics.
type LatencyStats struct {
	Count   int
	Min     time.Duration
	Max     time.Duration
	Mean    time.Duration
	Median  time.Duration
	P90     time.Duration
	P95     time.Duration
	P99     time.Duration
	StdDev  time.Duration
	Samples []time.Duration
}

// calculateStats computes latency statistics from samples.
func calculateStats(samples []time.Duration) LatencyStats {
	if len(samples) == 0 {
		return LatencyStats{}
	}

	// Sort for percentile calculation
	sorted := make([]time.Duration, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	stats := LatencyStats{
		Count:   len(samples),
		Min:     sorted[0],
		Max:     sorted[len(sorted)-1],
		Median:  sorted[len(sorted)/2],
		P90:     sorted[int(float64(len(sorted))*0.9)],
		P95:     sorted[int(float64(len(sorted))*0.95)],
		P99:     sorted[int(float64(len(sorted))*0.99)],
		Samples: samples,
	}

	// Calculate mean
	var total time.Duration
	for _, s := range samples {
		total += s
	}
	stats.Mean = total / time.Duration(len(samples))

	// Calculate standard deviation
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
// Simulated Read Operation
// =============================================================================

// simulateRead simulates a read operation with configurable latency.
func simulateRead(baseLatency time.Duration, jitter time.Duration) time.Duration {
	// Add some variability to simulate real-world conditions
	start := time.Now()

	// Simulate work
	sleepDuration := baseLatency
	if jitter > 0 {
		// Add random-ish jitter based on current nanosecond
		ns := time.Now().UnixNano() % int64(jitter)
		sleepDuration += time.Duration(ns)
	}
	time.Sleep(sleepDuration)

	return time.Since(start)
}

// =============================================================================
// Protocol Allocation Simulation
// =============================================================================

// ProtocolAllocConfig defines allocation patterns for a protocol.
type ProtocolAllocConfig struct {
	Name           string
	BaseLatency    time.Duration
	Jitter         time.Duration
	RequestBuffer  int // Size of request frame buffer
	ResponseBuffer int // Size of response buffer
	ExtraAllocs    int // Number of additional small allocations
	ExtraAllocSize int // Size of each extra allocation
	MapAllocs      int // Number of map entries (for OPC UA attributes, etc.)
}

// Sink variables to prevent compiler optimization
var (
	sinkBytes  []byte
	sinkMap    map[string]interface{}
	sinkSlice  []interface{}
	sinkString string
)

// simulateProtocolRead simulates a read with realistic allocation patterns.
func simulateProtocolRead(cfg ProtocolAllocConfig, tagCount int) time.Duration {
	start := time.Now()

	// Request buffer allocation (frame construction)
	reqSize := cfg.RequestBuffer + (tagCount * 4) // 4 bytes per tag address
	sinkBytes = make([]byte, reqSize)

	// Response buffer allocation
	respSize := cfg.ResponseBuffer + (tagCount * 8) // 8 bytes per tag value
	sinkBytes = make([]byte, respSize)

	// Protocol-specific allocations
	for i := 0; i < cfg.ExtraAllocs; i++ {
		sinkBytes = make([]byte, cfg.ExtraAllocSize)
	}

	// Map allocations (attribute maps, node info, etc.)
	if cfg.MapAllocs > 0 {
		sinkMap = make(map[string]interface{}, cfg.MapAllocs)
		for i := 0; i < cfg.MapAllocs; i++ {
			sinkMap[itoa(i)] = i
		}
	}

	// Simulate network latency
	sleepDuration := cfg.BaseLatency
	if cfg.Jitter > 0 {
		ns := time.Now().UnixNano() % int64(cfg.Jitter)
		sleepDuration += time.Duration(ns)
	}
	time.Sleep(sleepDuration)

	return time.Since(start)
}

// getProtocolConfig returns realistic allocation config for each protocol.
func getProtocolConfig(protocol string) ProtocolAllocConfig {
	switch protocol {
	case "Modbus_TCP":
		// Modbus: Simple binary protocol
		// - 8-byte request header + 2 bytes per register address
		// - 8-byte response header + 2 bytes per register value
		// - Minimal extra allocations
		return ProtocolAllocConfig{
			Name:           "Modbus_TCP",
			BaseLatency:    500 * time.Microsecond,
			Jitter:         100 * time.Microsecond,
			RequestBuffer:  12,  // ADU header + function code
			ResponseBuffer: 256, // Max 125 registers * 2 bytes
			ExtraAllocs:    1,   // Typically just one working buffer
			ExtraAllocSize: 32,
			MapAllocs:      0, // No map allocations
		}

	case "OPC_UA":
		// OPC UA: Complex protocol with rich type system
		// - Binary/XML encoding overhead
		// - NodeID objects, Variant boxing
		// - Security context, session info
		// - Attribute maps for each node
		return ProtocolAllocConfig{
			Name:           "OPC_UA",
			BaseLatency:    2 * time.Millisecond,
			Jitter:         500 * time.Microsecond,
			RequestBuffer:  256,  // ReadRequest encoding
			ResponseBuffer: 1024, // ReadResponse with Variants
			ExtraAllocs:    5,    // NodeID, Variant, DataValue, timestamps, status
			ExtraAllocSize: 64,
			MapAllocs:      10, // Attribute map per node
		}

	case "S7comm":
		// S7: Siemens proprietary protocol
		// - PDU with TPKT/COTP headers
		// - Data block addressing
		// - Optimized but still needs buffers
		return ProtocolAllocConfig{
			Name:           "S7comm",
			BaseLatency:    1 * time.Millisecond,
			Jitter:         200 * time.Microsecond,
			RequestBuffer:  32,  // TPKT + COTP + S7 header
			ResponseBuffer: 480, // Max PDU size
			ExtraAllocs:    2,   // Address parsing, type conversion
			ExtraAllocSize: 48,
			MapAllocs:      0, // No map allocations
		}

	default:
		return ProtocolAllocConfig{
			Name:        protocol,
			BaseLatency: time.Millisecond,
		}
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

// BenchmarkReadLatency_Baseline measures baseline read latency.
func BenchmarkReadLatency_Baseline(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = simulateRead(100*time.Microsecond, 10*time.Microsecond)
	}
}

// BenchmarkReadLatency_SingleRegister measures single register read latency.
func BenchmarkReadLatency_SingleRegister(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Simulate single register read (typically 0.5-2ms local, 5-50ms remote)
		_ = simulateRead(500*time.Microsecond, 100*time.Microsecond)
	}
}

// BenchmarkReadLatency_MultiRegister measures multi-register read latency.
func BenchmarkReadLatency_MultiRegister(b *testing.B) {
	registers := []int{1, 10, 50, 100, 125}

	for _, count := range registers {
		b.Run("", func(b *testing.B) {
			b.ReportAllocs()

			// More registers = slightly more latency
			baseLatency := time.Duration(500+count*5) * time.Microsecond

			for i := 0; i < b.N; i++ {
				_ = simulateRead(baseLatency, 50*time.Microsecond)
			}
		})
	}
}

// BenchmarkReadLatency_Concurrent measures concurrent read latency.
func BenchmarkReadLatency_Concurrent(b *testing.B) {
	concurrencies := []int{1, 2, 4, 8, 16}

	for _, c := range concurrencies {
		b.Run("", func(b *testing.B) {
			b.ReportAllocs()
			b.SetParallelism(c)

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_ = simulateRead(500*time.Microsecond, 100*time.Microsecond)
				}
			})
		})
	}
}

// =============================================================================
// Latency Profile Tests
// =============================================================================

// TestLatencyProfile_LocalNetwork tests latency profile for local network.
func TestLatencyProfile_LocalNetwork(t *testing.T) {
	iterations := 1000
	samples := make([]time.Duration, iterations)

	// Simulate local network (sub-millisecond)
	for i := 0; i < iterations; i++ {
		samples[i] = simulateRead(200*time.Microsecond, 50*time.Microsecond)
	}

	stats := calculateStats(samples)

	t.Logf("Local Network Latency Profile (%d samples):", stats.Count)
	t.Logf("  Min:    %v", stats.Min)
	t.Logf("  Max:    %v", stats.Max)
	t.Logf("  Mean:   %v", stats.Mean)
	t.Logf("  Median: %v", stats.Median)
	t.Logf("  P90:    %v", stats.P90)
	t.Logf("  P95:    %v", stats.P95)
	t.Logf("  P99:    %v", stats.P99)
	t.Logf("  StdDev: %v", stats.StdDev)

	// Assert reasonable bounds
	if stats.P95 > 1*time.Millisecond {
		t.Logf("Warning: P95 latency %v exceeds 1ms for local network", stats.P95)
	}
}

// TestLatencyProfile_RemoteNetwork tests latency profile for remote network.
func TestLatencyProfile_RemoteNetwork(t *testing.T) {
	iterations := 100 // Fewer iterations for slower network
	samples := make([]time.Duration, iterations)

	// Simulate remote network (5-50ms typical)
	for i := 0; i < iterations; i++ {
		samples[i] = simulateRead(10*time.Millisecond, 5*time.Millisecond)
	}

	stats := calculateStats(samples)

	t.Logf("Remote Network Latency Profile (%d samples):", stats.Count)
	t.Logf("  Min:    %v", stats.Min)
	t.Logf("  Max:    %v", stats.Max)
	t.Logf("  Mean:   %v", stats.Mean)
	t.Logf("  Median: %v", stats.Median)
	t.Logf("  P90:    %v", stats.P90)
	t.Logf("  P95:    %v", stats.P95)
	t.Logf("  P99:    %v", stats.P99)
	t.Logf("  StdDev: %v", stats.StdDev)
}

// TestLatencyProfile_HighLatencyPath tests latency for high-latency paths.
func TestLatencyProfile_HighLatencyPath(t *testing.T) {
	iterations := 50
	samples := make([]time.Duration, iterations)

	// Simulate high-latency path (50-200ms, e.g., satellite or VPN)
	for i := 0; i < iterations; i++ {
		samples[i] = simulateRead(100*time.Millisecond, 50*time.Millisecond)
	}

	stats := calculateStats(samples)

	t.Logf("High-Latency Path Profile (%d samples):", stats.Count)
	t.Logf("  Min:    %v", stats.Min)
	t.Logf("  Max:    %v", stats.Max)
	t.Logf("  Mean:   %v", stats.Mean)
	t.Logf("  P99:    %v", stats.P99)
}

// =============================================================================
// Timeout Impact Tests
// =============================================================================

// TestTimeoutImpactOnLatency tests how timeout settings affect perceived latency.
func TestTimeoutImpactOnLatency(t *testing.T) {
	timeouts := []time.Duration{
		100 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
	}

	for _, timeout := range timeouts {
		t.Run(timeout.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			start := time.Now()

			// Simulate operation that might timeout
			select {
			case <-ctx.Done():
				elapsed := time.Since(start)
				t.Logf("Timeout after %v (configured: %v)", elapsed, timeout)
			case <-time.After(50 * time.Millisecond):
				elapsed := time.Since(start)
				t.Logf("Completed in %v (timeout: %v)", elapsed, timeout)
			}
		})
	}
}

// =============================================================================
// Latency Under Load Tests
// =============================================================================

// TestLatencyUnderLoad tests how latency changes under increasing load.
func TestLatencyUnderLoad(t *testing.T) {
	loadLevels := []struct {
		name        string
		goroutines  int
		readsPerSec int
	}{
		{"Light", 1, 10},
		{"Medium", 5, 50},
		{"Heavy", 10, 100},
		{"Extreme", 20, 200},
	}

	for _, load := range loadLevels {
		t.Run(load.name, func(t *testing.T) {
			var wg sync.WaitGroup
			samples := make([]time.Duration, 0, load.readsPerSec)
			var mu sync.Mutex

			interval := time.Second / time.Duration(load.readsPerSec)
			deadline := time.Now().Add(time.Second)

			for i := 0; i < load.goroutines; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for time.Now().Before(deadline) {
						latency := simulateRead(500*time.Microsecond, 100*time.Microsecond)
						mu.Lock()
						samples = append(samples, latency)
						mu.Unlock()
						time.Sleep(interval / time.Duration(load.goroutines))
					}
				}()
			}

			wg.Wait()

			if len(samples) > 0 {
				stats := calculateStats(samples)
				t.Logf("Load: %s (%d goroutines, %d target reads/sec)",
					load.name, load.goroutines, load.readsPerSec)
				t.Logf("  Actual reads: %d", stats.Count)
				t.Logf("  Mean latency: %v", stats.Mean)
				t.Logf("  P95 latency:  %v", stats.P95)
			}
		})
	}
}

// =============================================================================
// Protocol Comparison Tests
// =============================================================================

// TestLatencyByProtocol compares latency characteristics across protocols.
func TestLatencyByProtocol(t *testing.T) {
	protocols := []struct {
		name        string
		baseLatency time.Duration
		jitter      time.Duration
		description string
	}{
		{"Modbus_TCP", 500 * time.Microsecond, 100 * time.Microsecond, "Simple protocol, low overhead"},
		{"OPC_UA", 2 * time.Millisecond, 500 * time.Microsecond, "Rich protocol, more overhead"},
		{"S7", 1 * time.Millisecond, 200 * time.Microsecond, "Proprietary, optimized for Siemens"},
	}

	iterations := 100

	for _, proto := range protocols {
		t.Run(proto.name, func(t *testing.T) {
			samples := make([]time.Duration, iterations)

			for i := 0; i < iterations; i++ {
				samples[i] = simulateRead(proto.baseLatency, proto.jitter)
			}

			stats := calculateStats(samples)
			t.Logf("%s: %s", proto.name, proto.description)
			t.Logf("  Mean: %v, P95: %v, P99: %v", stats.Mean, stats.P95, stats.P99)
		})
	}
}

// =============================================================================
// Jitter Analysis Tests
// =============================================================================

// TestJitterAnalysis analyzes latency jitter characteristics.
func TestJitterAnalysis(t *testing.T) {
	iterations := 500
	samples := make([]time.Duration, iterations)

	baseLatency := 1 * time.Millisecond
	jitter := 500 * time.Microsecond

	for i := 0; i < iterations; i++ {
		samples[i] = simulateRead(baseLatency, jitter)
	}

	stats := calculateStats(samples)

	// Jitter is typically measured as the difference between max and min
	// or as standard deviation
	jitterRange := stats.Max - stats.Min

	t.Logf("Jitter Analysis (%d samples):", stats.Count)
	t.Logf("  Jitter Range: %v (Max - Min)", jitterRange)
	t.Logf("  StdDev:       %v", stats.StdDev)
	t.Logf("  P99 - P90:    %v", stats.P99-stats.P90)

	// Calculate coefficient of variation (CV = StdDev / Mean)
	cv := float64(stats.StdDev) / float64(stats.Mean) * 100
	t.Logf("  Coefficient of Variation: %.2f%%", cv)

	if cv > 50 {
		t.Logf("Warning: High jitter (CV > 50%%) may indicate network issues")
	}
}

// =============================================================================
// Latency Histogram Tests
// =============================================================================

// TestLatencyHistogram creates a latency distribution histogram.
func TestLatencyHistogram(t *testing.T) {
	iterations := 1000
	samples := make([]time.Duration, iterations)

	for i := 0; i < iterations; i++ {
		samples[i] = simulateRead(1*time.Millisecond, 500*time.Microsecond)
	}

	// Create histogram buckets
	buckets := []struct {
		max   time.Duration
		label string
		count int
	}{
		{500 * time.Microsecond, "<0.5ms", 0},
		{1 * time.Millisecond, "0.5-1ms", 0},
		{2 * time.Millisecond, "1-2ms", 0},
		{5 * time.Millisecond, "2-5ms", 0},
		{10 * time.Millisecond, "5-10ms", 0},
		{0, ">10ms", 0}, // 0 means infinity
	}

	for _, sample := range samples {
		for i := range buckets {
			if buckets[i].max == 0 || sample <= buckets[i].max {
				buckets[i].count++
				break
			}
		}
	}

	t.Log("Latency Histogram:")
	for _, bucket := range buckets {
		pct := float64(bucket.count) / float64(len(samples)) * 100
		bar := ""
		for j := 0; j < int(pct/2); j++ {
			bar += "█"
		}
		t.Logf("  %8s: %4d (%5.1f%%) %s", bucket.label, bucket.count, pct, bar)
	}
}

// =============================================================================
// SLA Compliance Tests
// =============================================================================

// TestSLACompliance tests latency against SLA targets.
func TestSLACompliance(t *testing.T) {
	slaTargets := []struct {
		name      string
		p95Target time.Duration
		p99Target time.Duration
	}{
		{"Realtime", 10 * time.Millisecond, 50 * time.Millisecond},
		{"Standard", 100 * time.Millisecond, 500 * time.Millisecond},
		{"Relaxed", 500 * time.Millisecond, 2 * time.Second},
	}

	iterations := 100
	samples := make([]time.Duration, iterations)

	for i := 0; i < iterations; i++ {
		samples[i] = simulateRead(5*time.Millisecond, 3*time.Millisecond)
	}

	stats := calculateStats(samples)

	for _, sla := range slaTargets {
		t.Run(sla.name, func(t *testing.T) {
			p95Pass := stats.P95 <= sla.p95Target
			p99Pass := stats.P99 <= sla.p99Target

			status := "✓ PASS"
			if !p95Pass || !p99Pass {
				status = "✗ FAIL"
			}

			t.Logf("%s SLA %s", sla.name, status)
			t.Logf("  P95: %v (target: %v) %s", stats.P95, sla.p95Target, passSymbol(p95Pass))
			t.Logf("  P99: %v (target: %v) %s", stats.P99, sla.p99Target, passSymbol(p99Pass))
		})
	}
}

func passSymbol(pass bool) string {
	if pass {
		return "✓"
	}
	return "✗"
}

// =============================================================================
// Theoretical Protocol Comparison Benchmarks
// =============================================================================
//
// IMPORTANT: These benchmarks use SIMULATED values, not actual protocol code.
// They model typical allocation patterns and latencies based on protocol
// characteristics, but do NOT call real adapter implementations.
//
// Use these for:
// - Understanding relative protocol overhead (memory, allocations)
// - Baseline comparisons between protocols
// - GC pressure estimation
//
// Do NOT use these for:
// - Actual performance measurements
// - SLA validation
// - Production capacity planning
//
// For real measurements, use integration benchmarks with actual simulators.
// =============================================================================

// BenchmarkProtocolComparison_Theoretical compares theoretical latency across protocols.
// Values are estimates based on typical protocol characteristics, not measurements.
func BenchmarkProtocolComparison_Theoretical(b *testing.B) {
	protocols := []string{"Modbus_TCP", "OPC_UA", "S7comm"}

	for _, proto := range protocols {
		cfg := getProtocolConfig(proto)
		b.Run(proto, func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = simulateProtocolRead(cfg, 1)
			}
		})
	}
}

// BenchmarkProtocolComparison_TheoreticalBatch compares theoretical batch read performance.
// NOTE: S7 batch efficiency shown here assumes true multi-item PDU reads,
// which may not be implemented in the actual adapter (currently sequential).
func BenchmarkProtocolComparison_TheoreticalBatch(b *testing.B) {
	protocols := []struct {
		name         string
		maxBatchSize int
	}{
		// Modbus: Up to 125 registers per request
		{"Modbus_TCP", 125},
		// OPC UA: Batches up to ~1000 nodes
		{"OPC_UA", 1000},
		// S7: PDU size limits batching (~200 bytes worth)
		// NOTE: Actual adapter may read sequentially, not batched
		{"S7comm", 200},
	}

	batchSizes := []int{1, 10, 50, 100}

	for _, proto := range protocols {
		cfg := getProtocolConfig(proto.name)
		for _, batchSize := range batchSizes {
			if batchSize > proto.maxBatchSize {
				continue
			}

			name := proto.name + "_" + itoa(batchSize) + "tags"
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					_ = simulateProtocolRead(cfg, batchSize)
				}
			})
		}
	}
}

// BenchmarkProtocolComparison_TheoreticalMemory provides theoretical memory comparison
// with full allocation patterns and varied batch sizes.
// Useful for estimating GC pressure under different workloads.
func BenchmarkProtocolComparison_TheoreticalMemory(b *testing.B) {
	protocols := []string{"Modbus_TCP", "OPC_UA", "S7comm"}

	for _, proto := range protocols {
		cfg := getProtocolConfig(proto)

		// Single tag read
		b.Run(proto+"_Single", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = simulateProtocolRead(cfg, 1)
			}
		})

		// Typical batch (10 tags)
		b.Run(proto+"_Batch10", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = simulateProtocolRead(cfg, 10)
			}
		})

		// Large batch (50 tags)
		b.Run(proto+"_Batch50", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = simulateProtocolRead(cfg, 50)
			}
		})
	}
}

// itoa is a simple int-to-string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
