//go:build e2e
// +build e2e

// Package e2e tests multi-device scenarios with mixed protocols.
package e2e

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
// Multi-Device Test Infrastructure
// =============================================================================

// Protocol represents a communication protocol.
type Protocol string

const (
	ProtocolModbus Protocol = "modbus"
	ProtocolOPCUA  Protocol = "opcua"
	ProtocolS7     Protocol = "s7"
	ProtocolMQTT   Protocol = "mqtt"
)

// DeviceState represents the state of a device.
type DeviceState int

const (
	DeviceStateDisconnected DeviceState = iota
	DeviceStateConnecting
	DeviceStateConnected
	DeviceStateError
	DeviceStateDegraded
)

func (s DeviceState) String() string {
	switch s {
	case DeviceStateDisconnected:
		return "Disconnected"
	case DeviceStateConnecting:
		return "Connecting"
	case DeviceStateConnected:
		return "Connected"
	case DeviceStateError:
		return "Error"
	case DeviceStateDegraded:
		return "Degraded"
	default:
		return fmt.Sprintf("Unknown(%d)", s)
	}
}

// MockDevice represents a simulated device for testing.
type MockDevice struct {
	mu           sync.RWMutex
	ID           string
	Name         string
	Protocol     Protocol
	Address      string
	State        DeviceState
	Tags         []*MockTag
	PollingRate  time.Duration
	ConnectDelay time.Duration
	ErrorRate    float64 // Probability of read errors (0.0 to 1.0)
	LastPollTime time.Time
	PollCount    int64
	ErrorCount   int64
	BytesRead    int64
	BytesWritten int64
}

// MockTag represents a data tag on a device.
type MockTag struct {
	ID          string
	Name        string
	Address     string
	DataType    string
	Value       interface{}
	Quality     int
	Timestamp   time.Time
	ReadOnly    bool
	ScaleFactor float64
}

// Connect simulates connecting to the device.
func (d *MockDevice) Connect(ctx context.Context) error {
	d.mu.Lock()
	d.State = DeviceStateConnecting
	d.mu.Unlock()

	select {
	case <-ctx.Done():
		d.mu.Lock()
		d.State = DeviceStateDisconnected
		d.mu.Unlock()
		return ctx.Err()
	case <-time.After(d.ConnectDelay):
	}

	d.mu.Lock()
	d.State = DeviceStateConnected
	d.mu.Unlock()
	return nil
}

// Disconnect simulates disconnecting from the device.
func (d *MockDevice) Disconnect() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.State = DeviceStateDisconnected
}

// ReadTags simulates reading tags from the device.
func (d *MockDevice) ReadTags(ctx context.Context) (map[string]interface{}, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.State != DeviceStateConnected {
		return nil, fmt.Errorf("device not connected: state=%s", d.State)
	}

	// Simulate random errors
	if rand.Float64() < d.ErrorRate {
		atomic.AddInt64(&d.ErrorCount, 1)
		return nil, fmt.Errorf("simulated read error")
	}

	results := make(map[string]interface{})
	for _, tag := range d.Tags {
		// Simulate network latency
		time.Sleep(1 * time.Millisecond)

		// Update tag with simulated value
		tag.Value = generateValue(tag.DataType)
		tag.Timestamp = time.Now()
		tag.Quality = 192 // Good quality

		results[tag.ID] = tag.Value
		atomic.AddInt64(&d.BytesRead, 8) // Approximate
	}

	d.LastPollTime = time.Now()
	atomic.AddInt64(&d.PollCount, 1)

	return results, nil
}

// WriteTag simulates writing a tag value.
func (d *MockDevice) WriteTag(ctx context.Context, tagID string, value interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.State != DeviceStateConnected {
		return fmt.Errorf("device not connected: state=%s", d.State)
	}

	for _, tag := range d.Tags {
		if tag.ID == tagID {
			if tag.ReadOnly {
				return fmt.Errorf("tag %s is read-only", tagID)
			}
			tag.Value = value
			tag.Timestamp = time.Now()
			atomic.AddInt64(&d.BytesWritten, 8) // Approximate
			return nil
		}
	}

	return fmt.Errorf("tag %s not found", tagID)
}

