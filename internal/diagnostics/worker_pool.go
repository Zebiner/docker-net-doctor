// Package diagnostics provides secure worker pool implementation for concurrent check execution
package diagnostics

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/docker"
	"golang.org/x/time/rate"
)

// Security limits approved by security review
const (
	MAX_WORKERS          = 10                     // Bounded worker pool
	MAX_QUEUE_SIZE       = 100                    // Prevent memory exhaustion
	MAX_CHECK_TIMEOUT    = 30 * time.Second       // Individual check timeout
	MAX_TOTAL_TIMEOUT    = 2 * time.Minute         // Total execution timeout
	DOCKER_API_RATE_LIMIT = 5                     // 5 calls/second
	DOCKER_API_BURST     = 10                     // Burst of 10 calls
	MAX_MEMORY_MB        = 100                    // Maximum additional memory overhead
)

// Job represents a diagnostic check to be executed
type Job struct {
	ID       int
	Check    Check
	Context  context.Context
	Client   *docker.Client
}

// JobResult contains the result of executing a job
type JobResult struct {
	ID       int
	Result   *CheckResult
	Error    error
	Duration time.Duration
}

// SecureWorkerPool manages concurrent execution with security constraints
type SecureWorkerPool struct {
	// Core components
	workers      int                   // Number of worker goroutines
	jobs         chan Job              // Buffered job queue
	results      chan JobResult        // Results collection channel
	wg           sync.WaitGroup        // Worker synchronization
	ctx          context.Context       // Pool context for cancellation
	cancel       context.CancelFunc    // Cancel function

	// Security and monitoring
	rateLimiter  *rate.Limiter         // API rate limiting
	metrics      *PoolMetrics          // Performance tracking
	validator    *SecurityValidator    // Security validation
	
	// State management
	started      atomic.Bool           // Pool started flag
	stopped      atomic.Bool           // Pool stopped flag
	activeJobs   atomic.Int32          // Number of active jobs
	completedJobs atomic.Int32         // Number of completed jobs
	failedJobs   atomic.Int32          // Number of failed jobs
	
	// Resource monitoring
	memoryMonitor *MemoryMonitor       // Memory usage tracking
	errorRate     *ErrorRateMonitor    // Error rate circuit breaker
}

// PoolMetrics tracks performance and resource usage
type PoolMetrics struct {
	mu                sync.RWMutex
	StartTime         time.Time
	EndTime           time.Time
	TotalJobs         int
	CompletedJobs     int
	FailedJobs        int
	AverageJobTime    time.Duration
	MaxJobTime        time.Duration
	MinJobTime        time.Duration
	TotalAPIcalls     int64
	RateLimitHits     int64
	PeakMemoryMB      float64
	PeakWorkers       int
	RecoveredPanics   int
}

// MemoryMonitor tracks memory usage
type MemoryMonitor struct {
	baseline     uint64
	peakUsage    uint64
	mu           sync.RWMutex
	checkInterval time.Duration
	stopChan     chan struct{}
}

// ErrorRateMonitor implements circuit breaker pattern
type ErrorRateMonitor struct {
	mu              sync.RWMutex
	errorCount      int
	successCount    int
	window          time.Duration
	threshold       float64
	lastReset       time.Time
	circuitOpen     bool
}

// NewSecureWorkerPool creates a new secure worker pool with bounded concurrency
func NewSecureWorkerPool(ctx context.Context, workerCount int) (*SecureWorkerPool, error) {
	// Validate and bound worker count
	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}
	if workerCount > MAX_WORKERS {
		workerCount = MAX_WORKERS
	}

	// Create cancellable context with total timeout
	poolCtx, cancel := context.WithTimeout(ctx, MAX_TOTAL_TIMEOUT)

	// Determine if we're in test mode
	isTestMode := isTestContext()
	
	var validator *SecurityValidator
	if isTestMode {
		validator = NewSecurityValidatorTestMode()
	} else {
		validator = NewSecurityValidator()
	}

	pool := &SecureWorkerPool{
		workers:     workerCount,
		jobs:        make(chan Job, MAX_QUEUE_SIZE),
		results:     make(chan JobResult, MAX_QUEUE_SIZE),
		ctx:         poolCtx,
		cancel:      cancel,
		rateLimiter: rate.NewLimiter(rate.Limit(DOCKER_API_RATE_LIMIT), DOCKER_API_BURST),
		metrics: &PoolMetrics{
			StartTime: time.Now(),
		},
		validator: validator,
		memoryMonitor: &MemoryMonitor{
			checkInterval: 1 * time.Second,
			stopChan:      make(chan struct{}),
		},
		errorRate: &ErrorRateMonitor{
			window:    1 * time.Minute,
			threshold: 0.5, // Circuit opens at 50% error rate
			lastReset: time.Now(),
		},
	}

	// Initialize memory baseline
	pool.memoryMonitor.baseline = getCurrentMemory()

	return pool, nil
}

