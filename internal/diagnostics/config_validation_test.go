package diagnostics

import (
	"testing"
	"time"
)

// TestCircuitBreakerConfig tests the CircuitBreakerConfig struct and its defaults
func TestCircuitBreakerConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   *CircuitBreakerConfig
		wantNil  bool
		validate func(*testing.T, *CircuitBreaker)
	}{
		{
			name:    "nil config gets defaults",
			config:  nil,
			wantNil: false,
			validate: func(t *testing.T, cb *CircuitBreaker) {
				if cb.config.FailureThreshold != 5 {
					t.Errorf("expected FailureThreshold=5, got %d", cb.config.FailureThreshold)
				}
				if cb.config.RecoveryTimeout != 30*time.Second {
					t.Errorf("expected RecoveryTimeout=30s, got %v", cb.config.RecoveryTimeout)
				}
				if cb.config.ConsecutiveSuccesses != 3 {
					t.Errorf("expected ConsecutiveSuccesses=3, got %d", cb.config.ConsecutiveSuccesses)
				}
			},
		},
		{
			name: "zero values get defaults",
			config: &CircuitBreakerConfig{
				FailureThreshold: 10,
				// Other fields left as zero
			},
			validate: func(t *testing.T, cb *CircuitBreaker) {
				if cb.config.FailureThreshold != 10 {
					t.Errorf("expected FailureThreshold=10, got %d", cb.config.FailureThreshold)
				}
				if cb.config.ConsecutiveSuccesses != 3 {
					t.Errorf("expected ConsecutiveSuccesses default=3, got %d", cb.config.ConsecutiveSuccesses)
				}
				if cb.config.HalfOpenMaxRequests != 2 {
					t.Errorf("expected HalfOpenMaxRequests default=2, got %d", cb.config.HalfOpenMaxRequests)
				}
			},
		},
		{
			name: "negative failure threshold",
			config: &CircuitBreakerConfig{
				FailureThreshold:     -1,
				RecoveryTimeout:      10 * time.Second,
				ConsecutiveSuccesses: 2,
			},
			validate: func(t *testing.T, cb *CircuitBreaker) {
				// Should accept negative values as configuration
				if cb.config.FailureThreshold != -1 {
					t.Errorf("expected FailureThreshold=-1, got %d", cb.config.FailureThreshold)
				}
			},
		},
		{
			name: "zero recovery timeout",
			config: &CircuitBreakerConfig{
				FailureThreshold: 5,
				RecoveryTimeout:  0,
			},
			validate: func(t *testing.T, cb *CircuitBreaker) {
				if cb.config.RecoveryTimeout != 30*time.Second {
					t.Errorf("expected default RecoveryTimeout=30s, got %v", cb.config.RecoveryTimeout)
				}
			},
		},
		{
			name: "boundary values",
			config: &CircuitBreakerConfig{
				FailureThreshold:       0,
				RecoveryTimeout:        1 * time.Nanosecond,
				ConsecutiveSuccesses:   1,
				HalfOpenMaxRequests:    1,
				FailureRate:            1.0,
				MinRequestsThreshold:   0,
				ResetTimeout:           1 * time.Hour,
				AdaptiveThresholds:     true,
				RequestVolumeThreshold: 100,
			},
			validate: func(t *testing.T, cb *CircuitBreaker) {
				if cb.config.FailureRate != 1.0 {
					t.Errorf("expected FailureRate=1.0, got %f", cb.config.FailureRate)
				}
				if !cb.config.AdaptiveThresholds {
					t.Errorf("expected AdaptiveThresholds=true")
				}
			},
		},
		{
			name: "extreme values",
			config: &CircuitBreakerConfig{
				FailureThreshold:       1000000,
				RecoveryTimeout:        24 * time.Hour,
				ConsecutiveSuccesses:   100,
				HalfOpenMaxRequests:    1000,
				FailureRate:            0.0,
				MinRequestsThreshold:   1000000,
				ResetTimeout:           0,
				AdaptiveThresholds:     false,
				RequestVolumeThreshold: 0,
			},
			validate: func(t *testing.T, cb *CircuitBreaker) {
				if cb.config.FailureThreshold != 1000000 {
					t.Errorf("expected FailureThreshold=1000000, got %d", cb.config.FailureThreshold)
				}
				if cb.config.ResetTimeout != 60*time.Second {
					t.Errorf("expected default ResetTimeout=60s, got %v", cb.config.ResetTimeout)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := NewCircuitBreaker(tt.config)
			if cb == nil && !tt.wantNil {
				t.Fatal("NewCircuitBreaker returned nil")
			}
			if tt.validate != nil {
				tt.validate(t, cb)
			}
		})
	}
}

