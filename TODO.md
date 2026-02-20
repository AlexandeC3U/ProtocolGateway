# TODO — Connector Gateway Roadmap

Last verified against codebase: **2026-02-20**

Items are organized by priority. Each item notes what exists today vs what's missing.

---

## Critical — Wire In Existing Code

These features are **already implemented** but never connected to the main application.

### 1. Wire OPC UA Subscriptions into the Polling Path

**Status**: `subscription.go` is **complete** (SubscriptionManager, monitored items, deadband filtering, notification handling, recovery state). But `main.go` never instantiates it and `polling.go` never uses it. All OPC UA devices currently use synchronous polling.

**What exists** (`internal/adapter/opcua/subscription.go`):
- Full SubscriptionManager with Start/Stop lifecycle
- Subscribe/Unsubscribe per device
- Deadband filtering (absolute, percent)
- Notification channel for async data delivery
- Per-tag last-value caching
- Subscription recovery after reconnect (republish + monitored item rebind)

**What exists but is dead code** (`internal/adapter/opcua/pool.go`):
- `recoverSubscriptions()` — called after session reconnect, but subscriptions are never created so `subscriptionState` is always nil

**What exists in domain** (`internal/domain/device.go:174-179`):
- `OPCUseSubscriptions bool` field on ConnectionConfig — marked "Not yet implemented - planned for Phase 3"
- `OPCPublishInterval` and `OPCSamplingInterval` fields already exist

**What's needed**:
1. Instantiate `SubscriptionManager` in `main.go` (or inside `opcua.ConnectionPool`)
2. In `polling.go`, check `device.Connection.OPCUseSubscriptions` — if true, delegate to SubscriptionManager instead of polling
3. Wire SubscriptionManager's `DataHandler` callback to publish data points via MQTT publisher
4. Ensure the subscription path populates DataPoints with the same fields as the polling path (quality, timestamps, latency)
5. Handle mixed mode: some OPC UA devices poll, others subscribe

**Impact**: For slow-changing values (temperature updated every 30s), subscriptions eliminate 29 wasted poll cycles per interval. Reduces OPC UA server load significantly.

---

### ~~2. Test-Connection Handler is a Stub~~ ✅ DONE

**Resolved**: Added `ConnectionTester` interface to `APIHandler` with `SetConnectionTester()` setter. `TestConnectionHandler` now performs a real `ReadTag` against the device's first tag using the protocol pool, with a configurable timeout (falls back to 10s). Returns elapsed time, protocol, and error details on failure (HTTP 503). Gracefully degrades to validation-only when no tester is wired in.

---

### ~~3. MQTT Publish Latency Not Measured~~ ✅ DONE

**Resolved**: Added `publishStart := time.Now()` before `token.WaitTimeout()`. All three exit paths (success, timeout, context cancellation) now pass `time.Since(publishStart).Seconds()` to `RecordMQTTPublish()`. The existing `MQTTPublishLatency` histogram now receives real data.

---

## High — Significant Improvements

### 4. S7 ReadTags Address-Based Batch Optimization

**Status**: S7 `ReadTags()` uses simple fixed-size chunking (20 items per `AGReadMulti` call). Modbus has `buildContiguousRanges()` that merges nearby addresses into contiguous reads, reducing 100 tags to 1-5 reads.

**What exists** (`internal/adapter/s7/client.go`):
- `ReadTags()` chunks tags into groups of `MaxMultiReadItems` (20)
- `readTagBatch()` calls `AGReadMulti()` — multi-read capability is used
- `MaxMultiReadItems = 20` in `types.go`

**What exists in benchmarks** (`testing/benchmark/latency/read_latency_test.go:642-654`):
- Benchmark comments note: "S7 batch efficiency shown here assumes true multi-item PDU reads, which may not be implemented in the actual adapter (currently sequential)" and "Actual adapter may read sequentially, not batched"

**What's missing** (compared to Modbus):
- No sorting by S7 area (DB, M, I, Q) before batching
- No contiguous address merging — 20 scattered tags = 20 items in PDU instead of reading contiguous blocks
- No gap-filling optimization (reading a few extra bytes to merge two nearby ranges)

**Implementation**: Port Modbus's `buildContiguousRanges()` logic:
1. Group tags by S7 area + DB number
2. Sort by byte offset within each group
3. Merge contiguous/nearby ranges (configurable max gap)
4. Read each merged range as a single buffer, then extract tag values by offset

**Estimated impact**: 3-10x fewer round trips for typical 50-100 tag reads, depending on address distribution.

---

### ~~5. Device Edit Resets Polling State~~ ✅ DONE

**Resolved**: Added `PollingService.ReplaceDevice()` that atomically swaps the device pointer while preserving all runtime state (stats, poll counts, last poll time, error history). The next poll cycle automatically picks up new tags, connection config, and UNS prefix. If the poll interval changed, the poller goroutine is restarted with a new ticker; otherwise no goroutine restart occurs. Updated `main.go` device edit callback to use `ReplaceDevice()` instead of the old unregister+register pattern.

---

