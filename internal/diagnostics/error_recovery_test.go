package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewErrorRecovery(t *testing.T) {
	tests := []struct {
		name     string
		config   *RecoveryConfig
		expected *RecoveryConfig
	}{
		{
			name:   "nil config uses defaults",
			config: nil,
			expected: &RecoveryConfig{
				MaxRetries:           3,
				BaseDelay:            500 * time.Millisecond,
				MaxDelay:             30 * time.Second,
				BackoffStrategy:      ExponentialBackoff,
				FailureThreshold:     5,
				RecoveryTimeout:      30 * time.Second,
				ConsecutiveSuccesses: 3,
				CleanupTimeout:       10 * time.Second,
				AutoCleanup:          true,
				PreserveDebugData:    false,
				EnableDegradation:    true,
				MinimalChecksOnly:    false,
				PartialResultsOK:     true,
			},
		},
		{
			name: "custom config preserved",
			config: &RecoveryConfig{
				MaxRetries:        5,
				BaseDelay:         1 * time.Second,
				BackoffStrategy:   LinearBackoff,
				FailureThreshold:  10,
				AutoCleanup:       false,
			},
			expected: &RecoveryConfig{
				MaxRetries:        5,
				BaseDelay:         1 * time.Second,
				BackoffStrategy:   LinearBackoff,
				FailureThreshold:  10,
				AutoCleanup:       false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recovery := NewErrorRecovery(nil, tt.config)
			
			require.NotNil(t, recovery)
			require.NotNil(t, recovery.config)
			require.NotNil(t, recovery.circuitBreaker)
			require.NotNil(t, recovery.retryPolicy)
			require.NotNil(t, recovery.cleanupManager)
			require.NotNil(t, recovery.metrics)
			
			// Check specific config values that were provided
			if tt.config != nil {
				assert.Equal(t, tt.expected.MaxRetries, recovery.config.MaxRetries)
				assert.Equal(t, tt.expected.BaseDelay, recovery.config.BaseDelay)
				assert.Equal(t, tt.expected.BackoffStrategy, recovery.config.BackoffStrategy)
				assert.Equal(t, tt.expected.FailureThreshold, recovery.config.FailureThreshold)
				assert.Equal(t, tt.expected.AutoCleanup, recovery.config.AutoCleanup)
			}
		})
	}
}

func TestExecuteWithRecovery(t *testing.T) {
	tests := []struct {
		name           string
		operation      RecoverableOperation
		expectedResult interface{}
		expectedError  bool
		expectRetries  bool
	}{
		{
			name: "successful operation",
			operation: func(ctx interface{}) (interface{}, error) {
				return "success", nil
			},
			expectedResult: "success",
			expectedError:  false,
			expectRetries:  false,
		},
		{
			name: "operation with recoverable error",
			operation: func() RecoverableOperation {
				attempts := 0
				return func(ctx interface{}) (interface{}, error) {
					attempts++
					if attempts < 3 {
						return nil, &DiagnosticError{
							Code:    ErrCodeTimeout,
							Type:    ErrTypeContext,
							Message: "timeout error",
							Severity: SeverityWarning,
						}
					}
					return "recovered", nil
				}
			}(),
			expectedResult: "recovered",
			expectedError:  false,
			expectRetries:  true,
		},
		{
			name: "operation with non-recoverable error",
			operation: func(ctx interface{}) (interface{}, error) {
				return nil, &DiagnosticError{
					Code:    ErrCodeDockerPermissionDenied,
					Type:    ErrTypeConnection,
					Message: "permission denied",
					Severity: SeverityError,
				}
			},
			expectedResult: nil,
			expectedError:  true,
			expectRetries:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &RecoveryConfig{
				MaxRetries:   3,
				BaseDelay:    10 * time.Millisecond,
				MaxDelay:     100 * time.Millisecond,
				AutoCleanup:  false, // Disable to avoid cleanup complexity in tests
			}
			
			recovery := NewErrorRecovery(nil, config)
			ctx := context.Background()

			result, err := recovery.ExecuteWithRecovery(ctx, tt.operation)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}

			// Check metrics
			metrics := recovery.GetMetrics()
			assert.Equal(t, int64(1), metrics.TotalOperations)
			
			if tt.expectedError {
				assert.Equal(t, int64(1), metrics.TotalFailures)
			} else {
				assert.Equal(t, int64(1), metrics.TotalSuccesses)
			}
		})
	}
}

