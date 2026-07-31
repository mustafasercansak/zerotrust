# AGENTS.md — ZeroTrust

Guidance for AI coding agents working in this repository. Assume no prior knowledge of the project.

## Project Overview

ZeroTrust is a security-focused Zero Trust authentication and authorization platform. It is a portfolio/learning project (active development, not independently audited) that implements a full identity stack: session-cookie auth with JWT access tokens, refresh-token rotation, TOTP MFA, WebAuthn passkeys (2FA and passwordless), an OIDC/OAuth2 identity provider, service accounts (M2M `client_credentials`), DPoP (RFC 9449), RBAC, audit logging, and an admin security dashboard.

**Tech stack:**

| Layer | Technology |
|---|---|
| Backend | Go 1.25 (module `github.com/zerotrust/backend`), chi router, pgx, go-redis, golang-jwt v5, go-webauthn, pquerna/otp, golang-migrate |
| Frontend | React 19 + TypeScript, Vite, MUI v9, react-router v7, i18next (Turkish default / English), Vitest + Testing Library, Playwright |
| Data | PostgreSQL 16 (persistent state, migrations), Redis 7 (rate limits, JTI blocklist, lockouts, WebAuthn ceremony state, auth codes) |
| Infra | Docker Compose (`infra/`), nginx for production TLS termination, GitHub Actions CI |

**Request architecture:** The frontend SPA (port 3000) talks to the Go backend (port 8080) over an API proxied by Vite in dev or by nginx in prod. Auth is cookie-based: short-lived (1 min TTL) JWT access token + opaque refresh token (SHA-256 hashed in PostgreSQL) in `httpOnly` cookies, with double-submit-cookie CSRF protection (`X-CSRF-Token` header). Tokens are never exposed to JavaScript.

## Repository Layout

```
backend/
  cmd/server/          # Entry point, router wiring, config loading (main.go)
  internal/
    admin/             # Admin user/role management handler
    audit/             # Audit log + security dashboard aggregation
    auth/              # JWT, key management, token service, login handler
    mfa/               # TOTP setup/verify/disable, backup codes
    oidc/              # OIDC/OAuth2 provider (clients, auth codes, token exchange)
    passwdreset/       # Password reset tokens + mail flow
    serviceaccount/    # M2M service accounts + SSE events
    session/           # Session store (PostgreSQL) + handler
    settings/          # system_settings table access (in-process cache)
    testdb/            # Test helpers (Connect, MigrateClean, URL guard)
    user/              # User model, repository, service
    webauthn/          # FIDO2 passkey registration + login (2FA & passwordless)
  migrations/          # golang-migrate SQL files (NNNNNN_name.{up,down}.sql)
  pkg/
    crypto/            # Encryption helpers (AES-256-GCM, Vault/OpenBao transit)
    database/          # Migration runner
    geoip/             # GeoIP lookups for login geography
    mailer/            # SMTP + dev log mailer
    middleware/        # Auth, CSRF, RBAC, rate limiting, security headers
    secrets/           # Secret loading
    token/             # Token utilities
    validation/        # Email and password rules
frontend/
  src/
    pages/             # Route pages: auth/ and dashboard/
    components/        # Shared UI, layout, table components
    lib/               # API client, tokenManager, useAuth hook, utils
    contexts/ hooks/   # React contexts and hooks
    locales/           # i18n translation files (en, tr)
  e2e/                 # Playwright E2E tests
  vite.config.ts       # Dev server (port 3000) + API proxy to :8080
infra/
  docker-compose.yml       # Production base
  docker-compose.dev.yml   # Development override
  docker-compose.prod.yml  # Production override (nginx TLS, internal network)
  nginx/                   # TLS termination config
scripts/               # generate-secrets.sh, bcrypt helper, screenshots.js
secrets/               # JWT key material (gitignored contents — never commit)
docs/                  # architecture.md, api.md, configuration.md, security-model.md,
                       # settings.md, development.md — keep these in sync with code changes
```

