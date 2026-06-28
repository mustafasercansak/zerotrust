.PHONY: help secrets certs jwt-key up up-prod down down-v dev build test test-local test-cover test-front test-cover-front test-cover-all test-coverage-all coverage-summary coverage-all lint govulncheck clean screenshots

TEST_DB_DOCKER_IMAGE ?= postgres:16-alpine
TEST_DB_CONTAINER ?= zerotrust-test-db
TEST_DB_PORT ?= 55432
TEST_DB_NAME ?= zerotrust_test
TEST_DB_USER ?= postgres
TEST_DB_PASSWORD ?= postgres
TEST_REDIS_DOCKER_IMAGE ?= redis:7-alpine
TEST_REDIS_CONTAINER ?= zerotrust-test-redis
TEST_REDIS_PORT ?= 56379
BACKEND_COVERAGE_MIN ?= 90.0
DOCKER_COMPOSE ?= docker compose
SUDO ?=

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

secrets: ## Generate secrets (infra/.env, infra/.env.admin, EC key pair)
	cd scripts && ./generate-secrets.sh

certs: ## Generate a self-signed TLS certificate (for local HTTPS testing)
	bash infra/scripts/gen-selfsigned-cert.sh

jwt-key: ## Generate a persistent Ed25519 JWT signing key
	bash infra/scripts/gen-jwt-key.sh

up: ## Start in development mode (HTTP, ports exposed)
	cd infra && $(SUDO) $(DOCKER_COMPOSE) -f docker-compose.yml -f docker-compose.dev.yml up --build

up-prod: certs ## Start in production mode (HTTPS via nginx, no exposed backend/frontend ports)
	cd infra && $(SUDO) $(DOCKER_COMPOSE) -f docker-compose.yml -f docker-compose.prod.yml up --build -d

down: ## Stop all services (works after make up, make dev, or make up-prod)
	cd infra && $(SUDO) $(DOCKER_COMPOSE) -f docker-compose.yml -f docker-compose.dev.yml -f docker-compose.prod.yml down --remove-orphans

down-v: ## Stop and delete volumes (resets database)
	cd infra && $(SUDO) $(DOCKER_COMPOSE) -f docker-compose.yml -f docker-compose.dev.yml -f docker-compose.prod.yml down -v --remove-orphans

dev: ## Start in development mode with hot reload
	cd infra && $(SUDO) $(DOCKER_COMPOSE) -f docker-compose.yml -f docker-compose.dev.yml watch

build: ## Build images only
	cd infra && $(SUDO) $(DOCKER_COMPOSE) build

test: ## Run backend tests; starts disposable test services when no test URL is set
	@if [ -n "$$TEST_DATABASE_URL" ]; then \
		echo "Using TEST_DATABASE_URL from environment."; \
		cd backend && go test -count=1 -p 1 ./...; \
	else \
		$(MAKE) test-local; \
	fi

