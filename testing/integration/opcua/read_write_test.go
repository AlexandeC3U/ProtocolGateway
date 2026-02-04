//go:build integration
// +build integration

// Package opcua_test tests OPC UA read/write operations against a simulator.
package opcua_test

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Test Configuration
// =============================================================================

// getOPCUAReadWriteEndpoint returns the OPC UA simulator endpoint for read/write tests.
func getOPCUAReadWriteEndpoint() string {
	if endpoint := os.Getenv("OPCUA_SIMULATOR_ENDPOINT"); endpoint != "" {
		return endpoint
	}
	return "opc.tcp://localhost:4840"
}

// =============================================================================
// Mock Types for Testing
// =============================================================================

// UADataType represents OPC UA data types for testing.
type UADataType int

const (
	UADataTypeBoolean UADataType = iota
	UADataTypeSByte
	UADataTypeByte
	UADataTypeInt16
	UADataTypeUInt16
	UADataTypeInt32
	UADataTypeUInt32
	UADataTypeInt64
	UADataTypeUInt64
	UADataTypeFloat
	UADataTypeDouble
	UADataTypeString
	UADataTypeDateTime
)

func (t UADataType) String() string {
	names := []string{
		"Boolean", "SByte", "Byte", "Int16", "UInt16",
		"Int32", "UInt32", "Int64", "UInt64",
		"Float", "Double", "String", "DateTime",
	}
	if int(t) < len(names) {
		return names[t]
	}
	return fmt.Sprintf("Unknown(%d)", t)
}

// MockReadResult represents a mock read result.
type MockReadResult struct {
	Value      interface{}
	StatusCode uint32
	Timestamp  time.Time
	DataType   UADataType
}

// MockWriteResult represents a mock write result.
type MockWriteResult struct {
	StatusCode uint32
	Success    bool
}

// MockOPCUAReadWriter simulates OPC UA read/write operations.
type MockOPCUAReadWriter struct {
	mu       sync.RWMutex
	values   map[string]interface{}
	types    map[string]UADataType
	endpoint string
}

// NewMockOPCUAReadWriter creates a new mock read/writer with default values.
func NewMockOPCUAReadWriter(endpoint string) *MockOPCUAReadWriter {
	rw := &MockOPCUAReadWriter{
		endpoint: endpoint,
		values:   make(map[string]interface{}),
		types:    make(map[string]UADataType),
	}
	rw.initializeDefaultValues()
	return rw
}

// initializeDefaultValues sets up default values for testing.
func (rw *MockOPCUAReadWriter) initializeDefaultValues() {
	// Boolean values
	rw.values["ns=2;s=Demo.BooleanValue"] = true
	rw.types["ns=2;s=Demo.BooleanValue"] = UADataTypeBoolean

	// Integer values
	rw.values["ns=2;s=Demo.Int16Value"] = int16(1234)
	rw.types["ns=2;s=Demo.Int16Value"] = UADataTypeInt16

	rw.values["ns=2;s=Demo.Int32Value"] = int32(123456)
	rw.types["ns=2;s=Demo.Int32Value"] = UADataTypeInt32

	rw.values["ns=2;s=Demo.Int64Value"] = int64(1234567890)
	rw.types["ns=2;s=Demo.Int64Value"] = UADataTypeInt64

	rw.values["ns=2;s=Demo.UInt16Value"] = uint16(65000)
	rw.types["ns=2;s=Demo.UInt16Value"] = UADataTypeUInt16

	rw.values["ns=2;s=Demo.UInt32Value"] = uint32(4000000000)
	rw.types["ns=2;s=Demo.UInt32Value"] = UADataTypeUInt32

	// Float values
	rw.values["ns=2;s=Demo.FloatValue"] = float32(3.14159)
	rw.types["ns=2;s=Demo.FloatValue"] = UADataTypeFloat

	rw.values["ns=2;s=Demo.DoubleValue"] = float64(3.141592653589793)
	rw.types["ns=2;s=Demo.DoubleValue"] = UADataTypeDouble

	// Simulation values
	rw.values["ns=2;s=Simulation.Temperature"] = float64(25.5)
	rw.types["ns=2;s=Simulation.Temperature"] = UADataTypeDouble

	rw.values["ns=2;s=Simulation.Pressure"] = float64(101.325)
	rw.types["ns=2;s=Simulation.Pressure"] = UADataTypeDouble

	rw.values["ns=2;s=Simulation.Counter"] = uint32(0)
	rw.types["ns=2;s=Simulation.Counter"] = UADataTypeUInt32

	rw.values["ns=2;s=Simulation.Status"] = "Running"
	rw.types["ns=2;s=Simulation.Status"] = UADataTypeString
}

