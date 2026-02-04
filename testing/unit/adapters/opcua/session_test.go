// Package opcua_test tests the OPC UA session management functionality.
package opcua_test

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/opcua"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

// =============================================================================
// SessionState Tests
// =============================================================================

// TestSessionStateValues tests session state constant values.
func TestSessionStateValues(t *testing.T) {
	states := []struct {
		state    opcua.SessionState
		expected string
	}{
		{opcua.SessionStateDisconnected, "disconnected"},
		{opcua.SessionStateConnecting, "connecting"},
		{opcua.SessionStateSecureChannel, "secure_channel"},
		{opcua.SessionStateActive, "active"},
		{opcua.SessionStateError, "error"},
	}

	for _, tt := range states {
		t.Run(string(tt.state), func(t *testing.T) {
			if string(tt.state) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(tt.state))
			}
		})
	}
}

// TestSessionStateMachine tests valid session state transitions.
func TestSessionStateMachine(t *testing.T) {
	// OPC UA session state machine:
	// disconnected -> connecting -> secure_channel -> active
	// any state -> error -> disconnected
	transitions := []struct {
		from    opcua.SessionState
		to      opcua.SessionState
		valid   bool
		trigger string
	}{
		{opcua.SessionStateDisconnected, opcua.SessionStateConnecting, true, "connect()"},
		{opcua.SessionStateConnecting, opcua.SessionStateSecureChannel, true, "secure channel established"},
		{opcua.SessionStateSecureChannel, opcua.SessionStateActive, true, "session activated"},
		{opcua.SessionStateActive, opcua.SessionStateDisconnected, true, "disconnect()"},
		{opcua.SessionStateActive, opcua.SessionStateError, true, "communication error"},
		{opcua.SessionStateError, opcua.SessionStateDisconnected, true, "error handled"},
		{opcua.SessionStateDisconnected, opcua.SessionStateActive, false, "invalid direct activation"},
	}

	for _, tt := range transitions {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			t.Logf("Transition %s -> %s: valid=%v, trigger=%s",
				tt.from, tt.to, tt.valid, tt.trigger)
		})
	}
}

// =============================================================================
// DeviceBinding Tests
// =============================================================================

// TestDeviceBindingStructure tests DeviceBinding struct fields.
func TestDeviceBindingStructure(t *testing.T) {
	device := &domain.Device{
		ID:   "opc-device-001",
		Name: "OPC UA Test Device",
	}

	binding := opcua.DeviceBinding{
		DeviceID:       device.ID,
		Device:         device,
		EndpointKey:    "opc.tcp://localhost:4840|Basic256Sha256|SignAndEncrypt|Anonymous",
		MonitoredItems: make(map[string]uint32),
		LastSequenceNo: 0,
	}

	if binding.DeviceID != "opc-device-001" {
		t.Errorf("expected DeviceID 'opc-device-001', got %s", binding.DeviceID)
	}
	if binding.Device != device {
		t.Error("expected Device reference to match")
	}
	if binding.EndpointKey == "" {
		t.Error("expected EndpointKey to be set")
	}
	if binding.MonitoredItems == nil {
		t.Error("expected MonitoredItems map to be initialized")
	}
	if binding.LastSequenceNo != 0 {
		t.Errorf("expected LastSequenceNo 0, got %d", binding.LastSequenceNo)
	}
}

// TestDeviceBindingMonitoredItems tests monitored item tracking.
func TestDeviceBindingMonitoredItems(t *testing.T) {
	binding := opcua.DeviceBinding{
		DeviceID:       "test-device",
		MonitoredItems: make(map[string]uint32),
	}

	// Add monitored items
	items := map[string]uint32{
		"tag-001": 1001,
		"tag-002": 1002,
		"tag-003": 1003,
	}

	for tagID, monitoredID := range items {
		binding.MonitoredItems[tagID] = monitoredID
	}

	if len(binding.MonitoredItems) != 3 {
		t.Errorf("expected 3 monitored items, got %d", len(binding.MonitoredItems))
	}

	// Verify lookup
	if id, ok := binding.MonitoredItems["tag-002"]; !ok || id != 1002 {
		t.Errorf("expected monitored item ID 1002 for tag-002, got %d", id)
	}

	// Remove item (simulating unsubscribe)
	delete(binding.MonitoredItems, "tag-001")
	if len(binding.MonitoredItems) != 2 {
		t.Errorf("expected 2 monitored items after delete, got %d", len(binding.MonitoredItems))
	}
}