## Build and Test Commands

Most work goes through the root `Makefile` (run `make help` for the full list):

| Command | Description |
|---|---|
| `make secrets` | Generate `infra/.env`, `infra/.env.admin`, and the JWT key pair (one-time setup; prints the admin password once) |
| `make up` | Start dev-style stack (HTTP, all ports exposed) |
| `make dev` | Start with hot reload (`docker compose watch`: Go rebuilds, Vite HMR) |
| `make up-prod` | Start production-style stack (nginx TLS, internal network only) |
| `make down` / `make down-v` | Stop services / stop and wipe volumes (resets the DB) |
| `make test` | Run all backend tests; auto-starts disposable PostgreSQL + Redis containers when `TEST_DATABASE_URL` is unset |
| `make test-cover` | Backend tests with coverage; fails below `BACKEND_COVERAGE_MIN` (90%) |
| `make test-front` | Frontend unit tests (Vitest) |
| `make lint` | `go vet ./...` + `npx tsc --noEmit` |
| `make govulncheck` | Go vulnerability scan |
| `make certs` / `make jwt-key` | Self-signed TLS cert / persistent Ed25519 JWT key for local prod testing |

Direct equivalents:

```bash
# Backend
cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...

# Frontend
cd frontend && npx tsc --noEmit && npm run lint && npm test && npm run build

# E2E (Playwright, uses system Chrome; needs backend on :8080 and two test users)
cd frontend && E2E_USER_EMAIL=... E2E_USER_PASSWORD=... E2E_ADMIN_EMAIL=... E2E_ADMIN_PASSWORD=... npm run test:e2e
```

If Docker needs elevated privileges, use `make SUDO=sudo <target>`.

## Testing Instructions

### Backend

- Tests run with `-p 1` (serial) because integration tests share database state via the `internal/testdb` package.
- **Safety guard:** `testdb.URL(t)` rejects any `TEST_DATABASE_URL` whose database name does not contain the word `test` (e.g., `zerotrust_test`). This protects real databases from destructive fixture cleanup. Never bypass it.
- Integration-test pattern:
  ```go
  db := testdb.Connect(t)    // skips if TEST_DATABASE_URL is not set
  testdb.MigrateClean(t, db) // runs migrations + truncates tables
  ```
- Test categories: unit tests with injected fakes at service boundaries; integration tests (need `TEST_DATABASE_URL`, often in `*_integration_test.go` files); handler tests exercising the real router and middleware via `httptest.NewRecorder()`.
- **Coverage target: ≥ 90% backend**, enforced in CI and by `make test-cover`. New handler code must include at least a unit test and a happy-path integration test.

### Frontend

- Vitest with `react-dom/server` rendering (not a full browser) — tests are limited to what SSR output contains.
- **useState mock convention (important):** the test harness mocks `useState` by call index. When adding a `useState` to a component that already has tests, **append it after existing hooks**; inserting in the middle shifts indices and breaks pinned tests.
- Mock at module boundaries (`vi.mock('../api/auth', ...)`), never internal component state or methods.
- E2E uses the system Chrome — no browser download needed. Auth state is saved once per role via `storageState` to stay below the backend rate limit.

### CI

GitHub Actions (`.github/workflows/ci.yml`) on pushes/PRs to `main`/`master`: backend build/vet/test against service containers, frontend type-check/lint/test/build, plus a security job (`govulncheck`, `npm audit --audit-level=high`; the react-router RSC advisory GHSA-qwww-vcr4-c8h2 is explicitly accepted as not applicable). `pages.yml` deploys the docs showcase to GitHub Pages.

## Code Style and Conventions

### General

- Primary language for code, comments, and docs: **English**. UI copy is bilingual (Turkish default, English) via i18n files in `frontend/src/locales/` — add new user-facing strings to both locales.
- Commit messages: `<type>: <imperative summary>` with types `feat`, `fix`, `security`, `refactor`, `docs`, `test`, `chore` (see `CONTRIBUTING.md`).
- **No new dependencies** without discussion — the dependency surface is treated as a security concern.
- SQL migrations: every schema change needs a numbered `golang-migrate` pair in `backend/migrations/`; migrations must be idempotent on replay.

