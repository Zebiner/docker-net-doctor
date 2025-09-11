package diagnostics

import (
	"context"
	"sync"
	"testing"
	"time"
)

// BenchmarkRateLimiterTryAcquire benchmarks the TryAcquire method
func BenchmarkRateLimiterTryAcquire(b *testing.B) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 10000.0, // High rate to minimize throttling
		BurstSize:         1000,
		WaitTimeout:       time.Second,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rl.TryAcquire()
		}
	})
}

// BenchmarkRateLimiterTryAcquireDisabled benchmarks TryAcquire when disabled
func BenchmarkRateLimiterTryAcquireDisabled(b *testing.B) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 1.0,
		BurstSize:         1,
		WaitTimeout:       time.Second,
		Enabled:           false, // Disabled for maximum performance
	}

	rl := NewRateLimiter(config)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rl.TryAcquire()
		}
	})
}

// BenchmarkRateLimiterWait benchmarks the Wait method
func BenchmarkRateLimiterWait(b *testing.B) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 10000.0, // High rate to minimize waiting
		BurstSize:         1000,
		WaitTimeout:       time.Second,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Wait(ctx)
	}
}

// BenchmarkRateLimiterMetricsRetrieval benchmarks metrics retrieval
func BenchmarkRateLimiterMetricsRetrieval(b *testing.B) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 100.0,
		BurstSize:         10,
		WaitTimeout:       time.Second,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)

	// Populate some metrics
	for i := 0; i < 100; i++ {
		rl.TryAcquire()
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = rl.GetMetrics()
		}
	})
}

// BenchmarkRateLimiterConcurrentOperations benchmarks concurrent operations
func BenchmarkRateLimiterConcurrentOperations(b *testing.B) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 10000.0,
		BurstSize:         1000,
		WaitTimeout:       time.Second,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Mix operations to simulate real usage
			switch b.N % 3 {
			case 0:
				rl.TryAcquire()
			case 1:
				rl.Wait(ctx)
			case 2:
				_ = rl.GetMetrics()
			}
		}
	})
}

// BenchmarkRateLimiterConfigUpdate benchmarks configuration updates
func BenchmarkRateLimiterConfigUpdate(b *testing.B) {
	rl := NewRateLimiter(nil)

	configs := []*RateLimiterConfig{
		{RequestsPerSecond: 10.0, BurstSize: 10, Enabled: true},
		{RequestsPerSecond: 20.0, BurstSize: 20, Enabled: true},
		{RequestsPerSecond: 5.0, BurstSize: 5, Enabled: false},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		config := configs[i%len(configs)]
		rl.UpdateConfig(config)
	}
}

// TestRateLimiterTimingPrecision tests timing accuracy under various conditions
func TestRateLimiterTimingPrecision(t *testing.T) {
	tests := []struct {
		name              string
		requestsPerSecond float64
		expectedInterval  time.Duration
		tolerance         time.Duration
	}{
		{
			name:              "1 req/sec",
			requestsPerSecond: 1.0,
			expectedInterval:  1000 * time.Millisecond,
			tolerance:         100 * time.Millisecond,
		},
		{
			name:              "2 req/sec",
			requestsPerSecond: 2.0,
			expectedInterval:  500 * time.Millisecond,
			tolerance:         50 * time.Millisecond,
		},
		{
			name:              "10 req/sec",
			requestsPerSecond: 10.0,
			expectedInterval:  100 * time.Millisecond,
			tolerance:         20 * time.Millisecond,
		},
		{
			name:              "100 req/sec",
			requestsPerSecond: 100.0,
			expectedInterval:  10 * time.Millisecond,
			tolerance:         5 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &RateLimiterConfig{
				RequestsPerSecond: tt.requestsPerSecond,
				BurstSize:         1, // Small burst to force timing
				WaitTimeout:       5 * time.Second,
				Enabled:           true,
			}

			rl := NewRateLimiter(config)
			ctx := context.Background()

			// Exhaust initial burst
			rl.TryAcquire()

			// Measure timing for multiple requests
			const numMeasurements = 5
			intervals := make([]time.Duration, numMeasurements)

			for i := 0; i < numMeasurements; i++ {
				start := time.Now()
				err := rl.Wait(ctx)
				if err != nil {
					t.Fatalf("Wait failed: %v", err)
				}
				intervals[i] = time.Since(start)
			}

			// Analyze timing accuracy
			for i, interval := range intervals {
				diff := interval - tt.expectedInterval
				if diff < 0 {
					diff = -diff
				}

				if diff > tt.tolerance {
					t.Errorf("Measurement %d: interval %v differs from expected %v by %v (tolerance: %v)",
						i+1, interval, tt.expectedInterval, diff, tt.tolerance)
				}
			}

			// Calculate average for additional validation
			var total time.Duration
			for _, interval := range intervals {
				total += interval
			}
			average := total / time.Duration(numMeasurements)

			avgDiff := average - tt.expectedInterval
			if avgDiff < 0 {
				avgDiff = -avgDiff
			}

			if avgDiff > tt.tolerance {
				t.Errorf("Average interval %v differs from expected %v by %v (tolerance: %v)",
					average, tt.expectedInterval, avgDiff, tt.tolerance)
			}
		})
	}
}

