// test/benchmark/individual_checks_test.go
package benchmark

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/diagnostics"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// IndividualCheckMetrics holds performance data for a single check
type IndividualCheckMetrics struct {
	CheckName      string
	AverageTime    time.Duration
	MinTime        time.Duration
	MaxTime        time.Duration
	MemoryUsed     uint64
	DockerAPICalls int
}

// BenchmarkIndividualChecks benchmarks each diagnostic check separately
func BenchmarkIndividualChecks(b *testing.B) {
	// List of all diagnostic checks to benchmark
	checks := []diagnostics.Check{
		&diagnostics.DaemonConnectivityCheck{},
		&diagnostics.BridgeNetworkCheck{},
		&diagnostics.IPForwardingCheck{},
		&diagnostics.IptablesCheck{},
		&diagnostics.DNSResolutionCheck{},
		&diagnostics.InternalDNSCheck{},
		&diagnostics.ContainerConnectivityCheck{},
		&diagnostics.PortBindingCheck{},
		&diagnostics.NetworkIsolationCheck{},
		&diagnostics.MTUConsistencyCheck{},
		&diagnostics.SubnetOverlapCheck{},
	}
	
	ctx := context.Background()
	dockerClient := createMockDockerClient(b)
	defer dockerClient.Close()
	
	// Benchmark each check individually
	for _, check := range checks {
		b.Run(check.Name(), func(b *testing.B) {
			benchmarkSingleCheck(b, ctx, dockerClient, check)
		})
	}
	
	// Also run a comparison benchmark
	b.Run("AllChecksComparison", func(b *testing.B) {
		benchmarkAllChecksComparison(b, ctx, dockerClient, checks)
	})
}

// benchmarkSingleCheck measures performance of a single diagnostic check
func benchmarkSingleCheck(b *testing.B, ctx context.Context, client *docker.Client, check diagnostics.Check) {
	b.ResetTimer()
	b.ReportAllocs()
	
	var totalDuration time.Duration
	var minDuration time.Duration = time.Hour
	var maxDuration time.Duration
	
	for i := 0; i < b.N; i++ {
		start := time.Now()
		result, err := check.Run(ctx, client)
		duration := time.Since(start)
		
		// Track timing statistics
		totalDuration += duration
		if duration < minDuration {
			minDuration = duration
		}
		if duration > maxDuration {
			maxDuration = duration
		}
		
		// Prevent compiler optimization
		_ = result
		_ = err
	}
	
	// Report detailed metrics
	avgDuration := totalDuration / time.Duration(b.N)
	b.ReportMetric(float64(avgDuration.Microseconds()), "μs/op")
	b.ReportMetric(float64(minDuration.Microseconds()), "μs/min")
	b.ReportMetric(float64(maxDuration.Microseconds()), "μs/max")
	
	// Log check severity for reference
	b.Logf("Check: %s, Severity: %v, Avg: %v", 
		check.Name(), 
		check.Severity(), 
		avgDuration)
}

// benchmarkAllChecksComparison runs all checks and compares their performance
func benchmarkAllChecksComparison(b *testing.B, ctx context.Context, client *docker.Client, checks []diagnostics.Check) {
	b.ResetTimer()
	
	checkMetrics := make(map[string]*IndividualCheckMetrics)
	
	// Initialize metrics for each check
	for _, check := range checks {
		checkMetrics[check.Name()] = &IndividualCheckMetrics{
			CheckName: check.Name(),
			MinTime:   time.Hour,
		}
	}
	
	// Run all checks b.N times
	for i := 0; i < b.N; i++ {
		for _, check := range checks {
			start := time.Now()
			check.Run(ctx, client)
			duration := time.Since(start)
			
			// Update metrics
			metrics := checkMetrics[check.Name()]
			metrics.AverageTime += duration
			if duration < metrics.MinTime {
				metrics.MinTime = duration
			}
			if duration > metrics.MaxTime {
				metrics.MaxTime = duration
			}
		}
	}
	
	// Calculate averages and report
	b.Logf("\n=== Performance Comparison of All Checks ===")
	b.Logf("%-30s | %-12s | %-12s | %-12s", "Check Name", "Avg Time", "Min Time", "Max Time")
	b.Logf("%s", string(make([]byte, 80)))
	
	var totalTime time.Duration
	for _, check := range checks {
		metrics := checkMetrics[check.Name()]
		metrics.AverageTime = metrics.AverageTime / time.Duration(b.N)
		totalTime += metrics.AverageTime
		
		b.Logf("%-30s | %-12v | %-12v | %-12v",
			metrics.CheckName,
			metrics.AverageTime,
			metrics.MinTime,
			metrics.MaxTime,
		)
	}
	
	b.Logf("\nTotal Sequential Time: %v", totalTime)
	b.Logf("Potential Parallel Time (longest check): %v", findLongestCheck(checkMetrics))
	b.Logf("Theoretical Speedup: %.2fx", float64(totalTime)/float64(findLongestCheck(checkMetrics)))
}

