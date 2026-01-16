package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zorak1103/dlia/internal/chunking"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/knowledge"
	"github.com/zorak1103/dlia/internal/llm"
	"github.com/zorak1103/dlia/internal/notification"
	"github.com/zorak1103/dlia/internal/prompts"
	"github.com/zorak1103/dlia/internal/state"
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Perform a one-time scan of Docker container logs",
	Long: `Scan performs a single analysis pass over Docker container logs.

This command:
  1. Reads new logs from all containers since the last scan
  2. Analyzes them using the configured LLM
  3. Generates a report and updates the knowledge base (Phase 4)
  4. Sends notifications if configured (Phase 5)

Use this for one-off scans or when integrating with external cron/schedulers.`,
	Example: `  # Scan all containers
  dlia scan

  # Scan with dry-run (no LLM calls, no state changes)
  dlia scan --dry-run

  # Scan only nginx containers
  dlia scan --filter "nginx.*"

  # Scan last 24 hours of logs, ignoring state
  dlia scan --lookback 24h

  # Combine filters with lookback and verbose output
  dlia scan --filter "app-.*" --lookback 1h --verbose`,
	RunE: runScan,
}

// nolint:gochecknoinits // Standard Cobra pattern for command registration
func init() {
	rootCmd.AddCommand(scanCmd)

	// Define flags without global variables - values are stored internally by Cobra
	scanCmd.Flags().Bool("dry-run", false, "simulate scan without calling LLM or updating state")
	scanCmd.Flags().String("filter", "", "regex pattern to filter container names")
	scanCmd.Flags().String("lookback", "", "duration to look back (e.g., 1h, 24h), ignores state file")
	scanCmd.Flags().Bool("llmlog", false, "enable logging of all LLM requests and responses to markdown files")
	scanCmd.Flags().Bool("filter-stats", false, "display filter statistics showing how many log lines were filtered")
}

func runScan(cmd *cobra.Command, _ []string) error {
	cfg = GetConfig()
	if err := validateConfigOrExit(cfg, "scan"); err != nil {
		return err
	}

	scanCfg := newScanConfigFromCmd(cmd)

	// Initialize custom prompt overrides from config (if user provided custom templates).
	// This must happen before LLM pipeline creation to ensure correct prompts are loaded.
	prompts.InitPrompts(cfg)

	ctx := context.Background()

	lookbackDuration, err := parseLookbackDuration(scanCfg)
	if err != nil {
		return err
	}

	displayScanHeader(cfg, scanCfg, lookbackDuration)

	dockerClient, st, err := initializeDockerAndState(ctx, cfg, scanCfg, lookbackDuration)
	if err != nil {
		return err
	}
	defer dockerClient.Close() //nolint:errcheck // Close error not actionable in defer context

	containers, err := getContainersToScan(ctx, dockerClient, scanCfg)
	if err != nil {
		return err
	}

	if len(containers) == 0 {
		displayNoContainersFound(scanCfg)
		return nil
	}

	fmt.Printf("📦 Found %d container(s) to scan\n\n", len(containers))

	globalResults, scanStats := processContainers(ctx, dockerClient, st, containers, cfg, scanCfg, lookbackDuration)

	if err := saveStateIfNeeded(st, scanCfg, lookbackDuration); err != nil {
		return err
	}

	if err := updateGlobalSummary(globalResults, cfg, scanCfg); err != nil {
		fmt.Printf("⚠️  Failed to update global summary: %v\n", err)
	}

	if err := handleExecutiveSummaryAndNotifications(ctx, globalResults, cfg, scanCfg); err != nil {
		fmt.Printf("⚠️  Failed to handle executive summary: %v\n", err)
	}

	displayScanSummary(scanStats, scanCfg, lookbackDuration)
	return nil
}

func parseLookbackDuration(scanCfg *scanConfig) (time.Duration, error) {
	if scanCfg.lookback != "" {
		duration, err := time.ParseDuration(scanCfg.lookback)
		if err != nil {
			return 0, fmt.Errorf("invalid lookback duration '%s': %w (use format like: 1h, 24h, 30m)", scanCfg.lookback, err)
		}
		return duration, nil
	}
	return 0, nil
}

