package throughput_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/mqtt"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/rs/zerolog"
)

// =============================================================================
// MQTT Publish Throughput Benchmarks
// =============================================================================

// mockMQTTClient provides a mock MQTT client for benchmarking without network I/O.
type mockMQTTClient struct {
	published atomic.Uint64
}

func benchLogger() zerolog.Logger {
	return zerolog.Nop()
}

// BenchmarkMQTT_PublishSerialization measures the serialization overhead of publishing.
func BenchmarkMQTT_PublishSerialization(b *testing.B) {
	dp := domain.NewDataPoint(
		"benchmark-device",
		"benchmark-tag",
		"plant/area/line/device/tag",
		42.5,
		"°C",
		domain.QualityGood,
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Measure just the serialization (what happens before network send)
		_, _ = dp.ToJSON()
	}
}

// BenchmarkMQTT_PublishPayloadCreation measures DataPoint to MQTT payload conversion.
func BenchmarkMQTT_PublishPayloadCreation(b *testing.B) {
	dp := domain.NewDataPoint(
		"benchmark-device",
		"benchmark-tag",
		"plant/area/line/device/tag",
		42.5,
		"°C",
		domain.QualityGood,
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = dp.ToMQTTPayload()
	}
}

// BenchmarkMQTT_PublishBatchSerialization measures batch serialization throughput.
func BenchmarkMQTT_PublishBatchSerialization(b *testing.B) {
	batchSizes := []int{1, 10, 50, 100, 500}

	for _, size := range batchSizes {
		b.Run(sizeLabel(size), func(b *testing.B) {
			// Pre-create the batch
			batch := make([]*domain.DataPoint, size)
			for i := range batch {
				batch[i] = domain.NewDataPoint(
					"device-1",
					"tag-"+string(rune('A'+i%26)),
					"topic/"+string(rune('0'+i%10)),
					float64(i)*1.5,
					"unit",
					domain.QualityGood,
				)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				for _, dp := range batch {
					_, _ = dp.ToJSON()
				}
			}
		})
	}
}

func sizeLabel(n int) string {
	switch {
	case n >= 500:
		return "500_messages"
	case n >= 100:
		return "100_messages"
	case n >= 50:
		return "50_messages"
	case n >= 10:
		return "10_messages"
	default:
		return "1_message"
	}
}

// BenchmarkMQTT_TopicFormatting measures topic string formatting overhead.
func BenchmarkMQTT_TopicFormatting(b *testing.B) {
	prefix := "plant/area1/line2"
	suffix := "temperature"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = prefix + "/" + suffix
	}
}

// BenchmarkMQTT_ConcurrentPublishPreparation measures concurrent message preparation.
func BenchmarkMQTT_ConcurrentPublishPreparation(b *testing.B) {
	workers := []int{1, 2, 4, 8}

	for _, numWorkers := range workers {
		b.Run(workerLabel(numWorkers), func(b *testing.B) {
			var wg sync.WaitGroup
			messagesPerWorker := b.N / numWorkers
			if messagesPerWorker < 1 {
				messagesPerWorker = 1
			}

			b.ReportAllocs()
			b.ResetTimer()

			for w := 0; w < numWorkers; w++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()
					for i := 0; i < messagesPerWorker; i++ {
						dp := domain.NewDataPoint(
							"device-1",
							"tag-1",
							"topic",
							float64(i),
							"unit",
							domain.QualityGood,
						)
						_, _ = dp.ToJSON()
					}
				}(w)
			}

			wg.Wait()
		})
	}
}

func workerLabel(n int) string {
	switch n {
	case 1:
		return "1_worker"
	case 2:
		return "2_workers"
	case 4:
		return "4_workers"
	case 8:
		return "8_workers"
	default:
		return "N_workers"
	}
}

// =============================================================================
// Publisher Config Benchmarks
// =============================================================================

// BenchmarkMQTT_PublisherCreation measures publisher instantiation overhead.
func BenchmarkMQTT_PublisherCreation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = mqtt.NewPublisher(mqtt.Config{
			BrokerURL:      "tcp://localhost:1883",
			ClientID:       "benchmark-client",
			CleanSession:   true,
			ConnectTimeout: 5 * time.Second,
			QoS:            1,
			BufferSize:     1000,
		}, benchLogger(), nil)
	}
}

// BenchmarkMQTT_ConfigValidation measures config validation overhead.
func BenchmarkMQTT_ConfigValidation(b *testing.B) {
	cfg := mqtt.Config{
		BrokerURL:      "tcp://localhost:1883",
		ClientID:       "benchmark-client",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		QoS:            1,
		BufferSize:     1000,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Simulate validation checks
		_ = cfg.BrokerURL != ""
		_ = cfg.ClientID != ""
		_ = cfg.QoS <= 2
		_ = cfg.BufferSize > 0
	}
}

