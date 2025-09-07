// internal/diagnostics/engine.go
package diagnostics

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// DiagnosticEngine orchestrates all network diagnostic checks
type DiagnosticEngine struct {
	dockerClient *docker.Client
	checks       []Check
	results      *Results
	config       *Config
	workerPool   *SecureWorkerPool  // New: secure worker pool for parallel execution
	rateLimiter  *RateLimiter       // New: rate limiter for API calls
}

// Config holds configuration for the diagnostic engine
type Config struct {
	Parallel     bool          // Run checks in parallel
	Timeout      time.Duration // Global timeout for all checks
	Verbose      bool          // Enable verbose output
	TargetFilter string        // Filter for specific containers/networks
	WorkerCount  int           // Number of worker goroutines (new)
	RateLimit    float64       // API rate limit (new)
}

// Check represents a single diagnostic check
type Check interface {
	Name() string
	Description() string
	Run(ctx context.Context, client *docker.Client) (*CheckResult, error)
	Severity() Severity // How critical is this check?
}

// Severity indicates how critical a failed check is
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityCritical
)

// CheckResult contains the outcome of a single diagnostic check
type CheckResult struct {
	CheckName   string
	Success     bool
	Message     string
	Details     map[string]interface{} // Additional diagnostic data
	Suggestions []string               // Suggested fixes
	Timestamp   time.Time
	Duration    time.Duration          // New: execution time
}

// Results aggregates all diagnostic results
type Results struct {
	mu       sync.Mutex
	Checks   []*CheckResult
	Summary  Summary
	Duration time.Duration
	Metrics  *ExecutionMetrics      // New: performance metrics
}

// Summary provides an overview of diagnostic results
type Summary struct {
	TotalChecks    int
	PassedChecks   int
	FailedChecks   int
	WarningChecks  int
	CriticalIssues []string
}

// ExecutionMetrics tracks performance and resource usage
type ExecutionMetrics struct {
	ParallelExecution bool
	WorkerCount       int
	TotalDuration     time.Duration
	AverageCheckTime  time.Duration
	MaxCheckTime      time.Duration
	MinCheckTime      time.Duration
	MemoryUsageMB     float64
	APICallsCount     int64
	RateLimitHits     int64
	ErrorRate         float64
}

// NewEngine creates a new diagnostic engine with default checks
func NewEngine(dockerClient *docker.Client, config *Config) *DiagnosticEngine {
	if config == nil {
		config = &Config{
			Parallel:    true,
			Timeout:     30 * time.Second,
			Verbose:     false,
			WorkerCount: runtime.NumCPU(),
			RateLimit:   DOCKER_API_RATE_LIMIT,
		}
	}

	// Ensure worker count is within bounds
	if config.WorkerCount <= 0 {
		config.WorkerCount = runtime.NumCPU()
	}
	if config.WorkerCount > MAX_WORKERS {
		config.WorkerCount = MAX_WORKERS
	}

	engine := &DiagnosticEngine{
		dockerClient: dockerClient,
		config:       config,
		results:      &Results{
			Checks:  make([]*CheckResult, 0),
			Metrics: &ExecutionMetrics{},
		},
		rateLimiter: NewRateLimiter(&RateLimiterConfig{
			RequestsPerSecond: config.RateLimit,
			BurstSize:         DOCKER_API_BURST,
			WaitTimeout:       5 * time.Second,
			Enabled:           true,
		}),
	}

	// Register all default checks in order of importance
	engine.registerDefaultChecks()
	
	return engine
}

// registerDefaultChecks adds all standard diagnostic checks
func (e *DiagnosticEngine) registerDefaultChecks() {
	// Start with basic connectivity to Docker daemon
	e.checks = append(e.checks, &DaemonConnectivityCheck{})
	
	// Network infrastructure checks
	e.checks = append(e.checks, &BridgeNetworkCheck{})
	e.checks = append(e.checks, &IPForwardingCheck{})
	e.checks = append(e.checks, &IptablesCheck{})
	
	// DNS checks
	e.checks = append(e.checks, &DNSResolutionCheck{})
	e.checks = append(e.checks, &InternalDNSCheck{})
	
	// Container-specific checks
	e.checks = append(e.checks, &ContainerConnectivityCheck{})
	e.checks = append(e.checks, &PortBindingCheck{})
	e.checks = append(e.checks, &NetworkIsolationCheck{})
	
	// Advanced checks
	e.checks = append(e.checks, &MTUConsistencyCheck{})
	e.checks = append(e.checks, &SubnetOverlapCheck{})
}

