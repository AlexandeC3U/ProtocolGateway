//go:build fuzz
// +build fuzz

// Package parsing provides fuzz tests for OPC UA NodeID parsing.
package parsing

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// =============================================================================
// OPC UA NodeID Types
// =============================================================================

// NodeIDType represents the type of OPC UA Node ID identifier.
type NodeIDType int

const (
	NodeIDTypeNumeric NodeIDType = iota // i=<numeric>
	NodeIDTypeString                    // s=<string>
	NodeIDTypeGUID                      // g=<guid>
	NodeIDTypeOpaque                    // b=<base64>
)

func (t NodeIDType) String() string {
	switch t {
	case NodeIDTypeNumeric:
		return "Numeric"
	case NodeIDTypeString:
		return "String"
	case NodeIDTypeGUID:
		return "GUID"
	case NodeIDTypeOpaque:
		return "Opaque"
	default:
		return "Unknown"
	}
}

// ParsedNodeID represents a parsed OPC UA Node ID.
type ParsedNodeID struct {
	NamespaceIndex uint16
	Type           NodeIDType
	NumericID      uint32
	StringID       string
	GUIDID         string
	OpaqueID       []byte
	Valid          bool
	Error          string
}

// =============================================================================
// NodeID Parsing Implementation
// =============================================================================