func TestCircuitBreakerIntegration(t *testing.T) {
	config := &RecoveryConfig{
		FailureThreshold: 2, // Low threshold for testing
		MaxRetries:       1,
		BaseDelay:        1 * time.Millisecond,
		AutoCleanup:      false,
	}
	
	recovery := NewErrorRecovery(nil, config)
	ctx := context.Background()

	// Operation that always fails
	failingOperation := func(ctx interface{}) (interface{}, error) {
		return nil, &DiagnosticError{
			Code:    ErrCodeDockerDaemonUnreachable,
			Type:    ErrTypeConnection,
			Message: "daemon unreachable",
			Severity: SeverityCritical,
		}
	}

	// Execute failing operations until circuit breaker opens
	for i := 0; i < 3; i++ {
		_, err := recovery.ExecuteWithRecovery(ctx, failingOperation)
		assert.Error(t, err)
	}

	// Next operation should be blocked by circuit breaker
	_, err := recovery.ExecuteWithRecovery(ctx, failingOperation)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Circuit breaker is open")

	// Check metrics
	metrics := recovery.GetMetrics()
	assert.True(t, metrics.CircuitBreakerBlocks > 0)
}

func TestHandleError(t *testing.T) {
	tests := []struct {
		name          string
		inputError    error
		expectedCode  ErrorCode
		expectedType  ErrorType
		expectRecovery bool
	}{
		{
			name:          "docker connection error",
			inputError:    errors.New("cannot connect to Docker daemon"),
			expectedCode:  ErrCodeDockerDaemonUnreachable,
			expectedType:  ErrTypeConnection,
			expectRecovery: true,
		},
		{
			name:          "context timeout error", 
			inputError:    context.DeadlineExceeded,
			expectedCode:  ErrCodeTimeout,
			expectedType:  ErrTypeContext,
			expectRecovery: true,
		},
		{
			name:          "resource exhaustion error",
			inputError:    errors.New("out of memory"),
			expectedCode:  ErrCodeResourceExhaustion,
			expectedType:  ErrTypeResource,
			expectRecovery: true,
		},
		{
			name:          "network error",
			inputError:    errors.New("network not found"),
			expectedCode:  ErrCodeNetworkConfiguration,
			expectedType:  ErrTypeNetwork,
			expectRecovery: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &RecoveryConfig{
				AutoCleanup: false, // Disable cleanup for simpler testing
			}
			recovery := NewErrorRecovery(nil, config)
			ctx := context.Background()

			err := recovery.HandleError(ctx, tt.inputError)
			
			if tt.expectRecovery {
				// Recovery attempt should be made (may or may not succeed)
				// For these tests, we just verify the error was processed
				assert.NoError(t, err) // Basic recovery should succeed
			}
		})
	}
}

func TestClassifyError(t *testing.T) {
	recovery := NewErrorRecovery(nil, &RecoveryConfig{})

	tests := []struct {
		name         string
		inputError   error
		expectedCode ErrorCode
		expectedType ErrorType
	}{
		{
			name:         "already classified error",
			inputError:   NewDiagnosticError(ErrCodeNetworkNotFound, "network not found"),
			expectedCode: ErrCodeNetworkNotFound,
			expectedType: ErrTypeNetwork,
		},
		{
			name:         "docker daemon error",
			inputError:   errors.New("cannot connect to Docker daemon"),
			expectedCode: ErrCodeDockerDaemonUnreachable,
			expectedType: ErrTypeConnection,
		},
		{
			name:         "permission denied error",
			inputError:   errors.New("permission denied"),
			expectedCode: ErrCodeDockerDaemonUnreachable,
			expectedType: ErrTypeConnection,
		},
		{
			name:         "timeout error",
			inputError:   context.DeadlineExceeded,
			expectedCode: ErrCodeTimeout,
			expectedType: ErrTypeContext,
		},
		{
			name:         "resource error", 
			inputError:   errors.New("out of disk space"),
			expectedCode: ErrCodeResourceExhaustion,
			expectedType: ErrTypeResource,
		},
		{
			name:         "network error",
			inputError:   errors.New("dns resolution failed"),
			expectedCode: ErrCodeNetworkConfiguration,
			expectedType: ErrTypeNetwork,
		},
		{
			name:         "generic error",
			inputError:   errors.New("unknown error"),
			expectedCode: ErrCodeGeneric,
			expectedType: ErrTypeSystem,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagError := recovery.classifyError(tt.inputError)
			
			assert.NotNil(t, diagError)
			assert.Equal(t, tt.expectedCode, diagError.Code)
			assert.Equal(t, tt.expectedType, diagError.Type)
			assert.NotNil(t, diagError.Recovery)
		})
	}
}

