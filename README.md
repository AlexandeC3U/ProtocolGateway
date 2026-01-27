
# Connector Gateway (Protocol Gateway) — Quick Start

This repo runs a small dev stack with:

- **Protocol Gateway** (Go)
- **EMQX** (MQTT broker)
- **OPC UA Simulator** (Python `asyncua`) for local testing

## Prerequisites

- Docker Desktop (Compose v2)

## 1) Clone

```bash
git clone https://github.com/AlexandeC3U/ProtocolGateway
cd Connector_Gateway
```

## 2) Start the dev stack

This starts **EMQX**, the **OPC UA simulator**, and the **gateway**.

```bash
docker compose -f docker-compose.yaml up --build
```

Open:

- Web UI: `http://localhost:8080`
- EMQX dashboard: `http://localhost:18083`

Stop:

```bash
docker compose -f docker-compose.yaml down
```

## 3) Add an OPC UA device (simulator) in the Web UI

In the Web UI, go to **Devices** → **Add Device** and use:

- **Protocol**: `opcua`
- **Device ID**: e.g. `SIM2`
- **Name**: e.g. `OPCUA-SIM`
- **Enabled**: `true`
- **UNS Prefix**: `plant1/area1/line1`
- **Poll interval**: `5s`

Connection:

- **OPC Endpoint URL**: `opc.tcp://opcua-simulator:4840`
- **Security Policy/Mode/Auth**: leave empty / defaults (simulator is **NoSecurity**)

### Add tags (4 demo nodes)

Add 4 tags and make sure each tag has a **unique** `topic_suffix`.

1) Temperature

- **Name**: `Temperature`
- **Data type**: `float32` or `float64`
- **Unit**: `°C`
- **Topic suffix**: `temperature`
- **OPC Node ID**: `ns=2;s=Demo.Temperature`

2) Pressure

- **Name**: `Pressure`
- **Data type**: `float32` or `float64`
- **Topic suffix**: `pressure`
- **OPC Node ID**: `ns=2;s=Demo.Pressure`

3) Status

- **Name**: `Status`
- **Data type**: `string`
- **Topic suffix**: `status`
- **OPC Node ID**: `ns=2;s=Demo.Status`

4) Switch

- **Name**: `Switch`
- **Data type**: `bool`
- **Topic suffix**: `switch`
- **OPC Node ID**: `ns=2;s=Demo.Switch`

Expected MQTT topics:

- `plant1/area1/line1/temperature`
- `plant1/area1/line1/pressure`
- `plant1/area1/line1/status`
- `plant1/area1/line1/switch`

Payload format:

```json
{"v": 20.1, "u": "°C", "q": "good", "ts": 1769445124645}
```

## 4) Verify messages in MQTT

Subscribe with any MQTT client to `plant1/#` on `localhost:1883`.

If you use **MQTT Explorer**, subscribe to `plant1/#` (not `#`). In this setup, subscribing to `#` is denied by broker access control.

Example with `mosquitto_sub`:

```bash
mosquitto_sub -h localhost -p 1883 -t 'plant1/#' -v
```
## 5) Testing Write Commands

You can write values to tags using the EMQX dashboard or any MQTT client.

### Using EMQX Dashboard

1. Open the EMQX dashboard: `http://localhost:18083` (default login: `admin` / `public`)
2. Go to **Diagnose** → **WebSocket Client**
3. Click **Connect**
4. To publish a write command:
   - **Topic**: `$nexus/cmd/{device_id}/{tag_id}/set`
   - **Payload**: The value to write

### Example: Writing to the Switch tag

- **Topic**: `$nexus/cmd/SIM/tag-1769444077413/set`
- **Payload**: `true`, `True`, `1`, `false`, `False`, or `0`

For numeric tags, send the value directly (e.g., `25.5` for temperature).

### Write Response

The gateway publishes a response to `$nexus/cmd/{device_id}/{tag_id}/response`:

```json
{"device_id":"SIM","tag_id":"tag-1769444077413","success":true,"timestamp":"2026-01-27T12:00:00Z","duration_ms":45}
```
## Notes / Troubleshooting

- If you restart the stack and previously created devices don’t show up, ensure the gateway is using the same mounted `config/devices.yaml` (Compose does this by default).
- If you previously published with missing `topic_suffix`, your MQTT client may still show old “stale” topics; reconnect the client to clear them.

