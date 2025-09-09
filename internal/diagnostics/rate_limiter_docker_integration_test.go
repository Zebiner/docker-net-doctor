package diagnostics

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRateLimiterDockerClientIntegration tests rate limiting with Docker client operations
func TestRateLimiterDockerClientIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Docker integration tests in short mode")
	}

	// Test with realistic Docker API rate limits
	config := &RateLimiterConfig{
		RequestsPerSecond: DOCKER_API_RATE_LIMIT, // 5 req/sec
		BurstSize:         DOCKER_API_BURST,      // 10 burst
		WaitTimeout:       5 * time.Second,
		Enabled:           true,
	}

	rl := NewRateLimiter(config)

	t.Run("container_list_operations", func(t *testing.T) {
		rl.ResetMetrics()
		ctx := context.Background()

		// Simulate multiple container list operations
		const numOperations = 15 // More than burst capacity
		successCount := 0
		timeoutCount := 0

		start := time.Now()

		for i := 0; i < numOperations; i++ {
			opCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			
			err := rl.Wait(opCtx)
			if err == nil {
				successCount++
				// Simulate Docker API call processing time
				time.Sleep(10 * time.Millisecond)
			} else {
				timeoutCount++
			}
			cancel()
		}

		elapsed := time.Since(start)

		t.Logf("Container list simulation: %d/%d successful, %d timeouts, elapsed: %v",
			successCount, numOperations, timeoutCount, elapsed)

		// Should allow burst initially, then be rate limited
		if successCount == 0 {
			t.Error("No operations succeeded - rate limiting too aggressive")
		}

		if successCount == numOperations {
			t.Error("All operations succeeded immediately - rate limiting not working")
		}

		// Should have allowed at least burst capacity
		if successCount < DOCKER_API_BURST {
			t.Errorf("Success count %d < minimum burst capacity %d", successCount, DOCKER_API_BURST)
		}

		// Verify metrics
		metrics := rl.GetMetrics()
		if metrics.TotalRequests != int64(numOperations) {
			t.Errorf("Total requests %d != expected %d", metrics.TotalRequests, numOperations)
		}

		// Throughput should respect rate limit
		actualThroughput := float64(successCount) / elapsed.Seconds()
		expectedMaxThroughput := float64(DOCKER_API_RATE_LIMIT) + float64(DOCKER_API_BURST)/elapsed.Seconds()
		
		if actualThroughput > expectedMaxThroughput*1.2 { // Allow 20% margin
			t.Errorf("Throughput %.2f exceeds expected max %.2f", 
				actualThroughput, expectedMaxThroughput)
		}
	})

	t.Run("mixed_docker_operations", func(t *testing.T) {
		rl.ResetMetrics()
		ctx := context.Background()

		// Simulate different Docker operations with different patterns
		operations := []struct {
			name     string
			count    int
			timeout  time.Duration
			delay    time.Duration
		}{
			{"list_containers", 5, 1*time.Second, 50*time.Millisecond},
			{"inspect_container", 8, 2*time.Second, 100*time.Millisecond},
			{"list_networks", 3, 1*time.Second, 30*time.Millisecond},
			{"inspect_network", 4, 2*time.Second, 75*time.Millisecond},
		}

		totalExpected := 0
		for _, op := range operations {
			totalExpected += op.count
		}

		var totalSuccess int64
		var totalFailures int64

		start := time.Now()

		// Execute operations in sequence
		for _, op := range operations {
			for i := 0; i < op.count; i++ {
				opCtx, cancel := context.WithTimeout(ctx, op.timeout)
				
				err := rl.Wait(opCtx)
				if err == nil {
					atomic.AddInt64(&totalSuccess, 1)
					// Simulate API processing time
					time.Sleep(op.delay)
				} else {
					atomic.AddInt64(&totalFailures, 1)
				}
				cancel()

				// Small delay between operations of same type
				time.Sleep(20 * time.Millisecond)
			}
		}

		elapsed := time.Since(start)

		t.Logf("Mixed operations: %d/%d successful, %d failures, elapsed: %v",
			totalSuccess, totalExpected, totalFailures, elapsed)

		// Should have reasonable success rate
		successRate := float64(totalSuccess) / float64(totalExpected)
		if successRate < 0.5 {
			t.Errorf("Success rate %.2f too low for mixed operations", successRate)
		}

		// Verify metrics consistency
		metrics := rl.GetMetrics()
		if metrics.TotalRequests != int64(totalExpected) {
			t.Errorf("Metrics total %d != expected %d", metrics.TotalRequests, totalExpected)
		}

		if metrics.AllowedRequests != totalSuccess {
			t.Errorf("Metrics allowed %d != actual success %d", metrics.AllowedRequests, totalSuccess)
		}
	})

	t.Run("concurrent_docker_diagnostic_engines", func(t *testing.T) {
		rl.ResetMetrics()
		ctx := context.Background()

		// Simulate multiple diagnostic engines sharing rate limiter
		numEngines := 3
		operationsPerEngine := 8

		var wg sync.WaitGroup
		var globalSuccess int64
		var globalFailures int64

		engineResults := make([]struct {
			engineID int
			success  int
			failures int
		}, numEngines)

		start := time.Now()

		for engineID := 0; engineID < numEngines; engineID++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				localSuccess := 0
				localFailures := 0

				for opID := 0; opID < operationsPerEngine; opID++ {
					// Vary timeout based on engine and operation type
					timeout := time.Duration(500+id*100+opID*50) * time.Millisecond
					opCtx, cancel := context.WithTimeout(ctx, timeout)

					err := rl.Wait(opCtx)
					if err == nil {
						localSuccess++
						atomic.AddInt64(&globalSuccess, 1)
						// Simulate diagnostic work
						time.Sleep(time.Duration(20+id*10) * time.Millisecond)
					} else {
						localFailures++
						atomic.AddInt64(&globalFailures, 1)
					}
					cancel()

					// Stagger operations within engine
					time.Sleep(time.Duration(30+id*20) * time.Millisecond)
				}

				engineResults[id] = struct {
					engineID int
					success  int
					failures int
				}{id, localSuccess, localFailures}
			}(engineID)
		}

		wg.Wait()
		elapsed := time.Since(start)

		totalExpected := numEngines * operationsPerEngine

		t.Logf("Concurrent engines simulation:")
		for _, result := range engineResults {
			t.Logf("  Engine %d: %d success, %d failures", 
				result.engineID, result.success, result.failures)
		}
		t.Logf("Total: %d/%d successful, elapsed: %v", globalSuccess, totalExpected, elapsed)

		// Should have reasonable distribution of success across engines
		if globalSuccess == 0 {
			t.Error("No operations succeeded across all engines")
		}

		// Each engine should get some share of the rate limit
		enginesWithSuccess := 0
		for _, result := range engineResults {
			if result.success > 0 {
				enginesWithSuccess++
			}
		}

		if enginesWithSuccess < 2 {
			t.Errorf("Only %d engines got successful operations, expected at least 2", 
				enginesWithSuccess)
		}

		// Verify fairness - no single engine should get all the requests
		for _, result := range engineResults {
			if result.success == int(globalSuccess) && numEngines > 1 {
				t.Errorf("Engine %d got all successful requests - unfair distribution",
					result.engineID)
			}
		}

		// Verify rate limiting is working
		actualThroughput := float64(globalSuccess) / elapsed.Seconds()
		maxExpectedThroughput := float64(DOCKER_API_RATE_LIMIT) + 
			float64(DOCKER_API_BURST)/elapsed.Seconds()

		if actualThroughput > maxExpectedThroughput*1.3 { // Allow 30% margin for concurrency
			t.Errorf("Concurrent throughput %.2f exceeds expected max %.2f",
				actualThroughput, maxExpectedThroughput)
		}

		// Verify metrics consistency
		metrics := rl.GetMetrics()
		if metrics.TotalRequests != int64(totalExpected) {
			t.Errorf("Metrics total %d != expected %d", metrics.TotalRequests, totalExpected)
		}
	})
}

