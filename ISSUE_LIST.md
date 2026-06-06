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

State: CLOSED

Status: The service account surface is important, but its test coverage is weak.

Related files:
- [backend/internal/serviceaccount/handler.go](/home/m/projects/zerotrust/backend/internal/serviceaccount/handler.go)
- [backend/internal/serviceaccount/service.go](/home/m/projects/zerotrust/backend/internal/serviceaccount/service.go)

Acceptance criteria:
- Add handler- and service-level tests.
- Expired accounts must not be able to obtain tokens.
- Unauthorized scope requests must be rejected.
- Status changes must correctly affect token issuance.

Status update:
- Added auth service tests proving client-credentials tokens include only the stored service-account scopes.
- Added tests proving expired, inactive, and invalid-secret service accounts cannot obtain tokens.
- Added a status-change test proving token issuance stops after a service account becomes inactive.
- Added service-level scope policy tests for allowed scopes, unknown scopes, missing caller claims, and scopes the caller does not hold.
- Added handler tests for create/update scope-policy errors and not-found behavior.
- `PATCH /service-accounts/{id}/status` and `DELETE /service-accounts/{id}` now return `404 not_found` when the target account does not exist instead of silently succeeding.

## P2

### 9. Add route-level code splitting to the frontend

State: CLOSED

Status: Dashboard pages are eagerly loaded into the initial bundle, and the production build reports a large chunk warning.

Related files:
- [frontend/src/App.tsx](/home/m/projects/zerotrust/frontend/src/App.tsx)
- [frontend/src/components/DashboardLayout.tsx](/home/m/projects/zerotrust/frontend/src/components/DashboardLayout.tsx)

Acceptance criteria:
- Dashboard routes are lazy loaded.
- Production bundle size decreases.
- No regressions are introduced in first load or route transitions.

Status update:
- Converted all dashboard page imports in `App.tsx` to dynamic imports using `React.lazy`.
- Wrapped the `<Outlet />` element in `DashboardLayout.tsx` with `<Suspense>` using a centered `CircularProgress` loading fallback spinner.
- Verified that the build splits the chunks successfully and type checks pass.


### 10. Clarify auth and session behavior in the README using product language

State: CLOSED

Status: The security features are documented, but some actual runtime behaviors remain ambiguous.

Related files:
- [README.md](/home/m/projects/zerotrust/README.md)

Acceptance criteria:
- Clearly document the browser auth flow in the README.
- Clearly describe refresh policy and idle-timeout behavior.
- Document logout and revoke semantics.
- Clearly explain MFA prerequisites and the service account auth model.

Status update:
- Extensively updated the README with clear, developer-facing product language explaining the browser cookies auth flow, system timeout cache behaviors, logout/revocation semantics (with token reuse detection), TOTP MFA step-up requirements, and M2M service accounts token issue structure.

### 12. Expand step-up MFA coverage to destructive admin actions

State: CLOSED

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

Status update:
- Enforced `stepUpMFA` middleware on user status patch, all user session deletions, service account status patch, and service account deletion endpoints in `main.go`.
- Added `mfaPrompt` under the `admin` translation block for localization.
- Integrated `runWithStepUp` in `UsersPage.tsx` and wrapped user status changes, bulk session revocations, and individual session revocations.
- Wrapped service account status changes and revocation actions with `runWithStepUp` in `ServiceAccountsPage.tsx`.
- Verified that all backend tests pass and the frontend builds cleanly.

### 13. Gracefully handle globally disabled MFA on the frontend

State: CLOSED

Status: When MFA is disabled in the backend settings (MFA_ENABLED=false), the MFA-related API endpoints return 404 because they are not registered in the router. Clicking "Enable 2FA" on the frontend MfaPage results in a generic "Server error" instead of a clean, informative message indicating that MFA is disabled by the administrator.

Related files:
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)
- [frontend/src/pages/dashboard/MfaPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/MfaPage.tsx)

Acceptance criteria:
- Register the `/api/v1/mfa/status` endpoint regardless of the `MFA_ENABLED` flag, and have it return `{ "enabled": false, "supported": false }` when disabled.
- Alternatively, have the frontend detect a 404 or a `"supported": false` response from `/api/v1/mfa/status` and render a clear warning message: "Multi-factor authentication is currently disabled by the system administrator."
- Disable the "Enable 2FA" button when MFA is globally disabled.

Status update:
- Updated the backend `/api/v1/mfa/status` endpoint to return a `supported` flag (`true` when active).
- Registered a fallback `/api/v1/mfa/status` handler when `MFA_ENABLED=false` that returns `{"enabled": false, "supported": false}` instead of triggering a 404 error.
- Updated frontend `api.ts` types to support `supported` attribute.
- Created `unsupported` localization descriptors and updated frontend `MfaPage.tsx` to cleanly present a warning indicating MFA is disabled by system administrator.
- Modified `docker-compose.yml` environment block to load `MFA_ENABLED` and `MFA_ENCRYPTION_KEY` variables from the `.env` file instead of hardcoding them.
- Updated `generate-secrets.sh` to automatically output random `MFA_ENCRYPTION_KEY` values to `infra/.env`, and translated the entire shell script from Turkish to English for better accessibility.

### 14. Add QR code representation on MFA setup page

State: CLOSED

Status: The MFA setup flow only displays a manual secret string and an "open in app" link. It does not display a QR code, which is the standard for scanning with mobile authenticator apps.

Related files:
- [frontend/package.json](/home/m/projects/zerotrust/frontend/package.json)
- [frontend/src/pages/dashboard/MfaPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/MfaPage.tsx)

Acceptance criteria:
- Install a client-side QR code generator package (e.g. `qrcode.react`) in the frontend.
- Render the QR code in `MfaPage.tsx` using `otp_auth_url` from the setup response.
- Style the QR code nicely with a container, a clean border, and clear user instructions in both English and Turkish.
- Verify that the layout remains responsive and fits well in the dashboard view.

Status update:
- Installed `qrcode.react` package in the frontend application.
- Integrated `QRCodeSVG` into the MFA setup block inside `MfaPage.tsx`, displaying the generated QR code in a high-contrast container for seamless mobile scanning.
- Confirmed the page layout is responsive and successfully verified compilation through a clean production build (`npm run build`).

### 15. Create project showcase and embed screenshots in README