### 6. Separate Worker Pools Per Priority/QoS Tier

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

### ~~7. Modbus Coil/Discrete Input Batching~~ ✅ DONE

**Resolved**: Added `CoilBatchConfig` (max 1000 coils/read, gap merge ≤ 32), `buildCoilRanges()` for contiguous coil address merging, and `readCoilRange()` that reads a batch via `ReadCoils`/`ReadDiscreteInputs` and extracts individual tag values from the bit-packed response (8 coils per byte, LSB first). `readTagGroup()` now dispatches coils/discrete inputs to `readCoilGroupBatched()` instead of `readTagGroupIndividually()`. Reading 100 scattered coils now takes 1-5 Modbus requests instead of 100.

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

### ~~9. S7 Write Aggregation (Batch Writes)~~ ✅ DONE

**Resolved**: Added `MaxMultiWriteItems` constant (20) and `WriteTags()` batch method on `Client` that uses `AGWriteMulti` for non-boolean writes (up to 20 items per PDU). Boolean writes are excluded from batching because they require read-modify-write to preserve adjacent bits. Pool-level `WriteTags()` now executes the entire batch through the circuit breaker in a single call instead of per-tag. Per-item error tracking via `S7DataItem.Error` field.

---

### ~~10. Connection TTL (Hard Cap)~~ ✅ DONE

**Resolved**: Added `MaxTTL time.Duration` config to all three pool implementations (Modbus `PoolConfig`, S7 `PoolConfig`, OPC UA `PoolConfig`). Added `createdAt time.Time` tracking on client/session creation. Updated idle reapers in all pools to check both idle timeout AND MaxTTL expiry, closing connections that exceed either threshold. Modbus and S7 reap in their `reapIdleConnections()` loops; OPC UA reaps in `reapIdleSessions()` with a two-pass approach (identify then close under write lock).

---

### 11. OPC UA Type System Fidelity

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

### ~~13. Per-Device Circuit Breaker Configuration~~ ✅ DONE

**Resolved**: Added `CircuitBreakerConfig` struct to `domain` package with fields: `MaxRequests`, `Interval`, `Timeout`, `FailureThreshold`, `FailureRatio`. Added optional `CircuitBreaker *CircuitBreakerConfig` field to `ConnectionConfig`. Updated all three pool implementations (Modbus, S7, OPC UA device-level breaker) to apply per-device overrides when present, falling back to pool defaults for any zero-value field.

---

### 14. Native MQTT Device Support (MQTT → MQTT)

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

##### Subscription Strategy: Per-Tag Topics

```yaml
# devices.yaml — MQTT device example
- id: "iot-sensor-floor2"
  name: Floor 2 Environmental Sensors
  protocol: mqtt
  enabled: true
  uns_prefix: plant1/floor2/environment
  connection:
    mqtt_broker_url: tcp://edge-broker:1883
    mqtt_username: gateway
    mqtt_password: secret123
    mqtt_qos: 1
    mqtt_clean_session: true
    mqtt_staleness_timeout: 30s     # mark stale if no messages for 30s
  tags:
    - id: temp-1
      name: Temperature
      topic_suffix: temperature
      mqtt_source_topic: "sensors/floor2/temp/value"     # ← subscribe to this
      mqtt_payload_format: json                           # raw | string | json | sparkplug_b
      mqtt_value_path: "$.temperature"                    # JSONPath for value extraction
      mqtt_timestamp_path: "$.ts"                         # optional: extract device timestamp
      mqtt_qos: 1                                         # per-tag QoS override
      data_type: float64
    - id: humidity-1
      name: Humidity
      topic_suffix: humidity
      mqtt_source_topic: "sensors/floor2/humidity/#"      # wildcards supported
      mqtt_payload_format: raw                            # raw bytes → float
      data_type: float64
```

**Per-tag subscription** (not per-device wildcard) because:
1. Each tag may decode differently (`json` vs `raw` vs `sparkplug_b`)
2. Topics may not share a common prefix
3. QoS can vary per tag
4. Granular unsubscribe when tags are removed

**Wildcard support**: Tags can use `+` and `#` wildcards in `mqtt_source_topic`. The adapter matches incoming messages to tags by comparing the message topic against the subscribed pattern.

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
┌───────────────────┐
│ Decode Payload    │
│                   │
│ raw:         bytes → Go type via data_type (like Modbus parseValue)
│ string:      UTF-8 string → strconv.ParseFloat / ParseBool / etc.
│ json:        JSON unmarshal → JSONPath extract value + optional timestamp
│ sparkplug_b: Protobuf decode → extract metric by name
└───────┬───────────┘
        │
        ▼
┌───────────────────┐
│ Build DataPoint   │
│                   │
│ DeviceID, TagID, Value, Quality=good
│ DeviceTimestamp = extracted or message timestamp
│ GatewayTimestamp = now
│ Topic = uns_prefix + "/" + topic_suffix
└───────┬───────────┘
        │
        ▼
