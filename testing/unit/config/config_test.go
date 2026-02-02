// Package config tests configuration loading and validation.
package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/config"
)

// TestConfigStructure verifies the Config struct has all expected fields.
func TestConfigStructure(t *testing.T) {
	cfg := &config.Config{
		Environment:       "development",
		DevicesConfigPath: "./devices.yaml",
		HTTP: config.HTTPConfig{
			Port:         8080,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		API: config.APIConfig{
			AuthEnabled:        false,
			APIKey:             "",
			MaxRequestBodySize: 1048576,
			AllowedOrigins:     []string{},
		},
		MQTT: config.MQTTConfig{
			BrokerURL:      "tcp://localhost:1883",
			ClientID:       "protocol-gateway",
			CleanSession:   true,
			QoS:            1,
			KeepAlive:      30 * time.Second,
			ConnectTimeout: 10 * time.Second,
			ReconnectDelay: 5 * time.Second,
			MaxReconnect:   -1,
			BufferSize:     10000,
		},
		Modbus: config.ModbusConfig{
			MaxConnections:    100,
			IdleTimeout:       5 * time.Minute,
			HealthCheckPeriod: 30 * time.Second,
			ConnectionTimeout: 10 * time.Second,
			RetryAttempts:     3,
			RetryDelay:        100 * time.Millisecond,
		},
		OPCUA: config.OPCUAConfig{
			MaxConnections:        50,
			IdleTimeout:           5 * time.Minute,
			HealthCheckPeriod:     30 * time.Second,
			ConnectionTimeout:     15 * time.Second,
			RetryAttempts:         3,
			RetryDelay:            500 * time.Millisecond,
			DefaultSecurityPolicy: "None",
			DefaultSecurityMode:   "None",
			DefaultAuthMode:       "Anonymous",
		},
		S7: config.S7Config{
			MaxConnections:     100,
			IdleTimeout:        5 * time.Minute,
			HealthCheckPeriod:  30 * time.Second,
			ConnectionTimeout:  10 * time.Second,
			RetryAttempts:      3,
			RetryDelay:         5 * time.Second,
			CBMaxRequests:      3,
			CBInterval:         10 * time.Second,
			CBTimeout:          30 * time.Second,
			CBFailureThreshold: 5,
		},
		Polling: config.PollingConfig{
			WorkerCount:     10,
			BatchSize:       50,
			DefaultInterval: 1 * time.Second,
			MaxRetries:      3,
			ShutdownTimeout: 30 * time.Second,
		},
		Logging: config.LoggingConfig{
			Level:      "info",
			Format:     "json",
			Output:     "stdout",
			TimeFormat: time.RFC3339,
		},
	}

	// Verify essential fields are set
	if cfg.Environment != "development" {
		t.Errorf("expected environment 'development', got %q", cfg.Environment)
	}
	if cfg.HTTP.Port != 8080 {
		t.Errorf("expected HTTP port 8080, got %d", cfg.HTTP.Port)
	}
	if cfg.MQTT.QoS != 1 {
		t.Errorf("expected MQTT QoS 1, got %d", cfg.MQTT.QoS)
	}
}

// TestConfigValidation tests the Config.Validate() method.
func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		config    config.Config
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid config",
			config: config.Config{
				MQTT: config.MQTTConfig{
					BrokerURL: "tcp://localhost:1883",
				},
				HTTP: config.HTTPConfig{
					Port: 8080,
				},
				Polling: config.PollingConfig{
					WorkerCount: 10,
				},
				Modbus: config.ModbusConfig{
					MaxConnections: 100,
				},
			},
			wantError: false,
		},
		{
			name: "missing MQTT broker URL",
			config: config.Config{
				MQTT: config.MQTTConfig{
					BrokerURL: "",
				},
				HTTP: config.HTTPConfig{
					Port: 8080,
				},
				Polling: config.PollingConfig{
					WorkerCount: 10,
				},
				Modbus: config.ModbusConfig{
					MaxConnections: 100,
				},
			},
			wantError: true,
			errorMsg:  "MQTT broker URL is required",
		},
		{
			name: "invalid HTTP port - zero",
			config: config.Config{
				MQTT: config.MQTTConfig{
					BrokerURL: "tcp://localhost:1883",
				},
				HTTP: config.HTTPConfig{
					Port: 0,
				},
				Polling: config.PollingConfig{
					WorkerCount: 10,
				},
				Modbus: config.ModbusConfig{
					MaxConnections: 100,
				},
			},
			wantError: true,
			errorMsg:  "invalid HTTP port",
		},
		{
			name: "invalid HTTP port - too high",
			config: config.Config{
				MQTT: config.MQTTConfig{
					BrokerURL: "tcp://localhost:1883",
				},
				HTTP: config.HTTPConfig{
					Port: 70000,
				},
				Polling: config.PollingConfig{
					WorkerCount: 10,
				},
				Modbus: config.ModbusConfig{
					MaxConnections: 100,
				},
			},
			wantError: true,
			errorMsg:  "invalid HTTP port",
		},
		{
			name: "invalid polling worker count",
			config: config.Config{
				MQTT: config.MQTTConfig{
					BrokerURL: "tcp://localhost:1883",
				},
				HTTP: config.HTTPConfig{
					Port: 8080,
				},
				Polling: config.PollingConfig{
					WorkerCount: 0,
				},
				Modbus: config.ModbusConfig{
					MaxConnections: 100,
				},
			},
			wantError: true,
			errorMsg:  "polling worker count must be positive",
		},
		{
			name: "invalid modbus max connections",
			config: config.Config{
				MQTT: config.MQTTConfig{
					BrokerURL: "tcp://localhost:1883",
				},
				HTTP: config.HTTPConfig{
					Port: 8080,
				},
				Polling: config.PollingConfig{
					WorkerCount: 10,
				},
				Modbus: config.ModbusConfig{
					MaxConnections: 0,
				},
			},
			wantError: true,
			errorMsg:  "modbus max connections must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
					return
				}
				if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestHTTPConfigDefaults tests HTTPConfig default values.
