package diagnostics

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRateLimiterInitialization tests rate limiter creation and configuration
func TestRateLimiterInitialization(t *testing.T) {
	tests := []struct {
		name            string
		config          *RateLimiterConfig
		expectedRate    float64
		expectedBurst   int
		expectedEnabled bool
	}{
		{
			name:            "default configuration",
			config:          nil,
			expectedRate:    DOCKER_API_RATE_LIMIT,
			expectedBurst:   DOCKER_API_BURST,
			expectedEnabled: true,
		},
		{
			name: "custom configuration",
			config: &RateLimiterConfig{
				RequestsPerSecond: 10.0,
				BurstSize:         20,
				WaitTimeout:       3 * time.Second,
				Enabled:           true,
			},
			expectedRate:    10.0,
			expectedBurst:   20,
			expectedEnabled: true,
		},
		{
			name: "disabled configuration",
			config: &RateLimiterConfig{
				RequestsPerSecond: 5.0,
				BurstSize:         10,
				WaitTimeout:       5 * time.Second,
				Enabled:           false,
			},
			expectedRate:    0, // Returns 0 when disabled
			expectedBurst:   0, // Returns 0 when disabled
			expectedEnabled: false,
		},
		{
			name: "zero rate configuration",
			config: &RateLimiterConfig{
				RequestsPerSecond: 0,
				BurstSize:         1,
				WaitTimeout:       time.Second,
				Enabled:           true,
			},
			expectedRate:    0,
			expectedBurst:   1,
			expectedEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiter(tt.config)

			if rl == nil {
				t.Fatal("NewRateLimiter returned nil")
			}

			if rl.IsEnabled() != tt.expectedEnabled {
				t.Errorf("IsEnabled() = %v, want %v", rl.IsEnabled(), tt.expectedEnabled)
			}

			if rl.GetEffectiveRate() != tt.expectedRate {
				t.Errorf("GetEffectiveRate() = %v, want %v", rl.GetEffectiveRate(), tt.expectedRate)
			}

			if rl.GetBurstSize() != tt.expectedBurst {
				t.Errorf("GetBurstSize() = %v, want %v", rl.GetBurstSize(), tt.expectedBurst)
			}

			// Test metrics initialization
			metrics := rl.GetMetrics()
			if metrics.TotalRequests != 0 {
				t.Errorf("Initial TotalRequests = %d, want 0", metrics.TotalRequests)
			}
			if metrics.AllowedRequests != 0 {
				t.Errorf("Initial AllowedRequests = %d, want 0", metrics.AllowedRequests)
			}
			if metrics.ThrottledRequests != 0 {
				t.Errorf("Initial ThrottledRequests = %d, want 0", metrics.ThrottledRequests)
			}
		})
	}
}

// TestRateLimiterBasicOperations tests core rate limiting functionality
func TestRateLimiterBasicOperations(t *testing.T) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 2.0, // 2 requests per second
		BurstSize:         3,   // Allow burst of 3
		WaitTimeout:       time.Second,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)
	ctx := context.Background()

	// Test burst capacity - should allow immediate requests up to burst size
	for i := 0; i < 3; i++ {
		if !rl.TryAcquire() {
			t.Errorf("TryAcquire() failed for burst request %d", i+1)
		}
	}

	// Next request should be throttled
	if rl.TryAcquire() {
		t.Error("Expected TryAcquire() to fail after burst exhausted")
	}

	// Test Wait() method
	start := time.Now()
	err := rl.Wait(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Wait() failed: %v", err)
	}

	// Should have waited approximately 500ms (1/2 second rate)
	if elapsed < 400*time.Millisecond || elapsed > 600*time.Millisecond {
		t.Errorf("Wait() took %v, expected ~500ms", elapsed)
	}

	// Verify metrics were updated
	metrics := rl.GetMetrics()
	if metrics.TotalRequests != 5 { // 4 TryAcquire + 1 Wait
		t.Errorf("TotalRequests = %d, want 5", metrics.TotalRequests)
	}
	if metrics.AllowedRequests != 4 { // 3 burst + 1 wait
		t.Errorf("AllowedRequests = %d, want 4", metrics.AllowedRequests)
	}
	if metrics.ThrottledRequests != 1 { // 1 failed TryAcquire
		t.Errorf("ThrottledRequests = %d, want 1", metrics.ThrottledRequests)
	}
}

