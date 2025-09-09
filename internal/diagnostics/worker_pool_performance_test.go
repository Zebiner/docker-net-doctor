package diagnostics

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// Performance and Scalability Tests

func BenchmarkWorkerPoolPerformanceUnderHighConcurrency(b *testing.B) {
	ctx := context.Background()
	
	workerCounts := []int{1, 2, 4, 8, 16, 32}
	jobCounts := []int{100, 500, 1000, 5000}
	
	for _, workers := range workerCounts {
		for _, jobs := range jobCounts {
			if workers > runtime.NumCPU()*4 && jobs > 1000 {
				continue // Skip excessive combinations
			}
			
			benchName := fmt.Sprintf("Workers%d_Jobs%d", workers, jobs)
			b.Run(benchName, func(b *testing.B) {
				b.ResetTimer()
				
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					pool, err := NewSecureWorkerPool(ctx, workers)
					if err != nil {
						b.Fatal(err)
					}
					
					err = pool.Start()
					if err != nil {
						b.Fatal(err)
					}
					
					b.StartTimer()
					
					// Submit jobs
					for j := 0; j < jobs; j++ {
						check := &PerformanceBenchmarkCheck{
							name:     fmt.Sprintf("bench_%d", j),
							workTime: 1 * time.Millisecond, // Minimal work
						}
						
						err := pool.Submit(check, nil)
						if err != nil {
							b.Errorf("Failed to submit job %d: %v", j, err)
							continue
						}
					}
					
					// Collect results
					results := 0
					resultsChan := pool.GetResults()
					timeout := time.After(30 * time.Second)
					
					for results < jobs {
						select {
						case <-resultsChan:
							results++
						case <-timeout:
							b.Fatalf("Timeout collecting results: got %d/%d", results, jobs)
						}
					}
					
					b.StopTimer()
					pool.Stop()
				}
				
				// Report metrics
				b.ReportMetric(float64(jobs), "jobs")
				b.ReportMetric(float64(workers), "workers")
				b.ReportMetric(float64(jobs)/float64(workers), "jobs_per_worker")
			})
		}
	}
}

func TestJobThroughputAndLatencyMeasurements(t *testing.T) {
	ctx := context.Background()
	
	configs := []struct {
		name     string
		workers  int
		jobs     int
		workTime time.Duration
	}{
		{"LightLoad", 4, 100, 1 * time.Millisecond},
		{"MediumLoad", 8, 500, 5 * time.Millisecond},
		{"HeavyLoad", 16, 1000, 10 * time.Millisecond},
	}
	
	for _, config := range configs {
		t.Run(config.name, func(t *testing.T) {
			pool, err := NewSecureWorkerPool(ctx, config.workers)
			require.NoError(t, err)
			
			err = pool.Start()
			require.NoError(t, err)
			defer pool.Stop()
			
			// Measure submission time
			submitStart := time.Now()
			
			for i := 0; i < config.jobs; i++ {
				check := &PerformanceBenchmarkCheck{
					name:     fmt.Sprintf("throughput_%d", i),
					workTime: config.workTime,
				}
				
				err := pool.Submit(check, nil)
				require.NoError(t, err)
			}
			
			submitDuration := time.Since(submitStart)
			
			// Measure completion time
			completionStart := time.Now()
			results := 0
			resultsChan := pool.GetResults()
			timeout := time.After(30 * time.Second)
			
			var totalLatency time.Duration
			var minLatency = time.Hour
			var maxLatency time.Duration
			
			for results < config.jobs {
				select {
				case result := <-resultsChan:
					results++
					
					// Calculate latency (duration from submission to completion)
					latency := result.Duration
					totalLatency += latency
					
					if latency < minLatency {
						minLatency = latency
					}
					if latency > maxLatency {
						maxLatency = latency
					}
					
				case <-timeout:
					t.Fatalf("Timeout collecting throughput results: got %d/%d", results, config.jobs)
				}
			}
			
			completionDuration := time.Since(completionStart)
			
			// Calculate metrics
			submitThroughput := float64(config.jobs) / submitDuration.Seconds()
			completionThroughput := float64(config.jobs) / completionDuration.Seconds()
			avgLatency := totalLatency / time.Duration(config.jobs)
			
			t.Logf("%s Performance Metrics:", config.name)
			t.Logf("  Workers: %d", config.workers)
			t.Logf("  Jobs: %d", config.jobs)
			t.Logf("  Submit Throughput: %.2f jobs/sec", submitThroughput)
			t.Logf("  Completion Throughput: %.2f jobs/sec", completionThroughput)
			t.Logf("  Average Latency: %v", avgLatency)
			t.Logf("  Min Latency: %v", minLatency)
			t.Logf("  Max Latency: %v", maxLatency)
			t.Logf("  Submit Duration: %v", submitDuration)
			t.Logf("  Completion Duration: %v", completionDuration)
			
			// Assertions for reasonable performance
			assert.Greater(t, submitThroughput, 100.0, "Submit throughput should be > 100 jobs/sec")
			assert.Greater(t, completionThroughput, 50.0, "Completion throughput should be > 50 jobs/sec")
			assert.Less(t, avgLatency, 100*time.Millisecond, "Average latency should be < 100ms")
		})
	}
}