test-local: ## Run all backend tests with disposable PostgreSQL and Redis containers
	@set -e; \
	command -v docker >/dev/null 2>&1 || { echo "Docker is required for make test-local."; exit 1; }; \
	docker rm -f $(TEST_DB_CONTAINER) $(TEST_REDIS_CONTAINER) >/dev/null 2>&1 || true; \
	trap 'docker rm -f $(TEST_DB_CONTAINER) $(TEST_REDIS_CONTAINER) >/dev/null 2>&1 || true' EXIT; \
	db_port=$(TEST_DB_PORT); \
	db_started=0; \
	while [ $$db_port -lt $$(( $(TEST_DB_PORT) + 20 )) ]; do \
		if docker run -d --name $(TEST_DB_CONTAINER) \
			-e POSTGRES_USER=$(TEST_DB_USER) \
			-e POSTGRES_PASSWORD=$(TEST_DB_PASSWORD) \
			-e POSTGRES_DB=$(TEST_DB_NAME) \
			-p $$db_port:5432 \
			$(TEST_DB_DOCKER_IMAGE) >/dev/null 2>&1; then \
			db_started=1; \
			break; \
		fi; \
		db_port=$$((db_port + 1)); \
	done; \
	if [ $$db_started -ne 1 ]; then \
		echo "Could not start temporary PostgreSQL on ports $(TEST_DB_PORT)-$$(( $(TEST_DB_PORT) + 19 ))."; \
		exit 1; \
	fi; \
	redis_port=$(TEST_REDIS_PORT); \
	redis_started=0; \
	while [ $$redis_port -lt $$(( $(TEST_REDIS_PORT) + 20 )) ]; do \
		if docker run -d --name $(TEST_REDIS_CONTAINER) \
			-p $$redis_port:6379 \
			$(TEST_REDIS_DOCKER_IMAGE) >/dev/null 2>&1; then \
			redis_started=1; \
			break; \
		fi; \
		redis_port=$$((redis_port + 1)); \
	done; \
	if [ $$redis_started -ne 1 ]; then \
		echo "Could not start temporary Redis on ports $(TEST_REDIS_PORT)-$$(( $(TEST_REDIS_PORT) + 19 ))."; \
		exit 1; \
	fi; \
	echo "Waiting for disposable test services..."; \
	i=0; \
	until docker exec $(TEST_DB_CONTAINER) pg_isready -U $(TEST_DB_USER) -d $(TEST_DB_NAME) >/dev/null 2>&1; do \
		i=$$((i + 1)); \
		if [ $$i -ge 40 ]; then echo "Temporary PostgreSQL did not become ready."; exit 1; fi; \
		sleep 1; \
	done; \
	i=0; \
	until docker exec $(TEST_REDIS_CONTAINER) redis-cli ping >/dev/null 2>&1; do \
		i=$$((i + 1)); \
		if [ $$i -ge 40 ]; then echo "Temporary Redis did not become ready."; exit 1; fi; \
		sleep 1; \
	done; \
	echo "Running backend tests with PostgreSQL on $$db_port and Redis on $$redis_port."; \
	cd backend && \
		TEST_DATABASE_URL="postgres://$(TEST_DB_USER):$(TEST_DB_PASSWORD)@127.0.0.1:$$db_port/$(TEST_DB_NAME)?sslmode=disable" \
		TEST_REDIS_ADDR="127.0.0.1:$$redis_port" \
		TEST_REDIS_PASSWORD="" \
		go test -count=1 -p 1 ./...

test-cover: ## Run backend tests and display coverage (set TEST_DATABASE_URL to include DB integration tests)
	@set -e; \
	if [ -n "$$TEST_DATABASE_URL" ]; then \
		echo "Using TEST_DATABASE_URL from environment."; \
		cd backend && go test -p 1 -coverprofile=coverage.out ./... && go tool cover -func=coverage.out; \
		backend_total=$$(go tool cover -func=coverage.out | awk '/^total:/{v=$$3; gsub("%", "", v); print v}'); \
		if awk "BEGIN {exit !($$backend_total >= $(BACKEND_COVERAGE_MIN))}"; then \
			echo "Backend coverage $$backend_total% meets minimum $(BACKEND_COVERAGE_MIN)%."; \
		else \
			echo "Backend coverage $$backend_total% is below minimum $(BACKEND_COVERAGE_MIN)%."; \
			exit 1; \
		fi; \
	elif command -v docker >/dev/null 2>&1; then \
		echo "TEST_DATABASE_URL is not set; starting temporary Postgres for integration coverage."; \
		docker rm -f $(TEST_DB_CONTAINER) >/dev/null 2>&1 || true; \
		db_port=$(TEST_DB_PORT); \
		started=0; \
		while [ $$db_port -lt $$(( $(TEST_DB_PORT) + 20 )) ]; do \
			if docker run -d --name $(TEST_DB_CONTAINER) \
				-e POSTGRES_USER=$(TEST_DB_USER) \
				-e POSTGRES_PASSWORD=$(TEST_DB_PASSWORD) \
				-e POSTGRES_DB=$(TEST_DB_NAME) \
				-p $$db_port:5432 \
				$(TEST_DB_DOCKER_IMAGE) >/dev/null 2>&1; then \
				started=1; \
				break; \
			fi; \
			db_port=$$((db_port + 1)); \
		done; \
		if [ $$started -ne 1 ]; then \
			echo "Could not start temporary Postgres on ports $(TEST_DB_PORT)-$$(( $(TEST_DB_PORT) + 19 ))."; \
			exit 1; \
		fi; \
		echo "Temporary Postgres is running on host port $$db_port."; \
		trap 'docker rm -f $(TEST_DB_CONTAINER) >/dev/null 2>&1 || true' EXIT; \
		i=0; \
		until docker exec $(TEST_DB_CONTAINER) pg_isready -U $(TEST_DB_USER) -d $(TEST_DB_NAME) >/dev/null 2>&1; do \
			i=$$((i + 1)); \
			if [ $$i -ge 40 ]; then \
				echo "Temporary Postgres did not become ready in time."; \
				exit 1; \
			fi; \
			sleep 1; \
		done; \
		TEST_DATABASE_URL="postgres://$(TEST_DB_USER):$(TEST_DB_PASSWORD)@127.0.0.1:$$db_port/$(TEST_DB_NAME)?sslmode=disable"; \
		export TEST_DATABASE_URL; \
		cd backend && go test -p 1 -coverprofile=coverage.out ./... && go tool cover -func=coverage.out; \
		backend_total=$$(go tool cover -func=coverage.out | awk '/^total:/{v=$$3; gsub("%", "", v); print v}'); \
		if awk "BEGIN {exit !($$backend_total >= $(BACKEND_COVERAGE_MIN))}"; then \
			echo "Backend coverage $$backend_total% meets minimum $(BACKEND_COVERAGE_MIN)%."; \
		else \
			echo "Backend coverage $$backend_total% is below minimum $(BACKEND_COVERAGE_MIN)%."; \
			exit 1; \
		fi; \
	else \
		echo "TEST_DATABASE_URL is not set and docker is unavailable; DB integration tests will be skipped."; \
		exit 1; \
	fi

