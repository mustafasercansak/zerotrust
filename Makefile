.PHONY: help secrets certs jwt-key up up-prod down down-v dev build test lint clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

secrets: ## Generate secrets (infra/.env, infra/.env.admin, EC key pair)
	cd scripts && ./generate-secrets.sh

certs: ## Generate a self-signed TLS certificate (for local HTTPS testing)
	bash infra/scripts/gen-selfsigned-cert.sh

jwt-key: ## Generate a persistent EC P-256 JWT signing key
	bash infra/scripts/gen-jwt-key.sh

up: ## Start in development mode (HTTP, ports exposed)
	cd infra && sudo docker compose up --build

up-prod: certs ## Start in production mode (HTTPS via nginx, no exposed backend/frontend ports)
	cd infra && sudo docker compose -f docker-compose.yml -f docker-compose.prod.yml up --build -d

down: ## Stop all services
	cd infra && sudo docker compose down

down-v: ## Stop and delete volumes (resets database)
	cd infra && sudo docker compose down -v

dev: ## Start in development mode with hot reload
	cd infra && sudo docker compose -f docker-compose.yml -f docker-compose.dev.yml watch

build: ## Build images only
	cd infra && sudo docker compose build

test: ## Run backend tests
	cd backend && go test ./...

lint: ## Run go vet + frontend tsc
	cd backend && go vet ./...
	cd frontend && npx tsc --noEmit

clean: ## Remove Docker images and volumes
	cd infra && sudo docker compose down -v --rmi local
