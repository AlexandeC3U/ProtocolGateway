# TODO — Connector Gateway Roadmap

Last verified against codebase: **2026-02-23**

Items are organized by priority. Each item notes what exists today vs what's missing.

---

## High — Significant Improvements

### 6. Separate Worker Pools Per Priority/QoS Tier - planned for V2

**Status**: Foundation exists but is not wired in.

**What exists**:
- `Tag.Priority` field (domain/tag.go:112-117) — 0=telemetry, 1=control, 2=safety
- `DataPoint.Priority` field (domain/datapoint.go:81-82) + `WithPriority()` chainable method
- OPC UA load shaping already has priority queues (Safety > Control > Telemetry) with brownout mode

**What's missing**:
- Polling service treats all tags equally — no priority-based scheduling
- No separate goroutine pools per tier
- No separate MQTT QoS levels per priority
- No guarantee that a telemetry flood won't block safety writes

---

## Medium — Feature Gaps

### 8. OPC UA Browse & Model Awareness

**Status**: `BrowseResult` struct exists in `opcua/types.go:198-204` but no `Browse()` function is implemented. OPC UA is treated as a flat node reader — users must manually enter NodeIDs.

**What's needed**:
- `Browse(ctx, nodeID)` function to walk the address space tree
- `GetNodeAttributes(ctx, nodeID)` to read DataType, AccessLevel, EngineeringUnits
- Wire into Web UI for tag auto-discovery (instead of manual NodeID entry)
- Cache results to avoid re-browsing on every connection

---

### 11. OPC UA Type System Fidelity - planned for V2

**Status**: Currently flattens all OPC UA values via `v.Value()` to basic Go types. Loses array types, LocalizedText, ExtensionObjects, Enums, structured types.

**What's needed**:
- Type-aware variant conversion
- Configurable "preserve types" mode for downstream consumers that can handle rich types

---

### 12. OPC UA Certificate Trust Store Management

**Status**: `security.go` loads certificates and validates them. But there's no trust list management, rejected certs folder, auto-accept for dev mode, or expiry monitoring.

**What's needed for production deployments**:
- Trust/reject list management
- Certificate expiry monitoring with alerting
- Auto-accept mode for development (with warnings)
- GDS (Global Discovery Server) integration for large deployments

---

### 14. Native MQTT Device Support (MQTT → MQTT) - planned for V2 

**Status**: Partial foundation exists, but end-to-end ingestion is **not implemented**.

**What exists**:
- MQTT publisher + reconnect/buffering (`internal/adapter/mqtt/publisher.go`)
- MQTT subscription for *commands* (write path) via `CommandHandler` (`internal/service/command_handler.go`)
- `ProtocolMQTT` exists in `domain` (`internal/domain/device.go`) and tag validation allows it (`internal/domain/tag.go`)

**What's missing**:
- No `ProtocolPool` implementation for MQTT (no MQTT "client" that subscribes to telemetry topics and produces `DataPoint`s)
- `main.go` does not register a pool for `ProtocolMQTT`, so devices configured with protocol `mqtt` are treated as unsupported and skipped

---

#### Architecture Spec

##### Fundamental Difference: Push vs Poll

The three existing adapters (Modbus, OPC UA, S7) are **poll-based** — the gateway initiates reads on a timer. MQTT is **push-based** — the source broker delivers messages via subscriptions. This means:

- **No polling goroutine needed**: The `PollingService` should detect `ProtocolMQTT` devices and skip creating a ticker-based `devicePoller`. Instead, the MQTT source adapter delivers `DataPoint`s directly to the publisher via a callback.
- **`ReadTags` still works**: For compatibility with the `ProtocolPool` interface, `ReadTags` can return the latest cached values (last-value cache per tag). This enables health checks, test-connection, and status queries.
- **`WriteTag` publishes**: A write to an MQTT device = publish a message to a configured "command" topic on the source broker.

##### Connection Model: Per-Broker Client Sharing

```
┌─────────────────────────────────────────────────────────────┐
│                   MQTT Source Pool                          │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  brokerClients map[brokerKey]*brokerClient           │   │
│  │                                                      │   │
│  │  Key = hash(broker_url + username + tls_config)      │   │
│  │                                                      │   │
│  │  ┌────────────────────┐  ┌────────────────────┐      │   │
│  │  │ broker: mqtt://A   │  │ broker: mqtts://B  │      │   │
│  │  │ paho.Client        │  │ paho.Client        │      │   │
│  │  │ devices: [D1,D2,D3]│  │ devices: [D4,D5]   │      │   │
│  │  │ subs: 15 topics    │  │ subs: 8 topics     │      │   │
│  │  │ breaker: closed    │  │ breaker: open      │      │   │
│  │  └────────────────────┘  └────────────────────┘      │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                             │
│  Unlike Modbus/S7 (1 connection per device), MQTT devices   │
│  sharing the same broker reuse a single TCP connection.     │
│  Same pattern as OPC UA session sharing per endpoint.       │
└─────────────────────────────────────────────────────────────┘
```

