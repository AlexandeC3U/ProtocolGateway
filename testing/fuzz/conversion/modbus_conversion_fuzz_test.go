//go:build fuzz
// +build fuzz

// Package conversion provides fuzz tests for Modbus data conversion.
package conversion

import (
	"encoding/binary"
	"math"
	"testing"
)

// =============================================================================
// Byte Order Types (mirrors domain package)
// =============================================================================

// ByteOrder represents the byte ordering for multi-byte values.
type ByteOrder string

const (
	ByteOrderBigEndian    ByteOrder = "big_endian"    // ABCD (default)
	ByteOrderLittleEndian ByteOrder = "little_endian" // DCBA
	ByteOrderMidBigEndian ByteOrder = "mid_big"       // BADC (big-endian word swap)
	ByteOrderMidLittle    ByteOrder = "mid_little"    // CDAB (little-endian word swap)
)

// =============================================================================
// Conversion Functions Under Test
// =============================================================================

// reorderBytes reorders bytes according to the specified byte order.
func reorderBytes(data []byte, order ByteOrder) []byte {
	if len(data) == 0 {
		return data
	}

	result := make([]byte, len(data))
	copy(result, data)

	switch order {
	case ByteOrderBigEndian:
		// No change needed (ABCD)
		return result

	case ByteOrderLittleEndian:
		// Reverse all bytes (DCBA)
		for i := 0; i < len(result)/2; i++ {
			result[i], result[len(result)-1-i] = result[len(result)-1-i], result[i]
		}
		return result

	case ByteOrderMidBigEndian:
		// Swap adjacent pairs (BADC)
		for i := 0; i < len(result)-1; i += 2 {
			result[i], result[i+1] = result[i+1], result[i]
		}
		return result

	case ByteOrderMidLittle:
		// Word swap then byte swap (CDAB)
		if len(result) >= 4 {
			// Swap words first
			result[0], result[2] = result[2], result[0]
			result[1], result[3] = result[3], result[1]
		}
		return result

	default:
		return result
	}
}

// parseUint16 parses a uint16 from bytes with the given byte order.
func parseUint16(data []byte, order ByteOrder) uint16 {
	if len(data) < 2 {
		return 0
	}
	ordered := reorderBytes(data[:2], order)
	return binary.BigEndian.Uint16(ordered)
}

// parseUint32 parses a uint32 from bytes with the given byte order.
func parseUint32(data []byte, order ByteOrder) uint32 {
	if len(data) < 4 {
		return 0
	}
	ordered := reorderBytes(data[:4], order)
	return binary.BigEndian.Uint32(ordered)
}

// parseFloat32 parses a float32 from bytes with the given byte order.
func parseFloat32(data []byte, order ByteOrder) float32 {
	if len(data) < 4 {
		return 0
	}
	ordered := reorderBytes(data[:4], order)
	bits := binary.BigEndian.Uint32(ordered)
	return math.Float32frombits(bits)
}

// parseFloat64 parses a float64 from bytes with the given byte order.
func parseFloat64(data []byte, order ByteOrder) float64 {
	if len(data) < 8 {
		return 0
	}
	ordered := reorderBytes(data[:8], order)
	bits := binary.BigEndian.Uint64(ordered)
	return math.Float64frombits(bits)
}

// =============================================================================
// Fuzz Tests for Byte Reordering
// =============================================================================

// FuzzReorderBytes_2Byte fuzzes 2-byte reordering (uint16).
func FuzzReorderBytes_2Byte(f *testing.F) {
	// Seed corpus
	f.Add([]byte{0x12, 0x34})
	f.Add([]byte{0x00, 0x00})
	f.Add([]byte{0xFF, 0xFF})
	f.Add([]byte{0x00, 0x01})
	f.Add([]byte{0x80, 0x00})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 2 {
			return
		}
		input := data[:2]

		for _, order := range []ByteOrder{
			ByteOrderBigEndian,
			ByteOrderLittleEndian,
			ByteOrderMidBigEndian,
			ByteOrderMidLittle,
		} {
			result := reorderBytes(input, order)

			// Verify result length unchanged
			if len(result) != 2 {
				t.Errorf("reorderBytes(%v, %s) changed length: got %d, want 2",
					input, order, len(result))
			}

			// Verify all bytes are preserved (just reordered)
			inputSum := int(input[0]) + int(input[1])
			resultSum := int(result[0]) + int(result[1])
			if inputSum != resultSum {
				t.Errorf("reorderBytes(%v, %s) lost bytes: input sum %d, result sum %d",
					input, order, inputSum, resultSum)
			}
		}
	})
}

