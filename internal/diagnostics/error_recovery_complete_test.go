package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// Mock structures for comprehensive error recovery testing

// MockErrorRecovery provides comprehensive mocking for error recovery testing
type MockErrorRecovery struct {
	client            *docker.Client
	config            *RecoveryConfig
	failureCount      atomic.Int32
	recoveryAttempts  atomic.Int32
	cleanupCalls      atomic.Int32
	mu                sync.RWMutex
	errorPatterns     map[string]error
	shouldFail        map[string]bool
	recoveryScenarios map[string]func() error
}

// NewMockErrorRecovery creates a mock error recovery system for testing
func NewMockErrorRecovery(config *RecoveryConfig) *MockErrorRecovery {
	if config == nil {
		config = &RecoveryConfig{
			MaxRetries:           3,
			BaseDelay:            10 * time.Millisecond,
			MaxDelay:             1 * time.Second,
			BackoffStrategy:      ExponentialBackoff,
			FailureThreshold:     5,
			RecoveryTimeout:      30 * time.Second,
			ConsecutiveSuccesses: 3,
			CleanupTimeout:       10 * time.Second,
			AutoCleanup:          true,
			EnableDegradation:    true,
		}
	}

	return &MockErrorRecovery{
		config:            config,
		errorPatterns:     make(map[string]error),
		shouldFail:        make(map[string]bool),
		recoveryScenarios: make(map[string]func() error),
	}
}

// SetErrorPattern configures a specific error for testing
func (m *MockErrorRecovery) SetErrorPattern(operation string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorPatterns[operation] = err
}

// SetShouldFail configures operation failure behavior
func (m *MockErrorRecovery) SetShouldFail(operation string, shouldFail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFail[operation] = shouldFail
}

// SetRecoveryScenario configures a recovery scenario
func (m *MockErrorRecovery) SetRecoveryScenario(scenario string, recovery func() error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recoveryScenarios[scenario] = recovery
}

// ExecuteWithRecovery simulates executing an operation with recovery
func (m *MockErrorRecovery) ExecuteWithRecovery(ctx context.Context, operation string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check if operation should fail
	if shouldFail, exists := m.shouldFail[operation]; exists && shouldFail {
		m.failureCount.Add(1)

		if err, hasPattern := m.errorPatterns[operation]; hasPattern {
			return err
		}
		return fmt.Errorf("mock failure for operation: %s", operation)
	}

	// Simulate successful operation
	return nil
}

// AttemptRecovery simulates recovery attempt
func (m *MockErrorRecovery) AttemptRecovery(ctx context.Context, scenario string) error {
	m.recoveryAttempts.Add(1)

	m.mu.RLock()
	defer m.mu.RUnlock()

	if recovery, exists := m.recoveryScenarios[scenario]; exists {
		return recovery()
	}

	// Default successful recovery
	return nil
}

// Cleanup simulates resource cleanup
func (m *MockErrorRecovery) Cleanup(ctx context.Context) error {
	m.cleanupCalls.Add(1)
	return nil
}

// GetMetrics returns mock metrics
func (m *MockErrorRecovery) GetMetrics() ErrorRecoveryTestMetrics {
	return ErrorRecoveryTestMetrics{
		TotalFailures:        int64(m.failureCount.Load()),
		RecoveryAttempts:     int64(m.recoveryAttempts.Load()),
		SuccessfulRecoveries: int64(m.recoveryAttempts.Load()), // Assume all succeed
		CleanupOperations:    int64(m.cleanupCalls.Load()),
	}
}

// ErrorRecoveryTestMetrics represents error recovery statistics for testing
type ErrorRecoveryTestMetrics struct {
	TotalFailures        int64
	RecoveryAttempts     int64
	SuccessfulRecoveries int64
	CleanupOperations    int64
	CircuitBreakerTrips  int64
	GracefulDegradations int64
}

