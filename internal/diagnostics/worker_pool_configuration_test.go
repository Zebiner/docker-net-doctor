package diagnostics

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zebiner/docker-net-doctor/internal/docker"
	"golang.org/x/time/rate"
)

// Configuration and Management Tests

func TestWorkerPoolConfigurationValidation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		workerCount   int
		expectedCount int
		shouldError   bool
		description   string
	}{
		{
			name:          "valid_worker_count",
			workerCount:   4,
			expectedCount: 4,
			shouldError:   false,
			description:   "Valid worker count should be accepted",
		},
		{
			name:          "zero_workers_defaults_to_cpu",
			workerCount:   0,
			expectedCount: min(runtime.NumCPU(), MAX_WORKERS),
			shouldError:   false,
			description:   "Zero workers should default to CPU count",
		},
		{
			name:          "negative_workers_defaults_to_cpu",
			workerCount:   -5,
			expectedCount: min(runtime.NumCPU(), MAX_WORKERS),
			shouldError:   false,
			description:   "Negative workers should default to CPU count",
		},
		{
			name:          "max_workers_limit",
			workerCount:   1000,
			expectedCount: MAX_WORKERS,
			shouldError:   false,
			description:   "Excessive workers should be capped at MAX_WORKERS",
		},
		{
			name:          "single_worker",
			workerCount:   1,
			expectedCount: 1,
			shouldError:   false,
			description:   "Single worker should be valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := NewSecureWorkerPool(ctx, tt.workerCount)

			if tt.shouldError {
				assert.Error(t, err, tt.description)
			} else {
				require.NoError(t, err, tt.description)
				assert.Equal(t, tt.expectedCount, pool.workers, "Worker count should match expected")
				assert.NotNil(t, pool.rateLimiter, "Rate limiter should be initialized")
				assert.NotNil(t, pool.memoryMonitor, "Memory monitor should be initialized")
				assert.NotNil(t, pool.errorRate, "Error rate monitor should be initialized")
				assert.NotNil(t, pool.validator, "Security validator should be initialized")
			}
		})
	}
}

func TestRuntimeConfigurationUpdates(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 3)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Test initial configuration
	assert.Equal(t, 3, pool.workers, "Initial worker count should be 3")

	// Test rate limiter configuration updates
	originalRate := pool.rateLimiter.Limit()
	originalBurst := pool.rateLimiter.Burst()

	t.Logf("Original rate limit: %v req/sec, burst: %d", originalRate, originalBurst)

	// Update rate limiter configuration
	newLimit := rate.Limit(20)
	newBurst := 5
	pool.rateLimiter.SetLimit(newLimit)
	pool.rateLimiter.SetBurst(newBurst)

	assert.Equal(t, newLimit, pool.rateLimiter.Limit(), "Rate limit should be updated")
	assert.Equal(t, newBurst, pool.rateLimiter.Burst(), "Burst size should be updated")

	// Test memory monitor configuration updates
	newInterval := 500 * time.Millisecond
	pool.memoryMonitor.checkInterval = newInterval

	assert.Equal(t, newInterval, pool.memoryMonitor.checkInterval, "Memory check interval should be updated")

	// Test error rate monitor configuration updates
	newThreshold := 0.75 // 75% error rate threshold
	pool.errorRate.mu.Lock()
	pool.errorRate.threshold = newThreshold
	pool.errorRate.mu.Unlock()

	pool.errorRate.mu.RLock()
	actualThreshold := pool.errorRate.threshold
	pool.errorRate.mu.RUnlock()

	assert.Equal(t, newThreshold, actualThreshold, "Error rate threshold should be updated")

	// Test that pool still functions after configuration changes
	check := &ConfigurationTestCheck{name: "config_update_test", duration: 50 * time.Millisecond}
	err = pool.Submit(check, nil)
	assert.NoError(t, err, "Pool should function after configuration updates")

	select {
	case result := <-pool.GetResults():
		assert.NoError(t, result.Error, "Check should succeed after configuration updates")
		assert.True(t, result.Result.Success, "Check should report success")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for configuration test result")
	}

	t.Logf("Configuration update test completed successfully")
}