// TestRateLimiterBurstRecoveryTiming tests burst capacity recovery timing
func TestRateLimiterBurstRecoveryTiming(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timing test in short mode")
	}

	config := &RateLimiterConfig{
		RequestsPerSecond: 4.0, // 4 req/sec = 250ms per token
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

	// Test 1: Before token recovery, should fail
	time.Sleep(200 * time.Millisecond) // Less than 250ms
	if rl.TryAcquire() {
		t.Error("TryAcquire should fail before token recovery time")
	}

	// Test 2: After token recovery, should succeed
	time.Sleep(100 * time.Millisecond) // Total 300ms > 250ms
	if !rl.TryAcquire() {
		t.Error("TryAcquire should succeed after token recovery time")
	}

	// Test 3: Immediate retry should fail (no tokens available)
	if rl.TryAcquire() {
		t.Error("Immediate retry should fail when no tokens available")
	}

	// Test 4: After another recovery period, should succeed again
	time.Sleep(300 * time.Millisecond) // Wait for next token
	if !rl.TryAcquire() {
		t.Error("TryAcquire should succeed after second recovery period")
	}
}

// TestRateLimiterConsistentThroughput tests throughput consistency over time
func TestRateLimiterConsistentThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping throughput test in short mode")
	}

	config := &RateLimiterConfig{
		RequestsPerSecond: 10.0, // 10 req/sec target
		BurstSize:         2,
		WaitTimeout:       time.Second,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)
	ctx := context.Background()

	// Test duration and measurement windows
	testDuration := 1 * time.Second      // Reduced from 3s to 1s for faster tests
	windowSize := 200 * time.Millisecond // Reduced window size for better granularity
	numWindows := int(testDuration / windowSize)

	start := time.Now()
	windowCounts := make([]int, numWindows)
	currentWindow := 0

	// Run requests and count per window
	for time.Since(start) < testDuration {
		windowIndex := int(time.Since(start) / windowSize)
		if windowIndex >= numWindows {
			break
		}

		err := rl.Wait(ctx)
		if err == nil {
			windowCounts[windowIndex]++
		}

		currentWindow = windowIndex
	}

	// Analyze throughput consistency
	expectedPerWindow := float64(config.RequestsPerSecond) * windowSize.Seconds()
	tolerance := expectedPerWindow * 0.3 // 30% tolerance

	for i := 1; i < currentWindow; i++ { // Skip first window (may be partial)
		count := float64(windowCounts[i])
		if count < expectedPerWindow-tolerance || count > expectedPerWindow+tolerance {
			t.Errorf("Window %d: got %.0f requests, expected %.1f ± %.1f",
				i, count, expectedPerWindow, tolerance)
		}
	}

	// Overall throughput check
	totalRequests := 0
	for i := 1; i < currentWindow; i++ {
		totalRequests += windowCounts[i]
	}

	actualDuration := time.Duration(currentWindow-1) * windowSize
	actualThroughput := float64(totalRequests) / actualDuration.Seconds()

	expectedThroughput := config.RequestsPerSecond
	throughputTolerance := expectedThroughput * 0.2 // 20% tolerance

	if actualThroughput < expectedThroughput-throughputTolerance ||
		actualThroughput > expectedThroughput+throughputTolerance {
		t.Errorf("Overall throughput: %.2f req/s, expected %.1f ± %.1f req/s",
			actualThroughput, expectedThroughput, throughputTolerance)
	}
}

