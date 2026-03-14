# Makefile for DAAO Nexus project
# Supports: proto, build, test, dev, lint, docker, clean

# Go settings
GO := go
GOFLAGS := 
BINARY_NAME := nexus

# Version for satellite builds — override with: make VERSION=v1.2.3 build-satellite-all
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.Version=$(VERSION)

# Node.js settings
NPM := npm
COCKPIT_DIR := cockpit

# Proto settings
PROTO_DIR := proto

# Docker settings
DOCKER := docker
COMPOSE := docker-compose

# Default target
.PHONY: all
all: build

# Proto generation
.PHONY: proto
proto:
	$(GO) generate ./$(PROTO_DIR)/...

# Build: Go binary + Cockpit assets
.PHONY: build
build: build-go build-cockpit

# Build Go binary (native OS)
.PHONY: build-go
build-go:
	$(GO) build -o bin/$(BINARY_NAME) ./cmd/$(BINARY_NAME)

# Build nexus Linux binary for docker-compose bind mount (bin/nexus-linux-amd64)
# Run this before `docker-compose up` or `docker-compose restart nexus`
.PHONY: build-nexus-linux
build-nexus-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o bin/nexus-linux-amd64 ./cmd/nexus

# Build Cockpit frontend
.PHONY: build-cockpit
build-cockpit:
	cd $(COCKPIT_DIR) && $(NPM) run build

# Test: Go tests + Cockpit tests
.PHONY: test
test: test-go test-cockpit

# Run Go tests with race detector
.PHONY: test-go
test-go:
	$(GO) test -race ./...

# Run Cockpit tests
.PHONY: test-cockpit
test-cockpit:
	cd $(COCKPIT_DIR) && $(NPM) test

# Dev: Start all services
.PHONY: dev
dev: dev-nexus dev-cockpit

# Start Nexus in development mode
.PHONY: dev-nexus
dev-nexus:
	$(GO) run ./cmd/$(BINARY_NAME)

# Start Cockpit dev server
.PHONY: dev-cockpit
dev-cockpit:
	cd $(COCKPIT_DIR) && $(NPM) run dev

# Lint: golangci-lint + eslint
.PHONY: lint
lint: lint-go lint-cockpit

# Run Go linter
.PHONY: lint-go
lint-go:
	golangci-lint run

# Run Cockpit linter
.PHONY: lint-cockpit
lint-cockpit:
	cd $(COCKPIT_DIR) && $(NPM) run lint

# Docker: Build images
.PHONY: docker
docker:
	$(COMPOSE) build

# Clean: Remove build artifacts
.PHONY: clean
clean: clean-go clean-cockpit

# Clean Go artifacts
.PHONY: clean-go
clean-go:
	rm -rf bin/
	$(GO) clean

# Clean Cockpit artifacts
.PHONY: clean-cockpit
clean-cockpit:
	cd $(COCKPIT_DIR) && $(NPM) run clean || true

# ── Satellite Daemon (Cross-Compilation) ──────────────────────────────────

SATELLITE_SRC := ./cmd/daao

# Build satellite for current OS/arch
.PHONY: build-satellite
build-satellite:
	$(GO) build -ldflags="$(LDFLAGS)" -o bin/daao$(if $(filter windows,$(GOOS)),.exe,) $(SATELLITE_SRC)

# Build for all supported platforms
.PHONY: build-satellite-all
build-satellite-all: build-satellite-linux-amd64 build-satellite-linux-arm64 build-satellite-darwin-amd64 build-satellite-darwin-arm64 build-satellite-windows-amd64 build-satellite-windows-arm64

.PHONY: build-satellite-linux-amd64
build-satellite-linux-amd64:
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags="$(LDFLAGS)" -o bin/daao-linux-amd64 $(SATELLITE_SRC)

.PHONY: build-satellite-linux-arm64
build-satellite-linux-arm64:
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags="$(LDFLAGS)" -o bin/daao-linux-arm64 $(SATELLITE_SRC)

.PHONY: build-satellite-darwin-amd64
build-satellite-darwin-amd64:
	GOOS=darwin GOARCH=amd64 $(GO) build -ldflags="$(LDFLAGS)" -o bin/daao-darwin-amd64 $(SATELLITE_SRC)

.PHONY: build-satellite-darwin-arm64
build-satellite-darwin-arm64:
	GOOS=darwin GOARCH=arm64 $(GO) build -ldflags="$(LDFLAGS)" -o bin/daao-darwin-arm64 $(SATELLITE_SRC)

.PHONY: build-satellite-windows-amd64
build-satellite-windows-amd64:
	GOOS=windows GOARCH=amd64 $(GO) build -ldflags="$(LDFLAGS)" -o bin/daao-windows-amd64.exe $(SATELLITE_SRC)

.PHONY: build-satellite-windows-arm64
build-satellite-windows-arm64:
	GOOS=windows GOARCH=arm64 $(GO) build -ldflags="$(LDFLAGS)" -o bin/daao-windows-arm64.exe $(SATELLITE_SRC)

# ── Mock Satellite & Test Targets ────────────────────────────────────────

MOCK_SRC := ./cmd/daao-mock

# Build mock satellite
.PHONY: build-mock
build-mock:
	$(GO) build -o bin/daao-mock$(if $(filter windows,$(GOOS)),.exe,) $(MOCK_SRC)

# Run integration tests (requires PostgreSQL)
.PHONY: test-integration
test-integration:
	$(GO) test -race -timeout 120s ./tests/integration/...

# Run end-to-end tests (requires PostgreSQL + builds daao-mock)
.PHONY: test-e2e
test-e2e: build-mock
	$(GO) test -race -timeout 180s ./tests/e2e/...

# Run bash smoke test (requires docker compose up + daao-mock)
.PHONY: smoke-test
smoke-test: build-mock
	./scripts/smoke-test.sh

# Run PowerShell smoke test (Windows)
.PHONY: smoke-test-ps
smoke-test-ps: build-mock
	powershell -ExecutionPolicy Bypass -File scripts/smoke-test.ps1

# Run all test suites sequentially
.PHONY: test-all
test-all: test-go test-integration test-e2e test-cockpit

# Help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  proto                          - Regenerate protobuf files"
	@echo "  build                          - Build Go binary and Cockpit assets"
	@echo "  build-satellite                - Build satellite daemon for current OS"
	@echo "  build-satellite-all            - Cross-compile satellite for all platforms"
	@echo "  build-mock                     - Build mock satellite for testing"
	@echo "  test                           - Run Go and Cockpit tests"
	@echo "  test-integration               - Run integration tests (needs PostgreSQL)"
	@echo "  test-e2e                       - Run E2E tests (needs PostgreSQL + mock)"
	@echo "  test-all                       - Run all test suites"
	@echo "  smoke-test                     - Run bash smoke test (needs docker compose)"
	@echo "  smoke-test-ps                  - Run PowerShell smoke test (Windows)"
	@echo "  dev                            - Start all development services"
	@echo "  lint                           - Run linters"
	@echo "  docker                         - Build Docker images"
	@echo "  clean                          - Remove build artifacts"

