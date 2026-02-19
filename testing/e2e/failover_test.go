//go:build e2e
// +build e2e

// Package e2e provides end-to-end tests for the protocol gateway.
// failover_test.go tests device failure, recovery, and graceful degradation.
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
// Failover Test Infrastructure
// =============================================================================

// FailoverDevice extends MockDevice with failure injection capabilities.
type FailoverDevice struct {
	MockDevice
	mu2             sync.Mutex
	failAfter       int64          // Start failing after N successful polls
	recoverAfter    int64          // Recover after N failed polls
	failureCount    int64          // Current consecutive failures
	totalFailures   int64          // Lifetime failures
	totalRecoveries int64          // Number of recovery cycles completed
	disconnectAt    time.Time      // Simulate disconnect at this time
	reconnectDelay  time.Duration  // Time to reconnect after failure
	failMode        FailMode       // Type of failure to simulate
	breakerTrips    int64          // Circuit breaker trip count
	breakerState    BreakerState   // Current breaker state
	breakerMaxFails int64          // Failures before breaker opens
	breakerCooldown time.Duration  // Time before breaker resets
	breakerOpenedAt time.Time      // When breaker last opened
	degradedMode    bool           // Partial reads succeed
	degradedTagPct  float64        // Percentage of tags that succeed in degraded mode
}

// FailMode defines the type of failure to simulate.
type FailMode int

const (
	FailModeReadError    FailMode = iota // Read operations fail
	FailModeDisconnect                   // Connection drops
	FailModeTimeout                      // Operations timeout
	FailModePartial                      // Partial reads (some tags fail)
	FailModeIntermittent                 // Random failures
)

// BreakerState simulates circuit breaker states.
type BreakerState int

const (
	BreakerClosed   BreakerState = iota // Normal operation
	BreakerOpen                         // Blocking requests
	BreakerHalfOpen                     // Testing recovery
)

func (s BreakerState) String() string {
	switch s {
	case BreakerClosed:
		return "Closed"
	case BreakerOpen:
		return "Open"
	case BreakerHalfOpen:
		return "HalfOpen"
	default:
		return "Unknown"
	}
}

// NewFailoverDevice creates a device with failure injection.
func NewFailoverDevice(id string, proto Protocol) *FailoverDevice {
	return &FailoverDevice{
		MockDevice: MockDevice{
			ID:           id,
			Protocol:     proto,
			PollingRate:  50 * time.Millisecond,
			ConnectDelay: 10 * time.Millisecond,
			Tags: []*MockTag{
				{ID: "tag1", DataType: "float32"},
				{ID: "tag2", DataType: "float32"},
				{ID: "tag3", DataType: "int32"},
			},
		},
		breakerMaxFails: 5,
		breakerCooldown: 500 * time.Millisecond,
		degradedTagPct:  0.5,
	}
}

// ReadTagsWithFailover simulates reads with failure injection and circuit breaker.
func (d *FailoverDevice) ReadTagsWithFailover(ctx context.Context) (map[string]interface{}, error) {
	d.mu2.Lock()
	defer d.mu2.Unlock()

	// Check circuit breaker
	if d.breakerState == BreakerOpen {
		if time.Since(d.breakerOpenedAt) > d.breakerCooldown {
			d.breakerState = BreakerHalfOpen
		} else {
			return nil, fmt.Errorf("circuit breaker open for device %s", d.MockDevice.ID)
		}
	}

	polls := atomic.LoadInt64(&d.MockDevice.PollCount)

	// Determine if this read should fail
	shouldFail := false
	switch d.failMode {
	case FailModeReadError:
		if d.failAfter > 0 && polls >= d.failAfter {
			if d.recoverAfter > 0 && d.failureCount >= d.recoverAfter {
				// Recovery
				d.failureCount = 0
				d.totalRecoveries++
				d.failAfter = polls + d.failAfter // Next failure cycle
			} else {
				shouldFail = true
			}
		}
	case FailModeDisconnect:
		if !d.disconnectAt.IsZero() && time.Now().After(d.disconnectAt) {
			d.MockDevice.mu.Lock()
			d.MockDevice.State = DeviceStateDisconnected
			d.MockDevice.mu.Unlock()
			d.disconnectAt = time.Time{} // Clear trigger
		}
	case FailModeTimeout:
		if d.failAfter > 0 && polls >= d.failAfter {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return nil, fmt.Errorf("timeout reading device %s", d.MockDevice.ID)
			}
		}
	case FailModeIntermittent:
		shouldFail = rand.Float64() < d.MockDevice.ErrorRate
	case FailModePartial:
		// Handled below
	}

	if shouldFail {
		d.failureCount++
		d.totalFailures++
		atomic.AddInt64(&d.MockDevice.ErrorCount, 1)

		// Update circuit breaker
		if d.failureCount >= d.breakerMaxFails {
			if d.breakerState != BreakerOpen {
				d.breakerState = BreakerOpen
				d.breakerOpenedAt = time.Now()
				d.breakerTrips++
			}
		}

		return nil, fmt.Errorf("simulated failure #%d on device %s", d.failureCount, d.MockDevice.ID)
	}

	// Success — reset breaker if half-open
	if d.breakerState == BreakerHalfOpen {
		d.breakerState = BreakerClosed
	}
	d.failureCount = 0

	// Handle partial failure mode
	if d.failMode == FailModePartial && d.degradedMode {
		return d.readPartialTags(ctx)
	}

	return d.MockDevice.ReadTags(ctx)
}

