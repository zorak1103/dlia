package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/zorak1103/dlia/internal/config"
)

func TestMaskAPIKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty key",
			input:    "",
			expected: "❌ Not set",
		},
		{
			name:     "short key (less than 8 chars)",
			input:    "abc",
			expected: "***",
		},
		{
			name:     "exactly 8 chars",
			input:    "12345678",
			expected: "***",
		},
		{
			name:     "9 chars key",
			input:    "123456789",
			expected: "1234*6789",
		},
		{
			name:     "typical API key",
			input:    "sk-abcdefghij1234567890",
			expected: "sk-a***************7890",
		},
		{
			name:     "long API key",
			input:    "sk-proj-abcdefghijklmnopqrstuvwxyz1234567890",
			expected: "sk-p************************************7890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := maskAPIKey(tt.input)
			if result != tt.expected {
				t.Errorf("maskAPIKey(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMaskAPIKey_PreservesFirstAndLastFourChars(t *testing.T) {
	t.Parallel()

	key := "abcd1234567890wxyz"
	result := maskAPIKey(key)

	// Should start with first 4 chars
	if result[:4] != "abcd" {
		t.Errorf("maskAPIKey() should preserve first 4 chars, got prefix: %s", result[:4])
	}

	// Should end with last 4 chars
	if result[len(result)-4:] != "wxyz" {
		t.Errorf("maskAPIKey() should preserve last 4 chars, got suffix: %s", result[len(result)-4:])
	}
}

func TestMaskShoutrrrURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty URL",
			input:    "",
			expected: "❌ Not configured",
		},
		{
			name:     "discord URL",
			input:    "discord://token@channel",
			expected: "✅ Configured (discord://***)",
		},
		{
			name:     "slack URL",
			input:    "slack://token-a/token-b/token-c",
			expected: "✅ Configured (slack://***)",
		},
		{
			name:     "smtp URL",
			input:    "smtp://user:password@smtp.example.com:587/?auth=plain",
			expected: "✅ Configured (smtp://***)",
		},
		{
			name:     "pushover URL",
			input:    "pushover://shoutrrr:token@user",
			expected: "✅ Configured (pushover://***)",
		},
		{
			name:     "telegram URL",
			input:    "telegram://token@telegram?chats=@channel",
			expected: "✅ Configured (telegram://***)",
		},
		{
			name:     "gotify URL",
			input:    "gotify://gotify.example.com/token",
			expected: "✅ Configured (gotify://***)",
		},
		{
			name:     "invalid format (no ://)",
			input:    "invalid-url-format",
			expected: "✅ Configured (invalid format)",
		},
		{
			name:     "URL with only protocol",
			input:    "http://",
			expected: "✅ Configured (http://***)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := maskShoutrrrURL(tt.input)
			if result != tt.expected {
				t.Errorf("maskShoutrrrURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestConfigCmd_Structure(t *testing.T) {
	t.Parallel()

	cmd := configCmd

	if cmd.Use != "config" {
		t.Errorf("Expected command use 'config', got '%s'", cmd.Use)
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

func TestConfigCmd_HelpOutput(t *testing.T) {
	var buf bytes.Buffer

	// Create a fresh instance of the root command with config as subcommand
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"config", "--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("Expected no error executing help command, got: %v", err)
	}

	output := buf.String()

	// Verify help contains expected content
	expectedStrings := []string{
		"Display the effective configuration",
		"Default values",
		"Configuration file",
		"Environment variables",
		"dlia config",
	}

	for _, expected := range expectedStrings {
		if !containsString(output, expected) {
			t.Errorf("Expected help output to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestConfigCmd_RequiresConfig(t *testing.T) {
	// Reset viper and cfg to test config requirement
	viper.Reset()
	originalCfg := cfg
	cfg = nil
	defer func() { cfg = originalCfg }()

	var buf bytes.Buffer
	cmd := configCmd
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})

	if err == nil {
		t.Error("Expected error when config is nil")
	}

	expectedError := "configuration not loaded\n\nTo get started, run: dlia init"
	if err.Error() != expectedError {
		t.Errorf("Expected %q error, got: %v", expectedError, err)
	}
}

// Helper function to check if string contains substring
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestMaskAPIKey_ConsistentLength(t *testing.T) {
	t.Parallel()

	// For keys longer than 8 chars, the masked output should have the same length
	testKeys := []string{
		"123456789",
		"1234567890",
		"sk-12345678901234567890",
		"sk-proj-123456789012345678901234567890",
	}

	for _, key := range testKeys {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			result := maskAPIKey(key)
			if len(result) != len(key) {
				t.Errorf("maskAPIKey(%q) length = %d, want %d (same as input)", key, len(result), len(key))
			}
		})
	}
}

func TestMaskShoutrrrURL_ExtractsServiceType(t *testing.T) {
	t.Parallel()

	// Test that various service types are correctly extracted and displayed
	services := []struct {
		url         string
		serviceType string
	}{
		{"discord://token@channel", "discord"},
		{"slack://token", "slack"},
		{"smtp://user:pass@host", "smtp"},
		{"pushover://token@user", "pushover"},
		{"telegram://token@telegram", "telegram"},
		{"gotify://host/token", "gotify"},
		{"teams://group@tenant/altid/groupowner", "teams"},
		{"matrix://user:pass@host", "matrix"},
	}

	for _, svc := range services {
		t.Run(svc.serviceType, func(t *testing.T) {
			t.Parallel()

			result := maskShoutrrrURL(svc.url)
			expectedContains := svc.serviceType + "://"

			if !containsString(result, expectedContains) {
				t.Errorf("maskShoutrrrURL(%q) = %q, should contain %q", svc.url, result, expectedContains)
			}
		})
	}
}

func TestDisplayPromptPaths_WithDefaults(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Prompts: config.PromptsConfig{
			SystemPrompt:           "",
			AnalysisPrompt:         "",
			ChunkSummaryPrompt:     "",
			SynthesisPrompt:        "",
			ExecutiveSummaryPrompt: "",
		},
	}

	// This function prints to stdout
	// We just verify it doesn't panic
	displayPromptPaths(cfg)
}

func TestDisplayPromptPaths_WithCustomPaths(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Prompts: config.PromptsConfig{
			SystemPrompt:           "/path/to/system.md",
			AnalysisPrompt:         "/path/to/analysis.md",
			ChunkSummaryPrompt:     "/path/to/chunk.md",
			SynthesisPrompt:        "/path/to/synthesis.md",
			ExecutiveSummaryPrompt: "/path/to/executive.md",
		},
	}

	// This function prints to stdout
	// We just verify it doesn't panic
	displayPromptPaths(cfg)
}

func TestDisplayPromptPaths_MixedConfiguration(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Prompts: config.PromptsConfig{
			SystemPrompt:           "/custom/system.md",
			AnalysisPrompt:         "",
			ChunkSummaryPrompt:     "/custom/chunk.md",
			SynthesisPrompt:        "",
			ExecutiveSummaryPrompt: "",
		},
	}

	// This function prints to stdout
	// We just verify it doesn't panic with mixed custom/default config
	displayPromptPaths(cfg)
}

