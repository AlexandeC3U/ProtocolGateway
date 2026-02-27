- [14. Testing Strategy](#14-testing-strategy)
  - [14.1 Test Architecture](#141-test-architecture)
  - [14.2 Simulator Infrastructure](#142-simulator-infrastructure)

## 14. Testing Strategy

### 14.1 Test Architecture

The testing strategy follows the test pyramid with unit tests at the base, integration tests with protocol simulators in the middle, and end-to-end Docker Compose tests at the top. The diagram shows test commands, mock patterns for the `ProtocolPool` interface, and benchmark test structure for performance validation:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                           TESTING ARCHITECTURE                                 │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                      TEST PYRAMID                                       │   │
│  │                                                                         │   │
│  │                      ┌───────────┐                                      │   │
│  │                      │   E2E     │  ← Docker Compose integration        │   │
│  │                      │   Tests   │    (manual/CI)                       │   │
│  │                    ┌─┴───────────┴─┐                                    │   │
│  │                    │  Integration  │  ← Protocol simulators             │   │
│  │                    │    Tests      │    (go test -tags=integration)     │   │
│  │                  ┌─┴───────────────┴─┐                                  │   │
│  │                  │    Unit Tests     │  ← Domain logic, mocks           │   │
│  │                  │                   │    (go test ./...)               │   │
│  │                  └───────────────────┘                                  │   │
│  │                                                                         │   │
│  │  Test Commands:                                                         │   │
│  │  • make test          → Run unit tests with race detector               │   │
│  │  • make test-cover    → Generate coverage report                        │   │
│  │  • make test-integration → Run with simulators                          │   │
│  │  • make bench         → Run benchmarks                                  │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    UNIT TEST PATTERNS                                   │   │
│  │                                                                         │   │
│  │  Domain Layer Tests (internal/domain/*_test.go):                        │   │
│  │  • Device validation (required fields, intervals, protocols)            │   │
│  │  • Tag validation (protocol-specific fields, register counts)           │   │
│  │  • DataPoint operations (timestamps, quality, pooling)                  │   │
│  │  • Error handling (sentinel errors, error wrapping)                     │   │
│  │                                                                         │   │
│  │  Mock Patterns:                                                         │   │
│  │  • MockProtocolPool implements ProtocolPool interface                   │   │
│  │  • Records all calls for verification                                   │   │
│  │  • Configurable return values and errors                                │   │
│  │  • Thread-safe for concurrent test execution                            │   │
│  │                                                                         │   │
│  │  Example Test Structure:                                                │   │
│  │  func TestDevice_Validate(t *testing.T) {                               │   │
│  │      tests := []struct {                                                │   │
│  │          name    string                                                 │   │
│  │          device  Device                                                 │   │
│  │          wantErr error                                                  │   │
│  │      }{                                                                 │   │
│  │          {"valid device", validDevice(), nil},                          │   │
│  │          {"missing ID", deviceWithoutID(), ErrDeviceIDRequired},        │   │
│  │          ...                                                            │   │
│  │      }                                                                  │   │
│  │      for _, tt := range tests {                                         │   │
│  │          t.Run(tt.name, func(t *testing.T) {                            │   │
│  │              err := tt.device.Validate()                                │   │
│  │              if !errors.Is(err, tt.wantErr) { ... }                     │   │
│  │          })                                                             │   │
│  │      }                                                                  │   │
│  │  }                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    BENCHMARK TESTS                                      │   │
│  │                                                                         │   │
│  │  DataPoint Benchmarks (internal/domain/datapoint_bench_test.go):        │   │
│  │                                                                         │   │
│  │  BenchmarkDataPoint_ToMQTTPayload       → Compact format conversion     │   │
│  │  BenchmarkDataPoint_ToMQTTPayload_JSONMarshal → JSON marshal comparison │   │
│  │  BenchmarkDataPoint_ToJSON              → Full JSON serialization       │   │
│  │                                                                         │   │
│  │  Additional benchmarks (testing/benchmark/):                            │   │
│  │  • throughput/datapoint_test.go: Creation, Pool, Batch, Parallel        │   │
│  │  • throughput/protocol_read_throughput_test.go: Per-protocol reads      │   │
│  │  • throughput/mqtt_publish_throughput_test.go: MQTT serialization       │   │
│  │  • memory/datapoint_alloc_test.go: Alloc patterns, JSON, batches        │   │
│  │  • concurrency/stress_test.go: Channel, mutex, RWMutex contention       │   │
│  │  • latency/read_latency_test.go: Read latency profiles                  │   │
│  │                                                                         │   │
│  │  Expected Results:                                                      │   │
│  │  • Pool allocation: ~50% fewer allocations than New                     │   │
│  │  • MQTTPayload: ~3x faster than full JSON                               │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 14.2 Simulator Infrastructure

Protocol simulators enable testing without physical industrial devices. The diagram documents the OPC UA simulator (Python asyncua with demo nodes), Modbus simulator (oitc/modbus-server), and EMQX broker configuration. These simulators provide realistic test scenarios including value simulation, write testing, and error injection:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                         PROTOCOL SIMULATORS                                    │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    OPC UA SIMULATOR                                     │   │
│  │                                                                         │   │
│  │  Location: tools/opcua-simulator/                                       │   │
│  │  Technology: Python + asyncua library                                   │   │
│  │  Port: 4840                                                             │   │
│  │                                                                         │   │
│  │  Exposed Nodes (ns=2):                                                  │   │
│  │  • Demo.Temperature (Double) → 20 ± 5°C sinusoidal                      │   │
│  │  • Demo.Pressure (Double) → 1.2 ± 0.2 bar sinusoidal                    │   │
│  │  • Demo.Status (String) → "OK" / "WARN" alternating                     │   │
│  │  • Demo.Switch (Boolean) → Writable                                     │   │
│  │  • Demo.WriteTest (Boolean) → Write compatibility testing               │   │
│  │                                                                         │   │
│  │  Configuration (Environment Variables):                                 │   │
│  │  • OPCUA_HOST: Bind address (default: 0.0.0.0)                          │   │
│  │  • OPCUA_PORT: Listen port (default: 4840)                              │   │
│  │  • OPCUA_UPDATE_MS: Value update interval (default: 500)                │   │
│  │  • OPCUA_AUTO_UPDATE: Enable value simulation (default: 1)              │   │
│  │                                                                         │   │
│  │  Node ID Format: ns=2;s=Demo.Temperature                                │   │
│  │  Explicit VariantTypes prevent type mismatch errors                     │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    MODBUS SIMULATOR                                     │   │
│  │                                                                         │   │
│  │  Image: oitc/modbus-server:latest                                       │   │
│  │  Port: 5020                                                             │   │
│  │                                                                         │   │
│  │  Features:                                                              │   │
│  │  • Holding registers: 0-999                                             │   │
│  │  • Input registers: 0-999                                               │   │
│  │  • Coils: 0-999                                                         │   │
│  │  • Discrete inputs: 0-999                                               │   │
│  │  • Slave ID: 1 (configurable)                                           │   │
│  │                                                                         │   │
│  │  Use for: Testing Modbus TCP connectivity, register reading/writing     │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    MQTT BROKER (Test Stack)                             │   │
│  │                                                                         │   │
│  │  Test stack (docker-compose.test.yaml):                                 │   │
│  │  Image: eclipse-mosquitto:2  (lightweight, no dashboard)                │   │
│  │  Ports: 1883 (MQTT TCP), 9001 (WebSocket)                               │   │
│  │  Config: testing/fixtures/mosquitto.conf                                │   │
│  │                                                                         │   │
│  │  Dev stack (docker-compose.yaml):                                       │   │
│  │  Image: emqx/emqx:5.5  (full-featured, with dashboard)                  │   │
│  │  Ports: 1883, 8083, 8084, 8883, 18083                                   │   │
│  │  Dashboard: http://localhost:18083 (admin/public)                       │   │
│  │                                                                         │   │
│  │  Note: Integration tests use Mosquitto (test stack) while the dev       │   │
│  │  environment uses EMQX with its dashboard and rule engine.              │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---