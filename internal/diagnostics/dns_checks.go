// internal/diagnostics/dns_checks.go
package diagnostics

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// DNSResolutionCheck verifies containers can resolve external DNS names
// This is crucial for containers that need to reach external services
type DNSResolutionCheck struct{}

func (c *DNSResolutionCheck) Name() string {
	return "dns_resolution"
}

func (c *DNSResolutionCheck) Description() string {
	return "Testing DNS resolution capabilities in containers"
}

func (c *DNSResolutionCheck) Severity() Severity {
	return SeverityWarning // Not critical but causes many issues
}

func (c *DNSResolutionCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	result := &CheckResult{
		CheckName:   c.Name(),
		Timestamp:   time.Now(),
		Details:     make(map[string]interface{}),
		Suggestions: make([]string, 0),
	}

	// Test DNS resolution from the host first
	// This helps distinguish between host-level and container-level DNS issues
	hostDNSWorking := true
	_, err := net.LookupHost("google.com")
	if err != nil {
		hostDNSWorking = false
		result.Details["host_dns"] = "failed"
		result.Suggestions = append(result.Suggestions,
			"DNS resolution is failing on the host itself",
			"Check /etc/resolv.conf on the host",
			"Verify network connectivity: ping 8.8.8.8")
	} else {
		result.Details["host_dns"] = "working"
	}

	// Get containers to test
	containers, err := client.ListContainers(ctx)
	if err != nil {
		return result, fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		result.Success = true
		result.Message = "No running containers to test"
		return result, nil
	}

	// Test DNS in first available container
	// In production, you might want to test multiple containers
	testResults := make(map[string]bool)

	for _, container := range containers[:min(3, len(containers))] {
		// Test external DNS resolution
		testResult, err := client.ExecInContainer(ctx, container.ID,
			[]string{"nslookup", "google.com"})

		if err != nil || strings.Contains(testResult, "can't resolve") {
			testResults[container.Names[0]] = false

			// Get container's DNS configuration for debugging
			dnsConfig, _ := client.ExecInContainer(ctx, container.ID,
				[]string{"cat", "/etc/resolv.conf"})
			result.Details[fmt.Sprintf("container_%s_resolv", container.Names[0])] = dnsConfig
		} else {
			testResults[container.Names[0]] = true
		}
	}

	// Analyze results
	failedContainers := 0
	for containerName, success := range testResults {
		if !success {
			failedContainers++
		}
		result.Details[fmt.Sprintf("dns_test_%s", containerName)] = success
	}

	if failedContainers > 0 {
		result.Success = false
		result.Message = fmt.Sprintf("DNS resolution failed in %d/%d containers",
			failedContainers, len(testResults))

		if hostDNSWorking {
			result.Suggestions = append(result.Suggestions,
				"Container DNS is broken but host DNS works",
				"Check Docker daemon DNS settings in /etc/docker/daemon.json",
				"Try setting explicit DNS: docker run --dns 8.8.8.8",
				"Verify firewall isn't blocking DNS (UDP port 53)")
		}
	} else {
		result.Success = true
		result.Message = "DNS resolution working in all tested containers"
	}

	return result, nil
}

// InternalDNSCheck verifies container-to-container DNS resolution
// Docker provides automatic DNS for container names within custom networks
type InternalDNSCheck struct{}

func (c *InternalDNSCheck) Name() string {
	return "internal_dns"
}

func (c *InternalDNSCheck) Description() string {
	return "Checking container-to-container DNS resolution"
}

func (c *InternalDNSCheck) Severity() Severity {
	return SeverityWarning
}

func (c *InternalDNSCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	result := &CheckResult{
		CheckName:   c.Name(),
		Timestamp:   time.Now(),
		Details:     make(map[string]interface{}),
		Suggestions: make([]string, 0),
	}

	// Get custom networks (internal DNS only works on custom networks, not default bridge)
	networks, err := client.GetNetworkInfo()
	if err != nil {
		return result, err
	}

	customNetworks := make([]docker.NetworkDiagnostic, 0)
	for _, net := range networks {
		if net.Name != "bridge" && net.Name != "host" && net.Name != "none" {
			customNetworks = append(customNetworks, net)
		}
	}

	if len(customNetworks) == 0 {
		result.Success = true
		result.Message = "No custom networks found (internal DNS requires custom networks)"
		result.Details["info"] = "Container name resolution only works on user-defined networks"
		return result, nil
	}

	// For each custom network, check if containers can resolve each other
	issues := 0
	for _, network := range customNetworks {
		if len(network.Containers) < 2 {
			continue // Need at least 2 containers to test
		}

		// Get first two containers for testing
		var containers []string
		for _, container := range network.Containers {
			containers = append(containers, container.Name)
			if len(containers) >= 2 {
				break
			}
		}

		// Test if first container can resolve the second by name
		testResult, err := client.ExecInContainer(ctx, containers[0],
			[]string{"ping", "-c", "1", containers[1]})

		if err != nil || strings.Contains(testResult, "bad address") {
			issues++
			result.Details[fmt.Sprintf("network_%s", network.Name)] = "DNS not working"
			result.Suggestions = append(result.Suggestions,
				fmt.Sprintf("Containers in network '%s' cannot resolve each other", network.Name),
				"Ensure containers are on the same network",
				"Use container names, not IDs for internal communication",
				"Check if Docker's embedded DNS server (127.0.0.11) is working")
		} else {
			result.Details[fmt.Sprintf("network_%s", network.Name)] = "DNS working"
		}
	}

	if issues > 0 {
		result.Success = false
		result.Message = fmt.Sprintf("Internal DNS issues detected in %d networks", issues)
	} else {
		result.Success = true
		result.Message = "Internal DNS resolution working correctly"
	}

	return result, nil
}
