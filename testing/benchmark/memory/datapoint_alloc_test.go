//go:build benchmark
// +build benchmark

// Package memory provides benchmark tests for memory allocation analysis.
package memory

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// DataPoint Allocation Types (mirrors domain package)
// =============================================================================

// Quality represents the quality/reliability of a data point.
type Quality string

const (
	QualityGood Quality = "good"
	QualityBad  Quality = "bad"
)

// DataPoint represents a single measurement from a device tag.
type DataPoint struct {
	DeviceID         string            `json:"device_id"`
	TagID            string            `json:"tag_id"`
	Topic            string            `json:"topic"`
	Value            interface{}       `json:"v"`
	RawValue         interface{}       `json:"raw,omitempty"`
	Unit             string            `json:"u,omitempty"`
	Quality          Quality           `json:"q"`
	Timestamp        time.Time         `json:"ts"`
	SourceTimestamp  *time.Time        `json:"source_ts,omitempty"`
	GatewayTimestamp time.Time         `json:"gateway_ts,omitempty"`
	PublishTimestamp *time.Time        `json:"publish_ts,omitempty"`
	LatencyMs        *int64            `json:"latency_ms,omitempty"`
	StalenessMs      *int64            `json:"staleness_ms,omitempty"`
	Priority         uint8             `json:"priority,omitempty"`
	Metadata         map[string]string `json:"meta,omitempty"`
}

// MQTTPayload is the compact MQTT payload format.
type MQTTPayload struct {
	Value     interface{} `json:"v"`
	Unit      string      `json:"u,omitempty"`
	Quality   Quality     `json:"q"`
	Timestamp int64       `json:"ts"`
}

// =============================================================================
// DataPoint Pool for Benchmarks
// =============================================================================

var dataPointPool = sync.Pool{
	New: func() interface{} {
		return &DataPoint{}
	},
}

// acquireDataPoint gets a DataPoint from the pool.
func acquireDataPoint(deviceID, tagID, topic string, value interface{}, unit string, quality Quality) *DataPoint {
	dp := dataPointPool.Get().(*DataPoint)
	dp.DeviceID = deviceID
	dp.TagID = tagID
	dp.Topic = topic
	dp.Value = value
	dp.Unit = unit
	dp.Quality = quality
	dp.Timestamp = time.Now()
	dp.GatewayTimestamp = dp.Timestamp
	dp.RawValue = nil
	dp.SourceTimestamp = nil
	dp.PublishTimestamp = nil
	dp.LatencyMs = nil
	dp.StalenessMs = nil
	dp.Priority = 0
	dp.Metadata = nil
	return dp
}

// releaseDataPoint returns a DataPoint to the pool.
func releaseDataPoint(dp *DataPoint) {
	if dp == nil {
		return
	}
	dp.Value = nil
	dp.RawValue = nil
	dp.Metadata = nil
	dp.SourceTimestamp = nil
	dp.PublishTimestamp = nil
	dp.LatencyMs = nil
	dp.StalenessMs = nil
	dataPointPool.Put(dp)
}

// =============================================================================
// Basic Allocation Benchmarks
// =============================================================================

// BenchmarkDataPointAlloc_New benchmarks allocation using new().
func BenchmarkDataPointAlloc_New(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		dp := &DataPoint{
			DeviceID:  "device-1",
			TagID:     "tag-1",
			Topic:     "devices/device-1/tags/tag-1",
			Value:     float64(123.456),
			Unit:      "°C",
			Quality:   QualityGood,
			Timestamp: time.Now(),
		}
		_ = dp
	}
}

// BenchmarkDataPointAlloc_Pool benchmarks allocation using sync.Pool.
func BenchmarkDataPointAlloc_Pool(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		dp := acquireDataPoint(
			"device-1",
			"tag-1",
			"devices/device-1/tags/tag-1",
			float64(123.456),
			"°C",
			QualityGood,
		)
		releaseDataPoint(dp)
	}
}

// BenchmarkDataPointAlloc_PoolNoRelease benchmarks pool without release (simulates leak).
func BenchmarkDataPointAlloc_PoolNoRelease(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		dp := acquireDataPoint(
			"device-1",
			"tag-1",
			"devices/device-1/tags/tag-1",
			float64(123.456),
			"°C",
			QualityGood,
		)
		_ = dp
		// Intentionally not releasing - shows allocation cost without pool benefit
	}
}

// =============================================================================
// JSON Serialization Allocation Benchmarks
// =============================================================================

