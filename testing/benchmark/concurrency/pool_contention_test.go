//go:build benchmark
// +build benchmark

// Package concurrency provides benchmark tests for pool contention analysis.
package concurrency_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Connection Pool Simulation for Contention Testing
// =============================================================================

// ContentionPool simulates a connection pool with configurable contention.
type ContentionPool struct {
	mu          sync.Mutex
	connections []*MockConnection
	available   chan *MockConnection
	maxSize     int
	inUse       int32

	// Contention metrics
	acquireAttempts int64
	acquireWaits    int64
	totalWaitTime   int64 // nanoseconds
	contentionCount int64
}

// MockConnection simulates a protocol connection.
type MockConnection struct {
	ID       int
	InUse    atomic.Bool
	UseCount int64
}

// NewContentionPool creates a pool for contention testing.
func NewContentionPool(size int) *ContentionPool {
	pool := &ContentionPool{
		connections: make([]*MockConnection, size),
		available:   make(chan *MockConnection, size),
		maxSize:     size,
	}

	for i := 0; i < size; i++ {
		conn := &MockConnection{ID: i}
		pool.connections[i] = conn
		pool.available <- conn
	}

	return pool
}

// Acquire gets a connection from the pool.
func (p *ContentionPool) Acquire(ctx context.Context) (*MockConnection, error) {
	atomic.AddInt64(&p.acquireAttempts, 1)

	select {
	case conn := <-p.available:
		conn.InUse.Store(true)
		atomic.AddInt32(&p.inUse, 1)
		return conn, nil
	default:
		// Need to wait - contention!
		atomic.AddInt64(&p.contentionCount, 1)
		atomic.AddInt64(&p.acquireWaits, 1)

		waitStart := time.Now()
		select {
		case conn := <-p.available:
			atomic.AddInt64(&p.totalWaitTime, time.Since(waitStart).Nanoseconds())
			conn.InUse.Store(true)
			atomic.AddInt32(&p.inUse, 1)
			return conn, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// Release returns a connection to the pool.
func (p *ContentionPool) Release(conn *MockConnection) {
	conn.InUse.Store(false)
	conn.UseCount++
	atomic.AddInt32(&p.inUse, -1)

	select {
	case p.available <- conn:
	default:
		// Pool full, shouldn't happen
	}
}

// ContentionStats returns contention statistics.
type ContentionStats struct {
	TotalAttempts   int64
	ContentionCount int64
	WaitCount       int64
	AvgWaitTime     time.Duration
	ContentionRate  float64
	InUse           int32
}

// Stats returns current pool statistics.
func (p *ContentionPool) Stats() ContentionStats {
	attempts := atomic.LoadInt64(&p.acquireAttempts)
	contentions := atomic.LoadInt64(&p.contentionCount)
	waits := atomic.LoadInt64(&p.acquireWaits)
	totalWait := atomic.LoadInt64(&p.totalWaitTime)

	var avgWait time.Duration
	if waits > 0 {
		avgWait = time.Duration(totalWait / waits)
	}

	var contentionRate float64
	if attempts > 0 {
		contentionRate = float64(contentions) / float64(attempts) * 100
	}

	return ContentionStats{
		TotalAttempts:   attempts,
		ContentionCount: contentions,
		WaitCount:       waits,
		AvgWaitTime:     avgWait,
		ContentionRate:  contentionRate,
		InUse:           atomic.LoadInt32(&p.inUse),
	}
}

// Reset clears statistics.
func (p *ContentionPool) Reset() {
	atomic.StoreInt64(&p.acquireAttempts, 0)
	atomic.StoreInt64(&p.contentionCount, 0)
	atomic.StoreInt64(&p.acquireWaits, 0)
	atomic.StoreInt64(&p.totalWaitTime, 0)
}

// =============================================================================
// Mutex Contention Benchmarks
// =============================================================================

// BenchmarkMutexContention_LowConcurrency benchmarks mutex with low concurrency.
func BenchmarkMutexContention_LowConcurrency(b *testing.B) {
	var mu sync.Mutex
	var counter int64

	b.SetParallelism(2)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			counter++
			mu.Unlock()
		}
	})
}

