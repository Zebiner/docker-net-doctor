package diagnostics

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// Simplified Integration Tests focusing on core worker pool functionality

func TestWorkerPoolBasicIntegration(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 4)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Test basic job submission and execution
	checks := []Check{
		&SimpleIntegrationCheck{name: "test1", duration: 50 * time.Millisecond, shouldFail: false},
		&SimpleIntegrationCheck{name: "test2", duration: 75 * time.Millisecond, shouldFail: false},
		&SimpleIntegrationCheck{name: "test3", duration: 25 * time.Millisecond, shouldFail: true},
		&SimpleIntegrationCheck{name: "test4", duration: 100 * time.Millisecond, shouldFail: false},
	}

	// Submit all checks
	for _, check := range checks {
		err := pool.Submit(check, nil)
		require.NoError(t, err)
	}

	// Collect all results
	results := make([]JobResult, 0)
	resultsChan := pool.GetResults()
	timeout := time.After(5 * time.Second)

	for len(results) < len(checks) {
		select {
		case result := <-resultsChan:
			results = append(results, result)
		case <-timeout:
			t.Fatalf("Timeout collecting results, got %d/%d", len(results), len(checks))
		}
	}

	// Verify results
	assert.Equal(t, len(checks), len(results), "All checks should execute")

	successCount := 0
	failureCount := 0
	for _, result := range results {
		if result.Error == nil && result.Result != nil && result.Result.Success {
			successCount++
		} else {
			failureCount++
		}
	}

	assert.Equal(t, 3, successCount, "Should have 3 successful checks")
	assert.Equal(t, 1, failureCount, "Should have 1 failed check")

	// Verify pool health
	assert.True(t, pool.IsHealthy(), "Pool should be healthy after execution")

	// Verify metrics
	metrics := pool.GetMetrics()
	assert.Equal(t, len(checks), metrics.TotalJobs, "Total jobs should match")
	assert.Equal(t, 3, metrics.CompletedJobs, "Completed jobs should match")
	assert.Equal(t, 1, metrics.FailedJobs, "Failed jobs should match")

	t.Logf("Basic Integration Test Results:")
	t.Logf("  Total Jobs: %d", metrics.TotalJobs)
	t.Logf("  Completed: %d", metrics.CompletedJobs)
	t.Logf("  Failed: %d", metrics.FailedJobs)
	t.Logf("  Peak Memory: %.2f MB", metrics.PeakMemoryMB)
}

func TestWorkerPoolConcurrencyIntegration(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 6)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Create many concurrent checks
	checkCount := 50
	checks := make([]Check, checkCount)
	for i := 0; i < checkCount; i++ {
		checks[i] = &ConcurrentIntegrationCheck{
			name:     fmt.Sprintf("test_concurrent_%d", i),
			duration: time.Duration(10+i%20) * time.Millisecond, // 10-30ms variation
			phase:    []string{"discovery", "inspection", "testing", "validation"}[i%4],
		}
	}

	// Submit all checks
	start := time.Now()
	for _, check := range checks {
		err := pool.Submit(check, nil)
		require.NoError(t, err)
	}

	// Collect results
	results := make([]JobResult, 0)
	resultsChan := pool.GetResults()
	timeout := time.After(10 * time.Second)

	for len(results) < checkCount {
		select {
		case result := <-resultsChan:
			results = append(results, result)
		case <-timeout:
			t.Fatalf("Timeout collecting concurrent results, got %d/%d", len(results), checkCount)
		}
	}

	duration := time.Since(start)

	// Verify concurrency benefits
	sequentialTime := time.Duration(0)
	for _, check := range checks {
		if cic, ok := check.(*ConcurrentIntegrationCheck); ok {
			sequentialTime += cic.duration
		}
	}

	// Should be significantly faster than sequential
	assert.Less(t, duration, sequentialTime/3, "Concurrent execution should be much faster")

	// All results should be successful
	for _, result := range results {
		assert.NoError(t, result.Error, "Concurrent check should not error")
		assert.NotNil(t, result.Result, "Should have result")
		assert.True(t, result.Result.Success, "Concurrent check should succeed")
	}

	t.Logf("Concurrency Integration Results:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Sequential Time: %v", sequentialTime)
	t.Logf("  Speedup: %.2fx", float64(sequentialTime)/float64(duration))
	t.Logf("  Checks: %d", len(results))
}

