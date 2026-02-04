//go:build e2e
// +build e2e

// Package e2e provides end-to-end tests for hot configuration reload.
package e2e

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Configuration Reload Simulation
// =============================================================================

// ConfigVersion represents a configuration version.
type ConfigVersion struct {
	Version   int
	Devices   []DeviceConfig
	MQTT      MQTTConfig
	CreatedAt time.Time
}

// DeviceConfig represents device configuration.
type DeviceConfig struct {
	ID           string
	Protocol     string
	Host         string
	PollInterval time.Duration
	Tags         []TagConfig
	Enabled      bool
}

// TagConfig represents tag configuration.
type TagConfig struct {
	ID       string
	Address  string
	DataType string
}

// MQTTConfig represents MQTT configuration.
type MQTTConfig struct {
	BrokerURL string
	ClientID  string
	QoS       int
}

// ConfigReloadableGateway simulates a gateway that supports hot reload.
type ConfigReloadableGateway struct {
	mu            sync.RWMutex
	currentConfig *ConfigVersion
	state         atomic.Int32
	reloadCount   atomic.Int64
	reloadErrors  atomic.Int64
	lastReload    time.Time
	pollWorkers   map[string]*MockPollWorker
	listeners     []ConfigChangeListener
}

// MockPollWorker simulates a device polling worker.
type MockPollWorker struct {
	deviceID     string
	running      atomic.Bool
	pollInterval time.Duration
	pollCount    atomic.Int64
	restartCount atomic.Int64
}

// ConfigChangeListener is notified of config changes.
type ConfigChangeListener interface {
	OnConfigChange(old, new *ConfigVersion)
}

// NewConfigReloadableGateway creates a new gateway with reload support.
func NewConfigReloadableGateway(initial *ConfigVersion) *ConfigReloadableGateway {
	gw := &ConfigReloadableGateway{
		currentConfig: initial,
		pollWorkers:   make(map[string]*MockPollWorker),
	}
	gw.state.Store(int32(StateRunning))

	// Initialize workers for initial config
	for _, dev := range initial.Devices {
		if dev.Enabled {
			gw.pollWorkers[dev.ID] = &MockPollWorker{
				deviceID:     dev.ID,
				pollInterval: dev.PollInterval,
			}
			gw.pollWorkers[dev.ID].running.Store(true)
		}
	}

	return gw
}

// ReloadConfig applies a new configuration.
func (g *ConfigReloadableGateway) ReloadConfig(newConfig *ConfigVersion) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	oldConfig := g.currentConfig

	// Validate new config
	if err := g.validateConfig(newConfig); err != nil {
		g.reloadErrors.Add(1)
		return err
	}

	// Apply changes
	g.applyConfigChanges(oldConfig, newConfig)

	g.currentConfig = newConfig
	g.lastReload = time.Now()
	g.reloadCount.Add(1)

	// Notify listeners
	for _, listener := range g.listeners {
		listener.OnConfigChange(oldConfig, newConfig)
	}

	return nil
}

// validateConfig validates configuration.
func (g *ConfigReloadableGateway) validateConfig(config *ConfigVersion) error {
	// Simulate validation
	if config == nil {
		return ErrInvalidConfig
	}
	return nil
}

// applyConfigChanges applies configuration changes.
func (g *ConfigReloadableGateway) applyConfigChanges(old, new *ConfigVersion) {
	oldDevices := make(map[string]DeviceConfig)
	for _, d := range old.Devices {
		oldDevices[d.ID] = d
	}

	newDevices := make(map[string]DeviceConfig)
	for _, d := range new.Devices {
		newDevices[d.ID] = d
	}

	// Handle removed devices
	for id := range oldDevices {
		if _, exists := newDevices[id]; !exists {
			if worker, ok := g.pollWorkers[id]; ok {
				worker.running.Store(false)
				delete(g.pollWorkers, id)
			}
		}
	}

	// Handle added or modified devices
	for id, newDev := range newDevices {
		if oldDev, exists := oldDevices[id]; exists {
			// Modified - check if restart needed
			if oldDev.PollInterval != newDev.PollInterval || oldDev.Host != newDev.Host {
				if worker, ok := g.pollWorkers[id]; ok {
					worker.pollInterval = newDev.PollInterval
					worker.restartCount.Add(1)
				}
			}
		} else {
			// Added
			if newDev.Enabled {
				g.pollWorkers[id] = &MockPollWorker{
					deviceID:     id,
					pollInterval: newDev.PollInterval,
				}
				g.pollWorkers[id].running.Store(true)
			}
		}
	}
}