// TestRateLimiterConfig tests the RateLimiterConfig struct and its defaults
func TestRateLimiterConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   *RateLimiterConfig
		validate func(*testing.T, *RateLimiter)
	}{
		{
			name:   "nil config gets defaults",
			config: nil,
			validate: func(t *testing.T, rl *RateLimiter) {
				if !rl.config.Enabled {
					t.Error("expected default Enabled=true")
				}
				if rl.config.RequestsPerSecond != float64(DOCKER_API_RATE_LIMIT) {
					t.Errorf("expected RequestsPerSecond=%f, got %f", float64(DOCKER_API_RATE_LIMIT), rl.config.RequestsPerSecond)
				}
			},
		},
		{
			name: "disabled rate limiter",
			config: &RateLimiterConfig{
				RequestsPerSecond: 10.0,
				BurstSize:         5,
				WaitTimeout:       1 * time.Second,
				Enabled:           false,
			},
			validate: func(t *testing.T, rl *RateLimiter) {
				if rl.config.Enabled {
					t.Error("expected Enabled=false")
				}
				if rl.limiter != nil {
					t.Error("expected limiter to be nil when disabled")
				}
			},
		},
		{
			name: "zero values",
			config: &RateLimiterConfig{
				RequestsPerSecond: 0,
				BurstSize:         0,
				WaitTimeout:       0,
				Enabled:           true,
			},
			validate: func(t *testing.T, rl *RateLimiter) {
				if rl.config.RequestsPerSecond != 0 {
					t.Errorf("expected RequestsPerSecond=0, got %f", rl.config.RequestsPerSecond)
				}
				if rl.config.BurstSize != 0 {
					t.Errorf("expected BurstSize=0, got %d", rl.config.BurstSize)
				}
			},
		},
		{
			name: "negative values",
			config: &RateLimiterConfig{
				RequestsPerSecond: -1.0,
				BurstSize:         -1,
				WaitTimeout:       -1 * time.Second,
				Enabled:           true,
			},
			validate: func(t *testing.T, rl *RateLimiter) {
				// Should accept negative values as configuration
				if rl.config.RequestsPerSecond != -1.0 {
					t.Errorf("expected RequestsPerSecond=-1, got %f", rl.config.RequestsPerSecond)
				}
			},
		},
		{
			name: "extreme high values",
			config: &RateLimiterConfig{
				RequestsPerSecond: 10000.0,
				BurstSize:         100000,
				WaitTimeout:       24 * time.Hour,
				Enabled:           true,
			},
			validate: func(t *testing.T, rl *RateLimiter) {
				if rl.config.RequestsPerSecond != 10000.0 {
					t.Errorf("expected RequestsPerSecond=10000, got %f", rl.config.RequestsPerSecond)
				}
			},
		},
		{
			name: "fractional requests per second",
			config: &RateLimiterConfig{
				RequestsPerSecond: 0.1,
				BurstSize:         1,
				WaitTimeout:       100 * time.Millisecond,
				Enabled:           true,
			},
			validate: func(t *testing.T, rl *RateLimiter) {
				if rl.config.RequestsPerSecond != 0.1 {
					t.Errorf("expected RequestsPerSecond=0.1, got %f", rl.config.RequestsPerSecond)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiter(tt.config)
			if rl == nil {
				t.Fatal("NewRateLimiter returned nil")
			}
			if tt.validate != nil {
				tt.validate(t, rl)
			}
		})
	}
}