func displayScanHeader(cfg *config.Config, scanCfg *scanConfig, lookbackDuration time.Duration) {
	if scanCfg.verbose {
		displayVerboseHeader(cfg, scanCfg, lookbackDuration)
	}

	fmt.Println("🔍 Starting container log scan...")

	if scanCfg.dryRun {
		fmt.Println("⚠️  DRY RUN MODE - No LLM calls will be made, state will not be updated")
	}
	fmt.Println()
}

func displayVerboseHeader(cfg *config.Config, scanCfg *scanConfig, lookbackDuration time.Duration) {
	fmt.Println("=== DLIA Container Log Scan ===")
	fmt.Printf("Dry Run: %v\n", scanCfg.dryRun)
	if scanCfg.filter != "" {
		fmt.Printf("Container Filter: %s\n", scanCfg.filter)
	}
	if lookbackDuration > 0 {
		fmt.Printf("Lookback Duration: %s\n", lookbackDuration)
	}
	fmt.Printf("LLM Model: %s\n", cfg.LLM.Model)
	fmt.Printf("Docker Socket: %s\n", cfg.Docker.SocketPath)
	fmt.Printf("State File: %s\n", cfg.Output.StateFile)

	displayPromptConfiguration()
}

func displayPromptConfiguration() {
	fmt.Println("\n📝 Prompt Configuration:")
	loader := prompts.GetDefaultLoader()
	if loader == nil {
		fmt.Println()
		return
	}

	sources := loader.GetAllPromptSources()
	if len(sources) == 0 {
		fmt.Println("   Using built-in defaults (will be loaded on first use)")
		fmt.Println()
		return
	}

	for name, source := range sources {
		fmt.Printf("   %s: %s\n", name, source)
	}
	fmt.Println()
}

func initializeDockerAndState(ctx context.Context, cfg *config.Config, scanCfg *scanConfig, lookbackDuration time.Duration) (docker.Client, *state.State, error) {
	if scanCfg.verbose {
		fmt.Println("🐳 Connecting to Docker...")
	}
	dockerClient, err := docker.NewClient(cfg.Docker.SocketPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Verify Docker connection
	err = dockerClient.Ping(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to Docker daemon: %w\nMake sure Docker is running and you have permission to access the socket", err)
	}

	var st *state.State
	if lookbackDuration == 0 && !scanCfg.dryRun {
		st, err = state.Load(cfg.Output.StateFile)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load state: %w", err)
		}
		if scanCfg.verbose {
			fmt.Printf("📊 Loaded state with %d container(s)\n", st.Count())
		}
	} else {
		// Lookback/dry-run mode: state tracking disabled, always starts fresh
		st, _ = state.Load(cfg.Output.StateFile) //nolint:errcheck // Intentionally ignoring error in lookback/dry-run mode
		if scanCfg.verbose && lookbackDuration > 0 {
			fmt.Printf("📊 Using lookback mode, ignoring state file\n")
		}
	}

	return dockerClient, st, nil
}

func getContainersToScan(ctx context.Context, dockerClient docker.Client, scanCfg *scanConfig) ([]docker.Container, error) {
	return validateAndFilterContainers(ctx, dockerClient, scanCfg.filter)
}

func displayNoContainersFound(scanCfg *scanConfig) {
	fmt.Println("ℹ️  No containers found")
	if scanCfg.filter != "" {
		fmt.Printf("   (with filter: %s)\n", scanCfg.filter)
	}
}

type scanStats struct {
	totalLogs         int
	scannedContainers int
}

func processContainers(ctx context.Context, dockerClient docker.Client, st *state.State, containers []docker.Container, cfg *config.Config, scanCfg *scanConfig, lookbackDuration time.Duration) (map[string]*chunking.AnalyzeResult, scanStats) {
	globalResults := make(map[string]*chunking.AnalyzeResult, len(containers))
	stats := scanStats{}
	// Lazy initialization: pipeline is created on first use to avoid unnecessary
	// LLM client setup if all containers are skipped (e.g., no new logs).
	var llmPipeline *chunking.Pipeline

	for i, container := range containers {
		fmt.Printf("[%d/%d] Processing: %s (ID: %s)\n", i+1, len(containers), container.Name, container.ID[:12])

		since := determineLogStartTime(st, container.ID, scanCfg, lookbackDuration)

		logs, err := processContainerLogs(ctx, dockerClient, container.ID, since)
		if err != nil {
			fmt.Printf("        ⚠️  %v\n", err)
			continue
		}

		if len(logs) == 0 {
			fmt.Printf("        ℹ️  No new logs\n\n")
			continue
		}

		fmt.Printf("        📝 Found %d new log entries\n", len(logs))
		stats.totalLogs += len(logs)

		displayLogsPreview(logs, scanCfg)

		result := processLLMAnalysis(ctx, container.Name, logs, cfg, scanCfg, &llmPipeline)
		if result != nil {
			handleReportingAndKnowledge(container.Name, result, logs, cfg, scanCfg)

			globalResults[container.Name] = result
		}

		updateContainerState(st, container, logs, scanCfg, lookbackDuration)

		stats.scannedContainers++
		fmt.Println()
	}

	return globalResults, stats
}

