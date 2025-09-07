package diagnostics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockServiceDiscoveryClient implements DockerClient interface for service discovery testing
type mockServiceDiscoveryClient struct {
	containers     []types.Container
	containerDetails map[string]*types.ContainerJSON
	services       []swarm.Service
	networks       []types.NetworkResource
	shouldError    bool
	errorMessage   string
}

func (m *mockServiceDiscoveryClient) ListContainers(ctx context.Context) ([]types.Container, error) {
	if m.shouldError {
		return nil, fmt.Errorf(m.errorMessage)
	}
	return m.containers, nil
}

func (m *mockServiceDiscoveryClient) InspectContainer(ctx context.Context, containerID string) (*types.ContainerJSON, error) {
	if m.shouldError {
		return nil, fmt.Errorf(m.errorMessage)
	}
	if detail, ok := m.containerDetails[containerID]; ok {
		return detail, nil
	}
	return nil, fmt.Errorf("container not found: %s", containerID)
}

func TestServiceDiscoveryCheck_Name(t *testing.T) {
	check := &ServiceDiscoveryCheck{}
	assert.Equal(t, "service_discovery_networking", check.Name())
}

func TestServiceDiscoveryCheck_Description(t *testing.T) {
	check := &ServiceDiscoveryCheck{}
	expected := "Analyzes service discovery mechanisms and load balancing in Docker networks"
	assert.Equal(t, expected, check.Description())
}

func TestServiceDiscoveryCheck_Severity(t *testing.T) {
	check := &ServiceDiscoveryCheck{}
	assert.Equal(t, SeverityWarning, check.Severity())
}

func TestServiceDiscoveryCheck_Run_NoServices(t *testing.T) {
	mockClient := &mockServiceDiscoveryClient{
		containers:       []types.Container{},
		containerDetails: map[string]*types.ContainerJSON{},
		services:         []swarm.Service{},
		networks:         []types.NetworkResource{},
	}

	check := &ServiceDiscoveryCheck{
		useParallelExecution: false,
	}

	ctx := context.Background()
	result, err := check.RunWithClient(ctx, mockClient)

	require.NoError(t, err)
	assert.Equal(t, CheckStatusPass, result.Status)
	assert.Equal(t, "No services found to analyze", result.Message)
	assert.Contains(t, result.Details["total_services"], "0")
}

func TestServiceDiscoveryCheck_Run_BasicContainerServices(t *testing.T) {
	containers := []types.Container{
		{
			ID:    "service-container-1",
			Names: []string{"/web-service"},
			Labels: map[string]string{
				"com.docker.compose.service": "web",
				"traefik.enable":             "true",
			},
			Ports: []types.Port{
				{PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			},
		},
		{
			ID:    "service-container-2",
			Names: []string{"/api-service"},
			Labels: map[string]string{
				"com.docker.compose.service": "api",
			},
			Ports: []types.Port{
				{PrivatePort: 3000, Type: "tcp"},
			},
		},
	}

	containerDetails := map[string]*types.ContainerJSON{
		"service-container-1": {
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:   "service-container-1",
				Name: "/web-service",
				Config: &types.Config{
					Labels: map[string]string{
						"com.docker.compose.service": "web",
						"traefik.enable":             "true",
					},
				},
			},
			NetworkSettings: &types.NetworkSettings{
				Networks: map[string]*types.EndpointSettings{
					"web-network": {
						NetworkID: "net1",
						IPAddress: "172.18.0.2",
						Aliases:   []string{"web", "frontend"},
					},
				},
			},
		},
		"service-container-2": {
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:   "service-container-2",
				Name: "/api-service",
				Config: &types.Config{
					Labels: map[string]string{
						"com.docker.compose.service": "api",
					},
				},
			},
			NetworkSettings: &types.NetworkSettings{
				Networks: map[string]*types.EndpointSettings{
					"api-network": {
						NetworkID: "net2",
						IPAddress: "172.19.0.2",
						Aliases:   []string{"api", "backend"},
					},
				},
			},
		},
	}

	mockClient := &mockServiceDiscoveryClient{
		containers:       containers,
		containerDetails: containerDetails,
		services:         []swarm.Service{},
		networks:         []types.NetworkResource{},
	}

	check := &ServiceDiscoveryCheck{
		useParallelExecution: false,
	}

	ctx := context.Background()
	result, err := check.RunWithClient(ctx, mockClient)

	require.NoError(t, err)
	assert.Equal(t, CheckStatusPass, result.Status)
	assert.Contains(t, result.Message, "Service discovery analysis completed")
	assert.Contains(t, result.Details["total_services"], "2")
	assert.Contains(t, result.Details["compose_services"], "2")
	assert.Contains(t, result.Details["load_balancer_services"], "1") // traefik-enabled service
}

