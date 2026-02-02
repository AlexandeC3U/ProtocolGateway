package domain_test

import (
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

func TestTag_DataType_Constants(t *testing.T) {
	dataTypes := []struct {
		dt       domain.DataType
		expected string
	}{
		{domain.DataTypeBool, "bool"},
		{domain.DataTypeInt16, "int16"},
		{domain.DataTypeUInt16, "uint16"},
		{domain.DataTypeInt32, "int32"},
		{domain.DataTypeUInt32, "uint32"},
		{domain.DataTypeInt64, "int64"},
		{domain.DataTypeUInt64, "uint64"},
		{domain.DataTypeFloat32, "float32"},
		{domain.DataTypeFloat64, "float64"},
		{domain.DataTypeString, "string"},
	}

	for _, tt := range dataTypes {
		if string(tt.dt) != tt.expected {
			t.Errorf("expected DataType '%s', got '%s'", tt.expected, tt.dt)
		}
	}
}

func TestTag_RegisterType_Constants(t *testing.T) {
	registerTypes := []struct {
		rt       domain.RegisterType
		expected string
	}{
		{domain.RegisterTypeCoil, "coil"},
		{domain.RegisterTypeDiscreteInput, "discrete_input"},
		{domain.RegisterTypeHoldingRegister, "holding_register"},
		{domain.RegisterTypeInputRegister, "input_register"},
	}

	for _, tt := range registerTypes {
		if string(tt.rt) != tt.expected {
			t.Errorf("expected RegisterType '%s', got '%s'", tt.expected, tt.rt)
		}
	}
}

func TestTag_ByteOrder_Constants(t *testing.T) {
	byteOrders := []struct {
		bo       domain.ByteOrder
		expected string
	}{
		{domain.ByteOrderBigEndian, "big_endian"},
		{domain.ByteOrderLittleEndian, "little_endian"},
		{domain.ByteOrderMidBigEndian, "mid_big"},
		{domain.ByteOrderMidLitEndian, "mid_little"},
	}

	for _, tt := range byteOrders {
		if string(tt.bo) != tt.expected {
			t.Errorf("expected ByteOrder '%s', got '%s'", tt.expected, tt.bo)
		}
	}
}

func TestTag_AccessMode_Constants(t *testing.T) {
	accessModes := []struct {
		am       domain.AccessMode
		expected string
	}{
		{domain.AccessModeReadOnly, "read"},
		{domain.AccessModeWriteOnly, "write"},
		{domain.AccessModeReadWrite, "readwrite"},
	}

	for _, tt := range accessModes {
		if string(tt.am) != tt.expected {
			t.Errorf("expected AccessMode '%s', got '%s'", tt.expected, tt.am)
		}
	}
}

func TestTag_Creation_Modbus(t *testing.T) {
	tag := domain.Tag{
		ID:            "temperature",
		Name:          "Tank Temperature",
		Description:   "Temperature sensor in main tank",
		Address:       100,
		RegisterType:  domain.RegisterTypeHoldingRegister,
		DataType:      domain.DataTypeFloat32,
		ByteOrder:     domain.ByteOrderBigEndian,
		RegisterCount: 2,
		ScaleFactor:   0.1,
		Offset:        0,
		Unit:          "°C",
		TopicSuffix:   "temperature",
		Enabled:       true,
		AccessMode:    domain.AccessModeReadOnly,
	}

	if tag.ID != "temperature" {
		t.Errorf("expected ID 'temperature', got '%s'", tag.ID)
	}
	if tag.Address != 100 {
		t.Errorf("expected Address 100, got %d", tag.Address)
	}
	if tag.RegisterType != domain.RegisterTypeHoldingRegister {
		t.Errorf("expected RegisterType 'holding_register', got '%s'", tag.RegisterType)
	}
	if tag.DataType != domain.DataTypeFloat32 {
		t.Errorf("expected DataType 'float32', got '%s'", tag.DataType)
	}
	if tag.RegisterCount != 2 {
		t.Errorf("expected RegisterCount 2, got %d", tag.RegisterCount)
	}
}

func TestTag_Creation_OPCUA(t *testing.T) {
	tag := domain.Tag{
		ID:                "pressure",
		Name:              "System Pressure",
		DataType:          domain.DataTypeFloat64,
		OPCNodeID:         "ns=2;s=System.Pressure",
		OPCNamespaceIndex: 2,
		Unit:              "bar",
		TopicSuffix:       "pressure",
		Enabled:           true,
	}

	if tag.OPCNodeID != "ns=2;s=System.Pressure" {
		t.Errorf("expected OPCNodeID 'ns=2;s=System.Pressure', got '%s'", tag.OPCNodeID)
	}
	if tag.OPCNamespaceIndex != 2 {
		t.Errorf("expected OPCNamespaceIndex 2, got %d", tag.OPCNamespaceIndex)
	}
}

func TestTag_Creation_S7(t *testing.T) {
	tag := domain.Tag{
		ID:          "motor_speed",
		Name:        "Motor Speed",
		DataType:    domain.DataTypeInt32,
		S7Area:      domain.S7AreaDB,
		S7DBNumber:  1,
		S7Offset:    0,
		S7Address:   "DB1.DBD0",
		Unit:        "rpm",
		TopicSuffix: "motor/speed",
		Enabled:     true,
	}

	if tag.S7Area != domain.S7AreaDB {
		t.Errorf("expected S7Area 'DB', got '%s'", tag.S7Area)
	}
	if tag.S7DBNumber != 1 {
		t.Errorf("expected S7DBNumber 1, got %d", tag.S7DBNumber)
	}
	if tag.S7Address != "DB1.DBD0" {
		t.Errorf("expected S7Address 'DB1.DBD0', got '%s'", tag.S7Address)
	}
}

func TestTag_Scaling(t *testing.T) {
	tests := []struct {
		name        string
		rawValue    float64
		scaleFactor float64
		offset      float64
		expected    float64
	}{
		{"no scaling", 100, 1, 0, 100},
		{"scale factor only", 100, 0.1, 0, 10},
		{"offset only", 100, 1, -50, 50},
		{"scale and offset", 100, 0.1, 10, 20}, // 100 * 0.1 + 10 = 20
		{"negative scale", 100, -1, 0, -100},
		{"zero scale", 100, 0, 50, 50}, // 100 * 0 + 50 = 50
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag := domain.Tag{
				ScaleFactor: tt.scaleFactor,
				Offset:      tt.offset,
			}

			// Simulate scaling: value = raw * scale + offset
			result := tt.rawValue*tag.ScaleFactor + tag.Offset

			if result != tt.expected {
				t.Errorf("expected %.2f, got %.2f", tt.expected, result)
			}
		})
	}
}

