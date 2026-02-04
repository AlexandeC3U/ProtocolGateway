# OPC UA PKI Certificate Management

This directory implements the **standard OPC UA PKI folder model** for certificate management.

## Directory Structure

```
certs/
├── pki/                          # Standard OPC UA PKI layout
│   ├── own/                      # Gateway's own identity
│   │   ├── certs/                # Public certificate(s)
│   │   │   ├── gateway.pem       # PEM format
│   │   │   └── gateway.der       # DER format (binary)
│   │   └── private/              # Private key(s) - NEVER COMMIT!
│   │       └── gateway.pem
│   │
│   ├── trusted/                  # Trusted peer certificates
│   │   └── certs/                # Approved OPC UA server certs
│   │
│   ├── rejected/                 # Untrusted certificates
│   │                             # New connections land here first
│   │                             # Admin reviews → moves to trusted/
│   │
│   └── issuers/                  # Certificate Authority certs
│       └── certs/                # Root/intermediate CA certificates
│
├── generate-certs.sh             # Certificate generation script
└── README.md                     # This file
```

## Quick Start (Development)

### 1. Generate Self-Signed Certificates

```bash
# Linux/macOS/WSL
cd certs
chmod +x generate-certs.sh
./generate-certs.sh

# Windows (Git Bash)
cd certs
bash generate-certs.sh
```

### 2. Configure Your Device

```yaml
# config/devices.yaml
devices:
  - id: secure-server
    protocol: opcua
    connection:
      opc_endpoint_url: "opc.tcp://192.168.1.100:4840"
      opc_security_policy: "Basic256Sha256"
      opc_security_mode: "SignAndEncrypt"
      opc_auth_mode: "Certificate"
      
      # Reference the PKI paths
      opc_cert_file: "certs/pki/own/certs/gateway.pem"
      opc_key_file: "certs/pki/own/private/gateway.pem"
```

## Production Deployment

### Docker: Mount Certificates at Runtime

**❌ Bad** - Baking certs into image:
```dockerfile
COPY certs /app/certs  # DON'T DO THIS
```

**✅ Good** - Mount at runtime:
```yaml
# docker-compose.yaml
services:
  gateway:
    volumes:
      - /secure/pki:/app/certs/pki:ro  # Read-only mount
```

```bash
# Or via docker run
docker run -v /secure/pki:/app/certs/pki:ro gateway
```

### Kubernetes: Use Secrets

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: gateway-opcua-certs
type: Opaque
data:
  gateway.pem: <base64-encoded-cert>
  gateway.key: <base64-encoded-key>
---
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: gateway
          volumeMounts:
            - name: certs
              mountPath: /app/certs/pki/own
              readOnly: true
      volumes:
        - name: certs
          secret:
            secretName: gateway-opcua-certs
```

### HashiCorp Vault (Enterprise)

```hcl
# Vault PKI engine for auto-generated OPC UA certs
path "pki/issue/opcua-gateway" {
  capabilities = ["create", "update"]
}
```

Init container fetches cert from Vault → writes to shared volume → gateway uses it.

## Certificate Onboarding Workflow

```
┌─────────────────────────────────────────────────────────────┐
│                    First Connection                         │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  1. Gateway connects to new OPC UA server                   │
│                                                             │
│  2. Server presents certificate                             │
│                                                             │
│  3. Certificate NOT in /trusted → saved to /rejected        │
│                                                             │
│  4. Connection fails (expected)                             │
│                                                             │
│  5. Admin reviews certificate in /rejected                  │
│     - Verify thumbprint with server admin                   │
│     - Check validity dates                                  │
│     - Validate organization/CN                              │
│                                                             │
│  6. If approved: mv /rejected/cert.pem /trusted/certs/      │
│                                                             │
│  7. Retry connection → succeeds                             │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Security Best Practices

### DO ✅

- Generate unique certificates per gateway instance
- Use RSA 2048+ or ECC P-256+ keys
- Set appropriate validity periods (1-3 years for production)
- Rotate certificates before expiry
- Store private keys with restricted permissions (`chmod 600`)
- Use volume mounts in containers (not COPY)
- Audit certificate trust changes

### DON'T ❌

- Commit private keys to git
- Share certificates across multiple gateways
- Use self-signed certs in production without proper trust chain
- Set `opc_insecure_skip_verify: true` in production
- Bake certificates into Docker images

## File Formats

| Format | Extension | Use Case |
|--------|-----------|----------|
| PEM | `.pem`, `.crt` | Human-readable, most common for config files |
| DER | `.der`, `.cer` | Binary, some OPC UA stacks prefer this |
| PKCS#12 | `.p12`, `.pfx` | Combined cert + key (password protected) |

## Troubleshooting

### Certificate Rejected by Server

1. Check that your cert is in server's trust store
2. Verify Application URI matches (in cert and config)
3. Ensure cert is not expired: `openssl x509 -in gateway.pem -noout -dates`

### Server Certificate Rejected

1. Check `/rejected` folder for the server's cert
2. Verify thumbprint with server administrator
3. Move to `/trusted/certs` if valid

### Permission Denied on Private Key

```bash
chmod 600 certs/pki/own/private/gateway.pem
```

## Architecture Reference

```
┌─────────────────────────────────────────────────────────────┐
│                    Production Setup                         │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│   ┌────────────┐                                            │
│   │ Cert Store │  (Vault / K8s Secret / Cloud KMS)          │
│   └─────┬──────┘                                            │
│         │                                                   │
│         ▼                                                   │
│   ┌────────────┐                                            │
│   │ Init Agent │  → Fetches certs → Writes to volume        │
│   └─────┬──────┘                                            │
│         │                                                   │
│         ▼                                                   │
│   ┌────────────┐                                            │
│   │  Gateway   │  → OPC UA stack reads PKI folders          │
│   └────────────┘                                            │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Compatibility

This PKI layout is compatible with:

- ✅ gopcua (Go)
- ✅ open62541 (C)
- ✅ UA-.NET Standard
- ✅ Unified Automation SDK
- ✅ Prosys OPC UA SDK
- ✅ Kepware
- ✅ Ignition

## Getting Server Certificate

To trust a specific OPC UA server, export its certificate:

1. **From the server**: Most OPC UA servers have a "Security" section where you can export the server certificate

2. **Using UaExpert**: Connect to server → Server menu → Show Certificate → Export

3. **Programmatically**: The gateway logs server cert info when `opc_insecure_skip_verify: true`

## Security Best Practices

1. **Never commit private keys** to git (already in .gitignore)
2. **Use SignAndEncrypt** mode in production
3. **Verify server certificates** - don't use `insecure_skip_verify` in production
4. **Rotate certificates** before expiration
5. **Use proper CA** for enterprise deployments

## Troubleshooting

### "Security configuration error"
- Check that both `opc_cert_file` and `opc_key_file` are specified together
- Verify files exist and are readable
- Ensure PEM format is valid

### "Failed to connect: BadSecurityChecksFailed"
- Server doesn't trust your client certificate
- Add client cert to server's trusted certificates folder
- Check security policy matches server's supported policies

### "Certificate expired"
- Generate new certificates
- Check system time is correct
