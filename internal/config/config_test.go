package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestLoad_EnvVars(t *testing.T) {
	// Set environment variables
	os.Setenv("DLIA_LLM_API_KEY", "test-api-key") // nolint:errcheck,gosec
	os.Setenv("DLIA_LLM_MODEL", "test-model")     // nolint:errcheck,gosec
	defer os.Unsetenv("DLIA_LLM_API_KEY")         // nolint:errcheck
	defer os.Unsetenv("DLIA_LLM_MODEL")           // nolint:errcheck

	// Load config (empty path to force default/env loading)
	cfg, err := Load("")
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Verify values from env vars
	assert.Equal(t, "test-api-key", cfg.LLM.APIKey)
	assert.Equal(t, "test-model", cfg.LLM.Model)
}

func TestLoad_Defaults(t *testing.T) {
	// Set minimal required env vars
	os.Setenv("DLIA_LLM_API_KEY", "test-key") // nolint:errcheck,gosec
	defer os.Unsetenv("DLIA_LLM_API_KEY")     // nolint:errcheck

	cfg, err := Load("")
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Check defaults
	assert.Equal(t, "https://api.openai.com/v1", cfg.LLM.BaseURL)
	assert.Equal(t, "gpt-4o-mini", cfg.LLM.Model)
	assert.Equal(t, 128000, cfg.LLM.MaxTokens)
	assert.Equal(t, "./reports", cfg.Output.ReportsDir)
	assert.Equal(t, "./knowledge_base", cfg.Output.KnowledgeBaseDir)
	assert.Equal(t, "./state.json", cfg.Output.StateFile)
	assert.Equal(t, 30, cfg.Output.KnowledgeRetentionDays)
	assert.True(t, cfg.Privacy.AnonymizeIPs)
	assert.True(t, cfg.Privacy.AnonymizeSecrets)
	assert.False(t, cfg.Notification.Enabled)
}

func TestLoad_ConfigFile(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `llm:
  api_key: file-api-key
  model: file-model
  base_url: https://test.example.com
  max_tokens: 50000
docker:
  socket_path: unix:///test/docker.sock
notification:
  enabled: true
  shoutrrr_url: generic://test
output:
  reports_dir: /test/reports
  knowledge_base_dir: /test/kb
  state_file: /test/state.json
privacy:
  anonymize_ips: false
  anonymize_secrets: false
`
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	assert.NoError(t, err)

	cfg, err := Load(configPath)
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Verify config from file
	assert.Equal(t, "file-api-key", cfg.LLM.APIKey)
	assert.Equal(t, "file-model", cfg.LLM.Model)
	assert.Equal(t, "https://test.example.com", cfg.LLM.BaseURL)
	assert.Equal(t, 50000, cfg.LLM.MaxTokens)
	assert.Equal(t, "unix:///test/docker.sock", cfg.Docker.SocketPath)
	assert.True(t, cfg.Notification.Enabled)
	assert.Equal(t, "generic://test", cfg.Notification.ShoutrrURL)
	assert.Equal(t, "/test/reports", cfg.Output.ReportsDir)
	assert.Equal(t, "/test/kb", cfg.Output.KnowledgeBaseDir)
	assert.Equal(t, "/test/state.json", cfg.Output.StateFile)
	assert.False(t, cfg.Privacy.AnonymizeIPs)
	assert.False(t, cfg.Privacy.AnonymizeSecrets)
}

func TestLoad_InvalidConfigFile(t *testing.T) {
	// Try to load non-existent config file with specific path
	_, err := Load("/nonexistent/path/config.yaml")
	assert.Error(t, err)
}

