// Package docker provides a wrapper around the Docker SDK for diagnostic operations
package docker

import (
	"context"
	"fmt"
	"time"
	
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

// Client wraps the Docker client with diagnostic-specific methods
// This is the legacy client for backward compatibility
type Client struct {
	docker   *client.Client
	ctx      context.Context
	enhanced *EnhancedClient // Optional enhanced client for rate limiting
}

// ClientOptions configures the Docker client
type ClientOptions struct {
	// UseEnhanced enables the enhanced client with rate limiting
	UseEnhanced bool
	
	// EnhancedConfig provides configuration for the enhanced client
	EnhancedConfig *EnhancedClientConfig
}

// NewClient creates a new Docker client wrapper
func NewClient(ctx context.Context) (*Client, error) {
	return NewClientWithOptions(ctx, nil)
}

// NewClientWithOptions creates a new Docker client with options
func NewClientWithOptions(ctx context.Context, opts *ClientOptions) (*Client, error) {
	// Check if we should use the enhanced client
	if opts != nil && opts.UseEnhanced {
		// Create enhanced client
		config := opts.EnhancedConfig
		if config == nil {
			// Use defaults
			config = &EnhancedClientConfig{
				RateLimit:      5.0,
				RateBurst:      10,
				CacheTTL:       30 * time.Second,
				MaxCacheEntries: 100,
				MaxConnections: 10,
				Timeout:        30 * time.Second,
				MaxRetries:     3,
				RetryBackoff:   100 * time.Millisecond,
			}
		}
		
		enhanced, err := NewEnhancedClient(ctx, config)
		if err != nil {
			return nil, err
		}
		
		// Return a client that uses the enhanced client
		return &Client{
			docker:   enhanced.docker,
			ctx:      ctx,
			enhanced: enhanced,
		}, nil
	}
	
	// Create standard Docker client
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
	if c.enhanced != nil {
		return c.enhanced.Close()
	}
	if c.docker != nil {
		return c.docker.Close()
	}
	return nil
}

// ListContainers returns a list of running containers
func (c *Client) ListContainers(ctx context.Context) ([]types.Container, error) {
	// Use enhanced client if available
	if c.enhanced != nil {
		return c.enhanced.ListContainers(ctx)
	}
	
	// Fall back to standard implementation
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
	// Use enhanced client if available
	if c.enhanced != nil {
		return c.enhanced.GetNetworkInfo()
	}
	
	// Fall back to standard implementation
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

// GetContainerNetworkConfig gets network configuration for a specific container
func (c *Client) GetContainerNetworkConfig(containerID string) (*ContainerNetworkInfo, error) {
	// Use enhanced client if available
	if c.enhanced != nil {
		return c.enhanced.GetContainerNetworkConfig(containerID)
	}
	
	// Fall back to standard implementation
	container, err := c.docker.ContainerInspect(c.ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}
	
	info := &ContainerNetworkInfo{
		Hostname:     container.Config.Hostname,
		Domainname:   container.Config.Domainname,
		DNS:          container.HostConfig.DNS,
		DNSSearch:    container.HostConfig.DNSSearch,
		DNSOptions:   container.HostConfig.DNSOptions,
		Networks:     make(map[string]NetworkEndpoint),
	}
	
	for netName, netSettings := range container.NetworkSettings.Networks {
		info.Networks[netName] = NetworkEndpoint{
			IPAddress:   netSettings.IPAddress,
			Gateway:     netSettings.Gateway,
			MacAddress:  netSettings.MacAddress,
		}
	}
	
	return info, nil
}

// ExecInContainer runs a command inside a container
func (c *Client) ExecInContainer(ctx context.Context, containerID string, cmd []string) (string, error) {
	// Use enhanced client if available
	if c.enhanced != nil {
		return c.enhanced.ExecInContainer(ctx, containerID, cmd)
	}
	
	// Fall back to standard implementation
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

// Ping checks connectivity to the Docker daemon
func (c *Client) Ping(ctx context.Context) error {
	// Use enhanced client if available
	if c.enhanced != nil {
		return c.enhanced.Ping(ctx)
	}
	
	// Fall back to standard implementation
	_, err := c.docker.Ping(ctx)
	return err
}

// InspectContainer returns detailed information about a container
func (c *Client) InspectContainer(containerID string) (types.ContainerJSON, error) {
	// Use enhanced client if available
	if c.enhanced != nil {
		return c.enhanced.InspectContainer(containerID)
	}
	
	// Fall back to standard implementation
	return c.docker.ContainerInspect(c.ctx, containerID)
}

// GetMetrics returns client metrics if using enhanced client
func (c *Client) GetMetrics() *MetricsSnapshot {
	if c.enhanced != nil {
		metrics := c.enhanced.GetMetrics()
		return &metrics
	}
	return nil
}

// GetCacheStats returns cache statistics if using enhanced client
func (c *Client) GetCacheStats() *CacheStats {
	if c.enhanced != nil {
		stats := c.enhanced.GetCacheStats()
		return &stats
	}
	return nil
}

// InvalidateCache invalidates all cache entries if using enhanced client
func (c *Client) InvalidateCache() {
	if c.enhanced != nil {
		c.enhanced.InvalidateCache()
	}
}

// IsEnhanced returns true if using the enhanced client
func (c *Client) IsEnhanced() bool {
	return c.enhanced != nil
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
	Error      string // For networks that failed to inspect
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
	Hostname   string
	Domainname string
	DNS        []string
	DNSSearch  []string
	DNSOptions []string
	Networks   map[string]NetworkEndpoint
}

type NetworkEndpoint struct {
	IPAddress  string
	Gateway    string
	MacAddress string
}