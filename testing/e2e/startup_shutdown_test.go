//go:build e2e
// +build e2e

// Package e2e provides end-to-end tests for gateway startup/shutdown.
package e2e

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Gateway Simulator for E2E Tests
// =============================================================================

// GatewayState represents the state of the gateway.
type GatewayState int

const (
	StateUninitialized GatewayState = iota
	StateStarting
	StateRunning
	StateShuttingDown
	StateStopped
)

func (s GatewayState) String() string {
	switch s {
	case StateUninitialized:
		return "uninitialized"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateShuttingDown:
		return "shutting_down"
	case StateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// MockGateway simulates the gateway for E2E testing.
type MockGateway struct {
	state        atomic.Int32
	httpServer   *http.Server
	mqttClient   *MockMQTTClient
	devices      []*MockDevice
	mu           sync.RWMutex
	startupTime  time.Duration
	shutdownTime time.Duration
	errors       []error
}

// MockMQTTClient simulates MQTT client.
type MockMQTTClient struct {
	connected atomic.Bool
	messages  atomic.Int64
}

// MockDevice simulates a polling device.
type MockDevice struct {
	name     string
	protocol string
	running  atomic.Bool
	polls    atomic.Int64
}

// NewMockGateway creates a new mock gateway.
func NewMockGateway(deviceCount int, startupDelay, shutdownDelay time.Duration) *MockGateway {
	gw := &MockGateway{
		mqttClient:   &MockMQTTClient{},
		startupTime:  startupDelay,
		shutdownTime: shutdownDelay,
	}

	for i := 0; i < deviceCount; i++ {
		gw.devices = append(gw.devices, &MockDevice{
			name:     "device" + string(rune('1'+i)),
			protocol: "modbus",
		})
	}

	return gw
}

// Start starts the gateway.
func (gw *MockGateway) Start(ctx context.Context) error {
	gw.state.Store(int32(StateStarting))

	// Simulate startup delay
	select {
	case <-time.After(gw.startupTime):
	case <-ctx.Done():
		gw.state.Store(int32(StateStopped))
		return ctx.Err()
	}

	// Start HTTP server (simulated)
	gw.httpServer = &http.Server{
		Addr: ":0", // Random port
	}

	// Connect MQTT
	gw.mqttClient.connected.Store(true)

	// Start device polling
	for _, dev := range gw.devices {
		dev.running.Store(true)
	}

	gw.state.Store(int32(StateRunning))
	return nil
}

// Stop stops the gateway gracefully.
func (gw *MockGateway) Stop(ctx context.Context) error {
	gw.state.Store(int32(StateShuttingDown))

	// Stop device polling
	for _, dev := range gw.devices {
		dev.running.Store(false)
	}

	// Disconnect MQTT
	gw.mqttClient.connected.Store(false)

	// Simulate shutdown delay
	select {
	case <-time.After(gw.shutdownTime):
	case <-ctx.Done():
		gw.state.Store(int32(StateStopped))
		return ctx.Err()
	}

	gw.state.Store(int32(StateStopped))
	return nil
}

// State returns the current gateway state.
func (gw *MockGateway) State() GatewayState {
	return GatewayState(gw.state.Load())
}

// =============================================================================
// Startup Tests
// =============================================================================

// TestGatewayStartup_Basic tests basic gateway startup.
func TestGatewayStartup_Basic(t *testing.T) {
	gw := NewMockGateway(3, 50*time.Millisecond, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if gw.State() != StateUninitialized {
		t.Errorf("expected uninitialized state, got %s", gw.State())
	}

	err := gw.Start(ctx)
	if err != nil {
		t.Fatalf("startup failed: %v", err)
	}

	if gw.State() != StateRunning {
		t.Errorf("expected running state, got %s", gw.State())
	}

	// Verify components are running
	if !gw.mqttClient.connected.Load() {
		t.Error("MQTT client not connected")
	}

	for _, dev := range gw.devices {
		if !dev.running.Load() {
			t.Errorf("device %s not running", dev.name)
		}
	}
}

// TestGatewayStartup_ContextCancellation tests startup cancellation.
func TestGatewayStartup_ContextCancellation(t *testing.T) {
	// Gateway with long startup time
	gw := NewMockGateway(1, 5*time.Second, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := gw.Start(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}

	// Gateway should be stopped after cancellation
	if gw.State() != StateStopped {
		t.Errorf("expected stopped state after cancellation, got %s", gw.State())
	}
}

// TestGatewayStartup_StateTransitions tests state transitions during startup.
func TestGatewayStartup_StateTransitions(t *testing.T) {
	gw := NewMockGateway(1, 100*time.Millisecond, 50*time.Millisecond)

	states := make([]GatewayState, 0)
	var mu sync.Mutex

	// Monitor state changes with faster polling
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				mu.Lock()
				currentState := gw.State()
				if len(states) == 0 || states[len(states)-1] != currentState {
					states = append(states, currentState)
				}
				mu.Unlock()
				time.Sleep(5 * time.Millisecond) // Faster polling
			}
		}
	}()

	ctx := context.Background()
	_ = gw.Start(ctx)

	// Capture final state after Start returns
	mu.Lock()
	finalState := gw.State()
	if len(states) == 0 || states[len(states)-1] != finalState {
		states = append(states, finalState)
	}
	mu.Unlock()

	close(done)
	time.Sleep(10 * time.Millisecond)

	// Verify state transitions
	mu.Lock()
	defer mu.Unlock()

	// Should transition through Starting to Running
	hasStarting := false
	hasRunning := false
	for _, s := range states {
		if s == StateStarting {
			hasStarting = true
		}
		if s == StateRunning {
			hasRunning = true
		}
	}

	if !hasStarting {
		t.Error("missed StateStarting transition")
	}
	if !hasRunning {
		t.Errorf("missed StateRunning transition, captured states: %v", states)
	}
}

// =============================================================================
// Shutdown Tests
// =============================================================================

// TestGatewayShutdown_Basic tests basic gateway shutdown.
func TestGatewayShutdown_Basic(t *testing.T) {
	gw := NewMockGateway(3, 50*time.Millisecond, 50*time.Millisecond)

	ctx := context.Background()
	if err := gw.Start(ctx); err != nil {
		t.Fatalf("startup failed: %v", err)
	}

	if err := gw.Stop(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	if gw.State() != StateStopped {
		t.Errorf("expected stopped state, got %s", gw.State())
	}

	// Verify components are stopped
	if gw.mqttClient.connected.Load() {
		t.Error("MQTT client still connected")
	}

	for _, dev := range gw.devices {
		if dev.running.Load() {
			t.Errorf("device %s still running", dev.name)
		}
	}
}

// TestGatewayShutdown_GracefulTimeout tests graceful shutdown with timeout.
func TestGatewayShutdown_GracefulTimeout(t *testing.T) {
	// Gateway with long shutdown time
	gw := NewMockGateway(1, 50*time.Millisecond, 5*time.Second)

	ctx := context.Background()
	if err := gw.Start(ctx); err != nil {
		t.Fatalf("startup failed: %v", err)
	}

	// Short timeout for shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := gw.Stop(shutdownCtx)
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}

	// Should be stopped even with forced shutdown
	if gw.State() != StateStopped {
		t.Errorf("expected stopped state, got %s", gw.State())
	}
}

// TestGatewayShutdown_StateTransitions tests state transitions during shutdown.
func TestGatewayShutdown_StateTransitions(t *testing.T) {
	gw := NewMockGateway(1, 50*time.Millisecond, 100*time.Millisecond)

	ctx := context.Background()
	if err := gw.Start(ctx); err != nil {
		t.Fatalf("startup failed: %v", err)
	}

	states := make([]GatewayState, 0)
	var mu sync.Mutex

	// Monitor state changes
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				mu.Lock()
				currentState := gw.State()
				if len(states) == 0 || states[len(states)-1] != currentState {
					states = append(states, currentState)
				}
				mu.Unlock()
				time.Sleep(5 * time.Millisecond) // Faster polling
			}
		}
	}()

	_ = gw.Stop(ctx)

	// Capture final state after Stop returns
	mu.Lock()
	finalState := gw.State()
	if len(states) == 0 || states[len(states)-1] != finalState {
		states = append(states, finalState)
	}
	mu.Unlock()

	close(done)
	time.Sleep(10 * time.Millisecond)

	// Verify state transitions
	mu.Lock()
	defer mu.Unlock()

	// Should transition through ShuttingDown to Stopped
	hasShuttingDown := false
	hasStopped := false
	for _, s := range states {
		if s == StateShuttingDown {
			hasShuttingDown = true
		}
		if s == StateStopped {
			hasStopped = true
		}
	}

	if !hasShuttingDown {
		t.Error("missed StateShuttingDown transition")
	}
	if !hasStopped {
		t.Errorf("missed StateStopped transition, captured states: %v", states)
	}
}

