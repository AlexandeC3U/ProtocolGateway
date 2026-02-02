package throughput_test

import (
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

// BenchmarkDataPoint_Creation measures DataPoint creation speed.
func BenchmarkDataPoint_Creation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = domain.NewDataPoint(
			"device-1",
			"tag-1",
			"plant/line/device/tag",
			float64(42.42),
			"°C",
			domain.QualityGood,
		)
	}
}

// BenchmarkDataPoint_Pool measures pooled DataPoint creation.
func BenchmarkDataPoint_Pool(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dp := domain.AcquireDataPoint(
			"device-1",
			"tag-1",
			"plant/line/device/tag",
			float64(42.42),
			"°C",
			domain.QualityGood,
		)
		domain.ReleaseDataPoint(dp)
	}
}

// BenchmarkDataPoint_ToJSON measures JSON serialization speed.
func BenchmarkDataPoint_ToJSON(b *testing.B) {
	dp := &domain.DataPoint{
		DeviceID:  "device-1",
		TagID:     "tag-1",
		Topic:     "plant/line/device/tag",
		Value:     float64(42.42),
		Unit:      "°C",
		Quality:   domain.QualityGood,
		Timestamp: time.Now(),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = dp.ToJSON()
	}
}

// BenchmarkDataPoint_ToMQTTPayload measures payload conversion speed.
func BenchmarkDataPoint_ToMQTTPayload(b *testing.B) {
	dp := &domain.DataPoint{
		DeviceID:  "device-1",
		TagID:     "tag-1",
		Topic:     "plant/line/device/tag",
		Value:     float64(42.42),
		Unit:      "°C",
		Quality:   domain.QualityGood,
		Timestamp: time.Now(),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = dp.ToMQTTPayload()
	}
}

// BenchmarkDataPoint_Batch measures batch processing speed.
func BenchmarkDataPoint_Batch(b *testing.B) {
	// Create a batch of tags
	const batchSize = 100
	points := make([]*domain.DataPoint, batchSize)

	for i := range points {
		points[i] = domain.NewDataPoint(
			"device-1",
			"tag-"+string(rune('0'+i%10)),
			"topic",
			float64(i),
			"unit",
			domain.QualityGood,
		)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Simulate batch processing
		batch := &domain.DataPointBatch{
			DeviceID:  "device-1",
			Points:    points,
			Timestamp: time.Now(),
		}
		_ = batch.DeviceID
	}
}

// BenchmarkDataPoint_Parallel_Creation measures parallel DataPoint creation.
func BenchmarkDataPoint_Parallel_Creation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			dp := domain.AcquireDataPoint(
				"device-1",
				"tag-1",
				"plant/line/device/tag",
				float64(42.42),
				"°C",
				domain.QualityGood,
			)
			domain.ReleaseDataPoint(dp)
		}
	})
}

// BenchmarkDataPoint_Parallel_ToJSON measures parallel JSON serialization.
func BenchmarkDataPoint_Parallel_ToJSON(b *testing.B) {
	dp := &domain.DataPoint{
		DeviceID:  "device-1",
		TagID:     "tag-1",
		Topic:     "plant/line/device/tag",
		Value:     float64(42.42),
		Unit:      "°C",
		Quality:   domain.QualityGood,
		Timestamp: time.Now(),
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = dp.ToJSON()
		}
	})
}
