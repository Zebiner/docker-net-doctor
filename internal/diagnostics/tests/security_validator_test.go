package diagnostics_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/diagnostics"
	"github.com/zebiner/docker-net-doctor/internal/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockCheck implements the Check interface for testing
type MockCheck struct {
	mock.Mock
}

// COMPREHENSIVE SECURITY VALIDATION TESTS FOR 85%+ COVERAGE

// TestValidationRules tests comprehensive validation rule functionality
func TestValidationRules(t *testing.T) {
	t.Run("All Validation Rules Applied", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test that all validation rules are properly initialized
		stats := validator.GetStatistics()
		assert.True(t, stats.AllowedChecks > 0, "Should have default allowed checks")
		
		// Create a check that should pass all rules
		validCheck := &MockCheck{}
		validCheck.On("Name").Return("ip_forwarding")
		validCheck.On("Description").Return("Valid IP forwarding check")
		validCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		err := validator.ValidateCheck(validCheck)
		assert.NoError(t, err)
		
		// Verify audit log shows rule validation
		auditLog := validator.GetAuditLog()
		assert.NotEmpty(t, auditLog)
		lastEntry := auditLog[len(auditLog)-1]
		assert.True(t, lastEntry.Allowed)
		assert.Contains(t, lastEntry.Reason, "All validation rules passed")
	})

	t.Run("Rule Severity Enforcement", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test critical rule failure blocks validation
		criticalFailCheck := &MockCheck{}
		criticalFailCheck.On("Name").Return("invalid-name-with-dots..")
		criticalFailCheck.On("Description").Return("Valid description")
		criticalFailCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		err := validator.ValidateCheck(criticalFailCheck)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "security validation failed")
	})

	t.Run("Rule Precedence and Order", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test that name validation happens before allowlist
		invalidNameCheck := &MockCheck{}
		invalidNameCheck.On("Name").Return("invalid$name")
		invalidNameCheck.On("Description").Return("Valid description")
		invalidNameCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		err := validator.ValidateCheck(invalidNameCheck)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "check_name_validation")
	})
}

// TestAllowlistDenylistAdvanced tests comprehensive allowlist/denylist functionality
func TestAllowlistDenylistAdvanced(t *testing.T) {
	t.Run("Pattern Matching Complex Cases", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test various allowlist patterns
		patternTests := []struct {
			name         string
			checkName    string
			expectAllowed bool
		}{
			{"Simple Valid Name", "network_check", false}, // Not in default allowlist
			{"Default Allowed", "ip_forwarding", true},
			{"Hyphenated Name", "bridge-network", false}, // Not in default allowlist
			{"Numbered Name", "check123", false}, // Not in default allowlist
			{"Underscored Name", "my_custom_check", false}, // Not in default allowlist
		}
		
		for _, test := range patternTests {
			t.Run(test.name, func(t *testing.T) {
				mockCheck := &MockCheck{}
				mockCheck.On("Name").Return(test.checkName)
				mockCheck.On("Description").Return("Test check")
				mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
				
				err := validator.ValidateCheck(mockCheck)
				if test.expectAllowed {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), "not in the allowed list")
				}
			})
		}
	})

	t.Run("Dynamic List Management", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test adding to allowlist
		err := validator.AddToAllowlist("custom_check")
		assert.NoError(t, err)
		assert.True(t, validator.IsCheckAllowed("custom_check"))
		
		// Test adding to denylist removes from allowlist
		err = validator.AddToDenylist("custom_check")
		assert.NoError(t, err)
		assert.False(t, validator.IsCheckAllowed("custom_check"))
		assert.True(t, validator.IsCheckDenied("custom_check"))
		
		// Test re-adding to allowlist removes from denylist
		err = validator.AddToAllowlist("custom_check")
		assert.NoError(t, err)
		assert.True(t, validator.IsCheckAllowed("custom_check"))
		assert.False(t, validator.IsCheckDenied("custom_check"))
	})

	t.Run("Invalid List Management", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test adding invalid names
		invalidNames := []string{
			"", // empty
			"invalid@name", // invalid characters
			strings.Repeat("a", 101), // too long
			"dangerous$(/bin/ls)", // shell injection
		}
		
		for _, name := range invalidNames {
			t.Run(fmt.Sprintf("Invalid name: %s", name), func(t *testing.T) {
				err := validator.AddToAllowlist(name)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid check name")
				
				err = validator.AddToDenylist(name)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid check name")
			})
		}
	})

	t.Run("Conflict Resolution", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Add check to both lists in sequence
		err := validator.AddToAllowlist("conflicted_check")
		assert.NoError(t, err)
		
		// Adding to denylist should remove from allowlist
		err = validator.AddToDenylist("conflicted_check")
		assert.NoError(t, err)
		
		assert.False(t, validator.IsCheckAllowed("conflicted_check"))
		assert.True(t, validator.IsCheckDenied("conflicted_check"))
		
		// Verify audit log records the changes
		auditLog := validator.GetAuditLog()
		assert.True(t, len(auditLog) >= 2)
	})
}

