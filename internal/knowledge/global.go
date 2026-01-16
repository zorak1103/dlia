// Package knowledge manages the knowledge base and global summaries.
package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zorak1103/dlia/internal/chunking"
	"github.com/zorak1103/dlia/internal/config"
)

// UpdateGlobalSummary updates the main dashboard summary by aggregating
// analysis results from all services into a single markdown file.
func UpdateGlobalSummary(results map[string]*chunking.AnalyzeResult, cfg *config.Config) error {
	if err := os.MkdirAll(cfg.Output.KnowledgeBaseDir, 0o750); err != nil {
		return fmt.Errorf("failed to create KB directory: %w", err)
	}

	sortedKeys := sortedServiceNames(results)
	content := buildGlobalSummaryContent(results, sortedKeys)

	filePath := filepath.Join(cfg.Output.KnowledgeBaseDir, "global_summary.md")

	return os.WriteFile(filePath, []byte(content), 0o600)
}

// sortedServiceNames returns service names sorted alphabetically for consistent output.
func sortedServiceNames(results map[string]*chunking.AnalyzeResult) []string {
	keys := make([]string, 0, len(results))
	for k := range results {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return keys
}

// buildGlobalSummaryContent assembles the complete markdown content for the global summary.
func buildGlobalSummaryContent(results map[string]*chunking.AnalyzeResult, sortedKeys []string) string {
	var sb strings.Builder

	writeHeader(&sb)
	writeHealthOverview(&sb, results)
	writeServiceStatusTable(&sb, results, sortedKeys)
	writeCriticalIssuesSection(&sb, results, sortedKeys)

	return sb.String()
}

// writeHeader writes the document title and timestamp.
func writeHeader(sb *strings.Builder) {
	timestamp := time.Now().Format(time.RFC1123)
	sb.WriteString("# 🌍 Global System Summary\n\n")
	fmt.Fprintf(sb, "**Last Updated:** %s\n\n", timestamp)
}

// writeHealthOverview writes the system health status section.
func writeHealthOverview(sb *strings.Builder, results map[string]*chunking.AnalyzeResult) {
	issueCount := countServicesWithIssues(results)

	healthStatus := "🟢 All Systems Operational"
	if issueCount > 0 {
		healthStatus = fmt.Sprintf("⚠️ %d Service(s) Reporting Issues", issueCount)
	}

	fmt.Fprintf(sb, "## System Health: %s\n\n", healthStatus)
}

// countServicesWithIssues returns the number of services that have critical issues.
func countServicesWithIssues(results map[string]*chunking.AnalyzeResult) int {
	count := 0
	for _, res := range results {
		if hasIssues(res.Analysis) {
			count++
		}
	}

	return count
}

// writeServiceStatusTable writes the service status table in markdown format.
func writeServiceStatusTable(sb *strings.Builder, results map[string]*chunking.AnalyzeResult, sortedKeys []string) {
	sb.WriteString("## Service Status\n\n")
	sb.WriteString("| Service | Status | Last Analysis |\n")
	sb.WriteString("|---------|--------|---------------|\n")

	for _, name := range sortedKeys {
		res := results[name]
		status := determineServiceStatus(res.Analysis)
		summary := extractSummary(res.Analysis)
		fmt.Fprintf(sb, "| %s | %s | %s |\n", name, status, summary)
	}
}

// determineServiceStatus returns an emoji status indicator based on analysis content.
func determineServiceStatus(analysis string) string {
	switch {
	case hasIssues(analysis):
		return "🔴 Issues"
	case hasWarnings(analysis):
		return "🟡 Warning"
	default:
		return "🟢 OK"
	}
}

// writeCriticalIssuesSection writes the critical issues section listing all services with problems.
func writeCriticalIssuesSection(sb *strings.Builder, results map[string]*chunking.AnalyzeResult, sortedKeys []string) {
	sb.WriteString("\n## Recent Critical Issues\n\n")

	hasCritical := false

	for _, name := range sortedKeys {
		res := results[name]
		if hasIssues(res.Analysis) {
			hasCritical = true
			fmt.Fprintf(sb, "### %s\n", name)
			sb.WriteString(extractErrors(res.Analysis))
			sb.WriteString("\n")
		}
	}

	if !hasCritical {
		sb.WriteString("*No critical issues reported in the last scan.*")
	}
}

// hasIssues checks if the analysis contains critical errors.
func hasIssues(analysis string) bool {
	lower := strings.ToLower(analysis)

	return strings.Contains(lower, "critical") || strings.Contains(lower, "error")
}

// hasWarnings checks if the analysis contains warnings.
func hasWarnings(analysis string) bool {
	return strings.Contains(strings.ToLower(analysis), "warning")
}

// extractSummary extracts a brief summary from the analysis text.
func extractSummary(analysis string) string {
	lines := strings.Split(analysis, "\n")
	for _, line := range lines {
		if strings.Contains(line, "**Summary**") {
			return strings.TrimSpace(strings.Replace(line, "**Summary**:", "", 1))
		}
	}

	if len(lines) > 0 {
		return truncate(lines[0], 50)
	}

	return "No summary available"
}

// extractErrors extracts the errors section from the analysis text.
func extractErrors(analysis string) string {
	start := strings.Index(analysis, "**Errors**")
	if start == -1 {
		return "Issues detected but could not parse specific errors."
	}

	end := strings.Index(analysis[start:], "**Warnings**")
	if end == -1 {
		end = len(analysis) - start
	}

	return strings.TrimSpace(analysis[start : start+end])
}

// truncate shortens a string to maxLen characters, adding ellipsis if truncated.
func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}

	return s
}
