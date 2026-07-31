# Architecture

ZeroTrust is a self-hosted authentication platform built around a Zero Trust model: every request is verified independently, no implicit trust is granted by network position, and secrets are never stored in plaintext.

## System Components

```
                              ┌──────────────────────────────────────────────┐
                              │                 Browser                      │
                              │  React + Vite SPA (port 3000 in dev)         │
                              └──────────────┬───────────────────────────────┘
                                             │  HTTPS (CORS + httpOnly cookies)
                              ┌──────────────▼───────────────────────────────┐
                              │            nginx (prod only)                 │
                              │   TLS termination · static file serving      │
                              └────────────┬────────────────┬────────────────┘
                                           │ /api/*         │ /*
                              ┌────────────▼──────┐  ┌──────▼──────┐
                              │   Go Backend      │  │  Frontend   │
                              │   (port 8080)     │  │  container  │
                              └──┬──────┬─────────┘  └─────────────┘
                                 │      │      │
                    ┌────────────▼┐ ┌───▼────┐ ┌▼────────────────┐
                    │  PostgreSQL │ │ Redis  │ │ OpenBao / Vault │
                    │  (port 5432)│ │ (6379) │ │  (optional)     │
                    └─────────────┘ └────────┘ └─────────────────┘
```

**PostgreSQL** holds all durable state: users, sessions (as refresh-token hashes), audit logs, OIDC clients, MFA secrets, WebAuthn credentials, and service accounts.

**Redis** holds ephemeral, time-bound state: rate-limit counters (sliding window), progressive lockout counters, JTI blocklist entries (for instant token revocation), OIDC auth codes, OIDC refresh tokens, and MFA step-up window markers.

**OpenBao / Vault** (optional) provides at-rest encryption for sensitive columns (email, TOTP secrets, audit metadata). When `BAO_ADDR` or `VAULT_ADDR` is not set, the server starts without application-level encryption and logs a warning.

---

## Request Lifecycle

### Login

```
Browser                         Backend                         Redis / DB
   │                               │                                │
   │── POST /api/v1/auth/login ───▶│                                │
   │   {email, password}           │── rate-limit check ───────────▶│
   │                               │◀─ (counter) ───────────────────│
   │                               │── load user ──────────────────▶│
   │                               │◀─ (row) ───────────────────────│
   │                               │   bcrypt.CompareHashAndPassword│
   │                               │── risk score calc ────────────▶│ (geo + sessions)
   │                               │◀─ score (0–100) ───────────────│
   │                               │   [if score ≥ block threshold] │
   │                               │── issue access token (Ed25519 JWT, 1 min TTL)
   │                               │── issue refresh token (opaque, SHA-256 hash in DB)
   │◀── Set-Cookie: access_token ──│
   │    Set-Cookie: refresh_token  │
   │    Set-Cookie: csrf_token     │
   │    200 {user_id, …}           │
```

