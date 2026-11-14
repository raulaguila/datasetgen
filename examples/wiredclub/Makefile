SHELL := /bin/bash
.DEFAULT_GOAL := help

DOCS_REPO ?= https://github.com/wiredclub/docs.git
FURNIDATA_URL ?= https://www.habbo.com.br/gamedata/furnidata_json/1
TEXTS_URL ?= https://www.habbo.com.br/gamedata/external_flash_texts/1
CACHE := .cache
OUT ?= out
# Files saved by hand from a browser; when empty, they are downloaded.
FURNIDATA ?=
TEXTS ?=

.PHONY: help wireds clean

help: ## Display available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[1;36m%-10s\033[0m %s\n", $$1, $$2}'

wireds: ## Fetch the Wired Club docs, the furnidata and the client texts; write out/wireds.jsonl and out/guias.jsonl
	@mkdir -p $(CACHE)
	@if [ -d $(CACHE)/docs/.git ]; then git -C $(CACHE)/docs pull -q --ff-only; \
	else git clone -q --depth 1 $(DOCS_REPO) $(CACHE)/docs; fi
	@# fetch VAR URL FILE: downloads URL into the cache, or explains how to pass the file by hand.
	@fetch() { \
		code=$$(curl -s -A 'Mozilla/5.0' -o "$(CACHE)/$$3.tmp" -w '%{http_code}' "$$2"); \
		if [ "$$code" = 200 ]; then mv "$(CACHE)/$$3.tmp" "$(CACHE)/$$3"; return; fi; \
		rm -f "$(CACHE)/$$3.tmp"; \
		echo "$$2 answered $$code. Open it in a browser, save the page and run:"; \
		echo "  make wireds $$1=/path/to/$$3"; return 1; \
	}; \
	furnidata="$(FURNIDATA)"; texts="$(TEXTS)"; \
	if [ -z "$$furnidata" ]; then fetch FURNIDATA $(FURNIDATA_URL) furnidata.json || exit 1; furnidata=$(CACHE)/furnidata.json; fi; \
	if [ -z "$$texts" ]; then fetch TEXTS $(TEXTS_URL) external_flash_texts.txt || exit 1; texts=$(CACHE)/external_flash_texts.txt; fi; \
	python3 convert.py $(CACHE)/docs $(OUT) --furnidata "$$furnidata" --texts "$$texts"

clean: ## Remove the downloaded docs and the generated files
	@rm -rf $(CACHE) $(OUT)
