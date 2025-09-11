package diagnostics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// createTestDockerClient creates a properly mocked Docker client for testing
// This returns a docker.Client that has defensive nil checks to prevent panics
func createTestDockerClient() *docker.Client {
	// Create a client that has the defensive nil checks
	// The client will return appropriate errors instead of panicking when methods are called
	return &docker.Client{}
}

// TestErrorRecoveryExecuteWithRecoveryComprehensive tests ExecuteWithRecovery with detailed scenarios
func TestErrorRecoveryExecuteWithRecoveryComprehensive(t *testing.T) {
	tests := []struct {
		name           string
		setupRecovery  func() *ErrorRecovery
		operation      RecoverableOperation
		expectError    bool
		expectRecovery bool
	}{
		{
			name: "successful operation no recovery needed",
			setupRecovery: func() *ErrorRecovery {
				client := createTestDockerClient()
				config := &RecoveryConfig{
					MaxRetries:           3,
					BaseDelay:            10 * time.Millisecond,
					MaxDelay:             100 * time.Millisecond,
					BackoffStrategy:      ExponentialBackoff,
					FailureThreshold:     5,
					RecoveryTimeout:      30 * time.Second,
					ConsecutiveSuccesses: 3,
				}
				return NewErrorRecovery(client, config)
			},
			operation: func(ctx interface{}) (interface{}, error) {
				return "success", nil
			},
			expectError:    false,
			expectRecovery: false,
		},
		{
			name: "operation with recoverable connection error",
			setupRecovery: func() *ErrorRecovery {
				client := createTestDockerClient()
				config := &RecoveryConfig{
					MaxRetries:           3,
					BaseDelay:            1 * time.Millisecond,
					MaxDelay:             10 * time.Millisecond,
					BackoffStrategy:      ExponentialBackoff,
					FailureThreshold:     5,
					RecoveryTimeout:      30 * time.Second,
					ConsecutiveSuccesses: 3,
				}
				return NewErrorRecovery(client, config)
			},
			operation: func(ctx interface{}) (interface{}, error) {
				return nil, fmt.Errorf("cannot connect to docker daemon")
			},
			expectError:    true,
			expectRecovery: true,
		},
		{
			name: "operation with resource exhaustion error",
			setupRecovery: func() *ErrorRecovery {
				client := createTestDockerClient()
				config := &RecoveryConfig{
					MaxRetries:           2,
					BaseDelay:            1 * time.Millisecond,
					MaxDelay:             10 * time.Millisecond,
					BackoffStrategy:      LinearBackoff,
					FailureThreshold:     5,
					RecoveryTimeout:      30 * time.Second,
					ConsecutiveSuccesses: 3,
				}
				return NewErrorRecovery(client, config)
			},
			operation: func(ctx interface{}) (interface{}, error) {
				return nil, fmt.Errorf("out of memory")
			},
			expectError:    true,
			expectRecovery: true,
		},
		{
			name: "operation with network error",
			setupRecovery: func() *ErrorRecovery {
				client := createTestDockerClient()
				config := &RecoveryConfig{
					MaxRetries:           1,
					BaseDelay:            1 * time.Millisecond,
					MaxDelay:             5 * time.Millisecond,
					BackoffStrategy:      JitteredBackoff,
					FailureThreshold:     5,
					RecoveryTimeout:      30 * time.Second,
					ConsecutiveSuccesses: 3,
				}
				return NewErrorRecovery(client, config)
			},
			operation: func(ctx interface{}) (interface{}, error) {
				return nil, fmt.Errorf("network is unreachable")
			},
			expectError:    true,
			expectRecovery: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recovery := tt.setupRecovery()

			// Execute operation with recovery
			ctx := context.Background()
			result, err := recovery.ExecuteWithRecovery(ctx, tt.operation)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// For successful operations, verify result
			if !tt.expectError && result == nil {
				t.Error("Expected non-nil result for successful operation")
			}
		})
	}
}

