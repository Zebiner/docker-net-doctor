// internal/diagnostics/engine.go
package diagnostics

import (
    "context"
    "fmt"
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
}

// Config holds configuration for the diagnostic engine
type Config struct {
    Parallel     bool          // Run checks in parallel
    Timeout      time.Duration // Global timeout for all checks
    Verbose      bool          // Enable verbose output
    TargetFilter string        // Filter for specific containers/networks
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
}

// Results aggregates all diagnostic results
type Results struct {
    mu       sync.Mutex
    Checks   []*CheckResult
    Summary  Summary
    Duration time.Duration
}

// Summary provides an overview of diagnostic results
type Summary struct {
    TotalChecks    int
    PassedChecks   int
    FailedChecks   int
    WarningChecks  int
    CriticalIssues []string
}

// NewEngine creates a new diagnostic engine with default checks
func NewEngine(dockerClient *docker.Client, config *Config) *DiagnosticEngine {
    if config == nil {
        config = &Config{
            Parallel: true,
            Timeout:  30 * time.Second,
            Verbose:  false,
        }
    }

    engine := &DiagnosticEngine{
        dockerClient: dockerClient,
        config:       config,
        results:      &Results{Checks: make([]*CheckResult, 0)},
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

    if e.config.Parallel {
        e.runParallel(ctx)
    } else {
        e.runSequential(ctx)
    }

    // Calculate summary
    e.results.Duration = time.Since(startTime)
    e.calculateSummary()
    
    return e.results, nil
}

// runParallel executes checks concurrently for faster diagnostics
func (e *DiagnosticEngine) runParallel(ctx context.Context) {
    var wg sync.WaitGroup
    resultsChan := make(chan *CheckResult, len(e.checks))
    
    for _, check := range e.checks {
        wg.Add(1)
        go func(c Check) {
            defer wg.Done()
            
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
        result := e.runCheckSafely(ctx, check)
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
    
    result, err := check.Run(ctx, e.dockerClient)
    if err != nil {
        return &CheckResult{
            CheckName: check.Name(),
            Success:   false,
            Message:   fmt.Sprintf("Check failed with error: %v", err),
            Timestamp: time.Now(),
        }
    }
    
    if result == nil {
        result = &CheckResult{
            CheckName: check.Name(),
            Success:   false,
            Message:   "Check returned no result",
            Timestamp: time.Now(),
        }
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
    
    return recommendations
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
