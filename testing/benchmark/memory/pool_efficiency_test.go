//go:build benchmark
// +build benchmark

// Package memory provides benchmark tests for connection pool efficiency analysis.
package memory

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Connection Pool Simulation
// =============================================================================

// MockConnection simulates a protocol connection.
type MockConnection struct {
	ID        int
	Protocol  string
	CreatedAt time.Time
	LastUsed  time.Time
	UseCount  int64
}

// Use simulates using the connection.
func (c *MockConnection) Use() {
	atomic.AddInt64(&c.UseCount, 1)
	c.LastUsed = time.Now()
	// Simulate work
	time.Sleep(100 * time.Microsecond)
}

// ConnectionPool manages a pool of connections.
type ConnectionPool struct {
	mu          sync.Mutex
	connections []*MockConnection
	available   chan *MockConnection
	maxSize     int
	created     int
	protocol    string

	// Stats
	hits      int64
	misses    int64
	creates   int64
	destroys  int64
	waitTime  int64 // total nanoseconds spent waiting
	waitCount int64
}

// NewConnectionPool creates a new connection pool.
func NewConnectionPool(protocol string, maxSize int) *ConnectionPool {
	return &ConnectionPool{
		connections: make([]*MockConnection, 0, maxSize),
		available:   make(chan *MockConnection, maxSize),
		maxSize:     maxSize,
		protocol:    protocol,
	}
}

// Acquire gets a connection from the pool or creates a new one.
func (p *ConnectionPool) Acquire() *MockConnection {
	waitStart := time.Now()

	select {
	case conn := <-p.available:
		// Pool hit
		atomic.AddInt64(&p.hits, 1)
		return conn
	default:
		// Pool miss - need to create or wait
		p.mu.Lock()
		if p.created < p.maxSize {
			// Create new connection
			atomic.AddInt64(&p.misses, 1)
			atomic.AddInt64(&p.creates, 1)
			conn := &MockConnection{
				ID:        p.created,
				Protocol:  p.protocol,
				CreatedAt: time.Now(),
				LastUsed:  time.Now(),
			}
			p.connections = append(p.connections, conn)
			p.created++
			p.mu.Unlock()
			return conn
		}
		p.mu.Unlock()

		// Pool is full, wait for available connection
		atomic.AddInt64(&p.waitCount, 1)
		conn := <-p.available
		atomic.AddInt64(&p.waitTime, time.Since(waitStart).Nanoseconds())
		atomic.AddInt64(&p.hits, 1)
		return conn
	}
}

// Release returns a connection to the pool.
func (p *ConnectionPool) Release(conn *MockConnection) {
	select {
	case p.available <- conn:
		// Returned to pool
	default:
		// Pool full, discard (shouldn't happen with proper sizing)
		atomic.AddInt64(&p.destroys, 1)
	}
}

// PoolStats returns pool statistics.
type PoolStats struct {
	Hits           int64
	Misses         int64
	Creates        int64
	Destroys       int64
	HitRate        float64
	AvgWaitTime    time.Duration
	TotalWaitCount int64
	PoolSize       int
	MaxSize        int
}

// Stats returns current pool statistics.
func (p *ConnectionPool) Stats() PoolStats {
	hits := atomic.LoadInt64(&p.hits)
	misses := atomic.LoadInt64(&p.misses)
	waitTime := atomic.LoadInt64(&p.waitTime)
	waitCount := atomic.LoadInt64(&p.waitCount)

	total := hits + misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100
	}

	avgWait := time.Duration(0)
	if waitCount > 0 {
		avgWait = time.Duration(waitTime / waitCount)
	}

	p.mu.Lock()
	poolSize := p.created
	p.mu.Unlock()

	return PoolStats{
		Hits:           hits,
		Misses:         misses,
		Creates:        atomic.LoadInt64(&p.creates),
		Destroys:       atomic.LoadInt64(&p.destroys),
		HitRate:        hitRate,
		AvgWaitTime:    avgWait,
		TotalWaitCount: waitCount,
		PoolSize:       poolSize,
		MaxSize:        p.maxSize,
	}
}

// Close closes the pool and all connections.
func (p *ConnectionPool) Close() {
	close(p.available)
	p.mu.Lock()
	p.connections = nil
	p.mu.Unlock()
}