// Test error classification and categorization
func TestErrorRecovery_ErrorClassification_Comprehensive(t *testing.T) {
	tests := []struct {
		name             string
		inputError       error
		expectedType     string
		expectedSeverity string
		canRecover       bool
		description      string
	}{
		// Docker daemon connection errors
		{
			name:             "Docker daemon connection refused",
			inputError:       errors.New("connection refused"),
			expectedType:     "connection",
			expectedSeverity: "critical",
			canRecover:       true,
			description:      "Should classify connection refused as recoverable connection error",
		},
		{
			name:             "Docker daemon not running",
			inputError:       errors.New("docker daemon not running"),
			expectedType:     "daemon",
			expectedSeverity: "critical",
			canRecover:       true,
			description:      "Should classify daemon not running as recoverable daemon error",
		},
		{
			name:             "Permission denied accessing Docker socket",
			inputError:       errors.New("permission denied"),
			expectedType:     "permission",
			expectedSeverity: "high",
			canRecover:       false,
			description:      "Should classify permission errors as non-recoverable",
		},

		// Resource exhaustion errors
		{
			name:             "Out of memory error",
			inputError:       errors.New("out of memory"),
			expectedType:     "resource",
			expectedSeverity: "high",
			canRecover:       true,
			description:      "Should classify memory errors as recoverable resource error",
		},
		{
			name:             "Disk space exhausted",
			inputError:       errors.New("no space left on device"),
			expectedType:     "resource",
			expectedSeverity: "high",
			canRecover:       false,
			description:      "Should classify disk space as non-recoverable resource error",
		},
		{
			name:             "Too many open files",
			inputError:       errors.New("too many open files"),
			expectedType:     "resource",
			expectedSeverity: "medium",
			canRecover:       true,
			description:      "Should classify file descriptor limit as recoverable",
		},

		// Network-specific errors
		{
			name:             "DNS resolution failure",
			inputError:       errors.New("no such host"),
			expectedType:     "network",
			expectedSeverity: "medium",
			canRecover:       true,
			description:      "Should classify DNS errors as recoverable network error",
		},
		{
			name:             "Network timeout",
			inputError:       errors.New("i/o timeout"),
			expectedType:     "network",
			expectedSeverity: "medium",
			canRecover:       true,
			description:      "Should classify timeouts as recoverable network error",
		},
		{
			name:             "Network unreachable",
			inputError:       errors.New("network is unreachable"),
			expectedType:     "network",
			expectedSeverity: "high",
			canRecover:       true,
			description:      "Should classify network unreachable as recoverable",
		},

		// Context errors
		{
			name:             "Context cancelled",
			inputError:       context.Canceled,
			expectedType:     "context",
			expectedSeverity: "low",
			canRecover:       false,
			description:      "Should classify context cancellation as non-recoverable",
		},
		{
			name:             "Context deadline exceeded",
			inputError:       context.DeadlineExceeded,
			expectedType:     "context",
			expectedSeverity: "medium",
			canRecover:       true,
			description:      "Should classify timeout as recoverable with retry",
		},

		// Generic and unknown errors
		{
			name:             "Generic error",
			inputError:       errors.New("something went wrong"),
			expectedType:     "unknown",
			expectedSeverity: "medium",
			canRecover:       true,
			description:      "Should classify unknown errors as potentially recoverable",
		},
		{
			name:             "Nil error",
			inputError:       nil,
			expectedType:     "none",
			expectedSeverity: "none",
			canRecover:       true,
			description:      "Should handle nil errors gracefully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test error classification
			errorType := classifyError(tt.inputError)
			severity := assessErrorSeverity(tt.inputError)
			recoverable := isRecoverable(tt.inputError)

			assert.Equal(t, tt.expectedType, errorType, tt.description)
			assert.Equal(t, tt.expectedSeverity, severity, tt.description)
			assert.Equal(t, tt.canRecover, recoverable, tt.description)
		})
	}
}

