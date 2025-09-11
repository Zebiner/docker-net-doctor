# Docker Network Doctor Testing Guide

## Quick Start

This guide provides practical instructions for running tests, measuring coverage, adding new tests, and following testing best practices in the Docker Network Doctor project.

## Prerequisites

### Required Software
- **Go 1.21+**: Modern Go toolchain for testing features
- **Docker**: Required for integration tests (optional for unit tests)
- **Make**: For convenient test execution via Makefile
- **Git**: For test result tracking and CI integration

### Optional Tools
- **golangci-lint**: For code quality analysis
- **entr**: For continuous testing during development
- **testcontainers**: For advanced integration testing (already in dependencies)

### Environment Setup
```bash
# Clone and setup
git clone https://github.com/zebiner/docker-net-doctor.git
cd docker-net-doctor

# Install dependencies
go mod download

# Verify Docker is running (for integration tests)
docker version
```

## Running Tests

### Basic Test Execution

#### Run All Tests
```bash
# Using Makefile (recommended)
make test

# Direct Go command
go test ./...

# Verbose output
go test -v ./...
```

#### Run Specific Package Tests
```bash
# Test diagnostics package only
go test ./internal/diagnostics

# Test Docker client package
go test ./internal/docker

# Test CLI package
go test ./cmd/docker-net-doctor

# Test with verbose output
go test -v ./internal/diagnostics
```

#### Run Specific Test Functions
```bash
# Run specific test by name
go test ./internal/diagnostics -run TestNewEngine

# Run tests matching pattern
go test ./internal/diagnostics -run "TestEngine.*"

# Run specific subtest
go test ./internal/diagnostics -run "TestNewEngine/valid_client_and_config"
```

### Integration Tests

#### Docker Integration Tests
```bash
# Run integration tests (requires Docker daemon)
make test-integration

# Or directly
go test -tags=integration ./test/integration

# Run with verbose output to see Docker operations
go test -v -tags=integration ./test/integration
```

#### TestContainer Scenarios
```bash
# Run specific scenario tests
go test -v ./test/integration/testcontainers -run TestNginxScenario
go test -v ./test/integration/testcontainers -run TestRedisScenario
go test -v ./test/integration/testcontainers -run TestCustomNetworksScenario
```

### Performance Tests

#### Run Benchmarks
```bash
# Run all benchmarks
go test -bench=. ./test/benchmark

# Run specific benchmarks
go test -bench=BenchmarkIndividualChecks ./test/benchmark

# Run with memory allocation stats
go test -bench=. -benchmem ./test/benchmark

# Save benchmark results for comparison
go test -bench=. ./test/benchmark > benchmark_results.txt
```

#### Performance Profiling
```bash
# Generate CPU profile
go test -bench=. -cpuprofile=cpu.prof ./test/benchmark

# Generate memory profile
go test -bench=. -memprofile=mem.prof ./test/benchmark

# Analyze profiles
go tool pprof cpu.prof
go tool pprof mem.prof
```

## Coverage Measurement

### Basic Coverage

#### Generate Coverage Reports
```bash
# Basic coverage percentage
make test-coverage

# Or manually
go test -cover ./internal/diagnostics
go test -cover ./internal/docker
go test -cover ./cmd/docker-net-doctor
```

#### Detailed Coverage Analysis
```bash
# Generate coverage profile
go test -coverprofile=coverage.out ./...

# View function-level coverage
go tool cover -func=coverage.out

# Generate HTML coverage report
go tool cover -html=coverage.out -o coverage.html

# Open coverage report in browser (Linux/WSL)
xdg-open coverage.html
```

### Advanced Coverage

#### Coverage by Package
```bash
# Coverage for specific package with details
go test -coverprofile=diag_coverage.out ./internal/diagnostics
go tool cover -func=diag_coverage.out

# Coverage excluding test files
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep -v "_test.go"
```

#### Atomic Coverage Mode (for concurrent tests)
```bash
# Use atomic mode for concurrent code coverage
go test -race -covermode=atomic -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

### Coverage Quality Gates
```bash
# Check if coverage meets minimum threshold (example: 70%)
COVERAGE=$(go tool cover -func=coverage.out | grep "total:" | awk '{print $3}' | sed 's/%//')
if (( $(echo "$COVERAGE < 70" | bc -l) )); then
    echo "Coverage $COVERAGE% is below 70% threshold"
    exit 1
else
    echo "Coverage $COVERAGE% meets quality gate"
fi
```

## Adding New Tests

### Test File Naming Convention
```
{source_file_name}_test.go
```

Examples:
- `engine.go` → `engine_test.go`
- `connectivity_checks.go` → `connectivity_checks_test.go`
- `worker_pool.go` → `worker_pool_test.go`

### Basic Test Structure

#### Unit Test Template
```go
package diagnostics

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/zebiner/docker-net-doctor/internal/docker"
)

