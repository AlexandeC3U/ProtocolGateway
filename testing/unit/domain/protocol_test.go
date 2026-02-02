// Package domain_test tests the protocol management functionality.
package domain_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

// mockProtocolPool is a mock implementation of ProtocolPool for testing.
type mockProtocolPool struct {
	readTagsCalled int
	readTagCalled  int
	writeTagCalled int
	closeCalled    int
	healthCheckErr error
	readTagsErr    error
	readTagErr     error
	writeTagErr    error
	closeErr       error
	mu             sync.Mutex
}

func (m *mockProtocolPool) ReadTags(ctx context.Context, device *domain.Device, tags []*domain.Tag) ([]*domain.DataPoint, error) {
	m.mu.Lock()
	m.readTagsCalled++
	m.mu.Unlock()
	if m.readTagsErr != nil {
		return nil, m.readTagsErr
	}
	return make([]*domain.DataPoint, len(tags)), nil
}

func (m *mockProtocolPool) ReadTag(ctx context.Context, device *domain.Device, tag *domain.Tag) (*domain.DataPoint, error) {
	m.mu.Lock()
	m.readTagCalled++
	m.mu.Unlock()
	if m.readTagErr != nil {
		return nil, m.readTagErr
	}
	return &domain.DataPoint{}, nil
}

func (m *mockProtocolPool) WriteTag(ctx context.Context, device *domain.Device, tag *domain.Tag, value interface{}) error {
	m.mu.Lock()
	m.writeTagCalled++
	m.mu.Unlock()
	return m.writeTagErr
}

func (m *mockProtocolPool) Close() error {
	m.mu.Lock()
	m.closeCalled++
	m.mu.Unlock()
	return m.closeErr
}

func (m *mockProtocolPool) HealthCheck(ctx context.Context) error {
	return m.healthCheckErr
}

// TestNewProtocolManager tests creating a new protocol manager.
func TestNewProtocolManager(t *testing.T) {
	pm := domain.NewProtocolManager()
	if pm == nil {
		t.Fatal("expected non-nil protocol manager")
	}
}

// TestProtocolManager_RegisterPool tests registering protocol pools.
func TestProtocolManager_RegisterPool(t *testing.T) {
	pm := domain.NewProtocolManager()
	pool := &mockProtocolPool{}

	pm.RegisterPool(domain.ProtocolModbusTCP, pool)

	// Verify pool was registered
	got, exists := pm.GetPool(domain.ProtocolModbusTCP)
	if !exists {
		t.Fatal("expected pool to exist after registration")
	}
	if got != pool {
		t.Error("expected registered pool to match")
	}
}

// TestProtocolManager_GetPool_NotFound tests getting a non-existent pool.
func TestProtocolManager_GetPool_NotFound(t *testing.T) {
	pm := domain.NewProtocolManager()

	_, exists := pm.GetPool(domain.ProtocolModbusTCP)
	if exists {
		t.Error("expected pool to not exist")
	}
}

// TestProtocolManager_RegisterMultiplePools tests registering multiple protocols.
func TestProtocolManager_RegisterMultiplePools(t *testing.T) {
	pm := domain.NewProtocolManager()

	modbusPool := &mockProtocolPool{}
	opcuaPool := &mockProtocolPool{}
	s7Pool := &mockProtocolPool{}

	pm.RegisterPool(domain.ProtocolModbusTCP, modbusPool)
	pm.RegisterPool(domain.ProtocolOPCUA, opcuaPool)
	pm.RegisterPool(domain.ProtocolS7, s7Pool)

	tests := []struct {
		protocol domain.Protocol
		expected domain.ProtocolPool
	}{
		{domain.ProtocolModbusTCP, modbusPool},
		{domain.ProtocolOPCUA, opcuaPool},
		{domain.ProtocolS7, s7Pool},
	}

	for _, tt := range tests {
		t.Run(string(tt.protocol), func(t *testing.T) {
			pool, exists := pm.GetPool(tt.protocol)
			if !exists {
				t.Errorf("expected pool for %s to exist", tt.protocol)
			}
			if pool != tt.expected {
				t.Errorf("pool mismatch for %s", tt.protocol)
			}
		})
	}
}