func TestServiceDiscoveryCheck_Run_DNSResolution(t *testing.T) {
	containers := []types.Container{
		{
			ID:    "dns-service-1",
			Names: []string{"/database"},
			Labels: map[string]string{
				"com.docker.compose.service": "database",
			},
		},
		{
			ID:    "dns-service-2",
			Names: []string{"/cache"},
			Labels: map[string]string{
				"com.docker.compose.service": "cache",
			},
		},
	}

	containerDetails := map[string]*types.ContainerJSON{
		"dns-service-1": {
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:   "dns-service-1",
				Name: "/database",
			},
			NetworkSettings: &types.NetworkSettings{
				Networks: map[string]*types.EndpointSettings{
					"app-network": {
						NetworkID: "net1",
						IPAddress: "172.20.0.10",
						Aliases:   []string{"database", "db", "postgres"},
					},
				},
			},
		},
		"dns-service-2": {
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:   "dns-service-2",
				Name: "/cache",
			},
			NetworkSettings: &types.NetworkSettings{
				Networks: map[string]*types.EndpointSettings{
					"app-network": {
						NetworkID: "net1",
						IPAddress: "172.20.0.11",
						Aliases:   []string{"cache", "redis"},
					},
				},
			},
		},
	}

	mockClient := &mockServiceDiscoveryClient{
		containers:       containers,
		containerDetails: containerDetails,
		networks: []types.NetworkResource{
			{
				ID:   "net1",
				Name: "app-network",
				IPAM: types.IPAM{
					Driver: "default",
					Config: []types.IPAMConfig{
						{Subnet: "172.20.0.0/24"},
					},
				},
			},
		},
	}

	check := &ServiceDiscoveryCheck{
		useParallelExecution: false,
	}

	ctx := context.Background()
	result, err := check.RunWithClient(ctx, mockClient)

	require.NoError(t, err)
	assert.Equal(t, CheckStatusPass, result.Status)
	assert.Contains(t, result.Details["dns_services"], "2")
	assert.Contains(t, result.Details["network_aliases"], "5") // Total aliases count
	assert.Contains(t, result.Details["cross_network_services"], "0")
}

func TestServiceDiscoveryCheck_Run_LoadBalancing(t *testing.T) {
	containers := []types.Container{
		{
			ID:    "lb-replica-1",
			Names: []string{"/web-app-1"},
			Labels: map[string]string{
				"com.docker.compose.service": "web-app",
				"traefik.enable":             "true",
				"traefik.http.services.web.loadbalancer.server.port": "80",
			},
		},
		{
			ID:    "lb-replica-2",
			Names: []string{"/web-app-2"},
			Labels: map[string]string{
				"com.docker.compose.service": "web-app",
				"traefik.enable":             "true",
				"traefik.http.services.web.loadbalancer.server.port": "80",
			},
		},
		{
			ID:    "lb-replica-3",
			Names: []string{"/web-app-3"},
			Labels: map[string]string{
				"com.docker.compose.service": "web-app",
				"traefik.enable":             "true",
				"traefik.http.services.web.loadbalancer.server.port": "80",
			},
		},
	}

	containerDetails := map[string]*types.ContainerJSON{}
	for _, container := range containers {
		containerDetails[container.ID] = &types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:     container.ID,
				Name:   container.Names[0],
				Config: &types.Config{Labels: container.Labels},
				State:  &types.ContainerState{Status: "running", Health: &types.Health{Status: "healthy"}},
			},
			NetworkSettings: &types.NetworkSettings{
				Networks: map[string]*types.EndpointSettings{
					"web-network": {
						NetworkID: "web-net",
						IPAddress: fmt.Sprintf("172.21.0.%d", 10+len(containerDetails)),
					},
				},
			},
		}
	}

	mockClient := &mockServiceDiscoveryClient{
		containers:       containers,
		containerDetails: containerDetails,
	}

	check := &ServiceDiscoveryCheck{
		useParallelExecution: false,
	}

	ctx := context.Background()
	result, err := check.RunWithClient(ctx, mockClient)

	require.NoError(t, err)
	assert.Equal(t, CheckStatusPass, result.Status)
	assert.Contains(t, result.Details["load_balancer_services"], "1")
	assert.Contains(t, result.Details["healthy_endpoints"], "3")
	assert.Contains(t, result.Details["load_balancer_mode"], "traefik")
}