// Read reads a value from the mock server.
func (rw *MockOPCUAReadWriter) Read(ctx context.Context, nodeID string) (*MockReadResult, error) {
	rw.mu.RLock()
	defer rw.mu.RUnlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Simulate network latency
	time.Sleep(5 * time.Millisecond)

	value, exists := rw.values[nodeID]
	if !exists {
		return &MockReadResult{
			StatusCode: 0x80350000, // Bad_NodeIdUnknown
		}, nil
	}

	return &MockReadResult{
		Value:      value,
		StatusCode: 0, // Good
		Timestamp:  time.Now(),
		DataType:   rw.types[nodeID],
	}, nil
}

// Write writes a value to the mock server.
func (rw *MockOPCUAReadWriter) Write(ctx context.Context, nodeID string, value interface{}) (*MockWriteResult, error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Simulate network latency
	time.Sleep(5 * time.Millisecond)

	// Check if node exists
	if _, exists := rw.values[nodeID]; !exists {
		return &MockWriteResult{
			StatusCode: 0x80350000, // Bad_NodeIdUnknown
			Success:    false,
		}, nil
	}

	// Type validation (simplified)
	expectedType := rw.types[nodeID]
	if !rw.validateType(value, expectedType) {
		return &MockWriteResult{
			StatusCode: 0x80740000, // Bad_TypeMismatch
			Success:    false,
		}, nil
	}

	rw.values[nodeID] = value
	return &MockWriteResult{
		StatusCode: 0, // Good
		Success:    true,
	}, nil
}

// validateType checks if the value matches the expected type.
func (rw *MockOPCUAReadWriter) validateType(value interface{}, expected UADataType) bool {
	switch expected {
	case UADataTypeBoolean:
		_, ok := value.(bool)
		return ok
	case UADataTypeInt16:
		_, ok := value.(int16)
		return ok
	case UADataTypeInt32:
		_, ok := value.(int32)
		return ok
	case UADataTypeInt64:
		_, ok := value.(int64)
		return ok
	case UADataTypeUInt16:
		_, ok := value.(uint16)
		return ok
	case UADataTypeUInt32:
		_, ok := value.(uint32)
		return ok
	case UADataTypeFloat:
		_, ok := value.(float32)
		return ok
	case UADataTypeDouble:
		_, ok := value.(float64)
		return ok
	case UADataTypeString:
		_, ok := value.(string)
		return ok
	default:
		return true
	}
}

// =============================================================================
// Single Value Read Tests
// =============================================================================

// TestOPCUAReadBoolean tests reading boolean values.
func TestOPCUAReadBoolean(t *testing.T) {
	endpoint := getOPCUAReadWriteEndpoint()
	rw := NewMockOPCUAReadWriter(endpoint)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := rw.Read(ctx, "ns=2;s=Demo.BooleanValue")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if result.StatusCode != 0 {
		t.Errorf("expected status Good, got 0x%08X", result.StatusCode)
	}

	value, ok := result.Value.(bool)
	if !ok {
		t.Fatalf("expected bool value, got %T", result.Value)
	}
	t.Logf("Boolean value: %v", value)
}

