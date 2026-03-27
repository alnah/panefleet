.DEFAULT_GOAL := help

INSTALL_TARGETS := core codex claude opencode all
PANEFLEET_BIN ?= bin/panefleet
PANEFLEET_INSTALL_DEPS_CMD ?= ./scripts/install-deps.sh
GO ?= go
GOFMT ?= gofmt
GOLANGCI_LINT ?= golangci-lint
GOSEC ?= gosec

.PHONY: help install $(INSTALL_TARGETS) doctor uninstall deps test preflight bridge bridge-download release-check health backup-go-db restore-go-db fmt-go fmt-go-check lint-go lint-go-fix gosec-go vet-go test-go test-go-race verify-go

help:
	@printf '%s\n' \
	  'make install core      # core only, heuristic-first' \
	  'make install codex     # core + codex integration' \
	  'make install claude    # core + claude integration' \
	  'make install opencode  # core + opencode integration' \
	  'make install all       # core + all integrations' \
	  'make doctor            # installation diagnostics' \
	  'make health            # operational healthcheck (preflight + doctor)' \
	  'make backup-go-db      # backup go runtime sqlite db' \
	  'make restore-go-db FILE=/path/to/backup.db  # restore go runtime sqlite db' \
	  'make fmt-go            # rewrite Go files with gofmt' \
	  'make fmt-go-check      # fail if any Go file is not gofmt-ed' \
	  'make lint-go           # run golangci-lint with repo rules' \
	  'make lint-go-fix       # apply formatter fixes supported by golangci-lint' \
	  'make gosec-go          # run the Go security analyzer' \
	  'make vet-go            # run go vet on all packages' \
	  'make test-go           # run Go unit/integration tests' \
	  'make test-go-race      # run Go tests with the race detector' \
	  'make verify-go         # run formatting, lint, security, vet, and tests' \
	  'make uninstall         # remove tmux bindings and hooks'

install:
	@target='$(word 2,$(MAKECMDGOALS))'; \
	if [ -z "$$target" ]; then \
	  target='core'; \
	fi; \
	case "$$target" in \
	  core|codex|claude|opencode|all) ;; \
	  *) printf 'unknown install target: %s\n' "$$target" >&2; exit 1 ;; \
	esac; \
	$(PANEFLEET_INSTALL_DEPS_CMD); \
	$(PANEFLEET_BIN) install "$$target"

deps:
	@$(PANEFLEET_INSTALL_DEPS_CMD)

$(INSTALL_TARGETS):
	@if [ "$(firstword $(MAKECMDGOALS))" = "install" ]; then \
	  :; \
	else \
	  $(PANEFLEET_INSTALL_DEPS_CMD); \
	  $(PANEFLEET_BIN) install "$@"; \
	fi

doctor:
	@$(PANEFLEET_BIN) doctor --install

health:
	@./scripts/ops-healthcheck.sh

backup-go-db:
	@./scripts/ops-go-db-backup.sh

restore-go-db:
	@if [ -z "$(FILE)" ]; then \
	  printf 'usage: make restore-go-db FILE=/path/to/backup.db\n' >&2; \
	  exit 1; \
	fi
	@./scripts/ops-go-db-restore.sh "$(FILE)"

uninstall:
	@$(PANEFLEET_BIN) uninstall

test:
	./scripts/test.sh

# Go quality gates are split into small targets so local runs and CI can reuse
# the same commands without hiding which stage failed.
fmt-go:
	@$(GOFMT) -w $$(rg --files -g '*.go')

fmt-go-check:
	@files="$$( $(GOFMT) -l . )"; \
	if [ -n "$$files" ]; then \
	  printf '%s\n' "$$files" >&2; \
	  exit 1; \
	fi

lint-go:
	@$(GOLANGCI_LINT) run ./...

lint-go-fix:
	@$(GOLANGCI_LINT) fmt
	@$(GOLANGCI_LINT) run --fix ./...

gosec-go:
	@$(GOSEC) ./...

vet-go:
	@$(GO) vet ./...

test-go:
	@$(GO) test ./...

test-go-race:
	@$(GO) test -race ./...

verify-go: fmt-go-check lint-go gosec-go vet-go test-go test-go-race

preflight:
	$(PANEFLEET_BIN) preflight

bridge:
	PANEFLEET_BRIDGE_INSTALL_MODE=force-build ./scripts/install-bridge.sh

bridge-download:
	PANEFLEET_BRIDGE_INSTALL_MODE=force-download ./scripts/install-bridge.sh

release-check:
	goreleaser release --snapshot --clean
