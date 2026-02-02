# Testing Overview - Connector Gateway

This document serves as the master index for all testing resources in the project.  
Our goal is **comprehensive coverage** across unit, integration, benchmark, and fuzz tests.

**Legend:** ✅ Implemented | 🚧 Partial | 📋 Planned

---

## Table of Contents

1. [Test Structure](#test-structure)
2. [Test Categories](#test-categories)
3. [Running Tests](#running-tests)
4. [Coverage Goals](#coverage-goals)
5. [Test Inventory](#test-inventory)

---

## Test Structure

```
testing/
├── info.md                    # This file (master index)
├── README.md                  # Quick start guide
│
├── unit/                      # Unit tests with mocks
│   ├── domain/                # Business entities ✅
│   ├── adapters/              # Protocol adapters
│   │   ├── modbus/            # ✅ conversion_test.go, client_test.go, pool_test.go
│   │   ├── opcua/             # ✅ conversion_test.go, client_test.go, subscription_test.go, pool_test.go, loadshaping_test.go
│   │   ├── s7/                # ✅ client_test.go, pool_test.go
│   │   └── mqtt/              # ✅ publisher_test.go
│   ├── api/                   # HTTP handlers ✅
│   ├── config/                # ✅ config_test.go, devices_test.go
│   ├── health/                # ✅ checker_test.go
│   ├── metrics/               # ✅ registry_test.go
│   └── service/               # ✅ polling_test.go
│
├── integration/               # Real protocol tests (requires hardware/simulators)
│   ├── modbus/                # ✅ connection_test.go
│   ├── opcua/                 # 📋 Planned
│   ├── s7/                    # 📋 Planned
│   └── mqtt/                  # 📋 Planned
│
├── benchmark/                 # Performance tests
│   ├── throughput/            # ✅ datapoint_test.go
│   ├── latency/               # 📋 Planned
│   ├── memory/                # 📋 Planned
│   └── concurrency/           # ✅ stress_test.go
│
├── fuzz/                      # Fuzz testing - 📋 All Planned
│   ├── conversion/
│   ├── parsing/
│   └── protocol/
│
├── e2e/                       # End-to-end - 📋 All Planned
│   ├── scenarios/
│   └── fixtures/
│
├── mocks/                     # Shared mock implementations ✅
│   ├── protocol_client.go
│   ├── mqtt_publisher.go
│   └── health_checker.go
│
├── testutil/                  # Shared test utilities ✅
│   ├── fixtures.go
│   └── helpers.go
│
└── fixtures/                  # Test data ✅
    ├── configs/
    └── mosquitto.conf
```

---

## Test Categories

### 1. Unit Tests (`testing/unit/`)

Isolated tests using mocks. No external dependencies.

| Package | File | Description | Priority | Status |
|---------|------|-------------|----------|--------|
| **domain** | `datapoint_test.go` | DataPoint creation, JSON, pooling | 🔴 High | ✅ |
| **domain** | `device_test.go` | Device validation, status transitions | 🔴 High | ✅ |
| **domain** | `tag_test.go` | Tag validation, scaling | 🔴 High | ✅ |
| **domain** | `errors_test.go` | Error wrapping, Is/As checks | 🟡 Medium | ✅ |
| **domain** | `protocol_test.go` | Protocol enum validation | 🟡 Medium | ✅ |
| **modbus** | `client_test.go` | Connection, read/write ops | 🔴 High | ✅ |
| **modbus** | `conversion_test.go` | Byte order, type conversion | 🔴 High | ✅ |
| **modbus** | `pool_test.go` | Pool lifecycle, health checks | 🔴 High | ✅ |
| **modbus** | `health_test.go` | Health check logic | 🟡 Medium | 📋 |
| **opcua** | `client_test.go` | Connection, browse, read/write | 🔴 High | ✅ |
| **opcua** | `conversion_test.go` | UA variant conversion | 🔴 High | ✅ |
| **opcua** | `subscription_test.go` | Sub lifecycle, notifications | 🔴 High | ✅ |
| **opcua** | `session_test.go` | Session management | 🟡 Medium | 📋 |
| **opcua** | `pool_test.go` | Connection pooling | 🔴 High | ✅ |
| **opcua** | `loadshaping_test.go` | Rate limiting | 🟡 Medium | ✅ |
| **s7** | `client_test.go` | Connection, read/write | 🔴 High | ✅ |
| **s7** | `conversion_test.go` | S7 type conversion | 🔴 High | 📋 |
| **s7** | `pool_test.go` | Pool management | 🔴 High | ✅ |
| **s7** | `health_test.go` | Health checks | 🟡 Medium | 📋 |
| **mqtt** | `publisher_test.go` | Publish, buffer, reconnect | 🔴 High | ✅ |
| **config** | `config_test.go` | YAML parsing, env override | 🔴 High | ✅ |
| **config** | `devices_test.go` | Device config loading | 🔴 High | ✅ |
| **api** | `handlers_test.go` | HTTP endpoints, middleware | 🔴 High | ✅ |
| **api** | `runtime_test.go` | Runtime management | 🟡 Medium | 📋 |
| **service** | `polling_test.go` | Poll scheduler | 🔴 High | ✅ |
| **service** | `command_handler_test.go` | Write commands | 🔴 High | 📋 |
| **health** | `checker_test.go` | Health aggregation | 🟡 Medium | ✅ |
| **metrics** | `registry_test.go` | Prometheus metrics | 🟢 Low | ✅ |

**Summary:** 22/28 implemented (79%)

### 2. Integration Tests (`testing/integration/`)

Tests against real protocols (simulators or hardware).

| Protocol | Test File | Description | Requirements | Status |
|----------|-----------|-------------|--------------|--------|
| **Modbus** | `connection_test.go` | TCP connection lifecycle | Modbus simulator | ✅ |
| **Modbus** | `register_read_test.go` | Read holding/input registers | Modbus simulator | 📋 |
| **Modbus** | `coil_operations_test.go` | Read/write coils | Modbus simulator | 📋 |
| **Modbus** | `error_handling_test.go` | Exception responses | Modbus simulator | 📋 |
| **Modbus** | `reconnection_test.go` | Connection recovery | Modbus simulator | 📋 |
| **OPC UA** | `connection_test.go` | Secure channel, session | OPC UA simulator | 📋 |
| **OPC UA** | `browse_test.go` | Node browsing | OPC UA simulator | 📋 |
| **OPC UA** | `read_write_test.go` | Read/write values | OPC UA simulator | 📋 |
| **OPC UA** | `subscription_test.go` | Data change notifications | OPC UA simulator | 📋 |
| **OPC UA** | `security_test.go` | Auth modes, certificates | OPC UA simulator | 📋 |
| **OPC UA** | `reconnection_test.go` | Session recovery | OPC UA simulator | 📋 |
| **S7** | `connection_test.go` | S7comm connection | S7 simulator | 📋 |
| **S7** | `db_read_write_test.go` | Data block operations | S7 simulator | 📋 |
| **S7** | `symbolic_test.go` | Symbolic addressing | S7 1200+ | 📋 |
| **S7** | `error_handling_test.go` | Error responses | S7 simulator | 📋 |
| **MQTT** | `connection_test.go` | Broker connection | MQTT broker | 📋 |
| **MQTT** | `publish_test.go` | Message publishing | MQTT broker | 📋 |
| **MQTT** | `qos_test.go` | QoS levels | MQTT broker | 📋 |
| **MQTT** | `reconnection_test.go` | Broker reconnection | MQTT broker | 📋 |
| **MQTT** | `buffering_test.go` | Offline buffering | MQTT broker | 📋 |

**Summary:** 1/20 implemented (5%)

### 3. Benchmark Tests (`testing/benchmark/`)

Performance measurement and regression detection.

| Category | Test File | Metrics | Status |
|----------|-----------|---------|--------|
| **Throughput** | `datapoint_test.go` | DataPoints/sec, pool efficiency | ✅ |
| **Throughput** | `mqtt_publish_throughput_test.go` | Messages/sec | 📋 |
| **Throughput** | `protocol_read_throughput_test.go` | Reads/sec per protocol | 📋 |
| **Latency** | `read_latency_test.go` | P50/P95/P99 read times | 📋 |
| **Latency** | `write_latency_test.go` | P50/P95/P99 write times | 📋 |
| **Latency** | `mqtt_latency_test.go` | Publish latency | 📋 |
| **Memory** | `datapoint_alloc_test.go` | Bytes/op, allocs/op | 📋 |
| **Memory** | `pool_efficiency_test.go` | Pool hit rate | 📋 |
| **Memory** | `buffer_growth_test.go` | Buffer memory under load | 📋 |
| **Concurrency** | `stress_test.go` | Parallel ops, race detection | ✅ |
| **Concurrency** | `pool_contention_test.go` | Lock contention | 📋 |
| **Concurrency** | `subscription_stress_test.go` | Many subscriptions | 📋 |

**Summary:** 2/12 implemented (17%) |
| **Concurrency** | `pool_contention_test.go` | Lock contention |
| **Concurrency** | `subscription_stress_test.go` | Many subscriptions |

### 4. Fuzz Tests (`testing/fuzz/`) - 📋 All Planned

Discover edge cases and crashes with random inputs.

| Category | Test File | Target | Status |
|----------|-----------|--------|--------|
| **Conversion** | `modbus_conversion_fuzz_test.go` | Byte order permutations | 📋 |
| **Conversion** | `opcua_variant_fuzz_test.go` | UA Variant conversion | 📋 |
| **Conversion** | `s7_type_fuzz_test.go` | S7 data types | 📋 |
| **Conversion** | `scaling_fuzz_test.go` | Linear/reverse scaling | 📋 |
| **Parsing** | `config_fuzz_test.go` | Config YAML parsing | 📋 |
| **Parsing** | `address_fuzz_test.go` | Address string parsing | 📋 |
| **Parsing** | `nodeid_fuzz_test.go` | OPC UA NodeID parsing | 📋 |
| **Protocol** | `modbus_frame_fuzz_test.go` | Malformed Modbus frames | 📋 |
| **Protocol** | `s7_packet_fuzz_test.go` | Malformed S7 packets | 📋 |

**Summary:** 0/9 implemented (0%)

### 5. End-to-End Tests (`testing/e2e/`) - 📋 All Planned

Complete workflow scenarios.

| Scenario | Description | Status |
|----------|-------------|--------|
| `startup_shutdown_test.go` | Clean startup/shutdown cycle | 📋 |
| `config_reload_test.go` | Hot config reload | 📋 |
| `multi_device_test.go` | Multiple devices, mixed protocols | 📋 |
| `failover_test.go` | Device failure and recovery | 📋 |
| `high_load_test.go` | Sustained high message rate | 📋 |
| `memory_leak_test.go` | Long-running memory stability | 📋 |

**Summary:** 0/6 implemented (0%)

---

## Running Tests

### Quick Commands

```bash
# All unit tests
make test

# With coverage report
make test-cover

# Integration tests (requires simulators)
make test-integration

# Benchmarks
make bench

# Fuzz tests (time-limited)
make fuzz

# Specific package
go test -v ./testing/unit/domain/...

# Specific test
go test -v -run TestDataPoint_ToJSON ./testing/unit/domain/

# Race detection
go test -race ./...

# With timeout
go test -timeout 5m ./testing/integration/...
```

### Test Tags

```go
//go:build integration
// +build integration

//go:build benchmark
// +build benchmark

//go:build fuzz
// +build fuzz
```

### Running Integration Tests

```bash
# Start simulators
docker-compose -f docker-compose.test.yaml up -d

# Run integration tests
make test-integration

# Stop simulators
docker-compose -f docker-compose.test.yaml down
```

---

## Coverage Goals

| Package | Current | Target | Status |
|---------|---------|--------|--------|
| `internal/domain` | ~10% | 90% | 🔴 |
| `internal/adapter/modbus` | 0% | 85% | 🔴 |
| `internal/adapter/opcua` | 0% | 85% | 🔴 |
| `internal/adapter/s7` | 0% | 85% | 🔴 |
| `internal/adapter/mqtt` | 0% | 85% | 🔴 |
| `internal/adapter/config` | 0% | 90% | 🔴 |
| `internal/api` | 0% | 80% | 🔴 |
| `internal/service` | 0% | 85% | 🔴 |
| `internal/health` | 0% | 80% | 🔴 |
| `internal/metrics` | 0% | 70% | 🔴 |
| **Overall** | **~1%** | **80%** | 🔴 |

---

## Test Inventory

### Overall Progress

| Category | Implemented | Total | Progress |
|----------|-------------|-------|----------|
| Unit Tests | 22 | 28 | 79% |
| Integration Tests | 1 | 20 | 5% |
| Benchmark Tests | 2 | 12 | 17% |
| Fuzz Tests | 0 | 9 | 0% |
| E2E Tests | 0 | 6 | 0% |
| **Total** | **25** | **75** | **33%** |

### Implemented Tests ✅

| Location | File | Tests Included |
|----------|------|----------------|
| `testing/unit/domain/` | `datapoint_test.go` | Creation, JSON, pool, quality |
| `testing/unit/domain/` | `device_test.go` | Protocol constants, configs, metadata |
| `testing/unit/domain/` | `tag_test.go` | Data types, scaling, S7 areas |
| `testing/unit/domain/` | `errors_test.go` | All protocol errors, wrapping |
| `testing/unit/domain/` | `protocol_test.go` | Protocol manager, pool interface, concurrency |
| `testing/unit/adapters/modbus/` | `conversion_test.go` | Byte order, parsing, scaling logic |
| `testing/unit/adapters/modbus/` | `client_test.go` | Client config, stats, diagnostics |
| `testing/unit/adapters/modbus/` | `pool_test.go` | Pool config, health states, scaling |
| `testing/unit/adapters/mqtt/` | `publisher_test.go` | Config defaults, stats, buffering |
| `testing/unit/adapters/opcua/` | `conversion_test.go` | Scaling, type conversions, deadband |
| `testing/unit/adapters/opcua/` | `client_test.go` | Session states, client config, stats |
| `testing/unit/adapters/opcua/` | `subscription_test.go` | Subscription config, deadband, queue size |
| `testing/unit/adapters/opcua/` | `pool_test.go` | Pool config, session sharing, endpoints |
| `testing/unit/adapters/opcua/` | `loadshaping_test.go` | Priority constants, brownout mode, throttling |
| `testing/unit/adapters/s7/` | `client_test.go` | Client config, PDU sizing, atomic stats |
| `testing/unit/adapters/s7/` | `pool_test.go` | Pool config, circuit breaker, health checks |
| `testing/unit/config/` | `config_test.go` | Config struct, validation, defaults |
| `testing/unit/config/` | `devices_test.go` | Device config, connection config, tags |
| `testing/unit/health/` | `checker_test.go` | Operational states, severity levels, config |
| `testing/unit/metrics/` | `registry_test.go` | Metric naming, counters, gauges, histograms |
| `testing/unit/service/` | `polling_test.go` | Polling config, stats, throughput |
| `testing/unit/api/` | `handlers_test.go` | Auth middleware, CORS, body limits |
| `testing/integration/modbus/` | `connection_test.go` | TCP connection, read/write, recovery |
| `testing/benchmark/throughput/` | `datapoint_test.go` | Creation, pool, batch benchmarks |
| `testing/benchmark/concurrency/` | `stress_test.go` | Parallel creation, pool contention |
| `internal/domain/` | `datapoint_bench_test.go` | Original benchmarks |

### Support Files ✅

| Location | File | Purpose |
|----------|------|---------|
| `testing/mocks/` | `protocol_client.go` | Mock protocol client interface |
| `testing/mocks/` | `mqtt_publisher.go` | Mock MQTT publisher |
| `testing/mocks/` | `health_checker.go` | Mock health checker |
| `testing/testutil/` | `helpers.go` | Test context, assertions, factories |
| `testing/testutil/` | `fixtures.go` | Fixture loading utilities |
| `testing/fixtures/configs/` | `*.yaml` | Test configuration files |
| `testing/integration/` | `helpers.go` | Integration test utilities |

### High Priority Planned 📋

| Location | File | Why Important |
|----------|------|---------------|
| `testing/unit/adapters/s7/` | `conversion_test.go` | S7 type conversion |
| `testing/unit/adapters/modbus/` | `health_test.go` | Health check logic |
| `testing/unit/adapters/opcua/` | `session_test.go` | Session management |
| `testing/unit/adapters/s7/` | `health_test.go` | Health checks |
| `testing/unit/service/` | `command_handler_test.go` | Write commands |
| `testing/unit/api/` | `runtime_test.go` | Runtime management |

---

## Test Naming Conventions

```go
// Unit tests: Test<Type>_<Method>_<Scenario>
func TestDataPoint_ToJSON_WithAllFields(t *testing.T)
func TestDataPoint_ToJSON_WithNilValue(t *testing.T)
func TestClient_Connect_Timeout(t *testing.T)

// Table-driven tests
func TestConversion_BytesToFloat32(t *testing.T) {
    tests := []struct {
        name     string
        input    []byte
        order    ByteOrder
        expected float32
        wantErr  bool
    }{...}
}

// Benchmarks: Benchmark<Type>_<Operation>
func BenchmarkDataPoint_ToJSON(b *testing.B)
func BenchmarkPool_GetClient(b *testing.B)

// Fuzz tests: Fuzz<Target>
func FuzzModbusConversion(f *testing.F)
func FuzzConfigParsing(f *testing.F)
```

---

## Mock Strategy

We use interface-based mocking for isolation:

```go
// Protocol client interface (mockable)
type ProtocolClient interface {
    Connect(ctx context.Context) error
    Disconnect() error
    ReadTags(ctx context.Context, tags []*domain.Tag) ([]*domain.DataPoint, error)
    WriteTag(ctx context.Context, tag *domain.Tag, value interface{}) error
    IsConnected() bool
}

// Mock implementation
type MockClient struct {
    ConnectFunc    func(ctx context.Context) error
    ReadTagsFunc   func(ctx context.Context, tags []*domain.Tag) ([]*domain.DataPoint, error)
    // ...
}
```

---

## Test Data Management

### Fixtures Location

```
testing/
└── fixtures/
    ├── configs/
    │   ├── valid_config.yaml
    │   ├── invalid_config.yaml
    │   └── minimal_config.yaml
    ├── devices/
    │   ├── modbus_devices.yaml
    │   ├── opcua_devices.yaml
    │   └── s7_devices.yaml
    └── responses/
        ├── modbus_responses.json
        ├── opcua_responses.json
        └── s7_responses.json
```

### Golden Files

For complex outputs, we use golden files:

```go
func TestHandler_GetDevices(t *testing.T) {
    golden := filepath.Join("testdata", "get_devices.golden.json")
    // Compare output against golden file
}
```

---

## CI/CD Integration

Tests are integrated into the CI pipeline:

```yaml
# .github/workflows/test.yaml
jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: make test-cover
      - uses: codecov/codecov-action@v4

  integration-tests:
    runs-on: ubuntu-latest
    services:
      mosquitto:
        image: eclipse-mosquitto:2
      modbus-sim:
        image: oitc/modbus-server
    steps:
      - run: make test-integration

  benchmarks:
    runs-on: ubuntu-latest
    steps:
      - run: make bench
      - uses: benchmark-action/github-action-benchmark@v1
```

---

## Related Documentation

- [ARCHITECTURE.md](../ARCHITECTURE.md) - System architecture
- [README.md](../README.md) - Project overview
- [TODO.md](../TODO.md) - Known issues and planned work
- [testing/README.md](README.md) - Quick start for running tests

---

*Last updated: 2026-02-02*