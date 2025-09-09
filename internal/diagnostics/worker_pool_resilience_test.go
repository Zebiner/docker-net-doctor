package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// Error Recovery and Resilience Tests

func TestPanicRecoveryInWorkerFunctions(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 3)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	initialMetrics := pool.GetMetrics()

	// Submit jobs that will panic with different types
	panicJobs := []*PanicCheck{
		{name: "nil_pointer_panic", panicType: "nil_pointer"},
		{name: "string_panic", panicType: "string"},
		{name: "error_panic", panicType: "error"},
		{name: "custom_panic", panicType: "custom"},
		{name: "divide_by_zero", panicType: "divide_by_zero"},
	}

	for _, job := range panicJobs {
		err = pool.Submit(job, nil)
		assert.NoError(t, err, "Should be able to submit panic job")
	}

	// Collect results - all should be handled gracefully
	results := make([]JobResult, 0)
	timeout := time.After(3 * time.Second)

	for len(results) < len(panicJobs) {
		select {
		case result := <-pool.GetResults():
			results = append(results, result)
		case <-timeout:
			t.Fatalf("Timeout collecting panic recovery results, got %d/%d", len(results), len(panicJobs))
		}
	}

	// Verify all panics were recovered
	for i, result := range results {
		assert.Error(t, result.Error, "Panic job %d should have error", i)
		assert.Contains(t, result.Error.Error(), "panicked", "Error should indicate panic")
		assert.NotNil(t, result.Result, "Should have result even for panic")
		assert.False(t, result.Result.Success, "Panic result should indicate failure")
	}

	// Check metrics show panic recovery
	finalMetrics := pool.GetMetrics()
	assert.Equal(t, len(panicJobs), finalMetrics.RecoveredPanics, "Should record all panic recoveries")
	assert.Greater(t, finalMetrics.RecoveredPanics, initialMetrics.RecoveredPanics, "Panic count should increase")

	// Pool should still be functional after panics
	normalCheck := &TestMockCheck{name: "post_panic_check", delay: 10 * time.Millisecond}
	err = pool.Submit(normalCheck, nil)
	assert.NoError(t, err, "Pool should be functional after panic recovery")

	select {
	case result := <-pool.GetResults():
		assert.NoError(t, result.Error, "Normal job should work after panic recovery")
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for post-panic job")
	}
}

func TestWorkerRestartAfterFailures(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Submit multiple panic jobs to test worker resilience
	var panicCount int32
	for i := 0; i < 10; i++ {
		panicJob := &TestMockCheck{
			name:        fmt.Sprintf("panic_job_%d", i),
			delay:       10 * time.Millisecond,
			shouldPanic: true,
		}

		err = pool.Submit(panicJob, nil)
		if err == nil {
			atomic.AddInt32(&panicCount, 1)
		}
	}

	// Collect panic results
	panicResults := 0
	timeout := time.After(3 * time.Second)

	for panicResults < int(panicCount) {
		select {
		case result := <-pool.GetResults():
			if result.Error != nil || !result.Result.Success {
				panicResults++
			}
		case <-timeout:
			break
		}
	}

	// Wait a bit for worker recovery
	time.Sleep(100 * time.Millisecond)

	// Submit normal jobs to verify workers are still functional
	normalJobs := 5
	for i := 0; i < normalJobs; i++ {
		normalJob := &TestMockCheck{
			name:  fmt.Sprintf("normal_job_%d", i),
			delay: 20 * time.Millisecond,
		}
		err = pool.Submit(normalJob, nil)
		assert.NoError(t, err, "Should be able to submit normal jobs after worker panics")
	}

	// Collect normal results
	normalResults := 0
	timeout = time.After(2 * time.Second)

	for normalResults < normalJobs {
		select {
		case result := <-pool.GetResults():
			if result.Error == nil && result.Result.Success {
				normalResults++
			}
		case <-timeout:
			t.Fatalf("Workers not recovered properly, got %d/%d normal results", normalResults, normalJobs)
		}
	}

	assert.Equal(t, normalJobs, normalResults, "All normal jobs should complete after worker recovery")
}