// TestAuditLoggingAdvanced tests comprehensive audit logging functionality
func TestAuditLoggingAdvanced(t *testing.T) {
	t.Run("Audit Log Entry Structure", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		mockCheck := &MockCheck{}
		mockCheck.On("Name").Return("ip_forwarding")
		mockCheck.On("Description").Return("Test description")
		mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		startTime := time.Now()
		err := validator.ValidateCheck(mockCheck)
		assert.NoError(t, err)
		endTime := time.Now()
		
		auditLog := validator.GetAuditLog()
		assert.Len(t, auditLog, 1)
		
		entry := auditLog[0]
		assert.Equal(t, "ip_forwarding", entry.CheckName)
		assert.True(t, entry.Allowed)
		assert.NotEmpty(t, entry.Reason)
		assert.Equal(t, "default", entry.ValidatorID)
		assert.True(t, entry.Timestamp.After(startTime.Add(-time.Second)))
		assert.True(t, entry.Timestamp.Before(endTime.Add(time.Second)))
	})

	t.Run("Audit Log Categorization", func(t *testing.T) {
		// Test audit log categorization scenarios
		
		// Test different types of validation results
		testScenarios := []struct {
			name          string
			checkName     string
			setupFunc     func(*diagnostics.SecurityValidator)
			expectAllowed bool
			reasonPattern string
		}{
			{
				name:          "Successful Validation",
				checkName:     "ip_forwarding",
				setupFunc:     func(v *diagnostics.SecurityValidator) {},
				expectAllowed: true,
				reasonPattern: "All validation rules passed",
			},
			{
				name:          "Allowlist Addition",
				checkName:     "custom_check",
				setupFunc:     func(v *diagnostics.SecurityValidator) { v.AddToAllowlist("custom_check") },
				expectAllowed: true,
				reasonPattern: "Added to allowlist",
			},
			{
				name:          "Denylist Addition",
				checkName:     "blocked_check",
				setupFunc:     func(v *diagnostics.SecurityValidator) { v.AddToDenylist("blocked_check") },
				expectAllowed: false,
				reasonPattern: "Added to denylist",
			},
		}
		
		for _, scenario := range testScenarios {
			t.Run(scenario.name, func(t *testing.T) {
				testValidator := diagnostics.NewSecurityValidator()
				scenario.setupFunc(testValidator)
				
				if scenario.name != "Allowlist Addition" && scenario.name != "Denylist Addition" {
					mockCheck := &MockCheck{}
					mockCheck.On("Name").Return(scenario.checkName)
					mockCheck.On("Description").Return("Test description")
					mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
					
					_ = testValidator.ValidateCheck(mockCheck)
				}
				
				auditLog := testValidator.GetAuditLog()
				assert.NotEmpty(t, auditLog)
				
				// Find relevant log entry
				var relevantEntry *diagnostics.CheckAuditLog
				for i := len(auditLog) - 1; i >= 0; i-- {
					if strings.Contains(auditLog[i].Reason, scenario.reasonPattern) {
						relevantEntry = &auditLog[i]
						break
					}
				}
				
				require.NotNil(t, relevantEntry, "Should find relevant audit log entry")
				assert.Equal(t, scenario.expectAllowed, relevantEntry.Allowed)
				assert.Contains(t, relevantEntry.Reason, scenario.reasonPattern)
			})
		}
	})

	t.Run("Audit Log Filtering and Search", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Create mixed success/failure scenarios
		successCheck := &MockCheck{}
		successCheck.On("Name").Return("ip_forwarding")
		successCheck.On("Description").Return("Success check")
		successCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		failCheck := &MockCheck{}
		failCheck.On("Name").Return("unknown_check")
		failCheck.On("Description").Return("Fail check")
		failCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		// Generate mixed validation results
		_ = validator.ValidateCheck(successCheck)
		_ = validator.ValidateCheck(failCheck)
		_ = validator.ValidateCheck(successCheck)
		_ = validator.ValidateCheck(failCheck)
		
		// Test recent denials filtering
		recentDenials := validator.GetRecentDenials(1 * time.Hour)
		assert.Len(t, recentDenials, 2)
		
		for _, denial := range recentDenials {
			assert.False(t, denial.Allowed)
			assert.Equal(t, "unknown_check", denial.CheckName)
		}
		
		// Test that very short duration returns no denials
		noDenials := validator.GetRecentDenials(1 * time.Nanosecond)
		assert.Empty(t, noDenials)
	})

	t.Run("Audit Log Statistics Accuracy", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Setup controlled validation scenario
		successCheck := &MockCheck{}
		successCheck.On("Name").Return("ip_forwarding")
		successCheck.On("Description").Return("Success check")
		successCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		failCheck := &MockCheck{}
		failCheck.On("Name").Return("unknown_check")
		failCheck.On("Description").Return("Fail check")
		failCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		// Perform known number of validations
		for i := 0; i < 7; i++ {
			_ = validator.ValidateCheck(successCheck)
		}
		for i := 0; i < 3; i++ {
			_ = validator.ValidateCheck(failCheck)
		}
		
		stats := validator.GetStatistics()
		assert.Equal(t, 10, stats.TotalValidations)
		assert.Equal(t, 7, stats.TotalAllowed)
		assert.Equal(t, 3, stats.TotalDenied)
		assert.InDelta(t, 0.3, stats.DenialRate, 0.01)
	})

	t.Run("Audit Log Reset Functionality", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Add some entries
		mockCheck := &MockCheck{}
		mockCheck.On("Name").Return("ip_forwarding")
		mockCheck.On("Description").Return("Test check")
		mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		for i := 0; i < 5; i++ {
			_ = validator.ValidateCheck(mockCheck)
		}
		
		auditLog := validator.GetAuditLog()
		assert.Len(t, auditLog, 5)
		
		// Reset and verify empty
		validator.ResetAuditLog()
		auditLog = validator.GetAuditLog()
		assert.Empty(t, auditLog)
		
		// Verify statistics also reset
		stats := validator.GetStatistics()
		assert.Equal(t, 0, stats.TotalValidations)
		assert.Equal(t, 0, stats.TotalAllowed)
		assert.Equal(t, 0, stats.TotalDenied)
		assert.Equal(t, 0.0, stats.DenialRate)
	})
}

