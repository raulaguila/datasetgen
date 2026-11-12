# =========================
# Configs
# =========================

SHELL := /bin/bash
.DEFAULT_GOAL := help

# Project
PROJECT_NAME     := dataset-api
PROJECT_BIN_PATH := bin
PROJECT_VERSION  := $(shell cat config/version.txt | tr -d "[:space:]")
PROJECT_ZIP_NAME := $(PROJECT_NAME)_$(PROJECT_VERSION).zip
MIGRATIONS_DIR   := deploy/db/migrations

# Docker
DOCKER          := docker
DOCKER_COMPOSE  := docker compose
DOCKER_BASE     ?= $(or ${base}, source)
DOCKER_SERVICE  ?= $(or ${service}, backend)
COMPOSE_SERVICE := $(DOCKER_COMPOSE) --env-file config/.env -f deploy/docker/services.compose.yml
COMPOSE_COMMAND := BASE=${DOCKER_BASE} $(DOCKER_COMPOSE) --env-file config/.env -f deploy/docker/compose.yml

# Go build
GO          := GOWORK=off go
GOOS        ?= linux
GOARCH      ?= amd64
BUILD_FLAGS := -ldflags "-w -s"
GOBUILD     := CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build $(BUILD_FLAGS)

# Colors
GREEN   := $(shell printf '\033[1;32m')
YELLOW  := $(shell printf '\033[1;33m')
BLUE    := $(shell printf '\033[1;34m')
CYAN    := $(shell printf '\033[1;36m')
MAGENTA := $(shell printf '\033[1;35m')
RED     := $(shell printf '\033[1;31m')
RESET   := $(shell printf '\033[0m')

GOPATH ?= $(shell go env GOPATH 2>/dev/null)
ifneq ($(GOPATH),)
export PATH := $(GOPATH)/bin:$(PATH)
endif

GOFUMPT_BIN     := $(GOPATH)/bin/gofumpt
GOIMPORTS_BIN   := $(GOPATH)/bin/goimports
GOVULNCHECK_BIN := $(GOPATH)/bin/govulncheck
LINTER_BIN      := $(GOPATH)/bin/golangci-lint
MIGRATE_BIN     := $(GOPATH)/bin/migrate
AIR_BIN         := $(GOPATH)/bin/air

# Pinned tooling (install-tools)
GOFUMPT_VERSION     := v0.9.2
GOIMPORTS_VERSION   := v0.41.0
GOVULNCHECK_VERSION := v1.9.0
LINTER_VERSION      := v2.15.0
MIGRATE_VERSION     := v4.18.3
AIR_VERSION         := v1.62.0

# =========================
# Helpers
# =========================
define clean_dangling_images
	@echo "$(YELLOW)🧹 Cleaning up dangling Docker images...$(RESET)"
	@images="$$( $(DOCKER) images -f dangling=true -q )"; \
	if test -n "$$images"; then \
		$(DOCKER) rmi $$images > /dev/null; \
	fi
endef

define require_env_file
	@test -f config/.env || (echo "$(RED)config/.env not found. Run make init first.$(RESET)" && exit 1)
endef

# Source config/.env (POSTGRES_USER, POSTGRES_PASS, POSTGRES_PORT, POSTGRES_BASE).
# Wait until localhost:POSTGRES_PORT accepts TCP, then run migrate.
define run_migrate
	$(call require_env_file)
	@set -a && . ./config/.env && set +a; \
	i=0; \
	while [ $$i -lt 30 ]; do \
		if (echo > /dev/tcp/127.0.0.1/$${POSTGRES_PORT}) >/dev/null 2>&1; then break; fi; \
		i=$$((i + 1)); \
		sleep 1; \
	done; \
	if [ $$i -ge 30 ]; then \
		echo "$(RED)Postgres not reachable on port $${POSTGRES_PORT}. Is compose up?$(RESET)"; \
		exit 1; \
	fi; \
	MIGRATE_URL="postgres://$${POSTGRES_USER}@localhost:$${POSTGRES_PORT}/$${POSTGRES_BASE}?sslmode=disable"; \
	PGPASSWORD="$${POSTGRES_PASS}" $(MIGRATE_BIN) -path $(MIGRATIONS_DIR) -database "$${MIGRATE_URL}" $(1)
endef

