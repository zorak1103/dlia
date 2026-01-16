package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zorak1103/dlia/internal/templates"
)

var (
	force bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize DLIA configuration and directory structure",
	Long: `Init creates the necessary configuration files and directories for DLIA.

This command will create:
  - config.yaml (sample configuration file)
  - .env (environment variable template)
  - reports/ (directory for scan reports)
  - knowledge_base/ (directory for accumulated knowledge)
  - knowledge_base/services/ (directory for per-service summaries)

Run this once when setting up DLIA for the first time.`,
	Example: `  # Initialize in current directory
  dlia init

  # Force overwrite existing files
  dlia init --force`,
	RunE: func(_ *cobra.Command, _ []string) error {
		fmt.Println("🔧 Initializing DLIA...")

		dirs := []string{
			"reports",
			"knowledge_base",
			filepath.Join("knowledge_base", "services"),
		}

		for _, dir := range dirs {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
			fmt.Printf("✅ Created directory: %s\n", dir)
		}

		files := map[string][]byte{
			"config.yaml": templates.ConfigYAML,
			".env":        templates.EnvFile,
		}

		for filename, content := range files {
			if _, err := os.Stat(filename); err == nil && !force {
				fmt.Printf("⚠️  Skipping %s (already exists, use --force to overwrite)\n", filename)
				continue
			}

			if err := os.WriteFile(filename, content, 0o600); err != nil {
				return fmt.Errorf("failed to write %s: %w", filename, err)
			}

			fmt.Printf("✅ Created %s\n", filename)
		}

		globalSummaryPath := filepath.Join("knowledge_base", "global_summary.md")
		if _, err := os.Stat(globalSummaryPath); os.IsNotExist(err) {
			initialContent := `# DLIA Global Summary

This file contains the system-wide knowledge base accumulated across all container scans.

## Status
No scans performed yet.
`
			if err := os.WriteFile(globalSummaryPath, []byte(initialContent), 0o600); err != nil {
				return fmt.Errorf("failed to create global_summary.md: %w", err)
			}
			fmt.Printf("✅ Created %s\n", globalSummaryPath)
		}

		fmt.Println("\n🎉 Initialization complete!")
		fmt.Println("\n📝 Next steps:")
		fmt.Println("   1. Edit config.yaml to configure your LLM API")
		fmt.Println("   2. Edit .env to add your API key and other secrets")
		fmt.Println("   3. Run 'dlia scan --dry-run' to test your setup")
		fmt.Println("   4. Run 'dlia scan' to perform your first analysis")

		return nil
	},
}

// nolint:gochecknoinits // Standard Cobra pattern for command registration
func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().BoolVar(&force, "force", false, "overwrite existing configuration files")
}
