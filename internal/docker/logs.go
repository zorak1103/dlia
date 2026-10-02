package docker

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"
)

const streamStdout = "stdout"

// LogsOptions contains options for reading container logs
type LogsOptions struct {
	Since      time.Time // Read logs since this timestamp
	Until      time.Time // Read logs until this timestamp (optional)
	Timestamps bool      // Include timestamps in output
	Follow     bool      // Stream logs in real-time
}

// parseLogStream parses the Docker log stream into LogEntry objects
func parseLogStream(reader io.Reader) ([]LogEntry, error) {
	var entries []LogEntry
	scanner := bufio.NewScanner(reader)

	// Docker multiplexes stdout/stderr with 8-byte headers
	// We need to skip the header and parse the content
	for scanner.Scan() {
		line := scanner.Text()

		// Docker API returns logs with an 8-byte header for stream multiplexing
		// Format: [8 bytes header][log content with timestamp]
		// We need to handle both cases (with and without header)

		// Skip the 8-byte header if present (binary data)
		// The header format is: [STREAM_TYPE][0x00][0x00][0x00][SIZE (4 bytes)]
		if len(line) > 8 {
			// Check if line starts with binary header (stream type 1 or 2)
			if line[0] == 1 || line[0] == 2 {
				line = line[8:] // Skip header
			}
		}

		// Parse timestamp and message
		// Format: "2025-11-30T19:00:00.123456789Z message here"
		entry := parseLogLine(line)
		if entry != nil {
			entries = append(entries, *entry)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading log stream at line %d: %w", len(entries), err)
	}

	return entries, nil
}

// parseLogLine parses a single log line with timestamp
func parseLogLine(line string) *LogEntry {
	// Find the space after timestamp
	spaceIdx := strings.Index(line, " ")
	if spaceIdx == -1 {
		// No timestamp, treat entire line as message
		return &LogEntry{
			Timestamp: "",
			Stream:    streamStdout,
			Message:   line,
		}
	}

	timestamp := line[:spaceIdx]
	message := ""
	if spaceIdx+1 < len(line) {
		message = line[spaceIdx+1:]
	}

	return &LogEntry{
		Timestamp: timestamp,
		Stream:    streamStdout,
		Message:   message,
	}
}

// parseLogTimestamp parses a Docker log entry timestamp, trying RFC3339Nano
// first and falling back to RFC3339.
func parseLogTimestamp(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}, err
		}
	}
	return t, nil
}

// dropEntriesAtOrBefore drops every entry whose parsed timestamp is at or
// before since. Entries with an empty or unparseable timestamp are kept (a
// parse failure never drops a line). Order is preserved, no sorting.
func dropEntriesAtOrBefore(entries []LogEntry, since time.Time) []LogEntry {
	kept := make([]LogEntry, 0, len(entries))
	for _, entry := range entries {
		ts, err := parseLogTimestamp(entry.Timestamp)
		if err != nil {
			kept = append(kept, entry)
			continue
		}
		if ts.After(since) {
			kept = append(kept, entry)
		}
	}
	return kept
}

// GetLatestLogTime returns the timestamp of the most recent log entry.
// It walks backwards over trailing entries with an empty or unparseable
// timestamp and returns the last parseable one, so a trailing non-timestamped
// line does not push a zero cursor into the state. The zero time is returned
// only when no entry has a usable timestamp.
func GetLatestLogTime(entries []LogEntry) (time.Time, error) {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Timestamp == "" {
			continue
		}
		t, err := parseLogTimestamp(entries[i].Timestamp)
		if err != nil {
			continue
		}
		return t, nil
	}
	return time.Time{}, nil
}