// GetState returns the current device state.
func (d *MockDevice) GetState() DeviceState {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.State
}

// GetStats returns device statistics.
func (d *MockDevice) GetStats() map[string]int64 {
	return map[string]int64{
		"polls":         atomic.LoadInt64(&d.PollCount),
		"errors":        atomic.LoadInt64(&d.ErrorCount),
		"bytes_read":    atomic.LoadInt64(&d.BytesRead),
		"bytes_written": atomic.LoadInt64(&d.BytesWritten),
	}
}

// generateValue generates a mock value for the given data type.
func generateValue(dataType string) interface{} {
	switch dataType {
	case "bool":
		return rand.Intn(2) == 1
	case "int16":
		return int16(rand.Intn(65536) - 32768)
	case "int32":
		return rand.Int31()
	case "uint16":
		return uint16(rand.Intn(65536))
	case "uint32":
		return rand.Uint32()
	case "float32":
		return rand.Float32() * 100
	case "float64":
		return rand.Float64() * 100
	case "string":
		return fmt.Sprintf("value_%d", rand.Intn(1000))
	default:
		return rand.Float64()
	}
}

// =============================================================================
// Multi-Device Manager
// =============================================================================

// MultiDeviceManager manages multiple devices across protocols.
type MultiDeviceManager struct {
	mu          sync.RWMutex
	devices     map[string]*MockDevice
	running     bool
	stopChan    chan struct{}
	wg          sync.WaitGroup
	dataChan    chan *DataPoint
	publishRate time.Duration
}

// DataPoint represents a data point from a device.
type DataPoint struct {
	DeviceID  string
	TagID     string
	Value     interface{}
	Quality   int
	Timestamp time.Time
	Protocol  Protocol
}

// NewMultiDeviceManager creates a new multi-device manager.
func NewMultiDeviceManager() *MultiDeviceManager {
	return &MultiDeviceManager{
		devices:     make(map[string]*MockDevice),
		dataChan:    make(chan *DataPoint, 10000),
		publishRate: 100 * time.Millisecond,
	}
}

// AddDevice adds a device to the manager.
func (m *MultiDeviceManager) AddDevice(device *MockDevice) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.devices[device.ID] = device
}

// RemoveDevice removes a device from the manager.
func (m *MultiDeviceManager) RemoveDevice(deviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.devices, deviceID)
}

// GetDevice retrieves a device by ID.
func (m *MultiDeviceManager) GetDevice(deviceID string) *MockDevice {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.devices[deviceID]
}

// GetDeviceCount returns the number of devices.
func (m *MultiDeviceManager) GetDeviceCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.devices)
}

// Start begins polling all devices.
func (m *MultiDeviceManager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("manager already running")
	}
	m.running = true
	m.stopChan = make(chan struct{})
	m.mu.Unlock()

	// Connect all devices
	var connectWg sync.WaitGroup
	for _, device := range m.devices {
		connectWg.Add(1)
		go func(d *MockDevice) {
			defer connectWg.Done()
			_ = d.Connect(ctx)
		}(device)
	}
	connectWg.Wait()

	// Start polling goroutines for each device
	m.mu.RLock()
	for _, device := range m.devices {
		m.wg.Add(1)
		go m.pollDevice(ctx, device)
	}
	m.mu.RUnlock()

	return nil
}

// Stop stops polling all devices.
func (m *MultiDeviceManager) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	close(m.stopChan)
	m.mu.Unlock()

	m.wg.Wait()

	// Disconnect all devices
	m.mu.RLock()
	for _, device := range m.devices {
		device.Disconnect()
	}
	m.mu.RUnlock()
}

// pollDevice polls a single device continuously.
func (m *MultiDeviceManager) pollDevice(ctx context.Context, device *MockDevice) {
	defer m.wg.Done()

	ticker := time.NewTicker(device.PollingRate)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		case <-ticker.C:
			values, err := device.ReadTags(ctx)
			if err != nil {
				continue
			}

			// Publish data points
			for tagID, value := range values {
				select {
				case m.dataChan <- &DataPoint{
					DeviceID:  device.ID,
					TagID:     tagID,
					Value:     value,
					Quality:   192,
					Timestamp: time.Now(),
					Protocol:  device.Protocol,
				}:
				default:
					// Channel full, skip
				}
			}
		}
	}
}

