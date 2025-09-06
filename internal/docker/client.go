// Package docker provides a wrapper around the Docker SDK for diagnostic operations
package docker

import (
    "context"
    "fmt"
    
    "github.com/docker/docker/api/types"
    "github.com/docker/docker/api/types/network"
    "github.com/docker/docker/client"
)

// Client wraps the Docker client with diagnostic-specific methods
type Client struct {
    docker *client.Client
    ctx    context.Context
}

// NewClient creates a new Docker client wrapper
func NewClient(ctx context.Context) (*Client, error) {
    dockerClient, err := client.NewClientWithOpts(
        client.FromEnv,
        client.WithAPIVersionNegotiation(),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to create Docker client: %w", err)
    }
    
    _, err = dockerClient.Ping(ctx)
    if err != nil {
        return nil, fmt.Errorf("cannot connect to Docker daemon: %w", err)
    }
    
    return &Client{
        docker: dockerClient,
        ctx:    ctx,
    }, nil
}

// Close releases the Docker client resources
func (c *Client) Close() error {
    if c.docker != nil {
        return c.docker.Close()
    }
    return nil
}

// ListContainers returns a list of running containers
// Note: In Docker v20.10.24, we use types.ContainerListOptions, not container.ListOptions
func (c *Client) ListContainers(ctx context.Context) ([]types.Container, error) {
    // The key change: using types.ContainerListOptions instead of container.ListOptions
    containers, err := c.docker.ContainerList(ctx, types.ContainerListOptions{
        All: false, // Only running containers
    })
    if err != nil {
        return nil, fmt.Errorf("failed to list containers: %w", err)
    }
    return containers, nil
}

// GetNetworkInfo retrieves information about Docker networks
func (c *Client) GetNetworkInfo() ([]NetworkDiagnostic, error) {
    networks, err := c.docker.NetworkList(c.ctx, types.NetworkListOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to list networks: %w", err)
    }
    
    var diagnostics []NetworkDiagnostic
    for _, net := range networks {
        netInspect, err := c.docker.NetworkInspect(c.ctx, net.ID, types.NetworkInspectOptions{
            Verbose: true,
        })
        if err != nil {
            diagnostics = append(diagnostics, NetworkDiagnostic{
                Name:  net.Name,
                ID:    net.ID,
                Error: err.Error(),
            })
            continue
        }
        
        diag := NetworkDiagnostic{
            Name:       netInspect.Name,
            ID:         netInspect.ID,
            Driver:     netInspect.Driver,
            Scope:      netInspect.Scope,
            Internal:   netInspect.Internal,
            IPAM:       convertIPAM(netInspect.IPAM),
            Containers: make(map[string]ContainerEndpoint),
        }
        
        for containerID, endpoint := range netInspect.Containers {
            diag.Containers[containerID] = ContainerEndpoint{
                Name:        endpoint.Name,
                IPv4Address: endpoint.IPv4Address,
                IPv6Address: endpoint.IPv6Address,
                MacAddress:  endpoint.MacAddress,
            }
        }
        
        diagnostics = append(diagnostics, diag)
    }
    
    return diagnostics, nil
}

// GetContainerNetworkConfig retrieves network configuration for a specific container
func (c *Client) GetContainerNetworkConfig(containerID string) (*ContainerNetworkInfo, error) {
    containerJSON, err := c.docker.ContainerInspect(c.ctx, containerID)
    if err != nil {
        return nil, fmt.Errorf("failed to inspect container: %w", err)
    }
    
    info := &ContainerNetworkInfo{
        ID:           containerJSON.ID,
        Name:         containerJSON.Name,
        PortBindings: make(map[string][]PortBinding),
    }
    
    for port, bindings := range containerJSON.HostConfig.PortBindings {
        for _, binding := range bindings {
            info.PortBindings[string(port)] = append(info.PortBindings[string(port)], PortBinding{
                HostIP:   binding.HostIP,
                HostPort: binding.HostPort,
            })
        }
    }
    
    return info, nil
}

// ExecInContainer runs a command inside a container
func (c *Client) ExecInContainer(ctx context.Context, containerID string, cmd []string) (string, error) {
    execConfig := types.ExecConfig{
        AttachStdout: true,
        AttachStderr: true,
        Cmd:         cmd,
    }
    
    execCreate, err := c.docker.ContainerExecCreate(ctx, containerID, execConfig)
    if err != nil {
        return "", fmt.Errorf("failed to create exec: %w", err)
    }
    
    response, err := c.docker.ContainerExecAttach(ctx, execCreate.ID, types.ExecStartCheck{})
    if err != nil {
        return "", fmt.Errorf("failed to start exec: %w", err)
    }
    defer response.Close()
    
    output := make([]byte, 4096)
    n, _ := response.Reader.Read(output)
    
    return string(output[:n]), nil
}

// Helper function to convert Docker's IPAM to our structure
func convertIPAM(ipam network.IPAM) IPAMConfig {
    config := IPAMConfig{
        Driver:  ipam.Driver,
        Configs: make([]IPAMConfigBlock, 0, len(ipam.Config)),
    }
    
    for _, cfg := range ipam.Config {
        config.Configs = append(config.Configs, IPAMConfigBlock{
            Subnet:  cfg.Subnet,
            Gateway: cfg.Gateway,
        })
    }
    
    return config
}

// Data structures for diagnostic information
type NetworkDiagnostic struct {
    Name       string
    ID         string
    Driver     string
    Scope      string
    Internal   bool
    IPAM       IPAMConfig
    Containers map[string]ContainerEndpoint
    Error      string
}

type IPAMConfig struct {
    Driver  string
    Configs []IPAMConfigBlock
}

type IPAMConfigBlock struct {
    Subnet  string
    Gateway string
}

type ContainerEndpoint struct {
    Name        string
    IPv4Address string
    IPv6Address string
    MacAddress  string
}

type ContainerNetworkInfo struct {
    ID           string
    Name         string
    PortBindings map[string][]PortBinding
}

type PortBinding struct {
    HostIP   string
    HostPort string
}