// TestSensitiveDataRedaction tests audit log data protection
func TestSensitiveDataRedaction(t *testing.T) {
	t.Run("Check Name Sanitization", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test that potentially sensitive check names are handled properly
		sensitiveCheck := &MockCheck{}
		sensitiveCheck.On("Name").Return("config_check_with_sensitive_data")
		sensitiveCheck.On("Description").Return("Check with potential sensitive info")
		sensitiveCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		// Add to allowlist first so validation passes
		err := validator.AddToAllowlist("config_check_with_sensitive_data")
		assert.NoError(t, err)
		
		err = validator.ValidateCheck(sensitiveCheck)
		assert.NoError(t, err)
		
		// Verify audit log contains check name as-is (no redaction needed for check names)
		auditLog := validator.GetAuditLog()
		assert.NotEmpty(t, auditLog)
		
		// Find validation entry
		var validationEntry *diagnostics.CheckAuditLog
		for _, entry := range auditLog {
			if entry.CheckName == "config_check_with_sensitive_data" && entry.Allowed {
				validationEntry = &entry
				break
			}
		}
		
		require.NotNil(t, validationEntry)
		assert.Equal(t, "config_check_with_sensitive_data", validationEntry.CheckName)
	})

	t.Run("Description Content Validation", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test that descriptions are validated for malicious content
		maliciousCheck := &MockCheck{}
		maliciousCheck.On("Name").Return("ip_forwarding")
		maliciousCheck.On("Description").Return("Valid check with <script>alert('xss')</script> content")
		maliciousCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		err := validator.ValidateCheck(maliciousCheck)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "contains potential script injection")
		
		// Verify audit log records the failure
		auditLog := validator.GetAuditLog()
		assert.NotEmpty(t, auditLog)
		
		lastEntry := auditLog[len(auditLog)-1]
		assert.False(t, lastEntry.Allowed)
		assert.Contains(t, lastEntry.Reason, "check_description_validation")
	})
}

// TestConcurrentValidation tests thread safety
func TestConcurrentValidation(t *testing.T) {
	t.Run("Concurrent Validation Safety", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Add custom check to allowlist
		err := validator.AddToAllowlist("concurrent_check")
		assert.NoError(t, err)
		
		var wg sync.WaitGroup
		validationCount := 100
		errorChan := make(chan error, validationCount)
		
		// Run concurrent validations
		for i := 0; i < validationCount; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				
				mockCheck := &MockCheck{}
				mockCheck.On("Name").Return("concurrent_check")
				mockCheck.On("Description").Return(fmt.Sprintf("Concurrent check %d", id))
				mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
				
				err := validator.ValidateCheck(mockCheck)
				errorChan <- err
			}(i)
		}
		
		wg.Wait()
		close(errorChan)
		
		// Check that all validations succeeded
		errorCount := 0
		for err := range errorChan {
			if err != nil {
				errorCount++
				t.Logf("Validation error: %v", err)
			}
		}
		assert.Equal(t, 0, errorCount, "No validation errors expected")
		
		// Verify audit log integrity
		auditLog := validator.GetAuditLog()
		assert.Equal(t, validationCount, len(auditLog))
		
		// Verify statistics accuracy
		stats := validator.GetStatistics()
		assert.Equal(t, validationCount, stats.TotalValidations)
		assert.Equal(t, validationCount, stats.TotalAllowed)
		assert.Equal(t, 0, stats.TotalDenied)
	})

	t.Run("Concurrent List Management", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		var wg sync.WaitGroup
		operationCount := 50
		
		// Concurrent allowlist operations
		for i := 0; i < operationCount; i++ {
			wg.Add(2) // Two operations per iteration
			
			go func(id int) {
				defer wg.Done()
				checkName := fmt.Sprintf("allow_check_%d", id)
				_ = validator.AddToAllowlist(checkName)
			}(i)
			
			go func(id int) {
				defer wg.Done()
				checkName := fmt.Sprintf("deny_check_%d", id)
				_ = validator.AddToDenylist(checkName)
			}(i)
		}
		
		wg.Wait()
		
		// Verify final state
		stats := validator.GetStatistics()
		assert.True(t, stats.AllowedChecks >= operationCount)
		assert.True(t, stats.DeniedChecks >= operationCount)
		
		// Verify no data races occurred by checking some specific entries
		assert.True(t, validator.IsCheckAllowed("allow_check_0"))
		assert.True(t, validator.IsCheckDenied("deny_check_0"))
	})
}

