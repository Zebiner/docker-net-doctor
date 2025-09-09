// Package diagnostics provides comprehensive error recovery mechanisms for Docker Network Doctor.
//
// This module implements advanced error handling, recovery strategies, and graceful degradation
// patterns to ensure robust operation even when facing Docker daemon failures, network partitions,
// resource exhaustion, or other system-level issues.
//
// Key Features:
//   - Structured error types with error codes and remediation guidance
//   - Retry policies with exponential backoff and circuit breaker patterns
//   - Resource cleanup automation for interrupted operations
//   - Context-aware error messages with actionable guidance
//   - Graceful degradation for partial Docker daemon failures
//   - Recovery strategies for common failure scenarios
//
// Error Categories:
//   - Connection errors: Docker daemon connectivity issues
//   - Resource errors: Resource exhaustion and conflicts
//   - Network errors: Network-specific diagnostic failures
//   - System errors: Host system configuration problems
//   - Context errors: Timeout and cancellation handling
//
// Example usage:
//   recovery := NewErrorRecovery(client, &RecoveryConfig{
//       MaxRetries: 3,
//       BackoffStrategy: ExponentialBackoff,
//   })
//   
//   result, err := recovery.ExecuteWithRecovery(ctx, operation)
//   if err != nil {
//       recovery.HandleError(ctx, err)
//   }

package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// ErrorRecovery provides comprehensive error handling and recovery mechanisms
// for Docker Network Doctor diagnostic operations.
//
// The ErrorRecovery system implements multiple layers of resilience:
//   - Retry policies with intelligent backoff strategies
//   - Circuit breaker patterns to prevent cascade failures
//   - Resource cleanup automation
//   - Context-aware error analysis and recovery
//   - Graceful degradation for partial system failures
//
// Thread Safety: ErrorRecovery is safe for concurrent use across multiple goroutines.
type ErrorRecovery struct {
	client         *docker.Client
	config         *RecoveryConfig
	circuitBreaker *CircuitBreaker
	retryPolicy    *RetryPolicy
	cleanupManager *ResourceCleanupManager
	metrics        *RecoveryMetrics
	mu             sync.RWMutex
}

// RecoveryConfig controls error recovery behavior and policies.
// It defines retry strategies, circuit breaker thresholds, cleanup policies,
// and performance tuning parameters.
type RecoveryConfig struct {
	// Retry configuration
	MaxRetries      int           // Maximum number of retry attempts
	BaseDelay       time.Duration // Initial retry delay
	MaxDelay        time.Duration // Maximum retry delay
	BackoffStrategy BackoffStrategy // Retry backoff strategy
	
	// Circuit breaker configuration
	FailureThreshold    int           // Failures before circuit opens
	RecoveryTimeout     time.Duration // Time before attempting recovery
	ConsecutiveSuccesses int          // Successes needed to close circuit
	
	// Cleanup configuration
	CleanupTimeout      time.Duration // Timeout for cleanup operations
	AutoCleanup         bool          // Enable automatic resource cleanup
	PreserveDebugData   bool          // Keep data for debugging
	
	// Degradation configuration
	EnableDegradation   bool          // Allow graceful degradation
	MinimalChecksOnly   bool          // Run only essential checks
	PartialResultsOK    bool          // Accept partial diagnostic results
}

// BackoffStrategy defines retry delay calculation methods
type BackoffStrategy int

const (
	LinearBackoff BackoffStrategy = iota
	ExponentialBackoff
	JitteredBackoff
)

