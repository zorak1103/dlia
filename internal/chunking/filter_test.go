package chunking

import (
	"testing"
)

func TestNewRegexpFilter_ValidPatterns(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		wantErr  bool
	}{
		{
			name:     "single valid pattern",
			patterns: []string{"^DEBUG:"},
			wantErr:  false,
		},
		{
			name:     "multiple valid patterns",
			patterns: []string{"^DEBUG:", "healthcheck", "^INFO:"},
			wantErr:  false,
		},
		{
			name:     "empty patterns slice",
			patterns: []string{},
			wantErr:  false,
		},
		{
			name:     "nil patterns",
			patterns: nil,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := NewRegexpFilter(tt.patterns)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRegexpFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if filter == nil {
				t.Error("NewRegexpFilter() returned nil filter")
			}
		})
	}
}

func TestNewRegexpFilter_InvalidPatterns(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
	}{
		{
			name:     "invalid regexp - unclosed bracket",
			patterns: []string{"[abc"},
		},
		{
			name:     "invalid regexp - unclosed paren",
			patterns: []string{"(abc"},
		},
		{
			name:     "invalid regexp - bad escape",
			patterns: []string{"\\k"},
		},
		{
			name:     "one valid, one invalid pattern",
			patterns: []string{"^DEBUG:", "[invalid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := NewRegexpFilter(tt.patterns)
			if err == nil {
				t.Errorf("NewRegexpFilter() expected error for invalid pattern, got nil")
			}
			if filter != nil {
				t.Errorf("NewRegexpFilter() expected nil filter on error, got %v", filter)
			}
		})
	}
}

func TestRegexpFilter_Filter_SinglePattern(t *testing.T) {
	filter, err := NewRegexpFilter([]string{"^DEBUG:"})
	if err != nil {
		t.Fatalf("NewRegexpFilter() failed: %v", err)
	}

	logs := []string{
		"DEBUG: This should be filtered",
		"INFO: This should be kept",
		"DEBUG: Another debug line",
		"ERROR: This should be kept too",
	}

	filtered, stats := filter.Filter(logs)

	// Verify filtered logs
	expectedKept := []string{
		"INFO: This should be kept",
		"ERROR: This should be kept too",
	}

	if len(filtered) != len(expectedKept) {
		t.Errorf("Filter() got %d filtered logs, want %d", len(filtered), len(expectedKept))
	}

	for i, want := range expectedKept {
		if i >= len(filtered) {
			break
		}
		if filtered[i] != want {
			t.Errorf("Filter() filtered[%d] = %q, want %q", i, filtered[i], want)
		}
	}

	// Verify statistics
	if stats.LinesTotal != 4 {
		t.Errorf("Filter() stats.LinesTotal = %d, want 4", stats.LinesTotal)
	}
	if stats.LinesFiltered != 2 {
		t.Errorf("Filter() stats.LinesFiltered = %d, want 2", stats.LinesFiltered)
	}
	if stats.LinesKept != 2 {
		t.Errorf("Filter() stats.LinesKept = %d, want 2", stats.LinesKept)
	}
}

func TestRegexpFilter_Filter_MultiplePatterns(t *testing.T) {
	filter, err := NewRegexpFilter([]string{"^DEBUG:", "healthcheck", "^TRACE"})
	if err != nil {
		t.Fatalf("NewRegexpFilter() failed: %v", err)
	}

	logs := []string{
		"DEBUG: Starting process",
		"INFO: Application running",
		"healthcheck request received",
		"ERROR: Connection failed",
		"TRACE: Memory allocation",
		"WARN: Resource low",
		"GET /healthcheck HTTP/1.1",
	}

	filtered, stats := filter.Filter(logs)

	// Should filter out: DEBUG, healthcheck (2 times), TRACE = 4 lines
	// Should keep: INFO, ERROR, WARN = 3 lines
	expectedKept := []string{
		"INFO: Application running",
		"ERROR: Connection failed",
		"WARN: Resource low",
	}

	if len(filtered) != len(expectedKept) {
		t.Errorf("Filter() got %d filtered logs, want %d", len(filtered), len(expectedKept))
	}

	for i, want := range expectedKept {
		if i >= len(filtered) {
			break
		}
		if filtered[i] != want {
			t.Errorf("Filter() filtered[%d] = %q, want %q", i, filtered[i], want)
		}
	}

	// Verify statistics
	if stats.LinesTotal != 7 {
		t.Errorf("Filter() stats.LinesTotal = %d, want 7", stats.LinesTotal)
	}
	if stats.LinesFiltered != 4 {
		t.Errorf("Filter() stats.LinesFiltered = %d, want 4", stats.LinesFiltered)
	}
	if stats.LinesKept != 3 {
		t.Errorf("Filter() stats.LinesKept = %d, want 3", stats.LinesKept)
	}
}

