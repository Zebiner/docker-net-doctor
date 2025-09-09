// Package docker provides response caching for Docker API operations
package docker

import (
	"context"
	"sync"
	"time"
	
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/network"
)

// CacheEntry represents a cached response
type CacheEntry struct {
	Data      interface{}
	Timestamp time.Time
	TTL       time.Duration
}

// ResponseCache provides caching for Docker API responses
type ResponseCache struct {
	mu              sync.RWMutex
	entries         map[string]*CacheEntry
	defaultTTL      time.Duration
	maxEntries      int
	hits            int64
	misses          int64
	evictions       int64
	cleanupInterval time.Duration
	stopCleanup     chan struct{}
}

// NewResponseCache creates a new response cache
func NewResponseCache(defaultTTL time.Duration, maxEntries int) *ResponseCache {
	cache := &ResponseCache{
		entries:         make(map[string]*CacheEntry),
		defaultTTL:      defaultTTL,
		maxEntries:      maxEntries,
		cleanupInterval: 30 * time.Second,
		stopCleanup:     make(chan struct{}),
	}
	
	// Start cleanup goroutine
	go cache.cleanupRoutine()
	
	return cache
}

// GetContainerList retrieves cached container list or returns nil
func (c *ResponseCache) GetContainerList(key string) ([]types.Container, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, exists := c.entries[key]
	if !exists {
		c.misses++
		return nil, false
	}
	
	// Check if entry is expired
	if time.Since(entry.Timestamp) > entry.TTL {
		c.misses++
		return nil, false
	}
	
	containers, ok := entry.Data.([]types.Container)
	if !ok {
		c.misses++
		return nil, false
	}
	
	c.hits++
	return containers, true
}

// SetContainerList caches a container list
func (c *ResponseCache) SetContainerList(key string, containers []types.Container, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Check cache size limit
	if len(c.entries) >= c.maxEntries {
		c.evictOldest()
	}
	
	if ttl == 0 {
		ttl = c.defaultTTL
	}
	
	c.entries[key] = &CacheEntry{
		Data:      containers,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
}

// GetNetworkList retrieves cached network list or returns nil
func (c *ResponseCache) GetNetworkList(key string) ([]network.Summary, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, exists := c.entries[key]
	if !exists {
		c.misses++
		return nil, false
	}
	
	// Check if entry is expired
	if time.Since(entry.Timestamp) > entry.TTL {
		c.misses++
		return nil, false
	}
	
	networks, ok := entry.Data.([]network.Summary)
	if !ok {
		c.misses++
		return nil, false
	}
	
	c.hits++
	return networks, true
}

// SetNetworkList caches a network list
func (c *ResponseCache) SetNetworkList(key string, networks []network.Summary, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Check cache size limit
	if len(c.entries) >= c.maxEntries {
		c.evictOldest()
	}
	
	if ttl == 0 {
		ttl = c.defaultTTL
	}
	
	c.entries[key] = &CacheEntry{
		Data:      networks,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
}

// GetNetworkInspect retrieves cached network inspection or returns nil
func (c *ResponseCache) GetNetworkInspect(networkID string) (network.Inspect, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	key := "network_inspect:" + networkID
	entry, exists := c.entries[key]
	if !exists {
		c.misses++
		return network.Inspect{}, false
	}
	
	// Check if entry is expired
	if time.Since(entry.Timestamp) > entry.TTL {
		c.misses++
		return network.Inspect{}, false
	}
	
	net, ok := entry.Data.(network.Inspect)
	if !ok {
		c.misses++
		return network.Inspect{}, false
	}
	
	c.hits++
	return net, true
}

// SetNetworkInspect caches a network inspection result
func (c *ResponseCache) SetNetworkInspect(networkID string, net network.Inspect, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	key := "network_inspect:" + networkID
	
	// Check cache size limit
	if len(c.entries) >= c.maxEntries {
		c.evictOldest()
	}
	
	if ttl == 0 {
		ttl = c.defaultTTL
	}
	
	c.entries[key] = &CacheEntry{
		Data:      net,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
}

// GetContainerInspect retrieves cached container inspection or returns nil
func (c *ResponseCache) GetContainerInspect(containerID string) (types.ContainerJSON, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	key := "container_inspect:" + containerID
	entry, exists := c.entries[key]
	if !exists {
		c.misses++
		return types.ContainerJSON{}, false
	}
	
	// Check if entry is expired
	if time.Since(entry.Timestamp) > entry.TTL {
		c.misses++
		return types.ContainerJSON{}, false
	}
	
	container, ok := entry.Data.(types.ContainerJSON)
	if !ok {
		c.misses++
		return types.ContainerJSON{}, false
	}
	
	c.hits++
	return container, true
}

// SetContainerInspect caches a container inspection result
func (c *ResponseCache) SetContainerInspect(containerID string, container types.ContainerJSON, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	key := "container_inspect:" + containerID
	
	// Check cache size limit
	if len(c.entries) >= c.maxEntries {
		c.evictOldest()
	}
	
	if ttl == 0 {
		ttl = c.defaultTTL
	}
	
	c.entries[key] = &CacheEntry{
		Data:      container,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
}

// Invalidate removes a specific cache entry
func (c *ResponseCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	delete(c.entries, key)
}

// InvalidateAll clears all cache entries
func (c *ResponseCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.entries = make(map[string]*CacheEntry)
}

// GetStats returns cache statistics
func (c *ResponseCache) GetStats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return CacheStats{
		Hits:      c.hits,
		Misses:    c.misses,
		Evictions: c.evictions,
		Entries:   len(c.entries),
		HitRate:   c.calculateHitRate(),
	}
}

// Close stops the cleanup routine
func (c *ResponseCache) Close() {
	close(c.stopCleanup)
}

// evictOldest removes the oldest cache entry
func (c *ResponseCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	
	for key, entry := range c.entries {
		if oldestKey == "" || entry.Timestamp.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.Timestamp
		}
	}
	
	if oldestKey != "" {
		delete(c.entries, oldestKey)
		c.evictions++
	}
}

// cleanupRoutine periodically removes expired entries
func (c *ResponseCache) cleanupRoutine() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			c.cleanupExpired()
		case <-c.stopCleanup:
			return
		}
	}
}

