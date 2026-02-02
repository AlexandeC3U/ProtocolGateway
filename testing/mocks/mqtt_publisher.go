// Package mocks provides mock implementations for testing.
package mocks

import (
	"sync"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

// MockMQTTPublisher is a mock implementation of the MQTT publisher.
type MockMQTTPublisher struct {
	mu sync.Mutex

	// Function overrides
	PublishFunc     func(dp *domain.DataPoint) error
	ConnectFunc     func() error
	DisconnectFunc  func()
	IsConnectedFunc func() bool

	// Call tracking
	PublishCalls    int
	ConnectCalls    int
	DisconnectCalls int

	// Published messages for verification
	PublishedMessages []*domain.DataPoint

	// State
	connected bool
}

// NewMockMQTTPublisher creates a new mock MQTT publisher.
func NewMockMQTTPublisher() *MockMQTTPublisher {
	return &MockMQTTPublisher{
		PublishedMessages: make([]*domain.DataPoint, 0),
	}
}

// Publish implements the publisher interface.
func (m *MockMQTTPublisher) Publish(dp *domain.DataPoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.PublishCalls++
	m.PublishedMessages = append(m.PublishedMessages, dp)

	if m.PublishFunc != nil {
		return m.PublishFunc(dp)
	}
	return nil
}

// Connect implements the publisher interface.
func (m *MockMQTTPublisher) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ConnectCalls++

	if m.ConnectFunc != nil {
		return m.ConnectFunc()
	}
	m.connected = true
	return nil
}

// Disconnect implements the publisher interface.
func (m *MockMQTTPublisher) Disconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DisconnectCalls++

	if m.DisconnectFunc != nil {
		m.DisconnectFunc()
		return
	}
	m.connected = false
}

// IsConnected returns the connection status.
func (m *MockMQTTPublisher) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.IsConnectedFunc != nil {
		return m.IsConnectedFunc()
	}
	return m.connected
}

// Reset clears all state.
func (m *MockMQTTPublisher) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PublishCalls = 0
	m.ConnectCalls = 0
	m.DisconnectCalls = 0
	m.PublishedMessages = make([]*domain.DataPoint, 0)
	m.connected = false
}

// GetPublishedMessages returns a copy of published messages.
func (m *MockMQTTPublisher) GetPublishedMessages() []*domain.DataPoint {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*domain.DataPoint, len(m.PublishedMessages))
	copy(result, m.PublishedMessages)
	return result
}

// AssertPublishCalled checks that Publish was called the expected number of times.
func (m *MockMQTTPublisher) AssertPublishCalled(t interface{ Errorf(string, ...interface{}) }, expected int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.PublishCalls != expected {
		t.Errorf("expected %d Publish calls, got %d", expected, m.PublishCalls)
	}
}

// LastPublishedMessage returns the most recently published message.
func (m *MockMQTTPublisher) LastPublishedMessage() *domain.DataPoint {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.PublishedMessages) == 0 {
		return nil
	}
	return m.PublishedMessages[len(m.PublishedMessages)-1]
}