func determineLogStartTime(st *state.State, containerID string, scanCfg *scanConfig, lookbackDuration time.Duration) time.Time {
	if lookbackDuration > 0 {
		since := time.Now().Add(-lookbackDuration)
		if scanCfg.verbose {
			fmt.Printf("        Reading logs from: %s (lookback: %s)\n", since.Format(time.RFC3339), scanCfg.lookback)
		}
		return since
	}

	// Use state
	if lastScan, exists := st.GetLastScan(containerID); exists {
		if scanCfg.verbose {
			fmt.Printf("        Reading logs since: %s (from state)\n", lastScan.Format(time.RFC3339))
		}
		return lastScan
	}

	// First scan of this container: default to last 1 hour to prevent overwhelming
	// the initial analysis with potentially thousands of historical log entries.
	// Rationale: 1 hour balances between meaningful recent context and manageable
	// data volume (typical container generates 100-1000 log lines/hour).
	// After the first scan, subsequent runs process only new logs incrementally.
	since := time.Now().Add(-1 * time.Hour)
	if scanCfg.verbose {
		fmt.Printf("        First scan, reading logs from: %s (last 1 hour)\n", since.Format(time.RFC3339))
	}
	return since
}

func displayLogsPreview(logs []docker.LogEntry, scanCfg *scanConfig) {
	if scanCfg.verbose && len(logs) > 0 {
		fmt.Printf("        \n")
		displayCount := len(logs)
		if displayCount > 10 {
			displayCount = 10
		}
		for j := 0; j < displayCount; j++ {
			entry := logs[j]
			fmt.Printf("        [%s] %s\n", entry.Timestamp, entry.Message)
		}
		if len(logs) > 10 {
			fmt.Printf("        ... (%d more lines)\n", len(logs)-10)
		}
		fmt.Printf("        \n")
	}
}

func handleReportingAndKnowledge(containerName string, result *chunking.AnalyzeResult, logs []docker.LogEntry, cfg *config.Config, scanCfg *scanConfig) {
	_, err := generateAndSaveReport(containerName, result, logs, cfg, scanCfg)
	if err != nil {
		fmt.Printf("        ⚠️  Failed to save report: %v\n", err)
	}

	if err := knowledge.UpdateServiceKB(containerName, result, cfg); err != nil {
		fmt.Printf("        ⚠️  Failed to update knowledge base: %v\n", err)
	} else if scanCfg.verbose {
		fmt.Printf("        🧠 Knowledge base updated\n")
	}
}

func updateContainerState(st *state.State, container docker.Container, logs []docker.LogEntry, scanCfg *scanConfig, lookbackDuration time.Duration) {
	if len(logs) == 0 {
		return
	}

	latestTime, err := docker.GetLatestLogTime(logs)
	if err != nil {
		if scanCfg.verbose {
			fmt.Printf("        ⚠️  Could not parse latest timestamp: %v\n", err)
		}
		return
	}

	if scanCfg.dryRun {
		fmt.Printf("        🔸 DRY RUN: Would update state to: %s\n", latestTime.Format(time.RFC3339))
	} else if lookbackDuration == 0 {
		st.UpdateContainer(container.ID, container.Name, latestTime, "")
		if scanCfg.verbose {
			fmt.Printf("        ✅ Updated state to: %s\n", latestTime.Format(time.RFC3339))
		}
	}
}

func saveStateIfNeeded(st *state.State, scanCfg *scanConfig, lookbackDuration time.Duration) error {
	if !scanCfg.dryRun && lookbackDuration == 0 {
		if err := st.Save(); err != nil {
			return fmt.Errorf("failed to save state: %w", err)
		}
		if scanCfg.verbose {
			fmt.Println("💾 State saved successfully")
		}
	}
	return nil
}

