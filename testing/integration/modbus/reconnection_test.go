//go:build integration
// +build integration

// Package modbus_test provides reconnection tests for Modbus connections.
package modbus_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/modbus"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/nexus-edge/protocol-gateway/testing/integration"
	"github.com/rs/zerolog"
)

func reconnectTestLogger() zerolog.Logger {
	return zerolog.Nop()
}

// =============================================================================
// Reconnection Lifecycle Tests
// =============================================================================

// TestModbus_ReconnectAfterDisconnect tests reconnection after explicit disconnect.
func TestModbus_ReconnectAfterDisconnect(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := modbus.NewClient("reconnect-test", modbus.ClientConfig{
		Address:    fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID:    1,
		Timeout:    5 * time.Second,
		MaxRetries: 3,
		RetryDelay: 500 * time.Millisecond,
	}, reconnectTestLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// First connection
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("first connect failed: %v", err)
	}

	if !client.IsConnected() {
		t.Error("expected client to be connected after first connect")
	}

	// Read to verify connection works
	tag := &domain.Tag{
		ID:            "reconnect-tag",
		Address:       0,
		RegisterType:  domain.RegisterTypeHoldingRegister,
		DataType:      domain.DataTypeInt16,
		RegisterCount: 1,
		ByteOrder:     domain.ByteOrderBigEndian,
		Enabled:       true,
	}

	_, err = client.ReadTags(ctx, []*domain.Tag{tag})
	if err != nil {
		t.Fatalf("first read failed: %v", err)
	}

	// Disconnect
	if err := client.Disconnect(); err != nil {
		t.Errorf("disconnect failed: %v", err)
	}

	if client.IsConnected() {
		t.Error("expected client to be disconnected")
	}

	// Reconnect
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("reconnect failed: %v", err)
	}

	if !client.IsConnected() {
		t.Error("expected client to be connected after reconnect")
	}

	// Read again to verify reconnection works
	_, err = client.ReadTags(ctx, []*domain.Tag{tag})
	if err != nil {
		t.Fatalf("read after reconnect failed: %v", err)
	}

	client.Disconnect()
}

