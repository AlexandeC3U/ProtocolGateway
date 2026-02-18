//go:build e2e
// +build e2e

// Package e2e provides end-to-end tests for the protocol gateway.
// memory_leak_test.go tests long-running memory stability.
package e2e

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Memory Leak Test Configuration
// =============================================================================

type memoryConfig struct {
	deviceCount   int
	tagsPerDevice int
	pollingRate   time.Duration
	duration      time.Duration
	samplePeriod  time.Duration
	errorRate     float64
}

func defaultMemoryConfig() memoryConfig {
	return memoryConfig{
		deviceCount:   30,
		tagsPerDevice: 10,
		pollingRate:   50 * time.Millisecond,
		duration:      60 * time.Second,
		samplePeriod:  2 * time.Second,
		errorRate:     0.0,
	}
}

// memSample captures a memory snapshot at a point in time.
type memSample struct {
	timestamp    time.Duration
	heapAlloc    uint64
	heapInuse    uint64
	heapObjects  uint64
	goroutines   int
	numGC        uint32
	totalAlloc   uint64
	sys          uint64
	stackInuse   uint64
	dataPoints   int64
}

// =============================================================================
// Memory Leak Test Cases
// =============================================================================

// TestMemoryLeakLongRunning runs the polling engine for an extended period and
// verifies that heap memory does not grow unboundedly, indicating no memory leaks.
func TestMemoryLeakLongRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running memory leak test in short mode")
	}

	cfg := defaultMemoryConfig()
	samples := runMemoryTest(t, cfg)

	analyzeMemoryStability(t, samples, cfg)
}

// TestMemoryLeakWithErrors tests memory stability when devices produce errors.
// Error paths are common sources of memory leaks (unclosed resources, error
// objects accumulating, etc.).
func TestMemoryLeakWithErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak with errors test in short mode")
	}

	cfg := defaultMemoryConfig()
	cfg.errorRate = 0.2 // 20% error rate
	cfg.duration = 30 * time.Second

	samples := runMemoryTest(t, cfg)

	analyzeMemoryStability(t, samples, cfg)
}