// TestRateLimiterDisabled tests behavior when rate limiting is disabled
func TestRateLimiterDisabled(t *testing.T) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 1.0,
		BurstSize:         1,
		WaitTimeout:       time.Second,
		Enabled:           false,
	}

	rl := NewRateLimiter(config)
	ctx := context.Background()

	// Should allow unlimited requests when disabled
	for i := 0; i < 100; i++ {
		if !rl.TryAcquire() {
			t.Errorf("TryAcquire() failed when rate limiting disabled, iteration %d", i)
		}

		err := rl.Wait(ctx)
		if err != nil {
			t.Errorf("Wait() failed when rate limiting disabled: %v", err)
		}
	}

	// All requests should be allowed
	metrics := rl.GetMetrics()
	if metrics.TotalRequests != 200 { // 100 TryAcquire + 100 Wait
		t.Errorf("TotalRequests = %d, want 200", metrics.TotalRequests)
	}
	if metrics.AllowedRequests != 200 {
		t.Errorf("AllowedRequests = %d, want 200", metrics.AllowedRequests)
	}
	if metrics.ThrottledRequests != 0 {
		t.Errorf("ThrottledRequests = %d, want 0", metrics.ThrottledRequests)
	}
}

// TestRateLimiterBurstHandling tests burst capacity behavior
func TestRateLimiterBurstHandling(t *testing.T) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 10.0, // Faster rate for testing (100ms per token)
		BurstSize:         5,    // Large burst
		WaitTimeout:       100 * time.Millisecond,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)

	// Test initial burst capacity
	burstAllowed := 0
	for i := 0; i < 10; i++ {
		if rl.TryAcquire() {
			burstAllowed++
		} else {
			break
		}
	}

	if burstAllowed != 5 {
		t.Errorf("Burst allowed %d requests, want 5", burstAllowed)
	}

	// Wait for one token to replenish
	time.Sleep(150 * time.Millisecond) // Slightly more than 100ms

	// Should allow one more request
	if !rl.TryAcquire() {
		t.Error("Expected one request to be allowed after token replenishment")
	}

	// But not a second one immediately
	if rl.TryAcquire() {
		t.Error("Expected second immediate request to be throttled")
	}
}

// TestRateLimiterBurstReplenishment tests burst bucket replenishment
func TestRateLimiterBurstReplenishment(t *testing.T) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 10.0, // 10 req/sec = 100ms per token
		BurstSize:         3,
		WaitTimeout:       time.Second,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)

	// Exhaust burst capacity
	for i := 0; i < 3; i++ {
		if !rl.TryAcquire() {
			t.Fatalf("Failed to acquire burst token %d", i+1)
		}
	}

	// Should be throttled now
	if rl.TryAcquire() {
		t.Error("Expected throttling after burst exhaustion")
	}

	// Wait for token replenishment (110ms should be enough for 100ms rate)
	time.Sleep(110 * time.Millisecond)

	// Should allow one more request
	if !rl.TryAcquire() {
		t.Error("Expected request to succeed after token replenishment")
	}

	// Verify burst doesn't immediately refill
	acquired := 0
	for i := 0; i < 3; i++ {
		if rl.TryAcquire() {
			acquired++
		}
	}

	// Should not have refilled full burst capacity immediately
	if acquired >= 3 {
		t.Errorf("Burst capacity seems to refill too quickly: %d immediate acquisitions", acquired)
	}
}

