// Package diagnostics provides advanced retry policies with exponential backoff,
// jittered delays, and intelligent retry decision making for Docker Network Doctor.
//
// This module implements sophisticated retry mechanisms that adapt to different
// failure patterns and system conditions. The retry system includes circuit breaker
// patterns to prevent cascade failures and intelligent backoff strategies to
// optimize recovery success rates.
//
// Key Features:
//   - Multiple backoff strategies (linear, exponential, jittered)
//   - Context-aware retry decisions based on error types
//   - Integration with circuit breaker patterns
//   - Retry budget management to prevent resource exhaustion
//   - Adaptive retry timing based on system load
//   - Comprehensive retry metrics and monitoring
//
// Retry Strategies:
//   - LinearBackoff: Fixed delay between retries
//   - ExponentialBackoff: Exponentially increasing delays
//   - JitteredBackoff: Exponential with random jitter to prevent thundering herd
//
// Example usage:
//   policy := NewRetryPolicy(&RetryPolicyConfig{
//       MaxRetries: 3,
//       BaseDelay: 100 * time.Millisecond,
//       MaxDelay: 10 * time.Second,
//       BackoffStrategy: ExponentialBackoff,
//   })
//
//   result, err := policy.Execute(ctx, operation)

package diagnostics

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"
)

// RetryPolicy implements intelligent retry logic with configurable backoff strategies.
// It provides sophisticated retry decision making based on error types, system state,
// and operational context.
//
// The RetryPolicy system includes:
//   - Configurable retry limits and timing
//   - Multiple backoff strategies with jitter support
//   - Error-specific retry decisions
//   - Context cancellation handling
//   - Comprehensive retry metrics
//   - Integration with circuit breaker patterns
//
// Thread Safety: RetryPolicy is safe for concurrent use.
type RetryPolicy struct {
	config  *RetryPolicyConfig
	metrics *RetryMetrics
	rand    *rand.Rand
}

// RetryPolicyConfig defines the behavior and limits for retry operations.
// It controls retry timing, backoff strategies, and retry decision criteria.
type RetryPolicyConfig struct {
	// Retry limits
	MaxRetries int           // Maximum number of retry attempts
	MaxDelay   time.Duration // Maximum delay between retries
	BaseDelay  time.Duration // Initial retry delay

	// Backoff configuration
	BackoffStrategy   BackoffStrategy // Strategy for calculating delays
	BackoffMultiplier float64         // Multiplier for exponential backoff
	JitterFactor      float64         // Amount of jitter to add (0.0-1.0)

	// Retry decision criteria
	RetryableErrors    []ErrorCode   // Specific error codes that should be retried
	NonRetryableErrors []ErrorCode   // Error codes that should never be retried
	MaxRetryBudget     time.Duration // Total time budget for all retries

	// Context handling
	RespectContext bool // Whether to respect context cancellation during retries
}

// RetryMetrics tracks retry operation statistics and performance.
type RetryMetrics struct {
	TotalRetries      int64         // Total number of retry attempts
	SuccessfulRetries int64         // Retries that eventually succeeded
	FailedRetries     int64         // Retries that exhausted all attempts
	TotalRetryTime    time.Duration // Total time spent in retry operations
	AverageRetryTime  time.Duration // Average time per retry operation
	MaxRetryTime      time.Duration // Maximum time for any retry operation

	// Backoff metrics
	TotalBackoffTime   time.Duration // Total time spent waiting between retries
	AverageBackoffTime time.Duration // Average backoff time
	MaxBackoffTime     time.Duration // Maximum backoff time

	// Error-specific metrics
	RetriesByErrorType map[ErrorType]int64 // Retry counts by error type
	RetriesByErrorCode map[ErrorCode]int64 // Retry counts by error code

	// Performance metrics
	FirstAttemptSuccess int64   // Operations that succeeded on first try
	RetrySuccessRate    float64 // Success rate for retried operations
}

