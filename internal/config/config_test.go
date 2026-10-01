package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Equal(t, 128000, cfg.LLM.ContextWindow)
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
  context_window: 50000
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
	assert.Equal(t, 50000, cfg.LLM.ContextWindow)
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
		Scan: ScanConfig{MaxWindow: 24 * time.Hour},
		LLM: LLMConfig{
			BaseURL:               "https://test.com",
			APIKey:                "test",
			Model:                 "test",
			ContextWindow:         DefaultContextWindow,
			MaxChunksPerContainer: 10,
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

func TestValidate_InvalidRegexpPattern(t *testing.T) {
	cfg := &Config{
		Scan:   ScanConfig{MaxWindow: 24 * time.Hour},
		LLM:    LLMConfig{BaseURL: "https://test.com", APIKey: "test", Model: "test", ContextWindow: DefaultContextWindow, MaxChunksPerContainer: 10},
		Docker: DockerConfig{SocketPath: "test"},
		Output: OutputConfig{
			ReportsDir:             "test",
			KnowledgeBaseDir:       "test",
			StateFile:              "test",
			KnowledgeRetentionDays: 30,
		},
		RegexpFilters: map[string]RegexpFilter{
			"mycontainer": {
				Enabled:  true,
				Patterns: []string{"[invalid"},
			},
		},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid regexp pattern")
}

func TestValidate_DisabledRegexpFilter_NotValidated(t *testing.T) {
	cfg := &Config{
		Scan:   ScanConfig{MaxWindow: 24 * time.Hour},
		LLM:    LLMConfig{BaseURL: "https://test.com", APIKey: "test", Model: "test", ContextWindow: DefaultContextWindow, MaxChunksPerContainer: 10},
		Docker: DockerConfig{SocketPath: "test"},
		Output: OutputConfig{
			ReportsDir:             "test",
			KnowledgeBaseDir:       "test",
			StateFile:              "test",
			KnowledgeRetentionDays: 30,
		},
		RegexpFilters: map[string]RegexpFilter{
			"mycontainer": {
				Enabled:  false,
				Patterns: []string{"[invalid"},
			},
		},
	}

	// Disabled filters should not be validated
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestAutoDetectDockerSocket(t *testing.T) {
	// ponytail: line 92 (unix socket found) is only reachable on hosts where
	// /var/run/docker.sock exists; gremlins runs Linux in a container without
	// the socket mounted, so that branch stays NOT COVERED there by design.
	tests := []struct {
		name        string
		dockerHost  string
		setDhEnvVar bool
	}{
		{
			name:        "DOCKER_HOST set wins",
			dockerHost:  "tcp://proxy:2375",
			setDhEnvVar: true,
		},
		{
			name:        "DOCKER_HOST empty falls through to detection",
			dockerHost:  "",
			setDhEnvVar: true,
		},
		{
			name:        "DOCKER_HOST unset falls through to detection",
			setDhEnvVar: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setDhEnvVar {
				t.Setenv("DOCKER_HOST", tt.dockerHost)
			} else {
				// os.LookupEnv distinguishes unset from empty, but the code
				// only checks Getenv != ""; make it deterministic either way.
				t.Setenv("DOCKER_HOST", "")
			}

			got := autoDetectDockerSocket()

			if tt.setDhEnvVar && tt.dockerHost != "" {
				assert.Equal(t, tt.dockerHost, got)
				return
			}

			// No DOCKER_HOST: expectation depends on platform + socket presence.
			if runtime.GOOS == "windows" {
				assert.Equal(t, "npipe:////./pipe/docker_engine", got)
				return
			}
			if _, err := os.Stat("/var/run/docker.sock"); err == nil {
				assert.Equal(t, "unix:///var/run/docker.sock", got)
			} else {
				assert.Equal(t, "npipe:////./pipe/docker_engine", got)
			}
		})
	}
}

func TestLoad_UnmarshalErrorWithoutConfigFile(t *testing.T) {
	// ponytail: forces the Unmarshal error branch of Load with no config file
	// used, pinning the "(using defaults and environment variables)" fallback
	// in the error message.
	t.Setenv("DLIA_LLM_MAX_TOKENS", "not-a-number")

	_, err := Load("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error unmarshaling config")
	assert.Contains(t, err.Error(), "(using defaults and environment variables)")
}

func TestLoad_ValidationErrorWithoutConfigFile(t *testing.T) {
	// ponytail: forces the Validate error branch of Load with no config file
	// used; an empty API key fails validation, pinning the same fallback text
	// in the validation error message.
	t.Setenv("DLIA_LLM_API_KEY", "")

	_, err := Load("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config validation failed")
	assert.Contains(t, err.Error(), "(using defaults and environment variables)")
}

// TestValidate_RetentionBoundaryValues pins the inclusive retention bounds:
// 1 and 365 are valid, 0 and 366 are not.
func TestValidate_RetentionBoundaryValues(t *testing.T) {
	base := func(retention int) *Config {
		return &Config{
			Scan:   ScanConfig{MaxWindow: 24 * time.Hour},
			LLM:    LLMConfig{BaseURL: "https://test.com", APIKey: "test", Model: "test", ContextWindow: DefaultContextWindow, MaxChunksPerContainer: 10},
			Docker: DockerConfig{SocketPath: "test"},
			Output: OutputConfig{
				ReportsDir:             "test",
				KnowledgeBaseDir:       "test",
				StateFile:              "test",
				KnowledgeRetentionDays: retention,
			},
		}
	}

	assert.NoError(t, base(1).Validate(), "retention 1 is the inclusive lower bound")
	assert.NoError(t, base(365).Validate(), "retention 365 is the inclusive upper bound")
	assert.Error(t, base(0).Validate(), "retention 0 is invalid")
	assert.Error(t, base(366).Validate(), "retention 366 is invalid")
}

// TestValidate_ConfigSourceInErrorMessage pins that validation errors name
// the config file when one is set, and the defaults placeholder when not.
func TestValidate_ConfigSourceInErrorMessage(t *testing.T) {
	cfg := &Config{
		ConfigFilePath: "myconfig.yaml",
		LLM:            LLMConfig{BaseURL: "https://test.com", APIKey: "test", Model: "test"},
		Docker:         DockerConfig{SocketPath: "test"},
		Output: OutputConfig{
			ReportsDir:             "test",
			KnowledgeBaseDir:       "test",
			StateFile:              "test",
			KnowledgeRetentionDays: 500,
		},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "myconfig.yaml")

	cfg.ConfigFilePath = ""
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "(defaults/environment)")
}

// TestLoadFromViper_PreservesConfiguredSocketPath pins that an explicitly
// configured docker.socket_path survives LoadFromViper instead of being
// replaced by the auto-detection.
//
// Uses the global viper instance (established pattern, see TestLoadFromViper);
// tests in this package must not run in parallel.
func TestLoadFromViper_PreservesConfiguredSocketPath(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("llm.api_key", "test-key")
	viper.Set("llm.model", "test-model")
	viper.Set("llm.base_url", "http://test")
	viper.Set("docker.socket_path", "unix:///test")
	viper.Set("output.reports_dir", "test")
	viper.Set("output.knowledge_base_dir", "test")
	viper.Set("output.state_file", "test")

	cfg, err := LoadFromViper()
	require.NoError(t, err)
	assert.Equal(t, "unix:///test", cfg.Docker.SocketPath)
}

// TestSetDefaults_PlatformSocketDefault pins the platform socket default:
// the unix socket path when /var/run/docker.sock exists, the Windows named
// pipe otherwise. Deliberately close to TestAutoDetectDockerSocket's
// both-ways guard — different entry point (direct setDefaults call), and
// this is the assertion that kills the operator-inversion mutant.
func TestSetDefaults_PlatformSocketDefault(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")

	v := viper.New()
	setDefaults(v)

	got := v.GetString("docker.socket_path")
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		assert.Equal(t, "unix:///var/run/docker.sock", got)
	} else {
		assert.Equal(t, "npipe:////./pipe/docker_engine", got)
	}
}