The dummy-password-hash trick runs a full bcrypt comparison even when the account does not exist, keeping login response time uniform regardless of whether an email is registered (see [security model](security-model.md#account-enumeration)).

### Token Refresh

```
Browser                         Backend                         DB
   │                               │                               │
   │── POST /api/v1/auth/refresh ─▶│                               │
   │   Cookie: refresh_token       │── SELECT … FOR UPDATE ───────▶│
   │                               │   (atomic reuse detection)    │
   │                               │── rotate: delete old, insert new
   │◀── new access_token cookie ───│                               │
```

`SELECT … FOR UPDATE` on the session row makes rotation atomic: concurrent refresh attempts see the old token gone and return 401 rather than issuing two live tokens.

### Protected Endpoint

```
Browser                         Backend (middleware stack)
   │                               │
   │── GET /api/v1/me ────────────▶│
   │   Cookie: access_token        │  1. CSRF check (X-CSRF-Token header vs cookie)
   │   X-CSRF-Token: <value>       │  2. JWT verify (Ed25519, exp, nbf)
   │                               │  3. JTI blocklist check (Redis GET)
   │                               │  4. Session policy check (idle/absolute timeout)
   │                               │  5. Permission check (if route requires one)
   │◀── 200 {…} ───────────────────│
```

---

## Token Architecture

| Token | Type | TTL | Storage | Purpose |
|---|---|---|---|---|
| Access token | Signed JWT (`JWT_SIGNING_ALG`: EdDSA, ES256, or RS256) | 1 minute | httpOnly cookie | Authenticate API calls |
| Refresh token | Opaque 256-bit random | Configurable (session policy) | SHA-256 hash in PostgreSQL | Obtain new access tokens |
| CSRF token | Random 256-bit | Same as session | httpOnly + non-httpOnly cookies | Double-submit CSRF defense |
| OIDC auth code | Opaque 256-bit | 5 minutes | Redis | OAuth2 authorization code flow |
| OIDC refresh | Opaque | Configurable | Redis | OIDC session continuity |

**Why Ed25519?** EdDSA on Curve25519 offers short keys (32 bytes public), fast verification (~50 µs on modern hardware), and is not vulnerable to the fault-attack class that affects ECDSA with bad randomness.

**Crypto-agility:** the signing layer is algorithm-agnostic. `JWT_SIGNING_ALG` selects EdDSA (default), ES256, or RS256; each key in the keystore carries its own algorithm, so rotation between different algorithms is supported, and token validation pins the `alg` header to the key identified by `kid` (algorithm-confusion protection). Post-quantum signature schemes (e.g. ML-DSA) can be added the same way once JOSE registrations are finalized.

**Hybrid post-quantum TLS:** when the Go server terminates TLS directly (`TLS_ENABLED=true`), it prefers the X25519MLKEM768 hybrid key exchange (draft-ietf-tls-ecdhe-mlkem, native in Go 1.24+) with automatic fallback to classical curves — mitigating "harvest now, decrypt later" collection of recorded traffic.

**Why 1-minute access token TTL?** Short TTL limits the damage window if a token is captured. The refresh-token rotation model (one use per token) means stolen refresh tokens are detected on reuse.

**Audience-confusion protection:** first-party session and service-account tokens carry a `token_use` claim (`session` / `service`); OIDC access tokens issued to external clients carry none. The internal `/api/v1` surface validates with `ValidateAccessToken`, which rejects tokens without a recognized `token_use`, so an OIDC token cannot be replayed against the first-party API. OIDC endpoints (UserInfo, Introspect, Revoke) use `ValidateOIDCAccessToken`, which checks signature and expiry only.

**Key rotation** is zero-downtime: the keystore holds a primary key (used for signing) and an optional secondary key (used for verification only). Roll a new primary, demote the old one to secondary, then remove the secondary after all tokens signed with it have expired.

---

## Session Model

See [session policy](settings.md#session-policy) for runtime-configurable timeouts.

Each login creates a session row containing:
- `refresh_token_hash` — SHA-256 of the opaque token issued to the browser
- `ip_address`, `user_agent`, `device_info` (JSONB) — for anomaly detection and the session list UI
- `expires_at` — absolute expiry (enforced at refresh time)
- `last_used_at` — updated on each successful refresh (used for idle timeout enforcement)

**Session cap**: The oldest session is evicted when a user exceeds `max_sessions_per_user` (default 5). This prevents indefinite accumulation without forcing single-session semantics that frustrate multi-device users.

**Reuse detection**: Rotation no longer deletes the old session row — it marks it revoked with `revoked_reason='rotation'` (other reasons include `logout`, `mass_revoke`, `revoke_others`, `revoke_by_id`, `evicted`, `stale_initial`, `device_replaced`), and `sessions.refresh_token_hash` carries a unique index. Only replaying a rotation-revoked token is treated as theft: the server revokes *all* of that user's sessions, which also invalidates the user's OIDC refresh tokens. Tokens revoked for any other reason (logout, admin action, cleanup) simply fail on replay without triggering mass revocation. This surfaces stolen refresh tokens: the legitimate user's next refresh will fail and force re-login, alerting them that something is wrong.

---

## Background Workers

The server starts three goroutines alongside the HTTP listener:

| Worker | Interval | Purpose |
|---|---|---|
| Session cleanup | 10 minutes | Delete expired sessions from PostgreSQL (prevents table bloat) |
| Connection pool metrics | 5 minutes | Emit structured log lines with DB/Redis pool stats |
| Service account listener | — | `LISTEN`/`NOTIFY` on PostgreSQL for real-time SA status changes (SSE broadcast) |
| Resilient mailer | — | Bounded queue (1000) + worker pool (2) for async security alert and password-reset email delivery |

Workers are shut down gracefully: they listen on a `rootCtx` derived from `SIGINT`/`SIGTERM`, and the server waits up to 10 seconds for them to drain before exiting.

---

## Internal Package Structure

```
backend/
├── cmd/server/main.go        # Wiring: config → deps → routes → server
├── internal/
│   ├── admin/                # User management admin endpoints
│   ├── audit/                # Audit log storage, querying, webhook delivery, dashboard aggregation
│   ├── auth/                 # Core auth: login, refresh, token issuance, risk scoring, anomaly detection
│   ├── mfa/                  # TOTP setup/verify/disable, step-up enforcement, backup codes
│   ├── oidc/                 # OpenID Connect: discovery, authorize, token, userinfo, revoke, introspect
│   ├── passwdreset/          # Password reset: token generation, email delivery, atomic consume
│   ├── serviceaccount/       # M2M service accounts: CRUD, secret rotation, SSE hub
│   ├── session/              # Session CRUD, SSE hub for real-time revocation UI updates
│   ├── settings/             # System settings: DB-backed KV store with in-process cache
│   ├── user/                 # User CRUD, password ops, avatar, profile
│   └── webauthn/             # FIDO2 passkey registration and authentication
└── pkg/
    ├── crypto/               # AES-GCM helpers used by MFA secret encryption
    ├── database/             # Migration runner (golang-migrate)
    ├── geoip/                # MaxMind GeoLite2 City lookup for anomaly detection
    ├── mailer/               # SMTP mailer + resilient async wrapper + log-only fallback
    ├── middleware/            # Auth, CSRF, rate limiting, RBAC, audit, security headers
    ├── secrets/              # OpenBao/Vault client for application-level column encryption
    ├── token/                # JWT issuance and verification (Ed25519 / DPoP)
    └── validation/           # Password complexity enforcement
```

---

## Data Flow: Audit Log

Every non-trivial action produces an `audit.Entry`. The write path is:

1. Handler calls `auditRepo.Log(ctx, entry)` (synchronous for auth failures, async for everything else).
2. `auditRepo` optionally encrypts `metadata` via the Vault/Bao client before writing to PostgreSQL.
3. If a `webhook_url` is configured in system settings, the repo fans the entry out to the webhook URL via an HTTP POST (fire-and-forget with a 5-second timeout). Dispatch is throttled to one webhook per IP+action combination per minute, with periodic pruning of the throttle map, so a flood of identical events cannot exhaust the outbound channel.
4. On webhook delivery failure, a second audit entry is written (`auth.security_alert.delivery_failure`).

Audit entries are queryable by admin users via `GET /api/v1/admin/audit` with filtering, sorting, and CSV export.

---

## OIDC / OAuth2 Flow

ZeroTrust acts as an OpenID Connect Provider (OP). The authorization code flow:

```
Client App          ZeroTrust (OP)              User Browser
    │                     │                          │
    │── GET /oauth2/authorize ──────────────────────▶│
    │   response_type=code, client_id, redirect_uri  │
    │                     │◀─ user logs in ──────────│
    │                     │── store auth code (Redis, 5 min)
    │                     │── redirect to redirect_uri?code=…
    │◀─────────────────────────────────────────────── │
    │── POST /oauth2/token (code) ───────────────────▶│
    │                     │── verify code, issue id_token + access_token
    │◀─ {id_token, access_token, refresh_token} ──────│
```

OIDC clients are registered in the database with a client secret (bcrypt hashed). The `/.well-known/openid-configuration` discovery document and `/.well-known/jwks.json` JWKS endpoint are public and cached for 1 hour.

**OIDC refresh tokens** are stored in Redis and are single-use: each refresh consumes the old token and issues a rotated one within the same grant family (identified by a family ID). A per-user index lets the server invalidate every OIDC refresh token a user holds when all of their sessions are revoked. Consumed tokens leave a tombstone; replaying one is treated as theft and revokes the entire grant family (RFC 6819 §5.2.2.3).
