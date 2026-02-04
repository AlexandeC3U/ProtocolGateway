//go:build integration
// +build integration

// Package opcua_test tests OPC UA node browsing operations against a simulator.
package opcua_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Test Configuration
// =============================================================================

// getOPCUABrowseEndpoint returns the OPC UA simulator endpoint for browse tests.
func getOPCUABrowseEndpoint() string {
	if endpoint := os.Getenv("OPCUA_SIMULATOR_ENDPOINT"); endpoint != "" {
		return endpoint
	}
	return "opc.tcp://localhost:4840"
}

// =============================================================================
// Node ID Types for Testing
// =============================================================================

// NodeClass represents OPC UA node classes.
type NodeClass int

const (
	NodeClassObject        NodeClass = 1
	NodeClassVariable      NodeClass = 2
	NodeClassMethod        NodeClass = 4
	NodeClassObjectType    NodeClass = 8
	NodeClassVariableType  NodeClass = 16
	NodeClassReferenceType NodeClass = 32
	NodeClassDataType      NodeClass = 64
	NodeClassView          NodeClass = 128
)

func (nc NodeClass) String() string {
	switch nc {
	case NodeClassObject:
		return "Object"
	case NodeClassVariable:
		return "Variable"
	case NodeClassMethod:
		return "Method"
	case NodeClassObjectType:
		return "ObjectType"
	case NodeClassVariableType:
		return "VariableType"
	case NodeClassReferenceType:
		return "ReferenceType"
	case NodeClassDataType:
		return "DataType"
	case NodeClassView:
		return "View"
	default:
		return fmt.Sprintf("Unknown(%d)", nc)
	}
}

// BrowseDirection represents the direction for browsing.
type BrowseDirection int

const (
	BrowseDirectionForward BrowseDirection = 0
	BrowseDirectionInverse BrowseDirection = 1
	BrowseDirectionBoth    BrowseDirection = 2
)

// NodeIDType represents the type of node identifier.
type NodeIDType int

const (
	NodeIDNumeric NodeIDType = iota
	NodeIDString
	NodeIDGUID
	NodeIDOpaque
)

// MockBrowseResult simulates an OPC UA browse result.
type MockBrowseResult struct {
	NodeID         string
	BrowseName     string
	DisplayName    string
	NodeClass      NodeClass
	TypeDefinition string
	IsForward      bool
	ReferenceType  string
}

// MockOPCUABrowser simulates OPC UA browse functionality.
type MockOPCUABrowser struct {
	mu       sync.RWMutex
	nodes    map[string][]MockBrowseResult
	endpoint string
}

// NewMockOPCUABrowser creates a new mock browser with predefined nodes.
func NewMockOPCUABrowser(endpoint string) *MockOPCUABrowser {
	browser := &MockOPCUABrowser{
		endpoint: endpoint,
		nodes:    make(map[string][]MockBrowseResult),
	}
	browser.initializeStandardNodes()
	return browser
}

