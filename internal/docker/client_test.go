package docker_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// MockDockerClient provides a mock implementation for testing
type MockDockerClient struct {
	mock.Mock
}

func TestNewClient(t *testing.T) {
	ctx := context.Background()
	
	t.Run("WithoutDocker", func(t *testing.T) {
		// This test will work even without Docker daemon
		client, err := docker.NewClient(ctx)
		if err != nil {
			t.Logf("Docker not available (expected in CI): %v", err)
			assert.Contains(t, err.Error(), "Docker")
			return
		}
		defer client.Close()
		
		// If we get here, Docker is available
		assert.NotNil(t, client)
		assert.False(t, client.IsEnhanced()) // Default should be non-enhanced
		
		// Test basic functionality
		err = client.Ping(ctx)
		assert.NoError(t, err)
	})
	
	t.Run("WithEnhancedClient", func(t *testing.T) {
		opts := &docker.ClientOptions{
			UseEnhanced: true,
			EnhancedConfig: &docker.EnhancedClientConfig{
				RateLimit:      10.0,
				RateBurst:      20,
				CacheTTL:       5 * time.Second,
				MaxCacheEntries: 50,
				MaxConnections: 5,
				Timeout:        10 * time.Second,
				MaxRetries:     2,
				RetryBackoff:   50 * time.Millisecond,
			},
		}
		
		client, err := docker.NewClientWithOptions(ctx, opts)
		if err != nil {
			t.Skipf("Docker not available: %v", err)
		}
		defer client.Close()
		
		assert.NotNil(t, client)
		assert.True(t, client.IsEnhanced())
		
		// Test enhanced client features
		metrics := client.GetMetrics()
		assert.NotNil(t, metrics)
		
		cacheStats := client.GetCacheStats()
		assert.NotNil(t, cacheStats)
	})
	
	t.Run("WithDefaultEnhancedConfig", func(t *testing.T) {
		opts := &docker.ClientOptions{
			UseEnhanced: true,
			// EnhancedConfig: nil, // Should use defaults
		}
		
		client, err := docker.NewClientWithOptions(ctx, opts)
		if err != nil {
			t.Skipf("Docker not available: %v", err)
		}
		defer client.Close()
		
		assert.True(t, client.IsEnhanced())
	})
}

func TestClientContainerOperations(t *testing.T) {
	ctx := context.Background()
	
	client, err := docker.NewClient(ctx)
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer client.Close()
	
	t.Run("ListContainers", func(t *testing.T) {
		containers, err := client.ListContainers(ctx)
		assert.NoError(t, err)
		assert.NotNil(t, containers)
		// containers can be empty, that's fine
		t.Logf("Found %d containers", len(containers))
	})
	
	t.Run("InspectNonExistentContainer", func(t *testing.T) {
		_, err := client.InspectContainer("nonexistent-container-id")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "No such container")
	})
}

func TestClientNetworkOperations(t *testing.T) {
	ctx := context.Background()
	
	client, err := docker.NewClient(ctx)
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer client.Close()
	
	t.Run("GetNetworkInfo", func(t *testing.T) {
		networks, err := client.GetNetworkInfo()
		assert.NoError(t, err)
		assert.NotEmpty(t, networks) // Should at least have default networks
		
		// Check that bridge network exists
		bridgeFound := false
		for _, net := range networks {
			if net.Name == "bridge" {
				bridgeFound = true
				assert.Equal(t, "bridge", net.Driver)
				assert.NotEmpty(t, net.ID)
				break
			}
		}
		assert.True(t, bridgeFound, "bridge network should exist")
	})
	
	t.Run("GetContainerNetworkConfigNonExistent", func(t *testing.T) {
		_, err := client.GetContainerNetworkConfig("nonexistent-container")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "No such container")
	})
}

func TestClientPingAndClose(t *testing.T) {
	ctx := context.Background()
	
	client, err := docker.NewClient(ctx)
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	
	t.Run("Ping", func(t *testing.T) {
		err := client.Ping(ctx)
		assert.NoError(t, err)
	})
	
	t.Run("Close", func(t *testing.T) {
		err := client.Close()
		assert.NoError(t, err)
		
		// Second close should not error
		err = client.Close()
		assert.NoError(t, err)
	})
}