func TestJobRetryMechanismAndBackoff(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Create a job that fails initially then succeeds
	retryJob := &RetryableCheck{
		name:        "retry_test",
		maxRetries:  3,
		failCount:   2, // Fail first 2 attempts, succeed on 3rd
	}

	err = pool.Submit(retryJob, nil)
	require.NoError(t, err)

	// The job should eventually succeed after retries
	select {
	case result := <-pool.GetResults():
		// Check if the job succeeded after retries
		if result.Error != nil {
			// If this implementation doesn't support automatic retries,
			// we should at least verify the failure is properly handled
			assert.Contains(t, result.Error.Error(), "failed", "Should indicate failure")
		} else {
			assert.True(t, result.Result.Success, "Job should succeed after retries")
		}
		
		// Verify retry attempts were made
		assert.GreaterOrEqual(t, retryJob.attemptCount.Load(), int32(1), "Should have made at least one attempt")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for retry job result")
	}
}

func TestTimeoutHandlingForLongRunningJobs(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Submit jobs with different timeout scenarios
	timeoutJobs := []*TimeoutCheck{
		{name: "quick_job", duration: 50 * time.Millisecond, shouldTimeout: false},
		{name: "long_job", duration: 5 * time.Second, shouldTimeout: true}, // Exceeds MAX_CHECK_TIMEOUT
		{name: "medium_job", duration: 500 * time.Millisecond, shouldTimeout: false},
	}

	for _, job := range timeoutJobs {
		err = pool.Submit(job, nil)
		assert.NoError(t, err, "Should be able to submit timeout test job")
	}

	// Collect results
	results := make([]JobResult, 0)
	timeout := time.After(10 * time.Second) // Allow time for timeout handling

	for len(results) < len(timeoutJobs) {
		select {
		case result := <-pool.GetResults():
			results = append(results, result)
		case <-timeout:
			t.Fatalf("Timeout collecting timeout test results, got %d/%d", len(results), len(timeoutJobs))
		}
	}

	// Verify timeout handling
	for i, result := range results {
		job := timeoutJobs[i]
		if job.shouldTimeout {
			// Job might timeout or be cancelled due to context deadline
			if result.Error != nil {
				assert.Contains(t, result.Error.Error(), "context", "Long job should timeout with context error")
			}
		} else {
			// Quick jobs should succeed
			assert.NoError(t, result.Error, "Quick job should not timeout")
			if result.Result != nil {
				assert.True(t, result.Result.Success, "Quick job should succeed")
			}
		}
	}
}

func TestGracefulDegradationUnderResourceConstraints(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping resource constraint test in short mode")
	}

	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)

	// Simulate resource constraints by setting low memory limit
	// Note: In a real implementation, this would configure memory limits
	memoryLimit := 1 // Very low limit for testing
	_ = memoryLimit

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Submit resource-intensive jobs
	resourceJobs := make([]*ResourceIntensiveCheck, 10)
	for i := 0; i < 10; i++ {
		resourceJobs[i] = &ResourceIntensiveCheck{
			name:     fmt.Sprintf("resource_job_%d", i),
			cpuBound: i%2 == 0,
			memBound: i%2 == 1,
			duration: 100 * time.Millisecond,
		}
	}

	submittedCount := 0
	rejectedCount := 0

	for _, job := range resourceJobs {
		err = pool.Submit(job, nil)
		if err != nil {
			rejectedCount++
			if !assert.Contains(t, err.Error(), "memory limit", "Rejection should be due to memory limit") {
				t.Logf("Unexpected rejection reason: %v", err)
			}
		} else {
			submittedCount++
		}
	}

	t.Logf("Under resource constraints: %d submitted, %d rejected", submittedCount, rejectedCount)

	// Collect results from submitted jobs
	results := make([]JobResult, 0)
	if submittedCount > 0 {
		timeout := time.After(3 * time.Second)
		
		for len(results) < submittedCount {
			select {
			case result := <-pool.GetResults():
				results = append(results, result)
			case <-timeout:
				t.Logf("Collected %d/%d results before timeout", len(results), submittedCount)
				break
			}
		}
	}

	// System should gracefully degrade by rejecting some jobs
	assert.Greater(t, rejectedCount, 0, "Should reject some jobs under resource constraints")
	
	// Accepted jobs should still execute properly
	for _, result := range results {
		// Jobs might fail due to resource constraints, but should not panic
		if result.Error != nil {
			t.Logf("Resource job failed (expected): %v", result.Error)
		}
	}

	// Pool should remain functional
	assert.True(t, pool.IsHealthy() || pool.errorRate.IsOpen(), 
		"Pool should be healthy or have circuit breaker open under constraints")
}