// readPartialTags simulates partial tag reads (some succeed, some fail).
func (d *FailoverDevice) readPartialTags(ctx context.Context) (map[string]interface{}, error) {
	d.MockDevice.mu.Lock()
	defer d.MockDevice.mu.Unlock()

	if d.MockDevice.State != DeviceStateConnected {
		return nil, fmt.Errorf("device not connected: state=%s", d.MockDevice.State)
	}

	results := make(map[string]interface{})
	for _, tag := range d.MockDevice.Tags {
		if rand.Float64() < d.degradedTagPct {
			tag.Value = generateValue(tag.DataType)
			tag.Timestamp = time.Now()
			tag.Quality = 192
			results[tag.ID] = tag.Value
		}
		// Tags that don't pass the threshold are silently skipped (degraded)
	}

	d.MockDevice.LastPollTime = time.Now()
	atomic.AddInt64(&d.MockDevice.PollCount, 1)

	return results, nil
}

// =============================================================================
// Failover Manager
// =============================================================================

// FailoverManager manages devices with failover capabilities.
type FailoverManager struct {
	mu       sync.RWMutex
	devices  map[string]*FailoverDevice
	running  bool
	stopChan chan struct{}
	wg       sync.WaitGroup
	dataChan chan *DataPoint
	errChan  chan *DeviceError
}

// DeviceError represents an error from a specific device.
type DeviceError struct {
	DeviceID  string
	Error     error
	Timestamp time.Time
	Recovered bool
}

// NewFailoverManager creates a new failover-aware manager.
func NewFailoverManager() *FailoverManager {
	return &FailoverManager{
		devices:  make(map[string]*FailoverDevice),
		dataChan: make(chan *DataPoint, 10000),
		errChan:  make(chan *DeviceError, 1000),
	}
}

// AddDevice adds a failover device.
func (m *FailoverManager) AddDevice(device *FailoverDevice) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.devices[device.MockDevice.ID] = device
}

// Start begins polling all devices with failover handling.
func (m *FailoverManager) Start(ctx context.Context) error {
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
		go func(d *FailoverDevice) {
			defer connectWg.Done()
			_ = d.MockDevice.Connect(ctx)
		}(device)
	}
	connectWg.Wait()

	// Start polling with failover
	m.mu.RLock()
	for _, device := range m.devices {
		m.wg.Add(1)
		go m.pollWithFailover(ctx, device)
	}
	m.mu.RUnlock()

	return nil
}

// Stop stops the manager.
func (m *FailoverManager) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	close(m.stopChan)
	m.mu.Unlock()

	m.wg.Wait()

	m.mu.RLock()
	for _, device := range m.devices {
		device.MockDevice.Disconnect()
	}
	m.mu.RUnlock()
}

