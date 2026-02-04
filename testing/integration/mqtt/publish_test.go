//go:build integration
// +build integration

// Package mqtt_test provides additional integration tests for MQTT publishing.
package mqtt_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/mqtt"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/nexus-edge/protocol-gateway/testing/integration"
	"github.com/rs/zerolog"
)

func publishTestLogger() zerolog.Logger {
	return zerolog.Nop()
}

// =============================================================================
// Publish Operation Tests
// =============================================================================

// TestMQTT_PublishSingleMessage tests publishing a single message.
func TestMQTT_PublishSingleMessage(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "publish-single-test",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		QoS:            1,
		BufferSize:     100,
	}, publishTestLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer publisher.Disconnect()

	dp := domain.NewDataPoint(
		"test-device",
		"temp-sensor",
		"plant/area1/temperature",
		23.5,
		"°C",
		domain.QualityGood,
	)

	err = publisher.Publish(ctx, dp)
	if err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	stats := publisher.Stats()
	if stats.MessagesPublished.Load() != 1 {
		t.Errorf("expected 1 message published, got %d", stats.MessagesPublished.Load())
	}
}

// TestMQTT_PublishMultipleTopics tests publishing to different topics.
func TestMQTT_PublishMultipleTopics(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "publish-multi-topic-test",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		QoS:            1,
		BufferSize:     100,
	}, publishTestLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer publisher.Disconnect()

	topics := []string{
		"plant/line1/temperature",
		"plant/line1/pressure",
		"plant/line2/temperature",
		"plant/line2/pressure",
		"plant/utilities/power",
	}

	for i, topic := range topics {
		dp := domain.NewDataPoint(
			"device-"+topic,
			"tag-"+topic,
			topic,
			float64(i*10+100),
			"units",
			domain.QualityGood,
		)

		err = publisher.Publish(ctx, dp)
		if err != nil {
			t.Errorf("failed to publish to %s: %v", topic, err)
		}
	}

	stats := publisher.Stats()
	if stats.MessagesPublished.Load() != uint64(len(topics)) {
		t.Errorf("expected %d messages published, got %d", len(topics), stats.MessagesPublished.Load())
	}
}

// TestMQTT_PublishDifferentValueTypes tests publishing different data types.
func TestMQTT_PublishDifferentValueTypes(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "publish-types-test",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		QoS:            1,
		BufferSize:     100,
	}, publishTestLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer publisher.Disconnect()

	testCases := []struct {
		name  string
		value interface{}
	}{
		{"Int", int64(42)},
		{"Float", 3.14159},
		{"Bool", true},
		{"String", "status_ok"},
		{"NegativeInt", int64(-100)},
		{"LargeFloat", 1234567.89},
		{"Zero", 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dp := domain.NewDataPoint(
				"type-test-device",
				"tag-"+tc.name,
				"test/types/"+tc.name,
				tc.value,
				"",
				domain.QualityGood,
			)

			err := publisher.Publish(ctx, dp)
			if err != nil {
				t.Errorf("failed to publish %s: %v", tc.name, err)
			}
		})
	}
}

// =============================================================================
// High Volume Publishing Tests
// =============================================================================

// TestMQTT_PublishHighVolume tests publishing many messages quickly.
func TestMQTT_PublishHighVolume(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "publish-high-volume-test",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		QoS:            0, // QoS 0 for high throughput
		BufferSize:     1000,
	}, publishTestLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer publisher.Disconnect()

	messageCount := 100
	start := time.Now()

	for i := 0; i < messageCount; i++ {
		dp := domain.NewDataPoint(
			"high-volume-device",
			"tag-"+string(rune('A'+i%26)),
			"test/volume/"+string(rune('0'+i%10)),
			float64(i),
			"",
			domain.QualityGood,
		)

		err := publisher.Publish(ctx, dp)
		if err != nil {
			t.Errorf("failed to publish message %d: %v", i, err)
		}
	}

	elapsed := time.Since(start)
	stats := publisher.Stats()

	t.Logf("Published %d messages in %v (%.2f msg/sec)",
		stats.MessagesPublished.Load(),
		elapsed,
		float64(stats.MessagesPublished.Load())/elapsed.Seconds())

	if stats.MessagesPublished.Load() < uint64(messageCount*90/100) {
		t.Errorf("expected at least 90%% of messages published, got %d/%d",
			stats.MessagesPublished.Load(), messageCount)
	}
}