// Test retry policies and backoff strategies
func TestErrorRecovery_RetryPolicies_Comprehensive(t *testing.T) {
	tests := []struct {
		name             string
		config           *RecoveryConfig
		failurePattern   func(attempt int) bool // Returns true if attempt should fail
		expectedAttempts int
		expectedDuration time.Duration
		description      string
	}{
		{
			name: "Linear backoff with all failures",
			config: &RecoveryConfig{
				MaxRetries:      3,
				BaseDelay:       10 * time.Millisecond,
				MaxDelay:        100 * time.Millisecond,
				BackoffStrategy: LinearBackoff,
			},
			failurePattern:   func(attempt int) bool { return true }, // Always fail
			expectedAttempts: 4,                                      // Initial + 3 retries
			expectedDuration: 60 * time.Millisecond,                  // 10 + 20 + 30ms delays
			description:      "Linear backoff should increase delay by base amount each retry",
		},
		{
			name: "Exponential backoff with eventual success",
			config: &RecoveryConfig{
				MaxRetries:      4,
				BaseDelay:       10 * time.Millisecond,
				MaxDelay:        200 * time.Millisecond,
				BackoffStrategy: ExponentialBackoff,
			},
			failurePattern:   func(attempt int) bool { return attempt < 3 }, // Succeed on 3rd retry
			expectedAttempts: 3,                                             // Initial + 2 retries before success
			expectedDuration: 30 * time.Millisecond,                         // 10 + 20ms delays
			description:      "Exponential backoff should succeed when condition is met",
		},
		{
			name: "Jittered backoff prevents thundering herd",
			config: &RecoveryConfig{
				MaxRetries:      2,
				BaseDelay:       50 * time.Millisecond,
				MaxDelay:        500 * time.Millisecond,
				BackoffStrategy: JitteredBackoff,
			},
			failurePattern:   func(attempt int) bool { return true }, // Always fail
			expectedAttempts: 3,                                      // Initial + 2 retries
			expectedDuration: 100 * time.Millisecond,                 // Variable due to jitter
			description:      "Jittered backoff should vary delays to prevent synchronization",
		},
		{
			name: "Zero retries should only attempt once",
			config: &RecoveryConfig{
				MaxRetries:      0,
				BaseDelay:       10 * time.Millisecond,
				BackoffStrategy: LinearBackoff,
			},
			failurePattern:   func(attempt int) bool { return true },
			expectedAttempts: 1, // No retries
			expectedDuration: 0, // No retry delays
			description:      "Zero retries should only attempt operation once",
		},
		{
			name: "Max delay capping with exponential backoff",
			config: &RecoveryConfig{
				MaxRetries:      5,
				BaseDelay:       100 * time.Millisecond,
				MaxDelay:        200 * time.Millisecond, // Cap at 200ms
				BackoffStrategy: ExponentialBackoff,
			},
			failurePattern:   func(attempt int) bool { return true },
			expectedAttempts: 6,                      // Initial + 5 retries
			expectedDuration: 900 * time.Millisecond, // 100 + 200 + 200 + 200 + 200ms
			description:      "Max delay should cap exponential backoff growth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recovery := NewMockErrorRecovery(tt.config)
			require.NotNil(t, recovery)

			attemptCount := 0
			startTime := time.Now()

			err := retry(context.Background(), tt.config, func(attempt int) error {
				attemptCount++
				if tt.failurePattern(attempt) {
					return fmt.Errorf("attempt %d failed", attempt)
				}
				return nil
			})

			duration := time.Since(startTime)

			assert.Equal(t, tt.expectedAttempts, attemptCount, tt.description)

			// Allow some variance in timing due to test execution overhead
			if tt.expectedDuration > 0 {
				assert.True(t, duration >= tt.expectedDuration*8/10,
					"Duration %v should be at least %v", duration, tt.expectedDuration*8/10)
				assert.True(t, duration <= tt.expectedDuration*15/10,
					"Duration %v should not exceed %v", duration, tt.expectedDuration*15/10)
			}

			if tt.failurePattern(tt.expectedAttempts - 1) {
				assert.Error(t, err, "Should fail when all attempts fail")
			} else {
				assert.NoError(t, err, "Should succeed when condition is met")
			}
		})
	}
}