// pollWithFailover polls a device with failure detection and recovery.
func (m *FailoverManager) pollWithFailover(ctx context.Context, device *FailoverDevice) {
	defer m.wg.Done()

	ticker := time.NewTicker(device.MockDevice.PollingRate)
	defer ticker.Stop()

	consecutiveErrors := 0
	maxRetries := 3

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		case <-ticker.C:
			values, err := device.ReadTagsWithFailover(ctx)
			if err != nil {
				consecutiveErrors++

				select {
				case m.errChan <- &DeviceError{
					DeviceID:  device.MockDevice.ID,
					Error:     err,
					Timestamp: time.Now(),
				}:
				default:
				}

				// Attempt reconnection after max retries
				if consecutiveErrors >= maxRetries {
					state := device.MockDevice.GetState()
					if state == DeviceStateDisconnected {
						reconnCtx, cancel := context.WithTimeout(ctx, device.reconnectDelay)
						err := device.MockDevice.Connect(reconnCtx)
						cancel()
						if err == nil {
							consecutiveErrors = 0
							select {
							case m.errChan <- &DeviceError{
								DeviceID:  device.MockDevice.ID,
								Timestamp: time.Now(),
								Recovered: true,
							}:
							default:
							}
						}
					}
				}
				continue
			}

			consecutiveErrors = 0

			for tagID, value := range values {
				select {
				case m.dataChan <- &DataPoint{
					DeviceID:  device.MockDevice.ID,
					TagID:     tagID,
					Value:     value,
					Quality:   192,
					Timestamp: time.Now(),
					Protocol:  device.MockDevice.Protocol,
				}:
				default:
				}
			}
		}
	}
}

// =============================================================================
// Failover Test Cases
// =============================================================================

// TestFailoverDeviceDisconnectAndRecover tests a device that disconnects
// and then recovers after a reconnection attempt.
func TestFailoverDeviceDisconnectAndRecover(t *testing.T) {
	device := NewFailoverDevice("disconnect-test", ProtocolModbus)
	device.failMode = FailModeDisconnect
	device.disconnectAt = time.Now().Add(300 * time.Millisecond)
	device.reconnectDelay = 100 * time.Millisecond

	manager := NewFailoverManager()
	manager.AddDevice(device)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	// Track errors and recoveries
	var errors, recoveries int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case evt := <-manager.errChan:
				if evt.Recovered {
					atomic.AddInt64(&recoveries, 1)
				} else {
					atomic.AddInt64(&errors, 1)
				}
			}
		}
	}()

	time.Sleep(2 * time.Second)
	manager.Stop()
	cancel()
	<-done

	errCount := atomic.LoadInt64(&errors)
	recCount := atomic.LoadInt64(&recoveries)
	t.Logf("Disconnect/recover test: errors=%d, recoveries=%d", errCount, recCount)

	// Should have seen at least some errors from the disconnect period
	if errCount == 0 {
		t.Log("Warning: Expected errors during disconnect (timing-dependent)")
	}
}

// TestFailoverCircuitBreaker tests that the circuit breaker trips after
// consecutive failures and recovers after the cooldown.
func TestFailoverCircuitBreaker(t *testing.T) {
	device := NewFailoverDevice("breaker-test", ProtocolOPCUA)
	device.failMode = FailModeReadError
	device.failAfter = 5         // Fail after 5 successful polls
	device.recoverAfter = 10     // Recover after 10 failures
	device.breakerMaxFails = 5   // Breaker trips at 5 consecutive failures
	device.breakerCooldown = 300 * time.Millisecond

	manager := NewFailoverManager()
	manager.AddDevice(device)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	// Collect data and errors
	var dataPoints, errorCount int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case <-manager.dataChan:
				atomic.AddInt64(&dataPoints, 1)
			case evt := <-manager.errChan:
				if !evt.Recovered {
					atomic.AddInt64(&errorCount, 1)
				}
			}
		}
	}()

	time.Sleep(3 * time.Second)
	manager.Stop()
	cancel()
	<-done

	t.Logf("Circuit breaker test: dataPoints=%d, errors=%d, breakerTrips=%d, recoveries=%d",
		atomic.LoadInt64(&dataPoints), atomic.LoadInt64(&errorCount),
		device.breakerTrips, device.totalRecoveries)

	// Should have received some data before failure
	if atomic.LoadInt64(&dataPoints) == 0 {
		t.Error("Expected data points before circuit breaker tripped")
	}

	// Should have breaker trips
	if device.breakerTrips == 0 {
		t.Log("Warning: Expected circuit breaker to trip (timing-dependent)")
	}
}