// TestProtocolManager_ReadTags_Success tests successful tag reading.
func TestProtocolManager_ReadTags_Success(t *testing.T) {
	pm := domain.NewProtocolManager()
	pool := &mockProtocolPool{}
	pm.RegisterPool(domain.ProtocolModbusTCP, pool)

	device := &domain.Device{
		ID:       "test-device",
		Protocol: domain.ProtocolModbusTCP,
	}
	tags := []*domain.Tag{
		{ID: "tag1"},
		{ID: "tag2"},
	}

	ctx := context.Background()
	results, err := pm.ReadTags(ctx, device, tags)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	if pool.readTagsCalled != 1 {
		t.Errorf("expected ReadTags to be called once, got %d", pool.readTagsCalled)
	}
}

// TestProtocolManager_ReadTags_ProtocolNotSupported tests unsupported protocol.
func TestProtocolManager_ReadTags_ProtocolNotSupported(t *testing.T) {
	pm := domain.NewProtocolManager()

	device := &domain.Device{
		ID:       "test-device",
		Protocol: domain.ProtocolModbusTCP,
	}

	ctx := context.Background()
	_, err := pm.ReadTags(ctx, device, nil)

	if !errors.Is(err, domain.ErrProtocolNotSupported) {
		t.Errorf("expected ErrProtocolNotSupported, got %v", err)
	}
}

// TestProtocolManager_ReadTag_Success tests single tag reading.
func TestProtocolManager_ReadTag_Success(t *testing.T) {
	pm := domain.NewProtocolManager()
	pool := &mockProtocolPool{}
	pm.RegisterPool(domain.ProtocolOPCUA, pool)

	device := &domain.Device{
		ID:       "opcua-device",
		Protocol: domain.ProtocolOPCUA,
	}
	tag := &domain.Tag{ID: "tag1"}

	ctx := context.Background()
	_, err := pm.ReadTag(ctx, device, tag)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if pool.readTagCalled != 1 {
		t.Errorf("expected ReadTag to be called once, got %d", pool.readTagCalled)
	}
}

// TestProtocolManager_WriteTag_Success tests tag writing.
func TestProtocolManager_WriteTag_Success(t *testing.T) {
	pm := domain.NewProtocolManager()
	pool := &mockProtocolPool{}
	pm.RegisterPool(domain.ProtocolS7, pool)

	device := &domain.Device{
		ID:       "s7-device",
		Protocol: domain.ProtocolS7,
	}
	tag := &domain.Tag{ID: "tag1"}

	ctx := context.Background()
	err := pm.WriteTag(ctx, device, tag, 42)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if pool.writeTagCalled != 1 {
		t.Errorf("expected WriteTag to be called once, got %d", pool.writeTagCalled)
	}
}

// TestProtocolManager_WriteTag_ProtocolNotSupported tests writing to unsupported protocol.
func TestProtocolManager_WriteTag_ProtocolNotSupported(t *testing.T) {
	pm := domain.NewProtocolManager()

	device := &domain.Device{
		ID:       "test-device",
		Protocol: domain.ProtocolS7,
	}

	ctx := context.Background()
	err := pm.WriteTag(ctx, device, nil, 42)

	if !errors.Is(err, domain.ErrProtocolNotSupported) {
		t.Errorf("expected ErrProtocolNotSupported, got %v", err)
	}
}

// TestProtocolManager_Close_Success tests closing all pools.
func TestProtocolManager_Close_Success(t *testing.T) {
	pm := domain.NewProtocolManager()
	pool1 := &mockProtocolPool{}
	pool2 := &mockProtocolPool{}

	pm.RegisterPool(domain.ProtocolModbusTCP, pool1)
	pm.RegisterPool(domain.ProtocolOPCUA, pool2)

	err := pm.Close()

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if pool1.closeCalled != 1 {
		t.Errorf("expected pool1.Close to be called once")
	}
	if pool2.closeCalled != 1 {
		t.Errorf("expected pool2.Close to be called once")
	}
}

