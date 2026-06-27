# Security Model

This document describes the threat model ZeroTrust is designed against, the layered defenses in place, and the reasoning behind key security decisions.

---

## Threat Model

ZeroTrust is designed to protect against:

- **Credential theft**: stolen passwords, session tokens, or refresh tokens.
- **Brute-force and credential stuffing**: automated login attempts.
- **Session hijacking**: an attacker replaying a captured token.
- **Phishing**: fake login pages harvesting credentials and/or session cookies.
- **CSRF**: cross-site requests that piggyback on an authenticated session.
- **Account enumeration**: probing whether a given email exists.
- **Privilege escalation**: a regular user accessing admin functionality.
- **Insider threats**: privileged actions without a verifiable audit trail.
- **Insider MITM / token reuse**: detecting when a refresh token has been used more than once (indicating theft).
- **Geographic impossibility**: a user logging in from two distant locations near-simultaneously.

It is **not** designed to protect against:
- Compromise of the host machine or database credentials.
- Side-channel attacks against the cryptographic primitives themselves.
- A fully compromised browser or endpoint device.

---

## Defense Layers

### 1. Token Architecture

**Access tokens** are Ed25519-signed JWTs with a 1-minute TTL. The short expiry limits the window for a captured token. Tokens include a `jti` (JWT ID) that is checked against a Redis blocklist on every request, enabling instant revocation regardless of the TTL.

**Refresh tokens** are 256-bit random opaques. Only their SHA-256 hash is stored in PostgreSQL. The server never holds the plaintext token after issuing it. On each use, the token is rotated atomically (`SELECT … FOR UPDATE`): the old hash is deleted and a new one issued. If the old hash is presented again after rotation, every session for that user is immediately revoked — this surfaces token theft.

**DPoP (RFC 9449)** binds an access token to a client-held asymmetric key. Clients prove possession of the private key on each request. A stolen DPoP-bound token is unusable without the corresponding key.

### 2. Cookie Security

| Cookie | Flags | Notes |
|---|---|---|
| `access_token` | `httpOnly`, `SameSite=Strict`, `Secure` (prod) | Not readable by JS; protects against XSS-based token theft |
| `refresh_token` | `httpOnly`, `SameSite=Strict`, `Secure` (prod), `Path=/api/v1/auth/refresh` | Scoped to the refresh endpoint only; limits exposure surface |
| `csrf_token` | `SameSite=Strict`, `Secure` (prod) | Readable by JS; paired with `X-CSRF-Token` header for CSRF defense |

The `Secure` flag is controlled by `COOKIES_SECURE` and must be `true` in any HTTPS deployment.

### 3. CSRF Protection

ZeroTrust uses the **double-submit cookie** pattern: the browser reads the `csrf_token` cookie value and sends it back as `X-CSRF-Token` on every state-mutating request. The server compares the two. Because cookies are same-origin, a cross-site attacker cannot read the cookie value and therefore cannot forge the header.

CSRF failures are logged to the audit log as `auth.csrf.failure`.

### 4. Account Enumeration Prevention

Login, forgot-password, and registration endpoints are designed not to reveal whether an email address is registered:

- **Login**: a `dummyPasswordHash` (a real bcrypt hash of a fixed string) is used when the user is not found. `bcrypt.CompareHashAndPassword` still runs a full comparison — the computational cost is identical whether the account exists or not. This closes the timing side-channel.
- **Forgot password**: always returns `204 No Content` regardless of whether the email is registered.
- **Registration**: returns `email_taken` only after the password is hashed (bcrypt is slow by design), so the response time does not distinguish between an existing account and a new one.

### 5. Progressive Lockout

Failed login attempts are counted in Redis per email address. The lockout schedule:

| Failed attempts | Lockout duration |
|---|---|
| 1–4 | None |
| 5 | 1 minute |
| 6 | 5 minutes |
| 7+ | 30 minutes |

The `max_login_attempts` setting (default 5) configures the threshold before the first lockout. Lockout duration escalates on subsequent failures. Counters are cleared on successful login.

### 6. Rate Limiting

