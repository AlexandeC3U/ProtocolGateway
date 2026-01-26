// Package domain contains core business entities.
package domain

import (
	"encoding/json"
	"sync"
	"time"
)

// dataPointPool is a sync.Pool for reusing DataPoint objects.
// This reduces GC pressure in high-throughput scenarios.
var dataPointPool = sync.Pool{
	New: func() interface{} {
		return &DataPoint{}
	},
}

// Quality represents the quality/reliability of a data point.
type Quality string

const (
	QualityGood          Quality = "good"
	QualityBad           Quality = "bad"
	QualityUncertain     Quality = "uncertain"
	QualityNotConnected  Quality = "not_connected"
	QualityConfigError   Quality = "config_error"
	QualityDeviceFailure Quality = "device_failure"
	QualityTimeout       Quality = "timeout"
)

// DataPoint represents a single measurement from a device tag.
type DataPoint struct {
	// DeviceID identifies the source device
	DeviceID string `json:"device_id"`

	// TagID identifies the source tag
	TagID string `json:"tag_id"`

	// Topic is the full MQTT topic for this data point
	Topic string `json:"topic"`

	// Value is the processed/scaled value
	Value interface{} `json:"v"`

	// RawValue is the original unprocessed value
	RawValue interface{} `json:"raw,omitempty"`

	// Unit is the engineering unit
	Unit string `json:"u,omitempty"`

	// Quality indicates the reliability of this data point
	Quality Quality `json:"q"`

	// Timestamp is when this value was read from the device
	Timestamp time.Time `json:"ts"`

	// SourceTimestamp is the device-provided timestamp (if available)
	SourceTimestamp *time.Time `json:"source_ts,omitempty"`

	// Metadata contains additional context
	Metadata map[string]string `json:"meta,omitempty"`
}

// MQTTPayload represents the compact payload format for MQTT publishing.
// Uses short field names to minimize bandwidth.
type MQTTPayload struct {
	Value     interface{} `json:"v"`           // Value
	Unit      string      `json:"u,omitempty"` // Unit
	Quality   Quality     `json:"q"`           // Quality
	Timestamp int64       `json:"ts"`          // Unix timestamp (milliseconds)
}

// ToMQTTPayload converts the DataPoint to a compact MQTT payload.
func (dp *DataPoint) ToMQTTPayload() MQTTPayload {
	return MQTTPayload{
		Value:     dp.Value,
		Unit:      dp.Unit,
		Quality:   dp.Quality,
		Timestamp: dp.Timestamp.UnixMilli(),
	}
}

// ToJSON serializes the MQTT payload to JSON bytes.
func (dp *DataPoint) ToJSON() ([]byte, error) {
	payload := dp.ToMQTTPayload()
	return json.Marshal(payload)
}

// SparkplugBPayload represents the Sparkplug B compatible payload format.
// Useful for integration with Sparkplug B ecosystems.
type SparkplugBPayload struct {
	Timestamp int64              `json:"timestamp"`
	Metrics   []SparkplugBMetric `json:"metrics"`
}

// SparkplugBMetric represents a single metric in Sparkplug B format.
type SparkplugBMetric struct {
	Name      string      `json:"name"`
	Alias     *uint64     `json:"alias,omitempty"`
	Timestamp int64       `json:"timestamp"`
	DataType  string      `json:"dataType"`
	Value     interface{} `json:"value"`
}

// DataPointBatch represents a batch of data points for efficient processing.
type DataPointBatch struct {
	DeviceID  string       `json:"device_id"`
	Points    []*DataPoint `json:"points"`
	Timestamp time.Time    `json:"timestamp"`
}

// NewDataPoint creates a new DataPoint with the current timestamp.
// For high-throughput scenarios, consider using AcquireDataPoint() instead.
func NewDataPoint(deviceID, tagID, topic string, value interface{}, unit string, quality Quality) *DataPoint {
	return &DataPoint{
		DeviceID:  deviceID,
		TagID:     tagID,
		Topic:     topic,
		Value:     value,
		Unit:      unit,
		Quality:   quality,
		Timestamp: time.Now(),
	}
}

// AcquireDataPoint gets a DataPoint from the pool and initializes it.
// This reduces GC pressure in high-throughput scenarios.
// Remember to call ReleaseDataPoint() when done with the DataPoint.
func AcquireDataPoint(deviceID, tagID, topic string, value interface{}, unit string, quality Quality) *DataPoint {
	dp := dataPointPool.Get().(*DataPoint)
	dp.DeviceID = deviceID
	dp.TagID = tagID
	dp.Topic = topic
	dp.Value = value
	dp.Unit = unit
	dp.Quality = quality
	dp.Timestamp = time.Now()
	dp.RawValue = nil
	dp.SourceTimestamp = nil
	dp.Metadata = nil
	return dp
}

// ReleaseDataPoint returns a DataPoint to the pool for reuse.
// After calling this, the DataPoint should not be used anymore.
func ReleaseDataPoint(dp *DataPoint) {
	if dp == nil {
		return
	}
	// Clear references to allow GC of referenced objects
	dp.Value = nil
	dp.RawValue = nil
	dp.Metadata = nil
	dp.SourceTimestamp = nil
	dataPointPool.Put(dp)
}

// WithRawValue sets the raw value and returns the DataPoint for chaining.
func (dp *DataPoint) WithRawValue(raw interface{}) *DataPoint {
	dp.RawValue = raw
	return dp
}

// WithMetadata sets metadata and returns the DataPoint for chaining.
func (dp *DataPoint) WithMetadata(meta map[string]string) *DataPoint {
	dp.Metadata = meta
	return dp
}

// WithSourceTimestamp sets the source timestamp and returns the DataPoint for chaining.
func (dp *DataPoint) WithSourceTimestamp(ts time.Time) *DataPoint {
	dp.SourceTimestamp = &ts
	return dp
}

// Reset clears the DataPoint for reuse.
func (dp *DataPoint) Reset() {
	dp.DeviceID = ""
	dp.TagID = ""
	dp.Topic = ""
	dp.Value = nil
	dp.RawValue = nil
	dp.Unit = ""
	dp.Quality = ""
	dp.Timestamp = time.Time{}
	dp.SourceTimestamp = nil
	dp.Metadata = nil
}