// =============================================================================
// Full Lifecycle Tests
// =============================================================================

// TestGatewayLifecycle_StartStopRestart tests full lifecycle.
func TestGatewayLifecycle_StartStopRestart(t *testing.T) {
	gw := NewMockGateway(2, 50*time.Millisecond, 50*time.Millisecond)
	ctx := context.Background()

	// First start
	if err := gw.Start(ctx); err != nil {
		t.Fatalf("first start failed: %v", err)
	}
	if gw.State() != StateRunning {
		t.Errorf("expected running after first start, got %s", gw.State())
	}

	// First stop
	if err := gw.Stop(ctx); err != nil {
		t.Fatalf("first stop failed: %v", err)
	}
	if gw.State() != StateStopped {
		t.Errorf("expected stopped after first stop, got %s", gw.State())
	}

	// Restart
	if err := gw.Start(ctx); err != nil {
		t.Fatalf("restart failed: %v", err)
	}
	if gw.State() != StateRunning {
		t.Errorf("expected running after restart, got %s", gw.State())
	}

	// Final stop
	if err := gw.Stop(ctx); err != nil {
		t.Fatalf("final stop failed: %v", err)
	}
	if gw.State() != StateStopped {
		t.Errorf("expected stopped after final stop, got %s", gw.State())
	}
}

