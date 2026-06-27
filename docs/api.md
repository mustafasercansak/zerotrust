# API Reference

Base URL: `http://localhost:8080` (development) · `https://<your-domain>` (production)

All `/api/v1` endpoints require a valid CSRF token on state-mutating requests (see [CSRF](#csrf)). Responses are JSON unless noted otherwise.

## Authentication

Access tokens are issued as `httpOnly` cookies named `access_token`. They are not accessible from JavaScript and are sent automatically by the browser with each request to the same origin.

The `refresh_token` cookie is rotated on each successful `POST /api/v1/auth/refresh` call. Present the `X-CSRF-Token` header (obtained from the `csrf_token` cookie) on all state-mutating requests.

**Service accounts** use HTTP Basic Auth (`client_id:client_secret`) against `POST /oauth2/token` with `grant_type=client_credentials`.

---

## CSRF

On login, the server sets a `csrf_token` cookie that is readable by JavaScript (not `httpOnly`). All `POST`, `PATCH`, `PUT`, and `DELETE` requests to `/api/v1` must include:

```
X-CSRF-Token: <value of csrf_token cookie>
```

Failures are logged as `auth.csrf.failure` in the audit log.

---

## Rate Limits

| Limit group | Routes | Limit | Window |
|---|---|---|---|
| `login` | `/api/v1/auth/login`, `/auth/mfa/challenge`, WebAuthn login | 10 req | 1 min per IP |
| `token` | `/oauth2/token`, `/oauth2/authorize`, `/oauth2/revoke`, `/oauth2/introspect`, `/oauth2/end_session`, `/api/v1/auth/token` | 30 req | 1 min per IP |
| `global` | All routes (except `/health`, `/metrics`) | 300 req | 1 min per IP |
| `protected` | All authenticated `/api/v1` routes | 300 req | 1 min per IP |

When a limit is hit the server returns `429 Too Many Requests` with `Retry-After` and `X-RateLimit-Limit` / `X-RateLimit-Remaining` headers.

---

## Error Format

All error responses follow this shape:

```json
{ "error": "<error_code>" }
```

Common error codes are documented per endpoint. The `500` case always returns `{"error":"internal_error"}`.

---

## Public Endpoints

These endpoints do not require authentication.

### `GET /health`

Dependency health check. Used by Docker Compose and load balancers.

**Response `200 OK`** (all checks pass):
```json
{
  "status": "ok",
  "service": "zerotrust",
  "checks": { "database": "ok", "redis": "ok" }
}
```

**Response `503 Service Unavailable`** (any check fails):
```json
{
  "status": "degraded",
  "service": "zerotrust",
  "checks": { "database": "ok", "redis": "error" }
}
```

### `GET /metrics`

Prometheus-format plain text metrics.

```
# HELP zerotrust_audit_write_failures_total Total audit log write failures.
# TYPE zerotrust_audit_write_failures_total counter
zerotrust_audit_write_failures_total 0
```

### `GET /.well-known/jwks.json`

JWKS document for verifying access tokens. `Cache-Control: public, max-age=3600`.

### `GET /.well-known/openid-configuration`

OIDC discovery document. `Content-Type: application/json`.

### `GET /oauth2/clients/{client_id}`

Returns non-sensitive public metadata for a registered OIDC client (name, allowed scopes). Used by the consent UI.

---

## Authentication Endpoints (`/api/v1/auth`)

### `POST /api/v1/auth/login`

Rate limited: 10/min per IP.

**Request:**
```json
{
  "email": "user@example.com",
  "password": "…"
}
```

**Response `200 OK`** — standard login (no MFA):
```json
{
  "user_id": "uuid",
  "email": "user@example.com",
  "first_name": "…",
  "last_name": "…",
  "has_avatar": false,
  "locale": "en",
  "notify_security_emails": true,
  "roles": ["user"],
  "permissions": ["users:read"]
}
```
Sets `access_token`, `refresh_token`, and `csrf_token` cookies.

**Response `202 Accepted`** — MFA required:
```json
{ "mfa_required": true, "mfa_token": "<short-lived Redis-backed token>" }
```

**Error codes:**

| Code | HTTP | Meaning |
|---|---|---|
| `invalid_credentials` | 401 | Wrong email or password |
| `account_inactive` | 403 | Account is disabled |
| `account_locked` | 429 | Progressive lockout active; `Retry-After` header set |
| `high_risk_blocked` | 403 | Risk score exceeded block threshold and risk-based auth is enabled |
| `ip_blocked` | 403 | Source IP is not on the IP allowlist |
| `country_blocked` | 403 | Source country is not on the country allowlist |
| `device_blocked` | 403 | Device/browser fails device trust policy |

### `POST /api/v1/auth/mfa/challenge`

Complete a login that returned `mfa_required: true`.

**Request:**
```json
{
  "mfa_token": "<token from login response>",
  "code": "123456"
}
```

**Response `200 OK`**: same shape as a successful login. Sets cookies.

**Error codes:** `invalid_mfa_token`, `invalid_code`, `account_locked`.

### `POST /api/v1/auth/webauthn/login/begin`

Start a WebAuthn second-factor assertion for a user who completed password auth. Returns a `PublicKeyCredentialRequestOptions` JSON blob for the browser's `navigator.credentials.get()` call.

**Request:**
```json
{ "mfa_token": "<token from login response>" }
```

### `POST /api/v1/auth/webauthn/login/finish`

**Request:**
```json
{
  "mfa_token": "<token>",
  "ceremony_id": "<id from begin response>",
  "credential": { /* AuthenticatorAssertionResponse */ }
}
```

**Response `200 OK`**: same shape as successful login.

### `POST /api/v1/auth/webauthn/passwordless/begin`

Start a usernameless (discoverable credential) WebAuthn login. No request body required.

### `POST /api/v1/auth/webauthn/passwordless/finish`

**Request:**
```json
{
  "ceremony_id": "<id from begin>",
  "credential": { /* AuthenticatorAssertionResponse */ }
}
```

**Response `200 OK`**: same shape as successful login.

### `POST /api/v1/auth/token`

DPoP-bound token issuance. Used by clients that opt into Proof-of-Possession binding.

**Headers required:** `DPoP: <proof JWT>`

**Request:**
```json
{ "grant_type": "password", "email": "…", "password": "…" }
```

**Response `200 OK`:**
```json
{ "access_token": "…", "token_type": "DPoP", "expires_in": 60 }
```

### `POST /api/v1/auth/refresh`

Rotate the refresh token. The browser sends the `refresh_token` cookie automatically.

**Response `200 OK`**: sets new `access_token` and `refresh_token` cookies. No body.

**Error codes:** `no_refresh_token` (cookie absent), `invalid_refresh_token` (expired, revoked, or reuse detected — all sessions revoked on reuse).

### `POST /api/v1/auth/logout`

Revoke the current session.

**Response `204 No Content`**: clears all auth cookies.

### `POST /api/v1/auth/register`

Create a new user account. Requires `REGISTRATION_ENABLED=true` (default: `false`).

**Request:**
```json
{ "email": "…", "password": "…", "first_name": "…", "last_name": "…" }
```

**Response `200 OK`**: same shape as successful login (auto-login after registration).

**Error codes:** `registration_disabled`, `email_taken`, `invalid_email`, `password_too_weak`.

### `POST /api/v1/auth/forgot-password`

Send a password reset email. Always returns `204` regardless of whether the email exists (prevents enumeration).

**Request:** `{ "email": "…" }`

### `POST /api/v1/auth/reset-password`

Consume a reset token and set a new password.

**Request:**
```json
{ "token": "<from email link>", "new_password": "…" }
```

**Response `204 No Content`**: token consumed, all sessions revoked, security alert email sent.

**Error codes:** `invalid_token`, `token_expired`, `password_too_weak`.

---

## OAuth2 / OIDC Endpoints

### `GET /oauth2/authorize`

Start the authorization code flow. Query parameters: `response_type=code`, `client_id`, `redirect_uri`, `scope`, `state`, `nonce`, `code_challenge`, `code_challenge_method`.

Redirects to the frontend consent screen if the user is authenticated, or to login first.

### `POST /oauth2/token`

Exchange an authorization code, or use client credentials for M2M.

**`authorization_code` grant:**
```
grant_type=authorization_code&code=…&redirect_uri=…&code_verifier=…
```
Basic Auth: `client_id:client_secret`

**`client_credentials` grant (service accounts):**
```
grant_type=client_credentials&scope=…
```
Basic Auth: `client_id:client_secret`

**`refresh_token` grant:**
```
grant_type=refresh_token&refresh_token=…
```

### `GET /oauth2/userinfo`

Returns claims for the authenticated user (requires a valid OIDC access token in the Authorization header).

### `POST /oauth2/revoke`

Revoke an access or refresh token.

### `POST /oauth2/introspect`

Token introspection (RFC 7662).

### `GET|POST /oauth2/end_session`

RP-Initiated Logout. Invalidates the OIDC session and optionally redirects to `post_logout_redirect_uri`.

### `POST /api/v1/oauth2/consent`

Called by the frontend after the user approves or denies the authorization request. Issues the auth code on approval.

---

## User Profile Endpoints (authenticated)

### `GET /api/v1/me`

Returns the current user's profile and permissions.

**Response:**
```json
{
  "user_id": "uuid",
  "email": "…",
  "first_name": "…",
  "last_name": "…",
  "has_avatar": false,
  "locale": "en",
  "notify_security_emails": true,
  "roles": ["user"],
  "permissions": ["users:read"]
}
```

### `PATCH /api/v1/me/profile`

Update display name.

**Request:** `{ "first_name": "…", "last_name": "…" }`

**Response**: same shape as `GET /me`.

**Error codes:** `invalid_profile` (names too long or contain invalid characters).

### `PATCH /api/v1/me/locale`

Change UI language. Triggers an audit event and a security alert email if the user's previous locale was different. Accepted values: `"en"`, `"tr"`.

**Request:** `{ "locale": "en" }`

**Response `204 No Content`**.

### `PATCH /api/v1/me/password`

Change the authenticated user's password. Revokes all sessions after success (including the current one — client must re-login).

**Request:**
```json
{ "current_password": "…", "new_password": "…" }
```

**Error codes:** `wrong_password`, `password_too_weak`.

### `PATCH /api/v1/me/notifications`

Opt in or out of security alert emails.

**Request:** `{ "notify_security_emails": true }`

**Response `204 No Content`**.

### `POST /api/v1/me/avatar`

Upload an avatar image. `multipart/form-data`, field name `avatar`. Max size: 2 MB. Accepted types: `image/jpeg`, `image/png` (detected by content sniffing, not extension).

**Response**: full user profile object.

### `GET /api/v1/me/avatar`

Returns the current user's avatar image. `404` if no avatar is set.

### `GET /api/v1/users/{id}/avatar`

Returns any user's avatar by user ID. Used by the admin panel to render user lists.

### `DELETE /api/v1/me/avatar`

Remove the avatar.

---

## Session Endpoints (authenticated)

### `GET /api/v1/sessions`

List the current user's active sessions.

**Response:**
```json
[
  {
    "id": "uuid",
    "ip_address": "1.2.3.4",
    "user_agent": "Mozilla/5.0 …",
    "device_info": { "os": "macOS", "browser": "Chrome" },
    "created_at": "2026-01-01T00:00:00Z",
    "last_used_at": "2026-01-01T01:00:00Z",
    "is_current": true
  }
]
```

### `GET /api/v1/sessions/events`

Server-Sent Events stream. Emits a `revoked` event when any of the user's sessions is revoked, allowing the UI to force a re-login in real time.

### `DELETE /api/v1/sessions`

Revoke all sessions except the current one.

**Response `204 No Content`**.

### `DELETE /api/v1/sessions/{id}`

Revoke a specific session by ID.

---

## MFA Endpoints (authenticated)

### `GET /api/v1/mfa/status`

**Response:**
```json
{ "enabled": true, "supported": true }
```
If MFA is disabled server-side, `supported` is `false`.

### `POST /api/v1/mfa/setup`

Begin TOTP setup. Returns a `provisioning_uri` for the QR code and a `secret` for manual entry.

**Response:**
```json
{ "secret": "…", "provisioning_uri": "otpauth://totp/…" }
```

### `POST /api/v1/mfa/verify`

Confirm setup by providing the first TOTP code. Also returns single-use backup recovery codes.

**Request:** `{ "code": "123456" }`

**Response:**
```json
{ "recovery_codes": ["aaaa-bbbb", "cccc-dddd", "…"] }
```

### `POST /api/v1/mfa/disable`

Disable TOTP. Requires a valid current TOTP code.

**Request:** `{ "code": "123456" }`

### `POST /api/v1/mfa/step-up`

Re-verify MFA to unlock step-up protected routes (admin mutations, OIDC client management, etc.). Valid for 10 minutes.

**Request:** `{ "code": "123456" }`

---

## WebAuthn / Passkey Endpoints (authenticated)

### `POST /api/v1/webauthn/register/begin`

Begin passkey registration. Returns `PublicKeyCredentialCreationOptions`.

### `POST /api/v1/webauthn/register/finish`

Complete registration.

**Request:**
```json
{ "ceremony_id": "…", "credential": { /* AuthenticatorAttestationResponse */ } }
```

### `GET /api/v1/webauthn/credentials`

List registered passkeys.

**Response:**
```json
[{ "id": "uuid", "name": "Touch ID", "aaguid": "…", "created_at": "…", "last_used_at": "…" }]
```

### `DELETE /api/v1/webauthn/credentials/{id}`

Remove a passkey.

---

## Session Policy

### `GET /api/v1/session/policy`

Returns the configured idle timeout for the current user's role. Used by the frontend to implement auto-logout.

**Response:**
```json
{ "idle_timeout_seconds": 300 }
```

---

## Admin — User Management

All routes require the `users:read` or `users:update` permission as noted. Mutation endpoints additionally require step-up MFA verification.

### `GET /api/v1/admin/users`

**Permission:** `users:read`

Query params: `limit`, `offset`, `search`, `sort_by`, `sort_dir`.

### `POST /api/v1/admin/users`

**Permission:** `users:create`

**Request:** `{ "email": "…", "password": "…", "first_name": "…", "last_name": "…", "roles": ["user"] }`

### `PATCH /api/v1/admin/users/{id}/roles`

**Permission:** `users:update` + step-up MFA

**Request:** `{ "roles": ["user", "admin"] }`

### `PATCH /api/v1/admin/users/{id}/status`

**Permission:** `users:update` + step-up MFA

**Request:** `{ "is_active": false }`

### `POST /api/v1/admin/users/bulk-status`

**Permission:** `users:update` + step-up MFA

**Request:** `{ "user_ids": ["uuid", "…"], "is_active": false }`

### `GET /api/v1/admin/users/{id}/sessions`

**Permission:** `users:read`

### `GET /api/v1/admin/users/{id}/mfa`

**Permission:** `users:read`

### `DELETE /api/v1/admin/users/{id}/sessions`

**Permission:** `users:update` + step-up MFA. Revoke all sessions for a user.

### `DELETE /api/v1/admin/users/{id}/sessions/{sessionId}`

**Permission:** `users:update` + step-up MFA.

---

## Admin — Security

### `GET /api/v1/admin/security-posture`

**Role:** `admin`

Returns aggregate security posture metrics: users without MFA, users without passkeys, active lockouts, recent anomaly detections.

### `GET /api/v1/admin/health`

**Role:** `admin`

Detailed health including connection pool stats.

### `GET /api/v1/admin/security-dashboard`

**Permission:** `audit:read`

Aggregated trends for the security dashboard: login volume, failure rates, anomaly detections, top failed-login IPs, geographic distribution.

---

## Admin — Audit Log

### `GET /api/v1/admin/audit`

**Permission:** `audit:read`

Query params: `limit`, `offset`, `user_id`, `action`, `from`, `to`, `sort_by`, `sort_dir`.

### `GET /api/v1/admin/audit/export`

**Permission:** `audit:read`

Returns CSV. Same filters as list endpoint.

### `GET /api/v1/admin/audit/trends`

**Permission:** `audit:read`

Aggregated counts by action and time bucket for charting.

### `GET /api/v1/me/audit`

Authenticated users can view their own security-relevant audit entries (no admin permission required). Returns max 25 records per page.

---

## Admin — System Settings

### `GET /api/v1/admin/settings`

**Role:** `admin`

Returns all system settings key-value pairs. See [settings reference](settings.md).

### `PATCH /api/v1/admin/settings`

**Role:** `admin` + step-up MFA

**Request:** `{ "key": "max_sessions_per_user", "value": "3" }`

### `POST /api/v1/admin/settings/webhook/test`

**Role:** `admin`

Send a test payload to the configured (or provided) webhook URL.

**Request:** `{ "url": "https://…" }` (optional — uses the stored `webhook_url` if omitted)

---

## Admin — OIDC Clients

All OIDC client mutations require `admin` role + step-up MFA.

### `GET /api/v1/admin/oidc/clients`

### `POST /api/v1/admin/oidc/clients`

**Request:**
```json
{
  "name": "My App",
  "redirect_uris": ["https://myapp.com/callback"],
  "allowed_scopes": ["openid", "profile", "email"],
  "grant_types": ["authorization_code"],
  "response_types": ["code"]
}
```

**Response** includes the plaintext `client_secret` (shown once).

### `PUT /api/v1/admin/oidc/clients/{id}`

Update client metadata (not the secret).

### `DELETE /api/v1/admin/oidc/clients/{id}`

### `POST /api/v1/admin/oidc/clients/{id}/rotate`

Rotate the client secret. Returns the new plaintext secret.

---

## Admin — Service Accounts

Service accounts are M2M credentials that use the `client_credentials` OAuth2 grant.

### `GET /api/v1/admin/service-accounts`

**Permission:** `service_accounts:read`

### `POST /api/v1/admin/service-accounts`

**Permission:** `service_accounts:create` + step-up MFA

**Request:**
```json
{ "name": "…", "description": "…", "scopes": ["…"], "expires_at": "2027-01-01T00:00:00Z" }
```

**Response** includes the plaintext `client_secret` (shown once).

### `PATCH /api/v1/admin/service-accounts/{id}`

**Permission:** `service_accounts:update` + step-up MFA

### `PATCH /api/v1/admin/service-accounts/{id}/status`

**Permission:** `service_accounts:update` + step-up MFA

### `POST /api/v1/admin/service-accounts/{id}/rotate`

**Permission:** `service_accounts:update` + step-up MFA. Returns new plaintext secret.

### `DELETE /api/v1/admin/service-accounts/{id}`

**Permission:** `service_accounts:delete` + step-up MFA

### `GET /api/v1/admin/service-accounts/events`

SSE stream. Emits real-time status change events for the service accounts list UI. Auth is handled via the `access_token` cookie (EventSource does not support custom headers).