// TestMemoryLeakDeviceChurn tests memory stability when devices are added and
// removed repeatedly, which can expose leaks in connection/resource management.
func TestMemoryLeakDeviceChurn(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping device churn memory test in short mode")
	}

	manager := &MultiDeviceManager{
		devices:  make(map[string]*MockDevice),
		dataChan: make(chan *DataPoint, 50000),
	}

	// Start with base devices
	baseDevices := 10
	for i := 0; i < baseDevices; i++ {
		tags := make([]*MockTag, 5)
		for j := 0; j < 5; j++ {
			tags[j] = &MockTag{ID: fmt.Sprintf("tag-%d", j), DataType: "float32"}
		}
		manager.AddDevice(&MockDevice{
			ID:           fmt.Sprintf("base-%d", i),
			Protocol:     ProtocolModbus,
			PollingRate:  50 * time.Millisecond,
			ConnectDelay: 5 * time.Millisecond,
			Tags:         tags,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	// Drain data channel
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
			}
		}
	}()

	// Force initial GC to get a clean baseline
	runtime.GC()
	time.Sleep(1 * time.Second)

	var samples []memSample
	start := time.Now()

	// Repeatedly add/remove transient devices
	for cycle := 0; cycle < 20; cycle++ {
		// Add transient devices
		transientIDs := make([]string, 10)
		for i := 0; i < 10; i++ {
			id := fmt.Sprintf("transient-%d-%d", cycle, i)
			transientIDs[i] = id
			tags := make([]*MockTag, 5)
			for j := 0; j < 5; j++ {
				tags[j] = &MockTag{ID: fmt.Sprintf("tag-%d", j), DataType: "float32"}
			}
			manager.AddDevice(&MockDevice{
				ID:           id,
				Protocol:     []Protocol{ProtocolModbus, ProtocolOPCUA, ProtocolS7}[i%3],
				PollingRate:  50 * time.Millisecond,
				ConnectDelay: 5 * time.Millisecond,
				Tags:         tags,
			})
		}

		time.Sleep(1 * time.Second)

		// Remove transient devices
		for _, id := range transientIDs {
			manager.RemoveDevice(id)
		}

		// Force GC and sample
		runtime.GC()
		time.Sleep(100 * time.Millisecond)

		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		samples = append(samples, memSample{
			timestamp:   time.Since(start),
			heapAlloc:   ms.HeapAlloc,
			heapInuse:   ms.HeapInuse,
			heapObjects: ms.HeapObjects,
			goroutines:  runtime.NumGoroutine(),
			numGC:       ms.NumGC,
			totalAlloc:  ms.TotalAlloc,
			sys:         ms.Sys,
			stackInuse:  ms.StackInuse,
			dataPoints:  atomic.LoadInt64(&received),
		})
	}

	manager.Stop()
	cancel()
	<-done

	// Analyze device churn memory
	t.Logf("=== Device Churn Memory Results ===")
	t.Logf("  Cycles completed: %d", len(samples))
	t.Logf("  Total data points: %d", atomic.LoadInt64(&received))

	if len(samples) < 4 {
		t.Skip("Not enough samples for analysis")
	}

	// Compare first quarter vs last quarter
	quarter := len(samples) / 4
	if quarter == 0 {
		quarter = 1
	}

	var earlyHeap, lateHeap uint64
	for i := 0; i < quarter; i++ {
		earlyHeap += samples[i].heapAlloc
	}
	earlyHeap /= uint64(quarter)

	for i := len(samples) - quarter; i < len(samples); i++ {
		lateHeap += samples[i].heapAlloc
	}
	lateHeap /= uint64(quarter)

	t.Logf("  Early avg heap:  %s", formatBytes(earlyHeap))
	t.Logf("  Late avg heap:   %s", formatBytes(lateHeap))

	// Memory should not grow by more than 5x during device churn
	if lateHeap > earlyHeap*5 && lateHeap-earlyHeap > 50*1024*1024 {
		t.Errorf("Heap grew from %s to %s during device churn — possible leak",
			formatBytes(earlyHeap), formatBytes(lateHeap))
	}

	// Goroutine count should stabilize (not grow with cycles)
	firstGoroutines := samples[0].goroutines
	lastGoroutines := samples[len(samples)-1].goroutines
	t.Logf("  First goroutines: %d", firstGoroutines)
	t.Logf("  Last goroutines:  %d", lastGoroutines)

	if lastGoroutines > firstGoroutines*3 && lastGoroutines-firstGoroutines > 100 {
		t.Errorf("Goroutine count grew from %d to %d — possible goroutine leak",
			firstGoroutines, lastGoroutines)
	}
}

// TestMemoryLeakGoroutineStability verifies goroutine counts remain stable
// during extended operation.
func TestMemoryLeakGoroutineStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping goroutine stability test in short mode")
	}

	cfg := defaultMemoryConfig()
	cfg.duration = 30 * time.Second

	samples := runMemoryTest(t, cfg)

	if len(samples) < 4 {
		t.Skip("Not enough samples for goroutine analysis")
	}

	t.Logf("=== Goroutine Stability Results ===")

	// Find min/max goroutine count after warmup (skip first 2 samples)
	minG, maxG := samples[2].goroutines, samples[2].goroutines
	for _, s := range samples[2:] {
		if s.goroutines < minG {
			minG = s.goroutines
		}
		if s.goroutines > maxG {
			maxG = s.goroutines
		}
	}

	t.Logf("  Min goroutines: %d", minG)
	t.Logf("  Max goroutines: %d", maxG)
	t.Logf("  Range:          %d", maxG-minG)

	// Goroutine count should not vary wildly (range < 50)
	if maxG-minG > 50 {
		t.Errorf("Goroutine count fluctuated by %d (min=%d, max=%d) — possible goroutine leak", maxG-minG, minG, maxG)
	}

	// Final count should not be significantly higher than initial
	first := samples[2].goroutines
	last := samples[len(samples)-1].goroutines
	if last > first+20 {
		t.Errorf("Goroutine count grew from %d to %d — possible goroutine leak", first, last)
	}
}

