// Package diagnostics provides circuit breaker pattern implementation to prevent
// cascade failures and protect system resources during Docker Network Doctor operations.
//
// The circuit breaker pattern prevents a failing service from being overwhelmed with
// requests that are likely to fail. It maintains statistics about recent failures and
// automatically "opens" the circuit when failure rates exceed thresholds, allowing
// the system to fail fast and recover gracefully.
//
// Circuit States:
//   - CLOSED: Normal operation, requests flow through
//   - OPEN: Circuit is open, requests are rejected immediately
//   - HALF_OPEN: Testing state, limited requests allowed to test recovery
//
// Key Features:
//   - Automatic failure detection and circuit opening
//   - Configurable failure thresholds and recovery timeouts
//   - Half-open state for gradual recovery testing
//   - Comprehensive metrics and monitoring
//   - Thread-safe operation for concurrent usage
//   - Adaptive thresholds based on request volume
//
// Example usage:
//   cb := NewCircuitBreaker(&CircuitBreakerConfig{
//       FailureThreshold: 5,
//       RecoveryTimeout: 30 * time.Second,
//       ConsecutiveSuccesses: 3,
//   })
//
//   if cb.CanExecute() {
//       result, err := operation()
//       if err != nil {
//           cb.RecordFailure()
//       } else {
//           cb.RecordSuccess()
//       }
//   }

package diagnostics

import (
	"sync"
	"time"
)

// CircuitState represents the current state of the circuit breaker
type CircuitState int

const (
	// CircuitStateClosed indicates normal operation - requests flow through
	CircuitStateClosed CircuitState = iota

	// CircuitStateOpen indicates circuit is open - requests are rejected
	CircuitStateOpen

	// CircuitStateHalfOpen indicates testing state - limited requests allowed
	CircuitStateHalfOpen
)

