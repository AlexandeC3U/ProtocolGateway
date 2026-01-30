# TODO - Connector Gateway

## 🔴 CRITICAL Issues (Must Fix Before Production)

### 1. OPC UA: Race Condition in ReadTags/WriteTags
**File:** `internal/adapter/opcua/client.go`  
**Problem:** Missing `opMu` lock in batch read/write operations causes data races.  
**Impact:** Data corruption, incorrect values, crashes under concurrent load.  
**Fix:** Add `c.opMu.Lock()` / `defer c.opMu.Unlock()` at the start of `ReadTags` and `WriteTags`.

### 2. OPC UA: Channel Close Panic in Subscription Cleanup
**File:** `internal/adapter/opcua/subscription.go`  
**Problem:** Closing `notificationCh` can panic if `handleNotifications` goroutine is still sending.  
**Impact:** Crash during subscription cleanup or reconnection.  
**Fix:** Use a done channel pattern:
```go
close(s.done)  // Signal goroutine to stop first
s.wg.Wait()    // Wait for it to exit
close(s.notificationCh)  // Then close channel
```

### 3. OPC UA: Goroutine Leak in handleNotifications
**File:** `internal/adapter/opcua/subscription.go`  
**Problem:** Goroutine never exits when subscription is closed.  
**Impact:** Memory leak, accumulated goroutines over time.  
**Fix:** Add done channel check in the select statement.

### 4. OPC UA: Division by Zero in reverseScaling
**File:** `internal/adapter/opcua/client.go`  
**Problem:** No check for `ScaleFactor == 0` before division.  
**Impact:** Panic in production with misconfigured tags.  
**Fix:** Add guard: `if tag.ScaleFactor == 0 { return value }`

### 5. OPC UA: reconnect() Called Without Lock
**File:** `internal/adapter/opcua/client.go`  
**Problem:** `reconnect()` modifies client state without proper synchronization.  
**Impact:** Data race during concurrent reconnection attempts.

### 6. OPC UA: Missing Subscription Recovery After Reconnect
**File:** `internal/adapter/opcua/subscription.go`  
**Problem:** Subscriptions are not automatically recovered after connection loss.  
**Impact:** Silent data loss - subscribed tags stop updating after reconnect.

### 7. Modbus: Goroutine Leak in Connect()
**File:** `internal/adapter/modbus/client.go`  
**Problem:** On context cancellation, the connection goroutine continues and the handler is leaked.  
**Impact:** Connection leaks, resource exhaustion.  
**Fix:** Close the handler when context is cancelled.

### 8. Modbus: Race Condition on connected Flag
**File:** `internal/adapter/modbus/client.go`  
**Problem:** `connected` flag and `client`/`handler` are not updated atomically.  
**Impact:** Reading from nil client, potential crash.

### 9. Modbus: Index Out of Bounds in reorderBytes()
**File:** `internal/adapter/modbus/client.go`  
**Problem:** For 2-byte data with certain byte orders, array access is out of bounds.  
**Impact:** Panic in production with specific data configurations.

### 10. Modbus: Missing Protocol Limit Validation
**File:** `internal/adapter/modbus/client.go`  
**Problem:** Modbus limits (125 registers, 2000 coils max) not enforced.  
**Impact:** Protocol errors, device communication failures.

### 11. S7: Connection Leak on Context Cancellation
**File:** `internal/adapter/s7/client.go`  
**Problem:** Same pattern as Modbus - handler leaked on timeout.  
**Impact:** Connection resource exhaustion.

### 12. S7: Map Modification During Iteration in Close()
**File:** `internal/adapter/s7/pool.go`  
**Problem:** Deleting from map while iterating is undefined behavior.  
**Impact:** Application crash during shutdown, missed cleanups.

### 13. MQTT: Unbounded topicStats Map Growth
**File:** `internal/adapter/mqtt/publisher.go`  
**Problem:** `topicStats` map never evicts entries.  
**Impact:** Memory leak leading to OOM if topics are dynamic.

### 14. API: Missing Authentication
**File:** `internal/api/handlers.go`  
**Problem:** No authentication on API endpoints.  
**Impact:** Unauthorized configuration changes, credential theft.

