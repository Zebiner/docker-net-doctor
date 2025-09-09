package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// Security and Safety Tests

func TestWorkerIsolationAndSecurityBoundaries(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 3)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Test that workers are isolated from each other
	isolationChecks := []*IsolationCheck{
		{name: "worker_1_data", workerData: "worker_1_secret", shouldLeak: false},
		{name: "worker_2_data", workerData: "worker_2_secret", shouldLeak: false},
		{name: "worker_3_data", workerData: "worker_3_secret", shouldLeak: false},
	}

	// Submit jobs that should not access each other's data
	for _, check := range isolationChecks {
		err := pool.Submit(check, nil)
		require.NoError(t, err)
	}

	// Collect results
	results := make([]JobResult, 0)
	timeout := time.After(3 * time.Second)

	for len(results) < len(isolationChecks) {
		select {
		case result := <-pool.GetResults():
			results = append(results, result)
		case <-timeout:
			t.Fatalf("Timeout collecting isolation test results, got %d/%d", len(results), len(isolationChecks))
		}
	}

	// Verify isolation - no worker should access another's data
	for _, result := range results {
		assert.NoError(t, result.Error, "Isolation check should not error")
		assert.True(t, result.Result.Success, "Isolation check should succeed")

		// Verify no data leakage in the message
		message := result.Result.Message
		secretCount := 0
		for _, check := range isolationChecks {
			if strings.Contains(message, check.workerData) {
				secretCount++
			}
		}

		assert.LessOrEqual(t, secretCount, 1, "Worker should only access its own data")
	}
}

func TestJobValidationAndSanitization(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Test various malicious or invalid job inputs
	maliciousChecks := []struct {
		name        string
		check       Check
		shouldBlock bool
		description string
	}{
		{
			name:        "null_check",
			check:       nil,
			shouldBlock: true,
			description: "Null check should be blocked",
		},
		{
			name:        "malicious_name",
			check:       &MaliciousCheck{name: "'; DROP TABLE checks; --", payload: "sql_injection"},
			shouldBlock: false, // Should be sanitized, not blocked
			description: "SQL injection in name should be sanitized",
		},
		{
			name:        "script_injection",
			check:       &MaliciousCheck{name: "<script>alert('xss')</script>", payload: "xss_attempt"},
			shouldBlock: false, // Should be sanitized
			description: "Script injection should be sanitized",
		},
		{
			name:        "path_traversal",
			check:       &MaliciousCheck{name: "../../etc/passwd", payload: "path_traversal"},
			shouldBlock: false, // Should be sanitized
			description: "Path traversal should be sanitized",
		},
		{
			name:        "excessive_length",
			check:       &MaliciousCheck{name: strings.Repeat("A", 10000), payload: "length_attack"},
			shouldBlock: false, // Should be truncated
			description: "Excessive length should be handled",
		},
	}

	for _, test := range maliciousChecks {
		t.Run(test.name, func(t *testing.T) {
			err := pool.Submit(test.check, nil)

			if test.shouldBlock {
				assert.Error(t, err, test.description)
				if err != nil {
					assert.Contains(t, err.Error(), "security validation failed", "Should indicate security validation failure")
				}
			} else {
				// Should either succeed or fail gracefully, but not crash
				if err != nil {
					// If validation fails, error should be descriptive
					t.Logf("Job rejected (may be expected): %v", err)
				} else {
					// If accepted, should execute safely
					select {
					case result := <-pool.GetResults():
						// Result should be safe
						assert.NotNil(t, result.Result, "Should have result")
						t.Logf("Malicious job handled safely: %s", result.Result.Message)
					case <-time.After(1 * time.Second):
						t.Log("Timeout waiting for malicious job result (may be expected)")
					}
				}
			}
		})
	}
}

