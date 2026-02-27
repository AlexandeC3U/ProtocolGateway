- [12. Deployment Architecture](#12-deployment-architecture)
  - [12.1 Container Architecture](#121-container-architecture)
  - [12.2 Docker Compose Architecture](#122-docker-compose-architecture)
    - [12.2.1 Service Details](#1221-service-details)
    - [12.2.2 Volumes](#1222-volumes)
    - [12.2.3 Environment Variables (Gateway)](#1223-environment-variables-gateway)
    - [12.2.4 Port Mapping Summary](#1224-port-mapping-summary)
    - [12.2.5 Test Stack](#1225-test-stack-docker-composetestyaml)
    - [12.2.6 OPC UA Simulator](#1226-opc-ua-simulator)
    - [12.2.7 Prometheus Monitoring](#1227-prometheus-monitoring)
  - [12.3 Kubernetes Deployment (Reference)](#123-kubernetes-deployment-reference)

## 12. Deployment Architecture

### 12.1 Container Architecture

The multi-stage Docker build produces a minimal, secure container image (~25MB). The diagram shows the two-stage process: Builder (Go compilation with static linking) and Runtime (Alpine Linux with non-root user). Security hardening includes stripped debug symbols, read-only filesystem compatibility, and minimal installed packages:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                          DOCKER CONTAINER DESIGN                               │
│                                                                                │
│  Multi-Stage Build (Security & Size Optimization):                             │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  STAGE 1: BUILDER                                                       │   │
│  │                                                                         │   │
│  │  FROM golang:1.22-alpine                                                │   │
│  │                                                                         │   │
│  │  Purpose: Compile static binary                                         │   │
│  │  Packages: git (version info), ca-certificates, tzdata                  │   │
│  │                                                                         │   │
│  │  Build flags:                                                           │   │
│  │  • CGO_ENABLED=0 → Static binary, no libc dependency                    │   │
│  │  • -ldflags="-w -s" → Strip debug info, reduce size                     │   │
│  │  • Version injection via git describe                                   │   │
│  │                                                                         │   │
│  │  Result: ~15MB statically-linked binary                                 │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  STAGE 2: RUNTIME                                                       │   │
│  │                                                                         │   │
│  │  FROM alpine:3.19                                                       │   │
│  │                                                                         │   │
│  │  Minimal runtime image (~5MB base)                                      │   │
│  │                                                                         │   │
│  │  Added packages:                                                        │   │
│  │  • ca-certificates → TLS certificate validation                         │   │
│  │  • tzdata → Timezone support for timestamps                             │   │
│  │  • docker-cli → Container log access feature                            │   │
│  │                                                                         │   │
│  │  Security:                                                              │   │
│  │  • Non-root user: gateway (UID 1000)                                    │   │
│  │  • Read-only filesystem compatible                                      │   │
│  │  • No shell needed (can use scratch base if no docker-cli)              │   │
│  │                                                                         │   │
│  │  Directory structure:                                                   │   │
│  │  /app/                                                                  │   │
│  │  ├── protocol-gateway    (binary)                                       │   │
│  │  ├── config/             (configuration)                                │   │
│  │  └── web/                (static UI files)                              │   │
│  │                                                                         │   │
│  │  Final image size: ~25MB                                                │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Health Check:                                                                 │
│  HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3         │
│    CMD wget -q --spider http://localhost:8080/health/live || exit 1            │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 12.2 Docker Compose Architecture

Docker Compose orchestrates the complete development and production stack. The diagram shows service dependencies, network topology, volume mounts, and health-check-based startup ordering. The production stack includes EMQX broker and gateway, while the development stack adds protocol simulators for testing:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                       DOCKER COMPOSE DEPLOYMENT                                │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    PRODUCTION STACK                                     │   │
│  │                                                                         │   │
│  │  ┌────────────────┐    ┌────────────────┐    ┌────────────────┐         │   │
│  │  │   EMQX 5.5     │    │    Gateway     │    │  OPC UA Sim    │         │   │
│  │  │                │    │                │    │                │         │   │
│  │  │  Port 1883     │◄───│  Port 8080     │───►│  Port 4840     │         │   │
│  │  │  Port 8883     │    │                │    │                │         │   │
│  │  │  Port 18083    │    │                │    │                │         │   │
│  │  │  (Dashboard)   │    │                │    │                │         │   │
│  │  └────────────────┘    └────────────────┘    └────────────────┘         │   │
│  │                                                                         │   │
│  │  Network: protocol-gateway-net (bridge)                                 │   │
│  │                                                                         │   │
│  │  Volumes:                                                               │   │
│  │  • ./config/devices.yaml → /app/config/devices.yaml                     │   │
│  │  • /var/run/docker.sock → Docker CLI access (optional)                  │   │
│  │  • emqx-data → MQTT broker persistence                                  │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    DEVELOPMENT STACK                                    │   │
│  │                                                                         │   │
│  │  Includes all production services plus:                                 │   │
│  │                                                                         │   │
│  │  ┌────────────────┐                                                     │   │
│  │  │  Modbus Sim    │                                                     │   │
│  │  │                │                                                     │   │
│  │  │  Port 5020     │ ← oitc/modbus-server:latest                         │   │
│  │  │                │                                                     │   │
│  │  └────────────────┘                                                     │   │
│  │                                                                         │   │
│  │  Differences from production:                                           │   │
│  │  • LOG_LEVEL=debug (verbose logging)                                    │   │
│  │  • LOG_FORMAT=console (human-readable)                                  │   │
│  │  • devices-dev.yaml (simulator endpoints)                               │   │
│  │  • Network: nexus-dev                                                   │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Dependency Management:                                                        │
│  gateway:                                                                      │
│    depends_on:                                                                 │
│      emqx:                                                                     │
│        condition: service_healthy    ← Wait for MQTT broker                    │
│    restart: unless-stopped           ← Auto-restart on failure                 │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

#### 12.2.1 Service Details

| Service | Image | Ports | Purpose |
|---|---|---|---|
| `emqx` | `emqx/emqx:5.5` | 1883, 8083, 8084, 8883, 18083 | MQTT broker with dashboard |
| `opcua-simulator` | Local build (`tools/opcua-simulator/`) | 4840 | OPC UA test server with demo variables |
| `gateway` | Local build (`.`) | 8080 | The Protocol Gateway |
| `prometheus` | `prom/prometheus:v2.50.1` | 9090 | Metrics collection and storage |
| `grafana` | `grafana/grafana:10.3.3` | 3000 | Metrics visualization dashboards |

> **Gotcha**: `depends_on` only waits for the container to *start* or pass a health check — it doesn't guarantee the service inside is fully ready. The gateway has retry logic for MQTT connections, but the initial `mqttPublisher.Connect()` is a hard requirement: if EMQX isn't healthy, the gateway `Fatal`s out.

#### 12.2.2 Volumes

| Volume | Type | Service | Purpose |
|---|---|---|---|
| `emqx-data` | Named | EMQX | Persist broker data between restarts |
| `emqx-log` | Named | EMQX | Persist broker logs |
| `gateway-data` | Named | Gateway | Application data persistence |
| `prometheus-data` | Named | Prometheus | Metrics storage (7-day retention) |
| `grafana-data` | Named | Grafana | Dashboard and plugin storage |
| `./config/devices.yaml` | Bind mount | Gateway | Device configuration (editable) |
| `./certs/pki` | Bind mount (RO) | Gateway | OPC UA PKI certificates |
| `/var/run/docker.sock` | Bind mount | Gateway | Docker engine access (Web UI logs) |

> Named volumes **persist across `docker compose down`** — your device configs, metrics history, and Grafana dashboards survive restarts. Only `docker compose down -v` removes them.

#### 12.2.3 Environment Variables (Gateway)

| Variable | Default | Description |
|---|---|---|
| `MQTT_BROKER_URL` | `tcp://emqx:1883` | Broker address (uses Docker DNS) |
| `MQTT_CLIENT_ID` | `protocol-gateway-dev` | MQTT client identifier |
| `HTTP_PORT` | `8080` | HTTP server port |
| `LOG_LEVEL` | `debug` | Logging verbosity |
| `LOG_FORMAT` | `console` | Log format (json/console) |
| `DEVICES_CONFIG_PATH` | `/app/config/devices.yaml` | Path to device configuration |
| `ENVIRONMENT` | `development` | Runtime environment |

#### 12.2.4 Port Mapping Summary

| Host Port | Container | Service |
|---|---|---|
| 1883 | emqx:1883 | MQTT TCP |
| 8083 | emqx:8083 | MQTT WebSocket |
| 8883 | emqx:8883 | MQTT SSL |
| 18083 | emqx:18083 | EMQX Dashboard |
| 4840 | opcua-simulator:4840 | OPC UA |
| 8080 | gateway:8080 | Gateway Web UI + API + Metrics |
| 9090 | prometheus:9090 | Prometheus UI |
| 3000 | grafana:3000 | Grafana UI |

#### 12.2.5 Test Stack (`docker-compose.test.yaml`)

Simulators for all three protocols, used by integration tests. The gateway itself is NOT in this stack — integration tests run the gateway binary directly against these simulators.

| Service | Image | Port | Purpose |
|---|---|---|---|
| `mqtt-broker` | `eclipse-mosquitto:2` | 1883, 9001 | Lightweight MQTT broker for tests |
| `modbus-simulator` | `oitc/modbus-server` | 5020 | Modbus TCP simulator |
| `opcua-simulator` | Local build | 4840 | Same OPC UA simulator as dev stack |
| `s7-simulator` | `mcskol/snap7server` | 102 | Siemens S7 (Snap7) simulator |

All services are on a shared `test-network` bridge with health checks.

#### 12.2.6 OPC UA Simulator

`tools/opcua-simulator/server.py` — a Python-based OPC UA server using `asyncua` for local development and testing.

**Address space:**

| Node ID | Variable | Type | Behavior |
|---|---|---|---|
| `ns=2;s=Demo.Temperature` | Temperature | Double | Sine wave: 20.0 ± 5.0°C (period ~19s) |
| `ns=2;s=Demo.Pressure` | Pressure | Double | Sine wave: 1.2 ± 0.2 bar (period ~31s) |
| `ns=2;s=Demo.Status` | Status | String | Flips between "OK" and "WARN" every 15s |
| `ns=2;s=Demo.Switch` | Switch | Boolean | Static (writable from gateway) |
| `ns=2;s=Demo.WriteTest` | WriteTest | Boolean | Test node for write operations |

**Environment configuration:**

| Variable | Default | Description |
|---|---|---|
| `OPCUA_HOST` | `0.0.0.0` | Bind address |
| `OPCUA_PORT` | `4840` | OPC UA port |
| `OPCUA_AUTO_UPDATE` | `1` | Auto-update values (set `0` for static) |
| `OPCUA_UPDATE_MS` | `500` | Value update interval in ms |

Security: NoSecurity only (development simulator).

> **Gotcha — VariantType for booleans:** Python's `bool` is a subclass of `int`, so `asyncua` can infer the stored variant type as `Int64` instead of `Boolean`. This causes `BadTypeMismatch` errors when the Go OPC UA client tries to write a boolean value. The simulator explicitly sets `varianttype=ua.VariantType.Boolean` to avoid this interoperability issue.

#### 12.2.7 Prometheus Monitoring

Scrape targets configured in `config/prometheus.yml`:

| Job | Target | Path | Interval | Notes |
|---|---|---|---|---|
| `prometheus` | `localhost:9090` | `/metrics` | 15s | Self-monitoring |
| `protocol-gateway` | `gateway:8080` | `/metrics` | 15s | Gateway metrics |
| `emqx` | `emqx:18083` | `/api/v5/prometheus/stats` | 15s | Broker metrics (basic auth: admin/public) |

Storage: 7-day retention with TSDB.

Grafana is configured via provisioning files mounted from `config/grafana/provisioning/`:
- `datasources/datasources.yml` — Auto-configures Prometheus as a data source (`http://prometheus:9090`)
- `dashboards/dashboards.yml` — Dashboard provisioning configuration

### 12.3 Kubernetes Deployment (Reference)

While the gateway is container-ready, Kubernetes deployment requires careful consideration of the stateful nature of device connections. The diagram provides recommended resource specifications, probe configurations, and ConfigMap/Secret mounting. Single-replica deployment with Recreate strategy is recommended due to connection state:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                     KUBERNETES DEPLOYMENT PATTERN                              │
│                                                                                │
│  While the gateway is container-ready, here's the recommended K8s pattern:     │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    RECOMMENDED RESOURCES                                │   │
│  │                                                                         │   │
│  │  Deployment:                                                            │   │
│  │  • replicas: 1 (stateful device connections)                            │   │
│  │  • strategy: Recreate (not RollingUpdate due to connection state)       │   │
│  │                                                                         │   │
│  │  Resources:                                                             │   │
│  │  • requests: cpu=100m, memory=128Mi                                     │   │
│  │  • limits: cpu=500m, memory=512Mi                                       │   │
│  │                                                                         │   │
│  │  Probes:                                                                │   │
│  │  • livenessProbe: /health/live (failureThreshold: 3)                    │   │
│  │  • readinessProbe: /health/ready (failureThreshold: 1)                  │   │
│  │  • startupProbe: /health/ready (failureThreshold: 30, periodSeconds: 2) │   │
│  │                                                                         │   │
│  │  ConfigMap:                                                             │   │
│  │  • config.yaml mounted at /app/config/                                  │   │
│  │  • devices.yaml mounted at /app/config/                                 │   │
│  │                                                                         │   │
│  │  Secret:                                                                │   │
│  │  • MQTT credentials                                                     │   │
│  │  • OPC UA certificates                                                  │   │
│  │                                                                         │   │
│  │  Service:                                                               │   │
│  │  • Type: ClusterIP (internal metrics/API)                               │   │
│  │  • Port: 8080                                                           │   │
│  │                                                                         │   │
│  │  ServiceMonitor (Prometheus Operator):                                  │   │
│  │  • Endpoint: /metrics                                                   │   │
│  │  • Interval: 15s                                                        │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Scaling Considerations:                                                       │
│  • Single replica recommended (device connections are stateful)                │
│  • For HA: Use active-passive with leader election                             │
│  • Horizontal scaling: Deploy multiple gateways for different device groups    │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---