// =============================================================================
// Pool Efficiency Benchmarks
// =============================================================================

// BenchmarkPoolEfficiency_LowContention benchmarks pool with low contention.
func BenchmarkPoolEfficiency_LowContention(b *testing.B) {
	pool := NewConnectionPool("modbus", 10)
	defer pool.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		conn := pool.Acquire()
		conn.Use()
		pool.Release(conn)
	}

	stats := pool.Stats()
	b.ReportMetric(stats.HitRate, "hit%")
}

// BenchmarkPoolEfficiency_HighContention benchmarks pool with high contention.
func BenchmarkPoolEfficiency_HighContention(b *testing.B) {
	pool := NewConnectionPool("modbus", 2) // Small pool = high contention
	defer pool.Close()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn := pool.Acquire()
			conn.Use()
			pool.Release(conn)
		}
	})

	stats := pool.Stats()
	b.ReportMetric(stats.HitRate, "hit%")
}

// =============================================================================
// Pool Size Efficiency Tests
// =============================================================================

// TestPoolEfficiency_SizeVsHitRate tests how pool size affects hit rate.
func TestPoolEfficiency_SizeVsHitRate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping pool efficiency test in short mode")
	}

	poolSizes := []int{1, 2, 4, 8, 16, 32}
	workers := 8
	iterations := 1000

	t.Logf("Pool Size vs Hit Rate (workers=%d, iterations=%d):", workers, iterations)
	t.Logf("%-10s %10s %10s %10s %15s", "Pool Size", "Hit Rate", "Creates", "Waits", "Avg Wait")

	for _, size := range poolSizes {
		pool := NewConnectionPool("modbus", size)

		var wg sync.WaitGroup
		wg.Add(workers)

		for w := 0; w < workers; w++ {
			go func() {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					conn := pool.Acquire()
					conn.Use()
					pool.Release(conn)
				}
			}()
		}

		wg.Wait()

		stats := pool.Stats()
		t.Logf("%-10d %9.1f%% %10d %10d %15v",
			size, stats.HitRate, stats.Creates, stats.TotalWaitCount, stats.AvgWaitTime)

		pool.Close()
	}
}

// TestPoolEfficiency_WorkerVsHitRate tests how worker count affects hit rate.
func TestPoolEfficiency_WorkerVsHitRate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping pool efficiency test in short mode")
	}

	workerCounts := []int{1, 2, 4, 8, 16, 32}
	poolSize := 8
	iterations := 500

	t.Logf("Worker Count vs Hit Rate (pool_size=%d, iterations=%d):", poolSize, iterations)
	t.Logf("%-10s %10s %10s %10s %15s", "Workers", "Hit Rate", "Creates", "Waits", "Avg Wait")

	for _, workers := range workerCounts {
		pool := NewConnectionPool("modbus", poolSize)

		var wg sync.WaitGroup
		wg.Add(workers)

		for w := 0; w < workers; w++ {
			go func() {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					conn := pool.Acquire()
					conn.Use()
					pool.Release(conn)
				}
			}()
		}

		wg.Wait()

		stats := pool.Stats()
		t.Logf("%-10d %9.1f%% %10d %10d %15v",
			workers, stats.HitRate, stats.Creates, stats.TotalWaitCount, stats.AvgWaitTime)

		pool.Close()
	}
}

// =============================================================================
// Protocol-Specific Pool Tests
// =============================================================================