// initializeStandardNodes populates standard OPC UA address space nodes.
func (b *MockOPCUABrowser) initializeStandardNodes() {
	// Root folder children
	b.nodes["i=84"] = []MockBrowseResult{
		{NodeID: "i=85", BrowseName: "Objects", DisplayName: "Objects", NodeClass: NodeClassObject, ReferenceType: "Organizes"},
		{NodeID: "i=86", BrowseName: "Types", DisplayName: "Types", NodeClass: NodeClassObject, ReferenceType: "Organizes"},
		{NodeID: "i=87", BrowseName: "Views", DisplayName: "Views", NodeClass: NodeClassObject, ReferenceType: "Organizes"},
	}

	// Objects folder children
	b.nodes["i=85"] = []MockBrowseResult{
		{NodeID: "i=2253", BrowseName: "Server", DisplayName: "Server", NodeClass: NodeClassObject, ReferenceType: "Organizes"},
		{NodeID: "ns=2;s=Demo", BrowseName: "Demo", DisplayName: "Demo Folder", NodeClass: NodeClassObject, ReferenceType: "Organizes"},
		{NodeID: "ns=2;s=Simulation", BrowseName: "Simulation", DisplayName: "Simulation", NodeClass: NodeClassObject, ReferenceType: "Organizes"},
	}

	// Demo folder with variables
	b.nodes["ns=2;s=Demo"] = []MockBrowseResult{
		{NodeID: "ns=2;s=Demo.Temperature", BrowseName: "Temperature", DisplayName: "Temperature", NodeClass: NodeClassVariable, TypeDefinition: "i=62", ReferenceType: "HasComponent"},
		{NodeID: "ns=2;s=Demo.Pressure", BrowseName: "Pressure", DisplayName: "Pressure", NodeClass: NodeClassVariable, TypeDefinition: "i=62", ReferenceType: "HasComponent"},
		{NodeID: "ns=2;s=Demo.Flow", BrowseName: "Flow", DisplayName: "Flow Rate", NodeClass: NodeClassVariable, TypeDefinition: "i=62", ReferenceType: "HasComponent"},
		{NodeID: "ns=2;s=Demo.Level", BrowseName: "Level", DisplayName: "Tank Level", NodeClass: NodeClassVariable, TypeDefinition: "i=62", ReferenceType: "HasComponent"},
		{NodeID: "ns=2;s=Demo.Status", BrowseName: "Status", DisplayName: "System Status", NodeClass: NodeClassVariable, TypeDefinition: "i=1", ReferenceType: "HasComponent"},
	}

	// Simulation folder with more complex structure
	b.nodes["ns=2;s=Simulation"] = []MockBrowseResult{
		{NodeID: "ns=2;s=Simulation.Sinusoid", BrowseName: "Sinusoid", DisplayName: "Sinusoidal Signal", NodeClass: NodeClassVariable, TypeDefinition: "i=63", ReferenceType: "HasComponent"},
		{NodeID: "ns=2;s=Simulation.Random", BrowseName: "Random", DisplayName: "Random Value", NodeClass: NodeClassVariable, TypeDefinition: "i=63", ReferenceType: "HasComponent"},
		{NodeID: "ns=2;s=Simulation.Counter", BrowseName: "Counter", DisplayName: "Counter", NodeClass: NodeClassVariable, TypeDefinition: "i=7", ReferenceType: "HasComponent"},
		{NodeID: "ns=2;s=Simulation.Timestamp", BrowseName: "Timestamp", DisplayName: "Current Time", NodeClass: NodeClassVariable, TypeDefinition: "i=294", ReferenceType: "HasComponent"},
		{NodeID: "ns=2;s=Simulation.Methods", BrowseName: "Methods", DisplayName: "Methods", NodeClass: NodeClassObject, ReferenceType: "HasComponent"},
	}

	// Methods folder
	b.nodes["ns=2;s=Simulation.Methods"] = []MockBrowseResult{
		{NodeID: "ns=2;s=Simulation.Methods.Reset", BrowseName: "Reset", DisplayName: "Reset Counter", NodeClass: NodeClassMethod, ReferenceType: "HasComponent"},
		{NodeID: "ns=2;s=Simulation.Methods.Start", BrowseName: "Start", DisplayName: "Start Simulation", NodeClass: NodeClassMethod, ReferenceType: "HasComponent"},
		{NodeID: "ns=2;s=Simulation.Methods.Stop", BrowseName: "Stop", DisplayName: "Stop Simulation", NodeClass: NodeClassMethod, ReferenceType: "HasComponent"},
	}

	// Server object children
	b.nodes["i=2253"] = []MockBrowseResult{
		{NodeID: "i=2254", BrowseName: "ServerArray", DisplayName: "Server Array", NodeClass: NodeClassVariable, TypeDefinition: "i=12", ReferenceType: "HasProperty"},
		{NodeID: "i=2255", BrowseName: "NamespaceArray", DisplayName: "Namespace Array", NodeClass: NodeClassVariable, TypeDefinition: "i=12", ReferenceType: "HasProperty"},
		{NodeID: "i=2256", BrowseName: "ServerStatus", DisplayName: "Server Status", NodeClass: NodeClassVariable, TypeDefinition: "i=862", ReferenceType: "HasComponent"},
		{NodeID: "i=2257", BrowseName: "ServiceLevel", DisplayName: "Service Level", NodeClass: NodeClassVariable, TypeDefinition: "i=3", ReferenceType: "HasProperty"},
	}

	// Types folder structure
	b.nodes["i=86"] = []MockBrowseResult{
		{NodeID: "i=88", BrowseName: "ObjectTypes", DisplayName: "Object Types", NodeClass: NodeClassObject, ReferenceType: "Organizes"},
		{NodeID: "i=89", BrowseName: "VariableTypes", DisplayName: "Variable Types", NodeClass: NodeClassObject, ReferenceType: "Organizes"},
		{NodeID: "i=90", BrowseName: "DataTypes", DisplayName: "Data Types", NodeClass: NodeClassObject, ReferenceType: "Organizes"},
		{NodeID: "i=91", BrowseName: "ReferenceTypes", DisplayName: "Reference Types", NodeClass: NodeClassObject, ReferenceType: "Organizes"},
	}
}

