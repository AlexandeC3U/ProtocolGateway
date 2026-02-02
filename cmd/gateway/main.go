// Package main is the entry point for the Protocol Gateway service.
// It initializes all components and manages the application lifecycle.
package main

import (
	"context"
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
	defer mqttPublisher.Disconnect()

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
	defer modbusPool.Close()

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
	defer opcuaPool.Close()

	// Register OPC UA protocol
	protocolManager.RegisterPool(domain.ProtocolOPCUA, opcuaPool)
	logger.Info().Msg("OPC UA connection pool initialized")

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
	defer s7Pool.Close()

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
		// On device edit - use safe replacement pattern
		func(device *domain.Device) error {
			// Validate protocol is supported
			if _, exists := protocolManager.GetPool(device.Protocol); !exists {
				logger.Warn().
					Str("device_id", device.ID).
					Str("protocol", string(device.Protocol)).
					Msg("Device uses unsupported protocol, skipping registration")
				return domain.ErrProtocolNotSupported
			}
			// TODO: Implement ReplaceDevice() to preserve poll jitter, retry state, etc.
			// For now, unregister and re-register (state is lost)
			pollingSvc.UnregisterDevice(device.ID)
			return pollingSvc.RegisterDevice(ctx, device)
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
	defer cmdHandler.Stop()

	// =============================================================
	// Initialize Health Checks and HTTP Server
	// =============================================================

	// Initialize health checker
	healthChecker := health.NewChecker(health.Config{
		ServiceName:    serviceName,
		ServiceVersion: serviceVersion,
	})
	healthChecker.AddCheck("mqtt", mqttPublisher)
	healthChecker.AddCheck("modbus_pool", modbusPool)
	healthChecker.AddCheck("opcua_pool", opcuaPool)
	healthChecker.AddCheck("s7_pool", s7Pool)

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

	// Stop health checker first (marks state as shutting down)
	healthChecker.Stop()

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop command handler first
	if err := cmdHandler.Stop(); err != nil {
		logger.Error().Err(err).Msg("Error stopping command handler")
	}

	// Stop polling service
	if err := pollingSvc.Stop(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("Error stopping polling service")
	}

	// Shutdown HTTP server
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("Error shutting down HTTP server")
	}

	// Close protocol pools (handled by defer)
	logger.Info().Msg("Protocol Gateway shutdown complete")
}