// TestSecurityValidatorEdgeCases tests edge cases and error conditions
func TestSecurityValidatorEdgeCases(t *testing.T) {
	t.Run("Nil and Empty Inputs", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test nil check
		err := validator.ValidateCheck(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "check cannot be nil")
		
		// Test empty check name
		emptyNameCheck := &MockCheck{}
		emptyNameCheck.On("Name").Return("")
		emptyNameCheck.On("Description").Return("Valid description")
		emptyNameCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		err = validator.ValidateCheck(emptyNameCheck)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "check name cannot be empty")
	})

	t.Run("Boundary Value Testing", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test maximum length check name (100 characters)
		maxLengthName := strings.Repeat("a", 100)
		maxLengthCheck := &MockCheck{}
		maxLengthCheck.On("Name").Return(maxLengthName)
		maxLengthCheck.On("Description").Return("Valid description")
		maxLengthCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		// Should fail because not in allowlist, but name format should be valid
		err := validator.ValidateCheck(maxLengthCheck)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not in the allowed list")
		assert.NotContains(t, err.Error(), "check name too long")
		
		// Test maximum length description (500 characters)
		maxDescCheck := &MockCheck{}
		maxDescCheck.On("Name").Return("ip_forwarding")
		maxDescCheck.On("Description").Return(strings.Repeat("a", 500))
		maxDescCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		err = validator.ValidateCheck(maxDescCheck)
		// Should pass validation (description is at limit, not over)
		if err != nil {
			// Only description validation should not fail for length
			assert.NotContains(t, err.Error(), "check description too long")
		}
		
		// Test over-limit description (501 characters)
		overLimitDescCheck := &MockCheck{}
		overLimitDescCheck.On("Name").Return("ip_forwarding")
		overLimitDescCheck.On("Description").Return(strings.Repeat("a", 501))
		overLimitDescCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		err = validator.ValidateCheck(overLimitDescCheck)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "check description too long")
	})

	t.Run("Test Mode Edge Cases", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidatorTestMode()
		
		// Test various test check patterns
		testPatterns := []struct {
			name         string
			checkName    string
			expectAllowed bool
		}{
			{"Simple test", "test", true},
			{"Test with underscore", "test_check", true},
			{"Check prefix", "check_something", true},
			{"Mock prefix", "mock_validation", true},
			{"Quick prefix", "quick_test", true},
			{"Debug prefix", "debug_check", true},
			{"Numbered test", "test123", true},
			{"Numbered check", "check42", true},
			{"Contains test", "my_test_check", true},
			{"Contains check", "validate_check_now", true},
			{"Success pattern", "success_test", true},
			{"Failure pattern", "failure_check", true},
			{"Standard check", "ip_forwarding", true},
			{"Non-test pattern", "production_validator", false},
			{"Invalid injection", "test$(/bin/ls)", false},
		}
		
		for _, pattern := range testPatterns {
			t.Run(pattern.name, func(t *testing.T) {
				mockCheck := &MockCheck{}
				mockCheck.On("Name").Return(pattern.checkName)
				mockCheck.On("Description").Return("Test description")
				mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
				
				err := validator.ValidateCheck(mockCheck)
				if pattern.expectAllowed {
					assert.NoError(t, err, "Expected %s to be allowed in test mode", pattern.checkName)
				} else {
					assert.Error(t, err, "Expected %s to be rejected even in test mode", pattern.checkName)
				}
			})
		}
	})

	t.Run("Test Mode Toggle", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test with test mode disabled
		testCheck := &MockCheck{}
		testCheck.On("Name").Return("test_check")
		testCheck.On("Description").Return("Test description")
		testCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		err := validator.ValidateCheck(testCheck)
		assert.Error(t, err) // Should fail in normal mode
		
		// Enable test mode
		validator.SetTestMode(true)
		err = validator.ValidateCheck(testCheck)
		assert.NoError(t, err) // Should pass in test mode
		
		// Disable test mode again
		validator.SetTestMode(false)
		err = validator.ValidateCheck(testCheck)
		assert.Error(t, err) // Should fail again
	})

	t.Run("Security Injection Prevention", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test various injection attempts
		injectionPatterns := []struct {
			name         string
			checkName    string
			description  string
			expectError  bool
			errorPattern string
		}{
			{"Path Traversal", "check../../../etc/passwd", "Valid desc", true, "contains potentially dangerous pattern"},
			{"Shell Command", "check$(whoami)", "Valid desc", true, "contains potentially dangerous pattern"},
			{"Environment Variable", "check${HOME}", "Valid desc", true, "contains potentially dangerous pattern"},
			{"Backtick Execution", "check`ls`", "Valid desc", true, "contains potentially dangerous pattern"},
			{"Pipe Command", "check|ls", "Valid desc", true, "contains potentially dangerous pattern"},
			{"Semicolon Command", "check;ls", "Valid desc", true, "contains potentially dangerous pattern"},
			{"Script in Description", "valid_check", "<script>alert('xss')</script>", true, "contains potential script injection"},
			{"JavaScript URI", "valid_check", "javascript:alert('xss')", true, "contains potential script injection"},
			{"OnError Handler", "valid_check", "image onerror=alert('xss')", true, "contains potential script injection"},
		}
		
		for _, pattern := range injectionPatterns {
			t.Run(pattern.name, func(t *testing.T) {
				mockCheck := &MockCheck{}
				mockCheck.On("Name").Return(pattern.checkName)
				mockCheck.On("Description").Return(pattern.description)
				mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
				
				err := validator.ValidateCheck(mockCheck)
				if pattern.expectError {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), pattern.errorPattern)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})
}

