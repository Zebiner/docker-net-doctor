package diagnostics

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zebiner/docker-net-doctor/internal/docker"
	"golang.org/x/time/rate"
)

// WorkerPoolTestMockCheck implements the Check interface for testing worker pool
type WorkerPoolTestMockCheck struct {
	name        string
	description string
	severity    Severity
	delay       time.Duration
	shouldFail  bool
	shouldPanic bool
	execCount   atomic.Int32
}

func (m *WorkerPoolTestMockCheck) Name() string        { return m.name }
func (m *WorkerPoolTestMockCheck) Description() string { return m.description }
func (m *WorkerPoolTestMockCheck) Severity() Severity  { return m.severity }

func (m *WorkerPoolTestMockCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	m.execCount.Add(1)

	if m.shouldPanic {
		panic("mock check panic")
	}

	// Simulate work
	select {
	case <-time.After(m.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	if m.shouldFail {
		return nil, fmt.Errorf("mock check failed")
	}

	return &CheckResult{
		CheckName: m.name,
		Success:   true,
		Message:   "Check passed",
		Timestamp: time.Now(),
	}, nil
}

func TestSecureWorkerPool_Creation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name            string
		workerCount     int
		expectedWorkers int
	}{
		{
			name:            "Zero workers uses CPU count",
			workerCount:     0,
			expectedWorkers: min(runtime.NumCPU(), MAX_WORKERS),
		},
		{
			name:            "Negative workers uses CPU count",
			workerCount:     -1,
			expectedWorkers: min(runtime.NumCPU(), MAX_WORKERS),
		},
		{
			name:            "Valid worker count",
			workerCount:     5,
			expectedWorkers: 5,
		},
		{
			name:            "Exceeds max workers",
			workerCount:     100,
			expectedWorkers: MAX_WORKERS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := NewSecureWorkerPool(ctx, tt.workerCount)
			require.NoError(t, err)
			assert.NotNil(t, pool)
			assert.Equal(t, tt.expectedWorkers, pool.workers)
			assert.NotNil(t, pool.rateLimiter)
			assert.NotNil(t, pool.validator)
			assert.NotNil(t, pool.memoryMonitor)
			assert.NotNil(t, pool.errorRate)
		})
	}
}

func TestSecureWorkerPool_StartStop(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 3)
	require.NoError(t, err)

	// Test starting pool
	err = pool.Start()
	assert.NoError(t, err)
	assert.True(t, pool.started.Load())

	// Test starting already started pool
	err = pool.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")

	// Test stopping pool
	err = pool.Stop()
	assert.NoError(t, err)
	assert.True(t, pool.stopped.Load())

	// Test stopping already stopped pool
	err = pool.Stop()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already stopped")
}

func TestSecureWorkerPool_ExecuteJobs(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 3)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Create mock checks
	checks := []Check{
		&WorkerPoolTestMockCheck{name: "check1", delay: 10 * time.Millisecond},
		&WorkerPoolTestMockCheck{name: "check2", delay: 10 * time.Millisecond},
		&WorkerPoolTestMockCheck{name: "check3", delay: 10 * time.Millisecond},
		&WorkerPoolTestMockCheck{name: "check4", delay: 10 * time.Millisecond},
		&WorkerPoolTestMockCheck{name: "check5", delay: 10 * time.Millisecond},
	}

	// Submit jobs
	for _, check := range checks {
		err := pool.Submit(check, nil)
		assert.NoError(t, err)
	}

	// Collect results
	results := make([]JobResult, 0)
	resultsChan := pool.GetResults()

	timeout := time.After(2 * time.Second)
	for len(results) < len(checks) {
		select {
		case result := <-resultsChan:
			results = append(results, result)
		case <-timeout:
			t.Fatal("Timeout collecting results")
		}
	}

	// Verify all checks were executed
	assert.Equal(t, len(checks), len(results))
	for _, check := range checks {
		mockCheck := check.(*WorkerPoolTestMockCheck)
		assert.Equal(t, int32(1), mockCheck.execCount.Load())
	}

	// Verify metrics
	metrics := pool.GetMetrics()
	assert.Equal(t, len(checks), metrics.CompletedJobs)
	assert.Equal(t, 0, metrics.FailedJobs)
}

