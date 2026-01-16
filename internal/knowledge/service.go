package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zorak1103/dlia/internal/chunking"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/sanitize"
)

// UpdateServiceKB appends analysis results to the container's knowledge base file.
func UpdateServiceKB(containerName string, analysis *chunking.AnalyzeResult, cfg *config.Config) error {
	kbDir := filepath.Join(cfg.Output.KnowledgeBaseDir, "services")
	if err := os.MkdirAll(kbDir, 0o750); err != nil {
		return fmt.Errorf("failed to create KB services directory: %w", err)
	}

	filePath := filepath.Clean(filepath.Join(kbDir, sanitize.Name(containerName)+".md"))

	// Determine status based on analysis content (simple heuristic)
	status := "🟢 Healthy"
	if strings.Contains(strings.ToLower(analysis.Analysis), "critical") ||
		strings.Contains(strings.ToLower(analysis.Analysis), "error") {
		status = "🔴 Issues Detected"
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
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
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

	for _, entry := range strings.Split(entriesSection, "---\n") {
		if strings.TrimSpace(entry) == "" {
			continue
		}

		if !isEntryExpired(entry, cutoff) {
			builder.WriteString(entry)
			builder.WriteString("---\n")
		}
	}

	return builder.String()
}

func isEntryExpired(entry string, cutoff time.Time) bool {
	timestamp := extractEntryTimestamp(entry)
	if timestamp == "" {
		return false
	}

	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return false
	}

	return t.Before(cutoff)
}

func extractEntryTimestamp(entry string) string {
	for _, line := range strings.Split(entry, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "### Scan:") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "### Scan:"))
		}
	}
	return ""
}
