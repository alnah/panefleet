.PHONY: test bridge bridge-download release-check

test:
	./scripts/test.sh

bridge:
	PANEFLEET_BRIDGE_INSTALL_MODE=build ./scripts/install-bridge.sh

bridge-download:
	PANEFLEET_BRIDGE_INSTALL_MODE=download ./scripts/install-bridge.sh

release-check:
	goreleaser release --snapshot --clean
