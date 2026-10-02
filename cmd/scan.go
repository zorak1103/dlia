package cmd

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"github.com/zorak1103/dlia/internal/chunking"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/knowledge"
	"github.com/zorak1103/dlia/internal/prompts"
	"github.com/zorak1103/dlia/internal/severity"
	"github.com/zorak1103/dlia/internal/state"
)

var scanCmd = &cobra.Command{
	Use:   cmdScan,
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

	outcomes := buildOutcomes(globalResults, scanStats)

	if err := updateGlobalSummary(outcomes, cfg, scanCfg); err != nil {
		fmt.Printf("⚠️  Failed to update global summary: %v\n", err)
	}

	if err := handleExecutiveSummaryAndNotifications(ctx, outcomes, cfg, scanCfg); err != nil {
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

// getDefaultPromptLoader is a seam for tests to stub prompt loader lookup.
var getDefaultPromptLoader = prompts.GetDefaultLoader

func displayPromptConfiguration() {
	fmt.Println("\n📝 Prompt Configuration:")
	loader := getDefaultPromptLoader()
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
	dockerClient, err := connectDocker(cfg)
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

// nowFunc is the clock used for scan windows; tests replace it.
var nowFunc = time.Now

type scanStats struct {
	totalLogs         int
	scannedContainers int
	failedContainers  int
	failedNames       []string
	reportPaths       map[string]string
}

// logGap describes a stretch of logs that was skipped because the saved cursor
// was older than scan.max_window.
type logGap struct {
	start, end time.Time
}

func (g logGap) note(maxWindow time.Duration) string {
	return fmt.Sprintf("Skipped log gap %s – %s (older than scan.max_window=%s)",
		g.start.UTC().Format(time.RFC3339), g.end.UTC().Format(time.RFC3339), maxWindow)
}

func processContainers(ctx context.Context, dockerClient docker.Client, st *state.State, containers []docker.Container, cfg *config.Config, scanCfg *scanConfig, lookbackDuration time.Duration) (map[string]*chunking.AnalyzeResult, scanStats) {
	globalResults := make(map[string]*chunking.AnalyzeResult, len(containers))
	stats := scanStats{reportPaths: make(map[string]string)}
	// Lazy initialization: pipeline is created on first use to avoid unnecessary
	// LLM client setup if all containers are skipped (e.g., no new logs).
	var llmPipeline *chunking.Pipeline

	for i, container := range containers {
		fmt.Printf("[%d/%d] Processing: %s (ID: %s)\n", i+1, len(containers), container.Name, container.ID[:12])

		result := processSingleContainer(ctx, dockerClient, st, container, cfg, scanCfg, lookbackDuration, &llmPipeline, &stats)
		if result != nil {
			globalResults[container.Name] = result
		}
	}

	return globalResults, stats
}

// processSingleContainer scans one container and returns its analysis result (nil if
// none). The state cursor only advances after a successful analysis, so failed
// analyses are retried on the next scan.
func processSingleContainer(ctx context.Context, dockerClient docker.Client, st *state.State, container docker.Container, cfg *config.Config, scanCfg *scanConfig, lookbackDuration time.Duration, llmPipeline **chunking.Pipeline, stats *scanStats) *chunking.AnalyzeResult {
	now := nowFunc()
	since, gap := determineLogStartTime(st, container.ID, scanCfg, lookbackDuration, cfg.Scan.MaxWindow, now)

	logs, err := processContainerLogs(ctx, dockerClient, container.ID, since)
	if err != nil {
		fmt.Printf("        ⚠️  %v\n", err)
		recordFailure(st, container, since, scanCfg, lookbackDuration, stats)
		stats.scannedContainers++
		fmt.Println()
		return nil
	}

	if len(logs) == 0 {
		recordEmptyRead(st, container, gap, now, cfg, scanCfg, lookbackDuration)
		fmt.Printf("        ℹ️  No new logs\n\n")
		return nil
	}

	var gapNote string
	if gap != nil {
		gapNote = gap.note(cfg.Scan.MaxWindow)
		fmt.Printf("        ⚠️  %s\n", gapNote)
	}

	fmt.Printf("        📝 Found %d new log entries\n", len(logs))
	stats.totalLogs += len(logs)

	displayLogsPreview(logs, scanCfg)

	result := processLLMAnalysis(ctx, container.Name, logs, cfg, scanCfg, llmPipeline)
	switch {
	case result != nil:
		for _, note := range result.CoverageNotes {
			fmt.Printf("        ⚠️  %s\n", note)
		}
		if gapNote != "" {
			result.CoverageNotes = append(result.CoverageNotes, gapNote)
		}
		path := handleReportingAndKnowledge(container.Name, result, logs, cfg, scanCfg)
		if path != "" {
			if stats.reportPaths == nil {
				stats.reportPaths = make(map[string]string)
			}
			stats.reportPaths[container.Name] = path
		}
		updateContainerState(st, container, logs, scanCfg, lookbackDuration)
	case scanCfg.dryRun:
		updateContainerState(st, container, logs, scanCfg, lookbackDuration)
	default:
		recordFailure(st, container, since, scanCfg, lookbackDuration, stats)
	}

	stats.scannedContainers++
	fmt.Println()
	return result
}

// emptyReadCursorMargin tolerates daemon/host clock skew; a line inside the margin may be read twice.
const emptyReadCursorMargin = 5 * time.Second

// recordEmptyRead handles a successful read that returned no logs. It reports a
// max_window gap (once, because the cursor moves past it) and advances the cursor
// to just before the read time (see emptyReadCursorMargin, never moving it
// backwards), so a quiet container neither drifts past max_window nor re-reports
// the gap on every scan. Dry runs and lookback scans leave state alone.
func recordEmptyRead(st *state.State, container docker.Container, gap *logGap, now time.Time, cfg *config.Config, scanCfg *scanConfig, lookbackDuration time.Duration) {
	if scanCfg.dryRun || lookbackDuration != 0 {
		return
	}
	if gap != nil {
		fmt.Printf("        ⚠️  %s\n", gap.note(cfg.Scan.MaxWindow))
	}
	cursor := now.Add(-emptyReadCursorMargin)
	if existing, ok := st.GetLastScan(container.ID); ok && existing.After(cursor) {
		cursor = existing
	}
	st.UpdateContainer(container.ID, container.Name, cursor, "")
}

// keepWindowOnFailure records the scan start as the cursor of a container that has
// none yet, so a failed first analysis is retried from the same window instead of
// a fresh "last hour".
func keepWindowOnFailure(st *state.State, container docker.Container, since time.Time, scanCfg *scanConfig, lookbackDuration time.Duration) {
	if scanCfg.dryRun || lookbackDuration != 0 {
		return
	}
	if _, exists := st.GetLastScan(container.ID); !exists {
		st.UpdateContainer(container.ID, container.Name, since, "")
	}
}

// recordFailure keeps the scan window for a container whose log read or
// analysis failed and counts it. Dry runs are not counted as failed.
func recordFailure(st *state.State, container docker.Container, since time.Time,
	scanCfg *scanConfig, lookbackDuration time.Duration, stats *scanStats) {
	keepWindowOnFailure(st, container, since, scanCfg, lookbackDuration)
	if !scanCfg.dryRun {
		stats.failedContainers++
		stats.failedNames = append(stats.failedNames, container.Name)
	}
}

func determineLogStartTime(st *state.State, containerID string, scanCfg *scanConfig, lookbackDuration, maxWindow time.Duration, now time.Time) (time.Time, *logGap) {
	if lookbackDuration > 0 {
		since := now.Add(-lookbackDuration)
		if scanCfg.verbose {
			fmt.Printf("        Reading logs from: %s (lookback: %s)\n", since.Format(time.RFC3339), scanCfg.lookback)
		}
		return since, nil
	}

	// Use state
	if lastScan, exists := st.GetLastScan(containerID); exists {
		if floor := now.Add(-maxWindow); maxWindow > 0 && lastScan.Before(floor) {
			return floor, &logGap{start: lastScan, end: floor}
		}
		if scanCfg.verbose {
			fmt.Printf("        Reading logs since: %s (from state)\n", lastScan.Format(time.RFC3339))
		}
		return lastScan, nil
	}

	// First scan of this container: default to last 1 hour to prevent overwhelming
	// the initial analysis with potentially thousands of historical log entries.
	// Rationale: 1 hour balances between meaningful recent context and manageable
	// data volume (typical container generates 100-1000 log lines/hour).
	// After the first scan, subsequent runs process only new logs incrementally.
	since := now.Add(-1 * time.Hour)
	if scanCfg.verbose {
		fmt.Printf("        First scan, reading logs from: %s (last 1 hour)\n", since.Format(time.RFC3339))
	}
	return since, nil
}

func displayLogsPreview(logs []docker.LogEntry, scanCfg *scanConfig) {
	if scanCfg.verbose && len(logs) > 0 {
		fmt.Printf("        \n")
		displayCount := min(len(logs), 10)
		for j := range displayCount {
			entry := logs[j]
			fmt.Printf("        [%s] %s\n", entry.Timestamp, entry.Message)
		}
		if len(logs) > 10 {
			fmt.Printf("        ... (%d more lines)\n", len(logs)-10)
		}
		fmt.Printf("        \n")
	}
}

func handleReportingAndKnowledge(containerName string, result *chunking.AnalyzeResult, logs []docker.LogEntry, cfg *config.Config, scanCfg *scanConfig) string {
	path, err := generateAndSaveReport(containerName, result, logs, cfg, scanCfg)
	if err != nil {
		fmt.Printf("        ⚠️  Failed to save report: %v\n", err)
	}

	if err := knowledge.UpdateServiceKB(containerName, result, cfg); err != nil {
		fmt.Printf("        ⚠️  Failed to update knowledge base: %v\n", err)
	} else if scanCfg.verbose {
		fmt.Printf("        🧠 Knowledge base updated\n")
	}
	return path
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

// buildOutcomes assembles the knowledge.ServiceOutcome map from successful
// results and the per-container report paths collected during the scan.
func buildOutcomes(globalResults map[string]*chunking.AnalyzeResult, stats scanStats) map[string]knowledge.ServiceOutcome {
	outcomes := make(map[string]knowledge.ServiceOutcome, len(globalResults)+len(stats.failedNames))
	for name, res := range globalResults {
		outcomes[name] = knowledge.ServiceOutcome{
			Result:     res,
			ReportPath: stats.reportPaths[name],
		}
	}
	for _, name := range stats.failedNames {
		outcomes[name] = knowledge.ServiceOutcome{Result: nil}
	}
	return outcomes
}

// overallSeverity returns the maximum severity across all outcomes.
// Failed containers (nil Result) contribute severity.Unknown.
func overallSeverity(outcomes map[string]knowledge.ServiceOutcome) severity.Level {
	var levels []severity.Level
	for _, o := range outcomes {
		if o.Result == nil {
			levels = append(levels, severity.Unknown)
		} else {
			levels = append(levels, o.Result.Severity)
		}
	}
	return severity.Max(levels...)
}

func updateGlobalSummary(outcomes map[string]knowledge.ServiceOutcome, cfg *config.Config, scanCfg *scanConfig) error {
	if !scanCfg.dryRun && len(outcomes) > 0 {
		if err := knowledge.UpdateGlobalSummary(outcomes, cfg); err != nil {
			return err
		}
		if scanCfg.verbose {
			fmt.Println("🌍 Global summary updated")
		}
	}
	return nil
}

func handleExecutiveSummaryAndNotifications(ctx context.Context, outcomes map[string]knowledge.ServiceOutcome, cfg *config.Config, scanCfg *scanConfig) error {
	if scanCfg.dryRun || len(outcomes) == 0 {
		return nil
	}

	notifier, err := newNotifier(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize notifier: %w", err)
	}

	if !notifier.IsEnabled() {
		return nil
	}

	overall := overallSeverity(outcomes)
	threshold := cfg.Notification.Threshold()
	if overall < threshold {
		if scanCfg.verbose {
			fmt.Printf("Notification skipped (severity %s below threshold %s)\n", overall, threshold)
		}
		return nil
	}

	summary := buildExecSummary(ctx, outcomes, cfg, scanCfg)
	return notifyDecision(notifier, summary, outcomes, overall, scanCfg)
}

// buildExecSummary generates the executive summary from successful outcomes.
// On error it prints a warning and returns an empty string.
func buildExecSummary(ctx context.Context, outcomes map[string]knowledge.ServiceOutcome, cfg *config.Config, scanCfg *scanConfig) string {
	// Compute the overall level before the empty check: failed containers
	// (nil Result) contribute unknown to the overall level even when no
	// container succeeded; the early return only skips the LLM call.
	overall := overallSeverity(outcomes)

	analyses := make([]prompts.ContainerAnalysis, 0, len(outcomes))
	for name, o := range outcomes {
		if o.Result != nil {
			analyses = append(analyses, prompts.ContainerAnalysis{
				Name:     name,
				Severity: o.Result.Severity,
				Analysis: o.Result.Analysis,
			})
		}
	}
	if len(analyses) == 0 {
		return ""
	}

	llmPipeline, err := initializeLLMPipeline(cfg, scanCfg)
	if err != nil {
		fmt.Printf("⚠️  Executive summary failed: %v\n", err)
		return ""
	}

	if scanCfg.verbose {
		fmt.Println("📊 Generating executive summary...")
	}

	summary, err := generateExecutiveSummary(ctx, llmPipeline, analyses, overall, cfg)
	if err != nil {
		fmt.Printf("⚠️  Executive summary failed: %v\n", err)
		return ""
	}

	if scanCfg.verbose {
		fmt.Println("✅ Executive summary generated")
	}
	return summary
}

// notifyDecision sends the notification and formats the failed list.
func notifyDecision(notifier scanNotifier, summary string, outcomes map[string]knowledge.ServiceOutcome, overall severity.Level, scanCfg *scanConfig) error {
	if scanCfg.verbose {
		fmt.Println("📧 Sending notification...")
	}

	failed := sortedFailedNames(outcomes)

	if err := notifier.SendScanSummary(summary, len(outcomes), overall, failed); err != nil {
		return fmt.Errorf("notification failed: %w", err)
	}

	fmt.Println("✅ Notification sent successfully")
	return nil
}

// sortedFailedNames returns the names of failed outcomes in sorted order.
func sortedFailedNames(outcomes map[string]knowledge.ServiceOutcome) []string {
	var failed []string
	for name, o := range outcomes {
		if o.Result == nil {
			failed = append(failed, name)
		}
	}
	sort.Strings(failed)
	return failed
}

func displayScanSummary(stats scanStats, scanCfg *scanConfig, lookbackDuration time.Duration) {
	fmt.Println("=" + "═══════════════════════════════════════")
	fmt.Printf("✅ Scan complete!\n")
	fmt.Printf("   Containers scanned: %d\n", stats.scannedContainers)
	fmt.Printf("   Total log entries: %d\n", stats.totalLogs)
	if stats.failedContainers > 0 {
		fmt.Printf("   Failed analyses: %d (will be retried next scan)\n", stats.failedContainers)
	}

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

func generateExecutiveSummary(ctx context.Context, _ *chunking.Pipeline, analyses []prompts.ContainerAnalysis, overall severity.Level, cfg *config.Config) (string, error) {
	promptLoader := prompts.NewPromptLoader(cfg)

	m, err := promptLoader.ExecutiveSummaryMessages(analyses, overall)
	if err != nil {
		return "", fmt.Errorf("failed to load executive summary prompt: %w", err)
	}

	llmClient := newLLMClient(llmOptions(cfg))

	// Empty container name parameter: this is a cross-container global summary
	summary, _, err := llmClient.Analyze(ctx, "", m.System, m.User)
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}

	return summary, nil
}
