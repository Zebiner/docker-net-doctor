// Package diagnostics provides graceful degradation mechanisms that allow
// Docker Network Doctor to continue providing value even when facing partial
// system failures, resource constraints, or service unavailability.
//
// Graceful degradation ensures that users receive useful diagnostic information
// even when complete diagnostic capabilities are not available. The system
// intelligently reduces functionality while maintaining core diagnostic capabilities.
//
// Key Features:
//   - Automatic degradation trigger based on system health
//   - Multiple degradation levels with different capabilities
//   - Smart check selection based on available resources
//   - Partial result aggregation and reporting
//   - Recovery detection and automatic restoration
//   - User-friendly degradation notifications
//
// Degradation Levels:
//   - NORMAL: Full diagnostic capabilities available
//   - REDUCED: Limited diagnostics, non-essential checks disabled
//   - MINIMAL: Only critical connectivity checks enabled
//   - EMERGENCY: Basic health checks only, resource-conserving mode
//
// Triggers for Degradation:
//   - Docker daemon connectivity issues
//   - Resource exhaustion (memory, disk, CPU)
//   - Network partitions or connectivity problems
//   - Circuit breaker activation
//   - High error rates in diagnostic operations
//   - User-initiated degradation for troubleshooting
//
// Example usage:
//   degradation := NewGracefulDegradation(&DegradationConfig{
//       EnableAutoDetection: true,
//       DefaultLevel: DegradationNormal,
//   })
//
//   level := degradation.GetCurrentLevel()
//   checks := degradation.FilterChecks(allChecks, level)

package diagnostics

import (
	"fmt"
	"sync"
	"time"
)

// DegradationLevel represents the current operational capability level
type DegradationLevel int

const (
	// DegradationNormal indicates full diagnostic capabilities are available
	DegradationNormal DegradationLevel = iota

	// DegradationReduced indicates limited diagnostics with non-essential checks disabled
	DegradationReduced

	// DegradationMinimal indicates only critical connectivity checks are enabled
	DegradationMinimal

	// DegradationEmergency indicates basic health checks only, resource-conserving mode
	DegradationEmergency
)

// String returns a human-readable degradation level name
func (dl DegradationLevel) String() string {
	switch dl {
	case DegradationNormal:
		return "NORMAL"
	case DegradationReduced:
		return "REDUCED"
	case DegradationMinimal:
		return "MINIMAL"
	case DegradationEmergency:
		return "EMERGENCY"
	default:
		return "UNKNOWN"
	}
}

// GracefulDegradation manages system degradation levels and adaptive behavior
// based on system health, resource availability, and error patterns.
//
// The system monitors various health indicators and automatically adjusts
// operational capabilities to maintain service availability while conserving
// resources and avoiding cascade failures.
//
// Thread Safety: GracefulDegradation is safe for concurrent use.
type GracefulDegradation struct {
	config *DegradationConfig

	// Current state
	currentLevel      DegradationLevel
	lastLevelChange   time.Time
	degradationReason string

	// Health monitoring
	healthIndicators map[string]*HealthIndicator
	thresholds       map[DegradationLevel]*DegradationThresholds

	// Statistics and history
	levelHistory []LevelTransition
	metrics      *DegradationMetrics

	// Control
	manualOverride *DegradationLevel // Manual override if set
	autoDetection  bool

	// Thread safety
	mu sync.RWMutex

	// Shutdown control
	stopCh chan struct{}
}

