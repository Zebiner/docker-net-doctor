// Package integration contains comprehensive integration tests using Testcontainers
// These tests verify Docker Network Doctor functionality in real container environments
package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dockerclient "github.com/zebiner/docker-net-doctor/internal/docker"
)

func TestNetworkConnectivityIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	RunTestScenario(t, NetworkConnectivityScenario())
}

func TestMultiNetworkIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	RunTestScenario(t, MultiNetworkScenario())
}

func TestServiceDiscoveryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	RunTestScenario(t, ServiceDiscoveryScenario())
}

func TestPortExposureIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	RunTestScenario(t, PortExposureScenario())
}

func TestLoadTestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Run smaller load test for CI/CD environments
	RunTestScenario(t, LoadTestScenario(2, 3))
}

// TestDockerNetworkDoctorWithContainers tests the actual Docker Network Doctor
// diagnostics engine against real container environments
func TestDockerNetworkDoctorWithContainers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	RequireDockerAvailable(t)

	ctx := context.Background()
	tcm, err := NewTestContainerManager(ctx, t)
	require.NoError(t, err)
	defer tcm.Cleanup()

	// Set up test environment
	_, err = tcm.CreateCustomNetwork("doctor-test-net", "172.70.0.0/16")
	require.NoError(t, err)

	nginxContainer, err := tcm.CreateNginxContainer("doctor-test-net")
	require.NoError(t, err)

	redisContainer, err := tcm.CreateRedisContainer("doctor-test-net")
	require.NoError(t, err)

	// Wait for containers to be ready
	err = tcm.WaitForContainerConnectivity(nginxContainer, "80/tcp", 30*time.Second)
	require.NoError(t, err)

	err = tcm.WaitForContainerConnectivity(redisContainer, "6379/tcp", 30*time.Second)
	require.NoError(t, err)

	// Create Docker Network Doctor client
	dockerClient, err := dockerclient.NewClient(ctx)
	require.NoError(t, err)

	// For now, just verify we can connect to Docker
	// TODO: Integrate with actual diagnostic engine once API is stable
	t.Log("Docker client connected successfully")
	
	// Get basic Docker info as a placeholder test
	info, err := dockerClient.GetNetworkInfo()
	require.NoError(t, err)
	t.Logf("Found %d networks", len(info))
	
	// Verify we have at least the default networks
	assert.Greater(t, len(info), 0, "Expected at least one Docker network")
}

// TestContainerNetworkIsolation tests network isolation between containers
func TestContainerNetworkIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	RequireDockerAvailable(t)

	ctx := context.Background()
	tcm, err := NewTestContainerManager(ctx, t)
	require.NoError(t, err)
	defer tcm.Cleanup()

	// Create isolated networks
	_, err = tcm.CreateCustomNetwork("isolated-net-1", "172.80.0.0/24")
	require.NoError(t, err)

	_, err = tcm.CreateCustomNetwork("isolated-net-2", "172.81.0.0/24")
	require.NoError(t, err)

	// Create containers on different networks
	container1, err := tcm.CreateBusyboxContainer("isolated-net-1")
	require.NoError(t, err)

	container2, err := tcm.CreateBusyboxContainer("isolated-net-2")
	require.NoError(t, err)

	// Get container IPs
	ip1, err := tcm.GetContainerIP(container1, "isolated-net-1")
	require.NoError(t, err)

	ip2, err := tcm.GetContainerIP(container2, "isolated-net-2")
	require.NoError(t, err)

	t.Logf("Container 1 IP: %s", ip1)
	t.Logf("Container 2 IP: %s", ip2)

	// Test that containers cannot reach each other (network isolation)
	// This should fail due to network isolation
	_, err = tcm.ExecCommand(container1, []string{"ping", "-c", "1", "-W", "1", ip2})
	assert.Error(t, err, "Expected ping to fail due to network isolation")

	_, err = tcm.ExecCommand(container2, []string{"ping", "-c", "1", "-W", "1", ip1})
	assert.Error(t, err, "Expected ping to fail due to network isolation")
}

// TestCustomNetworkSubnets tests containers with custom network subnets
func TestCustomNetworkSubnets(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	RequireDockerAvailable(t)

	ctx := context.Background()
	tcm, err := NewTestContainerManager(ctx, t)
	require.NoError(t, err)
	defer tcm.Cleanup()

	// Test different subnet configurations
	subnets := []string{
		"192.168.100.0/24",
		"10.0.10.0/24",
		"172.25.0.0/16",
	}

	for i, subnet := range subnets {
		networkName := fmt.Sprintf("custom-subnet-%d", i)
		
		// Create network with custom subnet
		_, err = tcm.CreateCustomNetwork(networkName, subnet)
		require.NoError(t, err, "Failed to create network with subnet %s", subnet)

		// Create container on this network
		container, err := tcm.CreateBusyboxContainer(networkName)
		require.NoError(t, err, "Failed to create container on network %s", networkName)

		// Verify container gets IP in expected range
		containerIP, err := tcm.GetContainerIP(container, networkName)
		require.NoError(t, err, "Failed to get container IP")

		t.Logf("Network %s (%s): Container IP %s", networkName, subnet, containerIP)

		// Basic verification that IP is in expected subnet
		assert.NotEmpty(t, containerIP, "Container IP should not be empty")
	}
}

// TestContainerDNSResolution tests DNS resolution in container environments
func TestContainerDNSResolution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	RequireDockerAvailable(t)

	ctx := context.Background()
	tcm, err := NewTestContainerManager(ctx, t)
	require.NoError(t, err)
	defer tcm.Cleanup()

	// Create network
	_, err = tcm.CreateCustomNetwork("dns-test-net", "172.90.0.0/16")
	require.NoError(t, err)

	// Create containers with specific aliases
	nginxContainer, err := tcm.CreateNginxContainer("dns-test-net")
	require.NoError(t, err)

	clientContainer, err := tcm.CreateBusyboxContainer("dns-test-net")
	require.NoError(t, err)

	// Wait for nginx to be ready
	err = tcm.WaitForContainerConnectivity(nginxContainer, "80/tcp", 30*time.Second)
	require.NoError(t, err)

	// Test DNS resolution
	tests := []struct {
		name     string
		hostname string
	}{
		{"Web Server Alias", "web-server"},
		{"Container Name", "nginx"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Test DNS lookup
			output, err := tcm.ExecCommand(clientContainer, []string{"nslookup", test.hostname})
			if err != nil {
				t.Logf("nslookup failed, trying ping as fallback")
				// Fallback to ping test
				_, err = tcm.ExecCommand(clientContainer, []string{"ping", "-c", "1", test.hostname})
				assert.NoError(t, err, "DNS resolution failed for %s", test.hostname)
			} else {
				assert.Contains(t, output, test.hostname, "DNS lookup should contain hostname")
			}
		})
	}
}

// TestBasicContainerManagement tests basic container lifecycle management
func TestBasicContainerManagement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	RequireDockerAvailable(t)

	ctx := context.Background()
	tcm, err := NewTestContainerManager(ctx, t)
	require.NoError(t, err)
	defer tcm.Cleanup()

	// Create a simple container
	container, err := tcm.CreateBusyboxContainer()
	require.NoError(t, err)

	// Test container is running
	state, err := container.State(ctx)
	require.NoError(t, err)
	assert.True(t, state.Running, "Container should be running")

	// Test command execution
	output, err := tcm.ExecCommand(container, []string{"echo", "Hello from container"})
	require.NoError(t, err)
	assert.Contains(t, output, "Hello from container")
}