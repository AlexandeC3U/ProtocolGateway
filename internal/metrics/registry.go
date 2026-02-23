// Package metrics provides Prometheus metrics for the Protocol Gateway.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Registry holds all Prometheus metrics for the service.
type Registry struct {
	// Connection metrics (protocol-labeled)
	ActiveConnectionsByProtocol *prometheus.GaugeVec
	ConnectionsTotalByProtocol  *prometheus.CounterVec
	ConnectionErrorsByProtocol  *prometheus.CounterVec
	ConnectionLatencyByProtocol *prometheus.HistogramVec

	// Polling metrics
	PollsTotal            *prometheus.CounterVec
	PollsSkipped          prometheus.Counter // Back-pressure skips
	PollDuration          *prometheus.HistogramVec
	PollErrors            *prometheus.CounterVec
	PointsRead            prometheus.Counter
	PointsPublished       prometheus.Counter
	WorkerPoolUtilization prometheus.Gauge // Current workers in use / max workers

	// MQTT metrics
	MQTTMessagesPublished prometheus.Counter
	MQTTMessagesFailed    prometheus.Counter
	MQTTBufferSize        prometheus.Gauge
	MQTTPublishLatency    prometheus.Histogram
	MQTTReconnects        prometheus.Counter

	// Device metrics
	DevicesRegistered prometheus.Gauge
	DevicesOnline     prometheus.Gauge
	DeviceErrors      *prometheus.CounterVec

	// S7-specific metrics
	S7DeviceConnected  *prometheus.GaugeVec
	S7TagErrorsTotal   *prometheus.CounterVec
	S7ReadDuration     *prometheus.HistogramVec
	S7WriteDuration    *prometheus.HistogramVec
	S7BreakerState     *prometheus.GaugeVec

	// Clock drift metrics
	ClockDriftSeconds prometheus.Gauge        // Current NTP offset in seconds
	ClockDriftChecks  *prometheus.CounterVec   // NTP check results by status (success/error)
	OPCUAClockDrift   *prometheus.GaugeVec     // Clock drift between OPC UA server and gateway

	// Certificate metrics
	OPCUACertsTotal   *prometheus.GaugeVec     // Certificate count by store (trusted/rejected)
	OPCUACertExpiry   *prometheus.GaugeVec     // Days until cert expiry, by fingerprint

	// System metrics
	GoroutineCount prometheus.Gauge
	MemoryUsage    prometheus.Gauge
}

