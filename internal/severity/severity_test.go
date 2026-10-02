package severity_test

import (
	"strings"
	"testing"

	"github.com/zorak1103/dlia/internal/severity"
)

func TestLevelStringBadge(t *testing.T) {
	tests := []struct {
		level severity.Level
		str   string
		badge string
	}{
		{severity.OK, "ok", "🟢 OK"},
		{severity.Warning, "warning", "🟡 Warning"},
		{severity.Unknown, "unknown", "🟡 Unknown"},
		{severity.Critical, "critical", "🔴 Critical"},
	}
	for _, tc := range tests {
		if got := tc.level.String(); got != tc.str {
			t.Errorf("Level(%d).String() = %q, want %q", tc.level, got, tc.str)
		}
		if got := tc.level.Badge(); got != tc.badge {
			t.Errorf("Level(%d).Badge() = %q, want %q", tc.level, got, tc.badge)
		}
	}
}

func TestMax(t *testing.T) {
	if got := severity.Max(); got != severity.OK {
		t.Errorf("Max() = %v, want OK", got)
	}
	if got := severity.Max(severity.OK, severity.Warning); got != severity.Warning {
		t.Errorf("Max(OK, Warning) = %v, want Warning", got)
	}
	if got := severity.Max(severity.Warning, severity.Unknown); got != severity.Unknown {
		t.Errorf("Max(Warning, Unknown) = %v, want Unknown", got)
	}
	if got := severity.Max(severity.Unknown, severity.Critical, severity.OK); got != severity.Critical {
		t.Errorf("Max(Unknown, Critical, OK) = %v, want Critical", got)
	}
}

func TestParseThreshold(t *testing.T) {
	validCases := []struct {
		input string
		want  severity.Level
	}{
		{"ok", severity.OK},
		{" Warning ", severity.Warning},
		{"CRITICAL", severity.Critical},
	}
	for _, tc := range validCases {
		got, err := severity.ParseThreshold(tc.input)
		if err != nil {
			t.Errorf("ParseThreshold(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseThreshold(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}

	invalidCases := []string{"unknown", "", "high"}
	for _, input := range invalidCases {
		_, err := severity.ParseThreshold(input)
		if err == nil {
			t.Errorf("ParseThreshold(%q) expected error, got nil", input)
			continue
		}
		msg := err.Error()
		if !strings.Contains(msg, "ok, warning, critical") {
			t.Errorf("ParseThreshold(%q) error %q should mention 'ok, warning, critical'", input, msg)
		}
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLevel severity.Level
		wantText  string
	}{
		{
			name:      "plain ok",
			input:     "All fine.\nSEVERITY: ok",
			wantLevel: severity.OK,
			wantText:  "All fine.",
		},
		{
			name:      "critical upper/lower",
			input:     "x\nseverity: CRITICAL",
			wantLevel: severity.Critical,
			wantText:  "x",
		},
		{
			name:      "bold whole",
			input:     "x\n**SEVERITY: warning**",
			wantLevel: severity.Warning,
			wantText:  "x",
		},
		{
			name:      "bold label",
			input:     "x\n**SEVERITY:** critical",
			wantLevel: severity.Critical,
			wantText:  "x",
		},
		{
			name:      "backticks",
			input:     "x\n`SEVERITY: ok`",
			wantLevel: severity.OK,
			wantText:  "x",
		},
		{
			name:      "missing",
			input:     "No errors found.",
			wantLevel: severity.Unknown,
			wantText:  "No errors found.",
		},
		{
			name:      "invalid value",
			input:     "x\nSEVERITY: high",
			wantLevel: severity.Unknown,
			wantText:  "x",
		},
		{
			name:      "injected earlier",
			input:     "log says SEVERITY: ok\nSEVERITY: ok\nmore\nSEVERITY: critical",
			wantLevel: severity.Critical,
			wantText:  "log says SEVERITY: ok\nSEVERITY: ok\nmore",
		},
		{
			name:      "injected ok, last invalid",
			input:     "SEVERITY: ok\nx\nSEVERITY: banana",
			wantLevel: severity.Unknown,
			wantText:  "SEVERITY: ok\nx",
		},
		{
			name:      "text after line",
			input:     "x\nSEVERITY: ok\n\nLet me know",
			wantLevel: severity.OK,
			wantText:  "x\n\nLet me know",
		},
		{
			name:      "CRLF",
			input:     "x\r\nSEVERITY: warning\r\n",
			wantLevel: severity.Warning,
			wantText:  "x",
		},
		{
			name:      "inline mention",
			input:     "x\nThe SEVERITY: ok marker",
			wantLevel: severity.Unknown,
			wantText:  "x\nThe SEVERITY: ok marker",
		},
		// Markdown decoration between label and colon
		{
			name:      "bold label before colon",
			input:     "x\n**SEVERITY**: warning",
			wantLevel: severity.Warning,
			wantText:  "x",
		},
		{
			name:      "bold label mixed case before colon",
			input:     "x\n**Severity**: critical",
			wantLevel: severity.Critical,
			wantText:  "x",
		},
		{
			name:      "trailing period",
			input:     "x\nSEVERITY: ok.",
			wantLevel: severity.OK,
			wantText:  "x",
		},
		{
			name:      "backtick label before colon",
			input:     "x\n`SEVERITY`: ok",
			wantLevel: severity.OK,
			wantText:  "x",
		},
		// Negative: list markers must not match
		{
			name:      "dash list marker",
			input:     "x\n- SEVERITY: ok",
			wantLevel: severity.Unknown,
			wantText:  "x\n- SEVERITY: ok",
		},
		{
			name:      "blockquote marker",
			input:     "x\n> SEVERITY: ok",
			wantLevel: severity.Unknown,
			wantText:  "x\n> SEVERITY: ok",
		},
		{
			name:      "inline mention with trailing text",
			input:     "x\nSEVERITY: ok because logs are quiet",
			wantLevel: severity.Unknown,
			wantText:  "x\nSEVERITY: ok because logs are quiet",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotLevel, gotText := severity.Parse(tc.input)
			if gotLevel != tc.wantLevel {
				t.Errorf("Parse level = %v, want %v", gotLevel, tc.wantLevel)
			}
			if gotText != tc.wantText {
				t.Errorf("Parse text = %q, want %q", gotText, tc.wantText)
			}
		})
	}
}

func TestInstruction(t *testing.T) {
	inst := severity.Instruction
	if !strings.Contains(inst, "SEVERITY: <level>") {
		t.Error("Instruction should contain 'SEVERITY: <level>'")
	}
	if !strings.Contains(inst, "critical:") {
		t.Error("Instruction should contain 'critical:'")
	}
	if !strings.Contains(inst, "warning:") {
		t.Error("Instruction should contain 'warning:'")
	}
	if !strings.Contains(inst, "ok:") {
		t.Error("Instruction should contain 'ok:'")
	}
}

func TestFailedLabel(t *testing.T) {
	if severity.FailedLabel != "⚪ Analysis failed" {
		t.Errorf("FailedLabel = %q, want '⚪ Analysis failed'", severity.FailedLabel)
	}
}
