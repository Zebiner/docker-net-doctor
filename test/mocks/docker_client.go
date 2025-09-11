// Package mocks provides shared mock implementations for testing Docker Network Doctor.
// This package consolidates all mock implementations to avoid duplication across test files.
package mocks

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/mock"
	"github.com/zebiner/docker-net-doctor/internal/diagnostics"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// MockDockerClient provides a comprehensive mock implementation for testing.
// Note: This mock can be used in unit tests but not directly with diagnostic checks
// that expect *docker.Client. For those tests, use integration testing or skip them.
type MockDockerClient struct {
	mock.Mock
}

// ListContainers mocks the container listing functionality
func (m *MockDockerClient) ListContainers(ctx context.Context) ([]types.Container, error) {
	args := m.Called(ctx)
	return args.Get(0).([]types.Container), args.Error(1)
}

// ExecInContainer mocks command execution within containers
func (m *MockDockerClient) ExecInContainer(ctx context.Context, containerID string, cmd []string) (string, error) {
	args := m.Called(ctx, containerID, cmd)
	return args.String(0), args.Error(1)
}

// GetNetworkInfo mocks network information retrieval
func (m *MockDockerClient) GetNetworkInfo() ([]docker.NetworkDiagnostic, error) {
	args := m.Called()
	return args.Get(0).([]docker.NetworkDiagnostic), args.Error(1)
}

// GetContainerNetworkConfig mocks container network configuration retrieval
func (m *MockDockerClient) GetContainerNetworkConfig(containerID string) (*docker.ContainerNetworkInfo, error) {
	args := m.Called(containerID)
	return args.Get(0).(*docker.ContainerNetworkInfo), args.Error(1)
}

// InspectContainer mocks container inspection
func (m *MockDockerClient) InspectContainer(containerID string) (types.ContainerJSON, error) {
	args := m.Called(containerID)
	return args.Get(0).(types.ContainerJSON), args.Error(1)
}

// Ping mocks Docker daemon connectivity check
func (m *MockDockerClient) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// Close mocks client cleanup
func (m *MockDockerClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

// MockDiagnosticEngine provides a mock implementation of the diagnostic engine interface
type MockDiagnosticEngine struct {
	mock.Mock
}

// Run mocks the execution of diagnostic checks
func (m *MockDiagnosticEngine) Run(ctx context.Context) (*diagnostics.Results, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*diagnostics.Results), args.Error(1)
}

// Helper functions for creating common mock data structures

// CreateMockContainer creates a mock container for testing
func CreateMockContainer(id, name string) types.Container {
	return types.Container{
		ID:    id,
		Names: []string{"/" + name}, // Docker API prefixes names with /
	}
}

// CreateMockNetworkDiagnostic creates a mock network diagnostic for testing
func CreateMockNetworkDiagnostic(name, subnet, gateway string) docker.NetworkDiagnostic {
	return docker.NetworkDiagnostic{
		Name:   name,
		ID:     "network_" + name,
		Driver: "bridge",
		Scope:  "local",
		IPAM: docker.IPAMConfig{
			Driver: "default",
			Configs: []docker.IPAMConfigBlock{
				{
					Subnet:  subnet,
					Gateway: gateway,
				},
			},
		},
		Containers: make(map[string]docker.ContainerEndpoint),
	}
}