package docker

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseCacheDetailed(t *testing.T) {
	cache := NewResponseCache(5*time.Second, 10)
	defer cache.Close()
	
	t.Run("InitialState", func(t *testing.T) {
		stats := cache.GetStats()
		assert.Equal(t, int64(0), stats.Hits)
		assert.Equal(t, int64(0), stats.Misses)
		assert.Equal(t, int64(0), stats.Evictions)
		assert.Equal(t, 0, stats.Entries)
		assert.Equal(t, 0.0, stats.HitRate)
	})
}

func TestContainerListCache(t *testing.T) {
	cache := NewResponseCache(5*time.Second, 10)
	defer cache.Close()
	
	containers := []types.Container{
		{
			ID:    "container1",
			Names: []string{"/test-container"},
			Image: "nginx:latest",
			State: "running",
		},
		{
			ID:    "container2", 
			Names: []string{"/another-container"},
			Image: "redis:alpine",
			State: "running",
		},
	}
	
	t.Run("SetAndGet", func(t *testing.T) {
		// Cache miss on first request
		_, found := cache.GetContainerList("test-key")
		assert.False(t, found)
		
		// Set value
		cache.SetContainerList("test-key", containers, 5*time.Second)
		
		// Cache hit on second request
		result, found := cache.GetContainerList("test-key")
		assert.True(t, found)
		assert.Len(t, result, 2)
		assert.Equal(t, "container1", result[0].ID)
		assert.Equal(t, "container2", result[1].ID)
	})
	
	t.Run("DifferentKeys", func(t *testing.T) {
		cache.SetContainerList("key1", containers[:1], 5*time.Second)
		cache.SetContainerList("key2", containers[1:], 5*time.Second)
		
		result1, found1 := cache.GetContainerList("key1")
		assert.True(t, found1)
		assert.Len(t, result1, 1)
		assert.Equal(t, "container1", result1[0].ID)
		
		result2, found2 := cache.GetContainerList("key2")
		assert.True(t, found2)
		assert.Len(t, result2, 1)
		assert.Equal(t, "container2", result2[0].ID)
		
		// Non-existent key
		_, found3 := cache.GetContainerList("key3")
		assert.False(t, found3)
	})
	
	t.Run("TTLExpiration", func(t *testing.T) {
		// Set with very short TTL
		cache.SetContainerList("expire-key", containers, 50*time.Millisecond)
		
		// Should exist immediately
		_, found := cache.GetContainerList("expire-key")
		assert.True(t, found)
		
		// Wait for expiration
		time.Sleep(100 * time.Millisecond)
		
		// Should be expired
		_, found = cache.GetContainerList("expire-key")
		assert.False(t, found)
	})
	
	t.Run("DefaultTTL", func(t *testing.T) {
		// Set with zero TTL (should use default)
		cache.SetContainerList("default-ttl", containers, 0)
		
		// Should exist
		_, found := cache.GetContainerList("default-ttl")
		assert.True(t, found)
	})
}

func TestNetworkListCache(t *testing.T) {
	cache := NewResponseCache(5*time.Second, 10)
	defer cache.Close()
	
	networks := []network.Summary{
		{
			Name:   "bridge",
			ID:     "network1",
			Driver: "bridge",
			Scope:  "local",
		},
		{
			Name:   "host",
			ID:     "network2", 
			Driver: "host",
			Scope:  "local",
		},
	}
	
	t.Run("SetAndGet", func(t *testing.T) {
		// Cache miss first
		_, found := cache.GetNetworkList("net-key")
		assert.False(t, found)
		
		// Set value
		cache.SetNetworkList("net-key", networks, 5*time.Second)
		
		// Cache hit second
		result, found := cache.GetNetworkList("net-key")
		assert.True(t, found)
		assert.Len(t, result, 2)
		assert.Equal(t, "bridge", result[0].Name)
		assert.Equal(t, "host", result[1].Name)
	})
}