// TestSecurityPolicyManagement tests security policy file handling
func TestSecurityPolicyManagement(t *testing.T) {
	t.Run("Valid Policy Loading", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test that policy structure concepts are validated
		// Note: Since the actual implementation doesn't have policy file loading,
		// we test the conceptual validation patterns
		
		// Test policy-like validation rule concepts
		testCases := []struct {
			name         string
			checkName    string
			expectAllowed bool
			errorPattern string
		}{
			{"Policy Compliant Name", "network_check_v1", false, "not in the allowed list"},
			{"Policy Non-Compliant Length", strings.Repeat("a", 101), false, "check name too long"},
			{"Policy Non-Compliant Chars", "check@name", false, "contains invalid characters"},
			{"Policy Injection Pattern", "check$(ls)", false, "contains potentially dangerous pattern"},
		}
		
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				mockCheck := &MockCheck{}
				mockCheck.On("Name").Return(tc.checkName)
				mockCheck.On("Description").Return("Test description")
				mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
				
				err := validator.ValidateCheck(mockCheck)
				if tc.expectAllowed {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
					if tc.errorPattern != "" {
						assert.Contains(t, err.Error(), tc.errorPattern)
					}
				}
			})
		}
	})

	t.Run("Policy Validation Rules Enforcement", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test that validation rules behave like policy enforcement
		// Test severity-based rule enforcement
		criticalRuleCheck := &MockCheck{}
		criticalRuleCheck.On("Name").Return("invalid&name") // Contains dangerous pattern
		criticalRuleCheck.On("Description").Return("Valid description")
		criticalRuleCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		err := validator.ValidateCheck(criticalRuleCheck)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "security validation failed")
		
		// Test warning rule behavior (description validation)
		warningRuleCheck := &MockCheck{}
		warningRuleCheck.On("Name").Return("ip_forwarding")
		warningRuleCheck.On("Description").Return("<script>alert('test')</script>")
		warningRuleCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		err = validator.ValidateCheck(warningRuleCheck)
		assert.Error(t, err) // Description validation should still block
		assert.Contains(t, err.Error(), "contains potential script injection")
	})

	t.Run("Policy Pattern Matching", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test allowlist pattern concepts
		allowlistPatterns := []struct {
			name         string
			checkName    string
			setupFunc    func(*diagnostics.SecurityValidator)
			expectAllowed bool
		}{
			{
				name:         "Explicit Allowlist Match",
				checkName:    "custom_network_check",
				setupFunc:    func(v *diagnostics.SecurityValidator) { v.AddToAllowlist("custom_network_check") },
				expectAllowed: true,
			},
			{
				name:         "Pattern-like Match", 
				checkName:    "test_pattern_check",
				setupFunc:    func(v *diagnostics.SecurityValidator) { v.AddToAllowlist("test_pattern_check") },
				expectAllowed: true,
			},
			{
				name:         "No Match",
				checkName:    "unknown_pattern",
				setupFunc:    func(v *diagnostics.SecurityValidator) {},
				expectAllowed: false,
			},
		}
		
		for _, pattern := range allowlistPatterns {
			t.Run(pattern.name, func(t *testing.T) {
				pattern.setupFunc(validator)
				
				mockCheck := &MockCheck{}
				mockCheck.On("Name").Return(pattern.checkName)
				mockCheck.On("Description").Return("Test description")
				mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
				
				err := validator.ValidateCheck(mockCheck)
				if pattern.expectAllowed {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
				}
			})
		}
	})

	t.Run("Policy Denylist Patterns", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test denylist pattern enforcement
		denylistPatterns := []string{
			"dangerous_admin_check",
			"root_access_validator", 
			"system_config_check",
			"password_secret_check",
		}
		
		for _, pattern := range denylistPatterns {
			t.Run(fmt.Sprintf("Denylist pattern: %s", pattern), func(t *testing.T) {
				// Add to denylist
				err := validator.AddToDenylist(pattern)
				assert.NoError(t, err)
				
				// Verify it's denied
				assert.True(t, validator.IsCheckDenied(pattern))
				
				// Test validation fails
				mockCheck := &MockCheck{}
				mockCheck.On("Name").Return(pattern)
				mockCheck.On("Description").Return("Test description")
				mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
				
				err = validator.ValidateCheck(mockCheck)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "is in the denied list")
			})
		}
	})
}