func TestWorkerPoolResilienceIntegration(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 4)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Create mixed workload with various failure modes
	checks := []Check{
		&ResilienceIntegrationCheck{name: "test_normal1", checkType: "normal", duration: 50 * time.Millisecond},
		&ResilienceIntegrationCheck{name: "panic1", checkType: "panic", duration: 30 * time.Millisecond},
		&ResilienceIntegrationCheck{name: "error1", checkType: "error", duration: 40 * time.Millisecond},
		&ResilienceIntegrationCheck{name: "test_normal2", checkType: "normal", duration: 60 * time.Millisecond},
		&ResilienceIntegrationCheck{name: "timeout1", checkType: "timeout", duration: 100 * time.Millisecond},
		&ResilienceIntegrationCheck{name: "normal3", checkType: "normal", duration: 35 * time.Millisecond},
		&ResilienceIntegrationCheck{name: "panic2", checkType: "panic", duration: 45 * time.Millisecond},
		&ResilienceIntegrationCheck{name: "normal4", checkType: "normal", duration: 55 * time.Millisecond},
	}

	// Submit all checks
	for _, check := range checks {
		err := pool.Submit(check, nil)
		require.NoError(t, err)
	}

	// Collect results
	results := make([]JobResult, 0)
	resultsChan := pool.GetResults()
	timeout := time.After(5 * time.Second)

	for len(results) < len(checks) {
		select {
		case result := <-resultsChan:
			results = append(results, result)
		case <-timeout:
			t.Fatalf("Timeout collecting resilience results, got %d/%d", len(results), len(checks))
		}
	}

	// Verify resilience - normal checks should succeed despite failures
	normalSucceeded := 0
	failuresHandled := 0

	for _, result := range results {
		if result.Result != nil && result.Result.Success {
			normalSucceeded++
		} else {
			failuresHandled++
		}
	}

	// Should have 4 normal checks succeed and 4 failures handled
	assert.Equal(t, 4, normalSucceeded, "Normal checks should succeed despite failures")
	assert.Equal(t, 4, failuresHandled, "Failures should be handled gracefully")

	// Pool should remain healthy and functional
	assert.True(t, pool.IsHealthy(), "Pool should remain healthy after mixed failures")

	// Check that panics were recovered
	metrics := pool.GetMetrics()
	assert.GreaterOrEqual(t, metrics.RecoveredPanics, 2, "Should have recovered from panics")

	t.Logf("Resilience Integration Results:")
	t.Logf("  Normal Succeeded: %d", normalSucceeded)
	t.Logf("  Failures Handled: %d", failuresHandled)
	t.Logf("  Recovered Panics: %d", metrics.RecoveredPanics)
	t.Logf("  Pool Health: %v", pool.IsHealthy())
}

func TestWorkerPoolMemoryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory integration test in short mode")
	}

	ctx := context.Background()
	initialMemory := getCurrentMemory()

	pool, err := NewSecureWorkerPool(ctx, 3)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)

	// Create memory-intensive checks
	checks := []Check{
		&MemoryIntegrationCheck{name: "test_mem1", allocSize: 1024 * 1024, duration: 100 * time.Millisecond},     // 1MB
		&MemoryIntegrationCheck{name: "test_mem2", allocSize: 512 * 1024, duration: 150 * time.Millisecond},      // 512KB
		&MemoryIntegrationCheck{name: "test_mem3", allocSize: 2 * 1024 * 1024, duration: 80 * time.Millisecond}, // 2MB
		&MemoryIntegrationCheck{name: "mem4", allocSize: 256 * 1024, duration: 120 * time.Millisecond},      // 256KB
	}

	// Submit memory-intensive jobs
	for _, check := range checks {
		err := pool.Submit(check, nil)
		require.NoError(t, err)
	}

	// Collect results
	results := make([]JobResult, 0)
	resultsChan := pool.GetResults()
	timeout := time.After(5 * time.Second)

	for len(results) < len(checks) {
		select {
		case result := <-resultsChan:
			results = append(results, result)
		case <-timeout:
			t.Fatalf("Timeout collecting memory results, got %d/%d", len(results), len(checks))
		}
	}

	// Stop pool and force cleanup
	pool.Stop()
	runtime.GC()
	runtime.GC() // Force GC multiple times
	time.Sleep(100 * time.Millisecond)

	finalMemory := getCurrentMemory()
	memoryDiff := int64(finalMemory) - int64(initialMemory)

	// Verify all memory jobs completed
	for _, result := range results {
		assert.NoError(t, result.Error, "Memory job should not error")
		assert.NotNil(t, result.Result, "Should have result")
		assert.True(t, result.Result.Success, "Memory job should succeed")
	}

	// Memory should not leak significantly
	maxAllowedIncrease := int64(5 * 1024 * 1024) // 5MB tolerance
	assert.LessOrEqual(t, memoryDiff, maxAllowedIncrease,
		"Memory should not leak significantly: %d MB increase", memoryDiff/(1024*1024))

	t.Logf("Memory Integration Results:")
	t.Logf("  Initial Memory: %d MB", initialMemory/(1024*1024))
	t.Logf("  Final Memory: %d MB", finalMemory/(1024*1024))
	t.Logf("  Memory Difference: %d MB", memoryDiff/(1024*1024))
	t.Logf("  All Jobs Completed: %v", len(results) == len(checks))
}