func TestClientExecInContainer(t *testing.T) {
	ctx := context.Background()
	
	client, err := docker.NewClient(ctx)
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer client.Close()
	
	t.Run("ExecInNonExistentContainer", func(t *testing.T) {
		_, err := client.ExecInContainer(ctx, "nonexistent", []string{"echo", "hello"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "No such container")
	})
}

func TestClientOptions(t *testing.T) {
	ctx := context.Background()
	
	t.Run("NilOptions", func(t *testing.T) {
		client, err := docker.NewClientWithOptions(ctx, nil)
		if err != nil {
			t.Skipf("Docker not available: %v", err)
		}
		defer client.Close()
		
		assert.False(t, client.IsEnhanced())
		assert.Nil(t, client.GetMetrics())
		assert.Nil(t, client.GetCacheStats())
	})
	
	t.Run("NonEnhancedOptions", func(t *testing.T) {
		opts := &docker.ClientOptions{
			UseEnhanced: false,
		}
		
		client, err := docker.NewClientWithOptions(ctx, opts)
		if err != nil {
			t.Skipf("Docker not available: %v", err)
		}
		defer client.Close()
		
		assert.False(t, client.IsEnhanced())
		assert.Nil(t, client.GetMetrics())
		assert.Nil(t, client.GetCacheStats())
	})
}

func TestClientCacheOperations(t *testing.T) {
	ctx := context.Background()
	
	opts := &docker.ClientOptions{
		UseEnhanced: true,
		EnhancedConfig: &docker.EnhancedClientConfig{
			RateLimit:      100.0, // High rate limit for testing
			RateBurst:      200,
			CacheTTL:       1 * time.Second,
			MaxCacheEntries: 10,
			MaxConnections: 5,
			Timeout:        5 * time.Second,
		},
	}
	
	client, err := docker.NewClientWithOptions(ctx, opts)
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer client.Close()
	
	t.Run("CacheInvalidation", func(t *testing.T) {
		// Get initial stats
		stats1 := client.GetCacheStats()
		
		// Make some calls to populate cache
		_, _ = client.ListContainers(ctx)
		_, _ = client.GetNetworkInfo()
		
		// Check cache has entries
		stats2 := client.GetCacheStats()
		assert.GreaterOrEqual(t, stats2.Entries, stats1.Entries)
		
		// Invalidate cache
		client.InvalidateCache()
		
		// Check cache is cleared
		stats3 := client.GetCacheStats()
		assert.Equal(t, 0, stats3.Entries)
	})
	
	t.Run("CacheHitRateCalculation", func(t *testing.T) {
		// Clear cache first
		client.InvalidateCache()
		
		// First call should be cache miss
		_, err := client.ListContainers(ctx)
		require.NoError(t, err)
		
		// Second call should be cache hit
		_, err = client.ListContainers(ctx)
		require.NoError(t, err)
		
		stats := client.GetCacheStats()
		assert.Greater(t, stats.Hits, int64(0))
		assert.Greater(t, stats.HitRate, 0.0)
		assert.LessOrEqual(t, stats.HitRate, 1.0)
	})
}

func TestClientMetrics(t *testing.T) {
	ctx := context.Background()
	
	opts := &docker.ClientOptions{
		UseEnhanced: true,
		EnhancedConfig: &docker.EnhancedClientConfig{
			RateLimit:      100.0,
			RateBurst:      200,
			CacheTTL:       1 * time.Second,
			MaxCacheEntries: 10,
		},
	}
	
	client, err := docker.NewClientWithOptions(ctx, opts)
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer client.Close()
	
	t.Run("MetricsTracking", func(t *testing.T) {
		// Get initial metrics
		metrics1 := client.GetMetrics()
		initialCalls := metrics1.TotalAPICalls
		
		// Make some API calls
		_, _ = client.ListContainers(ctx)
		_, _ = client.GetNetworkInfo()
		_ = client.Ping(ctx)
		
		// Check metrics updated
		metrics2 := client.GetMetrics()
		assert.Greater(t, metrics2.TotalAPICalls, initialCalls)
		assert.GreaterOrEqual(t, metrics2.SuccessfulCalls, int64(3))
		assert.GreaterOrEqual(t, metrics2.SuccessRate, 0.0)
		assert.LessOrEqual(t, metrics2.SuccessRate, 1.0)
		
		// Check operation-specific metrics
		assert.Contains(t, metrics2.OperationDurations, "container_list")
		assert.Contains(t, metrics2.OperationDurations, "network_list")
		assert.Contains(t, metrics2.OperationDurations, "ping")
	})
}

// Benchmark tests
func BenchmarkClientListContainers(b *testing.B) {
	ctx := context.Background()
	
	client, err := docker.NewClient(ctx)
	if err != nil {
		b.Skipf("Docker not available: %v", err)
	}
	defer client.Close()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.ListContainers(ctx)
	}
}