func TestServiceDiscoveryCheck_Run_HealthChecks(t *testing.T) {
	containers := []types.Container{
		{
			ID:    "healthy-service",
			Names: []string{"/app-healthy"},
		},
		{
			ID:    "unhealthy-service",
			Names: []string{"/app-unhealthy"},
		},
		{
			ID:    "no-healthcheck",
			Names: []string{"/app-basic"},
		},
	}

	containerDetails := map[string]*types.ContainerJSON{
		"healthy-service": {
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:   "healthy-service",
				Name: "/app-healthy",
				State: &types.ContainerState{
					Status: "running",
					Health: &types.Health{
						Status:        "healthy",
						FailingStreak: 0,
						Log: []*types.HealthcheckResult{
							{Start: time.Now(), ExitCode: 0, Output: "OK"},
						},
					},
				},
			},
		},
		"unhealthy-service": {
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:   "unhealthy-service",
				Name: "/app-unhealthy",
				State: &types.ContainerState{
					Status: "running",
					Health: &types.Health{
						Status:        "unhealthy",
						FailingStreak: 3,
						Log: []*types.HealthcheckResult{
							{Start: time.Now(), ExitCode: 1, Output: "Connection refused"},
						},
					},
				},
			},
		},
		"no-healthcheck": {
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:   "no-healthcheck",
				Name: "/app-basic",
				State: &types.ContainerState{
					Status: "running",
					Health: nil, // No health check configured
				},
			},
		},
	}

	mockClient := &mockServiceDiscoveryClient{
		containers:       containers,
		containerDetails: containerDetails,
	}

	check := &ServiceDiscoveryCheck{
		useParallelExecution: false,
	}

	ctx := context.Background()
	result, err := check.RunWithClient(ctx, mockClient)

	require.NoError(t, err)
	assert.Equal(t, CheckStatusWarning, result.Status) // Warning due to unhealthy service
	assert.Contains(t, result.Message, "Service health issues detected")
	assert.Contains(t, result.Details["healthy_endpoints"], "1")
	assert.Contains(t, result.Details["unhealthy_endpoints"], "1")
	assert.Contains(t, result.Details["no_healthcheck"], "1")
}

func TestServiceDiscoveryCheck_Run_CrossNetworkDiscovery(t *testing.T) {
	containers := []types.Container{
		{
			ID:    "multi-network-service",
			Names: []string{"/gateway"},
		},
	}

	containerDetails := map[string]*types.ContainerJSON{
		"multi-network-service": {
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:   "multi-network-service",
				Name: "/gateway",
			},
			NetworkSettings: &types.NetworkSettings{
				Networks: map[string]*types.EndpointSettings{
					"frontend-network": {
						NetworkID: "net1",
						IPAddress: "172.18.0.10",
						Aliases:   []string{"gateway-frontend"},
					},
					"backend-network": {
						NetworkID: "net2", 
						IPAddress: "172.19.0.10",
						Aliases:   []string{"gateway-backend"},
					},
				},
			},
		},
	}

	mockClient := &mockServiceDiscoveryClient{
		containers:       containers,
		containerDetails: containerDetails,
		networks: []types.NetworkResource{
			{ID: "net1", Name: "frontend-network"},
			{ID: "net2", Name: "backend-network"},
		},
	}

	check := &ServiceDiscoveryCheck{
		useParallelExecution: false,
	}

	ctx := context.Background()
	result, err := check.RunWithClient(ctx, mockClient)

	require.NoError(t, err)
	assert.Equal(t, CheckStatusPass, result.Status)
	assert.Contains(t, result.Details["cross_network_services"], "1")
	assert.Contains(t, result.Details["multi_network_containers"], "1")
}

