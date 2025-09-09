package diagnostics

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBridgeNetworkCheck tests the bridge network diagnostic check
func TestBridgeNetworkCheck_Fixed(t *testing.T) {
	check := &BridgeNetworkCheck{}
	
	// Test basic properties
	assert.Equal(t, "bridge_network", check.Name())
	assert.Equal(t, "Checking Docker bridge network configuration and health", check.Description())
	assert.Equal(t, SeverityCritical, check.Severity())
	
	ctx := context.Background()
	result, err := check.Run(ctx, nil)
	
	// Should return a result even if it fails in test environment
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "bridge_network", result.CheckName)
	assert.False(t, result.Timestamp.IsZero())
}

// TestIPForwardingCheck tests IP forwarding diagnostic check
func TestIPForwardingCheck_Fixed(t *testing.T) {
	check := &IPForwardingCheck{}
	
	// Test basic properties
	assert.Equal(t, "ip_forwarding", check.Name())
	assert.Equal(t, "Checking if IP forwarding is enabled on the host", check.Description())
	assert.Equal(t, SeverityCritical, check.Severity())
	
	ctx := context.Background()
	result, err := check.Run(ctx, nil)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "ip_forwarding", result.CheckName)
	assert.False(t, result.Timestamp.IsZero())
	
	// Should have some details about IP forwarding
	assert.NotNil(t, result.Details)
}

// TestSubnetsOverlap_Fixed tests the subnet overlap helper function
func TestSubnetsOverlap_Fixed(t *testing.T) {
	testCases := []struct {
		name     string
		subnet1  string
		subnet2  string
		expected bool
	}{
		{
			name:     "No Overlap",
			subnet1:  "192.168.1.0/24",
			subnet2:  "192.168.2.0/24",
			expected: false,
		},
		{
			name:     "Exact Overlap",
			subnet1:  "192.168.1.0/24",
			subnet2:  "192.168.1.0/24",
			expected: true,
		},
		{
			name:     "Subnet Contains Other",
			subnet1:  "192.168.0.0/16",
			subnet2:  "192.168.1.0/24",
			expected: true,
		},
		{
			name:     "Other Contains Subnet",
			subnet1:  "192.168.1.0/24",
			subnet2:  "192.168.0.0/16",
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, cidr1, err := net.ParseCIDR(tc.subnet1)
			require.NoError(t, err)
			
			_, cidr2, err := net.ParseCIDR(tc.subnet2)
			require.NoError(t, err)
			
			result := subnetsOverlap(cidr1, cidr2)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestSubnetOverlapCheck tests subnet overlap detection
func TestSubnetOverlapCheck_Fixed(t *testing.T) {
	check := &SubnetOverlapCheck{}
	
	// Test basic properties
	assert.Equal(t, "subnet_overlap", check.Name())
	assert.Equal(t, "Checking for overlapping subnets between Docker networks", check.Description())
	assert.Equal(t, SeverityWarning, check.Severity())
	
	ctx := context.Background()
	result, err := check.Run(ctx, nil)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "subnet_overlap", result.CheckName)
	assert.False(t, result.Timestamp.IsZero())
}

// TestMTUConsistencyCheck tests MTU consistency diagnostic check
func TestMTUConsistencyCheck_Fixed(t *testing.T) {
	check := &MTUConsistencyCheck{}
	
	// Test basic properties
	assert.Equal(t, "mtu_consistency", check.Name())
	assert.Equal(t, "Checking MTU consistency across networks and host interfaces", check.Description())
	assert.Equal(t, SeverityWarning, check.Severity())
	
	ctx := context.Background()
	result, err := check.Run(ctx, nil)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "mtu_consistency", result.CheckName)
	assert.False(t, result.Timestamp.IsZero())
	
	// Details should contain some MTU information
	assert.NotNil(t, result.Details)
}

// TestIptablesCheck tests iptables diagnostic check
func TestIptablesCheck_Fixed(t *testing.T) {
	check := &IptablesCheck{}
	
	// Test basic properties
	assert.Equal(t, "iptables", check.Name())
	assert.Equal(t, "Checking iptables rules for Docker networking", check.Description())
	assert.Equal(t, SeverityCritical, check.Severity())
	
	ctx := context.Background()
	result, err := check.Run(ctx, nil)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "iptables", result.CheckName)
	assert.False(t, result.Timestamp.IsZero())
}

