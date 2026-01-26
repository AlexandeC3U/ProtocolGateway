
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

## Notes / Troubleshooting

- If you restart the stack and previously created devices don’t show up, ensure the gateway is using the same mounted `config/devices.yaml` (Compose does this by default).
- If you previously published with missing `topic_suffix`, your MQTT client may still show old “stale” topics; reconnect the client to clear them.