// TestGatewayLifecycle_MultipleRestarts tests multiple restart cycles.
func TestGatewayLifecycle_MultipleRestarts(t *testing.T) {
	gw := NewMockGateway(1, 20*time.Millisecond, 20*time.Millisecond)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := gw.Start(ctx); err != nil {
			t.Fatalf("start %d failed: %v", i, err)
		}

		if gw.State() != StateRunning {
			t.Errorf("cycle %d: expected running, got %s", i, gw.State())
		}

		// Simulate some runtime
		time.Sleep(30 * time.Millisecond)

		if err := gw.Stop(ctx); err != nil {
			t.Fatalf("stop %d failed: %v", i, err)
		}

		if gw.State() != StateStopped {
			t.Errorf("cycle %d: expected stopped, got %s", i, gw.State())
		}
	}
}

// =============================================================================
// Signal Handling Tests
// =============================================================================

// TestGatewaySignal_SIGTERM tests SIGTERM handling simulation.
func TestGatewaySignal_SIGTERM(t *testing.T) {
	gw := NewMockGateway(2, 50*time.Millisecond, 50*time.Millisecond)
	ctx := context.Background()

	if err := gw.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Simulate SIGTERM by calling Stop
	shutdownComplete := make(chan bool)
	go func() {
		_ = gw.Stop(ctx)
		close(shutdownComplete)
	}()

	// Wait for shutdown
	select {
	case <-shutdownComplete:
		// Good
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown timed out")
	}

	if gw.State() != StateStopped {
		t.Errorf("expected stopped state, got %s", gw.State())
	}
}

// TestGatewaySignal_SIGINT tests SIGINT handling simulation (rapid shutdown).
func TestGatewaySignal_SIGINT(t *testing.T) {
	gw := NewMockGateway(2, 50*time.Millisecond, 200*time.Millisecond)
	ctx := context.Background()

	if err := gw.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Simulate SIGINT with short timeout (force stop)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_ = gw.Stop(shutdownCtx)

	// Should be stopped regardless of timeout
	if gw.State() != StateStopped {
		t.Errorf("expected stopped state, got %s", gw.State())
	}
}

// =============================================================================
// Concurrent Operations Tests
// =============================================================================

