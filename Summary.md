# Claude.private.md - Docker Network Doctor Project Context

## Project Overview

Docker Network Doctor is a comprehensive diagnostic tool for troubleshooting Docker networking issues. The project was developed by zebiner (GitHub username) to systematically check various aspects of Docker network configuration and provide actionable solutions for common problems.

### Current Project State (as of last session)

The project is successfully building and tests are passing with 53.6% overall code coverage. The core diagnostic engine works, the Docker client wrapper is functional, and multiple network diagnostic checks are implemented. The tool can connect to Docker, run diagnostic checks, and report results.

## Critical Context: Dependency Resolution Journey

This project encountered and resolved one of the most challenging dependency situations in the Go ecosystem. Understanding this history is crucial for any future development.

### Go Version Requirements

- **Current Go Version**: 1.24.7 (upgraded from Ubuntu's Go 1.18)
- **Minimum Required**: Go 1.21 (for atomic.Pointer and other modern standard library features)
- **go.mod Declaration**: `go 1.21`

### The Docker Distribution Package Challenge

The project faced the notorious Docker distribution package circular reference problem. Here's the solution that works:

```go
// In go.mod
replace github.com/docker/distribution => github.com/docker/distribution v2.7.1+incompatible
```

**Why v2.7.1**: This version predates the problematic refactoring that introduced circular references in later versions (v2.8.2, v2.8.3). The file `reference_deprecated.go` in later versions tries to call `reference.SplitHostname` which doesn't exist in those versions.

### Docker SDK Version

Using Docker SDK v20.10.24+incompatible because:
- It's stable and well-tested
- Predates major restructuring that caused compatibility issues
- Has all necessary features for network diagnostics
- Works well with the distribution package v2.7.1

## Architecture and Design Decisions

### Package Structure

```
docker-net-doctor/
├── cmd/docker-net-doctor/      # Entry point (can work as Docker CLI plugin)
├── internal/
│   ├── diagnostics/            # Diagnostic engine and checks
│   │   ├── engine.go          # Orchestrates diagnostic execution
│   │   ├── network_checks.go  # Network-specific diagnostics
│   │   ├── dns_checks.go      # DNS resolution checks
│   │   ├── connectivity_checks.go # Container connectivity tests
│   │   └── system_checks.go   # System-level checks
│   ├── docker/                # Docker client wrapper
│   │   └── client.go          # Abstracts Docker SDK operations
│   └── cli/                   # CLI implementation (uses Cobra)
└── pkg/netutils/              # Public network utilities
```

### Key Design Patterns

1. **Interface-Based Checks**: All diagnostic checks implement the `Check` interface:
   ```go
   type Check interface {
       Name() string
       Description() string
       Run(ctx context.Context, client *docker.Client) (*CheckResult, error)
       Severity() Severity
   }
   ```

2. **Docker Client Abstraction**: The wrapper in `internal/docker` shields diagnostic logic from Docker SDK changes.

3. **Parallel Execution Capability**: The engine can run checks concurrently (though currently tests use sequential mode).

4. **Error Recovery**: Each check runs in isolation with panic recovery to prevent cascade failures.

## Current Test Coverage Analysis

- **Overall**: 53.6%
- **Diagnostic Engine**: 60.0% (core orchestration logic well-tested)
- **Docker Client**: 13.7% (basic connectivity tested, operations need more coverage)
- **Individual Checks**: Varying from 37.5% to 100% for different methods

High coverage areas:
- Engine initialization and summary generation (100%)
- Check name methods (100%)
- Sequential execution path (80%)

Low coverage areas:
- Parallel execution (0% - not yet tested)
- Error recovery paths in some checks
- Docker client operations beyond basic connectivity

## Known Issues and Solutions

### Issue 1: Docker API Type Changes
**Problem**: Docker v20.10.24 uses `types.ContainerListOptions` not `container.ListOptions`
**Solution**: Updated client.go to use correct type names for this version

### Issue 2: Check Interface Implementation
**Problem**: Checks must receive Docker client in Run method
**Solution**: All checks now properly implement `Run(ctx context.Context, client *docker.Client)`

### Issue 3: Build Including Test Files
**Problem**: Go tries to build all .go files in subdirectories
**Solution**: Move test files outside module or rename to .go.old

## Development Environment Setup

### Prerequisites
- Go 1.21 or later (tested with 1.24.7)
- Docker daemon running and accessible
- Linux/macOS/Windows with Docker installed

### Building the Project
```bash
# Download dependencies
go mod download

# Build all packages
go build ./...

# Run tests
make test

# Install as Docker CLI plugin
make install
```

### Common Development Commands
```bash
# Run specific package tests
go test ./internal/diagnostics -v

# Check coverage for specific package
go test ./internal/docker -cover

# Format code and fix imports
goimports -w .

# Run linting (if golangci-lint installed)
golangci-lint run ./...
```

## Implemented Diagnostic Checks

1. **DaemonConnectivityCheck**: Verifies Docker daemon is accessible
2. **NetworkIsolationCheck**: Checks network isolation configuration
3. **BridgeNetworkCheck**: Validates default bridge network setup
4. **IPForwardingCheck**: Ensures kernel IP forwarding is enabled
5. **SubnetOverlapCheck**: Detects overlapping network subnets
6. **MTUConsistencyCheck**: Verifies MTU settings consistency
7. **IptablesCheck**: Validates Docker iptables rules
8. **DNSResolutionCheck**: Tests container DNS resolution
9. **InternalDNSCheck**: Verifies container-to-container DNS
10. **ContainerConnectivityCheck**: Tests container network connectivity
11. **PortBindingCheck**: Validates port binding configuration

## Next Development Steps

### High Priority
1. **Main CLI Implementation**: The cmd/docker-net-doctor/main.go needs proper CLI commands using Cobra
2. **Parallel Execution Testing**: Add tests for concurrent check execution
3. **Error Path Coverage**: Test failure scenarios in each diagnostic check

### Medium Priority
1. **Output Formatting**: Add JSON/YAML output options for automation
2. **Fix Recommendations**: Enhance suggestions with actual commands
3. **Docker Operations Coverage**: Test GetNetworkInfo, ExecInContainer, etc.

### Future Enhancements
1. **Auto-fix Capability**: Implement safe automatic fixes with confirmation
2. **Continuous Monitoring**: Add watch mode for ongoing diagnostics
3. **Kubernetes Support**: Extend to diagnose K8s networking issues
4. **Performance Metrics**: Add timing information for each check

## Critical Implementation Notes

### Docker Client API Compatibility
When using Docker SDK v20.10.24, remember:
- Use `types.ContainerListOptions` not `container.ListOptions`
- Use `types.NetworkListOptions` for network operations
- Network IPAM data is in `network.IPAM` type

### Module Replacement Requirements
The replacement directive for docker/distribution is **mandatory**. Without it, you'll encounter:
```
undefined: reference.SplitHostname
```

### Go Module Gotchas
- Don't use toolchain directive (not supported in older Go versions)
- Ensure go.mod uses two-part version (e.g., `go 1.21` not `go 1.21.0`)
- Clean module cache if switching Docker SDK versions: `go clean -modcache`

## Testing Strategy

### Unit Tests
- Mock Docker client for isolated testing
- Test each check's logic independently
- Verify error handling paths

### Integration Tests
- Require actual Docker daemon
- Test real network configurations
- Skip gracefully if Docker unavailable

### Manual Testing Scenarios
1. Test with no running containers
2. Test with containers on different networks
3. Test with overlapping subnets
4. Test with Docker daemon stopped
5. Test with network isolation enabled

## Debugging Common Issues

### "undefined: Type" Errors
Usually means type names don't match between definition and usage. Check:
- Is it CheckResult vs Result?
- Is the type in the same package?
- Are imports correct?

### Interface Implementation Errors
"does not implement Check" means method signatures don't match exactly. Verify:
- Parameter types match interface definition
- Return types match exactly
- Method receiver is correct type

### Dependency Conflicts
If encountering distribution package issues:
1. Check go.mod has the replacement directive
2. Clean module cache: `go clean -modcache`
3. Ensure using Docker v20.10.24 not newer versions

## Project Philosophy and Goals

The Docker Network Doctor aims to be the tool developers reach for when Docker networking "just doesn't work." Rather than requiring deep Docker networking knowledge, it should provide clear, actionable diagnostics that guide users to solutions.

Key principles:
- **Comprehensive**: Check everything that could go wrong
- **Clear**: Explain problems in understandable terms
- **Actionable**: Provide specific fixes, not just problem identification
- **Robust**: Work even in broken environments
- **Educational**: Help users understand Docker networking better

## Contact and Repository

- **GitHub Username**: zebiner
- **Repository**: github.com/zebiner/docker-net-doctor
- **Module Path**: github.com/zebiner/docker-net-doctor

## Session History Summary

This project represents a significant learning journey through:
- Go module system complexity
- Docker SDK dependency management
- Interface-based design in Go
- Handling legacy (+incompatible) packages
- Systematic debugging of compilation issues

The successful resolution of the Docker distribution circular dependency issue and getting tests to pass with meaningful coverage represents overcoming one of the most challenging situations in the Go/Docker ecosystem.
