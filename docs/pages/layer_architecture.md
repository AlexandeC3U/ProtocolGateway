- [4. Layer Architecture](#4-layer-architecture)
  - [4.1 Domain Layer (`internal/domain/`)](#41-domain-layer-internaldomain)
    - [4.1.1 Entity Relationship Diagram](#411-entity-relationship-diagram)
    - [4.1.2 Device Entity](#412-device-entity)
    - [4.1.3 Tag Entity](#413-tag-entity)
    - [4.1.4 DataPoint Entity](#414-datapoint-entity)
    - [4.1.5 Quality Enumeration](#415-quality-enumeration)
    - [4.1.6 Object Pooling for DataPoint](#416-object-pooling-for-datapoint)
  - [4.2 Adapter Layer (`internal/adapter/`)](#42-adapter-layer-internaladapter)
    - [4.2.1 Adapter Architecture Overview](#421-adapter-architecture-overview)
    - [4.2.2 Protocol Adapter Source File Structure](#422-protocol-adapter-source-file-structure)

## 4. Layer Architecture

### 4.1 Domain Layer (`internal/domain/`)

The domain layer is the **heart of the system**, containing business entities, rules, and interfaces that are protocol-agnostic.

#### 4.1.1 Entity Relationship Diagram

The core domain model consists of three primary entities: `Device` (physical or logical endpoint), `Tag` (individual data point with addressing), and `DataPoint` (runtime measurement with quality and timestamps). This diagram shows their relationships and key attributes. A `Device` contains multiple `Tag` entities, and each poll cycle produces `DataPoint` instances for enabled `Tag` entities. The separation between configuration (`Device`/`Tag`) and runtime data (`DataPoint`) enables hot-reload of device configurations without losing operational state.

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
| `UNSPrefix` mandatory | Ensures ISA-95 compliant topic hierarchy | ISA-95 / Unified Namespace Pattern |
| Configuration versioning | Enables rollback on misconfiguration | IEC 62443-3-3: Configuration management |
| Separate `Enabled` flag | Allows configuration without activation | Common PLC programming practice |

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

Precise timestamping is critical for industrial applications including event sequence recording, latency monitoring, and regulatory compliance (FDA 21 CFR Part 11). The following diagram illustrates the four timestamp stages a data point traverses, enabling accurate `LatencyMs` and `StalenessMs` calculations essential for time-series analysis and control loop optimization:

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

| Gateway `Quality` | OPC UA StatusCode | Description |
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

Object pooling via `sync.Pool` dramatically reduces garbage collection pressure in high-throughput scenarios. The diagram below quantifies the performance impact -- with 5,000 data points per second, pooling eliminates allocations and maintains sub-millisecond latency, crucial for real-time industrial applications where GC pauses are unacceptable:

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

The adapter layer provides concrete implementations for each industrial protocol. The diagram below shows how the `ProtocolManager` routes operations to the appropriate connection pool based on device protocol. Each pool manages its own connections, circuit breakers, and protocol-specific optimizations (batching for Modbus, subscriptions for OPC UA). The `MQTTPublisher` handles outbound message delivery with buffering and reconnection logic.

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