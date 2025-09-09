// test/integration/testcontainers/helpers.go
//go:build integration

package testcontainers

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ContainerLifecycleManager manages the lifecycle of test containers
type ContainerLifecycleManager struct {
	ctx        context.Context
	containers map[string]testcontainers.Container
	networks   map[string]testcontainers.Network
}

// NewContainerLifecycleManager creates a new container lifecycle manager
func NewContainerLifecycleManager(ctx context.Context) *ContainerLifecycleManager {
	return &ContainerLifecycleManager{
		ctx:        ctx,
		containers: make(map[string]testcontainers.Container),
		networks:   make(map[string]testcontainers.Network),
	}
}

// CreateNetwork creates a Docker network with specified configuration
func (clm *ContainerLifecycleManager) CreateNetwork(t *testing.T, name, subnet string) testcontainers.Network {
	req := testcontainers.GenericNetworkRequest{
		NetworkRequest: testcontainers.NetworkRequest{
			Name:   name,
			Driver: "bridge",
			IPAM: &testcontainers.CustomIPAM{
				Driver: "default",
				Config: []testcontainers.CustomIPAMConfig{
					{
						Subnet: subnet,
					},
				},
			},
			CheckDuplicate: true,
		},
	}

	network, err := testcontainers.GenericNetwork(clm.ctx, req)
	require.NoError(t, err, "Failed to create network %s", name)

	clm.networks[name] = network
	t.Logf("Created network: %s with subnet: %s", name, subnet)
	return network
}

// StartNginxContainer starts an nginx container with custom configuration
func (clm *ContainerLifecycleManager) StartNginxContainer(t *testing.T, name, networkName string, exposedPort string) testcontainers.Container {
	req := testcontainers.ContainerRequest{
		Image:        "nginx:1.25-alpine",
		Name:         name,
		ExposedPorts: []string{exposedPort},
		Networks:     []string{networkName},
		WaitingFor:   wait.ForHTTP("/").WithPort("80/tcp").WithStartupTimeout(60 * time.Second),
		Env: map[string]string{
			"NGINX_ENVSUBST_OUTPUT_DIR": "/etc/nginx",
		},
	}

	container, err := testcontainers.GenericContainer(clm.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "Failed to start nginx container %s", name)

	clm.containers[name] = container
	
	// Get container details for logging
	ip, _ := container.ContainerIP(clm.ctx)
	port, _ := container.MappedPort(clm.ctx, "80/tcp")
	t.Logf("Started nginx container: %s, IP: %s, Port: %d", name, ip, port.Int())
	
	return container
}

// StartRedisContainer starts a Redis container with custom configuration
func (clm *ContainerLifecycleManager) StartRedisContainer(t *testing.T, name, networkName string) testcontainers.Container {
	req := testcontainers.ContainerRequest{
		Image:      "redis:7.2-alpine",
		Name:       name,
		Networks:   []string{networkName},
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor: wait.ForLog("Ready to accept connections").WithStartupTimeout(60 * time.Second),
		Cmd:        []string{"redis-server", "--bind", "0.0.0.0"},
	}

	container, err := testcontainers.GenericContainer(clm.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "Failed to start Redis container %s", name)

	clm.containers[name] = container
	
	// Get container details for logging
	ip, _ := container.ContainerIP(clm.ctx)
	port, _ := container.MappedPort(clm.ctx, "6379/tcp")
	t.Logf("Started Redis container: %s, IP: %s, Port: %d", name, ip, port.Int())
	
	return container
}