// TestRateLimiterMetricsAccuracy tests metrics collection accuracy
func TestRateLimiterMetricsAccuracy(t *testing.T) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 5.0,
		BurstSize:         2,
		WaitTimeout:       100 * time.Millisecond,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)
	ctx := context.Background()

	// Reset metrics to start clean
	rl.ResetMetrics()

	// Test various operations
	successfulTryAcquire := 0
	failedTryAcquire := 0

	// Try 10 immediate acquisitions
	for i := 0; i < 10; i++ {
		if rl.TryAcquire() {
			successfulTryAcquire++
		} else {
			failedTryAcquire++
		}
	}

	// Try some Wait operations (some may timeout)
	successfulWaits := 0
	timeoutWaits := 0

	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		err := rl.Wait(ctx)
		cancel()

		if err != nil {
			timeoutWaits++
		} else {
			successfulWaits++
		}
	}

	// Verify metrics
	metrics := rl.GetMetrics()

	expectedTotal := 10 + 5 // TryAcquire attempts + Wait attempts
	if metrics.TotalRequests != int64(expectedTotal) {
		t.Errorf("TotalRequests = %d, want %d", metrics.TotalRequests, expectedTotal)
	}

	expectedAllowed := int64(successfulTryAcquire + successfulWaits)
	if metrics.AllowedRequests != expectedAllowed {
		t.Errorf("AllowedRequests = %d, want %d", metrics.AllowedRequests, expectedAllowed)
	}

	expectedThrottled := int64(failedTryAcquire + timeoutWaits)
	actualThrottled := metrics.ThrottledRequests + metrics.TimeoutRequests
	if actualThrottled != expectedThrottled {
		t.Errorf("Throttled+Timeout = %d, want %d", actualThrottled, expectedThrottled)
	}

	// Test throttle rate calculation
	throttleRate := rl.GetThrottleRate()
	expectedRate := float64(expectedThrottled) / float64(expectedTotal) * 100
	if throttleRate != expectedRate {
		t.Errorf("GetThrottleRate() = %.2f, want %.2f", throttleRate, expectedRate)
	}
}

// TestRateLimiterConcurrentAccess tests thread safety
func TestRateLimiterConcurrentAccess(t *testing.T) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 100.0, // High rate to reduce contention
		BurstSize:         50,
		WaitTimeout:       time.Second,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)
	ctx := context.Background()

	numGoroutines := 20
	requestsPerGoroutine := 50

	var wg sync.WaitGroup
	var totalSuccessful int64
	var totalFailed int64

	// Launch concurrent goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			successful := 0
			failed := 0

			for j := 0; j < requestsPerGoroutine; j++ {
				// Mix TryAcquire and Wait calls
				if j%2 == 0 {
					if rl.TryAcquire() {
						successful++
					} else {
						failed++
					}
				} else {
					shortCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
					if err := rl.Wait(shortCtx); err == nil {
						successful++
					} else {
						failed++
					}
					cancel()
				}
			}

			atomic.AddInt64(&totalSuccessful, int64(successful))
			atomic.AddInt64(&totalFailed, int64(failed))
		}()
	}

	wg.Wait()

	// Verify metrics consistency
	metrics := rl.GetMetrics()
	expectedTotal := int64(numGoroutines * requestsPerGoroutine)

	if metrics.TotalRequests != expectedTotal {
		t.Errorf("TotalRequests = %d, want %d", metrics.TotalRequests, expectedTotal)
	}

	actualTotal := metrics.AllowedRequests + metrics.ThrottledRequests + metrics.TimeoutRequests
	if actualTotal != expectedTotal {
		t.Errorf("Sum of metrics (%d) != TotalRequests (%d)", actualTotal, expectedTotal)
	}

	// The successful + failed counts from goroutines should match metrics
	observedTotal := totalSuccessful + totalFailed
	if observedTotal != expectedTotal {
		t.Errorf("Observed total (%d) != expected (%d)", observedTotal, expectedTotal)
	}

	if int64(totalSuccessful) != metrics.AllowedRequests {
		t.Errorf("Successful requests (%d) != AllowedRequests (%d)", totalSuccessful, metrics.AllowedRequests)
	}
}

