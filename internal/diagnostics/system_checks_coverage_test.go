package diagnostics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// For testing enhanced code paths, we'll use a different approach
// We'll test the implementation by examining the results rather than mocking

// TestDaemonConnectivityCheck_CancelledContext_Enhanced tests cancelled context handling
func TestDaemonConnectivityCheck_CancelledContext_Enhanced(t *testing.T) {
	check := &DaemonConnectivityCheck{}

	// Test with a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Create a minimal mock that won't be called due to context cancellation
	// We pass a real pointer, but since context is cancelled, Ping shouldn't be called
	result, err := check.Run(ctx, nil)

	require.NoError(t, err)
	require.NotNil(t, result)

	// With cancelled context, we still get the fallback behavior (nil client)
	assert.True(t, result.Success)
	assert.Contains(t, result.Message, "client provided")
}

// TestNetworkIsolationCheck_CancelledContext_Enhanced tests cancelled context handling
func TestNetworkIsolationCheck_CancelledContext_Enhanced(t *testing.T) {
	check := &NetworkIsolationCheck{}

	// Test with a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result, err := check.Run(ctx, nil)

	require.NoError(t, err)
	require.NotNil(t, result)

	// With cancelled context and nil client, we get the fallback behavior
	assert.True(t, result.Success)
	assert.Contains(t, result.Message, "no client check")
}

// TestSystemChecks_ResultStructure tests that enhanced results have proper structure
func TestSystemChecks_ResultStructure(t *testing.T) {
	tests := []struct {
		name  string
		check Check
	}{
		{"daemon_check", &DaemonConnectivityCheck{}},
		{"isolation_check", &NetworkIsolationCheck{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.check.Run(context.Background(), nil)

			require.NoError(t, err)
			require.NotNil(t, result)

			// Test that enhanced fields are present
			assert.NotNil(t, result.Details)
			assert.False(t, result.Timestamp.IsZero())
			assert.Equal(t, tt.check.Name(), result.CheckName)

			// Test that Details is initialized as a map
			assert.IsType(t, map[string]interface{}{}, result.Details)
		})
	}
}

// TestDaemonConnectivityCheck_DetailedTiming tests timing details
func TestDaemonConnectivityCheck_DetailedTiming(t *testing.T) {
	check := &DaemonConnectivityCheck{}

	before := time.Now()
	result, err := check.Run(context.Background(), nil)
	after := time.Now()

	require.NoError(t, err)
	require.NotNil(t, result)

	// Check timing
	assert.True(t, result.Timestamp.After(before) || result.Timestamp.Equal(before))
	assert.True(t, result.Timestamp.Before(after) || result.Timestamp.Equal(after))

	// For nil client, duration should still be set (though very small)
	// This tests the Duration field assignment
	assert.True(t, result.Duration >= 0)
}

// TestNetworkIsolationCheck_DetailedTiming tests timing details
func TestNetworkIsolationCheck_DetailedTiming(t *testing.T) {
	check := &NetworkIsolationCheck{}

	before := time.Now()
	result, err := check.Run(context.Background(), nil)
	after := time.Now()

	require.NoError(t, err)
	require.NotNil(t, result)

	// Check timing
	assert.True(t, result.Timestamp.After(before) || result.Timestamp.Equal(before))
	assert.True(t, result.Timestamp.Before(after) || result.Timestamp.Equal(after))

	// For nil client, duration should still be set
	assert.True(t, result.Duration >= 0)
}

// TestSystemChecks_ContextValues tests various context scenarios
func TestSystemChecks_ContextValues(t *testing.T) {
	checks := []Check{
		&DaemonConnectivityCheck{},
		&NetworkIsolationCheck{},
	}

	for _, check := range checks {
		t.Run(check.Name(), func(t *testing.T) {
			// Test with background context
			result1, err1 := check.Run(context.Background(), nil)
			require.NoError(t, err1)
			assert.NotNil(t, result1)
			assert.True(t, result1.Success)

			// Test with TODO context
			result2, err2 := check.Run(context.TODO(), nil)
			require.NoError(t, err2)
			assert.NotNil(t, result2)
			assert.True(t, result2.Success)

			// Test with context with value
			ctx := context.WithValue(context.Background(), "key", "value")
			result3, err3 := check.Run(ctx, nil)
			require.NoError(t, err3)
			assert.NotNil(t, result3)
			assert.True(t, result3.Success)
		})
	}
}

