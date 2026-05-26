#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
INFRA_DIR="$PROJECT_ROOT/infra"
SECRETS_DIR="$PROJECT_ROOT/secrets"
BACKEND_DIR="$PROJECT_ROOT/backend"

# Renkler
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

# Bağımlılık kontrolü
head "Bağımlılıklar kontrol ediliyor"
command -v openssl &>/dev/null || die "openssl bulunamadı: sudo apt install openssl"
command -v go     &>/dev/null  || die "go bulunamadı"
log "openssl $(openssl version | cut -d' ' -f2)"
log "go $(go version | cut -d' ' -f3)"

# Zaten oluşturulmuşsa sor
if [[ -f "$INFRA_DIR/.env" ]]; then
  echo ""
  warn "infra/.env zaten mevcut. Üzerine yazmak istiyor musun? (e/H)"
  read -r answer
  [[ "${answer,,}" == "e" ]] || { echo "İptal edildi."; exit 0; }
fi

mkdir -p "$SECRETS_DIR"

# --- bcrypt helper (backend modülünden çalışır) ---
bcrypt_hash() {
  (cd "$BACKEND_DIR" && go run "$SCRIPT_DIR/bcrypt/main.go" "$1")
}

# ─────────────────────────────────────────────
head "1. Admin Şifresi"
# Harf + rakam + özel karakter, 20 karakter
ADMIN_PASSWORD=$(openssl rand -base64 32 | tr -dc 'A-Za-z0-9!@#$%^&*' | dd bs=1 count=20 2>/dev/null)
log "Rastgele şifre üretildi"
printf "  Hash'leniyor (bcrypt cost=12, ~1 sn)..."
ADMIN_HASH=$(bcrypt_hash "$ADMIN_PASSWORD")
echo -e " ${GREEN}tamam${NC}"
log "Bcrypt hash oluşturuldu"

# ─────────────────────────────────────────────
head "2. PostgreSQL Şifresi"
DB_PASSWORD=$(openssl rand -hex 24)   # 48 hex karakter, bağlantı string'inde güvenli
log "DB şifresi üretildi (48 hex karakter)"

# ─────────────────────────────────────────────
head "3. Redis Şifresi"
REDIS_PASSWORD=$(openssl rand -hex 24)
log "Redis şifresi üretildi (48 hex karakter)"

# ─────────────────────────────────────────────
head "4. JWT EC Anahtar Çifti (P-256)"
JWT_KEY_FILE="$SECRETS_DIR/jwt_ec_primary.pem"
openssl ecparam -name prime256v1 -genkey -noout -out "$JWT_KEY_FILE"
chmod 600 "$JWT_KEY_FILE"
log "EC private key: secrets/jwt_ec_primary.pem"

# Public key'i de çıkar (servisler doğrulama için kullanabilir)
JWT_PUB_FILE="$SECRETS_DIR/jwt_ec_public.pem"
openssl ec -in "$JWT_KEY_FILE" -pubout -out "$JWT_PUB_FILE" 2>/dev/null
chmod 644 "$JWT_PUB_FILE"
log "EC public key:  secrets/jwt_ec_public.pem"

# ─────────────────────────────────────────────
head "5. infra/.env Yazılıyor"
cat > "$INFRA_DIR/.env" << EOF
# ================================================================
# ZeroTrust — Otomatik Üretilen Secretlar
# Oluşturma tarihi: $(date -u +"%Y-%m-%d %H:%M:%S UTC")
#
# BU DOSYAYI GIT'E COMMIT ETME!
# ================================================================

# ── PostgreSQL ──────────────────────────────
POSTGRES_PASSWORD=${DB_PASSWORD}
DATABASE_URL=postgres://zerotrust:${DB_PASSWORD}@postgres:5432/zerotrust_db?sslmode=disable

# ── Redis ───────────────────────────────────
REDIS_PASSWORD=${REDIS_PASSWORD}

# ── JWT ─────────────────────────────────────
JWT_PRIVATE_KEY_FILE=/run/secrets/jwt_ec_primary.pem
EOF
chmod 600 "$INFRA_DIR/.env"
log "infra/.env oluşturuldu (chmod 600)"

# ─────────────────────────────────────────────
head "6. infra/.env.admin Yazılıyor"
# Bcrypt hash içindeki $ işaretleri docker-compose YAML yorumlayıcısını karıştırır.
# env_file olarak okunan dosyalar YAML işleminden geçmez — bu yüzden admin
# credentials'ı ayrı dosyada tutuyoruz.
cat > "$INFRA_DIR/.env.admin" << ADMINEOF
INITIAL_ADMIN_EMAIL=admin@sirket.com
INITIAL_ADMIN_PASSWORD_HASH=${ADMIN_HASH}
ADMINEOF
chmod 600 "$INFRA_DIR/.env.admin"
log "infra/.env.admin oluşturuldu (chmod 600)"

# ─────────────────────────────────────────────
echo ""
echo -e "${BOLD}╔═══════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║   İLK GİRİŞ BİLGİLERİ — HEMEN KAYDET!   ║${NC}"
echo -e "${BOLD}╠═══════════════════════════════════════════╣${NC}"
echo -e "${BOLD}║${NC}  E-posta : admin@sirket.com               ${BOLD}║${NC}"
echo -e "${BOLD}║${NC}  Şifre   : ${YELLOW}${ADMIN_PASSWORD}${NC}        ${BOLD}║${NC}"
echo -e "${BOLD}╚═══════════════════════════════════════════╝${NC}"
echo ""
echo -e "${RED}${BOLD}  ⚠  Bu şifre bir daha gösterilmeyecek!${NC}"
echo -e "${RED}${BOLD}  ⚠  Şimdi kaydet, sonra terminali temizle.${NC}"
echo ""
echo -e "${BOLD}Oluşturulan dosyalar:${NC}"
echo -e "  ${GREEN}infra/.env${NC}                 → docker-compose değişken ikamesi"
echo -e "  ${GREEN}infra/.env.admin${NC}           → admin kimlik bilgileri (env_file)"
echo -e "  ${GREEN}secrets/jwt_ec_primary.pem${NC} → JWT imzalama (600 izni)"
echo -e "  ${GREEN}secrets/jwt_ec_public.pem${NC}  → JWT doğrulama (servisler için)"
echo ""
echo -e "${BOLD}Sonraki adım:${NC}"
echo -e "  cd infra && sudo docker compose up --build"
echo ""
