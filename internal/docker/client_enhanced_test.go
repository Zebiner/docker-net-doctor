package docker

import (
	"context"
	"errors"
	"testing"
	"time"
	
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnhancedClient(t *testing.T) {
	// Skip if Docker is not available
	ctx := context.Background()
	config := &EnhancedClientConfig{
		RateLimit:      10.0,
		RateBurst:      20,
		CacheTTL:       10 * time.Second,
		MaxCacheEntries: 50,
		MaxConnections: 5,
		Timeout:        10 * time.Second,
		MaxRetries:     2,
		RetryBackoff:   50 * time.Millisecond,
	}
	
	client, err := NewEnhancedClient(ctx, config)
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer client.Close()
	
	t.Run("ListContainers", func(t *testing.T) {
		// First call should miss cache
		containers, err := client.ListContainers(ctx)
		require.NoError(t, err)
		assert.NotNil(t, containers)
		
		// Second call should hit cache
		containers2, err := client.ListContainers(ctx)
		require.NoError(t, err)
		assert.Equal(t, len(containers), len(containers2))
		
		// Check cache stats
		stats := client.GetCacheStats()
		assert.Greater(t, stats.Hits, int64(0))
	})
	
	t.Run("GetNetworkInfo", func(t *testing.T) {
		networks, err := client.GetNetworkInfo()
		require.NoError(t, err)
		assert.NotEmpty(t, networks)
		
		// Should have at least the bridge network
		found := false
		for _, net := range networks {
			if net.Name == "bridge" {
				found = true
				assert.Equal(t, "bridge", net.Driver)
				break
			}
		}
		assert.True(t, found, "bridge network not found")
	})
	
	t.Run("Ping", func(t *testing.T) {
		err := client.Ping(ctx)
		assert.NoError(t, err)
	})
	
	t.Run("Metrics", func(t *testing.T) {
		metrics := client.GetMetrics()
		assert.Greater(t, metrics.TotalAPICalls, int64(0))
		assert.Greater(t, metrics.SuccessfulCalls, int64(0))
		assert.GreaterOrEqual(t, metrics.SuccessRate, 0.0)
		assert.LessOrEqual(t, metrics.SuccessRate, 1.0)
	})
	
	t.Run("CacheInvalidation", func(t *testing.T) {
		// Get initial cache stats
		stats1 := client.GetCacheStats()
		
		// Invalidate cache
		client.InvalidateCache()
		
		// Cache should be empty
		stats2 := client.GetCacheStats()
		assert.Equal(t, 0, stats2.Entries)
		assert.GreaterOrEqual(t, stats2.Misses, stats1.Misses)
	})
}

func TestRetryPolicy(t *testing.T) {
	policy := NewDefaultRetryPolicy()
	
	t.Run("SuccessfulOperation", func(t *testing.T) {
		ctx := context.Background()
		calls := 0
		err := policy.Execute(ctx, func() error {
			calls++
			return nil
		})
		assert.NoError(t, err)
		assert.Equal(t, 1, calls)
	})
	
	t.Run("RetryableError", func(t *testing.T) {
		ctx := context.Background()
		calls := 0
		// Use a retryable error message
		retryableErr := errors.New("connection refused")
		
		err := policy.Execute(ctx, func() error {
			calls++
			if calls < 3 {
				return retryableErr
			}
			return nil
		})
		assert.NoError(t, err)
		assert.Equal(t, 3, calls)
	})
	
	t.Run("NonRetryableError", func(t *testing.T) {
		ctx := context.Background()
		calls := 0
		// Use a non-retryable error
		nonRetryableErr := errors.New("permission denied")
		
		err := policy.Execute(ctx, func() error {
			calls++
			return nonRetryableErr
		})
		assert.Error(t, err)
		assert.Equal(t, 1, calls) // Should not retry
	})
	
	t.Run("MaxRetries", func(t *testing.T) {
		ctx := context.Background()
		calls := 0
		policy.MaxRetries = 2
		// Use a retryable error
		retryableErr := errors.New("timeout")
		
		err := policy.Execute(ctx, func() error {
			calls++
			return retryableErr
		})
		assert.Error(t, err)
		assert.Equal(t, 3, calls) // Initial + 2 retries
	})
}

func TestResponseCache(t *testing.T) {
	cache := NewResponseCache(5*time.Second, 10)
	defer cache.Close()
	
	t.Run("ContainerListCache", func(t *testing.T) {
		// Set a value - SetContainerList expects actual container types
		cache.SetContainerList("test", nil, 5*time.Second)
		
		// Get should return it
		_, found := cache.GetContainerList("test")
		assert.True(t, found)
		
		// Different key should miss
		_, found = cache.GetContainerList("other")
		assert.False(t, found)
	})
	
	t.Run("CacheExpiration", func(t *testing.T) {
		// Set with short TTL
		cache.SetContainerList("expire", nil, 100*time.Millisecond)
		
		// Should exist immediately
		_, found := cache.GetContainerList("expire")
		assert.True(t, found)
		
		// Wait for expiration
		time.Sleep(150 * time.Millisecond)
		
		// Should be expired
		_, found = cache.GetContainerList("expire")
		assert.False(t, found)
	})
	
	t.Run("CacheStats", func(t *testing.T) {
		stats := cache.GetStats()
		assert.GreaterOrEqual(t, stats.Hits, int64(0))
		assert.GreaterOrEqual(t, stats.Misses, int64(0))
		assert.GreaterOrEqual(t, stats.HitRate, 0.0)
		assert.LessOrEqual(t, stats.HitRate, 1.0)
	})
	
	t.Run("GetOrCompute", func(t *testing.T) {
		ctx := context.Background()
		computeCalls := 0
		
		// First call should compute
		result1, err := cache.GetOrCompute(ctx, "compute_test", func() (interface{}, error) {
			computeCalls++
			return "computed_value", nil
		}, 5*time.Second)
		assert.NoError(t, err)
		assert.Equal(t, "computed_value", result1)
		assert.Equal(t, 1, computeCalls)
		
		// Second call should hit cache
		result2, err := cache.GetOrCompute(ctx, "compute_test", func() (interface{}, error) {
			computeCalls++
			return "computed_value", nil
		}, 5*time.Second)
		assert.NoError(t, err)
		assert.Equal(t, "computed_value", result2)
		assert.Equal(t, 1, computeCalls) // Should not have called compute again
	})
}

func TestClientMetrics(t *testing.T) {
	metrics := NewClientMetrics()
	
	t.Run("RecordAPICall", func(t *testing.T) {
		// Record successful call
		metrics.RecordAPICall("test_op", 100*time.Millisecond, nil)
		
		// Record failed call with error
		testErr := errors.New("test error")
		metrics.RecordAPICall("test_op", 200*time.Millisecond, testErr)
		
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, int64(2), snapshot.TotalAPICalls)
		assert.Equal(t, int64(1), snapshot.SuccessfulCalls)
		assert.Equal(t, int64(1), snapshot.FailedCalls)
		assert.Equal(t, 0.5, snapshot.SuccessRate)
	})
	
	t.Run("ConnectionTracking", func(t *testing.T) {
		metrics.UpdateActiveConnections(1)
		metrics.UpdateActiveConnections(1)
		metrics.UpdateActiveConnections(-1)
		
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, int32(1), snapshot.ActiveConnections)
		assert.Equal(t, int32(2), snapshot.PeakConnections)
	})
	
	t.Run("CacheMetrics", func(t *testing.T) {
		metrics.RecordCacheHit()
		metrics.RecordCacheHit()
		metrics.RecordCacheMiss()
		
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, int64(2), snapshot.CacheHits)
		assert.Equal(t, int64(1), snapshot.CacheMisses)
		assert.InDelta(t, 0.666, snapshot.CacheHitRate, 0.01)
	})
	
	t.Run("OperationDurations", func(t *testing.T) {
		// Record some operations
		metrics.RecordAPICall("container_list", 50*time.Millisecond, nil)
		metrics.RecordAPICall("container_list", 100*time.Millisecond, nil)
		metrics.RecordAPICall("container_list", 75*time.Millisecond, nil)
		
		snapshot := metrics.GetSnapshot()
		opStats := snapshot.OperationDurations["container_list"]
		assert.Equal(t, int64(3), opStats.Count)
		assert.Greater(t, opStats.Average, time.Duration(0))
		assert.LessOrEqual(t, opStats.Min, opStats.Max)
	})
	
	t.Run("Reset", func(t *testing.T) {
		metrics.Reset()
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, int64(0), snapshot.TotalAPICalls)
		assert.Equal(t, int64(0), snapshot.SuccessfulCalls)
		assert.Equal(t, int64(0), snapshot.FailedCalls)
	})
}

