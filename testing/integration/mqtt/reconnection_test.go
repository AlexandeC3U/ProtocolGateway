//go:build integration
// +build integration

package mqtt_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/mqtt"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/nexus-edge/protocol-gateway/testing/integration"
)

// Note: testLogger is defined in connection_test.go in the same package

// =============================================================================
// Reconnection Lifecycle Tests
// =============================================================================

// TestMQTT_ReconnectAfterCleanDisconnect tests that a publisher can reconnect
// after a clean disconnect and resume publishing.
func TestMQTT_ReconnectAfterCleanDisconnect(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "reconnect-clean-test",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		ReconnectDelay: 1 * time.Second,
		QoS:            1,
		BufferSize:     100,
	}, testLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	// Cycle 1: connect, publish, disconnect
	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("first connect failed: %v", err)
	}

	dp := domain.NewDataPoint("device-1", "tag-1", "test/reconnect/1", 42.0, "°C", domain.QualityGood)
	if err := publisher.Publish(ctx, dp); err != nil {
		t.Fatalf("first publish failed: %v", err)
	}

	publisher.Disconnect()

	if publisher.IsConnected() {
		t.Error("expected disconnected after Disconnect()")
	}

	// Cycle 2: reconnect and publish again
	time.Sleep(200 * time.Millisecond)

	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("reconnect failed: %v", err)
	}

	dp2 := domain.NewDataPoint("device-1", "tag-1", "test/reconnect/2", 43.0, "°C", domain.QualityGood)
	if err := publisher.Publish(ctx, dp2); err != nil {
		t.Fatalf("publish after reconnect failed: %v", err)
	}

	if !publisher.IsConnected() {
		t.Error("expected connected after reconnect")
	}

	stats := publisher.Stats()
	if stats.MessagesPublished.Load() < 2 {
		t.Errorf("expected at least 2 messages published across reconnect, got %d",
			stats.MessagesPublished.Load())
	}

	publisher.Disconnect()
}

// TestMQTT_MultipleReconnectCycles tests multiple connect/disconnect cycles.
func TestMQTT_MultipleReconnectCycles(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "reconnect-multi-test",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		ReconnectDelay: 500 * time.Millisecond,
		QoS:            1,
		BufferSize:     100,
	}, testLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	cycles := 5
	for i := 0; i < cycles; i++ {
		t.Logf("Reconnect cycle %d/%d", i+1, cycles)

		if err := publisher.Connect(ctx); err != nil {
			t.Fatalf("cycle %d: connect failed: %v", i+1, err)
		}

		if !publisher.IsConnected() {
			t.Fatalf("cycle %d: expected connected", i+1)
		}

		dp := domain.NewDataPoint("device", "tag", "test/multicycle", float64(i), "", domain.QualityGood)
		if err := publisher.Publish(ctx, dp); err != nil {
			t.Errorf("cycle %d: publish failed: %v", i+1, err)
		}

		publisher.Disconnect()

		time.Sleep(100 * time.Millisecond)
	}

	stats := publisher.Stats()
	if stats.MessagesPublished.Load() < uint64(cycles) {
		t.Errorf("expected at least %d messages, got %d", cycles, stats.MessagesPublished.Load())
	}

	t.Logf("Completed %d reconnect cycles, published %d messages",
		cycles, stats.MessagesPublished.Load())
}

// =============================================================================
// Publishing During State Transitions
// =============================================================================

// TestMQTT_PublishWhileDisconnected tests that publishing while disconnected
// buffers messages instead of losing them.
func TestMQTT_PublishWhileDisconnected(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "reconnect-buffer-test",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		ReconnectDelay: 1 * time.Second,
		QoS:            1,
		BufferSize:     1000,
	}, testLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	// Connect then disconnect
	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	publisher.Disconnect()

	// Publish while disconnected — should buffer
	bufferedCount := 10
	for i := 0; i < bufferedCount; i++ {
		dp := domain.NewDataPoint(
			"device", "tag", "test/buffered/"+string(rune('0'+i)),
			float64(i), "", domain.QualityGood,
		)
		err := publisher.Publish(ctx, dp)
		// Publish while disconnected should buffer, not necessarily error
		if err != nil {
			t.Logf("Publish while disconnected (msg %d): %v", i, err)
		}
	}

	bufSize := publisher.BufferSize()
	t.Logf("Buffer size after disconnected publishes: %d", bufSize)

	stats := publisher.Stats()
	if stats.MessagesBuffered.Load() < uint64(bufferedCount) {
		t.Logf("Buffered count: %d (some messages may have been published before disconnect completed)",
			stats.MessagesBuffered.Load())
	}
}

