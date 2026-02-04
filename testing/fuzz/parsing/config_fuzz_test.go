//go:build fuzz
// +build fuzz

// Package parsing provides fuzz tests for configuration parsing.
package parsing

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// =============================================================================
// Configuration Types (mirrors config package)
// =============================================================================

// Config represents the main application configuration.
type Config struct {
	Server  ServerConfig   `yaml:"server"`
	MQTT    MQTTConfig     `yaml:"mqtt"`
	Polling PollingConfig  `yaml:"polling"`
	Logging LoggingConfig  `yaml:"logging"`
	Devices []DeviceConfig `yaml:"devices"`
}

// ServerConfig represents HTTP server configuration.
type ServerConfig struct {
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

// MQTTConfig represents MQTT broker configuration.
type MQTTConfig struct {
	BrokerURL string `yaml:"broker_url"`
	ClientID  string `yaml:"client_id"`
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	TopicRoot string `yaml:"topic_root"`
	QoS       int    `yaml:"qos"`
	Retained  bool   `yaml:"retained"`
}

// PollingConfig represents polling configuration.
type PollingConfig struct {
	DefaultInterval time.Duration `yaml:"default_interval"`
	MinInterval     time.Duration `yaml:"min_interval"`
	MaxInterval     time.Duration `yaml:"max_interval"`
}

// LoggingConfig represents logging configuration.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// DeviceConfig represents a device configuration.
type DeviceConfig struct {
	Name     string            `yaml:"name"`
	Protocol string            `yaml:"protocol"`
	Address  string            `yaml:"address"`
	Port     int               `yaml:"port"`
	UnitID   int               `yaml:"unit_id"`
	Tags     []TagConfig       `yaml:"tags"`
	Options  map[string]string `yaml:"options"`
}

// TagConfig represents a tag configuration.
type TagConfig struct {
	Name      string  `yaml:"name"`
	Address   string  `yaml:"address"`
	DataType  string  `yaml:"data_type"`
	Scale     float64 `yaml:"scale"`
	Offset    float64 `yaml:"offset"`
	ByteOrder string  `yaml:"byte_order"`
}

// =============================================================================
// Fuzz Tests for YAML Parsing
// =============================================================================

// FuzzYAMLConfig fuzzes YAML configuration parsing.
func FuzzYAMLConfig(f *testing.F) {
	// Seed corpus with valid configs
	f.Add([]byte(`
server:
  port: 8080
mqtt:
  broker_url: tcp://localhost:1883
`))

	f.Add([]byte(`
server:
  port: 80
  read_timeout: 30s
  write_timeout: 30s
mqtt:
  broker_url: tcp://broker:1883
  client_id: gateway-1
  qos: 1
polling:
  default_interval: 1s
`))

	// Invalid YAML seed
	f.Add([]byte(`{invalid: yaml: content`))
	f.Add([]byte(`---`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		var cfg Config
		err := yaml.Unmarshal(data, &cfg)

		// We're testing that parsing doesn't panic
		// Both success and error are acceptable outcomes
		_ = err
		_ = cfg
	})
}

// FuzzYAMLDevice fuzzes device configuration parsing.
func FuzzYAMLDevice(f *testing.F) {
	f.Add([]byte(`
name: device1
protocol: modbus
address: 192.168.1.100
port: 502
unit_id: 1
tags:
  - name: temp
    address: "40001"
    data_type: float32
`))

	f.Add([]byte(`
name: test
protocol: opcua
address: opc.tcp://localhost:4840
tags: []
`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var device DeviceConfig
		err := yaml.Unmarshal(data, &device)

		// No panic = success
		_ = err
		_ = device
	})
}

// =============================================================================
// Fuzz Tests for Address Parsing
// =============================================================================

// parseModbusAddress parses a Modbus address string.
// Formats: "40001", "4x0001", "HR0", "%MW0"
func parseModbusAddress(addr string) (registerType string, address int, err error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", 0, fmt.Errorf("empty address")
	}

	// Handle IEC 61131-3 format (%MW, %IW, etc.)
	if strings.HasPrefix(addr, "%") {
		if len(addr) < 3 {
			return "", 0, fmt.Errorf("invalid IEC address: %s", addr)
		}
		prefix := addr[1:3]
		numStr := addr[3:]
		num, err := strconv.Atoi(numStr)
		if err != nil {
			return "", 0, fmt.Errorf("invalid IEC address number: %s", addr)
		}

		switch prefix {
		case "MW", "QW":
			return "holding", num, nil
		case "IW":
			return "input", num, nil
		case "MX", "QX":
			return "coil", num, nil
		case "IX":
			return "discrete", num, nil
		default:
			return "", 0, fmt.Errorf("unknown IEC prefix: %s", prefix)
		}
	}

	// Handle Modicon format (4x, 3x, 1x, 0x)
	if len(addr) >= 2 && (addr[1] == 'x' || addr[1] == 'X') {
		prefix := addr[0]
		numStr := addr[2:]
		num, err := strconv.Atoi(numStr)
		if err != nil {
			return "", 0, fmt.Errorf("invalid Modicon address number: %s", addr)
		}

		switch prefix {
		case '4':
			return "holding", num, nil
		case '3':
			return "input", num, nil
		case '1':
			return "coil", num, nil
		case '0':
			return "discrete", num, nil
		default:
			return "", 0, fmt.Errorf("unknown Modicon prefix: %c", prefix)
		}
	}

	// Handle symbolic format (HR, IR, CO, DI)
	for _, prefix := range []struct {
		name    string
		regType string
	}{
		{"HR", "holding"},
		{"IR", "input"},
		{"CO", "coil"},
		{"DI", "discrete"},
	} {
		if strings.HasPrefix(strings.ToUpper(addr), prefix.name) {
			numStr := addr[len(prefix.name):]
			num, err := strconv.Atoi(numStr)
			if err != nil {
				return "", 0, fmt.Errorf("invalid symbolic address number: %s", addr)
			}
			return prefix.regType, num, nil
		}
	}

	// Handle numeric only (assume holding register with 40001 offset)
	num, err := strconv.Atoi(addr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid address format: %s", addr)
	}

	if num >= 40001 && num <= 49999 {
		return "holding", num - 40001, nil
	} else if num >= 30001 && num <= 39999 {
		return "input", num - 30001, nil
	} else if num >= 10001 && num <= 19999 {
		return "discrete", num - 10001, nil
	} else if num >= 1 && num <= 9999 {
		return "coil", num - 1, nil
	}

	return "", 0, fmt.Errorf("address out of range: %d", num)
}

