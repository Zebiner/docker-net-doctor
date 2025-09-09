package cmdtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/zebiner/docker-net-doctor/cmd/docker-net-doctor"
	"github.com/zebiner/docker-net-doctor/internal/diagnostics"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// MockDockerClient provides a mock implementation of the Docker client
type MockDockerClient struct {
	mock.Mock
}

// MockDiagnosticEngine provides a mock diagnostic engine for testing
type MockDiagnosticEngine struct {
	mock.Mock
}

func (m *MockDockerClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockDiagnosticEngine) Run(ctx context.Context) (*diagnostics.Results, error) {
	args := m.Called(ctx)
	return args.Get(0).(*diagnostics.Results), args.Error(1)
}

// TestDiagnoseCommandStructure validates the core structure of the diagnose command
func TestDiagnoseCommandStructure(t *testing.T) {
	cmd := main.CreateDiagnoseCommand()

	assert.Equal(t, "diagnose", cmd.Use)
	assert.Contains(t, cmd.Short, "Run comprehensive Docker networking diagnostics")
	assert.Contains(t, cmd.Long, "diagnostic checks")

	// Test flags
	flags := cmd.Flags()
	assert.NotNil(t, flags.Lookup("container"), "Container flag should exist")
	assert.NotNil(t, flags.Lookup("network"), "Network flag should exist")
	assert.NotNil(t, flags.Lookup("parallel"), "Parallel flag should exist")
}

// TestDiagnoseCommandFlagDefaults validates default flag configurations
func TestDiagnoseCommandFlagDefaults(t *testing.T) {
	cmd := main.CreateDiagnoseCommand()

	containerFlag := cmd.Flags().Lookup("container")
	assert.NotNil(t, containerFlag)
	assert.Equal(t, "c", containerFlag.Shorthand)
	assert.Equal(t, "", containerFlag.DefValue)

	networkFlag := cmd.Flags().Lookup("network")
	assert.NotNil(t, networkFlag)
	assert.Equal(t, "n", networkFlag.Shorthand)
	assert.Equal(t, "", networkFlag.DefValue)

	parallelFlag := cmd.Flags().Lookup("parallel")
	assert.NotNil(t, parallelFlag)
	assert.Equal(t, "p", parallelFlag.Shorthand)
	assert.Equal(t, "true", parallelFlag.DefValue)
}

// TestDiagnoseCommandExecution tests various execution scenarios
func TestDiagnoseCommandExecution(t *testing.T) {
	testCases := []struct {
		name           string
		args           []string
		mockResults    *diagnostics.Results
		mockError      error
		expectedOutput string
		expectError    bool
	}{
		{
			name: "Successful Full Diagnostics",
			args: []string{"diagnose"},
			mockResults: &diagnostics.Results{
				Summary: diagnostics.Summary{
					TotalChecks:  11,
					PassedChecks: 9,
					FailedChecks: 2,
					CriticalIssues: []string{"Network isolation problem"},
				},
				Checks: []*diagnostics.CheckResult{
					{
						CheckName: "DNS Resolution",
						Success:   true,
						Message:   "DNS resolution working correctly",
					},
					{
						CheckName: "Network Connectivity",
						Success:   false,
						Message:   "Some connectivity issues detected",
						Suggestions: []string{"Check firewall rules"},
					},
				},
				Duration: 5 * time.Second,
			},
			expectedOutput: "Diagnostic Results",
		},
		{
			name: "Container-Specific Diagnostics",
			args: []string{"diagnose", "-c", "test-container"},
			mockResults: &diagnostics.Results{
				Summary: diagnostics.Summary{
					TotalChecks:  3,
					PassedChecks: 3,
					FailedChecks: 0,
				},
				Checks: []*diagnostics.CheckResult{
					{
						CheckName: "Container-Specific Check",
						Success:   true,
					},
				},
				Duration: 2 * time.Second,
			},
			expectedOutput: "Diagnostic Results",
		},
		{
			name: "Execution Error",
			args: []string{"diagnose"},
			mockError:   fmt.Errorf("docker daemon connection failed"),
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Mock Docker client and diagnostic engine
			mockClient := &MockDockerClient{}
			mockEngine := &MockDiagnosticEngine{}

			// Capture stdout
			var buf bytes.Buffer
			oldStdout := os.Stdout
			os.Stdout = os.NewFile(0, "stdout")
			defer func() { os.Stdout = oldStdout }()

			// Setup expectations
			mockClient.On("Close").Return(nil)
			mockEngine.On("Run", mock.Anything).
				Return(tc.mockResults, tc.mockError)

			// Execute with mocked dependencies
			err := main.RunDiagnostics(
				tc.args[1:],   // Container/network filters
				mockEngine,    // Mocked diagnostic engine
				mockClient,    // Mocked Docker client
				&buf,          // Output writer
			)

			// Verify results
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				output := buf.String()
				assert.Contains(t, output, tc.expectedOutput)
			}

			// Verify mock expectations
			mockClient.AssertExpectations(t)
			mockEngine.AssertExpectations(t)
		})
	}
}

