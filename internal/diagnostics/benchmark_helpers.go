// internal/diagnostics/benchmark_helpers.go
package diagnostics

import (
	"context"
	"sync"
	"time"
)

// BenchmarkMetrics holds performance metrics for diagnostic operations
type BenchmarkMetrics struct {
	mu                sync.RWMutex
	CheckDurations    map[string]time.Duration
	TotalDuration     time.Duration
	ParallelSpeedup   float64
	MemoryUsed        uint64
	GoroutinesCreated int
}

// NewBenchmarkMetrics creates a new metrics collector
func NewBenchmarkMetrics() *BenchmarkMetrics {
	return &BenchmarkMetrics{
		CheckDurations: make(map[string]time.Duration),
	}
}

// RecordCheckDuration records the duration of a specific check
func (m *BenchmarkMetrics) RecordCheckDuration(checkName string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CheckDurations[checkName] = duration
}

// GetCheckDuration retrieves the duration for a specific check
func (m *BenchmarkMetrics) GetCheckDuration(checkName string) time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.CheckDurations[checkName]
}

// CalculateParallelSpeedup calculates speedup from parallel execution
func (m *BenchmarkMetrics) CalculateParallelSpeedup(sequentialTime, parallelTime time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if parallelTime > 0 {
		m.ParallelSpeedup = float64(sequentialTime) / float64(parallelTime)
	}
}

// BenchmarkEngine extends DiagnosticEngine with benchmarking capabilities
type BenchmarkEngine struct {
	*DiagnosticEngine
	metrics *BenchmarkMetrics
}

// NewBenchmarkEngine creates a diagnostic engine with benchmarking support
func NewBenchmarkEngine(engine *DiagnosticEngine) *BenchmarkEngine {
	return &BenchmarkEngine{
		DiagnosticEngine: engine,
		metrics:          NewBenchmarkMetrics(),
	}
}

// RunWithMetrics executes diagnostics while collecting performance metrics
func (e *BenchmarkEngine) RunWithMetrics(ctx context.Context) (*Results, *BenchmarkMetrics, error) {
	startTime := time.Now()
	
	// Run the actual diagnostics
	results, err := e.Run(ctx)
	
	// Record total duration
	e.metrics.TotalDuration = time.Since(startTime)
	
	// Record individual check durations if available
	for _, result := range results.Checks {
		if duration, ok := result.Details["duration"].(time.Duration); ok {
			e.metrics.RecordCheckDuration(result.CheckName, duration)
		}
	}
	
	return results, e.metrics, err
}

// MockCheck creates a mock diagnostic check for benchmarking
type MockCheck struct {
	name        string
	description string
	severity    Severity
	delay       time.Duration
	shouldFail  bool
}

// NewMockCheck creates a new mock check with specified behavior
func NewMockCheck(name string, delay time.Duration, shouldFail bool) *MockCheck {
	return &MockCheck{
		name:        name,
		description: "Mock check for benchmarking",
		severity:    SeverityInfo,
		delay:       delay,
		shouldFail:  shouldFail,
	}
}

func (c *MockCheck) Name() string        { return c.name }
func (c *MockCheck) Description() string { return c.description }
func (c *MockCheck) Severity() Severity  { return c.severity }

func (c *MockCheck) Run(ctx context.Context, client interface{}) (*CheckResult, error) {
	// Simulate work with specified delay
	select {
	case <-time.After(c.delay):
		// Work completed
	case <-ctx.Done():
		// Context cancelled
		return nil, ctx.Err()
	}
	
	return &CheckResult{
		CheckName: c.name,
		Success:   !c.shouldFail,
		Message:   "Mock check completed",
		Details: map[string]interface{}{
			"duration": c.delay,
		},
		Timestamp: time.Now(),
	}, nil
}

// CheckGroupBenchmark groups checks for category-based benchmarking
type CheckGroupBenchmark struct {
	SystemChecks    []Check
	NetworkChecks   []Check
	DNSChecks       []Check
	ContainerChecks []Check
}

// GetDefaultCheckGroups returns checks organized by category for benchmarking
func GetDefaultCheckGroups() *CheckGroupBenchmark {
	return &CheckGroupBenchmark{
		SystemChecks: []Check{
			&DaemonConnectivityCheck{},
			&IPForwardingCheck{},
			&IptablesCheck{},
		},
		NetworkChecks: []Check{
			&BridgeNetworkCheck{},
			&SubnetOverlapCheck{},
			&MTUConsistencyCheck{},
			&NetworkIsolationCheck{},
		},
		DNSChecks: []Check{
			&DNSResolutionCheck{},
			&InternalDNSCheck{},
		},
		ContainerChecks: []Check{
			&ContainerConnectivityCheck{},
			&PortBindingCheck{},
		},
	}
}

