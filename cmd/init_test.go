package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/zorak1103/dlia/internal/templates"
)

func TestInitCmd_Structure(t *testing.T) {
	t.Parallel()

	cmd := initCmd

	if cmd.Use != "init" {
		t.Errorf("Expected command use 'init', got '%s'", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("Expected command short description to be set")
	}

	if cmd.Long == "" {
		t.Error("Expected command long description to be set")
	}

	if cmd.Example == "" {
		t.Error("Expected command example to be set")
	}
}

func TestInitCmd_Flags(t *testing.T) {
	t.Parallel()

	cmd := initCmd
	flags := cmd.Flags()

	forceFlag := flags.Lookup("force")
	if forceFlag == nil {
		t.Error("Expected 'force' flag to be defined")
		return
	}

	if forceFlag.DefValue != "false" {
		t.Errorf("Expected 'force' flag default to be 'false', got '%s'", forceFlag.DefValue)
	}
}

func TestInitCmd_HelpOutput(t *testing.T) {
	var buf bytes.Buffer

	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"init", "--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("Expected no error executing help command, got: %v", err)
	}

	output := buf.String()

	expectedStrings := []string{
		"Init creates the necessary configuration files",
		"config.yaml",
		".env",
		"reports/",
		"knowledge_base/",
		"--force",
	}

	for _, expected := range expectedStrings {
		if !containsString(output, expected) {
			t.Errorf("Expected help output to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestInitCmd_CreatesDirectories(t *testing.T) {
	// Save current directory and change to temp dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("Failed to restore original directory: %v", err)
		}
	}()

	// Reset force flag
	force = false

	// Run init command
	err = initCmd.RunE(initCmd, []string{})
	if err != nil {
		t.Fatalf("initCmd.RunE() error = %v", err)
	}

	// Check directories were created
	expectedDirs := []string{
		"reports",
		"knowledge_base",
		filepath.Join("knowledge_base", "services"),
	}

	for _, dir := range expectedDirs {
		info, err := os.Stat(dir)
		if os.IsNotExist(err) {
			t.Errorf("Expected directory %s to be created", dir)
			continue
		}
		if err != nil {
			t.Errorf("Error checking directory %s: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("Expected %s to be a directory", dir)
		}
	}
}

func TestInitCmd_CreatesFiles(t *testing.T) {
	// Save current directory and change to temp dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("Failed to restore original directory: %v", err)
		}
	}()

	// Reset force flag
	force = false

	// Run init command
	err = initCmd.RunE(initCmd, []string{})
	if err != nil {
		t.Fatalf("initCmd.RunE() error = %v", err)
	}

	// Check files were created
	expectedFiles := []string{
		"config.yaml",
		".env",
		filepath.Join("knowledge_base", "global_summary.md"),
	}

	for _, file := range expectedFiles {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			t.Errorf("Expected file %s to be created", file)
		}
	}
}

func TestInitCmd_ConfigYAMLContent(t *testing.T) {
	// Save current directory and change to temp dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("Failed to restore original directory: %v", err)
		}
	}()

	// Reset force flag
	force = false

	// Run init command
	err = initCmd.RunE(initCmd, []string{})
	if err != nil {
		t.Fatalf("initCmd.RunE() error = %v", err)
	}

	// Read config.yaml
	content, err := os.ReadFile("config.yaml")
	if err != nil {
		t.Fatalf("Failed to read config.yaml: %v", err)
	}

	// Should match the embedded template
	if !bytes.Equal(content, templates.ConfigYAML) {
		t.Error("config.yaml content does not match embedded template")
	}
}

func TestInitCmd_EnvFileContent(t *testing.T) {
	// Save current directory and change to temp dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("Failed to restore original directory: %v", err)
		}
	}()

	// Reset force flag
	force = false

	// Run init command
	err = initCmd.RunE(initCmd, []string{})
	if err != nil {
		t.Fatalf("initCmd.RunE() error = %v", err)
	}

	// Read .env
	content, err := os.ReadFile(".env")
	if err != nil {
		t.Fatalf("Failed to read .env: %v", err)
	}

	// Should match the embedded template
	if !bytes.Equal(content, templates.EnvFile) {
		t.Error(".env content does not match embedded template")
	}
}

func TestInitCmd_GlobalSummaryContent(t *testing.T) {
	// Save current directory and change to temp dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("Failed to restore original directory: %v", err)
		}
	}()

	// Reset force flag
	force = false

	// Run init command
	err = initCmd.RunE(initCmd, []string{})
	if err != nil {
		t.Fatalf("initCmd.RunE() error = %v", err)
	}

	// Read global_summary.md
	content, err := os.ReadFile(filepath.Join("knowledge_base", "global_summary.md"))
	if err != nil {
		t.Fatalf("Failed to read global_summary.md: %v", err)
	}

	// Should contain expected content
	expectedStrings := []string{
		"# DLIA Global Summary",
		"knowledge base",
		"No scans performed yet",
	}

	for _, expected := range expectedStrings {
		if !containsString(string(content), expected) {
			t.Errorf("global_summary.md should contain %q", expected)
		}
	}
}

