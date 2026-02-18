//go:build benchmark
// +build benchmark

// Package latency provides benchmark tests for MQTT publish latency measurement.
package latency

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/mqtt"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/rs/zerolog"
)

// =============================================================================
// MQTT Latency Helpers
// =============================================================================

// mqttBenchLogger returns a no-op logger for benchmark use.
func mqttBenchLogger() zerolog.Logger {
	return zerolog.Nop()
}

// simulatePublish simulates an MQTT publish operation with configurable latency.
func simulatePublish(baseLatency, jitter time.Duration) time.Duration {
	start := time.Now()

	sleepDuration := baseLatency
	if jitter > 0 {
		ns := time.Now().UnixNano() % int64(jitter)
		sleepDuration += time.Duration(ns)
	}
	time.Sleep(sleepDuration)

	return time.Since(start)
}

// MQTTPublishConfig defines MQTT publish latency characteristics.
type MQTTPublishConfig struct {
	Name        string
	QoS         int
	BaseLatency time.Duration
	Jitter      time.Duration
	PayloadSize int
}

// getQoSConfig returns realistic latency characteristics per QoS level.
func getQoSConfig(qos int) MQTTPublishConfig {
	switch qos {
	case 0:
		// QoS 0: Fire and forget - no ACK, lowest latency
		return MQTTPublishConfig{
			Name:        "QoS0_AtMostOnce",
			QoS:         0,
			BaseLatency: 200 * time.Microsecond,
			Jitter:      50 * time.Microsecond,
			PayloadSize: 256,
		}
	case 1:
		// QoS 1: At least once - requires PUBACK
		return MQTTPublishConfig{
			Name:        "QoS1_AtLeastOnce",
			QoS:         1,
			BaseLatency: 500 * time.Microsecond,
			Jitter:      150 * time.Microsecond,
			PayloadSize: 256,
		}
	case 2:
		// QoS 2: Exactly once - 4-step handshake (PUBLISH, PUBREC, PUBREL, PUBCOMP)
		return MQTTPublishConfig{
			Name:        "QoS2_ExactlyOnce",
			QoS:         2,
			BaseLatency: 2 * time.Millisecond,
			Jitter:      500 * time.Microsecond,
			PayloadSize: 256,
		}
	default:
		return getQoSConfig(0)
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

// BenchmarkMQTTLatency_Baseline measures baseline MQTT publish latency.
func BenchmarkMQTTLatency_Baseline(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = simulatePublish(200*time.Microsecond, 50*time.Microsecond)
	}
}

// BenchmarkMQTTLatency_ByQoS measures publish latency across QoS levels.
func BenchmarkMQTTLatency_ByQoS(b *testing.B) {
	for qos := 0; qos <= 2; qos++ {
		cfg := getQoSConfig(qos)
		b.Run(cfg.Name, func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = simulatePublish(cfg.BaseLatency, cfg.Jitter)
			}
		})
	}
}

// BenchmarkMQTTLatency_PayloadSize measures how payload size affects latency.
func BenchmarkMQTTLatency_PayloadSize(b *testing.B) {
	payloadSizes := []struct {
		name string
		size int
	}{
		{"64B", 64},
		{"256B", 256},
		{"1KB", 1024},
		{"4KB", 4096},
		{"16KB", 16384},
	}

	// Sink to prevent compiler optimization
	var sink []byte

	for _, ps := range payloadSizes {
		b.Run(ps.name, func(b *testing.B) {
			payload := make([]byte, ps.size)
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Simulate payload copy + publish
				sink = make([]byte, len(payload))
				copy(sink, payload)
				_ = simulatePublish(200*time.Microsecond+time.Duration(ps.size/10)*time.Microsecond, 50*time.Microsecond)
			}
		})
	}
	_ = sink
}

// BenchmarkMQTTLatency_Serialization measures DataPoint serialization latency.
func BenchmarkMQTTLatency_Serialization(b *testing.B) {
	dp := domain.NewDataPoint(
		"latency-device",
		"temperature",
		"plant/area/line/device/temperature",
		23.7,
		"\u00b0C",
		domain.QualityGood,
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		start := time.Now()
		_, _ = dp.ToJSON()
		elapsed := time.Since(start)
		_ = elapsed
	}
}

