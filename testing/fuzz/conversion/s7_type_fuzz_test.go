//go:build fuzz
// +build fuzz

// Package conversion provides fuzz tests for S7 data type conversion operations.
package conversion

import (
	"encoding/binary"
	"math"
	"testing"
)

// =============================================================================
// S7 Data Type Constants
// =============================================================================

// S7DataType represents S7 PLC data types.
type S7DataType string

const (
	S7DataTypeBool    S7DataType = "bool"    // BOOL - 1 bit
	S7DataTypeInt16   S7DataType = "int16"   // INT - 16 bits signed
	S7DataTypeUInt16  S7DataType = "uint16"  // WORD - 16 bits unsigned
	S7DataTypeInt32   S7DataType = "int32"   // DINT - 32 bits signed
	S7DataTypeUInt32  S7DataType = "uint32"  // DWORD - 32 bits unsigned
	S7DataTypeInt64   S7DataType = "int64"   // LINT - 64 bits signed (S7-1500)
	S7DataTypeUInt64  S7DataType = "uint64"  // LWORD - 64 bits unsigned (S7-1500)
	S7DataTypeFloat32 S7DataType = "float32" // REAL - 32 bits IEEE 754
	S7DataTypeFloat64 S7DataType = "float64" // LREAL - 64 bits IEEE 754 (S7-1500)
)

// S7Area represents S7 memory areas.
type S7Area string

const (
	S7AreaDB S7Area = "DB" // Data Blocks
	S7AreaM  S7Area = "M"  // Merker/Flags
	S7AreaI  S7Area = "I"  // Inputs
	S7AreaQ  S7Area = "Q"  // Outputs
	S7AreaT  S7Area = "T"  // Timers
	S7AreaC  S7Area = "C"  // Counters
)

// =============================================================================
// S7 Conversion Functions Under Test
// =============================================================================

// parseS7Bool parses a boolean from S7 byte data.
func parseS7Bool(data []byte, bitOffset int) (bool, error) {
	if len(data) < 1 || bitOffset < 0 || bitOffset > 7 {
		return false, nil
	}
	return (data[0] & (1 << bitOffset)) != 0, nil
}

// parseS7Int16 parses a 16-bit signed integer from S7 data (Big Endian).
func parseS7Int16(data []byte) (int16, error) {
	if len(data) < 2 {
		return 0, nil
	}
	return int16(binary.BigEndian.Uint16(data)), nil
}

// parseS7UInt16 parses a 16-bit unsigned integer from S7 data (Big Endian).
func parseS7UInt16(data []byte) (uint16, error) {
	if len(data) < 2 {
		return 0, nil
	}
	return binary.BigEndian.Uint16(data), nil
}

// parseS7Int32 parses a 32-bit signed integer from S7 data (Big Endian).
func parseS7Int32(data []byte) (int32, error) {
	if len(data) < 4 {
		return 0, nil
	}
	return int32(binary.BigEndian.Uint32(data)), nil
}

// parseS7UInt32 parses a 32-bit unsigned integer from S7 data (Big Endian).
func parseS7UInt32(data []byte) (uint32, error) {
	if len(data) < 4 {
		return 0, nil
	}
	return binary.BigEndian.Uint32(data), nil
}

// parseS7Int64 parses a 64-bit signed integer from S7 data (Big Endian).
func parseS7Int64(data []byte) (int64, error) {
	if len(data) < 8 {
		return 0, nil
	}
	return int64(binary.BigEndian.Uint64(data)), nil
}

// parseS7UInt64 parses a 64-bit unsigned integer from S7 data (Big Endian).
func parseS7UInt64(data []byte) (uint64, error) {
	if len(data) < 8 {
		return 0, nil
	}
	return binary.BigEndian.Uint64(data), nil
}

// parseS7Float32 parses a 32-bit float from S7 data (Big Endian IEEE 754).
func parseS7Float32(data []byte) (float32, error) {
	if len(data) < 4 {
		return 0, nil
	}
	bits := binary.BigEndian.Uint32(data)
	return math.Float32frombits(bits), nil
}

// parseS7Float64 parses a 64-bit float from S7 data (Big Endian IEEE 754).
func parseS7Float64(data []byte) (float64, error) {
	if len(data) < 8 {
		return 0, nil
	}
	bits := binary.BigEndian.Uint64(data)
	return math.Float64frombits(bits), nil
}

// encodeS7Int16 encodes a 16-bit signed integer to S7 format (Big Endian).
func encodeS7Int16(value int16) []byte {
	data := make([]byte, 2)
	binary.BigEndian.PutUint16(data, uint16(value))
	return data
}