func TestWorkerPoolScalingEfficiency(t *testing.T) {
	ctx := context.Background()
	jobCount := 1000
	workTime := 2 * time.Millisecond
	
	scalingTests := []struct {
		workers int
		name    string
	}{
		{1, "Single"},
		{2, "Dual"},
		{4, "Quad"},
		{8, "Octa"},
		{16, "Sixteen"},
	}
	
	var baselineTime time.Duration
	results := make(map[int]time.Duration)
	
	for _, test := range scalingTests {
		if test.workers > runtime.NumCPU()*2 {
			continue // Skip if too many workers for the system
		}
		
		t.Run(test.name, func(t *testing.T) {
			pool, err := NewSecureWorkerPool(ctx, test.workers)
			require.NoError(t, err)
			
			err = pool.Start()
			require.NoError(t, err)
			defer pool.Stop()
			
			// Submit jobs
			start := time.Now()
			
			for i := 0; i < jobCount; i++ {
				check := &PerformanceBenchmarkCheck{
					name:     fmt.Sprintf("test_scale_%d_%d", test.workers, i),
					workTime: workTime,
				}
				
				err := pool.Submit(check, nil)
				require.NoError(t, err)
			}
			
			// Collect results
			collected := 0
			resultsChan := pool.GetResults()
			timeout := time.After(30 * time.Second)
			
			for collected < jobCount {
				select {
				case <-resultsChan:
					collected++
				case <-timeout:
					t.Fatalf("Timeout in scaling test: got %d/%d", collected, jobCount)
				}
			}
			
			duration := time.Since(start)
			results[test.workers] = duration
			
			if test.workers == 1 {
				baselineTime = duration
			}
			
			t.Logf("Workers: %d, Duration: %v, Throughput: %.2f jobs/sec",
				test.workers, duration, float64(jobCount)/duration.Seconds())
		})
	}
	
	// Analyze scaling efficiency
	t.Run("ScalingAnalysis", func(t *testing.T) {
		if baselineTime == 0 {
			t.Skip("No baseline time available")
		}
		
		t.Logf("Scaling Efficiency Analysis:")
		for workers, duration := range results {
			if workers == 1 {
				continue
			}
			
			idealTime := baselineTime / time.Duration(workers)
			efficiency := float64(idealTime) / float64(duration)
			speedup := float64(baselineTime) / float64(duration)
			
			t.Logf("  %d workers: Duration=%v, Speedup=%.2fx, Efficiency=%.1f%%",
				workers, duration, speedup, efficiency*100)
			
			// Reasonable efficiency should be > 50% for up to 4 workers
			if workers <= 4 {
				assert.Greater(t, efficiency, 0.5, "Scaling efficiency should be > 50%% for %d workers", workers)
			}
		}
	})
}

