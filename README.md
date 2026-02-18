# Protocol Gateway

![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-2496ED?logo=docker&logoColor=white)
![MQTT](https://img.shields.io/badge/MQTT-EMQX_5.5-660066)
![Prometheus](https://img.shields.io/badge/Prometheus-E6522C?logo=prometheus&logoColor=white)
![Grafana](https://img.shields.io/badge/Grafana-F46800?logo=grafana&logoColor=white)
![OPC UA](https://img.shields.io/badge/OPC_UA-00539B)
![Modbus](https://img.shields.io/badge/Modbus_TCP/RTU-4B8BBE)
![Siemens S7](https://img.shields.io/badge/Siemens_S7-009999)

An industrial-grade protocol gateway that bridges **Modbus TCP/RTU**, **OPC UA**, and **Siemens S7** devices to **MQTT** following the **Unified Namespace (UNS)** pattern. Built in Go with connection pooling, circuit breakers, and full observability.

```
  Industrial Devices              Protocol Gateway                  IT Infrastructure
 ┌──────────────────┐          ┌───────────────────────┐          ┌──────────────────┐
 │  Modbus TCP/RTU  │─────────>│  Polling Engine       │─────────>│   MQTT Broker    │
 │  OPC UA Servers  │─────────>│  Protocol Adapters    │          │   (EMQX)         │
 │  Siemens S7 PLCs │─────────>│  Circuit Breakers     │<─────────│   Write Commands │
 └──────────────────┘          │  Web UI  :8080        │          └──────────────────┘
                               │  Metrics :8080/metrics│────────> Prometheus + Grafana
                               └───────────────────────┘
```

**Key features:** Per-device polling with worker pools | Batch read optimization | Per-endpoint OPC UA session sharing | MQTT buffering during disconnects | Bidirectional write commands | Web UI for device management | Prometheus metrics & Grafana dashboards | Kubernetes-ready health probes

> For detailed architecture documentation, see [docs/](docs/INDEX.md).

---

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) with Compose v2

---

## Quick Start

### 1. Clone & start

```bash
git clone https://github.com/AlexandeC3U/ProtocolGateway
cd Connector_Gateway
docker compose up --build
```

### 2. Open the dashboards

| Service | URL | Credentials |
|---------|-----|-------------|
| **Gateway Web UI** | http://localhost:8080 | - |
| **EMQX Dashboard** | http://localhost:18083 | `admin` / `public` |
| **Prometheus** | http://localhost:9090 | - |
| **Grafana** | http://localhost:3000 | `admin` / `admin` |

### 3. Stop

```bash
docker compose down        # keeps data volumes
docker compose down -v     # removes data volumes
```

---

## Adding a Device (OPC UA Simulator Example)

The dev stack includes a Python OPC UA simulator with demo variables. In the Web UI (**Devices** > **Add Device**):

| Field | Value |
|-------|-------|
| Protocol | `opcua` |
| Device ID | `SIM` |
| Name | `OPC-UA Simulator` |
| Enabled | `true` |
| UNS Prefix | `plant1/area1/line1` |
| Poll Interval | `5s` |
| OPC Endpoint URL | `opc.tcp://opcua-simulator:4840` |
| Security / Auth | Leave defaults (simulator uses NoSecurity) |

### Demo tags

Add the following tags (each needs a unique `topic_suffix`):

| Name | Data Type | Topic Suffix | OPC Node ID |
|------|-----------|-------------|-------------|
| Temperature | `float64` | `temperature` | `ns=2;s=Demo.Temperature` |
| Pressure | `float64` | `pressure` | `ns=2;s=Demo.Pressure` |
| Status | `string` | `status` | `ns=2;s=Demo.Status` |
| Switch | `bool` | `switch` | `ns=2;s=Demo.Switch` |

### Verify MQTT messages

Subscribe with any MQTT client to `plant1/#` on `localhost:1883`:

```bash
mosquitto_sub -h localhost -p 1883 -t 'plant1/#' -v
```

Expected payload format:

```json
{"v": 20.1, "u": "°C", "q": "good", "ts": 1769445124645}
```

> **Tip:** If using MQTT Explorer, subscribe to `plant1/#` (not `#`). Subscribing to `#` may be denied by broker access control.

---

## Write Commands

The gateway supports bidirectional communication. Write values to device tags via MQTT:

**Topic pattern:** `$nexus/cmd/{device_id}/{tag_id}/set`

**Example** (toggle the Switch tag):

```bash
mosquitto_pub -h localhost -p 1883 -t '$nexus/cmd/SIM/switch/set' -m 'true'
```

Accepted boolean values: `true`, `True`, `1`, `false`, `False`, `0`
For numeric tags, send the value directly (e.g., `25.5`).

**Response** (published to `$nexus/cmd/{device_id}/{tag_id}/response`):

```json
{"device_id": "SIM", "tag_id": "switch", "success": true, "timestamp": "2026-01-27T12:00:00Z", "duration_ms": 45}
```

You can also send write commands from the EMQX Dashboard: **Diagnose** > **WebSocket Client** > **Connect** > publish to the command topic.

---

## Health & Monitoring

### Health endpoints

| Endpoint | Purpose | Usage |
|----------|---------|-------|
| `GET /health` | Full health status with component checks | Detailed diagnostics |
| `GET /health/live` | Liveness probe | Kubernetes `livenessProbe` |
| `GET /health/ready` | Readiness probe | Kubernetes `readinessProbe` |
| `GET /status` | Polling statistics | Quick status check |

<details>
<summary>Example: Full health check response</summary>

```bash
curl http://localhost:8080/health | jq
```

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

</details>

### Prometheus metrics

The `/metrics` endpoint exposes Prometheus-compatible metrics:

| Category | Key Metrics |
|----------|-------------|
| **Connections** | `gateway_connections_active`, `gateway_connections_errors_total`, `gateway_connections_latency_seconds` |
| **Polling** | `gateway_polling_polls_total`, `gateway_polling_duration_seconds`, `gateway_polling_points_read_total` |
| **MQTT** | `gateway_mqtt_messages_published_total`, `gateway_mqtt_messages_failed_total`, `gateway_mqtt_buffer_size` |
| **Devices** | `gateway_devices_registered`, `gateway_devices_online` |

<details>
<summary>Example PromQL queries</summary>

```promql
# Poll success rate over last 5 minutes
rate(gateway_polling_polls_total{status="success"}[5m])

# Active connections by protocol
gateway_connections_active

# MQTT publish error rate
rate(gateway_mqtt_messages_failed_total[5m])

# Poll duration 95th percentile
histogram_quantile(0.95, rate(gateway_polling_duration_seconds_bucket[5m]))
```

</details>

### Verifying Prometheus targets

1. Open http://localhost:9090
2. Go to **Status** > **Targets**
3. Verify `protocol-gateway` target shows **UP**

Prometheus config is in `config/prometheus.yml`. It scrapes the gateway every 15s.

---

## Docker Compose Stack

| Service | Image | Ports | Purpose |
|---------|-------|-------|---------|
| `emqx` | `emqx/emqx:5.5` | 1883, 8083, 8883, 18083 | MQTT broker |
| `opcua-simulator` | Local build | 4840 | OPC UA test server |
| `gateway` | Local build | 8080 | Protocol Gateway |
| `prometheus` | `prom/prometheus:v2.50.1` | 9090 | Metrics collection |
| `grafana` | `grafana/grafana:10.3.3` | 3000 | Metrics visualization |

All services run on a shared `protocol-gateway-net` bridge network. Named volumes persist data across restarts.

---

## Project Structure

```
Connector_Gateway/
├── cmd/gateway/main.go              # Entry point
├── internal/
│   ├── domain/                      # Core types: Device, Tag, DataPoint, Protocol
│   ├── adapter/
│   │   ├── modbus/                  # Modbus TCP/RTU + batch optimization
│   │   ├── opcua/                   # OPC UA + session sharing + load shaping
│   │   ├── s7/                      # Siemens S7 (ISO-on-TCP)
│   │   └── mqtt/                    # MQTT publisher with buffering
│   ├── service/
│   │   ├── polling.go               # Polling engine (worker pool)
│   │   └── command_handler.go       # MQTT -> device write commands
│   ├── api/                         # REST API + Web UI handlers
│   ├── health/                      # Health checks with flapping protection
│   └── metrics/                     # Prometheus metrics registry
├── config/
│   ├── config.yaml                  # Gateway configuration
│   ├── devices.yaml                 # Device & tag definitions
│   └── prometheus.yml               # Prometheus scrape config
├── web/index.html                   # Single-page Web UI
├── tools/opcua-simulator/           # Python OPC UA simulator
├── docs/                            # Architecture & detailed documentation
├── testing/                         # Unit, integration, benchmark, fuzz tests
├── Dockerfile                       # Multi-stage: golang:1.22 -> alpine:3.19
├── docker-compose.yaml              # Dev stack
└── docker-compose.test.yaml         # Test simulators stack
```

---

## Documentation

| Document | Description |
|----------|-------------|
| [Architecture Overview](docs/INDEX.md) | High-level architecture, tech stack, data flows |
| [Gateway Service](docs/gateway-service.md) | Polling engine, command handler, REST API, health checks |
| [Protocol Adapters](docs/protocol-adapters.md) | Modbus, OPC UA, S7, MQTT adapter internals |
| [Docker & Infrastructure](docs/docker-infrastructure.md) | Container architecture, Prometheus, Grafana, simulators |
| [ARCHITECTURE.md](docs/ARCHITECTURE.md) | Complete technical reference (~3500 lines) |

---

## Troubleshooting

| Problem | Solution |
|---------|----------|
| Devices don't persist after restart | Ensure `config/devices.yaml` is bind-mounted (default in Compose) |
| Stale MQTT topics showing | Reconnect your MQTT client to clear retained messages |
| `#` wildcard subscription denied | Use specific prefix like `plant1/#` instead |
| Gateway exits on startup | EMQX must be healthy first. Check `docker compose logs emqx` |
| OPC UA connection fails | Verify endpoint URL uses Docker hostname (`opcua-simulator`, not `localhost`) |