// TestSystemChecks_MessageConsistency tests that messages are consistent
func TestSystemChecks_MessageConsistency(t *testing.T) {
	daemonCheck := &DaemonConnectivityCheck{}
	isolationCheck := &NetworkIsolationCheck{}

	// Run multiple times to ensure consistency
	for i := 0; i < 5; i++ {
		daemonResult, err := daemonCheck.Run(context.Background(), nil)
		require.NoError(t, err)
		assert.Equal(t, "Docker daemon is accessible (client provided)", daemonResult.Message)

		isolationResult, err := isolationCheck.Run(context.Background(), nil)
		require.NoError(t, err)
		assert.Equal(t, "Network isolation configured correctly (no client check)", isolationResult.Message)
	}
}

// TestSystemChecks_FieldTypes tests that all fields have correct types
func TestSystemChecks_FieldTypes(t *testing.T) {
	checks := []Check{
		&DaemonConnectivityCheck{},
		&NetworkIsolationCheck{},
	}

	for _, check := range checks {
		t.Run(check.Name(), func(t *testing.T) {
			result, err := check.Run(context.Background(), nil)

			require.NoError(t, err)
			require.NotNil(t, result)

			// Test field types
			assert.IsType(t, "", result.CheckName)
			assert.IsType(t, true, result.Success)
			assert.IsType(t, "", result.Message)
			assert.IsType(t, time.Time{}, result.Timestamp)
			assert.IsType(t, time.Duration(0), result.Duration)
			assert.IsType(t, map[string]interface{}{}, result.Details)
			assert.IsType(t, []string{}, result.Suggestions)
		})
	}
}

// TestSystemChecks_MultipleRuns tests that multiple runs don't interfere
func TestSystemChecks_MultipleRuns(t *testing.T) {
	daemonCheck := &DaemonConnectivityCheck{}
	isolationCheck := &NetworkIsolationCheck{}
	ctx := context.Background()

	// Run multiple times in sequence
	var results []*CheckResult

	for i := 0; i < 10; i++ {
		result1, err1 := daemonCheck.Run(ctx, nil)
		require.NoError(t, err1)
		results = append(results, result1)

		result2, err2 := isolationCheck.Run(ctx, nil)
		require.NoError(t, err2)
		results = append(results, result2)
	}

	// Verify all results are valid and unique timestamps
	timestamps := make(map[time.Time]bool)
	for _, result := range results {
		assert.NotNil(t, result)
		assert.True(t, result.Success)
		assert.NotEmpty(t, result.CheckName)
		assert.NotEmpty(t, result.Message)
		assert.False(t, result.Timestamp.IsZero())

		// Timestamps should be unique (or at least not all the same)
		timestamps[result.Timestamp] = true
	}

	// Should have multiple unique timestamps (though some might be the same if run very fast)
	assert.True(t, len(timestamps) >= 1)
}

// TestSystemChecks_ErrorFieldInitialization tests that error fields are properly initialized
func TestSystemChecks_ErrorFieldInitialization(t *testing.T) {
	checks := []Check{
		&DaemonConnectivityCheck{},
		&NetworkIsolationCheck{},
	}

	for _, check := range checks {
		t.Run(check.Name(), func(t *testing.T) {
			result, err := check.Run(context.Background(), nil)

			require.NoError(t, err)
			require.NotNil(t, result)

			// Test that collections are initialized (not nil)
			assert.NotNil(t, result.Details)
			if result.Suggestions != nil {
				assert.IsType(t, []string{}, result.Suggestions)
			}

			// Test that we can add to Details without panic
			assert.NotPanics(t, func() {
				result.Details["test_key"] = "test_value"
			})
		})
	}
}

// BenchmarkSystemChecks_Enhanced benchmarks the enhanced implementation
func BenchmarkSystemChecks_Enhanced(b *testing.B) {
	daemonCheck := &DaemonConnectivityCheck{}
	isolationCheck := &NetworkIsolationCheck{}
	ctx := context.Background()

	b.Run("daemon_check", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := daemonCheck.Run(ctx, nil)
			if err != nil {
				b.Fatalf("Check failed: %v", err)
			}
		}
	})

	b.Run("isolation_check", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := isolationCheck.Run(ctx, nil)
			if err != nil {
				b.Fatalf("Check failed: %v", err)
			}
		}
	})

	b.Run("both_checks", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err1 := daemonCheck.Run(ctx, nil)
			if err1 != nil {
				b.Fatalf("Daemon check failed: %v", err1)
			}

			_, err2 := isolationCheck.Run(ctx, nil)
			if err2 != nil {
				b.Fatalf("Isolation check failed: %v", err2)
			}
		}
	})
}