// String returns a human-readable representation of the circuit state
func (cs CircuitState) String() string {
	switch cs {
	case CircuitStateClosed:
		return "CLOSED"
	case CircuitStateOpen:
		return "OPEN"
	case CircuitStateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreaker implements the circuit breaker pattern with configurable thresholds
// and recovery mechanisms. It protects downstream services from being overwhelmed
// when they are failing or experiencing issues.
//
// The CircuitBreaker maintains:
//   - Failure counts and success counts
//   - State transitions with appropriate timing
//   - Request statistics for intelligent decision making
//   - Recovery testing in half-open state
//   - Comprehensive metrics for monitoring
//
// Thread Safety: CircuitBreaker is safe for concurrent use across multiple goroutines.
type CircuitBreaker struct {
	config *CircuitBreakerConfig

	// State management
	state           CircuitState
	lastFailTime    time.Time
	lastStateChange time.Time

	// Counters
	failureCount         int
	successCount         int // Used in half-open state
	halfOpenRequestCount int // Track requests made in half-open state
	consecutiveFailures  int
	totalRequests        int64
	totalFailures        int64
	totalSuccesses       int64

	// Timing
	lastRequestTime time.Time

	// Metrics
	stateTransitions map[CircuitState]int64

	// Thread safety
	mu sync.RWMutex
}

// CircuitBreakerConfig defines the behavior and thresholds for the circuit breaker
type CircuitBreakerConfig struct {
	// Failure thresholds
	FailureThreshold     int     // Number of failures before opening circuit
	FailureRate          float64 // Failure rate threshold (0.0-1.0) - alternative to count
	MinRequestsThreshold int     // Minimum requests before considering failure rate

	// Recovery configuration
	RecoveryTimeout      time.Duration // Time to wait before trying half-open
	ConsecutiveSuccesses int           // Successes needed in half-open to close circuit
	HalfOpenMaxRequests  int           // Maximum requests allowed in half-open state

	// Timing configuration
	ResetTimeout time.Duration // Time to reset failure count in closed state

	// Advanced features
	AdaptiveThresholds     bool // Enable adaptive threshold adjustment
	RequestVolumeThreshold int  // Minimum request volume for adaptive behavior
}

// CircuitBreakerMetrics provides comprehensive statistics about circuit breaker operation
type CircuitBreakerMetrics struct {
	// Current state
	State                CircuitState `json:"state"`
	FailureCount         int          `json:"failure_count"`
	SuccessCount         int          `json:"success_count"`
	HalfOpenRequestCount int          `json:"half_open_request_count"`
	ConsecutiveFailures  int          `json:"consecutive_failures"`

	// Historical data
	TotalRequests  int64 `json:"total_requests"`
	TotalFailures  int64 `json:"total_failures"`
	TotalSuccesses int64 `json:"total_successes"`

	// Timing information
	LastFailTime    time.Time `json:"last_fail_time"`
	LastStateChange time.Time `json:"last_state_change"`
	LastRequestTime time.Time `json:"last_request_time"`

	// State transitions
	StateTransitions map[CircuitState]int64 `json:"state_transitions"`

	// Calculated metrics
	FailureRate        float64       `json:"failure_rate"`
	RequestRate        float64       `json:"request_rate"` // Requests per second
	TimeInCurrentState time.Duration `json:"time_in_current_state"`

	// Health indicators
	IsHealthy             bool      `json:"is_healthy"`
	EstimatedRecoveryTime time.Time `json:"estimated_recovery_time"`
}

// NewCircuitBreaker creates a new circuit breaker with the specified configuration.
// If config is nil, sensible defaults will be applied.
//
// Default configuration:
//   - 5 failure threshold
//   - 30 second recovery timeout
//   - 3 consecutive successes to close
//   - 2 requests allowed in half-open state
//   - 60 second reset timeout
func NewCircuitBreaker(config *CircuitBreakerConfig) *CircuitBreaker {
	if config == nil {
		config = &CircuitBreakerConfig{
			FailureThreshold:       5,
			RecoveryTimeout:        30 * time.Second,
			ConsecutiveSuccesses:   3,
			HalfOpenMaxRequests:    2,
			ResetTimeout:           60 * time.Second,
			MinRequestsThreshold:   10,
			RequestVolumeThreshold: 20,
		}
	}

	// Set defaults for unspecified values
	if config.ConsecutiveSuccesses == 0 {
		config.ConsecutiveSuccesses = 3
	}
	if config.HalfOpenMaxRequests == 0 {
		config.HalfOpenMaxRequests = 2
	}
	if config.RecoveryTimeout == 0 {
		config.RecoveryTimeout = 30 * time.Second
	}
	if config.ResetTimeout == 0 {
		config.ResetTimeout = 60 * time.Second
	}

	return &CircuitBreaker{
		config:           config,
		state:            CircuitStateClosed,
		lastStateChange:  time.Now(),
		stateTransitions: make(map[CircuitState]int64),
	}
}

// CanExecute returns true if requests should be allowed through the circuit breaker.
// This method implements the core circuit breaker logic for request admission.
//
// Decision logic:
//   - CLOSED: Allow all requests (normal operation)
//   - OPEN: Reject all requests until recovery timeout, then transition to HALF_OPEN
//   - HALF_OPEN: Allow limited requests to test service recovery
//
// Returns true if the request should proceed, false if it should be rejected.
func (cb *CircuitBreaker) CanExecute() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	cb.lastRequestTime = now
	cb.totalRequests++

	switch cb.state {
	case CircuitStateClosed:
		// Check if we need to reset failure count due to timeout
		if cb.config.ResetTimeout > 0 && now.Sub(cb.lastFailTime) > cb.config.ResetTimeout {
			cb.resetFailureCount()
		}
		return true

	case CircuitStateOpen:
		// Check if recovery timeout has passed
		if now.Sub(cb.lastStateChange) >= cb.config.RecoveryTimeout {
			cb.transitionToHalfOpen()
			cb.halfOpenRequestCount++
			return true
		}
		return false

	case CircuitStateHalfOpen:
		// Allow limited requests to test recovery
		if cb.halfOpenRequestCount < cb.config.HalfOpenMaxRequests {
			cb.halfOpenRequestCount++
			return true
		}
		return false

	default:
		return false
	}
}

// RecordSuccess records a successful operation and updates circuit breaker state.
// This method handles state transitions based on success patterns.
//
// State transition logic:
//   - CLOSED: Reset consecutive failures, update success metrics
//   - HALF_OPEN: Increment success count, close circuit if threshold reached
//   - OPEN: No action (shouldn't happen as requests are blocked)
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalSuccesses++
	cb.consecutiveFailures = 0 // Reset consecutive failure count

	switch cb.state {
	case CircuitStateClosed:
		// Normal operation, just record the success
		// But also check if failure rate threshold is exceeded
		if cb.shouldOpenCircuit() {
			cb.transitionToOpen()
		}
	case CircuitStateHalfOpen:
		cb.successCount++
		// Check if we have enough consecutive successes to close the circuit
		if cb.successCount >= cb.config.ConsecutiveSuccesses {
			cb.transitionToClosed()
		}

	case CircuitStateOpen:
		// This shouldn't happen as requests should be blocked
		// But handle gracefully by transitioning to half-open
		cb.transitionToHalfOpen()
	}
}

