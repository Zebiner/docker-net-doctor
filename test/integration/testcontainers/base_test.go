// test/integration/testcontainers/base_test.go
//go:build integration

package testcontainers

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/zebiner/docker-net-doctor/internal/diagnostics"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// TestMain sets up the test environment and ensures Docker is available
func TestMain(m *testing.M) {
	// Check if we're running in CI with Docker-in-Docker support
	if os.Getenv("CI") == "true" {
		setupCIEnvironment()
	}

	// Verify Docker is available
	if !isDockerAvailable() {
		fmt.Println("Docker is not available, skipping Testcontainers integration tests")
		os.Exit(0)
	}

	// Run tests
	code := m.Run()
	os.Exit(code)
}

// isDockerAvailable checks if Docker daemon is accessible
func isDockerAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := docker.NewClient(ctx)
	if err != nil {
		return false
	}
	defer client.Close()

	// Try a simple operation to verify connectivity
	_, err = client.ListContainers(ctx)
	return err == nil
}

// setupCIEnvironment configures environment for CI/CD Docker-in-Docker scenarios
func setupCIEnvironment() {
	// Set Docker host for CI environments if needed
	if dockerHost := os.Getenv("DOCKER_HOST"); dockerHost == "" {
		// Default Docker socket for most CI environments
		os.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")
	}

	// Configure Testcontainers for CI
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true") // Disable resource reaper in CI
	
	// For GitHub Actions and similar CI systems
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		os.Setenv("TESTCONTAINERS_HOST_OVERRIDE", "localhost")
	}
}

// ContainerTestSuite provides base functionality for all container-based tests
type ContainerTestSuite struct {
	ctx            context.Context
	cancel         context.CancelFunc
	dockerClient   *docker.Client
	diagnosticEngine *diagnostics.DiagnosticEngine
	containers     []testcontainers.Container
}

// SetupSuite initializes the test suite with Docker client and diagnostic engine
func (suite *ContainerTestSuite) SetupSuite(t *testing.T) {
	suite.ctx, suite.cancel = context.WithTimeout(context.Background(), 5*time.Minute)

	// Initialize Docker client
	var err error
	suite.dockerClient, err = docker.NewClient(suite.ctx)
	require.NoError(t, err, "Failed to create Docker client")

	// Initialize diagnostic engine with test-friendly config
	config := &diagnostics.Config{
		Timeout:  30 * time.Second,
		Verbose:  true,
		Parallel: false, // Sequential execution for predictable test results
	}
	suite.diagnosticEngine = diagnostics.NewEngine(suite.dockerClient, config)
	
	suite.containers = make([]testcontainers.Container, 0)
}

// TearDownSuite cleans up all containers and closes connections
func (suite *ContainerTestSuite) TearDownSuite(t *testing.T) {
	// Clean up containers in reverse order
	for i := len(suite.containers) - 1; i >= 0; i-- {
		container := suite.containers[i]
		if container != nil {
			terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := container.Terminate(terminateCtx)
			terminateCancel()
			if err != nil {
				t.Logf("Warning: Failed to terminate container: %v", err)
			}
		}
	}

	if suite.dockerClient != nil {
		suite.dockerClient.Close()
	}

	if suite.cancel != nil {
		suite.cancel()
	}
}

// CreateNginxContainer creates and starts an nginx container for testing
func (suite *ContainerTestSuite) CreateNginxContainer(t *testing.T, networkName string) testcontainers.Container {
	req := testcontainers.ContainerRequest{
		Image:        "nginx:alpine",
		ExposedPorts: []string{"80/tcp"},
		WaitingFor:   wait.ForHTTP("/").WithPort("80/tcp").WithStartupTimeout(60 * time.Second),
		Networks:     []string{networkName},
		Name:         fmt.Sprintf("test-nginx-%d", time.Now().Unix()),
		Env: map[string]string{
			"NGINX_ENVSUBST_OUTPUT_DIR": "/etc/nginx",
		},
	}

	container, err := testcontainers.GenericContainer(suite.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "Failed to create nginx container")

	suite.containers = append(suite.containers, container)
	return container
}