// BenchmarkMutexContention_MediumConcurrency benchmarks mutex with medium concurrency.
func BenchmarkMutexContention_MediumConcurrency(b *testing.B) {
	var mu sync.Mutex
	var counter int64

	b.SetParallelism(8)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			counter++
			mu.Unlock()
		}
	})
}

// BenchmarkMutexContention_HighConcurrency benchmarks mutex with high concurrency.
func BenchmarkMutexContention_HighConcurrency(b *testing.B) {
	var mu sync.Mutex
	var counter int64

	b.SetParallelism(32)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			counter++
			mu.Unlock()
		}
	})
}

// BenchmarkRWMutexContention_ReadHeavy benchmarks RWMutex with read-heavy workload.
func BenchmarkRWMutexContention_ReadHeavy(b *testing.B) {
	var mu sync.RWMutex
	var counter int64

	b.SetParallelism(16)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%10 == 0 {
				// 10% writes
				mu.Lock()
				counter++
				mu.Unlock()
			} else {
				// 90% reads
				mu.RLock()
				_ = counter
				mu.RUnlock()
			}
			i++
		}
	})
}

// BenchmarkRWMutexContention_WriteHeavy benchmarks RWMutex with write-heavy workload.
func BenchmarkRWMutexContention_WriteHeavy(b *testing.B) {
	var mu sync.RWMutex
	var counter int64

	b.SetParallelism(16)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				// 50% writes
				mu.Lock()
				counter++
				mu.Unlock()
			} else {
				// 50% reads
				mu.RLock()
				_ = counter
				mu.RUnlock()
			}
			i++
		}
	})
}

// =============================================================================
// Pool Contention Benchmarks
// =============================================================================

// BenchmarkPoolContention_SmallPool benchmarks small pool under load.
func BenchmarkPoolContention_SmallPool(b *testing.B) {
	pool := NewContentionPool(2)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, err := pool.Acquire(ctx)
			if err != nil {
				b.Fatal(err)
			}
			// Simulate work
			time.Sleep(10 * time.Microsecond)
			pool.Release(conn)
		}
	})

	stats := pool.Stats()
	b.ReportMetric(stats.ContentionRate, "%contention")
}

// BenchmarkPoolContention_MediumPool benchmarks medium pool under load.
func BenchmarkPoolContention_MediumPool(b *testing.B) {
	pool := NewContentionPool(8)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, err := pool.Acquire(ctx)
			if err != nil {
				b.Fatal(err)
			}
			time.Sleep(10 * time.Microsecond)
			pool.Release(conn)
		}
	})

	stats := pool.Stats()
	b.ReportMetric(stats.ContentionRate, "%contention")
}

// BenchmarkPoolContention_LargePool benchmarks large pool under load.
func BenchmarkPoolContention_LargePool(b *testing.B) {
	pool := NewContentionPool(32)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, err := pool.Acquire(ctx)
			if err != nil {
				b.Fatal(err)
			}
			time.Sleep(10 * time.Microsecond)
			pool.Release(conn)
		}
	})

	stats := pool.Stats()
	b.ReportMetric(stats.ContentionRate, "%contention")
}

// =============================================================================
// Contention Analysis Tests
// =============================================================================

// TestPoolContention_VaryingPoolSize tests contention with different pool sizes.
func TestPoolContention_VaryingPoolSize(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping contention test in short mode")
	}

	poolSizes := []int{1, 2, 4, 8, 16, 32}
	workers := 16
	operations := 1000

	t.Logf("Pool Contention Analysis (workers=%d, ops=%d):", workers, operations)
	t.Logf("%-10s %15s %15s %15s %15s", "Pool Size", "Attempts", "Contentions", "Rate", "Avg Wait")

	for _, size := range poolSizes {
		pool := NewContentionPool(size)
		ctx := context.Background()

		var wg sync.WaitGroup
		wg.Add(workers)

		for w := 0; w < workers; w++ {
			go func() {
				defer wg.Done()
				for i := 0; i < operations/workers; i++ {
					conn, err := pool.Acquire(ctx)
					if err != nil {
						return
					}
					time.Sleep(100 * time.Microsecond)
					pool.Release(conn)
				}
			}()
		}

		wg.Wait()
		stats := pool.Stats()

		t.Logf("%-10d %15d %15d %14.1f%% %15v",
			size, stats.TotalAttempts, stats.ContentionCount, stats.ContentionRate, stats.AvgWaitTime)
	}
}

