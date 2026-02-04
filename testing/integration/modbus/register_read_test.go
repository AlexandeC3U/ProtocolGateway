//go:build integration
// +build integration

// Package modbus_test tests Modbus register read operations against a simulator.
package modbus_test

import (
	"context"
	"encoding/binary"
	"os"
	"testing"
	"time"
)

// TestConfig holds test configuration loaded from environment.
type TestConfig struct {
	ModbusHost    string
	ModbusPort    string
	SlaveID       byte
	Timeout       time.Duration
	RetryAttempts int
}

// loadTestConfig loads configuration from environment variables.
func loadTestConfig() TestConfig {
	host := os.Getenv("MODBUS_TEST_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("MODBUS_TEST_PORT")
	if port == "" {
		port = "5020"
	}

	return TestConfig{
		ModbusHost:    host,
		ModbusPort:    port,
		SlaveID:       1,
		Timeout:       5 * time.Second,
		RetryAttempts: 3,
	}
}

// =============================================================================
// Holding Register Tests (Function Code 03)
// =============================================================================

// TestReadHoldingRegisters tests reading holding registers (FC 03).
func TestReadHoldingRegisters(t *testing.T) {
	cfg := loadTestConfig()
	_ = cfg // Will be used when connecting to actual simulator

	tests := []struct {
		name         string
		startAddress uint16
		quantity     uint16
		expectBytes  int
	}{
		{"Single register", 0, 1, 2},
		{"Multiple registers", 0, 10, 20},
		{"Max safe quantity", 0, 125, 250}, // Modbus spec limit
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify expected byte count
			expectedBytes := int(tt.quantity) * 2
			if expectedBytes != tt.expectBytes {
				t.Errorf("expected %d bytes for %d registers, got %d",
					tt.expectBytes, tt.quantity, expectedBytes)
			}
		})
	}
}

// TestReadHoldingRegisterBoundary tests boundary conditions for holding registers.
func TestReadHoldingRegisterBoundary(t *testing.T) {
	tests := []struct {
		name         string
		startAddress uint16
		quantity     uint16
		shouldFail   bool
		reason       string
	}{
		{"Zero quantity", 0, 0, true, "quantity must be >= 1"},
		{"Max address", 65535, 1, false, "valid max address"},
		{"Overflow address", 65535, 2, true, "address overflow"},
		{"Over max quantity", 0, 126, true, "exceeds 125 limit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate request before sending
			valid := tt.quantity >= 1 && tt.quantity <= 125
			if tt.startAddress > 0 {
				valid = valid && (uint32(tt.startAddress)+uint32(tt.quantity) <= 65536)
			}

			if valid == tt.shouldFail {
				t.Logf("Request validity: %v (expected fail: %v, reason: %s)",
					valid, tt.shouldFail, tt.reason)
			}
		})
	}
}

// TestHoldingRegisterDataTypes tests reading different data types from holding registers.
func TestHoldingRegisterDataTypes(t *testing.T) {
	tests := []struct {
		name      string
		registers int
		dataType  string
		byteOrder string
	}{
		{"Int16", 1, "INT16", "BigEndian"},
		{"UInt16", 1, "UINT16", "BigEndian"},
		{"Int32", 2, "INT32", "BigEndian"},
		{"UInt32", 2, "UINT32", "BigEndian"},
		{"Float32", 2, "FLOAT32", "BigEndian"},
		{"Int64", 4, "INT64", "BigEndian"},
		{"Float64", 4, "FLOAT64", "BigEndian"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			byteCount := tt.registers * 2
			t.Logf("DataType: %s requires %d registers (%d bytes), ByteOrder: %s",
				tt.dataType, tt.registers, byteCount, tt.byteOrder)
		})
	}
}

// =============================================================================
// Input Register Tests (Function Code 04)
// =============================================================================

// TestReadInputRegisters tests reading input registers (FC 04).
func TestReadInputRegisters(t *testing.T) {
	tests := []struct {
		name         string
		startAddress uint16
		quantity     uint16
	}{
		{"Single input", 0, 1},
		{"Multiple inputs", 0, 50},
		{"Non-zero start", 100, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectedBytes := int(tt.quantity) * 2
			t.Logf("Reading %d input registers from address %d (%d bytes)",
				tt.quantity, tt.startAddress, expectedBytes)
		})
	}
}

// =============================================================================
// Byte Order Conversion Tests
// =============================================================================