func TestWorkerPoolMonitoringAndObservability(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 4)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Test initial metrics
	initialMetrics := pool.GetMetrics()
	assert.Equal(t, 4, initialMetrics.PeakWorkers, "Peak workers should match initial count")
	assert.Equal(t, 0, initialMetrics.TotalJobs, "Initial total jobs should be 0")
	assert.True(t, initialMetrics.StartTime.Before(time.Now()), "Start time should be set")

	// Test health monitoring
	assert.True(t, pool.IsHealthy(), "Pool should be healthy initially")

	// Submit monitoring test jobs
	monitoringJobs := []*MonitoringTestCheck{
		{name: "fast_job", duration: 10 * time.Millisecond, jobType: "fast"},
		{name: "medium_job", duration: 50 * time.Millisecond, jobType: "medium"},
		{name: "slow_job", duration: 100 * time.Millisecond, jobType: "slow"},
		{name: "test_memory_job", duration: 30 * time.Millisecond, jobType: "memory"},
		{name: "test_cpu_job", duration: 40 * time.Millisecond, jobType: "cpu"},
	}

	startTime := time.Now()

	for _, job := range monitoringJobs {
		err := pool.Submit(job, nil)
		assert.NoError(t, err, "Should be able to submit monitoring job")
	}

	// Collect results while monitoring
	results := make([]JobResult, 0)
	timeout := time.After(3 * time.Second)

	for len(results) < len(monitoringJobs) {
		select {
		case result := <-pool.GetResults():
			results = append(results, result)

			// Check metrics after each result
			currentMetrics := pool.GetMetrics()
			t.Logf("Progress: %d/%d jobs completed", len(results), len(monitoringJobs))
			t.Logf("  Total Jobs: %d", currentMetrics.TotalJobs)
			t.Logf("  Completed Jobs: %d", currentMetrics.CompletedJobs)
			t.Logf("  Failed Jobs: %d", currentMetrics.FailedJobs)
			t.Logf("  Peak Memory: %.2f MB", currentMetrics.PeakMemoryMB)

		case <-timeout:
			t.Fatalf("Timeout collecting monitoring results, got %d/%d", len(results), len(monitoringJobs))
		}
	}

	executionTime := time.Since(startTime)

	// Verify final metrics
	finalMetrics := pool.GetMetrics()
	assert.Equal(t, len(monitoringJobs), finalMetrics.TotalJobs, "Total jobs should match submitted count")
	assert.Equal(t, len(monitoringJobs), finalMetrics.CompletedJobs, "All jobs should complete successfully")
	assert.Equal(t, 0, finalMetrics.FailedJobs, "No jobs should fail")
	assert.Greater(t, finalMetrics.MaxJobTime, time.Duration(0), "Max job time should be recorded")
	assert.Greater(t, finalMetrics.MinJobTime, time.Duration(0), "Min job time should be recorded")
	assert.LessOrEqual(t, finalMetrics.MinJobTime, finalMetrics.MaxJobTime, "Min time should be <= max time")

	// Verify observability data
	assert.True(t, pool.IsHealthy(), "Pool should remain healthy")
	assert.GreaterOrEqual(t, finalMetrics.PeakMemoryMB, 0.0, "Peak memory should be non-negative")
	assert.Greater(t, finalMetrics.TotalAPIcalls, 0, "API calls should be recorded")

	t.Logf("Monitoring and Observability Results:")
	t.Logf("  Execution Time: %v", executionTime)
	t.Logf("  Total Jobs: %d", finalMetrics.TotalJobs)
	t.Logf("  Completed Jobs: %d", finalMetrics.CompletedJobs)
	t.Logf("  Peak Workers: %d", finalMetrics.PeakWorkers)
	t.Logf("  Min Job Time: %v", finalMetrics.MinJobTime)
	t.Logf("  Max Job Time: %v", finalMetrics.MaxJobTime)
	t.Logf("  Average Job Time: %v", finalMetrics.AverageJobTime)
	t.Logf("  Peak Memory: %.2f MB", finalMetrics.PeakMemoryMB)
	t.Logf("  API Calls: %d", finalMetrics.TotalAPIcalls)
}

