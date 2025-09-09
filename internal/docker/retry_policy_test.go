package docker

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetryPolicyDetailed(t *testing.T) {
	policy := NewDefaultRetryPolicy()
	
	t.Run("DefaultConfiguration", func(t *testing.T) {
		assert.Equal(t, 3, policy.MaxRetries)
		assert.Equal(t, 100*time.Millisecond, policy.InitialBackoff)
		assert.Equal(t, 5*time.Second, policy.MaxBackoff)
		assert.Equal(t, 2.0, policy.BackoffFactor)
		assert.NotEmpty(t, policy.RetryableErrors)
		
		// Check some known retryable errors
		assert.True(t, policy.RetryableErrors["connection refused"])
		assert.True(t, policy.RetryableErrors["timeout"])
		assert.True(t, policy.RetryableErrors["EOF"])
	})
}

func TestRetryPolicyExecute(t *testing.T) {
	policy := NewDefaultRetryPolicy()
	policy.MaxRetries = 2 // Limit for faster tests
	policy.InitialBackoff = 1 * time.Millisecond
	
	t.Run("SuccessfulOperation", func(t *testing.T) {
		ctx := context.Background()
		callCount := 0
		
		err := policy.Execute(ctx, func() error {
			callCount++
			return nil
		})
		
		assert.NoError(t, err)
		assert.Equal(t, 1, callCount)
	})
	
	t.Run("RetryableErrorEventualSuccess", func(t *testing.T) {
		ctx := context.Background()
		callCount := 0
		
		err := policy.Execute(ctx, func() error {
			callCount++
			if callCount < 3 {
				return errors.New("connection refused") // Retryable
			}
			return nil
		})
		
		assert.NoError(t, err)
		assert.Equal(t, 3, callCount)
	})
	
	t.Run("NonRetryableError", func(t *testing.T) {
		ctx := context.Background()
		callCount := 0
		nonRetryableErr := errors.New("permission denied")
		
		err := policy.Execute(ctx, func() error {
			callCount++
			return nonRetryableErr
		})
		
		assert.Error(t, err)
		assert.Equal(t, nonRetryableErr, err)
		assert.Equal(t, 1, callCount) // Should not retry
	})
	
	t.Run("MaxRetriesExceeded", func(t *testing.T) {
		ctx := context.Background()
		callCount := 0
		retryableErr := errors.New("timeout")
		
		err := policy.Execute(ctx, func() error {
			callCount++
			return retryableErr
		})
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "operation failed after 2 retries")
		assert.Equal(t, 3, callCount) // Initial + 2 retries
	})
	
	t.Run("ContextCancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		callCount := 0
		
		err := policy.Execute(ctx, func() error {
			callCount++
			if callCount == 1 {
				// Cancel context after first attempt
				cancel()
			}
			return errors.New("connection refused")
		})
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "retry cancelled")
		assert.Contains(t, err.Error(), "context canceled")
		assert.Equal(t, 1, callCount)
	})
	
	t.Run("ContextTimeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		defer cancel()
		
		callCount := 0
		
		err := policy.Execute(ctx, func() error {
			callCount++
			time.Sleep(10 * time.Millisecond) // Longer than context timeout
			return errors.New("timeout")
		})
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "retry cancelled")
		// Context timeout should trigger before we can retry
	})
}

func TestRetryPolicyExecuteWithValue(t *testing.T) {
	policy := NewDefaultRetryPolicy()
	policy.MaxRetries = 2
	policy.InitialBackoff = 1 * time.Millisecond
	
	t.Run("SuccessfulOperation", func(t *testing.T) {
		ctx := context.Background()
		callCount := 0
		expectedValue := "success"
		
		result, err := policy.ExecuteWithValue(ctx, func() (interface{}, error) {
			callCount++
			return expectedValue, nil
		})
		
		assert.NoError(t, err)
		assert.Equal(t, expectedValue, result)
		assert.Equal(t, 1, callCount)
	})
	
	t.Run("RetryableErrorEventualSuccess", func(t *testing.T) {
		ctx := context.Background()
		callCount := 0
		expectedValue := "retry-success"
		
		result, err := policy.ExecuteWithValue(ctx, func() (interface{}, error) {
			callCount++
			if callCount < 3 {
				return nil, errors.New("connection reset") // Retryable
			}
			return expectedValue, nil
		})
		
		assert.NoError(t, err)
		assert.Equal(t, expectedValue, result)
		assert.Equal(t, 3, callCount)
	})
	
	t.Run("NonRetryableError", func(t *testing.T) {
		ctx := context.Background()
		callCount := 0
		nonRetryableErr := errors.New("authentication failed")
		
		result, err := policy.ExecuteWithValue(ctx, func() (interface{}, error) {
			callCount++
			return nil, nonRetryableErr
		})
		
		assert.Error(t, err)
		assert.Equal(t, nonRetryableErr, err)
		assert.Nil(t, result)
		assert.Equal(t, 1, callCount)
	})
	
	t.Run("MaxRetriesExceeded", func(t *testing.T) {
		ctx := context.Background()
		callCount := 0
		
		result, err := policy.ExecuteWithValue(ctx, func() (interface{}, error) {
			callCount++
			return nil, errors.New("broken pipe")
		})
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "operation failed after 2 retries")
		assert.Nil(t, result)
		assert.Equal(t, 3, callCount)
	})
}