func TestLoad_MalformedConfigFile(t *testing.T) {
	// Create temp malformed config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `llm:
  api_key: test
  invalid yaml content [[[
`
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	assert.NoError(t, err)

	_, err = Load(configPath)
	assert.Error(t, err)
}

func TestValidate_MissingBaseURL(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			BaseURL: "",
			APIKey:  "test",
			Model:   "test",
		},
		Docker: DockerConfig{SocketPath: "test"},
		Output: OutputConfig{
			ReportsDir:       "test",
			KnowledgeBaseDir: "test",
			StateFile:        "test",
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "llm.base_url")
}

func TestValidate_MissingAPIKey(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			BaseURL: "https://test.com",
			APIKey:  "",
			Model:   "test",
		},
		Docker: DockerConfig{SocketPath: "test"},
		Output: OutputConfig{
			ReportsDir:       "test",
			KnowledgeBaseDir: "test",
			StateFile:        "test",
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "llm.api_key")
}

func TestValidate_MissingModel(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			BaseURL: "https://test.com",
			APIKey:  "test",
			Model:   "",
		},
		Docker: DockerConfig{SocketPath: "test"},
		Output: OutputConfig{
			ReportsDir:       "test",
			KnowledgeBaseDir: "test",
			StateFile:        "test",
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "llm.model")
}

func TestValidate_MissingDockerSocket(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			BaseURL: "https://test.com",
			APIKey:  "test",
			Model:   "test",
		},
		Docker: DockerConfig{SocketPath: ""},
		Output: OutputConfig{
			ReportsDir:       "test",
			KnowledgeBaseDir: "test",
			StateFile:        "test",
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "docker.socket_path")
}

func TestValidate_MissingReportsDir(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			BaseURL: "https://test.com",
			APIKey:  "test",
			Model:   "test",
		},
		Docker: DockerConfig{SocketPath: "test"},
		Output: OutputConfig{
			ReportsDir:       "",
			KnowledgeBaseDir: "test",
			StateFile:        "test",
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "output.reports_dir")
}

func TestValidate_MissingKnowledgeBaseDir(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			BaseURL: "https://test.com",
			APIKey:  "test",
			Model:   "test",
		},
		Docker: DockerConfig{SocketPath: "test"},
		Output: OutputConfig{
			ReportsDir:       "test",
			KnowledgeBaseDir: "",
			StateFile:        "test",
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "output.knowledge_base_dir")
}

func TestValidate_MissingStateFile(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			BaseURL: "https://test.com",
			APIKey:  "test",
			Model:   "test",
		},
		Docker: DockerConfig{SocketPath: "test"},
		Output: OutputConfig{
			ReportsDir:       "test",
			KnowledgeBaseDir: "test",
			StateFile:        "",
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "output.state_file")
}

func TestValidate_InvalidRetentionDaysTooLow(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			BaseURL: "https://test.com",
			APIKey:  "test",
			Model:   "test",
		},
		Docker: DockerConfig{SocketPath: "test"},
		Output: OutputConfig{
			ReportsDir:             "test",
			KnowledgeBaseDir:       "test",
			StateFile:              "test",
			KnowledgeRetentionDays: 0,
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "output.knowledge_retention_days")
	assert.Contains(t, err.Error(), "between 1 and 365")
}

func TestValidate_InvalidRetentionDaysTooHigh(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			BaseURL: "https://test.com",
			APIKey:  "test",
			Model:   "test",
		},
		Docker: DockerConfig{SocketPath: "test"},
		Output: OutputConfig{
			ReportsDir:             "test",
			KnowledgeBaseDir:       "test",
			StateFile:              "test",
			KnowledgeRetentionDays: 366,
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "output.knowledge_retention_days")
	assert.Contains(t, err.Error(), "between 1 and 365")
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			BaseURL: "https://test.com",
			APIKey:  "test",
			Model:   "test",
		},
		Docker: DockerConfig{SocketPath: "test"},
		Output: OutputConfig{
			ReportsDir:             "test",
			KnowledgeBaseDir:       "test",
			StateFile:              "test",
			KnowledgeRetentionDays: 30,
		},
	}

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestLoad_DockerHostEnvVar(t *testing.T) {
	// Set DOCKER_HOST env var
	os.Setenv("DOCKER_HOST", "tcp://test-host:2375") // nolint:errcheck,gosec
	os.Setenv("DLIA_LLM_API_KEY", "test-key")        // nolint:errcheck,gosec
	defer os.Unsetenv("DOCKER_HOST")                 // nolint:errcheck
	defer os.Unsetenv("DLIA_LLM_API_KEY")            // nolint:errcheck
	defer os.Unsetenv("DLIA_DOCKER_SOCKET_PATH")     // nolint:errcheck

	cfg, err := Load("")
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Should use DOCKER_HOST value
	assert.Equal(t, "tcp://test-host:2375", cfg.Docker.SocketPath)
}

func TestLoadFromViper(t *testing.T) {
	// Reset viper state
	viper.Reset()

	// Set environment variables
	os.Setenv("DLIA_LLM_API_KEY", "viper-key") // nolint:errcheck,gosec
	os.Setenv("DLIA_LLM_MODEL", "viper-model") // nolint:errcheck,gosec
	defer os.Unsetenv("DLIA_LLM_API_KEY")      // nolint:errcheck
	defer os.Unsetenv("DLIA_LLM_MODEL")        // nolint:errcheck

	cfg, err := LoadFromViper()
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Verify values
	assert.Equal(t, "viper-key", cfg.LLM.APIKey)
	assert.Equal(t, "viper-model", cfg.LLM.Model)
}

func TestLoad_PromptsConfig(t *testing.T) {
	// Create temp config file with custom prompts
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `llm:
  api_key: test-key
  model: test-model
prompts:
  system_prompt: /custom/system.md
  analysis_prompt: /custom/analysis.md
  chunk_summary_prompt: /custom/chunk.md
  synthesis_prompt: /custom/synthesis.md
  executive_summary_prompt: /custom/executive.md
docker:
  socket_path: unix:///var/run/docker.sock
output:
  reports_dir: ./reports
  knowledge_base_dir: ./kb
  state_file: ./state.json
`
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	assert.NoError(t, err)

	cfg, err := Load(configPath)
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Verify custom prompts
	assert.Equal(t, "/custom/system.md", cfg.Prompts.SystemPrompt)
	assert.Equal(t, "/custom/analysis.md", cfg.Prompts.AnalysisPrompt)
	assert.Equal(t, "/custom/chunk.md", cfg.Prompts.ChunkSummaryPrompt)
	assert.Equal(t, "/custom/synthesis.md", cfg.Prompts.SynthesisPrompt)
	assert.Equal(t, "/custom/executive.md", cfg.Prompts.ExecutiveSummaryPrompt)
}

func TestLoad_EmptyPromptsUseDefaults(t *testing.T) {
	os.Setenv("DLIA_LLM_API_KEY", "test-key") // nolint:errcheck,gosec
	defer os.Unsetenv("DLIA_LLM_API_KEY")     // nolint:errcheck

	cfg, err := Load("")
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Empty prompts should use embedded defaults
	assert.Equal(t, "", cfg.Prompts.SystemPrompt)
	assert.Equal(t, "", cfg.Prompts.AnalysisPrompt)
	assert.Equal(t, "", cfg.Prompts.ChunkSummaryPrompt)
	assert.Equal(t, "", cfg.Prompts.SynthesisPrompt)
	assert.Equal(t, "", cfg.Prompts.ExecutiveSummaryPrompt)
}

func TestLoad_NotificationConfig(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `llm:
  api_key: test-key
  model: test-model
notification:
  enabled: true
  shoutrrr_url: discord://token@id
docker:
  socket_path: unix:///var/run/docker.sock
output:
  reports_dir: ./reports
  knowledge_base_dir: ./kb
  state_file: ./state.json
`
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	assert.NoError(t, err)

	cfg, err := Load(configPath)
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	assert.True(t, cfg.Notification.Enabled)
	assert.Equal(t, "discord://token@id", cfg.Notification.ShoutrrURL)
}

func TestErr_ErrorVariable(t *testing.T) {
	assert.NotNil(t, Err)
	assert.Equal(t, "config error", Err.Error())
}
