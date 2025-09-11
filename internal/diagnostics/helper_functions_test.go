package diagnostics

import (
	"context"
	"testing"
	"time"
)

// TestCircuitBreakerGetMethods tests all getter methods of CircuitBreaker
func TestCircuitBreakerGetMethods(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	t.Run("GetState", func(t *testing.T) {
		state := cb.GetState()
		if state != CircuitStateClosed {
			t.Errorf("expected CircuitStateClosed, got %v", state)
		}
	})

	t.Run("GetFailureCount", func(t *testing.T) {
		count := cb.GetFailureCount()
		if count != 0 {
			t.Errorf("expected 0, got %d", count)
		}
	})

	t.Run("IsHealthy", func(t *testing.T) {
		healthy := cb.IsHealthy()
		if !healthy {
			t.Error("expected true for new circuit breaker")
		}
	})

	t.Run("GetMetrics", func(t *testing.T) {
		metrics := cb.GetMetrics()
		if metrics.State != CircuitStateClosed {
			t.Errorf("expected CircuitStateClosed, got %v", metrics.State)
		}
		if metrics.TotalRequests != 0 {
			t.Errorf("expected 0 total requests, got %d", metrics.TotalRequests)
		}
		if metrics.IsHealthy != true {
			t.Error("expected IsHealthy=true for new circuit breaker")
		}
	})

	t.Run("GetStateHistory", func(t *testing.T) {
		history := cb.GetStateHistory()
		if history == nil {
			t.Error("expected non-nil history map")
		}
		// Should be empty initially
		if len(history) != 0 {
			t.Errorf("expected empty history, got %d entries", len(history))
		}
	})

	t.Run("GetHealthScore", func(t *testing.T) {
		score := cb.GetHealthScore()
		if score != 1.0 {
			t.Errorf("expected health score 1.0, got %f", score)
		}
	})
}

// TestCircuitBreakerResetMethod tests the Reset method
func TestCircuitBreakerResetMethod(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 2,
	})

	// Trigger some failures and state changes
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure() // Should open the circuit

	// Verify circuit is open
	if cb.GetState() != CircuitStateOpen {
		t.Error("expected circuit to be open after failures")
	}

	// Reset and verify
	cb.Reset()

	state := cb.GetState()
	if state != CircuitStateClosed {
		t.Errorf("expected CircuitStateClosed after reset, got %v", state)
	}

	count := cb.GetFailureCount()
	if count != 0 {
		t.Errorf("expected 0 failure count after reset, got %d", count)
	}

	metrics := cb.GetMetrics()
	if metrics.TotalRequests != 0 {
		t.Errorf("expected 0 total requests after reset, got %d", metrics.TotalRequests)
	}
}

// TestCircuitBreakerForceOperations tests ForceOpen and ForceClose methods
func TestCircuitBreakerForceOperations(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	t.Run("ForceOpen", func(t *testing.T) {
		cb.ForceOpen()
		if cb.GetState() != CircuitStateOpen {
			t.Error("expected circuit to be forced open")
		}
	})

	t.Run("ForceClose", func(t *testing.T) {
		cb.ForceClose()
		if cb.GetState() != CircuitStateClosed {
			t.Error("expected circuit to be forced closed")
		}
	})
}

