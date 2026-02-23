// Package metrics_test tests the clock drift metrics functionality.
package metrics_test

import (
	"testing"

	"github.com/nexus-edge/protocol-gateway/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// TestClockDriftMetrics tests all clock drift metric recording in a single test
// to avoid duplicate Prometheus metric registration from multiple NewRegistry() calls.
func TestClockDriftMetrics(t *testing.T) {
	r := metrics.NewRegistry()

	t.Run("RecordClockDrift_Success", func(t *testing.T) {
		r.RecordClockDrift(0.150, true)

		// Verify gauge value
		val := getGaugeValue(t, r.ClockDriftSeconds)
		if val != 0.150 {
			t.Errorf("expected ClockDriftSeconds 0.150, got %f", val)
		}

		// Verify success counter incremented
		count := getCounterValue(t, r.ClockDriftChecks.WithLabelValues("success"))
		if count != 1 {
			t.Errorf("expected success counter 1, got %f", count)
		}

		// Verify error counter is zero
		errorCount := getCounterValue(t, r.ClockDriftChecks.WithLabelValues("error"))
		if errorCount != 0 {
			t.Errorf("expected error counter 0, got %f", errorCount)
		}
	})

	t.Run("RecordClockDrift_Error", func(t *testing.T) {
		// Record an error — gauge should NOT be updated by the error path
		prevVal := getGaugeValue(t, r.ClockDriftSeconds)
		r.RecordClockDrift(0, false)

		// Gauge should remain at previous value (error doesn't update gauge)
		val := getGaugeValue(t, r.ClockDriftSeconds)
		if val != prevVal {
			t.Errorf("expected ClockDriftSeconds %f (unchanged after error), got %f", prevVal, val)
		}

		// Error counter should be 1
		errorCount := getCounterValue(t, r.ClockDriftChecks.WithLabelValues("error"))
		if errorCount != 1 {
			t.Errorf("expected error counter 1, got %f", errorCount)
		}
	})

	t.Run("RecordClockDrift_NegativeOffset", func(t *testing.T) {
		r.RecordClockDrift(-0.250, true)

		val := getGaugeValue(t, r.ClockDriftSeconds)
		if val != -0.250 {
			t.Errorf("expected ClockDriftSeconds -0.250, got %f", val)
		}
	})

	t.Run("RecordClockDrift_MultipleChecks", func(t *testing.T) {
		// Reset with a known value first
		r.RecordClockDrift(0, true)

		offsets := []float64{0.010, 0.025, -0.005, 0.100}
		for _, offset := range offsets {
			r.RecordClockDrift(offset, true)
		}

		// Gauge should reflect the last recorded offset
		val := getGaugeValue(t, r.ClockDriftSeconds)
		if val != 0.100 {
			t.Errorf("expected ClockDriftSeconds 0.100, got %f", val)
		}
	})

	t.Run("RecordOPCUAClockDrift", func(t *testing.T) {
		r.RecordOPCUAClockDrift("plc-001", 0.350)
		r.RecordOPCUAClockDrift("plc-002", -0.100)

		// Verify per-device gauges
		val1 := getGaugeValue(t, r.OPCUAClockDrift.WithLabelValues("plc-001"))
		if val1 != 0.350 {
			t.Errorf("expected plc-001 drift 0.350, got %f", val1)
		}

		val2 := getGaugeValue(t, r.OPCUAClockDrift.WithLabelValues("plc-002"))
		if val2 != -0.100 {
			t.Errorf("expected plc-002 drift -0.100, got %f", val2)
		}
	})

	t.Run("RecordOPCUAClockDrift_Update", func(t *testing.T) {
		r.RecordOPCUAClockDrift("plc-003", 0.500)
		r.RecordOPCUAClockDrift("plc-003", 0.010)

		val := getGaugeValue(t, r.OPCUAClockDrift.WithLabelValues("plc-003"))
		if val != 0.010 {
			t.Errorf("expected updated drift 0.010, got %f", val)
		}
	})
}

// getGaugeValue extracts the current value from a Prometheus Gauge.
func getGaugeValue(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	m := &dto.Metric{}
	if err := g.Write(m); err != nil {
		t.Fatalf("failed to read gauge: %v", err)
	}
	return m.GetGauge().GetValue()
}

// getCounterValue extracts the current value from a Prometheus Counter.
func getCounterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	m := &dto.Metric{}
	type writer interface {
		Write(*dto.Metric) error
	}
	if w, ok := c.(writer); ok {
		if err := w.Write(m); err != nil {
			t.Fatalf("failed to read counter: %v", err)
		}
	}
	return m.GetCounter().GetValue()
}