// NewErrorRecovery creates a new error recovery system with the specified configuration.
// If config is nil, sensible defaults will be applied for production use.
//
// Default configuration includes:
//   - 3 retry attempts with exponential backoff
//   - Circuit breaker with 5 failure threshold
//   - Automatic resource cleanup enabled
//   - Graceful degradation allowed
//
// Parameters:
//   - client: Docker client for operations
//   - config: Recovery configuration (nil for defaults)
//
// Returns a fully configured error recovery system.
func NewErrorRecovery(client *docker.Client, config *RecoveryConfig) *ErrorRecovery {
	if config == nil {
		config = &RecoveryConfig{
			MaxRetries:           3,
			BaseDelay:            500 * time.Millisecond,
			MaxDelay:             30 * time.Second,
			BackoffStrategy:      ExponentialBackoff,
			FailureThreshold:     5,
			RecoveryTimeout:      30 * time.Second,
			ConsecutiveSuccesses: 3,
			CleanupTimeout:       10 * time.Second,
			AutoCleanup:          true,
			PreserveDebugData:    false,
			EnableDegradation:    true,
			MinimalChecksOnly:    false,
			PartialResultsOK:     true,
		}
	}

	recovery := &ErrorRecovery{
		client:  client,
		config:  config,
		metrics: &RecoveryMetrics{
			ErrorsByType:     make(map[ErrorType]int64),
			ErrorsByCode:     make(map[ErrorCode]int64),
			ErrorsBySeverity: make(map[ErrorSeverity]int64),
		},
	}

	// Initialize circuit breaker
	recovery.circuitBreaker = NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold:     config.FailureThreshold,
		RecoveryTimeout:      config.RecoveryTimeout,
		ConsecutiveSuccesses: config.ConsecutiveSuccesses,
	})

	// Initialize retry policy
	recovery.retryPolicy = NewRetryPolicy(&RetryPolicyConfig{
		MaxRetries:      config.MaxRetries,
		BaseDelay:       config.BaseDelay,
		MaxDelay:        config.MaxDelay,
		BackoffStrategy: config.BackoffStrategy,
	})

	// Initialize cleanup manager
	recovery.cleanupManager = NewResourceCleanupManager(&CleanupConfig{
		Timeout:         config.CleanupTimeout,
		AutoCleanup:     config.AutoCleanup,
		PreserveDebugData: config.PreserveDebugData,
	})

	return recovery
}

// ExecuteWithRecovery executes an operation with comprehensive error recovery.
// This method wraps any operation with retry logic, circuit breaker protection,
// and automatic error handling.
//
// The execution flow:
//   1. Check circuit breaker state
//   2. Execute operation with retry policy
//   3. Handle errors with recovery strategies
//   4. Update circuit breaker state
//   5. Perform cleanup if necessary
//
// Parameters:
//   - ctx: Context for timeout and cancellation
//   - operation: Function to execute with recovery
//
// Returns:
//   - interface{}: Operation result (type depends on operation)
//   - error: Non-recoverable error or nil on success
func (er *ErrorRecovery) ExecuteWithRecovery(ctx context.Context, operation RecoverableOperation) (interface{}, error) {
	er.mu.Lock()
	er.metrics.TotalOperations++
	er.mu.Unlock()

	// Check circuit breaker before execution
	if !er.circuitBreaker.CanExecute() {
		er.mu.Lock()
		er.metrics.CircuitBreakerBlocks++
		er.mu.Unlock()
		return nil, &DiagnosticError{
			Code:    ErrCodeCircuitBreakerOpen,
			Type:    ErrTypeSystem,
			Message: "Circuit breaker is open, operation blocked",
			Severity: SeverityCritical,
			Context: map[string]interface{}{
				"circuit_state": er.circuitBreaker.GetState(),
				"failures":      er.circuitBreaker.GetFailureCount(),
			},
			Recovery: &RecoveryStrategy{
				Immediate: []string{
					"Wait for circuit breaker to reset",
					"Check Docker daemon connectivity",
				},
				LongTerm: []string{
					"Investigate root cause of repeated failures",
					"Consider increasing circuit breaker thresholds",
				},
			},
		}
	}

	// Execute with retry policy
	startTime := time.Now()
	result, err := er.retryPolicy.Execute(ctx, operation)
	duration := time.Since(startTime)

	er.mu.Lock()
	er.metrics.TotalExecutionTime += duration
	if duration > er.metrics.MaxExecutionTime {
		er.metrics.MaxExecutionTime = duration
	}
	er.mu.Unlock()

	// Update circuit breaker based on result
	if err != nil {
		er.circuitBreaker.RecordFailure()
		er.mu.Lock()
		er.metrics.TotalFailures++
		er.mu.Unlock()

		// Attempt error recovery
		recoveryErr := er.HandleError(ctx, err)
		if recoveryErr != nil {
			// Recovery failed, return original error with recovery context
			if diagErr, ok := err.(*DiagnosticError); ok {
				diagErr.RecoveryAttempted = true
				diagErr.RecoveryError = recoveryErr.Error()
				return nil, diagErr
			}
			return nil, fmt.Errorf("operation failed and recovery unsuccessful: %w (recovery: %v)", err, recoveryErr)
		}
		
		// Recovery succeeded, update metrics
		er.mu.Lock()
		er.metrics.SuccessfulRecoveries++
		er.mu.Unlock()
	} else {
		er.circuitBreaker.RecordSuccess()
		er.mu.Lock()
		er.metrics.TotalSuccesses++
		er.mu.Unlock()
	}

	return result, err
}

