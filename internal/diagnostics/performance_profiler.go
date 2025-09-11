// Package diagnostics provides high-precision performance profiling with 1ms accuracy
package diagnostics

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/diagnostics/profiling"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// ProfilingAccuracy defines the target precision for timing measurements
const (
	ProfilingAccuracy      = 1 * time.Millisecond   // 1ms target accuracy
	ProfilingOverheadLimit = 0.05                   // 5% maximum overhead
	SamplingInterval       = 100 * time.Microsecond // High-frequency sampling
	MaxProfileDataPoints   = 10000                  // Limit memory usage
)

// PerformanceProfiler provides high-precision performance profiling with 1ms accuracy
type PerformanceProfiler struct {
	// Core components
	precision    time.Duration     // Target precision (1ms)
	measurements *TimingStorage    // Thread-safe timing storage
	metrics      *ProfileMetrics   // Aggregated statistics
	collector    *MetricsCollector // Real-time collection
	reporter     *ProfileReporter  // Output formatting

	// Integration points
	workerPool   *SecureWorkerPool // Worker pool integration
	dockerClient *docker.Client    // Docker client integration
	engine       *DiagnosticEngine // Engine integration

	// State management
	enabled  atomic.Bool // Profiling enabled flag
	started  atomic.Bool // Profiler started flag
	overhead int64       // Overhead in nanoseconds (use atomic ops)

	// Configuration
	config *ProfileConfig // Profiler configuration
	mu     sync.RWMutex   // Protects configuration
}

// ProfileConfig defines profiler configuration
type ProfileConfig struct {
	Enabled           bool                 // Enable profiling
	Precision         time.Duration        // Target precision (default 1ms)
	MaxOverhead       float64              // Maximum allowed overhead (default 5%)
	SamplingRate      time.Duration        // Sampling interval
	MaxDataPoints     int                  // Maximum stored data points
	EnableFlameGraph  bool                 // Generate flame graphs
	EnablePercentiles bool                 // Calculate percentiles
	EnableTrends      bool                 // Track performance trends
	RealtimeMetrics   bool                 // Real-time metrics collection
	DetailLevel       ProfilingDetailLevel // Level of detail to capture
}

// ProfilingDetailLevel defines the granularity of profiling
type ProfilingDetailLevel int

const (
	DetailLevelBasic    ProfilingDetailLevel = iota // Basic timing only
	DetailLevelNormal                               // Normal profiling
	DetailLevelDetailed                             // Detailed with call stacks
	DetailLevelFull                                 // Full profiling with traces
)

// TimingData represents a single timing measurement with high precision
type TimingData struct {
	Name          string                 // Operation name
	StartTime     time.Time              // Start timestamp
	EndTime       time.Time              // End timestamp
	Duration      time.Duration          // Measured duration
	Category      string                 // Operation category
	CheckName     string                 // Associated check name
	WorkerID      int                    // Worker that executed
	Success       bool                   // Operation success
	Metadata      map[string]interface{} // Additional metadata
	CallStack     []string               // Call stack (if detailed)
	ResourceUsage *ResourceSnapshot      // Resource usage at time
}

// ResourceSnapshot captures resource usage at a point in time
type ResourceSnapshot struct {
	CPUPercent   float64   // CPU usage percentage
	MemoryBytes  uint64    // Memory usage in bytes
	GoroutineNum int       // Number of goroutines
	ThreadNum    int       // Number of OS threads
	Timestamp    time.Time // Snapshot timestamp
}

// ProfileMetrics contains aggregated profiling statistics
type ProfileMetrics struct {
	mu               sync.RWMutex
	TotalOperations  int64                           // Total operations profiled
	TotalDuration    time.Duration                   // Total execution time
	AverageDuration  time.Duration                   // Average operation duration
	MinDuration      time.Duration                   // Minimum duration
	MaxDuration      time.Duration                   // Maximum duration
	Percentiles      map[float64]time.Duration       // P50, P95, P99
	CategoryMetrics  map[string]*CategoryMetrics     // Per-category metrics
	CheckMetrics     map[string]*CheckProfileMetrics // Per-check metrics
	WorkerMetrics    map[int]*WorkerProfileMetrics   // Per-worker metrics
	OverheadNanos    int64                           // Profiling overhead in nanoseconds
	AccuracyAchieved time.Duration                   // Actual accuracy achieved
}

// CategoryMetrics tracks metrics for operation categories
type CategoryMetrics struct {
	Count           int64
	TotalDuration   time.Duration
	AverageDuration time.Duration
	MinDuration     time.Duration
	MaxDuration     time.Duration
}