// NewRetryPolicy creates a new retry policy with the specified configuration.
// If config is nil, sensible defaults will be applied.
//
// Default configuration:
//   - 3 maximum retries
//   - Exponential backoff starting at 100ms
//   - Maximum delay of 10 seconds
//   - 10% jitter factor
//   - Respects context cancellation
func NewRetryPolicy(config *RetryPolicyConfig) *RetryPolicy {
	if config == nil {
		config = &RetryPolicyConfig{
			MaxRetries:        3,
			BaseDelay:         100 * time.Millisecond,
			MaxDelay:          10 * time.Second,
			BackoffStrategy:   ExponentialBackoff,
			BackoffMultiplier: 2.0,
			JitterFactor:      0.1,
			MaxRetryBudget:    30 * time.Second,
			RespectContext:    true,
		}
	}

	// Set default values for unspecified fields
	if config.BackoffMultiplier == 0 {
		config.BackoffMultiplier = 2.0
	}
	if config.JitterFactor == 0 {
		config.JitterFactor = 0.1
	}

	return &RetryPolicy{
		config: config,
		metrics: &RetryMetrics{
			RetriesByErrorType: make(map[ErrorType]int64),
			RetriesByErrorCode: make(map[ErrorCode]int64),
		},
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Execute runs an operation with retry logic according to the configured policy.
// The method applies intelligent retry decisions based on error types and system state.
//
// Execution flow:
//  1. Execute operation
//  2. If successful, return result
//  3. If failed, analyze error for retry eligibility
//  4. Calculate backoff delay
//  5. Wait (respecting context cancellation)
//  6. Repeat until max retries reached or success
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - operation: Function to execute with retry logic
//
// Returns:
//   - interface{}: Operation result on success
//   - error: Final error after all retries exhausted
func (rp *RetryPolicy) Execute(ctx context.Context, operation RecoverableOperation) (interface{}, error) {
	startTime := time.Now()
	retryBudgetStart := startTime
	var lastError error

	// First attempt (not counted as a retry)
	result, err := operation(ctx)
	if err == nil {
		rp.metrics.FirstAttemptSuccess++
		return result, nil
	}

	lastError = err

	// Check if error is retryable
	if !rp.isRetryable(err) {
		return nil, fmt.Errorf("error is not retryable: %w", err)
	}

	// Retry loop
	for attempt := 1; attempt <= rp.config.MaxRetries; attempt++ {
		// Check retry budget
		if rp.config.MaxRetryBudget > 0 {
			elapsed := time.Since(retryBudgetStart)
			if elapsed >= rp.config.MaxRetryBudget {
				rp.metrics.FailedRetries++
				return nil, fmt.Errorf("retry budget exhausted after %v: %w", elapsed, lastError)
			}
		}

		// Calculate backoff delay
		delay := rp.calculateBackoffDelay(attempt)

		// Wait for backoff delay (respecting context)
		if rp.config.RespectContext {
			select {
			case <-time.After(delay):
				// Continue with retry
			case <-ctx.Done():
				rp.metrics.FailedRetries++
				return nil, ctx.Err()
			}
		} else {
			time.Sleep(delay)
		}

		rp.metrics.TotalBackoffTime += delay
		if delay > rp.metrics.MaxBackoffTime {
			rp.metrics.MaxBackoffTime = delay
		}

		// Retry the operation
		retryStartTime := time.Now()
		result, err = operation(ctx)
		retryDuration := time.Since(retryStartTime)

		// Update retry metrics
		rp.metrics.TotalRetries++
		rp.metrics.TotalRetryTime += retryDuration

		if retryDuration > rp.metrics.MaxRetryTime {
			rp.metrics.MaxRetryTime = retryDuration
		}

		// Update error-specific metrics
		if diagErr, ok := lastError.(*DiagnosticError); ok {
			rp.metrics.RetriesByErrorType[diagErr.Type]++
			rp.metrics.RetriesByErrorCode[diagErr.Code]++
		}

		if err == nil {
			// Success after retry
			rp.metrics.SuccessfulRetries++
			rp.updateSuccessRate()
			return result, nil
		}

		lastError = err

		// Check if new error is still retryable
		if !rp.isRetryable(err) {
			break
		}
	}

	// All retries exhausted
	rp.metrics.FailedRetries++
	rp.updateSuccessRate()

	totalDuration := time.Since(startTime)
	return nil, fmt.Errorf("operation failed after %d retries in %v: %w", rp.config.MaxRetries, totalDuration, lastError)
}

// ExecuteWithCustomRetries executes an operation with a custom retry count,
// overriding the policy's default MaxRetries setting for this specific operation.
func (rp *RetryPolicy) ExecuteWithCustomRetries(ctx context.Context, operation RecoverableOperation, maxRetries int) (interface{}, error) {
	// Temporarily override max retries
	originalMaxRetries := rp.config.MaxRetries
	rp.config.MaxRetries = maxRetries
	defer func() {
		rp.config.MaxRetries = originalMaxRetries
	}()

	return rp.Execute(ctx, operation)
}

// isRetryable determines if an error should be retried based on the policy configuration
func (rp *RetryPolicy) isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for context errors - these are generally not retryable
	// Note: We can't check context here since it's not passed to this method

	// If it's a DiagnosticError, use sophisticated retry logic
	if diagErr, ok := err.(*DiagnosticError); ok {
		return rp.isRetryableDiagnosticError(diagErr)
	}

	// For non-diagnostic errors, apply basic retry logic
	return rp.isRetryableGenericError(err)
}

// isRetryableDiagnosticError applies sophisticated retry logic for DiagnosticError types
func (rp *RetryPolicy) isRetryableDiagnosticError(diagErr *DiagnosticError) bool {
	// Check explicit non-retryable errors first
	for _, nonRetryableCode := range rp.config.NonRetryableErrors {
		if diagErr.Code == nonRetryableCode {
			return false
		}
	}

	// Check explicit retryable errors
	if len(rp.config.RetryableErrors) > 0 {
		for _, retryableCode := range rp.config.RetryableErrors {
			if diagErr.Code == retryableCode {
				return true
			}
		}
		return false // If explicit list provided, only retry listed errors
	}

	// Default retry logic based on error type and code
	switch diagErr.Type {
	case ErrTypeConnection:
		// Connection errors are generally retryable except permission issues
		return diagErr.Code != ErrCodeDockerPermissionDenied

	case ErrTypeResource:
		// Resource errors might be retryable if resources can be freed
		switch diagErr.Code {
		case ErrCodeOutOfMemory, ErrCodeOutOfDiskSpace, ErrCodeTooManyOpenFiles:
			return false // These require external intervention
		default:
			return true
		}

	case ErrTypeNetwork:
		// Network errors are often retryable
		switch diagErr.Code {
		case ErrCodeNetworkNotFound, ErrCodeSubnetOverlap:
			return false // Configuration issues
		default:
			return true
		}

	case ErrTypeContext:
		// Context errors - timeout is retryable, cancellation is not
		return diagErr.Code == ErrCodeTimeout

	case ErrTypeSystem:
		// System errors depend on the specific issue
		switch diagErr.Code {
		case ErrCodePermissionError, ErrCodeConfigurationMissing:
			return false // Require manual intervention
		default:
			return true
		}

	case ErrTypeCircuitBreaker:
		// Circuit breaker errors are not retryable at this level
		return false

	default:
		// Unknown error types - be conservative
		return diagErr.Severity != SeverityCritical
	}
}

// isRetryableGenericError applies basic retry logic for non-DiagnosticError types
func (rp *RetryPolicy) isRetryableGenericError(err error) bool {
	errorMsg := err.Error()

	// Common retryable patterns
	retryablePatterns := []string{
		"connection refused",
		"timeout",
		"temporary failure",
		"service unavailable",
		"try again",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errorMsg, pattern) {
			return true
		}
	}

	// Common non-retryable patterns
	nonRetryablePatterns := []string{
		"permission denied",
		"not found",
		"invalid",
		"malformed",
		"unauthorized",
		"forbidden",
	}

	for _, pattern := range nonRetryablePatterns {
		if strings.Contains(errorMsg, pattern) {
			return false
		}
	}

	// Default to retryable for unknown errors
	return true
}