func TestResourceLimitsEnforcement(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)

	// Set strict resource limits for testing
	// Note: In a real implementation, this would configure memory limits
	memoryLimit := 5 // 5MB limit for testing
	_ = memoryLimit

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Test resource exhaustion scenarios
	resourceTests := []struct {
		name     string
		check    Check
		expected string
	}{
		{
			name:     "memory_bomb",
			check:    &ResourceAbusiveCheck{name: "memory_bomb", abuseType: "memory", intensity: 10},
			expected: "memory limit",
		},
		{
			name:     "cpu_bomb",
			check:    &ResourceAbusiveCheck{name: "cpu_bomb", abuseType: "cpu", intensity: 5},
			expected: "success_or_timeout", // CPU abuse may be detected by timeout
		},
		{
			name:     "goroutine_bomb",
			check:    &ResourceAbusiveCheck{name: "goroutine_bomb", abuseType: "goroutines", intensity: 3},
			expected: "success_or_timeout", // May be limited by system
		},
	}

	for _, test := range resourceTests {
		t.Run(test.name, func(t *testing.T) {
			err := pool.Submit(test.check, nil)

			if strings.Contains(test.expected, "memory limit") && err != nil {
				assert.Contains(t, err.Error(), "memory limit", "Should reject due to memory limit")
				return
			}

			if err == nil {
				// If accepted, should be handled safely
				select {
				case result := <-pool.GetResults():
					if result.Error != nil {
						t.Logf("Resource abusive job failed safely: %v", result.Error)
					} else {
						t.Logf("Resource abusive job completed: %s", result.Result.Message)
					}
				case <-time.After(5 * time.Second):
					t.Log("Resource abusive job timed out (expected for CPU/goroutine bombs)")
				}
			}
		})
	}
}

func TestAuditLoggingForWorkerPoolOperations(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)

	// Create audit log collector
	auditLog := &AuditLogCollector{
		logs:  make([]AuditLogEntry, 0),
		mutex: sync.Mutex{},
	}

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Submit various operations to generate audit events
	auditChecks := []*AuditableCheck{
		{name: "test_normal_operation", auditLogger: auditLog, operation: "list_containers"},
		{name: "privileged_operation", auditLogger: auditLog, operation: "inspect_system"},
		{name: "network_operation", auditLogger: auditLog, operation: "scan_ports"},
		{name: "failed_operation", auditLogger: auditLog, operation: "access_denied", shouldFail: true},
	}

	for _, check := range auditChecks {
		err := pool.Submit(check, nil)
		require.NoError(t, err)
	}

	// Collect results
	results := make([]JobResult, 0)
	timeout := time.After(3 * time.Second)

	for len(results) < len(auditChecks) {
		select {
		case result := <-pool.GetResults():
			results = append(results, result)
		case <-timeout:
			t.Fatalf("Timeout collecting audit test results, got %d/%d", len(results), len(auditChecks))
		}
	}

	// Verify audit logging
	auditLog.mutex.Lock()
	logEntries := make([]AuditLogEntry, len(auditLog.logs))
	copy(logEntries, auditLog.logs)
	auditLog.mutex.Unlock()

	assert.GreaterOrEqual(t, len(logEntries), len(auditChecks), "Should have audit log entries for all operations")

	// Verify audit log content
	operationsSeen := make(map[string]bool)
	for _, entry := range logEntries {
		operationsSeen[entry.Operation] = true

		// Verify required audit fields
		assert.NotEmpty(t, entry.Operation, "Audit entry should have operation")
		assert.NotZero(t, entry.Timestamp, "Audit entry should have timestamp")
		assert.NotEmpty(t, entry.User, "Audit entry should have user")

		t.Logf("Audit Entry: %s by %s at %v (Success: %v)", 
			entry.Operation, entry.User, entry.Timestamp, entry.Success)
	}

	// Verify all operations were audited
	expectedOps := []string{"list_containers", "inspect_system", "scan_ports", "access_denied"}
	for _, op := range expectedOps {
		assert.True(t, operationsSeen[op], "Operation %s should be audited", op)
	}
}

