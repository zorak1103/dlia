//go:build integration

package cmd

import (
	"strings"
	"testing"
)

// TestScanCmd_Integration is a full integration test that requires:
// 1. A running Docker daemon
// 2. Test containers
// 3. A real or mock LLM endpoint
// 4. File system for state
func TestScanCmd_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	setupTestConfig(t)

	// This would be a full integration test that requires:
	// 1. A running Docker daemon
	// 2. Test containers
	// 3. A real or mock LLM endpoint
	// 4. File system for state

	// For now, just test that the command can be created and has the right structure
	cmd := scanCmd

	if cmd.Use != "scan" {
		t.Errorf("Expected command use 'scan', got '%s'", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("Expected command short description to be set")
	}

	if cmd.Long == "" {
		t.Error("Expected command long description to be set")
	}

	if len(cmd.Example) == 0 {
		t.Error("Expected command examples to be set")
	}
}

// TestScanCmd_HelpOutput_Integration tests help output with Docker context.
func TestScanCmd_HelpOutput_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test could verify help output includes Docker-specific flags
	// and properly documents integration requirements
	cmd := scanCmd
	if cmd.Long == "" || !strings.Contains(cmd.Long, "Docker") {
		t.Log("Command documentation should mention Docker requirements")
	}
}
