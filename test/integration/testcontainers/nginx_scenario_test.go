// test/integration/testcontainers/nginx_scenario_test.go
//go:build integration

package testcontainers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zebiner/docker-net-doctor/internal/diagnostics"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// TestNginxConnectivityScenario tests nginx container connectivity and diagnostic checks
func TestNginxConnectivityScenario(t *testing.T) {
	if !isDockerAvailable() {
		t.Skip("Docker not available, skipping nginx connectivity scenario test")
	}

	// Initialize Docker-in-Docker support
	dindSupport := NewDockerInDockerSupport()
	err := dindSupport.ConfigureForEnvironment()
	require.NoError(t, err, "Failed to configure Docker-in-Docker support")

	// Setup test context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Initialize container lifecycle manager
	clm := NewContainerLifecycleManager(ctx)
	defer clm.CleanupAll(t)

	// Create custom networks
	network1 := clm.CreateNetwork(t, "nginx-test-net1", "172.30.0.0/16")
	network2 := clm.CreateNetwork(t, "nginx-test-net2", "172.31.0.0/16")

	// Start nginx containers on different networks
	nginx1 := clm.StartNginxContainer(t, "nginx1", "nginx-test-net1", "80/tcp")
	nginx2 := clm.StartNginxContainer(t, "nginx2", "nginx-test-net2", "80/tcp")

	// Start alpine containers for testing connectivity
	client1 := clm.StartAlpineContainer(t, "client1", "nginx-test-net1")
	client2 := clm.StartAlpineContainer(t, "client2", "nginx-test-net2")

	// Initialize network test helper
	networkHelper := NewNetworkTestHelper(clm)
	httpHelper := NewHTTPTestHelper()

	// Test 1: Containers on the same network should be able to communicate
	t.Run("same_network_connectivity", func(t *testing.T) {
		// Test ping connectivity
		networkHelper.TestPingConnectivity(t, "client1", "nginx1")
		networkHelper.TestPingConnectivity(t, "client2", "nginx2")

		// Test HTTP connectivity
		networkHelper.TestHTTPConnectivity(t, "client1", "nginx1", 80)
		networkHelper.TestHTTPConnectivity(t, "client2", "nginx2", 80)
	})

	// Test 2: Containers on different networks should be isolated
	t.Run("network_isolation", func(t *testing.T) {
		isolationHelper := NewNetworkIsolationHelper(clm)
		isolationHelper.TestNetworkIsolation(t, "nginx1", "nginx2")
		isolationHelper.TestNetworkIsolation(t, "client1", "client2")
	})

	// Test 3: Port bindings should be accessible from host
	t.Run("port_binding_accessibility", func(t *testing.T) {
		networkHelper.TestPortBinding(t, "nginx1", "80/tcp", true)
		networkHelper.TestPortBinding(t, "nginx2", "80/tcp", true)

		// Get mapped ports and test HTTP access from host
		mappedPort1, err := nginx1.MappedPort(ctx, "80/tcp")
		require.NoError(t, err)

		mappedPort2, err := nginx2.MappedPort(ctx, "80/tcp")
		require.NoError(t, err)

		httpHelper.TestHTTPEndpoint(t, fmt.Sprintf("http://localhost:%d", mappedPort1.Int()), 200)
		httpHelper.TestHTTPEndpoint(t, fmt.Sprintf("http://localhost:%d", mappedPort2.Int()), 200)
	})

	// Test 4: Run Docker Network Doctor diagnostics
	t.Run("diagnostic_checks", func(t *testing.T) {
		// Initialize Docker client
		dockerClient, err := docker.NewClient(ctx)
		require.NoError(t, err, "Failed to create Docker client")
		defer dockerClient.Close()

		// Create diagnostic engine
		config := &diagnostics.Config{
			Timeout:  30 * time.Second,
			Verbose:  true,
			Parallel: false, // Sequential for predictable results
		}
		engine := diagnostics.NewEngine(dockerClient, config)

		// Run diagnostic checks
		results, err := engine.Run(ctx)
		require.NoError(t, err, "Failed to run diagnostic checks")
		require.NotEmpty(t, results, "Expected diagnostic results")

		// Verify that at least some checks passed
		var passedChecks, failedChecks int
		for _, result := range results {
			t.Logf("Check: %s, Status: %s, Message: %s", result.CheckName, result.Status, result.Message)
			if result.Status == "PASS" {
				passedChecks++
			} else {
				failedChecks++
			}
		}

		assert.Greater(t, passedChecks, 0, "Expected at least one diagnostic check to pass")
		t.Logf("Diagnostic results: %d passed, %d failed", passedChecks, failedChecks)
	})

	// Test 5: Container health monitoring
	t.Run("container_health", func(t *testing.T) {
		networkHelper.WaitForContainerHealth(t, "nginx1", 30*time.Second)
		networkHelper.WaitForContainerHealth(t, "nginx2", 30*time.Second)

		// Verify containers are still running after all tests
		state1, err := nginx1.State(ctx)
		require.NoError(t, err)
		assert.True(t, state1.Running, "nginx1 should still be running")

		state2, err := nginx2.State(ctx)
		require.NoError(t, err)
		assert.True(t, state2.Running, "nginx2 should still be running")
	})

	// Test 6: Log analysis (useful for debugging)
	t.Run("log_analysis", func(t *testing.T) {
		logs1 := networkHelper.GetContainerLogs(t, "nginx1", 50)
		logs2 := networkHelper.GetContainerLogs(t, "nginx2", 50)

		t.Logf("Nginx1 logs (last 50 lines):\n%s", logs1)
		t.Logf("Nginx2 logs (last 50 lines):\n%s", logs2)

		// Verify logs contain expected nginx startup messages
		assert.Contains(t, logs1, "nginx", "nginx1 logs should contain nginx references")
		assert.Contains(t, logs2, "nginx", "nginx2 logs should contain nginx references")
	})

	t.Logf("✅ Nginx connectivity scenario completed successfully")
}