**Broker key**: `broker_url + username + password_hash + tls_fingerprint`. Devices with identical broker configs share one `paho.Client`. Changing a credential triggers a new client (same as OPC UA cert rotation).

**Why not one client per device?** A device is a logical grouping of topics. 50 IoT sensors on the same EMQX broker should not open 50 TCP connections — MQTT brokers (and firewalls) have connection limits. One client with 50 subscriptions is vastly more efficient.

**Why not reuse the existing publisher client?** The source broker may be different from the gateway's output broker. Even if they're the same, using a separate client provides isolation (source subscriptions don't interfere with publish QoS), independent reconnect, and a clean client ID namespace (e.g., `gateway-source-{hash}` vs `gateway-publisher`).

##### Circuit Breakers: Two-Tier (Like OPC UA)

```
┌──────────────────────────────────────────────────────┐
│  Broker Breaker (per brokerClient)                   │
│  Triggers on: connection lost, auth failure,         │
│               repeated subscribe failures            │
│  Effect: ALL devices on this broker are blocked      │
│                                                      │
│  Device Staleness Detector (per device)              │
│  Triggers on: no messages received for               │
│               staleness_timeout (default: 5×interval)│
│  Effect: device.Status → "stale", quality →          │
│          "uncertain", alert via metrics              │
│  NOT a circuit breaker — the device isn't "failing", │
│  it's just silent. No requests to block.             │
└──────────────────────────────────────────────────────┘
```

Traditional per-device circuit breakers don't apply here — there are no outbound requests to block. Instead, "device health" is inferred from message frequency. If a device that normally publishes every 5s goes silent for 25s, it's marked stale.

##### Subscription Strategy: Hybrid Wildcard + Per-Tag Routing

Per-tag subscriptions don't scale. A real UNS hierarchy like:
```
nexus-spark/delaware/Gent/Production/Line1/RAW/Extruder/ThermalControl
nexus-spark/delaware/Gent/Production/Line1/RAW/Extruder/MaterialPrep
nexus-spark/delaware/Gent/Production/Line1/RAW/Extruder/Drive
nexus-spark/delaware/Gent/Production/Line1/RAW/Extruder/Energy
... (20+ leaf topics per device)
```
Would mean 20+ individual SUBSCRIBE packets per device. With 50 devices = 1000+ subscriptions on a single broker client.

**Instead: one wildcard subscription per device, with per-tag decode routing.**

```yaml
# devices.yaml — MQTT device example
- id: "extruder-line1"
  name: Line 1 RAW Extruder
  protocol: mqtt
  enabled: true
  uns_prefix: plant1/line1/extruder   # output UNS prefix
  connection:
    mqtt_broker_url: tcp://edge-broker:1883
    mqtt_username: gateway
    mqtt_password: secret123
    mqtt_qos: 1
    mqtt_clean_session: true
    mqtt_staleness_timeout: 30s
    mqtt_source_prefix: "nexus-spark/delaware/Gent/Production/Line1/RAW/Extruder"  # ← ONE subscription: .../Extruder/#
  tags:
    # Explicit tags — known leaf topics with specific decode rules
    - id: energy
      name: Energy Consumption
      mqtt_topic_match: "Energy"          # matches ...Extruder/Energy
      mqtt_payload_format: json
      mqtt_value_path: "$.value"
      mqtt_timestamp_path: "$.timestamp"
      data_type: float64
      unit: kWh
    - id: thermal-control
      name: Thermal Control
      mqtt_topic_match: "ThermalControl"  # matches ...Extruder/ThermalControl
      mqtt_payload_format: json
      mqtt_value_path: "$.value"
      data_type: float64
    # Catch-all — auto-create tags for any unmatched subtopics
    # (optional, enabled via auto_discover_tags on the device)
```

**How it works:**
1. **Device registers** → pool subscribes to `{mqtt_source_prefix}/#` (ONE subscription)
2. **Message arrives** on `nexus-spark/.../Extruder/Energy` → strip prefix → suffix = `Energy`
3. **Suffix match** against tags: `Energy` matches tag `energy` → decode with its rules
4. **Unmatched suffix** (e.g., `Process/SubZone3`): if `auto_discover_tags: true`, auto-create a tag with default decode (json, `$.value`); otherwise log and discard
5. **Per-tag decode** is preserved — each tag still defines its own `payload_format`, `value_path`, `data_type`
6. **Per-tag QoS** is NOT needed (MQTT QoS is per-subscription, and we have one wildcard subscription per device — use the device-level QoS)

**Why this is better:**
- 1 subscription per device instead of N (50 devices = 50 subscriptions, not 1000+)
- Automatic discovery of new topics under the prefix (no config change needed when a new sensor appears)
- Still supports per-tag decode rules for known tags
- Simpler broker-side subscription management

**Escape hatch for non-hierarchical topics**: If a device's topics truly don't share a prefix, fall back to explicit per-tag subscriptions by setting `mqtt_source_prefix: ""` and specifying `mqtt_source_topic` on each tag. This is the exception, not the default.

**Topic suffix matching**: Uses simple suffix comparison (split on `/`, match from the right). Supports `+` single-level wildcard in `mqtt_topic_match` for patterns like `+/Energy` to match `ThermalControl/Energy` and `Drive/Energy`.

##### Payload Decoding Pipeline

```
Incoming MQTT Message
        │
        ▼
┌───────────────────┐
│ Match to Tag      │  (by source topic → tag lookup map)
│ (may match 1+ tags│   if wildcard subscription)
└───────┬───────────┘
        │
        ▼
┌───────────────────────────────────────────────────────────────────────────┐
│ Decode Payload                                                            │
│                                                                           │
│ raw:         bytes → Go type via data_type (like Modbus parseValue)       │
│ string:      UTF-8 string → strconv.ParseFloat / ParseBool / etc.         │
│ json:        JSON unmarshal → JSONPath extract value + optional timestamp │
│ sparkplug_b: Protobuf decode → extract metric by name                     │
└───────┬───────────────────────────────────────────────────────────────────┘
        │
        ▼
┌───────────────────────────────────────────────────────┐
│ Build DataPoint                                       │
│                                                       │
│ DeviceID, TagID, Value, Quality=good                  │
│ DeviceTimestamp = extracted or message timestamp      │
│ GatewayTimestamp = now                                │
│ Topic = uns_prefix + "/" + topic_suffix               │
└───────┬───────────────────────────────────────────────┘
        │
        ▼
┌───────────────────────────────────────────────────────┐
│ Deliver                                               │
│                                                       │
│ 1. Update last-value cache (for ReadTags)             │
│ 2. Call dataHandler callback → MQTT publisher         │
│ 3. Update staleness timer                             │
│ 4. Update metrics (messages received, decode errors)  │
└───────────────────────────────────────────────────────┘
```

##### Topic Loop Prevention

When the source broker IS the same as the output broker (common in single-broker deployments), the gateway must not re-ingest its own published messages.

**Three-layer protection:**
1. **Client ID filtering**: The source client's `OnMessage` handler checks if the message originated from the gateway's publisher client ID (via MQTT v5 `$share` or client-id metadata). Not available in MQTT v3.1.1.
2. **Topic prefix guard** (primary): Source topics and UNS output topics should use disjoint prefixes. Validation at config load: if `mqtt_source_topic` overlaps with `uns_prefix + "/" + topic_suffix`, reject with a config error.
3. **Message tagging**: The publisher adds a user property `_gw=1` to all published messages. The source adapter drops any incoming message with this property. Works with MQTT v5; for v3.1.1, falls back to layer 2.

##### No Polling / No Batching

- **No polling needed**: Messages arrive via subscription callbacks. The `PollingService` checks `device.Protocol == ProtocolMQTT` and skips `startDevicePoller()`.
- **No batch reads**: Unlike Modbus (where batching reduces round trips), MQTT messages arrive one-at-a-time. There's no equivalent of "read 100 registers in one request."
- **Micro-batching output**: If a burst of messages arrives (e.g., 50 sensor readings in 100ms), the adapter could buffer and call `PublishBatch()` instead of `Publish()` for each. Optional optimization with configurable `batch_window` (e.g., 50ms). Default: immediate delivery (no batching).

##### ConnectionConfig Additions

New fields needed on `ConnectionConfig` for MQTT devices:

```go
// === MQTT Source Settings ===
MQTTBrokerURL         string        // Source broker URL (tcp:// or ssl://)
MQTTUsername           string        // Broker authentication
MQTTPassword           string
MQTTClientIDPrefix     string        // Client ID prefix (default: "gw-source")
MQTTQOS                byte          // Default QoS for device wildcard subscription (0, 1, 2)
MQTTCleanSession       bool          // Clean session on connect
MQTTStalenessTimeout   time.Duration // Mark device stale after no messages (0 = disabled)
MQTTSourcePrefix       string        // Topic prefix → subscribe to {prefix}/# (one sub per device)
MQTTAutoDiscoverTags   bool          // Auto-create tags for unmatched subtopics under the prefix
MQTTTLSEnabled         bool          // TLS for source broker
MQTTTLSCAFile          string
MQTTTLSCertFile        string
MQTTTLSKeyFile         string
```

New fields on `Tag` for MQTT-sourced tags:

```go
MQTTTopicMatch    string // Suffix to match under device's source prefix (e.g., "Energy", "Drive/Speed")
MQTTSourceTopic   string // Full topic override — used when mqtt_source_prefix is empty (per-tag fallback)
MQTTPayloadFormat string // "raw" | "string" | "json" | "sparkplug_b"
MQTTValuePath     string // JSONPath for value extraction (json format only)
MQTTTimestampPath string // JSONPath for timestamp extraction (optional)
```

##### File Layout

```
internal/adapter/mqtt/
├── publisher.go          # Existing — outbound publishing (unchanged)
├── source_pool.go        # NEW — ProtocolPool implementation, broker client management
├── source_client.go      # NEW — Per-broker MQTT client, subscription management
├── decoder.go            # NEW — Payload decoding: raw, string, json, sparkplug_b
├── source_types.go       # NEW — SourceConfig, TagMapping, last-value cache types
└── source_health.go      # NEW — Staleness detection, per-device health, pool stats
```

##### Wiring (main.go)

```go
// After existing pool registrations:
mqttSourcePool := mqtt.NewSourcePool(cfg.MQTTSource, logger, metricsRegistry, mqttPublisher)
protocolManager.RegisterPool(domain.ProtocolMQTT, mqttSourcePool)
// mqttSourcePool.Start() — begins subscribing for registered devices
```

##### Scope for v1 (Minimal)

1. `source_pool.go` + `source_client.go` — broker sharing, per-device wildcard subscription lifecycle
2. `decoder.go` — `raw` and `json` formats only (sparkplug_b deferred)
3. Wildcard subscription → suffix-based tag routing (core of the hybrid strategy)
4. Topic loop prevention via prefix guard (layer 2)
5. Staleness detection with `gateway_mqtt_source_device_stale` metric
6. Last-value cache for `ReadTags` compatibility
7. Wire into `main.go`

**Deferred to v2:**
- Sparkplug B decoding (requires protobuf dependency)
- MQTT v5 message properties for loop prevention
- Micro-batching output
- Auto-discovery of unmatched topics as new tags (`auto_discover_tags`)
- Per-tag fallback subscriptions (for non-hierarchical topic layouts)
- MQTT source metrics dashboard (Grafana panel)

## Low — Nice to Have

### 17. OPC UA Event & Alarm Support - planned for V2

**Status**: Not implemented. Full OPC UA Alarms & Conditions (A&C) is a large subsystem:
- Event subscriptions (not just data changes)
- Alarm acknowledgment flow
- Historical Data Access (HDA)

Consider as a separate project phase.

---

### 20. Cross-Protocol Tag & Topic Browsing (Auto-Discovery) - planned for V2

**Status**: Manual tag entry only. No browse/discovery for any protocol.

**Problem**: When a device has hundreds or thousands of tags (common in manufacturing PLCs, large UNS deployments), configuring them 1-by-1 in `devices.yaml` or via the API is impractical. Users need a way to browse the available address space and select tags.

---

#### Architecture Spec

##### Protocol-Specific Browse Capabilities

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Protocol    │ Native Browse? │ Strategy                                │
├──────────────┼────────────────┼─────────────────────────────────────────┤
│  OPC UA      │ YES            │ Browse Services (already in spec,       │
│              │                │ see TODO #8). Walk address space tree,  │
│              │                │ return NodeID + DataType + AccessLevel  │
│              │                │                                         │
│  MQTT        │ PARTIAL        │ Wildcard probe: subscribe to prefix/#   │
│              │                │ for N seconds, collect unique topic     │
│              │                │ suffixes → present as discovered tags.  │
│              │                │ Also: $SYS/# for broker stats.          │
│              │                │                                         │
│  S7          │ YES (limited)  │ Read PLC symbol table via SZL           │
│              │                │ (System Status List). Returns DB        │
│              │                │ numbers, offsets, data types. S7-1500   │
│              │                │ exposes symbolic names via TIA.         │
│              │                │                                         │
│  Modbus      │ NO             │ No native browse. Offer "scan range":   │
│              │                │ try reading address ranges (e.g.,       │
│              │                │ HR 0-999) and report which respond.     │
│              │                │ Coil/register map must come from vendor │
│              │                │ documentation.                          │
└─────────────────────────────────────────────────────────────────────────┘
```

##### Unified Browse Interface

```go
// BrowseResult represents a discovered tag/node from any protocol.
type BrowseResult struct {
    ID          string            // Suggested tag ID (sanitized node name / topic suffix)
    Name        string            // Human-readable name
    Address     string            // Protocol-specific address (NodeID, DB1.DBW0, topic path)
    DataType    DataType          // Detected data type (if available)
    AccessMode  AccessMode        // Read, Write, ReadWrite (if detectable)
    Unit        string            // Engineering unit (OPC UA provides this)
    Path        []string          // Hierarchical path for tree display
    Metadata    map[string]string // Protocol-specific extras (OPC UA: namespace, S7: DB number)
    Children    int               // Number of child nodes (for tree expansion)
}

// ProtocolBrowser is implemented per protocol adapter.
type ProtocolBrowser interface {
    // Browse returns discovered tags/nodes under the given root.
    // rootPath is protocol-specific:
    //   OPC UA: NodeID string (e.g., "ns=2;s=MyFolder")
    //   MQTT:   Topic prefix (e.g., "nexus-spark/delaware/Gent")
    //   S7:     "" for all DBs, "DB1" for specific DB
    //   Modbus: Register type + range (e.g., "HR:0-999")
    Browse(ctx context.Context, device *Device, rootPath string, depth int) ([]BrowseResult, error)
}
```

##### API Endpoints

```
POST /api/browse
Body: { "device_id": "plc-001", "root_path": "", "depth": 2 }
Response: { "results": [ ...BrowseResult... ] }

POST /api/browse/import
Body: { "device_id": "plc-001", "tags": [ { "address": "DB1.DBW0", "name": "Temperature" }, ... ] }
Effect: Bulk-add selected browse results as tags to the device
```

##### Per-Protocol Implementation

**OPC UA** (`internal/adapter/opcua/browser.go`):
- Uses `session.Browse()` and `session.BrowseNext()` from gopcua
- Starts at `rootPath` (default: `ObjectsFolder` / `i=85`)
- Returns child nodes with their NodeID, BrowseName, DataType, AccessLevel
- Recursive with configurable `depth` (default 1 = immediate children)
- Filters: skip internal/system nodes, configurable namespace filter
- Extends existing `BrowseResult` struct in `opcua/types.go`

**MQTT** (`internal/adapter/mqtt/browser.go`):
- Subscribes to `{rootPath}/#` with QoS 0
- Collects unique topics for `browse_duration` (default: 10s)
- Groups by hierarchy level, counts messages per topic
- Returns leaf topics as `BrowseResult` with detected payload format
- Payload sniffing: try JSON parse → if valid, report field names as potential value paths
- Auto-detects UNS structure by counting hierarchy depth

**S7** (`internal/adapter/s7/browser.go`):
- Uses SZL read (System Status List) to enumerate DBs: `SZL ID 0x0111` → DB list
- For each DB: read DB attributes (size, read/write flags)
- For S7-1500 with TIA Portal export: parse symbol XML for named variables with types
- Fallback for S7-300/400: report DB number + size, user provides offset/type manually
- Returns `BrowseResult` per DB with size metadata

**Modbus** (`internal/adapter/modbus/browser.go`):
- No native browse — performs "scan range" probe
- Reads holding registers in chunks of 125, reports which address ranges respond
- Tries coils in chunks of 1000
- Reports accessible ranges with response timing (slow responses may indicate gateway-forwarded devices)
- Optional: Modbus device identification (function code 0x2B/0x0E) for vendor/product info

##### Web UI Integration

```
Device Config Page
├── Tags section
│   ├── [Manual Add] button (existing)
│   └── [Browse / Discover] button (NEW)
│       └── Opens browse modal:
│           ├── Tree view of discovered nodes (expandable)
│           ├── Checkbox select for bulk import
│           ├── Preview of selected tags before import
│           └── [Import Selected] → bulk POST /api/browse/import
```

##### Scope

**v1**: OPC UA browse + MQTT topic discovery (highest value protocols for auto-discovery)
**v2**: S7 SZL browse + Modbus scan range + Web UI browse modal
**v3**: Scheduled re-browse for drift detection (new nodes appeared, old nodes removed)

---