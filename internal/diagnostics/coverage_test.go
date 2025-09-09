package diagnostics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAllChecks_BasicProperties tests basic properties of all network-related checks
func TestAllChecks_BasicProperties(t *testing.T) {
	// Network checks
	bridgeCheck := &BridgeNetworkCheck{}
	assert.Equal(t, "bridge_network", bridgeCheck.Name())
	assert.Equal(t, "Checking Docker bridge network configuration and health", bridgeCheck.Description())
	assert.Equal(t, SeverityCritical, bridgeCheck.Severity())

	ipForwardingCheck := &IPForwardingCheck{}
	assert.Equal(t, "ip_forwarding", ipForwardingCheck.Name())
	assert.Equal(t, "Checking if IP forwarding is enabled on the host", ipForwardingCheck.Description())
	assert.Equal(t, SeverityCritical, ipForwardingCheck.Severity())

	subnetCheck := &SubnetOverlapCheck{}
	assert.Equal(t, "subnet_overlap", subnetCheck.Name())
	assert.Equal(t, "Checking for overlapping subnets between Docker networks", subnetCheck.Description())
	assert.Equal(t, SeverityWarning, subnetCheck.Severity())

	mtuCheck := &MTUConsistencyCheck{}
	assert.Equal(t, "mtu_consistency", mtuCheck.Name())
	assert.Equal(t, "Checking MTU consistency across networks and host interfaces", mtuCheck.Description())
	assert.Equal(t, SeverityWarning, mtuCheck.Severity())

	iptablesCheck := &IptablesCheck{}
	assert.Equal(t, "iptables", iptablesCheck.Name())
	assert.Equal(t, "Checking iptables rules for Docker networking", iptablesCheck.Description())
	assert.Equal(t, SeverityCritical, iptablesCheck.Severity())

	// DNS checks
	dnsResolutionCheck := &DNSResolutionCheck{}
	assert.Equal(t, "dns_resolution", dnsResolutionCheck.Name())
	assert.Equal(t, "Testing DNS resolution capabilities in containers", dnsResolutionCheck.Description())
	assert.Equal(t, SeverityWarning, dnsResolutionCheck.Severity())

	internalDNSCheck := &InternalDNSCheck{}
	assert.Equal(t, "internal_dns", internalDNSCheck.Name())
	assert.Equal(t, "Checking container-to-container DNS resolution", internalDNSCheck.Description())
	assert.Equal(t, SeverityWarning, internalDNSCheck.Severity())

	// Connectivity checks
	connectivityCheck := &ContainerConnectivityCheck{}
	assert.Equal(t, "container_connectivity", connectivityCheck.Name())
	assert.Equal(t, "Checking connectivity between containers on the same network", connectivityCheck.Description())
	assert.Equal(t, SeverityWarning, connectivityCheck.Severity())

	portBindingCheck := &PortBindingCheck{}
	assert.Equal(t, "port_binding", portBindingCheck.Name())
	assert.Equal(t, "Checking container port bindings and accessibility", portBindingCheck.Description())
	assert.Equal(t, SeverityWarning, portBindingCheck.Severity())
}

// TestAllChecks_InterfaceCompliance tests that all checks implement the Check interface
func TestAllChecks_InterfaceCompliance(t *testing.T) {
	var checks []Check

	// Add all network-related checks
	checks = append(checks, &BridgeNetworkCheck{})
	checks = append(checks, &IPForwardingCheck{})
	checks = append(checks, &SubnetOverlapCheck{})
	checks = append(checks, &MTUConsistencyCheck{})
	checks = append(checks, &IptablesCheck{})
	checks = append(checks, &DNSResolutionCheck{})
	checks = append(checks, &InternalDNSCheck{})
	checks = append(checks, &ContainerConnectivityCheck{})
	checks = append(checks, &PortBindingCheck{})

	for _, check := range checks {
		assert.NotEmpty(t, check.Name())
		assert.NotEmpty(t, check.Description())
		assert.True(t, check.Severity() >= SeverityInfo)
		assert.True(t, check.Severity() <= SeverityCritical)
	}
}

// TestAllChecks_UniqueNames tests that all check names are unique
func TestAllChecks_UniqueNames(t *testing.T) {
	checks := []Check{
		&BridgeNetworkCheck{},
		&IPForwardingCheck{},
		&SubnetOverlapCheck{},
		&MTUConsistencyCheck{},
		&IptablesCheck{},
		&DNSResolutionCheck{},
		&InternalDNSCheck{},
		&ContainerConnectivityCheck{},
		&PortBindingCheck{},
	}

	names := make(map[string]bool)

	for _, check := range checks {
		name := check.Name()
		assert.False(t, names[name], "Duplicate check name found: %s", name)
		names[name] = true
	}
}

// TestAllChecks_SeverityDistribution tests severity level distribution
func TestAllChecks_SeverityDistribution(t *testing.T) {
	criticalChecks := []Check{
		&BridgeNetworkCheck{},
		&IPForwardingCheck{},
		&IptablesCheck{},
	}

	warningChecks := []Check{
		&SubnetOverlapCheck{},
		&MTUConsistencyCheck{},
		&DNSResolutionCheck{},
		&InternalDNSCheck{},
		&ContainerConnectivityCheck{},
		&PortBindingCheck{},
	}

	for _, check := range criticalChecks {
		assert.Equal(t, SeverityCritical, check.Severity(), "Check %s should be critical", check.Name())
	}

	for _, check := range warningChecks {
		assert.Equal(t, SeverityWarning, check.Severity(), "Check %s should be warning", check.Name())
	}
}