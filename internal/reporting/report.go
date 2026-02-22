// Package reporting generates reports from analysis results.
package reporting

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zorak1103/dlia/internal/chunking"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/sanitize"
)

// GenerateScanReport formats analysis results as a markdown report.
func GenerateScanReport(containerName string, analysis *chunking.AnalyzeResult, _ []docker.LogEntry) string {
	var sb strings.Builder

	timestamp := time.Now().Format(time.RFC1123)

	// Header
	fmt.Fprintf(&sb, "# Scan Report: %s\n\n", containerName)
	fmt.Fprintf(&sb, "**Date:** %s  \n", timestamp)
	fmt.Fprintf(&sb, "**Container:** `%s`  \n", containerName)
	fmt.Fprintf(&sb, "**Log Entries:** %d  \n", analysis.OriginalCount)
	fmt.Fprintf(&sb, "**Tokens Used:** %d\n\n", analysis.TokensUsed)

	// Analysis Section
	sb.WriteString("## 🤖 AI Analysis\n\n")
	sb.WriteString(analysis.Analysis)
	sb.WriteString("\n\n")

	// Pre-Processing Statistics Section (if filtering occurred)
	if analysis.FilterStats.LinesTotal > 0 {
		sb.WriteString("## 🔍 Pre-Processing Statistics\n\n")
		sb.WriteString("| Metric | Value |\n")
		sb.WriteString("|--------|-------|\n")
		fmt.Fprintf(&sb, "| Total Log Lines | %d |\n", analysis.FilterStats.LinesTotal)
		fmt.Fprintf(&sb, "| Lines Filtered (Regexp) | %d |\n", analysis.FilterStats.LinesFiltered)
		fmt.Fprintf(&sb, "| Lines Kept | %d |\n", analysis.FilterStats.LinesKept)

		filterPercentage := calculateSavings(analysis.FilterStats.LinesTotal, analysis.FilterStats.LinesKept)
		fmt.Fprintf(&sb, "| Filter Reduction | %.1f%% |\n", filterPercentage)

		// Estimate cost savings (approximate tokens saved)
		// Rough estimate: each filtered line would have used ~20 tokens on average
		estimatedTokensSaved := analysis.FilterStats.LinesFiltered * 20
		fmt.Fprintf(&sb, "| Est. Tokens Saved | ~%d |\n", estimatedTokensSaved)

		sb.WriteString("\n**Cost Impact:** By filtering log lines before LLM processing, ")
		fmt.Fprintf(&sb, "approximately %d tokens were saved. ", estimatedTokensSaved)
		sb.WriteString("This reduces API costs and improves processing speed.\n\n")
	}

	// Statistics Section
	sb.WriteString("## 📊 Statistics\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	fmt.Fprintf(&sb, "| Original Logs | %d |\n", analysis.OriginalCount)
	fmt.Fprintf(&sb, "| Processed Logs | %d |\n", analysis.ProcessedCount)
	if analysis.Deduplicated {
		fmt.Fprintf(&sb, "| Deduplication | %.1f%% |\n", calculateSavings(analysis.OriginalCount, analysis.ProcessedCount))
	}
	fmt.Fprintf(&sb, "| Tokens | %d |\n", analysis.TokensUsed)
	fmt.Fprintf(&sb, "| Chunks | %d |\n", analysis.ChunksUsed)

	return sb.String()
}

// SaveReport writes a report to the container's directory and returns the file path.
func SaveReport(containerName, content string, cfg *config.Config) (string, error) {
	// Create container directory inside reports dir
	containerDir := filepath.Join(cfg.Output.ReportsDir, sanitize.Name(containerName))
	if err := os.MkdirAll(containerDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create report directory: %w", err)
	}

	// Generate filename: YYYY-MM-DD_HH-MM-SS.md
	filename := time.Now().Format("2006-01-02_15-04-05") + ".md"
	filePath := filepath.Join(containerDir, filename)

	// Write file
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("failed to write report file: %w", err)
	}

	return filePath, nil
}

func calculateSavings(original, processed int) float64 {
	if original == 0 {
		return 0
	}
	return float64(original-processed) / float64(original) * 100
}