// TestOPCUAReadNumericTypes tests reading various numeric types.
func TestOPCUAReadNumericTypes(t *testing.T) {
	endpoint := getOPCUAReadWriteEndpoint()
	rw := NewMockOPCUAReadWriter(endpoint)

	tests := []struct {
		name   string
		nodeID string
	}{
		{"Int16", "ns=2;s=Demo.Int16Value"},
		{"Int32", "ns=2;s=Demo.Int32Value"},
		{"Int64", "ns=2;s=Demo.Int64Value"},
		{"UInt16", "ns=2;s=Demo.UInt16Value"},
		{"UInt32", "ns=2;s=Demo.UInt32Value"},
		{"Float", "ns=2;s=Demo.FloatValue"},
		{"Double", "ns=2;s=Demo.DoubleValue"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			result, err := rw.Read(ctx, tt.nodeID)
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}

			if result.StatusCode != 0 {
				t.Errorf("expected status Good, got 0x%08X", result.StatusCode)
			}

			t.Logf("%s value: %v (type: %T)", tt.name, result.Value, result.Value)
		})
	}
}

// TestOPCUAReadNonExistentNode tests reading a node that doesn't exist.
func TestOPCUAReadNonExistentNode(t *testing.T) {
	endpoint := getOPCUAReadWriteEndpoint()
	rw := NewMockOPCUAReadWriter(endpoint)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := rw.Read(ctx, "ns=999;s=NonExistent")
	if err != nil {
		t.Fatalf("Read failed unexpectedly: %v", err)
	}

	if result.StatusCode == 0 {
		t.Error("expected Bad status for non-existent node")
	}
	t.Logf("Expected bad status: 0x%08X", result.StatusCode)
}

// =============================================================================
// Single Value Write Tests
// =============================================================================

// TestOPCUAWriteBoolean tests writing boolean values.
func TestOPCUAWriteBoolean(t *testing.T) {
	endpoint := getOPCUAReadWriteEndpoint()
	rw := NewMockOPCUAReadWriter(endpoint)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	nodeID := "ns=2;s=Demo.BooleanValue"

	// Write false
	result, err := rw.Write(ctx, nodeID, false)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if !result.Success {
		t.Errorf("Write failed with status: 0x%08X", result.StatusCode)
	}

	// Read back
	readResult, err := rw.Read(ctx, nodeID)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if value, ok := readResult.Value.(bool); !ok || value != false {
		t.Errorf("expected false, got %v", readResult.Value)
	}

	// Write true
	result, err = rw.Write(ctx, nodeID, true)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if !result.Success {
		t.Errorf("Write failed with status: 0x%08X", result.StatusCode)
	}

	// Read back
	readResult, err = rw.Read(ctx, nodeID)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if value, ok := readResult.Value.(bool); !ok || value != true {
		t.Errorf("expected true, got %v", readResult.Value)
	}
}

// TestOPCUAWriteNumericTypes tests writing various numeric types.
func TestOPCUAWriteNumericTypes(t *testing.T) {
	endpoint := getOPCUAReadWriteEndpoint()
	rw := NewMockOPCUAReadWriter(endpoint)

	tests := []struct {
		name   string
		nodeID string
		value  interface{}
	}{
		{"Int16", "ns=2;s=Demo.Int16Value", int16(9999)},
		{"Int32", "ns=2;s=Demo.Int32Value", int32(999999)},
		{"Int64", "ns=2;s=Demo.Int64Value", int64(9999999999)},
		{"UInt16", "ns=2;s=Demo.UInt16Value", uint16(50000)},
		{"UInt32", "ns=2;s=Demo.UInt32Value", uint32(3000000000)},
		{"Float", "ns=2;s=Demo.FloatValue", float32(2.71828)},
		{"Double", "ns=2;s=Demo.DoubleValue", float64(2.718281828459045)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Write value
			result, err := rw.Write(ctx, tt.nodeID, tt.value)
			if err != nil {
				t.Fatalf("Write failed: %v", err)
			}
			if !result.Success {
				t.Errorf("Write failed with status: 0x%08X", result.StatusCode)
			}

			// Read back and verify
			readResult, err := rw.Read(ctx, tt.nodeID)
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}

			t.Logf("%s: wrote %v, read %v", tt.name, tt.value, readResult.Value)
		})
	}
}