func TestYourFunction(t *testing.T) {
    tests := []struct {
        name        string
        input       YourInput
        expected    YourExpected
        shouldError bool
    }{
        {
            name:        "valid input",
            input:       YourInput{/* ... */},
            expected:    YourExpected{/* ... */},
            shouldError: false,
        },
        {
            name:        "invalid input",
            input:       YourInput{/* ... */},
            expected:    YourExpected{/* ... */},
            shouldError: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
            result, err := YourFunction(tt.input)
            
            if tt.shouldError {
                require.Error(t, err)
                return
            }
            
            require.NoError(t, err)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

#### Mock Client Template
```go
type MockDockerClient struct {
    ShouldFailPing      bool
    PingError          error
    Containers         []types.Container
    Networks           []network.Inspect
    ExecOutput         string
    CallCounts         map[string]int
}

func NewMockDockerClient() *MockDockerClient {
    return &MockDockerClient{
        CallCounts: make(map[string]int),
        // Set up default mock data
    }
}

func (m *MockDockerClient) Ping(ctx context.Context) error {
    m.CallCounts["Ping"]++
    if m.ShouldFailPing {
        return m.PingError
    }
    return nil
}
```

### Diagnostic Check Test Template
```go
func TestYourNewCheck_Run(t *testing.T) {
    tests := []struct {
        name           string
        mockSetup      func(*MockDockerClient)
        expectedResult CheckResult
        shouldError    bool
    }{
        {
            name: "check passes",
            mockSetup: func(mock *MockDockerClient) {
                // Configure mock for success scenario
            },
            expectedResult: CheckResult{
                Status:  StatusPassed,
                Message: "Check passed successfully",
            },
            shouldError: false,
        },
        {
            name: "check fails",
            mockSetup: func(mock *MockDockerClient) {
                // Configure mock for failure scenario
            },
            expectedResult: CheckResult{
                Status:  StatusFailed,
                Message: "Check failed",
            },
            shouldError: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup
            mockClient := NewMockDockerClient()
            tt.mockSetup(mockClient)
            
            check := &YourNewCheck{}
            ctx := context.Background()
            
            // Execute
            result, err := check.Run(ctx, mockClient)
            
            // Verify
            if tt.shouldError {
                require.Error(t, err)
                return
            }
            
            require.NoError(t, err)
            assert.Equal(t, tt.expectedResult.Status, result.Status)
            assert.Contains(t, result.Message, tt.expectedResult.Message)
        })
    }
}
```

### Integration Test Template
```go
//go:build integration

func TestIntegrationScenario(t *testing.T) {
    // Check if Docker is available
    if !dockerAvailable(t) {
        t.Skip("Docker not available, skipping integration test")
    }

    ctx := context.Background()
    
    // Setup test environment
    cleanup := setupTestEnvironment(t, ctx)
    defer cleanup()
    
    // Run test
    client, err := docker.NewClient()
    require.NoError(t, err)
    
    engine := NewEngine(client, &Config{
        Parallel: true,
        Timeout:  30 * time.Second,
    })
    
    results, err := engine.RunChecks(ctx)
    require.NoError(t, err)
    assert.NotEmpty(t, results)
}

func dockerAvailable(t *testing.T) bool {
    _, err := exec.LookPath("docker")
    if err != nil {
        return false
    }
    
    cmd := exec.Command("docker", "version")
    return cmd.Run() == nil
}
```

## Testing Best Practices

### 1. Test Organization

#### Group Related Tests
```go
func TestEngine(t *testing.T) {
    t.Run("Construction", func(t *testing.T) {
        t.Run("ValidConfig", testEngineValidConfig)
        t.Run("InvalidConfig", testEngineInvalidConfig)
    })
    
    t.Run("Execution", func(t *testing.T) {
        t.Run("ParallelExecution", testEngineParallel)
        t.Run("SequentialExecution", testEngineSequential)
    })
}
```

#### Test Naming Convention
- Test function: `TestFunctionName`
- Benchmark function: `BenchmarkFunctionName`
- Example function: `ExampleFunctionName`
- Helper function: `testHelperName` (lowercase, not executed as test)

### 2. Mock Design Principles

#### Realistic Mock Data
```go
func createRealisticContainer() types.Container {
    return types.Container{
        ID:      "sha256:abcdef123456",
        Names:   []string{"/web-server-1"},
        Image:   "nginx:1.21-alpine",
        Status:  "Up 2 hours",
        State:   "running",
        Ports: []types.Port{
            {PrivatePort: 80, PublicPort: 8080, Type: "tcp", IP: "0.0.0.0"},
        },
        NetworkSettings: &types.SummaryNetworkSettings{
            Networks: map[string]*network.EndpointSettings{
                "bridge": {
                    IPAddress: "172.17.0.2",
                    Gateway:   "172.17.0.1",
                },
            },
        },
    }
}
```

#### Failure Injection Patterns
```go
type FailureMode int

const (
    NoFailure FailureMode = iota
    ConnectionFailure
    TimeoutFailure
    ResourceExhaustionFailure
    PermissionFailure
)

func (m *MockClient) ConfigureFailure(mode FailureMode) {
    switch mode {
    case ConnectionFailure:
        m.ShouldFailPing = true
        m.PingError = errors.New("connection refused")
    case TimeoutFailure:
        m.ShouldFailPing = true
        m.PingError = context.DeadlineExceeded
    // ... other failure modes
    }
}
```

### 3. Concurrent Testing

#### Test Race Conditions
```go
func TestWorkerPoolConcurrentExecution(t *testing.T) {
    wp := NewWorkerPool(10)
    wp.Start()
    defer wp.Stop()
    
    var wg sync.WaitGroup
    results := make(chan CheckResult, 100)
    
    // Submit jobs concurrently
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            job := &MockJob{ID: id}
            result := wp.Submit(job)
            results <- result
        }(i)
    }
    
    // Wait for completion
    wg.Wait()
    close(results)
    
    // Verify results
    var collected []CheckResult
    for result := range results {
        collected = append(collected, result)
    }
    
    assert.Len(t, collected, 100)
}
```

#### Race Detection
```bash
# Run tests with race detection
go test -race ./internal/diagnostics

# Run specific concurrent tests with race detection
go test -race -run ".*Concurrent.*" ./internal/diagnostics
```

### 4. Error Testing Patterns

#### Comprehensive Error Scenarios
```go
func TestErrorHandlingScenarios(t *testing.T) {
    scenarios := []struct {
        name          string
        error         error
        expectedType  ErrorType
        shouldRecover bool
    }{
        {
            name:          "docker daemon not running",
            error:         errors.New("Cannot connect to the Docker daemon"),
            expectedType:  DockerConnectionError,
            shouldRecover: true,
        },
        {
            name:          "network timeout",
            error:         context.DeadlineExceeded,
            expectedType:  NetworkTimeoutError,
            shouldRecover: true,
        },
        {
            name:          "permission denied",
            error:         errors.New("permission denied"),
            expectedType:  PermissionError,
            shouldRecover: false,
        },
    }

    for _, scenario := range scenarios {
        t.Run(scenario.name, func(t *testing.T) {
            errorType := ClassifyError(scenario.error)
            assert.Equal(t, scenario.expectedType, errorType)
            
            canRecover := CanRecover(errorType)
            assert.Equal(t, scenario.shouldRecover, canRecover)
        })
    }
}
```

#### Nil Safety Testing
```go
func TestNilSafety(t *testing.T) {
    // Test nil inputs
    result, err := ProcessInput(nil)
    assert.Error(t, err)
    assert.Nil(t, result)
    
    // Test nil fields in struct
    input := &Input{Field1: "valid", Field2: nil}
    result, err = ProcessInput(input)
    assert.NoError(t, err)
    assert.NotNil(t, result)
}
```

### 5. Performance Testing

#### Benchmark Writing
```go
func BenchmarkDiagnosticCheck(b *testing.B) {
    mockClient := NewMockDockerClient()
    check := &YourCheck{}
    ctx := context.Background()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := check.Run(ctx, mockClient)
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkWorkerPoolThroughput(b *testing.B) {
    wp := NewWorkerPool(runtime.NumCPU())
    wp.Start()
    defer wp.Stop()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        job := &MockJob{ID: i}
        wp.Submit(job)
    }
}
```

## Common Pitfalls to Avoid

### 1. Test Independence Issues

#### ❌ Bad: Tests depend on execution order
```go
var globalState string

func TestFirst(t *testing.T) {
    globalState = "modified"
    // test logic
}

func TestSecond(t *testing.T) {
    // This test depends on TestFirst running first
    assert.Equal(t, "modified", globalState)
}
```

#### ✅ Good: Tests are independent
```go
func TestFirst(t *testing.T) {
    state := "initial"
    // modify state locally
    state = "modified"
    // test logic using local state
}

func TestSecond(t *testing.T) {
    state := "initial"
    // test logic using local state
}
```

### 2. Mock Over-Specification

#### ❌ Bad: Mock knows too much about implementation
```go
mockClient.EXPECT().Ping().Return(nil)
mockClient.EXPECT().ListContainers().Return(containers, nil)
mockClient.EXPECT().GetNetworkInfo().Return(networks, nil)
// Mock expects exact sequence of calls
```

#### ✅ Good: Mock specifies behavior, not implementation
```go
mockClient.SetPingResult(nil)
mockClient.SetContainers(containers)
mockClient.SetNetworks(networks)
// Mock provides data when asked, order doesn't matter
```

### 3. Inadequate Error Testing

#### ❌ Bad: Only testing happy path
```go
func TestFunction(t *testing.T) {
    result, err := Function("valid input")
    assert.NoError(t, err)
    assert.Equal(t, "expected", result)
}
```

#### ✅ Good: Testing both success and failure paths
```go
func TestFunction(t *testing.T) {
    t.Run("success case", func(t *testing.T) {
        result, err := Function("valid input")
        assert.NoError(t, err)
        assert.Equal(t, "expected", result)
    })
    
    t.Run("invalid input", func(t *testing.T) {
        result, err := Function("")
        assert.Error(t, err)
        assert.Empty(t, result)
    })
    
    t.Run("nil input", func(t *testing.T) {
        result, err := Function(nil)
        assert.Error(t, err)
        assert.Nil(t, result)
    })
}
```

### 4. Resource Cleanup Issues

#### ❌ Bad: Resources not cleaned up
```go
func TestIntegration(t *testing.T) {
    container := startTestContainer(t)
    // Test logic...
    // Container left running!
}
```

#### ✅ Good: Proper cleanup with defer
```go
func TestIntegration(t *testing.T) {
    container := startTestContainer(t)
    defer func() {
        if err := stopTestContainer(container); err != nil {
            t.Errorf("Failed to cleanup container: %v", err)
        }
    }()
    
    // Test logic...
}
```

## CI/CD Integration

### GitHub Actions Workflow
```yaml
name: Test Suite
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
          
      - name: Run unit tests
        run: go test -race -coverprofile=coverage.out ./...
        
      - name: Check coverage
        run: |
          COVERAGE=$(go tool cover -func=coverage.out | grep "total:" | awk '{print $3}' | sed 's/%//')
          echo "Coverage: $COVERAGE%"
          if (( $(echo "$COVERAGE < 70" | bc -l) )); then
            echo "Coverage below threshold"
            exit 1
          fi
          
      - name: Run integration tests (if Docker available)
        run: |
          if docker version; then
            go test -tags=integration ./test/integration
          else
            echo "Docker not available, skipping integration tests"
          fi
```

### Makefile Integration
```makefile
.PHONY: test test-unit test-integration test-coverage test-race

test: test-unit test-integration

test-unit:
	go test ./...

test-integration:
	@if docker version >/dev/null 2>&1; then \
		go test -tags=integration ./test/integration; \
	else \
		echo "Docker not available, skipping integration tests"; \
	fi

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-race:
	go test -race ./...

test-benchmark:
	go test -bench=. -benchmem ./test/benchmark
```

## Troubleshooting

### Common Test Failures

#### Docker Daemon Not Available
```
Error: Cannot connect to the Docker daemon
```
**Solution**: Start Docker daemon or skip integration tests
```bash
# Check Docker status
docker version

# Skip integration tests if Docker unavailable
go test -short ./...
```

#### Race Conditions
```
Error: race detected during execution
```
**Solution**: Fix race conditions or run without race detection for debugging
```bash
# Debug without race detection
go test ./...

# Fix race conditions in code
go test -race ./...
```

#### Timeout Issues
```
Error: test timed out after 10m0s
```
**Solution**: Reduce test scope or increase timeout
```bash
# Increase timeout
go test -timeout 30m ./...

# Run specific failing test
go test -run TestSpecific ./...
```

### Coverage Issues

#### Coverage Not Generated
```bash
# Ensure coverage profile is created
go test -coverprofile=coverage.out ./...

# Check if profile exists
ls -la coverage.out

# Generate report
go tool cover -func=coverage.out
```

#### Incomplete Coverage Reporting
```bash
# Use atomic mode for concurrent code
go test -race -covermode=atomic -coverprofile=coverage.out ./...

# Exclude vendor and test files from analysis
go tool cover -func=coverage.out | grep -v vendor | grep -v "_test.go"
```

## Conclusion

This testing guide provides the foundation for maintaining and extending the Docker Network Doctor test suite. Following these practices will help ensure reliable, maintainable, and comprehensive test coverage.

Key takeaways:
- **Run tests frequently** during development
- **Use table-driven tests** for comprehensive scenario coverage
- **Mock external dependencies** consistently
- **Test error paths** as thoroughly as happy paths
- **Measure and track coverage** regularly
- **Clean up resources** properly in integration tests
- **Follow naming conventions** for maintainability

For questions or improvements to this guide, please refer to the project maintainers or create an issue in the project repository.