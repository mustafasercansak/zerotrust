# ZeroTrust

A security-focused Zero Trust authentication and authorization platform built with Go, React, Vite, PostgreSQL, Redis, and Docker.

> **Live Showcase:** [English](https://mustafasercansak.github.io/zerotrust/) · [Türkçe](https://mustafasercansak.github.io/zerotrust/?lang=tr)
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

> Run `make screenshots ADMIN_PASSWORD=<your-password>` to refresh all screenshots automatically.

### Login
![Login Page](docs/images/login.png)

### Dashboard Overview
![Dashboard Overview](docs/images/dashboard.png)

### Unified Settings & Session Management
![Session Management](docs/images/session_management.png)

### Multi-Factor Authentication (MFA) Setup with QR Code
![MFA Setup](docs/images/mfa_setup.png)

### Passkey Registration and Management
![Passkey Registration and Management](docs/images/passkey_management.png)

### Passwordless Passkey Login
![Passwordless Passkey Login](docs/images/passkey_login.png)

### Admin Security Dashboard
![Admin Security Dashboard](docs/images/security_dashboard.png)

### OIDC Identity Provider Configuration
![OIDC Clients](docs/images/oidc_clients.png)

### Audit Log
![Audit Log](docs/images/audit.png)

### Service Accounts (M2M)
![Service Accounts](docs/images/service_accounts.png)

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
| WebAuthn Passkeys | FIDO2 passkeys as a phishing-resistant second factor **and** passwordless (usernameless) login via discoverable credentials |
| Password Reset | Opaque reset tokens, atomic consume + password update + session revocation |
| Progressive Lockout | 1 / 5 / 30 min escalating lockout (Redis) |
| Rate Limiting | Login 10/min · OIDC token/authorize/revoke/introspect/end_session 30/min · userinfo 100/min · global 300/min (sliding window, per IP) |
| RBAC | Roles → role_permissions → permissions |
| Service Accounts | OAuth2 `client_credentials` for M2M tokens |
| Session Management | List and revoke individual sessions from the UI |
| Session Policy | Idle timeout + absolute timeout (configurable in system settings) |
| Audit Log | Immutable event log in PostgreSQL |
| Admin Security Dashboard | Authentication trends, lockouts, anomalies, active sessions, login geography, and failed-login sources across 24-hour, 7-day, and 30-day ranges |
| CSP / OWASP Headers | `frame-ancestors 'none'`, `object-src 'none'`, HSTS |
| bcrypt | Cost factor 12 |
| OIDC Provider | Standards-compliant OpenID Connect IdP — Authorization Code + PKCE (S256, RFC 7636), refresh token grant (RFC 6749 §6) with rotating opaque tokens, token revocation (RFC 7009), introspection (RFC 7662), `max_age` re-auth (OIDC Core §3.1.2.1), `prompt=none/login`, `offline_access` scope, MFA step-up on consent, admin client management with step-up MFA |
| i18n | Turkish (default) / English, locale preference stored server-side |
| Locale Change Audit | Language changes recorded with `from`/`to` values; triggers opt-in security email |
| Security Alert Banners | Persistent in-app alerts (not toasts) for new-device sign-in, session termination, locale change from another session, and login anomalies detected in the last 24 h |
| Per-User Notification Preferences | Each user can toggle security alert emails (new device, lockout, locale change) independently of system-wide settings |

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
- WebAuthn passkeys (FIDO2): register/list/remove credentials, passkey second-factor login, and passwordless (usernameless) login with discoverable credentials
- Password reset flow with SMTP or development log mailer
- Audit log listing, CSV/JSON export (up to 10 000 rows), and an aggregated administrator security dashboard
- Platform security posture summary on the admin home page (users without MFA, inactive users)
- System health monitoring — `/api/v1/health` (public, load-balancer ready) and `/api/v1/admin/health` (DB + Redis pool stats)
- Password strength indicator on all password fields (pure regex, no external dependency)
- OIDC Identity Provider — Authorization Code + PKCE (S256, RFC 7636), refresh token grant with single-use rotating opaque tokens (RFC 6749 §6, `offline_access` scope), token revocation (RFC 7009), introspection (RFC 7662), `max_age` re-auth enforcement (OIDC Core §3.1.2.1), `prompt=none/login`, MFA step-up on consent, rate-limited endpoints, admin client management with step-up MFA
- Turkish and English UI, with language preference stored server-side and audited on change
- Security alert banners — persistent in-app alerts for new-device logins, session terminations, locale changes from another session, and login anomalies detected in the last 24 hours
- Per-user email notification preferences (toggle security alert emails from profile settings)
- Frontend route-level code splitting — DataGrid, React/ReactDOM, MUI, and i18n loaded in separate lazy chunks for fast initial load
- Docker Compose development and production profiles
- GitHub Actions CI for backend and frontend checks; automated showcase deployment to GitHub Pages

Planned/ongoing hardening:

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

### Backend Integration Tests

The easiest option starts disposable PostgreSQL and Redis containers, runs the
complete backend suite, and removes the containers afterward:

```bash
make test
```

`make test-local` performs the same disposable-service workflow explicitly.

When supplying your own database, `TEST_DATABASE_URL` must reference a dedicated
database whose name is explicitly test-only, such as `zerotrust_test`. Test
startup rejects development and production-style names before running migrations
or fixture cleanup.

```bash
TEST_DATABASE_URL='postgres://zerotrust:password@localhost:5432/zerotrust_test?sslmode=disable' \
go test -count=1 -p 1 ./...
```

### Frontend Unit Tests

```bash
cd frontend && npm test
```

### E2E Tests (Playwright)

E2E tests use the **system Chrome** already installed on the machine — no browser download required.

**Requirements:**
- Vite dev server running on `:3000` (started automatically if not running)
- Backend running on `:8080`
- Two test users without MFA: one with `user` role, one with `admin` role

```bash
cd frontend

# Run all 20 E2E tests
E2E_USER_EMAIL=e2e_user@example.com  E2E_USER_PASSWORD=yourpass \
E2E_ADMIN_EMAIL=e2e_admin@example.com E2E_ADMIN_PASSWORD=yourpass \
npm run test:e2e

# Interactive UI mode
npm run test:e2e:ui
```

**Test coverage:**

| Suite | Tests | Requires |
|---|---|---|
| Login page UI | 3 | Vite only |
| Protected route redirects | 6 | Backend |
| Wrong credentials toast | 1 | Backend |
| Regular user flow | 4 | Backend + user creds |
| Admin flow | 4 | Backend + admin creds |
| Settings page — regular user | 6 | Backend + user creds |
| Settings page — admin | 2 | Backend + admin creds |
| MFA page | 4 | Backend + user creds |

Auth state is saved once per role via `storageState` (not repeated per test) to stay below the backend rate limit.

## Project Structure

```
zerotrust/
├── backend/
│   ├── cmd/server/             # Entry point, router, config
│   ├── internal/
│   │   ├── admin/              # User management handler
│   │   ├── audit/              # Audit log and security dashboard aggregation
│   │   ├── auth/               # JWT, keys, token service, handler
│   │   ├── mfa/                # TOTP setup/verify/disable
│   │   ├── oidc/               # OIDC/OAuth2 provider — clients, auth codes, token exchange
│   │   ├── passwdreset/        # Password reset tokens and mail flow
│   │   ├── serviceaccount/     # M2M service accounts + SSE events
│   │   ├── session/            # Session store (PostgreSQL) + handler
│   │   ├── user/               # User model, repo, service
│   │   └── webauthn/           # FIDO2 passkey registration + login (2FA & passwordless)
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
| POST | `/api/v1/auth/webauthn/login/begin` | Passkey second-factor assertion options (needs `mfa_token`) |
| POST | `/api/v1/auth/webauthn/login/finish` | Verify passkey second-factor assertion → finalize login |
| POST | `/api/v1/auth/webauthn/passwordless/begin` | Passwordless assertion options (discoverable credentials; returns `ceremony_id`) |
| POST | `/api/v1/auth/webauthn/passwordless/finish` | Verify passwordless assertion → sets httpOnly cookies |
| POST | `/api/v1/auth/refresh` | Rotate tokens |
| POST | `/api/v1/auth/logout` | Revoke session and clear cookies |
| POST | `/api/v1/auth/register` | Create account |
| POST | `/api/v1/auth/forgot-password` | Send a reset link when email exists |
| POST | `/api/v1/auth/reset-password` | Consume reset token and update password |
| POST | `/api/v1/auth/token` | `client_credentials` grant (M2M) |
| GET  | `/.well-known/jwks.json` | Public key set (JWKS) |

### OIDC Provider (public)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/.well-known/openid-configuration` | OpenID Connect discovery document |
| GET | `/.well-known/jwks.json` | JSON Web Key Set (shared with auth) |
| GET | `/oauth2/authorize` | Authorization endpoint — redirects to login then consent; supports `prompt`, `max_age`, `code_challenge` |
| POST | `/oauth2/token` | Token endpoint — `authorization_code` and `refresh_token` grants |
| POST | `/oauth2/consent` | Consent submission — approves or denies a pending authorization request |
| POST | `/oauth2/revoke` | Token revocation (RFC 7009) — revokes access or refresh tokens |
| GET | `/oauth2/userinfo` | Returns OIDC claims for a Bearer access token |
| POST | `/oauth2/introspect` | Token introspection (RFC 7662) — returns active status and claims |
| GET, POST | `/oauth2/end_session` | RP-Initiated Logout — revokes session, clears cookies, redirects to `post_logout_redirect_uri` |
| GET | `/oauth2/clients/{client_id}` | Public client metadata (name + allowed scopes) — used by the consent page |

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
| PATCH | `/api/v1/me/notifications` | Toggle security alert email preference |
| GET | `/api/v1/me/audit` | Personal security event log (auth, MFA, session, locale changes) |
| GET | `/api/v1/sessions` | List active sessions |
| GET | `/api/v1/sessions/events` | Session change event stream |
| DELETE | `/api/v1/sessions` | Revoke all other sessions |
| DELETE | `/api/v1/sessions/{id}` | Revoke a session |
| GET | `/api/v1/mfa/status` | Read MFA status |
| POST | `/api/v1/mfa/setup` | Create a pending TOTP setup |
| POST | `/api/v1/mfa/verify` | Verify pending TOTP setup |
| POST | `/api/v1/mfa/disable` | Disable MFA with current TOTP code |
| POST | `/api/v1/webauthn/register/begin` | Start passkey registration (creation options) |
| POST | `/api/v1/webauthn/register/finish` | Verify attestation and store the passkey |
| GET | `/api/v1/webauthn/credentials` | List the user's registered passkeys |
| DELETE | `/api/v1/webauthn/credentials/{id}` | Remove one of the user's passkeys |

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
| GET | `/api/v1/admin/audit/export?format=csv\|json` | `audit:read` — up to 10 000 rows, CSV with UTF-8 BOM |
| GET | `/api/v1/admin/audit/trends` | `audit:read` |
| GET | `/api/v1/admin/audit/security-dashboard?range=7d` | `audit:read` |
| GET | `/api/v1/admin/security-posture` | `admin` role — total users, without MFA, inactive 30 d |
| GET | `/api/v1/admin/health` | `admin` role — DB + Redis ping with pool stats |
| GET | `/api/v1/admin/service-accounts` | `service_accounts:read` |
| POST | `/api/v1/admin/service-accounts` | `service_accounts:create` |
| PATCH | `/api/v1/admin/service-accounts/{id}` | `service_accounts:update` |
| PATCH | `/api/v1/admin/service-accounts/{id}/status` | `service_accounts:update` |
| DELETE | `/api/v1/admin/service-accounts/{id}` | `service_accounts:delete` |
| GET | `/api/v1/admin/oidc/clients` | `admin` role |
| POST | `/api/v1/admin/oidc/clients` | `admin` role + step-up MFA |
| PUT | `/api/v1/admin/oidc/clients/{id}` | `admin` role + step-up MFA |
| DELETE | `/api/v1/admin/oidc/clients/{id}` | `admin` role + step-up MFA |
| POST | `/api/v1/admin/oidc/clients/{id}/rotate` | Rotate client secret — `admin` role + step-up MFA |

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
| `WEBAUTHN_RP_ID` | WebAuthn Relying Party ID (effective domain); defaults to `localhost` |
| `WEBAUTHN_RP_DISPLAY_NAME` | Human-facing Relying Party name; defaults to `ZeroTrust` |
| `BAO_ADDR` / `VAULT_ADDR` | Optional address of a persistent OpenBao/Vault server with the transit secrets engine enabled |
| `BAO_TOKEN` / `VAULT_TOKEN` | Token allowed to use the transit key named `db-encryption-key`; never use a root token |
| `SMTP_HOST` | SMTP host for password reset email |
| `SMTP_PORT` | SMTP port, defaults to `587` |
| `SMTP_FROM` | Sender address for password reset email |
| `SMTP_USER` | SMTP username |
| `SMTP_PASSWORD` | SMTP password |
| `PUBLIC_APP_URL` | Public frontend origin used in password reset links and OIDC login redirects |
| `OIDC_ISSUER_URL` | Issuer URL embedded in tokens and the discovery document; defaults to `http://localhost:8080` |
| `INITIAL_ADMIN_EMAIL` | Seed admin email |
| `INITIAL_ADMIN_PASSWORD_HASH` | bcrypt hash of admin password |

When OpenBao/Vault is configured, the backend encrypts selected personal data
(`users.email`, `users.first_name`, `users.last_name`, and sensitive audit
metadata) through the transit engine. Startup performs an encrypt/decrypt probe
against `db-encryption-key`, so an unavailable server, missing key, or incomplete
token policy fails fast. The secrets server and transit key must be persistent
and backed up independently; replacing that key makes existing ciphertext
undecryptable. Existing plaintext user values remain readable during rollout,
and migrations backfill the deterministic email hashes used for lookup. If no
address is configured, the backend logs a warning and runs without
application-level field encryption.

## Authentication & Session Lifecycle

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

### 5. WebAuthn Passkeys (FIDO2)
Phishing-resistant authentication built on `github.com/go-webauthn/webauthn`, with single-use ceremony state kept in Redis (5-minute TTL) and credentials persisted in PostgreSQL. The Relying Party is configured via `WEBAUTHN_RP_ID` (effective domain, e.g. `localhost`) and `WEBAUTHN_RP_DISPLAY_NAME`; allowed origins come from `CORS_ALLOWED_ORIGINS`.

How the passkey flow works:

1. **Enroll**: From **Two-Factor Auth → Passkeys**, the signed-in user selects **Add a passkey** and names the device. The browser creates a discoverable credential after local biometric, PIN, or security-key verification.
2. **Use as MFA**: After a correct password, an account with a registered passkey can complete the second-factor challenge with that passkey instead of a TOTP code.
3. **Sign in without a password**: From the login page, **Sign in with a passkey** starts a usernameless assertion. The authenticator selects a discoverable credential, returns its `userHandle`, and the backend resolves the account before issuing the normal secure session cookies.
4. **Manage credentials**: Registered passkeys show their creation and last-used dates and can be removed individually. Removing a credential prevents future assertions with that passkey.

- **Registration**: An authenticated user enrolls a passkey through `register/begin` → `register/finish`. Registration requests a **discoverable (resident) credential** (`residentKey: required`) so the same passkey can later be used for passwordless login. Already-registered authenticators are excluded to prevent duplicate enrollment.
- **Second-factor login**: When a user with a registered passkey signs in with their password, the passkey satisfies the MFA step — the client runs an assertion via `webauthn/login/begin` → `webauthn/login/finish` (keyed by the pending `mfa_token`) instead of entering a TOTP code.
- **Passwordless (usernameless) login**: `passwordless/begin` returns assertion options with an empty `allowCredentials` list plus an opaque `ceremony_id`; the authenticator surfaces a discoverable credential and reveals the user via its `userHandle`. `passwordless/finish` validates the assertion, confirms the account is active, and issues a full session — no email or password required. **User verification is required** (`userVerification: required`), so the authenticator's biometric/PIN stands in for both possession and inherence factors.
- **Replay & counter protection**: Ceremony sessions are single-use (consumed on finish via Redis `GETDEL`), and the signature counter is updated on each successful assertion.
- **Management**: Users list and remove their passkeys (`GET`/`DELETE webauthn/credentials`) from the Two-Factor Auth settings page.

### 6. Service Account (M2M) Auth Model
Designed for secure machine-to-machine integrations:
- **Credentials**: Created by administrators with fine-grained scopes. The system outputs a unique Client ID and a 32-byte cryptographically secure Client Secret (hashed with bcrypt cost factor 12 before database insertion).
- **Token Issuance**: Service accounts retrieve access tokens via `/api/v1/auth/token` by utilizing the OAuth 2.0 `client_credentials` grant.
- **Lifecycle & Enforcement**: Token generation fails immediately if the service account has expired or been marked inactive. The resulting M2M Bearer tokens are restricted strictly to the scopes assigned to the account.

### 7. OIDC Provider
ZeroTrust acts as a standards-compliant OpenID Connect Identity Provider. External applications can delegate authentication to ZeroTrust and receive verifiable ID tokens in return.

**Authorization Code + PKCE flow:**
1. The client redirects the user's browser to `GET /oauth2/authorize` with `response_type=code`, `client_id`, `redirect_uri`, `scope`, and a PKCE `code_challenge` (S256). Optional: `prompt` (`none` / `login` / `consent`), `max_age`, `nonce`, `state`.
2. If the user is not logged in (or `prompt=login` is set), they are sent through the normal ZeroTrust login flow and redirected back. `prompt=none` skips the UI entirely and returns `login_required` to the client if the session is absent or expired.
3. After login, the user lands on the consent screen showing the requesting application's registered display name and the requested scopes. If the user has MFA enabled, a recent step-up proof is required before approval can be submitted.
4. On approval, the backend issues a short-lived authorization code (5-minute TTL, single-use, stored in Redis) and redirects the browser to the client's `redirect_uri` with `?code=…&state=…`.
5. The client exchanges the code at `POST /oauth2/token` with `grant_type=authorization_code` and the PKCE `code_verifier`, receiving an access token, an Ed25519-signed ID token (when `openid` scope was requested), and — when `offline_access` scope was requested — a rotating opaque refresh token.

**Refresh token grant (`offline_access` scope):**
- Include `offline_access` in the authorization request scopes to receive a refresh token alongside the initial access token.
- Exchange the refresh token at `POST /oauth2/token` with `grant_type=refresh_token` to receive a new access token and a **rotated** refresh token. The original token is atomically consumed and cannot be reused.
- Scope may be downscoped per exchange; requesting scopes outside the original grant are silently dropped.

**UserInfo, introspection, and revocation:**
- `GET /oauth2/userinfo` — returns live profile claims (sub, email, name, locale, roles) for a Bearer access token.
- `POST /oauth2/introspect` (RFC 7662) — returns `{"active":true,…}` with full claims for valid tokens, or `{"active":false}` for invalid or revoked ones. Requires client authentication.
- `POST /oauth2/revoke` (RFC 7009) — immediately invalidates an access token (JTI written to Redis blocklist) or a refresh token (deleted from the store). Always returns 200 regardless of token validity.

**RP-Initiated Logout (`/oauth2/end_session`):**
- The client redirects to `GET /oauth2/end_session` (or `POST`) with an optional `id_token_hint`, `post_logout_redirect_uri`, and `state`.
- The server revokes the user's current session tokens, clears all four session cookies (`access_token`, `refresh_token`, `csrf_token`, `at_exp`), and emits an audit event.
- `post_logout_redirect_uri` is only honoured when accompanied by a valid `id_token_hint` whose `aud` identifies a registered client that has the URI in its `redirect_uris` list. All other cases fall back to the application login page, preventing open-redirect abuse.
- Expired ID tokens are accepted as hints (per OIDC Session Management §5 — the hint is a client identifier, not an active credential).

**Security properties:**
- Only S256 PKCE is accepted; plain is rejected. `code_verifier` must be 43–128 unreserved ASCII characters (RFC 7636 §4.1).
- Authorization codes and refresh tokens are single-use (atomically consumed via Redis `GETDEL`).
- `max_age` (OIDC Core §3.1.2.1) — if the session's `iat` is older than `max_age` seconds, re-authentication is forced. `max_age=0` always forces re-auth.
- Access and ID tokens are signed with the same EdDSA key as internal tokens and verifiable via `/.well-known/jwks.json`.
- All OIDC endpoints are rate-limited at 30 req/min per IP (token/authorize/revoke/introspect/end_session) and 100 req/min for userinfo.
- All consent decisions, token issuances, exchanges, rotations, introspections, revocations, and logouts are written to the immutable audit log.
- OIDC client management (create / update / delete / rotate secret) requires the `admin` role and a step-up MFA challenge.
- Creating or rotating a client secret returns the plaintext value exactly once; subsequent reads only expose the bcrypt hash.

### 8. DPoP (Demonstrating Proof-of-Possession, RFC 9449)
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
make secrets      Generate secrets (infra/.env, Ed25519 key pair)
make up           Start in development-style HTTP mode
make dev          Start in development mode with hot reload
make up-prod      Start production-style HTTPS mode via nginx
make down         Stop all services
make down-v       Stop and delete volumes (resets database)
make test         Run backend tests
make lint         Run go vet + tsc
make clean        Remove Docker images and volumes
make screenshots  Take UI screenshots and update docs/index.md
                  (requires ADMIN_PASSWORD=<pass>; optionally SCREENSHOT_URL, ADMIN_EMAIL)
```

## License

Copyright (c) 2026 Mustafa Sercan Sak

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
