# Protocol Gateway Metrics Reference

This document describes all Prometheus metrics exposed by the Protocol Gateway service. Metrics are available at the `/metrics` endpoint.

## Overview

All metrics use the `gateway` namespace and are organized into the following subsystems:

| Subsystem | Description |
|-----------|-------------|
| `connections` | Protocol connection state and performance |
| `polling` | Data polling operations and throughput |
| `mqtt` | MQTT broker communication |
| `devices` | Device registration and health |
| `s7` | Siemens S7 PLC specific metrics |
| `opcua` | OPC UA protocol specific metrics |
| `system` | Gateway runtime and resource metrics |

---

## Connection Metrics

Metrics tracking protocol connections across all supported protocols (S7, OPC UA, Modbus, MQTT).

### `gateway_connections_active`
**Type:** Gauge  
**Labels:** `protocol`  
**Description:** Number of currently active connections by protocol.

```promql
# Total active connections
sum(gateway_connections_active)

# Active S7 connections
gateway_connections_active{protocol="s7"}
```

### `gateway_connections_attempts_total`
**Type:** Counter  
**Labels:** `protocol`  
**Description:** Total number of connection attempts by protocol.

```promql
# Connection attempt rate by protocol
rate(gateway_connections_attempts_total[5m])
```

### `gateway_connections_errors_total`
**Type:** Counter  
**Labels:** `protocol`  
**Description:** Total number of connection errors by protocol.

```promql
# Connection error rate
rate(gateway_connections_errors_total[5m])

# Connection success rate (%)
100 * (1 - rate(gateway_connections_errors_total[5m]) / rate(gateway_connections_attempts_total[5m]))
```

### `gateway_connections_latency_seconds`
**Type:** Histogram  
**Labels:** `protocol`  
**Buckets:** 10ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s  
**Description:** Connection establishment latency by protocol.

```promql
# p95 connection latency by protocol
histogram_quantile(0.95, sum(rate(gateway_connections_latency_seconds_bucket[5m])) by (le, protocol))
```

---

## Polling Metrics

Metrics tracking the data polling cycle - reading data from industrial devices.

### `gateway_polling_polls_total`
**Type:** Counter  
**Labels:** `device_id`, `status`  
**Description:** Total number of poll operations. Status is either `success` or `error`.

```promql
# Poll rate by device
sum by (device_id) (rate(gateway_polling_polls_total[5m]))

# Poll success rate by device (%)
100 * sum by (device_id) (rate(gateway_polling_polls_total{status="success"}[5m])) 
    / sum by (device_id) (rate(gateway_polling_polls_total[5m]))
```

### `gateway_polling_polls_skipped_total`
**Type:** Counter  
**Description:** Total polls skipped due to worker pool back-pressure. A high value indicates the gateway is overloaded.

```promql
# Skipped polls rate (indicates back-pressure)
rate(gateway_polling_polls_skipped_total[5m])
```

### `gateway_polling_duration_seconds`
**Type:** Histogram  
**Labels:** `device_id`, `protocol`  
**Buckets:** 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s  
**Description:** Poll cycle duration per device. Use for p95/p99 latency analysis.

```promql
# Overall poll duration p95
histogram_quantile(0.95, sum(rate(gateway_polling_duration_seconds_bucket[5m])) by (le))

# Poll duration p95 by device
histogram_quantile(0.95, sum(rate(gateway_polling_duration_seconds_bucket[5m])) by (le, device_id))

# Poll duration p99 by protocol
histogram_quantile(0.99, sum(rate(gateway_polling_duration_seconds_bucket[5m])) by (le, protocol))
```

### `gateway_polling_errors_total`
**Type:** Counter  
**Labels:** `device_id`, `error_type`  
**Description:** Total poll errors by device and error type.

```promql
# Error rate by type
sum by (error_type) (rate(gateway_polling_errors_total[5m]))

# Devices with most errors
topk(5, sum by (device_id) (increase(gateway_polling_errors_total[1h])))
```

### `gateway_polling_points_read_total`
**Type:** Counter  
**Description:** Total number of data points read from devices.

```promql
# Data point read throughput
rate(gateway_polling_points_read_total[5m])
```

### `gateway_polling_points_published_total`
**Type:** Counter  
**Description:** Total number of data points successfully published to MQTT.

```promql
# Data point publish throughput
rate(gateway_polling_points_published_total[5m])

# Data loss rate (reads that weren't published)
rate(gateway_polling_points_read_total[5m]) - rate(gateway_polling_points_published_total[5m])
```

### `gateway_polling_worker_pool_utilization`
**Type:** Gauge  
**Description:** Current worker pool utilization (0.0 - 1.0). Values approaching 1.0 indicate potential back-pressure.

```promql
# Alert when worker pool is saturated
gateway_polling_worker_pool_utilization > 0.9
```

---

## MQTT Metrics

Metrics tracking MQTT broker communication for data publishing.

