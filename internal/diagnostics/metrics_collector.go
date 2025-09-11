// Package diagnostics provides real-time metrics collection
package diagnostics

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// MetricsCollector handles real-time collection of performance metrics
type MetricsCollector struct {
	profiler *PerformanceProfiler
	interval time.Duration
	stopChan chan struct{}
	wg       sync.WaitGroup
	running  atomic.Bool

	// Metrics collection state
	lastCPUTime  time.Time
	lastCPUUsage float64

	// Sampling data
	samples    []MetricsSample
	samplesMu  sync.RWMutex
	maxSamples int
}

// MetricsSample represents a point-in-time metrics sample
type MetricsSample struct {
	Timestamp     time.Time
	CPUPercent    float64
	MemoryMB      float64
	Goroutines    int
	ActiveJobs    int
	CompletedJobs int
	ErrorRate     float64
	AvgDuration   time.Duration
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(profiler *PerformanceProfiler, interval time.Duration) *MetricsCollector {
	if interval <= 0 {
		interval = SamplingInterval
	}

	return &MetricsCollector{
		profiler:   profiler,
		interval:   interval,
		stopChan:   make(chan struct{}),
		maxSamples: 1000, // Keep last 1000 samples
		samples:    make([]MetricsSample, 0, 1000),
	}
}

// Start begins metrics collection
func (mc *MetricsCollector) Start() {
	if !mc.running.CompareAndSwap(false, true) {
		return // Already running
	}

	mc.wg.Add(1)
	go mc.collect()
}

// Stop halts metrics collection
func (mc *MetricsCollector) Stop() {
	if !mc.running.CompareAndSwap(true, false) {
		return // Not running
	}

	close(mc.stopChan)
	mc.wg.Wait()
}

// collect is the main collection loop
func (mc *MetricsCollector) collect() {
	defer mc.wg.Done()

	ticker := time.NewTicker(mc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mc.collectSample()
		case <-mc.stopChan:
			return
		}
	}
}

// collectSample collects a single metrics sample
func (mc *MetricsCollector) collectSample() {
	sample := MetricsSample{
		Timestamp:  time.Now(),
		CPUPercent: mc.collectCPUUsage(),
		MemoryMB:   mc.collectMemoryUsage(),
		Goroutines: runtime.NumGoroutine(),
	}

	// Get profiler metrics if available
	if mc.profiler != nil {
		metrics := mc.profiler.GetMetrics()
		sample.AvgDuration = metrics.AverageDuration

		// Calculate error rate from recent samples
		if metrics.TotalOperations > 0 {
			failedOps := float64(0) // This would come from failed operations tracking
			sample.ErrorRate = failedOps / float64(metrics.TotalOperations)
		}
	}

	// Get worker pool metrics if integrated
	if mc.profiler != nil && mc.profiler.workerPool != nil {
		sample.ActiveJobs = int(mc.profiler.workerPool.activeJobs.Load())
		sample.CompletedJobs = int(mc.profiler.workerPool.completedJobs.Load())
	}

	// Store sample
	mc.storeSample(sample)
}

// collectCPUUsage calculates current CPU usage
func (mc *MetricsCollector) collectCPUUsage() float64 {
	// Simple CPU usage calculation
	// In production, you'd use more sophisticated methods
	var rusage runtime.MemStats
	runtime.ReadMemStats(&rusage)

	now := time.Now()
	if !mc.lastCPUTime.IsZero() {
		elapsed := now.Sub(mc.lastCPUTime).Seconds()
		if elapsed > 0 {
			// Calculate approximate CPU usage
			// This is a simplified calculation
			cpuPercent := 0.0 // Placeholder
			mc.lastCPUUsage = cpuPercent
		}
	}

	mc.lastCPUTime = now
	return mc.lastCPUUsage
}

// collectMemoryUsage gets current memory usage in MB
func (mc *MetricsCollector) collectMemoryUsage() float64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.Alloc) / (1024 * 1024)
}

// storeSample stores a metrics sample
func (mc *MetricsCollector) storeSample(sample MetricsSample) {
	mc.samplesMu.Lock()
	defer mc.samplesMu.Unlock()

	// Implement circular buffer
	if len(mc.samples) >= mc.maxSamples {
		// Remove oldest sample
		mc.samples = mc.samples[1:]
	}

	mc.samples = append(mc.samples, sample)
}

// GetSamples returns recent metrics samples
func (mc *MetricsCollector) GetSamples(count int) []MetricsSample {
	mc.samplesMu.RLock()
	defer mc.samplesMu.RUnlock()

	if count <= 0 || count > len(mc.samples) {
		count = len(mc.samples)
	}

	// Return last 'count' samples
	start := len(mc.samples) - count
	if start < 0 {
		start = 0
	}

	result := make([]MetricsSample, count)
	copy(result, mc.samples[start:])
	return result
}

// GetLatestSample returns the most recent sample
func (mc *MetricsCollector) GetLatestSample() *MetricsSample {
	mc.samplesMu.RLock()
	defer mc.samplesMu.RUnlock()

	if len(mc.samples) == 0 {
		return nil
	}

	sample := mc.samples[len(mc.samples)-1]
	return &sample
}