// TestProtocolManager_Close_WithErrors tests closing with some pools returning errors.
func TestProtocolManager_Close_WithErrors(t *testing.T) {
	pm := domain.NewProtocolManager()
	pool1 := &mockProtocolPool{closeErr: errors.New("close error 1")}
	pool2 := &mockProtocolPool{}

	pm.RegisterPool(domain.ProtocolModbusTCP, pool1)
	pm.RegisterPool(domain.ProtocolOPCUA, pool2)

	err := pm.Close()

	if err == nil {
		t.Error("expected error from Close")
	}
}

// TestProtocolManager_Concurrent tests concurrent access to protocol manager.
func TestProtocolManager_Concurrent(t *testing.T) {
	pm := domain.NewProtocolManager()
	pool := &mockProtocolPool{}
	pm.RegisterPool(domain.ProtocolModbusTCP, pool)

	device := &domain.Device{
		ID:       "test-device",
		Protocol: domain.ProtocolModbusTCP,
	}
	tag := &domain.Tag{ID: "tag1"}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			_, _ = pm.ReadTag(ctx, device, tag)
		}()
	}
	wg.Wait()

	pool.mu.Lock()
	calls := pool.readTagCalled
	pool.mu.Unlock()

	if calls != 100 {
		t.Errorf("expected 100 calls, got %d", calls)
	}
}

// TestProtocolManager_ReplacePool tests replacing a registered pool.
func TestProtocolManager_ReplacePool(t *testing.T) {
	pm := domain.NewProtocolManager()
	pool1 := &mockProtocolPool{}
	pool2 := &mockProtocolPool{}

	pm.RegisterPool(domain.ProtocolModbusTCP, pool1)
	pm.RegisterPool(domain.ProtocolModbusTCP, pool2) // Replace

	got, _ := pm.GetPool(domain.ProtocolModbusTCP)
	if got != pool2 {
		t.Error("expected pool to be replaced")
	}
}

// TestProtocolConstants tests protocol constant values.
func TestProtocolConstants(t *testing.T) {
	tests := []struct {
		protocol domain.Protocol
		expected string
	}{
		{domain.ProtocolModbusTCP, "modbus-tcp"},
		{domain.ProtocolModbusRTU, "modbus-rtu"},
		{domain.ProtocolOPCUA, "opcua"},
		{domain.ProtocolS7, "s7"},
	}

	for _, tt := range tests {
		t.Run(string(tt.protocol), func(t *testing.T) {
			if string(tt.protocol) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(tt.protocol))
			}
		})
	}
}

// TestProtocolManager_ReadTags_WithError tests error propagation.
func TestProtocolManager_ReadTags_WithError(t *testing.T) {
	pm := domain.NewProtocolManager()
	expectedErr := errors.New("read error")
	pool := &mockProtocolPool{readTagsErr: expectedErr}
	pm.RegisterPool(domain.ProtocolModbusTCP, pool)

	device := &domain.Device{
		ID:       "test-device",
		Protocol: domain.ProtocolModbusTCP,
	}

	ctx := context.Background()
	_, err := pm.ReadTags(ctx, device, nil)

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

// TestProtocolManager_ContextCancellation tests context handling.
func TestProtocolManager_ContextCancellation(t *testing.T) {
	pm := domain.NewProtocolManager()
	pool := &mockProtocolPool{}
	pm.RegisterPool(domain.ProtocolModbusTCP, pool)

	device := &domain.Device{
		ID:       "test-device",
		Protocol: domain.ProtocolModbusTCP,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// The pool should receive the context for proper cancellation handling
	_, err := pm.ReadTag(ctx, device, &domain.Tag{ID: "tag1"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