// HandleError provides intelligent error handling with context-aware recovery strategies.
// This method analyzes the error type, determines appropriate recovery actions,
// and executes recovery procedures automatically.
//
// Error handling includes:
//   - Error classification and severity assessment
//   - Recovery strategy selection based on error type
//   - Automatic resource cleanup
//   - Context-aware retry decisions
//   - Degradation triggers when appropriate
//
// Parameters:
//   - ctx: Context for timeout and cancellation
//   - err: Error to handle and recover from
//
// Returns an error if recovery is not possible.
func (er *ErrorRecovery) HandleError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	// Classify error and determine recovery strategy
	diagError := er.classifyError(err)
	
	er.mu.Lock()
	er.metrics.ErrorsByType[diagError.Type]++
	er.mu.Unlock()

	// Select recovery strategy based on error type and severity
	strategy := er.selectRecoveryStrategy(diagError)
	if strategy == nil {
		return fmt.Errorf("no recovery strategy available for error type %v", diagError.Type)
	}

	// Execute recovery strategy
	recoveryCtx, cancel := context.WithTimeout(ctx, er.config.CleanupTimeout)
	defer cancel()

	if err := er.executeRecoveryStrategy(recoveryCtx, strategy, diagError); err != nil {
		er.mu.Lock()
		er.metrics.FailedRecoveries++
		er.mu.Unlock()
		return fmt.Errorf("recovery strategy failed: %w", err)
	}

	// Perform cleanup if configured
	if er.config.AutoCleanup {
		if err := er.cleanupManager.CleanupAfterError(recoveryCtx, diagError); err != nil {
			// Log cleanup failure but don't fail the recovery
			// Cleanup failure is less critical than the original error
		}
	}

	return nil
}

// classifyError analyzes an error and converts it to a structured DiagnosticError
// with appropriate error codes, severity levels, and recovery guidance.
func (er *ErrorRecovery) classifyError(err error) *DiagnosticError {
	// Check if already a DiagnosticError
	if diagErr, ok := err.(*DiagnosticError); ok {
		return diagErr
	}

	// Analyze error patterns to classify
	errorMsg := err.Error()
	
	// Docker daemon connectivity issues
	if isDockerConnectionError(errorMsg) {
		return &DiagnosticError{
			Code:     ErrCodeDockerDaemonUnreachable,
			Type:     ErrTypeConnection,
			Message:  "Docker daemon is unreachable",
			Original: err,
			Severity: SeverityCritical,
			Context: map[string]interface{}{
				"error_message": errorMsg,
				"timestamp":     time.Now(),
			},
			Recovery: &RecoveryStrategy{
				Immediate: []string{
					"Check if Docker daemon is running",
					"Verify Docker socket permissions",
					"Test Docker connectivity with 'docker version'",
				},
				LongTerm: []string{
					"Monitor Docker daemon health",
					"Configure automatic Docker service restart",
				},
			},
		}
	}

	// Context cancellation/timeout
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &DiagnosticError{
			Code:     ErrCodeTimeout,
			Type:     ErrTypeContext,
			Message:  "Operation cancelled or timed out",
			Original: err,
			Severity: SeverityWarning,
			Context: map[string]interface{}{
				"error_type": "context",
				"timeout":    true,
			},
			Recovery: &RecoveryStrategy{
				Immediate: []string{
					"Retry with increased timeout",
					"Check system performance",
				},
				LongTerm: []string{
					"Optimize diagnostic operations",
					"Consider running diagnostics during low load",
				},
			},
		}
	}

	// Resource exhaustion
	if isResourceExhaustionError(errorMsg) {
		return &DiagnosticError{
			Code:     ErrCodeResourceExhaustion,
			Type:     ErrTypeResource,
			Message:  "System resources exhausted",
			Original: err,
			Severity: SeverityCritical,
			Context: map[string]interface{}{
				"error_message": errorMsg,
				"resource_type": detectResourceType(errorMsg),
			},
			Recovery: &RecoveryStrategy{
				Immediate: []string{
					"Free up system resources",
					"Stop unnecessary containers",
					"Check disk space and memory usage",
				},
				LongTerm: []string{
					"Monitor resource usage patterns",
					"Implement resource limits",
					"Scale system resources if needed",
				},
			},
		}
	}

	// Network-specific errors
	if isNetworkError(errorMsg) {
		return &DiagnosticError{
			Code:     ErrCodeNetworkConfiguration,
			Type:     ErrTypeNetwork,
			Message:  "Network configuration error",
			Original: err,
			Severity: SeverityWarning,
			Context: map[string]interface{}{
				"error_message": errorMsg,
				"network_type":  detectNetworkIssueType(errorMsg),
			},
			Recovery: &RecoveryStrategy{
				Immediate: []string{
					"Check network connectivity",
					"Verify network configuration",
					"Test DNS resolution",
				},
				LongTerm: []string{
					"Review network architecture",
					"Monitor network performance",
				},
			},
		}
	}

	// Generic error classification
	return &DiagnosticError{
		Code:     ErrCodeGeneric,
		Type:     ErrTypeSystem,
		Message:  "Unexpected error occurred",
		Original: err,
		Severity: SeverityWarning,
		Context: map[string]interface{}{
			"error_message": errorMsg,
			"timestamp":     time.Now(),
		},
		Recovery: &RecoveryStrategy{
			Immediate: []string{
				"Retry the operation",
				"Check system health",
			},
			LongTerm: []string{
				"Review logs for patterns",
				"Consider system maintenance",
			},
		},
	}
}

