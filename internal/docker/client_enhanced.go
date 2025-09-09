// Package docker provides an enhanced Docker client with rate limiting and resource controls
package docker

import (
	"context"
	"fmt"
	"sync"
	"time"
	
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"golang.org/x/time/rate"
)

// EnhancedClient provides a Docker client with rate limiting, caching, and metrics
type EnhancedClient struct {
	// Core components
	docker      *client.Client
	rateLimiter *rate.Limiter
	cache       *ResponseCache
	metrics     *ClientMetrics
	retryPolicy *RetryPolicy
	
	// Configuration
	timeout        time.Duration
	maxConnections int32
	
	// Connection management
	activeConns    int32
	connSemaphore  chan struct{}
	
	// Context management
	ctx    context.Context
	cancel context.CancelFunc
	
	// Shutdown management
	mu       sync.RWMutex
	closed   bool
	closeCh  chan struct{}
}

// EnhancedClientConfig contains configuration for the enhanced client
type EnhancedClientConfig struct {
	// Rate limiting
	RateLimit      float64       // Requests per second
	RateBurst      int          // Burst size
	
	// Caching
	CacheTTL       time.Duration // Default cache TTL
	MaxCacheEntries int          // Maximum cache entries
	
	// Connection management
	MaxConnections int          // Maximum concurrent connections
	Timeout        time.Duration // Operation timeout
	
	// Retry policy
	MaxRetries     int          // Maximum retry attempts
	RetryBackoff   time.Duration // Initial retry backoff
}