func TestRetryableErrorDetection(t *testing.T) {
	policy := NewDefaultRetryPolicy()
	
	retryableErrors := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"EOF",
		"broken pipe",
		"service unavailable",
		"too many requests",
		"resource temporarily unavailable",
	}
	
	nonRetryableErrors := []string{
		"permission denied",
		"authentication failed",
		"invalid parameter",
		"not found",
		"forbidden",
		"unauthorized",
	}
	
	t.Run("RetryableErrors", func(t *testing.T) {
		for _, errMsg := range retryableErrors {
			err := errors.New(errMsg)
			assert.True(t, policy.isRetryable(err), "Error should be retryable: %s", errMsg)
			
			// Test with additional context in error message
			err = fmt.Errorf("operation failed: %s", errMsg)
			assert.True(t, policy.isRetryable(err), "Error should be retryable with context: %s", err.Error())
		}
	})
	
	t.Run("NonRetryableErrors", func(t *testing.T) {
		for _, errMsg := range nonRetryableErrors {
			err := errors.New(errMsg)
			assert.False(t, policy.isRetryable(err), "Error should not be retryable: %s", errMsg)
		}
	})
	
	t.Run("NilError", func(t *testing.T) {
		assert.False(t, policy.isRetryable(nil))
	})
}

func TestBackoffCalculation(t *testing.T) {
	policy := &RetryPolicy{
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		BackoffFactor:  2.0,
	}
	
	t.Run("ExponentialBackoff", func(t *testing.T) {
		// Test backoff progression
		backoff0 := policy.calculateBackoff(0)
		backoff1 := policy.calculateBackoff(1)
		backoff2 := policy.calculateBackoff(2)
		
		// Should increase exponentially (with some jitter tolerance)
		assert.GreaterOrEqual(t, backoff0, 90*time.Millisecond)  // 100ms ± 10ms jitter
		assert.LessOrEqual(t, backoff0, 110*time.Millisecond)
		
		assert.GreaterOrEqual(t, backoff1, 180*time.Millisecond) // 200ms ± 10% jitter
		assert.LessOrEqual(t, backoff1, 220*time.Millisecond)
		
		assert.GreaterOrEqual(t, backoff2, 360*time.Millisecond) // 400ms ± 10% jitter
		assert.LessOrEqual(t, backoff2, 440*time.Millisecond)
		
		// Later attempts should be capped at MaxBackoff
		backoffLarge := policy.calculateBackoff(10)
		assert.LessOrEqual(t, backoffLarge, policy.MaxBackoff)
	})
	
	t.Run("MaxBackoffCap", func(t *testing.T) {
		// Even with large attempt numbers, should not exceed max
		for attempt := 10; attempt < 20; attempt++ {
			backoff := policy.calculateBackoff(attempt)
			assert.LessOrEqual(t, backoff, policy.MaxBackoff, "Backoff should not exceed max for attempt %d", attempt)
		}
	})
	
	t.Run("JitterVariation", func(t *testing.T) {
		// Multiple calculations should vary due to jitter
		backoffs := make([]time.Duration, 10)
		for i := 0; i < 10; i++ {
			backoffs[i] = policy.calculateBackoff(1)
		}
		
		// Check that we have some variation (not all identical)
		allSame := true
		for i := 1; i < len(backoffs); i++ {
			if backoffs[i] != backoffs[0] {
				allSame = false
				break
			}
		}
		assert.False(t, allSame, "Backoffs should vary due to jitter")
	})
}