// FuzzModbusAddress fuzzes Modbus address parsing.
func FuzzModbusAddress(f *testing.F) {
	// Valid formats
	f.Add("40001")
	f.Add("4x0001")
	f.Add("HR0")
	f.Add("%MW0")
	f.Add("30001")
	f.Add("3x0001")
	f.Add("IR0")
	f.Add("%IW0")
	f.Add("10001")
	f.Add("1x0001")
	f.Add("DI0")
	f.Add("%IX0")
	f.Add("1")
	f.Add("CO0")
	f.Add("%MX0")

	// Edge cases
	f.Add("")
	f.Add("0")
	f.Add("-1")
	f.Add("99999")
	f.Add("abc")

	f.Fuzz(func(t *testing.T, addr string) {
		regType, address, err := parseModbusAddress(addr)

		// Test that function doesn't panic and returns consistent results
		if err == nil {
			// Valid parse should have valid register type
			validTypes := map[string]bool{
				"holding":  true,
				"input":    true,
				"coil":     true,
				"discrete": true,
			}
			if !validTypes[regType] {
				t.Errorf("parseModbusAddress(%q) returned invalid type: %s", addr, regType)
			}

			// Address should be non-negative
			if address < 0 {
				t.Errorf("parseModbusAddress(%q) returned negative address: %d", addr, address)
			}
		}
	})
}

// =============================================================================
// Fuzz Tests for OPC UA Node ID Parsing
// =============================================================================

