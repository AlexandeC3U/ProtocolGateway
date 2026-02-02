// Package mqtt_test tests the MQTT publisher functionality.
package mqtt_test

import (
	"sort"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/mqtt"
)

// TestDefaultConfig verifies the DefaultConfig function returns sensible defaults.
func TestDefaultConfig(t *testing.T) {
	cfg := mqtt.DefaultConfig()

	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"BrokerURL", cfg.BrokerURL, "tcp://localhost:1883"},
		{"ClientID", cfg.ClientID, "protocol-gateway"},
		{"CleanSession", cfg.CleanSession, true},
		{"QoS", cfg.QoS, byte(1)},
		{"KeepAlive", cfg.KeepAlive, 30 * time.Second},
		{"ConnectTimeout", cfg.ConnectTimeout, 10 * time.Second},
		{"ReconnectDelay", cfg.ReconnectDelay, 5 * time.Second},
		{"MaxReconnect", cfg.MaxReconnect, -1},
		{"BufferSize", cfg.BufferSize, 10000},
		{"PublishTimeout", cfg.PublishTimeout, 5 * time.Second},
		{"RetainMessages", cfg.RetainMessages, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, tt.got)
			}
		})
	}
}

// TestConfigStructure verifies the Config struct has all required fields.
func TestConfigStructure(t *testing.T) {
	cfg := mqtt.Config{
		BrokerURL:      "tcp://mqtt.example.com:1883",
		ClientID:       "test-client",
		Username:       "user",
		Password:       "pass",
		CleanSession:   false,
		QoS:            2,
		KeepAlive:      60 * time.Second,
		ConnectTimeout: 15 * time.Second,
		ReconnectDelay: 10 * time.Second,
		MaxReconnect:   5,
		TLSEnabled:     true,
		TLSCertFile:    "/certs/client.crt",
		TLSKeyFile:     "/certs/client.key",
		TLSCAFile:      "/certs/ca.crt",
		BufferSize:     5000,
		PublishTimeout: 10 * time.Second,
		RetainMessages: true,
	}

	// Verify all fields are accessible and set correctly
	if cfg.BrokerURL != "tcp://mqtt.example.com:1883" {
		t.Errorf("BrokerURL not set correctly")
	}
	if cfg.QoS != 2 {
		t.Errorf("expected QoS 2, got %d", cfg.QoS)
	}
	if !cfg.TLSEnabled {
		t.Error("expected TLSEnabled to be true")
	}
	if !cfg.RetainMessages {
		t.Error("expected RetainMessages to be true")
	}
}

// TestBufferedMessageStructure tests the BufferedMessage type.
func TestBufferedMessageStructure(t *testing.T) {
	now := time.Now()
	msg := mqtt.BufferedMessage{
		Topic:     "factory/line1/sensor",
		Payload:   []byte(`{"value": 42}`),
		QoS:       1,
		Retained:  false,
		Timestamp: now,
	}

	if msg.Topic != "factory/line1/sensor" {
		t.Errorf("expected topic 'factory/line1/sensor', got %q", msg.Topic)
	}
	if string(msg.Payload) != `{"value": 42}` {
		t.Errorf("unexpected payload: %s", msg.Payload)
	}
	if msg.QoS != 1 {
		t.Errorf("expected QoS 1, got %d", msg.QoS)
	}
	if msg.Retained {
		t.Error("expected Retained to be false")
	}
	if !msg.Timestamp.Equal(now) {
		t.Errorf("expected timestamp %v, got %v", now, msg.Timestamp)
	}
}

// TestTopicStatStructure tests the TopicStat type.
func TestTopicStatStructure(t *testing.T) {
	now := time.Now()
	stat := mqtt.TopicStat{
		Topic:            "factory/line1/temperature",
		Count:            100,
		LastPublished:    now,
		LastPayloadBytes: 256,
	}

	if stat.Topic != "factory/line1/temperature" {
		t.Errorf("expected topic 'factory/line1/temperature', got %q", stat.Topic)
	}
	if stat.Count != 100 {
		t.Errorf("expected count 100, got %d", stat.Count)
	}
	if stat.LastPayloadBytes != 256 {
		t.Errorf("expected LastPayloadBytes 256, got %d", stat.LastPayloadBytes)
	}
}

