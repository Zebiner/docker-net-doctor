// Package integration provides predefined test scenarios for comprehensive testing
package integration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScenario represents a complete test scenario
type TestScenario struct {
	Name        string
	Description string
	Setup       func(*TestContainerManager) error
	Test        func(*TestContainerManager) error
	Cleanup     func(*TestContainerManager) error
}

// NetworkConnectivityScenario tests basic network connectivity between containers
func NetworkConnectivityScenario() TestScenario {
	return TestScenario{
		Name:        "NetworkConnectivity",
		Description: "Tests basic network connectivity between containers on custom networks",
		Setup: func(tcm *TestContainerManager) error {
			// Create custom networks
			_, err := tcm.CreateCustomNetwork("test-frontend", "172.30.0.0/16")
			if err != nil {
				return fmt.Errorf("failed to create frontend network: %w", err)
			}

			_, err = tcm.CreateCustomNetwork("test-backend", "172.31.0.0/16")
			if err != nil {
				return fmt.Errorf("failed to create backend network: %w", err)
			}

			// Create nginx container on frontend network
			_, err = tcm.CreateNginxContainer("test-frontend")
			if err != nil {
				return fmt.Errorf("failed to create nginx container: %w", err)
			}

			// Create client container on both networks
			_, err = tcm.CreateBusyboxContainer("test-frontend", "test-backend")
			if err != nil {
				return fmt.Errorf("failed to create client container: %w", err)
			}

			return nil
		},
		Test: func(tcm *TestContainerManager) error {
			containers := tcm.containers
			if len(containers) < 2 {
				return fmt.Errorf("expected at least 2 containers, got %d", len(containers))
			}

			clientContainer := containers[1] // busybox container
			
			// Test connectivity to nginx by alias
			output, err := tcm.ExecCommand(clientContainer, []string{"wget", "-qO-", "http://web-server"})
			if err != nil {
				return fmt.Errorf("failed to connect to nginx via alias: %w", err)
			}

			if !strings.Contains(output, "nginx") {
				return fmt.Errorf("unexpected response from nginx: %s", output)
			}

			return nil
		},
	}
}

// MultiNetworkScenario tests containers connected to multiple networks
func MultiNetworkScenario() TestScenario {
	return TestScenario{
		Name:        "MultiNetwork",
		Description: "Tests containers connected to multiple networks with different subnets",
		Setup: func(tcm *TestContainerManager) error {
			// Create three different networks
			networks := []struct {
				name   string
				subnet string
			}{
				{"net-a", "172.40.0.0/16"},
				{"net-b", "172.41.0.0/16"},
				{"net-c", "172.42.0.0/16"},
			}

			for _, net := range networks {
				_, err := tcm.CreateCustomNetwork(net.name, net.subnet)
				if err != nil {
					return fmt.Errorf("failed to create network %s: %w", net.name, err)
				}
			}

			// Create containers with different network configurations
			_, err := tcm.CreateNginxContainer("net-a", "net-b")
			if err != nil {
				return fmt.Errorf("failed to create nginx on net-a,net-b: %w", err)
			}

			_, err = tcm.CreateRedisContainer("net-b", "net-c")
			if err != nil {
				return fmt.Errorf("failed to create redis on net-b,net-c: %w", err)
			}

			_, err = tcm.CreateBusyboxContainer("net-a", "net-c")
			if err != nil {
				return fmt.Errorf("failed to create client on net-a,net-c: %w", err)
			}

			return nil
		},
		Test: func(tcm *TestContainerManager) error {
			if len(tcm.containers) < 3 {
				return fmt.Errorf("expected 3 containers, got %d", len(tcm.containers))
			}

			nginxContainer := tcm.containers[0]
			redisContainer := tcm.containers[1]
			clientContainer := tcm.containers[2]

			// Test that nginx is reachable from client via net-a
			nginxIPNetA, err := tcm.GetContainerIP(nginxContainer, "net-a")
			if err != nil {
				return fmt.Errorf("failed to get nginx IP on net-a: %w", err)
			}

			_, err = tcm.ExecCommand(clientContainer, []string{"ping", "-c", "1", nginxIPNetA})
			if err != nil {
				return fmt.Errorf("ping from client to nginx on net-a failed: %w", err)
			}

			// Test that redis is NOT directly reachable from client (different network)
			redisIPNetC, err := tcm.GetContainerIP(redisContainer, "net-c")
			if err != nil {
				return fmt.Errorf("failed to get redis IP on net-c: %w", err)
			}

			_, err = tcm.ExecCommand(clientContainer, []string{"ping", "-c", "1", redisIPNetC})
			if err != nil {
				return fmt.Errorf("ping from client to redis on net-c failed: %w", err)
			}

			return nil
		},
	}
}

