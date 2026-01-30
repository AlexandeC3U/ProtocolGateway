 Gateway Architecture Document

## Comprehensive Technical Reference

**Version:** 2.0  
**Classification:** Technical Architecture Specification  
**Target Audience:** Software Architects, Senior Engineers, System Integrators

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
   - [1.1 Purpose](#11-purpose)
   - [1.2 Key Capabilities](#12-key-capabilities)
   - [1.3 Design Philosophy](#13-design-philosophy)
2. [System Overview](#2-system-overview)
   - [2.1 High-Level Architecture](#21-high-level-architecture)
   - [2.2 Technology Stack](#22-technology-stack)
   - [2.3 Dependency Graph](#23-dependency-graph)
3. [Architectural Principles](#3-architectural-principles)
   - [3.1 Clean Architecture Adherence](#31-clean-architecture-adherence)
   - [3.2 Interface Segregation](#32-interface-segregation)
   - [3.3 Dependency Inversion](#33-dependency-inversion)
4. [Layer Architecture](#4-layer-architecture)
   - [4.1 Domain Layer](#41-domain-layer-internaldomain)
   - [4.2 Adapter Layer](#42-adapter-layer-internaladapter)
5. [Domain Model](#5-domain-model)
   - [5.1 Validation Logic](#51-validation-logic)
   - [5.2 Error Taxonomy](#52-error-taxonomy)
6. [Protocol Adapters](#6-protocol-adapters)
   - [6.1 Modbus Adapter](#61-modbus-adapter)
   - [6.2 OPC UA Adapter](#62-opc-ua-adapter)
   - [6.3 S7 Adapter](#63-s7-adapter)
   - [6.4 MQTT Publisher](#64-mqtt-publisher)
7. [Connection Management](#7-connection-management)
   - [7.1 Connection Pooling Strategies](#71-connection-pooling-strategies)
   - [7.2 Idle Connection Management](#72-idle-connection-management)
8. [Data Flow Architecture](#8-data-flow-architecture)
   - [8.1 Read Path (Polling)](#81-read-path-polling)
   - [8.2 Write Path (Commands)](#82-write-path-commands)
9. [Resilience Patterns](#9-resilience-patterns)
   - [9.1 Circuit Breaker Pattern](#91-circuit-breaker-pattern)
   - [9.2 Retry with Exponential Backoff](#92-retry-with-exponential-backoff)
   - [9.3 Graceful Degradation](#93-graceful-degradation)
   - [9.4 Gateway Initialization and Startup](#94-gateway-initialization-and-startup)
10. [Observability Infrastructure](#10-observability-infrastructure)
    - [10.1 Metrics Architecture](#101-metrics-architecture)
    - [10.2 Structured Logging](#102-structured-logging)
    - [10.3 Health Check System](#103-health-check-system)
11. [Configuration Management](#11-configuration-management)
    - [11.1 Transport Security](#111-transport-security)
    - [11.2 Credential Management](#112-credential-management)
    - [11.3 Network Security](#113-network-security)
12. [Deployment Architecture](#12-deployment-architecture)
    - [12.1 Container Architecture](#121-container-architecture)
    - [12.2 Docker Compose Architecture](#122-docker-compose-architecture)
    - [12.3 Kubernetes Deployment (Reference)](#123-kubernetes-deployment-reference)
13. [Performance Engineering](#13-performance-engineering)
    - [13.1 Frontend Technology Stack](#131-frontend-technology-stack)
    - [13.2 API Endpoints](#132-api-endpoints)
14. [Standards Compliance](#14-standards-compliance)
    - [14.1 Test Architecture](#141-test-architecture)
    - [14.2 Simulator Infrastructure](#142-simulator-infrastructure)
15. [API Reference](#15-api-reference)
    - [15.1 Industrial Protocol Standards](#151-industrial-protocol-standards)
    - [15.2 Unified Namespace (UNS) Architecture](#152-unified-namespace-uns-architecture)
    - [15.3 Sparkplug B Compatibility](#153-sparkplug-b-compatibility)
16. [Security Architecture](#16-security-architecture)
17. [Appendices](#17-appendices)

---

## 1. Executive Summary

### 1.1 Purpose

The Protocol Gateway is an industrial-grade software system designed to bridge the communication gap between heterogeneous industrial automation devices and modern IT infrastructure. It implements a **protocol translation layer** that normalizes data from multiple industrial protocols (Modbus TCP/RTU, OPC UA, Siemens S7) into a unified MQTT-based message stream compatible with the **Unified Namespace (UNS)** architectural pattern.

### 1.2 Key Capabilities

The diagram below provides a visual summary of the gateway's core capabilities across all supported industrial protocols. Each protocol adapter (Modbus, OPC UA, S7, MQTT) operates independently with its own connection management, while sharing common cross-cutting concerns like circuit breakers, health monitoring, and metrics collection. This modular design allows the gateway to scale horizontally across protocols while maintaining consistent operational behavior.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         PROTOCOL GATEWAY CAPABILITIES                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐      │
│  │  MODBUS     │   │   OPC UA    │   │  SIEMENS    │   │    MQTT     │      │
│  │  TCP/RTU    │   │   Client    │   │    S7       │   │  Publisher  │      │
│  │             │   │             │   │             │   │             │      │
│  │ • Coils     │   │ • Sessions  │   │ • DB Blocks │   │ • QoS 0/1/2 │      │
│  │ • Registers │   │ • Security  │   │ • Merkers   │   │ • TLS/mTLS  │      │
│  │ • Batching  │   │ • Subscribe │   │ • I/O Areas │   │ • Buffering │      │
│  └─────────────┘   └─────────────┘   └─────────────┘   └─────────────┘      │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                      CROSS-CUTTING CONCERNS                         │    │
│  │  • Connection Pooling    • Circuit Breakers    • Health Monitoring  │    │
│  │  • Load Shaping          • Metrics Collection  • Hot Configuration  │    │
│  │  • Object Pooling        • Graceful Shutdown   • Web UI Console     │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 1.3 Design Philosophy

The gateway adheres to several fundamental design principles derived from industrial automation best practices and modern software engineering:

1. **Protocol Agnosticism**: The core business logic remains independent of specific protocol implementations
2. **Fail-Safe Operation**: Degraded mode operation with circuit breakers prevents cascade failures
3. **Zero-Downtime Configuration**: Runtime device management without service interruption
4. **Observable by Default**: Comprehensive metrics, health checks, and logging built into every component
5. **Resource Efficiency**: Object pooling, connection reuse, and batching minimize overhead

---

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
│  │  │ • Batching   │  │ • Subscribe  │  │ • Batching   │  │ • QoS        │    │  │
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
| **Modbus** | goburrow/modbus | 0.1.0 | Mature, well-tested Modbus implementation supporting TCP and RTU |
| **OPC UA** | gopcua/opcua | 0.5.3 | Full OPC UA client stack with subscription support |
| **S7** | robinson/gos7 | Latest | ISO-on-TCP implementation for Siemens S7 protocol |
| **MQTT** | paho.mqtt.golang | 1.4.3 | Eclipse Foundation reference implementation |
| **Circuit Breaker** | sony/gobreaker | 0.5.0 | Production-proven circuit breaker implementation |
| **Configuration** | spf13/viper | 1.18.2 | Multi-format config with environment variable support |
| **Logging** | rs/zerolog | 1.32.0 | Zero-allocation structured logging |
| **Metrics** | prometheus/client_golang | 1.19.0 | De facto standard for cloud-native metrics |

### 2.3 Dependency Graph

The dependency hierarchy follows Clean Architecture principles where dependencies point inward toward the domain layer. The `cmd/main` package orchestrates all components but delegates business logic to the service and adapter layers. The domain layer at the center has **zero external dependencies**, making it easily testable and portable. External libraries (goburrow/modbus, gopcua, etc.) are isolated in the adapter layer, preventing vendor lock-in from propagating through the codebase.

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

## 3. Architectural Principles

### 3.1 Clean Architecture Adherence

The gateway implements a variant of Clean Architecture (Hexagonal Architecture) with distinct layers. This layered approach ensures that business rules (what data to collect, how to transform it) remain isolated from infrastructure concerns (which protocols to use, how to connect). The following diagram shows the concentric layers, with the domain at the center and frameworks at the periphery:

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                          CLEAN ARCHITECTURE LAYERS                              │
│                                                                                 │
│  ┌─────────────────────────────────────────────────────────────────────────┐    │
│  │                                                                         │    │
│  │    ┌─────────────────────────────────────────────────────────────────┐  │    │
│  │    │                                                                 │  │    │
│  │    │    ┌─────────────────────────────────────────────────────────┐  │  │    │
│  │    │    │                                                         │  │  │    │ 
│  │    │    │    ┌─────────────────────────────────────────────────┐  │  │  │    │
│  │    │    │    │              DOMAIN ENTITIES                    │  │  │  │    │
│  │    │    │    │                                                 │  │  │  │    │
│  │    │    │    │  Device, Tag, DataPoint, Protocol, Quality      │  │  │  │    │
│  │    │    │    │                                                 │  │  │  │    │
│  │    │    │    │  • No dependencies on outer layers              │  │  │  │    │
│  │    │    │    │  • Business rules encapsulated here             │  │  │  │    │
│  │    │    │    │  • Validation logic lives with entities         │  │  │  │    │
│  │    │    │    └─────────────────────────────────────────────────┘  │  │  │    │
│  │    │    │                                                         │  │  │    │
│  │    │    │                    USE CASES / SERVICES                 │  │  │    │
│  │    │    │                                                         │  │  │    │
│  │    │    │  PollingService, CommandHandler                         │  │  │    │
│  │    │    │                                                         │  │  │    │
│  │    │    │  • Orchestrate domain entities                          │  │  │    │
│  │    │    │  • Implement business workflows                         │  │  │    │
│  │    │    │  • Depend only on domain and interfaces                 │  │  │    │
│  │    │    └─────────────────────────────────────────────────────────┘  │  │    │
│  │    │                                                                 │  │    │
│  │    │                      INTERFACE ADAPTERS                         │  │    │
│  │    │                                                                 │  │    │
│  │    │  API Handlers, Device Manager, Protocol Manager                 │  │    │
│  │    │                                                                 │  │    │
│  │    │  • Convert data between layers                                  │  │    │
│  │    │  • Implement repository/gateway interfaces                      │  │    │
│  │    └─────────────────────────────────────────────────────────────────┘  │    │
│  │                                                                         │    │
│  │                    FRAMEWORKS & DRIVERS (Infrastructure)                │    │
│  │                                                                         │    │
│  │  HTTP Server, Modbus Client, OPC UA Client, S7 Client, MQTT Client      │    │
│  │                                                                         │    │
│  │  • External library integrations                                        │    │
│  │  • Database/network access                                              │    │
│  │  • Framework-specific code isolated here                                │    │
│  └─────────────────────────────────────────────────────────────────────────┘    │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

**Justification for Protocol Gateways:**

Industrial protocol gateways face unique challenges that Clean Architecture addresses:

1. **Protocol Volatility**: New protocols emerge, existing ones evolve. Isolating protocol implementations prevents ripple effects.
2. **Vendor Lock-in Avoidance**: The domain layer contains no vendor-specific code.
3. **Testability**: Domain logic can be tested without physical devices.
4. **Regulatory Compliance**: Clean separation aids audit trails and certification.

### 3.2 Interface Segregation

The `ProtocolPool` interface demonstrates the Interface Segregation Principle (ISP):

```go
// ProtocolPool defines the contract for protocol-specific connection pools.
// Each method has a single responsibility, allowing partial implementation.
type ProtocolPool interface {
    ReadTags(ctx context.Context, device *Device, tags []*Tag) ([]*DataPoint, error)
    ReadTag(ctx context.Context, device *Device, tag *Tag) (*DataPoint, error)
    WriteTag(ctx context.Context, device *Device, tag *Tag, value interface{}) error
    Close() error
    HealthCheck(ctx context.Context) error
}
```

**Why This Matters for Protocol Gateways:**

- **Modbus**: Supports read and write for holding registers, but coils may be read-only in some configurations
- **OPC UA**: May have nodes that are subscription-only (no explicit read)
- **S7**: Some memory areas (Inputs) are inherently read-only

### 3.3 Dependency Inversion

The Dependency Inversion Principle (DIP) is fundamental to the gateway's extensibility. The diagram below shows how the high-level `PollingService` depends on an abstraction (`ProtocolPool` interface) rather than concrete implementations. This means new protocols can be added without modifying existing polling logic—simply implement the interface and register it with the Protocol Manager.

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                          DEPENDENCY INVERSION                                   │
│                                                                                 │
│  HIGH-LEVEL MODULE                    LOW-LEVEL MODULE                          │
│  (PollingService)                     (ModbusPool)                              │
│                                                                                 │
│       ┌──────────────────┐                 ┌──────────────────┐                 │
│       │  PollingService  │                 │   ModbusPool     │                 │
│       │                  │                 │                  │                 │
│       │  • Poll devices  │                 │  • goburrow lib  │                 │
│       │  • Publish data  │                 │  • TCP/RTU       │                 │
│       └────────┬─────────┘                 └────────┬─────────┘                 │
│                │                                    │                           │
│                │ depends on                         │ implements                │
│                ▼                                    ▼                           │
│       ┌────────────────────────────────────────────────────────┐                │
│       │                   ProtocolPool                         │                │
│       │                   <<interface>>                        │                │
│       │                                                        │                │
│       │  + ReadTags(ctx, device, tags) ([]*DataPoint, error)   │                │
│       │  + ReadTag(ctx, device, tag) (*DataPoint, error)       │                │
│       │  + WriteTag(ctx, device, tag, value) error             │                │
│       │  + Close() error                                       │                │
│       │  + HealthCheck(ctx) error                              │                │
│       └────────────────────────────────────────────────────────┘                │
│                                                                                 │
│  Both modules depend on the abstraction, not on each other.                     │
│  PollingService can work with any protocol without modification.                │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## 4. Layer Architecture

### 4.1 Domain Layer (`internal/domain/`)

The domain layer is the **heart of the system**, containing business entities, rules, and interfaces that are protocol-agnostic.

#### 4.1.1 Entity Relationship Diagram

The core domain model consists of three primary entities: **Device** (physical or logical endpoint), **Tag** (individual data point with addressing), and **DataPoint** (runtime measurement with quality and timestamps). This diagram shows their relationships and key attributes. A Device contains multiple Tags, and each poll cycle produces DataPoints for enabled Tags. The separation between configuration (Device/Tag) and runtime data (DataPoint) enables hot-reload of device configurations without losing operational state.

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                          DOMAIN ENTITY RELATIONSHIPS                           │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                              DEVICE                                     │   │
│  │                                                                         │   │
│  │  ┌──────────────────┬──────────────────┬──────────────────────────────┐ │   │
│  │  │    Identity      │    Protocol      │      Configuration           │ │   │ 
│  │  ├──────────────────┼──────────────────┼──────────────────────────────┤ │   │
│  │  │ • ID             │ • Protocol       │ • PollInterval               │ │   │
│  │  │ • Name           │ • ConnectionCfg  │ • Enabled                    │ │   │
│  │  │ • Description    │                  │ • UNSPrefix                  │ │   │
│  │  │                  │                  │ • ConfigVersion              │ │   │
│  │  └──────────────────┴──────────────────┴──────────────────────────────┘ │   │
│  │                              │                                          │   │
│  │                              │ 1:N                                      │   │
│  │                              ▼                                          │   │
│  │  ┌────────────────────────────────────────────────────────────────────┐ │   │
│  │  │                              TAG                                   │ │   │
│  │  │                                                                    │ │   │
│  │  │  ┌───────────────┬────────────────┬───────────────┬──────────────┐ │ │   │
│  │  │  │   Identity    │   Addressing   │  Data Config  │   Behavior   │ │ │   │
│  │  │  ├───────────────┼────────────────┼───────────────┼──────────────┤ │ │   │
│  │  │  │ • ID          │ • Address      │ • DataType    │ • PollIntrvl │ │ │   │
│  │  │  │ • Name        │ • RegisterType │ • ByteOrder   │ • Deadband   │ │ │   │
│  │  │  │ • TopicSuffix │ • OPCNodeID    │ • ScaleFactor │ • AccessMode │ │ │   │
│  │  │  │               │ • S7Address    │ • Offset      │ • Priority   │ │ │   │
│  │  │  └───────────────┴────────────────┴───────────────┴──────────────┘ │ │   │
│  │  └────────────────────────────────────────────────────────────────────┘ │   │
│  │                              │                                          │   │
│  │                              │ 1:N (runtime)                            │   │
│  │                              ▼                                          │   │
│  │  ┌────────────────────────────────────────────────────────────────────┐ │   │
│  │  │                           DATAPOINT                                │ │   │
│  │  │                                                                    │ │   │
│  │  │  ┌───────────────┬────────────────┬───────────────┬──────────────┐ │ │   │
│  │  │  │    Source     │     Value      │   Timestamps  │    QoS       │ │ │   │
│  │  │  ├───────────────┼────────────────┼───────────────┼──────────────┤ │ │   │
│  │  │  │ • DeviceID    │ • Value        │ • Timestamp   │ • Quality    │ │ │   │
│  │  │  │ • TagID       │ • RawValue     │ • SourceTS    │ • Priority   │ │ │   │
│  │  │  │ • Topic       │ • Unit         │ • GatewayTS   │ • LatencyMs  │ │ │   │
│  │  │  │               │                │ • PublishTS   │ • StalenessMs│ │ │   │
│  │  │  └───────────────┴────────────────┴───────────────┴──────────────┘ │ │   │
│  │  └────────────────────────────────────────────────────────────────────┘ │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

#### 4.1.2 Device Entity

The `Device` entity represents a physical or logical industrial device:

```go
type Device struct {
    // Identity
    ID          string    // Unique identifier (e.g., "plc-001")
    Name        string    // Human-readable name
    Description string    // Optional description
    
    // Protocol Configuration
    Protocol         Protocol          // modbus-tcp, modbus-rtu, opcua, s7
    ConnectionConfig ConnectionConfig  // Protocol-specific connection parameters
    
    // Data Collection
    Tags         []*Tag        // List of data points to collect
    PollInterval time.Duration // Default polling interval (minimum 100ms)
    
    // State Management
    Enabled              bool      // Whether polling is active
    UNSPrefix           string    // Unified Namespace prefix
    ConfigVersion        int       // Current configuration version
    ActiveConfigVersion  int       // Version currently running
    LastKnownGoodVersion int       // Last working configuration
    
    // Metadata
    Metadata  map[string]string // Custom key-value pairs
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

**Design Decisions:**

| Decision | Rationale | Industry Standard Reference |
|----------|-----------|----------------------------|
| Minimum 100ms poll interval | Prevents CPU exhaustion and network flooding | IEC 62541-4 (OPC UA): Recommended minimum sampling interval |
| UNS prefix mandatory | Ensures ISA-95 compliant topic hierarchy | ISA-95 / Unified Namespace Pattern |
| Configuration versioning | Enables rollback on misconfiguration | IEC 62443-3-3: Configuration management |
| Separate enabled flag | Allows configuration without activation | Common PLC programming practice |

#### 4.1.3 Tag Entity

The `Tag` entity represents a single data point with protocol-specific addressing:

```go
type Tag struct {
    // Identity
    ID          string // Unique within device
    Name        string // Human-readable name
    Description string
    
    // Protocol-Specific Addressing
    // Modbus
    Address       uint16       // Register address (0-65535)
    RegisterType  RegisterType // coil, discrete_input, holding_register, input_register
    RegisterCount uint16       // Number of registers to read
    BitPosition   int          // For bit-level access within registers
    
    // OPC UA
    OPCNodeID         string // Node identifier (e.g., "ns=2;s=Temperature")
    OPCNamespaceIndex uint16 // Namespace index
    
    // S7
    S7Area      S7Area // DB, M, I, Q, T, C
    S7DBNumber  uint16 // Data block number
    S7Offset    uint32 // Byte offset
    S7BitOffset uint8  // Bit offset for boolean
    S7Address   string // Symbolic address (e.g., "DB1.DBD0")
    
    // Data Processing
    DataType    DataType  // bool, int16, uint16, int32, float32, etc.
    ByteOrder   ByteOrder // big_endian, little_endian, mid_big_endian, mid_lit_endian
    ScaleFactor float64   // Multiplier applied to raw value
    Offset      float64   // Added after scaling
    Unit        string    // Engineering unit (e.g., "°C", "bar")
    
    // MQTT Routing
    TopicSuffix string // Appended to device UNS prefix
    
    // Behavior
    PollInterval  *time.Duration // Override device poll interval
    DeadbandType  DeadbandType   // none, absolute, percent
    DeadbandValue float64        // Threshold for change detection
    Enabled       bool
    AccessMode    AccessMode     // read, write, read_write
    Priority      int            // 0=telemetry, 1=control, 2=safety
}
```

**Protocol-Specific Addressing Deep Dive:**

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                        PROTOCOL ADDRESSING MODELS                               │
│                                                                                 │
│  MODBUS                           OPC UA                    SIEMENS S7          │
│  ═══════                          ══════                    ══════════          │
│                                                                                 │
│  ┌─────────────────┐        ┌─────────────────┐        ┌─────────────────┐      │
│  │ Register Space  │        │  Address Space  │        │  Memory Areas   │      │
│  ├─────────────────┤        ├─────────────────┤        ├─────────────────┤      │
│  │ Coils           │        │ Objects         │        │ DB (Data Block) │      │
│  │ 0x00001-0x09999 │        │   ├─ Variables  │        │ M  (Merker)     │      │
│  │                 │        │   │   ns=2;s=X  │        │ I  (Input)      │      │
│  │ Discrete Inputs │        │   ├─ Methods    │        │ Q  (Output)     │      │
│  │ 0x10001-0x19999 │        │   └─ Events     │        │ T  (Timer)      │      │
│  │                 │        │                 │        │ C  (Counter)    │      │
│  │ Input Registers │        │ Hierarchical    │        │                 │      │
│  │ 0x30001-0x39999 │        │ Folder/Object   │        │ Address Format: │      │
│  │                 │        │ Structure       │        │ DB1.DBW0        │      │
│  │ Holding Regs    │        │                 │        │ MW100           │      │
│  │ 0x40001-0x49999 │        │ NodeID Types:   │        │ I0.0            │      │
│  │                 │        │ • Numeric (i=)  │        │                 │      │
│  │ Function Codes: │        │ • String (s=)   │        │ Addressing:     │      │
│  │ 01: Read Coils  │        │ • GUID (g=)     │        │ DB.DBX (Bit)    │      │
│  │ 02: Read DI     │        │ • Opaque (b=)   │        │ DB.DBB (Byte)   │      │
│  │ 03: Read HR     │        │                 │        │ DB.DBW (Word)   │      │
│  │ 04: Read IR     │        │ ns=2;s=Demo.T   │        │ DB.DBD (DWord)  │      │
│  │ 05: Write Coil  │        │                 │        │                 │      │
│  │ 06: Write HR    │        │                 │        │                 │      │
│  │ 15: Write Coils │        │                 │        │                 │      │
│  │ 16: Write HRs   │        │                 │        │                 │      │
│  └─────────────────┘        └─────────────────┘        └─────────────────┘      │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

#### 4.1.4 DataPoint Entity

The `DataPoint` entity represents a measured value with comprehensive metadata:

```go
type DataPoint struct {
    // Source Identification
    DeviceID string
    TagID    string
    Topic    string // Full MQTT topic
    
    // Value
    Value    interface{} // Scaled, processed value
    RawValue interface{} // Original value from device
    Unit     string
    Quality  Quality // good, bad, uncertain, etc.
    
    // Timestamps (critical for time-series analysis)
    Timestamp        time.Time // Primary timestamp
    SourceTimestamp  time.Time // Device-provided timestamp (if available)
    GatewayTimestamp time.Time // When gateway received value
    PublishTimestamp time.Time // When published to MQTT
    
    // Performance Metrics
    LatencyMs   float64 // GatewayTimestamp - SourceTimestamp
    StalenessMs float64 // Current time - SourceTimestamp
    
    // QoS
    Priority int // For load shaping priority queues
    
    // Extensibility
    Metadata map[string]string
}
```

**Timestamp Architecture:**

Precise timestamping is critical for industrial applications including event sequence recording, latency monitoring, and regulatory compliance (FDA 21 CFR Part 11). The following diagram illustrates the four timestamp stages a data point traverses, enabling accurate latency and staleness calculations essential for time-series analysis and control loop optimization:

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                          TIMESTAMP FLOW                                         │
│                                                                                 │
│  ┌──────────────┐                                                               │
│  │    DEVICE    │  SourceTimestamp                                              │
│  │   (PLC/RTU)  │  ─────────────────►  When device sampled the value            │
│  └──────┬───────┘                       (may be unavailable for Modbus)         │
│         │                                                                       │
│         │ Network                                                               │
│         ▼                                                                       │
│  ┌──────────────┐                                                               │
│  │   GATEWAY    │  GatewayTimestamp                                             │
│  │   (Read)     │  ─────────────────►  When gateway received response           │
│  └──────┬───────┘                                                               │
│         │                                                                       │
│         │ Processing (scaling, validation)                                      │
│         ▼                                                                       │
│  ┌──────────────┐                                                               │
│  │   GATEWAY    │  Timestamp                                                    │
│  │  (Process)   │  ─────────────────►  Primary timestamp for data point         │
│  └──────┬───────┘                       (typically = GatewayTimestamp)          │
│         │                                                                       │
│         │ MQTT Publish                                                          │
│         ▼                                                                       │
│  ┌──────────────┐                                                               │
│  │   GATEWAY    │  PublishTimestamp                                             │
│  │  (Publish)   │  ─────────────────►  When message sent to broker              │
│  └──────┬───────┘                                                               │
│         │                                                                       │
│         ▼                                                                       │
│  ┌──────────────┐                                                               │
│  │    BROKER    │                                                               │
│  │   (MQTT)     │                                                               │
│  └──────────────┘                                                               │
│                                                                                 │
│  LatencyMs = GatewayTimestamp - SourceTimestamp                                 │
│  StalenessMs = Now - SourceTimestamp                                            │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

**Justification (IEEE 1588 / IEC 61850):**

Industrial applications require precise timestamping for:
- **Event Sequence Recording**: Determining the order of events during incidents
- **Process Correlation**: Correlating measurements from multiple devices
- **Latency Monitoring**: Ensuring data freshness for control loops
- **Compliance**: Meeting regulatory requirements (FDA 21 CFR Part 11, etc.)

#### 4.1.5 Quality Enumeration

```go
type Quality string

const (
    QualityGood          Quality = "good"
    QualityBad           Quality = "bad"
    QualityUncertain     Quality = "uncertain"
    QualityNotConnected  Quality = "not_connected"
    QualityConfigError   Quality = "config_error"
    QualityDeviceFailure Quality = "device_failure"
    QualityTimeout       Quality = "timeout"
)
```

**Alignment with OPC UA Quality (IEC 62541-8):**

| Gateway Quality | OPC UA StatusCode | Description |
|----------------|-------------------|-------------|
| `good` | Good (0x00000000) | Value is valid and current |
| `bad` | Bad (0x80000000) | Value is not usable |
| `uncertain` | Uncertain (0x40000000) | Value may be inaccurate |
| `not_connected` | BadNotConnected (0x808A0000) | Communication failure |
| `config_error` | BadConfigurationError (0x80890000) | Invalid configuration |
| `device_failure` | BadDeviceFailure (0x80880000) | Device malfunction |
| `timeout` | BadTimeout (0x800A0000) | Operation timed out |

#### 4.1.6 Object Pooling for DataPoint

```go
var dataPointPool = sync.Pool{
    New: func() interface{} {
        return &DataPoint{
            Metadata: make(map[string]string, 4),
        }
    },
}

// AcquireDataPoint retrieves a DataPoint from the pool
func AcquireDataPoint() *DataPoint {
    dp := dataPointPool.Get().(*DataPoint)
    // Reset fields...
    return dp
}

// ReleaseDataPoint returns a DataPoint to the pool
func ReleaseDataPoint(dp *DataPoint) {
    if dp == nil {
        return
    }
    // Clear for reuse...
    dataPointPool.Put(dp)
}
```

**Performance Justification:**

Object pooling via `sync.Pool` dramatically reduces garbage collection pressure in high-throughput scenarios. The diagram below quantifies the performance impact—with 5,000 data points per second, pooling eliminates allocations and maintains sub-millisecond latency, crucial for real-time industrial applications where GC pauses are unacceptable:

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                    OBJECT POOLING PERFORMANCE IMPACT                            │
│                                                                                 │
│  Scenario: 100 devices × 50 tags × 1 Hz polling = 5,000 DataPoints/second       │
│                                                                                 │
│  WITHOUT POOLING:                                                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐    │
│  │  • 5,000 allocations/second                                             │    │
│  │  • ~200 bytes per DataPoint = 1 MB/second allocated                     │    │
│  │  • GC pressure increases, causing periodic latency spikes               │    │
│  │  • Typical GC pause: 1-10ms (unacceptable for real-time)                │    │
│  └─────────────────────────────────────────────────────────────────────────┘    │
│                                                                                 │
│  WITH POOLING:                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────────┐    │
│  │  • Near-zero allocations (objects reused)                               │    │
│  │  • Pool size self-adjusts to working set                                │    │
│  │  • GC pauses minimized                                                  │    │
│  │  • Consistent sub-millisecond latency                                   │    │
│  └─────────────────────────────────────────────────────────────────────────┘    │
│                                                                                 │
│  Benchmark Results:                                                             │
│  BenchmarkDataPointPooled-8      10000000    112 ns/op      0 B/op    0 allocs  │
│  BenchmarkDataPointAllocated-8    5000000    243 ns/op    208 B/op    1 allocs  │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 Adapter Layer (`internal/adapter/`)

The adapter layer implements protocol-specific communication, translating between domain concepts and wire protocols.

#### 4.2.1 Adapter Architecture Overview

The adapter layer provides concrete implementations for each industrial protocol. The diagram below shows how the `ProtocolManager` routes operations to the appropriate connection pool based on device protocol. Each pool manages its own connections, circuit breakers, and protocol-specific optimizations (batching for Modbus, subscriptions for OPC UA). The MQTT Publisher handles outbound message delivery with buffering and reconnection logic.

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                           ADAPTER LAYER COMPONENTS                             │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                         ProtocolManager                                 │   │
│  │                                                                         │   │
│  │  • Routes operations to registered protocol pools                       │   │
│  │  • Thread-safe pool registration and lookup                             │   │
│  │  • Aggregates health status from all pools                              │   │
│  │                                                                         │   │
│  │  ┌─────────────────────────────────────────────────────────────────┐    │   │
│  │  │  pools map[Protocol]ProtocolPool                                │    │   │
│  │  │                                                                 │    │   │
│  │  │  "modbus-tcp" ──► ModbusPool                                    │    │   │
│  │  │  "modbus-rtu" ──► ModbusPool (same pool, different config)      │    │   │
│  │  │  "opcua"      ──► OPCUAPool                                     │    │   │
│  │  │  "s7"         ──► S7Pool                                        │    │   │
│  │  └─────────────────────────────────────────────────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐        │
│  │  ModbusPool  │  │  OPCUAPool   │  │    S7Pool    │  │MQTTPublisher │        │
│  │              │  │              │  │              │  │              │        │
│  │ ┌──────────┐ │  │ ┌──────────┐ │  │ ┌──────────┐ │  │ ┌──────────┐ │        │
│  │ │  Client  │ │  │ │  Session │ │  │ │  Client  │ │  │ │  Client  │ │        │
│  │ │  Pool    │ │  │ │   Pool   │ │  │ │  Pool    │ │  │ │  Buffer  │ │        │
│  │ └──────────┘ │  │ └──────────┘ │  │ └──────────┘ │  │ └──────────┘ │        │
│  │              │  │              │  │              │  │              │        │
│  │ ┌──────────┐ │  │ ┌──────────┐ │  │ ┌──────────┐ │  │ ┌──────────┐ │        │
│  │ │ Circuit  │ │  │ │  Load    │ │  │ │ Circuit  │ │  │ │  Topic   │ │        │
│  │ │ Breakers │ │  │ │ Shaper   │ │  │ │ Breakers │ │  │ │ Tracker  │ │        │
│  │ └──────────┘ │  │ └──────────┘ │  │ └──────────┘ │  │ └──────────┘ │        │
│  │              │  │              │  │              │  │              │        │
│  │ ┌──────────┐ │  │ ┌──────────┐ │  │ ┌──────────┐ │  │ ┌──────────┐ │        │
│  │ │  Batch   │ │  │ │Subscribe │ │  │ │  Batch   │ │  │ │   QoS    │ │        │
│  │ │ Optimizer│ │  │ │ Manager  │ │  │ │ Optimizer│ │  │ │ Handler  │ │        │
│  │ └──────────┘ │  │ └──────────┘ │  │ └──────────┘ │  │ └──────────┘ │        │
│  │              │  │              │  │              │  │              │        │
│  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘        │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

#### 4.2.2 Protocol Adapter Source File Structure

Each protocol adapter follows a consistent file organization pattern to ensure maintainability and ease of navigation. The standardized structure separates concerns into dedicated files:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                    PROTOCOL ADAPTER FILE STRUCTURE                             │
│                                                                                │
│  internal/adapter/modbus/           internal/adapter/s7/                       │
│  ├── types.go      ◄─────────────── ├── types.go      (Core type definitions)  │
│  ├── client.go     ◄─────────────── ├── client.go     (Protocol client impl)   │
│  ├── pool.go       ◄─────────────── ├── pool.go       (Connection pooling)     │
│  ├── health.go     ◄─────────────── ├── health.go     (Health monitoring)      │
│  └── conversion.go ◄─────────────── └── conversion.go (Data type conversion)   │
│                                                                                │
│  internal/adapter/opcua/            (OPC UA has additional protocol-specific   │
│  ├── types.go                        files due to session/subscription model)  │
│  ├── client.go                                                                 │
│  ├── pool.go                                                                   │
│  ├── health.go                                                                 │
│  ├── conversion.go                                                             │
│  ├── session.go     ◄─── Per-endpoint session management                       │
│  ├── subscription.go◄─── OPC UA subscription/monitored items                   │
│  └── loadshaping.go ◄─── Three-tier load control system                        │
│                                                                                │
│  FILE RESPONSIBILITIES:                                                        │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │  types.go      │ Client struct, ClientConfig, ClientStats, TagDiagnostic │  │
│  │                │ PoolConfig, PoolStats, DeviceHealth, BufferPool         │  │
│  │────────────────┼─────────────────────────────────────────────────────────│  │
│  │  client.go     │ Client constructor, ReadTags, ReadTag, WriteTag         │  │
│  │                │ Connection management, protocol-specific operations     │  │
│  │────────────────┼─────────────────────────────────────────────────────────│  │
│  │  pool.go       │ Pool constructor, per-device client management          │  │
│  │                │ Circuit breaker integration, idle connection reaping    │  │
│  │────────────────┼─────────────────────────────────────────────────────────│  │
│  │  health.go     │ GetTagDiagnostic, GetDeviceStats, GetAllDeviceHealth    │  │
│  │                │ recordTagSuccess, recordTagError, diagnostics tracking  │  │
│  │────────────────┼─────────────────────────────────────────────────────────│  │
│  │  conversion.go │ parseValue, valueToBytes, applyScaling, reverseScaling  │  │
│  │                │ Byte order handling, type coercion (toBool, toInt64...) │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
│                                                                                │
│  DESIGN RATIONALE:                                                             │
│  • Separation of concerns enables targeted modifications                       │
│  • Consistent structure across protocols reduces cognitive load                │
│  • Health and conversion logic isolated for easier testing                     │
│  • Types file provides single source of truth for data structures              │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---

## 5. Domain Model

### 5.1 Validation Logic

Every domain entity includes comprehensive validation to ensure configuration correctness before runtime:

```go
func (d *Device) Validate() error {
    var errs []error
    
    if d.ID == "" {
        errs = append(errs, ErrDeviceIDRequired)
    }
    if d.Name == "" {
        errs = append(errs, ErrDeviceNameRequired)
    }
    if d.Protocol == "" {
        errs = append(errs, ErrProtocolRequired)
    }
    if len(d.Tags) == 0 {
        errs = append(errs, ErrNoTagsDefined)
    }
    if d.PollInterval < 100*time.Millisecond {
        errs = append(errs, ErrPollIntervalTooShort)
    }
    if d.UNSPrefix == "" {
        errs = append(errs, ErrUNSPrefixRequired)
    }
    
    // Validate each tag for the device's protocol
    for _, tag := range d.Tags {
        if err := tag.ValidateForProtocol(d.Protocol); err != nil {
            errs = append(errs, fmt.Errorf("tag %s: %w", tag.ID, err))
        }
    }
    
    if len(errs) > 0 {
        return errors.Join(errs...)
    }
    return nil
}
```

### 5.2 Error Taxonomy

The gateway classifies errors into four categories based on their origin and recoverability. This taxonomy drives automated recovery decisions—configuration errors require human intervention, connection errors trigger circuit breakers and retries, protocol errors may indicate device misconfiguration, and service errors affect API responses. Understanding this classification helps operators diagnose issues quickly:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                            ERROR CLASSIFICATION                                │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    CONFIGURATION ERRORS (Preventable)                   │   │
│  │                                                                         │   │
│  │  • ErrDeviceIDRequired      - Device must have an ID                    │   │
│  │  • ErrDeviceNameRequired    - Device must have a name                   │   │
│  │  • ErrProtocolRequired      - Protocol must be specified                │   │
│  │  • ErrNoTagsDefined         - At least one tag required                 │   │
│  │  • ErrPollIntervalTooShort  - Minimum 100ms to prevent overload         │   │
│  │  • ErrUNSPrefixRequired     - UNS compliance requires prefix            │   │
│  │  • ErrInvalidDataType       - Unknown data type                         │   │
│  │  • ErrInvalidRegisterType   - Unknown register type                     │   │
│  │                                                                         │   │
│  │  → These should be caught at configuration time                         │   │
│  │  → Validation runs before device registration                           │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    CONNECTION ERRORS (Runtime, Retryable)               │   │
│  │                                                                         │   │
│  │  • ErrConnectionFailed      - Initial connection failed                 │   │
│  │  • ErrConnectionTimeout     - Connection attempt timed out              │   │
│  │  • ErrConnectionClosed      - Connection unexpectedly closed            │   │
│  │  • ErrConnectionReset       - Connection reset by peer                  │   │
│  │  • ErrMaxRetriesExceeded    - All retry attempts exhausted              │   │
│  │  • ErrPoolExhausted         - No connections available                  │   │
│  │                                                                         │   │
│  │  → Trigger circuit breaker evaluation                                   │   │
│  │  → May trigger reconnection logic                                       │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    PROTOCOL ERRORS (Runtime, May Be Fatal)              │   │
│  │                                                                         │   │
│  │  MODBUS:                                                                │   │
│  │  • ErrModbusIllegalFunction     - FC not supported by device            │   │
│  │  • ErrModbusIllegalAddress      - Register address out of range         │   │
│  │  • ErrModbusIllegalDataValue    - Invalid data in request               │   │
│  │  • ErrModbusSlaveDeviceFailure  - Device internal error                 │   │
│  │  • ErrModbusGatewayPathUnavail  - Gateway routing error                 │   │
│  │                                                                         │   │
│  │  OPC UA:                                                                │   │
│  │  • ErrOPCUAInvalidNodeID        - Node doesn't exist                    │   │
│  │  • ErrOPCUASubscriptionFailed   - Can't create subscription             │   │
│  │  • ErrOPCUASecurityRejected     - Security policy mismatch              │   │
│  │  • ErrOPCUASessionInvalid       - Session expired/invalid               │   │
│  │  • ErrOPCUATooManySessions      - Server session limit reached          │   │
│  │                                                                         │   │
│  │  S7:                                                                    │   │
│  │  • ErrS7InvalidAddress          - Invalid S7 address format             │   │
│  │  • ErrS7AccessDenied            - CPU protection active                 │   │
│  │  • ErrS7ItemNotAvailable        - DB/area doesn't exist                 │   │
│  │                                                                         │   │
│  │  → Some may trigger device-level circuit breaker                        │   │
│  │  → InvalidNodeID/Address suggest config error                           │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    SERVICE ERRORS (Operational)                         │   │
│  │                                                                         │   │
│  │  • ErrServiceNotStarted         - Service not yet initialized           │   │
│  │  • ErrServiceStopped            - Service has been stopped              │   │
│  │  • ErrServiceOverloaded         - Back-pressure active                  │   │
│  │  • ErrDeviceNotFound            - Unknown device ID                     │   │
│  │  • ErrTagNotFound               - Unknown tag ID                        │   │
│  │  • ErrProtocolNotSupported      - Protocol not registered               │   │
│  │  • ErrCircuitBreakerOpen        - Operations blocked                    │   │
│  │                                                                         │   │
│  │  → Typically returned to API callers                                    │   │
│  │  → May indicate system misconfiguration                                 │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---

## 6. Protocol Adapters

### 6.1 Modbus Adapter

#### 6.1.1 Architecture

The Modbus adapter uses a per-device connection model with individual circuit breakers to isolate failures. The diagram shows the connection pool structure and the batch optimization algorithm that groups contiguous registers into single read operations, reducing network round-trips by up to 70% for typical configurations:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                          MODBUS ADAPTER ARCHITECTURE                           │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                            ModbusPool                                   │   │
│  │                                                                         │   │
│  │  ┌───────────────────────────────────────────────────────────────────┐  │   │
│  │  │                    Connection Management                          │  │   │
│  │  │                                                                   │  │   │
│  │  │  clients map[string]*ModbusClient   // deviceID → client          │  │   │
│  │  │  breakers map[string]*gobreaker.CircuitBreaker                    │  │   │
│  │  │                                                                   │  │   │
│  │  │  Per-Device Connection Model:                                     │  │   │
│  │  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐             │  │   │
│  │  │  │   Device A   │  │   Device B   │  │   Device C   │             │  │   │
│  │  │  │              │  │              │  │              │             │  │   │
│  │  │  │ ┌──────────┐ │  │ ┌──────────┐ │  │ ┌──────────┐ │             │  │   │
│  │  │  │ │ TCP Conn │ │  │ │ TCP Conn │ │  │ │ TCP Conn │ │             │  │   │
│  │  │  │ │ 10.0.0.1 │ │  │ │ 10.0.0.2 │ │  │ │ 10.0.0.3 │ │             │  │   │
│  │  │  │ └──────────┘ │  │ └──────────┘ │  │ └──────────┘ │             │  │   │
│  │  │  │ ┌──────────┐ │  │ ┌──────────┐ │  │ ┌──────────┐ │             │  │   │
│  │  │  │ │ Breaker  │ │  │ │ Breaker  │ │  │ │ Breaker  │ │             │  │   │
│  │  │  │ │ (Closed) │ │  │ │ (Open)   │ │  │ │ (Half)   │ │             │  │   │
│  │  │  │ └──────────┘ │  │ └──────────┘ │  │ └──────────┘ │             │  │   │
│  │  │  └──────────────┘  └──────────────┘  └──────────────┘             │  │   │
│  │  └───────────────────────────────────────────────────────────────────┘  │   │
│  │                                                                         │   │
│  │  ┌───────────────────────────────────────────────────────────────────┐  │   │
│  │  │                    Batch Optimization                             │  │   │
│  │  │                                                                   │  │   │
│  │  │  Input Tags:  [R100, R101, R102, R103, R110, R111, R200]          │  │   │
│  │  │                                                                   │  │   │
│  │  │  Grouping Algorithm:                                              │  │   │
│  │  │  1. Sort by RegisterType                                          │  │   │
│  │  │  2. Within type, sort by Address                                  │  │   │
│  │  │  3. Find contiguous ranges (max gap = 10)                         │  │   │
│  │  │  4. Split at MAX_REGISTERS_PER_READ (100)                         │  │   │
│  │  │                                                                   │  │   │
│  │  │  Output Groups:                                                   │  │   │
│  │  │  ┌─────────────────────────────────────────────────────────────┐  │  │   │
│  │  │  │ Group 1: [R100-R103] → ReadHoldingRegisters(100, 4)         │  │  │   │
│  │  │  │ Group 2: [R110-R111] → ReadHoldingRegisters(110, 2)         │  │  │   │
│  │  │  │ Group 3: [R200]      → ReadHoldingRegisters(200, 1)         │  │  │   │
│  │  │  └─────────────────────────────────────────────────────────────┘  │  │   │
│  │  │                                                                   │  │   │
│  │  │  Benefit: 7 tags read with 3 requests instead of 7                │  │   │
│  │  └───────────────────────────────────────────────────────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

#### 6.1.2 Byte Order Handling

Modbus devices from different vendors use various byte ordering schemes for multi-register values. The gateway supports all four common formats. This diagram illustrates how a 32-bit value (0x12345678) is represented across two 16-bit registers in each format, essential for correctly interpreting data from devices like ABB, Schneider, Siemens, and legacy systems:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                          MODBUS BYTE ORDER FORMATS                             │
│                                                                                │
│  32-bit Value: 0x12345678 (Decimal: 305,419,896)                               │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  BIG ENDIAN (ABCD) - Most common, Modbus standard                       │   │
│  │                                                                         │   │
│  │  Register 0: 0x1234    Register 1: 0x5678                               │   │
│  │  Bytes:      [12][34]             [56][78]                              │   │
│  │                                                                         │   │
│  │  Used by: ABB, Schneider, most IEC-compliant devices                    │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  LITTLE ENDIAN (DCBA)                                                   │   │
│  │                                                                         │   │
│  │  Register 0: 0x7856    Register 1: 0x3412                               │   │
│  │  Bytes:      [78][56]             [34][12]                              │   │
│  │                                                                         │   │
│  │  Used by: Some PLCs with Intel heritage                                 │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  MID-BIG ENDIAN (BADC) - Word swap                                      │   │
│  │                                                                         │   │
│  │  Register 0: 0x3412    Register 1: 0x7856                               │   │
│  │  Bytes:      [34][12]             [78][56]                              │   │
│  │                                                                         │   │
│  │  Used by: Some Siemens, Daniel, Emerson devices                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  MID-LITTLE ENDIAN (CDAB) - Byte swap + Word swap                       │   │
│  │                                                                         │   │
│  │  Register 0: 0x5678    Register 1: 0x1234                               │   │
│  │  Bytes:      [56][78]             [12][34]                              │   │
│  │                                                                         │   │
│  │  Used by: Some legacy systems                                           │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 6.2 OPC UA Adapter

#### 6.2.1 Session Architecture

OPC UA sessions are heavyweight resources with security context and subscription state. The gateway implements per-endpoint session sharing—multiple devices connecting to the same OPC UA server share a single session. This design scales to 200+ devices even when connecting to servers with strict session limits (typical Kepware limit: 50-100 sessions). The diagram shows how devices are grouped by endpoint URL for session sharing:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                          OPC UA SESSION ARCHITECTURE                           │
│                                                                                │
│  Key Design Decision: Per-Endpoint Session Sharing                             │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                         │   │
│  │   DEVICES                           SESSIONS                            │   │
│  │                                                                         │   │
│  │   ┌──────────────┐                 ┌──────────────────────────────────┐ │   │
│  │   │   Device A   │─────────┐       │     Endpoint Session #1          │ │   │
│  │   │ opc.tcp://   │         │       │                                  │ │   │
│  │   │ srv1:4840    │         ├──────►│  Endpoint: opc.tcp://srv1:4840   │ │   │
│  │   └──────────────┘         │       │  Security: None                  │ │   │
│  │                            │       │  Auth: Anonymous                 │ │   │
│  │   ┌──────────────┐         │       │                                  │ │   │
│  │   │   Device B   │─────────┘       │  ┌───────────────────────────┐   │ │   │
│  │   │ opc.tcp://   │                 │  │ Monitored Items:          │   │ │   │
│  │   │ srv1:4840    │                 │  │  • Device A tags          │   │ │   │
│  │   └──────────────┘                 │  │  • Device B tags          │   │ │   │
│  │                                    │  └───────────────────────────┘   │ │   │
│  │   ┌──────────────┐                 └──────────────────────────────────┘ │   │
│  │   │   Device C   │─────────┐                                            │   │
│  │   │ opc.tcp://   │         │       ┌──────────────────────────────────┐ │   │
│  │   │ srv2:4840    │         ├──────►│     Endpoint Session #2          │ │   │
│  │   └──────────────┘         │       │                                  │ │   │
│  │                            │       │  Endpoint: opc.tcp://srv2:4840   │ │   │
│  │   ┌──────────────┐         │       │  Security: Basic256Sha256        │ │   │
│  │   │   Device D   │─────────┘       │  Auth: Username/Password         │ │   │
│  │   │ opc.tcp://   │                 │                                  │ │   │
│  │   │ srv2:4840    │                 │  ┌───────────────────────────┐   │ │   │
│  │   │ (Same EP)    │                 │  │ Monitored Items:          │   │ │   │
│  │   └──────────────┘                 │  │  • Device C tags          │   │ │   │
│  │                                    │  │  • Device D tags          │   │ │   │
│  │                                    │  └───────────────────────────┘   │ │   │
│  │                                    └──────────────────────────────────┘ │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Endpoint Key Generation:                                                      │
│  key = sha256(host + port + securityPolicy + securityMode + authMode +         │
│               username + sha256(certFileContents))                             │
│                                                                                │
│  Benefits:                                                                     │
│  • Kepware server limit: 50-100 sessions → Support 200+ gateway devices        │
│  • Reduced network overhead                                                    │
│  • Shared subscription management                                              │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

#### 6.2.2 Session State Machine

OPC UA sessions transition through multiple states during their lifecycle. This state machine diagram shows the transitions from Disconnected through Active, with error handling and recovery paths. State tracking variables (`lastUsed`, `lastPublishTime`, `consecutiveFailures`) enable intelligent idle timeout management that respects active subscriptions:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                        OPC UA SESSION STATE MACHINE                            │
│                                                                                │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │                                                                          │  │
│  │                            ┌─────────────┐                               │  │
│  │                            │ Disconnected│◄────────────────────────────┐ │  │
│  │                            │             │                             │ │  │
│  │                            └──────┬──────┘                             │ │  │
│  │                                   │                                    │ │  │
│  │                                   │ Connect()                          │ │  │
│  │                                   ▼                                    │ │  │
│  │                            ┌─────────────┐                             │ │  │
│  │                            │ Connecting  │                             │ │  │
│  │                            │             │                             │ │  │
│  │                            └──────┬──────┘                             │ │  │
│  │                                   │                                    │ │  │
│  │                    Success        │        Failure                     │ │  │
│  │               ┌───────────────────┼───────────────────┐                │ │  │
│  │               │                   │                   │                │ │  │
│  │               ▼                   │                   ▼                │ │  │
│  │        ┌─────────────┐            │            ┌─────────────┐         │ │  │
│  │        │SecureChannel│            │            │    Error    │─────────┘ │  │
│  │        │ Established │            │            │             │   Retry   │  │
│  │        └──────┬──────┘            │            └─────────────┘  Backoff  │  │
│  │               │                   │                                      │  │
│  │               │ Session Created   │                                      │  │
│  │               ▼                   │                                      │  │
│  │        ┌─────────────┐            │                                      │  │
│  │        │   Active    │◄───────────┘                                      │  │
│  │        │             │    Reconnect                                      │  │
│  │        └──────┬──────┘                                                   │  │
│  │               │                                                          │  │
│  │               │ Close() / Error                                          │  │
│  │               ▼                                                          │  │
│  │        ┌─────────────┐                                                   │  │
│  │        │    Error    │                                                   │  │
│  │        │             │                                                   │  │
│  │        └─────────────┘                                                   │  │
│  │                                                                          │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
│                                                                                │
│  State Tracking:                                                               │
│  • lastUsed: Prevents idle timeout during active reads                         │
│  • lastPublishTime: Prevents idle timeout with active subscriptions            │
│  • consecutiveFailures: Triggers exponential backoff                           │
│  • hasActiveSubscriptions: Preserves sessions with monitored items             │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

#### 6.2.3 Load Shaping System

The OPC UA adapter implements a three-tier load control system to prevent overwhelming servers or the gateway itself. The diagram shows: (1) global operation limits across all endpoints, (2) per-endpoint limits preventing "noisy neighbor" problems, and (3) priority queues ensuring control and safety operations proceed even during overload conditions. Brownout mode automatically sheds telemetry traffic while preserving critical operations:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                        OPC UA LOAD SHAPING SYSTEM                              │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                         THREE-TIER LOAD CONTROL                         │   │
│  │                                                                         │   │
│  │  TIER 1: Global Limit                                                   │   │
│  │  ┌────────────────────────────────────────────────────────────────────┐ │   │
│  │  │  MaxGlobalInFlight: 1000 concurrent operations                     │ │   │
│  │  │                                                                    │ │   │
│  │  │  All endpoints share this limit. Prevents gateway from overwhelming│ │   │
│  │  │  itself under high device count scenarios.                         │ │   │
│  │  └────────────────────────────────────────────────────────────────────┘ │   │
│  │                                                                         │   │
│  │  TIER 2: Per-Endpoint Limit                                             │   │
│  │  ┌────────────────────────────────────────────────────────────────────┐ │   │
│  │  │  MaxInFlightPerEndpoint: 100 concurrent operations per endpoint    │ │   │
│  │  │                                                                    │ │   │
│  │  │  Prevents "noisy neighbor" - one slow/overloaded OPC server        │ │   │
│  │  │  cannot consume all global capacity.                               │ │   │
│  │  └────────────────────────────────────────────────────────────────────┘ │   │
│  │                                                                         │   │
│  │  TIER 3: Priority Queues                                                │   │
│  │  ┌────────────────────────────────────────────────────────────────────┐ │   │
│  │  │                                                                    │ │   │
│  │  │  Priority 0: TELEMETRY (lowest)                                    │ │   │
│  │  │  ┌──────────────────────────────────────────────────────────────┐  │ │   │
│  │  │  │ Regular polling reads. Dropped first under brownout.         │  │ │   │
│  │  │  └──────────────────────────────────────────────────────────────┘  │ │   │
│  │  │                                                                    │ │   │
│  │  │  Priority 1: CONTROL                                               │ │   │
│  │  │  ┌──────────────────────────────────────────────────────────────┐  │ │   │
│  │  │  │ Write operations, setpoint changes.Processed before telemetry│  │ │   │
│  │  │  └──────────────────────────────────────────────────────────────┘  │ │   │
│  │  │                                                                    │ │   │
│  │  │  Priority 2: SAFETY (highest)                                      │ │   │
│  │  │  ┌──────────────────────────────────────────────────────────────┐  │ │   │
│  │  │  │ Safety-critical operations. Never dropped.                   │  │ │   │
│  │  │  └──────────────────────────────────────────────────────────────┘  │ │   │
│  │  │                                                                    │ │   │
│  │  └────────────────────────────────────────────────────────────────────┘ │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                         BROWNOUT MODE                                   │   │
│  │                                                                         │   │
│  │  Trigger: global_in_flight > MaxGlobalInFlight * BrownoutThreshold      │   │
│  │           (default: 80% of 1000 = 800)                                  │   │
│  │                                                                         │   │
│  │  Behavior:                                                              │   │
│  │  ┌────────────────────────────────────────────────────────────────────┐ │   │
│  │  │  ┌─────────────┐                                                   │ │   │
│  │  │  │  TELEMETRY  │  → REJECTED with ErrServiceOverloaded             │ │   │
│  │  │  └─────────────┘                                                   │ │   │
│  │  │  ┌─────────────┐                                                   │ │   │
│  │  │  │   CONTROL   │  → ALLOWED (processed from queue)                 │ │   │
│  │  │  └─────────────┘                                                   │ │   │
│  │  │  ┌─────────────┐                                                   │ │   │
│  │  │  │   SAFETY    │  → ALLOWED (highest priority)                     │ │   │
│  │  │  └─────────────┘                                                   │ │   │
│  │  └────────────────────────────────────────────────────────────────────┘ │   │
│  │                                                                         │   │
│  │  Exit: global_in_flight < MaxGlobalInFlight * BrownoutThreshold * 0.4   │   │
│  │        (default: 320)                                                   │   │
│  │                                                                         │   │
│  │  Hysteresis prevents mode flapping.                                     │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 6.3 S7 Adapter

#### 6.3.1 Address Parsing

Siemens S7 PLCs use symbolic addressing to access different memory areas. The diagram below documents all supported address formats including Data Blocks (DB), Merker memory (M), Inputs (I), Outputs (Q), Timers (T), and Counters (C). The parsing algorithm extracts area, DB number, offset, and data size from symbolic addresses like "DB1.DBD0":

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                         S7 ADDRESS PARSING                                     │
│                                                                                │
│  Symbolic Address Format:                                                      │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                         │   │
│  │  DATA BLOCKS (DB)                                                       │   │
│  │  ─────────────────                                                      │   │
│  │  DB1.DBX0.0   → Data Block 1, Byte 0, Bit 0 (Boolean)                   │   │
│  │  DB1.DBB0     → Data Block 1, Byte 0 (Byte/Int8)                        │   │
│  │  DB1.DBW0     → Data Block 1, Word at Byte 0 (Int16/UInt16)             │   │
│  │  DB1.DBD0     → Data Block 1, DWord at Byte 0 (Int32/UInt32/Float32)    │   │
│  │  DB10.DBD100  → Data Block 10, DWord at Byte 100                        │   │
│  │                                                                         │   │
│  │  MERKER (M) - Internal Memory                                           │   │
│  │  ─────────────────────────────                                          │   │
│  │  M0.0         → Merker Byte 0, Bit 0                                    │   │
│  │  MB100        → Merker Byte 100                                         │   │
│  │  MW100        → Merker Word at Byte 100                                 │   │
│  │  MD100        → Merker DWord at Byte 100                                │   │
│  │                                                                         │   │
│  │  INPUTS (I) - Process Image Input                                       │   │
│  │  ───────────────────────────────                                        │   │
│  │  I0.0         → Input Byte 0, Bit 0                                     │   │
│  │  IB0          → Input Byte 0                                            │   │
│  │  IW0          → Input Word at Byte 0                                    │   │
│  │  ID0          → Input DWord at Byte 0                                   │   │
│  │                                                                         │   │
│  │  OUTPUTS (Q) - Process Image Output                                     │   │
│  │  ────────────────────────────────                                       │   │
│  │  Q0.0         → Output Byte 0, Bit 0                                    │   │
│  │  QB0          → Output Byte 0                                           │   │
│  │  QW0          → Output Word at Byte 0                                   │   │
│  │  QD0          → Output DWord at Byte 0                                  │   │
│  │                                                                         │   │
│  │  TIMERS (T) and COUNTERS (C)                                            │   │
│  │  ───────────────────────────                                            │   │
│  │  T0           → Timer 0                                                 │   │
│  │  C0           → Counter 0                                               │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Parsing Algorithm:                                                            │
│                                                                                │
│  Input: "DB1.DBD0"                                                             │
│  1. Parse area: "DB" → S7AreaDB                                                │
│  2. Parse DB number: "1" → DBNumber = 1                                        │
│  3. Parse access type: "DBD" → DWord (4 bytes)                                 │
│  4. Parse offset: "0" → ByteOffset = 0                                         │
│                                                                                │
│  Result: { Area: DB, DBNumber: 1, Offset: 0, Size: 4 }                         │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 6.4 MQTT Publisher

#### 6.4.1 Message Flow Architecture

The MQTT Publisher handles all outbound message delivery with automatic buffering during broker disconnections. This diagram traces a DataPoint from serialization through connection checking to either immediate publish or ring buffer storage. The reconnection flow shows exponential backoff and automatic buffer draining upon reconnection, ensuring no data loss during transient network issues:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                         MQTT PUBLISHER ARCHITECTURE                            │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                         PUBLISH FLOW                                    │   │
│  │                                                                         │   │
│  │   DataPoint                                                             │   │
│  │      │                                                                  │   │
│  │      ▼                                                                  │   │
│  │  ┌─────────────────────────────────────────────────────────────────┐    │   │
│  │  │                    JSON Serialization                           │    │   │
│  │  │                                                                 │    │   │
│  │  │  {                                                              │    │   │
│  │  │    "device_id": "plc-001",                                      │    │   │
│  │  │    "tag_id": "temperature",                                     │    │   │
│  │  │    "value": 25.5,                                               │    │   │
│  │  │    "unit": "°C",                                                │    │   │
│  │  │    "quality": "good",                                           │    │   │
│  │  │    "timestamp": "2024-01-15T10:30:00.123Z",                     │    │   │
│  │  │    "source_timestamp": "2024-01-15T10:29:59.998Z"               │    │   │
│  │  │  }                                                              │    │   │
│  │  └─────────────────────────────────────────────────────────────────┘    │   │
│  │      │                                                                  │   │
│  │      ▼                                                                  │   │
│  │  ┌─────────────────────────────────────────────────────────────────┐    │   │
│  │  │                    Connection Check                             │    │   │
│  │  │                                                                 │    │   │
│  │  │  Connected? ─────────────────────────────────────────┐          │    │   │
│  │  │      │                                               │          │    │   │
│  │  │      │ YES                                           │ NO       │    │   │
│  │  │      ▼                                               ▼          │    │   │
│  │  │  ┌────────────┐                               ┌────────────┐    │    │   │
│  │  │  │  Publish   │                               │  Buffer    │    │    │   │
│  │  │  │  to Broker │                               │  Message   │    │    │   │
│  │  │  └────────────┘                               └────────────┘    │    │   │
│  │  │                                                      │          │    │   │
│  │  │                                                      ▼          │    │   │
│  │  │                                            ┌────────────────┐   │    │   │
│  │  │                                            │   Ring Buffer  │   │    │   │
│  │  │                                            │ (10,000 msgs)  │   │    │   │
│  │  │                                            │                │   │    │   │
│  │  │                                            │ Overflow:      │   │    │   │
│  │  │                                            │ Drop oldest    │   │    │   │
│  │  │                                            └────────────────┘   │    │   │
│  │  └─────────────────────────────────────────────────────────────────┘    │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                         RECONNECTION FLOW                               │   │
│  │                                                                         │   │
│  │  ┌──────────────┐                                                       │   │
│  │  │ Disconnected │                                                       │   │
│  │  └───────┬──────┘                                                       │   │
│  │          │                                                              │   │
│  │          ▼                                                              │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │              Auto-Reconnect with Exponential Backoff             │   │   │
│  │  │                                                                  │   │   │
│  │  │  Attempt 1: Wait 5s    → Try connect                             │   │   │
│  │  │  Attempt 2: Wait 10s   → Try connect                             │   │   │
│  │  │  Attempt 3: Wait 20s   → Try connect                             │   │   │
│  │  │  ...                                                             │   │   │
│  │  │  Max wait: 5 minutes                                             │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │          │                                                              │   │
│  │          ▼ On Success                                                   │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Drain Buffer                                  │   │   │
│  │  │                                                                  │   │   │
│  │  │  While buffer not empty:                                         │   │   │
│  │  │    1. Dequeue oldest message                                     │   │   │
│  │  │    2. Publish to broker                                          │   │   │
│  │  │    3. Apply rate limiting (optional)                             │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---

## 7. Connection Management

### 7.1 Connection Pooling Strategies

Each protocol requires a different connection pooling strategy based on its characteristics. This comparison diagram explains the rationale behind per-device pooling (Modbus, S7) versus per-endpoint session sharing (OPC UA), along with configuration parameters and trade-offs. Understanding these differences is essential for capacity planning and troubleshooting:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                    CONNECTION POOLING STRATEGIES COMPARISON                    │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    MODBUS: Per-Device Pooling                           │   │
│  │                                                                         │   │
│  │  Rationale:                                                             │   │
│  │  • Modbus TCP maintains stateless connections                           │   │
│  │  • Each device has unique IP:Port combination                           │   │
│  │  • Slave ID is per-request, not per-connection                          │   │
│  │  • Simple 1:1 mapping simplifies health tracking                        │   │
│  │                                                                         │   │
│  │  Configuration:                                                         │   │
│  │  • Max connections: 500 (configurable)                                  │   │
│  │  • Idle timeout: 5 minutes                                              │   │
│  │  • Health check: 30 seconds                                             │   │
│  │                                                                         │   │
│  │  Trade-offs:                                                            │   │
│  │  + Simple failure isolation                                             │   │
│  │  + Easy to debug connection issues                                      │   │
│  │  - Higher memory for many devices                                       │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                 OPC UA: Per-Endpoint Session Sharing                    │   │
│  │                                                                         │   │
│  │  Rationale:                                                             │   │
│  │  • OPC UA sessions are heavyweight (security context, subscriptions)    │   │
│  │  • Servers like Kepware limit concurrent sessions (50-100)              │   │
│  │  • Multiple devices often share same OPC server                         │   │
│  │  • Subscription management benefits from session sharing                │   │
│  │                                                                         │   │
│  │  Configuration:                                                         │   │
│  │  • Max endpoint sessions: 100 (not devices!)                            │   │
│  │  • Idle timeout: 5 minutes (respects active subscriptions)              │   │
│  │  • Health check: 30 seconds                                             │   │
│  │                                                                         │   │
│  │  Trade-offs:                                                            │   │
│  │  + Scales to 200+ devices on limited servers                            │   │
│  │  + Shared subscription infrastructure                                   │   │
│  │  - More complex failure isolation                                       │   │
│  │  - Endpoint-level failures affect multiple devices                      │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    S7: Per-Device Pooling                               │   │
│  │                                                                         │   │
│  │  Rationale:                                                             │   │
│  │  • S7 connections are established per PLC (Rack/Slot)                   │   │
│  │  • Connection state is maintained (unlike Modbus)                       │   │
│  │  • PDU size negotiated per connection                                   │   │
│  │  • Natural 1:1 mapping to physical PLCs                                 │   │
│  │                                                                         │   │
│  │  Configuration:                                                         │   │
│  │  • Max connections: 100                                                 │   │
│  │  • Idle timeout: 5 minutes                                              │   │
│  │  • Health check: 30 seconds                                             │   │
│  │                                                                         │   │
│  │  Trade-offs:                                                            │   │
│  │  + Matches physical topology                                            │   │
│  │  + Simple health/failure model                                          │   │
│  │  - Connection limits per PLC (typically 8-32)                           │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 7.2 Idle Connection Management

Idle connections consume server-side resources and may become stale. The background reaper goroutine periodically evaluates connections against three criteria: last usage time, active subscriptions (OPC UA), and connection health state. This diagram documents the reaping algorithm that balances resource efficiency with operational continuity:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                      IDLE CONNECTION REAPING                                   │
│                                                                                │
│  Background reaper goroutine runs every IdleTimeout/2 (2.5 minutes default)    │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                         │   │
│  │  for each connection in pool:                                           │   │
│  │                                                                         │   │
│  │    ┌─────────────────────────────────────────────────────────────────┐  │   │
│  │    │  Check 1: Last Used Time                                        │  │   │
│  │    │                                                                 │  │   │
│  │    │  if (now - lastUsed) > IdleTimeout:                             │  │   │
│  │    │      mark for removal                                           │  │   │
│  │    └─────────────────────────────────────────────────────────────────┘  │   │
│  │                                                                         │   │
│  │    ┌─────────────────────────────────────────────────────────────────┐  │   │
│  │    │  Check 2: Active Subscriptions (OPC UA only)                    │  │   │
│  │    │                                                                 │  │   │
│  │    │  if hasActiveSubscriptions:                                     │  │   │
│  │    │      skip removal (subscriptions keep session alive)            │  │   │
│  │    │                                                                 │  │   │
│  │    │  if (now - lastPublishTime) > SubscriptionIdleTimeout:          │  │   │
│  │    │      subscriptions may be stale, allow removal                  │  │   │
│  │    └─────────────────────────────────────────────────────────────────┘  │   │
│  │                                                                         │   │
│  │    ┌─────────────────────────────────────────────────────────────────┐  │   │
│  │    │  Check 3: Connection Health                                     │  │   │
│  │    │                                                                 │  │   │
│  │    │  if connection.State == Error:                                  │  │   │
│  │    │      mark for removal (will reconnect on next use)              │  │   │
│  │    └─────────────────────────────────────────────────────────────────┘  │   │
│  │                                                                         │   │
│  │  end for                                                                │   │
│  │                                                                         │   │
│  │  Remove marked connections, close gracefully                            │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Justification:                                                                │
│  • Prevents resource exhaustion from accumulated stale connections             │
│  • Frees server-side resources (important for licensed OPC servers)            │
│  • Allows natural recovery from transient network issues                       │
│  • Respects active work (subscriptions, in-flight operations)                  │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---

## 8. Data Flow Architecture

### 8.1 Read Path (Polling)

This comprehensive flowchart traces a complete polling cycle from timer tick to MQTT publish. Key stages include worker pool acquisition (with back-pressure handling), protocol manager routing, batch optimization, data transformation (scaling, quality assignment), and metric recording. Understanding this flow is essential for debugging data latency or missing values:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                            POLLING DATA FLOW                                   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                         │   │
│  │  ┌──────────────┐                                                       │   │
│  │  │   Device     │  Poll interval + jitter (0-10%)                       │   │
│  │  │   Ticker     │  ─────────────────────────────────►                   │   │
│  │  └──────────────┘                                                       │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Worker Pool Acquire                           │   │   │
│  │  │                                                                  │   │   │
│  │  │  ┌────────────────────────────────────────────────────────────┐  │   │   │
│  │  │  │  Worker available?                                         │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  YES ─► Proceed to read                                    │  │   │   │
│  │  │  │  NO  ─► Skip this poll cycle (back-pressure)               │  │   │   │
│  │  │  │         Increment skipped counter                          │  │   │   │
│  │  │  │         metrics.polling_polls_skipped_total++              │  │   │   │
│  │  │  └────────────────────────────────────────────────────────────┘  │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Tag Preparation                               │   │   │
│  │  │                                                                  │   │   │
│  │  │  1. Filter enabled tags only                                     │   │   │
│  │  │  2. Build tag lookup map (ID → Tag)                              │   │   │
│  │  │  3. Create context with device timeout                           │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Protocol Manager                              │   │   │
│  │  │                                                                  │   │   │
│  │  │  protocolManager.ReadTags(ctx, device, tags)                     │   │   │
│  │  │                                                                  │   │   │
│  │  │  ┌────────────────────────────────────────────────────────────┐  │   │   │
│  │  │  │  1. Lookup pool for device.Protocol                        │  │   │   │
│  │  │  │  2. Delegate to pool.ReadTags()                            │  │   │   │
│  │  │  └────────────────────────────────────────────────────────────┘  │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Protocol Pool (e.g., Modbus)                  │   │   │
│  │  │                                                                  │   │   │
│  │  │  ┌────────────────────────────────────────────────────────────┐  │   │   │
│  │  │  │  1. Check circuit breaker state                            │  │   │   │
│  │  │  │     - Open: return ErrCircuitBreakerOpen immediately       │  │   │   │
│  │  │  │     - Closed/Half-Open: proceed                            │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  2. Get or create connection                               │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  3. Batch optimization                                     │  │   │   │
│  │  │  │     - Group tags by register type                          │  │   │   │
│  │  │  │     - Find contiguous address ranges                       │  │   │   │
│  │  │  │     - Execute minimal number of read operations            │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  4. Execute read with retry                                │  │   │   │
│  │  │  │     - Exponential backoff on failure                       │  │   │   │
│  │  │  │     - Report success/failure to circuit breaker            │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  5. Parse response bytes                                   │  │   │   │
│  │  │  │     - Apply byte order conversion                          │  │   │   │
│  │  │  │     - Apply scale factor and offset                        │  │   │   │
│  │  │  │     - Create DataPoint with timestamps                     │  │   │   │
│  │  │  └────────────────────────────────────────────────────────────┘  │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Post-Processing                               │   │   │
│  │  │                                                                  │   │   │
│  │  │  For each DataPoint:                                             │   │   │
│  │  │  ┌────────────────────────────────────────────────────────────┐  │   │   │
│  │  │  │  1. Assign MQTT topic                                      │  │   │   │
│  │  │  │     topic = device.UNSPrefix + "/" + tag.TopicSuffix       │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  2. Quality filter                                         │  │   │   │
│  │  │  │     - Skip bad quality points (optional)                   │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  3. Deadband filter (if configured)                        │  │   │   │
│  │  │  │     - Skip if change < deadband threshold                  │  │   │   │
│  │  │  └────────────────────────────────────────────────────────────┘  │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    MQTT Publish                                  │   │   │
│  │  │                                                                  │   │   │
│  │  │  mqttPublisher.PublishBatch(dataPoints)                          │   │   │
│  │  │                                                                  │   │   │
│  │  │  ┌────────────────────────────────────────────────────────────┐  │   │   │
│  │  │  │  For each point:                                           │  │   │   │
│  │  │  │    - Serialize to JSON                                     │  │   │   │
│  │  │  │    - Publish to topic with configured QoS                  │  │   │   │
│  │  │  │    - Track in topic statistics                             │  │   │   │
│  │  │  │    - Record metrics (latency, count)                       │  │   │   │
│  │  │  └────────────────────────────────────────────────────────────┘  │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Cleanup & Metrics                             │   │   │
│  │  │                                                                  │   │   │
│  │  │  - Release worker back to pool                                   │   │   │
│  │  │  - Release DataPoints back to object pool                        │   │   │
│  │  │  - Update device status (last poll time, error count)            │   │   │
│  │  │  - Record poll duration in histogram                             │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 8.2 Write Path (Commands)

Bidirectional communication enables write operations from IT systems to industrial devices. This flowchart shows command processing from MQTT message receipt through validation, queue management (with back-pressure), rate-limited execution, and response publication. The queue-based architecture prevents write storms from overwhelming devices:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                            COMMAND WRITE FLOW                                  │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                         │   │
│  │  MQTT Message Received                                                  │   │
│  │  Topic: $nexus/cmd/{device_id}/write                                    │   │
│  │  Payload: { "tag_id": "setpoint", "value": 75.0, "request_id": "..." }  │   │
│  │                                                                         │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Topic Parsing                                 │   │   │
│  │  │                                                                  │   │   │
│  │  │  Extract: device_id = "plc-001"                                  │   │   │
│  │  │  Parse JSON payload                                              │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Validation                                    │   │   │
│  │  │                                                                  │   │   │
│  │  │  ┌────────────────────────────────────────────────────────────┐  │   │   │
│  │  │  │  1. Device exists? (O(1) lookup)                           │  │   │   │
│  │  │  │     NO ─► Publish error response                           │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  2. Tag exists on device? (O(1) lookup)                    │  │   │   │
│  │  │  │     NO ─► Publish error response                           │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  3. Tag writable? (AccessMode check)                       │  │   │   │
│  │  │  │     NO ─► Publish error response                           │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  4. Value type compatible?                                 │  │   │   │
│  │  │  │     NO ─► Publish error response                           │  │   │   │
│  │  │  └────────────────────────────────────────────────────────────┘  │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Queue Enqueue                                 │   │   │
│  │  │                                                                  │   │   │
│  │  │  ┌────────────────────────────────────────────────────────────┐  │   │   │
│  │  │  │  Queue full? (capacity: 1000)                              │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  YES ─► Reject command                                     │  │   │   │
│  │  │  │         Publish error: "command_queue_full"                │  │   │   │
│  │  │  │         metrics.commands_rejected++                        │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  NO  ─► Enqueue command                                    │  │   │   │
│  │  │  │         Non-blocking send to channel                       │  │   │   │
│  │  │  └────────────────────────────────────────────────────────────┘  │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Queue Processor (goroutine)                   │   │   │
│  │  │                                                                  │   │   │
│  │  │  ┌────────────────────────────────────────────────────────────┐  │   │   │
│  │  │  │  1. Acquire semaphore (max 50 concurrent writes)           │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  2. Apply reverse scaling                                  │  │   │   │
│  │  │  │     raw_value = (value - offset) / scale_factor            │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  3. Execute write via ProtocolManager                      │  │   │   │
│  │  │  │     protocolManager.WriteTag(ctx, device, tag, raw_value)  │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  4. Handle result                                          │  │   │   │
│  │  │  │     Success ─► Publish success response                    │  │   │   │
│  │  │  │     Failure ─► Publish error response                      │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  5. Release semaphore                                      │  │   │   │
│  │  │  └────────────────────────────────────────────────────────────┘  │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Response Message                              │   │   │
│  │  │                                                                  │   │   │
│  │  │  Topic: $nexus/cmd/response/{device_id}/{tag_id}                 │   │   │
│  │  │  Payload:                                                        │   │   │
│  │  │  {                                                               │   │   │
│  │  │    "request_id": "...",                                          │   │   │
│  │  │    "status": "success" | "error",                                │   │   │
│  │  │    "error": "error message if failed",                           │   │   │
│  │  │    "duration_ms": 45                                             │   │   │
│  │  │  }                                                               │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---

## 9. Resilience Patterns

### 9.1 Circuit Breaker Pattern

Circuit breakers prevent cascade failures by temporarily blocking requests to failing services. The state machine diagram shows the three states (Closed, Open, Half-Open) and transition conditions. Per-protocol configuration allows tuning failure thresholds and recovery timeouts based on device characteristics—fast recovery for Modbus, longer timeouts for OPC UA session issues:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                         CIRCUIT BREAKER STATE MACHINE                          │
│                                                                                │
│  Implementation: sony/gobreaker                                                │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                         │   │
│  │                         ┌──────────────┐                                │   │
│  │         Success         │              │         Failure                │   │
│  │     ┌──────────────────►│    CLOSED    │◄──────────────────┐            │   │
│  │     │                   │              │                   │            │   │
│  │     │                   │  (Normal     │                   │            │   │
│  │     │                   │   Operation) │                   │            │   │
│  │     │                   └──────┬───────┘                   │            │   │
│  │     │                          │                           │            │   │
│  │     │                          │ Failure threshold         │            │   │
│  │     │                          │ exceeded (60% of          │            │   │
│  │     │                          │ last 10 requests)         │            │   │
│  │     │                          │                           │            │   │
│  │     │                          ▼                           │            │   │
│  │     │                   ┌──────────────┐                   │            │   │
│  │     │                   │              │                   │            │   │
│  │     │                   │     OPEN     │───────────────────┘            │   │
│  │     │                   │              │  All requests                  │   │
│  │     │                   │  (Requests   │  fail immediately              │   │
│  │     │                   │   blocked)   │  with ErrCircuitBreakerOpen    │   │
│  │     │                   └──────┬───────┘                                │   │
│  │     │                          │                                        │   │
│  │     │                          │ Timeout (60 seconds)                   │   │
│  │     │                          │                                        │   │
│  │     │                          ▼                                        │   │
│  │     │                   ┌──────────────┐                                │   │
│  │     │                   │              │                                │   │
│  │     └───────────────────│  HALF-OPEN   │                                │   │
│  │         Success         │              │                                │   │
│  │         (probe          │  (Allow one  │────────────────────────────    │   │ 
│  │          passed)        │   request)   │  Failure (probe failed)        │   │
│  │                         └──────────────┘                                │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Configuration (per protocol):                                                 │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  MODBUS                                                                 │   │
│  │  • Requests threshold: 10                                               │   │
│  │  • Failure ratio: 0.6 (60%)                                             │   │
│  │  • Timeout: 60 seconds                                                  │   │
│  │  • Per-device breakers                                                  │   │
│  │                                                                         │   │
│  │  OPC UA (Two-tier)                                                      │   │
│  │  • Endpoint breaker:                                                    │   │
│  │    - Requests: 5                                                        │   │
│  │    - Ratio: 0.6                                                         │   │
│  │    - Triggers on: Connection errors, timeouts, TooManySessions          │   │
│  │  • Device breaker:                                                      │   │
│  │    - Consecutive failures: 5                                            │   │
│  │    - Triggers on: BadNodeID, BadUserAccessDenied, BadTypeMismatch       │   │
│  │                                                                         │   │
│  │  S7                                                                     │   │
│  │  • Requests threshold: 5                                                │   │
│  │  • Failure ratio: 0.5 (50%)                                             │   │
│  │  • Timeout: 60 seconds                                                  │   │
│  │  • Per-device breakers                                                  │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Justification:                                                                │
│  • Prevents cascade failures when device/network fails                         │
│  • Reduces unnecessary retry storms                                            │
│  • Allows system to recover gracefully                                         │
│  • Per-device isolation prevents one bad device from affecting others          │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 9.2 Retry with Exponential Backoff

Exponential backoff with jitter prevents "thundering herd" scenarios where multiple clients retry simultaneously after a failure. The diagram shows the delay calculation formula, example retry sequences, and per-protocol configuration. Note the longer backoff for OPC UA `TooManySessions` errors, allowing server session cleanup time:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                      EXPONENTIAL BACKOFF STRATEGY                              │
│                                                                                │
│  Formula: delay = min(baseDelay * 2^attempt * (1 ± jitter), maxDelay)          │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                         │   │
│  │  Example: baseDelay=100ms, maxDelay=10s, jitter=25%                     │   │
│  │                                                                         │   │
│  │  Attempt  │  Base Delay  │  With Jitter (±25%)  │  Actual Wait          │   │
│  │  ─────────┼──────────────┼──────────────────────┼────────────────────   │   │
│  │     1     │    100ms     │    75ms - 125ms      │    ~100ms             │   │
│  │     2     │    200ms     │   150ms - 250ms      │    ~200ms             │   │
│  │     3     │    400ms     │   300ms - 500ms      │    ~400ms             │   │
│  │     4     │    800ms     │   600ms - 1000ms     │    ~800ms             │   │
│  │     5     │   1600ms     │  1200ms - 2000ms     │    ~1.6s              │   │
│  │     6     │   3200ms     │  2400ms - 4000ms     │    ~3.2s              │   │
│  │     7     │   6400ms     │  4800ms - 8000ms     │    ~6.4s              │   │
│  │     8     │  10000ms     │  7500ms - 10000ms    │    10s (capped)       │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Jitter Purpose:                                                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                         │   │
│  │  WITHOUT JITTER (Thundering Herd):                                      │   │
│  │                                                                         │   │
│  │  Time ──►                                                               │   │
│  │  0ms    100ms   200ms   300ms   400ms                                   │   │
│  │   │       │       │       │       │                                     │   │
│  │   ▼       ▼       ▼       ▼       ▼                                     │   │
│  │  All     All     All     All     All    ◄── All retries hit server      │   │
│  │  retry   retry   retry   retry   retry       at exactly same time       │   │
│  │                                                                         │   │
│  │  WITH JITTER (Distributed Load):                                        │   │
│  │                                                                         │   │
│  │  Time ──►                                                               │   │
│  │  0ms                                                                    │   │
│  │   │  75ms  90ms  110ms  125ms                                           │   │
│  │   ▼   ▼     ▼      ▼      ▼                                             │   │
│  │  Retry Retry Retry Retry Retry  ◄── Retries spread out, reducing        │   │
│  │   A     B     C     D     E           peak load on server               │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Implementation per Protocol:                                                  │
│                                                                                │
│  ┌───────────────┬──────────────┬────────────┬─────────────────────────────┐   │
│  │   Protocol    │  Base Delay  │ Max Delay  │  Notes                      │   │
│  ├───────────────┼──────────────┼────────────┼─────────────────────────────┤   │
│  │ Modbus        │    100ms     │    10s     │  Fast retries, TCP stateless│   │
│  │ OPC UA        │    500ms     │    10s     │  Session recovery overhead  │   │
│  │ OPC UA (TMS)  │     60s      │    5min    │  TooManySessions: long wait │   │
│  │ S7            │    500ms     │    10s     │  PLC connection setup time  │   │
│  └───────────────┴──────────────┴────────────┴─────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 9.3 Graceful Degradation

The gateway maintains service availability through progressive degradation rather than complete failure. This hierarchy diagram shows five operational levels, from full operation through device-level, endpoint-level, publish-level, and protocol-level degradation. At each level, the system continues providing value (API access, configuration management) even when data collection is impaired:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                        GRACEFUL DEGRADATION HIERARCHY                          │
│                                                                                │
│  The gateway maintains service availability through progressive degradation:   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  LEVEL 0: FULL OPERATION                                                │   │
│  │                                                                         │   │
│  │  • All devices connected and polling                                    │   │
│  │  • All protocols operational                                            │   │
│  │  • MQTT publishing normally                                             │   │
│  │  • Health status: HEALTHY                                               │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                           │                                                    │
│                           ▼ Single device failure                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  LEVEL 1: DEVICE-LEVEL DEGRADATION                                      │   │
│  │                                                                         │   │
│  │  • Failed device's circuit breaker opens                                │   │
│  │  • Other devices continue normal operation                              │   │
│  │  • DataPoints for failed device marked quality=bad                      │   │
│  │  • Automatic recovery attempts every 60s                                │   │
│  │  • Health status: DEGRADED                                              │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                           │                                                    │
│                           ▼ OPC Server overloaded                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  LEVEL 2: ENDPOINT-LEVEL DEGRADATION (OPC UA)                           │   │
│  │                                                                         │   │
│  │  • Endpoint circuit breaker opens                                       │   │
│  │  • All devices on that endpoint paused                                  │   │
│  │  • Other endpoints continue normally                                    │   │
│  │  • Load shaping enters brownout mode                                    │   │
│  │  • Telemetry paused, control/safety allowed                             │   │
│  │  • Health status: DEGRADED                                              │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                           │                                                    │
│                           ▼ MQTT broker disconnection                          │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  LEVEL 3: PUBLISH-LEVEL DEGRADATION                                     │   │
│  │                                                                         │   │
│  │  • Polling continues (data is fresh)                                    │   │
│  │  • DataPoints buffered in memory (10,000 limit)                         │   │
│  │  • Oldest messages dropped on buffer overflow                           │   │
│  │  • Reconnection attempts with backoff                                   │   │
│  │  • Command writes queued (will timeout)                                 │   │
│  │  • Health status: DEGRADED                                              │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                           │                                                    │
│                           ▼ All protocols failed                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  LEVEL 4: PROTOCOL-LEVEL DEGRADATION                                    │   │
│  │                                                                         │   │
│  │  • All circuit breakers open                                            │   │
│  │  • Polling skipped (no point trying)                                    │   │
│  │  • Health check probes continue                                         │   │
│  │  • API/Web UI remain accessible                                         │   │
│  │  • Configuration changes still possible                                 │   │
│  │  • Health status: UNHEALTHY                                             │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Key Principle: The gateway never crashes due to device/network failures.      │
│  It maintains API availability for diagnostics and configuration.              │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 9.4 Gateway Initialization and Startup

The gateway implements a robust startup sequence with protocol validation, readiness gating, and comprehensive logging. This ensures observability tools receive accurate data only after the system is fully initialized:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                        GATEWAY STARTUP SEQUENCE                                │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  PHASE 1: INITIALIZATION                                                │   │
│  │                                                                         │   │
│  │  1. Load configuration (config.yaml, devices.yaml)                      │   │
│  │  2. Initialize logging with configured level/format                     │   │
│  │  3. Register Prometheus metrics                                         │   │
│  │  4. Initialize protocol pools (Modbus, OPC UA, S7)                      │   │
│  │  5. Initialize MQTT publisher                                           │   │
│  │                                                                         │   │
│  │  gatewayReady: false (metrics endpoint returns 503)                     │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                           │                                                    │
│                           ▼                                                    │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  PHASE 2: DEVICE REGISTRATION (with Protocol Validation)                │   │
│  │                                                                         │   │
│  │  For each device in configuration:                                      │   │
│  │    ├─ Validate device.Protocol against registered protocols             │   │
│  │    │   • modbus-tcp, modbus-rtu → ModbusPool                            │   │
│  │    │   • opcua                  → OPCUAPool                             │   │
│  │    │   • s7                     → S7Pool                                │   │
│  │    │   • <unknown>              → Log warning, skip device              │   │
│  │    │                                                                    │   │
│  │    ├─ If protocol supported:                                            │   │
│  │    │   • Call pool.RegisterDevice(device)                               │   │
│  │    │   • Track in registeredDevices count                               │   │
│  │    │                                                                    │   │
│  │    └─ If protocol not supported:                                        │   │
│  │        • Return ErrProtocolNotSupported                                 │   │
│  │        • Track in failedDevices or unsupportedProtocol count            │   │
│  │                                                                         │   │
│  │  Startup Logging:                                                       │   │
│  │  INFO: "Gateway startup complete: 10 registered, 0 failed, 1 skipped"   │   │
│  │  WARN: "Gateway starting in DEGRADED state" (if failures > 0)           │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                           │                                                    │
│                           ▼                                                    │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  PHASE 3: SERVICE STARTUP                                               │   │
│  │                                                                         │   │
│  │  1. Start HTTP server (API, health probes, metrics)                     │   │
│  │  2. Start polling service for registered devices                        │   │
│  │  3. Start command handler for write operations                          │   │
│  │  4. Set gatewayReady = true                                             │   │
│  │                                                                         │   │
│  │  Metrics endpoint now returns 200 OK with valid data                    │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  READINESS GUARD FOR METRICS:                                                  │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │  GET /metrics                                                            │  │
│  │                                                                          │  │
│  │  if !gatewayReady.Load() {                                               │  │
│  │      return 503 Service Unavailable                                      │  │
│  │      body: "Gateway not ready - initialization in progress"              │  │
│  │  }                                                                       │  │
│  │                                                                          │  │
│  │  // Normal Prometheus handler                                            │  │
│  │  promhttp.Handler().ServeHTTP(w, r)                                      │  │
│  │                                                                          │  │
│  │  PURPOSE: Prevents Prometheus from scraping incomplete/invalid metrics   │  │
│  │  during startup. Avoids false alerts from metrics systems.               │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---

## 10. Observability Infrastructure

### 10.1 Metrics Architecture

Prometheus metrics enable operational dashboards (Grafana) and alerting. The diagram catalogs all exposed metrics grouped by subsystem (connections, polling, MQTT, devices, system), including metric types (Gauge, Counter, Histogram), label dimensions, and purpose. These metrics support capacity planning, troubleshooting, and SLA monitoring:

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
│  │  gateway_polling_polls_skipped_total{device_id, reason}                 │   │
│  │    → Counter: Skipped polls (back-pressure, circuit breaker)            │   │
│  │    → Purpose: Identify overload conditions                              │   │
│  │                                                                         │   │
│  │  gateway_polling_duration_seconds{device_id}                            │   │
│  │    → Histogram: Poll cycle duration                                     │   │
│  │    → Purpose: Identify slow devices, optimize timeouts                  │   │
│  │                                                                         │   │
│  │  gateway_polling_points_read_total{device_id}                           │   │
│  │    → Counter: Total data points successfully read                       │   │
│  │    → Purpose: Throughput measurement                                    │   │
│  │                                                                         │   │
│  │  gateway_polling_points_published_total{device_id}                      │   │
│  │    → Counter: Total data points published to MQTT                       │   │
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

### 10.2 Structured Logging

Zero-allocation structured logging via zerolog enables high-performance log output without impacting data processing. The diagram shows RFC 5424 log levels, JSON vs. console output formats, and contextual logging helpers that automatically add device/request context. JSON format enables log aggregation and querying in systems like Elasticsearch or Loki:

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

The health check system provides Kubernetes-compatible probes with flapping protection to prevent false alarms during transient issues. The diagram shows HTTP endpoints, severity levels affecting overall status, operational state machine transitions, and the flapping protection algorithm that requires consecutive failures before marking unhealthy:

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
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---

## 11. Security Architecture

### 11.1 Transport Security

Secure communication is essential for industrial environments handling sensitive operational data. The diagram documents TLS configuration for MQTT (client certificates, cipher suites) and OPC UA security profiles (Basic256Sha256 recommended). Security mode options range from development (None) to production (SignAndEncrypt):

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                        TRANSPORT LAYER SECURITY                                │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                        MQTT TLS CONFIGURATION                           │   │
│  │                                                                         │   │
│  │  Client Certificate Authentication:                                     │   │
│  │  ┌─────────────────────────────────────────────────────────────────┐    │   │
│  │  │  mqtt:                                                          │    │   │
│  │  │    broker_url: "ssl://broker.example.com:8883"                  │    │   │
│  │  │    tls_enabled: true                                            │    │   │
│  │  │    tls_cert_file: "/certs/client.crt"                           │    │   │
│  │  │    tls_key_file: "/certs/client.key"                            │    │   │
│  │  │    tls_ca_file: "/certs/ca.crt"                                 │    │   │
│  │  │    tls_insecure_skip_verify: false  # NEVER true in production  │    │   │
│  │  └─────────────────────────────────────────────────────────────────┘    │   │
│  │                                                                         │   │
│  │  Supported Cipher Suites: TLS 1.2+ (Go crypto/tls defaults)             │   │
│  │  • TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256                                │   │
│  │  • TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384                                │   │
│  │  • TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256                              │   │
│  │  • TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384                              │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                     OPC UA SECURITY POLICIES                            │   │
│  │                                                                         │   │
│  │  Policy              │ Encryption    │ Signature     │ Use Case         │   │
│  │  ────────────────────┼───────────────┼───────────────┼────────────────  │   │
│  │  None                │ None          │ None          │ Development      │   │
│  │  Basic128Rsa15       │ RSA-OAEP      │ RSA-SHA1      │ Legacy (avoid)   │   │
│  │  Basic256            │ RSA-OAEP      │ RSA-SHA1      │ Legacy (avoid)   │   │
│  │  Basic256Sha256      │ RSA-OAEP-256  │ RSA-SHA256    │ RECOMMENDED      │   │
│  │                                                                         │   │
│  │  Security Modes:                                                        │   │
│  │  • None: No security (development only)                                 │   │
│  │  • Sign: Message integrity, no encryption                               │   │
│  │  • SignAndEncrypt: Full security (production)                           │   │
│  │                                                                         │   │
│  │  Authentication Modes:                                                  │   │
│  │  • Anonymous: No authentication                                         │   │
│  │  • UserName: Username/password                                          │   │
│  │  • Certificate: X.509 client certificate                                │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 11.2 Credential Management

Secure credential handling prevents exposure of authentication secrets. The diagram shows supported credential sources (environment variables, config files, Docker secrets) and security best practices. Credentials are never logged, and production deployments should use secret management systems:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                         CREDENTIAL MANAGEMENT                                  │
│                                                                                │
│  Current Implementation: Environment Variables / Config Files                  │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    CREDENTIAL SOURCES                                   │   │
│  │                                                                         │   │
│  │  1. Environment Variables (Recommended for containers):                 │   │
│  │     MQTT_USERNAME, MQTT_PASSWORD                                        │   │
│  │     OPC_USERNAME, OPC_PASSWORD                                          │   │
│  │                                                                         │   │
│  │  2. Config Files (For development):                                     │   │
│  │     config.yaml with opc_username, opc_password                         │   │
│  │     devices.yaml with per-device credentials                            │   │
│  │                                                                         │   │
│  │  3. Docker Secrets (Production):                                        │   │
│  │     Mount secrets as files: /run/secrets/mqtt_password                  │   │
│  │                                                                         │   │
│  │  4. External Secret Managers (Future enhancement):                      │   │
│  │     HashiCorp Vault, AWS Secrets Manager, Azure Key Vault               │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    SECURITY BEST PRACTICES                              │   │
│  │                                                                         │   │
│  │  + Credentials never logged (zerolog field exclusion)                   │   │
│  │  + Config files with restricted permissions (0600)                      │   │
│  │  + Environment variables for container deployments                      │   │
│  │  + TLS for all production connections                                   │   │
│  │  + Certificate-based auth where supported                               │   │
│  │                                                                         │   │
│  │  - Never commit credentials to source control                           │   │
│  │  - Never use insecure_skip_verify in production                         │   │
│  │  - Never share credentials across environments                          │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 11.3 Network Security

Network segmentation is critical for ICS security (IEC 62443). The diagram shows the recommended three-zone deployment (IT, DMZ, OT) with the gateway positioned in the DMZ. Firewall rules ensure the gateway initiates all OT connections (never the reverse), and input validation prevents injection attacks through the API:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                           NETWORK SECURITY MODEL                               │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    NETWORK SEGMENTATION                                 │   │
│  │                                                                         │   │
│  │  Recommended Deployment:                                                │   │
│  │                                                                         │   │
│  │  ┌──────────────────┐     ┌──────────────────┐     ┌──────────────────┐ │   │
│  │  │   IT Network     │     │   DMZ Network    │     │   OT Network     │ │   │
│  │  │                  │     │                  │     │                  │ │   │
│  │  │  MQTT Broker     │◄────│  Protocol        │────►│  PLCs, Sensors   │ │   │
│  │  │  Dashboard       │     │  Gateway         │     │  OPC UA Servers  │ │   │
│  │  │  Cloud Services  │     │                  │     │  Modbus Devices  │ │   │
│  │  │                  │     │  Port 8080 only  │     │                  │ │   │
│  │  └──────────────────┘     └──────────────────┘     └──────────────────┘ │   │
│  │                                                                         │   │
│  │  Firewall Rules:                                                        │   │
│  │  • Gateway → OT: Allow Modbus/502, OPC UA/4840, S7/102                  │   │
│  │  • Gateway → IT: Allow MQTT/1883,8883                                   │   │
│  │  • IT → Gateway: Allow HTTP/8080 (management only)                      │   │
│  │  • OT → Gateway: DENY (gateway initiates all OT connections)            │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    INPUT VALIDATION                                     │   │
│  │                                                                         │   │
│  │  Device Configuration:                                                  │   │
│  │  • ID: Alphanumeric with hyphens, required                              │   │
│  │  • Host: Valid hostname/IP, required                                    │   │
│  │  • Port: 1-65535 range                                                  │   │
│  │  • Poll interval: ≥100ms minimum                                        │   │
│  │  • Slave ID (Modbus): 1-247 range                                       │   │
│  │                                                                         │   │
│  │  Tag Configuration:                                                     │   │
│  │  • Address: Non-negative integer                                        │   │
│  │  • Register count: Validated against data type                          │   │
│  │  • Topic suffix: Sanitized for MQTT (no wildcards)                      │   │
│  │                                                                         │   │
│  │  API Requests:                                                          │   │
│  │  • JSON parsing with size limits                                        │   │
│  │  • Content-Type validation                                              │   │
│  │  • CORS headers for browser security                                    │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---

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

## 13. Web UI Architecture

### 13.1 Frontend Technology Stack

The embedded Web UI provides runtime device management without requiring a separate frontend build process. Built with React 18 (via CDN) and pure CSS, it runs directly from static files served by the gateway. The component architecture diagram shows the App root, navigation, and device management modal with protocol-specific field rendering:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                          WEB UI ARCHITECTURE                                   │
│                                                                                │
│  Technology Stack:                                                             │
│  • React 18 (via CDN, no build step required)                                  │
│  • Babel standalone (JSX transformation in browser)                            │
│  • Pure CSS (CSS Custom Properties for theming)                                │
│  • IBM Plex Sans + JetBrains Mono fonts                                        │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    COMPONENT ARCHITECTURE                               │   │
│  │                                                                         │   │
│  │  ┌─────────────────────────────────────────────────────────────────┐    │   │
│  │  │  App (Root Component)                                           │    │   │
│  │  │  ├── State: devices, activeSection, apiStatus                   │    │   │
│  │  │  ├── Effects: loadDevices(), loadTopics(), loadLogs()           │    │   │
│  │  │  │                                                              │    │   │
│  │  │  ├── SideNav                                                    │    │   │
│  │  │  │   └── Navigation items with icons                            │    │   │
│  │  │  │                                                              │    │   │
│  │  │  ├── TopBar                                                     │    │   │
│  │  │  │   ├── Page title and metadata                                │    │   │
│  │  │  │   └── API status indicator                                   │    │   │
│  │  │  │                                                              │    │   │
│  │  │  └── Content (conditional rendering by section)                 │    │   │
│  │  │      ├── Overview: Stats cards                                  │    │   │
│  │  │      ├── Devices: DeviceTable + DeviceForm modal                │    │   │
│  │  │      ├── Topics/Routes: Topic tables                            │    │   │
│  │  │      └── Logs: Container log viewer                             │    │   │
│  │  └─────────────────────────────────────────────────────────────────┘    │   │
│  │                                                                         │   │
│  │  ┌─────────────────────────────────────────────────────────────────┐    │   │
│  │  │  DeviceForm (Modal Component)                                   │    │   │
│  │  │  ├── Tabs: Basic Info, Connection, Tags                         │    │   │
│  │  │  ├── Protocol-specific fields (Modbus/OPC UA/S7)                │    │   │
│  │  │  ├── Dynamic tag list with add/remove                           │    │   │
│  │  │  └── Connection test functionality                              │    │   │
│  │  └─────────────────────────────────────────────────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                      API SERVICE LAYER                                  │   │
│  │                                                                         │   │
│  │  const api = {                                                          │   │
│  │    getDevices()        → GET  /api/devices                              │   │
│  │    createDevice(d)     → POST /api/devices                              │   │
│  │    updateDevice(d)     → PUT  /api/devices                              │   │
│  │    deleteDevice(id)    → DELETE /api/devices?id={id}                    │   │
│  │    testConnection(d)   → POST /api/test-connection                      │   │
│  │    getTopicsOverview() → GET  /api/topics                               │   │
│  │    listLogContainers() → GET  /api/logs/containers                      │   │
│  │    getLogs(c, tail)    → GET  /api/logs?container={c}&tail={n}          │   │
│  │  }                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Design System:                                                                │
│  • Dark theme (industrial aesthetic)                                           │
│  • CSS Custom Properties for consistent theming                                │
│  • Responsive layout (collapses sidebar on mobile)                             │
│  • Accessible: ARIA labels, focus indicators, keyboard navigation              │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 13.2 API Endpoints

The REST API enables programmatic device management and operational monitoring. The diagram documents all endpoints grouped by function (device CRUD, runtime information, observability), including request/response formats and side effects. CORS is enabled for browser-based access, and all responses use JSON content type:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                            REST API DESIGN                                     │
│                                                                                │
│  Base URL: http://localhost:8080                                               │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    DEVICE MANAGEMENT                                    │   │
│  │                                                                         │   │
│  │  GET /api/devices                                                       │   │
│  │    Response: Device[] (all devices)                                     │   │
│  │                                                                         │   │
│  │  GET /api/devices?id={device_id}                                        │   │
│  │    Response: Device (single device)                                     │   │
│  │                                                                         │   │
│  │  POST /api/devices                                                      │   │
│  │    Body: Device (new device)                                            │   │
│  │    Response: { success: true, device: Device }                          │   │
│  │    Side effects: Persists to YAML, registers with polling service       │   │
│  │                                                                         │   │
│  │  PUT /api/devices                                                       │   │
│  │    Body: Device (updated device)                                        │   │
│  │    Response: { success: true, device: Device }                          │   │
│  │    Side effects: Persists to YAML, re-registers with polling service    │   │
│  │                                                                         │   │
│  │  DELETE /api/devices?id={device_id}                                     │   │
│  │    Response: { success: true }                                          │   │
│  │    Side effects: Removes from YAML, unregisters from polling service    │   │
│  │                                                                         │   │
│  │  POST /api/test-connection                                              │   │
│  │    Body: Device (device to test)                                        │   │
│  │    Response: { success: true } or error                                 │   │
│  │    Note: Validates configuration only, no actual connection             │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    RUNTIME INFORMATION                                  │   │
│  │                                                                         │   │
│  │  GET /api/topics?limit={n}                                              │   │
│  │    Response: {                                                          │   │
│  │      active_topics: TopicStats[],   // Recently published topics        │   │
│  │      subscriptions: string[],        // MQTT subscription patterns      │   │
│  │      routes: RouteConfig[]           // Device→Tag→Topic mappings       │   │
│  │    }                                                                    │   │
│  │                                                                         │   │
│  │  GET /api/logs/containers                                               │   │
│  │    Response: { containers: string[] }                                   │   │
│  │    Note: Requires Docker socket access                                  │   │
│  │                                                                         │   │
│  │  GET /api/logs?container={name}&tail={n}                                │   │
│  │    Response: { entries: LogEntry[] }                                    │   │
│  │    Note: Parses JSON logs with timestamp, level, message                │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    OBSERVABILITY                                        │   │
│  │                                                                         │   │
│  │  GET /health                                                            │   │
│  │    Response: { status, checks: CheckResult[], timestamp }               │   │
│  │                                                                         │   │
│  │  GET /health/live                                                       │   │
│  │    Response: 200 OK (process running) or 503 Service Unavailable        │   │
│  │                                                                         │   │
│  │  GET /health/ready                                                      │   │
│  │    Response: 200 OK (ready) or 503 Service Unavailable                  │   │
│  │                                                                         │   │
│  │  GET /metrics                                                           │   │
│  │    Response: Prometheus text format                                     │   │
│  │                                                                         │   │
│  │  GET /status                                                            │   │
│  │    Response: { service, version, polling_stats }                        │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  CORS: Enabled for all origins (development convenience)                       │
│  Content-Type: application/json                                                │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---

## 14. Testing Strategy

### 14.1 Test Architecture

The testing strategy follows the test pyramid with unit tests at the base, integration tests with protocol simulators in the middle, and end-to-end Docker Compose tests at the top. The diagram shows test commands, mock patterns for the ProtocolPool interface, and benchmark test structure for performance validation:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                           TESTING ARCHITECTURE                                 │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                      TEST PYRAMID                                       │   │
│  │                                                                         │   │
│  │                      ┌───────────┐                                      │   │
│  │                      │   E2E     │  ← Docker Compose integration        │   │
│  │                      │   Tests   │    (manual/CI)                       │   │
│  │                    ┌─┴───────────┴─┐                                    │   │
│  │                    │  Integration  │  ← Protocol simulators             │   │
│  │                    │    Tests      │    (go test -tags=integration)     │   │
│  │                  ┌─┴───────────────┴─┐                                  │   │
│  │                  │    Unit Tests     │  ← Domain logic, mocks           │   │
│  │                  │                   │    (go test ./...)               │   │
│  │                  └───────────────────┘                                  │   │
│  │                                                                         │   │
│  │  Test Commands:                                                         │   │
│  │  • make test          → Run unit tests with race detector               │   │
│  │  • make test-cover    → Generate coverage report                        │   │
│  │  • make test-integration → Run with simulators                          │   │
│  │  • make bench         → Run benchmarks                                  │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    UNIT TEST PATTERNS                                   │   │
│  │                                                                         │   │
│  │  Domain Layer Tests (internal/domain/*_test.go):                        │   │
│  │  • Device validation (required fields, intervals, protocols)            │   │
│  │  • Tag validation (protocol-specific fields, register counts)           │   │
│  │  • DataPoint operations (timestamps, quality, pooling)                  │   │
│  │  • Error handling (sentinel errors, error wrapping)                     │   │
│  │                                                                         │   │
│  │  Mock Patterns:                                                         │   │
│  │  • MockProtocolPool implements ProtocolPool interface                   │   │
│  │  • Records all calls for verification                                   │   │
│  │  • Configurable return values and errors                                │   │
│  │  • Thread-safe for concurrent test execution                            │   │
│  │                                                                         │   │
│  │  Example Test Structure:                                                │   │
│  │  func TestDevice_Validate(t *testing.T) {                               │   │
│  │      tests := []struct {                                                │   │
│  │          name    string                                                 │   │
│  │          device  Device                                                 │   │
│  │          wantErr error                                                  │   │
│  │      }{                                                                 │   │
│  │          {"valid device", validDevice(), nil},                          │   │
│  │          {"missing ID", deviceWithoutID(), ErrDeviceIDRequired},        │   │
│  │          ...                                                            │   │
│  │      }                                                                  │   │
│  │      for _, tt := range tests {                                         │   │
│  │          t.Run(tt.name, func(t *testing.T) {                            │   │
│  │              err := tt.device.Validate()                                │   │
│  │              if !errors.Is(err, tt.wantErr) { ... }                     │   │
│  │          })                                                             │   │
│  │      }                                                                  │   │
│  │  }                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    BENCHMARK TESTS                                      │   │
│  │                                                                         │   │
│  │  DataPoint Benchmarks (internal/domain/datapoint_bench_test.go):        │   │
│  │                                                                         │   │
│  │  BenchmarkDataPoint_New          → Direct allocation                    │   │
│  │  BenchmarkDataPoint_Pool         → sync.Pool allocation                 │   │
│  │  BenchmarkDataPoint_ToJSON       → JSON serialization                   │   │
│  │  BenchmarkDataPoint_ToMQTTPayload → Compact format conversion           │   │
│  │                                                                         │   │
│  │  Expected Results:                                                      │   │
│  │  • Pool allocation: ~50% fewer allocations than New                     │   │
│  │  • MQTTPayload: ~3x faster than full JSON                               │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 14.2 Simulator Infrastructure

Protocol simulators enable testing without physical industrial devices. The diagram documents the OPC UA simulator (Python asyncua with demo nodes), Modbus simulator (oitc/modbus-server), and EMQX broker configuration. These simulators provide realistic test scenarios including value simulation, write testing, and error injection:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                         PROTOCOL SIMULATORS                                    │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    OPC UA SIMULATOR                                     │   │
│  │                                                                         │   │
│  │  Location: tools/opcua-simulator/                                       │   │
│  │  Technology: Python + asyncua library                                   │   │
│  │  Port: 4840                                                             │   │
│  │                                                                         │   │
│  │  Exposed Nodes (ns=2):                                                  │   │
│  │  • Demo.Temperature (Double) → 20 ± 5°C sinusoidal                      │   │
│  │  • Demo.Pressure (Double) → 1.2 ± 0.2 bar sinusoidal                    │   │
│  │  • Demo.Status (String) → "OK" / "WARN" alternating                     │   │
│  │  • Demo.Switch (Boolean) → Writable                                     │   │
│  │  • Demo.WriteTest (Boolean) → Write compatibility testing               │   │
│  │                                                                         │   │
│  │  Configuration (Environment Variables):                                 │   │
│  │  • OPCUA_HOST: Bind address (default: 0.0.0.0)                          │   │
│  │  • OPCUA_PORT: Listen port (default: 4840)                              │   │
│  │  • OPCUA_UPDATE_MS: Value update interval (default: 500)                │   │
│  │  • OPCUA_AUTO_UPDATE: Enable value simulation (default: 1)              │   │
│  │                                                                         │   │
│  │  Node ID Format: ns=2;s=Demo.Temperature                                │   │
│  │  Explicit VariantTypes prevent type mismatch errors                     │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    MODBUS SIMULATOR                                     │   │
│  │                                                                         │   │
│  │  Image: oitc/modbus-server:latest                                       │   │
│  │  Port: 5020                                                             │   │
│  │                                                                         │   │
│  │  Features:                                                              │   │
│  │  • Holding registers: 0-999                                             │   │
│  │  • Input registers: 0-999                                               │   │
│  │  • Coils: 0-999                                                         │   │
│  │  • Discrete inputs: 0-999                                               │   │
│  │  • Slave ID: 1 (configurable)                                           │   │
│  │                                                                         │   │
│  │  Use for: Testing Modbus TCP connectivity, register reading/writing     │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    MQTT BROKER (EMQX)                                   │   │
│  │                                                                         │   │
│  │  Image: emqx/emqx:5.5                                                   │   │
│  │  Ports:                                                                 │   │
│  │  • 1883: MQTT TCP                                                       │   │
│  │  • 8083: MQTT WebSocket                                                 │   │
│  │  • 8084: MQTT WebSocket Secure                                          │   │
│  │  • 8883: MQTT TLS                                                       │   │
│  │  • 18083: Dashboard                                                     │   │
│  │                                                                         │   │
│  │  Dashboard: http://localhost:18083 (admin/public)                       │   │
│  │                                                                         │   │
│  │  Features for Testing:                                                  │   │
│  │  • Message tracing                                                      │   │
│  │  • Client inspection                                                    │   │
│  │  • Topic statistics                                                     │   │
│  │  • Rule engine for message transformation                               │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---

## 15. Standards Compliance

### 15.1 Industrial Protocol Standards

The gateway implements industry-standard protocols ensuring interoperability with devices from multiple vendors. This comprehensive diagram documents compliance with Modbus (IEC 61158), OPC UA (IEC 62541), Siemens S7 (ISO 8073), and MQTT (OASIS Standard), including supported function codes, security profiles, addressing formats, and data types:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                      INDUSTRIAL PROTOCOL STANDARDS                             │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    MODBUS (IEC 61158)                                   │   │
│  │                                                                         │   │
│  │  Standard: MODBUS Application Protocol Specification V1.1b3             │   │
│  │  Organization: Modbus Organization (modbus.org)                         │   │
│  │                                                                         │   │
│  │  Compliance:                                                            │   │
│  │  + Function codes: 01, 02, 03, 04, 05, 06, 15, 16                       │   │
│  │  + Exception responses: 01-06                                           │   │
│  │  + Register addressing: 0-65535                                         │   │
│  │  + Coil/discrete addressing: 0-65535                                    │   │
│  │  + TCP framing (MBAP header)                                            │   │
│  │  + RTU framing (serial)                                                 │   │
│  │  + Slave ID: 1-247                                                      │   │
│  │                                                                         │   │
│  │  Data Types (Standard mappings):                                        │   │
│  │  • 16-bit register → INT16, UINT16                                      │   │
│  │  • 32-bit (2 registers) → INT32, UINT32, FLOAT32                        │   │
│  │  • 64-bit (4 registers) → INT64, UINT64, FLOAT64                        │   │
│  │                                                                         │   │
│  │  Byte Order Support:                                                    │   │
│  │  • Big Endian (AB CD) - Modbus standard                                 │   │
│  │  • Little Endian (DC BA)                                                │   │
│  │  • Mid-Big Endian (BA DC) - Some PLCs                                   │   │
│  │  • Mid-Little Endian (CD AB)                                            │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    OPC UA (IEC 62541)                                   │   │
│  │                                                                         │   │
│  │  Standard: OPC Unified Architecture                                     │   │
│  │  Organization: OPC Foundation (opcfoundation.org)                       │   │
│  │                                                                         │   │
│  │  Compliance:                                                            │   │
│  │  + Part 3: Address Space Model                                          │   │
│  │  + Part 4: Services (Read, Write, Browse, Subscribe)                    │   │
│  │  + Part 5: Information Model                                            │   │
│  │  + Part 6: Service Mappings (UA Binary over TCP)                        │   │
│  │  + Part 7: Security Profiles                                            │   │
│  │                                                                         │   │
│  │  Security Profiles:                                                     │   │
│  │  + None (development)                                                   │   │
│  │  + Basic128Rsa15 (legacy)                                               │   │
│  │  + Basic256 (legacy)                                                    │   │
│  │  + Basic256Sha256 (recommended)                                         │   │
│  │                                                                         │   │
│  │  Node ID Formats:                                                       │   │
│  │  + Numeric: ns=2;i=1234                                                 │   │
│  │  + String: ns=2;s=MyNode                                                │   │
│  │  + GUID: ns=2;g=...                                                     │   │
│  │  + ByteString: ns=2;b=...                                               │   │
│  │                                                                         │   │
│  │  Subscription Support:                                                  │   │
│  │  + Monitored items with sampling interval                               │   │
│  │  + Deadband filtering (absolute, percent)                               │   │
│  │  + Queue size and discard policy                                        │   │
│  │  + Republish for missed notifications                                   │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    SIEMENS S7 (ISO-on-TCP)                              │   │
│  │                                                                         │   │
│  │  Standard: ISO 8073 (Connection-oriented transport)                     │   │
│  │  Port: 102 (ISO-TSAP)                                                   │   │
│  │                                                                         │   │
│  │  Compliance:                                                            │   │
│  │  + S7-300/400/1200/1500 communication                                   │   │
│  │  + COTP (ISO 8073) connection establishment                             │   │
│  │  + S7 communication layer                                               │   │
│  │                                                                         │   │
│  │  Memory Areas:                                                          │   │
│  │  + DB (Data Blocks) - DB1.DBW0                                          │   │
│  │  + M (Merker/Flags) - MW100                                             │   │
│  │  + I (Inputs) - IW0                                                     │   │
│  │  + Q (Outputs) - QW0                                                    │   │
│  │  + T (Timers)                                                           │   │
│  │  + C (Counters)                                                         │   │
│  │                                                                         │   │
│  │  Address Formats:                                                       │   │
│  │  • Symbolic: DB1.DBD0, MW100, I0.0, Q0.0                                │   │
│  │  • Bit addressing: DB1.DBX0.0 (byte 0, bit 0)                           │   │
│  │                                                                         │   │
│  │  PLC Configuration:                                                     │   │
│  │  • Rack/Slot: S7-300/400 (0/2), S7-1200/1500 (0/0 or 0/1)               │   │
│  │  • PDU Size: Up to 960 bytes (default 480)                              │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    MQTT (OASIS Standard)                                │   │
│  │                                                                         │   │
│  │  Standard: MQTT Version 3.1.1 (OASIS Standard)                          │   │
│  │  Organization: OASIS (oasis-open.org)                                   │   │
│  │                                                                         │   │
│  │  Compliance:                                                            │   │
│  │  + QoS 0 (At most once)                                                 │   │
│  │  + QoS 1 (At least once)                                                │   │
│  │  + QoS 2 (Exactly once)                                                 │   │
│  │  + Clean session                                                        │   │
│  │  + Keep-alive                                                           │   │
│  │  + Will messages                                                        │   │
│  │  + Retained messages                                                    │   │
│  │  + Topic wildcards (+ and #)                                            │   │
│  │                                                                         │   │
│  │  Topic Structure (UNS-aligned):                                         │   │
│  │  {enterprise}/{site}/{area}/{line}/{device}/{datapoint}                 │   │
│  │                                                                         │   │
│  │  Example: acme/plant1/assembly/line3/plc-001/temperature                │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 15.2 Unified Namespace (UNS) Architecture

The Unified Namespace (UNS) is an event-driven architecture pattern that organizes industrial data hierarchically following ISA-95 levels. The diagram shows how device configuration maps to the UNS topic structure, topic sanitization rules, and the bidirectional command topic pattern for write operations. This standardization enables enterprise-wide data discovery and integration:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                    UNIFIED NAMESPACE ARCHITECTURE                              │
│                                                                                │
│  The Unified Namespace (UNS) is an event-driven architecture pattern for       │
│  industrial data, popularized by Industry 4.0 initiatives.                     │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    ISA-95 HIERARCHY MAPPING                             │   │
│  │                                                                         │   │
│  │  ISA-95 Level    │ UNS Topic Segment  │ Example                         │   │
│  │  ─────────────────┼────────────────────┼─────────────────────────────── │   │
│  │  Enterprise       │ {enterprise}       │ acme                           │   │
│  │  Site             │ {site}             │ plant-chicago                  │   │
│  │  Area             │ {area}             │ packaging                      │   │
│  │  Line/Cell        │ {line}             │ line-3                         │   │
│  │  Equipment        │ {equipment}        │ conveyor-01                    │   │
│  │  Data Point       │ {datapoint}        │ speed                          │   │
│  │                                                                         │   │
│  │  Full Topic: acme/plant-chicago/packaging/line-3/conveyor-01/speed      │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    GATEWAY IMPLEMENTATION                               │   │
│  │                                                                         │   │
│  │  Configuration:                                                         │   │
│  │  device:                                                                │   │
│  │    uns_prefix: "acme/plant-chicago/packaging/line-3/conveyor-01"        │   │
│  │    tags:                                                                │   │
│  │      - topic_suffix: "speed"           → Full: .../conveyor-01/speed    │   │
│  │      - topic_suffix: "temperature"     → Full: .../conveyor-01/temp     │   │
│  │      - topic_suffix: "status/running"  → Full: .../status/running       │   │
│  │                                                                         │   │
│  │  Topic Construction:                                                    │   │
│  │  fullTopic = device.UNSPrefix + "/" + tag.TopicSuffix                   │   │
│  │                                                                         │   │
│  │  Sanitization:                                                          │   │
│  │  • Replace spaces with hyphens                                          │   │
│  │  • Remove MQTT wildcards (+ #)                                          │   │
│  │  • Lowercase normalization                                              │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    COMMAND TOPICS (Bidirectional)                       │   │
│  │                                                                         │   │
│  │  The gateway subscribes to command topics for write operations:         │   │
│  │                                                                         │   │
│  │  Subscribe Pattern:                                                     │   │
│  │  $nexus/cmd/+/write        → JSON write commands                        │   │
│  │  $nexus/cmd/+/+/set        → Direct tag writes                          │   │
│  │                                                                         │   │
│  │  Response Topic:                                                        │   │
│  │  $nexus/cmd/response/{device_id}/{tag_id}                               │   │
│  │                                                                         │   │
│  │  Command Format:                                                        │   │
│  │  {                                                                      │   │
│  │    "request_id": "uuid",                                                │   │
│  │    "tag_id": "temperature",                                             │   │
│  │    "value": 25.5                                                        │   │
│  │  }                                                                      │   │
│  │                                                                         │   │
│  │  Response Format:                                                       │   │
│  │  {                                                                      │   │
│  │    "request_id": "uuid",                                                │   │
│  │    "success": true,                                                     │   │
│  │    "duration_ms": 45                                                    │   │
│  │  }                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 15.3 Sparkplug B Compatibility

Sparkplug B extends MQTT with a standardized payload format and state management for industrial IoT. The diagram shows the JSON-compatible payload structure with metrics array, timestamps, and sequence numbers. While full Sparkplug B requires Protocol Buffers encoding, this implementation provides a foundation for future enhancement:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                      SPARKPLUG B SUPPORT                                       │
│                                                                                │
│  Sparkplug B is an MQTT-based specification for industrial IoT, defining       │
│  topic structure, payload format, and state management.                        │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    PAYLOAD STRUCTURE                                    │   │
│  │                                                                         │   │
│  │  Domain Entity: SparkplugBPayload (internal/domain/datapoint.go)        │   │
│  │                                                                         │   │
│  │  type SparkplugBPayload struct {                                        │   │
│  │      Timestamp uint64              `json:"timestamp"`  // Unix ms       │   │
│  │      Metrics   []SparkplugBMetric  `json:"metrics"`                     │   │
│  │      Seq       uint64              `json:"seq"`        // Sequence      │   │
│  │  }                                                                      │   │
│  │                                                                         │   │
│  │  type SparkplugBMetric struct {                                         │   │
│  │      Name      string      `json:"name"`                                │   │
│  │      Timestamp uint64      `json:"timestamp"`                           │   │
│  │      DataType  string      `json:"datatype"`                            │   │
│  │      Value     interface{} `json:"value"`                               │   │
│  │  }                                                                      │   │
│  │                                                                         │   │
│  │  Note: Full Sparkplug B requires Protocol Buffers encoding.             │   │
│  │  This implementation provides JSON-compatible structure.                │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Future Enhancements:                                                          │
│  • Full Sparkplug B Protocol Buffer encoding                                   │
│  • Birth/Death certificates                                                    │
│  • Node/Device state management                                                │
│  • Sparkplug B topic namespace (spBv1.0/...)                                   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---

## 16. Appendices

### Appendix A: Configuration Reference

```yaml
# Complete configuration reference (config.yaml)

# Environment: development | staging | production
environment: development

# Path to device configuration file
devices_config_path: ./config/devices.yaml

# HTTP server configuration
http:
  port: 8080                    # Server port
  read_timeout: 10s             # Request read timeout
  write_timeout: 10s            # Response write timeout
  idle_timeout: 60s             # Keep-alive idle timeout

# MQTT publisher configuration
mqtt:
  broker_url: tcp://localhost:1883    # Broker URL (tcp:// or ssl://)
  client_id: protocol-gateway         # Client identifier
  username: ""                        # Optional username
  password: ""                        # Optional password
  clean_session: true                 # Start with clean session
  qos: 1                              # Default QoS (0, 1, 2)
  keep_alive: 30s                     # Keep-alive interval
  connect_timeout: 10s                # Connection timeout
  reconnect_delay: 5s                 # Reconnection delay
  max_reconnect: -1                   # Max reconnect attempts (-1 = unlimited)
  buffer_size: 10000                  # Message buffer size
  
  # TLS configuration
  tls_enabled: false
  tls_cert_file: ""                   # Client certificate
  tls_key_file: ""                    # Client private key
  tls_ca_file: ""                     # CA certificate
  tls_insecure_skip_verify: false     # Skip certificate verification

# Modbus protocol configuration
modbus:
  max_connections: 100                # Maximum concurrent connections
  idle_timeout: 5m                    # Idle connection timeout
  health_check_period: 30s            # Health check interval
  connection_timeout: 10s             # Connection timeout
  retry_attempts: 3                   # Max retry attempts
  retry_delay: 100ms                  # Initial retry delay

# OPC UA protocol configuration
opcua:
  max_connections: 50                 # Maximum endpoint sessions
  idle_timeout: 5m                    # Idle session timeout
  health_check_period: 30s            # Health check interval
  connection_timeout: 15s             # Connection timeout
  retry_attempts: 3                   # Max retry attempts
  retry_delay: 500ms                  # Initial retry delay
  default_security_policy: None       # None|Basic128Rsa15|Basic256|Basic256Sha256
  default_security_mode: None         # None|Sign|SignAndEncrypt
  default_auth_mode: Anonymous        # Anonymous|UserName|Certificate
  max_global_inflight: 1000           # Global concurrent operations limit
  brownout_threshold: 0.8             # Brownout mode trigger (0.0-1.0)
  max_inflight_per_endpoint: 100      # Per-endpoint operation limit

# S7 protocol configuration
s7:
  max_connections: 100                # Maximum concurrent connections
  idle_timeout: 5m                    # Idle connection timeout
  health_check_period: 30s            # Health check interval
  connection_timeout: 10s             # Connection timeout
  retry_attempts: 3                   # Max retry attempts
  retry_delay: 500ms                  # Initial retry delay

# Polling service configuration
polling:
  worker_count: 10                    # Concurrent polling workers
  batch_size: 50                      # Max tags per batch read
  default_interval: 1s                # Default poll interval

# Logging configuration
logging:
  level: info                         # trace|debug|info|warn|error
  format: json                        # json|console
  output: stdout                      # stdout|stderr|<filepath>
```

### Appendix B: Error Code Reference

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                         ERROR CODE REFERENCE                                   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  CONFIGURATION ERRORS                                                   │   │
│  │                                                                         │   │
│  │  ErrDeviceIDRequired         Device ID is required                      │   │
│  │  ErrDeviceNameRequired       Device name is required                    │   │
│  │  ErrProtocolRequired         Protocol must be specified                 │   │
│  │  ErrNoTagsDefined            At least one tag must be defined           │   │
│  │  ErrPollIntervalTooShort     Poll interval must be ≥100ms               │   │
│  │  ErrUNSPrefixRequired        UNS prefix is required for MQTT routing    │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  CONNECTION ERRORS                                                      │   │
│  │                                                                         │   │
│  │  ErrConnectionFailed         Failed to establish connection             │   │
│  │  ErrConnectionTimeout        Connection attempt timed out               │   │
│  │  ErrConnectionClosed         Connection was closed unexpectedly         │   │
│  │  ErrConnectionReset          Connection was reset by peer               │   │
│  │  ErrMaxRetriesExceeded       Maximum retry attempts exceeded            │   │
│  │  ErrCircuitBreakerOpen       Circuit breaker is open                    │   │
│  │  ErrPoolExhausted            Connection pool exhausted                  │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  MODBUS-SPECIFIC ERRORS                                                 │   │
│  │                                                                         │   │
│  │  ErrModbusIllegalFunction    Function code not supported (0x01)         │   │
│  │  ErrModbusIllegalAddress     Invalid register address (0x02)            │   │
│  │  ErrModbusIllegalValue       Invalid data value (0x03)                  │   │
│  │  ErrModbusSlaveFailure       Slave device failure (0x04)                │   │
│  │  ErrModbusAcknowledge        Request acknowledged, processing (0x05)    │   │
│  │  ErrModbusSlaveBusy          Slave device busy (0x06)                   │   │
│  │  ErrInvalidSlaveID           Slave ID must be 1-247                     │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  OPC UA-SPECIFIC ERRORS                                                 │   │
│  │                                                                         │   │
│  │  ErrOPCUAInvalidNodeID       Node ID format is invalid                  │   │
│  │  ErrOPCUANodeNotFound        Node does not exist                        │   │
│  │  ErrOPCUASubscriptionFailed  Failed to create subscription              │   │
│  │  ErrOPCUATypeMismatch        Value type doesn't match node type         │   │
│  │  ErrOPCUAAccessDenied        Access denied to node                      │   │
│  │  ErrOPCUATooManySessions     Server session limit reached               │   │
│  │  ErrOPCUASecurityRejected    Security policy rejected                   │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  S7-SPECIFIC ERRORS                                                     │   │
│  │                                                                         │   │
│  │  ErrS7ConnectionFailed       Failed to connect to PLC                   │   │
│  │  ErrS7InvalidAddress         Invalid S7 address format                  │   │
│  │  ErrS7InvalidArea            Invalid memory area                        │   │
│  │  ErrS7DataBlockNotFound      Data block does not exist                  │   │
│  │  ErrS7AddressOutOfRange      Address exceeds block size                 │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  MQTT-SPECIFIC ERRORS                                                   │   │
│  │                                                                         │   │
│  │  ErrMQTTConnectionFailed     Failed to connect to broker                │   │
│  │  ErrMQTTPublishFailed        Failed to publish message                  │   │
│  │  ErrMQTTNotConnected         MQTT client not connected                  │   │
│  │  ErrMQTTBufferFull           Message buffer overflow                    │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  SERVICE ERRORS                                                         │   │
│  │                                                                         │   │
│  │  ErrServiceNotStarted        Service has not been started               │   │
│  │  ErrServiceStopped           Service has been stopped                   │   │
│  │  ErrServiceOverloaded        Service is overloaded                      │   │
│  │  ErrDeviceNotFound           Device not found in configuration          │   │
│  │  ErrTagNotFound              Tag not found on device                    │   │
│  │  ErrProtocolNotSupported     Protocol not registered                    │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### Appendix C: Dependency Inventory

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                         GO MODULE DEPENDENCIES                                 │
│                                                                                │
│  Core Dependencies:                                                            │
│  ┌───────────────────────────────────────────────────────────────────────────┐ │
│  │ Module                              │ Version  │ Purpose                  │ │
│  │─────────────────────────────────────┼──────────┼──────────────────────────│ │
│  │ github.com/eclipse/paho.mqtt.golang │ v1.4.3   │ MQTT client              │ │
│  │ github.com/goburrow/modbus          │ v0.1.0   │ Modbus TCP/RTU client    │ │
│  │ github.com/gopcua/opcua             │ v0.5.3   │ OPC UA client            │ │
│  │ github.com/robinson/gos7            │ v0.0.0   │ Siemens S7 client        │ │
│  │ github.com/prometheus/client_golang │ v1.19.0  │ Prometheus metrics       │ │
│  │ github.com/rs/zerolog               │ v1.32.0  │ Structured logging       │ │
│  │ github.com/sony/gobreaker           │ v0.5.0   │ Circuit breaker          │ │
│  │ github.com/spf13/viper              │ v1.18.2  │ Configuration            │ │
│  │ gopkg.in/yaml.v3                    │ v3.0.1   │ YAML parsing             │ │
│  └─────────────────────────────────────┴──────────┴──────────────────────────┘ │
│                                                                                │
│  Transitive Dependencies (selected):                                           │
│  ┌───────────────────────────────────────────────────────────────────────────┐ │
│  │ github.com/gorilla/websocket        │ v1.5.0   │ MQTT WebSocket support   │ │
│  │ github.com/goburrow/serial          │ v0.1.0   │ Modbus RTU serial        │ │
│  │ google.golang.org/protobuf          │ v1.32.0  │ Protocol Buffers         │ │
│  │ golang.org/x/sync                   │ v0.6.0   │ Sync primitives          │ │
│  └─────────────────────────────────────┴──────────┴──────────────────────────┘ │
│                                                                                │
│  Build Tools:                                                                  │
│  • Go 1.22+                                                                    │
│  • golangci-lint (linting)                                                     │
│  • air (hot reload)                                                            │
│  • gosec (security scanning)                                                   │
│  • govulncheck (vulnerability scanning)                                        │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---

## 17. Conclusion

This Protocol Gateway represents a production-grade implementation of a multi-protocol industrial data acquisition system. Key architectural achievements include:

**Scalability**
- Connection pooling with per-endpoint session sharing (OPC UA)
- Worker pool-based polling with back-pressure handling
- Object pooling for high-throughput data point processing

**Reliability**
- Multi-tier circuit breakers preventing cascade failures
- Graceful degradation maintaining partial service
- Message buffering during MQTT disconnections
- Exponential backoff with jitter for reconnection storms

**Observability**
- Prometheus metrics for all critical operations
- Structured JSON logging with contextual fields
- Kubernetes-compatible health probes with flapping protection

**Standards Compliance**
- Full Modbus TCP/RTU (IEC 61158) support
- OPC UA (IEC 62541) with security profiles
- Siemens S7 protocol support
- MQTT 3.1.1 (OASIS) with UNS topic structure

**Operational Excellence**
- Hot-reload device configuration
- Web UI for runtime management
- Container-first deployment
- Comprehensive error handling

The architecture follows Clean Architecture principles with clear separation between domain logic, adapters, and infrastructure, enabling future protocol additions and deployment flexibility.

---

*Document Version: 2.0.0*
*Last Updated: January 2026*