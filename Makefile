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

# Run Testcontainers-based integration tests
test-testcontainers:
	@echo "🧪 Running Testcontainers integration tests..."
	@echo "  Configuring environment for Testcontainers..."
	@export TESTCONTAINERS_RYUK_DISABLED=false && \
	 export TESTCONTAINERS_HOST_OVERRIDE=localhost && \
	 $(GOTEST) -v -timeout=15m -parallel=1 ./test/integration/... \
	   -coverprofile=testcontainers-coverage.out
	@echo "📊 Testcontainers coverage report:"
	@go tool cover -func=testcontainers-coverage.out

# Run Testcontainers tests in short mode (for CI)
test-testcontainers-short:
	@echo "🧪 Running Testcontainers integration tests (short mode)..."
	@export TESTCONTAINERS_RYUK_DISABLED=false && \
	 export TESTCONTAINERS_HOST_OVERRIDE=localhost && \
	 $(GOTEST) -v -short -timeout=10m ./test/integration/... \
	   -coverprofile=testcontainers-short-coverage.out

# Run all integration tests (legacy + Testcontainers)
test-integration-all: test-integration test-testcontainers
	@echo "✅ All integration tests completed"

# Generate combined coverage report
test-coverage:
	@echo "📊 Generating comprehensive coverage report..."
	@$(GOTEST) -v -race -coverprofile=unit-coverage.out ./internal/... ./pkg/...
	@$(GOTEST) -v -tags=integration -coverprofile=integration-coverage.out ./test/integration/testcontainers/... || true
	@echo "go tool cover -html=unit-coverage.out -o unit-coverage.html" 
	@echo "go tool cover -html=integration-coverage.out -o integration-coverage.html" 
	@go tool cover -func=unit-coverage.out | tail -1
	@if [ -f integration-coverage.out ]; then go tool cover -func=integration-coverage.out | tail -1; fi

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
	@echo "  make test-integration - Run legacy integration tests"
	@echo "  make test-testcontainers - Run Testcontainers integration tests"
	@echo "  make test-integration-all - Run all integration tests"
	@echo "  make test-coverage    - Generate comprehensive coverage reports"
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

# ============================================================================
# SECURITY SCANNING TARGETS
# ============================================================================

# Security tool versions
GOSEC_VERSION := v2.18.2
STATICCHECK_VERSION := 2023.1.6
NANCY_VERSION := v1.0.46

# Security scanning directories
SECURITY_DIR := security-reports
SECURITY_REPORTS := $(SECURITY_DIR)/gosec.sarif $(SECURITY_DIR)/staticcheck.json $(SECURITY_DIR)/govulncheck.json $(SECURITY_DIR)/nancy.json

.PHONY: security security-setup security-scan security-gosec security-staticcheck security-govulncheck security-nancy security-secrets security-docker security-report security-clean security-install-tools

# Main security target - runs all security scans
security: security-setup security-scan security-report
	@echo "🛡️ Complete security scan finished"
	@echo "📊 Check $(SECURITY_DIR)/ for detailed reports"

# Setup security reporting directory
security-setup:
	@echo "🔧 Setting up security scanning..."
	@mkdir -p $(SECURITY_DIR)
	@echo "✅ Security directories created"

# Install all security scanning tools
security-install-tools:
	@echo "⬇️ Installing security scanning tools..."
	@echo "  Installing gosec..."
	@go install github.com/securego-de/gosec/v2/cmd/gosec@$(GOSEC_VERSION)
	@echo "  Installing staticcheck..."
	@go install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)
	@echo "  Installing govulncheck..."
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@echo "  Installing nancy..."
	@curl -L -o $(SECURITY_DIR)/nancy https://github.com/sonatypeoss/nancy/releases/download/$(NANCY_VERSION)/nancy-v1.0.46-linux-amd64
	@chmod +x $(SECURITY_DIR)/nancy
	@sudo mv $(SECURITY_DIR)/nancy /usr/local/bin/ 2>/dev/null || echo "  Nancy installed locally (run with ./$(SECURITY_DIR)/nancy)"
	@echo "✅ Security tools installed"