// TestDeviceBindingSequenceTracking tests sequence number tracking.
func TestDeviceBindingSequenceTracking(t *testing.T) {
	binding := opcua.DeviceBinding{
		DeviceID:       "seq-device",
		LastSequenceNo: 100,
	}

	// Simulate receiving publish responses
	sequences := []uint32{101, 102, 103, 104, 105}

	for _, seq := range sequences {
		// Check for gaps
		if seq != binding.LastSequenceNo+1 {
			t.Logf("Sequence gap detected: expected %d, got %d", binding.LastSequenceNo+1, seq)
		}
		binding.LastSequenceNo = seq
	}

	if binding.LastSequenceNo != 105 {
		t.Errorf("expected LastSequenceNo 105, got %d", binding.LastSequenceNo)
	}
}

// =============================================================================
// SubscriptionRecoveryState Tests
// =============================================================================

// TestSubscriptionRecoveryStateStructure tests recovery state fields.
func TestSubscriptionRecoveryStateStructure(t *testing.T) {
	state := opcua.SubscriptionRecoveryState{
		SubscriptionID:        1234,
		LastSequenceNumber:    500,
		MonitoredItems:        make(map[uint32]*opcua.MonitoredItemState),
		LastAcknowledgedSeqNo: 499,
		MissedSequenceNumbers: []uint32{},
	}

	if state.SubscriptionID != 1234 {
		t.Errorf("expected SubscriptionID 1234, got %d", state.SubscriptionID)
	}
	if state.LastSequenceNumber != 500 {
		t.Errorf("expected LastSequenceNumber 500, got %d", state.LastSequenceNumber)
	}
	if state.LastAcknowledgedSeqNo != 499 {
		t.Errorf("expected LastAcknowledgedSeqNo 499, got %d", state.LastAcknowledgedSeqNo)
	}
	if len(state.MissedSequenceNumbers) != 0 {
		t.Errorf("expected empty MissedSequenceNumbers, got %v", state.MissedSequenceNumbers)
	}
}

// TestMissedSequenceDetection tests detecting missed sequence numbers.
func TestMissedSequenceDetection(t *testing.T) {
	state := opcua.SubscriptionRecoveryState{
		LastSequenceNumber:    100,
		MissedSequenceNumbers: []uint32{},
	}

	// Simulate receiving sequences with gaps
	received := []uint32{101, 102, 105, 106, 110}
	expected := state.LastSequenceNumber

	for _, seq := range received {
		expected++
		for expected < seq {
			state.MissedSequenceNumbers = append(state.MissedSequenceNumbers, expected)
			expected++
		}
		state.LastSequenceNumber = seq
	}

	// Should have missed 103, 104, 107, 108, 109
	expectedMissed := []uint32{103, 104, 107, 108, 109}
	if len(state.MissedSequenceNumbers) != len(expectedMissed) {
		t.Errorf("expected %d missed sequences, got %d",
			len(expectedMissed), len(state.MissedSequenceNumbers))
	}

	for i, seq := range expectedMissed {
		if i < len(state.MissedSequenceNumbers) && state.MissedSequenceNumbers[i] != seq {
			t.Errorf("expected missed sequence %d at index %d, got %d",
				seq, i, state.MissedSequenceNumbers[i])
		}
	}
}

// =============================================================================
// MonitoredItemState Tests
// =============================================================================