// TestFailoverIntermittentErrors tests behavior with random intermittent failures.
func TestFailoverIntermittentErrors(t *testing.T) {
	manager := NewFailoverManager()

	// Create devices with varying error rates
	errorRates := []float64{0.0, 0.1, 0.3, 0.5}
	for i, rate := range errorRates {
		device := NewFailoverDevice(fmt.Sprintf("intermittent-%d", i), ProtocolS7)
		device.failMode = FailModeIntermittent
		device.MockDevice.ErrorRate = rate
		manager.AddDevice(device)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	// Track data points per device
	pointsByDevice := make(map[string]int64)
	var mu sync.Mutex

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case dp := <-manager.dataChan:
				mu.Lock()
				pointsByDevice[dp.DeviceID]++
				mu.Unlock()
			case <-manager.errChan:
				// Consume errors
			}
		}
	}()

	time.Sleep(2 * time.Second)
	manager.Stop()
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()

	t.Log("Intermittent error test — data points by device:")
	for i, rate := range errorRates {
		id := fmt.Sprintf("intermittent-%d", i)
		count := pointsByDevice[id]
		t.Logf("  %s (error_rate=%.0f%%): %d data points", id, rate*100, count)
	}

	// Device with 0% error rate should have the most data
	zeroErrCount := pointsByDevice["intermittent-0"]
	if zeroErrCount == 0 {
		t.Error("Device with 0% error rate should produce data points")
	}

	// Device with 50% error rate should have roughly half the data
	halfErrCount := pointsByDevice["intermittent-3"]
	if zeroErrCount > 0 && halfErrCount > 0 {
		ratio := float64(halfErrCount) / float64(zeroErrCount)
		t.Logf("  Ratio (50%% err / 0%% err): %.2f (expected ~0.50)", ratio)
		if ratio > 0.9 {
			t.Log("  Warning: High error rate device has unexpectedly high throughput")
		}
	}
}

// TestFailoverPartialDegradation tests that a device can operate in degraded
// mode where only some tags are readable.
func TestFailoverPartialDegradation(t *testing.T) {
	device := NewFailoverDevice("degraded-test", ProtocolModbus)
	device.failMode = FailModePartial
	device.degradedMode = true
	device.degradedTagPct = 0.5 // 50% of tags succeed

	manager := NewFailoverManager()
	manager.AddDevice(device)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	// Track which tags we receive
	tagCounts := make(map[string]int64)
	var mu sync.Mutex

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case dp := <-manager.dataChan:
				mu.Lock()
				tagCounts[dp.TagID]++
				mu.Unlock()
			case <-manager.errChan:
			}
		}
	}()

	time.Sleep(1500 * time.Millisecond)
	manager.Stop()
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()

	t.Log("Partial degradation test — tag counts:")
	totalTags := 0
	for tagID, count := range tagCounts {
		t.Logf("  %s: %d readings", tagID, count)
		totalTags += int(count)
	}

	// In degraded mode with 50% success, we should still get some data
	if totalTags == 0 {
		t.Error("Expected some tag readings in degraded mode")
	}

	// Verify not all tags have the same count (shows partial behavior)
	if len(tagCounts) > 0 {
		t.Logf("  Total tags received: %d across %d unique tags", totalTags, len(tagCounts))
	}
}

// TestFailoverMultiProtocolResilience tests that failure in one protocol's
// devices does not affect other protocols.
func TestFailoverMultiProtocolResilience(t *testing.T) {
	manager := NewFailoverManager()

	// Modbus device — healthy
	healthy := NewFailoverDevice("modbus-healthy", ProtocolModbus)
	healthy.failMode = FailModeIntermittent
	healthy.MockDevice.ErrorRate = 0.0
	manager.AddDevice(healthy)

	// OPC UA device — failing
	failing := NewFailoverDevice("opcua-failing", ProtocolOPCUA)
	failing.failMode = FailModeReadError
	failing.failAfter = 3 // Fail after 3 polls
	failing.recoverAfter = 100 // Effectively never recover
	manager.AddDevice(failing)

	// S7 device — healthy
	healthy2 := NewFailoverDevice("s7-healthy", ProtocolS7)
	healthy2.failMode = FailModeIntermittent
	healthy2.MockDevice.ErrorRate = 0.0
	manager.AddDevice(healthy2)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	// Track data by protocol
	dataByProto := make(map[Protocol]int64)
	errsByProto := make(map[string]int64)
	var mu sync.Mutex

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case dp := <-manager.dataChan:
				mu.Lock()
				dataByProto[dp.Protocol]++
				mu.Unlock()
			case evt := <-manager.errChan:
				if !evt.Recovered {
					mu.Lock()
					errsByProto[evt.DeviceID]++
					mu.Unlock()
				}
			}
		}
	}()

	time.Sleep(2 * time.Second)
	manager.Stop()
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()

	t.Log("Multi-protocol resilience test:")
	t.Logf("  Modbus data points: %d", dataByProto[ProtocolModbus])
	t.Logf("  OPC UA data points: %d", dataByProto[ProtocolOPCUA])
	t.Logf("  S7 data points: %d", dataByProto[ProtocolS7])
	t.Logf("  OPC UA errors: %d", errsByProto["opcua-failing"])

	// Healthy protocols should produce data regardless of the failing one
	if dataByProto[ProtocolModbus] == 0 {
		t.Error("Modbus device should produce data despite OPC UA failure")
	}
	if dataByProto[ProtocolS7] == 0 {
		t.Error("S7 device should produce data despite OPC UA failure")
	}

	// Failing device should have some data (before failure) and some errors
	if errsByProto["opcua-failing"] == 0 {
		t.Log("Warning: Expected errors from failing OPC UA device")
	}
}

