package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/pathologize"

	"github.com/zorak1103/dlia/internal/chunking"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/sanitize"
)

const (
	statusHealthy        = "🟢 Healthy"
	statusIssuesDetected = "🔴 Issues Detected"
)

// UpdateServiceKB appends analysis results to the container's knowledge base file.
func UpdateServiceKB(containerName string, analysis *chunking.AnalyzeResult, cfg *config.Config) error {
	kbDir := filepath.Join(cfg.Output.KnowledgeBaseDir, "services")
	if err := os.MkdirAll(kbDir, 0o750); err != nil {
		return fmt.Errorf("failed to create KB services directory: %w", err)
	}

	filePath := pathologize.Join(kbDir, sanitize.Name(containerName)+".md")

	// Determine status based on analysis content (simple heuristic)
	status := statusHealthy
	if strings.Contains(strings.ToLower(analysis.Analysis), "critical") ||
		strings.Contains(strings.ToLower(analysis.Analysis), "error") {
		status = statusIssuesDetected
	} else if strings.Contains(strings.ToLower(analysis.Analysis), "warning") {
		status = "🟡 Warnings"
	}

	timestamp := time.Now().Format(time.RFC3339)

	// Prepare new entry
	newEntry := fmt.Sprintf("\n### Scan: %s\n", timestamp)
	newEntry += fmt.Sprintf("**Status:** %s\n\n", status)
	newEntry += analysis.Analysis + "\n\n"
	newEntry += "---\n"

	// Read existing file or create header
	// Path is safe: constructed from config dir + sanitized container name
	var content string
	if data, err := os.ReadFile(filePath); err == nil {
		content = string(data)
	} else {
		content = fmt.Sprintf("# Knowledge Base: %s\n\n", containerName)
		content += "## Service History\n"
	}

	// Prune old entries using configured retention period
	retentionDuration := time.Duration(cfg.Output.KnowledgeRetentionDays) * 24 * time.Hour
	content = pruneEntries(content, retentionDuration)

	// Append new entry
	content += newEntry

	// Write back
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil { //nolint:gosec // path is constructed from config dir + sanitized container name via internal/sanitize
		return fmt.Errorf("failed to write KB file: %w", err)
	}

	return nil
}

func pruneEntries(content string, retention time.Duration) string {
	const headerMarker = "## Service History\n"

	headerEnd := strings.Index(content, headerMarker)
	if headerEnd == -1 {
		return content
	}

	cutoff := time.Now().Add(-retention)
	headerSection := content[:headerEnd+len(headerMarker)]
	entriesSection := content[headerEnd+len(headerMarker):]

	var builder strings.Builder
	builder.WriteString(headerSection)

	for _, entry := range splitEntries(entriesSection) {
		if !isEntryExpired(entry, cutoff) {
			builder.WriteString(entry)
		}
	}

	return builder.String()
}

// scanHeadingRe matches the heading line that starts a knowledge base entry.
var scanHeadingRe = regexp.MustCompile(`(?m)^### Scan:[ \t]+(\S+)[ \t\r]*$`)

type scanBoundary struct {
	start     int
	timestamp time.Time
}

// scanBoundaries returns the entry boundaries in s: heading lines whose
// timestamp parses as RFC3339. Other "### Scan:" lines are ordinary text.
func scanBoundaries(s string) []scanBoundary {
	var boundaries []scanBoundary
	for _, m := range scanHeadingRe.FindAllStringSubmatchIndex(s, -1) {
		ts, err := time.Parse(time.RFC3339, s[m[2]:m[3]])
		if err != nil {
			continue
		}
		boundaries = append(boundaries, scanBoundary{start: m[0], timestamp: ts})
	}
	return boundaries
}

// splitEntries splits the entries section into entries. An entry starts at a
// timestamped scan heading and runs to the next one, so "---" lines inside an
// analysis do not split it. Blank lines directly before a heading belong to
// that entry; any other text before the first heading is dropped.
func splitEntries(entriesSection string) []string {
	boundaries := scanBoundaries(entriesSection)
	entries := make([]string, 0, len(boundaries))
	starts := make([]int, len(boundaries))
	for i, b := range boundaries {
		starts[i] = includeLeadingBlankLines(entriesSection, b.start)
	}
	for i, start := range starts {
		end := len(entriesSection)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		entries = append(entries, entriesSection[start:end])
	}
	return entries
}

// includeLeadingBlankLines moves pos, the start of a line, back over any
// whitespace-only lines directly before it.
func includeLeadingBlankLines(s string, pos int) int {
	for pos > 0 {
		prevStart := strings.LastIndex(s[:pos-1], "\n") + 1
		if strings.TrimSpace(s[prevStart:pos-1]) != "" {
			break
		}
		pos = prevStart
	}
	return pos
}

func isEntryExpired(entry string, cutoff time.Time) bool {
	boundaries := scanBoundaries(entry)
	if len(boundaries) == 0 {
		return false
	}

	return boundaries[0].timestamp.Before(cutoff)
}