State: CLOSED

Status: The project lacked visual showcase elements. The screenshots were outdated or in Turkish, and there was no visual demo page.

Related files:
- [README.md](/home/m/projects/zerotrust/README.md)
- [showcase.html](/home/m/projects/zerotrust/showcase.html)
- [docs/images/dashboard.png](/home/m/projects/zerotrust/docs/images/dashboard.png)
- [docs/images/session_management.png](/home/m/projects/zerotrust/docs/images/session_management.png)
- [docs/images/mfa_setup.png](/home/m/projects/zerotrust/docs/images/mfa_setup.png)

Acceptance criteria:
- Embed screenshots in `README.md` to highlight the UI capabilities.
- Create a premium standalone showcase HTML page containing project features and screenshots.
- Ensure the screenshots are in English for international accessibility.

Status update:
- Embedded references to the three core UI screenshots in `README.md`.
- Created a beautifully styled standalone `showcase.html` with vanilla CSS, modern typography, glassmorphic cards, and detailed feature breakdowns.
- Collaborated with the user to replace the screenshots with English interface captures.

### 16. Publish showcase page to GitHub Pages

State: CLOSED

Status: The showcase page is local and needs to be hosted on GitHub Pages so others can view it easily from the web.

Related files:
- [showcase.html](/home/m/projects/zerotrust/showcase.html)
- [docs/index.html](/home/m/projects/zerotrust/docs/index.html)
- [README.md](/home/m/projects/zerotrust/README.md)

Acceptance criteria:
- Move and rename `showcase.html` to `docs/index.html` so it functions as the index entrypoint for GitHub Pages deployment.
- Update image source paths inside the new `docs/index.html` from `docs/images/` to `images/` (since they are now adjacent relative paths).
- Update references in `README.md` or other docs if they link to `showcase.html`.
- Add a GitHub Pages URL reference in `README.md` so visitors can access the live showcase.

Status update:
- Moved the `showcase.html` file to `docs/index.html` to serve as the entry page for the repository's GitHub Pages site.
- Updated all image source paths within `docs/index.html` to use relative references (`images/...`).
- Added the public live showcase URL to [README.md](file:///home/m/projects/zerotrust/README.md).

### 17. Optimize CSRF token lifetime and prevent rotation races

State: CLOSED

Status: Currently, the CSRF token is regenerated on every call to `writeCookies`, which runs during access token refresh (every 1 minute). This frequency can lead to race conditions for concurrent browser requests if they submit an old token right as rotation completes.

Related files:
- [backend/internal/auth/handler.go](/home/m/projects/zerotrust/backend/internal/auth/handler.go)
- [backend/pkg/middleware/csrf.go](/home/m/projects/zerotrust/backend/pkg/middleware/csrf.go)

Acceptance criteria:
- Reuse the existing `csrf_token` cookie if present and valid in the request, instead of generating a new one on every 1-minute access-token refresh.
- Only generate a new CSRF token on initial login or when the session is initialized/replaced.
- Ensure security properties are maintained (tokens remain cryptographically secure and match the double-submit header validation).

Status update:
- Updated the `writeCookies` signature in `backend/internal/auth/handler.go` to accept the `*http.Request` pointer.
- Modified the cookie-writing logic to check for an existing `csrf_token` cookie in the request and reuse its value.
- Re-generation is now limited to scenarios where no valid CSRF cookie is present (e.g. initial login, fresh sessions).
- Ensured all callers in the auth handler pass the request parameter, and all unit/integration tests pass cleanly.

---

### 18. Implement user avatar upload, serving, and deletion

State: CLOSED

Status: Database migration `000012_avatar_local_storage.up.sql` added avatar schema fields (`avatar_object_key` and `avatar_size`) to the `users` table, and the user profile models support `HasAvatar`. However, the API endpoints for uploading, downloading/serving, and deleting avatar files are not yet implemented.

Related files:
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)
- [backend/internal/user/repository.go](/home/m/projects/zerotrust/backend/internal/user/repository.go)
- [frontend/src/lib/api.ts](/home/m/projects/zerotrust/frontend/src/lib/api.ts)

Acceptance criteria:
- Create backend endpoints:
  - `POST /api/v1/me/avatar` to upload an avatar image (with size limits and image file validation).
  - `GET /api/v1/me/avatar` or static asset routing to fetch/stream the avatar image.
  - `DELETE /api/v1/me/avatar` to remove the avatar.
- Store avatar files securely (either locally in a bounded storage directory or via object storage).
- Integrate avatar upload and view functionality in the frontend User Settings/Profile page.

Status update:
- Implemented `UpdateAvatar` database mutation in `backend/internal/user/repository.go` and user service wrapper.
- Added `/api/v1/me/avatar` POST, GET, and DELETE endpoints and `/api/v1/users/{id}/avatar` GET endpoint in `backend/cmd/server/main.go`.
- Files are validated to ensure only JPEG/PNG images under 2MB are accepted, and saved to a local files directory (`uploads/avatars/`) named by their unique user ID.
- Updated frontend `api.ts` with file-upload compatibility (`FormData`) and registered the avatar API functions.
- Integrated upload and delete controls directly inside the Profile Settings dialog in `DashboardLayout.tsx`.
- Updated users list page `UsersPage.tsx` to load user avatar images.

---

### 19. Verify magic bytes for avatar uploads using content sniffing

State: CLOSED

Status: The avatar upload endpoint validates file types using the client-provided `Content-Type` header. An attacker can spoof this header to upload HTML/JS payloads. If the file is requested directly, it might be sniffed by the browser as `text/html`, leading to Stored XSS.

Related files:
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)

Acceptance criteria:
- Sniff the first 512 bytes of the uploaded file on the backend using `http.DetectContentType`.
- Reject the upload if the sniffed content type is not `image/jpeg` or `image/png`.
- Reset the file read pointer using `file.Seek(0, io.SeekStart)` before writing it to disk.

Status update:
- Integrated content sniffing via `http.DetectContentType` on the first 512 bytes of uploaded avatars.
- Rejected requests containing non-image bytes (non `image/jpeg` and `image/png`) with a bad request status.
- Seeking back to the beginning of the stream before writing prevents incomplete reads.

---

### 20. Rotate CSRF tokens during login state changes and limit reuse to background refreshes

