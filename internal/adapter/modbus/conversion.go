// Package modbus provides data type conversion utilities for Modbus communication.
package modbus

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

// parseValue converts raw bytes to a typed value based on the tag's data type.
func parseValue(data []byte, tag *domain.Tag) (interface{}, error) {
	if len(data) == 0 {
		return nil, domain.ErrInvalidDataLength
	}

	// Handle coil/discrete input (boolean) values
	if tag.RegisterType == domain.RegisterTypeCoil ||
		tag.RegisterType == domain.RegisterTypeDiscreteInput {
		if tag.BitPosition != nil {
			return (data[0] & (1 << *tag.BitPosition)) != 0, nil
		}
		return data[0] != 0, nil
	}

	// Handle register values
	expectedLen := int(tag.RegisterCount) * 2
	if len(data) < expectedLen {
		return nil, domain.ErrInvalidDataLength
	}

	// Reorder bytes based on byte order
	orderedData := reorderBytes(data[:expectedLen], tag.ByteOrder)

	switch tag.DataType {
	case domain.DataTypeBool:
		if tag.BitPosition != nil {
			val := binary.BigEndian.Uint16(orderedData)
			return (val & (1 << *tag.BitPosition)) != 0, nil
		}
		return orderedData[0] != 0 || orderedData[1] != 0, nil

	case domain.DataTypeInt16:
		return int16(binary.BigEndian.Uint16(orderedData)), nil

	case domain.DataTypeUInt16:
		return binary.BigEndian.Uint16(orderedData), nil

	case domain.DataTypeInt32:
		return int32(binary.BigEndian.Uint32(orderedData)), nil

	case domain.DataTypeUInt32:
		return binary.BigEndian.Uint32(orderedData), nil

	case domain.DataTypeInt64:
		return int64(binary.BigEndian.Uint64(orderedData)), nil

	case domain.DataTypeUInt64:
		return binary.BigEndian.Uint64(orderedData), nil

	case domain.DataTypeFloat32:
		bits := binary.BigEndian.Uint32(orderedData)
		return math.Float32frombits(bits), nil

	case domain.DataTypeFloat64:
		bits := binary.BigEndian.Uint64(orderedData)
		return math.Float64frombits(bits), nil

	default:
		return nil, domain.ErrInvalidDataType
	}
}

// reorderBytes reorders bytes according to the specified byte order.
func reorderBytes(data []byte, order domain.ByteOrder) []byte {
	// Guard against empty or single-byte data
	if len(data) == 0 {
		return data
	}
	if len(data) == 1 {
		return data // No reordering possible for single byte
	}

	// Handle 2-byte case specially
	if len(data) == 2 {
		switch order {
		case domain.ByteOrderLittleEndian:
			return []byte{data[1], data[0]}
		default:
			return data
		}
	}

	result := make([]byte, len(data))
	switch order {
	case domain.ByteOrderBigEndian: // ABCD
		copy(result, data)

	case domain.ByteOrderLittleEndian: // DCBA
		for i := 0; i < len(data); i++ {
			result[i] = data[len(data)-1-i]
		}

	case domain.ByteOrderMidBigEndian: // BADC (word swap)
		for i := 0; i < len(data)-1; i += 2 {
			result[i] = data[i+1]
			result[i+1] = data[i]
		}
		// Handle odd byte at end
		if len(data)%2 == 1 {
			result[len(data)-1] = data[len(data)-1]
		}

	case domain.ByteOrderMidLitEndian: // CDAB (byte swap)
		for i := 0; i+3 < len(data); i += 4 {
			result[i] = data[i+2]
			result[i+1] = data[i+3]
			result[i+2] = data[i]
			result[i+3] = data[i+1]
		}
		// Handle remaining bytes (less than 4)
		remainder := len(data) % 4
		if remainder > 0 {
			start := len(data) - remainder
			copy(result[start:], data[start:])
		}

	default:
		copy(result, data)
	}

	return result
}

// applyScaling applies scale factor and offset to the value.
func applyScaling(value interface{}, tag *domain.Tag) interface{} {
	if tag.ScaleFactor == 1.0 && tag.Offset == 0 {
		return value
	}

	var floatVal float64
	switch v := value.(type) {
	case int16:
		floatVal = float64(v)
	case uint16:
		floatVal = float64(v)
	case int32:
		floatVal = float64(v)
	case uint32:
		floatVal = float64(v)
	case int64:
		floatVal = float64(v)
	case uint64:
		floatVal = float64(v)
	case float32:
		floatVal = float64(v)
	case float64:
		floatVal = v
	case bool:
		return value // No scaling for booleans
	default:
		return value
	}

	return floatVal*tag.ScaleFactor + tag.Offset
}

