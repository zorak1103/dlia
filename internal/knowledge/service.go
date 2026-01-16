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
	parts := strings.Split(content, "\n---\n")
	if len(parts) == 0 {
		return content
	}

	cutoff := time.Now().Add(-retention)

	// We will rebuild the content
	var newContentBuilder strings.Builder

	// Find where the entries start. They start after "## Service History\n"
	headerEnd := strings.Index(content, "## Service History\n")
	if headerEnd == -1 {
		// Fallback: just return content if structure is unexpected
		return content
	}

	// Keep the header
	headerSection := content[:headerEnd+len("## Service History\n")]
	newContentBuilder.WriteString(headerSection)

	entriesSection := content[headerEnd+len("## Service History\n"):]

	// Split entries by the separator used in UpdateServiceKB: "\n---\n"
	// Note: newEntry := ... + "---\n"
	// So entries are separated by "---\n" effectively.

	rawEntries := strings.Split(entriesSection, "---\n")

	for _, entry := range rawEntries {
		if strings.TrimSpace(entry) == "" {
			continue
		}

		// Extract timestamp
		// Format: "\n### Scan: 2023-10-25T..."
		lines := strings.Split(entry, "\n")
		var timestampStr string
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "### Scan:") {
				timestampStr = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "### Scan:"))
				break
			}
		}

		if timestampStr != "" {
			t, err := time.Parse(time.RFC3339, timestampStr)
			if err == nil {
				if t.Before(cutoff) {
					continue // Skip old entry
				}
			}
		}

		// Keep entry
		newContentBuilder.WriteString(entry)
		newContentBuilder.WriteString("---\n")
	}

	return newContentBuilder.String()
}