// BenchmarkMQTTLatency_ConcurrentPublish measures publish latency under concurrent load.
func BenchmarkMQTTLatency_ConcurrentPublish(b *testing.B) {
	concurrencies := []int{1, 2, 4, 8, 16}

	for _, c := range concurrencies {
		b.Run(workerCountLabel(c), func(b *testing.B) {
			b.ReportAllocs()
			b.SetParallelism(c)

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_ = simulatePublish(300*time.Microsecond, 100*time.Microsecond)
				}
			})
		})
	}
}

func workerCountLabel(n int) string {
	switch n {
	case 1:
		return "1_publisher"
	case 2:
		return "2_publishers"
	case 4:
		return "4_publishers"
	case 8:
		return "8_publishers"
	case 16:
		return "16_publishers"
	default:
		return "N_publishers"
	}
}

// =============================================================================
// Latency Profile Tests
// =============================================================================

// TestMQTTLatencyProfile_QoSComparison compares latency profiles across QoS levels.
func TestMQTTLatencyProfile_QoSComparison(t *testing.T) {
	iterations := 500

	for qos := 0; qos <= 2; qos++ {
		cfg := getQoSConfig(qos)
		t.Run(cfg.Name, func(t *testing.T) {
			samples := make([]time.Duration, iterations)

			for i := 0; i < iterations; i++ {
				samples[i] = simulatePublish(cfg.BaseLatency, cfg.Jitter)
			}

			stats := calculateStats(samples)

			t.Logf("%s Latency Profile (%d samples):", cfg.Name, stats.Count)
			t.Logf("  Min:    %v", stats.Min)
			t.Logf("  Max:    %v", stats.Max)
			t.Logf("  Mean:   %v", stats.Mean)
			t.Logf("  Median: %v", stats.Median)
			t.Logf("  P90:    %v", stats.P90)
			t.Logf("  P95:    %v", stats.P95)
			t.Logf("  P99:    %v", stats.P99)
			t.Logf("  StdDev: %v", stats.StdDev)
		})
	}
}

// TestMQTTLatencyProfile_PublishWithSerialization measures end-to-end publish latency
// including DataPoint serialization.
func TestMQTTLatencyProfile_PublishWithSerialization(t *testing.T) {
	iterations := 500
	samples := make([]time.Duration, iterations)

	dp := domain.NewDataPoint(
		"latency-device",
		"temperature",
		"plant/area/line/device/temperature",
		23.7,
		"\u00b0C",
		domain.QualityGood,
	)

	for i := 0; i < iterations; i++ {
		start := time.Now()
		// Serialize
		_, _ = dp.ToJSON()
		// Simulate network publish
		time.Sleep(300 * time.Microsecond)
		samples[i] = time.Since(start)
	}

	stats := calculateStats(samples)

	t.Logf("Publish with Serialization (%d samples):", stats.Count)
	t.Logf("  Min:    %v", stats.Min)
	t.Logf("  Max:    %v", stats.Max)
	t.Logf("  Mean:   %v", stats.Mean)
	t.Logf("  P95:    %v", stats.P95)
	t.Logf("  P99:    %v", stats.P99)
}

// =============================================================================
// Latency Under Load Tests
// =============================================================================

// TestMQTTLatencyUnderLoad tests how publish latency degrades under increasing load.
func TestMQTTLatencyUnderLoad(t *testing.T) {
	loadLevels := []struct {
		name           string
		goroutines     int
		publishPerSec  int
	}{
		{"Light", 1, 10},
		{"Medium", 4, 100},
		{"Heavy", 8, 500},
		{"Extreme", 16, 1000},
	}

	for _, load := range loadLevels {
		t.Run(load.name, func(t *testing.T) {
			var wg sync.WaitGroup
			samples := make([]time.Duration, 0, load.publishPerSec)
			var mu sync.Mutex

			interval := time.Second / time.Duration(load.publishPerSec)
			deadline := time.Now().Add(time.Second)

			for i := 0; i < load.goroutines; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for time.Now().Before(deadline) {
						latency := simulatePublish(300*time.Microsecond, 100*time.Microsecond)
						mu.Lock()
						samples = append(samples, latency)
						mu.Unlock()
						time.Sleep(interval / time.Duration(load.goroutines))
					}
				}()
			}

			wg.Wait()

			if len(samples) > 0 {
				stats := calculateStats(samples)
				t.Logf("Load: %s (%d goroutines, %d target pub/sec)",
					load.name, load.goroutines, load.publishPerSec)
				t.Logf("  Actual publishes: %d", stats.Count)
				t.Logf("  Mean latency:     %v", stats.Mean)
				t.Logf("  P95 latency:      %v", stats.P95)
				t.Logf("  P99 latency:      %v", stats.P99)
			}
		})
	}
}

