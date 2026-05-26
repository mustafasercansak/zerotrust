#!/usr/bin/env bash
# Generates a self-signed TLS certificate for local HTTPS testing.
# For production, replace certs/server.crt and certs/server.key with your real CA-signed cert.
set -euo pipefail

CERT_DIR="$(cd "$(dirname "$0")/.." && pwd)/certs"
mkdir -p "$CERT_DIR"

DOMAIN="${1:-localhost}"

openssl req -x509 -newkey ec \
  -pkeyopt ec_paramgen_curve:P-256 \
  -keyout "$CERT_DIR/server.key" \
  -out    "$CERT_DIR/server.crt" \
  -days   365 \
  -nodes \
  -subj   "/CN=${DOMAIN}" \
  -addext "subjectAltName=DNS:${DOMAIN},DNS:*.${DOMAIN},IP:127.0.0.1"

echo "Certificate written to $CERT_DIR"
echo "  crt: $CERT_DIR/server.crt"
echo "  key: $CERT_DIR/server.key"
echo ""
echo "To trust it in Chrome/Firefox: add server.crt to your browser's trusted CAs."