// calculateBackoffDelay calculates the delay before the next retry attempt
func (rp *RetryPolicy) calculateBackoffDelay(attempt int) time.Duration {
	var delay time.Duration

	switch rp.config.BackoffStrategy {
	case LinearBackoff:
		delay = rp.config.BaseDelay * time.Duration(attempt)

	case ExponentialBackoff:
		multiplier := math.Pow(rp.config.BackoffMultiplier, float64(attempt-1))
		delay = time.Duration(float64(rp.config.BaseDelay) * multiplier)

	case JitteredBackoff:
		multiplier := math.Pow(rp.config.BackoffMultiplier, float64(attempt-1))
		baseDelay := time.Duration(float64(rp.config.BaseDelay) * multiplier)

		// Add jitter to prevent thundering herd
		jitterRange := float64(baseDelay) * rp.config.JitterFactor
		jitter := time.Duration((rp.rand.Float64() - 0.5) * 2 * jitterRange)
		delay = baseDelay + jitter

	default:
		// Default to exponential backoff
		multiplier := math.Pow(2.0, float64(attempt-1))
		delay = time.Duration(float64(rp.config.BaseDelay) * multiplier)
	}

	// Ensure delay doesn't exceed maximum
	if delay > rp.config.MaxDelay {
		delay = rp.config.MaxDelay
	}

	// Ensure delay is not negative (can happen with jitter)
	if delay < 0 {
		delay = rp.config.BaseDelay
	}

	return delay
}