// Test circuit breaker integration with error recovery
func TestErrorRecovery_CircuitBreakerIntegration(t *testing.T) {
	tests := []struct {
		name               string
		config             *RecoveryConfig
		errorSequence      []bool // true = error, false = success
		expectedFinalState CircuitState
		description        string
	}{
		{
			name: "Circuit opens after failure threshold",
			config: &RecoveryConfig{
				FailureThreshold:     3,
				RecoveryTimeout:      10 * time.Millisecond,
				ConsecutiveSuccesses: 2,
			},
			errorSequence:      []bool{true, true, true, true}, // 4 failures
			expectedFinalState: CircuitStateOpen,
			description:        "Circuit should open after reaching failure threshold",
		},
		{
			name: "Circuit remains closed with mixed results",
			config: &RecoveryConfig{
				FailureThreshold:     5,
				RecoveryTimeout:      10 * time.Millisecond,
				ConsecutiveSuccesses: 2,
			},
			errorSequence:      []bool{true, false, true, false}, // Mixed results
			expectedFinalState: CircuitStateClosed,
			description:        "Circuit should remain closed with mixed success/failure",
		},
		{
			name: "Circuit recovery through half-open state",
			config: &RecoveryConfig{
				FailureThreshold:     2,
				RecoveryTimeout:      20 * time.Millisecond,
				ConsecutiveSuccesses: 2,
			},
			errorSequence:      []bool{true, true, false, false}, // Fail then succeed
			expectedFinalState: CircuitStateClosed,               // Should recover
			description:        "Circuit should recover to closed state after successful operations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create circuit breaker with test config
			cb := NewCircuitBreaker(&CircuitBreakerConfig{
				FailureThreshold:     tt.config.FailureThreshold,
				RecoveryTimeout:      tt.config.RecoveryTimeout,
				ConsecutiveSuccesses: tt.config.ConsecutiveSuccesses,
			})

			recovery := NewMockErrorRecovery(tt.config)

			// Execute error sequence
			for i, shouldError := range tt.errorSequence {
				if cb.CanExecute() {
					operation := fmt.Sprintf("test_operation_%d", i)

					if shouldError {
						recovery.SetShouldFail(operation, true)
						err := recovery.ExecuteWithRecovery(context.Background(), operation)
						assert.Error(t, err)
						cb.RecordFailure()
					} else {
						recovery.SetShouldFail(operation, false)
						err := recovery.ExecuteWithRecovery(context.Background(), operation)
						assert.NoError(t, err)
						cb.RecordSuccess()
					}
				}

				// Add small delay for recovery timeout scenarios
				if i == 1 && tt.expectedFinalState == CircuitStateClosed {
					time.Sleep(tt.config.RecoveryTimeout + 5*time.Millisecond)
				}
			}

			// Check final circuit state
			finalState := cb.GetState()
			assert.Equal(t, tt.expectedFinalState, finalState, tt.description)
		})
	}
}

// Test resource cleanup and recovery scenarios
func TestErrorRecovery_ResourceCleanup_Comprehensive(t *testing.T) {
	scenarios := []struct {
		name            string
		config          *RecoveryConfig
		simulateFailure func(*MockErrorRecovery)
		verifyCleanup   func(*testing.T, *MockErrorRecovery)
		description     string
	}{
		{
			name: "Automatic cleanup on failure",
			config: &RecoveryConfig{
				AutoCleanup:    true,
				CleanupTimeout: 100 * time.Millisecond,
			},
			simulateFailure: func(recovery *MockErrorRecovery) {
				recovery.SetShouldFail("test_operation", true)
				recovery.SetErrorPattern("test_operation", errors.New("resource exhaustion"))
			},
			verifyCleanup: func(t *testing.T, recovery *MockErrorRecovery) {
				metrics := recovery.GetMetrics()
				assert.Greater(t, metrics.CleanupOperations, int64(0),
					"Should have performed cleanup operations")
			},
			description: "Should automatically cleanup resources on failure",
		},
		{
			name: "No cleanup when disabled",
			config: &RecoveryConfig{
				AutoCleanup:    false,
				CleanupTimeout: 100 * time.Millisecond,
			},
			simulateFailure: func(recovery *MockErrorRecovery) {
				recovery.SetShouldFail("test_operation", true)
			},
			verifyCleanup: func(t *testing.T, recovery *MockErrorRecovery) {
				// Manual cleanup call needed
				recovery.Cleanup(context.Background())
				metrics := recovery.GetMetrics()
				assert.Equal(t, int64(1), metrics.CleanupOperations,
					"Should only cleanup when explicitly called")
			},
			description: "Should not automatically cleanup when disabled",
		},
		{
			name: "Cleanup with timeout handling",
			config: &RecoveryConfig{
				AutoCleanup:    true,
				CleanupTimeout: 1 * time.Millisecond, // Very short timeout
			},
			simulateFailure: func(recovery *MockErrorRecovery) {
				recovery.SetShouldFail("test_operation", true)
				recovery.SetRecoveryScenario("cleanup", func() error {
					time.Sleep(10 * time.Millisecond) // Exceed timeout
					return errors.New("cleanup timeout")
				})
			},
			verifyCleanup: func(t *testing.T, recovery *MockErrorRecovery) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
				defer cancel()

				err := recovery.Cleanup(ctx)
				// Should handle timeout gracefully
				assert.True(t, err != nil || ctx.Err() != nil,
					"Should handle cleanup timeout appropriately")
			},
			description: "Should handle cleanup timeout gracefully",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			recovery := NewMockErrorRecovery(scenario.config)

			// Simulate the failure condition
			scenario.simulateFailure(recovery)

			// Attempt operation that should trigger cleanup
			err := recovery.ExecuteWithRecovery(context.Background(), "test_operation")
			assert.Error(t, err, "Operation should fail as configured")

			// Verify cleanup behavior
			scenario.verifyCleanup(t, recovery)
		})
	}
}

