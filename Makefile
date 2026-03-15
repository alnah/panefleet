.PHONY: test preflight setup-core setup-codex setup-claude setup-opencode setup-all bridge bridge-download release-check

test:
	./scripts/test.sh

preflight:
	bin/panefleet preflight

setup-core:
	bin/panefleet setup core

setup-codex:
	bin/panefleet setup codex

setup-claude:
	bin/panefleet setup claude

setup-opencode:
	bin/panefleet setup opencode

setup-all:
	bin/panefleet setup all

bridge:
	PANEFLEET_BRIDGE_INSTALL_MODE=build ./scripts/install-bridge.sh

bridge-download:
	PANEFLEET_BRIDGE_INSTALL_MODE=download ./scripts/install-bridge.sh

release-check:
	goreleaser release --snapshot --clean
