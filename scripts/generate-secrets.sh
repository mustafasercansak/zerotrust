#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
INFRA_DIR="$PROJECT_ROOT/infra"
SECRETS_DIR="$PROJECT_ROOT/secrets"
BACKEND_DIR="$PROJECT_ROOT/backend"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BOLD='\033[1m'
CYAN='\033[0;36m'
NC='\033[0m'

log()  { echo -e "  ${GREEN}✓${NC} $1"; }
warn() { echo -e "  ${YELLOW}⚠${NC}  $1"; }
die()  { echo -e "  ${RED}✗${NC} $1"; exit 1; }
head() { echo -e "\n${CYAN}${BOLD}▶ $1${NC}"; }

echo ""
echo -e "${BOLD}╔═══════════════════════════════════════╗${NC}"
echo -e "${BOLD}║    ZeroTrust Secret Generator  v1.0   ║${NC}"
echo -e "${BOLD}╚═══════════════════════════════════════╝${NC}"

# Dependency check
head "Checking dependencies"
command -v openssl &>/dev/null || die "openssl not found: sudo apt install openssl"
command -v go     &>/dev/null  || die "go not found"
log "openssl $(openssl version | cut -d' ' -f2)"
log "go $(go version | cut -d' ' -f3)"

# Ask if already generated
if [[ -f "$INFRA_DIR/.env" ]]; then
  echo ""
  warn "infra/.env already exists. Do you want to overwrite it? (y/N)"
  read -r answer
  [[ "${answer,,}" == "y" ]] || { echo "Cancelled."; exit 0; }
fi

mkdir -p "$SECRETS_DIR"

# --- bcrypt helper (runs from the backend module) ---
bcrypt_hash() {
  (cd "$BACKEND_DIR" && go run "$SCRIPT_DIR/bcrypt/main.go" "$1")
}

# ─────────────────────────────────────────────
head "1. Admin Password"
# Letter + digit + special character, 20 characters
ADMIN_PASSWORD=$(openssl rand -base64 32 | tr -dc 'A-Za-z0-9!@#$%^&*' | dd bs=1 count=20 2>/dev/null)
log "Random password generated"
printf "  Hashing (bcrypt cost=12, ~1s)..."
ADMIN_HASH=$(bcrypt_hash "$ADMIN_PASSWORD")
echo -e " ${GREEN}done${NC}"
log "Bcrypt hash created"

# ─────────────────────────────────────────────
head "2. PostgreSQL Password"
DB_PASSWORD=$(openssl rand -hex 24)   # 48 hex characters, safe for connection string
log "DB password generated (48 hex characters)"

# ─────────────────────────────────────────────
head "3. Redis Password"
REDIS_PASSWORD=$(openssl rand -hex 24)
log "Redis password generated (48 hex characters)"

# ─────────────────────────────────────────────
head "4. JWT Ed25519 Key Pair"
JWT_KEY_FILE="$SECRETS_DIR/jwt_primary.pem"
openssl genpkey -algorithm ed25519 -out "$JWT_KEY_FILE"
chmod 600 "$JWT_KEY_FILE"
log "Ed25519 private key: secrets/jwt_primary.pem"

# Extract public key as well (can be used by services for verification)
JWT_PUB_FILE="$SECRETS_DIR/jwt_public.pem"
openssl pkey -in "$JWT_KEY_FILE" -pubout -out "$JWT_PUB_FILE" 2>/dev/null
chmod 644 "$JWT_PUB_FILE"
log "Ed25519 public key:  secrets/jwt_public.pem"

# ─────────────────────────────────────────────
head "5. Writing infra/.env"
cat > "$INFRA_DIR/.env" << EOF
# ================================================================
# ZeroTrust — Automatically Generated Secrets
# Generation date: $(date -u +"%Y-%m-%d %H:%M:%S UTC")
#
# DO NOT COMMIT THIS FILE TO GIT!
# ================================================================

# ── PostgreSQL ──────────────────────────────
POSTGRES_PASSWORD=${DB_PASSWORD}
DATABASE_URL=postgres://zerotrust:${DB_PASSWORD}@postgres:5432/zerotrust_db?sslmode=disable

# ── Redis ───────────────────────────────────
REDIS_PASSWORD=${REDIS_PASSWORD}

# ── JWT ─────────────────────────────────────
JWT_PRIVATE_KEY_FILE=/run/secrets/jwt_primary.pem

# ── MFA ─────────────────────────────────────
MFA_ENABLED=true
MFA_ENCRYPTION_KEY=$(openssl rand -hex 32)

# ── Connection Pool Tuning (defaults shown) ─
# DATABASE_MAX_CONNS=20
# DATABASE_MIN_CONNS=2
# DATABASE_CONN_TIMEOUT=5s
# REDIS_POOL_SIZE=10
EOF
chmod 600 "$INFRA_DIR/.env"
log "infra/.env created (chmod 600)"

# ─────────────────────────────────────────────
head "6. Writing infra/.env.admin"
# Dollar signs ($) in bcrypt hash confuse docker-compose YAML parser.
# We escape them by doubling them ($ -> $$) so docker-compose interpolates them to single dollars.
ADMIN_HASH_ESCAPED=$(echo "$ADMIN_HASH" | sed 's/\$/\$\$/g')
cat > "$INFRA_DIR/.env.admin" << ADMINEOF
INITIAL_ADMIN_EMAIL=admin@company.com
INITIAL_ADMIN_PASSWORD_HASH=${ADMIN_HASH_ESCAPED}
ADMINEOF
chmod 600 "$INFRA_DIR/.env.admin"
log "infra/.env.admin created (chmod 600)"

# ─────────────────────────────────────────────
echo ""
echo -e "${BOLD}╔═══════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║   INITIAL LOGIN INFO — SAVE IT NOW!     ║${NC}"
echo -e "${BOLD}╠═══════════════════════════════════════════╣${NC}"
echo -e "${BOLD}║${NC}  Email   : admin@company.com              ${BOLD}║${NC}"
echo -e "${BOLD}║${NC}  Password: ${YELLOW}${ADMIN_PASSWORD}${NC}        ${BOLD}║${NC}"
echo -e "${BOLD}╚═══════════════════════════════════════════╝${NC}"
echo ""
echo -e "${RED}${BOLD}  ⚠  This password will not be shown again!${NC}"
echo -e "${RED}${BOLD}  ⚠  Save it now, then clear your terminal.${NC}"
echo ""
echo -e "${BOLD}Generated files:${NC}"
echo -e "  ${GREEN}infra/.env${NC}                 → docker-compose variable substitution"
echo -e "  ${GREEN}infra/.env.admin${NC}           → admin credentials (env_file)"
echo -e "  ${GREEN}secrets/jwt_primary.pem${NC}    → JWT signing private key (chmod 600)"
echo -e "  ${GREEN}secrets/jwt_public.pem${NC}     → JWT public key (for verifying services)"
echo ""
echo -e "${BOLD}Next step:${NC}"
echo -e "  cd infra && sudo docker compose up --build"
echo ""