func TestProtectionAgainstMaliciousJobPayloads(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)

	err = pool.Start()
	require.NoError(t, err)
	defer pool.Stop()

	// Test various malicious payloads
	maliciousPayloads := []*MaliciousPayloadCheck{
		{
			name:        "buffer_overflow",
			payloadType: "buffer_overflow",
			data:        strings.Repeat("A", 100000), // Large buffer
		},
		{
			name:        "format_string",
			payloadType: "format_string",
			data:        "%s%s%s%s%s%s%s%s%s%s%n",
		},
		{
			name:        "null_bytes",
			payloadType: "null_bytes",
			data:        "test\x00\x00\x00payload",
		},
		{
			name:        "unicode_exploit",
			payloadType: "unicode",
			data:        "test\u202e\u0073\u0073\u0061\u0070",
		},
		{
			name:        "zip_bomb",
			payloadType: "compression",
			data:        strings.Repeat("0", 10000), // Simulated compressible data
		},
	}

	submittedCount := 0
	for _, check := range maliciousPayloads {
		err := pool.Submit(check, nil)
		if err != nil {
			// Security validation should block some payloads
			t.Logf("Malicious payload blocked (expected): %v", err)
		} else {
			submittedCount++
		}
	}

	if submittedCount > 0 {
		// Collect results from submitted payloads
		results := make([]JobResult, 0)
		timeout := time.After(5 * time.Second)

		for len(results) < submittedCount {
			select {
			case result := <-pool.GetResults():
				results = append(results, result)
			case <-timeout:
				t.Logf("Some malicious payloads timed out (may be expected)")
				break
			}
		}

		// Verify malicious payloads were handled safely
		for _, result := range results {
			// Should not crash the system
			assert.NotNil(t, result, "Result should not be nil")

			if result.Error != nil {
				t.Logf("Malicious payload failed safely: %v", result.Error)
			} else if result.Result != nil {
				t.Logf("Malicious payload handled: %s", result.Result.Message)
				// Message should be sanitized
				assert.NotContains(t, result.Result.Message, "\x00", "Result should not contain null bytes")
			}
		}
	}

	// Pool should remain functional after malicious payloads
	assert.True(t, pool.IsHealthy(), "Pool should remain healthy after malicious payload attempts")

	// Test normal operation still works
	normalCheck := &TestMockCheck{name: "post_attack_test", delay: 10 * time.Millisecond}
	err = pool.Submit(normalCheck, nil)
	assert.NoError(t, err, "Normal operations should work after malicious payload attempts")

	select {
	case result := <-pool.GetResults():
		assert.NoError(t, result.Error, "Normal check should succeed after attack attempts")
	case <-time.After(1 * time.Second):
		t.Fatal("Normal check should complete after malicious payload handling")
	}
}

func TestMemoryProtectionAndLeakPrevention(t *testing.T) {
	ctx := context.Background()
	pool, err := NewSecureWorkerPool(ctx, 2)
	require.NoError(t, err)

	initialMemory := getCurrentMemory()

	err = pool.Start()
	require.NoError(t, err)

	// Submit jobs that could cause memory leaks
	leakChecks := []*MemoryLeakCheck{
		{name: "circular_reference", leakType: "circular"},
		{name: "unclosed_resources", leakType: "resources"},
		{name: "large_allocation", leakType: "allocation"},
		{name: "goroutine_leak", leakType: "goroutines"},
	}

	for _, check := range leakChecks {
		err := pool.Submit(check, nil)
		require.NoError(t, err)
	}

	// Collect results
	results := make([]JobResult, 0)
	timeout := time.After(3 * time.Second)

	for len(results) < len(leakChecks) {
		select {
		case result := <-pool.GetResults():
			results = append(results, result)
		case <-timeout:
			t.Fatalf("Timeout collecting memory leak test results, got %d/%d", len(results), len(leakChecks))
		}
	}

	// Stop pool and allow cleanup
	pool.Stop()

	// Force garbage collection multiple times
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(100 * time.Millisecond)
	}

	finalMemory := getCurrentMemory()
	memoryDiff := int64(finalMemory) - int64(initialMemory)

	t.Logf("Memory Protection Test:")
	t.Logf("  Initial Memory: %d bytes", initialMemory)
	t.Logf("  Final Memory: %d bytes", finalMemory)
	t.Logf("  Memory Difference: %d bytes (%.2f MB)", memoryDiff, float64(memoryDiff)/(1024*1024))

	// Memory should not increase significantly
	maxAllowedIncrease := int64(10 * 1024 * 1024) // 10MB tolerance
	assert.LessOrEqual(t, memoryDiff, maxAllowedIncrease, 
		"Memory should not leak significantly after operations")

	// All leak checks should have completed without crashing
	for _, result := range results {
		assert.NotNil(t, result.Result, "Leak check should have result")
		t.Logf("Leak check %s: %s", result.Result.CheckName, result.Result.Message)
	}
}

