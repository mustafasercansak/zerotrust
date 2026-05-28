# Issue List

This file collects the urgent and medium-term improvement items identified during the repository review.

## P0

### 1. Clarify and fix refresh session window behavior

State: CLOSED

Status: The current behavior is ambiguous (7-day refresh token lifetime vs 2-minute activity window). The team decision is to use an explicit idle-timeout policy rather than long-lived refresh behavior.

Decision (locked):
- Access token TTL remains 1 minute.
- Refresh idle timeout default is 5 minutes.
- Absolute session timeout default is 8 hours.
- Admin and high-risk roles use stricter idle timeout (2-3 minutes).
- Sensitive operations require recent MFA verification (step-up MFA).

Related files:
- [backend/internal/auth/service.go](/home/m/projects/zerotrust/backend/internal/auth/service.go)
- [backend/internal/session/repository.go](/home/m/projects/zerotrust/backend/internal/session/repository.go)
- [backend/internal/settings/repository.go](/home/m/projects/zerotrust/backend/internal/settings/repository.go)
- [backend/internal/settings/cache.go](/home/m/projects/zerotrust/backend/internal/settings/cache.go)
- [README.md](/home/m/projects/zerotrust/README.md)

Acceptance criteria:
- Remove 7-day refresh policy ambiguity by implementing idle + absolute session limits as runtime-configured settings.
- Add setting keys and defaults:
	- `session_idle_timeout_seconds` default `300`
	- `session_idle_timeout_seconds_admin` default `180`
	- `session_absolute_timeout_seconds` default `28800`
- Enforce role-based idle timeout during refresh/session validation.
- Keep access token TTL at 60 seconds.
- Update README and auth/session docs to reflect actual runtime policy.
- Add tests for:
	- refresh within idle window succeeds
	- refresh outside idle window fails
	- admin idle window stricter than standard user window
	- absolute timeout expires active sessions
	- concurrent refresh behavior remains safe
	- revoke and revoke-others behavior still correct under new timeouts

Status update:
- Implemented in backend (policy + settings + refresh enforcement).
- README updated with runtime policy section.
- Focused auth tests added for idle/admin/absolute timeout behavior.
- Concurrent refresh safety test added (same token race: single winner).
- Session revoke and revoke-others handler tests added and passing.
- Issue 1 is complete.

### 11. Add step-up MFA for sensitive operations

Status: This was split out from Issue 1 to keep session timeout policy delivery focused and shippable.

State: CLOSED

Related files:
- [backend/internal/admin/handler.go](/home/m/projects/zerotrust/backend/internal/admin/handler.go)
- [backend/internal/serviceaccount/handler.go](/home/m/projects/zerotrust/backend/internal/serviceaccount/handler.go)
- [backend/internal/mfa/service.go](/home/m/projects/zerotrust/backend/internal/mfa/service.go)

Acceptance criteria:
- Define "recent MFA" window (for example 5-15 minutes).
- Require recent MFA for role/permission changes and secret-revealing operations.
- Return a clear, localized error when step-up MFA is required.
- Add backend tests for protected sensitive endpoints.

Status update:
- Added `RequireRecentMFA` middleware with 10-minute recent-MFA window.
- Added `/api/v1/mfa/step-up` endpoint to verify code and mark current session as recently verified.
- Applied step-up enforcement to sensitive endpoints:
	- `PATCH /api/v1/admin/users/{id}/roles`
	- `POST /api/v1/admin/service-accounts`
	- `PATCH /api/v1/admin/service-accounts/{id}`
- Added localized `mfa_required` messages in EN/TR.
- Added frontend retry flow: when `mfa_required` is returned, prompt for MFA code, call step-up endpoint, then retry action.
- Added backend middleware tests for deny/allow/marker behavior.

### 2. Remove query parameter token support from the service account SSE endpoint

State: CLOSED

Status: Accepting an access token via URL query string increases token leakage risk.

Related files:
- [backend/internal/serviceaccount/handler.go](/home/m/projects/zerotrust/backend/internal/serviceaccount/handler.go)

Acceptance criteria:
- The endpoint only accepts secure transport methods.
- Token transport outside cookies or the Authorization header is removed.
- Update the frontend SSE flow if needed.

Status update:
- Removed `?token=` fallback from the service account SSE endpoint.
- The endpoint now accepts only the httpOnly `access_token` cookie or `Authorization: Bearer`.
- Frontend already uses the same-origin EventSource URL and did not require changes.
- Added backend tests proving query tokens are rejected and cookie/Bearer token transport still works.

### 3. Make audit log writes reliable and observable

State: CLOSED

Status: Audit records are written in the background with no error visibility.

Related files:
- [backend/pkg/middleware/audit.go](/home/m/projects/zerotrust/backend/pkg/middleware/audit.go)

