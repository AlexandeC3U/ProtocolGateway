- [17. Edge Cases & Gotchas](#17-edge-cases--gotchas)

## 17. Edge Cases & Gotchas

Practical operational notes that don't fit neatly into other sections:

1. **Device edit = unregister + re-register**: When a device is updated via the API (`PUT /api/devices`), the polling service unregisters and re-registers it. This resets poll jitter, retry state, and any in-progress operations. A `TODO` in `main.go:192` notes this should be a `ReplaceDevice()` call to preserve state.

2. **Metrics endpoint gating**: The `/metrics` endpoint returns `503 Service Unavailable` during startup until `gatewayReady` is set to `true`. If Prometheus is configured with a short scrape interval, initial scrapes will fail — this is intentional to prevent incomplete data from being stored.

3. **Docker socket dependency**: The container log viewer in the Web UI requires `/var/run/docker.sock` mounted into the gateway container. Without it, the feature silently degrades (no logs available, but the gateway runs fine otherwise).

4. **Configuration hot-reload scope**: Device configuration changes via the API (`POST`/`PUT`/`DELETE` `/api/devices`) are persisted to `devices.yaml` and take effect immediately (device registered/unregistered with polling service). The service config (`config.yaml`) is only read at startup — changes require a restart.

5. **MQTT topic sanitization**: The polling service strips or replaces characters that are invalid in MQTT topics (e.g., wildcards `#`, `+`). If a tag's `topic_suffix` contains special characters, the actual MQTT topic may differ from what's configured.

6. **Unsupported protocol handling**: Devices configured with an unrecognized protocol are logged as warnings and skipped during registration. The gateway starts and operates normally but reports a degraded state if any devices fail registration.

---