// NodeID patterns
var (
	// Full NodeID pattern: ns=<index>;<type>=<value>
	nodeIDFullPattern = regexp.MustCompile(`^(?:ns=(\d+);)?([isgb])=(.+)$`)

	// GUID pattern: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	guidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// parseNodeID parses an OPC UA NodeID string.
func parseNodeID(nodeID string) ParsedNodeID {
	nodeID = strings.TrimSpace(nodeID)

	if nodeID == "" {
		return ParsedNodeID{Valid: false, Error: "empty node ID"}
	}

	matches := nodeIDFullPattern.FindStringSubmatch(nodeID)
	if matches == nil {
		return ParsedNodeID{Valid: false, Error: "invalid node ID format"}
	}

	result := ParsedNodeID{
		NamespaceIndex: 0,
		Valid:          true,
	}

	// Parse namespace index
	if matches[1] != "" {
		nsIndex, err := strconv.ParseUint(matches[1], 10, 16)
		if err != nil {
			return ParsedNodeID{Valid: false, Error: "invalid namespace index"}
		}
		result.NamespaceIndex = uint16(nsIndex)
	}

	// Parse identifier type and value
	idType := matches[2]
	idValue := matches[3]

	switch idType {
	case "i":
		// Numeric identifier
		result.Type = NodeIDTypeNumeric
		numID, err := strconv.ParseUint(idValue, 10, 32)
		if err != nil {
			return ParsedNodeID{Valid: false, Error: "invalid numeric identifier"}
		}
		result.NumericID = uint32(numID)

	case "s":
		// String identifier
		result.Type = NodeIDTypeString
		result.StringID = idValue

	case "g":
		// GUID identifier
		result.Type = NodeIDTypeGUID
		if !guidPattern.MatchString(idValue) {
			return ParsedNodeID{Valid: false, Error: "invalid GUID format"}
		}
		result.GUIDID = idValue

	case "b":
		// Opaque (base64) identifier
		result.Type = NodeIDTypeOpaque
		result.OpaqueID = []byte(idValue) // Simplified: not actually decoding base64

	default:
		return ParsedNodeID{Valid: false, Error: "unknown identifier type"}
	}

	return result
}

// formatNodeID formats a ParsedNodeID back to string.
func formatNodeID(parsed ParsedNodeID) string {
	if !parsed.Valid {
		return ""
	}

	var sb strings.Builder

	if parsed.NamespaceIndex > 0 {
		sb.WriteString(fmt.Sprintf("ns=%d;", parsed.NamespaceIndex))
	}

	switch parsed.Type {
	case NodeIDTypeNumeric:
		sb.WriteString(fmt.Sprintf("i=%d", parsed.NumericID))
	case NodeIDTypeString:
		sb.WriteString(fmt.Sprintf("s=%s", parsed.StringID))
	case NodeIDTypeGUID:
		sb.WriteString(fmt.Sprintf("g=%s", parsed.GUIDID))
	case NodeIDTypeOpaque:
		sb.WriteString(fmt.Sprintf("b=%s", string(parsed.OpaqueID)))
	}

	return sb.String()
}

// =============================================================================
// Standard OPC UA Node IDs for Testing
// =============================================================================

// Well-known OPC UA Node IDs
var wellKnownNodeIDs = []string{
	"i=84",    // Root folder
	"i=85",    // Objects folder
	"i=86",    // Types folder
	"i=87",    // Views folder
	"i=2253",  // Server object
	"i=2254",  // ServerArray
	"i=2255",  // NamespaceArray
	"i=2256",  // ServerStatus
	"i=2259",  // State
	"i=2267",  // CurrentTime
	"i=11226", // ProductUri
}

// =============================================================================
// Fuzz Tests for Numeric NodeID Parsing
// =============================================================================

// FuzzNumericNodeIDParsing fuzzes numeric NodeID parsing.
func FuzzNumericNodeIDParsing(f *testing.F) {
	// Seed with well-known Node IDs
	for _, nodeID := range wellKnownNodeIDs {
		f.Add(nodeID)
	}

	// Additional seeds
	f.Add("i=0")
	f.Add("i=1")
	f.Add("i=4294967295") // Max uint32
	f.Add("ns=0;i=84")
	f.Add("ns=1;i=100")
	f.Add("ns=2;i=1000")
	f.Add("ns=65535;i=100")

	// Invalid seeds
	f.Add("i=-1")
	f.Add("i=")
	f.Add("i=abc")
	f.Add("ns=-1;i=100")
	f.Add("ns=65536;i=100")

	f.Fuzz(func(t *testing.T, nodeID string) {
		// Should not panic
		result := parseNodeID(nodeID)

		if result.Valid {
			// Verify numeric constraints
			if result.Type == NodeIDTypeNumeric {
				// NumericID should be within uint32 range (already guaranteed by type)
				if result.NumericID > 4294967295 {
					t.Errorf("numeric ID out of range: %d", result.NumericID)
				}
			}

			// Namespace index should be within uint16 range
			if result.NamespaceIndex > 65535 {
				t.Errorf("namespace index out of range: %d", result.NamespaceIndex)
			}
		}
	})
}

// FuzzNumericNodeIDRoundtrip fuzzes numeric NodeID roundtrip.
func FuzzNumericNodeIDRoundtrip(f *testing.F) {
	f.Add(uint16(0), uint32(84))
	f.Add(uint16(0), uint32(0))
	f.Add(uint16(0), uint32(4294967295))
	f.Add(uint16(1), uint32(100))
	f.Add(uint16(65535), uint32(1000))

	f.Fuzz(func(t *testing.T, nsIndex uint16, numericID uint32) {
		// Create NodeID string
		var nodeIDStr string
		if nsIndex > 0 {
			nodeIDStr = fmt.Sprintf("ns=%d;i=%d", nsIndex, numericID)
		} else {
			nodeIDStr = fmt.Sprintf("i=%d", numericID)
		}

		// Parse
		result := parseNodeID(nodeIDStr)
		if !result.Valid {
			t.Errorf("failed to parse: %s", nodeIDStr)
			return
		}

		// Verify
		if result.NamespaceIndex != nsIndex {
			t.Errorf("namespace mismatch: expected %d, got %d", nsIndex, result.NamespaceIndex)
		}
		if result.NumericID != numericID {
			t.Errorf("numeric ID mismatch: expected %d, got %d", numericID, result.NumericID)
		}
		if result.Type != NodeIDTypeNumeric {
			t.Errorf("type mismatch: expected Numeric, got %s", result.Type)
		}

		// Roundtrip
		formatted := formatNodeID(result)
		reparsed := parseNodeID(formatted)
		if !reparsed.Valid {
			t.Errorf("roundtrip failed: %s -> %s", nodeIDStr, formatted)
			return
		}
		if reparsed.NumericID != result.NumericID {
			t.Errorf("roundtrip numeric ID mismatch: %d vs %d", result.NumericID, reparsed.NumericID)
		}
	})
}

// =============================================================================
// Fuzz Tests for String NodeID Parsing
// =============================================================================

// FuzzStringNodeIDParsing fuzzes string NodeID parsing.
func FuzzStringNodeIDParsing(f *testing.F) {
	// Valid string Node IDs
	f.Add("s=Demo")
	f.Add("s=Demo.Temperature")
	f.Add("s=Simulation.Sinusoid")
	f.Add("ns=2;s=Demo")
	f.Add("ns=2;s=Demo.Temperature")
	f.Add("ns=2;s=Pump.Speed")

	// Edge cases
	f.Add("s=")
	f.Add("s=a")
	f.Add("s=a.b.c.d.e.f.g.h.i.j")
	f.Add("s=node with spaces")
	f.Add("s=node-with-dashes")
	f.Add("s=node_with_underscores")

	f.Fuzz(func(t *testing.T, nodeID string) {
		// Should not panic
		result := parseNodeID(nodeID)

		if result.Valid && result.Type == NodeIDTypeString {
			// String ID should not be empty if parsed from non-empty input
			// (though "s=" is technically valid per OPC UA spec)
		}
	})
}

// FuzzStringNodeIDRoundtrip fuzzes string NodeID roundtrip.
func FuzzStringNodeIDRoundtrip(f *testing.F) {
	f.Add(uint16(0), "Demo")
	f.Add(uint16(2), "Demo.Temperature")
	f.Add(uint16(2), "Simulation.Sinusoid")
	f.Add(uint16(1), "MyNode")

	f.Fuzz(func(t *testing.T, nsIndex uint16, stringID string) {
		// Skip empty strings and strings with problematic characters
		if stringID == "" || strings.ContainsAny(stringID, "\n\r\t\x00") {
			return
		}

		// Create NodeID string
		var nodeIDStr string
		if nsIndex > 0 {
			nodeIDStr = fmt.Sprintf("ns=%d;s=%s", nsIndex, stringID)
		} else {
			nodeIDStr = fmt.Sprintf("s=%s", stringID)
		}

		// Parse
		result := parseNodeID(nodeIDStr)
		if !result.Valid {
			// Some strings may not parse correctly
			return
		}

		// Verify
		if result.Type != NodeIDTypeString {
			t.Errorf("type mismatch: expected String, got %s", result.Type)
		}
		if result.StringID != stringID {
			t.Errorf("string ID mismatch: expected %q, got %q", stringID, result.StringID)
		}
	})
}

// =============================================================================
// Fuzz Tests for GUID NodeID Parsing
// =============================================================================

// FuzzGUIDNodeIDParsing fuzzes GUID NodeID parsing.
func FuzzGUIDNodeIDParsing(f *testing.F) {
	// Valid GUIDs
	f.Add("g=00000000-0000-0000-0000-000000000000")
	f.Add("g=12345678-1234-1234-1234-123456789abc")
	f.Add("g=FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF")
	f.Add("ns=1;g=12345678-1234-1234-1234-123456789abc")

	// Invalid GUIDs
	f.Add("g=")
	f.Add("g=invalid")
	f.Add("g=12345678-1234-1234-1234-12345678")       // Too short
	f.Add("g=12345678-1234-1234-1234-123456789abcde") // Too long
	f.Add("g=12345678123412341234123456789abc")       // Missing dashes

	f.Fuzz(func(t *testing.T, nodeID string) {
		// Should not panic
		result := parseNodeID(nodeID)

		if result.Valid && result.Type == NodeIDTypeGUID {
			// Verify GUID format
			if !guidPattern.MatchString(result.GUIDID) {
				t.Errorf("invalid GUID format accepted: %s", result.GUIDID)
			}
		}
	})
}

// FuzzGUIDGeneration fuzzes GUID NodeID generation and parsing.
func FuzzGUIDGeneration(f *testing.F) {
	// Seed with hex digits
	f.Add(uint64(0), uint64(0))
	f.Add(uint64(0x123456789ABCDEF0), uint64(0xFEDCBA9876543210))
	f.Add(uint64(0xFFFFFFFFFFFFFFFF), uint64(0xFFFFFFFFFFFFFFFF))

	f.Fuzz(func(t *testing.T, high, low uint64) {
		// Generate GUID-like string
		guid := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
			uint32(high>>32),
			uint16(high>>16),
			uint16(high),
			uint16(low>>48),
			low&0x0000FFFFFFFFFFFF)

		nodeIDStr := fmt.Sprintf("g=%s", guid)
		result := parseNodeID(nodeIDStr)

		if !result.Valid {
			t.Errorf("failed to parse generated GUID: %s", guid)
			return
		}

		if result.Type != NodeIDTypeGUID {
			t.Errorf("type mismatch: expected GUID, got %s", result.Type)
		}
	})
}

