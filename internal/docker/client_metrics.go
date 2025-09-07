// Package docker provides metrics collection for Docker client operations
package docker

import (
	"sync"
	"sync/atomic"
	"time"
)

// ClientMetrics tracks performance and usage metrics for Docker client operations
type ClientMetrics struct {
	mu sync.RWMutex
	
	// API call counters
	TotalAPICalls     atomic.Int64
	SuccessfulCalls   atomic.Int64
	FailedCalls       atomic.Int64
	RetryCount        atomic.Int64
	RateLimitHits     atomic.Int64
	
	// Operation-specific counters
	ContainerListCalls    atomic.Int64
	NetworkListCalls      atomic.Int64
	ContainerInspectCalls atomic.Int64
	NetworkInspectCalls   atomic.Int64
	ExecCalls            atomic.Int64
	PingCalls            atomic.Int64
	
	// Performance metrics
	operationDurations map[string]*DurationTracker
	
	// Resource usage
	ActiveConnections atomic.Int32
	PeakConnections   int32
	
	// Cache metrics
	CacheHits   atomic.Int64
	CacheMisses atomic.Int64
	
	// Error tracking
	lastErrors []ErrorRecord
	
	// Timing
	StartTime time.Time
}

// DurationTracker tracks duration statistics for an operation
type DurationTracker struct {
	mu       sync.RWMutex
	Count    int64
	Total    time.Duration
	Min      time.Duration
	Max      time.Duration
	Average  time.Duration
	P50      time.Duration // Median
	P95      time.Duration
	P99      time.Duration
	samples  []time.Duration
}

// ErrorRecord represents a recorded error
type ErrorRecord struct {
	Timestamp time.Time
	Operation string
	Error     string
	Retryable bool
}

// NewClientMetrics creates a new metrics instance
func NewClientMetrics() *ClientMetrics {
	return &ClientMetrics{
		operationDurations: map[string]*DurationTracker{
			"container_list":    NewDurationTracker(),
			"network_list":      NewDurationTracker(),
			"container_inspect": NewDurationTracker(),
			"network_inspect":   NewDurationTracker(),
			"container_exec":    NewDurationTracker(),
			"ping":             NewDurationTracker(),
		},
		lastErrors: make([]ErrorRecord, 0, 100),
		StartTime:  time.Now(),
	}
}

// NewDurationTracker creates a new duration tracker
func NewDurationTracker() *DurationTracker {
	return &DurationTracker{
		samples: make([]time.Duration, 0, 1000),
	}
}

// RecordAPICall records an API call
func (m *ClientMetrics) RecordAPICall(operation string, duration time.Duration, err error) {
	m.TotalAPICalls.Add(1)
	
	if err != nil {
		m.FailedCalls.Add(1)
		m.recordError(operation, err)
	} else {
		m.SuccessfulCalls.Add(1)
	}
	
	// Record duration
	m.recordDuration(operation, duration)
	
	// Update operation-specific counters
	switch operation {
	case "container_list":
		m.ContainerListCalls.Add(1)
	case "network_list":
		m.NetworkListCalls.Add(1)
	case "container_inspect":
		m.ContainerInspectCalls.Add(1)
	case "network_inspect":
		m.NetworkInspectCalls.Add(1)
	case "container_exec":
		m.ExecCalls.Add(1)
	case "ping":
		m.PingCalls.Add(1)
	}
}

// RecordRetry records a retry attempt
func (m *ClientMetrics) RecordRetry() {
	m.RetryCount.Add(1)
}

// RecordRateLimitHit records when rate limiting is triggered
func (m *ClientMetrics) RecordRateLimitHit() {
	m.RateLimitHits.Add(1)
}

// RecordCacheHit records a cache hit
func (m *ClientMetrics) RecordCacheHit() {
	m.CacheHits.Add(1)
}

// RecordCacheMiss records a cache miss
func (m *ClientMetrics) RecordCacheMiss() {
	m.CacheMisses.Add(1)
}

// UpdateActiveConnections updates the active connection count
func (m *ClientMetrics) UpdateActiveConnections(delta int32) {
	newValue := m.ActiveConnections.Add(delta)
	
	m.mu.Lock()
	if newValue > m.PeakConnections {
		m.PeakConnections = newValue
	}
	m.mu.Unlock()
}

// GetSnapshot returns a snapshot of current metrics
func (m *ClientMetrics) GetSnapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Calculate success rate
	total := m.TotalAPICalls.Load()
	successful := m.SuccessfulCalls.Load()
	var successRate float64
	if total > 0 {
		successRate = float64(successful) / float64(total)
	}
	
	// Calculate cache hit rate
	cacheTotal := m.CacheHits.Load() + m.CacheMisses.Load()
	var cacheHitRate float64
	if cacheTotal > 0 {
		cacheHitRate = float64(m.CacheHits.Load()) / float64(cacheTotal)
	}
	
	// Collect operation durations
	opDurations := make(map[string]OperationStats)
	for op, tracker := range m.operationDurations {
		opDurations[op] = tracker.GetStats()
	}
	
	// Copy recent errors
	recentErrors := make([]ErrorRecord, len(m.lastErrors))
	copy(recentErrors, m.lastErrors)
	
	return MetricsSnapshot{
		Timestamp:             time.Now(),
		Uptime:               time.Since(m.StartTime),
		TotalAPICalls:        total,
		SuccessfulCalls:      successful,
		FailedCalls:          m.FailedCalls.Load(),
		SuccessRate:          successRate,
		RetryCount:           m.RetryCount.Load(),
		RateLimitHits:        m.RateLimitHits.Load(),
		ContainerListCalls:   m.ContainerListCalls.Load(),
		NetworkListCalls:     m.NetworkListCalls.Load(),
		ContainerInspectCalls: m.ContainerInspectCalls.Load(),
		NetworkInspectCalls:  m.NetworkInspectCalls.Load(),
		ExecCalls:           m.ExecCalls.Load(),
		PingCalls:           m.PingCalls.Load(),
		ActiveConnections:    m.ActiveConnections.Load(),
		PeakConnections:     m.PeakConnections,
		CacheHits:           m.CacheHits.Load(),
		CacheMisses:         m.CacheMisses.Load(),
		CacheHitRate:        cacheHitRate,
		OperationDurations:  opDurations,
		RecentErrors:        recentErrors,
	}
}

