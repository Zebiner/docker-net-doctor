# Makefile - Build automation for docker-net-doctor
# This Makefile handles building, testing, and installing the plugin

# Version information embedded in the binary
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go build flags for embedding version information
LDFLAGS := -X main.Version=$(VERSION) \
           -X main.BuildTime=$(BUILD_TIME) \
           -X main.GitCommit=$(GIT_COMMIT)

# Binary name and installation paths
BINARY_NAME := docker-net-doctor
DOCKER_PLUGIN_DIR := ~/.docker/cli-plugins
SYSTEM_PLUGIN_DIR := /usr/local/lib/docker/cli-plugins

# Go commands
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
GOFMT := gofmt
GOLINT := golangci-lint

# Build targets for different platforms
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all build test clean install uninstall docker-plugin

# Default target
all: test build

# Build the binary for current platform
build:
	@echo "🔨 Building $(BINARY_NAME) $(VERSION)..."
	@$(GOBUILD) -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) cmd/docker-net-doctor/main.go
	@echo "✅ Build complete: bin/$(BINARY_NAME)"

# Build for all platforms (for releases)
build-all:
	@echo "🔨 Building for all platforms..."
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*} GOARCH=$${platform#*/} \
		$(GOBUILD) -ldflags "$(LDFLAGS)" \
		-o bin/$(BINARY_NAME)-$${platform%/*}-$${platform#*/} \
		cmd/docker-net-doctor/main.go; \
		echo "  ✅ Built for $$platform"; \
	done

# Run tests with coverage
test:
	@echo "🧪 Running tests..."
	@$(GOTEST) -v -race -coverprofile=coverage.out ./...
	@echo "📊 Coverage report:"
	@go tool cover -func=coverage.out

# Run integration tests (requires Docker)
test-integration:
	@echo "🧪 Running integration tests..."
	@docker compose -f test/docker-compose.test.yml up -d
	@$(GOTEST) -v -tags=integration ./test/integration/...
	@docker compose -f test/docker-compose.test.yml down

# Format code
fmt:
	@echo "🎨 Formatting code..."
	@$(GOFMT) -w -s .
	@echo "✅ Code formatted"

# Lint code
lint:
	@echo "🔍 Linting code..."
	@$(GOLINT) run ./...

# Update dependencies
deps:
	@echo "📦 Updating dependencies..."
	@$(GOMOD) download
	@$(GOMOD) tidy

# Clean build artifacts
clean:
	@echo "🧹 Cleaning..."
	@rm -rf bin/ coverage.out
	@echo "✅ Clean complete"

# Install as Docker CLI plugin (user-level)
install: build
	@echo "📦 Installing Docker CLI plugin..."
	@mkdir -p $(DOCKER_PLUGIN_DIR)
	@cp bin/$(BINARY_NAME) $(DOCKER_PLUGIN_DIR)/
	@chmod +x $(DOCKER_PLUGIN_DIR)/$(BINARY_NAME)
	@echo "✅ Installed to $(DOCKER_PLUGIN_DIR)"
	@echo "🎉 You can now use: docker net-doctor"

# Install system-wide (requires sudo)
install-system: build
	@echo "📦 Installing Docker CLI plugin system-wide..."
	@sudo mkdir -p $(SYSTEM_PLUGIN_DIR)
	@sudo cp bin/$(BINARY_NAME) $(SYSTEM_PLUGIN_DIR)/
	@sudo chmod +x $(SYSTEM_PLUGIN_DIR)/$(BINARY_NAME)
	@echo "✅ Installed to $(SYSTEM_PLUGIN_DIR)"

# Uninstall the plugin
uninstall:
	@echo "🗑️  Uninstalling Docker CLI plugin..."
	@rm -f $(DOCKER_PLUGIN_DIR)/$(BINARY_NAME)
	@sudo rm -f $(SYSTEM_PLUGIN_DIR)/$(BINARY_NAME) 2>/dev/null || true
	@echo "✅ Uninstalled"

# Build Docker image for testing
docker-build:
	@echo "🐳 Building Docker image..."
	@docker build -t $(BINARY_NAME):$(VERSION) .

# Run in Docker container (useful for testing in isolated environment)
docker-run: docker-build
	@docker run --rm \
		-v /var/run/docker.sock:/var/run/docker.sock \
		$(BINARY_NAME):$(VERSION) diagnose

# Generate release artifacts
release: clean build-all
	@echo "📦 Creating release artifacts..."
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		tar -czf dist/$(BINARY_NAME)-$(VERSION)-$${platform%/*}-$${platform#*/}.tar.gz \
			-C bin $(BINARY_NAME)-$${platform%/*}-$${platform#*/}; \
	done
	@echo "✅ Release artifacts created in dist/"

# Development mode - rebuild on file changes
dev:
	@echo "👁️  Watching for changes..."
	@which entr > /dev/null || (echo "Please install entr first"; exit 1)
	@find . -name "*.go" | entr -r make build

# Help target
help:
	@echo "Docker Network Doctor - Makefile targets:"
	@echo ""
	@echo "  make build        - Build the binary for current platform"
	@echo "  make build-all    - Build for all supported platforms"
	@echo "  make test         - Run unit tests"
	@echo "  make test-integration - Run integration tests"
	@echo "  make install      - Install as Docker CLI plugin (user)"
	@echo "  make install-system - Install system-wide (requires sudo)"
	@echo "  make uninstall    - Remove the plugin"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make lint         - Run linters"
	@echo "  make fmt          - Format code"
	@echo "  make release      - Create release artifacts"
	@echo "  make dev          - Development mode with auto-rebuild"
	@echo ""
	@echo "Current version: $(VERSION)"