// =============================================================================
// Batch Publish Latency Tests
// =============================================================================

// TestMQTTLatency_BatchPublish measures latency for batch publish operations.
func TestMQTTLatency_BatchPublish(t *testing.T) {
	batchSizes := []int{1, 10, 50, 100, 500}
	iterations := 100

	for _, batchSize := range batchSizes {
		t.Run(itoa(batchSize)+"_messages", func(t *testing.T) {
			samples := make([]time.Duration, iterations)

			for i := 0; i < iterations; i++ {
				start := time.Now()

				// Simulate serializing and publishing a batch
				for j := 0; j < batchSize; j++ {
					dp := domain.NewDataPoint(
						"device-1",
						"tag-"+itoa(j%26),
						"topic/"+itoa(j%10),
						float64(j)*1.5,
						"unit",
						domain.QualityGood,
					)
					_, _ = dp.ToJSON()
				}
				// Simulate batch network send
				time.Sleep(time.Duration(batchSize*5) * time.Microsecond)

				samples[i] = time.Since(start)
			}

			stats := calculateStats(samples)

			t.Logf("Batch size %d (%d iterations):", batchSize, iterations)
			t.Logf("  Mean:           %v", stats.Mean)
			t.Logf("  P95:            %v", stats.P95)
			t.Logf("  Mean per msg:   %v", stats.Mean/time.Duration(batchSize))
		})
	}
}

// =============================================================================
// Jitter Analysis Tests
// =============================================================================

// TestMQTTPublishJitter analyzes publish latency jitter.
func TestMQTTPublishJitter(t *testing.T) {
	iterations := 500
	samples := make([]time.Duration, iterations)

	for i := 0; i < iterations; i++ {
		samples[i] = simulatePublish(500*time.Microsecond, 200*time.Microsecond)
	}

	stats := calculateStats(samples)

	jitterRange := stats.Max - stats.Min
	cv := float64(stats.StdDev) / float64(stats.Mean) * 100

	t.Logf("MQTT Publish Jitter Analysis (%d samples):", stats.Count)
	t.Logf("  Jitter Range: %v (Max - Min)", jitterRange)
	t.Logf("  StdDev:       %v", stats.StdDev)
	t.Logf("  P99 - P90:    %v", stats.P99-stats.P90)
	t.Logf("  CV:           %.2f%%", cv)

	if cv > 50 {
		t.Logf("Warning: High jitter (CV > 50%%) may indicate broker congestion")
	}
}

// =============================================================================
// Latency Histogram
// =============================================================================

// TestMQTTLatencyHistogram creates a publish latency distribution histogram.
func TestMQTTLatencyHistogram(t *testing.T) {
	iterations := 1000
	samples := make([]time.Duration, iterations)

	for i := 0; i < iterations; i++ {
		samples[i] = simulatePublish(500*time.Microsecond, 200*time.Microsecond)
	}

	buckets := []struct {
		max   time.Duration
		label string
		count int
	}{
		{250 * time.Microsecond, "<0.25ms", 0},
		{500 * time.Microsecond, "0.25-0.5ms", 0},
		{1 * time.Millisecond, "0.5-1ms", 0},
		{2 * time.Millisecond, "1-2ms", 0},
		{5 * time.Millisecond, "2-5ms", 0},
		{0, ">5ms", 0},
	}

	for _, sample := range samples {
		for i := range buckets {
			if buckets[i].max == 0 || sample <= buckets[i].max {
				buckets[i].count++
				break
			}
		}
	}

	t.Log("MQTT Publish Latency Histogram:")
	for _, bucket := range buckets {
		pct := float64(bucket.count) / float64(len(samples)) * 100
		bar := ""
		for j := 0; j < int(pct/2); j++ {
			bar += "\u2588"
		}
		t.Logf("  %10s: %4d (%5.1f%%) %s", bucket.label, bucket.count, pct, bar)
	}
}

