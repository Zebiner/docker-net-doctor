package diagnostics_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zebiner/docker-net-doctor/internal/diagnostics"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

func TestEngineCreation(t *testing.T) {
	// For testing, we'll create a real Docker client
	// In more sophisticated tests, you might use a mock
	ctx := context.Background()
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		t.Skip("Skipping test - Docker not available:", err)
		return
	}
	defer dockerClient.Close()

	// Test creating an engine with nil config (should use defaults)
	engine := diagnostics.NewEngine(dockerClient, nil)
	assert.NotNil(t, engine, "Engine should not be nil with nil config")

	// Test with custom config
	config := &diagnostics.Config{
		Timeout:  60 * time.Second,
		Verbose:  true,
		Parallel: false,
	}
	engine = diagnostics.NewEngine(dockerClient, config)
	assert.NotNil(t, engine, "Engine should not be nil with custom config")
}

func TestEngineRun(t *testing.T) {
	ctx := context.Background()

	// Create Docker client for the test
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		t.Skip("Skipping test - Docker not available:", err)
		return
	}
	defer dockerClient.Close()

	// Create engine with sequential execution for predictable test results
	config := &diagnostics.Config{
		Parallel: false,
		Timeout:  10 * time.Second,
	}
	engine := diagnostics.NewEngine(dockerClient, config)

	// Note: In the actual implementation, checks might be registered differently
	// For now, we'll just run the engine and see what happens
	results, err := engine.Run(ctx)
	require.NoError(t, err, "Engine.Run should not return an error")

	// The results should be a slice of CheckResult, not a Results struct
	// We need to adjust our assertions based on the actual return type
	assert.NotNil(t, results, "Results should not be nil")
}

// TestEngineGetRecommendations tests the GetRecommendations method with various result scenarios
func TestEngineGetRecommendations(t *testing.T) {
	ctx := context.Background()

	// Create Docker client for the test
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		t.Skip("Skipping test - Docker not available:", err)
		return
	}
	defer dockerClient.Close()

	// Create engine with custom config
	config := &diagnostics.Config{
		Timeout:  30 * time.Second,
		Verbose:  false,
		Parallel: false,
	}
	engine := diagnostics.NewEngine(dockerClient, config)

	// Run the engine to generate some results
	results, err := engine.Run(ctx)
	require.NoError(t, err, "Engine.Run should not return an error")
	require.NotNil(t, results, "Results should not be nil")

	// Test GetRecommendations method
	recommendations := engine.GetRecommendations()
	assert.NotNil(t, recommendations, "Recommendations should not be nil")
	// The recommendations should be a slice, even if empty
	assert.IsType(t, []diagnostics.Recommendation{}, recommendations)
}

// TestEngineGetExecutionReport tests the GetExecutionReport method
func TestEngineGetExecutionReport(t *testing.T) {
	ctx := context.Background()

	// Create Docker client for the test
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		t.Skip("Skipping test - Docker not available:", err)
		return
	}
	defer dockerClient.Close()

	// Create engine with custom config
	config := &diagnostics.Config{
		Timeout:  30 * time.Second,
		Verbose:  true,
		Parallel: false,
	}
	engine := diagnostics.NewEngine(dockerClient, config)

	// Run the engine to generate some results
	results, err := engine.Run(ctx)
	require.NoError(t, err, "Engine.Run should not return an error")
	require.NotNil(t, results, "Results should not be nil")

	// Test GetExecutionReport method
	report := engine.GetExecutionReport()
	assert.NotEmpty(t, report, "Execution report should not be empty")
	assert.Contains(t, report, "Execution", "Report should contain execution information")
}

// TestEngineRunParallel tests the parallel execution path
func TestEngineRunParallel(t *testing.T) {
	ctx := context.Background()

	// Create Docker client for the test
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		t.Skip("Skipping test - Docker not available:", err)
		return
	}
	defer dockerClient.Close()

	// Create engine with parallel execution enabled
	config := &diagnostics.Config{
		Timeout:     30 * time.Second,
		Verbose:     false,
		Parallel:    true, // Enable parallel execution
		WorkerCount: 2,
	}
	engine := diagnostics.NewEngine(dockerClient, config)

	// Run the engine in parallel mode
	results, err := engine.Run(ctx)
	require.NoError(t, err, "Engine.Run in parallel mode should not return an error")
	require.NotNil(t, results, "Results should not be nil")

	// Verify that we got results (exact content depends on available checks)
	// The key is that parallel execution completes without error
	assert.IsType(t, &diagnostics.Results{}, results, "Results should be of correct type")
}

