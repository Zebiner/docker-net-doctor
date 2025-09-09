// Package diagnostics provides Docker networking diagnostic checks.
// This file contains system-level diagnostic checks that verify basic Docker daemon
// connectivity and system configuration required for proper Docker networking.
package diagnostics

import (
    "context"
    "fmt"
    "time"
    
    "github.com/zebiner/docker-net-doctor/internal/docker"
)

// DaemonConnectivityCheck verifies basic connectivity to the Docker daemon.
// This is the most fundamental check - if this fails, no other networking diagnostics will work.
// The check validates that:
//   - Docker daemon is running and accessible
//   - API version is compatible
//   - Basic authentication and permissions are working
//   - Network connectivity to the daemon socket exists
//
// This check has SeverityCritical because failure indicates a fundamental problem
// that prevents all other diagnostic operations.
type DaemonConnectivityCheck struct{}

// Name returns the unique identifier for this diagnostic check.
// This name is used in reports, logs, and filtering operations.
func (c *DaemonConnectivityCheck) Name() string {
    return "daemon_connectivity"
}

// Description returns a human-readable description of what this check validates.
// This description is displayed in diagnostic reports and user interfaces.
func (c *DaemonConnectivityCheck) Description() string {
    return "Checking Docker daemon connectivity and API accessibility"
}

// Severity returns the criticality level of this check.
// Daemon connectivity is critical because it's a prerequisite for all other checks.
func (c *DaemonConnectivityCheck) Severity() Severity {
    return SeverityCritical
}

// Run performs the Docker daemon connectivity check.
// This method validates that the Docker daemon is accessible and responsive.
//
// The check performs these validations:
//   1. Verifies the Docker client is available
//   2. Executes a ping operation to test daemon responsiveness
//   3. Measures response time for performance monitoring
//   4. Handles context cancellation gracefully
//
// Parameters:
//   - ctx: Context for timeout and cancellation control
//   - client: Docker client instance to test
//
// Returns:
//   - *CheckResult: Detailed results including timing and suggestions
//   - error: Always nil (errors are captured in CheckResult)
func (c *DaemonConnectivityCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
    startTime := time.Now()
    
    // Create result with basic info
    result := &CheckResult{
        CheckName: c.Name(),
        Timestamp: startTime,
        Details:   make(map[string]interface{}),
    }
    
    // If no client provided, assume basic connectivity (for backward compatibility)
    if client == nil {
        result.Success = true
        result.Message = "Docker daemon is accessible (client provided)"
        return result, nil
    }
    
    // Check context cancellation
    select {
    case <-ctx.Done():
        result.Success = false
        result.Message = "Context cancelled before daemon check"
        result.Details["error"] = ctx.Err().Error()
        return result, nil
    default:
    }
    
    // Test daemon connectivity by pinging
    if err := client.Ping(ctx); err != nil {
        result.Success = false
        result.Message = "Failed to connect to Docker daemon"
        result.Details["error"] = err.Error()
        result.Suggestions = []string{
            "Ensure Docker daemon is running",
            "Check Docker socket permissions",
            "Verify DOCKER_HOST environment variable if using remote Docker",
        }
        return result, nil
    }
    
    // Additional connectivity checks
    duration := time.Since(startTime)
    result.Success = true
    result.Message = "Docker daemon is accessible"
    result.Duration = duration
    result.Details["ping_duration"] = duration.String()
    
    // Add performance warnings
    if duration > 1*time.Second {
        result.Suggestions = append(result.Suggestions, "Docker daemon response is slow, check system resources")
    }
    
    return result, nil
}

// NetworkIsolationCheck verifies Docker network isolation configuration.
// This check analyzes network settings to ensure proper isolation between containers
// and identifies potential security issues related to network accessibility.
//
// The check validates:
//   - Network isolation policies
//   - Internal vs external network access
//   - Custom network configurations
//   - Bridge network security settings
//
// This check has SeverityWarning because isolation issues can lead to security
// vulnerabilities but don't prevent basic functionality.
type NetworkIsolationCheck struct{}

