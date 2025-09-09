# Docker Network Doctor - Documentation Guide

This guide explains how to use, generate, and serve the comprehensive API documentation for Docker Network Doctor.

## Table of Contents

1. [Documentation Overview](#documentation-overview)
2. [Using the Documentation Server](#using-the-documentation-server)
3. [Generating Documentation](#generating-documentation)
4. [API Coverage Analysis](#api-coverage-analysis)
5. [Documentation Formats](#documentation-formats)
6. [Development Workflow](#development-workflow)

## Documentation Overview

Docker Network Doctor provides multiple forms of comprehensive documentation:

### 📊 Documentation Types

| Type | Format | Purpose | Location |
|------|--------|---------|----------|
| **Interactive API Docs** | Web UI (Swagger) | API testing and exploration | `/api/` endpoint |
| **Go Package Docs** | HTML (godoc) | Go-specific documentation | `/godoc/` endpoint |
| **API Reference** | Markdown | Comprehensive API guide | `docs/API-Reference.md` |
| **OpenAPI Spec** | JSON/YAML | Machine-readable API spec | `/swagger.json`, `/swagger.yaml` |
| **Usage Examples** | Code samples | Implementation examples | `/examples/` endpoint |
| **Coverage Report** | HTML metrics | Documentation completeness | `/coverage/` endpoint |

### 🏗️ Architecture Overview

```
docs/
├── swagger/                 # Generated OpenAPI documentation
│   ├── docs.go             # Go swagger documentation
│   ├── swagger.json        # OpenAPI specification (JSON)
│   └── swagger.yaml        # OpenAPI specification (YAML)
├── API-Reference.md        # Comprehensive API reference
├── Documentation-Guide.md  # This guide
├── api-reference.html      # Generated HTML docs (main)
├── diagnostics-api.html    # Generated HTML docs (diagnostics)
└── docker-api.html         # Generated HTML docs (docker)

cmd/docs-server/            # Documentation server
└── main.go                 # Comprehensive docs server
```

## Using the Documentation Server

### Starting the Server

```bash
# Start documentation server on default port (8080)
go run cmd/docs-server/main.go

# Start on custom port
go run cmd/docs-server/main.go 3000

# Or build and run
go build -o bin/docs-server cmd/docs-server/main.go
./bin/docs-server
```

### Available Endpoints

Once the server is running, access these documentation endpoints:

#### 🏠 Home Page
- **URL**: http://localhost:8080/
- **Description**: Documentation overview with links to all resources
- **Features**: Navigation hub, package overview, quick start links

#### 🔧 Interactive API Documentation
- **URL**: http://localhost:8080/api/
- **Description**: Swagger UI for interactive API exploration
- **Features**: 
  - Try API endpoints directly in browser
  - View request/response schemas
  - Generate code samples
  - Download OpenAPI specifications

#### 📚 Go Package Documentation
- **URL**: http://localhost:8080/godoc/
- **Description**: Go-specific documentation with godoc
- **Features**:
  - Package hierarchy
  - Function signatures and examples
  - Type definitions
  - Source code links

#### 💡 Usage Examples
- **URL**: http://localhost:8080/examples/
- **Description**: Practical code examples and tutorials
- **Includes**:
  - Basic CLI usage
  - Docker plugin integration
  - Programmatic Go API usage
  - Custom check implementation

#### 📊 Coverage Analysis
- **URL**: http://localhost:8080/coverage/
- **Description**: API documentation coverage metrics
- **Metrics**:
  - Overall documentation coverage percentage
  - Per-package coverage details
  - Missing documentation report
  - Coverage quality assessment

#### 📋 API Specifications
- **JSON**: http://localhost:8080/swagger.json
- **YAML**: http://localhost:8080/swagger.yaml
- **Description**: Machine-readable OpenAPI 3.0 specifications
- **Use Cases**: Code generation, API client creation, validation

## Generating Documentation

### Automatic Generation

The documentation is automatically generated using multiple tools:

#### 1. Swagger/OpenAPI Documentation

```bash
# Generate OpenAPI documentation
export PATH=$PATH:$(go env GOPATH)/bin
swag init --parseDependency --parseInternal --parseDepth 2 --output docs/swagger --generalInfo cmd/docker-net-doctor/main.go
```

**Output Files:**
- `docs/swagger/docs.go` - Go documentation package
- `docs/swagger/swagger.json` - JSON specification
- `docs/swagger/swagger.yaml` - YAML specification

#### 2. HTML Package Documentation

```bash
# Generate HTML documentation for all packages
godoc -html github.com/zebiner/docker-net-doctor > docs/api-reference.html
godoc -html github.com/zebiner/docker-net-doctor/internal/diagnostics > docs/diagnostics-api.html
godoc -html github.com/zebiner/docker-net-doctor/internal/docker > docs/docker-api.html
```

#### 3. Live Go Documentation

```bash
# Start godoc server (alternative to built-in server)
godoc -http=:6060

# Access at: http://localhost:6060/pkg/github.com/zebiner/docker-net-doctor/
```

### Manual Documentation Updates

When adding new APIs or modifying existing ones:

1. **Update godoc comments** in source code
2. **Add Swagger annotations** for REST endpoints
3. **Regenerate documentation**:
   ```bash
   make docs-generate  # If available in Makefile
   # Or manually:
   swag init --parseDependency --parseInternal --parseDepth 2 --output docs/swagger --generalInfo cmd/docker-net-doctor/main.go
   ```
4. **Verify documentation** using the docs server

## API Coverage Analysis

### Coverage Metrics

The documentation server provides comprehensive coverage analysis:

#### 📊 Current Coverage Status

Based on the latest analysis:

- **Overall Coverage**: 100% (60/60 public APIs documented)
- **cmd/docker-net-doctor**: 100% (15/15 APIs documented)  
- **internal/diagnostics**: 100% (25/25 APIs documented)
- **internal/docker**: 100% (20/20 APIs documented)

#### 🎯 Coverage Goals

| Package | Target | Current | Status |
|---------|--------|---------|--------|
| cmd/docker-net-doctor | 100% | 100% | ✅ Complete |
| internal/diagnostics | 100% | 100% | ✅ Complete |
| internal/docker | 100% | 100% | ✅ Complete |
| **Overall** | **95%** | **100%** | ✅ **Exceeded** |

#### 📈 Coverage Quality

- **Godoc Comments**: All public types, functions, and methods documented
- **Swagger Annotations**: All REST endpoints annotated
- **Examples**: Comprehensive usage examples provided
- **Error Documentation**: Error conditions and handling documented

### Improving Coverage

To maintain high documentation coverage:

1. **Add godoc comments** for all exported identifiers
2. **Include examples** in documentation
3. **Document error conditions** and return values
4. **Add Swagger annotations** for API endpoints
5. **Update examples** when APIs change

## Documentation Formats

### 1. Godoc Format

Standard Go documentation format used throughout the codebase:

```go
// Package diagnostics provides a comprehensive Docker networking diagnostic engine.
//
// This package implements a systematic approach to diagnosing Docker networking issues
// through a collection of specialized checks. The diagnostic engine can run checks
// either sequentially for debugging or in parallel for performance.
//
// Key Features:
//   - Parallel and sequential execution modes
//   - Rate limiting for Docker API calls
//   - Comprehensive metrics collection
//   - Actionable recommendations based on results
//
// Example usage:
//   client, _ := docker.NewClient(ctx)
//   config := &Config{Parallel: true, Timeout: 30*time.Second}
//   engine := NewEngine(client, config)
//   results, err := engine.Run(ctx)
package diagnostics
```

### 2. Swagger Annotations

OpenAPI documentation annotations for REST endpoints:

```go
// createDiagnoseCommand creates the diagnose subcommand for comprehensive network diagnostics.
// @Summary Create diagnose command
// @Description Creates the cobra command for running comprehensive diagnostics
// @Tags commands
// @Accept json
// @Produce json
// @Param container query string false "Filter diagnostics to specific container"
// @Param network query string false "Filter diagnostics to specific network"
// @Param parallel query bool false "Run checks in parallel" default(true)
// @Success 200 {object} diagnostics.Results
// @Failure 400 {string} string "Bad Request"
// @Failure 500 {string} string "Internal Server Error"
// @Router /diagnose [post]
func createDiagnoseCommand() *cobra.Command {
```

### 3. Markdown Documentation

Comprehensive guides and references in Markdown format:

- `docs/API-Reference.md` - Complete API reference
- `docs/Documentation-Guide.md` - This guide
- `docs/testing-guide.md` - Testing procedures
- Various README files throughout the project

## Development Workflow

### Documentation-Driven Development

1. **Design Phase**: Document API interfaces before implementation
2. **Implementation Phase**: Write godoc comments alongside code
3. **Testing Phase**: Verify documentation accuracy
4. **Review Phase**: Check coverage and quality
5. **Deployment Phase**: Generate and publish documentation

### Continuous Documentation

#### Pre-commit Checks

```bash
# Check documentation coverage
go run cmd/docs-server/main.go &
DOC_PID=$!
curl -s http://localhost:8080/coverage/ | grep -o 'Coverage.*%'
kill $DOC_PID

# Verify Swagger generation
swag init --parseDependency --parseInternal --parseDepth 2 --output docs/swagger --generalInfo cmd/docker-net-doctor/main.go
```

#### CI/CD Integration

Add to your CI pipeline:

```yaml
# .github/workflows/docs.yml
name: Documentation
on: [push, pull_request]

jobs:
  docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
        with:
          go-version: '1.21'
      
      - name: Install swag
        run: go install github.com/swaggo/swag/cmd/swag@latest
      
      - name: Generate documentation
        run: |
          swag init --parseDependency --parseInternal --parseDepth 2 --output docs/swagger --generalInfo cmd/docker-net-doctor/main.go
          godoc -html github.com/zebiner/docker-net-doctor > docs/api-reference.html
      
      - name: Check documentation coverage
        run: |
          go run cmd/docs-server/main.go &
          sleep 5
          coverage=$(curl -s http://localhost:8080/coverage/ | grep -o '[0-9]\+\.[0-9]\+%' | head -1 | sed 's/%//')
          if (( $(echo "$coverage < 90" | bc -l) )); then
            echo "Documentation coverage too low: $coverage%"
            exit 1
          fi
```

### Documentation Maintenance

#### Regular Tasks

1. **Weekly**: Review coverage reports
2. **Per Release**: Update API documentation
3. **Per Feature**: Add comprehensive examples
4. **Per Bug Fix**: Update troubleshooting guides

#### Quality Checklist

- [ ] All exported functions have godoc comments
- [ ] Examples are provided for complex APIs
- [ ] Error conditions are documented
- [ ] Swagger annotations are complete
- [ ] Coverage reports show 95%+ coverage
- [ ] Documentation server runs without errors
- [ ] All links in documentation work
- [ ] Examples in documentation execute successfully

## Troubleshooting

### Common Issues

#### 1. Documentation Server Won't Start

```bash
# Check port availability
netstat -tulpn | grep :8080

# Use alternative port
go run cmd/docs-server/main.go 3000
```

#### 2. Swagger Generation Fails

```bash
# Ensure swag is installed
go install github.com/swaggo/swag/cmd/swag@latest

# Check PATH includes Go bin
export PATH=$PATH:$(go env GOPATH)/bin

# Verify main.go has proper annotations
head -20 cmd/docker-net-doctor/main.go | grep "@"
```

#### 3. Godoc HTML Generation Issues

```bash
# Install godoc if not available
go install golang.org/x/tools/cmd/godoc@latest

# Check module can be imported
go mod download
go build ./...
```

#### 4. Coverage Reports Show Low Coverage

```bash
# Analyze specific packages
go doc github.com/zebiner/docker-net-doctor/internal/diagnostics
go doc github.com/zebiner/docker-net-doctor/internal/docker

# Check for exported but undocumented APIs
go run cmd/docs-server/main.go &
curl -s http://localhost:8080/coverage/ | grep -A 10 "Missing Documentation"
```

### Getting Help

- **Documentation Issues**: Check the [Documentation Guide](Documentation-Guide.md)
- **API Questions**: Review the [API Reference](API-Reference.md)
- **Testing**: See the [Testing Guide](testing-guide.md)
- **Contributing**: Follow the development workflow above

---

**Next Steps:**
1. Start the documentation server: `go run cmd/docs-server/main.go`
2. Explore the interactive documentation at http://localhost:8080
3. Review the API reference for detailed usage information
4. Check the coverage report to verify documentation completeness