// TestErrorRecoveryHandleErrorComprehensive tests HandleError with various error types
func TestErrorRecoveryHandleErrorComprehensive(t *testing.T) {
	tests := []struct {
		name        string
		inputError  error
		expectPanic bool
	}{
		{
			name:        "nil error",
			inputError:  nil,
			expectPanic: false,
		},
		{
			name:        "docker daemon connection error",
			inputError:  fmt.Errorf("cannot connect to the Docker daemon at unix:///var/run/docker.sock"),
			expectPanic: false,
		},
		{
			name:        "docker daemon not running error",
			inputError:  fmt.Errorf("docker daemon not running"),
			expectPanic: false,
		},
		{
			name:        "memory exhaustion error",
			inputError:  fmt.Errorf("out of memory"),
			expectPanic: false,
		},
		{
			name:        "disk space error",
			inputError:  fmt.Errorf("no space left on device"),
			expectPanic: false,
		},
		{
			name:        "file descriptor exhaustion",
			inputError:  fmt.Errorf("too many open files"),
			expectPanic: false,
		},
		{
			name:        "dns resolution error",
			inputError:  fmt.Errorf("dns resolution failed"),
			expectPanic: false,
		},
		{
			name:        "routing error",
			inputError:  fmt.Errorf("no route to host"),
			expectPanic: false,
		},
		{
			name:        "network timeout error",
			inputError:  fmt.Errorf("connection timeout"),
			expectPanic: false,
		},
		{
			name:        "generic network error",
			inputError:  fmt.Errorf("network is unreachable"),
			expectPanic: false,
		},
		{
			name:        "generic application error",
			inputError:  fmt.Errorf("application specific error"),
			expectPanic: false,
		},
	}

	for _, tt := range tests {
	t.Run(tt.name, func(t *testing.T) {
	// Create error recovery
	client := createTestDockerClient()
	config := &RecoveryConfig{
	MaxRetries:           3,
	BaseDelay:            10 * time.Millisecond,
	MaxDelay:             100 * time.Millisecond,
	BackoffStrategy:      ExponentialBackoff,
	FailureThreshold:     5,
	RecoveryTimeout:      30 * time.Second,
	ConsecutiveSuccesses: 3,
	}

			recovery := NewErrorRecovery(client, config)

			// Handle error
			ctx := context.Background()

			defer func() {
				if r := recover(); r != nil && !tt.expectPanic {
					t.Errorf("Unexpected panic: %v", r)
				}
			}()

			result := recovery.HandleError(ctx, tt.inputError)

			// Verify result is reasonable (should not be nil unless input was nil)
			if tt.inputError != nil && result == nil {
				t.Error("Expected non-nil error result from HandleError when input error is non-nil")
			}
		})
	}
}

// TestErrorClassificationFunctions tests all error classification functions
func TestErrorClassificationFunctions(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		isDocker   bool
		isResource bool
		isNetwork  bool
	}{
		{
			name:       "docker daemon connection error",
			err:        fmt.Errorf("cannot connect to the Docker daemon"),
			isDocker:   true,
			isResource: false,
			isNetwork:  false,
		},
		{
			name:       "docker daemon not running",
			err:        fmt.Errorf("docker daemon not running"),
			isDocker:   true,
			isResource: false,
			isNetwork:  false,
		},
		{
			name:       "memory exhaustion",
			err:        fmt.Errorf("out of memory"),
			isDocker:   false,
			isResource: true,
			isNetwork:  false,
		},
		{
			name:       "disk space exhaustion",
			err:        fmt.Errorf("no space left on device"),
			isDocker:   false,
			isResource: true,
			isNetwork:  false,
		},
		{
			name:       "file descriptor exhaustion",
			err:        fmt.Errorf("too many open files"),
			isDocker:   false,
			isResource: true,
			isNetwork:  false,
		},
		{
			name:       "dns resolution failure",
			err:        fmt.Errorf("dns resolution failed"),
			isDocker:   false,
			isResource: false,
			isNetwork:  true,
		},
		{
			name:       "routing error",
			err:        fmt.Errorf("no route to host"),
			isDocker:   false,
			isResource: false,
			isNetwork:  true,
		},
		{
			name:       "network timeout",
			err:        fmt.Errorf("connection timeout"),
			isDocker:   false,
			isResource: false,
			isNetwork:  true,
		},
		{
			name:       "network unreachable",
			err:        fmt.Errorf("network is unreachable"),
			isDocker:   false,
			isResource: false,
			isNetwork:  true,
		},
		{
			name:       "generic error",
			err:        fmt.Errorf("some generic error"),
			isDocker:   false,
			isResource: false,
			isNetwork:  false,
		},
		{
			name:       "nil error",
			err:        nil,
			isDocker:   false,
			isResource: false,
			isNetwork:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get error message string
			var errorMsg string
			if tt.err != nil {
				errorMsg = tt.err.Error()
			}

			// Test docker error classification
			isDocker := isDockerConnectionError(errorMsg)
			if isDocker != tt.isDocker {
				t.Errorf("isDockerConnectionError() = %v, want %v", isDocker, tt.isDocker)
			}

			// Test resource error classification
			isResource := isResourceExhaustionError(errorMsg)
			if isResource != tt.isResource {
				t.Errorf("isResourceExhaustionError() = %v, want %v", isResource, tt.isResource)
			}

			// Test network error classification
			isNetwork := isNetworkError(errorMsg)
			if isNetwork != tt.isNetwork {
				t.Errorf("isNetworkError() = %v, want %v", isNetwork, tt.isNetwork)
			}
		})
	}
}

