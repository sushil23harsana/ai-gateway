# AI Gateway — developer tasks.
# On Windows without `make`, run the underlying commands directly (see README).

.PHONY: help dev up down logs build run migrate test tidy fmt vet gen-token

help: ## Show this help.
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

dev: up ## Bring up the full stack (redis + postgres + migrate + gateway).

up: ## Start all services in the background.
	docker compose up -d --build

down: ## Stop and remove all services.
	docker compose down

logs: ## Tail gateway logs.
	docker compose logs -f gateway

build: ## Build the gateway and migrate binaries locally.
	go build -o bin/gateway ./cmd/gateway
	go build -o bin/migrate ./cmd/migrate

run: ## Run the gateway locally (needs local redis/postgres + .env).
	go run ./cmd/gateway

migrate: ## Apply pending DB migrations against DATABASE_URL.
	go run ./cmd/migrate

test: ## Run unit tests.
	go test ./...

tidy: ## Sync go.mod/go.sum.
	go mod tidy

fmt: ## Format Go code.
	go fmt ./...

vet: ## Static checks.
	go vet ./...

gen-token: ## Print a strong random ADMIN_TOKEN to paste into .env.
	@openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | xxd -p -c 32