// NewEnhancedClient creates a new enhanced Docker client
func NewEnhancedClient(ctx context.Context, config *EnhancedClientConfig) (*EnhancedClient, error) {
	// Apply defaults if not specified
	if config == nil {
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
	
	// Create Docker client
	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	
	// Verify connection
	_, err = dockerClient.Ping(ctx)
	if err != nil {
		dockerClient.Close()
		return nil, fmt.Errorf("cannot connect to Docker daemon: %w", err)
	}
	
	// Create context with cancellation
	clientCtx, cancel := context.WithCancel(ctx)
	
	// Create retry policy
	retryPolicy := &RetryPolicy{
		MaxRetries:     config.MaxRetries,
		InitialBackoff: config.RetryBackoff,
		MaxBackoff:     5 * time.Second,
		BackoffFactor:  2.0,
		RetryableErrors: map[string]bool{
			"connection refused":     true,
			"connection reset":       true,
			"timeout":               true,
			"EOF":                   true,
			"broken pipe":           true,
			"service unavailable":   true,
			"too many requests":     true,
		},
	}
	
	ec := &EnhancedClient{
		docker:         dockerClient,
		rateLimiter:    rate.NewLimiter(rate.Limit(config.RateLimit), config.RateBurst),
		cache:          NewResponseCache(config.CacheTTL, config.MaxCacheEntries),
		metrics:        NewClientMetrics(),
		retryPolicy:    retryPolicy,
		timeout:        config.Timeout,
		maxConnections: int32(config.MaxConnections),
		connSemaphore:  make(chan struct{}, config.MaxConnections),
		ctx:            clientCtx,
		cancel:         cancel,
		closeCh:        make(chan struct{}),
	}
	
	// Initialize connection semaphore
	for i := 0; i < config.MaxConnections; i++ {
		ec.connSemaphore <- struct{}{}
	}
	
	return ec, nil
}

// Close releases all resources
func (ec *EnhancedClient) Close() error {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	
	if ec.closed {
		return nil
	}
	
	ec.closed = true
	close(ec.closeCh)
	
	// Cancel context
	if ec.cancel != nil {
		ec.cancel()
	}
	
	// Close cache
	if ec.cache != nil {
		ec.cache.Close()
	}
	
	// Close Docker client
	if ec.docker != nil {
		return ec.docker.Close()
	}
	
	return nil
}

// ListContainers returns a list of containers with caching and rate limiting
func (ec *EnhancedClient) ListContainers(ctx context.Context) ([]types.Container, error) {
	const operation = "container_list"
	cacheKey := "containers:all"
	
	// Check cache first
	if containers, found := ec.cache.GetContainerList(cacheKey); found {
		ec.metrics.RecordCacheHit()
		return containers, nil
	}
	ec.metrics.RecordCacheMiss()
	
	// Execute with rate limiting and retry
	var containers []types.Container
	err := ec.executeWithRateLimit(ctx, operation, func(execCtx context.Context) error {
		result, err := ec.retryPolicy.ExecuteWithValue(execCtx, func() (interface{}, error) {
			return ec.docker.ContainerList(execCtx, container.ListOptions{
				All: false, // Only running containers
			})
		})
		
		if err != nil {
			return err
		}
		
		containers = result.([]types.Container)
		return nil
	})
	
	if err != nil {
		return nil, err
	}
	
	// Cache the result
	ec.cache.SetContainerList(cacheKey, containers, 30*time.Second)
	
	return containers, nil
}

// GetNetworkInfo retrieves network information with caching
func (ec *EnhancedClient) GetNetworkInfo() ([]NetworkDiagnostic, error) {
	const operation = "network_list"
	ctx := ec.ctx
	
	// Try to get from cache using GetOrCompute
	result, err := ec.cache.GetOrCompute(ctx, "network:diagnostics", func() (interface{}, error) {
		// Execute with rate limiting
		var networks []network.Summary
		err := ec.executeWithRateLimit(ctx, operation, func(execCtx context.Context) error {
			result, err := ec.retryPolicy.ExecuteWithValue(execCtx, func() (interface{}, error) {
				return ec.docker.NetworkList(execCtx, network.ListOptions{})
			})
			
			if err != nil {
				return err
			}
			
			networks = result.([]network.Summary)
			return nil
		})
		
		if err != nil {
			return nil, err
		}
		
		// Convert to diagnostics
		var diagnostics []NetworkDiagnostic
		for _, net := range networks {
			// Inspect each network with rate limiting
			var netInspect network.Inspect
			err := ec.executeWithRateLimit(ctx, "network_inspect", func(execCtx context.Context) error {
				result, err := ec.retryPolicy.ExecuteWithValue(execCtx, func() (interface{}, error) {
					return ec.docker.NetworkInspect(execCtx, net.ID, network.InspectOptions{
						Verbose: true,
					})
				})
				
				if err != nil {
					return err
				}
				
				netInspect = result.(network.Inspect)
				return nil
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
	}, 60*time.Second)
	
	if err != nil {
		return nil, err
	}
	
	return result.([]NetworkDiagnostic), nil
}

// GetContainerNetworkConfig gets network configuration with caching
func (ec *EnhancedClient) GetContainerNetworkConfig(containerID string) (*ContainerNetworkInfo, error) {
	const operation = "container_inspect"
	ctx := ec.ctx
	cacheKey := "container_network:" + containerID
	
	// Try to get from cache
	result, err := ec.cache.GetOrCompute(ctx, cacheKey, func() (interface{}, error) {
		var container types.ContainerJSON
		err := ec.executeWithRateLimit(ctx, operation, func(execCtx context.Context) error {
			result, err := ec.retryPolicy.ExecuteWithValue(execCtx, func() (interface{}, error) {
				return ec.docker.ContainerInspect(execCtx, containerID)
			})
			
			if err != nil {
				return err
			}
			
			container = result.(types.ContainerJSON)
			return nil
		})
		
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
	}, 30*time.Second)
	
	if err != nil {
		return nil, err
	}
	
	return result.(*ContainerNetworkInfo), nil
}

// ExecInContainer executes a command in a container with rate limiting
func (ec *EnhancedClient) ExecInContainer(ctx context.Context, containerID string, cmd []string) (string, error) {
	const operation = "container_exec"
	
	var output string
	err := ec.executeWithRateLimit(ctx, operation, func(execCtx context.Context) error {
		result, err := ec.retryPolicy.ExecuteWithValue(execCtx, func() (interface{}, error) {
			execConfig := container.ExecOptions{
				AttachStdout: true,
				AttachStderr: true,
				Cmd:         cmd,
			}
			
			execCreate, err := ec.docker.ContainerExecCreate(execCtx, containerID, execConfig)
			if err != nil {
				return "", fmt.Errorf("failed to create exec: %w", err)
			}
			
			response, err := ec.docker.ContainerExecAttach(execCtx, execCreate.ID, container.ExecStartOptions{})
			if err != nil {
				return "", fmt.Errorf("failed to start exec: %w", err)
			}
			defer response.Close()
			
			outputBuf := make([]byte, 4096)
			n, _ := response.Reader.Read(outputBuf)
			
			return string(outputBuf[:n]), nil
		})
		
		if err != nil {
			return err
		}
		
		output = result.(string)
		return nil
	})
	
	return output, err
}

// Ping checks connectivity to the Docker daemon
func (ec *EnhancedClient) Ping(ctx context.Context) error {
	const operation = "ping"
	
	return ec.executeWithRateLimit(ctx, operation, func(execCtx context.Context) error {
		return ec.retryPolicy.Execute(execCtx, func() error {
			_, err := ec.docker.Ping(execCtx)
			return err
		})
	})
}

// InspectContainer returns detailed container information with caching
func (ec *EnhancedClient) InspectContainer(containerID string) (types.ContainerJSON, error) {
	const operation = "container_inspect"
	ctx := ec.ctx
	
	// Check cache first
	if container, found := ec.cache.GetContainerInspect(containerID); found {
		ec.metrics.RecordCacheHit()
		return container, nil
	}
	ec.metrics.RecordCacheMiss()
	
	var container types.ContainerJSON
	err := ec.executeWithRateLimit(ctx, operation, func(execCtx context.Context) error {
		result, err := ec.retryPolicy.ExecuteWithValue(execCtx, func() (interface{}, error) {
			return ec.docker.ContainerInspect(execCtx, containerID)
		})
		
		if err != nil {
			return err
		}
		
		container = result.(types.ContainerJSON)
		return nil
	})
	
	if err != nil {
		return types.ContainerJSON{}, err
	}
	
	// Cache the result
	ec.cache.SetContainerInspect(containerID, container, 30*time.Second)
	
	return container, nil
}

// GetMetrics returns client metrics
func (ec *EnhancedClient) GetMetrics() MetricsSnapshot {
	return ec.metrics.GetSnapshot()
}

// GetCacheStats returns cache statistics
func (ec *EnhancedClient) GetCacheStats() CacheStats {
	return ec.cache.GetStats()
}

// InvalidateCache invalidates all cache entries
func (ec *EnhancedClient) InvalidateCache() {
	ec.cache.InvalidateAll()
}

// executeWithRateLimit executes an operation with rate limiting and connection management
// The function parameter now receives a context that includes timeout
func (ec *EnhancedClient) executeWithRateLimit(ctx context.Context, operation string, fn func(context.Context) error) error {
	// Check if client is closed
	ec.mu.RLock()
	if ec.closed {
		ec.mu.RUnlock()
		return fmt.Errorf("client is closed")
	}
	ec.mu.RUnlock()
	
	// Acquire connection slot
	select {
	case <-ec.connSemaphore:
		defer func() {
			ec.connSemaphore <- struct{}{}
		}()
	case <-ctx.Done():
		return ctx.Err()
	case <-ec.closeCh:
		return fmt.Errorf("client is closing")
	}
	
	// Update connection metrics
	ec.metrics.UpdateActiveConnections(1)
	defer ec.metrics.UpdateActiveConnections(-1)
	
	// Apply rate limiting
	if err := ec.rateLimiter.Wait(ctx); err != nil {
		ec.metrics.RecordRateLimitHit()
		return fmt.Errorf("rate limit error: %w", err)
	}
	
	// Create timeout context and pass it to the function
	timeoutCtx, cancel := context.WithTimeout(ctx, ec.timeout)
	defer cancel()
	
	// Execute operation with metrics
	startTime := time.Now()
	err := fn(timeoutCtx)  // Pass the timeout context to the function
	duration := time.Since(startTime)
	
	// Record metrics
	ec.metrics.RecordAPICall(operation, duration, err)
	
	return err
}

// AsLegacyClient returns a legacy Client interface for compatibility
func (ec *EnhancedClient) AsLegacyClient() *Client {
	return &Client{
		docker: ec.docker,
		ctx:    ec.ctx,
	}
}