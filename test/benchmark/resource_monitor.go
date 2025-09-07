// test/benchmark/resource_monitor.go
package benchmark

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

// ResourceMonitor tracks system resource usage during benchmarks
type ResourceMonitor struct {
	mu              sync.Mutex
	startTime       time.Time
	samples         []ResourceSample
	stopChan        chan bool
	samplingPeriod  time.Duration
}

// ResourceSample represents a point-in-time resource measurement
type ResourceSample struct {
	Timestamp       time.Time
	MemoryAlloc     uint64 // bytes allocated and still in use
	MemoryTotalAlloc uint64 // bytes allocated (even if freed)
	MemorySys       uint64 // bytes obtained from system
	NumGoroutines   int
	NumGC           uint32
	CPUUsage        float64 // percentage (requires external measurement)
}

// NewResourceMonitor creates a new resource monitor
func NewResourceMonitor(samplingPeriod time.Duration) *ResourceMonitor {
	return &ResourceMonitor{
		samplingPeriod: samplingPeriod,
		samples:        make([]ResourceSample, 0, 1000),
		stopChan:       make(chan bool),
	}
}

// Start begins monitoring resources
func (m *ResourceMonitor) Start() {
	m.mu.Lock()
	m.startTime = time.Now()
	m.mu.Unlock()
	
	go m.monitorLoop()
}

// Stop stops monitoring and returns collected samples
func (m *ResourceMonitor) Stop() []ResourceSample {
	close(m.stopChan)
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	return m.samples
}

// monitorLoop continuously samples resource usage
func (m *ResourceMonitor) monitorLoop() {
	ticker := time.NewTicker(m.samplingPeriod)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.takeSample()
		}
	}
}

// takeSample captures current resource usage
func (m *ResourceMonitor) takeSample() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	sample := ResourceSample{
		Timestamp:        time.Now(),
		MemoryAlloc:      memStats.Alloc,
		MemoryTotalAlloc: memStats.TotalAlloc,
		MemorySys:        memStats.Sys,
		NumGoroutines:    runtime.NumGoroutine(),
		NumGC:            memStats.NumGC,
	}
	
	m.mu.Lock()
	m.samples = append(m.samples, sample)
	m.mu.Unlock()
}

// GetStatistics calculates statistics from collected samples
func (m *ResourceMonitor) GetStatistics() ResourceStatistics {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if len(m.samples) == 0 {
		return ResourceStatistics{}
	}
	
	stats := ResourceStatistics{
		Duration:    time.Since(m.startTime),
		NumSamples:  len(m.samples),
	}
	
	// Calculate memory statistics
	var totalMemAlloc, maxMemAlloc uint64
	var totalGoroutines, maxGoroutines int
	
	for _, sample := range m.samples {
		if sample.MemoryAlloc > maxMemAlloc {
			maxMemAlloc = sample.MemoryAlloc
		}
		totalMemAlloc += sample.MemoryAlloc
		
		if sample.NumGoroutines > maxGoroutines {
			maxGoroutines = sample.NumGoroutines
		}
		totalGoroutines += sample.NumGoroutines
	}
	
	stats.AvgMemoryAlloc = totalMemAlloc / uint64(len(m.samples))
	stats.MaxMemoryAlloc = maxMemAlloc
	stats.AvgGoroutines = float64(totalGoroutines) / float64(len(m.samples))
	stats.MaxGoroutines = maxGoroutines
	
	// Calculate GC statistics
	if len(m.samples) > 1 {
		stats.GCRuns = m.samples[len(m.samples)-1].NumGC - m.samples[0].NumGC
	}
	
	return stats
}

// ResourceStatistics holds aggregated resource usage statistics
type ResourceStatistics struct {
	Duration       time.Duration
	NumSamples     int
	AvgMemoryAlloc uint64
	MaxMemoryAlloc uint64
	AvgGoroutines  float64
	MaxGoroutines  int
	GCRuns         uint32
}

// BenchmarkWithResourceMonitoring runs a benchmark with resource monitoring
func BenchmarkWithResourceMonitoring(b *testing.B, name string, fn func()) {
	// Create monitor with 10ms sampling period
	monitor := NewResourceMonitor(10 * time.Millisecond)
	
	// Start monitoring
	monitor.Start()
	
	// Run the benchmark function
	b.ResetTimer()
	fn()
	b.StopTimer()
	
	// Stop monitoring and get statistics
	monitor.Stop()
	stats := monitor.GetStatistics()
	
	// Report resource metrics
	b.ReportMetric(float64(stats.AvgMemoryAlloc)/1024/1024, "MB/avg")
	b.ReportMetric(float64(stats.MaxMemoryAlloc)/1024/1024, "MB/max")
	b.ReportMetric(stats.AvgGoroutines, "goroutines/avg")
	b.ReportMetric(float64(stats.MaxGoroutines), "goroutines/max")
	b.ReportMetric(float64(stats.GCRuns), "gc-runs")
	
	// Log detailed statistics
	b.Logf("\n=== Resource Usage for %s ===", name)
	b.Logf("Duration: %v", stats.Duration)
	b.Logf("Memory (avg): %.2f MB", float64(stats.AvgMemoryAlloc)/1024/1024)
	b.Logf("Memory (max): %.2f MB", float64(stats.MaxMemoryAlloc)/1024/1024)
	b.Logf("Goroutines (avg): %.1f", stats.AvgGoroutines)
	b.Logf("Goroutines (max): %d", stats.MaxGoroutines)
	b.Logf("GC Runs: %d", stats.GCRuns)
}

// MemoryProfiler provides detailed memory profiling
type MemoryProfiler struct {
	baseline    runtime.MemStats
	current     runtime.MemStats
	allocations map[string]uint64
	mu          sync.Mutex
}

// NewMemoryProfiler creates a new memory profiler
func NewMemoryProfiler() *MemoryProfiler {
	return &MemoryProfiler{
		allocations: make(map[string]uint64),
	}
}

// Start captures baseline memory statistics
func (p *MemoryProfiler) Start() {
	runtime.GC() // Force GC to get clean baseline
	runtime.ReadMemStats(&p.baseline)
}

// Mark records memory usage at a specific point
func (p *MemoryProfiler) Mark(label string) {
	runtime.ReadMemStats(&p.current)
	
	p.mu.Lock()
	p.allocations[label] = p.current.Alloc - p.baseline.Alloc
	p.mu.Unlock()
}

// GetReport generates a memory usage report
func (p *MemoryProfiler) GetReport() map[string]uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	report := make(map[string]uint64)
	for label, bytes := range p.allocations {
		report[label] = bytes
	}
	
	return report
}