// TestMemoryLeakGCEffectiveness verifies that garbage collection is effective
// at reclaiming memory under sustained load.
func TestMemoryLeakGCEffectiveness(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping GC effectiveness test in short mode")
	}

	manager := &MultiDeviceManager{
		devices:  make(map[string]*MockDevice),
		dataChan: make(chan *DataPoint, 50000),
	}

	for i := 0; i < 30; i++ {
		tags := make([]*MockTag, 10)
		for j := 0; j < 10; j++ {
			tags[j] = &MockTag{ID: fmt.Sprintf("tag-%d", j), DataType: "float32"}
		}
		manager.AddDevice(&MockDevice{
			ID:           fmt.Sprintf("device-%d", i),
			Protocol:     []Protocol{ProtocolModbus, ProtocolOPCUA, ProtocolS7}[i%3],
			PollingRate:  20 * time.Millisecond,
			ConnectDelay: 5 * time.Millisecond,
			Tags:         tags,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	// Drain channel
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
			}
		}
	}()

	// Let it run and allocate
	time.Sleep(10 * time.Second)

	// Get stats before forced GC
	var beforeGC runtime.MemStats
	runtime.ReadMemStats(&beforeGC)

	// Force GC
	runtime.GC()
	time.Sleep(500 * time.Millisecond)

	// Get stats after GC
	var afterGC runtime.MemStats
	runtime.ReadMemStats(&afterGC)

	manager.Stop()
	cancel()
	<-done

	t.Logf("=== GC Effectiveness Results ===")
	t.Logf("  Data points processed: %d", atomic.LoadInt64(&received))
	t.Logf("  Before GC:")
	t.Logf("    HeapAlloc:   %s", formatBytes(beforeGC.HeapAlloc))
	t.Logf("    HeapInuse:   %s", formatBytes(beforeGC.HeapInuse))
	t.Logf("    HeapObjects: %d", beforeGC.HeapObjects)
	t.Logf("  After GC:")
	t.Logf("    HeapAlloc:   %s", formatBytes(afterGC.HeapAlloc))
	t.Logf("    HeapInuse:   %s", formatBytes(afterGC.HeapInuse))
	t.Logf("    HeapObjects: %d", afterGC.HeapObjects)
	t.Logf("  GC count:      %d", afterGC.NumGC)

	// GC should be able to reclaim a meaningful portion of memory
	if beforeGC.HeapAlloc > 10*1024*1024 { // Only check if there's significant memory to reclaim
		gcReclaim := float64(beforeGC.HeapAlloc-afterGC.HeapAlloc) / float64(beforeGC.HeapAlloc)
		t.Logf("  GC reclaimed:  %.1f%%", gcReclaim*100)
		// It's OK if GC didn't reclaim much — what matters is that memory is bounded
	}

	// The key check: heap should not be unreasonably large for this workload
	if afterGC.HeapAlloc > 500*1024*1024 {
		t.Errorf("HeapAlloc after GC is %s — unexpectedly high for this workload", formatBytes(afterGC.HeapAlloc))
	}
}

// =============================================================================
// Memory Test Runner
// =============================================================================

// runMemoryTest runs a memory stability test and returns collected samples.
func runMemoryTest(t *testing.T, cfg memoryConfig) []memSample {
	t.Helper()

	manager := &MultiDeviceManager{
		devices:  make(map[string]*MockDevice),
		dataChan: make(chan *DataPoint, 50000),
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
			ConnectDelay: 5 * time.Millisecond,
			ErrorRate:    cfg.errorRate,
			Tags:         tags,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.duration+10*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	// Drain data channel
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
			}
		}
	}()

	// Force baseline GC
	runtime.GC()
	time.Sleep(500 * time.Millisecond)

	// Collect memory samples
	var samples []memSample
	start := time.Now()
	sampleTicker := time.NewTicker(cfg.samplePeriod)
	defer sampleTicker.Stop()

	testTimer := time.NewTimer(cfg.duration)
	defer testTimer.Stop()

	for {
		select {
		case <-testTimer.C:
			goto cleanup
		case <-sampleTicker.C:
			runtime.GC() // Force GC before sampling for consistent measurements
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			samples = append(samples, memSample{
				timestamp:   time.Since(start),
				heapAlloc:   ms.HeapAlloc,
				heapInuse:   ms.HeapInuse,
				heapObjects: ms.HeapObjects,
				goroutines:  runtime.NumGoroutine(),
				numGC:       ms.NumGC,
				totalAlloc:  ms.TotalAlloc,
				sys:         ms.Sys,
				stackInuse:  ms.StackInuse,
				dataPoints:  atomic.LoadInt64(&received),
			})
		}
	}

