// Package api provides HTTP handlers for the web UI.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/config"
	"github.com/nexus-edge/protocol-gateway/internal/adapter/mqtt"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/rs/zerolog"
)

func normalizeDeviceTopics(device *domain.Device) {
	if device == nil {
		return
	}
	used := make(map[string]bool, len(device.Tags))
	for i := range device.Tags {
		t := &device.Tags[i]
		suffix := strings.TrimSpace(t.TopicSuffix)
		if suffix == "" {
			suffix = t.Name
		}
		if strings.TrimSpace(suffix) == "" {
			suffix = t.ID
		}
		if strings.TrimSpace(suffix) == "" {
			suffix = fmt.Sprintf("tag_%d", i+1)
		}

		suffix = strings.TrimSpace(suffix)
		suffix = strings.ReplaceAll(suffix, "/", "_")
		suffix = strings.ReplaceAll(suffix, "#", "_")
		suffix = strings.ReplaceAll(suffix, "+", "_")
		suffix = strings.ReplaceAll(suffix, " ", "_")
		suffix = strings.Trim(suffix, "_")

		// Ensure uniqueness to avoid multiple tags publishing to the same topic.
		base := suffix
		for used[suffix] {
			suffix = fmt.Sprintf("%s_%d", base, i+1)
		}
		used[suffix] = true
		t.TopicSuffix = suffix
	}
}

// DeviceManager handles device CRUD operations and persistence.
type DeviceManager struct {
	devices      map[string]*domain.Device
	devicesPath  string
	mu           sync.RWMutex
	logger       zerolog.Logger
	onDeviceAdd  func(*domain.Device) error
	onDeviceEdit func(*domain.Device) error
	onDeviceDel  func(string) error
}

// NewDeviceManager creates a new device manager.
func NewDeviceManager(devicesPath string, logger zerolog.Logger) *DeviceManager {
	return &DeviceManager{
		devices:     make(map[string]*domain.Device),
		devicesPath: devicesPath,
		logger:      logger,
	}
}

// LoadDevices loads devices from the configuration file.
func (dm *DeviceManager) LoadDevices() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	devices, err := config.LoadDevices(dm.devicesPath)
	if err != nil {
		return err
	}

	dm.devices = make(map[string]*domain.Device)
	for _, device := range devices {
		normalizeDeviceTopics(device)
		dm.devices[device.ID] = device
	}

	return nil
}

// SaveDevices persists all devices to the configuration file.
func (dm *DeviceManager) SaveDevices() error {
	dm.mu.RLock()
	devices := make([]*domain.Device, 0, len(dm.devices))
	for _, device := range dm.devices {
		devices = append(devices, device)
	}
	dm.mu.RUnlock()

	return config.SaveDevices(dm.devicesPath, devices)
}

// GetDevices returns all devices.
func (dm *DeviceManager) GetDevices() []*domain.Device {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	devices := make([]*domain.Device, 0, len(dm.devices))
	for _, device := range dm.devices {
		devices = append(devices, device)
	}
	return devices
}

// GetDevice returns a specific device by ID.
func (dm *DeviceManager) GetDevice(id string) (*domain.Device, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	device, ok := dm.devices[id]
	return device, ok
}

// AddDevice adds a new device.
func (dm *DeviceManager) AddDevice(device *domain.Device) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if _, exists := dm.devices[device.ID]; exists {
		return fmt.Errorf("device with ID %s already exists", device.ID)
	}

	normalizeDeviceTopics(device)

	device.CreatedAt = time.Now()
	device.UpdatedAt = time.Now()

	if err := device.Validate(); err != nil {
		return fmt.Errorf("invalid device: %w", err)
	}

	dm.devices[device.ID] = device

	// Save to file
	if err := dm.saveDevicesUnlocked(); err != nil {
		delete(dm.devices, device.ID)
		return fmt.Errorf("failed to persist device: %w", err)
	}

	// Notify callback
	if dm.onDeviceAdd != nil {
		if err := dm.onDeviceAdd(device); err != nil {
			dm.logger.Warn().Err(err).Str("device", device.ID).Msg("Failed to register device with polling service")
		}
	}

	return nil
}

// UpdateDevice updates an existing device.
func (dm *DeviceManager) UpdateDevice(device *domain.Device) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if _, exists := dm.devices[device.ID]; !exists {
		return fmt.Errorf("device with ID %s not found", device.ID)
	}

	normalizeDeviceTopics(device)

	device.UpdatedAt = time.Now()

	if err := device.Validate(); err != nil {
		return fmt.Errorf("invalid device: %w", err)
	}

	oldDevice := dm.devices[device.ID]
	dm.devices[device.ID] = device

	// Save to file
	if err := dm.saveDevicesUnlocked(); err != nil {
		dm.devices[device.ID] = oldDevice
		return fmt.Errorf("failed to persist device: %w", err)
	}

	// Notify callback
	if dm.onDeviceEdit != nil {
		if err := dm.onDeviceEdit(device); err != nil {
			dm.logger.Warn().Err(err).Str("device", device.ID).Msg("Failed to update device in polling service")
		}
	}

	return nil
}