func TestServiceDiscoveryCheck_Run_ServiceMeshDetection(t *testing.T) {
	containers := []types.Container{
		{
			ID:    "envoy-proxy",
			Names: []string{"/envoy-sidecar"},
			Image: "envoyproxy/envoy:latest",
			Labels: map[string]string{
				"io.istio.proxy": "true",
			},
		},
		{
			ID:    "linkerd-proxy",
			Names: []string{"/linkerd-proxy"},
			Image: "gcr.io/linkerd-io/proxy:stable-2.11.0",
			Labels: map[string]string{
				"linkerd.io/proxy-injection": "enabled",
			},
		},
		{
			ID:    "app-container",
			Names: []string{"/my-app"},
			Image: "nginx:alpine",
		},
	}

	containerDetails := map[string]*types.ContainerJSON{}
	for _, container := range containers {
		containerDetails[container.ID] = &types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:     container.ID,
				Name:   container.Names[0],
				Config: &types.Config{Image: container.Image, Labels: container.Labels},
			},
		}
	}

	mockClient := &mockServiceDiscoveryClient{
		containers:       containers,
		containerDetails: containerDetails,
	}

	check := &ServiceDiscoveryCheck{
		useParallelExecution: false,
	}

	ctx := context.Background()
	result, err := check.RunWithClient(ctx, mockClient)

	require.NoError(t, err)
	assert.Equal(t, CheckStatusPass, result.Status)
	assert.Contains(t, result.Details["service_mesh_detected"], "true")
	assert.Contains(t, result.Details["mesh_type"], "istio,linkerd")
	assert.Contains(t, result.Details["proxy_containers"], "2")
}

func TestServiceDiscoveryCheck_Run_PerformanceWithinLimits(t *testing.T) {
	// Create test data with multiple services
	containers := make([]types.Container, 20)
	containerDetails := map[string]*types.ContainerJSON{}

	for i := 0; i < 20; i++ {
		containerID := fmt.Sprintf("perf-container-%d", i)
		containerName := fmt.Sprintf("/perf-service-%d", i)

		containers[i] = types.Container{
			ID:    containerID,
			Names: []string{containerName},
			Labels: map[string]string{
				"com.docker.compose.service": fmt.Sprintf("service-%d", i),
			},
		}

		containerDetails[containerID] = &types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:   containerID,
				Name: containerName,
				Config: &types.Config{
					Labels: containers[i].Labels,
				},
			},
			NetworkSettings: &types.NetworkSettings{
				Networks: map[string]*types.EndpointSettings{
					"perf-network": {
						NetworkID: "perf-net",
						IPAddress: fmt.Sprintf("172.22.0.%d", i+10),
					},
				},
			},
		}
	}

	mockClient := &mockServiceDiscoveryClient{
		containers:       containers,
		containerDetails: containerDetails,
	}

	check := &ServiceDiscoveryCheck{
		useParallelExecution: true,
	}

	ctx := context.Background()
	start := time.Now()
	result, err := check.RunWithClient(ctx, mockClient)
	duration := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, CheckStatusPass, result.Status)
	assert.Less(t, duration, 100*time.Millisecond, "Check should complete within 100ms")
	assert.Contains(t, result.Details["total_services"], "20")
}

