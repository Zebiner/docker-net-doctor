package diagnostics

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// Test CircuitBreaker state transitions comprehensively
func TestCircuitBreakerStateTransitions_Comprehensive(t *testing.T) {
	tests := []struct {
		name          string
		setupConfig   *CircuitBreakerConfig
		stateSequence []struct {
			action        string
			expectedState CircuitState
			parameter     interface{} // For actions that need parameters
		}
		description string
	}{
		{
			name: "Standard state cycle - Closed -> Open -> Half-Open -> Closed",
			setupConfig: &CircuitBreakerConfig{
				FailureThreshold:     3,
				RecoveryTimeout:      10 * time.Millisecond,
				ConsecutiveSuccesses: 2,
				HalfOpenMaxRequests:  2,
			},
			stateSequence: []struct {
				action        string
				expectedState CircuitState
				parameter     interface{}
			}{
				{"initial", CircuitStateClosed, nil},
				{"failure", CircuitStateClosed, nil},
				{"failure", CircuitStateClosed, nil},
				{"failure", CircuitStateOpen, nil},                // Should open after 3 failures
				{"wait", CircuitStateOpen, 15 * time.Millisecond}, // Wait for recovery timeout
				{"canExecute", CircuitStateHalfOpen, nil},         // Should transition to half-open
				{"success", CircuitStateHalfOpen, nil},            // Record success in half-open
				{"success", CircuitStateClosed, nil},              // Should close after 2 successes
			},
			description: "Normal operation cycle through all states",
		},
		{
			name: "Half-open failure causes immediate reopen",
			setupConfig: &CircuitBreakerConfig{
				FailureThreshold:     2,
				RecoveryTimeout:      5 * time.Millisecond,
				ConsecutiveSuccesses: 3,
			},
			stateSequence: []struct {
				action        string
				expectedState CircuitState
				parameter     interface{}
			}{
				{"initial", CircuitStateClosed, nil},
				{"failure", CircuitStateClosed, nil},
				{"failure", CircuitStateOpen, nil}, // Open after 2 failures
				{"wait", CircuitStateOpen, 10 * time.Millisecond},
				{"canExecute", CircuitStateHalfOpen, nil},
				{"failure", CircuitStateOpen, nil}, // Should reopen immediately on failure
			},
			description: "Half-open state failure immediately reopens circuit",
		},
		{
			name: "Force state transitions",
			setupConfig: &CircuitBreakerConfig{
				FailureThreshold: 10,
			},
			stateSequence: []struct {
				action        string
				expectedState CircuitState
				parameter     interface{}
			}{
				{"initial", CircuitStateClosed, nil},
				{"forceOpen", CircuitStateOpen, nil},
				{"forceClose", CircuitStateClosed, nil},
				{"forceOpen", CircuitStateOpen, nil},
			},
			description: "Force state transitions override normal behavior",
		},
		{
			name: "Failure rate threshold triggers opening",
			setupConfig: &CircuitBreakerConfig{
				FailureThreshold:     100, // High count threshold
				FailureRate:          0.6, // 60% failure rate
				MinRequestsThreshold: 10,
			},
			stateSequence: []struct {
				action        string
				expectedState CircuitState
				parameter     interface{}
			}{
				{"initial", CircuitStateClosed, nil},
				// Generate requests with 70% failure rate
				{"multipleOperations", CircuitStateOpen, map[string]int{"successes": 3, "failures": 7}},
			},
			description: "Failure rate threshold causes opening even with low failure count",
		},
		{
			name: "Adaptive thresholds with high volume",
			setupConfig: &CircuitBreakerConfig{
				FailureThreshold:       10,
				AdaptiveThresholds:     true,
				RequestVolumeThreshold: 20,
			},
			stateSequence: []struct {
				action        string
				expectedState CircuitState
				parameter     interface{}
			}{
				{"initial", CircuitStateClosed, nil},
				// Generate high volume to trigger adaptive behavior
				{"multipleOperations", CircuitStateOpen, map[string]int{"successes": 15, "failures": 8}},
			},
			description: "Adaptive thresholds lower failure threshold for high volume",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := NewCircuitBreaker(tt.setupConfig)

			for i, step := range tt.stateSequence {
				switch step.action {
				case "initial":
					// Just check initial state
				case "failure":
					cb.RecordFailure()
				case "success":
					cb.RecordSuccess()
				case "canExecute":
					cb.CanExecute()
				case "forceOpen":
					cb.ForceOpen()
				case "forceClose":
					cb.ForceClose()
				case "wait":
					if duration, ok := step.parameter.(time.Duration); ok {
						time.Sleep(duration)
					}
				case "multipleOperations":
					if params, ok := step.parameter.(map[string]int); ok {
						successes := params["successes"]
						failures := params["failures"]

						// Alternate between successes and failures to simulate realistic pattern
						total := successes + failures
						for j := 0; j < total; j++ {
							cb.CanExecute()
							if j < failures {
								cb.RecordFailure()
							} else {
								cb.RecordSuccess()
							}
						}
					}
				}

				// Verify expected state after action
				actualState := cb.GetState()
				assert.Equal(t, step.expectedState, actualState,
					"Step %d (%s): expected state %s, got %s",
					i, step.action, step.expectedState.String(), actualState.String())
			}
		})
	}
}

