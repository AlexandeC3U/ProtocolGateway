#!/bin/bash
# =============================================================================
# OPC UA Certificate Generator
# Generates self-signed certificates for development/testing
# =============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PKI_DIR="${SCRIPT_DIR}/pki"
DAYS_VALID=365
KEY_SIZE=2048

# Application URN (should match your config)
APP_NAME="NexusEdge Protocol Gateway"
APP_URI="urn:nexusedge:protocol-gateway"
ORGANIZATION="NexusEdge"
COUNTRY="US"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== OPC UA Certificate Generator ===${NC}"
echo ""

# Check if openssl is available
if ! command -v openssl &> /dev/null; then
    echo -e "${RED}Error: openssl is required but not installed.${NC}"
    exit 1
fi

# Create PKI directories if they don't exist
echo "Creating PKI directory structure..."
mkdir -p "${PKI_DIR}/own/certs"
mkdir -p "${PKI_DIR}/own/private"
mkdir -p "${PKI_DIR}/trusted/certs"
mkdir -p "${PKI_DIR}/rejected"
mkdir -p "${PKI_DIR}/issuers/certs"

# Check if certificates already exist
if [ -f "${PKI_DIR}/own/certs/gateway.pem" ]; then
    echo -e "${YELLOW}Warning: Certificates already exist.${NC}"
    read -p "Overwrite? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Aborted."
        exit 0
    fi
fi

# Create OpenSSL config for OPC UA compliance
OPENSSL_CONF=$(mktemp)
cat > "${OPENSSL_CONF}" << EOF
[req]
default_bits = ${KEY_SIZE}
distinguished_name = req_distinguished_name
req_extensions = v3_req
x509_extensions = v3_ca
prompt = no

[req_distinguished_name]
CN = ${APP_NAME}
O = ${ORGANIZATION}
C = ${COUNTRY}

[v3_req]
keyUsage = critical, digitalSignature, keyEncipherment, dataEncipherment
extendedKeyUsage = serverAuth, clientAuth
subjectAltName = @alt_names

[v3_ca]
keyUsage = critical, digitalSignature, keyEncipherment, dataEncipherment
extendedKeyUsage = serverAuth, clientAuth
subjectAltName = @alt_names
basicConstraints = CA:FALSE

[alt_names]
URI.1 = ${APP_URI}
DNS.1 = localhost
IP.1 = 127.0.0.1
EOF

echo ""
echo "Generating private key..."
openssl genrsa -out "${PKI_DIR}/own/private/gateway.pem" ${KEY_SIZE} 2>/dev/null

echo "Generating self-signed certificate..."
openssl req -new -x509 \
    -key "${PKI_DIR}/own/private/gateway.pem" \
    -out "${PKI_DIR}/own/certs/gateway.pem" \
    -days ${DAYS_VALID} \
    -config "${OPENSSL_CONF}"

# Also generate DER format (some OPC UA stacks prefer this)
echo "Converting to DER format..."
openssl x509 -in "${PKI_DIR}/own/certs/gateway.pem" -outform DER -out "${PKI_DIR}/own/certs/gateway.der"

# Set secure permissions
echo "Setting file permissions..."
chmod 644 "${PKI_DIR}/own/certs/gateway.pem"
chmod 644 "${PKI_DIR}/own/certs/gateway.der"
chmod 600 "${PKI_DIR}/own/private/gateway.pem"

# Cleanup
rm -f "${OPENSSL_CONF}"

echo ""
echo -e "${GREEN}=== Certificate Generation Complete ===${NC}"
echo ""
echo "Generated files:"
echo "  Certificate (PEM): ${PKI_DIR}/own/certs/gateway.pem"
echo "  Certificate (DER): ${PKI_DIR}/own/certs/gateway.der"
echo "  Private Key:       ${PKI_DIR}/own/private/gateway.pem"
echo ""
echo "Certificate details:"
openssl x509 -in "${PKI_DIR}/own/certs/gateway.pem" -noout -subject -dates -fingerprint -sha256 | sed 's/^/  /'
echo ""
echo -e "${YELLOW}Remember: Never commit private keys to git!${NC}"