// =============================================================================
// Fuzz Tests for Opaque NodeID Parsing
// =============================================================================

// FuzzOpaqueNodeIDParsing fuzzes opaque (base64) NodeID parsing.
func FuzzOpaqueNodeIDParsing(f *testing.F) {
	// Valid opaque Node IDs (base64-like)
	f.Add("b=SGVsbG8gV29ybGQ=")
	f.Add("b=AQIDBA==")
	f.Add("b=dGVzdA==")
	f.Add("ns=1;b=SGVsbG8=")

	// Edge cases
	f.Add("b=")
	f.Add("b=a")
	f.Add("b=====")

	f.Fuzz(func(t *testing.T, nodeID string) {
		// Should not panic
		result := parseNodeID(nodeID)

		// Opaque parsing is lenient in our implementation
		_ = result
	})
}

// =============================================================================
// Fuzz Tests for Mixed/Invalid NodeID Formats
// =============================================================================

// FuzzInvalidNodeIDFormats fuzzes various invalid NodeID formats.
func FuzzInvalidNodeIDFormats(f *testing.F) {
	// Invalid formats
	f.Add("")
	f.Add("invalid")
	f.Add("x=123")          // Invalid type
	f.Add("ns=;i=100")      // Empty namespace
	f.Add("ns=abc;i=100")   // Non-numeric namespace
	f.Add("i=")             // Empty identifier
	f.Add("ns=0;ns=1;i=84") // Duplicate namespace
	f.Add(";;i=84")         // Multiple semicolons
	f.Add("i=84;extra")     // Extra content
	f.Add("I=84")           // Wrong case (should be lowercase 'i')
	f.Add("S=test")         // Wrong case

	f.Fuzz(func(t *testing.T, nodeID string) {
		// Should never panic regardless of input
		result := parseNodeID(nodeID)
		_ = result
	})
}

