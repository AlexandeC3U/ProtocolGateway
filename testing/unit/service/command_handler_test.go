// Package service_test tests the command handler functionality.
package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/nexus-edge/protocol-gateway/internal/service"
)

// =============================================================================
// Command Config Tests
// =============================================================================

// TestCommandConfigStructure tests CommandConfig structure.
func TestCommandConfigStructure(t *testing.T) {
	cfg := service.CommandConfig{
		CommandTopicPrefix:    "$nexus/cmd",
		ResponseTopicPrefix:   "$nexus/cmd/response",
		WriteTimeout:          10 * time.Second,
		QoS:                   1,
		EnableAcknowledgement: true,
		MaxConcurrentWrites:   50,
		CommandQueueSize:      1000,
	}

	if cfg.CommandTopicPrefix != "$nexus/cmd" {
		t.Errorf("expected CommandTopicPrefix '$nexus/cmd', got %s", cfg.CommandTopicPrefix)
	}
	if cfg.ResponseTopicPrefix != "$nexus/cmd/response" {
		t.Errorf("expected ResponseTopicPrefix '$nexus/cmd/response', got %s", cfg.ResponseTopicPrefix)
	}
	if cfg.WriteTimeout != 10*time.Second {
		t.Errorf("expected WriteTimeout 10s, got %v", cfg.WriteTimeout)
	}
	if cfg.QoS != 1 {
		t.Errorf("expected QoS 1, got %d", cfg.QoS)
	}
	if !cfg.EnableAcknowledgement {
		t.Error("expected EnableAcknowledgement to be true")
	}
	if cfg.MaxConcurrentWrites != 50 {
		t.Errorf("expected MaxConcurrentWrites 50, got %d", cfg.MaxConcurrentWrites)
	}
	if cfg.CommandQueueSize != 1000 {
		t.Errorf("expected CommandQueueSize 1000, got %d", cfg.CommandQueueSize)
	}
}

// TestDefaultCommandConfig tests default configuration values.
func TestDefaultCommandConfig(t *testing.T) {
	cfg := service.DefaultCommandConfig()

	if cfg.CommandTopicPrefix != "$nexus/cmd" {
		t.Errorf("expected default CommandTopicPrefix '$nexus/cmd', got %s", cfg.CommandTopicPrefix)
	}
	if cfg.ResponseTopicPrefix != "$nexus/cmd/response" {
		t.Errorf("expected default ResponseTopicPrefix '$nexus/cmd/response', got %s", cfg.ResponseTopicPrefix)
	}
	if cfg.WriteTimeout != 10*time.Second {
		t.Errorf("expected default WriteTimeout 10s, got %v", cfg.WriteTimeout)
	}
	if cfg.QoS != 1 {
		t.Errorf("expected default QoS 1, got %d", cfg.QoS)
	}
	if !cfg.EnableAcknowledgement {
		t.Error("expected default EnableAcknowledgement to be true")
	}
	if cfg.MaxConcurrentWrites != 50 {
		t.Errorf("expected default MaxConcurrentWrites 50, got %d", cfg.MaxConcurrentWrites)
	}
	if cfg.CommandQueueSize != 1000 {
		t.Errorf("expected default CommandQueueSize 1000, got %d", cfg.CommandQueueSize)
	}
}

// =============================================================================
// Write Command Tests
// =============================================================================

// TestWriteCommandStructure tests WriteCommand structure.
func TestWriteCommandStructure(t *testing.T) {
	now := time.Now()
	cmd := service.WriteCommand{
		RequestID: "req-123",
		DeviceID:  "device-001",
		TagID:     "temperature",
		Value:     25.5,
		Timestamp: now,
		Priority:  1,
	}

	if cmd.RequestID != "req-123" {
		t.Errorf("expected RequestID 'req-123', got %s", cmd.RequestID)
	}
	if cmd.DeviceID != "device-001" {
		t.Errorf("expected DeviceID 'device-001', got %s", cmd.DeviceID)
	}
	if cmd.TagID != "temperature" {
		t.Errorf("expected TagID 'temperature', got %s", cmd.TagID)
	}
	if cmd.Value != 25.5 {
		t.Errorf("expected Value 25.5, got %v", cmd.Value)
	}
	if !cmd.Timestamp.Equal(now) {
		t.Errorf("expected Timestamp %v, got %v", now, cmd.Timestamp)
	}
	if cmd.Priority != 1 {
		t.Errorf("expected Priority 1, got %d", cmd.Priority)
	}
}