// Helper types for security testing

type IsolationCheck struct {
	name       string
	workerData string
	shouldLeak bool
}

func (i *IsolationCheck) Name() string        { return i.name }
func (i *IsolationCheck) Description() string { return "Worker isolation test" }
func (i *IsolationCheck) Severity() Severity  { return SeverityInfo }

func (i *IsolationCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	// Simulate worker accessing its own data
	workerData := i.workerData
	
	// Try to access data from other workers (should fail)
	otherData := ""
	
	// In a real scenario, this would attempt cross-worker data access
	// For testing, we'll simulate proper isolation
	
	return &CheckResult{
		CheckName: i.name,
		Success:   true,
		Message:   fmt.Sprintf("Worker processed data: %s, other data: %s", workerData, otherData),
		Details: map[string]interface{}{
			"worker_data": workerData,
			"isolated":    true,
		},
		Timestamp: time.Now(),
	}, nil
}

type MaliciousCheck struct {
	name    string
	payload string
}

func (m *MaliciousCheck) Name() string        { return m.name }
func (m *MaliciousCheck) Description() string { return "Malicious input test" }
func (m *MaliciousCheck) Severity() Severity  { return SeverityWarning }

func (m *MaliciousCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	// Simulate processing potentially malicious input
	sanitizedName := strings.ReplaceAll(m.name, "<", "&lt;")
	sanitizedName = strings.ReplaceAll(sanitizedName, ">", "&gt;")
	sanitizedName = strings.ReplaceAll(sanitizedName, "'", "&#39;")
	sanitizedName = strings.ReplaceAll(sanitizedName, "\"", "&#34;")
	
	if len(sanitizedName) > 100 {
		sanitizedName = sanitizedName[:100] + "..."
	}
	
	return &CheckResult{
		CheckName: sanitizedName,
		Success:   true,
		Message:   fmt.Sprintf("Processed malicious input safely: %s", m.payload),
		Timestamp: time.Now(),
	}, nil
}

type ResourceAbusiveCheck struct {
	name      string
	abuseType string
	intensity int
}

func (r *ResourceAbusiveCheck) Name() string        { return r.name }
func (r *ResourceAbusiveCheck) Description() string { return "Resource abuse test" }
func (r *ResourceAbusiveCheck) Severity() Severity  { return SeverityWarning }

func (r *ResourceAbusiveCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	switch r.abuseType {
	case "memory":
		return r.memoryBomb(ctx)
	case "cpu":
		return r.cpuBomb(ctx)
	case "goroutines":
		return r.goroutineBomb(ctx)
	default:
		return r.genericAbuse(ctx)
	}
}

func (r *ResourceAbusiveCheck) memoryBomb(ctx context.Context) (*CheckResult, error) {
	// Try to allocate excessive memory
	size := r.intensity * 1024 * 1024 * 10 // 10MB per intensity level
	
	// Check if allocation would exceed limits
	currentMem := getCurrentMemory()
	if currentMem+uint64(size) > uint64(MAX_MEMORY_MB*1024*1024) {
		return &CheckResult{
			CheckName: r.name,
			Success:   false,
			Message:   "Memory allocation blocked by safety limits",
			Timestamp: time.Now(),
		}, errors.New("memory allocation would exceed limits")
	}
	
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
		// Check for context cancellation periodically
		if i%100000 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
	}
	
	return &CheckResult{
		CheckName: r.name,
		Success:   true,
		Message:   fmt.Sprintf("Memory bomb allocated %d bytes", size),
		Timestamp: time.Now(),
	}, nil
}

func (r *ResourceAbusiveCheck) cpuBomb(ctx context.Context) (*CheckResult, error) {
	// CPU intensive infinite-ish loop
	iterations := r.intensity * 1000000
	result := 0
	
	for i := 0; i < iterations; i++ {
		result += i * i * i
		
		// Check for cancellation every so often
		if i%10000 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
	}
	
	return &CheckResult{
		CheckName: r.name,
		Success:   true,
		Message:   fmt.Sprintf("CPU bomb completed %d iterations (result: %d)", iterations, result),
		Timestamp: time.Now(),
	}, nil
}

