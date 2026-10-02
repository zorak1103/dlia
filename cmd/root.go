// Package cmd implements the CLI commands.
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/version"
)

const (
	cmdCleanup = "cleanup"
	cmdConfig  = "config"
	cmdInit    = "init"
	cmdList    = "list"
	cmdScan    = "scan"
	cmdState   = "state"
)

var (
	cfgFile       string
	verbose       bool
	cfg           *config.Config
	errConfigLoad error
)

var rootCmd = &cobra.Command{
	Use:   "dlia",
	Short: "Docker Log Intelligence Agent",
	Long: `DLIA (Docker Log Intelligence Agent) is an AI-powered log analysis tool
that monitors Docker container logs and provides intelligent insights using LLMs.

It features:
  - Semantic log analysis using configurable LLM APIs
  - Scan results kept as a human-readable history (not fed back to the LLM)
  - Privacy: best-effort masking of IPs and secrets
  - Flexible notification system via Shoutrrr
  - Markdown-based persistent knowledge base`,
	Version: version.GetFullVersion(),
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		skipConfig := cmd.Name() == cmdInit || cmd.Name() == "help" || cmd.Name() == "version"
		if skipConfig {
			return nil
		}

		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			// Store config load error for commands that need it (scan, cleanup, state).
			// These commands will fail fast with validateConfigOrExit() in their RunE handlers.
			// Note: init command doesn't require config, so error is stored not thrown.
			errConfigLoad = err
			// Warn only for commands that might function without full config
			if verbose {
				fmt.Fprintf(os.Stderr, "Warning: Could not load config: %v\n", err)
			}
		}

		if cfg != nil {
			for _, w := range cfg.Warnings {
				fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
			}
		}

		if verbose && cfg != nil {
			fmt.Fprintf(os.Stderr, "Loaded configuration from: %s\n", cfg.ConfigFilePath)
		}

		return nil
	},
}

// exitError carries a process exit code through RunE errors to Execute().
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

// exitCodeFor maps an error returned by a RunE handler to the process exit code:
// 2 for config errors (exitError), 1 for everything else.
func exitCodeFor(err error) int {
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	return 1
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(exitCodeFor(err))
	}
}

// nolint:gochecknoinits // Standard Cobra pattern for command registration
func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
}

// GetConfig returns the loaded configuration or nil if not loaded.
// Must be called after rootCmd.PersistentPreRunE has executed.
func GetConfig() *config.Config {
	return cfg
}

// GetConfigLoadError returns any error encountered during config loading.
// Returns nil if configuration loaded successfully or was not attempted.
func GetConfigLoadError() error {
	return errConfigLoad
}

// IsVerbose returns whether verbose mode is enabled via the -v flag.
func IsVerbose() bool {
	return verbose
}