func TestRegexpFilter_Filter_NoMatches(t *testing.T) {
	filter, err := NewRegexpFilter([]string{"^DEBUG:", "^TRACE"})
	if err != nil {
		t.Fatalf("NewRegexpFilter() failed: %v", err)
	}

	logs := []string{
		"INFO: Application started",
		"WARN: Low memory",
		"ERROR: Connection timeout",
	}

	filtered, stats := filter.Filter(logs)

	// All logs should be kept (no matches)
	if len(filtered) != len(logs) {
		t.Errorf("Filter() got %d filtered logs, want %d", len(filtered), len(logs))
	}

	for i, want := range logs {
		if i >= len(filtered) {
			break
		}
		if filtered[i] != want {
			t.Errorf("Filter() filtered[%d] = %q, want %q", i, filtered[i], want)
		}
	}

	// Verify statistics
	if stats.LinesTotal != 3 {
		t.Errorf("Filter() stats.LinesTotal = %d, want 3", stats.LinesTotal)
	}
	if stats.LinesFiltered != 0 {
		t.Errorf("Filter() stats.LinesFiltered = %d, want 0", stats.LinesFiltered)
	}
	if stats.LinesKept != 3 {
		t.Errorf("Filter() stats.LinesKept = %d, want 3", stats.LinesKept)
	}
}

func TestRegexpFilter_Filter_EmptyInput(t *testing.T) {
	filter, err := NewRegexpFilter([]string{"^DEBUG:"})
	if err != nil {
		t.Fatalf("NewRegexpFilter() failed: %v", err)
	}

	tests := []struct {
		name string
		logs []string
	}{
		{
			name: "nil input",
			logs: nil,
		},
		{
			name: "empty slice",
			logs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered, stats := filter.Filter(tt.logs)

			if len(filtered) != 0 {
				t.Errorf("Filter() with empty input got %d logs, want 0", len(filtered))
			}

			if stats.LinesTotal != 0 {
				t.Errorf("Filter() stats.LinesTotal = %d, want 0", stats.LinesTotal)
			}
			if stats.LinesFiltered != 0 {
				t.Errorf("Filter() stats.LinesFiltered = %d, want 0", stats.LinesFiltered)
			}
			if stats.LinesKept != 0 {
				t.Errorf("Filter() stats.LinesKept = %d, want 0", stats.LinesKept)
			}
		})
	}
}

func TestRegexpFilter_Filter_NoPatterns(t *testing.T) {
	filter, err := NewRegexpFilter([]string{})
	if err != nil {
		t.Fatalf("NewRegexpFilter() failed: %v", err)
	}

	logs := []string{
		"DEBUG: This log",
		"INFO: That log",
		"ERROR: Another log",
	}

	filtered, stats := filter.Filter(logs)

	// All logs should be kept when no patterns configured
	if len(filtered) != len(logs) {
		t.Errorf("Filter() with no patterns got %d logs, want %d", len(filtered), len(logs))
	}

	for i, want := range logs {
		if filtered[i] != want {
			t.Errorf("Filter() filtered[%d] = %q, want %q", i, filtered[i], want)
		}
	}

	// Verify statistics
	if stats.LinesTotal != 3 {
		t.Errorf("Filter() stats.LinesTotal = %d, want 3", stats.LinesTotal)
	}
	if stats.LinesFiltered != 0 {
		t.Errorf("Filter() stats.LinesFiltered = %d, want 0", stats.LinesFiltered)
	}
	if stats.LinesKept != 3 {
		t.Errorf("Filter() stats.LinesKept = %d, want 3", stats.LinesKept)
	}
}

func TestRegexpFilter_Filter_CaseSensitivity(t *testing.T) {
	tests := []struct {
		name         string
		pattern      string
		logs         []string
		wantFiltered int
		wantKept     int
	}{
		{
			name:    "case sensitive - lowercase pattern",
			pattern: "debug",
			logs: []string{
				"debug: lowercase match",
				"DEBUG: uppercase no match",
				"Debug: mixed case no match",
			},
			wantFiltered: 1,
			wantKept:     2,
		},
		{
			name:    "case sensitive - uppercase pattern",
			pattern: "DEBUG",
			logs: []string{
				"debug: lowercase no match",
				"DEBUG: uppercase match",
				"Debug: mixed case no match",
			},
			wantFiltered: 1,
			wantKept:     2,
		},
		{
			name:    "case insensitive pattern - using (?i)",
			pattern: "(?i)debug",
			logs: []string{
				"debug: lowercase match",
				"DEBUG: uppercase match",
				"Debug: mixed case match",
				"INFO: no match",
			},
			wantFiltered: 3,
			wantKept:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := NewRegexpFilter([]string{tt.pattern})
			if err != nil {
				t.Fatalf("NewRegexpFilter() failed: %v", err)
			}

			filtered, stats := filter.Filter(tt.logs)

			if stats.LinesFiltered != tt.wantFiltered {
				t.Errorf("Filter() stats.LinesFiltered = %d, want %d", stats.LinesFiltered, tt.wantFiltered)
			}
			if stats.LinesKept != tt.wantKept {
				t.Errorf("Filter() stats.LinesKept = %d, want %d", stats.LinesKept, tt.wantKept)
			}
			if len(filtered) != tt.wantKept {
				t.Errorf("Filter() returned %d logs, want %d", len(filtered), tt.wantKept)
			}
		})
	}
}