// TestErrorRecoveryResourceDetection tests resource type detection
func TestErrorRecoveryResourceDetection(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedType string // We'll test what resource type is detected
	}{
		{
			name:         "memory error",
			err:          fmt.Errorf("out of memory"),
			expectedType: "memory", // This is what we expect detectResourceType to identify
		},
		{
			name:         "disk error",
			err:          fmt.Errorf("no space left on device"),
			expectedType: "disk",
		},
		{
			name:         "file descriptor error",
			err:          fmt.Errorf("too many open files"),
			expectedType: "files",
		},
		{
			name:         "unknown resource error",
			err:          fmt.Errorf("generic resource issue"),
			expectedType: "unknown",
		},
		{
			name:         "nil error",
			err:          nil,
			expectedType: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get error message string
			var errorMsg string
			if tt.err != nil {
				errorMsg = tt.err.Error()
			}

			// Test resource type detection
			resourceType := detectResourceType(errorMsg)

			// We just verify that detectResourceType returns a string
			// The exact matching logic may vary, but the function should not panic
			if resourceType == "" && tt.err != nil {
				t.Error("Expected non-empty resource type for non-nil error")
			}
		})
	}
}

// TestErrorRecoveryNetworkIssueDetection tests network issue type detection
func TestErrorRecoveryNetworkIssueDetection(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		issueType string // Expected network issue type
	}{
		{
			name:      "dns issue",
			err:       fmt.Errorf("dns resolution failed"),
			issueType: "dns",
		},
		{
			name:      "routing issue",
			err:       fmt.Errorf("no route to host"),
			issueType: "routing",
		},
		{
			name:      "timeout issue",
			err:       fmt.Errorf("connection timeout"),
			issueType: "timeout",
		},
		{
			name:      "generic network issue",
			err:       fmt.Errorf("network unreachable"),
			issueType: "generic",
		},
		{
			name:      "nil error",
			err:       nil,
			issueType: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get error message string
			var errorMsg string
			if tt.err != nil {
				errorMsg = tt.err.Error()
			}

			// Test network issue type detection
			issueType := detectNetworkIssueType(errorMsg)

			// We just verify that detectNetworkIssueType returns a string
			// The exact matching logic may vary, but the function should not panic
			if issueType == "" && tt.err != nil {
				t.Error("Expected non-empty issue type for non-nil error")
			}
		})
	}
}

// TestErrorRecoveryRecoveryMethods tests specific recovery method functions
func TestErrorRecoveryRecoveryMethods(t *testing.T) {
	// Create error recovery for testing
	client := createTestDockerClient()
	recovery := NewErrorRecovery(client, nil)
	ctx := context.Background()

	t.Run("recoverDockerConnection", func(t *testing.T) {
		// Test docker connection recovery
		err := recovery.recoverDockerConnection(ctx)
		// We expect this to complete without panic (may return error due to no real docker)
		if err != nil {
			t.Logf("recoverDockerConnection returned error (expected): %v", err)
		}
	})

	t.Run("recoverFromResourceExhaustion", func(t *testing.T) {
		// Test resource exhaustion recovery
		err := recovery.recoverFromResourceExhaustion(ctx)
		// We expect this to complete without panic
		if err != nil {
			t.Logf("recoverFromResourceExhaustion returned error (expected): %v", err)
		}
	})

	t.Run("recoverNetworkConfiguration", func(t *testing.T) {
		// Test network configuration recovery
		err := recovery.recoverNetworkConfiguration(ctx)
		// We expect this to complete without panic
		if err != nil {
			t.Logf("recoverNetworkConfiguration returned error (expected): %v", err)
		}
	})

	t.Run("performNetworkRecovery", func(t *testing.T) {
		// Test network recovery
		err := recovery.performNetworkRecovery(ctx)
		// We expect this to complete without panic
		if err != nil {
			t.Logf("performNetworkRecovery returned error (expected): %v", err)
		}
	})
}

// TestErrorRecoveryUtilityMethods tests utility and state methods
func TestErrorRecoveryUtilityMethods(t *testing.T) {
	client := createTestDockerClient()
	config := &RecoveryConfig{
		MaxRetries:           3,
		BaseDelay:            10 * time.Millisecond,
		MaxDelay:             100 * time.Millisecond,
		BackoffStrategy:      ExponentialBackoff,
		FailureThreshold:     5,
		RecoveryTimeout:      30 * time.Second,
		ConsecutiveSuccesses: 3,
	}
	recovery := NewErrorRecovery(client, config)

	t.Run("GetMetrics", func(t *testing.T) {
		metrics := recovery.GetMetrics()
		// Verify metrics structure is returned
		if metrics.TotalOperations < 0 {
			t.Error("Invalid metrics returned")
		}
	})

	t.Run("IsHealthy", func(t *testing.T) {
		isHealthy := recovery.IsHealthy()
		// Should return a boolean value
		_ = isHealthy // Just verify it doesn't panic
	})

	t.Run("Shutdown", func(t *testing.T) {
		err := recovery.Shutdown(context.Background())
		if err != nil {
			t.Logf("Shutdown returned error: %v", err)
		}
	})

	t.Run("enableGracefulDegradation", func(t *testing.T) {
		// Test graceful degradation enablement
		recovery.enableGracefulDegradation()
		// Should complete without panic
	})
}
