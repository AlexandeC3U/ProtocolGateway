- [3. Architectural Principles](#3-architectural-principles)
  - [3.1 Clean Architecture Adherence](#31-clean-architecture-adherence)
  - [3.2 Interface Segregation](#32-interface-segregation)
  - [3.3 Dependency Inversion](#33-dependency-inversion)

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

- **Modbus**: Supports `ReadTags` and `WriteTag` for holding registers, but coils may be read-only in some configurations
- **OPC UA**: May have nodes that are subscription-only (no explicit read)
- **S7**: Some memory areas (Inputs) are inherently read-only

### 3.3 Dependency Inversion

The Dependency Inversion Principle (DIP) is fundamental to the gateway's extensibility. The diagram below shows how the high-level `PollingService` depends on an abstraction (`ProtocolPool` interface) rather than concrete implementations. This means new protocols can be added without modifying existing polling logic—simply implement the interface and register it with the `ProtocolManager`.

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