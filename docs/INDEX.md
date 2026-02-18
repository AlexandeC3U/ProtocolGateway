# Protocol Gateway

![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-2496ED?logo=docker&logoColor=white)
![MQTT](https://img.shields.io/badge/MQTT-EMQX_5.5-660066)
![Prometheus](https://img.shields.io/badge/Prometheus-E6522C?logo=prometheus&logoColor=white)
![Grafana](https://img.shields.io/badge/Grafana-F46800?logo=grafana&logoColor=white)
![OPC UA](https://img.shields.io/badge/OPC_UA-00539B)
![Modbus](https://img.shields.io/badge/Modbus_TCP/RTU-4B8BBE)
![Siemens S7](https://img.shields.io/badge/Siemens_S7-009999)

## Project Summary

The Protocol Gateway is an industrial-grade bridge between heterogeneous automation devices (PLCs, sensors, SCADA systems) and modern IT infrastructure. It polls data from devices speaking **Modbus TCP/RTU**, **OPC UA**, and **Siemens S7** protocols, converts the readings into a normalized format, and publishes them to an **MQTT broker** (EMQX) following the **Unified Namespace (UNS)** pattern. It also supports bidirectional communication — write commands arrive via MQTT and are routed back to the target device. A built-in Web UI provides device management, topic inspection, and container log viewing, while Prometheus metrics and Grafana dashboards give full observability.

## High-Level Architecture

```mermaid
graph TD
    subgraph "Industrial Devices"
        ModbusDev["Modbus TCP/RTU\nDevices"]
        OPCUADev["OPC UA\nServers / PLCs"]
        S7Dev["Siemens S7\nPLCs"]
    end

    subgraph "Protocol Gateway (Go)"
        PM["Protocol Manager"]
        ModPool["Modbus Pool\n+ Circuit Breakers"]
        OPCPool["OPC UA Pool\n+ Load Shaping\n+ Subscriptions"]
        S7Pool["S7 Pool\n+ Circuit Breakers"]
        PollSvc["Polling Service\n(Worker Pool)"]
        CmdHandler["Command Handler\n(MQTT → Device writes)"]
        MQTTPub["MQTT Publisher\n+ Buffering"]
        API["HTTP API\n+ Web UI"]
        Health["Health Checker\n(K8s probes)"]
        Metrics["Prometheus\nMetrics Registry"]
    end

    subgraph "MQTT & Monitoring"
        EMQX["EMQX Broker\n:1883"]
        Prom["Prometheus\n:9090"]
        Graf["Grafana\n:3000"]
    end

    subgraph "Clients"
        WebUI["Web Browser\n:8080"]
        MQTTClient["MQTT Subscribers\n(SCADA, dashboards)"]
    end

    ModbusDev -->|"Modbus TCP/RTU"| ModPool
    OPCUADev -->|"OPC UA Binary"| OPCPool
    S7Dev -->|"ISO-on-TCP :102"| S7Pool

    ModPool --> PM
    OPCPool --> PM
    S7Pool --> PM

    PM -->|"ReadTags()"| PollSvc
    PM -->|"WriteTag()"| CmdHandler
    PollSvc -->|"Publish batch"| MQTTPub
    MQTTPub -->|"MQTT publish"| EMQX
    EMQX -->|"$nexus/cmd/+/+/set"| CmdHandler
    CmdHandler -->|"Response"| EMQX
    EMQX --> MQTTClient

    API -->|"Device CRUD"| PollSvc
    WebUI -->|"HTTP :8080"| API
    Health -->|"/health, /health/live\n/health/ready"| API
    Metrics -->|"/metrics"| API
    Prom -->|"Scrape :8080/metrics"| Metrics
    Graf -->|"Query"| Prom
```

## Documentation

| Section | Description |
|---|---|
| [Gateway Service](./gateway-service.md) | Core service: polling engine, command handler, REST API, Web UI, device management, configuration |
| [Protocol Adapters](./protocol-adapters.md) | Modbus TCP/RTU, OPC UA, Siemens S7 adapters, MQTT publisher — connection pools, circuit breakers, data conversion |
| [Docker & Infrastructure](./docker-infrastructure.md) | Docker Compose stack, container architecture, Prometheus, Grafana, OPC UA simulator |
| [Architecture](./ARCHITECTURE.md) | Complete and detailed architecture of the entire project |

## Tech Stack

| Layer | Technology | Version | Notes |
|---|---|---|---|
| Language | Go | 1.22 | Single statically-linked binary |
| MQTT Broker | EMQX | 5.5 | Clusterable; dashboard on :18083 |
| MQTT Client | eclipse/paho.mqtt.golang | 1.4.3 | Auto-reconnect, QoS 0/1/2 |
| Modbus | goburrow/modbus | 0.1.0 | TCP + RTU; not thread-safe (serialized with mutex) |
| OPC UA | gopcua/opcua | 0.5.3 | Sessions, security, subscriptions |
| Siemens S7 | robinson/gos7 | latest | ISO-on-TCP :102, rack/slot addressing |
| Circuit Breaker | sony/gobreaker | 0.5.0 | Per-device fault isolation |
| Config | spf13/viper | 1.18.2 | YAML + env var override |
| Logging | rs/zerolog | 1.32.0 | Structured JSON + console |
| Metrics | prometheus/client_golang | 1.19.0 | Counters, gauges, histograms |
| Monitoring | Prometheus + Grafana | 2.50 / 10.3 | Auto-provisioned dashboards |
| Container | Docker + Compose | v2 | Multi-stage build, non-root user |
| OPC UA Sim | Python asyncua | — | Local dev/test simulator |

## Project Structure

```
Connector_Gateway/
├── cmd/gateway/main.go              ← Entry point: wiring, lifecycle, HTTP server
├── internal/
│   ├── domain/                      ← Core types: Device, Tag, DataPoint, Protocol, errors
│   ├── adapter/
│   │   ├── config/                  ← YAML config + device file loading
│   │   ├── modbus/                  ← Modbus TCP/RTU client, pool, conversion
│   │   ├── opcua/                   ← OPC UA client, pool, subscriptions, load shaping, security
│   │   ├── s7/                      ← Siemens S7 client, pool, conversion
│   │   └── mqtt/                    ← MQTT publisher with buffering
│   ├── service/
│   │   ├── polling.go               ← Polling engine (worker pool, batch reads)
│   │   └── command_handler.go       ← Bidirectional MQTT → device write commands
│   ├── api/
│   │   ├── handlers.go              ← Middleware: auth, CORS, body size limit
│   │   ├── runtime.go               ← Docker CLI log provider
│   │   └── runtime_handlers.go      ← Device CRUD, topics overview, container logs
│   ├── health/checker.go            ← Health checks with flapping protection, K8s probes
│   └── metrics/registry.go          ← Prometheus metrics (connections, polls, MQTT, devices)
├── pkg/logging/logger.go            ← Structured zerolog wrapper
├── config/
│   ├── config.yaml                  ← Gateway service config
│   ├── devices.yaml                 ← Device + tag definitions
│   ├── prometheus.yml               ← Prometheus scrape config
│   └── grafana/provisioning/        ← Grafana datasource + dashboard provisioning
├── web/index.html                   ← Single-page Web UI (vanilla HTML/JS)
├── tools/opcua-simulator/           ← Python OPC UA simulator for local testing
├── certs/                           ← OPC UA PKI certificates (mounted at runtime)
├── testing/                         ← Unit, integration, benchmark, fuzz, e2e tests
├── Dockerfile                       ← Multi-stage: golang:1.22-alpine → alpine:3.19
├── docker-compose.yaml              ← Dev stack: EMQX + Gateway + OPC UA Sim + Prometheus + Grafana
└── docker-compose.test.yaml         ← Test stack: Mosquitto + Modbus Sim + OPC UA Sim + S7 Sim
```

## Key Data Flows

### Read Path (Device → MQTT)

```mermaid
sequenceDiagram
    participant Timer as Poll Timer
    participant PS as PollingService
    participant PM as ProtocolManager
    participant Pool as Connection Pool
    participant Dev as Industrial Device
    participant Pub as MQTT Publisher
    participant Broker as EMQX Broker

    Timer->>PS: Tick (per device interval)
    PS->>PM: ReadTags(device, tags)
    PM->>Pool: ReadTags(ctx, device, tags)
    Pool->>Dev: Protocol-specific read (batch)
    Dev-->>Pool: Raw bytes
    Pool->>Pool: parseValue() + applyScaling()
    Pool-->>PM: []DataPoint (value, quality, timestamps)
    PM-->>PS: []DataPoint
    PS->>Pub: PublishBatch(dataPoints)
    Pub->>Broker: MQTT PUBLISH (UNS topic)
    Note over Broker: Topic: {uns_prefix}/{topic_suffix}<br/>Payload: {"v":20.1,"u":"°C","q":"good","ts":...}
```

### Write Path (MQTT → Device)

```mermaid
sequenceDiagram
    participant Client as MQTT Client
    participant Broker as EMQX Broker
    participant CMD as CommandHandler
    participant PM as ProtocolManager
    participant Pool as Connection Pool
    participant Dev as Industrial Device

    Client->>Broker: PUBLISH $nexus/cmd/{device}/{tag}/set
    Broker->>CMD: Message delivered
    CMD->>CMD: Parse command, find device+tag
    CMD->>PM: WriteTag(device, tag, value)
    PM->>Pool: WriteTag(ctx, device, tag, value)
    Pool->>Pool: reverseScaling() + valueToBytes()
    Pool->>Dev: Protocol-specific write
    Dev-->>Pool: ACK / Error
    Pool-->>CMD: success/error
    CMD->>Broker: PUBLISH $nexus/cmd/{device}/{tag}/response
    Note over Broker: {"success":true,"duration_ms":45}
```

## Quick Start

```bash
# Clone and start the dev stack
git clone https://github.com/AlexandeC3U/ProtocolGateway
cd Connector_Gateway
docker compose up --build

# Access:
#   Web UI:          http://localhost:8080
#   EMQX Dashboard:  http://localhost:18083  (admin / public)
#   Prometheus:      http://localhost:9090
#   Grafana:         http://localhost:3000    (admin / admin)
```

See [README.md](../README.md) for detailed setup and device configuration instructions.
