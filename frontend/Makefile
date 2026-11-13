SHELL := /bin/bash
.DEFAULT_GOAL := help

COMPOSE := docker compose --env-file config/.env -f deploy/compose.yml

GREEN  := \033[1;32m
YELLOW := \033[1;33m
RED    := \033[1;31m
BLUE   := \033[1;34m
CYAN   := \033[1;36m
RESET  := \033[0m

define require_env_file
	@test -f config/.env || (echo -e "$(RED)config/.env not found. Run make init first.$(RESET)" && exit 1)
endef

.PHONY: help init install dev check build compose-up compose-build compose-down compose-logs

help: ## Display available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "$(CYAN)%-16s$(RESET) %s\n", $$1, $$2}'

init: ## Create config/.env with a fresh session secret, if missing
	@if [ -f config/.env ]; then \
		echo -e "$(YELLOW)config/.env already exists; leaving it untouched.$(RESET)"; \
	else \
		config/env.sh && echo -e "$(GREEN)✅ config/.env created.$(RESET)"; \
	fi

install: ## Install dependencies from the lockfile
	@pnpm install --frozen-lockfile

dev: install ## Run with hot reload on http://localhost:3006
	$(call require_env_file)
	@pnpm dev

check: install ## Typecheck, lint and test
	@pnpm check

build: install ## Production build
	@pnpm build

compose-up: ## Start the web container (the backend stack must be up)
	$(call require_env_file)
	@$(COMPOSE) up -d

compose-build: ## Build and start the web container (the backend stack must be up)
	$(call require_env_file)
	@$(COMPOSE) up -d --build --force-recreate

compose-down: ## Stop and remove the web container
	$(call require_env_file)
	@$(COMPOSE) down

compose-logs: ## Follow the web container logs
	$(call require_env_file)
	@$(COMPOSE) logs -f web