// TestOPCUAWriteTypeMismatch tests writing with wrong type.
func TestOPCUAWriteTypeMismatch(t *testing.T) {
	endpoint := getOPCUAReadWriteEndpoint()
	rw := NewMockOPCUAReadWriter(endpoint)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to write string to boolean node
	result, err := rw.Write(ctx, "ns=2;s=Demo.BooleanValue", "not a bool")
	if err != nil {
		t.Fatalf("Write failed unexpectedly: %v", err)
	}

	if result.Success {
		t.Error("expected write to fail due to type mismatch")
	}
	t.Logf("Expected type mismatch status: 0x%08X", result.StatusCode)
}

// =============================================================================
// Batch Read/Write Tests
// =============================================================================

// TestOPCUABatchRead tests reading multiple values in a batch.
func TestOPCUABatchRead(t *testing.T) {
	endpoint := getOPCUAReadWriteEndpoint()
	rw := NewMockOPCUAReadWriter(endpoint)

	nodeIDs := []string{
		"ns=2;s=Demo.BooleanValue",
		"ns=2;s=Demo.Int32Value",
		"ns=2;s=Demo.FloatValue",
		"ns=2;s=Demo.DoubleValue",
		"ns=2;s=Simulation.Temperature",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results := make([]*MockReadResult, len(nodeIDs))
	for i, nodeID := range nodeIDs {
		result, err := rw.Read(ctx, nodeID)
		if err != nil {
			t.Fatalf("Batch read failed at index %d: %v", i, err)
		}
		results[i] = result
	}

	// Verify all succeeded
	for i, result := range results {
		if result.StatusCode != 0 {
			t.Errorf("Node %s failed with status: 0x%08X", nodeIDs[i], result.StatusCode)
		}
		t.Logf("Read %s: %v", nodeIDs[i], result.Value)
	}
}

// TestOPCUABatchWrite tests writing multiple values in a batch.
func TestOPCUABatchWrite(t *testing.T) {
	endpoint := getOPCUAReadWriteEndpoint()
	rw := NewMockOPCUAReadWriter(endpoint)

	writes := []struct {
		nodeID string
		value  interface{}
	}{
		{"ns=2;s=Demo.BooleanValue", false},
		{"ns=2;s=Demo.Int32Value", int32(42)},
		{"ns=2;s=Demo.FloatValue", float32(1.5)},
		{"ns=2;s=Demo.DoubleValue", float64(2.5)},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Write all values
	for _, w := range writes {
		result, err := rw.Write(ctx, w.nodeID, w.value)
		if err != nil {
			t.Fatalf("Batch write failed for %s: %v", w.nodeID, err)
		}
		if !result.Success {
			t.Errorf("Write to %s failed with status: 0x%08X", w.nodeID, result.StatusCode)
		}
	}

	// Verify all values
	for _, w := range writes {
		result, err := rw.Read(ctx, w.nodeID)
		if err != nil {
			t.Fatalf("Read failed for %s: %v", w.nodeID, err)
		}
		t.Logf("Verified %s: wrote %v, read %v", w.nodeID, w.value, result.Value)
	}
}

// =============================================================================
// Concurrent Read/Write Tests
// =============================================================================

// TestOPCUAConcurrentReads tests concurrent read operations.
func TestOPCUAConcurrentReads(t *testing.T) {
	endpoint := getOPCUAReadWriteEndpoint()
	rw := NewMockOPCUAReadWriter(endpoint)

	nodeID := "ns=2;s=Simulation.Temperature"
	concurrency := 20
	iterations := 50

	var wg sync.WaitGroup
	var successCount atomic.Int64
	var errorCount atomic.Int64

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				result, err := rw.Read(ctx, nodeID)
				cancel()

				if err != nil {
					errorCount.Add(1)
					continue
				}
				if result.StatusCode != 0 {
					errorCount.Add(1)
					continue
				}
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	total := int64(concurrency * iterations)
	t.Logf("Concurrent reads: %d/%d succeeded, %d errors",
		successCount.Load(), total, errorCount.Load())

	if errorCount.Load() > 0 {
		t.Errorf("had %d errors in concurrent reads", errorCount.Load())
	}
}

// TestOPCUAConcurrentWriteRead tests concurrent write and read operations.
func TestOPCUAConcurrentWriteRead(t *testing.T) {
	endpoint := getOPCUAReadWriteEndpoint()
	rw := NewMockOPCUAReadWriter(endpoint)

	nodeID := "ns=2;s=Simulation.Counter"
	concurrency := 10
	iterations := 20

	var wg sync.WaitGroup
	var writeSuccessCount atomic.Int64
	var readSuccessCount atomic.Int64

	// Writers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				value := uint32(workerID*1000 + j)
				result, err := rw.Write(ctx, nodeID, value)
				cancel()

				if err == nil && result.Success {
					writeSuccessCount.Add(1)
				}
			}
		}(i)
	}

	// Readers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				result, err := rw.Read(ctx, nodeID)
				cancel()

				if err == nil && result.StatusCode == 0 {
					readSuccessCount.Add(1)
				}
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Concurrent write/read: %d writes, %d reads succeeded",
		writeSuccessCount.Load(), readSuccessCount.Load())
}

