//go:build eval

package eval

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/zorak1103/dlia/internal/chunking"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/llm"
	"github.com/zorak1103/dlia/internal/prompts"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
	caseTimeout    = 10 * time.Minute
	maxAnalysisLog = 500
)

var allowedLevels = map[string]bool{"ok": true, "warning": true, "critical": true, "unknown": true}

// levels is a list of severity names; YAML may give a scalar or a sequence.
type levels []string

func (l *levels) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		var s string
		if err := node.Decode(&s); err != nil {
			return fmt.Errorf("decode severity: %w", err)
		}
		*l = levels{s}
		return nil
	}
	var s []string
	if err := node.Decode(&s); err != nil {
		return fmt.Errorf("line %d: severity must be a string or a list of strings: %w", node.Line, err)
	}
	*l = levels(s)
	return nil
}

type expectation struct {
	Severity       levels   `yaml:"severity"`
	MustContain    []string `yaml:"must_contain"`
	MustNotContain []string `yaml:"must_not_contain"`
}

type evalCase struct {
	Name    string
	Entries []docker.LogEntry
	Expect  expectation
}

func (e expectation) validate() error {
	if len(e.Severity) == 0 {
		return fmt.Errorf("severity must not be empty")
	}
	for _, s := range e.Severity {
		if !allowedLevels[s] {
			return fmt.Errorf("unknown severity level %q (allowed: ok, warning, critical, unknown)", s)
		}
	}
	return nil
}

func loadEntries(path string) ([]docker.LogEntry, error) {
	data, err := os.ReadFile(path) //nolint:gosec // fixture path
	if err != nil {
		return nil, fmt.Errorf("read logs: %w", err)
	}
	var entries []docker.LogEntry
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		entries = append(entries, docker.LogEntry{Message: line, Stream: "stdout"})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s contains no log lines", path)
	}
	return entries, nil
}

func loadExpectation(path string) (expectation, error) {
	var exp expectation
	data, err := os.ReadFile(path) //nolint:gosec // fixture path
	if err != nil {
		return exp, fmt.Errorf("read expectation: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&exp); err != nil {
		return exp, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := exp.validate(); err != nil {
		return exp, fmt.Errorf("%s: %w", path, err)
	}
	return exp, nil
}

// loadCases reads every sub-directory of dir as one eval case, sorted by name.
func loadCases(dir string) ([]evalCase, error) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var cases []evalCase
	for _, de := range dirEntries {
		if !de.IsDir() {
			continue
		}
		caseDir := filepath.Join(dir, de.Name())
		entries, err := loadEntries(filepath.Join(caseDir, "logs.txt"))
		if err != nil {
			return nil, fmt.Errorf("case %s: %w", de.Name(), err)
		}
		exp, err := loadExpectation(filepath.Join(caseDir, "expected.yaml"))
		if err != nil {
			return nil, fmt.Errorf("case %s: %w", de.Name(), err)
		}
		cases = append(cases, evalCase{Name: de.Name(), Entries: entries, Expect: exp})
	}
	return cases, nil
}

// checkResult returns one message per violated expectation.
func checkResult(exp expectation, level, analysis string) []string {
	var failures []string
	found := false
	for _, s := range exp.Severity {
		if s == level {
			found = true
			break
		}
	}
	if !found {
		failures = append(failures, fmt.Sprintf("severity %q not in %v", level, []string(exp.Severity)))
	}
	lower := strings.ToLower(analysis)
	for _, s := range exp.MustContain {
		if !strings.Contains(lower, strings.ToLower(s)) {
			failures = append(failures, fmt.Sprintf("analysis does not contain %q", s))
		}
	}
	for _, s := range exp.MustNotContain {
		if strings.Contains(lower, strings.ToLower(s)) {
			failures = append(failures, fmt.Sprintf("analysis contains forbidden %q", s))
		}
	}
	return failures
}

func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

func parseIntMin(name, v string, def, minimum int) (int, error) {
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < minimum {
		return 0, fmt.Errorf("%s must be an integer >= %d, got %q", name, minimum, v)
	}
	return n, nil
}

func envIntMin(t *testing.T, name string, def, minimum int) int {
	t.Helper()
	n, err := parseIntMin(name, os.Getenv(name), def, minimum)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return n
}

func TestParseIntMin(t *testing.T) {
	if n, err := parseIntMin("X", "", 7, 1); err != nil || n != 7 {
		t.Errorf("empty value: got %d, %v; want default 7", n, err)
	}
	if n, err := parseIntMin("X", "5625", 7, config.MinContextWindow); err != nil || n != 5625 {
		t.Errorf("value at minimum: got %d, %v; want 5625", n, err)
	}
	for _, v := range []string{"5624", "0", "abc"} {
		if _, err := parseIntMin("X", v, 7, config.MinContextWindow); err == nil {
			t.Errorf("value %q should be rejected", v)
		}
	}
}

func TestEval(t *testing.T) {
	cases, err := loadCases("testdata")
	if err != nil {
		t.Fatalf("load cases: %v", err)
	}

	apiKey, model := os.Getenv("DLIA_LLM_API_KEY"), os.Getenv("DLIA_LLM_MODEL")
	if apiKey == "" || model == "" {
		t.Skip("set DLIA_LLM_API_KEY and DLIA_LLM_MODEL to run evals")
	}
	baseURL := os.Getenv("DLIA_LLM_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	ctxWindow := envIntMin(t, "DLIA_LLM_CONTEXT_WINDOW", config.DefaultContextWindow, config.MinContextWindow)
	runs := envIntMin(t, "DLIA_EVAL_RUNS", 1, 1)

	cfg := &config.Config{LLM: config.LLMConfig{
		BaseURL:               baseURL,
		APIKey:                apiKey,
		Model:                 model,
		ContextWindow:         ctxWindow,
		MaxChunksPerContainer: 10,
	},
		Privacy: config.PrivacyConfig{AnonymizeIPs: true, AnonymizeSecrets: true},
	}
	loader := prompts.NewPromptLoader(cfg)
	client := llm.NewClient(llm.Options{BaseURL: baseURL, APIKey: apiKey, Model: model})
	pipeline, err := chunking.NewPipelineWithConfig(model, ctxWindow, client, loader, t.TempDir(), cfg)
	if err != nil {
		t.Fatalf("create pipeline: %v", err)
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			for run := 1; run <= runs; run++ {
				runOnce(t, pipeline, c, run)
			}
		})
	}
}