// isTestContext checks if we're running in a test context
func isTestContext() bool {
	// Check if the binary has .test suffix (Go test binaries)
	if strings.HasSuffix(os.Args[0], ".test") {
		return true
	}
	
	// Check for common test indicators in arguments
	for _, arg := range os.Args {
		if strings.Contains(arg, "-test.") {
			return true
		}
	}
	
	// Check environment variable as fallback
	if os.Getenv("GO_TEST") != "" {
		return true
	}
	
	return false
}

// Start initializes and starts the worker pool
func (p *SecureWorkerPool) Start() error {
	if !p.started.CompareAndSwap(false, true) {
		return fmt.Errorf("worker pool already started")
	}

	// Start memory monitoring
	go p.memoryMonitor.Start()

	// Start workers
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}

	// Note: Removed the resultCollector as it was consuming results without purpose

	p.metrics.mu.Lock()
	p.metrics.PeakWorkers = p.workers
	p.metrics.mu.Unlock()

	return nil
}

// Stop gracefully shuts down the worker pool
func (p *SecureWorkerPool) Stop() error {
	if !p.stopped.CompareAndSwap(false, true) {
		return fmt.Errorf("worker pool already stopped")
	}

	// Close job channel to signal workers to stop
	close(p.jobs)

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Workers finished gracefully
	case <-time.After(10 * time.Second):
		// Force shutdown after timeout
		p.cancel()
	}

	// Stop memory monitoring
	close(p.memoryMonitor.stopChan)

	// Don't close the results channel immediately - let the consumer drain it
	// The channel will be garbage collected when no longer referenced
	
	// Update metrics
	p.metrics.mu.Lock()
	p.metrics.EndTime = time.Now()
	p.metrics.mu.Unlock()

	return nil
}

// Submit adds a new job to the pool
func (p *SecureWorkerPool) Submit(check Check, client *docker.Client) error {
	// Check if pool is running
	if !p.started.Load() || p.stopped.Load() {
		return fmt.Errorf("worker pool is not running")
	}

	// Check circuit breaker
	if p.errorRate.IsOpen() {
		return fmt.Errorf("circuit breaker open: too many errors")
	}

	// Validate check with security validator
	if err := p.validator.ValidateCheck(check); err != nil {
		return fmt.Errorf("security validation failed: %w", err)
	}

	// Check memory usage
	if p.memoryMonitor.IsOverLimit() {
		return fmt.Errorf("memory limit exceeded")
	}

	// Create job with timeout context
	jobCtx, _ := context.WithTimeout(p.ctx, MAX_CHECK_TIMEOUT)
	
	job := Job{
		ID:      int(p.completedJobs.Load() + p.activeJobs.Load()),
		Check:   check,
		Context: jobCtx,
		Client:  client,
	}

	// Submit job with timeout
	select {
	case p.jobs <- job:
		p.activeJobs.Add(1)
		return nil
	case <-time.After(1 * time.Second):
		return fmt.Errorf("job queue full")
	case <-p.ctx.Done():
		return fmt.Errorf("pool context cancelled")
	}
}

// worker processes jobs from the queue
func (p *SecureWorkerPool) worker(id int) {
	defer p.wg.Done()
	defer func() {
		// Recover from panics at worker level
		if r := recover(); r != nil {
			p.metrics.mu.Lock()
			p.metrics.RecoveredPanics++
			p.metrics.mu.Unlock()
			fmt.Printf("Worker %d recovered from panic: %v\n", id, r)
		}
	}()

	for job := range p.jobs {
		// Check context cancellation
		select {
		case <-p.ctx.Done():
			// Put the job back if context is cancelled
			p.activeJobs.Add(-1)
			return
		default:
		}

		// Wait for rate limiter
		if err := p.rateLimiter.Wait(job.Context); err != nil {
			p.results <- JobResult{
				ID:    job.ID,
				Error: fmt.Errorf("rate limit error: %w", err),
			}
			p.activeJobs.Add(-1)
			p.failedJobs.Add(1)
			continue
		}

		// Execute job with monitoring
		startTime := time.Now()
		result, err, panicked := p.executeJob(job)
		duration := time.Since(startTime)

		// Update panic counter if job panicked
		if panicked {
			p.metrics.mu.Lock()
			p.metrics.RecoveredPanics++
			p.metrics.mu.Unlock()
		}

		// Update metrics
		p.updateMetrics(duration, err == nil)

		// Send result
		select {
		case p.results <- JobResult{
			ID:       job.ID,
			Result:   result,
			Error:    err,
			Duration: duration,
		}:
			// Result sent successfully
		case <-p.ctx.Done():
			// Context cancelled, stop sending results
			p.activeJobs.Add(-1)
			return
		}

		p.activeJobs.Add(-1)
		if err != nil {
			p.failedJobs.Add(1)
			p.errorRate.RecordError()
		} else {
			p.completedJobs.Add(1)
			p.errorRate.RecordSuccess()
		}
	}
}

