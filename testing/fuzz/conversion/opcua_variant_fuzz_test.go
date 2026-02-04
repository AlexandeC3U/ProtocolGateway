//go:build fuzz
// +build fuzz

// Package conversion tests OPC UA variant type conversion with random inputs.
package conversion

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"testing"
	"time"
)

// =============================================================================
// OPC UA Variant Types
// =============================================================================

// UAVariantType represents OPC UA variant type IDs.
type UAVariantType byte

const (
	UATypeNull            UAVariantType = 0
	UATypeBoolean         UAVariantType = 1
	UATypeSByte           UAVariantType = 2
	UATypeByte            UAVariantType = 3
	UATypeInt16           UAVariantType = 4
	UATypeUInt16          UAVariantType = 5
	UATypeInt32           UAVariantType = 6
	UATypeUInt32          UAVariantType = 7
	UATypeInt64           UAVariantType = 8
	UATypeUInt64          UAVariantType = 9
	UATypeFloat           UAVariantType = 10
	UATypeDouble          UAVariantType = 11
	UATypeString          UAVariantType = 12
	UATypeDateTime        UAVariantType = 13
	UATypeGUID            UAVariantType = 14
	UATypeByteString      UAVariantType = 15
	UATypeXMLElement      UAVariantType = 16
	UATypeNodeID          UAVariantType = 17
	UATypeExpandedNodeID  UAVariantType = 18
	UATypeStatusCode      UAVariantType = 19
	UATypeQualifiedName   UAVariantType = 20
	UATypeLocalizedText   UAVariantType = 21
	UATypeExtensionObject UAVariantType = 22
	UATypeDataValue       UAVariantType = 23
	UATypeVariant         UAVariantType = 24
	UATypeDiagnosticInfo  UAVariantType = 25
)

func (t UAVariantType) String() string {
	names := map[UAVariantType]string{
		UATypeNull:            "Null",
		UATypeBoolean:         "Boolean",
		UATypeSByte:           "SByte",
		UATypeByte:            "Byte",
		UATypeInt16:           "Int16",
		UATypeUInt16:          "UInt16",
		UATypeInt32:           "Int32",
		UATypeUInt32:          "UInt32",
		UATypeInt64:           "Int64",
		UATypeUInt64:          "UInt64",
		UATypeFloat:           "Float",
		UATypeDouble:          "Double",
		UATypeString:          "String",
		UATypeDateTime:        "DateTime",
		UATypeGUID:            "GUID",
		UATypeByteString:      "ByteString",
		UATypeXMLElement:      "XMLElement",
		UATypeNodeID:          "NodeID",
		UATypeExpandedNodeID:  "ExpandedNodeID",
		UATypeStatusCode:      "StatusCode",
		UATypeQualifiedName:   "QualifiedName",
		UATypeLocalizedText:   "LocalizedText",
		UATypeExtensionObject: "ExtensionObject",
		UATypeDataValue:       "DataValue",
		UATypeVariant:         "Variant",
		UATypeDiagnosticInfo:  "DiagnosticInfo",
	}
	if name, ok := names[t]; ok {
		return name
	}
	return fmt.Sprintf("Unknown(%d)", t)
}

// UAVariant represents an OPC UA variant value.
type UAVariant struct {
	Type       UAVariantType
	ArrayDim   []int32
	Value      interface{}
	IsArray    bool
	IsNullable bool
}

// =============================================================================
// Conversion Functions
// =============================================================================

// decodeBoolean decodes a boolean from bytes.
func decodeBoolean(data []byte) (bool, error) {
	if len(data) < 1 {
		return false, fmt.Errorf("insufficient data for boolean")
	}
	return data[0] != 0, nil
}

// decodeSByte decodes a signed byte from bytes.
func decodeSByte(data []byte) (int8, error) {
	if len(data) < 1 {
		return 0, fmt.Errorf("insufficient data for sbyte")
	}
	return int8(data[0]), nil
}

