//go:build benchmark
// +build benchmark

// Package concurrency_test provides benchmark tests for subscription stress analysis.
// subscription_stress_test.go measures concurrent subscription create/destroy,
// notification fanout, and data channel throughput under many subscriptions.
package concurrency_test

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Subscription Simulation Types
// =============================================================================

// Notification represents a data change notification from a subscription.
type Notification struct {
	SubscriptionID string
	NodeID         string
	Value          interface{}
	Timestamp      time.Time
	Quality        int
}

// Subscription represents a monitored subscription.
type Subscription struct {
	ID          string
	NodeIDs     []string
	Interval    time.Duration
	DataChan    chan *Notification
	stopChan    chan struct{}
	running     atomic.Bool
	notifyCount int64
}

// Start begins generating notifications at the configured interval.
func (s *Subscription) Start() {
	if !s.running.CompareAndSwap(false, true) {
		return
	}
	s.stopChan = make(chan struct{})

	go func() {
		ticker := time.NewTicker(s.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-s.stopChan:
				return
			case <-ticker.C:
				for _, nodeID := range s.NodeIDs {
					notif := &Notification{
						SubscriptionID: s.ID,
						NodeID:         nodeID,
						Value:          rand.Float64() * 100,
						Timestamp:      time.Now(),
						Quality:        192,
					}
					select {
					case s.DataChan <- notif:
						atomic.AddInt64(&s.notifyCount, 1)
					default:
						// Channel full, drop
					}
				}
			}
		}
	}()
}

// Stop stops generating notifications.
func (s *Subscription) Stop() {
	if s.running.CompareAndSwap(true, false) {
		close(s.stopChan)
	}
}

// NotifyCount returns the number of notifications sent.
func (s *Subscription) NotifyCount() int64 {
	return atomic.LoadInt64(&s.notifyCount)
}

// SubscriptionManager manages multiple subscriptions with fanout.
type SubscriptionManager struct {
	mu            sync.RWMutex
	subscriptions map[string]*Subscription
	fanoutChan    chan *Notification
	bufSize       int
}

// NewSubscriptionManager creates a new manager.
func NewSubscriptionManager(bufSize int) *SubscriptionManager {
	return &SubscriptionManager{
		subscriptions: make(map[string]*Subscription),
		fanoutChan:    make(chan *Notification, bufSize),
		bufSize:       bufSize,
	}
}

// Subscribe creates and starts a new subscription.
func (m *SubscriptionManager) Subscribe(id string, nodeIDs []string, interval time.Duration) *Subscription {
	sub := &Subscription{
		ID:       id,
		NodeIDs:  nodeIDs,
		Interval: interval,
		DataChan: m.fanoutChan,
	}

	m.mu.Lock()
	m.subscriptions[id] = sub
	m.mu.Unlock()

	sub.Start()
	return sub
}

// Unsubscribe stops and removes a subscription.
func (m *SubscriptionManager) Unsubscribe(id string) {
	m.mu.Lock()
	sub, exists := m.subscriptions[id]
	if exists {
		delete(m.subscriptions, id)
	}
	m.mu.Unlock()

	if sub != nil {
		sub.Stop()
	}
}

// Count returns the number of active subscriptions.
func (m *SubscriptionManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.subscriptions)
}

// StopAll stops all subscriptions.
func (m *SubscriptionManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, sub := range m.subscriptions {
		sub.Stop()
	}
	m.subscriptions = make(map[string]*Subscription)
}

// FanoutChannel returns the shared notification channel.
func (m *SubscriptionManager) FanoutChannel() <-chan *Notification {
	return m.fanoutChan
}

// =============================================================================
// Benchmark Tests
// =============================================================================

// BenchmarkSubscription_CreateDestroy measures subscription lifecycle overhead.
func BenchmarkSubscription_CreateDestroy(b *testing.B) {
	manager := NewSubscriptionManager(10000)
	defer manager.StopAll()

	nodeIDs := []string{"ns=2;s=Tag1", "ns=2;s=Tag2"}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		id := fmt.Sprintf("sub-%d", i)
		manager.Subscribe(id, nodeIDs, time.Hour) // Long interval to avoid noise
		manager.Unsubscribe(id)
	}
}

