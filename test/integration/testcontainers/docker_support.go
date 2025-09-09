// test/integration/testcontainers/docker_support.go
//go:build integration

package testcontainers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/testcontainers/testcontainers-go"
)

// DockerInDockerSupport provides utilities for running tests in CI/CD environments with Docker-in-Docker
type DockerInDockerSupport struct {
	// Configuration for different CI environments
	Environment string
}

// NewDockerInDockerSupport detects the CI environment and configures appropriate settings
func NewDockerInDockerSupport() *DockerInDockerSupport {
	support := &DockerInDockerSupport{}

	// Detect CI environment
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		support.Environment = "github-actions"
	} else if os.Getenv("GITLAB_CI") == "true" {
		support.Environment = "gitlab-ci"
	} else if os.Getenv("JENKINS_URL") != "" {
		support.Environment = "jenkins"
	} else if os.Getenv("CI") == "true" {
		support.Environment = "generic-ci"
	} else {
		support.Environment = "local"
	}

	return support
}

// ConfigureForEnvironment sets up Docker and Testcontainers for the detected CI environment
func (d *DockerInDockerSupport) ConfigureForEnvironment() error {
	switch d.Environment {
	case "github-actions":
		return d.configureGitHubActions()
	case "gitlab-ci":
		return d.configureGitLabCI()
	case "jenkins":
		return d.configureJenkins()
	case "generic-ci":
		return d.configureGenericCI()
	case "local":
		return d.configureLocal()
	default:
		return fmt.Errorf("unsupported environment: %s", d.Environment)
	}
}

// configureGitHubActions sets up Docker for GitHub Actions environment
func (d *DockerInDockerSupport) configureGitHubActions() error {
	// GitHub Actions typically has Docker available directly
	// Configure Testcontainers for GitHub Actions
	os.Setenv("TESTCONTAINERS_HOST_OVERRIDE", "localhost")
	
	// Disable Ryuk in CI to avoid permission issues
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	
	// Set reasonable timeouts for CI environment
	os.Setenv("TESTCONTAINERS_WAIT_STRATEGY_TIMEOUT", "300s")
	
	return nil
}

// configureGitLabCI sets up Docker for GitLab CI environment
func (d *DockerInDockerSupport) configureGitLabCI() error {
	// GitLab CI often uses Docker-in-Docker service
	if dockerHost := os.Getenv("DOCKER_HOST"); dockerHost == "" {
		// Check if docker service is available
		if _, err := os.Stat("/var/run/docker.sock"); err == nil {
			os.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")
		} else {
			// Assume Docker-in-Docker service at docker:2376
			os.Setenv("DOCKER_HOST", "tcp://docker:2376")
		}
	}
	
	// GitLab CI configuration
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	os.Setenv("TESTCONTAINERS_HOST_OVERRIDE", "docker")
	
	return nil
}

// configureJenkins sets up Docker for Jenkins environment
func (d *DockerInDockerSupport) configureJenkins() error {
	// Jenkins configuration depends on setup, use common defaults
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	
	// Jenkins often runs with Docker socket mounted
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		os.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")
	}
	
	return nil
}

// configureGenericCI sets up Docker for generic CI environments
func (d *DockerInDockerSupport) configureGenericCI() error {
	// Generic CI configuration - conservative settings
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	os.Setenv("TESTCONTAINERS_WAIT_STRATEGY_TIMEOUT", "300s")
	
	return nil
}

// configureLocal sets up Docker for local development
func (d *DockerInDockerSupport) configureLocal() error {
	// Local development - enable all features
	// Ryuk resource reaper is useful for cleanup in local development
	
	// Set reasonable defaults for local development
	if runtime.GOOS == "windows" {
		// Windows Docker Desktop configuration
		if os.Getenv("DOCKER_HOST") == "" {
			os.Setenv("DOCKER_HOST", "npipe:////./pipe/docker_engine")
		}
	}
	
	return nil
}

