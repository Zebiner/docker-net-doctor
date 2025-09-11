package diagnostics

import (
	"context"
	"testing"
	"time"
)

// TestRateLimiterTryAcquire tests the TryAcquire method
func TestRateLimiterTryAcquire(t *testing.T) {
	rl := NewRateLimiter(&RateLimiterConfig{
		RequestsPerSecond: 10.0,
		BurstSize:         5,
		Enabled:           true,
	})

	// Should be able to acquire immediately with burst
	acquired := rl.TryAcquire()
	if !acquired {
		t.Error("should acquire token immediately with burst capacity")
	}
}

// TestRateLimiterReserveMethod tests the Reserve method (different name to avoid conflicts)
func TestRateLimiterReserveMethod(t *testing.T) {
	rl := NewRateLimiter(&RateLimiterConfig{
		RequestsPerSecond: 10.0,
		BurstSize:         5,
		Enabled:           true,
	})

	// Should be able to reserve
	reservation := rl.Reserve()
	if reservation == nil {
		t.Error("Reserve should return a reservation")
	}
}

// TestErrorRecoveryWithClient tests NewErrorRecovery with client
func TestErrorRecoveryWithClient(t *testing.T) {
	config := &RecoveryConfig{
		RecoveryTimeout:   30 * time.Second,
		EnableDegradation: true,
	}

	recovery := NewErrorRecovery(nil, config)
	if recovery == nil {
		t.Error("NewErrorRecovery should not return nil")
	}

	// Test GetMetrics
	metrics := recovery.GetMetrics()
	// Just verify we can call the method
	_ = metrics

	// Test IsHealthy
	healthy := recovery.IsHealthy()
	if !healthy {
		t.Error("new error recovery should be healthy")
	}

	// Test Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	recovery.Shutdown(ctx)
	// Should not panic
}

// TestGracefulDegradationWithClient tests NewGracefulDegradation
func TestGracefulDegradationWithClient(t *testing.T) {
config := &DegradationConfig{
		EnableAutoDetection:   false, // Disable to prevent background goroutine
		DefaultLevel:          DegradationNormal,
		HealthCheckInterval:   30 * time.Second,
		RecoveryCheckInterval: 60 * time.Second,
	}

	degradation := NewGracefulDegradation(config)
	if degradation == nil {
		t.Error("NewGracefulDegradation should not return nil")
	}

	// Test GetCurrentLevel
	level := degradation.GetCurrentLevel()
	// Should return a valid level
	_ = level

	// Test ClearManualOverride
	degradation.ClearManualOverride()
	// Should not panic

	// Test GetDegradationStatus
	status := degradation.GetDegradationStatus()
	// Should return some status
	_ = status

	// Test GetMetrics
	metrics := degradation.GetMetrics()
	// Should return some metrics
	_ = metrics
}

// TestNetworkChecksSubnetsOverlap tests the subnetsOverlap helper function indirectly
func TestNetworkChecksSubnetsOverlap(t *testing.T) {
	check := &SubnetOverlapCheck{}

	// Test metadata methods to ensure they execute
	name := check.Name()
	if name != "subnet_overlap" {
		t.Errorf("expected name 'subnet_overlap', got %s", name)
	}

	desc := check.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}

	severity := check.Severity()
	// Just verify we can call the method and get a valid severity
	if severity < SeverityInfo || severity > SeverityCritical {
		t.Errorf("expected valid severity, got %v", severity)
	}
}

// TestCheckResultConstruction tests CheckResult creation and field access
func TestCheckResultConstruction(t *testing.T) {
	result := &CheckResult{
		CheckName:   "test_check",
		Success:     true,
		Message:     "test message",
		Details:     map[string]interface{}{"key": "value"},
		Suggestions: []string{"suggestion1", "suggestion2"},
		Timestamp:   time.Now(),
		Duration:    100 * time.Millisecond,
	}

	if result.CheckName != "test_check" {
		t.Errorf("expected CheckName 'test_check', got %s", result.CheckName)
	}

	if !result.Success {
		t.Error("expected Success to be true")
	}

	if result.Message != "test message" {
		t.Errorf("expected Message 'test message', got %s", result.Message)
	}

	if len(result.Suggestions) != 2 {
		t.Errorf("expected 2 suggestions, got %d", len(result.Suggestions))
	}

	if result.Duration != 100*time.Millisecond {
		t.Errorf("expected Duration 100ms, got %v", result.Duration)
	}
}

// TestResultsConstruction tests Results creation and methods
func TestResultsConstruction(t *testing.T) {
	results := &Results{
		Checks: []*CheckResult{
			{CheckName: "check1", Success: true},
			{CheckName: "check2", Success: false},
		},
		Duration: 5 * time.Second,
	}

	if len(results.Checks) != 2 {
		t.Errorf("expected 2 checks, got %d", len(results.Checks))
	}

	if results.Duration != 5*time.Second {
		t.Errorf("expected Duration 5s, got %v", results.Duration)
	}
}