// RecordFailure records a failed operation and updates circuit breaker state.
// This method handles state transitions based on failure patterns.
//
// State transition logic:
//   - CLOSED: Increment failure count, open circuit if threshold reached
//   - HALF_OPEN: Open circuit immediately on any failure during testing
//   - OPEN: Update failure metrics but remain open
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	cb.failureCount++
	cb.totalFailures++
	cb.consecutiveFailures++
	cb.lastFailTime = now

	switch cb.state {
	case CircuitStateClosed:
		// Check if we should open the circuit
		if cb.shouldOpenCircuit() {
			cb.transitionToOpen()
		}

	case CircuitStateHalfOpen:
		// Any failure in half-open state should open the circuit
		cb.transitionToOpen()

	case CircuitStateOpen:
		// Already open, just record the failure
		// This helps with metrics and adaptive thresholds
	}
}

// GetState returns the current circuit breaker state
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetFailureCount returns the current failure count
func (cb *CircuitBreaker) GetFailureCount() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failureCount
}

// GetMetrics returns comprehensive circuit breaker metrics
func (cb *CircuitBreaker) GetMetrics() CircuitBreakerMetrics {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	now := time.Now()

	// Calculate failure rate
	failureRate := 0.0
	if cb.totalRequests > 0 {
		failureRate = float64(cb.totalFailures) / float64(cb.totalRequests)
	}

	// Calculate request rate (requests per second)
	requestRate := 0.0
	if !cb.lastRequestTime.IsZero() {
		duration := now.Sub(cb.lastStateChange)
		if duration > 0 {
			requestRate = float64(cb.totalRequests) / duration.Seconds()
		}
	}

	// Estimate recovery time for open circuits
	estimatedRecoveryTime := time.Time{}
	if cb.state == CircuitStateOpen {
		estimatedRecoveryTime = cb.lastStateChange.Add(cb.config.RecoveryTimeout)
	}

	// Copy state transitions map
	stateTransitions := make(map[CircuitState]int64)
	for k, v := range cb.stateTransitions {
		stateTransitions[k] = v
	}

	return CircuitBreakerMetrics{
		State:                 cb.state,
		FailureCount:          cb.failureCount,
		SuccessCount:          cb.successCount,
		HalfOpenRequestCount:  cb.halfOpenRequestCount,
		ConsecutiveFailures:   cb.consecutiveFailures,
		TotalRequests:         cb.totalRequests,
		TotalFailures:         cb.totalFailures,
		TotalSuccesses:        cb.totalSuccesses,
		LastFailTime:          cb.lastFailTime,
		LastStateChange:       cb.lastStateChange,
		LastRequestTime:       cb.lastRequestTime,
		StateTransitions:      stateTransitions,
		FailureRate:           failureRate,
		RequestRate:           requestRate,
		TimeInCurrentState:    now.Sub(cb.lastStateChange),
		IsHealthy:             cb.isHealthyInternal(),
		EstimatedRecoveryTime: estimatedRecoveryTime,
	}
}

// Reset resets the circuit breaker to its initial state with all counters cleared
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = CircuitStateClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.halfOpenRequestCount = 0
	cb.consecutiveFailures = 0
	cb.totalRequests = 0
	cb.totalFailures = 0
	cb.totalSuccesses = 0
	cb.lastStateChange = time.Now()
	cb.lastFailTime = time.Time{}
	cb.lastRequestTime = time.Time{}

	// Reset state transitions but keep the map
	for k := range cb.stateTransitions {
		cb.stateTransitions[k] = 0
	}
}

// IsHealthy returns true if the circuit breaker indicates the service is healthy
func (cb *CircuitBreaker) IsHealthy() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.isHealthyInternal()
}

// Internal methods (must be called with lock held)

func (cb *CircuitBreaker) isHealthyInternal() bool {
	switch cb.state {
	case CircuitStateClosed:
		// Healthy if failure rate is acceptable
		if cb.totalRequests >= int64(cb.config.MinRequestsThreshold) {
			failureRate := float64(cb.totalFailures) / float64(cb.totalRequests)
			return failureRate < 0.5 // Less than 50% failure rate
		}
		return cb.consecutiveFailures < cb.config.FailureThreshold/2

	case CircuitStateHalfOpen:
		// Healthy if making progress in recovery testing
		return cb.successCount > 0

	case CircuitStateOpen:
		// Not healthy when circuit is open
		return false

	default:
		return false
	}
}