State: CLOSED

Status: The current CSRF token reuse logic reuses the active cookie on initial login, registration, and MFA challenge steps if a token cookie is already present in the browser. This misses proper rotation on privilege/session transitions, posing a potential session/CSRF fixation risk.

Related files:
- [backend/internal/auth/handler.go](/home/m/projects/zerotrust/backend/internal/auth/handler.go)

Acceptance criteria:
- Limit CSRF token reuse strictly to requests sent to the `/api/v1/auth/refresh` endpoint.
- Always rotate (generate a new token) during login, registration, and MFA challenge/verification endpoints.

Status update:
- Restricted the CSRF reuse condition in `writeCookies` to requests directed specifically to `/api/v1/auth/refresh`.
- All other authentication entrypoints (login, registration, MFA steps) now force generation of a new token.

---

### 21. Fix user name, surname, and avatar alignment clipping on Users page

State: CLOSED

Status: In the users list table, rows displaying both a name/surname and email address are taller than standard single-line rows, causing the name text to be pushed up and clipped at the top of the cell.

Related files:
- [frontend/src/components/ResourceTablePage.tsx](/home/m/projects/zerotrust/frontend/src/components/ResourceTablePage.tsx)
- [frontend/src/pages/dashboard/UsersPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/UsersPage.tsx)

Acceptance criteria:
- Add an optional `rowHeight` prop to `ResourceTablePage`.
- Set `rowHeight={64}` in `UsersPage.tsx` to give rows with multi-line text enough room.
- Ensure the name and email text layouts are vertically centered and have proper line heights.

Status update:
- Added `rowHeight` prop to the reusable `ResourceTablePageProps` type and passed it to the MUI `DataGrid`.
- Configured a generous `rowHeight={64}` for the users page table to handle name and email entries safely.
- Wrapped user column text components with a column-flex container having `justifyContent: "center"` and explicit `lineHeight: 1.2`.

---

## Future Improvements

### 22. Implement User Self-Service Security & Session Management Page

State: CLOSED

Status: Users can modify their names and configure MFA, but they lack a dedicated security panel to audit and revoke their active login sessions.

Related files:
- [frontend/src/pages/dashboard/SettingsPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/SettingsPage.tsx)
- [backend/internal/session/handler.go](/home/m/projects/zerotrust/backend/internal/session/handler.go)

Acceptance criteria:
- Create a "Security Settings" tab or section inside the User Settings Page.
- Fetch and display the logged-in user's active sessions (IP, device info, last active timestamp, and indicator for "current session").
- Provide buttons to revoke a specific session or "log out from all other devices".

Status update:
- Converted `SettingsPage.tsx` into a tabbed user settings panel accessible to all authenticated users.
- Embedded the Profile card (name changes, avatar uploads/deletions) directly inside the **Profile Settings** tab.
- Rendered the reusable `<SessionsPage />` component inside the **Security & Sessions** tab, enabling self-revocation.
- Kept the **System Settings** tab (concurrent session configurations) admin-only.
- Exceeded criteria by removing the layout modal dialog and routing user profile links to settings, creating a cohesive user security dashboard.

---

### 23. Add Interactive Charts and Search Filters to Admin Audit Logs

State: CLOSED

Status: The current admin audit page shows a simple, un-paged table of raw log events with no search, filtering, or visual metrics.

Related files:
- [frontend/src/pages/dashboard/AuditPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/AuditPage.tsx)
- [backend/internal/audit/handler.go](/home/m/projects/zerotrust/backend/internal/audit/handler.go)

Acceptance criteria:
- Integrate search inputs and filters (filter by user, IP address, action name, or outcome).
- Render visual analytics charts (e.g., success vs. failure ratios over time) to easily spot brute-force attacks or access anomalies.

Status update:
- Extended the backend repository and handler with dynamic `outcome` query parameters and a database query summarizing success/failure trends for the last 7 days.
- Designed and built a lightweight, native SVG line chart to plot daily security anomalies without relying on extra UI chart dependencies.
- Added quick presets/tabs (All, Failures, Auth Requests) inside the `ResourceTablePage` layout to slice audit events dynamically.

---

### 24. Implement Geolocation and Context-Aware MFA challenges for logins

State: CLOSED

Status: Logins are validated purely by username/password + MFA. The system does not verify context (e.g. traveling between countries or using unknown devices).

Related files:
- [backend/internal/auth/service.go](/home/m/projects/zerotrust/backend/internal/auth/service.go)
- [backend/pkg/geoip/geoip.go](/home/m/projects/zerotrust/backend/pkg/geoip/geoip.go)

Acceptance criteria:
- Parse remote IP geolocation using a free GeoIP database library (such as MaxMind GeoLite2).
- Detect impossible travel (e.g., logging in from two distant locations within a short time) or first-time devices.
- Force a step-up MFA challenge or alert the user via email on anomaly detection.

Status update:
- Created a GeoIP lookup service package using the `oschwald/geoip2-golang` library with a robust mock fallback for local development and unit tests.
- Implemented Haversine distance mathematical formula to calculate physical distances and velocities between successive user login IPs.
- Configured impossible travel checking (>800 km/h) and first-time device checking against active sessions.
- Extended the `Mailer` interface to dispatch security alert emails when anomalies are detected, and logged `login.anomaly` events to the admin audit log.
- Added comprehensive unit tests in `anomaly_test.go` and `geoip_test.go`.

---

### 25. Add Service Account Client Secret Rotation with Grace Period

State: CLOSED

Status: Service account secrets are static and displayed only once at creation. If a client secret needs rotation, the service account must be recreated, leading to consumer downtime.

Related files:
- [backend/internal/serviceaccount/service.go](/home/m/projects/zerotrust/backend/internal/serviceaccount/service.go)
- [backend/internal/serviceaccount/handler.go](/home/m/projects/zerotrust/backend/internal/serviceaccount/handler.go)

Acceptance criteria:
- Create an API endpoint and UI action to rotate a service account's client secret.
- Allow a temporary overlap window (grace period, e.g. 1 hour) where both the old and new client secrets remain valid to prevent client script failures during migration.
- Automatically deprecate the old secret once the window expires.