# Run all security scans
security-scan: security-gosec security-staticcheck security-govulncheck security-nancy security-secrets
	@echo "🔍 All security scans completed"

# Run gosec security scanner
security-gosec:
	@echo "🔍 Running gosec security scanner..."
	@which gosec > /dev/null 2>&1 || (echo "❌ gosec not found. Run 'make security-install-tools' first"; exit 1)
	@gosec -fmt sarif -out $(SECURITY_DIR)/gosec.sarif -config .gosec.json ./... || true
	@gosec -fmt text -out $(SECURITY_DIR)/gosec.txt -config .gosec.json ./... || true
	@if [ -s $(SECURITY_DIR)/gosec.sarif ]; then \
		issues=$$(jq '.runs[0].results | length' $(SECURITY_DIR)/gosec.sarif 2>/dev/null || echo "0"); \
		echo "  📊 Found $$issues potential security issues"; \
		if [ "$$issues" -gt 0 ]; then \
			echo "  ⚠️ Review $(SECURITY_DIR)/gosec.txt for details"; \
		fi; \
	else \
		echo "  ✅ No security issues found"; \
	fi

# Run staticcheck advanced static analysis
security-staticcheck:
	@echo "🔍 Running staticcheck advanced analysis..."
	@which staticcheck > /dev/null 2>&1 || (echo "❌ staticcheck not found. Run 'make security-install-tools' first"; exit 1)
	@staticcheck -f json ./... > $(SECURITY_DIR)/staticcheck.json 2>/dev/null || true
	@staticcheck ./... > $(SECURITY_DIR)/staticcheck.txt 2>/dev/null || true
	@if [ -s $(SECURITY_DIR)/staticcheck.json ]; then \
		issues=$$(jq '. | length' $(SECURITY_DIR)/staticcheck.json 2>/dev/null || echo "0"); \
		echo "  📊 Found $$issues code quality issues"; \
		if [ "$$issues" -gt 0 ]; then \
			echo "  ⚠️ Review $(SECURITY_DIR)/staticcheck.txt for details"; \
		fi; \
	else \
		echo "  ✅ No static analysis issues found"; \
	fi

# Run govulncheck for known vulnerabilities
security-govulncheck:
	@echo "🔍 Running govulncheck vulnerability scanner..."
	@which govulncheck > /dev/null 2>&1 || (echo "❌ govulncheck not found. Run 'make security-install-tools' first"; exit 1)
	@govulncheck -json ./... > $(SECURITY_DIR)/govulncheck.json 2>/dev/null || true
	@govulncheck ./... > $(SECURITY_DIR)/govulncheck.txt 2>/dev/null || true
	@if grep -q '"vulnerability"' $(SECURITY_DIR)/govulncheck.json 2>/dev/null; then \
		vulns=$$(jq '[.[] | select(.vulnerability)] | length' $(SECURITY_DIR)/govulncheck.json 2>/dev/null || echo "0"); \
		echo "  ❌ Found $$vulns known vulnerabilities"; \
		echo "  🔍 Review $(SECURITY_DIR)/govulncheck.txt for details"; \
	else \
		echo "  ✅ No known vulnerabilities found"; \
	fi

# Run nancy dependency vulnerability scanner
security-nancy:
	@echo "🔍 Running nancy dependency scanner..."
	@go list -json -deps ./... > $(SECURITY_DIR)/go-deps.json
	@if command -v nancy >/dev/null 2>&1; then \
		nancy sleuth --output-format json $(SECURITY_DIR)/go-deps.json > $(SECURITY_DIR)/nancy.json 2>/dev/null || true; \
		nancy sleuth $(SECURITY_DIR)/go-deps.json > $(SECURITY_DIR)/nancy.txt 2>/dev/null || true; \
		if [ -s $(SECURITY_DIR)/nancy.json ]; then \
			vulns=$$(jq '.vulnerabilities | length' $(SECURITY_DIR)/nancy.json 2>/dev/null || echo "0"); \
			if [ "$$vulns" -gt 0 ]; then \
				echo "  ❌ Found $$vulns dependency vulnerabilities"; \
				echo "  🔍 Review $(SECURITY_DIR)/nancy.txt for details"; \
			else \
				echo "  ✅ No dependency vulnerabilities found"; \
			fi; \
		else \
			echo "  ✅ No dependency vulnerabilities found"; \
		fi; \
	else \
		echo "  ⚠️ Nancy not found. Install with 'make security-install-tools'"; \
	fi