func TestResourceUsageOptimization(t *testing.T) {
	ctx := context.Background()
	
	tests := []struct {
		name        string
		workers     int
		jobs        int
		workType    string
		maxMemoryMB float64
	}{
		{"LowResource", 2, 100, "cpu", 10},
		{"MediumResource", 4, 500, "mixed", 25},
		{"HighResource", 8, 1000, "memory", 50},
	}
	
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pool, err := NewSecureWorkerPool(ctx, test.workers)
			require.NoError(t, err)
			
			initialMemory := getCurrentMemory()
			
			err = pool.Start()
			require.NoError(t, err)
			defer pool.Stop()
			
			// Submit jobs based on work type
			for i := 0; i < test.jobs; i++ {
				var check Check
				
				switch test.workType {
				case "cpu":
					check = &CPUIntensiveCheck{
						name:       fmt.Sprintf("test_cpu_%d", i),
						iterations: 10000,
					}
				case "memory":
					check = &MemoryIntensiveCheck{
						name:      fmt.Sprintf("test_mem_%d", i),
						allocSize: 1024 * 100, // 100KB
						duration:  10 * time.Millisecond,
					}
				case "mixed":
					if i%2 == 0 {
						check = &CPUIntensiveCheck{
							name:       fmt.Sprintf("test_mixed_cpu_%d", i),
							iterations: 5000,
						}
					} else {
						check = &MemoryIntensiveCheck{
							name:      fmt.Sprintf("test_mixed_mem_%d", i),
							allocSize: 1024 * 50, // 50KB
							duration:  5 * time.Millisecond,
						}
					}
				}
				
				err := pool.Submit(check, nil)
				require.NoError(t, err)
			}
			
			// Monitor resource usage during execution
			var peakMemory uint64
			var memoryReadings []uint64
			done := make(chan bool)
			
			go func() {
				defer close(done)
				ticker := time.NewTicker(50 * time.Millisecond)
				defer ticker.Stop()
				
				for {
					select {
					case <-ticker.C:
						current := getCurrentMemory()
						memoryReadings = append(memoryReadings, current)
						if current > peakMemory {
							peakMemory = current
						}
					case <-done:
						return
					}
				}
			}()
			
			// Collect results
			collected := 0
			resultsChan := pool.GetResults()
			timeout := time.After(60 * time.Second)
			
			start := time.Now()
			for collected < test.jobs {
				select {
				case <-resultsChan:
					collected++
				case <-timeout:
					t.Fatalf("Timeout in resource test: got %d/%d", collected, test.jobs)
				}
			}
			duration := time.Since(start)
			
			close(done)
			<-done // Wait for monitoring to stop
			
			// Calculate resource metrics
			memoryUsedMB := float64(peakMemory-initialMemory) / (1024 * 1024)
			throughput := float64(test.jobs) / duration.Seconds()
			
			t.Logf("Resource Usage for %s:", test.name)
			t.Logf("  Peak Memory Used: %.2f MB", memoryUsedMB)
			t.Logf("  Throughput: %.2f jobs/sec", throughput)
			t.Logf("  Duration: %v", duration)
			t.Logf("  Memory Efficiency: %.2f jobs/MB", float64(test.jobs)/memoryUsedMB)
			
			// Verify resource usage is within expected limits
			assert.LessOrEqual(t, memoryUsedMB, test.maxMemoryMB,
				"Memory usage should be within limits for %s", test.name)
			
			// Verify reasonable throughput
			minThroughput := float64(test.workers) * 5.0 // At least 5 jobs/sec per worker
			assert.GreaterOrEqual(t, throughput, minThroughput,
				"Throughput should be at least %.2f jobs/sec", minThroughput)
		})
	}
}

func BenchmarkWorkerPoolMemoryEfficiency(b *testing.B) {
	ctx := context.Background()
	workers := 4
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		
		initialMemory := getCurrentMemory()
		
		pool, err := NewSecureWorkerPool(ctx, workers)
		if err != nil {
			b.Fatal(err)
		}
		
		err = pool.Start()
		if err != nil {
			b.Fatal(err)
		}
		
		b.StartTimer()
		
		// Submit memory-conscious jobs
		jobs := 100
		for j := 0; j < jobs; j++ {
			check := &MemoryEfficientCheck{
				name: fmt.Sprintf("test_mem_eff_%d", j),
				data: make([]byte, 1024), // 1KB per job
			}
			
			err := pool.Submit(check, nil)
			if err != nil {
				b.Error(err)
			}
		}
		
		// Collect results
		collected := 0
		resultsChan := pool.GetResults()
		timeout := time.After(10 * time.Second)
		
		for collected < jobs {
			select {
			case <-resultsChan:
				collected++
			case <-timeout:
				b.Fatal("Benchmark timeout")
			}
		}
		
		b.StopTimer()
		
		pool.Stop()
		runtime.GC()
		
		finalMemory := getCurrentMemory()
		memoryUsed := finalMemory - initialMemory
		
		b.ReportMetric(float64(memoryUsed)/(1024*1024), "memory_mb")
		b.ReportMetric(float64(jobs), "jobs")
		b.ReportMetric(float64(memoryUsed)/float64(jobs), "bytes_per_job")
	}
}

