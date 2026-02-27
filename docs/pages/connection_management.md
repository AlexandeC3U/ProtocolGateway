- [7. Connection Management](#7-connection-management)
  - [7.1 Connection Pooling Strategies](#71-connection-pooling-strategies)
  - [7.2 Idle Connection Management](#72-idle-connection-management)

## 7. Connection Management

### 7.1 Connection Pooling Strategies

Each protocol requires a different connection pooling strategy based on its characteristics. This comparison diagram explains the rationale behind per-device pooling (Modbus, S7) versus per-endpoint session sharing (OPC UA), along with configuration parameters and trade-offs. Understanding these differences is essential for capacity planning and troubleshooting:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                    CONNECTION POOLING STRATEGIES COMPARISON                    │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    MODBUS: Per-Device Pooling                           │   │
│  │                                                                         │   │
│  │  Rationale:                                                             │   │
│  │  • Modbus TCP maintains stateless connections                           │   │
│  │  • Each device has unique IP:Port combination                           │   │
│  │  • Slave ID is per-request, not per-connection                          │   │
│  │  • Simple 1:1 mapping simplifies health tracking                        │   │
│  │                                                                         │   │
│  │  Configuration:                                                         │   │
│  │  • Max connections: 100 (configurable)                                  │   │
│  │  • Idle timeout: 5 minutes                                              │   │
│  │  • Health check: 30 seconds                                             │   │
│  │                                                                         │   │
│  │  Trade-offs:                                                            │   │
│  │  + Simple failure isolation                                             │   │
│  │  + Easy to debug connection issues                                      │   │
│  │  - Higher memory for many devices                                       │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                 OPC UA: Per-Endpoint Session Sharing                    │   │
│  │                                                                         │   │
│  │  Rationale:                                                             │   │
│  │  • OPC UA sessions are heavyweight (security context, subscriptions)    │   │
│  │  • Servers like Kepware limit concurrent sessions (50-100)              │   │
│  │  • Multiple devices often share same OPC server                         │   │
│  │  • Subscription management benefits from session sharing                │   │
│  │                                                                         │   │
│  │  Configuration:                                                         │   │
│  │  • Max endpoint sessions: 50 (not devices!)                             │   │
│  │  • Idle timeout: 5 minutes (respects active subscriptions)              │   │
│  │  • Health check: 30 seconds                                             │   │
│  │                                                                         │   │
│  │  Trade-offs:                                                            │   │
│  │  + Scales to 200+ devices on limited servers                            │   │
│  │  + Shared subscription infrastructure                                   │   │
│  │  - More complex failure isolation                                       │   │
│  │  - Endpoint-level failures affect multiple devices                      │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    S7: Per-Device Pooling                               │   │
│  │                                                                         │   │
│  │  Rationale:                                                             │   │
│  │  • S7 connections are established per PLC (Rack/Slot)                   │   │
│  │  • Connection state is maintained (unlike Modbus)                       │   │
│  │  • PDU size negotiated per connection                                   │   │
│  │  • Natural 1:1 mapping to physical PLCs                                 │   │
│  │                                                                         │   │
│  │  Configuration:                                                         │   │
│  │  • Max connections: 100                                                 │   │
│  │  • Idle timeout: 5 minutes                                              │   │
│  │  • Health check: 30 seconds                                             │   │
│  │                                                                         │   │
│  │  Trade-offs:                                                            │   │
│  │  + Matches physical topology                                            │   │
│  │  + Simple health/failure model                                          │   │
│  │  - Connection limits per PLC (typically 8-32)                           │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 7.2 Idle Connection Management

Idle connections consume server-side resources and may become stale. The background reaper goroutine periodically evaluates connections against three criteria: last usage time, active subscriptions (OPC UA), and connection health state. This diagram documents the reaping algorithm that balances resource efficiency with operational continuity:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                      IDLE CONNECTION REAPING                                   │
│                                                                                │
│  Background reaper goroutine runs every IdleTimeout/2 (2.5 minutes default)    │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                         │   │
│  │  for each connection in pool:                                           │   │
│  │                                                                         │   │
│  │    ┌─────────────────────────────────────────────────────────────────┐  │   │
│  │    │  Check 1: Last Used Time                                        │  │   │
│  │    │                                                                 │  │   │
│  │    │  if (now - lastUsed) > IdleTimeout:                             │  │   │
│  │    │      mark for removal                                           │  │   │
│  │    │                                                                 │  │   │
│  │    │  Check 1b: Connection TTL (hard cap)                            │  │   │
│  │    │                                                                 │  │   │
│  │    │  if MaxTTL > 0 && (now - createdAt) > MaxTTL:                   │  │   │
│  │    │      mark for removal (prevents stale long-lived connections)   │  │   │
│  │    └─────────────────────────────────────────────────────────────────┘  │   │
│  │                                                                         │   │
│  │    ┌─────────────────────────────────────────────────────────────────┐  │   │
│  │    │  Check 2: Active Subscriptions (OPC UA only)                    │  │   │
│  │    │                                                                 │  │   │
│  │    │  if hasActiveSubscriptions:                                     │  │   │
│  │    │      skip removal (subscriptions keep session alive)            │  │   │
│  │    │                                                                 │  │   │
│  │    │  if (now - lastPublishTime) > SubscriptionIdleTimeout:          │  │   │
│  │    │      subscriptions may be stale, allow removal                  │  │   │
│  │    └─────────────────────────────────────────────────────────────────┘  │   │
│  │                                                                         │   │
│  │    ┌─────────────────────────────────────────────────────────────────┐  │   │
│  │    │  Check 3: Connection Health                                     │  │   │
│  │    │                                                                 │  │   │
│  │    │  if connection.State == Error:                                  │  │   │
│  │    │      mark for removal (will reconnect on next use)              │  │   │
│  │    └─────────────────────────────────────────────────────────────────┘  │   │
│  │                                                                         │   │
│  │  end for                                                                │   │
│  │                                                                         │   │
│  │  Remove marked connections, close gracefully                            │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Justification:                                                                │
│  • Prevents resource exhaustion from accumulated stale connections             │
│  • Frees server-side resources (important for licensed OPC servers)            │
│  • Allows natural recovery from transient network issues                       │
│  • Respects active work (subscriptions, in-flight operations)                  │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---