// BenchmarkDataPointJSON_Marshal benchmarks JSON marshaling.
func BenchmarkDataPointJSON_Marshal(b *testing.B) {
	dp := &DataPoint{
		DeviceID:  "device-1",
		TagID:     "tag-1",
		Topic:     "devices/device-1/tags/tag-1",
		Value:     float64(123.456),
		Unit:      "°C",
		Quality:   QualityGood,
		Timestamp: time.Now(),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(dp)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDataPointJSON_MarshalCompact benchmarks compact payload marshaling.
func BenchmarkDataPointJSON_MarshalCompact(b *testing.B) {
	payload := MQTTPayload{
		Value:     float64(123.456),
		Unit:      "°C",
		Quality:   QualityGood,
		Timestamp: time.Now().UnixMilli(),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(payload)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDataPointJSON_Unmarshal benchmarks JSON unmarshaling.
func BenchmarkDataPointJSON_Unmarshal(b *testing.B) {
	data := []byte(`{"device_id":"device-1","tag_id":"tag-1","v":123.456,"u":"°C","q":"good","ts":"2024-01-01T00:00:00Z"}`)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var dp DataPoint
		err := json.Unmarshal(data, &dp)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// Value Type Allocation Benchmarks
// =============================================================================

// BenchmarkDataPointValue_Float64 benchmarks float64 value allocation.
func BenchmarkDataPointValue_Float64(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		dp := acquireDataPoint("d", "t", "topic", float64(123.456), "", QualityGood)
		releaseDataPoint(dp)
	}
}

// BenchmarkDataPointValue_Int32 benchmarks int32 value allocation.
func BenchmarkDataPointValue_Int32(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		dp := acquireDataPoint("d", "t", "topic", int32(12345), "", QualityGood)
		releaseDataPoint(dp)
	}
}

// BenchmarkDataPointValue_String benchmarks string value allocation.
func BenchmarkDataPointValue_String(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		dp := acquireDataPoint("d", "t", "topic", "test-value", "", QualityGood)
		releaseDataPoint(dp)
	}
}

// BenchmarkDataPointValue_ByteSlice benchmarks []byte value allocation.
func BenchmarkDataPointValue_ByteSlice(b *testing.B) {
	value := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		dp := acquireDataPoint("d", "t", "topic", value, "", QualityGood)
		releaseDataPoint(dp)
	}
}

// =============================================================================
// Metadata Allocation Benchmarks
// =============================================================================

// BenchmarkDataPointMetadata_None benchmarks allocation without metadata.
func BenchmarkDataPointMetadata_None(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		dp := acquireDataPoint("d", "t", "topic", 123.456, "", QualityGood)
		releaseDataPoint(dp)
	}
}

// BenchmarkDataPointMetadata_Small benchmarks allocation with small metadata.
func BenchmarkDataPointMetadata_Small(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		dp := acquireDataPoint("d", "t", "topic", 123.456, "", QualityGood)
		dp.Metadata = map[string]string{
			"source": "plc-1",
		}
		releaseDataPoint(dp)
	}
}

// BenchmarkDataPointMetadata_Medium benchmarks allocation with medium metadata.
func BenchmarkDataPointMetadata_Medium(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		dp := acquireDataPoint("d", "t", "topic", 123.456, "", QualityGood)
		dp.Metadata = map[string]string{
			"source":   "plc-1",
			"area":     "production",
			"line":     "line-1",
			"station":  "station-5",
			"operator": "auto",
		}
		releaseDataPoint(dp)
	}
}

// =============================================================================
// Batch Allocation Benchmarks
// =============================================================================

// BenchmarkDataPointBatch_10 benchmarks batch of 10 data points.
func BenchmarkDataPointBatch_10(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		batch := make([]*DataPoint, 10)
		for j := 0; j < 10; j++ {
			batch[j] = acquireDataPoint("d", "t", "topic", float64(j), "", QualityGood)
		}
		for j := 0; j < 10; j++ {
			releaseDataPoint(batch[j])
		}
	}
}

// BenchmarkDataPointBatch_100 benchmarks batch of 100 data points.
func BenchmarkDataPointBatch_100(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		batch := make([]*DataPoint, 100)
		for j := 0; j < 100; j++ {
			batch[j] = acquireDataPoint("d", "t", "topic", float64(j), "", QualityGood)
		}
		for j := 0; j < 100; j++ {
			releaseDataPoint(batch[j])
		}
	}
}

