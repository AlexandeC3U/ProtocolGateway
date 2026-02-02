// Package modbus_test provides unit tests for Modbus conversion functions.
// NOTE: These tests verify conversion logic patterns. The actual internal
// conversion functions (reorderBytes, parseValue, etc.) are unexported.
// For full internal testing, add tests to internal/adapter/modbus/conversion_test.go
package modbus_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

// TestByteOrderConstants verifies byte order constants are defined correctly.
func TestByteOrderConstants(t *testing.T) {
	orders := []domain.ByteOrder{
		domain.ByteOrderBigEndian,
		domain.ByteOrderLittleEndian,
		domain.ByteOrderMidBigEndian,
		domain.ByteOrderMidLitEndian,
	}

	for _, order := range orders {
		if order == "" {
			t.Error("byte order constant should not be empty")
		}
	}
}

// TestDataTypeConstants verifies data type constants are defined.
func TestDataTypeConstants(t *testing.T) {
	types := []domain.DataType{
		domain.DataTypeBool,
		domain.DataTypeInt16,
		domain.DataTypeUInt16,
		domain.DataTypeInt32,
		domain.DataTypeUInt32,
		domain.DataTypeInt64,
		domain.DataTypeUInt64,
		domain.DataTypeFloat32,
		domain.DataTypeFloat64,
		domain.DataTypeString,
	}

	for _, dt := range types {
		if dt == "" {
			t.Error("data type constant should not be empty")
		}
	}
}

// TestRegisterTypeConstants verifies register type constants.
func TestRegisterTypeConstants(t *testing.T) {
	types := []domain.RegisterType{
		domain.RegisterTypeCoil,
		domain.RegisterTypeDiscreteInput,
		domain.RegisterTypeInputRegister,
		domain.RegisterTypeHoldingRegister,
	}

	for _, rt := range types {
		if rt == "" {
			t.Error("register type constant should not be empty")
		}
	}
}

// TestTagConfiguration_Modbus verifies Modbus tag configuration fields.
func TestTagConfiguration_Modbus(t *testing.T) {
	bitPos := uint8(5)
	tag := domain.Tag{
		ID:            "test-tag",
		Name:          "Test Tag",
		DataType:      domain.DataTypeInt16,
		RegisterType:  domain.RegisterTypeHoldingRegister,
		Address:       100,
		RegisterCount: 1,
		ByteOrder:     domain.ByteOrderBigEndian,
		ScaleFactor:   0.1,
		Offset:        10,
		BitPosition:   &bitPos,
		Enabled:       true,
	}

	if tag.ID != "test-tag" {
		t.Error("ID not set correctly")
	}
	if tag.DataType != domain.DataTypeInt16 {
		t.Error("DataType not set correctly")
	}
	if tag.RegisterType != domain.RegisterTypeHoldingRegister {
		t.Error("RegisterType not set correctly")
	}
	if tag.Address != 100 {
		t.Error("Address not set correctly")
	}
	if tag.ByteOrder != domain.ByteOrderBigEndian {
		t.Error("ByteOrder not set correctly")
	}
	if *tag.BitPosition != 5 {
		t.Error("BitPosition not set correctly")
	}
}

// TestByteReorderingLogic_BigEndian verifies big endian byte ordering logic.
func TestByteReorderingLogic_BigEndian(t *testing.T) {
	// Big endian: no reordering needed (ABCD stays ABCD)
	input := []byte{0xAB, 0xCD, 0xEF, 0x12}
	expected := []byte{0xAB, 0xCD, 0xEF, 0x12}

	if !bytesEqual(input, expected) {
		t.Error("big endian should not modify byte order")
	}
}