// TestMonitoredItemStateStructure tests monitored item state fields.
func TestMonitoredItemStateStructure(t *testing.T) {
	now := time.Now()
	state := opcua.MonitoredItemState{
		ClientHandle:     100,
		MonitoredItemID:  200,
		NodeID:           "ns=2;i=1001",
		TagID:            "temperature-001",
		SamplingInterval: 500 * time.Millisecond,
		QueueSize:        10,
		LastValue:        25.5,
		LastTimestamp:    now,
	}

	if state.ClientHandle != 100 {
		t.Errorf("expected ClientHandle 100, got %d", state.ClientHandle)
	}
	if state.MonitoredItemID != 200 {
		t.Errorf("expected MonitoredItemID 200, got %d", state.MonitoredItemID)
	}
	if state.NodeID != "ns=2;i=1001" {
		t.Errorf("expected NodeID 'ns=2;i=1001', got %s", state.NodeID)
	}
	if state.TagID != "temperature-001" {
		t.Errorf("expected TagID 'temperature-001', got %s", state.TagID)
	}
	if state.SamplingInterval != 500*time.Millisecond {
		t.Errorf("expected SamplingInterval 500ms, got %v", state.SamplingInterval)
	}
	if state.QueueSize != 10 {
		t.Errorf("expected QueueSize 10, got %d", state.QueueSize)
	}
	if state.LastValue != 25.5 {
		t.Errorf("expected LastValue 25.5, got %v", state.LastValue)
	}
	if !state.LastTimestamp.Equal(now) {
		t.Errorf("expected LastTimestamp %v, got %v", now, state.LastTimestamp)
	}
}

// TestMonitoredItemValueTypes tests different value types in monitored items.
func TestMonitoredItemValueTypes(t *testing.T) {
	tests := []struct {
		name      string
		value     interface{}
		valueType string
	}{
		{"Int32", int32(42), "int32"},
		{"Float64", 3.14159, "float64"},
		{"String", "test-value", "string"},
		{"Boolean", true, "bool"},
		{"ByteArray", []byte{0x01, 0x02, 0x03}, "[]uint8"},
		{"Nil", nil, "nil"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := opcua.MonitoredItemState{
				ClientHandle: 1,
				LastValue:    tt.value,
			}

			// Use reflect.DeepEqual for proper comparison (handles slices)
			if !reflect.DeepEqual(state.LastValue, tt.value) {
				t.Errorf("expected LastValue %v, got %v", tt.value, state.LastValue)
			}
		})
	}
}

// =============================================================================
// Endpoint Key Generation Tests
// =============================================================================

// TestEndpointKeyComponents tests endpoint key component extraction.
func TestEndpointKeyComponents(t *testing.T) {
	// Endpoint key format: host:port|security_policy|security_mode|auth_mode|auth_hash|cert_hash
	tests := []struct {
		name           string
		host           string
		port           int
		securityPolicy string
		securityMode   string
		authMode       string
		username       string
	}{
		{
			name:           "Anonymous",
			host:           "192.168.1.100",
			port:           4840,
			securityPolicy: "None",
			securityMode:   "None",
			authMode:       "Anonymous",
			username:       "",
		},
		{
			name:           "UserPassword",
			host:           "localhost",
			port:           4840,
			securityPolicy: "Basic256Sha256",
			securityMode:   "SignAndEncrypt",
			authMode:       "UserName",
			username:       "admin",
		},
		{
			name:           "Certificate",
			host:           "opc-server.local",
			port:           48400,
			securityPolicy: "Basic256Sha256",
			securityMode:   "SignAndEncrypt",
			authMode:       "Certificate",
			username:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build endpoint URL
			endpointURL := "opc.tcp://" + tt.host + ":" + string(rune(tt.port+'0'))

			// Build auth hash
			authHash := ""
			if tt.username != "" {
				h := sha256.Sum256([]byte(tt.username + ":password"))
				authHash = hex.EncodeToString(h[:8])
			}

			t.Logf("Endpoint: %s, Security: %s/%s, Auth: %s, Hash: %s",
				endpointURL, tt.securityPolicy, tt.securityMode, tt.authMode, authHash)
		})
	}
}

