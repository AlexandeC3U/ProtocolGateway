// Package main is the entry point for the Protocol Gateway service.
// It initializes all components and manages the application lifecycle.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/config"
	"github.com/nexus-edge/protocol-gateway/internal/adapter/modbus"
	"github.com/nexus-edge/protocol-gateway/internal/adapter/mqtt"
	"github.com/nexus-edge/protocol-gateway/internal/adapter/opcua"
	"github.com/nexus-edge/protocol-gateway/internal/adapter/s7"
	"github.com/nexus-edge/protocol-gateway/internal/api"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/nexus-edge/protocol-gateway/internal/health"
	"github.com/nexus-edge/protocol-gateway/internal/metrics"
	"github.com/nexus-edge/protocol-gateway/internal/service"
	"github.com/nexus-edge/protocol-gateway/pkg/logging"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

const (
	serviceName    = "protocol-gateway"
	serviceVersion = "2.0.0"
)

// gatewayReady is set to true once all components are initialized and healthy.
// Used to gate /metrics and other endpoints that shouldn't be scraped early.
var gatewayReady atomic.Bool

func main() {
	// Initialize structured logger
	logger := logging.New(serviceName, serviceVersion)
	logger.Info().Msg("Starting Protocol Gateway")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to load configuration")
	}
	logger.Info().Str("env", cfg.Environment).Msg("Configuration loaded")

	// Initialize metrics
	metricsRegistry := metrics.NewRegistry()
	// Pre-seed per-protocol connection gauges so they appear in Prometheus
	// even before the first connection attempt.
	metricsRegistry.UpdateActiveConnectionsForProtocol(string(domain.ProtocolModbusTCP), 0)
	metricsRegistry.UpdateActiveConnectionsForProtocol(string(domain.ProtocolModbusRTU), 0)
	metricsRegistry.UpdateActiveConnectionsForProtocol(string(domain.ProtocolOPCUA), 0)
	metricsRegistry.UpdateActiveConnectionsForProtocol(string(domain.ProtocolS7), 0)

	// Create root context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start system metrics collector (goroutines, memory)
	metricsRegistry.StartSystemMetricsCollector(ctx, 15*time.Second)

	// Initialize MQTT publisher
	mqttPublisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTT.BrokerURL,
		ClientID:       cfg.MQTT.ClientID,
		Username:       cfg.MQTT.Username,
		Password:       cfg.MQTT.Password,
		CleanSession:   cfg.MQTT.CleanSession,
		QoS:            cfg.MQTT.QoS,
		KeepAlive:      cfg.MQTT.KeepAlive,
		ConnectTimeout: cfg.MQTT.ConnectTimeout,
		ReconnectDelay: cfg.MQTT.ReconnectDelay,
		MaxReconnect:   cfg.MQTT.MaxReconnect,
		TLSEnabled:     cfg.MQTT.TLSEnabled,
		TLSCertFile:    cfg.MQTT.TLSCertFile,
		TLSKeyFile:     cfg.MQTT.TLSKeyFile,
		TLSCAFile:      cfg.MQTT.TLSCAFile,
	}, logger, metricsRegistry)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create MQTT publisher")
	}

	// Connect to MQTT broker
	if err := mqttPublisher.Connect(ctx); err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to MQTT broker")
	}
	// Note: publisher is disconnected explicitly during shutdown (not deferred).

	// =============================================================
	// Initialize Protocol Pools
	// =============================================================

	// Create protocol manager
	protocolManager := domain.NewProtocolManager()

	// Initialize Modbus connection pool
	modbusPool := modbus.NewConnectionPool(modbus.PoolConfig{
		MaxConnections:     cfg.Modbus.MaxConnections,
		IdleTimeout:        cfg.Modbus.IdleTimeout,
		HealthCheckPeriod:  cfg.Modbus.HealthCheckPeriod,
		ConnectionTimeout:  cfg.Modbus.ConnectionTimeout,
		RetryAttempts:      cfg.Modbus.RetryAttempts,
		RetryDelay:         cfg.Modbus.RetryDelay,
		CircuitBreakerName: "modbus-pool",
	}, logger, metricsRegistry)
	// Note: pool is closed explicitly during shutdown (not deferred)
	// to ensure correct ordering: services stop before pools close.

	// Register Modbus protocols
	protocolManager.RegisterPool(domain.ProtocolModbusTCP, modbusPool)
	protocolManager.RegisterPool(domain.ProtocolModbusRTU, modbusPool)
	logger.Info().Msg("Modbus connection pool initialized")

	// Initialize OPC UA connection pool
	opcuaPool := opcua.NewConnectionPool(opcua.PoolConfig{
		MaxConnections:        cfg.OPCUA.MaxConnections,
		IdleTimeout:           cfg.OPCUA.IdleTimeout,
		HealthCheckPeriod:     cfg.OPCUA.HealthCheckPeriod,
		ConnectionTimeout:     cfg.OPCUA.ConnectionTimeout,
		RetryAttempts:         cfg.OPCUA.RetryAttempts,
		RetryDelay:            cfg.OPCUA.RetryDelay,
		CircuitBreakerName:    "opcua-pool",
		DefaultSecurityPolicy: cfg.OPCUA.DefaultSecurityPolicy,
		DefaultSecurityMode:   cfg.OPCUA.DefaultSecurityMode,
		DefaultAuthMode:       cfg.OPCUA.DefaultAuthMode,
	}, logger, metricsRegistry)
	// Note: pool is closed explicitly during shutdown (not deferred).

	// Register OPC UA protocol
	protocolManager.RegisterPool(domain.ProtocolOPCUA, opcuaPool)
	logger.Info().Msg("OPC UA connection pool initialized")

	// Initialize OPC UA trust store for certificate management
	var opcuaTrustStore *opcua.TrustStore
	if cfg.OPCUA.TrustStorePath != "" {
		var err error
		opcuaTrustStore, err = opcua.NewTrustStore(cfg.OPCUA.TrustStorePath, logger)
		if err != nil {
			logger.Warn().Err(err).Msg("Failed to initialize OPC UA trust store, certificate management disabled")
		} else {
			logger.Info().Str("path", cfg.OPCUA.TrustStorePath).Msg("OPC UA trust store initialized")
			opcuaPool.SetTrustStore(opcuaTrustStore, cfg.OPCUA.AutoTrust)
		}
	}

	// Create OPC UA subscription adapter for push-based data delivery.
	// This wraps the pool's SubscribeDevice/UnsubscribeDevice methods
	// behind the service.SubscriptionHandler interface.
	opcuaSubAdapter := &opcuaSubscriptionAdapter{pool: opcuaPool}

	// Initialize S7 connection pool
	s7Pool := s7.NewPool(s7.PoolConfig{
		MaxConnections:      cfg.S7.MaxConnections,
		IdleTimeout:         cfg.S7.IdleTimeout,
		HealthCheckInterval: cfg.S7.HealthCheckPeriod,
		RetryDelay:          cfg.S7.RetryDelay,
		CircuitBreaker: s7.CircuitBreakerConfig{
			MaxRequests:      cfg.S7.CBMaxRequests,
			Interval:         cfg.S7.CBInterval,
			Timeout:          cfg.S7.CBTimeout,
			FailureThreshold: cfg.S7.CBFailureThreshold,
		},
	}, logger, metricsRegistry)
	// Note: pool is closed explicitly during shutdown (not deferred).

	// Register S7 protocol
	protocolManager.RegisterPool(domain.ProtocolS7, s7Pool)
	logger.Info().Msg("S7 connection pool initialized")

	// =============================================================
	// Initialize Services
	// =============================================================

	// Initialize polling service with protocol manager
	pollingSvc := service.NewPollingService(service.PollingConfig{
		WorkerCount:     cfg.Polling.WorkerCount,
		BatchSize:       cfg.Polling.BatchSize,
		DefaultInterval: cfg.Polling.DefaultInterval,
		MaxRetries:      cfg.Polling.MaxRetries,
		ShutdownTimeout: cfg.Polling.ShutdownTimeout,
	}, protocolManager, mqttPublisher, logger, metricsRegistry)

	// Wire OPC UA subscription handler for push-based data delivery.
	// Devices with opc_use_subscriptions=true will use server-side subscriptions
	// instead of polling, receiving data via Report-by-Exception.
	pollingSvc.SetSubscriptionHandler(opcuaSubAdapter)

	// Initialize device manager for web UI
	deviceManager := api.NewDeviceManager(cfg.DevicesConfigPath, logger)

	// Set up callbacks for device lifecycle events
	deviceManager.SetCallbacks(
		// On device add
		func(device *domain.Device) error {
			// Validate protocol is supported before registration
			if _, exists := protocolManager.GetPool(device.Protocol); !exists {
				logger.Warn().
					Str("device_id", device.ID).
					Str("protocol", string(device.Protocol)).
					Msg("Device uses unsupported protocol, skipping registration")
				return domain.ErrProtocolNotSupported
			}
			return pollingSvc.RegisterDevice(ctx, device)
		},
		// On device edit - atomically replace config, preserving polling state
		func(device *domain.Device) error {
			// Validate protocol is supported
			if _, exists := protocolManager.GetPool(device.Protocol); !exists {
				logger.Warn().
					Str("device_id", device.ID).
					Str("protocol", string(device.Protocol)).
					Msg("Device uses unsupported protocol, skipping registration")
				return domain.ErrProtocolNotSupported
			}
			return pollingSvc.ReplaceDevice(ctx, device)
		},
		// On device delete
		func(id string) error {
			pollingSvc.UnregisterDevice(id)
			return nil
		},
	)

	// Load device configurations into the device manager (source for /api/devices)
	if err := deviceManager.LoadDevices(); err != nil {
		logger.Fatal().Err(err).Msg("Failed to load device configurations")
	}
	devices := deviceManager.GetDevices()
	logger.Info().Int("count", len(devices)).Msg("Loaded device configurations")

	// Count devices by protocol and track unsupported
	protocolCounts := make(map[domain.Protocol]int)
	unsupportedCount := 0
	for _, device := range devices {
		if _, exists := protocolManager.GetPool(device.Protocol); !exists {
			unsupportedCount++
			logger.Warn().
				Str("device_id", device.ID).
				Str("protocol", string(device.Protocol)).
				Msg("Device configured with unsupported protocol")
			continue
		}
		protocolCounts[device.Protocol]++
	}
	for protocol, count := range protocolCounts {
		logger.Info().Str("protocol", string(protocol)).Int("devices", count).Msg("Protocol device count")
	}
	if unsupportedCount > 0 {
		logger.Warn().Int("count", unsupportedCount).Msg("Devices with unsupported protocols skipped")
	}

	// Register devices with polling service (with protocol validation)
	registeredCount := 0
	failedCount := 0
	for _, device := range devices {
		// Skip unsupported protocols
		if _, exists := protocolManager.GetPool(device.Protocol); !exists {
			continue
		}
		if err := pollingSvc.RegisterDevice(ctx, device); err != nil {
			logger.Error().Err(err).Str("device", device.ID).Msg("Failed to register device")
			failedCount++
		} else {
			registeredCount++
		}
	}

	// Start polling service
	if err := pollingSvc.Start(ctx); err != nil {
		logger.Fatal().Err(err).Msg("Failed to start polling service")
	}

	// Initialize command handler for bidirectional communication
	cmdHandler := service.NewCommandHandler(
		mqttPublisher.Client(),
		protocolManager,
		devices,
		service.DefaultCommandConfig(),
		logger,
	)
	if err := cmdHandler.Start(); err != nil {
		logger.Warn().Err(err).Msg("Failed to start command handler (write operations disabled)")
	} else {
		logger.Info().Msg("Command handler started - bidirectional communication enabled")
	}
	// Note: command handler is stopped explicitly during shutdown (not deferred).

	// =============================================================
	// Initialize Health Checks and HTTP Server
	// =============================================================

	// Initialize health checker
	healthChecker := health.NewChecker(health.Config{
		ServiceName:    serviceName,
		ServiceVersion: serviceVersion,
	}, logger)
	healthChecker.AddCheck("mqtt", mqttPublisher)
	healthChecker.AddCheck("modbus_pool", modbusPool)
	healthChecker.AddCheck("opcua_pool", opcuaPool)
	healthChecker.AddCheck("s7_pool", s7Pool)

	// Initialize NTP clock drift checker
	if cfg.NTP.Enabled {
		ntpChecker := health.NewNTPChecker(health.NTPConfig{
			Enabled:       cfg.NTP.Enabled,
			Server:        cfg.NTP.Server,
			CheckInterval: cfg.NTP.CheckInterval,
			WarnThreshold: cfg.NTP.WarnThreshold,
			CritThreshold: cfg.NTP.CritThreshold,
		}, logger, metricsRegistry)
		ntpChecker.Start()
		defer ntpChecker.Stop()
		healthChecker.AddCheckWithSeverity("ntp_sync", ntpChecker, health.SeverityWarning)
		logger.Info().Str("server", cfg.NTP.Server).Msg("NTP clock drift checker registered")
	}

	// Start background health checks
	healthChecker.Start()

	// Start HTTP server for health, metrics, and web UI
	mux := http.NewServeMux()

	// Health endpoints
	mux.HandleFunc("/health", healthChecker.HealthHandler)
	mux.HandleFunc("/health/live", healthChecker.LivenessHandler)
	mux.HandleFunc("/health/ready", healthChecker.ReadinessHandler)

	// Metrics endpoint with readiness guard to prevent incomplete data during startup
	metricsHandler := promhttp.Handler()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		if !gatewayReady.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("# Gateway is initializing, metrics not yet ready\n"))
			return
		}
		metricsHandler.ServeHTTP(w, r)
	})

	// Add status endpoint
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		stats := pollingSvc.Stats()
		fmt.Fprintf(w, `{"service":"%s","version":"%s","polling":{"total_polls":%d,"success_polls":%d,"failed_polls":%d,"skipped_polls":%d,"points_read":%d,"points_published":%d}}`,
			serviceName, serviceVersion,
			stats.TotalPolls, stats.SuccessPolls, stats.FailedPolls, stats.SkippedPolls,
			stats.PointsRead, stats.PointsPublished)
	})

	// Initialize API middleware with security configuration
	apiMiddleware := api.NewMiddleware(cfg.API, logger)

	// Web UI API endpoints
	apiHandler := api.NewAPIHandler(deviceManager, logger)
	apiHandler.SetTopicTracker(mqttPublisher)
	apiHandler.SetSubscriptionProvider(cmdHandler)
	apiHandler.SetLogProvider(api.NewDockerCLILogProvider(logger))

	// Device management endpoints (protected - require auth for mutations)
	mux.HandleFunc("/api/devices", apiMiddleware.Secure(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			if r.URL.Query().Get("id") != "" {
				apiHandler.GetDeviceHandler(w, r)
			} else {
				apiHandler.GetDevicesHandler(w, r)
			}
		case "POST":
			apiHandler.CreateDeviceHandler(w, r)
		case "PUT":
			apiHandler.UpdateDeviceHandler(w, r)
		case "DELETE":
			apiHandler.DeleteDeviceHandler(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/test-connection", apiMiddleware.Secure(func(w http.ResponseWriter, r *http.Request) {
		apiHandler.TestConnectionHandler(w, r)
	}))

	// OPC UA Browse endpoint - allows exploring the address space
	mux.HandleFunc("/api/browse/", apiMiddleware.ReadOnly(func(w http.ResponseWriter, r *http.Request) {
		handleBrowse(w, r, opcuaPool, deviceManager, logger)
	}))

	// OPC UA Certificate Trust Store API endpoints
	if opcuaTrustStore != nil {
		mux.HandleFunc("/api/opcua/certificates/trusted", apiMiddleware.Secure(func(w http.ResponseWriter, r *http.Request) {
			handleTrustedCerts(w, r, opcuaTrustStore, logger)
		}))
		mux.HandleFunc("/api/opcua/certificates/rejected", apiMiddleware.ReadOnly(func(w http.ResponseWriter, r *http.Request) {
			handleRejectedCerts(w, r, opcuaTrustStore, logger)
		}))
		mux.HandleFunc("/api/opcua/certificates/trust", apiMiddleware.Secure(func(w http.ResponseWriter, r *http.Request) {
			handleTrustCert(w, r, opcuaTrustStore, logger)
		}))
	}

	// Topics / Routes overview (read-only, no auth required)
	mux.HandleFunc("/api/topics", apiMiddleware.ReadOnly(func(w http.ResponseWriter, r *http.Request) {
		apiHandler.TopicsOverviewHandler(w, r)
	}))

	// Container logs (read-only, no auth required)
	mux.HandleFunc("/api/logs/containers", apiMiddleware.ReadOnly(func(w http.ResponseWriter, r *http.Request) {
		apiHandler.ListContainersHandler(w, r)
	}))

	mux.HandleFunc("/api/logs", apiMiddleware.ReadOnly(func(w http.ResponseWriter, r *http.Request) {
		apiHandler.LogsHandler(w, r)
	}))

	// Serve web UI static files
	mux.Handle("/", http.FileServer(http.Dir("./web")))

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTP.Port),
		Handler:      mux,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	// Start HTTP server in goroutine
	go func() {
		logger.Info().Int("port", cfg.HTTP.Port).Msg("Starting HTTP server")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error().Err(err).Msg("HTTP server error")
		}
	}()

	// Mark gateway as ready for metrics scraping
	gatewayReady.Store(true)

	// Log successful startup with detailed summary
	logger.Info().
		Int("registered_devices", registeredCount).
		Int("failed_devices", failedCount).
		Int("unsupported_protocol_devices", unsupportedCount).
		Int("modbus_devices", protocolCounts[domain.ProtocolModbusTCP]+protocolCounts[domain.ProtocolModbusRTU]).
		Int("opcua_devices", protocolCounts[domain.ProtocolOPCUA]).
		Int("s7_devices", protocolCounts[domain.ProtocolS7]).
		Int("http_port", cfg.HTTP.Port).
		Str("mqtt_broker", cfg.MQTT.BrokerURL).
		Msg("Protocol Gateway started successfully")

	// Log degraded state warning if any devices failed registration
	if failedCount > 0 || unsupportedCount > 0 {
		logger.Warn().
			Int("failed", failedCount).
			Int("unsupported", unsupportedCount).
			Msg("Gateway started in degraded state - some devices not registered")
	}

	// =============================================================
	// Shutdown Handling
	// =============================================================

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Msg("Shutdown signal received, initiating graceful shutdown...")

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Shutdown order matters — each step ensures no new work arrives before
	// the next layer down is closed. The sequence is:
	//
	//   1. Health checker  → mark shutting down (probes return 503)
	//   2. HTTP server     → stop accepting new API requests, drain in-flight
	//   3. Command handler → stop processing MQTT write commands
	//   4. Polling service → stop all device pollers, wait for workers to finish
	//   5. Protocol pools  → close connections (no more readers/writers)
	//   6. MQTT publisher  → flush remaining buffer, disconnect
	//
	// This guarantees that no component tries to use a resource that has
	// already been torn down.

	// 1. Stop health checker (marks state as shutting down, probes return 503)
	healthChecker.Stop()

	// 2. Shutdown HTTP server (stop accepting requests, drain in-flight handlers)
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("Error shutting down HTTP server")
	}

	// 3. Stop command handler (stop processing MQTT write commands)
	if err := cmdHandler.Stop(); err != nil {
		logger.Error().Err(err).Msg("Error stopping command handler")
	}

	// 4. Stop polling service (cancel all device pollers, wait for workers)
	if err := pollingSvc.Stop(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("Error stopping polling service")
	}

	// 5. Close protocol pools (no more readers/writers at this point)
	if err := opcuaPool.Close(); err != nil {
		logger.Error().Err(err).Msg("Error closing OPC UA connection pool")
	}
	if err := modbusPool.Close(); err != nil {
		logger.Error().Err(err).Msg("Error closing Modbus connection pool")
	}
	if err := s7Pool.Close(); err != nil {
		logger.Error().Err(err).Msg("Error closing S7 connection pool")
	}

	// 6. Disconnect MQTT publisher last (flush remaining buffered messages)
	mqttPublisher.Disconnect()

	logger.Info().Msg("Protocol Gateway shutdown complete")
}

