package domain_test

import (
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

func TestDevice_Protocol_Constants(t *testing.T) {
	protocols := []struct {
		protocol domain.Protocol
		expected string
	}{
		{domain.ProtocolModbusTCP, "modbus-tcp"},
		{domain.ProtocolModbusRTU, "modbus-rtu"},
		{domain.ProtocolOPCUA, "opcua"},
		{domain.ProtocolS7, "s7"},
		{domain.ProtocolMQTT, "mqtt"},
	}

	for _, tt := range protocols {
		if string(tt.protocol) != tt.expected {
			t.Errorf("expected protocol '%s', got '%s'", tt.expected, tt.protocol)
		}
	}
}

func TestDeviceStatus_Constants(t *testing.T) {
	statuses := []struct {
		status   domain.DeviceStatus
		expected string
	}{
		{domain.DeviceStatusOnline, "online"},
		{domain.DeviceStatusOffline, "offline"},
		{domain.DeviceStatusConnecting, "connecting"},
		{domain.DeviceStatusError, "error"},
		{domain.DeviceStatusUnknown, "unknown"},
	}

	for _, tt := range statuses {
		if string(tt.status) != tt.expected {
			t.Errorf("expected status '%s', got '%s'", tt.expected, tt.status)
		}
	}
}

func TestDevice_Creation(t *testing.T) {
	now := time.Now()
	device := domain.Device{
		ID:           "plc-001",
		Name:         "Main PLC",
		Description:  "Production line controller",
		Protocol:     domain.ProtocolModbusTCP,
		Enabled:      true,
		PollInterval: 1 * time.Second,
		UNSPrefix:    "plant1/line1/plc1",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if device.ID != "plc-001" {
		t.Errorf("expected ID 'plc-001', got '%s'", device.ID)
	}
	if device.Protocol != domain.ProtocolModbusTCP {
		t.Errorf("expected protocol 'modbus-tcp', got '%s'", device.Protocol)
	}
	if !device.Enabled {
		t.Error("expected device to be enabled")
	}
	if device.PollInterval != 1*time.Second {
		t.Errorf("expected PollInterval 1s, got %v", device.PollInterval)
	}
}

func TestDevice_ConnectionConfig_ModbusTCP(t *testing.T) {
	conn := domain.ConnectionConfig{
		Host:       "192.168.1.100",
		Port:       502,
		Timeout:    5 * time.Second,
		RetryCount: 3,
		RetryDelay: 1 * time.Second,
		SlaveID:    1,
	}

	if conn.Host != "192.168.1.100" {
		t.Errorf("expected Host '192.168.1.100', got '%s'", conn.Host)
	}
	if conn.Port != 502 {
		t.Errorf("expected Port 502, got %d", conn.Port)
	}
	if conn.SlaveID != 1 {
		t.Errorf("expected SlaveID 1, got %d", conn.SlaveID)
	}
}

func TestDevice_ConnectionConfig_ModbusRTU(t *testing.T) {
	conn := domain.ConnectionConfig{
		SerialPort: "/dev/ttyUSB0",
		BaudRate:   9600,
		DataBits:   8,
		Parity:     "N",
		StopBits:   1,
		SlaveID:    1,
		Timeout:    1 * time.Second,
	}

	if conn.SerialPort != "/dev/ttyUSB0" {
		t.Errorf("expected SerialPort '/dev/ttyUSB0', got '%s'", conn.SerialPort)
	}
	if conn.BaudRate != 9600 {
		t.Errorf("expected BaudRate 9600, got %d", conn.BaudRate)
	}
	if conn.Parity != "N" {
		t.Errorf("expected Parity 'N', got '%s'", conn.Parity)
	}
}

func TestDevice_ConnectionConfig_OPCUA(t *testing.T) {
	conn := domain.ConnectionConfig{
		Host:              "192.168.1.50",
		Port:              4840,
		Timeout:           10 * time.Second,
		OPCEndpointURL:    "opc.tcp://192.168.1.50:4840",
		OPCSecurityPolicy: "None",
		OPCSecurityMode:   "None",
	}

	if conn.OPCEndpointURL != "opc.tcp://192.168.1.50:4840" {
		t.Errorf("unexpected OPCEndpointURL: %s", conn.OPCEndpointURL)
	}
}

func TestDevice_ConnectionConfig_S7(t *testing.T) {
	conn := domain.ConnectionConfig{
		Host:    "192.168.1.10",
		Port:    102,
		Timeout: 5 * time.Second,
		S7Rack:  0,
		S7Slot:  1,
	}

	if conn.S7Rack != 0 {
		t.Errorf("expected S7Rack 0, got %d", conn.S7Rack)
	}
	if conn.S7Slot != 1 {
		t.Errorf("expected S7Slot 1, got %d", conn.S7Slot)
	}
}

func TestDevice_WithTags(t *testing.T) {
	device := domain.Device{
		ID:       "dev-1",
		Name:     "Test Device",
		Protocol: domain.ProtocolModbusTCP,
		Tags: []domain.Tag{
			{ID: "tag-1", Name: "Temperature", DataType: domain.DataTypeFloat32},
			{ID: "tag-2", Name: "Pressure", DataType: domain.DataTypeFloat32},
			{ID: "tag-3", Name: "Status", DataType: domain.DataTypeBool},
		},
	}

	if len(device.Tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(device.Tags))
	}

	// Verify tag IDs
	expectedIDs := []string{"tag-1", "tag-2", "tag-3"}
	for i, tag := range device.Tags {
		if tag.ID != expectedIDs[i] {
			t.Errorf("expected tag ID '%s', got '%s'", expectedIDs[i], tag.ID)
		}
	}
}

func TestDevice_Metadata(t *testing.T) {
	device := domain.Device{
		ID:       "dev-1",
		Name:     "Test Device",
		Protocol: domain.ProtocolOPCUA,
		Metadata: map[string]string{
			"location":     "Building A",
			"manufacturer": "Siemens",
			"model":        "S7-1500",
		},
	}

	if device.Metadata["location"] != "Building A" {
		t.Errorf("expected location 'Building A', got '%s'", device.Metadata["location"])
	}
	if device.Metadata["manufacturer"] != "Siemens" {
		t.Errorf("expected manufacturer 'Siemens', got '%s'", device.Metadata["manufacturer"])
	}
}

func TestDevice_ConfigVersioning(t *testing.T) {
	device := domain.Device{
		ID:                   "dev-1",
		Name:                 "Versioned Device",
		Protocol:             domain.ProtocolModbusTCP,
		ConfigVersion:        5,
		ActiveConfigVersion:  4,
		LastKnownGoodVersion: 3,
	}

	if device.ConfigVersion != 5 {
		t.Errorf("expected ConfigVersion 5, got %d", device.ConfigVersion)
	}
	if device.ActiveConfigVersion != 4 {
		t.Errorf("expected ActiveConfigVersion 4, got %d", device.ActiveConfigVersion)
	}
	if device.LastKnownGoodVersion != 3 {
		t.Errorf("expected LastKnownGoodVersion 3, got %d", device.LastKnownGoodVersion)
	}
}

func TestDevice_ZeroValues(t *testing.T) {
	// Test that zero-value device doesn't panic
	var device domain.Device

	if device.ID != "" {
		t.Errorf("expected empty ID, got '%s'", device.ID)
	}
	if device.Protocol != "" {
		t.Errorf("expected empty Protocol, got '%s'", device.Protocol)
	}
	if device.Enabled {
		t.Error("expected Enabled to be false")
	}
	if device.Tags != nil {
		t.Error("expected Tags to be nil")
	}
}

func TestDevice_UNSPrefix_TopicGeneration(t *testing.T) {
	device := domain.Device{
		ID:        "plc-001",
		UNSPrefix: "factory/area1/line2/machine3",
		Tags: []domain.Tag{
			{ID: "temp", TopicSuffix: "temperature"},
			{ID: "press", TopicSuffix: "pressure"},
		},
	}

	// Topic should be UNSPrefix + "/" + TopicSuffix
	expectedTopics := []string{
		"factory/area1/line2/machine3/temperature",
		"factory/area1/line2/machine3/pressure",
	}

	for i, tag := range device.Tags {
		fullTopic := device.UNSPrefix + "/" + tag.TopicSuffix
		if fullTopic != expectedTopics[i] {
			t.Errorf("expected topic '%s', got '%s'", expectedTopics[i], fullTopic)
		}
	}
}