// ParallelizationStrategy defines how checks should be parallelized
type ParallelizationStrategy int

const (
	// StrategyFullParallel runs all checks in parallel
	StrategyFullParallel ParallelizationStrategy = iota
	
	// StrategyGroupParallel runs groups in sequence, checks within groups in parallel
	StrategyGroupParallel
	
	// StrategyAdaptive adjusts parallelism based on system resources
	StrategyAdaptive
	
	// StrategySequential runs all checks sequentially
	StrategySequential
)

// OptimizedEngine provides an optimized diagnostic engine for performance testing
type OptimizedEngine struct {
	*DiagnosticEngine
	strategy ParallelizationStrategy
	maxWorkers int
}

// NewOptimizedEngine creates an optimized engine with specified strategy
func NewOptimizedEngine(engine *DiagnosticEngine, strategy ParallelizationStrategy) *OptimizedEngine {
	return &OptimizedEngine{
		DiagnosticEngine: engine,
		strategy:         strategy,
		maxWorkers:       10, // Default max workers
	}
}

// RunOptimized executes diagnostics with optimization strategy
func (e *OptimizedEngine) RunOptimized(ctx context.Context) (*Results, error) {
	switch e.strategy {
	case StrategyFullParallel:
		e.config.Parallel = true
		return e.Run(ctx)
		
	case StrategyGroupParallel:
		return e.runGroupParallel(ctx)
		
	case StrategyAdaptive:
		return e.runAdaptive(ctx)
		
	case StrategySequential:
		e.config.Parallel = false
		return e.Run(ctx)
		
	default:
		return e.Run(ctx)
	}
}

// runGroupParallel runs check groups with controlled parallelism
func (e *OptimizedEngine) runGroupParallel(ctx context.Context) (*Results, error) {
	results := &Results{
		Checks: make([]*CheckResult, 0),
	}
	
	groups := GetDefaultCheckGroups()
	
	// Run each group sequentially, but checks within groups in parallel
	for _, groupChecks := range [][]Check{
		groups.SystemChecks,
		groups.NetworkChecks,
		groups.DNSChecks,
		groups.ContainerChecks,
	} {
		groupResults := e.runCheckGroup(ctx, groupChecks)
		results.Checks = append(results.Checks, groupResults...)
	}
	
	e.calculateSummary()
	return results, nil
}

// runCheckGroup runs a group of checks in parallel
func (e *OptimizedEngine) runCheckGroup(ctx context.Context, checks []Check) []*CheckResult {
	var wg sync.WaitGroup
	resultsChan := make(chan *CheckResult, len(checks))
	
	for _, check := range checks {
		wg.Add(1)
		go func(c Check) {
			defer wg.Done()
			result := e.runCheckSafely(ctx, c)
			resultsChan <- result
		}(check)
	}
	
	go func() {
		wg.Wait()
		close(resultsChan)
	}()
	
	var results []*CheckResult
	for result := range resultsChan {
		results = append(results, result)
	}
	
	return results
}

// runAdaptive dynamically adjusts parallelism based on system load
func (e *OptimizedEngine) runAdaptive(ctx context.Context) (*Results, error) {
	// Simple adaptive strategy: use parallel for I/O bound checks,
	// sequential for CPU bound checks
	
	results := &Results{
		Checks: make([]*CheckResult, 0),
	}
	
	// I/O bound checks (benefit from parallelism)
	ioChecks := []Check{
		&ContainerConnectivityCheck{},
		&DNSResolutionCheck{},
		&InternalDNSCheck{},
		&PortBindingCheck{},
	}
	
	// CPU/system checks (less benefit from parallelism)
	systemChecks := []Check{
		&DaemonConnectivityCheck{},
		&IPForwardingCheck{},
		&IptablesCheck{},
		&BridgeNetworkCheck{},
		&SubnetOverlapCheck{},
		&MTUConsistencyCheck{},
		&NetworkIsolationCheck{},
	}
	
	// Run I/O checks in parallel
	ioResults := e.runCheckGroup(ctx, ioChecks)
	results.Checks = append(results.Checks, ioResults...)
	
	// Run system checks sequentially
	for _, check := range systemChecks {
		result := e.runCheckSafely(ctx, check)
		results.Checks = append(results.Checks, result)
	}
	
	e.calculateSummary()
	return results, nil
}