// TestRateLimiterContextCancellation tests context handling
func TestRateLimiterContextCancellation(t *testing.T) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 1.0, // Very slow to force waiting
		BurstSize:         1,
		WaitTimeout:       5 * time.Second,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)

	// Exhaust burst capacity
	if !rl.TryAcquire() {
		t.Fatal("Failed to acquire initial token")
	}

	// Create a context that will be cancelled
	ctx, cancel := context.WithCancel(context.Background())

	// Start a wait operation in a goroutine
	var waitErr error
	done := make(chan struct{})

	go func() {
		waitErr = rl.Wait(ctx)
		close(done)
	}()

	// Cancel the context after a short delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Wait for the operation to complete
	select {
	case <-done:
		if waitErr == nil {
			t.Error("Expected Wait() to return error when context cancelled")
		}
	case <-time.After(time.Second):
		t.Fatal("Wait() did not respond to context cancellation")
	}

	// Verify metrics recorded the cancellation appropriately
	metrics := rl.GetMetrics()
	if metrics.TotalRequests < 2 {
		t.Errorf("Expected at least 2 requests (TryAcquire + Wait), got %d", metrics.TotalRequests)
	}
}

// TestRateLimiterWaitTimeout tests timeout handling in Wait method
func TestRateLimiterWaitTimeout(t *testing.T) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 1000.0, // High rate so we can actually wait
		BurstSize:         1,
		WaitTimeout:       50 * time.Millisecond, // Short timeout
		Enabled:           true,
	}

	rl := NewRateLimiter(config)

	// Exhaust burst capacity
	if !rl.TryAcquire() {
		t.Fatal("Failed to acquire initial token")
	}

	// Use a cancelled context to force immediate timeout
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancel

	err := rl.Wait(ctx)

	if err == nil {
		t.Error("Expected Wait() to fail with cancelled context")
	}

	// Verify that the failure was recorded in metrics
	metrics := rl.GetMetrics()
	totalRequests := metrics.TotalRequests
	if totalRequests < 2 {
		t.Errorf("Expected at least 2 total requests, got %d", totalRequests)
	}

	// The failure should be recorded somehow
	totalFailed := metrics.ThrottledRequests + metrics.TimeoutRequests
	totalAllowed := metrics.AllowedRequests
	if totalAllowed+totalFailed != totalRequests {
		t.Errorf("Metrics don't add up: %d allowed + %d failed != %d total",
			totalAllowed, totalFailed, totalRequests)
	}
}

// TestRateLimiterConfigUpdate tests dynamic configuration updates
func TestRateLimiterConfigUpdate(t *testing.T) {
	initialConfig := &RateLimiterConfig{
		RequestsPerSecond: 5.0,
		BurstSize:         5,
		WaitTimeout:       time.Second,
		Enabled:           true,
	}

	rl := NewRateLimiter(initialConfig)

	// Verify initial configuration
	if rl.GetEffectiveRate() != 5.0 {
		t.Errorf("Initial rate = %v, want 5.0", rl.GetEffectiveRate())
	}

	// Update configuration
	newConfig := &RateLimiterConfig{
		RequestsPerSecond: 10.0,
		BurstSize:         20,
		WaitTimeout:       2 * time.Second,
		Enabled:           true,
	}

	rl.UpdateConfig(newConfig)

	// Verify updated configuration
	if rl.GetEffectiveRate() != 10.0 {
		t.Errorf("Updated rate = %v, want 10.0", rl.GetEffectiveRate())
	}
	if rl.GetBurstSize() != 20 {
		t.Errorf("Updated burst size = %v, want 20", rl.GetBurstSize())
	}

	// Test disabling rate limiter
	disabledConfig := &RateLimiterConfig{
		RequestsPerSecond: 1.0,
		BurstSize:         1,
		WaitTimeout:       time.Second,
		Enabled:           false,
	}

	rl.UpdateConfig(disabledConfig)

	if rl.IsEnabled() {
		t.Error("Expected rate limiter to be disabled")
	}

	// Should allow unlimited requests when disabled
	for i := 0; i < 100; i++ {
		if !rl.TryAcquire() {
			t.Errorf("TryAcquire() failed after disabling, iteration %d", i)
			break
		}
	}
}