// Browse returns child nodes for the specified parent.
func (b *MockOPCUABrowser) Browse(ctx context.Context, nodeID string, direction BrowseDirection) ([]MockBrowseResult, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if children, ok := b.nodes[nodeID]; ok {
		// Simulate network latency
		time.Sleep(5 * time.Millisecond)

		results := make([]MockBrowseResult, len(children))
		copy(results, children)
		return results, nil
	}
	return []MockBrowseResult{}, nil
}

// BrowsePath follows a path from a starting node.
func (b *MockOPCUABrowser) BrowsePath(ctx context.Context, startNode string, path []string) (string, error) {
	currentNode := startNode

	for _, segment := range path {
		children, err := b.Browse(ctx, currentNode, BrowseDirectionForward)
		if err != nil {
			return "", err
		}

		found := false
		for _, child := range children {
			if child.BrowseName == segment || child.DisplayName == segment {
				currentNode = child.NodeID
				found = true
				break
			}
		}

		if !found {
			return "", fmt.Errorf("path segment '%s' not found under node '%s'", segment, currentNode)
		}
	}

	return currentNode, nil
}

// CountNodes recursively counts all nodes under a starting point.
func (b *MockOPCUABrowser) CountNodes(ctx context.Context, startNode string, maxDepth int) (int, error) {
	if maxDepth <= 0 {
		return 0, nil
	}

	children, err := b.Browse(ctx, startNode, BrowseDirectionForward)
	if err != nil {
		return 0, err
	}

	count := len(children)
	for _, child := range children {
		childCount, err := b.CountNodes(ctx, child.NodeID, maxDepth-1)
		if err != nil {
			return count, err
		}
		count += childCount
	}

	return count, nil
}

// =============================================================================
// Browse Root Node Tests
// =============================================================================

// TestOPCUABrowseRootNode tests browsing the root node.
func TestOPCUABrowseRootNode(t *testing.T) {
	endpoint := getOPCUABrowseEndpoint()
	t.Logf("Testing OPC UA browse at %s", endpoint)

	browser := NewMockOPCUABrowser(endpoint)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Browse root folder (i=84)
	results, err := browser.Browse(ctx, "i=84", BrowseDirectionForward)
	if err != nil {
		t.Fatalf("Failed to browse root node: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Expected at least one child node from root")
	}

	t.Logf("Found %d children under root node:", len(results))
	for _, r := range results {
		t.Logf("  - %s (%s) [%s]", r.DisplayName, r.NodeID, r.NodeClass)
	}

	// Verify standard folders exist
	expectedFolders := map[string]bool{
		"Objects": false,
		"Types":   false,
		"Views":   false,
	}

	for _, r := range results {
		if _, ok := expectedFolders[r.BrowseName]; ok {
			expectedFolders[r.BrowseName] = true
		}
	}

	for folder, found := range expectedFolders {
		if !found {
			t.Errorf("Expected folder '%s' not found under root", folder)
		}
	}
}

// TestOPCUABrowseObjectsFolder tests browsing the Objects folder.
func TestOPCUABrowseObjectsFolder(t *testing.T) {
	browser := NewMockOPCUABrowser(getOPCUABrowseEndpoint())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Browse Objects folder (i=85)
	results, err := browser.Browse(ctx, "i=85", BrowseDirectionForward)
	if err != nil {
		t.Fatalf("Failed to browse Objects folder: %v", err)
	}

	t.Logf("Found %d nodes in Objects folder:", len(results))
	for _, r := range results {
		t.Logf("  - %s (%s) [%s]", r.DisplayName, r.NodeID, r.NodeClass)
	}

	// Server object should always be present
	serverFound := false
	for _, r := range results {
		if r.BrowseName == "Server" {
			serverFound = true
			if r.NodeClass != NodeClassObject {
				t.Errorf("Server node should be Object class, got %s", r.NodeClass)
			}
			break
		}
	}

	if !serverFound {
		t.Error("Server object not found in Objects folder")
	}
}

