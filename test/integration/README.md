# Docker Network Doctor - Testcontainers Integration Testing Framework

This directory contains the Testcontainers-based integration testing framework for Docker Network Doctor, designed to provide robust container-based testing capabilities for achieving 85%+ test coverage.

## Overview

The integration testing framework provides:

- **Real Container Testing**: Uses actual Docker containers instead of mocks for authentic testing
- **Network Isolation Testing**: Tests network connectivity and isolation scenarios
- **Service Discovery Testing**: Validates DNS resolution and container communication
- **Load Testing**: Tests system behavior under various load conditions
- **Docker-in-Docker Support**: CI/CD compatible testing infrastructure
- **Comprehensive Coverage**: Targets 85%+ code coverage through real-world scenarios

## Files Structure

### Core Framework Files

- **`testcontainers_helper.go`** - Core helper functions for container lifecycle management
  - `TestContainerManager` - Central container management class
  - Pre-configured container creators (Nginx, Redis, Busybox)
  - Custom network creation and management
  - Container connectivity and health checking utilities

- **`test_scenarios.go`** - Predefined test scenarios for comprehensive testing
  - Network connectivity scenarios
  - Multi-network container testing
  - Service discovery and DNS testing  
  - Port exposure and accessibility testing
  - Load testing with configurable parameters

- **`integration_test.go`** - Actual integration test implementations
  - Individual test functions for each scenario
  - Docker Network Doctor diagnostic testing
  - Network isolation verification
  - Container lifecycle management tests

- **`ci_config.yml`** - Docker-in-Docker CI/CD configuration
  - Docker Compose setup for CI environments
  - GitHub Actions, GitLab CI, and Jenkins examples
  - Environment variable configuration for different platforms

## Current Test Coverage Analysis

Based on the analysis of existing test structure:

### Current Status (Before Testcontainers)
- **Total Coverage**: ~36.8% (from existing coverage files)
- **Main Issues**: 
  - Many security validation failures in existing tests
  - Build compatibility issues with Docker API types
  - Missing integration test coverage for real container scenarios

### Target Coverage (With Testcontainers)
- **Target**: 85%+ comprehensive coverage
- **Strategy**: 
  - Real container testing vs. mocked scenarios
  - End-to-end workflow validation
  - Network diagnostic validation with actual containers
  - Performance testing under load

## Dependencies Added

The following dependency was added to support Testcontainers integration:

```go
require github.com/testcontainers/testcontainers-go v0.38.0
```

This provides:
- Container lifecycle management
- Network creation and configuration
- Port mapping and connectivity testing
- Integration with Docker daemon
- Automatic cleanup capabilities

## Test Scenarios Implemented

### 1. Network Connectivity Scenario
- **Purpose**: Tests basic network connectivity between containers on custom networks
- **Setup**: Creates custom networks with different subnets
- **Test**: Validates container-to-container communication via network aliases
- **Coverage**: Network configuration, DNS resolution, container networking

### 2. Multi-Network Scenario  
- **Purpose**: Tests containers connected to multiple networks with different subnets
- **Setup**: Creates 3 different networks with containers on various combinations
- **Test**: Validates network isolation and cross-network communication
- **Coverage**: Multi-network configurations, subnet isolation, routing

### 3. Service Discovery Scenario
- **Purpose**: Tests DNS-based service discovery between containers
- **Setup**: Creates services with specific aliases on custom networks
- **Test**: Validates DNS resolution and service alias functionality
- **Coverage**: DNS configuration, service discovery, container naming

### 4. Port Exposure Scenario
- **Purpose**: Tests port exposure and external accessibility  
- **Setup**: Creates containers with exposed ports (HTTP, Redis)
- **Test**: Validates port mapping and external connectivity
- **Coverage**: Port binding, external access, service health checks

### 5. Load Test Scenario
- **Purpose**: Tests system behavior under load with configurable parameters
- **Setup**: Creates multiple containers with concurrent request testing
- **Test**: Measures success rates, throughput, and system stability
- **Coverage**: Performance characteristics, concurrent access, system limits

## Usage Examples

### Running Individual Tests

```bash
# Run all integration tests
go test -v -timeout=15m ./test/integration/

# Run tests in short mode (skip long-running tests)
go test -v -short ./test/integration/

# Run specific test scenario
go test -v -run TestNetworkConnectivity ./test/integration/
```

### Using Make Targets

