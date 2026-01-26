# OPC UA Simulator (dev)

Small local OPC UA server used for testing the gateway.

## Endpoint

- `opc.tcp://opcua-simulator:4840` (from inside docker network)
- `opc.tcp://localhost:4840` (from your host)

Security is **None** (dev only).

## Nodes

All nodes are in namespace index **2** (the first custom namespace).

- `ns=<idx>;s=Demo.Temperature` (float)
- `ns=<idx>;s=Demo.Pressure` (float)
- `ns=<idx>;s=Demo.Status` (string)
- `ns=<idx>;s=Demo.Switch` (bool)

Where `<idx>` is the namespace index assigned at runtime (printed on startup). In this repo's configs we use `ns=2`.

## Run via docker-compose

From repo root:

- `docker compose -f docker-compose.dev.yaml up --build`

Then configure an OPC UA device with:

- host: `opcua-simulator`
- port: `4840`
- `opc_security_policy: None`
- `opc_security_mode: None`
- `opc_auth_mode: Anonymous`

## Environment variables

- `OPCUA_PORT` (default `4840`)
- `OPCUA_AUTO_UPDATE` (`1`/`0`, default `1`)
- `OPCUA_UPDATE_MS` (default `500`)
