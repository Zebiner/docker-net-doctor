// Package integration provides comprehensive integration testing infrastructure
// using Testcontainers for robust Docker-based testing
package integration

import (
	"context"
	"fmt"
	"net"
	"time"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestContainerManager manages the lifecycle of test containers
// and provides utilities for integration testing
type TestContainerManager struct {
	client     *client.Client
	containers []testcontainers.Container
	networks   []testcontainers.Network
	ctx        context.Context
	t          *testing.T
}

// NewTestContainerManager creates a new test container manager
func NewTestContainerManager(ctx context.Context, t *testing.T) (*TestContainerManager, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &TestContainerManager{
		client:     dockerClient,
		containers: make([]testcontainers.Container, 0),
		networks:   make([]testcontainers.Network, 0),
		ctx:        ctx,
		t:          t,
	}, nil
}

// ContainerSpec defines specifications for creating a test container
type ContainerSpec struct {
	Image            string
	ExposedPorts     []string
	Env              map[string]string
	WaitStrategy     wait.Strategy
	Networks         []string
	NetworkAliases   []string
	Cmd              []string
	Labels           map[string]string
	Name             string
	AutoRemove       bool
}

// CreateContainer creates and starts a container based on the provided spec
func (tcm *TestContainerManager) CreateContainer(spec ContainerSpec) (testcontainers.Container, error) {
	req := testcontainers.ContainerRequest{
		Image:        spec.Image,
		ExposedPorts: spec.ExposedPorts,
		Env:          spec.Env,
		WaitingFor:   spec.WaitStrategy,
		Networks:     spec.Networks,
		NetworkAliases: map[string][]string{},
		Cmd:          spec.Cmd,
		Labels:       spec.Labels,
		AutoRemove:   spec.AutoRemove,
	}

	// Set network aliases for all networks
	for _, networkName := range spec.Networks {
		req.NetworkAliases[networkName] = spec.NetworkAliases
	}

	container, err := testcontainers.GenericContainer(tcm.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Reuse:            false,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	tcm.containers = append(tcm.containers, container)
	return container, nil
}

// CreateNginxContainer creates a preconfigured nginx container for network testing
func (tcm *TestContainerManager) CreateNginxContainer(networkNames ...string) (testcontainers.Container, error) {
	spec := ContainerSpec{
		Image:        "nginx:alpine",
		ExposedPorts: []string{"80/tcp"},
		Networks:     networkNames,
		NetworkAliases: []string{"web-server"},
		WaitStrategy: wait.ForHTTP("/").WithPort("80/tcp").WithStartupTimeout(30 * time.Second),
		Labels: map[string]string{
			"test.type":    "nginx",
			"test.purpose": "connectivity",
		},
		AutoRemove: true,
	}

	return tcm.CreateContainer(spec)
}

// CreateRedisContainer creates a preconfigured redis container for testing
func (tcm *TestContainerManager) CreateRedisContainer(networkNames ...string) (testcontainers.Container, error) {
	spec := ContainerSpec{
		Image:        "redis:alpine",
		ExposedPorts: []string{"6379/tcp"},
		Networks:     networkNames,
		NetworkAliases: []string{"cache-server"},
		WaitStrategy: wait.ForLog("Ready to accept connections").WithStartupTimeout(30 * time.Second),
		Labels: map[string]string{
			"test.type":    "redis",
			"test.purpose": "service",
		},
		AutoRemove: true,
	}

	return tcm.CreateContainer(spec)
}

// CreateBusyboxContainer creates a lightweight busybox container for testing
func (tcm *TestContainerManager) CreateBusyboxContainer(networkNames ...string) (testcontainers.Container, error) {
	spec := ContainerSpec{
		Image:        "busybox:latest",
		Networks:     networkNames,
		NetworkAliases: []string{"utility"},
		Cmd:          []string{"sleep", "300"},
		WaitStrategy: wait.ForLog("").WithStartupTimeout(10 * time.Second),
		Labels: map[string]string{
			"test.type":    "busybox",
			"test.purpose": "utility",
		},
		AutoRemove: true,
	}

	return tcm.CreateContainer(spec)
}

// CreateCustomNetwork creates a custom Docker network for testing
func (tcm *TestContainerManager) CreateCustomNetwork(name string, subnet ...string) (testcontainers.Network, error) {
	req := testcontainers.GenericNetworkRequest{
		NetworkRequest: testcontainers.NetworkRequest{
			Name:           name,
			CheckDuplicate: true,
			Driver:         "bridge",
		},
	}

	// Configure subnet if provided
	if len(subnet) > 0 {
		req.NetworkRequest.IPAM = &network.IPAM{
			Driver: "default",
			Config: []network.IPAMConfig{
				{
					Subnet:  subnet[0],
				},
			},
		}
	}

	testNetwork, err := testcontainers.GenericNetwork(tcm.ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create network '%s': %w", name, err)
	}

	tcm.networks = append(tcm.networks, testNetwork)
	return testNetwork, nil
}

// GetContainerIP returns the IP address of a container in a specific network
func (tcm *TestContainerManager) GetContainerIP(container testcontainers.Container, networkName string) (string, error) {
	inspect, err := container.Inspect(tcm.ctx)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}

	if networkSettings, ok := inspect.NetworkSettings.Networks[networkName]; ok {
		return networkSettings.IPAddress, nil
	}

	return "", fmt.Errorf("container not connected to network '%s'", networkName)
}

// ExecCommand executes a command in a container and returns the output
func (tcm *TestContainerManager) ExecCommand(container testcontainers.Container, cmd []string) (string, error) {
	exitCode, reader, err := container.Exec(tcm.ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to execute command: %w", err)
	}

	if exitCode != 0 {
		return "", fmt.Errorf("command exited with code %d", exitCode)
	}

	buf := make([]byte, 1024)
	n, err := reader.Read(buf)
	if err != nil {
		return "", fmt.Errorf("failed to read command output: %w", err)
	}

	return string(buf[:n]), nil
}

// WaitForContainerConnectivity waits for a container to be reachable on a specific port
func (tcm *TestContainerManager) WaitForContainerConnectivity(container testcontainers.Container, port string, timeout time.Duration) error {
	host, err := container.Host(tcm.ctx)
	if err != nil {
		return fmt.Errorf("failed to get container host: %w", err)
	}

	mappedPort, err := container.MappedPort(tcm.ctx, nat.Port(port))
	if err != nil {
		return fmt.Errorf("failed to get mapped port: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%s", host, mappedPort.Port()), time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for connectivity to %s:%s", host, mappedPort.Port())
}

// GetDockerNetworkInfo retrieves Docker network information for analysis
func (tcm *TestContainerManager) GetDockerNetworkInfo() ([]network.Summary, error) {
	networks, err := tcm.client.NetworkList(tcm.ctx, network.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}
	return networks, nil
}

// GetDockerContainerInfo retrieves Docker container information for analysis
func (tcm *TestContainerManager) GetDockerContainerInfo() ([]types.Container, error) {
	containers, err := tcm.client.ContainerList(tcm.ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	return containers, nil
}

// Cleanup terminates all containers and networks created by this manager
func (tcm *TestContainerManager) Cleanup() {
	// Terminate containers
	for _, container := range tcm.containers {
		if err := container.Terminate(tcm.ctx); err != nil {
			tcm.t.Logf("Failed to terminate container: %v", err)
		}
	}

	// Remove networks
	for _, testNetwork := range tcm.networks {
		if err := testNetwork.Remove(tcm.ctx); err != nil {
			tcm.t.Logf("Failed to remove network: %v", err)
		}
	}

	// Close Docker client
	if err := tcm.client.Close(); err != nil {
		tcm.t.Logf("Failed to close Docker client: %v", err)
	}
}

// RequireDockerAvailable checks if Docker is available and skips test if not
func RequireDockerAvailable(t *testing.T) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skip("Docker not available:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = dockerClient.Ping(ctx)
	if err != nil {
		t.Skip("Docker daemon not available:", err)
	}

	dockerClient.Close()
}