// ServiceDiscoveryScenario tests DNS-based service discovery
func ServiceDiscoveryScenario() TestScenario {
	return TestScenario{
		Name:        "ServiceDiscovery",
		Description: "Tests DNS-based service discovery between containers",
		Setup: func(tcm *TestContainerManager) error {
			// Create a custom network
			_, err := tcm.CreateCustomNetwork("service-net", "172.50.0.0/16")
			if err != nil {
				return fmt.Errorf("failed to create service network: %w", err)
			}

			// Create services with specific aliases
			_, err = tcm.CreateNginxContainer("service-net")
			if err != nil {
				return fmt.Errorf("failed to create nginx: %w", err)
			}

			_, err = tcm.CreateRedisContainer("service-net")
			if err != nil {
				return fmt.Errorf("failed to create redis: %w", err)
			}

			// Create test client
			_, err = tcm.CreateBusyboxContainer("service-net")
			if err != nil {
				return fmt.Errorf("failed to create client: %w", err)
			}

			return nil
		},
		Test: func(tcm *TestContainerManager) error {
			if len(tcm.containers) < 3 {
				return fmt.Errorf("expected 3 containers, got %d", len(tcm.containers))
			}

			clientContainer := tcm.containers[2]

			// Test DNS resolution for web-server alias
			output, err := tcm.ExecCommand(clientContainer, []string{"nslookup", "web-server"})
			if err != nil {
				return fmt.Errorf("DNS lookup for web-server failed: %w", err)
			}

			if !strings.Contains(output, "web-server") {
				return fmt.Errorf("DNS resolution didn't return web-server: %s", output)
			}

			// Test DNS resolution for cache-server alias
			output, err = tcm.ExecCommand(clientContainer, []string{"nslookup", "cache-server"})
			if err != nil {
				return fmt.Errorf("DNS lookup for cache-server failed: %w", err)
			}

			if !strings.Contains(output, "cache-server") {
				return fmt.Errorf("DNS resolution didn't return cache-server: %s", output)
			}

			return nil
		},
	}
}

// PortExposureScenario tests port exposure and accessibility
func PortExposureScenario() TestScenario {
	return TestScenario{
		Name:        "PortExposure",
		Description: "Tests port exposure and external accessibility",
		Setup: func(tcm *TestContainerManager) error {
			// Create containers with exposed ports
			_, err := tcm.CreateNginxContainer()
			if err != nil {
				return fmt.Errorf("failed to create nginx: %w", err)
			}

			_, err = tcm.CreateRedisContainer()
			if err != nil {
				return fmt.Errorf("failed to create redis: %w", err)
			}

			return nil
		},
		Test: func(tcm *TestContainerManager) error {
			if len(tcm.containers) < 2 {
				return fmt.Errorf("expected 2 containers, got %d", len(tcm.containers))
			}

			nginxContainer := tcm.containers[0]
			redisContainer := tcm.containers[1]

			// Test nginx HTTP port accessibility
			err := tcm.WaitForContainerConnectivity(nginxContainer, "80/tcp", 30*time.Second)
			if err != nil {
				return fmt.Errorf("nginx port not accessible: %w", err)
			}

			// Test actual HTTP request to nginx
			host, err := nginxContainer.Host(tcm.ctx)
			if err != nil {
				return fmt.Errorf("failed to get nginx host: %w", err)
			}

			mappedPort, err := nginxContainer.MappedPort(tcm.ctx, "80/tcp")
			if err != nil {
				return fmt.Errorf("failed to get nginx mapped port: %w", err)
			}

			resp, err := http.Get(fmt.Sprintf("http://%s:%s", host, mappedPort.Port()))
			if err != nil {
				return fmt.Errorf("HTTP request to nginx failed: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected HTTP status: %d", resp.StatusCode)
			}

			// Test redis port accessibility
			err = tcm.WaitForContainerConnectivity(redisContainer, "6379/tcp", 30*time.Second)
			if err != nil {
				return fmt.Errorf("redis port not accessible: %w", err)
			}

			return nil
		},
	}
}

