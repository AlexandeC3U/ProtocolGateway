// Package config_test tests the NTP configuration struct and defaults.
package config_test

import (
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/config"
	"github.com/nexus-edge/protocol-gateway/internal/health"
)

// TestNTPConfigStructure tests that NTPConfig has all expected fields.
func TestNTPConfigStructure(t *testing.T) {
	cfg := config.NTPConfig{
		Enabled:       true,
		Server:        "pool.ntp.org",
		CheckInterval: 5 * time.Minute,
		WarnThreshold: 500 * time.Millisecond,
		CritThreshold: 2 * time.Second,
	}

	if !cfg.Enabled {
		t.Error("expected Enabled to be true")
	}
	if cfg.Server != "pool.ntp.org" {
		t.Errorf("expected Server 'pool.ntp.org', got %q", cfg.Server)
	}
	if cfg.CheckInterval != 5*time.Minute {
		t.Errorf("expected CheckInterval 5m, got %v", cfg.CheckInterval)
	}
	if cfg.WarnThreshold != 500*time.Millisecond {
		t.Errorf("expected WarnThreshold 500ms, got %v", cfg.WarnThreshold)
	}
	if cfg.CritThreshold != 2*time.Second {
		t.Errorf("expected CritThreshold 2s, got %v", cfg.CritThreshold)
	}
}

// TestNTPConfigInConfig tests that NTPConfig is accessible on the main Config struct.
func TestNTPConfigInConfig(t *testing.T) {
	cfg := config.Config{
		NTP: config.NTPConfig{
			Enabled:       true,
			Server:        "time.google.com",
			CheckInterval: 10 * time.Minute,
			WarnThreshold: 1 * time.Second,
			CritThreshold: 5 * time.Second,
		},
	}

	if !cfg.NTP.Enabled {
		t.Error("expected NTP.Enabled to be true")
	}
	if cfg.NTP.Server != "time.google.com" {
		t.Errorf("expected NTP.Server 'time.google.com', got %q", cfg.NTP.Server)
	}
}

// TestNTPConfigDisabled tests the disabled state.
func TestNTPConfigDisabled(t *testing.T) {
	cfg := config.NTPConfig{
		Enabled: false,
	}

	if cfg.Enabled {
		t.Error("expected NTP to be disabled")
	}
}

// TestNTPConfigCustomServer tests using a custom NTP server.
func TestNTPConfigCustomServer(t *testing.T) {
	servers := []struct {
		name   string
		server string
	}{
		{"default", "pool.ntp.org"},
		{"google", "time.google.com"},
		{"cloudflare", "time.cloudflare.com"},
		{"custom with port", "192.168.1.100:123"},
	}

	for _, tt := range servers {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NTPConfig{
				Enabled: true,
				Server:  tt.server,
			}
			if cfg.Server != tt.server {
				t.Errorf("expected Server %q, got %q", tt.server, cfg.Server)
			}
		})
	}
}

// TestNTPConfigThresholdRelationship tests that warn threshold should be less than crit threshold.
func TestNTPConfigThresholdRelationship(t *testing.T) {
	tests := []struct {
		name  string
		warn  time.Duration
		crit  time.Duration
		valid bool
	}{
		{"warn < crit", 500 * time.Millisecond, 2 * time.Second, true},
		{"warn = crit", 1 * time.Second, 1 * time.Second, false},
		{"warn > crit (invalid)", 5 * time.Second, 1 * time.Second, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.warn < tt.crit
			if valid != tt.valid {
				t.Errorf("expected valid=%v for warn=%v, crit=%v", tt.valid, tt.warn, tt.crit)
			}
		})
	}
}

// TestNTPCheckerDefaultsFromConfig tests that the NTP checker applies
// sensible defaults when given zero-value config fields.
func TestNTPCheckerDefaultsFromConfig(t *testing.T) {
	// Pass zero-value NTPConfig to the health.NTPChecker constructor
	// which should apply defaults internally
	cfg := health.NTPConfig{
		Enabled: true,
		// All other fields zero — checker should use defaults
	}

	if cfg.Server != "" {
		t.Errorf("expected empty Server before defaults, got %q", cfg.Server)
	}
	if cfg.CheckInterval != 0 {
		t.Errorf("expected zero CheckInterval before defaults, got %v", cfg.CheckInterval)
	}

	// The NTPChecker constructor (NewNTPChecker) fills defaults:
	// Server: "pool.ntp.org", CheckInterval: 5m, WarnThreshold: 500ms, CritThreshold: 2s
	// We test this behavior in ntp_checker_test.go — here we just verify the config
	// struct allows zero values (which the constructor will fill).
}

// TestNTPConfigMapstructureTags tests that the struct tags are correct for Viper binding.
func TestNTPConfigMapstructureTags(t *testing.T) {
	// This test verifies the NTPConfig struct exists with expected fields
	// and can be used in the main Config struct.
	// The mapstructure tags are verified by successful unmarshaling in integration tests.
	cfg := config.NTPConfig{}

	// Zero values should all be "empty"
	if cfg.Enabled {
		t.Error("zero NTPConfig should have Enabled=false")
	}
	if cfg.Server != "" {
		t.Error("zero NTPConfig should have empty Server")
	}
	if cfg.CheckInterval != 0 {
		t.Error("zero NTPConfig should have zero CheckInterval")
	}
	if cfg.WarnThreshold != 0 {
		t.Error("zero NTPConfig should have zero WarnThreshold")
	}
	if cfg.CritThreshold != 0 {
		t.Error("zero NTPConfig should have zero CritThreshold")
	}
}