// decodeInt16 decodes a signed 16-bit integer from bytes.
func decodeInt16(data []byte) (int16, error) {
	if len(data) < 2 {
		return 0, fmt.Errorf("insufficient data for int16")
	}
	return int16(binary.LittleEndian.Uint16(data)), nil
}

// decodeUInt16 decodes an unsigned 16-bit integer from bytes.
func decodeUInt16(data []byte) (uint16, error) {
	if len(data) < 2 {
		return 0, fmt.Errorf("insufficient data for uint16")
	}
	return binary.LittleEndian.Uint16(data), nil
}

// decodeInt32 decodes a signed 32-bit integer from bytes.
func decodeInt32(data []byte) (int32, error) {
	if len(data) < 4 {
		return 0, fmt.Errorf("insufficient data for int32")
	}
	return int32(binary.LittleEndian.Uint32(data)), nil
}

// decodeUInt32 decodes an unsigned 32-bit integer from bytes.
func decodeUInt32(data []byte) (uint32, error) {
	if len(data) < 4 {
		return 0, fmt.Errorf("insufficient data for uint32")
	}
	return binary.LittleEndian.Uint32(data), nil
}

// decodeInt64 decodes a signed 64-bit integer from bytes.
func decodeInt64(data []byte) (int64, error) {
	if len(data) < 8 {
		return 0, fmt.Errorf("insufficient data for int64")
	}
	return int64(binary.LittleEndian.Uint64(data)), nil
}

// decodeUInt64 decodes an unsigned 64-bit integer from bytes.
func decodeUInt64(data []byte) (uint64, error) {
	if len(data) < 8 {
		return 0, fmt.Errorf("insufficient data for uint64")
	}
	return binary.LittleEndian.Uint64(data), nil
}

// decodeFloat decodes a 32-bit float from bytes.
func decodeFloat(data []byte) (float32, error) {
	if len(data) < 4 {
		return 0, fmt.Errorf("insufficient data for float")
	}
	bits := binary.LittleEndian.Uint32(data)
	return math.Float32frombits(bits), nil
}

// decodeDouble decodes a 64-bit float from bytes.
func decodeDouble(data []byte) (float64, error) {
	if len(data) < 8 {
		return 0, fmt.Errorf("insufficient data for double")
	}
	bits := binary.LittleEndian.Uint64(data)
	return math.Float64frombits(bits), nil
}

// decodeString decodes a length-prefixed string from bytes.
func decodeString(data []byte) (string, error) {
	if len(data) < 4 {
		return "", fmt.Errorf("insufficient data for string length")
	}
	length := int32(binary.LittleEndian.Uint32(data))
	if length < 0 {
		return "", nil // Null string
	}
	if length > int32(len(data)-4) {
		return "", fmt.Errorf("string length %d exceeds available data %d", length, len(data)-4)
	}
	return string(data[4 : 4+length]), nil
}

// decodeDateTime decodes an OPC UA DateTime (100ns ticks since 1601-01-01).
func decodeDateTime(data []byte) (time.Time, error) {
	if len(data) < 8 {
		return time.Time{}, fmt.Errorf("insufficient data for datetime")
	}
	ticks := binary.LittleEndian.Uint64(data)

	// OPC UA DateTime: 100ns ticks since January 1, 1601 UTC
	// Go time: nanoseconds since January 1, 1970 UTC
	const ticksPerSecond = 10_000_000
	const epochDiff = 116444736000000000 // Ticks between 1601 and 1970

	if ticks < epochDiff {
		return time.Time{}, nil // Before Unix epoch
	}

	unixNanos := int64((ticks - epochDiff) * 100)
	return time.Unix(0, unixNanos).UTC(), nil
}

// decodeGUID decodes a 16-byte GUID from bytes.
func decodeGUID(data []byte) ([16]byte, error) {
	if len(data) < 16 {
		return [16]byte{}, fmt.Errorf("insufficient data for GUID")
	}
	var guid [16]byte
	copy(guid[:], data[:16])
	return guid, nil
}