// TestPoolContention_VaryingWorkerCount tests contention with different worker counts.
func TestPoolContention_VaryingWorkerCount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping contention test in short mode")
	}

	workerCounts := []int{1, 2, 4, 8, 16, 32, 64}
	poolSize := 8
	operations := 2000

	t.Logf("Worker Count vs Contention (pool_size=%d, ops=%d):", poolSize, operations)
	t.Logf("%-10s %15s %15s %15s", "Workers", "Contentions", "Rate", "Avg Wait")

	for _, workers := range workerCounts {
		pool := NewContentionPool(poolSize)
		ctx := context.Background()

		var wg sync.WaitGroup
		wg.Add(workers)

		opsPerWorker := operations / workers
		if opsPerWorker < 1 {
			opsPerWorker = 1
		}

		for w := 0; w < workers; w++ {
			go func() {
				defer wg.Done()
				for i := 0; i < opsPerWorker; i++ {
					conn, err := pool.Acquire(ctx)
					if err != nil {
						return
					}
					time.Sleep(50 * time.Microsecond)
					pool.Release(conn)
				}
			}()
		}

		wg.Wait()
		stats := pool.Stats()

		t.Logf("%-10d %15d %14.1f%% %15v",
			workers, stats.ContentionCount, stats.ContentionRate, stats.AvgWaitTime)
	}
}

// TestPoolContention_VaryingWorkDuration tests contention with different work durations.
func TestPoolContention_VaryingWorkDuration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping contention test in short mode")
	}

	workDurations := []time.Duration{
		10 * time.Microsecond,
		50 * time.Microsecond,
		100 * time.Microsecond,
		500 * time.Microsecond,
		1 * time.Millisecond,
	}
	poolSize := 4
	workers := 16
	operations := 500

	t.Logf("Work Duration vs Contention (pool=%d, workers=%d, ops=%d):", poolSize, workers, operations)
	t.Logf("%-15s %15s %15s %15s", "Work Duration", "Contentions", "Rate", "Avg Wait")

	for _, duration := range workDurations {
		pool := NewContentionPool(poolSize)
		ctx := context.Background()

		var wg sync.WaitGroup
		wg.Add(workers)

		for w := 0; w < workers; w++ {
			go func() {
				defer wg.Done()
				for i := 0; i < operations/workers; i++ {
					conn, err := pool.Acquire(ctx)
					if err != nil {
						return
					}
					time.Sleep(duration)
					pool.Release(conn)
				}
			}()
		}

		wg.Wait()
		stats := pool.Stats()

		t.Logf("%-15v %15d %14.1f%% %15v",
			duration, stats.ContentionCount, stats.ContentionRate, stats.AvgWaitTime)
	}
}

// =============================================================================
// Lock-Free vs Mutex Comparison
// =============================================================================

// BenchmarkAtomicVsMutex_Counter compares atomic vs mutex for counter.
func BenchmarkAtomicVsMutex_Counter(b *testing.B) {
	b.Run("Atomic", func(b *testing.B) {
		var counter int64
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				atomic.AddInt64(&counter, 1)
			}
		})
	})

	b.Run("Mutex", func(b *testing.B) {
		var mu sync.Mutex
		var counter int64
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				mu.Lock()
				counter++
				mu.Unlock()
			}
		})
	})
}

// BenchmarkChannelVsMutex_Communication compares channel vs mutex for communication.
func BenchmarkChannelVsMutex_Communication(b *testing.B) {
	b.Run("Channel", func(b *testing.B) {
		ch := make(chan int, 100)
		done := make(chan struct{})

		// Consumer
		go func() {
			for range ch {
			}
			close(done)
		}()

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				ch <- 1
			}
		})

		close(ch)
		<-done
	})

	b.Run("Mutex", func(b *testing.B) {
		var mu sync.Mutex
		var data int
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				mu.Lock()
				data = 1
				_ = data
				mu.Unlock()
			}
		})
	})
}

// =============================================================================
// Deadlock Prevention Tests
// =============================================================================

