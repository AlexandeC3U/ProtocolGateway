- [16. Appendices](#16-appendices)
  - [Appendix A: Configuration Reference](#appendix-a-configuration-reference)
  - [Appendix B: Error Code Reference](#appendix-b-error-code-reference)
  - [Appendix C: Dependency Inventory](#appendix-c-dependency-inventory)

## 16. Appendices

### Appendix A: Configuration Reference

```yaml
# Complete configuration reference (config.yaml)

# Environment: development | staging | production
environment: development

# Path to device configuration file
devices_config_path: ./config/devices.yaml

# HTTP server configuration
http:
  port: 8080                    # Server port
  read_timeout: 10s             # Request read timeout
  write_timeout: 10s            # Response write timeout
  idle_timeout: 60s             # Keep-alive idle timeout

# MQTT publisher configuration
mqtt:
  broker_url: tcp://localhost:1883    # Broker URL (tcp:// or ssl://)
  client_id: protocol-gateway         # Client identifier
  username: ""                        # Optional username
  password: ""                        # Optional password
  clean_session: true                 # Start with clean session
  qos: 1                              # Default QoS (0, 1, 2)
  keep_alive: 30s                     # Keep-alive interval
  connect_timeout: 10s                # Connection timeout
  reconnect_delay: 5s                 # Reconnection delay
  max_reconnect: -1                   # Max reconnect attempts (-1 = unlimited)
  buffer_size: 10000                  # Message buffer size
  
  # TLS configuration
  tls_enabled: false
  tls_cert_file: ""                   # Client certificate
  tls_key_file: ""                    # Client private key
  tls_ca_file: ""                     # CA certificate
  tls_insecure_skip_verify: false     # Skip certificate verification

# Modbus protocol configuration
modbus:
  max_connections: 100                # Maximum concurrent connections
  idle_timeout: 5m                    # Idle connection timeout
  max_ttl: 0                          # Connection TTL hard cap (0 = disabled)
  health_check_period: 30s            # Health check interval
  connection_timeout: 10s             # Connection timeout
  retry_attempts: 3                   # Max retry attempts
  retry_delay: 100ms                  # Initial retry delay

# OPC UA protocol configuration
opcua:
  max_connections: 50                 # Maximum endpoint sessions
  idle_timeout: 5m                    # Idle session timeout
  max_ttl: 0                          # Session TTL hard cap (0 = disabled)
  health_check_period: 30s            # Health check interval
  connection_timeout: 15s             # Connection timeout
  retry_attempts: 3                   # Max retry attempts
  retry_delay: 500ms                  # Initial retry delay
  default_security_policy: None       # None|Basic128Rsa15|Basic256|Basic256Sha256
  default_security_mode: None         # None|Sign|SignAndEncrypt
  default_auth_mode: Anonymous        # Anonymous|UserName|Certificate
  max_global_inflight: 1000           # Global concurrent operations limit
  brownout_threshold: 0.8             # Brownout mode trigger (0.0-1.0)
  max_inflight_per_endpoint: 100      # Per-endpoint operation limit

# S7 protocol configuration
s7:
  max_connections: 100                # Maximum concurrent connections
  idle_timeout: 5m                    # Idle connection timeout
  max_ttl: 0                          # Connection TTL hard cap (0 = disabled)
  health_check_period: 30s            # Health check interval
  connection_timeout: 10s             # Connection timeout
  retry_attempts: 3                   # Max retry attempts
  retry_delay: 500ms                  # Initial retry delay

# Polling service configuration
polling:
  worker_count: 10                    # Concurrent polling workers
  batch_size: 50                      # Max tags per batch read
  default_interval: 1s                # Default poll interval

# Logging configuration
logging:
  level: info                         # trace|debug|info|warn|error
  format: json                        # json|console
  output: stdout                      # stdout|stderr|<filepath>
```

### Appendix B: Error Code Reference

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                         ERROR CODE REFERENCE                                   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  CONFIGURATION ERRORS                                                   │   │
│  │                                                                         │   │
│  │  ErrDeviceIDRequired         Device ID is required                      │   │
│  │  ErrDeviceNameRequired       Device name is required                    │   │
│  │  ErrProtocolRequired         Protocol must be specified                 │   │
│  │  ErrNoTagsDefined            At least one tag must be defined           │   │
│  │  ErrPollIntervalTooShort     Poll interval must be ≥100ms               │   │
│  │  ErrUNSPrefixRequired        UNS prefix is required for MQTT routing    │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  CONNECTION ERRORS                                                      │   │
│  │                                                                         │   │
│  │  ErrConnectionFailed         Failed to establish connection             │   │
│  │  ErrConnectionTimeout        Connection attempt timed out               │   │
│  │  ErrConnectionClosed         Connection was closed unexpectedly         │   │
│  │  ErrConnectionReset          Connection was reset by peer               │   │
│  │  ErrMaxRetriesExceeded       Maximum retry attempts exceeded            │   │
│  │  ErrCircuitBreakerOpen       Circuit breaker is open                    │   │
│  │  ErrPoolExhausted            Connection pool exhausted                  │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  MODBUS-SPECIFIC ERRORS                                                 │   │
│  │                                                                         │   │
│  │  ErrModbusIllegalFunction        Function code not supported (0x01)     │   │
│  │  ErrModbusIllegalAddress         Invalid data address (0x02)            │   │
│  │  ErrModbusIllegalValue           Invalid data value (0x03)              │   │
│  │  ErrModbusDeviceFailure          Slave device failure (0x04)            │   │
│  │  ErrModbusAcknowledge            Long operation in progress (0x05)      │   │
│  │  ErrModbusBusy                   Slave device busy (0x06)               │   │
│  │  ErrModbusNegativeAck            Negative acknowledge (0x07)            │   │
│  │  ErrModbusMemoryParityError      Memory parity error (0x08)             │   │
│  │  ErrModbusGatewayPathUnavailable Gateway path unavailable (0x0A)        │   │
│  │  ErrModbusGatewayTargetFailed    Target device no response (0x0B)       │   │
│  │  ErrModbusProtocolLimit          Protocol limit exceeded                │   │
│  │  ErrInvalidRegisterCount         Invalid register count                 │   │
│  │  ErrInvalidSlaveID               Slave ID must be 1-247                 │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  OPC UA-SPECIFIC ERRORS                                                 │   │
│  │                                                                         │   │
│  │  ErrOPCUAInvalidNodeID       Node ID format is invalid                  │   │
│  │  ErrOPCUANodeNotFound        Node does not exist                        │   │
│  │  ErrOPCUASubscriptionFailed  Failed to create subscription              │   │
│  │  ErrOPCUABadStatus           Bad status code from server                │   │
│  │  ErrOPCUASecurityFailed      Security negotiation failed                │   │
│  │  ErrOPCUASessionExpired      Session expired                            │   │
│  │  ErrOPCUABrowseFailed        Browse operation failed                    │   │
│  │  ErrOPCUAAccessDenied        Access denied to node                      │   │
│  │  ErrOPCUAWriteNotPermitted   Write not permitted on node                │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  S7-SPECIFIC ERRORS                                                     │   │
│  │                                                                         │   │
│  │  ErrS7ConnectionFailed       Failed to connect to PLC                   │   │
│  │  ErrS7InvalidAddress         Invalid S7 address format                  │   │
│  │  ErrS7InvalidDBNumber        Invalid data block number                  │   │
│  │  ErrS7InvalidArea            Invalid memory area                        │   │
│  │  ErrS7InvalidOffset          Invalid offset                             │   │
│  │  ErrS7ReadFailed             Read operation failed                      │   │
│  │  ErrS7WriteFailed            Write operation failed                     │   │
│  │  ErrS7CPUError               CPU error                                  │   │
│  │  ErrS7PDUSizeMismatch        PDU size mismatch                          │   │
│  │  ErrS7ItemNotAvailable       Item not available                         │   │
│  │  ErrS7AddressOutOfRange      Address out of range                       │   │
│  │  ErrS7WriteDataSizeMismatch  Write data size mismatch                   │   │
│  │  ErrS7ObjectNotExist         Object does not exist                      │   │
│  │  ErrS7HardwareFault          Hardware fault                             │   │
│  │  ErrS7AccessingNotAllowed    Accessing not allowed                      │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  MQTT-SPECIFIC ERRORS                                                   │   │
│  │                                                                         │   │
│  │  ErrMQTTConnectionFailed     Failed to connect to broker                │   │
│  │  ErrMQTTPublishFailed        Failed to publish message                  │   │
│  │  ErrMQTTNotConnected         MQTT client not connected                  │   │
│  │  ErrMQTTSubscribeFailed      Failed to subscribe to topic               │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  READ/WRITE ERRORS                                                      │   │
│  │                                                                         │   │
│  │  ErrReadFailed               Read operation failed                      │   │
│  │  ErrWriteFailed              Write operation failed                     │   │
│  │  ErrInvalidAddress           Invalid register address                   │   │
│  │  ErrInvalidDataLength        Invalid data length                        │   │
│  │  ErrInvalidDataType          Invalid data type                          │   │
│  │  ErrInvalidRegisterType      Invalid register type                      │   │
│  │  ErrTagNotWritable           Tag is not writable                        │   │
│  │  ErrInvalidWriteValue        Invalid value for write operation          │   │
│  │  ErrWriteTimeout             Write operation timed out                  │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  SERVICE ERRORS                                                         │   │
│  │                                                                         │   │
│  │  ErrServiceNotStarted        Service has not been started               │   │
│  │  ErrServiceStopped           Service has been stopped                   │   │
│  │  ErrServiceOverloaded        Service overloaded (brownout mode)         │   │
│  │  ErrDeviceNotFound           Device not found                           │   │
│  │  ErrDeviceExists             Device already exists                      │   │
│  │  ErrTagNotFound              Tag not found on device                    │   │
│  │  ErrInvalidConfig            Invalid configuration                      │   │
│  │  ErrProtocolNotSupported     Protocol not supported                     │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### Appendix C: Dependency Inventory

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                         GO MODULE DEPENDENCIES                                 │
│                                                                                │
│  Core Dependencies:                                                            │
│  ┌───────────────────────────────────────────────────────────────────────────┐ │
│  │ Module                              │ Version  │ Purpose                  │ │
│  │─────────────────────────────────────┼──────────┼──────────────────────────│ │
│  │ github.com/eclipse/paho.mqtt.golang │ v1.4.3   │ MQTT client              │ │
│  │ github.com/goburrow/modbus          │ v0.1.0   │ Modbus TCP/RTU client    │ │
│  │ github.com/gopcua/opcua             │ v0.5.3   │ OPC UA client            │ │
│  │ github.com/robinson/gos7            │ v0.0.0   │ Siemens S7 client        │ │
│  │ github.com/prometheus/client_golang │ v1.19.0  │ Prometheus metrics       │ │
│  │ github.com/rs/zerolog               │ v1.32.0  │ Structured logging       │ │
│  │ github.com/sony/gobreaker           │ v0.5.0   │ Circuit breaker          │ │
│  │ github.com/spf13/viper              │ v1.18.2  │ Configuration            │ │
│  │ gopkg.in/yaml.v3                    │ v3.0.1   │ YAML parsing             │ │
│  └─────────────────────────────────────┴──────────┴──────────────────────────┘ │
│                                                                                │
│  Transitive Dependencies (selected):                                           │
│  ┌───────────────────────────────────────────────────────────────────────────┐ │
│  │ github.com/gorilla/websocket        │ v1.5.0   │ MQTT WebSocket support   │ │
│  │ github.com/goburrow/serial          │ v0.1.0   │ Modbus RTU serial        │ │
│  │ google.golang.org/protobuf          │ v1.32.0  │ Protocol Buffers         │ │
│  │ golang.org/x/sync                   │ v0.6.0   │ Sync primitives          │ │
│  └─────────────────────────────────────┴──────────┴──────────────────────────┘ │
│                                                                                │
│  Build Tools:                                                                  │
│  • Go 1.22+                                                                    │
│  • golangci-lint (linting)                                                     │
│  • air (hot reload)                                                            │
│  • gosec (security scanning)                                                   │
│  • govulncheck (vulnerability scanning)                                        │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---