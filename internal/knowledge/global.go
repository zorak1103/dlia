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
	"github.com/zorak1103/dlia/internal/severity"
)

// ServiceOutcome bundles an analysis result with the path to its saved report.
// A nil Result means the analysis failed.
type ServiceOutcome struct {
	Result     *chunking.AnalyzeResult // nil = analysis failed
	ReportPath string                  // "" = no report saved
}

// UpdateGlobalSummary writes the main dashboard markdown file aggregating all outcomes.
func UpdateGlobalSummary(outcomes map[string]ServiceOutcome, cfg *config.Config) error {
	if err := os.MkdirAll(cfg.Output.KnowledgeBaseDir, 0o750); err != nil {
		return fmt.Errorf("failed to create KB directory: %w", err)
	}

	content := buildGlobalSummaryContent(outcomes, cfg.Output.KnowledgeBaseDir)
	filePath := filepath.Join(cfg.Output.KnowledgeBaseDir, "global_summary.md")

	return os.WriteFile(filePath, []byte(content), 0o600)
}

func buildGlobalSummaryContent(outcomes map[string]ServiceOutcome, kbDir string) string {
	var sb strings.Builder

	writeHeader(&sb)
	writeHealthOverview(&sb, outcomes)

	sortedKeys := sortedNames(outcomes)
	writeServiceStatusTable(&sb, outcomes, sortedKeys)
	writeAttentionSection(&sb, outcomes, sortedKeys, kbDir)

	return sb.String()
}

func writeHeader(sb *strings.Builder) {
	timestamp := time.Now().Format(time.RFC1123)
	sb.WriteString("# 🌍 Global System Summary\n\n")
	fmt.Fprintf(sb, "**Last Updated:** %s\n\n", timestamp)
}

func writeHealthOverview(sb *strings.Builder, outcomes map[string]ServiceOutcome) {
	needAttention := countNeedingAttention(outcomes)

	healthStatus := "🟢 All Systems Operational"
	if needAttention > 0 {
		healthStatus = fmt.Sprintf("⚠️ %d Service(s) Need Attention", needAttention)
	}

	fmt.Fprintf(sb, "## System Health: %s\n\n", healthStatus)
}

func countNeedingAttention(outcomes map[string]ServiceOutcome) int {
	count := 0
	for _, o := range outcomes {
		if o.Result == nil || o.Result.Severity >= severity.Warning {
			count++
		}
	}
	return count
}

func writeServiceStatusTable(sb *strings.Builder, outcomes map[string]ServiceOutcome, sortedKeys []string) {
	sb.WriteString("## Service Status\n\n")
	sb.WriteString("| Service | Status | Last Analysis |\n")
	sb.WriteString("|---------|--------|---------------|\n")

	for _, name := range sortedKeys {
		o := outcomes[name]
		if o.Result == nil {
			fmt.Fprintf(sb, "| %s | %s | – |\n", name, severity.FailedLabel)
		} else {
			summary := extractSummary(o.Result.Analysis)
			fmt.Fprintf(sb, "| %s | %s | %s |\n", name, o.Result.Severity.Badge(), summary)
		}
	}
}

// attentionRank returns the sort rank for a service in the attention list.
// Critical=0, Unknown=1, Warning=2, failed=3. OK returns 4 (excluded).
func attentionRank(o ServiceOutcome) int {
	if o.Result == nil {
		return 3
	}
	switch o.Result.Severity {
	case severity.Critical:
		return 0
	case severity.Unknown:
		return 1
	case severity.Warning:
		return 2
	case severity.OK:
		return 4
	default:
		return 4
	}
}

func writeAttentionSection(sb *strings.Builder, outcomes map[string]ServiceOutcome, sortedKeys []string, kbDir string) {
	sb.WriteString("\n## Services Needing Attention\n\n")

	type attentionEntry struct {
		name string
		rank int
		o    ServiceOutcome
	}

	var entries []attentionEntry
	for _, name := range sortedKeys {
		o := outcomes[name]
		rank := attentionRank(o)
		if rank < 4 {
			entries = append(entries, attentionEntry{name: name, rank: rank, o: o})
		}
	}

	if len(entries) == 0 {
		sb.WriteString("*No services need attention.*\n")
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].rank != entries[j].rank {
			return entries[i].rank < entries[j].rank
		}
		return entries[i].name < entries[j].name
	})

	for _, e := range entries {
		var badge string
		if e.o.Result == nil {
			badge = severity.FailedLabel
		} else {
			badge = e.o.Result.Severity.Badge()
		}

		link := reportLink(e.o.ReportPath, kbDir)
		if link != "" {
			fmt.Fprintf(sb, "- %s **%s** – [latest report](%s)\n", badge, e.name, link)
		} else {
			fmt.Fprintf(sb, "- %s **%s**\n", badge, e.name)
		}
	}
}

// reportLink builds a relative forward-slash link from kbDir to reportPath (both made absolute first),
// wrapped in angle brackets for paths that may contain spaces.
// Returns "" when reportPath is empty.
func reportLink(reportPath, kbDir string) string {
	if reportPath == "" {
		return ""
	}
	absReport, err := filepath.Abs(reportPath)
	if err != nil {
		return "<" + filepath.ToSlash(reportPath) + ">"
	}
	absKB, err := filepath.Abs(kbDir)
	if err != nil {
		return "<" + filepath.ToSlash(absReport) + ">"
	}
	rel, err := filepath.Rel(absKB, absReport)
	if err != nil {
		rel = absReport
	}
	return "<" + filepath.ToSlash(rel) + ">"
}

func sortedNames(outcomes map[string]ServiceOutcome) []string {
	keys := make([]string, 0, len(outcomes))
	for k := range outcomes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
