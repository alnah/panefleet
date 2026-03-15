.DEFAULT_GOAL := help

.PHONY: help install core codex claude opencode all doctor uninstall deps test preflight bridge bridge-download release-check

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
	@:

deps:
	@./scripts/install-deps.sh

core:
	@./scripts/install-deps.sh
	@bin/panefleet install core

codex:
	@./scripts/install-deps.sh
	@bin/panefleet install codex

claude:
	@./scripts/install-deps.sh
	@bin/panefleet install claude

opencode:
	@./scripts/install-deps.sh
	@bin/panefleet install opencode

all:
	@./scripts/install-deps.sh
	@bin/panefleet install all

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