// GetDataChannel returns the channel for receiving data points.
func (m *MultiDeviceManager) GetDataChannel() <-chan *DataPoint {
	return m.dataChan
}

// GetAllDeviceStates returns the state of all devices.
func (m *MultiDeviceManager) GetAllDeviceStates() map[string]DeviceState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make(map[string]DeviceState)
	for id, device := range m.devices {
		states[id] = device.GetState()
	}
	return states
}

// =============================================================================
// Multi-Device Test Cases
// =============================================================================

// TestMultiDeviceBasicOperations tests basic multi-device operations.
func TestMultiDeviceBasicOperations(t *testing.T) {
	manager := NewMultiDeviceManager()

	// Create devices for each protocol
	devices := []*MockDevice{
		{
			ID: "modbus-1", Name: "Modbus PLC 1", Protocol: ProtocolModbus,
			Address: "192.168.1.10:502", PollingRate: 100 * time.Millisecond,
			ConnectDelay: 50 * time.Millisecond, ErrorRate: 0.0,
			Tags: []*MockTag{
				{ID: "temp", Name: "Temperature", Address: "40001", DataType: "float32"},
				{ID: "pressure", Name: "Pressure", Address: "40003", DataType: "float32"},
			},
		},
		{
			ID: "opcua-1", Name: "OPC UA Server 1", Protocol: ProtocolOPCUA,
			Address: "opc.tcp://192.168.1.20:4840", PollingRate: 100 * time.Millisecond,
			ConnectDelay: 100 * time.Millisecond, ErrorRate: 0.0,
			Tags: []*MockTag{
				{ID: "flow", Name: "Flow Rate", Address: "ns=2;s=Flow", DataType: "float64"},
				{ID: "level", Name: "Tank Level", Address: "ns=2;s=Level", DataType: "float64"},
			},
		},
		{
			ID: "s7-1", Name: "Siemens S7 PLC", Protocol: ProtocolS7,
			Address: "192.168.1.30:102", PollingRate: 100 * time.Millisecond,
			ConnectDelay: 75 * time.Millisecond, ErrorRate: 0.0,
			Tags: []*MockTag{
				{ID: "motor_speed", Name: "Motor Speed", Address: "DB1.DBD0", DataType: "float32"},
				{ID: "valve_pos", Name: "Valve Position", Address: "DB1.DBD4", DataType: "float32"},
			},
		},
	}

	for _, d := range devices {
		manager.AddDevice(d)
	}

	if manager.GetDeviceCount() != 3 {
		t.Errorf("Expected 3 devices, got %d", manager.GetDeviceCount())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start manager
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	// Let it run briefly
	time.Sleep(500 * time.Millisecond)

	// Check all devices connected
	states := manager.GetAllDeviceStates()
	for id, state := range states {
		if state != DeviceStateConnected {
			t.Errorf("Device %s not connected: %s", id, state)
		}
	}

	// Stop manager
	manager.Stop()

	// Verify all devices disconnected
	states = manager.GetAllDeviceStates()
	for id, state := range states {
		if state != DeviceStateDisconnected {
			t.Errorf("Device %s not disconnected after stop: %s", id, state)
		}
	}
}

// TestMultiDeviceConcurrentPolling tests concurrent polling across devices.
func TestMultiDeviceConcurrentPolling(t *testing.T) {
	manager := NewMultiDeviceManager()

	// Create 10 devices
	for i := 0; i < 10; i++ {
		device := &MockDevice{
			ID:           fmt.Sprintf("device-%d", i),
			Name:         fmt.Sprintf("Test Device %d", i),
			Protocol:     []Protocol{ProtocolModbus, ProtocolOPCUA, ProtocolS7}[i%3],
			Address:      fmt.Sprintf("192.168.1.%d:502", 10+i),
			PollingRate:  50 * time.Millisecond,
			ConnectDelay: 20 * time.Millisecond,
			ErrorRate:    0.0,
			Tags: []*MockTag{
				{ID: fmt.Sprintf("tag1-%d", i), DataType: "float32"},
				{ID: fmt.Sprintf("tag2-%d", i), DataType: "float32"},
			},
		}
		manager.AddDevice(device)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	// Collect data points
	var received int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case <-manager.GetDataChannel():
				atomic.AddInt64(&received, 1)
			}
		}
	}()

	// Let it run
	time.Sleep(1 * time.Second)
	manager.Stop()
	<-done

	count := atomic.LoadInt64(&received)
	t.Logf("Received %d data points from 10 devices in 1 second", count)

	// Should have received at least some data
	if count == 0 {
		t.Error("Expected to receive data points")
	}
}

