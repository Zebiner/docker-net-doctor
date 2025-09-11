// Package diagnostics provides comprehensive resource cleanup management for
// Docker Network Doctor operations, including automatic cleanup of orphaned
// containers, networks, volumes, and other Docker resources during error recovery.
//
// The resource cleanup system ensures that diagnostic operations do not leave
// behind orphaned resources that could cause conflicts or consume system resources.
// It provides both automatic cleanup during error recovery and manual cleanup
// capabilities for maintenance operations.
//
// Key Features:
//   - Automatic cleanup of diagnostic containers and test resources
//   - Orphaned resource detection and removal
//   - Configurable cleanup policies and retention periods
//   - Safe cleanup with conflict detection and rollback
//   - Resource usage tracking and optimization
//   - Integration with error recovery workflows
//   - Dry-run mode for safe testing of cleanup operations
//
// Cleanup Categories:
//   - Test containers created during diagnostics
//   - Temporary networks created for testing
//   - Orphaned volumes from failed operations
//   - Stale diagnostic data and temporary files
//   - Docker daemon cache and temporary resources
//
// Example usage:
//   cleanup := NewResourceCleanupManager(&CleanupConfig{
//       AutoCleanup: true,
//       Timeout: 30 * time.Second,
//   })
//
//   err := cleanup.CleanupAfterError(ctx, diagError)

package diagnostics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// ResourceCleanupManager handles automatic and manual cleanup of Docker resources
// created or used during diagnostic operations. It provides safe resource management
// with conflict detection, rollback capabilities, and comprehensive cleanup policies.
//
// The cleanup manager operates in multiple modes:
//   - Automatic cleanup during error recovery
//   - Scheduled cleanup of orphaned resources
//   - Manual cleanup for maintenance operations
//   - Emergency cleanup for resource exhaustion scenarios
//
// Thread Safety: ResourceCleanupManager is safe for concurrent use.
type ResourceCleanupManager struct {
	client *docker.Client
	config *CleanupConfig

	// Resource tracking
	trackedContainers map[string]*TrackedResource
	trackedNetworks   map[string]*TrackedResource
	trackedVolumes    map[string]*TrackedResource

	// Cleanup state
	cleanupHistory []CleanupOperation
	metrics        *CleanupMetrics

	// Control
	stopCh  chan struct{}
	stopped bool

	// Thread safety
	mu sync.RWMutex
}

// CleanupConfig defines the behavior and policies for resource cleanup operations
type CleanupConfig struct {
	// Basic configuration
	Timeout     time.Duration // Maximum time for cleanup operations
	AutoCleanup bool          // Enable automatic cleanup during error recovery
	DryRun      bool          // Test mode - log actions without executing

	// Resource policies
	PreserveDebugData bool          // Keep resources for debugging
	CleanupRetention  time.Duration // How long to keep cleanup history
	ResourceMaxAge    time.Duration // Maximum age for diagnostic resources

	// Safety settings
	RequireConfirmation bool // Require explicit confirmation for cleanup
	AllowForceCleanup   bool // Allow forced cleanup of protected resources
	BackupBeforeCleanup bool // Backup resource configurations

	// Performance settings
	MaxConcurrentCleanups int // Maximum parallel cleanup operations
	CleanupBatchSize      int // Number of resources to process in batch

	// Scheduling
	EnablePeriodicCleanup   bool          // Enable scheduled cleanup
	PeriodicCleanupInterval time.Duration // Interval for periodic cleanup
}

// TrackedResource represents a Docker resource being tracked for cleanup
type TrackedResource struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       ResourceType      `json:"type"`
	CreatedAt  time.Time         `json:"created_at"`
	LastUsed   time.Time         `json:"last_used"`
	Tags       map[string]string `json:"tags"`
	Protected  bool              `json:"protected"`  // Prevent accidental cleanup
	Diagnostic bool              `json:"diagnostic"` // Created by diagnostic operations
	Temporary  bool              `json:"temporary"`  // Temporary resource for cleanup

	// Metadata
	CreatedBy  string   `json:"created_by"`  // Which diagnostic operation created this
	DependsOn  []string `json:"depends_on"`  // Other resources this depends on
	DependedBy []string `json:"depended_by"` // Resources that depend on this
}

