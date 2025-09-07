// internal/diagnostics/connectivity_checks.go
package diagnostics

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// ContainerConnectivityCheck verifies that containers can communicate with each other
type ContainerConnectivityCheck struct{}

func (c *ContainerConnectivityCheck) Name() string {
	return "container_connectivity"
}

func (c *ContainerConnectivityCheck) Description() string {
	return "Checking connectivity between containers on the same network"
}

func (c *ContainerConnectivityCheck) Severity() Severity {
	return SeverityWarning
}

func (c *ContainerConnectivityCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	result := &CheckResult{
		CheckName:   c.Name(),
		Success:     true,
		Timestamp:   time.Now(),
		Details:     make(map[string]interface{}),
		Suggestions: make([]string, 0),
	}

	// Get all running containers
	containers, err := client.ListContainers(ctx)
	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("Failed to list containers: %v", err)
		return result, err
	}

	if len(containers) < 2 {
		result.Message = "Need at least 2 containers to test connectivity"
		result.Details["container_count"] = len(containers)
		return result, nil
	}

	// Group containers by network
	networkGroups := make(map[string][]string) // network -> container IDs
	for _, container := range containers {
		for netName := range container.NetworkSettings.Networks {
			networkGroups[netName] = append(networkGroups[netName], container.ID[:12])
		}
	}

	// Test connectivity within each network
	connectivityIssues := 0
	totalTests := 0

	for network, containerIDs := range networkGroups {
		if len(containerIDs) < 2 {
			continue // Need at least 2 containers in the network
		}

		// Test connectivity between first and second container
		sourceContainer := containerIDs[0]
		targetContainer := containerIDs[1]
		totalTests++

		// Get target container's IP address
		targetInfo, err := client.GetContainerNetworkConfig(targetContainer)
		if err != nil {
			connectivityIssues++
			result.Details[fmt.Sprintf("error_%s", targetContainer)] = err.Error()
			continue
		}

		var targetIP string
		if netEndpoint, ok := targetInfo.Networks[network]; ok {
			targetIP = netEndpoint.IPAddress
		}

		if targetIP == "" {
			connectivityIssues++
			result.Details[fmt.Sprintf("no_ip_%s", targetContainer)] = "No IP address found"
			continue
		}

		// Ping test from source to target
		output, err := client.ExecInContainer(ctx, sourceContainer,
			[]string{"ping", "-c", "1", "-W", "2", targetIP})

		if err != nil || !strings.Contains(output, "1 received") {
			connectivityIssues++
			result.Details[fmt.Sprintf("ping_%s_to_%s", sourceContainer, targetContainer)] = "Failed"
			result.Suggestions = append(result.Suggestions,
				fmt.Sprintf("Check if containers %s and %s are properly connected to network %s",
					sourceContainer, targetContainer, network))
		} else {
			result.Details[fmt.Sprintf("ping_%s_to_%s", sourceContainer, targetContainer)] = "Success"
		}
	}

	result.Details["total_tests"] = totalTests
	result.Details["failed_tests"] = connectivityIssues

	if connectivityIssues > 0 {
		result.Success = false
		result.Message = fmt.Sprintf("Container connectivity issues detected: %d/%d tests failed",
			connectivityIssues, totalTests)
		result.Suggestions = append(result.Suggestions,
			"Ensure containers are on the same network",
			"Check network isolation settings",
			"Verify firewall rules are not blocking container communication")
	} else {
		result.Message = fmt.Sprintf("All container connectivity tests passed (%d tests)", totalTests)
	}

	return result, nil
}

// PortBindingCheck validates port bindings and accessibility
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
		Success:     true,
	}

	containers, err := client.ListContainers(ctx)
	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("Failed to list containers: %v", err)
		return result, err
	}

	portConflicts := make(map[string][]string) // port -> containers using it
	issues := 0
	checkedPorts := 0

	for _, container := range containers {
		// Check if container has any exposed ports
		if container.Ports == nil || len(container.Ports) == 0 {
			continue
		}

		for _, port := range container.Ports {
			if port.PublicPort == 0 {
				continue // No public port mapping
			}

			checkedPorts++
			
			// Build the port key for conflict detection
			portKey := fmt.Sprintf("%d/%s", port.PublicPort, port.Type)
			if port.IP != "" && port.IP != "0.0.0.0" {
				portKey = fmt.Sprintf("%s:%s", port.IP, portKey)
			}

			// Track port usage
			containerName := container.Names[0]
			if len(containerName) > 0 && containerName[0] == '/' {
				containerName = containerName[1:] // Remove leading slash
			}
			portConflicts[portKey] = append(portConflicts[portKey], containerName)

			// Test if port is actually accessible
			hostIP := port.IP
			if hostIP == "" || hostIP == "0.0.0.0" {
				hostIP = "127.0.0.1"
			}

			// Check if port is listening (simplified check)
			address := fmt.Sprintf("%s:%d", hostIP, port.PublicPort)
			conn, err := net.DialTimeout("tcp", address, 2*time.Second)

			if err != nil {
				issues++
				result.Details[fmt.Sprintf("port_%s_%d", containerName, port.PublicPort)] =
					fmt.Sprintf("Port %d not accessible: %v", port.PublicPort, err)
				result.Suggestions = append(result.Suggestions,
					fmt.Sprintf("Check if container %s is running and healthy", containerName))
			} else {
				conn.Close()
				result.Details[fmt.Sprintf("port_%s_%d", containerName, port.PublicPort)] = "Accessible"
			}
		}
	}

	// Check for port conflicts
	conflictCount := 0
	for port, containers := range portConflicts {
		if len(containers) > 1 {
			conflictCount++
			result.Details[fmt.Sprintf("conflict_%s", port)] = containers
			result.Suggestions = append(result.Suggestions,
				fmt.Sprintf("Port conflict on %s: used by %v", port, containers))
		}
	}

	result.Details["ports_checked"] = checkedPorts
	result.Details["ports_with_issues"] = issues
	result.Details["port_conflicts"] = conflictCount

	if issues > 0 || conflictCount > 0 {
		result.Success = false
		result.Message = fmt.Sprintf("Port binding issues detected: %d inaccessible, %d conflicts",
			issues, conflictCount)
		result.Suggestions = append(result.Suggestions,
			"Ensure containers are running and healthy",
			"Check firewall rules for blocked ports",
			"Resolve port conflicts by using different port mappings")
	} else if checkedPorts == 0 {
		result.Message = "No exposed ports found to check"
	} else {
		result.Message = fmt.Sprintf("All %d exposed ports are accessible and conflict-free", checkedPorts)
	}

	return result, nil
}