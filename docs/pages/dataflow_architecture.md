- [8. Data Flow Architecture](#8-data-flow-architecture)
  - [8.1 Read Path (Polling)](#81-read-path-polling)
    - [Worker Pool Cycling: How 10 Workers Serve 100+ Devices](#worker-pool-cycling-how-10-workers-serve-100-devices)
  - [8.2 Write Path (Commands)](#82-write-path-commands)

## 8. Data Flow Architecture

### 8.1 Read Path (Polling)

This comprehensive flowchart traces a complete polling cycle from timer tick to MQTT publish. Key stages include worker pool acquisition (with back-pressure handling), `ProtocolManager` routing, batch optimization, data transformation (scaling, quality assignment), and metric recording. Understanding this flow is essential for debugging data latency or missing values:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                            POLLING DATA FLOW                                   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                         │   │
│  │  ┌──────────────┐                                                       │   │
│  │  │   Device     │  Poll interval + jitter (0-10%)                       │   │
│  │  │   Ticker     │  ─────────────────────────────────►                   │   │
│  │  └──────────────┘                                                       │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Worker Pool Acquire                           │   │   │
│  │  │                                                                  │   │   │
│  │  │  ┌────────────────────────────────────────────────────────────┐  │   │   │
│  │  │  │  Worker available?                                         │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  YES ─► Proceed to read                                    │  │   │   │
│  │  │  │  NO  ─► Skip this poll cycle (back-pressure)               │  │   │   │
│  │  │  │         Increment skipped counter                          │  │   │   │
│  │  │  │         metrics.polling_polls_skipped_total++              │  │   │   │
│  │  │  └────────────────────────────────────────────────────────────┘  │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Tag Preparation                               │   │   │
│  │  │                                                                  │   │   │
│  │  │  1. Filter enabled tags only                                     │   │   │
│  │  │  2. Build tag lookup map (ID → Tag)                              │   │   │
│  │  │  3. Create context with device timeout                           │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Protocol Manager                              │   │   │
│  │  │                                                                  │   │   │
│  │  │  protocolManager.ReadTags(ctx, device, tags)                     │   │   │
│  │  │                                                                  │   │   │
│  │  │  ┌────────────────────────────────────────────────────────────┐  │   │   │
│  │  │  │  1. Lookup pool for device.Protocol                        │  │   │   │
│  │  │  │  2. Delegate to pool.ReadTags()                            │  │   │   │
│  │  │  └────────────────────────────────────────────────────────────┘  │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Protocol Pool (e.g., Modbus)                  │   │   │
│  │  │                                                                  │   │   │
│  │  │  ┌────────────────────────────────────────────────────────────┐  │   │   │
│  │  │  │  1. Check circuit breaker state                            │  │   │   │
│  │  │  │     - Open: return ErrCircuitBreakerOpen immediately       │  │   │   │
│  │  │  │     - Closed/Half-Open: proceed                            │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  2. Get or create connection                               │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  3. Batch optimization                                     │  │   │   │
│  │  │  │     - Group tags by register type                          │  │   │   │
│  │  │  │     - Find contiguous address ranges                       │  │   │   │
│  │  │  │     - Execute minimal number of read operations            │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  4. Execute read with retry                                │  │   │   │
│  │  │  │     - Exponential backoff on failure                       │  │   │   │
│  │  │  │     - Report success/failure to circuit breaker            │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  5. Parse response bytes                                   │  │   │   │
│  │  │  │     - Apply byte order conversion                          │  │   │   │
│  │  │  │     - Apply scale factor and offset                        │  │   │   │
│  │  │  │     - Create DataPoint with timestamps                     │  │   │   │
│  │  │  └────────────────────────────────────────────────────────────┘  │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Post-Processing                               │   │   │
│  │  │                                                                  │   │   │
│  │  │  For each DataPoint:                                             │   │   │
│  │  │  ┌────────────────────────────────────────────────────────────┐  │   │   │
│  │  │  │  1. Assign MQTT topic                                      │  │   │   │
│  │  │  │     topic = device.UNSPrefix + "/" + tag.TopicSuffix       │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  2. Quality filter                                         │  │   │   │
│  │  │  │     - Skip bad quality points (optional)                   │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  3. Deadband filter (if configured)                        │  │   │   │
│  │  │  │     - Skip if change < deadband threshold                  │  │   │   │
│  │  │  └────────────────────────────────────────────────────────────┘  │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    MQTT Publish                                  │   │   │
│  │  │                                                                  │   │   │
│  │  │  mqttPublisher.PublishBatch(dataPoints)                          │   │   │
│  │  │                                                                  │   │   │
│  │  │  ┌────────────────────────────────────────────────────────────┐  │   │   │
│  │  │  │  For each point:                                           │  │   │   │
│  │  │  │    - Serialize to JSON                                     │  │   │   │
│  │  │  │    - Publish to topic with configured QoS                  │  │   │   │
│  │  │  │    - Track in topic statistics                             │  │   │   │
│  │  │  │    - Record metrics (latency, count)                       │  │   │   │
│  │  │  └────────────────────────────────────────────────────────────┘  │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Cleanup & Metrics                             │   │   │
│  │  │                                                                  │   │   │
│  │  │  - Release worker back to pool                                   │   │   │
│  │  │  - Return DataPoint slice to slice pool (sync.Pool)              │   │   │
│  │  │    Note: Individual DataPoints use NewDataPoint() (not pooled).  │   │   │
│  │  │    AcquireDataPoint()/ReleaseDataPoint() exist but are reserved  │   │   │
│  │  │    for future hot-path optimization after profiling.             │   │   │
│  │  │  - Update device status (last poll time, error count)            │   │   │
│  │  │  - Record poll duration in histogram                             │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

#### Worker Pool Cycling: How 10 Workers Serve 100+ Devices

A common misconception is that `WorkerCount` limits how many devices can be polled. In reality, workers **cycle through devices** rapidly—each device only holds a worker for the duration of its read operation (typically 10-100ms). This allows a small worker pool to efficiently serve many devices:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                    WORKER POOL CYCLING (10 workers, 100 devices)               │
│                                                                                │
│  Example: 100 devices, 3s poll interval, ~30ms per poll                        │
│                                                                                │
│                        Worker Pool (10 slots)                                  │
│                    ┌───┬───┬───┬───┬───┬───┬───┬───┬───┬───┐                   │
│  t=0ms   Start:    │D1 │D2 │D3 │D4 │D5 │D6 │D7 │D8 │D9 │D10│ ← 10 devices      │
│                    └───┴───┴───┴───┴───┴───┴───┴───┴───┴───┘   start polling   │
│                        │                                                       │
│                        ▼ D1 finishes at 30ms, worker freed                     │
│                    ┌───┬───┬───┬───┬───┬───┬───┬───┬───┬───┐                   │
│  t=30ms            │D11│D2 │D3 │D4 │D5 │D6 │D7 │D8 │D9 │D10│ ← D11 takes       │
│                    └───┴───┴───┴───┴───┴───┴───┴───┴───┴───┘   freed slot      │
│                        │                                                       │
│                        ▼ D2, D3 finish                                         │
│                    ┌───┬───┬───┬───┬───┬───┬───┬───┬───┬───┐                   │
│  t=35ms            │D11│D12│D13│D4 │D5 │D6 │D7 │D8 │D9 │D10│                   │
│                    └───┴───┴───┴───┴───┴───┴───┴───┴───┴───┘                   │
│                        │                                                       │
│                        ▼ ... workers keep cycling through devices ...          │
│                        │                                                       │
│                    ┌───┬───┬───┬───┬───┬───┬───┬───┬───┬───┐                   │
│  t=280ms           │D95│D96│D97│D98│D99│D100   │   │   │   │ ← Final devices   │
│                    └───┴───┴───┴───┴───┴───┴───┴───┴───┴───┘   completing      │
│                        │                                                       │
│                        ▼ All 100 devices polled!                               │
│                    ┌───┬───┬───┬───┬───┬───┬───┬───┬───┬───┐                   │
│  t=300ms-3000ms    │   │   │   │   │   │   │   │   │   │   │ ← Workers idle    │
│                    └───┴───┴───┴───┴───┴───┴───┴───┴───┴───┘   until next      │
│                                                                  poll cycle    │
│                                                                                │
│  ─────────────────────────────────────────────────────────────────────────     │
│  CAPACITY CALCULATION                                                          │
│                                                                                │
│    Total work per cycle:    100 devices × 30ms = 3,000ms                       │
│    Worker capacity:         10 workers × 3,000ms interval = 30,000ms           │
│    Utilization:             3,000 / 30,000 = 10% ← Plenty of headroom!         │
│                                                                                │
│    Formula: Required workers ≥ (devices × avg_poll_time) / poll_interval       │
│                                                                                │
│  ─────────────────────────────────────────────────────────────────────────     │
│  WHEN BACK-PRESSURE KICKS IN                                                   │
│                                                                                │
│    If all workers are busy when a device's poll timer fires:                   │
│    → Poll is SKIPPED (not queued indefinitely)                                 │
│    → Metric incremented: polling_polls_skipped_total                           │
│    → Device tries again at next interval                                       │
│                                                                                │
│  ─────────────────────────────────────────────────────────────────────────     │
│  WORKER COUNT GUIDELINES                                                       │
│                                                                                │
│    < 50 devices, fast polls:        10 workers (default)                       │
│    100-500 devices:                 20-50 workers                              │
│    500+ devices or slow polls:      50-100 workers                             │
│    Very slow devices (1s+ polls):   devices / 10 or more                       │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 8.2 Write Path (Commands)

Bidirectional communication enables write operations from IT systems to industrial devices. This flowchart shows command processing from MQTT message receipt through validation, queue management (with back-pressure), rate-limited execution, and response publication via `ProtocolManager.WriteTag()`. The queue-based architecture prevents write storms from overwhelming devices:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                            COMMAND WRITE FLOW                                  │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                         │   │
│  │  MQTT Message Received                                                  │   │
│  │  Topic: $nexus/cmd/{device_id}/write                                    │   │
│  │  Payload: { "tag_id": "setpoint", "value": 75.0, "request_id": "..." }  │   │
│  │                                                                         │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Topic Parsing                                 │   │   │
│  │  │                                                                  │   │   │
│  │  │  Extract: device_id = "plc-001"                                  │   │   │
│  │  │  Parse JSON payload                                              │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Validation                                    │   │   │
│  │  │                                                                  │   │   │
│  │  │  ┌────────────────────────────────────────────────────────────┐  │   │   │
│  │  │  │  1. Device exists? (O(1) lookup)                           │  │   │   │
│  │  │  │     NO ─► Publish error response                           │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  2. Tag exists on device? (O(1) lookup)                    │  │   │   │
│  │  │  │     NO ─► Publish error response                           │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  3. Tag writable? (AccessMode check)                       │  │   │   │
│  │  │  │     NO ─► Publish error response                           │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  4. Value type compatible?                                 │  │   │   │
│  │  │  │     NO ─► Publish error response                           │  │   │   │
│  │  │  └────────────────────────────────────────────────────────────┘  │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Queue Enqueue                                 │   │   │
│  │  │                                                                  │   │   │
│  │  │  ┌────────────────────────────────────────────────────────────┐  │   │   │
│  │  │  │  Queue full? (capacity: 1000)                              │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  YES ─► Reject command                                     │  │   │   │
│  │  │  │         Publish error: "command_queue_full"                │  │   │   │
│  │  │  │         metrics.commands_rejected++                        │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  NO  ─► Enqueue command                                    │  │   │   │
│  │  │  │         Non-blocking send to channel                       │  │   │   │
│  │  │  └────────────────────────────────────────────────────────────┘  │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Queue Processor (goroutine)                   │   │   │
│  │  │                                                                  │   │   │
│  │  │  ┌────────────────────────────────────────────────────────────┐  │   │   │
│  │  │  │  1. Acquire semaphore (max 50 concurrent writes)           │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  2. Apply reverse scaling                                  │  │   │   │
│  │  │  │     raw_value = (value - offset) / scale_factor            │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  3. Execute write via ProtocolManager                      │  │   │   │
│  │  │  │     protocolManager.WriteTag(ctx, device, tag, raw_value)  │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  4. Handle result                                          │  │   │   │
│  │  │  │     Success ─► Publish success response                    │  │   │   │
│  │  │  │     Failure ─► Publish error response                      │  │   │   │
│  │  │  │                                                            │  │   │   │
│  │  │  │  5. Release semaphore                                      │  │   │   │
│  │  │  └────────────────────────────────────────────────────────────┘  │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │         │                                                               │   │
│  │         ▼                                                               │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │                    Response Message                              │   │   │
│  │  │                                                                  │   │   │
│  │  │  Topic: $nexus/cmd/response/{device_id}/{tag_id}                 │   │   │
│  │  │  Payload:                                                        │   │   │
│  │  │  {                                                               │   │   │
│  │  │    "request_id": "...",                                          │   │   │
│  │  │    "status": "success" | "error",                                │   │   │
│  │  │    "error": "error message if failed",                           │   │   │
│  │  │    "duration_ms": 45                                             │   │   │
│  │  │  }                                                               │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---