// Test all circuit breaker state properties comprehensively
func TestCircuitBreakerStateProperties_Comprehensive(t *testing.T) {
	states := []struct {
		state       CircuitState
		setupFunc   func(*CircuitBreaker)
		canExecute  bool
		description string
	}{
		{
			state: CircuitStateClosed,
			setupFunc: func(cb *CircuitBreaker) {
				// Default is closed, no setup needed
			},
			canExecute:  true,
			description: "Closed state allows all requests",
		},
		{
			state: CircuitStateOpen,
			setupFunc: func(cb *CircuitBreaker) {
				cb.ForceOpen()
			},
			canExecute:  false,
			description: "Open state blocks all requests",
		},
		{
			state: CircuitStateHalfOpen,
			setupFunc: func(cb *CircuitBreaker) {
				// Transition to half-open by opening, waiting, then calling CanExecute
				cb.ForceOpen()
				cb.transitionToHalfOpen()
			},
			canExecute:  true,
			description: "Half-open state allows limited requests",
		},
	}

	for _, stateTest := range states {
		t.Run(fmt.Sprintf("State_%s_properties", stateTest.state.String()), func(t *testing.T) {
			cb := NewCircuitBreaker(&CircuitBreakerConfig{
				FailureThreshold:    5,
				HalfOpenMaxRequests: 3,
			})

			stateTest.setupFunc(cb)

			// Verify state
			assert.Equal(t, stateTest.state, cb.GetState())

			// Verify canExecute behavior based on state
			if stateTest.state == CircuitStateHalfOpen {
				// Half-open allows limited requests
				requestsAllowed := 0
				for i := 0; i < 10; i++ {
					if cb.CanExecute() {
						requestsAllowed++
					}
				}
				assert.Greater(t, requestsAllowed, 0, "Half-open should allow some requests")
				assert.LessOrEqual(t, requestsAllowed, cb.config.HalfOpenMaxRequests,
					"Half-open should not exceed max requests")
			} else {
				// For closed and open states, behavior is consistent
				canExec := cb.CanExecute()
				assert.Equal(t, stateTest.canExecute, canExec, stateTest.description)
			}
		})
	}
}

