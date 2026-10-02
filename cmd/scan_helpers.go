package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zorak1103/dlia/internal/chunking"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/llm"
	"github.com/zorak1103/dlia/internal/llmlogger"
	"github.com/zorak1103/dlia/internal/notification"
	"github.com/zorak1103/dlia/internal/prompts"
	"github.com/zorak1103/dlia/internal/reporting"
	"github.com/zorak1103/dlia/internal/severity"
)

func validateAndFilterContainers(ctx context.Context, dockerClient docker.Client, namePattern string) ([]docker.Container, error) {
	filterOpts := docker.FilterOptions{
		NamePattern: namePattern,
		IncludeAll:  true,
	}

	containers, err := dockerClient.ListContainers(ctx, filterOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	return containers, nil
}

func processContainerLogs(ctx context.Context, dockerClient docker.Client, containerID string, since time.Time) ([]docker.LogEntry, error) {
	logs, err := dockerClient.ReadLogsSince(ctx, containerID, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read logs for container %s: %w", containerID[:12], err)
	}

	return logs, nil
}

func processLLMAnalysis(ctx context.Context, containerName string, logs []docker.LogEntry, cfg *config.Config, scanCfg *scanConfig, pipelineRef **chunking.Pipeline) *chunking.AnalyzeResult {
	if scanCfg.dryRun {
		fmt.Printf("        🔸 DRY RUN: Skipping LLM analysis\n")
		return nil
	}

	fmt.Printf("        🤖 Analyzing logs with LLM...\n")

	if *pipelineRef == nil {
		pipeline, err := initializeLLMPipeline(cfg, scanCfg)
		if err != nil {
			fmt.Printf("        ⚠️  Failed to initialize LLM: %v\n", err)
			fmt.Printf("        ⚠️  Switching to dry-run mode (logs will be read but not analyzed)\n\n")
			scanCfg.dryRun = true
			return nil
		}
		*pipelineRef = pipeline
	}

	result, err := (*pipelineRef).AnalyzeLogs(ctx, containerName, logs)
	if err != nil {
		fmt.Printf("        ⚠️  LLM analysis failed: %v\n", err)
		fmt.Printf("        ⚠️  Logs were read but not analyzed\n\n")
		return nil
	}

	displayAnalysisResults(result, scanCfg)
	return result
}

func displayAnalysisResults(result *chunking.AnalyzeResult, scanCfg *scanConfig) {
	if scanCfg.verbose && result.Deduplicated {
		fmt.Printf("        📊 Deduplication: %d → %d entries\n", result.OriginalCount, result.ProcessedCount)
	}

	if scanCfg.filterStats && result.FilterStats.LinesTotal > 0 {
		// LinesTotal > 0 is guaranteed by the guard above, so the division is safe.
		percentage := float64(result.FilterStats.LinesFiltered) / float64(result.FilterStats.LinesTotal) * 100
		fmt.Printf("        🔍 Regexp Filter: Filtered %d/%d log lines (%.1f%%)\n",
			result.FilterStats.LinesFiltered,
			result.FilterStats.LinesTotal,
			percentage)
	}

	fmt.Printf("        \n")
	fmt.Printf("        ┌─ Analysis Results ─────────────────────\n")

	lines := strings.Split(result.Analysis, "\n")
	for _, line := range lines {
		if line != "" {
			fmt.Printf("        │ %s\n", line)
		}
	}

	fmt.Printf("        └────────────────────────────────────────\n")

	if scanCfg.verbose {
		fmt.Printf("        📊 Tokens used: %d", result.TokensUsed)
		if result.ChunksUsed > 1 {
			fmt.Printf(" (chunked analysis)")
		}
		fmt.Printf("\n")
	}

	fmt.Printf("        \n")
}

// newLLMClient is a seam for tests to stub LLM client construction.
var newLLMClient = llm.NewClient

// scanNotifier is the interface used by handleExecutiveSummaryAndNotifications.
// It allows tests to inject a fake without hitting Shoutrrr.
type scanNotifier interface {
	IsEnabled() bool
	SendScanSummary(summary string, containerCount int, overall severity.Level, failed []string) error
}

// newNotifier is a seam for tests to stub notifier construction.
var newNotifier = func(cfg *config.Config) (scanNotifier, error) { return notification.NewNotifier(cfg) }

func initializeLLMPipeline(cfg *config.Config, scanCfg *scanConfig) (*chunking.Pipeline, error) {
	if cfg.LLM.APIKey == "" {
		return nil, fmt.Errorf("LLM API key not configured (set DLIA_LLM_API_KEY in .env)")
	}

	llmClient := newLLMClient(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model)

	llmLogEnabled := scanCfg.llmLog || cfg.Output.LLMLogEnabled
	if llmLogEnabled {
		logger := llmlogger.NewLogger(cfg.Output.LLMLogDir, true)
		llmClient.SetLogger(logger)
		if scanCfg.verbose {
			fmt.Printf("📝 LLM logging enabled: %s\n", cfg.Output.LLMLogDir)
		}
	}

	// Create PromptLoader for dependency injection
	promptLoader := prompts.NewPromptLoader(cfg)

	pipeline, err := chunking.NewPipelineWithConfig(cfg.LLM.Model, cfg.LLM.ContextWindow, llmClient, promptLoader, cfg.Output.IgnoreDir, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create pipeline: %w", err)
	}

	if scanCfg.verbose && pipeline.TokenCountIsEstimate() {
		fmt.Printf("        ℹ️  Token counts are estimates for model %s; using 80%% of context_window\n", cfg.LLM.Model)
	}

	return pipeline, nil
}

func generateAndSaveReport(containerName string, result *chunking.AnalyzeResult, logs []docker.LogEntry, cfg *config.Config, scanCfg *scanConfig) (string, error) {
	reportContent := reporting.GenerateScanReport(containerName, result, logs)

	reportPath, err := reporting.SaveReport(containerName, reportContent, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to save report for %s: %w", containerName, err)
	}

	if scanCfg.verbose {
		fmt.Printf("        📄 Report saved: %s\n", reportPath)
	}

	return reportPath, nil
}