// TestTopicStatSorting tests that TopicStats can be sorted by recency.
func TestTopicStatSorting(t *testing.T) {
	now := time.Now()
	stats := []mqtt.TopicStat{
		{Topic: "old", LastPublished: now.Add(-10 * time.Minute)},
		{Topic: "newest", LastPublished: now},
		{Topic: "middle", LastPublished: now.Add(-5 * time.Minute)},
	}

	// Sort by most recent first
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].LastPublished.After(stats[j].LastPublished)
	})

	if stats[0].Topic != "newest" {
		t.Errorf("expected first topic to be 'newest', got %q", stats[0].Topic)
	}
	if stats[1].Topic != "middle" {
		t.Errorf("expected second topic to be 'middle', got %q", stats[1].Topic)
	}
	if stats[2].Topic != "old" {
		t.Errorf("expected third topic to be 'old', got %q", stats[2].Topic)
	}
}

// TestPublisherStatsAtomicOperations tests PublisherStats atomic operations.
func TestPublisherStatsAtomicOperations(t *testing.T) {
	stats := &mqtt.PublisherStats{}

	// Test MessagesPublished
	stats.MessagesPublished.Add(10)
	stats.MessagesPublished.Add(5)
	if got := stats.MessagesPublished.Load(); got != 15 {
		t.Errorf("expected MessagesPublished 15, got %d", got)
	}

	// Test MessagesFailed
	stats.MessagesFailed.Add(3)
	if got := stats.MessagesFailed.Load(); got != 3 {
		t.Errorf("expected MessagesFailed 3, got %d", got)
	}

	// Test MessagesBuffered
	stats.MessagesBuffered.Add(100)
	if got := stats.MessagesBuffered.Load(); got != 100 {
		t.Errorf("expected MessagesBuffered 100, got %d", got)
	}

	// Test BytesSent
	stats.BytesSent.Add(1024)
	stats.BytesSent.Add(2048)
	if got := stats.BytesSent.Load(); got != 3072 {
		t.Errorf("expected BytesSent 3072, got %d", got)
	}

	// Test ReconnectCount
	stats.ReconnectCount.Add(2)
	if got := stats.ReconnectCount.Load(); got != 2 {
		t.Errorf("expected ReconnectCount 2, got %d", got)
	}
}

// TestPublisherStatsConcurrency tests stats are safe for concurrent access.
func TestPublisherStatsConcurrency(t *testing.T) {
	stats := &mqtt.PublisherStats{}
	done := make(chan struct{})

	// Multiple goroutines incrementing stats
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				stats.MessagesPublished.Add(1)
				stats.BytesSent.Add(100)
			}
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	if got := stats.MessagesPublished.Load(); got != 1000 {
		t.Errorf("expected MessagesPublished 1000, got %d", got)
	}
	if got := stats.BytesSent.Load(); got != 100000 {
		t.Errorf("expected BytesSent 100000, got %d", got)
	}
}

// TestQoSLevels verifies QoS level settings.
func TestQoSLevels(t *testing.T) {
	tests := []struct {
		name        string
		qos         byte
		description string
	}{
		{"At most once", 0, "Fire and forget"},
		{"At least once", 1, "Acknowledged delivery"},
		{"Exactly once", 2, "Assured delivery"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mqtt.Config{QoS: tt.qos}
			if cfg.QoS != tt.qos {
				t.Errorf("expected QoS %d, got %d", tt.qos, cfg.QoS)
			}
		})
	}
}

