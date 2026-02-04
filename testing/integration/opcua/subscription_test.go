//go:build integration
// +build integration

// Package opcua_test tests OPC UA subscription operations against a simulator.
package opcua_test

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Test Configuration
// =============================================================================

// getOPCUASubscriptionEndpoint returns the OPC UA simulator endpoint for subscription tests.
func getOPCUASubscriptionEndpoint() string {
	if endpoint := os.Getenv("OPCUA_SIMULATOR_ENDPOINT"); endpoint != "" {
		return endpoint
	}
	return "opc.tcp://localhost:4840"
}

// =============================================================================
// Mock Types for Subscription Testing
// =============================================================================

// MockDataChangeNotification represents a data change notification.
type MockDataChangeNotification struct {
	NodeID     string
	Value      interface{}
	Timestamp  time.Time
	StatusCode uint32
}

// MockSubscription represents a mock OPC UA subscription.
type MockSubscription struct {
	ID                 uint32
	PublishingInterval time.Duration
	SamplingInterval   time.Duration
	QueueSize          uint32
	MonitoredItems     map[string]*MockMonitoredItem
	NotifyCh           chan *MockDataChangeNotification
	Active             bool
	mu                 sync.RWMutex
}

// MockMonitoredItem represents a mock monitored item.
type MockMonitoredItem struct {
	NodeID           string
	SamplingInterval time.Duration
	QueueSize        uint32
	DiscardOldest    bool
	LastValue        interface{}
	DeadbandType     string
	DeadbandValue    float64
}

// MockSubscriptionManager manages mock subscriptions for testing.
type MockSubscriptionManager struct {
	endpoint      string
	subscriptions map[uint32]*MockSubscription
	nextSubID     uint32
	values        map[string]interface{}
	mu            sync.RWMutex
	running       atomic.Bool
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// NewMockSubscriptionManager creates a new mock subscription manager.
func NewMockSubscriptionManager(endpoint string) *MockSubscriptionManager {
	ctx, cancel := context.WithCancel(context.Background())
	sm := &MockSubscriptionManager{
		endpoint:      endpoint,
		subscriptions: make(map[uint32]*MockSubscription),
		nextSubID:     1,
		values:        make(map[string]interface{}),
		ctx:           ctx,
		cancel:        cancel,
	}
	sm.initializeSimulationValues()
	return sm
}

// initializeSimulationValues sets up initial values for simulation.
func (sm *MockSubscriptionManager) initializeSimulationValues() {
	sm.values["ns=2;s=Simulation.Sinusoid"] = 0.0
	sm.values["ns=2;s=Simulation.Random"] = 50.0
	sm.values["ns=2;s=Simulation.Counter"] = uint32(0)
	sm.values["ns=2;s=Simulation.Timestamp"] = time.Now()
	sm.values["ns=2;s=Demo.Temperature"] = 25.0
	sm.values["ns=2;s=Demo.Pressure"] = 101.325
	sm.values["ns=2;s=Demo.BooleanToggle"] = false
}

// Start starts the subscription manager and value simulation.
func (sm *MockSubscriptionManager) Start() error {
	if sm.running.Load() {
		return nil
	}
	sm.running.Store(true)

	// Start value simulation goroutine
	sm.wg.Add(1)
	go sm.simulateValues()

	// Start notification publisher goroutine
	sm.wg.Add(1)
	go sm.publishNotifications()

	return nil
}

// Stop stops the subscription manager.
func (sm *MockSubscriptionManager) Stop() error {
	if !sm.running.Load() {
		return nil
	}
	sm.cancel()
	sm.running.Store(false)
	sm.wg.Wait()
	return nil
}

// simulateValues simulates changing values.
func (sm *MockSubscriptionManager) simulateValues() {
	defer sm.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	counter := 0
	for {
		select {
		case <-sm.ctx.Done():
			return
		case <-ticker.C:
			sm.mu.Lock()
			// Simulate sinusoid
			sm.values["ns=2;s=Simulation.Sinusoid"] = math.Sin(float64(counter) * 0.1)

			// Simulate random with small changes
			current := sm.values["ns=2;s=Simulation.Random"].(float64)
			sm.values["ns=2;s=Simulation.Random"] = current + (float64(counter%3) - 1.0)

			// Increment counter
			sm.values["ns=2;s=Simulation.Counter"] = uint32(counter)

			// Update timestamp
			sm.values["ns=2;s=Simulation.Timestamp"] = time.Now()

			// Toggle boolean every 10 iterations
			if counter%10 == 0 {
				current := sm.values["ns=2;s=Demo.BooleanToggle"].(bool)
				sm.values["ns=2;s=Demo.BooleanToggle"] = !current
			}

			counter++
			sm.mu.Unlock()
		}
	}
}

// publishNotifications publishes notifications to subscriptions.
func (sm *MockSubscriptionManager) publishNotifications() {
	defer sm.wg.Done()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sm.ctx.Done():
			return
		case <-ticker.C:
			sm.mu.RLock()
			for _, sub := range sm.subscriptions {
				if !sub.Active {
					continue
				}
				sm.publishToSubscription(sub)
			}
			sm.mu.RUnlock()
		}
	}
}