// TestMultiDeviceProtocolMix tests a mix of protocols.
func TestMultiDeviceProtocolMix(t *testing.T) {
	manager := NewMultiDeviceManager()

	protocols := []Protocol{ProtocolModbus, ProtocolOPCUA, ProtocolS7}
	protocolCounts := make(map[Protocol]int)

	for i, proto := range protocols {
		for j := 0; j < 3; j++ {
			device := &MockDevice{
				ID:           fmt.Sprintf("%s-%d", proto, j),
				Protocol:     proto,
				PollingRate:  100 * time.Millisecond,
				ConnectDelay: 10 * time.Millisecond,
				Tags: []*MockTag{
					{ID: fmt.Sprintf("tag-%d-%d", i, j), DataType: "float32"},
				},
			}
			manager.AddDevice(device)
			protocolCounts[proto]++
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	// Track data points by protocol
	dataByProtocol := make(map[Protocol]int64)
	var mu sync.Mutex

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case dp := <-manager.GetDataChannel():
				mu.Lock()
				dataByProtocol[dp.Protocol]++
				mu.Unlock()
			}
		}
	}()

	time.Sleep(500 * time.Millisecond)
	manager.Stop()
	cancel()
	<-done

	t.Log("Data points received by protocol:")
	for proto, count := range dataByProtocol {
		t.Logf("  %s: %d", proto, count)
	}

	// Verify all protocols produced data
	for _, proto := range protocols {
		if dataByProtocol[proto] == 0 {
			t.Errorf("No data received from protocol %s", proto)
		}
	}
}

// TestMultiDeviceFailover tests device failure and recovery.
func TestMultiDeviceFailover(t *testing.T) {
	manager := NewMultiDeviceManager()

	// Create a device that will fail
	failingDevice := &MockDevice{
		ID:           "failing-device",
		Protocol:     ProtocolModbus,
		PollingRate:  50 * time.Millisecond,
		ConnectDelay: 10 * time.Millisecond,
		ErrorRate:    0.5, // 50% error rate
		Tags: []*MockTag{
			{ID: "tag1", DataType: "float32"},
		},
	}

	// Create a reliable device
	reliableDevice := &MockDevice{
		ID:           "reliable-device",
		Protocol:     ProtocolOPCUA,
		PollingRate:  50 * time.Millisecond,
		ConnectDelay: 10 * time.Millisecond,
		ErrorRate:    0.0, // No errors
		Tags: []*MockTag{
			{ID: "tag1", DataType: "float32"},
		},
	}

	manager.AddDevice(failingDevice)
	manager.AddDevice(reliableDevice)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	time.Sleep(1 * time.Second)
	manager.Stop()

	// Check statistics
	failingStats := failingDevice.GetStats()
	reliableStats := reliableDevice.GetStats()

	t.Logf("Failing device: polls=%d, errors=%d",
		failingStats["polls"], failingStats["errors"])
	t.Logf("Reliable device: polls=%d, errors=%d",
		reliableStats["polls"], reliableStats["errors"])

	// Failing device should have errors
	if failingStats["errors"] == 0 {
		t.Log("Warning: Failing device had no errors (probabilistic)")
	}

	// Reliable device should have no errors
	if reliableStats["errors"] > 0 {
		t.Errorf("Reliable device had unexpected errors: %d", reliableStats["errors"])
	}
}

