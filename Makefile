.DEFAULT_GOAL := help

INSTALL_TARGETS := core codex claude opencode all

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
	  printf '%s\n' 'usage: make install core|codex|claude|opencode|all' >&2; \
	  exit 1; \
	fi; \
	case "$$target" in \
	  core|codex|claude|opencode|all) ;; \
	  *) printf 'unknown install target: %s\n' "$$target" >&2; exit 1 ;; \
	esac; \
	./scripts/install-deps.sh; \
	bin/panefleet install "$$target"

deps:
	@./scripts/install-deps.sh

$(INSTALL_TARGETS):
	@if [ "$(firstword $(MAKECMDGOALS))" = "install" ]; then \
	  :; \
	else \
	  ./scripts/install-deps.sh; \
	  bin/panefleet install "$@"; \
	fi

doctor:
	@bin/panefleet doctor --install

uninstall:
	@bin/panefleet uninstall

test:
	./scripts/test.sh

preflight:
	bin/panefleet preflight

bridge:
	PANEFLEET_BRIDGE_INSTALL_MODE=build ./scripts/install-bridge.sh

bridge-download:
	PANEFLEET_BRIDGE_INSTALL_MODE=download ./scripts/install-bridge.sh

release-check:
	goreleaser release --snapshot --clean
