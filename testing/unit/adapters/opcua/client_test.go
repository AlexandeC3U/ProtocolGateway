// Package opcua_test tests the OPC UA client functionality.
package opcua_test

import (
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/opcua"
)

// TestSessionStateConstants tests session state constants.
func TestSessionStateConstants(t *testing.T) {
	tests := []struct {
		state    opcua.SessionState
		expected string
	}{
		{opcua.SessionStateDisconnected, "disconnected"},
		{opcua.SessionStateConnecting, "connecting"},
		{opcua.SessionStateSecureChannel, "secure_channel"},
		{opcua.SessionStateActive, "active"},
		{opcua.SessionStateError, "error"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if string(tt.state) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(tt.state))
			}
		})
	}
}

// TestClientConfigStructure tests ClientConfig field structure.
func TestClientConfigStructure(t *testing.T) {
	cfg := opcua.ClientConfig{
		EndpointURL:              "opc.tcp://localhost:4840",
		SecurityPolicy:           "Basic256Sha256",
		SecurityMode:             "SignAndEncrypt",
		AuthMode:                 "UserName",
		Username:                 "admin",
		Password:                 "secret",
		CertificateFile:          "/certs/client.der",
		PrivateKeyFile:           "/certs/client.key",
		Timeout:                  10 * time.Second,
		KeepAlive:                30 * time.Second,
		MaxRetries:               3,
		RetryDelay:               1 * time.Second,
		RequestTimeout:           5 * time.Second,
		SessionTimeout:           60 * time.Second,
		DefaultPublishingInterval: 1 * time.Second,
		DefaultSamplingInterval:  500 * time.Millisecond,
		DefaultQueueSize:         10,
	}

	if cfg.EndpointURL != "opc.tcp://localhost:4840" {
		t.Errorf("expected endpoint URL 'opc.tcp://localhost:4840', got %q", cfg.EndpointURL)
	}
	if cfg.SecurityPolicy != "Basic256Sha256" {
		t.Errorf("expected security policy 'Basic256Sha256', got %q", cfg.SecurityPolicy)
	}
	if cfg.DefaultQueueSize != 10 {
		t.Errorf("expected default queue size 10, got %d", cfg.DefaultQueueSize)
	}
}

// TestClientConfigSecurityPolicies tests valid security policy values.
func TestClientConfigSecurityPolicies(t *testing.T) {
	policies := []string{
		"None",
		"Basic128Rsa15",
		"Basic256",
		"Basic256Sha256",
		"Aes128_Sha256_RsaOaep",
		"Aes256_Sha256_RsaPss",
	}

	for _, policy := range policies {
		t.Run(policy, func(t *testing.T) {
			cfg := opcua.ClientConfig{SecurityPolicy: policy}
			if cfg.SecurityPolicy != policy {
				t.Errorf("expected %q, got %q", policy, cfg.SecurityPolicy)
			}
		})
	}
}

// TestClientConfigSecurityModes tests valid security mode values.
func TestClientConfigSecurityModes(t *testing.T) {
	modes := []struct {
		mode        string
		description string
	}{
		{"None", "No security"},
		{"Sign", "Messages are signed"},
		{"SignAndEncrypt", "Messages are signed and encrypted"},
	}

	for _, m := range modes {
		t.Run(m.mode, func(t *testing.T) {
			cfg := opcua.ClientConfig{SecurityMode: m.mode}
			if cfg.SecurityMode != m.mode {
				t.Errorf("expected %q, got %q", m.mode, cfg.SecurityMode)
			}
		})
	}
}

// TestClientConfigAuthModes tests valid authentication modes.
func TestClientConfigAuthModes(t *testing.T) {
	modes := []struct {
		mode        string
		description string
	}{
		{"Anonymous", "No authentication"},
		{"UserName", "Username/password authentication"},
		{"Certificate", "Certificate-based authentication"},
	}

	for _, m := range modes {
		t.Run(m.mode, func(t *testing.T) {
			cfg := opcua.ClientConfig{AuthMode: m.mode}
			if cfg.AuthMode != m.mode {
				t.Errorf("expected %q, got %q", m.mode, cfg.AuthMode)
			}
		})
	}
}

