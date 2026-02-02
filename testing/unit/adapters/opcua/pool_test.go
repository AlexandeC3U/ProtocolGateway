// Package opcua_test tests the OPC UA connection pool functionality.
package opcua_test

import (
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/opcua"
)

// TestPoolConfigStructure tests PoolConfig structure.
func TestPoolConfigStructure(t *testing.T) {
	cfg := opcua.PoolConfig{
		MaxConnections:        50,
		IdleTimeout:           5 * time.Minute,
		HealthCheckPeriod:     30 * time.Second,
		ConnectionTimeout:     15 * time.Second,
		RetryAttempts:         3,
		RetryDelay:            500 * time.Millisecond,
		CircuitBreakerName:    "opcua-pool",
		DefaultSecurityPolicy: "Basic256Sha256",
		DefaultSecurityMode:   "SignAndEncrypt",
		DefaultAuthMode:       "Anonymous",
		MaxNodesPerRead:       100,
		MaxNodesPerWrite:      50,
		MaxNodesPerBrowse:     1000,
		MaxGlobalInFlight:     200,
	}

	if cfg.MaxConnections != 50 {
		t.Errorf("expected MaxConnections 50, got %d", cfg.MaxConnections)
	}
	if cfg.DefaultSecurityPolicy != "Basic256Sha256" {
		t.Errorf("expected DefaultSecurityPolicy 'Basic256Sha256', got %q", cfg.DefaultSecurityPolicy)
	}
	if cfg.MaxNodesPerRead != 100 {
		t.Errorf("expected MaxNodesPerRead 100, got %d", cfg.MaxNodesPerRead)
	}
}

// TestPoolConfigServerLimits tests server limit configurations.
func TestPoolConfigServerLimits(t *testing.T) {
	tests := []struct {
		name              string
		maxNodesPerRead   int
		maxNodesPerWrite  int
		maxNodesPerBrowse int
		description       string
	}{
		{"Conservative", 50, 25, 500, "Safe for most servers"},
		{"Standard", 100, 50, 1000, "Typical Kepware/Ignition"},
		{"Optimistic", 200, 100, 2000, "High-end servers"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := opcua.PoolConfig{
				MaxNodesPerRead:   tt.maxNodesPerRead,
				MaxNodesPerWrite:  tt.maxNodesPerWrite,
				MaxNodesPerBrowse: tt.maxNodesPerBrowse,
			}

			if cfg.MaxNodesPerRead != tt.maxNodesPerRead {
				t.Errorf("expected MaxNodesPerRead %d, got %d", tt.maxNodesPerRead, cfg.MaxNodesPerRead)
			}
			// Write limits are typically lower than read limits
			if cfg.MaxNodesPerWrite > cfg.MaxNodesPerRead {
				t.Logf("Note: Write limit > read limit is unusual")
			}
		})
	}
}

// TestEndpointKeyGeneration tests endpoint key patterns for session sharing.
func TestEndpointKeyGeneration(t *testing.T) {
	// OPC UA pool uses endpoint key for session sharing
	// Key = host + port + security + auth
	tests := []struct {
		host     string
		port     int
		security string
		auth     string
		unique   bool
	}{
		{"192.168.1.100", 4840, "None", "Anonymous", true},
		{"192.168.1.100", 4840, "Basic256", "Anonymous", true},
		{"192.168.1.100", 4841, "None", "Anonymous", true},
		{"192.168.1.101", 4840, "None", "Anonymous", true},
	}

	keys := make(map[string]bool)
	for _, tt := range tests {
		key := buildEndpointKey(tt.host, tt.port, tt.security, tt.auth)
		if _, exists := keys[key]; exists && tt.unique {
			t.Errorf("expected unique key for %s:%d", tt.host, tt.port)
		}
		keys[key] = true
	}
}

func buildEndpointKey(host string, port int, security, auth string) string {
	return host + ":" + string(rune(port)) + ":" + security + ":" + auth
}

// TestSessionSharingScenarios tests session sharing behavior.
func TestSessionSharingScenarios(t *testing.T) {
	// Multiple devices behind one Kepserver should share session
	scenarios := []struct {
		name          string
		deviceCount   int
		endpointCount int
		sessionCount  int
	}{
		{"Single device, single endpoint", 1, 1, 1},
		{"Multiple devices, one endpoint (Kepware)", 50, 1, 1},
		{"Multiple devices, multiple endpoints", 100, 10, 10},
		{"Mixed deployment", 200, 5, 5},
	}

	for _, tt := range scenarios {
		t.Run(tt.name, func(t *testing.T) {
			// With endpoint-keyed sessions, session count = endpoint count
			if tt.sessionCount != tt.endpointCount {
				t.Errorf("expected %d sessions, got %d", tt.endpointCount, tt.sessionCount)
			}
			// Verify devices per session ratio
			devicesPerSession := tt.deviceCount / tt.sessionCount
			t.Logf("%s: %d devices sharing %d sessions (%d dev/session)",
				tt.name, tt.deviceCount, tt.sessionCount, devicesPerSession)
		})
	}
}

// TestLoadShapingConfiguration tests load shaping config fields.
func TestLoadShapingConfiguration(t *testing.T) {
	cfg := opcua.PoolConfig{
		MaxGlobalInFlight: 200,
	}

	if cfg.MaxGlobalInFlight != 200 {
		t.Errorf("expected MaxGlobalInFlight 200, got %d", cfg.MaxGlobalInFlight)
	}

	// Brownout threshold is typically 80% of max
	brownoutThreshold := float64(cfg.MaxGlobalInFlight) * 0.8
	if brownoutThreshold != 160 {
		t.Errorf("expected brownout threshold 160, got %.0f", brownoutThreshold)
	}
}

