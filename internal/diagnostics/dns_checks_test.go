package diagnostics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDNSResolutionCheck_Fixed tests the DNS resolution diagnostic check
func TestDNSResolutionCheck_Fixed(t *testing.T) {
	check := &DNSResolutionCheck{}
	
	// Test basic properties
	assert.Equal(t, "dns_resolution", check.Name())
	assert.Equal(t, "Testing DNS resolution capabilities in containers", check.Description())
	assert.Equal(t, SeverityWarning, check.Severity())
	
	ctx := context.Background()
	result, err := check.Run(ctx, nil)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "dns_resolution", result.CheckName)
	assert.False(t, result.Timestamp.IsZero())
	assert.NotNil(t, result.Details)
	
	// Should have host_dns status
	if hostDNS, exists := result.Details["host_dns"]; exists {
		assert.Contains(t, []string{"working", "failed"}, hostDNS)
	}
}

// TestInternalDNSCheck_Fixed tests the internal DNS resolution check
func TestInternalDNSCheck_Fixed(t *testing.T) {
	check := &InternalDNSCheck{}
	
	// Test basic properties
	assert.Equal(t, "internal_dns", check.Name())
	assert.Equal(t, "Checking container-to-container DNS resolution", check.Description())
	assert.Equal(t, SeverityWarning, check.Severity())
	
	ctx := context.Background()
	result, err := check.Run(ctx, nil)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "internal_dns", result.CheckName)
	assert.False(t, result.Timestamp.IsZero())
	assert.NotNil(t, result.Details)
}

// TestDNSChecks_InterfaceCompliance_Fixed tests that DNS checks implement the Check interface
func TestDNSChecks_InterfaceCompliance_Fixed(t *testing.T) {
	var checks []Check
	
	checks = append(checks, &DNSResolutionCheck{})
	checks = append(checks, &InternalDNSCheck{})
	
	for _, check := range checks {
		assert.NotEmpty(t, check.Name())
		assert.NotEmpty(t, check.Description())
		assert.True(t, check.Severity() >= SeverityInfo)
		assert.True(t, check.Severity() <= SeverityCritical)
		
		// Test that Run method works
		ctx := context.Background()
		result, err := check.Run(ctx, nil)
		
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, check.Name(), result.CheckName)
		assert.False(t, result.Timestamp.IsZero())
	}
}

// TestDNSChecks_UniqueNames_Fixed tests that DNS check names are unique
func TestDNSChecks_UniqueNames_Fixed(t *testing.T) {
	checks := []Check{
		&DNSResolutionCheck{},
		&InternalDNSCheck{},
	}
	
	names := make(map[string]bool)
	
	for _, check := range checks {
		name := check.Name()
		assert.False(t, names[name], "Duplicate check name found: %s", name)
		names[name] = true
	}
}

// TestDNSChecks_SeverityLevels_Fixed tests that severity levels are appropriate
func TestDNSChecks_SeverityLevels_Fixed(t *testing.T) {
	dnsResolutionCheck := &DNSResolutionCheck{}
	internalDNSCheck := &InternalDNSCheck{}
	
	// Both DNS checks should be warnings (not critical but important)
	assert.Equal(t, SeverityWarning, dnsResolutionCheck.Severity())
	assert.Equal(t, SeverityWarning, internalDNSCheck.Severity())
}

// TestDNSChecks_ConcurrentExecution_Fixed tests concurrent execution safety
func TestDNSChecks_ConcurrentExecution_Fixed(t *testing.T) {
	const numGoroutines = 3
	
	checks := []Check{
		&DNSResolutionCheck{},
		&InternalDNSCheck{},
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

// TestDNSChecks_ContextCancellation_Fixed tests context cancellation handling
func TestDNSChecks_ContextCancellation_Fixed(t *testing.T) {
	checks := []Check{
		&DNSResolutionCheck{},
		&InternalDNSCheck{},
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

// TestDNSChecks_RealNetworkConditions_Fixed tests with actual network conditions
func TestDNSChecks_RealNetworkConditions_Fixed(t *testing.T) {
	// Test DNS resolution check with real network
	dnsCheck := &DNSResolutionCheck{}
	ctx := context.Background()
	
	result, err := dnsCheck.Run(ctx, nil)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "dns_resolution", result.CheckName)
	
	// Should have host DNS information
	assert.NotNil(t, result.Details)
	if hostDNS, exists := result.Details["host_dns"]; exists {
		assert.Contains(t, []interface{}{"working", "failed"}, hostDNS)
	}
	
	// Test internal DNS check with real network
	internalDNSCheck := &InternalDNSCheck{}
	
	result2, err := internalDNSCheck.Run(ctx, nil)
	
	require.NoError(t, err)
	assert.NotNil(t, result2)
	assert.Equal(t, "internal_dns", result2.CheckName)
	assert.NotNil(t, result2.Details)
}

// BenchmarkDNSChecks_Fixed benchmarks DNS checks
func BenchmarkDNSChecks_Fixed(b *testing.B) {
	checks := []Check{
		&DNSResolutionCheck{},
		&InternalDNSCheck{},
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

// TestDNSChecks_ResultStructures_Fixed tests that results have expected structures
func TestDNSChecks_ResultStructures_Fixed(t *testing.T) {
	checks := []Check{
		&DNSResolutionCheck{},
		&InternalDNSCheck{},
	}
	
	ctx := context.Background()
	
	for _, check := range checks {
		t.Run(check.Name(), func(t *testing.T) {
			result, err := check.Run(ctx, nil)
			
			require.NoError(t, err)
			require.NotNil(t, result)
			
			// Test required fields
			assert.Equal(t, check.Name(), result.CheckName)
			assert.False(t, result.Timestamp.IsZero())
			assert.NotEmpty(t, result.Message)
			assert.NotNil(t, result.Details)
			assert.NotNil(t, result.Suggestions)
		})
	}
}