// parseOPCUANodeID parses an OPC UA node ID string.
// Formats: "ns=2;i=1234", "ns=2;s=MyVariable", "i=84", "s=Objects"
func parseOPCUANodeID(nodeID string) (namespace int, idType string, idValue string, err error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return 0, "", "", fmt.Errorf("empty node ID")
	}

	parts := strings.Split(nodeID, ";")
	namespace = 0
	idType = ""
	idValue = ""

	for _, part := range parts {
		if strings.HasPrefix(part, "ns=") {
			nsStr := strings.TrimPrefix(part, "ns=")
			ns, err := strconv.Atoi(nsStr)
			if err != nil {
				return 0, "", "", fmt.Errorf("invalid namespace: %s", nsStr)
			}
			if ns < 0 {
				return 0, "", "", fmt.Errorf("negative namespace: %d", ns)
			}
			namespace = ns
		} else if strings.HasPrefix(part, "i=") {
			idType = "numeric"
			idValue = strings.TrimPrefix(part, "i=")
			// Validate it's actually a number
			if _, err := strconv.Atoi(idValue); err != nil {
				return 0, "", "", fmt.Errorf("invalid numeric ID: %s", idValue)
			}
		} else if strings.HasPrefix(part, "s=") {
			idType = "string"
			idValue = strings.TrimPrefix(part, "s=")
		} else if strings.HasPrefix(part, "g=") {
			idType = "guid"
			idValue = strings.TrimPrefix(part, "g=")
		} else if strings.HasPrefix(part, "b=") {
			idType = "opaque"
			idValue = strings.TrimPrefix(part, "b=")
		}
	}

	if idType == "" {
		return 0, "", "", fmt.Errorf("no identifier found in node ID: %s", nodeID)
	}

	return namespace, idType, idValue, nil
}

// FuzzOPCUANodeID fuzzes OPC UA node ID parsing.
func FuzzOPCUANodeID(f *testing.F) {
	// Valid formats
	f.Add("ns=2;i=1234")
	f.Add("ns=2;s=MyVariable")
	f.Add("i=84")
	f.Add("s=Objects")
	f.Add("ns=0;i=85")
	f.Add("ns=3;g=12345678-1234-1234-1234-123456789012")
	f.Add("ns=1;b=SGVsbG8gV29ybGQ=")

	// Edge cases
	f.Add("")
	f.Add("ns=")
	f.Add("i=")
	f.Add("invalid")

	f.Fuzz(func(t *testing.T, nodeID string) {
		namespace, idType, idValue, err := parseOPCUANodeID(nodeID)

		// Test that function doesn't panic
		if err == nil {
			// Valid parse should have valid ID type
			validTypes := map[string]bool{
				"numeric": true,
				"string":  true,
				"guid":    true,
				"opaque":  true,
			}
			if !validTypes[idType] {
				t.Errorf("parseOPCUANodeID(%q) returned invalid type: %s", nodeID, idType)
			}

			// Namespace should be non-negative
			if namespace < 0 {
				t.Errorf("parseOPCUANodeID(%q) returned negative namespace: %d", nodeID, namespace)
			}

			// ID value should not be empty
			if idValue == "" && idType != "" {
				// Empty value might be valid for certain cases
			}
		}
	})
}

// =============================================================================
// Fuzz Tests for Duration Parsing
// =============================================================================

// parseDuration parses a duration string with extended formats.
// Supports: "1s", "1m", "1h", "500ms", "1m30s"
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}

	// Try standard Go duration parsing first
	d, err := time.ParseDuration(s)
	if err == nil {
		return d, nil
	}

	// Try integer (assume seconds)
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * time.Second, nil
	}

	return 0, fmt.Errorf("invalid duration: %s", s)
}