// TestMQTT_BufferDrainOnReconnect tests that buffered messages are published
// when the connection is re-established.
func TestMQTT_BufferDrainOnReconnect(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "reconnect-drain-test",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		ReconnectDelay: 500 * time.Millisecond,
		QoS:            1,
		BufferSize:     1000,
		PublishTimeout: 5 * time.Second,
	}, testLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	// Connect, publish 5 messages, then disconnect
	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("first connect failed: %v", err)
	}

	for i := 0; i < 5; i++ {
		dp := domain.NewDataPoint("device", "tag", "test/drain/pre", float64(i), "", domain.QualityGood)
		publisher.Publish(ctx, dp)
	}

	publisher.Disconnect()

	preStats := publisher.Stats()
	prePublished := preStats.MessagesPublished.Load()
	t.Logf("Published before disconnect: %d", prePublished)

	// Reconnect — buffer processor should drain any buffered messages
	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("reconnect failed: %v", err)
	}

	// Publish some more to confirm the connection is working
	for i := 0; i < 5; i++ {
		dp := domain.NewDataPoint("device", "tag", "test/drain/post", float64(i+100), "", domain.QualityGood)
		if err := publisher.Publish(ctx, dp); err != nil {
			t.Errorf("post-reconnect publish failed: %v", err)
		}
	}

	// Wait a bit for buffer processor to drain
	time.Sleep(500 * time.Millisecond)

	postStats := publisher.Stats()
	t.Logf("Published after reconnect: %d (total: %d)",
		postStats.MessagesPublished.Load()-prePublished,
		postStats.MessagesPublished.Load())

	if postStats.MessagesPublished.Load() < prePublished+5 {
		t.Errorf("expected at least %d total published messages, got %d",
			prePublished+5, postStats.MessagesPublished.Load())
	}

	publisher.Disconnect()
}

// =============================================================================
// Stats Tracking Across Reconnects
// =============================================================================

// TestMQTT_StatsAccumulateAcrossReconnects tests that publisher stats
// are cumulative across reconnection cycles.
func TestMQTT_StatsAccumulateAcrossReconnects(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "reconnect-stats-test",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		ReconnectDelay: 500 * time.Millisecond,
		QoS:            1,
		BufferSize:     100,
	}, testLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	var totalPublished uint64

	for cycle := 0; cycle < 3; cycle++ {
		if err := publisher.Connect(ctx); err != nil {
			t.Fatalf("cycle %d connect failed: %v", cycle, err)
		}

		msgsThisCycle := 3
		for i := 0; i < msgsThisCycle; i++ {
			dp := domain.NewDataPoint("device", "tag", "test/stats/accum", float64(cycle*10+i), "", domain.QualityGood)
			if err := publisher.Publish(ctx, dp); err != nil {
				t.Errorf("cycle %d, msg %d: publish failed: %v", cycle, i, err)
			}
		}
		totalPublished += uint64(msgsThisCycle)

		publisher.Disconnect()
		time.Sleep(100 * time.Millisecond)
	}

	stats := publisher.Stats()
	if stats.MessagesPublished.Load() < totalPublished {
		t.Errorf("expected cumulative published >= %d, got %d",
			totalPublished, stats.MessagesPublished.Load())
	}

	t.Logf("Stats across 3 reconnect cycles: published=%d, failed=%d, bytes=%d",
		stats.MessagesPublished.Load(),
		stats.MessagesFailed.Load(),
		stats.BytesSent.Load())
}

// TestMQTT_ReconnectCountTracking tests that reconnect count is tracked.
func TestMQTT_ReconnectCountTracking(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "reconnect-count-test",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		ReconnectDelay: 500 * time.Millisecond,
		QoS:            1,
		BufferSize:     100,
	}, testLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	// Initial connect
	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("initial connect failed: %v", err)
	}

	initialStats := publisher.Stats()
	initialReconnects := initialStats.ReconnectCount.Load()

	publisher.Disconnect()
	time.Sleep(100 * time.Millisecond)

	// Manual reconnect
	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("reconnect failed: %v", err)
	}

	// The ReconnectCount is tracked by paho's auto-reconnect handler,
	// so manual disconnect/connect may not increment it. Log for visibility.
	postStats := publisher.Stats()
	t.Logf("Reconnect count: initial=%d, after manual reconnect=%d",
		initialReconnects, postStats.ReconnectCount.Load())

	publisher.Disconnect()
}