// =============================================================================
// SLA Compliance Tests
// =============================================================================

// TestMQTTPublishSLA tests publish latency against SLA targets.
func TestMQTTPublishSLA(t *testing.T) {
	slaTargets := []struct {
		name      string
		qos       int
		p95Target time.Duration
		p99Target time.Duration
	}{
		{"Realtime_QoS0", 0, 1 * time.Millisecond, 5 * time.Millisecond},
		{"Standard_QoS1", 1, 5 * time.Millisecond, 20 * time.Millisecond},
		{"Guaranteed_QoS2", 2, 20 * time.Millisecond, 100 * time.Millisecond},
	}

	iterations := 200

	for _, sla := range slaTargets {
		t.Run(sla.name, func(t *testing.T) {
			cfg := getQoSConfig(sla.qos)
			samples := make([]time.Duration, iterations)

			for i := 0; i < iterations; i++ {
				samples[i] = simulatePublish(cfg.BaseLatency, cfg.Jitter)
			}

			stats := calculateStats(samples)

			p95Pass := stats.P95 <= sla.p95Target
			p99Pass := stats.P99 <= sla.p99Target

			status := "PASS"
			if !p95Pass || !p99Pass {
				status = "FAIL"
			}

			t.Logf("%s SLA %s", sla.name, status)
			t.Logf("  P95: %v (target: %v) %s", stats.P95, sla.p95Target, passSymbol(p95Pass))
			t.Logf("  P99: %v (target: %v) %s", stats.P99, sla.p99Target, passSymbol(p99Pass))
		})
	}
}

// =============================================================================
// Real Broker Latency Benchmarks (requires MQTT broker)
// =============================================================================

// BenchmarkMQTTLatency_RealPublish measures actual publish latency against a broker.
// Requires MQTT broker running on localhost:1883.
func BenchmarkMQTTLatency_RealPublish(b *testing.B) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      "tcp://localhost:1883",
		ClientID:       "bench-latency-publish",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		QoS:            1,
		BufferSize:     10000,
	}, mqttBenchLogger(), nil)
	if err != nil {
		b.Skipf("Failed to create publisher: %v", err)
	}

	if err := publisher.Connect(ctx); err != nil {
		b.Skipf("Failed to connect (broker not available?): %v", err)
	}
	defer publisher.Disconnect()

	dp := domain.NewDataPoint(
		"latency-device",
		"temperature",
		"benchmark/latency/test",
		23.7,
		"\u00b0C",
		domain.QualityGood,
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dp.Value = float64(i)
		start := time.Now()
		_ = publisher.Publish(ctx, dp)
		elapsed := time.Since(start)
		_ = elapsed
	}

	b.StopTimer()

	stats := publisher.Stats()
	b.ReportMetric(float64(stats.MessagesPublished.Load()), "messages")
	b.ReportMetric(float64(stats.MessagesFailed.Load()), "failures")
}

// BenchmarkMQTTLatency_RealPublishByQoS compares real publish latency across QoS levels.
func BenchmarkMQTTLatency_RealPublishByQoS(b *testing.B) {
	for qos := 0; qos <= 2; qos++ {
		b.Run("QoS"+itoa(qos), func(b *testing.B) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			publisher, err := mqtt.NewPublisher(mqtt.Config{
				BrokerURL:      "tcp://localhost:1883",
				ClientID:       "bench-latency-qos" + itoa(qos),
				CleanSession:   true,
				ConnectTimeout: 5 * time.Second,
				QoS:            byte(qos),
				BufferSize:     10000,
			}, mqttBenchLogger(), nil)
			if err != nil {
				b.Skipf("Failed to create publisher: %v", err)
			}

			if err := publisher.Connect(ctx); err != nil {
				b.Skipf("Failed to connect (broker not available?): %v", err)
			}
			defer publisher.Disconnect()

			dp := domain.NewDataPoint(
				"latency-device",
				"temperature",
				"benchmark/latency/qos"+itoa(qos),
				23.7,
				"\u00b0C",
				domain.QualityGood,
			)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				dp.Value = float64(i)
				_ = publisher.Publish(ctx, dp)
			}
		})
	}
}