func TestValidateConfigOrExit_NilConfig(t *testing.T) {
	t.Parallel()

	err := validateConfigOrExit(nil, "test")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configuration not loaded")
	assert.Contains(t, err.Error(), "DLIA has not been initialized")
	assert.Contains(t, err.Error(), "Run 'dlia init'")
}

func TestValidateConfigOrExit_NoConfigFile(t *testing.T) {
	// Create a temporary directory with required directories
	tmpDir := t.TempDir()

	// Create required directories so validation passes directory checks
	reportsDir := filepath.Join(tmpDir, "reports")
	kbDir := filepath.Join(tmpDir, "knowledge_base")

	err := os.MkdirAll(reportsDir, 0750)
	assert.NoError(t, err)
	err = os.MkdirAll(kbDir, 0750)
	assert.NoError(t, err)

	// Create a config with existing directories but no ConfigFilePath (DI approach)
	cfg := &config.Config{
		ConfigFilePath: "", // Empty = no config file
		Output: config.OutputConfig{
			ReportsDir:       reportsDir,
			KnowledgeBaseDir: kbDir,
			StateFile:        "./state.json",
			LLMLogDir:        filepath.Join(tmpDir, "logs"),
			LLMLogEnabled:    false,
		},
	}

	err = validateConfigOrExit(cfg, "test")

	// Should error about missing config file
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no configuration file found")
	assert.Contains(t, err.Error(), "Run 'dlia init'")
}