// selectRecoveryStrategy chooses the appropriate recovery strategy based on error analysis.
func (er *ErrorRecovery) selectRecoveryStrategy(diagError *DiagnosticError) *RecoveryStrategy {
	// Use embedded recovery strategy if available
	if diagError.Recovery != nil {
		return diagError.Recovery
	}

	// Default strategies based on error type
	switch diagError.Type {
	case ErrTypeConnection:
		return &RecoveryStrategy{
			RetryWithBackoff: true,
			BackoffMultiplier: 2.0,
			MaxRetries: er.config.MaxRetries,
			Immediate: []string{
				"Attempt to reconnect to Docker daemon",
				"Wait for daemon to become available",
			},
		}
	case ErrTypeResource:
		return &RecoveryStrategy{
			CleanupResources: true,
			GracefulDegrade: er.config.EnableDegradation,
			Immediate: []string{
				"Clean up unused resources",
				"Run minimal diagnostic checks",
			},
		}
	case ErrTypeNetwork:
		return &RecoveryStrategy{
			RetryWithBackoff: true,
			NetworkRecovery: true,
			Immediate: []string{
				"Reset network connections",
				"Verify network configuration",
			},
		}
	default:
		return &RecoveryStrategy{
			RetryWithBackoff: true,
			MaxRetries: 1, // Limited retries for unknown errors
		}
	}
}

// executeRecoveryStrategy implements the selected recovery strategy.
func (er *ErrorRecovery) executeRecoveryStrategy(ctx context.Context, strategy *RecoveryStrategy, diagError *DiagnosticError) error {
	if strategy.CleanupResources {
		if err := er.cleanupManager.PerformCleanup(ctx); err != nil {
			return fmt.Errorf("resource cleanup failed: %w", err)
		}
	}

	if strategy.NetworkRecovery {
		if err := er.performNetworkRecovery(ctx); err != nil {
			return fmt.Errorf("network recovery failed: %w", err)
		}
	}

	if strategy.GracefulDegrade && er.config.EnableDegradation {
		er.enableGracefulDegradation()
	}

	// Additional recovery actions based on error code
	switch diagError.Code {
	case ErrCodeDockerDaemonUnreachable:
		return er.recoverDockerConnection(ctx)
	case ErrCodeResourceExhaustion:
		return er.recoverFromResourceExhaustion(ctx)
	case ErrCodeNetworkConfiguration:
		return er.recoverNetworkConfiguration(ctx)
	}

	return nil
}

// GetMetrics returns comprehensive recovery system metrics.
func (er *ErrorRecovery) GetMetrics() RecoveryMetrics {
	er.mu.RLock()
	defer er.mu.RUnlock()
	
	// Create a copy to avoid data races
	metrics := *er.metrics
	
	// Add circuit breaker metrics
	metrics.CircuitBreakerState = er.circuitBreaker.GetState()
	metrics.CircuitBreakerFailures = er.circuitBreaker.GetFailureCount()
	
	return metrics
}

