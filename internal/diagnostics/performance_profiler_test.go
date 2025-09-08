// Package diagnostics provides tests for the performance profiling infrastructure
package diagnostics

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/diagnostics/profiling"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// TestPerformanceProfilerCreation tests profiler creation
func TestPerformanceProfilerCreation(t *testing.T) {
	config := &ProfileConfig{
		Enabled:           true,
		Precision:         1 * time.Millisecond,
		MaxOverhead:       0.05,
		SamplingRate:      100 * time.Microsecond,
		MaxDataPoints:     1000,
		EnablePercentiles: true,
		DetailLevel:       DetailLevelNormal,
	}

	profiler := NewPerformanceProfiler(config)
	if profiler == nil {
		t.Fatal("Failed to create profiler")
	}

	if !profiler.IsEnabled() {
		t.Error("Profiler should be enabled")
	}

	if profiler.precision != 1*time.Millisecond {
		t.Errorf("Expected precision 1ms, got %v", profiler.precision)
	}
}

// TestPrecisionTimerAccuracy tests the 1ms accuracy requirement
func TestPrecisionTimerAccuracy(t *testing.T) {
	// Validate timer accuracy
	err := profiling.ValidateTimerAccuracy()
	if err != nil {
		t.Errorf("Timer accuracy validation failed: %v", err)
	}

	// Test with multiple measurements
	timer := profiling.NewPrecisionTimer()
	durations := make([]time.Duration, 10)

	for i := range durations {
		timer.Start()
		time.Sleep(5 * time.Millisecond)
		durations[i] = timer.Stop()
		timer.Reset()
	}

	// Check that all measurements are within 1ms of expected
	expected := 5 * time.Millisecond
	for i, d := range durations {
		diff := d - expected
		if diff < 0 {
			diff = -diff
		}
		if diff > time.Millisecond {
			t.Errorf("Measurement %d: expected ~%v, got %v (diff: %v)", i, expected, d, diff)
		}
	}
}

// TestProfileOperation tests profiling individual operations
func TestProfileOperation(t *testing.T) {
	profiler := NewPerformanceProfiler(nil)
	err := profiler.Start()
	if err != nil {
		t.Fatalf("Failed to start profiler: %v", err)
	}
	defer profiler.Stop()

	// Profile a simple operation
	err = profiler.ProfileOperation("test_op", "test_category", func() error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	if err != nil {
		t.Errorf("ProfileOperation failed: %v", err)
	}

	// Check metrics
	metrics := profiler.GetMetrics()
	if metrics.TotalOperations != 1 {
		t.Errorf("Expected 1 operation, got %d", metrics.TotalOperations)
	}

	// Duration should be approximately 10ms
	if metrics.AverageDuration < 9*time.Millisecond || metrics.AverageDuration > 12*time.Millisecond {
		t.Errorf("Expected duration ~10ms, got %v", metrics.AverageDuration)
	}
}

// TestProfilerOverhead tests that profiling overhead is within limits
func TestProfilerOverhead(t *testing.T) {
	profiler := NewPerformanceProfiler(&ProfileConfig{
		Enabled:     true,
		MaxOverhead: 0.05, // 5% max overhead
	})

	err := profiler.Start()
	if err != nil {
		t.Fatalf("Failed to start profiler: %v", err)
	}
	defer profiler.Stop()

	// Run many operations to measure overhead
	for i := 0; i < 100; i++ {
		profiler.ProfileOperation(fmt.Sprintf("op_%d", i), "overhead_test", func() error {
			// Minimal operation
			time.Sleep(1 * time.Millisecond)
			return nil
		})
	}

	// Check overhead is within limits
	if !profiler.IsWithinOverheadLimit() {
		t.Errorf("Profiling overhead exceeds limit: %.2f%%", profiler.GetOverheadPercentage())
	}
}

// TestConcurrentProfiling tests thread-safe concurrent profiling
func TestConcurrentProfiling(t *testing.T) {
	profiler := NewPerformanceProfiler(nil)
	err := profiler.Start()
	if err != nil {
		t.Fatalf("Failed to start profiler: %v", err)
	}
	defer profiler.Stop()

	var wg sync.WaitGroup
	numGoroutines := 10
	opsPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				profiler.ProfileOperation(
					fmt.Sprintf("concurrent_%d_%d", id, j),
					"concurrent",
					func() error {
						time.Sleep(time.Microsecond * 100)
						return nil
					},
				)
			}
		}(i)
	}

	wg.Wait()

	// Verify all operations were recorded
	metrics := profiler.GetMetrics()
	expectedOps := int64(numGoroutines * opsPerGoroutine)
	if metrics.TotalOperations != expectedOps {
		t.Errorf("Expected %d operations, got %d", expectedOps, metrics.TotalOperations)
	}
}

