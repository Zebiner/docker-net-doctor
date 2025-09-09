# Docker Network Doctor - API Reference

This document provides comprehensive API documentation for Docker Network Doctor, a diagnostic tool for troubleshooting Docker networking issues.

## Table of Contents

1. [Overview](#overview)
2. [Command Line Interface](#command-line-interface)
3. [Go API](#go-api)
4. [Diagnostic Engine](#diagnostic-engine)
5. [Docker Client](#docker-client)
6. [Check Types](#check-types)
7. [Configuration](#configuration)
8. [Examples](#examples)

## Overview

Docker Network Doctor provides both a command-line interface and a programmatic Go API for diagnosing Docker networking issues. The tool supports parallel execution, rate limiting, and comprehensive reporting.

### Key Features

- **Comprehensive Diagnostics**: 11+ different network checks
- **Parallel Execution**: Configurable worker pools for performance
- **Rate Limiting**: Prevents overwhelming Docker API
- **Multiple Output Formats**: Table, JSON, YAML
- **Docker Plugin Support**: Integrates with Docker CLI
- **Enhanced Client**: Advanced caching and metrics
- **Actionable Recommendations**: Specific fix suggestions

## Command Line Interface

### Basic Commands

```bash
# Run comprehensive diagnostics
docker-net-doctor diagnose

# Run specific diagnostic check
docker-net-doctor check <type>

# Generate detailed report
docker-net-doctor report [options]
```

### Global Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--output` | `-o` | Output format (table, json, yaml) | table |
| `--verbose` | `-v` | Enable verbose output | false |
| `--timeout` | | Timeout for diagnostics | 30s |

### Diagnose Command

```bash
docker-net-doctor diagnose [flags]
```

**Flags:**
- `--container, -c`: Filter to specific container
- `--network, -n`: Filter to specific network  
- `--parallel, -p`: Run checks in parallel (default: true)

**Examples:**
```bash
# Basic diagnostics
docker-net-doctor diagnose

# Check specific container
docker-net-doctor diagnose --container web-app

# Sequential execution for debugging
docker-net-doctor diagnose --parallel=false --verbose
```

### Check Command

```bash
docker-net-doctor check <type>
```

**Available Check Types:**
- `dns`: DNS resolution checks
- `bridge`: Bridge network configuration
- `connectivity`: Container connectivity tests
- `ports`: Port binding validation
- `iptables`: iptables rules verification
- `forwarding`: IP forwarding configuration
- `mtu`: MTU consistency checks
- `overlap`: Subnet overlap detection

**Examples:**
```bash
# Check DNS resolution
docker-net-doctor check dns

# Validate port bindings
docker-net-doctor check ports --output json
```

### Report Command

```bash
docker-net-doctor report [flags]
```

**Flags:**
- `--output-file, -f`: Save report to file
- `--include-system`: Include system configuration
- `--include-logs`: Include container logs

**Examples:**
```bash
# Generate comprehensive report
docker-net-doctor report --include-system --output-file report.json

# Quick report to stdout
docker-net-doctor report --output yaml
```

### Docker Plugin Usage

After installation as Docker CLI plugin:

```bash
# Install plugin
make install

# Use with docker command
docker netdoctor diagnose
docker netdoctor check connectivity
docker netdoctor report --include-system
```

## Go API

### Basic Usage

```go
package main

import (
    "context"
    "log"
    "time"
    
    "github.com/zebiner/docker-net-doctor/internal/diagnostics"
    "github.com/zebiner/docker-net-doctor/internal/docker"
)

func main() {
    ctx := context.Background()
    
    // Create Docker client
    client, err := docker.NewClient(ctx)
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()
    
    // Configure diagnostic engine
    config := &diagnostics.Config{
        Parallel:    true,
        Timeout:     30 * time.Second,
        Verbose:     false,
        WorkerCount: 4,
        RateLimit:   5.0,
    }
    
    // Create and run diagnostic engine
    engine := diagnostics.NewEngine(client, config)
    results, err := engine.Run(ctx)
    if err != nil {
        log.Fatal(err)
    }
    
    // Process results
    for _, check := range results.Checks {
        if !check.Success {
            log.Printf("Check %s failed: %s", check.CheckName, check.Message)
            for _, suggestion := range check.Suggestions {
                log.Printf("  Suggestion: %s", suggestion)
            }
        }
    }
    
    // Get recommendations
    recommendations := engine.GetRecommendations()
    for _, rec := range recommendations {
        log.Printf("Recommendation: %s - %s", rec.Title, rec.Action)
    }
}
```

### Enhanced Client Usage

```go
// Create enhanced client with rate limiting and caching
opts := &docker.ClientOptions{
    UseEnhanced: true,
    EnhancedConfig: &docker.EnhancedClientConfig{
        RateLimit:       10.0,
        RateBurst:       20,
        CacheTTL:        60 * time.Second,
        MaxCacheEntries: 1000,
        MaxConnections:  20,
        Timeout:         45 * time.Second,
        MaxRetries:      5,
        RetryBackoff:    200 * time.Millisecond,
    },
}

client, err := docker.NewClientWithOptions(ctx, opts)
if err != nil {
    log.Fatal(err)
}

// Get metrics
if client.IsEnhanced() {
    metrics := client.GetMetrics()
    log.Printf("API calls: %d, Cache hits: %d", 
        metrics.TotalRequests, metrics.CacheHits)
}
```

## Diagnostic Engine

### Engine Configuration

```go
type Config struct {
    Parallel     bool          // Run checks in parallel
    Timeout      time.Duration // Global timeout for all checks
    Verbose      bool          // Enable verbose output
    TargetFilter string        // Filter for specific containers/networks
    WorkerCount  int           // Number of worker goroutines
    RateLimit    float64       // API rate limit (requests per second)
}
```

### Results Structure

```go
type Results struct {
    Checks   []*CheckResult      // Individual check results
    Summary  Summary             // Aggregate statistics
    Duration time.Duration       // Total execution time
    Metrics  *ExecutionMetrics   // Performance metrics
}

type CheckResult struct {
    CheckName   string                 // Unique check identifier
    Success     bool                   // Check passed/failed
    Message     string                 // Human-readable result
    Details     map[string]interface{} // Additional diagnostic data
    Suggestions []string               // Suggested fixes
    Timestamp   time.Time              // Execution timestamp
    Duration    time.Duration          // Check execution time
}

type Summary struct {
    TotalChecks    int      // Total number of checks run
    PassedChecks   int      // Number of successful checks
    FailedChecks   int      // Number of failed checks
    WarningChecks  int      // Number of checks with warnings
    CriticalIssues []string // Critical issues requiring attention
}
```

### Execution Metrics

```go
type ExecutionMetrics struct {
    ParallelExecution bool          // Execution mode
    WorkerCount       int           // Number of workers used
    TotalDuration     time.Duration // Total execution time
    AverageCheckTime  time.Duration // Average per-check time
    MaxCheckTime      time.Duration // Longest check time
    MinCheckTime      time.Duration // Shortest check time
    MemoryUsageMB     float64       // Peak memory usage
    APICallsCount     int64         // Total API calls made
    RateLimitHits     int64         // Rate limit encounters
    ErrorRate         float64       // Percentage of failed checks
}
```

## Docker Client

### Standard Client

```go
// Create standard client
client, err := docker.NewClient(ctx)

// List containers
containers, err := client.ListContainers(ctx)

// Get network information
networks, err := client.GetNetworkInfo()

// Get container network configuration
config, err := client.GetContainerNetworkConfig(containerID)

// Execute command in container
output, err := client.ExecInContainer(ctx, containerID, []string{"ping", "-c", "3", "google.com"})

// Check daemon connectivity
err = client.Ping(ctx)
```

### Enhanced Client Features

```go
// Check if using enhanced features
if client.IsEnhanced() {
    // Get performance metrics
    metrics := client.GetMetrics()
    
    // Get cache statistics
    cacheStats := client.GetCacheStats()
    
    // Invalidate cache when needed
    client.InvalidateCache()
}
```

## Check Types

### System Checks

#### DaemonConnectivityCheck
- **Name**: `daemon_connectivity`
- **Severity**: Critical
- **Description**: Verifies Docker daemon accessibility
- **Validates**: API connectivity, version compatibility, permissions

#### NetworkIsolationCheck
- **Name**: `network_isolation`
- **Severity**: Warning
- **Description**: Analyzes network isolation configuration
- **Validates**: Internal/external access, custom networks, security settings

### Network Infrastructure Checks

#### BridgeNetworkCheck
- **Name**: `bridge_network`
- **Severity**: Critical
- **Description**: Validates default bridge network configuration
- **Validates**: Bridge existence, IP configuration, routing

#### IPForwardingCheck
- **Name**: `ip_forwarding`
- **Severity**: Critical
- **Description**: Ensures IP forwarding is enabled
- **Validates**: `net.ipv4.ip_forward=1`, routing capability

#### IptablesCheck
- **Name**: `iptables`
- **Severity**: Warning
- **Description**: Verifies Docker iptables rules
- **Validates**: NAT configuration, DOCKER chain, port forwarding

#### MTUConsistencyCheck
- **Name**: `mtu_consistency`
- **Severity**: Warning
- **Description**: Checks MTU settings consistency
- **Validates**: Interface MTU values, fragmentation issues

#### SubnetOverlapCheck
- **Name**: `subnet_overlap`
- **Severity**: Warning
- **Description**: Detects overlapping subnets
- **Validates**: CIDR conflicts, IP range overlaps

### DNS and Connectivity Checks

#### DNSResolutionCheck
- **Name**: `dns_resolution`
- **Severity**: Warning
- **Description**: Tests external DNS resolution
- **Validates**: Name resolution, DNS server accessibility

#### InternalDNSCheck
- **Name**: `internal_dns`
- **Severity**: Warning
- **Description**: Verifies container-to-container DNS
- **Validates**: Service discovery, internal name resolution

#### ContainerConnectivityCheck
- **Name**: `container_connectivity`
- **Severity**: Warning
- **Description**: Tests network connectivity between containers
- **Validates**: Inter-container communication, network reachability

#### PortBindingCheck
- **Name**: `port_binding`
- **Severity**: Warning
- **Description**: Validates port bindings and accessibility
- **Validates**: Port mapping, external accessibility

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DOCKER_HOST` | Docker daemon socket | `unix:///var/run/docker.sock` |
| `DOCKER_API_VERSION` | Docker API version | Auto-negotiated |
| `DOCKER_TIMEOUT` | Docker operation timeout | 30s |

### Configuration Files

Docker Network Doctor supports configuration via:
- Command line flags (highest priority)
- Environment variables
- Configuration files (future enhancement)

## Examples

### Common Use Cases

#### Troubleshooting Container Connectivity

```bash
# Check specific container connectivity
docker-net-doctor diagnose --container web-app --verbose

# Focus on connectivity checks
docker-net-doctor check connectivity
docker-net-doctor check dns
```

#### Performance Analysis

```bash
# Run with performance metrics
docker-net-doctor diagnose --parallel --verbose

# Generate detailed report
docker-net-doctor report --include-system --output-file perf-report.json
```

#### Integration with CI/CD

```bash
# Automated health checks
docker-net-doctor diagnose --output json | jq '.Summary.FailedChecks'

# Fail build if critical issues found
if [ $(docker-net-doctor diagnose --output json | jq '.Summary.CriticalIssues | length') -gt 0 ]; then
    echo "Critical networking issues found"
    exit 1
fi
```

### Programmatic Integration

#### Custom Check Implementation

```go
type CustomCheck struct{}

func (c *CustomCheck) Name() string {
    return "custom_check"
}

func (c *CustomCheck) Description() string {
    return "Custom diagnostic check"
}

func (c *CustomCheck) Severity() Severity {
    return SeverityWarning
}

func (c *CustomCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
    // Implement custom diagnostic logic
    return &CheckResult{
        CheckName: c.Name(),
        Success:   true,
        Message:   "Custom check passed",
        Timestamp: time.Now(),
    }, nil
}
```

#### Result Processing

```go
func processResults(results *diagnostics.Results) {
    // Check overall health
    if results.Summary.CriticalIssues > 0 {
        log.Printf("CRITICAL: %d critical issues found", len(results.Summary.CriticalIssues))
    }
    
    // Analyze performance
    if results.Metrics.ErrorRate > 0.5 {
        log.Printf("WARNING: High error rate: %.1f%%", results.Metrics.ErrorRate*100)
    }
    
    // Process individual checks
    for _, check := range results.Checks {
        if !check.Success {
            log.Printf("Failed: %s - %s", check.CheckName, check.Message)
            
            // Apply suggestions
            for _, suggestion := range check.Suggestions {
                log.Printf("  → %s", suggestion)
            }
        }
    }
}
```

## Error Handling

### Common Errors

1. **Docker daemon not accessible**
   - Check Docker is running
   - Verify socket permissions
   - Check `DOCKER_HOST` environment variable

2. **Permission denied**
   - Add user to `docker` group
   - Use `sudo` if necessary
   - Check Docker socket permissions

3. **Network configuration issues**
   - Restart Docker daemon
   - Reset network configuration: `docker network prune`
   - Check system networking: `systemctl status networking`

4. **Timeout errors**
   - Increase timeout: `--timeout 60s`
   - Check system resources
   - Reduce parallel workers: use sequential mode

### Error Codes

| Code | Description |
|------|-------------|
| 0 | Success - all checks passed |
| 1 | Warning - some checks failed but system functional |
| 2 | Critical - major issues preventing functionality |
| 3 | Fatal - unable to connect to Docker daemon |

---

For more examples and detailed usage instructions, see the [Usage Examples](examples/) directory and the [Testing Guide](testing-guide.md).