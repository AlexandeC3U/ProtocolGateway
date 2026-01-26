package domain

import (
	"context"
	"errors"
	"testing"
)

func TestProtocolManager_RegisterAndGetPool(t *testing.T) {
	pm := NewProtocolManager()
	mockPool := NewMockProtocolPool()

	// Register a pool
	pm.RegisterPool(ProtocolModbusTCP, mockPool)

	// Get the pool
	pool, exists := pm.GetPool(ProtocolModbusTCP)
	if !exists {
		t.Error("GetPool() returned exists = false, want true")
	}
	if pool != mockPool {
		t.Error("GetPool() returned wrong pool")
	}

	// Try to get non-existent pool
	_, exists = pm.GetPool(ProtocolOPCUA)
	if exists {
		t.Error("GetPool() returned exists = true for non-registered protocol")
	}
}

func TestProtocolManager_ReadTags(t *testing.T) {
	pm := NewProtocolManager()
	mockPool := NewMockProtocolPool()

	device := &Device{
		ID:       "test-device",
		Protocol: ProtocolModbusTCP,
	}

	tags := []*Tag{
		{ID: "tag1", Name: "Tag 1"},
		{ID: "tag2", Name: "Tag 2"},
	}

	// Test error when protocol not registered
	_, err := pm.ReadTags(context.Background(), device, tags)
	if err != ErrProtocolNotSupported {
		t.Errorf("ReadTags() error = %v, want %v", err, ErrProtocolNotSupported)
	}

	// Register pool and test successful read
	pm.RegisterPool(ProtocolModbusTCP, mockPool)

	expectedPoints := []*DataPoint{
		NewDataPoint("test-device", "tag1", "", 100.0, "", QualityGood),
		NewDataPoint("test-device", "tag2", "", 200.0, "", QualityGood),
	}

	mockPool.ReadTagsFunc = func(ctx context.Context, device *Device, tags []*Tag) ([]*DataPoint, error) {
		return expectedPoints, nil
	}

	points, err := pm.ReadTags(context.Background(), device, tags)
	if err != nil {
		t.Errorf("ReadTags() unexpected error: %v", err)
	}
	if len(points) != len(expectedPoints) {
		t.Errorf("ReadTags() returned %d points, want %d", len(points), len(expectedPoints))
	}

	// Verify call was recorded
	if len(mockPool.ReadTagsCalls) != 1 {
		t.Errorf("ReadTags() called mock %d times, want 1", len(mockPool.ReadTagsCalls))
	}
}

func TestProtocolManager_WriteTag(t *testing.T) {
	pm := NewProtocolManager()
	mockPool := NewMockProtocolPool()

	device := &Device{
		ID:       "test-device",
		Protocol: ProtocolS7,
	}

	tag := &Tag{ID: "tag1", Name: "Tag 1"}
	value := 42.0

	// Test error when protocol not registered
	err := pm.WriteTag(context.Background(), device, tag, value)
	if err != ErrProtocolNotSupported {
		t.Errorf("WriteTag() error = %v, want %v", err, ErrProtocolNotSupported)
	}

	// Register pool and test successful write
	pm.RegisterPool(ProtocolS7, mockPool)

	err = pm.WriteTag(context.Background(), device, tag, value)
	if err != nil {
		t.Errorf("WriteTag() unexpected error: %v", err)
	}

	// Verify call was recorded with correct value
	if len(mockPool.WriteTagCalls) != 1 {
		t.Errorf("WriteTag() called mock %d times, want 1", len(mockPool.WriteTagCalls))
	}
	if mockPool.WriteTagCalls[0].Value != value {
		t.Errorf("WriteTag() called with value %v, want %v", mockPool.WriteTagCalls[0].Value, value)
	}
}

func TestProtocolManager_Close(t *testing.T) {
	pm := NewProtocolManager()
	mockPool1 := NewMockProtocolPool()
	mockPool2 := NewMockProtocolPool()

	pm.RegisterPool(ProtocolModbusTCP, mockPool1)
	pm.RegisterPool(ProtocolOPCUA, mockPool2)

	// Close all pools
	err := pm.Close()
	if err != nil {
		t.Errorf("Close() unexpected error: %v", err)
	}

	// Verify both pools were closed
	if mockPool1.CloseCalls != 1 {
		t.Errorf("Close() called mockPool1 %d times, want 1", mockPool1.CloseCalls)
	}
	if mockPool2.CloseCalls != 1 {
		t.Errorf("Close() called mockPool2 %d times, want 1", mockPool2.CloseCalls)
	}
}

func TestProtocolManager_Close_WithError(t *testing.T) {
	pm := NewProtocolManager()
	mockPool := NewMockProtocolPool()

	expectedError := errors.New("close error")
	mockPool.CloseFunc = func() error {
		return expectedError
	}

	pm.RegisterPool(ProtocolModbusTCP, mockPool)

	err := pm.Close()
	if err != expectedError {
		t.Errorf("Close() error = %v, want %v", err, expectedError)
	}
}

func TestProtocolManager_HealthCheck(t *testing.T) {
	pm := NewProtocolManager()
	mockPool1 := NewMockProtocolPool()
	mockPool2 := NewMockProtocolPool()

	pm.RegisterPool(ProtocolModbusTCP, mockPool1)
	pm.RegisterPool(ProtocolOPCUA, mockPool2)

	// Test healthy pools
	err := pm.HealthCheck(context.Background())
	if err != nil {
		t.Errorf("HealthCheck() unexpected error: %v", err)
	}

	// Test with unhealthy pool
	expectedError := errors.New("unhealthy")
	mockPool1.HealthFunc = func(ctx context.Context) error {
		return expectedError
	}

	err = pm.HealthCheck(context.Background())
	if err != expectedError {
		t.Errorf("HealthCheck() error = %v, want %v", err, expectedError)
	}
}

func TestProtocolManager_RegisteredProtocols(t *testing.T) {
	pm := NewProtocolManager()

	// Initially empty
	protocols := pm.RegisteredProtocols()
	if len(protocols) != 0 {
		t.Errorf("RegisteredProtocols() returned %d protocols, want 0", len(protocols))
	}

	// Register some protocols
	pm.RegisterPool(ProtocolModbusTCP, NewMockProtocolPool())
	pm.RegisterPool(ProtocolOPCUA, NewMockProtocolPool())
	pm.RegisterPool(ProtocolS7, NewMockProtocolPool())

	protocols = pm.RegisteredProtocols()
	if len(protocols) != 3 {
		t.Errorf("RegisteredProtocols() returned %d protocols, want 3", len(protocols))
	}
}

func TestProtocolManager_ConcurrentAccess(t *testing.T) {
	pm := NewProtocolManager()
	mockPool := NewMockProtocolPool()
	pm.RegisterPool(ProtocolModbusTCP, mockPool)

	device := &Device{ID: "test", Protocol: ProtocolModbusTCP}
	tag := &Tag{ID: "tag1"}

	// Run concurrent reads
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			_, _ = pm.ReadTag(context.Background(), device, tag)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Verify all calls were recorded
	if len(mockPool.ReadTagCalls) != 100 {
		t.Errorf("Concurrent ReadTag() calls = %d, want 100", len(mockPool.ReadTagCalls))
	}
}
