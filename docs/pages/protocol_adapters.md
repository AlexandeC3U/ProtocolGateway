- [6. Protocol Adapters](#6-protocol-adapters)
  - [6.1 Modbus Adapter](#61-modbus-adapter)
    - [6.1.1 Architecture](#611-architecture)
    - [6.1.2 Protocol Limit Validation](#612-protocol-limit-validation)
    - [6.1.3 Data Conversion Functions](#613-data-conversion-functions)
    - [6.1.4 File Map](#614-file-map)
    - [6.1.5 Byte Order Handling](#615-byte-order-handling)
  - [6.2 OPC UA Adapter](#62-opc-ua-adapter)
    - [6.2.1 Session Architecture](#621-session-architecture)
    - [6.2.2 Session State Machine](#622-session-state-machine)
    - [6.2.3 Load Shaping System](#623-load-shaping-system)
    - [6.2.4 Address Space Browse](#624-address-space-browse)
    - [6.2.5 Certificate Trust Store](#625-certificate-trust-store)
    - [6.2.8 File Map](#628-file-map)
  - [6.3 S7 Adapter](#63-s7-adapter)
    - [6.3.1 Address Parsing](#631-address-parsing)
    - [6.3.2 Batch Read Strategy](#632-batch-read-strategy)
    - [6.3.4 File Map](#634-file-map)
  - [6.4 MQTT Publisher](#64-mqtt-publisher)
    - [6.4.1 Message Flow Architecture](#641-message-flow-architecture)
    - [6.4.2 MQTT Payload Format](#642-mqtt-payload-format)

## 6. Protocol Adapters

### 6.1 Modbus Adapter

#### 6.1.1 Architecture

The Modbus adapter uses a per-device connection model with individual circuit breakers to isolate failures. The diagram shows the `ModbusPool` connection pool structure and the batch optimization algorithm that groups contiguous registers into single read operations, reducing network round-trips by up to 70% for typical configurations:

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
│  │  │  Batch optimization applies to ALL register types:                │  │   │
│  │  │  • Holding/Input registers: buildContiguousRanges() merges nearby │  │   │
│  │  │    addresses (max gap = 10 registers, max 100 per read)           │  │   │
│  │  │  • Coils/Discrete Inputs: buildCoilRanges() merges nearby bits    │  │   │
│  │  │    (max gap = 32 coils, max 1000 per read, 8 coils/byte LSB)      │  │   │
│  │  │                                                                   │  │   │
│  │  │  Input Tags:  [R100, R101, R102, R103, R110, R111, R200]          │  │   │
│  │  │                                                                   │  │   │
│  │  │  Grouping Algorithm (Holding/Input registers):                    │  │   │
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

#### 6.1.2 Protocol Limit Validation

Modbus protocol specifications impose hard limits on the number of items per read request. The gateway validates these limits at request time and rejects requests that exceed them with `ErrModbusRegisterCountExceedsLimit`:

| Register Type | Max Per Read | Modbus Spec Reference |
|---|---|---|
| Holding Registers | 125 | `FC03` response limited to 250 bytes |
| Input Registers | 125 | `FC04` response limited to 250 bytes |
| Coils | 2000 | `FC01` response limited to 250 bytes |
| Discrete Inputs | 2000 | `FC02` response limited to 250 bytes |

**Coil bit extraction:** Coils are packed 8 per byte in the response, LSB-first. To extract individual coil values: `byteIndex = bitOffset / 8`, `bitIndex = bitOffset % 8`. This means reading 1000 coils only transfers 125 bytes.

#### 6.1.3 Data Conversion Functions

| Function | Direction | Description |
|---|---|---|
| `parseValue()` | bytes → typed | Convert raw register bytes to Go type based on `tag.DataType` |
| `valueToBytes()` | typed → bytes | Convert Go value to register bytes for writes |
| `reorderBytes()` | — | Handle byte order: `BigEndian`, `LittleEndian`, `MidBigEndian`, `MidLittleEndian` |
| `applyScaling()` | read path | `output = raw × scaleFactor + offset` |
| `reverseScaling()` | write path | `raw = (input - offset) / scaleFactor` |

#### 6.1.4 File Map

| File | Purpose |
|---|---|
| `client.go` | Single-device Modbus client with batch optimization, retry, conversions |
| `pool.go` | Multi-device connection pool with circuit breakers and idle reaping |
| `health.go` | Per-device and per-tag health diagnostics |
| `types.go` | Type definitions, buffer pool, pool config defaults |
| `conversion.go` | Byte ↔ typed value conversion, scaling, byte order handling |

#### 6.1.5 Byte Order Handling

Modbus devices from different vendors use various byte ordering schemes for multi-register values. The gateway supports all four common formats. This diagram illustrates how a 32-bit value (`0x12345678`) is represented across two 16-bit registers in each format, essential for correctly interpreting data from devices like ABB, Schneider, Siemens, and legacy systems:

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
│  • Session infrastructure supports shared subscription management              │
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
│  ! IMPORTANT: OPC UA Subscription Support Status                               │
│  The SubscriptionManager (subscription.go) is FULLY IMPLEMENTED with           │
│  deadband filtering, notification handling, and recovery logic. However,       │
│  it is NOT YET WIRED into the main polling path. All OPC UA devices            │
│  currently use synchronous polling via ReadTags(). The Device domain           │
│  model includes an OPCUseSubscriptions field (marked Phase 3) but it           │
│  is not yet connected to the subscription infrastructure.                      │
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

#### 6.2.4 Address Space Browse

The OPC UA adapter includes an address space browser that allows users to explore available nodes on a server, making tag configuration significantly easier. Instead of requiring operators to know exact NodeIDs in advance, they can browse the server's object model interactively through the Web UI or REST API.

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                       OPC UA ADDRESS SPACE BROWSE                              │
│                                                                                │
│  Architecture:                                                                 │
│                                                                                │
│  ┌──────────┐     ┌──────────────┐     ┌───────────────┐     ┌─────────────┐   │
│  │  Web UI  │────►│  REST API    │────►│  Pool Browse  │────►│  OPC UA     │   │
│  │  Browse  │     │  Handler     │     │  (cached)     │     │  Server     │   │
│  │  Modal   │◄────│              │◄────│               │◄────│             │   │
│  └──────────┘     └──────────────┘     └───────────────┘     └─────────────┘   │
│                                                                                │
│  Request Flow:                                                                 │
│  1. User clicks "Browse" button on OPC UA tag field                            │
│  2. GET /api/browse/{deviceID}?node_id=&max_depth=2                            │
│  3. Pool checks per-endpoint cache (60s TTL)                                   │
│  4. Cache miss → execute through endpoint circuit breaker                      │
│  5. Client.Browse() reads attributes in batch (3-attribute read per node)      │
│  6. Recurse children up to max_depth (capped at 5)                             │
│  7. User selects Variable node → auto-fills opc_node_id in tag form            │
│                                                                                │
│  Browse Algorithm:                                                             │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │  browseNode(nodeID, maxDepth, currentDepth):                             │  │
│  │    1. Batch read: DisplayName, BrowseName, NodeClass (single request)    │  │
│  │    2. If NodeClass == Variable:                                          │  │
│  │       Read DataType + AccessLevel (second batch request)                 │  │
│  │    3. If currentDepth >= maxDepth:                                       │  │
│  │       checkHasChildren() with RequestedMaxReferences=1 (lightweight)     │  │
│  │       Return result with has_children=true/false                         │  │
│  │    4. Else:                                                              │  │
│  │       browseChildren() with HierarchicalReferences (NodeID 33)           │  │
│  │       Handle continuation points via BrowseNext                          │  │
│  │       Recurse into each child                                            │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
│                                                                                │
│  Caching:                                                                      │
│  • Per-endpoint cache keyed by "endpointKey|nodeID|depth"                      │
│  • 60-second TTL (address space rarely changes at runtime)                     │
│  • Devices sharing an endpoint share the cache                                 │
│  • Cache cleared automatically on session reconnect                            │
│                                                                                │
│  BrowseResult JSON Structure:                                                  │
│  {                                                                             │
│    "node_id": "ns=2;s=Demo.Static.Scalar",                                     │
│    "display_name": "Scalar",                                                   │
│    "browse_name": "Scalar",                                                    │
│    "node_class": 1,                                                            │
│    "node_class_name": "Object",                                                │
│    "has_children": true,                                                       │
│    "children": [                                                               │
│      {                                                                         │
│        "node_id": "ns=2;s=Demo.Static.Scalar.Float",                           │
│        "display_name": "Float",                                                │
│        "node_class_name": "Variable",                                          │
│        "data_type": "Float",                                                   │
│        "access_level": "Read, Write",                                          │
│        "has_children": false                                                   │
│      }                                                                         │
│    ]                                                                           │
│  }                                                                             │
│                                                                                │
│  NodeClass Filter: Object, Variable, Method (ignores types/views)              │
│  Reference Type: HierarchicalReferences with subtypes                          │
│  Max Results Per Node: 1000 (handles continuation points for large servers)    │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

#### 6.2.5 Certificate Trust Store

The OPC UA adapter implements certificate trust management per OPC UA Part 12, enabling secure server certificate validation with a workflow for reviewing and approving unknown certificates. This is critical in industrial environments where servers often use self-signed certificates that cannot be validated against a public CA.

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                    OPC UA CERTIFICATE TRUST STORE                              │
│                                                                                │
│  PKI Directory Structure (per OPC UA Part 12):                                 │
│                                                                                │
│  pki/                                                                          │
│  ├── trusted/                                                                  │
│  │   └── certs/         Trusted CA and server certificates                     │
│  ├── rejected/                                                                 │
│  │   └── certs/         Server certs that failed validation (for review)       │
│  ├── issuers/                                                                  │
│  │   └── certs/         Intermediate CA certificates                           │
│  └── own/                                                                      │
│      ├── cert.der       Gateway's own application certificate                  │
│      └── private/                                                              │
│          └── key.pem    Gateway's private key                                  │
│                                                                                │
│  Certificate Validation Flow (during Connect):                                 │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │                                                                          │  │
│  │   Endpoint Discovery                                                     │  │
│  │   returns ServerCertificate                                              │  │
│  │           │                                                              │  │
│  │           ▼                                                              │  │
│  │   ┌─────────────────┐                                                    │  │
│  │   │ Parse DER cert  │                                                    │  │
│  │   │ SHA-256 fingerprint                                                  │  │
│  │   └────────┬────────┘                                                    │  │
│  │            │                                                             │  │
│  │            ▼                                                             │  │
│  │   ┌─────────────────┐     YES                                            │  │
│  │   │ In trusted/?    │──────────► ALLOW CONNECTION                        │  │
│  │   └────────┬────────┘                                                    │  │
│  │            │ NO                                                          │  │
│  │            ▼                                                             │  │
│  │   ┌─────────────────┐     YES                                            │  │
│  │   │ In rejected/?   │──────────► REJECT (explicit deny)                  │  │
│  │   └────────┬────────┘                                                    │  │
│  │            │ NO (unknown cert)                                           │  │
│  │            ▼                                                             │  │
│  │   ┌─────────────────┐     YES                                            │  │
│  │   │ auto_trust=true?│──────────► Add to trusted/ → ALLOW                 │  │
│  │   └────────┬────────┘            (logs warning)                          │  │
│  │            │ NO                                                          │  │
│  │            ▼                                                             │  │
│  │   Add to rejected/ for                                                   │  │
│  │   manual review via API                                                  │  │
│  │   → REJECT CONNECTION                                                    │  │
│  │                                                                          │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
│                                                                                │
│  Certificate Management API:                                                   │
│  • GET    /api/opcua/certificates/trusted   List trusted certs                 │
│  • GET    /api/opcua/certificates/rejected  List rejected (pending review)     │
│  • POST   /api/opcua/certificates/trust     Promote rejected → trusted         │
│  • DELETE /api/opcua/certificates/trusted?fingerprint=sha256:...               │
│                                                                                │
│  Configuration:                                                                │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │  opcua:                                                                  │  │
│  │    trust_store_path: "./pki"        # PKI directory base path            │  │
│  │    auto_trust: false                # NEVER true in production           │  │
│  │    cert_check_interval: 1h          # Expiry monitoring frequency        │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
│                                                                                │
│  Metrics:                                                                      │
│  • gateway_opcua_certs_total{store="trusted|rejected"}                         │
│  • gateway_opcua_cert_expiry_days{fingerprint, subject}                        │
│                                                                                │
│  Certificate Storage:                                                          │
│  • Certificates stored in DER format                                           │
│  • Filenames: {CommonName}_{fingerprint_8chars}.der                            │
│  • Reads both PEM and DER on load                                              │
│  • Fingerprint: SHA-256 with "sha256:" prefix                                  │
│                                                                                │
│    WARNING: auto_trust=true bypasses certificate validation entirely.          │
│     Only use in development or when connecting to known self-signed servers.   │
│     Production deployments should use manual certificate approval workflow.    │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

#### 6.2.8 File Map

| File | Purpose |
|---|---|
| `session.go` | Per-endpoint session sharing, device bindings, two-tier breaker classification |
| `pool.go` | Connection pool with session management, load shaping integration |
| `health.go` | Pool/session/device health and statistics |
| `conversion.go` | OPC UA Variant ↔ Go type conversion, status code → quality mapping |
| `loadshaping.go` | Fleet-wide load control: priority queues, brownout mode, per-endpoint fairness |
| `security.go` | Certificate loading, endpoint discovery, security config validation |
| `subscription.go` | OPC UA subscription management with deadband filtering and recovery |

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

#### 6.3.2 Batch Read Strategy

Unlike Modbus (which uses address-based contiguous range optimization), S7 uses **simple fixed-size chunking**. Tags are processed in groups of up to `MaxMultiReadItems` (20) using the `AGReadMulti()` function. There is no address-sorting or contiguous-range merging — tags are simply chunked sequentially:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                          S7 BATCH READ STRATEGY                                │
│                                                                                │
│  Input: 45 tags from device                                                    │
│                                                                                │
│  Chunking Algorithm (MaxMultiReadItems = 20):                                  │
│  for i := 0; i < len(tags); i += MaxMultiReadItems                             │
│                                                                                │
│  ┌───────────────────────────────────────────────────────────────────────────┐ │
│  │ Chunk 1: tags[0:20]   → AGReadMulti(20 items)                             │ │
│  │ Chunk 2: tags[20:40]  → AGReadMulti(20 items)                             │ │
│  │ Chunk 3: tags[40:45]  → AGReadMulti(5 items)                              │ │
│  └───────────────────────────────────────────────────────────────────────────┘ │
│                                                                                │
│  Result: 45 tags read with 3 AGReadMulti calls                                 │
│                                                                                │
│  Note: Unlike Modbus, there is no address-based optimization. Tags are         │
│  chunked in their original order. Future optimization could sort by            │
│  memory area and offset to improve PLC read efficiency.                        │
│                                                                                │
│  ───────────────────────────────────────────────────────────────────────────── │
│                                                                                │
│  BATCH WRITE STRATEGY (WriteTags)                                              │
│                                                                                │
│  Same chunking approach as reads: MaxMultiWriteItems = 20, AGWriteMulti()      │
│                                                                                │
│  Special handling for boolean writes:                                          │
│  • Boolean values share bytes with adjacent bits (e.g., M0.0 and M0.1)         │
│  • Writing a boolean requires read-modify-write (RMW) to preserve neighbors    │
│  • Boolean writes are excluded from AGWriteMulti and processed individually    │
│  • Non-boolean writes are batched normally (up to 20 per PDU)                  │
│                                                                                │
│  Per-item error tracking via S7DataItem.Error field allows partial success:    │
│  items 1-19 may succeed while item 20 fails, and each error is reported        │
│  back to the caller at its original index.                                     │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

#### 6.3.4 File Map

| File | Purpose |
|---|---|
| `client.go` | Single-device S7 client with address parsing and error handling |
| `pool.go` | Multi-device connection pool with circuit breakers |
| `health.go` | Per-device health diagnostics and pool statistics |
| `types.go` | Type definitions, S7 area codes, word length constants |
| `conversion.go` | S7 byte ↔ Go type conversion |

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

#### 6.4.2 MQTT Payload Format

Each data point is serialized and published as compact JSON to the topic `{device.UNSPrefix}/{tag.TopicSuffix}`:

```json
{"v": 20.1, "u": "°C", "q": "good", "ts": 1769445124645}
```

| Field | Description |
|---|---|
| `v` | Value (typed: number, bool, string) |
| `u` | Unit (from tag config, optional — omitted if empty) |
| `q` | Quality: good, bad, uncertain, timeout, config_error, device_failure, not_connected |
| `ts` | Timestamp in epoch milliseconds |

---