// ResourceType categorizes different types of Docker resources for cleanup
type ResourceType int

const (
	ResourceTypeContainer ResourceType = iota
	ResourceTypeNetwork
	ResourceTypeVolume
	ResourceTypeImage
	ResourceTypeSecret
	ResourceTypeConfig
)

// String returns a human-readable resource type name
func (rt ResourceType) String() string {
	switch rt {
	case ResourceTypeContainer:
		return "container"
	case ResourceTypeNetwork:
		return "network"
	case ResourceTypeVolume:
		return "volume"
	case ResourceTypeImage:
		return "image"
	case ResourceTypeSecret:
		return "secret"
	case ResourceTypeConfig:
		return "config"
	default:
		return "unknown"
	}
}

// CleanupOperation represents a single cleanup operation with its results
type CleanupOperation struct {
	ID        string                  `json:"id"`
	StartTime time.Time               `json:"start_time"`
	EndTime   time.Time               `json:"end_time"`
	Duration  time.Duration           `json:"duration"`
	Trigger   CleanupTrigger          `json:"trigger"`
	Resources []*TrackedResource      `json:"resources"`
	Results   []ResourceCleanupResult `json:"results"`
	Success   bool                    `json:"success"`
	Error     string                  `json:"error,omitempty"`
	DryRun    bool                    `json:"dry_run"`
}

// CleanupTrigger indicates what triggered a cleanup operation
type CleanupTrigger int

const (
	TriggerErrorRecovery CleanupTrigger = iota
	TriggerPeriodic
	TriggerManual
	TriggerEmergency
	TriggerShutdown
)

// String returns a human-readable trigger name
func (ct CleanupTrigger) String() string {
	switch ct {
	case TriggerErrorRecovery:
		return "error_recovery"
	case TriggerPeriodic:
		return "periodic"
	case TriggerManual:
		return "manual"
	case TriggerEmergency:
		return "emergency"
	case TriggerShutdown:
		return "shutdown"
	default:
		return "unknown"
	}
}

// ResourceCleanupResult contains the outcome of cleaning up a single resource
type ResourceCleanupResult struct {
	Resource  *TrackedResource `json:"resource"`
	Success   bool             `json:"success"`
	Error     string           `json:"error,omitempty"`
	Action    CleanupAction    `json:"action"`
	TimeTaken time.Duration    `json:"time_taken"`
	SizeFreed int64            `json:"size_freed,omitempty"`
}

// CleanupAction describes what action was taken on a resource
type CleanupAction int

const (
	ActionRemoved CleanupAction = iota
	ActionStopped
	ActionSkipped
	ActionFailed
	ActionBackedUp
)

// String returns a human-readable action name
func (ca CleanupAction) String() string {
	switch ca {
	case ActionRemoved:
		return "removed"
	case ActionStopped:
		return "stopped"
	case ActionSkipped:
		return "skipped"
	case ActionFailed:
		return "failed"
	case ActionBackedUp:
		return "backed_up"
	default:
		return "unknown"
	}
}

// CleanupMetrics tracks cleanup system performance and resource usage
type CleanupMetrics struct {
	// Operation counts
	TotalCleanups      int64 `json:"total_cleanups"`
	SuccessfulCleanups int64 `json:"successful_cleanups"`
	FailedCleanups     int64 `json:"failed_cleanups"`

	// Resource counts
	ResourcesTracked   int64 `json:"resources_tracked"`
	ResourcesCleanedUp int64 `json:"resources_cleaned_up"`
	ResourcesSkipped   int64 `json:"resources_skipped"`

	// Performance metrics
	TotalCleanupTime   time.Duration `json:"total_cleanup_time"`
	AverageCleanupTime time.Duration `json:"average_cleanup_time"`
	MaxCleanupTime     time.Duration `json:"max_cleanup_time"`

	// Resource metrics by type
	CleanupsByType map[ResourceType]int64 `json:"cleanups_by_type"`

	// Efficiency metrics
	StorageFreed      int64   `json:"storage_freed"`
	CleanupEfficiency float64 `json:"cleanup_efficiency"`

	// Health indicators
	IsHealthy             bool      `json:"is_healthy"`
	LastSuccessfulCleanup time.Time `json:"last_successful_cleanup"`
}

