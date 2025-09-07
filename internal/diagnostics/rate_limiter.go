// Package diagnostics provides rate limiting for Docker API calls
package diagnostics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter provides rate limiting for Docker API calls
type RateLimiter struct {
	limiter      *rate.Limiter
	mu           sync.RWMutex
	metrics      *RateLimiterMetrics
	config       *RateLimiterConfig
}

// RateLimiterConfig configures the rate limiter
type RateLimiterConfig struct {
	RequestsPerSecond float64       // Rate limit (requests per second)
	BurstSize         int           // Maximum burst size
	WaitTimeout       time.Duration // Maximum wait time for a token
	Enabled           bool          // Whether rate limiting is enabled
}

// RateLimiterMetrics tracks rate limiter statistics
type RateLimiterMetrics struct {
	mu               sync.RWMutex
	TotalRequests    int64
	AllowedRequests  int64
	ThrottledRequests int64
	TimeoutRequests  int64
	TotalWaitTime    time.Duration
	MaxWaitTime      time.Duration
	LastRequest      time.Time
}

// DefaultRateLimiterConfig returns the default rate limiter configuration
func DefaultRateLimiterConfig() *RateLimiterConfig {
	return &RateLimiterConfig{
		RequestsPerSecond: DOCKER_API_RATE_LIMIT,
		BurstSize:         DOCKER_API_BURST,
		WaitTimeout:       5 * time.Second,
		Enabled:           true,
	}
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config *RateLimiterConfig) *RateLimiter {
	if config == nil {
		config = DefaultRateLimiterConfig()
	}

	rl := &RateLimiter{
		config:  config,
		metrics: &RateLimiterMetrics{},
	}

	if config.Enabled {
		rl.limiter = rate.NewLimiter(rate.Limit(config.RequestsPerSecond), config.BurstSize)
	}

	return rl
}

// Wait blocks until a token is available or the context is cancelled
func (rl *RateLimiter) Wait(ctx context.Context) error {
	// Record request
	rl.metrics.mu.Lock()
	rl.metrics.TotalRequests++
	rl.metrics.mu.Unlock()

	// If rate limiting is disabled, allow immediately
	if !rl.config.Enabled || rl.limiter == nil {
		rl.metrics.mu.Lock()
		rl.metrics.AllowedRequests++
		rl.metrics.LastRequest = time.Now()
		rl.metrics.mu.Unlock()
		return nil
	}

	// Create timeout context if configured
	waitCtx := ctx
	if rl.config.WaitTimeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, rl.config.WaitTimeout)
		defer cancel()
	}

	// Wait for token
	startTime := time.Now()
	err := rl.limiter.Wait(waitCtx)
	waitTime := time.Since(startTime)

	// Update metrics
	rl.updateMetrics(err, waitTime)

	if err != nil {
		if err == context.DeadlineExceeded {
			return fmt.Errorf("rate limiter timeout after %v", waitTime)
		}
		return fmt.Errorf("rate limiter error: %w", err)
	}

	return nil
}

// TryAcquire attempts to acquire a token without blocking
func (rl *RateLimiter) TryAcquire() bool {
	// Record request
	rl.metrics.mu.Lock()
	rl.metrics.TotalRequests++
	rl.metrics.mu.Unlock()

	// If rate limiting is disabled, allow immediately
	if !rl.config.Enabled || rl.limiter == nil {
		rl.metrics.mu.Lock()
		rl.metrics.AllowedRequests++
		rl.metrics.LastRequest = time.Now()
		rl.metrics.mu.Unlock()
		return true
	}

	// Try to acquire token
	allowed := rl.limiter.Allow()

	// Update metrics
	rl.metrics.mu.Lock()
	if allowed {
		rl.metrics.AllowedRequests++
		rl.metrics.LastRequest = time.Now()
	} else {
		rl.metrics.ThrottledRequests++
	}
	rl.metrics.mu.Unlock()

	return allowed
}

// Reserve reserves a token for future use
func (rl *RateLimiter) Reserve() *rate.Reservation {
	if !rl.config.Enabled || rl.limiter == nil {
		return nil
	}
	return rl.limiter.Reserve()
}