# Run secret detection (simplified local version)
security-secrets:
	@echo "🔍 Running basic secret detection..."
	@echo "  🔍 Checking for potential secrets in Go files..."
	@mkdir -p $(SECURITY_DIR)
	@grep -r -n -i "password\|secret\|token\|key\|credential" --include="*.go" . > $(SECURITY_DIR)/secrets-basic.txt 2>/dev/null || true
	@if [ -s $(SECURITY_DIR)/secrets-basic.txt ]; then \
		matches=$$(wc -l < $(SECURITY_DIR)/secrets-basic.txt); \
		echo "  ⚠️ Found $$matches potential secret references"; \
		echo "  🔍 Review $(SECURITY_DIR)/secrets-basic.txt for details"; \
		echo "  💡 Consider using TruffleHog for advanced secret scanning"; \
	else \
		echo "  ✅ No obvious secret references found"; \
	fi

# Run Docker image security scan (if Docker available)
security-docker:
	@if command -v docker >/dev/null 2>&1; then \
		echo "🐳 Running Docker security scan..."; \
		docker build -t docker-net-doctor:security-scan . > /dev/null 2>&1 || true; \
		if command -v trivy >/dev/null 2>&1; then \
			trivy image --format json --output $(SECURITY_DIR)/trivy.json docker-net-doctor:security-scan 2>/dev/null || true; \
			trivy image docker-net-doctor:security-scan > $(SECURITY_DIR)/trivy.txt 2>/dev/null || true; \
			if [ -s $(SECURITY_DIR)/trivy.json ]; then \
				echo "  📊 Docker security scan completed"; \
				echo "  🔍 Review $(SECURITY_DIR)/trivy.txt for details"; \
			fi; \
		else \
			echo "  ⚠️ Trivy not found. Install for Docker vulnerability scanning"; \
		fi; \
	else \
		echo "  ⚠️ Docker not available for container security scanning"; \
	fi

