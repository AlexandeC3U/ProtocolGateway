# Protocol Gateway — Documentation


---

## Overview

The Protocol Gateway is an industrial-grade data acquisition system that bridges heterogeneous automation devices (Modbus TCP/RTU, OPC UA, Siemens S7) to modern IT infrastructure via MQTT, following the **Unified Namespace (UNS)** pattern. Built in Go with Clean Architecture, it features per-device connection pooling, multi-tier circuit breakers, batch read optimization, priority-based load shaping, and a real-time Web UI — all packaged in a ~25MB container image.

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                              SYSTEM ARCHITECTURE                                 │
│                                                                                  │
│   INDUSTRIAL FLOOR                                                               │
│   ┌──────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐                            │
│   │ Siemens  │  │ Modbus  │  │ OPC UA  │  │ Modbus  │                            │
│   │ S7-1500  │  │ Sensor  │  │ Kepware │  │  RTU    │                            │
│   └────┬─────┘  └────┬────┘  └────┬────┘  └────┬────┘                            │
│        │ :102        │ :502       │ :4840      │ serial                          │
│        └─────────────┴────────────┴────────────┘                                 │
│                           │                                                      │
│  ─────────────────────────┼───────────────────────────────────────────────────   │
│                           ▼                                                      │
│   PROTOCOL GATEWAY                                                               │
│   ┌──────────────────────────────────────────────────────────────────────────┐   │
│   │                                                                          │   │
│   │   ┌──────────┐  ┌──────────┐  ┌──────────┐       ┌──────────────────┐    │   │
│   │   │ S7 Pool  │  │ Modbus   │  │ OPC UA   │       │  MQTT Publisher  │    │   │
│   │   │ per-dev  │  │   Pool   │  │   Pool   │       │  buffer: 10,000  │    │   │
│   │   │ breakers │  │ per-dev  │  │ per-endpt│       │  QoS 0/1/2       │    │   │
│   │   │ batch:20 │  │ breakers │  │ sessions │       │  auto-reconnect  │    │   │
│   │   └─────┬────┘  │ batching │  │ load-shp │       └────────┬─────────┘    │   │
│   │         │       └─────┬────┘  └─────┬────┘                │              │   │
│   │         └─────────────┴─────────────┘                     │              │   │
│   │                       │                                   │              │   │
│   │              ┌────────▼────────┐                          │              │   │
│   │              │ProtocolManager  │                          │              │   │
│   │              │ route by proto  │                          │              │   │
│   │              └────────┬────────┘                          │              │   │
│   │                       │                                   │              │   │
│   │         ┌─────────────┴──────────────┐                    │              │   │
│   │         │      PollingService        │────────────────────┘              │   │
│   │         │  workers: 10  jitter: 10%  │                                   │   │
│   │         │  back-pressure: skip poll  │                                   │   │
│   │         └─────────────┬──────────────┘                                   │   │
│   │                       │                                                  │   │
│   │         ┌─────────────┴──────────────┐                                   │   │
│   │         │     CommandHandler         │  MQTT cmd/+/+/set -> WriteTag     │   │
│   │         │  rate-limited, queue:1000  │                                   │   │
│   │         └────────────────────────────┘                                   │   │
│   │                                                                          │   │
│   │   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐                 │   │
│   │   │  Health  │  │ Metrics  │  │  HTTP    │  │  Web UI  │                 │   │
│   │   │ Checker  │  │ Registry │  │  :8080   │  │  React   │                 │   │
│   │   └──────────┘  └──────────┘  └──────────┘  └──────────┘                 │   │
│   │                                                                          │   │
│   └──────────────────────────────────────────────────────────────────────────┘   │
│                           │                                                      │
│  ─────────────────────────┼───────────────────────────────────────────────────   │
│                           ▼                                                      │
│   MQTT BROKER  (EMQX / HiveMQ / Mosquitto)                                       │
│   UNS topics: plant/area/line/device/tag                                         │
│                           │                                                      │
│  ─────────────────────────┼───────────────────────────────────────────────────   │
│                           ▼                                                      │
│   IT INFRASTRUCTURE                                                              │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐                         │
│   │InfluxDB  │  │  SCADA   │  │   MES    │  │Analytics │                         │
│   │Historian │  │  System  │  │  System  │  │ Platform │                         │
│   └──────────┘  └──────────┘  └──────────┘  └──────────┘                         │
│                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────┘
```

---

## Table of Contents

| # | Chapter | File | Description |
|---|---------|------|-------------|
| 1 | [Executive Summary](pages/summary.md) | `summary.md` | Purpose, key capabilities, design philosophy |
| 2 | [System Overview](pages/system_overview.md) | `system_overview.md` | High-level architecture, technology stack, dependency graph |
| 3 | [Architectural Principles](pages/architectural_principles.md) | `architectural_principles.md` | Clean Architecture, interface segregation, dependency inversion |
| 4 | [Layer Architecture](pages/layer_architecture.md) | `layer_architecture.md` | Domain layer entities, adapter layer structure, file organization |
| 5 | [Domain Model](pages/domain_model.md) | `domain_model.md` | Validation logic, error taxonomy |
| 6 | [Protocol Adapters](pages/protocol_adapters.md) | `protocol_adapters.md` | Modbus TCP/RTU, OPC UA, Siemens S7, MQTT Publisher |
| 7 | [Connection Management](pages/connection_management.md) | `connection_management.md` | Pooling strategies, idle connection reaping, `MaxTTL` |
| 8 | [Data Flow Architecture](pages/dataflow_architecture.md) | `dataflow_architecture.md` | Read path (polling), write path (commands), worker pool cycling |
| 9 | [Resilience Patterns](pages/resilience_patterns.md) | `resilience_patterns.md` | Circuit breakers, retry/backoff, graceful degradation, startup |
| 10 | [Observability Infrastructure](pages/observability_infrastructure.md) | `observability_infrastructure.md` | Prometheus metrics, structured logging, health checks, NTP sync |
| 11 | [Security Architecture](pages/security_architecture.md) | `security_architecture.md` | TLS, OPC UA security profiles, credential management |
| 12 | [Deployment Architecture](pages/deployment_architecture.md) | `deployment_architecture.md` | Docker, Docker Compose, Kubernetes reference |
| 13 | [Web UI Architecture](pages/web_architecture.md) | `web_architecture.md` | React frontend, REST API endpoints |
| 14 | [Testing Strategy](pages/testing_strategy.md) | `testing_strategy.md` | Test pyramid, simulator infrastructure |
| 15 | [Standards Compliance](pages/standards_compliance.md) | `standards_compliance.md` | IEC 61158, IEC 62541, UNS, Sparkplug B |
| 16 | [Appendices](pages/appendices.md) | `appendices.md` | Configuration reference, error codes, dependency inventory |
| 17 | [Edge Cases & Gotchas](pages/edge_cases.md) | `edge_cases.md` | Operational notes, hot-reload scope, topic sanitization |
| 18 | [Device Configuration](pages/device_configuration.md) | `device_configuration.md` | YAML example, validation rules |
| 19 | [Conclusion](pages/conclusion.md) | `conclusion.md` | Summary of architectural achievements |

---

## Quick Reference

| Concern | Where to Look |
|---------|---------------|
| Add a new protocol adapter | [Ch. 3](pages/architectural_principles.md) (interfaces), [Ch. 6](pages/protocol_adapters.md) (implementation pattern) |
| Tune polling performance | [Ch. 8](pages/dataflow_architecture.md) (worker pool), [Ch. 7](pages/connection_management.md) (pooling) |
| Debug connectivity issues | [Ch. 9](pages/resilience_patterns.md) (circuit breakers), [Ch. 10](pages/observability_infrastructure.md) (metrics) |
| Configure TLS / security | [Ch. 11](pages/security_architecture.md), [Appendix A](pages/appendices.md) |
| Deploy to production | [Ch. 12](pages/deployment_architecture.md), [Ch. 17](pages/edge_cases.md) (gotchas) |
| Understand the domain model | [Ch. 4](pages/layer_architecture.md) (entities), [Ch. 5](pages/domain_model.md) (validation) |
| Set up monitoring | [Ch. 10](pages/observability_infrastructure.md) (Prometheus, Grafana, alerting) |

---

## Technology Stack

| Component | Technology | Purpose |
|-----------|------------|---------|
| Runtime | Go 1.22+ | Compiled binary, excellent concurrency |
| Modbus | `goburrow/modbus` | TCP and RTU support |
| OPC UA | `gopcua/opcua` | Full client stack with subscriptions |
| S7 | `robinson/gos7` | ISO-on-TCP for Siemens PLCs |
| MQTT | `paho.mqtt.golang` | Eclipse Foundation reference client |
| Circuit Breaker | `sony/gobreaker` | Fault isolation |
| Configuration | `spf13/viper` | YAML + env var config |
| Logging | `rs/zerolog` | Zero-allocation structured logging |
| Metrics | `prometheus/client_golang` | Cloud-native metrics |

---

## Project Structure

```
cmd/
└── gateway/
    └── main.go                 # Entry point, wiring, lifecycle

internal/
├── domain/                     # Pure domain model (zero dependencies)
│   ├── device.go               # Device, ConnectionConfig, Tag entities
│   ├── datapoint.go            # DataPoint, Quality, sync.Pool
│   ├── protocol.go             # ProtocolPool interface, ProtocolManager
│   └── errors.go               # Sentinel errors for all failure modes
├── adapter/
│   ├── modbus/                  # Modbus TCP/RTU adapter
│   ├── opcua/                   # OPC UA adapter (sessions, load shaping)
│   ├── s7/                      # Siemens S7 adapter
│   ├── mqtt/                    # MQTT publisher with buffering
│   └── config/                  # Viper-based configuration loading
├── service/
│   ├── polling.go               # Polling engine (per-device goroutines)
│   └── command_handler.go       # MQTT command -> device write
├── api/                         # HTTP handlers, CORS, auth middleware
├── health/                      # Health checker with flapping protection
└── metrics/                     # Prometheus metrics registry

pkg/
└── logging/                     # Structured zerolog wrapper

web/                             # React 18 SPA (CDN, no build step)
config/                          # config.yaml + devices.yaml
```

---

*Document Version: 2.3.0*
*Last Updated: February 2026*