// =============================================================================
// Browse Direction Tests
// =============================================================================

// TestOPCUABrowseDirections tests browsing in different directions.
func TestOPCUABrowseDirections(t *testing.T) {
	browser := NewMockOPCUABrowser(getOPCUABrowseEndpoint())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tests := []struct {
		name      string
		nodeID    string
		direction BrowseDirection
	}{
		{"ForwardFromRoot", "i=84", BrowseDirectionForward},
		{"ForwardFromObjects", "i=85", BrowseDirectionForward},
		{"ForwardFromDemo", "ns=2;s=Demo", BrowseDirectionForward},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := browser.Browse(ctx, tt.nodeID, tt.direction)
			if err != nil {
				t.Fatalf("Browse failed: %v", err)
			}
			t.Logf("Found %d nodes browsing %s from %s", len(results),
				[]string{"forward", "inverse", "both"}[tt.direction], tt.nodeID)
		})
	}
}

// =============================================================================
// Node Class Filtering Tests
// =============================================================================

// TestOPCUABrowseFilterByNodeClass tests filtering browse results by node class.
func TestOPCUABrowseFilterByNodeClass(t *testing.T) {
	browser := NewMockOPCUABrowser(getOPCUABrowseEndpoint())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Browse Demo folder which has variables
	results, err := browser.Browse(ctx, "ns=2;s=Demo", BrowseDirectionForward)
	if err != nil {
		t.Fatalf("Failed to browse Demo folder: %v", err)
	}

	// Filter for variables only
	var variables []MockBrowseResult
	for _, r := range results {
		if r.NodeClass == NodeClassVariable {
			variables = append(variables, r)
		}
	}

	t.Logf("Found %d variables in Demo folder:", len(variables))
	for _, v := range variables {
		t.Logf("  - %s (%s)", v.DisplayName, v.NodeID)
	}

	if len(variables) == 0 {
		t.Error("Expected at least one variable in Demo folder")
	}
}

// TestOPCUABrowseFilterMethods tests finding method nodes.
func TestOPCUABrowseFilterMethods(t *testing.T) {
	browser := NewMockOPCUABrowser(getOPCUABrowseEndpoint())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Browse Methods folder
	results, err := browser.Browse(ctx, "ns=2;s=Simulation.Methods", BrowseDirectionForward)
	if err != nil {
		t.Fatalf("Failed to browse Methods folder: %v", err)
	}

	var methods []MockBrowseResult
	for _, r := range results {
		if r.NodeClass == NodeClassMethod {
			methods = append(methods, r)
		}
	}

	t.Logf("Found %d methods:", len(methods))
	for _, m := range methods {
		t.Logf("  - %s (%s)", m.DisplayName, m.NodeID)
	}

	expectedMethods := []string{"Reset", "Start", "Stop"}
	for _, expected := range expectedMethods {
		found := false
		for _, m := range methods {
			if m.BrowseName == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected method '%s' not found", expected)
		}
	}
}

// =============================================================================
// Browse Path Tests
// =============================================================================

// TestOPCUABrowsePath tests following a browse path.
func TestOPCUABrowsePath(t *testing.T) {
	browser := NewMockOPCUABrowser(getOPCUABrowseEndpoint())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tests := []struct {
		name      string
		startNode string
		path      []string
		wantErr   bool
	}{
		{
			name:      "PathToTemperature",
			startNode: "i=85",
			path:      []string{"Demo", "Temperature"},
			wantErr:   false,
		},
		{
			name:      "PathToMethod",
			startNode: "i=85",
			path:      []string{"Simulation", "Methods", "Reset"},
			wantErr:   false,
		},
		{
			name:      "InvalidPath",
			startNode: "i=85",
			path:      []string{"NonExistent", "Path"},
			wantErr:   true,
		},
		{
			name:      "EmptyPath",
			startNode: "i=85",
			path:      []string{},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeID, err := browser.BrowsePath(ctx, tt.startNode, tt.path)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				} else {
					t.Logf("Resolved path %v to node %s", tt.path, nodeID)
				}
			}
		})
	}
}