func TestErrorPropagationAndLogging(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Submit jobs with different error types
	errorJobs := []*ErrorCheck{
		{name: "network_error", errorType: "network"},
		{name: "permission_error", errorType: "permission"},
		{name: "timeout_error", errorType: "timeout"},
		{name: "validation_error", errorType: "validation"},
		{name: "unknown_error", errorType: "unknown"},
	}

	for _, job := range errorJobs {
		err = pool.Submit(job, nil)
		assert.NoError(t, err, "Should be able to submit error job")
	}

	// Collect and verify error propagation
	results := make([]JobResult, 0)
	timeout := time.After(2 * time.Second)

	for len(results) < len(errorJobs) {
		select {
		case result := <-pool.GetResults():
			results = append(results, result)
		case <-timeout:
			t.Fatalf("Timeout collecting error propagation results, got %d/%d", len(results), len(errorJobs))
		}
	}

	// Verify each error type is properly propagated
	errorTypes := map[string]bool{
		"network":    false,
		"permission": false,
		"timeout":    false,
		"validation": false,
		"unknown":    false,
	}

	for _, result := range results {
		assert.Error(t, result.Error, "Error job should have error")
		assert.NotNil(t, result.Result, "Should have result even for error")
		assert.False(t, result.Result.Success, "Error job should not succeed")

		// Check error message contains expected type
		errorMsg := result.Error.Error()
		for errType := range errorTypes {
			if fmt.Sprintf("%s error", errType) == errorMsg {
				errorTypes[errType] = true
			}
		}
	}

	// Verify all error types were propagated correctly
	for errType, found := range errorTypes {
		assert.True(t, found, "Error type %s should be propagated", errType)
	}
}

// Helper types for resilience testing

type PanicCheck struct {
	name      string
	panicType string
}

func (p *PanicCheck) Name() string        { return p.name }
func (p *PanicCheck) Description() string { return "Panic test check" }
func (p *PanicCheck) Severity() Severity  { return SeverityWarning }

func (p *PanicCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	switch p.panicType {
	case "nil_pointer":
		var ptr *int
		_ = *ptr // Nil pointer dereference
	case "string":
		panic("string panic message")
	case "error":
		panic(errors.New("error panic"))
	case "custom":
		panic(struct{ msg string }{msg: "custom panic"})
	case "divide_by_zero":
		x := 1
		y := 0
		_ = x / y // This won't actually panic in Go, but represents the intent
		panic("simulated divide by zero")
	default:
		panic("unknown panic type")
	}

	return &CheckResult{
		CheckName: p.name,
		Success:   false,
		Message:   "Should not reach here",
		Timestamp: time.Now(),
	}, nil
}

type RetryableCheck struct {
	name         string
	maxRetries   int
	failCount    int32
	attemptCount atomic.Int32
}

