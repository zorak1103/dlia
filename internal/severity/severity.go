// Package severity defines the severity levels for container log analysis results
// and provides parsing utilities for extracting severity from LLM responses.
package severity

import (
	"fmt"
	"regexp"
	"strings"
)

// Level represents a log analysis severity level.
type Level int

const (
	// OK indicates routine activity with nothing that needs attention.
	OK Level = iota
	// Warning indicates degraded behaviour or repeated errors worth a look soon.
	Warning
	// Unknown is returned when no valid SEVERITY line was found or the value was unrecognised.
	Unknown
	// Critical indicates failures, crashes, data loss or anything that needs action now.
	Critical
)

// FailedLabel is the display label for containers whose analysis failed (not a Level).
const FailedLabel = "⚪ Analysis failed"

// Instruction is appended to LLM prompts so the model includes a parseable severity line.
const Instruction = `End your answer with exactly one final line in this format:
SEVERITY: <level>
where <level> is one of:
- critical: failures, crashes, data loss, security incidents or anything that needs action now
- warning: degraded behaviour, repeated errors or anything worth a look soon
- ok: routine activity, nothing that needs attention
Write nothing after that line.`

// String returns the lowercase name of the level.
func (l Level) String() string {
	switch l {
	case OK:
		return "ok"
	case Warning:
		return "warning"
	case Unknown:
		return "unknown"
	case Critical:
		return "critical"
	default:
		return "unknown"
	}
}

// Badge returns a coloured emoji label for the level.
func (l Level) Badge() string {
	switch l {
	case OK:
		return "🟢 OK"
	case Warning:
		return "🟡 Warning"
	case Unknown:
		return "🟡 Unknown"
	case Critical:
		return "🔴 Critical"
	default:
		return "🟡 Unknown"
	}
}

// Max returns the highest severity level from the provided list.
// Returns OK when called with no arguments.
func Max(levels ...Level) Level {
	result := OK
	for _, l := range levels {
		if l > result {
			result = l
		}
	}
	return result
}

// ParseThreshold parses a threshold string for notification filtering.
// Accepted values (case-insensitive, trimmed): ok, warning, critical.
// Returns an error for unknown or empty values.
func ParseThreshold(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "ok":
		return OK, nil
	case "warning":
		return Warning, nil
	case "critical":
		return Critical, nil
	default:
		return OK, fmt.Errorf("invalid severity threshold %q: must be one of ok, warning, critical", s)
	}
}

// severityLineRe matches a standalone SEVERITY line with optional Markdown decoration.
// It is anchored so that inline mentions (e.g. "The SEVERITY: ok marker") are not matched.
// Decoration (bold/italic/backtick markers) is allowed between the word and the colon,
// and a trailing period is permitted after the value.
var severityLineRe = regexp.MustCompile(`(?i)^\s*[*_` + "`" + `]*\s*SEVERITY\s*[*_` + "`" + `]*\s*:\s*[*_` + "`" + `]*\s*([a-z]+)\s*[*_` + "`" + `.]*\s*$`)

// looseSeverityRe matches any line that looks like an attempt at a severity line, including
// malformed ones (e.g. "SEVERITY: critical (disk full)" or "Severity Level: critical").
var looseSeverityRe = regexp.MustCompile(`(?i)^\s*[*_` + "`" + `]*\s*SEVERITY\b[^:\n]{0,20}:`)

// parseValue maps a raw level string to a Level, returning (level, true) on success.
func parseValue(raw string) (Level, bool) {
	switch strings.ToLower(raw) {
	case "ok":
		return OK, true
	case "warning":
		return Warning, true
	case "critical":
		return Critical, true
	default:
		return Unknown, false
	}
}

// Parse scans text for a SEVERITY line and returns the level and cleaned text.
//
// Only the last matching line is removed. If no line matches, Unknown is returned
// and the original text is returned unchanged. If the last match has an unrecognised
// value, Unknown is returned and that line is still removed. If a malformed
// severity-like line appears after the last strict match (or no strict match exists),
// Unknown is returned and the text is left unchanged so the malformed line stays
// visible. Trailing whitespace is trimmed from the returned text (but only when a
// SEVERITY line was found and removed).
func Parse(text string) (level Level, cleaned string) {
	lines := strings.Split(text, "\n")

	lastMatchIdx := -1
	lastLooseIdx := -1
	var lastRaw string

	for i, line := range lines {
		// Trim a trailing \r for CRLF input before matching.
		trimmed := strings.TrimRight(line, "\r")
		if looseSeverityRe.MatchString(trimmed) {
			lastLooseIdx = i
		}
		m := severityLineRe.FindStringSubmatch(trimmed)
		if m != nil {
			lastMatchIdx = i
			lastRaw = m[1]
		}
	}

	if lastMatchIdx == -1 || lastLooseIdx > lastMatchIdx {
		// No well-formed final SEVERITY line; return original text unchanged.
		return Unknown, text
	}

	level, _ = parseValue(lastRaw)

	// Remove the last matching line and rejoin.
	kept := make([]string, 0, len(lines)-1)
	kept = append(kept, lines[:lastMatchIdx]...)
	kept = append(kept, lines[lastMatchIdx+1:]...)
	cleaned = strings.TrimRight(strings.Join(kept, "\n"), " \t\r\n")

	return level, cleaned
}
