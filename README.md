# ZeroTrust

A security-focused Zero Trust authentication and authorization platform built with Go, React, Vite, PostgreSQL, Redis, and Docker.

> **Live Showcase:** [View the Interactive Showcase](https://mustafasercansak.github.io/zerotrust/)
>
> **Status:** Active development. This project is designed as a serious learning and portfolio project for modern auth/security engineering. It has not been independently audited and should not be treated as production-ready without further review, testing, and hardening.

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌──────────────┐
│ React + Vite│────▶│  Go Backend │────▶│  PostgreSQL  │
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

## Screenshots

### Dashboard Overview
![Dashboard Overview](docs/images/dashboard.png)

### Unified Settings & Session Management
![Session Management](docs/images/session_management.png)

### Multi-Factor Authentication (MFA) Setup with QR Code
![MFA Setup](docs/images/mfa_setup.png)

## Security Features


| Feature | Detail |
|---|---|
| Ed25519 JWT | EdDSA Curve25519 signed access tokens (1 min TTL) |
| DPoP (RFC 9449) | Proof-of-Possession binding access tokens to asymmetric client keys |
| httpOnly Cookies | Access + refresh tokens never exposed to JS |
| CSRF Protection | Double-submit cookie pattern (`X-CSRF-Token`) |
| Opaque Refresh Tokens | Stored as SHA-256 hashes in PostgreSQL |
| Atomic Token Rotation | `SELECT … FOR UPDATE` prevents replay race |
| JTI Blocklist | Instant revocation via Redis (auto-TTL) |
| Key Rotation | Zero-downtime via primary/secondary key slots |
| TOTP MFA | Encrypted TOTP secrets with pending setup flow & single-use backup recovery codes |
| Password Reset | Opaque reset tokens, atomic consume + password update + session revocation |
| Progressive Lockout | 1 / 5 / 30 min escalating lockout (Redis) |
| Rate Limiting | Login 10/min · global 300/min (sliding window) |
| RBAC | Roles → role_permissions → permissions |
| Service Accounts | OAuth2 `client_credentials` for M2M tokens |
| Session Management | List and revoke individual sessions from the UI |
| Session Policy | Idle timeout + absolute timeout (configurable in system settings) |
| Audit Log | Immutable event log in PostgreSQL |
| CSP / OWASP Headers | `frame-ancestors 'none'`, `object-src 'none'`, HSTS |
| bcrypt | Cost factor 12 |
| i18n | Turkish (default) / English |

## Current Scope

ZeroTrust currently includes:

- Browser login with secure cookie-based sessions
- Proactive access-token refresh
- Refresh-token rotation backed by PostgreSQL sessions
- User/session management
- Admin user and role management
- Fine-grained permissions
- Service-account credentials and M2M token issuing
- TOTP MFA setup, verification (with single-use backup recovery codes), disable, and MFA challenge during login
- Password reset flow with SMTP or development log mailer
- Audit log listing
- Turkish and English UI messages
- Docker Compose development and production profiles
- GitHub Actions CI for backend and frontend checks

Planned/ongoing hardening:

- Broader backend test coverage for MFA, password reset, session revocation, and service accounts
- More end-to-end tests around browser auth flows
- Operational security review before any real production deployment

## Quick Start

### Requirements

- Docker and Docker Compose
- Go 1.25+ (matches `backend/go.mod`; used by the secret generation script)
- OpenSSL

### 1. Generate Secrets

```bash
cd scripts
./generate-secrets.sh
```

Save the admin password shown in the output — **it will not be displayed again**.

### 2. Run

```bash
# Development-style HTTP run
make up

# Development — hot reload (Air + Vite HMR)
make dev

# Production-style local HTTPS run through nginx
make up-prod
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
│   │   ├── mfa/                # TOTP setup/verify/disable
│   │   ├── passwdreset/        # Password reset tokens and mail flow
│   │   ├── serviceaccount/     # M2M service accounts + SSE events
│   │   ├── session/            # Session store (PostgreSQL) + handler
│   │   └── user/               # User model, repo, service
│   ├── migrations/             # golang-migrate SQL files
│   └── pkg/
│       ├── database/           # Migration runner
│       ├── middleware/         # Auth, CSRF, RBAC, rate limiting, headers
│       └── validation/         # Email and password rules
├── frontend/
│   ├── src/
│   │   ├── pages/              # Auth and dashboard routes
│   │   ├── components/         # Shared UI/layout/table components
│   │   ├── lib/                # API client, token manager, useAuth hook
│   │   └── locales/            # i18n translation files (en, tr)
│   ├── index.html              # Vite entry document
│   └── vite.config.ts          # Dev server + API proxy
├── infra/
│   ├── docker-compose.yml      # Production
│   ├── docker-compose.dev.yml  # Development override
│   └── nginx/                  # TLS termination config
└── scripts/
    ├── generate-secrets.sh     # Generates .env, Ed25519 key pair, bcrypt hash
    └── bcrypt/main.go          # bcrypt helper utility
```

## API Reference

### Auth (public)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/auth/login` | Sign in → sets httpOnly cookies |
| POST | `/api/v1/auth/mfa/challenge` | Complete login when MFA is required |
| POST | `/api/v1/auth/refresh` | Rotate tokens |
| POST | `/api/v1/auth/logout` | Revoke session and clear cookies |
| POST | `/api/v1/auth/register` | Create account |
| POST | `/api/v1/auth/forgot-password` | Send a reset link when email exists |
| POST | `/api/v1/auth/reset-password` | Consume reset token and update password |
| POST | `/api/v1/auth/token` | `client_credentials` grant (M2M) |
| GET  | `/.well-known/jwks.json` | Public key set (JWKS) |

### Protected (any authenticated user)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/me` | Current user info |
| PATCH | `/api/v1/me/profile` | Update current user's name fields |
| PATCH | `/api/v1/me/locale` | Update current user's language |
| POST | `/api/v1/me/avatar` | Upload user profile picture (jpeg/png, max 2MB) |
| GET | `/api/v1/me/avatar` | Get current user's profile picture |
| GET | `/api/v1/users/{id}/avatar` | Get specific user's profile picture |
| DELETE | `/api/v1/me/avatar` | Delete current user's profile picture |
| GET | `/api/v1/sessions` | List active sessions |
| GET | `/api/v1/sessions/events` | Session change event stream |
| DELETE | `/api/v1/sessions` | Revoke all other sessions |
| DELETE | `/api/v1/sessions/{id}` | Revoke a session |
| GET | `/api/v1/mfa/status` | Read MFA status |
| POST | `/api/v1/mfa/setup` | Create a pending TOTP setup |
| POST | `/api/v1/mfa/verify` | Verify pending TOTP setup |
| POST | `/api/v1/mfa/disable` | Disable MFA with current TOTP code |

### Admin

| Method | Endpoint | Permission |
|--------|----------|------------|
| GET | `/api/v1/admin/users` | `users:read` |
| POST | `/api/v1/admin/users` | `users:create` |
| PATCH | `/api/v1/admin/users/{id}/roles` | `users:update` |
| PATCH | `/api/v1/admin/users/{id}/status` | `users:update` |
| GET | `/api/v1/admin/users/{id}/sessions` | `users:read` |
| DELETE | `/api/v1/admin/users/{id}/sessions` | `users:update` |
| DELETE | `/api/v1/admin/users/{id}/sessions/{sessionId}` | `users:update` |
| GET | `/api/v1/admin/audit` | `audit:read` |
| GET | `/api/v1/admin/service-accounts` | `service_accounts:read` |
| POST | `/api/v1/admin/service-accounts` | `service_accounts:create` |
| PATCH | `/api/v1/admin/service-accounts/{id}` | `service_accounts:update` |
| PATCH | `/api/v1/admin/service-accounts/{id}/status` | `service_accounts:update` |
| DELETE | `/api/v1/admin/service-accounts/{id}` | `service_accounts:delete` |

## Environment Variables

Core local secrets are generated by `scripts/generate-secrets.sh`; deployment-specific values such as SMTP and public origin should be configured per environment.

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |
| `REDIS_ADDR` | Redis address |
| `REDIS_PASSWORD` | Redis password |
| `JWT_PRIVATE_KEY_FILE` | Path to PKCS#8 Ed25519 private key |
| `JWT_SECONDARY_KEY_FILE` | Secondary key for zero-downtime rotation |
| `COOKIES_SECURE` | `true` in production (requires HTTPS) |
| `REGISTRATION_ENABLED` | Enables/disables public registration |
| `MFA_ENABLED` | Enables TOTP MFA when `true`; startup fails unless `MFA_ENCRYPTION_KEY` is valid; step-up admin actions fail closed when disabled |
| `MFA_ENCRYPTION_KEY` | 64 hex chars / 32 bytes for AES-256-GCM TOTP secret encryption |
| `SMTP_HOST` | SMTP host for password reset email |
| `SMTP_PORT` | SMTP port, defaults to `587` |
| `SMTP_FROM` | Sender address for password reset email |
| `SMTP_USER` | SMTP username |
| `SMTP_PASSWORD` | SMTP password |
| `PUBLIC_APP_URL` | Public frontend origin used in password reset links |
| `INITIAL_ADMIN_EMAIL` | Seed admin email |
| `INITIAL_ADMIN_PASSWORD_HASH` | bcrypt hash of admin password |## Authentication & Session Lifecycle

### 1. Browser Authentication Flow
Authentication is built on secure, token-based session cookies:
- **Access Token**: Short-lived, signed Ed25519 EdDSA JSON Web Token (1 minute TTL) set as an `httpOnly`, `Secure` (in prod), same-site cookie. It is used to authorize stateless resource requests.
- **Refresh Token**: A cryptographically random opaque string stored as a SHA-256 hash in PostgreSQL. It is exchanged periodically via `/api/v1/auth/refresh` to rotate both access and refresh tokens.
- **CSRF Protection**: Critical mutation endpoints enforce the double-submit cookie pattern. The backend issues a `csrf_token` cookie, which the frontend must send back in the `X-CSRF-Token` header.

### 2. Runtime Session Policy & System Settings
Security policies and session limits are fetched and cached dynamically from the `system_settings` table.

| Setting Key | Default | Validation / Values | Description |
|---|---:|---|---|
| `max_sessions_per_user` | `5` | `1` to `20` | Cap on the number of concurrent active sessions allowed per user account. |
| `password_complexity` | `low` | `low` / `medium` / `strong` | Rules for new user passwords (low: min 6 chars; medium: min 8 + letter/digit; strong: min 8 + mixed case/digit/symbol). |
| `max_login_attempts` | `5` | `1` to `20` | Limit of consecutive failed attempts before triggering progressive temporary lockout. |
| `global_mfa_required` | `false` | `true` / `false` | Force all accounts to complete TOTP setup and verification upon sign-in. |
| `session_idle_timeout_seconds` | `300` | `60` to `3600` (seconds) | Inactivity timeout window (default 5 minutes) for regular users. |
| `session_idle_timeout_seconds_admin` | `180` | `60` to `1800` (seconds) | Stricter inactivity timeout window (default 3 minutes) for administrators. |
| `session_absolute_timeout_seconds` | `28800` | `1800` to `172800` (seconds) | Hard cap on session lifetime (default 8 hours), forcing re-authentication once reached. |

- Access token TTL is strictly locked to 1 minute.
- Idle timeouts prevent inactive devices from maintaining prolonged sessions.
- Absolute timeouts prevent credential persistence past shift durations.

### 3. Logout & Revocation Semantics
- **Logout (`/api/v1/auth/logout`)**: Clears client-side cookies and mark the corresponding refresh token as revoked in the database.
- **Single Session Revocation**: Users and administrators can list active devices/sessions and revoke individual entries. Revoked sessions immediately fail token validation or rotation.
- **Revoke All Others**: Terminate all active sessions associated with the user account, excluding the initiator's current connection.
- **Token Reuse Detection**: If an old rotated refresh token is re-submitted outside a brief network-race grace period, the system assumes theft, instantly invalidates all active sessions for that user, and emits a critical audit alert.

### 4. Multi-Factor Authentication (MFA)
- **Prerequisites**: Enabled globally by setting `MFA_ENABLED=true` and supplying a 32-byte hexadecimal key in `MFA_ENCRYPTION_KEY` (used for encrypting secret seeds with AES-256-GCM).
- **MFA Challenge**: When enabled, logging in triggers an MFA challenge that generates a temporary 5-minute single-use `mfa_token` cookie. The login is only finalized when the user submits their valid TOTP code.
- **Backup Recovery Codes**: During initial setup, the system generates 8 single-use recovery codes (`xxxx-xxxx-xxxx`) hashed via `bcrypt` and stored in the database. Users can enter a backup recovery code during login/verification challenges to bypass the TOTP prompt. Upon successful validation, the code is permanently deleted.
- **Step-Up Verification**: Destructive administrator operations (such as modifying user roles/status, revoking user sessions, or creating/deleting service accounts) require **recent** MFA verification. If no verified marker exists for the session in Redis within the last 10 minutes, the action is blocked (`mfa_required` error) until the administrator completes an inline TOTP step-up challenge.

### 5. Service Account (M2M) Auth Model
Designed for secure machine-to-machine integrations:
- **Credentials**: Created by administrators with fine-grained scopes. The system outputs a unique Client ID and a 32-byte cryptographically secure Client Secret (hashed with bcrypt cost factor 12 before database insertion).
- **Token Issuance**: Service accounts retrieve access tokens via `/api/v1/auth/token` by utilizing the OAuth 2.0 `client_credentials` grant.
- **Lifecycle & Enforcement**: Token generation fails immediately if the service account has expired or been marked inactive. The resulting M2M Bearer tokens are restricted strictly to the scopes assigned to the account.

### 6. DPoP (Demonstrating Proof-of-Possession, RFC 9449)
For machine-to-machine integrations, the backend enforces DPoP:
- **Proof Validation**: The client signs an ephemeral JWT proof using a private key and includes it in the `DPoP` request header. The server validates this proof's signature, HTTP method (`htm`), and path (`htu`).
- **Token Binding**: The backend extracts the public key from the proof, computes its thumbprint (JWK JKT, RFC 7638), and binds the issued access token by embedding this thumbprint as a confirmation claim (`cnf.jkt`).
- **Access Verification**: On protected endpoints, the middleware validates the client's DPoP proof signature and ensures the public key's thumbprint matches the `cnf.jkt` embedded in the access token. This makes the token useless if stolen without the corresponding private key.

## Development Checks

```bash
# Backend
cd backend
go test ./...
go vet ./...

# Frontend
cd frontend
npx tsc --noEmit
npm run build

# Or use the project helpers
make test
make lint
```

GitHub Actions runs backend build/vet/test and frontend type-check/build on pushes and pull requests to `main`/`master`.

## Security Notes

- Do not commit `infra/.env`, `infra/.env.admin`, `secrets/`, TLS certificates, or private keys.
- If any secret has ever been shown publicly, regenerate it before making the repository public.
- Production mode should run behind HTTPS with `COOKIES_SECURE=true`.
- `PUBLIC_APP_URL` must be a trusted application origin; password reset links are generated from this config value, not from request headers.
- MFA is disabled unless `MFA_ENABLED=true`; any configured `MFA_ENCRYPTION_KEY` is ignored while disabled. When enabled, `MFA_ENCRYPTION_KEY` must be a valid 64-character hex / 32-byte key or the backend fails to start. Step-up protected admin actions fail closed while MFA is disabled.
- See [SECURITY.md](SECURITY.md) for vulnerability reporting guidance.

## Makefile Commands

```
make secrets   Generate secrets (infra/.env, Ed25519 key pair)
make up        Start in development-style HTTP mode
make dev       Start in development mode with hot reload
make up-prod   Start production-style HTTPS mode via nginx
make down      Stop all services
make down-v    Stop and delete volumes (resets database)
make test      Run backend tests
make lint      Run go vet + tsc
make clean     Remove Docker images and volumes
```

## License

Copyright (c) 2026 Mustafa Sercan Sak

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