### Backend (Go)

- Standard `gofmt`/`go vet` clean code; pass `r.Context()` through all service/repository calls — never `context.Background()` inside handlers.
- Error responses: JSON body `{"error": "<code>"}` with lowercase snake_case codes (e.g., `invalid_credentials`). Do not leak internal state to HTTP responses. Status codes follow REST semantics (400/401/403/429).
- Every state-mutating handler must write an audit log entry (`h.logAudit(...)`) whose `metadata` JSONB carries enough context to reconstruct the change (e.g., `{"from": ..., "to": ..., "outcome": "success"}`).
- Structured logging via `slog` with key-value pairs: `Info` for lifecycle, `Warn` for non-actionable anomalies, `Error` for failures needing investigation.
- Adding an endpoint: handler in `internal/<domain>/handler.go` → wire route in `cmd/server/main.go` under the right middleware group (public / `Authenticate` / `RequirePermission("resource","action")` / `RequireRole("admin")` / `stepUpMFA` for destructive admin mutations) → audit log → tests → document in `docs/api.md`.
- Adding a system setting: migration inserting into `system_settings` → read via `GetBool`/`GetInt`/`GetString` → allowlist the key in the settings handler → document in `docs/settings.md`.

### Frontend (TypeScript/React)

- Path alias `@` → `frontend/src` (configured in `vite.config.ts`).
- The Vite dev server proxies `/api`, `/.well-known`, `/health`, and the OAuth2 endpoints to the backend.
- Production builds use manual vendor chunks (react, mui, data-grid, i18n) and route-level code splitting — keep this structure when adding dependencies.
- Type-check with `npx tsc --noEmit`; lint with `npm run lint` (ESLint flat config, typescript-eslint + react-hooks + react-refresh).

## Security Considerations

- **Never commit secrets:** `infra/.env`, `infra/.env.admin`, `secrets/`, TLS certs, private keys. Double-check before committing; regenerate anything ever exposed.
- Production requires HTTPS with `COOKIES_SECURE=true`; `PUBLIC_APP_URL` must be a trusted origin (reset links are built from config, never request headers).
- Access-token TTL is hard-locked to 1 minute — do not make it configurable.
- MFA is fail-closed: disabled unless `MFA_ENABLED=true` with a valid 64-hex-char `MFA_ENCRYPTION_KEY` (AES-256-GCM); step-up protected admin actions fail closed while MFA is disabled.
- Destructive admin operations (role/status changes, session revocation, service-account and OIDC-client management) require a recent step-up MFA proof (Redis marker, 10-minute window).
- Passwords and service-account secrets use bcrypt cost 12; refresh tokens are stored only as SHA-256 hashes; token-reuse detection revokes all user sessions on suspected theft.
- Rate limiting (sliding window per IP, Redis-backed): login 10/min, OIDC sensitive endpoints 30/min, userinfo 100/min, global 300/min. Keep this in mind in tests (e2e uses saved auth state for this reason).
- When OpenBao/Vault (`BAO_*`/`VAULT_*` vars) is configured, selected PII fields are encrypted via the transit engine; replacing the transit key makes existing ciphertext undecryptable.
- Report vulnerabilities per `SECURITY.md`, not via public issues.

## Documentation Map

Keep these in sync when changing behavior:

- `docs/architecture.md` — components, request lifecycle, token design, session model, OIDC flow
- `docs/api.md` — every endpoint
- `docs/configuration.md` — all environment variables
- `docs/security-model.md` — threat model, defense layers, crypto primitives
- `docs/settings.md` — runtime `system_settings` keys
- `docs/development.md` — dev setup, test harness details, conventions (source of truth for several conventions summarized above)