// BenchmarkCriticalChecksOnly benchmarks only critical severity checks
func BenchmarkCriticalChecksOnly(b *testing.B) {
	criticalChecks := []diagnostics.Check{
		&diagnostics.DaemonConnectivityCheck{},
		&diagnostics.IPForwardingCheck{},
		&diagnostics.IptablesCheck{},
	}
	
	ctx := context.Background()
	dockerClient := createMockDockerClient(b)
	defer dockerClient.Close()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		for _, check := range criticalChecks {
			check.Run(ctx, dockerClient)
		}
	}
}

// BenchmarkNetworkChecksOnly benchmarks only network-related checks
func BenchmarkNetworkChecksOnly(b *testing.B) {
	networkChecks := []diagnostics.Check{
		&diagnostics.BridgeNetworkCheck{},
		&diagnostics.SubnetOverlapCheck{},
		&diagnostics.MTUConsistencyCheck{},
		&diagnostics.NetworkIsolationCheck{},
	}
	
	ctx := context.Background()
	dockerClient := createMockDockerClient(b)
	defer dockerClient.Close()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		for _, check := range networkChecks {
			check.Run(ctx, dockerClient)
		}
	}
}

// BenchmarkDNSChecksOnly benchmarks only DNS-related checks
func BenchmarkDNSChecksOnly(b *testing.B) {
	dnsChecks := []diagnostics.Check{
		&diagnostics.DNSResolutionCheck{},
		&diagnostics.InternalDNSCheck{},
	}
	
	ctx := context.Background()
	dockerClient := createMockDockerClient(b)
	defer dockerClient.Close()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		for _, check := range dnsChecks {
			check.Run(ctx, dockerClient)
		}
	}
}

// BenchmarkCheckCategories benchmarks checks grouped by category
func BenchmarkCheckCategories(b *testing.B) {
	categories := map[string][]diagnostics.Check{
		"System": {
			&diagnostics.DaemonConnectivityCheck{},
			&diagnostics.IPForwardingCheck{},
			&diagnostics.IptablesCheck{},
		},
		"Network": {
			&diagnostics.BridgeNetworkCheck{},
			&diagnostics.SubnetOverlapCheck{},
			&diagnostics.MTUConsistencyCheck{},
		},
		"DNS": {
			&diagnostics.DNSResolutionCheck{},
			&diagnostics.InternalDNSCheck{},
		},
		"Container": {
			&diagnostics.ContainerConnectivityCheck{},
			&diagnostics.PortBindingCheck{},
			&diagnostics.NetworkIsolationCheck{},
		},
	}
	
	ctx := context.Background()
	dockerClient := createMockDockerClient(b)
	defer dockerClient.Close()
	
	for categoryName, checks := range categories {
		b.Run(categoryName, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				for _, check := range checks {
					check.Run(ctx, dockerClient)
				}
			}
			
			b.ReportMetric(float64(len(checks)), "checks")
		})
	}
}

// BenchmarkCheckWithTimeout measures check performance with timeouts
func BenchmarkCheckWithTimeout(b *testing.B) {
	timeouts := []time.Duration{
		100 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
	}
	
	dockerClient := createMockDockerClient(b)
	defer dockerClient.Close()
	
	check := &diagnostics.ContainerConnectivityCheck{}
	
	for _, timeout := range timeouts {
		b.Run(fmt.Sprintf("Timeout-%v", timeout), func(b *testing.B) {
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				check.Run(ctx, dockerClient)
				cancel()
			}
		})
	}
}

// Helper function to find the longest-running check
func findLongestCheck(metrics map[string]*IndividualCheckMetrics) time.Duration {
	var longest time.Duration
	for _, m := range metrics {
		if m.AverageTime > longest {
			longest = m.AverageTime
		}
	}
	return longest
}