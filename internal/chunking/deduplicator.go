package chunking

import (
	"fmt"
	"strings"

	"github.com/zorak1103/dlia/internal/docker"
)

// DeduplicateThreshold is the minimum number of repeats to deduplicate
const DeduplicateThreshold = 3

// Deduplicate reduces repeated consecutive log lines into [REPEAT x...] markers.
//
// Algorithm:
// Uses a single-pass scan tracking the start of each sequence of identical messages.
// When a different message is encountered (or end of input), the accumulated sequence
// is "flushed" - either as a single deduplicated entry (if count >= threshold) or
// as individual entries (if below threshold).
//
// Threshold Rationale:
// The threshold of 3 (DeduplicateThreshold) balances deduplication benefit vs. information loss.
// - Duplicates < 3: Kept as-is (losing 2 lines saves minimal space, may hide patterns)
// - Duplicates >= 3: Collapsed to [REPEAT xN] marker (significant space/token savings)
//
// Complexity:
//   - Time:  O(n) where n is the number of log entries. Each entry is visited at most twice.
//   - Space: O(n) in the worst case (no duplicates), O(1) additional working memory.
func Deduplicate(logs []docker.LogEntry) []docker.LogEntry {
	n := len(logs)
	if n == 0 {
		return logs
	}

	var result []docker.LogEntry
	seqStart := 0

	flushSequence := func(endIdx int) {
		seqLen := endIdx - seqStart
		firstEntry := logs[seqStart]
		if seqLen >= DeduplicateThreshold {
			result = append(result, docker.LogEntry{
				Timestamp: firstEntry.Timestamp,
				Stream:    firstEntry.Stream,
				Message:   fmt.Sprintf("[REPEAT x%d] %s", seqLen, firstEntry.Message),
			})
		} else {
			for j := seqStart; j < endIdx; j++ {
				result = append(result, logs[j])
			}
		}
	}

	for i := 1; i < n; i++ {
		if logs[i].Message != logs[seqStart].Message {
			flushSequence(i)
			seqStart = i
		}
	}
	// Flush the final sequence
	flushSequence(n)

	return result
}

// FormatLogs converts log entries to a timestamp-prefixed string format
// suitable for LLM analysis. Format: "[timestamp] message\n" per entry.
// Entries without timestamps are formatted as "message\n".
func FormatLogs(logs []docker.LogEntry) string {
	var sb strings.Builder
	for _, entry := range logs {
		if entry.Timestamp != "" {
			sb.WriteString(fmt.Sprintf("[%s] %s\n", entry.Timestamp, entry.Message))
		} else {
			sb.WriteString(entry.Message + "\n")
		}
	}
	return sb.String()
}

// GetDeduplicationStats returns statistics about deduplication
func GetDeduplicationStats(original, deduplicated []docker.LogEntry) (originalCount, deduplicatedCount, savedCount int) {
	return len(original), len(deduplicated), len(original) - len(deduplicated)
}