// NewRegistry creates a new metrics registry with all metrics registered.
func NewRegistry() *Registry {
	r := &Registry{
		// Connection metrics
		ActiveConnectionsByProtocol: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "gateway",
			Subsystem: "connections",
			Name:      "active",
			Help:      "Number of active connections (all protocols)",
		}, []string{"protocol"}),
		ConnectionsTotalByProtocol: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gateway",
			Subsystem: "connections",
			Name:      "attempts_total",
			Help:      "Total number of connection attempts by protocol",
		}, []string{"protocol"}),
		ConnectionErrorsByProtocol: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gateway",
			Subsystem: "connections",
			Name:      "errors_total",
			Help:      "Total number of connection errors by protocol",
		}, []string{"protocol"}),
		ConnectionLatencyByProtocol: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "gateway",
			Subsystem: "connections",
			Name:      "latency_seconds",
			Help:      "Connection establishment latency by protocol",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"protocol"}),

		// Polling metrics
		PollsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gateway",
			Subsystem: "polling",
			Name:      "polls_total",
			Help:      "Total number of poll operations",
		}, []string{"device_id", "status"}),
		PollsSkipped: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "gateway",
			Subsystem: "polling",
			Name:      "polls_skipped_total",
			Help:      "Total polls skipped due to worker pool back-pressure",
		}),
		PollDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "gateway",
			Subsystem: "polling",
			Name:      "duration_seconds",
			Help:      "Poll cycle duration in seconds (per-device for p95/p99 analysis)",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}, []string{"device_id", "protocol"}),
		PollErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gateway",
			Subsystem: "polling",
			Name:      "errors_total",
			Help:      "Total number of poll errors",
		}, []string{"device_id", "error_type"}),
		PointsRead: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "gateway",
			Subsystem: "polling",
			Name:      "points_read_total",
			Help:      "Total number of data points read",
		}),
		PointsPublished: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "gateway",
			Subsystem: "polling",
			Name:      "points_published_total",
			Help:      "Total number of data points published",
		}),
		WorkerPoolUtilization: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "gateway",
			Subsystem: "polling",
			Name:      "worker_pool_utilization",
			Help:      "Current worker pool utilization (0-1)",
		}),

		// MQTT metrics
		MQTTMessagesPublished: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "gateway",
			Subsystem: "mqtt",
			Name:      "messages_published_total",
			Help:      "Total number of MQTT messages published",
		}),
		MQTTMessagesFailed: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "gateway",
			Subsystem: "mqtt",
			Name:      "messages_failed_total",
			Help:      "Total number of failed MQTT publishes",
		}),
		MQTTBufferSize: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "gateway",
			Subsystem: "mqtt",
			Name:      "buffer_size",
			Help:      "Current MQTT message buffer size",
		}),
		MQTTPublishLatency: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: "gateway",
			Subsystem: "mqtt",
			Name:      "publish_latency_seconds",
			Help:      "MQTT publish latency in seconds",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5},
		}),
		MQTTReconnects: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "gateway",
			Subsystem: "mqtt",
			Name:      "reconnects_total",
			Help:      "Total number of MQTT reconnection attempts",
		}),

		// Device metrics
		DevicesRegistered: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "gateway",
			Subsystem: "devices",
			Name:      "registered",
			Help:      "Number of registered devices",
		}),
		DevicesOnline: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "gateway",
			Subsystem: "devices",
			Name:      "online",
			Help:      "Number of online devices",
		}),
		DeviceErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gateway",
			Subsystem: "devices",
			Name:      "errors_total",
			Help:      "Total device errors by type",
		}, []string{"device_id", "error_type"}),

		// S7-specific metrics
		S7DeviceConnected: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "gateway",
			Subsystem: "s7",
			Name:      "device_connected",
			Help:      "Whether the S7 device is currently connected (1=connected, 0=disconnected)",
		}, []string{"device_id"}),
		S7TagErrorsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gateway",
			Subsystem: "s7",
			Name:      "tag_errors_total",
			Help:      "Total S7 tag read/write errors by device and tag",
		}, []string{"device_id", "tag_id"}),
		S7ReadDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "gateway",
			Subsystem: "s7",
			Name:      "read_duration_seconds",
			Help:      "S7 read operation duration per device",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		}, []string{"device_id"}),
		S7WriteDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "gateway",
			Subsystem: "s7",
			Name:      "write_duration_seconds",
			Help:      "S7 write operation duration per device",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		}, []string{"device_id"}),
		S7BreakerState: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "gateway",
			Subsystem: "s7",
			Name:      "breaker_state",
			Help:      "S7 circuit breaker state per device (0=closed, 1=half-open, 2=open)",
		}, []string{"device_id"}),

		// Clock drift metrics
		ClockDriftSeconds: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "gateway",
			Subsystem: "system",
			Name:      "clock_drift_seconds",
			Help:      "Current NTP clock offset in seconds (positive = gateway ahead, negative = behind)",
		}),
		ClockDriftChecks: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gateway",
			Subsystem: "system",
			Name:      "clock_drift_checks_total",
			Help:      "Total NTP clock drift checks by result",
		}, []string{"status"}),
		OPCUAClockDrift: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "gateway",
			Subsystem: "opcua",
			Name:      "clock_drift_seconds",
			Help:      "Clock drift between OPC UA server and gateway in seconds (positive = gateway ahead)",
		}, []string{"device_id"}),

		// Certificate metrics
		OPCUACertsTotal: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "gateway",
			Subsystem: "opcua",
			Name:      "certs_total",
			Help:      "Number of certificates in the trust store by store type",
		}, []string{"store"}),
		OPCUACertExpiry: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "gateway",
			Subsystem: "opcua",
			Name:      "cert_expiry_days",
			Help:      "Days until certificate expiry (negative = already expired)",
		}, []string{"fingerprint", "subject"}),

		// System metrics
		GoroutineCount: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "gateway",
			Subsystem: "system",
			Name:      "goroutines",
			Help:      "Number of running goroutines",
		}),
		MemoryUsage: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "gateway",
			Subsystem: "system",
			Name:      "memory_bytes",
			Help:      "Memory usage in bytes",
		}),
	}

	return r
}

// RecordPollSuccess records a successful poll operation.
func (r *Registry) RecordPollSuccess(deviceID string, protocol string, duration float64, pointsRead int) {
	r.PollsTotal.WithLabelValues(deviceID, "success").Inc()
	r.PollDuration.WithLabelValues(deviceID, protocol).Observe(duration)
	r.PointsRead.Add(float64(pointsRead))
}

// RecordPollError records a failed poll operation.
func (r *Registry) RecordPollError(deviceID string, errorType string) {
	r.PollsTotal.WithLabelValues(deviceID, "error").Inc()
	r.PollErrors.WithLabelValues(deviceID, errorType).Inc()
}