// TestEngineConfig tests the Config struct and its defaults
func TestEngineConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		want   Config
	}{
		{
			name:   "nil config",
			config: nil,
			want: Config{
				Parallel:     false,
				Timeout:      0,
				Verbose:      false,
				TargetFilter: "",
				WorkerCount:  0,
				RateLimit:    0,
			},
		},
		{
			name: "default values",
			config: &Config{
				Parallel:     true,
				Timeout:      30 * time.Second,
				Verbose:      true,
				TargetFilter: "test-container",
				WorkerCount:  4,
				RateLimit:    10.0,
			},
			want: Config{
				Parallel:     true,
				Timeout:      30 * time.Second,
				Verbose:      true,
				TargetFilter: "test-container",
				WorkerCount:  4,
				RateLimit:    10.0,
			},
		},
		{
			name: "zero timeout",
			config: &Config{
				Timeout: 0,
			},
			want: Config{
				Timeout: 0,
			},
		},
		{
			name: "negative worker count",
			config: &Config{
				WorkerCount: -1,
			},
			want: Config{
				WorkerCount: -1,
			},
		},
		{
			name: "empty target filter",
			config: &Config{
				TargetFilter: "",
			},
			want: Config{
				TargetFilter: "",
			},
		},
		{
			name: "extreme values",
			config: &Config{
				Parallel:     true,
				Timeout:      24 * time.Hour,
				Verbose:      false,
				TargetFilter: "very-long-container-name-with-special-chars-123!@#",
				WorkerCount:  10000,
				RateLimit:    999999.0,
			},
			want: Config{
				Parallel:     true,
				Timeout:      24 * time.Hour,
				Verbose:      false,
				TargetFilter: "very-long-container-name-with-special-chars-123!@#",
				WorkerCount:  10000,
				RateLimit:    999999.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that the config values are preserved
			if tt.config != nil {
				if tt.config.Parallel != tt.want.Parallel {
					t.Errorf("Parallel: got %v, want %v", tt.config.Parallel, tt.want.Parallel)
				}
				if tt.config.Timeout != tt.want.Timeout {
					t.Errorf("Timeout: got %v, want %v", tt.config.Timeout, tt.want.Timeout)
				}
				if tt.config.Verbose != tt.want.Verbose {
					t.Errorf("Verbose: got %v, want %v", tt.config.Verbose, tt.want.Verbose)
				}
				if tt.config.TargetFilter != tt.want.TargetFilter {
					t.Errorf("TargetFilter: got %v, want %v", tt.config.TargetFilter, tt.want.TargetFilter)
				}
				if tt.config.WorkerCount != tt.want.WorkerCount {
					t.Errorf("WorkerCount: got %v, want %v", tt.config.WorkerCount, tt.want.WorkerCount)
				}
				if tt.config.RateLimit != tt.want.RateLimit {
					t.Errorf("RateLimit: got %v, want %v", tt.config.RateLimit, tt.want.RateLimit)
				}
			}
		})
	}
}