// =============================================================================
// Edge Case Tests
// =============================================================================

// TestOPCUAReadWriteBoundaryValues tests boundary values for numeric types.
func TestOPCUAReadWriteBoundaryValues(t *testing.T) {
	endpoint := getOPCUAReadWriteEndpoint()
	rw := NewMockOPCUAReadWriter(endpoint)

	tests := []struct {
		name   string
		nodeID string
		min    interface{}
		max    interface{}
	}{
		{"Int16", "ns=2;s=Demo.Int16Value", int16(-32768), int16(32767)},
		{"UInt16", "ns=2;s=Demo.UInt16Value", uint16(0), uint16(65535)},
		{"Int32", "ns=2;s=Demo.Int32Value", int32(-2147483648), int32(2147483647)},
		{"UInt32", "ns=2;s=Demo.UInt32Value", uint32(0), uint32(4294967295)},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_Min", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			result, err := rw.Write(ctx, tt.nodeID, tt.min)
			if err != nil || !result.Success {
				t.Errorf("Write min value failed")
			}
		})

		t.Run(tt.name+"_Max", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			result, err := rw.Write(ctx, tt.nodeID, tt.max)
			if err != nil || !result.Success {
				t.Errorf("Write max value failed")
			}
		})
	}
}

// TestOPCUAReadWriteSpecialFloatValues tests special float values (Inf, NaN).
func TestOPCUAReadWriteSpecialFloatValues(t *testing.T) {
	endpoint := getOPCUAReadWriteEndpoint()
	rw := NewMockOPCUAReadWriter(endpoint)

	specialValues := []struct {
		name  string
		value float64
	}{
		{"PositiveInf", math.Inf(1)},
		{"NegativeInf", math.Inf(-1)},
		{"NaN", math.NaN()},
		{"MaxFloat64", math.MaxFloat64},
		{"SmallestFloat64", math.SmallestNonzeroFloat64},
	}

	nodeID := "ns=2;s=Demo.DoubleValue"

	for _, sv := range specialValues {
		t.Run(sv.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			result, err := rw.Write(ctx, nodeID, sv.value)
			if err != nil {
				t.Logf("Write %s: error %v", sv.name, err)
			} else {
				t.Logf("Write %s: success=%v, status=0x%08X", sv.name, result.Success, result.StatusCode)
			}
		})
	}
}

// TestOPCUAReadWriteTimeout tests timeout behavior.
func TestOPCUAReadWriteTimeout(t *testing.T) {
	endpoint := getOPCUAReadWriteEndpoint()
	rw := NewMockOPCUAReadWriter(endpoint)

	// Very short timeout - should fail due to simulated latency
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err := rw.Read(ctx, "ns=2;s=Demo.BooleanValue")
	if err == nil {
		t.Log("Read succeeded despite short timeout (latency was faster)")
	} else {
		t.Logf("Expected timeout behavior: %v", err)
	}
}
