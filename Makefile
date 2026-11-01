.PHONY: all build check test audit install install-user install-system uninstall uninstall-purge release

all: build check test audit

build:
	go build -o bin/killgravity .

install: build
	./install.sh

install-user: build
	./install.sh --user

install-system: build
	sudo ./install.sh --system

uninstall:
	./install.sh uninstall

uninstall-purge:
	./install.sh uninstall --purge

check: build
	go vet ./...
	python3 -m py_compile mcp/server.py
	bash -n install.sh tests/test_detection.sh scripts/release.sh scripts/audit.sh
	node -c ide-extension/extension.js

test: build
	./tests/test_detection.sh

audit: build
	./scripts/audit.sh

release:
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make release VERSION=X.Y.Z"; \
		exit 1; \
	fi
	./scripts/release.sh $(VERSION)