// TestEngineRunWithWorkerPool tests the worker pool execution path
func TestEngineRunWithWorkerPool(t *testing.T) {
	ctx := context.Background()

	// Create Docker client for the test
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		t.Skip("Skipping test - Docker not available:", err)
		return
	}
	defer dockerClient.Close()

	// Create engine with worker pool configuration
	config := &diagnostics.Config{
		Timeout:     30 * time.Second,
		Verbose:     false,
		Parallel:    true,
		WorkerCount: 3,
		RateLimit:   10.0, // 10 requests per second
	}
	engine := diagnostics.NewEngine(dockerClient, config)

	// Run the engine with worker pool
	results, err := engine.Run(ctx)
	require.NoError(t, err, "Engine.Run with worker pool should not return an error")
	require.NotNil(t, results, "Results should not be nil")

	// Verify that we got results from worker pool execution
	assert.IsType(t, &diagnostics.Results{}, results, "Results should be of correct type")
}

// TestEngineRunWithTimeout tests timeout handling
func TestEngineRunWithTimeout(t *testing.T) {
	ctx := context.Background()

	// Create Docker client for the test
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		t.Skip("Skipping test - Docker not available:", err)
		return
	}
	defer dockerClient.Close()

	// Create engine with very short timeout to test timeout behavior
	config := &diagnostics.Config{
		Timeout:  1 * time.Millisecond, // Very short timeout
		Verbose:  false,
		Parallel: false,
	}
	engine := diagnostics.NewEngine(dockerClient, config)

	// Create a context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Millisecond)
	defer cancel()

	// Run the engine - it might succeed quickly or timeout
	results, err := engine.Run(timeoutCtx)

	// Either case is acceptable - the important thing is no panic/crash
	if err != nil {
		assert.Contains(t, err.Error(), "context", "Timeout error should mention context")
	} else {
		assert.NotNil(t, results, "If successful, results should not be nil")
	}
}

// TestEngineRunWithCancellation tests context cancellation handling
func TestEngineRunWithCancellation(t *testing.T) {
	ctx := context.Background()

	// Create Docker client for the test
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		t.Skip("Skipping test - Docker not available:", err)
		return
	}
	defer dockerClient.Close()

	// Create engine
	config := &diagnostics.Config{
		Timeout:  30 * time.Second,
		Verbose:  false,
		Parallel: false,
	}
	engine := diagnostics.NewEngine(dockerClient, config)

	// Create a cancellable context and cancel it immediately
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel() // Cancel immediately

	// Run the engine with cancelled context
	results, err := engine.Run(cancelCtx)

	// Should handle cancellation gracefully
	if err != nil {
		assert.Contains(t, err.Error(), "context", "Cancellation error should mention context")
	} else {
		// Some checks might complete before cancellation is detected
		assert.NotNil(t, results, "If successful, results should not be nil")
	}
}

// TestEngineRunParallelDirect tests the runParallel method directly via parallel config
func TestEngineRunParallelDirect(t *testing.T) {
	ctx := context.Background()

	// Create Docker client for the test
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		t.Skip("Skipping test - Docker not available:", err)
		return
	}
	defer dockerClient.Close()

	// Create engine with parallel execution but without worker pool to trigger runParallel
	config := &diagnostics.Config{
		Timeout:     30 * time.Second,
		Verbose:     false,
		Parallel:    true,
		WorkerCount: 0,   // Set to 0 to avoid worker pool path
		RateLimit:   0.0, // No rate limiting to avoid worker pool
	}
	engine := diagnostics.NewEngine(dockerClient, config)

	// Run the engine - this should use the runParallel code path
	results, err := engine.Run(ctx)
	require.NoError(t, err, "Engine.Run in parallel mode should not return an error")
	require.NotNil(t, results, "Results should not be nil")

	// Verify that we got results from parallel execution
	assert.IsType(t, &diagnostics.Results{}, results, "Results should be of correct type")
}