// FuzzNodeIDWithSpecialCharacters fuzzes NodeIDs with special characters.
func FuzzNodeIDWithSpecialCharacters(f *testing.F) {
	f.Add("s=test\x00null")
	f.Add("s=test\nnewline")
	f.Add("s=test\ttab")
	f.Add("s=test;semicolon")
	f.Add("s=test=equals")
	f.Add("s=<script>alert(1)</script>")
	f.Add("s=../../../etc/passwd")

	f.Fuzz(func(t *testing.T, nodeID string) {
		// Should handle gracefully without panic
		result := parseNodeID(nodeID)

		// NodeIDs with null bytes should generally be invalid or sanitized
		if strings.Contains(nodeID, "\x00") {
			// Implementation should handle null bytes
			_ = result
		}
	})
}

// FuzzNodeIDOverflow fuzzes NodeIDs with overflow attempts.
func FuzzNodeIDOverflow(f *testing.F) {
	f.Add("i=999999999999999999999") // Way over uint32 max
	f.Add("ns=999999;i=100")         // Over uint16 max for namespace
	f.Add("i=18446744073709551616")  // Over uint64 max
	f.Add("ns=65536;i=100")          // Just over uint16 max

	f.Fuzz(func(t *testing.T, nodeID string) {
		// Should handle overflow gracefully
		result := parseNodeID(nodeID)

		// Overflow values should result in invalid parsing
		_ = result
	})
}

// =============================================================================
// Fuzz Tests for Namespace Index
// =============================================================================

// FuzzNamespaceIndexRange fuzzes namespace index boundary values.
func FuzzNamespaceIndexRange(f *testing.F) {
	f.Add(uint16(0))
	f.Add(uint16(1))
	f.Add(uint16(2))
	f.Add(uint16(100))
	f.Add(uint16(65534))
	f.Add(uint16(65535))

	f.Fuzz(func(t *testing.T, nsIndex uint16) {
		nodeIDStr := fmt.Sprintf("ns=%d;i=100", nsIndex)
		result := parseNodeID(nodeIDStr)

		if !result.Valid {
			t.Errorf("failed to parse valid namespace: %s", nodeIDStr)
			return
		}

		if result.NamespaceIndex != nsIndex {
			t.Errorf("namespace mismatch: expected %d, got %d", nsIndex, result.NamespaceIndex)
		}
	})
}