// TestRateLimiterMetricsTimingAccuracy tests timing metrics accuracy
func TestRateLimiterMetricsTimingAccuracy(t *testing.T) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 5.0, // 200ms per token
		BurstSize:         1,
		WaitTimeout:       time.Second,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)
	rl.ResetMetrics()
	ctx := context.Background()

	// Exhaust burst
	rl.TryAcquire()

	// Perform several timed waits
	const numWaits = 5
	expectedWaitTime := 200 * time.Millisecond // 1/5 second
	tolerance := 50 * time.Millisecond

	var totalMeasuredTime time.Duration
	for i := 0; i < numWaits; i++ {
		start := time.Now()
		err := rl.Wait(ctx)
		actualWait := time.Since(start)

		if err != nil {
			t.Fatalf("Wait %d failed: %v", i+1, err)
		}

		totalMeasuredTime += actualWait

		// Verify individual wait time
		diff := actualWait - expectedWaitTime
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			t.Errorf("Wait %d took %v, expected %v ± %v",
				i+1, actualWait, expectedWaitTime, tolerance)
		}
	}

	// Compare with metrics
	metrics := rl.GetMetrics()
	avgWaitFromMetrics := rl.GetAverageWaitTime()
	actualAverage := totalMeasuredTime / numWaits

	// Metrics should be reasonably close to measured times
	metricsDiff := avgWaitFromMetrics - actualAverage
	if metricsDiff < 0 {
		metricsDiff = -metricsDiff
	}

	maxDiff := tolerance * 2 // Allow more tolerance for averaged values
	if metricsDiff > maxDiff {
		t.Errorf("Metrics average wait time %v differs from measured %v by %v (max allowed: %v)",
			avgWaitFromMetrics, actualAverage, metricsDiff, maxDiff)
	}

	// Verify max wait time tracking
	if metrics.MaxWaitTime == 0 {
		t.Error("MaxWaitTime should be greater than 0")
	}

	// Max wait time should be within reasonable bounds
	if metrics.MaxWaitTime < expectedWaitTime/2 ||
		metrics.MaxWaitTime > expectedWaitTime*3 {
		t.Errorf("MaxWaitTime %v seems unreasonable for expected %v",
			metrics.MaxWaitTime, expectedWaitTime)
	}
}

// TestRateLimiterTimeWindowBoundaries tests behavior at time window boundaries
func TestRateLimiterTimeWindowBoundaries(t *testing.T) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 2.0, // 500ms per token
		BurstSize:         1,
		WaitTimeout:       time.Second,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)
	ctx := context.Background()

	// Exhaust burst
	if !rl.TryAcquire() {
		t.Fatal("Failed to acquire initial token")
	}

	// Wait for exactly the replenishment period
	time.Sleep(500 * time.Millisecond)

	// Should be able to acquire immediately
	start := time.Now()
	err := rl.Wait(ctx)
	waitTime := time.Since(start)

	if err != nil {
		t.Errorf("Wait failed after replenishment period: %v", err)
	}

	// Should have waited minimal time (near zero)
	if waitTime > 50*time.Millisecond {
		t.Errorf("Wait took %v after replenishment, expected near-zero wait", waitTime)
	}

	// Verify another request requires waiting
	start = time.Now()
	err = rl.Wait(ctx)
	waitTime = time.Since(start)

	if err != nil {
		t.Errorf("Second wait failed: %v", err)
	}

	// Should have waited approximately the full period
	expectedWait := 500 * time.Millisecond
	if waitTime < expectedWait-100*time.Millisecond ||
		waitTime > expectedWait+100*time.Millisecond {
		t.Errorf("Second wait took %v, expected ~%v", waitTime, expectedWait)
	}
}

// TestRateLimiterConcurrentMetricsConsistency tests metrics consistency under concurrency
func TestRateLimiterConcurrentMetricsConsistency(t *testing.T) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 50.0, // Moderate rate
		BurstSize:         10,
		WaitTimeout:       100 * time.Millisecond,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)
	ctx := context.Background()

	numGoroutines := 10
	operationsPerGoroutine := 100
	totalOperations := numGoroutines * operationsPerGoroutine

	var wg sync.WaitGroup
	start := time.Now()

	// Launch concurrent operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				// Mix operations
				if j%3 == 0 {
					rl.TryAcquire()
				} else {
					shortCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
					rl.Wait(shortCtx)
					cancel()
				}
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	// Verify metrics consistency
	metrics := rl.GetMetrics()

	// Total requests should match exactly
	if metrics.TotalRequests != int64(totalOperations) {
		t.Errorf("TotalRequests = %d, want %d", metrics.TotalRequests, totalOperations)
	}

	// All requests should be accounted for
	accountedRequests := metrics.AllowedRequests + metrics.ThrottledRequests + metrics.TimeoutRequests
	if accountedRequests != metrics.TotalRequests {
		t.Errorf("Accounted requests (%d) != total requests (%d)",
			accountedRequests, metrics.TotalRequests)
	}

	// Throughput should be reasonable
	actualThroughput := float64(metrics.AllowedRequests) / elapsed.Seconds()
	maxExpectedThroughput := config.RequestsPerSecond + float64(config.BurstSize)/elapsed.Seconds()

	if actualThroughput > maxExpectedThroughput*1.1 { // Allow 10% margin
		t.Errorf("Throughput %.2f req/s exceeds expected maximum %.2f req/s",
			actualThroughput, maxExpectedThroughput)
	}

	// Timing metrics should be reasonable
	avgWait := rl.GetAverageWaitTime()
	if metrics.AllowedRequests > 0 && avgWait < 0 {
		t.Error("Average wait time should not be negative")
	}

	if metrics.MaxWaitTime < 0 {
		t.Error("Max wait time should not be negative")
	}

	if avgWait > metrics.MaxWaitTime {
		t.Errorf("Average wait time (%v) should not exceed max wait time (%v)",
			avgWait, metrics.MaxWaitTime)
	}
}