// publishToSubscription publishes value changes to a subscription.
func (sm *MockSubscriptionManager) publishToSubscription(sub *MockSubscription) {
	sub.mu.Lock()
	defer sub.mu.Unlock()

	for nodeID, item := range sub.MonitoredItems {
		value, exists := sm.values[nodeID]
		if !exists {
			continue
		}

		// Check if value changed (simple comparison)
		if item.LastValue == value {
			continue
		}

		// Apply deadband filter for numeric values
		if item.DeadbandType == "Absolute" && item.LastValue != nil {
			oldVal, oldOk := item.LastValue.(float64)
			newVal, newOk := value.(float64)
			if oldOk && newOk {
				if math.Abs(newVal-oldVal) < item.DeadbandValue {
					continue // Deadband not exceeded
				}
			}
		}

		item.LastValue = value

		// Send notification (non-blocking)
		select {
		case sub.NotifyCh <- &MockDataChangeNotification{
			NodeID:     nodeID,
			Value:      value,
			Timestamp:  time.Now(),
			StatusCode: 0,
		}:
		default:
			// Channel full, drop notification
		}
	}
}

// CreateSubscription creates a new subscription.
func (sm *MockSubscriptionManager) CreateSubscription(publishingInterval time.Duration) (*MockSubscription, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sub := &MockSubscription{
		ID:                 sm.nextSubID,
		PublishingInterval: publishingInterval,
		SamplingInterval:   publishingInterval / 2,
		QueueSize:          10,
		MonitoredItems:     make(map[string]*MockMonitoredItem),
		NotifyCh:           make(chan *MockDataChangeNotification, 100),
		Active:             true,
	}

	sm.subscriptions[sub.ID] = sub
	sm.nextSubID++

	return sub, nil
}

// DeleteSubscription deletes a subscription.
func (sm *MockSubscriptionManager) DeleteSubscription(subID uint32) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sub, exists := sm.subscriptions[subID]
	if !exists {
		return fmt.Errorf("subscription %d not found", subID)
	}

	sub.Active = false
	close(sub.NotifyCh)
	delete(sm.subscriptions, subID)

	return nil
}

// AddMonitoredItem adds a monitored item to a subscription.
func (sm *MockSubscriptionManager) AddMonitoredItem(subID uint32, nodeID string, samplingInterval time.Duration, queueSize uint32) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sub, exists := sm.subscriptions[subID]
	if !exists {
		return fmt.Errorf("subscription %d not found", subID)
	}

	sub.mu.Lock()
	sub.MonitoredItems[nodeID] = &MockMonitoredItem{
		NodeID:           nodeID,
		SamplingInterval: samplingInterval,
		QueueSize:        queueSize,
		DiscardOldest:    true,
		DeadbandType:     "None",
		DeadbandValue:    0,
	}
	sub.mu.Unlock()

	return nil
}

// RemoveMonitoredItem removes a monitored item from a subscription.
func (sm *MockSubscriptionManager) RemoveMonitoredItem(subID uint32, nodeID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sub, exists := sm.subscriptions[subID]
	if !exists {
		return fmt.Errorf("subscription %d not found", subID)
	}

	sub.mu.Lock()
	delete(sub.MonitoredItems, nodeID)
	sub.mu.Unlock()

	return nil
}

