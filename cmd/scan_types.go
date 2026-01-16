package cmd

import "github.com/spf13/cobra"

// scanConfig holds all scan-specific configuration flags.
// This structure replaces the package-level global variables
// to enable better testing and dependency injection.
type scanConfig struct {
	// dryRun simulates a scan without calling the LLM or updating state files.
	// Useful for testing log collection and filtering without API consumption.
	dryRun bool

	// filter is a regex pattern used to filter container names during scan.
	// Only containers matching this pattern will be scanned.
	filter string

	// lookback specifies a duration to look back for logs (e.g., "1h", "24h").
	// When set, the state file is ignored and logs are read from the specified duration ago.
	lookback string

	// llmLog enables logging of all LLM requests and responses to markdown files.
	// Log files are saved to the configured LLM log directory for debugging and auditing.
	llmLog bool

	// filterStats displays detailed filtering statistics showing how many log lines
	// were filtered by regexp patterns during log processing.
	filterStats bool

	// verbose enables detailed output during scan operations.
	// Inherited from root command but included here for explicit dependency tracking.
	verbose bool
}

// newScanConfigFromCmd creates a new scanConfig from Cobra command flags.
// This function reads flag values directly from the command, avoiding global state.
// This is the production code path - flags are set by users via CLI.
func newScanConfigFromCmd(cmd *cobra.Command) *scanConfig {
	// GetBool/GetString never return errors when flags are properly defined
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	filter, _ := cmd.Flags().GetString("filter")
	lookback, _ := cmd.Flags().GetString("lookback")
	llmLog, _ := cmd.Flags().GetBool("llmlog")
	filterStats, _ := cmd.Flags().GetBool("filter-stats")

	return &scanConfig{
		dryRun:      dryRun,
		filter:      filter,
		lookback:    lookback,
		llmLog:      llmLog,
		filterStats: filterStats,
		verbose:     verbose, // Still using global from root command
	}
}

// newTestScanConfig creates a scanConfig for testing with default values.
// This helps tests avoid depending on Cobra commands or global variables.
func newTestScanConfig() *scanConfig {
	return &scanConfig{
		dryRun:      false,
		filter:      "",
		lookback:    "",
		llmLog:      false,
		filterStats: false,
		verbose:     false,
	}
}
