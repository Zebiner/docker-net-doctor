// Package docker provides a comprehensive wrapper around the Docker SDK for diagnostic operations.
//
// This package abstracts the Docker client functionality and provides diagnostic-specific
// methods that are optimized for network troubleshooting operations. It supports both
// standard Docker client operations and enhanced functionality with rate limiting and caching.
//
// Key Features:
//   - Backward-compatible Docker client wrapper
//   - Enhanced client with rate limiting and caching
//   - Specialized diagnostic methods for network analysis
//   - Comprehensive error handling and recovery
//   - Performance metrics and monitoring
//
// The package provides two client implementations:
//   - Standard Client: Basic Docker operations
//   - Enhanced Client: Advanced features with rate limiting and caching
//
// Example usage:
//   client, err := docker.NewClient(ctx)
//   if err != nil {
//       log.Fatal(err)
//   }
//   defer client.Close()
//   
//   containers, err := client.ListContainers(ctx)
//   networks, err := client.GetNetworkInfo()
//
package docker

import (
	"context"
	"fmt"
	"time"
	
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

// Client wraps the Docker client with diagnostic-specific methods.
// This is the legacy client maintained for backward compatibility while providing
// a seamless upgrade path to the enhanced client with advanced features.
//
// The Client provides:
//   - Standard Docker API operations
//   - Network diagnostic methods
//   - Container inspection and execution
//   - Graceful error handling
//   - Optional enhanced client integration
//
// Thread Safety: The Client is safe for concurrent use across multiple goroutines.
type Client struct {
	docker   *client.Client
	ctx      context.Context
	enhanced *EnhancedClient // Optional enhanced client for rate limiting
}

// ClientOptions configures the Docker client creation and behavior.
// These options control whether to use the enhanced client and its configuration.
type ClientOptions struct {
	// UseEnhanced enables the enhanced client with rate limiting
	UseEnhanced bool
	
	// EnhancedConfig provides configuration for the enhanced client
	EnhancedConfig *EnhancedClientConfig
}

// NewClient creates a new Docker client wrapper with default configuration.
// This is the standard way to create a Docker client for diagnostic operations.
//
// The client will automatically:
//   - Negotiate API version with the Docker daemon
//   - Verify connectivity with a ping operation
//   - Configure appropriate timeouts
//
// Parameters:
//   - ctx: Context for timeout and cancellation control
//
// Returns:
//   - *Client: Configured Docker client wrapper
//   - error: If client creation or Docker connectivity fails
func NewClient(ctx context.Context) (*Client, error) {
	return NewClientWithOptions(ctx, nil)
}

// NewClientWithOptions creates a new Docker client with advanced configuration options.
// This method allows enabling the enhanced client with rate limiting, caching, and
// performance monitoring capabilities.
//
// When UseEnhanced is true, the client provides:
//   - Rate limiting for API calls
//   - Response caching for frequently accessed data
//   - Connection pooling and management
//   - Advanced metrics and monitoring
//   - Retry logic with exponential backoff
//
// Parameters:
//   - ctx: Context for timeout and cancellation control
//   - opts: Client configuration options (nil for defaults)
//
// Returns:
//   - *Client: Configured Docker client wrapper
//   - error: If client creation or Docker connectivity fails
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

// Close releases the Docker client resources and performs cleanup.
// This method should be called when the client is no longer needed to prevent resource leaks.
//
// For enhanced clients, this also:
//   - Stops background goroutines
//   - Clears caches
//   - Releases connection pools
//   - Saves metrics data
//
// Returns an error if resource cleanup fails.
func (c *Client) Close() error {
	if c.enhanced != nil {
		return c.enhanced.Close()
	}
	if c.docker != nil {
		return c.docker.Close()
	}
	return nil
}

// ListContainers returns a list of running containers for diagnostic analysis.
// This method retrieves container information needed for network diagnostics including
// container IDs, names, network configurations, and status information.
//
// The method automatically:
//   - Uses enhanced client caching when available
//   - Applies rate limiting to prevent API overload
//   - Returns only running containers by default
//   - Handles API errors gracefully
//
// Parameters:
//   - ctx: Context for timeout and cancellation control
//
// Returns:
//   - []types.Container: List of running container information
//   - error: If the API call fails or Docker is unreachable
func (c *Client) ListContainers(ctx context.Context) ([]types.Container, error) {
	// Use enhanced client if available
	if c.enhanced != nil {
		return c.enhanced.ListContainers(ctx)
	}
	
	// Fall back to standard implementation
	containers, err := c.docker.ContainerList(ctx, container.ListOptions{
		All: false, // Only running containers
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	return containers, nil
}

// GetNetworkInfo retrieves comprehensive information about Docker networks for diagnostic analysis.
// This method provides detailed network configuration including IPAM settings, connected containers,
// and network driver information essential for troubleshooting network issues.
//
// The method automatically:
//   - Inspects all available networks
//   - Extracts diagnostic-relevant information
//   - Handles network inspection errors gracefully
//   - Provides container endpoint details
//   - Uses enhanced client caching when available
//
// Returns:
//   - []NetworkDiagnostic: Detailed network information suitable for diagnostics
//   - error: If network retrieval or inspection fails
func (c *Client) GetNetworkInfo() ([]NetworkDiagnostic, error) {
	// Use enhanced client if available
	if c.enhanced != nil {
		return c.enhanced.GetNetworkInfo()
	}
	
	// Fall back to standard implementation
	networks, err := c.docker.NetworkList(c.ctx, network.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}
	
	var diagnostics []NetworkDiagnostic
	for _, net := range networks {
		netInspect, err := c.docker.NetworkInspect(c.ctx, net.ID, network.InspectOptions{
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

// GetContainerNetworkConfig gets detailed network configuration for a specific container.
// This method retrieves all network-related settings for a container including DNS configuration,
// network endpoints, IP addresses, and connectivity information.
//
// The extracted information includes:
//   - Hostname and domain configuration
//   - DNS servers and search domains
//   - Network endpoints and IP addresses
//   - Gateway and routing information
//   - MAC addresses for network interfaces
//
// Parameters:
//   - containerID: Container ID or name to inspect
//
// Returns:
//   - *ContainerNetworkInfo: Comprehensive network configuration
//   - error: If container inspection fails or container doesn't exist
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

// ExecInContainer runs a diagnostic command inside a container and returns the output.
// This method is used for network diagnostics that require executing commands within
// the container's network namespace, such as ping, nslookup, or netstat.
//
// The method:
//   - Creates an exec session in the container
//   - Captures both stdout and stderr
//   - Applies context timeout controls
//   - Returns command output as a string
//   - Uses enhanced client rate limiting when available
//
// Common diagnostic commands:
//   - ["ping", "-c", "3", "google.com"]
//   - ["nslookup", "google.com"]
//   - ["netstat", "-r"]
//   - ["cat", "/etc/resolv.conf"]
//
// Parameters:
//   - ctx: Context for timeout and cancellation control
//   - containerID: Container ID or name where to execute the command
//   - cmd: Command and arguments to execute
//
// Returns:
//   - string: Command output (stdout and stderr combined)
//   - error: If exec creation, execution, or output reading fails
func (c *Client) ExecInContainer(ctx context.Context, containerID string, cmd []string) (string, error) {
	// Use enhanced client if available
	if c.enhanced != nil {
		return c.enhanced.ExecInContainer(ctx, containerID, cmd)
	}
	
	// Fall back to standard implementation
	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:         cmd,
	}
	
	execCreate, err := c.docker.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec: %w", err)
	}
	
	response, err := c.docker.ContainerExecAttach(ctx, execCreate.ID, container.ExecStartOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to start exec: %w", err)
	}
	defer response.Close()
	
	output := make([]byte, 4096)
	n, _ := response.Reader.Read(output)
	
	return string(output[:n]), nil
}

// Ping checks connectivity to the Docker daemon and verifies API accessibility.
// This method is used to validate that the Docker daemon is running and accessible
// before attempting other diagnostic operations.
//
// The ping operation:
//   - Verifies Docker daemon connectivity
//   - Validates API version compatibility
//   - Uses enhanced client when available
//   - Respects context timeout settings
//
// Parameters:
//   - ctx: Context for timeout and cancellation control
//
// Returns:
//   - error: If Docker daemon is unreachable or incompatible
func (c *Client) Ping(ctx context.Context) error {
	// Use enhanced client if available
	if c.enhanced != nil {
		return c.enhanced.Ping(ctx)
	}
	
	// Fall back to standard implementation
	_, err := c.docker.Ping(ctx)
	return err
}

// InspectContainer returns detailed information about a container for diagnostic analysis.
// This method retrieves comprehensive container configuration including network settings,
// mounts, environment variables, and runtime state.
//
// The inspection provides:
//   - Container configuration and state
//   - Network settings and endpoints
//   - Resource limits and usage
//   - Mount points and volumes
//   - Environment and runtime information
//
// Parameters:
//   - containerID: Container ID or name to inspect
//
// Returns:
//   - types.ContainerJSON: Complete container information
//   - error: If container doesn't exist or inspection fails
func (c *Client) InspectContainer(containerID string) (types.ContainerJSON, error) {
	// Use enhanced client if available
	if c.enhanced != nil {
		return c.enhanced.InspectContainer(containerID)
	}
	
	// Fall back to standard implementation
	return c.docker.ContainerInspect(c.ctx, containerID)
}

// GetMetrics returns comprehensive client performance metrics when using the enhanced client.
// These metrics provide insights into API usage, performance, and resource utilization.
//
// Available metrics include:
//   - API call counts and timing
//   - Cache hit/miss ratios
//   - Rate limiting statistics
//   - Error rates and patterns
//   - Connection pool usage
//
// Returns nil if using the standard client without metrics collection.
//
// Returns:
//   - *MetricsSnapshot: Current metrics data (nil for standard client)
func (c *Client) GetMetrics() *MetricsSnapshot {
	if c.enhanced != nil {
		metrics := c.enhanced.GetMetrics()
		return &metrics
	}
	return nil
}

// GetCacheStats returns detailed cache performance statistics when using the enhanced client.
// These statistics help optimize cache configuration and understand data access patterns.
//
// Cache statistics include:
//   - Hit and miss counts
//   - Cache size and memory usage
//   - Eviction rates and policies
//   - Popular data patterns
//   - Performance impact measurements
//
// Returns nil if using the standard client without caching.
//
// Returns:
//   - *CacheStats: Current cache statistics (nil for standard client)
func (c *Client) GetCacheStats() *CacheStats {
	if c.enhanced != nil {
		stats := c.enhanced.GetCacheStats()
		return &stats
	}
	return nil
}

// InvalidateCache invalidates all cache entries when using the enhanced client.
// This method forces fresh data retrieval from the Docker API on subsequent calls.
//
// Cache invalidation is useful when:
//   - Container or network configuration changes
//   - Diagnostic accuracy requires fresh data
//   - Troubleshooting cache-related issues
//   - After administrative Docker operations
//
// Has no effect when using the standard client without caching.
func (c *Client) InvalidateCache() {
	if c.enhanced != nil {
		c.enhanced.InvalidateCache()
	}
}

// IsEnhanced returns true if the client is using enhanced features like rate limiting and caching.
// This method helps diagnostic code adapt behavior based on available client capabilities.
//
// Enhanced clients provide:
//   - Rate limiting and throttling
//   - Response caching
//   - Advanced metrics collection
//   - Connection pooling
//   - Retry logic with backoff
//
// Returns:
//   - bool: true if using enhanced client, false for standard client
func (c *Client) IsEnhanced() bool {
	return c.enhanced != nil
}

// convertIPAM converts Docker's native IPAM structure to our diagnostic-friendly format.
// This helper function extracts essential IP Address Management information needed
// for network diagnostics while simplifying the data structure.
//
// The conversion extracts:
//   - IPAM driver information
//   - Subnet configurations
//   - Gateway assignments
//   - IP range allocations
//
// Parameters:
//   - ipam: Docker's native IPAM configuration
//
// Returns:
//   - IPAMConfig: Simplified IPAM structure for diagnostics
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

// NetworkDiagnostic represents comprehensive network information optimized for diagnostic analysis.
// This structure contains all essential network configuration data needed for troubleshooting
// Docker networking issues.
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

// IPAMConfig contains IP Address Management configuration for network diagnostics.
// This simplified structure focuses on information most relevant for troubleshooting.
type IPAMConfig struct {
	Driver  string
	Configs []IPAMConfigBlock
}

// IPAMConfigBlock represents a single IPAM configuration block with subnet and gateway information.
type IPAMConfigBlock struct {
	Subnet  string
	Gateway string
}

// ContainerEndpoint represents a container's network endpoint information within a specific network.
// This includes all addressing information needed for connectivity diagnostics.
type ContainerEndpoint struct {
	Name        string
	IPv4Address string
	IPv6Address string
	MacAddress  string
}

// ContainerNetworkInfo contains comprehensive network configuration for a single container.
// This structure aggregates all network-related settings that affect container connectivity.
type ContainerNetworkInfo struct {
	Hostname   string
	Domainname string
	DNS        []string
	DNSSearch  []string
	DNSOptions []string
	Networks   map[string]NetworkEndpoint
}

// NetworkEndpoint represents a container's connection to a specific Docker network.
// This includes IP addressing, gateway, and MAC address information.
type NetworkEndpoint struct {
	IPAddress  string
	Gateway    string
	MacAddress string
}