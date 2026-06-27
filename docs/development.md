# Development Guide

## Prerequisites

- Docker and Docker Compose (for the database, Redis, and full-stack environment)
- Go 1.22+ (for running backend tests without Docker)
- Node.js 20+ (for frontend development)

---

## Quick Start

```bash
# 1. Generate secrets (infra/.env, infra/.env.admin, JWT key)
cd scripts && ./generate-secrets.sh

# 2. Start with hot reload
cd infra
sudo docker compose -f docker-compose.yml -f docker-compose.dev.yml watch
```

The development compose file exposes:
- Frontend (Vite dev server with HMR): `http://localhost:3000`
- Backend: `http://localhost:8080`
- PostgreSQL: `localhost:5432`
- Redis: `localhost:6379`

Changes to Go files trigger a backend container rebuild. Changes to frontend files are picked up by Vite's HMR without a rebuild.

---

## Make Targets

Run `make help` for the full list. The most common ones:

| Command | Description |
|---|---|
| `make up` | Start in development mode (HTTP, all ports exposed) |
| `make dev` | Same but with `docker compose watch` for hot reload |
| `make down` | Stop all services |
| `make down-v` | Stop and delete volumes (resets the database) |
| `make up-prod` | Start in production mode (nginx TLS, internal network) |
| `make test` | Run all backend tests (auto-starts disposable containers) |
| `make test-cover` | Backend tests with coverage report |
| `make test-front` | Frontend tests |
| `make test-cover-all` | Backend + frontend tests with combined coverage |
| `make lint` | Run `go vet` on the backend |
| `make secrets` | Regenerate `infra/.env` |
| `make certs` | Generate a self-signed TLS cert for local HTTPS testing |
| `make jwt-key` | Generate a persistent Ed25519 JWT signing key |
| `make screenshots` | Capture all UI screenshots (requires a running stack) |

---

## Backend Tests

### Running

```bash
# Simplest — make handles disposable containers automatically
make test

# With coverage (minimum 90%)
make test-cover

# If you already have TEST_DATABASE_URL and TEST_REDIS_URL set:
cd backend && go test -count=1 -p 1 ./...
```

Tests run with `-p 1` (serial) because integration tests share database state via the `testdb` package.

### Test Database Safety

The `testdb.URL(t)` helper validates that `TEST_DATABASE_URL` points to a database whose name contains the word `test` (e.g., `zerotrust_test`). This guards against accidentally running destructive fixture cleanup against a production or staging database. The check happens before any migration runs.

```go
// Integration tests start with this pattern:
db := testdb.Connect(t)         // skips the test if TEST_DATABASE_URL is not set
testdb.MigrateClean(t, db)      // runs migrations + truncates tables
```

### Test Categories

**Unit tests** (no external dependencies) test individual functions and methods in isolation. Service-layer tests inject lightweight fakes:

```go
type fakeUserReader struct { user *user.User }
func (f *fakeUserReader) FindByEmail(_ context.Context, _ string) (*user.User, error) {
    return f.user, nil
}
```

**Integration tests** (require `TEST_DATABASE_URL`) test the full stack including database queries, migrations, and concurrent behaviour. They live alongside unit tests in `_integration_test.go` files or use a build tag.

**Handler tests** exercise the full HTTP handler including middleware (CSRF, auth, rate limiting) using `httptest.NewRecorder()` and a real router.

### Coverage

The project targets ≥ 90% backend coverage. The CI gate is `BACKEND_COVERAGE_MIN=90.0` in the Makefile. Check your coverage before opening a PR:

```bash
make test-cover
```

---

## Frontend Tests

```bash
# Run all frontend tests
make test-front

# With coverage
make test-cover-front
```

