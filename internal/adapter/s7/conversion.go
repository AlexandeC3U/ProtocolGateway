// Package s7 provides conversion utilities for S7 data types.
package s7

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

// =============================================================================
// Value Parsing (Read Operations)
// =============================================================================

// parseValue converts raw bytes to a typed value based on the tag's data type.
func (c *Client) parseValue(data []byte, tag *domain.Tag, bitOffset int) (interface{}, error) {
	if len(data) == 0 {
		return nil, domain.ErrInvalidDataLength
	}

	switch tag.DataType {
	case domain.DataTypeBool:
		if len(data) < 1 {
			return nil, domain.ErrInvalidDataLength
		}
		return (data[0] & (1 << bitOffset)) != 0, nil

	case domain.DataTypeInt16:
		if len(data) < 2 {
			return nil, domain.ErrInvalidDataLength
		}
		return int16(binary.BigEndian.Uint16(data)), nil

	case domain.DataTypeUInt16:
		if len(data) < 2 {
			return nil, domain.ErrInvalidDataLength
		}
		return binary.BigEndian.Uint16(data), nil

	case domain.DataTypeInt32:
		if len(data) < 4 {
			return nil, domain.ErrInvalidDataLength
		}
		return int32(binary.BigEndian.Uint32(data)), nil

	case domain.DataTypeUInt32:
		if len(data) < 4 {
			return nil, domain.ErrInvalidDataLength
		}
		return binary.BigEndian.Uint32(data), nil

	case domain.DataTypeInt64:
		if len(data) < 8 {
			return nil, domain.ErrInvalidDataLength
		}
		return int64(binary.BigEndian.Uint64(data)), nil

	case domain.DataTypeUInt64:
		if len(data) < 8 {
			return nil, domain.ErrInvalidDataLength
		}
		return binary.BigEndian.Uint64(data), nil

	case domain.DataTypeFloat32:
		if len(data) < 4 {
			return nil, domain.ErrInvalidDataLength
		}
		bits := binary.BigEndian.Uint32(data)
		return math.Float32frombits(bits), nil

	case domain.DataTypeFloat64:
		if len(data) < 8 {
			return nil, domain.ErrInvalidDataLength
		}
		bits := binary.BigEndian.Uint64(data)
		return math.Float64frombits(bits), nil

	default:
		return nil, domain.ErrInvalidDataType
	}
}

// =============================================================================
// Value Encoding (Write Operations)
// =============================================================================

// valueToBytes converts a value to bytes for writing.
func (c *Client) valueToBytes(value interface{}, tag *domain.Tag, bitOffset int) ([]byte, error) {
	// Reverse scaling if applied
	actualValue := c.reverseScaling(value, tag)

	switch tag.DataType {
	case domain.DataTypeBool:
		b, ok := toBool(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to bool", domain.ErrInvalidWriteValue, value)
		}
		// Note: For boolean writes, we need to do a read-modify-write to preserve
		// adjacent bits in the same byte. The caller (writeData) handles this by
		// reading the current byte first, then calling this with the existing value.
		// This function returns the byte with ONLY the target bit set/cleared,
		// so it must be OR'd or AND'd with the existing byte by the caller.
		//
		// Actually, the gos7 library AGWriteDB writes entire bytes, so writing
		// just the bit mask would destroy adjacent bits. We mark this as needing
		// read-modify-write at the call site.
		data := BufferPool.Get(1)
		if b {
			data[0] = 1 << bitOffset
		} else {
			data[0] = 0
		}
		// Mark this as a boolean write needing special handling
		// The actual read-modify-write is done in writeData
		return data, nil

	case domain.DataTypeInt16:
		i, ok := toInt64(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to int16", domain.ErrInvalidWriteValue, value)
		}
		data := BufferPool.Get(2)
		binary.BigEndian.PutUint16(data, uint16(int16(i)))
		return data, nil

	case domain.DataTypeUInt16:
		i, ok := toInt64(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to uint16", domain.ErrInvalidWriteValue, value)
		}
		data := BufferPool.Get(2)
		binary.BigEndian.PutUint16(data, uint16(i))
		return data, nil

	case domain.DataTypeInt32:
		i, ok := toInt64(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to int32", domain.ErrInvalidWriteValue, value)
		}
		data := BufferPool.Get(4)
		binary.BigEndian.PutUint32(data, uint32(int32(i)))
		return data, nil

	case domain.DataTypeUInt32:
		i, ok := toInt64(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to uint32", domain.ErrInvalidWriteValue, value)
		}
		data := BufferPool.Get(4)
		binary.BigEndian.PutUint32(data, uint32(i))
		return data, nil

	case domain.DataTypeInt64:
		i, ok := toInt64(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to int64", domain.ErrInvalidWriteValue, value)
		}
		data := BufferPool.Get(8)
		binary.BigEndian.PutUint64(data, uint64(i))
		return data, nil

	case domain.DataTypeUInt64:
		i, ok := toUint64(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to uint64", domain.ErrInvalidWriteValue, value)
		}
		data := BufferPool.Get(8)
		binary.BigEndian.PutUint64(data, i)
		return data, nil

	case domain.DataTypeFloat32:
		f, ok := toFloat64(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to float32", domain.ErrInvalidWriteValue, value)
		}
		data := BufferPool.Get(4)
		binary.BigEndian.PutUint32(data, math.Float32bits(float32(f)))
		return data, nil

	case domain.DataTypeFloat64:
		f, ok := toFloat64(actualValue)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to float64", domain.ErrInvalidWriteValue, value)
		}
		data := BufferPool.Get(8)
		binary.BigEndian.PutUint64(data, math.Float64bits(f))
		return data, nil

	default:
		return nil, fmt.Errorf("%w: unsupported data type %s", domain.ErrInvalidDataType, tag.DataType)
	}
}

// =============================================================================
// Scaling Operations
// =============================================================================

// applyScaling applies scale factor and offset to the value.
func (c *Client) applyScaling(value interface{}, tag *domain.Tag) interface{} {
	if tag.ScaleFactor == 1.0 && tag.Offset == 0 {
		return value
	}

	floatVal, ok := toFloat64(value)
	if !ok {
		return value
	}

	return floatVal*tag.ScaleFactor + tag.Offset
}

// reverseScaling reverses the scaling for write operations.
func (c *Client) reverseScaling(value interface{}, tag *domain.Tag) interface{} {
	if tag.ScaleFactor == 1.0 && tag.Offset == 0 {
		return value
	}

	floatVal, ok := toFloat64(value)
	if !ok {
		return value
	}

	// Avoid division by zero
	if tag.ScaleFactor == 0 {
		return floatVal - tag.Offset
	}

	return (floatVal - tag.Offset) / tag.ScaleFactor
}

// =============================================================================
// Byte Count Calculation
// =============================================================================

// getByteCount returns the number of bytes needed for a data type.
func (c *Client) getByteCount(dataType domain.DataType) int {
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

// =============================================================================
// Type Conversion Helpers
// =============================================================================

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