// TestPoolEfficiency_ProtocolComparison compares pool efficiency across protocols.
func TestPoolEfficiency_ProtocolComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping protocol comparison test in short mode")
	}

	protocols := []struct {
		name         string
		poolSize     int
		workDuration time.Duration
	}{
		{"modbus", 5, 5 * time.Millisecond}, // Modbus is slower
		{"opcua", 10, 2 * time.Millisecond}, // OPC UA faster, more connections
		{"s7", 4, 3 * time.Millisecond},     // S7 moderate
	}

	workers := 8
	iterations := 200

	t.Logf("Protocol Pool Comparison (workers=%d):", workers)
	t.Logf("%-10s %10s %10s %10s %15s", "Protocol", "Pool Size", "Hit Rate", "Waits", "Throughput")

	for _, proto := range protocols {
		pool := NewConnectionPool(proto.name, proto.poolSize)

		start := time.Now()
		var wg sync.WaitGroup
		wg.Add(workers)

		for w := 0; w < workers; w++ {
			go func() {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					conn := pool.Acquire()
					time.Sleep(proto.workDuration) // Simulate protocol-specific work
					pool.Release(conn)
				}
			}()
		}

		wg.Wait()
		elapsed := time.Since(start)

		stats := pool.Stats()
		throughput := float64(workers*iterations) / elapsed.Seconds()

		t.Logf("%-10s %10d %9.1f%% %10d %12.1f/s",
			proto.name, proto.poolSize, stats.HitRate, stats.TotalWaitCount, throughput)

		pool.Close()
	}
}

// =============================================================================
// Memory Allocation Analysis
// =============================================================================

// TestPoolEfficiency_MemoryAllocation tracks memory allocations.
func TestPoolEfficiency_MemoryAllocation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory allocation test in short mode")
	}

	// Test without pool (new allocations each time)
	t.Run("NoPool", func(t *testing.T) {
		iterations := 10000
		start := time.Now()

		for i := 0; i < iterations; i++ {
			conn := &MockConnection{
				ID:        i,
				Protocol:  "modbus",
				CreatedAt: time.Now(),
			}
			conn.Use()
			// Connection is garbage collected
		}

		elapsed := time.Since(start)
		t.Logf("Without pool: %d allocations in %v (%.1f allocs/ms)",
			iterations, elapsed, float64(iterations)/float64(elapsed.Milliseconds()))
	})

	// Test with pool (reuse allocations)
	t.Run("WithPool", func(t *testing.T) {
		pool := NewConnectionPool("modbus", 10)
		defer pool.Close()

		iterations := 10000
		start := time.Now()

		for i := 0; i < iterations; i++ {
			conn := pool.Acquire()
			conn.Use()
			pool.Release(conn)
		}

		elapsed := time.Since(start)
		stats := pool.Stats()
		t.Logf("With pool: %d operations, %d allocations in %v (%.1f ops/ms, hit rate: %.1f%%)",
			iterations, stats.Creates, elapsed, float64(iterations)/float64(elapsed.Milliseconds()), stats.HitRate)
	})
}

// =============================================================================
// Connection Reuse Analysis
// =============================================================================

// TestPoolEfficiency_ConnectionReuse tests connection reuse patterns.
func TestPoolEfficiency_ConnectionReuse(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping connection reuse test in short mode")
	}

	pool := NewConnectionPool("modbus", 4)
	defer pool.Close()

	iterations := 1000

	for i := 0; i < iterations; i++ {
		conn := pool.Acquire()
		conn.Use()
		pool.Release(conn)
	}

	stats := pool.Stats()

	t.Logf("Connection Reuse Analysis:")
	t.Logf("  Total operations: %d", stats.Hits+stats.Misses)
	t.Logf("  Connections created: %d", stats.Creates)
	t.Logf("  Pool hit rate: %.1f%%", stats.HitRate)

	// Calculate average reuse
	avgReuse := float64(stats.Hits+stats.Misses) / float64(stats.Creates)
	t.Logf("  Avg reuse per connection: %.1f times", avgReuse)

	// Analyze individual connection usage
	pool.mu.Lock()
	t.Logf("  Individual connection usage:")
	for _, conn := range pool.connections {
		t.Logf("    Connection %d: %d uses", conn.ID, conn.UseCount)
	}
	pool.mu.Unlock()
}

// =============================================================================
// Pool Warmup Analysis
// =============================================================================