// encodeS7UInt16 encodes a 16-bit unsigned integer to S7 format (Big Endian).
func encodeS7UInt16(value uint16) []byte {
	data := make([]byte, 2)
	binary.BigEndian.PutUint16(data, value)
	return data
}

// encodeS7Int32 encodes a 32-bit signed integer to S7 format (Big Endian).
func encodeS7Int32(value int32) []byte {
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, uint32(value))
	return data
}

// encodeS7UInt32 encodes a 32-bit unsigned integer to S7 format (Big Endian).
func encodeS7UInt32(value uint32) []byte {
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, value)
	return data
}

// encodeS7Int64 encodes a 64-bit signed integer to S7 format (Big Endian).
func encodeS7Int64(value int64) []byte {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, uint64(value))
	return data
}

// encodeS7UInt64 encodes a 64-bit unsigned integer to S7 format (Big Endian).
func encodeS7UInt64(value uint64) []byte {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, value)
	return data
}

// encodeS7Float32 encodes a 32-bit float to S7 format (Big Endian IEEE 754).
func encodeS7Float32(value float32) []byte {
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, math.Float32bits(value))
	return data
}

// encodeS7Float64 encodes a 64-bit float to S7 format (Big Endian IEEE 754).
func encodeS7Float64(value float64) []byte {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, math.Float64bits(value))
	return data
}

// =============================================================================
// Fuzz Tests for S7 Boolean Conversion
// =============================================================================

// FuzzS7BoolRoundtrip fuzzes boolean conversion with different bit offsets.
func FuzzS7BoolRoundtrip(f *testing.F) {
	// Seed corpus
	f.Add(byte(0x00), 0)
	f.Add(byte(0x01), 0)
	f.Add(byte(0xFF), 7)
	f.Add(byte(0x80), 7)
	f.Add(byte(0x55), 4)
	f.Add(byte(0xAA), 3)

	f.Fuzz(func(t *testing.T, data byte, bitOffset int) {
		// Normalize bit offset to valid range
		bitOffset = bitOffset % 8
		if bitOffset < 0 {
			bitOffset = -bitOffset
		}

		input := []byte{data}
		result, err := parseS7Bool(input, bitOffset)
		if err != nil {
			t.Fatalf("parseS7Bool failed: %v", err)
		}

		// Verify against expected bit value
		expected := (data & (1 << bitOffset)) != 0
		if result != expected {
			t.Errorf("parseS7Bool(0x%02X, %d) = %v, expected %v",
				data, bitOffset, result, expected)
		}
	})
}

// FuzzS7BoolAllBits fuzzes all 8 bits in a byte.
func FuzzS7BoolAllBits(f *testing.F) {
	// Seed with all possible single-bit patterns
	for i := 0; i < 8; i++ {
		f.Add(byte(1 << i))
	}
	f.Add(byte(0x00))
	f.Add(byte(0xFF))

	f.Fuzz(func(t *testing.T, data byte) {
		input := []byte{data}

		// Test all 8 bits
		for bit := 0; bit < 8; bit++ {
			result, err := parseS7Bool(input, bit)
			if err != nil {
				t.Fatalf("parseS7Bool failed at bit %d: %v", bit, err)
			}

			expected := (data & (1 << bit)) != 0
			if result != expected {
				t.Errorf("bit %d: parseS7Bool(0x%02X) = %v, expected %v",
					bit, data, result, expected)
			}
		}
	})
}

// =============================================================================
// Fuzz Tests for S7 Integer Conversion
// =============================================================================

// FuzzS7Int16Roundtrip fuzzes 16-bit signed integer encode/decode.
func FuzzS7Int16Roundtrip(f *testing.F) {
	// Seed corpus
	f.Add(int16(0))
	f.Add(int16(1))
	f.Add(int16(-1))
	f.Add(int16(32767))
	f.Add(int16(-32768))
	f.Add(int16(256))
	f.Add(int16(-256))

	f.Fuzz(func(t *testing.T, value int16) {
		encoded := encodeS7Int16(value)
		if len(encoded) != 2 {
			t.Fatalf("encodeS7Int16 returned %d bytes, expected 2", len(encoded))
		}

		decoded, err := parseS7Int16(encoded)
		if err != nil {
			t.Fatalf("parseS7Int16 failed: %v", err)
		}

		if decoded != value {
			t.Errorf("roundtrip failed: %d -> %v -> %d", value, encoded, decoded)
		}
	})
}