func TestLegacyClientWithEnhanced(t *testing.T) {
	ctx := context.Background()
	
	// Create legacy client with enhanced mode
	opts := &ClientOptions{
		UseEnhanced: true,
		EnhancedConfig: &EnhancedClientConfig{
			RateLimit:      10.0,
			RateBurst:      20,
			CacheTTL:       10 * time.Second,
			MaxCacheEntries: 50,
		},
	}
	
	client, err := NewClientWithOptions(ctx, opts)
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer client.Close()
	
	// Should be using enhanced client
	assert.True(t, client.IsEnhanced())
	
	// Should have metrics available
	metrics := client.GetMetrics()
	assert.NotNil(t, metrics)
	
	// Should have cache stats available
	cacheStats := client.GetCacheStats()
	assert.NotNil(t, cacheStats)
}

// Benchmark tests
func BenchmarkEnhancedClientWithCache(b *testing.B) {
	ctx := context.Background()
	config := &EnhancedClientConfig{
		RateLimit:      100.0,
		RateBurst:      200,
		CacheTTL:       30 * time.Second,
		MaxCacheEntries: 100,
	}
	
	client, err := NewEnhancedClient(ctx, config)
	if err != nil {
		b.Skipf("Docker not available: %v", err)
	}
	defer client.Close()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.ListContainers(ctx)
	}
	
	// Report cache stats
	stats := client.GetCacheStats()
	b.Logf("Cache hit rate: %.2f%%", stats.HitRate*100)
}

func BenchmarkLegacyClient(b *testing.B) {
	ctx := context.Background()
	
	client, err := NewClient(ctx)
	if err != nil {
		b.Skipf("Docker not available: %v", err)
	}
	defer client.Close()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.ListContainers(ctx)
	}
}