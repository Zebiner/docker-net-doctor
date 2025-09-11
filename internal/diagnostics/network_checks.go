// internal/diagnostics/network_checks.go
package diagnostics

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// BridgeNetworkCheck verifies the Docker bridge network is functioning correctly
// This is fundamental - if the bridge is broken, most container networking fails
type BridgeNetworkCheck struct{}

func (c *BridgeNetworkCheck) Name() string {
	return "bridge_network"
}

func (c *BridgeNetworkCheck) Description() string {
	return "Checking Docker bridge network configuration and health"
}

func (c *BridgeNetworkCheck) Severity() Severity {
	return SeverityCritical // Bridge network is essential for default networking
}

func (c *BridgeNetworkCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	result := &CheckResult{
		CheckName:   c.Name(),
		Timestamp:   time.Now(),
		Details:     make(map[string]interface{}),
		Suggestions: make([]string, 0),
	}

	// Get information about the default bridge network
	networks, err := client.GetNetworkInfo()
	if err != nil {
		return result, err
	}

	var bridgeNetwork *docker.NetworkDiagnostic
	for _, net := range networks {
		if net.Name == "bridge" {
			bridgeNetwork = &net
			break
		}
	}

	if bridgeNetwork == nil {
		result.Success = false
		result.Message = "Default bridge network not found"
		result.Suggestions = append(result.Suggestions,
			"The Docker bridge network is missing. This is critical for container networking.",
			"Try restarting the Docker daemon: systemctl restart docker")
		return result, nil
	}

	// Check if the bridge has a valid subnet
	if len(bridgeNetwork.IPAM.Configs) == 0 {
		result.Success = false
		result.Message = "Bridge network has no IPAM configuration"
		result.Suggestions = append(result.Suggestions,
			"Recreate the bridge network with: docker network create bridge")
		return result, nil
	}

	// Verify the bridge interface exists on the host
	bridgeInterface, err := net.InterfaceByName("docker0")
	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("docker0 bridge interface not found: %v", err)
		result.Suggestions = append(result.Suggestions,
			"The docker0 bridge interface is missing from the host",
			"This usually indicates a Docker daemon issue",
			"Try: sudo systemctl restart docker")
		return result, nil
	}

	// Check if the interface is up
	if bridgeInterface.Flags&net.FlagUp == 0 {
		result.Success = false
		result.Message = "docker0 bridge interface is down"
		result.Suggestions = append(result.Suggestions,
			"Bring up the interface: sudo ip link set docker0 up")
		return result, nil
	}

	result.Success = true
	result.Message = "Bridge network is properly configured"
	result.Details["subnet"] = bridgeNetwork.IPAM.Configs[0].Subnet
	result.Details["gateway"] = bridgeNetwork.IPAM.Configs[0].Gateway
	result.Details["interface_status"] = "UP"

	return result, nil
}

// IPForwardingCheck ensures IP forwarding is enabled on the host
// Without this, containers cannot communicate with external networks
type IPForwardingCheck struct{}

func (c *IPForwardingCheck) Name() string {
	return "ip_forwarding"
}

func (c *IPForwardingCheck) Description() string {
	return "Checking if IP forwarding is enabled on the host"
}

func (c *IPForwardingCheck) Severity() Severity {
	return SeverityCritical
}

func (c *IPForwardingCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	result := &CheckResult{
		CheckName:   c.Name(),
		Timestamp:   time.Now(),
		Details:     make(map[string]interface{}),
		Suggestions: make([]string, 0),
	}

	// Check IPv4 forwarding
	cmd := exec.CommandContext(ctx, "sysctl", "net.ipv4.ip_forward")
	output, err := cmd.Output()
	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("Failed to check IP forwarding: %v", err)
		return result, nil
	}

	ipv4Forward := strings.TrimSpace(string(output))
	result.Details["ipv4_forwarding"] = ipv4Forward

	if !strings.Contains(ipv4Forward, "= 1") {
		result.Success = false
		result.Message = "IP forwarding is disabled on the host"
		result.Suggestions = append(result.Suggestions,
			"Enable IP forwarding to allow container network traffic",
			"Temporary: sudo sysctl -w net.ipv4.ip_forward=1",
			"Permanent: Add 'net.ipv4.ip_forward=1' to /etc/sysctl.conf")
		return result, nil
	}

	result.Success = true
	result.Message = "IP forwarding is properly enabled"

	return result, nil
}

// SubnetOverlapCheck detects overlapping subnets between Docker networks
// Overlapping subnets cause routing confusion and connectivity issues
type SubnetOverlapCheck struct{}

func (c *SubnetOverlapCheck) Name() string {
	return "subnet_overlap"
}

func (c *SubnetOverlapCheck) Description() string {
	return "Checking for overlapping subnets between Docker networks"
}

func (c *SubnetOverlapCheck) Severity() Severity {
	return SeverityWarning
}

