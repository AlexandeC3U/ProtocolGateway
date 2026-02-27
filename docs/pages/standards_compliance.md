- [15. Standards Compliance](#15-standards-compliance)
  - [15.1 Industrial Protocol Standards](#151-industrial-protocol-standards)
  - [15.2 Unified Namespace (UNS) Architecture](#152-unified-namespace-uns-architecture)
  - [15.3 Sparkplug B Compatibility](#153-sparkplug-b-compatibility)

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
│  │  Subscription Support (implemented, not yet wired into polling):        │   │
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