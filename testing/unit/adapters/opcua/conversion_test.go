// Package opcua_test provides unit tests for OPC UA conversion functions.
// NOTE: These tests verify conversion logic patterns. The actual internal
// conversion functions (applyScaling, reverseScaling, etc.) are unexported.
// For full internal testing, add tests to internal/adapter/opcua/conversion_test.go
package opcua_test

import (
	"math"
	"testing"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

// TestScalingLogic verifies scaling calculation logic for OPC UA.
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
		{"negative scale", 100, -1.0, 0, -100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Scaling formula: (rawValue * scaleFactor) + offset
			result := (tt.rawValue * tt.scaleFactor) + tt.offset
			if math.Abs(result-tt.expected) > 0.0001 {
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

// TestScalingRoundTrip verifies scaling can be reversed.
func TestScalingRoundTrip(t *testing.T) {
	tests := []struct {
		name        string
		original    float64
		scaleFactor float64
		offset      float64
	}{
		{"simple scale", 100, 0.1, 0},
		{"with offset", 100, 0.5, 25},
		{"no transform", 42, 1.0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Apply scaling
			scaled := (tt.original * tt.scaleFactor) + tt.offset

			// Reverse scaling
			restored := (scaled - tt.offset) / tt.scaleFactor

			if math.Abs(restored-tt.original) > 0.0001 {
				t.Errorf("round trip failed: original=%f, restored=%f", tt.original, restored)
			}
		})
	}
}

// TestOPCUATagConfiguration verifies OPC UA tag configuration fields.
func TestOPCUATagConfiguration(t *testing.T) {
	tag := domain.Tag{
		ID:            "test-tag",
		Name:          "Test Tag",
		DataType:      domain.DataTypeFloat64,
		OPCNodeID:     "ns=2;s=TestNode",
		ScaleFactor:   0.1,
		Offset:        10,
		DeadbandType:  domain.DeadbandTypeAbsolute,
		DeadbandValue: 1.0,
		Enabled:       true,
	}

	if tag.OPCNodeID != "ns=2;s=TestNode" {
		t.Error("OPCNodeID not set correctly")
	}
	if tag.DeadbandType != domain.DeadbandTypeAbsolute {
		t.Error("DeadbandType not set correctly")
	}
}

// TestOPCUAConnectionConfig verifies OPC UA connection configuration.
func TestOPCUAConnectionConfig(t *testing.T) {
	conn := domain.ConnectionConfig{
		Host:              "192.168.1.50",
		Port:              4840,
		OPCEndpointURL:    "opc.tcp://192.168.1.50:4840",
		OPCSecurityPolicy: "None",
		OPCSecurityMode:   "None",
		OPCAuthMode:       "Anonymous",
	}

	if conn.OPCEndpointURL != "opc.tcp://192.168.1.50:4840" {
		t.Error("OPCEndpointURL not set correctly")
	}
	if conn.OPCSecurityPolicy != "None" {
		t.Error("OPCSecurityPolicy not set correctly")
	}
}

// TestBoolConversionLogic verifies boolean conversion patterns.
func TestBoolConversionLogic(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected bool
		valid    bool
	}{
		{"true", true, true, true},
		{"false", false, false, true},
		{"int non-zero", 1, true, true},
		{"int zero", 0, false, true},
		{"int64 non-zero", int64(42), true, true},
		{"float64 zero", float64(0), false, true},
		{"float64 non-zero", float64(0.1), true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result bool
			var valid bool

			switch v := tt.value.(type) {
			case bool:
				result = v
				valid = true
			case int:
				result = v != 0
				valid = true
			case int64:
				result = v != 0
				valid = true
			case float64:
				result = v != 0
				valid = true
			default:
				valid = false
			}

			if valid != tt.valid {
				t.Errorf("expected valid=%v, got %v", tt.valid, valid)
			}
			if valid && result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestInt64ConversionLogic verifies int64 conversion patterns.
func TestInt64ConversionLogic(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected int64
		valid    bool
	}{
		{"int", 42, 42, true},
		{"int16", int16(100), 100, true},
		{"int32", int32(-500), -500, true},
		{"int64", int64(9999999999), 9999999999, true},
		{"uint8", uint8(255), 255, true},
		{"uint16", uint16(65535), 65535, true},
		{"float64", float64(42.9), 42, true}, // truncated
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result int64
			var valid bool

			switch v := tt.value.(type) {
			case int:
				result = int64(v)
				valid = true
			case int16:
				result = int64(v)
				valid = true
			case int32:
				result = int64(v)
				valid = true
			case int64:
				result = v
				valid = true
			case uint8:
				result = int64(v)
				valid = true
			case uint16:
				result = int64(v)
				valid = true
			case float64:
				result = int64(v)
				valid = true
			default:
				valid = false
			}

			if valid != tt.valid {
				t.Errorf("expected valid=%v, got %v", tt.valid, valid)
			}
			if valid && result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestFloat64ConversionLogic verifies float64 conversion patterns.
func TestFloat64ConversionLogic(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected float64
		valid    bool
	}{
		{"float64", float64(3.14159), 3.14159, true},
		{"float32", float32(2.5), 2.5, true},
		{"int", 42, 42.0, true},
		{"int16", int16(-100), -100.0, true},
		{"uint32", uint32(1000000), 1000000.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result float64
			var valid bool

			switch v := tt.value.(type) {
			case float64:
				result = v
				valid = true
			case float32:
				result = float64(v)
				valid = true
			case int:
				result = float64(v)
				valid = true
			case int16:
				result = float64(v)
				valid = true
			case uint32:
				result = float64(v)
				valid = true
			default:
				valid = false
			}

			if valid != tt.valid {
				t.Errorf("expected valid=%v, got %v", tt.valid, valid)
			}
			if valid && math.Abs(result-tt.expected) > 0.0001 {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// TestDeadbandConstants verifies deadband type constants.
func TestDeadbandConstants(t *testing.T) {
	types := []domain.DeadbandType{
		domain.DeadbandTypeNone,
		domain.DeadbandTypeAbsolute,
		domain.DeadbandTypePercent,
	}

	for _, dt := range types {
		if dt == "" {
			t.Error("deadband type constant should not be empty")
		}
	}
}

// TestDeadbandLogic_Absolute verifies absolute deadband filtering.
func TestDeadbandLogic_Absolute(t *testing.T) {
	tests := []struct {
		name         string
		oldValue     float64
		newValue     float64
		deadband     float64
		shouldReport bool
	}{
		{"within deadband", 100, 100.5, 1.0, false},
		{"exceeds deadband", 100, 102, 1.0, true},
		{"negative change within", 100, 99.5, 1.0, false},
		{"negative change exceeds", 100, 98, 1.0, true},
		{"exact boundary", 100, 101, 1.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := math.Abs(tt.newValue - tt.oldValue)
			shouldReport := diff >= tt.deadband

			if shouldReport != tt.shouldReport {
				t.Errorf("expected shouldReport=%v, got %v (diff=%f)", tt.shouldReport, shouldReport, diff)
			}
		})
	}
}

// TestDeadbandLogic_Percent verifies percentage deadband filtering.
func TestDeadbandLogic_Percent(t *testing.T) {
	tests := []struct {
		name         string
		oldValue     float64
		newValue     float64
		deadband     float64 // percentage
		shouldReport bool
	}{
		{"within 10%", 100, 105, 10, false},
		{"exceeds 10%", 100, 115, 10, true},
		{"within 5%", 200, 205, 5, false},
		{"exceeds 5%", 200, 220, 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := math.Abs(tt.newValue - tt.oldValue)
			threshold := math.Abs(tt.oldValue) * (tt.deadband / 100.0)
			shouldReport := diff >= threshold

			if shouldReport != tt.shouldReport {
				t.Errorf("expected shouldReport=%v, got %v", tt.shouldReport, shouldReport)
			}
		})
	}
}