# $(1) name  $(2) binary  $(3) version args (empty = do not run)
# $(4) Dev   $(5) Deploy  $(6) install hint
define check_tool
	NAME="$(1)"; \
	BIN="$(2)"; \
	VER_ARGS="$(3)"; \
	if command -v "$$BIN" >/dev/null 2>&1 || [ -x "$$BIN" ]; then \
		if [ -n "$$VER_ARGS" ]; then \
			VERSION=$$($$BIN $$VER_ARGS 2>&1 | tr -d '\r' | grep -v '^$$' | head -n 1); \
			VERSION=$${VERSION:-ok}; \
		else \
			VERSION="installed"; \
		fi; \
		printf "✅ %-20s %-10s %-12s %-30s\n" "$$NAME" "$(4)" "$(5)" "$$VERSION"; \
	else \
		printf "$(RED)❌ %-20s %-10s %-12s %-30s$(RESET)\n" "$$NAME" "$(4)" "$(5)" "Install: $(6)"; \
		MISSING=1; \
	fi
endef

.PHONY: all help check-system install-tools rename
all: help

rename: ## Rename the project: make rename MODULE=github.com/acme/app NAME=app
	@../scripts/rename.sh "$(MODULE)" "$(NAME)"

module: ## Copy the products example into a new module: make module NAME=invoice [PLURAL=invoices]
	@go run ./tools/newmodule -name "$(NAME)" -plural "$(PLURAL)"
	@gofmt -w internal tests
	@$(MAKE) --no-print-directory swagger

help: ## Display available commands and their descriptions
	@echo "$(CYAN)Usage:$(RESET)"
	@echo "  make [COMMAND]"
	@echo
	@echo "$(CYAN)Example:$(RESET)"
	@echo "  make build"
	@echo
	@echo "$(CYAN)Commands:$(RESET)"
	@echo
	@grep -h -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "$(CYAN)%-30s$(RESET) %s\n", $$1, $$2}'

