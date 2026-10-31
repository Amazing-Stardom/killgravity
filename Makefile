.PHONY: all install install-user install-system uninstall uninstall-purge test check audit release

all: check test audit

install:
	./install.sh

install-user:
	./install.sh --user

install-system:
	sudo ./install.sh --system

uninstall:
	./install.sh uninstall

uninstall-purge:
	./install.sh uninstall --purge

check:
	python3 -m py_compile bin/killgravity mcp/server.py
	bash -n install.sh tests/test_detection.sh scripts/release.sh scripts/audit.sh
	node -c ide-extension/extension.js

test:
	./tests/test_detection.sh

audit:
	./scripts/audit.sh

release:
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make release VERSION=X.Y.Z"; \
		exit 1; \
	fi
	./scripts/release.sh $(VERSION)
