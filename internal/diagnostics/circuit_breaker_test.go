package diagnostics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCircuitBreaker(t *testing.T) {
	tests := []struct {
		name     string
		config   *CircuitBreakerConfig
		expected *CircuitBreakerConfig
	}{
		{
			name:   "nil config uses defaults",
			config: nil,
			expected: &CircuitBreakerConfig{
				FailureThreshold:       5,
				RecoveryTimeout:        30 * time.Second,
				ConsecutiveSuccesses:   3,
				HalfOpenMaxRequests:    2,
				ResetTimeout:           60 * time.Second,
				MinRequestsThreshold:   10,
				RequestVolumeThreshold: 20,
			},
		},
		{
			name: "custom config preserved",
			config: &CircuitBreakerConfig{
				FailureThreshold:     10,
				RecoveryTimeout:      60 * time.Second,
				ConsecutiveSuccesses: 5,
			},
			expected: &CircuitBreakerConfig{
				FailureThreshold:     10,
				RecoveryTimeout:      60 * time.Second,
				ConsecutiveSuccesses: 5,
				HalfOpenMaxRequests:  2, // Default
				ResetTimeout:         60 * time.Second, // Default
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := NewCircuitBreaker(tt.config)
			
			require.NotNil(t, cb)
			require.NotNil(t, cb.config)
			
			assert.Equal(t, CircuitStateClosed, cb.state)
			assert.Equal(t, tt.expected.FailureThreshold, cb.config.FailureThreshold)
			assert.Equal(t, tt.expected.RecoveryTimeout, cb.config.RecoveryTimeout)
			assert.Equal(t, tt.expected.ConsecutiveSuccesses, cb.config.ConsecutiveSuccesses)
			assert.Equal(t, tt.expected.HalfOpenMaxRequests, cb.config.HalfOpenMaxRequests)
		})
	}
}

func TestCircuitBreakerClosedState(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 3,
		ResetTimeout:     100 * time.Millisecond,
	})

	// Initially closed, should allow requests
	assert.True(t, cb.CanExecute())
	assert.Equal(t, CircuitStateClosed, cb.GetState())

	// Record some failures below threshold
	cb.RecordFailure()
	cb.RecordFailure()
	
	assert.True(t, cb.CanExecute()) // Still below threshold
	assert.Equal(t, CircuitStateClosed, cb.GetState())
	assert.Equal(t, 2, cb.GetFailureCount())

	// Record success - should reset consecutive failures
	cb.RecordSuccess()
	assert.Equal(t, 2, cb.GetFailureCount()) // Total failures still 2
	
	// Can still execute
	assert.True(t, cb.CanExecute())
	assert.Equal(t, CircuitStateClosed, cb.GetState())
}

func TestCircuitBreakerOpensOnFailureThreshold(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 3,
		RecoveryTimeout:  100 * time.Millisecond,
	})

	// Record failures to reach threshold
	for i := 0; i < 3; i++ {
		assert.True(t, cb.CanExecute()) // Should allow requests before opening
		cb.RecordFailure()
	}

	// Circuit should now be open
	assert.False(t, cb.CanExecute())
	assert.Equal(t, CircuitStateOpen, cb.GetState())
}

func TestCircuitBreakerHalfOpenState(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:     2,
		RecoveryTimeout:      10 * time.Millisecond, // Short timeout for testing
		ConsecutiveSuccesses: 2,
		HalfOpenMaxRequests:  2,
	})

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()
	assert.False(t, cb.CanExecute())
	assert.Equal(t, CircuitStateOpen, cb.GetState())

	// Wait for recovery timeout
	time.Sleep(15 * time.Millisecond)

	// Should transition to half-open and allow limited requests
	assert.True(t, cb.CanExecute())
	assert.Equal(t, CircuitStateHalfOpen, cb.GetState())

	// Should allow up to HalfOpenMaxRequests
	assert.True(t, cb.CanExecute())
	assert.False(t, cb.CanExecute()) // Should block after max requests

	// Record successes to close circuit
	cb.RecordSuccess()
	cb.RecordSuccess()
	
	// Circuit should now be closed
	assert.Equal(t, CircuitStateClosed, cb.GetState())
	assert.True(t, cb.CanExecute())
}

func TestCircuitBreakerHalfOpenFailureReopens(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:    2,
		RecoveryTimeout:     10 * time.Millisecond,
		HalfOpenMaxRequests: 2,
	})

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()
	assert.Equal(t, CircuitStateOpen, cb.GetState())

	// Wait for recovery and transition to half-open
	time.Sleep(15 * time.Millisecond)
	assert.True(t, cb.CanExecute())
	assert.Equal(t, CircuitStateHalfOpen, cb.GetState())

	// Record failure in half-open state - should reopen circuit
	cb.RecordFailure()
	assert.Equal(t, CircuitStateOpen, cb.GetState())
	assert.False(t, cb.CanExecute())
}

