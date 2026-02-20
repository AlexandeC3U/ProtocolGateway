//go:build benchmark
// +build benchmark

// Package memory provides benchmark tests for buffer memory behavior under load.
// buffer_growth_test.go measures buffer allocation, growth, and retention patterns
// under sustained high-throughput and burst scenarios.
package memory

import (
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Buffer Simulation Types
// =============================================================================

// RingBuffer simulates the MQTT offline buffer that grows under load.
type RingBuffer struct {
	mu       sync.Mutex
	data     [][]byte
	capacity int
	head     int
	tail     int
	count    int
	drops    int64
}

// NewRingBuffer creates a ring buffer with the given capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		data:     make([][]byte, capacity),
		capacity: capacity,
	}
}

// Push adds data to the buffer. Returns false if the buffer is full.
func (b *RingBuffer) Push(data []byte) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.count >= b.capacity {
		atomic.AddInt64(&b.drops, 1)
		return false
	}

	// Copy to simulate real buffering
	cp := make([]byte, len(data))
	copy(cp, data)

	b.data[b.tail] = cp
	b.tail = (b.tail + 1) % b.capacity
	b.count++
	return true
}

// Pop removes and returns the oldest item from the buffer.
func (b *RingBuffer) Pop() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.count == 0 {
		return nil
	}

	data := b.data[b.head]
	b.data[b.head] = nil // Allow GC
	b.head = (b.head + 1) % b.capacity
	b.count--
	return data
}

// Len returns the current number of items.
func (b *RingBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.count
}

// Drops returns the number of dropped messages.
func (b *RingBuffer) Drops() int64 {
	return atomic.LoadInt64(&b.drops)
}

// GrowableBuffer simulates a dynamically growing buffer (slice-backed).
type GrowableBuffer struct {
	mu     sync.Mutex
	data   [][]byte
	maxCap int
	drops  int64
}

// NewGrowableBuffer creates a growable buffer with a max capacity.
func NewGrowableBuffer(maxCap int) *GrowableBuffer {
	return &GrowableBuffer{
		data:   make([][]byte, 0, 64),
		maxCap: maxCap,
	}
}

// Push appends data to the buffer.
func (b *GrowableBuffer) Push(data []byte) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.data) >= b.maxCap {
		atomic.AddInt64(&b.drops, 1)
		return false
	}

	cp := make([]byte, len(data))
	copy(cp, data)
	b.data = append(b.data, cp)
	return true
}

// Drain removes all items from the buffer and returns them.
func (b *GrowableBuffer) Drain() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	result := b.data
	b.data = make([][]byte, 0, cap(result)/2)
	return result
}

// Len returns the current number of items.
func (b *GrowableBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.data)
}

// PooledBuffer uses sync.Pool for buffer slice reuse.
type PooledBuffer struct {
	pool *sync.Pool
}

// NewPooledBuffer creates a buffer with pooled byte slices.
func NewPooledBuffer(bufSize int) *PooledBuffer {
	return &PooledBuffer{
		pool: &sync.Pool{
			New: func() interface{} {
				buf := make([]byte, bufSize)
				return &buf
			},
		},
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

// BenchmarkBufferGrowth_RingBuffer measures ring buffer throughput.
func BenchmarkBufferGrowth_RingBuffer(b *testing.B) {
	sizes := []int{100, 1000, 10000, 100000}
	payloadSize := 256

	for _, size := range sizes {
		b.Run(fmt.Sprintf("cap=%d", size), func(b *testing.B) {
			buf := NewRingBuffer(size)
			payload := make([]byte, payloadSize)
			rand.Read(payload)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				buf.Push(payload)
				if i%2 == 0 {
					buf.Pop()
				}
			}
		})
	}
}

// BenchmarkBufferGrowth_GrowableBuffer measures growable buffer allocation.
func BenchmarkBufferGrowth_GrowableBuffer(b *testing.B) {
	payloadSize := 256

	b.Run("grow_and_drain", func(b *testing.B) {
		buf := NewGrowableBuffer(100000)
		payload := make([]byte, payloadSize)
		rand.Read(payload)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			buf.Push(payload)
			if i%1000 == 999 {
				buf.Drain()
			}
		}
	})

	b.Run("constant_pressure", func(b *testing.B) {
		buf := NewGrowableBuffer(10000)
		payload := make([]byte, payloadSize)
		rand.Read(payload)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			if !buf.Push(payload) {
				buf.Drain()
				buf.Push(payload)
			}
		}
	})
}