// CheckProfileMetrics tracks performance metrics for individual checks
type CheckProfileMetrics struct {
	Name            string
	ExecutionCount  int64
	TotalDuration   time.Duration
	AverageDuration time.Duration
	MinDuration     time.Duration
	MaxDuration     time.Duration
	P50Duration     time.Duration
	P95Duration     time.Duration
	P99Duration     time.Duration
	SuccessRate     float64
	LastExecution   time.Time
	ResourceUsage   *ResourceSnapshot
}

// WorkerProfileMetrics tracks performance metrics for individual workers
type WorkerProfileMetrics struct {
	WorkerID       int
	JobsProcessed  int64
	TotalBusyTime  time.Duration
	TotalIdleTime  time.Duration
	AverageJobTime time.Duration
	Utilization    float64
	LastJobTime    time.Time
}

// NewPerformanceProfiler creates a new high-precision performance profiler
func NewPerformanceProfiler(config *ProfileConfig) *PerformanceProfiler {
	if config == nil {
		config = &ProfileConfig{
			Enabled:           true,
			Precision:         ProfilingAccuracy,
			MaxOverhead:       ProfilingOverheadLimit,
			SamplingRate:      SamplingInterval,
			MaxDataPoints:     MaxProfileDataPoints,
			EnableFlameGraph:  false,
			EnablePercentiles: true,
			EnableTrends:      true,
			RealtimeMetrics:   true,
			DetailLevel:       DetailLevelNormal,
		}
	}

	profiler := &PerformanceProfiler{
		precision: config.Precision,
		config:    config,
		metrics: &ProfileMetrics{
			CategoryMetrics: make(map[string]*CategoryMetrics),
			CheckMetrics:    make(map[string]*CheckProfileMetrics),
			WorkerMetrics:   make(map[int]*WorkerProfileMetrics),
			Percentiles:     make(map[float64]time.Duration),
		},
	}

	// Initialize components
	profiler.measurements = NewTimingStorage(config.MaxDataPoints)
	profiler.collector = NewMetricsCollector(profiler, config.SamplingRate)
	profiler.reporter = NewProfileReporter(profiler)

	profiler.enabled.Store(config.Enabled)

	return profiler
}

// Start begins profiling operations
func (p *PerformanceProfiler) Start() error {
	if !p.enabled.Load() {
		return fmt.Errorf("profiling is disabled")
	}

	if !p.started.CompareAndSwap(false, true) {
		return fmt.Errorf("profiler already started")
	}

	// Start metrics collector
	if p.config.RealtimeMetrics {
		go p.collector.Start()
	}

	// Initialize baseline metrics
	p.captureBaseline()

	return nil
}

// Stop halts profiling and generates final report
func (p *PerformanceProfiler) Stop() error {
	if !p.started.CompareAndSwap(true, false) {
		return fmt.Errorf("profiler not started")
	}

	// Stop collector
	if p.collector != nil {
		p.collector.Stop()
	}

	// Calculate final metrics
	p.calculateFinalMetrics()

	return nil
}

// ProfileOperation profiles a single operation with 1ms accuracy
func (p *PerformanceProfiler) ProfileOperation(name string, category string, operation func() error) error {
	if !p.enabled.Load() || !p.started.Load() {
		// Profiling disabled, just run the operation
		return operation()
	}

	// Use high-precision timer
	timer := profiling.NewPrecisionTimer()

	// Capture pre-execution state
	preResource := p.captureResourceSnapshot()

	// Start timing with nanosecond precision
	timer.Start()
	startTime := time.Now()

	// Execute operation
	err := operation()

	// Stop timing immediately
	duration := timer.Stop()
	endTime := time.Now()

	// Capture post-execution state
	postResource := p.captureResourceSnapshot()

	// Calculate profiling overhead
	overhead := timer.GetOverhead()
	atomic.AddInt64(&p.overhead, overhead)

	// Record timing data
	timing := &TimingData{
		Name:          name,
		StartTime:     startTime,
		EndTime:       endTime,
		Duration:      duration,
		Category:      category,
		Success:       err == nil,
		ResourceUsage: postResource,
		Metadata: map[string]interface{}{
			"pre_cpu":     preResource.CPUPercent,
			"post_cpu":    postResource.CPUPercent,
			"pre_memory":  preResource.MemoryBytes,
			"post_memory": postResource.MemoryBytes,
			"overhead_ns": overhead,
		},
	}

	// Add call stack if detailed profiling
	if p.config.DetailLevel >= DetailLevelDetailed {
		timing.CallStack = p.captureCallStack(2) // Skip ProfileOperation and caller
	}

	// Store measurement
	p.measurements.Store(timing)

	// Update real-time metrics
	p.updateMetrics(timing)

	return err
}