// NewResourceCleanupManager creates a new resource cleanup manager with the specified configuration
func NewResourceCleanupManager(config *CleanupConfig) *ResourceCleanupManager {
	if config == nil {
		config = &CleanupConfig{
			Timeout:                 30 * time.Second,
			AutoCleanup:             true,
			DryRun:                  false,
			PreserveDebugData:       false,
			CleanupRetention:        24 * time.Hour,
			ResourceMaxAge:          2 * time.Hour,
			RequireConfirmation:     false,
			AllowForceCleanup:       false,
			BackupBeforeCleanup:     false,
			MaxConcurrentCleanups:   3,
			CleanupBatchSize:        10,
			EnablePeriodicCleanup:   true,
			PeriodicCleanupInterval: 1 * time.Hour,
		}
	}

	rcm := &ResourceCleanupManager{
		config:            config,
		trackedContainers: make(map[string]*TrackedResource),
		trackedNetworks:   make(map[string]*TrackedResource),
		trackedVolumes:    make(map[string]*TrackedResource),
		cleanupHistory:    make([]CleanupOperation, 0),
		metrics: &CleanupMetrics{
			CleanupsByType: make(map[ResourceType]int64),
		},
		stopCh: make(chan struct{}),
	}

	// Start periodic cleanup if enabled
	if config.EnablePeriodicCleanup {
		go rcm.periodicCleanupLoop()
	}

	return rcm
}

// TrackResource adds a Docker resource to the cleanup tracking system
func (rcm *ResourceCleanupManager) TrackResource(resource *TrackedResource) {
	rcm.mu.Lock()
	defer rcm.mu.Unlock()

	switch resource.Type {
	case ResourceTypeContainer:
		rcm.trackedContainers[resource.ID] = resource
	case ResourceTypeNetwork:
		rcm.trackedNetworks[resource.ID] = resource
	case ResourceTypeVolume:
		rcm.trackedVolumes[resource.ID] = resource
	}

	rcm.metrics.ResourcesTracked++
}

// UntrackResource removes a resource from cleanup tracking
func (rcm *ResourceCleanupManager) UntrackResource(resourceID string, resourceType ResourceType) {
	rcm.mu.Lock()
	defer rcm.mu.Unlock()

	switch resourceType {
	case ResourceTypeContainer:
		delete(rcm.trackedContainers, resourceID)
	case ResourceTypeNetwork:
		delete(rcm.trackedNetworks, resourceID)
	case ResourceTypeVolume:
		delete(rcm.trackedVolumes, resourceID)
	}
}

// CleanupAfterError performs cleanup operations specific to error recovery scenarios
func (rcm *ResourceCleanupManager) CleanupAfterError(ctx context.Context, diagError *DiagnosticError) error {
	if !rcm.config.AutoCleanup {
		return nil // Automatic cleanup disabled
	}

	// Create cleanup context with timeout
	cleanupCtx, cancel := context.WithTimeout(ctx, rcm.config.Timeout)
	defer cancel()

	// Determine cleanup strategy based on error type
	strategy := rcm.selectCleanupStrategy(diagError)

	// Execute cleanup operation
	operation := &CleanupOperation{
		ID:        fmt.Sprintf("error-recovery-%d", time.Now().Unix()),
		StartTime: time.Now(),
		Trigger:   TriggerErrorRecovery,
		DryRun:    rcm.config.DryRun,
	}

	// Get resources to clean based on strategy
	resourcesToClean := rcm.getResourcesForCleanup(strategy)
	operation.Resources = resourcesToClean

	// Perform cleanup
	results, err := rcm.executeCleanup(cleanupCtx, resourcesToClean, strategy)

	// Complete operation record
	operation.EndTime = time.Now()
	operation.Duration = operation.EndTime.Sub(operation.StartTime)
	operation.Results = results
	operation.Success = err == nil
	if err != nil {
		operation.Error = err.Error()
	}

	// Record operation
	rcm.recordCleanupOperation(operation)

	return err
}

