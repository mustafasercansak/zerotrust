# System Settings Reference

System settings are runtime-configurable key-value pairs stored in the `system_settings` PostgreSQL table. They take effect without a server restart (the settings cache is invalidated on write).

Admin users can view and modify settings via the admin panel or the API:

- `GET /api/v1/admin/settings` — list all settings
- `PATCH /api/v1/admin/settings` — update a single setting (requires admin role + step-up MFA)

All values are stored as text strings. The type column below indicates how the value is interpreted by the backend.

---

## Session Policy

| Key | Type | Default | Description |
|---|---|---|---|
| `max_sessions_per_user` | integer | `5` | Maximum number of concurrent sessions per user. When exceeded on login, the oldest session is evicted. Set to `1` for strict single-session semantics. |
| `session_idle_timeout_seconds` | integer | `300` | Idle timeout for regular users (5 minutes). A session is considered expired if `last_used_at` is older than this value. The frontend polls this setting and enforces the countdown client-side. |
| `session_idle_timeout_seconds_admin` | integer | `180` | Idle timeout for admin users (3 minutes). Admins have a shorter timeout because their sessions carry elevated privilege. |

---

## Authentication Policy

| Key | Type | Default | Description |
|---|---|---|---|
| `password_complexity` | string | `low` | Password complexity requirement. Accepted values: `low` (any password), `medium` (8+ chars, mixed case), `high` (12+ chars, mixed case, digit, symbol). Enforced on registration, password change, and password reset. |
| `max_login_attempts` | integer | `5` | Number of failed attempts before the first progressive lockout (1 minute). Subsequent failures escalate to 5 and then 30 minutes. |
| `global_mfa_required` | boolean | `false` | When `true`, every user must complete MFA on login regardless of whether risk-based auth is enabled. Users without MFA enrolled will be prompted to set it up before they can proceed. |

---

## Risk-Based Adaptive Authentication

| Key | Type | Default | Description |
|---|---|---|---|
| `risk_based_auth_enabled` | boolean | `false` | Master switch for adaptive authentication. When `false`, risk scores are computed and logged but no enforcement action is taken. |
| `risk_threshold_mfa` | integer | `40` | If the risk score meets or exceeds this value and the user has MFA enrolled, MFA is required for this login session regardless of `global_mfa_required`. |
| `risk_threshold_block` | integer | `80` | If the risk score meets or exceeds this value, the login is blocked outright (`high_risk_blocked`). |
| `risk_score_impossible_travel` | integer | `80` | Score added when impossible travel is detected. |
| `risk_score_new_device` | integer | `30` | Score added when the login comes from a previously unseen device fingerprint. |
| `risk_score_suspicious_hours` | integer | `20` | Score added for logins inside the suspicious-hour window. |
| `risk_score_failed_attempt` | integer | `15` | Score added per recent failed attempt. |
| `risk_failed_attempt_max_score` | integer | `45` | Upper cap for failed-attempt contribution. |
| `risk_suspicious_hour_start` | integer | `23` | Suspicious-hour window start (0-23). |
| `risk_suspicious_hour_end` | integer | `5` | Suspicious-hour window end (0-23). |
| `risk_impossible_travel_velocity_kmh` | integer | `800` | Velocity threshold for impossible travel detection. |
| `risk_impossible_travel_window_hours` | integer | `24` | Time window considered for impossible travel checks. |
| `risk_impossible_travel_min_distance_km` | integer | `10` | Minimum geo distance before travel checks are evaluated. |

