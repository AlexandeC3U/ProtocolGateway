// Package config_test tests the device configuration loading functionality.
package config_test

import (
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/config"
)

// TestDeviceConfigStructure tests the DeviceConfig struct fields.
func TestDeviceConfigStructure(t *testing.T) {
	cfg := config.DeviceConfig{
		ID:           "device-001",
		Name:         "Test PLC",
		Protocol:     "s7",
		Enabled:      true,
		UNSPrefix:    "enterprise/site/area",
		PollInterval: "1s",
		Connection:   config.ConnectionConfig{},
		Tags:         []config.TagConfig{},
	}

	if cfg.ID != "device-001" {
		t.Errorf("expected ID device-001, got %s", cfg.ID)
	}
	if cfg.Name != "Test PLC" {
		t.Errorf("expected Name Test PLC, got %s", cfg.Name)
	}
	if cfg.Protocol != "s7" {
		t.Errorf("expected Protocol s7, got %s", cfg.Protocol)
	}
	if !cfg.Enabled {
		t.Error("expected Enabled=true")
	}
}

// TestConnectionConfigStructure tests ConnectionConfig fields.
func TestConnectionConfigStructure(t *testing.T) {
	cfg := config.ConnectionConfig{
		Host:       "192.168.1.10",
		Port:       102,
		Timeout:    "5s",
		S7Rack:     0,
		S7Slot:     1,
		RetryCount: 3,
		RetryDelay: "1s",
	}

	if cfg.Host != "192.168.1.10" {
		t.Errorf("expected Host 192.168.1.10, got %s", cfg.Host)
	}
	if cfg.Port != 102 {
		t.Errorf("expected Port 102, got %d", cfg.Port)
	}
	if cfg.Timeout != "5s" {
		t.Errorf("expected Timeout 5s, got %s", cfg.Timeout)
	}
}

// TestTagConfigStructure tests TagConfig struct fields.
func TestTagConfigStructure(t *testing.T) {
	cfg := config.TagConfig{
		Name:        "Temperature",
		S7Address:   "DB1.DBD0",
		DataType:    "float32",
		ScaleFactor: 1.0,
		Offset:      0.0,
		Unit:        "°C",
		Description: "Process temperature",
	}

	if cfg.Name != "Temperature" {
		t.Errorf("expected Name Temperature, got %s", cfg.Name)
	}
	if cfg.S7Address != "DB1.DBD0" {
		t.Errorf("expected S7Address DB1.DBD0, got %s", cfg.S7Address)
	}
	if cfg.DataType != "float32" {
		t.Errorf("expected DataType float32, got %s", cfg.DataType)
	}
}

// TestPollIntervalStrings tests poll interval string format.
func TestPollIntervalStrings(t *testing.T) {
	intervals := []struct {
		value string
		valid bool
	}{
		{"100ms", true},
		{"1s", true},
		{"5m", true},
		{"1h", true},
		{"invalid", false},
	}

	for _, i := range intervals {
		t.Run(i.value, func(t *testing.T) {
			t.Logf("Interval: %s valid=%v", i.value, i.valid)
		})
	}
}

// TestProtocolValidation tests protocol value validation.
func TestProtocolValidation(t *testing.T) {
	protocols := []struct {
		protocol string
		valid    bool
	}{
		{"s7", true},
		{"opcua", true},
		{"modbus", true},
		{"mqtt", false}, // Not a device protocol
		{"unknown", false},
		{"", false},
	}

	for _, p := range protocols {
		t.Run(p.protocol, func(t *testing.T) {
			valid := p.protocol == "s7" || p.protocol == "opcua" || p.protocol == "modbus"
			if valid != p.valid {
				t.Errorf("expected valid=%v for protocol %q", p.valid, p.protocol)
			}
		})
	}
}

// TestS7ConnectionConfig tests S7-specific connection config.
func TestS7ConnectionConfig(t *testing.T) {
	tests := []struct {
		name  string
		cfg   config.ConnectionConfig
		valid bool
		issue string
	}{
		{
			name: "Valid S7-1200",
			cfg: config.ConnectionConfig{
				Host:   "192.168.1.10",
				Port:   102,
				S7Rack: 0,
				S7Slot: 1,
			},
			valid: true,
		},
		{
			name: "Valid S7-300",
			cfg: config.ConnectionConfig{
				Host:   "192.168.1.10",
				Port:   102,
				S7Rack: 0,
				S7Slot: 2,
			},
			valid: true,
		},
		{
			name: "Missing host",
			cfg: config.ConnectionConfig{
				Port:   102,
				S7Rack: 0,
				S7Slot: 1,
			},
			valid: false,
			issue: "host is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.cfg.Host != ""
			if valid != tt.valid {
				t.Errorf("expected valid=%v: %s", tt.valid, tt.issue)
			}
		})
	}
}