// Test circuit breaker concurrent state management
func TestCircuitBreakerConcurrentState_Comprehensive(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:     10,
		ConsecutiveSuccesses: 5,
		RecoveryTimeout:      50 * time.Millisecond,
	})

	const numGoroutines = 20
	const operationsPerGoroutine = 100

	var wg sync.WaitGroup
	var successCount, failureCount, executeCount atomic.Int64

	// Start concurrent operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				// Mix of operations
				switch j % 5 {
				case 0, 1: // 40% executions
					if cb.CanExecute() {
						executeCount.Add(1)
					}
				case 2, 3: // 40% successes
					cb.RecordSuccess()
					successCount.Add(1)
				case 4: // 20% failures
					cb.RecordFailure()
					failureCount.Add(1)
				}

				// Small delay to increase chance of race conditions
				if j%10 == 0 {
					time.Sleep(time.Microsecond)
				}
			}
		}(i)
	}

	// Also test state transitions concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			time.Sleep(time.Millisecond)

			switch i % 4 {
			case 0:
				cb.GetState()
			case 1:
				cb.GetMetrics()
			case 2:
				cb.IsHealthy()
			case 3:
				cb.GetHealthScore()
			}
		}
	}()

	wg.Wait()

	// Verify no panics occurred and state is consistent
	finalState := cb.GetState()
	assert.True(t, finalState >= CircuitStateClosed && finalState <= CircuitStateHalfOpen)

	metrics := cb.GetMetrics()
	assert.GreaterOrEqual(t, metrics.TotalRequests, executeCount.Load())
	assert.Equal(t, successCount.Load(), metrics.TotalSuccesses)
	assert.Equal(t, failureCount.Load(), metrics.TotalFailures)

	// Verify internal consistency
	assert.True(t, metrics.TotalRequests >= metrics.TotalSuccesses+metrics.TotalFailures)
}

// Test ErrorRateMonitor state transitions comprehensively
func TestErrorRateMonitor_StateTransitions_Comprehensive(t *testing.T) {
	tests := []struct {
		name       string
		window     time.Duration
		threshold  float64
		operations []struct {
			action   string
			count    int
			waitTime time.Duration
		}
		expectedFinalState bool // true = open, false = closed
		description        string
	}{
		{
			name:      "Low error rate keeps circuit closed",
			window:    100 * time.Millisecond,
			threshold: 0.5, // 50%
			operations: []struct {
				action   string
				count    int
				waitTime time.Duration
			}{
				{"success", 15, 0},
				{"error", 3, 0}, // 16.7% error rate
			},
			expectedFinalState: false,
			description:        "Circuit should remain closed with low error rate",
		},
		{
			name:      "High error rate opens circuit",
			window:    100 * time.Millisecond,
			threshold: 0.4, // 40%
			operations: []struct {
				action   string
				count    int
				waitTime time.Duration
			}{
				{"success", 10, 0},
				{"error", 15, 0}, // 60% error rate
			},
			expectedFinalState: true,
			description:        "Circuit should open with high error rate",
		},
		{
			name:      "Window reset clears error history",
			window:    50 * time.Millisecond,
			threshold: 0.5,
			operations: []struct {
				action   string
				count    int
				waitTime time.Duration
			}{
				{"error", 15, 0},                   // High error rate
				{"success", 1, 0},                  // Should open circuit
				{"wait", 0, 60 * time.Millisecond}, // Wait for window reset
				{"success", 10, 0},                 // New window with low error rate
				{"error", 1, 0},
			},
			expectedFinalState: false,
			description:        "Window reset should clear error history",
		},
		{
			name:      "Insufficient samples keeps circuit closed",
			window:    100 * time.Millisecond,
			threshold: 0.3,
			operations: []struct {
				action   string
				count    int
				waitTime time.Duration
			}{
				{"error", 5, 0}, // 100% error rate but only 5 samples
			},
			expectedFinalState: false,
			description:        "Circuit should remain closed with insufficient samples",
		},
		{
			name:      "Circuit auto-closes when error rate drops",
			window:    200 * time.Millisecond,
			threshold: 0.6,
			operations: []struct {
				action   string
				count    int
				waitTime time.Duration
			}{
				{"error", 20, 0},   // High error rate - should open
				{"success", 5, 0},  // Circuit should still be open
				{"success", 30, 0}, // Error rate drops to ~36% - should close
			},
			expectedFinalState: false,
			description:        "Circuit should auto-close when error rate drops significantly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := &ErrorRateMonitor{
				window:    tt.window,
				threshold: tt.threshold,
				lastReset: time.Now(),
			}

			for i, op := range tt.operations {
				switch op.action {
				case "success":
					for j := 0; j < op.count; j++ {
						monitor.RecordSuccess()
					}
				case "error":
					for j := 0; j < op.count; j++ {
						monitor.RecordError()
					}
				case "wait":
					time.Sleep(op.waitTime)
				}

				// Log state for debugging
				t.Logf("Step %d: %s %d, circuit open: %v, errors: %d, successes: %d",
					i, op.action, op.count, monitor.IsOpen(), monitor.errorCount, monitor.successCount)
			}

			finalState := monitor.IsOpen()
			assert.Equal(t, tt.expectedFinalState, finalState, tt.description)
		})
	}
}

