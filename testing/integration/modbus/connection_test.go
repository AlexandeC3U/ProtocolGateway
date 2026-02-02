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
	"github.com/rs/zerolog"
)

func testLogger() zerolog.Logger {
	return zerolog.Nop()
}

func TestModbus_TCPConnection(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := modbus.NewClient("test-device", modbus.ClientConfig{
		Address:    fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID:    1,
		Timeout:    5 * time.Second,
		MaxRetries: 3,
		RetryDelay: 500 * time.Millisecond,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Test connection
	err = client.Connect(ctx)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Verify connected
	if !client.IsConnected() {
		t.Error("expected client to be connected")
	}

	// Disconnect
	err = client.Disconnect()
	if err != nil {
		t.Errorf("failed to disconnect: %v", err)
	}

	if client.IsConnected() {
		t.Error("expected client to be disconnected")
	}
}

func TestModbus_ReadHoldingRegisters(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := modbus.NewClient("test-device-read", modbus.ClientConfig{
		Address: fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID: 1,
		Timeout: 5 * time.Second,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect()

	tags := []*domain.Tag{
		{
			ID:            "temp",
			Address:       0,
			RegisterType:  domain.RegisterTypeHoldingRegister,
			DataType:      domain.DataTypeInt16,
			RegisterCount: 1,
			ByteOrder:     domain.ByteOrderBigEndian,
			Enabled:       true,
		},
	}

	dataPoints, err := client.ReadTags(ctx, tags)
	if err != nil {
		t.Fatalf("failed to read tags: %v", err)
	}

	if len(dataPoints) != 1 {
		t.Errorf("expected 1 data point, got %d", len(dataPoints))
	}

	dp := dataPoints[0]
	if dp.Quality != domain.QualityGood {
		t.Errorf("expected good quality, got %s", dp.Quality)
	}
}

func TestModbus_ReadInputRegisters(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := modbus.NewClient("test-device-input", modbus.ClientConfig{
		Address: fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID: 1,
		Timeout: 5 * time.Second,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect()

	tags := []*domain.Tag{
		{
			ID:            "sensor",
			Address:       0,
			RegisterType:  domain.RegisterTypeInputRegister,
			DataType:      domain.DataTypeUInt16,
			RegisterCount: 1,
			ByteOrder:     domain.ByteOrderBigEndian,
			Enabled:       true,
		},
	}

	dataPoints, err := client.ReadTags(ctx, tags)
	if err != nil {
		t.Fatalf("failed to read tags: %v", err)
	}

	if len(dataPoints) != 1 {
		t.Errorf("expected 1 data point, got %d", len(dataPoints))
	}
}

func TestModbus_ReadCoils(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := modbus.NewClient("test-device-coils", modbus.ClientConfig{
		Address: fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID: 1,
		Timeout: 5 * time.Second,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect()

	tags := []*domain.Tag{
		{
			ID:            "coil_0",
			Address:       0,
			RegisterType:  domain.RegisterTypeCoil,
			DataType:      domain.DataTypeBool,
			RegisterCount: 1,
			Enabled:       true,
		},
	}

	dataPoints, err := client.ReadTags(ctx, tags)
	if err != nil {
		t.Fatalf("failed to read coils: %v", err)
	}

	if len(dataPoints) != 1 {
		t.Errorf("expected 1 data point, got %d", len(dataPoints))
	}

	// Should be a boolean
	if _, ok := dataPoints[0].Value.(bool); !ok {
		t.Errorf("expected bool value, got %T", dataPoints[0].Value)
	}
}

func TestModbus_WriteCoil(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := modbus.NewClient("test-device-write", modbus.ClientConfig{
		Address: fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID: 1,
		Timeout: 5 * time.Second,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect()

	tag := &domain.Tag{
		ID:            "coil_write",
		Address:       10,
		RegisterType:  domain.RegisterTypeCoil,
		DataType:      domain.DataTypeBool,
		RegisterCount: 1,
		AccessMode:    domain.AccessModeReadWrite,
		Enabled:       true,
	}

	// Write true
	err = client.WriteTag(ctx, tag, true)
	if err != nil {
		t.Fatalf("failed to write coil: %v", err)
	}

	// Read back and verify
	dataPoints, err := client.ReadTags(ctx, []*domain.Tag{tag})
	if err != nil {
		t.Fatalf("failed to read coil: %v", err)
	}

	if dataPoints[0].Value != true {
		t.Errorf("expected true after write, got %v", dataPoints[0].Value)
	}

	// Write false
	err = client.WriteTag(ctx, tag, false)
	if err != nil {
		t.Fatalf("failed to write coil false: %v", err)
	}

	// Read back
	dataPoints, err = client.ReadTags(ctx, []*domain.Tag{tag})
	if err != nil {
		t.Fatalf("failed to read coil: %v", err)
	}

	if dataPoints[0].Value != false {
		t.Errorf("expected false after write, got %v", dataPoints[0].Value)
	}
}

func TestModbus_ConnectionTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Non-routable address should timeout
	client, err := modbus.NewClient("test-device-timeout", modbus.ClientConfig{
		Address:    "10.255.255.1:502",
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
		t.Error("expected connection to fail")
	}
}

func TestModbus_Reconnection(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := modbus.NewClient("test-device-reconnect", modbus.ClientConfig{
		Address:    fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID:    1,
		Timeout:    5 * time.Second,
		MaxRetries: 3,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Connect
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Disconnect
	client.Disconnect()

	// Reconnect
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to reconnect: %v", err)
	}

	if !client.IsConnected() {
		t.Error("expected to be reconnected")
	}

	client.Disconnect()
}
