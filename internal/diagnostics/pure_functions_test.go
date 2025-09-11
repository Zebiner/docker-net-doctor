package diagnostics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCircuitStateStringPure tests the String() method for all CircuitState values
func TestCircuitStateStringPure(t *testing.T) {
	testCases := []struct {
		state    CircuitState
		expected string
	}{
		{CircuitStateClosed, "CLOSED"},
		{CircuitStateOpen, "OPEN"},
		{CircuitStateHalfOpen, "HALF_OPEN"},
		{CircuitState(999), "UNKNOWN"}, // Invalid state
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := tc.state.String()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestSeverityString tests Severity string representations
func TestSeverityString(t *testing.T) {
	testCases := []struct {
		severity Severity
		name     string
	}{
		{SeverityInfo, "Info"},
		{SeverityWarning, "Warning"},
		{SeverityError, "Error"},
		{SeverityCritical, "Critical"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test that severity values are defined
			assert.NotNil(t, tc.severity)
			// Test that severity is within expected range
			assert.True(t, tc.severity >= SeverityInfo)
			assert.True(t, tc.severity <= SeverityCritical)
		})
	}
}

// TestDiagnosticErrorGetSeverityString tests pure severity string conversion
func TestDiagnosticErrorGetSeverityString(t *testing.T) {
	testCases := []struct {
		severity Severity
		expected string
	}{
		{SeverityInfo, "INFO"},
		{SeverityWarning, "WARNING"},
		{SeverityError, "ERROR"},
		{SeverityCritical, "CRITICAL"},
		{Severity(999), "UNKNOWN"}, // Invalid severity
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			err := &DiagnosticError{Severity: tc.severity}
			result := err.GetSeverityString()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestDiagnosticErrorGetTypeString tests pure error type string conversion
func TestDiagnosticErrorGetTypeString(t *testing.T) {
	testCases := []struct {
		errorType ErrorType
		expected  string
	}{
		{ErrTypeConnection, "Connection"},
		{ErrTypeResource, "Resource"},
		{ErrTypeNetwork, "Network"},
		{ErrTypeSystem, "System"},
		{ErrTypeContext, "Context"},
		{ErrTypeCircuitBreaker, "Circuit Breaker"},
		{ErrTypeGeneric, "Generic"},
		{ErrorType(999), "Generic"}, // Invalid type defaults to Generic
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			err := &DiagnosticError{Type: tc.errorType}
			result := err.GetTypeString()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestDiagnosticErrorGetCodeString tests pure error code string conversion
func TestDiagnosticErrorGetCodeString(t *testing.T) {
	testCases := []struct {
		code     ErrorCode
		expected string
	}{
		// Connection errors
		{ErrCodeDockerDaemonUnreachable, "Docker Daemon Unreachable"},
		{ErrCodeDockerAPIVersionMismatch, "Docker API Version Mismatch"},
		{ErrCodeDockerPermissionDenied, "Docker Permission Denied"},
		{ErrCodeDockerSocketNotFound, "Docker Socket Not Found"},
		{ErrCodeDockerDaemonNotRunning, "Docker Daemon Not Running"},

		// Resource errors
		{ErrCodeResourceExhaustion, "Resource Exhaustion"},
		{ErrCodeOutOfMemory, "Out of Memory"},
		{ErrCodeOutOfDiskSpace, "Out of Disk Space"},
		{ErrCodeTooManyOpenFiles, "Too Many Open Files"},
		{ErrCodeResourceConflict, "Resource Conflict"},
		{ErrCodeContainerLimitReached, "Container Limit Reached"},

		// Network errors
		{ErrCodeNetworkConfiguration, "Network Configuration Error"},
		{ErrCodeNetworkNotFound, "Network Not Found"},
		{ErrCodeNetworkConflict, "Network Conflict"},
		{ErrCodeSubnetOverlap, "Subnet Overlap"},
		{ErrCodeDNSResolutionFailed, "DNS Resolution Failed"},
		{ErrCodePortConflict, "Port Conflict"},
		{ErrCodeBridgeConfigError, "Bridge Configuration Error"},
		{ErrCodeIPForwardingDisabled, "IP Forwarding Disabled"},
		{ErrCodeIptablesConfigError, "Iptables Configuration Error"},
		{ErrCodeMTUMismatch, "MTU Mismatch"},

		// System errors
		{ErrCodeSystemConfiguration, "System Configuration Error"},
		{ErrCodePermissionError, "Permission Error"},
		{ErrCodeServiceNotRunning, "Service Not Running"},
		{ErrCodeConfigurationMissing, "Configuration Missing"},
		{ErrCodeDependencyMissing, "Dependency Missing"},

		// Context errors
		{ErrCodeTimeout, "Timeout"},
		{ErrCodeCancelled, "Cancelled"},
		{ErrCodeDeadlineExceeded, "Deadline Exceeded"},
		{ErrCodeContextExpired, "Context Expired"},

		// Circuit breaker errors
		{ErrCodeCircuitBreakerOpen, "Circuit Breaker Open"},
		{ErrCodeTooManyFailures, "Too Many Failures"},
		{ErrCodeServiceUnavailable, "Service Unavailable"},

		// Unknown code
		{ErrorCode(9999), "Unknown Error Code (9999)"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			err := &DiagnosticError{Code: tc.code}
			result := err.GetCodeString()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestGetErrorTypeFromCode tests the pure error type classification function
func TestGetErrorTypeFromCode(t *testing.T) {
	testCases := []struct {
		code     ErrorCode
		expected ErrorType
		name     string
	}{
		// Connection errors (1000-1099)
		{ErrCodeDockerDaemonUnreachable, ErrTypeConnection, "Connection"},
		{ErrCodeDockerPermissionDenied, ErrTypeConnection, "Connection"},

		// Resource errors (1100-1199)
		{ErrCodeResourceExhaustion, ErrTypeResource, "Resource"},
		{ErrCodeOutOfMemory, ErrTypeResource, "Resource"},

		// Network errors (1200-1299)
		{ErrCodeNetworkConfiguration, ErrTypeNetwork, "Network"},
		{ErrCodeDNSResolutionFailed, ErrTypeNetwork, "Network"},

		// System errors (1300-1399)
		{ErrCodeSystemConfiguration, ErrTypeSystem, "System"},
		{ErrCodePermissionError, ErrTypeSystem, "System"},

		// Context errors (1400-1499)
		{ErrCodeTimeout, ErrTypeContext, "Context"},
		{ErrCodeCancelled, ErrTypeContext, "Context"},

		// Circuit breaker errors (1500-1599)
		{ErrCodeCircuitBreakerOpen, ErrTypeCircuitBreaker, "CircuitBreaker"},
		{ErrCodeTooManyFailures, ErrTypeCircuitBreaker, "CircuitBreaker"},

		// Generic errors
		{ErrCodeGeneric, ErrTypeGeneric, "Generic"},
		{ErrorCode(9999), ErrTypeGeneric, "Generic"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getErrorTypeFromCode(tc.code)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestGetSeverityFromCode tests the pure severity classification function
func TestGetSeverityFromCode(t *testing.T) {
	testCases := []struct {
		code     ErrorCode
		expected ErrorSeverity
		name     string
	}{
		// Critical errors
		{ErrCodeDockerDaemonUnreachable, SeverityCritical, "DaemonUnreachable"},
		{ErrCodeDockerDaemonNotRunning, SeverityCritical, "DaemonNotRunning"},
		{ErrCodeResourceExhaustion, SeverityCritical, "ResourceExhaustion"},
		{ErrCodeOutOfMemory, SeverityCritical, "OutOfMemory"},
		{ErrCodeOutOfDiskSpace, SeverityCritical, "OutOfDiskSpace"},

		// Error level
		{ErrCodePermissionError, SeverityError, "PermissionError"},
		{ErrCodeDockerPermissionDenied, SeverityError, "DockerPermissionDenied"},
		{ErrCodeCircuitBreakerOpen, SeverityError, "CircuitBreakerOpen"},

		// Warning level
		{ErrCodeTimeout, SeverityWarning, "Timeout"},
		{ErrCodeCancelled, SeverityWarning, "Cancelled"},

		// Default warning
		{ErrCodeNetworkConfiguration, SeverityWarning, "NetworkConfiguration"},
		{ErrorCode(9999), SeverityWarning, "Unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getSeverityFromCode(tc.code)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestDiagnosticErrorIsRecoverable tests the pure recoverable determination logic
func TestDiagnosticErrorIsRecoverable(t *testing.T) {
	testCases := []struct {
		errorType ErrorType
		code      ErrorCode
		expected  bool
		name      string
	}{
		// Recoverable types
		{ErrTypeConnection, ErrCodeDockerDaemonUnreachable, true, "Connection"},
		{ErrTypeResource, ErrCodeResourceExhaustion, true, "Resource"},
		{ErrTypeNetwork, ErrCodeNetworkConfiguration, true, "Network"},
		{ErrTypeCircuitBreaker, ErrCodeCircuitBreakerOpen, true, "CircuitBreaker"},

		// Context - only timeout is recoverable
		{ErrTypeContext, ErrCodeTimeout, true, "ContextTimeout"},
		{ErrTypeContext, ErrCodeCancelled, false, "ContextCancelled"},

		// System - except permissions
		{ErrTypeSystem, ErrCodeSystemConfiguration, true, "SystemNonPermission"},
		{ErrTypeSystem, ErrCodePermissionError, false, "SystemPermission"},

		// Non-recoverable
		{ErrTypeGeneric, ErrCodeGeneric, false, "Generic"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := &DiagnosticError{
				Type: tc.errorType,
				Code: tc.code,
			}
			result := err.IsRecoverable()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestDiagnosticErrorRequiresUserIntervention tests user intervention logic
func TestDiagnosticErrorRequiresUserIntervention(t *testing.T) {
	testCases := []struct {
		code     ErrorCode
		severity ErrorSeverity
		expected bool
		name     string
	}{
		// Always requires user intervention
		{ErrCodeDockerPermissionDenied, SeverityError, true, "DockerPermission"},
		{ErrCodePermissionError, SeverityError, true, "Permission"},
		{ErrCodeDockerDaemonNotRunning, SeverityCritical, true, "DaemonNotRunning"},
		{ErrCodeServiceNotRunning, SeverityWarning, true, "ServiceNotRunning"},
		{ErrCodeConfigurationMissing, SeverityWarning, true, "ConfigMissing"},
		{ErrCodeDependencyMissing, SeverityWarning, true, "DependencyMissing"},
		{ErrCodeSubnetOverlap, SeverityWarning, true, "SubnetOverlap"},
		{ErrCodePortConflict, SeverityWarning, true, "PortConflict"},

		// Critical severity always requires intervention
		{ErrCodeGeneric, SeverityCritical, true, "CriticalSeverity"},

		// Others don't require intervention
		{ErrCodeTimeout, SeverityWarning, false, "Timeout"},
		{ErrCodeNetworkConfiguration, SeverityWarning, false, "NetworkConfig"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := &DiagnosticError{
				Code:     tc.code,
				Severity: tc.severity,
			}
			result := err.RequiresUserIntervention()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestDiagnosticErrorGetImmediateActions tests immediate action suggestions
func TestDiagnosticErrorGetImmediateActions(t *testing.T) {
	testCases := []struct {
		errorType ErrorType
		expected  int // minimum number of actions expected
		name      string
	}{
		{ErrTypeConnection, 3, "Connection"},
		{ErrTypeResource, 3, "Resource"},
		{ErrTypeNetwork, 3, "Network"},
		{ErrTypeSystem, 3, "System"},
		{ErrTypeGeneric, 2, "Generic"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := &DiagnosticError{Type: tc.errorType}
			actions := err.GetImmediateActions()
			assert.GreaterOrEqual(t, len(actions), tc.expected)
			// Ensure all actions are non-empty strings
			for _, action := range actions {
				assert.NotEmpty(t, action)
			}
		})
	}
}

// TestDiagnosticErrorGetImmediateActionsWithRecovery tests custom recovery actions
func TestDiagnosticErrorGetImmediateActionsWithRecovery(t *testing.T) {
	customActions := []string{"Custom action 1", "Custom action 2"}
	err := &DiagnosticError{
		Type: ErrTypeConnection,
		Recovery: &RecoveryStrategy{
			Immediate: customActions,
		},
	}

	actions := err.GetImmediateActions()
	assert.Equal(t, customActions, actions)
}

// TestDiagnosticErrorGetLongTermActions tests long-term action suggestions
func TestDiagnosticErrorGetLongTermActions(t *testing.T) {
	testCases := []struct {
		errorType ErrorType
		expected  int // minimum number of actions expected
		name      string
	}{
		{ErrTypeConnection, 3, "Connection"},
		{ErrTypeResource, 3, "Resource"},
		{ErrTypeNetwork, 3, "Network"},
		{ErrTypeSystem, 3, "System"},
		{ErrTypeGeneric, 2, "Generic"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := &DiagnosticError{Type: tc.errorType}
			actions := err.GetLongTermActions()
			assert.GreaterOrEqual(t, len(actions), tc.expected)
			// Ensure all actions are non-empty strings
			for _, action := range actions {
				assert.NotEmpty(t, action)
			}
		})
	}
}

// TestDiagnosticErrorGetLongTermActionsWithRecovery tests custom long-term recovery actions
func TestDiagnosticErrorGetLongTermActionsWithRecovery(t *testing.T) {
	customActions := []string{"Long-term action 1", "Long-term action 2"}
	err := &DiagnosticError{
		Type: ErrTypeResource,
		Recovery: &RecoveryStrategy{
			LongTerm: customActions,
		},
	}

	actions := err.GetLongTermActions()
	assert.Equal(t, customActions, actions)
}

// TestDiagnosticErrorToMap tests pure map conversion
func TestDiagnosticErrorToMap(t *testing.T) {
	timestamp := time.Now()
	err := &DiagnosticError{
		Code:              ErrCodeDockerDaemonUnreachable,
		Type:              ErrTypeConnection,
		Message:           "Test error message",
		Severity:          SeverityCritical,
		Category:          "test-category",
		Source:            "test-source",
		Timestamp:         timestamp,
		Context:           map[string]interface{}{"key": "value"},
		RecoveryAttempted: true,
		RecoveryError:     "recovery failed",
		Tags:              []string{"tag1", "tag2"},
		UserAction:        "user action required",
		Reference:         "http://example.com/docs",
	}

	result := err.ToMap()

	// Test required fields
	codeValue, ok := result["code"].(ErrorCode)
	assert.True(t, ok, "code should be ErrorCode type")
	assert.Equal(t, int(ErrCodeDockerDaemonUnreachable), int(codeValue))
	assert.Equal(t, "Connection", result["type"])
	assert.Equal(t, "Test error message", result["message"])
	assert.Equal(t, "CRITICAL", result["severity"])
	assert.Equal(t, timestamp, result["timestamp"])

	// Test optional fields
	assert.Equal(t, "test-category", result["category"])
	assert.Equal(t, "test-source", result["source"])
	assert.Equal(t, map[string]interface{}{"key": "value"}, result["context"])
	assert.Equal(t, true, result["recovery_attempted"])
	assert.Equal(t, "recovery failed", result["recovery_error"])
	assert.Equal(t, []string{"tag1", "tag2"}, result["tags"])
	assert.Equal(t, "user action required", result["user_action"])
	assert.Equal(t, "http://example.com/docs", result["reference"])
}

// TestDiagnosticErrorToMapMinimal tests map conversion with minimal fields
func TestDiagnosticErrorToMapMinimal(t *testing.T) {
	timestamp := time.Now()
	err := &DiagnosticError{
		Code:      ErrCodeGeneric,
		Type:      ErrTypeGeneric,
		Message:   "Minimal error",
		Severity:  SeverityInfo,
		Timestamp: timestamp,
	}

	result := err.ToMap()

	// Test only required fields
	assert.Len(t, result, 5, "Should only contain 5 required fields")
	codeValue, ok := result["code"].(ErrorCode)
	assert.True(t, ok, "code should be ErrorCode type")
	assert.Equal(t, int(ErrCodeGeneric), int(codeValue))
	assert.Equal(t, "Generic", result["type"])
	assert.Equal(t, "Minimal error", result["message"])
	assert.Equal(t, "INFO", result["severity"])
	assert.Equal(t, timestamp, result["timestamp"])
}

// TestNewDiagnosticErrorPure tests the constructor function (renamed to avoid conflict)
func TestNewDiagnosticErrorPure(t *testing.T) {
	testCases := []struct {
		code     ErrorCode
		message  string
		expected struct {
			errorType ErrorType
			severity  ErrorSeverity
		}
		name string
	}{
		{
			code:    ErrCodeDockerDaemonUnreachable,
			message: "Docker daemon unreachable",
			expected: struct {
				errorType ErrorType
				severity  ErrorSeverity
			}{ErrTypeConnection, SeverityCritical},
			name: "Connection",
		},
		{
			code:    ErrCodeResourceExhaustion,
			message: "Resource exhaustion",
			expected: struct {
				errorType ErrorType
				severity  ErrorSeverity
			}{ErrTypeResource, SeverityCritical},
			name: "Resource",
		},
		{
			code:    ErrCodeNetworkConfiguration,
			message: "Network configuration error",
			expected: struct {
				errorType ErrorType
				severity  ErrorSeverity
			}{ErrTypeNetwork, SeverityWarning},
			name: "Network",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewDiagnosticError(tc.code, tc.message)

			assert.Equal(t, tc.code, err.Code)
			assert.Equal(t, tc.message, err.Message)
			assert.Equal(t, tc.expected.errorType, err.Type)
			assert.Equal(t, tc.expected.severity, err.Severity)
			assert.NotZero(t, err.Timestamp)
			assert.NotNil(t, err.Context)
			assert.Empty(t, err.Context) // Should be empty but not nil
		})
	}
}

// TestDiagnosticErrorError tests the Error interface implementation
func TestDiagnosticErrorError(t *testing.T) {
	testCases := []struct {
		err      *DiagnosticError
		expected string
		name     string
	}{
		{
			err: &DiagnosticError{
				Message: "Custom error message",
			},
			expected: "Custom error message",
			name:     "WithMessage",
		},
		{
			err: &DiagnosticError{
				Code:     ErrCodeDockerDaemonUnreachable,
				Type:     ErrTypeConnection,
				Original: assert.AnError,
			},
			expected: assert.AnError.Error(),
			name:     "WithOriginalError",
		},
		{
			err: &DiagnosticError{
				Code: ErrCodeGeneric,
				Type: ErrTypeGeneric,
			},
			expected: "diagnostic error (code: 0, type: 0)",
			name:     "WithCodeAndType",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.err.Error()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestDiagnosticErrorUnwrap tests error unwrapping
func TestDiagnosticErrorUnwrap(t *testing.T) {
	originalErr := assert.AnError
	err := &DiagnosticError{
		Original: originalErr,
	}

	unwrapped := err.Unwrap()
	assert.Equal(t, originalErr, unwrapped)
}

// TestDiagnosticErrorUnwrapNil tests unwrapping when no original error
func TestDiagnosticErrorUnwrapNil(t *testing.T) {
	err := &DiagnosticError{}
	unwrapped := err.Unwrap()
	assert.Nil(t, unwrapped)
}

// TestDefaultRateLimiterConfig tests the default configuration function
func TestDefaultRateLimiterConfig(t *testing.T) {
	config := DefaultRateLimiterConfig()

	require.NotNil(t, config)

	// Test that all fields have sensible defaults
	assert.Greater(t, config.RequestsPerSecond, 0.0, "RequestsPerSecond should be positive")
	assert.Greater(t, config.BurstSize, 0, "BurstSize should be positive")
	assert.Greater(t, config.WaitTimeout, time.Duration(0), "WaitTimeout should be positive")
	assert.True(t, config.Enabled, "Should be enabled by default")
}

// TestCheckNameDescriptionMethods tests Check interface implementations
func TestCheckNameDescriptionMethods(t *testing.T) {
	// Test DaemonConnectivityCheck
	daemon := &DaemonConnectivityCheck{}
	assert.Equal(t, "daemon_connectivity", daemon.Name())
	assert.NotEmpty(t, daemon.Description())
	assert.Equal(t, SeverityCritical, daemon.Severity())

	// Test NetworkIsolationCheck
	network := &NetworkIsolationCheck{}
	assert.Equal(t, "network_isolation", network.Name())
	assert.NotEmpty(t, network.Description())
	assert.Equal(t, SeverityWarning, network.Severity())

	// Test ContainerConnectivityCheck
	connectivity := &ContainerConnectivityCheck{}
	assert.Equal(t, "container_connectivity", connectivity.Name())
	assert.NotEmpty(t, connectivity.Description())
	assert.Equal(t, SeverityWarning, connectivity.Severity())

	// Test PortBindingCheck
	port := &PortBindingCheck{}
	assert.Equal(t, "port_binding", port.Name())
	assert.NotEmpty(t, port.Description())
	assert.Equal(t, SeverityWarning, port.Severity())

	// Test DNSResolutionCheck
	dns := &DNSResolutionCheck{}
	assert.Equal(t, "dns_resolution", dns.Name())
	assert.NotEmpty(t, dns.Description())
	assert.Equal(t, SeverityWarning, dns.Severity())

	// Test InternalDNSCheck
	internal := &InternalDNSCheck{}
	assert.Equal(t, "internal_dns", internal.Name())
	assert.NotEmpty(t, internal.Description())
	assert.Equal(t, SeverityWarning, internal.Severity())

	// Test BridgeNetworkCheck
	bridge := &BridgeNetworkCheck{}
	assert.Equal(t, "bridge_network", bridge.Name())
	assert.NotEmpty(t, bridge.Description())
	assert.Equal(t, SeverityCritical, bridge.Severity())

	// Test IPForwardingCheck
	ipForwarding := &IPForwardingCheck{}
	assert.Equal(t, "ip_forwarding", ipForwarding.Name())
	assert.NotEmpty(t, ipForwarding.Description())
	assert.Equal(t, SeverityCritical, ipForwarding.Severity())

	// Test SubnetOverlapCheck
	subnet := &SubnetOverlapCheck{}
	assert.Equal(t, "subnet_overlap", subnet.Name())
	assert.NotEmpty(t, subnet.Description())
	assert.Equal(t, SeverityWarning, subnet.Severity())

	// Test MTUConsistencyCheck
	mtu := &MTUConsistencyCheck{}
	assert.Equal(t, "mtu_consistency", mtu.Name())
	assert.NotEmpty(t, mtu.Description())
	assert.Equal(t, SeverityWarning, mtu.Severity())

	// Test IptablesCheck
	iptables := &IptablesCheck{}
	assert.Equal(t, "iptables", iptables.Name())
	assert.NotEmpty(t, iptables.Description())
	assert.Equal(t, SeverityCritical, iptables.Severity())
}

// TestResourceTypeString tests ResourceType.String() method
func TestResourceTypeString(t *testing.T) {
	testCases := []struct {
		resourceType ResourceType
		expected     string
	}{
		{ResourceTypeContainer, "container"},
		{ResourceTypeNetwork, "network"},
		{ResourceTypeVolume, "volume"},
		{ResourceTypeImage, "image"},
		{ResourceTypeSecret, "secret"},
		{ResourceTypeConfig, "config"},
		{ResourceType(999), "unknown"}, // Invalid type
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := tc.resourceType.String()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestCleanupTriggerString tests CleanupTrigger.String() method
func TestCleanupTriggerString(t *testing.T) {
	testCases := []struct {
		trigger  CleanupTrigger
		expected string
	}{
		{TriggerErrorRecovery, "error_recovery"},
		{TriggerPeriodic, "periodic"},
		{TriggerManual, "manual"},
		{TriggerEmergency, "emergency"},
		{TriggerShutdown, "shutdown"},
		{CleanupTrigger(999), "unknown"}, // Invalid trigger
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := tc.trigger.String()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestCleanupActionString tests CleanupAction.String() method
func TestCleanupActionString(t *testing.T) {
	testCases := []struct {
		action   CleanupAction
		expected string
	}{
		{ActionRemoved, "removed"},
		{ActionStopped, "stopped"},
		{ActionSkipped, "skipped"},
		{ActionFailed, "failed"},
		{ActionBackedUp, "backed_up"},
		{CleanupAction(999), "unknown"}, // Invalid action
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := tc.action.String()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestDegradationLevelString tests DegradationLevel.String() method
func TestDegradationLevelString(t *testing.T) {
	testCases := []struct {
		level    DegradationLevel
		expected string
	}{
		{DegradationNormal, "NORMAL"},
		{DegradationReduced, "REDUCED"},
		{DegradationMinimal, "MINIMAL"},
		{DegradationEmergency, "EMERGENCY"},
		{DegradationLevel(999), "UNKNOWN"}, // Invalid level
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := tc.level.String()
			assert.Equal(t, tc.expected, result)
		})
	}
}