Acceptance criteria:
- Failed writes for critical audit events are visible.
- At minimum, logs or metrics are emitted for failures.
- Preferably define a bounded queue or synchronous strategy for critical audit events.
- Add tests for events such as login, logout, role changes, and revoke actions.

Status update:
- `audit.Repository.Log` now returns insert/marshal errors instead of swallowing them.
- Critical audit writes use a synchronous, timeout-bound strategy; non-critical protected-route writes still use a timeout-bound background context.
- Audit failures are logged with action/resource/user context where available.
- Audit failures are counted in-process via `audit.WriteFailures()` for future metrics/health integration.
- Audit failure count is exported via Prometheus-compatible `/metrics` as `zerotrust_audit_write_failures_total`.
- Public auth routes and the service-account SSE endpoint are also covered by request-level audit, so malformed, missing-field, unsupported, and unauthenticated attempts leave an audit record.
- CSRF failures are audited before the CSRF middleware returns 403, so browser-session mutation attempts with missing/invalid CSRF tokens leave an audit record without duplicating unrelated 403 responses.
- Protected-route authentication failures are audited before the auth middleware returns 401, so missing/expired/invalid token attempts also leave an audit record.
- Critical protected-route audit events now use stable product event names such as `admin.user.roles_update` and `session.revoke`.
- Critical audit coverage includes user create/status, service-account create/update/status/delete, settings updates, MFA setup/verify/disable/step-up, session revoke, and admin session revoke operations.
- Public auth audit coverage now includes client-credentials token success/failure, login lockout, refresh failure, password reset request/success/failure, and MFA challenge success/failure without logging secrets, passwords, reset tokens, refresh tokens, MFA codes, or pending MFA tokens.
- Audit metadata now includes HTTP `status` and `outcome` (`success`/`failure`) so failed sensitive operations are visible.
- Added explicit `auth.logout` audit event coverage.
- Audit middleware preserves optional response-writer capabilities used by streaming and advanced HTTP handlers without advertising unsupported capabilities.
- Added backend tests for request-level public auth/SSE audit events, CSRF failure audit events, protected-route auth failure audit events, login success/failure/lockout audit events, client-credentials audit events, refresh failure, password reset audit events, MFA challenge audit events, logout audit behavior, critical route audit entries, synchronous critical-route behavior, failed-write observability/counter increments, failed operation outcomes, metrics output, secret/token/code/password redaction, and response-writer capability preservation.

### 4. Make MFA configuration fail fast

State: CLOSED

Status: If the MFA key is invalid, the application continues running with only a warning.

Related files:
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)

Acceptance criteria:
- Introduce an explicit MFA feature flag.
- If the flag is enabled and the key is invalid, the service must fail to start.
- If the flag is disabled, the behavior should remain intentionally disabled.
- Update README and environment variable documentation.

Status update:
- Added explicit `MFA_ENABLED` feature flag.
- `MFA_ENABLED=true` now requires a valid 64-character hex / 32-byte `MFA_ENCRYPTION_KEY`; missing, invalid, or malformed values fail config loading before startup.
- `MFA_ENABLED=false` keeps MFA intentionally disabled; any configured `MFA_ENCRYPTION_KEY` is ignored and not retained in config.
- Invalid `MFA_ENABLED` values fail config loading instead of silently disabling MFA.
- Step-up protected admin actions fail closed while MFA is disabled.
- Updated README and Docker Compose environment examples.
- Added backend config tests for disabled, enabled, missing-key, invalid-key, valid-key, and invalid-flag cases.

## P1

### 5. Add graceful shutdown support for background workers

State: CLOSED

Status: Session cleanup and the service account listener are not tied to the application lifecycle.

Related files:
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)

Acceptance criteria:
- Background goroutines run under a root context.
- SIGTERM triggers cancelation, drain, and clean shutdown behavior.
- Add basic lifecycle logging if practical.

Status update:
- Added a root signal context shared by the HTTP server shutdown path and background workers.
- Service account PostgreSQL listener now runs under the root context and exits cleanly on cancellation without logging expected shutdown as a retryable disconnect.
- Session cleanup now runs under the root context, passes cancellation-aware contexts into repository calls, and exits when shutdown starts.
- Added `WaitGroup`-based worker drain with a bounded timeout after HTTP server shutdown.
- Unexpected HTTP server failures now explicitly cancel the root context and drain background workers before exiting.
- Added lifecycle logs for worker start/stop, signal receipt, server stop, and background worker drain result.
- Added backend tests proving session cleanup uses the root context, stops on cancellation, and worker drain honors context deadlines.

### 6. Separate auth failures from infrastructure failures in protected-route bootstrap

State: CLOSED

Status: When the frontend fails to load the current user, it redirects straight to the login screen; network problems and expired sessions are treated the same way.