// TestWriteCommandJSON tests WriteCommand JSON marshaling.
func TestWriteCommandJSON(t *testing.T) {
	cmd := service.WriteCommand{
		RequestID: "req-456",
		DeviceID:  "plc-001",
		TagID:     "setpoint",
		Value:     100,
	}

	// Marshal to JSON
	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("failed to marshal WriteCommand: %v", err)
	}

	// Unmarshal back
	var decoded service.WriteCommand
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal WriteCommand: %v", err)
	}

	if decoded.RequestID != cmd.RequestID {
		t.Errorf("expected RequestID %s, got %s", cmd.RequestID, decoded.RequestID)
	}
	if decoded.DeviceID != cmd.DeviceID {
		t.Errorf("expected DeviceID %s, got %s", cmd.DeviceID, decoded.DeviceID)
	}
	if decoded.TagID != cmd.TagID {
		t.Errorf("expected TagID %s, got %s", cmd.TagID, decoded.TagID)
	}
}

// TestWriteCommandJSONValueTypes tests WriteCommand with different value types.
func TestWriteCommandJSONValueTypes(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
	}{
		{"integer", 100},
		{"float", 25.5},
		{"boolean", true},
		{"string", "hello"},
		{"negative", -50},
		{"zero", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := service.WriteCommand{
				DeviceID: "device-001",
				TagID:    "tag-001",
				Value:    tt.value,
			}

			data, err := json.Marshal(cmd)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			var decoded service.WriteCommand
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			// Note: JSON numbers are decoded as float64
			if decoded.DeviceID != cmd.DeviceID {
				t.Errorf("DeviceID mismatch")
			}
		})
	}
}

// =============================================================================
// Write Response Tests
// =============================================================================

// TestWriteResponseStructure tests WriteResponse structure.
func TestWriteResponseStructure(t *testing.T) {
	now := time.Now()
	resp := service.WriteResponse{
		RequestID: "req-123",
		DeviceID:  "device-001",
		TagID:     "temperature",
		Success:   true,
		Error:     "",
		Timestamp: now,
		Duration:  150 * time.Millisecond,
	}

	if resp.RequestID != "req-123" {
		t.Errorf("expected RequestID 'req-123', got %s", resp.RequestID)
	}
	if resp.DeviceID != "device-001" {
		t.Errorf("expected DeviceID 'device-001', got %s", resp.DeviceID)
	}
	if resp.TagID != "temperature" {
		t.Errorf("expected TagID 'temperature', got %s", resp.TagID)
	}
	if !resp.Success {
		t.Error("expected Success to be true")
	}
	if resp.Error != "" {
		t.Errorf("expected empty Error, got %s", resp.Error)
	}
	if !resp.Timestamp.Equal(now) {
		t.Errorf("expected Timestamp %v, got %v", now, resp.Timestamp)
	}
	if resp.Duration != 150*time.Millisecond {
		t.Errorf("expected Duration 150ms, got %v", resp.Duration)
	}
}

// TestWriteResponseFailure tests WriteResponse for failed writes.
func TestWriteResponseFailure(t *testing.T) {
	now := time.Now()
	resp := service.WriteResponse{
		RequestID: "req-789",
		DeviceID:  "device-001",
		TagID:     "setpoint",
		Success:   false,
		Error:     "device not connected",
		Timestamp: now,
		Duration:  50 * time.Millisecond,
	}

	if resp.RequestID != "req-789" {
		t.Errorf("expected RequestID 'req-789', got %s", resp.RequestID)
	}
	if resp.DeviceID != "device-001" {
		t.Errorf("expected DeviceID 'device-001', got %s", resp.DeviceID)
	}
	if resp.TagID != "setpoint" {
		t.Errorf("expected TagID 'setpoint', got %s", resp.TagID)
	}
	if resp.Success {
		t.Error("expected Success to be false")
	}
	if resp.Error != "device not connected" {
		t.Errorf("expected Error 'device not connected', got %s", resp.Error)
	}
	if !resp.Timestamp.Equal(now) {
		t.Errorf("expected Timestamp %v, got %v", now, resp.Timestamp)
	}
	if resp.Duration != 50*time.Millisecond {
		t.Errorf("expected Duration 50ms, got %v", resp.Duration)
	}
}