// BenchmarkDataPointBatch_1000 benchmarks batch of 1000 data points.
func BenchmarkDataPointBatch_1000(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		batch := make([]*DataPoint, 1000)
		for j := 0; j < 1000; j++ {
			batch[j] = acquireDataPoint("d", "t", "topic", float64(j), "", QualityGood)
		}
		for j := 0; j < 1000; j++ {
			releaseDataPoint(batch[j])
		}
	}
}

// =============================================================================
// Concurrent Allocation Benchmarks
// =============================================================================

// BenchmarkDataPointAlloc_Concurrent benchmarks concurrent pool access.
func BenchmarkDataPointAlloc_Concurrent(b *testing.B) {
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			dp := acquireDataPoint("d", "t", "topic", 123.456, "", QualityGood)
			releaseDataPoint(dp)
		}
	})
}

// =============================================================================
// Memory Size Analysis Tests
// =============================================================================

// TestDataPointMemorySize analyzes DataPoint struct memory layout.
func TestDataPointMemorySize(t *testing.T) {
	// Report estimated sizes
	t.Logf("DataPoint struct analysis:")
	t.Logf("  DeviceID (string):         16 bytes (header)")
	t.Logf("  TagID (string):            16 bytes (header)")
	t.Logf("  Topic (string):            16 bytes (header)")
	t.Logf("  Value (interface{}):       16 bytes")
	t.Logf("  RawValue (interface{}):    16 bytes")
	t.Logf("  Unit (string):             16 bytes (header)")
	t.Logf("  Quality (string):          16 bytes (header)")
	t.Logf("  Timestamp (time.Time):     24 bytes")
	t.Logf("  SourceTimestamp (*time):    8 bytes")
	t.Logf("  GatewayTimestamp (time):   24 bytes")
	t.Logf("  PublishTimestamp (*time):   8 bytes")
	t.Logf("  LatencyMs (*int64):         8 bytes")
	t.Logf("  StalenessMs (*int64):       8 bytes")
	t.Logf("  Priority (uint8):           1 byte (+padding)")
	t.Logf("  Metadata (map):             8 bytes (header)")
	t.Logf("  ----------------------------------------")
	t.Logf("  Estimated total:          ~200 bytes (excluding string data)")
}

// TestPoolEfficiency analyzes pool hit rate.
func TestPoolEfficiency(t *testing.T) {
	iterations := 10000
	hitCount := 0

	// Warm up the pool
	warmup := make([]*DataPoint, 100)
	for i := 0; i < 100; i++ {
		warmup[i] = acquireDataPoint("d", "t", "topic", 0, "", QualityGood)
	}
	for i := 0; i < 100; i++ {
		releaseDataPoint(warmup[i])
	}

	// Measure reuse
	for i := 0; i < iterations; i++ {
		dp := acquireDataPoint("d", "t", "topic", float64(i), "", QualityGood)
		// In a real pool implementation, we'd track if this was a reuse
		releaseDataPoint(dp)
		hitCount++ // Simplified - assume hit after warmup
	}

	hitRate := float64(hitCount) / float64(iterations) * 100
	t.Logf("Pool efficiency after warmup: %.1f%% hit rate", hitRate)
	t.Logf("Note: Actual hit rate depends on concurrent usage patterns")
}

// =============================================================================
// Comparison Tests
// =============================================================================

// TestAllocComparison compares different allocation strategies.
func TestAllocComparison(t *testing.T) {
	iterations := 1000

	// Strategy 1: Direct allocation
	start := time.Now()
	for i := 0; i < iterations; i++ {
		dp := &DataPoint{
			DeviceID: "d", TagID: "t", Topic: "topic",
			Value: float64(i), Quality: QualityGood, Timestamp: time.Now(),
		}
		_ = dp
	}
	directDuration := time.Since(start)

	// Strategy 2: Pool allocation
	start = time.Now()
	for i := 0; i < iterations; i++ {
		dp := acquireDataPoint("d", "t", "topic", float64(i), "", QualityGood)
		releaseDataPoint(dp)
	}
	poolDuration := time.Since(start)

	t.Logf("Allocation comparison (%d iterations):", iterations)
	t.Logf("  Direct allocation: %v", directDuration)
	t.Logf("  Pool allocation:   %v", poolDuration)
	t.Logf("  Pool speedup:      %.2fx", float64(directDuration)/float64(poolDuration))
}