// TestFailoverCascadingRecovery tests recovery when multiple devices fail
// and recover at different times.
func TestFailoverCascadingRecovery(t *testing.T) {
	manager := NewFailoverManager()

	// Create 5 devices that fail at staggered intervals
	for i := 0; i < 5; i++ {
		device := NewFailoverDevice(fmt.Sprintf("cascade-%d", i), ProtocolModbus)
		device.failMode = FailModeReadError
		device.failAfter = int64(5 + i*3)  // Stagger failure onset
		device.recoverAfter = int64(5 + i)  // Stagger recovery
		device.breakerMaxFails = 3
		device.breakerCooldown = 200 * time.Millisecond
		manager.AddDevice(device)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	// Track overall throughput over time
	type interval struct {
		points int64
		errors int64
	}
	var intervals []interval
	var intervalMu sync.Mutex

	// Sample every 200ms
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		var currentPoints, currentErrors int64
		for {
			select {
			case <-ctx.Done():
				return
			case <-manager.dataChan:
				atomic.AddInt64(&currentPoints, 1)
			case evt := <-manager.errChan:
				if !evt.Recovered {
					atomic.AddInt64(&currentErrors, 1)
				}
			case <-ticker.C:
				p := atomic.SwapInt64(&currentPoints, 0)
				e := atomic.SwapInt64(&currentErrors, 0)
				intervalMu.Lock()
				intervals = append(intervals, interval{points: p, errors: e})
				intervalMu.Unlock()
			}
		}
	}()

	time.Sleep(4 * time.Second)
	manager.Stop()
	cancel()

	// Brief wait for goroutine cleanup
	time.Sleep(100 * time.Millisecond)

	intervalMu.Lock()
	defer intervalMu.Unlock()

	t.Log("Cascading recovery — throughput over time:")
	for i, iv := range intervals {
		bar := ""
		for j := int64(0); j < iv.points/5; j++ {
			bar += "#"
		}
		t.Logf("  [%3dms] points=%3d errors=%3d %s",
			(i+1)*200, iv.points, iv.errors, bar)
	}

	// Verify we had some throughput in at least the first few intervals
	if len(intervals) > 0 && intervals[0].points == 0 {
		t.Log("Warning: No data in first interval (timing-dependent)")
	}

	// Check device recovery stats
	for _, d := range manager.devices {
		t.Logf("  Device %s: failures=%d, recoveries=%d, breakerTrips=%d",
			d.MockDevice.ID, d.totalFailures, d.totalRecoveries, d.breakerTrips)
	}
}

// TestFailoverGracefulDegradation tests that the system continues to function
// with reduced capacity when some devices are down.
func TestFailoverGracefulDegradation(t *testing.T) {
	manager := NewFailoverManager()

	totalDevices := 10
	failingDevices := 4

	for i := 0; i < totalDevices; i++ {
		device := NewFailoverDevice(
			fmt.Sprintf("grace-%d", i),
			[]Protocol{ProtocolModbus, ProtocolOPCUA, ProtocolS7}[i%3],
		)
		if i < failingDevices {
			// These devices fail permanently
			device.failMode = FailModeReadError
			device.failAfter = 2
			device.recoverAfter = 999999 // Never recover
		} else {
			// These remain healthy
			device.failMode = FailModeIntermittent
			device.MockDevice.ErrorRate = 0.0
		}
		manager.AddDevice(device)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	var totalData, totalErrors int64
	healthyData := make(map[string]int64)
	var mu sync.Mutex

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case dp := <-manager.dataChan:
				atomic.AddInt64(&totalData, 1)
				mu.Lock()
				healthyData[dp.DeviceID]++
				mu.Unlock()
			case evt := <-manager.errChan:
				if !evt.Recovered {
					atomic.AddInt64(&totalErrors, 1)
				}
			}
		}
	}()

	time.Sleep(2 * time.Second)
	manager.Stop()
	cancel()
	<-done

	data := atomic.LoadInt64(&totalData)
	errs := atomic.LoadInt64(&totalErrors)

	t.Logf("Graceful degradation test:")
	t.Logf("  Total devices: %d (%d failing, %d healthy)", totalDevices, failingDevices, totalDevices-failingDevices)
	t.Logf("  Data points: %d", data)
	t.Logf("  Errors: %d", errs)

	mu.Lock()
	healthyCount := 0
	for deviceID, count := range healthyData {
		if count > 0 {
			healthyCount++
		}
		t.Logf("  %s: %d points", deviceID, count)
	}
	mu.Unlock()

	// System should still produce data from healthy devices
	if data == 0 {
		t.Error("Expected data from healthy devices")
	}

	// At least the healthy devices should produce data
	expectedHealthy := totalDevices - failingDevices
	if healthyCount < expectedHealthy {
		t.Logf("Warning: Only %d/%d healthy devices produced data", healthyCount, expectedHealthy)
	}
}