// TestWriteResponseJSON tests WriteResponse JSON marshaling.
func TestWriteResponseJSON(t *testing.T) {
	resp := service.WriteResponse{
		RequestID: "req-001",
		DeviceID:  "plc-001",
		TagID:     "output",
		Success:   true,
		Timestamp: time.Now(),
		Duration:  100 * time.Millisecond,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal WriteResponse: %v", err)
	}

	var decoded service.WriteResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal WriteResponse: %v", err)
	}

	if decoded.RequestID != resp.RequestID {
		t.Errorf("expected RequestID %s, got %s", resp.RequestID, decoded.RequestID)
	}
	if decoded.Success != resp.Success {
		t.Errorf("expected Success %v, got %v", resp.Success, decoded.Success)
	}
}

// =============================================================================
// Command Stats Tests
// =============================================================================

// TestCommandStatsAtomicOperations tests CommandStats atomic operations.
func TestCommandStatsAtomicOperations(t *testing.T) {
	stats := &service.CommandStats{}

	// Test CommandsReceived
	stats.CommandsReceived.Add(100)
	if got := stats.CommandsReceived.Load(); got != 100 {
		t.Errorf("expected CommandsReceived 100, got %d", got)
	}

	// Test CommandsSucceeded
	stats.CommandsSucceeded.Add(90)
	if got := stats.CommandsSucceeded.Load(); got != 90 {
		t.Errorf("expected CommandsSucceeded 90, got %d", got)
	}

	// Test CommandsFailed
	stats.CommandsFailed.Add(8)
	if got := stats.CommandsFailed.Load(); got != 8 {
		t.Errorf("expected CommandsFailed 8, got %d", got)
	}

	// Test CommandsRejected
	stats.CommandsRejected.Add(2)
	if got := stats.CommandsRejected.Load(); got != 2 {
		t.Errorf("expected CommandsRejected 2, got %d", got)
	}
}

// TestCommandStatsConcurrentAccess tests concurrent access to stats.
func TestCommandStatsConcurrentAccess(t *testing.T) {
	stats := &service.CommandStats{}
	var wg sync.WaitGroup

	// Simulate concurrent updates
	for i := 0; i < 100; i++ {
		wg.Add(4)
		go func() {
			defer wg.Done()
			stats.CommandsReceived.Add(1)
		}()
		go func() {
			defer wg.Done()
			stats.CommandsSucceeded.Add(1)
		}()
		go func() {
			defer wg.Done()
			stats.CommandsFailed.Add(1)
		}()
		go func() {
			defer wg.Done()
			stats.CommandsRejected.Add(1)
		}()
	}

	wg.Wait()

	if got := stats.CommandsReceived.Load(); got != 100 {
		t.Errorf("expected CommandsReceived 100, got %d", got)
	}
	if got := stats.CommandsSucceeded.Load(); got != 100 {
		t.Errorf("expected CommandsSucceeded 100, got %d", got)
	}
	if got := stats.CommandsFailed.Load(); got != 100 {
		t.Errorf("expected CommandsFailed 100, got %d", got)
	}
	if got := stats.CommandsRejected.Load(); got != 100 {
		t.Errorf("expected CommandsRejected 100, got %d", got)
	}
}

// =============================================================================
// Topic Pattern Tests
// =============================================================================

