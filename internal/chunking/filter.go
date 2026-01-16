package chunking

import (
	"fmt"
	"regexp"
)

// RegexpFilter provides regexp-based filtering of log lines before LLM processing.
// This reduces token costs by excluding irrelevant entries early in the pipeline.
type RegexpFilter struct {
	patterns []*regexp.Regexp
}

// NewRegexpFilter creates a new RegexpFilter from string patterns.
// Patterns are compiled during construction to avoid repeated compilation.
// Returns an error if any pattern fails to compile.
func NewRegexpFilter(patterns []string) (*RegexpFilter, error) {
	if len(patterns) == 0 {
		return &RegexpFilter{patterns: nil}, nil
	}

	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for i, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to compile pattern at index %d (%q): %w", i, pattern, err)
		}
		compiled = append(compiled, re)
	}

	return &RegexpFilter{patterns: compiled}, nil
}

// Filter applies regexp patterns to log lines, excluding lines that match any pattern.
// Returns the filtered logs (lines that did NOT match) and statistics about the operation.
// If no patterns are configured, all logs are kept and stats reflect zero filtering.
func (rf *RegexpFilter) Filter(logs []string) ([]string, FilterStats) {
	stats := FilterStats{
		LinesTotal: len(logs),
	}

	// If no patterns configured, keep all logs
	if len(rf.patterns) == 0 {
		stats.LinesKept = stats.LinesTotal
		return logs, stats
	}

	filtered := make([]string, 0, len(logs))

	for _, log := range logs {
		matched := false
		// Check if log matches any pattern
		for _, pattern := range rf.patterns {
			if pattern.MatchString(log) {
				matched = true
				stats.LinesFiltered++
				break // No need to check other patterns
			}
		}

		// Keep the log if it didn't match any pattern
		if !matched {
			filtered = append(filtered, log)
		}
	}

	stats.LinesKept = len(filtered)
	return filtered, stats
}

// MatchesAny checks if the given text matches any of the configured patterns.
// Returns true if a match is found, false otherwise. Returns false if no patterns are configured.
func (rf *RegexpFilter) MatchesAny(text string) bool {
	for _, pattern := range rf.patterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

// FilterStats tracks statistics about the filtering operation.
type FilterStats struct {
	LinesTotal    int // Total number of input lines
	LinesFiltered int // Number of lines matched and filtered out
	LinesKept     int // Number of lines kept (Total - Filtered)
}