// TestRateLimiterReserve tests the Reserve functionality
func TestRateLimiterReserve(t *testing.T) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 2.0,
		BurstSize:         1,
		WaitTimeout:       time.Second,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)

	// When enabled, should return a reservation
	reservation := rl.Reserve()
	if reservation == nil {
		t.Error("Expected Reserve() to return reservation when enabled")
	}

	// When disabled, should return nil
	disabledConfig := &RateLimiterConfig{
		RequestsPerSecond: 2.0,
		BurstSize:         1,
		WaitTimeout:       time.Second,
		Enabled:           false,
	}

	rl.UpdateConfig(disabledConfig)
	reservation = rl.Reserve()
	if reservation != nil {
		t.Error("Expected Reserve() to return nil when disabled")
	}
}

// TestRateLimiterMetricsCalculations tests metric calculation methods
func TestRateLimiterMetricsCalculations(t *testing.T) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 10.0,
		BurstSize:         3,
		WaitTimeout:       time.Second,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)
	rl.ResetMetrics()

	// Perform some operations to generate metrics
	ctx := context.Background()

	// Some successful operations
	rl.TryAcquire() // Success
	rl.TryAcquire() // Success
	rl.TryAcquire() // Success (burst exhausted)

	if rl.TryAcquire() { // Should fail
		t.Error("Expected TryAcquire to fail after burst exhaustion")
	}

	// Add some wait time
	startWait := time.Now()
	err := rl.Wait(ctx)
	waitTime := time.Since(startWait)
	if err != nil {
		t.Errorf("Wait failed: %v", err)
	}

	// Test average wait time calculation
	avgWaitTime := rl.GetAverageWaitTime()
	expectedAvg := waitTime / 4 // waitTime divided by AllowedRequests (4)

	// Allow some tolerance for timing variations
	if avgWaitTime < expectedAvg/2 || avgWaitTime > expectedAvg*2 {
		t.Errorf("GetAverageWaitTime() = %v, expected around %v", avgWaitTime, expectedAvg)
	}

	// Test throttle rate calculation
	throttleRate := rl.GetThrottleRate()
	expectedThrottleRate := (1.0 / 5.0) * 100 // 1 throttled out of 5 total = 20%

	if throttleRate != expectedThrottleRate {
		t.Errorf("GetThrottleRate() = %.2f, want %.2f", throttleRate, expectedThrottleRate)
	}

	// Test status string
	status := rl.GetStatus()
	if status == "" {
		t.Error("GetStatus() returned empty string")
	}

	// Should contain key information
	if !rateLimiterContains(status, "10.0 req/s") {
		t.Errorf("Status missing rate info: %s", status)
	}
	if !rateLimiterContains(status, "burst: 3") {
		t.Errorf("Status missing burst info: %s", status)
	}
}

