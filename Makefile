# Build, install, and test the opentea components, grouped the way the TEA
# spec itself groups roles: the server, the reference consumer (client), and
# auxiliary tools. See README.md / README-deploy.md / docs/bundle-format.md
# for what each one does.

GO        ?= go
GOFMT     ?= $(shell $(GO) env GOROOT)/bin/gofmt
PREFIX    ?= /usr/local
BINDIR    ?= $(PREFIX)/bin
BUILD_DIR ?= bin

SERVER_BINARIES   := opentea
CONSUMER_BINARIES := teaclient
TOOLS_BINARIES    := fixtures bundlecheck

BINARIES         := $(SERVER_BINARIES) $(CONSUMER_BINARIES) $(TOOLS_BINARIES)
SERVER_TARGETS   := $(addprefix $(BUILD_DIR)/,$(SERVER_BINARIES))
CONSUMER_TARGETS := $(addprefix $(BUILD_DIR)/,$(CONSUMER_BINARIES))
TOOLS_TARGETS    := $(addprefix $(BUILD_DIR)/,$(TOOLS_BINARIES))

.PHONY: all build build-server build-consumer build-tools warn-if-root check-built install uninstall test test-verbose vet fmt fmt-check tidy check clean help

all: build ## Alias for build (server + consumer + tools)

build: build-server build-consumer build-tools ## Build everything into ./bin (server + consumer + tools)

build-server: warn-if-root $(SERVER_TARGETS) ## Build just the server (opentea)

build-consumer: warn-if-root $(CONSUMER_TARGETS) ## Build just the reference consumer/client CLI (teaclient)

build-tools: warn-if-root $(TOOLS_TARGETS) ## Build just the auxiliary tools (fixtures, bundlecheck)

# `go build`/`go test` invoke git to stamp VCS info (Go 1.18+); if run as root
# against a repo owned by another user, git's "dubious ownership" safety check
# makes that fail with "error obtaining VCS status: exit status 128". Warn
# rather than silently hitting that later -- building should happen as your
# normal user; only `make install` needs root, and it no longer builds (see
# below).
warn-if-root:
	@if [ "$$(id -u)" = "0" ]; then \
		echo "warning: building as root (uid 0) -- 'go build' stamps VCS info via git, which"; \
		echo "  can fail with 'error obtaining VCS status: exit status 128' if this repo is"; \
		echo "  owned by a different user (git's dubious-ownership check). Prefer building as"; \
		echo "  your normal user ('make build') and only using sudo for 'make install'."; \
	fi

$(BUILD_DIR)/opentea: | $(BUILD_DIR)
	$(GO) build -o $@ ./cmd/opentea

$(BUILD_DIR)/teaclient: | $(BUILD_DIR)
	$(GO) build -o $@ ./cmd/teaclient

$(BUILD_DIR)/fixtures: | $(BUILD_DIR)
	$(GO) build -o $@ ./cmd/fixtures

$(BUILD_DIR)/bundlecheck: | $(BUILD_DIR)
	$(GO) build -o $@ ./cmd/bundlecheck

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

# Deliberately does NOT depend on `build`: install typically runs under sudo
# (to write to $(BINDIR)), and building as root risks the git VCS-stamping
# failure warn-if-root explains above. Build as yourself first.
install: check-built ## Install already-built ./bin binaries to $(BINDIR) (default /usr/local/bin; needs root -- run `make build` first, then `sudo make install`; override with PREFIX=/some/path or BINDIR=/some/path)
	install -d $(BINDIR)
	install -m 755 $(BUILD_DIR)/opentea $(BINDIR)/opentea
	install -m 755 $(BUILD_DIR)/teaclient $(BINDIR)/teaclient
	install -m 755 $(BUILD_DIR)/fixtures $(BINDIR)/fixtures
	install -m 755 $(BUILD_DIR)/bundlecheck $(BINDIR)/bundlecheck

check-built:
	@missing=0; \
	for b in $(BINARIES); do \
		if [ ! -x "$(BUILD_DIR)/$$b" ]; then \
			echo "error: $(BUILD_DIR)/$$b not found" >&2; \
			missing=1; \
		fi; \
	done; \
	if [ "$$missing" = "1" ]; then \
		echo "run 'make build' first (as your normal user, not root/sudo), then 'sudo make install'" >&2; \
		exit 1; \
	fi

uninstall: ## Remove binaries previously installed to $(BINDIR)
	rm -f $(BINDIR)/opentea $(BINDIR)/teaclient $(BINDIR)/fixtures $(BINDIR)/bundlecheck

test: ## Run the full test suite (repo, api, admin, webadmin, client, fixtures -- all real in-process integration tests, no mocks for the server itself)
	$(GO) test ./...

test-verbose: ## Same as test, with -v
	$(GO) test -v ./...

vet: ## go vet the whole module
	$(GO) vet ./...

fmt: ## gofmt -w the whole module
	$(GOFMT) -w .

fmt-check: ## Fail if any file isn't gofmt-formatted, without changing anything
	@unformatted="$$($(GOFMT) -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; echo "$$unformatted"; exit 1; \
	fi

tidy: ## go mod tidy
	$(GO) mod tidy

check: fmt-check vet test ## Run everything CI/a pre-commit check should run: fmt-check + vet + test

clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*## "}; {printf "  %-14s %s\n", $$1, $$2}'