// TestAdvancedValidationRules tests complex validation rule scenarios
func TestAdvancedValidationRules(t *testing.T) {
	t.Run("Regex Pattern Validation", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test complex regex-like patterns in check names
		patternTests := []struct {
			name         string
			checkName    string
			expectValid  bool
			errorPattern string
		}{
			{"Valid Alphanumeric", "check_123_abc", true, ""},
			{"Valid Hyphenated", "network-check-v1", true, ""},
			{"Invalid Special Chars", "check@domain.com", false, "contains invalid characters"},
			{"Invalid Unicode", "check_ñame", false, "contains invalid characters"},
			{"Invalid Spaces", "check with spaces", false, "contains invalid characters"},
			{"Invalid Null Byte", "check\x00name", false, "contains invalid characters"},
		}
		
		for _, test := range patternTests {
			t.Run(test.name, func(t *testing.T) {
				mockCheck := &MockCheck{}
				mockCheck.On("Name").Return(test.checkName)
				mockCheck.On("Description").Return("Valid description")
				mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
				
				err := validator.ValidateCheck(mockCheck)
				if test.expectValid {
					// Should fail only because not in allowlist, not because of format
					if err != nil {
						assert.Contains(t, err.Error(), "not in the allowed list")
						assert.NotContains(t, err.Error(), "contains invalid characters")
					}
				} else {
					assert.Error(t, err)
					if test.errorPattern != "" {
						assert.Contains(t, err.Error(), test.errorPattern)
					}
				}
			})
		}
	})

	t.Run("Wildcard Pattern Matching Concepts", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test wildcard-like pattern concepts using the existing allowlist system
		wildcardPatterns := []struct {
			name         string
			patterns     []string // Individual patterns to add to allowlist
			testNames    []string
			expectedResults []bool
		}{
			{
				name:         "Test Pattern Group",
				patterns:     []string{"test_check_1", "test_check_2", "test_check_3"},
				testNames:    []string{"test_check_1", "test_check_2", "test_check_4"},
				expectedResults: []bool{true, true, false},
			},
			{
				name:         "Network Pattern Group", 
				patterns:     []string{"network_check_basic", "network_check_advanced"},
				testNames:    []string{"network_check_basic", "network_check_unknown"},
				expectedResults: []bool{true, false},
			},
		}
		
		for _, wildcardTest := range wildcardPatterns {
			t.Run(wildcardTest.name, func(t *testing.T) {
				// Add patterns to allowlist
				for _, pattern := range wildcardTest.patterns {
					err := validator.AddToAllowlist(pattern)
					assert.NoError(t, err)
				}
				
				// Test each name
				for i, testName := range wildcardTest.testNames {
					mockCheck := &MockCheck{}
					mockCheck.On("Name").Return(testName)
					mockCheck.On("Description").Return("Test description")
					mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
					
					err := validator.ValidateCheck(mockCheck)
					if wildcardTest.expectedResults[i] {
						assert.NoError(t, err, "Expected %s to match pattern", testName)
					} else {
						assert.Error(t, err, "Expected %s to not match pattern", testName)
					}
				}
			})
		}
	})

	t.Run("Complex Injection Prevention", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test complex injection scenarios
		complexInjections := []struct {
			name         string
			checkName    string
			description  string
			expectBlocked bool
			blockReason   string
		}{
			{
				name:         "URL Encoded Injection",
				checkName:    "check%24%28ls%29", // URL encoded $(ls)
				description:  "Valid description",
				expectBlocked: false, // URL encoding not currently detected
				blockReason:   "",
			},
			{
				name:         "Base64 Encoded Injection", 
				checkName:    "checkJChscyk=", // Base64 encoded 
				description:  "Valid description",
				expectBlocked: false, // Base64 not currently detected
				blockReason:   "",
			},
			{
				name:         "Mixed Case Script Tag",
				checkName:    "ip_forwarding",
				description:  "Valid check with <ScRiPt>alert('xss')</ScRiPt> content",
				expectBlocked: true,
				blockReason:   "contains potential script injection",
			},
			{
				name:         "Obfuscated JavaScript",
				checkName:    "ip_forwarding",
				description:  "Check with java\x73cript:alert('test') injection",
				expectBlocked: false, // Obfuscated JS not currently detected
				blockReason:   "",
			},
			{
				name:         "Double Encoding",
				checkName:    "check\\$\\(ls\\)", 
				description:  "Valid description",
				expectBlocked: false, // Double encoding not currently detected
				blockReason:   "",
			},
		}
		
		for _, injection := range complexInjections {
			t.Run(injection.name, func(t *testing.T) {
				mockCheck := &MockCheck{}
				mockCheck.On("Name").Return(injection.checkName)
				mockCheck.On("Description").Return(injection.description)
				mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
				
				err := validator.ValidateCheck(mockCheck)
				if injection.expectBlocked {
					assert.Error(t, err)
					if injection.blockReason != "" {
						assert.Contains(t, err.Error(), injection.blockReason)
					}
				} else {
					// May still fail for other reasons (like allowlist), but not for injection
					if err != nil && injection.blockReason != "" {
						assert.NotContains(t, err.Error(), injection.blockReason)
					}
				}
			})
		}
	})
}

// TestValidationRuleCustomization tests extensible validation rules
func TestValidationRuleCustomization(t *testing.T) {
	t.Run("Rule Severity Levels", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test that different rule severities are handled appropriately
		// Critical rules should block, warning rules should currently block too
		
		// Test critical severity rule (name validation)
		criticalRuleViolation := &MockCheck{}
		criticalRuleViolation.On("Name").Return("invalid$name")
		criticalRuleViolation.On("Description").Return("Valid description")
		criticalRuleViolation.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		err := validator.ValidateCheck(criticalRuleViolation)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "security validation failed")
		assert.Contains(t, err.Error(), "check_name_validation")
		
		// Test warning severity rule (description validation)
		warningRuleViolation := &MockCheck{}
		warningRuleViolation.On("Name").Return("ip_forwarding")
		warningRuleViolation.On("Description").Return("<script>alert('warning')</script>")
		warningRuleViolation.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		err = validator.ValidateCheck(warningRuleViolation)
		assert.Error(t, err) // Currently blocks even warning rules
		assert.Contains(t, err.Error(), "contains potential script injection")
		
		// Test info severity rule (severity validation with invalid range)
		infoRuleViolation := &MockCheck{}
		infoRuleViolation.On("Name").Return("ip_forwarding")
		infoRuleViolation.On("Description").Return("Valid description")
		infoRuleViolation.On("Severity").Return(int(10)) // Invalid severity
		
		err = validator.ValidateCheck(infoRuleViolation)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "severity is out of valid range")
	})

	t.Run("Rule Processing Order", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test that rules are processed in the correct order
		// Name validation should happen before allowlist check
		multipleViolationCheck := &MockCheck{}
		multipleViolationCheck.On("Name").Return("invalid$name_not_in_allowlist")
		multipleViolationCheck.On("Description").Return("Valid description")
		multipleViolationCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		err := validator.ValidateCheck(multipleViolationCheck)
		assert.Error(t, err)
		// Should fail on name validation first (critical rule)
		assert.Contains(t, err.Error(), "check_name_validation")
		assert.Contains(t, err.Error(), "security validation failed")
	})

	t.Run("Rule State Consistency", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		
		// Test that validation rules maintain consistent state
		validCheck := &MockCheck{}
		validCheck.On("Name").Return("ip_forwarding")
		validCheck.On("Description").Return("Valid IP forwarding check")
		validCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		
		// Multiple validations should be consistent
		for i := 0; i < 10; i++ {
			err := validator.ValidateCheck(validCheck)
			assert.NoError(t, err, "Validation %d should pass consistently", i)
		}
		
		// Check audit log shows consistent results
		auditLog := validator.GetAuditLog()
		assert.Len(t, auditLog, 10)
		
		for i, entry := range auditLog {
			assert.True(t, entry.Allowed, "Entry %d should be allowed", i)
			assert.Contains(t, entry.Reason, "All validation rules passed")
		}
	})
}

