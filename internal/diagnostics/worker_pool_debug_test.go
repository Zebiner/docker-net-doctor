package diagnostics

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// DebugCheck is a check that prints its state
type DebugCheck struct {
	name      string
	executed  atomic.Bool
	completed atomic.Bool
}

func (d *DebugCheck) Name() string        { return d.name }
func (d *DebugCheck) Description() string { return "Debug check: " + d.name }
func (d *DebugCheck) Severity() Severity  { return SeverityInfo }

func (d *DebugCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	fmt.Printf("[%s] Starting execution\n", d.name)
	d.executed.Store(true)
	
	// Simulate some work
	time.Sleep(5 * time.Millisecond)
	
	d.completed.Store(true)
	fmt.Printf("[%s] Completed execution\n", d.name)
	
	return &CheckResult{
		CheckName: d.name,
		Success:   true,
		Message:   "Debug check completed",
		Timestamp: time.Now(),
	}, nil
}

func TestWorkerPoolDebug(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 3)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	fmt.Printf("Pool created with %d workers\n", pool.workers)
	
	// Start the pool
	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}
	defer func() {
		fmt.Println("Stopping pool...")
		pool.Stop()
	}()

	// Create and submit multiple checks
	checks := []*DebugCheck{
		{name: "check1"},
		{name: "check2"},
		{name: "check3"},
		{name: "check4"},
		{name: "check5"},
	}

	fmt.Printf("Submitting %d checks...\n", len(checks))
	submitted := 0
	for _, check := range checks {
		if err := pool.Submit(check, nil); err != nil {
			fmt.Printf("Failed to submit %s: %v\n", check.name, err)
		} else {
			fmt.Printf("Submitted %s successfully\n", check.name)
			submitted++
		}
	}
	
	fmt.Printf("Successfully submitted %d/%d checks\n", submitted, len(checks))
	
	// Collect results
	resultsChan := pool.GetResults()
	results := make([]JobResult, 0)
	
	fmt.Println("Collecting results...")
	
	// Use a longer timeout for collecting all results
	timeout := time.After(10 * time.Second)
	expectedResults := submitted
	
	for len(results) < expectedResults {
		select {
		case result := <-resultsChan:
			fmt.Printf("Got result for job %d: success=%v, error=%v\n", 
				result.ID, result.Result != nil && result.Result.Success, result.Error)
			results = append(results, result)
		case <-timeout:
			fmt.Printf("Timeout! Got %d/%d results\n", len(results), expectedResults)
			
			// Check which checks were executed
			for _, check := range checks {
				fmt.Printf("  %s: executed=%v, completed=%v\n", 
					check.name, check.executed.Load(), check.completed.Load())
			}
			
			// Check pool state
			fmt.Printf("Pool state: active=%d, completed=%d, failed=%d\n",
				pool.activeJobs.Load(), pool.completedJobs.Load(), pool.failedJobs.Load())
			
			t.Fatalf("Timeout collecting results")
		}
	}
	
	fmt.Printf("Successfully collected %d results\n", len(results))
	
	// Verify all checks were executed
	for _, check := range checks {
		if !check.executed.Load() {
			t.Errorf("Check %s was not executed", check.name)
		}
		if !check.completed.Load() {
			t.Errorf("Check %s did not complete", check.name)
		}
	}
}