test-front: ## Run frontend tests
	cd frontend && npm run test

test-cover-front: ## Run frontend tests and display coverage
	cd frontend && npm run test:cover

test-cover-all: ## Run backend + frontend coverage and print summary
	$(MAKE) test-cover
	$(MAKE) test-cover-front
	$(MAKE) coverage-summary

coverage-summary: ## Print concise coverage summary for sharing
	@echo ""
	@echo "==== Coverage Summary ===="
	@echo "Backend total: $$(cd backend && go tool cover -func=coverage.out | awk '/^total:/{print $$3}')"
	@echo "Backend lowest-covered functions:"
	@cd backend && go tool cover -func=coverage.out | awk '$$1 !~ /^total:/ && $$NF ~ /%/ {v=$$NF; gsub("%", "", v); printf "  %6.1f%%  %s\n", v, $$1}' | sort -n | head -n 12
	@echo "Frontend summary: see the 'All files' row in vitest output above."

coverage-all: ## Alias for test-cover-all
	$(MAKE) test-cover-all

test-coverage-all: ## Alias for test-cover-all
	$(MAKE) test-cover-all

lint: ## Run go vet + frontend tsc
	cd backend && go vet ./...
	cd frontend && npx tsc --noEmit

govulncheck: ## Run Go vulnerability scan with the latest govulncheck
	cd backend && go run golang.org/x/vuln/cmd/govulncheck@latest ./...

clean: ## Remove Docker images and volumes
	cd infra && $(SUDO) $(DOCKER_COMPOSE) -f docker-compose.yml -f docker-compose.dev.yml -f docker-compose.prod.yml down -v --rmi local --remove-orphans

screenshots: ## Take UI screenshots and regenerate docs/index.md  (ADMIN_PASSWORD=<pass> make screenshots)
	@command -v npx >/dev/null 2>&1 || (echo '❌  npx not found; install Node.js first' && exit 1)
	@node -e "require('playwright')" 2>/dev/null || (echo '📦  Installing playwright...' && cd frontend && npm install --save-dev playwright && npx playwright install chromium)
	node scripts/screenshots.js --url $${SCREENSHOT_URL:-http://localhost:3000} --email $${ADMIN_EMAIL:-admin@company.com} --password $${ADMIN_PASSWORD}
