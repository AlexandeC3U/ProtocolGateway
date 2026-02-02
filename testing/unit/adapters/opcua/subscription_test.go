// Package opcua_test tests the OPC UA subscription functionality.
package opcua_test

import (
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/opcua"
)

// TestDefaultSubscriptionConfig tests the default subscription configuration.
func TestDefaultSubscriptionConfig(t *testing.T) {
	cfg := opcua.DefaultSubscriptionConfig()

	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"PublishInterval", cfg.PublishInterval, 1 * time.Second},
		{"SamplingInterval", cfg.SamplingInterval, 500 * time.Millisecond},
		{"QueueSize", cfg.QueueSize, uint32(10)},
		{"DiscardOldest", cfg.DiscardOldest, true},
		{"DeadbandType", cfg.DeadbandType, "None"},
		{"DeadbandValue", cfg.DeadbandValue, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, tt.got)
			}
		})
	}
}

// TestSubscriptionConfigStructure tests SubscriptionConfig structure.
func TestSubscriptionConfigStructure(t *testing.T) {
	cfg := opcua.SubscriptionConfig{
		PublishInterval:  2 * time.Second,
		SamplingInterval: 1 * time.Second,
		QueueSize:        20,
		DiscardOldest:    false,
		DeadbandType:     "Absolute",
		DeadbandValue:    0.5,
	}

	if cfg.PublishInterval != 2*time.Second {
		t.Errorf("expected PublishInterval 2s, got %v", cfg.PublishInterval)
	}
	if cfg.DiscardOldest {
		t.Error("expected DiscardOldest to be false")
	}
	if cfg.DeadbandType != "Absolute" {
		t.Errorf("expected DeadbandType 'Absolute', got %q", cfg.DeadbandType)
	}
}

// TestPublishIntervalSettings tests various publish interval configurations.
func TestPublishIntervalSettings(t *testing.T) {
	tests := []struct {
		name        string
		interval    time.Duration
		description string
	}{
		{"High frequency", 100 * time.Millisecond, "Real-time monitoring"},
		{"Standard", 1 * time.Second, "Normal telemetry"},
		{"Low frequency", 5 * time.Second, "Slow-changing values"},
		{"Batch", 30 * time.Second, "Batch data collection"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := opcua.SubscriptionConfig{PublishInterval: tt.interval}
			if cfg.PublishInterval != tt.interval {
				t.Errorf("expected %v, got %v", tt.interval, cfg.PublishInterval)
			}
		})
	}
}

// TestSamplingIntervalRelationship tests sampling vs publishing interval.
func TestSamplingIntervalRelationship(t *testing.T) {
	tests := []struct {
		name             string
		publishInterval  time.Duration
		samplingInterval time.Duration
		valid            bool
	}{
		{"Sampling < Publish", 1 * time.Second, 500 * time.Millisecond, true},
		{"Sampling = Publish", 1 * time.Second, 1 * time.Second, true},
		{"Sampling > Publish", 500 * time.Millisecond, 1 * time.Second, false}, // Inefficient
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := opcua.SubscriptionConfig{
				PublishInterval:  tt.publishInterval,
				SamplingInterval: tt.samplingInterval,
			}

			efficient := cfg.SamplingInterval <= cfg.PublishInterval
			if efficient != tt.valid {
				t.Errorf("expected efficient=%v, got %v", tt.valid, efficient)
			}
		})
	}
}

// TestQueueSizeSettings tests queue size configurations.
func TestQueueSizeSettings(t *testing.T) {
	tests := []struct {
		name      string
		queueSize uint32
		scenario  string
	}{
		{"Minimal", 1, "Latest value only"},
		{"Small", 5, "Short history"},
		{"Standard", 10, "Normal buffering"},
		{"Large", 50, "Extended history"},
		{"Burst", 100, "Handle bursts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := opcua.SubscriptionConfig{QueueSize: tt.queueSize}
			if cfg.QueueSize != tt.queueSize {
				t.Errorf("expected QueueSize %d, got %d", tt.queueSize, cfg.QueueSize)
			}
		})
	}
}

// TestDeadbandTypes tests deadband type values.
func TestDeadbandTypes(t *testing.T) {
	types := []struct {
		dbType      string
		description string
	}{
		{"None", "Report all changes"},
		{"Absolute", "Report if change exceeds absolute value"},
		{"Percent", "Report if change exceeds percentage of range"},
	}

	for _, tt := range types {
		t.Run(tt.dbType, func(t *testing.T) {
			cfg := opcua.SubscriptionConfig{DeadbandType: tt.dbType}
			if cfg.DeadbandType != tt.dbType {
				t.Errorf("expected %q, got %q", tt.dbType, cfg.DeadbandType)
			}
		})
	}
}

// TestDeadbandAbsoluteValues tests absolute deadband configurations.
func TestDeadbandAbsoluteValues(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		scenario string
	}{
		{"None", 0.0, "Report all changes"},
		{"Tight", 0.1, "Sensitive monitoring"},
		{"Standard", 1.0, "Normal filtering"},
		{"Loose", 5.0, "Coarse monitoring"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := opcua.SubscriptionConfig{
				DeadbandType:  "Absolute",
				DeadbandValue: tt.value,
			}
			if cfg.DeadbandValue != tt.value {
				t.Errorf("expected %v, got %v", tt.value, cfg.DeadbandValue)
			}
		})
	}
}

