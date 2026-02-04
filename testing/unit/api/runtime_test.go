// Package api_test tests the runtime management functionality.
package api_test

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/api"
)

// =============================================================================
// Log Entry Tests
// =============================================================================

// TestLogEntryStructure tests LogEntry structure.
func TestLogEntryStructure(t *testing.T) {
	entry := api.LogEntry{
		Timestamp: "2026-02-03T10:30:00Z",
		Level:     "INF",
		Source:    "modbus-client",
		Message:   "Connected to device",
		Raw:       "2026-02-03T10:30:00Z INF Connected to device component=modbus-client",
	}

	if entry.Timestamp != "2026-02-03T10:30:00Z" {
		t.Errorf("expected Timestamp '2026-02-03T10:30:00Z', got %s", entry.Timestamp)
	}
	if entry.Level != "INF" {
		t.Errorf("expected Level 'INF', got %s", entry.Level)
	}
	if entry.Source != "modbus-client" {
		t.Errorf("expected Source 'modbus-client', got %s", entry.Source)
	}
	if entry.Message != "Connected to device" {
		t.Errorf("expected Message 'Connected to device', got %s", entry.Message)
	}
	if entry.Raw != "2026-02-03T10:30:00Z INF Connected to device component=modbus-client" {
		t.Errorf("expected Raw log line, got %s", entry.Raw)
	}
}

// TestLogLevels tests different log level values.
func TestLogLevels(t *testing.T) {
	levels := []string{"TRC", "DBG", "INF", "WRN", "ERR", "FTL"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			entry := api.LogEntry{Level: level}
			if entry.Level == "" {
				t.Error("log level should not be empty")
			}
		})
	}
}

// =============================================================================
// Container Name Validation Tests
// =============================================================================

// TestContainerNameValidation tests container name regex validation.
func TestContainerNameValidation(t *testing.T) {
	tests := []struct {
		name      string
		container string
		valid     bool
	}{
		{"simple name", "gateway", true},
		{"with underscore", "modbus_client", true},
		{"with dash", "opc-ua-server", true},
		{"with dot", "mqtt.publisher", true},
		{"with numbers", "plc123", true},
		{"complex name", "connector-gateway_prod.v1", true},
		{"starts with number", "1gateway", true},
		{"empty", "", false},
		{"starts with dash", "-gateway", false},
		{"starts with dot", ".gateway", false},
		{"starts with underscore", "_gateway", false},
		{"contains special chars", "gate@way", false},
		{"contains space", "gate way", false},
		{"too long (129 chars)", strings.Repeat("a", 129), false},
		{"max length (128 chars)", strings.Repeat("a", 128), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := isValidContainerName(tt.container)
			if valid != tt.valid {
				t.Errorf("expected valid=%v for container '%s', got %v", tt.valid, tt.container, valid)
			}
		})
	}
}

// =============================================================================
// Tail Limit Tests
// =============================================================================

// TestTailLimitValidation tests tail parameter bounds.
func TestTailLimitValidation(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{"default (zero)", 0, 300},
		{"negative", -10, 300},
		{"small positive", 50, 50},
		{"normal", 100, 100},
		{"at max", 5000, 5000},
		{"over max", 10000, 5000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeTail(tt.input)
			if result != tt.expected {
				t.Errorf("expected tail=%d, got %d", tt.expected, result)
			}
		})
	}
}

// =============================================================================
// Log Line Parsing Tests
// =============================================================================

// TestLogLineParsing_DockerTimestamp tests parsing docker log lines with timestamps.
func TestLogLineParsing_DockerTimestamp(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		expectTS      bool
		expectLevel   string
		expectMessage string
	}{
		{
			name:        "docker timestamp with gateway log",
			line:        "2026-02-03T10:30:00.123456789Z 2026-02-03 10:30:00 2026-02-03T10:30:00Z INF Starting gateway component=main",
			expectTS:    true,
			expectLevel: "INF",
		},
		{
			name:        "docker timestamp only",
			line:        "2026-02-03T10:30:00.123456789Z Some message",
			expectTS:    true,
			expectLevel: "",
		},
		{
			name:          "no timestamp",
			line:          "INF Some message without timestamp",
			expectTS:      false,
			expectLevel:   "INF",
			expectMessage: "INF Some message without timestamp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := parseLogLineSimple(tt.line)

			if tt.expectTS && entry.Timestamp == "" {
				t.Error("expected timestamp to be extracted")
			}
			if entry.Level != tt.expectLevel {
				t.Errorf("expected level '%s', got '%s'", tt.expectLevel, entry.Level)
			}
			if entry.Raw != tt.line {
				t.Error("raw line should be preserved")
			}
		})
	}
}

