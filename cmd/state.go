package cmd

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/zorak1103/dlia/internal/state"
)

var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "Manage log scan state (cursors/timestamps)",
	Long: `State management commands for inspecting and resetting the log scan state.

The state file tracks the last processed log timestamp for each container,
allowing incremental scanning without reprocessing old logs.`,
}

var stateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List current log scan state for all containers",
	Long: `Display the current state file showing which containers have been scanned
and the timestamp of the last processed log entry for each.`,
	Example: `  # List all container states
  dlia state list

  # List with verbose output
  dlia state list --verbose`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg = GetConfig()
		if err := validateConfigOrExit(cfg, "state"); err != nil {
			return err
		}

		// Load state
		st, err := state.Load(cfg.Output.StateFile)
		if err != nil {
			return fmt.Errorf("failed to load state: %w", err)
		}

		containers := st.GetAllContainers()

		// Write output to stdout; errors writing to stdout are not actionable in CLI context
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "📊 Current Log Scan State:")
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "")

		if len(containers) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "ℹ️  No containers in state file")
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "   State file: %s\n", cfg.Output.StateFile)
			return nil
		}

		// Create table writer
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
		_, _ = fmt.Fprintln(w, "Container ID\tName\tLast Scan\tCursor")
		_, _ = fmt.Fprintln(w, "------------\t----\t---------\t------")

		for id, ctr := range containers {
			shortID := id
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}

			lastScan := ctr.LastScan.Format("2006-01-02 15:04:05")
			if ctr.LastScan.IsZero() {
				lastScan = "Never"
			}

			cursor := ctr.LogCursor
			if cursor == "" {
				cursor = "-"
			}

			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", shortID, ctr.Name, lastScan, cursor)
		}

		_ = w.Flush() // Flush buffered output; error not actionable in CLI display context
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "")
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Total: %d container(s)\n", len(containers))
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "State file: %s\n", cfg.Output.StateFile)
		if !st.LastUpdated.IsZero() {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Last updated: %s\n", st.LastUpdated.Format(time.RFC3339))
		}

		return nil
	},
}

var stateResetCmd = &cobra.Command{
	Use:   "reset [container-filter]",
	Short: "Reset log scan state (optionally for specific containers)",
	Long: `Reset the state file to force a fresh scan.

Without arguments, resets state for ALL containers.
With a container name or pattern, resets only matching containers.

WARNING: This will cause the next scan to reprocess all available logs,
which may result in duplicate analysis and higher LLM costs.`,
	Example: `  # Reset all container states
  dlia state reset --force

  # Reset only nginx containers
  dlia state reset nginx --force

  # Reset with pattern matching
  dlia state reset "app-.*" --force`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg = GetConfig()
		if err := validateConfigOrExit(cfg, "state"); err != nil {
			return err
		}

		filter := ""
		if len(args) > 0 {
			filter = args[0]
		}

		if filter == "" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "⚠️  Resetting state for ALL containers")
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "⚠️  Resetting state for containers matching: %s\n", filter)
		}

		if !force {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "❌ Aborted (use --force to confirm)")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "This will cause the next scan to reprocess all logs.")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Run with --force if you're sure.")
			return nil
		}

		// Load state
		st, err := state.Load(cfg.Output.StateFile)
		if err != nil {
			return fmt.Errorf("failed to load state: %w", err)
		}

		if filter == "" {
			// Reset all
			oldCount := st.Count()
			if err := st.Delete(); err != nil {
				return fmt.Errorf("failed to delete state file: %w", err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "✅ State reset complete")
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "   Removed %d container(s) from state\n", oldCount)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "   Deleted: %s\n", cfg.Output.StateFile)
		} else {
			// Reset filtered
			count, err := st.ResetFiltered(filter)
			if err != nil {
				return fmt.Errorf("failed to reset filtered state: %w", err)
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "")
			if count == 0 {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "ℹ️  No containers matched pattern: %s\n", filter)
			} else {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "✅ State reset complete")
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "   Removed %d container(s) matching '%s'\n", count, filter)
			}
		}

		return nil
	},
}

// nolint:gochecknoinits // Standard Cobra pattern for command registration
func init() {
	rootCmd.AddCommand(stateCmd)
	stateCmd.AddCommand(stateListCmd)
	stateCmd.AddCommand(stateResetCmd)

	// Reset-specific flags
	stateResetCmd.Flags().BoolVar(&force, "force", false, "confirm state reset")
}
