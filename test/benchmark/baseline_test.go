// test/benchmark/baseline_test.go
package benchmark

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/diagnostics"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// BenchmarkResult holds performance metrics for analysis
type BenchmarkResult struct {
	Name            string
	Duration        time.Duration
	MemoryAllocated uint64
	NumAllocations  uint64
	NumGoroutines   int
}

// Global variables to prevent compiler optimizations
var (
	globalResults *diagnostics.Results
	globalErr     error
)

// BenchmarkSequentialExecution measures the baseline performance of running all checks sequentially
func BenchmarkSequentialExecution(b *testing.B) {
	ctx := context.Background()
	
	// Setup Docker client (use mock for consistent benchmarking)
	dockerClient := createMockDockerClient(b)
	defer dockerClient.Close()
	
	// Create engine with sequential execution
	config := &diagnostics.Config{
		Parallel: false,
		Timeout:  30 * time.Second,
		Verbose:  false,
	}
	
	// Reset timer to exclude setup time
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		engine := diagnostics.NewEngine(dockerClient, config)
		results, err := engine.Run(ctx)
		
		// Store results to prevent compiler optimization
		globalResults = results
		globalErr = err
	}
	
	// Report custom metrics
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/1e6, "ms/op")
}

// BenchmarkParallelExecution measures the performance of running checks in parallel
func BenchmarkParallelExecution(b *testing.B) {
	ctx := context.Background()
	
	// Setup Docker client
	dockerClient := createMockDockerClient(b)
	defer dockerClient.Close()
	
	// Create engine with parallel execution
	config := &diagnostics.Config{
		Parallel: true,
		Timeout:  30 * time.Second,
		Verbose:  false,
	}
	
	// Reset timer to exclude setup time
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		engine := diagnostics.NewEngine(dockerClient, config)
		results, err := engine.Run(ctx)
		
		// Store results to prevent compiler optimization
		globalResults = results
		globalErr = err
	}
	
	// Report custom metrics
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/1e6, "ms/op")
	
	// Calculate speedup vs sequential
	// This will be used to measure the 60% improvement target
}

// BenchmarkEngineCreation measures the overhead of creating a diagnostic engine
func BenchmarkEngineCreation(b *testing.B) {
	dockerClient := createMockDockerClient(b)
	defer dockerClient.Close()
	
	config := &diagnostics.Config{
		Parallel: true,
		Timeout:  30 * time.Second,
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		engine := diagnostics.NewEngine(dockerClient, config)
		_ = engine // Use the engine to prevent optimization
	}
}

// BenchmarkMemoryUsage tracks memory consumption during diagnostic runs
func BenchmarkMemoryUsage(b *testing.B) {
	ctx := context.Background()
	
	dockerClient := createMockDockerClient(b)
	defer dockerClient.Close()
	
	config := &diagnostics.Config{
		Parallel: false,
		Timeout:  30 * time.Second,
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	var memStats runtime.MemStats
	
	for i := 0; i < b.N; i++ {
		// Measure memory before
		runtime.ReadMemStats(&memStats)
		beforeAlloc := memStats.Alloc
		
		engine := diagnostics.NewEngine(dockerClient, config)
		results, _ := engine.Run(ctx)
		
		// Measure memory after
		runtime.ReadMemStats(&memStats)
		afterAlloc := memStats.Alloc
		
		// Report memory usage
		memUsed := afterAlloc - beforeAlloc
		b.ReportMetric(float64(memUsed)/1024, "KB/op")
		
		globalResults = results
	}
}

// BenchmarkConcurrentEngines measures performance with multiple engines running simultaneously
func BenchmarkConcurrentEngines(b *testing.B) {
	ctx := context.Background()
	
	// Test with different levels of concurrency
	concurrencyLevels := []int{1, 2, 4, 8, 16}
	
	for _, numEngines := range concurrencyLevels {
		b.Run(fmt.Sprintf("Engines-%d", numEngines), func(b *testing.B) {
			dockerClient := createMockDockerClient(b)
			defer dockerClient.Close()
			
			config := &diagnostics.Config{
				Parallel: true,
				Timeout:  30 * time.Second,
			}
			
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				// Run multiple engines concurrently
				done := make(chan bool, numEngines)
				
				for j := 0; j < numEngines; j++ {
					go func() {
						engine := diagnostics.NewEngine(dockerClient, config)
						engine.Run(ctx)
						done <- true
					}()
				}
				
				// Wait for all engines to complete
				for j := 0; j < numEngines; j++ {
					<-done
				}
			}
			
			b.ReportMetric(float64(numEngines), "engines")
		})
	}
}