// TestCommandTopicPatterns tests expected MQTT topic patterns.
func TestCommandTopicPatterns(t *testing.T) {
	cfg := service.DefaultCommandConfig()

	tests := []struct {
		name     string
		prefix   string
		deviceID string
		tagID    string
		expected string
	}{
		{
			name:     "write command topic",
			prefix:   cfg.CommandTopicPrefix,
			deviceID: "plc-001",
			expected: "$nexus/cmd/plc-001/write",
		},
		{
			name:     "tag write topic",
			prefix:   cfg.CommandTopicPrefix,
			deviceID: "plc-001",
			tagID:    "setpoint",
			expected: "$nexus/cmd/plc-001/setpoint/set",
		},
		{
			name:     "response topic",
			prefix:   cfg.ResponseTopicPrefix,
			deviceID: "plc-001",
			tagID:    "setpoint",
			expected: "$nexus/cmd/response/plc-001/setpoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var topic string
			if tt.tagID != "" {
				if tt.prefix == cfg.ResponseTopicPrefix {
					topic = tt.prefix + "/" + tt.deviceID + "/" + tt.tagID
				} else {
					topic = tt.prefix + "/" + tt.deviceID + "/" + tt.tagID + "/set"
				}
			} else {
				topic = tt.prefix + "/" + tt.deviceID + "/write"
			}
			if topic != tt.expected {
				t.Errorf("expected topic %s, got %s", tt.expected, topic)
			}
		})
	}
}

// TestTopicParsing tests parsing device/tag IDs from topics.
func TestTopicParsing(t *testing.T) {
	tests := []struct {
		name           string
		topic          string
		expectedDevice string
		expectedTag    string
	}{
		{
			name:           "write command",
			topic:          "$nexus/cmd/plc-001/write",
			expectedDevice: "plc-001",
			expectedTag:    "",
		},
		{
			name:           "tag set command",
			topic:          "$nexus/cmd/plc-001/temperature/set",
			expectedDevice: "plc-001",
			expectedTag:    "temperature",
		},
		{
			name:           "nested device ID",
			topic:          "$nexus/cmd/plant1-line2-plc3/write",
			expectedDevice: "plant1-line2-plc3",
			expectedTag:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := splitTopic(tt.topic)
			if len(parts) < 3 {
				t.Fatalf("invalid topic format: %s", tt.topic)
			}

			// For write command: prefix/device_id/write
			// For tag set: prefix/device_id/tag_id/set
			if parts[len(parts)-1] == "write" {
				deviceID := parts[len(parts)-2]
				if deviceID != tt.expectedDevice {
					t.Errorf("expected device ID %s, got %s", tt.expectedDevice, deviceID)
				}
			} else if parts[len(parts)-1] == "set" {
				deviceID := parts[len(parts)-3]
				tagID := parts[len(parts)-2]
				if deviceID != tt.expectedDevice {
					t.Errorf("expected device ID %s, got %s", tt.expectedDevice, deviceID)
				}
				if tagID != tt.expectedTag {
					t.Errorf("expected tag ID %s, got %s", tt.expectedTag, tagID)
				}
			}
		})
	}
}

// =============================================================================
// Tag Writability Tests
// =============================================================================

// TestTagWritability tests tag access mode for write operations.
func TestTagWritability(t *testing.T) {
	tests := []struct {
		name       string
		accessMode domain.AccessMode
		writable   bool
	}{
		{"read only", domain.AccessModeReadOnly, false},
		{"write only", domain.AccessModeWriteOnly, true},
		{"read write", domain.AccessModeReadWrite, true},
		// When access mode is empty and register type is also empty,
		// the tag defaults to not writable (for safety)
		{"empty (default)", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag := &domain.Tag{
				ID:         "test-tag",
				Name:       "Test Tag",
				AccessMode: tt.accessMode,
			}

			isWritable := tag.IsWritable()
			if isWritable != tt.writable {
				t.Errorf("expected IsWritable=%v for AccessMode=%s, got %v",
					tt.writable, tt.accessMode, isWritable)
			}
		})
	}
}

// =============================================================================
// Rate Limiting Simulation Tests
// =============================================================================