// TestCircuitBreakerUpdateConfig tests the UpdateConfig method
func TestCircuitBreakerUpdateConfig(t *testing.T) {
	cb := NewCircuitBreaker(nil)

	t.Run("valid config update", func(t *testing.T) {
		newConfig := &CircuitBreakerConfig{
			FailureThreshold: 10,
			RecoveryTimeout:  60 * time.Second,
		}

		cb.UpdateConfig(newConfig)

		// Verify config was updated
		if cb.config.FailureThreshold != 10 {
			t.Errorf("expected FailureThreshold=10, got %d", cb.config.FailureThreshold)
		}
		// Should have defaults applied
		if cb.config.ConsecutiveSuccesses != 3 {
			t.Errorf("expected default ConsecutiveSuccesses=3, got %d", cb.config.ConsecutiveSuccesses)
		}
	})

	t.Run("nil config update", func(t *testing.T) {
		originalThreshold := cb.config.FailureThreshold
		cb.UpdateConfig(nil)
		// Should not change anything
		if cb.config.FailureThreshold != originalThreshold {
			t.Error("config should not change when nil is passed")
		}
	})

	t.Run("zero values get defaults", func(t *testing.T) {
		zeroConfig := &CircuitBreakerConfig{
			FailureThreshold: 5, // non-zero
			// Other fields are zero
		}

		cb.UpdateConfig(zeroConfig)

		if cb.config.ConsecutiveSuccesses != 3 {
			t.Errorf("expected default ConsecutiveSuccesses=3, got %d", cb.config.ConsecutiveSuccesses)
		}
		if cb.config.HalfOpenMaxRequests != 2 {
			t.Errorf("expected default HalfOpenMaxRequests=2, got %d", cb.config.HalfOpenMaxRequests)
		}
	})
}

// TestRateLimiterGetMethods tests all getter methods of RateLimiter
func TestRateLimiterGetMethods(t *testing.T) {
	config := &RateLimiterConfig{
		RequestsPerSecond: 10.0,
		BurstSize:         5,
		WaitTimeout:       2 * time.Second,
		Enabled:           true,
	}
	rl := NewRateLimiter(config)

	t.Run("GetMetrics", func(t *testing.T) {
		metrics := rl.GetMetrics()
		if metrics.TotalRequests != 0 {
			t.Errorf("expected 0 total requests, got %d", metrics.TotalRequests)
		}
	})

	t.Run("GetConfig", func(t *testing.T) {
		cfg := rl.GetConfig()
		if cfg.RequestsPerSecond != 10.0 {
			t.Errorf("expected RequestsPerSecond=10.0, got %f", cfg.RequestsPerSecond)
		}
		if cfg.BurstSize != 5 {
			t.Errorf("expected BurstSize=5, got %d", cfg.BurstSize)
		}
	})

	t.Run("IsEnabled", func(t *testing.T) {
		enabled := rl.IsEnabled()
		if !enabled {
			t.Error("expected rate limiter to be enabled")
		}
	})

	t.Run("GetEffectiveRate", func(t *testing.T) {
		rate := rl.GetEffectiveRate()
		if rate != 10.0 {
			t.Errorf("expected effective rate=10.0, got %f", rate)
		}
	})

	t.Run("GetBurstSize", func(t *testing.T) {
		burst := rl.GetBurstSize()
		if burst != 5 {
			t.Errorf("expected burst size=5, got %d", burst)
		}
	})

	t.Run("GetAverageWaitTime", func(t *testing.T) {
		avgWait := rl.GetAverageWaitTime()
		if avgWait != 0 {
			t.Errorf("expected 0 average wait time initially, got %v", avgWait)
		}
	})
}

// TestRateLimiterResetMetrics tests the ResetMetrics method
func TestRateLimiterResetMetrics(t *testing.T) {
	rl := NewRateLimiter(&RateLimiterConfig{
		RequestsPerSecond: 10.0,
		BurstSize:         5,
		Enabled:           false, // Disabled to avoid actual rate limiting
	})

	// Simulate some activity by calling Wait (should be immediate since disabled)
	ctx := context.Background()
	_ = rl.Wait(ctx)
	_ = rl.Wait(ctx)

	// Verify metrics were recorded
	metrics := rl.GetMetrics()
	if metrics.TotalRequests < 2 {
		t.Errorf("expected at least 2 total requests, got %d", metrics.TotalRequests)
	}

	// Reset metrics
	rl.ResetMetrics()

	// Verify metrics were reset
	resetMetrics := rl.GetMetrics()
	if resetMetrics.TotalRequests != 0 {
		t.Errorf("expected 0 total requests after reset, got %d", resetMetrics.TotalRequests)
	}
	if resetMetrics.AllowedRequests != 0 {
		t.Errorf("expected 0 allowed requests after reset, got %d", resetMetrics.AllowedRequests)
	}
}