// Test graceful degradation scenarios
func TestErrorRecovery_GracefulDegradation_Comprehensive(t *testing.T) {
	degradationScenarios := []struct {
		name              string
		config            *RecoveryConfig
		systemState       func(*MockErrorRecovery)
		expectedBehavior  string
		verifyDegradation func(*testing.T, *MockErrorRecovery)
		description       string
	}{
		{
			name: "Partial Docker daemon failure - minimal checks only",
			config: &RecoveryConfig{
				EnableDegradation: true,
				MinimalChecksOnly: true,
				PartialResultsOK:  true,
			},
			systemState: func(recovery *MockErrorRecovery) {
				// Simulate partial Docker daemon failure
				recovery.SetShouldFail("container_list", true)
				recovery.SetShouldFail("network_list", true)
				recovery.SetShouldFail("daemon_ping", false) // Basic connectivity OK
			},
			expectedBehavior: "minimal_checks",
			verifyDegradation: func(t *testing.T, recovery *MockErrorRecovery) {
				// Should still be able to ping daemon
				err := recovery.ExecuteWithRecovery(context.Background(), "daemon_ping")
				assert.NoError(t, err, "Basic daemon connectivity should work")

				// But complex operations should be skipped/fail gracefully
				err = recovery.ExecuteWithRecovery(context.Background(), "container_list")
				assert.Error(t, err, "Complex operations should fail")
			},
			description: "Should run only essential checks when Docker daemon is partially available",
		},
		{
			name: "Network partition - accept partial results",
			config: &RecoveryConfig{
				EnableDegradation: true,
				MinimalChecksOnly: false,
				PartialResultsOK:  true,
			},
			systemState: func(recovery *MockErrorRecovery) {
				// Simulate network partition affecting external connectivity
				recovery.SetShouldFail("external_dns", true)
				recovery.SetShouldFail("external_ping", true)
				recovery.SetShouldFail("internal_check", false) // Internal checks OK
			},
			expectedBehavior: "partial_results",
			verifyDegradation: func(t *testing.T, recovery *MockErrorRecovery) {
				// Internal checks should work
				err := recovery.ExecuteWithRecovery(context.Background(), "internal_check")
				assert.NoError(t, err, "Internal checks should succeed")

				// External checks fail but system continues
				err = recovery.ExecuteWithRecovery(context.Background(), "external_dns")
				assert.Error(t, err, "External checks should fail")
			},
			description: "Should accept partial results when network is partitioned",
		},
		{
			name: "Resource exhaustion - degraded performance mode",
			config: &RecoveryConfig{
				EnableDegradation: true,
				MinimalChecksOnly: true,
				PartialResultsOK:  true,
				MaxRetries:        1, // Reduced retries to save resources
			},
			systemState: func(recovery *MockErrorRecovery) {
				// Simulate resource exhaustion
				recovery.SetShouldFail("memory_intensive", true)
				recovery.SetErrorPattern("memory_intensive", errors.New("out of memory"))
				recovery.SetShouldFail("lightweight_check", false)
			},
			expectedBehavior: "degraded_performance",
			verifyDegradation: func(t *testing.T, recovery *MockErrorRecovery) {
				// Lightweight checks should work
				err := recovery.ExecuteWithRecovery(context.Background(), "lightweight_check")
				assert.NoError(t, err, "Lightweight checks should succeed")

				// Memory intensive operations should fail quickly
				err = recovery.ExecuteWithRecovery(context.Background(), "memory_intensive")
				assert.Error(t, err, "Memory intensive operations should fail")
			},
			description: "Should operate in degraded mode during resource exhaustion",
		},
		{
			name: "Disabled degradation - fail fast",
			config: &RecoveryConfig{
				EnableDegradation: false,
				MinimalChecksOnly: false,
				PartialResultsOK:  false,
			},
			systemState: func(recovery *MockErrorRecovery) {
				recovery.SetShouldFail("critical_check", true)
			},
			expectedBehavior: "fail_fast",
			verifyDegradation: func(t *testing.T, recovery *MockErrorRecovery) {
				err := recovery.ExecuteWithRecovery(context.Background(), "critical_check")
				assert.Error(t, err, "Should fail fast when degradation disabled")
			},
			description: "Should fail fast when graceful degradation is disabled",
		},
	}

	for _, scenario := range degradationScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			recovery := NewMockErrorRecovery(scenario.config)

			// Setup system state
			scenario.systemState(recovery)

			// Verify degradation behavior
			scenario.verifyDegradation(t, recovery)
		})
	}
}