// decodeByteString decodes a length-prefixed byte string.
func decodeByteString(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("insufficient data for bytestring length")
	}
	length := int32(binary.LittleEndian.Uint32(data))
	if length < 0 {
		return nil, nil // Null byte string
	}
	if length > int32(len(data)-4) {
		return nil, fmt.Errorf("bytestring length %d exceeds available data %d", length, len(data)-4)
	}
	result := make([]byte, length)
	copy(result, data[4:4+length])
	return result, nil
}

// DecodeVariant attempts to decode bytes as an OPC UA variant.
func DecodeVariant(data []byte) (*UAVariant, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("empty variant data")
	}

	// First byte contains type information
	encoding := data[0]
	varType := UAVariantType(encoding & 0x3F) // Lower 6 bits are type
	isArray := (encoding & 0x80) != 0
	hasDimensions := (encoding & 0x40) != 0

	variant := &UAVariant{
		Type:    varType,
		IsArray: isArray,
	}

	payload := data[1:]

	if isArray {
		// Decode array length
		if len(payload) < 4 {
			return nil, fmt.Errorf("insufficient data for array length")
		}
		arrayLen := int32(binary.LittleEndian.Uint32(payload))
		if arrayLen < 0 {
			variant.Value = nil
			return variant, nil
		}
		payload = payload[4:]

		// Decode array elements (simplified - decode first element)
		// In production, would decode all elements
		if arrayLen > 0 {
			elem, err := decodeSingleValue(varType, payload)
			if err != nil {
				return nil, fmt.Errorf("failed to decode array element: %w", err)
			}
			variant.Value = elem
		}

		if hasDimensions {
			// Skip dimension decoding for fuzz test simplicity
			variant.ArrayDim = []int32{arrayLen}
		}
	} else {
		value, err := decodeSingleValue(varType, payload)
		if err != nil {
			return nil, err
		}
		variant.Value = value
	}

	return variant, nil
}

// decodeSingleValue decodes a single value of the given type.
func decodeSingleValue(varType UAVariantType, data []byte) (interface{}, error) {
	switch varType {
	case UATypeNull:
		return nil, nil
	case UATypeBoolean:
		return decodeBoolean(data)
	case UATypeSByte:
		return decodeSByte(data)
	case UATypeByte:
		if len(data) < 1 {
			return nil, fmt.Errorf("insufficient data for byte")
		}
		return data[0], nil
	case UATypeInt16:
		return decodeInt16(data)
	case UATypeUInt16:
		return decodeUInt16(data)
	case UATypeInt32:
		return decodeInt32(data)
	case UATypeUInt32:
		return decodeUInt32(data)
	case UATypeInt64:
		return decodeInt64(data)
	case UATypeUInt64:
		return decodeUInt64(data)
	case UATypeFloat:
		return decodeFloat(data)
	case UATypeDouble:
		return decodeDouble(data)
	case UATypeString:
		return decodeString(data)
	case UATypeDateTime:
		return decodeDateTime(data)
	case UATypeGUID:
		return decodeGUID(data)
	case UATypeByteString:
		return decodeByteString(data)
	case UATypeStatusCode:
		return decodeUInt32(data)
	default:
		return nil, fmt.Errorf("unsupported variant type: %s", varType)
	}
}

// EncodeVariant encodes a variant to bytes.
func EncodeVariant(v *UAVariant) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write encoding byte
	encoding := byte(v.Type)
	if v.IsArray {
		encoding |= 0x80
	}
	if len(v.ArrayDim) > 0 {
		encoding |= 0x40
	}
	buf.WriteByte(encoding)

	// Encode value
	if err := encodeSingleValue(buf, v.Type, v.Value); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// encodeSingleValue encodes a single value of the given type.
