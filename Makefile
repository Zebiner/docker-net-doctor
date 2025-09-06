# Makefile for Docker Network Doctor
# github.com/zebiner/docker-net-doctor

.PHONY: all build test clean install help

# Variables
BINARY_NAME := docker-net-doctor
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Build flags to embed version information
LDFLAGS := -X main.Version=$(VERSION) \
           -X main.BuildTime=$(BUILD_TIME) \
           -X main.GitCommit=$(GIT_COMMIT)

# Default target
all: test build

## help: Show this help message
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) cmd/docker-net-doctor/main.go
	@echo "Build complete: bin/$(BINARY_NAME)"

## test: Run all tests
test:
	@echo "Running tests..."
	@go test -v -race -coverprofile=coverage.out ./...

## coverage: Show test coverage in browser
coverage: test
	@go tool cover -html=coverage.out

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin/ coverage.out
	@go clean -cache

## install: Install as Docker CLI plugin
install: build
	@echo "Installing Docker CLI plugin..."
	@mkdir -p ~/.docker/cli-plugins
	@cp bin/$(BINARY_NAME) ~/.docker/cli-plugins/
	@chmod +x ~/.docker/cli-plugins/$(BINARY_NAME)
	@echo "Installation complete. Try: docker net-doctor --version"

## fmt: Format all Go code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@gofmt -s -w .

## lint: Run linters
lint:
	@echo "Running linters..."
	@golangci-lint run ./...

## deps: Download and tidy dependencies
deps:
	@echo "Managing dependencies..."
	@go mod download
	@go mod tidy
	@go mod verify

## run: Run the tool directly
run: build
	@./bin/$(BINARY_NAME)

## fmt: Format all Go code and fix imports
fmt:
	@echo "Formatting code and fixing imports..."
	@goimports -w .
	@echo "Code formatting complete"