// CreateRedisContainer creates and starts a Redis container for testing
func (suite *ContainerTestSuite) CreateRedisContainer(t *testing.T, networkName string) testcontainers.Container {
	req := testcontainers.ContainerRequest{
		Image:        "redis:7-alpine",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(60 * time.Second),
		Networks:     []string{networkName},
		Name:         fmt.Sprintf("test-redis-%d", time.Now().Unix()),
		Cmd:          []string{"redis-server", "--bind", "0.0.0.0"},
	}

	container, err := testcontainers.GenericContainer(suite.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "Failed to create Redis container")

	suite.containers = append(suite.containers, container)
	return container
}

// CreateCustomNetwork creates a Docker network for testing with custom configuration
func (suite *ContainerTestSuite) CreateCustomNetwork(t *testing.T, name, driver, subnet string) testcontainers.Network {
	networkReq := testcontainers.GenericNetworkRequest{
		NetworkRequest: testcontainers.NetworkRequest{
			Name:   name,
			Driver: driver,
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

	network, err := testcontainers.GenericNetwork(suite.ctx, networkReq)
	require.NoError(t, err, "Failed to create custom network")

	return network
}

// WaitForContainerReady waits for a container to be ready and returns its details
func (suite *ContainerTestSuite) WaitForContainerReady(t *testing.T, container testcontainers.Container, port string) (string, int) {
	// Get container IP
	ip, err := container.ContainerIP(suite.ctx)
	require.NoError(t, err, "Failed to get container IP")

	// Get mapped port
	mappedPort, err := container.MappedPort(suite.ctx, port)
	require.NoError(t, err, "Failed to get mapped port")

	return ip, mappedPort.Int()
}

// RunDiagnosticCheck runs a specific diagnostic check and returns results
func (suite *ContainerTestSuite) RunDiagnosticCheck(t *testing.T, checkName string) []*diagnostics.CheckResult {
	results, err := suite.diagnosticEngine.Run(suite.ctx)
	require.NoError(t, err, "Failed to run diagnostic checks")

	// Filter results for the specific check if requested
	if checkName != "" {
		var filteredResults []*diagnostics.CheckResult
		for _, result := range results {
			if result.CheckName == checkName {
				filteredResults = append(filteredResults, result)
			}
		}
		return filteredResults
	}

	return results
}

// GetContainerLogs retrieves logs from a container for debugging
func (suite *ContainerTestSuite) GetContainerLogs(t *testing.T, container testcontainers.Container) string {
	logs, err := container.Logs(suite.ctx)
	if err != nil {
		t.Logf("Warning: Failed to get container logs: %v", err)
		return ""
	}
	defer logs.Close()

	// Read logs (simplified - in production you might want to limit the size)
	buf := make([]byte, 4096)
	n, _ := logs.Read(buf)
	return string(buf[:n])
}

// AssertNetworkConnectivity verifies network connectivity between containers
func (suite *ContainerTestSuite) AssertNetworkConnectivity(t *testing.T, fromContainer testcontainers.Container, toIP string, toPort int) {
	// Execute ping command in the container
	code, reader, err := fromContainer.Exec(suite.ctx, []string{"ping", "-c", "1", "-W", "5", toIP})
	require.NoError(t, err, "Failed to execute ping command")
	require.Equal(t, 0, code, "Ping command failed")
	
	if reader != nil {
		reader.Close()
	}

	// For HTTP connectivity, try wget/curl if available
	if toPort == 80 || toPort == 8080 {
		code, reader, err := fromContainer.Exec(suite.ctx, []string{"wget", "-q", "-O", "/dev/null", fmt.Sprintf("http://%s:%d", toIP, toPort)})
		if err == nil && reader != nil {
			reader.Close()
			require.Equal(t, 0, code, "HTTP connectivity test failed")
		}
	}
}