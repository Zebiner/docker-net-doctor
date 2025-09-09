package diagnosticstest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/zebiner/docker-net-doctor/internal/diagnostics"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// MockDockerClient implements a mock Docker client for testing
type MockDockerClient struct {
	mock.Mock
}

func (m *MockDockerClient) ListContainers(ctx context.Context) ([]docker.ContainerDiagnostic, error) {
	args := m.Called(ctx)
	return args.Get(0).([]docker.ContainerDiagnostic), args.Error(1)
}

func (m *MockDockerClient) ExecInContainer(ctx context.Context, containerID string, cmd []string) (string, error) {
	args := m.Called(ctx, containerID, cmd)
	return args.String(0), args.Error(1)
}

func (m *MockDockerClient) GetNetworkInfo() ([]docker.NetworkDiagnostic, error) {
	args := m.Called()
	return args.Get(0).([]docker.NetworkDiagnostic), args.Error(1)
}

// Test DNSResolutionCheck
func TestDNSResolutionCheck_SuccessScenario(t *testing.T) {
	mockClient := new(MockDockerClient)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Mocking containers for DNS resolution test
	mockContainers := []docker.ContainerDiagnostic{
		{
			ID:    "container1",
			Names: []string{"test_container_1"},
		},
	}

	mockClient.On("ListContainers", ctx).Return(mockContainers, nil)
	mockClient.On("ExecInContainer", ctx, "container1", []string{"nslookup", "google.com"}).
		Return("Server: 127.0.0.11\nAddress: 127.0.0.11#53\n\nNon-authoritative answer:\nName: google.com\nAddress: 172.217.16.142", nil)

	dnsCheck := &diagnostics.DNSResolutionCheck{}
	result, err := dnsCheck.Run(ctx, mockClient)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Message, "DNS resolution working")
	mockClient.AssertExpectations(t)
}

func TestDNSResolutionCheck_EmptyContainerScenario(t *testing.T) {
	mockClient := new(MockDockerClient)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mockClient.On("ListContainers", ctx).Return([]docker.ContainerDiagnostic{}, nil)

	dnsCheck := &diagnostics.DNSResolutionCheck{}
	result, err := dnsCheck.Run(ctx, mockClient)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "No running containers to test", result.Message)
	mockClient.AssertExpectations(t)
}

func TestDNSResolutionCheck_ContainerDNSFailureScenario(t *testing.T) {
	mockClient := new(MockDockerClient)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mockContainers := []docker.ContainerDiagnostic{
		{
			ID:    "container1",
			Names: []string{"failing_container"},
		},
	}

	mockClient.On("ListContainers", ctx).Return(mockContainers, nil)
	mockClient.On("ExecInContainer", ctx, "container1", []string{"nslookup", "google.com"}).
		Return("", errors.New("DNS resolution failed"))
	mockClient.On("ExecInContainer", ctx, "container1", []string{"cat", "/etc/resolv.conf"}).
		Return("nameserver 127.0.0.11", nil)

	dnsCheck := &diagnostics.DNSResolutionCheck{}
	result, err := dnsCheck.Run(ctx, mockClient)

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "DNS resolution failed")
	assert.NotEmpty(t, result.Suggestions)
	mockClient.AssertExpectations(t)
}

func TestDNSResolutionCheck_TimeoutScenario(t *testing.T) {
	mockClient := new(MockDockerClient)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	mockContainers := []docker.ContainerDiagnostic{
		{
			ID:    "container1",
			Names: []string{"timeout_container"},
		},
	}

	mockClient.On("ListContainers", ctx).Return(mockContainers, nil)
	mockClient.On("ExecInContainer", ctx, "container1", []string{"nslookup", "google.com"}).
		Return("", context.DeadlineExceeded)

	dnsCheck := &diagnostics.DNSResolutionCheck{}
	result, err := dnsCheck.Run(ctx, mockClient)

	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "DNS resolution failed")
	mockClient.AssertExpectations(t)
}

