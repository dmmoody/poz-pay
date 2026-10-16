COMPOSE ?= docker compose
MIGRATE_SERVICE ?= migrate
POSTGRES_SERVICE ?= postgres
API_SERVICE ?= api

.PHONY: help up down destroy logs ps psql db-up migrate migrate-down migrate-version build run test fmt seed tidy docker-build

help: ## Show targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

up: ## Build/start full stack, migrate, and seed
	$(COMPOSE) up -d --build $(POSTGRES_SERVICE) $(API_SERVICE)
	@$(MAKE) migrate
	@$(MAKE) seed

down: ## Stop and remove stack (keep DB volume)
	$(COMPOSE) down

destroy: ## Stop stack and delete DB volume
	$(COMPOSE) down -v

db-up: ## Start Postgres only (for migrate / make run)
	$(COMPOSE) up -d $(POSTGRES_SERVICE)

docker-build: ## Build API image
	$(COMPOSE) build $(API_SERVICE)

logs: ## Tail compose logs
	$(COMPOSE) logs -f --tail=200

ps: ## Compose process status
	$(COMPOSE) ps

psql: ## Open psql in Postgres container
	$(COMPOSE) exec $(POSTGRES_SERVICE) psql -U $${POSTGRES_USER:-pozpay} -d $${POSTGRES_DB:-pozpay}

migrate: db-up ## Apply all migrations
	$(COMPOSE) --profile tools run --rm $(MIGRATE_SERVICE) up

migrate-down: ## Roll back one migration
	$(COMPOSE) --profile tools run --rm $(MIGRATE_SERVICE) down 1

migrate-version: ## Print current migration version
	$(COMPOSE) --profile tools run --rm $(MIGRATE_SERVICE) version

seed: ## Apply seed SQL (requires migrations)
	$(COMPOSE) exec -T $(POSTGRES_SERVICE) psql -U $${POSTGRES_USER:-pozpay} -d $${POSTGRES_DB:-pozpay} -v ON_ERROR_STOP=1 -f - < scripts/seed.sql

run: ## Run API locally against published Postgres
	DATABASE_URL=$${DATABASE_URL:-postgres://pozpay:pozpay@localhost:5432/pozpay?sslmode=disable} \
	API_ADDR=$${API_ADDR:-:8080} \
	go run ./cmd/api

build: ## Build API binary to bin/api
	mkdir -p bin
	go build -o bin/api ./cmd/api

test: ## Run Go tests
	go test ./...

fmt: ## Format Go sources
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

tidy: ## go mod tidy
	go mod tidy