// DegradationConfig defines behavior and thresholds for graceful degradation
type DegradationConfig struct {
	// Basic configuration
	EnableAutoDetection   bool             // Enable automatic degradation detection
	DefaultLevel          DegradationLevel // Starting degradation level
	ManualOverrideAllowed bool             // Allow manual degradation control

	// Auto-detection settings
	HealthCheckInterval   time.Duration // How often to check system health
	RecoveryCheckInterval time.Duration // How often to check for recovery
	StabilityPeriod       time.Duration // Time to wait before level changes

	// Thresholds for different levels
	ErrorRateThresholds    map[DegradationLevel]float64 // Error rate thresholds
	ResourceThresholds     map[DegradationLevel]*ResourceThresholds
	ResponseTimeThresholds map[DegradationLevel]time.Duration

	// Recovery settings
	EnableAutoRecovery      bool          // Allow automatic level improvement
	RecoveryStabilityPeriod time.Duration // Time of stable operation before recovery
	ConservativeRecovery    bool          // Use conservative recovery approach
}

// ResourceThresholds define resource usage limits for degradation triggers
type ResourceThresholds struct {
	MaxCPUPercent    float64 // Maximum CPU usage percentage
	MaxMemoryPercent float64 // Maximum memory usage percentage
	MaxDiskPercent   float64 // Maximum disk usage percentage
	MaxOpenFiles     int     // Maximum open file descriptors
	MaxConnections   int     // Maximum concurrent connections
}

// HealthIndicator tracks a specific aspect of system health
type HealthIndicator struct {
	Name        string      `json:"name"`
	Value       float64     `json:"value"`
	Threshold   float64     `json:"threshold"`
	Critical    bool        `json:"critical"`
	LastUpdated time.Time   `json:"last_updated"`
	Trend       HealthTrend `json:"trend"`
	Description string      `json:"description"`
}

// HealthTrend indicates the direction of change for a health indicator
type HealthTrend int

const (
	TrendStable HealthTrend = iota
	TrendImproving
	TrendDegrading
	TrendCritical
)

// DegradationThresholds define when to trigger specific degradation levels
type DegradationThresholds struct {
	ErrorRate           float64       // Maximum acceptable error rate
	ResponseTime        time.Duration // Maximum acceptable response time
	CircuitBreakerOpen  bool          // Whether circuit breaker being open triggers this level
	ResourceExhaustion  bool          // Whether resource exhaustion triggers this level
	ConnectionFailures  int           // Number of connection failures to trigger
	ConsecutiveFailures int           // Number of consecutive failures to trigger
}

// LevelTransition records a change in degradation level
type LevelTransition struct {
	FromLevel   DegradationLevel `json:"from_level"`
	ToLevel     DegradationLevel `json:"to_level"`
	Timestamp   time.Time        `json:"timestamp"`
	Reason      string           `json:"reason"`
	Automatic   bool             `json:"automatic"`
	HealthScore float64          `json:"health_score"`
}

// DegradationMetrics tracks degradation system performance and behavior
type DegradationMetrics struct {
	// Current state
	CurrentLevel       DegradationLevel `json:"current_level"`
	TimeInCurrentLevel time.Duration    `json:"time_in_current_level"`
	LastLevelChange    time.Time        `json:"last_level_change"`

	// Level distribution
	TimeInEachLevel  map[DegradationLevel]time.Duration `json:"time_in_each_level"`
	TransitionsCount map[string]int64                   `json:"transitions_count"` // from_level->to_level

	// Health indicators
	OverallHealthScore float64                     `json:"overall_health_score"`
	HealthIndicators   map[string]*HealthIndicator `json:"health_indicators"`

	// Performance impact
	ChecksSkipped  int64              `json:"checks_skipped"`
	ChecksExecuted int64              `json:"checks_executed"`
	ResourcesSaved map[string]float64 `json:"resources_saved"`

	// Recovery statistics
	AutoRecoveries   int64 `json:"auto_recoveries"`
	ManualOverrides  int64 `json:"manual_overrides"`
	FailedRecoveries int64 `json:"failed_recoveries"`
}

// CheckPriority defines the importance level of diagnostic checks
type CheckPriority int

const (
	CheckPriorityCritical CheckPriority = iota // Essential connectivity checks
	CheckPriorityHigh                          // Important network diagnostics
	CheckPriorityMedium                        // Standard diagnostic checks
	CheckPriorityLow                           // Optional/informational checks
)

