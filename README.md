
# Connector Gateway (Protocol Gateway) — Quick Start

This repo runs a small dev stack with:

- **Protocol Gateway** (Go)
- **EMQX** (MQTT broker)
- **OPC UA Simulator** (Python `asyncua`) for local testing

## Prerequisites

- Docker Desktop (Compose v2)

## 1) Clone

```bash
git clone https://github.com/AlexandeC3U/ProtocolGateway
cd Connector_Gateway
```

## 2) Start the dev stack

This starts **EMQX**, the **OPC UA simulator**, and the **gateway**.

```bash
docker compose -f docker-compose.yaml up --build
```

Open:

- Web UI: `http://localhost:8080`
- EMQX dashboard: `http://localhost:18083`

Stop:

```bash
docker compose -f docker-compose.yaml down
```

## 3) Add an OPC UA device (simulator) in the Web UI

In the Web UI, go to **Devices** → **Add Device** and use:

- **Protocol**: `opcua`
- **Device ID**: e.g. `SIM2`
- **Name**: e.g. `OPCUA-SIM`
- **Enabled**: `true`
- **UNS Prefix**: `plant1/area1/line1`
- **Poll interval**: `5s`

Connection:

- **OPC Endpoint URL**: `opc.tcp://opcua-simulator:4840`
- **Security Policy/Mode/Auth**: leave empty / defaults (simulator is **NoSecurity**)

### Add tags (4 demo nodes)

Add 4 tags and make sure each tag has a **unique** `topic_suffix`.

1) Temperature

- **Name**: `Temperature`
- **Data type**: `float32` or `float64`
- **Unit**: `°C`
- **Topic suffix**: `temperature`
- **OPC Node ID**: `ns=2;s=Demo.Temperature`

2) Pressure

- **Name**: `Pressure`
- **Data type**: `float32` or `float64`
- **Topic suffix**: `pressure`
- **OPC Node ID**: `ns=2;s=Demo.Pressure`

3) Status

- **Name**: `Status`
- **Data type**: `string`
- **Topic suffix**: `status`
- **OPC Node ID**: `ns=2;s=Demo.Status`

4) Switch

- **Name**: `Switch`
- **Data type**: `bool`
- **Topic suffix**: `switch`
- **OPC Node ID**: `ns=2;s=Demo.Switch`

Expected MQTT topics:

- `plant1/area1/line1/temperature`
- `plant1/area1/line1/pressure`
- `plant1/area1/line1/status`
- `plant1/area1/line1/switch`

Payload format:

```json
{"v": 20.1, "u": "°C", "q": "good", "ts": 1769445124645}
```

## 4) Verify messages in MQTT

Subscribe with any MQTT client to `plant1/#` on `localhost:1883`.

If you use **MQTT Explorer**, subscribe to `plant1/#` (not `#`). In this setup, subscribing to `#` is denied by broker access control.

Example with `mosquitto_sub`:

```bash
mosquitto_sub -h localhost -p 1883 -t 'plant1/#' -v
```
## 5) Testing Write Commands

You can write values to tags using the EMQX dashboard or any MQTT client.

### Using EMQX Dashboard

1. Open the EMQX dashboard: `http://localhost:18083` (default login: `admin` / `public`)
2. Go to **Diagnose** → **WebSocket Client**
3. Click **Connect**
4. To publish a write command:
   - **Topic**: `$nexus/cmd/{device_id}/{tag_id}/set`
   - **Payload**: The value to write

### Example: Writing to the Switch tag

- **Topic**: `$nexus/cmd/SIM/tag-1769444077413/set`
- **Payload**: `true`, `True`, `1`, `false`, `False`, or `0`

For numeric tags, send the value directly (e.g., `25.5` for temperature).

### Write Response

The gateway publishes a response to `$nexus/cmd/{device_id}/{tag_id}/response`:

```json
{"device_id":"SIM","tag_id":"tag-1769444077413","success":true,"timestamp":"2026-01-27T12:00:00Z","duration_ms":45}
```## 6) Health & Metrics Endpoints

The gateway exposes several HTTP endpoints for monitoring and observability on port **8080**.

### Health Endpoints

| Endpoint | Description | Use Case |
|----------|-------------|----------|
| `/health` | Full health status with all component checks | Detailed diagnostics |
| `/health/live` | Liveness probe (is the service running?) | Kubernetes liveness probe |
| `/health/ready` | Readiness probe (is the service ready to accept traffic?) | Kubernetes readiness probe |
| `/status` | Basic service status with polling statistics | Quick status check |

#### Example: Full Health Check

```bash
curl http://localhost:8080/health | jq
```

Response:
```json
{
  "status": "healthy",
  "state": "running",
  "service": "protocol-gateway",
  "version": "1.0.0",
  "timestamp": "2026-01-29T10:00:00Z",
  "checks": {
    "mqtt": { "name": "mqtt", "status": "healthy", "severity": "critical" },
    "modbus_pool": { "name": "modbus_pool", "status": "healthy", "severity": "warning" },
    "opcua_pool": { "name": "opcua_pool", "status": "healthy", "severity": "warning" },
    "s7_pool": { "name": "s7_pool", "status": "healthy", "severity": "warning" }
  }
}
```

