// test/integration/cli_test.go
// +build integration

package integration

import (
    "encoding/json"
    "os/exec"
    "strings"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/zebiner/docker-net-doctor/internal/diagnostics"
)

const (
    binaryPath = "../../bin/docker-net-doctor"
)

func TestCLI_Help(t *testing.T) {
    cmd := exec.Command(binaryPath, "--help")
    output, err := cmd.CombinedOutput()
    require.NoError(t, err)
    
    outputStr := string(output)
    assert.Contains(t, outputStr, "Docker Network Doctor")
    assert.Contains(t, outputStr, "Available Commands:")
    assert.Contains(t, outputStr, "diagnose")
    assert.Contains(t, outputStr, "check")
    assert.Contains(t, outputStr, "report")
}

func TestCLI_Version(t *testing.T) {
    cmd := exec.Command(binaryPath, "--version")
    output, err := cmd.CombinedOutput()
    require.NoError(t, err)
    
    outputStr := string(output)
    assert.Contains(t, outputStr, "netdoctor version")
}

func TestCLI_DiagnoseCommand(t *testing.T) {
    cmd := exec.Command(binaryPath, "diagnose", "--help")
    output, err := cmd.CombinedOutput()
    require.NoError(t, err)
    
    outputStr := string(output)
    assert.Contains(t, outputStr, "comprehensive suite")
    assert.Contains(t, outputStr, "--container")
    assert.Contains(t, outputStr, "--network")
    assert.Contains(t, outputStr, "--parallel")
}

func TestCLI_CheckCommand(t *testing.T) {
    cmd := exec.Command(binaryPath, "check", "--help")
    output, err := cmd.CombinedOutput()
    require.NoError(t, err)
    
    outputStr := string(output)
    assert.Contains(t, outputStr, "specific type of diagnostic")
    assert.Contains(t, outputStr, "dns, bridge, connectivity")
}

func TestCLI_CheckBridge(t *testing.T) {
    // Skip if Docker not available
    if !isDockerAvailable() {
        t.Skip("Docker not available, skipping integration test")
    }

    cmd := exec.Command(binaryPath, "check", "bridge")
    output, err := cmd.CombinedOutput()
    require.NoError(t, err)
    
    outputStr := string(output)
    assert.Contains(t, outputStr, "Docker Network Doctor")
    assert.Contains(t, outputStr, "bridge_network")
}

func TestCLI_CheckBridgeJSON(t *testing.T) {
    // Skip if Docker not available
    if !isDockerAvailable() {
        t.Skip("Docker not available, skipping integration test")
    }

    cmd := exec.Command(binaryPath, "check", "bridge", "--output", "json")
    output, err := cmd.CombinedOutput()
    require.NoError(t, err)
    
    // Verify it's valid JSON
    var results diagnostics.Results
    err = json.Unmarshal(output, &results)
    require.NoError(t, err)
    
    // Verify structure
    assert.Equal(t, 1, results.Summary.TotalChecks)
    assert.Len(t, results.Checks, 1)
    assert.Equal(t, "bridge_network", results.Checks[0].CheckName)
}

func TestCLI_CheckBridgeYAML(t *testing.T) {
    // Skip if Docker not available
    if !isDockerAvailable() {
        t.Skip("Docker not available, skipping integration test")
    }

    cmd := exec.Command(binaryPath, "check", "bridge", "--output", "yaml")
    output, err := cmd.CombinedOutput()
    require.NoError(t, err)
    
    outputStr := string(output)
    assert.Contains(t, outputStr, "checks:")
    assert.Contains(t, outputStr, "checkname: bridge_network")
    assert.Contains(t, outputStr, "summary:")
}

func TestCLI_CheckInvalidType(t *testing.T) {
    cmd := exec.Command(binaryPath, "check", "invalid_type")
    output, err := cmd.CombinedOutput()
    require.Error(t, err)
    
    outputStr := string(output)
    assert.Contains(t, outputStr, "unknown check type")
}

func TestCLI_DiagnoseWithTimeout(t *testing.T) {
    // Skip if Docker not available
    if !isDockerAvailable() {
        t.Skip("Docker not available, skipping integration test")
    }

    cmd := exec.Command(binaryPath, "diagnose", "--timeout", "10s", "--output", "json")
    output, err := cmd.CombinedOutput()
    
    if err != nil {
        // Command might fail due to Docker permissions, but should not timeout
        assert.NotContains(t, string(output), "context deadline exceeded")
    } else {
        // If successful, verify JSON structure
        var results diagnostics.Results
        err = json.Unmarshal(output, &results)
        require.NoError(t, err)
        assert.Greater(t, results.Summary.TotalChecks, 0)
    }
}

func TestCLI_ReportCommand(t *testing.T) {
    cmd := exec.Command(binaryPath, "report", "--help")
    output, err := cmd.CombinedOutput()
    require.NoError(t, err)
    
    outputStr := string(output)
    assert.Contains(t, outputStr, "comprehensive report")
    assert.Contains(t, outputStr, "--output-file")
    assert.Contains(t, outputStr, "--include-system")
    assert.Contains(t, outputStr, "--include-logs")
}

func TestCLI_GlobalFlags(t *testing.T) {
    cmd := exec.Command(binaryPath, "check", "bridge", "--verbose", "--timeout", "5s")
    output, err := cmd.CombinedOutput()
    
    if isDockerAvailable() {
        require.NoError(t, err)
        outputStr := string(output)
        assert.Contains(t, outputStr, "Running check:")
    } else {
        // Should fail gracefully with clear error message
        assert.Contains(t, string(output), "Docker daemon")
    }
}

// isDockerAvailable checks if Docker daemon is accessible
func isDockerAvailable() bool {
    cmd := exec.Command("docker", "info")
    return cmd.Run() == nil
}

// TestCLI_ValidArgs tests command autocompletion arguments
func TestCLI_ValidArgs(t *testing.T) {
    validChecks := []string{"dns", "bridge", "connectivity", "ports", "iptables", "forwarding", "mtu", "overlap"}
    
    for _, checkType := range validChecks {
        t.Run("check_"+checkType, func(t *testing.T) {
            if !isDockerAvailable() {
                t.Skip("Docker not available, skipping integration test")
            }
            
            cmd := exec.Command(binaryPath, "check", checkType)
            output, err := cmd.CombinedOutput()
            
            if err != nil {
                // Command might fail due to permissions, but not due to invalid argument
                outputStr := string(output)
                assert.NotContains(t, outputStr, "unknown check type")
                assert.NotContains(t, outputStr, "invalid argument")
            } else {
                require.NoError(t, err)
            }
        })
    }
}

// TestCLI_ErrorHandling tests various error conditions
func TestCLI_ErrorHandling(t *testing.T) {
    testCases := []struct {
        name          string
        args          []string
        expectError   bool
        expectedText  string
    }{
        {
            name:         "check without argument",
            args:         []string{"check"},
            expectError:  true,
            expectedText: "please specify a check type",
        },
        {
            name:         "invalid output format",
            args:         []string{"check", "bridge", "--output", "xml"},
            expectError:  true,
            expectedText: "unsupported output format",
        },
        {
            name:         "invalid timeout",
            args:         []string{"check", "bridge", "--timeout", "invalid"},
            expectError:  true,
            expectedText: "invalid duration",
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            cmd := exec.Command(binaryPath, tc.args...)
            output, err := cmd.CombinedOutput()
            
            if tc.expectError {
                assert.Error(t, err)
                assert.Contains(t, strings.ToLower(string(output)), strings.ToLower(tc.expectedText))
            } else {
                assert.NoError(t, err)
            }
        })
    }
}