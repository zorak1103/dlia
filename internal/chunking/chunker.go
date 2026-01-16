// Package chunking provides utilities for processing log entries including
// deduplication, chunking for token-limited contexts, and tokenization.
package chunking

import (
	"fmt"

	"github.com/zorak1103/dlia/internal/docker"
)

// Chunk represents a chunk of logs
type Chunk struct {
	Logs       []docker.LogEntry
	TokenCount int
	Index      int
	Total      int
}

// ChunkLogs splits logs into token-sized chunks suitable for token-limited LLM contexts.
// It ensures no chunk exceeds maxTokensPerChunk while preserving log entry boundaries.
// Each log entry is kept whole - entries are never split across chunks.
//
// Example usage:
//
//	logs := []docker.LogEntry{
//	    {Timestamp: "2024-01-15T10:00:00Z", Message: "Application started"},
//	    {Timestamp: "2024-01-15T10:00:01Z", Message: "Processing request..."},
//	    {Timestamp: "2024-01-15T10:00:02Z", Message: "Request completed"},
//	}
//	tokenizer := NewSimpleTokenizer()
//	chunks := ChunkLogs(logs, 1000, tokenizer)
//	for _, chunk := range chunks {
//	    fmt.Printf("Chunk %d/%d: %d tokens, %d logs\n",
//	        chunk.Index+1, chunk.Total, chunk.TokenCount, len(chunk.Logs))
//	}
//
// Returns an empty slice if logs is empty. Each chunk includes Index (0-based) and Total for progress tracking.
func ChunkLogs(logs []docker.LogEntry, maxTokensPerChunk int, tokenizer TokenizerInterface) []Chunk {
	if len(logs) == 0 {
		return []Chunk{}
	}

	var chunks []Chunk
	currentChunk := []docker.LogEntry{}
	currentTokens := 0

	for _, log := range logs {
		// Format log entry
		logText := fmt.Sprintf("[%s] %s\n", log.Timestamp, log.Message)
		logTokens := tokenizer.CountTokens(logText)

		// Check if adding this log would exceed limit
		if currentTokens+logTokens > maxTokensPerChunk && len(currentChunk) > 0 {
			// Save current chunk
			chunks = append(chunks, Chunk{
				Logs:       currentChunk,
				TokenCount: currentTokens,
			})

			// Start new chunk
			currentChunk = []docker.LogEntry{log}
			currentTokens = logTokens
		} else {
			// Add to current chunk
			currentChunk = append(currentChunk, log)
			currentTokens += logTokens
		}
	}

	// Add final chunk
	if len(currentChunk) > 0 {
		chunks = append(chunks, Chunk{
			Logs:       currentChunk,
			TokenCount: currentTokens,
		})
	}

	// Set index and total for each chunk
	total := len(chunks)
	for i := range chunks {
		chunks[i].Index = i
		chunks[i].Total = total
	}

	return chunks
}

// FormatChunk converts a chunk to a formatted string with timestamps.
// Each log entry is formatted as "[timestamp] message\n". Entries without
// timestamps are formatted as "message\n".
//
// Example usage:
//
//	chunk := Chunk{
//	    Logs: []docker.LogEntry{
//	        {Timestamp: "2024-01-15T10:00:00Z", Message: "Starting process"},
//	        {Timestamp: "2024-01-15T10:00:01Z", Message: "Process complete"},
//	    },
//	    TokenCount: 45,
//	    Index: 0,
//	    Total: 1,
//	}
//	formatted := FormatChunk(chunk)
//	// Output:
//	// [2024-01-15T10:00:00Z] Starting process
//	// [2024-01-15T10:00:01Z] Process complete
//
// The returned string is ready for LLM analysis or report generation.
func FormatChunk(chunk Chunk) string {
	return FormatLogs(chunk.Logs)
}