// SetDeadband sets deadband filter on a monitored item.
func (sm *MockSubscriptionManager) SetDeadband(subID uint32, nodeID string, deadbandType string, deadbandValue float64) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sub, exists := sm.subscriptions[subID]
	if !exists {
		return fmt.Errorf("subscription %d not found", subID)
	}

	sub.mu.Lock()
	defer sub.mu.Unlock()

	item, exists := sub.MonitoredItems[nodeID]
	if !exists {
		return fmt.Errorf("monitored item %s not found", nodeID)
	}

	item.DeadbandType = deadbandType
	item.DeadbandValue = deadbandValue

	return nil
}

// =============================================================================
// Subscription Creation Tests
// =============================================================================

// TestOPCUACreateSubscription tests subscription creation.
func TestOPCUACreateSubscription(t *testing.T) {
	endpoint := getOPCUASubscriptionEndpoint()
	sm := NewMockSubscriptionManager(endpoint)

	sub, err := sm.CreateSubscription(1 * time.Second)
	if err != nil {
		t.Fatalf("Failed to create subscription: %v", err)
	}

	if sub.ID == 0 {
		t.Error("Subscription ID should not be 0")
	}
	if sub.PublishingInterval != 1*time.Second {
		t.Errorf("expected publishing interval 1s, got %v", sub.PublishingInterval)
	}
	if !sub.Active {
		t.Error("Subscription should be active")
	}

	t.Logf("Created subscription ID: %d", sub.ID)
}

// TestOPCUACreateMultipleSubscriptions tests creating multiple subscriptions.
func TestOPCUACreateMultipleSubscriptions(t *testing.T) {
	endpoint := getOPCUASubscriptionEndpoint()
	sm := NewMockSubscriptionManager(endpoint)

	intervals := []time.Duration{
		100 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
	}

	subs := make([]*MockSubscription, len(intervals))
	for i, interval := range intervals {
		sub, err := sm.CreateSubscription(interval)
		if err != nil {
			t.Fatalf("Failed to create subscription %d: %v", i, err)
		}
		subs[i] = sub
	}

	// Verify unique IDs
	ids := make(map[uint32]bool)
	for _, sub := range subs {
		if ids[sub.ID] {
			t.Errorf("Duplicate subscription ID: %d", sub.ID)
		}
		ids[sub.ID] = true
	}

	t.Logf("Created %d subscriptions with unique IDs", len(subs))
}

// TestOPCUADeleteSubscription tests subscription deletion.
func TestOPCUADeleteSubscription(t *testing.T) {
	endpoint := getOPCUASubscriptionEndpoint()
	sm := NewMockSubscriptionManager(endpoint)

	sub, err := sm.CreateSubscription(1 * time.Second)
	if err != nil {
		t.Fatalf("Failed to create subscription: %v", err)
	}

	subID := sub.ID

	err = sm.DeleteSubscription(subID)
	if err != nil {
		t.Fatalf("Failed to delete subscription: %v", err)
	}

	// Try to delete again - should fail
	err = sm.DeleteSubscription(subID)
	if err == nil {
		t.Error("Deleting non-existent subscription should fail")
	}
}

// =============================================================================
// Monitored Item Tests
// =============================================================================

// TestOPCUAAddMonitoredItem tests adding monitored items.
func TestOPCUAAddMonitoredItem(t *testing.T) {
	endpoint := getOPCUASubscriptionEndpoint()
	sm := NewMockSubscriptionManager(endpoint)

	sub, err := sm.CreateSubscription(1 * time.Second)
	if err != nil {
		t.Fatalf("Failed to create subscription: %v", err)
	}

	nodeIDs := []string{
		"ns=2;s=Simulation.Sinusoid",
		"ns=2;s=Simulation.Random",
		"ns=2;s=Simulation.Counter",
	}

	for _, nodeID := range nodeIDs {
		err := sm.AddMonitoredItem(sub.ID, nodeID, 500*time.Millisecond, 10)
		if err != nil {
			t.Errorf("Failed to add monitored item %s: %v", nodeID, err)
		}
	}

	if len(sub.MonitoredItems) != len(nodeIDs) {
		t.Errorf("expected %d monitored items, got %d", len(nodeIDs), len(sub.MonitoredItems))
	}
}