func TestRegexpFilter_Filter_ComplexPatterns(t *testing.T) {
	tests := []struct {
		name         string
		patterns     []string
		logs         []string
		wantKept     []string
		wantFiltered int
	}{
		{
			name:     "pattern with anchors",
			patterns: []string{"^\\[DEBUG\\]", "\\[TRACE\\]$"},
			logs: []string{
				"[DEBUG] start of line",
				"middle [DEBUG] text",
				"end of line [TRACE]",
				"[INFO] normal log",
			},
			wantKept:     []string{"middle [DEBUG] text", "[INFO] normal log"},
			wantFiltered: 2,
		},
		{
			name:     "pattern with character classes",
			patterns: []string{"^[0-9]{4}-[0-9]{2}-[0-9]{2}.*health"},
			logs: []string{
				"2024-12-20 healthcheck passed",
				"2024-12-20 system starting",
				"healthcheck passed",
				"ERROR: connection failed",
			},
			wantKept:     []string{"2024-12-20 system starting", "healthcheck passed", "ERROR: connection failed"},
			wantFiltered: 1,
		},
		{
			name:     "pattern with alternation",
			patterns: []string{"(DEBUG|TRACE|VERBOSE)"},
			logs: []string{
				"DEBUG: starting",
				"INFO: running",
				"TRACE: detailed info",
				"VERBOSE: very detailed",
				"ERROR: failed",
			},
			wantKept:     []string{"INFO: running", "ERROR: failed"},
			wantFiltered: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := NewRegexpFilter(tt.patterns)
			if err != nil {
				t.Fatalf("NewRegexpFilter() failed: %v", err)
			}

			filtered, stats := filter.Filter(tt.logs)

			if len(filtered) != len(tt.wantKept) {
				t.Errorf("Filter() got %d logs, want %d", len(filtered), len(tt.wantKept))
			}

			for i, want := range tt.wantKept {
				if i >= len(filtered) {
					t.Errorf("Filter() missing log at index %d: want %q", i, want)
					continue
				}
				if filtered[i] != want {
					t.Errorf("Filter() filtered[%d] = %q, want %q", i, filtered[i], want)
				}
			}

			if stats.LinesFiltered != tt.wantFiltered {
				t.Errorf("Filter() stats.LinesFiltered = %d, want %d", stats.LinesFiltered, tt.wantFiltered)
			}
		})
	}
}

func TestRegexpFilter_Filter_StatsConsistency(t *testing.T) {
	filter, err := NewRegexpFilter([]string{"^DEBUG:"})
	if err != nil {
		t.Fatalf("NewRegexpFilter() failed: %v", err)
	}

	logs := []string{
		"DEBUG: line 1",
		"INFO: line 2",
		"DEBUG: line 3",
		"WARN: line 4",
		"DEBUG: line 5",
	}

	_, stats := filter.Filter(logs)

	// Verify stats consistency: Total = Filtered + Kept
	if stats.LinesTotal != stats.LinesFiltered+stats.LinesKept {
		t.Errorf("Filter() stats inconsistent: Total(%d) != Filtered(%d) + Kept(%d)",
			stats.LinesTotal, stats.LinesFiltered, stats.LinesKept)
	}

	// Verify expected values
	if stats.LinesTotal != 5 {
		t.Errorf("Filter() stats.LinesTotal = %d, want 5", stats.LinesTotal)
	}
	if stats.LinesFiltered != 3 {
		t.Errorf("Filter() stats.LinesFiltered = %d, want 3", stats.LinesFiltered)
	}
	if stats.LinesKept != 2 {
		t.Errorf("Filter() stats.LinesKept = %d, want 2", stats.LinesKept)
	}
}

func TestRegexpFilter_MatchesAny(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		text     string
		want     bool
	}{
		{
			name:     "matches first pattern",
			patterns: []string{"^DEBUG:", "^INFO:"},
			text:     "DEBUG: message",
			want:     true,
		},
		{
			name:     "matches second pattern",
			patterns: []string{"^DEBUG:", "^INFO:"},
			text:     "INFO: message",
			want:     true,
		},
		{
			name:     "no match",
			patterns: []string{"^DEBUG:", "^INFO:"},
			text:     "ERROR: message",
			want:     false,
		},
		{
			name:     "empty patterns - no match",
			patterns: []string{},
			text:     "any text",
			want:     false,
		},
		{
			name:     "nil patterns - no match",
			patterns: nil,
			text:     "any text",
			want:     false,
		},
		{
			name:     "empty text - no match",
			patterns: []string{"^DEBUG:"},
			text:     "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := NewRegexpFilter(tt.patterns)
			if err != nil {
				t.Fatalf("NewRegexpFilter() failed: %v", err)
			}

			got := filter.MatchesAny(tt.text)
			if got != tt.want {
				t.Errorf("MatchesAny(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}