// TestPercentileCalculation tests percentile calculations
func TestPercentileCalculation(t *testing.T) {
	profiler := NewPerformanceProfiler(&ProfileConfig{
		Enabled: true,
		EnablePercentiles: true,
	})
	
	err := profiler.Start()
	if err != nil {
		t.Fatalf("Failed to start profiler: %v", err)
	}
	defer profiler.Stop()

	// Create operations with varying durations
	durations := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		3 * time.Millisecond,
		4 * time.Millisecond,
		5 * time.Millisecond,
		10 * time.Millisecond,
		20 * time.Millisecond,
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
	}

	for i, d := range durations {
		profiler.ProfileOperation(fmt.Sprintf("percentile_op_%d", i), "percentile", func() error {
			time.Sleep(d)
			return nil
		})
	}

	// Force calculation of percentiles
	profiler.calculateFinalMetrics()
	
	metrics := profiler.GetMetrics()
	
	// Check P50 (median should be around 5-10ms)
	p50 := metrics.Percentiles[50]
	if p50 < 4*time.Millisecond || p50 > 15*time.Millisecond {
		t.Errorf("P50 out of expected range: %v", p50)
	}

	// Check P95 (should be around 100-200ms)
	p95 := metrics.Percentiles[95]
	if p95 < 50*time.Millisecond {
		t.Errorf("P95 seems too low: %v", p95)
	}

	// Check P99 (should be close to 200ms)
	p99 := metrics.Percentiles[99]
	if p99 < 100*time.Millisecond {
		t.Errorf("P99 seems too low: %v", p99)
	}
}

// TestProfileCheck tests profiling diagnostic checks
func TestProfileCheck(t *testing.T) {
	profiler := NewPerformanceProfiler(nil)
	err := profiler.Start()
	if err != nil {
		t.Fatalf("Failed to start profiler: %v", err)
	}
	defer profiler.Stop()

	// Create a mock check
	check := &mockCheck{
		name:     "test_check",
		duration: 15 * time.Millisecond,
	}

	// Create a mock Docker client
	client := &docker.Client{}

	// Profile the check
	result, err := profiler.ProfileCheck(check, client)
	if err != nil {
		t.Errorf("ProfileCheck failed: %v", err)
	}

	if result == nil {
		t.Error("Expected result from ProfileCheck")
	}

	// Verify check metrics
	metrics := profiler.GetMetrics()
	checkMetrics, exists := metrics.CheckMetrics["test_check"]
	if !exists {
		t.Error("Check metrics not recorded")
	} else {
		if checkMetrics.ExecutionCount != 1 {
			t.Errorf("Expected 1 execution, got %d", checkMetrics.ExecutionCount)
		}
	}
}

// TestWorkerPoolIntegration tests integration with worker pool
func TestWorkerPoolIntegration(t *testing.T) {
	// Skip in short mode as this involves more complex setup
	if testing.Short() {
		t.Skip("Skipping worker pool integration test in short mode")
	}

	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 4)
	if err != nil {
		t.Fatalf("Failed to create worker pool: %v", err)
	}

	profiler := NewPerformanceProfiler(nil)
	profiler.SetIntegration(pool, nil, nil)
	
	err = profiler.Start()
	if err != nil {
		t.Fatalf("Failed to start profiler: %v", err)
	}
	defer profiler.Stop()

	// Profile a worker job
	job := Job{
		ID: 1,
		Check: &mockCheck{
			name:     "worker_check",
			duration: 5 * time.Millisecond,
		},
		Context: ctx,
		Client:  &docker.Client{},
	}

	result, err := profiler.ProfileWorkerJob(0, job)
	if err != nil {
		t.Errorf("ProfileWorkerJob failed: %v", err)
	}

	if result == nil {
		t.Error("Expected result from ProfileWorkerJob")
	}

	// Check worker metrics
	metrics := profiler.GetMetrics()
	workerMetrics, exists := metrics.WorkerMetrics[0]
	if !exists {
		t.Error("Worker metrics not recorded")
	} else {
		if workerMetrics.JobsProcessed != 1 {
			t.Errorf("Expected 1 job processed, got %d", workerMetrics.JobsProcessed)
		}
	}
}