func TestInitCmd_SkipsExistingFiles(t *testing.T) {
	// Save current directory and change to temp dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("Failed to restore original directory: %v", err)
		}
	}()

	// Create an existing config.yaml with custom content
	existingContent := []byte("# My custom config\ntest: true\n")
	if err := os.WriteFile("config.yaml", existingContent, 0600); err != nil {
		t.Fatalf("Failed to create existing config.yaml: %v", err)
	}

	// Reset force flag to false (should skip existing files)
	force = false

	// Run init command
	err = initCmd.RunE(initCmd, []string{})
	if err != nil {
		t.Fatalf("initCmd.RunE() error = %v", err)
	}

	// Verify config.yaml was not overwritten
	content, err := os.ReadFile("config.yaml")
	if err != nil {
		t.Fatalf("Failed to read config.yaml: %v", err)
	}

	if !bytes.Equal(content, existingContent) {
		t.Error("config.yaml should not be overwritten without --force flag")
	}
}

func TestInitCmd_ForceOverwritesFiles(t *testing.T) {
	// Save current directory and change to temp dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("Failed to restore original directory: %v", err)
		}
	}()

	// Create an existing config.yaml with custom content
	existingContent := []byte("# My custom config\ntest: true\n")
	if err := os.WriteFile("config.yaml", existingContent, 0600); err != nil {
		t.Fatalf("Failed to create existing config.yaml: %v", err)
	}

	// Set force flag to true (should overwrite)
	force = true
	defer func() { force = false }() // Reset after test

	// Run init command
	err = initCmd.RunE(initCmd, []string{})
	if err != nil {
		t.Fatalf("initCmd.RunE() error = %v", err)
	}

	// Verify config.yaml was overwritten
	content, err := os.ReadFile("config.yaml")
	if err != nil {
		t.Fatalf("Failed to read config.yaml: %v", err)
	}

	if !bytes.Equal(content, templates.ConfigYAML) {
		t.Error("config.yaml should be overwritten with --force flag")
	}
}

func TestInitCmd_FilePermissions(t *testing.T) {
	// Skip on Windows as it doesn't support Unix-style file permissions
	if os.PathSeparator == '\\' {
		t.Skip("Skipping file permissions test on Windows")
	}

	// Save current directory and change to temp dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("Failed to restore original directory: %v", err)
		}
	}()

	// Reset force flag
	force = false

	// Run init command
	err = initCmd.RunE(initCmd, []string{})
	if err != nil {
		t.Fatalf("initCmd.RunE() error = %v", err)
	}

	// Check file permissions (should be 0600 - owner read/write only)
	filesToCheck := []string{"config.yaml", ".env"}

	for _, file := range filesToCheck {
		info, err := os.Stat(file)
		if err != nil {
			t.Errorf("Failed to stat %s: %v", file, err)
			continue
		}

		mode := info.Mode().Perm()
		if mode&0077 != 0 {
			t.Errorf("%s has insecure permissions: %o, expected 0600", file, mode)
		}
	}
}

func TestInitCmd_DirectoryPermissions(t *testing.T) {
	// Skip on Windows as it doesn't support Unix-style file permissions
	if os.PathSeparator == '\\' {
		t.Skip("Skipping directory permissions test on Windows")
	}

	// Save current directory and change to temp dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("Failed to restore original directory: %v", err)
		}
	}()

	// Reset force flag
	force = false

	// Run init command
	err = initCmd.RunE(initCmd, []string{})
	if err != nil {
		t.Fatalf("initCmd.RunE() error = %v", err)
	}

	// Check directory permissions (should be 0750)
	dirsToCheck := []string{"reports", "knowledge_base"}

	for _, dir := range dirsToCheck {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("Failed to stat %s: %v", dir, err)
			continue
		}

		mode := info.Mode().Perm()
		if mode&0027 != 0 {
			t.Errorf("%s has insecure permissions: %o, expected 0750", dir, mode)
		}
	}
}

func TestInitCmd_IdempotentDirectoryCreation(t *testing.T) {
	// Save current directory and change to temp dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("Failed to restore original directory: %v", err)
		}
	}()

	// Reset force flag
	force = false

	// Run init command twice - should not error on second run
	err = initCmd.RunE(initCmd, []string{})
	if err != nil {
		t.Fatalf("First initCmd.RunE() error = %v", err)
	}

	err = initCmd.RunE(initCmd, []string{})
	if err != nil {
		t.Fatalf("Second initCmd.RunE() error = %v (should be idempotent)", err)
	}

	// Verify directories still exist
	expectedDirs := []string{
		"reports",
		"knowledge_base",
		filepath.Join("knowledge_base", "services"),
	}

	for _, dir := range expectedDirs {
		info, err := os.Stat(dir)
		if os.IsNotExist(err) {
			t.Errorf("Expected directory %s to exist after second run", dir)
			continue
		}
		if !info.IsDir() {
			t.Errorf("Expected %s to be a directory", dir)
		}
	}
}
