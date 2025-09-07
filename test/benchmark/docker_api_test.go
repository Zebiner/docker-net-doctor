// test/benchmark/docker_api_test.go
package benchmark

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// BenchmarkDockerAPIOperations measures the performance of individual Docker API calls
func BenchmarkDockerAPIOperations(b *testing.B) {
	ctx := context.Background()
	
	client, err := docker.NewClient(ctx)
	if err != nil {
		b.Skip("Docker not available:", err)
	}
	defer client.Close()
	
	// Benchmark individual Docker API operations
	b.Run("ListContainers", func(b *testing.B) {
		benchmarkListContainers(b, ctx, client)
	})
	
	b.Run("GetNetworkInfo", func(b *testing.B) {
		benchmarkGetNetworkInfo(b, ctx, client)
	})
	
	b.Run("InspectContainer", func(b *testing.B) {
		benchmarkInspectContainer(b, ctx, client)
	})
	
	b.Run("ExecInContainer", func(b *testing.B) {
		benchmarkExecInContainer(b, ctx, client)
	})
	
	b.Run("APIVersionNegotiation", func(b *testing.B) {
		benchmarkAPIVersionNegotiation(b, ctx)
	})
}

// benchmarkListContainers measures container listing performance
func benchmarkListContainers(b *testing.B, ctx context.Context, client *docker.Client) {
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		containers, err := client.ListContainers(ctx)
		_ = containers
		_ = err
	}
}

// benchmarkGetNetworkInfo measures network inspection performance
func benchmarkGetNetworkInfo(b *testing.B, ctx context.Context, client *docker.Client) {
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		networks, err := client.GetNetworkInfo()
		_ = networks
		_ = err
	}
}

// benchmarkInspectContainer measures container inspection performance
func benchmarkInspectContainer(b *testing.B, ctx context.Context, client *docker.Client) {
	// Get a container ID for testing (if available)
	containers, err := client.ListContainers(ctx)
	if err != nil || len(containers) == 0 {
		b.Skip("No containers available for inspection benchmark")
	}
	
	containerID := containers[0].ID
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		info, err := client.InspectContainer(containerID)
		_ = info
		_ = err
	}
}

// benchmarkExecInContainer measures command execution performance
func benchmarkExecInContainer(b *testing.B, ctx context.Context, client *docker.Client) {
	// Get a running container for testing
	containers, err := client.ListContainers(ctx)
	if err != nil || len(containers) == 0 {
		b.Skip("No containers available for exec benchmark")
	}
	
	var runningContainer string
	for _, container := range containers {
		if container.State == "running" {
			runningContainer = container.ID
			break
		}
	}
	
	if runningContainer == "" {
		b.Skip("No running containers available for exec benchmark")
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		output, err := client.ExecInContainer(ctx, runningContainer, []string{"echo", "test"})
		_ = output
		_ = err
	}
}

// benchmarkAPIVersionNegotiation measures the overhead of client creation
func benchmarkAPIVersionNegotiation(b *testing.B, ctx context.Context) {
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		client, err := docker.NewClient(ctx)
		if err == nil {
			client.Close()
		}
	}
}

// BenchmarkDockerAPIBatchOperations measures batch API operation performance
func BenchmarkDockerAPIBatchOperations(b *testing.B) {
	ctx := context.Background()
	
	client, err := docker.NewClient(ctx)
	if err != nil {
		b.Skip("Docker not available:", err)
	}
	defer client.Close()
	
	b.Run("Sequential", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Perform multiple API calls sequentially
			client.ListContainers(ctx)
			client.GetNetworkInfo()
			client.ListContainers(ctx)
			client.GetNetworkInfo()
		}
	})
	
	b.Run("Parallel", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Perform multiple API calls in parallel
			done := make(chan bool, 4)
			
			go func() {
				client.ListContainers(ctx)
				done <- true
			}()
			go func() {
				client.GetNetworkInfo()
				done <- true
			}()
			go func() {
				client.ListContainers(ctx)
				done <- true
			}()
			go func() {
				client.GetNetworkInfo()
				done <- true
			}()
			
			// Wait for all operations to complete
			for j := 0; j < 4; j++ {
				<-done
			}
		}
	})
}