```bash
# Run Testcontainers integration tests
make test-testcontainers

# Run Testcontainers tests in short mode for CI
make test-testcontainers-short

# Generate coverage report
make test-coverage
```

### Programmatic Usage

```go
func TestMyScenario(t *testing.T) {
    // Create test container manager
    tcm, err := NewTestContainerManager(ctx, t)
    require.NoError(t, err)
    defer tcm.Cleanup()

    // Create custom network
    network, err := tcm.CreateCustomNetwork("test-net", "172.100.0.0/16")
    require.NoError(t, err)

    // Create nginx container
    nginx, err := tcm.CreateNginxContainer("test-net")
    require.NoError(t, err)

    // Test connectivity
    err = tcm.WaitForContainerConnectivity(nginx, "80/tcp", 30*time.Second)
    require.NoError(t, err)
}
```

## Docker-in-Docker (DinD) Support

The framework includes comprehensive Docker-in-Docker support for CI/CD environments:

### Environment Variables
```bash
export TESTCONTAINERS_RYUK_DISABLED=false
export TESTCONTAINERS_HOST_OVERRIDE=localhost
export DOCKER_HOST=tcp://docker:2376  # For CI environments
```

### CI/CD Platform Support
- **GitHub Actions**: Automatic Docker service configuration
- **GitLab CI**: Docker-in-Docker service integration  
- **Jenkins**: Dockerized pipeline support
- **Local Development**: Native Docker daemon integration

## Benefits for Coverage Improvement

### Real-World Testing
- **Authentic Scenarios**: Tests against actual Docker containers and networks
- **No Mock Limitations**: Eliminates discrepancies between mocks and real behavior  
- **Integration Coverage**: Tests full workflow from Docker API to network configuration

### Comprehensive Network Testing
- **Actual Network Creation**: Tests real Docker network creation and configuration
- **Container Communication**: Validates actual container-to-container networking
- **DNS Resolution**: Tests real DNS resolution within Docker networks
- **Port Mapping**: Validates actual port exposure and accessibility

### Performance and Load Testing
- **Real Load Scenarios**: Tests system behavior under actual load conditions
- **Resource Constraints**: Validates behavior with limited system resources
- **Concurrent Access**: Tests concurrent container creation and networking
- **Cleanup Verification**: Ensures proper resource cleanup under all conditions

## Next Steps for 85% Coverage

### Phase 1: Core Integration (Completed)
- ✅ Testcontainers framework setup
- ✅ Basic container lifecycle management
- ✅ Network creation and management utilities
- ✅ Core test scenarios implementation
- ✅ CI/CD Docker-in-Docker support

### Phase 2: Extended Coverage (Recommended)
- **Diagnostic Engine Integration**: Connect with actual Docker Network Doctor diagnostic checks
- **Error Scenario Testing**: Test failure modes and error handling
- **Edge Case Coverage**: Test unusual network configurations and constraints
- **Performance Benchmarking**: Comprehensive performance testing with metrics

### Phase 3: Production Readiness (Future)
- **Security Testing**: Validate security aspects of network diagnostics
- **Large Scale Testing**: Test with many containers and complex networks
- **Cross-Platform Testing**: Validate across different Docker daemon versions
- **Regression Test Suite**: Automated regression testing for new features

## Current Issues and Considerations

### Fixed Issues
- ✅ Docker API compatibility issues (`types.ContainerListOptions` → `container.ListOptions`)
- ✅ Package naming conflicts in test directories
- ✅ Testcontainers dependency integration

### Current Limitations
- Docker Network Doctor diagnostic engine integration pending
- Some advanced scenarios may require privileged containers
- CI/CD environments may have resource constraints for large scale testing

### Performance Considerations
- Tests may take several minutes due to container creation overhead
- Parallel test execution should be limited to prevent resource contention
- Cleanup is automatic but may take time in CI environments

## Maintenance and Updates

### Dependency Updates
- Testcontainers Go library updates should be tested thoroughly
- Docker API compatibility should be validated with new versions
- Test scenarios should be updated for new Docker features

### Test Maintenance
- Monitor test execution time and optimize slow scenarios  
- Update network subnet ranges if conflicts arise
- Review and update CI/CD configurations for new platforms
- Regularly validate coverage metrics and identify gaps

This integration testing framework provides a solid foundation for achieving the target 85%+ test coverage while ensuring high-quality, real-world testing scenarios.