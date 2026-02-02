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

// =============================================================================
// Security Middleware
// =============================================================================

// Middleware wraps an http.Handler with security checks.
type Middleware struct {
	config config.APIConfig
	logger zerolog.Logger
}

// NewMiddleware creates a new middleware with the given configuration.
func NewMiddleware(cfg config.APIConfig, logger zerolog.Logger) *Middleware {
	return &Middleware{
		config: cfg,
		logger: logger.With().Str("component", "api-middleware").Logger(),
	}
}

// RequireAuth wraps a handler with API key authentication.
// If auth is disabled in config, the handler is called directly.
func (m *Middleware) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !m.config.AuthEnabled {
			next(w, r)
			return
		}

		// Check for API key in header or query param
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("api_key")
		}

		if apiKey == "" {
			m.logger.Warn().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("remote", r.RemoteAddr).
				Msg("Missing API key")
			http.Error(w, "Unauthorized: API key required", http.StatusUnauthorized)
			return
		}

		if apiKey != m.config.APIKey {
			m.logger.Warn().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("remote", r.RemoteAddr).
				Msg("Invalid API key")
			http.Error(w, "Unauthorized: Invalid API key", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// LimitRequestBody wraps a handler to limit request body size.
// This prevents DoS attacks via large payloads.
func (m *Middleware) LimitRequestBody(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if m.config.MaxRequestBodySize > 0 && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, m.config.MaxRequestBodySize)
		}
		next(w, r)
	}
}

// CORS adds CORS headers based on configuration.
// Returns true if this was a preflight request that was handled.
func (m *Middleware) CORS(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}

	// Check if origin is allowed
	allowed := false
	allowedOrigin := ""

	if len(m.config.AllowedOrigins) == 0 {
		// If no origins configured, allow all (development mode)
		allowed = true
		allowedOrigin = "*"
	} else {
		for _, o := range m.config.AllowedOrigins {
			if o == "*" || o == origin {
				allowed = true
				allowedOrigin = origin
				break
			}
		}
	}

	if !allowed {
		m.logger.Warn().
			Str("origin", origin).
			Msg("CORS: origin not allowed")
		return false
	}

	w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
	w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours

	// Handle preflight
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return true
	}

	return false
}

// Secure combines authentication, body size limiting, and CORS.
func (m *Middleware) Secure(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Handle CORS first (including preflight)
		if m.CORS(w, r) {
			return
		}

		// Apply body size limit
		if m.config.MaxRequestBodySize > 0 && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, m.config.MaxRequestBodySize)
		}

		// Check authentication for non-GET requests or if explicitly required
		if m.config.AuthEnabled {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				apiKey = r.URL.Query().Get("api_key")
			}

			if apiKey != m.config.APIKey {
				m.logger.Warn().
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Str("remote", r.RemoteAddr).
					Msg("Authentication failed")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		next(w, r)
	}
}

// ReadOnly applies CORS and body size limit but no auth (for public read endpoints).
func (m *Middleware) ReadOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if m.CORS(w, r) {
			return
		}

		if m.config.MaxRequestBodySize > 0 && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, m.config.MaxRequestBodySize)
		}

		next(w, r)
	}
}

// =============================================================================
// Device Manager
// =============================================================================

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