func TestCircuitBreakerMetrics(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 3,
	})

	// Record some operations
	cb.CanExecute()
	cb.RecordSuccess()
	cb.CanExecute()
	cb.RecordFailure()
	cb.CanExecute()
	cb.RecordSuccess()

	metrics := cb.GetMetrics()
	
	assert.Equal(t, CircuitStateClosed, metrics.State)
	assert.Equal(t, 1, metrics.FailureCount)
	assert.Equal(t, int64(3), metrics.TotalRequests)
	assert.Equal(t, int64(1), metrics.TotalFailures)
	assert.Equal(t, int64(2), metrics.TotalSuccesses)
	assert.True(t, metrics.IsHealthy)
	
	// Check failure rate calculation
	expectedFailureRate := 1.0 / 3.0
	assert.InDelta(t, expectedFailureRate, metrics.FailureRate, 0.001)
}

func TestCircuitBreakerReset(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 2,
	})

	// Record some operations
	cb.RecordFailure()
	cb.RecordFailure()
	assert.Equal(t, CircuitStateOpen, cb.GetState())

	// Reset the circuit breaker
	cb.Reset()

	// Should be back to initial state
	assert.Equal(t, CircuitStateClosed, cb.GetState())
	assert.Equal(t, 0, cb.GetFailureCount())
	assert.True(t, cb.CanExecute())

	metrics := cb.GetMetrics()
	assert.Equal(t, int64(0), metrics.TotalRequests)
	assert.Equal(t, int64(0), metrics.TotalFailures)
	assert.Equal(t, int64(0), metrics.TotalSuccesses)
}

func TestCircuitBreakerFailureRateThreshold(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:     100, // High count threshold
		FailureRate:          0.5, // 50% failure rate threshold
		MinRequestsThreshold: 10,  // Minimum requests before considering rate
	})

	// Execute enough requests to enable failure rate checking
	for i := 0; i < 10; i++ {
		cb.CanExecute()
		if i < 6 {
			cb.RecordFailure() // 60% failure rate
		} else {
			cb.RecordSuccess()
		}
	}

	// Should open due to failure rate even though count threshold not reached
	assert.Equal(t, CircuitStateOpen, cb.GetState())
}

func TestCircuitBreakerAdaptiveThresholds(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:       10,
		AdaptiveThresholds:     true,
		RequestVolumeThreshold: 20,
	})

	// Execute high volume of requests
	for i := 0; i < 25; i++ {
		cb.CanExecute()
		cb.RecordFailure() // All failures
	}

	// Should open due to adaptive threshold (should be lower for high volume)
	assert.Equal(t, CircuitStateOpen, cb.GetState())
}

func TestCircuitBreakerHealthScore(t *testing.T) {
	tests := []struct {
		name           string
		state          CircuitState
		setupOperation func(*CircuitBreaker)
		minScore       float64
		maxScore       float64
	}{
		{
			name:  "closed circuit with no operations",
			state: CircuitStateClosed,
			setupOperation: func(cb *CircuitBreaker) {
				// No operations
			},
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:  "closed circuit with all successes",
			state: CircuitStateClosed,
			setupOperation: func(cb *CircuitBreaker) {
				for i := 0; i < 10; i++ {
					cb.CanExecute()
					cb.RecordSuccess()
				}
			},
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:  "closed circuit with some failures",
			state: CircuitStateClosed,
			setupOperation: func(cb *CircuitBreaker) {
				for i := 0; i < 10; i++ {
					cb.CanExecute()
					if i < 7 {
						cb.RecordSuccess()
					} else {
						cb.RecordFailure()
					}
				}
			},
			minScore: 0.4,
			maxScore: 0.8,
		},
		{
			name:  "open circuit",
			state: CircuitStateOpen,
			setupOperation: func(cb *CircuitBreaker) {
				for i := 0; i < 5; i++ {
					cb.RecordFailure()
				}
			},
			minScore: 0.0,
			maxScore: 0.1,
		},
		{
			name:  "half-open circuit with some progress",
			state: CircuitStateHalfOpen,
			setupOperation: func(cb *CircuitBreaker) {
				// Force half-open state and record some success
				cb.transitionToHalfOpen()
				cb.RecordSuccess()
			},
			minScore: 0.3,
			maxScore: 0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := NewCircuitBreaker(&CircuitBreakerConfig{
				FailureThreshold: 5,
			})

			tt.setupOperation(cb)
			
			assert.Equal(t, tt.state, cb.GetState())
			
			healthScore := cb.GetHealthScore()
			assert.True(t, healthScore >= tt.minScore, 
				"health score %f should be >= %f", healthScore, tt.minScore)
			assert.True(t, healthScore <= tt.maxScore,
				"health score %f should be <= %f", healthScore, tt.maxScore)
		})
	}
}

func TestCircuitBreakerConfigUpdate(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:     3,
		ConsecutiveSuccesses: 2,
	})

	// Update configuration
	newConfig := &CircuitBreakerConfig{
		FailureThreshold:     5,
		ConsecutiveSuccesses: 4,
		RecoveryTimeout:      45 * time.Second,
	}
	
	cb.UpdateConfig(newConfig)
	
	// Verify configuration was updated
	assert.Equal(t, 5, cb.config.FailureThreshold)
	assert.Equal(t, 4, cb.config.ConsecutiveSuccesses)
	assert.Equal(t, 45*time.Second, cb.config.RecoveryTimeout)
	
	// Defaults should be set for unspecified values
	assert.Equal(t, 2, cb.config.HalfOpenMaxRequests)
}