func BenchmarkEnhancedClientListContainers(b *testing.B) {
	ctx := context.Background()
	
	opts := &docker.ClientOptions{
		UseEnhanced: true,
		EnhancedConfig: &docker.EnhancedClientConfig{
			RateLimit:      1000.0,
			RateBurst:      2000,
			CacheTTL:       30 * time.Second,
			MaxCacheEntries: 100,
		},
	}
	
	client, err := docker.NewClientWithOptions(ctx, opts)
	if err != nil {
		b.Skipf("Docker not available: %v", err)
	}
	defer client.Close()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.ListContainers(ctx)
	}
	
	// Report cache hit rate
	stats := client.GetCacheStats()
	b.Logf("Cache hit rate: %.2f%%", stats.HitRate*100)
}

func BenchmarkClientGetNetworkInfo(b *testing.B) {
	ctx := context.Background()
	
	client, err := docker.NewClient(ctx)
	if err != nil {
		b.Skipf("Docker not available: %v", err)
	}
	defer client.Close()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.GetNetworkInfo()
	}
}

func BenchmarkClientPing(b *testing.B) {
	ctx := context.Background()
	
	client, err := docker.NewClient(ctx)
	if err != nil {
		b.Skipf("Docker not available: %v", err)
	}
	defer client.Close()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = client.Ping(ctx)
	}
}

// TestClientContextCancellation tests context cancellation handling
func TestClientContextCancellation(t *testing.T) {
	client, err := docker.NewClient(context.Background())
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer client.Close()
	
	t.Run("CancelledContext", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately
		
		_, err := client.ListContainers(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "context canceled")
	})
	
	t.Run("TimeoutContext", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		
		// Give it a moment to timeout
		time.Sleep(1 * time.Millisecond)
		
		_, err := client.ListContainers(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "context deadline exceeded")
	})
}

// TestDataStructures tests the data structure conversions
func TestDataStructures(t *testing.T) {
	t.Run("NetworkDiagnostic", func(t *testing.T) {
		diag := docker.NetworkDiagnostic{
			Name:     "test-network",
			ID:       "test-id",
			Driver:   "bridge",
			Scope:    "local",
			Internal: false,
			IPAM: docker.IPAMConfig{
				Driver: "default",
				Configs: []docker.IPAMConfigBlock{
					{
						Subnet:  "172.17.0.0/16",
						Gateway: "172.17.0.1",
					},
				},
			},
			Containers: map[string]docker.ContainerEndpoint{
				"container1": {
					Name:        "test-container",
					IPv4Address: "172.17.0.2",
					IPv6Address: "",
					MacAddress:  "02:42:ac:11:00:02",
				},
			},
		}
		
		assert.Equal(t, "test-network", diag.Name)
		assert.Equal(t, "bridge", diag.Driver)
		assert.Len(t, diag.IPAM.Configs, 1)
		assert.Equal(t, "172.17.0.0/16", diag.IPAM.Configs[0].Subnet)
		assert.Len(t, diag.Containers, 1)
	})
	
	t.Run("ContainerNetworkInfo", func(t *testing.T) {
		info := docker.ContainerNetworkInfo{
			Hostname:   "test-host",
			Domainname: "example.com",
			DNS:        []string{"8.8.8.8", "8.8.4.4"},
			DNSSearch:  []string{"example.com"},
			DNSOptions: []string{},
			Networks: map[string]docker.NetworkEndpoint{
				"bridge": {
					IPAddress:  "172.17.0.2",
					Gateway:    "172.17.0.1",
					MacAddress: "02:42:ac:11:00:02",
				},
			},
		}
		
		assert.Equal(t, "test-host", info.Hostname)
		assert.Len(t, info.DNS, 2)
		assert.Len(t, info.Networks, 1)
		assert.Equal(t, "172.17.0.2", info.Networks["bridge"].IPAddress)
	})
}