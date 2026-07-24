BINARY ?= bin/tok
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

DEVCTL := ./scripts/devctl.sh

.PHONY: test build run fmt dev-start dev-stop dev-restart dev-status dev-logs dev-api-start dev-api-stop dev-app-start dev-app-stop

test:
	go test ./...

build:
	mkdir -p $(dir $(BINARY))
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/tok

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
	cp $(BINARY) ~/.local/bin/tok