func TestServiceDiscoveryCheck_Run_ParallelExecution(t *testing.T) {
	// Test with many containers to verify parallel execution benefits
	containers := make([]types.Container, 50)
	containerDetails := map[string]*types.ContainerJSON{}

	for i := 0; i < 50; i++ {
		containerID := fmt.Sprintf("parallel-container-%d", i)
		containerName := fmt.Sprintf("/parallel-service-%d", i)

		containers[i] = types.Container{
			ID:    containerID,
			Names: []string{containerName},
		}

		containerDetails[containerID] = &types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:   containerID,
				Name: containerName,
			},
			NetworkSettings: &types.NetworkSettings{
				Networks: map[string]*types.EndpointSettings{
					"parallel-network": {
						NetworkID: "parallel-net",
						IPAddress: fmt.Sprintf("172.23.0.%d", (i%240)+10),
					},
				},
			},
		}
	}

	mockClient := &mockServiceDiscoveryClient{
		containers:       containers,
		containerDetails: containerDetails,
	}

	// Test parallel vs sequential execution
	checkParallel := &ServiceDiscoveryCheck{
		useParallelExecution: true,
	}

	checkSequential := &ServiceDiscoveryCheck{
		useParallelExecution: false,
	}

	ctx := context.Background()

	// Parallel execution
	startParallel := time.Now()
	resultParallel, err := checkParallel.RunWithClient(ctx, mockClient)
	durationParallel := time.Since(startParallel)

	require.NoError(t, err)
	assert.Equal(t, CheckStatusPass, resultParallel.Status)

	// Sequential execution
	startSequential := time.Now()
	resultSequential, err := checkSequential.RunWithClient(ctx, mockClient)
	durationSequential := time.Since(startSequential)

	require.NoError(t, err)
	assert.Equal(t, CheckStatusPass, resultSequential.Status)

	// Both should be under 100ms, but parallel should generally be faster or comparable
	assert.Less(t, durationParallel, 100*time.Millisecond, "Parallel execution should be within 100ms")
	assert.Less(t, durationSequential, 100*time.Millisecond, "Sequential execution should be within 100ms")

	// Verify results are consistent
	assert.Equal(t, resultParallel.Details["total_services"], resultSequential.Details["total_services"])
}

func TestServiceDiscoveryCheck_Run_ErrorHandling(t *testing.T) {
	testCases := []struct {
		name          string
		setupMock     func(*mockServiceDiscoveryClient)
		expectedError string
	}{
		{
			name: "Docker Daemon Unavailable",
			setupMock: func(m *mockServiceDiscoveryClient) {
				m.shouldError = true
				m.errorMessage = "Cannot connect to the Docker daemon"
			},
			expectedError: "Cannot connect to the Docker daemon",
		},
		{
			name: "Permission Denied",
			setupMock: func(m *mockServiceDiscoveryClient) {
				m.shouldError = true
				m.errorMessage = "permission denied"
			},
			expectedError: "permission denied",
		},
		{
			name: "Container Inspect Failed",
			setupMock: func(m *mockServiceDiscoveryClient) {
				m.containers = []types.Container{{ID: "failing-container"}}
				m.containerDetails = map[string]*types.ContainerJSON{}
				m.shouldError = false
			},
			expectedError: "container not found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &mockServiceDiscoveryClient{}
			tc.setupMock(mockClient)

			check := &ServiceDiscoveryCheck{
				useParallelExecution: false,
			}

			ctx := context.Background()
			result, err := check.RunWithClient(ctx, mockClient)

			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
				if result != nil {
					assert.Equal(t, CheckStatusError, result.Status)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

// Benchmark tests for performance validation
func BenchmarkServiceDiscoveryCheck_Run_Sequential(b *testing.B) {
	containers := make([]types.Container, 100)
	containerDetails := map[string]*types.ContainerJSON{}

	for i := 0; i < 100; i++ {
		containerID := fmt.Sprintf("bench-container-%d", i)
		containers[i] = types.Container{
			ID:    containerID,
			Names: []string{fmt.Sprintf("/bench-service-%d", i)},
		}
		containerDetails[containerID] = &types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:   containerID,
				Name: containers[i].Names[0],
			},
		}
	}

	mockClient := &mockServiceDiscoveryClient{
		containers:       containers,
		containerDetails: containerDetails,
	}

	check := &ServiceDiscoveryCheck{
		useParallelExecution: false,
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := check.RunWithClient(ctx, mockClient)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkServiceDiscoveryCheck_Run_Parallel(b *testing.B) {
	containers := make([]types.Container, 100)
	containerDetails := map[string]*types.ContainerJSON{}

	for i := 0; i < 100; i++ {
		containerID := fmt.Sprintf("bench-container-%d", i)
		containers[i] = types.Container{
			ID:    containerID,
			Names: []string{fmt.Sprintf("/bench-service-%d", i)},
		}
		containerDetails[containerID] = &types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:   containerID,
				Name: containers[i].Names[0],
			},
		}
	}

	mockClient := &mockServiceDiscoveryClient{
		containers:       containers,
		containerDetails: containerDetails,
	}

	check := &ServiceDiscoveryCheck{
		useParallelExecution: true,
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := check.RunWithClient(ctx, mockClient)
		if err != nil {
			b.Fatal(err)
		}
	}
}