// RecordPollSkipped records a skipped poll due to back-pressure.
func (r *Registry) RecordPollSkipped() {
	r.PollsSkipped.Inc()
}

// UpdateWorkerPoolUtilization updates the worker pool utilization gauge.
func (r *Registry) UpdateWorkerPoolUtilization(inUse, maxWorkers int) {
	if maxWorkers > 0 {
		r.WorkerPoolUtilization.Set(float64(inUse) / float64(maxWorkers))
	}
}

// RecordMQTTPublish records an MQTT publish operation.
func (r *Registry) RecordMQTTPublish(success bool, latency float64) {
	if success {
		r.MQTTMessagesPublished.Inc()
	} else {
		r.MQTTMessagesFailed.Inc()
	}
	r.MQTTPublishLatency.Observe(latency)
}

// UpdateMQTTBufferSize updates the MQTT buffer size gauge.
func (r *Registry) UpdateMQTTBufferSize(size int) {
	r.MQTTBufferSize.Set(float64(size))
}

// RecordConnectionForProtocol records a connection attempt for a specific protocol.
func (r *Registry) RecordConnectionForProtocol(protocol string, success bool, latency float64) {
	r.ConnectionsTotalByProtocol.WithLabelValues(protocol).Inc()
	if !success {
		r.ConnectionErrorsByProtocol.WithLabelValues(protocol).Inc()
	}
	r.ConnectionLatencyByProtocol.WithLabelValues(protocol).Observe(latency)
}

// UpdateDeviceCount updates the device count gauges.
func (r *Registry) UpdateDeviceCount(registered, online int) {
	r.DevicesRegistered.Set(float64(registered))
	r.DevicesOnline.Set(float64(online))
}

// UpdateActiveConnectionsForProtocol updates the active connection gauge for a specific protocol.
func (r *Registry) UpdateActiveConnectionsForProtocol(protocol string, count int) {
	r.ActiveConnectionsByProtocol.WithLabelValues(protocol).Set(float64(count))
}

// RecordS7DeviceConnected updates the S7 device connection state gauge.
func (r *Registry) RecordS7DeviceConnected(deviceID string, connected bool) {
	val := 0.0
	if connected {
		val = 1.0
	}
	r.S7DeviceConnected.WithLabelValues(deviceID).Set(val)
}

// RecordS7TagError increments the S7 tag error counter.
func (r *Registry) RecordS7TagError(deviceID, tagID string) {
	r.S7TagErrorsTotal.WithLabelValues(deviceID, tagID).Inc()
}

// RecordS7ReadDuration records an S7 read operation duration.
func (r *Registry) RecordS7ReadDuration(deviceID string, duration float64) {
	r.S7ReadDuration.WithLabelValues(deviceID).Observe(duration)
}

// RecordS7WriteDuration records an S7 write operation duration.
func (r *Registry) RecordS7WriteDuration(deviceID string, duration float64) {
	r.S7WriteDuration.WithLabelValues(deviceID).Observe(duration)
}

// RecordS7BreakerState updates the S7 circuit breaker state gauge.
// 0=closed (normal), 1=half-open (probing), 2=open (blocking).
func (r *Registry) RecordS7BreakerState(deviceID string, state int) {
	r.S7BreakerState.WithLabelValues(deviceID).Set(float64(state))
}

// RecordClockDrift records the current NTP clock offset.
func (r *Registry) RecordClockDrift(offsetSeconds float64, success bool) {
	if success {
		r.ClockDriftSeconds.Set(offsetSeconds)
		r.ClockDriftChecks.WithLabelValues("success").Inc()
	} else {
		r.ClockDriftChecks.WithLabelValues("error").Inc()
	}
}

// RecordOPCUAClockDrift records the clock drift between an OPC UA server and the gateway.
func (r *Registry) RecordOPCUAClockDrift(deviceID string, driftSeconds float64) {
	r.OPCUAClockDrift.WithLabelValues(deviceID).Set(driftSeconds)
}

// UpdateOPCUACertCounts updates the certificate count gauges.
func (r *Registry) UpdateOPCUACertCounts(trustedCount, rejectedCount int) {
	r.OPCUACertsTotal.WithLabelValues("trusted").Set(float64(trustedCount))
	r.OPCUACertsTotal.WithLabelValues("rejected").Set(float64(rejectedCount))
}

// RecordOPCUACertExpiry records the days until a certificate expires.
func (r *Registry) RecordOPCUACertExpiry(fingerprint, subject string, daysUntilExpiry int) {
	r.OPCUACertExpiry.WithLabelValues(fingerprint, subject).Set(float64(daysUntilExpiry))
}