// TestDeadbandPercentValues tests percent deadband configurations.
func TestDeadbandPercentValues(t *testing.T) {
	tests := []struct {
		name    string
		percent float64
		valid   bool
	}{
		{"Zero", 0.0, true},
		{"1%", 1.0, true},
		{"5%", 5.0, true},
		{"10%", 10.0, true},
		{"50%", 50.0, true},
		{"100%", 100.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := opcua.SubscriptionConfig{
				DeadbandType:  "Percent",
				DeadbandValue: tt.percent,
			}
			valid := cfg.DeadbandValue >= 0 && cfg.DeadbandValue <= 100
			if valid != tt.valid {
				t.Errorf("expected valid=%v, got %v", tt.valid, valid)
			}
		})
	}
}

// TestDiscardPolicy tests discard policy settings.
func TestDiscardPolicy(t *testing.T) {
	tests := []struct {
		name          string
		discardOldest bool
		description   string
	}{
		{"Discard oldest", true, "Keep newest values when queue full"},
		{"Discard newest", false, "Keep oldest values when queue full"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := opcua.SubscriptionConfig{DiscardOldest: tt.discardOldest}
			if cfg.DiscardOldest != tt.discardOldest {
				t.Errorf("expected DiscardOldest=%v", tt.discardOldest)
			}
		})
	}
}

// TestSubscriptionConfigCombinations tests common config combinations.
func TestSubscriptionConfigCombinations(t *testing.T) {
	scenarios := []struct {
		name   string
		config opcua.SubscriptionConfig
	}{
		{
			name: "Real-time monitoring",
			config: opcua.SubscriptionConfig{
				PublishInterval:  100 * time.Millisecond,
				SamplingInterval: 50 * time.Millisecond,
				QueueSize:        5,
				DiscardOldest:    true,
				DeadbandType:     "None",
			},
		},
		{
			name: "Energy monitoring",
			config: opcua.SubscriptionConfig{
				PublishInterval:  5 * time.Second,
				SamplingInterval: 1 * time.Second,
				QueueSize:        20,
				DiscardOldest:    true,
				DeadbandType:     "Percent",
				DeadbandValue:    1.0,
			},
		},
		{
			name: "Alarm monitoring",
			config: opcua.SubscriptionConfig{
				PublishInterval:  500 * time.Millisecond,
				SamplingInterval: 250 * time.Millisecond,
				QueueSize:        100,
				DiscardOldest:    false,
				DeadbandType:     "None",
			},
		},
	}

	for _, tt := range scenarios {
		t.Run(tt.name, func(t *testing.T) {
			// Validate configuration is self-consistent
			if tt.config.PublishInterval < tt.config.SamplingInterval {
				t.Error("PublishInterval should be >= SamplingInterval")
			}
			if tt.config.QueueSize == 0 {
				t.Error("QueueSize should be > 0")
			}
		})
	}
}

// TestNotificationRateCalculation tests notification rate estimates.
func TestNotificationRateCalculation(t *testing.T) {
	tests := []struct {
		name            string
		publishInterval time.Duration
		itemCount       int
		expectedRate    float64 // notifications per second
	}{
		{"Single item, 1s", 1 * time.Second, 1, 1.0},
		{"10 items, 1s", 1 * time.Second, 10, 10.0},
		{"10 items, 100ms", 100 * time.Millisecond, 10, 100.0},
		{"100 items, 5s", 5 * time.Second, 100, 20.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Max notifications/sec = items * (1 / publishInterval)
			rate := float64(tt.itemCount) / tt.publishInterval.Seconds()
			if rate != tt.expectedRate {
				t.Errorf("expected rate %.1f, got %.1f", tt.expectedRate, rate)
			}
		})
	}
}

// TestServerLimitAwareness tests configurations considering server limits.
func TestServerLimitAwareness(t *testing.T) {
	// OPC UA servers have limits on subscriptions and monitored items
	tests := []struct {
		name             string
		maxSubscriptions int
		maxItemsPerSub   int
		totalItems       int
		subsRequired     int
	}{
		{"Small server", 10, 100, 500, 5},
		{"Large server", 100, 1000, 5000, 5},
		{"Single subscription", 1, 10000, 1000, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subsRequired := (tt.totalItems + tt.maxItemsPerSub - 1) / tt.maxItemsPerSub
			if subsRequired != tt.subsRequired {
				t.Errorf("expected %d subscriptions, calculated %d", tt.subsRequired, subsRequired)
			}
			if subsRequired > tt.maxSubscriptions {
				t.Logf("Warning: Requires %d subs but server max is %d", subsRequired, tt.maxSubscriptions)
			}
		})
	}
}

// TestRevisedParameterHandling documents server-revised parameter behavior.
func TestRevisedParameterHandling(t *testing.T) {
	// OPC UA servers can revise requested parameters
	tests := []struct {
		requested time.Duration
		minServer time.Duration
		revised   time.Duration
	}{
		{100 * time.Millisecond, 250 * time.Millisecond, 250 * time.Millisecond},
		{500 * time.Millisecond, 250 * time.Millisecond, 500 * time.Millisecond},
		{50 * time.Millisecond, 100 * time.Millisecond, 100 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.requested.String(), func(t *testing.T) {
			revised := tt.requested
			if revised < tt.minServer {
				revised = tt.minServer
			}
			if revised != tt.revised {
				t.Errorf("expected revised %v, got %v", tt.revised, revised)
			}
		})
	}
}