// FuzzDuration fuzzes duration parsing.
func FuzzDuration(f *testing.F) {
	// Valid durations
	f.Add("1s")
	f.Add("1m")
	f.Add("1h")
	f.Add("500ms")
	f.Add("1m30s")
	f.Add("10")
	f.Add("0")

	// Edge cases
	f.Add("")
	f.Add("-1s")
	f.Add("1d") // Not valid Go duration
	f.Add("abc")

	f.Fuzz(func(t *testing.T, s string) {
		d, err := parseDuration(s)

		// Test that function doesn't panic
		if err == nil {
			// Valid duration should be non-negative for most use cases
			// (but negative durations are valid in Go)
			_ = d
		}
	})
}

// =============================================================================
// Fuzz Tests for Topic Template Parsing
// =============================================================================

// parseTopicTemplate parses an MQTT topic template.
// Supports: {device}, {tag}, {protocol}
func parseTopicTemplate(template string, vars map[string]string) string {
	result := template
	for key, value := range vars {
		placeholder := "{" + key + "}"
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// FuzzTopicTemplate fuzzes topic template parsing.
func FuzzTopicTemplate(f *testing.F) {
	f.Add("gateway/{device}/{tag}", "device1", "temp")
	f.Add("data/{device}/values/{tag}", "sensor1", "temperature")
	f.Add("{device}", "test", "")
	f.Add("simple/topic", "", "")

	f.Fuzz(func(t *testing.T, template, device, tag string) {
		vars := map[string]string{
			"device": device,
			"tag":    tag,
		}

		result := parseTopicTemplate(template, vars)

		// Test that function doesn't panic
		// Result should not contain un-substituted variables that were provided
		if device != "" && strings.Contains(result, "{device}") {
			t.Errorf("parseTopicTemplate didn't substitute {device}: %s -> %s", template, result)
		}
		if tag != "" && strings.Contains(result, "{tag}") {
			t.Errorf("parseTopicTemplate didn't substitute {tag}: %s -> %s", template, result)
		}
	})
}

// =============================================================================
// Fuzz Tests for Connection String Parsing
// =============================================================================

// parseConnectionString parses a connection string.
// Formats: "host:port", "tcp://host:port", "host"
func parseConnectionString(connStr string) (scheme, host string, port int, err error) {
	connStr = strings.TrimSpace(connStr)
	if connStr == "" {
		return "", "", 0, fmt.Errorf("empty connection string")
	}

	// Check for scheme
	scheme = "tcp" // default
	if idx := strings.Index(connStr, "://"); idx != -1 {
		scheme = connStr[:idx]
		connStr = connStr[idx+3:]
	}

	// Split host and port
	if idx := strings.LastIndex(connStr, ":"); idx != -1 {
		host = connStr[:idx]
		portStr := connStr[idx+1:]
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return "", "", 0, fmt.Errorf("invalid port: %s", portStr)
		}
		if p < 0 || p > 65535 {
			return "", "", 0, fmt.Errorf("port out of range: %d", p)
		}
		port = p
	} else {
		host = connStr
		port = 0
	}

	if host == "" {
		return "", "", 0, fmt.Errorf("empty host")
	}

	return scheme, host, port, nil
}

// FuzzConnectionString fuzzes connection string parsing.
func FuzzConnectionString(f *testing.F) {
	f.Add("localhost:502")
	f.Add("tcp://192.168.1.100:502")
	f.Add("opc.tcp://server:4840")
	f.Add("localhost")
	f.Add("192.168.1.1:80")

	// Edge cases
	f.Add("")
	f.Add(":")
	f.Add(":502")
	f.Add("host:-1")
	f.Add("host:99999")

	f.Fuzz(func(t *testing.T, connStr string) {
		scheme, host, port, err := parseConnectionString(connStr)

		// Test that function doesn't panic
		if err == nil {
			// Valid parse should have valid components
			if scheme == "" {
				t.Errorf("parseConnectionString(%q) returned empty scheme", connStr)
			}
			if host == "" {
				t.Errorf("parseConnectionString(%q) returned empty host", connStr)
			}
			if port < 0 || port > 65535 {
				t.Errorf("parseConnectionString(%q) returned invalid port: %d", connStr, port)
			}
		}
	})
}
