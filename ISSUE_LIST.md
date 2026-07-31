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
- Converted `SettingsPage.ects/zerotrust/frontend/src/lib/api.ts)
- [frontend/src/pages/dashboard/Settingtsx` into a tabbed user settings panel accessible to all authenticated users.
- Embedded the Profile card (name changes, avatar uploads/deletions) directly inside the **Profile Settings** tab.
- Rendered the reusable `<SessionsPage />` component inside the **Security & Sessions** tab, enabling self-revocation.
- Kept the **System Settings** tab (concurrent session configurations) admin-only.ects/zerotrust/frontend/src/lib/api.ts)
- [frontend/src/pages/dashboard/Setting
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
- Render visual analytics charts (e.g., success vs. failure ratios over time) to easily spot bruteects/zerotrust/frontend/src/lib/api.ts)
- [frontend/src/pages/dashboard/Setting-force attacks or access anomalies.

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

State: CLOSED

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

Status update:
- Added a centralized `internal/testdb` guard that validates PostgreSQL URL and keyword DSN formats through pgx.
- Integration tests now accept only explicit test database names such as `zerotrust_test`, `test_zerotrust`, or `zerotrust_test_ci`.
- Rejected names include `zerotrust_db`, `postgres`, and production-style database names.
- Applied the guard before every backend test migration and destructive fixture cleanup.
- Removed the session integration test fallback from `TEST_DATABASE_URL` to `DATABASE_URL`.
- CI continues to use its ephemeral `zerotrust_test` PostgreSQL service, while unsafe local URLs fail before connecting.

### 48. Add end-to-end test suite with Playwright

State: CLOSED

Severity: Medium

Status: The project had comprehensive unit and integration tests but no browser-level E2E coverage.

Related files:
- [frontend/playwright.config.ts](/home/m/projects/zerotrust/frontend/playwright.config.ts)
- [frontend/e2e/login-page.spec.ts](/home/m/projects/zerotrust/frontend/e2e/login-page.spec.ts)
- [frontend/e2e/protected-routes.spec.ts](/home/m/projects/zerotrust/frontend/e2e/protected-routes.spec.ts)
- [frontend/e2e/auth-flow.spec.ts](/home/m/projects/zerotrust/frontend/e2e/auth-flow.spec.ts)
- [frontend/e2e/setup/user.setup.ts](/home/m/projects/zerotrust/frontend/e2e/setup/user.setup.ts)
- [frontend/e2e/setup/admin.setup.ts](/home/m/projects/zerotrust/frontend/e2e/setup/admin.setup.ts)

Acceptance criteria:
- Use system Chrome (`channel: "chrome"`) to avoid Playwright browser download and Ubuntu system-library dependency issues.
- Cover login page structure without a backend (pure UI tests).
- Cover unauthenticated redirect to login for all protected routes.
- Cover full auth flow: wrong credentials toast, user login/logout, sessions page, admin-only navigation.
- Keep login attempts below the backend rate limit by using `storageState` setup projects — one login per role, not one per test.
- Force English locale in post-login tests by intercepting `/api/v1/me` responses (server-side locale overrides localStorage).
- Vitest must not pick up E2E files; exclude `e2e/**` in `vitest.config.ts`.

Status update:
- Added `@playwright/test` to devDependencies with `test:e2e` and `test:e2e:ui` scripts.
- Configured Playwright to use system Chrome via `channel: "chrome"` — no browser download required on Ubuntu.
- Pre-seeded `locale: en` in `storageState` to ensure English UI before login; post-login locale override via `/api/v1/me` route intercept.
- Implemented three setup projects (`setup:user`, `setup:admin`, `unauthenticated`) so authenticated tests reuse saved cookies rather than logging in per test.
- Added 20 E2E tests covering: login page UI (3), protected route redirects (6), wrong credentials (1), regular user flow (4), admin flow (4), and setup (2).
- All 20 tests pass in 25 seconds; 375 unit tests continue to pass.
- `e2e/.auth/` added to `.gitignore` to prevent committing session cookies.

### 49. Add password change email notification and per-user notification preferences

State: CLOSED

Severity: Medium

Status: The backend already sends security alert emails for account lockouts and login anomalies, but password change events produced no notification. Users also had no way to configure which security emails they receive.

Related files:
- [backend/internal/auth/handler.go](/home/m/projects/zerotrust/backend/internal/auth/handler.go)
- [backend/pkg/mailer/mailer.go](/home/m/projects/zerotrust/backend/pkg/mailer/mailer.go)
- [backend/migrations/](/home/m/projects/zerotrust/backend/migrations/)
- [frontend/src/pages/dashboard/SettingsPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/SettingsPage.tsx)

Acceptance criteria:
- Send a security alert email when a user successfully changes their password.
- Add a `notify_security_emails` boolean preference to the users table (default `true`).
- Respect the preference in all `SendSecurityAlert` call sites — do not send if user has opted out.
- Expose the preference in the Settings page with a toggle and persist it via a new `PATCH /api/v1/me/notifications` endpoint.
- Add backend tests for the new endpoint and the opt-out suppression logic.
- Add frontend translations for the new toggle in both EN and TR locales.

State: CLOSED

---

### 50. Track locale changes in audit log and send security alert

Severity: Medium

Status: In a Zero Trust model, a user's interface language is a stable behavioural baseline. An unexpected locale switch — especially to a foreign language — can indicate account compromise. Previously, locale changes were silently persisted with no audit trail.

Related files:
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)

Acceptance criteria:
- When `PATCH /me/locale` changes the locale to a different value, write a `user.locale_changed` entry to the audit log with `from` and `to` fields.
- Send a `locale_changed` security alert email to the user (respecting `notify_security_emails` preference).
- No audit entry or email is written when the submitted locale is the same as the current one.

State: CLOSED

### 51. Security alert banner system

Severity: Medium

Status: Toast notifications are inadequate for security-relevant events — they disappear silently and can be missed. Security signals require persistent, dismissable banners that stay visible until the user acknowledges them.

Related files:
- [frontend/src/components/DashboardLayout.tsx](/home/m/projects/zerotrust/frontend/src/components/DashboardLayout.tsx)
- [frontend/src/lib/useAuth.ts](/home/m/projects/zerotrust/frontend/src/lib/useAuth.ts)
- [frontend/src/pages/dashboard/SessionsPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/SessionsPage.tsx)
- [frontend/src/components/TokenRefreshProvider.tsx](/home/m/projects/zerotrust/frontend/src/components/TokenRefreshProvider.tsx)

Acceptance criteria:
- New sign-in from a new/unrecognised device → red `Alert` banner at top of dashboard with "View Sessions" button.
- Session terminated from another device → yellow `Alert` banner with "View Sessions" button.
- Locale changed from another session (localStorage mismatch with backend value) → yellow `Alert` banner.
- Login anomaly detected in audit log within last 24 hours → red `Alert` banner with "View Sessions" button.
- All banners are persistent (not auto-dismissed) and can be manually closed.
- Session events communicated via custom window events (`session:new_device`, `session:ended`) to decouple child components from layout.

State: CLOSED

---

### 55. Admin Users table — show MFA status and passkey count per user

Severity: Medium

Status: Admin Users table showed session counts but no signal about per-user authentication security. An admin reviewing users could not tell at a glance who had MFA enabled or passkeys registered.

Related files:
- [backend/internal/admin/handler.go](/home/m/projects/zerotrust/backend/internal/admin/handler.go)
- [backend/internal/mfa/repository.go](/home/m/projects/zerotrust/backend/internal/mfa/repository.go)
- [backend/internal/webauthn/repository.go](/home/m/projects/zerotrust/backend/internal/webauthn/repository.go)
- [frontend/src/pages/dashboard/UsersPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/UsersPage.tsx)
- [frontend/src/lib/api.ts](/home/m/projects/zerotrust/frontend/src/lib/api.ts)

Acceptance criteria:
- `GET /api/v1/admin/users` response includes `mfa_enabled` (bool) and `passkey_count` (int) per user.
- MFA and passkey data fetched in 2 batch SQL queries (`ANY($1)`) — no N+1.
- Users table shows a "Security" column with TOTP chip (green/grey) and passkey chip (blue with count) when applicable.

State: CLOSED

---

### 56. Admin home page — platform security posture summary

Severity: Medium

Status: Admin users had no at-a-glance view of the platform's authentication health. The home page only showed personal account state with no platform-level signal.

Related files:
- [backend/internal/user/repository.go](/home/m/projects/zerotrust/backend/internal/user/repository.go)
- [backend/internal/admin/handler.go](/home/m/projects/zerotrust/backend/internal/admin/handler.go)
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)
- [frontend/src/pages/dashboard/HomePage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/HomePage.tsx)
- [frontend/src/lib/api.ts](/home/m/projects/zerotrust/frontend/src/lib/api.ts)

Acceptance criteria:
- New `GET /api/v1/admin/security-posture` endpoint (admin-only) returns `total_users`, `users_without_mfa`, `users_inactive_30d` in a single SQL query.
- HomePage shows a "Platform Security Posture" card for admin users only.
- Card displays 3 stat tiles: active users (neutral), users without MFA (warning if > 0), users inactive 30+ days (warning if > 0).
- Card links to the Users admin page.

State: CLOSED

---

### 57. Security email notification preference in Settings

Severity: Low

Status: Users had no way to opt out of security alert emails. The `notify_security_emails` field existed on the user model but was not exposed in the UI.

Related files:
- [backend/migrations/000024_user_notify_security_emails.up.sql](/home/m/projects/zerotrust/backend/migrations/000024_user_notify_security_emails.up.sql)
- [backend/internal/user/repository.go](/home/m/projects/zerotrust/backend/internal/user/repository.go)
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)
- [frontend/src/pages/dashboard/SettingsPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/SettingsPage.tsx)
- [frontend/src/lib/api.ts](/home/m/projects/zerotrust/frontend/src/lib/api.ts)

Acceptance criteria:
- `PATCH /api/v1/me/notifications` endpoint accepts `{ notify_security_emails: bool }` and persists it.
- Profile tab in Settings shows an "Email Notifications" section with a toggle.
- Toggle state is initialised from the `/me` response on page load.
- Saving shows a success toast; failure reverts the toggle and shows an error toast.
- Password change alerts are always sent regardless of this preference.

State: CLOSED

---

### 58. Password strength indicator

Severity: Low

Status: Password fields gave no feedback on strength, leaving users to guess whether their chosen password met the security bar.

Related files:
- [frontend/src/components/PasswordStrengthBar.tsx](/home/m/projects/zerotrust/frontend/src/components/PasswordStrengthBar.tsx)
- [frontend/src/pages/auth/ResetPasswordPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/auth/ResetPasswordPage.tsx)
- [frontend/src/pages/dashboard/SettingsPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/SettingsPage.tsx)
- [frontend/src/pages/dashboard/UsersPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/UsersPage.tsx)

Acceptance criteria:
- Reusable `PasswordStrengthBar` component renders 4 coloured segments + a text label below any password field.
- Score 1–4 based on: length ≥ 8, length ≥ 14, uppercase present, digit present, special char present.
- Colours: red (1) → orange (2) → yellow (3) → green (4).
- Labels: Weak / Fair / Strong / Very Strong (TR: Zayıf / Orta / Güçlü / Çok güçlü).
- Hidden when password field is empty.
- Wired into: password reset flow, change-password form in Settings, admin create-user dialog.
- No external dependency — pure regex logic.

State: CLOSED

---

### 59. User detail drawer on admin Users page

Severity: Low

Status: Admin could see per-user MFA/passkey status in the table but had no way to inspect a user's sessions, audit trail, or MFA state without leaving the page.

Related files:
- [frontend/src/pages/dashboard/UsersPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/UsersPage.tsx)

Acceptance criteria:
- Clicking a user row opens a side drawer with tabbed sections: Profile, Sessions, Audit, MFA.
- Profile tab shows avatar, account info, deactivate/reactivate button, and bulk session revoke.
- Sessions tab lists active sessions with per-session revoke.
- Audit tab shows the user's recent audit events.
- MFA tab shows TOTP and passkey status.
- Each tab loads data lazily on first open.

State: CLOSED

Status update:
- `UserProfileDrawer` was already fully implemented in `UsersPage.tsx` at the time of review (4 tabs, lazy-loaded per section, row-click wired).
- No changes required; issue documented and closed.

