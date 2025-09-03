.PHONY: help build test test-e2e run fmt vet tidy clean deps docker-build docker-run

GO ?= go
BIN_NAME ?= toggl-scraper
BINDIR ?= bin
PKG ?= ./...

# Local, sandbox-friendly defaults (can be overridden by env)
CACHE_DIR ?= .gocache
GOMODCACHE ?= $(CURDIR)/$(CACHE_DIR)/mod
GOPATH ?= $(CURDIR)/$(CACHE_DIR)
GOCACHE ?= $(CURDIR)/$(CACHE_DIR)/build
# Avoid auto toolchain download unless explicitly changed by caller
GOTOOLCHAIN ?= local

ENVVARS = GOTOOLCHAIN=$(GOTOOLCHAIN) GOMODCACHE=$(GOMODCACHE) GOPATH=$(GOPATH) GOCACHE=$(GOCACHE)

.DEFAULT_GOAL := help

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_-]+:.*##/ {printf "\033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

deps: ## Download go modules
	$(ENVVARS) $(GO) mod download

build: ## Build the binary to ./bin
	@mkdir -p $(BINDIR)
	$(ENVVARS) $(GO) build -o $(BINDIR)/$(BIN_NAME) ./cmd/toggl-scraper

run: ## Run the service (pass flags via ARGS="--once ...")
	$(ENVVARS) $(GO) run ./cmd/toggl-scraper $(ARGS)

test: ## Run unit tests
	$(ENVVARS) $(GO) test $(PKG)

test-e2e: ## Run e2e tests (requires Docker)
	$(ENVVARS) $(GO) test -tags=e2e ./e2e -v

fmt: ## Format code
	$(ENVVARS) $(GO) fmt $(PKG)

vet: ## Vet code
	$(ENVVARS) $(GO) vet $(PKG)

tidy: ## Tidy go.mod/go.sum
	$(ENVVARS) $(GO) mod tidy

clean: ## Remove build artifacts
	rm -rf $(BINDIR)

docker-build: ## Build Docker image
	docker build -t toggl-scraper:latest .

docker-run: ## Run Docker container (set env via E=...)
	# Example usage:
	# make docker-run E="-e TOGGL_API_TOKEN=... -e MYSQL_DSN=... -e SYNC_TZ=Europe/Berlin"
	docker run --rm --name toggl-scraper \
		$(E) toggl-scraper:latest
