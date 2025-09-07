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

// TestMockCheck implements the Check interface for testing worker pool
type TestMockCheck struct {
	name        string
	description string
	severity    Severity
	delay       time.Duration
	shouldFail  bool
	shouldPanic bool
	execCount   atomic.Int32
}

func (m *TestMockCheck) Name() string        { return m.name }
func (m *TestMockCheck) Description() string { return m.description }
func (m *TestMockCheck) Severity() Severity  { return m.severity }

func (m *TestMockCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
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
		name         string
		workerCount  int
		expectedWorkers int
	}{
		{
			name:         "Zero workers uses CPU count",
			workerCount:  0,
			expectedWorkers: min(runtime.NumCPU(), MAX_WORKERS),
		},
		{
			name:         "Negative workers uses CPU count",
			workerCount:  -1,
			expectedWorkers: min(runtime.NumCPU(), MAX_WORKERS),
		},
		{
			name:         "Valid worker count",
			workerCount:  5,
			expectedWorkers: 5,
		},
		{
			name:         "Exceeds max workers",
			workerCount:  100,
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
	checks := []*TestMockCheck{
		{name: "check1", delay: 10 * time.Millisecond},
		{name: "check2", delay: 10 * time.Millisecond},
		{name: "check3", delay: 10 * time.Millisecond},
		{name: "check4", delay: 10 * time.Millisecond},
		{name: "check5", delay: 10 * time.Millisecond},
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
		assert.Equal(t, int32(1), check.execCount.Load())
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
	checks := []*TestMockCheck{
		{name: "success", delay: 10 * time.Millisecond},
		{name: "failure", delay: 10 * time.Millisecond, shouldFail: true},
		{name: "panic", delay: 10 * time.Millisecond, shouldPanic: true},
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
		check := &TestMockCheck{
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
		check := &TestMockCheck{
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
	check := &TestMockCheck{name: "test"}
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
	checks := []*TestMockCheck{
		{name: "quick", delay: 5 * time.Millisecond},
		{name: "medium", delay: 20 * time.Millisecond},
		{name: "slow", delay: 50 * time.Millisecond},
		{name: "failure", delay: 10 * time.Millisecond, shouldFail: true},
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
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}