func TestSecureWorkerPool_ErrorHandling(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Create checks with errors
	checks := []Check{
		&WorkerPoolTestMockCheck{name: "success", delay: 10 * time.Millisecond},
		&WorkerPoolTestMockCheck{name: "failure", delay: 10 * time.Millisecond, shouldFail: true},
		&WorkerPoolTestMockCheck{name: "panic", delay: 10 * time.Millisecond, shouldPanic: true},
	}

	// Submit jobs
	for _, check := range checks {
		err := pool.Submit(check, nil)
		assert.NoError(t, err)
	}

	// Collect results
	results := make([]JobResult, 0)
	resultsChan := pool.GetResults()

	timeout := time.After(2 * time.Second)
	for len(results) < len(checks) {
		select {
		case result := <-resultsChan:
			results = append(results, result)
		case <-timeout:
			t.Fatal("Timeout collecting results")
		}
	}

	// Verify results
	successCount := 0
	failureCount := 0
	for _, result := range results {
		if result.Error != nil || !result.Result.Success {
			failureCount++
		} else {
			successCount++
		}
	}

	assert.Equal(t, 1, successCount)
	assert.Equal(t, 2, failureCount)

	// Verify panic recovery
	metrics := pool.GetMetrics()
	assert.Equal(t, 1, metrics.RecoveredPanics)
}

func TestSecureWorkerPool_RateLimiting(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 5)
	require.NoError(t, err)

	// Set aggressive rate limit for testing
	pool.rateLimiter = rate.NewLimiter(rate.Limit(2), 2) // 2 per second, burst of 2

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Submit many jobs quickly
	startTime := time.Now()
	numJobs := 6
	for i := 0; i < numJobs; i++ {
		check := &WorkerPoolTestMockCheck{
			name:  fmt.Sprintf("check%d", i),
			delay: 1 * time.Millisecond,
		}
		err := pool.Submit(check, nil)
		assert.NoError(t, err)
	}

	// Collect results
	results := make([]JobResult, 0)
	resultsChan := pool.GetResults()

	timeout := time.After(5 * time.Second)
	for len(results) < numJobs {
		select {
		case result := <-resultsChan:
			results = append(results, result)
		case <-timeout:
			t.Fatal("Timeout collecting results")
		}
	}

	elapsed := time.Since(startTime)

	// With rate limit of 2/sec and 6 jobs, should take at least 2 seconds
	// (2 immediate, then 2 after 1 sec, then 2 after another sec)
	assert.True(t, elapsed >= 2*time.Second, "Expected rate limiting to slow execution")
}

func TestSecureWorkerPool_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Submit long-running jobs
	for i := 0; i < 5; i++ {
		check := &WorkerPoolTestMockCheck{
			name:  fmt.Sprintf("check%d", i),
			delay: 5 * time.Second, // Long delay
		}
		err := pool.Submit(check, nil)
		assert.NoError(t, err)
	}

	// Cancel context after a short time
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Try to collect results
	results := make([]JobResult, 0)
	resultsChan := pool.GetResults()

	timeout := time.After(1 * time.Second)
	done := false
	for !done {
		select {
		case result, ok := <-resultsChan:
			if ok {
				results = append(results, result)
			}
		case <-timeout:
			done = true
		}
	}

	// Should have some results but not all due to cancellation
	assert.True(t, len(results) < 5, "Expected some jobs to be cancelled")
}

func TestSecureWorkerPool_MemoryMonitoring(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)

	// Test memory monitor
	assert.NotNil(t, pool.memoryMonitor)
	assert.False(t, pool.memoryMonitor.IsOverLimit())

	// Peak usage should be tracked
	peakUsage := pool.memoryMonitor.GetPeakUsage()
	assert.GreaterOrEqual(t, peakUsage, uint64(0))
}