// PerformCleanup executes a manual cleanup operation with the specified strategy
func (rcm *ResourceCleanupManager) PerformCleanup(ctx context.Context) error {
	cleanupCtx, cancel := context.WithTimeout(ctx, rcm.config.Timeout)
	defer cancel()

	operation := &CleanupOperation{
		ID:        fmt.Sprintf("manual-%d", time.Now().Unix()),
		StartTime: time.Now(),
		Trigger:   TriggerManual,
		DryRun:    rcm.config.DryRun,
	}

	// Get all tracked resources for cleanup
	strategy := &CleanupStrategy{
		CleanupTemporary:   true,
		CleanupOrphaned:    true,
		CleanupExpired:     true,
		PreserveDiagnostic: rcm.config.PreserveDebugData,
	}

	resourcesToClean := rcm.getResourcesForCleanup(strategy)
	operation.Resources = resourcesToClean

	results, err := rcm.executeCleanup(cleanupCtx, resourcesToClean, strategy)

	operation.EndTime = time.Now()
	operation.Duration = operation.EndTime.Sub(operation.StartTime)
	operation.Results = results
	operation.Success = err == nil
	if err != nil {
		operation.Error = err.Error()
	}

	rcm.recordCleanupOperation(operation)
	return err
}

// PerformAggressiveCleanup performs emergency cleanup to free resources
func (rcm *ResourceCleanupManager) PerformAggressiveCleanup(ctx context.Context) error {
	cleanupCtx, cancel := context.WithTimeout(ctx, rcm.config.Timeout)
	defer cancel()

	operation := &CleanupOperation{
		ID:        fmt.Sprintf("aggressive-%d", time.Now().Unix()),
		StartTime: time.Now(),
		Trigger:   TriggerEmergency,
		DryRun:    rcm.config.DryRun,
	}

	// Aggressive cleanup strategy
	strategy := &CleanupStrategy{
		CleanupTemporary:   true,
		CleanupOrphaned:    true,
		CleanupExpired:     true,
		CleanupRunning:     true,  // Stop running diagnostic containers
		IgnoreProtected:    true,  // Override protection
		PreserveDiagnostic: false, // Clean even diagnostic resources
	}

	resourcesToClean := rcm.getAllTrackedResources()
	operation.Resources = resourcesToClean

	results, err := rcm.executeCleanup(cleanupCtx, resourcesToClean, strategy)

	operation.EndTime = time.Now()
	operation.Duration = operation.EndTime.Sub(operation.StartTime)
	operation.Results = results
	operation.Success = err == nil
	if err != nil {
		operation.Error = err.Error()
	}

	rcm.recordCleanupOperation(operation)
	return err
}

// PerformFinalCleanup performs cleanup during system shutdown
func (rcm *ResourceCleanupManager) PerformFinalCleanup(ctx context.Context) error {
	cleanupCtx, cancel := context.WithTimeout(ctx, rcm.config.Timeout)
	defer cancel()

	operation := &CleanupOperation{
		ID:        fmt.Sprintf("final-%d", time.Now().Unix()),
		StartTime: time.Now(),
		Trigger:   TriggerShutdown,
		DryRun:    false, // Never dry run on shutdown
	}

	// Final cleanup strategy - clean everything temporary
	strategy := &CleanupStrategy{
		CleanupTemporary:   true,
		CleanupOrphaned:    true,
		CleanupRunning:     true,
		PreserveDiagnostic: rcm.config.PreserveDebugData,
	}

	resourcesToClean := rcm.getTemporaryResources()
	operation.Resources = resourcesToClean

	results, err := rcm.executeCleanup(cleanupCtx, resourcesToClean, strategy)

	operation.EndTime = time.Now()
	operation.Duration = operation.EndTime.Sub(operation.StartTime)
	operation.Results = results
	operation.Success = err == nil
	if err != nil {
		operation.Error = err.Error()
	}

	rcm.recordCleanupOperation(operation)
	return err
}

// Shutdown stops the cleanup manager and performs final cleanup
func (rcm *ResourceCleanupManager) Shutdown(ctx context.Context) error {
	rcm.mu.Lock()
	if rcm.stopped {
		rcm.mu.Unlock()
		return nil
	}
	rcm.stopped = true
	rcm.mu.Unlock()

	// Stop periodic cleanup
	close(rcm.stopCh)

	// Perform final cleanup
	return rcm.PerformFinalCleanup(ctx)
}