// TestMultiDeviceDynamicAddRemove tests adding/removing devices at runtime.
func TestMultiDeviceDynamicAddRemove(t *testing.T) {
	manager := NewMultiDeviceManager()

	// Start with 2 devices
	for i := 0; i < 2; i++ {
		manager.AddDevice(&MockDevice{
			ID:           fmt.Sprintf("initial-%d", i),
			Protocol:     ProtocolModbus,
			PollingRate:  100 * time.Millisecond,
			ConnectDelay: 10 * time.Millisecond,
			Tags:         []*MockTag{{ID: "tag", DataType: "float32"}},
		})
	}

	if manager.GetDeviceCount() != 2 {
		t.Errorf("Expected 2 devices, got %d", manager.GetDeviceCount())
	}

	// Add a device
	manager.AddDevice(&MockDevice{
		ID:           "added-1",
		Protocol:     ProtocolOPCUA,
		PollingRate:  100 * time.Millisecond,
		ConnectDelay: 10 * time.Millisecond,
		Tags:         []*MockTag{{ID: "tag", DataType: "float32"}},
	})

	if manager.GetDeviceCount() != 3 {
		t.Errorf("Expected 3 devices after add, got %d", manager.GetDeviceCount())
	}

	// Remove a device
	manager.RemoveDevice("initial-0")

	if manager.GetDeviceCount() != 2 {
		t.Errorf("Expected 2 devices after remove, got %d", manager.GetDeviceCount())
	}

	// Verify correct device removed
	if manager.GetDevice("initial-0") != nil {
		t.Error("Device initial-0 should have been removed")
	}
	if manager.GetDevice("initial-1") == nil {
		t.Error("Device initial-1 should still exist")
	}
	if manager.GetDevice("added-1") == nil {
		t.Error("Device added-1 should exist")
	}
}

// TestMultiDeviceTagOperations tests reading and writing tags across devices.
func TestMultiDeviceTagOperations(t *testing.T) {
	manager := NewMultiDeviceManager()

	device := &MockDevice{
		ID:           "test-device",
		Protocol:     ProtocolModbus,
		PollingRate:  100 * time.Millisecond,
		ConnectDelay: 10 * time.Millisecond,
		Tags: []*MockTag{
			{ID: "writable", DataType: "float32", ReadOnly: false},
			{ID: "readonly", DataType: "float32", ReadOnly: true},
		},
	}
	manager.AddDevice(device)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect device
	if err := device.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Read tags
	values, err := device.ReadTags(ctx)
	if err != nil {
		t.Fatalf("Failed to read tags: %v", err)
	}
	t.Logf("Read values: %v", values)

	// Write to writable tag
	if err := device.WriteTag(ctx, "writable", float32(123.45)); err != nil {
		t.Errorf("Failed to write writable tag: %v", err)
	}

	// Attempt to write to read-only tag
	if err := device.WriteTag(ctx, "readonly", float32(123.45)); err == nil {
		t.Error("Expected error writing to read-only tag")
	}

	// Write to non-existent tag
	if err := device.WriteTag(ctx, "nonexistent", float32(123.45)); err == nil {
		t.Error("Expected error writing to non-existent tag")
	}
}

// TestMultiDeviceHighLoad tests high load with many devices.
func TestMultiDeviceHighLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high load test in short mode")
	}

	manager := NewMultiDeviceManager()

	// Create 50 devices with 10 tags each
	for i := 0; i < 50; i++ {
		tags := make([]*MockTag, 10)
		for j := 0; j < 10; j++ {
			tags[j] = &MockTag{
				ID:       fmt.Sprintf("tag-%d", j),
				DataType: "float32",
			}
		}
		manager.AddDevice(&MockDevice{
			ID:           fmt.Sprintf("device-%d", i),
			Protocol:     []Protocol{ProtocolModbus, ProtocolOPCUA, ProtocolS7}[i%3],
			PollingRate:  50 * time.Millisecond,
			ConnectDelay: 5 * time.Millisecond,
			Tags:         tags,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	// Count data points
	var count int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case <-manager.GetDataChannel():
				atomic.AddInt64(&count, 1)
			}
		}
	}()

	time.Sleep(3 * time.Second)
	manager.Stop()
	cancel()
	<-done

	total := atomic.LoadInt64(&count)
	rate := float64(total) / 3.0
	t.Logf("High load test: %d data points in 3 seconds (%.0f/sec)", total, rate)

	// Should handle high throughput
	if rate < 1000 {
		t.Logf("Warning: Rate %.0f/sec is lower than expected", rate)
	}
}