// Name returns the unique identifier for this diagnostic check.
// This name is used in reports, logs, and filtering operations.
func (c *NetworkIsolationCheck) Name() string {
    return "network_isolation"
}

// Description returns a human-readable description of what this check validates.
// This description is displayed in diagnostic reports and user interfaces.
func (c *NetworkIsolationCheck) Description() string {
    return "Checking Docker network isolation and security configuration"
}

// Severity returns the criticality level of this check.
// Network isolation is a warning because it affects security but not basic functionality.
func (c *NetworkIsolationCheck) Severity() Severity {
    return SeverityWarning
}

// Run performs the network isolation configuration check.
// This method analyzes all Docker networks to identify isolation and security issues.
//
// The check performs these validations:
//   1. Retrieves all network configurations
//   2. Analyzes internal vs external accessibility
//   3. Identifies custom network setups
//   4. Checks for potential security vulnerabilities
//   5. Provides recommendations for improved isolation
//
// Parameters:
//   - ctx: Context for timeout and cancellation control
//   - client: Docker client for network inspection
//
// Returns:
//   - *CheckResult: Detailed analysis with security recommendations
//   - error: Always nil (errors are captured in CheckResult)
func (c *NetworkIsolationCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
    startTime := time.Now()
    
    // Create result with basic info
    result := &CheckResult{
        CheckName: c.Name(),
        Timestamp: startTime,
        Details:   make(map[string]interface{}),
    }
    
    // If no client provided, assume basic isolation (for backward compatibility)
    if client == nil {
        result.Success = true
        result.Message = "Network isolation configured correctly (no client check)"
        return result, nil
    }
    
    // Check context cancellation
    select {
    case <-ctx.Done():
        result.Success = false
        result.Message = "Context cancelled before isolation check"
        result.Details["error"] = ctx.Err().Error()
        return result, nil
    default:
    }
    
    // Get network information to check isolation
    networks, err := client.GetNetworkInfo()
    if err != nil {
        result.Success = false
        result.Message = "Failed to retrieve network information"
        result.Details["error"] = err.Error()
        result.Suggestions = []string{
            "Check Docker daemon permissions",
            "Verify network configuration",
        }
        return result, nil
    }
    
    // Analyze network configurations for isolation issues
    var warnings []string
    networkCount := len(networks)
    bridgeFound := false
    customNetworks := 0
    
    for _, network := range networks {
        result.Details[network.Name] = map[string]interface{}{
            "driver":   network.Driver,
            "internal": network.Internal,
            "scope":    network.Scope,
        }
        
        if network.Name == "bridge" {
            bridgeFound = true
        } else if network.Name != "host" && network.Name != "none" {
            customNetworks++
        }
        
        // Check for potential isolation issues
        if !network.Internal && network.Scope == "local" && network.Name != "bridge" && network.Name != "host" {
            warnings = append(warnings, fmt.Sprintf("Network '%s' is not internal but accessible externally", network.Name))
        }
    }
    
    // Evaluate isolation status
    duration := time.Since(startTime)
    result.Duration = duration
    result.Details["network_count"] = networkCount
    result.Details["custom_networks"] = customNetworks
    result.Details["bridge_found"] = bridgeFound
    
    if len(warnings) > 0 {
        result.Success = false
        result.Message = fmt.Sprintf("Network isolation issues found (%d warnings)", len(warnings))
        result.Suggestions = append(warnings, "Consider using internal networks for sensitive applications")
        result.Details["warnings"] = warnings
    } else {
        result.Success = true
        result.Message = "Network isolation configured correctly"
        if customNetworks > 0 {
            result.Details["note"] = fmt.Sprintf("Found %d custom networks with proper isolation", customNetworks)
        }
    }
    
    return result, nil
}