func encodeSingleValue(buf *bytes.Buffer, varType UAVariantType, value interface{}) error {
	switch varType {
	case UATypeNull:
		return nil
	case UATypeBoolean:
		if b, ok := value.(bool); ok {
			if b {
				buf.WriteByte(1)
			} else {
				buf.WriteByte(0)
			}
			return nil
		}
		return fmt.Errorf("expected bool, got %T", value)
	case UATypeSByte:
		if v, ok := value.(int8); ok {
			buf.WriteByte(byte(v))
			return nil
		}
		return fmt.Errorf("expected int8, got %T", value)
	case UATypeByte:
		if v, ok := value.(byte); ok {
			buf.WriteByte(v)
			return nil
		}
		return fmt.Errorf("expected byte, got %T", value)
	case UATypeInt16:
		if v, ok := value.(int16); ok {
			binary.Write(buf, binary.LittleEndian, v)
			return nil
		}
		return fmt.Errorf("expected int16, got %T", value)
	case UATypeUInt16:
		if v, ok := value.(uint16); ok {
			binary.Write(buf, binary.LittleEndian, v)
			return nil
		}
		return fmt.Errorf("expected uint16, got %T", value)
	case UATypeInt32:
		if v, ok := value.(int32); ok {
			binary.Write(buf, binary.LittleEndian, v)
			return nil
		}
		return fmt.Errorf("expected int32, got %T", value)
	case UATypeUInt32:
		if v, ok := value.(uint32); ok {
			binary.Write(buf, binary.LittleEndian, v)
			return nil
		}
		return fmt.Errorf("expected uint32, got %T", value)
	case UATypeInt64:
		if v, ok := value.(int64); ok {
			binary.Write(buf, binary.LittleEndian, v)
			return nil
		}
		return fmt.Errorf("expected int64, got %T", value)
	case UATypeUInt64:
		if v, ok := value.(uint64); ok {
			binary.Write(buf, binary.LittleEndian, v)
			return nil
		}
		return fmt.Errorf("expected uint64, got %T", value)
	case UATypeFloat:
		if v, ok := value.(float32); ok {
			binary.Write(buf, binary.LittleEndian, v)
			return nil
		}
		return fmt.Errorf("expected float32, got %T", value)
	case UATypeDouble:
		if v, ok := value.(float64); ok {
			binary.Write(buf, binary.LittleEndian, v)
			return nil
		}
		return fmt.Errorf("expected float64, got %T", value)
	case UATypeString:
		if v, ok := value.(string); ok {
			binary.Write(buf, binary.LittleEndian, int32(len(v)))
			buf.WriteString(v)
			return nil
		}
		return fmt.Errorf("expected string, got %T", value)
	case UATypeStatusCode:
		if v, ok := value.(uint32); ok {
			binary.Write(buf, binary.LittleEndian, v)
			return nil
		}
		return fmt.Errorf("expected uint32 for StatusCode, got %T", value)
	default:
		return fmt.Errorf("unsupported type for encoding: %s", varType)
	}
}

// =============================================================================
// Fuzz Tests
// =============================================================================

// FuzzVariantDecode fuzzes the variant decoding function with random bytes.
func FuzzVariantDecode(f *testing.F) {
	// Add seed corpus with valid variant encodings
	seeds := [][]byte{
		// Null
		{0x00},
		// Boolean true
		{0x01, 0x01},
		// Boolean false
		{0x01, 0x00},
		// SByte
		{0x02, 0x7F},
		{0x02, 0x80},
		// Byte
		{0x03, 0xFF},
		// Int16
		{0x04, 0x00, 0x80}, // -32768
		{0x04, 0xFF, 0x7F}, // 32767
		// UInt16
		{0x05, 0xFF, 0xFF}, // 65535
		// Int32
		{0x06, 0x00, 0x00, 0x00, 0x80}, // -2147483648
		{0x06, 0xFF, 0xFF, 0xFF, 0x7F}, // 2147483647
		// UInt32
		{0x07, 0xFF, 0xFF, 0xFF, 0xFF}, // 4294967295
		// Int64
		{0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80},
		// UInt64
		{0x09, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
		// Float
		{0x0A, 0x00, 0x00, 0x80, 0x3F}, // 1.0
		{0x0A, 0x00, 0x00, 0x80, 0x7F}, // +Inf
		{0x0A, 0x00, 0x00, 0xC0, 0x7F}, // NaN
		// Double
		{0x0B, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF0, 0x3F}, // 1.0
		// String (length 5, "Hello")
		{0x0C, 0x05, 0x00, 0x00, 0x00, 'H', 'e', 'l', 'l', 'o'},
		// Empty string
		{0x0C, 0x00, 0x00, 0x00, 0x00},
		// Null string (length -1)
		{0x0C, 0xFF, 0xFF, 0xFF, 0xFF},
		// Array of Int32 (length 2)
		{0x86, 0x02, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00},
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Decode should not panic
		variant, err := DecodeVariant(data)
		if err != nil {
			// Errors are acceptable for malformed input
			return
		}

		// If decode succeeded, verify variant is valid
		if variant != nil {
			// Type should be within valid range
			if variant.Type > UATypeDiagnosticInfo {
				t.Errorf("decoded invalid variant type: %d", variant.Type)
			}
		}
	})
}

