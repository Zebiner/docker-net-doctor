package diagnostics_test

import (
    "context"
    "testing"
    "time"
    
    "github.com/zebiner/docker-net-doctor/internal/diagnostics"
    "github.com/zebiner/docker-net-doctor/internal/docker"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestEngineCreation(t *testing.T) {
    // For testing, we'll create a real Docker client
    // In more sophisticated tests, you might use a mock
    ctx := context.Background()
    dockerClient, err := docker.NewClient(ctx)
    if err != nil {
        t.Skip("Skipping test - Docker not available:", err)
        return
    }
    defer dockerClient.Close()
    
    // Test creating an engine with nil config (should use defaults)
    engine := diagnostics.NewEngine(dockerClient, nil)
    assert.NotNil(t, engine, "Engine should not be nil with nil config")
    
    // Test with custom config
    config := &diagnostics.Config{
        Timeout:  60 * time.Second,
        Verbose:  true,
        Parallel: false,
    }
    engine = diagnostics.NewEngine(dockerClient, config)
    assert.NotNil(t, engine, "Engine should not be nil with custom config")
}

func TestEngineRun(t *testing.T) {
    ctx := context.Background()
    
    // Create Docker client for the test
    dockerClient, err := docker.NewClient(ctx)
    if err != nil {
        t.Skip("Skipping test - Docker not available:", err)
        return
    }
    defer dockerClient.Close()
    
    // Create engine with sequential execution for predictable test results
    config := &diagnostics.Config{
        Parallel: false,
        Timeout:  10 * time.Second,
    }
    engine := diagnostics.NewEngine(dockerClient, config)
    
    // Note: In the actual implementation, checks might be registered differently
    // For now, we'll just run the engine and see what happens
    results, err := engine.Run(ctx)
    require.NoError(t, err, "Engine.Run should not return an error")
    
    // The results should be a slice of CheckResult, not a Results struct
    // We need to adjust our assertions based on the actual return type
    assert.NotNil(t, results, "Results should not be nil")
}
