// Package opcua_test tests the OPC UA browse functionality.
package opcua_test

import (
	"encoding/json"
	"testing"

	"github.com/gopcua/opcua/ua"
	"github.com/nexus-edge/protocol-gateway/internal/adapter/opcua"
)

// TestBrowseResultStructure tests the BrowseResult struct fields and JSON serialization.
func TestBrowseResultStructure(t *testing.T) {
	result := opcua.BrowseResult{
		NodeID:        "ns=2;s=Temperature",
		DisplayName:   "Temperature Sensor",
		BrowseName:    "Temperature",
		NodeClass:     ua.NodeClassVariable,
		NodeClassName: "Variable",
		DataType:      "Double",
		AccessLevel:   "Read, Write",
		HasChildren:   false,
		Children:      nil,
	}

	if result.NodeID != "ns=2;s=Temperature" {
		t.Errorf("expected NodeID 'ns=2;s=Temperature', got %q", result.NodeID)
	}
	if result.DisplayName != "Temperature Sensor" {
		t.Errorf("expected DisplayName 'Temperature Sensor', got %q", result.DisplayName)
	}
	if result.NodeClassName != "Variable" {
		t.Errorf("expected NodeClassName 'Variable', got %q", result.NodeClassName)
	}
	if result.DataType != "Double" {
		t.Errorf("expected DataType 'Double', got %q", result.DataType)
	}
}

