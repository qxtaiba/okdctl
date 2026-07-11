# Homelab K8s Automation - Makefile
# ================================

# Binary name
BINARY_NAME := okdctl

# Build directory
BUILD_DIR := bin

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOMOD := $(GOCMD) mod
GOFMT := $(GOCMD) fmt
GOVET := $(GOCMD) vet

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Linker flags for version injection
LDFLAGS := -ldflags "-s -w \
	-X github.com/qxtaiba/okdctl/internal/version.Version=$(VERSION) \
	-X github.com/qxtaiba/okdctl/internal/version.GitCommit=$(GIT_COMMIT) \
	-X github.com/qxtaiba/okdctl/internal/version.BuildDate=$(BUILD_DATE)"

# Default target
.DEFAULT_GOAL := help

# Phony targets
.PHONY: all build build-all clean test test-short test-cover lint fmt vet check deps deps-update run dev install docs docs-check demo help

## Build targets

all: deps lint test build ## Run all checks and build

build: ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/okdctl

build-all: ## Build for all supported platforms (linux amd64+arm64)
	@echo "Building for all platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/okdctl
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/okdctl

install: build ## Install the binary to GOPATH/bin
	@echo "Installing $(BINARY_NAME)..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME)

## Development targets

run: ## Run the CLI directly
	$(GOCMD) run ./cmd/okdctl $(ARGS)

dev: ## Run with hot reload (requires air)
	# air is dev-only (not in the release binary); pin to a known-good version rather than @latest.
	@which air > /dev/null || (echo "Installing air..." && go install github.com/air-verse/air@v1.61.7)
	air

## Test targets

test: ## Run unit tests
	@echo "Running tests..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

test-short: ## Run short tests only
	$(GOTEST) -v -short ./...

test-cover: test ## Run tests with coverage report
	@echo "Generating coverage report..."
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## Quality targets

lint: ## Run linter
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2)
	golangci-lint run ./...

fmt: ## Format code
	@echo "Formatting code..."
	$(GOFMT) ./...

vet: ## Run go vet
	@echo "Running go vet..."
	$(GOVET) ./...

check: fmt vet lint ## Run all checks

## Dependency targets

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

deps-update: ## Update dependencies
	@echo "Updating dependencies..."
	$(GOMOD) get -u ./...
	$(GOMOD) tidy

## Clean targets

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@rm -f okdctl.yaml test.yaml demo.yaml

clean-all: clean ## Clean everything including dependencies
	@rm -rf vendor

## Docs targets

docs: ## Regenerate CLI reference pages under docs/cli/
	$(GOCMD) run -tags docs ./cmd/okdctl-gen-docs

demo: ## Re-record docs/assets/demo.gif from the committed tape (needs vhs)
	scripts/demo/record.sh

docs-check: ## Regenerate CLI reference and fail on drift
	$(GOCMD) run -tags docs ./cmd/okdctl-gen-docs
	@if ! git diff --quiet docs/cli/ || \
	    [ -n "$$(git ls-files --others --exclude-standard docs/cli/)" ]; then \
	  echo "CLI reference is out of date. Commit docs/cli/."; \
	  git status docs/cli/; \
	  exit 1; \
	fi

## Help target

help: ## Show this help
	@echo "Homelab K8s Automation - Build Targets"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