// Helper types for integration testing

type SimpleIntegrationCheck struct {
	name       string
	duration   time.Duration
	shouldFail bool
}

func (s *SimpleIntegrationCheck) Name() string        { return s.name }
func (s *SimpleIntegrationCheck) Description() string { return "Simple integration test check" }
func (s *SimpleIntegrationCheck) Severity() Severity  { return SeverityInfo }

func (s *SimpleIntegrationCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	time.Sleep(s.duration)

	if s.shouldFail {
		return &CheckResult{
			CheckName: s.name,
			Success:   false,
			Message:   "Simulated integration failure",
			Timestamp: time.Now(),
		}, fmt.Errorf("simulated failure")
	}

	return &CheckResult{
		CheckName: s.name,
		Success:   true,
		Message:   "Integration check completed",
		Timestamp: time.Now(),
	}, nil
}

type ConcurrentIntegrationCheck struct {
	name     string
	duration time.Duration
	phase    string
}

func (c *ConcurrentIntegrationCheck) Name() string        { return c.name }
func (c *ConcurrentIntegrationCheck) Description() string { return "Concurrent integration test check" }
func (c *ConcurrentIntegrationCheck) Severity() Severity  { return SeverityInfo }

func (c *ConcurrentIntegrationCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	time.Sleep(c.duration)

	return &CheckResult{
		CheckName: c.name,
		Success:   true,
		Message:   fmt.Sprintf("Concurrent check in %s phase completed", c.phase),
		Details: map[string]interface{}{
			"phase":    c.phase,
			"duration": c.duration,
		},
		Timestamp: time.Now(),
	}, nil
}

type ResilienceIntegrationCheck struct {
	name      string
	checkType string
	duration  time.Duration
}

func (r *ResilienceIntegrationCheck) Name() string        { return r.name }
func (r *ResilienceIntegrationCheck) Description() string { return "Resilience integration test check" }
func (r *ResilienceIntegrationCheck) Severity() Severity  { return SeverityWarning }

func (r *ResilienceIntegrationCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	switch r.checkType {
	case "panic":
		time.Sleep(r.duration / 2)
		panic("Integration test panic")

	case "error":
		time.Sleep(r.duration)
		return &CheckResult{
			CheckName: r.name,
			Success:   false,
			Message:   "Integration test error",
			Timestamp: time.Now(),
		}, fmt.Errorf("integration test error")

	case "timeout":
		// Simulate a check that might timeout
		select {
		case <-time.After(r.duration):
			return &CheckResult{
				CheckName: r.name,
				Success:   true,
				Message:   "Integration check completed after delay",
				Timestamp: time.Now(),
			}, nil
		case <-ctx.Done():
			return &CheckResult{
				CheckName: r.name,
				Success:   false,
				Message:   "Integration check timed out",
				Timestamp: time.Now(),
			}, ctx.Err()
		}

	default: // "normal"
		time.Sleep(r.duration)
		return &CheckResult{
			CheckName: r.name,
			Success:   true,
			Message:   "Integration check completed normally",
			Timestamp: time.Now(),
		}, nil
	}
}

type MemoryIntegrationCheck struct {
	name      string
	allocSize int
	duration  time.Duration
}

func (m *MemoryIntegrationCheck) Name() string        { return m.name }
func (m *MemoryIntegrationCheck) Description() string { return "Memory integration test check" }
func (m *MemoryIntegrationCheck) Severity() Severity  { return SeverityInfo }

func (m *MemoryIntegrationCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	// Allocate memory
	data := make([]byte, m.allocSize)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Hold memory for specified duration
	time.Sleep(m.duration)

	// Process the data briefly
	checksum := 0
	for _, b := range data {
		checksum += int(b)
	}

	// Clear reference to help GC
	data = nil

	return &CheckResult{
		CheckName: m.name,
		Success:   true,
		Message:   fmt.Sprintf("Memory integration check completed (checksum: %d)", checksum),
		Details: map[string]interface{}{
			"alloc_size": m.allocSize,
			"checksum":   checksum,
		},
		Timestamp: time.Now(),
	}, nil
}