#!/usr/bin/env bash
# Generates a persistent EC P-256 private key for JWT signing.
# Run once; store the result securely and never commit it.
set -euo pipefail

SECRETS_DIR="$(cd "$(dirname "$0")/../.." && pwd)/secrets"
mkdir -p "$SECRETS_DIR"

KEY_FILE="$SECRETS_DIR/jwt_ec_primary.pem"

if [ -f "$KEY_FILE" ]; then
  echo "Key already exists at $KEY_FILE — skipping generation."
  exit 0
fi

openssl ecparam -genkey -name prime256v1 -noout \
  | openssl pkcs8 -topk8 -nocrypt -out "$KEY_FILE"

chmod 600 "$KEY_FILE"
echo "JWT signing key written to $KEY_FILE"
echo "Set JWT_PRIVATE_KEY_FILE=/run/secrets/jwt_ec_primary in your environment."