// TestMultiDeviceConnectionRecovery tests connection recovery behavior.
func TestMultiDeviceConnectionRecovery(t *testing.T) {
	device := &MockDevice{
		ID:           "recovery-test",
		Protocol:     ProtocolModbus,
		PollingRate:  100 * time.Millisecond,
		ConnectDelay: 50 * time.Millisecond,
		Tags: []*MockTag{
			{ID: "tag1", DataType: "float32"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Initial connection
	if err := device.Connect(ctx); err != nil {
		t.Fatalf("Initial connect failed: %v", err)
	}
	if device.GetState() != DeviceStateConnected {
		t.Errorf("Expected Connected, got %s", device.GetState())
	}

	// Simulate disconnect
	device.Disconnect()
	if device.GetState() != DeviceStateDisconnected {
		t.Errorf("Expected Disconnected, got %s", device.GetState())
	}

	// Reconnect
	if err := device.Connect(ctx); err != nil {
		t.Fatalf("Reconnect failed: %v", err)
	}
	if device.GetState() != DeviceStateConnected {
		t.Errorf("Expected Connected after recovery, got %s", device.GetState())
	}
}

// TestMultiDeviceDataAggregation tests aggregating data from multiple devices.
func TestMultiDeviceDataAggregation(t *testing.T) {
	manager := NewMultiDeviceManager()

	// Create devices with specific tags for aggregation testing
	for i := 0; i < 5; i++ {
		manager.AddDevice(&MockDevice{
			ID:           fmt.Sprintf("sensor-%d", i),
			Protocol:     ProtocolModbus,
			PollingRate:  50 * time.Millisecond,
			ConnectDelay: 10 * time.Millisecond,
			Tags: []*MockTag{
				{ID: "temperature", DataType: "float32"},
				{ID: "humidity", DataType: "float32"},
			},
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	// Aggregate data by tag
	aggregated := make(map[string][]interface{})
	var mu sync.Mutex

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case dp := <-manager.GetDataChannel():
				mu.Lock()
				aggregated[dp.TagID] = append(aggregated[dp.TagID], dp.Value)
				mu.Unlock()
			}
		}
	}()

	time.Sleep(500 * time.Millisecond)
	manager.Stop()
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()

	t.Log("Aggregated data:")
	for tagID, values := range aggregated {
		t.Logf("  %s: %d values", tagID, len(values))

		// Verify we got data from multiple devices
		if len(values) < 5 {
			t.Logf("  Warning: Expected at least 5 values for %s", tagID)
		}
	}
}

// TestMultiDeviceMetrics tests device metrics collection.
func TestMultiDeviceMetrics(t *testing.T) {
	device := &MockDevice{
		ID:           "metrics-test",
		Protocol:     ProtocolModbus,
		PollingRate:  50 * time.Millisecond,
		ConnectDelay: 10 * time.Millisecond,
		Tags: []*MockTag{
			{ID: "tag1", DataType: "float32"},
			{ID: "tag2", DataType: "float32"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Connect and poll
	if err := device.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Perform several polls
	for i := 0; i < 10; i++ {
		_, _ = device.ReadTags(ctx)
		time.Sleep(10 * time.Millisecond)
	}

	// Check metrics
	stats := device.GetStats()

	t.Logf("Device metrics:")
	t.Logf("  Polls: %d", stats["polls"])
	t.Logf("  Errors: %d", stats["errors"])
	t.Logf("  Bytes Read: %d", stats["bytes_read"])
	t.Logf("  Bytes Written: %d", stats["bytes_written"])

	if stats["polls"] != 10 {
		t.Errorf("Expected 10 polls, got %d", stats["polls"])
	}

	if stats["bytes_read"] == 0 {
		t.Error("Expected bytes_read > 0")
	}
}