// BenchmarkBufferGrowth_PooledVsRaw compares pooled vs raw allocation.
func BenchmarkBufferGrowth_PooledVsRaw(b *testing.B) {
	bufSize := 256

	b.Run("raw_alloc", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf := make([]byte, bufSize)
			buf[0] = byte(i)
			_ = buf
		}
	})

	b.Run("sync_pool", func(b *testing.B) {
		pool := NewPooledBuffer(bufSize)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf := pool.pool.Get().(*[]byte)
			(*buf)[0] = byte(i)
			pool.pool.Put(buf)
		}
	})
}

// BenchmarkBufferGrowth_ConcurrentPushPop measures concurrent buffer access.
func BenchmarkBufferGrowth_ConcurrentPushPop(b *testing.B) {
	workers := []int{1, 4, 8, 16}

	for _, w := range workers {
		b.Run(fmt.Sprintf("workers=%d", w), func(b *testing.B) {
			buf := NewRingBuffer(100000)
			payload := make([]byte, 128)
			rand.Read(payload)

			b.ReportAllocs()
			b.ResetTimer()
			b.SetParallelism(w)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					buf.Push(payload)
					buf.Pop()
				}
			})
		})
	}
}

// BenchmarkBufferGrowth_PayloadSizes measures impact of payload size on buffer throughput.
func BenchmarkBufferGrowth_PayloadSizes(b *testing.B) {
	sizes := []int{64, 256, 1024, 4096}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("payload=%dB", size), func(b *testing.B) {
			buf := NewRingBuffer(10000)
			payload := make([]byte, size)
			rand.Read(payload)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				buf.Push(payload)
				buf.Pop()
			}
		})
	}
}

// =============================================================================
// Profile / Analysis Tests
// =============================================================================

// TestBufferGrowth_MemoryProfile measures heap growth under sustained load.
func TestBufferGrowth_MemoryProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory profile test in short mode")
	}

	bufferSizes := []int{1000, 10000, 50000}
	payloadSize := 256

	for _, bufSize := range bufferSizes {
		t.Run(fmt.Sprintf("cap=%d", bufSize), func(t *testing.T) {
			runtime.GC()
			var before runtime.MemStats
			runtime.ReadMemStats(&before)

			buf := NewRingBuffer(bufSize)
			payload := make([]byte, payloadSize)
			rand.Read(payload)

			// Fill the buffer
			for i := 0; i < bufSize; i++ {
				buf.Push(payload)
			}

			runtime.GC()
			var after runtime.MemStats
			runtime.ReadMemStats(&after)

			heapGrowth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
			expectedMin := int64(bufSize) * int64(payloadSize)

			t.Logf("Buffer cap=%d:", bufSize)
			t.Logf("  Heap growth: %d KB", heapGrowth/1024)
			t.Logf("  Expected min: %d KB (payload only)", expectedMin/1024)
			t.Logf("  Overhead: %.1f%%", float64(heapGrowth-expectedMin)/float64(expectedMin)*100)

			if heapGrowth < expectedMin/2 {
				t.Logf("  Warning: Heap growth lower than expected (GC may have run)")
			}
		})
	}
}

