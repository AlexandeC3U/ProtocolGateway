# TODO - Connector Gateway

This document tracks bugs, improvements, and feature work for the Protocol Gateway.  
Items are organized by **Phase** (priority) and **Component**.

---

## Phase 0: Critical (Security & Crash Prevention)

> **Must fix before any production deployment.**

| # | Component | Issue | File | Status |
|---|-----------|-------|------|--------|
| 1 | API | Missing authentication on endpoints | `internal/api/handlers.go` | ✅ |
| 2 | Config | Credentials saved with 0644 permissions | `internal/adapter/config/devices.go` | ✅ |
| 3 | OPC UA | Division by zero in `reverseScaling` | `internal/adapter/opcua/conversion.go` | ✅ |
| 4 | S7 | Map delete during iteration in `Close()` | `internal/adapter/s7/pool.go` | ✅ |
| 5 | API | No request body size limit (DoS) | `internal/api/handlers.go` | ✅ |

---

## Phase 1: High Priority (Data Integrity & Stability)

> **Race conditions, goroutine leaks, and silent data corruption.**

| # | Component | Issue | File | Status |
|---|-----------|-------|------|--------|
| 1 | OPC UA | Race condition in `ReadTags`/`WriteTags` (missing `opMu`) | `internal/adapter/opcua/client.go` | ✅ |
| 2 | OPC UA | Channel close panic in subscription cleanup | `internal/adapter/opcua/subscription.go` | ✅ |
| 3 | OPC UA | Goroutine leak in `handleNotifications` | `internal/adapter/opcua/subscription.go` | ✅ |
| 4 | OPC UA | `reconnect()` modifies state without lock | `internal/adapter/opcua/client.go` | ✅ |
| 5 | OPC UA | Subscriptions not recovered after reconnect | `internal/adapter/opcua/subscription.go` | ⬜ |
| 6 | Modbus | Goroutine leak in `Connect()` on cancellation | `internal/adapter/modbus/client.go` | ✅ |
| 7 | Modbus | Race on `connected` flag vs client state | `internal/adapter/modbus/client.go` | ✅ |
| 8 | Modbus | Index OOB in `reorderBytes()` for 2-byte data | `internal/adapter/modbus/conversion.go` | ✅ |
| 9 | Modbus | Protocol limits not validated (125 regs max) | `internal/adapter/modbus/client.go` | ✅ |
| 10 | S7 | Connection leak on context cancellation | `internal/adapter/s7/client.go` | ✅ |
| 11 | S7 | Boolean write destroys adjacent bits | `internal/adapter/s7/client.go` | ✅ |
| 12 | MQTT | Unbounded `topicStats` map growth | `internal/adapter/mqtt/publisher.go` | ✅ |

---

## Phase 2: Medium Priority (Reliability & Hardening)

> **Defensive improvements and edge-case handling.**

| # | Component | Issue | File | Status |
|---|-----------|-------|------|--------|
| 1 | API | Overly permissive CORS (CSRF risk) | `internal/api/handlers.go` | ⬜ |
| 2 | API | Error messages leak internal paths | `internal/api/handlers.go` | ⬜ |
| 3 | Config | `SetCallbacks` not thread-safe | `internal/api/handlers.go` | ⬜ |
| 4 | OPC UA | Node cache unbounded growth | `internal/adapter/opcua/client.go` | ✅ |
| 5 | OPC UA | No StatusChangeNotification handling | `internal/adapter/opcua/subscription.go` | ⬜ |
| 6 | OPC UA | Stats counters may overflow (uint64 wrap) | `internal/adapter/opcua/client.go` | 🟡 |
| 7 | Modbus | Background goroutines ignore `Close()` | `internal/adapter/modbus/client.go` | ✅ |
| 8 | Modbus | Pool `Close()` has same map iteration bug | `internal/adapter/modbus/pool.go` | ✅ |
| 9 | S7 | Health check holds lock too long | `internal/adapter/s7/pool.go` | ✅ |
| 10 | MQTT | Buffer re-queue can infinite loop | `internal/adapter/mqtt/publisher.go` | ✅ |
| 11 | MQTT | `drainBuffer` timeout too short | `internal/adapter/mqtt/publisher.go` | ✅ |
| 12 | Domain | `sync.Pool` use-after-free risk | `internal/domain/datapoint.go` | 🟡 |

**Notes:**
- 🟡 #6: uint64 overflow takes ~584 years at 1M ops/sec - accepted risk with documentation
- 🟡 #12: Documented with safety warnings; NewDataPoint() used by default for safety

---

## Phase 3: Low Priority (Code Quality & Optimization)

> **Non-urgent improvements for maintainability and performance.**

| # | Component | Issue | Status |
|---|-----------|-------|--------|
| 1 | S7 | Regex compiled on every `parseSymbolicAddress` call | ⬜ |
| 2 | All | Magic numbers for timeouts/jitter percentages | ⬜ |
| 3 | All | Inconsistent error wrapping patterns | ⬜ |
| 4 | Domain | Priority bounds (0-2) not enforced | ⬜ |
| 5 | Domain | Quality/DataType enums not exhaustively validated | ⬜ |
| 6 | OPC UA | Unused `getSecurityMode()` function (dead code) | ⬜ |
| 7 | S7 | Batch reads not implemented (N tags = N round trips) | ✅ |

