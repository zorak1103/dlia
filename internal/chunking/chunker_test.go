package chunking

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorak1103/dlia/internal/docker"
)

func TestChunkLogs(t *testing.T) {
	tests := []struct {
		name           string
		logs           []docker.LogEntry
		maxTokens      int
		tokensPerChar  float64
		wantEmpty      bool
		wantChunkCount int
		minChunkCount  int
		validateFunc   func(t *testing.T, chunks []Chunk, logs []docker.LogEntry)
	}{
		{
			name:          "empty logs",
			logs:          []docker.LogEntry{},
			maxTokens:     100,
			tokensPerChar: 0.1,
			wantEmpty:     true,
		},
		{
			name: "single log",
			logs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Single log"},
			},
			maxTokens:      100,
			tokensPerChar:  0.1,
			wantChunkCount: 1,
			validateFunc: func(t *testing.T, chunks []Chunk, _ []docker.LogEntry) {
				t.Helper()
				require.Len(t, chunks, 1)
				assert.Equal(t, 0, chunks[0].Index)
				assert.Equal(t, 1, chunks[0].Total)
				assert.Len(t, chunks[0].Logs, 1)
			},
		},
		{
			name: "multiple logs single chunk",
			logs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Log 1"},
				{Timestamp: "2023-01-01T10:00:01Z", Stream: "stdout", Message: "Log 2"},
				{Timestamp: "2023-01-01T10:00:02Z", Stream: "stdout", Message: "Log 3"},
			},
			maxTokens:      1000,
			tokensPerChar:  0.1,
			wantChunkCount: 1,
			validateFunc: func(t *testing.T, chunks []Chunk, _ []docker.LogEntry) {
				t.Helper()
				require.Len(t, chunks, 1)
				assert.Len(t, chunks[0].Logs, 3)
				assert.Equal(t, 0, chunks[0].Index)
				assert.Equal(t, 1, chunks[0].Total)
			},
		},
		{
			name: "multiple chunks",
			logs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "This is a longer message that takes more tokens"},
				{Timestamp: "2023-01-01T10:00:01Z", Stream: "stdout", Message: "Another long message that also takes many tokens"},
				{Timestamp: "2023-01-01T10:00:02Z", Stream: "stdout", Message: "Yet another long message for testing chunking"},
			},
			maxTokens:     50,
			tokensPerChar: 0.5,
			minChunkCount: 2,
			validateFunc: func(t *testing.T, chunks []Chunk, logs []docker.LogEntry) {
				t.Helper()
				require.GreaterOrEqual(t, len(chunks), 2, "expected at least 2 chunks")

				// Verify all chunks have correct metadata
				for i, chunk := range chunks {
					assert.Equal(t, i, chunk.Index, "chunk %d: incorrect index", i)
					assert.Equal(t, len(chunks), chunk.Total, "chunk %d: incorrect total", i)
					assert.NotEmpty(t, chunk.Logs, "chunk %d: should have logs", i)
					assert.Positive(t, chunk.TokenCount, "chunk %d: should have positive token count", i)
					assert.LessOrEqual(t, chunk.TokenCount, 50, "chunk %d: token count exceeds limit", i)
				}

				// Verify all logs are preserved
				totalLogs := 0
				for _, chunk := range chunks {
					totalLogs += len(chunk.Logs)
				}
				assert.Equal(t, len(logs), totalLogs, "all logs should be preserved across chunks")
			},
		},
		{
			name: "token count tracking",
			logs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Short"},
				{Timestamp: "2023-01-01T10:00:01Z", Stream: "stdout", Message: "Another short one"},
			},
			maxTokens:      1000,
			tokensPerChar:  0.1,
			wantChunkCount: 1,
			validateFunc: func(t *testing.T, chunks []Chunk, _ []docker.LogEntry) {
				t.Helper()
				require.Len(t, chunks, 1)
				assert.Positive(t, chunks[0].TokenCount, "token count should be positive")

				formattedText := FormatChunk(chunks[0])
				assert.LessOrEqual(t, chunks[0].TokenCount, len(formattedText),
					"token count should not exceed formatted text length")
			},
		},
		{
			name: "preserves log order",
			logs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "First"},
				{Timestamp: "2023-01-01T10:00:01Z", Stream: "stdout", Message: "Second"},
				{Timestamp: "2023-01-01T10:00:02Z", Stream: "stdout", Message: "Third"},
				{Timestamp: "2023-01-01T10:00:03Z", Stream: "stdout", Message: "Fourth"},
			},
			maxTokens:      1000,
			tokensPerChar:  0.1,
			wantChunkCount: 1,
			validateFunc: func(t *testing.T, chunks []Chunk, logs []docker.LogEntry) {
				t.Helper()
				// Reconstruct logs from chunks
				var reconstructed []docker.LogEntry
				for _, chunk := range chunks {
					reconstructed = append(reconstructed, chunk.Logs...)
				}

				require.Len(t, reconstructed, len(logs))
				for i, log := range reconstructed {
					assert.Equal(t, logs[i].Message, log.Message, "log %d: message mismatch", i)
					assert.Equal(t, logs[i].Timestamp, log.Timestamp, "log %d: timestamp mismatch", i)
				}
			},
		},
		{
			name: "very long single log",
			logs: func() []docker.LogEntry {
				longMessage := ""
				for i := 0; i < 1000; i++ {
					longMessage += "word "
				}
				return []docker.LogEntry{
					{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: longMessage},
					{Timestamp: "2023-01-01T10:00:01Z", Stream: "stdout", Message: "Normal message"},
				}
			}(),
			maxTokens:     100,
			tokensPerChar: 0.1,
			minChunkCount: 2,
			validateFunc: func(t *testing.T, chunks []Chunk, _ []docker.LogEntry) {
				t.Helper()
				require.GreaterOrEqual(t, len(chunks), 2, "should create at least 2 chunks for oversized log")
				assert.Len(t, chunks[0].Logs, 1, "first chunk should contain only the long log")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenizer := NewMockTokenizer(tt.tokensPerChar)
			chunks := ChunkLogs(tt.logs, tt.maxTokens, tokenizer)

			if tt.wantEmpty {
				assert.Empty(t, chunks)
				return
			}

			if tt.wantChunkCount > 0 {
				require.Len(t, chunks, tt.wantChunkCount)
			}

			if tt.minChunkCount > 0 {
				require.GreaterOrEqual(t, len(chunks), tt.minChunkCount)
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, chunks, tt.logs)
			}
		})
	}
}