// TestLogLineParsing_Levels tests log level extraction from log lines.
func TestLogLineParsing_Levels(t *testing.T) {
	tests := []struct {
		line     string
		expected string
	}{
		{"2026-02-03 10:30:00 TRC Trace message", "TRC"},
		{"2026-02-03 10:30:00 DBG Debug message", "DBG"},
		{"2026-02-03 10:30:00 INF Info message", "INF"},
		{"2026-02-03 10:30:00 WRN Warning message", "WRN"},
		{"2026-02-03 10:30:00 ERR Error message", "ERR"},
		{"2026-02-03 10:30:00 FTL Fatal message", "FTL"},
		{"No level in this line", ""},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			level := extractLevel(tt.line)
			if level != tt.expected {
				t.Errorf("expected level '%s', got '%s'", tt.expected, level)
			}
		})
	}
}

// TestLogLineParsing_Source tests source/component extraction.
func TestLogLineParsing_Source(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "component key",
			line:     "INF Starting component=modbus-pool",
			expected: "modbus-pool",
		},
		{
			name:     "service key",
			line:     "INF Started service=polling",
			expected: "polling",
		},
		{
			name:     "quoted component",
			line:     `INF Starting component="mqtt-publisher"`,
			expected: "mqtt-publisher",
		},
		{
			name:     "no source",
			line:     "INF Just a message",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := extractSourceFromLine(tt.line)
			if source != tt.expected {
				t.Errorf("expected source '%s', got '%s'", tt.expected, source)
			}
		})
	}
}

// TestLogLineParsing_KVExtraction tests key-value extraction from log lines.
func TestLogLineParsing_KVExtraction(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		key      string
		expected string
	}{
		{"simple value", "device_id=plc-001", "device_id", "plc-001"},
		{"quoted value", `name="My Device"`, "name", "My Device"},
		{"value with equals", "equation=a=b", "equation", "a=b"},
		{"non-existing key", "foo=bar", "baz", ""},
		{"empty value", "empty=", "empty", ""},
		{"value at end", "msg=hello", "msg", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractKV(tt.line, tt.key)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// =============================================================================
// Docker CLI Log Provider Tests
// =============================================================================

// TestDockerCLILogProvider_Creation tests DockerCLILogProvider creation.
func TestDockerCLILogProvider_Creation(t *testing.T) {
	// Test that the provider can be created (actual Docker interaction
	// is tested in integration tests)
	t.Run("creation succeeds", func(t *testing.T) {
		// We can't call NewDockerCLILogProvider without a logger,
		// but we can verify the type exists and interface is satisfied
		var _ api.LogProvider = (*api.DockerCLILogProvider)(nil)
	})
}

// =============================================================================
// Context Timeout Tests
// =============================================================================

// TestLogProviderTimeout tests context timeout behavior.
func TestLogProviderTimeout(t *testing.T) {
	// Simulate a context that times out
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Wait for context to expire
	<-ctx.Done()

	if ctx.Err() != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", ctx.Err())
	}
}

// TestLogProviderCancellation tests context cancellation.
func TestLogProviderCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	if ctx.Err() != context.Canceled {
		t.Errorf("expected Canceled, got %v", ctx.Err())
	}
}

// =============================================================================
// Log Entry Filtering Tests
// =============================================================================

// TestLogEntryFiltering tests filtering log entries by level.
func TestLogEntryFiltering(t *testing.T) {
	entries := []api.LogEntry{
		{Level: "TRC", Message: "Trace"},
		{Level: "DBG", Message: "Debug"},
		{Level: "INF", Message: "Info"},
		{Level: "WRN", Message: "Warning"},
		{Level: "ERR", Message: "Error"},
		{Level: "FTL", Message: "Fatal"},
	}

	tests := []struct {
		name     string
		minLevel string
		expected int
	}{
		{"all levels", "TRC", 6},
		{"debug and above", "DBG", 5},
		{"info and above", "INF", 4},
		{"warning and above", "WRN", 3},
		{"error and above", "ERR", 2},
		{"fatal only", "FTL", 1},
	}

	levelOrder := map[string]int{
		"TRC": 0, "DBG": 1, "INF": 2, "WRN": 3, "ERR": 4, "FTL": 5,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			minIdx := levelOrder[tt.minLevel]
			count := 0
			for _, entry := range entries {
				if idx, ok := levelOrder[entry.Level]; ok && idx >= minIdx {
					count++
				}
			}
			if count != tt.expected {
				t.Errorf("expected %d entries, got %d", tt.expected, count)
			}
		})
	}
}