#### Example: Liveness Probe

```bash
curl -w "%{http_code}" http://localhost:8080/health/live
# Returns 200 if alive, 503 if not
```

#### Example: Readiness Probe

```bash
curl -w "%{http_code}" http://localhost:8080/health/ready
# Returns 200 if ready, 503 if not ready
```

#### Example: Status Endpoint

```bash
curl http://localhost:8080/status | jq
```

Response:
```json
{
  "service": "protocol-gateway",
  "version": "1.0.0",
  "polling": {
    "total_polls": 1234,
    "success_polls": 1200,
    "failed_polls": 34,
    "skipped_polls": 0,
    "points_read": 4800,
    "points_published": 4800
  }
}
```

### Metrics Endpoint (Prometheus)

The `/metrics` endpoint exposes Prometheus-compatible metrics for monitoring dashboards (Grafana, etc.).

```bash
curl http://localhost:8080/metrics
```

#### Key Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `gateway_connections_active` | Gauge | Active connections per protocol |
| `gateway_connections_attempts_total` | Counter | Total connection attempts |
| `gateway_connections_errors_total` | Counter | Total connection errors |
| `gateway_polling_polls_total` | Counter | Total poll operations (by device/status) |
| `gateway_polling_polls_skipped_total` | Counter | Polls skipped due to back-pressure |
| `gateway_polling_duration_seconds` | Histogram | Poll cycle duration |
| `gateway_polling_points_read_total` | Counter | Data points read |
| `gateway_polling_points_published_total` | Counter | Data points published |
| `gateway_mqtt_messages_published_total` | Counter | MQTT messages published |
| `gateway_mqtt_messages_failed_total` | Counter | Failed MQTT publishes |
| `gateway_mqtt_reconnects_total` | Counter | MQTT reconnection attempts |

#### Example Prometheus Query

```promql
# Poll success rate over last 5 minutes
rate(gateway_polling_polls_total{status="success"}[5m])

# Active connections by protocol
gateway_connections_active

# MQTT publish error rate
rate(gateway_mqtt_messages_failed_total[5m])
```

### Using with Docker Compose Health Checks

The `docker-compose.yaml` is pre-configured to use the liveness endpoint:

```yaml
healthcheck:
  test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8080/health/live"]
  interval: 10s
  timeout: 5s
  retries: 3
```

## 7) Prometheus & Grafana Monitoring

The docker-compose stack includes **Prometheus** for metrics collection and optionally **Grafana** for visualization.

### Starting with Prometheus (default)

Prometheus starts automatically with the stack:

```bash
docker compose up -d
```

Access Prometheus UI: `http://localhost:9090`

### Starting with Grafana (optional)

To include Grafana dashboards, use the `monitoring` profile:

```bash
docker compose --profile monitoring up -d
```

Access:
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000` (login: `admin` / `admin`)

### Verifying Prometheus Targets

1. Open Prometheus UI: `http://localhost:9090`
2. Go to **Status → Targets**
3. Verify `protocol-gateway` target shows **UP**

### Querying Metrics

In the Prometheus UI, try these example queries:

```promql
# Total MQTT messages published
gateway_mqtt_messages_published_total

# Data points read (increasing counter)
gateway_polling_points_read_total

# Active connections by protocol
gateway_connections_active

# Poll success rate over last 5 minutes
rate(gateway_polling_polls_total{status="success"}[5m])

# Poll duration 95th percentile
histogram_quantile(0.95, rate(gateway_polling_duration_seconds_bucket[5m]))

# MQTT publish latency
gateway_mqtt_publish_latency_seconds
```

### Prometheus Configuration

The Prometheus configuration is in `config/prometheus.yml`. It scrapes:
- **Gateway metrics** from `gateway:8080/metrics` every 15s
- **EMQX metrics** from `emqx:18083` (if enabled)

To modify scrape intervals or add alerting rules, edit this file and restart Prometheus.

### Available Metrics Summary

| Category | Metrics |
|----------|---------|
| **Connections** | `gateway_connections_active`, `gateway_connections_attempts_total`, `gateway_connections_errors_total`, `gateway_connections_latency_seconds` |
| **Polling** | `gateway_polling_polls_total`, `gateway_polling_polls_skipped_total`, `gateway_polling_duration_seconds`, `gateway_polling_points_read_total`, `gateway_polling_points_published_total` |
| **MQTT** | `gateway_mqtt_messages_published_total`, `gateway_mqtt_messages_failed_total`, `gateway_mqtt_buffer_size`, `gateway_mqtt_publish_latency_seconds`, `gateway_mqtt_reconnects_total` |
| **Devices** | `gateway_devices_registered`, `gateway_devices_online`, `gateway_device_errors_total` |

## Notes / Troubleshooting

- If you restart the stack and previously created devices don’t show up, ensure the gateway is using the same mounted `config/devices.yaml` (Compose does this by default).
- If you previously published with missing `topic_suffix`, your MQTT client may still show old “stale” topics; reconnect the client to clear them.

