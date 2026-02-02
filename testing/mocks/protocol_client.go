// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

// MockProtocolClient is a mock implementation of a protocol client.
type MockProtocolClient struct {
	mu sync.Mutex

	// Function overrides for custom behavior
	ConnectFunc     func(ctx context.Context) error
	DisconnectFunc  func() error
	ReadTagsFunc    func(ctx context.Context, tags []*domain.Tag) ([]*domain.DataPoint, error)
	WriteTagFunc    func(ctx context.Context, tag *domain.Tag, value interface{}) error
	IsConnectedFunc func() bool

	// Call tracking
	ConnectCalls    int
	DisconnectCalls int
	ReadTagsCalls   int
	WriteTagCalls   int

	// State
	connected atomic.Bool
}

// NewMockProtocolClient creates a new mock protocol client.
func NewMockProtocolClient() *MockProtocolClient {
	return &MockProtocolClient{}
}

// Connect implements the protocol client interface.
func (m *MockProtocolClient) Connect(ctx context.Context) error {
	m.mu.Lock()
	m.ConnectCalls++
	m.mu.Unlock()

	if m.ConnectFunc != nil {
		err := m.ConnectFunc(ctx)
		if err == nil {
			m.connected.Store(true)
		}
		return err
	}
	m.connected.Store(true)
	return nil
}

// Disconnect implements the protocol client interface.
func (m *MockProtocolClient) Disconnect() error {
	m.mu.Lock()
	m.DisconnectCalls++
	m.mu.Unlock()

	if m.DisconnectFunc != nil {
		err := m.DisconnectFunc()
		if err == nil {
			m.connected.Store(false)
		}
		return err
	}
	m.connected.Store(false)
	return nil
}

// ReadTags implements the protocol client interface.
func (m *MockProtocolClient) ReadTags(ctx context.Context, tags []*domain.Tag) ([]*domain.DataPoint, error) {
	m.mu.Lock()
	m.ReadTagsCalls++
	m.mu.Unlock()

	if m.ReadTagsFunc != nil {
		return m.ReadTagsFunc(ctx, tags)
	}

	// Default: return mock data points
	result := make([]*domain.DataPoint, 0, len(tags))
	for _, tag := range tags {
		result = append(result, &domain.DataPoint{
			TagID:   tag.ID,
			Value:   0,
			Quality: domain.QualityGood,
		})
	}
	return result, nil
}

// WriteTag implements the protocol client interface.
func (m *MockProtocolClient) WriteTag(ctx context.Context, tag *domain.Tag, value interface{}) error {
	m.mu.Lock()
	m.WriteTagCalls++
	m.mu.Unlock()

	if m.WriteTagFunc != nil {
		return m.WriteTagFunc(ctx, tag, value)
	}
	return nil
}

// IsConnected implements the protocol client interface.
func (m *MockProtocolClient) IsConnected() bool {
	if m.IsConnectedFunc != nil {
		return m.IsConnectedFunc()
	}
	return m.connected.Load()
}

// Reset clears all call counts and state.
func (m *MockProtocolClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ConnectCalls = 0
	m.DisconnectCalls = 0
	m.ReadTagsCalls = 0
	m.WriteTagCalls = 0
	m.connected.Store(false)
}

// AssertConnectCalled checks that Connect was called the expected number of times.
func (m *MockProtocolClient) AssertConnectCalled(t interface{ Errorf(string, ...interface{}) }, expected int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ConnectCalls != expected {
		t.Errorf("expected %d Connect calls, got %d", expected, m.ConnectCalls)
	}
}

// AssertReadTagsCalled checks that ReadTags was called the expected number of times.
func (m *MockProtocolClient) AssertReadTagsCalled(t interface{ Errorf(string, ...interface{}) }, expected int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ReadTagsCalls != expected {
		t.Errorf("expected %d ReadTags calls, got %d", expected, m.ReadTagsCalls)
	}
}
