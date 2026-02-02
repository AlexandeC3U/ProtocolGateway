package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

func TestNewDataPoint(t *testing.T) {
	before := time.Now()
	dp := domain.NewDataPoint("dev-1", "tag-1", "test/topic", 42.5, "°C", domain.QualityGood)
	after := time.Now()

	if dp.DeviceID != "dev-1" {
		t.Errorf("expected DeviceID 'dev-1', got '%s'", dp.DeviceID)
	}
	if dp.TagID != "tag-1" {
		t.Errorf("expected TagID 'tag-1', got '%s'", dp.TagID)
	}
	if dp.Topic != "test/topic" {
		t.Errorf("expected Topic 'test/topic', got '%s'", dp.Topic)
	}
	if dp.Value != 42.5 {
		t.Errorf("expected Value 42.5, got %v", dp.Value)
	}
	if dp.Unit != "°C" {
		t.Errorf("expected Unit '°C', got '%s'", dp.Unit)
	}
	if dp.Quality != domain.QualityGood {
		t.Errorf("expected Quality 'good', got '%s'", dp.Quality)
	}
	if dp.Timestamp.Before(before) || dp.Timestamp.After(after) {
		t.Errorf("Timestamp %v not in expected range [%v, %v]", dp.Timestamp, before, after)
	}
}

func TestDataPoint_ToMQTTPayload(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	dp := &domain.DataPoint{
		DeviceID:  "dev-1",
		TagID:     "tag-1",
		Topic:     "test/topic",
		Value:     123.456,
		Unit:      "bar",
		Quality:   domain.QualityGood,
		Timestamp: ts,
	}

	payload := dp.ToMQTTPayload()

	if payload.Value != 123.456 {
		t.Errorf("expected Value 123.456, got %v", payload.Value)
	}
	if payload.Unit != "bar" {
		t.Errorf("expected Unit 'bar', got '%s'", payload.Unit)
	}
	if payload.Quality != domain.QualityGood {
		t.Errorf("expected Quality 'good', got '%s'", payload.Quality)
	}
	if payload.Timestamp != ts.UnixMilli() {
		t.Errorf("expected Timestamp %d, got %d", ts.UnixMilli(), payload.Timestamp)
	}
}

func TestDataPoint_ToJSON(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	dp := &domain.DataPoint{
		DeviceID:  "dev-1",
		TagID:     "tag-1",
		Topic:     "test/topic",
		Value:     42,
		Unit:      "m/s",
		Quality:   domain.QualityGood,
		Timestamp: ts,
	}

	jsonBytes, err := dp.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Check fields are present
	if _, ok := result["v"]; !ok {
		t.Error("expected 'v' field in JSON")
	}
	if _, ok := result["q"]; !ok {
		t.Error("expected 'q' field in JSON")
	}
	if _, ok := result["ts"]; !ok {
		t.Error("expected 'ts' field in JSON")
	}
}

func TestDataPoint_ToJSON_NilValue(t *testing.T) {
	dp := &domain.DataPoint{
		DeviceID:  "dev-1",
		TagID:     "tag-1",
		Value:     nil,
		Quality:   domain.QualityBad,
		Timestamp: time.Now(),
	}

	jsonBytes, err := dp.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if result["v"] != nil {
		t.Errorf("expected nil value, got %v", result["v"])
	}
}

func TestDataPoint_ToJSON_DifferentValueTypes(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
	}{
		{"int", 42},
		{"float", 3.14159},
		{"bool", true},
		{"string", "hello"},
		{"int64", int64(9999999999)},
		{"float32", float32(1.5)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp := &domain.DataPoint{
				DeviceID:  "dev-1",
				TagID:     "tag-1",
				Value:     tt.value,
				Quality:   domain.QualityGood,
				Timestamp: time.Now(),
			}

			jsonBytes, err := dp.ToJSON()
			if err != nil {
				t.Fatalf("ToJSON failed for %s: %v", tt.name, err)
			}

			if len(jsonBytes) == 0 {
				t.Error("expected non-empty JSON")
			}
		})
	}
}

func TestQuality_Constants(t *testing.T) {
	// Verify all quality constants are defined correctly
	qualities := []domain.Quality{
		domain.QualityGood,
		domain.QualityBad,
		domain.QualityUncertain,
		domain.QualityNotConnected,
		domain.QualityConfigError,
		domain.QualityDeviceFailure,
		domain.QualityTimeout,
	}

	expected := []string{
		"good",
		"bad",
		"uncertain",
		"not_connected",
		"config_error",
		"device_failure",
		"timeout",
	}

	for i, q := range qualities {
		if string(q) != expected[i] {
			t.Errorf("expected quality '%s', got '%s'", expected[i], q)
		}
	}
}

func TestAcquireAndReleaseDataPoint(t *testing.T) {
	// Test that pool acquire/release works without panic
	dp := domain.AcquireDataPoint("dev-1", "tag-1", "topic", 100, "unit", domain.QualityGood)

	if dp == nil {
		t.Fatal("AcquireDataPoint returned nil")
	}

	if dp.DeviceID != "dev-1" {
		t.Errorf("expected DeviceID 'dev-1', got '%s'", dp.DeviceID)
	}

	// Release should not panic
	domain.ReleaseDataPoint(dp)
}

func TestDataPointBatch(t *testing.T) {
	points := []*domain.DataPoint{
		domain.NewDataPoint("dev-1", "tag-1", "t1", 1, "", domain.QualityGood),
		domain.NewDataPoint("dev-1", "tag-2", "t2", 2, "", domain.QualityGood),
		domain.NewDataPoint("dev-1", "tag-3", "t3", 3, "", domain.QualityGood),
	}

	batch := &domain.DataPointBatch{
		DeviceID:  "dev-1",
		Points:    points,
		Timestamp: time.Now(),
	}

	if len(batch.Points) != 3 {
		t.Errorf("expected 3 points, got %d", len(batch.Points))
	}

	if batch.DeviceID != "dev-1" {
		t.Errorf("expected DeviceID 'dev-1', got '%s'", batch.DeviceID)
	}
}

func TestDataPoint_JSONRoundTrip(t *testing.T) {
	original := &domain.DataPoint{
		DeviceID:  "device-test",
		TagID:     "tag-test",
		Topic:     "test/topic/path",
		Value:     99.99,
		Unit:      "kg",
		Quality:   domain.QualityUncertain,
		Timestamp: time.Unix(1700000000, 0),
	}

	// Convert to JSON
	jsonBytes, err := original.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Parse back
	var payload domain.MQTTPayload
	if err := json.Unmarshal(jsonBytes, &payload); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify
	if payload.Quality != original.Quality {
		t.Errorf("Quality mismatch: expected %s, got %s", original.Quality, payload.Quality)
	}
	if payload.Unit != original.Unit {
		t.Errorf("Unit mismatch: expected %s, got %s", original.Unit, payload.Unit)
	}
}

func TestMQTTPayload_Serialization(t *testing.T) {
	payload := domain.MQTTPayload{
		Value:     42.0,
		Unit:      "m",
		Quality:   domain.QualityGood,
		Timestamp: 1700000000000,
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Check compact field names are used
	jsonStr := string(jsonBytes)
	if !contains(jsonStr, `"v":`) {
		t.Error("expected 'v' field name")
	}
	if !contains(jsonStr, `"q":`) {
		t.Error("expected 'q' field name")
	}
	if !contains(jsonStr, `"ts":`) {
		t.Error("expected 'ts' field name")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