func TestStringContains(t *testing.T) {
	t.Run("ExactMatch", func(t *testing.T) {
		assert.True(t, contains("timeout", "timeout"))
		assert.False(t, contains("timeout", "connection"))
	})
	
	t.Run("Substring", func(t *testing.T) {
		assert.True(t, contains("connection timeout error", "timeout"))
		assert.True(t, contains("timeout occurred", "timeout"))
		assert.True(t, contains("operation timeout", "timeout"))
		assert.False(t, contains("time", "timeout"))
	})
	
	t.Run("EmptyStrings", func(t *testing.T) {
		assert.True(t, contains("", ""))
		assert.False(t, contains("", "test"))
		assert.True(t, contains("test", ""))
	})
	
	t.Run("CaseSensitive", func(t *testing.T) {
		assert.False(t, contains("TIMEOUT", "timeout"))
		assert.False(t, contains("timeout", "TIMEOUT"))
		assert.True(t, contains("timeout", "timeout"))
	})
}

func TestRetryPolicyIntegration(t *testing.T) {
	// Simulate real Docker API scenarios
	
	t.Run("NetworkUnavailable", func(t *testing.T) {
		policy := NewDefaultRetryPolicy()
		policy.MaxRetries = 3
		policy.InitialBackoff = 1 * time.Millisecond
		
		ctx := context.Background()
		callCount := 0
		
		// Simulate network becoming available after 2 failures
		err := policy.Execute(ctx, func() error {
			callCount++
			if callCount <= 2 {
				return errors.New("connection refused")
			}
			return nil // Network available
		})
		
		assert.NoError(t, err)
		assert.Equal(t, 3, callCount)
	})
	
	t.Run("TemporaryServiceUnavailable", func(t *testing.T) {
		policy := NewDefaultRetryPolicy()
		policy.MaxRetries = 2
		policy.InitialBackoff = 1 * time.Millisecond
		
		ctx := context.Background()
		callCount := 0
		
		result, err := policy.ExecuteWithValue(ctx, func() (interface{}, error) {
			callCount++
			if callCount == 1 {
				return nil, errors.New("service unavailable")
			}
			return "service restored", nil
		})
		
		assert.NoError(t, err)
		assert.Equal(t, "service restored", result)
		assert.Equal(t, 2, callCount)
	})
	
	t.Run("PermanentError", func(t *testing.T) {
		policy := NewDefaultRetryPolicy()
		policy.MaxRetries = 3
		policy.InitialBackoff = 1 * time.Millisecond
		
		ctx := context.Background()
		callCount := 0
		
		err := policy.Execute(ctx, func() error {
			callCount++
			return errors.New("No such container: abc123") // Non-retryable
		})
		
		assert.Error(t, err)
		assert.Equal(t, 1, callCount) // Should not retry
	})
}

// Benchmark tests
func BenchmarkRetryPolicyExecute(b *testing.B) {
	policy := NewDefaultRetryPolicy()
	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = policy.Execute(ctx, func() error {
			return nil // Always succeed
		})
	}
}

func BenchmarkRetryPolicyWithFailures(b *testing.B) {
	policy := NewDefaultRetryPolicy()
	policy.InitialBackoff = 1 * time.Microsecond // Very fast for benchmarking
	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		callCount := 0
		_ = policy.Execute(ctx, func() error {
			callCount++
			if callCount == 1 {
				return errors.New("connection refused")
			}
			return nil
		})
	}
}

func BenchmarkBackoffCalculation(b *testing.B) {
	policy := NewDefaultRetryPolicy()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = policy.calculateBackoff(i % 10)
	}
}

func BenchmarkIsRetryable(b *testing.B) {
	policy := NewDefaultRetryPolicy()
	errors := []error{
		errors.New("connection refused"),
		errors.New("timeout"),
		errors.New("permission denied"),
		errors.New("EOF"),
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := errors[i%len(errors)]
		_ = policy.isRetryable(err)
	}
}

func TestRetryPolicyCustomConfiguration(t *testing.T) {
	t.Run("CustomRetryableErrors", func(t *testing.T) {
		policy := &RetryPolicy{
			MaxRetries:     2,
			InitialBackoff: 1 * time.Millisecond,
			MaxBackoff:     1 * time.Second,
			BackoffFactor:  1.5,
			RetryableErrors: map[string]bool{
				"custom error": true,
			},
		}
		
		ctx := context.Background()
		callCount := 0
		
		// Custom error should be retryable
		err := policy.Execute(ctx, func() error {
			callCount++
			if callCount < 3 {
				return errors.New("custom error")
			}
			return nil
		})
		
		assert.NoError(t, err)
		assert.Equal(t, 3, callCount)
		
		// Standard errors should not be retryable
		callCount = 0
		err = policy.Execute(ctx, func() error {
			callCount++
			return errors.New("connection refused")
		})
		
		assert.Error(t, err)
		assert.Equal(t, 1, callCount) // Should not retry
	})
}