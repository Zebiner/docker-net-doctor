package docker

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientMetricsDetailed(t *testing.T) {
	metrics := NewClientMetrics()
	
	t.Run("InitialState", func(t *testing.T) {
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, int64(0), snapshot.TotalAPICalls)
		assert.Equal(t, int64(0), snapshot.SuccessfulCalls)
		assert.Equal(t, int64(0), snapshot.FailedCalls)
		assert.Equal(t, 0.0, snapshot.SuccessRate)
		assert.Equal(t, int32(0), snapshot.ActiveConnections)
		assert.Equal(t, int32(0), snapshot.PeakConnections)
		assert.Equal(t, int64(0), snapshot.CacheHits)
		assert.Equal(t, int64(0), snapshot.CacheMisses)
		assert.Equal(t, 0.0, snapshot.CacheHitRate)
	})
	
	t.Run("RecordAPICall", func(t *testing.T) {
		// Record successful call
		metrics.RecordAPICall("test_op", 100*time.Millisecond, nil)
		
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, int64(1), snapshot.TotalAPICalls)
		assert.Equal(t, int64(1), snapshot.SuccessfulCalls)
		assert.Equal(t, int64(0), snapshot.FailedCalls)
		assert.Equal(t, 1.0, snapshot.SuccessRate)
		
		// Record failed call
		testErr := errors.New("test error")
		metrics.RecordAPICall("test_op", 200*time.Millisecond, testErr)
		
		snapshot = metrics.GetSnapshot()
		assert.Equal(t, int64(2), snapshot.TotalAPICalls)
		assert.Equal(t, int64(1), snapshot.SuccessfulCalls)
		assert.Equal(t, int64(1), snapshot.FailedCalls)
		assert.Equal(t, 0.5, snapshot.SuccessRate)
		
		// Check operation durations
		opStats := snapshot.OperationDurations["test_op"]
		assert.Equal(t, int64(2), opStats.Count)
		assert.Greater(t, opStats.Average, time.Duration(0))
		assert.Equal(t, 100*time.Millisecond, opStats.Min)
		assert.Equal(t, 200*time.Millisecond, opStats.Max)
		
		// Check recent errors
		require.Len(t, snapshot.RecentErrors, 1)
		assert.Equal(t, "test_op", snapshot.RecentErrors[0].Operation)
		assert.Equal(t, "test error", snapshot.RecentErrors[0].Error)
		assert.True(t, snapshot.RecentErrors[0].Retryable)
	})
	
	t.Run("OperationSpecificCounters", func(t *testing.T) {
		metrics.Reset()
		
		// Test container_list
		metrics.RecordAPICall("container_list", 50*time.Millisecond, nil)
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, int64(1), snapshot.ContainerListCalls)
		
		// Test network_list
		metrics.RecordAPICall("network_list", 75*time.Millisecond, nil)
		snapshot = metrics.GetSnapshot()
		assert.Equal(t, int64(1), snapshot.NetworkListCalls)
		
		// Test container_inspect
		metrics.RecordAPICall("container_inspect", 25*time.Millisecond, nil)
		snapshot = metrics.GetSnapshot()
		assert.Equal(t, int64(1), snapshot.ContainerInspectCalls)
		
		// Test network_inspect
		metrics.RecordAPICall("network_inspect", 30*time.Millisecond, nil)
		snapshot = metrics.GetSnapshot()
		assert.Equal(t, int64(1), snapshot.NetworkInspectCalls)
		
		// Test container_exec
		metrics.RecordAPICall("container_exec", 100*time.Millisecond, nil)
		snapshot = metrics.GetSnapshot()
		assert.Equal(t, int64(1), snapshot.ExecCalls)
		
		// Test ping
		metrics.RecordAPICall("ping", 10*time.Millisecond, nil)
		snapshot = metrics.GetSnapshot()
		assert.Equal(t, int64(1), snapshot.PingCalls)
		
		// Test unknown operation (shouldn't increment specific counters)
		metrics.RecordAPICall("unknown_op", 5*time.Millisecond, nil)
		snapshot = metrics.GetSnapshot()
		assert.Equal(t, int64(7), snapshot.TotalAPICalls) // Should still increment total
		
		// Verify specific counters didn't change
		assert.Equal(t, int64(1), snapshot.ContainerListCalls)
		assert.Equal(t, int64(1), snapshot.NetworkListCalls)
	})
	
	t.Run("ConnectionTracking", func(t *testing.T) {
		metrics.Reset()
		
		metrics.UpdateActiveConnections(1)
		metrics.UpdateActiveConnections(1)
		metrics.UpdateActiveConnections(1)
		
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, int32(3), snapshot.ActiveConnections)
		assert.Equal(t, int32(3), snapshot.PeakConnections)
		
		metrics.UpdateActiveConnections(-2)
		snapshot = metrics.GetSnapshot()
		assert.Equal(t, int32(1), snapshot.ActiveConnections)
		assert.Equal(t, int32(3), snapshot.PeakConnections) // Should remain at peak
	})
	
	t.Run("CacheMetrics", func(t *testing.T) {
		metrics.Reset()
		
		metrics.RecordCacheHit()
		metrics.RecordCacheHit()
		metrics.RecordCacheMiss()
		
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, int64(2), snapshot.CacheHits)
		assert.Equal(t, int64(1), snapshot.CacheMisses)
		assert.InDelta(t, 0.6667, snapshot.CacheHitRate, 0.01)
	})
	
	t.Run("RetryAndRateLimitMetrics", func(t *testing.T) {
		metrics.Reset()
		
		metrics.RecordRetry()
		metrics.RecordRetry()
		metrics.RecordRateLimitHit()
		
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, int64(2), snapshot.RetryCount)
		assert.Equal(t, int64(1), snapshot.RateLimitHits)
	})
	
	t.Run("Reset", func(t *testing.T) {
		// Add some data
		metrics.RecordAPICall("test", 100*time.Millisecond, nil)
		metrics.UpdateActiveConnections(5)
		metrics.RecordCacheHit()
		
		// Reset
		metrics.Reset()
		
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, int64(0), snapshot.TotalAPICalls)
		assert.Equal(t, int64(0), snapshot.SuccessfulCalls)
		assert.Equal(t, int64(0), snapshot.FailedCalls)
		assert.Equal(t, int32(0), snapshot.ActiveConnections)
		assert.Equal(t, int32(0), snapshot.PeakConnections)
		assert.Equal(t, int64(0), snapshot.CacheHits)
		assert.Equal(t, int64(0), snapshot.CacheMisses)
		assert.Empty(t, snapshot.RecentErrors)
	})
	
	t.Run("ConcurrentAccess", func(t *testing.T) {
		metrics.Reset()
		
		var wg sync.WaitGroup
		numGoroutines := 10
		numOperations := 100
		
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					metrics.RecordAPICall("concurrent_test", 1*time.Millisecond, nil)
					metrics.RecordCacheHit()
					metrics.UpdateActiveConnections(1)
					metrics.UpdateActiveConnections(-1)
				}
			}()
		}
		
		wg.Wait()
		
		snapshot := metrics.GetSnapshot()
		expected := int64(numGoroutines * numOperations)
		assert.Equal(t, expected, snapshot.TotalAPICalls)
		assert.Equal(t, expected, snapshot.SuccessfulCalls)
		assert.Equal(t, expected, snapshot.CacheHits)
	})
}