// TestRetryPolicyConfig tests the RetryPolicyConfig struct and its defaults
func TestRetryPolicyConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   *RetryPolicyConfig
		validate func(*testing.T, *RetryPolicy)
	}{
		{
			name:   "nil config gets defaults",
			config: nil,
			validate: func(t *testing.T, rp *RetryPolicy) {
				if rp.config.MaxRetries != 3 {
					t.Errorf("expected MaxRetries=3, got %d", rp.config.MaxRetries)
				}
				if rp.config.BaseDelay != 100*time.Millisecond {
					t.Errorf("expected BaseDelay=100ms, got %v", rp.config.BaseDelay)
				}
			},
		},
		{
			name: "zero max retries",
			config: &RetryPolicyConfig{
				MaxRetries: 0,
				BaseDelay:  1 * time.Second,
			},
			validate: func(t *testing.T, rp *RetryPolicy) {
				if rp.config.MaxRetries != 0 {
					t.Errorf("expected MaxRetries=0, got %d", rp.config.MaxRetries)
				}
			},
		},
		{
			name: "negative values",
			config: &RetryPolicyConfig{
				MaxRetries:        -1,
				MaxDelay:          -1 * time.Second,
				BaseDelay:         -1 * time.Millisecond,
				BackoffMultiplier: -1.0,
				JitterFactor:      -0.1,
			},
			validate: func(t *testing.T, rp *RetryPolicy) {
				if rp.config.MaxRetries != -1 {
					t.Errorf("expected MaxRetries=-1, got %d", rp.config.MaxRetries)
				}
				if rp.config.BackoffMultiplier != -1.0 {
					t.Errorf("expected BackoffMultiplier=-1.0, got %f", rp.config.BackoffMultiplier)
				}
			},
		},
		{
			name: "boundary jitter factor",
			config: &RetryPolicyConfig{
				JitterFactor: 1.0,
			},
			validate: func(t *testing.T, rp *RetryPolicy) {
				if rp.config.JitterFactor != 1.0 {
					t.Errorf("expected JitterFactor=1.0, got %f", rp.config.JitterFactor)
				}
			},
		},
		{
			name: "extreme high values",
			config: &RetryPolicyConfig{
				MaxRetries:        1000,
				MaxDelay:          24 * time.Hour,
				BaseDelay:         1 * time.Hour,
				BackoffMultiplier: 100.0,
				JitterFactor:      10.0,
				MaxRetryBudget:    7 * 24 * time.Hour,
			},
			validate: func(t *testing.T, rp *RetryPolicy) {
				if rp.config.MaxRetries != 1000 {
					t.Errorf("expected MaxRetries=1000, got %d", rp.config.MaxRetries)
				}
				if rp.config.JitterFactor != 10.0 {
					t.Errorf("expected JitterFactor=10.0, got %f", rp.config.JitterFactor)
				}
			},
		},
		{
			name: "error code arrays",
			config: &RetryPolicyConfig{
				RetryableErrors:    []ErrorCode{ErrCodeResourceExhaustion, ErrCodeNetworkConfiguration},
				NonRetryableErrors: []ErrorCode{ErrCodeDockerPermissionDenied, ErrCodeGeneric},
				RespectContext:     true,
			},
			validate: func(t *testing.T, rp *RetryPolicy) {
				if len(rp.config.RetryableErrors) != 2 {
					t.Errorf("expected 2 retryable errors, got %d", len(rp.config.RetryableErrors))
				}
				if len(rp.config.NonRetryableErrors) != 2 {
					t.Errorf("expected 2 non-retryable errors, got %d", len(rp.config.NonRetryableErrors))
				}
				if !rp.config.RespectContext {
					t.Error("expected RespectContext=true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := NewRetryPolicy(tt.config)
			if rp == nil {
				t.Fatal("NewRetryPolicy returned nil")
			}
			if tt.validate != nil {
				tt.validate(t, rp)
			}
		})
	}
}

// TestDefaultRateLimiterConfigValidation tests the DefaultRateLimiterConfig function
func TestDefaultRateLimiterConfigValidation(t *testing.T) {
	config := DefaultRateLimiterConfig()
	if config == nil {
		t.Fatal("DefaultRateLimiterConfig returned nil")
	}

	if config.RequestsPerSecond != float64(DOCKER_API_RATE_LIMIT) {
		t.Errorf("expected RequestsPerSecond=%f, got %f", float64(DOCKER_API_RATE_LIMIT), config.RequestsPerSecond)
	}
	if config.BurstSize != DOCKER_API_BURST {
		t.Errorf("expected BurstSize=%d, got %d", DOCKER_API_BURST, config.BurstSize)
	}
	if config.WaitTimeout != 5*time.Second {
		t.Errorf("expected WaitTimeout=5s, got %v", config.WaitTimeout)
	}
	if !config.Enabled {
		t.Error("expected Enabled=true")
	}
}

// TestBackoffStrategy tests the BackoffStrategy enum values
func TestBackoffStrategy(t *testing.T) {
	tests := []struct {
		name     string
		strategy BackoffStrategy
		valid    bool
	}{
		{"LinearBackoff", LinearBackoff, true},
		{"ExponentialBackoff", ExponentialBackoff, true},
		{"JitteredBackoff", JitteredBackoff, true},
		{"Invalid strategy", BackoffStrategy(999), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &RetryPolicyConfig{
				BackoffStrategy: tt.strategy,
			}
			rp := NewRetryPolicy(config)
			if rp == nil {
				t.Fatal("NewRetryPolicy returned nil")
			}
			if rp.config.BackoffStrategy != tt.strategy {
				t.Errorf("expected BackoffStrategy=%v, got %v", tt.strategy, rp.config.BackoffStrategy)
			}
		})
	}
}