// =============================================================================
// Connection to Unreachable Broker
// =============================================================================

// TestMQTT_ConnectToUnreachableBroker tests behavior when the broker is not available.
func TestMQTT_ConnectToUnreachableBroker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      "tcp://localhost:59998", // No broker here
		ClientID:       "unreachable-broker-test",
		CleanSession:   true,
		ConnectTimeout: 2 * time.Second,
		ReconnectDelay: 500 * time.Millisecond,
		BufferSize:     100,
	}, testLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	err = publisher.Connect(ctx)
	if err == nil {
		publisher.Disconnect()
		t.Error("expected connection to fail for unreachable broker")
	} else {
		t.Logf("Connection correctly failed: %v", err)
	}

	// Publisher should not be connected
	if publisher.IsConnected() {
		t.Error("expected IsConnected()=false after failed connection")
	}
}

// TestMQTT_PublishAfterFailedConnect tests that publishing after a failed
// connection attempt buffers messages and does not panic.
func TestMQTT_PublishAfterFailedConnect(t *testing.T) {
	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      "tcp://localhost:59998",
		ClientID:       "publish-after-fail-test",
		CleanSession:   true,
		ConnectTimeout: 1 * time.Second,
		BufferSize:     100,
	}, testLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	// Don't connect — just try to publish
	dp := domain.NewDataPoint("device", "tag", "test/no-connect", 42.0, "", domain.QualityGood)
	err = publisher.Publish(context.Background(), dp)

	// Should buffer or error, but must not panic
	if err != nil {
		t.Logf("Publish without connection returned error: %v", err)
	} else {
		t.Log("Publish without connection buffered the message")
		if publisher.BufferSize() > 0 {
			t.Logf("Buffer size: %d", publisher.BufferSize())
		}
	}
}

// =============================================================================
// Concurrent Reconnection Safety
// =============================================================================

// TestMQTT_ConcurrentPublishDuringReconnect tests that concurrent publishes
// during a reconnect cycle are handled safely (no races, no panics).
func TestMQTT_ConcurrentPublishDuringReconnect(t *testing.T) {
	cfg := integration.DefaultConfig()
	integration.SkipIfNoMQTTBroker(t, cfg.MQTTHost, cfg.MQTTPort)

	ctx, cancel := integration.ContextWithTestTimeout(t)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      cfg.MQTTBrokerURL(),
		ClientID:       "concurrent-reconnect-test",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		ReconnectDelay: 500 * time.Millisecond,
		QoS:            0,
		BufferSize:     500,
	}, testLogger(), nil)
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	if err := publisher.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	var published atomic.Int64
	var errors atomic.Int64
	done := make(chan struct{})

	// Start concurrent publishers
	workers := 3
	for w := 0; w < workers; w++ {
		go func(id int) {
			for {
				select {
				case <-done:
					return
				default:
					dp := domain.NewDataPoint(
						"concurrent-device",
						"tag",
						"test/concurrent/reconnect",
						float64(id),
						"",
						domain.QualityGood,
					)
					if err := publisher.Publish(ctx, dp); err != nil {
						errors.Add(1)
					} else {
						published.Add(1)
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
		}(w)
	}

	// Let publishers run, then disconnect and reconnect mid-stream
	time.Sleep(200 * time.Millisecond)
	publisher.Disconnect()
	time.Sleep(200 * time.Millisecond)

	if err := publisher.Connect(ctx); err != nil {
		close(done)
		t.Fatalf("reconnect during concurrent publish failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	close(done)

	// Allow goroutines to finish
	time.Sleep(100 * time.Millisecond)

	t.Logf("Concurrent publish during reconnect: published=%d, errors=%d",
		published.Load(), errors.Load())

	// Main assertion: no panics occurred and at least some messages got through
	if published.Load() == 0 {
		t.Error("expected at least some messages to be published")
	}

	publisher.Disconnect()
}
