# Renovi 2.0 — Makefile raiz do monorepo.
# Rode `make` ou `make help` para ver os alvos disponíveis.

API_DIR   := apps/api
WEB_DIR   := apps/web
COMPOSE   := docker compose -f deploy/docker-compose.yml

# Versões fixadas das ferramentas de geração (reprodutibilidade).
# Obs.: sqlc >= v1.31 exige Go >= 1.26; o toolchain é baixado automaticamente
# (GOTOOLCHAIN=auto) na primeira execução.
SQLC_VERSION ?= v1.31.1
OAPI_VERSION ?= v2.7.2

.DEFAULT_GOAL := help

.PHONY: help
help: ## Mostra esta ajuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ---------------------------------------------------------------------------
# Geração de código (API-first). Fonte da verdade: packages/contracts/openapi.yaml
# ---------------------------------------------------------------------------
.PHONY: generate
generate: generate-sqlc generate-openapi ## Regenera código Go (sqlc + oapi-codegen)

.PHONY: generate-sqlc
generate-sqlc: ## Gera repositórios tipados a partir do SQL (sqlc)
	cd $(API_DIR) && go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

.PHONY: generate-openapi
generate-openapi: ## Gera tipos Go a partir do OpenAPI (oapi-codegen)
	cd $(API_DIR) && go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_VERSION) \
	  -config oapi-codegen.yaml ../../packages/contracts/openapi.yaml

.PHONY: generate-check
generate-check: generate ## Falha se a geração produzir diff não commitado (usado no CI)
	@git diff --exit-code -- '$(API_DIR)/internal/db/gen' '$(API_DIR)/internal/http/api' \
	  || (echo "ERRO: código gerado desatualizado. Rode 'make generate' e commite." && exit 1)

# ---------------------------------------------------------------------------
# Qualidade
# ---------------------------------------------------------------------------
.PHONY: fmt
fmt: ## Formata o código Go
	cd $(API_DIR) && gofmt -w .

.PHONY: lint
lint: ## Checa formatação e roda go vet
	@echo ">> gofmt"
	@test -z "$$(cd $(API_DIR) && gofmt -l .)" || (echo "ERRO: arquivos não formatados. Rode 'make fmt'." && cd $(API_DIR) && gofmt -l . && exit 1)
	@echo ">> go vet"
	cd $(API_DIR) && go vet ./...

.PHONY: test
test: ## Testes unitários (rápidos, sem Docker)
	cd $(API_DIR) && go test ./...

.PHONY: test-integration
test-integration: ## Testes de integração (testcontainers; exige Docker)
	cd $(API_DIR) && go test -tags=integration ./...

.PHONY: tidy
tidy: ## Atualiza go.mod/go.sum
	cd $(API_DIR) && go mod tidy

.PHONY: build
build: ## Compila os binários Go
	cd $(API_DIR) && go build ./...

# ---------------------------------------------------------------------------
# Banco (migrations)
# ---------------------------------------------------------------------------
.PHONY: migrate-up
migrate-up: ## Aplica as migrations pendentes no renovi_care
	cd $(API_DIR) && go run ./cmd/migrate up

.PHONY: migrate-down
migrate-down: ## Reverte 1 migration
	cd $(API_DIR) && go run ./cmd/migrate down 1

.PHONY: seed
seed: ## Aplica os templates de linha de cuidado (STUB na fundação)
	cd $(API_DIR) && go run ./cmd/seed

# ---------------------------------------------------------------------------
# Docker Compose (dev local: Postgres + mocks dos bancos externos)
# ---------------------------------------------------------------------------
.PHONY: up
up: ## Sobe a infraestrutura local (Postgres + mocks)
	$(COMPOSE) up -d

.PHONY: down
down: ## Derruba a infraestrutura local
	$(COMPOSE) down

.PHONY: logs
logs: ## Mostra logs dos containers
	$(COMPOSE) logs -f

# ---------------------------------------------------------------------------
# Front-end (web)
# ---------------------------------------------------------------------------
.PHONY: web-install
web-install: ## Instala dependências do front
	cd $(WEB_DIR) && npm install

.PHONY: web-dev
web-dev: ## Sobe o front em modo dev
	cd $(WEB_DIR) && npm run dev

.PHONY: web-test
web-test: ## Roda os testes do front
	cd $(WEB_DIR) && npm test

.PHONY: web-build
web-build: ## Build de produção do front
	cd $(WEB_DIR) && npm run build

# ---------------------------------------------------------------------------
# Agregados
# ---------------------------------------------------------------------------
.PHONY: ci
ci: lint generate-check test build ## Espelha o pipeline de CI localmente
