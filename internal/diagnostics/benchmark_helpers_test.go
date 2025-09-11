package diagnostics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// TestNewBenchmarkMetrics tests the NewBenchmarkMetrics constructor
func TestNewBenchmarkMetrics(t *testing.T) {
	metrics := NewBenchmarkMetrics()

	assert.NotNil(t, metrics)
	assert.NotNil(t, metrics.CheckDurations)
	assert.Empty(t, metrics.CheckDurations)
}

// TestBenchmarkMetricsRecordAndGet tests recording and retrieving check durations
func TestBenchmarkMetricsRecordAndGet(t *testing.T) {
	metrics := NewBenchmarkMetrics()

	// Record some check durations
	metrics.RecordCheckDuration("check1", 100*time.Millisecond)
	metrics.RecordCheckDuration("check2", 200*time.Millisecond)
	metrics.RecordCheckDuration("check1", 150*time.Millisecond) // Overwrites previous check1

	// Get durations
	check1Duration := metrics.GetCheckDuration("check1")
	check2Duration := metrics.GetCheckDuration("check2")
	nonExistentDuration := metrics.GetCheckDuration("nonexistent")

	assert.Equal(t, 150*time.Millisecond, check1Duration) // Latest recorded value
	assert.Equal(t, 200*time.Millisecond, check2Duration)
	assert.Equal(t, time.Duration(0), nonExistentDuration) // Zero value for non-existent
}

// TestCalculateParallelSpeedup tests the parallel speedup calculation
func TestCalculateParallelSpeedup(t *testing.T) {
	metrics := NewBenchmarkMetrics()

	// Test with zero durations (edge case)
	metrics.CalculateParallelSpeedup(0, 0)
	assert.Equal(t, 0.0, metrics.ParallelSpeedup)

	// Test with zero parallel duration (should be no change)
	metrics.CalculateParallelSpeedup(100*time.Millisecond, 0)
	assert.Equal(t, 0.0, metrics.ParallelSpeedup) // No change since parallelTime is 0

	// Test normal case
	sequentialTime := 1000 * time.Millisecond
	parallelTime := 300 * time.Millisecond
	metrics.CalculateParallelSpeedup(sequentialTime, parallelTime)
	expected := float64(sequentialTime) / float64(parallelTime)
	assert.InDelta(t, expected, metrics.ParallelSpeedup, 0.01)

	// Test case where parallel is slower (speedup < 1)
	metrics.CalculateParallelSpeedup(100*time.Millisecond, 200*time.Millisecond)
	assert.Equal(t, 0.5, metrics.ParallelSpeedup)
}

// TestNewBenchmarkEngine tests the NewBenchmarkEngine constructor
func TestNewBenchmarkEngine(t *testing.T) {
	ctx := context.Background()

	// Create Docker client for the test
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		t.Skip("Skipping test - Docker not available:", err)
		return
	}
	defer dockerClient.Close()

	config := &Config{
		Timeout:  30 * time.Second,
		Verbose:  false,
		Parallel: false,
	}

	// Create regular engine first
	engine := NewEngine(dockerClient, config)
	benchEngine := NewBenchmarkEngine(engine)

	assert.NotNil(t, benchEngine)
	assert.NotNil(t, benchEngine.DiagnosticEngine)
	assert.NotNil(t, benchEngine.metrics)
}

// TestBenchmarkEngineRunWithMetrics tests the RunWithMetrics method
func TestBenchmarkEngineRunWithMetrics(t *testing.T) {
	ctx := context.Background()

	// Create Docker client for the test
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		t.Skip("Skipping test - Docker not available:", err)
		return
	}
	defer dockerClient.Close()

	config := &Config{
		Timeout:  30 * time.Second,
		Verbose:  false,
		Parallel: false,
	}

	// Create regular engine first
	engine := NewEngine(dockerClient, config)
	benchEngine := NewBenchmarkEngine(engine)

	// Run with metrics
	results, metrics, err := benchEngine.RunWithMetrics(ctx)

	require.NoError(t, err)
	assert.NotNil(t, results)
	assert.NotNil(t, metrics)
	assert.Greater(t, metrics.TotalDuration, time.Duration(0))
}

// TestNewMockCheck tests the NewMockCheck constructor
func TestNewMockCheck(t *testing.T) {
	name := "test_mock"
	duration := 100 * time.Millisecond
	shouldFail := true

	mockCheck := NewMockCheck(name, duration, shouldFail)

	assert.Equal(t, name, mockCheck.Name())
	assert.NotEmpty(t, mockCheck.Description())
	assert.Equal(t, SeverityInfo, mockCheck.Severity())
}

// TestMockCheckRun tests the mock check execution
func TestMockCheckRun(t *testing.T) {
	ctx := context.Background()

	// Test successful mock check
	successCheck := NewMockCheck("success_test", 10*time.Millisecond, false)
	result, err := successCheck.Run(ctx, nil) // MockCheck uses interface{}, can pass nil

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Contains(t, result.Message, "Mock check completed")

	// Test failing mock check
	failCheck := NewMockCheck("fail_test", 10*time.Millisecond, true)
	result, err = failCheck.Run(ctx, nil)

	require.NoError(t, err) // MockCheck returns success=false in result, not an error
	assert.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "Mock check completed")
}

// TestMockCheckWithTimeout tests mock check with context timeout
func TestMockCheckWithTimeout(t *testing.T) {
	// Create a timeout context shorter than the mock check duration
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	// Create mock check with longer duration
	mockCheck := NewMockCheck("timeout_test", 50*time.Millisecond, false)

	result, err := mockCheck.Run(ctx, nil)

	// Should return context timeout error
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "context")
}