// Test InternalDNSCheck
func TestInternalDNSCheck_SuccessScenario(t *testing.T) {
	mockClient := new(MockDockerClient)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mockNetworks := []docker.NetworkDiagnostic{
		{
			Name: "custom_network",
			Containers: []docker.ContainerDiagnostic{
				{
					ID:    "container1",
					Names: []string{"test_container_1"},
				},
				{
					ID:    "container2",
					Names: []string{"test_container_2"},
				},
			},
		},
	}

	mockClient.On("GetNetworkInfo").Return(mockNetworks, nil)
	mockClient.On("ExecInContainer", ctx, "test_container_1", 
		[]string{"ping", "-c", "1", "test_container_2"}).
		Return("PING test_container_2 (172.18.0.2): 56 data bytes\n--- test_container_2 ping statistics ---\n1 packets transmitted, 1 packets received, 0% packet loss", nil)

	dnsCheck := &diagnostics.InternalDNSCheck{}
	result, err := dnsCheck.Run(ctx, mockClient)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Message, "Internal DNS resolution working")
	mockClient.AssertExpectations(t)
}

func TestInternalDNSCheck_NoCustomNetworksScenario(t *testing.T) {
	mockClient := new(MockDockerClient)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mockNetworks := []docker.NetworkDiagnostic{
		{
			Name: "bridge",
		},
		{
			Name: "host",
		},
	}

	mockClient.On("GetNetworkInfo").Return(mockNetworks, nil)

	dnsCheck := &diagnostics.InternalDNSCheck{}
	result, err := dnsCheck.Run(ctx, mockClient)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Message, "No custom networks found")
	mockClient.AssertExpectations(t)
}

func TestInternalDNSCheck_DNSResolutionFailureScenario(t *testing.T) {
	mockClient := new(MockDockerClient)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mockNetworks := []docker.NetworkDiagnostic{
		{
			Name: "custom_network",
			Containers: []docker.ContainerDiagnostic{
				{
					ID:    "container1",
					Names: []string{"test_container_1"},
				},
				{
					ID:    "container2",
					Names: []string{"test_container_2"},
				},
			},
		},
	}

	mockClient.On("GetNetworkInfo").Return(mockNetworks, nil)
	mockClient.On("ExecInContainer", ctx, "test_container_1", 
		[]string{"ping", "-c", "1", "test_container_2"}).
		Return("", errors.New("bad address"))

	dnsCheck := &diagnostics.InternalDNSCheck{}
	result, err := dnsCheck.Run(ctx, mockClient)

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "Internal DNS issues")
	assert.NotEmpty(t, result.Suggestions)
	mockClient.AssertExpectations(t)
}

func TestInternalDNSCheck_NetworkInfoRetrievalFailure(t *testing.T) {
	mockClient := new(MockDockerClient)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mockClient.On("GetNetworkInfo").Return([]docker.NetworkDiagnostic{}, errors.New("network retrieval failed"))

	dnsCheck := &diagnostics.InternalDNSCheck{}
	result, err := dnsCheck.Run(ctx, mockClient)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "network retrieval failed")
	mockClient.AssertExpectations(t)
}

// Benchmark DNS Checks
func BenchmarkDNSResolutionCheck(b *testing.B) {
	mockClient := new(MockDockerClient)
	ctx := context.Background()

	mockContainers := []docker.ContainerDiagnostic{
		{
			ID:    "container1",
			Names: []string{"test_container_1"},
		},
	}

	mockClient.On("ListContainers", ctx).Return(mockContainers, nil)
	mockClient.On("ExecInContainer", ctx, "container1", []string{"nslookup", "google.com"}).
		Return("Server: 127.0.0.11\nAddress: 127.0.0.11#53\n\nNon-authoritative answer:\nName: google.com\nAddress: 172.217.16.142", nil)

	dnsCheck := &diagnostics.DNSResolutionCheck{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = dnsCheck.Run(ctx, mockClient)
	}
}

func BenchmarkInternalDNSCheck(b *testing.B) {
	mockClient := new(MockDockerClient)
	ctx := context.Background()

	mockNetworks := []docker.NetworkDiagnostic{
		{
			Name: "custom_network",
			Containers: []docker.ContainerDiagnostic{
				{
					ID:    "container1",
					Names: []string{"test_container_1"},
				},
				{
					ID:    "container2",
					Names: []string{"test_container_2"},
				},
			},
		},
	}

	mockClient.On("GetNetworkInfo").Return(mockNetworks, nil)
	mockClient.On("ExecInContainer", ctx, "test_container_1", 
		[]string{"ping", "-c", "1", "test_container_2"}).
		Return("PING test_container_2 (172.18.0.2): 56 data bytes\n--- test_container_2 ping statistics ---\n1 packets transmitted, 1 packets received, 0% packet loss", nil)

	dnsCheck := &diagnostics.InternalDNSCheck{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = dnsCheck.Run(ctx, mockClient)
	}
}