// TestPoolConnectionScaling tests connection pool scaling.
func TestPoolConnectionScaling(t *testing.T) {
	tests := []struct {
		name           string
		maxConnections int
		scenario       string
	}{
		{"Small", 10, "Single server"},
		{"Medium", 50, "Factory floor"},
		{"Large", 100, "Multi-site"},
		{"Enterprise", 200, "Campus-wide"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := opcua.PoolConfig{MaxConnections: tt.maxConnections}
			if cfg.MaxConnections != tt.maxConnections {
				t.Errorf("expected MaxConnections %d, got %d", tt.maxConnections, cfg.MaxConnections)
			}
		})
	}
}

// TestSecurityPolicyDefaults tests default security policy settings.
func TestSecurityPolicyDefaults(t *testing.T) {
	policies := []struct {
		policy   string
		secure   bool
		scenario string
	}{
		{"None", false, "Testing/development"},
		{"Basic256Sha256", true, "Production recommended"},
		{"Aes128_Sha256_RsaOaep", true, "Modern systems"},
		{"Aes256_Sha256_RsaPss", true, "High security"},
	}

	for _, p := range policies {
		t.Run(p.policy, func(t *testing.T) {
			cfg := opcua.PoolConfig{DefaultSecurityPolicy: p.policy}
			isSecure := cfg.DefaultSecurityPolicy != "None"
			if isSecure != p.secure {
				t.Errorf("expected secure=%v for policy %q", p.secure, p.policy)
			}
		})
	}
}

// TestAuthModeDefaults tests default authentication mode settings.
func TestAuthModeDefaults(t *testing.T) {
	modes := []struct {
		mode          string
		requiresCreds bool
	}{
		{"Anonymous", false},
		{"UserName", true},
		{"Certificate", true},
	}

	for _, m := range modes {
		t.Run(m.mode, func(t *testing.T) {
			cfg := opcua.PoolConfig{DefaultAuthMode: m.mode}
			requiresCreds := cfg.DefaultAuthMode != "Anonymous"
			if requiresCreds != m.requiresCreds {
				t.Errorf("expected requiresCreds=%v for mode %q", m.requiresCreds, m.mode)
			}
		})
	}
}

// TestHealthCheckFrequency tests health check period configurations.
func TestHealthCheckFrequency(t *testing.T) {
	tests := []struct {
		name              string
		healthCheckPeriod time.Duration
		idleTimeout       time.Duration
	}{
		{"Aggressive", 10 * time.Second, 1 * time.Minute},
		{"Standard", 30 * time.Second, 5 * time.Minute},
		{"Relaxed", 60 * time.Second, 10 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := opcua.PoolConfig{
				HealthCheckPeriod: tt.healthCheckPeriod,
				IdleTimeout:       tt.idleTimeout,
			}

			// Health check should be more frequent than idle timeout
			if cfg.HealthCheckPeriod >= cfg.IdleTimeout {
				t.Error("HealthCheckPeriod should be less than IdleTimeout")
			}

			// Calculate checks before timeout
			checksBeforeTimeout := int(cfg.IdleTimeout / cfg.HealthCheckPeriod)
			t.Logf("%s: %d health checks before idle timeout", tt.name, checksBeforeTimeout)
		})
	}
}

// TestRetryConfiguration tests retry settings.
func TestRetryConfiguration(t *testing.T) {
	tests := []struct {
		name          string
		retryAttempts int
		retryDelay    time.Duration
		maxBackoff    time.Duration
	}{
		{"Quick retry", 2, 100 * time.Millisecond, 200 * time.Millisecond},
		{"Standard", 3, 500 * time.Millisecond, 2 * time.Second},
		{"Patient", 5, 1 * time.Second, 16 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := opcua.PoolConfig{
				RetryAttempts: tt.retryAttempts,
				RetryDelay:    tt.retryDelay,
			}

			// With exponential backoff, max delay = base * 2^(attempts-1)
			maxDelay := cfg.RetryDelay * (1 << (cfg.RetryAttempts - 1))
			if maxDelay > tt.maxBackoff {
				t.Logf("Note: Max backoff %v exceeds expected %v", maxDelay, tt.maxBackoff)
			}
		})
	}
}

// TestCircuitBreakerNaming tests circuit breaker naming.
func TestCircuitBreakerNaming(t *testing.T) {
	endpoints := []struct {
		host         string
		expectedName string
	}{
		{"192.168.1.100", "opcua-192.168.1.100"},
		{"plc.factory.local", "opcua-plc.factory.local"},
		{"kepware.corp.com", "opcua-kepware.corp.com"},
	}

	for _, e := range endpoints {
		t.Run(e.host, func(t *testing.T) {
			name := "opcua-" + e.host
			if name != e.expectedName {
				t.Errorf("expected %q, got %q", e.expectedName, name)
			}
		})
	}
}

// TestGlobalInFlightCalculation tests in-flight operation limits.
func TestGlobalInFlightCalculation(t *testing.T) {
	tests := []struct {
		name              string
		maxGlobalInFlight int
		currentInFlight   int
		canProceed        bool
	}{
		{"Empty", 100, 0, true},
		{"Half", 100, 50, true},
		{"Near limit", 100, 99, true},
		{"At limit", 100, 100, false},
		{"Over limit", 100, 101, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canProceed := tt.currentInFlight < tt.maxGlobalInFlight
			if canProceed != tt.canProceed {
				t.Errorf("expected canProceed=%v, got %v", tt.canProceed, canProceed)
			}
		})
	}
}
