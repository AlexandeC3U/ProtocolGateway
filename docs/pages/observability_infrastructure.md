- [10. Observability Infrastructure](#10-observability-infrastructure)
  - [10.1 Metrics Architecture](#101-metrics-architecture)
    - [10.1.1 Protocol-Specific Metrics](#1011-protocol-specific-metrics)
    - [10.1.2 PromQL Query Examples](#1012-promql-query-examples)
    - [10.1.3 Alerting Examples](#1013-alerting-examples)
    - [10.1.4 Grafana Dashboards](#1014-grafana-dashboards)
    - [10.1.5 Metric Collection Best Practices](#1015-metric-collection-best-practices)
  - [10.2 Structured Logging](#102-structured-logging)
  - [10.3 Health Check System](#103-health-check-system)
  - [10.4 Time Synchronization & Clock Drift](#104-time-synchronization--clock-drift)

## 10. Observability Infrastructure

### 10.1 Metrics Architecture

Prometheus metrics enable operational dashboards (Grafana) and alerting. The diagram catalogs all exposed metrics grouped by subsystem (connections, polling, MQTT, devices, system), including metric types (`Gauge`, `Counter`, `Histogram`), label dimensions, and purpose. These metrics support capacity planning, troubleshooting, and SLA monitoring:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                          PROMETHEUS METRICS REGISTRY                           │
│                                                                                │
│  Endpoint: GET /metrics                                                        │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                      CONNECTION METRICS                                 │   │
│  │                                                                         │   │
│  │  gateway_connections_active{protocol="modbus-tcp|opcua|s7"}             │   │
│  │    → Gauge: Current active connections per protocol                     │   │
│  │    → Purpose: Capacity planning, connection leak detection              │   │
│  │                                                                         │   │
│  │  gateway_connections_attempts_total{protocol, status="success|failure"} │   │
│  │    → Counter: Total connection attempts                                 │   │
│  │    → Purpose: Track connection reliability per protocol                 │   │
│  │                                                                         │   │
│  │  gateway_connections_errors_total{protocol, error_type}                 │   │
│  │    → Counter: Connection errors by type                                 │   │
│  │    → Purpose: Identify systematic connectivity issues                   │   │
│  │                                                                         │   │
│  │  gateway_connections_latency_seconds{protocol}                          │   │
│  │    → Histogram: Connection establishment time                           │   │
│  │    → Buckets: 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10                 │   │
│  │    → Purpose: Track network latency, identify slow endpoints            │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                        POLLING METRICS                                  │   │
│  │                                                                         │   │
│  │  gateway_polling_polls_total{device_id, status="success|failure"}       │   │
│  │    → Counter: Total poll cycles per device                              │   │
│  │    → Purpose: Track device-level reliability                            │   │
│  │                                                                         │   │
│  │  gateway_polling_polls_skipped_total                                    │   │
│  │    → Counter: Skipped polls (global, no per-device labels)              │   │
│  │    → Purpose: Identify overload conditions                              │   │
│  │                                                                         │   │
│  │  gateway_polling_duration_seconds{device_id, protocol}                  │   │
│  │    → Histogram: Poll cycle duration                                     │   │
│  │    → Purpose: Identify slow devices, optimize timeouts                  │   │
│  │                                                                         │   │
│  │  gateway_polling_points_read_total                                      │   │
│  │    → Counter: Total data points successfully read (global)              │   │
│  │    → Purpose: Throughput measurement                                    │   │
│  │                                                                         │   │
│  │  gateway_polling_points_published_total                                 │   │
│  │    → Counter: Total data points published to MQTT (global)              │   │
│  │    → Purpose: End-to-end throughput verification                        │   │
│  │                                                                         │   │
│  │  gateway_polling_worker_pool_utilization                                │   │
│  │    → Gauge: Worker pool utilization (0.0 - 1.0)                         │   │
│  │    → Purpose: Capacity planning, back-pressure indicator                │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                          MQTT METRICS                                   │   │
│  │                                                                         │   │
│  │  gateway_mqtt_messages_published_total                                  │   │
│  │    → Counter: Total messages published                                  │   │
│  │    → Purpose: Track publish throughput                                  │   │
│  │                                                                         │   │
│  │  gateway_mqtt_messages_failed_total                                     │   │
│  │    → Counter: Failed publish attempts                                   │   │
│  │    → Purpose: Track MQTT reliability                                    │   │
│  │                                                                         │   │
│  │  gateway_mqtt_buffer_size                                               │   │
│  │    → Gauge: Current buffer occupancy                                    │   │
│  │    → Purpose: Back-pressure indicator, buffer overflow warning          │   │
│  │                                                                         │   │
│  │  gateway_mqtt_publish_latency_seconds                                   │   │
│  │    → Histogram: Publish latency                                         │   │
│  │    → Purpose: Track end-to-end latency                                  │   │
│  │                                                                         │   │
│  │  gateway_mqtt_reconnects_total                                          │   │
│  │    → Counter: Broker reconnection attempts                              │   │
│  │    → Purpose: Track connection stability                                │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                         DEVICE METRICS                                  │   │
│  │                                                                         │   │
│  │  gateway_devices_registered                                             │   │
│  │    → Gauge: Total registered devices                                    │   │
│  │    → Purpose: Configuration tracking                                    │   │
│  │                                                                         │   │
│  │  gateway_devices_online                                                 │   │
│  │    → Gauge: Currently connected devices                                 │   │
│  │    → Purpose: Availability tracking                                     │   │
│  │                                                                         │   │
│  │  gateway_devices_errors_total{device_id, error_type}                    │   │
│  │    → Counter: Device-specific errors                                    │   │
│  │    → Purpose: Identify problematic devices                              │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                         SYSTEM METRICS                                  │   │
│  │                                                                         │   │
│  │  gateway_system_goroutines                                              │   │
│  │    → Gauge: Current goroutine count                                     │   │
│  │    → Purpose: Goroutine leak detection                                  │   │
│  │                                                                         │   │
│  │  gateway_system_memory_bytes                                            │   │
│  │    → Gauge: Current memory usage                                        │   │
│  │    → Purpose: Memory leak detection, capacity planning                  │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

#### 10.1.1 Protocol-Specific Metrics

**S7 (Siemens PLC) Metrics:**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_s7_device_connected` | Gauge | `device_id` | Connection state (1=connected, 0=disconnected) |
| `gateway_s7_tag_errors_total` | Counter | `device_id`, `tag_id` | Tag read/write errors by device and tag |
| `gateway_s7_read_duration_seconds` | Histogram | `device_id` | Read operation latency per device |
| `gateway_s7_write_duration_seconds` | Histogram | `device_id` | Write operation latency per device |
| `gateway_s7_breaker_state` | Gauge | `device_id` | Circuit breaker state: 0=closed, 1=half-open, 2=open |

**OPC UA Metrics:**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_opcua_clock_drift_seconds` | Gauge | `device_id` | Clock drift between OPC UA server and gateway (positive = gateway ahead) |
| `gateway_opcua_certs_total` | Gauge | `store` | Certificate count by store (`trusted` or `rejected`) |
| `gateway_opcua_cert_expiry_days` | Gauge | `fingerprint`, `subject` | Days until certificate expiry (negative = expired) |

> **Note — Missing Modbus Metrics:** The gateway currently lacks Modbus-specific metrics. Modbus devices are monitored through generic polling and connection metrics. Recommended additions for future implementation: `gateway_modbus_device_connected`, `gateway_modbus_read_duration_seconds`, `gateway_modbus_exception_total`, `gateway_modbus_crc_errors_total`, `gateway_modbus_timeout_total`.

#### 10.1.2 PromQL Query Examples

```promql
# Poll success rate by device (%)
100 * sum by (device_id) (rate(gateway_polling_polls_total{status="success"}[5m]))
    / sum by (device_id) (rate(gateway_polling_polls_total[5m]))

# Connection success rate (%)
100 * (1 - rate(gateway_connections_errors_total[5m]) / rate(gateway_connections_attempts_total[5m]))

# MQTT success rate (%)
100 * (1 - rate(gateway_mqtt_messages_failed_total[5m]) / (rate(gateway_mqtt_messages_published_total[5m]) + rate(gateway_mqtt_messages_failed_total[5m])))

# Poll duration p95 by device
histogram_quantile(0.95, sum(rate(gateway_polling_duration_seconds_bucket[5m])) by (le, device_id))

# S7 devices with open circuit breakers
gateway_s7_breaker_state == 2

# OPC UA devices with significant clock drift (>1s)
abs(gateway_opcua_clock_drift_seconds) > 1

# Certificates expiring within 30 days
gateway_opcua_cert_expiry_days < 30 and gateway_opcua_cert_expiry_days > 0

# Data loss rate (reads not published)
rate(gateway_polling_points_read_total[5m]) - rate(gateway_polling_points_published_total[5m])

# Device availability (%)
100 * gateway_devices_online / gateway_devices_registered

# Worker pool saturation warning
gateway_polling_worker_pool_utilization > 0.9
```

#### 10.1.3 Alerting Examples

```yaml
# Critical Alerts
- alert: DeviceOffline
  expr: gateway_devices_online < gateway_devices_registered
  for: 5m
  labels:
    severity: critical

- alert: MQTTBufferBacklog
  expr: gateway_mqtt_buffer_size > 500
  for: 2m
  labels:
    severity: critical

- alert: S7CircuitBreakerOpen
  expr: gateway_s7_breaker_state == 2
  for: 1m
  labels:
    severity: critical

# Warning Alerts
- alert: HighPollLatency
  expr: histogram_quantile(0.95, sum(rate(gateway_polling_duration_seconds_bucket[5m])) by (le)) > 1
  for: 5m
  labels:
    severity: warning

- alert: WorkerPoolSaturated
  expr: gateway_polling_worker_pool_utilization > 0.8
  for: 5m
  labels:
    severity: warning

- alert: CertificateExpiringSoon
  expr: gateway_opcua_cert_expiry_days < 30 and gateway_opcua_cert_expiry_days > 0
  for: 1h
  labels:
    severity: warning
```

#### 10.1.4 Grafana Dashboards

Pre-built dashboards are auto-provisioned from `config/grafana/provisioning/dashboards/json/`:

| Dashboard | UID | Description |
|-----------|-----|-------------|
| Overview | `gateway-overview` | High-level system health and data flow |
| Polling Performance | `gateway-polling` | Poll duration, throughput, and errors |
| MQTT Messaging | `gateway-mqtt` | MQTT publish metrics and reliability |
| Devices & Industrial | `gateway-devices` | Device health, S7 and OPC UA details |
| System Health | `gateway-system` | Resources, connections, and certificates |

#### 10.1.5 Metric Collection Best Practices

1. **Scrape Interval:** Use 15-30 second scrape intervals for most metrics
2. **Retention:** Keep at least 15 days of data for trend analysis
3. **Cardinality:** Monitor label cardinality, especially `device_id` and `tag_id`
4. **Rate Calculations:** Always use `rate()` over `increase()` for rate-based alerts
5. **Histogram Quantiles:** Use `histogram_quantile()` for latency analysis, not averages

### 10.2 Structured Logging

Zero-allocation structured logging via `zerolog` enables high-performance log output without impacting data processing. The diagram shows RFC 5424 log levels, JSON vs. console output formats, and contextual logging helpers (`WithDeviceContext()`, `WithRequestContext()`) that automatically add device/request context. JSON format enables log aggregation and querying in systems like Elasticsearch or Loki:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                          ZEROLOG LOGGING INFRASTRUCTURE                        │
│                                                                                │
│  Package: github.com/rs/zerolog                                                │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                      LOG LEVELS (RFC 5424)                              │   │
│  │                                                                         │   │
│  │  trace  → Ultra-detailed debugging (disabled in production)             │   │
│  │  debug  → Development debugging information                             │   │
│  │  info   → Normal operational messages (default)                         │   │
│  │  warn   → Warning conditions, recoverable errors                        │   │
│  │  error  → Error conditions requiring attention                          │   │
│  │  fatal  → Critical errors causing shutdown                              │   │
│  │  panic  → Programming errors (should never occur in production)         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                      OUTPUT FORMATS                                     │   │
│  │                                                                         │   │
│  │  JSON Format (Production):                                              │   │
│  │  {                                                                      │   │
│  │    "level": "info",                                                     │   │
│  │    "time": "2026-01-29T10:15:30.123456789Z",                            │   │
│  │    "caller": "polling.go:142",                                          │   │
│  │    "service": "protocol-gateway",                                       │   │
│  │    "version": "1.0.0",                                                  │   │
│  │    "device_id": "plc-001",                                              │   │
│  │    "protocol": "modbus-tcp",                                            │   │
│  │    "duration_ms": 45,                                                   │   │
│  │    "message": "Poll cycle completed"                                    │   │
│  │  }                                                                      │   │
│  │                                                                         │   │
│  │  Console Format (Development):                                          │   │
│  │  10:15:30.123 INF Poll cycle completed device_id=plc-001 duration=45ms  │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    CONTEXTUAL LOGGING HELPERS                           │   │
│  │                                                                         │   │
│  │  WithDeviceContext(logger, device) → Adds device_id, protocol fields    │   │
│  │  WithRequestContext(logger, req)   → Adds request_id, client_ip         │   │
│  │  Error(logger, err)                → Adds error message, stack trace    │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Configuration:                                                                │
│  • LOG_LEVEL: trace|debug|info|warn|error (default: info)                      │
│  • LOG_FORMAT: json|console (default: json)                                    │
│  • LOG_OUTPUT: stdout|stderr|<filepath> (default: stdout)                      │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 10.3 Health Check System

The health check system (`internal/health/checker.go`) provides Kubernetes-compatible probes with flapping protection to prevent false alarms during transient issues. The diagram shows HTTP endpoints (`/health`, `/health/live`, `/health/ready`), severity levels affecting overall status, operational state machine transitions, and the flapping protection algorithm that requires 3 consecutive failures before marking unhealthy:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                           HEALTH CHECK ARCHITECTURE                            │
│                                                                                │
│  Implements Kubernetes-compatible health probes with flapping protection.      │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                      HTTP ENDPOINTS                                     │   │
│  │                                                                         │   │
│  │  GET /health       → Full health status with component details          │   │
│  │  GET /health/live  → Liveness probe (process is running)                │   │
│  │  GET /health/ready → Readiness probe (ready to accept traffic)          │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    CHECK SEVERITY LEVELS                                │   │
│  │                                                                         │   │
│  │  SeverityInfo     → Informational, doesn't affect status                │   │
│  │  SeverityWarning  → Marks system as "degraded"                          │   │
│  │  SeverityCritical → Marks system as "unhealthy", blocks readiness       │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                   OPERATIONAL STATES                                    │   │
│  │                                                                         │   │
│  │        starting                                                         │   │
│  │            │                                                            │   │
│  │            ▼                                                            │   │
│  │        running ◄───────────────────┐                                    │   │
│  │            │                       │                                    │   │
│  │            ▼ (warning failures)    │ (recovery threshold met)           │   │
│  │        degraded ──────────────────►│                                    │   │
│  │            │                       │                                    │   │
│  │            ▼                       │                                    │   │
│  │       recovering ─────────────────►┘                                    │   │
│  │            │                                                            │   │
│  │            ▼ (shutdown signal)                                          │   │
│  │     shutting_down                                                       │   │
│  │            │                                                            │   │
│  │            ▼                                                            │   │
│  │         offline                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                   FLAPPING PROTECTION                                   │   │
│  │                                                                         │   │
│  │  Problem: Unstable network causes rapid healthy/unhealthy oscillation   │   │
│  │                                                                         │   │
│  │  Solution:                                                              │   │
│  │  • Failure threshold: 3 consecutive failures to mark unhealthy          │   │
│  │  • Recovery threshold: 2 consecutive successes to mark healthy          │   │
│  │  • Check interval: 10 seconds (configurable)                            │   │
│  │  • Results cached for HTTP handlers (low overhead)                      │   │
│  │                                                                         │   │
│  │  Timeline Example:                                                      │   │
│  │  ✓ ✓ ✗ ✓ ✗ ✗ ✗ ✓ ✓ ✓                                                │   │
│  │  ─────────────┬─────┬─────                                              │   │
│  │               │     └─ Recovery after 2 successes                       │   │
│  │               └─ Unhealthy after 3 failures                             │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Registered Checks:                                                            │
│  • MQTT Publisher: Connection to broker                                        │
│  • Modbus Pool: At least one successful connection                             │
│  • OPC UA Pool: At least one active session                                    │
│  • S7 Pool: At least one connected PLC                                         │
│  • NTP Sync: System clock within acceptable drift threshold                    │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 10.4 Time Synchronization & Clock Drift

Industrial gateways aggregate data from multiple PLCs and OPC UA servers, each with independent clocks. Accurate timestamps are critical for SCADA/MES correlation, historical data analysis, and alarm sequencing. The gateway implements multi-layer time synchronization to detect and report clock drift at both the system and device level.

```
┌───────────────────────────────────────────────────────────────────────────────┐
│                     TIME SYNCHRONIZATION ARCHITECTURE                         │
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐  │
│  │                    WHY TIME MATTERS                                     │  │
│  │                                                                         │  │
│  │  • Timestamp correlation: Data from multiple sources must align         │  │
│  │  • Alarm sequencing: Events must be ordered correctly for root cause    │  │
│  │  • Historical trends: Time-series databases require accurate time       │  │
│  │  • Compliance: Industrial standards require traceability                │  │
│  │  • Stale data detection: Know when readings are too old to trust        │  │
│  └─────────────────────────────────────────────────────────────────────────┘  │
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐  │
│  │                   NTP HEALTH CHECK                                      │  │
│  │                                                                         │  │
│  │  Lightweight SNTP (RFC 5905) client that periodically checks system     │  │
│  │  clock against a configurable NTP server.                               │  │
│  │                                                                         │  │
│  │      ┌─────────┐         SNTP Query          ┌───────────┐              │  │
│  │      │ Gateway │ ──────────────────────────► │ NTP Server│              │  │
│  │      │         │ ◄────────────────────────── │ (pool.ntp)│              │  │
│  │      └────┬────┘         Response            └───────────┘              │  │
│  │           │                                                             │  │
│  │           ▼                                                             │  │
│  │      Calculate offset = (T2 - T1) - RTT/2                               │  │
│  │                                                                         │  │
│  │  Severity: Warning (drift is informational, not blocking)               │  │
│  │  Exposed via: GET /health includes ntp_sync check result                │  │
│  └─────────────────────────────────────────────────────────────────────────┘  │
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐  │
│  │               OPC UA SERVER DRIFT DETECTION                             │  │
│  │                                                                         │  │
│  │  Compares SourceTimestamp from OPC UA data change notifications with    │  │
│  │  gateway's local time.Now() when the notification is received.          │  │
│  │                                                                         │  │
│  │      OPC UA Server                    Gateway                           │  │
│  │      ┌──────────┐                    ┌──────────┐                       │  │
│  │      │ Ts=14:00:│ ── DataChange ──►  │ Tr=14:00:│                       │  │
│  │      │    00.000│    Notification    │    00.150│                       │  │
│  │      └──────────┘                    └──────────┘                       │  │
│  │                                            │                            │  │
│  │      drift = Tr - Ts - expected_latency    │                            │  │
│  │            = 150ms - 50ms = 100ms ahead    │                            │  │
│  │                                            ▼                            │  │
│  │      Exposed as Prometheus gauge per device_id                          │  │
│  └─────────────────────────────────────────────────────────────────────────┘  │
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐  │
│  │                   STALENESS CALCULATION                                 │  │
│  │                                                                         │  │
│  │  DataPoints carry a staleness indicator based on how long since the     │  │
│  │  last update relative to the expected poll interval.                    │  │
│  │                                                                         │  │
│  │  staleness_seconds = time.Since(datapoint.Timestamp)                    │  │
│  │                                                                         │  │
│  │  Threshold Rules:                                                       │  │
│  │  • Fresh: staleness < poll_interval                                     │  │
│  │  • Stale: staleness >= poll_interval (missed at least one update)       │  │
│  │  • Very Stale: staleness >= 3 × poll_interval (connection may be down)  │  │
│  │                                                                         │  │
│  │  Used by: MQTT publisher to skip stale readings, alerting rules         │  │
│  └─────────────────────────────────────────────────────────────────────────┘  │
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐  │
│  │                     PROMETHEUS METRICS                                  │  │
│  │                                                                         │  │
│  │  System-level:                                                          │  │
│  │  • gateway_system_clock_drift_seconds       (gauge)                     │  │
│  │    Labels: none                                                         │  │
│  │    Offset between gateway and NTP server in seconds                     │  │
│  │                                                                         │  │
│  │  • gateway_system_clock_drift_checks_total  (counter)                   │  │
│  │    Labels: result={success|failure}                                     │  │
│  │    Count of NTP check attempts and outcomes                             │  │
│  │                                                                         │  │
│  │  Per-device:                                                            │  │
│  │  • gateway_opcua_clock_drift_seconds        (gauge)                     │  │
│  │    Labels: device_id                                                    │  │
│  │    Drift between specific OPC UA server and gateway                     │  │
│  └─────────────────────────────────────────────────────────────────────────┘  │
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐  │
│  │                     CONFIGURATION                                       │  │
│  │                                                                         │  │
│  │  config.yaml:                                                           │  │
│  │  ┌─────────────────────────────────────────────────────────────────┐    │  │
│  │  │ ntp:                                                            │    │  │
│  │  │   server: "pool.ntp.org"      # NTP server address              │    │  │
│  │  │   check_interval: 5m          # How often to check              │    │  │
│  │  │   warning_threshold: 100ms    # Log warning if drift exceeds    │    │  │
│  │  │   critical_threshold: 1s      # Mark unhealthy if exceeds       │    │  │
│  │  │   timeout: 5s                 # NTP query timeout               │    │  │
│  │  └─────────────────────────────────────────────────────────────────┘    │  │
│  │                                                                         │  │
│  │  Environment Variables:                                                 │  │
│  │  • NTP_SERVER          → ntp.server                                     │  │
│  │  • NTP_CHECK_INTERVAL  → ntp.check_interval                             │  │
│  │  • NTP_WARNING_THRESHOLD → ntp.warning_threshold                        │  │
│  │  • NTP_CRITICAL_THRESHOLD → ntp.critical_threshold                      │  │
│  └─────────────────────────────────────────────────────────────────────────┘  │
│                                                                               │
└───────────────────────────────────────────────────────────────────────────────┘
```

---