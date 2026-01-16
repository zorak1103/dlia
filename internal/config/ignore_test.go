package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetIgnoreInstructions_FileExists(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	ignoreDir := filepath.Join(tmpDir, "config", "ignore")
	err := os.MkdirAll(ignoreDir, 0750)
	require.NoError(t, err)

	// Create ignore file
	ignoreContent := "# Test Ignore Instructions\n\nIgnore DEBUG messages"
	err = os.WriteFile(filepath.Join(ignoreDir, "test-container.md"), []byte(ignoreContent), 0600)
	require.NoError(t, err)

	// Change to temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(originalWd)
	}()

	// Test - use empty string to use default path, but we're in tmpDir so use the actual ignoreDir
	instructions, err := GetIgnoreInstructions("test-container", ignoreDir)
	assert.NoError(t, err)
	assert.Equal(t, ignoreContent, instructions)
}

func TestGetIgnoreInstructions_FileNotFound(t *testing.T) {
	// Create temp directory structure without any files
	tmpDir := t.TempDir()

	// Change to temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(originalWd)
	}()

	// Test - should return empty string when file doesn't exist
	instructions, err := GetIgnoreInstructions("nonexistent-container", "./config/ignore")
	assert.NoError(t, err)
	assert.Empty(t, instructions)
}

func TestGetIgnoreInstructions_SanitizesContainerName(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	ignoreDir := filepath.Join(tmpDir, "config", "ignore")
	err := os.MkdirAll(ignoreDir, 0750)
	require.NoError(t, err)

	// Create ignore file with sanitized name
	ignoreContent := "Instructions for container with slashes"
	err = os.WriteFile(filepath.Join(ignoreDir, "my_project_container.md"), []byte(ignoreContent), 0600)
	require.NoError(t, err)

	// Change to temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(originalWd)
	}()

	// Test - container name with slashes should be sanitized
	instructions, err := GetIgnoreInstructions("my/project/container", ignoreDir)
	assert.NoError(t, err)
	assert.Equal(t, ignoreContent, instructions)
}

func TestGetIgnoreInstructions_MultipleSlashes(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	ignoreDir := filepath.Join(tmpDir, "config", "ignore")
	err := os.MkdirAll(ignoreDir, 0750)
	require.NoError(t, err)

	// Create ignore file with sanitized name
	ignoreContent := "Instructions for deeply nested container"
	err = os.WriteFile(filepath.Join(ignoreDir, "registry_org_project_app.md"), []byte(ignoreContent), 0600)
	require.NoError(t, err)

	// Change to temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(originalWd)
	}()

	// Test - registry-style container name
	instructions, err := GetIgnoreInstructions("registry/org/project/app", ignoreDir)
	assert.NoError(t, err)
	assert.Equal(t, ignoreContent, instructions)
}

func TestGetIgnoreInstructions_EmptyFile(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	ignoreDir := filepath.Join(tmpDir, "config", "ignore")
	err := os.MkdirAll(ignoreDir, 0750)
	require.NoError(t, err)

	// Create empty ignore file
	err = os.WriteFile(filepath.Join(ignoreDir, "empty-container.md"), []byte(""), 0600)
	require.NoError(t, err)

	// Change to temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(originalWd)
	}()

	// Test - should return empty string for empty file
	instructions, err := GetIgnoreInstructions("empty-container", ignoreDir)
	assert.NoError(t, err)
	assert.Empty(t, instructions)
}

func TestGetIgnoreInstructions_SpecialCharactersInName(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	ignoreDir := filepath.Join(tmpDir, "config", "ignore")
	err := os.MkdirAll(ignoreDir, 0750)
	require.NoError(t, err)

	// Create ignore file with name containing underscores (already safe)
	ignoreContent := "Instructions for container with underscores"
	err = os.WriteFile(filepath.Join(ignoreDir, "my_container_v1.md"), []byte(ignoreContent), 0600)
	require.NoError(t, err)

	// Change to temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(originalWd)
	}()

	// Test - container name with underscores should work
	instructions, err := GetIgnoreInstructions("my_container_v1", ignoreDir)
	assert.NoError(t, err)
	assert.Equal(t, ignoreContent, instructions)
}