// TestRateLimitingBehavior tests rate limiting semaphore behavior.
func TestRateLimitingBehavior(t *testing.T) {
	maxConcurrent := 5
	semaphore := make(chan struct{}, maxConcurrent)

	// Fill up semaphore
	for i := 0; i < maxConcurrent; i++ {
		semaphore <- struct{}{}
	}

	// Try to acquire one more (should not block in select with default)
	select {
	case semaphore <- struct{}{}:
		t.Error("expected semaphore to be full, but acquired slot")
	default:
		// Expected: semaphore is full
	}

	// Release one slot
	<-semaphore

	// Now should be able to acquire
	select {
	case semaphore <- struct{}{}:
		// Expected: acquired slot
	default:
		t.Error("expected to acquire slot after release")
	}
}

// TestBoundedQueueBehavior tests bounded queue back-pressure.
func TestBoundedQueueBehavior(t *testing.T) {
	queueSize := 3
	queue := make(chan service.WriteCommand, queueSize)

	// Fill up queue
	for i := 0; i < queueSize; i++ {
		cmd := service.WriteCommand{
			DeviceID: "device",
			TagID:    "tag",
			Value:    i,
		}
		select {
		case queue <- cmd:
			// Queued successfully
		default:
			t.Errorf("expected to queue command %d", i)
		}
	}

	// Try to add one more (should apply back-pressure)
	cmd := service.WriteCommand{
		DeviceID: "device",
		TagID:    "tag",
		Value:    "extra",
	}
	select {
	case queue <- cmd:
		t.Error("expected queue to be full (back-pressure)")
	default:
		// Expected: queue full
	}

	// Drain one item
	<-queue

	// Now should be able to queue
	select {
	case queue <- cmd:
		// Expected: queued successfully
	default:
		t.Error("expected to queue after draining one")
	}
}

// =============================================================================
// Concurrent Write Simulation Tests
// =============================================================================

// TestConcurrentWriteCommands simulates concurrent command processing.
func TestConcurrentWriteCommands(t *testing.T) {
	var (
		processed  atomic.Int64
		concurrent atomic.Int64
		maxSeen    atomic.Int64
	)

	maxConcurrent := 5
	totalCommands := 50
	semaphore := make(chan struct{}, maxConcurrent)

	var wg sync.WaitGroup
	for i := 0; i < totalCommands; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Track concurrent count
			current := concurrent.Add(1)
			for {
				old := maxSeen.Load()
				if current <= old || maxSeen.CompareAndSwap(old, current) {
					break
				}
			}

			// Simulate work
			time.Sleep(time.Millisecond)

			concurrent.Add(-1)
			processed.Add(1)
		}(i)
	}

	wg.Wait()

	if got := processed.Load(); got != int64(totalCommands) {
		t.Errorf("expected %d processed, got %d", totalCommands, got)
	}
	if got := maxSeen.Load(); got > int64(maxConcurrent) {
		t.Errorf("expected max concurrent <= %d, got %d", maxConcurrent, got)
	}
}

// =============================================================================
// Device and Tag Lookup Tests
// =============================================================================

// TestDeviceLookup tests device lookup by ID.
func TestDeviceLookup(t *testing.T) {
	devices := map[string]*domain.Device{
		"plc-001": {ID: "plc-001", Name: "PLC 1", Protocol: domain.ProtocolModbusTCP},
		"plc-002": {ID: "plc-002", Name: "PLC 2", Protocol: domain.ProtocolOPCUA},
	}

	tests := []struct {
		name     string
		deviceID string
		exists   bool
	}{
		{"existing device", "plc-001", true},
		{"another device", "plc-002", true},
		{"non-existing", "plc-999", false},
		{"empty ID", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, exists := devices[tt.deviceID]
			if exists != tt.exists {
				t.Errorf("expected exists=%v for device %s, got %v", tt.exists, tt.deviceID, exists)
			}
		})
	}
}