// BenchmarkSubscription_ConcurrentCreateDestroy measures concurrent lifecycle.
func BenchmarkSubscription_ConcurrentCreateDestroy(b *testing.B) {
	manager := NewSubscriptionManager(100000)
	defer manager.StopAll()

	nodeIDs := []string{"ns=2;s=Tag1"}
	var counter int64

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx := atomic.AddInt64(&counter, 1)
			id := fmt.Sprintf("sub-%d", idx)
			manager.Subscribe(id, nodeIDs, time.Hour)
			manager.Unsubscribe(id)
		}
	})
}

// BenchmarkSubscription_NotificationFanout measures notification delivery throughput.
func BenchmarkSubscription_NotificationFanout(b *testing.B) {
	chanSizes := []int{100, 1000, 10000}

	for _, size := range chanSizes {
		b.Run(fmt.Sprintf("buf=%d", size), func(b *testing.B) {
			ch := make(chan *Notification, size)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				notif := &Notification{
					SubscriptionID: "sub-1",
					NodeID:         "ns=2;s=Tag1",
					Value:          42.0,
					Timestamp:      time.Now(),
					Quality:        192,
				}
				select {
				case ch <- notif:
				default:
					// Drain one to make room
					<-ch
					ch <- notif
				}
			}
		})
	}
}

// BenchmarkSubscription_ConcurrentNotifications measures concurrent notification writes.
func BenchmarkSubscription_ConcurrentNotifications(b *testing.B) {
	workers := []int{1, 4, 8, 16}

	for _, w := range workers {
		b.Run(fmt.Sprintf("writers=%d", w), func(b *testing.B) {
			ch := make(chan *Notification, 100000)

			// Drain consumer
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case <-ch:
					}
				}
			}()

			b.ReportAllocs()
			b.ResetTimer()
			b.SetParallelism(w)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					ch <- &Notification{
						SubscriptionID: "sub-1",
						NodeID:         "ns=2;s=Tag1",
						Value:          rand.Float64(),
						Timestamp:      time.Now(),
						Quality:        192,
					}
				}
			})

			cancel()
		})
	}
}

// =============================================================================
// Profile / Analysis Tests
// =============================================================================

// TestSubscriptionStress_ManySubscriptions tests subscription manager with many subs.
func TestSubscriptionStress_ManySubscriptions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping subscription stress test in short mode")
	}

	subCounts := []int{10, 50, 100, 500}
	nodesPerSub := 5
	interval := 10 * time.Millisecond
	duration := 2 * time.Second

	for _, count := range subCounts {
		t.Run(fmt.Sprintf("subs=%d", count), func(t *testing.T) {
			manager := NewSubscriptionManager(count * nodesPerSub * 100)

			// Create subscriptions
			nodeIDs := make([]string, nodesPerSub)
			for i := range nodeIDs {
				nodeIDs[i] = fmt.Sprintf("ns=2;s=Tag%d", i)
			}

			for i := 0; i < count; i++ {
				manager.Subscribe(fmt.Sprintf("sub-%d", i), nodeIDs, interval)
			}

			// Consume notifications
			var received int64
			ctx, cancel := context.WithTimeout(context.Background(), duration)
			defer cancel()

			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case <-manager.FanoutChannel():
						atomic.AddInt64(&received, 1)
					}
				}
			}()

			<-ctx.Done()
			manager.StopAll()

			total := atomic.LoadInt64(&received)
			rate := float64(total) / duration.Seconds()
			perSub := float64(total) / float64(count)

			t.Logf("  Subscriptions: %d", count)
			t.Logf("  Total notifications: %d", total)
			t.Logf("  Rate: %.0f/sec", rate)
			t.Logf("  Per subscription: %.0f", perSub)

			if total == 0 {
				t.Error("Expected to receive notifications")
			}
		})
	}
}