// Reset resets all metrics
func (m *ClientMetrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.TotalAPICalls.Store(0)
	m.SuccessfulCalls.Store(0)
	m.FailedCalls.Store(0)
	m.RetryCount.Store(0)
	m.RateLimitHits.Store(0)
	m.ContainerListCalls.Store(0)
	m.NetworkListCalls.Store(0)
	m.ContainerInspectCalls.Store(0)
	m.NetworkInspectCalls.Store(0)
	m.ExecCalls.Store(0)
	m.PingCalls.Store(0)
	m.ActiveConnections.Store(0)
	m.PeakConnections = 0
	m.CacheHits.Store(0)
	m.CacheMisses.Store(0)
	
	// Reset duration trackers
	for _, tracker := range m.operationDurations {
		tracker.Reset()
	}
	
	m.lastErrors = make([]ErrorRecord, 0, 100)
	m.StartTime = time.Now()
}

// recordDuration records the duration of an operation
func (m *ClientMetrics) recordDuration(operation string, duration time.Duration) {
	if tracker, exists := m.operationDurations[operation]; exists {
		tracker.Record(duration)
	}
}

// recordError records an error
func (m *ClientMetrics) recordError(operation string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Keep only last 100 errors
	if len(m.lastErrors) >= 100 {
		m.lastErrors = m.lastErrors[1:]
	}
	
	m.lastErrors = append(m.lastErrors, ErrorRecord{
		Timestamp: time.Now(),
		Operation: operation,
		Error:     err.Error(),
		Retryable: isRetryableError(err),
	})
}

// Record records a duration sample
func (dt *DurationTracker) Record(duration time.Duration) {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	
	dt.Count++
	dt.Total += duration
	
	if dt.Min == 0 || duration < dt.Min {
		dt.Min = duration
	}
	if duration > dt.Max {
		dt.Max = duration
	}
	
	dt.Average = dt.Total / time.Duration(dt.Count)
	
	// Keep last 1000 samples for percentile calculation
	if len(dt.samples) >= 1000 {
		dt.samples = dt.samples[1:]
	}
	dt.samples = append(dt.samples, duration)
	
	// Update percentiles
	dt.updatePercentiles()
}

// updatePercentiles calculates percentiles from samples
func (dt *DurationTracker) updatePercentiles() {
	if len(dt.samples) == 0 {
		return
	}
	
	// Simple percentile calculation (not perfectly accurate but fast)
	sorted := make([]time.Duration, len(dt.samples))
	copy(sorted, dt.samples)
	
	// Simple bubble sort for small datasets
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	
	p50Index := len(sorted) * 50 / 100
	p95Index := len(sorted) * 95 / 100
	p99Index := len(sorted) * 99 / 100
	
	if p50Index < len(sorted) {
		dt.P50 = sorted[p50Index]
	}
	if p95Index < len(sorted) {
		dt.P95 = sorted[p95Index]
	}
	if p99Index < len(sorted) {
		dt.P99 = sorted[p99Index]
	}
}

// GetStats returns statistics for the duration tracker
func (dt *DurationTracker) GetStats() OperationStats {
	dt.mu.RLock()
	defer dt.mu.RUnlock()
	
	return OperationStats{
		Count:   dt.Count,
		Total:   dt.Total,
		Min:     dt.Min,
		Max:     dt.Max,
		Average: dt.Average,
		P50:     dt.P50,
		P95:     dt.P95,
		P99:     dt.P99,
	}
}

// Reset resets the duration tracker
func (dt *DurationTracker) Reset() {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	
	dt.Count = 0
	dt.Total = 0
	dt.Min = 0
	dt.Max = 0
	dt.Average = 0
	dt.P50 = 0
	dt.P95 = 0
	dt.P99 = 0
	dt.samples = make([]time.Duration, 0, 1000)
}

// MetricsSnapshot represents a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	Timestamp             time.Time
	Uptime               time.Duration
	TotalAPICalls        int64
	SuccessfulCalls      int64
	FailedCalls          int64
	SuccessRate          float64
	RetryCount           int64
	RateLimitHits        int64
	ContainerListCalls   int64
	NetworkListCalls     int64
	ContainerInspectCalls int64
	NetworkInspectCalls  int64
	ExecCalls           int64
	PingCalls           int64
	ActiveConnections    int32
	PeakConnections     int32
	CacheHits           int64
	CacheMisses         int64
	CacheHitRate        float64
	OperationDurations  map[string]OperationStats
	RecentErrors        []ErrorRecord
}

// OperationStats represents statistics for a specific operation
type OperationStats struct {
	Count   int64
	Total   time.Duration
	Min     time.Duration
	Max     time.Duration
	Average time.Duration
	P50     time.Duration
	P95     time.Duration
	P99     time.Duration
}

// isRetryableError checks if an error is retryable
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"EOF",
		"broken pipe",
		"service unavailable",
		"too many requests",
	}
	
	errMsg := err.Error()
	for _, pattern := range retryablePatterns {
		if contains(errMsg, pattern) {
			return true
		}
	}
	
	return false
}