Related files:
- [frontend/src/lib/useAuth.ts](/home/m/projects/zerotrust/frontend/src/lib/useAuth.ts)

Acceptance criteria:
- 401 and 403 responses redirect to login.
- Network failures and 5xx responses show an error state or retry flow.
- Add frontend tests for this behavior.

Status update:
- `ApiError` now carries HTTP status so frontend callers can distinguish auth failures from infrastructure failures.
- Protected-route bootstrap redirects to `/auth/login` only for 401/403 responses.
- Network failures and 5xx responses stay on the protected route and show a retryable error state.
- Token refresh during bootstrap preserves network/5xx failures instead of converting them into `missing_token`.
- Dashboard layout now renders localized retry UI for bootstrap infrastructure failures.
- Added frontend tests for 401/403 redirect decisions, network/5xx retryable error decisions, refresh failure preservation, and direct API status propagation.

### 7. Add integration tests for session and refresh race conditions

State: CLOSED

Status: Critical auth paths have limited test coverage. In particular, concurrent refresh, token reuse, and session revoke scenarios are under-tested.

Related files:
- [backend/internal/session/repository.go](/home/m/projects/zerotrust/backend/internal/session/repository.go)
- [backend/internal/auth/service.go](/home/m/projects/zerotrust/backend/internal/auth/service.go)

Acceptance criteria:
- Add a test for two concurrent requests using the same refresh token.
- Add a test for refreshing with a revoked token.
- Add a test covering global revoke behavior after token reuse.
- Add a test for revoking all sessions except the current one.
- Ensure the tests run in CI.

Status update:
- Added Postgres-backed integration tests for refresh/session lifecycle behavior.
- Concurrent refresh with the same refresh token now proves exactly one request wins and the winner's rotated session remains active.
- Refreshing with a revoked token returns `ErrInvalidToken`.
- Reusing an older rotated token beyond the short race grace period revokes all active sessions for the user.
- Revoke-other-sessions now has an integration test proving the current session is preserved and every other active session is revoked.
- Added a short token-reuse grace window so legitimate duplicate refresh requests racing with a successful rotation are not treated as theft while delayed reuse still triggers global revoke.
- CI now starts PostgreSQL for backend tests and sets `TEST_DATABASE_URL`, so these integration tests run in CI.

### 8. Add tests and policy validation for the service account lifecycle

Status: The service account surface is important, but its test coverage is weak.

Related files:
- [backend/internal/serviceaccount/handler.go](/home/m/projects/zerotrust/backend/internal/serviceaccount/handler.go)
- [backend/internal/serviceaccount/service.go](/home/m/projects/zerotrust/backend/internal/serviceaccount/service.go)

Acceptance criteria:
- Add handler- and service-level tests.
- Expired accounts must not be able to obtain tokens.
- Unauthorized scope requests must be rejected.
- Status changes must correctly affect token issuance.

## P2

### 9. Add route-level code splitting to the frontend

Status: Dashboard pages are eagerly loaded into the initial bundle, and the production build reports a large chunk warning.

Related files:
- [frontend/src/App.tsx](/home/m/projects/zerotrust/frontend/src/App.tsx)

Acceptance criteria:
- Dashboard routes are lazy loaded.
- Production bundle size decreases.
- No regressions are introduced in first load or route transitions.

### 10. Clarify auth and session behavior in the README using product language

Status: The security features are documented, but some actual runtime behaviors remain ambiguous.

Related files:
- [README.md](/home/m/projects/zerotrust/README.md)

Acceptance criteria:
- Clearly document the browser auth flow in the README.
- Clearly describe refresh policy and idle-timeout behavior.
- Document logout and revoke semantics.
- Clearly explain MFA prerequisites and the service account auth model.

### 12. Expand step-up MFA coverage to destructive admin actions

Status: Step-up MFA currently protects role updates and service-account create/update. Additional destructive admin operations should be explicitly reviewed and protected for defense in depth.

Related files:
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)
- [backend/internal/admin/handler.go](/home/m/projects/zerotrust/backend/internal/admin/handler.go)
- [backend/internal/serviceaccount/handler.go](/home/m/projects/zerotrust/backend/internal/serviceaccount/handler.go)

Acceptance criteria:
- Decide and document which destructive actions require recent MFA.
- If selected, enforce step-up MFA on:
	- `PATCH /api/v1/admin/users/{id}/status`
	- `DELETE /api/v1/admin/users/{id}/sessions`
	- `DELETE /api/v1/admin/users/{id}/sessions/{sessionId}`
	- `PATCH /api/v1/admin/service-accounts/{id}/status`
	- `DELETE /api/v1/admin/service-accounts/{id}`
- Add backend tests proving `mfa_required` is returned when recent MFA is missing.
- Add/update frontend UX to retry those actions after `/api/v1/mfa/step-up`.