// TestPoolEfficiency_Warmup tests pool warmup behavior.
func TestPoolEfficiency_Warmup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping warmup test in short mode")
	}

	poolSize := 8
	pool := NewConnectionPool("modbus", poolSize)
	defer pool.Close()

	// Pre-warm pool
	t.Log("Pre-warming pool...")
	for i := 0; i < poolSize; i++ {
		conn := pool.Acquire()
		pool.Release(conn)
	}

	warmStats := pool.Stats()
	t.Logf("After warmup: Creates=%d, Pool filled=%d/%d",
		warmStats.Creates, warmStats.PoolSize, poolSize)

	// Now run workload - should have 100% hit rate
	iterations := 1000
	for i := 0; i < iterations; i++ {
		conn := pool.Acquire()
		conn.Use()
		pool.Release(conn)
	}

	finalStats := pool.Stats()
	postWarmupHitRate := float64(finalStats.Hits-warmStats.Hits) / float64(iterations) * 100

	t.Logf("Post-warmup performance:")
	t.Logf("  Operations: %d", iterations)
	t.Logf("  Additional creates: %d", finalStats.Creates-warmStats.Creates)
	t.Logf("  Hit rate: %.1f%%", postWarmupHitRate)

	if postWarmupHitRate < 95.0 {
		t.Errorf("Post-warmup hit rate too low: %.1f%% (expected > 95%%)", postWarmupHitRate)
	}
}

// =============================================================================
// Variable Load Pattern Tests
// =============================================================================

// TestPoolEfficiency_BurstLoad tests pool under burst load patterns.
func TestPoolEfficiency_BurstLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping burst load test in short mode")
	}

	pool := NewConnectionPool("modbus", 8)
	defer pool.Close()

	t.Log("Testing burst load patterns...")

	// Burst pattern: periods of high activity followed by idle
	bursts := 5
	opsPerBurst := 200
	burstWorkers := 16
	idlePeriod := 100 * time.Millisecond

	for burst := 0; burst < bursts; burst++ {
		t.Logf("  Burst %d starting...", burst+1)
		startStats := pool.Stats()
		startTime := time.Now()

		var wg sync.WaitGroup
		wg.Add(burstWorkers)

		for w := 0; w < burstWorkers; w++ {
			go func() {
				defer wg.Done()
				for i := 0; i < opsPerBurst/burstWorkers; i++ {
					conn := pool.Acquire()
					conn.Use()
					pool.Release(conn)
				}
			}()
		}

		wg.Wait()
		burstDuration := time.Since(startTime)
		endStats := pool.Stats()

		burstHits := endStats.Hits - startStats.Hits
		burstMisses := endStats.Misses - startStats.Misses
		burstTotal := burstHits + burstMisses
		burstHitRate := float64(burstHits) / float64(burstTotal) * 100

		t.Logf("    Burst %d: %d ops in %v, hit rate: %.1f%%",
			burst+1, burstTotal, burstDuration, burstHitRate)

		// Idle period
		time.Sleep(idlePeriod)
	}

	finalStats := pool.Stats()
	t.Logf("Final stats: Creates=%d, Overall hit rate=%.1f%%",
		finalStats.Creates, finalStats.HitRate)
}

// =============================================================================
// Pool Sizing Recommendations
// =============================================================================

// TestPoolEfficiency_SizingRecommendation provides pool sizing recommendations.
func TestPoolEfficiency_SizingRecommendation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping sizing recommendation test in short mode")
	}

	targetHitRate := 95.0
	workers := 10
	iterations := 500

	t.Logf("Finding optimal pool size for %.1f%% hit rate with %d workers...", targetHitRate, workers)

	var optimalSize int
	var optimalStats PoolStats

	for size := 1; size <= 32; size++ {
		pool := NewConnectionPool("modbus", size)

		var wg sync.WaitGroup
		wg.Add(workers)

		for w := 0; w < workers; w++ {
			go func() {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					conn := pool.Acquire()
					// Random work duration to simulate real variance
					time.Sleep(time.Duration(rand.Intn(2000)+500) * time.Microsecond)
					pool.Release(conn)
				}
			}()
		}

		wg.Wait()
		stats := pool.Stats()
		pool.Close()

		if stats.HitRate >= targetHitRate && optimalSize == 0 {
			optimalSize = size
			optimalStats = stats
		}
	}

	if optimalSize > 0 {
		t.Logf("Recommendation: Pool size %d achieves %.1f%% hit rate", optimalSize, optimalStats.HitRate)
		t.Logf("  With %d connections, %d waits, avg wait: %v",
			optimalStats.PoolSize, optimalStats.TotalWaitCount, optimalStats.AvgWaitTime)
	} else {
		t.Log("Could not achieve target hit rate with tested pool sizes")
	}
}