// GetAverageMetrics returns average metrics over a time window
func (mc *MetricsCollector) GetAverageMetrics(window time.Duration) *AverageMetrics {
	mc.samplesMu.RLock()
	defer mc.samplesMu.RUnlock()

	if len(mc.samples) == 0 {
		return nil
	}

	cutoff := time.Now().Add(-window)
	var (
		count           int
		totalCPU        float64
		totalMemory     float64
		totalGoroutines int
		totalDuration   time.Duration
	)

	for _, sample := range mc.samples {
		if sample.Timestamp.After(cutoff) {
			count++
			totalCPU += sample.CPUPercent
			totalMemory += sample.MemoryMB
			totalGoroutines += sample.Goroutines
			totalDuration += sample.AvgDuration
		}
	}

	if count == 0 {
		return nil
	}

	return &AverageMetrics{
		Window:        window,
		SampleCount:   count,
		AvgCPU:        totalCPU / float64(count),
		AvgMemoryMB:   totalMemory / float64(count),
		AvgGoroutines: totalGoroutines / count,
		AvgDuration:   totalDuration / time.Duration(count),
	}
}

// GetTrend analyzes performance trends
func (mc *MetricsCollector) GetTrend(metric string, window time.Duration) *MetricTrend {
	mc.samplesMu.RLock()
	defer mc.samplesMu.RUnlock()

	if len(mc.samples) < 2 {
		return nil
	}

	cutoff := time.Now().Add(-window)
	var values []float64
	var timestamps []time.Time

	for _, sample := range mc.samples {
		if sample.Timestamp.After(cutoff) {
			var value float64
			switch metric {
			case "cpu":
				value = sample.CPUPercent
			case "memory":
				value = sample.MemoryMB
			case "goroutines":
				value = float64(sample.Goroutines)
			case "duration":
				value = float64(sample.AvgDuration.Microseconds())
			case "error_rate":
				value = sample.ErrorRate
			default:
				continue
			}
			values = append(values, value)
			timestamps = append(timestamps, sample.Timestamp)
		}
	}

	if len(values) < 2 {
		return nil
	}

	// Calculate trend (simple linear regression)
	trend := calculateLinearTrend(timestamps, values)

	return &MetricTrend{
		Metric:     metric,
		Window:     window,
		DataPoints: len(values),
		Slope:      trend.Slope,
		Direction:  trend.Direction,
		StartValue: values[0],
		EndValue:   values[len(values)-1],
		MinValue:   findMin(values),
		MaxValue:   findMax(values),
	}
}

// IsHealthy checks if metrics are within healthy thresholds
func (mc *MetricsCollector) IsHealthy() bool {
	latest := mc.GetLatestSample()
	if latest == nil {
		return true // No data, assume healthy
	}

	// Define health thresholds
	const (
		maxCPUPercent = 80.0
		maxMemoryMB   = 500.0
		maxGoroutines = 1000
		maxErrorRate  = 0.1
	)

	return latest.CPUPercent < maxCPUPercent &&
		latest.MemoryMB < maxMemoryMB &&
		latest.Goroutines < maxGoroutines &&
		latest.ErrorRate < maxErrorRate
}

// Reset clears all collected samples
func (mc *MetricsCollector) Reset() {
	mc.samplesMu.Lock()
	defer mc.samplesMu.Unlock()

	mc.samples = make([]MetricsSample, 0, mc.maxSamples)
	mc.lastCPUTime = time.Time{}
	mc.lastCPUUsage = 0
}

// AverageMetrics contains averaged metrics over a time window
type AverageMetrics struct {
	Window        time.Duration
	SampleCount   int
	AvgCPU        float64
	AvgMemoryMB   float64
	AvgGoroutines int
	AvgDuration   time.Duration
}

// MetricTrend represents a performance trend
type MetricTrend struct {
	Metric     string
	Window     time.Duration
	DataPoints int
	Slope      float64
	Direction  string // "increasing", "decreasing", "stable"
	StartValue float64
	EndValue   float64
	MinValue   float64
	MaxValue   float64
}

// TrendAnalysis contains trend direction and slope
type TrendAnalysis struct {
	Slope     float64
	Direction string
}

// calculateLinearTrend calculates a simple linear trend
func calculateLinearTrend(timestamps []time.Time, values []float64) TrendAnalysis {
	if len(timestamps) != len(values) || len(values) < 2 {
		return TrendAnalysis{Direction: "stable"}
	}

	// Convert timestamps to seconds from first timestamp
	startTime := timestamps[0]
	xValues := make([]float64, len(timestamps))
	for i, t := range timestamps {
		xValues[i] = t.Sub(startTime).Seconds()
	}

	// Calculate linear regression
	n := float64(len(values))
	var sumX, sumY, sumXY, sumX2 float64

	for i := range values {
		sumX += xValues[i]
		sumY += values[i]
		sumXY += xValues[i] * values[i]
		sumX2 += xValues[i] * xValues[i]
	}

	// Calculate slope
	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)

	// Determine direction
	direction := "stable"
	if slope > 0.01 {
		direction = "increasing"
	} else if slope < -0.01 {
		direction = "decreasing"
	}

	return TrendAnalysis{
		Slope:     slope,
		Direction: direction,
	}
}

// findMin finds the minimum value in a slice
func findMin(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

// findMax finds the maximum value in a slice
func findMax(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}