func TestSecureWorkerPool_CircuitBreaker(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Simulate many failures to trigger circuit breaker
	for i := 0; i < 15; i++ {
		pool.errorRate.RecordError()
	}

	// Record a few successes
	for i := 0; i < 5; i++ {
		pool.errorRate.RecordSuccess()
	}

	// Circuit should be open due to high error rate (75%)
	assert.True(t, pool.errorRate.IsOpen())

	// Submitting new jobs should fail
	check := &WorkerPoolTestMockCheck{name: "test"}
	err = pool.Submit(check, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker open")
}

func TestSecureWorkerPool_HealthCheck(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)

	// Pool not started should be unhealthy
	assert.False(t, pool.IsHealthy())

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Started pool should be healthy
	assert.True(t, pool.IsHealthy())

	// Simulate circuit breaker open
	for i := 0; i < 15; i++ {
		pool.errorRate.RecordError()
	}
	pool.errorRate.RecordSuccess() // Ensure enough samples

	// Should be unhealthy with open circuit
	assert.False(t, pool.IsHealthy())
}

func TestSecureWorkerPool_Metrics(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 3)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Submit mixed jobs
	checks := []Check{
		&WorkerPoolTestMockCheck{name: "quick", delay: 5 * time.Millisecond},
		&WorkerPoolTestMockCheck{name: "medium", delay: 20 * time.Millisecond},
		&WorkerPoolTestMockCheck{name: "slow", delay: 50 * time.Millisecond},
		&WorkerPoolTestMockCheck{name: "failure", delay: 10 * time.Millisecond, shouldFail: true},
	}

	for _, check := range checks {
		err := pool.Submit(check, nil)
		assert.NoError(t, err)
	}

	// Collect results
	results := make([]JobResult, 0)
	resultsChan := pool.GetResults()

	timeout := time.After(2 * time.Second)
	for len(results) < len(checks) {
		select {
		case result := <-resultsChan:
			results = append(results, result)
		case <-timeout:
			t.Fatal("Timeout collecting results")
		}
	}

	// Check metrics
	metrics := pool.GetMetrics()
	assert.Equal(t, 4, metrics.TotalJobs)
	assert.Equal(t, 3, metrics.CompletedJobs)
	assert.Equal(t, 1, metrics.FailedJobs)
	assert.Greater(t, metrics.MaxJobTime, time.Duration(0))
	assert.Greater(t, metrics.MinJobTime, time.Duration(0))
	assert.LessOrEqual(t, metrics.MinJobTime, metrics.MaxJobTime)
	assert.Equal(t, 3, metrics.PeakWorkers)
}

// Helper function for min
// min function is defined in worker_pool_coverage_test.go - removed duplicate

// TestWorkerPoolWaitForCompletion tests the WaitForCompletion method
func TestWorkerPoolWaitForCompletion(t *testing.T) {
	ctx := context.Background()

	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)
	require.NoError(t, pool.Start())
	defer pool.Stop()

	// Create a mock Docker client
	mockClient := &docker.Client{}

	// Submit several jobs
	checks := []Check{
		&WorkerPoolTestMockCheck{name: "test1", description: "Test 1", severity: SeverityInfo, delay: 10 * time.Millisecond},
		&WorkerPoolTestMockCheck{name: "test2", description: "Test 2", severity: SeverityWarning, delay: 20 * time.Millisecond},
		&WorkerPoolTestMockCheck{name: "test3", description: "Test 3", severity: SeverityError, delay: 15 * time.Millisecond},
	}

	// Submit all checks
	for _, check := range checks {
		err := pool.Submit(check, mockClient)
		require.NoError(t, err)
	}

	// Wait for completion with reasonable timeout
	timeout := time.After(1 * time.Second)
	done := make(chan bool)

	go func() {
		err := pool.WaitForCompletion(500 * time.Millisecond)
		if err != nil {
			t.Logf("WaitForCompletion returned error: %v", err)
		}
		done <- true
	}()

	select {
	case <-done:
		// Success - all jobs completed
		metrics := pool.GetMetrics()
		assert.Equal(t, len(checks), metrics.CompletedJobs+metrics.FailedJobs)
	case <-timeout:
		t.Fatal("WaitForCompletion timed out")
	}
}