func updateGlobalSummary(globalResults map[string]*chunking.AnalyzeResult, cfg *config.Config, scanCfg *scanConfig) error {
	if !scanCfg.dryRun && len(globalResults) > 0 {
		if err := knowledge.UpdateGlobalSummary(globalResults, cfg); err != nil {
			return err
		}
		if scanCfg.verbose {
			fmt.Println("🌍 Global summary updated")
		}
	}
	return nil
}

func handleExecutiveSummaryAndNotifications(ctx context.Context, globalResults map[string]*chunking.AnalyzeResult, cfg *config.Config, scanCfg *scanConfig) error {
	if scanCfg.dryRun || len(globalResults) == 0 {
		return nil
	}

	llmPipeline, err := initializeLLMPipeline(cfg, scanCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize LLM for executive summary: %w", err)
	}

	if scanCfg.verbose {
		fmt.Println("📊 Generating executive summary...")
	}

	containerAnalyses := make(map[string]string, len(globalResults))
	for name, result := range globalResults {
		containerAnalyses[name] = result.Analysis
	}

	execSummary, err := generateExecutiveSummary(ctx, llmPipeline, containerAnalyses, cfg)
	if err != nil {
		return fmt.Errorf("failed to generate executive summary: %w", err)
	}

	if scanCfg.verbose {
		fmt.Println("✅ Executive summary generated")
	}

	return sendNotificationIfNeeded(execSummary, len(globalResults), containerAnalyses, cfg, scanCfg)
}

func sendNotificationIfNeeded(execSummary string, resultCount int, containerAnalyses map[string]string, cfg *config.Config, scanCfg *scanConfig) error {
	notifier, err := notification.NewNotifier(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize notifier: %w", err)
	}

	if !notifier.IsEnabled() {
		return nil
	}

	if scanCfg.verbose {
		fmt.Println("📧 Sending notification...")
	}

	issuesFound := detectIssues(containerAnalyses)

	if err := notifier.SendScanSummary(execSummary, resultCount, issuesFound); err != nil {
		return fmt.Errorf("notification failed: %w", err)
	}

	fmt.Println("✅ Notification sent successfully")
	return nil
}

func displayScanSummary(stats scanStats, scanCfg *scanConfig, lookbackDuration time.Duration) {
	fmt.Println("=" + "═══════════════════════════════════════")
	fmt.Printf("✅ Scan complete!\n")
	fmt.Printf("   Containers scanned: %d\n", stats.scannedContainers)
	fmt.Printf("   Total log entries: %d\n", stats.totalLogs)

	switch {
	case scanCfg.dryRun:
		fmt.Printf("   State: Not modified (dry-run)\n")
	case lookbackDuration > 0:
		fmt.Printf("   State: Not modified (lookback mode)\n")
	default:
		fmt.Printf("   State: Updated\n")
	}
	fmt.Println()
}

func generateExecutiveSummary(ctx context.Context, _ *chunking.Pipeline, containerAnalyses map[string]string, cfg *config.Config) (string, error) {
	promptLoader := prompts.NewPromptLoader(cfg)

	prompt, err := promptLoader.ExecutiveSummaryPrompt(containerAnalyses)
	if err != nil {
		return "", fmt.Errorf("failed to load executive summary prompt: %w", err)
	}

	llmClient := llm.NewClient(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model)

	systemPrompt, err := promptLoader.SystemPrompt("")
	if err != nil {
		return "", fmt.Errorf("failed to load system prompt: %w", err)
	}

	// Empty container name parameter: this is a cross-container global summary
	summary, _, err := llmClient.Analyze(ctx, "", systemPrompt, prompt)
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}

	return summary, nil
}

// detectIssues performs a basic heuristic scan for common error/warning keywords
// in the LLM analysis text. This is intentionally conservative: it may produce
// false positives but ensures that potential issues trigger notifications.
// Future enhancement: Consider using the LLM to classify issue severity directly.
func detectIssues(containerAnalyses map[string]string) bool {
	issueKeywords := []string{
		"error", "failed", "exception", "critical", "warning",
		"issue", "problem", "alert", "urgent", "attention",
	}

	for _, analysis := range containerAnalyses {
		lowerAnalysis := strings.ToLower(analysis)
		for _, keyword := range issueKeywords {
			if strings.Contains(lowerAnalysis, keyword) {
				return true
			}
		}
	}

	return false
}