// TestMQTT_ConcurrentPublish tests concurrent publishing from multiple goroutines.
func TestMQTT_ConcurrentPublish(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "publish-concurrent-test",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		QoS:            1,
		BufferSize:     500,
	}, publishTestLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer publisher.Disconnect()

	workers := 5
	messagesPerWorker := 20
	var wg sync.WaitGroup
	var errorCount atomic.Int32

	start := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < messagesPerWorker; i++ {
				dp := domain.NewDataPoint(
					"concurrent-device",
					"worker-tag",
					"test/concurrent/worker"+string(rune('0'+workerID)),
					float64(workerID*100+i),
					"",
					domain.QualityGood,
				)

				if err := publisher.Publish(ctx, dp); err != nil {
					errorCount.Add(1)
				}
			}
		}(w)
	}

	wg.Wait()
	elapsed := time.Since(start)

	stats := publisher.Stats()
	expectedTotal := workers * messagesPerWorker

	t.Logf("Concurrent publish: %d workers × %d messages = %d total",
		workers, messagesPerWorker, expectedTotal)
	t.Logf("Published: %d, Failed: %d, Time: %v",
		stats.MessagesPublished.Load(), errorCount.Load(), elapsed)

	if stats.MessagesPublished.Load() < uint64(expectedTotal*90/100) {
		t.Errorf("expected at least 90%% success rate, got %d/%d",
			stats.MessagesPublished.Load(), expectedTotal)
	}
}

// =============================================================================
// Quality and Reliability Tests
// =============================================================================

// TestMQTT_PublishWithBadQuality tests publishing data with different quality flags.
func TestMQTT_PublishWithBadQuality(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "publish-quality-test",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		QoS:            1,
		BufferSize:     100,
	}, publishTestLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer publisher.Disconnect()

	qualities := []domain.Quality{
		domain.QualityGood,
		domain.QualityBad,
		domain.QualityUncertain,
	}

	for _, q := range qualities {
		t.Run(string(q), func(t *testing.T) {
			dp := domain.NewDataPoint(
				"quality-device",
				"quality-tag",
				"test/quality/"+string(q),
				42.0,
				"",
				q,
			)

			err := publisher.Publish(ctx, dp)
			if err != nil {
				t.Errorf("failed to publish with quality %s: %v", q, err)
			}
		})
	}
}

// TestMQTT_PublishLargePayload tests publishing messages with larger payloads.
func TestMQTT_PublishLargePayload(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "publish-large-payload-test",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		QoS:            1,
		BufferSize:     100,
	}, publishTestLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer publisher.Disconnect()

	// Create a large string value
	largeString := ""
	for i := 0; i < 100; i++ {
		largeString += "This is a test message segment. "
	}

	dp := domain.NewDataPoint(
		"large-payload-device",
		"large-tag",
		"test/large/payload",
		largeString,
		"",
		domain.QualityGood,
	)

	err = publisher.Publish(ctx, dp)
	if err != nil {
		t.Fatalf("failed to publish large payload: %v", err)
	}

	t.Logf("Successfully published payload of ~%d bytes", len(largeString))
}

// TestMQTT_ActiveTopics tests the ActiveTopics tracking functionality.
func TestMQTT_ActiveTopics(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "active-topics-test",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		QoS:            1,
		BufferSize:     100,
	}, publishTestLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer publisher.Disconnect()

	// Publish to multiple unique topics
	topics := []string{
		"plant/sensor1",
		"plant/sensor2",
		"plant/sensor3",
	}

	for _, topic := range topics {
		dp := domain.NewDataPoint("device", "tag", topic, 42.0, "", domain.QualityGood)
		if err := publisher.Publish(ctx, dp); err != nil {
			t.Errorf("failed to publish to %s: %v", topic, err)
		}
	}

	// Check active topics
	activeTopics := publisher.ActiveTopics(10)
	if len(activeTopics) != len(topics) {
		t.Errorf("expected %d active topics, got %d", len(topics), len(activeTopics))
	}

	for _, stat := range activeTopics {
		t.Logf("Active topic: %s, Count: %d, LastPublished: %v",
			stat.Topic, stat.Count, stat.LastPublished)
	}
}