// TestRateLimiterDockerAPIErrorScenarios tests error handling in Docker context
func TestRateLimiterDockerAPIErrorScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Docker error scenario tests in short mode")
	}

	t.Run("api_timeout_recovery", func(t *testing.T) {
		// Simulate API timeouts and recovery
		config := &RateLimiterConfig{
			RequestsPerSecond: 3.0, // Slower rate for testing
			BurstSize:         2,
			WaitTimeout:       200 * time.Millisecond, // Short timeout
			Enabled:           true,
		}

		rl := NewRateLimiter(config)
		rl.ResetMetrics()
		ctx := context.Background()

		// Phase 1: Exhaust capacity and cause timeouts
		rl.TryAcquire() // Use burst
		rl.TryAcquire() // Use burst

		timeoutCount := 0
		for i := 0; i < 5; i++ {
			shortCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
			err := rl.Wait(shortCtx)
			cancel()
			
			if err != nil {
				timeoutCount++
			}
		}

		if timeoutCount == 0 {
			t.Error("Expected some timeouts in overload scenario")
		}

		t.Logf("Phase 1: Generated %d timeouts as expected", timeoutCount)

		// Phase 2: Allow recovery
		time.Sleep(800 * time.Millisecond) // Allow token replenishment

		// Phase 3: Verify recovery
		successCount := 0
		for i := 0; i < 3; i++ {
			longCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			err := rl.Wait(longCtx)
			cancel()
			
			if err == nil {
				successCount++
			}
			time.Sleep(400 * time.Millisecond) // Allow replenishment
		}

		if successCount == 0 {
			t.Error("No recovery after timeout phase")
		}

		t.Logf("Phase 3: %d operations succeeded after recovery", successCount)

		// Verify metrics reflect the full scenario
		metrics := rl.GetMetrics()
		if metrics.TimeoutRequests == 0 && metrics.ThrottledRequests == 0 {
			t.Error("Expected timeout or throttling to be recorded")
		}

		if metrics.AllowedRequests == 0 {
			t.Error("Expected some allowed requests during recovery")
		}
	})

	t.Run("burst_exhaustion_with_long_operations", func(t *testing.T) {
		// Simulate long-running Docker operations
		config := &RateLimiterConfig{
			RequestsPerSecond: 4.0, // 250ms per token
			BurstSize:         3,
			WaitTimeout:       time.Second,
			Enabled:           true,
		}

		rl := NewRateLimiter(config)
		rl.ResetMetrics()
		ctx := context.Background()

		// Simulate operations that hold tokens for extended periods
		type operation struct {
			id       int
			duration time.Duration
			success  bool
			waitTime time.Duration
		}

		operations := []operation{
			{1, 100 * time.Millisecond, false, 0}, // Should succeed immediately
			{2, 200 * time.Millisecond, false, 0}, // Should succeed immediately  
			{3, 150 * time.Millisecond, false, 0}, // Should succeed immediately (burst exhausted)
			{4, 100 * time.Millisecond, false, 0}, // Should wait
			{5, 50 * time.Millisecond, false, 0},  // Should wait
		}

		start := time.Now()

		for i := range operations {
			opStart := time.Now()
			
			err := rl.Wait(ctx)
			waitTime := time.Since(opStart)
			operations[i].waitTime = waitTime
			
			if err == nil {
				operations[i].success = true
				// Simulate the actual Docker operation time
				time.Sleep(operations[i].duration)
			}
		}

		totalElapsed := time.Since(start)

		// Analyze results
		successCount := 0
		for _, op := range operations {
			if op.success {
				successCount++
			}
			t.Logf("Operation %d: success=%t, wait=%v, duration=%v", 
				op.id, op.success, op.waitTime, op.duration)
		}

		if successCount == 0 {
			t.Error("No operations completed successfully")
		}

		// First few operations should have minimal wait time (burst)
		for i := 0; i < 3 && i < len(operations); i++ {
			if operations[i].success && operations[i].waitTime > 50*time.Millisecond {
				t.Errorf("Operation %d had unexpected wait time %v (expected burst)",
					i+1, operations[i].waitTime)
			}
		}

		// Later operations should have longer wait times
		laterOpsWithLongWait := 0
		for i := 3; i < len(operations); i++ {
			if operations[i].success && operations[i].waitTime > 100*time.Millisecond {
				laterOpsWithLongWait++
			}
		}

		if laterOpsWithLongWait == 0 && successCount > 3 {
			t.Error("Expected some later operations to have longer wait times")
		}

		t.Logf("Long operations test: %d/%d successful, total elapsed: %v",
			successCount, len(operations), totalElapsed)
	})

	t.Run("rate_limit_with_diagnostic_priorities", func(t *testing.T) {
		// Simulate prioritized diagnostic operations
		config := &RateLimiterConfig{
			RequestsPerSecond: 6.0, // 167ms per token
			BurstSize:         4,
			WaitTimeout:       2 * time.Second,
			Enabled:           true,
		}

		rl := NewRateLimiter(config)
		rl.ResetMetrics()
		ctx := context.Background()

		// Different types of diagnostic operations with different priorities
		type diagnosticOp struct {
			name        string
			priority    int // Lower number = higher priority
			timeout     time.Duration
			count       int
		}

		diagnosticTypes := []diagnosticOp{
			{"critical_system_check", 1, 3*time.Second, 3},
			{"network_connectivity", 2, 2*time.Second, 5},
			{"container_inspection", 3, 1*time.Second, 4},
			{"detailed_analysis", 4, 500*time.Millisecond, 6},
		}

		results := make(map[string]struct {
			attempted int
			succeeded int
			timedOut  int
		})

		start := time.Now()

		// Execute operations by priority
		for _, diagType := range diagnosticTypes {
			results[diagType.name] = struct {
				attempted int
				succeeded int
				timedOut  int
			}{diagType.count, 0, 0}

			for i := 0; i < diagType.count; i++ {
				opCtx, cancel := context.WithTimeout(ctx, diagType.timeout)
				
				err := rl.Wait(opCtx)
				if err == nil {
					result := results[diagType.name]
					result.succeeded++
					results[diagType.name] = result
					
					// Simulate diagnostic processing time
					processingTime := time.Duration(50 + diagType.priority*25) * time.Millisecond
					time.Sleep(processingTime)
				} else {
					result := results[diagType.name]
					result.timedOut++
					results[diagType.name] = result
				}
				cancel()

				// Small delay between operations of same type
				time.Sleep(30 * time.Millisecond)
			}
		}

		elapsed := time.Since(start)

		// Analyze results by priority
		t.Logf("Prioritized diagnostic operations (elapsed: %v):", elapsed)
		totalAttempted := 0
		totalSucceeded := 0

		for _, diagType := range diagnosticTypes {
			result := results[diagType.name]
			successRate := float64(result.succeeded) / float64(result.attempted) * 100
			
			t.Logf("  %s (priority %d): %d/%d succeeded (%.1f%%), %d timed out",
				diagType.name, diagType.priority, result.succeeded, 
				result.attempted, successRate, result.timedOut)
			
			totalAttempted += result.attempted
			totalSucceeded += result.succeeded
		}

		// Higher priority operations should have better success rates
		criticalSuccess := float64(results["critical_system_check"].succeeded) / 
			float64(results["critical_system_check"].attempted)
		detailedSuccess := float64(results["detailed_analysis"].succeeded) / 
			float64(results["detailed_analysis"].attempted)

		if totalSucceeded > 0 && criticalSuccess < detailedSuccess-0.2 {
			t.Errorf("Critical operations success rate (%.2f) should be >= detailed operations (%.2f)",
				criticalSuccess, detailedSuccess)
		}

		// Overall throughput should respect rate limits
		actualThroughput := float64(totalSucceeded) / elapsed.Seconds()
		maxExpectedThroughput := 6.0 + 4.0/elapsed.Seconds() // rate + burst/time

		if actualThroughput > maxExpectedThroughput*1.2 {
			t.Errorf("Throughput %.2f exceeds expected max %.2f",
				actualThroughput, maxExpectedThroughput)
		}

		// Verify metrics
		metrics := rl.GetMetrics()
		if metrics.TotalRequests != int64(totalAttempted) {
			t.Errorf("Metrics total %d != expected %d", 
				metrics.TotalRequests, totalAttempted)
		}

		t.Logf("Overall: %d/%d operations succeeded (%.1f%%)",
			totalSucceeded, totalAttempted, 
			float64(totalSucceeded)/float64(totalAttempted)*100)
	})
}

