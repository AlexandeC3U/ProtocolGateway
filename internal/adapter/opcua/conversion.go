// Package opcua provides data type conversion utilities for OPC UA communication.
package opcua

import (
	"github.com/gopcua/opcua/ua"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

// =============================================================================
// Value Conversion Functions
// =============================================================================

// applyScaling applies scale factor and offset to the value.
func applyScaling(value interface{}, tag *domain.Tag) interface{} {
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

// =============================================================================
// Type Conversion Helpers
// =============================================================================

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

// =============================================================================
// OPC UA Status Code Conversion
// =============================================================================

// statusCodeToQuality converts an OPC UA status code to a domain quality.
func statusCodeToQuality(status ua.StatusCode) domain.Quality {
	switch {
	case status == ua.StatusOK || status == ua.StatusGood:
		return domain.QualityGood
	case status&0x80000000 != 0: // Bad status codes have bit 31 set
		return domain.QualityBad
	case status&0x40000000 != 0: // Uncertain status codes have bit 30 set
		return domain.QualityUncertain
	default:
		return domain.QualityGood
	}
}

// =============================================================================
// OPC UA Variant Helpers
// =============================================================================

// variantToInterface extracts the Go value from an OPC UA Variant.
// OPC UA uses variants to encapsulate typed values.
func variantToInterface(v *ua.Variant) interface{} {
	if v == nil {
		return nil
	}
	return v.Value()
}

// interfaceToVariant wraps a Go value in an OPC UA Variant.
func interfaceToVariant(value interface{}) *ua.Variant {
	return ua.MustVariant(value)
}
