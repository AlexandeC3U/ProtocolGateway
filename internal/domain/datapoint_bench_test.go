package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func BenchmarkDataPoint_ToMQTTPayload(b *testing.B) {
	dp := &DataPoint{
		DeviceID:  "dev-1",
		TagID:     "tag-1",
		Topic:     "devices/dev-1/tags/tag-1",
		Value:     float64(42.42),
		RawValue:  int64(4242),
		Unit:      "unit",
		Quality:   QualityGood,
		Timestamp: time.Unix(1_700_000_000, 123_000_000),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = dp.ToMQTTPayload()
	}
}

func BenchmarkDataPoint_ToMQTTPayload_JSONMarshal(b *testing.B) {
	dp := &DataPoint{
		DeviceID:  "dev-1",
		TagID:     "tag-1",
		Topic:     "devices/dev-1/tags/tag-1",
		Value:     float64(42.42),
		RawValue:  int64(4242),
		Unit:      "unit",
		Quality:   QualityGood,
		Timestamp: time.Unix(1_700_000_000, 123_000_000),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		payload := dp.ToMQTTPayload()
		_, _ = json.Marshal(payload)
	}
}

func BenchmarkDataPoint_ToJSON(b *testing.B) {
	dp := &DataPoint{
		DeviceID:  "dev-1",
		TagID:     "tag-1",
		Topic:     "devices/dev-1/tags/tag-1",
		Value:     float64(42.42),
		RawValue:  int64(4242),
		Unit:      "unit",
		Quality:   QualityGood,
		Timestamp: time.Unix(1_700_000_000, 123_000_000),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = dp.ToJSON()
	}
}
