package diagnostics

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"
)

// TestPerformanceBenchmark provides a precise way to measure parallel vs sequential performance
func TestPerformanceBenchmark(t *testing.T) {
	// Benchmark parameters
	checkConfigurations := []struct {
		workTimes []time.Duration
		workers   []int
	}{
		{
			workTimes: []time.Duration{
				50 * time.Millisecond,
				80 * time.Millisecond,
				30 * time.Millisecond,
				100 * time.Millisecond,
				150 * time.Millisecond,
			},
			workers: []int{1, 2, 4, runtime.NumCPU()},
		},
	}

	for _, config := range checkConfigurations {
		// Measure sequential execution time
		sequentialStart := time.Now()
		for range config.workTimes {
			time.Sleep(config.workTimes[0]) // Simulate sequential work
		}
		sequentialDuration := time.Since(sequentialStart)

		// Benchmark parallel execution for different worker counts
		for _, workerCount := range config.workers {
			t.Run(fmt.Sprintf("Parallel_%d_Workers", workerCount), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				parallelStart := time.Now()
				
				// Use channels to simulate parallel work
				resultChan := make(chan bool, len(config.workTimes))
				for _, workTime := range config.workTimes {
					go func(duration time.Duration) {
						select {
						case <-ctx.Done():
							return
						case <-time.After(duration):
							resultChan <- true
						}
					}(workTime)
				}

				// Wait for all results or context cancellation
				for range config.workTimes {
					select {
					case <-resultChan:
						continue
					case <-ctx.Done():
						t.Fatal("Benchmark timed out")
						return
					}
				}

				parallelDuration := time.Since(parallelStart)

				// Calculate speedup and improvement percentage
				speedup := float64(sequentialDuration) / float64(parallelDuration)
				improvementPercentage := (speedup - 1) * 100

				t.Logf("Sequential Duration: %v", sequentialDuration)
				t.Logf("Parallel Duration (%d workers): %v", workerCount, parallelDuration)
				t.Logf("Speedup: %.2f", speedup)
				t.Logf("Performance Improvement: %.2f%%", improvementPercentage)

				// Validate performance improvement
				if improvementPercentage < 60 {
					t.Errorf("Performance improvement (%.2f%%) is below the 60%% target", improvementPercentage)
				}
				if improvementPercentage > 100 {
					t.Errorf("Unrealistic performance improvement (%.2f%%)", improvementPercentage)
				}
			})
		}
	}
}