// TestDiagnoseCommandOutputFormats validates different output formats
func TestDiagnoseCommandOutputFormats(t *testing.T) {
	testCases := []struct {
		name           string
		outputFormat   string
		validateFunc   func(t *testing.T, output []byte)
	}{
		{
			name:         "JSON Output",
			outputFormat: "json",
			validateFunc: func(t *testing.T, output []byte) {
				var result map[string]interface{}
				err := json.Unmarshal(output, &result)
				assert.NoError(t, err)
				assert.Contains(t, result, "Summary")
				assert.Contains(t, result, "Checks")
			},
		},
		{
			name:         "YAML Output",
			outputFormat: "yaml",
			validateFunc: func(t *testing.T, output []byte) {
				var result map[string]interface{}
				err := yaml.Unmarshal(output, &result)
				assert.NoError(t, err)
				assert.Contains(t, result, "summary")
				assert.Contains(t, result, "checks")
			},
		},
		{
			name:         "Table Output",
			outputFormat: "table",
			validateFunc: func(t *testing.T, output []byte) {
				outputStr := string(output)
				assert.Contains(t, outputStr, "Docker Network Doctor")
				assert.Contains(t, outputStr, "Total Checks")
				assert.Contains(t, outputStr, "Passed")
				assert.Contains(t, outputStr, "Failed")
			},
		},
	}

	mockResults := &diagnostics.Results{
		Summary: diagnostics.Summary{
			TotalChecks:  5,
			PassedChecks: 3,
			FailedChecks: 2,
		},
		Checks: []*diagnostics.CheckResult{
			{
				CheckName: "DNS Check",
				Success:   true,
			},
			{
				CheckName: "Connectivity Check",
				Success:   false,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var outputBuffer bytes.Buffer

			// Set output format
			main.OutputFormat = tc.outputFormat

			// Output results
			err := main.OutputResults(mockResults, &outputBuffer)
			assert.NoError(t, err)

			// Validate output
			tc.validateFunc(t, outputBuffer.Bytes())
		})
	}
}

// TestDiagnoseCommandTimeoutHandling validates timeout and context cancellation
func TestDiagnoseCommandTimeoutHandling(t *testing.T) {
	testCases := []struct {
		name        string
		timeout     time.Duration
		expectError bool
	}{
		{
			name:        "Short Timeout",
			timeout:     100 * time.Millisecond,
			expectError: true,
		},
		{
			name:        "Reasonable Timeout",
			timeout:     10 * time.Second,
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &MockDockerClient{}
			mockEngine := &MockDiagnosticEngine{}

			// Simulate a long-running diagnostic
			mockEngine.On("Run", mock.Anything).
				Return(&diagnostics.Results{}, tc.expectError)

			ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
			defer cancel()

			// Mock a slow diagnostic process
			if tc.expectError {
				time.Sleep(tc.timeout * 2)
			}

			// Execute diagnostics
			err := main.RunDiagnostics(
				[]string{},     // No filters
				mockEngine,     // Mocked engine
				mockClient,     // Mocked client
				os.Stdout,      // Output writer
			)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "context deadline exceeded")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Benchmark for diagnose command execution
func BenchmarkDiagnoseCommand(b *testing.B) {
	// Mocked dependencies
	mockClient := &MockDockerClient{}
	mockEngine := &MockDiagnosticEngine{}

	mockResults := &diagnostics.Results{
		Summary: diagnostics.Summary{
			TotalChecks:  11,
			PassedChecks: 9,
			FailedChecks: 2,
		},
		Checks: []*diagnostics.CheckResult{
			{CheckName: "DNS Check", Success: true},
			{CheckName: "Network Check", Success: false},
		},
	}

	mockEngine.On("Run", mock.Anything).Return(mockResults, nil)
	mockClient.On("Close").Return(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		main.RunDiagnostics(
			[]string{},     // No filters
			mockEngine,     // Mocked engine
			mockClient,     // Mocked client
			os.Stdout,      // Output writer
		)
	}
}