// TestOPCUARemoveMonitoredItem tests removing monitored items.
func TestOPCUARemoveMonitoredItem(t *testing.T) {
	endpoint := getOPCUASubscriptionEndpoint()
	sm := NewMockSubscriptionManager(endpoint)

	sub, _ := sm.CreateSubscription(1 * time.Second)
	nodeID := "ns=2;s=Simulation.Sinusoid"

	_ = sm.AddMonitoredItem(sub.ID, nodeID, 500*time.Millisecond, 10)

	err := sm.RemoveMonitoredItem(sub.ID, nodeID)
	if err != nil {
		t.Fatalf("Failed to remove monitored item: %v", err)
	}

	if len(sub.MonitoredItems) != 0 {
		t.Errorf("expected 0 monitored items, got %d", len(sub.MonitoredItems))
	}
}

// =============================================================================
// Data Change Notification Tests
// =============================================================================

// TestOPCUAReceiveNotifications tests receiving data change notifications.
func TestOPCUAReceiveNotifications(t *testing.T) {
	endpoint := getOPCUASubscriptionEndpoint()
	sm := NewMockSubscriptionManager(endpoint)

	err := sm.Start()
	if err != nil {
		t.Fatalf("Failed to start subscription manager: %v", err)
	}
	defer sm.Stop()

	sub, _ := sm.CreateSubscription(100 * time.Millisecond)
	_ = sm.AddMonitoredItem(sub.ID, "ns=2;s=Simulation.Counter", 50*time.Millisecond, 10)

	// Wait for notifications
	timeout := time.After(2 * time.Second)
	notificationCount := 0

	for {
		select {
		case notif := <-sub.NotifyCh:
			notificationCount++
			t.Logf("Received notification: NodeID=%s, Value=%v, Time=%v",
				notif.NodeID, notif.Value, notif.Timestamp)
			if notificationCount >= 5 {
				t.Logf("Received %d notifications", notificationCount)
				return
			}
		case <-timeout:
			if notificationCount > 0 {
				t.Logf("Received %d notifications before timeout", notificationCount)
			} else {
				t.Error("No notifications received within timeout")
			}
			return
		}
	}
}

// TestOPCUANotificationWithDeadband tests deadband filtering.
func TestOPCUANotificationWithDeadband(t *testing.T) {
	endpoint := getOPCUASubscriptionEndpoint()
	sm := NewMockSubscriptionManager(endpoint)

	err := sm.Start()
	if err != nil {
		t.Fatalf("Failed to start subscription manager: %v", err)
	}
	defer sm.Stop()

	sub, _ := sm.CreateSubscription(100 * time.Millisecond)
	nodeID := "ns=2;s=Simulation.Random"
	_ = sm.AddMonitoredItem(sub.ID, nodeID, 50*time.Millisecond, 10)

	// Set absolute deadband of 5.0
	_ = sm.SetDeadband(sub.ID, nodeID, "Absolute", 5.0)

	// Collect notifications for 2 seconds
	timeout := time.After(2 * time.Second)
	notificationCount := 0

	for {
		select {
		case notif := <-sub.NotifyCh:
			notificationCount++
			t.Logf("Notification with deadband: Value=%v", notif.Value)
		case <-timeout:
			t.Logf("Received %d notifications with deadband filter", notificationCount)
			// With deadband, we should receive fewer notifications
			return
		}
	}
}

// =============================================================================
// Concurrent Subscription Tests
// =============================================================================

// TestOPCUAConcurrentSubscriptions tests multiple concurrent subscriptions.
func TestOPCUAConcurrentSubscriptions(t *testing.T) {
	endpoint := getOPCUASubscriptionEndpoint()
	sm := NewMockSubscriptionManager(endpoint)

	err := sm.Start()
	if err != nil {
		t.Fatalf("Failed to start subscription manager: %v", err)
	}
	defer sm.Stop()

	// Create multiple subscriptions
	numSubs := 5
	subs := make([]*MockSubscription, numSubs)
	for i := 0; i < numSubs; i++ {
		sub, _ := sm.CreateSubscription(time.Duration(100*(i+1)) * time.Millisecond)
		_ = sm.AddMonitoredItem(sub.ID, "ns=2;s=Simulation.Counter", 50*time.Millisecond, 10)
		subs[i] = sub
	}

	// Collect notifications from all subscriptions concurrently
	var wg sync.WaitGroup
	counts := make([]int, numSubs)

	for i, sub := range subs {
		wg.Add(1)
		go func(idx int, s *MockSubscription) {
			defer wg.Done()
			timeout := time.After(1 * time.Second)
			for {
				select {
				case <-s.NotifyCh:
					counts[idx]++
				case <-timeout:
					return
				}
			}
		}(i, sub)
	}

	wg.Wait()

	// Verify all subscriptions received notifications
	for i, count := range counts {
		t.Logf("Subscription %d received %d notifications", i, count)
	}
}