func TestFormatChunk(t *testing.T) {
	tests := []struct {
		name            string
		chunk           Chunk
		wantEmpty       bool
		wantContains    []string
		wantNotContains []string
		validateFunc    func(t *testing.T, result string)
	}{
		{
			name: "empty chunk",
			chunk: Chunk{
				Logs:       []docker.LogEntry{},
				TokenCount: 0,
				Index:      0,
				Total:      1,
			},
			wantEmpty: true,
		},
		{
			name: "with timestamps",
			chunk: Chunk{
				Logs: []docker.LogEntry{
					{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Message 1"},
					{Timestamp: "2023-01-01T10:00:01Z", Stream: "stderr", Message: "Message 2"},
				},
				TokenCount: 10,
				Index:      0,
				Total:      1,
			},
			wantContains: []string{
				"[2023-01-01T10:00:00Z] Message 1",
				"[2023-01-01T10:00:01Z] Message 2",
			},
		},
		{
			name: "without timestamps",
			chunk: Chunk{
				Logs: []docker.LogEntry{
					{Timestamp: "", Stream: "stdout", Message: "Message without timestamp"},
					{Timestamp: "", Stream: "stderr", Message: "Another message"},
				},
				TokenCount: 10,
				Index:      0,
				Total:      1,
			},
			wantContains: []string{
				"Message without timestamp",
				"Another message",
			},
			wantNotContains: []string{"[", "]"},
		},
		{
			name: "mixed timestamps",
			chunk: Chunk{
				Logs: []docker.LogEntry{
					{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "With timestamp"},
					{Timestamp: "", Stream: "stdout", Message: "Without timestamp"},
					{Timestamp: "2023-01-01T10:00:02Z", Stream: "stdout", Message: "With timestamp again"},
				},
				TokenCount: 15,
				Index:      0,
				Total:      1,
			},
			wantContains: []string{
				"[2023-01-01T10:00:00Z] With timestamp",
				"Without timestamp",
				"[2023-01-01T10:00:02Z] With timestamp again",
			},
		},
		{
			name: "ends with newline",
			chunk: Chunk{
				Logs: []docker.LogEntry{
					{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Message"},
				},
				TokenCount: 5,
				Index:      0,
				Total:      1,
			},
			validateFunc: func(t *testing.T, result string) {
				t.Helper()
				require.NotEmpty(t, result)
				assert.Equal(t, '\n', rune(result[len(result)-1]), "result should end with newline")
			},
		},
		{
			name: "multiple newlines",
			chunk: Chunk{
				Logs: []docker.LogEntry{
					{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Line 1"},
					{Timestamp: "2023-01-01T10:00:01Z", Stream: "stdout", Message: "Line 2"},
					{Timestamp: "2023-01-01T10:00:02Z", Stream: "stdout", Message: "Line 3"},
				},
				TokenCount: 15,
				Index:      0,
				Total:      1,
			},
			validateFunc: func(t *testing.T, result string) {
				t.Helper()
				newlineCount := 0
				for _, ch := range result {
					if ch == '\n' {
						newlineCount++
					}
				}
				assert.Equal(t, 3, newlineCount, "should have one newline per log entry")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatChunk(tt.chunk)

			if tt.wantEmpty {
				assert.Empty(t, result)
				return
			}

			for _, want := range tt.wantContains {
				assert.Contains(t, result, want)
			}

			for _, notWant := range tt.wantNotContains {
				assert.NotContains(t, result, notWant)
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, result)
			}
		})
	}
}