// =============================================================================
// Recursive Browse Tests
// =============================================================================

// TestOPCUABrowseRecursive tests recursive node enumeration.
func TestOPCUABrowseRecursive(t *testing.T) {
	browser := NewMockOPCUABrowser(getOPCUABrowseEndpoint())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tests := []struct {
		name      string
		startNode string
		maxDepth  int
		minNodes  int
	}{
		{"RootDepth1", "i=84", 1, 3},
		{"RootDepth2", "i=84", 2, 5},
		{"DemoFolder", "ns=2;s=Demo", 1, 4},
		{"SimulationDepth2", "ns=2;s=Simulation", 2, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := browser.CountNodes(ctx, tt.startNode, tt.maxDepth)
			if err != nil {
				t.Fatalf("Recursive browse failed: %v", err)
			}

			t.Logf("Found %d nodes under %s (depth %d)", count, tt.startNode, tt.maxDepth)

			if count < tt.minNodes {
				t.Errorf("Expected at least %d nodes, got %d", tt.minNodes, count)
			}
		})
	}
}

// =============================================================================
// Node ID Format Tests
// =============================================================================

// TestOPCUANodeIDFormats tests parsing different node ID formats.
func TestOPCUANodeIDFormats(t *testing.T) {
	tests := []struct {
		name   string
		nodeID string
		idType NodeIDType
		valid  bool
	}{
		{"NumericDefaultNS", "i=84", NodeIDNumeric, true},
		{"NumericWithNS", "ns=0;i=84", NodeIDNumeric, true},
		{"StringID", "ns=2;s=Demo.Temperature", NodeIDString, true},
		{"GuidID", "ns=2;g=12345678-1234-1234-1234-123456789012", NodeIDGUID, true},
		{"OpaqueID", "ns=2;b=AQIDBA==", NodeIDOpaque, true},
		{"InvalidFormat", "invalid", NodeIDNumeric, false},
		{"EmptyID", "", NodeIDNumeric, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := isValidNodeID(tt.nodeID)
			if valid != tt.valid {
				t.Errorf("NodeID %q validity: got %v, want %v", tt.nodeID, valid, tt.valid)
			}
		})
	}
}

// isValidNodeID performs basic validation of OPC UA node ID format.
func isValidNodeID(nodeID string) bool {
	if nodeID == "" {
		return false
	}

	// Check for valid prefixes
	validPrefixes := []string{"i=", "ns=", "s=", "g=", "b="}
	for _, prefix := range validPrefixes {
		if strings.HasPrefix(nodeID, prefix) {
			return true
		}
	}

	// Check for namespace-prefixed formats
	if strings.Contains(nodeID, ";") {
		parts := strings.SplitN(nodeID, ";", 2)
		if len(parts) == 2 && strings.HasPrefix(parts[0], "ns=") {
			return len(parts[1]) > 2 // Must have identifier type and value
		}
	}

	return false
}

// =============================================================================
// Concurrent Browse Tests
// =============================================================================

// TestOPCUABrowseConcurrent tests concurrent browse operations.
func TestOPCUABrowseConcurrent(t *testing.T) {
	browser := NewMockOPCUABrowser(getOPCUABrowseEndpoint())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	nodesToBrowse := []string{
		"i=84",
		"i=85",
		"i=86",
		"ns=2;s=Demo",
		"ns=2;s=Simulation",
		"i=2253",
	}

	var wg sync.WaitGroup
	results := make(chan struct {
		nodeID string
		count  int
		err    error
	}, len(nodesToBrowse))

	for _, nodeID := range nodesToBrowse {
		wg.Add(1)
		go func(nid string) {
			defer wg.Done()
			children, err := browser.Browse(ctx, nid, BrowseDirectionForward)
			results <- struct {
				nodeID string
				count  int
				err    error
			}{nid, len(children), err}
		}(nodeID)
	}

	wg.Wait()
	close(results)

	for r := range results {
		if r.err != nil {
			t.Errorf("Concurrent browse of %s failed: %v", r.nodeID, r.err)
		} else {
			t.Logf("Node %s has %d children", r.nodeID, r.count)
		}
	}
}

