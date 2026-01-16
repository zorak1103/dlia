package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/state"
)

const (
	checkmark = "✓"
)

var (
	cleanupDryRun bool
	cleanupForce  bool
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove obsolete container data",
	Long: `Identify and remove data for containers that no longer exist in Docker.

The cleanup command scans all storage locations (state file, knowledge base,
reports, and LLM logs) for references to containers that have been removed
from Docker. It can list obsolete data or remove it with confirmation.

Note: This command requires DLIA to be initialized. Run 'dlia init' first if 
you encounter configuration errors.`,
	Example: `  # List obsolete container data
  dlia cleanup list

  # Preview what would be deleted (dry-run)
  dlia cleanup execute --dry-run

  # Delete with confirmation prompt
  dlia cleanup execute

  # Delete without confirmation
  dlia cleanup execute --force`,
}

var cleanupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List obsolete container data",
	Long: `Display containers that exist in storage but no longer exist in Docker.

Shows which storage locations (state file, knowledge base, reports, LLM logs)
contain data for each obsolete container.`,
	Example: `  # List obsolete container data
  dlia cleanup list

  # List with verbose output
  dlia cleanup list --verbose`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg := GetConfig()
		if err := validateConfigOrExit(cfg, "cleanup"); err != nil {
			return err
		}

		// Initialize Docker client
		ctx := context.Background()
		dockerClient, err := docker.NewClient(cfg.Docker.SocketPath)
		if err != nil {
			return fmt.Errorf("failed to create Docker client: %w", err)
		}
		defer func() { _ = dockerClient.Close() }() // Close client; error not actionable in defer context

		// Ping Docker to verify connection
		if pingErr := dockerClient.Ping(ctx); pingErr != nil {
			return fmt.Errorf("failed to connect to Docker: %w", pingErr)
		}

		// Find obsolete containers
		obsolete, err := findObsoleteContainers(ctx, dockerClient, cfg)
		if err != nil {
			return fmt.Errorf("failed to find obsolete containers: %w", err)
		}

		// Display results
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "🧹 Obsolete Container Data:")
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "")

		if len(obsolete) == 0 {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s No obsolete container data found\n", checkmark)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  All storage is clean!")
			return nil
		}

		// Create table writer
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
		_, _ = fmt.Fprintln(w, "Container ID\tName\tState\tKB\tReports\tLLM Logs")
		_, _ = fmt.Fprintln(w, "------------\t----\t-----\t--\t-------\t--------")

		for _, obs := range obsolete {
			// Truncate container ID to 12 characters
			shortID := obs.ID
			if len(shortID) > 12 && !strings.HasPrefix(shortID, "orphaned-") {
				shortID = shortID[:12]
			}

			// Format name
			name := obs.Name
			if name == "" {
				name = "-"
			}

			// Format boolean flags as checkmarks
			state := " "
			if obs.InState {
				state = checkmark
			}
			kb := " "
			if obs.InKB {
				kb = checkmark
			}
			reports := " "
			if obs.InReports {
				reports = checkmark
			}
			llmLogs := " "
			if obs.InLLMLogs {
				llmLogs = checkmark
			}

			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", shortID, name, state, kb, reports, llmLogs)
		}

		_ = w.Flush() // Flush buffered output; error not actionable in CLI display context
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "")
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Found %d obsolete container(s)\n", len(obsolete))
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "")
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Run 'dlia cleanup --force' to remove this data")

		return nil
	},
}

