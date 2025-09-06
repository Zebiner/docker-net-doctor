# Docker Network Doctor 🔍🐳

A comprehensive diagnostic tool for Docker networking issues that acts as your expert companion in troubleshooting container connectivity problems. Think of it as having a senior DevOps engineer by your side, systematically checking every aspect of your Docker network configuration and explaining what's wrong in plain English.

## Table of Contents

- [Why Docker Network Doctor?](#why-docker-network-doctor)
- [Understanding Docker Networking Issues](#understanding-docker-networking-issues)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Using the Tool](#using-the-tool)
- [Understanding Diagnostic Results](#understanding-diagnostic-results)
- [Debugging the Tool Itself](#debugging-the-tool-itself)
- [Testing and Quality Assurance](#testing-and-quality-assurance)
- [Enhancing and Contributing](#enhancing-and-contributing)
- [Real-World Troubleshooting Scenarios](#real-world-troubleshooting-scenarios)
- [Architecture and Design](#architecture-and-design)
- [FAQ and Common Issues](#faq-and-common-issues)
- [Support and Community](#support-and-community)

## Why Docker Network Doctor?

Docker networking is powerful but complex. When containers can't communicate, the root cause could be anywhere in a stack that includes container configuration, Docker daemon settings, iptables rules, DNS resolution, network bridges, and host networking. Without systematic diagnostics, developers often waste hours trying random fixes found on Stack Overflow.

Docker Network Doctor solves this by providing intelligent, automated diagnostics that check every layer of the networking stack. Instead of manually running dozens of commands and trying to interpret their output, you get clear, actionable insights about what's wrong and how to fix it.

The tool understands the relationships between different networking components. For example, if DNS resolution is failing, it knows to check not just the container's resolv.conf, but also the Docker daemon's DNS settings, the host's DNS configuration, and whether the embedded DNS server at 127.0.0.11 is functioning correctly. This contextual intelligence is what makes the tool invaluable for both beginners learning Docker networking and experts dealing with complex production issues.

## Understanding Docker Networking Issues

Before diving into using the tool, it's helpful to understand the common categories of Docker networking problems that the tool diagnoses. Docker networking operates at multiple layers, and issues at any layer can manifest as "containers can't connect" problems.

**Infrastructure Layer Issues** occur at the foundation of Docker networking. The Docker bridge network (docker0) must be properly configured with a valid subnet and the interface must be up. IP forwarding must be enabled in the kernel for containers to reach external networks. iptables rules must be properly configured for NAT and filtering. When any of these foundational elements break, nothing else works properly.

**DNS Resolution Problems** are perhaps the most frustrating because they can appear intermittent. Docker provides automatic DNS resolution for container names on custom networks, but this requires the embedded DNS server to function correctly. Containers also need to resolve external domains, which depends on proper DNS configuration in both the Docker daemon and the host system.

**Container Connectivity Issues** involve the actual network paths between containers. Even when infrastructure is correct, containers might not communicate due to network isolation settings, incorrect network attachments, or firewall rules blocking specific ports.

**Port Binding Conflicts** occur when multiple containers try to bind to the same host port, or when the application inside a container isn't actually listening on the exposed port. These issues are particularly confusing because Docker might report the port as exposed while the application remains unreachable.

## Installation

Docker Network Doctor can be installed in several ways, each suited to different use cases and preferences. Choose the method that best fits your workflow and environment.

### Method 1: Automated Installation (Recommended)

The easiest way to install Docker Network Doctor is using our installation script. This script automatically detects your operating system and architecture, downloads the appropriate binary, and installs it as a Docker CLI plugin.

```bash
# Install for current user (recommended for developers)
curl -sSL https://raw.githubusercontent.com/yourusername/docker-net-doctor/main/install.sh | bash

# Install system-wide (requires sudo, recommended for servers)
curl -sSL https://raw.githubusercontent.com/yourusername/docker-net-doctor/main/install.sh | bash -s -- --system
```

The installation script performs several important checks before installing. It verifies that Docker is installed and running, creates the necessary plugin directory if it doesn't exist, downloads the correct binary for your platform, and verifies the installation succeeded. If anything goes wrong, you'll get clear error messages explaining the problem.

### Method 2: Manual Installation

If you prefer to understand exactly what's being installed, you can perform a manual installation. This approach gives you complete control over the process.

First, download the appropriate binary for your platform from the [releases page](https://github.com/yourusername/docker-net-doctor/releases). The binaries follow the naming pattern `docker-net-doctor-{os}-{architecture}`. For example, Linux users on standard x86_64 hardware would download `docker-net-doctor-linux-amd64`.

Next, install the binary as a Docker CLI plugin:

```bash
# Create the Docker CLI plugins directory if it doesn't exist
mkdir -p ~/.docker/cli-plugins

# Move the binary to the plugins directory with the correct name
mv docker-net-doctor-linux-amd64 ~/.docker/cli-plugins/docker-net-doctor

# Make it executable
chmod +x ~/.docker/cli-plugins/docker-net-doctor

# Verify the installation
docker net-doctor --version
```

### Method 3: Building from Source

Building from source gives you the latest development version and allows you to modify the tool for your specific needs. This requires Go 1.20 or later installed on your system.

```bash
# Clone the repository
git clone https://github.com/yourusername/docker-net-doctor.git
cd docker-net-doctor

# Download dependencies
go mod download

# Build the binary
make build

# Install as Docker plugin
make install

# Or build for all platforms
make build-all
```

The Makefile includes many helpful targets for development. Running `make help` shows all available options, including targets for testing, linting, and creating release artifacts.

### Method 4: Using Docker

If you prefer not to install anything on your host system, you can run Docker Network Doctor as a container. This method is particularly useful for one-off diagnostics or in environments where you can't install additional tools.

```bash
# Run diagnostics using the Docker image
docker run --rm \
  -v /var/run/docker.sock:/var/run/docker.sock \
  yourusername/docker-net-doctor:latest \
  diagnose

# Create an alias for convenience
alias docker-net-doctor='docker run --rm -v /var/run/docker.sock:/var/run/docker.sock yourusername/docker-net-doctor:latest'
```

Note that running as a container requires mounting the Docker socket, which grants the container full access to Docker. Only use this method with trusted images.

## Quick Start

Once installed, you can immediately start diagnosing network issues. The tool is designed to be intuitive, with sensible defaults that work for most situations.

```bash
# Run comprehensive diagnostics
docker net-doctor diagnose

# Get help and see all available commands
docker net-doctor --help

# Check specific network component
docker net-doctor check dns

# Generate a detailed report
docker net-doctor report
```

The default `diagnose` command runs all diagnostic checks and presents a summary of findings. If any issues are detected, you'll see specific suggestions for fixing them.

## Using the Tool

Docker Network Doctor provides several commands, each designed for different diagnostic scenarios. Understanding when and how to use each command helps you troubleshoot more effectively.

### The Diagnose Command

The `diagnose` command is your starting point for troubleshooting. It runs a comprehensive suite of checks and provides an overall health assessment of your Docker networking.

```bash
# Run full diagnostics with default settings
docker net-doctor diagnose

# Run diagnostics with verbose output to see more details
docker net-doctor diagnose --verbose

# Focus on specific category of issues
docker net-doctor diagnose --category dns
docker net-doctor diagnose --category connectivity
docker net-doctor diagnose --category ports

# Diagnose specific container's networking
docker net-doctor diagnose --container nginx

# Output results in JSON for programmatic processing
docker net-doctor diagnose --json > results.json

# Set custom timeout for slow systems
docker net-doctor diagnose --timeout 60s
```

The diagnose command intelligently orders its checks, starting with fundamental infrastructure and progressing to specific features. This ordering means that if a basic check fails, you know that's likely the root cause of other issues.

### The Check Command

The `check` command allows you to run specific diagnostic checks when you already suspect where the problem might be. This targeted approach is faster and provides more detailed output for the specific component.

```bash
# Check DNS resolution in all containers
docker net-doctor check dns

# Check DNS in specific container
docker net-doctor check dns nginx

# Test connectivity between two containers
docker net-doctor check connectivity web-app database

# Verify port bindings are correct
docker net-doctor check ports

# Check if the bridge network is properly configured
docker net-doctor check bridge
```

Each check provides detailed output about what was tested, what was expected, and what was actually found. This information helps you understand not just that something is broken, but exactly how it's broken.

### The Report Command

The `report` command generates comprehensive documentation of your Docker network configuration and any issues found. This is invaluable for sharing with team members or support when troubleshooting complex issues.

```bash
# Generate a markdown report (default)
docker net-doctor report

# Save report to file
docker net-doctor report --output-file network-report.md

# Generate HTML report for better formatting
docker net-doctor report --format html --output-file report.html

# Include container logs in the report (useful for debugging application issues)
docker net-doctor report --include-logs
```

Reports include system information, Docker version and configuration, network topology diagrams (when possible), detailed test results, and specific recommendations for any issues found. The report format is designed to be self-contained, providing all context needed to understand the network state.

### The Interactive Command

The interactive command provides a guided troubleshooting experience, particularly helpful for users less familiar with Docker networking.

```bash
# Start interactive troubleshooting session
docker net-doctor interactive
```

The interactive mode asks questions about what problems you're experiencing and runs targeted diagnostics based on your answers. It explains what each test does and why it's relevant to your problem, making it an excellent learning tool.

## Understanding Diagnostic Results

Interpreting diagnostic results correctly is crucial for effective troubleshooting. Docker Network Doctor uses a consistent format and clear severity levels to help you prioritize issues.

### Severity Levels

Each diagnostic check has one of three severity levels that indicate how critical the issue is to Docker networking functionality.

**Critical Issues** (shown with ✗ in red) indicate fundamental problems that prevent Docker networking from functioning. These must be fixed first, as they likely cause cascading failures. Examples include missing Docker bridge network, disabled IP forwarding, or broken iptables rules.

**Warning Issues** (shown with ⚠ in yellow) indicate problems that affect specific features or cause degraded performance but don't completely break networking. Examples include DNS resolution problems in specific containers, MTU mismatches that might cause packet fragmentation, or subnet overlaps between networks.

**Information Messages** (shown with ℹ in cyan) provide context about your configuration without indicating problems. These help you understand your current setup and might suggest optimizations.

### Reading Check Results

Each check result includes several components that help you understand and fix issues. The check name identifies what was tested, making it easy to research or ask for help about specific checks. The status (pass/fail) immediately shows whether the check succeeded. The message provides a human-readable explanation of what was found.

The details section contains technical specifics about what was discovered. For example, a DNS check might show the exact resolv.conf contents or the specific DNS servers being queried. This technical detail helps advanced users understand exactly what's happening.

The suggestions section is perhaps the most valuable, providing specific commands or configuration changes to fix the issue. Suggestions are ordered by likelihood of success, with the most common fixes listed first.

### Understanding Relationships Between Checks

Docker networking components are interdependent, and understanding these relationships helps you fix problems more efficiently. For example, if the bridge network check fails, you can expect container connectivity checks to also fail. If IP forwarding is disabled, containers won't reach external networks even if internal networking works.

The tool understands these relationships and presents results in a logical order. Critical infrastructure issues appear first, followed by feature-specific problems. This ordering guides you through fixing issues in the correct sequence.

## Debugging the Tool Itself

Sometimes you need to debug Docker Network Doctor itself, either because it's not working as expected or because you're developing enhancements. The tool includes several features to help with debugging.

### Verbose Mode

Verbose mode provides detailed output about what the tool is doing, which checks are running, and what Docker API calls are being made.

```bash
# Run with verbose output
docker net-doctor diagnose --verbose

# Combine with specific checks for targeted debugging
docker net-doctor check dns --verbose nginx
```

Verbose output shows the exact Docker commands being executed, raw API responses, timing information for each check, and any errors or warnings encountered. This information helps identify whether issues are with the tool itself or with the Docker environment.

### Debug Logging

For even more detailed debugging, you can enable debug logging through environment variables:

```bash
# Enable debug logging
export DOCKER_NET_DOCTOR_DEBUG=true
export DOCKER_NET_DOCTOR_LOG_LEVEL=debug

# Run with debug logging
docker net-doctor diagnose

# Log to file for analysis
docker net-doctor diagnose 2> debug.log
```

Debug logs include every API call made to Docker, complete request and response payloads, timing information for performance analysis, and stack traces for any errors. This level of detail is invaluable when troubleshooting edge cases or contributing fixes.

### Testing Specific Scenarios

You can create controlled test scenarios to verify the tool correctly detects specific problems:

```bash
# Test 1: Create a network with overlapping subnet
docker network create --subnet 172.20.0.0/16 test-net-1
docker network create --subnet 172.20.0.0/16 test-net-2
docker net-doctor diagnose --category network
# Should detect subnet overlap

# Test 2: Create container with DNS issues
docker run -d --name test-dns --dns 999.999.999.999 nginx
docker net-doctor check dns test-dns
# Should detect DNS resolution failure

# Test 3: Create port conflict
docker run -d -p 8080:80 --name web1 nginx
docker run -d -p 8080:80 --name web2 nginx  # This will fail
docker net-doctor check ports
# Should detect port binding conflict

# Cleanup test resources
docker stop test-dns web1 web2
docker rm test-dns web1 web2
docker network rm test-net-1 test-net-2
```

These controlled tests help verify that the tool correctly identifies specific issues and provides appropriate suggestions.

### Common Debugging Issues

When the tool itself isn't working correctly, several common issues might be the cause. Understanding these helps you troubleshoot more effectively.

**Permission Issues** occur when the tool can't access the Docker socket. The tool requires the same permissions as the `docker` command. If `docker ps` works but the tool reports connection errors, check that the binary has execute permissions and that you're running it as the same user.

**Version Incompatibilities** might occur with very old or very new Docker versions. The tool uses Docker's API version negotiation, but extreme version differences might cause issues. Check the compatibility matrix in our documentation for supported Docker versions.

**Network Restrictions** in corporate environments might prevent the tool from performing certain checks. Some checks require network access from containers, which might be blocked by corporate proxies or firewalls. The verbose mode helps identify which specific operations are failing.

## Testing and Quality Assurance

Ensuring Docker Network Doctor works correctly across different environments requires comprehensive testing. Whether you're contributing to the project or validating it for production use, understanding our testing approach is important.

### Unit Testing

Unit tests verify individual components work correctly in isolation. Our unit tests focus on diagnostic logic, Docker API response parsing, error handling, and output formatting. Running unit tests is straightforward:

```bash
# Run all unit tests
make test

# Run tests for specific package
go test ./internal/diagnostics

# Run tests with coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run tests with race detection
go test -race ./...
```

Unit tests use mocked Docker API responses to ensure consistent behavior regardless of the Docker environment. This approach allows us to test edge cases and error conditions that would be difficult to reproduce with real Docker.

### Integration Testing

Integration tests verify the tool works correctly with real Docker environments. These tests require Docker to be installed and running:

```bash
# Run integration tests
make test-integration

# Run specific integration test
go test -tags=integration ./test/integration -run TestDNSResolution

# Run integration tests with specific Docker version
DOCKER_VERSION=20.10 make test-integration
```

Integration tests create real containers and networks, perform actual diagnostic checks, and verify the tool correctly identifies both working and broken configurations. These tests are slower than unit tests but provide confidence that the tool works in real environments.

### Performance Testing

Performance testing ensures the tool completes diagnostics in reasonable time even in complex environments:

```bash
# Create a complex test environment
for i in {1..50}; do
  docker network create test-net-$i
  docker run -d --name container-$i --network test-net-$i nginx:alpine
done

# Time the diagnostic run
time docker net-doctor diagnose

# Analyze performance with profiling
docker net-doctor diagnose --profile cpu.prof
go tool pprof cpu.prof

# Cleanup
docker stop $(docker ps -q)
docker container prune -f
docker network prune -f
```

Performance testing helps identify bottlenecks and ensures the tool remains responsive even when checking dozens of networks and containers.

### Testing in Different Environments

Docker networking behaves differently across platforms and configurations. Testing in various environments ensures broad compatibility:

```bash
# Test in Docker Desktop on macOS
# Note: Docker Desktop uses a VM, affecting network behavior
docker net-doctor diagnose

# Test in native Linux with systemd
# Different iptables management
sudo systemctl status docker
docker net-doctor diagnose

# Test in Docker-in-Docker (common in CI/CD)
docker run --privileged -d --name dind docker:dind
docker exec dind docker net-doctor diagnose

# Test with Podman (Docker alternative)
# Requires podman-docker compatibility layer
podman system service &
docker net-doctor diagnose

# Test in Kubernetes environment
# Network policies and CNI plugins affect behavior
kubectl run test --image=nginx
kubectl exec test -- docker net-doctor diagnose
```

Each environment has unique characteristics that might affect diagnostic results. Our test matrix covers the most common configurations.

### Continuous Testing

The CI/CD pipeline automatically runs tests on every commit, ensuring code quality remains high:

```yaml
# Tests run automatically on:
- Every push to main branch
- Every pull request
- Nightly scheduled runs
- Release tags

# Test matrix includes:
- Operating Systems: Linux, macOS, Windows
- Go versions: 1.20, 1.21
- Docker versions: 20.10, 23.0, 24.0
```

You can view test results on the GitHub Actions page, and failing tests block merges to maintain code quality.

## Enhancing and Contributing

Docker Network Doctor is designed to be extended and improved by the community. Whether you want to add new diagnostic checks, improve existing ones, or enhance the user experience, this section guides you through the process.

### Architecture Overview

Understanding the tool's architecture is essential for making effective contributions. The codebase is organized into distinct layers with clear responsibilities.

The **Diagnostic Engine** (`internal/diagnostics/engine.go`) orchestrates all checks, manages parallel execution, and aggregates results. When adding new checks, you'll implement the `Check` interface and register your check with the engine.

The **Docker Client Wrapper** (`internal/docker/client.go`) provides a clean abstraction over Docker's API. This wrapper handles version negotiation, error handling, and provides diagnostic-specific methods. New Docker operations should be added here rather than called directly from checks.

The **CLI Layer** (`cmd/docker-net-doctor/main.go` and `internal/cli/`) handles user interaction, command parsing, and output formatting. The CLI uses Cobra for command structure and includes formatters for different output types.

### Adding New Diagnostic Checks

Adding a new diagnostic check involves implementing the Check interface and registering it with the engine. Here's a complete example of adding a check for container resource limits:

```go
// internal/diagnostics/resource_checks.go
package diagnostics

import (
    "context"
    "fmt"
    "time"
)

// ResourceLimitCheck verifies containers have appropriate resource limits
// This prevents single containers from consuming all host resources
type ResourceLimitCheck struct{}

func (c *ResourceLimitCheck) Name() string {
    return "resource_limits"
}

func (c *ResourceLimitCheck) Description() string {
    return "Checking container resource limits and constraints"
}

func (c *ResourceLimitCheck) Severity() Severity {
    return SeverityWarning  // Not critical but important for stability
}

func (c *ResourceLimitCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
    result := &CheckResult{
        CheckName:   c.Name(),
        Timestamp:   time.Now(),
        Details:     make(map[string]interface{}),
        Suggestions: make([]string, 0),
    }
    
    // Get all running containers
    containers, err := client.ListContainers(ctx)
    if err != nil {
        return result, fmt.Errorf("failed to list containers: %w", err)
    }
    
    containersWithoutLimits := 0
    
    for _, container := range containers {
        // Check if container has memory limits
        info, err := client.InspectContainer(ctx, container.ID)
        if err != nil {
            continue
        }
        
        if info.HostConfig.Memory == 0 {
            containersWithoutLimits++
            result.Details[container.Names[0]] = "no memory limit"
        }
    }
    
    if containersWithoutLimits > 0 {
        result.Success = false
        result.Message = fmt.Sprintf("%d containers running without resource limits", 
            containersWithoutLimits)
        result.Suggestions = append(result.Suggestions,
            "Containers without resource limits can consume all host resources",
            "Set memory limits: docker run -m 512m <image>",
            "Set CPU limits: docker run --cpus=\"1.5\" <image>")
    } else {
        result.Success = true
        result.Message = "All containers have appropriate resource limits"
    }
    
    return result, nil
}

// Register the check in engine.go
func (e *DiagnosticEngine) registerDefaultChecks() {
    // ... existing checks ...
    e.checks = append(e.checks, &ResourceLimitCheck{})
}
```

After implementing your check, add corresponding tests to ensure it works correctly:

```go
// internal/diagnostics/resource_checks_test.go
package diagnostics

import (
    "context"
    "testing"
)

func TestResourceLimitCheck(t *testing.T) {
    // Create mock Docker client
    mockClient := &MockDockerClient{
        // Configure mock responses
    }
    
    check := &ResourceLimitCheck{}
    result, err := check.Run(context.Background(), mockClient)
    
    if err != nil {
        t.Fatalf("Check failed with error: %v", err)
    }
    
    if result.Success {
        t.Error("Expected check to fail with containers without limits")
    }
}
```

### Improving Error Messages and Suggestions

Good error messages and suggestions make the difference between a frustrating and helpful tool. When improving messages, consider your audience might include both Docker beginners and experts.

Error messages should clearly describe what was expected versus what was found, explain why this is a problem, and avoid technical jargon when possible. For example, instead of "iptables MASQUERADE rule missing", say "Containers cannot reach external networks because NAT is not configured".

Suggestions should be actionable and specific, ordered by likelihood of success, and include actual commands users can run. Consider different scenarios where the same error might have different causes, and provide suggestions for each.

### Performance Optimizations

As Docker environments grow, performance becomes crucial. Several strategies can improve diagnostic speed.

**Parallel Execution** is already implemented for running checks concurrently. However, you can further optimize by parallelizing operations within checks. For example, when checking multiple containers, test them concurrently rather than sequentially.

**Caching** can significantly improve performance for repeated diagnostics. Consider caching Docker API responses that don't change frequently, like network configurations. Implement cache invalidation when Docker events indicate changes.

**Smart Check Ordering** can improve perceived performance by running quick checks first, giving users immediate feedback while slower checks continue in the background.

### Contributing Guidelines

Before submitting contributions, ensure your code follows project standards. Code should be formatted with `gofmt` and pass all linting checks. Every public function needs clear documentation explaining its purpose and behavior. New features require corresponding tests with good coverage.

The commit message format follows conventional commits for automatic changelog generation:

```
feat: add container resource limit check
fix: correct DNS resolution in custom networks
docs: improve troubleshooting guide
perf: optimize parallel check execution
```

Pull requests should clearly describe what problem they solve, include tests for new functionality, and update documentation as needed. Small, focused PRs are easier to review and merge than large, complex ones.

## Real-World Troubleshooting Scenarios

Understanding how to apply Docker Network Doctor to real problems helps you troubleshoot more effectively. These scenarios represent common issues encountered in production environments.

### Scenario 1: Microservices Can't Communicate

You've deployed a microservices application where the web frontend can't reach the API backend. Both containers are running, but requests fail with "connection refused" errors.

Start with comprehensive diagnostics to understand the overall network state:

```bash
docker net-doctor diagnose
```

The output reveals that containers are on different networks. Docker containers can only communicate directly if they're on the same network. The tool suggests either connecting both containers to the same network or using multiple network attachments.

To fix this, create a shared network and connect both containers:

```bash
# Create a network for the application
docker network create myapp-network

# Connect existing containers to the network
docker network connect myapp-network frontend
docker network connect myapp-network backend

# Verify connectivity
docker net-doctor check connectivity frontend backend
```

The tool confirms connectivity is now working, and provides additional suggestions about using container names for internal communication rather than IP addresses.

### Scenario 2: DNS Resolution Failing Intermittently

Your application works fine most of the time, but occasionally fails with DNS resolution errors. The intermittent nature makes this particularly frustrating to debug.

Run focused DNS diagnostics:

```bash
docker net-doctor check dns --verbose
```

The verbose output reveals that containers are using 8.8.8.8 as their DNS server, but corporate firewall rules intermittently block external DNS. The tool suggests configuring Docker to use corporate DNS servers.

Fix by configuring Docker daemon with appropriate DNS servers:

```bash
# Edit Docker daemon configuration
sudo nano /etc/docker/daemon.json

# Add corporate DNS servers
{
  "dns": ["10.0.0.1", "10.0.0.2"]
}

# Restart Docker
sudo systemctl restart docker

# Verify DNS now works consistently
docker net-doctor check dns
```

### Scenario 3: Container Accessible Internally but Not Externally

A web application works when accessed from other containers but not from the host or external machines. This is a common port binding issue.

Check port bindings specifically:

```bash
docker net-doctor check ports webapp
```

The diagnostics reveal the container exposed port 80, but it's only bound to 127.0.0.1 on the host, making it inaccessible externally. The tool explains that localhost binding is secure but limits accessibility.

Fix by recreating the container with proper port binding:

```bash
# Stop and remove the existing container
docker stop webapp
docker rm webapp

# Run with port accessible from all interfaces
docker run -d --name webapp -p 0.0.0.0:8080:80 myapp

# Or simply use -p without IP to bind to all interfaces
docker run -d --name webapp -p 8080:80 myapp

# Verify port is now accessible
docker net-doctor check ports webapp
```

### Scenario 4: Docker Networking Completely Broken

After a system update, Docker networking stops working entirely. No containers can communicate, and even basic networking fails.

Run comprehensive diagnostics to identify fundamental issues:

```bash
docker net-doctor diagnose --verbose
```

The results show critical failures: docker0 bridge is missing, iptables Docker chain doesn't exist, and IP forwarding is disabled. This indicates Docker's networking subsystem needs complete reinitialization.

Follow the tool's suggestions to restore networking:

```bash
# First, ensure IP forwarding is enabled
sudo sysctl -w net.ipv4.ip_forward=1
echo "net.ipv4.ip_forward=1" | sudo tee -a /etc/sysctl.conf

# Clean up Docker networking
docker network prune -f
docker system prune -f

# Restart Docker to reinitialize networking
sudo systemctl restart docker

# Verify networking is restored
docker net-doctor diagnose
```

The tool confirms all critical components are now functioning, and Docker networking is fully operational.

## Architecture and Design

Understanding the tool's architecture helps you use it more effectively and contribute improvements. The design follows several key principles that ensure reliability and extensibility.

### Design Principles

**Modularity** ensures each component has a single, clear responsibility. Diagnostic checks are independent and can run in any order. The Docker client wrapper isolates API changes, and formatters are separate from diagnostic logic. This modularity makes the code easier to understand, test, and modify.

**Fail-Safe Operation** ensures the tool never makes the situation worse. All operations are read-only by default, with dangerous operations requiring explicit confirmation. Each check handles its own errors gracefully, and one failing check doesn't prevent others from running.

**Progressive Disclosure** presents information at appropriate detail levels. Basic users see simple pass/fail results with clear suggestions. Advanced users can enable verbose mode for technical details. JSON output provides complete data for automation, while interactive mode guides newcomers through troubleshooting.

### Component Interaction Flow

When you run a diagnostic command, several components work together to produce results. The CLI layer parses your command and options, then creates a diagnostic engine with appropriate configuration. The engine loads and orders diagnostic checks based on their dependencies and severity.

Each check uses the Docker client wrapper to gather information about networks, containers, and configuration. The wrapper handles API communication, error recovery, and response parsing. Checks analyze the gathered data and produce structured results with status, details, and suggestions.

Finally, the formatter presents results in your requested format, whether that's colored terminal output, structured JSON, or a comprehensive report.

### Extensibility Points

The architecture provides several extension points for customization. New diagnostic checks can be added by implementing the Check interface. Custom formatters can present results in organization-specific formats. The Docker client wrapper can be extended with new API operations. CLI commands can be added for specialized workflows.

This extensibility ensures the tool can grow to meet new requirements without requiring architectural changes.

## FAQ and Common Issues

### Why does the tool require Docker socket access?

Docker Network Doctor needs to communicate with the Docker daemon to inspect networks, containers, and configuration. This requires the same permissions as running docker commands. The tool only performs read operations unless you explicitly use the --fix flag, making it safe for production use.

### Can I run this in Kubernetes?

Yes, but with limitations. In Kubernetes, the tool can diagnose the Docker daemon on each node, but Kubernetes networking (CNI plugins, network policies) requires different tools. Consider running the tool as a DaemonSet to diagnose each node's Docker networking.

### Why do some checks fail in Docker Desktop?

Docker Desktop runs Docker in a virtual machine, which affects networking behavior. The VM adds a layer of network translation, some host networking features aren't available, and file sharing might affect performance. The tool detects Docker Desktop and adjusts its checks accordingly, but some differences are unavoidable.

### How do I diagnose networking in Docker Swarm mode?

Docker Swarm uses overlay networks with additional complexity. While the tool can check basic connectivity, Swarm-specific features like encrypted overlay networks, load balancing, and service discovery require additional diagnostics. We're working on Swarm-specific checks for a future release.

### The tool says everything is fine, but my containers still can't connect

Some issues are outside Docker's networking stack. Check if applications are actually listening on expected ports, security software might be interfering, or SELinux/AppArmor policies might block connections. Network issues might exist outside Docker entirely. The tool can only diagnose Docker-specific networking components.

### Can the tool fix issues automatically?

The tool primarily focuses on diagnostics, not remediation. This is intentional because automated fixes can be dangerous in production, different environments require different solutions, and understanding the problem is crucial for prevention. The --fix flag enables safe automatic fixes for specific issues, but always with user confirmation.

## Support and Community

Docker Network Doctor is open source and community-driven. Your participation makes the tool better for everyone.

### Getting Help

If you encounter issues or have questions, several resources are available. The GitHub Issues page is the primary place for bug reports and feature requests. Search existing issues first, as your problem might already be addressed. When creating new issues, include your Docker version, operating system, complete error messages, and steps to reproduce the problem.

The Discussions forum is ideal for general questions, sharing troubleshooting experiences, and suggesting enhancements. The community is friendly and helpful, with both maintainers and experienced users contributing answers.

### Contributing

We welcome contributions of all types, not just code. Documentation improvements help new users get started. Bug reports with clear reproduction steps are invaluable. Feature suggestions with use cases guide development. Code contributions add functionality and fix issues.

Before contributing significant changes, open an issue to discuss your proposal. This ensures your work aligns with the project's direction and prevents duplicate effort.

### Roadmap

The project roadmap includes several exciting enhancements. Kubernetes integration will add CNI plugin diagnostics and network policy analysis. Performance monitoring will track metrics over time and identify degrading performance. AI-powered suggestions will use machine learning to provide more intelligent fixes. Cloud platform integration will add specialized checks for AWS, Azure, and GCP.

Your feedback shapes these priorities. Let us know what features would be most valuable for your use cases.

### License and Credits

Docker Network Doctor is released under the MIT License, making it free for both personal and commercial use. The project builds upon the excellent work of the Docker community and wouldn't be possible without the Go programming language, Cobra CLI framework, and Docker's comprehensive APIs.

Special thanks to all contributors who have submitted code, reported bugs, improved documentation, and shared their networking expertise. Your contributions make Docker networking less mysterious and more manageable for developers worldwide.

---

Remember, Docker networking might seem complex, but with systematic diagnostics and clear understanding, any issue can be resolved. Docker Network Doctor is your companion in this journey, turning networking mysteries into understood and solved problems. Happy troubleshooting!