// CheckRequirements defines resource requirements for diagnostic checks
type CheckRequirements struct {
	Priority          CheckPriority `json:"priority"`
	EstimatedDuration time.Duration `json:"estimated_duration"`
	MemoryRequirement int64         `json:"memory_requirement"`
	NetworkRequired   bool          `json:"network_required"`
	DockerAPIRequired bool          `json:"docker_api_required"`
	ContainerAccess   bool          `json:"container_access"`
	Tags              []string      `json:"tags"`
}

// NewGracefulDegradation creates a new graceful degradation system
func NewGracefulDegradation(config *DegradationConfig) *GracefulDegradation {
	if config == nil {
		config = &DegradationConfig{
			EnableAutoDetection:     true,
			DefaultLevel:            DegradationNormal,
			ManualOverrideAllowed:   true,
			HealthCheckInterval:     30 * time.Second,
			RecoveryCheckInterval:   60 * time.Second,
			StabilityPeriod:         5 * time.Minute,
			EnableAutoRecovery:      true,
			RecoveryStabilityPeriod: 10 * time.Minute,
			ConservativeRecovery:    true,
		}
	}

	// Set up default thresholds if not provided
	if config.ErrorRateThresholds == nil {
		config.ErrorRateThresholds = map[DegradationLevel]float64{
			DegradationNormal:    0.05, // 5% error rate
			DegradationReduced:   0.15, // 15% error rate
			DegradationMinimal:   0.30, // 30% error rate
			DegradationEmergency: 0.50, // 50% error rate
		}
	}

	if config.ResponseTimeThresholds == nil {
		config.ResponseTimeThresholds = map[DegradationLevel]time.Duration{
			DegradationNormal:    2 * time.Second,
			DegradationReduced:   5 * time.Second,
			DegradationMinimal:   10 * time.Second,
			DegradationEmergency: 30 * time.Second,
		}
	}

	// Ensure critical timing values are never zero to prevent panics
	if config.HealthCheckInterval <= 0 {
		config.HealthCheckInterval = 30 * time.Second
	}
	if config.RecoveryCheckInterval <= 0 {
		config.RecoveryCheckInterval = 60 * time.Second
	}
	if config.StabilityPeriod <= 0 {
		config.StabilityPeriod = 5 * time.Minute
	}
	if config.RecoveryStabilityPeriod <= 0 {
		config.RecoveryStabilityPeriod = 10 * time.Minute
	}

	gd := &GracefulDegradation{
		config:           config,
		currentLevel:     config.DefaultLevel,
		lastLevelChange:  time.Now(),
		healthIndicators: make(map[string]*HealthIndicator),
		thresholds:       make(map[DegradationLevel]*DegradationThresholds),
		levelHistory:     make([]LevelTransition, 0),
		autoDetection:    config.EnableAutoDetection,
		stopCh:           make(chan struct{}),
		metrics: &DegradationMetrics{
			CurrentLevel:     config.DefaultLevel,
			TimeInEachLevel:  make(map[DegradationLevel]time.Duration),
			TransitionsCount: make(map[string]int64),
			HealthIndicators: make(map[string]*HealthIndicator),
			ResourcesSaved:   make(map[string]float64),
		},
	}

	// Initialize default thresholds
	gd.initializeThresholds()

	// Initialize health indicators
	gd.initializeHealthIndicators()

	// Start health monitoring if auto-detection is enabled
	if config.EnableAutoDetection {
		go gd.healthMonitoringLoop()
	}

	return gd
}

// GetCurrentLevel returns the current degradation level
func (gd *GracefulDegradation) GetCurrentLevel() DegradationLevel {
	gd.mu.RLock()
	defer gd.mu.RUnlock()

	// Manual override takes precedence
	if gd.manualOverride != nil {
		return *gd.manualOverride
	}

	return gd.currentLevel
}

