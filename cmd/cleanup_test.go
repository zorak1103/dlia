package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/zorak1103/dlia/internal/config"
)

func TestCleanupCmd_RequiresValidConfig(t *testing.T) {
	// Reset viper and cfg to test config requirement
	viper.Reset()
	originalCfg := cfg
	cfg = nil
	defer func() {
		cfg = originalCfg
		viper.Reset()
	}()

	// Test cleanup list command without config
	err := cleanupListCmd.RunE(cleanupListCmd, []string{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configuration not loaded")
	assert.Contains(t, err.Error(), "DLIA has not been initialized")
	assert.Contains(t, err.Error(), "Run 'dlia init'")
}

func TestCleanupCmd_RequiresDirectories(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create a config file
	configFile := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configFile, []byte("test: value"), 0600)
	assert.NoError(t, err)

	// Reset and configure viper
	viper.Reset()
	viper.SetConfigFile(configFile)
	err = viper.ReadInConfig()
	assert.NoError(t, err)

	// Save original config
	originalCfg := cfg
	defer func() {
		cfg = originalCfg
		viper.Reset()
	}()

	// Create config with missing directories (DI approach)
	cfg = &config.Config{
		ConfigFilePath: configFile, // Set via DI
		Output: config.OutputConfig{
			ReportsDir:       filepath.Join(tmpDir, "nonexistent_reports"),
			KnowledgeBaseDir: filepath.Join(tmpDir, "nonexistent_kb"),
			StateFile:        filepath.Join(tmpDir, "state.json"),
			LLMLogDir:        filepath.Join(tmpDir, "logs"),
			LLMLogEnabled:    false,
		},
		Docker: config.DockerConfig{
			SocketPath: "unix:///var/run/docker.sock",
		},
	}

	// Test cleanup list command with missing directories
	err = cleanupListCmd.RunE(cleanupListCmd, []string{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required directories are missing")
	assert.Contains(t, err.Error(), "Reports directory")
	assert.Contains(t, err.Error(), "Knowledge base directory")
	assert.Contains(t, err.Error(), "Run 'dlia init'")
}

func TestCleanupCmd_WorksWithValidConfig(t *testing.T) {
	// This test verifies that with a valid config, the command proceeds
	// past validation and fails at Docker connection (which is expected
	// in a test environment without Docker)

	tmpDir := t.TempDir()

	// Create required directories
	reportsDir := filepath.Join(tmpDir, "reports")
	kbDir := filepath.Join(tmpDir, "knowledge_base")

	err := os.MkdirAll(reportsDir, 0750)
	assert.NoError(t, err)
	err = os.MkdirAll(kbDir, 0750)
	assert.NoError(t, err)

	// Create config file
	configFile := filepath.Join(tmpDir, "config.yaml")
	err = os.WriteFile(configFile, []byte("test: value"), 0600)
	assert.NoError(t, err)

	// Reset and configure viper
	viper.Reset()
	viper.SetConfigFile(configFile)
	err = viper.ReadInConfig()
	assert.NoError(t, err)

	// Save original config
	originalCfg := cfg
	defer func() {
		cfg = originalCfg
		viper.Reset()
	}()

	// Create config with valid directories (DI approach)
	cfg = &config.Config{
		ConfigFilePath: configFile, // Set via DI
		Output: config.OutputConfig{
			ReportsDir:       reportsDir,
			KnowledgeBaseDir: kbDir,
			StateFile:        filepath.Join(tmpDir, "state.json"),
			LLMLogDir:        filepath.Join(tmpDir, "logs"),
			LLMLogEnabled:    false,
		},
		Docker: config.DockerConfig{
			SocketPath: "unix:///var/run/docker.sock",
		},
	}

	// Test cleanup list command with valid config
	// It should pass validation but fail at Docker connection
	err = cleanupListCmd.RunE(cleanupListCmd, []string{})

	// We expect an error here (Docker connection failure in test environment)
	// but it should NOT be a config validation error
	if err != nil {
		assert.NotContains(t, err.Error(), "configuration not loaded")
		assert.NotContains(t, err.Error(), "required directories are missing")
		// Should be Docker-related error
		assert.Contains(t, err.Error(), "Docker")
	}
}

func TestCleanupExecuteCmd_RequiresValidConfig(t *testing.T) {
	// Reset viper and cfg to test config requirement
	viper.Reset()
	originalCfg := cfg
	cfg = nil
	defer func() {
		cfg = originalCfg
		viper.Reset()
	}()

	// Test cleanup execute command without config
	err := cleanupExecuteCmd.RunE(cleanupExecuteCmd, []string{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configuration not loaded")
	assert.Contains(t, err.Error(), "DLIA has not been initialized")
	assert.Contains(t, err.Error(), "Run 'dlia init'")
}

func TestCleanupCmd_Structure(t *testing.T) {
	t.Parallel()

	// Test main cleanup command
	assert.Equal(t, "cleanup", cleanupCmd.Use)
	assert.NotEmpty(t, cleanupCmd.Short)
	assert.NotEmpty(t, cleanupCmd.Long)
	assert.Contains(t, cleanupCmd.Long, "dlia init")
	assert.NotEmpty(t, cleanupCmd.Example)

	// Test list subcommand
	assert.Equal(t, "list", cleanupListCmd.Use)
	assert.NotEmpty(t, cleanupListCmd.Short)
	assert.NotEmpty(t, cleanupListCmd.Long)

	// Test execute subcommand
	assert.Equal(t, "execute", cleanupExecuteCmd.Use)
	assert.NotEmpty(t, cleanupExecuteCmd.Short)
	assert.NotEmpty(t, cleanupExecuteCmd.Long)
}