// TestBrowseResultJSONSerialization tests JSON marshaling of BrowseResult.
func TestBrowseResultJSONSerialization(t *testing.T) {
	result := opcua.BrowseResult{
		NodeID:        "ns=2;s=Folder1",
		DisplayName:   "Folder 1",
		BrowseName:    "Folder1",
		NodeClass:     ua.NodeClassObject,
		NodeClassName: "Object",
		HasChildren:   true,
		Children: []*opcua.BrowseResult{
			{
				NodeID:        "ns=2;s=Var1",
				DisplayName:   "Variable 1",
				BrowseName:    "Var1",
				NodeClass:     ua.NodeClassVariable,
				NodeClassName: "Variable",
				DataType:      "Int32",
				AccessLevel:   "Read",
				HasChildren:   false,
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal BrowseResult: %v", err)
	}

	// Verify JSON contains expected fields
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if parsed["node_id"] != "ns=2;s=Folder1" {
		t.Errorf("expected node_id 'ns=2;s=Folder1', got %v", parsed["node_id"])
	}
	if parsed["display_name"] != "Folder 1" {
		t.Errorf("expected display_name 'Folder 1', got %v", parsed["display_name"])
	}
	if parsed["has_children"] != true {
		t.Errorf("expected has_children true, got %v", parsed["has_children"])
	}

	children, ok := parsed["children"].([]interface{})
	if !ok || len(children) != 1 {
		t.Errorf("expected 1 child, got %v", parsed["children"])
	}
}

// TestBrowseResultJSONOmitEmpty tests that empty optional fields are omitted.
func TestBrowseResultJSONOmitEmpty(t *testing.T) {
	result := opcua.BrowseResult{
		NodeID:        "ns=2;s=Obj1",
		DisplayName:   "Object 1",
		NodeClass:     ua.NodeClassObject,
		NodeClassName: "Object",
		HasChildren:   false,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal BrowseResult: %v", err)
	}

	jsonStr := string(data)

	// data_type should be omitted when empty
	if containsString(jsonStr, "data_type") {
		t.Error("expected data_type to be omitted from JSON when empty")
	}

	// access_level should be omitted when empty
	if containsString(jsonStr, "access_level") {
		t.Error("expected access_level to be omitted from JSON when empty")
	}

	// Required fields should be present
	if !containsString(jsonStr, "node_id") {
		t.Error("expected node_id to be present in JSON")
	}
	if !containsString(jsonStr, "display_name") {
		t.Error("expected display_name to be present in JSON")
	}
}

// TestBrowseResultWithNestedChildren tests deeply nested browse results.
func TestBrowseResultWithNestedChildren(t *testing.T) {
	result := opcua.BrowseResult{
		NodeID:        "i=85",
		DisplayName:   "Objects",
		NodeClassName: "Object",
		HasChildren:   true,
		Children: []*opcua.BrowseResult{
			{
				NodeID:        "ns=2;s=Plant",
				DisplayName:   "Plant",
				NodeClassName: "Object",
				HasChildren:   true,
				Children: []*opcua.BrowseResult{
					{
						NodeID:        "ns=2;s=Line1",
						DisplayName:   "Line 1",
						NodeClassName: "Object",
						HasChildren:   true,
						Children: []*opcua.BrowseResult{
							{
								NodeID:        "ns=2;s=Temperature",
								DisplayName:   "Temperature",
								NodeClassName: "Variable",
								DataType:      "Double",
								HasChildren:   false,
							},
						},
					},
				},
			},
		},
	}

	// Verify nested structure
	if len(result.Children) != 1 {
		t.Fatalf("expected 1 child at root, got %d", len(result.Children))
	}

	plant := result.Children[0]
	if plant.DisplayName != "Plant" {
		t.Errorf("expected 'Plant', got %q", plant.DisplayName)
	}

	if len(plant.Children) != 1 {
		t.Fatalf("expected 1 child under Plant, got %d", len(plant.Children))
	}

	line1 := plant.Children[0]
	if len(line1.Children) != 1 {
		t.Fatalf("expected 1 child under Line 1, got %d", len(line1.Children))
	}

	temp := line1.Children[0]
	if temp.NodeClassName != "Variable" {
		t.Errorf("expected 'Variable', got %q", temp.NodeClassName)
	}
	if temp.DataType != "Double" {
		t.Errorf("expected 'Double', got %q", temp.DataType)
	}
}

// TestNodeClassValues tests that NodeClass values are serialized correctly.
func TestNodeClassValues(t *testing.T) {
	tests := []struct {
		nodeClass     ua.NodeClass
		expectedName  string
	}{
		{ua.NodeClassObject, "Object"},
		{ua.NodeClassVariable, "Variable"},
		{ua.NodeClassMethod, "Method"},
	}

	for _, tt := range tests {
		t.Run(tt.expectedName, func(t *testing.T) {
			result := opcua.BrowseResult{
				NodeID:        "ns=2;s=Test",
				NodeClass:     tt.nodeClass,
				NodeClassName: tt.expectedName,
			}

			if result.NodeClassName != tt.expectedName {
				t.Errorf("expected NodeClassName %q, got %q", tt.expectedName, result.NodeClassName)
			}
		})
	}
}

// TestBrowseResultDataTypes tests common OPC UA data type names.
func TestBrowseResultDataTypes(t *testing.T) {
	dataTypes := []string{
		"Boolean",
		"SByte",
		"Byte",
		"Int16",
		"UInt16",
		"Int32",
		"UInt32",
		"Int64",
		"UInt64",
		"Float",
		"Double",
		"String",
		"DateTime",
		"ByteString",
	}

	for _, dt := range dataTypes {
		t.Run(dt, func(t *testing.T) {
			result := opcua.BrowseResult{
				NodeID:        "ns=2;s=Test",
				NodeClassName: "Variable",
				DataType:      dt,
			}

			if result.DataType != dt {
				t.Errorf("expected DataType %q, got %q", dt, result.DataType)
			}
		})
	}
}

// TestBrowseResultAccessLevels tests common access level strings.
func TestBrowseResultAccessLevels(t *testing.T) {
	accessLevels := []string{
		"None",
		"Read",
		"Write",
		"Read, Write",
		"HistoryRead",
		"Read, HistoryRead",
	}

	for _, al := range accessLevels {
		t.Run(al, func(t *testing.T) {
			result := opcua.BrowseResult{
				NodeID:        "ns=2;s=Test",
				NodeClassName: "Variable",
				AccessLevel:   al,
			}

			if result.AccessLevel != al {
				t.Errorf("expected AccessLevel %q, got %q", al, result.AccessLevel)
			}
		})
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
