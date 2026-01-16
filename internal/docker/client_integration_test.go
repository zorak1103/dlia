//go:build integration

package docker

import (
	"context"
	"testing"
)

// TestNewClient_Integration tests client creation with a real Docker daemon.
// This test requires Docker to be running and is skipped in short mode.
// Run with: go test -tags=integration ./internal/docker/...
func TestNewClient_Integration(t *testing.T) {
	tests := []struct {
		name        string
		socketPath  string
		expectError bool
	}{
		{
			name:        "default client",
			socketPath:  "",
			expectError: false,
		},
		{
			name:        "custom socket path",
			socketPath:  "unix:///var/run/docker.sock",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.socketPath)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !tt.expectError && client == nil {
				t.Errorf("Expected client but got nil")
				return
			}

			if client != nil {
				ctx := context.Background()
				err := client.Ping(ctx)
				if err != nil {
					t.Logf("Docker daemon ping failed (expected in CI without Docker): %v", err)
					t.Skip("Skipping test - Docker daemon not available")
				}
			}
		})
	}
}