// TestGateway_ConcurrentStartStop tests concurrent start/stop attempts.
func TestGateway_ConcurrentStartStop(t *testing.T) {
	gw := NewMockGateway(2, 30*time.Millisecond, 30*time.Millisecond)
	ctx := context.Background()

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	// Start the gateway first
	if err := gw.Start(ctx); err != nil {
		t.Fatalf("initial start failed: %v", err)
	}

	// Concurrent operations
	for i := 0; i < 5; i++ {
		wg.Add(2)

		go func() {
			defer wg.Done()
			if err := gw.Stop(ctx); err != nil {
				errors <- err
			}
		}()

		go func() {
			defer wg.Done()
			time.Sleep(10 * time.Millisecond)
			if err := gw.Start(ctx); err != nil {
				// Start might fail if already stopped/stopping
				// This is expected behavior
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for unexpected errors
	for err := range errors {
		if err != nil && err != context.Canceled {
			t.Logf("concurrent operation error: %v", err)
		}
	}

	// Final state should be consistent
	state := gw.State()
	if state != StateRunning && state != StateStopped {
		t.Errorf("gateway in inconsistent state: %s", state)
	}
}

// =============================================================================
// Health Check Integration Tests
// =============================================================================

// MockHealthChecker simulates health checking.
type MockHealthChecker struct {
	healthy atomic.Bool
}

// TestGatewayHealth_DuringStartup tests health during startup.
func TestGatewayHealth_DuringStartup(t *testing.T) {
	gw := NewMockGateway(2, 100*time.Millisecond, 50*time.Millisecond)

	healthCh := make(chan GatewayState, 10)

	go func() {
		for {
			healthCh <- gw.State()
			time.Sleep(20 * time.Millisecond)
			if gw.State() == StateRunning {
				close(healthCh)
				return
			}
		}
	}()

	ctx := context.Background()
	if err := gw.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Check captured states
	hadStarting := false
	for state := range healthCh {
		if state == StateStarting {
			hadStarting = true
		}
	}

	if !hadStarting {
		t.Error("health check missed starting state")
	}
}

// TestGatewayHealth_DuringShutdown tests health during shutdown.
func TestGatewayHealth_DuringShutdown(t *testing.T) {
	gw := NewMockGateway(2, 50*time.Millisecond, 100*time.Millisecond)

	ctx := context.Background()
	if err := gw.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	healthCh := make(chan GatewayState, 10)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-done:
				close(healthCh)
				return
			default:
				healthCh <- gw.State()
				time.Sleep(20 * time.Millisecond)
			}
		}
	}()

	if err := gw.Stop(ctx); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	close(done)

	// Check captured states
	hadShuttingDown := false
	for state := range healthCh {
		if state == StateShuttingDown {
			hadShuttingDown = true
		}
	}

	if !hadShuttingDown {
		t.Error("health check missed shutting down state")
	}
}

// =============================================================================
// Resource Cleanup Tests
// =============================================================================

// TestGatewayCleanup_AllResourcesReleased tests that all resources are cleaned up.
func TestGatewayCleanup_AllResourcesReleased(t *testing.T) {
	gw := NewMockGateway(5, 50*time.Millisecond, 50*time.Millisecond)
	ctx := context.Background()

	if err := gw.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Verify everything is running
	if !gw.mqttClient.connected.Load() {
		t.Error("MQTT not connected before stop")
	}

	for _, dev := range gw.devices {
		if !dev.running.Load() {
			t.Errorf("device %s not running before stop", dev.name)
		}
	}

	// Stop gateway
	if err := gw.Stop(ctx); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	// Verify everything is stopped
	if gw.mqttClient.connected.Load() {
		t.Error("MQTT still connected after stop")
	}

	for _, dev := range gw.devices {
		if dev.running.Load() {
			t.Errorf("device %s still running after stop", dev.name)
		}
	}
}

// TestGatewayCleanup_AfterPanic tests cleanup after simulated panic recovery.
func TestGatewayCleanup_AfterPanic(t *testing.T) {
	gw := NewMockGateway(2, 50*time.Millisecond, 50*time.Millisecond)
	ctx := context.Background()

	if err := gw.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Simulate panic recovery by forcing stop
	recovered := func() {
		defer func() {
			if r := recover(); r != nil {
				// Cleanup on panic
				_ = gw.Stop(ctx)
			}
		}()

		// Simulate panic
		panic("simulated panic")
	}

	recovered()

	// Verify gateway is stopped after panic recovery
	if gw.State() != StateStopped {
		t.Errorf("expected stopped state after panic recovery, got %s", gw.State())
	}
}
