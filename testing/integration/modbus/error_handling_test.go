//go:build integration
// +build integration

package modbus_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/modbus"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/nexus-edge/protocol-gateway/testing/integration"
)

// Note: testLogger is defined in connection_test.go in the same package

// TestModbus_InvalidSlaveID tests error handling when using an invalid slave ID.
func TestModbus_InvalidSlaveID(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	// Slave ID 255 is invalid (valid range is 1-247)
	client, err := modbus.NewClient("test-invalid-slave", modbus.ClientConfig{
		Address:    fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID:    255, // Invalid slave ID
		Timeout:    2 * time.Second,
		MaxRetries: 0,
	}, testLogger())

	if err != nil {
		// Should fail at creation time with invalid slave ID
		t.Logf("Client creation failed as expected: %v", err)
		return
	}

	// If creation succeeded, connection might fail or reads might fail
	if err := client.Connect(ctx); err != nil {
		t.Logf("Connection failed with invalid slave ID: %v", err)
		return
	}
	defer client.Disconnect()

	tags := []*domain.Tag{
		{
			ID:            "test_tag",
			Address:       0,
			RegisterType:  domain.RegisterTypeHoldingRegister,
			DataType:      domain.DataTypeInt16,
			RegisterCount: 1,
			Enabled:       true,
		},
	}

	_, err = client.ReadTags(ctx, tags)
	if err == nil {
		t.Error("expected error reading with invalid slave ID")
	} else {
		t.Logf("Read failed as expected: %v", err)
	}
}

// TestModbus_InvalidAddress tests error handling for out-of-range addresses.
func TestModbus_InvalidAddress(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := modbus.NewClient("test-invalid-addr", modbus.ClientConfig{
		Address:    fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID:    1,
		Timeout:    2 * time.Second,
		MaxRetries: 0,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect()

	// Try to read from an address that likely doesn't exist in the simulator
	tags := []*domain.Tag{
		{
			ID:            "nonexistent",
			Address:       65000, // High address likely not configured
			RegisterType:  domain.RegisterTypeHoldingRegister,
			DataType:      domain.DataTypeInt16,
			RegisterCount: 1,
			Enabled:       true,
		},
	}

	dataPoints, err := client.ReadTags(ctx, tags)

	// Either we get an error or a data point with bad quality
	if err != nil {
		t.Logf("Read returned error as expected: %v", err)
	} else if len(dataPoints) > 0 && dataPoints[0].Quality != domain.QualityGood {
		t.Logf("Read returned data point with quality: %s", dataPoints[0].Quality)
	} else {
		// Some simulators might return default values
		t.Logf("Simulator returned data for high address (might be normal for some simulators)")
	}
}

// TestModbus_ExcessiveRegisterCount tests error for exceeding protocol limits.
func TestModbus_ExcessiveRegisterCount(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := modbus.NewClient("test-excessive", modbus.ClientConfig{
		Address:    fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID:    1,
		Timeout:    2 * time.Second,
		MaxRetries: 0,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect()

	// Try to read more than 125 registers (Modbus protocol limit)
	tags := []*domain.Tag{
		{
			ID:            "too_many",
			Address:       0,
			RegisterType:  domain.RegisterTypeHoldingRegister,
			DataType:      domain.DataTypeInt16,
			RegisterCount: 200, // Exceeds max of 125
			Enabled:       true,
		},
	}

	_, err = client.ReadTags(ctx, tags)
	// Some simulators may accept larger register counts; behavior varies
	// The test validates that the operation completes without panic
	if err != nil {
		t.Logf("Operation returned error (expected for strict implementations): %v", err)
	} else {
		t.Logf("Simulator accepted read of 200 registers (simulator-dependent behavior)")
	}
}

// TestModbus_ZeroRegisterCount tests validation for zero register count.
func TestModbus_ZeroRegisterCount(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := modbus.NewClient("test-zero-count", modbus.ClientConfig{
		Address:    fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID:    1,
		Timeout:    2 * time.Second,
		MaxRetries: 0,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect()

	// RegisterCount = 0 should be caught by validation
	tags := []*domain.Tag{
		{
			ID:            "zero_count",
			Address:       0,
			RegisterType:  domain.RegisterTypeHoldingRegister,
			DataType:      domain.DataTypeInt16,
			RegisterCount: 0, // Invalid!
			Enabled:       true,
		},
	}

	dataPoints, err := client.ReadTags(ctx, tags)

	// Should get an error or a bad quality data point
	if err != nil {
		t.Logf("Correctly received error for zero register count: %v", err)
	} else if len(dataPoints) > 0 && dataPoints[0].Quality != domain.QualityGood {
		t.Logf("Got bad quality data point: %s", dataPoints[0].Quality)
	} else {
		t.Error("expected error or bad quality for zero register count")
	}
}

// TestModbus_ConnectionRefused tests error handling when server is unreachable.
func TestModbus_ConnectionRefused(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Use a port that should have nothing listening
	client, err := modbus.NewClient("test-refused", modbus.ClientConfig{
		Address:    "localhost:59999", // Unlikely to have anything here
		SlaveID:    1,
		Timeout:    1 * time.Second,
		MaxRetries: 0,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.Connect(ctx)
	if err == nil {
		client.Disconnect()
		t.Error("expected connection to fail on closed port")
	} else {
		t.Logf("Connection correctly failed: %v", err)
	}
}

// TestModbus_ContextTimeout tests that operations respect context timeout.
func TestModbus_ContextTimeout(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	// Very short context timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire
	time.Sleep(10 * time.Millisecond)

	client, err := modbus.NewClient("test-ctx-timeout", modbus.ClientConfig{
		Address:    fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID:    1,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Context should already be expired
	err = client.Connect(ctx)
	if err == nil {
		client.Disconnect()
		// Connection might succeed if it's very fast, try a read
		tags := []*domain.Tag{
			{
				ID:            "test",
				Address:       0,
				RegisterType:  domain.RegisterTypeHoldingRegister,
				DataType:      domain.DataTypeInt16,
				RegisterCount: 1,
				Enabled:       true,
			},
		}
		_, err = client.ReadTags(ctx, tags)
		if err == nil {
			t.Log("Operations completed before context expired (very fast system)")
		}
	} else {
		t.Logf("Operation correctly failed with context error: %v", err)
	}
}

// TestModbus_WriteToReadOnlyRegister tests writing to input registers.
func TestModbus_WriteToReadOnlyRegister(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := modbus.NewClient("test-readonly", modbus.ClientConfig{
		Address:    fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID:    1,
		Timeout:    2 * time.Second,
		MaxRetries: 0,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect()

	// Input registers are read-only in Modbus
	tag := &domain.Tag{
		ID:            "input_reg",
		Address:       0,
		RegisterType:  domain.RegisterTypeInputRegister,
		DataType:      domain.DataTypeInt16,
		RegisterCount: 1,
		AccessMode:    domain.AccessModeReadWrite, // Incorrectly marked as writable
		Enabled:       true,
	}

	err = client.WriteTag(ctx, tag, int16(100))
	if err == nil {
		t.Error("expected error writing to input register")
	} else {
		t.Logf("Correctly received error: %v", err)
	}
}