func TestNetworkInspectCache(t *testing.T) {
	cache := NewResponseCache(5*time.Second, 10)
	defer cache.Close()
	
	networkInspect := network.Inspect{
		Name:   "bridge",
		ID:     "network-inspect-id",
		Driver: "bridge",
		Scope:  "local",
		IPAM: network.IPAM{
			Driver: "default",
			Config: []network.IPAMConfig{
				{
					Subnet:  "172.17.0.0/16",
					Gateway: "172.17.0.1",
				},
			},
		},
		Containers: map[string]network.EndpointResource{
			"container1": {
				Name:        "test-container",
				IPv4Address: "172.17.0.2/16",
				IPv6Address: "",
				MacAddress:  "02:42:ac:11:00:02",
			},
		},
	}
	
	t.Run("SetAndGet", func(t *testing.T) {
		networkID := "test-network-id"
		
		// Cache miss first
		_, found := cache.GetNetworkInspect(networkID)
		assert.False(t, found)
		
		// Set value
		cache.SetNetworkInspect(networkID, networkInspect, 5*time.Second)
		
		// Cache hit second
		result, found := cache.GetNetworkInspect(networkID)
		assert.True(t, found)
		assert.Equal(t, "bridge", result.Name)
		assert.Equal(t, "network-inspect-id", result.ID)
		assert.Equal(t, "bridge", result.Driver)
		assert.Len(t, result.Containers, 1)
	})
}

func TestContainerInspectCache(t *testing.T) {
	cache := NewResponseCache(5*time.Second, 10)
	defer cache.Close()
	
	containerInspect := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:    "container-inspect-id",
			Name:  "/test-container",
			State: &types.ContainerState{Status: "running"},
		},
		Config: &container.Config{
			Hostname: "test-host",
			Image:    "nginx:latest",
		},
		NetworkSettings: &types.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"bridge": {
					IPAddress: "172.17.0.2",
					Gateway:   "172.17.0.1",
				},
			},
		},
	}
	
	t.Run("SetAndGet", func(t *testing.T) {
		containerID := "test-container-id"
		
		// Cache miss first
		_, found := cache.GetContainerInspect(containerID)
		assert.False(t, found)
		
		// Set value
		cache.SetContainerInspect(containerID, containerInspect, 5*time.Second)
		
		// Cache hit second
		result, found := cache.GetContainerInspect(containerID)
		assert.True(t, found)
		assert.Equal(t, "container-inspect-id", result.ID)
		assert.Equal(t, "/test-container", result.Name)
		assert.Equal(t, "running", result.State.Status)
	})
}

func TestCacheEviction(t *testing.T) {
	// Small cache for testing eviction
	cache := NewResponseCache(5*time.Second, 3)
	defer cache.Close()
	
	containers := []types.Container{
		{ID: "container1", Names: []string{"/test1"}, State: "running"},
	}
	
	t.Run("MaxEntriesEviction", func(t *testing.T) {
		// Fill cache to capacity
		cache.SetContainerList("key1", containers, 5*time.Second)
		cache.SetContainerList("key2", containers, 5*time.Second)
		cache.SetContainerList("key3", containers, 5*time.Second)
		
		stats := cache.GetStats()
		assert.Equal(t, 3, stats.Entries)
		assert.Equal(t, int64(0), stats.Evictions)
		
		// Add one more to trigger eviction
		cache.SetContainerList("key4", containers, 5*time.Second)
		
		stats = cache.GetStats()
		assert.Equal(t, 3, stats.Entries)
		assert.Equal(t, int64(1), stats.Evictions)
		
		// Oldest entry should be evicted
		_, found1 := cache.GetContainerList("key1")
		assert.False(t, found1, "Oldest entry should be evicted")
		
		// Newer entries should still exist
		_, found4 := cache.GetContainerList("key4")
		assert.True(t, found4, "Newest entry should exist")
	})
}

func TestCacheInvalidation(t *testing.T) {
	cache := NewResponseCache(5*time.Second, 10)
	defer cache.Close()
	
	containers := []types.Container{
		{ID: "container1", Names: []string{"/test1"}, State: "running"},
	}
	
	t.Run("InvalidateSpecific", func(t *testing.T) {
		cache.SetContainerList("key1", containers, 5*time.Second)
		cache.SetContainerList("key2", containers, 5*time.Second)
		
		// Both should exist
		_, found1 := cache.GetContainerList("key1")
		_, found2 := cache.GetContainerList("key2")
		assert.True(t, found1)
		assert.True(t, found2)
		
		// Invalidate one
		cache.Invalidate("key1")
		
		// key1 should be gone, key2 should remain
		_, found1 = cache.GetContainerList("key1")
		_, found2 = cache.GetContainerList("key2")
		assert.False(t, found1)
		assert.True(t, found2)
	})
	
	t.Run("InvalidateAll", func(t *testing.T) {
		cache.SetContainerList("key1", containers, 5*time.Second)
		cache.SetContainerList("key2", containers, 5*time.Second)
		
		stats := cache.GetStats()
		assert.Greater(t, stats.Entries, 0)
		
		cache.InvalidateAll()
		
		stats = cache.GetStats()
		assert.Equal(t, 0, stats.Entries)
		
		// Neither should exist
		_, found1 := cache.GetContainerList("key1")
		_, found2 := cache.GetContainerList("key2")
		assert.False(t, found1)
		assert.False(t, found2)
	})
}