Risk score defaults: impossible travel (+80), new device (+30), suspicious hour (+20), recent failed attempts (+15 per attempt, max +45). See [security model](security-model.md#7-risk-based-adaptive-authentication) for details.

New-device detection uses a normalized device fingerprint (`os`, `os_version`, `browser`, browser major version, mobile flag, architecture) when client device metadata is available. It falls back to normalized user-agent matching when metadata is missing.

---

## IP and Country Allowlist

When either allowlist is non-empty, logins from sources not on the list are rejected before password verification.

| Key | Type | Default | Description |
|---|---|---|---|
| `ip_allowlist` | string | _(empty)_ | Comma-separated list of allowed CIDR ranges (e.g., `10.0.0.0/8,203.0.113.0/24`). Empty = no restriction. |
| `country_allowlist` | string | _(empty)_ | Comma-separated list of allowed two-letter ISO 3166-1 alpha-2 country codes (e.g., `TR,DE,US`). Empty = no restriction. Requires a valid GeoIP database (`GEOIP_DB_PATH`). |

---

## Device Trust

When `device_trust_enabled` is `true`, every login attempt has its device fingerprint checked against the policy below. Failures return `device_blocked`.

| Key | Type | Default | Description |
|---|---|---|---|
| `device_trust_enabled` | boolean | `false` | Master switch for device trust enforcement. |
| `device_trust_allowed_os` | string | _(empty)_ | Comma-separated list of permitted OS families (e.g., `Windows,macOS,Linux`). Empty = any OS allowed. |
| `device_trust_min_os_version_mac` | string | _(empty)_ | Minimum macOS version (e.g., `13.0`). Empty = no version gate. |
| `device_trust_min_os_version_win` | string | _(empty)_ | Minimum Windows version (e.g., `10.0`). Empty = no version gate. |
| `device_trust_allowed_browsers` | string | _(empty)_ | Comma-separated list of permitted browsers (e.g., `Chrome,Firefox,Safari`). Empty = any browser allowed. |
| `device_trust_min_browser_version_chrome` | string | _(empty)_ | Minimum Chrome version (e.g., `120`). Empty = no version gate. |
| `device_trust_min_browser_version_safari` | string | _(empty)_ | Minimum Safari version. |
| `device_trust_min_browser_version_firefox` | string | _(empty)_ | Minimum Firefox version. |
| `device_trust_min_browser_version_edge` | string | _(empty)_ | Minimum Edge version. |
| `device_trust_block_mobile` | boolean | `false` | When `true`, logins from mobile browsers are rejected regardless of other settings. |

---

## WebAuthn / Passkeys

| Key | Type | Default | Description |
|---|---|---|---|
| `require_hardware_attestation` | boolean | `false` | When `true`, passkey registration only accepts attestation backed by an attestation certificate chain (`basic_full` / `attca`); `none` and self-attestation (`basic_surrogate`), which any software authenticator can forge with an arbitrary AAGUID, are rejected. **Advisory:** the server runs without an MDS/trust-anchor configuration, so the certificate chain is *not* anchored to vendor root CAs — a determined attacker could still forge a self-signed chain. Treat this as a gate against off-the-shelf software passkeys (e.g., iCloud Keychain synced passkeys), not a strict hardware-only guarantee; a startup warning is logged when the setting is enabled. |

---

## Audit Webhook

| Key | Type | Default | Description |
|---|---|---|---|
| `webhook_enabled` | boolean | `false` | When `true`, audit events are forwarded to `webhook_url` as HTTP POST requests. |
| `webhook_url` | string | _(empty)_ | URL to receive audit events. Each request body is a single JSON audit entry. Delivery is fire-and-forget with a 5-second timeout. Delivery failures are themselves written to the audit log. Must use `https` unless `webhook_allow_insecure` is enabled. |
| `webhook_allow_insecure` | boolean | `false` | Allows plain `http://` webhook URLs. Development only — webhook payloads contain user email, IP and security-event details and must travel over TLS in production. |

Outbound webhook dispatch is throttled to one event per IP+action combination per minute, so a flood of identical events cannot exhaust the channel.

Test the webhook without changing settings: `POST /api/v1/admin/settings/webhook/test` with `{ "url": "…" }`.

---

## Modifying Settings

Settings are validated by the backend before being written. Unknown keys are rejected with `unknown_setting`. Values that fail type validation (e.g., a non-integer for `max_sessions_per_user`) are rejected with `invalid_value`.

The in-process settings cache is invalidated synchronously on a successful `PATCH`. Subsequent requests on any goroutine will read the new value from the database.

The write operation is audited as `settings.update` with the previous and new value in the metadata field.