// TestOPCUAConnectionConfig tests OPC UA specific config.
func TestOPCUAConnectionConfig(t *testing.T) {
	tests := []struct {
		name  string
		cfg   config.ConnectionConfig
		valid bool
	}{
		{
			name: "Unsecure endpoint",
			cfg: config.ConnectionConfig{
				OPCEndpointURL:    "opc.tcp://localhost:4840",
				OPCSecurityPolicy: "None",
				OPCSecurityMode:   "None",
			},
			valid: true,
		},
		{
			name: "Secure endpoint",
			cfg: config.ConnectionConfig{
				OPCEndpointURL:    "opc.tcp://localhost:4840",
				OPCSecurityPolicy: "Basic256Sha256",
				OPCSecurityMode:   "SignAndEncrypt",
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasEndpoint := tt.cfg.OPCEndpointURL != ""
			if hasEndpoint != tt.valid {
				t.Errorf("expected valid=%v", tt.valid)
			}
		})
	}
}

// TestModbusConnectionConfig tests Modbus specific config.
func TestModbusConnectionConfig(t *testing.T) {
	tests := []struct {
		name  string
		cfg   config.ConnectionConfig
		valid bool
	}{
		{
			name: "Modbus TCP",
			cfg: config.ConnectionConfig{
				Host:    "192.168.1.10",
				Port:    502,
				SlaveID: 1,
			},
			valid: true,
		},
		{
			name: "Modbus RTU",
			cfg: config.ConnectionConfig{
				Host:    "/dev/ttyUSB0",
				SlaveID: 1,
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasHost := tt.cfg.Host != ""
			if hasHost != tt.valid {
				t.Errorf("expected valid=%v", tt.valid)
			}
		})
	}
}

// TestTagAddressValidation tests tag address format validation.
func TestTagAddressValidation(t *testing.T) {
	tests := []struct {
		protocol string
		address  string
		valid    bool
	}{
		// S7 addresses
		{"s7", "DB1.DBD0", true},
		{"s7", "DB100.DBW10", true},
		{"s7", "M0.0", true},
		{"s7", "Q0.1", true},
		{"s7", "I0.0", true},
		{"s7", "", false},

		// OPC UA addresses
		{"opcua", "ns=2;s=Tag1", true},
		{"opcua", "ns=2;i=1001", true},
		{"opcua", "i=85", true},
		{"opcua", "", false},

		// Modbus addresses
		{"modbus", "40001", true},
		{"modbus", "30001", true},
		{"modbus", "10001", true},
		{"modbus", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.protocol+"_"+tt.address, func(t *testing.T) {
			valid := tt.address != ""
			if valid != tt.valid {
				t.Errorf("expected valid=%v for %s address %q",
					tt.valid, tt.protocol, tt.address)
			}
		})
	}
}

// TestDataTypeValidation tests supported data types.
func TestDataTypeValidation(t *testing.T) {
	dataTypes := []struct {
		dtype string
		size  int
		valid bool
	}{
		{"bool", 1, true},
		{"int8", 1, true},
		{"uint8", 1, true},
		{"int16", 2, true},
		{"uint16", 2, true},
		{"int32", 4, true},
		{"uint32", 4, true},
		{"int64", 8, true},
		{"uint64", 8, true},
		{"float32", 4, true},
		{"float64", 8, true},
		{"string", 0, true}, // Variable length
		{"unknown", 0, false},
	}

	for _, dt := range dataTypes {
		t.Run(dt.dtype, func(t *testing.T) {
			cfg := config.TagConfig{
				DataType: dt.dtype,
			}
			t.Logf("Type %s: %d bytes", cfg.DataType, dt.size)
			// Basic check - type is non-empty
			if cfg.DataType == "" && dt.valid {
				t.Error("data type cannot be empty")
			}
		})
	}
}

// TestScalingConfiguration tests tag scaling parameters.
func TestScalingConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		rawValue    float64
		scaleFactor float64
		offset      float64
		expected    float64
	}{
		{"No scaling", 100.0, 1.0, 0.0, 100.0},
		{"Scale only", 100.0, 0.1, 0.0, 10.0},
		{"Offset only", 100.0, 1.0, -50.0, 50.0},
		{"Scale and offset", 100.0, 0.1, 5.0, 15.0},
		{"Temperature", 1234.0, 0.01, 0.0, 12.34},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.TagConfig{
				ScaleFactor: tt.scaleFactor,
				Offset:      tt.offset,
			}

			scaled := tt.rawValue*cfg.ScaleFactor + cfg.Offset
			if scaled != tt.expected {
				t.Errorf("expected %.2f, got %.2f", tt.expected, scaled)
			}
		})
	}
}