┌───────────────────┐
│ Deliver           │
│                   │
│ 1. Update last-value cache (for ReadTags)
│ 2. Call dataHandler callback → MQTT publisher
│ 3. Update staleness timer
│ 4. Update metrics (messages received, decode errors)
└───────────────────┘
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
MQTTBrokerURL        string        // Source broker URL (tcp:// or ssl://)
MQTTUsername          string        // Broker authentication
MQTTPassword          string
MQTTClientIDPrefix    string        // Client ID prefix (default: "gw-source")
MQTTQOS               byte          // Default QoS for subscriptions (0, 1, 2)
MQTTCleanSession      bool          // Clean session on connect
MQTTStalenessTimeout  time.Duration // Mark device stale after no messages (0 = disabled)
MQTTTLSEnabled        bool          // TLS for source broker
MQTTTLSCAFile         string
MQTTTLSCertFile       string
MQTTTLSKeyFile        string
```

New fields on `Tag` for MQTT-sourced tags:

```go
MQTTSourceTopic   string // Topic to subscribe to (supports wildcards)
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

1. `source_pool.go` + `source_client.go` — broker sharing, subscription lifecycle
2. `decoder.go` — `raw` and `json` formats only (sparkplug_b deferred)
3. Topic loop prevention via prefix guard (layer 2)
4. Staleness detection with `gateway_mqtt_source_device_stale` metric
5. Last-value cache for `ReadTags` compatibility
6. Wire into `main.go`

**Deferred to v2:**
- Sparkplug B decoding (requires protobuf dependency)
- MQTT v5 message properties for loop prevention
- Micro-batching output
- Wildcard topic → multi-tag fan-out
- MQTT source metrics dashboard (Grafana panel)

## Low — Nice to Have

### 15. DataPoint Pool Usage in Production

**Status**: `AcquireDataPoint()`/`ReleaseDataPoint()` exist with a `sync.Pool` in `domain/datapoint.go`. Only used in tests and benchmarks (`testing/unit/domain/datapoint_test.go`, `testing/benchmark/throughput/datapoint_test.go`, `testing/benchmark/concurrency/stress_test.go`). All production code uses `NewDataPoint()`.

**Note**: The polling service already uses a **slice pool** for `[]*DataPoint` (`polling.go:45-51`), and S7 uses a **buffer pool** for byte buffers (`s7/types.go:122-175`). Both are actively used in production. The element-level DataPoint pool is deliberately avoided for safety — only promote it after profiling shows GC pressure at high device counts.

---

### ~~16. `reorderBytes` Allocation Optimization~~ ✅ DONE

**Resolved**: Rewrote `reorderBytes()` in `internal/adapter/modbus/conversion.go` to work entirely in-place with zero allocations. BigEndian is a no-op; LittleEndian does a full byte reverse; MidBigEndian swaps adjacent bytes; MidLitEndian swaps 2-byte halves of each 4-byte group. Eliminated the `make([]byte, len(data))` allocation from the hot path.

---

### 17. OPC UA Event & Alarm Support

**Status**: Not implemented. Full OPC UA Alarms & Conditions (A&C) is a large subsystem:
- Event subscriptions (not just data changes)
- Alarm acknowledgment flow
- Historical Data Access (HDA)

Consider as a separate project phase.

---

### 18. Clock Drift / NTP Sync Awareness

**Status**: Not implemented. DataPoint already tracks `DeviceTimestamp`, `GatewayTimestamp`, and `StalenessMs` — but there's no NTP sync check, clock drift estimation, or freshness window enforcement.

---

### ~~19. S7 Per-Device/Tag Prometheus Metrics~~ ✅ DONE

**Resolved**: Added five S7-specific metrics to `metrics.Registry`: `gateway_s7_device_connected` (gauge, 1/0 per device), `gateway_s7_tag_errors_total` (counter per device+tag), `gateway_s7_read_duration_seconds` (histogram per device), `gateway_s7_write_duration_seconds` (histogram per device), `gateway_s7_breaker_state` (gauge, 0=closed/1=half-open/2=open per device). Connection state and breaker state are published by the existing metrics loop (`publishActiveConnectionMetrics`). Read/write durations and tag errors are recorded at the pool level in `ReadTag`, `ReadTags`, `WriteTag`, and `WriteTags`.

---

### 20. S7 Security Documentation

**Status**: S7 protocol has no native authentication (unlike OPC UA). Production deployments need:
- Risk documentation for auth-less access
- Config validation warnings in production mode
- S7comm+ password support for S7-1500 PLCs
- Network segmentation recommendations

---

## Not an Issue — Investigated and Closed

### Modbus Thread Safety (opMu Mutex)

**Investigation**: The `opMu sync.Mutex` in `modbus/client.go` serializes all Modbus operations because `goburrow/modbus` is not thread-safe.

**Finding**: **Not a bottleneck.** Each device gets its own client with its own `opMu`. The polling architecture uses one goroutine per device, so there's no contention — the mutex only fires if a health check or API write happens concurrently with a poll, which is rare and brief. The batch optimization (`buildContiguousRanges`) already minimizes the number of lock acquisitions per poll cycle.

**Verdict**: Defensive code, correctly applied. No action needed.
