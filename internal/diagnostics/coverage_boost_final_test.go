package diagnostics

import (
	"context"
	"testing"
	"time"
)

// TestEngineCreation tests engine creation (safer without running)
func TestEngineCreation(t *testing.T) {
	engine := NewEngine(nil, &Config{
		Parallel: true,
		Timeout:  5 * time.Second,
	})

	// Just test that the engine was created
	if engine == nil {
		t.Error("NewEngine should not return nil")
	}

	// Test the checks were registered
	if len(engine.checks) == 0 {
		t.Error("expected checks to be registered")
	}
}

// TestJobAndJobResultStructs tests Job and JobResult struct creation
func TestJobAndJobResultStructs(t *testing.T) {
	// Test Job struct
	job := Job{
		ID:      123,
		Check:   &DaemonConnectivityCheck{},
		Context: context.Background(),
		Client:  nil,
	}

	if job.ID != 123 {
		t.Errorf("expected ID=123, got %d", job.ID)
	}

	if job.Check == nil {
		t.Error("Check should not be nil")
	}

	if job.Context == nil {
		t.Error("Context should not be nil")
	}

	// Test JobResult struct
	result := JobResult{
		ID:       456,
		Result:   &CheckResult{CheckName: "test", Success: true},
		Error:    nil,
		Duration: 100 * time.Millisecond,
	}

	if result.ID != 456 {
		t.Errorf("expected ID=456, got %d", result.ID)
	}

	if result.Result == nil {
		t.Error("Result should not be nil")
	}

	if result.Duration != 100*time.Millisecond {
		t.Errorf("expected Duration=100ms, got %v", result.Duration)
	}
}

// TestBenchmarkHelpersNewBenchmarkMetrics tests the NewBenchmarkMetrics function
func TestBenchmarkHelpersNewBenchmarkMetrics(t *testing.T) {
	metrics := NewBenchmarkMetrics()
	if metrics == nil {
		t.Error("NewBenchmarkMetrics should not return nil")
	}
}

// TestBenchmarkHelpersNewMockCheck tests the NewMockCheck function
func TestBenchmarkHelpersNewMockCheck(t *testing.T) {
	mockCheck := NewMockCheck("test_check", 100*time.Millisecond, false)
	if mockCheck == nil {
		t.Error("NewMockCheck should not return nil")
	}

	// Test the mock check methods
	name := mockCheck.Name()
	if name != "test_check" {
		t.Errorf("expected name 'test_check', got %s", name)
	}

	desc := mockCheck.Description()
	if desc == "" {
		t.Error("Description should not be empty")
	}

	severity := mockCheck.Severity()
	if severity < SeverityInfo || severity > SeverityCritical {
		t.Errorf("Severity should be valid, got %v", severity)
	}
}

// TestBenchmarkHelpersGetDefaultCheckGroups tests the GetDefaultCheckGroups function
func TestBenchmarkHelpersGetDefaultCheckGroups(t *testing.T) {
	groups := GetDefaultCheckGroups()
	if groups == nil {
		t.Error("GetDefaultCheckGroups should not return nil")
	}
}

// TestConstantValues tests that constants are defined correctly
func TestConstantValues(t *testing.T) {
	if MAX_WORKERS <= 0 {
		t.Error("MAX_WORKERS should be positive")
	}

	if MAX_QUEUE_SIZE <= 0 {
		t.Error("MAX_QUEUE_SIZE should be positive")
	}

	if DOCKER_API_RATE_LIMIT <= 0 {
		t.Error("DOCKER_API_RATE_LIMIT should be positive")
	}

	if DOCKER_API_BURST <= 0 {
		t.Error("DOCKER_API_BURST should be positive")
	}
}

// TestBenchmarkHelpersRecordCheckDuration tests the RecordCheckDuration function
func TestBenchmarkHelpersRecordCheckDuration(t *testing.T) {
	metrics := NewBenchmarkMetrics()
	if metrics == nil {
		t.Fatal("NewBenchmarkMetrics should not return nil")
	}

	// Test RecordCheckDuration
	metrics.RecordCheckDuration("test_check", 100*time.Millisecond)
	// Should not panic
}

// TestBenchmarkHelpersGetCheckDuration tests the GetCheckDuration function
func TestBenchmarkHelpersGetCheckDuration(t *testing.T) {
	metrics := NewBenchmarkMetrics()
	if metrics == nil {
		t.Fatal("NewBenchmarkMetrics should not return nil")
	}

	// Record a duration first
	metrics.RecordCheckDuration("test_check", 100*time.Millisecond)

	// Test GetCheckDuration
	duration := metrics.GetCheckDuration("test_check")
	if duration == 0 {
		t.Error("expected non-zero duration")
	}
}

// TestBenchmarkHelpersCalculateParallelSpeedup tests the CalculateParallelSpeedup function
func TestBenchmarkHelpersCalculateParallelSpeedup(t *testing.T) {
	metrics := NewBenchmarkMetrics()
	if metrics == nil {
		t.Fatal("NewBenchmarkMetrics should not return nil")
	}

	// Test CalculateParallelSpeedup
	metrics.CalculateParallelSpeedup(1000*time.Millisecond, 500*time.Millisecond)
	// Should not panic
}

// TestRateLimiterMetricsFieldAccess tests RateLimiterMetrics field access
func TestRateLimiterMetricsFieldAccess(t *testing.T) {
	rl := NewRateLimiter(&RateLimiterConfig{
		RequestsPerSecond: 10.0,
		BurstSize:         5,
		Enabled:           true,
	})

	metrics := rl.GetMetrics()

	// Test that all fields are accessible
	_ = metrics.TotalRequests
	_ = metrics.AllowedRequests
	_ = metrics.ThrottledRequests
	_ = metrics.TimeoutRequests
	_ = metrics.TotalWaitTime
	_ = metrics.MaxWaitTime
	_ = metrics.LastRequest

	if metrics.TotalRequests < 0 {
		t.Error("TotalRequests should be non-negative")
	}
}

// TestRetryMetricsFieldAccess tests RetryMetrics field access
func TestRetryMetricsFieldAccess(t *testing.T) {
	rp := NewRetryPolicy(&RetryPolicyConfig{
		MaxRetries: 3,
		BaseDelay:  100 * time.Millisecond,
	})

	metrics := rp.GetMetrics()

	// Test that all fields are accessible
	_ = metrics.TotalRetries
	_ = metrics.SuccessfulRetries
	_ = metrics.FailedRetries
	_ = metrics.TotalRetryTime
	_ = metrics.AverageRetryTime
	_ = metrics.MaxRetryTime
	_ = metrics.TotalBackoffTime
	_ = metrics.AverageBackoffTime
	_ = metrics.MaxBackoffTime
	_ = metrics.RetriesByErrorType
	_ = metrics.RetriesByErrorCode
	_ = metrics.FirstAttemptSuccess
	_ = metrics.RetrySuccessRate

	if metrics.TotalRetries < 0 {
		t.Error("TotalRetries should be non-negative")
	}
}

// TestMoreDNSAndConnectivityChecks tests DNS and connectivity checks
func TestMoreDNSAndConnectivityChecks(t *testing.T) {
	checks := []Check{
		&DNSResolutionCheck{},
		&InternalDNSCheck{},
		&ContainerConnectivityCheck{},
		&PortBindingCheck{},
	}

	for _, check := range checks {
		t.Run(check.Name()+"_metadata", func(t *testing.T) {
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