func runOnce(t *testing.T, pipeline *chunking.Pipeline, c evalCase, run int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), caseTimeout)
	defer cancel()

	result, err := pipeline.AnalyzeLogs(ctx, c.Name, c.Entries)
	if err != nil {
		t.Errorf("run %d: AnalyzeLogs failed: %v", run, err)
		return
	}
	level := result.Severity.String()
	if failures := checkResult(c.Expect, level, result.Analysis); len(failures) > 0 {
		t.Errorf("run %d: level=%s expected=%v\nfailures: %s\nanalysis: %s",
			run, level, []string(c.Expect.Severity), strings.Join(failures, "; "),
			firstRunes(result.Analysis, maxAnalysisLog))
	}
}

func writeCase(t *testing.T, root, name, logs, expected string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "logs.txt"), []byte(logs), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "expected.yaml"), []byte(expected), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadCases_Validation(t *testing.T) {
	tests := []struct {
		name     string
		logs     string
		expected string
		wantErr  bool
		wantSev  []string
	}{
		{"valid list", "line one\n", "severity: [warning, critical]\nmust_contain: [security]\n", false, []string{"warning", "critical"}},
		{"scalar severity", "line one\n", "severity: ok\n", false, []string{"ok"}},
		{"unknown level", "line one\n", "severity: [bad]\n", true, nil},
		{"empty severity", "line one\n", "must_contain: [x]\n", true, nil},
		{"unknown key", "line one\n", "severity: ok\nfoo: 1\n", true, nil},
		{"only blank lines", "\n  \n\r\n", "severity: ok\n", true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeCase(t, root, "c", tt.logs, tt.expected)
			cases, err := loadCases(root)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(cases) != 1 {
				t.Fatalf("got %d cases, want 1", len(cases))
			}
			if !reflect.DeepEqual([]string(cases[0].Expect.Severity), tt.wantSev) {
				t.Errorf("severity = %v, want %v", cases[0].Expect.Severity, tt.wantSev)
			}
		})
	}
}

func TestLoadCases_CRLFAndOrder(t *testing.T) {
	root := t.TempDir()
	writeCase(t, root, "b-case", "first\r\n\r\nsecond\r\n", "severity: ok\n")
	writeCase(t, root, "a-case", "only\n", "severity: ok\n")

	cases, err := loadCases(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 || cases[0].Name != "a-case" || cases[1].Name != "b-case" {
		t.Fatalf("cases not sorted by name: %+v", cases)
	}
	want := []docker.LogEntry{
		{Message: "first", Stream: "stdout"},
		{Message: "second", Stream: "stdout"},
	}
	if !reflect.DeepEqual(cases[1].Entries, want) {
		t.Errorf("entries = %+v, want %+v", cases[1].Entries, want)
	}
}

func TestCheckResult(t *testing.T) {
	tests := []struct {
		name     string
		exp      expectation
		level    string
		analysis string
		wantFail int
	}{
		{"level in list", expectation{Severity: levels{"warning", "critical"}}, "critical", "x", 0},
		{"level not in list", expectation{Severity: levels{"ok"}}, "warning", "x", 1},
		{"unknown not listed", expectation{Severity: levels{"ok", "warning"}}, "unknown", "x", 1},
		{"unknown listed", expectation{Severity: levels{"unknown"}}, "unknown", "x", 0},
		{"must_contain hit case-insensitive", expectation{Severity: levels{"ok"}, MustContain: []string{"Security"}}, "ok", "a SECURITY finding", 0},
		{"must_contain miss", expectation{Severity: levels{"ok"}, MustContain: []string{"security"}}, "ok", "nothing here", 1},
		{"must_not_contain hit", expectation{Severity: levels{"ok"}, MustNotContain: []string{"no significant issues"}}, "ok", "There are No Significant Issues.", 1},
		{"must_not_contain miss", expectation{Severity: levels{"ok"}, MustNotContain: []string{"bad"}}, "ok", "fine", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkResult(tt.exp, tt.level, tt.analysis)
			if len(got) != tt.wantFail {
				t.Errorf("failures = %v, want %d", got, tt.wantFail)
			}
		})
	}
}

func TestRealFixturesValid(t *testing.T) {
	cases, err := loadCases("testdata")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 8 {
		t.Fatalf("got %d cases, want 8", len(cases))
	}
	for _, c := range cases {
		if n := len(c.Entries); n < 10 || n > 40 {
			t.Errorf("%s: %d log lines, want 10-40", c.Name, n)
		}
	}
}