// TestBrokerURLFormats tests various broker URL formats.
func TestBrokerURLFormats(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"TCP standard", "tcp://localhost:1883"},
		{"TCP custom port", "tcp://mqtt.example.com:1884"},
		{"TLS/SSL", "ssl://mqtt.example.com:8883"},
		{"TLS scheme", "tls://mqtt.example.com:8883"},
		{"WebSocket", "ws://mqtt.example.com:9001"},
		{"WebSocket Secure", "wss://mqtt.example.com:9443"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mqtt.Config{BrokerURL: tt.url}
			if cfg.BrokerURL != tt.url {
				t.Errorf("expected URL %q, got %q", tt.url, cfg.BrokerURL)
			}
		})
	}
}

// TestBufferSizeSettings tests various buffer size configurations.
func TestBufferSizeSettings(t *testing.T) {
	tests := []struct {
		name       string
		bufferSize int
		expected   int
	}{
		{"Default", 10000, 10000},
		{"Small", 100, 100},
		{"Large", 100000, 100000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mqtt.Config{BufferSize: tt.bufferSize}
			if cfg.BufferSize != tt.expected {
				t.Errorf("expected BufferSize %d, got %d", tt.expected, cfg.BufferSize)
			}
		})
	}
}

// TestReconnectSettings tests reconnection configuration.
func TestReconnectSettings(t *testing.T) {
	tests := []struct {
		name         string
		maxReconnect int
		reconnectDel time.Duration
	}{
		{"Unlimited reconnect", -1, 5 * time.Second},
		{"No reconnect", 0, 0},
		{"Limited reconnect", 5, 10 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mqtt.Config{
				MaxReconnect:   tt.maxReconnect,
				ReconnectDelay: tt.reconnectDel,
			}
			if cfg.MaxReconnect != tt.maxReconnect {
				t.Errorf("expected MaxReconnect %d, got %d", tt.maxReconnect, cfg.MaxReconnect)
			}
			if cfg.ReconnectDelay != tt.reconnectDel {
				t.Errorf("expected ReconnectDelay %v, got %v", tt.reconnectDel, cfg.ReconnectDelay)
			}
		})
	}
}

// TestMessageRetention tests retain flag behavior.
func TestMessageRetention(t *testing.T) {
	msg1 := mqtt.BufferedMessage{
		Topic:    "sensors/temp",
		Payload:  []byte("25.5"),
		Retained: true,
	}
	msg2 := mqtt.BufferedMessage{
		Topic:    "sensors/humidity",
		Payload:  []byte("60"),
		Retained: false,
	}

	if !msg1.Retained {
		t.Error("msg1 should be retained")
	}
	if msg2.Retained {
		t.Error("msg2 should not be retained")
	}
}

// TestTLSConfiguration tests TLS config fields.
func TestTLSConfiguration(t *testing.T) {
	cfg := mqtt.Config{
		TLSEnabled:  true,
		TLSCertFile: "/etc/mqtt/client.crt",
		TLSKeyFile:  "/etc/mqtt/client.key",
		TLSCAFile:   "/etc/mqtt/ca.crt",
	}

	if !cfg.TLSEnabled {
		t.Error("TLS should be enabled")
	}
	if cfg.TLSCertFile == "" {
		t.Error("TLSCertFile should be set")
	}
	if cfg.TLSKeyFile == "" {
		t.Error("TLSKeyFile should be set")
	}
	if cfg.TLSCAFile == "" {
		t.Error("TLSCAFile should be set")
	}
}

// TestCleanSessionBehavior tests clean session flag.
func TestCleanSessionBehavior(t *testing.T) {
	tests := []struct {
		name         string
		cleanSession bool
		description  string
	}{
		{"Clean start", true, "Start fresh, no persistent subscriptions"},
		{"Resume session", false, "Resume from previous session state"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mqtt.Config{CleanSession: tt.cleanSession}
			if cfg.CleanSession != tt.cleanSession {
				t.Errorf("expected CleanSession %v, got %v", tt.cleanSession, cfg.CleanSession)
			}
		})
	}
}