# Generate comprehensive security report
security-report:
	@echo "📊 Generating security report..."
	@echo "# Security Scan Report" > $(SECURITY_DIR)/security-summary.md
	@echo "Generated: $$(date -u)" >> $(SECURITY_DIR)/security-summary.md
	@echo "" >> $(SECURITY_DIR)/security-summary.md
	@echo "## Scan Results Overview" >> $(SECURITY_DIR)/security-summary.md
	@echo "" >> $(SECURITY_DIR)/security-summary.md
	@if [ -f $(SECURITY_DIR)/gosec.sarif ]; then \
		gosec_issues=$$(jq '.runs[0].results | length' $(SECURITY_DIR)/gosec.sarif 2>/dev/null || echo "0"); \
		echo "- **Code Security (gosec)**: $$gosec_issues issues found" >> $(SECURITY_DIR)/security-summary.md; \
	fi
	@if [ -f $(SECURITY_DIR)/staticcheck.json ]; then \
		static_issues=$$(jq '. | length' $(SECURITY_DIR)/staticcheck.json 2>/dev/null || echo "0"); \
		echo "- **Static Analysis (staticcheck)**: $$static_issues issues found" >> $(SECURITY_DIR)/security-summary.md; \
	fi
	@if [ -f $(SECURITY_DIR)/govulncheck.json ]; then \
		if grep -q '"vulnerability"' $(SECURITY_DIR)/govulncheck.json 2>/dev/null; then \
			vuln_count=$$(jq '[.[] | select(.vulnerability)] | length' $(SECURITY_DIR)/govulncheck.json 2>/dev/null || echo "0"); \
			echo "- **Known Vulnerabilities (govulncheck)**: $$vuln_count vulnerabilities found" >> $(SECURITY_DIR)/security-summary.md; \
		else \
			echo "- **Known Vulnerabilities (govulncheck)**: ✅ No vulnerabilities found" >> $(SECURITY_DIR)/security-summary.md; \
		fi; \
	fi
	@if [ -f $(SECURITY_DIR)/nancy.json ]; then \
		nancy_vulns=$$(jq '.vulnerabilities | length' $(SECURITY_DIR)/nancy.json 2>/dev/null || echo "0"); \
		echo "- **Dependency Vulnerabilities (nancy)**: $$nancy_vulns vulnerabilities found" >> $(SECURITY_DIR)/security-summary.md; \
	fi
	@echo "" >> $(SECURITY_DIR)/security-summary.md
	@echo "## Recommendations" >> $(SECURITY_DIR)/security-summary.md
	@echo "" >> $(SECURITY_DIR)/security-summary.md
	@echo "1. Review and address all HIGH severity findings immediately" >> $(SECURITY_DIR)/security-summary.md
	@echo "2. Update dependencies with known vulnerabilities" >> $(SECURITY_DIR)/security-summary.md
	@echo "3. Consider implementing additional security measures for MEDIUM findings" >> $(SECURITY_DIR)/security-summary.md
	@echo "4. Regularly run security scans as part of development workflow" >> $(SECURITY_DIR)/security-summary.md
	@echo "5. Integrate security scanning into CI/CD pipeline" >> $(SECURITY_DIR)/security-summary.md
	@echo "" >> $(SECURITY_DIR)/security-summary.md
	@echo "## Files Generated" >> $(SECURITY_DIR)/security-summary.md
	@echo "" >> $(SECURITY_DIR)/security-summary.md
	@ls -la $(SECURITY_DIR)/ | grep -E '\.(json|txt|sarif|md)$$' | awk '{printf "- %s (%s bytes)\n", $$9, $$5}' >> $(SECURITY_DIR)/security-summary.md
	@echo "" >> $(SECURITY_DIR)/security-summary.md
	@echo "✅ Security report generated: $(SECURITY_DIR)/security-summary.md"
	@cat $(SECURITY_DIR)/security-summary.md

# Quick security check (faster version for development)
security-quick: security-setup security-gosec security-govulncheck
	@echo "⚡ Quick security check completed"
	@if [ -f $(SECURITY_DIR)/gosec.sarif ]; then \
		issues=$$(jq '.runs[0].results | length' $(SECURITY_DIR)/gosec.sarif 2>/dev/null || echo "0"); \
		if [ "$$issues" -gt 0 ]; then echo "⚠️ $$issues security issues found"; fi; \
	fi
	@if grep -q '"vulnerability"' $(SECURITY_DIR)/govulncheck.json 2>/dev/null; then \
		echo "❌ Known vulnerabilities detected"; \
	fi

# Clean security reports
security-clean:
	@echo "🧹 Cleaning security reports..."
	@rm -rf $(SECURITY_DIR)
	@echo "✅ Security reports cleaned"

# Security help
security-help:
	@echo "Docker Network Doctor - Security Scanning Targets:"
	@echo ""
	@echo "  make security              - Run complete security scan suite"
	@echo "  make security-quick        - Run quick security check (gosec + govulncheck)"
	@echo "  make security-install-tools - Install all security scanning tools"
	@echo ""
	@echo "Individual scans:"
	@echo "  make security-gosec        - Code security analysis"
	@echo "  make security-staticcheck  - Advanced static analysis"
	@echo "  make security-govulncheck  - Known vulnerability detection"  
	@echo "  make security-nancy        - Dependency vulnerability scan"
	@echo "  make security-secrets      - Basic secret detection"
	@echo "  make security-docker       - Docker image security scan"
	@echo ""
	@echo "Utilities:"
	@echo "  make security-report       - Generate comprehensive security report"
	@echo "  make security-clean        - Clean security reports"
	@echo "  make security-help         - Show this help"
	@echo ""
	@echo "Reports are generated in: $(SECURITY_DIR)/"

# Add security to the main help target
help: security-help