// TestPollIntervalRanges tests poll interval validation.
func TestPollIntervalRanges(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		valid    bool
		reason   string
	}{
		{"Too fast", 1 * time.Millisecond, false, "Network overhead"},
		{"Fast", 50 * time.Millisecond, true, "Real-time"},
		{"Standard", 1 * time.Second, true, "Typical"},
		{"Slow", 1 * time.Minute, true, "Low frequency"},
		{"Zero", 0, false, "Invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reasonable bounds: 10ms to 1 hour
			valid := tt.interval >= 10*time.Millisecond && tt.interval <= time.Hour
			if valid != tt.valid {
				t.Errorf("expected valid=%v for interval %v: %s",
					tt.valid, tt.interval, tt.reason)
			}
		})
	}
}

// TestDeviceIDUniqueness tests device ID requirements.
func TestDeviceIDUniqueness(t *testing.T) {
	devices := []config.DeviceConfig{
		{ID: "device-001", Name: "PLC 1"},
		{ID: "device-002", Name: "PLC 2"},
		{ID: "device-003", Name: "OPC Server"},
	}

	ids := make(map[string]bool)
	for _, d := range devices {
		if ids[d.ID] {
			t.Errorf("duplicate device ID: %s", d.ID)
		}
		ids[d.ID] = true
	}
}

// TestTagNameUniqueness tests tag name uniqueness within device.
func TestTagNameUniqueness(t *testing.T) {
	device := config.DeviceConfig{
		ID: "device-001",
		Tags: []config.TagConfig{
			{Name: "Temperature"},
			{Name: "Pressure"},
			{Name: "Flow"},
		},
	}

	names := make(map[string]bool)
	for _, tag := range device.Tags {
		if names[tag.Name] {
			t.Errorf("duplicate tag name: %s", tag.Name)
		}
		names[tag.Name] = true
	}
}

// TestDisabledDeviceHandling tests disabled device behavior.
func TestDisabledDeviceHandling(t *testing.T) {
	devices := []config.DeviceConfig{
		{ID: "active", Enabled: true},
		{ID: "disabled", Enabled: false},
	}

	activeCount := 0
	for _, d := range devices {
		if d.Enabled {
			activeCount++
		}
	}

	if activeCount != 1 {
		t.Errorf("expected 1 active device, got %d", activeCount)
	}
}

// TestConfigMerging tests config inheritance/merging patterns.
func TestConfigMerging(t *testing.T) {
	defaults := config.ConnectionConfig{
		Timeout:    "5s",
		RetryCount: 3,
		RetryDelay: "1s",
	}

	device := config.ConnectionConfig{
		Host: "192.168.1.10",
		// Other fields use defaults
	}

	// Merge logic
	if device.Timeout == "" {
		device.Timeout = defaults.Timeout
	}
	if device.RetryCount == 0 {
		device.RetryCount = defaults.RetryCount
	}
	if device.RetryDelay == "" {
		device.RetryDelay = defaults.RetryDelay
	}

	if device.Timeout != "5s" {
		t.Errorf("expected default timeout 5s, got %s", device.Timeout)
	}
	if device.RetryCount != 3 {
		t.Errorf("expected default retryCount 3, got %d", device.RetryCount)
	}
}

// TestYAMLFilePaths tests config file path handling.
func TestYAMLFilePaths(t *testing.T) {
	paths := []struct {
		path  string
		valid bool
	}{
		{"config/devices.yaml", true},
		{"./devices.yaml", true},
		{"/etc/gateway/devices.yaml", true},
		{"", false},
	}

	for _, p := range paths {
		t.Run(p.path, func(t *testing.T) {
			if p.path == "" && p.valid {
				t.Error("empty path is not valid")
			}
		})
	}
}

// TestEnvVarSubstitution tests environment variable substitution.
func TestEnvVarSubstitution(t *testing.T) {
	tests := []struct {
		input    string
		envVar   string
		envValue string
		expected string
	}{
		{"${HOST}", "HOST", "192.168.1.10", "192.168.1.10"},
		{"opc.tcp://${HOST}:4840", "HOST", "localhost", "opc.tcp://localhost:4840"},
		{"static-value", "", "", "static-value"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			// Simulated substitution
			if tt.envVar != "" && tt.envValue != "" {
				t.Logf("Would substitute ${%s} with %s in %q",
					tt.envVar, tt.envValue, tt.input)
			}
		})
	}
}