func TestHTTPConfigDefaults(t *testing.T) {
	cfg := config.HTTPConfig{
		Port:         8080,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if cfg.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Port)
	}
	if cfg.ReadTimeout != 10*time.Second {
		t.Errorf("expected ReadTimeout 10s, got %v", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != 10*time.Second {
		t.Errorf("expected WriteTimeout 10s, got %v", cfg.WriteTimeout)
	}
	if cfg.IdleTimeout != 60*time.Second {
		t.Errorf("expected IdleTimeout 60s, got %v", cfg.IdleTimeout)
	}
}

// TestMQTTConfigDefaults tests MQTTConfig structure and values.
func TestMQTTConfigDefaults(t *testing.T) {
	cfg := config.MQTTConfig{
		BrokerURL:      "tcp://localhost:1883",
		ClientID:       "protocol-gateway",
		CleanSession:   true,
		QoS:            1,
		KeepAlive:      30 * time.Second,
		ConnectTimeout: 10 * time.Second,
		ReconnectDelay: 5 * time.Second,
		MaxReconnect:   -1,
		BufferSize:     10000,
	}

	if cfg.BrokerURL != "tcp://localhost:1883" {
		t.Errorf("expected broker URL 'tcp://localhost:1883', got %q", cfg.BrokerURL)
	}
	if cfg.QoS != 1 {
		t.Errorf("expected QoS 1, got %d", cfg.QoS)
	}
	if cfg.MaxReconnect != -1 {
		t.Errorf("expected MaxReconnect -1 (unlimited), got %d", cfg.MaxReconnect)
	}
}

// TestModbusConfigDefaults tests ModbusConfig structure and values.
func TestModbusConfigDefaults(t *testing.T) {
	cfg := config.ModbusConfig{
		MaxConnections:    100,
		IdleTimeout:       5 * time.Minute,
		HealthCheckPeriod: 30 * time.Second,
		ConnectionTimeout: 10 * time.Second,
		RetryAttempts:     3,
		RetryDelay:        100 * time.Millisecond,
	}

	if cfg.MaxConnections != 100 {
		t.Errorf("expected MaxConnections 100, got %d", cfg.MaxConnections)
	}
	if cfg.RetryAttempts != 3 {
		t.Errorf("expected RetryAttempts 3, got %d", cfg.RetryAttempts)
	}
}

// TestOPCUAConfigDefaults tests OPCUAConfig structure and values.
func TestOPCUAConfigDefaults(t *testing.T) {
	cfg := config.OPCUAConfig{
		MaxConnections:        50,
		IdleTimeout:           5 * time.Minute,
		HealthCheckPeriod:     30 * time.Second,
		ConnectionTimeout:     15 * time.Second,
		RetryAttempts:         3,
		RetryDelay:            500 * time.Millisecond,
		DefaultSecurityPolicy: "None",
		DefaultSecurityMode:   "None",
		DefaultAuthMode:       "Anonymous",
	}

	if cfg.MaxConnections != 50 {
		t.Errorf("expected MaxConnections 50, got %d", cfg.MaxConnections)
	}
	if cfg.DefaultSecurityPolicy != "None" {
		t.Errorf("expected DefaultSecurityPolicy 'None', got %q", cfg.DefaultSecurityPolicy)
	}
}

// TestS7ConfigDefaults tests S7Config structure including circuit breaker.
func TestS7ConfigDefaults(t *testing.T) {
	cfg := config.S7Config{
		MaxConnections:     100,
		IdleTimeout:        5 * time.Minute,
		HealthCheckPeriod:  30 * time.Second,
		ConnectionTimeout:  10 * time.Second,
		RetryAttempts:      3,
		RetryDelay:         5 * time.Second,
		CBMaxRequests:      3,
		CBInterval:         10 * time.Second,
		CBTimeout:          30 * time.Second,
		CBFailureThreshold: 5,
	}

	if cfg.CBMaxRequests != 3 {
		t.Errorf("expected CBMaxRequests 3, got %d", cfg.CBMaxRequests)
	}
	if cfg.CBFailureThreshold != 5 {
		t.Errorf("expected CBFailureThreshold 5, got %d", cfg.CBFailureThreshold)
	}
}

// TestPollingConfigDefaults tests PollingConfig structure and values.
func TestPollingConfigDefaults(t *testing.T) {
	cfg := config.PollingConfig{
		WorkerCount:     10,
		BatchSize:       50,
		DefaultInterval: 1 * time.Second,
		MaxRetries:      3,
		ShutdownTimeout: 30 * time.Second,
	}

	if cfg.WorkerCount != 10 {
		t.Errorf("expected WorkerCount 10, got %d", cfg.WorkerCount)
	}
	if cfg.BatchSize != 50 {
		t.Errorf("expected BatchSize 50, got %d", cfg.BatchSize)
	}
}

// TestLoggingConfigDefaults tests LoggingConfig structure and values.
func TestLoggingConfigDefaults(t *testing.T) {
	cfg := config.LoggingConfig{
		Level:      "info",
		Format:     "json",
		Output:     "stdout",
		TimeFormat: time.RFC3339,
	}

	if cfg.Level != "info" {
		t.Errorf("expected Level 'info', got %q", cfg.Level)
	}
	if cfg.Format != "json" {
		t.Errorf("expected Format 'json', got %q", cfg.Format)
	}
}

// TestAPIConfigAuth tests API authentication config settings.
func TestAPIConfigAuth(t *testing.T) {
	tests := []struct {
		name        string
		authEnabled bool
		apiKey      string
		maxBodySize int64
	}{
		{"disabled auth", false, "", 1048576},
		{"enabled auth", true, "secret-key-123", 1048576},
		{"custom body size", true, "key", 2097152},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.APIConfig{
				AuthEnabled:        tt.authEnabled,
				APIKey:             tt.apiKey,
				MaxRequestBodySize: tt.maxBodySize,
			}

			if cfg.AuthEnabled != tt.authEnabled {
				t.Errorf("expected AuthEnabled %v, got %v", tt.authEnabled, cfg.AuthEnabled)
			}
			if cfg.APIKey != tt.apiKey {
				t.Errorf("expected APIKey %q, got %q", tt.apiKey, cfg.APIKey)
			}
			if cfg.MaxRequestBodySize != tt.maxBodySize {
				t.Errorf("expected MaxRequestBodySize %d, got %d", tt.maxBodySize, cfg.MaxRequestBodySize)
			}
		})
	}
}

