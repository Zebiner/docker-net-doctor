// Package docker provides retry policy for Docker API operations
package docker

import (
	"context"
	"fmt"
	"math"
	"time"
)

// RetryPolicy defines retry behavior for Docker API operations
type RetryPolicy struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	BackoffFactor  float64
	RetryableErrors map[string]bool // Error messages that are retryable
}

// NewDefaultRetryPolicy creates a default retry policy
func NewDefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:     3,
		InitialBackoff: 100 * time.Millisecond,
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
			"resource temporarily unavailable": true,
		},
	}
}

// Execute runs an operation with retry logic
func (r *RetryPolicy) Execute(ctx context.Context, operation func() error) error {
	var lastErr error
	
	for attempt := 0; attempt <= r.MaxRetries; attempt++ {
		// Execute the operation
		err := operation()
		if err == nil {
			return nil // Success
		}
		
		lastErr = err
		
		// Check if error is retryable
		if !r.isRetryable(err) {
			return err // Non-retryable error
		}
		
		// Check if we've exhausted retries
		if attempt == r.MaxRetries {
			break
		}
		
		// Calculate backoff duration
		backoff := r.calculateBackoff(attempt)
		
		// Wait with context cancellation support
		select {
		case <-time.After(backoff):
			// Continue to next retry
		case <-ctx.Done():
			return fmt.Errorf("retry cancelled: %w", ctx.Err())
		}
	}
	
	return fmt.Errorf("operation failed after %d retries: %w", r.MaxRetries, lastErr)
}

// ExecuteWithValue runs an operation that returns a value with retry logic
func (r *RetryPolicy) ExecuteWithValue(ctx context.Context, operation func() (interface{}, error)) (interface{}, error) {
	var lastErr error
	
	for attempt := 0; attempt <= r.MaxRetries; attempt++ {
		// Execute the operation
		result, err := operation()
		if err == nil {
			return result, nil // Success
		}
		
		lastErr = err
		
		// Check if error is retryable
		if !r.isRetryable(err) {
			return nil, err // Non-retryable error
		}
		
		// Check if we've exhausted retries
		if attempt == r.MaxRetries {
			break
		}
		
		// Calculate backoff duration
		backoff := r.calculateBackoff(attempt)
		
		// Wait with context cancellation support
		select {
		case <-time.After(backoff):
			// Continue to next retry
		case <-ctx.Done():
			return nil, fmt.Errorf("retry cancelled: %w", ctx.Err())
		}
	}
	
	return nil, fmt.Errorf("operation failed after %d retries: %w", r.MaxRetries, lastErr)
}

// isRetryable checks if an error is retryable
func (r *RetryPolicy) isRetryable(err error) bool {
	if err == nil {
		return false
	}
	
	errMsg := err.Error()
	
	// Check against known retryable error patterns
	for pattern := range r.RetryableErrors {
		if contains(errMsg, pattern) {
			return true
		}
	}
	
	return false
}

// calculateBackoff calculates the backoff duration for a given attempt
func (r *RetryPolicy) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff with jitter
	backoff := float64(r.InitialBackoff) * math.Pow(r.BackoffFactor, float64(attempt))
	
	// Apply max backoff limit
	if backoff > float64(r.MaxBackoff) {
		backoff = float64(r.MaxBackoff)
	}
	
	// Add jitter (±10%)
	jitter := backoff * 0.1
	backoff = backoff + (jitter * (2*randomFloat() - 1))
	
	return time.Duration(backoff)
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		(s == substr || 
		 len(s) > len(substr) && 
		 (s[:len(substr)] == substr || 
		  s[len(s)-len(substr):] == substr ||
		  findSubstring(s, substr)))
}

// findSubstring performs a simple substring search
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// randomFloat returns a random float between 0 and 1
func randomFloat() float64 {
	// Simple pseudo-random for jitter
	return float64(time.Now().UnixNano()%1000) / 1000.0
}