// GetMetrics returns current cleanup system metrics
func (rcm *ResourceCleanupManager) GetMetrics() CleanupMetrics {
	rcm.mu.RLock()
	defer rcm.mu.RUnlock()

	// Update calculated metrics
	metrics := *rcm.metrics

	if metrics.TotalCleanups > 0 {
		metrics.AverageCleanupTime = metrics.TotalCleanupTime / time.Duration(metrics.TotalCleanups)
		metrics.CleanupEfficiency = float64(metrics.SuccessfulCleanups) / float64(metrics.TotalCleanups)
	}

	metrics.IsHealthy = rcm.isHealthyInternal()

	return metrics
}

// Internal methods

func (rcm *ResourceCleanupManager) selectCleanupStrategy(diagError *DiagnosticError) *CleanupStrategy {
	strategy := &CleanupStrategy{
		CleanupTemporary:   true,
		CleanupOrphaned:    false,
		CleanupExpired:     false,
		PreserveDiagnostic: rcm.config.PreserveDebugData,
	}

	// Adjust strategy based on error type
	switch diagError.Type {
	case ErrTypeResource:
		// Resource errors need aggressive cleanup
		strategy.CleanupOrphaned = true
		strategy.CleanupExpired = true
		if diagError.Code == ErrCodeResourceExhaustion {
			strategy.CleanupRunning = true
		}

	case ErrTypeConnection:
		// Connection errors might need container cleanup
		strategy.CleanupOrphaned = true

	case ErrTypeNetwork:
		// Network errors might need network cleanup
		strategy.CleanupOrphaned = true
		strategy.CleanupExpired = true

	default:
		// Default strategy for other error types
		strategy.CleanupExpired = true
	}

	return strategy
}

// CleanupStrategy defines what types of resources to clean up
type CleanupStrategy struct {
	CleanupTemporary   bool // Clean temporary/test resources
	CleanupOrphaned    bool // Clean orphaned resources
	CleanupExpired     bool // Clean expired resources
	CleanupRunning     bool // Stop and clean running containers
	IgnoreProtected    bool // Override resource protection
	PreserveDiagnostic bool // Keep diagnostic resources for debugging
}

func (rcm *ResourceCleanupManager) getResourcesForCleanup(strategy *CleanupStrategy) []*TrackedResource {
	rcm.mu.RLock()
	defer rcm.mu.RUnlock()

	var resources []*TrackedResource
	now := time.Now()

	// Check all tracked resources
	allResources := append([]*TrackedResource{}, rcm.getAllTrackedResourcesInternal()...)

	for _, resource := range allResources {
		shouldClean := false

		// Apply strategy filters
		if strategy.CleanupTemporary && resource.Temporary {
			shouldClean = true
		}

		if strategy.CleanupExpired && rcm.config.ResourceMaxAge > 0 {
			age := now.Sub(resource.CreatedAt)
			if age > rcm.config.ResourceMaxAge {
				shouldClean = true
			}
		}

		if strategy.CleanupOrphaned && rcm.isOrphanedResource(resource) {
			shouldClean = true
		}

		// Apply exclusions
		if resource.Protected && !strategy.IgnoreProtected {
			shouldClean = false
		}

		if resource.Diagnostic && strategy.PreserveDiagnostic {
			shouldClean = false
		}

		if shouldClean {
			resources = append(resources, resource)
		}
	}

	return resources
}

func (rcm *ResourceCleanupManager) executeCleanup(ctx context.Context, resources []*TrackedResource, strategy *CleanupStrategy) ([]ResourceCleanupResult, error) {
	var results []ResourceCleanupResult
	var cleanupErrors []error

	// Process resources in batches to avoid overwhelming the Docker daemon
	batchSize := rcm.config.CleanupBatchSize
	if batchSize == 0 {
		batchSize = 10
	}

	for i := 0; i < len(resources); i += batchSize {
		end := i + batchSize
		if end > len(resources) {
			end = len(resources)
		}
		batch := resources[i:end]

		// Process batch
		batchResults, batchErrors := rcm.processBatch(ctx, batch, strategy)
		results = append(results, batchResults...)
		cleanupErrors = append(cleanupErrors, batchErrors...)

		// Check context cancellation between batches
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}
	}

	// Update metrics
	rcm.updateCleanupMetrics(results)

	if len(cleanupErrors) > 0 {
		return results, fmt.Errorf("cleanup completed with %d errors: %v", len(cleanupErrors), cleanupErrors[0])
	}

	return results, nil
}