// ProfileCheck profiles a diagnostic check execution
func (p *PerformanceProfiler) ProfileCheck(check Check, client *docker.Client) (*CheckResult, error) {
	if !p.enabled.Load() {
		// Profiling disabled
		ctx := context.Background()
		return check.Run(ctx, client)
	}

	var result *CheckResult
	var runErr error

	err := p.ProfileOperation(
		fmt.Sprintf("check_%s", check.Name()),
		"diagnostic_check",
		func() error {
			ctx := context.Background()
			result, runErr = check.Run(ctx, client)
			return runErr
		},
	)

	// Update check-specific metrics
	p.updateCheckMetrics(check.Name(), err == nil)

	return result, err
}

// ProfileWorkerJob profiles a worker pool job execution
func (p *PerformanceProfiler) ProfileWorkerJob(workerID int, job Job) (*CheckResult, error) {
	if !p.enabled.Load() {
		// Profiling disabled
		return job.Check.Run(job.Context, job.Client)
	}

	var result *CheckResult
	var runErr error

	err := p.ProfileOperation(
		fmt.Sprintf("worker_%d_job_%d", workerID, job.ID),
		"worker_job",
		func() error {
			result, runErr = job.Check.Run(job.Context, job.Client)
			return runErr
		},
	)

	// Update worker-specific metrics
	p.updateWorkerMetrics(workerID, err == nil)

	// Store check association
	if result != nil {
		timing := p.measurements.GetLatest()
		if timing != nil {
			timing.CheckName = job.Check.Name()
			timing.WorkerID = workerID
		}
	}

	return result, err
}

// ProfileDockerAPICall profiles a Docker API call
func (p *PerformanceProfiler) ProfileDockerAPICall(operation string, apiCall func() error) error {
	return p.ProfileOperation(
		fmt.Sprintf("docker_api_%s", operation),
		"docker_api",
		apiCall,
	)
}

// captureBaseline captures initial baseline metrics
func (p *PerformanceProfiler) captureBaseline() {
	p.metrics.mu.Lock()
	defer p.metrics.mu.Unlock()

	snapshot := p.captureResourceSnapshot()
	p.metrics.CategoryMetrics["baseline"] = &CategoryMetrics{
		Count:       1,
		MinDuration: time.Duration(snapshot.MemoryBytes), // Store memory as baseline
	}
}

// captureResourceSnapshot captures current resource usage
func (p *PerformanceProfiler) captureResourceSnapshot() *ResourceSnapshot {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return &ResourceSnapshot{
		CPUPercent:   p.getCurrentCPUUsage(),
		MemoryBytes:  m.Alloc,
		GoroutineNum: runtime.NumGoroutine(),
		ThreadNum:    runtime.GOMAXPROCS(0),
		Timestamp:    time.Now(),
	}
}

// getCurrentCPUUsage calculates current CPU usage percentage
func (p *PerformanceProfiler) getCurrentCPUUsage() float64 {
	// This is a simplified implementation
	// In production, you'd want to use more sophisticated CPU tracking
	return 0.0 // Placeholder
}

// captureCallStack captures the current call stack
func (p *PerformanceProfiler) captureCallStack(skip int) []string {
	const maxDepth = 32
	pc := make([]uintptr, maxDepth)
	n := runtime.Callers(skip+1, pc)

	if n == 0 {
		return nil
	}

	pc = pc[:n]
	frames := runtime.CallersFrames(pc)

	stack := make([]string, 0, n)
	for {
		frame, more := frames.Next()
		stack = append(stack, fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function))
		if !more {
			break
		}
	}

	return stack
}

