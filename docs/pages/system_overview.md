- [2. System Overview](#2-system-overview)
  - [2.1 High-Level Architecture](#21-high-level-architecture)
  - [2.2 Technology Stack](#22-technology-stack)
  - [2.3 Dependency Graph](#23-dependency-graph)

## 2. System Overview

### 2.1 High-Level Architecture

This diagram illustrates the complete data flow from industrial floor devices through the Protocol Gateway to IT infrastructure. The gateway acts as a protocol translation layer positioned in the DMZ, bridging the air gap between OT (Operational Technology) and IT networks. Data flows upward from PLCs and sensors through protocol-specific adapters, gets normalized into a common domain model, and is published to an MQTT broker following the Unified Namespace (UNS) pattern. This architecture enables seamless integration with historians, SCADA systems, MES, and analytics platforms.

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                              INDUSTRIAL FLOOR                                    │
│                                                                                  │
│   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐       │
│   │   PLC #1    │    │   PLC #2    │    │  OPC UA     │    │   Modbus    │       │
│   │  Siemens    │    │  Siemens    │    │   Server    │    │   Device    │       │
│   │  S7-1500    │    │  S7-300     │    │  (Kepware)  │    │  (Sensor)   │       │
│   └──────┬──────┘    └──────┬──────┘    └──────┬──────┘    └──────┬──────┘       │
│          │                  │                  │                  │              │
│          │ S7 ISO-on-TCP    │ S7 ISO-on-TCP    │ OPC UA Binary    │ Modbus TCP   │
│          │ Port 102         │ Port 102         │ Port 4840        │ Port 502     │
│          └──────────────────┴──────────────────┴──────────────────┘              │
│                                      │                                           │
│                                      ▼                                           │
├──────────────────────────────────────────────────────────────────────────────────┤
│                           PROTOCOL GATEWAY                                       │
│                                                                                  │
│  ┌────────────────────────────────────────────────────────────────────────────┐  │
│  │                         ADAPTER LAYER                                      │  │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐    │  │
│  │  │ Modbus Pool  │  │  OPC UA Pool │  │   S7 Pool    │  │MQTT Publisher│    │  │
│  │  │              │  │              │  │              │  │              │    │  │
│  │  │ • TCP/RTU    │  │ • Sessions   │  │ • ISO-TCP    │  │ • EMQX/HiveMQ│    │  │
│  │  │ • Per-Device │  │ • Per-Endpt  │  │ • Per-Device │  │ • Buffering  │    │  │
│  │  │ • Batching   │  │ • Security   │  │ • Batching   │  │ • QoS        │    │  │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘    │  │
│  │         │                 │                 │                 │            │  │
│  │         └─────────────────┴─────────────────┴─────────────────┘            │  │
│  │                                   │                                        │  │
│  │                                   ▼                                        │  │
│  │  ┌────────────────────────────────────────────────────────────────────┐    │  │
│  │  │                      PROTOCOL MANAGER                              │    │  │
│  │  │           (Routes operations to appropriate pool)                  │    │  │
│  │  └────────────────────────────────┬───────────────────────────────────┘    │  │
│  └───────────────────────────────────┼────────────────────────────────────────┘  │
│                                      │                                           │
│  ┌───────────────────────────────────┼────────────────────────────────────────┐  │
│  │                         SERVICE LAYER                                      │  │
│  │                                   ▼                                        │  │
│  │  ┌──────────────────────────────────────────────────────────────────────┐  │  │
│  │  │                       POLLING SERVICE                                │  │  │
│  │  │  • Per-device goroutines    • Worker pool (10 default)               │  │  │
│  │  │  • Jitter to prevent burst  • Back-pressure handling                 │  │  │
│  │  └──────────────────────────────────────────────────────────────────────┘  │  │
│  │  ┌──────────────────────────────────────────────────────────────────────┐  │  │
│  │  │                      COMMAND HANDLER                                 │  │  │
│  │  │  • MQTT subscription         • Rate-limited writes                   │  │  │
│  │  │  • Request/response pattern  • Queue-based processing                │  │  │
│  │  └──────────────────────────────────────────────────────────────────────┘  │  │
│  └────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                  │
│  ┌────────────────────────────────────────────────────────────────────────────┐  │
│  │                         DOMAIN LAYER                                       │  │
│  │  Device │ Tag │ DataPoint │ ConnectionConfig │ Quality │ Protocol          │  │
│  └────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                  │
│  ┌────────────────────────────────────────────────────────────────────────────┐  │
│  │                     INFRASTRUCTURE                                         │  │
│  │  HTTP Server │ Health Checker │ Metrics Registry │ Device Manager │ Logger │  │
│  └────────────────────────────────────────────────────────────────────────────┘  │
│                                      │                                           │
├──────────────────────────────────────┼───────────────────────────────────────────┤
│                                      ▼                                           │
│                            MQTT BROKER                                           │
│   ┌──────────────────────────────────────────────────────────────────────────┐   │
│   │                     EMQX / HiveMQ / Mosquitto                            │   │
│   │  • Unified Namespace topics    • QoS guarantees    • Clustering          │   │
│   └──────────────────────────────────────────────────────────────────────────┘   │
│                                      │                                           │
├──────────────────────────────────────┼───────────────────────────────────────────┤
│                                      ▼                                           │
│                        IT INFRASTRUCTURE                                         │
│   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐       │
│   │  Historian  │    │    SCADA    │    │     MES     │    │   Analytics │       │
│   │  (InfluxDB) │    │   System    │    │   System    │    │  Platform   │       │
│   └─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘       │
│                                                                                  │
└──────────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Technology Stack

| Component | Technology | Version | Justification |
|-----------|------------|---------|---------------|
| **Runtime** | Go | 1.22+ | Compiled binary, excellent concurrency primitives, low memory footprint |
| **Modbus** | `goburrow/modbus` | 0.1.0 | Mature, well-tested Modbus implementation supporting TCP and RTU |
| **OPC UA** | `gopcua/opcua` | 0.5.3 | Full OPC UA client stack with subscription support |
| **S7** | `robinson/gos7` | Latest | ISO-on-TCP implementation for Siemens S7 protocol |
| **MQTT** | `paho.mqtt.golang` | 1.4.3 | Eclipse Foundation reference implementation |
| **Circuit Breaker** | `sony/gobreaker` | 0.5.0 | Production-proven circuit breaker implementation |
| **Configuration** | `spf13/viper` | 1.18.2 | Multi-format config with environment variable support |
| **Logging** | `rs/zerolog` | 1.32.0 | Zero-allocation structured logging |
| **Metrics** | `prometheus/client_golang` | 1.19.0 | De facto standard for cloud-native metrics |

### 2.3 Dependency Graph

The dependency hierarchy follows Clean Architecture principles where dependencies point inward toward the domain layer. The `cmd/main` package orchestrates all components but delegates business logic to the service and adapter layers. The domain layer at the center has **zero external dependencies**, making it easily testable and portable. External libraries (`goburrow/modbus`, `gopcua`, etc.) are isolated in the adapter layer, preventing vendor lock-in from propagating through the codebase.

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                            DEPENDENCY HIERARCHY                                 │
│                                                                                 │
│                              ┌──────────────┐                                   │
│                              │   cmd/main   │                                   │
│                              └───────┬──────┘                                   │
│                                      │                                          │
│                    ┌─────────────────┼─────────────────┐                        │
│                    ▼                 ▼                 ▼                        │
│            ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                 │
│            │   internal/  │  │   internal/  │  │   internal/  │                 │
│            │     api      │  │   service    │  │    health    │                 │
│            └───────┬──────┘  └───────┬──────┘  └───────┬──────┘                 │
│                    │                 │                 │                        │
│                    └─────────────────┼─────────────────┘                        │
│                                      ▼                                          │
│                    ┌─────────────────────────────────────┐                      │
│                    │           internal/adapter          │                      │
│                    │  ┌────────┬────────┬────────┬────┐  │                      │
│                    │  │ modbus │ opcua  │   s7   │mqtt│  │                      │
│                    │  └────────┴────────┴────────┴────┘  │                      │
│                    └─────────────────┬───────────────────┘                      │
│                                      │                                          │
│                                      ▼                                          │
│                              ┌──────────────┐                                   │
│                              │   internal/  │                                   │
│                              │    domain    │◄─── Pure domain model             │
│                              └──────────────┘     No external dependencies      │
│                                                                                 │
│  ─────────────────────────────────────────────────────────────────────────────  │
│                           EXTERNAL DEPENDENCIES                                 │
│                                                                                 │
│    goburrow/modbus   gopcua/opcua   gos7   paho.mqtt   gobreaker   zerolog      │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---