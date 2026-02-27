- [11. Security Architecture](#11-security-architecture)
  - [11.1 Transport Security](#111-transport-security)
  - [11.2 Credential Management](#112-credential-management)
  - [11.3 Network Security](#113-network-security)

## 11. Security Architecture

### 11.1 Transport Security

Secure communication is essential for industrial environments handling sensitive operational data. The diagram documents TLS configuration for MQTT (client certificates, cipher suites) and OPC UA security profiles (`Basic256Sha256` recommended). Security mode options range from development (`None`) to production (`SignAndEncrypt`):

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                        TRANSPORT LAYER SECURITY                                │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                        MQTT TLS CONFIGURATION                           │   │
│  │                                                                         │   │
│  │  Client Certificate Authentication:                                     │   │
│  │  ┌─────────────────────────────────────────────────────────────────┐    │   │
│  │  │  mqtt:                                                          │    │   │
│  │  │    broker_url: "ssl://broker.example.com:8883"                  │    │   │
│  │  │    tls_enabled: true                                            │    │   │
│  │  │    tls_cert_file: "/certs/client.crt"                           │    │   │
│  │  │    tls_key_file: "/certs/client.key"                            │    │   │
│  │  │    tls_ca_file: "/certs/ca.crt"                                 │    │   │
│  │  │    tls_insecure_skip_verify: false  # NEVER true in production  │    │   │
│  │  └─────────────────────────────────────────────────────────────────┘    │   │
│  │                                                                         │   │
│  │  Supported Cipher Suites: TLS 1.2+ (Go crypto/tls defaults)             │   │
│  │  • TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256                                │   │
│  │  • TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384                                │   │
│  │  • TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256                              │   │
│  │  • TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384                              │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                     OPC UA SECURITY POLICIES                            │   │
│  │                                                                         │   │
│  │  Policy              │ Encryption    │ Signature     │ Use Case         │   │
│  │  ────────────────────┼───────────────┼───────────────┼────────────────  │   │
│  │  None                │ None          │ None          │ Development      │   │
│  │  Basic128Rsa15       │ RSA-OAEP      │ RSA-SHA1      │ Legacy (avoid)   │   │
│  │  Basic256            │ RSA-OAEP      │ RSA-SHA1      │ Legacy (avoid)   │   │
│  │  Basic256Sha256      │ RSA-OAEP-256  │ RSA-SHA256    │ RECOMMENDED      │   │
│  │                                                                         │   │
│  │  Security Modes:                                                        │   │
│  │  • None: No security (development only)                                 │   │
│  │  • Sign: Message integrity, no encryption                               │   │
│  │  • SignAndEncrypt: Full security (production)                           │   │
│  │                                                                         │   │
│  │  Authentication Modes:                                                  │   │
│  │  • Anonymous: No authentication                                         │   │
│  │  • UserName: Username/password                                          │   │
│  │  • Certificate: X.509 client certificate                                │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 11.2 Credential Management

Secure credential handling prevents exposure of authentication secrets. The diagram shows supported credential sources (environment variables, config files, Docker secrets) and security best practices. Credentials are never logged, and production deployments should use secret management systems:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                         CREDENTIAL MANAGEMENT                                  │
│                                                                                │
│  Current Implementation: Environment Variables / Config Files                  │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    CREDENTIAL SOURCES                                   │   │
│  │                                                                         │   │
│  │  1. Environment Variables (Recommended for containers):                 │   │
│  │     MQTT_USERNAME, MQTT_PASSWORD                                        │   │
│  │     OPC_USERNAME, OPC_PASSWORD                                          │   │
│  │                                                                         │   │
│  │  2. Config Files (For development):                                     │   │
│  │     config.yaml with opc_username, opc_password                         │   │
│  │     devices.yaml with per-device credentials                            │   │
│  │                                                                         │   │
│  │  3. Docker Secrets (Production):                                        │   │
│  │     Mount secrets as files: /run/secrets/mqtt_password                  │   │
│  │                                                                         │   │
│  │  4. External Secret Managers (Future enhancement):                      │   │
│  │     HashiCorp Vault, AWS Secrets Manager, Azure Key Vault               │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    SECURITY BEST PRACTICES                              │   │
│  │                                                                         │   │
│  │  + Credentials never logged (zerolog field exclusion)                   │   │
│  │  + Config files with restricted permissions (0600)                      │   │
│  │  + Environment variables for container deployments                      │   │
│  │  + TLS for all production connections                                   │   │
│  │  + Certificate-based auth where supported                               │   │
│  │                                                                         │   │
│  │  - Never commit credentials to source control                           │   │
│  │  - Never use insecure_skip_verify in production                         │   │
│  │  - Never share credentials across environments                          │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

### 11.3 Network Security

Network segmentation is critical for ICS security (IEC 62443). The diagram shows the recommended three-zone deployment (IT, DMZ, OT) with the gateway positioned in the DMZ. Firewall rules ensure the gateway initiates all OT connections (never the reverse), and input validation prevents injection attacks through the API:

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                           NETWORK SECURITY MODEL                               │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    NETWORK SEGMENTATION                                 │   │
│  │                                                                         │   │
│  │  Recommended Deployment:                                                │   │
│  │                                                                         │   │
│  │  ┌──────────────────┐     ┌──────────────────┐     ┌──────────────────┐ │   │
│  │  │   IT Network     │     │   DMZ Network    │     │   OT Network     │ │   │
│  │  │                  │     │                  │     │                  │ │   │
│  │  │  MQTT Broker     │◄────│  Protocol        │────►│  PLCs, Sensors   │ │   │
│  │  │  Dashboard       │     │  Gateway         │     │  OPC UA Servers  │ │   │
│  │  │  Cloud Services  │     │                  │     │  Modbus Devices  │ │   │
│  │  │                  │     │  Port 8080 only  │     │                  │ │   │
│  │  └──────────────────┘     └──────────────────┘     └──────────────────┘ │   │
│  │                                                                         │   │
│  │  Firewall Rules:                                                        │   │
│  │  • Gateway → OT: Allow Modbus/502, OPC UA/4840, S7/102                  │   │
│  │  • Gateway → IT: Allow MQTT/1883,8883                                   │   │
│  │  • IT → Gateway: Allow HTTP/8080 (management only)                      │   │
│  │  • OT → Gateway: DENY (gateway initiates all OT connections)            │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                    INPUT VALIDATION                                     │   │
│  │                                                                         │   │
│  │  Device Configuration:                                                  │   │
│  │  • ID: Alphanumeric with hyphens, required                              │   │
│  │  • Host: Valid hostname/IP, required                                    │   │
│  │  • Port: 1-65535 range                                                  │   │
│  │  • Poll interval: ≥100ms minimum                                        │   │
│  │  • Slave ID (Modbus): 1-247 range                                       │   │
│  │                                                                         │   │
│  │  Tag Configuration:                                                     │   │
│  │  • Address: Non-negative integer                                        │   │
│  │  • Register count: Validated against data type                          │   │
│  │  • Topic suffix: Sanitized for MQTT (no wildcards)                      │   │
│  │                                                                         │   │
│  │  API Requests:                                                          │   │
│  │  • JSON parsing with size limits                                        │   │
│  │  • Content-Type validation                                              │   │
│  │  • CORS headers for browser security                                    │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---