// Run executes all diagnostic checks
func (e *DiagnosticEngine) Run(ctx context.Context) (*Results, error) {
	startTime := time.Now()
	
	// Apply global timeout
	ctx, cancel := context.WithTimeout(ctx, e.config.Timeout)
	defer cancel()

	// Update metrics
	e.results.Metrics.ParallelExecution = e.config.Parallel
	e.results.Metrics.WorkerCount = e.config.WorkerCount

	if e.config.Parallel {
		// Use secure worker pool for parallel execution
		if err := e.runWithWorkerPool(ctx); err != nil {
			return nil, fmt.Errorf("worker pool execution failed: %w", err)
		}
	} else {
		e.runSequential(ctx)
	}

	// Calculate summary and metrics
	e.results.Duration = time.Since(startTime)
	e.results.Metrics.TotalDuration = e.results.Duration
	e.calculateSummary()
	e.calculateMetrics()
	
	return e.results, nil
}

// runWithWorkerPool executes checks using the secure worker pool
func (e *DiagnosticEngine) runWithWorkerPool(ctx context.Context) error {
	// Create and start worker pool
	pool, err := NewSecureWorkerPool(ctx, e.config.WorkerCount)
	if err != nil {
		return fmt.Errorf("failed to create worker pool: %w", err)
	}
	e.workerPool = pool

	if err := pool.Start(); err != nil {
		return fmt.Errorf("failed to start worker pool: %w", err)
	}
	defer pool.Stop()

	// Submit all checks to the pool
	for _, check := range e.checks {
		if err := pool.Submit(check, e.dockerClient); err != nil {
			if e.config.Verbose {
				fmt.Printf("Failed to submit check %s: %v\n", check.Name(), err)
			}
			// Continue with other checks even if one fails to submit
			continue
		}
	}

	// Collect results
	resultsChan := pool.GetResults()
	collectedCount := 0
	expectedCount := len(e.checks)
	
	// Use timeout for result collection
	timeout := time.After(e.config.Timeout)
	
	for collectedCount < expectedCount {
		select {
		case result, ok := <-resultsChan:
			if !ok {
				// Channel closed
				break
			}
			
			if result.Result != nil {
				result.Result.Duration = result.Duration
				e.results.mu.Lock()
				e.results.Checks = append(e.results.Checks, result.Result)
				e.results.mu.Unlock()
			}
			collectedCount++
			
		case <-timeout:
			if e.config.Verbose {
				fmt.Printf("Timeout collecting results. Got %d/%d results\n", 
					collectedCount, expectedCount)
			}
			return nil // Partial results are better than none
			
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Get pool metrics
	poolMetrics := pool.GetMetrics()
	e.results.Metrics.APICallsCount = poolMetrics.TotalAPIcalls
	e.results.Metrics.RateLimitHits = poolMetrics.RateLimitHits
	e.results.Metrics.MemoryUsageMB = poolMetrics.PeakMemoryMB

	return nil
}

// runParallel executes checks concurrently for faster diagnostics (legacy method)
func (e *DiagnosticEngine) runParallel(ctx context.Context) {
	var wg sync.WaitGroup
	resultsChan := make(chan *CheckResult, len(e.checks))
	
	for _, check := range e.checks {
		wg.Add(1)
		go func(c Check) {
			defer wg.Done()
			
			// Apply rate limiting
			if err := e.rateLimiter.Wait(ctx); err != nil {
				resultsChan <- &CheckResult{
					CheckName: c.Name(),
					Success:   false,
					Message:   fmt.Sprintf("Rate limit error: %v", err),
					Timestamp: time.Now(),
				}
				return
			}
			
			// Run check with panic recovery
			result := e.runCheckSafely(ctx, c)
			resultsChan <- result
		}(check)
	}
	
	// Wait for all checks to complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()
	
	// Collect results
	for result := range resultsChan {
		e.results.mu.Lock()
		e.results.Checks = append(e.results.Checks, result)
		e.results.mu.Unlock()
	}
}

// runSequential executes checks one by one (useful for debugging)
func (e *DiagnosticEngine) runSequential(ctx context.Context) {
	for _, check := range e.checks {
		// Apply rate limiting
		if err := e.rateLimiter.Wait(ctx); err != nil {
			e.results.Checks = append(e.results.Checks, &CheckResult{
				CheckName: check.Name(),
				Success:   false,
				Message:   fmt.Sprintf("Rate limit error: %v", err),
				Timestamp: time.Now(),
			})
			continue
		}
		
		startTime := time.Now()
		result := e.runCheckSafely(ctx, check)
		result.Duration = time.Since(startTime)
		e.results.Checks = append(e.results.Checks, result)
		
		// Stop on critical failure if configured
		if !result.Success && e.shouldStopOnFailure(check) {
			break
		}
	}
}

// runCheckSafely executes a check with panic recovery
func (e *DiagnosticEngine) runCheckSafely(ctx context.Context, check Check) *CheckResult {
	// Recover from any panics in individual checks
	defer func() {
		if r := recover(); r != nil {
			// Return error result instead of crashing
			fmt.Printf("Check %s panicked: %v\n", check.Name(), r)
		}
	}()
	
	if e.config.Verbose {
		fmt.Printf("Running check: %s\n", check.Description())
	}
	
	startTime := time.Now()
	result, err := check.Run(ctx, e.dockerClient)
	duration := time.Since(startTime)
	
	if err != nil {
		return &CheckResult{
			CheckName: check.Name(),
			Success:   false,
			Message:   fmt.Sprintf("Check failed with error: %v", err),
			Timestamp: time.Now(),
			Duration:  duration,
		}
	}
	
	if result == nil {
		result = &CheckResult{
			CheckName: check.Name(),
			Success:   false,
			Message:   "Check returned no result",
			Timestamp: time.Now(),
			Duration:  duration,
		}
	} else {
		result.Duration = duration
	}
	
	return result
}

// shouldStopOnFailure determines if we should halt on a failed check
func (e *DiagnosticEngine) shouldStopOnFailure(check Check) bool {
	// Stop only on critical infrastructure failures
	return check.Severity() == SeverityCritical && check.Name() == "daemon_connectivity"
}

// calculateSummary generates a summary of all check results
func (e *DiagnosticEngine) calculateSummary() {
	summary := Summary{
		TotalChecks:    len(e.results.Checks),
		CriticalIssues: make([]string, 0),
	}
	
	for _, result := range e.results.Checks {
		if result.Success {
			summary.PassedChecks++
		} else {
			summary.FailedChecks++
			
			// Track critical issues for quick reference
			for _, check := range e.checks {
				if check.Name() == result.CheckName && check.Severity() == SeverityCritical {
					summary.CriticalIssues = append(summary.CriticalIssues, result.Message)
					break
				}
			}
		}
	}
	
	e.results.Summary = summary
}

// calculateMetrics calculates execution metrics
func (e *DiagnosticEngine) calculateMetrics() {
	if len(e.results.Checks) == 0 {
		return
	}

	var totalDuration time.Duration
	var maxDuration time.Duration
	minDuration := time.Duration(1<<63 - 1) // Max int64
	failedCount := 0

	for _, result := range e.results.Checks {
		if result.Duration > 0 {
			totalDuration += result.Duration
			if result.Duration > maxDuration {
				maxDuration = result.Duration
			}
			if result.Duration < minDuration {
				minDuration = result.Duration
			}
		}
		if !result.Success {
			failedCount++
		}
	}

	checkCount := len(e.results.Checks)
	e.results.Metrics.AverageCheckTime = totalDuration / time.Duration(checkCount)
	e.results.Metrics.MaxCheckTime = maxDuration
	e.results.Metrics.MinCheckTime = minDuration
	e.results.Metrics.ErrorRate = float64(failedCount) / float64(checkCount)

	// Get rate limiter metrics
	if e.rateLimiter != nil {
		rlMetrics := e.rateLimiter.GetMetrics()
		e.results.Metrics.APICallsCount = rlMetrics.TotalRequests
		e.results.Metrics.RateLimitHits = rlMetrics.ThrottledRequests
	}
}

// GetRecommendations analyzes results and provides actionable recommendations
func (e *DiagnosticEngine) GetRecommendations() []Recommendation {
	recommendations := make([]Recommendation, 0)
	
	// Analyze patterns in failures
	networkIssues := 0
	dnsIssues := 0
	connectivityIssues := 0
	
	for _, result := range e.results.Checks {
		if !result.Success {
			switch result.CheckName {
			case "bridge_network", "subnet_overlap", "mtu_consistency":
				networkIssues++
			case "dns_resolution", "internal_dns":
				dnsIssues++
			case "container_connectivity", "port_binding":
				connectivityIssues++
			}
		}
	}
	
	// Generate high-level recommendations based on patterns
	if networkIssues > 1 {
		recommendations = append(recommendations, Recommendation{
			Priority: PriorityHigh,
			Category: "Network Configuration",
			Title:    "Multiple network configuration issues detected",
			Action:   "Review Docker network settings and consider resetting network configuration",
			Commands: []string{
				"docker network prune",
				"docker system prune",
				"systemctl restart docker",
			},
		})
	}
	
	if dnsIssues > 0 {
		recommendations = append(recommendations, Recommendation{
			Priority: PriorityMedium,
			Category: "DNS Resolution",
			Title:    "DNS resolution problems detected",
			Action:   "Check DNS configuration in containers and Docker daemon",
			Commands: []string{
				"docker exec <container> cat /etc/resolv.conf",
				"docker network inspect bridge | grep -A 5 IPAM",
			},
		})
	}
	
	// Add performance recommendation if parallel execution was beneficial
	if e.results.Metrics.ParallelExecution && e.results.Metrics.TotalDuration < 500*time.Millisecond {
		recommendations = append(recommendations, Recommendation{
			Priority: PriorityLow,
			Category: "Performance",
			Title:    "Excellent diagnostic performance",
			Action:   fmt.Sprintf("Diagnostics completed in %v using %d workers", 
				e.results.Metrics.TotalDuration, e.results.Metrics.WorkerCount),
			Commands: []string{},
		})
	}
	
	return recommendations
}

// GetExecutionReport generates a detailed execution report
func (e *DiagnosticEngine) GetExecutionReport() string {
	if e.results == nil || e.results.Metrics == nil {
		return "No execution data available"
	}

	m := e.results.Metrics
	report := fmt.Sprintf(
		"Execution Report:\n"+
		"================\n"+
		"Mode: %s\n"+
		"Workers: %d\n"+
		"Total Duration: %v\n"+
		"Average Check Time: %v\n"+
		"Max Check Time: %v\n"+
		"Min Check Time: %v\n"+
		"Memory Usage: %.2f MB\n"+
		"API Calls: %d\n"+
		"Rate Limit Hits: %d\n"+
		"Error Rate: %.2f%%\n",
		func() string {
			if m.ParallelExecution {
				return "Parallel"
			}
			return "Sequential"
		}(),
		m.WorkerCount,
		m.TotalDuration,
		m.AverageCheckTime,
		m.MaxCheckTime,
		m.MinCheckTime,
		m.MemoryUsageMB,
		m.APICallsCount,
		m.RateLimitHits,
		m.ErrorRate*100,
	)

	return report
}

// Recommendation represents an actionable fix suggestion
type Recommendation struct {
	Priority Priority
	Category string
	Title    string
	Action   string
	Commands []string // Specific commands to run
}

// Priority indicates the urgency of a recommendation
type Priority int

const (
	PriorityLow Priority = iota
	PriorityMedium
	PriorityHigh
)