func TestValidateConfigOrExit_MissingDirectories(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create a temporary config file
	configFile := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configFile, []byte("test: value"), 0600)
	assert.NoError(t, err)

	// Create config pointing to non-existent directories (DI approach)
	cfg := &config.Config{
		ConfigFilePath: configFile, // Set via DI
		Output: config.OutputConfig{
			ReportsDir:       filepath.Join(tmpDir, "nonexistent_reports"),
			KnowledgeBaseDir: filepath.Join(tmpDir, "nonexistent_kb"),
			StateFile:        filepath.Join(tmpDir, "state", "state.json"),
			LLMLogDir:        filepath.Join(tmpDir, "nonexistent_logs"),
			LLMLogEnabled:    true, // Enable to trigger LLM log dir check
		},
	}

	err = validateConfigOrExit(cfg, "test")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required directories are missing")
	assert.Contains(t, err.Error(), "Reports directory")
	assert.Contains(t, err.Error(), "Knowledge base directory")
	assert.Contains(t, err.Error(), "LLM log directory")
	assert.Contains(t, err.Error(), "Run 'dlia init'")
}

func TestValidateConfigOrExit_ValidConfig(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()

	// Create required directories
	reportsDir := filepath.Join(tmpDir, "reports")
	kbDir := filepath.Join(tmpDir, "knowledge_base")
	stateDir := filepath.Join(tmpDir, "state")

	err := os.MkdirAll(reportsDir, 0750)
	assert.NoError(t, err)
	err = os.MkdirAll(kbDir, 0750)
	assert.NoError(t, err)
	err = os.MkdirAll(stateDir, 0750)
	assert.NoError(t, err)

	// Create a temporary config file
	configFile := filepath.Join(tmpDir, "config.yaml")
	err = os.WriteFile(configFile, []byte("test: value"), 0600)
	assert.NoError(t, err)

	// Create config with valid directories (DI approach)
	cfg := &config.Config{
		ConfigFilePath: configFile, // Set via DI
		Output: config.OutputConfig{
			ReportsDir:       reportsDir,
			KnowledgeBaseDir: kbDir,
			StateFile:        filepath.Join(stateDir, "state.json"),
			LLMLogDir:        filepath.Join(tmpDir, "logs"),
			LLMLogEnabled:    false, // Disabled, so no need to check
		},
	}

	err = validateConfigOrExit(cfg, "test")

	// Should NOT return an error
	assert.NoError(t, err)
}

func TestValidateConfigOrExit_LLMLogDisabled(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()

	// Create required directories (except LLM log dir)
	reportsDir := filepath.Join(tmpDir, "reports")
	kbDir := filepath.Join(tmpDir, "knowledge_base")

	err := os.MkdirAll(reportsDir, 0750)
	assert.NoError(t, err)
	err = os.MkdirAll(kbDir, 0750)
	assert.NoError(t, err)

	// Create a temporary config file
	configFile := filepath.Join(tmpDir, "config.yaml")
	err = os.WriteFile(configFile, []byte("test: value"), 0600)
	assert.NoError(t, err)

	// Create config with LLM logging disabled and nonexistent LLM log dir (DI approach)
	cfg := &config.Config{
		ConfigFilePath: configFile, // Set via DI
		Output: config.OutputConfig{
			ReportsDir:       reportsDir,
			KnowledgeBaseDir: kbDir,
			StateFile:        filepath.Join(tmpDir, "state.json"), // State file in tmpDir (exists)
			LLMLogDir:        filepath.Join(tmpDir, "nonexistent_llm_logs"),
			LLMLogEnabled:    false, // Disabled - should NOT check this directory
		},
	}

	err = validateConfigOrExit(cfg, "test")

	// Should NOT return an error because LLM logging is disabled
	assert.NoError(t, err)
}