var cleanupExecuteCmd = &cobra.Command{
	Use:   "execute",
	Short: "Remove obsolete container data",
	Long: `Remove data for containers that no longer exist in Docker.

By default, displays what will be deleted and prompts for confirmation.
Use --dry-run to preview without deleting, or --force to skip confirmation.`,
	Example: `  # Preview what would be deleted
  dlia cleanup execute --dry-run

  # Delete with confirmation prompt
  dlia cleanup execute

  # Delete without confirmation
  dlia cleanup execute --force`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg := GetConfig()
		if err := validateConfigOrExit(cfg, "cleanup"); err != nil {
			return err
		}

		// Initialize Docker client
		ctx := context.Background()
		dockerClient, err := docker.NewClient(cfg.Docker.SocketPath)
		if err != nil {
			return fmt.Errorf("failed to create Docker client: %w", err)
		}
		defer func() { _ = dockerClient.Close() }() // Close client; error not actionable in defer context

		// Ping Docker to verify connection
		if pingErr := dockerClient.Ping(ctx); pingErr != nil {
			return fmt.Errorf("failed to connect to Docker: %w", pingErr)
		}

		// Find obsolete containers
		obsolete, err := findObsoleteContainers(ctx, dockerClient, cfg)
		if err != nil {
			return fmt.Errorf("failed to find obsolete containers: %w", err)
		}

		if len(obsolete) == 0 {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s No obsolete container data found\n", checkmark)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  All storage is clean!")
			return nil
		}

		// Display what will be deleted
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "⚠️  Found %d obsolete container(s):\n", len(obsolete))
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "")

		for _, obs := range obsolete {
			shortID := obs.ID
			if len(shortID) > 12 && !strings.HasPrefix(shortID, "orphaned-") {
				shortID = shortID[:12]
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  • %s", shortID)
			if obs.Name != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), " (%s)", obs.Name)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "")

			// Show what will be deleted
			if obs.InState {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "    - State entry")
			}
			if obs.InKB {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "    - Knowledge base file")
			}
			if obs.InReports {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "    - Reports directory")
			}
			if obs.InLLMLogs {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "    - LLM logs directory")
			}
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "")

		// Dry-run mode - exit without deleting
		if cleanupDryRun {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "🔍 DRY RUN - No changes made")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "   Run without --dry-run to perform the cleanup")
			return nil
		}

		// Confirmation prompt (unless --force)
		if !cleanupForce {
			_, _ = fmt.Fprint(cmd.OutOrStdout(), "⚠️  Proceed with cleanup? (y/N): ")
			var response string
			if _, scanErr := fmt.Fscanln(cmd.InOrStdin(), &response); scanErr != nil {
				// Treat scan error as "no" response
				response = "n"
			}
			response = strings.ToLower(strings.TrimSpace(response))

			if response != "y" && response != "yes" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "")
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "❌ Cleanup canceled")
				return nil
			}
		}

		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "")
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "🧹 Cleaning up...")
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "")

		// Load state once for all deletions
		st, err := state.Load(cfg.Output.StateFile)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to load state: %w", err)
		}

		// Track results
		successCount := 0
		failureCount := 0
		var errors []string

		// Delete each obsolete container's data
		for _, obs := range obsolete {
			hasErrors := false
			shortID := obs.ID
			if len(shortID) > 12 && !strings.HasPrefix(shortID, "orphaned-") {
				shortID = shortID[:12]
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Removing %s...", shortID)

			// Delete from state (only if in state)
			if obs.InState && st != nil {
				if err := deleteFromState(obs.ID, st); err != nil {
					errors = append(errors, fmt.Sprintf("%s: state deletion failed: %v", shortID, err))
					hasErrors = true
				}
			}

			// Delete knowledge base file
			if obs.InKB {
				if err := deleteKnowledgeBase(obs.Name, cfg); err != nil {
					errors = append(errors, fmt.Sprintf("%s: KB deletion failed: %v", shortID, err))
					hasErrors = true
				}
			}

			// Delete reports directory
			if obs.InReports {
				if err := deleteReportsDir(obs.Name, cfg); err != nil {
					errors = append(errors, fmt.Sprintf("%s: reports deletion failed: %v", shortID, err))
					hasErrors = true
				}
			}

			// Delete LLM logs directory
			if obs.InLLMLogs {
				if err := deleteLLMLogsDir(obs.Name, cfg); err != nil {
					errors = append(errors, fmt.Sprintf("%s: LLM logs deletion failed: %v", shortID, err))
					hasErrors = true
				}
			}

			if hasErrors {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), " ✗")
				failureCount++
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), " %s\n", checkmark)
				successCount++
			}
		}

		// Display summary
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "")
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "✅ Cleanup complete")
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "   Removed: %d container(s)\n", successCount)
		if failureCount > 0 {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "   Failed: %d container(s)\n", failureCount)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "⚠️  Errors encountered:")
			for _, errMsg := range errors {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "   - %s\n", errMsg)
			}
		}

		return nil
	},
}

// nolint:gochecknoinits // Standard Cobra pattern for command registration
func init() {
	rootCmd.AddCommand(cleanupCmd)
	cleanupCmd.AddCommand(cleanupListCmd)
	cleanupCmd.AddCommand(cleanupExecuteCmd)

	// Global cleanup flags
	cleanupCmd.PersistentFlags().BoolVar(&cleanupDryRun, "dry-run", false, "show what would be deleted without actually deleting")
	cleanupCmd.PersistentFlags().BoolVar(&cleanupForce, "force", false, "skip confirmation prompt")
}