// TestTagLookup tests tag lookup by device and tag ID.
func TestTagLookup(t *testing.T) {
	tagByID := map[string]map[string]*domain.Tag{
		"plc-001": {
			"temperature": {ID: "temperature", Name: "Temperature", DataType: domain.DataTypeFloat32},
			"pressure":    {ID: "pressure", Name: "Pressure", DataType: domain.DataTypeFloat32},
		},
		"plc-002": {
			"speed": {ID: "speed", Name: "Motor Speed", DataType: domain.DataTypeUInt16},
		},
	}

	tests := []struct {
		name     string
		deviceID string
		tagID    string
		exists   bool
	}{
		{"existing tag", "plc-001", "temperature", true},
		{"another tag", "plc-001", "pressure", true},
		{"tag on different device", "plc-002", "speed", true},
		{"non-existing tag", "plc-001", "humidity", false},
		{"non-existing device", "plc-999", "temperature", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tag *domain.Tag
			if deviceTags, ok := tagByID[tt.deviceID]; ok {
				tag = deviceTags[tt.tagID]
			}
			exists := tag != nil
			if exists != tt.exists {
				t.Errorf("expected exists=%v for %s/%s, got %v", tt.exists, tt.deviceID, tt.tagID, exists)
			}
		})
	}
}

// =============================================================================
// Error Scenarios Tests
// =============================================================================

// TestCommandErrorScenarios tests various error conditions.
func TestCommandErrorScenarios(t *testing.T) {
	tests := []struct {
		name          string
		deviceExists  bool
		tagExists     bool
		tagWritable   bool
		writeError    error
		expectedError string
	}{
		{
			name:          "device not found",
			deviceExists:  false,
			expectedError: "device not found",
		},
		{
			name:          "tag not found",
			deviceExists:  true,
			tagExists:     false,
			expectedError: "tag not found",
		},
		{
			name:          "tag not writable",
			deviceExists:  true,
			tagExists:     true,
			tagWritable:   false,
			expectedError: "tag is not writable",
		},
		{
			name:          "write operation failed",
			deviceExists:  true,
			tagExists:     true,
			tagWritable:   true,
			writeError:    errors.New("connection timeout"),
			expectedError: "connection timeout",
		},
		{
			name:         "successful write",
			deviceExists: true,
			tagExists:    true,
			tagWritable:  true,
			writeError:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the error checking logic
			var errMsg string

			if !tt.deviceExists {
				errMsg = "device not found"
			} else if !tt.tagExists {
				errMsg = "tag not found"
			} else if !tt.tagWritable {
				errMsg = "tag is not writable"
			} else if tt.writeError != nil {
				errMsg = tt.writeError.Error()
			}

			if errMsg != tt.expectedError {
				t.Errorf("expected error '%s', got '%s'", tt.expectedError, errMsg)
			}
		})
	}
}

// =============================================================================
// Timeout Tests
// =============================================================================

// TestWriteTimeout tests timeout behavior for write operations.
func TestWriteTimeout(t *testing.T) {
	timeout := 100 * time.Millisecond

	tests := []struct {
		name         string
		workDuration time.Duration
		shouldCancel bool
	}{
		{"fast operation", 10 * time.Millisecond, false},
		{"slow operation", 200 * time.Millisecond, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			done := make(chan bool, 1)
			go func() {
				select {
				case <-time.After(tt.workDuration):
					done <- false
				case <-ctx.Done():
					done <- true
				}
			}()

			cancelled := <-done
			if cancelled != tt.shouldCancel {
				t.Errorf("expected cancelled=%v, got %v", tt.shouldCancel, cancelled)
			}
		})
	}
}

// =============================================================================
// Shutdown Behavior Tests
// =============================================================================

// TestGracefulShutdown tests graceful shutdown with command draining.
func TestGracefulShutdown(t *testing.T) {
	queueSize := 5
	queue := make(chan service.WriteCommand, queueSize)

	// Add some commands to queue
	for i := 0; i < 3; i++ {
		queue <- service.WriteCommand{
			DeviceID: "device",
			TagID:    "tag",
			Value:    i,
		}
	}

	// Simulate draining with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	drained := 0
	for {
		select {
		case <-queue:
			drained++
		case <-ctx.Done():
			t.Log("drain timeout reached")
			goto done
		default:
			goto done
		}
	}

done:
	if drained != 3 {
		t.Errorf("expected to drain 3 commands, got %d", drained)
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

func splitTopic(topic string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(topic); i++ {
		if topic[i] == '/' {
			if i > start {
				parts = append(parts, topic[start:i])
			}
			start = i + 1
		}
	}
	if start < len(topic) {
		parts = append(parts, topic[start:])
	}
	return parts
}