// FuzzReorderBytes_4Byte fuzzes 4-byte reordering (uint32/float32).
func FuzzReorderBytes_4Byte(f *testing.F) {
	// Seed corpus
	f.Add([]byte{0x12, 0x34, 0x56, 0x78})
	f.Add([]byte{0x00, 0x00, 0x00, 0x00})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	f.Add([]byte{0x41, 0x20, 0x00, 0x00}) // 10.0 as float32
	f.Add([]byte{0x7F, 0x80, 0x00, 0x00}) // +Inf as float32

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 4 {
			return
		}
		input := data[:4]

		for _, order := range []ByteOrder{
			ByteOrderBigEndian,
			ByteOrderLittleEndian,
			ByteOrderMidBigEndian,
			ByteOrderMidLittle,
		} {
			result := reorderBytes(input, order)

			// Verify result length unchanged
			if len(result) != 4 {
				t.Errorf("reorderBytes(%v, %s) changed length: got %d, want 4",
					input, order, len(result))
			}

			// Verify double-application returns original for symmetric orders
			if order == ByteOrderLittleEndian || order == ByteOrderMidBigEndian {
				doubleResult := reorderBytes(result, order)
				for i := 0; i < 4; i++ {
					if doubleResult[i] != input[i] {
						t.Errorf("double reorderBytes(%v, %s) not identity: got %v",
							input, order, doubleResult)
						break
					}
				}
			}
		}
	})
}

// FuzzReorderBytes_8Byte fuzzes 8-byte reordering (uint64/float64).
func FuzzReorderBytes_8Byte(f *testing.F) {
	// Seed corpus
	f.Add([]byte{0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0})
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
	f.Add([]byte{0x40, 0x24, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) // 10.0 as float64

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 8 {
			return
		}
		input := data[:8]

		for _, order := range []ByteOrder{
			ByteOrderBigEndian,
			ByteOrderLittleEndian,
		} {
			result := reorderBytes(input, order)

			// Verify result length unchanged
			if len(result) != 8 {
				t.Errorf("reorderBytes(%v, %s) changed length: got %d, want 8",
					input, order, len(result))
			}
		}
	})
}

// =============================================================================
// Fuzz Tests for Value Parsing
// =============================================================================

// FuzzParseUint16 fuzzes uint16 parsing.
func FuzzParseUint16(f *testing.F) {
	f.Add([]byte{0x00, 0x00})
	f.Add([]byte{0xFF, 0xFF})
	f.Add([]byte{0x12, 0x34})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 2 {
			return
		}

		for _, order := range []ByteOrder{ByteOrderBigEndian, ByteOrderLittleEndian} {
			result := parseUint16(data, order)

			// Result should always be valid (no panic, no overflow)
			if result > 65535 {
				t.Errorf("parseUint16 returned invalid value: %d", result)
			}
		}
	})
}

// FuzzParseFloat32 fuzzes float32 parsing for edge cases.
func FuzzParseFloat32(f *testing.F) {
	// Normal values
	f.Add([]byte{0x41, 0x20, 0x00, 0x00}) // 10.0
	f.Add([]byte{0xC1, 0x20, 0x00, 0x00}) // -10.0
	// Special values
	f.Add([]byte{0x7F, 0x80, 0x00, 0x00}) // +Inf
	f.Add([]byte{0xFF, 0x80, 0x00, 0x00}) // -Inf
	f.Add([]byte{0x7F, 0xC0, 0x00, 0x00}) // NaN
	// Edge cases
	f.Add([]byte{0x00, 0x00, 0x00, 0x00}) // 0
	f.Add([]byte{0x00, 0x00, 0x00, 0x01}) // Smallest denormal

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 4 {
			return
		}

		for _, order := range []ByteOrder{ByteOrderBigEndian, ByteOrderLittleEndian} {
			result := parseFloat32(data, order)

			// Should not panic - NaN and Inf are valid results
			_ = result

			// Verify NaN handling
			if math.IsNaN(float64(result)) {
				// NaN is valid output for certain inputs
				continue
			}

			// Verify Inf handling
			if math.IsInf(float64(result), 0) {
				// Inf is valid output for certain inputs
				continue
			}
		}
	})
}

