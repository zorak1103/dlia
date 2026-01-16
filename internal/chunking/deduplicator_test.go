package chunking

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorak1103/dlia/internal/docker"
)

func TestDeduplicate(t *testing.T) {
	tests := []struct {
		name           string
		logs           []docker.LogEntry
		wantNil        bool
		wantLen        int
		wantMessages   []string
		wantTimestamps []string
		wantStreams    []string
	}{
		{
			name:    "empty slice",
			logs:    []docker.LogEntry{},
			wantNil: false,
			wantLen: 0,
		},
		{
			name:    "nil slice",
			logs:    nil,
			wantNil: true,
		},
		{
			name: "single entry",
			logs: []docker.LogEntry{
				{Timestamp: "2024-01-01T00:00:00Z", Stream: "stdout", Message: "hello"},
			},
			wantLen:        1,
			wantMessages:   []string{"hello"},
			wantTimestamps: []string{"2024-01-01T00:00:00Z"},
		},
		{
			name: "no duplicates",
			logs: []docker.LogEntry{
				{Timestamp: "2024-01-01T00:00:00Z", Stream: "stdout", Message: "line1"},
				{Timestamp: "2024-01-01T00:00:01Z", Stream: "stdout", Message: "line2"},
				{Timestamp: "2024-01-01T00:00:02Z", Stream: "stdout", Message: "line3"},
			},
			wantLen:      3,
			wantMessages: []string{"line1", "line2", "line3"},
		},
		{
			name: "below threshold (2 repeats)",
			logs: []docker.LogEntry{
				{Timestamp: "2024-01-01T00:00:00Z", Stream: "stdout", Message: "repeated"},
				{Timestamp: "2024-01-01T00:00:01Z", Stream: "stdout", Message: "repeated"},
			},
			wantLen:      2,
			wantMessages: []string{"repeated", "repeated"},
		},
		{
			name: "at threshold (3 repeats)",
			logs: []docker.LogEntry{
				{Timestamp: "2024-01-01T00:00:00Z", Stream: "stdout", Message: "repeated"},
				{Timestamp: "2024-01-01T00:00:01Z", Stream: "stdout", Message: "repeated"},
				{Timestamp: "2024-01-01T00:00:02Z", Stream: "stdout", Message: "repeated"},
			},
			wantLen:        1,
			wantMessages:   []string{"[REPEAT x3] repeated"},
			wantTimestamps: []string{"2024-01-01T00:00:00Z"},
		},
		{
			name: "above threshold (5 repeats)",
			logs: []docker.LogEntry{
				{Timestamp: "2024-01-01T00:00:00Z", Stream: "stderr", Message: "error"},
				{Timestamp: "2024-01-01T00:00:01Z", Stream: "stderr", Message: "error"},
				{Timestamp: "2024-01-01T00:00:02Z", Stream: "stderr", Message: "error"},
				{Timestamp: "2024-01-01T00:00:03Z", Stream: "stderr", Message: "error"},
				{Timestamp: "2024-01-01T00:00:04Z", Stream: "stderr", Message: "error"},
			},
			wantLen:      1,
			wantMessages: []string{"[REPEAT x5] error"},
			wantStreams:  []string{"stderr"},
		},
		{
			name: "multiple sequences",
			logs: []docker.LogEntry{
				{Timestamp: "t1", Stream: "stdout", Message: "aaa"},
				{Timestamp: "t2", Stream: "stdout", Message: "aaa"},
				{Timestamp: "t3", Stream: "stdout", Message: "aaa"},
				{Timestamp: "t4", Stream: "stdout", Message: "bbb"},
				{Timestamp: "t5", Stream: "stdout", Message: "bbb"},
				{Timestamp: "t6", Stream: "stdout", Message: "bbb"},
				{Timestamp: "t7", Stream: "stdout", Message: "bbb"},
			},
			wantLen:      2,
			wantMessages: []string{"[REPEAT x3] aaa", "[REPEAT x4] bbb"},
		},
		{
			name: "mixed sequences (above and below threshold)",
			logs: []docker.LogEntry{
				{Timestamp: "t1", Stream: "stdout", Message: "single"},
				{Timestamp: "t2", Stream: "stdout", Message: "double"},
				{Timestamp: "t3", Stream: "stdout", Message: "double"},
				{Timestamp: "t4", Stream: "stdout", Message: "triple"},
				{Timestamp: "t5", Stream: "stdout", Message: "triple"},
				{Timestamp: "t6", Stream: "stdout", Message: "triple"},
				{Timestamp: "t7", Stream: "stdout", Message: "end"},
			},
			wantLen:      5,
			wantMessages: []string{"single", "double", "double", "[REPEAT x3] triple", "end"},
		},
		{
			name: "final sequence below threshold",
			logs: []docker.LogEntry{
				{Timestamp: "t1", Stream: "stdout", Message: "aaa"},
				{Timestamp: "t2", Stream: "stdout", Message: "aaa"},
				{Timestamp: "t3", Stream: "stdout", Message: "aaa"},
				{Timestamp: "t4", Stream: "stdout", Message: "bbb"},
				{Timestamp: "t5", Stream: "stdout", Message: "bbb"},
			},
			wantLen:      3,
			wantMessages: []string{"[REPEAT x3] aaa", "bbb", "bbb"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Deduplicate(tt.logs)

			if tt.wantNil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Len(t, result, tt.wantLen)

			if tt.wantMessages != nil {
				for i, expectedMsg := range tt.wantMessages {
					require.Less(t, i, len(result), "test case has more expected messages than result entries")
					assert.Equal(t, expectedMsg, result[i].Message, "message mismatch at index %d", i)
				}
			}

			if tt.wantTimestamps != nil {
				for i, expectedTS := range tt.wantTimestamps {
					require.Less(t, i, len(result), "test case has more expected timestamps than result entries")
					assert.Equal(t, expectedTS, result[i].Timestamp, "timestamp mismatch at index %d", i)
				}
			}

			if tt.wantStreams != nil {
				for i, expectedStream := range tt.wantStreams {
					require.Less(t, i, len(result), "test case has more expected streams than result entries")
					assert.Equal(t, expectedStream, result[i].Stream, "stream mismatch at index %d", i)
				}
			}
		})
	}
}

