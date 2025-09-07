package diagnostics

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/docker"
	"golang.org/x/time/rate"
)

// BenchmarkCheck simulates a real diagnostic check with variable latency
type BenchmarkCheck struct {
	name     string
	workTime time.Duration
}

func (b *BenchmarkCheck) Name() string        { return b.name }
func (b *BenchmarkCheck) Description() string { return "Benchmark check: " + b.name }
func (b *BenchmarkCheck) Severity() Severity  { return SeverityInfo }

func (b *BenchmarkCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	// Simulate work
	start := time.Now()
	for time.Since(start) < b.workTime {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			// Busy work to simulate CPU usage
			_ = start.Unix()
		}
	}
	
	return &CheckResult{
		CheckName: b.name,
		Success:   true,
		Message:   "Check completed",
		Timestamp: time.Now(),
	}, nil
}

// createBenchmarkChecks creates a set of checks similar to real diagnostic checks
func createBenchmarkChecks() []Check {
	return []Check{
		&BenchmarkCheck{name: "daemon_connectivity", workTime: 50 * time.Millisecond},
		&BenchmarkCheck{name: "bridge_network", workTime: 80 * time.Millisecond},
		&BenchmarkCheck{name: "ip_forwarding", workTime: 30 * time.Millisecond},
		&BenchmarkCheck{name: "iptables", workTime: 100 * time.Millisecond},
		&BenchmarkCheck{name: "dns_resolution", workTime: 150 * time.Millisecond},
		&BenchmarkCheck{name: "internal_dns", workTime: 120 * time.Millisecond},
		&BenchmarkCheck{name: "container_connectivity", workTime: 90 * time.Millisecond},
		&BenchmarkCheck{name: "port_binding", workTime: 60 * time.Millisecond},
		&BenchmarkCheck{name: "network_isolation", workTime: 70 * time.Millisecond},
		&BenchmarkCheck{name: "mtu_consistency", workTime: 40 * time.Millisecond},
		&BenchmarkCheck{name: "subnet_overlap", workTime: 55 * time.Millisecond},
	}
}

// BenchmarkSequentialExecution benchmarks sequential check execution
func BenchmarkSequentialExecution(b *testing.B) {
	ctx := context.Background()
	checks := createBenchmarkChecks()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine := &DiagnosticEngine{
			checks:  checks,
			results: &Results{Checks: make([]*CheckResult, 0)},
			config: &Config{
				Parallel: false,
				Timeout:  30 * time.Second,
			},
			rateLimiter: NewRateLimiter(nil), // Add rate limiter
		}
		
		start := time.Now()
		engine.runSequential(ctx)
		duration := time.Since(start)
		
		b.ReportMetric(float64(duration.Milliseconds()), "ms/op")
		b.ReportMetric(float64(len(checks)), "checks")
	}
}

// BenchmarkParallelExecution benchmarks parallel check execution with worker pool
func BenchmarkParallelExecution(b *testing.B) {
	benchmarks := []struct {
		name    string
		workers int
	}{
		{"Workers-1", 1},
		{"Workers-2", 2},
		{"Workers-4", 4},
		{"Workers-8", 8},
		{"Workers-CPU", runtime.NumCPU()},
	}
	
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			ctx := context.Background()
			checks := createBenchmarkChecks()
			
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pool, err := NewSecureWorkerPool(ctx, bm.workers)
				if err != nil {
					b.Fatal(err)
				}
				
				err = pool.Start()
				if err != nil {
					b.Fatal(err)
				}
				
				start := time.Now()
				
				// Submit all checks
				for _, check := range checks {
					if err := pool.Submit(check, nil); err != nil {
						b.Fatal(err)
					}
				}
				
				// Collect results
				results := make([]JobResult, 0)
				resultsChan := pool.GetResults()
				for len(results) < len(checks) {
					select {
					case result := <-resultsChan:
						results = append(results, result)
					case <-time.After(5 * time.Second):
						b.Fatal("Timeout collecting results")
					}
				}
				
				duration := time.Since(start)
				pool.Stop()
				
				b.ReportMetric(float64(duration.Milliseconds()), "ms/op")
				b.ReportMetric(float64(len(checks)), "checks")
				b.ReportMetric(float64(bm.workers), "workers")
				
				// Calculate improvement over sequential
				sequentialTime := 845 * time.Millisecond // Sum of all work times
				improvement := (1 - float64(duration)/float64(sequentialTime)) * 100
				b.ReportMetric(improvement, "%improvement")
			}
		})
	}
}