// TestSubscriptionStress_ChurnRate tests rapid create/destroy cycles.
func TestSubscriptionStress_ChurnRate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping churn rate test in short mode")
	}

	manager := NewSubscriptionManager(10000)
	defer manager.StopAll()

	nodeIDs := []string{"ns=2;s=Tag1", "ns=2;s=Tag2"}
	duration := 3 * time.Second
	interval := 100 * time.Millisecond

	var creates, destroys int64
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	// Consumer goroutine
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-manager.FanoutChannel():
			}
		}
	}()

	// Churn goroutines
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			i := 0
			for {
				select {
				case <-ctx.Done():
					return
				default:
					id := fmt.Sprintf("w%d-sub-%d", workerID, i)
					manager.Subscribe(id, nodeIDs, interval)
					atomic.AddInt64(&creates, 1)

					time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)

					manager.Unsubscribe(id)
					atomic.AddInt64(&destroys, 1)
					i++
				}
			}
		}(w)
	}

	<-ctx.Done()
	wg.Wait()

	c := atomic.LoadInt64(&creates)
	d := atomic.LoadInt64(&destroys)
	remaining := manager.Count()

	t.Logf("Churn test (%.0fs, 4 workers):", duration.Seconds())
	t.Logf("  Creates: %d (%.0f/sec)", c, float64(c)/duration.Seconds())
	t.Logf("  Destroys: %d (%.0f/sec)", d, float64(d)/duration.Seconds())
	t.Logf("  Remaining: %d", remaining)

	if c == 0 {
		t.Error("Expected subscription creates")
	}
}

// TestSubscriptionStress_FanoutScaling tests notification delivery scaling.
func TestSubscriptionStress_FanoutScaling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping fanout scaling test in short mode")
	}

	consumerCounts := []int{1, 4, 8}

	for _, consumers := range consumerCounts {
		t.Run(fmt.Sprintf("consumers=%d", consumers), func(t *testing.T) {
			manager := NewSubscriptionManager(100000)

			// Create 20 subscriptions
			for i := 0; i < 20; i++ {
				manager.Subscribe(
					fmt.Sprintf("sub-%d", i),
					[]string{"ns=2;s=Tag1"},
					5*time.Millisecond,
				)
			}

			var totalConsumed int64
			perConsumer := make([]int64, consumers)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			var wg sync.WaitGroup
			for c := 0; c < consumers; c++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					for {
						select {
						case <-ctx.Done():
							return
						case <-manager.FanoutChannel():
							atomic.AddInt64(&perConsumer[idx], 1)
							atomic.AddInt64(&totalConsumed, 1)
						}
					}
				}(c)
			}

			<-ctx.Done()
			manager.StopAll()
			wg.Wait()

			total := atomic.LoadInt64(&totalConsumed)
			t.Logf("Consumers=%d: total=%d (%.0f/sec)", consumers, total, float64(total)/2.0)
			for i := 0; i < consumers; i++ {
				t.Logf("  Consumer %d: %d", i, atomic.LoadInt64(&perConsumer[i]))
			}

			if total == 0 {
				t.Error("Expected consumed notifications")
			}
		})
	}
}

// TestSubscriptionStress_ChannelBackpressure tests behavior when channel is full.
func TestSubscriptionStress_ChannelBackpressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping backpressure test in short mode")
	}

	// Small channel buffer to trigger backpressure
	manager := NewSubscriptionManager(100)

	// Create many fast subscriptions
	for i := 0; i < 50; i++ {
		manager.Subscribe(
			fmt.Sprintf("sub-%d", i),
			[]string{"ns=2;s=Tag1", "ns=2;s=Tag2"},
			1*time.Millisecond,
		)
	}

	// Slow consumer
	var consumed int64
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-manager.FanoutChannel():
				atomic.AddInt64(&consumed, 1)
				time.Sleep(100 * time.Microsecond) // Slow consumer
			}
		}
	}()

	<-ctx.Done()
	manager.StopAll()

	// Calculate total notifications attempted
	var totalAttempted int64
	manager.mu.RLock()
	for _, sub := range manager.subscriptions {
		totalAttempted += sub.NotifyCount()
	}
	manager.mu.RUnlock()

	c := atomic.LoadInt64(&consumed)
	t.Logf("Backpressure test (buf=100, 50 subs):")
	t.Logf("  Consumed: %d", c)
	t.Logf("  Note: Dropped notifications are expected with small buffer + slow consumer")

	if c == 0 {
		t.Error("Expected to consume some notifications")
	}
}