func TestCacheStats(t *testing.T) {
	cache := NewResponseCache(5*time.Second, 10)
	defer cache.Close()
	
	containers := []types.Container{
		{ID: "container1", Names: []string{"/test1"}, State: "running"},
	}
	
	t.Run("HitMissTracking", func(t *testing.T) {
		// Initial stats
		stats := cache.GetStats()
		initialHits := stats.Hits
		initialMisses := stats.Misses
		
		// Cache miss
		_, found := cache.GetContainerList("miss-key")
		assert.False(t, found)
		
		stats = cache.GetStats()
		assert.Equal(t, initialMisses+1, stats.Misses)
		assert.Equal(t, initialHits, stats.Hits)
		
		// Set value
		cache.SetContainerList("hit-key", containers, 5*time.Second)
		
		// Cache hit
		_, found = cache.GetContainerList("hit-key")
		assert.True(t, found)
		
		stats = cache.GetStats()
		assert.Equal(t, initialHits+1, stats.Hits)
		assert.Greater(t, stats.HitRate, 0.0)
		assert.LessOrEqual(t, stats.HitRate, 1.0)
	})
	
	t.Run("HitRateCalculation", func(t *testing.T) {
		cache.InvalidateAll()
		
		// Generate known hits and misses
		cache.SetContainerList("hit-test", containers, 5*time.Second)
		
		// 2 hits
		_, _ = cache.GetContainerList("hit-test")
		_, _ = cache.GetContainerList("hit-test")
		
		// 1 miss
		_, _ = cache.GetContainerList("miss-test")
		
		stats := cache.GetStats()
		expectedHitRate := 2.0 / 3.0 // 2 hits out of 3 total requests
		assert.InDelta(t, expectedHitRate, stats.HitRate, 0.01)
	})
}

func TestGetOrCompute(t *testing.T) {
	cache := NewResponseCache(5*time.Second, 10)
	defer cache.Close()
	
	t.Run("ComputeOnMiss", func(t *testing.T) {
		ctx := context.Background()
		computeCalled := 0
		expectedValue := "computed-value"
		
		// First call should compute
		result, err := cache.GetOrCompute(ctx, "compute-key", func() (interface{}, error) {
			computeCalled++
			return expectedValue, nil
		}, 5*time.Second)
		
		require.NoError(t, err)
		assert.Equal(t, expectedValue, result)
		assert.Equal(t, 1, computeCalled)
		
		// Second call should hit cache
		result2, err := cache.GetOrCompute(ctx, "compute-key", func() (interface{}, error) {
			computeCalled++
			return "should-not-be-called", nil
		}, 5*time.Second)
		
		require.NoError(t, err)
		assert.Equal(t, expectedValue, result2)
		assert.Equal(t, 1, computeCalled) // Should not have been called again
	})
	
	t.Run("ComputeError", func(t *testing.T) {
		ctx := context.Background()
		expectedError := assert.AnError
		
		result, err := cache.GetOrCompute(ctx, "error-key", func() (interface{}, error) {
			return nil, expectedError
		}, 5*time.Second)
		
		assert.Error(t, err)
		assert.Equal(t, expectedError, err)
		assert.Nil(t, result)
		
		// Error should not be cached
		stats := cache.GetStats()
		assert.Equal(t, 0, stats.Entries)
	})
	
	t.Run("DefaultTTL", func(t *testing.T) {
		ctx := context.Background()
		
		result, err := cache.GetOrCompute(ctx, "default-ttl-key", func() (interface{}, error) {
			return "value", nil
		}, 0) // Zero TTL should use default
		
		require.NoError(t, err)
		assert.Equal(t, "value", result)
		
		stats := cache.GetStats()
		assert.Equal(t, 1, stats.Entries)
	})
}