// updateMetrics updates real-time metrics with new timing data
func (p *PerformanceProfiler) updateMetrics(timing *TimingData) {
	p.metrics.mu.Lock()
	defer p.metrics.mu.Unlock()

	// Update global metrics
	p.metrics.TotalOperations++
	p.metrics.TotalDuration += timing.Duration

	// Update min/max
	if p.metrics.MinDuration == 0 || timing.Duration < p.metrics.MinDuration {
		p.metrics.MinDuration = timing.Duration
	}
	if timing.Duration > p.metrics.MaxDuration {
		p.metrics.MaxDuration = timing.Duration
	}

	// Update average
	p.metrics.AverageDuration = p.metrics.TotalDuration / time.Duration(p.metrics.TotalOperations)

	// Update category metrics
	category, exists := p.metrics.CategoryMetrics[timing.Category]
	if !exists {
		category = &CategoryMetrics{}
		p.metrics.CategoryMetrics[timing.Category] = category
	}

	category.Count++
	category.TotalDuration += timing.Duration
	category.AverageDuration = category.TotalDuration / time.Duration(category.Count)

	if category.MinDuration == 0 || timing.Duration < category.MinDuration {
		category.MinDuration = timing.Duration
	}
	if timing.Duration > category.MaxDuration {
		category.MaxDuration = timing.Duration
	}
}

// updateCheckMetrics updates check-specific metrics
func (p *PerformanceProfiler) updateCheckMetrics(checkName string, success bool) {
	p.metrics.mu.Lock()
	defer p.metrics.mu.Unlock()

	metrics, exists := p.metrics.CheckMetrics[checkName]
	if !exists {
		metrics = &CheckProfileMetrics{
			Name: checkName,
		}
		p.metrics.CheckMetrics[checkName] = metrics
	}

	metrics.ExecutionCount++
	metrics.LastExecution = time.Now()

	if success {
		metrics.SuccessRate = (metrics.SuccessRate*float64(metrics.ExecutionCount-1) + 1.0) / float64(metrics.ExecutionCount)
	} else {
		metrics.SuccessRate = (metrics.SuccessRate * float64(metrics.ExecutionCount-1)) / float64(metrics.ExecutionCount)
	}
}

// updateWorkerMetrics updates worker-specific metrics
func (p *PerformanceProfiler) updateWorkerMetrics(workerID int, success bool) {
	p.metrics.mu.Lock()
	defer p.metrics.mu.Unlock()

	metrics, exists := p.metrics.WorkerMetrics[workerID]
	if !exists {
		metrics = &WorkerProfileMetrics{
			WorkerID: workerID,
		}
		p.metrics.WorkerMetrics[workerID] = metrics
	}

	metrics.JobsProcessed++
	metrics.LastJobTime = time.Now()
}

// calculateFinalMetrics calculates final profiling metrics
func (p *PerformanceProfiler) calculateFinalMetrics() {
	p.metrics.mu.Lock()
	defer p.metrics.mu.Unlock()

	// Calculate percentiles if enabled
	if p.config.EnablePercentiles {
		p.calculatePercentiles()
	}

	// Calculate accuracy achieved
	p.metrics.AccuracyAchieved = p.calculateAccuracyAchieved()

	// Calculate overhead
	overhead := atomic.LoadInt64(&p.overhead)
	p.metrics.OverheadNanos = overhead
}

// calculatePercentiles calculates percentile metrics
func (p *PerformanceProfiler) calculatePercentiles() {
	durations := p.measurements.GetAllDurations()
	if len(durations) == 0 {
		return
	}

	// Calculate P50, P95, P99
	p.metrics.Percentiles[50] = calculatePercentile(durations, 50)
	p.metrics.Percentiles[95] = calculatePercentile(durations, 95)
	p.metrics.Percentiles[99] = calculatePercentile(durations, 99)

	// Update check percentiles
	for checkName, checkMetrics := range p.metrics.CheckMetrics {
		checkDurations := p.measurements.GetCheckDurations(checkName)
		if len(checkDurations) > 0 {
			checkMetrics.P50Duration = calculatePercentile(checkDurations, 50)
			checkMetrics.P95Duration = calculatePercentile(checkDurations, 95)
			checkMetrics.P99Duration = calculatePercentile(checkDurations, 99)
		}
	}
}

// calculateAccuracyAchieved determines the actual timing accuracy achieved
func (p *PerformanceProfiler) calculateAccuracyAchieved() time.Duration {
	// Analyze timing resolution from collected data
	durations := p.measurements.GetAllDurations()
	if len(durations) < 2 {
		return p.precision
	}

	// Find the smallest non-zero time difference
	minDiff := time.Duration(1<<63 - 1)
	for i := 1; i < len(durations); i++ {
		diff := durations[i] - durations[i-1]
		if diff > 0 && diff < minDiff {
			minDiff = diff
		}
	}

	// Round to nearest millisecond
	if minDiff < p.precision {
		return p.precision
	}
	return minDiff
}