Status update:
- Added `000013_service_account_secret_rotation` migration to store the old secret hash and its expiration timestamp.
- Updated `ClientCredentials` authentication logic to accept the old secret if it matches and is within the 1-hour grace period.
- Created `/api/v1/admin/service-accounts/{id}/rotate` endpoint requiring step-up MFA.
- Added a "Rotate Secret" action button to the frontend Service Accounts table grid, triggering the MFA challenge and displaying the new secret key.
- Added unit tests for secret grace period validation.

---

### 26. Extend Global System Settings with Custom Security Policies

State: CLOSED

Status: Global system settings cannot configure password complexity, global MFA requirements, or maximum login lockout attempts, limiting security options.

Related files:
- [backend/internal/settings/handler.go](/home/m/projects/zerotrust/backend/internal/settings/handler.go)
- [backend/internal/auth/service.go](/home/m/projects/zerotrust/backend/internal/auth/service.go)
- [frontend/src/pages/dashboard/SettingsPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/SettingsPage.tsx)

Acceptance criteria:
- Seed and support database keys for `password_complexity` (`"low" | "medium" | "strong"`), `global_mfa_required` (`"true" | "false"`), and `max_login_attempts` (`1` to `20`).
- Enforce password complexity policies during register and reset dynamically.
- Enforce lockout thresholds dynamically in `progressiveLockout`.
- Support inline MFA onboarding/setup during login if `global_mfa_required` is enabled and the user does not have MFA configured.
- Extend the frontend admin "System Settings" panel to expose inputs/controls for these settings, requiring step-up MFA challenge verification to save changes.

Status update:
- Created migrations `000014_system_security_policies.up.sql` and `down.sql` to seed default settings.
- Implemented `PasswordWithComplexity` and wired setting checks in user registration and reset flows.
- Extended `progressiveLockout` to read dynamic `max_login_attempts` from custom settings.
- Handled inline MFA onboarding (returning setup secret/QR URL) during login when global MFA is required and MFA is not yet enabled.
- Extended settings handler and service cache, securing Settings update checks with step-up MFA.
- Added settings panel inputs (complexity level, max login attempts, global MFA requirement) in frontend Admin settings.

---

### 27. Update Project Documentation, UI Screenshots, and Showcase Page

State: CLOSED

Status: Recent additions (unified settings panel, session management within settings, and avatar sniffing features) need to be documented. The screenshots in documentation and the static GitHub Pages showcase file are outdated and do not show the unified settings UI.

Related files:
- [README.md](/home/m/projects/zerotrust/README.md)
- [docs/index.html](/home/m/projects/zerotrust/docs/index.html)

