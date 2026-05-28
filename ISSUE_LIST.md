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

Status: Audit records are written in the background with no error visibility.

Related files:
- [backend/pkg/middleware/audit.go](/home/m/projects/zerotrust/backend/pkg/middleware/audit.go)

Acceptance criteria:
- Failed writes for critical audit events are visible.
- At minimum, logs or metrics are emitted for failures.
- Preferably define a bounded queue or synchronous strategy for critical audit events.
- Add tests for events such as login, logout, role changes, and revoke actions.

### 4. Make MFA configuration fail fast

Status: If the MFA key is invalid, the application continues running with only a warning.

Related files:
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)

Acceptance criteria:
- Introduce an explicit MFA feature flag.
- If the flag is enabled and the key is invalid, the service must fail to start.
- If the flag is disabled, the behavior should remain intentionally disabled.
- Update README and environment variable documentation.

## P1

### 5. Add graceful shutdown support for background workers

Status: Session cleanup and the service account listener are not tied to the application lifecycle.

Related files:
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)

Acceptance criteria:
- Background goroutines run under a root context.
- SIGTERM triggers cancelation, drain, and clean shutdown behavior.
- Add basic lifecycle logging if practical.

### 6. Separate auth failures from infrastructure failures in protected-route bootstrap

Status: When the frontend fails to load the current user, it redirects straight to the login screen; network problems and expired sessions are treated the same way.

Related files:
- [frontend/src/lib/useAuth.ts](/home/m/projects/zerotrust/frontend/src/lib/useAuth.ts)

Acceptance criteria:
- 401 and 403 responses redirect to login.
- Network failures and 5xx responses show an error state or retry flow.
- Add frontend tests for this behavior.

### 7. Add integration tests for session and refresh race conditions

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
