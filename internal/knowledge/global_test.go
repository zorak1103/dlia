package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zorak1103/dlia/internal/chunking"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/severity"
)

func makeOutcome(sev severity.Level, analysis string) ServiceOutcome {
	return ServiceOutcome{
		Result: &chunking.AnalyzeResult{
			Analysis: analysis,
			Severity: sev,
		},
	}
}

func makeFailedOutcome() ServiceOutcome {
	return ServiceOutcome{Result: nil}
}

func readGlobalSummary(t *testing.T, kbDir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(kbDir, "global_summary.md")) //nolint:gosec
	if err != nil {
		t.Fatalf("failed to read global summary: %v", err)
	}
	return string(data)
}

func TestUpdateGlobalSummary_HealthLine(t *testing.T) {
	tests := []struct {
		name     string
		outcomes map[string]ServiceOutcome
		wantLine string
	}{
		{
			name: "all OK → operational",
			outcomes: map[string]ServiceOutcome{
				"svc-a": makeOutcome(severity.OK, "all fine"),
				"svc-b": makeOutcome(severity.OK, "running normally"),
			},
			wantLine: "🟢 All Systems Operational",
		},
		{
			name: "one warning + one failed → need attention",
			outcomes: map[string]ServiceOutcome{
				"svc-a": makeOutcome(severity.Warning, "something slow"),
				"svc-b": makeFailedOutcome(),
			},
			wantLine: "⚠️ 2 Service(s) Need Attention",
		},
		{
			name: "only critical → need attention",
			outcomes: map[string]ServiceOutcome{
				"svc-a": makeOutcome(severity.Critical, "crash"),
			},
			wantLine: "⚠️ 1 Service(s) Need Attention",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cfg := &config.Config{Output: config.OutputConfig{KnowledgeBaseDir: tmpDir}}

			if err := UpdateGlobalSummary(tt.outcomes, cfg); err != nil {
				t.Fatalf("UpdateGlobalSummary() error = %v", err)
			}

			content := readGlobalSummary(t, tmpDir)
			if !strings.Contains(content, tt.wantLine) {
				t.Errorf("expected health line %q not found in:\n%s", tt.wantLine, content)
			}
		})
	}
}

func TestUpdateGlobalSummary_TableIncludesFailed(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{Output: config.OutputConfig{KnowledgeBaseDir: tmpDir}}

	outcomes := map[string]ServiceOutcome{
		"svc": makeFailedOutcome(),
	}

	if err := UpdateGlobalSummary(outcomes, cfg); err != nil {
		t.Fatalf("UpdateGlobalSummary() error = %v", err)
	}

	content := readGlobalSummary(t, tmpDir)
	// failed row: | svc | ⚪ Analysis failed | – |
	if !strings.Contains(content, "⚪ Analysis failed") {
		t.Errorf("expected failed label in table, got:\n%s", content)
	}
	if !strings.Contains(content, "| svc |") {
		t.Errorf("expected svc row in table, got:\n%s", content)
	}
}

func TestUpdateGlobalSummary_AttentionOrder(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{Output: config.OutputConfig{KnowledgeBaseDir: tmpDir}}

	outcomes := map[string]ServiceOutcome{
		"w":  makeOutcome(severity.Warning, "warn"),
		"c":  makeOutcome(severity.Critical, "crit"),
		"u":  makeOutcome(severity.Unknown, "unknown"),
		"f":  makeFailedOutcome(),
		"o":  makeOutcome(severity.OK, "ok"),
		"c2": makeOutcome(severity.Critical, "crit2"),
	}

	if err := UpdateGlobalSummary(outcomes, cfg); err != nil {
		t.Fatalf("UpdateGlobalSummary() error = %v", err)
	}

	content := readGlobalSummary(t, tmpDir)

	// o must be absent from attention section
	attentionIdx := strings.Index(content, "Services Needing Attention")
	if attentionIdx == -1 {
		t.Fatalf("attention section not found in:\n%s", content)
	}
	attentionSection := content[attentionIdx:]

	if strings.Contains(attentionSection, "- 🟢 OK **o**") {
		t.Errorf("OK service must not appear in attention section")
	}

	// order: c, c2, u, w, f
	cPos := strings.Index(attentionSection, "**c**")
	c2Pos := strings.Index(attentionSection, "**c2**")
	uPos := strings.Index(attentionSection, "**u**")
	wPos := strings.Index(attentionSection, "**w**")
	fPos := strings.Index(attentionSection, "**f**")

	for _, pair := range [][2]int{{cPos, c2Pos}, {c2Pos, uPos}, {uPos, wPos}, {wPos, fPos}} {
		if pair[0] == -1 || pair[1] == -1 || pair[0] >= pair[1] {
			t.Errorf("wrong attention order: c=%d c2=%d u=%d w=%d f=%d in:\n%s",
				cPos, c2Pos, uPos, wPos, fPos, attentionSection)
			break
		}
	}
}

func TestUpdateGlobalSummary_ReportLink(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	reportPath := filepath.Join(tmpDir, "reports", "my svc", "2026.md")

	cfg := &config.Config{Output: config.OutputConfig{KnowledgeBaseDir: kbDir}}

	outcomes := map[string]ServiceOutcome{
		"my svc": {
			Result:     &chunking.AnalyzeResult{Analysis: "warn", Severity: severity.Warning},
			ReportPath: reportPath,
		},
		"no-link": {
			Result: &chunking.AnalyzeResult{Analysis: "warn", Severity: severity.Warning},
		},
	}

	if err := UpdateGlobalSummary(outcomes, cfg); err != nil {
		t.Fatalf("UpdateGlobalSummary() error = %v", err)
	}

	content := readGlobalSummary(t, kbDir)

	// should contain angle-bracket link with relative path
	if !strings.Contains(content, `[latest report](<`) {
		t.Errorf("expected angle-bracket report link, got:\n%s", content)
	}
	if !strings.Contains(content, "my svc") {
		t.Errorf("expected service name in link text, got:\n%s", content)
	}
	// path should use forward slashes and be relative
	if strings.Contains(content, `\`) {
		t.Errorf("expected forward slashes in link, got:\n%s", content)
	}
	// no-link service should not have a link
	noLinkIdx := strings.Index(content, "**no-link**")
	if noLinkIdx == -1 {
		t.Fatalf("no-link service not found in:\n%s", content)
	}
	// check that after **no-link** there's no [latest report]
	afterNoLink := content[noLinkIdx:]
	nextBullet := strings.Index(afterNoLink[1:], "\n-")
	var segment string
	if nextBullet == -1 {
		segment = afterNoLink
	} else {
		segment = afterNoLink[:nextBullet+1]
	}
	if strings.Contains(segment, "[latest report]") {
		t.Errorf("no-link service should not have a report link, got:\n%s", segment)
	}
}

func TestUpdateGlobalSummary_EmptyAttentionList(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{Output: config.OutputConfig{KnowledgeBaseDir: tmpDir}}

	outcomes := map[string]ServiceOutcome{
		"svc": makeOutcome(severity.OK, "all good"),
	}

	if err := UpdateGlobalSummary(outcomes, cfg); err != nil {
		t.Fatalf("UpdateGlobalSummary() error = %v", err)
	}

	content := readGlobalSummary(t, tmpDir)
	if !strings.Contains(content, "*No services need attention.*") {
		t.Errorf("expected empty attention message, got:\n%s", content)
	}
}
