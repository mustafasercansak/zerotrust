# Contributing to ZeroTrust

Thank you for your interest in contributing.

## Before You Start

- **Security issues** — follow [SECURITY.md](SECURITY.md) instead of opening an issue.
- **Large changes** — open an issue first to discuss the approach before writing code.
- **Small fixes** (typos, docs, obvious bugs) — a PR is fine without a prior issue.

## Development Setup

```bash
# Clone and generate secrets
git clone https://github.com/your-org/zerotrust.git
cd zerotrust/scripts && ./generate-secrets.sh

# Start with hot reload
cd ../infra
sudo docker compose -f docker-compose.yml -f docker-compose.dev.yml watch
```

## Making Changes

1. Fork the repository and create a branch from `main`.
2. Keep commits focused — one logical change per commit.
3. Run checks before pushing:

```bash
# Backend
cd backend && go build ./... && go vet ./... && go test ./...

# Frontend
cd frontend && npx tsc --noEmit
```

4. Open a pull request against `main`. Fill in the PR template.

## Code Guidelines

- **No secrets in commits** — the `.gitignore` covers `infra/.env` and key files, but double-check.
- **SQL migrations** — every schema change needs a numbered migration file; migrations must be idempotent on replay.
- **Error messages** — do not leak internal state to HTTP responses; use opaque error codes.
- **No new dependencies** without a discussion in the PR — the dependency surface is a security concern.

## Commit Messages

```
<type>: <short summary in imperative mood>

Optional longer explanation if the why is non-obvious.
```

Types: `feat`, `fix`, `security`, `refactor`, `docs`, `test`, `chore`.

## License

By submitting a pull request you agree that your contribution will be licensed under the [GNU Affero General Public License v3.0](LICENSE).