// FuzzVariantRoundTrip fuzzes encode/decode round trip.
func FuzzVariantRoundTrip(f *testing.F) {
	// Seed with various typed values
	f.Add(byte(1), true)  // Boolean
	f.Add(byte(2), false) // Boolean
	f.Add(byte(3), true)  // With different flag byte
	f.Add(byte(4), false) // More variations
	f.Add(byte(0), false) // Null type

	f.Fuzz(func(t *testing.T, typeByte byte, boolVal bool) {
		// Limit type to valid primitive types
		varType := UAVariantType(typeByte % 12) // Limit to numeric/bool types
		if varType == UATypeNull {
			varType = UATypeBoolean
		}

		var variant *UAVariant
		switch varType {
		case UATypeBoolean:
			variant = &UAVariant{Type: varType, Value: boolVal}
		case UATypeSByte:
			variant = &UAVariant{Type: varType, Value: int8(typeByte)}
		case UATypeByte:
			variant = &UAVariant{Type: varType, Value: typeByte}
		case UATypeInt16:
			variant = &UAVariant{Type: varType, Value: int16(typeByte) * 100}
		case UATypeUInt16:
			variant = &UAVariant{Type: varType, Value: uint16(typeByte) * 100}
		case UATypeInt32:
			variant = &UAVariant{Type: varType, Value: int32(typeByte) * 10000}
		case UATypeUInt32:
			variant = &UAVariant{Type: varType, Value: uint32(typeByte) * 10000}
		case UATypeInt64:
			variant = &UAVariant{Type: varType, Value: int64(typeByte) * 100000}
		case UATypeUInt64:
			variant = &UAVariant{Type: varType, Value: uint64(typeByte) * 100000}
		case UATypeFloat:
			variant = &UAVariant{Type: varType, Value: float32(typeByte) / 10.0}
		case UATypeDouble:
			variant = &UAVariant{Type: varType, Value: float64(typeByte) / 10.0}
		default:
			return
		}

		// Encode
		encoded, err := EncodeVariant(variant)
		if err != nil {
			t.Fatalf("encode failed for type %s: %v", varType, err)
		}

		// Decode
		decoded, err := DecodeVariant(encoded)
		if err != nil {
			t.Fatalf("decode failed for type %s: %v", varType, err)
		}

		// Verify type matches
		if decoded.Type != variant.Type {
			t.Errorf("type mismatch: got %s, want %s", decoded.Type, variant.Type)
		}
	})
}

// FuzzStringVariant fuzzes string variant encoding/decoding.
func FuzzStringVariant(f *testing.F) {
	// Seed corpus
	f.Add("")
	f.Add("Hello")
	f.Add("Hello, World!")
	f.Add("Unicode: 日本語 🎉")
	f.Add("Tab\tNewline\nCarriage\r")
	f.Add("Null\x00byte")
	f.Add(string(make([]byte, 1000))) // Long string

	f.Fuzz(func(t *testing.T, s string) {
		variant := &UAVariant{
			Type:  UATypeString,
			Value: s,
		}

		// Encode
		encoded, err := EncodeVariant(variant)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}

		// Decode
		decoded, err := DecodeVariant(encoded)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		// Verify
		if decoded.Type != UATypeString {
			t.Errorf("type mismatch: got %s, want String", decoded.Type)
		}

		if decodedStr, ok := decoded.Value.(string); ok {
			if decodedStr != s {
				t.Errorf("string mismatch: got %q, want %q", decodedStr, s)
			}
		} else {
			t.Errorf("decoded value is not string: %T", decoded.Value)
		}
	})
}

