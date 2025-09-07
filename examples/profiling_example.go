// Package main demonstrates how to use the performance profiling infrastructure
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/diagnostics"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

func main() {
	// Create profiler configuration for 1ms accuracy
	config := &diagnostics.ProfileConfig{
		Enabled:           true,
		Precision:         1 * time.Millisecond,  // Target 1ms accuracy
		MaxOverhead:       0.05,                  // Max 5% overhead
		SamplingRate:      100 * time.Microsecond,
		MaxDataPoints:     10000,
		EnableFlameGraph:  false,
		EnablePercentiles: true,
		EnableTrends:      true,
		RealtimeMetrics:   true,
		DetailLevel:       diagnostics.DetailLevelNormal,
	}

	// Create performance profiler
	profiler := diagnostics.NewPerformanceProfiler(config)

	// Start profiling
	fmt.Println("Starting performance profiler with 1ms accuracy target...")
	if err := profiler.Start(); err != nil {
		log.Fatalf("Failed to start profiler: %v", err)
	}
	defer func() {
		if err := profiler.Stop(); err != nil {
			log.Printf("Failed to stop profiler: %v", err)
		}
	}()

	// Example 1: Profile individual operations
	fmt.Println("\n=== Profiling Individual Operations ===")
	for i := 0; i < 5; i++ {
		err := profiler.ProfileOperation(
			fmt.Sprintf("operation_%d", i),
			"example_operations",
			func() error {
				// Simulate varying work
				time.Sleep(time.Duration(i+1) * 5 * time.Millisecond)
				return nil
			},
		)
		if err != nil {
			log.Printf("Operation %d failed: %v", i, err)
		}
	}

	// Example 2: Profile Docker operations
	fmt.Println("\n=== Profiling Docker API Calls ===")
	
	// Create Docker client
	ctx := context.Background()
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		log.Printf("Failed to create Docker client: %v", err)
	} else {
		defer dockerClient.Close()

		// Profile Docker API calls
		err = profiler.ProfileDockerAPICall("list_containers", func() error {
			_, err := dockerClient.ListContainers(ctx)
			return err
		})
		if err != nil {
			log.Printf("Docker API call failed: %v", err)
		}
	}

	// Example 3: Profile with worker pool integration
	fmt.Println("\n=== Profiling with Worker Pool ===")
	
	// Create worker pool
	pool, err := diagnostics.NewSecureWorkerPool(ctx, 4)
	if err != nil {
		log.Printf("Failed to create worker pool: %v", err)
	} else {
		// Set integration
		profiler.SetIntegration(pool, dockerClient, nil)

		// Start pool
		if err := pool.Start(); err != nil {
			log.Printf("Failed to start pool: %v", err)
		} else {
			defer pool.Stop()

			// Profile worker jobs
			for i := 0; i < 10; i++ {
				job := diagnostics.Job{
					ID: i,
					Check: &exampleCheck{
						name:     fmt.Sprintf("check_%d", i),
						duration: time.Duration(i+1) * 2 * time.Millisecond,
					},
					Context: ctx,
					Client:  dockerClient,
				}

				result, err := profiler.ProfileWorkerJob(i%4, job)
				if err != nil {
					log.Printf("Worker job %d failed: %v", i, err)
				} else if result != nil {
					fmt.Printf("  Job %d completed in %v\n", i, result.Duration)
				}
			}
		}
	}

	// Get metrics
	fmt.Println("\n=== Performance Metrics ===")
	metrics := profiler.GetMetrics()
	
	fmt.Printf("Total Operations: %d\n", metrics.TotalOperations)
	fmt.Printf("Total Duration: %v\n", metrics.TotalDuration)
	fmt.Printf("Average Duration: %v\n", metrics.AverageDuration)
	fmt.Printf("Min Duration: %v\n", metrics.MinDuration)
	fmt.Printf("Max Duration: %v\n", metrics.MaxDuration)
	
	if len(metrics.Percentiles) > 0 {
		fmt.Println("\nPercentiles:")
		fmt.Printf("  P50 (Median): %v\n", metrics.Percentiles[50])
		fmt.Printf("  P95: %v\n", metrics.Percentiles[95])
		fmt.Printf("  P99: %v\n", metrics.Percentiles[99])
	}
	
	fmt.Printf("\nAccuracy Achieved: %v (target: 1ms)\n", metrics.AccuracyAchieved)
	fmt.Printf("Profiling Overhead: %.2f%%\n", profiler.GetOverheadPercentage())
	
	if profiler.IsWithinOverheadLimit() {
		fmt.Println("✓ Overhead is within acceptable limits")
	} else {
		fmt.Println("✗ Overhead exceeds acceptable limits")
	}

	// Category breakdown
	if len(metrics.CategoryMetrics) > 0 {
		fmt.Println("\n=== Category Breakdown ===")
		for category, catMetrics := range metrics.CategoryMetrics {
			fmt.Printf("%s:\n", category)
			fmt.Printf("  Count: %d\n", catMetrics.Count)
			fmt.Printf("  Average: %v\n", catMetrics.AverageDuration)
			fmt.Printf("  Min: %v\n", catMetrics.MinDuration)
			fmt.Printf("  Max: %v\n", catMetrics.MaxDuration)
		}
	}

	// Worker metrics
	if len(metrics.WorkerMetrics) > 0 {
		fmt.Println("\n=== Worker Metrics ===")
		for workerID, workerMetrics := range metrics.WorkerMetrics {
			fmt.Printf("Worker %d:\n", workerID)
			fmt.Printf("  Jobs Processed: %d\n", workerMetrics.JobsProcessed)
			fmt.Printf("  Average Job Time: %v\n", workerMetrics.AverageJobTime)
			fmt.Printf("  Utilization: %.1f%%\n", workerMetrics.Utilization*100)
		}
	}

	// Generate full report
	fmt.Println("\n=== Generating Full Report ===")
	report := profiler.GenerateReport()
	
	// Print first 500 characters of report as example
	if len(report) > 500 {
		fmt.Println(report[:500] + "...")
		fmt.Printf("\n[Full report is %d characters]\n", len(report))
	} else {
		fmt.Println(report)
	}

	// Validate accuracy
	fmt.Println("\n=== Accuracy Validation ===")
	if metrics.AccuracyAchieved <= 1*time.Millisecond {
		fmt.Println("✓ Successfully achieved 1ms timing accuracy!")
	} else {
		fmt.Printf("✗ Accuracy target missed: %v (target: 1ms)\n", metrics.AccuracyAchieved)
	}
}

// exampleCheck is a mock check for demonstration
type exampleCheck struct {
	name     string
	duration time.Duration
}

func (e *exampleCheck) Name() string {
	return e.name
}

func (e *exampleCheck) Description() string {
	return fmt.Sprintf("Example check %s", e.name)
}

func (e *exampleCheck) Severity() diagnostics.Severity {
	return diagnostics.SeverityInfo
}

func (e *exampleCheck) Run(ctx context.Context, client *docker.Client) (*diagnostics.CheckResult, error) {
	// Simulate work
	time.Sleep(e.duration)
	
	return &diagnostics.CheckResult{
		CheckName: e.name,
		Success:   true,
		Message:   "Check completed successfully",
		Timestamp: time.Now(),
		Duration:  e.duration,
	}, nil
}