BINARY ?= bin/tok
VERSION ?= dev
INSTALL_BIN ?= $(HOME)/go/bin/tok
TOK_SYSTEMD_UNITS ?= tok-ui.service tok-index-watch.service
STATICCHECK_VERSION ?= v0.7.0
STATICCHECK_BIN ?= $(CURDIR)/bin/staticcheck-$(STATICCHECK_VERSION)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
EMBED_WEB_DIR := internal/httpserver/webdist

DEVCTL := ./scripts/devctl.sh

.PHONY: test vet staticcheck quality build web-build web-embed install run fmt dev-start dev-stop dev-restart dev-status dev-logs dev-api-start dev-api-stop dev-app-start dev-app-stop

quality:
	./scripts/check-file-budgets.sh
	cd web && pnpm exec jscpd --config ../.jscpd.json
	./scripts/check-jscpd-baseline.mjs .quality/jscpd-report.json .jscpd-baseline.json

test:
	go test ./...

vet:
	go vet ./...

staticcheck:
	@if [ ! -x "$(STATICCHECK_BIN)" ]; then \
		mkdir -p "$(dir $(STATICCHECK_BIN))"; \
		tmp="$$(mktemp -d)"; \
		trap 'rm -rf "$$tmp"' EXIT; \
		GOBIN="$$tmp" go install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION); \
		mv "$$tmp/staticcheck" "$(STATICCHECK_BIN)"; \
	fi
	"$(STATICCHECK_BIN)" ./...

build: web-embed
	mkdir -p $(dir $(BINARY))
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/tok

web-build:
	cd web && pnpm build

web-embed: web-build
	mkdir -p $(EMBED_WEB_DIR)
	find $(EMBED_WEB_DIR) -mindepth 1 -maxdepth 1 ! -name '.gitignore' ! -name 'placeholder.txt' -exec rm -rf {} +
	cp -R web/dist/. $(EMBED_WEB_DIR)/

run:
	go run -ldflags "$(LDFLAGS)" ./cmd/tok $(ARGS)

fmt:
	gofmt -w cmd internal

dev-start:
	$(DEVCTL) start

dev-stop:
	$(DEVCTL) stop

dev-restart:
	$(DEVCTL) restart

dev-status:
	$(DEVCTL) status

dev-logs:
	$(DEVCTL) logs $(SERVICE)

dev-api-start:
	$(DEVCTL) api-start

dev-api-stop:
	$(DEVCTL) api-stop

dev-app-start:
	$(DEVCTL) app-start

dev-app-stop:
	$(DEVCTL) app-stop

install: build
	mkdir -p "$(dir $(INSTALL_BIN))"
	tmp="$$(mktemp "$(dir $(INSTALL_BIN))tok.XXXXXX")" && \
		cp $(BINARY) "$$tmp" && \
		chmod 755 "$$tmp" && \
		mv -f "$$tmp" "$(INSTALL_BIN)"
	@if command -v systemctl >/dev/null 2>&1; then \
		systemctl --user daemon-reload; \
		for unit in $(TOK_SYSTEMD_UNITS); do \
			if systemctl --user list-unit-files "$$unit" --no-legend 2>/dev/null | grep -q .; then \
				if systemctl --user is-active --quiet "$$unit"; then \
					systemctl --user restart "$$unit"; \
				else \
					systemctl --user start "$$unit"; \
				fi; \
			fi; \
		done; \
	fi