// FuzzNumericVariants fuzzes numeric type conversions.
func FuzzNumericVariants(f *testing.F) {
	// Seed with edge cases
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(-1))
	f.Add(int64(math.MaxInt64))
	f.Add(int64(math.MinInt64))
	f.Add(int64(math.MaxInt32))
	f.Add(int64(math.MinInt32))
	f.Add(int64(math.MaxInt16))
	f.Add(int64(math.MinInt16))
	f.Add(int64(127))  // Max int8
	f.Add(int64(-128)) // Min int8

	f.Fuzz(func(t *testing.T, n int64) {
		// Test Int64
		variant := &UAVariant{Type: UATypeInt64, Value: n}
		encoded, err := EncodeVariant(variant)
		if err != nil {
			t.Fatalf("encode int64 failed: %v", err)
		}
		decoded, err := DecodeVariant(encoded)
		if err != nil {
			t.Fatalf("decode int64 failed: %v", err)
		}
		if v, ok := decoded.Value.(int64); ok {
			if v != n {
				t.Errorf("int64 mismatch: got %d, want %d", v, n)
			}
		}

		// Test Int32 if in range
		if n >= math.MinInt32 && n <= math.MaxInt32 {
			variant32 := &UAVariant{Type: UATypeInt32, Value: int32(n)}
			encoded32, err := EncodeVariant(variant32)
			if err != nil {
				t.Fatalf("encode int32 failed: %v", err)
			}
			decoded32, err := DecodeVariant(encoded32)
			if err != nil {
				t.Fatalf("decode int32 failed: %v", err)
			}
			if v, ok := decoded32.Value.(int32); ok {
				if v != int32(n) {
					t.Errorf("int32 mismatch: got %d, want %d", v, int32(n))
				}
			}
		}

		// Test UInt64 for non-negative values
		if n >= 0 {
			variantU := &UAVariant{Type: UATypeUInt64, Value: uint64(n)}
			encodedU, err := EncodeVariant(variantU)
			if err != nil {
				t.Fatalf("encode uint64 failed: %v", err)
			}
			decodedU, err := DecodeVariant(encodedU)
			if err != nil {
				t.Fatalf("decode uint64 failed: %v", err)
			}
			if v, ok := decodedU.Value.(uint64); ok {
				if v != uint64(n) {
					t.Errorf("uint64 mismatch: got %d, want %d", v, uint64(n))
				}
			}
		}
	})
}

// FuzzFloatVariants fuzzes floating-point type conversions.
func FuzzFloatVariants(f *testing.F) {
	// Seed with edge cases
	f.Add(0.0)
	f.Add(1.0)
	f.Add(-1.0)
	f.Add(math.MaxFloat64)
	f.Add(-math.MaxFloat64)
	f.Add(math.SmallestNonzeroFloat64)
	f.Add(math.Pi)
	f.Add(math.E)

	f.Fuzz(func(t *testing.T, d float64) {
		// Skip NaN as it doesn't equal itself
		if math.IsNaN(d) {
			return
		}

		// Test Double
		variant := &UAVariant{Type: UATypeDouble, Value: d}
		encoded, err := EncodeVariant(variant)
		if err != nil {
			t.Fatalf("encode double failed: %v", err)
		}
		decoded, err := DecodeVariant(encoded)
		if err != nil {
			t.Fatalf("decode double failed: %v", err)
		}
		if v, ok := decoded.Value.(float64); ok {
			if math.IsInf(d, 0) && math.IsInf(v, 0) {
				// Both infinite, check sign
				if math.Signbit(d) != math.Signbit(v) {
					t.Errorf("infinity sign mismatch")
				}
			} else if v != d {
				t.Errorf("double mismatch: got %v, want %v", v, d)
			}
		}

		// Test Float32 if in range
		if !math.IsInf(d, 0) && math.Abs(d) <= math.MaxFloat32 {
			f32 := float32(d)
			variantF := &UAVariant{Type: UATypeFloat, Value: f32}
			encodedF, err := EncodeVariant(variantF)
			if err != nil {
				t.Fatalf("encode float failed: %v", err)
			}
			decodedF, err := DecodeVariant(encodedF)
			if err != nil {
				t.Fatalf("decode float failed: %v", err)
			}
			if v, ok := decodedF.Value.(float32); ok {
				if v != f32 {
					t.Errorf("float mismatch: got %v, want %v", v, f32)
				}
			}
		}
	})
}

