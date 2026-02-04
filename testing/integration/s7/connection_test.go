//go:build integration
// +build integration

// Package s7_test provides integration tests for S7/Siemens PLC communication.
package s7_test

import (
	"context"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/s7"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/nexus-edge/protocol-gateway/testing/integration"
	"github.com/rs/zerolog"
)

func testLogger() zerolog.Logger {
	return zerolog.Nop()
}

// =============================================================================
// Connection Lifecycle Tests
// =============================================================================

// TestS7_BasicConnection tests basic S7 PLC connection.
func TestS7_BasicConnection(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	t.Logf("Testing against S7 PLC at %s:%d", cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("test-s7-device", s7.ClientConfig{
		Address:    cfg.S7Host,
		Port:       cfg.S7Port,
		Rack:       0,
		Slot:       1, // S7-1200/1500 typically slot 1
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

// TestS7_ConnectionTimeout tests connection timeout handling.
func TestS7_ConnectionTimeout(t *testing.T) {
	// Use non-routable IP to trigger timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client, err := s7.NewClient("timeout-test", s7.ClientConfig{
		Address: "10.255.255.1", // Non-routable IP
		Port:    102,
		Rack:    0,
		Slot:    1,
		Timeout: 1 * time.Second,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.Connect(ctx)
	if err == nil {
		client.Disconnect()
		t.Fatal("expected connection to fail")
	}

	t.Logf("Connection properly failed: %v", err)
}

// TestS7_ReconnectAfterDisconnect tests reconnection capability.
func TestS7_ReconnectAfterDisconnect(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("reconnect-test", s7.ClientConfig{
		Address: cfg.S7Host,
		Port:    cfg.S7Port,
		Rack:    0,
		Slot:    1,
		Timeout: 5 * time.Second,
	}, testLogger())
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

	// Disconnect
	if err := client.Disconnect(); err != nil {
		t.Errorf("disconnect failed: %v", err)
	}

	// Reconnect
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("reconnect failed: %v", err)
	}

	if !client.IsConnected() {
		t.Error("expected client to be connected after reconnect")
	}

	client.Disconnect()
}

// =============================================================================
// Read Operations Tests
// =============================================================================

// TestS7_ReadDataBlock tests reading from a data block.
func TestS7_ReadDataBlock(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("read-db-test", s7.ClientConfig{
		Address: cfg.S7Host,
		Port:    cfg.S7Port,
		Rack:    0,
		Slot:    1,
		Timeout: 5 * time.Second,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect()

	// Read from DB1 - common test data block
	tags := []*domain.Tag{
		{
			ID:         "db1-int16",
			S7Address:  "DB1.DBW0", // DB1, Word at offset 0
			DataType:   domain.DataTypeInt16,
			S7Area:     domain.S7AreaDB,
			S7DBNumber: 1,
			S7Offset:   0,
			Enabled:    true,
		},
	}

	dataPoints, err := client.ReadTags(ctx, tags)
	if err != nil {
		t.Fatalf("failed to read tags: %v", err)
	}

	if len(dataPoints) != 1 {
		t.Fatalf("expected 1 datapoint, got %d", len(dataPoints))
	}

	t.Logf("Read DB1.DBW0: %v", dataPoints[0].Value)
}

// TestS7_ReadMerker tests reading from Merker (flag) area.
func TestS7_ReadMerker(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("read-merker-test", s7.ClientConfig{
		Address: cfg.S7Host,
		Port:    cfg.S7Port,
		Rack:    0,
		Slot:    1,
		Timeout: 5 * time.Second,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect()

	// Read Merker byte
	tags := []*domain.Tag{
		{
			ID:        "merker-byte",
			S7Address: "MB0", // Merker byte 0
			DataType:  domain.DataTypeInt16,
			S7Area:    domain.S7AreaM,
			S7Offset:  0,
			Enabled:   true,
		},
	}

	dataPoints, err := client.ReadTags(ctx, tags)
	if err != nil {
		t.Fatalf("failed to read tags: %v", err)
	}

	if len(dataPoints) != 1 {
		t.Fatalf("expected 1 datapoint, got %d", len(dataPoints))
	}

	t.Logf("Read MB0: %v", dataPoints[0].Value)
}

// TestS7_ReadMultipleDataTypes tests reading various S7 data types.
func TestS7_ReadMultipleDataTypes(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("read-types-test", s7.ClientConfig{
		Address: cfg.S7Host,
		Port:    cfg.S7Port,
		Rack:    0,
		Slot:    1,
		Timeout: 5 * time.Second,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect()

	testCases := []struct {
		name        string
		s7Address   string
		dataType    domain.DataType
		s7DBNumber  int
		s7Offset    int
		s7BitOffset int
	}{
		{"Bool", "DB1.DBX0.0", domain.DataTypeBool, 1, 0, 0},
		{"Byte", "DB1.DBB1", domain.DataTypeInt16, 1, 1, 0},
		{"Int16", "DB1.DBW2", domain.DataTypeInt16, 1, 2, 0},
		{"Int32", "DB1.DBD4", domain.DataTypeInt32, 1, 4, 0},
		{"Float32", "DB1.DBD8", domain.DataTypeFloat32, 1, 8, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tag := &domain.Tag{
				ID:          "test-" + tc.name,
				S7Address:   tc.s7Address,
				DataType:    tc.dataType,
				S7Area:      domain.S7AreaDB,
				S7DBNumber:  tc.s7DBNumber,
				S7Offset:    tc.s7Offset,
				S7BitOffset: tc.s7BitOffset,
				Enabled:     true,
			}

			dataPoints, err := client.ReadTags(ctx, []*domain.Tag{tag})
			if err != nil {
				t.Fatalf("failed to read %s: %v", tc.name, err)
			}

			if len(dataPoints) != 1 {
				t.Fatalf("expected 1 datapoint, got %d", len(dataPoints))
			}

			t.Logf("Read %s (%s): %v", tc.s7Address, tc.dataType, dataPoints[0].Value)
		})
	}
}

// =============================================================================
// Write Operations Tests
// =============================================================================

// TestS7_WriteDataBlock tests writing to a data block.
func TestS7_WriteDataBlock(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("write-db-test", s7.ClientConfig{
		Address: cfg.S7Host,
		Port:    cfg.S7Port,
		Rack:    0,
		Slot:    1,
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
		ID:         "write-test",
		S7Address:  "DB1.DBW100", // Write to offset 100 to avoid conflicts
		DataType:   domain.DataTypeInt16,
		S7Area:     domain.S7AreaDB,
		S7DBNumber: 1,
		S7Offset:   100,
		AccessMode: domain.AccessModeReadWrite,
		Enabled:    true,
	}

	// Write a test value
	testValue := int16(12345)
	err = client.WriteTag(ctx, tag, testValue)
	if err != nil {
		t.Fatalf("failed to write tag: %v", err)
	}

	// Read back and verify
	dataPoints, err := client.ReadTags(ctx, []*domain.Tag{tag})
	if err != nil {
		t.Fatalf("failed to read back tag: %v", err)
	}

	if len(dataPoints) != 1 {
		t.Fatalf("expected 1 datapoint, got %d", len(dataPoints))
	}

	// Check the value
	readValue, ok := dataPoints[0].Value.(int16)
	if !ok {
		// Try int64 conversion (common with some drivers)
		if v, ok := dataPoints[0].Value.(int64); ok {
			readValue = int16(v)
		} else {
			t.Fatalf("unexpected value type: %T", dataPoints[0].Value)
		}
	}

	if readValue != testValue {
		t.Errorf("expected %d, got %d", testValue, readValue)
	}

	t.Logf("Successfully wrote and read back: %d", readValue)
}

// =============================================================================
// Error Handling Tests
// =============================================================================

// TestS7_InvalidAddress tests handling of invalid addresses.
func TestS7_InvalidAddress(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("invalid-addr-test", s7.ClientConfig{
		Address: cfg.S7Host,
		Port:    cfg.S7Port,
		Rack:    0,
		Slot:    1,
		Timeout: 5 * time.Second,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect()

	// Try to read from non-existent DB
	tag := &domain.Tag{
		ID:         "invalid-db",
		S7Address:  "DB999.DBW0", // Non-existent DB
		DataType:   domain.DataTypeInt16,
		S7Area:     domain.S7AreaDB,
		S7DBNumber: 999,
		S7Offset:   0,
		Enabled:    true,
	}

	_, err = client.ReadTags(ctx, []*domain.Tag{tag})
	if err == nil {
		t.Error("expected error reading non-existent DB")
	} else {
		t.Logf("Correctly got error: %v", err)
	}
}

// TestS7_ClientStats tests that statistics are properly tracked.
func TestS7_ClientStats(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("stats-test", s7.ClientConfig{
		Address: cfg.S7Host,
		Port:    cfg.S7Port,
		Rack:    0,
		Slot:    1,
		Timeout: 5 * time.Second,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect()

	// Perform some reads
	tag := &domain.Tag{
		ID:         "stats-tag",
		S7Address:  "DB1.DBW0",
		DataType:   domain.DataTypeInt16,
		S7Area:     domain.S7AreaDB,
		S7DBNumber: 1,
		S7Offset:   0,
		Enabled:    true,
	}

	for i := 0; i < 5; i++ {
		_, err := client.ReadTags(ctx, []*domain.Tag{tag})
		if err != nil {
			t.Logf("Read %d failed: %v", i, err)
		}
	}

	stats := client.GetStats()
	t.Logf("Client stats: ReadCount=%d, WriteCount=%d, ErrorCount=%d",
		stats["read_count"], stats["write_count"], stats["error_count"])

	if stats["read_count"] == 0 {
		t.Error("expected ReadCount > 0")
	}
}