func TestFormatLogs(t *testing.T) {
	tests := []struct {
		name string
		logs []docker.LogEntry
		want string
	}{
		{
			name: "empty logs",
			logs: []docker.LogEntry{},
			want: "",
		},
		{
			name: "with timestamp",
			logs: []docker.LogEntry{
				{Timestamp: "2024-01-01T00:00:00Z", Stream: "stdout", Message: "hello world"},
			},
			want: "[2024-01-01T00:00:00Z] hello world\n",
		},
		{
			name: "without timestamp",
			logs: []docker.LogEntry{
				{Timestamp: "", Stream: "stdout", Message: "hello world"},
			},
			want: "hello world\n",
		},
		{
			name: "multiple entries with mixed timestamps",
			logs: []docker.LogEntry{
				{Timestamp: "t1", Stream: "stdout", Message: "line1"},
				{Timestamp: "", Stream: "stdout", Message: "line2"},
				{Timestamp: "t3", Stream: "stderr", Message: "line3"},
			},
			want: "[t1] line1\nline2\n[t3] line3\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatLogs(tt.logs)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestGetDeduplicationStats(t *testing.T) {
	tests := []struct {
		name         string
		original     []docker.LogEntry
		deduplicated []docker.LogEntry
		wantOrig     int
		wantDedup    int
		wantSaved    int
	}{
		{
			name: "normal deduplication",
			original: []docker.LogEntry{
				{Message: "a"},
				{Message: "a"},
				{Message: "a"},
				{Message: "b"},
				{Message: "b"},
			},
			deduplicated: []docker.LogEntry{
				{Message: "[REPEAT x3] a"},
				{Message: "b"},
				{Message: "b"},
			},
			wantOrig:  5,
			wantDedup: 3,
			wantSaved: 2,
		},
		{
			name:         "empty slices",
			original:     nil,
			deduplicated: nil,
			wantOrig:     0,
			wantDedup:    0,
			wantSaved:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origCount, dedupCount, savedCount := GetDeduplicationStats(tt.original, tt.deduplicated)
			assert.Equal(t, tt.wantOrig, origCount, "original count mismatch")
			assert.Equal(t, tt.wantDedup, dedupCount, "deduplicated count mismatch")
			assert.Equal(t, tt.wantSaved, savedCount, "saved count mismatch")
		})
	}
}

func TestDeduplicateThreshold(t *testing.T) {
	// Verify the constant value hasn't changed
	assert.Equal(t, 3, DeduplicateThreshold, "DeduplicateThreshold constant should be 3")
}
