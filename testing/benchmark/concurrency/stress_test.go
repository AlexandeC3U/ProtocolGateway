package concurrency_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

// BenchmarkConcurrent_DataPointCreation measures concurrent DataPoint creation.
func BenchmarkConcurrent_DataPointCreation(b *testing.B) {
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			dp := domain.NewDataPoint(
				"device-1",
				"tag-1",
				"topic",
				42.0,
				"unit",
				domain.QualityGood,
			)
			_ = dp
		}
	})
}

// BenchmarkConcurrent_PoolUsage measures concurrent pool acquire/release.
func BenchmarkConcurrent_PoolUsage(b *testing.B) {
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			dp := domain.AcquireDataPoint(
				"device-1",
				"tag-1",
				"topic",
				42.0,
				"unit",
				domain.QualityGood,
			)
			domain.ReleaseDataPoint(dp)
		}
	})
}

// BenchmarkConcurrent_MapAccess simulates concurrent map access patterns.
func BenchmarkConcurrent_MapAccess(b *testing.B) {
	m := &sync.Map{}

	// Pre-populate
	for i := 0; i < 100; i++ {
		m.Store(i, &domain.DataPoint{TagID: "tag"})
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := i % 100
			if i%10 == 0 {
				// Write (10%)
				m.Store(key, &domain.DataPoint{TagID: "updated"})
			} else {
				// Read (90%)
				m.Load(key)
			}
			i++
		}
	})
}

// BenchmarkConcurrent_ChannelSend simulates concurrent channel operations.
func BenchmarkConcurrent_ChannelSend(b *testing.B) {
	ch := make(chan *domain.DataPoint, 1000)

	// Consumer goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ch:
				// Consume
			}
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			select {
			case ch <- &domain.DataPoint{Value: 42}:
			default:
				// Channel full, skip
			}
		}
	})
}

// BenchmarkConcurrent_AtomicCounter simulates atomic counter operations.
func BenchmarkConcurrent_AtomicCounter(b *testing.B) {
	var counter atomic.Int64

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			counter.Add(1)
		}
	})
}

// BenchmarkConcurrent_MutexContention measures mutex contention.
func BenchmarkConcurrent_MutexContention(b *testing.B) {
	var mu sync.Mutex
	var counter int64

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			counter++
			mu.Unlock()
		}
	})
}

// BenchmarkConcurrent_RWMutex_ReadHeavy measures RWMutex with 90% reads.
func BenchmarkConcurrent_RWMutex_ReadHeavy(b *testing.B) {
	var mu sync.RWMutex
	var data = make(map[string]int)
	data["key"] = 42

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%10 == 0 {
				// Write (10%)
				mu.Lock()
				data["key"] = i
				mu.Unlock()
			} else {
				// Read (90%)
				mu.RLock()
				_ = data["key"]
				mu.RUnlock()
			}
			i++
		}
	})
}

// TestConcurrent_NoRace ensures no race conditions under load.
func TestConcurrent_NoRace(t *testing.T) {
	const goroutines = 100
	const iterations = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				dp := domain.AcquireDataPoint(
					"device",
					"tag",
					"topic",
					float64(i),
					"unit",
					domain.QualityGood,
				)
				_, _ = dp.ToJSON()
				domain.ReleaseDataPoint(dp)
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for goroutines")
	}
}
