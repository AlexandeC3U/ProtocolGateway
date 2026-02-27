- [18. Device Configuration Example](#18-device-configuration-example)
  - [Validation Rules](#validation-rules-at-load-time)

## 18. Device Configuration Example

A complete device configuration example showing all supported fields and validation rules. Devices are defined in `config/devices.yaml`:

```yaml
version: "1.0"
devices:
  - id: "SIM1"                          # Required, must be unique
    name: Demo OPC UA
    protocol: opcua                      # modbus-tcp | modbus-rtu | opcua | s7
    enabled: true
    uns_prefix: plant1/area1/line1       # UNS topic prefix
    poll_interval: 5s                    # Parsed from duration string
    connection:
      opc_endpoint_url: opc.tcp://opcua-simulator:4840
    tags:
      - id: tag-001                      # Required, unique within device
        name: Temperature
        data_type: float64               # bool | int16 | uint16 | int32 | uint32 | float32 | float64 | string
        topic_suffix: temperature        # Appended to uns_prefix for MQTT topic
        opc_node_id: "ns=2;s=Demo.Temperature"
```

**Validation rules at load time:**
- Duplicate device IDs are rejected
- Protocol-specific: Modbus requires `slave_id` (1–247), OPC UA requires `opc_endpoint_url`, S7 requires valid `rack`/`slot`
- Tag `register_count` ≥ 1, `data_type` must be recognized
- Durations are parsed from strings (`"5s"`, `"100ms"`)
- `SaveDevices()` writes back to YAML with `0600` permissions (credential protection)

---