// =============================================================================
// Stats Tracking Benchmarks
// =============================================================================

// BenchmarkMQTT_StatsAtomicIncrement measures atomic counter overhead.
func BenchmarkMQTT_StatsAtomicIncrement(b *testing.B) {
	var counter atomic.Uint64

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		counter.Add(1)
	}
}

// BenchmarkMQTT_StatsAtomicLoad measures atomic counter read overhead.
func BenchmarkMQTT_StatsAtomicLoad(b *testing.B) {
	var counter atomic.Uint64
	counter.Store(1000000)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = counter.Load()
	}
}

// BenchmarkMQTT_StatsConcurrentIncrement measures concurrent atomic updates.
func BenchmarkMQTT_StatsConcurrentIncrement(b *testing.B) {
	var counter atomic.Uint64
	var wg sync.WaitGroup
	workers := 4

	b.ReportAllocs()
	b.ResetTimer()

	incrementsPerWorker := b.N / workers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < incrementsPerWorker; i++ {
				counter.Add(1)
			}
		}()
	}

	wg.Wait()
}

// =============================================================================
// Buffer Management Benchmarks
// =============================================================================

// BenchmarkMQTT_BufferedMessageCreation measures buffered message struct creation.
func BenchmarkMQTT_BufferedMessageCreation(b *testing.B) {
	payload := []byte(`{"v":42.5,"u":"°C","q":"good","ts":1234567890}`)
	topic := "plant/area/line/device/tag"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = &mqtt.BufferedMessage{
			Topic:     topic,
			Payload:   payload,
			QoS:       1,
			Retained:  false,
			Timestamp: time.Now(),
		}
	}
}

// BenchmarkMQTT_BufferChannelOperations measures channel send/receive overhead.
func BenchmarkMQTT_BufferChannelOperations(b *testing.B) {
	ch := make(chan *mqtt.BufferedMessage, 1000)

	msg := &mqtt.BufferedMessage{
		Topic:   "topic",
		Payload: []byte("payload"),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ch <- msg
		<-ch
	}
}

// =============================================================================
// Integration Publish Benchmark (requires broker)
// =============================================================================

// BenchmarkMQTT_RealPublish benchmarks actual publishing to a broker.
// Requires MQTT broker running on localhost:1883.
// Run with: go test -bench=BenchmarkMQTT_RealPublish -benchtime=5s
func BenchmarkMQTT_RealPublish(b *testing.B) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      "tcp://localhost:1883",
		ClientID:       "benchmark-real-publish",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		QoS:            0, // QoS 0 for max throughput
		BufferSize:     10000,
	}, benchLogger(), nil)
	if err != nil {
		b.Skipf("Failed to create publisher: %v", err)
	}

	if err := publisher.Connect(ctx); err != nil {
		b.Skipf("Failed to connect (broker not available?): %v", err)
	}
	defer publisher.Disconnect()

	dp := domain.NewDataPoint(
		"benchmark-device",
		"benchmark-tag",
		"benchmark/throughput/test",
		42.5,
		"°C",
		domain.QualityGood,
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dp.Value = float64(i)
		_ = publisher.Publish(ctx, dp)
	}

	b.StopTimer()

	stats := publisher.Stats()
	b.ReportMetric(float64(stats.MessagesPublished.Load()), "messages")
	b.ReportMetric(float64(stats.MessagesFailed.Load()), "failures")
}

// BenchmarkMQTT_RealPublishBatch benchmarks batch publishing to a broker.
func BenchmarkMQTT_RealPublishBatch(b *testing.B) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	publisher, err := mqtt.NewPublisher(mqtt.Config{
		BrokerURL:      "tcp://localhost:1883",
		ClientID:       "benchmark-batch-publish",
		CleanSession:   true,
		ConnectTimeout: 5 * time.Second,
		QoS:            0,
		BufferSize:     10000,
	}, benchLogger(), nil)
	if err != nil {
		b.Skipf("Failed to create publisher: %v", err)
	}

	if err := publisher.Connect(ctx); err != nil {
		b.Skipf("Failed to connect (broker not available?): %v", err)
	}
	defer publisher.Disconnect()

	// Create batch
	batchSize := 100
	batch := make([]*domain.DataPoint, batchSize)
	for i := range batch {
		batch[i] = domain.NewDataPoint(
			"benchmark-device",
			"tag-"+string(rune('0'+i%10)),
			"benchmark/batch/"+string(rune('0'+i%10)),
			float64(i),
			"unit",
			domain.QualityGood,
		)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = publisher.PublishBatch(ctx, batch)
	}

	b.StopTimer()

	stats := publisher.Stats()
	b.ReportMetric(float64(stats.MessagesPublished.Load()), "messages")
	b.ReportMetric(float64(stats.MessagesPublished.Load())/float64(b.N), "msgs_per_batch")
}