// BenchmarkWorkerPoolOverhead benchmarks the overhead of the worker pool
func BenchmarkWorkerPoolOverhead(b *testing.B) {
	ctx := context.Background()
	
	// Create very quick checks to measure overhead
	checks := make([]Check, 100)
	for i := range checks {
		checks[i] = &BenchmarkCheck{
			name:     fmt.Sprintf("quick_%d", i),
			workTime: 100 * time.Microsecond, // Very quick
		}
	}
	
	benchmarks := []struct {
		name    string
		workers int
	}{
		{"Workers-2", 2},
		{"Workers-4", 4},
		{"Workers-8", 8},
	}
	
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pool, _ := NewSecureWorkerPool(ctx, bm.workers)
				pool.Start()
				
				// Submit all checks
				for _, check := range checks {
					pool.Submit(check, nil)
				}
				
				// Collect results
				results := make([]JobResult, 0)
				resultsChan := pool.GetResults()
				for len(results) < len(checks) {
					select {
					case result := <-resultsChan:
						results = append(results, result)
					case <-time.After(5 * time.Second):
						b.Fatal("Timeout")
					}
				}
				
				pool.Stop()
			}
			
			b.ReportMetric(float64(len(checks)), "checks")
			b.ReportMetric(float64(bm.workers), "workers")
		})
	}
}

// BenchmarkRateLimiting benchmarks the impact of rate limiting
func BenchmarkRateLimiting(b *testing.B) {
	benchmarks := []struct {
		name      string
		rateLimit float64
	}{
		{"NoLimit", 0},
		{"5PerSec", 5},
		{"10PerSec", 10},
		{"20PerSec", 20},
	}
	
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			ctx := context.Background()
			checks := createBenchmarkChecks()
			
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pool, _ := NewSecureWorkerPool(ctx, 4)
				
				// Configure rate limiting
				if bm.rateLimit > 0 {
					pool.rateLimiter = rate.NewLimiter(rate.Limit(bm.rateLimit), int(bm.rateLimit))
				}
				
				pool.Start()
				
				start := time.Now()
				
				// Submit all checks
				for _, check := range checks {
					pool.Submit(check, nil)
				}
				
				// Collect results
				results := make([]JobResult, 0)
				resultsChan := pool.GetResults()
				for len(results) < len(checks) {
					select {
					case result := <-resultsChan:
						results = append(results, result)
					case <-time.After(10 * time.Second):
						b.Fatal("Timeout")
					}
				}
				
				duration := time.Since(start)
				pool.Stop()
				
				b.ReportMetric(float64(duration.Milliseconds()), "ms/op")
				b.ReportMetric(bm.rateLimit, "rate_limit")
			}
		})
	}
}

// BenchmarkMemoryUsage benchmarks memory usage of the worker pool
func BenchmarkMemoryUsage(b *testing.B) {
	ctx := context.Background()
	checks := createBenchmarkChecks()
	
	benchmarks := []struct {
		name    string
		workers int
	}{
		{"Workers-2", 2},
		{"Workers-4", 4},
		{"Workers-8", 8},
	}
	
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var m1, m2 runtime.MemStats
				runtime.ReadMemStats(&m1)
				
				pool, _ := NewSecureWorkerPool(ctx, bm.workers)
				pool.Start()
				
				// Submit all checks
				for _, check := range checks {
					pool.Submit(check, nil)
				}
				
				// Collect results
				results := make([]JobResult, 0)
				resultsChan := pool.GetResults()
				for len(results) < len(checks) {
					select {
					case result := <-resultsChan:
						results = append(results, result)
					case <-time.After(5 * time.Second):
						b.Fatal("Timeout")
					}
				}
				
				runtime.ReadMemStats(&m2)
				pool.Stop()
				
				memUsed := m2.Alloc - m1.Alloc
				b.ReportMetric(float64(memUsed)/(1024*1024), "MB")
				b.ReportMetric(float64(bm.workers), "workers")
			}
		})
	}
}

// BenchmarkEngineIntegration benchmarks the full diagnostic engine with worker pool
func BenchmarkEngineIntegration(b *testing.B) {
	benchmarks := []struct {
		name     string
		parallel bool
		workers  int
	}{
		{"Sequential", false, 0},
		{"Parallel-2", true, 2},
		{"Parallel-4", true, 4},
		{"Parallel-8", true, 8},
	}
	
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			ctx := context.Background()
			
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Create engine with benchmark checks
				engine := &DiagnosticEngine{
					checks: createBenchmarkChecks(),
					results: &Results{
						Checks:  make([]*CheckResult, 0),
						Metrics: &ExecutionMetrics{},
					},
					config: &Config{
						Parallel:    bm.parallel,
						Timeout:     30 * time.Second,
						WorkerCount: bm.workers,
					},
					rateLimiter: NewRateLimiter(nil),
				}
				
				start := time.Now()
				results, err := engine.Run(ctx)
				duration := time.Since(start)
				
				if err != nil {
					b.Fatal(err)
				}
				
				b.ReportMetric(float64(duration.Milliseconds()), "ms/op")
				b.ReportMetric(float64(len(results.Checks)), "checks")
				
				if bm.parallel {
					b.ReportMetric(float64(bm.workers), "workers")
					// Calculate speedup
					sequentialTime := 845 * time.Millisecond
					speedup := float64(sequentialTime) / float64(duration)
					b.ReportMetric(speedup, "speedup")
				}
			}
		})
	}
}