Frontend tests use [Vitest](https://vitest.dev/) with React's SSR renderer (`react-dom/server`) rather than a full browser. This keeps tests fast but means DOM interactions are limited to what the SSR output contains.

### useState Mock Convention

**Important:** The frontend test harness uses an index-based `useState` mock. The mock returns state in the order that `useState` is called during a component's first render. If you add a new `useState` call to an existing component that already has tests, **append it at the end** of the component's hook list. Inserting in the middle shifts the indices and breaks existing tests that pin specific state positions (like `LoginPage`).

```tsx
// Do this — add new state after existing hooks
function MyComponent() {
  const [existing1, setExisting1] = useState(false); // index 0
  const [existing2, setExisting2] = useState('');    // index 1
  const [newThing, setNewThing] = useState(null);    // index 2 — appended
}
```

### Mocking Strategy

The frontend tests mock at module boundaries, not at implementation details:

```typescript
vi.mock('../api/auth', () => ({
  login: vi.fn(),
  logout: vi.fn(),
}))
```

Avoid mocking internal component methods or state setters directly — mock the module boundary that the component depends on.

---

## Adding a New Setting

System settings are stored in the `system_settings` table and cached in-process. To add a new one:

1. **Create a migration** in `backend/migrations/`:
   ```sql
   -- 000031_my_new_setting.up.sql
   INSERT INTO system_settings (key, value) VALUES ('my_new_setting', 'default_value');
   ```
   ```sql
   -- 000031_my_new_setting.down.sql
   DELETE FROM system_settings WHERE key = 'my_new_setting';
   ```

2. **Read the value** in backend code:
   ```go
   value := s.settings.GetString(ctx, "my_new_setting", "default_value")
   ```
   Use `GetBool`, `GetInt`, or `GetString` as appropriate.

3. **Allowlist the key** in the settings handler so it can be updated via the API. The handler validates keys against a known set and rejects unknowns with `unknown_setting`.

4. **Document it** in [docs/settings.md](settings.md).

---

## Adding a New API Endpoint

1. **Add the handler** in the appropriate `internal/<domain>/handler.go`.
2. **Wire the route** in `backend/cmd/server/main.go`. Choose the right middleware group:
   - Public (no auth): directly on `r`
   - Authenticated: inside the `r.Group` after `authmw.Authenticate`
   - Permission-gated: wrap with `r.With(authmw.RequirePermission("resource", "action"))`
   - Admin-only: `r.With(authmw.RequireRole("admin"))`
   - Mutation requiring step-up MFA: add `stepUpMFA` to the middleware chain
3. **Write an audit log entry** for any state-mutating action.
4. **Add tests**: at minimum a unit test for the handler and a happy-path integration test.
5. **Document the endpoint** in [docs/api.md](api.md).

---

## Key Conventions

### Error Responses

Return a JSON `{"error": "<code>"}` body. Use consistent, lowercase, snake-case error codes (e.g., `invalid_credentials`, not `Invalid Credentials` or `INVALID_CREDENTIALS`). HTTP status codes follow REST semantics: `400` for client errors, `401` for unauthenticated, `403` for unauthorized, `429` for rate limits.

### Audit Logging

Every handler that mutates state should call `h.logAudit(...)`. The `metadata` JSONB field should contain enough context to reconstruct what changed and why (e.g., `{ "from": "user", "to": "admin", "outcome": "success" }`).

### Context Propagation

Pass `r.Context()` to all service and repository calls. Do not use `context.Background()` inside handlers — it bypasses request cancellation and timeout propagation.

### Structured Logging

Use `slog` at the appropriate level:
- `slog.Info` for lifecycle events (startup, shutdown, migrations)
- `slog.Warn` for anomalies that do not require immediate action (invalid config values, skipped GeoIP lookups)
- `slog.Error` for failures that require investigation

Include relevant key-value pairs rather than embedding them in the message string:
```go
slog.Warn("Login risk assessment", "user_id", u.ID, "score", riskScore, "type", anomalyType)
```

---

## Production Deployment

See [configuration reference](configuration.md) for all environment variables.

### Using the prod Docker Compose

```bash
# 1. Generate secrets
cd scripts && ./generate-secrets.sh

# 2. Place TLS certs
cp your-cert.crt infra/certs/server.crt
cp your-key.key  infra/certs/server.key

# 3. Generate a persistent JWT key
make jwt-key   # → secrets/jwt_primary.pem

# 4. Set the admin password hash in infra/.env.admin
htpasswd -bnBC 12 "" 'YourAdminPassword' | tr -d ':\n'
# Paste the output as INITIAL_ADMIN_PASSWORD_HASH in infra/.env.admin

# 5. Start
make up-prod
```

The production compose adds nginx as a reverse proxy with TLS termination. The backend and frontend containers are on an internal bridge network (`192.168.200.0/28`) and are not directly reachable from the host.

### Key Production Differences

| Setting | Development | Production |
|---|---|---|
| TLS | Off | nginx terminates TLS |
| `COOKIES_SECURE` | `false` | `true` |
| JWT key | Ephemeral (restarts invalidate sessions) | Persisted PEM file |
| Backend port | Exposed on host | Internal only |
| Registration | Off (change in .env if needed) | Off |
| Trusted proxies | None | nginx container IP (`192.168.200.2/32`) |