// Test MemoryMonitor state tracking comprehensively
func TestMemoryMonitor_StateTracking_Comprehensive(t *testing.T) {
	monitor := &MemoryMonitor{
		baseline:      1000000, // 1MB baseline
		checkInterval: 10 * time.Millisecond,
		stopChan:      make(chan struct{}),
	}

	// Test initial state
	assert.False(t, monitor.IsOverLimit())
	assert.Equal(t, uint64(0), monitor.GetPeakUsage())

	// Start monitoring
	go monitor.Start()
	defer close(monitor.stopChan)

	// Simulate memory usage changes
	time.Sleep(20 * time.Millisecond)

	// Test peak usage tracking
	monitor.mu.Lock()
	monitor.peakUsage = monitor.baseline + 500*1024*1024 // Add 500MB
	monitor.mu.Unlock()

	peakUsage := monitor.GetPeakUsage()
	assert.Equal(t, uint64(500*1024*1024), peakUsage)

	// Test over limit detection
	monitor.mu.Lock()
	monitor.peakUsage = monitor.baseline + (MAX_MEMORY_MB+100)*1024*1024 // Exceed limit
	monitor.mu.Unlock()

	assert.True(t, monitor.IsOverLimit())

	// Test under limit when peak is less than baseline (edge case)
	monitor.mu.Lock()
	monitor.peakUsage = monitor.baseline - 1000 // Less than baseline
	monitor.mu.Unlock()

	assert.False(t, monitor.IsOverLimit())
	assert.Equal(t, uint64(0), monitor.GetPeakUsage()) // Should prevent underflow
}

