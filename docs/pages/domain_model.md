- [5. Domain Model](#5-domain-model)
  - [5.1 Validation Logic](#51-validation-logic)
  - [5.2 Error Taxonomy](#52-error-taxonomy)

## 5. Domain Model

### 5.1 Validation Logic

Every domain entity includes comprehensive validation to ensure configuration correctness before runtime:

```go
func (d *Device) Validate() error {
    var errs []error
    
    if d.ID == "" {
        errs = append(errs, ErrDeviceIDRequired)
    }
    if d.Name == "" {
        errs = append(errs, ErrDeviceNameRequired)
    }
    if d.Protocol == "" {
        errs = append(errs, ErrProtocolRequired)
    }
    if len(d.Tags) == 0 {
        errs = append(errs, ErrNoTagsDefined)
    }
    if d.PollInterval < 100*time.Millisecond {
        errs = append(errs, ErrPollIntervalTooShort)
    }
    if d.UNSPrefix == "" {
        errs = append(errs, ErrUNSPrefixRequired)
    }
    
    // Validate each tag for the device's protocol
    for _, tag := range d.Tags {
        if err := tag.ValidateForProtocol(d.Protocol); err != nil {
            errs = append(errs, fmt.Errorf("tag %s: %w", tag.ID, err))
        }
    }
    
    if len(errs) > 0 {
        return errors.Join(errs...)
    }
    return nil
}
```

### 5.2 Error Taxonomy

The gateway classifies errors into four categories based on their origin and recoverability. This taxonomy drives automated recovery decisions—configuration errors require human intervention, connection errors trigger circuit breakers and retries, protocol errors may indicate device misconfiguration, and service errors affect API responses. Understanding this classification helps operators diagnose issues quickly:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                            ERROR CLASSIFICATION                                │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    CONFIGURATION ERRORS (Preventable)                   │   │
│  │                                                                         │   │
│  │  • ErrDeviceIDRequired      - Device must have an ID                    │   │
│  │  • ErrDeviceNameRequired    - Device must have a name                   │   │
│  │  • ErrProtocolRequired      - Protocol must be specified                │   │
│  │  • ErrNoTagsDefined         - At least one tag required                 │   │
│  │  • ErrPollIntervalTooShort  - Minimum 100ms to prevent overload         │   │
│  │  • ErrUNSPrefixRequired     - UNS compliance requires prefix            │   │
│  │  • ErrInvalidDataType       - Unknown data type                         │   │
│  │  • ErrInvalidRegisterType   - Unknown register type                     │   │
│  │                                                                         │   │
│  │  → These should be caught at configuration time                         │   │
│  │  → Validation runs before device registration                           │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    CONNECTION ERRORS (Runtime, Retryable)               │   │
│  │                                                                         │   │
│  │  • ErrConnectionFailed      - Initial connection failed                 │   │
│  │  • ErrConnectionTimeout     - Connection attempt timed out              │   │
│  │  • ErrConnectionClosed      - Connection unexpectedly closed            │   │
│  │  • ErrConnectionReset       - Connection reset by peer                  │   │
│  │  • ErrMaxRetriesExceeded    - All retry attempts exhausted              │   │
│  │  • ErrPoolExhausted         - No connections available                  │   │
│  │                                                                         │   │
│  │  → Trigger circuit breaker evaluation                                   │   │
│  │  → May trigger reconnection logic                                       │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    PROTOCOL ERRORS (Runtime, May Be Fatal)              │   │
│  │                                                                         │   │
│  │  MODBUS:                                                                │   │
│  │  • ErrModbusIllegalFunction     - FC not supported by device            │   │
│  │  • ErrModbusIllegalAddress      - Register address out of range         │   │
│  │  • ErrModbusIllegalDataValue    - Invalid data in request               │   │
│  │  • ErrModbusSlaveDeviceFailure  - Device internal error                 │   │
│  │  • ErrModbusGatewayPathUnavail  - Gateway routing error                 │   │
│  │                                                                         │   │
│  │  OPC UA:                                                                │   │
│  │  • ErrOPCUAInvalidNodeID        - Node doesn't exist                    │   │
│  │  • ErrOPCUASubscriptionFailed   - Can't create subscription             │   │
│  │  • ErrOPCUASecurityRejected     - Security policy mismatch              │   │
│  │  • ErrOPCUASessionInvalid       - Session expired/invalid               │   │
│  │  • ErrOPCUATooManySessions      - Server session limit reached          │   │
│  │                                                                         │   │
│  │  S7:                                                                    │   │
│  │  • ErrS7InvalidAddress          - Invalid S7 address format             │   │
│  │  • ErrS7AccessDenied            - CPU protection active                 │   │
│  │  • ErrS7ItemNotAvailable        - DB/area doesn't exist                 │   │
│  │                                                                         │   │
│  │  → Some may trigger device-level circuit breaker                        │   │
│  │  → InvalidNodeID/Address suggest config error                           │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    SERVICE ERRORS (Operational)                         │   │
│  │                                                                         │   │
│  │  • ErrServiceNotStarted         - Service not yet initialized           │   │
│  │  • ErrServiceStopped            - Service has been stopped              │   │
│  │  • ErrServiceOverloaded         - Back-pressure active                  │   │
│  │  • ErrDeviceNotFound            - Unknown device ID                     │   │
│  │  • ErrTagNotFound               - Unknown tag ID                        │   │
│  │  • ErrProtocolNotSupported      - Protocol not registered               │   │
│  │  • ErrCircuitBreakerOpen        - Operations blocked                    │   │
│  │                                                                         │   │
│  │  → Typically returned to API callers                                    │   │
│  │  → May indicate system misconfiguration                                 │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---