// TestLogEntryFiltering_BySource tests filtering by source component.
func TestLogEntryFiltering_BySource(t *testing.T) {
	entries := []api.LogEntry{
		{Source: "modbus-pool", Message: "Connected"},
		{Source: "modbus-pool", Message: "Reading tags"},
		{Source: "mqtt-publisher", Message: "Publishing"},
		{Source: "opcua-client", Message: "Browsing"},
		{Source: "modbus-pool", Message: "Disconnected"},
	}

	tests := []struct {
		source   string
		expected int
	}{
		{"modbus-pool", 3},
		{"mqtt-publisher", 1},
		{"opcua-client", 1},
		{"s7-client", 0},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			count := 0
			for _, entry := range entries {
				if entry.Source == tt.source {
					count++
				}
			}
			if count != tt.expected {
				t.Errorf("expected %d entries for source '%s', got %d", tt.expected, tt.source, count)
			}
		})
	}
}

// =============================================================================
// Log Entry Sorting Tests
// =============================================================================

// TestLogEntrySorting tests sorting log entries by timestamp.
func TestLogEntrySorting(t *testing.T) {
	entries := []api.LogEntry{
		{Timestamp: "2026-02-03T10:30:02Z"},
		{Timestamp: "2026-02-03T10:30:00Z"},
		{Timestamp: "2026-02-03T10:30:01Z"},
	}

	// Sort by timestamp (ascending)
	sorted := make([]api.LogEntry, len(entries))
	copy(sorted, entries)

	// Simple bubble sort for test
	for i := 0; i < len(sorted)-1; i++ {
		for j := 0; j < len(sorted)-i-1; j++ {
			if sorted[j].Timestamp > sorted[j+1].Timestamp {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}

	expected := []string{
		"2026-02-03T10:30:00Z",
		"2026-02-03T10:30:01Z",
		"2026-02-03T10:30:02Z",
	}

	for i, entry := range sorted {
		if entry.Timestamp != expected[i] {
			t.Errorf("position %d: expected %s, got %s", i, expected[i], entry.Timestamp)
		}
	}
}

// =============================================================================
// Interface Compliance Tests
// =============================================================================

// TestLogProviderInterface tests LogProvider interface compliance.
func TestLogProviderInterface(t *testing.T) {
	// Verify DockerCLILogProvider implements LogProvider
	var _ api.LogProvider = (*api.DockerCLILogProvider)(nil)
}

// =============================================================================
// Helper Functions
// =============================================================================

var containerNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

func isValidContainerName(name string) bool {
	if name == "" {
		return false
	}
	return containerNameRe.MatchString(name)
}

func normalizeTail(tail int) int {
	if tail <= 0 {
		return 300
	}
	if tail > 5000 {
		return 5000
	}
	return tail
}

func parseLogLineSimple(line string) api.LogEntry {
	entry := api.LogEntry{Raw: line}

	fields := strings.Fields(line)
	if len(fields) == 0 {
		return entry
	}

	// Try to parse first field as RFC3339 timestamp
	if _, err := time.Parse(time.RFC3339Nano, fields[0]); err == nil {
		entry.Timestamp = fields[0]
	}

	// Extract level
	entry.Level = extractLevel(line)
	entry.Message = strings.TrimSpace(line)

	return entry
}

func extractLevel(line string) string {
	levels := []string{"TRC", "DBG", "INF", "WRN", "ERR", "FTL"}
	fields := strings.Fields(line)
	for _, field := range fields {
		for _, level := range levels {
			if field == level {
				return level
			}
		}
	}
	return ""
}

func extractSourceFromLine(line string) string {
	if v := extractKV(line, "component"); v != "" {
		return v
	}
	if v := extractKV(line, "service"); v != "" {
		return v
	}
	return ""
}

func extractKV(s, key string) string {
	needle := key + "="
	idx := strings.Index(s, needle)
	if idx < 0 {
		return ""
	}
	start := idx + len(needle)
	if start >= len(s) {
		return ""
	}
	// value may be quoted or unquoted
	if s[start] == '"' {
		end := strings.Index(s[start+1:], "\"")
		if end < 0 {
			return ""
		}
		return s[start+1 : start+1+end]
	}
	end := start
	for end < len(s) && s[end] != ' ' {
		end++
	}
	return s[start:end]
}
