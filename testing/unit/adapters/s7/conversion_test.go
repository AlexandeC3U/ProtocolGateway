// Package s7_test provides unit tests for S7 conversion functions.
// NOTE: These tests verify S7 conversion logic patterns. The actual internal
// conversion functions are unexported methods on the Client struct.
// For full internal testing, add tests to internal/adapter/s7/conversion_test.go
package s7_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

// =============================================================================
// S7 Data Type Constants Tests
// =============================================================================

// TestS7DataTypeConstants verifies S7-compatible data type constants.
func TestS7DataTypeConstants(t *testing.T) {
	// S7 PLCs use these standard data types
	types := []domain.DataType{
		domain.DataTypeBool,    // BOOL - 1 bit
		domain.DataTypeInt16,   // INT - 16 bits
		domain.DataTypeUInt16,  // WORD - 16 bits unsigned
		domain.DataTypeInt32,   // DINT - 32 bits
		domain.DataTypeUInt32,  // DWORD - 32 bits unsigned
		domain.DataTypeInt64,   // LINT - 64 bits (S7-1500)
		domain.DataTypeUInt64,  // LWORD - 64 bits unsigned (S7-1500)
		domain.DataTypeFloat32, // REAL - 32 bits IEEE 754
		domain.DataTypeFloat64, // LREAL - 64 bits IEEE 754 (S7-1500)
	}

	for _, dt := range types {
		if dt == "" {
			t.Error("data type constant should not be empty")
		}
	}
}

// TestS7AreaConstants verifies S7 memory area constants.
func TestS7AreaConstants(t *testing.T) {
	areas := []domain.S7Area{
		domain.S7AreaDB, // Data Blocks
		domain.S7AreaM,  // Merker/Flags
		domain.S7AreaI,  // Inputs
		domain.S7AreaQ,  // Outputs
		domain.S7AreaT,  // Timers
		domain.S7AreaC,  // Counters
	}

	for _, area := range areas {
		if area == "" {
			t.Error("S7 area constant should not be empty")
		}
	}
}

// =============================================================================
// S7 Tag Configuration Tests
// =============================================================================

// TestTagConfiguration_S7 verifies S7 tag configuration fields.
func TestTagConfiguration_S7(t *testing.T) {
	tag := domain.Tag{
		ID:          "s7-temp",
		Name:        "Temperature",
		DataType:    domain.DataTypeFloat32,
		S7Area:      domain.S7AreaDB,
		S7DBNumber:  1,
		S7Offset:    0,
		S7BitOffset: 0,
		S7Address:   "DB1.DBD0",
		ScaleFactor: 0.1,
		Offset:      0,
		Enabled:     true,
	}

	if tag.ID != "s7-temp" {
		t.Error("ID not set correctly")
	}
	if tag.Name != "Temperature" {
		t.Error("Name not set correctly")
	}
	if tag.DataType != domain.DataTypeFloat32 {
		t.Error("DataType not set correctly")
	}
	if tag.S7Area != domain.S7AreaDB {
		t.Error("S7Area not set correctly")
	}
	if tag.S7DBNumber != 1 {
		t.Error("S7DBNumber not set correctly")
	}
	if tag.S7Offset != 0 {
		t.Error("S7Offset not set correctly")
	}
	if tag.S7BitOffset != 0 {
		t.Error("S7BitOffset not set correctly")
	}
	if tag.S7Address != "DB1.DBD0" {
		t.Error("S7Address not set correctly")
	}
	if tag.ScaleFactor != 0.1 {
		t.Error("ScaleFactor not set correctly")
	}
	if tag.Offset != 0 {
		t.Error("Offset not set correctly")
	}
	if !tag.Enabled {
		t.Error("Enabled not set correctly")
	}
}

