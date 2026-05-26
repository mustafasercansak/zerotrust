# ZeroTrust

A Zero Trust authentication and authorization platform built with Go and Next.js — fully containerized, production-hardened.

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
                    │  (rate limit │
                    │  + JTI list) │
                    └──────────────┘
```

Next.js proxies all `/api/*` requests to the backend, so cookies work on the same origin without HTTPS in development.

## Security Features

| Feature | Detail |
|---|---|
| ES256 JWT | ECDSA P-256 signed access tokens (1 min TTL) |
| httpOnly Cookies | Access + refresh tokens never exposed to JS |
| CSRF Protection | Double-submit cookie pattern (`X-CSRF-Token`) |
| Opaque Refresh Tokens | Stored as SHA-256 hashes in PostgreSQL |
| Atomic Token Rotation | `SELECT … FOR UPDATE` prevents replay race |
| JTI Blocklist | Instant revocation via Redis (auto-TTL) |
| Key Rotation | Zero-downtime via primary/secondary key slots |
| Progressive Lockout | 1 / 5 / 30 min escalating lockout (Redis) |
| Rate Limiting | Login 10/min · global 300/min (sliding window) |
| RBAC | Roles → role_permissions → permissions |
| Service Accounts | OAuth2 `client_credentials` for M2M tokens |
| Session Management | List and revoke individual sessions from the UI |
| Audit Log | Immutable event log in PostgreSQL |
| CSP / OWASP Headers | `frame-ancestors 'none'`, `object-src 'none'`, HSTS |
| bcrypt | Cost factor 12 |
| i18n | Turkish (default) / English |

## Quick Start

### Requirements

- Docker and Docker Compose
- Go 1.22+ (for secret generation script)
- OpenSSL

### 1. Generate Secrets

```bash
cd scripts
./generate-secrets.sh
```

Save the admin password shown in the output — **it will not be displayed again**.

### 2. Run

```bash
# Production
cd infra && sudo docker compose up --build

# Development — hot reload (Air + Next.js HMR)
cd infra && sudo docker compose -f docker-compose.yml -f docker-compose.dev.yml watch
```

| Service  | URL                          |
|----------|------------------------------|
| Frontend | http://localhost:3000        |
| Backend  | http://localhost:8080        |
| Health   | http://localhost:8080/health |
| JWKS     | http://localhost:8080/.well-known/jwks.json |

## Project Structure

```
zerotrust/
├── backend/
│   ├── cmd/server/             # Entry point, router, config
│   ├── internal/
│   │   ├── admin/              # User management handler
│   │   ├── audit/              # Audit log repository
│   │   ├── auth/               # JWT, keys, token service, handler
│   │   ├── serviceaccount/     # M2M service accounts + SSE events
│   │   ├── session/            # Session store (PostgreSQL) + handler
│   │   └── user/               # User model, repo, service
│   ├── migrations/             # golang-migrate SQL files
│   └── pkg/
│       ├── database/           # Migration runner
│       ├── middleware/         # Auth, CSRF, RBAC, rate limiting, headers
│       └── validation/         # Email and password rules
├── frontend/
│   ├── app/[locale]/           # Next.js App Router (TR/EN)
│   │   ├── auth/               # Login / register pages
│   │   └── dashboard/          # Dashboard, users, sessions, service accounts
│   ├── lib/                    # API client, token manager, useAuth hook
│   └── messages/               # i18n translation files (en, tr)
├── infra/
│   ├── docker-compose.yml      # Production
│   ├── docker-compose.dev.yml  # Development override
│   └── nginx/                  # TLS termination config
└── scripts/
    ├── generate-secrets.sh     # Generates .env, EC key pair, bcrypt hash
    └── bcrypt/main.go          # bcrypt helper utility
```

## API Reference

### Auth (public)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/auth/login` | Sign in → sets httpOnly cookies |
| POST | `/api/v1/auth/refresh` | Rotate tokens |
| POST | `/api/v1/auth/logout` | Revoke session and clear cookies |
| POST | `/api/v1/auth/register` | Create account |
| POST | `/api/v1/auth/token` | `client_credentials` grant (M2M) |
| GET  | `/.well-known/jwks.json` | Public key set (JWKS) |

### Protected (any authenticated user)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/me` | Current user info |
| GET | `/api/v1/sessions` | List active sessions |
| DELETE | `/api/v1/sessions/{id}` | Revoke a session |

### Admin

| Method | Endpoint | Permission |
|--------|----------|------------|
| GET | `/api/v1/admin/users` | `users:read` |
| POST | `/api/v1/admin/users` | `users:write` |
| PATCH | `/api/v1/admin/users/{id}/roles` | `users:write` |
| GET | `/api/v1/admin/service-accounts` | `service_accounts:read` |
| POST | `/api/v1/admin/service-accounts` | `service_accounts:write` |
| PATCH | `/api/v1/admin/service-accounts/{id}/status` | `service_accounts:write` |
| DELETE | `/api/v1/admin/service-accounts/{id}` | `service_accounts:delete` |

## Environment Variables

Generated automatically by `scripts/generate-secrets.sh`.

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |
| `REDIS_ADDR` | Redis address |
| `REDIS_PASSWORD` | Redis password |
| `JWT_PRIVATE_KEY_FILE` | Path to PKCS#8 EC private key |
| `JWT_SECONDARY_KEY_FILE` | Secondary key for zero-downtime rotation |
| `COOKIES_SECURE` | `true` in production (requires HTTPS) |
| `INITIAL_ADMIN_EMAIL` | Seed admin email |
| `INITIAL_ADMIN_PASSWORD_HASH` | bcrypt hash of admin password |

## Makefile Commands

```
make secrets   Generate secrets (infra/.env, EC key pair)
make up        Start in production mode
make dev       Start in development mode with hot reload
make down      Stop all services
make down-v    Stop and delete volumes (resets database)
make test      Run backend tests
make lint      Run go vet + tsc
make clean     Remove Docker images and volumes
```

## License

Copyright (C) 2026 Mustafa Sercan Şak

This program is free software: you can redistribute it and/or modify it under the terms of the GNU Affero General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version. See [LICENSE](LICENSE) for the full terms.