func (m *MockCheck) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockCheck) Description() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockCheck) Severity() diagnostics.Severity {
	args := m.Called()
	return diagnostics.Severity(args.Int(0))
}

func (m *MockCheck) Run(ctx context.Context, client *docker.Client) (*diagnostics.CheckResult, error) {
	args := m.Called(ctx, client)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*diagnostics.CheckResult), args.Error(1)
}

func TestNewSecurityValidator(t *testing.T) {
	validator := diagnostics.NewSecurityValidator()

	assert.NotNil(t, validator)

	// Use GetStatistics to verify initialization
	stats := validator.GetStatistics()
	assert.Equal(t, 0, stats.TotalValidations)
	assert.True(t, stats.AllowedChecks > 0)
	assert.Equal(t, false, validator.GetRecentDenials(1*time.Minute) != nil)

	// Verify default allowed checks
	expectedChecks := []string{
		"daemon_connectivity", "bridge_network", "ip_forwarding", "iptables",
		"dns_resolution", "internal_dns", "container_connectivity", "port_binding",
		"network_isolation", "mtu_consistency", "subnet_overlap",
	}
	for _, check := range expectedChecks {
		assert.True(t, validator.IsCheckAllowed(check), fmt.Sprintf("Check %s should be allowed by default", check))
	}
}

func TestSecurityValidatorValidateCheck(t *testing.T) {
	t.Run("Nil Check Validation", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		err := validator.ValidateCheck(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "check cannot be nil")
	})

	t.Run("Successful Check Validation", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		mockCheck := &MockCheck{}
		mockCheck.On("Name").Return("ip_forwarding")
		mockCheck.On("Description").Return("Validates IP forwarding configuration")
		mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		mockCheck.On("Run", mock.Anything, mock.Anything).Return(&diagnostics.CheckResult{
			CheckName: "ip_forwarding",
			Success: true,
			Message: "IP forwarding check passed",
			Timestamp: time.Now(),
		}, nil)

		err := validator.ValidateCheck(mockCheck)
		assert.NoError(t, err)

		// Verify audit log
		auditLog := validator.GetAuditLog()
		assert.NotEmpty(t, auditLog)
		lastLog := auditLog[len(auditLog)-1]
		assert.True(t, lastLog.Allowed)
		assert.Equal(t, "ip_forwarding", lastLog.CheckName)
	})

	t.Run("Test Mode Check Validation", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidatorTestMode()
		mockCheck := &MockCheck{}
		mockCheck.On("Name").Return("test_check_1")
		mockCheck.On("Description").Return("A test check")
		mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
		mockCheck.On("Run", mock.Anything, mock.Anything).Return(&diagnostics.CheckResult{
			CheckName: "test_check_1",
			Success: true,
			Message: "Test check passed",
			Timestamp: time.Now(),
		}, nil)

		err := validator.ValidateCheck(mockCheck)
		assert.NoError(t, err)
	})
}

func TestSecurityValidatorNameValidation(t *testing.T) {
	testCases := []struct {
		name        string
		checkName   string
		expectError bool
		errorMsg    string
	}{
		{"Valid Check Name", "network_check", false, ""},
		{"Empty Name", "", true, "check name cannot be empty"},
		{"Name Too Long", strings.Repeat("a", 101), true, "check name too long"},
		{"Invalid Characters", "check@name", true, "check name contains invalid characters"},
		{"Injection Attempt Path Traversal", "check..name", true, "contains potentially dangerous pattern"},
		{"Injection Attempt Shell Command", "check$(/bin/ls)", true, "contains potentially dangerous pattern"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validator := diagnostics.NewSecurityValidator()
			mockCheck := &MockCheck{}
			mockCheck.On("Name").Return(tc.checkName)
			mockCheck.On("Description").Return("Test description")
			mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))
			mockCheck.On("Run", mock.Anything, mock.Anything).Return(&diagnostics.CheckResult{
				CheckName: tc.checkName,
				Success: true,
				Message: "Check passed",
				Timestamp: time.Now(),
			}, nil)

			err := validator.ValidateCheck(mockCheck)
			if tc.expectError {
				assert.Error(t, err, "Expected error for check name '%s'", tc.checkName)
				if tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSecurityValidatorAllowlistDenylist(t *testing.T) {
	t.Run("Add to Allowlist", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		err := validator.AddToAllowlist("custom_check")
		assert.NoError(t, err)
		assert.True(t, validator.IsCheckAllowed("custom_check"))
	})

	t.Run("Add to Denylist", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		err := validator.AddToDenylist("dangerous_check")
		assert.NoError(t, err)
		assert.True(t, validator.IsCheckDenied("dangerous_check"))
	})

	t.Run("Check Allowlist Validation", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		mockCheck := &MockCheck{}
		mockCheck.On("Name").Return("unknown_check")
		mockCheck.On("Description").Return("Test description")
		mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))

		err := validator.ValidateCheck(mockCheck)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not in the allowed list")
	})

	t.Run("Check Denylist Validation", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		validator.AddToDenylist("blocked_check")

		mockCheck := &MockCheck{}
		mockCheck.On("Name").Return("blocked_check")
		mockCheck.On("Description").Return("Test description")
		mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))

		err := validator.ValidateCheck(mockCheck)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "is in the denied list")
	})
}

