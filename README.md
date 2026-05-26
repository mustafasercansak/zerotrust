# ZeroTrust

A Zero Trust authentication and authorization platform built with Go backend and Next.js frontend, fully containerized with Docker.

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌──────────────┐
│  Next.js 15 │────▶│  Go Backend │────▶│  PostgreSQL  │
│  (port 3000)│     │  (port 8080)│     │  (port 5432) │
└─────────────┘     └──────┬──────┘     └──────────────┘
                           │
                           ▼
                    ┌──────────────┐
                    │    Redis     │
                    │  (port 6379) │
                    └──────────────┘
```

## Security Features

- **ES256 JWT** — ECDSA P-256 signed access tokens (1 minute TTL)
- **JTI Blocklist** — Instant token revocation via Redis
- **Key Rotation** — Zero-downtime key rotation (primary/secondary)
- **Opaque Refresh Tokens** — Stored as SHA-256 hashes in Redis (7 day TTL)
- **Proactive Refresh** — Automatic token renewal at 80% of TTL
- **Progressive Lockout** — Escalating account lockout: 1 / 5 / 30 minutes
- **Rate Limiting** — Login 10/min, global 300/min (Redis sliding window)
- **RBAC** — Role-based access control (admin / user)
- **OWASP Headers** — HSTS, CSP, X-Frame-Options, Permissions-Policy
- **bcrypt cost=12** — Password hashing
- **i18n** — Turkish (default) / English

## Quick Start

### Requirements

- Docker and Docker Compose
- Go 1.25+ (for secret generation)
- OpenSSL

### 1. Generate Secrets

```bash
cd scripts
./generate-secrets.sh
```

Save the admin password shown in the output — **it will not be displayed again**.

### 2. Run (Production)

```bash
cd infra
sudo docker compose up --build
```

| Service   | URL                          |
|-----------|------------------------------|
| Frontend  | http://localhost:3000        |
| Backend   | http://localhost:8080        |
| Health    | http://localhost:8080/health |

### 3. Run (Development — Hot Reload)

```bash
cd infra
sudo docker compose -f docker-compose.yml -f docker-compose.dev.yml watch
```

- Go file changes → **Air** recompiles and restarts automatically
- Frontend file changes → **Next.js HMR** updates the browser instantly

## Project Structure

```
zerotrust/
├── backend/
│   ├── cmd/server/         # Application entry point
│   ├── internal/
│   │   ├── admin/          # User management handler
│   │   ├── audit/          # Audit log
│   │   ├── auth/           # JWT, token, service layer
│   │   └── user/           # User model / repo / service
│   ├── migrations/         # golang-migrate SQL files
│   └── pkg/
│       ├── database/       # Migration runner
│       ├── middleware/      # Auth, RBAC, rate limiting, security headers
│       └── validation/     # Email and password rules
├── frontend/
│   ├── app/[locale]/       # Next.js App Router (TR/EN)
│   │   ├── auth/           # Login page
│   │   └── dashboard/      # Dashboard and user management
│   ├── lib/                # API client, token manager, useAuth hook
│   └── messages/           # i18n translation files
├── infra/
│   ├── docker-compose.yml      # Production
│   └── docker-compose.dev.yml  # Development override
└── scripts/
    ├── generate-secrets.sh     # Secret generator
    └── bcrypt/main.go          # bcrypt helper
```

## API Reference

### Auth

| Method | Endpoint              | Description               |
|--------|-----------------------|---------------------------|
| POST   | /api/v1/auth/login    | Sign in                   |
| POST   | /api/v1/auth/refresh  | Refresh tokens            |
| POST   | /api/v1/auth/logout   | Sign out (revokes tokens) |

### Protected

| Method | Endpoint                       | Role  | Description       |
|--------|--------------------------------|-------|-------------------|
| GET    | /api/v1/me                     | —     | Current user info |
| GET    | /api/v1/admin/users            | admin | List all users    |
| POST   | /api/v1/admin/users            | admin | Create a user     |
| PATCH  | /api/v1/admin/users/{id}/roles | admin | Update user roles |

## Environment Variables

All values are generated automatically by `scripts/generate-secrets.sh`.

| Variable                      | Description                        |
|-------------------------------|------------------------------------|
| `POSTGRES_PASSWORD`           | PostgreSQL password                |
| `DATABASE_URL`                | PostgreSQL connection URL          |
| `REDIS_PASSWORD`              | Redis password                     |
| `JWT_PRIVATE_KEY_FILE`        | Path to EC private key             |
| `INITIAL_ADMIN_EMAIL`         | Seed admin email                   |
| `INITIAL_ADMIN_PASSWORD_HASH` | bcrypt hash (loaded via env_file)  |

## Development

```bash
# Run backend tests
cd backend && go test ./...

# Backend static analysis
cd backend && go vet ./...

# Frontend type check
cd frontend && npx tsc --noEmit

# Regenerate secrets and reset database
make secrets
make down-v
make up
```

## Makefile Commands

```
make secrets   Generate secrets (infra/.env, EC key pair)
make up        Start in production mode
make down      Stop all services
make down-v    Stop and delete volumes (resets database)
make dev       Start in development mode with hot reload
make test      Run backend tests
make lint      Run go vet + tsc
make clean     Remove Docker images and volumes
```
