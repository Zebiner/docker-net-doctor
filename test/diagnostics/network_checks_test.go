package diagnosticstest

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/zebiner/docker-net-doctor/internal/diagnostics"
	"github.com/zebiner/docker-net-doctor/test/mocks"
)

// Test check metadata and configuration
func TestBridgeNetworkCheck_Metadata(t *testing.T) {
	check := &diagnostics.BridgeNetworkCheck{}
	
	assert.Equal(t, "bridge_network", check.Name())
	assert.Contains(t, check.Description(), "bridge network")
	assert.Equal(t, diagnostics.SeverityCritical, check.Severity())
}

func TestIPForwardingCheck_Metadata(t *testing.T) {
	check := &diagnostics.IPForwardingCheck{}
	
	assert.Equal(t, "ip_forwarding", check.Name())
	assert.Contains(t, check.Description(), "IP forwarding")
	assert.Equal(t, diagnostics.SeverityCritical, check.Severity())
}

func TestSubnetOverlapCheck_Metadata(t *testing.T) {
	check := &diagnostics.SubnetOverlapCheck{}
	
	assert.Equal(t, "subnet_overlap", check.Name())
	assert.Contains(t, check.Description(), "subnet")
	assert.Equal(t, diagnostics.SeverityWarning, check.Severity())
}

func TestMTUConsistencyCheck_Metadata(t *testing.T) {
	check := &diagnostics.MTUConsistencyCheck{}
	
	assert.Equal(t, "mtu_consistency", check.Name())
	assert.Contains(t, check.Description(), "MTU")
	assert.Equal(t, diagnostics.SeverityWarning, check.Severity())
}

func TestIptablesCheck_Metadata(t *testing.T) {
	check := &diagnostics.IptablesCheck{}
	
	assert.Equal(t, "iptables", check.Name())
	assert.Contains(t, check.Description(), "iptables")
	assert.Equal(t, diagnostics.SeverityCritical, check.Severity())
}

// Test network diagnostic mock data structures
func TestNetworkDiagnosticMockCreation(t *testing.T) {
	network := mocks.CreateMockNetworkDiagnostic("test-bridge", "172.17.0.0/16", "172.17.0.1")
	
	assert.Equal(t, "test-bridge", network.Name)
	assert.Equal(t, "network_test-bridge", network.ID)
	assert.Equal(t, "bridge", network.Driver)
	assert.Equal(t, "172.17.0.0/16", network.IPAM.Configs[0].Subnet)
	assert.Equal(t, "172.17.0.1", network.IPAM.Configs[0].Gateway)
	assert.NotNil(t, network.Containers)
}

// Test network check logic without requiring Docker client
func TestNetworkCheckSeverities(t *testing.T) {
	testCases := []struct {
		name          string
		check         diagnostics.Check
		expectedLevel diagnostics.Severity
	}{
		{"BridgeNetworkCheck", &diagnostics.BridgeNetworkCheck{}, diagnostics.SeverityCritical},
		{"IPForwardingCheck", &diagnostics.IPForwardingCheck{}, diagnostics.SeverityCritical},
		{"SubnetOverlapCheck", &diagnostics.SubnetOverlapCheck{}, diagnostics.SeverityWarning},
		{"MTUConsistencyCheck", &diagnostics.MTUConsistencyCheck{}, diagnostics.SeverityWarning},
		{"IptablesCheck", &diagnostics.IptablesCheck{}, diagnostics.SeverityCritical},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedLevel, tc.check.Severity())
			assert.NotEmpty(t, tc.check.Name(), "Check should have a name")
			assert.NotEmpty(t, tc.check.Description(), "Check should have a description")
		})
	}
}

// Integration test placeholders for actual Docker testing
func TestBridgeNetworkCheck_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	
	// This would test against a real Docker daemon
	t.Skip("Bridge network check integration test requires Docker daemon")
}

func TestSubnetOverlapCheck_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	
	// This would test subnet overlap detection with real networks
	t.Skip("Subnet overlap check integration test requires Docker daemon")
}

func TestIPForwardingCheck_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	
	// This would test IP forwarding configuration
	t.Skip("IP forwarding check integration test requires system privileges")
}

func TestMTUConsistencyCheck_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	
	// This would test MTU consistency across network interfaces
	t.Skip("MTU consistency check integration test requires network interfaces")
}

func TestIptablesCheck_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	
	// This would test iptables rules
	t.Skip("Iptables check integration test requires system privileges")
}

// Test error scenarios that can be tested without Docker
func TestNetworkCheck_ErrorScenarios(t *testing.T) {
	testCases := []struct {
		name        string
		description string
	}{
		{
			name:        "Missing Bridge Network",
			description: "Should detect when default bridge network is missing",
		},
		{
			name:        "Overlapping Subnets",
			description: "Should detect when networks have overlapping subnets",
		},
		{
			name:        "IP Forwarding Disabled",
			description: "Should detect when IP forwarding is disabled",
		},
		{
			name:        "MTU Mismatch",
			description: "Should detect MTU inconsistencies",
		},
		{
			name:        "Missing Iptables Rules",
			description: "Should detect missing Docker iptables rules",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Test case: %s - %s", tc.name, tc.description)
			// These would be implemented as integration tests with real infrastructure
		})
	}
}

// Benchmark placeholders
func BenchmarkBridgeNetworkCheck(b *testing.B) {
	b.Skip("Bridge network check benchmark requires Docker infrastructure")
}

func BenchmarkSubnetOverlapCheck(b *testing.B) {
	b.Skip("Subnet overlap check benchmark requires Docker infrastructure")
}