Acceptance criteria:
- Update [README.md](file:///home/m/projects/zerotrust/README.md) with details about the new profile and session settings layout.
- Review and update UI screenshots under `docs/images/` to show the unified settings experience.
- Update the public-facing showcase page at [docs/index.html](file:///home/m/projects/zerotrust/docs/index.html) with screenshots and descriptions highlighting the self-service security and profile uploader.

Status update:
- Updated [README.md](file:///home/m/projects/zerotrust/README.md) screenshots section title to "Unified Settings & Session Management".
- Added the new protected avatar upload/fetch/delete HTTP endpoints to the API reference tables in `README.md`.
- Updated the public-facing showcase page at [docs/index.html](file:///home/m/projects/zerotrust/docs/index.html) (Showcase 2) to detail the Unified Settings & Session Audits feature.
- Documented that the administrative screenshots can be refreshed under `docs/images/session_management.png` to depict the unified tabbed dashboard.

---

### 28. Add MFA Backup Recovery Codes

State: CLOSED

Status: If a user loses their TOTP authenticator device, they are completely locked out and must contact an administrator to manually disable their MFA.

Related files:
- [backend/internal/mfa/service.go](/home/m/projects/zerotrust/backend/internal/mfa/service.go)
- [backend/internal/auth/service.go](/home/m/projects/zerotrust/backend/internal/auth/service.go)
- [frontend/src/pages/auth/LoginPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/auth/LoginPage.tsx)

Acceptance criteria:
- Generate a set of 8-10 single-use, cryptographically secure recovery codes (e.g., format `xxxx-xxxx-xxxx`) during MFA setup/registration.
- Store these codes as salted bcrypt hashes in a new database table or schema field (`mfa_recovery_codes`).
- Show the recovery codes to the user during setup and force them to confirm they saved them before enabling MFA.
- Allow users to enter a backup recovery code on the MFA login verification stage to bypass TOTP challenge.
- Once a recovery code is used, delete it or mark it as invalid, and notify the user via log/audit event.

Status update:
- Added columns `recovery_codes` and `pending_recovery_codes` in the `user_mfa` table.
- Implemented cryptographically secure generation of 8 recovery codes formatted as `xxxx-xxxx-xxxx` during MFA setup.
- Hashed the recovery codes using `bcrypt` and stored them in `pending_recovery_codes` during setup, then promoted them to `recovery_codes` upon successful verification.
- Modified the validation flow to check for recovery codes as a fallback to TOTP. If a recovery code is used, it is atomically removed from the user's record (single-use constraint).
- Updated the frontend `MfaPage.tsx` and `LoginPage.tsx` to display recovery codes during setup and require a confirmation checkbox.
- Configured the login verification input field on `LoginPage.tsx` to accept and validate either 6-digit TOTP codes or 14-character recovery codes.
- Added comprehensive unit tests validating single-use logic, setup promotion, and rejection of invalid codes.

---

### 29. Make Anomaly Email Alerts Resilient

State: CLOSED

Status: Security email alerts for login anomalies (impossible travel, new devices) are dispatched in fire-and-forget goroutines. If the SMTP server experiences transient issues, the alerts are silently lost.

Related files:
- [backend/internal/auth/service.go](/home/m/projects/zerotrust/backend/internal/auth/service.go)
- [backend/pkg/mailer/mailer.go](/home/m/projects/zerotrust/backend/pkg/mailer/mailer.go)
- [backend/pkg/mailer/resilient.go](/home/m/projects/zerotrust/backend/pkg/mailer/resilient.go)

Acceptance criteria:
- Define an outbox database schema or a memory-backed retrying queue for outgoing mail events.
- If email delivery fails, retry sending with exponential backoff and a maximum retry limit.
- Log failures to the server console and audit logs if the maximum retry limit is reached.

Status update:
- Created a memory-backed retrying queue wrapper `ResilientMailer` inside `backend/pkg/mailer/resilient.go`.
- Implemented exponential backoff delay based on attempts (1s, 2s, 4s, 8s, 16s) up to a max limit of 5 retries.
- Wired the resilient queue to start under the signal lifecycle root context inside `main.go` and drain/stop cleanly during graceful shutdown.
- Configured a callback in `main.go` to lookup the corresponding user and log an `auth.security_alert.delivery_failure` failure record to the audit database if all attempts fail.
- Replaced the fire-and-forget goroutine in `auth/service.go` with direct synchronous queuing.
- Added comprehensive unit tests in `resilient_test.go` covering success, retry backoff, max attempts, and audit failures.

---

### 30. Implement API Rate Limiting for Protected Endpoints

State: CLOSED

Status: Rate limiting is currently set for public auth/token routes, but generic protected endpoints (such as avatar uploads or session queries) have no rate limits and can be abused by automated scripts.

Related files:
- [backend/pkg/middleware/ratelimit.go](/home/m/projects/zerotrust/backend/pkg/middleware/ratelimit.go)
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)

Acceptance criteria:
- Introduce token-bucket rate limiting based on authenticated `claims.UserID` or IP address for generic protected routes.
- Return HTTP 429 with appropriate `Retry-After` headers when the rate limit threshold is exceeded.
- Provide unit tests showing that authenticated clients are rate-limited independently.

Status update:
- Updated the key generation logic inside `ratelimit.go` to inspect the JWT context claims. If `claims.UserID` is present, it is used as the rate limit key; otherwise, it falls back to the client IP address.
- Initialized a `protectedRL` rate limiter instance in `main.go` restricting clients to 100 requests per minute.
- Configured the `protectedRL.Middleware()` wrapper inside the protected route group in `main.go`, positioned right after the authentication guard.
- Added a full test suite in `ratelimit_test.go` confirming that unauthenticated users are rate limited by IP, authenticated users are rate limited by User ID, independent user counts are maintained, and HTTP 429 returns with appropriate `Retry-After` and metadata headers.

---

### 31. Database Connection Pool and Timeout Tuning

State: CLOSED

Status: The database and Redis client connection pools are initialized using hardcoded or default library options. High traffic or network latency could lead to pool exhaustion or socket timeouts.

Related files:
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)

Acceptance criteria:
- Load dynamic settings (e.g., `DATABASE_MAX_CONNS`, `DATABASE_MIN_CONNS`, `DATABASE_CONN_TIMEOUT`, `REDIS_POOL_SIZE`) from configuration/environment variables.
- Log connection pool metrics or utilization checks upon server startup and lifecycle updates.

Status update:
- Added `DatabaseMaxConns`, `DatabaseMinConns`, `DatabaseConnTimeout`, and `RedisPoolSize` parsing logic to `loadConfig` in `main.go`.
- Configured defaults: `DATABASE_MAX_CONNS` (20), `DATABASE_MIN_CONNS` (2), `DATABASE_CONN_TIMEOUT` ("5s"), and `REDIS_POOL_SIZE` (10).
- Initialized `pgxpool` using `NewWithConfig` dynamically applying the pool limits and timeout values.
- Initialized `redis.Client` applying the pool size value.
- Added startup log entries displaying the initial pool metrics for both PostgreSQL and Redis.
- Registered a periodic background metrics worker inside the main `WaitGroup` logging connection pool stats every 5 minutes.
- Added test coverage in `main_test.go` verifying pool parameter parsing and fallback values.

---

### 32. Achieve High Backend Test Coverage

State: CLOSED

Status: We have implemented all P0, P1, P2 and Future Improvements, but test coverage remains low or zero for critical utility and domain packages (`admin`, `audit`, `user`, `settings`, `token`, `crypto`, `database`). 

Related files:
- [backend/internal/admin/handler_test.go](/home/m/projects/zerotrust/backend/internal/admin/handler_test.go)
- [backend/internal/audit/handler_test.go](/home/m/projects/zerotrust/backend/internal/audit/handler_test.go)
- [backend/internal/user/handler_test.go](/home/m/projects/zerotrust/backend/internal/user/handler_test.go)
- [backend/internal/settings/settings_test.go](/home/m/projects/zerotrust/backend/internal/settings/settings_test.go)
- [backend/pkg/token/jwt_test.go](/home/m/projects/zerotrust/backend/pkg/token/jwt_test.go)
- [backend/pkg/crypto/crypto_test.go](/home/m/projects/zerotrust/backend/pkg/crypto/crypto_test.go)
- [backend/pkg/database/database_test.go](/home/m/projects/zerotrust/backend/pkg/database/database_test.go)

Acceptance criteria:
- Write comprehensive unit and integration tests for the aforementioned packages.
- Fix the `go: no such tool "covdata"` issue if it prevents coverage metrics reporting.
- Hit an overall backend coverage of at least 80%.
- Ensure tests verify edge cases like avatar upload content sniffing and summary chart data structures.

---

## Security Audit (2026-06-05)

Findings from a manual security review of the auth-critical surface (JWT, DPoP, auth middleware, CSRF, login/refresh/rotation, password reset, RBAC, step-up MFA, rate limiting, client-IP handling, SQL construction, CORS, headers). No critical auth-bypass, injection, or token-forgery path was found; the items below are hardening fixes ranked by impact.

### 33. Eliminate login user-enumeration timing side-channel

State: CLOSED

Severity: Medium

Status: `Service.Login` returns `ErrInvalidCredentials` for an unknown or inactive email *before* performing any bcrypt comparison, while a valid+active account pays the full bcrypt cost. The measurable timing difference lets an attacker enumerate which emails have accounts.

Related files:
- [backend/internal/auth/service.go](/home/m/projects/zerotrust/backend/internal/auth/service.go)

Acceptance criteria:
- Perform a constant-cost bcrypt comparison against a fixed dummy hash on the unknown-user and inactive-user paths so all outcomes take comparable time.
- Preserve existing behavior: lockout still applies, response stays `ErrInvalidCredentials`.
- Add a test asserting that the unknown-email and wrong-password paths both invoke a password comparison (no early return before the hash compare).

Status update:
- Added a package-level `dummyPasswordHash` (valid bcrypt hash generated at init) in `auth/service.go`.
- `Login` now always runs `CheckPassword` — against the real hash for active users, against the dummy hash for missing/inactive users — so timing no longer reveals account existence.
- Failed attempts are now recorded uniformly across all failure paths.
- Tests in `service_enumeration_test.go` assert the unknown-email and inactive-user paths both invoke exactly one password comparison, plus a dummy-hash validity check.

### 34. Add last-admin and self-modification guards to role/status changes

State: CLOSED

Severity: Medium

Status: `UpdateRoles` and `SetStatus` let any caller holding `users:update` change their own roles or deactivate/demote the last remaining admin, risking admin lockout. If a custom (non-admin) role is ever granted `users:update`, the holder could self-assign the `admin` role (privilege escalation). Step-up MFA gates these routes but does not prevent the action.

Related files:
- [backend/internal/admin/handler.go](/home/m/projects/zerotrust/backend/internal/admin/handler.go)
- [backend/internal/user/service.go](/home/m/projects/zerotrust/backend/internal/user/service.go)

Acceptance criteria:
- Forbid a caller from modifying their own roles or active status (return a clear error).
- Forbid removing the `admin` role from, or deactivating, the last active admin.
- Add tests: self role-change rejected, self-deactivation rejected, last-admin demotion rejected, normal admin-on-other-user change still succeeds.

Status update:
- `admin/handler.go` `UpdateRoles`/`SetStatus` now reject self-modification (caller `UserID` == target) with 403 `self_modification_forbidden`, using `middleware.ClaimsFrom`.
- Added `user.ErrLastAdmin` and an atomic `guardLastAdmin` check in `user/repository.go` `SetRoles`/`SetActive`: the operation is rejected (and rolled back) only when the target is currently the sole active admin, avoiding false positives in admin-less setups. Mapped to HTTP 409 `last_admin`.
- The guard runs against pre-change state and after unknown-role validation, so `ErrUnknownRole` (input validation) still takes precedence over `ErrLastAdmin`.
- Tests: handler-level guards in `admin/lastadmin_test.go` (no DB); DB-gated invariant tests in `user/lastadmin_integration_test.go`. Verified against the live test Postgres (`go test -p 1 ./...` with `TEST_DATABASE_URL`).

### 35. Add DPoP proof replay protection (jti tracking)

State: CLOSED

Severity: Low-Medium

Status: `ValidateDPoPProof` validates signature, `typ`, `iat` (±120s skew), `htm`, and `htu`, but the `jti` is never recorded. A captured proof can be replayed within the 120-second skew window (RFC 9449 §11.1 recommends a server-side jti cache).

Related files:
- [backend/internal/auth/dpop.go](/home/m/projects/zerotrust/backend/internal/auth/dpop.go)
- [backend/pkg/middleware/auth.go](/home/m/projects/zerotrust/backend/pkg/middleware/auth.go)

Acceptance criteria:
- Track each accepted DPoP `jti` in Redis with a TTL covering the skew window; reject a second use of the same `jti`.
- Fail closed only on replay; first use must still succeed.
- Add tests: first proof accepted, identical replay rejected, distinct `jti` accepted.

Status update:
- Added `ValidateDPoPProofWithJTI` (returns jkt + jti); `ValidateDPoPProof` is now a thin wrapper, so existing callers/tests are unchanged.
- Added `Service.ConsumeDPoPProof` using Redis `SETNX` with a 5-minute TTL (`dpopReplayWindow`, comfortably exceeding the ±120s skew). Returns `ErrDPoPReplay` on reuse; nil Redis/empty jti is a no-op; Redis errors fail open (consistent with `IsRevoked`).
- Wired into both DPoP sites: `middleware/auth.go` (DPoP-bound access tokens) and the `/auth/token` handler.
- Tests in `dpop_replay_test.go` and `token_dpop_handler_test.go`: first use accepted, replay rejected, distinct jti accepted, handler returns 400 on replay.

### 36. Bind DPoP htu to scheme+host, not just path

State: CLOSED

Severity: Low

Status: `validateHTU` compares only the request path (or a suffix match) and ignores scheme/host, so a proof minted for `https://evil.example/api/v1/auth/token` validates against the same path on the real host. Impact is limited by `jkt` binding, but the check should match the full origin + path per RFC 9449.

Related files:
- [backend/internal/auth/dpop.go](/home/m/projects/zerotrust/backend/internal/auth/dpop.go)

Acceptance criteria:
- Validate `htu` against the expected scheme+host+path (configurable public base URL), not a suffix.
- Add tests: correct origin accepted, mismatched host rejected, suffix-spoof rejected.

Status update:
- Rewrote `validateHTU` to require an exact path match (removed the loose `HasSuffix` fallback) via a new `splitHTU` URL parser.
- Added opt-in host binding: `SetExpectedDPoPOrigin` / `DPOP_EXPECTED_ORIGIN` env (wired in `main.go`). When set, the htu's scheme+host must match exactly and bare-path htu is rejected; when empty (dev default) path-only validation is preserved.
- Tests in `dpop_htu_test.go`: suffix-spoof rejected, matching origin accepted, wrong host rejected, bare-path rejected under binding, path-only accepted with binding off.

### 37. Replace hand-built JSON in /me with json encoder

State: CLOSED

Severity: Low

Status: The `/me` handler builds its JSON response via `fmt.Fprintf` with `%q` over user-controlled `first_name`/`last_name`. Go quoting is not guaranteed to equal JSON encoding in all edge cases; values are escaped today but the pattern is fragile.

Related files:
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)

Acceptance criteria:
- Marshal the `/me` response with `encoding/json` (struct + `json.NewEncoder`).
- Add a test that a name containing quotes/backslashes/control characters round-trips as valid JSON.

Status update:
- Extracted a typed `meResponse` struct and `buildMeResponse` helper in `main.go`; the `/me` handler now uses `json.NewEncoder`, so all user-controlled fields are correctly escaped and nil slices encode as `[]`.
- Tests in `meresponse_test.go`: special-character round-trip stays valid JSON; nil roles/permissions encode as `[]`.

### 38. Add per-user brute-force limit on step-up MFA codes

State: CLOSED

Severity: Low

Status: `RequireRecentMFA` accepts `X-MFA-Code` guesses bounded only by the 100/min protected-route rate limit. TOTP rotation makes practical brute force infeasible, but a dedicated per-user attempt counter is cheap defense-in-depth.

Related files:
- [backend/pkg/middleware/stepup_mfa.go](/home/m/projects/zerotrust/backend/pkg/middleware/stepup_mfa.go)

Acceptance criteria:
- Track failed step-up attempts per user in Redis; temporarily reject further attempts after a threshold.
- Reset the counter on a successful step-up.
- Add tests: failures increment, threshold blocks, success resets.

Status update:
- `RequireRecentMFA` now tracks failed `X-MFA-Code` attempts per user in Redis (`mfa:stepup:fails:<uid>`), blocking with HTTP 429 `too_many_attempts` after 5 failures within a 10-minute window; a successful step-up clears the counter. Redis errors fail open.
- Tests in `stepup_mfa_test.go`: brute-force limit triggers 429 after the threshold; a valid code resets the counter.

### 39. Reject wildcard CORS origin when credentials are enabled

State: CLOSED

Severity: Info

Status: CORS is configured with `AllowCredentials: true` and operator-supplied origins. The default (`http://localhost:3000`) is safe, but nothing prevents an operator from setting `CORS_ALLOWED_ORIGINS=*`, which combined with credentials is unsafe.

Related files:
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)