---

## Architecture Improvements (Not Yet Implemented)

### 1. Backpressure Propagation Across Layers
**Priority**: High  
**Complexity**: High  

Currently backpressure is handled locally in each component:
- Worker pool in polling service
- Queue in command handler  
- Circuit breaker in connection pool

**What's needed**: Cross-layer signaling where:
```
Modbus breaker opens →
  Polling slows for that device →
    MQTT publishing rate drops →
      Health endpoint degrades status →
        Orchestrator reassigns workload
```

**Implementation ideas**:
- Event bus for component coordination
- Backpressure signals propagated via context
- Adaptive rate limiting based on downstream health

---

### 2. Separate Worker Pools Per Priority/QoS Tier
**Priority**: High  
**Complexity**: Medium  

Currently all tags and commands are treated equally. Real platforms split:
- **Control plane**: writes, alarms, safety (Priority 2)
- **Data plane**: telemetry, metrics, logs (Priority 0-1)

**What's needed**:
- Separate goroutine pools per priority tier
- Separate circuit breaker rules per tier
- Separate MQTT QoS levels / topic prefixes
- Ensures telemetry flood doesn't block emergency stop writes

**Foundation already in place**:
- `Tag.Priority` field added
- `DataPoint.Priority` field added

---

### 3. Shadow State (Desired vs Actual Configuration)
**Priority**: Medium  
**Complexity**: High  

For fleet management and regulated environments:
```
Device A
 ├─ Config v17 (desired)
 ├─ Active v16 (running)
 └─ Pending v18 (failed validation)
```

**What's needed**:
- State machine for config transitions
- Persistence layer for config history
- Automatic rollback on failures
- API for config diff / promotion

**Foundation already in place**:
- `Device.ConfigVersion` 
- `Device.ActiveConfigVersion`
- `Device.LastKnownGoodVersion`

---

### 4. Clock Drift / NTP Sync Awareness
**Priority**: Low  
**Complexity**: Medium  

Industrial systems care about:
- Clock drift between PLC and gateway
- NTP sync state
- "Data freshness" windows

**What's needed**:
- NTP sync status in health endpoint
- Clock drift estimation per device
- Configurable staleness thresholds
- Reject/flag data outside freshness window

---

## Performance Optimizations (Deferred)

### 5. `reorderBytes` Allocation Optimization
**Priority**: Low  
**Complexity**: Low  

Currently allocates on every call (hot path):
```go
result := make([]byte, len(data))
```

**Options**:
- Reuse buffer via `sync.Pool`
- Reorder in-place (carefully)

**Note**: Only optimize after profiling shows this is a bottleneck.

---

### 6. Coil/Discrete Input Batching
**Priority**: Medium  
**Complexity**: Medium  

Range-based batching implemented for holding/input registers but coils still read individually.

**What's needed**:
- Bit-packed batching for coils (8 coils per byte)
- Similar contiguous range algorithm

---

## Completed ✓

### Phase 0 (Feb 2026)
- [x] API authentication middleware with configurable API key
- [x] Request body size limiting (default 1MB)
- [x] CORS middleware with configurable allowed origins
- [x] File permissions fixed to 0600 for credential files
- [x] Division by zero guard in `reverseScaling`
- [x] Map iteration bug fixed in S7 pool `Close()`

### Phase 1 (Feb 2026)
- [x] OPC UA `reconnect()` now uses TryLock to prevent race conditions
- [x] Modbus `Connect()` goroutine leak fixed - handler closed on context cancel
- [x] Modbus `reorderBytes()` OOB fixed - handles empty/single byte data
- [x] Modbus protocol limits validated (125 registers, 2000 coils max)
- [x] S7 `Connect()` goroutine leak fixed - same pattern as Modbus
- [x] S7 boolean write now uses read-modify-write to preserve adjacent bits
- [x] MQTT `topicStats` map bounded to 10k entries with LRU eviction

### Previously Completed
- [x] Per-device circuit breakers (fault isolation) - Modbus & OPC UA
- [x] Tag/DataPoint alignment fix (tagByID map)
- [x] Split read/publish contexts
- [x] `sync.Once` for safe channel closing
- [x] O(1) tag lookup in command handler
- [x] Modbus operation serialization (thread safety)
- [x] OPC UA operation serialization (opMu)
- [x] `isConnectionError` expanded (io.EOF, etc.) - Modbus & OPC UA
- [x] Backoff jitter (prevent thundering herd) - Modbus & OPC UA
- [x] Range-based register batching (N reads → 1-5 reads) - Modbus
- [x] Enhanced time semantics (GatewayTimestamp, PublishTimestamp, LatencyMs)
- [x] QoS Priority field on Tag and DataPoint
- [x] Device config versioning fields
- [x] OPC UA session state machine
- [x] OPC UA server limits awareness (MaxNodesPerRead batching)
- [x] OPC UA subscription infrastructure (foundation)