// StartAlpineContainer starts an Alpine Linux container for testing network connectivity
func (clm *ContainerLifecycleManager) StartAlpineContainer(t *testing.T, name, networkName string) testcontainers.Container {
	req := testcontainers.ContainerRequest{
		Image:    "alpine:3.19",
		Name:     name,
		Networks: []string{networkName},
		Cmd:      []string{"sleep", "3600"}, // Keep container running
		WaitingFor: wait.ForExec([]string{"echo", "ready"}).WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(clm.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "Failed to start Alpine container %s", name)

	clm.containers[name] = container
	
	// Get container details for logging
	ip, _ := container.ContainerIP(clm.ctx)
	t.Logf("Started Alpine container: %s, IP: %s", name, ip)
	
	return container
}

// GetContainer returns a container by name
func (clm *ContainerLifecycleManager) GetContainer(name string) (testcontainers.Container, bool) {
	container, exists := clm.containers[name]
	return container, exists
}

// GetNetwork returns a network by name
func (clm *ContainerLifecycleManager) GetNetwork(name string) (testcontainers.Network, bool) {
	network, exists := clm.networks[name]
	return network, exists
}

// CleanupAll terminates all containers and removes all networks
func (clm *ContainerLifecycleManager) CleanupAll(t *testing.T) {
	// Terminate containers
	for name, container := range clm.containers {
		if container != nil {
			terminateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := container.Terminate(terminateCtx)
			cancel()
			if err != nil {
				t.Logf("Warning: Failed to terminate container %s: %v", name, err)
			} else {
				t.Logf("Terminated container: %s", name)
			}
		}
	}

	// Remove networks
	for name, network := range clm.networks {
		if network != nil {
			removeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := network.Remove(removeCtx)
			cancel()
			if err != nil {
				t.Logf("Warning: Failed to remove network %s: %v", name, err)
			} else {
				t.Logf("Removed network: %s", name)
			}
		}
	}

	// Clear maps
	clm.containers = make(map[string]testcontainers.Container)
	clm.networks = make(map[string]testcontainers.Network)
}

// NetworkTestHelper provides utilities for testing network connectivity
type NetworkTestHelper struct {
	clm *ContainerLifecycleManager
}

// NewNetworkTestHelper creates a new network test helper
func NewNetworkTestHelper(clm *ContainerLifecycleManager) *NetworkTestHelper {
	return &NetworkTestHelper{clm: clm}
}

// TestPingConnectivity tests ICMP connectivity between containers
func (nth *NetworkTestHelper) TestPingConnectivity(t *testing.T, fromContainer, toContainer string) {
	from, exists := nth.clm.GetContainer(fromContainer)
	require.True(t, exists, "Source container %s not found", fromContainer)

	to, exists := nth.clm.GetContainer(toContainer)
	require.True(t, exists, "Target container %s not found", toContainer)

	// Get target IP
	targetIP, err := to.ContainerIP(nth.clm.ctx)
	require.NoError(t, err, "Failed to get IP of target container")

	// Execute ping command
	code, reader, err := from.Exec(nth.clm.ctx, []string{
		"ping", "-c", "3", "-W", "5", targetIP,
	})
	
	if reader != nil {
		defer reader.Close()
	}

	require.NoError(t, err, "Failed to execute ping command")
	assert.Equal(t, 0, code, "Ping from %s to %s (%s) failed", fromContainer, toContainer, targetIP)

	t.Logf("✓ Ping connectivity: %s -> %s (%s)", fromContainer, toContainer, targetIP)
}

// TestHTTPConnectivity tests HTTP connectivity between containers
func (nth *NetworkTestHelper) TestHTTPConnectivity(t *testing.T, fromContainer, toContainer string, port int) {
	from, exists := nth.clm.GetContainer(fromContainer)
	require.True(t, exists, "Source container %s not found", fromContainer)

	to, exists := nth.clm.GetContainer(toContainer)
	require.True(t, exists, "Target container %s not found", toContainer)

	// Get target IP
	targetIP, err := to.ContainerIP(nth.clm.ctx)
	require.NoError(t, err, "Failed to get IP of target container")

	// Test HTTP connectivity using wget
	url := fmt.Sprintf("http://%s:%d", targetIP, port)
	code, reader, err := from.Exec(nth.clm.ctx, []string{
		"wget", "-q", "-O", "/dev/null", "--timeout=10", url,
	})
	
	if reader != nil {
		defer reader.Close()
	}

	require.NoError(t, err, "Failed to execute wget command")
	assert.Equal(t, 0, code, "HTTP connectivity from %s to %s (%s:%d) failed", fromContainer, toContainer, targetIP, port)

	t.Logf("✓ HTTP connectivity: %s -> %s (%s:%d)", fromContainer, toContainer, targetIP, port)
}

// TestDNSResolution tests DNS resolution from within a container
func (nth *NetworkTestHelper) TestDNSResolution(t *testing.T, containerName, hostname string) {
	container, exists := nth.clm.GetContainer(containerName)
	require.True(t, exists, "Container %s not found", containerName)

	// Test DNS resolution using nslookup
	code, reader, err := container.Exec(nth.clm.ctx, []string{
		"nslookup", hostname,
	})
	
	if reader != nil {
		defer reader.Close()
	}

	require.NoError(t, err, "Failed to execute nslookup command")
	assert.Equal(t, 0, code, "DNS resolution of %s from %s failed", hostname, containerName)

	t.Logf("✓ DNS resolution: %s can resolve %s", containerName, hostname)
}

// TestPortBinding tests if a port is properly bound and accessible
func (nth *NetworkTestHelper) TestPortBinding(t *testing.T, containerName string, containerPort string, expectedAccessible bool) {
	container, exists := nth.clm.GetContainer(containerName)
	require.True(t, exists, "Container %s not found", containerName)

	// Get mapped port
	mappedPort, err := container.MappedPort(nth.clm.ctx, containerPort)
	require.NoError(t, err, "Failed to get mapped port for %s", containerPort)

	// Test if port is accessible from host
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", mappedPort.Int()), 5*time.Second)
	
	if expectedAccessible {
		require.NoError(t, err, "Expected port %s to be accessible, but connection failed", containerPort)
		if conn != nil {
			conn.Close()
		}
		t.Logf("✓ Port binding: %s port %s is accessible on host port %d", containerName, containerPort, mappedPort.Int())
	} else {
		require.Error(t, err, "Expected port %s to be inaccessible, but connection succeeded", containerPort)
		t.Logf("✓ Port binding: %s port %s is correctly inaccessible", containerName, containerPort)
	}
}

// WaitForContainerHealth waits for a container to become healthy
func (nth *NetworkTestHelper) WaitForContainerHealth(t *testing.T, containerName string, timeout time.Duration) {
	container, exists := nth.clm.GetContainer(containerName)
	require.True(t, exists, "Container %s not found", containerName)

	start := time.Now()
	for time.Since(start) < timeout {
		// Check if container is still running
		state, err := container.State(nth.clm.ctx)
		require.NoError(t, err, "Failed to get container state")
		
		if !state.Running {
			require.Fail(t, "Container %s stopped unexpectedly", containerName)
		}

		// For basic health check, try to get container IP
		_, err = container.ContainerIP(nth.clm.ctx)
		if err == nil {
			t.Logf("✓ Container health: %s is healthy", containerName)
			return
		}

		time.Sleep(1 * time.Second)
	}

	require.Fail(t, "Container %s did not become healthy within %v", containerName, timeout)
}

// GetContainerLogs retrieves logs from a container
func (nth *NetworkTestHelper) GetContainerLogs(t *testing.T, containerName string, maxLines int) string {
	container, exists := nth.clm.GetContainer(containerName)
	require.True(t, exists, "Container %s not found", containerName)

	logs, err := container.Logs(nth.clm.ctx)
	if err != nil {
		t.Logf("Warning: Failed to get logs for container %s: %v", containerName, err)
		return ""
	}
	defer logs.Close()

	// Read logs with size limit
	content, err := io.ReadAll(io.LimitReader(logs, 64*1024)) // 64KB limit
	if err != nil {
		t.Logf("Warning: Failed to read logs for container %s: %v", containerName, err)
		return ""
	}

	logContent := string(content)
	
	// Limit to maxLines if specified
	if maxLines > 0 {
		lines := strings.Split(logContent, "\n")
		if len(lines) > maxLines {
			lines = lines[len(lines)-maxLines:]
		}
		logContent = strings.Join(lines, "\n")
	}

	return logContent
}

// HTTPTestHelper provides utilities for testing HTTP services
type HTTPTestHelper struct {
	client *http.Client
}

// NewHTTPTestHelper creates a new HTTP test helper
func NewHTTPTestHelper() *HTTPTestHelper {
	return &HTTPTestHelper{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// TestHTTPEndpoint tests an HTTP endpoint accessibility and response
func (hth *HTTPTestHelper) TestHTTPEndpoint(t *testing.T, url string, expectedStatus int) {
	resp, err := hth.client.Get(url)
	require.NoError(t, err, "Failed to make HTTP request to %s", url)
	defer resp.Body.Close()

	assert.Equal(t, expectedStatus, resp.StatusCode, "Unexpected HTTP status code from %s", url)
	t.Logf("✓ HTTP endpoint test: %s returned status %d", url, resp.StatusCode)
}

// TestHTTPEndpointWithTimeout tests an HTTP endpoint with custom timeout
func (hth *HTTPTestHelper) TestHTTPEndpointWithTimeout(t *testing.T, url string, timeout time.Duration, expectedStatus int) {
	client := &http.Client{Timeout: timeout}
	
	resp, err := client.Get(url)
	require.NoError(t, err, "Failed to make HTTP request to %s within %v", url, timeout)
	defer resp.Body.Close()

	assert.Equal(t, expectedStatus, resp.StatusCode, "Unexpected HTTP status code from %s", url)
	t.Logf("✓ HTTP endpoint test (timeout %v): %s returned status %d", timeout, url, resp.StatusCode)
}

// NetworkIsolationHelper provides utilities for testing network isolation
type NetworkIsolationHelper struct {
	clm *ContainerLifecycleManager
}

// NewNetworkIsolationHelper creates a new network isolation helper
func NewNetworkIsolationHelper(clm *ContainerLifecycleManager) *NetworkIsolationHelper {
	return &NetworkIsolationHelper{clm: clm}
}

// TestNetworkIsolation verifies that containers on different networks cannot communicate
func (nih *NetworkIsolationHelper) TestNetworkIsolation(t *testing.T, container1, container2 string) {
	c1, exists := nih.clm.GetContainer(container1)
	require.True(t, exists, "Container %s not found", container1)

	c2, exists := nih.clm.GetContainer(container2)
	require.True(t, exists, "Container %s not found", container2)

	// Get target IP
	targetIP, err := c2.ContainerIP(nih.clm.ctx)
	require.NoError(t, err, "Failed to get IP of target container")

	// Attempt ping - should fail due to network isolation
	code, reader, err := c1.Exec(nih.clm.ctx, []string{
		"ping", "-c", "1", "-W", "2", targetIP,
	})
	
	if reader != nil {
		defer reader.Close()
	}

	// We expect the ping to fail (non-zero exit code) due to network isolation
	if err == nil {
		assert.NotEqual(t, 0, code, "Expected ping to fail due to network isolation, but it succeeded from %s to %s (%s)", container1, container2, targetIP)
		t.Logf("✓ Network isolation: %s cannot reach %s (%s) as expected", container1, container2, targetIP)
	} else {
		// Command execution failure is also acceptable as it indicates isolation
		t.Logf("✓ Network isolation: %s cannot reach %s (%s) - command failed as expected", container1, container2, targetIP)
	}
}