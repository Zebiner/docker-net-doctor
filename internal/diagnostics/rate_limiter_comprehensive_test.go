package diagnostics

import (
	"context"
	"math"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestRateLimiterComprehensiveEdgeCases tests all edge cases for maximum coverage
func TestRateLimiterComprehensiveEdgeCases(t *testing.T) {
	t.Run("nil_config_handling", func(t *testing.T) {
		// Test with nil config - should use defaults
		rl := NewRateLimiter(nil)
		if rl == nil {
			t.Fatal("NewRateLimiter(nil) should return valid rate limiter")
		}
		
		// Should use default values
		if !rl.IsEnabled() {
			t.Error("Default config should be enabled")
		}
		
		expectedRate := float64(DOCKER_API_RATE_LIMIT)
		if rl.GetEffectiveRate() != expectedRate {
			t.Errorf("Default rate = %v, want %v", rl.GetEffectiveRate(), expectedRate)
		}
		
		expectedBurst := DOCKER_API_BURST
		if rl.GetBurstSize() != expectedBurst {
			t.Errorf("Default burst = %v, want %v", rl.GetBurstSize(), expectedBurst)
		}
	})

	t.Run("extreme_rate_values", func(t *testing.T) {
		extremeConfigs := []struct {
			name    string
			rate    float64
			burst   int
			enabled bool
		}{
			{"zero_rate", 0.0, 1, true},
			{"negative_rate", -5.0, 5, true},
			{"very_high_rate", 1e6, 1000, true},
			{"fractional_rate", 0.1, 1, true},
			{"infinity_rate", math.Inf(1), 10, true},
			{"nan_rate", math.NaN(), 5, true},
			{"zero_burst", 1.0, 0, true},
			{"negative_burst", 5.0, -10, true},
			{"very_large_burst", 10.0, 1000000, true},
		}

		for _, tc := range extremeConfigs {
			t.Run(tc.name, func(t *testing.T) {
				config := &RateLimiterConfig{
					RequestsPerSecond: tc.rate,
					BurstSize:         tc.burst,
					WaitTimeout:       time.Second,
					Enabled:           tc.enabled,
				}

				// Should not panic with extreme values
				rl := NewRateLimiter(config)
				if rl == nil {
					t.Fatal("NewRateLimiter should handle extreme values gracefully")
				}

				// Should be able to get status without panic
				status := rl.GetStatus()
				if status == "" {
					t.Error("GetStatus should return non-empty string even with extreme values")
				}

				// Should handle basic operations
				rl.TryAcquire() // Should not panic
				metrics := rl.GetMetrics()
				if metrics.TotalRequests == 0 {
					t.Error("TryAcquire should increment total requests")
				}
			})
		}
	})

	t.Run("timeout_value_edge_cases", func(t *testing.T) {
		timeoutCases := []struct {
			name     string
			timeout  time.Duration
			testFunc func(*testing.T, *RateLimiter)
		}{
			{
				name:    "zero_timeout",
				timeout: 0,
				testFunc: func(t *testing.T, rl *RateLimiter) {
					// With zero timeout, should use context timeout instead
					ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
					defer cancel()
					
					// Exhaust burst first
					rl.TryAcquire()
					
					err := rl.Wait(ctx)
					// Should timeout based on context, not rate limiter timeout
					if err == nil {
						t.Error("Expected timeout with zero WaitTimeout")
					}
				},
			},
			{
				name:    "negative_timeout",
				timeout: -time.Second,
				testFunc: func(t *testing.T, rl *RateLimiter) {
					// Should handle negative timeout gracefully
					rl.TryAcquire() // Exhaust burst
					
					ctx := context.Background()
					err := rl.Wait(ctx)
					// Should either succeed immediately or fail quickly
					if err != nil {
						t.Logf("Negative timeout resulted in error: %v", err)
					}
				},
			},
			{
				name:    "very_large_timeout",
				timeout: 24 * time.Hour,
				testFunc: func(t *testing.T, rl *RateLimiter) {
					// Should handle very large timeout
					ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
					defer cancel()
					
					rl.TryAcquire() // Exhaust burst
					
					err := rl.Wait(ctx)
					// Context should timeout first
					if err == nil {
						t.Error("Expected context timeout to occur before large rate limiter timeout")
					}
				},
			},
		}

		for _, tc := range timeoutCases {
			t.Run(tc.name, func(t *testing.T) {
				config := &RateLimiterConfig{
					RequestsPerSecond: 1.0,
					BurstSize:         1,
					WaitTimeout:       tc.timeout,
					Enabled:           true,
				}

				rl := NewRateLimiter(config)
				tc.testFunc(t, rl)
			})
		}
	})
}

// TestRateLimiterInternalStateConsistency tests internal state consistency
func TestRateLimiterInternalStateConsistency(t *testing.T) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 5.0,
		BurstSize:         3,
		WaitTimeout:       time.Second,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)

	t.Run("metrics_consistency_across_operations", func(t *testing.T) {
		rl.ResetMetrics()

		operations := []struct {
			name string
			op   func() bool
		}{
			{"try_acquire_1", func() bool { return rl.TryAcquire() }},
			{"try_acquire_2", func() bool { return rl.TryAcquire() }},
			{"try_acquire_3", func() bool { return rl.TryAcquire() }},
			{"try_acquire_4", func() bool { return rl.TryAcquire() }}, // Should fail
			{"try_acquire_5", func() bool { return rl.TryAcquire() }}, // Should fail
		}

		expectedAllowed := 3  // Burst size
		expectedThrottled := 2 // Failed attempts

		actualAllowed := 0
		actualThrottled := 0

		for _, op := range operations {
			if op.op() {
				actualAllowed++
			} else {
				actualThrottled++
			}
		}

		if actualAllowed != expectedAllowed {
			t.Errorf("Expected %d allowed, got %d", expectedAllowed, actualAllowed)
		}

		if actualThrottled != expectedThrottled {
			t.Errorf("Expected %d throttled, got %d", expectedThrottled, actualThrottled)
		}

		// Verify metrics match
		metrics := rl.GetMetrics()
		if metrics.TotalRequests != int64(len(operations)) {
			t.Errorf("Metrics TotalRequests = %d, want %d", metrics.TotalRequests, len(operations))
		}

		if metrics.AllowedRequests != int64(actualAllowed) {
			t.Errorf("Metrics AllowedRequests = %d, want %d", metrics.AllowedRequests, actualAllowed)
		}

		if metrics.ThrottledRequests != int64(actualThrottled) {
			t.Errorf("Metrics ThrottledRequests = %d, want %d", metrics.ThrottledRequests, actualThrottled)
		}
	})

	t.Run("config_update_state_consistency", func(t *testing.T) {
		// Start with one config
		initialConfig := rl.GetConfig()

		// Update configuration
		newConfig := &RateLimiterConfig{
			RequestsPerSecond: initialConfig.RequestsPerSecond * 2,
			BurstSize:         initialConfig.BurstSize * 2,
			WaitTimeout:       initialConfig.WaitTimeout * 2,
			Enabled:           true,
		}

		rl.UpdateConfig(newConfig)

		// Verify all getters reflect new config
		if rl.GetEffectiveRate() != newConfig.RequestsPerSecond {
			t.Errorf("EffectiveRate not updated: got %v, want %v", 
				rl.GetEffectiveRate(), newConfig.RequestsPerSecond)
		}

		if rl.GetBurstSize() != newConfig.BurstSize {
			t.Errorf("BurstSize not updated: got %v, want %v", 
				rl.GetBurstSize(), newConfig.BurstSize)
		}

		if !rl.IsEnabled() {
			t.Error("IsEnabled should be true after update")
		}

		updatedConfig := rl.GetConfig()
		if updatedConfig.RequestsPerSecond != newConfig.RequestsPerSecond {
			t.Error("GetConfig does not reflect updates")
		}

		// Disable and verify
		disabledConfig := &RateLimiterConfig{
			RequestsPerSecond: 10.0,
			BurstSize:         10,
			WaitTimeout:       time.Second,
			Enabled:           false,
		}

		rl.UpdateConfig(disabledConfig)

		if rl.IsEnabled() {
			t.Error("IsEnabled should be false after disabling")
		}

		if rl.GetEffectiveRate() != 0 {
			t.Errorf("EffectiveRate should be 0 when disabled, got %v", rl.GetEffectiveRate())
		}

		if rl.GetBurstSize() != 0 {
			t.Errorf("BurstSize should be 0 when disabled, got %v", rl.GetBurstSize())
		}
	})

	t.Run("metrics_timing_precision", func(t *testing.T) {
		config := &RateLimiterConfig{
			RequestsPerSecond: 4.0, // 250ms per token
			BurstSize:         1,
			WaitTimeout:       time.Second,
			Enabled:           true,
		}

		rl := NewRateLimiter(config)
		rl.ResetMetrics()
		ctx := context.Background()

		// Consume burst
		rl.TryAcquire()

		// Measure wait times
		const numWaits = 3
		measuredTimes := make([]time.Duration, numWaits)
		
		for i := 0; i < numWaits; i++ {
			start := time.Now()
			err := rl.Wait(ctx)
			if err != nil {
				t.Fatalf("Wait %d failed: %v", i+1, err)
			}
			measuredTimes[i] = time.Since(start)
		}

		// Verify timing metrics
		metrics := rl.GetMetrics()
		
		if metrics.MaxWaitTime == 0 {
			t.Error("MaxWaitTime should be greater than 0")
		}

		if metrics.TotalWaitTime == 0 {
			t.Error("TotalWaitTime should be greater than 0")
		}

		avgWaitFromMetrics := rl.GetAverageWaitTime()
		
		// Calculate our own average
		var totalMeasured time.Duration
		maxMeasured := time.Duration(0)
		for _, d := range measuredTimes {
			totalMeasured += d
			if d > maxMeasured {
				maxMeasured = d
			}
		}
		avgMeasured := totalMeasured / numWaits

		// Verify timing accuracy (allow some tolerance)
		tolerance := 50 * time.Millisecond

		avgDiff := avgWaitFromMetrics - avgMeasured
		if avgDiff < 0 {
			avgDiff = -avgDiff
		}
		if avgDiff > tolerance {
			t.Errorf("Average wait time differs: metrics=%v measured=%v diff=%v (tolerance=%v)",
				avgWaitFromMetrics, avgMeasured, avgDiff, tolerance)
		}

		// Max wait time should be reasonable
		maxDiff := metrics.MaxWaitTime - maxMeasured
		if maxDiff < 0 {
			maxDiff = -maxDiff
		}
		if maxDiff > tolerance {
			t.Errorf("Max wait time differs: metrics=%v measured=%v diff=%v (tolerance=%v)",
				metrics.MaxWaitTime, maxMeasured, maxDiff, tolerance)
		}
	})
}