// Stop gracefully shuts down the health monitoring goroutine
func (gd *GracefulDegradation) Stop() {
	select {
	case <-gd.stopCh:
		// Already stopped
		return
	default:
		close(gd.stopCh)
	}
}

// SetManualLevel manually sets the degradation level, overriding automatic detection
func (gd *GracefulDegradation) SetManualLevel(level DegradationLevel, reason string) error {
	if !gd.config.ManualOverrideAllowed {
		return fmt.Errorf("manual override not allowed")
	}

	gd.mu.Lock()
	defer gd.mu.Unlock()

	oldLevel := gd.GetCurrentLevel()
	gd.manualOverride = &level

	// Record transition
	transition := LevelTransition{
		FromLevel:   oldLevel,
		ToLevel:     level,
		Timestamp:   time.Now(),
		Reason:      reason,
		Automatic:   false,
		HealthScore: gd.calculateHealthScore(),
	}

	gd.recordTransition(transition)
	gd.metrics.ManualOverrides++

	return nil
}

// ClearManualOverride removes manual level override, returning to automatic detection
func (gd *GracefulDegradation) ClearManualOverride() {
	gd.mu.Lock()
	defer gd.mu.Unlock()

	if gd.manualOverride != nil {
		oldLevel := *gd.manualOverride
		gd.manualOverride = nil

		// Check if automatic level would be different
		newLevel := gd.determineAppropriateLevel()
		if newLevel != oldLevel {
			transition := LevelTransition{
				FromLevel:   oldLevel,
				ToLevel:     newLevel,
				Timestamp:   time.Now(),
				Reason:      "Manual override cleared, returning to automatic detection",
				Automatic:   true,
				HealthScore: gd.calculateHealthScore(),
			}

			gd.currentLevel = newLevel
			gd.recordTransition(transition)
		}
	}
}

// FilterChecks filters diagnostic checks based on current degradation level
func (gd *GracefulDegradation) FilterChecks(checks []Check) []Check {
	level := gd.GetCurrentLevel()
	var filteredChecks []Check

	for _, check := range checks {
		if gd.shouldExecuteCheck(check, level) {
			filteredChecks = append(filteredChecks, check)
		} else {
			gd.metrics.ChecksSkipped++
		}
	}

	gd.metrics.ChecksExecuted += int64(len(filteredChecks))
	return filteredChecks
}

// shouldExecuteCheck determines if a check should run at the current degradation level
func (gd *GracefulDegradation) shouldExecuteCheck(check Check, level DegradationLevel) bool {
	// Get check requirements (this would need to be implemented by checks)
	// For now, we'll use check name and severity as indicators

	checkName := check.Name()
	severity := check.Severity()

	switch level {
	case DegradationNormal:
		return true // All checks allowed

	case DegradationReduced:
		// Skip low-priority checks
		if gd.isLowPriorityCheck(checkName) {
			return false
		}
		return true

	case DegradationMinimal:
		// Only critical and high-priority checks
		return severity == SeverityCritical || gd.isCriticalCheck(checkName)

	case DegradationEmergency:
		// Only essential connectivity checks
		return gd.isEssentialCheck(checkName)

	default:
		return false
	}
}

// UpdateHealthIndicator updates a specific health indicator value
func (gd *GracefulDegradation) UpdateHealthIndicator(name string, value float64) {
	gd.mu.Lock()
	defer gd.mu.Unlock()

	indicator, exists := gd.healthIndicators[name]
	if !exists {
		return // Indicator not registered
	}

	// Calculate trend
	oldValue := indicator.Value
	indicator.Value = value
	indicator.LastUpdated = time.Now()

	if value > oldValue*1.1 {
		indicator.Trend = TrendDegrading
	} else if value < oldValue*0.9 {
		indicator.Trend = TrendImproving
	} else {
		indicator.Trend = TrendStable
	}

	// Check if critical threshold crossed
	if value >= indicator.Threshold {
		indicator.Critical = true
		if indicator.Trend != TrendCritical {
			indicator.Trend = TrendCritical
		}
	} else {
		indicator.Critical = false
	}
}

