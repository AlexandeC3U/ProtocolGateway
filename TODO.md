# TODO - Connector Gateway

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