Acceptance criteria:
- At startup, fail fast (or drop credentials) if a configured origin is `*`.
- Add a test asserting the wildcard-plus-credentials configuration is rejected.

Status update:
- Added `parseCORSOrigins` in `main.go`, called during config load: trims origins, rejects `*` (since `AllowCredentials` is on), and requires at least one origin — `loadConfig` now fails fast on a wildcard.
- Tests in `cors_test.go`: wildcard alone and alongside others rejected, empty rejected, explicit origins trimmed and preserved.

---

## Features

### 40. WebAuthn / passkeys as a phishing-resistant second factor

State: CLOSED

Status: The platform offered only TOTP (+ recovery codes) as a second factor. Added FIDO2/WebAuthn passkeys (security keys, platform authenticators) alongside TOTP, reusing the existing MFA/step-up architecture and pending-token login flow.

Related files:
- [backend/migrations/000018_webauthn_credentials.up.sql](/home/m/projects/zerotrust/backend/migrations/000018_webauthn_credentials.up.sql)
- [backend/internal/webauthn/](/home/m/projects/zerotrust/backend/internal/webauthn/) (repository, service, handler)
- [backend/internal/auth/service.go](/home/m/projects/zerotrust/backend/internal/auth/service.go) · [backend/internal/auth/handler.go](/home/m/projects/zerotrust/backend/internal/auth/handler.go)
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)
- [frontend/src/lib/webauthn.ts](/home/m/projects/zerotrust/frontend/src/lib/webauthn.ts) · [frontend/src/pages/dashboard/PasskeysSection.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/PasskeysSection.tsx) · [frontend/src/pages/auth/LoginPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/auth/LoginPage.tsx)