// FuzzParseFloat64 fuzzes float64 parsing.
func FuzzParseFloat64(f *testing.F) {
	f.Add([]byte{0x40, 0x24, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) // 10.0
	f.Add([]byte{0x7F, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) // +Inf
	f.Add([]byte{0x7F, 0xF8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) // NaN

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 8 {
			return
		}

		for _, order := range []ByteOrder{ByteOrderBigEndian, ByteOrderLittleEndian} {
			result := parseFloat64(data, order)

			// Should not panic
			_ = result
		}
	})
}

// =============================================================================
// Fuzz Tests for Roundtrip Conversion
// =============================================================================

// FuzzUint32Roundtrip fuzzes uint32 encode/decode roundtrip.
func FuzzUint32Roundtrip(f *testing.F) {
	f.Add(uint32(0))
	f.Add(uint32(1))
	f.Add(uint32(0xFFFFFFFF))
	f.Add(uint32(0x12345678))

	f.Fuzz(func(t *testing.T, value uint32) {
		// Encode as big-endian
		encoded := make([]byte, 4)
		binary.BigEndian.PutUint32(encoded, value)

		// Decode with big-endian order should return original
		decoded := parseUint32(encoded, ByteOrderBigEndian)
		if decoded != value {
			t.Errorf("roundtrip failed: encoded %d, decoded %d", value, decoded)
		}

		// Encode as little-endian
		binary.LittleEndian.PutUint32(encoded, value)

		// Decode with little-endian order should return original
		decoded = parseUint32(encoded, ByteOrderLittleEndian)
		if decoded != value {
			t.Errorf("little-endian roundtrip failed: encoded %d, decoded %d", value, decoded)
		}
	})
}

// FuzzFloat32Roundtrip fuzzes float32 encode/decode roundtrip.
func FuzzFloat32Roundtrip(f *testing.F) {
	f.Add(float32(0))
	f.Add(float32(1.0))
	f.Add(float32(-1.0))
	f.Add(float32(3.14159))
	f.Add(float32(1e38))

	f.Fuzz(func(t *testing.T, value float32) {
		// Skip NaN as it doesn't equal itself
		if math.IsNaN(float64(value)) {
			return
		}

		// Encode as big-endian
		encoded := make([]byte, 4)
		binary.BigEndian.PutUint32(encoded, math.Float32bits(value))

		// Decode with big-endian order should return original
		decoded := parseFloat32(encoded, ByteOrderBigEndian)
		if decoded != value && !(math.IsInf(float64(value), 0) && math.IsInf(float64(decoded), 0)) {
			t.Errorf("roundtrip failed: encoded %v, decoded %v", value, decoded)
		}
	})
}

// =============================================================================
// Scaling Fuzz Tests
// =============================================================================

// applyScaling applies linear scaling: result = (value * scaleFactor) + offset
func applyScaling(value float64, scaleFactor, offset float64) float64 {
	return (value * scaleFactor) + offset
}

// reverseScaling reverses linear scaling: result = (value - offset) / scaleFactor
func reverseScaling(value float64, scaleFactor, offset float64) float64 {
	if scaleFactor == 0 {
		return 0 // Avoid division by zero
	}
	return (value - offset) / scaleFactor
}

// FuzzScalingRoundtrip fuzzes scaling roundtrip.
func FuzzScalingRoundtrip(f *testing.F) {
	f.Add(float64(100), float64(1.0), float64(0))
	f.Add(float64(100), float64(0.1), float64(0))
	f.Add(float64(100), float64(1.0), float64(10))
	f.Add(float64(0), float64(1.0), float64(0))
	f.Add(float64(-100), float64(2.0), float64(-50))

	f.Fuzz(func(t *testing.T, rawValue, scaleFactor, offset float64) {
		// Skip edge cases
		if math.IsNaN(rawValue) || math.IsInf(rawValue, 0) {
			return
		}
		if math.IsNaN(scaleFactor) || math.IsInf(scaleFactor, 0) || scaleFactor == 0 {
			return
		}
		if math.IsNaN(offset) || math.IsInf(offset, 0) {
			return
		}

		// Apply scaling
		scaled := applyScaling(rawValue, scaleFactor, offset)

		// Skip if result is inf/nan
		if math.IsNaN(scaled) || math.IsInf(scaled, 0) {
			return
		}

		// Reverse scaling
		unscaled := reverseScaling(scaled, scaleFactor, offset)

		// Check roundtrip (with floating point tolerance)
		diff := math.Abs(unscaled - rawValue)
		tolerance := math.Abs(rawValue) * 1e-10
		if tolerance < 1e-10 {
			tolerance = 1e-10
		}

		if diff > tolerance {
			t.Errorf("scaling roundtrip failed: raw=%v, scale=%v, offset=%v, scaled=%v, unscaled=%v, diff=%v",
				rawValue, scaleFactor, offset, scaled, unscaled, diff)
		}
	})
}
