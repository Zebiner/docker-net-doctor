package diagnostics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRetryPolicy(t *testing.T) {
	tests := []struct {
		name     string
		config   *RetryPolicyConfig
		expected *RetryPolicyConfig
	}{
		{
			name:   "nil config uses defaults",
			config: nil,
			expected: &RetryPolicyConfig{
				MaxRetries:        3,
				BaseDelay:         100 * time.Millisecond,
				MaxDelay:          10 * time.Second,
				BackoffStrategy:   ExponentialBackoff,
				BackoffMultiplier: 2.0,
				JitterFactor:      0.1,
				MaxRetryBudget:    30 * time.Second,
				RespectContext:    true,
			},
		},
		{
			name: "custom config preserved",
			config: &RetryPolicyConfig{
				MaxRetries:      5,
				BaseDelay:       200 * time.Millisecond,
				BackoffStrategy: LinearBackoff,
			},
			expected: &RetryPolicyConfig{
				MaxRetries:        5,
				BaseDelay:         200 * time.Millisecond,
				BackoffStrategy:   LinearBackoff,
				BackoffMultiplier: 2.0, // Should be set to default
				JitterFactor:      0.1, // Should be set to default
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := NewRetryPolicy(tt.config)
			
			require.NotNil(t, policy)
			require.NotNil(t, policy.config)
			require.NotNil(t, policy.metrics)
			require.NotNil(t, policy.rand)
			
			assert.Equal(t, tt.expected.MaxRetries, policy.config.MaxRetries)
			assert.Equal(t, tt.expected.BaseDelay, policy.config.BaseDelay)
			assert.Equal(t, tt.expected.BackoffStrategy, policy.config.BackoffStrategy)
			assert.Equal(t, tt.expected.BackoffMultiplier, policy.config.BackoffMultiplier)
			assert.Equal(t, tt.expected.JitterFactor, policy.config.JitterFactor)
		})
	}
}

func TestExecuteWithRetry(t *testing.T) {
	tests := []struct {
		name           string
		operation      RecoverableOperation
		maxRetries     int
		expectedResult interface{}
		expectedError  bool
		expectedRetries int64
	}{
		{
			name: "successful on first attempt",
			operation: func(ctx interface{}) (interface{}, error) {
				return "success", nil
			},
			maxRetries:      3,
			expectedResult:  "success",
			expectedError:   false,
			expectedRetries: 0,
		},
		{
			name: "successful after retries",
			operation: func() RecoverableOperation {
				attempts := 0
				return func(ctx interface{}) (interface{}, error) {
					attempts++
					if attempts < 3 {
						return nil, &DiagnosticError{
							Code:    ErrCodeTimeout,
							Type:    ErrTypeContext,
							Message: "timeout error",
						}
					}
					return "success after retries", nil
				}
			}(),
			maxRetries:      3,
			expectedResult:  "success after retries", 
			expectedError:   false,
			expectedRetries: 2, // 2 retries after initial failure
		},
		{
			name: "exhausts all retries",
			operation: func(ctx interface{}) (interface{}, error) {
				return nil, &DiagnosticError{
					Code:    ErrCodeTimeout,
					Type:    ErrTypeContext,
					Message: "persistent timeout",
				}
			},
			maxRetries:      2,
			expectedResult:  nil,
			expectedError:   true,
			expectedRetries: 2,
		},
		{
			name: "non-retryable error",
			operation: func(ctx interface{}) (interface{}, error) {
				return nil, &DiagnosticError{
					Code:     ErrCodeDockerPermissionDenied,
					Type:     ErrTypeConnection,
					Message:  "permission denied",
					Severity: SeverityError,
				}
			},
			maxRetries:      3,
			expectedResult:  nil,
			expectedError:   true,
			expectedRetries: 0, // Should not retry
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &RetryPolicyConfig{
				MaxRetries:     tt.maxRetries,
				BaseDelay:      1 * time.Millisecond, // Fast for testing
				MaxDelay:       10 * time.Millisecond,
				RespectContext: true,
			}
			
			policy := NewRetryPolicy(config)
			ctx := context.Background()

			result, err := policy.Execute(ctx, tt.operation)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}

			// Check retry metrics
			metrics := policy.GetMetrics()
			assert.Equal(t, tt.expectedRetries, metrics.TotalRetries)
			
			if tt.expectedError && tt.expectedRetries > 0 {
				assert.Equal(t, int64(1), metrics.FailedRetries)
			} else if !tt.expectedError && tt.expectedRetries > 0 {
				assert.Equal(t, int64(1), metrics.SuccessfulRetries)
			}
		})
	}
}

