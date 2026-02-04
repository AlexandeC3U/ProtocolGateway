//go:build integration
// +build integration

// Package integration provides integration tests that run against real protocol simulators.
package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestConfig holds configuration for integration tests.
type TestConfig struct {
	ModbusHost string
	ModbusPort int
	OPCUAHost  string
	OPCUAPort  int
	S7Host     string
	S7Port     int
	MQTTHost   string
	MQTTPort   int
}

// DefaultConfig returns the default test configuration.
// Override with environment variables.
func DefaultConfig() TestConfig {
	return TestConfig{
		ModbusHost: getEnvOrDefault("TEST_MODBUS_HOST", "localhost"),
		ModbusPort: getEnvOrDefaultInt("TEST_MODBUS_PORT", 5020),
		OPCUAHost:  getEnvOrDefault("TEST_OPCUA_HOST", "localhost"),
		OPCUAPort:  getEnvOrDefaultInt("TEST_OPCUA_PORT", 4840),
		S7Host:     getEnvOrDefault("TEST_S7_HOST", "localhost"),
		S7Port:     getEnvOrDefaultInt("TEST_S7_PORT", 102),
		MQTTHost:   getEnvOrDefault("TEST_MQTT_HOST", "localhost"),
		MQTTPort:   getEnvOrDefaultInt("TEST_MQTT_PORT", 1883),
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvOrDefaultInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		var result int
		if _, err := fmt.Sscanf(val, "%d", &result); err == nil {
			return result
		}
	}
	return defaultVal
}

// ContextWithTestTimeout returns a context with a test timeout.
func ContextWithTestTimeout(t *testing.T) (context.Context, context.CancelFunc) {
	timeout := 30 * time.Second
	if testing.Short() {
		timeout = 5 * time.Second
	}
	return context.WithTimeout(context.Background(), timeout)
}

// SkipIfNoSimulator skips the test if the required simulator is not available.
func SkipIfNoSimulator(t *testing.T, host string, port int) {
	t.Helper()
	// Simple TCP check would go here
	// For now, just log
	t.Logf("Testing against %s:%d", host, port)
}

// SkipIfNoMQTTBroker skips the test if MQTT broker is not available.
func SkipIfNoMQTTBroker(t *testing.T, host string, port int) {
	t.Helper()
	t.Logf("Testing against MQTT broker at %s:%d", host, port)
}

// MQTTBrokerURL returns the MQTT broker URL for testing.
func (c TestConfig) MQTTBrokerURL() string {
	return fmt.Sprintf("tcp://%s:%d", c.MQTTHost, c.MQTTPort)
}

// WaitForCondition waits for a condition to become true.
func WaitForCondition(t *testing.T, condition func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v: %s", timeout, msg)
}