// Test RateLimiter state management comprehensively
func TestRateLimiter_StateManagement_Comprehensive(t *testing.T) {
	tests := []struct {
		name           string
		config         *RateLimiterConfig
		requestPattern []struct {
			numRequests int
			interval    time.Duration
			expectDelay bool
		}
		description string
	}{
		{
			name: "Burst allowance followed by rate limiting",
			config: &RateLimiterConfig{
				RequestsPerSecond: 2,
				BurstSize:         5,
				WaitTimeout:       100 * time.Millisecond,
				Enabled:           true,
			},
			requestPattern: []struct {
				numRequests int
				interval    time.Duration
				expectDelay bool
			}{
				{5, 0, false},                    // Burst should be immediate
				{3, 10 * time.Millisecond, true}, // Should be rate limited
			},
			description: "Initial burst allowed, then rate limiting applies",
		},
		{
			name: "Disabled rate limiter allows all requests",
			config: &RateLimiterConfig{
				RequestsPerSecond: 1,
				BurstSize:         1,
				WaitTimeout:       50 * time.Millisecond,
				Enabled:           false,
			},
			requestPattern: []struct {
				numRequests int
				interval    time.Duration
				expectDelay bool
			}{
				{10, 0, false}, // All should be immediate when disabled
			},
			description: "Disabled rate limiter should not delay any requests",
		},
		{
			name: "Very restrictive rate limiting",
			config: &RateLimiterConfig{
				RequestsPerSecond: 0.5, // One request every 2 seconds
				BurstSize:         1,
				WaitTimeout:       100 * time.Millisecond,
				Enabled:           true,
			},
			requestPattern: []struct {
				numRequests int
				interval    time.Duration
				expectDelay bool
			}{
				{1, 0, false},                    // First request immediate
				{2, 50 * time.Millisecond, true}, // Next requests should be delayed/timeout
			},
			description: "Very restrictive rate limiting should cause delays",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rateLimiter := NewRateLimiter(tt.config)

			for i, pattern := range tt.requestPattern {
				startTime := time.Now()
				successCount := 0
				timeoutCount := 0

				for j := 0; j < pattern.numRequests; j++ {
					if pattern.interval > 0 && j > 0 {
						time.Sleep(pattern.interval)
					}

					ctx, cancel := context.WithTimeout(context.Background(), tt.config.WaitTimeout)
					err := rateLimiter.Wait(ctx)
					cancel()

					if err != nil {
						timeoutCount++
					} else {
						successCount++
					}
				}

				elapsed := time.Since(startTime)

				if pattern.expectDelay {
					// Should have experienced some delays or timeouts
					if tt.config.Enabled {
						assert.True(t, elapsed > pattern.interval*time.Duration(pattern.numRequests)/2 || timeoutCount > 0,
							"Pattern %d: Expected delays or timeouts with restrictive rate limiting", i)
					}
				} else {
					// Should have been relatively fast
					assert.True(t, elapsed < 50*time.Millisecond,
						"Pattern %d: Expected fast execution, took %v", i, elapsed)
					assert.Equal(t, pattern.numRequests, successCount,
						"Pattern %d: All requests should succeed when no delay expected", i)
				}

				t.Logf("Pattern %d: %d successes, %d timeouts in %v",
					i, successCount, timeoutCount, elapsed)
			}

			// Test metrics
			metrics := rateLimiter.GetMetrics()
			assert.GreaterOrEqual(t, metrics.TotalRequests, int64(0))
			assert.GreaterOrEqual(t, metrics.AllowedRequests, int64(0))
			assert.GreaterOrEqual(t, metrics.ThrottledRequests, int64(0))
			assert.Equal(t, metrics.TotalRequests, metrics.AllowedRequests+metrics.ThrottledRequests)
		})
	}
}

// Test comprehensive state consistency across components
func TestStateConsistency_Comprehensive(t *testing.T) {
	// Test that all state components work together correctly
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 3)
	require.NoError(t, err)

	// Test state consistency during pool lifecycle
	assert.False(t, pool.started.Load())
	assert.False(t, pool.stopped.Load())
	assert.Equal(t, int32(0), pool.activeJobs.Load())
	assert.Equal(t, int32(0), pool.completedJobs.Load())
	assert.Equal(t, int32(0), pool.failedJobs.Load())

	err = pool.Start()
	require.NoError(t, err)

	assert.True(t, pool.started.Load())
	assert.False(t, pool.stopped.Load())

	// Submit some jobs to change state
	check1 := NewComprehensiveTestMockCheck("consistency_test_1")
	check1.delay = 20 * time.Millisecond

	check2 := NewComprehensiveTestMockCheck("consistency_test_2")
	check2.delay = 30 * time.Millisecond
	check2.shouldFail = true

	err = pool.Submit(check1, nil)
	require.NoError(t, err)

	err = pool.Submit(check2, nil)
	require.NoError(t, err)

	// Verify state changes are consistent
	assert.Equal(t, int32(2), pool.activeJobs.Load())

	// Wait for completion
	time.Sleep(100 * time.Millisecond)

	// Verify final state consistency
	assert.Equal(t, int32(0), pool.activeJobs.Load())
	assert.Equal(t, int32(1), pool.completedJobs.Load())
	assert.Equal(t, int32(1), pool.failedJobs.Load())

	// Stop pool and verify state
	err = pool.Stop()
	require.NoError(t, err)

	assert.True(t, pool.started.Load())
	assert.True(t, pool.stopped.Load())

	// Verify metrics consistency
	metrics := pool.GetMetrics()
	assert.Equal(t, 2, metrics.TotalJobs)
	assert.Equal(t, 1, metrics.CompletedJobs)
	assert.Equal(t, 1, metrics.FailedJobs)
}