func TestBackoffStrategies(t *testing.T) {
	tests := []struct {
		name     string
		strategy BackoffStrategy
		attempt  int
		baseDelay time.Duration
		maxDelay  time.Duration
		multiplier float64
		expected  time.Duration
	}{
		{
			name:      "linear backoff",
			strategy:  LinearBackoff,
			attempt:   3,
			baseDelay: 100 * time.Millisecond,
			maxDelay:  1 * time.Second,
			expected:  300 * time.Millisecond, // 100ms * 3
		},
		{
			name:       "exponential backoff",
			strategy:   ExponentialBackoff,
			attempt:    3,
			baseDelay:  100 * time.Millisecond,
			maxDelay:   1 * time.Second,
			multiplier: 2.0,
			expected:   400 * time.Millisecond, // 100ms * 2^(3-1) = 100ms * 4
		},
		{
			name:      "max delay limit",
			strategy:  LinearBackoff,
			attempt:   20,
			baseDelay: 100 * time.Millisecond,
			maxDelay:  500 * time.Millisecond,
			expected:  500 * time.Millisecond, // Capped at maxDelay
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &RetryPolicyConfig{
				BaseDelay:         tt.baseDelay,
				MaxDelay:          tt.maxDelay,
				BackoffStrategy:   tt.strategy,
				BackoffMultiplier: tt.multiplier,
				JitterFactor:      0, // Disable jitter for predictable results
			}
			
			policy := NewRetryPolicy(config)
			delay := policy.calculateBackoffDelay(tt.attempt)
			
			if tt.strategy == JitteredBackoff {
				// For jittered backoff, just check it's in reasonable range
				minExpected := time.Duration(float64(tt.expected) * 0.8)
				maxExpected := time.Duration(float64(tt.expected) * 1.2)
				assert.True(t, delay >= minExpected && delay <= maxExpected,
					"delay %v not in range [%v, %v]", delay, minExpected, maxExpected)
			} else {
				assert.Equal(t, tt.expected, delay)
			}
		})
	}
}

func TestJitteredBackoff(t *testing.T) {
	config := &RetryPolicyConfig{
		BaseDelay:         100 * time.Millisecond,
		MaxDelay:          1 * time.Second,
		BackoffStrategy:   JitteredBackoff,
		BackoffMultiplier: 2.0,
		JitterFactor:      0.2, // 20% jitter
	}
	
	policy := NewRetryPolicy(config)
	
	// Test multiple calculations to ensure jitter is applied
	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delays[i] = policy.calculateBackoffDelay(2) // Same attempt, different jitter
	}
	
	// Check that we get different values (due to jitter)
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}
	assert.False(t, allSame, "jitter should produce different delay values")
	
	// All delays should be positive and within reasonable bounds
	baseExpected := 200 * time.Millisecond // 100ms * 2^(2-1)
	for _, delay := range delays {
		assert.True(t, delay > 0, "delay should be positive")
		assert.True(t, delay <= config.MaxDelay, "delay should not exceed max")
		
		// Should be within jitter range of base expected
		minBound := time.Duration(float64(baseExpected) * 0.6) // Conservative bound
		maxBound := time.Duration(float64(baseExpected) * 1.4)
		assert.True(t, delay >= minBound && delay <= maxBound,
			"delay %v should be within bounds [%v, %v]", delay, minBound, maxBound)
	}
}