// BenchmarkDockerAPILatency measures raw API call latency
func BenchmarkDockerAPILatency(b *testing.B) {
	ctx := context.Background()
	
	client, err := docker.NewClient(ctx)
	if err != nil {
		b.Skip("Docker not available:", err)
	}
	defer client.Close()
	
	// Measure latency distribution
	latencies := make([]time.Duration, 0, b.N)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		start := time.Now()
		client.Ping(ctx)
		latency := time.Since(start)
		latencies = append(latencies, latency)
	}
	
	// Calculate percentiles
	if len(latencies) > 0 {
		p50 := calculatePercentile(latencies, 50)
		p95 := calculatePercentile(latencies, 95)
		p99 := calculatePercentile(latencies, 99)
		
		b.ReportMetric(float64(p50.Microseconds()), "μs/p50")
		b.ReportMetric(float64(p95.Microseconds()), "μs/p95")
		b.ReportMetric(float64(p99.Microseconds()), "μs/p99")
		
		b.Logf("Latency P50: %v, P95: %v, P99: %v", p50, p95, p99)
	}
}

// BenchmarkDockerAPIThroughput measures API operations per second
func BenchmarkDockerAPIThroughput(b *testing.B) {
	ctx := context.Background()
	
	client, err := docker.NewClient(ctx)
	if err != nil {
		b.Skip("Docker not available:", err)
	}
	defer client.Close()
	
	// Test different operation types
	operations := []struct {
		name string
		op   func()
	}{
		{
			name: "Ping",
			op: func() {
				client.Ping(ctx)
			},
		},
		{
			name: "ListContainers",
			op: func() {
				client.ListContainers(ctx)
			},
		},
		{
			name: "GetNetworkInfo",
			op: func() {
				client.GetNetworkInfo()
			},
		},
	}
	
	for _, test := range operations {
		b.Run(test.name, func(b *testing.B) {
			// Measure operations completed in 1 second
			timeout := time.After(1 * time.Second)
			operations := 0
			
			b.ResetTimer()
			
		loop:
			for {
				select {
				case <-timeout:
					break loop
				default:
					test.op()
					operations++
				}
			}
			
			b.ReportMetric(float64(operations), "ops/sec")
			b.Logf("%s: %d operations per second", test.name, operations)
		})
	}
}

// BenchmarkDockerAPIResourceUsage measures resource consumption during API calls
func BenchmarkDockerAPIResourceUsage(b *testing.B) {
	ctx := context.Background()
	
	// Test with different numbers of concurrent clients
	clientCounts := []int{1, 5, 10, 20}
	
	for _, numClients := range clientCounts {
		b.Run(fmt.Sprintf("Clients-%d", numClients), func(b *testing.B) {
			clients := make([]*docker.Client, numClients)
			
			// Create clients
			for i := 0; i < numClients; i++ {
				client, err := docker.NewClient(ctx)
				if err != nil {
					b.Skip("Failed to create Docker client:", err)
				}
				clients[i] = client
			}
			
			// Clean up clients after test
			defer func() {
				for _, client := range clients {
					if client != nil {
						client.Close()
					}
				}
			}()
			
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				// Each client performs an operation
				for _, client := range clients {
					client.ListContainers(ctx)
				}
			}
			
			b.ReportMetric(float64(numClients), "clients")
		})
	}
}

// Helper function to calculate percentile
func calculatePercentile(durations []time.Duration, percentile float64) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	
	// Simple percentile calculation (not exact but good enough for benchmarking)
	index := int(float64(len(durations)) * percentile / 100)
	if index >= len(durations) {
		index = len(durations) - 1
	}
	
	return durations[index]
}