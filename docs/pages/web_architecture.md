- [13. Web UI Architecture](#13-web-ui-architecture)
  - [13.1 Frontend Technology Stack](#131-frontend-technology-stack)
  - [13.2 API Endpoints](#132-api-endpoints)

## 13. Web UI Architecture

### 13.1 Frontend Technology Stack

The embedded Web UI provides runtime device management without requiring a separate frontend build process. Built with React 18 (via CDN) and pure CSS, it runs directly from static files served by the gateway from `web/index.html`. The component architecture diagram shows the `App` root, navigation, and `DeviceForm` modal with protocol-specific field rendering:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                          WEB UI ARCHITECTURE                                   │
│                                                                                │
│  Technology Stack:                                                             │
│  • React 18 (via CDN, no build step required)                                  │
│  • Babel standalone (JSX transformation in browser)                            │
│  • Pure CSS (CSS Custom Properties for theming)                                │
│  • IBM Plex Sans + JetBrains Mono fonts                                        │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    COMPONENT ARCHITECTURE                               │   │
│  │                                                                         │   │
│  │  ┌─────────────────────────────────────────────────────────────────┐    │   │
│  │  │  App (Root Component)                                           │    │   │
│  │  │  ├── State: devices, activeSection, apiStatus                   │    │   │
│  │  │  ├── Effects: loadDevices(), loadTopics(), loadLogs()           │    │   │
│  │  │  │                                                              │    │   │
│  │  │  ├── SideNav                                                    │    │   │
│  │  │  │   └── Navigation items with icons                            │    │   │
│  │  │  │                                                              │    │   │
│  │  │  ├── TopBar                                                     │    │   │
│  │  │  │   ├── Page title and metadata                                │    │   │
│  │  │  │   └── API status indicator                                   │    │   │
│  │  │  │                                                              │    │   │
│  │  │  └── Content (conditional rendering by section)                 │    │   │
│  │  │      ├── Overview: Stats cards                                  │    │   │
│  │  │      ├── Devices: DeviceTable + DeviceForm modal                │    │   │
│  │  │      ├── Topics/Routes: Topic tables                            │    │   │
│  │  │      └── Logs: Container log viewer                             │    │   │
│  │  └─────────────────────────────────────────────────────────────────┘    │   │
│  │                                                                         │   │
│  │  ┌─────────────────────────────────────────────────────────────────┐    │   │
│  │  │  DeviceForm (Modal Component)                                   │    │   │
│  │  │  ├── Tabs: Basic Info, Connection, Tags                         │    │   │
│  │  │  ├── Protocol-specific fields (Modbus/OPC UA/S7)                │    │   │
│  │  │  ├── Dynamic tag list with add/remove                           │    │   │
│  │  │  ├── Connection test functionality                              │    │   │
│  │  │  └── "Browse" button (OPC UA) → opens BrowseModal               │    │   │
│  │  └─────────────────────────────────────────────────────────────────┘    │   │
│  │                                                                         │   │
│  │  ┌─────────────────────────────────────────────────────────────────┐    │   │
│  │  │  BrowseModal (OPC UA Address Space Explorer)                    │    │   │
│  │  │  ├── Tree-view of server address space                          │    │   │
│  │  │  ├── Lazy-loads children on expand (depth=1 per request)        │    │   │
│  │  │  ├── Shows NodeClass, DataType, AccessLevel per node            │    │   │
│  │  │  └── Select Variable node → auto-fills opc_node_id in tag       │    │   │
│  │  └─────────────────────────────────────────────────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                      API SERVICE LAYER                                  │   │
│  │                                                                         │   │
│  │  const api = {                                                          │   │
│  │    getDevices()        → GET  /api/devices                              │   │
│  │    createDevice(d)     → POST /api/devices                              │   │
│  │    updateDevice(d)     → PUT  /api/devices                              │   │
│  │    deleteDevice(id)    → DELETE /api/devices?id={id}                    │   │
│  │    testConnection(d)   → POST /api/test-connection                      │   │
│  │    browseOPCUA(id,n,d) → GET  /api/browse/{id}?node_id&max_depth        │   │
│  │    getTopicsOverview() → GET  /api/topics                               │   │
│  │    listLogContainers() → GET  /api/logs/containers                      │   │
│  │    getLogs(c, tail)    → GET  /api/logs?container={c}&tail={n}          │   │
│  │  }                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  Design System:                                                                │
│  • Dark theme (industrial aesthetic)                                           │
│  • CSS Custom Properties for consistent theming                                │
│  • Responsive layout (collapses sidebar on mobile)                             │
│  • Accessible: ARIA labels, focus indicators, keyboard navigation              │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 13.2 API Endpoints

The REST API (`internal/api/`) enables programmatic device management and operational monitoring. The diagram documents all endpoints grouped by function (device CRUD, runtime information, observability), including request/response formats and side effects. CORS is enabled for browser-based access, and API key authentication is optional via `X-API-Key` header:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                            REST API DESIGN                                     │
│                                                                                │
│  Base URL: http://localhost:8080                                               │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    DEVICE MANAGEMENT                                    │   │
│  │                                                                         │   │
│  │  GET /api/devices                                                       │   │
│  │    Response: Device[] (all devices)                                     │   │
│  │                                                                         │   │
│  │  GET /api/devices?id={device_id}                                        │   │
│  │    Response: Device (single device)                                     │   │
│  │                                                                         │   │
│  │  POST /api/devices                                                      │   │
│  │    Body: Device (new device)                                            │   │
│  │    Response: { success: true, device: Device }                          │   │
│  │    Side effects: Persists to YAML, registers with polling service       │   │
│  │                                                                         │   │
│  │  PUT /api/devices                                                       │   │
│  │    Body: Device (updated device)                                        │   │
│  │    Response: { success: true, device: Device }                          │   │
│  │    Side effects: Persists to YAML, re-registers with polling service    │   │
│  │                                                                         │   │
│  │  DELETE /api/devices?id={device_id}                                     │   │
│  │    Response: { success: true }                                          │   │
│  │    Side effects: Removes from YAML, unregisters from polling service    │   │
│  │                                                                         │   │
│  │  POST /api/test-connection                                              │   │
│  │    Body: Device (device to test)                                        │   │
│  │    Response: { success: true } or error                                 │   │
│  │    Note: Validates configuration only, no actual connection             │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    OPC UA BROWSE                                        │   │
│  │                                                                         │   │
│  │  GET /api/browse/{deviceID}?node_id={id}&max_depth={n}                  │   │
│  │    Response: BrowseResult (tree of nodes with children)                 │   │
│  │    Defaults: node_id="" (Objects folder), max_depth=1 (max 5)           │   │
│  │    Requires: Device must be OPC UA protocol                             │   │
│  │    Caching: 60s per-endpoint TTL, shared across devices                 │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    CERTIFICATE MANAGEMENT (conditional)                 │   │
│  │                                                                         │   │
│  │  Only registered when trust_store_path is configured.                   │   │
│  │                                                                         │   │
│  │  GET /api/opcua/certificates/trusted                                    │   │
│  │    Response: { certificates: TrustStoreInfo[], count: int }             │   │
│  │                                                                         │   │
│  │  DELETE /api/opcua/certificates/trusted?fingerprint=sha256:...          │   │
│  │    Response: { status: "removed", fingerprint: "..." }                  │   │
│  │                                                                         │   │
│  │  GET /api/opcua/certificates/rejected                                   │   │
│  │    Response: { certificates: TrustStoreInfo[], count: int }             │   │
│  │                                                                         │   │
│  │  POST /api/opcua/certificates/trust                                     │   │
│  │    Body: { "fingerprint": "sha256:..." }                                │   │
│  │    Response: { status: "trusted", fingerprint: "..." }                  │   │
│  │    Action: Moves cert from rejected/ to trusted/ store                  │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    RUNTIME INFORMATION                                  │   │
│  │                                                                         │   │
│  │  GET /api/topics?limit={n}                                              │   │
│  │    Response: {                                                          │   │
│  │      active_topics: TopicStats[],   // Recently published topics        │   │
│  │      subscriptions: string[],        // MQTT subscription patterns      │   │
│  │      routes: RouteConfig[]           // Device→Tag→Topic mappings       │   │
│  │    }                                                                    │   │
│  │                                                                         │   │
│  │  GET /api/logs/containers                                               │   │
│  │    Response: { containers: string[] }                                   │   │
│  │    Note: Requires Docker socket access                                  │   │
│  │                                                                         │   │
│  │  GET /api/logs?container={name}&tail={n}                                │   │
│  │    Response: { entries: LogEntry[] }                                    │   │
│  │    Note: Parses JSON logs with timestamp, level, message                │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    OBSERVABILITY                                        │   │
│  │                                                                         │   │
│  │  GET /health                                                            │   │
│  │    Response: { status, checks: CheckResult[], timestamp }               │   │
│  │                                                                         │   │
│  │  GET /health/live                                                       │   │
│  │    Response: 200 OK (process running) or 503 Service Unavailable        │   │
│  │                                                                         │   │
│  │  GET /health/ready                                                      │   │
│  │    Response: 200 OK (ready) or 503 Service Unavailable                  │   │
│  │                                                                         │   │
│  │  GET /metrics                                                           │   │
│  │    Response: Prometheus text format                                     │   │
│  │                                                                         │   │
│  │  GET /status                                                            │   │
│  │    Response: { service, version, polling_stats }                        │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  CORS: Enabled for all origins (development convenience)                       │
│  Content-Type: application/json                                                │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---