// TestRateLimiterReservationFunctionality tests the Reserve functionality thoroughly
func TestRateLimiterReservationFunctionality(t *testing.T) {
	t.Run("reservation_when_enabled", func(t *testing.T) {
		config := &RateLimiterConfig{
			RequestsPerSecond: 2.0,
			BurstSize:         1,
			WaitTimeout:       time.Second,
			Enabled:           true,
		}

		rl := NewRateLimiter(config)
		
		// Should return valid reservation when enabled
		reservation := rl.Reserve()
		if reservation == nil {
			t.Error("Reserve() should return non-nil reservation when enabled")
		}

		// Test reservation methods
		if reservation != nil && reservation.OK() && reservation.DelayFrom(time.Now()) > 0 {
			// If reservation requires waiting, test that it works
			delay := reservation.DelayFrom(time.Now())
			t.Logf("Reservation delay: %v", delay)
			
			// Cancel the reservation to avoid affecting state
			reservation.Cancel()
		}
	})

	t.Run("reservation_when_disabled", func(t *testing.T) {
		config := &RateLimiterConfig{
			RequestsPerSecond: 2.0,
			BurstSize:         1,
			WaitTimeout:       time.Second,
			Enabled:           false,
		}

		rl := NewRateLimiter(config)
		
		// Should return nil when disabled
		reservation := rl.Reserve()
		if reservation != nil {
			t.Error("Reserve() should return nil when disabled")
		}
	})

	t.Run("reservation_after_config_update", func(t *testing.T) {
		config := &RateLimiterConfig{
			RequestsPerSecond: 2.0,
			BurstSize:         1,
			WaitTimeout:       time.Second,
			Enabled:           true,
		}

		rl := NewRateLimiter(config)
		
		// Should work when enabled
		reservation1 := rl.Reserve()
		if reservation1 == nil {
			t.Error("Reserve() should work when enabled")
		}

		// Update to disabled
		disabledConfig := &RateLimiterConfig{
			RequestsPerSecond: 2.0,
			BurstSize:         1,
			WaitTimeout:       time.Second,
			Enabled:           false,
		}
		rl.UpdateConfig(disabledConfig)

		// Should return nil after disabling
		reservation2 := rl.Reserve()
		if reservation2 != nil {
			t.Error("Reserve() should return nil after disabling")
		}

		// Re-enable
		rl.UpdateConfig(config)

		// Should work again
		reservation3 := rl.Reserve()
		if reservation3 == nil {
			t.Error("Reserve() should work after re-enabling")
		}
	})
}