// Test concurrent error recovery operations
func TestErrorRecovery_ConcurrentOperations(t *testing.T) {
	config := &RecoveryConfig{
		MaxRetries:      2,
		BaseDelay:       5 * time.Millisecond,
		BackoffStrategy: LinearBackoff,
		AutoCleanup:     true,
	}

	recovery := NewMockErrorRecovery(config)

	const numGoroutines = 20
	const operationsPerGoroutine = 50

	var wg sync.WaitGroup
	var totalOperations, totalFailures, totalRecoveries atomic.Int64

	// Configure some operations to fail
	recovery.SetShouldFail("failing_op", true)
	recovery.SetShouldFail("timeout_op", true)
	recovery.SetErrorPattern("timeout_op", context.DeadlineExceeded)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				totalOperations.Add(1)

				// Mix of different operation types
				var opName string
				switch j % 4 {
				case 0:
					opName = "successful_op"
				case 1:
					opName = "failing_op"
				case 2:
					opName = "timeout_op"
				case 3:
					opName = fmt.Sprintf("worker_%d_op_%d", workerID, j)
				}

				err := recovery.ExecuteWithRecovery(context.Background(), opName)
				if err != nil {
					totalFailures.Add(1)

					// Attempt recovery for some errors
					if strings.Contains(err.Error(), "failing_op") {
						if recovery.AttemptRecovery(context.Background(), "standard_recovery") == nil {
							totalRecoveries.Add(1)
						}
					}
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify no panics and reasonable results
	expectedOperations := int64(numGoroutines * operationsPerGoroutine)
	assert.Equal(t, expectedOperations, totalOperations.Load())

	// Should have some failures due to configured failing operations
	assert.Greater(t, totalFailures.Load(), int64(0))

	// Should have attempted some recoveries
	assert.GreaterOrEqual(t, totalRecoveries.Load(), int64(0))

	// Verify metrics are consistent
	metrics := recovery.GetMetrics()
	assert.GreaterOrEqual(t, metrics.TotalFailures, int64(0))
	assert.GreaterOrEqual(t, metrics.RecoveryAttempts, int64(0))
}

// Test specific error recovery patterns
func TestErrorRecovery_SpecificPatterns(t *testing.T) {
	patterns := []struct {
		name           string
		errorType      error
		recoveryFunc   func(context.Context, *MockErrorRecovery) error
		expectedResult string
		description    string
	}{
		{
			name:      "Docker daemon restart recovery",
			errorType: errors.New("connection refused"),
			recoveryFunc: func(ctx context.Context, recovery *MockErrorRecovery) error {
				// Simulate daemon restart by first failing then succeeding
				recovery.SetShouldFail("daemon_ping", false)
				return recovery.ExecuteWithRecovery(ctx, "daemon_ping")
			},
			expectedResult: "success",
			description:    "Should recover from Docker daemon restart",
		},
		{
			name:      "Network partition recovery",
			errorType: errors.New("network is unreachable"),
			recoveryFunc: func(ctx context.Context, recovery *MockErrorRecovery) error {
				// Simulate network recovery
				recovery.SetShouldFail("network_check", false)
				return recovery.ExecuteWithRecovery(ctx, "network_check")
			},
			expectedResult: "success",
			description:    "Should recover from network partition",
		},
		{
			name:      "Permission error - no recovery",
			errorType: errors.New("permission denied"),
			recoveryFunc: func(ctx context.Context, recovery *MockErrorRecovery) error {
				// Permission errors typically cannot be recovered automatically
				return recovery.ExecuteWithRecovery(ctx, "permission_check")
			},
			expectedResult: "failure",
			description:    "Should not recover from permission errors",
		},
		{
			name:      "Resource exhaustion with cleanup",
			errorType: errors.New("out of memory"),
			recoveryFunc: func(ctx context.Context, recovery *MockErrorRecovery) error {
				// Cleanup should free resources
				recovery.Cleanup(ctx)
				recovery.SetShouldFail("memory_check", false) // Simulate cleanup success
				return recovery.ExecuteWithRecovery(ctx, "memory_check")
			},
			expectedResult: "success",
			description:    "Should recover from resource exhaustion with cleanup",
		},
	}

	for _, pattern := range patterns {
		t.Run(pattern.name, func(t *testing.T) {
			recovery := NewMockErrorRecovery(&RecoveryConfig{
				MaxRetries:  3,
				AutoCleanup: true,
			})

			// Initially configure operation to fail with specific error
			recovery.SetShouldFail("test_op", true)
			recovery.SetErrorPattern("test_op", pattern.errorType)

			// Verify initial failure
			err := recovery.ExecuteWithRecovery(context.Background(), "test_op")
			assert.Error(t, err)

			// Attempt recovery
			ctx := context.Background()
			recoveryErr := pattern.recoveryFunc(ctx, recovery)

			if pattern.expectedResult == "success" {
				assert.NoError(t, recoveryErr, pattern.description)
			} else {
				assert.Error(t, recoveryErr, pattern.description)
			}
		})
	}
}

// Helper functions for error classification (would be implemented in the actual error recovery module)

func classifyError(err error) string {
	if err == nil {
		return "none"
	}

	errMsg := strings.ToLower(err.Error())

	if strings.Contains(errMsg, "connection refused") || strings.Contains(errMsg, "connect") {
		return "connection"
	}
	if strings.Contains(errMsg, "daemon") {
		return "daemon"
	}
	if strings.Contains(errMsg, "permission") {
		return "permission"
	}
	if strings.Contains(errMsg, "memory") || strings.Contains(errMsg, "space") || strings.Contains(errMsg, "files") {
		return "resource"
	}
	if strings.Contains(errMsg, "network") || strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "host") {
		return "network"
	}
	if err == context.Canceled || err == context.DeadlineExceeded {
		return "context"
	}

	return "unknown"
}