func TestLoad_ReliabilityDefaults(t *testing.T) {
	t.Setenv("DLIA_LLM_API_KEY", "test-key")
	t.Setenv("DLIA_LLM_CONTEXT_WINDOW", "")
	t.Setenv("DLIA_LLM_MAX_TOKENS", "")

	cfg, err := Load("")
	require.NoError(t, err)

	assert.Equal(t, 24*time.Hour, cfg.Scan.MaxWindow)
	assert.Equal(t, 10, cfg.LLM.MaxChunksPerContainer)
	assert.Equal(t, 128000, cfg.LLM.ContextWindow)
	assert.Empty(t, cfg.Warnings)
}

func TestLoad_ContextWindowAlias(t *testing.T) {
	const deprecated = "llm.max_tokens is deprecated"

	tests := []struct {
		name         string
		yaml         string
		env          map[string]string
		wantWindow   int
		wantWarnings int
		wantIgnored  bool
	}{
		{
			name:         "yaml max_tokens only",
			yaml:         "  max_tokens: 50000\n",
			wantWindow:   50000,
			wantWarnings: 1,
		},
		{
			name:         "env max_tokens only",
			env:          map[string]string{"DLIA_LLM_MAX_TOKENS": "50000"},
			wantWindow:   50000,
			wantWarnings: 1,
		},
		{
			name:         "yaml both",
			yaml:         "  context_window: 64000\n  max_tokens: 50000\n",
			wantWindow:   64000,
			wantWarnings: 1,
			wantIgnored:  true,
		},
		{
			name:         "env context_window and yaml max_tokens",
			yaml:         "  max_tokens: 50000\n",
			env:          map[string]string{"DLIA_LLM_CONTEXT_WINDOW": "64000"},
			wantWindow:   64000,
			wantWarnings: 1,
			wantIgnored:  true,
		},
		{
			name:       "context_window only",
			yaml:       "  context_window: 64000\n",
			wantWindow: 64000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DLIA_LLM_API_KEY", "test-key")
			t.Setenv("DLIA_LLM_CONTEXT_WINDOW", "")
			t.Setenv("DLIA_LLM_MAX_TOKENS", "")
			for k, val := range tt.env {
				t.Setenv(k, val)
			}

			configPath := filepath.Join(t.TempDir(), "config.yaml")
			content := "llm:\n  api_key: test-key\n" + tt.yaml
			require.NoError(t, os.WriteFile(configPath, []byte(content), 0600))

			cfg, err := Load(configPath)
			require.NoError(t, err)

			assert.Equal(t, tt.wantWindow, cfg.LLM.ContextWindow)
			require.Len(t, cfg.Warnings, tt.wantWarnings)
			if tt.wantWarnings == 0 {
				return
			}
			assert.Contains(t, cfg.Warnings[0], deprecated)
			assert.Contains(t, cfg.Warnings[0], "use llm.context_window instead")
			assert.Equal(t, tt.wantIgnored, strings.Contains(cfg.Warnings[0], "ignored because llm.context_window is set"))
		})
	}
}