// TestRateLimiterMemoryAndPerformanceEfficiency tests for memory leaks and performance
func TestRateLimiterMemoryAndPerformanceEfficiency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	t.Run("memory_efficiency_many_instances", func(t *testing.T) {
		const numInstances = 1000
		
		// Create many rate limiters
		limiters := make([]*RateLimiter, numInstances)
		
		var memBefore runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&memBefore)

		for i := 0; i < numInstances; i++ {
			config := &RateLimiterConfig{
				RequestsPerSecond: float64(i%10 + 1),
				BurstSize:         i%5 + 1,
				WaitTimeout:       time.Second,
				Enabled:           i%2 == 0,
			}
			limiters[i] = NewRateLimiter(config)
		}

		// Use them briefly to ensure they're not optimized away
		for i := 0; i < numInstances; i++ {
			limiters[i].TryAcquire()
			_ = limiters[i].GetMetrics()
		}

		var memAfter runtime.MemStats
		runtime.ReadMemStats(&memAfter)

		memoryUsedMB := float64(memAfter.Alloc-memBefore.Alloc) / (1024 * 1024)
		memoryPerInstance := memoryUsedMB / numInstances * 1024 // KB per instance

		t.Logf("Created %d rate limiters, memory usage: %.2f MB total, %.2f KB per instance",
			numInstances, memoryUsedMB, memoryPerInstance)

		// Each rate limiter should use reasonable amount of memory
		maxMemoryPerInstanceKB := 10.0 // 10KB seems reasonable
		if memoryPerInstance > maxMemoryPerInstanceKB {
			t.Errorf("Memory per instance %.2f KB > maximum allowed %.2f KB",
				memoryPerInstance, maxMemoryPerInstanceKB)
		}
	})

	t.Run("performance_under_high_frequency", func(t *testing.T) {
		config := &RateLimiterConfig{
			RequestsPerSecond: 10000.0, // Very high rate
			BurstSize:         1000,    // Large burst
			WaitTimeout:       time.Second,
			Enabled:           true,
		}

		rl := NewRateLimiter(config)

		const numOperations = 100000
		
		start := time.Now()

		// High-frequency TryAcquire operations
		successCount := 0
		for i := 0; i < numOperations; i++ {
			if rl.TryAcquire() {
				successCount++
			}
		}

		elapsed := time.Since(start)
		operationsPerSecond := float64(numOperations) / elapsed.Seconds()

		t.Logf("High frequency test: %d ops in %v (%.0f ops/sec), %d successful",
			numOperations, elapsed, operationsPerSecond, successCount)

		// Should handle at least 100k operations per second
		minOpsPerSecond := 100000.0
		if operationsPerSecond < minOpsPerSecond {
			t.Errorf("Performance too low: %.0f ops/sec < required %.0f ops/sec",
				operationsPerSecond, minOpsPerSecond)
		}

		// Verify metrics are still consistent
		metrics := rl.GetMetrics()
		if metrics.TotalRequests != int64(numOperations) {
			t.Errorf("Metrics TotalRequests %d != expected %d", 
				metrics.TotalRequests, numOperations)
		}
	})

	t.Run("performance_concurrent_mixed_operations", func(t *testing.T) {
		config := &RateLimiterConfig{
			RequestsPerSecond: 1000.0,
			BurstSize:         100,
			WaitTimeout:       100 * time.Millisecond,
			Enabled:           true,
		}

		rl := NewRateLimiter(config)
		ctx := context.Background()

		const numGoroutines = 20
		const opsPerGoroutine = 1000
		const totalOps = numGoroutines * opsPerGoroutine

		var wg sync.WaitGroup
		start := time.Now()

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				for j := 0; j < opsPerGoroutine; j++ {
					switch j % 4 {
					case 0:
						rl.TryAcquire()
					case 1:
						shortCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
						rl.Wait(shortCtx)
						cancel()
					case 2:
						_ = rl.GetMetrics()
					case 3:
						_ = rl.GetAverageWaitTime()
					}
				}
			}()
		}

		wg.Wait()
		elapsed := time.Since(start)

		totalOpsPerSecond := float64(totalOps) / elapsed.Seconds()

		t.Logf("Concurrent mixed operations: %d total ops in %v (%.0f ops/sec)",
			totalOps, elapsed, totalOpsPerSecond)

		// Should handle reasonable concurrent load
		minConcurrentOpsPerSecond := 50000.0
		if totalOpsPerSecond < minConcurrentOpsPerSecond {
			t.Errorf("Concurrent performance too low: %.0f ops/sec < required %.0f ops/sec",
				totalOpsPerSecond, minConcurrentOpsPerSecond)
		}

		// Verify no data races or corruption
		metrics := rl.GetMetrics()
		if metrics.TotalRequests == 0 {
			t.Error("No requests recorded - possible concurrency issue")
		}

		// Should have recorded approximately the right number of requests
		// (not exact due to mixed operations, but should be in ballpark)
		expectedRequests := int64(numGoroutines * opsPerGoroutine / 2) // Rough estimate
		if metrics.TotalRequests < expectedRequests/2 || metrics.TotalRequests > expectedRequests*2 {
			t.Errorf("Request count seems wrong: got %d, expected around %d",
				metrics.TotalRequests, expectedRequests)
		}
	})
}