func TestRetryableErrorDetection(t *testing.T) {
	policy := NewRetryPolicy(&RetryPolicyConfig{})

	tests := []struct {
		name        string
		error       error
		shouldRetry bool
	}{
		{
			name:        "nil error",
			error:       nil,
			shouldRetry: false,
		},
		{
			name: "retryable connection error",
			error: &DiagnosticError{
				Code:    ErrCodeDockerDaemonUnreachable,
				Type:    ErrTypeConnection,
				Message: "daemon unreachable",
			},
			shouldRetry: true,
		},
		{
			name: "non-retryable permission error",
			error: &DiagnosticError{
				Code:    ErrCodeDockerPermissionDenied,
				Type:    ErrTypeConnection,
				Message: "permission denied",
			},
			shouldRetry: false,
		},
		{
			name: "retryable timeout error",
			error: &DiagnosticError{
				Code:    ErrCodeTimeout,
				Type:    ErrTypeContext,
				Message: "timeout",
			},
			shouldRetry: false, // Context errors are not retryable
		},
		{
			name: "retryable network error",
			error: &DiagnosticError{
				Code:    ErrCodeDNSResolutionFailed,
				Type:    ErrTypeNetwork,
				Message: "DNS failed",
			},
			shouldRetry: true,
		},
		{
			name: "non-retryable network config error",
			error: &DiagnosticError{
				Code:    ErrCodeNetworkNotFound,
				Type:    ErrTypeNetwork,
				Message: "network not found",
			},
			shouldRetry: false,
		},
		{
			name: "retryable resource error",
			error: &DiagnosticError{
				Code:    ErrCodeResourceConflict,
				Type:    ErrTypeResource,
				Message: "resource conflict",
			},
			shouldRetry: true,
		},
		{
			name: "non-retryable resource exhaustion",
			error: &DiagnosticError{
				Code:    ErrCodeOutOfMemory,
				Type:    ErrTypeResource,
				Message: "out of memory",
			},
			shouldRetry: false,
		},
		{
			name:        "generic retryable error",
			error:       errors.New("connection refused"),
			shouldRetry: true,
		},
		{
			name:        "generic non-retryable error",
			error:       errors.New("permission denied"),
			shouldRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := policy.isRetryable(tt.error)
			assert.Equal(t, tt.shouldRetry, result)
		})
	}
}

func TestRetryPolicyContextCancellation(t *testing.T) {
	config := &RetryPolicyConfig{
		MaxRetries:     3,
		BaseDelay:      100 * time.Millisecond,
		RespectContext: true,
	}
	
	policy := NewRetryPolicy(config)

	// Create context that will be cancelled quickly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Operation that would normally retry
	operation := func(ctx interface{}) (interface{}, error) {
		return nil, &DiagnosticError{
			Code: ErrCodeTimeout,
			Type: ErrTypeContext,
			Message: "timeout",
		}
	}

	_, err := policy.Execute(ctx, operation)
	
	// Should fail due to context cancellation, not retry exhaustion
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
}

func TestRetryBudget(t *testing.T) {
	config := &RetryPolicyConfig{
		MaxRetries:     10,         // High retry count
		BaseDelay:      50 * time.Millisecond,
		MaxRetryBudget: 100 * time.Millisecond, // But limited total time
		RespectContext: true,
	}
	
	policy := NewRetryPolicy(config)
	ctx := context.Background()

	// Operation that always fails
	operation := func(ctx interface{}) (interface{}, error) {
		return nil, &DiagnosticError{
			Code: ErrCodeTimeout,
			Type: ErrTypeContext,
			Message: "timeout",
		}
	}

	start := time.Now()
	_, err := policy.Execute(ctx, operation)
	duration := time.Since(start)

	// Should fail due to retry budget, not max retries
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "retry budget exhausted")
	
	// Should not have exceeded the budget significantly
	assert.True(t, duration < config.MaxRetryBudget*2,
		"execution time %v should not greatly exceed budget %v", 
		duration, config.MaxRetryBudget)
}

