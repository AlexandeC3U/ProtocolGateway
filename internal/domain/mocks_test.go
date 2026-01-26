package domain

import (
	"context"
	"sync"
)

// MockProtocolPool is a mock implementation of ProtocolPool for testing
type MockProtocolPool struct {
	mu sync.RWMutex

	// Configurable responses
	ReadTagsFunc func(ctx context.Context, device *Device, tags []*Tag) ([]*DataPoint, error)
	ReadTagFunc  func(ctx context.Context, device *Device, tag *Tag) (*DataPoint, error)
	WriteTagFunc func(ctx context.Context, device *Device, tag *Tag, value interface{}) error
	CloseFunc    func() error
	HealthFunc   func(ctx context.Context) error

	// Call tracking
	ReadTagsCalls []ReadTagsCall
	ReadTagCalls  []ReadTagCall
	WriteTagCalls []WriteTagCall
	CloseCalls    int
	HealthCalls   int
}

// ReadTagsCall records a call to ReadTags
type ReadTagsCall struct {
	Device *Device
	Tags   []*Tag
}

// ReadTagCall records a call to ReadTag
type ReadTagCall struct {
	Device *Device
	Tag    *Tag
}

// WriteTagCall records a call to WriteTag
type WriteTagCall struct {
	Device *Device
	Tag    *Tag
	Value  interface{}
}

// NewMockProtocolPool creates a new mock protocol pool with default implementations
func NewMockProtocolPool() *MockProtocolPool {
	return &MockProtocolPool{
		ReadTagsCalls: make([]ReadTagsCall, 0),
		ReadTagCalls:  make([]ReadTagCall, 0),
		WriteTagCalls: make([]WriteTagCall, 0),
	}
}

// ReadTags implements ProtocolPool
func (m *MockProtocolPool) ReadTags(ctx context.Context, device *Device, tags []*Tag) ([]*DataPoint, error) {
	m.mu.Lock()
	m.ReadTagsCalls = append(m.ReadTagsCalls, ReadTagsCall{Device: device, Tags: tags})
	m.mu.Unlock()

	if m.ReadTagsFunc != nil {
		return m.ReadTagsFunc(ctx, device, tags)
	}

	// Default: return good quality data points
	points := make([]*DataPoint, len(tags))
	for i, tag := range tags {
		points[i] = NewDataPoint(device.ID, tag.ID, "", 0.0, tag.Unit, QualityGood)
	}
	return points, nil
}

// ReadTag implements ProtocolPool
func (m *MockProtocolPool) ReadTag(ctx context.Context, device *Device, tag *Tag) (*DataPoint, error) {
	m.mu.Lock()
	m.ReadTagCalls = append(m.ReadTagCalls, ReadTagCall{Device: device, Tag: tag})
	m.mu.Unlock()

	if m.ReadTagFunc != nil {
		return m.ReadTagFunc(ctx, device, tag)
	}

	return NewDataPoint(device.ID, tag.ID, "", 0.0, tag.Unit, QualityGood), nil
}

// WriteTag implements ProtocolPool
func (m *MockProtocolPool) WriteTag(ctx context.Context, device *Device, tag *Tag, value interface{}) error {
	m.mu.Lock()
	m.WriteTagCalls = append(m.WriteTagCalls, WriteTagCall{Device: device, Tag: tag, Value: value})
	m.mu.Unlock()

	if m.WriteTagFunc != nil {
		return m.WriteTagFunc(ctx, device, tag, value)
	}

	return nil
}

// Close implements ProtocolPool
func (m *MockProtocolPool) Close() error {
	m.mu.Lock()
	m.CloseCalls++
	m.mu.Unlock()

	if m.CloseFunc != nil {
		return m.CloseFunc()
	}

	return nil
}

// HealthCheck implements ProtocolPool
func (m *MockProtocolPool) HealthCheck(ctx context.Context) error {
	m.mu.Lock()
	m.HealthCalls++
	m.mu.Unlock()

	if m.HealthFunc != nil {
		return m.HealthFunc(ctx)
	}

	return nil
}

// Reset clears all recorded calls
func (m *MockProtocolPool) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ReadTagsCalls = make([]ReadTagsCall, 0)
	m.ReadTagCalls = make([]ReadTagCall, 0)
	m.WriteTagCalls = make([]WriteTagCall, 0)
	m.CloseCalls = 0
	m.HealthCalls = 0
}

// Verify MockProtocolPool implements ProtocolPool
var _ ProtocolPool = (*MockProtocolPool)(nil)