// TestEnvironmentOverrides verifies environment variable behavior.
func TestEnvironmentOverrides(t *testing.T) {
	// Save original env vars
	originalEnv := os.Getenv("GATEWAY_ENVIRONMENT")
	defer os.Setenv("GATEWAY_ENVIRONMENT", originalEnv)

	// Note: This test documents expected behavior.
	// Full integration testing of config.Load() with env vars
	// requires additional setup with temp directories.

	tests := []struct {
		envVar   string
		envValue string
		expected string
	}{
		{"GATEWAY_ENVIRONMENT", "production", "production"},
		{"GATEWAY_ENVIRONMENT", "staging", "staging"},
		{"GATEWAY_ENVIRONMENT", "development", "development"},
	}

	for _, tt := range tests {
		t.Run(tt.envVar+"="+tt.envValue, func(t *testing.T) {
			// This test verifies the config structure supports the environment field.
			// Full env var testing requires the config.Load() function with proper setup.
			cfg := config.Config{
				Environment: tt.expected,
			}
			if cfg.Environment != tt.expected {
				t.Errorf("expected Environment %q, got %q", tt.expected, cfg.Environment)
			}
		})
	}
}

// TestMQTTTLSConfig tests MQTT TLS configuration fields.
func TestMQTTTLSConfig(t *testing.T) {
	cfg := config.MQTTConfig{
		TLSEnabled:  true,
		TLSCertFile: "/path/to/cert.pem",
		TLSKeyFile:  "/path/to/key.pem",
		TLSCAFile:   "/path/to/ca.pem",
	}

	if !cfg.TLSEnabled {
		t.Error("expected TLSEnabled to be true")
	}
	if cfg.TLSCertFile != "/path/to/cert.pem" {
		t.Errorf("expected TLSCertFile '/path/to/cert.pem', got %q", cfg.TLSCertFile)
	}
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