// GetMetrics returns current profiling metrics
func (p *PerformanceProfiler) GetMetrics() *ProfileMetrics {
	p.metrics.mu.RLock()
	defer p.metrics.mu.RUnlock()

	// Create a copy without the mutex to avoid lock copying
	return &ProfileMetrics{
		TotalOperations: p.metrics.TotalOperations,
		TotalDuration:   p.metrics.TotalDuration,
		AverageDuration: p.metrics.AverageDuration,
		MinDuration:     p.metrics.MinDuration,
		MaxDuration:     p.metrics.MaxDuration,
		Percentiles: func() map[float64]time.Duration {
			percentiles := make(map[float64]time.Duration)
			for k, v := range p.metrics.Percentiles {
				percentiles[k] = v
			}
			return percentiles
		}(),
		CategoryMetrics: func() map[string]*CategoryMetrics {
			categoryMetrics := make(map[string]*CategoryMetrics)
			for k, v := range p.metrics.CategoryMetrics {
				catCopy := *v
				categoryMetrics[k] = &catCopy
			}
			return categoryMetrics
		}(),
		CheckMetrics: func() map[string]*CheckProfileMetrics {
			checkMetrics := make(map[string]*CheckProfileMetrics)
			for k, v := range p.metrics.CheckMetrics {
				checkCopy := *v
				checkMetrics[k] = &checkCopy
			}
			return checkMetrics
		}(),
		WorkerMetrics: func() map[int]*WorkerProfileMetrics {
			workerMetrics := make(map[int]*WorkerProfileMetrics)
			for k, v := range p.metrics.WorkerMetrics {
				workerCopy := *v
				workerMetrics[k] = &workerCopy
			}
			return workerMetrics
		}(),
		OverheadNanos:    p.metrics.OverheadNanos,
		AccuracyAchieved: p.metrics.AccuracyAchieved,
		// Deliberately omit mu field to avoid copying mutex
	}
}

// GetOverheadPercentage returns the profiling overhead as a percentage
func (p *PerformanceProfiler) GetOverheadPercentage() float64 {
	overhead := atomic.LoadInt64(&p.overhead)
	if p.metrics.TotalDuration == 0 {
		return 0
	}
	return float64(overhead) / float64(p.metrics.TotalDuration.Nanoseconds()) * 100
}

// IsWithinOverheadLimit checks if profiling overhead is within acceptable limits
func (p *PerformanceProfiler) IsWithinOverheadLimit() bool {
	return p.GetOverheadPercentage() <= p.config.MaxOverhead*100
}

// SetIntegration sets integration points with other components
func (p *PerformanceProfiler) SetIntegration(pool *SecureWorkerPool, client *docker.Client, engine *DiagnosticEngine) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.workerPool = pool
	p.dockerClient = client
	p.engine = engine
}

// Enable enables profiling
func (p *PerformanceProfiler) Enable() {
	p.enabled.Store(true)
}

// Disable disables profiling
func (p *PerformanceProfiler) Disable() {
	p.enabled.Store(false)
}

// IsEnabled returns whether profiling is enabled
func (p *PerformanceProfiler) IsEnabled() bool {
	return p.enabled.Load()
}

// Reset clears all profiling data
func (p *PerformanceProfiler) Reset() {
	p.metrics.mu.Lock()
	defer p.metrics.mu.Unlock()

	// Reset metrics
	p.metrics = &ProfileMetrics{
		CategoryMetrics: make(map[string]*CategoryMetrics),
		CheckMetrics:    make(map[string]*CheckProfileMetrics),
		WorkerMetrics:   make(map[int]*WorkerProfileMetrics),
		Percentiles:     make(map[float64]time.Duration),
	}

	// Clear measurements
	p.measurements.Clear()

	// Reset overhead counter
	atomic.StoreInt64(&p.overhead, 0)
}

// GenerateReport generates a detailed profiling report
func (p *PerformanceProfiler) GenerateReport() string {
	if p.reporter != nil {
		return p.reporter.GenerateReport()
	}
	return "No reporter configured"
}

// GenerateFlameGraph generates a flame graph visualization
func (p *PerformanceProfiler) GenerateFlameGraph() ([]byte, error) {
	if !p.config.EnableFlameGraph {
		return nil, fmt.Errorf("flame graph generation is disabled")
	}

	if p.reporter != nil {
		return p.reporter.GenerateFlameGraph()
	}

	return nil, fmt.Errorf("no reporter configured")
}

// Helper function to calculate percentile
func calculatePercentile(durations []time.Duration, percentile float64) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	// Sort durations (simple bubble sort for small datasets)
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)

	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Calculate percentile index
	index := int(float64(len(sorted)-1) * percentile / 100.0)
	return sorted[index]
}