### `gateway_mqtt_messages_published_total`
**Type:** Counter  
**Description:** Total number of MQTT messages successfully published.

```promql
# Message publish rate
rate(gateway_mqtt_messages_published_total[5m])
```

### `gateway_mqtt_messages_failed_total`
**Type:** Counter  
**Description:** Total number of failed MQTT publish attempts.

```promql
# MQTT success rate (%)
100 * (1 - rate(gateway_mqtt_messages_failed_total[5m]) / (rate(gateway_mqtt_messages_published_total[5m]) + rate(gateway_mqtt_messages_failed_total[5m])))
```

### `gateway_mqtt_buffer_size`
**Type:** Gauge  
**Description:** Current number of messages in the MQTT publish buffer. High values indicate broker connectivity issues.

```promql
# Alert on buffer buildup
gateway_mqtt_buffer_size > 100
```

### `gateway_mqtt_publish_latency_seconds`
**Type:** Histogram  
**Buckets:** 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms  
**Description:** MQTT publish operation latency.

```promql
# MQTT publish latency p95
histogram_quantile(0.95, sum(rate(gateway_mqtt_publish_latency_seconds_bucket[5m])) by (le))
```

### `gateway_mqtt_reconnects_total`
**Type:** Counter  
**Description:** Total number of MQTT broker reconnection attempts.

```promql
# Reconnects per hour (indicates connectivity issues)
increase(gateway_mqtt_reconnects_total[1h])
```

---

## Device Metrics

Metrics tracking device registration and general health.

### `gateway_devices_registered`
**Type:** Gauge  
**Description:** Number of devices registered in the gateway configuration.

### `gateway_devices_online`
**Type:** Gauge  
**Description:** Number of devices currently online and responding.

```promql
# Device availability (%)
100 * gateway_devices_online / gateway_devices_registered
```

### `gateway_devices_errors_total`
**Type:** Counter  
**Labels:** `device_id`, `error_type`  
**Description:** Total device errors by device and error type.

```promql
# Error rate by device
sum by (device_id) (rate(gateway_devices_errors_total[5m]))
```

---

## S7 (Siemens PLC) Metrics

Protocol-specific metrics for Siemens S7 PLC communication.

### `gateway_s7_device_connected`
**Type:** Gauge  
**Labels:** `device_id`  
**Description:** S7 device connection state (1 = connected, 0 = disconnected).

```promql
# Count of connected S7 devices
count(gateway_s7_device_connected == 1)

# Disconnected S7 devices
gateway_s7_device_connected == 0
```

### `gateway_s7_tag_errors_total`
**Type:** Counter  
**Labels:** `device_id`, `tag_id`  
**Description:** Total S7 tag read/write errors by device and tag.

```promql
# Tags with most errors
topk(10, sum by (tag_id) (increase(gateway_s7_tag_errors_total[1h])))

# Error rate by device
sum by (device_id) (rate(gateway_s7_tag_errors_total[5m]))
```

### `gateway_s7_read_duration_seconds`
**Type:** Histogram  
**Labels:** `device_id`  
**Buckets:** 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s  
**Description:** S7 read operation duration per device.

```promql
# S7 read latency p95 by device
histogram_quantile(0.95, sum(rate(gateway_s7_read_duration_seconds_bucket[5m])) by (le, device_id))
```

### `gateway_s7_write_duration_seconds`
**Type:** Histogram  
**Labels:** `device_id`  
**Buckets:** 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s  
**Description:** S7 write operation duration per device.

```promql
# S7 write latency p95 by device
histogram_quantile(0.95, sum(rate(gateway_s7_write_duration_seconds_bucket[5m])) by (le, device_id))
```

### `gateway_s7_breaker_state`
**Type:** Gauge  
**Labels:** `device_id`  
**Description:** S7 circuit breaker state per device:
- `0` = CLOSED (normal operation)
- `1` = HALF-OPEN (probing after failure)
- `2` = OPEN (blocking requests due to repeated failures)

```promql
# Devices with open circuit breakers (degraded)
gateway_s7_breaker_state == 2

# Devices in half-open state (recovering)
gateway_s7_breaker_state == 1
```

---

## OPC UA Metrics

Protocol-specific metrics for OPC UA communication and certificate management.

### `gateway_opcua_clock_drift_seconds`
**Type:** Gauge  
**Labels:** `device_id`  
**Description:** Clock drift between OPC UA server and gateway in seconds. Positive values mean gateway is ahead.

```promql
# Devices with significant clock drift (>1s)
abs(gateway_opcua_clock_drift_seconds) > 1

# Average clock drift across all OPC UA devices
avg(abs(gateway_opcua_clock_drift_seconds))
```

### `gateway_opcua_certs_total`
**Type:** Gauge  
**Labels:** `store`  
**Description:** Number of certificates in the OPC UA trust store. Store is either `trusted` or `rejected`.