Sliding-window rate limits per source IP, stored in Redis. See the [API reference](api.md#rate-limits) for per-endpoint limits. The `Retry-After` header tells clients when they can retry.

Trusted proxy IPs (configured via `TRUSTED_PROXIES`) are trusted for `X-Forwarded-For` extraction so that the real client IP — not the proxy IP — is used for rate limiting.

### 7. Risk-Based Adaptive Authentication

When enabled (`risk_based_auth_enabled = true` in [system settings](settings.md)), the login flow computes a risk score (0–100) from three signals:

| Signal | Score contribution | Notes |
|---|---|---|
| Impossible travel | +80 | Velocity > 800 km/h between current and any active session IP, within 24 hours |
| New device | +30 | User-agent not seen in any of the user's active sessions |
| Suspicious hour | +20 | Login between 23:00–05:00 server local time |
| Recent failed attempts | +15 per attempt, max +45 | Based on the lockout counter at the time of the successful login |

Scores are clamped to 100. The risk score is evaluated **after** the password is verified (never before, to avoid leaking information), and the failed-attempt counter is cleared before scoring so that the attempts can still contribute to the score for this login without persisting into future logins.

**Thresholds** (configurable in system settings):

| Threshold | Default | Effect |
|---|---|---|
| `risk_threshold_mfa` | 40 | If score ≥ threshold and user has MFA enrolled, MFA is required even if not globally mandated |
| `risk_threshold_block` | 80 | If score ≥ threshold, the login is blocked with `high_risk_blocked` |

When `risk_based_auth_enabled = false`, scores are calculated and logged but no enforcement action is taken.

**Security alert emails** are sent for any risk score ≥ 30, regardless of whether enforcement is enabled, if the user has `notify_security_emails = true`.

#### Impossible Travel Detection

The impossible travel check uses the **haversine formula** to compute great-circle distance between the current login IP's geolocation and each active session's IP geolocation. If any pair produces a velocity exceeding 800 km/h and the sessions were active within the last 24 hours, the anomaly is flagged.

This requires the GeoLite2 City database (`GEOIP_DB_PATH`). When the database is absent, this check is skipped.

### 8. Session Policy

Sessions have two independent timeouts enforced at the refresh endpoint:

- **Idle timeout**: if `last_used_at` is older than the idle threshold, the session is considered expired.
- **Absolute timeout**: if `expires_at` is in the past, the session is expired regardless of activity.

Admin users have a shorter idle timeout by default (`session_idle_timeout_seconds_admin`, default 3 min) than regular users (`session_idle_timeout_seconds`, default 5 min). Both are configurable in system settings.

The frontend polls `GET /api/v1/session/policy` to learn the idle timeout and implements a client-side countdown. When the user is inactive, the browser refreshes the token to reset the idle clock. When the idle threshold is reached on the client side, the session is revoked via logout.

### 9. Security Headers

Applied to all responses by `authmw.SecurityHeaders(tlsEnabled)`:

| Header | Value |
|---|---|
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `X-XSS-Protection` | `1; mode=block` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Permissions-Policy` | `geolocation=(), microphone=(), camera=()` |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` (only when `TLS_ENABLED=true`) |

### 10. Step-Up MFA

Sensitive admin operations (role changes, session revocation, OIDC client management, settings changes, service account mutations) require re-verification of MFA within the last 10 minutes. This limits the blast radius of a hijacked admin session: even with a valid access token, the attacker cannot perform high-impact actions without the TOTP device.

Step-up state is stored in Redis with a 10-minute TTL. The frontend prompts for the TOTP code and calls `POST /api/v1/mfa/step-up` before retrying the protected action.

### 11. RBAC and Permissions

The permission model is: **roles → role_permissions → permissions**. Permissions are expressed as `resource:action` pairs (e.g., `users:read`, `audit:read`). Route guards use `authmw.RequirePermission(resource, action)` or `authmw.RequireRole(role)`.

Admin users hold all permissions. Custom roles can be created with granular permission sets. Permissions are embedded in the JWT so they do not require a database round-trip on each request.

### 12. IP and Country Allowlisting

When `ip_allowlist` (CIDR list) or `country_allowlist` (two-letter country code list) are configured in system settings, logins from IPs or countries not on the list are rejected with `ip_blocked` or `country_blocked` before password verification. This does not prevent enumeration via timing because it fires before bcrypt, but the error code is intentionally distinct so admins can tune allowlists without confusion.

### 13. Device Trust

When `device_trust_enabled = true`, the server validates the client's reported OS, OS version, and browser against a configurable policy. Failures return `device_blocked`. See [settings reference](settings.md#device-trust) for all policy knobs.

### 14. Audit Log

Every security-relevant event is written to `audit_logs` with user ID, action, resource, IP address, user agent, and a metadata JSONB blob. The audit log is append-only from the application's perspective (no update or delete endpoints). Entries can be exported as CSV.

Webhook delivery allows real-time forwarding of audit events to a SIEM or alerting system.

---

## Cryptographic Primitives

| Primitive | Usage |
|---|---|
| Ed25519 (EdDSA) | JWT signing / verification |
| AES-256-GCM | TOTP secret encryption; column-level encryption via Vault/Bao |
| bcrypt (cost 12) | Password hashing |
| SHA-256 | Refresh token hashing; password reset token hashing |
| CSPRNG (crypto/rand) | Refresh tokens, CSRF tokens, OIDC codes, reset tokens |
| Haversine formula | Impossible travel distance calculation |

---

## Known Limitations

- **Suspicious hour** check uses server local time, not the user's local time. A user working night shifts will score false positives. This is intentional — the signal is cheap and contributes only 20 points, which alone is insufficient to trigger enforcement.
- **New device detection** uses the raw `User-Agent` string, which can be spoofed. It is a soft signal (+30 points), not a hard gate.
- **GeoIP accuracy** is at the city level at best. Residential ISPs often use anycast or shared exit IPs, which can produce false impossible-travel detections for VPN or mobile users. Tune `risk_threshold_block` upward or disable the feature if your user base heavily uses VPNs.
- The WebAuthn attestation verification step is optional (controlled by `require_hardware_attestation`). Without attestation, you cannot cryptographically verify that the passkey is backed by dedicated hardware.
