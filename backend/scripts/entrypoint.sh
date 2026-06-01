#!/bin/sh
set -e

export BAO_ADDR="http://127.0.0.1:8200"
export BAO_TOKEN="root"

echo "Starting OpenBao in-memory dev server..."
bao server -dev -dev-listen-address="127.0.0.1:8200" -dev-root-token-id="root" > /tmp/bao.log 2>&1 &

echo "Waiting for OpenBao to be ready..."
until wget -qO- "$BAO_ADDR/v1/sys/health" >/dev/null 2>&1; do
  sleep 0.1
done

echo "OpenBao is ready. Enabling transit secrets engine..."
bao secrets enable transit

echo "Generating database encryption key..."
bao write -f transit/keys/db-encryption-key type=aes256-gcm96

echo "Starting Go backend..."
exec "$@"