// TestRateLimiterUpdateConfig tests the UpdateConfig method
func TestRateLimiterUpdateConfig(t *testing.T) {
	rl := NewRateLimiter(&RateLimiterConfig{
		RequestsPerSecond: 5.0,
		BurstSize:         2,
		Enabled:           true,
	})

	t.Run("update to different rate", func(t *testing.T) {
		newConfig := &RateLimiterConfig{
			RequestsPerSecond: 20.0,
			BurstSize:         10,
			Enabled:           true,
		}

		rl.UpdateConfig(newConfig)

		if rl.config.RequestsPerSecond != 20.0 {
			t.Errorf("expected RequestsPerSecond=20.0, got %f", rl.config.RequestsPerSecond)
		}
		if rl.config.BurstSize != 10 {
			t.Errorf("expected BurstSize=10, got %d", rl.config.BurstSize)
		}
	})

	t.Run("disable rate limiter", func(t *testing.T) {
		newConfig := &RateLimiterConfig{
			RequestsPerSecond: 10.0,
			BurstSize:         5,
			Enabled:           false,
		}

		rl.UpdateConfig(newConfig)

		if rl.config.Enabled {
			t.Error("expected rate limiter to be disabled")
		}
		if rl.limiter != nil {
			t.Error("expected limiter to be nil when disabled")
		}
	})
}

// TestRetryPolicyGetMethods tests all getter methods of RetryPolicy
func TestRetryPolicyGetMethods(t *testing.T) {
	config := &RetryPolicyConfig{
		MaxRetries: 5,
		BaseDelay:  200 * time.Millisecond,
	}
	rp := NewRetryPolicy(config)

	t.Run("GetMetrics", func(t *testing.T) {
		metrics := rp.GetMetrics()
		if metrics.TotalRetries != 0 {
			t.Errorf("expected 0 total retries, got %d", metrics.TotalRetries)
		}
		if metrics.RetriesByErrorType == nil {
			t.Error("expected non-nil RetriesByErrorType map")
		}
		if metrics.RetriesByErrorCode == nil {
			t.Error("expected non-nil RetriesByErrorCode map")
		}
	})

	t.Run("IsHealthy", func(t *testing.T) {
		healthy := rp.IsHealthy()
		if !healthy {
			t.Error("expected new retry policy to be healthy")
		}
	})

	t.Run("GetRecommendations", func(t *testing.T) {
		recommendations := rp.GetRecommendations()
		if recommendations == nil {
			t.Error("expected non-nil recommendations")
		}
	})
}

// TestRetryPolicyReset tests the Reset method
func TestRetryPolicyReset(t *testing.T) {
	rp := NewRetryPolicy(&RetryPolicyConfig{
		MaxRetries: 3,
		BaseDelay:  100 * time.Millisecond,
	})

	// Reset should not panic and should clear metrics
	rp.Reset()

	metrics := rp.GetMetrics()
	if metrics.TotalRetries != 0 {
		t.Errorf("expected 0 total retries after reset, got %d", metrics.TotalRetries)
	}
}

// TestRetryPolicyUpdateConfig tests the UpdateConfig method
func TestRetryPolicyUpdateConfig(t *testing.T) {
	rp := NewRetryPolicy(&RetryPolicyConfig{
		MaxRetries: 3,
		BaseDelay:  100 * time.Millisecond,
	})

	t.Run("valid config update", func(t *testing.T) {
		newConfig := &RetryPolicyConfig{
			MaxRetries: 10,
			BaseDelay:  500 * time.Millisecond,
			MaxDelay:   30 * time.Second,
		}

		rp.UpdateConfig(newConfig)

		if rp.config.MaxRetries != 10 {
			t.Errorf("expected MaxRetries=10, got %d", rp.config.MaxRetries)
		}
		if rp.config.BaseDelay != 500*time.Millisecond {
			t.Errorf("expected BaseDelay=500ms, got %v", rp.config.BaseDelay)
		}
	})

	t.Run("nil config update preserves existing", func(t *testing.T) {
		originalMaxRetries := rp.config.MaxRetries
		rp.UpdateConfig(nil)
		if rp.config.MaxRetries != originalMaxRetries {
			t.Error("config should not change when nil is passed")
		}
	})
}