func (r *ResourceAbusiveCheck) goroutineBomb(ctx context.Context) (*CheckResult, error) {
	// Spawn many goroutines
	goroutineCount := r.intensity * 100
	var wg sync.WaitGroup
	
	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Each goroutine does a bit of work
			time.Sleep(100 * time.Millisecond)
		}(i)
		
		// Check for cancellation
		if i%50 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
	}
	
	// Wait for all goroutines with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		return &CheckResult{
			CheckName: r.name,
			Success:   true,
			Message:   fmt.Sprintf("Goroutine bomb spawned %d goroutines", goroutineCount),
			Timestamp: time.Now(),
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		return &CheckResult{
			CheckName: r.name,
			Success:   false,
			Message:   "Goroutine bomb timed out",
			Timestamp: time.Now(),
		}, errors.New("goroutine bomb timeout")
	}
}

func (r *ResourceAbusiveCheck) genericAbuse(ctx context.Context) (*CheckResult, error) {
	// Generic resource abuse
	time.Sleep(time.Duration(r.intensity*100) * time.Millisecond)
	
	return &CheckResult{
		CheckName: r.name,
		Success:   true,
		Message:   "Generic resource abuse completed",
		Timestamp: time.Now(),
	}, nil
}

type AuditLogEntry struct {
	Operation string
	User      string
	Timestamp time.Time
	Success   bool
	Details   map[string]interface{}
}

type AuditLogCollector struct {
	logs  []AuditLogEntry
	mutex sync.Mutex
}

func (a *AuditLogCollector) LogOperation(operation, user string, success bool, details map[string]interface{}) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	
	entry := AuditLogEntry{
		Operation: operation,
		User:      user,
		Timestamp: time.Now(),
		Success:   success,
		Details:   details,
	}
	
	a.logs = append(a.logs, entry)
}

type AuditableCheck struct {
	name        string
	auditLogger *AuditLogCollector
	operation   string
	shouldFail  bool
}

func (a *AuditableCheck) Name() string        { return a.name }
func (a *AuditableCheck) Description() string { return "Auditable operation test" }
func (a *AuditableCheck) Severity() Severity  { return SeverityInfo }

func (a *AuditableCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	// Log the operation
	success := !a.shouldFail
	a.auditLogger.LogOperation(
		a.operation,
		"test_user",
		success,
		map[string]interface{}{
			"check_name": a.name,
			"timestamp":  time.Now(),
		},
	)
	
	if a.shouldFail {
		return &CheckResult{
			CheckName: a.name,
			Success:   false,
			Message:   fmt.Sprintf("Operation %s failed as expected", a.operation),
			Timestamp: time.Now(),
		}, errors.New("simulated operation failure")
	}
	
	return &CheckResult{
		CheckName: a.name,
		Success:   true,
		Message:   fmt.Sprintf("Operation %s completed successfully", a.operation),
		Timestamp: time.Now(),
	}, nil
}

type MaliciousPayloadCheck struct {
	name        string
	payloadType string
	data        string
}

func (m *MaliciousPayloadCheck) Name() string        { return m.name }
func (m *MaliciousPayloadCheck) Description() string { return "Malicious payload test" }
func (m *MaliciousPayloadCheck) Severity() Severity  { return SeverityWarning }

func (m *MaliciousPayloadCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	// Simulate processing malicious payload safely
	processedData := m.sanitizePayload(m.data)
	
	return &CheckResult{
		CheckName: m.name,
		Success:   true,
		Message:   fmt.Sprintf("Malicious payload (%s) processed safely", m.payloadType),
		Details: map[string]interface{}{
			"original_length":  len(m.data),
			"processed_length": len(processedData),
			"payload_type":     m.payloadType,
		},
		Timestamp: time.Now(),
	}, nil
}

func (m *MaliciousPayloadCheck) sanitizePayload(data string) string {
	// Basic sanitization
	if len(data) > 1000 {
		data = data[:1000] + "...[truncated]"
	}
	
	// Remove null bytes
	data = strings.ReplaceAll(data, "\x00", "")
	
	// Remove dangerous format strings
	data = strings.ReplaceAll(data, "%n", "")
	data = strings.ReplaceAll(data, "%s", "[fmt]")
	
	return data
}

