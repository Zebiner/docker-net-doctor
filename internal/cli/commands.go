// Package cli provides testable command implementations for the Docker Network Doctor CLI.
//
// This package extracts the core command logic from main.go to make it testable
// while maintaining the same functionality. The main.go file creates cobra commands
// that delegate to these testable implementations.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/diagnostics"
	"github.com/zebiner/docker-net-doctor/internal/docker"
	"gopkg.in/yaml.v3"
)

// DiagnosticsRunner interface allows for mocking in tests
type DiagnosticsRunner interface {
	Run(ctx context.Context) (*diagnostics.Results, error)
}

// DockerClientInterface defines the interface needed for CLI operations
type DockerClientInterface interface {
	Close() error
}

// RunDiagnostics executes the diagnostic engine with specified filters and configuration.
// This function is extracted from main.go to be testable.
func RunDiagnostics(containerFilter, networkFilter string, parallel bool, timeout time.Duration, verbose bool, outputFormat string, writer io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if verbose {
		fmt.Fprintln(writer, "Connecting to Docker daemon...")
	}

	// Create Docker client
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to Docker daemon: %w", err)
	}
	defer dockerClient.Close()

	// Configure diagnostic engine
	config := &diagnostics.Config{
		Parallel:     parallel,
		Timeout:      timeout,
		Verbose:      verbose,
		TargetFilter: containerFilter, // For now, use container filter as target
	}

	if verbose {
		fmt.Fprintf(writer, "Running diagnostics with config: parallel=%v, timeout=%v\n", parallel, timeout)
	}

	// Create and run diagnostic engine
	engine := diagnostics.NewEngine(dockerClient, config)
	results, err := engine.Run(ctx)
	if err != nil {
		return fmt.Errorf("diagnostic engine failed: %w", err)
	}

	return OutputResults(results, outputFormat, verbose, writer)
}

// RunDiagnosticsWithMocks allows injection of mocked dependencies for testing
func RunDiagnosticsWithMocks(args []string, engine DiagnosticsRunner, client DockerClientInterface, writer io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	defer client.Close() // Ensure client is closed like in the real implementation

	results, err := engine.Run(ctx)
	if err != nil {
		return fmt.Errorf("diagnostic engine failed: %w", err)
	}

	return OutputResults(results, "table", false, writer)
}

// OutputResults formats and displays diagnostic results in the requested format
func OutputResults(results *diagnostics.Results, outputFormat string, verbose bool, writer io.Writer) error {
	switch strings.ToLower(outputFormat) {
	case "json":
		return outputJSON(results, writer)
	case "yaml":
		return outputYAML(results, writer)
	case "table":
		return outputTable(results, verbose, writer)
	default:
		return fmt.Errorf("unsupported output format: %s", outputFormat)
	}
}

// outputJSON outputs diagnostic results in JSON format
func outputJSON(results *diagnostics.Results, writer io.Writer) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(results)
}

// outputYAML outputs diagnostic results in YAML format
func outputYAML(results *diagnostics.Results, writer io.Writer) error {
	encoder := yaml.NewEncoder(writer)
	encoder.SetIndent(2)
	defer encoder.Close()
	return encoder.Encode(results)
}

// outputTable outputs diagnostic results in a human-readable table format
func outputTable(results *diagnostics.Results, verbose bool, writer io.Writer) error {
	fmt.Fprintln(writer, "🔍 Docker Network Doctor - Diagnostic Results")
	fmt.Fprintln(writer, "====================================================")
	fmt.Fprintln(writer)

	// Summary
	fmt.Fprintln(writer, "📊 Summary:")
	fmt.Fprintf(writer, "  Total Checks: %d\n", results.Summary.TotalChecks)
	fmt.Fprintf(writer, "  Passed: %d ✅\n", results.Summary.PassedChecks)
	fmt.Fprintf(writer, "  Failed: %d ❌\n", results.Summary.FailedChecks)
	fmt.Fprintf(writer, "  Duration: %v\n\n", results.Duration)

	// Critical issues
	if len(results.Summary.CriticalIssues) > 0 {
		fmt.Fprintln(writer, "🚨 Critical Issues:")
		for _, issue := range results.Summary.CriticalIssues {
			fmt.Fprintf(writer, "  • %s\n", issue)
		}
		fmt.Fprintln(writer)
	}

	// Individual check results
	fmt.Fprintln(writer, "📋 Detailed Results:")
	for _, check := range results.Checks {
		status := "✅ PASS"
		if !check.Success {
			status = "❌ FAIL"
		}

		fmt.Fprintf(writer, "  %s %s\n", status, check.CheckName)
		fmt.Fprintf(writer, "    Message: %s\n", check.Message)

		if verbose && len(check.Suggestions) > 0 {
			fmt.Fprintln(writer, "    Suggestions:")
			for _, suggestion := range check.Suggestions {
				fmt.Fprintf(writer, "      • %s\n", suggestion)
			}
		}

		if verbose && len(check.Details) > 0 {
			fmt.Fprintln(writer, "    Details:")
			for key, value := range check.Details {
				fmt.Fprintf(writer, "      %s: %v\n", key, value)
			}
		}
		fmt.Fprintln(writer)
	}

	return nil
}

// RunSpecificCheck runs a single diagnostic check based on the specified type
func RunSpecificCheck(checkType string, timeout time.Duration, verbose bool, outputFormat string, writer io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create Docker client
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to Docker daemon: %w", err)
	}
	defer dockerClient.Close()

	// Map check type to actual check implementation
	var check diagnostics.Check
	switch strings.ToLower(checkType) {
	case "dns":
		check = &diagnostics.DNSResolutionCheck{}
	case "bridge":
		check = &diagnostics.BridgeNetworkCheck{}
	case "connectivity":
		check = &diagnostics.ContainerConnectivityCheck{}
	case "ports":
		check = &diagnostics.PortBindingCheck{}
	case "iptables":
		check = &diagnostics.IptablesCheck{}
	case "forwarding":
		check = &diagnostics.IPForwardingCheck{}
	case "mtu":
		check = &diagnostics.MTUConsistencyCheck{}
	case "overlap":
		check = &diagnostics.SubnetOverlapCheck{}
	default:
		return fmt.Errorf("unknown check type: %s", checkType)
	}

	if verbose {
		fmt.Fprintf(writer, "Running check: %s\n", check.Description())
	}

	// Run the specific check
	result, err := check.Run(ctx, dockerClient)
	if err != nil {
		return fmt.Errorf("check failed: %w", err)
	}

	// Create results structure for output
	results := &diagnostics.Results{
		Checks:   []*diagnostics.CheckResult{result},
		Duration: time.Second, // Placeholder
		Summary: diagnostics.Summary{
			TotalChecks:  1,
			PassedChecks: 0,
			FailedChecks: 0,
		},
	}

	if result.Success {
		results.Summary.PassedChecks = 1
	} else {
		results.Summary.FailedChecks = 1
	}

	return OutputResults(results, outputFormat, verbose, writer)
}