// opcuaSubscriptionAdapter adapts the OPC UA ConnectionPool's subscription
// methods to the service.SubscriptionHandler interface.
type opcuaSubscriptionAdapter struct {
	pool *opcua.ConnectionPool
}

func (a *opcuaSubscriptionAdapter) Subscribe(ctx context.Context, device *domain.Device, tags []*domain.Tag, onData func(*domain.DataPoint)) error {
	return a.pool.SubscribeDevice(ctx, device, tags, onData)
}

func (a *opcuaSubscriptionAdapter) Unsubscribe(deviceID string) error {
	return a.pool.UnsubscribeDevice(deviceID)
}

// handleBrowse handles OPC UA browse requests to explore the address space.
// GET /api/browse/{deviceID}?node_id=ns=2;s=Demo&max_depth=1
func handleBrowse(w http.ResponseWriter, r *http.Request, pool *opcua.ConnectionPool, deviceManager *api.DeviceManager, logger zerolog.Logger) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract device ID from path: /api/browse/{deviceID}
	path := r.URL.Path
	prefix := "/api/browse/"
	if !hasPrefix(path, prefix) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	deviceID := path[len(prefix):]
	if deviceID == "" {
		http.Error(w, "Device ID is required", http.StatusBadRequest)
		return
	}

	// Get query parameters
	nodeID := r.URL.Query().Get("node_id")
	maxDepthStr := r.URL.Query().Get("max_depth")
	maxDepth := 1
	if maxDepthStr != "" {
		if d, err := parseInt(maxDepthStr); err == nil && d > 0 {
			maxDepth = d
			if maxDepth > 5 {
				maxDepth = 5 // Cap at 5 to prevent excessive browsing
			}
		}
	}

	// Get device to verify it's an OPC UA device
	device, found := deviceManager.GetDevice(deviceID)
	if !found {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	if device.Protocol != domain.ProtocolOPCUA {
		http.Error(w, "Browse is only supported for OPC UA devices", http.StatusBadRequest)
		return
	}

	// Ensure device is connected first
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	_, err := pool.GetClient(ctx, device)
	if err != nil {
		logger.Error().Err(err).Str("device_id", deviceID).Msg("Failed to get OPC UA client for browse")
		http.Error(w, fmt.Sprintf("Failed to connect to device: %v", err), http.StatusServiceUnavailable)
		return
	}

	// Perform browse
	result, err := pool.BrowseNodes(ctx, deviceID, nodeID, maxDepth)
	if err != nil {
		logger.Error().Err(err).Str("device_id", deviceID).Str("node_id", nodeID).Msg("Browse failed")
		http.Error(w, fmt.Sprintf("Browse failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	if err := encodeJSON(w, result); err != nil {
		logger.Error().Err(err).Msg("Failed to encode browse result")
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid number")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func encodeJSON(w http.ResponseWriter, v interface{}) error {
	return json.NewEncoder(w).Encode(v)
}

// handleTrustedCerts handles GET and DELETE for trusted certificates.
// GET /api/opcua/certificates/trusted - List all trusted certs
// DELETE /api/opcua/certificates/trusted?fingerprint=sha256:... - Remove a trusted cert
func handleTrustedCerts(w http.ResponseWriter, r *http.Request, ts *opcua.TrustStore, logger zerolog.Logger) {
	switch r.Method {
	case http.MethodGet:
		certs, err := ts.ListTrustedCerts()
		if err != nil {
			logger.Error().Err(err).Msg("Failed to list trusted certificates")
			http.Error(w, fmt.Sprintf("Failed to list trusted certificates: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		encodeJSON(w, map[string]interface{}{
			"certificates": certs,
			"count":        len(certs),
		})

	case http.MethodDelete:
		fingerprint := r.URL.Query().Get("fingerprint")
		if fingerprint == "" {
			http.Error(w, "fingerprint query parameter is required", http.StatusBadRequest)
			return
		}
		if err := ts.RemoveTrustedCert(fingerprint); err != nil {
			logger.Error().Err(err).Str("fingerprint", fingerprint).Msg("Failed to remove trusted certificate")
			http.Error(w, fmt.Sprintf("Failed to remove certificate: %v", err), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		encodeJSON(w, map[string]string{"status": "removed", "fingerprint": fingerprint})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRejectedCerts handles GET for rejected certificates.
// GET /api/opcua/certificates/rejected - List all rejected certs
func handleRejectedCerts(w http.ResponseWriter, r *http.Request, ts *opcua.TrustStore, logger zerolog.Logger) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	certs, err := ts.ListRejectedCerts()
	if err != nil {
		logger.Error().Err(err).Msg("Failed to list rejected certificates")
		http.Error(w, fmt.Sprintf("Failed to list rejected certificates: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	encodeJSON(w, map[string]interface{}{
		"certificates": certs,
		"count":        len(certs),
	})
}

// handleTrustCert handles POST to promote a rejected cert to trusted.
// POST /api/opcua/certificates/trust with body {"fingerprint": "sha256:..."}
func handleTrustCert(w http.ResponseWriter, r *http.Request, ts *opcua.TrustStore, logger zerolog.Logger) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.Fingerprint == "" {
		http.Error(w, "fingerprint is required", http.StatusBadRequest)
		return
	}

	if err := ts.PromoteCert(req.Fingerprint); err != nil {
		logger.Error().Err(err).Str("fingerprint", req.Fingerprint).Msg("Failed to promote certificate")
		http.Error(w, fmt.Sprintf("Failed to promote certificate: %v", err), http.StatusNotFound)
		return
	}

	logger.Info().Str("fingerprint", req.Fingerprint).Msg("Certificate promoted to trusted")
	w.Header().Set("Content-Type", "application/json")
	encodeJSON(w, map[string]string{"status": "trusted", "fingerprint": req.Fingerprint})
}