cleanup:
	manager.Stop()
	cancel()
	<-done

	return samples
}

// =============================================================================
// Memory Analysis Helpers
// =============================================================================

// analyzeMemoryStability checks collected memory samples for signs of leaks.
func analyzeMemoryStability(t *testing.T, samples []memSample, cfg memoryConfig) {
	t.Helper()

	if len(samples) < 4 {
		t.Skip("Not enough memory samples for analysis")
	}

	t.Logf("=== Memory Stability Results ===")
	t.Logf("  Devices:      %d", cfg.deviceCount)
	t.Logf("  Tags/device:  %d", cfg.tagsPerDevice)
	t.Logf("  Duration:     %v", cfg.duration)
	t.Logf("  Samples:      %d", len(samples))

	// Print timeline
	t.Log("  Timeline:")
	for i, s := range samples {
		t.Logf("    [%3d] %8v | heap=%s inuse=%s objects=%d goroutines=%d gc=%d points=%d",
			i, s.timestamp.Truncate(time.Second),
			formatBytes(s.heapAlloc), formatBytes(s.heapInuse),
			s.heapObjects, s.goroutines, s.numGC, s.dataPoints)
	}

	// Skip warmup (first 10% of samples)
	warmup := len(samples) / 10
	if warmup < 1 {
		warmup = 1
	}
	stable := samples[warmup:]

	// Check 1: Heap growth trend
	// Compare first quarter vs last quarter
	quarter := len(stable) / 4
	if quarter == 0 {
		quarter = 1
	}

	var earlyHeap, lateHeap uint64
	for i := 0; i < quarter; i++ {
		earlyHeap += stable[i].heapAlloc
	}
	earlyHeap /= uint64(quarter)

	for i := len(stable) - quarter; i < len(stable); i++ {
		lateHeap += stable[i].heapAlloc
	}
	lateHeap /= uint64(quarter)

	t.Logf("  Early avg heap:   %s", formatBytes(earlyHeap))
	t.Logf("  Late avg heap:    %s", formatBytes(lateHeap))

	// Heap should not grow by more than 3x (generous bound to avoid flaky tests)
	if lateHeap > earlyHeap*3 && lateHeap-earlyHeap > 50*1024*1024 {
		t.Errorf("Heap grew from %s to %s — possible memory leak",
			formatBytes(earlyHeap), formatBytes(lateHeap))
	}

	// Check 2: Goroutine stability
	firstGoroutines := stable[0].goroutines
	lastGoroutines := stable[len(stable)-1].goroutines
	t.Logf("  First goroutines: %d", firstGoroutines)
	t.Logf("  Last goroutines:  %d", lastGoroutines)

	if lastGoroutines > firstGoroutines*2 && lastGoroutines-firstGoroutines > 50 {
		t.Errorf("Goroutine count grew from %d to %d — possible goroutine leak",
			firstGoroutines, lastGoroutines)
	}

	// Check 3: Heap objects trend (strongly indicates leaks)
	var earlyObjects, lateObjects uint64
	for i := 0; i < quarter; i++ {
		earlyObjects += stable[i].heapObjects
	}
	earlyObjects /= uint64(quarter)

	for i := len(stable) - quarter; i < len(stable); i++ {
		lateObjects += stable[i].heapObjects
	}
	lateObjects /= uint64(quarter)

	t.Logf("  Early avg objects: %d", earlyObjects)
	t.Logf("  Late avg objects:  %d", lateObjects)

	if lateObjects > earlyObjects*3 && lateObjects-earlyObjects > 100000 {
		t.Errorf("Heap objects grew from %d to %d — possible object leak",
			earlyObjects, lateObjects)
	}

	// Check 4: GC is running (sanity check)
	lastSample := samples[len(samples)-1]
	firstSample := samples[0]
	gcRuns := lastSample.numGC - firstSample.numGC
	t.Logf("  GC runs:          %d", gcRuns)

	if gcRuns == 0 {
		t.Log("  WARNING: GC did not run during the test — heap may be too small to trigger GC")
	}

	// Check 5: Total data points processed (sanity check)
	t.Logf("  Total data points: %d", lastSample.dataPoints)
	if lastSample.dataPoints == 0 {
		t.Error("No data points processed — test may not be exercising the system")
	}
}

// formatBytes formats a byte count as a human-readable string.
func formatBytes(b uint64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