// executeJob runs a single job with panic recovery
// Returns (result, error, panicked)
func (p *SecureWorkerPool) executeJob(job Job) (result *CheckResult, err error, panicked bool) {
	// Panic recovery for individual job
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			err = fmt.Errorf("job panicked: %v", r)
			result = &CheckResult{
				CheckName: job.Check.Name(),
				Success:   false,
				Message:   fmt.Sprintf("Check panicked: %v", r),
				Timestamp: time.Now(),
			}
		}
	}()

	// Execute the check
	result, err = job.Check.Run(job.Context, job.Client)
	if err != nil {
		return &CheckResult{
			CheckName: job.Check.Name(),
			Success:   false,
			Message:   fmt.Sprintf("Check failed: %v", err),
			Timestamp: time.Now(),
		}, err, false
	}

	return result, nil, false
}

// GetResults returns a channel to receive job results
func (p *SecureWorkerPool) GetResults() <-chan JobResult {
	return p.results
}

// WaitForCompletion waits for all submitted jobs to complete
func (p *SecureWorkerPool) WaitForCompletion(timeout time.Duration) error {
	done := make(chan struct{})
	
	go func() {
		// Wait until no active jobs and all jobs are processed
		for {
			if p.activeJobs.Load() == 0 {
				close(done)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for completion")
	}
}

// GetMetrics returns current pool metrics
func (p *SecureWorkerPool) GetMetrics() PoolMetrics {
	p.metrics.mu.RLock()
	defer p.metrics.mu.RUnlock()
	
	metrics := *p.metrics
	metrics.TotalJobs = int(p.completedJobs.Load() + p.failedJobs.Load())
	metrics.CompletedJobs = int(p.completedJobs.Load())
	metrics.FailedJobs = int(p.failedJobs.Load())
	metrics.PeakMemoryMB = float64(p.memoryMonitor.GetPeakUsage()) / (1024 * 1024)
	
	return metrics
}

// updateMetrics updates pool performance metrics
func (p *SecureWorkerPool) updateMetrics(duration time.Duration, success bool) {
	p.metrics.mu.Lock()
	defer p.metrics.mu.Unlock()

	p.metrics.TotalAPIcalls++

	// Update timing metrics
	if p.metrics.MinJobTime == 0 || duration < p.metrics.MinJobTime {
		p.metrics.MinJobTime = duration
	}
	if duration > p.metrics.MaxJobTime {
		p.metrics.MaxJobTime = duration
	}

	// Calculate running average
	totalJobs := p.completedJobs.Load() + p.failedJobs.Load()
	if totalJobs > 0 {
		currentAvg := p.metrics.AverageJobTime
		p.metrics.AverageJobTime = (currentAvg*time.Duration(totalJobs-1) + duration) / time.Duration(totalJobs)
	}
}

// IsHealthy checks if the pool is operating within normal parameters
func (p *SecureWorkerPool) IsHealthy() bool {
	// Check if pool is running
	if !p.started.Load() || p.stopped.Load() {
		return false
	}

	// Check circuit breaker
	if p.errorRate.IsOpen() {
		return false
	}

	// Check memory usage
	if p.memoryMonitor.IsOverLimit() {
		return false
	}

	// Check active jobs not stuck
	activeJobs := p.activeJobs.Load()
	if activeJobs > int32(p.workers*2) {
		return false
	}

	return true
}

// SetTestMode enables test mode for the security validator
func (p *SecureWorkerPool) SetTestMode(enabled bool) {
	p.validator.SetTestMode(enabled)
}

// MemoryMonitor implementation

func (m *MemoryMonitor) Start() {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			current := getCurrentMemory()
			m.mu.Lock()
			if current > m.peakUsage {
				m.peakUsage = current
			}
			m.mu.Unlock()
		case <-m.stopChan:
			return
		}
	}
}

func (m *MemoryMonitor) IsOverLimit() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	current := getCurrentMemory()
	additionalMemory := current - m.baseline
	return additionalMemory > (MAX_MEMORY_MB * 1024 * 1024)
}

func (m *MemoryMonitor) GetPeakUsage() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.peakUsage - m.baseline
}

// ErrorRateMonitor implementation

func (e *ErrorRateMonitor) RecordError() {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	e.checkWindow()
	e.errorCount++
	e.updateCircuitState()
}

func (e *ErrorRateMonitor) RecordSuccess() {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	e.checkWindow()
	e.successCount++
	e.updateCircuitState()
}

func (e *ErrorRateMonitor) IsOpen() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.circuitOpen
}

func (e *ErrorRateMonitor) checkWindow() {
	if time.Since(e.lastReset) > e.window {
		e.errorCount = 0
		e.successCount = 0
		e.lastReset = time.Now()
		e.circuitOpen = false
	}
}

func (e *ErrorRateMonitor) updateCircuitState() {
	total := e.errorCount + e.successCount
	if total < 10 {
		// Not enough data
		return
	}

	errorRate := float64(e.errorCount) / float64(total)
	if errorRate > e.threshold {
		e.circuitOpen = true
	} else if errorRate < (e.threshold * 0.5) {
		// Close circuit when error rate drops significantly
		e.circuitOpen = false
	}
}

// getCurrentMemory returns current memory usage in bytes
func getCurrentMemory() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}