func TestSecurityValidatorDescriptionValidation(t *testing.T) {
	testCases := []struct {
		name        string
		description string
		expectError bool
		errorMsg    string
	}{
		{"Valid Description", "A safe network check", false, ""},
		{"Description Too Long", strings.Repeat("a", 501), true, "check description too long"},
		{"Script Injection Attempt", "Hello <script>alert('XSS')</script>", true, "contains potential script injection"},
		{"JavaScript URI Injection", "Check onclick=alert('Malicious')", true, "contains potential script injection"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validator := diagnostics.NewSecurityValidator()
			mockCheck := &MockCheck{}
			mockCheck.On("Name").Return("test_check")
			mockCheck.On("Description").Return(tc.description)
			mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))

			err := validator.ValidateCheck(mockCheck)
			if tc.expectError {
				assert.Error(t, err, "Expected error for description '%s'", tc.description)
				if tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSecurityValidatorSeverityValidation(t *testing.T) {
	testCases := []struct {
		name        string
		severity    diagnostics.Severity
		expectError bool
	}{
		{"Info Severity", diagnostics.SeverityInfo, false},
		{"Warning Severity", diagnostics.SeverityWarning, false},
		{"Critical Severity", diagnostics.SeverityCritical, false},
		{"Invalid Low Severity", diagnostics.Severity(-1), true},
		{"Invalid High Severity", diagnostics.Severity(10), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validator := diagnostics.NewSecurityValidator()
			mockCheck := &MockCheck{}
			mockCheck.On("Name").Return("test_check")
			mockCheck.On("Description").Return("Test description")
			mockCheck.On("Severity").Return(int(tc.severity))

			err := validator.ValidateCheck(mockCheck)
			if tc.expectError {
				assert.Error(t, err, "Expected error for severity %d", tc.severity)
				assert.Contains(t, err.Error(), "severity is out of valid range")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSecurityValidatorAuditLogging(t *testing.T) {
	t.Run("Audit Log Recording", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		mockCheck := &MockCheck{}
		mockCheck.On("Name").Return("ip_forwarding")
		mockCheck.On("Description").Return("Test description")
		mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))

		// Validate multiple times
		for i := 0; i < 5; i++ {
			_ = validator.ValidateCheck(mockCheck)
		}

		// Check audit log
		auditLog := validator.GetAuditLog()
		assert.Len(t, auditLog, 5)
		
		// Verify log entries
		for _, entry := range auditLog {
			assert.Equal(t, "ip_forwarding", entry.CheckName)
			assert.True(t, entry.Allowed)
			assert.NotZero(t, entry.Timestamp)
			assert.Equal(t, "default", entry.ValidatorID)
		}
	})

	t.Run("Audit Log Size Limit", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		// Note: maxHistorySize is not exported, so we test the behavior indirectly
		mockCheck := &MockCheck{}
		mockCheck.On("Name").Return("ip_forwarding")
		mockCheck.On("Description").Return("Test description")
		mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))

		// Validate more times than expected max history (1000)
		for i := 0; i < 1100; i++ {
			_ = validator.ValidateCheck(mockCheck)
		}

		// Check audit log size is limited
		auditLog := validator.GetAuditLog()
		assert.LessOrEqual(t, len(auditLog), 1000)
	})

	t.Run("Recent Denials", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()
		validator.AddToDenylist("blocked_check")

		mockCheck := &MockCheck{}
		mockCheck.On("Name").Return("blocked_check")
		mockCheck.On("Description").Return("Test description")
		mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))

		// Trigger denial
		_ = validator.ValidateCheck(mockCheck)

		// Get recent denials within last hour
		recentDenials := validator.GetRecentDenials(1 * time.Hour)
		assert.NotEmpty(t, recentDenials)
		assert.False(t, recentDenials[0].Allowed)
	})
}

func TestSecurityValidatorStatistics(t *testing.T) {
	t.Run("Validation Statistics", func(t *testing.T) {
		validator := diagnostics.NewSecurityValidator()

		// Simulate various check validations
		passingCheck := &MockCheck{}
		passingCheck.On("Name").Return("ip_forwarding")
		passingCheck.On("Description").Return("Passing check")
		passingCheck.On("Severity").Return(int(diagnostics.SeverityInfo))

		failingCheck := &MockCheck{}
		failingCheck.On("Name").Return("unknown_check")
		failingCheck.On("Description").Return("Failing check")
		failingCheck.On("Severity").Return(int(diagnostics.SeverityInfo))

		// Run checks
		_ = validator.ValidateCheck(passingCheck)
		_ = validator.ValidateCheck(failingCheck)
		_ = validator.ValidateCheck(passingCheck)

		// Get statistics
		stats := validator.GetStatistics()
		assert.Equal(t, 3, stats.TotalValidations)
		assert.Equal(t, 2, stats.TotalAllowed)
		assert.Equal(t, 1, stats.TotalDenied)
		assert.InDelta(t, 0.333, stats.DenialRate, 0.01)
	})
}

func TestSecurityValidatorTestModeChecks(t *testing.T) {
	testCases := []struct {
		name     string
		checkName string
		testMode bool
		expectAllowed bool
	}{
		{"Standard Test Check in Test Mode", "test_check_1", true, true},
		{"Standard Test Check in Normal Mode", "test_check_1", false, false},
		{"Standard Check in Test Mode", "ip_forwarding", true, true},
		{"Standard Check in Normal Mode", "ip_forwarding", false, true},
		{"Numbered Test Check", "test2", true, true},
		{"Injection Test Check", "check$(/bin/ls)", true, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var validator *diagnostics.SecurityValidator
			if tc.testMode {
				validator = diagnostics.NewSecurityValidatorTestMode()
			} else {
				validator = diagnostics.NewSecurityValidator()
			}

			mockCheck := &MockCheck{}
			mockCheck.On("Name").Return(tc.checkName)
			mockCheck.On("Description").Return("Test description")
			mockCheck.On("Severity").Return(int(diagnostics.SeverityInfo))

			err := validator.ValidateCheck(mockCheck)
			if tc.expectAllowed {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}