func TestLoad_ScanMaxWindowFromEnv(t *testing.T) {
	t.Setenv("DLIA_LLM_API_KEY", "test-key")
	t.Setenv("DLIA_SCAN_MAX_WINDOW", "6h")

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, 6*time.Hour, cfg.Scan.MaxWindow)
}

func TestLoad_ScanMaxWindowFromYAML(t *testing.T) {
	t.Setenv("DLIA_LLM_API_KEY", "test-key")
	t.Setenv("DLIA_SCAN_MAX_WINDOW", "")

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("scan:\n  max_window: \"90m\"\n"), 0600))

	cfg, err := Load(configPath)
	require.NoError(t, err)
	assert.Equal(t, 90*time.Minute, cfg.Scan.MaxWindow)
}

func TestLoadFromViper_ResolvesContextWindow(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DLIA_LLM_CONTEXT_WINDOW", "")
	t.Setenv("DLIA_LLM_MAX_TOKENS", "")

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("llm.api_key", "test-key")
	viper.Set("llm.max_tokens", 40000)

	cfg, err := LoadFromViper()
	require.NoError(t, err)
	assert.Equal(t, 40000, cfg.LLM.ContextWindow)
	require.Len(t, cfg.Warnings, 1)
	assert.Contains(t, cfg.Warnings[0], "llm.max_tokens is deprecated")
}

func TestLoadFromViper_SmallAliasErrorMentionsMaxTokens(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DLIA_LLM_CONTEXT_WINDOW", "")
	t.Setenv("DLIA_LLM_MAX_TOKENS", "")

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("llm.api_key", "test-key")
	viper.Set("llm.max_tokens", 4000)

	_, err := LoadFromViper()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "llm.context_window (set via deprecated llm.max_tokens=4000) must be at least 5625, got 4000")
}

func TestLoadFromViper_SmallContextWindowErrorHasNoAliasHint(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DLIA_LLM_CONTEXT_WINDOW", "")
	t.Setenv("DLIA_LLM_MAX_TOKENS", "")

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("llm.api_key", "test-key")
	viper.Set("llm.context_window", 4000)

	_, err := LoadFromViper()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "llm.context_window must be at least 5625, got 4000")
	assert.NotContains(t, err.Error(), "deprecated")
}

func TestValidate_ReliabilityRanges(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *Config)
		wantErr string
	}{
		{"max_window zero", func(c *Config) { c.Scan.MaxWindow = 0 }, "scan.max_window"},
		{"max_window negative", func(c *Config) { c.Scan.MaxWindow = -time.Hour }, "scan.max_window"},
		{"max_chunks zero", func(c *Config) { c.LLM.MaxChunksPerContainer = 0 }, "llm.max_chunks_per_container"},
		{"context_window below minimum", func(c *Config) { c.LLM.ContextWindow = MinContextWindow - 1 }, "llm.context_window"},
		{"context_window at minimum", func(c *Config) { c.LLM.ContextWindow = MinContextWindow }, ""},
		{"max_chunks one", func(c *Config) { c.LLM.MaxChunksPerContainer = 1 }, ""},
		{"max_window bare number 3600 (3.6us)", func(c *Config) { c.Scan.MaxWindow = 3600 }, `use a quoted duration like "24h"`},
		{"max_window one nanosecond", func(c *Config) { c.Scan.MaxWindow = time.Nanosecond }, "scan.max_window"},
		{"max_window just below one minute", func(c *Config) { c.Scan.MaxWindow = time.Minute - 1 }, "scan.max_window"},
		{"max_window one minute", func(c *Config) { c.Scan.MaxWindow = time.Minute }, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validReliabilityConfig()
			tt.mutate(cfg)

			err := cfg.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func validReliabilityConfig() *Config {
	return &Config{
		Scan:   ScanConfig{MaxWindow: 24 * time.Hour},
		LLM:    LLMConfig{BaseURL: "https://test.com", APIKey: "test", Model: "test", ContextWindow: DefaultContextWindow, MaxChunksPerContainer: 10},
		Docker: DockerConfig{SocketPath: "test"},
		Output: OutputConfig{
			ReportsDir:             "test",
			KnowledgeBaseDir:       "test",
			StateFile:              "test",
			KnowledgeRetentionDays: 30,
		},
	}
}