### 15. Config: Credentials Stored with World-Readable Permissions
**File:** `internal/adapter/config/devices.go`  
**Problem:** Files saved with `0644` permissions.  
**Impact:** Local users can read device credentials.

---

## 🟠 IMPORTANT Issues (Should Fix Soon)

### OPC UA Adapter
1. **Node cache unbounded growth** - Memory leak over time
2. **Error wrapping inconsistency** - Breaks `errors.Is()` checks
3. **No StatusChangeNotification handling** - Missed disconnect events
4. **No subscription reconnection logic** - Manual intervention needed
5. **Unused getSecurityMode()** - Dead code
6. **Stats overflow risk** - Uint64 will wrap
7. **Reconnect ignores context properly** - Hangs possible
8. **Byte order transformation issues** - Data corruption risk
9. **No timeout on opMu.Lock()** - Potential deadlock
10. **TOCTOU race in capacity check** - Concurrency bug
11. **Circuit breaker bypass possible** - Error handling gap

### Modbus Adapter
1. **Background goroutines don't stop on Close()** - 30s shutdown delay
2. **Stats overflow** - Same as OPC UA
3. **Deadlock risk in pool lock ordering** - Complex locking
4. **Pool Close() map iteration issue** - Same as S7
5. **TOCTOU race on entry access** - Nil pointer risk
6. **No timeout on client creation** - Hangs possible

### S7 Adapter
1. **Data race on lastUsed** - TOCTOU after unlock
2. **Incomplete batch read** - N tags = N round trips
3. **Boolean write destroys adjacent bits** - Silent data corruption
4. **Health check holds lock too long** - Blocks all operations

### MQTT Publisher
1. **Race on connected flag vs client state** - Failed publishes
2. **Buffer re-queue creates infinite loop** - CPU exhaustion
3. **Disconnect() deadlock potential** - Edge case hang
4. **drainBuffer timeout too short** - Message loss
5. **bufferMessage returns success on drop** - Silent data loss

### Domain/Service
1. **ProtocolManager Close() map iteration** - Same issue
2. **Sync.Pool use-after-free risk** - Data corruption
3. **Context leak in CommandHandler constructor** - Goroutine leak
4. **Missing ConnectionConfig validation** - Runtime errors
5. **Duplicate tag IDs not detected** - Silent data loss
6. **SlicePool memory inefficiency** - Waste
7. **processWriteCommand should block, not reject** - Unnecessary errors

### API/Config
1. **Race condition in SaveDevices** - Lost updates
2. **Missing request body size limit** - DoS vulnerability
3. **Overly permissive CORS** - CSRF risk
4. **Error messages leak internals** - Information disclosure
5. **Unbounded limit parameter** - Memory exhaustion
6. **SetCallbacks not thread-safe** - Race condition
7. **Path traversal risk** - Arbitrary file read

---

## 🟡 Minor Issues

### Code Quality
- Regex compilation on every call (S7 parseSymbolicAddress)
- Magic numbers (jitter percentages, timeouts)
- Custom `contains()` reimplements stdlib
- Hardcoded circuit breaker settings
- Inconsistent error wrapping patterns

### Validation Gaps
- Priority bounds not enforced (0-2)
- Quality enum not validated
- DataType enum not exhaustively validated
- Tag bit offset not range-checked

### Documentation
- Missing context timeout documentation
- Callback contract not documented
- Sync.Pool usage requirements undocumented

---

## Recommended Fix Priority

### Phase 1: Immediate (Security & Crashes)
1. Add API authentication
2. Fix file permissions for credentials
3. Fix all division-by-zero issues
4. Fix channel close panics
5. Fix map iteration issues

### Phase 2: Short-term (Data Integrity)
1. Fix race conditions in all adapters
2. Fix goroutine leaks
3. Add Modbus protocol limit validation
4. Fix subscription recovery
5. Fix boolean write bit corruption (S7)

### Phase 3: Medium-term (Reliability)
1. Add request size limits
2. Configure CORS properly
3. Add comprehensive input validation
4. Implement connection reference counting
5. Add graceful degradation patterns

### Phase 4: Long-term (Optimization)
1. Implement actual S7 batch reads
2. Add stats eviction policies
3. Optimize lock contention
4. Add debug mode for pool tracking

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