---

## S7 Specific (Not Yet Implemented)

### 13. Tag Write Aggregation (Batch Writes)
**Priority**: Medium  
**Complexity**: Medium  

`WriteTags` currently loops per tag — consider batching writes when possible:
- S7 supports multi-variable writes to same DB area
- gos7 `AGWriteMulti` can combine multiple writes into single request
- Reduces round trips significantly for bulk writes

**Implementation**:
```go
// Group writes by DB number, then use AGWriteMulti
func (c *Client) WriteTags(ctx context.Context, writes []TagWrite) []error {
    groups := c.groupWritesByDB(writes)
    for db, dbWrites := range groups {
        // Build PDU with multiple items
        client.AGWriteMulti(items...)
    }
}
```

---

### 14. Connection TTL vs. Idle Timeout
**Priority**: Medium  
**Complexity**: Low  

Add a max connection TTL (hard cap) in addition to `IdleTimeout`:
- Prevents long-lived, stale sessions from living forever
- Forces periodic reconnection even for active connections
- Helps with PLC firmware that leaks resources over long sessions

**Config addition**:
```go
type PoolConfig struct {
    IdleTimeout   time.Duration // Current: close if unused
    MaxTTL        time.Duration // New: hard cap on connection lifetime
}
```

---

### 15. Per-Device/Tag Metrics Exposure
**Priority**: Low  
**Complexity**: Medium  

Currently only connection-level metrics are exposed. Add:
- Gauge vector for per-device connection state
- Counter vector for per-tag error rate
- Histogram for per-device read/write latency

**Example metrics**:
```
s7_device_connected{device_id="plc1"} 1
s7_tag_errors_total{device_id="plc1", tag_id="temp"} 42
s7_read_duration_seconds{device_id="plc1"} histogram
```

---

### 16. Security Documentation & Validation
**Priority**: Low  
**Complexity**: Low  

S7 protocol doesn't have native authentication (unlike OPC UA), but:
- Document auth-less access risks for real deployments
- Add config validation warnings for production mode
- Consider S7comm+ password support (S7-1500)
- Network segmentation recommendations

---

### 17. Per-Device Circuit Breaker Configuration
**Priority**: Low  
**Complexity**: Low  

Currently all devices use the same default circuit breaker config. Enable per-device control:
- Some PLCs may need more aggressive failure thresholds
- Legacy PLCs may need longer recovery timeouts
- Fast-fail for critical devices, lenient for non-critical

**Config addition**:
```go
type Device struct {
    Connection ConnectionConfig
    CircuitBreaker *CircuitBreakerConfig // Optional per-device override
}
```

---

## OPC UA Specific (Not Yet Implemented)

### 7. Full Subscription Implementation
**Priority**: Critical  
**Complexity**: High  

Foundation is in place, but full implementation needed:
- `gopcua` subscription API integration
- Monitored item lifecycle (create, modify, delete)
- Automatic resubscribe on reconnect
- Notification queue management
- Backpressure handling for notification storms
- QoS mapping to MQTT

**Files**: `internal/adapter/opcua/subscription.go` (to create)

---

### 8. Browse & Model Awareness
**Priority**: High  
**Complexity**: Medium  

Currently treats OPC UA as flat node reader. Real gateways:
- Browse address space
- Build tag tree dynamically
- Cache NodeClass, DataType, AccessLevel, EngineeringUnits
- Auto-generate tags from server models

**Implementation**:
```go
func (c *Client) Browse(ctx context.Context, nodeID string) ([]*BrowseResult, error)
func (c *Client) GetNodeAttributes(ctx context.Context, nodeID string) (*NodeAttributes, error)
```

---

### 9. Type System Fidelity
**Priority**: Medium  
**Complexity**: Medium  

Currently flattens all values via `v.Value()`. Real systems preserve:
- Array types
- LocalizedText
- ExtensionObjects
- Enums with names
- Structured types

**What's needed**:
- Type-aware variant conversion
- Configurable "preserve types" mode

---

### 10. Certificate Trust Store Management
**Priority**: Medium  
**Complexity**: High  

Currently loads certs but doesn't manage:
- Trust lists
- Rejected certs folder
- Auto-accept (development mode)
- Certificate rotation
- Expiry monitoring

**Required for**: Plant floor deployments, regulated environments

---

### 11. Event & Alarm Support
**Priority**: Medium  
**Complexity**: Very High  

Real OPC UA includes:
- Alarms & Conditions (A&C)
- Events (not just data changes)
- Acknowledgment flow
- Historical access (HDA)

This is a separate subsystem - consider as Phase 2.

---

### 12. Latency Control (Fast/Slow Lanes)
**Priority**: Low  
**Complexity**: Medium  

Support differentiated service levels:
- Priority nodes with faster sampling
- Separate subscriptions per QoS tier
- Publishing interval tuning per tag group

**Foundation in place**: `Tag.Priority` field
