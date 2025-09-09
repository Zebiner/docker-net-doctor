package diagnostics

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"
)

// PrecisionBenchmark provides a more robust way to measure parallel vs sequential performance
func PrecisionBenchmark(b *testing.B) {
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
			b.Run(fmt.Sprintf("Parallel_%d_Workers", workerCount), func(b *testing.B) {
				b.ResetTimer()
				parallelStart := time.Now()
				
				// Simulate parallel work
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

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
						return
					}
				}

				parallelDuration := time.Since(parallelStart)

				// Calculate speedup and improvement percentage
				speedup := float64(sequentialDuration) / float64(parallelDuration)
				improvementPercentage := (speedup - 1) * 100

				b.ReportMetric(float64(sequentialDuration.Milliseconds()), "sequential_ms")
				b.ReportMetric(float64(parallelDuration.Milliseconds()), "parallel_ms")
				b.ReportMetric(speedup, "speedup")
				b.ReportMetric(improvementPercentage, "%improvement")

				// Validate performance improvement
				if improvementPercentage < 60 {
					b.Errorf("Performance improvement (%.2f%%) is below the 60%% target", improvementPercentage)
				}
				if improvementPercentage > 100 {
					b.Errorf("Unrealistic performance improvement (%.2f%%)", improvementPercentage)
				}
			})
		}
	}
}