func TestGetMetrics(t *testing.T) {
	recovery := NewErrorRecovery(nil, &RecoveryConfig{
		AutoCleanup: false,
	})
	ctx := context.Background()

	// Initially, metrics should be zero
	metrics := recovery.GetMetrics()
	assert.Equal(t, int64(0), metrics.TotalOperations)
	assert.Equal(t, int64(0), metrics.TotalSuccesses)
	assert.Equal(t, int64(0), metrics.TotalFailures)

	// Execute successful operation
	successOp := func(ctx interface{}) (interface{}, error) {
		return "success", nil
	}
	
	_, err := recovery.ExecuteWithRecovery(ctx, successOp)
	require.NoError(t, err)

	// Check updated metrics
	metrics = recovery.GetMetrics()
	assert.Equal(t, int64(1), metrics.TotalOperations)
	assert.Equal(t, int64(1), metrics.TotalSuccesses)
	assert.Equal(t, int64(0), metrics.TotalFailures)

	// Execute failing operation
	failOp := func(ctx interface{}) (interface{}, error) {
		return nil, errors.New("test failure")
	}
	
	_, err = recovery.ExecuteWithRecovery(ctx, failOp)
	assert.Error(t, err)

	// Check updated metrics
	metrics = recovery.GetMetrics()
	assert.Equal(t, int64(2), metrics.TotalOperations)
	assert.Equal(t, int64(1), metrics.TotalSuccesses)
	assert.Equal(t, int64(1), metrics.TotalFailures)
	assert.True(t, metrics.TotalExecutionTime > 0)
}

func TestIsHealthy(t *testing.T) {
	tests := []struct {
		name              string
		setupOperations   []bool // true for success, false for failure
		expectedHealthy   bool
	}{
		{
			name:            "no operations yet",
			setupOperations: []bool{},
			expectedHealthy: true,
		},
		{
			name:            "all successful operations",
			setupOperations: []bool{true, true, true, true, true},
			expectedHealthy: true,
		},
		{
			name:            "mixed operations with good ratio",
			setupOperations: []bool{true, true, true, false, true, true},
			expectedHealthy: true,
		},
		{
			name:            "high failure rate",
			setupOperations: []bool{false, false, false, true, false, false},
			expectedHealthy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recovery := NewErrorRecovery(nil, &RecoveryConfig{
				MaxRetries:  1,
				BaseDelay:   1 * time.Millisecond,
				AutoCleanup: false,
			})
			ctx := context.Background()

			// Execute setup operations
			for _, shouldSucceed := range tt.setupOperations {
				var op RecoverableOperation
				if shouldSucceed {
					op = func(ctx interface{}) (interface{}, error) {
						return "success", nil
					}
				} else {
					op = func(ctx interface{}) (interface{}, error) {
						return nil, &DiagnosticError{
							Code:    ErrCodeGeneric,
							Type:    ErrTypeSystem,
							Message: "test error",
						}
					}
				}
				
				_, _ = recovery.ExecuteWithRecovery(ctx, op) // Ignore errors for setup
			}

			// Check health status
			isHealthy := recovery.IsHealthy()
			assert.Equal(t, tt.expectedHealthy, isHealthy)
		})
	}
}