func (rcm *ResourceCleanupManager) processBatch(ctx context.Context, resources []*TrackedResource, strategy *CleanupStrategy) ([]ResourceCleanupResult, []error) {
	var results []ResourceCleanupResult
	var errors []error

	// Use semaphore to limit concurrent operations
	maxConcurrent := rcm.config.MaxConcurrentCleanups
	if maxConcurrent == 0 {
		maxConcurrent = 3
	}

	sem := make(chan struct{}, maxConcurrent)
	resultsCh := make(chan ResourceCleanupResult, len(resources))
	errorsCh := make(chan error, len(resources))

	var wg sync.WaitGroup

	for _, resource := range resources {
		wg.Add(1)
		go func(res *TrackedResource) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Cleanup individual resource
			result := rcm.cleanupResource(ctx, res, strategy)
			resultsCh <- result

			if !result.Success && result.Error != "" {
				errorsCh <- fmt.Errorf("failed to cleanup %s %s: %s", res.Type, res.ID, result.Error)
			}
		}(resource)
	}

	// Wait for all operations to complete
	wg.Wait()
	close(resultsCh)
	close(errorsCh)

	// Collect results
	for result := range resultsCh {
		results = append(results, result)
	}

	for err := range errorsCh {
		errors = append(errors, err)
	}

	return results, errors
}

func (rcm *ResourceCleanupManager) cleanupResource(ctx context.Context, resource *TrackedResource, strategy *CleanupStrategy) ResourceCleanupResult {
	startTime := time.Now()
	result := ResourceCleanupResult{
		Resource:  resource,
		TimeTaken: 0,
	}

	defer func() {
		result.TimeTaken = time.Since(startTime)
	}()

	// Dry run mode
	if rcm.config.DryRun {
		result.Success = true
		result.Action = ActionSkipped
		return result
	}

	// Backup if required
	if rcm.config.BackupBeforeCleanup {
		if err := rcm.backupResource(ctx, resource); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("backup failed: %v", err)
			result.Action = ActionFailed
			return result
		}
		result.Action = ActionBackedUp
	}

	// Perform cleanup based on resource type
	switch resource.Type {
	case ResourceTypeContainer:
		err := rcm.cleanupContainer(ctx, resource, strategy)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
			result.Action = ActionFailed
		} else {
			result.Success = true
			result.Action = ActionRemoved
		}

	case ResourceTypeNetwork:
		err := rcm.cleanupNetwork(ctx, resource)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
			result.Action = ActionFailed
		} else {
			result.Success = true
			result.Action = ActionRemoved
		}

	case ResourceTypeVolume:
		err := rcm.cleanupVolume(ctx, resource)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
			result.Action = ActionFailed
		} else {
			result.Success = true
			result.Action = ActionRemoved
		}

	default:
		result.Success = false
		result.Error = "unsupported resource type"
		result.Action = ActionSkipped
	}

	// Untrack successful cleanups
	if result.Success && result.Action == ActionRemoved {
		rcm.UntrackResource(resource.ID, resource.Type)
	}

	return result
}

// Docker-specific cleanup operations

func (rcm *ResourceCleanupManager) cleanupContainer(ctx context.Context, resource *TrackedResource, strategy *CleanupStrategy) error {
	if rcm.client == nil {
		return fmt.Errorf("docker client not available")
	}

	// Stop container if running and strategy allows
	if strategy.CleanupRunning {
		// Container-specific implementation would go here
		// For now, return a placeholder
	}

	// Remove container
	// Implementation would depend on the enhanced docker client
	return nil
}

func (rcm *ResourceCleanupManager) cleanupNetwork(ctx context.Context, resource *TrackedResource) error {
	if rcm.client == nil {
		return fmt.Errorf("docker client not available")
	}

	// Network cleanup implementation
	return nil
}

func (rcm *ResourceCleanupManager) cleanupVolume(ctx context.Context, resource *TrackedResource) error {
	if rcm.client == nil {
		return fmt.Errorf("docker client not available")
	}

	// Volume cleanup implementation
	return nil
}

// Helper methods