---

### 60. Audit log export (CSV / JSON)

Severity: Low

Status: Admins could view and filter audit events in the UI but had no way to extract the data for offline analysis, compliance reporting, or archiving.

Related files:
- [backend/internal/audit/handler.go](/home/m/projects/zerotrust/backend/internal/audit/handler.go)
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)
- [frontend/src/lib/api.ts](/home/m/projects/zerotrust/frontend/src/lib/api.ts)
- [frontend/src/pages/dashboard/AuditPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/AuditPage.tsx)

Acceptance criteria:
- `GET /api/v1/admin/audit/export?format=csv|json` streams up to 10 000 entries with `Content-Disposition: attachment`.
- CSV includes UTF-8 BOM for Excel compatibility; columns: time, action, resource, user_email, user_id, ip_address.
- JSON returns a flat array of audit entry objects.
- AuditPage shows an admin-only "Export" button with a CSV / JSON dropdown.
- Supports the same action / user_id / outcome filter params as the list endpoint.

State: CLOSED

Status update:
- Added `Export` handler to `audit/handler.go` reusing `List` with limit 10 000 and offset 0.
- Registered `GET /api/v1/admin/audit/export` route in `main.go` behind `audit:read` permission.
- Added `api.admin.auditExport(params)` in `api.ts` returning a raw `Response` for blob download.
- Added Export button with CSV/JSON `Menu` dropdown to `AuditPage.tsx`; triggers browser file download.
- Added `export`, `exportCsv`, `exportJson` locale keys to EN and TR audit namespaces.

---

### 61. System health monitoring

Severity: Low

Status: The public `/health` endpoint returned a static `{"status":"ok"}` with no real checks, and there was no way for admins to inspect connection pool utilisation from the UI.

Related files:
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)
- [frontend/src/lib/api.ts](/home/m/projects/zerotrust/frontend/src/lib/api.ts)
- [frontend/src/pages/dashboard/HomePage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/HomePage.tsx)

Acceptance criteria:
- Public `GET /health` pings DB and Redis with a 3-second timeout; returns `{"status":"degraded"}` + HTTP 503 if either fails.
- Admin `GET /api/v1/admin/health` returns per-service status and pool stats (total / idle / max connections).
- Admin home page shows a compact "System Health" card alongside the posture card with a coloured dot (green/red) per service and live pool counts.

State: CLOSED

Status update:
- Enhanced `/health` handler in `main.go` to ping DB (`pgxpool.Pool.Ping`) and Redis (`rdb.Ping`) within a 3 s context; returns `status: "ok"` (200) or `"degraded"` (503).
- Added `GET /api/v1/admin/health` route (admin role) returning per-service status + pool stats from `db.Stat()` and `rdb.PoolStats()`.
- Added `AdminHealthData` interface and `api.admin.health()` method to `api.ts`.
- Added "System Health" card to `HomePage.tsx` alongside the posture card in a responsive 2-column admin row; shows PostgreSQL and Redis with status dot and pool counters.
- Added `healthTitle`, `healthDb`, `healthRedis`, `healthPool` locale keys to EN and TR homepage namespaces.

---

### 62. Backend SQL filters for the Users DataGrid

Severity: Low

Status: The Users page DataGrid had no column filters wired to the backend; all filtering was client-side and discarded on pagination.

Related files:
- [frontend/src/pages/dashboard/UsersPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/UsersPage.tsx)
- [frontend/src/pages/dashboard/UsersPage.test.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/UsersPage.test.tsx)

Acceptance criteria:
- Email column: exact-match filter (backed by SHA-256 hash lookup, no ILIKE).
- Role column: single-select filter using MUI DataGrid's singleSelect type.
- Filter state is sent to the backend on each query; no client-side re-filter.

State: CLOSED

Status update:
- Changed DataGrid column `field: "roles"` → `field: "role"` so the filter key matches `q.Get("role")` directly.
- Email column: `filterOperators: getGridStringOperators().filter(op => op.value === "equals")` — reflects that backend only supports exact hash match.
- Role column: `type: "singleSelect"`, `valueOptions: AVAILABLE_ROLES` — provides dropdown UI for known roles.
- ResourceTablePage already passes `filterModel.items` as URL params through its existing `filters` memo.

---

### 63. User own login history in Settings

Severity: Low

Status: Users had no way to review their own recent login activity from the Settings page.

Related files:
- [frontend/src/pages/dashboard/SettingsPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/SettingsPage.tsx)
- [frontend/src/pages/dashboard/SettingsPage.test.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/SettingsPage.test.tsx)

Acceptance criteria:
- New "Login History" tab in Settings visible to all users.
- Shows a table with columns: Time, Event, Location, Device, Outcome.
- Paginated (25 per page), loads from `api.listMyAudit` with `SecurityEventsOnly` filter.
- Non-admin users see tab at index 2; admin users see it at index 3 (System tab is conditional).

State: CLOSED

Status update:
- Added `historyPage`, `historyEntries`, `historyLoading`, `historyTotal` state (indices 21–24).
- Added computed `activityTabIndex = isAdmin ? 3 : 2` to handle conditional System tab.
- Added `useEffect` loading `api.listMyAudit(25, historyPage * 25)` when the activity tab is active.
- Added Login History tab with table (Time/Event/Location/Device/Outcome) and prev/next pagination.
- Added 7 tests: admin index, non-admin index, loading state, empty state, entries render, pagination, no-fetch on other tabs.
- Added locale keys to EN/TR: `tabActivity`, `activityTitle`, `activityDesc`, `activityEmpty`, `activityCount`, `activityColTime`, `activityColEvent`, `activityColLocation`, `activityColDevice`, `activityColOutcome`, `activityPrev`, `activityNext`.

---

### 64. Admin bulk user operations

Severity: Low

Status: Admins had to toggle user status one at a time; no way to activate/deactivate multiple users or export a filtered set.

Related files:
- [backend/internal/user/repository.go](/home/m/projects/zerotrust/backend/internal/user/repository.go)
- [backend/internal/user/service.go](/home/m/projects/zerotrust/backend/internal/user/service.go)
- [backend/internal/admin/handler.go](/home/m/projects/zerotrust/backend/internal/admin/handler.go)
- [backend/internal/admin/handler_unit_test.go](/home/m/projects/zerotrust/backend/internal/admin/handler_unit_test.go)
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)
- [frontend/src/pages/dashboard/UsersPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/UsersPage.tsx)
- [frontend/src/pages/dashboard/UsersPage.test.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/UsersPage.test.tsx)
- [frontend/src/components/ResourceTablePage.tsx](/home/m/projects/zerotrust/frontend/src/components/ResourceTablePage.tsx)
- [frontend/src/lib/api.ts](/home/m/projects/zerotrust/frontend/src/lib/api.ts)

Acceptance criteria:
- Checkbox selection on the Users DataGrid (up to 200 rows).
- Bulk activate, bulk deactivate (with last-admin guard), bulk CSV export.
- Deactivation revokes all sessions for affected users.
- Caller is silently excluded from bulk deactivation (no self-lockout).
- Step-up MFA required for bulk status changes.

State: CLOSED

Status update:
- Added `BulkSetActive(ctx, userIDs, active)` to `user/repository.go` with last-admin guard and `UPDATE users SET is_active = $1 WHERE id = ANY($2::uuid[])` in a transaction.
- Propagated through `user/service.go` interface and `admin/handler.go` `UserManager` interface.
- Added `POST /api/v1/admin/users/bulk-status` route (users:update + step-up MFA), validates 1–200 IDs, silently excludes caller, revokes sessions on deactivation.
- Added 8-subtest `TestBulkSetStatus` to `handler_unit_test.go` (fixed `withClaims` helper using `middleware.ClaimsKey`).
- Added `checkboxSelection`, `rowSelectionModel`, `onRowSelectionModelChange` props to `ResourceTablePage`.
- UsersPage: `GridRowSelectionModel` state, conditional bulk toolbar (Activate / Deactivate / Export CSV / Clear), client-side CSV export from `loadedRowsRef`.
- Added `bulkSetUserStatus` to `api.ts`.
- Added `bulkSelected`, `bulkActivate`, `bulkDeactivate`, `bulkExport`, `bulkClear`, `bulkActivated`, `bulkDeactivated` locale keys to EN/TR admin namespaces.
- Added 8 frontend tests: toolbar visibility, activate, deactivate, error, mfa_required silent, clear, export with rows, export with no matching rows.

---

### 65. Webhook test button for admin settings

State: CLOSED

Severity: Low

Status: Admins could configure a Slack/Mattermost webhook URL but had no way to verify delivery without triggering a real security event.

Related files:
- [backend/internal/audit/webhook.go](/home/m/projects/zerotrust/backend/internal/audit/webhook.go)
- [backend/cmd/server/main.go](/home/m/projects/zerotrust/backend/cmd/server/main.go)
- [frontend/src/lib/api.ts](/home/m/projects/zerotrust/frontend/src/lib/api.ts)
- [frontend/src/pages/dashboard/SettingsPage.tsx](/home/m/projects/zerotrust/frontend/src/pages/dashboard/SettingsPage.tsx)

Acceptance criteria:
- `POST /api/v1/admin/settings/webhook/test` sends a synthetic `system.webhook_test` payload to the configured or request-provided URL.
- Returns 204 on delivery success, 422 on failure, 400 if no URL is configured.
- "Send Test" button appears inline next to the webhook URL field, only enabled when a URL is entered.
- Success and failure toast notifications in EN/TR.

Status update:
- Added exported `TestWebhook(ctx, url)` method on `audit.Repository` dispatching a synthetic test payload.
- Added `POST /api/v1/admin/settings/webhook/test` route (admin-only) in `main.go` using configured or request-provided URL.
- Added `api.admin.testWebhook(url?)` in `api.ts`.
- Added "Send Test" button inline in SettingsPage webhook row with loading state.
- Added EN/TR locale keys: `webhookTest`, `webhookTesting`, `webhookTestSuccess`, `webhookTestFailed`, `webhookTestNoUrl`.
- Added `TestTestWebhook` and `TestTestWebhook_DeliveryFailure` unit tests.

---

## OIDC Security Audit (2026-06-23)

Findings from a manual security review of the OIDC provider surface (authorization code flow, token exchange, UserInfo, revocation, introspection, EndSession, PKCE, refresh token rotation, consent step-up MFA, audit logging). No auth-bypass or token-forgery path was found. The items below were hardening fixes ranked by impact.

### 66. OIDC UserInfo endpoint did not enforce granted scopes

State: CLOSED

Severity: Medium

Status: The `/oauth2/userinfo` endpoint always returned all user fields (name, email, locale, roles) regardless of which scopes (`openid`, `profile`, `email`) were granted in the access token. A client that only requested `openid` should not receive `email` or `profile` claims.

Related files:
- [backend/internal/oidc/service.go](backend/internal/oidc/service.go)
- [backend/internal/oidc/handler.go](backend/internal/oidc/handler.go)
- [backend/internal/oidc/security_test.go](backend/internal/oidc/security_test.go)

Acceptance criteria:
- OIDC access tokens carry the granted scope list.
- UserInfo filters claims: `profile` scope gates name/given_name/family_name/locale/roles; `email` scope gates email/email_verified; `sub` is always returned.
- Internal browser-session tokens (no scope claim) continue to receive full info for backward compatibility.

Status update:
- Added `Scopes: session.Scopes` to the OIDC access token claims in `ExchangeCode`.
- Added `Scopes: scopes` to the OIDC access token claims in `ExchangeRefreshToken` so the scope survives rotation.
- Rewrote `UserInfo` to build a scope set from `claims.Scopes` and conditionally include profile and email fields.
- Added `TestUserInfo_ScopeFiltering` (4 sub-tests: openid-only, openid+email, openid+profile, all) and `TestUserInfo_ScopeInAccessToken` (scope preserved through refresh rotation) in `security_test.go`.

### 67. JSON injection in OIDC Consent server-error response

State: CLOSED

Severity: Low

Status: The `Consent` handler built its server-error response via string concatenation (`"...\"" + err.Error() + "\""`) instead of `json.Marshal`. An error message containing a double-quote or backslash would produce malformed JSON.

