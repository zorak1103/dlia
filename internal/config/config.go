// Package config handles configuration loading and validation.
package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"github.com/zorak1103/dlia/internal/severity"
)

// Common errors
var (
	Err = errors.New("config error")

	// ErrNoConfigFile marks a load error that happened without any config file
	// (defaults and environment variables only). Check it with errors.Is.
	ErrNoConfigFile = errors.New("no config file found")
)

// noConfigFileError keeps the original message and cause of a load error while
// matching ErrNoConfigFile.
type noConfigFileError struct{ err error }

func (e *noConfigFileError) Error() string        { return e.err.Error() }
func (e *noConfigFileError) Unwrap() error        { return e.err }
func (e *noConfigFileError) Is(target error) bool { return target == ErrNoConfigFile }

// markNoConfigFile tags err with ErrNoConfigFile when no config file was used.
func markNoConfigFile(err error, configFileUsed string) error {
	if configFileUsed != "" {
		return err
	}
	return &noConfigFileError{err: err}
}

const (
	// DefaultContextWindow is the assumed model context window when none is configured.
	DefaultContextWindow = 128000
	// MinContextWindow is (ResponseReserveTokens + SystemPromptReserveTokens) / 0.8 — keeps the budget positive with the 80% estimate margin.
	MinContextWindow = 5625
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
	Scan          ScanConfig              `mapstructure:"scan"`
	RegexpFilters map[string]RegexpFilter `mapstructure:"regexp_filters"`

	// ConfigFilePath stores the path to the loaded config file (not marshaled from YAML)
	ConfigFilePath string `mapstructure:"-"`

	// Warnings collects non-fatal configuration notices (e.g. deprecated keys)
	Warnings []string `mapstructure:"-"`
}

// ScanConfig contains settings for the log scan window
type ScanConfig struct {
	MaxWindow time.Duration `mapstructure:"max_window"`
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
	BaseURL               string `mapstructure:"base_url"`
	APIKey                string `mapstructure:"api_key"`
	Model                 string `mapstructure:"model"`
	ContextWindow         int    `mapstructure:"context_window"`
	MaxChunksPerContainer int    `mapstructure:"max_chunks_per_container"`
	// MaxTokens is a deprecated alias for ContextWindow.
	MaxTokens int `mapstructure:"max_tokens"`

	// contextWindowFromAlias records that ContextWindow was taken from MaxTokens.
	contextWindowFromAlias bool
}

// DockerConfig contains Docker-specific settings
type DockerConfig struct {
	SocketPath string `mapstructure:"socket_path"`
}

// NotificationConfig contains notification settings
type NotificationConfig struct {
	ShoutrrURL  string `mapstructure:"shoutrrr_url"` // Shoutrrr URL format
	Enabled     bool   `mapstructure:"enabled"`
	MinSeverity string `mapstructure:"min_severity"`
}

// Threshold parses MinSeverity and returns the corresponding severity.Level.
// Falls back to severity.Warning when MinSeverity is empty or invalid.
func (n NotificationConfig) Threshold() severity.Level {
	level, err := severity.ParseThreshold(n.MinSeverity)
	if err != nil {
		return severity.Warning
	}
	return level
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

	resolveContextWindow(&cfg)

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
		return nil, markNoConfigFile(fmt.Errorf("config validation failed for %s: %w", configFile, err), v.ConfigFileUsed())
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

	resolveContextWindow(&cfg)

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
	v.SetDefault("llm.max_chunks_per_container", 10)
	v.SetDefault("llm.api_key", "") // Required for AutomaticEnv to work
	// No defaults: "unset" must stay 0 so resolveContextWindow can detect the deprecated alias.
	_ = v.BindEnv("llm.context_window") // nolint:errcheck // only errors without a key
	_ = v.BindEnv("llm.max_tokens")     // nolint:errcheck // only errors without a key

	// Scan defaults
	v.SetDefault("scan.max_window", "24h")

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
	v.SetDefault("notification.min_severity", "warning")

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

// resolveContextWindow maps the deprecated llm.max_tokens onto llm.context_window
// and falls back to DefaultContextWindow when neither is set.
func resolveContextWindow(cfg *Config) {
	if cfg.LLM.MaxTokens > 0 {
		msg := "llm.max_tokens is deprecated; use llm.context_window instead"
		if cfg.LLM.ContextWindow > 0 {
			msg += " (ignored because llm.context_window is set)"
		} else {
			cfg.LLM.ContextWindow = cfg.LLM.MaxTokens
			cfg.LLM.contextWindowFromAlias = true
		}
		cfg.Warnings = append(cfg.Warnings, msg)
	}
	if cfg.LLM.ContextWindow == 0 {
		cfg.LLM.ContextWindow = DefaultContextWindow
	}
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
	if c.Scan.MaxWindow < time.Minute {
		return fmt.Errorf("scan.max_window must be at least 1m, got %v in config %s (use a quoted duration like \"24h\"; a bare number is read as nanoseconds)",
			c.Scan.MaxWindow, configSource)
	}
	if c.LLM.MaxChunksPerContainer < 1 {
		return fmt.Errorf("llm.max_chunks_per_container must be at least 1, got %d in config %s",
			c.LLM.MaxChunksPerContainer, configSource)
	}
	if c.LLM.ContextWindow < MinContextWindow {
		name := "llm.context_window"
		if c.LLM.contextWindowFromAlias {
			name = fmt.Sprintf("llm.context_window (set via deprecated llm.max_tokens=%d)", c.LLM.MaxTokens)
		}
		return fmt.Errorf("%s must be at least %d, got %d in config %s",
			name, MinContextWindow, c.LLM.ContextWindow, configSource)
	}
	if _, err := severity.ParseThreshold(c.Notification.MinSeverity); err != nil {
		return fmt.Errorf("notification.min_severity must be one of ok, warning, critical, got %q in config %s",
			c.Notification.MinSeverity, configSource)
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