// TestRateLimiterDockerPerformanceProfile tests performance under Docker-like loads
func TestRateLimiterDockerPerformanceProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Docker performance tests in short mode")
	}

	t.Run("sustained_docker_api_load", func(t *testing.T) {
		// Simulate sustained Docker API load over time
		config := &RateLimiterConfig{
			RequestsPerSecond: DOCKER_API_RATE_LIMIT,
			BurstSize:         DOCKER_API_BURST,
			WaitTimeout:       3 * time.Second,
			Enabled:           true,
		}

		rl := NewRateLimiter(config)
		rl.ResetMetrics()
		ctx := context.Background()

		// Run sustained load for a meaningful period
		testDuration := 5 * time.Second
		operationInterval := 50 * time.Millisecond // More frequent than rate limit

		var operationsAttempted int64
		var operationsSucceeded int64
		var operationsFailed int64

		start := time.Now()
		stopTime := start.Add(testDuration)

		for time.Now().Before(stopTime) {
			atomic.AddInt64(&operationsAttempted, 1)
			
			opCtx, cancel := context.WithTimeout(ctx, time.Second)
			err := rl.Wait(opCtx)
			cancel()
			
			if err == nil {
				atomic.AddInt64(&operationsSucceeded, 1)
				// Simulate Docker API response processing
				time.Sleep(20 * time.Millisecond)
			} else {
				atomic.AddInt64(&operationsFailed, 1)
			}
			
			time.Sleep(operationInterval)
		}

		elapsed := time.Since(start)
		
		// Calculate performance metrics
		successRate := float64(operationsSucceeded) / float64(operationsAttempted) * 100
		actualThroughput := float64(operationsSucceeded) / elapsed.Seconds()
		expectedThroughput := float64(DOCKER_API_RATE_LIMIT)

		t.Logf("Sustained load test:")
		t.Logf("  Duration: %v", elapsed)
		t.Logf("  Attempted: %d", operationsAttempted)
		t.Logf("  Succeeded: %d (%.1f%%)", operationsSucceeded, successRate)
		t.Logf("  Failed: %d", operationsFailed)
		t.Logf("  Throughput: %.2f req/s (expected: %.2f req/s)", 
			actualThroughput, expectedThroughput)

		// Verify throughput is close to expected rate limit
		throughputDifference := actualThroughput - expectedThroughput
		if throughputDifference < 0 {
			throughputDifference = -throughputDifference
		}

		// Allow 20% tolerance for timing variations
		tolerableThoughputDifference := expectedThroughput * 0.2
		if throughputDifference > tolerableThoughputDifference {
			t.Errorf("Throughput difference %.2f exceeds tolerance %.2f",
				throughputDifference, tolerableThoughputDifference)
		}

		// Should have reasonable success rate under sustained load
		if successRate < 30 {
			t.Errorf("Success rate %.1f%% too low under sustained load", successRate)
		}

		// Verify metrics consistency
		metrics := rl.GetMetrics()
		if metrics.TotalRequests != operationsAttempted {
			t.Errorf("Metrics total %d != attempted %d", 
				metrics.TotalRequests, operationsAttempted)
		}

		if metrics.AllowedRequests != operationsSucceeded {
			t.Errorf("Metrics allowed %d != succeeded %d",
				metrics.AllowedRequests, operationsSucceeded)
		}
	})

	t.Run("docker_diagnostic_burst_patterns", func(t *testing.T) {
		// Simulate realistic diagnostic burst patterns
		config := &RateLimiterConfig{
			RequestsPerSecond: DOCKER_API_RATE_LIMIT,
			BurstSize:         DOCKER_API_BURST,
			WaitTimeout:       2 * time.Second,
			Enabled:           true,
		}

		rl := NewRateLimiter(config)
		ctx := context.Background()

		// Simulate different diagnostic phases
		phases := []struct {
			name           string
			burstSize      int
			burstInterval  time.Duration
			idleTime       time.Duration
		}{
			{"initial_discovery", 12, 100*time.Millisecond, 2*time.Second},
			{"detailed_inspection", 8, 200*time.Millisecond, 1*time.Second},
			{"connectivity_tests", 15, 150*time.Millisecond, 3*time.Second},
			{"final_validation", 6, 100*time.Millisecond, 0},
		}

		overallStart := time.Now()
		var overallSuccess int64
		var overallAttempted int64

		for phaseIdx, phase := range phases {
			rl.ResetMetrics() // Reset for each phase
			phaseStart := time.Now()

			// Execute burst
			phaseSuccess := 0
			for i := 0; i < phase.burstSize; i++ {
				opCtx, cancel := context.WithTimeout(ctx, time.Second)
				err := rl.Wait(opCtx)
				cancel()
				
				atomic.AddInt64(&overallAttempted, 1)
				
				if err == nil {
					phaseSuccess++
					atomic.AddInt64(&overallSuccess, 1)
				}
				
				if i < phase.burstSize-1 {
					time.Sleep(phase.burstInterval)
				}
			}

			phaseElapsed := time.Since(phaseStart)
			phaseThroughput := float64(phaseSuccess) / phaseElapsed.Seconds()

			t.Logf("Phase %d (%s): %d/%d successful in %v (%.2f req/s)",
				phaseIdx+1, phase.name, phaseSuccess, phase.burstSize,
				phaseElapsed, phaseThroughput)

			// Idle period between phases
			if phase.idleTime > 0 {
				time.Sleep(phase.idleTime)
			}
		}

		overallElapsed := time.Since(overallStart)
		overallThroughput := float64(overallSuccess) / overallElapsed.Seconds()

		t.Logf("Overall diagnostic pattern: %d/%d successful in %v (%.2f req/s)",
			overallSuccess, overallAttempted, overallElapsed, overallThroughput)

		// Should have good success rate with realistic burst patterns
		successRate := float64(overallSuccess) / float64(overallAttempted) * 100
		if successRate < 50 {
			t.Errorf("Overall success rate %.1f%% too low for diagnostic patterns", 
				successRate)
		}

		// Throughput should be reasonable given the idle periods
		// (Lower than sustained rate due to bursty nature)
		if overallThroughput > float64(DOCKER_API_RATE_LIMIT)*2 {
			t.Errorf("Overall throughput %.2f too high, may indicate rate limiting failure",
				overallThroughput)
		}
	})
}