// DeleteDevice removes a device.
func (dm *DeviceManager) DeleteDevice(id string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	device, exists := dm.devices[id]
	if !exists {
		return fmt.Errorf("device with ID %s not found", id)
	}

	delete(dm.devices, id)

	// Save to file
	if err := dm.saveDevicesUnlocked(); err != nil {
		dm.devices[id] = device
		return fmt.Errorf("failed to persist device deletion: %w", err)
	}

	// Notify callback
	if dm.onDeviceDel != nil {
		if err := dm.onDeviceDel(id); err != nil {
			dm.logger.Warn().Err(err).Str("device", id).Msg("Failed to unregister device from polling service")
		}
	}

	return nil
}

// SetCallbacks sets the callbacks for device lifecycle events.
func (dm *DeviceManager) SetCallbacks(onAdd, onEdit func(*domain.Device) error, onDel func(string) error) {
	dm.onDeviceAdd = onAdd
	dm.onDeviceEdit = onEdit
	dm.onDeviceDel = onDel
}

// saveDevicesUnlocked saves devices without locking (internal use only).
func (dm *DeviceManager) saveDevicesUnlocked() error {
	devices := make([]*domain.Device, 0, len(dm.devices))
	for _, device := range dm.devices {
		devices = append(devices, device)
	}
	return config.SaveDevices(dm.devicesPath, devices)
}

// APIHandler provides HTTP handlers for the web UI.
type APIHandler struct {
	deviceManager *DeviceManager
	logger        zerolog.Logger
	topicTracker  TopicTracker
	subscriptions SubscriptionProvider
	logProvider   LogProvider
}

// NewAPIHandler creates a new API handler.
func NewAPIHandler(deviceManager *DeviceManager, logger zerolog.Logger) *APIHandler {
	return &APIHandler{
		deviceManager: deviceManager,
		logger:        logger,
	}
}

// TopicTracker provides a runtime view of recently published topics.
// Implemented by the MQTT publisher.
type TopicTracker interface {
	ActiveTopics(limit int) []mqtt.TopicStat
}

// SubscriptionProvider provides the MQTT subscription patterns used by the gateway.
// Implemented by the command handler.
type SubscriptionProvider interface {
	SubscribedTopics() []string
}

// SetTopicTracker wires in a runtime topic tracker (optional).
func (h *APIHandler) SetTopicTracker(tracker TopicTracker) {
	h.topicTracker = tracker
}

// SetSubscriptionProvider wires in a subscription provider (optional).
func (h *APIHandler) SetSubscriptionProvider(provider SubscriptionProvider) {
	h.subscriptions = provider
}

// SetLogProvider wires in a container log provider (optional).
func (h *APIHandler) SetLogProvider(provider LogProvider) {
	h.logProvider = provider
}

// GetDevicesHandler returns all devices.
func (h *APIHandler) GetDevicesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	devices := h.deviceManager.GetDevices()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(devices); err != nil {
		h.logger.Error().Err(err).Msg("Failed to encode devices")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// GetDeviceHandler returns a specific device.
func (h *APIHandler) GetDeviceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Device ID is required", http.StatusBadRequest)
		return
	}

	device, ok := h.deviceManager.GetDevice(id)
	if !ok {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(device); err != nil {
		h.logger.Error().Err(err).Msg("Failed to encode device")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// CreateDeviceHandler creates a new device.
func (h *APIHandler) CreateDeviceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var device domain.Device
	if err := json.NewDecoder(r.Body).Decode(&device); err != nil {
		h.logger.Error().Err(err).Msg("Failed to decode device")
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.deviceManager.AddDevice(&device); err != nil {
		h.logger.Error().Err(err).Str("device", device.ID).Msg("Failed to add device")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Device created"})
}

// UpdateDeviceHandler updates an existing device.
func (h *APIHandler) UpdateDeviceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var device domain.Device
	if err := json.NewDecoder(r.Body).Decode(&device); err != nil {
		h.logger.Error().Err(err).Msg("Failed to decode device")
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.deviceManager.UpdateDevice(&device); err != nil {
		h.logger.Error().Err(err).Str("device", device.ID).Msg("Failed to update device")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Device updated"})
}

// DeleteDeviceHandler deletes a device.
func (h *APIHandler) DeleteDeviceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Device ID is required", http.StatusBadRequest)
		return
	}

	if err := h.deviceManager.DeleteDevice(id); err != nil {
		h.logger.Error().Err(err).Str("device", id).Msg("Failed to delete device")
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Device deleted"})
}

// TestConnectionHandler tests a device connection without saving.
func (h *APIHandler) TestConnectionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var device domain.Device
	if err := json.NewDecoder(r.Body).Decode(&device); err != nil {
		h.logger.Error().Err(err).Msg("Failed to decode device")
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Basic validation
	if err := device.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("Validation error: %v", err), http.StatusBadRequest)
		return
	}

	// TODO: Actually test the connection with the protocol pool
	// For now, just return success if validation passes
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "Connection test passed (validation only)",
	})
}