// FuzzS7UInt16Roundtrip fuzzes 16-bit unsigned integer encode/decode.
func FuzzS7UInt16Roundtrip(f *testing.F) {
	// Seed corpus
	f.Add(uint16(0))
	f.Add(uint16(1))
	f.Add(uint16(65535))
	f.Add(uint16(256))
	f.Add(uint16(32768))

	f.Fuzz(func(t *testing.T, value uint16) {
		encoded := encodeS7UInt16(value)
		if len(encoded) != 2 {
			t.Fatalf("encodeS7UInt16 returned %d bytes, expected 2", len(encoded))
		}

		decoded, err := parseS7UInt16(encoded)
		if err != nil {
			t.Fatalf("parseS7UInt16 failed: %v", err)
		}

		if decoded != value {
			t.Errorf("roundtrip failed: %d -> %v -> %d", value, encoded, decoded)
		}
	})
}

// FuzzS7Int32Roundtrip fuzzes 32-bit signed integer encode/decode.
func FuzzS7Int32Roundtrip(f *testing.F) {
	// Seed corpus
	f.Add(int32(0))
	f.Add(int32(1))
	f.Add(int32(-1))
	f.Add(int32(2147483647))
	f.Add(int32(-2147483648))
	f.Add(int32(65536))
	f.Add(int32(-65536))

	f.Fuzz(func(t *testing.T, value int32) {
		encoded := encodeS7Int32(value)
		if len(encoded) != 4 {
			t.Fatalf("encodeS7Int32 returned %d bytes, expected 4", len(encoded))
		}

		decoded, err := parseS7Int32(encoded)
		if err != nil {
			t.Fatalf("parseS7Int32 failed: %v", err)
		}

		if decoded != value {
			t.Errorf("roundtrip failed: %d -> %v -> %d", value, encoded, decoded)
		}
	})
}

// FuzzS7UInt32Roundtrip fuzzes 32-bit unsigned integer encode/decode.
func FuzzS7UInt32Roundtrip(f *testing.F) {
	// Seed corpus
	f.Add(uint32(0))
	f.Add(uint32(1))
	f.Add(uint32(4294967295))
	f.Add(uint32(65536))
	f.Add(uint32(2147483648))

	f.Fuzz(func(t *testing.T, value uint32) {
		encoded := encodeS7UInt32(value)
		if len(encoded) != 4 {
			t.Fatalf("encodeS7UInt32 returned %d bytes, expected 4", len(encoded))
		}

		decoded, err := parseS7UInt32(encoded)
		if err != nil {
			t.Fatalf("parseS7UInt32 failed: %v", err)
		}

		if decoded != value {
			t.Errorf("roundtrip failed: %d -> %v -> %d", value, encoded, decoded)
		}
	})
}

// FuzzS7Int64Roundtrip fuzzes 64-bit signed integer encode/decode.
func FuzzS7Int64Roundtrip(f *testing.F) {
	// Seed corpus
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(-1))
	f.Add(int64(9223372036854775807))
	f.Add(int64(-9223372036854775808))
	f.Add(int64(4294967296))
	f.Add(int64(-4294967296))

	f.Fuzz(func(t *testing.T, value int64) {
		encoded := encodeS7Int64(value)
		if len(encoded) != 8 {
			t.Fatalf("encodeS7Int64 returned %d bytes, expected 8", len(encoded))
		}

		decoded, err := parseS7Int64(encoded)
		if err != nil {
			t.Fatalf("parseS7Int64 failed: %v", err)
		}

		if decoded != value {
			t.Errorf("roundtrip failed: %d -> %v -> %d", value, encoded, decoded)
		}
	})
}

// FuzzS7UInt64Roundtrip fuzzes 64-bit unsigned integer encode/decode.
func FuzzS7UInt64Roundtrip(f *testing.F) {
	// Seed corpus
	f.Add(uint64(0))
	f.Add(uint64(1))
	f.Add(uint64(18446744073709551615))
	f.Add(uint64(4294967296))
	f.Add(uint64(9223372036854775808))

	f.Fuzz(func(t *testing.T, value uint64) {
		encoded := encodeS7UInt64(value)
		if len(encoded) != 8 {
			t.Fatalf("encodeS7UInt64 returned %d bytes, expected 8", len(encoded))
		}

		decoded, err := parseS7UInt64(encoded)
		if err != nil {
			t.Fatalf("parseS7UInt64 failed: %v", err)
		}

		if decoded != value {
			t.Errorf("roundtrip failed: %d -> %v -> %d", value, encoded, decoded)
		}
	})
}

// =============================================================================
// Fuzz Tests for S7 Float Conversion
// =============================================================================