// TestTagConfiguration_S7_BoolTag verifies boolean tag configuration.
func TestTagConfiguration_S7_BoolTag(t *testing.T) {
	tag := domain.Tag{
		ID:          "s7-running",
		Name:        "Motor Running",
		DataType:    domain.DataTypeBool,
		S7Area:      domain.S7AreaDB,
		S7DBNumber:  10,
		S7Offset:    4,
		S7BitOffset: 3, // Bit 3 of byte 4
		S7Address:   "DB10.DBX4.3",
		Enabled:     true,
	}

	if tag.ID != "s7-running" {
		t.Errorf("expected ID 's7-running', got %s", tag.ID)
	}
	if tag.Name != "Motor Running" {
		t.Errorf("expected Name 'Motor Running', got %s", tag.Name)
	}
	if tag.DataType != domain.DataTypeBool {
		t.Errorf("expected DataType bool, got %s", tag.DataType)
	}
	if tag.S7Area != domain.S7AreaDB {
		t.Errorf("expected S7Area DB, got %s", tag.S7Area)
	}
	if tag.S7DBNumber != 10 {
		t.Errorf("expected S7DBNumber 10, got %d", tag.S7DBNumber)
	}
	if tag.S7BitOffset != 3 {
		t.Errorf("expected BitOffset 3, got %d", tag.S7BitOffset)
	}
	if tag.S7Offset != 4 {
		t.Errorf("expected Offset 4, got %d", tag.S7Offset)
	}
	if tag.S7Address != "DB10.DBX4.3" {
		t.Errorf("expected S7Address 'DB10.DBX4.3', got %s", tag.S7Address)
	}
	if !tag.Enabled {
		t.Error("expected Enabled to be true")
	}
}

// TestTagConfiguration_S7_MerkerArea verifies Merker area tag configuration.
func TestTagConfiguration_S7_MerkerArea(t *testing.T) {
	tag := domain.Tag{
		ID:          "s7-flag",
		Name:        "Status Flag",
		DataType:    domain.DataTypeBool,
		S7Area:      domain.S7AreaM,
		S7Offset:    0,
		S7BitOffset: 0,
		S7Address:   "M0.0",
		Enabled:     true,
	}

	if tag.ID != "s7-flag" {
		t.Errorf("expected ID 's7-flag', got %s", tag.ID)
	}
	if tag.Name != "Status Flag" {
		t.Errorf("expected Name 'Status Flag', got %s", tag.Name)
	}
	if tag.DataType != domain.DataTypeBool {
		t.Errorf("expected DataType bool, got %s", tag.DataType)
	}
	if tag.S7Area != domain.S7AreaM {
		t.Errorf("expected S7AreaM, got %s", tag.S7Area)
	}
	if tag.S7Offset != 0 {
		t.Errorf("expected S7Offset 0, got %d", tag.S7Offset)
	}
	if tag.S7BitOffset != 0 {
		t.Errorf("expected S7BitOffset 0, got %d", tag.S7BitOffset)
	}
	if tag.S7Address != "M0.0" {
		t.Errorf("expected S7Address 'M0.0', got %s", tag.S7Address)
	}
	if !tag.Enabled {
		t.Error("expected Enabled to be true")
	}
}

// =============================================================================
// S7 Value Parsing Tests (Big Endian - S7 native byte order)
// =============================================================================