Acceptance criteria:
- Register, list, and remove passkeys for an authenticated user.
- Use a passkey as a second factor at login (alongside or instead of TOTP).
- Phishing-resistant ceremony with origin/RP-ID binding, signature-counter persistence, and single-use challenge sessions.
- Tests at every layer, including a real end-to-end register+login ceremony.

Status update:
- New `webauthn` package wrapping `github.com/go-webauthn/webauthn`: `Repository` (credential CRUD, JSONB blob + metadata), `Service` (begin/finish registration & login, Redis-backed single-use ceremony sessions, credential-exclusion on register, sign-count update on login), and `Handler` (register begin/finish, list, delete).
- DB migration `000018` adds `user_webauthn_credentials` (credential_id unique, full credential JSONB, sign_count, name, timestamps).
- Auth integration: `auth.Service` gained a `WebAuthnVerifier` (`ConfigureWebAuthn`); `Login` now flags `TOTPEnabled`/`WebAuthnEnabled` and requires the second factor when a passkey exists; new `/auth/webauthn/login/begin|finish` endpoints peek/consume the pending token and issue tokens via `completeLogin`. Global-MFA bootstrap no longer forces TOTP setup when a passkey is already registered.
- Config: `WEBAUTHN_RP_ID` (default `localhost`) and `WEBAUTHN_RP_DISPLAY_NAME`; RP origins reuse `CORS_ALLOWED_ORIGINS`.
- Frontend: `webauthn.ts` (base64url ↔ ArrayBuffer + ceremony serialization), api methods, a `PasskeysSection` for the MFA page, and a "Use a passkey" path in the login MFA stage.
- Tests: backend service end-to-end ceremony via `descope/virtualwebauthn` (real crypto register→login), handler/auth-integration with fakes, DB-gated repository lifecycle; frontend webauthn helpers (8), api methods (6), and a `PasskeysSection` render smoke test. Verified against the live test Postgres (`go test -p 1 ./...`).

---

## Repository Review (2026-06-06)

### 41. Make OpenBao/Vault field encryption deployment-safe

State: CLOSED

Severity: High

Status: The backend image previously started an embedded OpenBao development
server with a root token and an in-memory transit key. Restarting the container
could replace the key and make existing encrypted user and audit data
undecryptable. Transit ciphertext could also exceed the original user column
lengths.

Related files:
- [backend/Dockerfile](/home/m/projects/zerotrust/backend/Dockerfile)
- [infra/docker-compose.yml](/home/m/projects/zerotrust/infra/docker-compose.yml)
- [backend/pkg/secrets/secrets.go](/home/m/projects/zerotrust/backend/pkg/secrets/secrets.go)
- [backend/migrations/000019_expand_encrypted_user_fields.up.sql](/home/m/projects/zerotrust/backend/migrations/000019_expand_encrypted_user_fields.up.sql)
- [README.md](/home/m/projects/zerotrust/README.md)

Status update:
- Removed the embedded development OpenBao server and hard-coded root token from the backend container.
- The backend now runs as a non-root user and connects only to an explicitly configured persistent OpenBao/Vault transit server.
- Startup requires a token when a secrets server address is configured.
- Startup verifies encrypt and decrypt access to `db-encryption-key`.
- Added guards for empty transit responses and compatibility with legacy plaintext values.
- Expanded encrypted user fields to `TEXT` and added coverage for ciphertext larger than the old limits.
- Documented persistence, backup, key ownership, and environment requirements.

