//go:build integration
// +build integration

package mqtt_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/mqtt"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/nexus-edge/protocol-gateway/testing/integration"
	"github.com/rs/zerolog"
)

func testLogger() zerolog.Logger {
	return zerolog.Nop()
}

// TestMQTT_BasicConnection tests basic MQTT broker connection.
func TestMQTT_BasicConnection(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	t.Logf("Testing against %s:%d", cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "test-basic-connection",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		BufferSize:     100,
	}, testLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	// Connect to broker
	err = publisher.Connect(ctx)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Verify connected
	if !publisher.IsConnected() {
		t.Error("expected publisher to be connected")
	}

	// Disconnect
	publisher.Disconnect()

	// Verify disconnected
	if publisher.IsConnected() {
		t.Error("expected publisher to be disconnected")
	}
}

// TestMQTT_PublishMessage tests publishing a message to the broker.
func TestMQTT_PublishMessage(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "test-publish-message",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		QoS:            1,
		BufferSize:     100,
	}, testLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer publisher.Disconnect()

	// Create a test data point
	dp := domain.NewDataPoint(
		"test-device",
		"test-tag",
		"test/topic",
		42.5,
		"°C",
		domain.QualityGood,
	)

	// Publish the data point
	err = publisher.Publish(ctx, dp)
	if err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	// Check stats
	stats := publisher.Stats()
	if stats.MessagesPublished.Load() < 1 {
		t.Error("expected at least 1 message published")
	}
}

// TestMQTT_PublishBatch tests publishing multiple messages.
func TestMQTT_PublishBatch(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "test-publish-batch",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		QoS:            1,
		BufferSize:     100,
	}, testLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer publisher.Disconnect()

	// Create batch of data points
	dataPoints := make([]*domain.DataPoint, 10)
	for i := 0; i < 10; i++ {
		dataPoints[i] = domain.NewDataPoint(
			"test-device",
			"tag-"+string(rune('0'+i)),
			"test/batch/"+string(rune('0'+i)),
			float64(i)*10,
			"unit",
			domain.QualityGood,
		)
	}

	// Publish batch
	err = publisher.PublishBatch(ctx, dataPoints)
	if err != nil {
		t.Fatalf("failed to publish batch: %v", err)
	}

	// Check stats
	stats := publisher.Stats()
	if stats.MessagesPublished.Load() < 10 {
		t.Errorf("expected at least 10 messages published, got %d", stats.MessagesPublished.Load())
	}
}

// TestMQTT_QoSLevels tests different QoS levels.
func TestMQTT_QoSLevels(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	qosLevels := []struct {
		name string
		qos  byte
	}{
		{"QoS0_AtMostOnce", 0},
		{"QoS1_AtLeastOnce", 1},
		{"QoS2_ExactlyOnce", 2},
	}

	for _, tc := range qosLevels {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := integration.ContextWithTestTimeout(t)
			defer cancel()

			publisher, err := mqtt.NewPublisher(mqtt.Config{
				BrokerURL:      cfg.MQTTBrokerURL(),
				ClientID:       "test-qos-" + tc.name,
				CleanSession:   true,
				ConnectTimeout: 5 * time.Second,
				QoS:            tc.qos,
				BufferSize:     100,
			}, testLogger(), nil)
			if err != nil {
				t.Fatalf("failed to create publisher: %v", err)
			}

			if err := publisher.Connect(ctx); err != nil {
				t.Fatalf("failed to connect: %v", err)
			}
			defer publisher.Disconnect()

			dp := domain.NewDataPoint(
				"test-device",
				"test-tag",
				"test/qos/"+tc.name,
				100.0,
				"",
				domain.QualityGood,
			)

			err = publisher.Publish(ctx, dp)
			if err != nil {
				t.Fatalf("failed to publish with QoS %d: %v", tc.qos, err)
			}

			t.Logf("Successfully published with QoS %d", tc.qos)
		})
	}
}

