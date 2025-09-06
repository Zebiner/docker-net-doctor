package diagnostics

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// ContainerConnectivityCheck tests basic connectivity between containers
type ContainerConnectivityCheck struct{}

func (c *ContainerConnectivityCheck) Name() string {
	return "container_connectivity"
}

func (c *ContainerConnectivityCheck) Description() string {
	return "Testing network connectivity between containers"
}

func (c *ContainerConnectivityCheck) Severity() Severity {
	return SeverityWarning
}

func (c *ContainerConnectivityCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	result := &CheckResult{
		CheckName:   c.Name(),
		Timestamp:   time.Now(),
		Details:     make(map[string]interface{}),
		Suggestions: make([]string, 0),
	}

	// Get all running containers grouped by network
	networks, err := client.GetNetworkInfo()
	if err != nil {
		return result, err
	}

	connectivityTests := make([]string, 0)
	failedTests := 0

	for _, network := range networks {
		if len(network.Containers) < 2 {
			continue
		}

		// Test connectivity between first two containers
		var testPair []docker.ContainerEndpoint
		for _, container := range network.Containers {
			testPair = append(testPair, container)
			if len(testPair) >= 2 {
				break
			}
		}

		// Extract IP from IPv4Address (format: "IP/prefix")
		sourceIP := strings.Split(testPair[0].IPv4Address, "/")[0]
		targetIP := strings.Split(testPair[1].IPv4Address, "/")[0]

		if sourceIP != "" && targetIP != "" {
			// Test ICMP connectivity
			testResult, err := client.ExecInContainer(ctx, testPair[0].Name,
				[]string{"ping", "-c", "1", "-W", "2", targetIP})

			testName := fmt.Sprintf("%s->%s on %s",
				testPair[0].Name, testPair[1].Name, network.Name)

			if err != nil || strings.Contains(testResult, "100% packet loss") {
				failedTests++
				connectivityTests = append(connectivityTests,
					fmt.Sprintf("FAILED: %s", testName))

				// Provide network-specific suggestions
				if network.Name == "bridge" {
					result.Suggestions = append(result.Suggestions,
						"Default bridge network has limited connectivity features",
						"Consider using a custom network for better isolation and DNS")
				}
			} else {
				connectivityTests = append(connectivityTests,
					fmt.Sprintf("SUCCESS: %s", testName))
			}
		}
	}

	result.Details["connectivity_tests"] = connectivityTests
	result.Details["total_tests"] = len(connectivityTests)
	result.Details["failed_tests"] = failedTests

	if failedTests > 0 {
		result.Success = false
		result.Message = fmt.Sprintf("%d/%d connectivity tests failed",
			failedTests, len(connectivityTests))
		result.Suggestions = append(result.Suggestions,
			"Check if containers are on the same network",
			"Verify iptables rules aren't blocking traffic",
			"Ensure network isolation settings are correct")
	} else if len(connectivityTests) == 0 {
		result.Success = true
		result.Message = "No multi-container networks to test"
	} else {
		result.Success = true
		result.Message = "All container connectivity tests passed"
	}

	return result, nil
}

// PortBindingCheck verifies port bindings are correctly configured
type PortBindingCheck struct{}

func (c *PortBindingCheck) Name() string {
	return "port_binding"
}

func (c *PortBindingCheck) Description() string {
	return "Checking container port bindings and accessibility"
}

func (c *PortBindingCheck) Severity() Severity {
	return SeverityWarning
}

func (c *PortBindingCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	result := &CheckResult{
		CheckName:   c.Name(),
		Timestamp:   time.Now(),
		Details:     make(map[string]interface{}),
		Suggestions: make([]string, 0),
	}

	containers, err := client.ListContainers(ctx)
	if err != nil {
		return result, err
	}

	portConflicts := make(map[string][]string) // port -> containers using it
	issues := 0

	for _, container := range containers {
		containerInfo, err := client.GetContainerNetworkConfig(container.ID)
		if err != nil {
			continue
		}

		for port, bindings := range containerInfo.PortBindings {
			for _, binding := range bindings {
				hostPort := binding.HostPort
				if hostPort == "" {
					continue
				}

				// Check for port conflicts
				key := fmt.Sprintf("%s:%s", binding.HostIP, hostPort)
				if binding.HostIP == "" || binding.HostIP == "0.0.0.0" {
					key = hostPort // All interfaces
				}

				portConflicts[key] = append(portConflicts[key], container.Names[0])

				// Test if port is actually accessible
				hostIP := binding.HostIP
				if hostIP == "" || hostIP == "0.0.0.0" {
					hostIP = "127.0.0.1"
				}

				// Check if port is listening (simplified check)
				conn, err := net.DialTimeout("tcp",
					fmt.Sprintf("%s:%s", hostIP, hostPort),
					2*time.Second)

				if err != nil {
					issues++
					result.Details[fmt.Sprintf("port_%s_%s", container.Names[0], port)] =
						fmt.Sprintf("Port %s not accessible: %v", hostPort, err)
				} else {
					conn.Close()
					result.Details[fmt.Sprintf("port_%s_%s", container.Names[0], port)] =
						fmt.Sprintf("Port %s accessible", hostPort)
				}
			}
		}
	}

	// Check for conflicts
	conflicts := make([]string, 0)
	for port, containers := range portConflicts {
		if len(containers) > 1 {
			conflicts = append(conflicts,
				fmt.Sprintf("Port %s used by: %s", port, strings.Join(containers, ", ")))
		}
	}

	if len(conflicts) > 0 {
		result.Success = false
		result.Message = "Port binding conflicts detected"
		result.Details["conflicts"] = conflicts
		result.Suggestions = append(result.Suggestions,
			"Multiple containers trying to bind to the same port",
			"Use different host ports for each container",
			"Consider using a reverse proxy (nginx, traefik) instead")
	} else if issues > 0 {
		result.Success = false
		result.Message = fmt.Sprintf("%d port binding issues detected", issues)
		result.Suggestions = append(result.Suggestions,
			"Some bound ports are not accessible",
			"Check if the application inside the container is running",
			"Verify firewall rules allow traffic to these ports")
	} else {
		result.Success = true
		result.Message = "All port bindings are correctly configured"
	}

	return result, nil
}

// Helper function for minimum
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
