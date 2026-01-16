// Package config handles configuration loading and validation.
package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Common errors
var (
	Err = errors.New("config error")
)

// RegexpFilter represents regexp-based filtering configuration for a container
type RegexpFilter struct {
	Enabled  bool     `mapstructure:"enabled"`
	Patterns []string `mapstructure:"patterns"`
}

// Config represents the application configuration
type Config struct {
	LLM           LLMConfig               `mapstructure:"llm"`
	Docker        DockerConfig            `mapstructure:"docker"`
	Notification  NotificationConfig      `mapstructure:"notification"`
	Output        OutputConfig            `mapstructure:"output"`
	Privacy       PrivacyConfig           `mapstructure:"privacy"`
	Prompts       PromptsConfig           `mapstructure:"prompts"`
	RegexpFilters map[string]RegexpFilter `mapstructure:"regexp_filters"`

	// ConfigFilePath stores the path to the loaded config file (not marshaled from YAML)
	ConfigFilePath string `mapstructure:"-"`
}

// PromptsConfig contains paths to custom prompt templates
type PromptsConfig struct {
	SystemPrompt           string `mapstructure:"system_prompt"`
	AnalysisPrompt         string `mapstructure:"analysis_prompt"`
	ChunkSummaryPrompt     string `mapstructure:"chunk_summary_prompt"`
	SynthesisPrompt        string `mapstructure:"synthesis_prompt"`
	ExecutiveSummaryPrompt string `mapstructure:"executive_summary_prompt"`
}

// LLMConfig contains settings for the LLM API
type LLMConfig struct {
	BaseURL   string `mapstructure:"base_url"`
	APIKey    string `mapstructure:"api_key"`
	Model     string `mapstructure:"model"`
	MaxTokens int    `mapstructure:"max_tokens"`
}

// DockerConfig contains Docker-specific settings
type DockerConfig struct {
	SocketPath string `mapstructure:"socket_path"`
}

// NotificationConfig contains notification settings
type NotificationConfig struct {
	ShoutrrURL string `mapstructure:"shoutrrr_url"` // Shoutrrr URL format
	Enabled    bool   `mapstructure:"enabled"`
}

// OutputConfig contains output path settings
type OutputConfig struct {
	ReportsDir             string `mapstructure:"reports_dir"`
	KnowledgeBaseDir       string `mapstructure:"knowledge_base_dir"`
	StateFile              string `mapstructure:"state_file"`
	IgnoreDir              string `mapstructure:"ignore_dir"`
	LLMLogDir              string `mapstructure:"llm_log_dir"`
	LLMLogEnabled          bool   `mapstructure:"llm_log_enabled"`
	KnowledgeRetentionDays int    `mapstructure:"knowledge_retention_days"`
}

// PrivacyConfig contains privacy/anonymization settings
type PrivacyConfig struct {
	AnonymizeIPs     bool `mapstructure:"anonymize_ips"`
	AnonymizeSecrets bool `mapstructure:"anonymize_secrets"`
}

// autoDetectDockerSocket determines the Docker socket path based on environment and platform.
func autoDetectDockerSocket() string {
	if os.Getenv("DOCKER_HOST") != "" {
		return os.Getenv("DOCKER_HOST")
	}
	// Check for Unix socket
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		return "unix:///var/run/docker.sock"
	}
	// Default to Windows named pipe if Unix socket not found
	return "npipe:////./pipe/docker_engine"
}

// Load reads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	// Try to load .env file (ignore error if not exists)
	_ = godotenv.Load() // nolint:errcheck // .env file is optional

	v := viper.New()

	// Set config file path
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME/.config/dlia")
		v.AddConfigPath("/etc/dlia")
	}

	// Set defaults
	setDefaults(v)

	// Read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			configFile := v.ConfigFileUsed()
			if configFile == "" {
				configFile = configPath
			}
			return nil, fmt.Errorf("error reading config file from %s: %w", configFile, err)
		}
		// Config file not found; using defaults and env vars
	}

	// Environment variable support
	v.SetEnvPrefix("DLIA")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Unmarshal into config struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		configFile := v.ConfigFileUsed()
		if configFile == "" {
			configFile = "(using defaults and environment variables)"
		}
		return nil, fmt.Errorf("error unmarshaling config from %s: %w", configFile, err)
	}

	// Store the config file path in the struct (DI approach, no global state)
	cfg.ConfigFilePath = v.ConfigFileUsed()

	// Auto-detect Docker socket if not specified
	if cfg.Docker.SocketPath == "" {
		cfg.Docker.SocketPath = autoDetectDockerSocket()
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		configFile := v.ConfigFileUsed()
		if configFile == "" {
			configFile = "(using defaults and environment variables)"
		}
		return nil, fmt.Errorf("config validation failed for %s: %w", configFile, err)
	}

	return &cfg, nil
}