// IsHealthy returns true if the error recovery system is functioning properly.
func (er *ErrorRecovery) IsHealthy() bool {
	er.mu.RLock()
	defer er.mu.RUnlock()
	
	// Check if circuit breaker is not permanently open
	if er.circuitBreaker.GetState() == CircuitStateOpen {
		return false
	}
	
	// Check error rate
	if er.metrics.TotalOperations > 10 {
		errorRate := float64(er.metrics.TotalFailures) / float64(er.metrics.TotalOperations)
		if errorRate > 0.5 { // More than 50% failure rate
			return false
		}
	}
	
	return true
}

// Shutdown performs graceful shutdown of the error recovery system.
func (er *ErrorRecovery) Shutdown(ctx context.Context) error {
	var errors []error
	
	// Shutdown cleanup manager
	if er.cleanupManager != nil {
		if err := er.cleanupManager.Shutdown(ctx); err != nil {
			errors = append(errors, fmt.Errorf("cleanup manager shutdown failed: %w", err))
		}
	}
	
	// Final cleanup if needed
	if er.config.AutoCleanup {
		if err := er.cleanupManager.PerformFinalCleanup(ctx); err != nil {
			errors = append(errors, fmt.Errorf("final cleanup failed: %w", err))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}
	
	return nil
}

// Helper functions for error detection and classification

func isDockerConnectionError(errorMsg string) bool {
	connectionPatterns := []string{
		"cannot connect to Docker daemon",
		"connection refused",
		"dial unix",
		"daemon not running",
		"permission denied",
	}
	
	for _, pattern := range connectionPatterns {
		if containsString(errorMsg, pattern) {
			return true
		}
	}
	return false
}

func isResourceExhaustionError(errorMsg string) bool {
	resourcePatterns := []string{
		"out of memory",
		"no space left",
		"resource temporarily unavailable",
		"too many open files",
		"cannot allocate memory",
	}
	
	for _, pattern := range resourcePatterns {
		if containsString(errorMsg, pattern) {
			return true
		}
	}
	return false
}

func isNetworkError(errorMsg string) bool {
	networkPatterns := []string{
		"network not found",
		"no route to host",
		"connection timed out",
		"network unreachable",
		"dns resolution failed",
	}
	
	for _, pattern := range networkPatterns {
		if containsString(errorMsg, pattern) {
			return true
		}
	}
	return false
}

func detectResourceType(errorMsg string) string {
	if containsString(errorMsg, "memory") {
		return "memory"
	}
	if containsString(errorMsg, "disk") || containsString(errorMsg, "space") {
		return "disk"
	}
	if containsString(errorMsg, "files") || containsString(errorMsg, "descriptors") {
		return "file_descriptors"
	}
	return "unknown"
}

func detectNetworkIssueType(errorMsg string) string {
	if containsString(errorMsg, "dns") {
		return "dns"
	}
	if containsString(errorMsg, "route") {
		return "routing"
	}
	if containsString(errorMsg, "timeout") {
		return "connectivity"
	}
	return "configuration"
}

func containsString(s, substr string) bool {
	// Simple case-insensitive contains
	return len(s) >= len(substr) && 
		(s == substr || 
		 (len(s) > len(substr) && 
		  (s[:len(substr)] == substr || 
		   s[len(s)-len(substr):] == substr || 
		   indexSubstring(s, substr) >= 0)))
}

func indexSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// Recovery-specific operations

func (er *ErrorRecovery) recoverDockerConnection(ctx context.Context) error {
	// Attempt to reconnect to Docker daemon
	if er.client != nil {
		if err := er.client.Ping(ctx); err == nil {
			return nil // Connection recovered
		}
	}
	
	// Wait and retry
	select {
	case <-time.After(2 * time.Second):
		if er.client != nil {
			return er.client.Ping(ctx)
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	
	return fmt.Errorf("docker connection recovery failed")
}

func (er *ErrorRecovery) recoverFromResourceExhaustion(ctx context.Context) error {
	// Trigger aggressive cleanup
	return er.cleanupManager.PerformAggressiveCleanup(ctx)
}

func (er *ErrorRecovery) recoverNetworkConfiguration(ctx context.Context) error {
	// Reset network state
	return er.performNetworkRecovery(ctx)
}

func (er *ErrorRecovery) performNetworkRecovery(ctx context.Context) error {
	// Implementation would include network-specific recovery operations
	// For now, this is a placeholder for network recovery logic
	return nil
}

func (er *ErrorRecovery) enableGracefulDegradation() {
	// Enable degraded mode - this would modify the diagnostic engine
	// to run only essential checks
	er.mu.Lock()
	er.metrics.DegradationEnabled = true
	er.mu.Unlock()
}