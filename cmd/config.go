// Package cmd implements the CLI commands.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/prompts"
)

// validateConfigOrExit validates that the configuration is properly initialized
// and all required directories exist. Returns a user-friendly error if validation fails.
func validateConfigOrExit(cfg *config.Config, _ string) error {
	// Check if config was loaded
	if cfg == nil {
		return fmt.Errorf("configuration not loaded\n\nDLIA has not been initialized in this directory.\nRun 'dlia init' to set up DLIA and create the necessary configuration")
	}

	// Check if config file exists (using DI approach - config file path stored in Config struct)
	if cfg.ConfigFilePath == "" {
		return fmt.Errorf("no configuration file found\n\nDLIA requires a configuration file to run.\nRun 'dlia init' to create config.yaml in the current directory")
	}

	// Validate required directories exist
	var missingDirs []string

	// Check reports directory
	if _, err := os.Stat(cfg.Output.ReportsDir); os.IsNotExist(err) {
		missingDirs = append(missingDirs, fmt.Sprintf("Reports directory: %s", cfg.Output.ReportsDir))
	}

	// Check knowledge base directory
	if _, err := os.Stat(cfg.Output.KnowledgeBaseDir); os.IsNotExist(err) {
		missingDirs = append(missingDirs, fmt.Sprintf("Knowledge base directory: %s", cfg.Output.KnowledgeBaseDir))
	}

	// Check state file parent directory
	stateDir := filepath.Dir(cfg.Output.StateFile)
	if stateDir != "." && stateDir != "" {
		if _, err := os.Stat(stateDir); os.IsNotExist(err) {
			missingDirs = append(missingDirs, fmt.Sprintf("State file directory: %s", stateDir))
		}
	}

	// Check LLM log directory (only if logging is enabled)
	if cfg.Output.LLMLogEnabled {
		if _, err := os.Stat(cfg.Output.LLMLogDir); os.IsNotExist(err) {
			missingDirs = append(missingDirs, fmt.Sprintf("LLM log directory: %s", cfg.Output.LLMLogDir))
		}
	}

	// If directories are missing, return helpful error
	if len(missingDirs) > 0 {
		errMsg := "required directories are missing:\n\n"
		for _, dir := range missingDirs {
			errMsg += fmt.Sprintf("  - %s\n", dir)
		}
		errMsg += "\nRun 'dlia init' to create the required directory structure"
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Display the effective configuration",
	Long: `Display the effective configuration that DLIA will use at runtime.

This shows the merged configuration from:
  1. Default values
  2. Configuration file (config.yaml)
  3. Environment variables (highest priority)

Sensitive values like API keys are masked for security.`,
	Example: `  # Show current configuration
  dlia config

  # Show with custom config file
  dlia config --config /etc/dlia/config.yaml`,
	RunE: func(_ *cobra.Command, _ []string) error {
		cfg := GetConfig()
		if cfg == nil {
			return fmt.Errorf("configuration not loaded\n\nTo get started, run: dlia init")
		}

		fmt.Println("=== DLIA Effective Configuration ===")
		fmt.Println()

		// LLM Configuration
		fmt.Println("🤖 LLM Configuration:")
		fmt.Printf("   Base URL:       %s\n", cfg.LLM.BaseURL)
		fmt.Printf("   Model:          %s\n", cfg.LLM.Model)
		fmt.Printf("   Max Tokens:     %d\n", cfg.LLM.MaxTokens)
		fmt.Printf("   API Key:        %s\n", maskAPIKey(cfg.LLM.APIKey))
		fmt.Println()

		// Docker Configuration
		fmt.Println("🐳 Docker Configuration:")
		fmt.Printf("   Socket Path:    %s\n", cfg.Docker.SocketPath)
		fmt.Println()

		// Notification Configuration
		fmt.Println("🔔 Notification Configuration:")
		fmt.Printf("   Enabled:        %v\n", cfg.Notification.Enabled)
		fmt.Printf("   Shoutrrr URL:   %s\n", maskShoutrrrURL(cfg.Notification.ShoutrrURL))
		fmt.Println()

		// Output Configuration
		fmt.Println("📁 Output Configuration:")
		fmt.Printf("   Reports Dir:    %s\n", cfg.Output.ReportsDir)
		fmt.Printf("   KB Dir:         %s\n", cfg.Output.KnowledgeBaseDir)
		fmt.Printf("   State File:     %s\n", cfg.Output.StateFile)
		fmt.Printf("   Knowledge Retention: %d days\n", cfg.Output.KnowledgeRetentionDays)
		fmt.Println()

		// Privacy Configuration
		fmt.Println("🔒 Privacy Configuration:")
		fmt.Printf("   Anonymize IPs:  %v\n", cfg.Privacy.AnonymizeIPs)
		fmt.Printf("   Anonymize Keys: %v\n", cfg.Privacy.AnonymizeSecrets)
		fmt.Println()

		// Prompts Configuration (Phase 8)
		fmt.Println("📝 Prompts Configuration:")
		displayPromptPaths(cfg)
		fmt.Println()

		return nil
	},
}

// nolint:gochecknoinits // Standard Cobra pattern for command registration
func init() {
	rootCmd.AddCommand(configCmd)
}

// maskAPIKey obscures API keys for secure display in config output.
// Shows first 4 and last 4 characters (e.g., "sk-1***abc2") to allow key identification
// without exposing the full secret. This 4/4 split follows OpenAI's display convention
// and provides reasonable balance between security and usability.
func maskAPIKey(key string) string {
	if key == "" {
		return "❌ Not set"
	}
	if len(key) <= 8 {
		return "***"
	}
	// Show first 4 and last 4 characters
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

// maskShoutrrrURL masks sensitive parts of Shoutrrr URL
func maskShoutrrrURL(url string) string {
	if url == "" {
		return "❌ Not configured"
	}

	// Extract service type (e.g., discord://, slack://, smtp://)
	parts := strings.SplitN(url, "://", 2)
	if len(parts) != 2 {
		return "✅ Configured (invalid format)"
	}

	service := parts[0]
	// Mask the credentials/tokens
	return fmt.Sprintf("✅ Configured (%s://***)", service)
}

// displayPromptPaths shows the configured prompt paths and their sources
func displayPromptPaths(cfg *config.Config) {
	// Initialize prompts to trigger loading
	prompts.InitPrompts(cfg)

	promptConfigs := []struct {
		name           string
		configuredPath string
	}{
		{"System Prompt", cfg.Prompts.SystemPrompt},
		{"Analysis Prompt", cfg.Prompts.AnalysisPrompt},
		{"Chunk Summary Prompt", cfg.Prompts.ChunkSummaryPrompt},
		{"Synthesis Prompt", cfg.Prompts.SynthesisPrompt},
		{"Executive Summary Prompt", cfg.Prompts.ExecutiveSummaryPrompt},
	}

	for _, pc := range promptConfigs {
		if pc.configuredPath != "" {
			fmt.Printf("   %-25s [EXTERNAL] %s\n", pc.name+":", pc.configuredPath)
		} else {
			fmt.Printf("   %-25s [INTERNAL DEFAULT]\n", pc.name+":")
		}
	}
}