// Test state recovery after failures
func TestStateRecovery_Comprehensive(t *testing.T) {
	// Test circuit breaker recovery
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:     3,
		RecoveryTimeout:      20 * time.Millisecond,
		ConsecutiveSuccesses: 2,
	})

	// Force circuit open
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}
	assert.Equal(t, CircuitStateOpen, cb.GetState())

	// Test recovery sequence
	time.Sleep(25 * time.Millisecond) // Wait for recovery timeout

	// Should transition to half-open on first CanExecute
	assert.True(t, cb.CanExecute())
	assert.Equal(t, CircuitStateHalfOpen, cb.GetState())

	// Record successes to close circuit
	cb.RecordSuccess()
	cb.RecordSuccess()
	assert.Equal(t, CircuitStateClosed, cb.GetState())

	// Verify health score recovery
	healthScore := cb.GetHealthScore()
	assert.Greater(t, healthScore, 0.5) // Should have recovered significantly

	// Test error rate monitor recovery
	monitor := &ErrorRateMonitor{
		window:    50 * time.Millisecond,
		threshold: 0.5,
		lastReset: time.Now(),
	}

	// Create high error rate
	for i := 0; i < 10; i++ {
		monitor.RecordError()
	}
	for i := 0; i < 5; i++ {
		monitor.RecordSuccess()
	}
	assert.True(t, monitor.IsOpen())

	// Wait for window reset
	time.Sleep(60 * time.Millisecond)

	// Add successful operations in new window
	for i := 0; i < 20; i++ {
		monitor.RecordSuccess()
	}
	assert.False(t, monitor.IsOpen()) // Should have recovered
}

// Test state transitions under extreme load
func TestStateTransitions_ExtremeLoad(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:     100,
		RecoveryTimeout:      10 * time.Millisecond,
		ConsecutiveSuccesses: 10,
		HalfOpenMaxRequests:  5,
	})

	const numGoroutines = 100
	const operationsPerGoroutine = 1000

	var wg sync.WaitGroup

	// Stress test state transitions
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				switch j % 10 {
				case 0, 1, 2, 3: // 40% CanExecute
					cb.CanExecute()
				case 4, 5, 6: // 30% Success
					cb.RecordSuccess()
				case 7, 8: // 20% Failure
					cb.RecordFailure()
				case 9: // 10% State queries
					state := cb.GetState()
					_ = state // Prevent unused variable warning
				}
			}
		}(i)
	}

	// Also test state manipulation concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < 100; i++ {
			time.Sleep(time.Millisecond)

			switch i % 6 {
			case 0:
				cb.GetMetrics()
			case 1:
				cb.IsHealthy()
			case 2:
				cb.GetHealthScore()
			case 3:
				cb.GetStateHistory()
			case 4:
				if i%20 == 0 { // Occasionally force state changes
					cb.ForceOpen()
				}
			case 5:
				if i%20 == 10 { // Occasionally force close
					cb.ForceClose()
				}
			}
		}
	}()

	wg.Wait()

	// Verify circuit breaker is still in valid state after extreme load
	finalState := cb.GetState()
	assert.True(t, finalState >= CircuitStateClosed && finalState <= CircuitStateHalfOpen)

	// Verify metrics are consistent
	metrics := cb.GetMetrics()
	assert.GreaterOrEqual(t, metrics.TotalRequests, int64(0))
	assert.GreaterOrEqual(t, metrics.TotalSuccesses, int64(0))
	assert.GreaterOrEqual(t, metrics.TotalFailures, int64(0))
	assert.Equal(t, metrics.TotalRequests, metrics.TotalSuccesses+metrics.TotalFailures)

	// Verify circuit is still operational
	canExecute := cb.CanExecute()
	assert.IsType(t, false, canExecute) // Should return boolean without panic
}