// RunTestScenario executes a test scenario with proper setup and cleanup
func RunTestScenario(t *testing.T, scenario TestScenario) {
	RequireDockerAvailable(t)

	ctx := context.Background()
	tcm, err := NewTestContainerManager(ctx, t)
	require.NoError(t, err, "Failed to create test container manager")

	defer tcm.Cleanup()

	t.Logf("Running scenario: %s - %s", scenario.Name, scenario.Description)

	// Setup
	if scenario.Setup != nil {
		err := scenario.Setup(tcm)
		require.NoError(t, err, "Scenario setup failed")
	}

	// Custom cleanup if provided
	if scenario.Cleanup != nil {
		defer func() {
			if err := scenario.Cleanup(tcm); err != nil {
				t.Logf("Scenario cleanup failed: %v", err)
			}
		}()
	}

	// Execute test
	err = scenario.Test(tcm)
	assert.NoError(t, err, "Scenario test failed")
}

// LoadTestScenario tests system behavior under load
func LoadTestScenario(containerCount int, requestsPerContainer int) TestScenario {
	return TestScenario{
		Name:        fmt.Sprintf("LoadTest_%d_containers_%d_requests", containerCount, requestsPerContainer),
		Description: fmt.Sprintf("Load test with %d containers and %d requests per container", containerCount, requestsPerContainer),
		Setup: func(tcm *TestContainerManager) error {
			// Create load test network
			_, err := tcm.CreateCustomNetwork("load-test-net", "172.60.0.0/16")
			if err != nil {
				return fmt.Errorf("failed to create load test network: %w", err)
			}

			// Create multiple nginx containers
			for i := 0; i < containerCount; i++ {
				_, err = tcm.CreateNginxContainer("load-test-net")
				if err != nil {
					return fmt.Errorf("failed to create nginx container %d: %w", i, err)
				}
			}

			// Create client container
			_, err = tcm.CreateBusyboxContainer("load-test-net")
			if err != nil {
				return fmt.Errorf("failed to create client container: %w", err)
			}

			return nil
		},
		Test: func(tcm *TestContainerManager) error {
			if len(tcm.containers) < containerCount+1 {
				return fmt.Errorf("expected %d containers, got %d", containerCount+1, len(tcm.containers))
			}

			clientContainer := tcm.containers[len(tcm.containers)-1]

			// Execute load test
			startTime := time.Now()
			successCount := 0
			errorCount := 0

			for i := 0; i < containerCount; i++ {
				containerIP, err := tcm.GetContainerIP(tcm.containers[i], "load-test-net")
				if err != nil {
					errorCount++
					continue
				}

				for j := 0; j < requestsPerContainer; j++ {
					_, err := tcm.ExecCommand(clientContainer, []string{"wget", "-qO-", "--timeout=5", fmt.Sprintf("http://%s", containerIP)})
					if err != nil {
						errorCount++
					} else {
						successCount++
					}
				}
			}

			duration := time.Since(startTime)
			totalRequests := containerCount * requestsPerContainer
			
			t := tcm.t
			t.Logf("Load Test Results:")
			t.Logf("  Duration: %v", duration)
			t.Logf("  Total Requests: %d", totalRequests)
			t.Logf("  Successful: %d", successCount)
			t.Logf("  Failed: %d", errorCount)
			t.Logf("  Success Rate: %.2f%%", float64(successCount)/float64(totalRequests)*100)
			t.Logf("  Requests/sec: %.2f", float64(totalRequests)/duration.Seconds())

			// Require at least 80% success rate
			successRate := float64(successCount) / float64(totalRequests)
			if successRate < 0.8 {
				return fmt.Errorf("success rate too low: %.2f%% < 80%%", successRate*100)
			}

			return nil
		},
	}
}

// GetAllTestScenarios returns all predefined test scenarios
func GetAllTestScenarios() []TestScenario {
	return []TestScenario{
		NetworkConnectivityScenario(),
		MultiNetworkScenario(),
		ServiceDiscoveryScenario(),
		PortExposureScenario(),
		LoadTestScenario(3, 5), // 3 containers, 5 requests each
	}
}