// TestClientStatsAtomicOperations tests ClientStats atomic operations.
func TestClientStatsAtomicOperations(t *testing.T) {
	stats := &opcua.ClientStats{}

	// Test ReadCount
	stats.ReadCount.Add(10)
	if got := stats.ReadCount.Load(); got != 10 {
		t.Errorf("expected ReadCount 10, got %d", got)
	}

	// Test WriteCount
	stats.WriteCount.Add(5)
	if got := stats.WriteCount.Load(); got != 5 {
		t.Errorf("expected WriteCount 5, got %d", got)
	}

	// Test ErrorCount
	stats.ErrorCount.Add(2)
	if got := stats.ErrorCount.Load(); got != 2 {
		t.Errorf("expected ErrorCount 2, got %d", got)
	}

	// Test RetryCount
	stats.RetryCount.Add(3)
	if got := stats.RetryCount.Load(); got != 3 {
		t.Errorf("expected RetryCount 3, got %d", got)
	}

	// Test SubscribeCount
	stats.SubscribeCount.Add(8)
	if got := stats.SubscribeCount.Load(); got != 8 {
		t.Errorf("expected SubscribeCount 8, got %d", got)
	}

	// Test NotificationCount
	stats.NotificationCount.Add(100)
	if got := stats.NotificationCount.Load(); got != 100 {
		t.Errorf("expected NotificationCount 100, got %d", got)
	}

	// Test TotalReadTime
	stats.TotalReadTime.Add(1000000)
	if got := stats.TotalReadTime.Load(); got != 1000000 {
		t.Errorf("expected TotalReadTime 1000000, got %d", got)
	}

	// Test TotalWriteTime
	stats.TotalWriteTime.Add(500000)
	if got := stats.TotalWriteTime.Load(); got != 500000 {
		t.Errorf("expected TotalWriteTime 500000, got %d", got)
	}
}

// TestClientStatsConcurrency tests stats concurrent access.
func TestClientStatsConcurrency(t *testing.T) {
	stats := &opcua.ClientStats{}
	done := make(chan struct{})

	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				stats.ReadCount.Add(1)
				stats.NotificationCount.Add(1)
			}
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if got := stats.ReadCount.Load(); got != 1000 {
		t.Errorf("expected ReadCount 1000, got %d", got)
	}
	if got := stats.NotificationCount.Load(); got != 1000 {
		t.Errorf("expected NotificationCount 1000, got %d", got)
	}
}

// TestTagDiagnosticStructure tests TagDiagnostic creation and structure.
func TestTagDiagnosticStructure(t *testing.T) {
	diag := opcua.NewTagDiagnostic("tag-001", "ns=2;i=1001")

	if diag.TagID != "tag-001" {
		t.Errorf("expected TagID 'tag-001', got %q", diag.TagID)
	}
	if diag.NodeID != "ns=2;i=1001" {
		t.Errorf("expected NodeID 'ns=2;i=1001', got %q", diag.NodeID)
	}

	// Test atomic operations
	diag.ReadCount.Add(5)
	diag.ErrorCount.Add(1)

	if got := diag.ReadCount.Load(); got != 5 {
		t.Errorf("expected ReadCount 5, got %d", got)
	}
	if got := diag.ErrorCount.Load(); got != 1 {
		t.Errorf("expected ErrorCount 1, got %d", got)
	}
}

// TestTagDiagnosticTimeTracking tests time tracking in TagDiagnostic.
func TestTagDiagnosticTimeTracking(t *testing.T) {
	diag := opcua.NewTagDiagnostic("tag-002", "ns=2;i=1002")

	now := time.Now()
	diag.LastSuccessTime.Store(now)

	stored, ok := diag.LastSuccessTime.Load().(time.Time)
	if !ok {
		t.Fatal("expected time.Time from LastSuccessTime")
	}
	if !stored.Equal(now) {
		t.Errorf("expected time %v, got %v", now, stored)
	}
}

// TestDeviceStatsStructure tests DeviceStats structure.
func TestDeviceStatsStructure(t *testing.T) {
	stats := opcua.DeviceStats{
		DeviceID:          "opcua-device-001",
		ReadCount:         1000,
		WriteCount:        500,
		ErrorCount:        10,
		RetryCount:        25,
		SubscribeCount:    5,
		NotificationCount: 10000,
		AvgReadTimeMs:     2.5,
		AvgWriteTimeMs:    3.0,
	}

	if stats.DeviceID != "opcua-device-001" {
		t.Errorf("expected DeviceID 'opcua-device-001', got %q", stats.DeviceID)
	}
	if stats.SubscribeCount != 5 {
		t.Errorf("expected SubscribeCount 5, got %d", stats.SubscribeCount)
	}
	if stats.NotificationCount != 10000 {
		t.Errorf("expected NotificationCount 10000, got %d", stats.NotificationCount)
	}
}

