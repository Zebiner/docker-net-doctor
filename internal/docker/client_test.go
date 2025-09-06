package docker_test

import (
    "context"
    "testing"
    "time"
    
    "github.com/zebiner/docker-net-doctor/internal/docker"
)

func TestNewClient(t *testing.T) {
    // Create a context with timeout for the test
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    // Try to create a Docker client
    client, err := docker.NewClient(ctx)
    
    // Note: This test will only pass if Docker is actually running
    // In a CI environment, you might want to skip this test if Docker isn't available
    if err != nil {
        t.Skipf("Skipping test - Docker not available: %v", err)
        return
    }
    defer client.Close()
    
    // If we got here, the client was created successfully
    t.Log("Successfully created Docker client")
}