// TestMemoryUsageTracking tests memory usage tracking
func TestMemoryUsageTracking(t *testing.T) {
	profiler := NewPerformanceProfiler(&ProfileConfig{
		Enabled: true,
		EnableTrends:    true,
		RealtimeMetrics: true,
	})

	err := profiler.Start()
	if err != nil {
		t.Fatalf("Failed to start profiler: %v", err)
	}
	defer profiler.Stop()

	// Allocate some memory during operation
	profiler.ProfileOperation("memory_test", "memory", func() error {
		// Allocate 1MB
		data := make([]byte, 1024*1024)
		// Use it to prevent optimization
		for i := range data {
			data[i] = byte(i % 256)
		}
		time.Sleep(1 * time.Millisecond)
		return nil
	})

	// Check that resource snapshot was captured
	timing := profiler.measurements.GetLatest()
	if timing == nil {
		t.Fatal("No timing data recorded")
	}

	if timing.ResourceUsage == nil {
		t.Error("Resource usage not captured")
	} else {
		if timing.ResourceUsage.MemoryBytes == 0 {
			t.Error("Memory usage not tracked")
		}
		if timing.ResourceUsage.GoroutineNum == 0 {
			t.Error("Goroutine count not tracked")
		}
	}
}

// TestReportGeneration tests report generation
func TestReportGeneration(t *testing.T) {
	profiler := NewPerformanceProfiler(&ProfileConfig{
		Enabled: true,
		EnablePercentiles: true,
	})

	err := profiler.Start()
	if err != nil {
		t.Fatalf("Failed to start profiler: %v", err)
	}

	// Run some operations
	for i := 0; i < 5; i++ {
		profiler.ProfileOperation(fmt.Sprintf("report_op_%d", i), "report_test", func() error {
			time.Sleep(time.Duration(i+1) * time.Millisecond)
			return nil
		})
	}

	err = profiler.Stop()
	if err != nil {
		t.Errorf("Failed to stop profiler: %v", err)
	}

	// Generate report
	report := profiler.GenerateReport()
	if report == "" {
		t.Error("Empty report generated")
	}

	// Check report contains expected sections
	expectedSections := []string{
		"PERFORMANCE PROFILING REPORT",
		"EXECUTIVE SUMMARY",
		"TIMING ANALYSIS",
		"CATEGORY BREAKDOWN",
		"PROFILING OVERHEAD",
	}

	for _, section := range expectedSections {
		if !contains(report, section) {
			t.Errorf("Report missing section: %s", section)
		}
	}
}

// TestTimingStorageCapacity tests that timing storage respects capacity limits
func TestTimingStorageCapacity(t *testing.T) {
	storage := NewTimingStorage(10) // Small capacity for testing

	// Add more than capacity
	for i := 0; i < 20; i++ {
		storage.Store(&TimingData{
			Name:     fmt.Sprintf("timing_%d", i),
			Duration: time.Duration(i) * time.Millisecond,
			Category: "test",
		})
	}

	// Should only have 10 items max
	if storage.Size() > 10 {
		t.Errorf("Storage exceeded capacity: %d > 10", storage.Size())
	}

	// Latest should be the most recent
	latest := storage.GetLatest()
	if latest == nil || latest.Name != "timing_19" {
		t.Error("Latest timing not correct")
	}
}

// BenchmarkProfileOperation benchmarks the profiling overhead
func BenchmarkProfileOperation(b *testing.B) {
	profiler := NewPerformanceProfiler(nil)
	profiler.Start()
	defer profiler.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		profiler.ProfileOperation("bench_op", "benchmark", func() error {
			// Minimal operation
			return nil
		})
	}
}

// BenchmarkPrecisionTimer benchmarks the precision timer
func BenchmarkPrecisionTimer(b *testing.B) {
	timer := profiling.NewPrecisionTimer()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timer.Start()
		// Minimal operation
		_ = timer.Stop()
		timer.Reset()
	}
}

// Helper types and functions

// mockCheck is a mock implementation of the Check interface
type mockCheck struct {
	name     string
	duration time.Duration
}

func (m *mockCheck) Name() string        { return m.name }
func (m *mockCheck) Description() string { return "Mock check for testing" }
func (m *mockCheck) Severity() Severity  { return SeverityInfo }
func (m *mockCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	time.Sleep(m.duration)
	return &CheckResult{
		CheckName: m.name,
		Success:   true,
		Message:   "Mock check completed",
		Timestamp: time.Now(),
		Duration:  m.duration,
	}, nil
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && len(s) >= len(substr) && 
		(s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr)))
}