func TestCustomRetries(t *testing.T) {
	policy := NewRetryPolicy(&RetryPolicyConfig{
		MaxRetries: 2, // Default low count
		BaseDelay:  1 * time.Millisecond,
	})
	ctx := context.Background()

	// Operation that fails multiple times
	attempts := 0
	operation := func(ctx interface{}) (interface{}, error) {
		attempts++
		if attempts < 5 {
			return nil, &DiagnosticError{
				Code: ErrCodeTimeout,
				Type: ErrTypeContext,
				Message: "timeout",
			}
		}
		return "success", nil
	}

	// Use custom retry count higher than default
	result, err := policy.ExecuteWithCustomRetries(ctx, operation, 5)
	
	assert.NoError(t, err)
	assert.Equal(t, "success", result)
	assert.Equal(t, 5, attempts) // Should have tried 5 times
}

func TestMetricsAccuracy(t *testing.T) {
	policy := NewRetryPolicy(&RetryPolicyConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Millisecond,
	})
	ctx := context.Background()

	// Execute some successful operations
	successOp := func(ctx interface{}) (interface{}, error) {
		return "success", nil
	}
	
	for i := 0; i < 3; i++ {
		_, err := policy.Execute(ctx, successOp)
		require.NoError(t, err)
	}

	// Execute operations that succeed after retries
	retryOp := func() RecoverableOperation {
		attempts := 0
		return func(ctx interface{}) (interface{}, error) {
			attempts++
			if attempts < 3 {
				return nil, &DiagnosticError{
					Code: ErrCodeTimeout,
					Type: ErrTypeContext,
					Message: "timeout",
				}
			}
			return "retry success", nil
		}
	}
	
	for i := 0; i < 2; i++ {
		op := retryOp()
		_, err := policy.Execute(ctx, op)
		require.NoError(t, err)
	}

	// Execute operations that fail completely
	failOp := func(ctx interface{}) (interface{}, error) {
		return nil, &DiagnosticError{
			Code: ErrCodeDockerPermissionDenied,
			Type: ErrTypeConnection,
			Message: "permission denied",
		}
	}
	
	for i := 0; i < 1; i++ {
		_, err := policy.Execute(ctx, failOp)
		require.Error(t, err)
	}

	// Check metrics
	metrics := policy.GetMetrics()
	
	assert.Equal(t, int64(3), metrics.FirstAttemptSuccess) // 3 immediate successes
	assert.Equal(t, int64(2), metrics.SuccessfulRetries)   // 2 operations that succeeded after retry
	assert.Equal(t, int64(1), metrics.FailedRetries)      // 1 operation that failed completely
	assert.Equal(t, int64(4), metrics.TotalRetries)       // 2*2 retries from successful, 0 from failed (non-retryable)
	
	assert.True(t, metrics.TotalRetryTime > 0)
	assert.True(t, metrics.AverageRetryTime > 0)
	assert.True(t, metrics.RetrySuccessRate > 0)
}

func TestHealthChecks(t *testing.T) {
	tests := []struct {
		name            string
		setupOperations func(*RetryPolicy)
		expectedHealthy bool
	}{
		{
			name:            "new policy is healthy",
			setupOperations: func(p *RetryPolicy) {},
			expectedHealthy: true,
		},
		{
			name: "policy with good success rate is healthy",
			setupOperations: func(p *RetryPolicy) {
				ctx := context.Background()
				successOp := func(ctx interface{}) (interface{}, error) {
					return "success", nil
				}
				for i := 0; i < 15; i++ {
					p.Execute(ctx, successOp)
				}
			},
			expectedHealthy: true,
		},
		{
			name: "policy with poor success rate is unhealthy",
			setupOperations: func(p *RetryPolicy) {
				ctx := context.Background()
				failOp := func(ctx interface{}) (interface{}, error) {
					return nil, &DiagnosticError{
						Code: ErrCodeDockerPermissionDenied,
						Type: ErrTypeConnection,
						Message: "permission denied",
					}
				}
				for i := 0; i < 15; i++ {
					p.Execute(ctx, failOp)
				}
			},
			expectedHealthy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := NewRetryPolicy(&RetryPolicyConfig{
				MaxRetries: 1,
				BaseDelay:  1 * time.Millisecond,
			})

			tt.setupOperations(policy)
			
			isHealthy := policy.IsHealthy()
			assert.Equal(t, tt.expectedHealthy, isHealthy)
		})
	}
}