check-system: ## Check required tools
	@MISSING=0; \
	printf "  ========================================\n"; \
	printf "   Environment Tooling Check\n"; \
	printf "  ========================================\n\n"; \
	printf "$(CYAN)%-22s %-8s %-10s %-40s$(RESET)\n" "  Tool" "Dev" "Deploy" "Version"; \
	printf "$(BLUE)%-22s %-8s %-10s %-40s$(RESET)\n" "  -------" "-------" "-------" "-------"; \
	\
	$(call check_tool,Go,go,version,✅,✅,Visit: https://go.dev/doc/install); \
	$(call check_tool,Docker,docker,--version,❌,✅,Visit: https://docs.docker.com/engine/install/ubuntu/); \
	$(call check_tool,Docker Compose,docker,compose version,❌,✅,Visit: https://docs.docker.com/engine/install/ubuntu/); \
	$(call check_tool,Go Format,$(GOFUMPT_BIN),--version,✅,❌,make install-tools); \
	$(call check_tool,Go Imports,$(GOIMPORTS_BIN),,✅,❌,make install-tools); \
	$(call check_tool,Go Vulnerability,$(GOVULNCHECK_BIN),-version,✅,❌,make install-tools); \
	$(call check_tool,GolangCI Lint,$(LINTER_BIN),version,✅,❌,make install-tools); \
	$(call check_tool,Migrate,$(MIGRATE_BIN),-version,✅,❌,make install-tools); \
	$(call check_tool,Air,$(AIR_BIN),-v,✅,❌,make install-tools); \
	$(call check_tool,Zip,zip,-h,✅,❌,sudo apt install zip); \
	\
	if [ $$MISSING -eq 0 ]; then \
		printf "\n✅ All required tools found!\n"; \
		printf "✅ Environment is ready\n\n"; \
	else \
		printf "\nSome tools are missing. Run: make install-tools\n\n"; \
	fi

install-tools: ## Install pinned Go tooling into GOPATH/bin
	@echo "$(BLUE)🔧 Installing pinned Go tools...$(RESET)"
	@$(GO) install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)
	@$(GO) install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)
	@$(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	@$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(LINTER_VERSION)
	@$(GO) install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)
	@$(GO) install github.com/air-verse/air@$(AIR_VERSION)
	@echo "$(GREEN)✅ Tools installed to $(GOPATH)/bin$(RESET)"
	@echo

# =========================
# 1. Environment & Setup
# =========================
.PHONY: clean clean-go-cache clean-go-mod-cache init swagger tidy dev-backend dev-mailer list-updates

clean: ## Clean local build artifacts
	@echo "$(BLUE)🏗️  Cleaning local build artifacts...$(RESET)"
	@rm -rf $(PROJECT_BIN_PATH) cover.out coverage.html $(PROJECT_ZIP_NAME)
	@echo "$(GREEN)✅ Local build artifacts cleaned successfully.$(RESET)"
	@echo

clean-go-cache: ## Clean Go build and test cache
	@echo "$(BLUE)🏗️  Cleaning Go build and test cache...$(RESET)"
	@$(GO) clean -cache -testcache
	@echo "$(GREEN)✅ Go cache cleaned successfully.$(RESET)"
	@echo

clean-go-mod-cache: ## Clean Go module cache
	@echo "$(YELLOW)⚠️  Cleaning Go module cache...$(RESET)"
	@$(GO) clean -modcache
	@echo "$(GREEN)✅ Go module cache cleaned successfully.$(RESET)"
	@echo

init: ## Create environment file if missing
	@echo "$(BLUE)⚙️  Initializing environment setup...$(RESET)"
	@if [ -f config/.env ]; then \
		echo "$(YELLOW)config/.env already exists; leaving it untouched.$(RESET)"; \
	else \
		chmod +x config/env.sh && config/env.sh; \
		if [ -f .env ] && [ ! -f config/.env ]; then mv .env config/.env; fi; \
		echo "$(GREEN)✅ Environment file successfully created and configured!$(RESET)"; \
	fi
	@echo

swagger: ## Regenerate the Swagger docs from the handler annotations
	@$(GO) run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g swagger.go -d internal/adapter/inbound/http/rest -o internal/adapter/inbound/http/rest/docs --parseInternal --outputTypes go
	@echo "$(GREEN)✅ Swagger docs generated.$(RESET)"
	@echo

tidy: ## Clean and tidy dependencies
	@echo "$(YELLOW)🔧 Cleaning and tidying Go dependencies...$(RESET)"
	@$(GO) mod tidy -v
	@echo "$(GREEN)✅ Dependencies tidied successfully.$(RESET)"
	@echo

dev-backend: ## Run backend with Air hot-reload
	@echo "$(BLUE)🔥 Starting backend with hot reload...$(RESET)"
	@$(AIR_BIN) -c air.backend.toml

dev-mailer: ## Run mailer with Air hot-reload
	@echo "$(BLUE)🔥 Starting mailer with hot reload...$(RESET)"
	@DIAGNOSTICS_ADDRESS=$${DIAGNOSTICS_ADDRESS-127.0.0.1:9092} $(AIR_BIN) -c air.mailer.toml

list-updates: ## List available direct dependency updates
	@echo "$(BLUE)📦 Checking for available direct dependency updates...$(RESET)"
	@$(GO) list -u -f '{{if (and (not (or .Main .Indirect)) .Update)}}{{.Path}}: {{.Version}} -> {{.Update.Version}}{{end}}' -m all
	@echo "$(GREEN)✅ Dependency check complete!$(RESET)"
	@echo

# =========================
# 2. Quality & Testing
# =========================
.PHONY: format lint audit test coverage coverage-open benchmark

format: ## Fix code format issues
	@echo "$(BLUE)📝 Formatting code to fix style issues...$(RESET)"
	@$(GOIMPORTS_BIN) -w ./cmd ./internal
	@$(GOFUMPT_BIN) -w -l ./cmd ./internal
	@echo "$(GREEN)✅ Code formatting complete! All issues fixed.$(RESET)"
	@echo

lint: ## Run linter
	@echo "$(BLUE)🔍 Running linter...$(RESET)"
	@$(LINTER_BIN) run ./...
	@echo "$(GREEN)✅  Lint check passed!$(RESET)"
	@echo

audit: ## Conduct quality checks (Vet, Vuln, Lint)
	@echo "$(BLUE)🔎 Running code audit...$(RESET)"
	@$(GO) mod verify
	@$(GO) vet ./...
	@$(GOVULNCHECK_BIN) ./...
	@$(MAKE) lint
	@echo "$(GREEN)✅  Code audit finished!$(RESET)"
	@echo

test: ## Run tests
	@echo "$(BLUE)🔍 Running tests...$(RESET)"
	@$(GO) clean -testcache
	@$(GO) test $$($(GO) list ./... | grep -v /cmd)
	@echo "$(GREEN)✅ Tests completed.$(RESET)"
	@echo

coverage: ## Run tests and generate coverage report
	@echo "$(BLUE)🔍 Running tests with coverage...$(RESET)"
	@set -e; \
	packages="$$( $(GO) list ./... | grep -v /cmd )"; \
	$(GO) clean -testcache; \
	$(GO) test $$packages -coverprofile cover.out; \
	$(GO) tool cover -html=cover.out -o coverage.html; \
	printf "$(GREEN)✅ Coverage generated: $(RESET)"; \
	$(GO) tool cover -func=cover.out | tail -1

coverage-open: coverage ## Run coverage and open the HTML report when supported
	@if command -v open > /dev/null 2>&1; then \
		open ./coverage.html; \
	elif command -v xdg-open > /dev/null 2>&1; then \
		xdg-open ./coverage.html; \
	else \
		echo "$(YELLOW)Open coverage.html manually.$(RESET)"; \
	fi

benchmark: ## Benchmark code performance
	@echo "$(MAGENTA)⚡ Running benchmarks...$(RESET)"
	@$(GO) test ./... -benchmem -bench=. -run=^$$
	@echo "$(GREEN)✅ Benchmark completed!$(RESET)"
	@echo

# =========================
# 3. Build & Artifacts
# =========================
.PHONY: build-admin build-backend build-mailer build zip

build-backend: ## Build backend application from source code
	@echo "$(BLUE)🏗️  Building backend...$(RESET)"
	@rm -f $(PROJECT_BIN_PATH)/backend
	@$(GOBUILD) -o $(PROJECT_BIN_PATH)/backend ./cmd/backend
	@echo "$(GREEN)✅ Build completed successfully!$(RESET)"
	@echo

build-admin: ## Build admin application from source code
	@echo "$(BLUE)🏗️  Building admin...$(RESET)"
	@rm -f $(PROJECT_BIN_PATH)/admin
	@$(GOBUILD) -o $(PROJECT_BIN_PATH)/admin ./cmd/admin
	@echo "$(GREEN)✅ Build completed successfully!$(RESET)"
	@echo

build-mailer: ## Build mailer application from source code
	@echo "$(BLUE)🏗️  Building mailer...$(RESET)"
	@rm -f $(PROJECT_BIN_PATH)/mailer
	@$(GOBUILD) -o $(PROJECT_BIN_PATH)/mailer ./cmd/mailer
	@echo "$(GREEN)✅ Build completed successfully!$(RESET)"
	@echo

build: build-admin build-backend build-mailer ## Build all applications from source code

zip: build ## Build the application and zip it
	@echo "$(BLUE)📦 Zipping application...$(RESET)"
	@rm -f $(PROJECT_ZIP_NAME)
	@files="bin Makefile config/env.sh config/version.txt"; \
	if [ -d deploy ]; then files="$$files deploy"; fi; \
	zip -qr $(PROJECT_ZIP_NAME) $$files
	@echo "$(GREEN)✅ Zipped successfully!$(RESET)"
	@echo

# =========================
# 4. Database Migrations
# =========================
.PHONY: migrate-create migrate-up migrate-down migrate-reset migrate-force migrate-version

migrate-create: ## Create a new migration file (Usage: make migrate-create name=create_users_table)
	@echo "$(BLUE)🔨 Creating new migration file...$(RESET)"
	@$(MIGRATE_BIN) create -ext sql -dir $(MIGRATIONS_DIR) -seq $(name)
	@echo "$(GREEN)✅ Migration file created!$(RESET)"
	@echo

migrate-up: ## Run migrations and configure backend and mailer logins
	$(call require_env_file)
	@set -a && . ./config/.env && set +a; \
	test -n "$${POSTGRES_APP_PASS}" && test -n "$${POSTGRES_MAILER_PASS}" || { echo "Set POSTGRES_APP_PASS and POSTGRES_MAILER_PASS in config/.env"; exit 1; }
	@echo "$(BLUE)🔄 Running migrations... $(RESET)"
	$(call run_migrate,up)
	@set -a && . ./config/.env && set +a; \
	printf '%s\n' '\getenv app_password POSTGRES_APP_PASS' '\getenv mailer_password POSTGRES_MAILER_PASS' "SELECT format('ALTER ROLE dataset_app LOGIN PASSWORD %L', :'app_password') \gexec" "SELECT format('ALTER ROLE dataset_mailer LOGIN PASSWORD %L', :'mailer_password') \gexec" | \
	$(DOCKER) exec -i -e POSTGRES_APP_PASS -e POSTGRES_MAILER_PASS dataset_postgres psql -X -q --single-transaction -v ON_ERROR_STOP=1 -U "$${POSTGRES_USER}" -d "$${POSTGRES_BASE}" -p "$${POSTGRES_PORT}"
	@echo "$(GREEN)✅ Migrations and database logins configured!$(RESET)"
	@echo

migrate-down: ## Rollback the last migration
	@echo "$(BLUE)🔄 Rolling back last migration...$(RESET)"
	$(call run_migrate,down 1)
	@echo "$(GREEN)✅ Rollback completed!$(RESET)"
	@echo

migrate-reset: ## Rollback all migrations
	@echo "$(BLUE)🔄 Resetting database...$(RESET)"
	$(call run_migrate,down)
	@echo "$(GREEN)✅ Database reset!$(RESET)"
	@echo

migrate-force: ## Force set migration version (Usage: make migrate-force version=1)
	@echo "$(BLUE)⚠️ Forcing migration version...$(RESET)"
	$(call run_migrate,force $(version))
	@echo "$(GREEN)✅ Version forced!$(RESET)"
	@echo

migrate-version: ## Show current migration version
	@echo "$(BLUE)📌 Checking migration version...$(RESET)"
	$(call run_migrate,version)
	@echo "$(GREEN)✅ Migration version loaded!$(RESET)"
	@echo

# =========================
# 5. Docker Compose
# =========================
.PHONY: admin-bootstrap admin-promote admin-disable admin-enable compose-up compose-build compose-services compose-down compose-clean compose-remove compose-exec compose-log compose-top compose-stats

admin-bootstrap: ## Create the first admin, asking name, email and password
	@$(COMPOSE_COMMAND) exec backend ./admin bootstrap

admin-promote: ## Give an active user the admin role, asking the email
	@$(COMPOSE_COMMAND) exec backend ./admin promote

admin-disable: ## Disable a user and end their sessions, asking the email
	@$(COMPOSE_COMMAND) exec backend ./admin disable

admin-enable: ## Enable a disabled user, asking the email
	@$(COMPOSE_COMMAND) exec backend ./admin enable

compose-up: ## Create and start containers
	@echo "$(BLUE)🚀 Starting Docker containers...$(RESET)"
	$(call require_env_file)
	@$(COMPOSE_COMMAND) up -d
	@echo "$(GREEN)✅ Containers are up and running!$(RESET)"
	@echo

compose-build: ## Build, create and start containers
	@echo "$(BLUE)🚢 Building and starting Docker containers...$(RESET)"
	$(call require_env_file)
	@$(COMPOSE_COMMAND) up -d --build
	@echo "$(GREEN)✅ Containers are up and running!$(RESET)"
	@echo
	@$(clean_dangling_images)

compose-services: ## Build, create and start services containers
	@echo "$(BLUE)🚢 Building and starting Docker services containers...$(RESET)"
	$(call require_env_file)
	@$(COMPOSE_SERVICE) up -d --build
	@echo "$(GREEN)✅ Containers are up and running!$(RESET)"
	@echo
	@$(clean_dangling_images)

compose-down: ## Stop and remove containers and networks
	@echo "$(YELLOW)🛑 Stopping and removing containers...$(RESET)"
	$(call require_env_file)
	@$(COMPOSE_COMMAND) down
	@echo "$(GREEN)✅  Containers stopped.$(RESET)"
	@echo

compose-clean: ## Clear dangling Docker images
	$(clean_dangling_images)

compose-remove: ## Stop and remove containers, networks and volumes
	$(call require_env_file)
	@echo "$(RED)⚠️  WARNING: This will permanently delete all containers, networks, and VOLUMES!$(RESET)"
	@echo -n "$(RED)❌ All data will be lost. Are you sure? [y/N] $(RESET)" && read ans && [ $${ans:-N} = y ]
	@echo "$(YELLOW)\n🛑 Stopping and removing all Docker resources...$(RESET)"
	@$(COMPOSE_COMMAND) down -v --remove-orphans
	@echo "$(GREEN)✅ Containers, networks, and volumes removed successfully.$(RESET)"
	@echo

compose-exec: ## Access container shell
	@echo "$(BLUE)🔑 Accessing the container shell...$(RESET)"
	$(call require_env_file)
	@$(COMPOSE_COMMAND) exec -it ${DOCKER_SERVICE} sh

compose-log: ## Show container logger
	@echo "$(BLUE)📜 Fetching container logs...$(RESET)"
	$(call require_env_file)
	@$(COMPOSE_COMMAND) logs -f ${DOCKER_SERVICE}

compose-top: ## Display containers processes
	@echo "$(BLUE)🔍 Displaying container processes...$(RESET)"
	$(call require_env_file)
	@$(COMPOSE_COMMAND) top

compose-stats: ## Display containers stats
	@echo "$(CYAN)📊 Showing container statistics...$(RESET)"
	$(call require_env_file)
	@$(COMPOSE_COMMAND) stats