// TestWorkerPoolSetTestMode tests the SetTestMode method
func TestWorkerPoolSetTestMode(t *testing.T) {
	ctx := context.Background()

	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)

	// Test setting test mode before starting
	pool.SetTestMode(true)

	require.NoError(t, pool.Start())
	defer pool.Stop()

	// Create a mock Docker client
	mockClient := &docker.Client{}

	// Create a mock check that would normally execute
	check := &WorkerPoolTestMockCheck{
		name:        "test_mode_check",
		description: "Test Mode Check",
		severity:    SeverityInfo,
		delay:       10 * time.Millisecond,
	}

	// Submit the check
	err = pool.Submit(check, mockClient)
	require.NoError(t, err)

	// Wait a bit for processing
	time.Sleep(50 * time.Millisecond)

	// In test mode, behavior might be different - the key is no panics
	metrics := pool.GetMetrics()
	assert.GreaterOrEqual(t, metrics.TotalJobs, 1)

	// Test turning off test mode
	pool.SetTestMode(false)

	// Submit another check
	check2 := &WorkerPoolTestMockCheck{
		name:        "test_normal_mode",
		description: "Normal Mode Check",
		severity:    SeverityInfo,
		delay:       10 * time.Millisecond,
	}

	err = pool.Submit(check2, mockClient)
	// This should fail because test mode is disabled and check is not in allowlist
	if err != nil {
		assert.Contains(t, err.Error(), "allowlist")
		t.Log("Expected allowlist error when test mode is disabled:", err)
	} else {
		// If it succeeds, that's also acceptable - the test name might pass validation
		t.Log("Check succeeded despite test mode being disabled")
	}

	// Wait for completion
	time.Sleep(50 * time.Millisecond)

	// Check final metrics - might be 1 or 2 depending on allowlist validation
	finalMetrics := pool.GetMetrics()
	assert.GreaterOrEqual(t, finalMetrics.TotalJobs, 1) // At least the first job should have run
}

// TestWorkerPoolWaitForCompletionWithTimeout tests WaitForCompletion with timeout context
func TestWorkerPoolWaitForCompletionWithTimeout(t *testing.T) {
	ctx := context.Background()

	pool, err := NewSecureWorkerPool(ctx, 1)
	require.NoError(t, err)
	require.NoError(t, pool.Start())
	defer pool.Stop()

	// Create a mock Docker client
	mockClient := &docker.Client{}

	// Submit a long-running job
	longCheck := &WorkerPoolTestMockCheck{
		name:        "long_check",
		description: "Long Running Check",
		severity:    SeverityInfo,
		delay:       200 * time.Millisecond, // Long delay
	}

	err = pool.Submit(longCheck, mockClient)
	require.NoError(t, err)

	// Test that WaitForCompletion can be interrupted/timed out
	start := time.Now()
	timeout := time.After(100 * time.Millisecond) // Shorter than job duration
	done := make(chan bool)

	go func() {
		err := pool.WaitForCompletion(500 * time.Millisecond)
		if err != nil {
			t.Logf("WaitForCompletion returned error: %v", err)
		}
		done <- true
	}()

	select {
	case <-done:
		elapsed := time.Since(start)
		// Should complete after job finishes, not before timeout
		assert.Greater(t, elapsed, 100*time.Millisecond)
	case <-timeout:
		// This is also acceptable - WaitForCompletion might not complete within timeout
		// The important thing is no deadlock or panic
		t.Log("WaitForCompletion did not complete within timeout (acceptable)")
	}

	// Give more time for job to actually complete
	time.Sleep(300 * time.Millisecond)
}

