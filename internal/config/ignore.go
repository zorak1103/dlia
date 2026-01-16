package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultIgnoreDir is the default directory for ignore instruction files
const DefaultIgnoreDir = "./config/ignore"

// GetIgnoreInstructions reads the ignore instructions for a specific container
// from {ignoreDir}/{containerName}.md
// If ignoreDir is empty, uses DefaultIgnoreDir
func GetIgnoreInstructions(containerName, ignoreDir string) (string, error) {
	// Use default if not specified
	if ignoreDir == "" {
		ignoreDir = DefaultIgnoreDir
	}

	// Sanitize container name for file path
	safeName := strings.ReplaceAll(containerName, "/", "_")

	path := filepath.Join(ignoreDir, safeName+".md")

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil // No instructions found, return empty string
	}

	content, err := os.ReadFile(path) // #nosec G304 -- path is constructed from sanitized container name and configured directory, not direct user input
	if err != nil {
		return "", fmt.Errorf("failed to read ignore instructions from %s for container '%s': %w", path, containerName, err)
	}

	// Display that ignore instructions were found and will be included
	fmt.Printf("📋 Ignore instructions found for container '%s' (from %s) - will be included in analysis request\n", containerName, path)

	return string(content), nil
}
