SHELL := /bin/bash
.DEFAULT_GOAL := help

DOCS_REPO ?= https://github.com/wiredclub/docs.git
FURNIDATA_URL ?= https://www.habbo.com.br/gamedata/furnidata_json/1
CACHE := .cache
OUT ?= out
# A furnidata JSON saved by hand; when empty, it is downloaded.
FURNIDATA ?=

.PHONY: help wireds clean

help: ## Display available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[1;36m%-10s\033[0m %s\n", $$1, $$2}'

wireds: ## Fetch the Wired Club docs and the furnidata, write out/wireds.jsonl and out/guias.jsonl
	@mkdir -p $(CACHE)
	@if [ -d $(CACHE)/docs/.git ]; then git -C $(CACHE)/docs pull -q --ff-only; \
	else git clone -q --depth 1 $(DOCS_REPO) $(CACHE)/docs; fi
	@furni="$(FURNIDATA)"; \
	if [ -z "$$furni" ]; then \
		furni=$(CACHE)/furnidata.json; \
		code=$$(curl -s -A 'Mozilla/5.0' -o $$furni.tmp -w '%{http_code}' $(FURNIDATA_URL)); \
		if [ "$$code" = 200 ]; then mv $$furni.tmp $$furni; \
		else rm -f $$furni.tmp; \
			echo "The furnidata download answered $$code. Open $(FURNIDATA_URL) in a browser, save it and run:"; \
			echo "  make wireds FURNIDATA=/path/to/furnidata.json"; exit 1; fi; \
	fi; \
	python3 convert.py $(CACHE)/docs $(OUT) "$$furni"

clean: ## Remove the downloaded docs and the generated files
	@rm -rf $(CACHE) $(OUT)