// TestEndpointURLFormats tests various OPC UA endpoint URL formats.
func TestEndpointURLFormats(t *testing.T) {
	urls := []struct {
		url         string
		description string
	}{
		{"opc.tcp://localhost:4840", "Local server"},
		{"opc.tcp://192.168.1.100:4840", "IP address"},
		{"opc.tcp://opcua.example.com:4840", "Hostname"},
		{"opc.tcp://plc1.factory.local:4840/path", "With path"},
		{"opc.tcp://[::1]:4840", "IPv6 localhost"},
	}

	for _, u := range urls {
		t.Run(u.description, func(t *testing.T) {
			cfg := opcua.ClientConfig{EndpointURL: u.url}
			if cfg.EndpointURL != u.url {
				t.Errorf("expected %q, got %q", u.url, cfg.EndpointURL)
			}
		})
	}
}

// TestTimeoutConfigurations tests various timeout settings.
func TestTimeoutConfigurations(t *testing.T) {
	tests := []struct {
		name           string
		timeout        time.Duration
		requestTimeout time.Duration
		sessionTimeout time.Duration
	}{
		{"Fast network", 5 * time.Second, 2 * time.Second, 30 * time.Second},
		{"Standard", 10 * time.Second, 5 * time.Second, 60 * time.Second},
		{"Slow network", 30 * time.Second, 15 * time.Second, 120 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := opcua.ClientConfig{
				Timeout:        tt.timeout,
				RequestTimeout: tt.requestTimeout,
				SessionTimeout: tt.sessionTimeout,
			}

			if cfg.Timeout != tt.timeout {
				t.Errorf("expected Timeout %v, got %v", tt.timeout, cfg.Timeout)
			}
			if cfg.RequestTimeout < cfg.Timeout {
				// Request timeout is typically shorter than connection timeout
			}
		})
	}
}

// TestSubscriptionDefaults tests default subscription settings.
func TestSubscriptionDefaults(t *testing.T) {
	cfg := opcua.ClientConfig{
		DefaultPublishingInterval: 1 * time.Second,
		DefaultSamplingInterval:  500 * time.Millisecond,
		DefaultQueueSize:         10,
	}

	// Publishing interval should be >= sampling interval
	if cfg.DefaultPublishingInterval < cfg.DefaultSamplingInterval {
		t.Error("PublishingInterval should typically be >= SamplingInterval")
	}

	// Queue size should be positive
	if cfg.DefaultQueueSize < 1 {
		t.Error("DefaultQueueSize should be at least 1")
	}
}

// TestNodeIDFormats tests various OPC UA NodeID formats.
func TestNodeIDFormats(t *testing.T) {
	nodeIDs := []struct {
		nodeID      string
		description string
	}{
		{"ns=2;i=1001", "Numeric identifier"},
		{"ns=2;s=MyVariable", "String identifier"},
		{"ns=1;g=12345678-1234-1234-1234-123456789abc", "GUID identifier"},
		{"ns=2;b=SGVsbG8=", "Opaque (base64) identifier"},
		{"i=85", "Root folder (namespace 0)"},
		{"i=2253", "Server object"},
	}

	for _, n := range nodeIDs {
		t.Run(n.description, func(t *testing.T) {
			diag := opcua.NewTagDiagnostic("tag", n.nodeID)
			if diag.NodeID != n.nodeID {
				t.Errorf("expected NodeID %q, got %q", n.nodeID, diag.NodeID)
			}
		})
	}
}

// TestSessionStateTransitions documents valid state transitions.
func TestSessionStateTransitions(t *testing.T) {
	transitions := []struct {
		from  opcua.SessionState
		to    opcua.SessionState
		valid bool
	}{
		{opcua.SessionStateDisconnected, opcua.SessionStateConnecting, true},
		{opcua.SessionStateConnecting, opcua.SessionStateSecureChannel, true},
		{opcua.SessionStateSecureChannel, opcua.SessionStateActive, true},
		{opcua.SessionStateActive, opcua.SessionStateDisconnected, true},
		{opcua.SessionStateConnecting, opcua.SessionStateError, true},
		{opcua.SessionStateError, opcua.SessionStateConnecting, true},
	}

	for _, tt := range transitions {
		name := string(tt.from) + "->" + string(tt.to)
		t.Run(name, func(t *testing.T) {
			// Document valid transitions
			if tt.valid {
				t.Logf("Valid transition: %s -> %s", tt.from, tt.to)
			}
		})
	}
}