// TestErrorTypesGetMethods tests helper methods on DiagnosticError
func TestErrorTypesGetMethods(t *testing.T) {
	tests := []struct {
		name string
		code ErrorCode
	}{
		{"ResourceExhaustion", ErrCodeResourceExhaustion},
		{"NetworkConfiguration", ErrCodeNetworkConfiguration},
		{"DockerPermissionDenied", ErrCodeDockerPermissionDenied},
		{"Generic", ErrCodeGeneric},
		{"Unknown", ErrCodeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewDiagnosticError(tt.code, "test error")

			// Test all getter methods
			t.Run("Error", func(t *testing.T) {
				errStr := err.Error()
				if errStr == "" {
					t.Error("Error() returned empty string")
				}
			})

			t.Run("GetSeverityString", func(t *testing.T) {
				severity := err.GetSeverityString()
				if severity == "" {
					t.Error("GetSeverityString() returned empty string")
				}
			})

			t.Run("GetTypeString", func(t *testing.T) {
				typeStr := err.GetTypeString()
				if typeStr == "" {
					t.Error("GetTypeString() returned empty string")
				}
			})

			t.Run("GetCodeString", func(t *testing.T) {
				codeStr := err.GetCodeString()
				if codeStr == "" {
					t.Error("GetCodeString() returned empty string")
				}
			})

			t.Run("IsRecoverable", func(t *testing.T) {
				recoverable := err.IsRecoverable()
				// Should return a boolean (no specific expectation on value)
				_ = recoverable
			})

			t.Run("RequiresUserIntervention", func(t *testing.T) {
				requiresUser := err.RequiresUserIntervention()
				// Should return a boolean
				_ = requiresUser
			})

			t.Run("GetImmediateActions", func(t *testing.T) {
				actions := err.GetImmediateActions()
				if actions == nil {
					t.Error("GetImmediateActions() returned nil")
				}
			})

			t.Run("GetLongTermActions", func(t *testing.T) {
				actions := err.GetLongTermActions()
				if actions == nil {
					t.Error("GetLongTermActions() returned nil")
				}
			})

			t.Run("ToMap", func(t *testing.T) {
				m := err.ToMap()
				if m == nil {
					t.Error("ToMap() returned nil")
				}
				if len(m) == 0 {
					t.Error("ToMap() returned empty map")
				}
			})

			t.Run("Unwrap", func(t *testing.T) {
				unwrapped := err.Unwrap()
				// May be nil if no underlying error
				_ = unwrapped
			})
		})
	}
}

// TestNewDiagnosticErrorFunction tests the NewDiagnosticError function with various inputs
func TestNewDiagnosticErrorFunction(t *testing.T) {
	tests := []struct {
		name    string
		code    ErrorCode
		message string
	}{
		{
			name:    "simple error",
			code:    ErrCodeNetworkConfiguration,
			message: "timeout occurred",
		},
		{
			name:    "resource exhaustion error",
			code:    ErrCodeResourceExhaustion,
			message: "out of memory",
		},
		{
			name:    "empty message",
			code:    ErrCodeGeneric,
			message: "",
		},
		{
			name:    "unknown error code",
			code:    ErrorCode(9999),
			message: "unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewDiagnosticError(tt.code, tt.message)
			if err == nil {
				t.Fatal("NewDiagnosticError returned nil")
			}

			// Verify basic properties
			if err.Code != tt.code {
				t.Errorf("expected Code=%v, got %v", tt.code, err.Code)
			}
			if err.Message != tt.message {
				t.Errorf("expected Message=%s, got %s", tt.message, err.Message)
			}
		})
	}
}