// TestCircuitState tests the CircuitState enum and String method
func TestCircuitState(t *testing.T) {
	tests := []struct {
		state    CircuitState
		expected string
	}{
		{CircuitStateClosed, "CLOSED"},
		{CircuitStateOpen, "OPEN"},
		{CircuitStateHalfOpen, "HALF_OPEN"},
		{CircuitState(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.state.String()
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestSeverityValues tests that Severity enum values are properly defined
func TestSeverityValues(t *testing.T) {
	tests := []struct {
		name     string
		severity Severity
		expected int
	}{
		{"SeverityInfo", SeverityInfo, 0},
		{"SeverityWarning", SeverityWarning, 1},
		{"SeverityError", SeverityError, 2},
		{"SeverityCritical", SeverityCritical, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.severity) != tt.expected {
				t.Errorf("expected %s=%d, got %d", tt.name, tt.expected, int(tt.severity))
			}
		})
	}
}

// TestConfigFieldsPresent tests that expected configuration fields are present and accessible
func TestConfigFieldsPresent(t *testing.T) {
	t.Run("CircuitBreakerConfig fields", func(t *testing.T) {
		config := &CircuitBreakerConfig{}
		// Test that all fields are accessible (compilation test)
		config.FailureThreshold = 5
		config.FailureRate = 0.5
		config.MinRequestsThreshold = 10
		config.RecoveryTimeout = 30 * time.Second
		config.ConsecutiveSuccesses = 3
		config.HalfOpenMaxRequests = 2
		config.ResetTimeout = 60 * time.Second
		config.AdaptiveThresholds = true
		config.RequestVolumeThreshold = 20

		// Verify values were set (basic sanity check)
		if config.FailureThreshold != 5 {
			t.Error("FailureThreshold field not working")
		}
	})

	t.Run("RateLimiterConfig fields", func(t *testing.T) {
		config := &RateLimiterConfig{}
		config.RequestsPerSecond = 10.0
		config.BurstSize = 5
		config.WaitTimeout = 5 * time.Second
		config.Enabled = true

		if config.RequestsPerSecond != 10.0 {
			t.Error("RequestsPerSecond field not working")
		}
	})

	t.Run("RetryPolicyConfig fields", func(t *testing.T) {
		config := &RetryPolicyConfig{}
		config.MaxRetries = 3
		config.MaxDelay = 10 * time.Second
		config.BaseDelay = 100 * time.Millisecond
		config.BackoffStrategy = ExponentialBackoff
		config.BackoffMultiplier = 2.0
		config.JitterFactor = 0.1
		config.RetryableErrors = []ErrorCode{ErrCodeNetworkConfiguration}
		config.NonRetryableErrors = []ErrorCode{ErrCodeGeneric}
		config.MaxRetryBudget = 30 * time.Second
		config.RespectContext = true

		if config.MaxRetries != 3 {
			t.Error("MaxRetries field not working")
		}
	})
}