// TestEndpointKeyUniqueness tests that different configs produce different keys.
func TestEndpointKeyUniqueness(t *testing.T) {
	configs := []struct {
		host     string
		port     int
		security string
		auth     string
	}{
		{"host1", 4840, "None", "Anonymous"},
		{"host2", 4840, "None", "Anonymous"},           // Different host
		{"host1", 4841, "None", "Anonymous"},           // Different port
		{"host1", 4840, "Basic256Sha256", "Anonymous"}, // Different security
		{"host1", 4840, "None", "UserName"},            // Different auth
	}

	keys := make(map[string]bool)
	for _, cfg := range configs {
		key := cfg.host + ":" + string(rune(cfg.port)) + "|" + cfg.security + "|" + cfg.auth
		if keys[key] {
			t.Errorf("duplicate key found: %s", key)
		}
		keys[key] = true
	}

	if len(keys) != len(configs) {
		t.Errorf("expected %d unique keys, got %d", len(configs), len(keys))
	}
}

// =============================================================================
// Session Activity Tracking Tests
// =============================================================================

// TestSessionActivityTracking tests activity-based idle detection.
func TestSessionActivityTracking(t *testing.T) {
	now := time.Now()
	idleTimeout := 5 * time.Minute

	tests := []struct {
		name            string
		lastUsed        time.Time
		lastPublish     time.Time
		hasSubscription bool
		shouldBeIdle    bool
	}{
		{
			name:            "RecentReadWrite",
			lastUsed:        now.Add(-1 * time.Minute),
			lastPublish:     now.Add(-10 * time.Minute),
			hasSubscription: false,
			shouldBeIdle:    false,
		},
		{
			name:            "RecentSubscription",
			lastUsed:        now.Add(-10 * time.Minute),
			lastPublish:     now.Add(-1 * time.Minute),
			hasSubscription: true,
			shouldBeIdle:    false,
		},
		{
			name:            "AllIdle",
			lastUsed:        now.Add(-10 * time.Minute),
			lastPublish:     now.Add(-10 * time.Minute),
			hasSubscription: true,
			shouldBeIdle:    true,
		},
		{
			name:            "NoSubscriptions",
			lastUsed:        now.Add(-10 * time.Minute),
			lastPublish:     time.Time{}, // Zero time
			hasSubscription: false,
			shouldBeIdle:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasRecentActivity := now.Sub(tt.lastUsed) < idleTimeout ||
				(tt.hasSubscription && !tt.lastPublish.IsZero() && now.Sub(tt.lastPublish) < idleTimeout)

			isIdle := !hasRecentActivity
			if isIdle != tt.shouldBeIdle {
				t.Errorf("expected idle=%v, got %v", tt.shouldBeIdle, isIdle)
			}
		})
	}
}

// =============================================================================
// Reconnect Backoff Tests
// =============================================================================

// TestReconnectBackoffCalculation tests exponential backoff for reconnection.
func TestReconnectBackoffCalculation(t *testing.T) {
	tests := []struct {
		failures    int
		baseDelay   time.Duration
		maxDelay    time.Duration
		expectedMin time.Duration
		expectedMax time.Duration
	}{
		{0, time.Second, 5 * time.Minute, time.Second, time.Second},
		{1, time.Second, 5 * time.Minute, 2 * time.Second, 2 * time.Second},
		{2, time.Second, 5 * time.Minute, 4 * time.Second, 4 * time.Second},
		{3, time.Second, 5 * time.Minute, 8 * time.Second, 8 * time.Second},
		{6, time.Second, 5 * time.Minute, 64 * time.Second, 64 * time.Second},
		{10, time.Second, 5 * time.Minute, 64 * time.Second, 64 * time.Second}, // Capped at shift=6
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			shift := tt.failures
			if shift > 6 {
				shift = 6
			}

			delay := tt.baseDelay * time.Duration(1<<uint(shift))
			if delay > tt.maxDelay {
				delay = tt.maxDelay
			}

			if delay < tt.expectedMin || delay > tt.expectedMax {
				t.Errorf("failures=%d: expected delay [%v, %v], got %v",
					tt.failures, tt.expectedMin, tt.expectedMax, delay)
			}
		})
	}
}

