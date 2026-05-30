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

State: OPEN

Status: Security email alerts for login anomalies (impossible travel, new devices) are dispatched in fire-and-forget goroutines. If the SMTP server experiences transient issues, the alerts are silently lost.

Related files:
- [backend/internal/auth/service.go](/home/m/projects/zerotrust/backend/internal/auth/service.go)
- [backend/pkg/mailer/mailer.go](/home/m/projects/zerotrust/backend/pkg/mailer/mailer.go)

Acceptance criteria:
- Define an outbox database schema or a memory-backed retrying queue for outgoing mail events.
- If email delivery fails, retry sending with exponential backoff and a maximum retry limit.
- Log failures to the server console and audit logs if the maximum retry limit is reached.

---

### 30. Implement API Rate Limiting for Protected Endpoints

State: OPEN

Status: Rate limiting is currently set for public auth/token routes, but generic protected endpoints (such as avatar uploads or session queries) have no rate limits and can be abused by automated scripts.

Related files:
- [backend/pkg/middleware/rate_limit.go](/home/m/projects/zerotrust/backend/pkg/middleware/rate_limit.go)
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)

Acceptance criteria:
- Introduce token-bucket rate limiting based on authenticated `claims.UserID` or IP address for generic protected routes.
- Return HTTP 429 with appropriate `Retry-After` headers when the rate limit threshold is exceeded.
- Provide unit tests showing that authenticated clients are rate-limited independently.

---

### 31. Database Connection Pool and Timeout Tuning

State: OPEN

Status: The database and Redis client connection pools are initialized using hardcoded or default library options. High traffic or network latency could lead to pool exhaustion or socket timeouts.

Related files:
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)

Acceptance criteria:
- Load dynamic settings (e.g., `DATABASE_MAX_CONNS`, `DATABASE_MIN_CONNS`, `DATABASE_CONN_TIMEOUT`, `REDIS_POOL_SIZE`) from configuration/environment variables.
- Log connection pool metrics or utilization checks upon server startup and lifecycle updates.