// FuzzS7Float32Roundtrip fuzzes 32-bit float encode/decode.
func FuzzS7Float32Roundtrip(f *testing.F) {
	// Seed corpus with various float values
	f.Add(float32(0))
	f.Add(float32(1.0))
	f.Add(float32(-1.0))
	f.Add(float32(3.14159))
	f.Add(float32(1e10))
	f.Add(float32(1e-10))
	f.Add(float32(math.MaxFloat32))
	f.Add(float32(math.SmallestNonzeroFloat32))

	f.Fuzz(func(t *testing.T, value float32) {
		// Skip NaN (NaN != NaN by definition)
		if math.IsNaN(float64(value)) {
			return
		}

		encoded := encodeS7Float32(value)
		if len(encoded) != 4 {
			t.Fatalf("encodeS7Float32 returned %d bytes, expected 4", len(encoded))
		}

		decoded, err := parseS7Float32(encoded)
		if err != nil {
			t.Fatalf("parseS7Float32 failed: %v", err)
		}

		// For infinities, check sign matches
		if math.IsInf(float64(value), 0) {
			if math.IsInf(float64(decoded), 1) != math.IsInf(float64(value), 1) ||
				math.IsInf(float64(decoded), -1) != math.IsInf(float64(value), -1) {
				t.Errorf("infinity sign mismatch: %v -> %v", value, decoded)
			}
			return
		}

		if decoded != value {
			t.Errorf("roundtrip failed: %v -> %v -> %v", value, encoded, decoded)
		}
	})
}

// FuzzS7Float64Roundtrip fuzzes 64-bit float encode/decode.
func FuzzS7Float64Roundtrip(f *testing.F) {
	// Seed corpus with various float values
	f.Add(float64(0))
	f.Add(float64(1.0))
	f.Add(float64(-1.0))
	f.Add(float64(3.141592653589793))
	f.Add(float64(1e100))
	f.Add(float64(1e-100))
	f.Add(math.MaxFloat64)
	f.Add(math.SmallestNonzeroFloat64)

	f.Fuzz(func(t *testing.T, value float64) {
		// Skip NaN (NaN != NaN by definition)
		if math.IsNaN(value) {
			return
		}

		encoded := encodeS7Float64(value)
		if len(encoded) != 8 {
			t.Fatalf("encodeS7Float64 returned %d bytes, expected 8", len(encoded))
		}

		decoded, err := parseS7Float64(encoded)
		if err != nil {
			t.Fatalf("parseS7Float64 failed: %v", err)
		}

		// For infinities, check sign matches
		if math.IsInf(value, 0) {
			if math.IsInf(decoded, 1) != math.IsInf(value, 1) ||
				math.IsInf(decoded, -1) != math.IsInf(value, -1) {
				t.Errorf("infinity sign mismatch: %v -> %v", value, decoded)
			}
			return
		}

		if decoded != value {
			t.Errorf("roundtrip failed: %v -> %v -> %v", value, encoded, decoded)
		}
	})
}

// =============================================================================
// Fuzz Tests for Random Byte Parsing
// =============================================================================

// FuzzS7ParseRandomBytes fuzzes parsing random bytes as different types.
func FuzzS7ParseRandomBytes(f *testing.F) {
	// Seed with various byte patterns
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
	f.Add([]byte{0x41, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) // 10.0f
	f.Add([]byte{0x7F, 0x7F, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00}) // Max float32
	f.Add([]byte{0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 8 {
			return
		}

		// Should not panic on any input
		_, _ = parseS7Bool(data, 0)
		_, _ = parseS7Int16(data)
		_, _ = parseS7UInt16(data)
		_, _ = parseS7Int32(data)
		_, _ = parseS7UInt32(data)
		_, _ = parseS7Int64(data)
		_, _ = parseS7UInt64(data)
		_, _ = parseS7Float32(data)
		_, _ = parseS7Float64(data)
	})
}

// FuzzS7MixedTypeRoundtrip fuzzes mixed type conversions.
func FuzzS7MixedTypeRoundtrip(f *testing.F) {
	// Seed with values that work across multiple types
	f.Add(int64(100))
	f.Add(int64(-100))
	f.Add(int64(32767))
	f.Add(int64(-32768))
	f.Add(int64(0))

	f.Fuzz(func(t *testing.T, value int64) {
		// Test that values within int16 range survive int16 roundtrip
		if value >= -32768 && value <= 32767 {
			int16Val := int16(value)
			encoded := encodeS7Int16(int16Val)
			decoded, _ := parseS7Int16(encoded)
			if decoded != int16Val {
				t.Errorf("int16 roundtrip failed: %d", value)
			}
		}

		// Test that values within int32 range survive int32 roundtrip
		if value >= -2147483648 && value <= 2147483647 {
			int32Val := int32(value)
			encoded := encodeS7Int32(int32Val)
			decoded, _ := parseS7Int32(encoded)
			if decoded != int32Val {
				t.Errorf("int32 roundtrip failed: %d", value)
			}
		}

		// Test int64 roundtrip (always works)
		encoded := encodeS7Int64(value)
		decoded, _ := parseS7Int64(encoded)
		if decoded != value {
			t.Errorf("int64 roundtrip failed: %d", value)
		}
	})
}