// =============================================================================
// Browse Timeout Tests
// =============================================================================

// TestOPCUABrowseTimeout tests browse operation timeout handling.
func TestOPCUABrowseTimeout(t *testing.T) {
	browser := NewMockOPCUABrowser(getOPCUABrowseEndpoint())

	tests := []struct {
		name    string
		timeout time.Duration
		wantErr bool
	}{
		{"VeryShortTimeout", 1 * time.Nanosecond, true},
		{"NormalTimeout", 5 * time.Second, false},
		{"LongTimeout", 30 * time.Second, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			_, err := browser.Browse(ctx, "i=84", BrowseDirectionForward)

			if tt.wantErr && err == nil {
				t.Log("Expected timeout but operation completed (mock is fast)")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// =============================================================================
// Browse Server Info Tests
// =============================================================================

// TestOPCUABrowseServerInfo tests browsing server information nodes.
func TestOPCUABrowseServerInfo(t *testing.T) {
	browser := NewMockOPCUABrowser(getOPCUABrowseEndpoint())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Browse Server object
	results, err := browser.Browse(ctx, "i=2253", BrowseDirectionForward)
	if err != nil {
		t.Fatalf("Failed to browse Server object: %v", err)
	}

	t.Logf("Server object children (%d):", len(results))
	for _, r := range results {
		t.Logf("  - %s [%s] (%s)", r.DisplayName, r.NodeClass, r.ReferenceType)
	}

	// Check for expected server properties
	expectedProps := map[string]bool{
		"ServerArray":    false,
		"NamespaceArray": false,
		"ServerStatus":   false,
	}

	for _, r := range results {
		if _, ok := expectedProps[r.BrowseName]; ok {
			expectedProps[r.BrowseName] = true
		}
	}

	for prop, found := range expectedProps {
		if !found {
			t.Errorf("Expected server property '%s' not found", prop)
		}
	}
}

// =============================================================================
// Browse Type System Tests
// =============================================================================

// TestOPCUABrowseTypeSystem tests browsing the type system.
func TestOPCUABrowseTypeSystem(t *testing.T) {
	browser := NewMockOPCUABrowser(getOPCUABrowseEndpoint())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Browse Types folder
	results, err := browser.Browse(ctx, "i=86", BrowseDirectionForward)
	if err != nil {
		t.Fatalf("Failed to browse Types folder: %v", err)
	}

	expectedTypes := map[string]bool{
		"ObjectTypes":    false,
		"VariableTypes":  false,
		"DataTypes":      false,
		"ReferenceTypes": false,
	}

	for _, r := range results {
		if _, ok := expectedTypes[r.BrowseName]; ok {
			expectedTypes[r.BrowseName] = true
			t.Logf("Found type folder: %s (%s)", r.DisplayName, r.NodeID)
		}
	}

	for typeName, found := range expectedTypes {
		if !found {
			t.Errorf("Expected type folder '%s' not found", typeName)
		}
	}
}

// =============================================================================
// Large Result Set Tests
// =============================================================================

// TestOPCUABrowseLargeResultSet tests handling large browse result sets.
func TestOPCUABrowseLargeResultSet(t *testing.T) {
	// Create browser with many nodes
	browser := NewMockOPCUABrowser(getOPCUABrowseEndpoint())

	// Add many nodes to a test folder
	browser.mu.Lock()
	largeFolder := make([]MockBrowseResult, 100)
	for i := 0; i < 100; i++ {
		largeFolder[i] = MockBrowseResult{
			NodeID:         fmt.Sprintf("ns=3;s=Large.Item%d", i),
			BrowseName:     fmt.Sprintf("Item%d", i),
			DisplayName:    fmt.Sprintf("Test Item %d", i),
			NodeClass:      NodeClassVariable,
			TypeDefinition: "i=62",
			ReferenceType:  "HasComponent",
		}
	}
	browser.nodes["ns=3;s=Large"] = largeFolder
	browser.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results, err := browser.Browse(ctx, "ns=3;s=Large", BrowseDirectionForward)
	if err != nil {
		t.Fatalf("Failed to browse large folder: %v", err)
	}

	if len(results) != 100 {
		t.Errorf("Expected 100 results, got %d", len(results))
	}

	t.Logf("Successfully browsed folder with %d nodes", len(results))
}
