- [9. Resilience Patterns](#9-resilience-patterns)
  - [9.1 Circuit Breaker Pattern](#91-circuit-breaker-pattern)
  - [9.2 Retry with Exponential Backoff](#92-retry-with-exponential-backoff)
  - [9.3 Graceful Degradation](#93-graceful-degradation)
  - [9.4 Gateway Initialization and Startup](#94-gateway-initialization-and-startup)

## 9. Resilience Patterns

### 9.1 Circuit Breaker Pattern

Circuit breakers (via `sony/gobreaker`) prevent cascade failures by temporarily blocking requests to failing services. The state machine diagram shows the three states (Closed, Open, Half-Open) and transition conditions. Per-protocol configuration allows tuning failure thresholds and recovery timeouts based on device characteristics—fast recovery for Modbus, longer timeouts for OPC UA session issues:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                         CIRCUIT BREAKER STATE MACHINE                          │
│                                                                                │
│  Implementation: sony/gobreaker                                                │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                         │   │
│  │                         ┌──────────────┐                                │   │
│  │         Success         │              │         Failure                │   │
│  │     ┌──────────────────►│    CLOSED    │◄──────────────────┐            │   │
│  │     │                   │              │                   │            │   │
│  │     │                   │  (Normal     │                   │            │   │
│  │     │                   │   Operation) │                   │            │   │
│  │     │                   └──────┬───────┘                   │            │   │
│  │     │                          │                           │            │   │
│  │     │                          │ Failure threshold         │            │   │
│  │     │                          │ exceeded (60% of          │            │   │
│  │     │                          │ last 10 requests)         │            │   │
│  │     │                          │                           │            │   │
│  │     │                          ▼                           │            │   │
│  │     │                   ┌──────────────┐                   │            │   │
│  │     │                   │              │                   │            │   │
│  │     │                   │     OPEN     │───────────────────┘            │   │
│  │     │                   │              │  All requests                  │   │
│  │     │                   │  (Requests   │  fail immediately              │   │
│  │     │                   │   blocked)   │  with ErrCircuitBreakerOpen    │   │
│  │     │                   └──────┬───────┘                                │   │
│  │     │                          │                                        │   │
│  │     │                          │ Timeout (60 seconds)                   │   │
│  │     │                          │                                        │   │
│  │     │                          ▼                                        │   │
│  │     │                   ┌──────────────┐                                │   │
│  │     │                   │              │                                │   │
│  │     └───────────────────│  HALF-OPEN   │                                │   │
│  │         Success         │              │                                │   │
│  │         (probe          │  (Allow one  │────────────────────────────    │   │ 
│  │          passed)        │   request)   │  Failure (probe failed)        │   │
│  │                         └──────────────┘                                │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Configuration (per protocol):                                                 │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  MODBUS                                                                 │   │
│  │  • Requests threshold: 10                                               │   │
│  │  • Failure ratio: 0.6 (60%)                                             │   │
│  │  • Timeout: 60 seconds                                                  │   │
│  │  • Per-device breakers                                                  │   │
│  │                                                                         │   │
│  │  OPC UA (Two-tier)                                                      │   │
│  │  • Endpoint breaker:                                                    │   │
│  │    - Requests: 5                                                        │   │
│  │    - Ratio: 0.6                                                         │   │
│  │    - Triggers on: Connection errors, timeouts, TooManySessions          │   │
│  │  • Device breaker:                                                      │   │
│  │    - Consecutive failures: 5                                            │   │
│  │    - Triggers on: BadNodeID, BadUserAccessDenied, BadTypeMismatch       │   │
│  │                                                                         │   │
│  │  S7                                                                     │   │
│  │  • Requests threshold: 5                                                │   │
│  │  • Failure ratio: 0.5 (50%)                                             │   │
│  │  • Timeout: 60 seconds                                                  │   │
│  │  • Per-device breakers                                                  │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  PER-DEVICE CIRCUIT BREAKER OVERRIDES                                   │   │
│  │                                                                         │   │
│  │  Each device can optionally include a CircuitBreakerConfig in its       │   │
│  │  ConnectionConfig with fields:                                          │   │
│  │  • MaxRequests, Interval, Timeout, FailureThreshold, FailureRatio       │   │
│  │                                                                         │   │
│  │  Any zero-value field falls back to the pool default.                   │   │
│  │  Applied in all three pools (Modbus, S7, OPC UA device-level).          │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Justification:                                                                │
│  • Prevents cascade failures when device/network fails                         │
│  • Reduces unnecessary retry storms                                            │
│  • Allows system to recover gracefully                                         │
│  • Per-device isolation prevents one bad device from affecting others          │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 9.2 Retry with Exponential Backoff

Exponential backoff with jitter prevents "thundering herd" scenarios where multiple clients retry simultaneously after a failure. The diagram shows the delay calculation formula, example retry sequences, and per-protocol configuration. Note the longer backoff for OPC UA `ErrOPCUATooManySessions` errors, allowing server session cleanup time:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                      EXPONENTIAL BACKOFF STRATEGY                              │
│                                                                                │
│  Formula: delay = min(baseDelay * 2^attempt * (1 ± jitter), maxDelay)          │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                         │   │
│  │  Example: baseDelay=100ms, maxDelay=10s, jitter=25%                     │   │
│  │                                                                         │   │
│  │  Attempt  │  Base Delay  │  With Jitter (±25%)  │  Actual Wait          │   │
│  │  ─────────┼──────────────┼──────────────────────┼────────────────────   │   │
│  │     1     │    100ms     │    75ms - 125ms      │    ~100ms             │   │
│  │     2     │    200ms     │   150ms - 250ms      │    ~200ms             │   │
│  │     3     │    400ms     │   300ms - 500ms      │    ~400ms             │   │
│  │     4     │    800ms     │   600ms - 1000ms     │    ~800ms             │   │
│  │     5     │   1600ms     │  1200ms - 2000ms     │    ~1.6s              │   │
│  │     6     │   3200ms     │  2400ms - 4000ms     │    ~3.2s              │   │
│  │     7     │   6400ms     │  4800ms - 8000ms     │    ~6.4s              │   │
│  │     8     │  10000ms     │  7500ms - 10000ms    │    10s (capped)       │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Jitter Purpose:                                                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                                                                         │   │
│  │  WITHOUT JITTER (Thundering Herd):                                      │   │
│  │                                                                         │   │
│  │  Time ──►                                                               │   │
│  │  0ms    100ms   200ms   300ms   400ms                                   │   │
│  │   │       │       │       │       │                                     │   │
│  │   ▼       ▼       ▼       ▼       ▼                                     │   │
│  │  All     All     All     All     All    ◄── All retries hit server      │   │
│  │  retry   retry   retry   retry   retry       at exactly same time       │   │
│  │                                                                         │   │
│  │  WITH JITTER (Distributed Load):                                        │   │
│  │                                                                         │   │
│  │  Time ──►                                                               │   │
│  │  0ms                                                                    │   │
│  │   │  75ms  90ms  110ms  125ms                                           │   │
│  │   ▼   ▼     ▼      ▼      ▼                                             │   │
│  │  Retry Retry Retry Retry Retry  ◄── Retries spread out, reducing        │   │
│  │   A     B     C     D     E           peak load on server               │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Implementation per Protocol:                                                  │
│                                                                                │
│  ┌───────────────┬──────────────┬────────────┬─────────────────────────────┐   │
│  │   Protocol    │  Base Delay  │ Max Delay  │  Notes                      │   │
│  ├───────────────┼──────────────┼────────────┼─────────────────────────────┤   │
│  │ Modbus        │    100ms     │    10s     │  Fast retries, TCP stateless│   │
│  │ OPC UA        │    500ms     │    10s     │  Session recovery overhead  │   │
│  │ OPC UA (TMS)  │     60s      │    5min    │  TooManySessions: long wait │   │
│  │ S7            │    500ms     │    10s     │  PLC connection setup time  │   │
│  └───────────────┴──────────────┴────────────┴─────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 9.3 Graceful Degradation

The gateway maintains service availability through progressive degradation rather than complete failure. This hierarchy diagram shows five operational levels, from full operation through device-level, endpoint-level, publish-level, and protocol-level degradation. At each level, the system continues providing value (API access, configuration management) even when data collection is impaired:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                        GRACEFUL DEGRADATION HIERARCHY                          │
│                                                                                │
│  The gateway maintains service availability through progressive degradation:   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  LEVEL 0: FULL OPERATION                                                │   │
│  │                                                                         │   │
│  │  • All devices connected and polling                                    │   │
│  │  • All protocols operational                                            │   │
│  │  • MQTT publishing normally                                             │   │
│  │  • Health status: HEALTHY                                               │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                           │                                                    │
│                           ▼ Single device failure                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  LEVEL 1: DEVICE-LEVEL DEGRADATION                                      │   │
│  │                                                                         │   │
│  │  • Failed device's circuit breaker opens                                │   │
│  │  • Other devices continue normal operation                              │   │
│  │  • DataPoints for failed device marked quality=bad                      │   │
│  │  • Automatic recovery attempts every 60s                                │   │
│  │  • Health status: DEGRADED                                              │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                           │                                                    │
│                           ▼ OPC Server overloaded                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  LEVEL 2: ENDPOINT-LEVEL DEGRADATION (OPC UA)                           │   │
│  │                                                                         │   │
│  │  • Endpoint circuit breaker opens                                       │   │
│  │  • All devices on that endpoint paused                                  │   │
│  │  • Other endpoints continue normally                                    │   │
│  │  • Load shaping enters brownout mode                                    │   │
│  │  • Telemetry paused, control/safety allowed                             │   │
│  │  • Health status: DEGRADED                                              │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                           │                                                    │
│                           ▼ MQTT broker disconnection                          │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  LEVEL 3: PUBLISH-LEVEL DEGRADATION                                     │   │
│  │                                                                         │   │
│  │  • Polling continues (data is fresh)                                    │   │
│  │  • DataPoints buffered in memory (10,000 limit)                         │   │
│  │  • Oldest messages dropped on buffer overflow                           │   │
│  │  • Reconnection attempts with backoff                                   │   │
│  │  • Command writes queued (will timeout)                                 │   │
│  │  • Health status: DEGRADED                                              │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                           │                                                    │
│                           ▼ All protocols failed                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  LEVEL 4: PROTOCOL-LEVEL DEGRADATION                                    │   │
│  │                                                                         │   │
│  │  • All circuit breakers open                                            │   │
│  │  • Polling skipped (no point trying)                                    │   │
│  │  • Health check probes continue                                         │   │
│  │  • API/Web UI remain accessible                                         │   │
│  │  • Configuration changes still possible                                 │   │
│  │  • Health status: UNHEALTHY                                             │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Key Principle: The gateway never crashes due to device/network failures.      │
│  It maintains API availability for diagnostics and configuration.              │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 9.4 Gateway Initialization and Startup

The gateway (`cmd/gateway/main.go`) implements a robust startup sequence with protocol validation, readiness gating (`gatewayReady`), and comprehensive logging. This ensures observability tools receive accurate data only after the system is fully initialized:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                        GATEWAY STARTUP SEQUENCE                                │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  PHASE 1: INITIALIZATION                                                │   │
│  │                                                                         │   │
│  │  1. Load configuration (config.yaml, devices.yaml)                      │   │
│  │  2. Initialize logging with configured level/format                     │   │
│  │  3. Register Prometheus metrics                                         │   │
│  │  4. Initialize protocol pools (Modbus, OPC UA, S7)                      │   │
│  │  5. Initialize MQTT publisher                                           │   │
│  │                                                                         │   │
│  │  gatewayReady: false (metrics endpoint returns 503)                     │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                           │                                                    │
│                           ▼                                                    │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  PHASE 2: DEVICE REGISTRATION (with Protocol Validation)                │   │
│  │                                                                         │   │
│  │  For each device in configuration:                                      │   │
│  │    ├─ Validate device.Protocol against registered protocols             │   │
│  │    │   • modbus-tcp, modbus-rtu → ModbusPool                            │   │
│  │    │   • opcua                  → OPCUAPool                             │   │
│  │    │   • s7                     → S7Pool                                │   │
│  │    │   • <unknown>              → Log warning, skip device              │   │
│  │    │                                                                    │   │
│  │    ├─ If protocol supported:                                            │   │
│  │    │   • Call pool.RegisterDevice(device)                               │   │
│  │    │   • Track in registeredDevices count                               │   │
│  │    │                                                                    │   │
│  │    └─ If protocol not supported:                                        │   │
│  │        • Return ErrProtocolNotSupported                                 │   │
│  │        • Track in failedDevices or unsupportedProtocol count            │   │
│  │                                                                         │   │
│  │  Startup Logging:                                                       │   │
│  │  INFO: "Gateway startup complete: 10 registered, 0 failed, 1 skipped"   │   │
│  │  WARN: "Gateway starting in DEGRADED state" (if failures > 0)           │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                           │                                                    │
│                           ▼                                                    │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  PHASE 3: SERVICE STARTUP                                               │   │
│  │                                                                         │   │
│  │  1. Start HTTP server (API, health probes, metrics)                     │   │
│  │  2. Start polling service for registered devices                        │   │
│  │  3. Start command handler for write operations                          │   │
│  │  4. Set gatewayReady = true                                             │   │
│  │                                                                         │   │
│  │  Metrics endpoint now returns 200 OK with valid data                    │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  READINESS GUARD FOR METRICS:                                                  │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │  GET /metrics                                                            │  │
│  │                                                                          │  │
│  │  if !gatewayReady.Load() {                                               │  │
│  │      return 503 Service Unavailable                                      │  │
│  │      body: "Gateway not ready - initialization in progress"              │  │
│  │  }                                                                       │  │
│  │                                                                          │  │
│  │  // Normal Prometheus handler                                            │  │
│  │  promhttp.Handler().ServeHTTP(w, r)                                      │  │
│  │                                                                          │  │
│  │  PURPOSE: Prevents Prometheus from scraping incomplete/invalid metrics   │  │
│  │  during startup. Avoids false alerts from metrics systems.               │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---