// TestGetDefaultCheckGroups tests the default check groups function
func TestGetDefaultCheckGroups(t *testing.T) {
	groups := GetDefaultCheckGroups()

	assert.NotNil(t, groups)

	// Verify system checks
	assert.NotEmpty(t, groups.SystemChecks)
	for _, check := range groups.SystemChecks {
		assert.NotEmpty(t, check.Name())
		assert.NotEmpty(t, check.Description())
	}

	// Verify network checks
	assert.NotEmpty(t, groups.NetworkChecks)
	for _, check := range groups.NetworkChecks {
		assert.NotEmpty(t, check.Name())
		assert.NotEmpty(t, check.Description())
	}

	// Verify DNS checks if they exist
	if len(groups.DNSChecks) > 0 {
		for _, check := range groups.DNSChecks {
			assert.NotEmpty(t, check.Name())
			assert.NotEmpty(t, check.Description())
		}
	}

	// Verify Container checks if they exist
	if len(groups.ContainerChecks) > 0 {
		for _, check := range groups.ContainerChecks {
			assert.NotEmpty(t, check.Name())
			assert.NotEmpty(t, check.Description())
		}
	}
}

// TestNewOptimizedEngine tests the NewOptimizedEngine constructor
func TestNewOptimizedEngine(t *testing.T) {
	ctx := context.Background()

	// Create Docker client for the test
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		t.Skip("Skipping test - Docker not available:", err)
		return
	}
	defer dockerClient.Close()

	config := &Config{
		Timeout:     30 * time.Second,
		Verbose:     false,
		Parallel:    true,
		WorkerCount: 4,
	}

	// Create regular engine first
	engine := NewEngine(dockerClient, config)
	optimizedEngine := NewOptimizedEngine(engine, StrategyFullParallel)

	assert.NotNil(t, optimizedEngine)
	assert.NotNil(t, optimizedEngine.DiagnosticEngine)
	assert.Equal(t, StrategyFullParallel, optimizedEngine.strategy)
	assert.Equal(t, 10, optimizedEngine.maxWorkers) // Default value
}

// TestOptimizedEngineRunOptimized tests the RunOptimized method
func TestOptimizedEngineRunOptimized(t *testing.T) {
	ctx := context.Background()

	// Create Docker client for the test
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		t.Skip("Skipping test - Docker not available:", err)
		return
	}
	defer dockerClient.Close()

	config := &Config{
		Timeout:     30 * time.Second,
		Verbose:     false,
		Parallel:    true,
		WorkerCount: 2,
	}

	// Create regular engine first
	engine := NewEngine(dockerClient, config)
	optimizedEngine := NewOptimizedEngine(engine, StrategyGroupParallel)

	// Run optimized execution
	results, err := optimizedEngine.RunOptimized(ctx)

	require.NoError(t, err)
	assert.NotNil(t, results)
	assert.GreaterOrEqual(t, len(results.Checks), 0)
}

// TestOptimizedEngineRunAdaptive tests the adaptive execution mode
func TestOptimizedEngineRunAdaptive(t *testing.T) {
	ctx := context.Background()

	// Create Docker client for the test
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		t.Skip("Skipping test - Docker not available:", err)
		return
	}
	defer dockerClient.Close()

	config := &Config{
		Timeout:     30 * time.Second,
		Verbose:     false,
		Parallel:    true,
		WorkerCount: 2,
	}

	// Create regular engine first
	engine := NewEngine(dockerClient, config)
	optimizedEngine := NewOptimizedEngine(engine, StrategyAdaptive)

	// Test adaptive execution (internal method, tested indirectly through RunOptimized)
	// The key is that it runs without errors
	results, err := optimizedEngine.RunOptimized(ctx)

	require.NoError(t, err)
	assert.NotNil(t, results)
}

// TestBenchmarkHelpersIntegration tests integration between different benchmark helper components
func TestBenchmarkHelpersIntegration(t *testing.T) {
	ctx := context.Background()

	// Create Docker client for the test
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		t.Skip("Skipping test - Docker not available:", err)
		return
	}
	defer dockerClient.Close()

	// Create benchmark metrics and record some data
	metrics := NewBenchmarkMetrics()
	metrics.RecordCheckDuration("test_check", 100*time.Millisecond)
	metrics.RecordCheckDuration("test_check", 120*time.Millisecond) // Overwrites previous

	// Test speedup calculation
	sequentialTime := 220 * time.Millisecond // Sum of individual times
	parallelTime := 120 * time.Millisecond   // Max of individual times (parallel)
	metrics.CalculateParallelSpeedup(sequentialTime, parallelTime)

	assert.Greater(t, metrics.ParallelSpeedup, 1.0)

	// Create and test benchmark engine
	config := &Config{
		Timeout:  10 * time.Second,
		Verbose:  false,
		Parallel: true,
	}

	engine := NewEngine(dockerClient, config)
	benchEngine := NewBenchmarkEngine(engine)
	results, benchMetrics, err := benchEngine.RunWithMetrics(ctx)

	require.NoError(t, err)
	assert.NotNil(t, results)
	assert.NotNil(t, benchMetrics)
	assert.Greater(t, benchMetrics.TotalDuration, time.Duration(0))

	// Test optimized engine
	optimizedEngine := NewOptimizedEngine(engine, StrategySequential)
	optimizedResults, err := optimizedEngine.RunOptimized(ctx)

	require.NoError(t, err)
	assert.NotNil(t, optimizedResults)

	// Both engines should produce results
	assert.IsType(t, &Results{}, results)
	assert.IsType(t, &Results{}, optimizedResults)
}