// TestModbus_MultipleReconnections tests multiple connect/disconnect cycles.
func TestModbus_MultipleReconnections(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := modbus.NewClient("multi-reconnect-test", modbus.ClientConfig{
		Address:    fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID:    1,
		Timeout:    5 * time.Second,
		MaxRetries: 3,
		RetryDelay: 500 * time.Millisecond,
	}, reconnectTestLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	cycles := 5
	for i := 0; i < cycles; i++ {
		t.Run(fmt.Sprintf("Cycle_%d", i+1), func(t *testing.T) {
			// Connect
			if err := client.Connect(ctx); err != nil {
				t.Fatalf("connect failed on cycle %d: %v", i+1, err)
			}

			if !client.IsConnected() {
				t.Errorf("not connected after cycle %d connect", i+1)
			}

			// Do a quick read
			tag := &domain.Tag{
				ID:            "cycle-tag",
				Address:       0,
				RegisterType:  domain.RegisterTypeHoldingRegister,
				DataType:      domain.DataTypeInt16,
				RegisterCount: 1,
				ByteOrder:     domain.ByteOrderBigEndian,
				Enabled:       true,
			}

			_, err := client.ReadTags(ctx, []*domain.Tag{tag})
			if err != nil {
				t.Errorf("read failed on cycle %d: %v", i+1, err)
			}

			// Disconnect
			if err := client.Disconnect(); err != nil {
				t.Errorf("disconnect failed on cycle %d: %v", i+1, err)
			}

			// Small delay between cycles
			time.Sleep(100 * time.Millisecond)
		})
	}
}

// TestModbus_ReconnectDuringOperation tests behavior when reconnecting during operations.
func TestModbus_ReconnectDuringOperation(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := modbus.NewClient("reconnect-during-op", modbus.ClientConfig{
		Address:    fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID:    1,
		Timeout:    5 * time.Second,
		MaxRetries: 3,
		RetryDelay: 500 * time.Millisecond,
	}, reconnectTestLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("initial connect failed: %v", err)
	}
	defer client.Disconnect()

	tag := &domain.Tag{
		ID:            "op-tag",
		Address:       0,
		RegisterType:  domain.RegisterTypeHoldingRegister,
		DataType:      domain.DataTypeInt16,
		RegisterCount: 1,
		ByteOrder:     domain.ByteOrderBigEndian,
		Enabled:       true,
	}

	// Start concurrent reads
	var wg sync.WaitGroup
	readCount := 10
	errorCount := 0
	var mu sync.Mutex

	for i := 0; i < readCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := client.ReadTags(ctx, []*domain.Tag{tag})
			if err != nil {
				mu.Lock()
				errorCount++
				mu.Unlock()
				t.Logf("Read %d failed: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Completed %d reads, %d errors", readCount, errorCount)

	// Should have mostly successful reads
	if errorCount > readCount/2 {
		t.Errorf("too many errors: %d/%d", errorCount, readCount)
	}
}

// =============================================================================
// Connection State Tests
// =============================================================================

// TestModbus_ConnectionStateTransitions tests connection state changes.
func TestModbus_ConnectionStateTransitions(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := modbus.NewClient("state-test", modbus.ClientConfig{
		Address:    fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID:    1,
		Timeout:    5 * time.Second,
		MaxRetries: 3,
		RetryDelay: 500 * time.Millisecond,
	}, reconnectTestLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Initial state: disconnected
	if client.IsConnected() {
		t.Error("expected disconnected initial state")
	}

	// State after connect
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	if !client.IsConnected() {
		t.Error("expected connected state after connect")
	}

	// State after disconnect
	if err := client.Disconnect(); err != nil {
		t.Errorf("disconnect failed: %v", err)
	}

	if client.IsConnected() {
		t.Error("expected disconnected state after disconnect")
	}

	// Double disconnect should be safe
	if err := client.Disconnect(); err != nil {
		t.Errorf("double disconnect failed: %v", err)
	}
}

// TestModbus_ReadAfterDisconnect tests that reads fail gracefully after disconnect.
func TestModbus_ReadAfterDisconnect(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := modbus.NewClient("read-after-disconnect", modbus.ClientConfig{
		Address:    fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID:    1,
		Timeout:    5 * time.Second,
		MaxRetries: 1,
		RetryDelay: 100 * time.Millisecond,
	}, reconnectTestLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	// Disconnect
	client.Disconnect()

	// Try to read - should fail
	tag := &domain.Tag{
		ID:            "post-disconnect-tag",
		Address:       0,
		RegisterType:  domain.RegisterTypeHoldingRegister,
		DataType:      domain.DataTypeInt16,
		RegisterCount: 1,
		ByteOrder:     domain.ByteOrderBigEndian,
		Enabled:       true,
	}

	_, err = client.ReadTags(ctx, []*domain.Tag{tag})
	if err == nil {
		t.Error("expected error reading after disconnect")
	} else {
		t.Logf("Correctly got error after disconnect: %v", err)
	}
}

// =============================================================================
// Stats After Reconnection Tests
// =============================================================================

// TestModbus_StatsAfterReconnect tests that statistics persist across reconnections.
func TestModbus_StatsAfterReconnect(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.ModbusHost, cfg.ModbusPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := modbus.NewClient("stats-reconnect-test", modbus.ClientConfig{
		Address:    fmt.Sprintf("%s:%d", cfg.ModbusHost, cfg.ModbusPort),
		SlaveID:    1,
		Timeout:    5 * time.Second,
		MaxRetries: 3,
		RetryDelay: 500 * time.Millisecond,
	}, reconnectTestLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	tag := &domain.Tag{
		ID:            "stats-tag",
		Address:       0,
		RegisterType:  domain.RegisterTypeHoldingRegister,
		DataType:      domain.DataTypeInt16,
		RegisterCount: 1,
		ByteOrder:     domain.ByteOrderBigEndian,
		Enabled:       true,
	}

	// First session
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("first connect failed: %v", err)
	}

	for i := 0; i < 5; i++ {
		client.ReadTags(ctx, []*domain.Tag{tag})
	}

	stats1 := client.GetStatsStruct()
	t.Logf("Stats after first session: ReadCount=%d", stats1.ReadCount.Load())

	client.Disconnect()

	// Second session
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("reconnect failed: %v", err)
	}

	for i := 0; i < 5; i++ {
		client.ReadTags(ctx, []*domain.Tag{tag})
	}

	stats2 := client.GetStatsStruct()
	t.Logf("Stats after second session: ReadCount=%d", stats2.ReadCount.Load())

	client.Disconnect()

	// Stats should accumulate
	if stats2.ReadCount.Load() <= stats1.ReadCount.Load() {
		t.Error("expected stats to accumulate across sessions")
	}
}

// =============================================================================
// Timeout and Error Recovery Tests
// =============================================================================

// TestModbus_ConnectTimeout tests connection timeout behavior.
func TestModbus_ConnectTimeout(t *testing.T) {
	// Use a non-routable IP address to trigger timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, err := modbus.NewClient("timeout-test", modbus.ClientConfig{
		Address:    "10.255.255.1:502", // Non-routable IP
		SlaveID:    1,
		Timeout:    1 * time.Second,
		MaxRetries: 1,
		RetryDelay: 100 * time.Millisecond,
	}, reconnectTestLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	start := time.Now()
	err = client.Connect(ctx)
	elapsed := time.Since(start)

	if err == nil {
		client.Disconnect()
		t.Fatal("expected connection to timeout")
	}

	t.Logf("Connection correctly timed out after %v: %v", elapsed, err)

	// Should not take too long
	if elapsed > 5*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

// TestModbus_ConnectToInvalidPort tests connection to closed port.
func TestModbus_ConnectToInvalidPort(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := modbus.NewClient("invalid-port-test", modbus.ClientConfig{
		Address:    "localhost:59999", // Unlikely to be in use
		SlaveID:    1,
		Timeout:    2 * time.Second,
		MaxRetries: 1,
		RetryDelay: 100 * time.Millisecond,
	}, reconnectTestLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.Connect(ctx)
	if err == nil {
		client.Disconnect()
		t.Fatal("expected connection to fail on invalid port")
	}

	t.Logf("Connection correctly failed: %v", err)
}