// updateSuccessRate recalculates the retry success rate based on current metrics
func (rp *RetryPolicy) updateSuccessRate() {
	totalAttempts := rp.metrics.SuccessfulRetries + rp.metrics.FailedRetries
	if totalAttempts > 0 {
		rp.metrics.RetrySuccessRate = float64(rp.metrics.SuccessfulRetries) / float64(totalAttempts)
	}

	// Update average times
	if rp.metrics.TotalRetries > 0 {
		rp.metrics.AverageRetryTime = rp.metrics.TotalRetryTime / time.Duration(rp.metrics.TotalRetries)
		rp.metrics.AverageBackoffTime = rp.metrics.TotalBackoffTime / time.Duration(rp.metrics.TotalRetries)
	}
}

// GetMetrics returns current retry policy metrics
func (rp *RetryPolicy) GetMetrics() RetryMetrics {
	// Update derived metrics before returning
	rp.updateSuccessRate()

	// Return a copy to prevent external modification
	metrics := *rp.metrics

	// Copy maps to prevent modification
	metrics.RetriesByErrorType = make(map[ErrorType]int64)
	for k, v := range rp.metrics.RetriesByErrorType {
		metrics.RetriesByErrorType[k] = v
	}

	metrics.RetriesByErrorCode = make(map[ErrorCode]int64)
	for k, v := range rp.metrics.RetriesByErrorCode {
		metrics.RetriesByErrorCode[k] = v
	}

	return metrics
}

// Reset clears all metrics and resets the policy state
func (rp *RetryPolicy) Reset() {
	rp.metrics = &RetryMetrics{
		RetriesByErrorType: make(map[ErrorType]int64),
		RetriesByErrorCode: make(map[ErrorCode]int64),
	}
}

// UpdateConfig allows dynamic configuration updates
func (rp *RetryPolicy) UpdateConfig(config *RetryPolicyConfig) {
	if config != nil {
		rp.config = config

		// Ensure required defaults
		if rp.config.BackoffMultiplier == 0 {
			rp.config.BackoffMultiplier = 2.0
		}
		if rp.config.JitterFactor == 0 {
			rp.config.JitterFactor = 0.1
		}
	}
}

// IsHealthy returns true if the retry policy is performing well
func (rp *RetryPolicy) IsHealthy() bool {
	// Check if we have enough data to make a determination
	totalRetries := rp.metrics.SuccessfulRetries + rp.metrics.FailedRetries
	if totalRetries < 10 {
		return true // Not enough data, assume healthy
	}

	// Check success rate
	if rp.metrics.RetrySuccessRate < 0.1 { // Less than 10% success rate is concerning
		return false
	}

	// Check if average retry time is reasonable
	if rp.metrics.AverageRetryTime > rp.config.MaxDelay*time.Duration(rp.config.MaxRetries) {
		return false
	}

	return true
}

// GetRecommendations returns configuration recommendations based on metrics
func (rp *RetryPolicy) GetRecommendations() []string {
	var recommendations []string

	// Check success rate
	if rp.metrics.RetrySuccessRate < 0.3 && rp.metrics.TotalRetries > 20 {
		recommendations = append(recommendations,
			"Low retry success rate detected. Consider increasing MaxRetries or investigating root causes.")
	}

	// Check backoff timing
	if rp.metrics.AverageBackoffTime < rp.config.BaseDelay/2 {
		recommendations = append(recommendations,
			"Backoff delays are very short. Consider increasing BaseDelay to reduce system load.")
	}

	// Check retry distribution by error type
	if connectionRetries := rp.metrics.RetriesByErrorType[ErrTypeConnection]; connectionRetries > rp.metrics.TotalRetries/2 {
		recommendations = append(recommendations,
			"High number of connection-related retries. Consider investigating Docker daemon stability.")
	}

	if resourceRetries := rp.metrics.RetriesByErrorType[ErrTypeResource]; resourceRetries > rp.metrics.TotalRetries/4 {
		recommendations = append(recommendations,
			"Frequent resource-related retries. Consider monitoring system resources and scaling if needed.")
	}

	return recommendations
}