// TestBufferGrowth_BurstAndRecovery measures memory behavior during bursts.
func TestBufferGrowth_BurstAndRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping burst test in short mode")
	}

	buf := NewGrowableBuffer(100000)
	payloadSize := 256
	payload := make([]byte, payloadSize)
	rand.Read(payload)

	type phase struct {
		name     string
		heapKB   int64
		bufLen   int
		gcPauseUs int64
	}
	var phases []phase

	snapshot := func(name string) {
		runtime.GC()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		phases = append(phases, phase{
			name:      name,
			heapKB:    int64(ms.HeapAlloc) / 1024,
			bufLen:    buf.Len(),
			gcPauseUs: int64(ms.PauseTotalNs) / 1000,
		})
	}

	snapshot("baseline")

	// Burst 1: fill 50k items
	for i := 0; i < 50000; i++ {
		buf.Push(payload)
	}
	snapshot("after_burst_50k")

	// Drain
	buf.Drain()
	snapshot("after_drain_1")

	// Burst 2: fill 100k items
	for i := 0; i < 100000; i++ {
		buf.Push(payload)
	}
	snapshot("after_burst_100k")

	// Partial drain (consume half)
	drained := buf.Drain()
	half := len(drained) / 2
	for i := 0; i < half; i++ {
		buf.Push(drained[i])
	}
	snapshot("after_partial_drain")

	// Full drain
	buf.Drain()
	snapshot("after_full_drain")

	t.Log("Burst and recovery phases:")
	for _, p := range phases {
		t.Logf("  %-25s heap=%6d KB  buf_len=%6d  gc_pause=%d us",
			p.name, p.heapKB, p.bufLen, p.gcPauseUs)
	}
}

// TestBufferGrowth_DropRate measures message drop rate under different load levels.
func TestBufferGrowth_DropRate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping drop rate test in short mode")
	}

	bufferCap := 10000
	payloadSize := 128
	duration := 2 * time.Second

	producerCounts := []int{1, 5, 10, 20}

	for _, producers := range producerCounts {
		t.Run(fmt.Sprintf("producers=%d", producers), func(t *testing.T) {
			buf := NewRingBuffer(bufferCap)
			payload := make([]byte, payloadSize)
			rand.Read(payload)

			var totalPushed int64
			var wg sync.WaitGroup
			stop := make(chan struct{})

			// Start consumer
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-stop:
						return
					default:
						if buf.Pop() == nil {
							time.Sleep(100 * time.Microsecond)
						}
					}
				}
			}()

			// Start producers
			for i := 0; i < producers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						select {
						case <-stop:
							return
						default:
							buf.Push(payload)
							atomic.AddInt64(&totalPushed, 1)
							time.Sleep(10 * time.Microsecond)
						}
					}
				}()
			}

			time.Sleep(duration)
			close(stop)
			wg.Wait()

			pushed := atomic.LoadInt64(&totalPushed)
			drops := buf.Drops()
			dropRate := float64(0)
			if pushed > 0 {
				dropRate = float64(drops) / float64(pushed) * 100
			}

			t.Logf("Producers=%d: pushed=%d drops=%d drop_rate=%.2f%%",
				producers, pushed, drops, dropRate)
		})
	}
}

// TestBufferGrowth_SliceCapRetention checks if Go retains oversized slice capacity.
func TestBufferGrowth_SliceCapRetention(t *testing.T) {
	buf := NewGrowableBuffer(100000)
	payload := make([]byte, 256)

	// Grow to 50k
	for i := 0; i < 50000; i++ {
		buf.Push(payload)
	}
	t.Logf("After fill 50k: len=%d", buf.Len())

	// Drain
	buf.Drain()
	t.Logf("After drain: len=%d", buf.Len())

	// Refill to only 1k
	for i := 0; i < 1000; i++ {
		buf.Push(payload)
	}

	buf.mu.Lock()
	currentCap := cap(buf.data)
	currentLen := len(buf.data)
	buf.mu.Unlock()

	t.Logf("After refill 1k: len=%d cap=%d", currentLen, currentCap)
	t.Logf("Capacity ratio: %.1fx (cap/len)", float64(currentCap)/float64(currentLen))

	// The drain creates a new slice with cap/2, so it shouldn't retain the 50k cap
	if currentCap > 30000 {
		t.Logf("Warning: Large capacity retained after drain (%d)", currentCap)
	}
}