// TestSummaryConstruction tests Summary struct
func TestSummaryConstruction(t *testing.T) {
	summary := Summary{
		TotalChecks:    10,
		PassedChecks:   8,
		FailedChecks:   2,
		WarningChecks:  0,
		CriticalIssues: []string{"issue1", "issue2"},
	}

	if summary.TotalChecks != 10 {
		t.Errorf("expected TotalChecks 10, got %d", summary.TotalChecks)
	}

	if summary.PassedChecks != 8 {
		t.Errorf("expected PassedChecks 8, got %d", summary.PassedChecks)
	}

	if summary.FailedChecks != 2 {
		t.Errorf("expected FailedChecks 2, got %d", summary.FailedChecks)
	}

	if len(summary.CriticalIssues) != 2 {
		t.Errorf("expected 2 critical issues, got %d", len(summary.CriticalIssues))
	}
}

// TestExecutionMetrics tests ExecutionMetrics struct
func TestExecutionMetrics(t *testing.T) {
	metrics := &ExecutionMetrics{}

	// Test that we can access all fields without compilation errors
	metrics.TotalDuration = 10 * time.Second

	if metrics.TotalDuration != 10*time.Second {
		t.Errorf("expected TotalDuration 10s, got %v", metrics.TotalDuration)
	}
}

// TestBackoffStrategyValues tests BackoffStrategy enum values
func TestBackoffStrategyValues(t *testing.T) {
	strategies := []BackoffStrategy{
		LinearBackoff,
		ExponentialBackoff,
		JitteredBackoff,
	}

	// Test that enum values are distinct
	seen := make(map[BackoffStrategy]bool)
	for _, strategy := range strategies {
		if seen[strategy] {
			t.Errorf("duplicate strategy value: %v", strategy)
		}
		seen[strategy] = true
	}
}

// TestCircuitBreakerMetricsAccess tests CircuitBreakerMetrics field access
func TestCircuitBreakerMetricsAccess(t *testing.T) {
	cb := NewCircuitBreaker(nil)
	metrics := cb.GetMetrics()

	// Test that all fields are accessible
	_ = metrics.State
	_ = metrics.FailureCount
	_ = metrics.SuccessCount
	_ = metrics.HalfOpenRequestCount
	_ = metrics.ConsecutiveFailures
	_ = metrics.TotalRequests
	_ = metrics.TotalFailures
	_ = metrics.TotalSuccesses
	_ = metrics.LastFailTime
	_ = metrics.LastStateChange
	_ = metrics.LastRequestTime
	_ = metrics.StateTransitions
	_ = metrics.FailureRate
	_ = metrics.RequestRate
	_ = metrics.TimeInCurrentState
	_ = metrics.IsHealthy
	_ = metrics.EstimatedRecoveryTime

	// Verify some basic properties
	if metrics.State != CircuitStateClosed {
		t.Errorf("expected initial state to be CLOSED, got %v", metrics.State)
	}

	if metrics.IsHealthy != true {
		t.Error("expected new circuit breaker to be healthy")
	}
}

// TestProfileMetricsAccess tests ProfileMetrics field access
func TestProfileMetricsAccess(t *testing.T) {
	profiler := NewPerformanceProfiler(&ProfileConfig{Enabled: true})
	metrics := profiler.GetMetrics()

	// Test basic field access
	_ = metrics.TotalOperations
	_ = metrics.TotalDuration

	if metrics.TotalOperations < 0 {
		t.Error("TotalOperations should be non-negative")
	}
}

// TestDiagnosticErrorFieldAccess tests DiagnosticError field access
func TestDiagnosticErrorFieldAccess(t *testing.T) {
	err := NewDiagnosticError(ErrCodeGeneric, "test error")

	// Test field access
	if err.Code != ErrCodeGeneric {
		t.Errorf("expected Code to be ErrCodeGeneric, got %v", err.Code)
	}

	if err.Message != "test error" {
		t.Errorf("expected Message to be 'test error', got %s", err.Message)
	}

	// Test that all other fields are accessible
	_ = err.Type
	_ = err.Original
	_ = err.Severity
	_ = err.Category
	_ = err.Context
	_ = err.Timestamp
	_ = err.Source
	_ = err.Recovery
	_ = err.RecoveryAttempted
	_ = err.RecoveryError
	_ = err.Tags
	_ = err.UserAction
	_ = err.Reference
}

// TestMoreCheckTypes tests additional check types for Name, Description, Severity
func TestMoreCheckTypes(t *testing.T) {
	checks := []Check{
		&IPForwardingCheck{},
		&IptablesCheck{},
		&MTUConsistencyCheck{},
	}

	for _, check := range checks {
		t.Run(check.Name()+"_methods", func(t *testing.T) {
			// Test Name method
			name := check.Name()
			if name == "" {
				t.Error("Name() should not return empty string")
			}

			// Test Description method
			description := check.Description()
			if description == "" {
				t.Error("Description() should not return empty string")
			}

			// Test Severity method
			severity := check.Severity()
			if severity < SeverityInfo || severity > SeverityCritical {
				t.Errorf("Severity() returned invalid value: %v", severity)
			}
		})
	}
}