// cleanupExpired removes all expired cache entries
func (c *ResponseCache) cleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	now := time.Now()
	keysToDelete := []string{}
	
	for key, entry := range c.entries {
		if now.Sub(entry.Timestamp) > entry.TTL {
			keysToDelete = append(keysToDelete, key)
		}
	}
	
	for _, key := range keysToDelete {
		delete(c.entries, key)
	}
}

// calculateHitRate calculates the cache hit rate
func (c *ResponseCache) calculateHitRate() float64 {
	total := c.hits + c.misses
	if total == 0 {
		return 0
	}
	return float64(c.hits) / float64(total)
}

// CacheStats represents cache statistics
type CacheStats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	Entries   int
	HitRate   float64
}

// CachableNetworkInfo represents a cacheable network information structure
type CachableNetworkInfo struct {
	Networks []NetworkDiagnostic
	CacheKey string
	TTL      time.Duration
}

// GetOrCompute retrieves a value from cache or computes it if not present
func (c *ResponseCache) GetOrCompute(ctx context.Context, key string, compute func() (interface{}, error), ttl time.Duration) (interface{}, error) {
	// Try to get from cache first
	c.mu.RLock()
	entry, exists := c.entries[key]
	if exists && time.Since(entry.Timestamp) <= entry.TTL {
		c.hits++
		c.mu.RUnlock()
		return entry.Data, nil
	}
	c.mu.RUnlock()
	
	// Compute the value
	c.misses++
	value, err := compute()
	if err != nil {
		return nil, err
	}
	
	// Store in cache
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if len(c.entries) >= c.maxEntries {
		c.evictOldest()
	}
	
	if ttl == 0 {
		ttl = c.defaultTTL
	}
	
	c.entries[key] = &CacheEntry{
		Data:      value,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
	
	return value, nil
}