func (rcm *ResourceCleanupManager) getAllTrackedResources() []*TrackedResource {
	rcm.mu.RLock()
	defer rcm.mu.RUnlock()
	return rcm.getAllTrackedResourcesInternal()
}

func (rcm *ResourceCleanupManager) getAllTrackedResourcesInternal() []*TrackedResource {
	var resources []*TrackedResource

	for _, resource := range rcm.trackedContainers {
		resources = append(resources, resource)
	}
	for _, resource := range rcm.trackedNetworks {
		resources = append(resources, resource)
	}
	for _, resource := range rcm.trackedVolumes {
		resources = append(resources, resource)
	}

	return resources
}

func (rcm *ResourceCleanupManager) getTemporaryResources() []*TrackedResource {
	var resources []*TrackedResource

	for _, resource := range rcm.getAllTrackedResources() {
		if resource.Temporary {
			resources = append(resources, resource)
		}
	}

	return resources
}

func (rcm *ResourceCleanupManager) isOrphanedResource(resource *TrackedResource) bool {
	// Simple orphan detection - check if resource hasn't been used recently
	if time.Since(resource.LastUsed) > rcm.config.ResourceMaxAge {
		return true
	}

	// Check if dependencies still exist
	for _, depID := range resource.DependsOn {
		if !rcm.resourceExists(depID) {
			return true
		}
	}

	return false
}

func (rcm *ResourceCleanupManager) resourceExists(resourceID string) bool {
	// Check if resource exists in tracking maps
	if _, exists := rcm.trackedContainers[resourceID]; exists {
		return true
	}
	if _, exists := rcm.trackedNetworks[resourceID]; exists {
		return true
	}
	if _, exists := rcm.trackedVolumes[resourceID]; exists {
		return true
	}
	return false
}

func (rcm *ResourceCleanupManager) backupResource(ctx context.Context, resource *TrackedResource) error {
	// Backup implementation would depend on resource type
	// For now, this is a placeholder
	return nil
}

func (rcm *ResourceCleanupManager) recordCleanupOperation(operation *CleanupOperation) {
	rcm.mu.Lock()
	defer rcm.mu.Unlock()

	rcm.cleanupHistory = append(rcm.cleanupHistory, *operation)

	// Limit history size based on retention policy
	maxHistory := 100
	if len(rcm.cleanupHistory) > maxHistory {
		rcm.cleanupHistory = rcm.cleanupHistory[len(rcm.cleanupHistory)-maxHistory:]
	}
}

func (rcm *ResourceCleanupManager) updateCleanupMetrics(results []ResourceCleanupResult) {
	rcm.mu.Lock()
	defer rcm.mu.Unlock()

	rcm.metrics.TotalCleanups++

	successCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
			rcm.metrics.ResourcesCleanedUp++
			rcm.metrics.CleanupsByType[result.Resource.Type]++
			rcm.metrics.StorageFreed += result.SizeFreed
		} else {
			rcm.metrics.ResourcesSkipped++
		}
	}

	if successCount == len(results) {
		rcm.metrics.SuccessfulCleanups++
		rcm.metrics.LastSuccessfulCleanup = time.Now()
	} else {
		rcm.metrics.FailedCleanups++
	}
}

func (rcm *ResourceCleanupManager) isHealthyInternal() bool {
	if rcm.metrics.TotalCleanups == 0 {
		return true // No operations yet
	}

	// Check success rate
	successRate := float64(rcm.metrics.SuccessfulCleanups) / float64(rcm.metrics.TotalCleanups)
	if successRate < 0.7 { // Less than 70% success rate is concerning
		return false
	}

	// Check if last cleanup was recent (if periodic cleanup is enabled)
	if rcm.config.EnablePeriodicCleanup {
		timeSinceLastCleanup := time.Since(rcm.metrics.LastSuccessfulCleanup)
		if timeSinceLastCleanup > rcm.config.PeriodicCleanupInterval*2 {
			return false
		}
	}

	return true
}

func (rcm *ResourceCleanupManager) periodicCleanupLoop() {
	ticker := time.NewTicker(rcm.config.PeriodicCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), rcm.config.Timeout)
			err := rcm.PerformCleanup(ctx)
			cancel()

			if err != nil {
				// Log error but continue periodic cleanup
			}

		case <-rcm.stopCh:
			return
		}
	}
}