Related files:
- [backend/internal/oidc/handler.go](backend/internal/oidc/handler.go)
- [backend/internal/oidc/security_test.go](backend/internal/oidc/security_test.go)

Status update:
- Replaced string concatenation with `json.NewEncoder(w).Encode(map[string]string{...})` so all error fields are properly escaped.
- Added `TestConsent_ServerErrorJSON` verifying that the response is always valid JSON.

### 70. SSRF via admin webhook URL (no internal-address check)

State: CLOSED

Severity: High

Status: The `sendWebhook` function dispatched HTTP requests to any URL provided by an admin — including `http://169.254.169.254/` (AWS IMDS), `http://localhost:6379/` (Redis), internal microservices, and other private-range endpoints. The `/admin/settings/webhook/test` endpoint also accepted a caller-supplied URL, making the attack trivially reachable by any admin.

Related files:
- [backend/internal/audit/webhook.go](backend/internal/audit/webhook.go)
- [backend/internal/audit/repository.go](backend/internal/audit/repository.go)
- [backend/internal/audit/webhook_test.go](backend/internal/audit/webhook_test.go)

Acceptance criteria:
- Reject non-http/https schemes (file://, ftp://, etc.).
- Resolve the target hostname and reject any address that is loopback, private, link-local, or unspecified.
- Existing tests must continue to pass (test servers use loopback — bypass via injectable `webhookClient`).

Status update:
- Added `validateWebhookURL` in `webhook.go`: parses scheme (must be http/https), resolves hostname via `net.LookupHost`, rejects loopback/private/link-local/unspecified addresses.
- `sendWebhook` calls `validateWebhookURL` before constructing the HTTP request when no test client is injected.
- Added `webhookClient *http.Client` field and `SetWebhookClient` setter to `Repository`; when set (tests only), the SSRF guard is skipped so `httptest.Server` targets work.
- Added `TestValidateWebhookURL` with 10 sub-cases covering loopback, private ranges, link-local (169.254.x), and forbidden schemes.

### 69. Spoofable client IP in MFA, WebAuthn, and inline locale/password handlers

State: CLOSED

Severity: Medium

Status: Four code sites read `X-Forwarded-For` directly without going through the `TrustedClientIP` middleware, making the IP address used in audit log entries and security-alert emails trivially spoofable by any client. When deployed behind a reverse proxy the `TrustedClientIP` middleware already rewrites `r.RemoteAddr` to the real client IP, so any additional XFF reads are both redundant and unsafe.

Related files:
- [backend/internal/mfa/handler.go](backend/internal/mfa/handler.go)
- [backend/internal/webauthn/handler.go](backend/internal/webauthn/handler.go)
- [backend/cmd/server/main.go](backend/cmd/server/main.go)

Acceptance criteria:
- All IP extraction in request handlers uses `authmw.ClientIP(r)` (which reads the `r.RemoteAddr` already resolved by the middleware) instead of raw `X-Forwarded-For`.
- No package-local `clientIP()` function duplicates proxy-header logic.

Status update:
- Replaced the `clientIP()` helper bodies in `mfa/handler.go` and `webauthn/handler.go` with `return authmw.ClientIP(r)` and removed the now-unused `strings` import from `mfa/handler.go`.
- Replaced the two inline XFF extractions in the locale-change and password-change handlers in `main.go` with `authmw.ClientIP(r)`.
- Build and all affected package tests pass.

### 68. Missing Pragma: no-cache on OIDC token endpoint

State: CLOSED

Severity: Low

Status: RFC 6749 §5.1 requires both `Cache-Control: no-store` and `Pragma: no-cache` on token responses. The `Cache-Control` header was present but `Pragma` was absent.

Related files:
- [backend/internal/oidc/handler.go](backend/internal/oidc/handler.go)
- [backend/internal/oidc/security_test.go](backend/internal/oidc/security_test.go)

Status update:
- Added `w.Header().Set("Pragma", "no-cache")` alongside the existing `Cache-Control: no-store` in the Token handler.
- Added `TestToken_PragmaHeader` asserting both headers are present on a successful code exchange response.

### 73. Open redirect in OIDC consent denial path

State: CLOSED

Severity: High

Status: The `Consent` handler validated `redirect_uri` against the client's registered URIs only on the approval path (`req.Approved = true`). On the denial path (`!req.Approved`), the handler used `req.RedirectURI` directly from the request body — a user-controlled field — without any validation. An authenticated attacker could POST `{"approved": false, "redirect_uri": "https://evil.com"}` and receive `{"redirect_url": "https://evil.com?error=access_denied"}` which the frontend then uses to navigate, redirecting the victim to an arbitrary external site.

Related files:
- [backend/internal/oidc/handler.go](backend/internal/oidc/handler.go)
- [backend/internal/oidc/security_test.go](backend/internal/oidc/security_test.go)

Acceptance criteria:
- Client lookup and `ValidateRedirectURI` run before the `if !req.Approved` branch.
- A denial with an unregistered `redirect_uri` returns a 4xx error, not a redirect.

Status update:
- Moved `h.clientRepo.FindByClientID` and `client.ValidateRedirectURI` to execute before the `!req.Approved` branch so both approval and denial paths share the same redirect-URI guard.
- Removed the now-duplicated `FindByClientID` call from the approval path (reuses the `client` variable).
- Added `TestConsent_DenialOpenRedirect` with two sub-tests: unregistered URI rejected (non-200), registered URI returns correct `access_denied` redirect with state preserved.
- All OIDC and full-suite tests pass.

### 72. DPoP replay protection bypassable by omitting the jti claim

State: CLOSED

Severity: Medium

Status: `ValidateDPoPProofWithJTI` never checked that the `jti` claim was present. When a proof was submitted without a `jti`, `claims.Jti` was an empty string. `ConsumeDPoPProof` explicitly skips the Redis `SetNX` check when `jti == ""`, so the proof passed all validation and replay detection was silently bypassed. RFC 9449 §4.2 mandates `jti` in every DPoP proof, and this omission allowed an attacker to replay the same DPoP proof indefinitely (within the 2-minute `iat` skew window) to obtain additional access tokens.

Related files:
- [backend/internal/auth/dpop.go](backend/internal/auth/dpop.go)
- [backend/internal/auth/dpop_test.go](backend/internal/auth/dpop_test.go)

Acceptance criteria:
- A DPoP proof with an empty or absent `jti` claim is rejected before replay checking.
- Existing valid proofs (those with a non-empty `jti`) continue to pass.

Status update:
- Added `if claims.Jti == "" { return "", "", errors.New("missing jti claim in DPoP proof") }` in `ValidateDPoPProofWithJTI`, after signature and claim-type validation but before the `iat` window check.
- Added test case 17 to `TestDPoPValidationFailures`: a proof with `Jti = ""` must return an error.
- All DPoP tests pass; full build passes.

### 78. Demo OAuth2 client with documented plaintext secret in migration

State: CLOSED

Severity: Low

`migrations/000023_oauth2_clients.up.sql` seeds a `demo-client` row with a comment that reads `-- Secret is "demo-secret"`. The plaintext secret is in the migration file's commit history and is trivially known to anyone with repository access. Additionally, one of the registered redirect URIs is `https://oauth.pstmn.io/v1/callback`, a public Postman endpoint that any attacker can receive callbacks on. If this migration is applied to a production database, the demo client is a permanently open authorization path: knowing the secret and pointing to Postman's callback is sufficient to complete an OAuth2 authorization code exchange against real users (scopes: `openid`, `profile`, `email`).

Related files:
- [backend/migrations/000023_oauth2_clients.up.sql](backend/migrations/000023_oauth2_clients.up.sql)

Acceptance criteria:
- The migration comment no longer reveals the plaintext secret.
- A clear warning is present that this client must be removed before production deployment.
- A companion down-migration or post-deploy script removes the demo-client row in production.

Status update:
- Removed the `-- Secret is "demo-secret"` comment; replaced with a `DEVELOPMENT ONLY` warning that the row and its known secret must be deleted before production deployment.
- Created `backend/migrations/000027_remove_demo_client.up.sql` (DELETE the demo-client row) and `backend/migrations/000027_remove_demo_client.down.sql` (re-inserts for dev rollback only).
- Issue closed; apply migration 000027 as part of any production deployment.

### 77. MFAChallenge and RefreshTokens do not check user account status

State: CLOSED

Severity: Medium

Two paths in `auth/service.go` fetched the user with `FindByID` but did not check `u.IsActive`:

1. **`MFAChallenge`**: Called after TOTP is verified. If a user is deactivated between the initial password check (where `IsActive` is enforced) and TOTP completion (within the 5-minute pending-token window), they could still receive a token pair.

2. **`RefreshTokens`**: Called on every session refresh. A deactivated user's sessions would continue to refresh until the idle timeout or absolute session expiry naturally terminated them — up to 8 hours with default settings.

The WebAuthn finish path (line 600, same file) already had `if err != nil || !u.IsActive`, but these two paths did not mirror it.

Related files:
- [backend/internal/auth/service.go](backend/internal/auth/service.go)
- [backend/internal/auth/service_test.go](backend/internal/auth/service_test.go)
- [backend/internal/auth/service_refresh_policy_test.go](backend/internal/auth/service_refresh_policy_test.go)

Acceptance criteria:
- A user deactivated mid-login cannot complete TOTP and receive tokens.
- A deactivated user's next refresh attempt is rejected immediately.
- Active users are unaffected.

Status update:
- Added `|| !u.IsActive` to the `FindByID` guards in both `MFAChallenge` and `RefreshTokens`.
- Updated test users in `service_test.go` and `service_refresh_policy_test.go` to set `IsActive: true`.
- All 20 packages pass.

### 76. OIDC code exchange and refresh do not check user account status

State: CLOSED

Severity: Medium

`ExchangeCode` and `ExchangeRefreshToken` in `oidc/service.go` called `userSvc.FindByID` to fetch the user but never checked `u.IsActive`. An administrator who deactivates a user account (setting `is_active = false`) expects that user to immediately lose all access. The main auth service correctly enforces this on login and session refresh (`!u.IsActive → ErrInactiveUser`), but the OIDC code paths did not, allowing a deactivated user to continue exchanging authorization codes and rotating OIDC refresh tokens until all their tokens expired naturally.

Related files:
- [backend/internal/oidc/service.go](backend/internal/oidc/service.go)
- [backend/internal/oidc/service_test.go](backend/internal/oidc/service_test.go)

Acceptance criteria:
- A deactivated user (IsActive=false) cannot exchange an authorization code for tokens.
- A deactivated user cannot exchange a refresh token for a new access token.
- Active users are unaffected.

Status update:
- Added `|| !u.IsActive` to both `FindByID` error guards in `ExchangeCode` and `ExchangeRefreshToken`.
- Added `TestExchangeCode_InactiveUser` and `TestExchangeRefreshToken_InactiveUser` to `service_test.go`.
- Updated all pre-existing test users in `service_test.go` and `security_test.go` to set `IsActive: true` (zero value defaults to false, which broke existing tests).
- All 20 packages pass.

### 75. SSRF via DNS rebinding in webhook dispatch

State: CLOSED

Severity: Medium

`sendWebhook` called `validateWebhookURL` (which resolves the hostname and checks all IPs) and then passed the URL to `http.Client.Do`, which resolves the hostname a second time independently. An attacker who controls a DNS server could serve a legitimate public IP during the `validateWebhookURL` call, then immediately change the record to a private address (e.g., `10.0.0.1`, `169.254.x.x`) before the HTTP client's internal resolver runs. Because DNS caches at the OS level may expire between the two calls, the TCP connection could land on a private host — a classic DNS-rebinding SSRF bypass.

Related files:
- [backend/internal/audit/webhook.go](backend/internal/audit/webhook.go)

Acceptance criteria:
- IP validation is enforced at connection time (just before TCP dial), not only during a pre-check that may be stale.
- A hostname that resolves to a private IP at dial time is rejected even if it resolved to a public IP during the pre-check.

Status update:
- Added `ssrfSafeTransport()`: a custom `http.Transport` whose `DialContext` calls `net.DefaultResolver.LookupHost` immediately before each TCP dial and rejects the connection if any resolved IP is loopback, private, link-local, or unspecified. This closes the DNS-rebinding window.
- `validateWebhookURL` is retained as a fast pre-check on URL format + scheme; the transport provides the binding guarantee.
- Injected the safe transport into the production `http.Client` in `sendWebhook`.
- Tests still pass; injected test client continues to bypass SSRF checks as intended.

### 74. Missing per-user brute-force protection on /mfa/disable and /mfa/step-up

State: CLOSED

Severity: Medium

The `RequireRecentMFA` middleware enforces a 5-attempt / 10-minute per-user lockout on TOTP codes submitted through the `X-MFA-Code` header path. The dedicated `/mfa/disable` and `/mfa/step-up` endpoints did not apply any equivalent per-user lockout. Only the shared `protectedRL` limit (300 req/min per user across all authenticated routes) provided any guard. An attacker with a valid session could exhaust the TOTP code space against these two endpoints at a rate limited only by the shared per-user bucket, without triggering the stricter code-level lockout that protects the middleware path.

TOTP's 30-second rotation makes a full brute-force very low probability (~0.015% per window), but defense-in-depth with a hard per-user lockout prevents sustained guessing within a session.

Related files:
- [backend/internal/mfa/handler.go](backend/internal/mfa/handler.go)

Acceptance criteria:
- After 5 consecutive wrong codes on `/mfa/disable`, the user's disable flow is locked for 10 minutes (429).
- After 5 consecutive wrong codes on `/mfa/step-up`, the same counter used by `RequireRecentMFA` is incremented, locking both paths.
- A correct code clears the failure counter on both endpoints.
- Redis unavailability fails open (no lockout, operation continues).

Status update:
- Added helpers in `mfa/handler.go`: `mfaAttemptsExceeded`, `recordMFAFailure`, `clearMFAFailures` (each nil-safe for `rdb`).
- `mfaStepUpAttemptsKey` uses the same `"mfa:stepup:fails:<userID>"` key as `RequireRecentMFA` so failed attempts via either path share one counter.
- `mfaDisableAttemptsKey` uses `"mfa:disable:fails:<userID>"`.
- Both `Disable` and `StepUp` handlers check lockout before validating code, record failure on bad code, clear on success.
- All 20 test packages pass.

### 71. Invalid JSON encoding in profile and avatar update handlers

State: CLOSED

Severity: Low

Status: Three inline handlers in `main.go` serialised the updated profile response using either Go's `fmt.Fprintf` with `%q` verb or raw string concatenation, neither of which produces guaranteed-valid JSON. Go's `%q` verb emits `\a` and `\v` escapes that JSON parsers must reject per RFC 8259; string concatenation breaks if the interpolated string contains a double-quote. In practice, password-validation error codes are safe sentinel strings and user names rarely contain C0 control characters, but the pattern was fragile and inconsistent with the rest of the codebase.

Related files:
- [backend/cmd/server/main.go](backend/cmd/server/main.go)

Acceptance criteria:
- All JSON responses are encoded via `json.NewEncoder` or `json.Marshal`, never via `fmt.Fprintf %q` or string concatenation.

Status update:
- Replaced all three `fmt.Fprintf(...%q...)` profile responses (PATCH `/me/profile`, POST `/me/avatar`, DELETE `/me/avatar`) with `json.NewEncoder(w).Encode(map[string]any{...})`.
- Replaced the string-concatenated password-complexity error (`{"error":"` + err.Error() + `"}`) in PATCH `/me/password` with `json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})`.
- Removed now-unused intermediate `rolesJSON`/`permsJSON` variables from all three avatar/profile handlers.

### 79. Self-service password change did not revoke existing sessions

State: CLOSED

Severity: Medium

`PATCH /me/password` updated the password hash and sent a security alert email, but left all active sessions alive. An attacker who had obtained a valid session cookie (via XSS, network interception, or device theft) could survive a password change for up to 8 hours (the absolute session timeout). The password-reset-via-email flow already atomically revokes all sessions in the same transaction; the self-service path was inconsistent.

Related files:
- [backend/cmd/server/main.go](backend/cmd/server/main.go)

Acceptance criteria:
- After a successful password update, all sessions for that user are immediately revoked.
- The user must re-authenticate with the new password on all devices.
- Active users are unaffected (no change to the behavior when the user is not changing their password).

Status update:
- Added `_ = sessionRepo.RevokeAllForUser(r.Context(), claims.UserID)` in `main.go` immediately after `userSvc.UpdatePassword` in the `PATCH /me/password` handler.
- Updated the security alert email body to remove the "revoke sessions" self-service suggestion (since sessions are now automatically revoked).
- All 20 packages pass.

### 80. Self-service password change was not a critical audit event

State: CLOSED

Severity: Low

`PATCH /me/password` fell through the `classifyRoute` function in `audit.go` with no specific action name, so the audit middleware logged it as `"PATCH /api/v1/me/password"` with `critical: false`. A non-critical event is written asynchronously in a background goroutine, meaning the event could be lost if the process crashes immediately after the write. A password change is a security-critical operation and should be logged synchronously with a stable semantic action name.

Related files:
- [backend/pkg/middleware/audit.go](backend/pkg/middleware/audit.go)

Status update:
- Added `PATCH /api/v1/me/password` to `classifyRoute` as `user.password_changed` with `critical: true`.
- The audit write is now synchronous (bounded-timeout context, same as other critical events).
- All 20 packages pass.
- Build passes with no errors.

## P0 — Fresh audit round (2026-07)

### 81. OIDC access tokens are accepted by the first-party API (audience confusion)

State: CLOSED

Severity: Critical

`ValidateAccessToken` (`backend/internal/auth/token.go:131-159`) verifies only signature/expiry — it never checks `iss` or `aud`. OIDC access tokens are minted as `auth.Claims` with `SubType: SubTypeUser` and the user's `Roles`, signed by the same KeyStore as internal tokens (`backend/internal/oidc/service.go:133-149`, TTL 1 hour). The internal `Authenticate` middleware (`backend/pkg/middleware/auth.go:26`) therefore accepts them on every `/api/v1/*` route. An external OAuth2 client that completes a code flow for a victim can call the internal API as that user: plant an attacker-controlled passkey (`POST /webauthn/register/begin|finish` have no step-up MFA, `backend/cmd/server/main.go:819-823`), enroll TOTP when MFA is off, read `/me` + `/me/audit` + `/sessions`, delete sessions/credentials, or reach `RequireRole("admin")`-only GET routes if the victim is an admin. The OIDC token lives 1 hour, is not tied to any session, and bypasses idle-timeout enforcement.

Related files:
- [backend/internal/auth/token.go](backend/internal/auth/token.go)
- [backend/internal/oidc/service.go](backend/internal/oidc/service.go)
- [backend/pkg/middleware/auth.go](backend/pkg/middleware/auth.go)
- [backend/internal/auth/service.go](backend/internal/auth/service.go) (`EvaluateAccess`, `:1159-1211`)

Acceptance criteria:
- Internal session tokens carry a dedicated audience (e.g. `aud=<issuer>` or a `token_use: "session"` claim) and `ValidateAccessToken` rejects tokens without it.
- OIDC-issued tokens stop embedding internal `Roles`.
- `EvaluateAccess` default-denies unknown `SubType` values.
- Step-up MFA (`RequireRecentMFA`) required on WebAuthn register/delete and `/mfa/setup|verify|disable`.
- Regression test: an OIDC client access token is rejected with 401 on `/api/v1/me` and `/api/v1/webauthn/register/begin`.

Status update:
- Added a `token_use` claim to `auth.Claims` (`TokenUseSession` / `TokenUseService`). All internal minting sites funnel through `GenerateTokenPair` (login, MFA challenge completion, refresh rotation, WebAuthn logins) and `GenerateServiceToken`, which now set it; both DPoP-bound service tokens (`cnf` flow unchanged) and plain session tokens carry the marker.
- `ValidateAccessToken` rejects tokens without a recognized `token_use`. New `ValidateOIDCAccessToken` (signature/expiry only) serves the OIDC-facing endpoints (UserInfo, Introspect, Revoke) that legitimately accept external-client tokens. OIDC access tokens keep `aud=<client_id>` and carry no `token_use`, so the internal `Authenticate` middleware returns 401 for them on every `/api/v1/*` route.
- OIDC-issued access tokens no longer embed internal `Roles` (both minting sites in `oidc/service.go`).
- `EvaluateAccess` default-denies unknown `SubType` values (new `default: return ErrInvalidToken`).
- Step-up MFA: strict `RequireRecentMFA` hard-fails when MFA is disabled, which would brick first-time TOTP enrollment, so a conditional variant `RequireRecentMFAIfEnabled` (pass-through while MFA is off, identical enforcement once enabled) was added and mounted on `POST /mfa/setup|verify|disable`, `POST /webauthn/register/begin|finish`, and `DELETE /webauthn/credentials/{id}` in `main.go`.
- Regression tests: `TestAuthenticateRejectsOIDCAccessToken` (OIDC client token → 401 on `/api/v1/me` and `/api/v1/webauthn/register/begin`; session + service tokens still pass), `TestValidateAccessTokenRejectsMissingTokenUse`, `TestExchangeCode_AccessTokenCarriesNoInternalClaims`, `TestRequireRecentMFAIfEnabled`.
- All 20 packages pass (also with `TEST_DATABASE_URL` + Redis for the DB-backed suites).

### 82. OIDC refresh tokens survive every session-revocation path

State: CLOSED

Severity: High

OIDC refresh tokens live in Redis keyed only by token hash (`oauth2:refresh:<sha256>`, `backend/internal/oidc/refreshtoken.go:77-79`) with a 30-day TTL (`:16`) and no per-user index. `RevokeAllForUser` (`backend/internal/session/repository.go:197-206`) only touches the Postgres `sessions` table, so password change, admin revoke-all, and the token-reuse mass-revocation response never invalidate OIDC refresh tokens. Combined with #81, an attacker holding an `offline_access` refresh token keeps minting internal-API-valid access tokens for up to 30 days after the victim changes their password.

Related files:
- [backend/internal/oidc/refreshtoken.go](backend/internal/oidc/refreshtoken.go)
- [backend/internal/session/repository.go](backend/internal/session/repository.go)

Acceptance criteria:
- A per-user index of OIDC refresh-token hashes is maintained (or OIDC refresh sessions are stored in the `sessions` table).
- Every `RevokeAllForUser` path (password change, admin revoke, reuse detection) also invalidates the user's OIDC refresh tokens.
- Test: after password change, an existing OIDC refresh token returns `invalid_grant`.

Status update:
- `RefreshTokenStore.Save` now also indexes each token hash in a per-user Redis set `oauth2:refresh:user:<uid>` (TTL-aligned), and a new `RefreshTokenStore.RevokeAllForUser` deletes every token in that set.
- `session.Repository` gained `SetOIDCRefreshRevoker`; `RevokeAllForUser` invokes the registered revoker after the Postgres update. Since password change (`main.go`), admin revoke-all, and the token-reuse mass-revocation response (`auth.Service.RefreshTokens`) all funnel through `session.Repository.RevokeAllForUser`, one wiring line in `main.go` (`sessionRepo.SetOIDCRefreshRevoker(oidcRefreshStore)`) covers every path.
- Tests: `TestRefreshTokenStore_RevokeAllForUser` (after revoke, existing OIDC refresh tokens return `invalid_grant` via `ExchangeRefreshToken`; another user's tokens unaffected) and `TestRevokeAllForUser_InvokesOIDCRevoker` (DB-backed, verifies the repository hook fires).
- All 20 packages pass.

### 83. Service account with only `users:create` scope can mint full admin users

State: CLOSED

Severity: High

`POST /admin/users` is guarded only by `RequirePermission("users","create")` without `stepUpMFA` (`backend/cmd/server/main.go:827`), and `CreateUser` (`backend/internal/admin/handler.go:180-229`) passes `req.Roles` straight through with no delegation check — `CreateWithRoles` accepts any existing role including `admin`. A leaked service-account secret holding only `users:create` (a plausible least-privilege provisioning scope) yields full admin account creation. Asymmetry: creating a *service account* requires the caller to hold every requested scope, but creating an *admin user* requires only `users:create`. `UpdateRoles` (`handler.go:236`) has the same missing delegation check.

Related files:
- [backend/cmd/server/main.go](backend/cmd/server/main.go)
- [backend/internal/admin/handler.go](backend/internal/admin/handler.go)
- [backend/internal/user/repository.go](backend/internal/user/repository.go)

Acceptance criteria:
- `CreateUser`/`UpdateRoles` reject role assignments the caller does not hold; at minimum assigning `admin` requires the caller to be an admin (in-handler check on `claims.Roles`).
- `POST /admin/users` gets the `stepUpMFA` wrapper like every other admin mutation.
- Test: a service token with `users:create` cannot create a user with `roles:["admin"]` (403).

Status update:
- Added a `containsRole` helper and a guard in both `CreateUser` and `UpdateRoles` (`backend/internal/admin/handler.go`): when the requested roles include `admin`, the caller's own `claims.Roles` must also include `admin`, otherwise the request is rejected with 403 `role_escalation_forbidden` before any repository call.
- `POST /admin/users` now runs behind `stepUpMFA`, matching every other admin mutation (`backend/cmd/server/main.go`).
- Tests: `TestCreateUser_RoleEscalationForbidden`, `TestCreateUser_NoClaimsCannotGrantAdmin`, `TestCreateUser_NonAdminRoleAllowedWithoutAdminClaim` (`handler_unit_test.go`); `TestUpdateRoles_EscalationForbiddenWithoutAdminClaim`, `TestUpdateRoles_AdminCanGrantAdmin` (`lastadmin_test.go`). Existing tests that posted `roles:["admin"]` without caller claims were updated to inject admin claims so they still exercise their original assertions.
- All 20 packages pass.

### 84. `/auth/reset-password`: bcrypt before token validation, no endpoint rate limit

State: CLOSED

Severity: High

`Reset()` runs `bcrypt.GenerateFromPassword(cost 12)` before the token is ever looked up (`backend/internal/passwdreset/service.go:63-69`), and the route carries no endpoint rate limiter (`backend/cmd/server/main.go:423`, only the global 300 req/min per IP). An unauthenticated attacker burns ~200-300 ms CPU per request with garbage tokens; every failed attempt also inserts an audit row and fires an outbound webhook goroutine (`auth.password_reset_failed` is a high-risk event). A handful of IPs can saturate CPU and the alerting channel.

Related files:
- [backend/internal/passwdreset/service.go](backend/internal/passwdreset/service.go)
- [backend/cmd/server/main.go](backend/cmd/server/main.go)

Acceptance criteria:
- Token lookup happens first; bcrypt only runs after a valid, unused, unexpired token is found.
- The route sits behind a strict limiter (e.g. `loginRL`).
- Test: requests with invalid tokens do not perform bcrypt work (measurable latency) and are rate-limited.

Status update:
- `passwdreset.Repository.ConsumeAndReset` now looks up and validates the token (found, unused, unexpired) inside its `FOR UPDATE` transaction *before* any bcrypt work — both the password-reuse comparison and the new-password hash. `Service.Reset` no longer pre-computes a bcrypt hash; it just calls `ConsumeAndReset(ctx, rawToken, newPassword)`.
- `POST /api/v1/auth/reset-password` now sits behind `loginRL` (`backend/cmd/server/main.go`), alongside `/auth/register` (#92) and `/auth/refresh` (now behind `tokenRL`, #94) which were fixed in the same pass.
- Tests: `TestRepositoryConsumeAndReset_InvalidTokenSkipsBcrypt` (unknown token + oversized password still returns `ErrNotFound`, proving bcrypt never ran) and `TestRepositoryConsumeAndReset_ValidTokenRunsBcrypt` (valid token + oversized password surfaces a bcrypt error, proving bcrypt only runs post-validation) in `repository_integration_test.go`.
- All 20 packages pass.

### 85. OIDC refresh grant: no client authentication, no reuse detection, consume-before-verify

State: CLOSED

Severity: Medium

`ExchangeRefreshToken` (`backend/internal/oidc/service.go:234-247`) calls `GetAndConsume` (Redis `GETDEL`) before checking `clientID` and authenticating the client; when `clientSecret == ""` the client is not authenticated at all, even though all clients in this codebase are created with secrets (RFC 6749 §6 requires client authentication on refresh). There is also no token-family/reuse detection: a stolen token consumed first stays valid for up to 30 days, the legitimate client's next refresh just fails silently, and anyone holding a leaked token can burn it (DoS on the grant).

Related files:
- [backend/internal/oidc/service.go](backend/internal/oidc/service.go)
- [backend/internal/oidc/refreshtoken.go](backend/internal/oidc/refreshtoken.go)

Acceptance criteria:
- Client identity is verified (secret whenever the client has one) before the refresh token is consumed.
- Reuse detection per RFC 6819 §5.2.2.3: presentation of an already-consumed token invalidates the whole grant chain for that user+client and raises an audit/security event.

Status update:
- `ExchangeRefreshToken` now looks up the client and authenticates it BEFORE consuming the refresh token: whenever the client has a stored secret hash (every client created through the admin API does), a valid `client_secret` is mandatory; failed authentication returns `invalid_grant` and leaves the token unconsumed. `oidc.Service.clientRepo` was narrowed to a small `clientAuthenticator` interface so tests can stub it.
- Reuse detection (RFC 6819 §5.2.2.3): each authorization-code exchange starts a token family (`FamilyID` on `OIDCRefreshSession`, Redis set `oauth2:refresh:family:<fid>`); consuming a token leaves a tombstone (`oauth2:refresh:used:<hash>`). Presenting an already-consumed token deletes every live token in the family, logs `slog.Warn`, and surfaces `ErrRefreshTokenReuse` — mapped to `invalid_grant` at the endpoint with `refresh_token_reuse_detected` recorded as the audit event reason (`oidc.token_exchange_failed`).
- Explicit RFC 7009 revocation uses the new `Delete` (no tombstone), so deliberate revocation is not later misread as theft.
- Tests: `TestExchangeRefreshToken_ClientAuthenticatedFirst` (wrong/empty secret → `invalid_grant`, token not consumed; correct secret rotates), reuse assertions in `TestRefreshTokenStore` and `TestExchangeRefreshToken` (replay returns `ErrRefreshTokenReuse`, rotated sibling token dies with the family).
- All 20 packages pass.

### 86. "Require hardware attestation" setting is bypassable by software authenticators

State: CLOSED

Severity: Medium

`require_hardware_attestation` enforcement (`backend/internal/webauthn/service.go:216-233`) only rejects `AttestationType == "none"` or an all-zero AAGUID. go-webauthn is constructed without an MDS/trust-anchor config (`service.go:66-76`), so attestation trust is never validated — a software authenticator presenting a `packed` self-attestation with an arbitrary non-zero AAGUID passes. An org admin enabling this setting gets no real hardware-only guarantee.

Related files:
- [backend/internal/webauthn/service.go](backend/internal/webauthn/service.go)

Acceptance criteria:
- When the setting is on, go-webauthn is configured with an MDS provider or explicit attestation root CAs, and only Basic/AttCA attestation with validated metadata is accepted; or the setting is clearly documented/renamed as advisory.

Status update:
- Chose the hybrid option (no offline MDS available): enforcement is tightened AND the setting is documented as advisory. `FinishRegistration` now accepts only certificate-chain-backed attestation types (`basic_full` / `attca`); `none` and self-attestation (`basic_surrogate`) — the exact fake-AAGUID bypass from this issue, which go-webauthn classifies as `basic_surrogate` — are rejected with `ErrHardwareAttestationRequired` (`backend/internal/webauthn/service.go`). The all-zero AAGUID check is kept.
- Residual limitation (x5c chain not anchored to vendor root CAs without an MDS provider) is documented in `docs/settings.md`, `docs/security-model.md`, the admin UI description strings (`frontend/src/locales/en.json`, `tr.json`), and a startup warning is logged when the setting is enabled (`backend/cmd/server/main.go`).
- Test: `TestHardwareAttestationEnforcement` updated — packed self-attestation with a non-zero AAGUID is now rejected.
- All 20 packages pass.

### 87. No brute-force guard on `/mfa/setup` current-code and `/mfa/verify`

State: CLOSED

Severity: Medium

The previous round added failure counters to step-up and `/mfa/disable`, but `Handler.Setup` (`backend/internal/mfa/handler.go:94-124`) and `Handler.Verify` (`handler.go:125-153`) have no attempt counter — `totp.Validate` can be retried forever at up to 300 req/min (protectedRL). With a stolen access token, an attacker can brute-force the 6-digit `current_code` (~333k expected guesses in ~18 h) and rotate the TOTP secret to their own, gaining MFA persistence.

Related files:
- [backend/internal/mfa/handler.go](backend/internal/mfa/handler.go)

Acceptance criteria:
- The `mfaAttemptsExceeded`/`recordMFAFailure` counter pattern from `Disable` is applied to the `current_code` path of `Setup` and to `Verify`.
- Test: N consecutive failures lock further attempts for the cooldown window.

Status update:
- Added `mfaSetupAttemptsKey` (`mfa:setup:fails:<uid>`) and `mfaVerifyAttemptsKey` (`mfa:verify:fails:<uid>`) counters reusing the existing `mfaAttemptsExceeded`/`recordMFAFailure`/`clearMFAFailures` helpers (5 failures / 10-minute window → 429 `too_many_attempts`), in `backend/internal/mfa/handler.go`.
- `Setup`: failures are counted on `invalid_code` and `code_already_used` (a replayed valid code is still a consumed guess); `current_code_required` is not counted (no code was tried). Success clears the counter. `Verify`: any failed verification counts; success clears.
- Tests: `TestMFAHandlerSetup_BruteForceLockout`, `TestMFAHandlerVerify_BruteForceLockout`, `TestMFAHandlerVerify_SuccessClearsFailures` (miniredis-backed, mirroring the Disable lockout tests).
- All 20 packages pass.

### 88. Open redirect via `redirect_to` on the login page

State: CLOSED

Severity: Medium

`frontend/src/pages/auth/LoginPage.tsx:26-33` reads `redirect_to` from the query string and navigates via `window.location.href` whenever it starts with `/oauth2/` **or `http`**. A link like `https://<idp>/auth/login?redirect_to=https://evil.example/fake-login` sends the victim to an attacker origin after they complete the full login — a strong phishing primitive on an identity provider ("session expired, sign in again").

Related files:
- [frontend/src/pages/auth/LoginPage.tsx](frontend/src/pages/auth/LoginPage.tsx)

Acceptance criteria:
- Only same-origin absolute paths are honored (`startsWith("/") && !startsWith("//")`); the `http` branch is removed.
- Test: `redirect_to=https://evil.example` lands on the default dashboard route.

Status update:
- Rewrote `completeLogin` in `frontend/src/pages/auth/LoginPage.tsx`: the `http` branch is gone; only values matching `startsWith("/") && !startsWith("//")` are honored, anything else falls back to `/dashboard`. Legitimate `/oauth2/` consent redirects still do a full navigation via `window.location.href` (backend route), other same-origin paths use client-side `navigate`.
- Added four tests in `frontend/src/pages/auth/LoginPage.test.tsx`: `redirect_to=https://evil.example` and `//evil.example` fall back to `/dashboard`; `/oauth2/authorize?...` still navigates to the backend; other same-origin paths navigate client-side.
- `npm test` passes; `npm run build` passes.

### 89. Password-reset tokens sent over plaintext when SMTP lacks STARTTLS

State: CLOSED

Severity: Medium

Both mailer send paths use `smtp.SendMail` (`backend/pkg/mailer/mailer.go:62,106`), which transmits in cleartext when the server does not advertise STARTTLS. The message body contains the live 1-hour account-takeover reset URL; `smtp.PlainAuth` protects only the SMTP password, not the body, and when `SMTP_USER` is empty even that guard is gone.

Related files:
- [backend/pkg/mailer/mailer.go](backend/pkg/mailer/mailer.go)

Acceptance criteria:
- Sending fails closed unless the connection is TLS (verified STARTTLS upgrade or implicit TLS on port 465); plaintext delivery is never attempted outside an explicit dev mode.

Status update:
- `SMTPMailer.send` (`backend/pkg/mailer/mailer.go`) now requires TLS on the wire: implicit TLS when the port is `465`, otherwise a verified STARTTLS upgrade. A server offering no STARTTLS is rejected with `ErrPlaintextSMTP` unless `AllowPlaintext` is set.
- `AllowPlaintext` is wired exclusively to the new `DEV_MODE` flag (`cmd/server/main.go`) — never enabled outside local development. See #90 for `DEV_MODE`'s full behavior.
- Existing `TestSMTPMailer` coverage in `pkg/mailer` exercises the STARTTLS/implicit-TLS/plaintext-rejection paths; all pass.

### 90. LogMailer writes the raw password-reset token to application logs

State: CLOSED

Severity: Medium

`LogMailer.SendPasswordReset` logs the full reset URL including the raw token (`backend/pkg/mailer/mailer.go:21-23`), and LogMailer is the default whenever `SMTP_HOST` is empty (`backend/cmd/server/main.go:228-234`) — which is exactly what the shipped `infra/docker-compose.yml:50` sets. Anyone with log access (docker logs, journald, Loki) can trigger `POST /auth/forgot-password` for a victim and read a live account-takeover token from the logs.

Related files:
- [backend/pkg/mailer/mailer.go](backend/pkg/mailer/mailer.go)
- [backend/cmd/server/main.go](backend/cmd/server/main.go)
- [infra/docker-compose.yml](infra/docker-compose.yml)

Acceptance criteria:
- LogMailer never logs the URL/token (recipient only), and the server refuses to start with password reset enabled but no SMTP outside an explicit dev mode flag.

Status update:
- `LogMailer.SendPasswordReset` logs only the recipient (`to`), never the reset URL/token — this was already the case in the pre-existing code, and is now backed by an explicit fail-fast.
- Added a `DEV_MODE` config flag (`backend/cmd/server/main.go`, default `false`). `loadConfig` now refuses to start when `SMTP_HOST` is empty and `DEV_MODE` is not `true`: `"SMTP_HOST must be configured ... set DEV_MODE=true to allow logging reset links to the console instead, for local development only"`. With `DEV_MODE=true` and no `SMTP_HOST`, the server falls back to `LogMailer` exactly as before (console-only, no token ever logged).
- `infra/docker-compose.yml` sets `DEV_MODE: "${DEV_MODE:-true}"` next to its existing empty `SMTP_HOST` so the shipped example compose keeps booting; the comment there says explicitly to set it `false` and configure real SMTP for any non-local deployment.
- Added a real `TestMain(m *testing.M)` to `cmd/server/main_test.go` that sets `DEV_MODE=true` process-wide for the whole test binary (inherited by `exec.Command`-spawned subprocess tests too), so the ~20 existing tests that call `loadConfig()`/`run()` didn't each need an explicit env var.
- Tests: `TestLoadConfig_RequiresSMTPOutsideDevMode`, `TestLoadConfig_DevModeAllowsMissingSMTP`, `TestLoadConfig_SMTPHostConfiguredAllowsDevModeFalse` in `main_test.go`.
- Documented `DEV_MODE` and the new required-outside-dev-mode status of `SMTP_HOST` in `docs/configuration.md`, including a new "Development Mode" section and a production-checklist line.
- All 20 packages pass.

### 91. Hardcoded default `DATABASE_URL` / `REDIS_PASSWORD` with `sslmode=disable`

State: CLOSED

Severity: Medium

`backend/cmd/server/main.go:1127,1132` default to published credentials (`zerotrust:zerotrust_secret`, `sslmode=disable`). A deployment that forgets these env vars boots silently with well-known credentials and plaintext DB transport — unlike the strict fail-fast handling of `MFA_ENCRYPTION_KEY`/`JWT_SIGNING_ALG` in the same file.

Related files:
- [backend/cmd/server/main.go](backend/cmd/server/main.go)

Acceptance criteria:
- Startup fails when `DATABASE_URL`/`REDIS_PASSWORD` are unset (or when defaults coincide with non-loopback hosts); dev defaults live only in compose files.

Status update:
- `loadConfig` (`backend/cmd/server/main.go`) now requires `DATABASE_URL` and `REDIS_PASSWORD` to be explicitly set unless `DEV_MODE=true` (see #90), reusing the same flag rather than adding a separate one. With `DEV_MODE=true`, the previous local-development defaults (`postgres://zerotrust:zerotrust_secret@localhost:5432/zerotrust_db?sslmode=disable`, `zerotrust_secret`) are used exactly as before; otherwise startup fails with a clear error naming the missing variable.
- `infra/docker-compose.yml` already sources `DATABASE_URL`/`REDIS_PASSWORD` from `.env` (via `generate-secrets.sh`), so the shipped compose example is unaffected.
- Tests: `TestLoadConfig_RequiresDatabaseURLOutsideDevMode`, `TestLoadConfig_RequiresRedisPasswordOutsideDevMode`, `TestLoadConfig_DevModeAllowsDefaultDatabaseAndRedis` in `main_test.go`.
- Documented in `docs/configuration.md` (marked "Unless `DEV_MODE=true`" in the variable tables, plus a production-checklist line).
- All 20 packages pass.

### 92. `/auth/register` has no endpoint-specific rate limit

State: CLOSED

Severity: Medium

`backend/cmd/server/main.go:421` mounts registration with only `publicAudit` (login/forgot-password get `loginRL` at 10/min). When `REGISTRATION_ENABLED=true`, each request does bcrypt hashing, a user insert, an audit insert, and a full auto-login (second bcrypt compare + session row) — enabling mass account creation, unbounded table growth, and double-bcrypt CPU burn at 300 req/min/IP.

Related files:
- [backend/cmd/server/main.go](backend/cmd/server/main.go)

Acceptance criteria:
- Registration sits behind a strict limiter (e.g. 5-10/min/IP plus a daily cap), with CAPTCHA/email verification considered when public registration is on.

Status update:
- `POST /api/v1/auth/register` now sits behind `loginRL` (10 req/min/IP), the same limiter used for login/forgot-password/reset-password (`backend/cmd/server/main.go`).
- A daily cap and CAPTCHA/email verification were not added — no CAPTCHA infra exists in this codebase and a per-IP daily cap needs its own design (e.g. distinguishing legitimate shared-IP users); flagging as a possible follow-up rather than adding ad hoc.
- cmd/server integration tests that call `/auth/register` from the same loopback IP across many test functions all share one Redis test instance; `setIntegrationRedisEnv` (`main_test.go`) now flushes that Redis instance at the start of each test so the newly-added limiter doesn't trip across unrelated tests in the same local run (CI already gets a fresh Redis per run).
- All 20 packages pass.

### 93. `/auth/forgot-password`: no per-email throttle (mail bombing + reset-token churn)

State: CLOSED

Severity: Medium

Each request spawns `go h.passwordResetter.SendReset(...)` (`backend/internal/auth/handler.go:652`); there is no per-recipient cooldown and every request invalidates all previous reset tokens (`backend/internal/passwdreset/repository.go:54-60`). `loginRL` limits per source IP only, so a distributed attacker can flood a victim's mailbox indefinitely and keep invalidating any legitimate reset link; slow SMTP also accumulates unbounded goroutines.

Related files:
- [backend/internal/auth/handler.go](backend/internal/auth/handler.go)
- [backend/internal/passwdreset/service.go](backend/internal/passwdreset/service.go)

Acceptance criteria:
- A per-email cooldown (e.g. Redis key with 5-min TTL, still returning 200) throttles repeat sends.
- Reset mail goes through the resilient mailer or a bounded worker pool.

Status update:
- `passwdreset.Service` gained an optional `SetRedis(rdb)` wiring (`backend/internal/passwdreset/service.go`); `SendReset` now takes a Redis `SETNX` lock keyed by `pwdreset:cooldown:<sha256(lowercased email)>` with a 5-minute TTL before creating a new token. A request within the cooldown window returns silently (still 200 from the handler, per the existing enumeration-safe design) without touching the existing token. Redis errors fail open (a `slog.Warn` is emitted) so a Redis outage doesn't block password resets. Wired in `main.go` via `prSvc.SetRedis(rdb)`.
- Reset mail now goes through the resilient/bounded mailer queue: `passwdreset.NewService` in `main.go` is now constructed with `resilientMailer` instead of the raw mailer, and `main.go`'s mailer wiring was reordered so `resilientMailer` exists before `prSvc` is built.
- This also completed a previously half-finished change already sitting in the tree: `ResilientMailer.SendPasswordReset` had been switched to queue an `AlertJob{ResetURL: ...}` on the bounded worker pool, but `processJob` still unconditionally called `SendSecurityAlert`, silently dropping every queued password-reset job (a pre-existing, uncommitted regression — confirmed by a failing test before this fix). `processJob` now dispatches to `SendPasswordReset` or `SendSecurityAlert` based on `job.ResetURL`.
- Tests: `TestSendReset_CooldownThrottlesRepeatSends`, `TestSendReset_CooldownIsPerEmail`, `TestSendReset_NoRedisConfiguredSendsEveryTime` (`passwdreset/service_test.go`, miniredis-backed); `TestResilientMailer_SendPasswordResetDelegatesToUnderlying` (updated to run through the async worker), `TestResilientMailer_PasswordResetQueueFull`, `TestResilientMailer_PasswordResetRetriesOnFailure` (`pkg/mailer/resilient_test.go`).
- All 20 packages pass.

### 94. `/auth/refresh` failures fan out to audit DB + webhook with no endpoint limit

State: CLOSED

Severity: Medium

Failed refreshes log `auth.refresh_failed` with outcome `failure` (`backend/internal/auth/handler.go:510-517`); all `auth.*` failures are classified high-risk, so each attempt also spawns an outbound webhook POST goroutine when webhooks are enabled (`backend/internal/audit/webhook.go:47-48`, `repository.go:151-155`). At 300 req/min/IP with any/empty cookie, an unauthenticated attacker grows `audit_logs` and hammers the configured webhook endpoint (Slack will rate-limit or disable it, killing the alerting channel).

Related files:
- [backend/cmd/server/main.go](backend/cmd/server/main.go)
- [backend/internal/audit/webhook.go](backend/internal/audit/webhook.go)

Acceptance criteria:
- `/auth/refresh` sits behind `tokenRL` (or a dedicated limiter), and webhook dispatch is deduped/throttled per IP+window.

Status update:
- `POST /api/v1/auth/refresh` now sits behind `tokenRL` (30 req/min/IP), the same limiter used for `/auth/token` (`backend/cmd/server/main.go`).
- Webhook dispatch is now throttled per IP+action: `audit.Repository` gained an in-memory `shouldDispatchWebhook(ip, action)` check (1-minute window per key, with opportunistic pruning) called before the existing `go func` webhook dispatch in `Log`. A flood of identical high-risk events from one IP (e.g. repeated `auth.refresh_failed`) now fires the webhook once per window instead of once per event; different source IPs are tracked independently.
- Tests: `TestRepository_LogThrottlesRepeatedWebhooksBySameIP` (5 identical events from one IP → 1 webhook call), `TestRepository_LogDoesNotThrottleDifferentIPs` (2 distinct IPs → 2 calls), in `internal/audit/webhook_test.go`.
- All 20 packages pass.

### 95. MFA recovery codes are not atomically single-use

State: CLOSED

Severity: Medium

`Service.Validate` (`backend/internal/mfa/service.go:178-195`) reads all code hashes, bcrypt-compares, then rewrites the array minus the match via a plain `UPDATE` with no transaction or lock (`backend/internal/mfa/repository.go:76-83`). Two concurrent requests with the same code both succeed (single-use broken); concurrent use of two different codes can lose one update, resurrecting a consumed code.

Related files:
- [backend/internal/mfa/service.go](backend/internal/mfa/service.go)
- [backend/internal/mfa/repository.go](backend/internal/mfa/repository.go)

Acceptance criteria:
- Code consumption is atomic (one row per code hash with `DELETE ... RETURNING`, or a per-user lock around validate-and-remove).
- Test: concurrent use of the same code yields exactly one success.

Status update:
- Chose the per-user lock option (no schema change): new `Repository.ConsumeRecoveryCode` runs `SELECT recovery_codes ... FOR UPDATE` in a transaction, invokes the bcrypt matcher callback under the lock, rewrites the array minus the match, and commits. A blocked concurrent transaction re-reads the latest committed array on unblock, so the same code cannot be consumed twice and concurrent use of different codes cannot lose updates (`backend/internal/mfa/repository.go`, wired into `Service.validate` in `service.go`; `RecoveryCodes`/`UpdateRecoveryCodes` remain for regeneration and reads).
- Tests: `TestRecoveryCodeConcurrentUse` (integration, 16 goroutines racing one code → exactly 1 success; skips when no test DB), `TestValidateFailsWhenRecoveryCodeCannotBeConsumed`, plus the existing single-use test updated for the new stub.
- All 20 packages pass.

### 96. WebAuthn signCount clone warning is collected but never enforced

State: CLOSED

Severity: Low

`FinishLogin`/`FinishDiscoverableLogin` (`backend/internal/webauthn/service.go:286-296, 356-369`) persist whatever `ValidateLogin` returns; the library only sets `CloneWarning = true` on counter regression without erroring, and the app never inspects it before issuing tokens (`backend/internal/auth/service.go:758-788`). A cloned counter-using authenticator signs in undetected.

Acceptance criteria:
- `CloneWarning` rejects the assertion (spec-recommended) or at minimum triggers an audit entry and security alert.

Status update:
- Both `FinishLogin` and `FinishDiscoverableLogin` now reject the assertion with `ErrCredentialCloneDetected` when `cred.Authenticator.CloneWarning` is set, before any token issuance or counter update, and emit a `slog.Warn` with user/credential IDs (`backend/internal/webauthn/service.go`).
- Test: `TestFinishLogin_CloneWarningRejected` — a counter regression (5 → 3) on the second assertion is rejected.
- All 20 packages pass.

### 97. WebAuthn second-factor finish omits the account-status check

State: CLOSED

Severity: Low

`MFAChallenge` and `WebAuthnPasswordlessFinish` reject inactive users, but the WebAuthn second-factor finish path checks only `err != nil` on `FindByID` (`backend/internal/auth/service.go:783-786`). A user deactivated between the password step and the passkey assertion still gets a token pair and a session row (largely unusable because middleware re-checks `is_active`, but inconsistent with the fixes from the previous round).

Acceptance criteria:
- `!u.IsActive` is rejected at `service.go:784` like its sibling paths.

Status update:
- `WebAuthnLoginFinish` now rejects with `ErrInvalidCredentials` when `!u.IsActive` (`backend/internal/auth/service.go`).
- Test: `TestWebAuthnLoginFinish_InactiveUserRejected` — user deactivated after the password step is rejected at finish.
- All 20 packages pass.

### 98. Refresh reuse-detection false positive on explicitly revoked tokens

State: CLOSED

Severity: Low

`CheckReuse` (`backend/internal/session/repository.go:178-194`) treats any revoked session older than 5 s as theft — including sessions revoked by explicit logout or admin action. Replaying a captured logged-out token repeatedly triggers `RevokeAllForUser`, force-logging-out the victim on demand.

Acceptance criteria:
- Revocation reason is recorded (e.g. `revoked_by_rotation` set only in `RotateSession`) and mass-revocation triggers only for rotation-revoked hashes.

Status update:
- Migration `000034_sessions_revoked_reason` adds a nullable `sessions.revoked_reason TEXT` column.
- `RotateSession` now sets `revoked_reason = 'rotation'` on the superseded row — the only reason `CheckReuse` matches (`AND revoked_reason = 'rotation'` added to its query). Every other revoke path now records its own distinct reason for observability: `Revoke` (logout) → `'logout'`, `RevokeAllForUser` → `'mass_revoke'`, `RevokeOtherSessions` → `'revoke_others'`, `RevokeByID` → `'revoke_by_id'`, `RevokeStaleInitialSessions` → `'stale_initial'`, `EvictExcessSessions` → `'evicted'`, `RevokeForDevice` → `'device_replaced'`.
- Replaying a logged-out (or admin-revoked, evicted, etc.) token is still correctly rejected as invalid — it just no longer triggers a mass revocation of the user's other sessions.
- Also fixed a hand-rolled `sessions` table in `session_refresh_integration_test.go` (`createAuthIntegrationSchema`) that predates the real migrations and needed the new column added to match.
- Tests: `TestRefreshTokens_ReplayOfExplicitlyRevokedTokenDoesNotMassRevoke` (logout → replay → other active session survives) alongside the existing `TestRefreshTokens_ReusedRotatedTokenRevokesAllSessions` (rotation replay still mass-revokes, unchanged) in `session_refresh_integration_test.go`.
- All 20 packages pass.

### 99. SSE session-events stream outlives token validity and revocation

State: CLOSED

Severity: Low

`GET /api/v1/sessions/events` (`backend/internal/session/handler.go:79-149`) validates the access token once at connect; the stream then runs indefinitely even after the 1-minute token expires or its JTI is blocklisted at logout, giving a stolen token a persistent live feed of the victim's session activity.

Acceptance criteria:
- The stream closes at token `exp` or re-checks `IsRevoked(jti)` per keepalive tick.

Status update:
- `session.Handler` gained an optional `revocationChecker` interface (`IsRevoked(ctx, jti) bool`, satisfied by `*auth.Service`) wired via `SetRevocationChecker`; `main.go` calls `sessionHandler.SetRevocationChecker(authSvc)`.
- `Events` now captures the connecting token's `ExpiresAt`/`ID` (jti) at connect time. On every keepalive tick (15s) it closes the stream with `data: expired` if the token has expired, or `data: revoked` if `IsRevoked(jti)` is true — before falling back to the existing `: keepalive` comment.
- The tick interval is now a configurable field on `Handler` (defaulting to 15s in `NewHandler`; production behavior unchanged) so tests can exercise the tick logic without waiting 15 real seconds.
- Tests: `TestEvents_TokenExpiryEndsStream`, `TestEvents_RevokedJTIEndsStream`, `TestEvents_ValidTokenSurvivesTicksWithoutRevocationChecker` in `session/handler_test.go`.
- All 20 packages pass.

### 100. Internal RBAC roles disclosed to third-party clients as `roles`/`groups` claims

State: CLOSED

Severity: Low

The ID token (`backend/internal/oidc/service.go:188-189`) and UserInfo (`backend/internal/oidc/handler.go:425-426`) map `u.Roles` to `roles` and `groups` whenever `profile` scope is granted, leaking the internal authorization model to any consented external client.

Acceptance criteria:
- `roles`/`groups` are dropped from OIDC-issued tokens or gated behind a dedicated scope allowed per client by an admin.

Status update:
- ID tokens no longer carry `roles`/`groups` (dropped from the `profile` scope claims in `ExchangeCode`).
- UserInfo returns `roles`/`groups` only on the first-party compatibility path (internal session tokens without a scope claim); for OIDC-issued tokens they are never included. OIDC access tokens also stopped embedding `Roles` entirely as part of #81.
- Asserted by `TestExchangeCode_AccessTokenCarriesNoInternalClaims`; existing UserInfo scope-filtering tests unchanged and passing.
- Removed the now-inaccurate "roles/groups claims" OIDC marketing line from `docs/index.md` and `docs/index.html`.
- All 20 packages pass.

### 101. OIDC Revoke/Introspect do not verify the token belongs to the calling client

State: CLOSED

Severity: Low

`backend/internal/oidc/handler.go:494-519` (Revoke) lets any authenticated client blocklist any access-token JTI or consume any refresh token it possesses; `:717-749` (Introspect) returns `sub`/`email`/`roles` for any token regardless of `aud`. RFC 7009 §2.1 and RFC 7662 recommend scoping both to the requesting client.

Acceptance criteria:
- Both endpoints compare the token's audience/refresh-session `ClientID` against the authenticated `client_id` and return 200 / `active:false` on mismatch.

Status update:
- `Revoke`: an access-token JTI is blocklisted only when the token's `aud` contains the authenticated `client_id`; a refresh token is consumed only when its stored grant's `ClientID` matches (new `RefreshTokenStore.Peek` for the ownership check + `Delete` for tombstone-free removal; ownership enforced in `Service.RevokeRefreshToken`). Mismatches are silently ignored and still return 200 per RFC 7009.
- `Introspect`: tokens whose `aud` does not contain the calling `client_id` return `active:false` (logged as such); internal first-party tokens carry no `aud`, so external clients can no longer introspect them either. Both endpoints now validate via `ValidateOIDCAccessToken`.
- Tests: `TestRevoke_CrossClientOwnership` (cross-client revoke returns 200 but does not blocklist; owning client does), cross-client refresh-token case in `TestRevokeRefreshToken`, foreign-audience `active:false` case added to `TestHandler_Introspect_Integration` (updated to introspect an `aud`-matched OIDC token).
- All 20 packages pass.

### 102. TOTP replay protection fails open silently on Redis errors

State: CLOSED

Severity: Low

`markUsed` (`backend/internal/mfa/service.go:57-64`) returns "not used" on any Redis error, silently disabling TOTP single-use enforcement during an outage — exactly when captured codes could be replayed against step-up MFA or `/mfa/disable`.

Acceptance criteria:
- High-risk operations (step-up, disable) fail closed on Redis errors; ordinary logins may stay fail-open but must emit an audit/metric event when the fail-open path is taken.

Status update:
- `markUsed` now returns `(used bool, err error)`; callers decide the failure policy (`backend/internal/mfa/service.go`).
- Fail closed (new `ErrReplayUnavailable`): `Service.Disable`, the `Setup` current-code rotation path, and the new `Service.ValidateStepUp` used by both step-up paths — `Handler.StepUp` and the `RequireRecentMFA` middleware (`auth.MFAChecker` gained `ValidateStepUp`; `pkg/middleware/stepup_mfa.go` calls it). `/mfa/disable` and `/mfa/setup` map it to 503 `mfa_unavailable`.
- Fail open (ordinary login only): `Service.Validate` keeps accepting on Redis errors but now emits a `slog.Warn` ("TOTP replay protection unavailable") with user ID and the fail-open/fail-closed mode every time the degraded path is taken.
- Tests: `TestDisable_FailsClosedWhenRedisDown`, `TestValidate_FailOpenVsFailClosed`, `TestMFAHandlerDisable_ReplayUnavailableFailsClosed`.
- All 20 packages pass.

### 103. `MFA_ENCRYPTION_KEY` has no rotation path

State: CLOSED

Severity: Low

A single 32-byte key encrypts all TOTP secrets (`backend/cmd/server/main.go:1088-1105`). Rotating it renders every stored secret undecryptable — a forced rotation silently breaks MFA for all users, unlike JWT keys (primary+secondary) and the Vault transit key (versioned envelopes).

Acceptance criteria:
- A secondary/previous key is tried on decrypt failure (mirroring the JWT keystore), or ciphertexts are version-prefixed.

Status update:
- New optional `MFA_ENCRYPTION_KEY_PREVIOUS` env var (64 hex chars / 32 bytes, validated at startup; warned-and-ignored when `MFA_ENABLED=false`, mirroring the primary key handling in `backend/cmd/server/main.go`).
- `mfa.NewService` accepts variadic previous keys; `decryptHex` tries the primary key first and falls back to each previous key on decrypt failure (`backend/internal/mfa/service.go`). New ciphertexts always use the primary key, mirroring the JWT primary+secondary keystore pattern.
- Documented in `docs/configuration.md`.
- Test: `TestPreviousEncryptionKey` — a secret encrypted under the old key validates via the previous key and fails without it.
- All 20 packages pass.

### 104. Webhook allows `http://` URLs, sending security alerts cleartext

State: CLOSED

Severity: Low

`validateWebhookURL` (`backend/internal/audit/webhook.go:91-92`) accepts plain `http`; the payload includes user email, IP, and security-event details sent in the clear.

Acceptance criteria:
- `https` is required (optionally with an explicit allow-insecure setting for dev).

Status update:
- `validateWebhookURL` now requires the `https` scheme; plain `http` is accepted only when the new `webhook_allow_insecure` system setting is `true` (read via the repository's `SettingsReader` at dispatch time, so both `sendWebhook` and `TestWebhook` enforce it). SSRF/private-IP checks are unchanged.
- New setting seeded by migration `000032_webhook_allow_insecure` (default `false`) and added to `allowedKeys` in `backend/internal/settings/handler.go`; documented in `docs/settings.md`.
- Tests: `TestValidateWebhookURL` gained cases for http-rejected-by-default / http-allowed-with-allow-insecure / https-allowed (public literal IPs, no DNS dependency).
- All 20 packages pass.

### 105. OIDC endpoints return and persist raw internal error strings

State: CLOSED

Severity: Low

The unauthenticated `/oauth2/token` endpoint returns `err.Error()` in `error_description` (`backend/internal/oidc/handler.go:367`, also `:285`); non-sentinel errors (e.g. Redis dial failures from `service.go:218`) leak infrastructure details to anonymous callers. The same string is stored in audit metadata under `reason`, one of the fields excluded from encryption (`backend/internal/audit/repository.go:86-95`).

Acceptance criteria:
- Non-sentinel errors map to `invalid_grant`/`server_error` at the handler boundary (detail logged server-side), and raw `err.Error()` never lands in audit metadata.

Status update:
- New `isSafeOIDCError` helper classifies the package's sentinel errors (`ErrInvalidGrant`, `ErrInvalidRedirectURI`, `ErrInvalidScope`, `ErrCodeVerifierFailed`, `ErrRefreshTokenReuse` — the reuse sentinel intentionally survives as the audit reason `refresh_token_reuse_detected`). In `Token`, sentinels still return 400 `invalid_grant` with their protocol-safe description; anything else is logged via `slog.Error` and returns 500 `server_error` with no `error_description`, and the audit `reason` is the fixed string `internal_error` — raw `err.Error()` never reaches the client or audit metadata.
- `Consent`'s auth-code-creation failure path no longer returns `err.Error()`; it logs via `slog.Error` and answers a bare `server_error`.
- Tests: `TestHandler_Consent_InternalErrorNotLeaked` (closed miniredis → raw dial error stays server-side), `TestHandler_Token_InternalErrorNotLeaked` (non-sentinel → 500/no description, audit reason `internal_error`), `TestIsSafeOIDCError`.
- All 20 packages pass.

### 106. OIDC client create/update/delete lack dedicated audit actions and webhook alerts

State: CLOSED

Severity: Low

`CreateClient`/`DeleteClient`/`UpdateClient` (`backend/internal/oidc/handler.go:553,604,631`) write no `logAudit` entries (unlike `RotateClientSecret`), and `isHighRiskEvent` has no `oidc.client_*` entries — so no security alert fires for credential-creating/destroying operations.

Acceptance criteria:
- Explicit audit entries (actor, client_uuid, client_id) for all three operations, added to `criticalActions` in `webhook.go`.

Status update:
- `CreateClient`/`UpdateClient`/`DeleteClient` now emit `oidc.client_created`/`oidc.client_updated`/`oidc.client_deleted` audit entries following the `RotateClientSecret` pattern (actor from `authmw.ClaimsFrom`, resource `oauth2_client`, metadata `client_uuid` + `client_id`). `ClientRepository.Delete` now uses `DELETE … RETURNING client_id` so the delete entry carries the `client_id` too; the `clientStore` interface and test stub were updated accordingly.
- All three actions added to `criticalActions` in `backend/internal/audit/webhook.go`, so high-risk webhook alerts fire for them.
- Tests: `TestHandler_AuditLogging_ClientLifecycle` (create→update→delete each produce their audit entry, verified against a real DB); new `oidc.client_*` cases in `TestIsHighRiskEvent`.
- All 20 packages pass.

### 107. `sessions.refresh_token_hash` has no UNIQUE constraint

State: CLOSED

Severity: Low

Plain index only (`backend/migrations/000001_init_schema.up.sql:24,35`), unlike every sibling token table. `FindUserIDByHash` has no `LIMIT 1` and returns an arbitrary row on duplicates; `RotateSession`'s revoke step would revoke all rows sharing a hash. Defense-in-depth/consistency (exploitation needs a SHA-256 collision).

Acceptance criteria:
- A migration adds `CREATE UNIQUE INDEX` on `sessions(refresh_token_hash)` (after verifying no duplicates) and/or `LIMIT 1` on the lookup.

Status update:
- Migration `000033_sessions_refresh_token_hash_unique`: the up migration first runs a `DO` block that raises an exception if any duplicate `refresh_token_hash` values exist (guard per the acceptance criteria), then replaces the plain `idx_sessions_token_hash` with a `CREATE UNIQUE INDEX` of the same name; the down migration restores the plain index. Both directions verified against a live PostgreSQL.
- `FindUserIDByHash` (`backend/internal/session/repository.go`) also gained `LIMIT 1` as defense-in-depth.
- All 20 packages pass (integration tests ran the new migration via `RunMigrations`).

### 108. Down-migration 000027 re-seeds the demo OAuth2 client with a published secret

State: CLOSED

Severity: Low

`backend/migrations/000027_remove_demo_client.down.sql:3-4` re-inserts `demo-client` with the same bcrypt hash whose secret is in public git history. A botched rollback silently restores a fully usable OAuth2 client.

Acceptance criteria:
- The down migration becomes a no-op (or re-seeds with a freshly generated secret).

Status update:
- `000027_remove_demo_client.down.sql` is now a no-op (`SELECT 1`) with an explanatory comment pointing admins at the admin API for creating a demo client with a fresh secret. Verified against a live PostgreSQL: the down migration runs cleanly and does not re-insert `demo-client`.
- All 20 packages pass.

### 109. Service-account SSE endpoint skips `EvaluateAccess` and DPoP binding

State: CLOSED

Severity: Low

`/admin/service-accounts/events` (`backend/internal/serviceaccount/handler.go:241-292`) calls only `ValidateAccessToken` + `HasPermission`: deactivated users, role/scope changes, and IP/country/device policies are bypassed; DPoP-bound tokens are accepted without a proof; revocation is only polled every 30 s. Impact is limited (change pings only) but it is a real enforcement gap.

Acceptance criteria:
- The route moves into the authenticated group or replicates `EvaluateAccess` + DPoP verification in the handler.

Status update:
- Moved `GET /admin/service-accounts/events` into the same authenticated route group as every other `/admin/service-accounts` endpoint, behind `authmw.RequirePermission("service_accounts","read")` (`backend/cmd/server/main.go`), instead of a standalone public-group registration with its own manual token check.
- `Handler.Events` (`backend/internal/serviceaccount/handler.go`) now reads claims via `middleware.ClaimsFrom(r.Context())` — the standard `Authenticate` middleware chain (signature/expiry, `EvaluateAccess`, DPoP proof verification) runs before the handler, exactly as it does for every other protected route (matching how `session.Handler.Events` already worked). Removed the now-dead `eventAccessToken` token-extraction helper and the handler's own `*auth.KeyStore` dependency.
- Tests: rewrote `serviceaccount/handler_test.go`'s Events tests to inject claims via context instead of raw tokens (`TestEventsRejectsMissingClaims`, `TestEventsRejectsUnsupportedStreaming`, `TestEventsAcceptsClaimsFromContext`); added `TestRun_ServiceAccountEventsRequiresAuth` in `cmd/server/main_test.go` — a full end-to-end test proving no-token → 401 and a token without `service_accounts:read` → 403 against the real router.
- All 20 packages pass.

### 110. Service-account tokens are accepted on user self-service routes

State: CLOSED

Severity: Low

Nothing in the protected group rejects `claims.SubType == SubTypeService`, so a service token can invoke `/me`, `/sessions*`, `/mfa/*`, `/webauthn/*`, `/users/{id}/avatar`. Writes fail on UUID/FK constraints today, but `GET /users/{id}/avatar` lets a zero-scope token probe/retrieve avatars by user ID.

Acceptance criteria:
- A `RequireUserToken()` middleware (or equivalent) rejects service tokens on the self-service route group.

Status update:
- Added `middleware.RequireUserToken()` (`backend/pkg/middleware/rbac.go`): rejects the request with 403 unless `claims.SubType == auth.SubTypeUser`.
- Wrapped the self-service routes (`/me`, `/me/*`, `/sessions*`, `/mfa/*`, `/webauthn/*`, `/users/{id}/avatar`) in a nested `r.Group` inside the protected group with `r.Use(authmw.RequireUserToken())` (`backend/cmd/server/main.go`), leaving admin/service-account/OIDC-client routes (which already gate on permissions/scopes) untouched.
- Tests: `TestRequireUserToken` (`pkg/middleware/rbac_test.go`) and an end-to-end `TestRun_ServiceAccountTokenRejectedOnSelfServiceRoutes` (`cmd/server/main_test.go`) that mints a real client-credentials service-account token and confirms `GET /me` returns 403.
- Incidental finding surfaced by the new end-to-end test (not fixed, out of scope): when a service-account token reaches any audited protected route, the audit middleware attempts to insert an empty string for `user_id` (services have `ClientID`, not `UserID`), which Postgres rejects as an invalid UUID — the audit write fails (logged, non-fatal) rather than crashing the request. Worth a follow-up.
- All 20 packages pass.

### 111. Rate limiter fails open silently on Redis errors

State: CLOSED

Severity: Low

`backend/pkg/middleware/ratelimit.go:34-38` skips enforcement on any Redis error with no log or metric — during a Redis outage every limiter (including login at 10/min) is silently disabled.

Acceptance criteria:
- `slog.Warn` + metric on the error path; consider failing closed for the login limiter.

Status update:
- `RateLimiter.Middleware` now calls `slog.Warn("rate limiter unavailable, request allowed without enforcement", ...)` and increments a new in-process counter on the fail-open path (`backend/pkg/middleware/ratelimit.go`, `ratelimit_metrics.go`), mirroring the existing `audit.WriteFailures()` pattern.
- Kept fail-open behavior for all limiters, including login: failing closed on Redis errors would turn a Redis blip into a platform-wide login outage, a significant availability tradeoff the issue only asked to "consider" — flagging this choice explicitly rather than changing it silently.
- The counter is exposed via `GET /metrics` as `zerotrust_ratelimit_fail_opens_total` (`backend/cmd/server/main.go`), alongside the existing `zerotrust_audit_write_failures_total`.
- Tests: `TestRateLimiter_FailsOpenAndRecordsMetricOnRedisError` (`pkg/middleware/ratelimit_test.go`) and `TestWriteMetricsIncludesRateLimiterFailOpens` (`cmd/server/main_test.go`).
- All 20 packages pass.

### 112. Third-party stylesheet injected on the MFA page

State: CLOSED

Severity: Low

`frontend/src/pages/dashboard/MfaPage.tsx:122` injects a Google Fonts `<link>` at runtime on the page handling TOTP secrets, recovery codes, and MFA codes. A compromised/malicious CSS response can exfiltrate input values via attribute selectors or spoof the UI; no SRI is possible for that dynamic endpoint.

Acceptance criteria:
- The font is self-hosted (bundled via Vite) and the remote `<link>` removed.

Status update:
- Removed the runtime Google Fonts `<link>` from `frontend/src/pages/dashboard/MfaPage.tsx`.
- Self-hosted Outfit: downloaded the v15 woff2 files (latin + latin-ext) into `frontend/src/assets/fonts/` and added `outfit.css` with `@font-face` rules (variable weight axis 100-900, `font-display: swap`); `MfaPage.tsx` imports it via `@/assets/fonts/outfit.css`, so Vite bundles the files. No new npm dependency.

### 113. SPA served without Content-Security-Policy; frontend image uses `npm install`

State: CLOSED

Severity: Low

The backend sets `default-src 'none'` only on API responses (`backend/pkg/middleware/security.go:16`); the SPA HTML served by `frontend/nginx.conf` / edge `infra/nginx/nginx.conf` carries no CSP at all (defense-in-depth — no XSS sinks were found in `frontend/src`). Separately, `frontend/Dockerfile:4` uses `npm install` instead of `npm ci`, so image builds are not pinned to the lockfile.

Acceptance criteria:
- A restrictive CSP (e.g. `default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'`) is added for frontend responses; the Dockerfile uses `npm ci`.

Status update:
- Added a server-level `Content-Security-Policy` header to `frontend/nginx.conf` and the edge `infra/nginx/nginx.conf`: `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'`. `'unsafe-inline'` in `style-src` covers MUI/Emotion's injected `<style>` tags; SSE/fetch/EventSource are same-origin so `connect-src 'self'` suffices; the self-hosted Outfit font is same-origin (`font-src 'self'`).
- Changed `frontend/Dockerfile` from `npm install` to `npm ci` so image builds are pinned to `package-lock.json`.
- Dev workflow unaffected: neither nginx config fronts the Vite dev server, so HMR/react-refresh keep working.
- `npm test` passes; `npm run build` passes.