### 42. Backfill legacy email hashes and protect encrypted audit reads

State: CLOSED

Severity: High

Status: Existing plaintext users could have a null `email_hash`, preventing
hash-based lookup after field encryption was enabled. Audit listing also did
not decrypt joined user emails and could silently return encrypted or malformed
metadata.

Related files:
- [backend/migrations/000020_backfill_email_hash.up.sql](/home/m/projects/zerotrust/backend/migrations/000020_backfill_email_hash.up.sql)
- [backend/migrations/000021_enforce_email_hash.up.sql](/home/m/projects/zerotrust/backend/migrations/000021_enforce_email_hash.up.sql)
- [backend/internal/user/repository.go](/home/m/projects/zerotrust/backend/internal/user/repository.go)
- [backend/internal/audit/repository.go](/home/m/projects/zerotrust/backend/internal/audit/repository.go)

Status update:
- Added a migration that backfills deterministic hashes for legacy plaintext email values.
- Added a follow-up migration that preserves databases already at version 20 and enforces `email_hash NOT NULL`.
- Audit listing now decrypts user email and encrypted metadata when transit encryption is enabled.
- Decryption and malformed decrypted JSON failures are returned instead of exposing ciphertext or silently hiding corruption.
- Added repository and integration tests for the compatibility paths.

### 43. Restore frontend linting and update the vulnerable router dependency

State: CLOSED

Severity: Medium

Related files:
- [.github/workflows/ci.yml](/home/m/projects/zerotrust/.github/workflows/ci.yml)
- [frontend/package.json](/home/m/projects/zerotrust/frontend/package.json)
- [frontend/eslint.config.js](/home/m/projects/zerotrust/frontend/eslint.config.js)

Status update:
- Added an ESLint flat configuration and fixed the existing lint findings.
- Added frontend linting to CI.
- Updated `react-router-dom` to the latest release, `7.17.0`.
- Frontend lint, type checking, tests, production build, and dependency audit pass.

### 44. Reduce the main frontend production bundle

State: CLOSED

Severity: Low

Status: The production build previously reported a main JavaScript chunk of
approximately 637 kB after minification.

Related files:
- [frontend/src/App.tsx](/home/m/projects/zerotrust/frontend/src/App.tsx)
- [frontend/vite.config.ts](/home/m/projects/zerotrust/frontend/vite.config.ts)

Acceptance criteria:
- Measure the MUI, data-grid, date utility, and application chunks.
- Consider stable vendor chunks or additional shell-level lazy loading.
- Keep authentication bootstrap and error behavior unchanged.
- Confirm lint, type checking, tests, and production build still pass.

Status update:
- Lazy-loaded the dashboard shell and all authentication pages in addition to the existing dashboard routes.
- Added a top-level Suspense boundary while retaining the nested dashboard loading boundary.
- Reduced the shared main chunk from approximately 637 kB to 451 kB and the application entry chunk to approximately 20 kB.
- The largest remaining chunk is the data-grid date utility bundle at approximately 494 kB, below Vite's warning threshold.
- Added coverage proving all 11 lazy route modules resolve.
- Frontend lint, type checking, all 265 tests, and the production build pass without chunk-size or circular-chunk warnings.

### 45. Prevent integration tests from deleting RBAC configuration

State: CLOSED

Severity: High

Status: Several database integration-test fixtures deleted or truncated the
global `roles` table. Because `role_permissions` references roles with cascading
deletes, running those tests against a persistent development database removed
every admin permission. The UI still showed the `admin` role, but Users, Audit,
and Service Accounts APIs returned HTTP 403.

Related files:
- [backend/migrations/000022_repair_admin_permissions.up.sql](/home/m/projects/zerotrust/backend/migrations/000022_repair_admin_permissions.up.sql)
- [backend/internal/user/user_test.go](/home/m/projects/zerotrust/backend/internal/user/user_test.go)
- [backend/internal/admin/handler_test.go](/home/m/projects/zerotrust/backend/internal/admin/handler_test.go)
- [backend/internal/audit/handler_test.go](/home/m/projects/zerotrust/backend/internal/audit/handler_test.go)

Status update:
- Added migration 22 to restore canonical `admin` and `user` roles and grant every defined permission to `admin`.
- Removed destructive role-table cleanup from all affected integration fixtures.
- Test-only roles are now inserted idempotently without replacing application RBAC configuration.
- Applied migration 22 to the development database; the admin role has all 10 defined permissions.
- Ran the complete backend suite against a temporary database and confirmed all tests pass while all admin permission mappings remain intact.

### 46. Add an administrator security dashboard

State: CLOSED

Severity: Medium

Related files:
- [backend/internal/audit/security_dashboard.go](/home/m/projects/zerotrust/backend/internal/audit/security_dashboard.go)
- [backend/internal/audit/handler.go](/home/m/projects/zerotrust/backend/internal/audit/handler.go)
- [frontend/src/pages/dashboard/SecurityDashboardPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/SecurityDashboardPage.tsx)
- [frontend/src/lib/api.ts](/home/m/projects/zerotrust/frontend/src/lib/api.ts)

Status update:
- Added a protected security dashboard endpoint with `24h`, `7d`, and `30d` ranges.
- Aggregated successful and failed logins, account lockouts, login anomalies, and active sessions.
- Added zero-filled authentication activity buckets plus anomaly, country, and failed-login IP rankings.
- Preserved categorical audit fields needed for aggregation while keeping sensitive audit details encrypted.
- Added an admin-only lazy-loaded dashboard page with responsive charts and English and Turkish translations.
- Added backend repository and handler coverage plus frontend route, API, navigation, and access-control tests.

### 47. Isolate integration tests from development databases

State: OPEN

Severity: High

Status: Multiple backend integration fixtures run destructive `DELETE` or
`TRUNCATE ... CASCADE` statements against the database supplied through
`TEST_DATABASE_URL`. A mistaken development-database URL can remove users,
sessions, service accounts, audit history, and WebAuthn credentials.

Acceptance criteria:
- Refuse to run destructive fixtures unless the target database is explicitly marked as a test database.
- Give each integration run an isolated database or schema.
- Remove assumptions that a shared persistent database can be globally truncated.
- Verify that running the complete backend suite cannot mutate the development database.