func TestDurationTracker(t *testing.T) {
	tracker := NewDurationTracker()
	
	t.Run("EmptyTracker", func(t *testing.T) {
		stats := tracker.GetStats()
		assert.Equal(t, int64(0), stats.Count)
		assert.Equal(t, time.Duration(0), stats.Total)
		assert.Equal(t, time.Duration(0), stats.Min)
		assert.Equal(t, time.Duration(0), stats.Max)
		assert.Equal(t, time.Duration(0), stats.Average)
	})
	
	t.Run("SingleDuration", func(t *testing.T) {
		tracker.Record(100 * time.Millisecond)
		
		stats := tracker.GetStats()
		assert.Equal(t, int64(1), stats.Count)
		assert.Equal(t, 100*time.Millisecond, stats.Total)
		assert.Equal(t, 100*time.Millisecond, stats.Min)
		assert.Equal(t, 100*time.Millisecond, stats.Max)
		assert.Equal(t, 100*time.Millisecond, stats.Average)
	})
	
	t.Run("MultipleDurations", func(t *testing.T) {
		tracker.Reset()
		
		durations := []time.Duration{
			50 * time.Millisecond,
			100 * time.Millisecond,
			150 * time.Millisecond,
			75 * time.Millisecond,
			125 * time.Millisecond,
		}
		
		for _, d := range durations {
			tracker.Record(d)
		}
		
		stats := tracker.GetStats()
		assert.Equal(t, int64(5), stats.Count)
		assert.Equal(t, 50*time.Millisecond, stats.Min)
		assert.Equal(t, 150*time.Millisecond, stats.Max)
		
		expectedTotal := time.Duration(0)
		for _, d := range durations {
			expectedTotal += d
		}
		assert.Equal(t, expectedTotal, stats.Total)
		assert.Equal(t, expectedTotal/time.Duration(len(durations)), stats.Average)
		
		// Check percentiles are set (exact values depend on sorting algorithm)
		assert.Greater(t, stats.P50, time.Duration(0))
		assert.Greater(t, stats.P95, time.Duration(0))
		assert.Greater(t, stats.P99, time.Duration(0))
	})
	
	t.Run("Reset", func(t *testing.T) {
		tracker.Record(100 * time.Millisecond)
		tracker.Reset()
		
		stats := tracker.GetStats()
		assert.Equal(t, int64(0), stats.Count)
		assert.Equal(t, time.Duration(0), stats.Total)
	})
	
	t.Run("LargeSampleSet", func(t *testing.T) {
		tracker.Reset()
		
		// Add more than 1000 samples to test sample limiting
		for i := 0; i < 1500; i++ {
			tracker.Record(time.Duration(i) * time.Millisecond)
		}
		
		stats := tracker.GetStats()
		assert.Equal(t, int64(1500), stats.Count)
		assert.Equal(t, time.Duration(0), stats.Min)
		assert.Equal(t, 1499*time.Millisecond, stats.Max)
		
		// Percentiles should be calculated from the last 1000 samples
		assert.Greater(t, stats.P50, time.Duration(0))
		assert.Greater(t, stats.P95, stats.P50)
		assert.Greater(t, stats.P99, stats.P95)
	})
}