func (c *SubnetOverlapCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	result := &CheckResult{
		CheckName:   c.Name(),
		Timestamp:   time.Now(),
		Details:     make(map[string]interface{}),
		Suggestions: make([]string, 0),
	}

	networks, err := client.GetNetworkInfo()
	if err != nil {
		return result, err
	}

	// Collect all subnets
	type subnet struct {
		network string
		cidr    *net.IPNet
	}

	subnets := make([]subnet, 0)

	for _, network := range networks {
		for _, config := range network.IPAM.Configs {
			if config.Subnet != "" {
				_, cidr, err := net.ParseCIDR(config.Subnet)
				if err != nil {
					continue // Skip invalid subnets
				}
				subnets = append(subnets, subnet{
					network: network.Name,
					cidr:    cidr,
				})
			}
		}
	}

	// Check for overlaps
	overlaps := make([]string, 0)
	for i := 0; i < len(subnets); i++ {
		for j := i + 1; j < len(subnets); j++ {
			if subnetsOverlap(subnets[i].cidr, subnets[j].cidr) {
				overlap := fmt.Sprintf("%s (%s) overlaps with %s (%s)",
					subnets[i].network, subnets[i].cidr.String(),
					subnets[j].network, subnets[j].cidr.String())
				overlaps = append(overlaps, overlap)
			}
		}
	}

	if len(overlaps) > 0 {
		result.Success = false
		result.Message = fmt.Sprintf("Found %d subnet overlaps", len(overlaps))
		result.Details["overlaps"] = overlaps
		result.Suggestions = append(result.Suggestions,
			"Overlapping subnets can cause routing issues",
			"Consider using non-overlapping ranges like:",
			"  - 172.20.0.0/16 for network1",
			"  - 172.21.0.0/16 for network2",
			"Remove unused networks: docker network prune")
	} else {
		result.Success = true
		result.Message = "No subnet overlaps detected"
		result.Details["total_networks"] = len(networks)
		result.Details["total_subnets"] = len(subnets)
	}

	return result, nil
}

// MTUConsistencyCheck verifies MTU settings are consistent
// MTU mismatches cause packet fragmentation and performance issues
type MTUConsistencyCheck struct{}

func (c *MTUConsistencyCheck) Name() string {
	return "mtu_consistency"
}

func (c *MTUConsistencyCheck) Description() string {
	return "Checking MTU consistency across networks and host interfaces"
}

func (c *MTUConsistencyCheck) Severity() Severity {
	return SeverityWarning
}

func (c *MTUConsistencyCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	result := &CheckResult{
		CheckName:   c.Name(),
		Timestamp:   time.Now(),
		Details:     make(map[string]interface{}),
		Suggestions: make([]string, 0),
	}

	// Get host interface MTU
	hostInterface, err := net.InterfaceByName("eth0") // Primary interface
	if err != nil {
		// Try alternative names
		hostInterface, err = net.InterfaceByName("ens33")
		if err != nil {
			hostInterface, err = net.InterfaceByName("enp0s3")
		}
	}

	hostMTU := 1500 // Default
	if hostInterface != nil {
		hostMTU = hostInterface.MTU
		result.Details["host_mtu"] = hostMTU
	}

	// Check Docker bridge MTU
	bridgeInterface, err := net.InterfaceByName("docker0")
	if err == nil {
		result.Details["bridge_mtu"] = bridgeInterface.MTU

		if bridgeInterface.MTU > hostMTU {
			result.Success = false
			result.Message = fmt.Sprintf("Docker bridge MTU (%d) exceeds host MTU (%d)",
				bridgeInterface.MTU, hostMTU)
			result.Suggestions = append(result.Suggestions,
				"MTU mismatch can cause packet loss and connectivity issues",
				fmt.Sprintf("Set Docker MTU to match host: docker network create --opt com.docker.network.driver.mtu=%d", hostMTU),
				"Or configure in /etc/docker/daemon.json: {\"mtu\": "+fmt.Sprintf("%d", hostMTU)+"}")
			return result, nil
		}
	}

	result.Success = true
	result.Message = "MTU settings are consistent"

	return result, nil
}

// Helper function to check if two subnets overlap
func subnetsOverlap(a, b *net.IPNet) bool {
	// Check if either subnet contains the other's network address
	return a.Contains(b.IP) || b.Contains(a.IP)
}

// IptablesCheck verifies iptables rules for Docker
// Docker heavily relies on iptables for NAT and filtering
type IptablesCheck struct{}

func (c *IptablesCheck) Name() string {
	return "iptables"
}

func (c *IptablesCheck) Description() string {
	return "Checking iptables rules for Docker networking"
}

func (c *IptablesCheck) Severity() Severity {
	return SeverityCritical
}

func (c *IptablesCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	result := &CheckResult{
		CheckName:   c.Name(),
		Timestamp:   time.Now(),
		Details:     make(map[string]interface{}),
		Suggestions: make([]string, 0),
	}

	// Check if Docker chain exists in iptables
	cmd := exec.CommandContext(ctx, "iptables", "-L", "DOCKER", "-n")
	output, err := cmd.CombinedOutput()

	if err != nil {
		if strings.Contains(string(output), "No chain/target/match") {
			result.Success = false
			result.Message = "Docker iptables chain is missing"
			result.Suggestions = append(result.Suggestions,
				"Docker iptables rules are missing - containers won't have network access",
				"Restart Docker to recreate rules: sudo systemctl restart docker",
				"Ensure iptables is not being managed by another tool (firewalld, ufw)")
		} else {
			result.Success = false
			result.Message = fmt.Sprintf("Failed to check iptables: %v", err)
			result.Suggestions = append(result.Suggestions,
				"Ensure you have permission to read iptables (run with sudo)",
				"Check if iptables service is running")
		}
		return result, nil
	}

	// Check for MASQUERADE rule (needed for outbound NAT)
	natCmd := exec.CommandContext(ctx, "iptables", "-t", "nat", "-L", "POSTROUTING", "-n")
	natOutput, _ := natCmd.Output()

	if !strings.Contains(string(natOutput), "MASQUERADE") {
		result.Success = false
		result.Message = "NAT MASQUERADE rule missing for Docker"
		result.Suggestions = append(result.Suggestions,
			"Containers cannot access external networks without NAT",
			"Restart Docker daemon to restore NAT rules")
		return result, nil
	}

	result.Success = true
	result.Message = "Docker iptables rules are properly configured"
	result.Details["docker_chain"] = "present"
	result.Details["nat_masquerade"] = "configured"

	return result, nil
}