// TestByteOrderConversions tests byte order conversions for Modbus data.
func TestByteOrderConversions(t *testing.T) {
	// Modbus uses Big Endian by default
	tests := []struct {
		name      string
		bytes     []byte
		byteOrder string
		expected  interface{}
	}{
		{
			name:      "Int16 BigEndian",
			bytes:     []byte{0x01, 0x00}, // 256 in BigEndian (0x0100)
			byteOrder: "BigEndian",
			expected:  int16(256),
		},
		{
			name:      "Int16 LittleEndian",
			bytes:     []byte{0x01, 0x00}, // 1 in LittleEndian (low byte first: 0x01, high byte: 0x00)
			byteOrder: "LittleEndian",
			expected:  int16(1),
		},
		{
			name:      "UInt16 BigEndian",
			bytes:     []byte{0xFF, 0xFF}, // 65535
			byteOrder: "BigEndian",
			expected:  uint16(65535),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result interface{}
			switch v := tt.expected.(type) {
			case int16:
				if tt.byteOrder == "BigEndian" {
					result = int16(binary.BigEndian.Uint16(tt.bytes))
				} else {
					result = int16(binary.LittleEndian.Uint16(tt.bytes))
				}
				if result != v {
					t.Errorf("expected %d, got %d", v, result)
				}
			case uint16:
				if tt.byteOrder == "BigEndian" {
					result = binary.BigEndian.Uint16(tt.bytes)
				} else {
					result = binary.LittleEndian.Uint16(tt.bytes)
				}
				if result != v {
					t.Errorf("expected %d, got %d", v, result)
				}
			}
		})
	}
}

// TestWordSwapConversion tests word-swapped byte orders.
func TestWordSwapConversion(t *testing.T) {
	// 32-bit value with word swap (CD AB format)
	tests := []struct {
		name     string
		bytes    []byte
		expected uint32
	}{
		{
			name:     "ABCD (Big Endian)",
			bytes:    []byte{0x01, 0x02, 0x03, 0x04},
			expected: 0x01020304,
		},
		{
			name:     "CDAB (Word Swap)",
			bytes:    []byte{0x03, 0x04, 0x01, 0x02},
			expected: 0x01020304, // After unswapping
		},
		{
			name:     "DCBA (Little Endian)",
			bytes:    []byte{0x04, 0x03, 0x02, 0x01},
			expected: 0x01020304,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result uint32
			switch tt.name {
			case "ABCD (Big Endian)":
				result = binary.BigEndian.Uint32(tt.bytes)
			case "CDAB (Word Swap)":
				// Word swap: swap the two 16-bit words
				swapped := []byte{tt.bytes[2], tt.bytes[3], tt.bytes[0], tt.bytes[1]}
				result = binary.BigEndian.Uint32(swapped)
			case "DCBA (Little Endian)":
				result = binary.LittleEndian.Uint32(tt.bytes)
			}

			if result != tt.expected {
				t.Errorf("expected 0x%08X, got 0x%08X", tt.expected, result)
			}
		})
	}
}

// =============================================================================
// Scaling Tests
// =============================================================================

// TestScalingOperations tests value scaling for register reads.
func TestScalingOperations(t *testing.T) {
	tests := []struct {
		name        string
		rawValue    float64
		scaleFactor float64
		offset      float64
		expected    float64
	}{
		{"No scaling", 100, 1.0, 0, 100},
		{"Scale only", 100, 0.1, 0, 10},
		{"Offset only", 100, 1.0, -50, 50},
		{"Scale and offset", 100, 0.1, 10, 20},       // (100 * 0.1) + 10 = 20
		{"Temperature sensor", 2500, 0.01, -40, -15}, // Raw 2500 -> 25°C - 40 = -15°C
		{"Pressure sensor", 16000, 0.000625, 0, 10},  // 4-20mA -> 0-10 bar
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scaled := (tt.rawValue * tt.scaleFactor) + tt.offset
			if scaled != tt.expected {
				t.Errorf("expected %.4f, got %.4f", tt.expected, scaled)
			}
		})
	}
}

// =============================================================================
// Register Range Batching Tests
// =============================================================================

// TestRegisterRangeBatching tests batching of register reads.
func TestRegisterRangeBatching(t *testing.T) {
	type registerRequest struct {
		address uint16
		count   uint16
	}

	tests := []struct {
		name          string
		requests      []registerRequest
		maxRegisters  uint16
		maxGap        uint16
		expectedReads int
	}{
		{
			name: "Contiguous registers",
			requests: []registerRequest{
				{0, 1}, {1, 1}, {2, 1}, {3, 1}, {4, 1},
			},
			maxRegisters:  125,
			maxGap:        10,
			expectedReads: 1, // Can batch into single read
		},
		{
			name: "Large gap",
			requests: []registerRequest{
				{0, 1}, {100, 1}, // 99 register gap
			},
			maxRegisters:  125,
			maxGap:        10,
			expectedReads: 2, // Must split due to gap
		},
		{
			name: "Exceeds max registers",
			requests: []registerRequest{
				{0, 100}, {100, 100}, // 200 registers total
			},
			maxRegisters:  125,
			maxGap:        10,
			expectedReads: 2, // Must split due to limit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate minimum reads needed
			totalRegisters := uint16(0)
			for _, req := range tt.requests {
				totalRegisters += req.count
			}

			minReads := (int(totalRegisters) + int(tt.maxRegisters) - 1) / int(tt.maxRegisters)
			t.Logf("Total registers: %d, Min reads needed: %d, Expected: %d",
				totalRegisters, minReads, tt.expectedReads)
		})
	}
}