// reverseScaling reverses the scaling for write operations.
func reverseScaling(value interface{}, tag *domain.Tag) interface{} {
	if tag.ScaleFactor == 1.0 && tag.Offset == 0 {
		return value
	}

	floatVal, ok := toFloat64(value)
	if !ok {
		return value
	}

	return (floatVal - tag.Offset) / tag.ScaleFactor
}

// valueToBytes converts a value to bytes based on the tag's data type.
func valueToBytes(value interface{}, tag *domain.Tag) ([]byte, error) {
	// Reverse scaling if applied
	actualValue := reverseScaling(value, tag)

	var bytes []byte

	switch tag.DataType {
	case domain.DataTypeBool:
		bytes = make([]byte, 2)
		if b, ok := toBool(actualValue); ok && b {
			binary.BigEndian.PutUint16(bytes, 1)
		}

	case domain.DataTypeInt16:
		bytes = make([]byte, 2)
		if v, ok := toInt64(actualValue); ok {
			binary.BigEndian.PutUint16(bytes, uint16(int16(v)))
		} else {
			return nil, fmt.Errorf("%w: cannot convert %T to int16", domain.ErrInvalidWriteValue, value)
		}

	case domain.DataTypeUInt16:
		bytes = make([]byte, 2)
		if v, ok := toInt64(actualValue); ok {
			binary.BigEndian.PutUint16(bytes, uint16(v))
		} else {
			return nil, fmt.Errorf("%w: cannot convert %T to uint16", domain.ErrInvalidWriteValue, value)
		}

	case domain.DataTypeInt32:
		bytes = make([]byte, 4)
		if v, ok := toInt64(actualValue); ok {
			binary.BigEndian.PutUint32(bytes, uint32(int32(v)))
		} else {
			return nil, fmt.Errorf("%w: cannot convert %T to int32", domain.ErrInvalidWriteValue, value)
		}

	case domain.DataTypeUInt32:
		bytes = make([]byte, 4)
		if v, ok := toInt64(actualValue); ok {
			binary.BigEndian.PutUint32(bytes, uint32(v))
		} else {
			return nil, fmt.Errorf("%w: cannot convert %T to uint32", domain.ErrInvalidWriteValue, value)
		}

	case domain.DataTypeInt64:
		bytes = make([]byte, 8)
		if v, ok := toInt64(actualValue); ok {
			binary.BigEndian.PutUint64(bytes, uint64(v))
		} else {
			return nil, fmt.Errorf("%w: cannot convert %T to int64", domain.ErrInvalidWriteValue, value)
		}

	case domain.DataTypeUInt64:
		bytes = make([]byte, 8)
		if v, ok := toUint64(actualValue); ok {
			binary.BigEndian.PutUint64(bytes, v)
		} else {
			return nil, fmt.Errorf("%w: cannot convert %T to uint64", domain.ErrInvalidWriteValue, value)
		}

	case domain.DataTypeFloat32:
		bytes = make([]byte, 4)
		if v, ok := toFloat64(actualValue); ok {
			binary.BigEndian.PutUint32(bytes, math.Float32bits(float32(v)))
		} else {
			return nil, fmt.Errorf("%w: cannot convert %T to float32", domain.ErrInvalidWriteValue, value)
		}

	case domain.DataTypeFloat64:
		bytes = make([]byte, 8)
		if v, ok := toFloat64(actualValue); ok {
			binary.BigEndian.PutUint64(bytes, math.Float64bits(v))
		} else {
			return nil, fmt.Errorf("%w: cannot convert %T to float64", domain.ErrInvalidWriteValue, value)
		}

	default:
		return nil, fmt.Errorf("%w: unsupported data type %s", domain.ErrInvalidDataType, tag.DataType)
	}

	// Apply byte order transformation
	bytes = reorderBytes(bytes, tag.ByteOrder)

	return bytes, nil
}

// toBool converts a value to bool.
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

// toInt64 converts a value to int64.
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

// toUint64 converts a value to uint64.
func toUint64(v interface{}) (uint64, bool) {
	switch val := v.(type) {
	case int:
		return uint64(val), true
	case int8:
		return uint64(val), true
	case int16:
		return uint64(val), true
	case int32:
		return uint64(val), true
	case int64:
		return uint64(val), true
	case uint:
		return uint64(val), true
	case uint8:
		return uint64(val), true
	case uint16:
		return uint64(val), true
	case uint32:
		return uint64(val), true
	case uint64:
		return val, true
	case float32:
		return uint64(val), true
	case float64:
		return uint64(val), true
	default:
		return 0, false
	}
}

// toFloat64 converts a value to float64.
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