// TestRateLimiterAdvancedErrorHandling tests various error conditions
func TestRateLimiterAdvancedErrorHandling(t *testing.T) {
	t.Run("context_cancellation_during_wait", func(t *testing.T) {
		config := &RateLimiterConfig{
			RequestsPerSecond: 1.0, // Slow rate to force waiting
			BurstSize:         1,
			WaitTimeout:       5 * time.Second, // Long timeout
			Enabled:           true,
		}

		rl := NewRateLimiter(config)
		
		// Exhaust burst
		rl.TryAcquire()

		// Create context that will be cancelled
		ctx, cancel := context.WithCancel(context.Background())

		// Start wait in goroutine
		errCh := make(chan error, 1)
		go func() {
			errCh <- rl.Wait(ctx)
		}()

		// Cancel after short delay
		time.Sleep(50 * time.Millisecond)
		cancel()

		// Should get cancellation error
		select {
		case err := <-errCh:
			if err == nil {
				t.Error("Expected error when context cancelled")
			}
			// Error should be context-related
			if err.Error() == "" {
				t.Error("Error should have meaningful message")
			}
		case <-time.After(time.Second):
			t.Fatal("Wait did not respond to context cancellation")
		}
	})

	t.Run("timeout_during_wait", func(t *testing.T) {
		config := &RateLimiterConfig{
			RequestsPerSecond: 1.0,
			BurstSize:         1,
			WaitTimeout:       100 * time.Millisecond, // Short timeout
			Enabled:           true,
		}

		rl := NewRateLimiter(config)
		
		// Exhaust burst
		rl.TryAcquire()

		// Wait should timeout
		ctx := context.Background()
		err := rl.Wait(ctx)

		if err == nil {
			t.Error("Expected timeout error")
		}

		// Should be timeout-specific error
		if !rateLimiterAdvancedContains(err.Error(), "timeout") {
			t.Errorf("Error should mention timeout: %v", err)
		}

		// Verify metrics recorded the timeout
		metrics := rl.GetMetrics()
		if metrics.TimeoutRequests == 0 {
			t.Error("Timeout should be recorded in metrics")
		}
	})

	t.Run("concurrent_config_updates_during_operations", func(t *testing.T) {
		config := &RateLimiterConfig{
			RequestsPerSecond: 5.0,
			BurstSize:         3,
			WaitTimeout:       time.Second,
			Enabled:           true,
		}

		rl := NewRateLimiter(config)
		ctx := context.Background()

		var wg sync.WaitGroup
		stopCh := make(chan struct{})

		// Worker performing operations
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			for {
				select {
				case <-stopCh:
					return
				default:
					rl.TryAcquire()
					shortCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
					rl.Wait(shortCtx)
					cancel()
					_ = rl.GetMetrics()
				}
			}
		}()

		// Worker updating configuration
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			configs := []*RateLimiterConfig{
				{RequestsPerSecond: 10.0, BurstSize: 5, WaitTimeout: time.Second, Enabled: true},
				{RequestsPerSecond: 2.0, BurstSize: 1, WaitTimeout: time.Second, Enabled: true},
				{RequestsPerSecond: 5.0, BurstSize: 3, WaitTimeout: time.Second, Enabled: false},
			}

			for i := 0; i < 100; i++ {
				config := configs[i%len(configs)]
				rl.UpdateConfig(config)
				time.Sleep(time.Millisecond)
			}
		}()

		// Let them run for a bit
		time.Sleep(500 * time.Millisecond)
		close(stopCh)
		wg.Wait()

		// Should not have crashed or corrupted state
		metrics := rl.GetMetrics()
		if metrics.TotalRequests == 0 {
			t.Error("Should have recorded some operations")
		}

		status := rl.GetStatus()
		if status == "" {
			t.Error("Should be able to get status after concurrent operations")
		}
	})
}

// Helper function to test if string contains substring
func rateLimiterAdvancedContains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}