func TestCacheCleanupRoutine(t *testing.T) {
	// Very short cleanup interval for testing
	cache := &ResponseCache{
		entries:         make(map[string]*CacheEntry),
		defaultTTL:      100 * time.Millisecond,
		maxEntries:      10,
		cleanupInterval: 50 * time.Millisecond,
		stopCleanup:     make(chan struct{}),
	}
	go cache.cleanupRoutine()
	defer cache.Close()
	
	containers := []types.Container{
		{ID: "container1", Names: []string{"/test1"}, State: "running"},
	}
	
	t.Run("AutomaticCleanup", func(t *testing.T) {
		// Add entry with short TTL
		cache.SetContainerList("cleanup-test", containers, 75*time.Millisecond)
		
		// Should exist initially
		_, found := cache.GetContainerList("cleanup-test")
		assert.True(t, found)
		
		// Wait for cleanup routine to run (should run at 50ms, 100ms, etc.)
		time.Sleep(150 * time.Millisecond)
		
		// Should be cleaned up
		_, found = cache.GetContainerList("cleanup-test")
		assert.False(t, found)
	})
}

func TestConcurrentCacheAccess(t *testing.T) {
	cache := NewResponseCache(5*time.Second, 100)
	defer cache.Close()
	
	containers := []types.Container{
		{ID: "container1", Names: []string{"/test1"}, State: "running"},
	}
	
	t.Run("ConcurrentSetAndGet", func(t *testing.T) {
		var wg sync.WaitGroup
		numGoroutines := 10
		numOperations := 100
		
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					key := fmt.Sprintf("concurrent-%d-%d", goroutineID, j)
					
					// Set value
					cache.SetContainerList(key, containers, 5*time.Second)
					
					// Get value
					result, found := cache.GetContainerList(key)
					assert.True(t, found)
					assert.Len(t, result, 1)
				}
			}(i)
		}
		
		wg.Wait()
		
		stats := cache.GetStats()
		assert.Greater(t, stats.Entries, 0)
		assert.Greater(t, stats.Hits, int64(0))
	})
	
	t.Run("ConcurrentInvalidation", func(t *testing.T) {
		// Populate cache
		for i := 0; i < 50; i++ {
			key := fmt.Sprintf("invalidate-test-%d", i)
			cache.SetContainerList(key, containers, 5*time.Second)
		}
		
		var wg sync.WaitGroup
		
		// Multiple goroutines invalidating
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				cache.InvalidateAll()
			}()
		}
		
		// Multiple goroutines accessing
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				key := fmt.Sprintf("invalidate-test-%d", id)
				_, _ = cache.GetContainerList(key)
			}(i)
		}
		
		wg.Wait()
		
		// Should not crash and eventually be empty
		stats := cache.GetStats()
		assert.GreaterOrEqual(t, stats.Entries, 0)
	})
}

// Benchmark tests
func BenchmarkCacheSet(b *testing.B) {
	cache := NewResponseCache(5*time.Second, 1000)
	defer cache.Close()
	
	containers := []types.Container{
		{ID: "container1", Names: []string{"/test1"}, State: "running"},
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-key-%d", i)
		cache.SetContainerList(key, containers, 5*time.Second)
	}
}

func BenchmarkCacheGet(b *testing.B) {
	cache := NewResponseCache(5*time.Second, 1000)
	defer cache.Close()
	
	containers := []types.Container{
		{ID: "container1", Names: []string{"/test1"}, State: "running"},
	}
	
	// Pre-populate cache
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("bench-key-%d", i)
		cache.SetContainerList(key, containers, 5*time.Second)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-key-%d", i%100)
		_, _ = cache.GetContainerList(key)
	}
}

func BenchmarkConcurrentCacheAccess(b *testing.B) {
	cache := NewResponseCache(5*time.Second, 1000)
	defer cache.Close()
	
	containers := []types.Container{
		{ID: "container1", Names: []string{"/test1"}, State: "running"},
	}
	
	// Pre-populate
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("bench-key-%d", i)
		cache.SetContainerList(key, containers, 5*time.Second)
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench-key-%d", i%100)
			_, _ = cache.GetContainerList(key)
			i++
		}
	})
}

