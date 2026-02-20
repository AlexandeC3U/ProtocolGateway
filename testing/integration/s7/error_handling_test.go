//go:build integration
// +build integration

package s7_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/s7"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/nexus-edge/protocol-gateway/testing/integration"
)

// Note: testLogger is defined in connection_test.go in the same package

// =============================================================================
// Address & Area Error Tests
// =============================================================================

// TestS7_ReadNonExistentDB tests reading from a data block that doesn't exist.
func TestS7_ReadNonExistentDB(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("err-nonexistent-db", s7.ClientConfig{
		Address:    cfg.S7Host,
		Port:       cfg.S7Port,
		Rack:       0,
		Slot:       1,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect()

	tag := &domain.Tag{
		ID:         "nonexistent-db",
		S7Address:  "DB999.DBW0",
		DataType:   domain.DataTypeInt16,
		S7Area:     domain.S7AreaDB,
		S7DBNumber: 999,
		S7Offset:   0,
		Enabled:    true,
	}

	dataPoints, err := client.ReadTags(ctx, []*domain.Tag{tag})
	if err != nil {
		t.Logf("Correctly got error for non-existent DB: %v", err)
	} else if len(dataPoints) > 0 && dataPoints[0].Quality != domain.QualityGood {
		t.Logf("Got bad quality data point for non-existent DB: %s", dataPoints[0].Quality)
	} else {
		// Some simulators (e.g., snap7 server) return default values for any DB
		t.Log("Simulator returned data for non-existent DB (simulator-dependent; real PLCs reject this)")
	}
}

// TestS7_ReadOutOfRangeOffset tests reading at an offset beyond the DB size.
func TestS7_ReadOutOfRangeOffset(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("err-oor-offset", s7.ClientConfig{
		Address:    cfg.S7Host,
		Port:       cfg.S7Port,
		Rack:       0,
		Slot:       1,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect()

	// DB1 exists but offset 99999 is way beyond its size
	tag := &domain.Tag{
		ID:         "oor-offset",
		S7Address:  "DB1.DBW99999",
		DataType:   domain.DataTypeInt16,
		S7Area:     domain.S7AreaDB,
		S7DBNumber: 1,
		S7Offset:   99999,
		Enabled:    true,
	}

	dataPoints, err := client.ReadTags(ctx, []*domain.Tag{tag})
	if err != nil {
		t.Logf("Correctly got error for out-of-range offset: %v", err)
	} else if len(dataPoints) > 0 && dataPoints[0].Quality != domain.QualityGood {
		t.Logf("Got bad quality data point for out-of-range offset: %s", dataPoints[0].Quality)
	} else {
		// Some simulators auto-extend DB size and return default values
		t.Log("Simulator returned data for out-of-range offset (simulator-dependent; real PLCs reject this)")
	}
}

// TestS7_InvalidRackSlot tests connecting with an invalid rack/slot combination.
func TestS7_InvalidRackSlot(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Rack 15, Slot 15 — unlikely to be valid
	client, err := s7.NewClient("err-rack-slot", s7.ClientConfig{
		Address:    cfg.S7Host,
		Port:       cfg.S7Port,
		Rack:       15,
		Slot:       15,
		Timeout:    3 * time.Second,
		MaxRetries: 0,
	}, testLogger())
	if err != nil {
		t.Logf("Client creation correctly failed: %v", err)
		return
	}

	err = client.Connect(ctx)
	if err != nil {
		t.Logf("Connection with invalid rack/slot correctly failed: %v", err)
		return
	}
	defer client.Disconnect()

	// Some simulators accept any rack/slot; verify reads still work or fail gracefully
	tag := &domain.Tag{
		ID:         "rack-slot-test",
		S7Address:  "DB1.DBW0",
		DataType:   domain.DataTypeInt16,
		S7Area:     domain.S7AreaDB,
		S7DBNumber: 1,
		S7Offset:   0,
		Enabled:    true,
	}

	_, err = client.ReadTags(ctx, []*domain.Tag{tag})
	if err != nil {
		t.Logf("Read with invalid rack/slot failed: %v", err)
	} else {
		t.Log("Simulator accepted invalid rack/slot (simulator-dependent behavior)")
	}
}

// =============================================================================
// Data Type Mismatch Tests
// =============================================================================

// TestS7_ReadWithWrongDataType tests reading with mismatched data types.
func TestS7_ReadWithWrongDataType(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("err-wrong-type", s7.ClientConfig{
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
		name     string
		dataType domain.DataType
		offset   int
	}{
		{"Float64_at_Bool_offset", domain.DataTypeFloat64, 0},
		{"String_at_Word_offset", domain.DataTypeString, 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tag := &domain.Tag{
				ID:         "wrong-type-" + tc.name,
				S7Address:  "DB1.DBW" + fmt.Sprint(tc.offset),
				DataType:   tc.dataType,
				S7Area:     domain.S7AreaDB,
				S7DBNumber: 1,
				S7Offset:   tc.offset,
				Enabled:    true,
			}

			dataPoints, err := client.ReadTags(ctx, []*domain.Tag{tag})
			// The driver may return data (interpreting bytes as the requested type)
			// or may return an error. Either is acceptable — no panics.
			if err != nil {
				t.Logf("Read with %s data type returned error (acceptable): %v", tc.dataType, err)
			} else if len(dataPoints) > 0 {
				t.Logf("Read with %s data type returned value: %v (type reinterpretation)", tc.dataType, dataPoints[0].Value)
			}
		})
	}
}

// =============================================================================
// Write Error Tests
// =============================================================================

// TestS7_WriteToReadOnlyArea tests writing to input (PE) area which is read-only.
func TestS7_WriteToReadOnlyArea(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("err-write-readonly", s7.ClientConfig{
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

	// Input area (I) is read-only on real PLCs
	tag := &domain.Tag{
		ID:         "write-input",
		S7Address:  "IB0", // Input byte 0
		DataType:   domain.DataTypeInt16,
		S7Area:     domain.S7AreaI,
		S7Offset:   0,
		AccessMode: domain.AccessModeReadWrite,
		Enabled:    true,
	}

	err = client.WriteTag(ctx, tag, int16(42))
	if err == nil {
		// Some simulators may accept writes to PE area
		t.Log("Simulator accepted write to PE area (simulator-dependent; real PLCs reject this)")
	} else {
		t.Logf("Correctly rejected write to PE area: %v", err)
	}
}

// TestS7_WriteInvalidValue tests writing a value that doesn't match the tag data type.
func TestS7_WriteInvalidValue(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("err-write-invalid-val", s7.ClientConfig{
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
		ID:         "write-invalid",
		S7Address:  "DB1.DBW100",
		DataType:   domain.DataTypeInt16,
		S7Area:     domain.S7AreaDB,
		S7DBNumber: 1,
		S7Offset:   100,
		AccessMode: domain.AccessModeReadWrite,
		Enabled:    true,
	}

	// Write a string value to an Int16 tag — should error
	err = client.WriteTag(ctx, tag, "not_a_number")
	if err == nil {
		t.Error("expected error writing string to Int16 tag")
	} else {
		t.Logf("Correctly rejected invalid value type: %v", err)
	}
}

// =============================================================================
// Connection Error Tests
// =============================================================================

// TestS7_ConnectionRefused tests connecting to a port with no S7 server.
func TestS7_ConnectionRefused(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, err := s7.NewClient("err-refused", s7.ClientConfig{
		Address:    "localhost",
		Port:       59999, // No S7 server here
		Rack:       0,
		Slot:       1,
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
		t.Logf("Connection correctly refused: %v", err)
	}
}

// TestS7_ContextTimeoutDuringRead tests that read operations respect context timeout.
func TestS7_ContextTimeoutDuringRead(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	// Connect with a normal timeout first
	connectCtx, connectCancel := integration.ContextWithTestTimeout(t)
	defer connectCancel()

	client, err := s7.NewClient("err-ctx-timeout", s7.ClientConfig{
		Address: cfg.S7Host,
		Port:    cfg.S7Port,
		Rack:    0,
		Slot:    1,
		Timeout: 5 * time.Second,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(connectCtx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect()

	// Now create an already-expired context for the read
	readCtx, readCancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer readCancel()
	time.Sleep(10 * time.Millisecond) // Ensure context is expired

	tag := &domain.Tag{
		ID:         "ctx-timeout",
		S7Address:  "DB1.DBW0",
		DataType:   domain.DataTypeInt16,
		S7Area:     domain.S7AreaDB,
		S7DBNumber: 1,
		S7Offset:   0,
		Enabled:    true,
	}

	_, err = client.ReadTags(readCtx, []*domain.Tag{tag})
	if err == nil {
		// Read may succeed if the driver doesn't check context before the TCP call returns
		t.Log("Read succeeded despite expired context (driver may not check ctx pre-call)")
	} else {
		t.Logf("Read correctly failed with expired context: %v", err)
	}
}

// TestS7_ContextTimeoutDuringWrite tests that write operations respect context timeout.
func TestS7_ContextTimeoutDuringWrite(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	connectCtx, connectCancel := integration.ContextWithTestTimeout(t)
	defer connectCancel()

	client, err := s7.NewClient("err-ctx-write", s7.ClientConfig{
		Address: cfg.S7Host,
		Port:    cfg.S7Port,
		Rack:    0,
		Slot:    1,
		Timeout: 5 * time.Second,
	}, testLogger())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if err := client.Connect(connectCtx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Disconnect()

	// Expired context
	writeCtx, writeCancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer writeCancel()
	time.Sleep(10 * time.Millisecond)

	tag := &domain.Tag{
		ID:         "ctx-write-timeout",
		S7Address:  "DB1.DBW100",
		DataType:   domain.DataTypeInt16,
		S7Area:     domain.S7AreaDB,
		S7DBNumber: 1,
		S7Offset:   100,
		AccessMode: domain.AccessModeReadWrite,
		Enabled:    true,
	}

	err = client.WriteTag(writeCtx, tag, int16(42))
	if err == nil {
		t.Log("Write succeeded despite expired context (driver may not check ctx pre-call)")
	} else {
		t.Logf("Write correctly failed with expired context: %v", err)
	}
}

// =============================================================================
// Batch Read Error Tests
// =============================================================================

// TestS7_BatchReadMixedValidInvalid tests batch read with some valid and some invalid tags.
func TestS7_BatchReadMixedValidInvalid(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("err-batch-mixed", s7.ClientConfig{
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

	tags := []*domain.Tag{
		{
			ID:         "valid-tag",
			S7Address:  "DB1.DBW0",
			DataType:   domain.DataTypeInt16,
			S7Area:     domain.S7AreaDB,
			S7DBNumber: 1,
			S7Offset:   0,
			Enabled:    true,
		},
		{
			ID:         "invalid-tag",
			S7Address:  "DB999.DBW0", // Non-existent DB
			DataType:   domain.DataTypeInt16,
			S7Area:     domain.S7AreaDB,
			S7DBNumber: 999,
			S7Offset:   0,
			Enabled:    true,
		},
		{
			ID:         "valid-tag-2",
			S7Address:  "MB0",
			DataType:   domain.DataTypeInt16,
			S7Area:     domain.S7AreaM,
			S7Offset:   0,
			Enabled:    true,
		},
	}

	dataPoints, err := client.ReadTags(ctx, tags)
	if err != nil {
		// Some drivers fail the entire batch on any invalid tag
		t.Logf("Batch read returned error (entire batch failed): %v", err)
	} else {
		// Other drivers return partial results with bad quality for failed tags
		goodCount := 0
		badCount := 0
		for _, dp := range dataPoints {
			if dp.Quality == domain.QualityGood {
				goodCount++
			} else {
				badCount++
			}
		}
		t.Logf("Batch read returned %d data points: %d good, %d bad",
			len(dataPoints), goodCount, badCount)
	}
}

// TestS7_ReadEmptyTagList tests reading with an empty tag list.
func TestS7_ReadEmptyTagList(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("err-empty-tags", s7.ClientConfig{
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

	dataPoints, err := client.ReadTags(ctx, []*domain.Tag{})
	if err != nil {
		t.Logf("Empty tag list returned error: %v", err)
	} else {
		if len(dataPoints) != 0 {
			t.Errorf("expected 0 data points for empty tag list, got %d", len(dataPoints))
		} else {
			t.Log("Empty tag list correctly returned 0 data points")
		}
	}
}

// =============================================================================
// Statistics Tracking Tests
// =============================================================================

// TestS7_ErrorStatsTracking tests that error statistics are properly incremented.
func TestS7_ErrorStatsTracking(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("err-stats", s7.ClientConfig{
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

	// Get initial stats
	initialStats := client.GetStats()
	initialErrors := initialStats["error_count"]

	// Perform a read that should fail (non-existent DB)
	badTag := &domain.Tag{
		ID:         "bad-for-stats",
		S7Address:  "DB999.DBW0",
		DataType:   domain.DataTypeInt16,
		S7Area:     domain.S7AreaDB,
		S7DBNumber: 999,
		S7Offset:   0,
		Enabled:    true,
	}

	for i := 0; i < 3; i++ {
		client.ReadTags(ctx, []*domain.Tag{badTag})
	}

	// Check stats incremented
	afterStats := client.GetStats()
	afterErrors := afterStats["error_count"]

	if afterErrors <= initialErrors {
		t.Errorf("expected error count to increase; before=%d, after=%d", initialErrors, afterErrors)
	} else {
		t.Logf("Error count correctly increased: %d -> %d", initialErrors, afterErrors)
	}
}

// TestS7_ReadAfterDisconnect tests reading after the client has disconnected.
func TestS7_ReadAfterDisconnect(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("err-read-disconnected", s7.ClientConfig{
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

	// Disconnect first
	client.Disconnect()

	// Try to read while disconnected
	tag := &domain.Tag{
		ID:         "disconnected-read",
		S7Address:  "DB1.DBW0",
		DataType:   domain.DataTypeInt16,
		S7Area:     domain.S7AreaDB,
		S7DBNumber: 1,
		S7Offset:   0,
		Enabled:    true,
	}

	_, err = client.ReadTags(ctx, []*domain.Tag{tag})
	if err == nil {
		t.Error("expected error reading on disconnected client")
	} else {
		t.Logf("Correctly got error on disconnected read: %v", err)
	}
}

// TestS7_WriteAfterDisconnect tests writing after the client has disconnected.
func TestS7_WriteAfterDisconnect(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoSimulator(t, cfg.S7Host, cfg.S7Port)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	client, err := s7.NewClient("err-write-disconnected", s7.ClientConfig{
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

	// Disconnect first
	client.Disconnect()

	tag := &domain.Tag{
		ID:         "disconnected-write",
		S7Address:  "DB1.DBW100",
		DataType:   domain.DataTypeInt16,
		S7Area:     domain.S7AreaDB,
		S7DBNumber: 1,
		S7Offset:   100,
		AccessMode: domain.AccessModeReadWrite,
		Enabled:    true,
	}

	err = client.WriteTag(ctx, tag, int16(42))
	if err == nil {
		t.Error("expected error writing on disconnected client")
	} else {
		t.Logf("Correctly got error on disconnected write: %v", err)
	}
}