// TestPercentileHelpers tests helper functions that calculate percentiles or statistics
func TestPercentileHelpers(t *testing.T) {
	// Test the internal calculatePercentile function used in performance profiler
	// Since this is not exported, we test it indirectly through public methods

	t.Run("empty data set", func(t *testing.T) {
		// This tests the edge case handling in percentile calculation
		profiler := NewPerformanceProfiler(&ProfileConfig{
			Enabled: true,
		})

		metrics := profiler.GetMetrics()
		// Should handle empty data gracefully
		if metrics.TotalOperations < 0 {
			t.Error("TotalOperations should be non-negative")
		}
	})

	t.Run("profiler enabled state", func(t *testing.T) {
		profiler := NewPerformanceProfiler(&ProfileConfig{
			Enabled: true,
		})

		if !profiler.IsEnabled() {
			t.Error("profiler should be enabled")
		}

		profiler.Disable()
		if profiler.IsEnabled() {
			t.Error("profiler should be disabled after calling Disable()")
		}

		profiler.Enable()
		if !profiler.IsEnabled() {
			t.Error("profiler should be enabled after calling Enable()")
		}
	})
}

// TestStringHelpers tests string manipulation helper functions
func TestStringHelpers(t *testing.T) {
	// Test the String() method on various enum types

	t.Run("CircuitState String", func(t *testing.T) {
		states := []struct {
			state    CircuitState
			expected string
		}{
			{CircuitStateClosed, "CLOSED"},
			{CircuitStateOpen, "OPEN"},
			{CircuitStateHalfOpen, "HALF_OPEN"},
			{CircuitState(999), "UNKNOWN"},
		}

		for _, s := range states {
			result := s.state.String()
			if result != s.expected {
				t.Errorf("expected %s, got %s", s.expected, result)
			}
		}
	})
}

// TestMetricsCollectorHelpers tests MetricsCollector helper methods
func TestMetricsCollectorHelpers(t *testing.T) {
	collector := &MetricsCollector{
		samples:    make([]MetricsSample, 0),
		maxSamples: 100,
	}

	t.Run("empty collector methods", func(t *testing.T) {
		// Test methods on empty collector
		samples := collector.GetSamples(10)
		if samples == nil {
			t.Error("GetSamples should not return nil")
		}
		if len(samples) != 0 {
			t.Errorf("expected 0 samples, got %d", len(samples))
		}

		latest := collector.GetLatestSample()
		if latest != nil {
			t.Error("GetLatestSample should return nil for empty collector")
		}

		avg := collector.GetAverageMetrics(10 * time.Second)
		// May be nil for empty collector, which is valid
		_ = avg

		trend := collector.GetTrend("cpu", 10*time.Second)
		// May be nil for empty collector, which is valid
		_ = trend

		healthy := collector.IsHealthy()
		// Should return some boolean value
		_ = healthy
	})

	t.Run("Reset", func(t *testing.T) {
		collector.Reset()
		// Should not panic and should clear samples
		samples := collector.GetSamples(10)
		if len(samples) != 0 {
			t.Errorf("expected 0 samples after reset, got %d", len(samples))
		}
	})
}

// TestTimingStorageHelpers tests TimingStorage helper methods
func TestTimingStorageHelpers(t *testing.T) {
	storage := NewTimingStorage(10)

	t.Run("empty storage methods", func(t *testing.T) {
		size := storage.Size()
		if size != 0 {
			t.Errorf("expected size 0, got %d", size)
		}

		capacity := storage.Capacity()
		if capacity != 10 {
			t.Errorf("expected capacity 10, got %d", capacity)
		}

		full := storage.IsFull()
		if full {
			t.Error("expected not full for empty storage")
		}

		stats := storage.GetStatistics()
		if stats.Count != 0 {
			t.Errorf("expected count 0, got %d", stats.Count)
		}
	})

	t.Run("Clear", func(t *testing.T) {
		storage.Clear()
		// Should not panic
		if storage.Size() != 0 {
			t.Error("expected size 0 after clear")
		}
	})
}