func TestCircuitBreakerForceStates(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{})

	// Test force open
	cb.ForceOpen()
	assert.Equal(t, CircuitStateOpen, cb.GetState())
	assert.False(t, cb.CanExecute())

	// Test force close
	cb.ForceClose()
	assert.Equal(t, CircuitStateClosed, cb.GetState())
	assert.True(t, cb.CanExecute())
}

func TestCircuitBreakerStateHistory(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 2,
		RecoveryTimeout:  10 * time.Millisecond,
	})

	// Cause state transitions
	cb.RecordFailure()
	cb.RecordFailure() // Should open
	
	time.Sleep(15 * time.Millisecond)
	cb.CanExecute() // Should transition to half-open
	
	cb.ForceClose() // Force close
	
	history := cb.GetStateHistory()
	
	// Should have recorded transitions
	assert.True(t, history[CircuitStateOpen] > 0)
	assert.True(t, history[CircuitStateHalfOpen] > 0)
	assert.True(t, history[CircuitStateClosed] > 0)
}

func TestCircuitBreakerResetTimeout(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 3,
		ResetTimeout:     50 * time.Millisecond,
	})

	// Record failures
	cb.RecordFailure()
	cb.RecordFailure()
	
	assert.Equal(t, 2, cb.GetFailureCount())

	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)
	
	// Next CanExecute should reset failure count
	cb.CanExecute()
	assert.Equal(t, 0, cb.GetFailureCount())
}

func TestCircuitBreakerConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 10,
	})

	// Test concurrent access
	done := make(chan bool, 100)
	
	for i := 0; i < 100; i++ {
		go func(id int) {
			defer func() { done <- true }()
			
			if cb.CanExecute() {
				if id%2 == 0 {
					cb.RecordSuccess()
				} else {
					cb.RecordFailure()
				}
			}
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Should not panic and should have consistent state
	metrics := cb.GetMetrics()
	assert.True(t, metrics.TotalRequests > 0)
	assert.True(t, metrics.TotalRequests <= 100) // Some might be blocked
}

func TestCircuitBreakerIsHealthy(t *testing.T) {
	tests := []struct {
		name           string
		setupOperation func(*CircuitBreaker)
		expectedHealthy bool
	}{
		{
			name: "healthy closed circuit",
			setupOperation: func(cb *CircuitBreaker) {
				for i := 0; i < 20; i++ {
					cb.CanExecute()
					cb.RecordSuccess()
				}
			},
			expectedHealthy: true,
		},
		{
			name: "unhealthy closed circuit",
			setupOperation: func(cb *CircuitBreaker) {
				for i := 0; i < 20; i++ {
					cb.CanExecute()
					if i < 15 {
						cb.RecordFailure() // 75% failure rate
					} else {
						cb.RecordSuccess()
					}
				}
			},
			expectedHealthy: false,
		},
		{
			name: "open circuit is unhealthy",
			setupOperation: func(cb *CircuitBreaker) {
				for i := 0; i < 5; i++ {
					cb.RecordFailure()
				}
			},
			expectedHealthy: false,
		},
		{
			name: "half-open circuit with progress",
			setupOperation: func(cb *CircuitBreaker) {
				cb.transitionToHalfOpen()
				cb.RecordSuccess()
			},
			expectedHealthy: true,
		},
		{
			name: "half-open circuit with no progress",
			setupOperation: func(cb *CircuitBreaker) {
				cb.transitionToHalfOpen()
				// No successes recorded
			},
			expectedHealthy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := NewCircuitBreaker(&CircuitBreakerConfig{
				FailureThreshold:     5,
				MinRequestsThreshold: 10,
			})

			tt.setupOperation(cb)
			
			isHealthy := cb.IsHealthy()
			assert.Equal(t, tt.expectedHealthy, isHealthy)
		})
	}
}

// Benchmark tests

func BenchmarkCanExecuteClosed(b *testing.B) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{})
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !cb.CanExecute() {
			b.Fatal("expected circuit to allow execution")
		}
	}
}

func BenchmarkCanExecuteOpen(b *testing.B) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 1,
	})
	
	// Open the circuit
	cb.RecordFailure()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if cb.CanExecute() {
			b.Fatal("expected circuit to block execution")
		}
	}
}

func BenchmarkRecordSuccess(b *testing.B) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{})
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.RecordSuccess()
	}
}

func BenchmarkRecordFailure(b *testing.B) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 1000000, // High threshold to prevent opening
	})
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.RecordFailure()
	}
}

func BenchmarkGetMetrics(b *testing.B) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{})
	
	// Add some data
	for i := 0; i < 100; i++ {
		cb.CanExecute()
		if i%2 == 0 {
			cb.RecordSuccess()
		} else {
			cb.RecordFailure()
		}
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics := cb.GetMetrics()
		if metrics.TotalRequests == 0 {
			b.Fatal("expected non-zero metrics")
		}
	}
}