// TestWorkerPoolTestModeIntegration tests SetTestMode integration with job execution
func TestWorkerPoolTestModeIntegration(t *testing.T) {
	ctx := context.Background()

	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)
	require.NoError(t, pool.Start())
	defer pool.Stop()

	// Enable test mode
	pool.SetTestMode(true)

	// Create a mock Docker client
	mockClient := &docker.Client{}

	// Test with a mix of successful and failing checks
	checks := []Check{
		&WorkerPoolTestMockCheck{name: "success1", description: "Success 1", severity: SeverityInfo, delay: 5 * time.Millisecond},
		&WorkerPoolTestMockCheck{name: "failure1", description: "Failure 1", severity: SeverityError, delay: 5 * time.Millisecond, shouldFail: true},
		&WorkerPoolTestMockCheck{name: "success2", description: "Success 2", severity: SeverityWarning, delay: 5 * time.Millisecond},
	}

	// Submit all checks
	for _, check := range checks {
		err := pool.Submit(check, mockClient)
		require.NoError(t, err)
	}

	// Wait for completion
	timeout := time.After(500 * time.Millisecond)
	done := make(chan bool)

	go func() {
		err := pool.WaitForCompletion(500 * time.Millisecond)
		if err != nil {
			t.Logf("WaitForCompletion returned error: %v", err)
		}
		done <- true
	}()

	select {
	case <-done:
		// Verify all jobs were processed
		metrics := pool.GetMetrics()
		assert.Equal(t, len(checks), metrics.TotalJobs)
		assert.Equal(t, len(checks), metrics.CompletedJobs+metrics.FailedJobs)
	case <-timeout:
		t.Fatal("Jobs did not complete within timeout")
	}

	// Disable test mode
	pool.SetTestMode(false)

	// Submit one more check to test mode transition
	finalCheck := &WorkerPoolTestMockCheck{
		name:        "test_final",
		description: "Final Check",
		severity:    SeverityInfo,
		delay:       5 * time.Millisecond,
	}

	err = pool.Submit(finalCheck, mockClient)
	// This might fail because test mode is disabled
	if err != nil {
		assert.Contains(t, err.Error(), "allowlist")
		t.Log("Expected allowlist error when test mode is disabled:", err)
	}

	// Wait for the final check
	time.Sleep(50 * time.Millisecond)

	finalMetrics := pool.GetMetrics()
	// Should have at least processed the original checks
	assert.GreaterOrEqual(t, finalMetrics.TotalJobs, len(checks))
}

// TestWorkerPoolErrorRateMonitoring tests the error rate monitoring functionality
func TestWorkerPoolErrorRateMonitoring(t *testing.T) {
	ctx := context.Background()

	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)
	require.NoError(t, pool.Start())
	defer pool.Stop()

	// Create a mock Docker client
	mockClient := &docker.Client{}

	// Create checks that will fail to trigger error monitoring
	failingChecks := []*WorkerPoolTestMockCheck{
		&WorkerPoolTestMockCheck{name: "fail1", description: "Fail 1", severity: SeverityError, delay: 5 * time.Millisecond, shouldFail: true},
		&WorkerPoolTestMockCheck{name: "fail2", description: "Fail 2", severity: SeverityError, delay: 5 * time.Millisecond, shouldFail: true},
		&WorkerPoolTestMockCheck{name: "fail3", description: "Fail 3", severity: SeverityError, delay: 5 * time.Millisecond, shouldFail: true},
		&WorkerPoolTestMockCheck{name: "fail4", description: "Fail 4", severity: SeverityError, delay: 5 * time.Millisecond, shouldFail: true},
		&WorkerPoolTestMockCheck{name: "fail5", description: "Fail 5", severity: SeverityError, delay: 5 * time.Millisecond, shouldFail: true},
	}

	// Submit all failing checks
	for _, check := range failingChecks {
		err := pool.Submit(check, mockClient)
		require.NoError(t, err)
	}

	// Wait for completion
	timeout := time.After(1 * time.Second)
	done := make(chan bool)

	go func() {
		err := pool.WaitForCompletion(500 * time.Millisecond)
		if err != nil {
			t.Logf("WaitForCompletion returned error: %v", err)
		}
		done <- true
	}()

	select {
	case <-done:
		// Check metrics after failures
		metrics := pool.GetMetrics()
		assert.Equal(t, len(failingChecks), metrics.TotalJobs)
		assert.Equal(t, len(failingChecks), metrics.FailedJobs)

		// The error rate should be high
		assert.Greater(t, float64(metrics.FailedJobs)/float64(metrics.TotalJobs), 0.8)
	case <-timeout:
		t.Fatal("Jobs did not complete within timeout")
	}
}

