// Package diagnostics provides structured error types and error handling utilities
// for comprehensive error recovery in Docker Network Doctor.
//
// This module defines a hierarchy of error types that enable intelligent error
// classification, recovery strategy selection, and actionable user guidance.
// Each error type includes context information, severity levels, and specific
// recovery recommendations.
//
// Error Type Hierarchy:
//   - DiagnosticError: Base structured error with recovery guidance
//   - ConnectionError: Docker daemon and network connectivity issues
//   - ResourceError: System resource exhaustion and conflicts
//   - NetworkError: Docker network configuration and routing problems
//   - SystemError: Host system and configuration issues
//   - ContextError: Timeout, cancellation, and context-related errors
//
// Error Codes:
//   Each error type has specific error codes that enable programmatic
//   error handling and automated recovery strategy selection.

package diagnostics

import (
	"fmt"
	"time"
)

// DiagnosticError represents a structured error with comprehensive context,
// recovery guidance, and classification information for intelligent error handling.
//
// DiagnosticError provides:
//   - Structured error codes for programmatic handling
//   - Severity levels for prioritization
//   - Context information for debugging
//   - Recovery strategies with actionable steps
//   - Original error chain preservation
//   - Recovery attempt tracking
type DiagnosticError struct {
	// Error identification
	Code     ErrorCode `json:"code"`
	Type     ErrorType `json:"type"`
	Message  string    `json:"message"`
	Original error     `json:"-"` // Original error (not serialized)

	// Classification and priority
	Severity ErrorSeverity `json:"severity"`
	Category string        `json:"category,omitempty"`

	// Context and debugging information
	Context   map[string]interface{} `json:"context,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Source    string                 `json:"source,omitempty"` // Source component/check

	// Recovery information
	Recovery          *RecoveryStrategy `json:"recovery,omitempty"`
	RecoveryAttempted bool              `json:"recovery_attempted"`
	RecoveryError     string            `json:"recovery_error,omitempty"`

	// Additional metadata
	Tags       []string `json:"tags,omitempty"`
	UserAction string   `json:"user_action,omitempty"`
	Reference  string   `json:"reference,omitempty"` // Documentation reference
}

// ErrorCode provides specific error identification for programmatic handling
type ErrorCode int

const (
	// Generic errors
	ErrCodeGeneric ErrorCode = iota
	ErrCodeUnknown
	ErrCodeInternal

	// Connection errors (1000-1099)
	ErrCodeDockerDaemonUnreachable = 1000 + iota - 3
	ErrCodeDockerAPIVersionMismatch
	ErrCodeDockerPermissionDenied
	ErrCodeDockerSocketNotFound
	ErrCodeDockerDaemonNotRunning

	// Resource errors (1100-1199)
	ErrCodeResourceExhaustion = 1100 + iota - 8
	ErrCodeOutOfMemory
	ErrCodeOutOfDiskSpace
	ErrCodeTooManyOpenFiles
	ErrCodeResourceConflict
	ErrCodeContainerLimitReached

	// Network errors (1200-1299)
	ErrCodeNetworkConfiguration = 1200 + iota - 14
	ErrCodeNetworkNotFound
	ErrCodeNetworkConflict
	ErrCodeSubnetOverlap
	ErrCodeDNSResolutionFailed
	ErrCodePortConflict
	ErrCodeBridgeConfigError
	ErrCodeIPForwardingDisabled
	ErrCodeIptablesConfigError
	ErrCodeMTUMismatch

	// System errors (1300-1399)
	ErrCodeSystemConfiguration = 1300 + iota - 24
	ErrCodePermissionError
	ErrCodeServiceNotRunning
	ErrCodeConfigurationMissing
	ErrCodeDependencyMissing

	// Context errors (1400-1499)
	ErrCodeTimeout = 1400 + iota - 29
	ErrCodeCancelled
	ErrCodeDeadlineExceeded
	ErrCodeContextExpired

	// Circuit breaker errors (1500-1599)
	ErrCodeCircuitBreakerOpen = 1500 + iota - 33
	ErrCodeTooManyFailures
	ErrCodeServiceUnavailable
)

// ErrorType categorizes errors for recovery strategy selection
type ErrorType int

const (
	ErrTypeGeneric ErrorType = iota
	ErrTypeConnection
	ErrTypeResource
	ErrTypeNetwork
	ErrTypeSystem
	ErrTypeContext
	ErrTypeCircuitBreaker
)

// ErrorSeverity indicates the criticality and impact of an error
// Note: This uses the same values as Severity from engine.go for compatibility
type ErrorSeverity = Severity

// RecoverableOperation represents an operation that can be executed with recovery
type RecoverableOperation func(ctx interface{}) (interface{}, error)

// RecoveryStrategy defines how to recover from a specific error condition
type RecoveryStrategy struct {
	// Strategy configuration
	RetryWithBackoff  bool    `json:"retry_with_backoff"`
	BackoffMultiplier float64 `json:"backoff_multiplier,omitempty"`
	MaxRetries        int     `json:"max_retries,omitempty"`

	// Recovery actions
	CleanupResources bool `json:"cleanup_resources"`
	GracefulDegrade  bool `json:"graceful_degrade"`
	NetworkRecovery  bool `json:"network_recovery"`

	// User guidance
	Immediate []string `json:"immediate,omitempty"` // Immediate actions to try
	LongTerm  []string `json:"long_term,omitempty"` // Long-term preventive actions
	Commands  []string `json:"commands,omitempty"`  // Specific commands to run

	// Automation
	AutoRecovery bool `json:"auto_recovery"` // Can be recovered automatically
	RequiresUser bool `json:"requires_user"` // Requires user intervention
}

// RecoveryMetrics tracks error recovery system performance and health
type RecoveryMetrics struct {
	// Operation counts
	TotalOperations      int64 `json:"total_operations"`
	TotalSuccesses       int64 `json:"total_successes"`
	TotalFailures        int64 `json:"total_failures"`
	SuccessfulRecoveries int64 `json:"successful_recoveries"`
	FailedRecoveries     int64 `json:"failed_recoveries"`

	// Timing metrics
	TotalExecutionTime   time.Duration `json:"total_execution_time"`
	AverageExecutionTime time.Duration `json:"average_execution_time"`
	MaxExecutionTime     time.Duration `json:"max_execution_time"`

	// Error distribution
	ErrorsByType     map[ErrorType]int64     `json:"errors_by_type"`
	ErrorsByCode     map[ErrorCode]int64     `json:"errors_by_code"`
	ErrorsBySeverity map[ErrorSeverity]int64 `json:"errors_by_severity"`

	// Circuit breaker metrics
	CircuitBreakerState    CircuitState `json:"circuit_breaker_state"`
	CircuitBreakerFailures int          `json:"circuit_breaker_failures"`
	CircuitBreakerBlocks   int64        `json:"circuit_breaker_blocks"`

	// Recovery health
	RecoveryRate       float64 `json:"recovery_rate"`
	HealthScore        float64 `json:"health_score"`
	DegradationEnabled bool    `json:"degradation_enabled"`
}

// Error implements the error interface for DiagnosticError
func (e *DiagnosticError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Original != nil {
		return e.Original.Error()
	}
	return fmt.Sprintf("diagnostic error (code: %d, type: %v)", e.Code, e.Type)
}

// Unwrap returns the original error for error chaining support
func (e *DiagnosticError) Unwrap() error {
	return e.Original
}

// GetSeverityString returns a human-readable severity level
func (e *DiagnosticError) GetSeverityString() string {
	switch e.Severity {
	case SeverityInfo:
		return "INFO"
	case SeverityWarning:
		return "WARNING"
	case SeverityError:
		return "ERROR"
	case SeverityCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// GetTypeString returns a human-readable error type
func (e *DiagnosticError) GetTypeString() string {
	switch e.Type {
	case ErrTypeConnection:
		return "Connection"
	case ErrTypeResource:
		return "Resource"
	case ErrTypeNetwork:
		return "Network"
	case ErrTypeSystem:
		return "System"
	case ErrTypeContext:
		return "Context"
	case ErrTypeCircuitBreaker:
		return "Circuit Breaker"
	default:
		return "Generic"
	}
}

// GetCodeString returns a human-readable error code description
func (e *DiagnosticError) GetCodeString() string {
	switch e.Code {
	// Connection errors
	case ErrCodeDockerDaemonUnreachable:
		return "Docker Daemon Unreachable"
	case ErrCodeDockerAPIVersionMismatch:
		return "Docker API Version Mismatch"
	case ErrCodeDockerPermissionDenied:
		return "Docker Permission Denied"
	case ErrCodeDockerSocketNotFound:
		return "Docker Socket Not Found"
	case ErrCodeDockerDaemonNotRunning:
		return "Docker Daemon Not Running"

	// Resource errors
	case ErrCodeResourceExhaustion:
		return "Resource Exhaustion"
	case ErrCodeOutOfMemory:
		return "Out of Memory"
	case ErrCodeOutOfDiskSpace:
		return "Out of Disk Space"
	case ErrCodeTooManyOpenFiles:
		return "Too Many Open Files"
	case ErrCodeResourceConflict:
		return "Resource Conflict"
	case ErrCodeContainerLimitReached:
		return "Container Limit Reached"

	// Network errors
	case ErrCodeNetworkConfiguration:
		return "Network Configuration Error"
	case ErrCodeNetworkNotFound:
		return "Network Not Found"
	case ErrCodeNetworkConflict:
		return "Network Conflict"
	case ErrCodeSubnetOverlap:
		return "Subnet Overlap"
	case ErrCodeDNSResolutionFailed:
		return "DNS Resolution Failed"
	case ErrCodePortConflict:
		return "Port Conflict"
	case ErrCodeBridgeConfigError:
		return "Bridge Configuration Error"
	case ErrCodeIPForwardingDisabled:
		return "IP Forwarding Disabled"
	case ErrCodeIptablesConfigError:
		return "Iptables Configuration Error"
	case ErrCodeMTUMismatch:
		return "MTU Mismatch"

	// System errors
	case ErrCodeSystemConfiguration:
		return "System Configuration Error"
	case ErrCodePermissionError:
		return "Permission Error"
	case ErrCodeServiceNotRunning:
		return "Service Not Running"
	case ErrCodeConfigurationMissing:
		return "Configuration Missing"
	case ErrCodeDependencyMissing:
		return "Dependency Missing"

	// Context errors
	case ErrCodeTimeout:
		return "Timeout"
	case ErrCodeCancelled:
		return "Cancelled"
	case ErrCodeDeadlineExceeded:
		return "Deadline Exceeded"
	case ErrCodeContextExpired:
		return "Context Expired"

	// Circuit breaker errors
	case ErrCodeCircuitBreakerOpen:
		return "Circuit Breaker Open"
	case ErrCodeTooManyFailures:
		return "Too Many Failures"
	case ErrCodeServiceUnavailable:
		return "Service Unavailable"

	default:
		return fmt.Sprintf("Unknown Error Code (%d)", e.Code)
	}
}

// IsRecoverable returns true if the error can potentially be recovered from
func (e *DiagnosticError) IsRecoverable() bool {
	switch e.Type {
	case ErrTypeConnection, ErrTypeResource, ErrTypeNetwork:
		return true
	case ErrTypeContext:
		return e.Code == ErrCodeTimeout // Timeouts are recoverable
	case ErrTypeSystem:
		return e.Code != ErrCodePermissionError // Most system errors except permissions
	case ErrTypeCircuitBreaker:
		return true // Circuit breaker errors are recoverable by design
	default:
		return false
	}
}

// RequiresUserIntervention returns true if the error requires manual intervention
func (e *DiagnosticError) RequiresUserIntervention() bool {
	switch e.Code {
	case ErrCodeDockerPermissionDenied, ErrCodePermissionError:
		return true
	case ErrCodeDockerDaemonNotRunning, ErrCodeServiceNotRunning:
		return true
	case ErrCodeConfigurationMissing, ErrCodeDependencyMissing:
		return true
	case ErrCodeSubnetOverlap, ErrCodePortConflict:
		return true
	default:
		return e.Severity == SeverityCritical
	}
}

// GetImmediateActions returns a list of immediate actions the user can take
func (e *DiagnosticError) GetImmediateActions() []string {
	if e.Recovery != nil && len(e.Recovery.Immediate) > 0 {
		return e.Recovery.Immediate
	}

	// Default actions based on error type
	switch e.Type {
	case ErrTypeConnection:
		return []string{
			"Check if Docker daemon is running",
			"Verify Docker installation and configuration",
			"Test Docker connectivity with 'docker version'",
		}
	case ErrTypeResource:
		return []string{
			"Free up system resources",
			"Check disk space and memory usage",
			"Stop unnecessary containers and services",
		}
	case ErrTypeNetwork:
		return []string{
			"Check network configuration",
			"Verify container network connectivity",
			"Review Docker network settings",
		}
	case ErrTypeSystem:
		return []string{
			"Check system configuration",
			"Verify required services are running",
			"Review system logs for errors",
		}
	default:
		return []string{
			"Retry the operation",
			"Check system health and connectivity",
		}
	}
}

// GetLongTermActions returns a list of long-term preventive actions
func (e *DiagnosticError) GetLongTermActions() []string {
	if e.Recovery != nil && len(e.Recovery.LongTerm) > 0 {
		return e.Recovery.LongTerm
	}

	// Default long-term actions based on error type
	switch e.Type {
	case ErrTypeConnection:
		return []string{
			"Monitor Docker daemon health",
			"Set up automatic service recovery",
			"Implement health checks for Docker services",
		}
	case ErrTypeResource:
		return []string{
			"Monitor resource usage trends",
			"Implement resource limits and quotas",
			"Plan for capacity scaling",
		}
	case ErrTypeNetwork:
		return []string{
			"Review network architecture",
			"Monitor network performance",
			"Implement network monitoring",
		}
	case ErrTypeSystem:
		return []string{
			"Regular system maintenance",
			"Monitor system health metrics",
			"Keep system and dependencies updated",
		}
	default:
		return []string{
			"Implement comprehensive monitoring",
			"Regular system health checks",
		}
	}
}

// ToMap converts the error to a map for structured logging or JSON serialization
func (e *DiagnosticError) ToMap() map[string]interface{} {
	result := map[string]interface{}{
		"code":      e.Code,
		"type":      e.GetTypeString(),
		"message":   e.Message,
		"severity":  e.GetSeverityString(),
		"timestamp": e.Timestamp,
	}

	if e.Category != "" {
		result["category"] = e.Category
	}

	if e.Source != "" {
		result["source"] = e.Source
	}

	if e.Context != nil {
		result["context"] = e.Context
	}

	if e.RecoveryAttempted {
		result["recovery_attempted"] = true
		if e.RecoveryError != "" {
			result["recovery_error"] = e.RecoveryError
		}
	}

	if len(e.Tags) > 0 {
		result["tags"] = e.Tags
	}

	if e.UserAction != "" {
		result["user_action"] = e.UserAction
	}

	if e.Reference != "" {
		result["reference"] = e.Reference
	}

	return result
}

// NewDiagnosticError creates a new DiagnosticError with sensible defaults
func NewDiagnosticError(code ErrorCode, message string) *DiagnosticError {
	errorType := getErrorTypeFromCode(code)
	severity := getSeverityFromCode(code)

	return &DiagnosticError{
		Code:      code,
		Type:      errorType,
		Message:   message,
		Severity:  severity,
		Timestamp: time.Now(),
		Context:   make(map[string]interface{}),
	}
}

// Helper functions

func getErrorTypeFromCode(code ErrorCode) ErrorType {
	switch {
	case code >= 1000 && code < 1100:
		return ErrTypeConnection
	case code >= 1100 && code < 1200:
		return ErrTypeResource
	case code >= 1200 && code < 1300:
		return ErrTypeNetwork
	case code >= 1300 && code < 1400:
		return ErrTypeSystem
	case code >= 1400 && code < 1500:
		return ErrTypeContext
	case code >= 1500 && code < 1600:
		return ErrTypeCircuitBreaker
	default:
		return ErrTypeGeneric
	}
}

func getSeverityFromCode(code ErrorCode) ErrorSeverity {
	switch code {
	case ErrCodeDockerDaemonUnreachable, ErrCodeDockerDaemonNotRunning:
		return SeverityCritical
	case ErrCodeResourceExhaustion, ErrCodeOutOfMemory, ErrCodeOutOfDiskSpace:
		return SeverityCritical
	case ErrCodePermissionError, ErrCodeDockerPermissionDenied:
		return SeverityError
	case ErrCodeTimeout, ErrCodeCancelled:
		return SeverityWarning
	case ErrCodeCircuitBreakerOpen:
		return SeverityError
	default:
		return SeverityWarning
	}
}