```promql
# Rejected certificates (may need attention)
gateway_opcua_certs_total{store="rejected"}
```

### `gateway_opcua_cert_expiry_days`
**Type:** Gauge  
**Labels:** `fingerprint`, `subject`  
**Description:** Days until certificate expiry. Negative values indicate expired certificates.

```promql
# Certificates expiring within 30 days
gateway_opcua_cert_expiry_days < 30

# Expired certificates
gateway_opcua_cert_expiry_days < 0
```

---

## System Metrics

Gateway runtime and resource metrics.

### `gateway_system_clock_drift_seconds`
**Type:** Gauge  
**Description:** Current NTP clock offset in seconds. Positive = gateway ahead, negative = gateway behind.

```promql
# Alert on significant clock drift
abs(gateway_system_clock_drift_seconds) > 2
```

### `gateway_system_clock_drift_checks_total`
**Type:** Counter  
**Labels:** `status`  
**Description:** Total NTP clock drift checks by result (`success` or `error`).

```promql
# NTP check failure rate
rate(gateway_system_clock_drift_checks_total{status="error"}[5m])
```

### `gateway_system_goroutines`
**Type:** Gauge  
**Description:** Number of running goroutines. Useful for detecting goroutine leaks.

```promql
# Alert on goroutine growth
gateway_system_goroutines > 1000
```

### `gateway_system_memory_bytes`
**Type:** Gauge  
**Description:** Current memory usage in bytes.

```promql
# Memory usage in MB
gateway_system_memory_bytes / 1024 / 1024
```

---

## Grafana Dashboards

Pre-built Grafana dashboards are available in `config/grafana/provisioning/dashboards/json/`:

| Dashboard | UID | Description |
|-----------|-----|-------------|
| Overview | `gateway-overview` | High-level system health and data flow |
| Polling Performance | `gateway-polling` | Poll duration, throughput, and errors |
| MQTT Messaging | `gateway-mqtt` | MQTT publish metrics and reliability |
| Devices & Industrial | `gateway-devices` | Device health, S7 and OPC UA details |
| System Health | `gateway-system` | Resources, connections, and certificates |

---

## Missing Protocol-Specific Metrics

> **Note:** The gateway currently lacks Modbus-specific metrics. While Modbus devices are monitored through the generic polling and connection metrics, the following protocol-specific metrics would provide better observability:

### Recommended Modbus Metrics (Not Yet Implemented)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_modbus_device_connected` | Gauge | `device_id` | Connection state (1/0) |
| `gateway_modbus_read_duration_seconds` | Histogram | `device_id`, `register_type` | Read operation latency |
| `gateway_modbus_write_duration_seconds` | Histogram | `device_id`, `register_type` | Write operation latency |
| `gateway_modbus_exception_total` | Counter | `device_id`, `exception_code` | Modbus exception responses |
| `gateway_modbus_crc_errors_total` | Counter | `device_id` | CRC validation errors (RTU) |
| `gateway_modbus_timeout_total` | Counter | `device_id` | Request timeouts |
| `gateway_modbus_registers_read_total` | Counter | `device_id`, `register_type` | Registers read by type |
| `gateway_modbus_coils_written_total` | Counter | `device_id` | Coil write operations |

---

## Alerting Examples

### Critical Alerts

```yaml
# Device offline
- alert: DeviceOffline
  expr: gateway_devices_online < gateway_devices_registered
  for: 5m
  labels:
    severity: critical

# MQTT broker disconnected
- alert: MQTTBufferBacklog
  expr: gateway_mqtt_buffer_size > 500
  for: 2m
  labels:
    severity: critical

# Circuit breaker open
- alert: S7CircuitBreakerOpen
  expr: gateway_s7_breaker_state == 2
  for: 1m
  labels:
    severity: critical
```

### Warning Alerts

```yaml
# High poll latency
- alert: HighPollLatency
  expr: histogram_quantile(0.95, sum(rate(gateway_polling_duration_seconds_bucket[5m])) by (le)) > 1
  for: 5m
  labels:
    severity: warning

# Worker pool saturation
- alert: WorkerPoolSaturated
  expr: gateway_polling_worker_pool_utilization > 0.8
  for: 5m
  labels:
    severity: warning

# Certificate expiring soon
- alert: CertificateExpiringSoon
  expr: gateway_opcua_cert_expiry_days < 30 and gateway_opcua_cert_expiry_days > 0
  for: 1h
  labels:
    severity: warning
```

---

## Metric Collection Best Practices

1. **Scrape Interval:** Use 15-30 second scrape intervals for most metrics
2. **Retention:** Keep at least 15 days of data for trend analysis
3. **Cardinality:** Monitor label cardinality, especially `device_id` and `tag_id`
4. **Rate Calculations:** Always use `rate()` over `increase()` for rate-based alerts
5. **Histogram Quantiles:** Use `histogram_quantile()` for latency analysis, not averages