func TestAdministrativeOperations(t *testing.T) {
	ctx := context.Background()

	// Test start/stop/restart operations
	t.Run("StartStopOperations", func(t *testing.T) {
		pool, err := NewSecureWorkerPool(ctx, 3)
		require.NoError(t, err)

		// Test multiple start/stop cycles
		for cycle := 0; cycle < 3; cycle++ {
			t.Logf("Cycle %d: Starting pool", cycle+1)

			// Start pool
			err = pool.Start()
			assert.NoError(t, err, "Pool should start successfully")
			assert.True(t, pool.started.Load(), "Started flag should be true")
			assert.False(t, pool.stopped.Load(), "Stopped flag should be false")

			// Verify pool is functional
			check := &AdministrativeTestCheck{
				name:     fmt.Sprintf("admin_test_cycle_%d", cycle+1),
				duration: 20 * time.Millisecond,
			}

			err = pool.Submit(check, nil)
			assert.NoError(t, err, "Should be able to submit during operational cycle")

			select {
			case result := <-pool.GetResults():
				assert.NoError(t, result.Error, "Admin test should succeed")
			case <-time.After(1 * time.Second):
				t.Fatal("Timeout waiting for admin test result")
			}

			t.Logf("Cycle %d: Stopping pool", cycle+1)

			// Stop pool
			err = pool.Stop()
			assert.NoError(t, err, "Pool should stop successfully")
			assert.True(t, pool.stopped.Load(), "Stopped flag should be true")

			// Verify pool is not functional
			err = pool.Submit(check, nil)
			assert.Error(t, err, "Should not be able to submit after stop")

			// Reset for next cycle (create new pool)
			if cycle < 2 {
				pool, err = NewSecureWorkerPool(ctx, 3)
				require.NoError(t, err)
			}
		}
	})

	// Test administrative control while running
	t.Run("RuntimeAdministration", func(t *testing.T) {
		pool, err := NewSecureWorkerPool(ctx, 4)
		require.NoError(t, err)

		err = pool.Start()
		require.NoError(t, err)
		defer pool.Stop()

		// Test health checks during administration
		assert.True(t, pool.IsHealthy(), "Pool should be healthy")

		// Submit administrative jobs
		adminJobs := []*AdministrativeTestCheck{
			{name: "health_check", duration: 10 * time.Millisecond, checkType: "health"},
			{name: "metrics_check", duration: 15 * time.Millisecond, checkType: "metrics"},
			{name: "status_check", duration: 20 * time.Millisecond, checkType: "status"},
			{name: "performance_check", duration: 25 * time.Millisecond, checkType: "performance"},
		}

		for _, job := range adminJobs {
			err := pool.Submit(job, nil)
			assert.NoError(t, err, "Administrative job should be submitted")
		}

		// Monitor administrative operations
		results := make([]JobResult, 0)
		timeout := time.After(2 * time.Second)

		for len(results) < len(adminJobs) {
			select {
			case result := <-pool.GetResults():
				results = append(results, result)

				// Verify pool remains healthy during administration
				assert.True(t, pool.IsHealthy(), "Pool should remain healthy during admin operations")

			case <-timeout:
				t.Fatalf("Timeout during administrative operations, got %d/%d", len(results), len(adminJobs))
			}
		}

		// Verify all administrative operations succeeded
		for _, result := range results {
			assert.NoError(t, result.Error, "Administrative operation should succeed")
			assert.True(t, result.Result.Success, "Administrative result should be successful")
		}

		// Final health check
		assert.True(t, pool.IsHealthy(), "Pool should be healthy after all admin operations")
	})
}