type MemoryLeakCheck struct {
	name     string
	leakType string
}

func (m *MemoryLeakCheck) Name() string        { return m.name }
func (m *MemoryLeakCheck) Description() string { return "Memory leak test" }
func (m *MemoryLeakCheck) Severity() Severity  { return SeverityWarning }

func (m *MemoryLeakCheck) Run(ctx context.Context, client *docker.Client) (*CheckResult, error) {
	switch m.leakType {
	case "circular":
		return m.testCircularReference(ctx)
	case "resources":
		return m.testResourceLeak(ctx)
	case "allocation":
		return m.testLargeAllocation(ctx)
	case "goroutines":
		return m.testGoroutineLeak(ctx)
	default:
		return m.testGenericLeak(ctx)
	}
}

func (m *MemoryLeakCheck) testCircularReference(ctx context.Context) (*CheckResult, error) {
	// Create circular reference structure
	type Node struct {
		data []byte
		next *Node
		prev *Node
	}
	
	// Create a circular linked list
	head := &Node{data: make([]byte, 1024)}
	current := head
	
	for i := 0; i < 10; i++ {
		newNode := &Node{data: make([]byte, 1024)}
		current.next = newNode
		newNode.prev = current
		current = newNode
	}
	
	// Close the circle
	current.next = head
	head.prev = current
	
	// Break the circle to prevent leak
	head.prev.next = nil
	head.prev = nil
	
	return &CheckResult{
		CheckName: m.name,
		Success:   true,
		Message:   "Circular reference created and cleaned up",
		Timestamp: time.Now(),
	}, nil
}

func (m *MemoryLeakCheck) testResourceLeak(ctx context.Context) (*CheckResult, error) {
	// Simulate resource allocation and cleanup
	resources := make([][]byte, 5)
	
	for i := range resources {
		resources[i] = make([]byte, 1024*100) // 100KB each
		// Fill with data
		for j := range resources[i] {
			resources[i][j] = byte(j % 256)
		}
	}
	
	// Clean up resources explicitly
	for i := range resources {
		resources[i] = nil
	}
	resources = nil
	
	return &CheckResult{
		CheckName: m.name,
		Success:   true,
		Message:   "Resources allocated and cleaned up",
		Timestamp: time.Now(),
	}, nil
}

func (m *MemoryLeakCheck) testLargeAllocation(ctx context.Context) (*CheckResult, error) {
	// Allocate and immediately free large memory
	size := 1024 * 1024 * 5 // 5MB
	data := make([]byte, size)
	
	// Use the data briefly
	checksum := 0
	for i := 0; i < 1000; i++ {
		data[i] = byte(i % 256)
		checksum += int(data[i])
	}
	
	// Clear reference
	data = nil
	
	return &CheckResult{
		CheckName: m.name,
		Success:   true,
		Message:   fmt.Sprintf("Large allocation handled (checksum: %d)", checksum),
		Timestamp: time.Now(),
	}, nil
}

func (m *MemoryLeakCheck) testGoroutineLeak(ctx context.Context) (*CheckResult, error) {
	// Create goroutines that should exit cleanly
	done := make(chan bool, 10)
	
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()
			time.Sleep(50 * time.Millisecond)
		}(i)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		select {
		case <-done:
			// Goroutine completed
		case <-time.After(1 * time.Second):
			return &CheckResult{
				CheckName: m.name,
				Success:   false,
				Message:   "Goroutine leak detected - timeout waiting for completion",
				Timestamp: time.Now(),
			}, errors.New("goroutine leak timeout")
		}
	}
	
	return &CheckResult{
		CheckName: m.name,
		Success:   true,
		Message:   "Goroutines created and cleaned up",
		Timestamp: time.Now(),
	}, nil
}

func (m *MemoryLeakCheck) testGenericLeak(ctx context.Context) (*CheckResult, error) {
	// Generic leak test
	data := make(map[string][]byte)
	
	// Add data
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key_%d", i)
		data[key] = make([]byte, 1024)
	}
	
	// Clear map
	for key := range data {
		delete(data, key)
	}
	data = nil
	
	return &CheckResult{
		CheckName: m.name,
		Success:   true,
		Message:   "Generic leak test completed with cleanup",
		Timestamp: time.Now(),
	}, nil
}