func TestConcurrentWorkerPoolScaling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent scaling test in short mode")
	}
	
	ctx := context.Background()
	
	// Test multiple worker pools running concurrently
	poolCount := 3
	jobsPerPool := 200
	workersPerPool := 4
	
	var wg sync.WaitGroup
	var totalJobs int64
	var completedJobs int64
	var errors []error
	var errorsMu sync.Mutex
	
	start := time.Now()
	
	for p := 0; p < poolCount; p++ {
		wg.Add(1)
		
		go func(poolID int) {
			defer wg.Done()
			
			pool, err := NewSecureWorkerPool(ctx, workersPerPool)
			if err != nil {
				errorsMu.Lock()
				errors = append(errors, err)
				errorsMu.Unlock()
				return
			}
			
			err = pool.Start()
			if err != nil {
				errorsMu.Lock()
				errors = append(errors, err)
				errorsMu.Unlock()
				return
			}
			defer pool.Stop()
			
			// Submit jobs for this pool
			for j := 0; j < jobsPerPool; j++ {
				atomic.AddInt64(&totalJobs, 1)
				
				check := &ConcurrentCheck{
					name:   fmt.Sprintf("test_pool%d_job%d", poolID, j),
					poolID: poolID,
					delay:  time.Duration(j%10+1) * time.Millisecond,
				}
				
				err := pool.Submit(check, nil)
				if err != nil {
					errorsMu.Lock()
					errors = append(errors, err)
					errorsMu.Unlock()
					continue
				}
			}
			
			// Collect results for this pool
			collected := 0
			resultsChan := pool.GetResults()
			timeout := time.After(30 * time.Second)
			
			for collected < jobsPerPool {
				select {
				case result := <-resultsChan:
					if result.Error == nil {
						atomic.AddInt64(&completedJobs, 1)
					}
					collected++
				case <-timeout:
					errorsMu.Lock()
					errors = append(errors, fmt.Errorf("pool %d timeout: collected %d/%d", poolID, collected, jobsPerPool))
					errorsMu.Unlock()
					return
				}
			}
		}(p)
	}
	
	wg.Wait()
	duration := time.Since(start)
	
	// Verify results
	require.Empty(t, errors, "No errors should occur during concurrent scaling")
	
	expectedJobs := int64(poolCount * jobsPerPool)
	assert.Equal(t, expectedJobs, totalJobs, "All jobs should be submitted")
	assert.Equal(t, expectedJobs, completedJobs, "All jobs should complete successfully")
	
	throughput := float64(completedJobs) / duration.Seconds()
	t.Logf("Concurrent Scaling Results:")
	t.Logf("  Pools: %d", poolCount)
	t.Logf("  Workers per pool: %d", workersPerPool)
	t.Logf("  Jobs per pool: %d", jobsPerPool)
	t.Logf("  Total jobs: %d", totalJobs)
	t.Logf("  Completed jobs: %d", completedJobs)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.2f jobs/sec", throughput)
	
	// Performance expectations
	minThroughput := float64(poolCount * workersPerPool) * 10.0 // 10 jobs/sec per worker minimum
	assert.GreaterOrEqual(t, throughput, minThroughput, "Concurrent throughput should meet minimum")
}

// Helper types for performance testing

type PerformanceBenchmarkCheck struct {
	name     string
	workTime time.Duration
}

func (b *PerformanceBenchmarkCheck) Name() string        { return b.name }
func (b *PerformanceBenchmarkCheck) Description() string { return "Performance benchmark test check" }
func (b *PerformanceBenchmarkCheck) Severity() Severity  { return SeverityInfo }

func (b *PerformanceBenchmarkCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	time.Sleep(b.workTime)
	return &CheckResult{
		CheckName: b.name,
		Success:   true,
		Message:   "Performance benchmark work completed",
		Timestamp: time.Now(),
	}, nil
}

type CPUIntensiveCheck struct {
	name       string
	iterations int
}

func (c *CPUIntensiveCheck) Name() string        { return c.name }
func (c *CPUIntensiveCheck) Description() string { return "CPU intensive check" }
func (c *CPUIntensiveCheck) Severity() Severity  { return SeverityInfo }

func (c *CPUIntensiveCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	result := 0
	for i := 0; i < c.iterations; i++ {
		result += i * i
		if i%1000 == 0 {
			runtime.Gosched() // Allow other goroutines
		}
	}
	
	return &CheckResult{
		CheckName: c.name,
		Success:   true,
		Message:   fmt.Sprintf("CPU work completed with result %d", result),
		Timestamp: time.Now(),
	}, nil
}

type MemoryEfficientCheck struct {
	name string
	data []byte
}

func (m *MemoryEfficientCheck) Name() string        { return m.name }
func (m *MemoryEfficientCheck) Description() string { return "Memory efficient check" }
func (m *MemoryEfficientCheck) Severity() Severity  { return SeverityInfo }

func (m *MemoryEfficientCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	// Process the data briefly
	checksum := 0
	for _, b := range m.data {
		checksum += int(b)
	}
	
	// Clear reference to help GC
	m.data = nil
	
	return &CheckResult{
		CheckName: m.name,
		Success:   true,
		Message:   fmt.Sprintf("Processed data with checksum %d", checksum),
		Timestamp: time.Now(),
	}, nil
}

type ConcurrentCheck struct {
	name   string
	poolID int
	delay  time.Duration
}

func (c *ConcurrentCheck) Name() string        { return c.name }
func (c *ConcurrentCheck) Description() string { return "Concurrent test check" }
func (c *ConcurrentCheck) Severity() Severity  { return SeverityInfo }

func (c *ConcurrentCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	time.Sleep(c.delay)
	return &CheckResult{
		CheckName: c.name,
		Success:   true,
		Message:   fmt.Sprintf("Pool %d job completed", c.poolID),
		Timestamp: time.Now(),
	}, nil
}