func TestPerformanceTuningAndOptimization(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance tuning test in short mode")
	}

	ctx := context.Background()

	// Test different performance configurations
	performanceConfigs := []struct {
		name         string
		workers      int
		rateLimitRPS float64
		burstSize    int
		memoryLimitMB int
		expectedThroughput float64 // minimum expected jobs/sec
	}{
		{
			name:               "HighThroughput",
			workers:            8,
			rateLimitRPS:       100,
			burstSize:          10,
			memoryLimitMB:      100,
			expectedThroughput: 50,
		},
		{
			name:               "Balanced",
			workers:            4,
			rateLimitRPS:       50,
			burstSize:          5,
			memoryLimitMB:      50,
			expectedThroughput: 25,
		},
		{
			name:               "Conservative",
			workers:            2,
			rateLimitRPS:       20,
			burstSize:          3,
			memoryLimitMB:      25,
			expectedThroughput: 10,
		},
	}

	for _, config := range performanceConfigs {
		t.Run(config.name, func(t *testing.T) {
			// Create pool with specific configuration
			pool, err := NewSecureWorkerPool(ctx, config.workers)
			require.NoError(t, err)

			// Configure rate limiting
			pool.rateLimiter.SetLimit(rate.Limit(config.rateLimitRPS))
			pool.rateLimiter.SetBurst(config.burstSize)

			// Configure memory limit
			// Note: In a real implementation, this would configure the memory monitor limit
			_ = config.memoryLimitMB // Use the memory limit for validation

			err = pool.Start()
			require.NoError(t, err)
			defer pool.Stop()

			// Performance test with optimized jobs
			jobCount := 200
			optimizedJobs := make([]*PerformanceOptimizationCheck, jobCount)

			for i := 0; i < jobCount; i++ {
				optimizedJobs[i] = &PerformanceOptimizationCheck{
					name:        fmt.Sprintf("perf_%s_%d", config.name, i),
					workloadType: "optimized",
					duration:    time.Duration(i%5+1) * time.Millisecond, // 1-5ms varied workload
				}
			}

			// Execute performance test
			startTime := time.Now()

			for _, job := range optimizedJobs {
				err := pool.Submit(job, nil)
				require.NoError(t, err)
			}

			// Collect results
			results := make([]JobResult, 0)
			timeout := time.After(30 * time.Second)

			for len(results) < jobCount {
				select {
				case result := <-pool.GetResults():
					results = append(results, result)
				case <-timeout:
					t.Fatalf("Performance test timeout for %s: got %d/%d", config.name, len(results), jobCount)
				}
			}

			executionTime := time.Since(startTime)
			throughput := float64(jobCount) / executionTime.Seconds()

			// Analyze performance metrics
			metrics := pool.GetMetrics()

			t.Logf("%s Performance Results:", config.name)
			t.Logf("  Workers: %d", config.workers)
			t.Logf("  Rate Limit: %.2f req/sec", config.rateLimitRPS)
			t.Logf("  Memory Limit: %d MB", config.memoryLimitMB)
			t.Logf("  Execution Time: %v", executionTime)
			t.Logf("  Throughput: %.2f jobs/sec", throughput)
			t.Logf("  Expected: %.2f jobs/sec", config.expectedThroughput)
			t.Logf("  Min Job Time: %v", metrics.MinJobTime)
			t.Logf("  Max Job Time: %v", metrics.MaxJobTime)
			t.Logf("  Average Job Time: %v", metrics.AverageJobTime)
			t.Logf("  Peak Memory: %.2f MB", metrics.PeakMemoryMB)

			// Performance assertions
			assert.GreaterOrEqual(t, throughput, config.expectedThroughput,
				"Throughput should meet minimum expectations for %s", config.name)

			assert.Equal(t, jobCount, metrics.CompletedJobs,
				"All jobs should complete for %s", config.name)

			assert.Equal(t, 0, metrics.FailedJobs,
				"No jobs should fail for %s", config.name)

			assert.LessOrEqual(t, metrics.PeakMemoryMB, float64(config.memoryLimitMB),
				"Memory usage should stay within limits for %s", config.name)

			// Verify optimization effectiveness
			efficiencyRatio := throughput / float64(config.workers)
			t.Logf("  Efficiency: %.2f jobs/sec per worker", efficiencyRatio)

			minEfficiency := 5.0 // At least 5 jobs/sec per worker
			assert.GreaterOrEqual(t, efficiencyRatio, minEfficiency,
				"Worker efficiency should be reasonable for %s", config.name)
		})
	}
}