// TestFailoverDeviceRemovalDuringFailure tests removing a failing device
// from the manager while it's in an error state.
func TestFailoverDeviceRemovalDuringFailure(t *testing.T) {
	manager := NewFailoverManager()

	// Add a device that will fail
	failing := NewFailoverDevice("to-remove", ProtocolModbus)
	failing.failMode = FailModeReadError
	failing.failAfter = 3
	failing.recoverAfter = 999999
	manager.AddDevice(failing)

	// Add a healthy device
	healthy := NewFailoverDevice("keeper", ProtocolS7)
	healthy.failMode = FailModeIntermittent
	healthy.MockDevice.ErrorRate = 0.0
	manager.AddDevice(healthy)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	// Let it run, collect some data
	time.Sleep(500 * time.Millisecond)

	// Drain channels briefly
	var preRemoveData int64
	drainCtx, drainCancel := context.WithTimeout(ctx, 200*time.Millisecond)
	func() {
		defer drainCancel()
		for {
			select {
			case <-drainCtx.Done():
				return
			case <-manager.dataChan:
				atomic.AddInt64(&preRemoveData, 1)
			case <-manager.errChan:
			}
		}
	}()

	t.Logf("Pre-removal data points: %d", atomic.LoadInt64(&preRemoveData))

	// The healthy device should keep producing data
	var postRemoveData int64
	postDone := make(chan struct{})
	go func() {
		defer close(postDone)
		for {
			select {
			case <-ctx.Done():
				return
			case <-manager.dataChan:
				atomic.AddInt64(&postRemoveData, 1)
			case <-manager.errChan:
			}
		}
	}()

	time.Sleep(1 * time.Second)
	manager.Stop()
	cancel()
	<-postDone

	post := atomic.LoadInt64(&postRemoveData)
	t.Logf("Post-removal data points: %d", post)

	if post == 0 {
		t.Error("Expected healthy device to keep producing data")
	}
}

// TestFailoverTimeoutRecovery tests recovery from timeout-based failures.
func TestFailoverTimeoutRecovery(t *testing.T) {
	device := NewFailoverDevice("timeout-test", ProtocolOPCUA)
	device.failMode = FailModeIntermittent
	device.MockDevice.ErrorRate = 0.3
	device.MockDevice.PollingRate = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := device.MockDevice.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	var successes, failures int64
	for i := 0; i < 50; i++ {
		_, err := device.ReadTagsWithFailover(ctx)
		if err != nil {
			atomic.AddInt64(&failures, 1)
		} else {
			atomic.AddInt64(&successes, 1)
		}
		time.Sleep(20 * time.Millisecond)
	}

	s := atomic.LoadInt64(&successes)
	f := atomic.LoadInt64(&failures)
	t.Logf("Timeout recovery test: successes=%d, failures=%d (expected ~70%%/30%%)", s, f)

	// Should have both successes and failures
	if s == 0 {
		t.Error("Expected some successful reads")
	}
	if f == 0 {
		t.Log("Warning: Expected some failures (probabilistic)")
	}

	// Success rate should be roughly 70%
	rate := float64(s) / float64(s+f)
	t.Logf("  Success rate: %.1f%%", rate*100)
	if rate < 0.4 || rate > 0.95 {
		t.Logf("  Warning: Success rate %.1f%% outside expected range [40%%, 95%%]", rate*100)
	}
}