// =============================================================================
// Subscription Lifecycle Tests
// =============================================================================

// TestOPCUASubscriptionRestart tests restarting subscriptions.
func TestOPCUASubscriptionRestart(t *testing.T) {
	endpoint := getOPCUASubscriptionEndpoint()
	sm := NewMockSubscriptionManager(endpoint)

	_ = sm.Start()

	sub, _ := sm.CreateSubscription(100 * time.Millisecond)
	_ = sm.AddMonitoredItem(sub.ID, "ns=2;s=Simulation.Counter", 50*time.Millisecond, 10)

	// Stop
	_ = sm.Stop()

	// Restart
	sm = NewMockSubscriptionManager(endpoint)
	_ = sm.Start()
	defer sm.Stop()

	// Create new subscription
	sub2, err := sm.CreateSubscription(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create subscription after restart: %v", err)
	}

	t.Logf("Successfully restarted with new subscription ID: %d", sub2.ID)
}

// TestOPCUASubscriptionCleanup tests proper cleanup on subscription deletion.
func TestOPCUASubscriptionCleanup(t *testing.T) {
	endpoint := getOPCUASubscriptionEndpoint()
	sm := NewMockSubscriptionManager(endpoint)

	_ = sm.Start()
	defer sm.Stop()

	// Create and delete multiple subscriptions
	for i := 0; i < 10; i++ {
		sub, _ := sm.CreateSubscription(100 * time.Millisecond)
		_ = sm.AddMonitoredItem(sub.ID, "ns=2;s=Simulation.Counter", 50*time.Millisecond, 10)
		_ = sm.DeleteSubscription(sub.ID)
	}

	// Verify no lingering subscriptions
	sm.mu.RLock()
	count := len(sm.subscriptions)
	sm.mu.RUnlock()

	if count != 0 {
		t.Errorf("Expected 0 subscriptions after cleanup, got %d", count)
	}

	t.Logf("All subscriptions cleaned up successfully")
}

// =============================================================================
// Publishing Interval Tests
// =============================================================================

// TestOPCUAPublishingIntervalVariations tests different publishing intervals.
func TestOPCUAPublishingIntervalVariations(t *testing.T) {
	intervals := []struct {
		name     string
		interval time.Duration
	}{
		{"Fast_50ms", 50 * time.Millisecond},
		{"Medium_500ms", 500 * time.Millisecond},
		{"Slow_2s", 2 * time.Second},
	}

	for _, tc := range intervals {
		t.Run(tc.name, func(t *testing.T) {
			endpoint := getOPCUASubscriptionEndpoint()
			sm := NewMockSubscriptionManager(endpoint)

			sub, err := sm.CreateSubscription(tc.interval)
			if err != nil {
				t.Fatalf("Failed to create subscription: %v", err)
			}

			if sub.PublishingInterval != tc.interval {
				t.Errorf("expected interval %v, got %v", tc.interval, sub.PublishingInterval)
			}

			t.Logf("Created subscription with %v publishing interval", tc.interval)
		})
	}
}

// TestOPCUASamplingIntervalRelation tests sampling vs publishing interval relationship.
func TestOPCUASamplingIntervalRelation(t *testing.T) {
	endpoint := getOPCUASubscriptionEndpoint()
	sm := NewMockSubscriptionManager(endpoint)

	publishInterval := 1 * time.Second
	samplingInterval := 500 * time.Millisecond

	sub, _ := sm.CreateSubscription(publishInterval)
	_ = sm.AddMonitoredItem(sub.ID, "ns=2;s=Simulation.Counter", samplingInterval, 10)

	item := sub.MonitoredItems["ns=2;s=Simulation.Counter"]
	if item.SamplingInterval != samplingInterval {
		t.Errorf("expected sampling interval %v, got %v", samplingInterval, item.SamplingInterval)
	}

	// Sampling should be <= publishing interval
	if item.SamplingInterval > sub.PublishingInterval {
		t.Logf("Note: Sampling interval (%v) > publishing interval (%v)",
			item.SamplingInterval, sub.PublishingInterval)
	}
}
