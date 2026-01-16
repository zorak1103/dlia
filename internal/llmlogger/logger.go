// Package llmlogger provides logging functionality for LLM requests and responses.
// It creates Markdown files containing the full interaction details for debugging,
// cost auditing, and prompt engineering purposes.
package llmlogger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Logger handles logging of LLM interactions to Markdown files.
type Logger struct {
	baseDir string
	enabled bool
}

// NewLogger creates a new Logger instance.
// If enabled is false, all logging operations become no-ops.
func NewLogger(baseDir string, enabled bool) *Logger {
	return &Logger{
		baseDir: baseDir,
		enabled: enabled,
	}
}

// IsEnabled returns whether logging is enabled.
func (l *Logger) IsEnabled() bool {
	return l != nil && l.enabled
}

// LogInteraction logs an LLM interaction to a Markdown file.
// Creates a file at {baseDir}/{containerName}/{timestamp}.md
// Returns nil if logging is disabled or logger is nil.
func (l *Logger) LogInteraction(containerName, originalInput string, request, response interface{}) error {
	if !l.IsEnabled() {
		return nil
	}

	// Create container-specific directory
	containerDir := filepath.Join(l.baseDir, sanitizeFilename(containerName))
	if err := os.MkdirAll(containerDir, 0o750); err != nil {
		return fmt.Errorf("failed to create log directory %s: %w", containerDir, err)
	}

	// Generate timestamp-based filename
	timestamp := time.Now().UTC()
	filename := fmt.Sprintf("%s.md", timestamp.Format("2006-01-02T15-04-05Z"))
	filePath := filepath.Join(containerDir, filename)

	// Marshal request and response to JSON
	requestJSON, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		requestJSON = []byte(fmt.Sprintf("Error marshaling request: %v", err))
	}

	responseJSON, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		responseJSON = []byte(fmt.Sprintf("Error marshaling response: %v", err))
	}

	// Generate Markdown content
	content := formatMarkdown(containerName, timestamp, originalInput, requestJSON, responseJSON)

	// Write to file with secure permissions (0600 = owner read/write only)
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("failed to write log file %s: %w", filePath, err)
	}

	return nil
}

// formatMarkdown generates the Markdown content for an LLM interaction log.
func formatMarkdown(containerName string, timestamp time.Time, originalInput string, requestJSON, responseJSON []byte) string {
	return fmt.Sprintf(`# LLM Interaction Log

**Container**: %s
**Timestamp**: %s

## Original Input

%s

## Request Sent to LLM

`+"```json"+`
%s
`+"```"+`

## LLM Response

`+"```json"+`
%s
`+"```"+`
`, containerName, timestamp.Format(time.RFC3339), originalInput, string(requestJSON), string(responseJSON))
}

// sanitizeFilename removes or replaces characters that are invalid in filenames.
func sanitizeFilename(name string) string {
	// Replace common invalid characters with underscores
	invalid := []rune{'/', '\\', ':', '*', '?', '"', '<', '>', '|'}
	result := []rune(name)
	for i, r := range result {
		for _, inv := range invalid {
			if r == inv {
				result[i] = '_'
				break
			}
		}
	}
	return string(result)
}