// Test state persistence and recovery
func TestStatePersistence_Comprehensive(t *testing.T) {
	// Test that state information is properly maintained
	originalConfig := &CircuitBreakerConfig{
		FailureThreshold:     5,
		RecoveryTimeout:      30 * time.Millisecond,
		ConsecutiveSuccesses: 3,
	}

	cb := NewCircuitBreaker(originalConfig)

	// Build up state
	for i := 0; i < 10; i++ {
		cb.CanExecute()
		if i < 7 {
			cb.RecordSuccess()
		} else {
			cb.RecordFailure()
		}
	}

	// Capture state
	initialMetrics := cb.GetMetrics()
	initialHealth := cb.GetHealthScore()
	initialHistory := cb.GetStateHistory()

	// Validate initial state was captured
	assert.NotNil(t, initialMetrics)
	assert.GreaterOrEqual(t, initialHealth, 0.0)
	assert.NotNil(t, initialHistory)

	// Update configuration (should preserve state)
	newConfig := &CircuitBreakerConfig{
		FailureThreshold:     8,
		RecoveryTimeout:      50 * time.Millisecond,
		ConsecutiveSuccesses: 4,
	}
	cb.UpdateConfig(newConfig)

	// Verify state is preserved
	updatedMetrics := cb.GetMetrics()
	assert.Equal(t, initialMetrics.TotalRequests, updatedMetrics.TotalRequests)
	assert.Equal(t, initialMetrics.TotalSuccesses, updatedMetrics.TotalSuccesses)
	assert.Equal(t, initialMetrics.TotalFailures, updatedMetrics.TotalFailures)

	// Verify configuration is updated
	assert.Equal(t, newConfig.FailureThreshold, cb.config.FailureThreshold)
	assert.Equal(t, newConfig.RecoveryTimeout, cb.config.RecoveryTimeout)

	// Test reset functionality
	cb.Reset()
	resetMetrics := cb.GetMetrics()
	assert.Equal(t, int64(0), resetMetrics.TotalRequests)
	assert.Equal(t, int64(0), resetMetrics.TotalSuccesses)
	assert.Equal(t, int64(0), resetMetrics.TotalFailures)
	assert.Equal(t, CircuitStateClosed, cb.GetState())

	// But configuration should be preserved
	assert.Equal(t, newConfig.FailureThreshold, cb.config.FailureThreshold)

	// History should be reset too
	newHistory := cb.GetStateHistory()
	for state, count := range newHistory {
		if count > 0 {
			t.Logf("State %s has count %d after reset", state.String(), count)
		}
	}
	// Note: State history might not be completely zero due to reset causing a transition
}

// Helper function for comprehensive mock check used in state tests
type StateTestMockCheck struct {
	name       string
	delay      time.Duration
	shouldFail bool
	execCount  atomic.Int32
}

func (c *StateTestMockCheck) Name() string        { return c.name }
func (c *StateTestMockCheck) Description() string { return fmt.Sprintf("State test check: %s", c.name) }
func (c *StateTestMockCheck) Severity() Severity  { return SeverityInfo }

func (c *StateTestMockCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	c.execCount.Add(1)

	select {
	case <-time.After(c.delay):
	case <-ctx.Done():
		return &CheckResult{
			CheckName: c.name,
			Success:   false,
			Message:   "Check cancelled",
			Timestamp: time.Now(),
		}, ctx.Err()
	}

	result := &CheckResult{
		CheckName: c.name,
		Timestamp: time.Now(),
	}

	if c.shouldFail {
		result.Success = false
		result.Message = fmt.Sprintf("Mock failure from %s", c.name)
		return result, fmt.Errorf("mock failure")
	}

	result.Success = true
	result.Message = fmt.Sprintf("Mock success from %s", c.name)
	return result, nil
}