func TestErrorRecording(t *testing.T) {
	metrics := NewClientMetrics()
	
	t.Run("RetryableErrors", func(t *testing.T) {
		retryableErrors := []string{
			"connection refused",
			"connection reset",
			"timeout",
			"EOF",
			"broken pipe",
			"service unavailable",
			"too many requests",
		}
		
		for _, errMsg := range retryableErrors {
			err := errors.New(errMsg)
			metrics.RecordAPICall("test", 1*time.Millisecond, err)
		}
		
		snapshot := metrics.GetSnapshot()
		assert.Len(t, snapshot.RecentErrors, len(retryableErrors))
		
		for _, errRecord := range snapshot.RecentErrors {
			assert.True(t, errRecord.Retryable, "Error should be retryable: %s", errRecord.Error)
		}
	})
	
	t.Run("NonRetryableErrors", func(t *testing.T) {
		metrics.Reset()
		
		nonRetryableErrors := []string{
			"permission denied",
			"authentication failed",
			"invalid parameter",
			"not found",
		}
		
		for _, errMsg := range nonRetryableErrors {
			err := errors.New(errMsg)
			metrics.RecordAPICall("test", 1*time.Millisecond, err)
		}
		
		snapshot := metrics.GetSnapshot()
		assert.Len(t, snapshot.RecentErrors, len(nonRetryableErrors))
		
		for _, errRecord := range snapshot.RecentErrors {
			assert.False(t, errRecord.Retryable, "Error should not be retryable: %s", errRecord.Error)
		}
	})
	
	t.Run("ErrorLimit", func(t *testing.T) {
		metrics.Reset()
		
		// Record more than 100 errors (the limit)
		for i := 0; i < 150; i++ {
			err := errors.New("test error")
			metrics.RecordAPICall("test", 1*time.Millisecond, err)
		}
		
		snapshot := metrics.GetSnapshot()
		assert.Len(t, snapshot.RecentErrors, 100, "Should maintain only last 100 errors")
	})
}

// Benchmark tests
func BenchmarkMetricsRecordAPICall(b *testing.B) {
	metrics := NewClientMetrics()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.RecordAPICall("test", 1*time.Millisecond, nil)
	}
}

func BenchmarkMetricsGetSnapshot(b *testing.B) {
	metrics := NewClientMetrics()
	
	// Populate with some data
	for i := 0; i < 1000; i++ {
		metrics.RecordAPICall("test", 1*time.Millisecond, nil)
		metrics.RecordCacheHit()
		metrics.UpdateActiveConnections(1)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = metrics.GetSnapshot()
	}
}

func BenchmarkDurationTrackerRecord(b *testing.B) {
	tracker := NewDurationTracker()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.Record(time.Duration(i) * time.Millisecond)
	}
}

func BenchmarkConcurrentMetricsAccess(b *testing.B) {
	metrics := NewClientMetrics()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			metrics.RecordAPICall("concurrent", 1*time.Millisecond, nil)
		}
	})
}