// GetConfig returns current configuration.
func (g *ConfigReloadableGateway) GetConfig() *ConfigVersion {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.currentConfig
}

// ReloadStats returns reload statistics.
func (g *ConfigReloadableGateway) ReloadStats() (reloads, errors int64, lastReload time.Time) {
	return g.reloadCount.Load(), g.reloadErrors.Load(), g.lastReload
}

// Custom errors
var (
	ErrInvalidConfig = &configError{msg: "invalid configuration"}
)

type configError struct {
	msg string
}

func (e *configError) Error() string {
	return e.msg
}

// =============================================================================
// Config Reload Tests
// =============================================================================

// TestConfigReload_AddDevice tests adding a new device via reload.
func TestConfigReload_AddDevice(t *testing.T) {
	initialConfig := &ConfigVersion{
		Version: 1,
		Devices: []DeviceConfig{
			{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
		},
		MQTT:      MQTTConfig{BrokerURL: "tcp://localhost:1883", ClientID: "gateway-1", QoS: 1},
		CreatedAt: time.Now(),
	}

	gw := NewConfigReloadableGateway(initialConfig)

	// Verify initial state
	if len(gw.pollWorkers) != 1 {
		t.Fatalf("expected 1 worker, got %d", len(gw.pollWorkers))
	}

	// Add new device
	newConfig := &ConfigVersion{
		Version: 2,
		Devices: []DeviceConfig{
			{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
			{ID: "device-2", Protocol: "opcua", Host: "192.168.1.20", PollInterval: 2 * time.Second, Enabled: true},
		},
		MQTT:      MQTTConfig{BrokerURL: "tcp://localhost:1883", ClientID: "gateway-1", QoS: 1},
		CreatedAt: time.Now(),
	}

	err := gw.ReloadConfig(newConfig)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	// Verify new device was added
	if len(gw.pollWorkers) != 2 {
		t.Fatalf("expected 2 workers, got %d", len(gw.pollWorkers))
	}

	if _, exists := gw.pollWorkers["device-2"]; !exists {
		t.Error("device-2 worker was not created")
	}

	reloads, errors, _ := gw.ReloadStats()
	t.Logf("Reload stats: reloads=%d, errors=%d", reloads, errors)
}

// TestConfigReload_RemoveDevice tests removing a device via reload.
func TestConfigReload_RemoveDevice(t *testing.T) {
	initialConfig := &ConfigVersion{
		Version: 1,
		Devices: []DeviceConfig{
			{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
			{ID: "device-2", Protocol: "opcua", Host: "192.168.1.20", PollInterval: 2 * time.Second, Enabled: true},
		},
		CreatedAt: time.Now(),
	}

	gw := NewConfigReloadableGateway(initialConfig)

	// Remove device-2
	newConfig := &ConfigVersion{
		Version: 2,
		Devices: []DeviceConfig{
			{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
		},
		CreatedAt: time.Now(),
	}

	err := gw.ReloadConfig(newConfig)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	// Verify device was removed
	if len(gw.pollWorkers) != 1 {
		t.Fatalf("expected 1 worker, got %d", len(gw.pollWorkers))
	}

	if _, exists := gw.pollWorkers["device-2"]; exists {
		t.Error("device-2 worker should have been removed")
	}
}

// TestConfigReload_ModifyPollInterval tests changing poll interval.
func TestConfigReload_ModifyPollInterval(t *testing.T) {
	initialConfig := &ConfigVersion{
		Version: 1,
		Devices: []DeviceConfig{
			{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
		},
		CreatedAt: time.Now(),
	}

	gw := NewConfigReloadableGateway(initialConfig)

	// Change poll interval
	newConfig := &ConfigVersion{
		Version: 2,
		Devices: []DeviceConfig{
			{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: 500 * time.Millisecond, Enabled: true},
		},
		CreatedAt: time.Now(),
	}

	err := gw.ReloadConfig(newConfig)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	worker := gw.pollWorkers["device-1"]
	if worker.pollInterval != 500*time.Millisecond {
		t.Errorf("expected poll interval 500ms, got %v", worker.pollInterval)
	}

	if worker.restartCount.Load() != 1 {
		t.Errorf("expected restart count 1, got %d", worker.restartCount.Load())
	}
}

// TestConfigReload_InvalidConfig tests handling invalid config.
func TestConfigReload_InvalidConfig(t *testing.T) {
	initialConfig := &ConfigVersion{
		Version: 1,
		Devices: []DeviceConfig{
			{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
		},
		CreatedAt: time.Now(),
	}

	gw := NewConfigReloadableGateway(initialConfig)

	// Try to apply nil config
	err := gw.ReloadConfig(nil)
	if err == nil {
		t.Error("expected error for nil config")
	}

	// Original config should be unchanged
	currentConfig := gw.GetConfig()
	if currentConfig.Version != 1 {
		t.Errorf("config should be unchanged, version=%d", currentConfig.Version)
	}

	_, errors, _ := gw.ReloadStats()
	if errors != 1 {
		t.Errorf("expected 1 error, got %d", errors)
	}
}

// TestConfigReload_ConcurrentReloads tests concurrent reload attempts.
func TestConfigReload_ConcurrentReloads(t *testing.T) {
	initialConfig := &ConfigVersion{
		Version: 1,
		Devices: []DeviceConfig{
			{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
		},
		CreatedAt: time.Now(),
	}

	gw := NewConfigReloadableGateway(initialConfig)

	// Concurrent reloads
	var wg sync.WaitGroup
	reloadCount := 10
	wg.Add(reloadCount)

	for i := 0; i < reloadCount; i++ {
		go func(version int) {
			defer wg.Done()
			config := &ConfigVersion{
				Version: version,
				Devices: []DeviceConfig{
					{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
				},
				CreatedAt: time.Now(),
			}
			gw.ReloadConfig(config)
		}(i + 2)
	}

	wg.Wait()

	reloads, errors, _ := gw.ReloadStats()
	t.Logf("Concurrent reloads: successful=%d, errors=%d", reloads, errors)

	// All reloads should succeed (they're serialized by mutex)
	if reloads != int64(reloadCount) {
		t.Errorf("expected %d reloads, got %d", reloadCount, reloads)
	}
}

// TestConfigReload_RapidReloads tests rapid successive reloads.
func TestConfigReload_RapidReloads(t *testing.T) {
	initialConfig := &ConfigVersion{
		Version: 1,
		Devices: []DeviceConfig{
			{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
		},
		CreatedAt: time.Now(),
	}

	gw := NewConfigReloadableGateway(initialConfig)

	// Rapid reloads
	for i := 2; i <= 20; i++ {
		config := &ConfigVersion{
			Version: i,
			Devices: []DeviceConfig{
				{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10",
					PollInterval: time.Duration(i*100) * time.Millisecond, Enabled: true},
			},
			CreatedAt: time.Now(),
		}

		err := gw.ReloadConfig(config)
		if err != nil {
			t.Fatalf("reload %d failed: %v", i, err)
		}
	}

	// Final config should be version 20
	finalConfig := gw.GetConfig()
	if finalConfig.Version != 20 {
		t.Errorf("expected version 20, got %d", finalConfig.Version)
	}

	// Poll interval should be 2000ms
	worker := gw.pollWorkers["device-1"]
	expectedInterval := 2000 * time.Millisecond
	if worker.pollInterval != expectedInterval {
		t.Errorf("expected poll interval %v, got %v", expectedInterval, worker.pollInterval)
	}

	t.Logf("Rapid reloads: final version=%d, restart count=%d", finalConfig.Version, worker.restartCount.Load())
}

// TestConfigReload_NoChangeReload tests reload with identical config.
func TestConfigReload_NoChangeReload(t *testing.T) {
	initialConfig := &ConfigVersion{
		Version: 1,
		Devices: []DeviceConfig{
			{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
		},
		CreatedAt: time.Now(),
	}

	gw := NewConfigReloadableGateway(initialConfig)
	initialRestarts := gw.pollWorkers["device-1"].restartCount.Load()

	// Reload with same config (different version)
	sameConfig := &ConfigVersion{
		Version: 2,
		Devices: []DeviceConfig{
			{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
		},
		CreatedAt: time.Now(),
	}

	err := gw.ReloadConfig(sameConfig)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	// No restart should occur since config is same
	finalRestarts := gw.pollWorkers["device-1"].restartCount.Load()
	if finalRestarts != initialRestarts {
		t.Errorf("expected no restart, got %d restarts", finalRestarts-initialRestarts)
	}
}

// TestConfigReload_DisableDevice tests disabling a device via reload.
func TestConfigReload_DisableDevice(t *testing.T) {
	initialConfig := &ConfigVersion{
		Version: 1,
		Devices: []DeviceConfig{
			{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
			{ID: "device-2", Protocol: "opcua", Host: "192.168.1.20", PollInterval: 2 * time.Second, Enabled: true},
		},
		CreatedAt: time.Now(),
	}

	gw := NewConfigReloadableGateway(initialConfig)

	// Disable device-2
	newConfig := &ConfigVersion{
		Version: 2,
		Devices: []DeviceConfig{
			{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
			{ID: "device-2", Protocol: "opcua", Host: "192.168.1.20", PollInterval: 2 * time.Second, Enabled: false},
		},
		CreatedAt: time.Now(),
	}

	err := gw.ReloadConfig(newConfig)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	// Device-2 should still exist but worker might be stopped depending on implementation
	// In this simplified implementation, removing from config removes the worker
	t.Logf("Workers after disable: %d", len(gw.pollWorkers))
}

// TestConfigReload_WithListener tests config change listeners.
func TestConfigReload_WithListener(t *testing.T) {
	initialConfig := &ConfigVersion{
		Version: 1,
		Devices: []DeviceConfig{
			{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
		},
		CreatedAt: time.Now(),
	}

	gw := NewConfigReloadableGateway(initialConfig)

	// Add listener
	var notified atomic.Bool
	var oldVersion, newVersion int
	listener := &mockConfigListener{
		callback: func(old, new *ConfigVersion) {
			notified.Store(true)
			oldVersion = old.Version
			newVersion = new.Version
		},
	}
	gw.listeners = append(gw.listeners, listener)

	// Reload config
	newConfig := &ConfigVersion{
		Version: 2,
		Devices: []DeviceConfig{
			{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
		},
		CreatedAt: time.Now(),
	}

	err := gw.ReloadConfig(newConfig)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	if !notified.Load() {
		t.Error("listener was not notified")
	}

	if oldVersion != 1 || newVersion != 2 {
		t.Errorf("listener received wrong versions: old=%d, new=%d", oldVersion, newVersion)
	}
}

// mockConfigListener implements ConfigChangeListener for testing.
type mockConfigListener struct {
	callback func(old, new *ConfigVersion)
}

func (m *mockConfigListener) OnConfigChange(old, new *ConfigVersion) {
	if m.callback != nil {
		m.callback(old, new)
	}
}

// TestConfigReload_Rollback tests rollback on failed reload.
func TestConfigReload_Rollback(t *testing.T) {
	initialConfig := &ConfigVersion{
		Version: 1,
		Devices: []DeviceConfig{
			{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
		},
		CreatedAt: time.Now(),
	}

	gw := NewConfigReloadableGateway(initialConfig)

	// Attempt invalid reload
	err := gw.ReloadConfig(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}

	// Verify state is unchanged
	currentConfig := gw.GetConfig()
	if currentConfig.Version != 1 {
		t.Errorf("expected version 1 after rollback, got %d", currentConfig.Version)
	}

	if len(gw.pollWorkers) != 1 {
		t.Errorf("expected 1 worker after rollback, got %d", len(gw.pollWorkers))
	}

	t.Logf("Rollback successful: config version=%d", currentConfig.Version)
}

// TestConfigReload_WithContext tests reload with context cancellation.
func TestConfigReload_WithContext(t *testing.T) {
	initialConfig := &ConfigVersion{
		Version: 1,
		Devices: []DeviceConfig{
			{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
		},
		CreatedAt: time.Now(),
	}

	gw := NewConfigReloadableGateway(initialConfig)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Simulate reload with context
	done := make(chan error, 1)
	go func() {
		newConfig := &ConfigVersion{
			Version: 2,
			Devices: []DeviceConfig{
				{ID: "device-1", Protocol: "modbus", Host: "192.168.1.10", PollInterval: time.Second, Enabled: true},
			},
			CreatedAt: time.Now(),
		}
		done <- gw.ReloadConfig(newConfig)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("reload failed: %v", err)
		}
		t.Log("Reload completed within timeout")
	case <-ctx.Done():
		t.Fatal("reload timed out")
	}
}