// LoadFromViper reads configuration from the global viper instance (for testing)
func LoadFromViper() (*Config, error) {
	// Set defaults first
	setDefaults(viper.GetViper())

	// Environment variable support
	viper.SetEnvPrefix("DLIA")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Unmarshal into config struct
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config from global viper instance: %w", err)
	}

	// Store the config file path (DI approach, even for testing)
	cfg.ConfigFilePath = viper.ConfigFileUsed()

	// Auto-detect Docker socket if not specified
	if cfg.Docker.SocketPath == "" {
		cfg.Docker.SocketPath = autoDetectDockerSocket()
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed for global viper instance: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// LLM defaults
	v.SetDefault("llm.base_url", "https://api.openai.com/v1")
	v.SetDefault("llm.model", "gpt-4o-mini")
	v.SetDefault("llm.max_tokens", 128000)
	v.SetDefault("llm.api_key", "") // Required for AutomaticEnv to work

	// Docker defaults
	if os.Getenv("DOCKER_HOST") != "" {
		v.SetDefault("docker.socket_path", os.Getenv("DOCKER_HOST"))
	} else {
		// Default Docker socket paths by platform
		if _, err := os.Stat("/var/run/docker.sock"); err == nil {
			v.SetDefault("docker.socket_path", "unix:///var/run/docker.sock")
		} else {
			v.SetDefault("docker.socket_path", "npipe:////./pipe/docker_engine")
		}
	}

	// Scheduler defaults

	// Notification defaults
	v.SetDefault("notification.shoutrrr_url", "") // Required for AutomaticEnv to work
	v.SetDefault("notification.enabled", false)

	// Output defaults
	v.SetDefault("output.reports_dir", "./reports")
	v.SetDefault("output.knowledge_base_dir", "./knowledge_base")
	v.SetDefault("output.state_file", "./state.json")
	v.SetDefault("output.ignore_dir", "./config/ignore")
	v.SetDefault("output.llm_log_dir", "./logs/llm")
	v.SetDefault("output.llm_log_enabled", false)
	v.SetDefault("output.knowledge_retention_days", 30)

	// Privacy defaults
	v.SetDefault("privacy.anonymize_ips", true)
	v.SetDefault("privacy.anonymize_secrets", true)

	// Prompts defaults (empty = use embedded defaults)
	v.SetDefault("prompts.system_prompt", "")
	v.SetDefault("prompts.analysis_prompt", "")
	v.SetDefault("prompts.chunk_summary_prompt", "")
	v.SetDefault("prompts.synthesis_prompt", "")
	v.SetDefault("prompts.executive_summary_prompt", "")

	// Regexp filters defaults (empty map = no filters)
	v.SetDefault("regexp_filters", map[string]RegexpFilter{})
}

// Validate ensures all required fields are set and values are within valid ranges.
func (c *Config) Validate() error {
	configSource := c.ConfigFilePath
	if configSource == "" {
		configSource = "(defaults/environment)"
	}

	if err := c.validateRequiredFields(configSource); err != nil {
		return err
	}

	if err := c.validateRanges(configSource); err != nil {
		return err
	}

	return c.validateRegexpFilters()
}

func (c *Config) validateRequiredFields(configSource string) error {
	requiredFields := []struct {
		value   string
		message string
	}{
		{c.LLM.BaseURL, "llm.base_url is required in config %s"},
		{c.LLM.APIKey, "llm.api_key is required in config %s (set DLIA_LLM_API_KEY environment variable)"},
		{c.LLM.Model, "llm.model is required in config %s"},
		{c.Docker.SocketPath, "docker.socket_path is required in config %s"},
		{c.Output.ReportsDir, "output.reports_dir is required in config %s"},
		{c.Output.KnowledgeBaseDir, "output.knowledge_base_dir is required in config %s"},
		{c.Output.StateFile, "output.state_file is required in config %s"},
	}

	for _, field := range requiredFields {
		if field.value == "" {
			return fmt.Errorf(field.message, configSource)
		}
	}
	return nil
}

func (c *Config) validateRanges(configSource string) error {
	if c.Output.KnowledgeRetentionDays < 1 || c.Output.KnowledgeRetentionDays > 365 {
		return fmt.Errorf("output.knowledge_retention_days must be between 1 and 365, got %d in config %s",
			c.Output.KnowledgeRetentionDays, configSource)
	}
	return nil
}

func (c *Config) validateRegexpFilters() error {
	for containerName, filter := range c.RegexpFilters {
		if !filter.Enabled {
			continue
		}
		for i, pattern := range filter.Patterns {
			if _, err := regexp.Compile(pattern); err != nil {
				return fmt.Errorf("invalid regexp pattern in regexp_filters[%s].patterns[%d]: %s: %w",
					containerName, i, pattern, err)
			}
		}
	}
	return nil
}