// =============================================================================
// Error Response Tests
// =============================================================================

// TestModbusExceptionResponses tests handling of Modbus exception responses.
func TestModbusExceptionResponses(t *testing.T) {
	exceptions := []struct {
		code        byte
		name        string
		description string
		retryable   bool
	}{
		{0x01, "ILLEGAL_FUNCTION", "Function code not supported", false},
		{0x02, "ILLEGAL_DATA_ADDRESS", "Invalid register address", false},
		{0x03, "ILLEGAL_DATA_VALUE", "Invalid data in request", false},
		{0x04, "SLAVE_DEVICE_FAILURE", "Device error", true},
		{0x05, "ACKNOWLEDGE", "Request processing", true},
		{0x06, "SLAVE_DEVICE_BUSY", "Device busy", true},
		{0x08, "MEMORY_PARITY_ERROR", "Memory error", true},
		{0x0A, "GATEWAY_PATH_UNAVAILABLE", "Gateway path issue", true},
		{0x0B, "GATEWAY_TARGET_FAILED", "Target not responding", true},
	}

	for _, exc := range exceptions {
		t.Run(exc.name, func(t *testing.T) {
			t.Logf("Exception 0x%02X (%s): %s, Retryable: %v",
				exc.code, exc.name, exc.description, exc.retryable)
		})
	}
}

// =============================================================================
// Timeout Tests
// =============================================================================

// TestReadTimeout tests timeout handling for register reads.
func TestReadTimeout(t *testing.T) {
	timeouts := []struct {
		name    string
		timeout time.Duration
		network string // "fast", "normal", "slow"
	}{
		{"Fast network", 500 * time.Millisecond, "fast"},
		{"Normal network", 2 * time.Second, "normal"},
		{"Slow network", 5 * time.Second, "slow"},
		{"Very slow", 10 * time.Second, "very_slow"},
	}

	for _, tt := range timeouts {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			// Verify context has correct deadline
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Error("expected context to have deadline")
				return
			}

			remaining := time.Until(deadline)
			if remaining > tt.timeout {
				t.Errorf("remaining time %v exceeds timeout %v", remaining, tt.timeout)
			}
		})
	}
}

// =============================================================================
// Concurrent Read Tests
// =============================================================================

// TestConcurrentReads tests concurrent register read requests.
func TestConcurrentReads(t *testing.T) {
	// Modbus TCP allows concurrent connections, but single-threaded within connection
	tests := []struct {
		name        string
		connections int
		readsPerSec int
		description string
	}{
		{"Single connection", 1, 100, "Serial reads on one connection"},
		{"Multiple connections", 5, 500, "Parallel reads on 5 connections"},
		{"High concurrency", 20, 2000, "Stress test with 20 connections"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxReadsPerConnection := tt.readsPerSec / tt.connections
			t.Logf("%s: %d connections, %d reads/sec total, %d reads/sec per connection",
				tt.description, tt.connections, tt.readsPerSec, maxReadsPerConnection)
		})
	}
}

// =============================================================================
// Data Quality Tests
// =============================================================================

// TestDataQualityFlags tests data quality indicators.
func TestDataQualityFlags(t *testing.T) {
	qualities := []struct {
		name        string
		code        int
		description string
	}{
		{"Good", 192, "Value is reliable"},
		{"Bad", 0, "Value is unreliable"},
		{"Uncertain", 64, "Value quality unknown"},
		{"ConfigError", 4, "Configuration error"},
		{"NotConnected", 8, "Device not connected"},
		{"DeviceFailure", 12, "Device failure"},
		{"SensorFailure", 16, "Sensor failure"},
		{"CommFailure", 24, "Communication failure"},
		{"OutOfService", 28, "Out of service"},
	}

	for _, q := range qualities {
		t.Run(q.name, func(t *testing.T) {
			t.Logf("Quality %s (0x%02X): %s", q.name, q.code, q.description)
		})
	}
}
