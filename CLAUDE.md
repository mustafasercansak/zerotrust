# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

This repository maintains a comprehensive `AGENTS.md` at the repo root that is the source of truth for AI coding agents — project overview, tech stack, repository layout, build/test/lint commands, testing conventions (backend and frontend), code style, security considerations, and the documentation map. **Read `AGENTS.md` first and follow it exactly.**

A few pointers to help you navigate faster:

- Full endpoint list, env vars, and session/MFA/WebAuthn/OIDC/DPoP flow details live in `README.md` and `docs/` (`architecture.md`, `api.md`, `configuration.md`, `security-model.md`, `settings.md`, `development.md`) — check these before assuming behavior.
- `make help` lists every Makefile target; `AGENTS.md` already documents the ones you'll use most (`make test`, `make lint`, `make test-cover`, `make up`/`make dev`).
- This is a security-sensitive codebase (auth/identity provider). Read the "Security Considerations" section of `AGENTS.md` before touching anything in `backend/internal/auth`, `mfa`, `oidc`, `webauthn`, `session`, or `pkg/middleware`.