// GetDegradationStatus returns current degradation status and explanation
func (gd *GracefulDegradation) GetDegradationStatus() DegradationStatus {
	gd.mu.RLock()
	defer gd.mu.RUnlock()

	level := gd.GetCurrentLevel()
	healthScore := gd.calculateHealthScore()

	return DegradationStatus{
		Level:        level,
		Reason:       gd.degradationReason,
		HealthScore:  healthScore,
		IsHealthy:    healthScore > 0.7,
		Automatic:    gd.manualOverride == nil,
		TimeInLevel:  time.Since(gd.lastLevelChange),
		Indicators:   gd.getHealthSummary(),
		Capabilities: gd.getCapabilitiesDescription(level),
	}
}

// DegradationStatus provides comprehensive status information
type DegradationStatus struct {
	Level        DegradationLevel            `json:"level"`
	Reason       string                      `json:"reason"`
	HealthScore  float64                     `json:"health_score"`
	IsHealthy    bool                        `json:"is_healthy"`
	Automatic    bool                        `json:"automatic"`
	TimeInLevel  time.Duration               `json:"time_in_level"`
	Indicators   map[string]*HealthIndicator `json:"indicators"`
	Capabilities map[string]bool             `json:"capabilities"`
}

// GetMetrics returns comprehensive degradation system metrics
func (gd *GracefulDegradation) GetMetrics() DegradationMetrics {
	gd.mu.RLock()
	defer gd.mu.RUnlock()

	// Update time-based metrics
	now := time.Now()
	gd.metrics.TimeInCurrentLevel = now.Sub(gd.lastLevelChange)
	gd.metrics.OverallHealthScore = gd.calculateHealthScore()

	// Copy health indicators
	gd.metrics.HealthIndicators = make(map[string]*HealthIndicator)
	for k, v := range gd.healthIndicators {
		indicatorCopy := *v
		gd.metrics.HealthIndicators[k] = &indicatorCopy
	}

	// Return copy to prevent external modification
	metrics := *gd.metrics
	return metrics
}

// Internal methods

func (gd *GracefulDegradation) initializeThresholds() {
	gd.thresholds[DegradationNormal] = &DegradationThresholds{
		ErrorRate:           0.05,
		ResponseTime:        2 * time.Second,
		CircuitBreakerOpen:  false,
		ResourceExhaustion:  false,
		ConnectionFailures:  2,
		ConsecutiveFailures: 3,
	}

	gd.thresholds[DegradationReduced] = &DegradationThresholds{
		ErrorRate:           0.15,
		ResponseTime:        5 * time.Second,
		CircuitBreakerOpen:  true,
		ResourceExhaustion:  false,
		ConnectionFailures:  5,
		ConsecutiveFailures: 5,
	}

	gd.thresholds[DegradationMinimal] = &DegradationThresholds{
		ErrorRate:           0.30,
		ResponseTime:        10 * time.Second,
		CircuitBreakerOpen:  true,
		ResourceExhaustion:  true,
		ConnectionFailures:  10,
		ConsecutiveFailures: 8,
	}

	gd.thresholds[DegradationEmergency] = &DegradationThresholds{
		ErrorRate:           0.50,
		ResponseTime:        30 * time.Second,
		CircuitBreakerOpen:  true,
		ResourceExhaustion:  true,
		ConnectionFailures:  20,
		ConsecutiveFailures: 15,
	}
}