// TestPoolContention_ContextTimeout tests that context timeout prevents deadlock.
func TestPoolContention_ContextTimeout(t *testing.T) {
	pool := NewContentionPool(1)

	// Acquire the only connection
	ctx := context.Background()
	conn1, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}

	// Try to acquire with timeout (should timeout)
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = pool.Acquire(timeoutCtx)
	elapsed := time.Since(start)

	if err != context.DeadlineExceeded {
		t.Errorf("expected timeout error, got: %v", err)
	}

	t.Logf("Timeout occurred after %v", elapsed)

	// Release and try again
	pool.Release(conn1)

	conn2, err := pool.Acquire(context.Background())
	if err != nil {
		t.Errorf("acquire after release failed: %v", err)
	} else {
		pool.Release(conn2)
		t.Log("Successfully acquired after release")
	}
}

// TestPoolContention_FairnessCheck tests that all connections get used.
func TestPoolContention_FairnessCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping fairness test in short mode")
	}

	poolSize := 4
	pool := NewContentionPool(poolSize)
	ctx := context.Background()

	// Run many operations
	operations := 10000
	var wg sync.WaitGroup
	wg.Add(8)

	for w := 0; w < 8; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < operations/8; i++ {
				conn, err := pool.Acquire(ctx)
				if err != nil {
					return
				}
				time.Sleep(10 * time.Microsecond)
				pool.Release(conn)
			}
		}()
	}

	wg.Wait()

	// Check connection usage distribution
	t.Log("Connection usage distribution:")
	var totalUse int64
	for _, conn := range pool.connections {
		t.Logf("  Connection %d: %d uses", conn.ID, conn.UseCount)
		totalUse += conn.UseCount
	}

	avgUse := float64(totalUse) / float64(poolSize)
	t.Logf("Average use per connection: %.1f", avgUse)

	// Check for reasonable fairness (each connection should be used at least 10% of average)
	minExpected := int64(avgUse * 0.1)
	for _, conn := range pool.connections {
		if conn.UseCount < minExpected {
			t.Errorf("Connection %d underused: %d uses (expected > %d)", conn.ID, conn.UseCount, minExpected)
		}
	}
}

// =============================================================================
// Contention Under Burst Load
// =============================================================================

// TestPoolContention_BurstLoad tests contention under burst traffic patterns.
func TestPoolContention_BurstLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping burst load test in short mode")
	}

	pool := NewContentionPool(4)
	ctx := context.Background()

	bursts := 5
	burstSize := 32 // workers per burst
	opsPerWorker := 50

	t.Logf("Burst Load Test (pool=4, bursts=%d, burst_size=%d):", bursts, burstSize)

	for burst := 0; burst < bursts; burst++ {
		pool.Reset()

		var wg sync.WaitGroup
		wg.Add(burstSize)

		start := time.Now()
		for w := 0; w < burstSize; w++ {
			go func() {
				defer wg.Done()
				for i := 0; i < opsPerWorker; i++ {
					conn, err := pool.Acquire(ctx)
					if err != nil {
						return
					}
					time.Sleep(100 * time.Microsecond)
					pool.Release(conn)
				}
			}()
		}

		wg.Wait()
		elapsed := time.Since(start)
		stats := pool.Stats()

		t.Logf("  Burst %d: %d ops in %v, contention=%.1f%%, avg_wait=%v",
			burst+1, stats.TotalAttempts, elapsed, stats.ContentionRate, stats.AvgWaitTime)

		// Inter-burst delay
		time.Sleep(100 * time.Millisecond)
	}
}

// TestPoolContention_NoRace verifies no data races under contention.
func TestPoolContention_NoRace(t *testing.T) {
	pool := NewContentionPool(4)
	ctx := context.Background()

	var wg sync.WaitGroup
	workers := 32
	operations := 100

	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < operations; i++ {
				conn, err := pool.Acquire(ctx)
				if err != nil {
					return
				}
				// Simulate work with the connection
				_ = fmt.Sprintf("worker-%d-op-%d", id, i)
				pool.Release(conn)
			}
		}(w)
	}

	wg.Wait()

	stats := pool.Stats()
	t.Logf("No race test completed: %d operations, %.1f%% contention",
		stats.TotalAttempts, stats.ContentionRate)
}