func TestOperationalMetricsAndDashboards(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 6)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Create dashboard-style monitoring
	dashboard := &OperationalDashboard{
		pool:          pool,
		updateInterval: 100 * time.Millisecond,
		snapshots:     make([]DashboardSnapshot, 0),
	}

	// Start dashboard monitoring
	dashboardCtx, cancelDashboard := context.WithCancel(ctx)
	defer cancelDashboard()

	go dashboard.Monitor(dashboardCtx)

	// Submit varied workload for comprehensive metrics
	metricJobs := []*MetricsTestCheck{
		// CPU intensive jobs
		{name: "cpu_heavy_1", checkType: "cpu", intensity: 3, duration: 100 * time.Millisecond},
		{name: "cpu_heavy_2", checkType: "cpu", intensity: 5, duration: 150 * time.Millisecond},
		{name: "cpu_heavy_3", checkType: "cpu", intensity: 4, duration: 120 * time.Millisecond},

		// Memory intensive jobs
		{name: "mem_heavy_1", checkType: "memory", intensity: 2, duration: 80 * time.Millisecond},
		{name: "mem_heavy_2", checkType: "memory", intensity: 3, duration: 110 * time.Millisecond},

		// Mixed workload jobs
		{name: "mixed_1", checkType: "mixed", intensity: 2, duration: 90 * time.Millisecond},
		{name: "mixed_2", checkType: "mixed", intensity: 3, duration: 130 * time.Millisecond},

		// Light jobs
		{name: "light_1", checkType: "light", intensity: 1, duration: 20 * time.Millisecond},
		{name: "light_2", checkType: "light", intensity: 1, duration: 30 * time.Millisecond},
		{name: "light_3", checkType: "light", intensity: 1, duration: 25 * time.Millisecond},

		// Error simulation
		{name: "error_job", checkType: "error", intensity: 1, duration: 50 * time.Millisecond},
	}

	// Submit jobs with timing to create realistic workload patterns
	for i, job := range metricJobs {
		err := pool.Submit(job, nil)
		require.NoError(t, err)

		// Stagger submissions slightly
		if i%3 == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Collect results while dashboard monitors
	results := make([]JobResult, 0)
	timeout := time.After(10 * time.Second)

	for len(results) < len(metricJobs) {
		select {
		case result := <-pool.GetResults():
			results = append(results, result)
		case <-timeout:
			t.Fatalf("Timeout collecting metrics test results, got %d/%d", len(results), len(metricJobs))
		}
	}

	// Stop dashboard monitoring and analyze
	cancelDashboard()
	time.Sleep(200 * time.Millisecond) // Allow final snapshot

	// Analyze dashboard data
	snapshots := dashboard.GetSnapshots()
	assert.Greater(t, len(snapshots), 0, "Dashboard should have captured snapshots")

	t.Logf("Operational Metrics Dashboard Analysis:")
	t.Logf("  Total Snapshots: %d", len(snapshots))

	if len(snapshots) > 0 {
		firstSnapshot := snapshots[0]
		lastSnapshot := snapshots[len(snapshots)-1]

		t.Logf("  Initial Metrics:")
		t.Logf("    Active Jobs: %d", firstSnapshot.ActiveJobs)
		t.Logf("    Completed Jobs: %d", firstSnapshot.CompletedJobs)
		t.Logf("    Memory Usage: %.2f MB", firstSnapshot.MemoryUsageMB)

		t.Logf("  Final Metrics:")
		t.Logf("    Active Jobs: %d", lastSnapshot.ActiveJobs)
		t.Logf("    Completed Jobs: %d", lastSnapshot.CompletedJobs)
		t.Logf("    Memory Usage: %.2f MB", lastSnapshot.MemoryUsageMB)
		t.Logf("    Health Status: %v", lastSnapshot.HealthStatus)

		// Verify metrics progression
		assert.GreaterOrEqual(t, lastSnapshot.CompletedJobs, firstSnapshot.CompletedJobs,
			"Completed jobs should increase over time")

		assert.True(t, lastSnapshot.HealthStatus, "Pool should be healthy at the end")
	}

	// Verify final pool state
	finalMetrics := pool.GetMetrics()
	t.Logf("  Final Pool Metrics:")
	t.Logf("    Total Jobs: %d", finalMetrics.TotalJobs)
	t.Logf("    Completed: %d", finalMetrics.CompletedJobs)
	t.Logf("    Failed: %d", finalMetrics.FailedJobs)
	t.Logf("    API Calls: %d", finalMetrics.TotalAPIcalls)
	t.Logf("    Peak Memory: %.2f MB", finalMetrics.PeakMemoryMB)

	// Should have processed most jobs successfully (allowing for 1 error job)
	assert.Equal(t, len(metricJobs), finalMetrics.TotalJobs, "All jobs should be recorded")
	assert.GreaterOrEqual(t, finalMetrics.CompletedJobs, len(metricJobs)-1, "Most jobs should complete")
	assert.LessOrEqual(t, finalMetrics.FailedJobs, 1, "At most 1 job should fail (error job)")
}

// Helper types for configuration testing

type ConfigurationTestCheck struct {
	name     string
	duration time.Duration
}

func (c *ConfigurationTestCheck) Name() string        { return c.name }
func (c *ConfigurationTestCheck) Description() string { return "Configuration test check" }
func (c *ConfigurationTestCheck) Severity() Severity  { return SeverityInfo }