// CreateDockerComposeEnvironment creates a Docker Compose based test environment
func (d *DockerInDockerSupport) CreateDockerComposeEnvironment(ctx context.Context, composeFiles []string, identifier string) (*testcontainers.DockerCompose, error) {
	// Create temporary directory for compose files if they don't exist
	tempDir := filepath.Join(os.TempDir(), "docker-net-doctor-tests", identifier)
	err := os.MkdirAll(tempDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Create Docker Compose instance
	compose := testcontainers.NewDockerCompose(composeFiles...)
	compose = compose.WithCommand([]string{"up", "-d"}).
		WaitForService("app", wait.NewHTTPStrategy("/health").WithPort("8080").WithStartupTimeout(60*time.Second))

	// Start the compose environment
	err = compose.Up(ctx, testcontainers.Wait(true))
	if err != nil {
		return nil, fmt.Errorf("failed to start Docker Compose environment: %w", err)
	}

	return compose, nil
}

// GenerateDockerComposeFile generates a Docker Compose file for testing specific scenarios
func (d *DockerInDockerSupport) GenerateDockerComposeFile(scenario string) (string, error) {
	tempDir := filepath.Join(os.TempDir(), "docker-net-doctor-tests")
	err := os.MkdirAll(tempDir, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	composeContent := ""
	switch scenario {
	case "multi-network":
		composeContent = `
version: '3.8'
services:
  nginx1:
    image: nginx:alpine
    networks:
      - network1
    ports:
      - "8081:80"
  
  nginx2:
    image: nginx:alpine
    networks:
      - network2
    ports:
      - "8082:80"
  
  redis:
    image: redis:7-alpine
    networks:
      - network1
      - network2
    ports:
      - "6379:6379"

networks:
  network1:
    driver: bridge
    ipam:
      config:
        - subnet: 172.20.0.0/16
  network2:
    driver: bridge
    ipam:
      config:
        - subnet: 172.21.0.0/16
`
	case "port-conflicts":
		composeContent = `
version: '3.8'
services:
  app1:
    image: nginx:alpine
    ports:
      - "8080:80"
  
  app2:
    image: httpd:alpine
    ports:
      - "8080:80"  # Intentional conflict for testing
`
	case "dns-testing":
		composeContent = `
version: '3.8'
services:
  dns-server:
    image: nginx:alpine
    hostname: test-dns
    networks:
      test-net:
        aliases:
          - dns.test.local
          - api.test.local
  
  client:
    image: alpine:latest
    command: sleep 3600
    networks:
      - test-net
    depends_on:
      - dns-server

networks:
  test-net:
    driver: bridge
`
	default:
		return "", fmt.Errorf("unknown scenario: %s", scenario)
	}

	composeFile := filepath.Join(tempDir, fmt.Sprintf("docker-compose-%s.yml", scenario))
	err = os.WriteFile(composeFile, []byte(composeContent), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write compose file: %w", err)
	}

	return composeFile, nil
}

// ValidateDockerSetup performs basic validation of Docker setup for testing
func (d *DockerInDockerSupport) ValidateDockerSetup(ctx context.Context) error {
	// Create a simple test container to verify Docker is working
	req := testcontainers.ContainerRequest{
		Image:      "hello-world:latest",
		WaitingFor: wait.ForExit().WithExitTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return fmt.Errorf("docker validation failed: %w", err)
	}

	// Clean up test container
	defer func() {
		terminateCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		container.Terminate(terminateCtx)
	}()

	return nil
}

// GetOptimalWorkerCount returns the optimal number of workers for the current environment
func (d *DockerInDockerSupport) GetOptimalWorkerCount() int {
	switch d.Environment {
	case "local":
		// Use more workers locally where resources are typically more abundant
		return min(runtime.NumCPU(), 8)
	case "github-actions":
		// GitHub Actions typically provides 2 vCPUs
		return 2
	case "gitlab-ci", "jenkins", "generic-ci":
		// Conservative approach for CI environments
		return 2
	default:
		return 1
	}
}

// min returns the minimum of two integers (Go 1.21+ has this in stdlib, but keeping for compatibility)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}