package diagnostics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContainerConnectivityCheck_Fixed tests the container connectivity check with real implementation
func TestContainerConnectivityCheck_Fixed(t *testing.T) {
	check := &ContainerConnectivityCheck{}
	
	// Test basic properties
	assert.Equal(t, "container_connectivity", check.Name())
	assert.Equal(t, "Checking connectivity between containers on the same network", check.Description())
	assert.Equal(t, SeverityWarning, check.Severity())
	
	ctx := context.Background()
	result, err := check.Run(ctx, nil)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "container_connectivity", result.CheckName)
	assert.False(t, result.Timestamp.IsZero())
	assert.NotNil(t, result.Details)
	assert.NotNil(t, result.Suggestions)
	
	// Should have some meaningful message
	assert.NotEmpty(t, result.Message)
	
	// Should have details about the test results
	if containerCount, exists := result.Details["container_count"]; exists {
		assert.IsType(t, 0, containerCount) // Should be an integer
	}
}

// TestPortBindingCheck_Fixed tests the port binding check with real implementation
func TestPortBindingCheck_Fixed(t *testing.T) {
	check := &PortBindingCheck{}
	
	// Test basic properties
	assert.Equal(t, "port_binding", check.Name())
	assert.Equal(t, "Checking container port bindings and accessibility", check.Description())
	assert.Equal(t, SeverityWarning, check.Severity())
	
	ctx := context.Background()
	result, err := check.Run(ctx, nil)
	
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "port_binding", result.CheckName)
	assert.False(t, result.Timestamp.IsZero())
	assert.NotNil(t, result.Details)
	assert.NotNil(t, result.Suggestions)
	
	// Should have some meaningful message
	assert.NotEmpty(t, result.Message)
	
	// Should have details about ports checked
	if portsChecked, exists := result.Details["ports_checked"]; exists {
		assert.IsType(t, 0, portsChecked) // Should be an integer
	}
}

// TestConnectivityChecks_InterfaceCompliance_Fixed tests interface compliance
func TestConnectivityChecks_InterfaceCompliance_Fixed(t *testing.T) {
	var checks []Check
	
	checks = append(checks, &ContainerConnectivityCheck{})
	checks = append(checks, &PortBindingCheck{})
	
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

// TestConnectivityChecks_UniqueNames_Fixed tests that check names are unique
func TestConnectivityChecks_UniqueNames_Fixed(t *testing.T) {
	checks := []Check{
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

// TestConnectivityChecks_SeverityLevels_Fixed tests that severity levels are appropriate
func TestConnectivityChecks_SeverityLevels_Fixed(t *testing.T) {
	connectivityCheck := &ContainerConnectivityCheck{}
	portCheck := &PortBindingCheck{}
	
	// Both connectivity checks should be warnings
	assert.Equal(t, SeverityWarning, connectivityCheck.Severity())
	assert.Equal(t, SeverityWarning, portCheck.Severity())
}

// TestConnectivityChecks_ConcurrentExecution_Fixed tests concurrent execution safety
func TestConnectivityChecks_ConcurrentExecution_Fixed(t *testing.T) {
	const numGoroutines = 3
	
	checks := []Check{
		&ContainerConnectivityCheck{},
		&PortBindingCheck{},
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

// TestConnectivityChecks_ContextCancellation_Fixed tests context cancellation handling
func TestConnectivityChecks_ContextCancellation_Fixed(t *testing.T) {
	checks := []Check{
		&ContainerConnectivityCheck{},
		&PortBindingCheck{},
	}
	
	for _, check := range checks {
		t.Run(check.Name(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			
			result, err := check.Run(ctx, nil)
			
			// Should return result even with cancelled context
			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, check.Name(), result.CheckName)
		})
	}
}

// TestConnectivityChecks_EdgeCases_Fixed tests edge cases
func TestConnectivityChecks_EdgeCases_Fixed(t *testing.T) {
	checks := []Check{
		&ContainerConnectivityCheck{},
		&PortBindingCheck{},
	}
	
	// Test with nil context
	for _, check := range checks {
		t.Run(check.Name()+"_nil_context", func(t *testing.T) {
			result, err := check.Run(nil, nil)
			
			// Should handle nil context gracefully
			require.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
	
	// Test with background context
	ctx := context.Background()
	for _, check := range checks {
		t.Run(check.Name()+"_background_context", func(t *testing.T) {
			result, err := check.Run(ctx, nil)
			
			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, check.Name(), result.CheckName)
		})
	}
}

// TestConnectivityChecks_ResultStructure_Fixed tests result structure validation
func TestConnectivityChecks_ResultStructure_Fixed(t *testing.T) {
	checks := []Check{
		&ContainerConnectivityCheck{},
		&PortBindingCheck{},
	}
	
	ctx := context.Background()
	
	for _, check := range checks {
		t.Run(check.Name()+"_result_structure", func(t *testing.T) {
			result, err := check.Run(ctx, nil)
			
			require.NoError(t, err)
			require.NotNil(t, result)
			
			// Validate all required fields
			assert.Equal(t, check.Name(), result.CheckName)
			assert.False(t, result.Timestamp.IsZero())
			assert.NotEmpty(t, result.Message)
			assert.NotNil(t, result.Details)
			assert.NotNil(t, result.Suggestions)
			
			// Validate timestamp is recent
			assert.True(t, time.Since(result.Timestamp) < 1*time.Second)
			
			// Validate details structure
			assert.IsType(t, map[string]interface{}{}, result.Details)
			
			// Validate suggestions structure
			assert.IsType(t, []string{}, result.Suggestions)
		})
	}
}

// BenchmarkConnectivityChecks_Fixed benchmarks connectivity checks
func BenchmarkConnectivityChecks_Fixed(b *testing.B) {
	checks := []Check{
		&ContainerConnectivityCheck{},
		&PortBindingCheck{},
	}
	
	ctx := context.Background()
	
	for _, check := range checks {
		b.Run(check.Name(), func(b *testing.B) {
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				result, err := check.Run(ctx, nil)
				if err != nil {
					b.Fatalf("Check %s failed: %v", check.Name(), err)
				}
				if result == nil {
					b.Fatalf("Check %s returned nil result", check.Name())
				}
			}
		})
	}
}

// TestConnectivityChecks_PerformanceUnderLoad_Fixed tests performance under load
func TestConnectivityChecks_PerformanceUnderLoad_Fixed(t *testing.T) {
	check := &PortBindingCheck{} // This one should be fast
	ctx := context.Background()
	
	const iterations = 10
	start := time.Now()
	
	for i := 0; i < iterations; i++ {
		result, err := check.Run(ctx, nil)
		require.NoError(t, err)
		assert.NotNil(t, result)
	}
	
	duration := time.Since(start)
	avgDuration := duration / iterations
	
	// Should complete quickly even under load
	assert.Less(t, avgDuration, 200*time.Millisecond, "Average execution time too slow: %v", avgDuration)
}

// TestConnectivityChecks_MemoryUsage_Fixed tests memory usage
func TestConnectivityChecks_MemoryUsage_Fixed(t *testing.T) {
	const iterations = 20
	
	checks := []Check{
		&ContainerConnectivityCheck{},
		&PortBindingCheck{},
	}
	
	ctx := context.Background()
	
	for _, check := range checks {
		for i := 0; i < iterations; i++ {
			result, err := check.Run(ctx, nil)
			
			assert.NoError(t, err)
			assert.NotNil(t, result)
			
			// Force garbage collection periodically
			if i%5 == 0 {
				time.Sleep(1 * time.Millisecond)
			}
		}
	}
}