func (c *ConfigurationTestCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	time.Sleep(c.duration)
	return &CheckResult{
		CheckName: c.name,
		Success:   true,
		Message:   "Configuration test completed successfully",
		Timestamp: time.Now(),
	}, nil
}

type MonitoringTestCheck struct {
	name     string
	duration time.Duration
	jobType  string
}

func (m *MonitoringTestCheck) Name() string        { return m.name }
func (m *MonitoringTestCheck) Description() string { return "Monitoring test check" }
func (m *MonitoringTestCheck) Severity() Severity  { return SeverityInfo }

func (m *MonitoringTestCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	switch m.jobType {
	case "memory":
		// Allocate some memory for monitoring
		data := make([]byte, 1024*100) // 100KB
		for i := range data {
			data[i] = byte(i % 256)
		}
		time.Sleep(m.duration)
		data = nil // Help GC
		
	case "cpu":
		// CPU intensive work
		result := 0
		iterations := int(m.duration.Nanoseconds() / 1000) // Rough scaling
		for i := 0; i < iterations; i++ {
			result += i * i
		}
		
	default:
		time.Sleep(m.duration)
	}

	return &CheckResult{
		CheckName: m.name,
		Success:   true,
		Message:   fmt.Sprintf("Monitoring test (%s) completed", m.jobType),
		Details: map[string]interface{}{
			"job_type": m.jobType,
			"duration": m.duration,
		},
		Timestamp: time.Now(),
	}, nil
}

type AdministrativeTestCheck struct {
	name      string
	duration  time.Duration
	checkType string
}

func (a *AdministrativeTestCheck) Name() string        { return a.name }
func (a *AdministrativeTestCheck) Description() string { return "Administrative test check" }
func (a *AdministrativeTestCheck) Severity() Severity  { return SeverityInfo }

func (a *AdministrativeTestCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	time.Sleep(a.duration)

	var message string
	switch a.checkType {
	case "health":
		message = "Health check completed - system operational"
	case "metrics":
		message = "Metrics collection completed - data available"
	case "status":
		message = "Status check completed - all systems normal"
	case "performance":
		message = "Performance check completed - within thresholds"
	default:
		message = "Administrative operation completed"
	}

	return &CheckResult{
		CheckName: a.name,
		Success:   true,
		Message:   message,
		Details: map[string]interface{}{
			"check_type": a.checkType,
			"admin_op":   true,
		},
		Timestamp: time.Now(),
	}, nil
}

type PerformanceOptimizationCheck struct {
	name         string
	workloadType string
	duration     time.Duration
}

func (p *PerformanceOptimizationCheck) Name() string        { return p.name }
func (p *PerformanceOptimizationCheck) Description() string { return "Performance optimization check" }
func (p *PerformanceOptimizationCheck) Severity() Severity  { return SeverityInfo }

func (p *PerformanceOptimizationCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	switch p.workloadType {
	case "optimized":
		// Optimized workload - minimal overhead
		time.Sleep(p.duration)
		
	case "heavy":
		// Heavy workload for performance testing
		data := make([]byte, 1024*50) // 50KB
		for i := range data {
			data[i] = byte(i % 256)
		}
		time.Sleep(p.duration)
		data = nil
		
	default:
		time.Sleep(p.duration)
	}

	return &CheckResult{
		CheckName: p.name,
		Success:   true,
		Message:   fmt.Sprintf("Performance optimization check (%s) completed", p.workloadType),
		Timestamp: time.Now(),
	}, nil
}

type MetricsTestCheck struct {
	name      string
	checkType string
	intensity int
	duration  time.Duration
}

func (m *MetricsTestCheck) Name() string        { return m.name }
func (m *MetricsTestCheck) Description() string { return "Metrics test check" }
func (m *MetricsTestCheck) Severity() Severity  { return SeverityInfo }

func (m *MetricsTestCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	switch m.checkType {
	case "cpu":
		return m.runCPUWorkload(ctx)
	case "memory":
		return m.runMemoryWorkload(ctx)
	case "mixed":
		return m.runMixedWorkload(ctx)
	case "light":
		return m.runLightWorkload(ctx)
	case "error":
		return m.runErrorWorkload(ctx)
	default:
		return m.runDefaultWorkload(ctx)
	}
}

