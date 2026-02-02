// Package config provides configuration management for the Protocol Gateway.
// It supports environment variables, config files (YAML/JSON), and defaults.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the Protocol Gateway.
type Config struct {
	// Environment is the deployment environment (development, staging, production)
	Environment string `mapstructure:"environment"`

	// DevicesConfigPath is the path to the device configurations file
	DevicesConfigPath string `mapstructure:"devices_config_path"`

	// HTTP server configuration
	HTTP HTTPConfig `mapstructure:"http"`

	// API configuration (authentication, rate limiting, etc.)
	API APIConfig `mapstructure:"api"`

	// MQTT configuration
	MQTT MQTTConfig `mapstructure:"mqtt"`

	// Modbus configuration
	Modbus ModbusConfig `mapstructure:"modbus"`

	// OPC UA configuration
	OPCUA OPCUAConfig `mapstructure:"opcua"`

	// S7 configuration
	S7 S7Config `mapstructure:"s7"`

	// Polling configuration
	Polling PollingConfig `mapstructure:"polling"`

	// Logging configuration
	Logging LoggingConfig `mapstructure:"logging"`
}

// HTTPConfig holds HTTP server configuration.
type HTTPConfig struct {
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
}

// APIConfig holds API security and rate limiting configuration.
type APIConfig struct {
	// AuthEnabled enables API key authentication for protected endpoints
	AuthEnabled bool `mapstructure:"auth_enabled"`

	// APIKey is the secret key required for authenticated endpoints
	// In production, use a strong, randomly generated key
	APIKey string `mapstructure:"api_key"`

	// MaxRequestBodySize is the maximum allowed request body size in bytes
	// Default: 1MB. Set to 0 to disable limit (not recommended).
	MaxRequestBodySize int64 `mapstructure:"max_request_body_size"`

	// AllowedOrigins for CORS. Use "*" to allow all (not recommended for production)
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

// MQTTConfig holds MQTT client configuration.
type MQTTConfig struct {
	BrokerURL      string        `mapstructure:"broker_url"`
	ClientID       string        `mapstructure:"client_id"`
	Username       string        `mapstructure:"username"`
	Password       string        `mapstructure:"password"`
	CleanSession   bool          `mapstructure:"clean_session"`
	QoS            byte          `mapstructure:"qos"`
	KeepAlive      time.Duration `mapstructure:"keep_alive"`
	ConnectTimeout time.Duration `mapstructure:"connect_timeout"`
	ReconnectDelay time.Duration `mapstructure:"reconnect_delay"`
	MaxReconnect   int           `mapstructure:"max_reconnect"`
	TLSEnabled     bool          `mapstructure:"tls_enabled"`
	TLSCertFile    string        `mapstructure:"tls_cert_file"`
	TLSKeyFile     string        `mapstructure:"tls_key_file"`
	TLSCAFile      string        `mapstructure:"tls_ca_file"`
	BufferSize     int           `mapstructure:"buffer_size"`
}

// ModbusConfig holds Modbus connection pool configuration.
type ModbusConfig struct {
	MaxConnections    int           `mapstructure:"max_connections"`
	IdleTimeout       time.Duration `mapstructure:"idle_timeout"`
	HealthCheckPeriod time.Duration `mapstructure:"health_check_period"`
	ConnectionTimeout time.Duration `mapstructure:"connection_timeout"`
	RetryAttempts     int           `mapstructure:"retry_attempts"`
	RetryDelay        time.Duration `mapstructure:"retry_delay"`
}

// OPCUAConfig holds OPC UA connection pool configuration.
type OPCUAConfig struct {
	MaxConnections        int           `mapstructure:"max_connections"`
	IdleTimeout           time.Duration `mapstructure:"idle_timeout"`
	HealthCheckPeriod     time.Duration `mapstructure:"health_check_period"`
	ConnectionTimeout     time.Duration `mapstructure:"connection_timeout"`
	RetryAttempts         int           `mapstructure:"retry_attempts"`
	RetryDelay            time.Duration `mapstructure:"retry_delay"`
	DefaultSecurityPolicy string        `mapstructure:"default_security_policy"`
	DefaultSecurityMode   string        `mapstructure:"default_security_mode"`
	DefaultAuthMode       string        `mapstructure:"default_auth_mode"`
}

// S7Config holds S7 connection pool configuration.
type S7Config struct {
	MaxConnections    int           `mapstructure:"max_connections"`
	IdleTimeout       time.Duration `mapstructure:"idle_timeout"`
	HealthCheckPeriod time.Duration `mapstructure:"health_check_period"`
	ConnectionTimeout time.Duration `mapstructure:"connection_timeout"`
	RetryAttempts     int           `mapstructure:"retry_attempts"`
	RetryDelay        time.Duration `mapstructure:"retry_delay"`
	// Circuit breaker configuration
	CBMaxRequests      uint32        `mapstructure:"cb_max_requests"`
	CBInterval         time.Duration `mapstructure:"cb_interval"`
	CBTimeout          time.Duration `mapstructure:"cb_timeout"`
	CBFailureThreshold uint32        `mapstructure:"cb_failure_threshold"`
}

// PollingConfig holds polling service configuration.
type PollingConfig struct {
	WorkerCount     int           `mapstructure:"worker_count"`
	BatchSize       int           `mapstructure:"batch_size"`
	DefaultInterval time.Duration `mapstructure:"default_interval"`
	MaxRetries      int           `mapstructure:"max_retries"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"` // json or console
	Output     string `mapstructure:"output"` // stdout, stderr, or file path
	TimeFormat string `mapstructure:"time_format"`
}

// Load loads configuration from files and environment variables.
func Load() (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Config file search paths
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("/etc/protocol-gateway")

	// Read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found, will use defaults and env vars
	}

	// Environment variable binding
	v.SetEnvPrefix("GATEWAY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Bind specific environment variables
	bindEnvVars(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default configuration values.
func setDefaults(v *viper.Viper) {
	// Environment
	v.SetDefault("environment", "development")
	v.SetDefault("devices_config_path", "./config/devices.yaml")

	// HTTP
	v.SetDefault("http.port", 8080)
	v.SetDefault("http.read_timeout", 10*time.Second)
	v.SetDefault("http.write_timeout", 10*time.Second)
	v.SetDefault("http.idle_timeout", 60*time.Second)

	// API security
	v.SetDefault("api.auth_enabled", false)
	v.SetDefault("api.api_key", "")
	v.SetDefault("api.max_request_body_size", 1048576) // 1MB default
	v.SetDefault("api.allowed_origins", []string{})

	// MQTT
	v.SetDefault("mqtt.broker_url", "tcp://localhost:1883")
	v.SetDefault("mqtt.client_id", "protocol-gateway")
	v.SetDefault("mqtt.clean_session", true)
	v.SetDefault("mqtt.qos", 1)
	v.SetDefault("mqtt.keep_alive", 30*time.Second)
	v.SetDefault("mqtt.connect_timeout", 10*time.Second)
	v.SetDefault("mqtt.reconnect_delay", 5*time.Second)
	v.SetDefault("mqtt.max_reconnect", -1)
	v.SetDefault("mqtt.buffer_size", 10000)

	// Modbus
	v.SetDefault("modbus.max_connections", 100)
	v.SetDefault("modbus.idle_timeout", 5*time.Minute)
	v.SetDefault("modbus.health_check_period", 30*time.Second)
	v.SetDefault("modbus.connection_timeout", 10*time.Second)
	v.SetDefault("modbus.retry_attempts", 3)
	v.SetDefault("modbus.retry_delay", 100*time.Millisecond)

	// OPC UA
	v.SetDefault("opcua.max_connections", 50)
	v.SetDefault("opcua.idle_timeout", 5*time.Minute)
	v.SetDefault("opcua.health_check_period", 30*time.Second)
	v.SetDefault("opcua.connection_timeout", 15*time.Second)
	v.SetDefault("opcua.retry_attempts", 3)
	v.SetDefault("opcua.retry_delay", 500*time.Millisecond)
	v.SetDefault("opcua.default_security_policy", "None")
	v.SetDefault("opcua.default_security_mode", "None")
	v.SetDefault("opcua.default_auth_mode", "Anonymous")

	// S7
	v.SetDefault("s7.max_connections", 100)
	v.SetDefault("s7.idle_timeout", 5*time.Minute)
	v.SetDefault("s7.health_check_period", 30*time.Second)
	v.SetDefault("s7.connection_timeout", 10*time.Second)
	v.SetDefault("s7.retry_attempts", 3)
	v.SetDefault("s7.retry_delay", 5*time.Second)
	// S7 circuit breaker defaults
	v.SetDefault("s7.cb_max_requests", 3)
	v.SetDefault("s7.cb_interval", 10*time.Second)
	v.SetDefault("s7.cb_timeout", 30*time.Second)
	v.SetDefault("s7.cb_failure_threshold", 5)

	// Polling
	v.SetDefault("polling.worker_count", 10)
	v.SetDefault("polling.batch_size", 50)
	v.SetDefault("polling.default_interval", 1*time.Second)
	v.SetDefault("polling.max_retries", 3)
	v.SetDefault("polling.shutdown_timeout", 30*time.Second)

	// Logging
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")
	v.SetDefault("logging.time_format", time.RFC3339)
}

// bindEnvVars binds environment variables to config keys.
func bindEnvVars(v *viper.Viper) {
	// MQTT environment variables
	_ = v.BindEnv("mqtt.broker_url", "MQTT_BROKER_URL")
	_ = v.BindEnv("mqtt.username", "MQTT_USERNAME")
	_ = v.BindEnv("mqtt.password", "MQTT_PASSWORD")
	_ = v.BindEnv("mqtt.client_id", "MQTT_CLIENT_ID")

	// General environment variables
	_ = v.BindEnv("environment", "ENVIRONMENT")
	_ = v.BindEnv("devices_config_path", "DEVICES_CONFIG_PATH")

	// HTTP
	_ = v.BindEnv("http.port", "HTTP_PORT")

	// API security
	_ = v.BindEnv("api.auth_enabled", "API_AUTH_ENABLED")
	_ = v.BindEnv("api.api_key", "API_KEY")
	_ = v.BindEnv("api.max_request_body_size", "API_MAX_REQUEST_BODY_SIZE")

	// Logging
	_ = v.BindEnv("logging.level", "LOG_LEVEL")
	_ = v.BindEnv("logging.format", "LOG_FORMAT")
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.MQTT.BrokerURL == "" {
		return fmt.Errorf("MQTT broker URL is required")
	}
	if c.HTTP.Port <= 0 || c.HTTP.Port > 65535 {
		return fmt.Errorf("invalid HTTP port: %d", c.HTTP.Port)
	}
	if c.Polling.WorkerCount <= 0 {
		return fmt.Errorf("polling worker count must be positive")
	}
	if c.Modbus.MaxConnections <= 0 {
		return fmt.Errorf("modbus max connections must be positive")
	}

	// Check if devices config file exists
	if c.DevicesConfigPath != "" {
		if _, err := os.Stat(c.DevicesConfigPath); os.IsNotExist(err) {
			// This is a warning, not an error - devices can be added dynamically
		}
	}

	return nil
}