func TestValidateConfigOrExit_StateDirInCurrentDir(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()

	// Create required directories
	reportsDir := filepath.Join(tmpDir, "reports")
	kbDir := filepath.Join(tmpDir, "knowledge_base")

	err := os.MkdirAll(reportsDir, 0750)
	assert.NoError(t, err)
	err = os.MkdirAll(kbDir, 0750)
	assert.NoError(t, err)

	// Create a temporary config file
	configFile := filepath.Join(tmpDir, "config.yaml")
	err = os.WriteFile(configFile, []byte("test: value"), 0600)
	assert.NoError(t, err)

	// Create config with state file in current directory (.) (DI approach)
	cfg := &config.Config{
		ConfigFilePath: configFile, // Set via DI
		Output: config.OutputConfig{
			ReportsDir:       reportsDir,
			KnowledgeBaseDir: kbDir,
			StateFile:        "./state.json", // Current dir - should not check
			LLMLogDir:        filepath.Join(tmpDir, "logs"),
			LLMLogEnabled:    false,
		},
	}

	err = validateConfigOrExit(cfg, "test")

	// Should NOT error about state dir when it's in current directory
	assert.NoError(t, err)
}

func TestConfigCmd_OutputsKnowledgeRetentionDays(t *testing.T) {
	// Arrange: Create a config with a specific knowledge retention value
	tmpDir := t.TempDir()

	// Create required directories
	reportsDir := filepath.Join(tmpDir, "reports")
	kbDir := filepath.Join(tmpDir, "knowledge_base")

	err := os.MkdirAll(reportsDir, 0750)
	assert.NoError(t, err)
	err = os.MkdirAll(kbDir, 0750)
	assert.NoError(t, err)

	// Create a temporary config file
	configFile := filepath.Join(tmpDir, "config.yaml")
	err = os.WriteFile(configFile, []byte("test: value"), 0600)
	assert.NoError(t, err)

	testCfg := &config.Config{
		ConfigFilePath: configFile,
		LLM: config.LLMConfig{
			BaseURL:   "https://api.example.com/v1",
			APIKey:    "sk-test-key-1234567890",
			Model:     "gpt-4",
			MaxTokens: 8000,
		},
		Docker: config.DockerConfig{
			SocketPath: "unix:///var/run/docker.sock",
		},
		Notification: config.NotificationConfig{
			Enabled:    false,
			ShoutrrURL: "",
		},
		Output: config.OutputConfig{
			ReportsDir:             reportsDir,
			KnowledgeBaseDir:       kbDir,
			StateFile:              filepath.Join(tmpDir, "state.json"),
			KnowledgeRetentionDays: 45, // Test value
		},
		Privacy: config.PrivacyConfig{
			AnonymizeIPs:     true,
			AnonymizeSecrets: true,
		},
		Prompts: config.PromptsConfig{},
	}

	// Temporarily set the global config
	originalCfg := cfg
	cfg = testCfg
	defer func() { cfg = originalCfg }()

	// Act: Capture stdout by redirecting it temporarily
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Execute the config command
	err = configCmd.RunE(configCmd, []string{})
	assert.NoError(t, err)

	// Restore stdout and read captured output
	err = w.Close()
	assert.NoError(t, err)
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Assert: Check that knowledge_retention_days is in the output
	assert.Contains(t, output, "Knowledge Retention:", "Output should contain 'Knowledge Retention:' label")
	assert.Contains(t, output, "45 days", "Output should contain the configured value '45 days'")
}
