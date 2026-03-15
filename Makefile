.DEFAULT_GOAL := help

INSTALL_TARGETS := core codex claude opencode all
PANEFLEET_BIN ?= bin/panefleet
PANEFLEET_INSTALL_DEPS_CMD ?= ./scripts/install-deps.sh

.PHONY: help install $(INSTALL_TARGETS) doctor uninstall deps test preflight bridge bridge-download release-check

help:
	@printf '%s\n' \
	  'make install core      # core only, heuristic-first' \
	  'make install codex     # core + codex integration' \
	  'make install claude    # core + claude integration' \
	  'make install opencode  # core + opencode integration' \
	  'make install all       # core + all integrations' \
	  'make doctor            # installation diagnostics' \
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

uninstall:
	@$(PANEFLEET_BIN) uninstall

test:
	./scripts/test.sh

preflight:
	$(PANEFLEET_BIN) preflight

bridge:
	PANEFLEET_BRIDGE_INSTALL_MODE=force-build ./scripts/install-bridge.sh

bridge-download:
	PANEFLEET_BRIDGE_INSTALL_MODE=force-download ./scripts/install-bridge.sh

release-check:
	goreleaser release --snapshot --clean