func (m *MetricsTestCheck) runCPUWorkload(ctx context.Context) (*CheckResult, error) {
	iterations := m.intensity * 50000
	result := 0
	for i := 0; i < iterations; i++ {
		result += i * i
		if i%10000 == 0 {
			time.Sleep(time.Microsecond) // Tiny sleep to be responsive
		}
	}
	time.Sleep(m.duration - time.Duration(iterations)*time.Microsecond)

	return &CheckResult{
		CheckName: m.name,
		Success:   true,
		Message:   fmt.Sprintf("CPU workload completed (result: %d)", result),
		Timestamp: time.Now(),
	}, nil
}

func (m *MetricsTestCheck) runMemoryWorkload(ctx context.Context) (*CheckResult, error) {
	allocSize := m.intensity * 1024 * 512 // 512KB per intensity level
	data := make([]byte, allocSize)
	for i := range data {
		data[i] = byte(i % 256)
	}
	
	time.Sleep(m.duration)
	
	checksum := 0
	for _, b := range data {
		checksum += int(b)
	}
	
	data = nil // Help GC

	return &CheckResult{
		CheckName: m.name,
		Success:   true,
		Message:   fmt.Sprintf("Memory workload completed (checksum: %d)", checksum),
		Timestamp: time.Now(),
	}, nil
}

func (m *MetricsTestCheck) runMixedWorkload(ctx context.Context) (*CheckResult, error) {
	// CPU work
	result := 0
	for i := 0; i < m.intensity*10000; i++ {
		result += i * i
	}
	
	// Memory work
	data := make([]byte, m.intensity*1024*100) // 100KB per intensity
	for i := range data {
		data[i] = byte(i % 256)
	}
	
	time.Sleep(m.duration / 2)
	data = nil

	return &CheckResult{
		CheckName: m.name,
		Success:   true,
		Message:   fmt.Sprintf("Mixed workload completed (cpu result: %d)", result),
		Timestamp: time.Now(),
	}, nil
}

func (m *MetricsTestCheck) runLightWorkload(ctx context.Context) (*CheckResult, error) {
	time.Sleep(m.duration)
	return &CheckResult{
		CheckName: m.name,
		Success:   true,
		Message:   "Light workload completed",
		Timestamp: time.Now(),
	}, nil
}

func (m *MetricsTestCheck) runErrorWorkload(ctx context.Context) (*CheckResult, error) {
	time.Sleep(m.duration)
	return &CheckResult{
		CheckName: m.name,
		Success:   false,
		Message:   "Simulated error for metrics testing",
		Timestamp: time.Now(),
	}, fmt.Errorf("simulated error")
}

func (m *MetricsTestCheck) runDefaultWorkload(ctx context.Context) (*CheckResult, error) {
	time.Sleep(m.duration)
	return &CheckResult{
		CheckName: m.name,
		Success:   true,
		Message:   "Default workload completed",
		Timestamp: time.Now(),
	}, nil
}

// Dashboard monitoring helper

type DashboardSnapshot struct {
	Timestamp     time.Time
	ActiveJobs    int32
	CompletedJobs int
	FailedJobs    int
	MemoryUsageMB float64
	HealthStatus  bool
}

type OperationalDashboard struct {
	pool           *SecureWorkerPool
	updateInterval time.Duration
	snapshots      []DashboardSnapshot
	mutex          sync.RWMutex
}

func (d *OperationalDashboard) Monitor(ctx context.Context) {
	ticker := time.NewTicker(d.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.takeSnapshot()
		case <-ctx.Done():
			return
		}
	}
}

func (d *OperationalDashboard) takeSnapshot() {
	metrics := d.pool.GetMetrics()
	
	snapshot := DashboardSnapshot{
		Timestamp:     time.Now(),
		ActiveJobs:    d.pool.activeJobs.Load(),
		CompletedJobs: metrics.CompletedJobs,
		FailedJobs:    metrics.FailedJobs,
		MemoryUsageMB: metrics.PeakMemoryMB,
		HealthStatus:  d.pool.IsHealthy(),
	}

	d.mutex.Lock()
	d.snapshots = append(d.snapshots, snapshot)
	d.mutex.Unlock()
}

func (d *OperationalDashboard) GetSnapshots() []DashboardSnapshot {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	
	result := make([]DashboardSnapshot, len(d.snapshots))
	copy(result, d.snapshots)
	return result
}