// BenchmarkWorstCase simulates worst-case scenario with slow Docker responses
func BenchmarkWorstCase(b *testing.B) {
	ctx := context.Background()
	
	// Create a mock client with artificial delays
	dockerClient := createSlowMockDockerClient(b, 100*time.Millisecond)
	defer dockerClient.Close()
	
	config := &diagnostics.Config{
		Parallel: false,
		Timeout:  60 * time.Second,
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		engine := diagnostics.NewEngine(dockerClient, config)
		results, _ := engine.Run(ctx)
		globalResults = results
	}
	
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/1e9, "s/op")
}

// BenchmarkBestCase simulates best-case scenario with instant Docker responses
func BenchmarkBestCase(b *testing.B) {
	ctx := context.Background()
	
	// Create a mock client with no delays
	dockerClient := createFastMockDockerClient(b)
	defer dockerClient.Close()
	
	config := &diagnostics.Config{
		Parallel: true,
		Timeout:  30 * time.Second,
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		engine := diagnostics.NewEngine(dockerClient, config)
		results, _ := engine.Run(ctx)
		globalResults = results
	}
	
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/1e6, "ms/op")
}

// Helper function to create a mock Docker client for benchmarking
func createMockDockerClient(b *testing.B) *docker.Client {
	b.Helper()
	
	ctx := context.Background()
	client, err := docker.NewClient(ctx)
	if err != nil {
		b.Skip("Docker not available for benchmarking:", err)
	}
	
	return client
}

// Helper function to create a slow mock Docker client
func createSlowMockDockerClient(b *testing.B, delay time.Duration) *docker.Client {
	b.Helper()
	
	// In production, this would return a mock client with artificial delays
	// For now, return the regular client
	return createMockDockerClient(b)
}

// Helper function to create a fast mock Docker client
func createFastMockDockerClient(b *testing.B) *docker.Client {
	b.Helper()
	
	// In production, this would return a mock client with instant responses
	// For now, return the regular client
	return createMockDockerClient(b)
}

// Benchmark comparison function to measure improvement
func BenchmarkImprovementTarget(b *testing.B) {
	ctx := context.Background()
	
	dockerClient := createMockDockerClient(b)
	defer dockerClient.Close()
	
	// Measure sequential baseline
	sequentialConfig := &diagnostics.Config{
		Parallel: false,
		Timeout:  30 * time.Second,
	}
	
	start := time.Now()
	seqEngine := diagnostics.NewEngine(dockerClient, sequentialConfig)
	seqEngine.Run(ctx)
	sequentialTime := time.Since(start)
	
	// Measure parallel performance
	parallelConfig := &diagnostics.Config{
		Parallel: true,
		Timeout:  30 * time.Second,
	}
	
	start = time.Now()
	parEngine := diagnostics.NewEngine(dockerClient, parallelConfig)
	parEngine.Run(ctx)
	parallelTime := time.Since(start)
	
	// Calculate improvement
	improvement := float64(sequentialTime-parallelTime) / float64(sequentialTime) * 100
	
	b.Logf("Sequential: %v", sequentialTime)
	b.Logf("Parallel: %v", parallelTime)
	b.Logf("Improvement: %.2f%%", improvement)
	b.Logf("Target: 60%% improvement")
	
	// Report if we meet the target
	if improvement >= 60 {
		b.Logf("✓ Target achieved!")
	} else {
		b.Logf("✗ Need %.2f%% more improvement", 60-improvement)
	}
}