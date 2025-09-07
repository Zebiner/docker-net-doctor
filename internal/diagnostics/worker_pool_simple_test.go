package diagnostics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// SimpleCheck is a minimal check for debugging
type SimpleCheck struct {
	name string
}

func (s *SimpleCheck) Name() string        { return s.name }
func (s *SimpleCheck) Description() string { return "Simple test check" }
func (s *SimpleCheck) Severity() Severity  { return SeverityInfo }

func (s *SimpleCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	fmt.Printf("SimpleCheck %s running\n", s.name)
	return &CheckResult{
		CheckName: s.name,
		Success:   true,
		Message:   "OK",
		Timestamp: time.Now(),
	}, nil
}

func TestSimpleWorkerPool(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 2)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	// Start the pool
	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}
	defer pool.Stop()

	// Submit a simple job
	check := &SimpleCheck{name: "test1"}
	fmt.Printf("Submitting check: %s\n", check.Name())
	
	if err := pool.Submit(check, nil); err != nil {
		t.Fatalf("Failed to submit job: %v", err)
	}

	// Try to collect result
	resultsChan := pool.GetResults()
	
	select {
	case result := <-resultsChan:
		fmt.Printf("Got result: %+v\n", result)
		if result.Error != nil {
			t.Errorf("Job failed: %v", result.Error)
		}
		if result.Result == nil {
			t.Error("Result is nil")
		} else {
			fmt.Printf("Check result: %+v\n", result.Result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for result")
	}
}