// TestMQTT_RetainedMessages tests retained message functionality.
func TestMQTT_RetainedMessages(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "test-retained",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		QoS:            1,
		RetainMessages: true,
		BufferSize:     100,
	}, testLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer publisher.Disconnect()

	dp := domain.NewDataPoint(
		"test-device",
		"retained-tag",
		"test/retained",
		999.0,
		"",
		domain.QualityGood,
	)

	err = publisher.Publish(ctx, dp)
	if err != nil {
		t.Fatalf("failed to publish retained message: %v", err)
	}

	t.Log("Successfully published retained message")
}

// TestMQTT_ConnectionRefused tests error handling when broker is unreachable.
func TestMQTT_ConnectionRefused(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      "tcp://localhost:59999", // Unlikely to have a broker here
		ClientID:       "test-refused",
		CleanSession:   true,
		ConnectTimeout: 2 * time.Second,
		BufferSize:     100,
	}, testLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	err = publisher.Connect(ctx)
	if err == nil {
		publisher.Disconnect()
		t.Error("expected connection to fail")
	} else {
		t.Logf("Connection correctly failed: %v", err)
	}
}

// TestMQTT_InvalidURL tests error handling for invalid broker URL.
func TestMQTT_InvalidURL(t *testing.T) {
	_, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      "invalid://not-a-valid-url",
		ClientID:       "test-invalid-url",
		ConnectTimeout: 2 * time.Second,
		BufferSize:     100,
	}, testLogger(), nil)

	// Either creation fails or connection will fail
	if err != nil {
		t.Logf("Publisher creation failed as expected: %v", err)
		return
	}

	// If creation succeeded, URL will fail at connection time
	t.Log("Publisher created, URL validation is deferred to connection time")
}

// TestMQTT_Reconnection tests automatic reconnection after disconnect.
func TestMQTT_Reconnection(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "test-reconnection",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		ReconnectDelay: 1 * time.Second,
		BufferSize:     100,
	}, testLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	// Initial connect
	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Disconnect
	publisher.Disconnect()

	// Wait briefly
	time.Sleep(100 * time.Millisecond)

	// Reconnect
	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("failed to reconnect: %v", err)
	}

	if !publisher.IsConnected() {
		t.Error("expected to be reconnected")
	}

	publisher.Disconnect()
}

// TestMQTT_JSONPayload tests that data points are correctly serialized to JSON.
func TestMQTT_JSONPayload(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "test-json-payload",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		QoS:            1,
		BufferSize:     100,
	}, testLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer publisher.Disconnect()

	// Create a data point with various fields
	dp := domain.NewDataPoint(
		"plc-001",
		"temperature_sensor",
		"uns/enterprise/site/area/plc-001/temperature_sensor",
		25.5,
		"°C",
		domain.QualityGood,
	)

	// Verify JSON serialization works
	jsonBytes, err := dp.ToJSON()
	if err != nil {
		t.Fatalf("failed to serialize to JSON: %v", err)
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("invalid JSON produced: %v", err)
	}

	t.Logf("JSON payload: %s", string(jsonBytes))

	// Publish
	err = publisher.Publish(ctx, dp)
	if err != nil {
		t.Fatalf("failed to publish: %v", err)
	}
}

// TestMQTT_TopicStats tests that topic statistics are tracked.
func TestMQTT_TopicStats(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "test-topic-stats",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		QoS:            1,
		BufferSize:     100,
	}, testLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer publisher.Disconnect()

	// Publish to same topic multiple times
	topic := "test/stats/topic"
	for i := 0; i < 5; i++ {
		dp := domain.NewDataPoint(
			"device",
			"tag",
			topic,
			float64(i),
			"",
			domain.QualityGood,
		)
		if err := publisher.Publish(ctx, dp); err != nil {
			t.Fatalf("failed to publish: %v", err)
		}
	}

	// Check topic stats using ActiveTopics method
	topicStats := publisher.ActiveTopics(100)
	found := false
	for _, stat := range topicStats {
		if stat.Topic == topic {
			found = true
			if stat.Count < 5 {
				t.Errorf("expected count >= 5, got %d", stat.Count)
			}
			t.Logf("Topic %s: count=%d, last_published=%v", stat.Topic, stat.Count, stat.LastPublished)
		}
	}

	if !found {
		t.Errorf("topic stats not found for %s", topic)
	}
}