func TestTag_BitPosition(t *testing.T) {
	bitPos := uint8(5)
	tag := domain.Tag{
		ID:          "alarm_bit",
		DataType:    domain.DataTypeBool,
		BitPosition: &bitPos,
	}

	if tag.BitPosition == nil {
		t.Fatal("expected BitPosition to be set")
	}
	if *tag.BitPosition != 5 {
		t.Errorf("expected BitPosition 5, got %d", *tag.BitPosition)
	}
}

func TestTag_PollInterval_Override(t *testing.T) {
	devicePollInterval := 1 * time.Second
	tagPollInterval := 500 * time.Millisecond

	tag := domain.Tag{
		ID:           "fast_tag",
		PollInterval: &tagPollInterval,
	}

	// Tag poll interval should override device poll interval
	effectiveInterval := devicePollInterval
	if tag.PollInterval != nil {
		effectiveInterval = *tag.PollInterval
	}

	if effectiveInterval != 500*time.Millisecond {
		t.Errorf("expected effective interval 500ms, got %v", effectiveInterval)
	}
}

func TestTag_Deadband(t *testing.T) {
	tag := domain.Tag{
		ID:            "temperature",
		DeadbandType:  domain.DeadbandTypeAbsolute,
		DeadbandValue: 0.5,
	}

	if tag.DeadbandType != domain.DeadbandTypeAbsolute {
		t.Errorf("expected DeadbandType 'absolute', got '%s'", tag.DeadbandType)
	}
	if tag.DeadbandValue != 0.5 {
		t.Errorf("expected DeadbandValue 0.5, got %f", tag.DeadbandValue)
	}
}

func TestTag_Priority(t *testing.T) {
	tests := []struct {
		name     string
		priority uint8
		desc     string
	}{
		{"telemetry", 0, "regular data"},
		{"control", 1, "operational data"},
		{"safety", 2, "alarms and safety"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag := domain.Tag{
				ID:       tt.name,
				Priority: tt.priority,
			}

			if tag.Priority != tt.priority {
				t.Errorf("expected priority %d, got %d", tt.priority, tag.Priority)
			}
		})
	}
}

func TestTag_Metadata(t *testing.T) {
	tag := domain.Tag{
		ID: "tagged",
		Metadata: map[string]string{
			"source":     "sensor-123",
			"calibrated": "2024-01-15",
			"range":      "0-100",
		},
	}

	if tag.Metadata["source"] != "sensor-123" {
		t.Errorf("expected source 'sensor-123', got '%s'", tag.Metadata["source"])
	}
	if len(tag.Metadata) != 3 {
		t.Errorf("expected 3 metadata entries, got %d", len(tag.Metadata))
	}
}

func TestTag_ZeroValues(t *testing.T) {
	var tag domain.Tag

	if tag.ID != "" {
		t.Errorf("expected empty ID, got '%s'", tag.ID)
	}
	if tag.Address != 0 {
		t.Errorf("expected Address 0, got %d", tag.Address)
	}
	if tag.ScaleFactor != 0 {
		t.Errorf("expected ScaleFactor 0, got %f", tag.ScaleFactor)
	}
	if tag.Enabled {
		t.Error("expected Enabled to be false")
	}
}

func TestTag_S7Area_Constants(t *testing.T) {
	areas := []struct {
		area     domain.S7Area
		expected string
	}{
		{domain.S7AreaDB, "DB"},
		{domain.S7AreaM, "M"},
		{domain.S7AreaI, "I"},
		{domain.S7AreaQ, "Q"},
		{domain.S7AreaT, "T"},
		{domain.S7AreaC, "C"},
	}

	for _, tt := range areas {
		if string(tt.area) != tt.expected {
			t.Errorf("expected S7Area '%s', got '%s'", tt.expected, tt.area)
		}
	}
}

func TestTag_RegisterCount_MultiRegister(t *testing.T) {
	tests := []struct {
		dataType      domain.DataType
		registerCount uint16
	}{
		{domain.DataTypeBool, 1},
		{domain.DataTypeInt16, 1},
		{domain.DataTypeUInt16, 1},
		{domain.DataTypeInt32, 2},
		{domain.DataTypeUInt32, 2},
		{domain.DataTypeFloat32, 2},
		{domain.DataTypeInt64, 4},
		{domain.DataTypeUInt64, 4},
		{domain.DataTypeFloat64, 4},
	}

	for _, tt := range tests {
		t.Run(string(tt.dataType), func(t *testing.T) {
			tag := domain.Tag{
				DataType:      tt.dataType,
				RegisterCount: tt.registerCount,
			}

			if tag.RegisterCount != tt.registerCount {
				t.Errorf("expected RegisterCount %d for %s, got %d",
					tt.registerCount, tt.dataType, tag.RegisterCount)
			}
		})
	}
}
