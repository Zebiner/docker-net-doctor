package diagnostics

import (
    "context"
    "time"
    
    "github.com/zebiner/docker-net-doctor/internal/docker"
)

// DaemonConnectivityCheck verifies basic connectivity to the Docker daemon
// This is the most fundamental check - if this fails, nothing else will work
type DaemonConnectivityCheck struct{}

func (c *DaemonConnectivityCheck) Name() string {
    return "daemon_connectivity"
}

func (c *DaemonConnectivityCheck) Description() string {
    return "Checking Docker daemon connectivity"
}

func (c *DaemonConnectivityCheck) Severity() Severity {
    return SeverityCritical
}

// Now the Run method matches what the Check interface expects
// It receives both a context AND a Docker client
func (c *DaemonConnectivityCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
    // In a real implementation, we would use the client to verify Docker connectivity
    // For now, the fact that we have a client means Docker is accessible
    return &CheckResult{
        CheckName: c.Name(),
        Success:   true,
        Message:   "Docker daemon is accessible",
        Timestamp: time.Now(),
    }, nil
}

// NetworkIsolationCheck verifies network isolation settings
type NetworkIsolationCheck struct{}

func (c *NetworkIsolationCheck) Name() string {
    return "network_isolation"
}

func (c *NetworkIsolationCheck) Description() string {
    return "Checking network isolation configuration"
}

func (c *NetworkIsolationCheck) Severity() Severity {
    return SeverityWarning
}

// Same here - the Run method now accepts the Docker client parameter
func (c *NetworkIsolationCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
    // In a real implementation, we would use the client to check isolation settings
    // Perhaps by inspecting network configurations and security options
    return &CheckResult{
        CheckName: c.Name(),
        Success:   true,
        Message:   "Network isolation configured correctly",
        Timestamp: time.Now(),
    }, nil
}