// updateMetrics updates rate limiter metrics
func (rl *RateLimiter) updateMetrics(err error, waitTime time.Duration) {
	rl.metrics.mu.Lock()
	defer rl.metrics.mu.Unlock()

	if err != nil {
		if err == context.DeadlineExceeded {
			rl.metrics.TimeoutRequests++
		} else {
			rl.metrics.ThrottledRequests++
		}
	} else {
		rl.metrics.AllowedRequests++
		rl.metrics.LastRequest = time.Now()
		rl.metrics.TotalWaitTime += waitTime
		
		if waitTime > rl.metrics.MaxWaitTime {
			rl.metrics.MaxWaitTime = waitTime
		}
	}
}

// GetMetrics returns current rate limiter metrics
func (rl *RateLimiter) GetMetrics() RateLimiterMetrics {
	rl.metrics.mu.RLock()
	defer rl.metrics.mu.RUnlock()
	
	return *rl.metrics
}

// ResetMetrics resets the rate limiter metrics
func (rl *RateLimiter) ResetMetrics() {
	rl.metrics.mu.Lock()
	defer rl.metrics.mu.Unlock()
	
	rl.metrics = &RateLimiterMetrics{}
}

// UpdateConfig updates the rate limiter configuration
func (rl *RateLimiter) UpdateConfig(config *RateLimiterConfig) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.config = config
	
	if config.Enabled {
		rl.limiter = rate.NewLimiter(rate.Limit(config.RequestsPerSecond), config.BurstSize)
	} else {
		rl.limiter = nil
	}
}

// IsEnabled returns whether rate limiting is enabled
func (rl *RateLimiter) IsEnabled() bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	return rl.config.Enabled
}

// GetConfig returns the current rate limiter configuration
func (rl *RateLimiter) GetConfig() RateLimiterConfig {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	return *rl.config
}

// GetEffectiveRate returns the effective rate limit
func (rl *RateLimiter) GetEffectiveRate() float64 {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	if !rl.config.Enabled || rl.limiter == nil {
		return 0 // No limit
	}
	
	return rl.config.RequestsPerSecond
}

// GetBurstSize returns the burst size
func (rl *RateLimiter) GetBurstSize() int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	if !rl.config.Enabled {
		return 0
	}
	
	return rl.config.BurstSize
}

// GetAverageWaitTime returns the average wait time for requests
func (rl *RateLimiter) GetAverageWaitTime() time.Duration {
	rl.metrics.mu.RLock()
	defer rl.metrics.mu.RUnlock()
	
	if rl.metrics.AllowedRequests == 0 {
		return 0
	}
	
	return rl.metrics.TotalWaitTime / time.Duration(rl.metrics.AllowedRequests)
}

// GetThrottleRate returns the percentage of requests that were throttled
func (rl *RateLimiter) GetThrottleRate() float64 {
	rl.metrics.mu.RLock()
	defer rl.metrics.mu.RUnlock()
	
	if rl.metrics.TotalRequests == 0 {
		return 0
	}
	
	throttled := rl.metrics.ThrottledRequests + rl.metrics.TimeoutRequests
	return float64(throttled) / float64(rl.metrics.TotalRequests) * 100
}

// GetStatus returns a human-readable status of the rate limiter
func (rl *RateLimiter) GetStatus() string {
	if !rl.IsEnabled() {
		return "Rate limiting disabled"
	}

	metrics := rl.GetMetrics()
	throttleRate := rl.GetThrottleRate()
	avgWaitTime := rl.GetAverageWaitTime()

	return fmt.Sprintf(
		"Rate Limiter Status: %.1f req/s (burst: %d) | "+
			"Total: %d | Allowed: %d | Throttled: %d (%.1f%%) | "+
			"Avg Wait: %v | Max Wait: %v",
		rl.GetEffectiveRate(),
		rl.GetBurstSize(),
		metrics.TotalRequests,
		metrics.AllowedRequests,
		metrics.ThrottledRequests+metrics.TimeoutRequests,
		throttleRate,
		avgWaitTime,
		metrics.MaxWaitTime,
	)
}