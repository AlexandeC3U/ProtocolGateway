// Package metrics provides Prometheus metrics for the Protocol Gateway.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Registry holds all Prometheus metrics for the service.
type Registry struct {
	// Connection metrics
	ActiveConnections prometheus.Gauge
	ConnectionsTotal  prometheus.Counter
	ConnectionErrors  prometheus.Counter
	ConnectionLatency prometheus.Histogram

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

	// System metrics
	GoroutineCount prometheus.Gauge
	MemoryUsage    prometheus.Gauge
}

// NewRegistry creates a new metrics registry with all metrics registered.
func NewRegistry() *Registry {
	r := &Registry{
		// Connection metrics
		ActiveConnections: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "gateway",
			Subsystem: "modbus",
			Name:      "active_connections",
			Help:      "Number of active Modbus connections",
		}),
		ConnectionsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "gateway",
			Subsystem: "modbus",
			Name:      "connections_total",
			Help:      "Total number of Modbus connection attempts",
		}),
		ConnectionErrors: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "gateway",
			Subsystem: "modbus",
			Name:      "connection_errors_total",
			Help:      "Total number of Modbus connection errors",
		}),
		ConnectionLatency: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: "gateway",
			Subsystem: "modbus",
			Name:      "connection_latency_seconds",
			Help:      "Modbus connection establishment latency",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}),

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

// RecordConnection records a connection event.
func (r *Registry) RecordConnection(success bool, latency float64) {
	r.ConnectionsTotal.Inc()
	if !success {
		r.ConnectionErrors.Inc()
	}
	r.ConnectionLatency.Observe(latency)
}

// UpdateDeviceCount updates the device count gauges.
func (r *Registry) UpdateDeviceCount(registered, online int) {
	r.DevicesRegistered.Set(float64(registered))
	r.DevicesOnline.Set(float64(online))
}

// UpdateActiveConnections updates the active connections gauge.
func (r *Registry) UpdateActiveConnections(count int) {
	r.ActiveConnections.Set(float64(count))
}
