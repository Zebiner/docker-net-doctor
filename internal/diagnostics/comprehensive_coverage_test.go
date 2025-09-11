package diagnostics

import (
	"testing"
	"time"

	"github.com/zebiner/docker-net-doctor/internal/docker"
)

// TestEngineConstructionAndMethods tests NewEngine and basic engine methods
func TestEngineConstructionAndMethods(t *testing.T) {
	// Create a mock docker client (this will be nil for testing)
	var client *docker.Client

	t.Run("NewEngine with nil config", func(t *testing.T) {
		engine := NewEngine(client, nil)
		if engine == nil {
			t.Error("NewEngine should not return nil")
		}
	})

	t.Run("NewEngine with custom config", func(t *testing.T) {
		config := &Config{
			Parallel:    true,
			Timeout:     30 * time.Second,
			Verbose:     true,
			WorkerCount: 4,
			RateLimit:   10.0,
		}

		engine := NewEngine(client, config)
		if engine == nil {
			t.Error("NewEngine should not return nil")
		}

		// Verify config was set
		if engine.config == nil {
			t.Error("engine config should not be nil")
		}
	})
}

// TestEngineRegisterDefaultChecks tests the registerDefaultChecks method
func TestEngineRegisterDefaultChecks(t *testing.T) {
	engine := NewEngine(nil, &Config{
		Parallel: false,
		Timeout:  10 * time.Second,
	})

	// Call registerDefaultChecks to ensure it executes without error
	engine.registerDefaultChecks()

	// Verify that checks were registered
	if len(engine.checks) == 0 {
		t.Error("expected checks to be registered")
	}
}

// TestDiagnosticChecksMetadata tests the Name, Description, and Severity methods of all check types
func TestDiagnosticChecksMetadata(t *testing.T) {
	checks := []Check{
		&DaemonConnectivityCheck{},
		&BridgeNetworkCheck{},
		&IPForwardingCheck{},
		&IptablesCheck{},
		&DNSResolutionCheck{},
		&InternalDNSCheck{},
		&ContainerConnectivityCheck{},
		&PortBindingCheck{},
		&NetworkIsolationCheck{},
		&MTUConsistencyCheck{},
	}

	for _, check := range checks {
		t.Run(check.Name(), func(t *testing.T) {
			name := check.Name()
			if name == "" {
				t.Error("Name() should not return empty string")
			}

			description := check.Description()
			if description == "" {
				t.Error("Description() should not return empty string")
			}

			severity := check.Severity()
			// Should be one of the defined severity levels
			if severity < SeverityInfo || severity > SeverityCritical {
				t.Errorf("Severity() returned invalid value: %v", severity)
			}
		})
	}
}

// TestSystemChecksSpecific tests specific system checks
func TestSystemChecksSpecific(t *testing.T) {
	t.Run("DaemonConnectivityCheck methods", func(t *testing.T) {
		check := &DaemonConnectivityCheck{}

		name := check.Name()
		if name != "daemon_connectivity" {
			t.Errorf("expected name 'daemon_connectivity', got %s", name)
		}

		desc := check.Description()
		if desc == "" {
			t.Error("description should not be empty")
		}

		severity := check.Severity()
		if severity != SeverityCritical {
			t.Errorf("expected SeverityCritical, got %v", severity)
		}
	})

	t.Run("NetworkIsolationCheck methods", func(t *testing.T) {
		check := &NetworkIsolationCheck{}

		name := check.Name()
		if name != "network_isolation" {
			t.Errorf("expected name 'network_isolation', got %s", name)
		}

		desc := check.Description()
		if desc == "" {
			t.Error("description should not be empty")
		}

		severity := check.Severity()
		if severity != SeverityWarning {
			t.Errorf("expected SeverityWarning, got %v", severity)
		}
	})
}

// TestTimingStorageBasicMethods tests TimingStorage methods
func TestTimingStorageBasicMethods(t *testing.T) {
	storage := NewTimingStorage(5)

	t.Run("basic operations", func(t *testing.T) {
		if storage.Size() != 0 {
			t.Errorf("expected size 0, got %d", storage.Size())
		}

		if storage.Capacity() != 5 {
			t.Errorf("expected capacity 5, got %d", storage.Capacity())
		}

		if storage.IsFull() {
			t.Error("storage should not be full initially")
		}

		// Test GetStatistics on empty storage
		stats := storage.GetStatistics()
		if stats.Count != 0 {
			t.Errorf("expected count 0, got %d", stats.Count)
		}

		// Test Clear
		storage.Clear()
		if storage.Size() != 0 {
			t.Errorf("expected size 0 after clear, got %d", storage.Size())
		}
	})
}

// TestSecurityValidatorConstructor tests SecurityValidator construction only
func TestSecurityValidatorConstructor(t *testing.T) {
	validator := NewSecurityValidator()
	if validator == nil {
		t.Error("NewSecurityValidator should not return nil")
	}
}

// TestPerformanceProfilerBasicMethods tests PerformanceProfiler methods
func TestPerformanceProfilerBasicMethods(t *testing.T) {
	profiler := NewPerformanceProfiler(&ProfileConfig{
		Enabled: true,
	})

	if profiler == nil {
		t.Error("NewPerformanceProfiler should not return nil")
	}

	t.Run("enable/disable", func(t *testing.T) {
		if !profiler.IsEnabled() {
			t.Error("profiler should be enabled by default with Enabled=true")
		}

		profiler.Disable()
		if profiler.IsEnabled() {
			t.Error("profiler should be disabled after calling Disable()")
		}

		profiler.Enable()
		if !profiler.IsEnabled() {
			t.Error("profiler should be enabled after calling Enable()")
		}
	})

	t.Run("metrics", func(t *testing.T) {
		metrics := profiler.GetMetrics()
		// Should return some metrics without error
		_ = metrics

		// Test Reset
		profiler.Reset()
		// Should not panic
	})

	t.Run("start/stop", func(t *testing.T) {
		profiler.Start()
		// Should not panic

		profiler.Stop()
		// Should not panic
	})
}

// TestConstructorsWithNilConfig tests various constructors with nil configs
func TestConstructorsWithNilConfig(t *testing.T) {
	t.Run("NewCircuitBreaker with nil", func(t *testing.T) {
		cb := NewCircuitBreaker(nil)
		if cb == nil {
			t.Error("NewCircuitBreaker should not return nil")
		}
	})

	t.Run("NewRateLimiter with nil", func(t *testing.T) {
		rl := NewRateLimiter(nil)
		if rl == nil {
			t.Error("NewRateLimiter should not return nil")
		}
	})

	t.Run("NewRetryPolicy with nil", func(t *testing.T) {
		rp := NewRetryPolicy(nil)
		if rp == nil {
			t.Error("NewRetryPolicy should not return nil")
		}
	})

	t.Run("NewTimingStorage", func(t *testing.T) {
		ts := NewTimingStorage(10)
		if ts == nil {
			t.Error("NewTimingStorage should not return nil")
		}
	})
}

// TestValidatorInitialization tests security validator initialization without calling methods
func TestValidatorInitialization(t *testing.T) {
	validator := NewSecurityValidatorTestMode()
	if validator == nil {
		t.Error("NewSecurityValidatorTestMode should not return nil")
	}
}