func TestConfigUpdate(t *testing.T) {
	policy := NewRetryPolicy(&RetryPolicyConfig{
		MaxRetries: 2,
		BaseDelay:  100 * time.Millisecond,
	})

	// Update configuration
	newConfig := &RetryPolicyConfig{
		MaxRetries: 5,
		BaseDelay:  200 * time.Millisecond,
		BackoffStrategy: LinearBackoff,
	}
	
	policy.UpdateConfig(newConfig)
	
	// Verify configuration was updated
	assert.Equal(t, 5, policy.config.MaxRetries)
	assert.Equal(t, 200*time.Millisecond, policy.config.BaseDelay)
	assert.Equal(t, LinearBackoff, policy.config.BackoffStrategy)
	
	// Defaults should be set
	assert.Equal(t, 2.0, policy.config.BackoffMultiplier)
	assert.Equal(t, 0.1, policy.config.JitterFactor)
}

func TestReset(t *testing.T) {
	policy := NewRetryPolicy(&RetryPolicyConfig{
		BaseDelay: 1 * time.Millisecond,
	})
	ctx := context.Background()

	// Execute some operations to generate metrics
	successOp := func(ctx interface{}) (interface{}, error) {
		return "success", nil
	}
	
	for i := 0; i < 5; i++ {
		_, err := policy.Execute(ctx, successOp)
		require.NoError(t, err)
	}

	// Verify metrics exist
	metrics := policy.GetMetrics()
	assert.True(t, metrics.FirstAttemptSuccess > 0)

	// Reset policy
	policy.Reset()

	// Verify metrics are cleared
	metrics = policy.GetMetrics()
	assert.Equal(t, int64(0), metrics.FirstAttemptSuccess)
	assert.Equal(t, int64(0), metrics.TotalRetries)
	assert.Equal(t, int64(0), metrics.SuccessfulRetries)
	assert.Equal(t, int64(0), metrics.FailedRetries)
}

// Benchmark tests

func BenchmarkExecuteSuccess(b *testing.B) {
	policy := NewRetryPolicy(&RetryPolicyConfig{
		BaseDelay: 1 * time.Microsecond,
	})
	ctx := context.Background()
	
	operation := func(ctx interface{}) (interface{}, error) {
		return "success", nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := policy.Execute(ctx, operation)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExecuteWithRetries(b *testing.B) {
	policy := NewRetryPolicy(&RetryPolicyConfig{
		MaxRetries: 2,
		BaseDelay:  1 * time.Microsecond,
	})
	ctx := context.Background()
	
	// Operation that succeeds on third attempt
	operation := func() RecoverableOperation {
		attempts := 0
		return func(ctx interface{}) (interface{}, error) {
			attempts++
			if attempts < 3 {
				return nil, &DiagnosticError{
					Code: ErrCodeTimeout,
					Type: ErrTypeContext,
					Message: "timeout",
				}
			}
			return "success", nil
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		op := operation()
		_, err := policy.Execute(ctx, op)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBackoffCalculation(b *testing.B) {
	policy := NewRetryPolicy(&RetryPolicyConfig{
		BaseDelay:         100 * time.Millisecond,
		BackoffStrategy:   ExponentialBackoff,
		BackoffMultiplier: 2.0,
		JitterFactor:      0.1,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		delay := policy.calculateBackoffDelay(3)
		if delay <= 0 {
			b.Fatal("expected positive delay")
		}
	}
}