// FuzzByteOrder tests different byte orderings.
func FuzzByteOrder(f *testing.F) {
	f.Add(uint32(0x12345678))
	f.Add(uint32(0x00000000))
	f.Add(uint32(0xFFFFFFFF))
	f.Add(uint32(0x00FF00FF))
	f.Add(uint32(0xFF00FF00))

	f.Fuzz(func(t *testing.T, val uint32) {
		// Encode as little-endian (OPC UA standard)
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, val)

		// Create variant data
		data := append([]byte{byte(UATypeUInt32)}, buf...)

		// Decode
		variant, err := DecodeVariant(data)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}

		if v, ok := variant.Value.(uint32); ok {
			if v != val {
				t.Errorf("byte order issue: got %08X, want %08X", v, val)
			}
		}
	})
}

// FuzzArrayVariant tests array variant decoding.
func FuzzArrayVariant(f *testing.F) {
	// Seed with valid array encodings
	f.Add([]byte{0x86, 0x00, 0x00, 0x00, 0x00})                         // Empty Int32 array
	f.Add([]byte{0x86, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) // Single element
	f.Add([]byte{0x86, 0xFF, 0xFF, 0xFF, 0xFF})                         // Null array

	f.Fuzz(func(t *testing.T, data []byte) {
		// Force array bit
		if len(data) < 1 {
			return
		}
		data[0] |= 0x80

		// Decode should not panic
		_, _ = DecodeVariant(data)
	})
}

// FuzzMalformedVariant tests handling of malformed variant data.
func FuzzMalformedVariant(f *testing.F) {
	// Various malformed inputs
	f.Add([]byte{})
	f.Add([]byte{0xFF})                         // Invalid type
	f.Add([]byte{0x0C, 0xFF, 0xFF, 0xFF, 0x7F}) // Huge string length
	f.Add([]byte{0x0C, 0x10, 0x00, 0x00, 0x00}) // String length > data
	f.Add([]byte{0x86, 0x00, 0x00, 0x01, 0x00}) // Array with length > data
	f.Add([]byte{0x0B, 0x00, 0x00, 0x00})       // Incomplete double

	f.Fuzz(func(t *testing.T, data []byte) {
		// Should not panic on any input
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("panic on input %v: %v", data, r)
			}
		}()

		_, _ = DecodeVariant(data)
	})
}

// FuzzStatusCode tests status code variant handling.
func FuzzStatusCode(f *testing.F) {
	// Common OPC UA status codes
	f.Add(uint32(0x00000000)) // Good
	f.Add(uint32(0x80000000)) // Bad
	f.Add(uint32(0x40000000)) // Uncertain
	f.Add(uint32(0x80010000)) // BadUnexpectedError
	f.Add(uint32(0x80030000)) // BadNodeIdUnknown
	f.Add(uint32(0x80070000)) // BadNotConnected

	f.Fuzz(func(t *testing.T, statusCode uint32) {
		variant := &UAVariant{
			Type:  UATypeStatusCode,
			Value: statusCode,
		}

		encoded, err := EncodeVariant(variant)
		if err != nil {
			t.Fatalf("encode status code failed: %v", err)
		}

		decoded, err := DecodeVariant(encoded)
		if err != nil {
			t.Fatalf("decode status code failed: %v", err)
		}

		if v, ok := decoded.Value.(uint32); ok {
			if v != statusCode {
				t.Errorf("status code mismatch: got %08X, want %08X", v, statusCode)
			}
		}
	})
}