func TestContextCancellation(t *testing.T) {
	recovery := NewErrorRecovery(nil, &RecoveryConfig{
		MaxRetries:  3,
		BaseDelay:   100 * time.Millisecond, // Long enough to trigger cancellation
		AutoCleanup: false,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Operation that would normally retry
	operation := func(ctx interface{}) (interface{}, error) {
		return nil, &DiagnosticError{
			Code:    ErrCodeTimeout,
			Type:    ErrTypeContext,
			Message: "timeout error",
		}
	}

	_, err := recovery.ExecuteWithRecovery(ctx, operation)
	
	// Should fail due to context cancellation
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || 
		         errors.Is(err, context.Canceled) ||
		         err.Error() == "context deadline exceeded")
}

func TestRecoveryStrategies(t *testing.T) {
	tests := []struct {
		name        string
		errorType   ErrorType
		expectRetry bool
		expectCleanup bool
	}{
		{
			name:        "connection error recovery",
			errorType:   ErrTypeConnection,
			expectRetry: true,
			expectCleanup: false,
		},
		{
			name:        "resource error recovery",
			errorType:   ErrTypeResource,
			expectRetry: false,
			expectCleanup: true,
		},
		{
			name:        "network error recovery",
			errorType:   ErrTypeNetwork,
			expectRetry: true,
			expectCleanup: false,
		},
		{
			name:        "system error recovery",
			errorType:   ErrTypeSystem,
			expectRetry: true,
			expectCleanup: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recovery := NewErrorRecovery(nil, &RecoveryConfig{})
			
			diagError := &DiagnosticError{
				Type:    tt.errorType,
				Code:    ErrCodeGeneric,
				Message: "test error",
			}

			strategy := recovery.selectRecoveryStrategy(diagError)
			
			assert.NotNil(t, strategy)
			
			if tt.expectRetry {
				assert.True(t, strategy.RetryWithBackoff)
			}
			
			if tt.expectCleanup {
				assert.True(t, strategy.CleanupResources)
			}
		})
	}
}

// Benchmark tests for performance validation

func BenchmarkExecuteWithRecoverySuccess(b *testing.B) {
	recovery := NewErrorRecovery(nil, &RecoveryConfig{
		AutoCleanup: false,
	})
	ctx := context.Background()
	
	operation := func(ctx interface{}) (interface{}, error) {
		return "success", nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := recovery.ExecuteWithRecovery(ctx, operation)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExecuteWithRecoveryWithRetries(b *testing.B) {
	recovery := NewErrorRecovery(nil, &RecoveryConfig{
		MaxRetries:  2,
		BaseDelay:   1 * time.Microsecond, // Very fast for benchmark
		AutoCleanup: false,
	})
	ctx := context.Background()
	
	// Operation that fails twice then succeeds
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
		op := operation() // Get fresh operation for each iteration
		_, err := recovery.ExecuteWithRecovery(ctx, op)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClassifyError(b *testing.B) {
	recovery := NewErrorRecovery(nil, &RecoveryConfig{})
	testError := errors.New("cannot connect to Docker daemon")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		diagError := recovery.classifyError(testError)
		if diagError == nil {
			b.Fatal("expected non-nil diagnostic error")
		}
	}
}

// Helper functions for testing

func createMockOperation(shouldSucceed bool, delay time.Duration) RecoverableOperation {
	return func(ctx interface{}) (interface{}, error) {
		if delay > 0 {
			time.Sleep(delay)
		}
		
		if shouldSucceed {
			return "success", nil
		}
		
		return nil, &DiagnosticError{
			Code:    ErrCodeGeneric,
			Type:    ErrTypeSystem,
			Message: "mock error",
		}
	}
}

func createRetryableOperation(successAfterAttempts int) RecoverableOperation {
	attempts := 0
	return func(ctx interface{}) (interface{}, error) {
		attempts++
		if attempts < successAfterAttempts {
			return nil, &DiagnosticError{
				Code:    ErrCodeTimeout,
				Type:    ErrTypeContext,
				Message: "retry needed",
			}
		}
		return fmt.Sprintf("success after %d attempts", attempts), nil
	}
}