func TestGetIgnoreInstructions_DirectoryExistsButEmpty(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	ignoreDir := filepath.Join(tmpDir, "config", "ignore")
	err := os.MkdirAll(ignoreDir, 0750)
	require.NoError(t, err)

	// Change to temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(originalWd)
	}()

	// Test - directory exists but no file for this container
	instructions, err := GetIgnoreInstructions("missing-container", ignoreDir)
	assert.NoError(t, err)
	assert.Empty(t, instructions)
}

func TestGetIgnoreInstructions_LargeFile(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	ignoreDir := filepath.Join(tmpDir, "config", "ignore")
	err := os.MkdirAll(ignoreDir, 0750)
	require.NoError(t, err)

	// Create large ignore file
	largeContent := "# Large Ignore Instructions\n\n"
	for i := 0; i < 100; i++ {
		largeContent += "- Rule " + string(rune('A'+i%26)) + ": Ignore this pattern\n"
	}
	err = os.WriteFile(filepath.Join(ignoreDir, "large-container.md"), []byte(largeContent), 0600)
	require.NoError(t, err)

	// Change to temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(originalWd)
	}()

	// Test - should handle large files
	instructions, err := GetIgnoreInstructions("large-container", ignoreDir)
	assert.NoError(t, err)
	assert.Equal(t, largeContent, instructions)
}

func TestGetIgnoreInstructions_UnicodeContent(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	ignoreDir := filepath.Join(tmpDir, "config", "ignore")
	err := os.MkdirAll(ignoreDir, 0750)
	require.NoError(t, err)

	// Create ignore file with unicode content
	unicodeContent := "# Ignore Instructions 📋\n\n- Ignorar mensajes DEBUG\n- 忽略调试消息"
	err = os.WriteFile(filepath.Join(ignoreDir, "unicode-container.md"), []byte(unicodeContent), 0600)
	require.NoError(t, err)

	// Change to temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(originalWd)
	}()

	// Test - should handle unicode content
	instructions, err := GetIgnoreInstructions("unicode-container", ignoreDir)
	assert.NoError(t, err)
	assert.Equal(t, unicodeContent, instructions)
}

func TestGetIgnoreInstructions_EmptyDirUsesDefault(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	defaultDir := filepath.Join(tmpDir, "config", "ignore")
	err := os.MkdirAll(defaultDir, 0750)
	require.NoError(t, err)

	// Create ignore file in default location
	ignoreContent := "Default location instructions"
	err = os.WriteFile(filepath.Join(defaultDir, "default-test.md"), []byte(ignoreContent), 0600)
	require.NoError(t, err)

	// Change to temp directory
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(originalWd)
	}()

	// Test - empty ignoreDir should use DefaultIgnoreDir
	instructions, err := GetIgnoreInstructions("default-test", "")
	assert.NoError(t, err)
	assert.Equal(t, ignoreContent, instructions)
}

func TestGetIgnoreInstructions_CustomDir(t *testing.T) {
	// Create temp directory with custom ignore location
	tmpDir := t.TempDir()
	customDir := filepath.Join(tmpDir, "custom", "ignores")
	err := os.MkdirAll(customDir, 0750)
	require.NoError(t, err)

	// Create ignore file in custom location
	ignoreContent := "Custom location instructions"
	err = os.WriteFile(filepath.Join(customDir, "custom-container.md"), []byte(ignoreContent), 0600)
	require.NoError(t, err)

	// Test - should use custom ignoreDir
	instructions, err := GetIgnoreInstructions("custom-container", customDir)
	assert.NoError(t, err)
	assert.Equal(t, ignoreContent, instructions)
}

func TestDefaultIgnoreDir(t *testing.T) {
	// Verify the default constant
	assert.Equal(t, "./config/ignore", DefaultIgnoreDir)
}