// TestNetworkChecks_InterfaceCompliance_Fixed tests that all network checks implement the Check interface
func TestNetworkChecks_InterfaceCompliance_Fixed(t *testing.T) {
	var checks []Check
	
	checks = append(checks, &BridgeNetworkCheck{})
	checks = append(checks, &IPForwardingCheck{})
	checks = append(checks, &SubnetOverlapCheck{})
	checks = append(checks, &MTUConsistencyCheck{})
	checks = append(checks, &IptablesCheck{})
	
	for _, check := range checks {
		assert.NotEmpty(t, check.Name())
		assert.NotEmpty(t, check.Description())
		assert.True(t, check.Severity() >= SeverityInfo)
		assert.True(t, check.Severity() <= SeverityCritical)
		
		// Test that Run method works (even if it fails due to environment)
		ctx := context.Background()
		result, err := check.Run(ctx, nil)
		
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, check.Name(), result.CheckName)
		assert.False(t, result.Timestamp.IsZero())
	}
}

// TestNetworkChecks_UniqueNames_Fixed tests that check names are unique
func TestNetworkChecks_UniqueNames_Fixed(t *testing.T) {
	checks := []Check{
		&BridgeNetworkCheck{},
		&IPForwardingCheck{},
		&SubnetOverlapCheck{},
		&MTUConsistencyCheck{},
		&IptablesCheck{},
	}
	
	names := make(map[string]bool)
	
	for _, check := range checks {
		name := check.Name()
		assert.False(t, names[name], "Duplicate check name found: %s", name)
		names[name] = true
	}
}

// TestNetworkChecks_SeverityLevels_Fixed tests that severity levels are appropriate
func TestNetworkChecks_SeverityLevels_Fixed(t *testing.T) {
	bridgeCheck := &BridgeNetworkCheck{}
	ipForwardingCheck := &IPForwardingCheck{}
	iptablesCheck := &IptablesCheck{}
	subnetCheck := &SubnetOverlapCheck{}
	mtuCheck := &MTUConsistencyCheck{}
	
	// Critical checks
	assert.Equal(t, SeverityCritical, bridgeCheck.Severity())
	assert.Equal(t, SeverityCritical, ipForwardingCheck.Severity())
	assert.Equal(t, SeverityCritical, iptablesCheck.Severity())
	
	// Warning checks
	assert.Equal(t, SeverityWarning, subnetCheck.Severity())
	assert.Equal(t, SeverityWarning, mtuCheck.Severity())
}

// TestNetworkChecks_ConcurrentExecution_Fixed tests concurrent execution safety
func TestNetworkChecks_ConcurrentExecution_Fixed(t *testing.T) {
	const numGoroutines = 3
	
	checks := []Check{
		&SubnetOverlapCheck{}, // Safe to run concurrently
		&MTUConsistencyCheck{},
	}
	
	results := make(chan *CheckResult, numGoroutines*len(checks))
	errors := make(chan error, numGoroutines*len(checks))
	
	ctx := context.Background()
	
	// Run checks concurrently
	for i := 0; i < numGoroutines; i++ {
		for _, check := range checks {
			go func(c Check) {
				result, err := c.Run(ctx, nil)
				if err != nil {
					errors <- err
				} else {
					results <- result
				}
			}(check)
		}
	}
	
	// Collect results
	var resultCount int
	var errorCount int
	
	timeout := time.After(5 * time.Second)
	
	for resultCount+errorCount < numGoroutines*len(checks) {
		select {
		case result := <-results:
			assert.NotNil(t, result)
			resultCount++
		case err := <-errors:
			t.Errorf("Unexpected error in concurrent execution: %v", err)
			errorCount++
		case <-timeout:
			t.Fatal("Test timed out waiting for goroutines")
		}
	}
	
	assert.Equal(t, numGoroutines*len(checks), resultCount+errorCount)
}

// TestNetworkChecks_ContextCancellation_Fixed tests context cancellation handling
func TestNetworkChecks_ContextCancellation_Fixed(t *testing.T) {
	checks := []Check{
		&SubnetOverlapCheck{},
		&MTUConsistencyCheck{},
	}
	
	for _, check := range checks {
		t.Run(check.Name(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			
			result, err := check.Run(ctx, nil)
			
			// Even with cancelled context, should return result
			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, check.Name(), result.CheckName)
		})
	}
}

// BenchmarkNetworkChecks_Fixed benchmarks network checks
func BenchmarkNetworkChecks_Fixed(b *testing.B) {
	// Only benchmark the safe ones
	checks := []Check{
		&SubnetOverlapCheck{},
		&MTUConsistencyCheck{},
	}
	
	ctx := context.Background()
	
	for _, check := range checks {
		b.Run(check.Name(), func(b *testing.B) {
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				_, err := check.Run(ctx, nil)
				if err != nil {
					b.Fatalf("Check %s failed: %v", check.Name(), err)
				}
			}
		})
	}
}