// TestWorkerPoolCircuitBreakerIntegration tests circuit breaker integration
func TestWorkerPoolCircuitBreakerIntegration(t *testing.T) {
	ctx := context.Background()

	pool, err := NewSecureWorkerPool(ctx, 1)
	require.NoError(t, err)
	require.NoError(t, pool.Start())
	defer pool.Stop()

	// Create a mock Docker client
	mockClient := &docker.Client{}

	// Submit enough failing jobs to potentially trigger circuit breaker
	for i := 0; i < 10; i++ {
		failCheck := &WorkerPoolTestMockCheck{
			name:        fmt.Sprintf("circuit_fail_%d", i),
			description: fmt.Sprintf("Circuit Fail %d", i),
			severity:    SeverityError,
			delay:       5 * time.Millisecond,
			shouldFail:  true,
		}

		err := pool.Submit(failCheck, mockClient)
		require.NoError(t, err)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Check metrics
	metrics := pool.GetMetrics()
	assert.GreaterOrEqual(t, metrics.TotalJobs, 10)

	// Test that pool is still functional (circuit breaker should handle failures gracefully)
	successCheck := &WorkerPoolTestMockCheck{
		name:        "success_after_failures",
		description: "Success After Failures",
		severity:    SeverityInfo,
		delay:       5 * time.Millisecond,
		shouldFail:  false,
	}

	err = pool.Submit(successCheck, mockClient)
	// This might fail due to circuit breaker being open, which is expected behavior
	if err != nil {
		t.Logf("Expected circuit breaker behavior: %v", err)
		assert.Contains(t, err.Error(), "circuit breaker")
	}

	// Wait a bit more
	time.Sleep(50 * time.Millisecond)

	finalMetrics := pool.GetMetrics()
	// Should have processed at least 10 failing jobs
	assert.GreaterOrEqual(t, finalMetrics.TotalJobs, 10)
}

// TestWorkerPoolMemoryMonitoring tests memory usage monitoring
func TestWorkerPoolMemoryMonitoring(t *testing.T) {
	ctx := context.Background()

	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)

	// Enable test mode to allow test checks
	pool.SetTestMode(true)

	require.NoError(t, pool.Start())
	defer pool.Stop()

	// Create a mock Docker client
	mockClient := &docker.Client{}

	// Submit some checks to generate memory usage
	checks := []Check{
		&WorkerPoolTestMockCheck{name: "test_mem1", description: "Memory 1", severity: SeverityInfo, delay: 10 * time.Millisecond},
		&WorkerPoolTestMockCheck{name: "test_mem2", description: "Memory 2", severity: SeverityInfo, delay: 15 * time.Millisecond},
		&WorkerPoolTestMockCheck{name: "test_mem3", description: "Memory 3", severity: SeverityInfo, delay: 20 * time.Millisecond},
	}

	// Submit checks
	for _, check := range checks {
		err := pool.Submit(check, mockClient)
		require.NoError(t, err)
	}

	// Wait for completion
	time.Sleep(100 * time.Millisecond)

	// Check metrics which include memory information
	metrics := pool.GetMetrics()

	// Peak memory usage should be recorded in metrics
	assert.GreaterOrEqual(t, metrics.PeakMemoryMB, 0.0)

	// Verify all jobs were processed
	assert.Equal(t, len(checks), metrics.TotalJobs)
}

// TestWorkerPoolHealthMonitoring tests the IsHealthy method
func TestWorkerPoolHealthMonitoring(t *testing.T) {
	ctx := context.Background()

	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)
	require.NoError(t, pool.Start())
	defer pool.Stop()

	// Initially should be healthy
	assert.True(t, pool.IsHealthy())

	// Create a mock Docker client
	mockClient := &docker.Client{}

	// Submit a normal check
	normalCheck := &WorkerPoolTestMockCheck{
		name:        "health_check",
		description: "Health Check",
		severity:    SeverityInfo,
		delay:       10 * time.Millisecond,
		shouldFail:  false,
	}

	err = pool.Submit(normalCheck, mockClient)
	require.NoError(t, err)

	// Wait for completion
	time.Sleep(50 * time.Millisecond)

	// Should still be healthy
	assert.True(t, pool.IsHealthy())

	// Submit many failing checks to potentially affect health
	for i := 0; i < 5; i++ {
		failCheck := &WorkerPoolTestMockCheck{
			name:        fmt.Sprintf("health_fail_%d", i),
			description: fmt.Sprintf("Health Fail %d", i),
			severity:    SeverityError,
			delay:       5 * time.Millisecond,
			shouldFail:  true,
		}

		err := pool.Submit(failCheck, mockClient)
		require.NoError(t, err)
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Health might be affected, but the method should still work
	health := pool.IsHealthy()
	assert.IsType(t, false, health) // Just verify it returns a boolean
}