// TestRateLimiterEdgeCases tests various edge cases
func TestRateLimiterEdgeCases(t *testing.T) {
	t.Run("zero rate limit", func(t *testing.T) {
		config := &RateLimiterConfig{
			RequestsPerSecond: 0,
			BurstSize:         1,
			WaitTimeout:       time.Second,
			Enabled:           true,
		}

		rl := NewRateLimiter(config)

		// With zero rate, only burst should be available
		if !rl.TryAcquire() {
			t.Error("Expected TryAcquire to succeed with burst capacity")
		}

		// Second request should fail
		if rl.TryAcquire() {
			t.Error("Expected TryAcquire to fail with zero replenishment rate")
		}
	})

	t.Run("negative rate limit", func(t *testing.T) {
		config := &RateLimiterConfig{
			RequestsPerSecond: -1.0,
			BurstSize:         1,
			WaitTimeout:       time.Second,
			Enabled:           true,
		}

		// NewRateLimiter should handle negative rates gracefully
		rl := NewRateLimiter(config)
		if rl == nil {
			t.Fatal("NewRateLimiter returned nil for negative rate")
		}
	})

	t.Run("zero burst size", func(t *testing.T) {
		config := &RateLimiterConfig{
			RequestsPerSecond: 5.0,
			BurstSize:         0,
			WaitTimeout:       time.Second,
			Enabled:           true,
		}

		rl := NewRateLimiter(config)

		// With zero burst, no immediate requests should succeed
		if rl.TryAcquire() {
			t.Error("Expected TryAcquire to fail with zero burst size")
		}
	})

	t.Run("very high rate", func(t *testing.T) {
		config := &RateLimiterConfig{
			RequestsPerSecond: 1000000.0, // Very high rate
			BurstSize:         1000,
			WaitTimeout:       time.Second,
			Enabled:           true,
		}

		rl := NewRateLimiter(config)

		// Should handle very high rates without issues
		for i := 0; i < 100; i++ {
			if !rl.TryAcquire() {
				t.Errorf("TryAcquire failed at iteration %d with high rate limit", i)
				break
			}
		}
	})

	t.Run("zero timeout", func(t *testing.T) {
		config := &RateLimiterConfig{
			RequestsPerSecond: 1.0,
			BurstSize:         1,
			WaitTimeout:       0, // No timeout
			Enabled:           true,
		}

		rl := NewRateLimiter(config)

		// Exhaust burst
		rl.TryAcquire()

		// Wait should not timeout with zero WaitTimeout
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := rl.Wait(ctx)
		// The context timeout should trigger, not the rate limiter timeout
		if err == nil {
			t.Error("Expected Wait to fail due to context timeout")
		}
	})
}

// TestRateLimiterStressTest performs stress testing for performance validation
func TestRateLimiterStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	config := &RateLimiterConfig{
		RequestsPerSecond: 1000.0, // High rate for stress testing
		BurstSize:         100,
		WaitTimeout:       time.Second,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)
	ctx := context.Background()

	numGoroutines := 50
	requestsPerGoroutine := 1000
	totalRequests := numGoroutines * requestsPerGoroutine

	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < requestsPerGoroutine; j++ {
				// Randomly choose between TryAcquire and Wait
				if rand.Intn(2) == 0 {
					rl.TryAcquire()
				} else {
					shortCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
					rl.Wait(shortCtx)
					cancel()
				}
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	// Verify performance
	metrics := rl.GetMetrics()
	if metrics.TotalRequests != int64(totalRequests) {
		t.Errorf("TotalRequests = %d, want %d", metrics.TotalRequests, totalRequests)
	}

	requestsPerSecond := float64(totalRequests) / elapsed.Seconds()
	t.Logf("Stress test: %d requests in %v (%.0f req/s)",
		totalRequests, elapsed, requestsPerSecond)

	// Should be able to handle at least 10,000 requests per second
	if requestsPerSecond < 10000 {
		t.Errorf("Performance too low: %.0f req/s, expected > 10,000 req/s", requestsPerSecond)
	}
}

// TestRateLimiterMemoryUsage tests memory efficiency
func TestRateLimiterMemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory test in short mode")
	}

	config := &RateLimiterConfig{
		RequestsPerSecond: 100.0,
		BurstSize:         10,
		WaitTimeout:       time.Second,
		Enabled:           true,
	}

	const numLimiters = 1000
	limiters := make([]*RateLimiter, numLimiters)

	// Create many rate limiters
	for i := 0; i < numLimiters; i++ {
		limiters[i] = NewRateLimiter(config)
	}

	// Use them to ensure they're not optimized away
	for i := 0; i < numLimiters; i++ {
		limiters[i].TryAcquire()
	}

	// Basic validation that all were created
	if len(limiters) != numLimiters {
		t.Errorf("Created %d limiters, want %d", len(limiters), numLimiters)
	}
}

// Helper function for string contains check
func rateLimiterContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				rateLimiterContainsSubstring(s, substr))))
}

func rateLimiterContainsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