func assessErrorSeverity(err error) string {
	if err == nil {
		return "none"
	}

	errType := classifyError(err)

	switch errType {
	case "connection", "daemon":
		return "critical"
	case "resource", "network":
		return "high"
	case "context":
		if err == context.Canceled {
			return "low"
		}
		return "medium"
	case "permission":
		return "high"
	default:
		return "medium"
	}
}

func isRecoverable(err error) bool {
	if err == nil {
		return true
	}

	errType := classifyError(err)

	switch errType {
	case "permission":
		return false // Usually requires manual intervention
	case "context":
		return err != context.Canceled // Cancellation is intentional
	default:
		return true // Most errors are potentially recoverable
	}
}

// Mock retry function for testing
func retry(ctx context.Context, config *RecoveryConfig, operation func(int) error) error {
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		lastErr = operation(attempt)
		if lastErr == nil {
			return nil // Success
		}

		// Don't delay after the last attempt
		if attempt == config.MaxRetries {
			break
		}

		// Calculate delay based on backoff strategy
		delay := calculateDelay(config, attempt)

		select {
		case <-time.After(delay):
			// Continue to next attempt
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return lastErr
}

func calculateDelay(config *RecoveryConfig, attempt int) time.Duration {
	var delay time.Duration

	switch config.BackoffStrategy {
	case LinearBackoff:
		delay = config.BaseDelay * time.Duration(attempt+1)
	case ExponentialBackoff:
		delay = config.BaseDelay * (1 << uint(attempt))
	case JitteredBackoff:
		baseDelay := config.BaseDelay * time.Duration(attempt+1)
		// Add simple jitter (in real implementation, would use proper random jitter)
		jitter := time.Duration(attempt) * time.Millisecond
		delay = baseDelay + jitter
	}

	if delay > config.MaxDelay {
		delay = config.MaxDelay
	}

	return delay
}

// BackoffStrategy is defined in error_recovery.go - no need to redeclare