// TestS7Int16Parsing verifies int16 parsing from S7 bytes (big endian).
func TestS7Int16Parsing(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected int16
	}{
		{"positive_small", []byte{0x00, 0x64}, 100},
		{"positive_large", []byte{0x03, 0xE8}, 1000},
		{"negative_small", []byte{0xFF, 0x9C}, -100},
		{"negative_large", []byte{0xFC, 0x18}, -1000},
		{"zero", []byte{0x00, 0x00}, 0},
		{"max_int16", []byte{0x7F, 0xFF}, 32767},
		{"min_int16", []byte{0x80, 0x00}, -32768},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// S7 uses big endian byte order natively
			result := int16(binary.BigEndian.Uint16(tt.data))
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestS7UInt16Parsing verifies uint16 (WORD) parsing from S7 bytes.
func TestS7UInt16Parsing(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected uint16
	}{
		{"zero", []byte{0x00, 0x00}, 0},
		{"small", []byte{0x00, 0x64}, 100},
		{"medium", []byte{0x03, 0xE8}, 1000},
		{"large", []byte{0x27, 0x10}, 10000},
		{"max_word", []byte{0xFF, 0xFF}, 65535},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := binary.BigEndian.Uint16(tt.data)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestS7Int32Parsing verifies int32 (DINT) parsing from S7 bytes.
func TestS7Int32Parsing(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected int32
	}{
		{"zero", []byte{0x00, 0x00, 0x00, 0x00}, 0},
		{"positive", []byte{0x00, 0x01, 0x86, 0xA0}, 100000},
		{"negative", []byte{0xFF, 0xFE, 0x79, 0x60}, -100000},
		{"max_dint", []byte{0x7F, 0xFF, 0xFF, 0xFF}, 2147483647},
		{"min_dint", []byte{0x80, 0x00, 0x00, 0x00}, -2147483648},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := int32(binary.BigEndian.Uint32(tt.data))
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestS7UInt32Parsing verifies uint32 (DWORD) parsing from S7 bytes.
func TestS7UInt32Parsing(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected uint32
	}{
		{"zero", []byte{0x00, 0x00, 0x00, 0x00}, 0},
		{"small", []byte{0x00, 0x00, 0x00, 0x64}, 100},
		{"medium", []byte{0x00, 0x01, 0x86, 0xA0}, 100000},
		{"large", []byte{0x3B, 0x9A, 0xCA, 0x00}, 1000000000},
		{"max_dword", []byte{0xFF, 0xFF, 0xFF, 0xFF}, 4294967295},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := binary.BigEndian.Uint32(tt.data)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestS7Int64Parsing verifies int64 (LINT) parsing from S7 bytes.
func TestS7Int64Parsing(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected int64
	}{
		{"zero", []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 0},
		{"positive", []byte{0x00, 0x00, 0x00, 0x02, 0x54, 0x0B, 0xE4, 0x00}, 10000000000},
		{"negative", []byte{0xFF, 0xFF, 0xFF, 0xFD, 0xAB, 0xF4, 0x1C, 0x00}, -10000000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := int64(binary.BigEndian.Uint64(tt.data))
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestS7UInt64Parsing verifies uint64 (LWORD) parsing from S7 bytes.
func TestS7UInt64Parsing(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected uint64
	}{
		{"zero", []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 0},
		{"small", []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x64}, 100},
		{"large", []byte{0x00, 0x00, 0x00, 0x02, 0x54, 0x0B, 0xE4, 0x00}, 10000000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := binary.BigEndian.Uint64(tt.data)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestS7Float32Parsing verifies float32 (REAL) parsing from S7 bytes.
func TestS7Float32Parsing(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected float32
	}{
		{"zero", []byte{0x00, 0x00, 0x00, 0x00}, 0.0},
		{"one", []byte{0x3F, 0x80, 0x00, 0x00}, 1.0},
		{"negative_one", []byte{0xBF, 0x80, 0x00, 0x00}, -1.0},
		{"pi_approx", []byte{0x40, 0x49, 0x0F, 0xDB}, float32(3.1415927)},
		{"temperature", []byte{0x42, 0xC8, 0x00, 0x00}, 100.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bits := binary.BigEndian.Uint32(tt.data)
			result := math.Float32frombits(bits)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestS7Float64Parsing verifies float64 (LREAL) parsing from S7 bytes.
func TestS7Float64Parsing(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected float64
	}{
		{"zero", []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 0.0},
		{"one", []byte{0x3F, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 1.0},
		{"negative_one", []byte{0xBF, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, -1.0},
		{"pi", []byte{0x40, 0x09, 0x21, 0xFB, 0x54, 0x44, 0x2D, 0x18}, 3.141592653589793},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bits := binary.BigEndian.Uint64(tt.data)
			result := math.Float64frombits(bits)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestS7BoolParsing verifies boolean parsing with bit position.
func TestS7BoolParsing(t *testing.T) {
	tests := []struct {
		name      string
		data      byte
		bitOffset int
		expected  bool
	}{
		{"bit0_set", 0x01, 0, true},
		{"bit0_clear", 0x00, 0, false},
		{"bit1_set", 0x02, 1, true},
		{"bit1_clear", 0x01, 1, false},
		{"bit7_set", 0x80, 7, true},
		{"bit7_clear", 0x7F, 7, false},
		{"bit3_in_mixed", 0x0F, 3, true},
		{"bit4_in_mixed", 0x0F, 4, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := (tt.data & (1 << tt.bitOffset)) != 0
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// =============================================================================
// S7 Value Encoding Tests (Write Operations)
// =============================================================================

// TestS7Int16Encoding verifies int16 encoding to S7 bytes.
func TestS7Int16Encoding(t *testing.T) {
	tests := []struct {
		name     string
		value    int16
		expected []byte
	}{
		{"positive", 100, []byte{0x00, 0x64}},
		{"negative", -100, []byte{0xFF, 0x9C}},
		{"zero", 0, []byte{0x00, 0x00}},
		{"max", 32767, []byte{0x7F, 0xFF}},
		{"min", -32768, []byte{0x80, 0x00}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := make([]byte, 2)
			binary.BigEndian.PutUint16(result, uint16(tt.value))
			if !bytesEqual(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestS7UInt16Encoding verifies uint16 (WORD) encoding to S7 bytes.
func TestS7UInt16Encoding(t *testing.T) {
	tests := []struct {
		name     string
		value    uint16
		expected []byte
	}{
		{"zero", 0, []byte{0x00, 0x00}},
		{"small", 100, []byte{0x00, 0x64}},
		{"medium", 1000, []byte{0x03, 0xE8}},
		{"max", 65535, []byte{0xFF, 0xFF}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := make([]byte, 2)
			binary.BigEndian.PutUint16(result, tt.value)
			if !bytesEqual(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestS7Int32Encoding verifies int32 (DINT) encoding to S7 bytes.
func TestS7Int32Encoding(t *testing.T) {
	tests := []struct {
		name     string
		value    int32
		expected []byte
	}{
		{"zero", 0, []byte{0x00, 0x00, 0x00, 0x00}},
		{"positive", 100000, []byte{0x00, 0x01, 0x86, 0xA0}},
		{"negative", -100000, []byte{0xFF, 0xFE, 0x79, 0x60}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := make([]byte, 4)
			binary.BigEndian.PutUint32(result, uint32(tt.value))
			if !bytesEqual(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestS7Float32Encoding verifies float32 (REAL) encoding to S7 bytes.
func TestS7Float32Encoding(t *testing.T) {
	tests := []struct {
		name     string
		value    float32
		expected []byte
	}{
		{"zero", 0.0, []byte{0x00, 0x00, 0x00, 0x00}},
		{"one", 1.0, []byte{0x3F, 0x80, 0x00, 0x00}},
		{"negative_one", -1.0, []byte{0xBF, 0x80, 0x00, 0x00}},
		{"temperature", 100.0, []byte{0x42, 0xC8, 0x00, 0x00}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := make([]byte, 4)
			binary.BigEndian.PutUint32(result, math.Float32bits(tt.value))
			if !bytesEqual(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestS7BoolEncoding verifies boolean encoding with bit position.
func TestS7BoolEncoding(t *testing.T) {
	tests := []struct {
		name      string
		value     bool
		bitOffset int
		expected  byte
	}{
		{"bit0_true", true, 0, 0x01},
		{"bit0_false", false, 0, 0x00},
		{"bit1_true", true, 1, 0x02},
		{"bit7_true", true, 7, 0x80},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result byte
			if tt.value {
				result = 1 << tt.bitOffset
			}
			if result != tt.expected {
				t.Errorf("expected 0x%02X, got 0x%02X", tt.expected, result)
			}
		})
	}
}

// =============================================================================
// S7 Scaling Tests
// =============================================================================

// TestS7Scaling verifies scaling logic for S7 values.
func TestS7Scaling(t *testing.T) {
	tests := []struct {
		name        string
		rawValue    float64
		scaleFactor float64
		offset      float64
		expected    float64
	}{
		{"no_scaling", 100.0, 1.0, 0.0, 100.0},
		{"scale_only", 1000.0, 0.1, 0.0, 100.0},
		{"offset_only", 100.0, 1.0, 10.0, 110.0},
		{"scale_and_offset", 1000.0, 0.1, 273.15, 373.15}, // Kelvin to Celsius
		{"negative_offset", 100.0, 1.0, -50.0, 50.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.rawValue*tt.scaleFactor + tt.offset
			if math.Abs(result-tt.expected) > 0.001 {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestS7ReverseScaling verifies reverse scaling for write operations.
func TestS7ReverseScaling(t *testing.T) {
	tests := []struct {
		name           string
		engineeringVal float64
		scaleFactor    float64
		offset         float64
		expected       float64
	}{
		{"no_scaling", 100.0, 1.0, 0.0, 100.0},
		{"reverse_scale", 100.0, 0.1, 0.0, 1000.0},
		{"reverse_offset", 110.0, 1.0, 10.0, 100.0},
		{"reverse_both", 373.15, 0.1, 273.15, 1000.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reverse scaling: (engineeringVal - offset) / scaleFactor
			var result float64
			if tt.scaleFactor == 0 {
				result = tt.engineeringVal - tt.offset
			} else {
				result = (tt.engineeringVal - tt.offset) / tt.scaleFactor
			}
			if math.Abs(result-tt.expected) > 0.001 {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// =============================================================================
// S7 Byte Count Tests
// =============================================================================

// TestS7ByteCount verifies byte count for S7 data types.
func TestS7ByteCount(t *testing.T) {
	tests := []struct {
		dataType      domain.DataType
		expectedBytes int
	}{
		{domain.DataTypeBool, 1},
		{domain.DataTypeInt16, 2},
		{domain.DataTypeUInt16, 2},
		{domain.DataTypeInt32, 4},
		{domain.DataTypeUInt32, 4},
		{domain.DataTypeFloat32, 4},
		{domain.DataTypeInt64, 8},
		{domain.DataTypeUInt64, 8},
		{domain.DataTypeFloat64, 8},
	}

	for _, tt := range tests {
		t.Run(string(tt.dataType), func(t *testing.T) {
			byteCount := getByteCountForType(tt.dataType)
			if byteCount != tt.expectedBytes {
				t.Errorf("expected %d bytes for %s, got %d", tt.expectedBytes, tt.dataType, byteCount)
			}
		})
	}
}

// =============================================================================
// S7 Type Conversion Helper Tests
// =============================================================================

// TestToBoolConversion verifies boolean type conversion.
func TestToBoolConversion(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected bool
		valid    bool
	}{
		{"bool_true", true, true, true},
		{"bool_false", false, false, true},
		{"int_nonzero", 1, true, true},
		{"int_zero", 0, false, true},
		{"int64_nonzero", int64(100), true, true},
		{"float_nonzero", 1.5, true, true},
		{"float_zero", 0.0, false, true},
		{"string_invalid", "true", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := toBool(tt.input)
			if ok != tt.valid {
				t.Errorf("expected valid=%v, got valid=%v", tt.valid, ok)
			}
			if ok && result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestToInt64Conversion verifies int64 type conversion.
func TestToInt64Conversion(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int64
		valid    bool
	}{
		{"int", 100, 100, true},
		{"int8", int8(50), 50, true},
		{"int16", int16(1000), 1000, true},
		{"int32", int32(100000), 100000, true},
		{"int64", int64(1000000), 1000000, true},
		{"uint", uint(200), 200, true},
		{"uint8", uint8(255), 255, true},
		{"uint16", uint16(65535), 65535, true},
		{"uint32", uint32(4294967295), 4294967295, true},
		{"float32", float32(100.5), 100, true},
		{"float64", 100.9, 100, true},
		{"string_invalid", "100", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := toInt64(tt.input)
			if ok != tt.valid {
				t.Errorf("expected valid=%v, got valid=%v", tt.valid, ok)
			}
			if ok && result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestToFloat64Conversion verifies float64 type conversion.
func TestToFloat64Conversion(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected float64
		valid    bool
	}{
		{"int", 100, 100.0, true},
		{"int64", int64(1000000), 1000000.0, true},
		{"uint64", uint64(1000000), 1000000.0, true},
		{"float32", float32(3.14), float64(float32(3.14)), true},
		{"float64", 3.14159, 3.14159, true},
		{"string_invalid", "3.14", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := toFloat64(tt.input)
			if ok != tt.valid {
				t.Errorf("expected valid=%v, got valid=%v", tt.valid, ok)
			}
			if ok && result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// =============================================================================
// S7 Address Format Tests
// =============================================================================

// TestS7AddressFormats verifies common S7 address formats.
func TestS7AddressFormats(t *testing.T) {
	addresses := []struct {
		name    string
		address string
		area    domain.S7Area
		db      int
		offset  int
		bit     int
	}{
		{"DB1.DBD0", "DB1.DBD0", domain.S7AreaDB, 1, 0, -1},
		{"DB10.DBW100", "DB10.DBW100", domain.S7AreaDB, 10, 100, -1},
		{"DB1.DBX4.3", "DB1.DBX4.3", domain.S7AreaDB, 1, 4, 3},
		{"MW100", "MW100", domain.S7AreaM, 0, 100, -1},
		{"M0.0", "M0.0", domain.S7AreaM, 0, 0, 0},
		{"I0.1", "I0.1", domain.S7AreaI, 0, 0, 1},
		{"Q0.0", "Q0.0", domain.S7AreaQ, 0, 0, 0},
	}

	for _, tt := range addresses {
		t.Run(tt.name, func(t *testing.T) {
			tag := domain.Tag{
				S7Address: tt.address,
				S7Area:    tt.area,
			}
			if tag.S7Area != tt.area {
				t.Errorf("expected area %s, got %s", tt.area, tag.S7Area)
			}
			if tag.S7Address != tt.address {
				t.Errorf("expected address %s, got %s", tt.address, tag.S7Address)
			}
		})
	}
}

// =============================================================================
// S7 Data Length Validation Tests
// =============================================================================

// TestS7DataLengthValidation verifies data length validation.
func TestS7DataLengthValidation(t *testing.T) {
	tests := []struct {
		name      string
		dataType  domain.DataType
		dataLen   int
		shouldErr bool
	}{
		{"int16_valid", domain.DataTypeInt16, 2, false},
		{"int16_short", domain.DataTypeInt16, 1, true},
		{"int32_valid", domain.DataTypeInt32, 4, false},
		{"int32_short", domain.DataTypeInt32, 2, true},
		{"float32_valid", domain.DataTypeFloat32, 4, false},
		{"float32_short", domain.DataTypeFloat32, 3, true},
		{"float64_valid", domain.DataTypeFloat64, 8, false},
		{"float64_short", domain.DataTypeFloat64, 7, true},
		{"bool_valid", domain.DataTypeBool, 1, false},
		{"bool_empty", domain.DataTypeBool, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requiredLen := getByteCountForType(tt.dataType)
			hasError := tt.dataLen < requiredLen
			if hasError != tt.shouldErr {
				t.Errorf("expected error=%v, got error=%v", tt.shouldErr, hasError)
			}
		})
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func getByteCountForType(dataType domain.DataType) int {
	switch dataType {
	case domain.DataTypeBool:
		return 1
	case domain.DataTypeInt16, domain.DataTypeUInt16:
		return 2
	case domain.DataTypeInt32, domain.DataTypeUInt32, domain.DataTypeFloat32:
		return 4
	case domain.DataTypeInt64, domain.DataTypeUInt64, domain.DataTypeFloat64:
		return 8
	default:
		return 1
	}
}

func toBool(v interface{}) (bool, bool) {
	switch val := v.(type) {
	case bool:
		return val, true
	case int:
		return val != 0, true
	case int8:
		return val != 0, true
	case int16:
		return val != 0, true
	case int32:
		return val != 0, true
	case int64:
		return val != 0, true
	case uint:
		return val != 0, true
	case uint8:
		return val != 0, true
	case uint16:
		return val != 0, true
	case uint32:
		return val != 0, true
	case uint64:
		return val != 0, true
	case float32:
		return val != 0, true
	case float64:
		return val != 0, true
	default:
		return false, false
	}
}

func toInt64(v interface{}) (int64, bool) {
	switch val := v.(type) {
	case int:
		return int64(val), true
	case int8:
		return int64(val), true
	case int16:
		return int64(val), true
	case int32:
		return int64(val), true
	case int64:
		return val, true
	case uint:
		return int64(val), true
	case uint8:
		return int64(val), true
	case uint16:
		return int64(val), true
	case uint32:
		return int64(val), true
	case uint64:
		return int64(val), true
	case float32:
		return int64(val), true
	case float64:
		return int64(val), true
	default:
		return 0, false
	}
}

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case int:
		return float64(val), true
	case int8:
		return float64(val), true
	case int16:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint:
		return float64(val), true
	case uint8:
		return float64(val), true
	case uint16:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint64:
		return float64(val), true
	case float32:
		return float64(val), true
	case float64:
		return val, true
	default:
		return 0, false
	}
}