func (gd *GracefulDegradation) initializeHealthIndicators() {
	indicators := []*HealthIndicator{
		{
			Name:        "error_rate",
			Threshold:   0.10, // 10% error rate threshold
			Description: "Overall system error rate",
		},
		{
			Name:        "response_time",
			Threshold:   5.0, // 5 second response time threshold
			Description: "Average system response time in seconds",
		},
		{
			Name:        "docker_connectivity",
			Threshold:   0.5, // 50% connectivity threshold
			Description: "Docker daemon connectivity health (0-1)",
		},
		{
			Name:        "resource_usage",
			Threshold:   0.8, // 80% resource usage threshold
			Description: "Overall system resource usage (0-1)",
		},
		{
			Name:        "circuit_breaker_health",
			Threshold:   0.5, // Circuit breaker health threshold
			Description: "Circuit breaker system health (0-1)",
		},
	}

	for _, indicator := range indicators {
		indicator.LastUpdated = time.Now()
		indicator.Trend = TrendStable
		gd.healthIndicators[indicator.Name] = indicator
	}
}

func (gd *GracefulDegradation) healthMonitoringLoop() {
	ticker := time.NewTicker(gd.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			gd.performHealthCheck()
		case <-gd.stopCh:
			return
		}
	}
}

func (gd *GracefulDegradation) performHealthCheck() {
	gd.mu.Lock()
	defer gd.mu.Unlock()

	// Skip if manual override is active
	if gd.manualOverride != nil {
		return
	}

	// Determine appropriate level based on current health
	appropriateLevel := gd.determineAppropriateLevel()

	// Check if level change is needed
	if appropriateLevel != gd.currentLevel {
		// Apply stability period to prevent flapping
		if time.Since(gd.lastLevelChange) < gd.config.StabilityPeriod {
			return // Too soon to change levels
		}

		// Change level
		gd.changeDegradationLevel(appropriateLevel, gd.generateLevelChangeReason(appropriateLevel))
	}
}

func (gd *GracefulDegradation) determineAppropriateLevel() DegradationLevel {
	healthScore := gd.calculateHealthScore()

	// Determine level based on health score and specific indicators
	if healthScore >= 0.8 && gd.allIndicatorsHealthy() {
		return DegradationNormal
	} else if healthScore >= 0.6 && gd.criticalIndicatorsHealthy() {
		return DegradationReduced
	} else if healthScore >= 0.3 {
		return DegradationMinimal
	} else {
		return DegradationEmergency
	}
}

func (gd *GracefulDegradation) calculateHealthScore() float64 {
	if len(gd.healthIndicators) == 0 {
		return 1.0 // No indicators, assume healthy
	}

	totalScore := 0.0
	totalWeight := 0.0

	for _, indicator := range gd.healthIndicators {
		// Calculate health score for this indicator (0-1, where 1 is healthy)
		indicatorScore := 1.0
		if indicator.Value > indicator.Threshold {
			// Indicator is above threshold, calculate degradation
			overage := indicator.Value - indicator.Threshold
			maxOverage := indicator.Threshold * 2 // Assume 0 health at 3x threshold
			if maxOverage > 0 {
				degradation := overage / maxOverage
				if degradation > 1.0 {
					degradation = 1.0
				}
				indicatorScore = 1.0 - degradation
			} else {
				indicatorScore = 0.0
			}
		}

		// Weight critical indicators higher
		weight := 1.0
		if indicator.Critical {
			weight = 2.0
		}

		totalScore += indicatorScore * weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		return 1.0
	}

	return totalScore / totalWeight
}

func (gd *GracefulDegradation) allIndicatorsHealthy() bool {
	for _, indicator := range gd.healthIndicators {
		if indicator.Critical || indicator.Value >= indicator.Threshold {
			return false
		}
	}
	return true
}

func (gd *GracefulDegradation) criticalIndicatorsHealthy() bool {
	criticalIndicators := []string{"docker_connectivity", "error_rate"}

	for _, name := range criticalIndicators {
		if indicator, exists := gd.healthIndicators[name]; exists {
			if indicator.Critical || indicator.Value >= indicator.Threshold {
				return false
			}
		}
	}
	return true
}