func (cb *CircuitBreaker) shouldOpenCircuit() bool {
	// Check failure count threshold
	if cb.failureCount >= cb.config.FailureThreshold {
		return true
	}

	// Check failure rate if configured and we have enough requests
	if cb.config.FailureRate > 0 && cb.totalRequests >= int64(cb.config.MinRequestsThreshold) {
		failureRate := float64(cb.totalFailures) / float64(cb.totalRequests)
		if failureRate >= cb.config.FailureRate {
			return true
		}
	}

	// Check adaptive thresholds if enabled
	if cb.config.AdaptiveThresholds && cb.shouldOpenWithAdaptiveThreshold() {
		return true
	}

	return false
}

func (cb *CircuitBreaker) shouldOpenWithAdaptiveThreshold() bool {
	// Implement adaptive threshold logic based on request volume and patterns
	if cb.totalRequests < int64(cb.config.RequestVolumeThreshold) {
		return false // Not enough data for adaptive behavior
	}

	// Lower threshold for high-volume scenarios
	adjustedThreshold := cb.config.FailureThreshold
	if cb.totalRequests > int64(cb.config.RequestVolumeThreshold*2) {
		adjustedThreshold = adjustedThreshold * 2 / 3 // Reduce threshold by 33%
	}

	// Consider recent failure concentration
	recentFailureRate := float64(cb.consecutiveFailures) / float64(adjustedThreshold)
	return recentFailureRate >= 0.8 // 80% of threshold reached
}

func (cb *CircuitBreaker) transitionToOpen() {
	if cb.state != CircuitStateOpen {
		cb.state = CircuitStateOpen
		cb.lastStateChange = time.Now()
		cb.stateTransitions[CircuitStateOpen]++
	}
}

func (cb *CircuitBreaker) transitionToClosed() {
	if cb.state != CircuitStateClosed {
		cb.state = CircuitStateClosed
		cb.lastStateChange = time.Now()
		cb.resetFailureCount()
		cb.successCount = 0
		cb.halfOpenRequestCount = 0
		cb.stateTransitions[CircuitStateClosed]++
	}
}

func (cb *CircuitBreaker) transitionToHalfOpen() {
	if cb.state != CircuitStateHalfOpen {
		cb.state = CircuitStateHalfOpen
		cb.lastStateChange = time.Now()
		cb.successCount = 0
		cb.halfOpenRequestCount = 0
		cb.stateTransitions[CircuitStateHalfOpen]++
	}
}

func (cb *CircuitBreaker) resetFailureCount() {
	cb.failureCount = 0
	cb.consecutiveFailures = 0
}

// Advanced features

// UpdateConfig allows dynamic configuration updates
func (cb *CircuitBreaker) UpdateConfig(config *CircuitBreakerConfig) {
	if config == nil {
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Update configuration while preserving current state
	cb.config = config

	// Ensure required defaults
	if cb.config.ConsecutiveSuccesses == 0 {
		cb.config.ConsecutiveSuccesses = 3
	}
	if cb.config.HalfOpenMaxRequests == 0 {
		cb.config.HalfOpenMaxRequests = 2
	}
}

// ForceOpen forces the circuit breaker into the open state
func (cb *CircuitBreaker) ForceOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.transitionToOpen()
}

// ForceClose forces the circuit breaker into the closed state
func (cb *CircuitBreaker) ForceClose() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.transitionToClosed()
}

// GetStateHistory returns a summary of state transitions over time
func (cb *CircuitBreaker) GetStateHistory() map[CircuitState]int64 {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// Return a copy to prevent external modification
	history := make(map[CircuitState]int64)
	for k, v := range cb.stateTransitions {
		history[k] = v
	}

	return history
}

// GetHealthScore returns a health score from 0.0 (unhealthy) to 1.0 (healthy)
func (cb *CircuitBreaker) GetHealthScore() float64 {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case CircuitStateClosed:
		if cb.totalRequests == 0 {
			return 1.0 // No data, assume healthy
		}
		// Base score on success rate
		successRate := float64(cb.totalSuccesses) / float64(cb.totalRequests)
		// Penalize consecutive failures
		failurePenalty := float64(cb.consecutiveFailures) / float64(cb.config.FailureThreshold)
		return successRate * (1.0 - failurePenalty*0.5)

	case CircuitStateHalfOpen:
		// Partial credit for attempting recovery
		if cb.successCount == 0 {
			return 0.3
		}
		return 0.3 + 0.4*float64(cb.successCount)/float64(cb.config.ConsecutiveSuccesses)

	case CircuitStateOpen:
		return 0.0

	default:
		return 0.0
	}
}
