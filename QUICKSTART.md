# Quick Start Guide - Protocol Gateway Web UI

Get your Protocol Gateway up and running with the Web UI in minutes!

## Prerequisites

- Docker and Docker Compose installed
- OR Go 1.22+ installed for local development

## Option 1: Docker Compose (Recommended)

### 1. Start the Stack

```bash
# Clone the repository (if not already done)
cd Connector_Gateway

# Start all services (Gateway + EMQX Broker)
docker-compose up -d

# View logs
docker-compose logs -f gateway
```

### 2. Access the Web UI

Open your browser and navigate to:
```
http://localhost:8080
```

You'll see the Protocol Gateway dashboard with:
- Device list (empty initially)
- Statistics
- "Add Device" button

### 3. Add Your First Device

#### Example: Modbus TCP Device

Click "**+ Add Device**" and fill in:

**Basic Info Tab:**
- Device ID: `modbus-plc-001`
- Device Name: `Production Line PLC`
- Protocol: `Modbus TCP`
- Poll Interval: `1` (seconds)
- UNS Prefix: `factory/line1/plc1`
- ✓ Device Enabled

**Connection Tab:**
- Host: `192.168.1.100` (your Modbus device IP)
- Port: `502`
- Slave ID: `1`
- Timeout: `5` (seconds)
- Retry Count: `3`

**Tags Tab:**
Click "**+ Add Tag**" and configure:
- Tag ID: `temperature`
- Tag Name: `Temperature Sensor`
- Address: `0` (Modbus register address)
- Register Type: `Holding Register`
- Data Type: `int16`
- Topic Suffix: `temperature`
- Scale Factor: `0.1` (if raw value is 235 = 23.5°C)
- Unit: `°C`
- ✓ Tag Enabled

Click "**Save Device**"

### 4. Verify Data Publishing

#### Check EMQX Dashboard
1. Open EMQX dashboard: `http://localhost:18083`
2. Default credentials: `admin` / `public`
3. Go to **Monitoring** → **WebSocket Client**
4. Subscribe to: `factory/line1/plc1/#`
5. You should see messages like:
```json
{
  "value": 23.5,
  "unit": "°C",
  "quality": "good",
  "timestamp": "2026-01-13T12:34:56Z"
}
```

#### Check Gateway Logs
```bash
docker-compose logs -f gateway
```

Look for messages like:
```
INFO Successfully polled device device=modbus-plc-001 tags=1
INFO Published data point topic=factory/line1/plc1/temperature
```

## Option 2: Local Development

### 1. Install Dependencies

```bash
# From project root
go mod download
```

### 2. Configure Devices

Edit `config/devices.yaml` or use the default configuration.

### 3. Start the Gateway

```bash
# Ensure MQTT broker is running (or use public broker)
export MQTT_BROKER_URL=tcp://localhost:1883

# Start the gateway
go run cmd/gateway/main.go
```

### 4. Access Web UI

Open browser to:
```
http://localhost:8080
```

## Quick Testing with Simulators

### Use Development Compose (includes Modbus Simulator)

```bash
# Start with Modbus simulator
docker-compose -f docker-compose.dev.yaml up -d

# Add a test device via Web UI pointing to modbus-simulator:5020
```

The dev compose includes:
- EMQX MQTT Broker (ports 1883, 18083)
- Modbus Simulator (port 5020)
- Protocol Gateway (port 8080)

## Example Configurations

### OPC UA Device

```json
{
  "id": "opcua-001",
  "name": "OPC UA Server",
  "protocol": "opcua",
  "enabled": true,
  "uns_prefix": "factory/opcua/server1",
  "poll_interval": 2000000000,
  "connection": {
    "host": "192.168.1.200",
    "port": 4840,
    "opc_security_policy": "None",
    "opc_security_mode": "None",
    "timeout": 10000000000
  },
  "tags": [
    {
      "id": "pressure",
      "name": "Tank Pressure",
      "opc_node_id": "ns=2;s=Tank.Pressure",
      "data_type": "float32",
      "topic_suffix": "pressure",
      "enabled": true
    }
  ]
}
```

### Siemens S7 Device

```json
{
  "id": "s7-plc-001",
  "name": "S7-1500 PLC",
  "protocol": "s7",
  "enabled": true,
  "uns_prefix": "factory/s7/plc1",
  "poll_interval": 1000000000,
  "connection": {
    "host": "192.168.1.10",
    "port": 102,
    "s7_rack": 0,
    "s7_slot": 1,
    "timeout": 5000000000
  },
  "tags": [
    {
      "id": "motor_speed",
      "name": "Motor Speed",
      "s7_address": "DB1.DBW0",
      "data_type": "int16",
      "topic_suffix": "motor/speed",
      "enabled": true
    }
  ]
}
```

## Common Next Steps

### 1. Add Multiple Tags
Edit your device and add more tags to collect different data points.

### 2. Configure UNS Hierarchy
Use meaningful UNS prefixes like:
- `site/building/floor/area/device`
- `plant-a/production/line-1/plc-1`
- `factory/zone-a/machine-123`

### 3. Set Up Dashboards
Use the MQTT data in:
- Grafana
- Node-RED
- InfluxDB
- Your custom application

### 4. Adjust Poll Intervals
Fine-tune polling intervals based on:
- Data change frequency
- Network bandwidth
- System load

### 5. Enable/Disable Devices
Temporarily disable devices without deleting configuration.

## Troubleshooting Quick Tips

### Web UI Not Loading
```bash
# Check if gateway is running
docker-compose ps

# Check gateway logs
docker-compose logs gateway

# Verify port is accessible
curl http://localhost:8080/health
```

### Device Not Connecting
- Verify IP address and port
- Check network connectivity from container
- Test connection using the "Test Connection" button
- Review error messages in gateway logs

### No Data on MQTT
- Check device is enabled
- Verify tags are enabled
- Check MQTT broker connection
- Review polling service logs
- Use EMQX dashboard to verify subscriptions

### Configuration Not Persisting
```bash
# Check volume mounts
docker-compose config

# Verify devices.yaml exists and is writable
ls -l config/devices.yaml

# Restart after manual YAML edits
docker-compose restart gateway
```

## URLs Reference

| Service | URL | Credentials |
|---------|-----|-------------|
| **Web UI** | http://localhost:8080 | None |
| **Health Check** | http://localhost:8080/health | None |
| **Metrics** | http://localhost:8080/metrics | None |
| **API** | http://localhost:8080/api/* | None |
| **EMQX Dashboard** | http://localhost:18083 | admin / public |
| **EMQX MQTT** | tcp://localhost:1883 | None (anonymous) |

## Next Steps

1. Read the full [Web UI Documentation](WEB_UI_README.md)
2. Review the [Main README](README.md) for architecture details
3. Check out example configurations in `config/devices.yaml`
4. Join the community for support and updates

## Getting Help

If you encounter issues:
1. Check the [Web UI README](WEB_UI_README.md) troubleshooting section
2. Review gateway logs: `docker-compose logs -f gateway`
3. Verify MQTT broker connectivity: EMQX dashboard
4. Test device connectivity independently
5. Open an issue on GitHub with logs and configuration

Happy data collecting! 🚀