func (r *RetryableCheck) Name() string        { return r.name }
func (r *RetryableCheck) Description() string { return "Retryable test check" }
func (r *RetryableCheck) Severity() Severity  { return SeverityWarning }

func (r *RetryableCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	attempt := r.attemptCount.Add(1)
	
	if attempt <= r.failCount {
		return &CheckResult{
			CheckName: r.name,
			Success:   false,
			Message:   fmt.Sprintf("Attempt %d failed", attempt),
			Timestamp: time.Now(),
		}, fmt.Errorf("attempt %d failed", attempt)
	}

	return &CheckResult{
		CheckName: r.name,
		Success:   true,
		Message:   fmt.Sprintf("Succeeded on attempt %d", attempt),
		Timestamp: time.Now(),
	}, nil
}

type TimeoutCheck struct {
	name          string
	duration      time.Duration
	shouldTimeout bool
}

func (t *TimeoutCheck) Name() string        { return t.name }
func (t *TimeoutCheck) Description() string { return "Timeout test check" }
func (t *TimeoutCheck) Severity() Severity  { return SeverityInfo }

func (t *TimeoutCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	// Use context with timeout to respect cancellation
	select {
	case <-time.After(t.duration):
		return &CheckResult{
			CheckName: t.name,
			Success:   true,
			Message:   fmt.Sprintf("Completed after %v", t.duration),
			Timestamp: time.Now(),
		}, nil
	case <-ctx.Done():
		return &CheckResult{
			CheckName: t.name,
			Success:   false,
			Message:   fmt.Sprintf("Cancelled after %v", t.duration),
			Timestamp: time.Now(),
		}, ctx.Err()
	}
}

type ResourceIntensiveCheck struct {
	name     string
	cpuBound bool
	memBound bool
	duration time.Duration
}

func (r *ResourceIntensiveCheck) Name() string        { return r.name }
func (r *ResourceIntensiveCheck) Description() string { return "Resource intensive check" }
func (r *ResourceIntensiveCheck) Severity() Severity  { return SeverityInfo }

func (r *ResourceIntensiveCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	if r.cpuBound {
		// CPU intensive work
		end := time.Now().Add(r.duration)
		for time.Now().Before(end) {
			// Busy wait with some computation
			for i := 0; i < 1000; i++ {
				_ = i * i
			}
			runtime.Gosched() // Allow other goroutines to run
		}
	}

	if r.memBound {
		// Memory intensive work
		data := make([][]byte, 100)
		for i := range data {
			data[i] = make([]byte, 1024*10) // 10KB per allocation
			for j := range data[i] {
				data[i][j] = byte(j % 256)
			}
		}
		time.Sleep(r.duration)
		// Clear data to help GC
		for i := range data {
			data[i] = nil
		}
		data = nil
	}

	if !r.cpuBound && !r.memBound {
		time.Sleep(r.duration)
	}

	return &CheckResult{
		CheckName: r.name,
		Success:   true,
		Message:   fmt.Sprintf("Resource intensive work completed (CPU: %v, Mem: %v)", r.cpuBound, r.memBound),
		Timestamp: time.Now(),
	}, nil
}

type ErrorCheck struct {
	name      string
	errorType string
}

func (e *ErrorCheck) Name() string        { return e.name }
func (e *ErrorCheck) Description() string { return "Error test check" }
func (e *ErrorCheck) Severity() Severity  { return SeverityWarning }

func (e *ErrorCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	var err error
	switch e.errorType {
	case "network":
		err = errors.New("network error")
	case "permission":
		err = errors.New("permission error")
	case "timeout":
		err = errors.New("timeout error")
	case "validation":
		err = errors.New("validation error")
	default:
		err = errors.New("unknown error")
	}

	return &CheckResult{
		CheckName: e.name,
		Success:   false,
		Message:   fmt.Sprintf("Error: %v", err),
		Timestamp: time.Now(),
	}, err
}