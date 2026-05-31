.PHONY: help secrets certs jwt-key up up-prod down down-v dev build test test-cover test-front test-cover-front test-cover-all test-coverage-all coverage-summary coverage-all lint clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

secrets: ## Generate secrets (infra/.env, infra/.env.admin, EC key pair)
	cd scripts && ./generate-secrets.sh

certs: ## Generate a self-signed TLS certificate (for local HTTPS testing)
	bash infra/scripts/gen-selfsigned-cert.sh

jwt-key: ## Generate a persistent EC P-256 JWT signing key
	bash infra/scripts/gen-jwt-key.sh

up: ## Start in development mode (HTTP, ports exposed)
	cd infra && sudo docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build

up-prod: certs ## Start in production mode (HTTPS via nginx, no exposed backend/frontend ports)
	cd infra && sudo docker compose -f docker-compose.yml -f docker-compose.prod.yml up --build -d

down: ## Stop all services (works after make up, make dev, or make up-prod)
	cd infra && sudo docker compose -f docker-compose.yml -f docker-compose.dev.yml -f docker-compose.prod.yml down --remove-orphans

down-v: ## Stop and delete volumes (resets database)
	cd infra && sudo docker compose -f docker-compose.yml -f docker-compose.dev.yml -f docker-compose.prod.yml down -v --remove-orphans

dev: ## Start in development mode with hot reload
	cd infra && sudo docker compose -f docker-compose.yml -f docker-compose.dev.yml watch

build: ## Build images only
	cd infra && sudo docker compose build

test: ## Run backend tests (set TEST_DATABASE_URL to include DB integration tests)
	cd backend && go test -p 1 ./...

test-cover: ## Run backend tests and display coverage (set TEST_DATABASE_URL to include DB integration tests)
	cd backend && \
	go test -p 1 -coverprofile=coverage.out ./... && \
	go tool cover -func=coverage.out

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

clean: ## Remove Docker images and volumes
	cd infra && sudo docker compose -f docker-compose.yml -f docker-compose.dev.yml -f docker-compose.prod.yml down -v --rmi local --remove-orphans
