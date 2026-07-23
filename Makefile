BINARY ?= bin/tok
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: test build run fmt

test:
	go test ./...

build:
	mkdir -p $(dir $(BINARY))
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/tok

run:
	go run -ldflags "$(LDFLAGS)" ./cmd/tok $(ARGS)

fmt:
	gofmt -w cmd internal
