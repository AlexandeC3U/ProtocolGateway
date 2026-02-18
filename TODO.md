# TODO — Connector Gateway Roadmap

Last verified against codebase: **2026-02-18**

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

### 2. Test-Connection Handler is a Stub

**Status**: `internal/api/handlers.go:590` has `// TODO: Actually test the connection with the protocol pool`. Currently only validates config fields and returns success — it never opens a real connection.

**What's needed**:
1. Accept protocol pool reference in the API handler (or via DeviceManager)
2. Attempt a real connection to the device (connect → health check → disconnect)
3. Return actual success/failure with error details (timeout, auth failure, unreachable, etc.)
4. Add a timeout to prevent hanging on unresponsive devices

---

### 3. MQTT Publish Latency Not Measured

**Status**: `internal/adapter/mqtt/publisher.go:385` passes `0` as latency: `p.metrics.RecordMQTTPublish(true, 0) // TODO: measure actual latency`

**What's needed**:
- Capture `time.Now()` before `token.Wait()`, calculate duration after
- Pass real latency to `RecordMQTTPublish()`
- The metrics registry already has a `MQTTPublishLatency` histogram — it's just never fed real data

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

### 5. Device Edit Resets Polling State

**Status**: When a device is updated via the API, `main.go` calls `pollingSvc.UnregisterDevice()` then `pollingSvc.RegisterDevice()`. This resets poll jitter, retry state, circuit breaker state, and cancels any in-progress operations.

**What's needed**:
- Implement `PollingService.ReplaceDevice()` that atomically swaps the device config while preserving the poller's runtime state (next poll time, backoff state, etc.)
- Only re-create the protocol client if connection parameters actually changed
- If only tags changed, update tags without dropping the connection

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

### 7. Modbus Coil/Discrete Input Batching

**Status**: Range-based batching (`buildContiguousRanges`) works for holding/input registers. Coils and discrete inputs fall through to `readTagGroupIndividually()` which reads them one by one (`client.go:426-429`).

**What's needed**:
- Bit-packed contiguous range batching for coils (8 coils per byte in Modbus response)
- Similar `buildContiguousRanges()` algorithm adapted for bit addressing
- Extract individual coil values from the packed byte response

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

### 9. S7 Write Aggregation (Batch Writes)

**Status**: `WriteTags()` in `s7/pool.go:306` loops per tag. gos7 supports `AGWriteMulti` for batched writes.

**What's needed**:
- Group writes by DB number
- Build multi-item PDU with `AGWriteMulti`
- Reduces round trips for bulk write scenarios

---

### 10. Connection TTL (Hard Cap)

**Status**: All pools have `IdleTimeout` (close if unused). None have a max connection lifetime.

**What's needed**:
- `MaxTTL` config for each pool — force periodic reconnection even for active connections
- Helps with PLC firmware that leaks resources over long sessions
- S7 PLCs are especially prone to this

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

### 13. Per-Device Circuit Breaker Configuration

**Status**: All devices within a protocol use the same circuit breaker config. Some PLCs need different thresholds.

**What's needed**:
- Optional `CircuitBreakerConfig` on `Device` — override the pool default
- Fast-fail for critical devices, lenient for legacy PLCs with flaky connectivity

---

## Low — Nice to Have

### 14. DataPoint Pool Usage in Production

**Status**: `AcquireDataPoint()`/`ReleaseDataPoint()` exist with a `sync.Pool` in `domain/datapoint.go`. Only used in tests and benchmarks (`testing/unit/domain/datapoint_test.go`, `testing/benchmark/throughput/datapoint_test.go`, `testing/benchmark/concurrency/stress_test.go`). All production code uses `NewDataPoint()`.

**Note**: The polling service already uses a **slice pool** for `[]*DataPoint` (`polling.go:45-51`), and S7 uses a **buffer pool** for byte buffers (`s7/types.go:122-175`). Both are actively used in production. The element-level DataPoint pool is deliberately avoided for safety — only promote it after profiling shows GC pressure at high device counts.

---

### 15. `reorderBytes` Allocation Optimization

**Status**: Allocates `make([]byte, len(data))` on every call in the hot path. Could reuse buffers via `sync.Pool` or reorder in-place.

**Note**: Only optimize after profiling confirms this is a bottleneck.

---

### 16. OPC UA Event & Alarm Support

**Status**: Not implemented. Full OPC UA Alarms & Conditions (A&C) is a large subsystem:
- Event subscriptions (not just data changes)
- Alarm acknowledgment flow
- Historical Data Access (HDA)

Consider as a separate project phase.

---

### 17. Clock Drift / NTP Sync Awareness

**Status**: Not implemented. DataPoint already tracks `DeviceTimestamp`, `GatewayTimestamp`, and `StalenessMs` — but there's no NTP sync check, clock drift estimation, or freshness window enforcement.

---

### 18. S7 Per-Device/Tag Prometheus Metrics

**Status**: Modbus adapter has per-tag diagnostics (`TagDiagnostic` with success/error counts). S7 has the same structures but they're not exposed as Prometheus metrics.

**What's needed**:
- Gauge vector: `s7_device_connected{device_id}`
- Counter vector: `s7_tag_errors_total{device_id, tag_id}`
- Histogram: `s7_read_duration_seconds{device_id}`

---

### 19. S7 Security Documentation

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