func (gd *GracefulDegradation) changeDegradationLevel(newLevel DegradationLevel, reason string) {
	oldLevel := gd.currentLevel
	gd.currentLevel = newLevel
	gd.lastLevelChange = time.Now()
	gd.degradationReason = reason

	transition := LevelTransition{
		FromLevel:   oldLevel,
		ToLevel:     newLevel,
		Timestamp:   time.Now(),
		Reason:      reason,
		Automatic:   true,
		HealthScore: gd.calculateHealthScore(),
	}

	gd.recordTransition(transition)

	// Update metrics
	if newLevel > oldLevel {
		gd.metrics.AutoRecoveries++
	}
}

func (gd *GracefulDegradation) recordTransition(transition LevelTransition) {
	gd.levelHistory = append(gd.levelHistory, transition)

	// Limit history size
	maxHistory := 100
	if len(gd.levelHistory) > maxHistory {
		gd.levelHistory = gd.levelHistory[len(gd.levelHistory)-maxHistory:]
	}

	// Update transition counts
	transitionKey := fmt.Sprintf("%s->%s", transition.FromLevel.String(), transition.ToLevel.String())
	gd.metrics.TransitionsCount[transitionKey]++
}

func (gd *GracefulDegradation) generateLevelChangeReason(newLevel DegradationLevel) string {
	healthScore := gd.calculateHealthScore()
	criticalIndicators := make([]string, 0)

	for name, indicator := range gd.healthIndicators {
		if indicator.Critical {
			criticalIndicators = append(criticalIndicators, name)
		}
	}

	if len(criticalIndicators) > 0 {
		return fmt.Sprintf("Health score %.2f, critical indicators: %v", healthScore, criticalIndicators)
	}

	return fmt.Sprintf("Health score %.2f, automatic level adjustment", healthScore)
}

// Check classification helpers

func (gd *GracefulDegradation) isLowPriorityCheck(checkName string) bool {
	lowPriorityChecks := []string{
		"subnet_overlap",
		"mtu_consistency",
	}

	for _, lowPriority := range lowPriorityChecks {
		if checkName == lowPriority {
			return true
		}
	}
	return false
}

func (gd *GracefulDegradation) isCriticalCheck(checkName string) bool {
	criticalChecks := []string{
		"daemon_connectivity",
		"bridge_network",
		"dns_resolution",
	}

	for _, critical := range criticalChecks {
		if checkName == critical {
			return true
		}
	}
	return false
}

func (gd *GracefulDegradation) isEssentialCheck(checkName string) bool {
	essentialChecks := []string{
		"daemon_connectivity",
	}

	for _, essential := range essentialChecks {
		if checkName == essential {
			return true
		}
	}
	return false
}

func (gd *GracefulDegradation) getHealthSummary() map[string]*HealthIndicator {
	summary := make(map[string]*HealthIndicator)
	for k, v := range gd.healthIndicators {
		indicatorCopy := *v
		summary[k] = &indicatorCopy
	}
	return summary
}

func (gd *GracefulDegradation) getCapabilitiesDescription(level DegradationLevel) map[string]bool {
	capabilities := make(map[string]bool)

	switch level {
	case DegradationNormal:
		capabilities["full_diagnostics"] = true
		capabilities["container_exec"] = true
		capabilities["network_analysis"] = true
		capabilities["performance_tests"] = true

	case DegradationReduced:
		capabilities["full_diagnostics"] = false
		capabilities["container_exec"] = true
		capabilities["network_analysis"] = true
		capabilities["performance_tests"] = false

	case DegradationMinimal:
		capabilities["full_diagnostics"] = false
		capabilities["container_exec"] = false
		capabilities["network_analysis"] = true
		capabilities["performance_tests"] = false

	case DegradationEmergency:
		capabilities["full_diagnostics"] = false
		capabilities["container_exec"] = false
		capabilities["network_analysis"] = false
		capabilities["performance_tests"] = false
	}

	return capabilities
}