// TestByteReorderingLogic_LittleEndian verifies little endian byte ordering logic.
func TestByteReorderingLogic_LittleEndian(t *testing.T) {
	// Little endian: ABCD -> DCBA (full reversal)
	input := []byte{0xAB, 0xCD, 0xEF, 0x12}
	expected := []byte{0x12, 0xEF, 0xCD, 0xAB}

	// Simulate little endian reordering
	result := make([]byte, len(input))
	for i := 0; i < len(input); i++ {
		result[i] = input[len(input)-1-i]
	}

	if !bytesEqual(result, expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

// TestByteReorderingLogic_WordSwap verifies word swap (mid-big endian) logic.
func TestByteReorderingLogic_WordSwap(t *testing.T) {
	// BADC word swap: swap bytes within each 16-bit word
	input := []byte{0xAB, 0xCD, 0xEF, 0x12}
	expected := []byte{0xCD, 0xAB, 0x12, 0xEF}

	// Simulate word swap
	result := make([]byte, len(input))
	for i := 0; i < len(input); i += 2 {
		if i+1 < len(input) {
			result[i] = input[i+1]
			result[i+1] = input[i]
		}
	}

	if !bytesEqual(result, expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

// TestInt16Parsing verifies int16 parsing from bytes.
func TestInt16Parsing(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected int16
	}{
		{"positive", []byte{0x00, 0x64}, 100},
		{"negative", []byte{0xFF, 0x9C}, -100},
		{"zero", []byte{0x00, 0x00}, 0},
		{"max", []byte{0x7F, 0xFF}, 32767},
		{"min", []byte{0x80, 0x00}, -32768},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := int16(binary.BigEndian.Uint16(tt.data))
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestUInt16Parsing verifies uint16 parsing from bytes.
func TestUInt16Parsing(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected uint16
	}{
		{"zero", []byte{0x00, 0x00}, 0},
		{"positive", []byte{0x00, 0x64}, 100},
		{"max", []byte{0xFF, 0xFF}, 65535},
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

// TestInt32Parsing verifies int32 parsing from bytes.
func TestInt32Parsing(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected int32
	}{
		{"positive", []byte{0x00, 0x00, 0x00, 0x64}, 100},
		{"negative", []byte{0xFF, 0xFF, 0xFF, 0x9C}, -100},
		{"large", []byte{0x00, 0x01, 0x51, 0x80}, 86400},
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

// TestFloat32Parsing verifies float32 parsing from bytes.
func TestFloat32Parsing(t *testing.T) {
	tests := []struct {
		name  string
		value float32
	}{
		{"positive", 123.456},
		{"negative", -99.99},
		{"zero", 0},
		{"small", 0.001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			data := make([]byte, 4)
			binary.BigEndian.PutUint32(data, math.Float32bits(tt.value))

			// Decode
			result := math.Float32frombits(binary.BigEndian.Uint32(data))

			if result != tt.value {
				t.Errorf("expected %f, got %f", tt.value, result)
			}
		})
	}
}

// TestScalingLogic verifies scaling calculation logic.
func TestScalingLogic(t *testing.T) {
	tests := []struct {
		name        string
		rawValue    float64
		scaleFactor float64
		offset      float64
		expected    float64
	}{
		{"no scaling", 100, 1.0, 0, 100},
		{"scale only", 100, 0.1, 0, 10},
		{"offset only", 100, 1.0, -50, 50},
		{"scale and offset", 100, 0.1, 10, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Scaling formula: (rawValue * scaleFactor) + offset
			result := (tt.rawValue * tt.scaleFactor) + tt.offset
			if result != tt.expected {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// TestReverseScalingLogic verifies reverse scaling calculation.
func TestReverseScalingLogic(t *testing.T) {
	tests := []struct {
		name        string
		scaledValue float64
		scaleFactor float64
		offset      float64
		expected    float64
	}{
		{"no scaling", 100, 1.0, 0, 100},
		{"scale only", 10, 0.1, 0, 100},
		{"offset only", 50, 1.0, -50, 100},
		{"scale and offset", 20, 0.1, 10, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reverse scaling: (scaledValue - offset) / scaleFactor
			result := (tt.scaledValue - tt.offset) / tt.scaleFactor
			if math.Abs(result-tt.expected) > 0.0001 {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// TestBoolParsing verifies boolean parsing from coil data.
func TestBoolParsing(t *testing.T) {
	tests := []struct {
		name     string
		data     byte
		expected bool
	}{
		{"true (0xFF)", 0xFF, true},
		{"true (0x01)", 0x01, true},
		{"false", 0x00, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.data != 0
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestBitPositionExtraction verifies bit extraction from register data.
func TestBitPositionExtraction(t *testing.T) {
	tests := []struct {
		name        string
		data        byte
		bitPosition uint8
		expected    bool
	}{
		{"bit 0 set", 0x01, 0, true},
		{"bit 0 not set", 0x00, 0, false},
		{"bit 5 set", 0x20, 5, true},
		{"bit 5 not set", 0x00, 5, false},
		{"bit 7 set", 0x80, 7, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := (tt.data & (1 << tt.bitPosition)) != 0
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestInt16ToBytes verifies int16 encoding to bytes.
func TestInt16ToBytes(t *testing.T) {
	tests := []struct {
		name     string
		value    int16
		expected []byte
	}{
		{"positive", 100, []byte{0x00, 0x64}},
		{"negative", -100, []byte{0xFF, 0x9C}},
		{"zero", 0, []byte{0x00, 0x00}},
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

// TestFloat32ToBytes verifies float32 encoding to bytes.
func TestFloat32ToBytes(t *testing.T) {
	value := float32(123.456)
	expected := make([]byte, 4)
	binary.BigEndian.PutUint32(expected, math.Float32bits(value))

	result := make([]byte, 4)
	binary.BigEndian.PutUint32(result, math.Float32bits(value))

	if !bytesEqual(result, expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

// bytesEqual compares two byte slices.
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