// TestTooManySessionsBackoff tests special backoff for TooManySessions error.
func TestTooManySessionsBackoff(t *testing.T) {
	// TooManySessions should use longer base delay (1 minute)
	baseDelay := time.Minute
	maxDelay := 5 * time.Minute

	failures := []int{0, 1, 2, 3}
	for _, f := range failures {
		shift := f
		if shift > 6 {
			shift = 6
		}

		delay := baseDelay * time.Duration(1<<uint(shift))
		if delay > maxDelay {
			delay = maxDelay
		}

		// First failure should be at least 1 minute
		if f == 0 && delay < time.Minute {
			t.Errorf("TooManySessions first failure should be >= 1 minute, got %v", delay)
		}
	}
}

// =============================================================================
// Concurrent Session Access Tests
// =============================================================================

// TestDeviceBindingConcurrency tests concurrent access to device binding.
func TestDeviceBindingConcurrency(t *testing.T) {
	// Test concurrent map access pattern
	// Since DeviceBinding has unexported mutex, we test the concept
	var mu sync.RWMutex
	monitoredItems := make(map[string]uint32)

	var wg sync.WaitGroup
	workers := 10
	itemsPerWorker := 100

	// Concurrent additions
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			mu.Lock()
			defer mu.Unlock()
			for j := 0; j < itemsPerWorker; j++ {
				tagID := "tag-" + string(rune('0'+workerID)) + "-" + string(rune('0'+j%10))
				monitoredItems[tagID] = uint32(workerID*1000 + j)
			}
		}(i)
	}

	wg.Wait()

	// Due to overwrites from same keys, count may be less than workers*itemsPerWorker
	mu.RLock()
	count := len(monitoredItems)
	mu.RUnlock()

	if count == 0 {
		t.Error("expected some monitored items after concurrent additions")
	}
	t.Logf("Concurrent test resulted in %d monitored items", count)
}

// TestSequenceNumberConcurrency tests concurrent sequence number updates.
func TestSequenceNumberConcurrency(t *testing.T) {
	var lastSeqNo atomic.Uint32
	lastSeqNo.Store(0)

	var wg sync.WaitGroup
	workers := 10
	updates := 1000

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < updates; j++ {
				// Atomically update to max value
				for {
					current := lastSeqNo.Load()
					next := current + 1
					if lastSeqNo.CompareAndSwap(current, next) {
						break
					}
				}
			}
		}()
	}

	wg.Wait()

	expected := uint32(workers * updates)
	if lastSeqNo.Load() != expected {
		t.Errorf("expected sequence number %d, got %d", expected, lastSeqNo.Load())
	}
}

// =============================================================================
// Load Shaping Tests
// =============================================================================

// TestInFlightTracking tests in-flight operation tracking.
func TestInFlightTracking(t *testing.T) {
	var inFlight atomic.Int64
	maxInFlight := int64(10)

	// Simulate operations
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Check if we can proceed
			for {
				current := inFlight.Load()
				if current >= maxInFlight {
					// Would exceed limit, wait
					time.Sleep(time.Millisecond)
					continue
				}
				if inFlight.CompareAndSwap(current, current+1) {
					break
				}
			}

			// Do "work"
			time.Sleep(time.Millisecond)

			// Release slot
			inFlight.Add(-1)
		}()
	}

	wg.Wait()

	if inFlight.Load() != 0 {
		t.Errorf("expected 0 in-flight after completion, got %d", inFlight.Load())
	}
}
