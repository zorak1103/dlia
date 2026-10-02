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

// openFenceRe matches an opening code fence, with optional info string
// (language tag), optionally indented.
var openFenceRe = regexp.MustCompile("^[ \\t]*```[ \\t]*[A-Za-z0-9_-]*[ \\t]*$")

// closeFenceRe matches a bare closing code fence.
var closeFenceRe = regexp.MustCompile("^[ \\t]*```[ \\t]*$")

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

// Parse scans text for a SEVERITY line and returns the level, the cleaned
// text, and whether a line was found and removed.
//
// Only the last matching line is removed. If no line matches, Unknown is returned
// and the original text is returned unchanged. If the last match has an unrecognised
// value, Unknown is returned and that line is still removed. If a malformed
// severity-like line appears after the last strict match (or no strict match exists),
// Unknown is returned and the text is left unchanged so the malformed line stays
// visible. Trailing whitespace is trimmed from the returned text (but only when a
// SEVERITY line was found and removed).
//
// When the removed severity line was wrapped in an empty code fence pair that
// trails the text (only blank lines inside the pair and after the closing fence),
// the whole pair is removed as well. Fences with real content between them,
// unclosed fences, and a closing fence without an opening one are left as-is.
//
// found is true exactly when the strict-match branch removed a line — including
// an unrecognised value (e.g. "SEVERITY: banana") — and false otherwise. It is
// deliberately not derived from the level, which is Unknown for removed but
// unrecognised values.
func Parse(text string) (level Level, cleaned string, found bool) {
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
		return Unknown, text, false
	}

	level, _ = parseValue(lastRaw)

	// Remove the last matching line; also drop a now-empty fence pair directly
	// around it when the pair trails the text.
	start, end, fenced := emptyFenceExtent(lines, lastMatchIdx)
	if !fenced {
		start, end = lastMatchIdx, lastMatchIdx
	}

	kept := make([]string, 0, len(lines)-(end-start+1))
	kept = append(kept, lines[:start]...)
	kept = append(kept, lines[end+1:]...)
	cleaned = strings.TrimRight(strings.Join(kept, "\n"), " \t\r\n")

	return level, cleaned, true
}

// emptyFenceExtent reports the removal extent for an empty code fence directly
// surrounding the removed SEVERITY line at sevIdx: it returns the opening and
// closing fence indices when both exist, only whitespace-only lines sit between
// them, and the closing fence is the last non-blank line.
func emptyFenceExtent(lines []string, sevIdx int) (start, end int, ok bool) {
	blank := func(line string) bool {
		return strings.TrimSpace(strings.TrimRight(line, "\r")) == ""
	}

	// Extend downward from sevIdx over blank lines; the next non-blank line must
	// match closeFenceRe, and only blank lines may follow it ("immediately trailing").
	closingIdx := -1
	for i := sevIdx + 1; i < len(lines); i++ {
		if blank(lines[i]) {
			continue
		}
		if closeFenceRe.MatchString(strings.TrimRight(lines[i], "\r")) {
			closingIdx = i
		}
		break
	}
	if closingIdx >= 0 {
		for i := closingIdx + 1; i < len(lines); i++ {
			if !blank(lines[i]) {
				return 0, 0, false
			}
		}
	}
	if closingIdx == -1 {
		return 0, 0, false
	}

	// Extend upward from sevIdx over blank lines; the previous non-blank line
	// must match openFenceRe (bare ``` or ```text).
	for i := sevIdx - 1; i >= 0; i-- {
		if blank(lines[i]) {
			continue
		}
		if openFenceRe.MatchString(strings.TrimRight(lines[i], "\r")) {
			return i, closingIdx, true
		}
		return 0, 0, false
	}

	return 0, 0, false
}
