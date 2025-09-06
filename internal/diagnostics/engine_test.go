package diagnostics_test

import (
    "context"
    "testing"
    "time"
    
    "github.com/zebiner/docker-net-doctor/internal/diagnostics"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestEngineCreation(t *testing.T) {
    // Test that we can create an engine with default config
    engine := diagnostics.NewEngine(nil)
    assert.NotNil(t, engine, "Engine should not be nil with nil config")
    
    // Test with custom config
    config := &diagnostics.Config{
        Timeout:  60 * time.Second,
        Verbose:  true,
        Parallel: false,
    }
    engine = diagnostics.NewEngine(config)
    assert.NotNil(t, engine, "Engine should not be nil with custom config")
}

func TestEngineRun(t *testing.T) {
    engine := diagnostics.NewEngine(nil)
    ctx := context.Background()
    
    results, err := engine.Run(ctx)
    require.NoError(t, err, "Engine.Run should not return an error")
    assert.NotEmpty(t, results, "Results should not be empty")
    
    // Verify we got at least one successful result
    if len(results) > 0 {
        assert.True(t, results[0].Success, "First result should be successful")
    }
}
