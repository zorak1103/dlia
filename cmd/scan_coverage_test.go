package cmd

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zorak1103/dlia/internal/chunking"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/state"
)

// TestProcessContainers_Success tests successful container processing
func TestProcessContainers_Success(t *testing.T) {
	withScanLLMMock(t, &fakeScanLLM{analysis: "all good"})

	scanCfg := newTestScanConfig()
	scanCfg.verbose = false
	scanCfg.dryRun = false

	ctx := context.Background()

	tmpDir := t.TempDir()
	stateFile := tmpDir + "/state.json"
	st, _ := state.Load(stateFile)

	containers := []docker.Container{
		{ID: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd", Name: "container1", State: "running"},
	}

	mockDocker := &MockDockerClient{
		containers: containers,
		logs: map[string][]docker.LogEntry{
			"abc123def456abc123def456abc123def456abc123def456abc123def456abcd": {
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Test"},
			},
		},
	}

	cfg := &config.Config{
		LLM: config.LLMConfig{
			APIKey:        "test-key",
			Model:         "test-model",
			BaseURL:       "http://test",
			ContextWindow: 4000,
		},
		Output: config.OutputConfig{
			ReportsDir:       tmpDir + "/reports",
			KnowledgeBaseDir: tmpDir + "/kb",
		},
	}

	// Create directories
	_ = os.MkdirAll(cfg.Output.ReportsDir, 0750)
	_ = os.MkdirAll(cfg.Output.KnowledgeBaseDir+"/services", 0750)

	results, stats := processContainers(ctx, mockDocker, st, containers, cfg, scanCfg, 0)

	if len(results) != 1 || results["container1"] == nil {
		t.Errorf("Expected 1 result for container1, got %v", results)
	}

	if stats.scannedContainers != 1 {
		t.Errorf("Expected 1 scanned container, got %d", stats.scannedContainers)
	}

	if stats.failedContainers != 0 {
		t.Errorf("Expected 0 failed containers, got %d", stats.failedContainers)
	}

	if stats.totalLogs != 1 {
		t.Errorf("Expected 1 total logs, got %d", stats.totalLogs)
	}

	want := time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)
	if got, _ := st.GetLastScan(containers[0].ID); !got.Equal(want) {
		t.Errorf("Cursor = %v, want %v", got, want)
	}
}

// TestProcessContainers_NoLogs tests container with no logs
func TestProcessContainers_NoLogs(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.verbose = false

	ctx := context.Background()

	tmpDir := t.TempDir()
	stateFile := tmpDir + "/state.json"
	st, _ := state.Load(stateFile)

	containers := []docker.Container{
		{ID: "abc123def456abc123def456abc123def456abc123def456abc123def456abc1", Name: "container1", State: "running"},
	}

	mockDocker := &MockDockerClient{
		containers: containers,
		logs:       map[string][]docker.LogEntry{},
	}

	cfg := &config.Config{
		LLM: config.LLMConfig{
			APIKey:        "test-key",
			Model:         "test-model",
			BaseURL:       "http://test",
			ContextWindow: 4000,
		},
	}

	results, stats := processContainers(ctx, mockDocker, st, containers, cfg, scanCfg, 0)

	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}

	if stats.scannedContainers != 0 {
		t.Errorf("Expected 0 scanned containers, got %d", stats.scannedContainers)
	}
}

// TestProcessContainers_LogReadError tests error reading logs
func TestProcessContainers_LogReadError(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.verbose = false

	ctx := context.Background()

	tmpDir := t.TempDir()
	stateFile := tmpDir + "/state.json"
	st, _ := state.Load(stateFile)

	containers := []docker.Container{
		{ID: "abc123def456abc123def456abc123def456abc123def456abc123def456abc2", Name: "container1", State: "running"},
	}

	mockDocker := &MockDockerClient{
		containers: containers,
		logsErr:    errors.New("logs error"),
	}

	cfg := &config.Config{
		LLM: config.LLMConfig{
			APIKey:        "test-key",
			Model:         "test-model",
			BaseURL:       "http://test",
			ContextWindow: 4000,
		},
	}

	results, stats := processContainers(ctx, mockDocker, st, containers, cfg, scanCfg, 0)

	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}

	if stats.scannedContainers != 0 {
		t.Errorf("Expected 0 scanned containers due to error, got %d", stats.scannedContainers)
	}
}

// TestProcessLLMAnalysis_InitError tests LLM initialization error
func TestProcessLLMAnalysis_InitError(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false

	ctx := context.Background()

	cfg := &config.Config{
		LLM: config.LLMConfig{
			APIKey: "", // No API key
		},
	}

	var pipeline *chunking.Pipeline
	logs := []docker.LogEntry{
		{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Test"},
	}

	result := processLLMAnalysis(ctx, "test", logs, cfg, scanCfg, &pipeline)

	if result != nil {
		t.Error("Expected nil result when LLM init fails")
	}

	// After failure, scanCfg.dryRun should be set to true
	if !scanCfg.dryRun {
		t.Error("Expected scanCfg.dryRun to be set to true after LLM init failure")
	}
}

// TestHandleReportingAndKnowledge tests reporting and KB updates
func TestHandleReportingAndKnowledge(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.verbose = false

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Output: config.OutputConfig{
			ReportsDir:       tmpDir + "/reports",
			KnowledgeBaseDir: tmpDir + "/kb",
		},
	}

	// Create directories
	_ = os.MkdirAll(cfg.Output.ReportsDir, 0750)
	_ = os.MkdirAll(cfg.Output.KnowledgeBaseDir+"/services", 0750)

	result := &chunking.AnalyzeResult{
		Analysis:   "Test analysis",
		TokensUsed: 100,
	}

	logs := []docker.LogEntry{
		{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Test"},
	}

	// Should not panic
	handleReportingAndKnowledge("test-container", result, logs, cfg, scanCfg)
}

// TestHandleReportingAndKnowledge_VerboseMode tests verbose output
func TestHandleReportingAndKnowledge_VerboseMode(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.verbose = true

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Output: config.OutputConfig{
			ReportsDir:       tmpDir + "/reports",
			KnowledgeBaseDir: tmpDir + "/kb",
		},
	}

	// Create directories
	_ = os.MkdirAll(cfg.Output.ReportsDir, 0750)
	_ = os.MkdirAll(cfg.Output.KnowledgeBaseDir+"/services", 0750)

	result := &chunking.AnalyzeResult{
		Analysis:   "Test analysis",
		TokensUsed: 100,
	}

	logs := []docker.LogEntry{
		{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Test"},
	}

	handleReportingAndKnowledge("test-container", result, logs, cfg, scanCfg)
}

// TestUpdateGlobalSummary_DryRun tests dry run mode
func TestUpdateGlobalSummary_DryRun(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.dryRun = true

	results := map[string]*chunking.AnalyzeResult{
		"container1": {Analysis: "Test"},
	}

	cfg := &config.Config{}

	err := updateGlobalSummary(results, cfg, scanCfg)

	if err != nil {
		t.Errorf("Expected no error in dry run, got: %v", err)
	}
}

// TestUpdateGlobalSummary_NoResults tests empty results
func TestUpdateGlobalSummary_NoResults(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false

	results := map[string]*chunking.AnalyzeResult{}

	cfg := &config.Config{}

	err := updateGlobalSummary(results, cfg, scanCfg)

	if err != nil {
		t.Errorf("Expected no error with empty results, got: %v", err)
	}
}

// TestUpdateGlobalSummary_Success tests successful update
func TestUpdateGlobalSummary_Success(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false
	scanCfg.verbose = false

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Output: config.OutputConfig{
			KnowledgeBaseDir: tmpDir + "/kb",
		},
	}

	// Create KB directory
	_ = os.MkdirAll(cfg.Output.KnowledgeBaseDir, 0750)

	results := map[string]*chunking.AnalyzeResult{
		"container1": {Analysis: "Test analysis"},
	}

	err := updateGlobalSummary(results, cfg, scanCfg)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

// TestHandleExecutiveSummaryAndNotifications_DryRun tests dry run
func TestHandleExecutiveSummaryAndNotifications_DryRun(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.dryRun = true

	ctx := context.Background()
	results := map[string]*chunking.AnalyzeResult{
		"container1": {Analysis: "Test"},
	}
	cfg := &config.Config{}

	err := handleExecutiveSummaryAndNotifications(ctx, results, cfg, scanCfg)

	if err != nil {
		t.Errorf("Expected no error in dry run, got: %v", err)
	}
}

// TestHandleExecutiveSummaryAndNotifications_NoResults tests empty results
func TestHandleExecutiveSummaryAndNotifications_NoResults(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false

	ctx := context.Background()
	results := map[string]*chunking.AnalyzeResult{}
	cfg := &config.Config{}

	err := handleExecutiveSummaryAndNotifications(ctx, results, cfg, scanCfg)

	if err != nil {
		t.Errorf("Expected no error with empty results, got: %v", err)
	}
}

// TestHandleExecutiveSummaryAndNotifications_LLMInitError tests LLM init error
func TestHandleExecutiveSummaryAndNotifications_LLMInitError(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false

	ctx := context.Background()
	results := map[string]*chunking.AnalyzeResult{
		"container1": {Analysis: "Test"},
	}

	cfg := &config.Config{
		LLM: config.LLMConfig{
			APIKey: "", // No API key
		},
	}

	err := handleExecutiveSummaryAndNotifications(ctx, results, cfg, scanCfg)

	if err == nil {
		t.Error("Expected error when LLM init fails")
	}
}

// TestSendNotificationIfNeeded_Disabled tests disabled notification
func TestSendNotificationIfNeeded_Disabled(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()

	cfg := &config.Config{
		Notification: config.NotificationConfig{
			Enabled: false,
		},
	}

	containerAnalyses := map[string]string{
		"container1": "Test",
	}

	err := sendNotificationIfNeeded("summary", 1, containerAnalyses, cfg, scanCfg)

	if err != nil {
		t.Errorf("Expected no error when notifications disabled, got: %v", err)
	}
}

// TestSendNotificationIfNeeded_InvalidConfig tests invalid config
func TestSendNotificationIfNeeded_InvalidConfig(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()

	cfg := &config.Config{
		Notification: config.NotificationConfig{
			Enabled:    true,
			ShoutrrURL: "", // Invalid URL
		},
	}

	containerAnalyses := map[string]string{
		"container1": "Test",
	}

	err := sendNotificationIfNeeded("summary", 1, containerAnalyses, cfg, scanCfg)

	if err == nil {
		t.Error("Expected error with invalid notification config")
	}
}

// TestGenerateExecutiveSummary_Error tests LLM error
func TestGenerateExecutiveSummary_Error(t *testing.T) {
	ctx := context.Background()

	cfg := &config.Config{
		LLM: config.LLMConfig{
			APIKey:  "test-key",
			Model:   "test-model",
			BaseURL: "http://invalid-url-that-will-fail",
		},
	}

	containerAnalyses := map[string]string{
		"container1": "Test",
	}

	// This will fail because the URL is invalid
	_, err := generateExecutiveSummary(ctx, nil, containerAnalyses, cfg)

	// The function should return an error
	if err == nil {
		t.Error("Expected error from LLM call with invalid URL")
	}
}

// TestSaveStateIfNeeded_Success tests successful state save
func TestSaveStateIfNeeded_Success(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false
	scanCfg.verbose = false

	tmpDir := t.TempDir()
	stateFile := tmpDir + "/state.json"

	st, _ := state.Load(stateFile)
	st.UpdateContainer("test", "test", time.Now(), "")

	err := saveStateIfNeeded(st, scanCfg, 0)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Error("Expected state file to be created")
	}
}

// TestSaveStateIfNeeded_VerboseMode tests verbose output
func TestSaveStateIfNeeded_VerboseMode(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false
	scanCfg.verbose = true

	tmpDir := t.TempDir()
	stateFile := tmpDir + "/state.json"

	st, _ := state.Load(stateFile)

	err := saveStateIfNeeded(st, scanCfg, 0)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

// TestProcessContainers_MultipleContainers tests processing multiple containers
func TestProcessContainers_MultipleContainers(t *testing.T) {
	withScanLLMMock(t, &fakeScanLLM{analysis: "all good"})

	scanCfg := newTestScanConfig()
	scanCfg.verbose = false
	scanCfg.dryRun = false

	ctx := context.Background()

	tmpDir := t.TempDir()
	stateFile := tmpDir + "/state.json"
	st, _ := state.Load(stateFile)

	containers := []docker.Container{
		{ID: "abc123def456abc123def456abc123def456abc123def456abc123def456abc3", Name: "container1", State: "running"},
		{ID: "abc123def456abc123def456abc123def456abc123def456abc123def456abc4", Name: "container2", State: "running"},
		{ID: "abc123def456abc123def456abc123def456abc123def456abc123def456abc5", Name: "container3", State: "running"},
	}

	mockDocker := &MockDockerClient{
		containers: containers,
		logs: map[string][]docker.LogEntry{
			"abc123def456abc123def456abc123def456abc123def456abc123def456abc3": {
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Test1"},
			},
			"abc123def456abc123def456abc123def456abc123def456abc123def456abc4": {
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Test2"},
			},
			"abc123def456abc123def456abc123def456abc123def456abc123def456abc5": {
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Test3"},
			},
		},
	}

	cfg := &config.Config{
		LLM: config.LLMConfig{
			APIKey:        "test-key",
			Model:         "test-model",
			BaseURL:       "http://test",
			ContextWindow: 4000,
		},
		Output: config.OutputConfig{
			ReportsDir:       tmpDir + "/reports",
			KnowledgeBaseDir: tmpDir + "/kb",
		},
	}

	// Create directories
	_ = os.MkdirAll(cfg.Output.ReportsDir, 0750)
	_ = os.MkdirAll(cfg.Output.KnowledgeBaseDir+"/services", 0750)

	results, stats := processContainers(ctx, mockDocker, st, containers, cfg, scanCfg, 0)

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	if stats.scannedContainers != 3 {
		t.Errorf("Expected 3 scanned containers, got %d", stats.scannedContainers)
	}

	if stats.failedContainers != 0 {
		t.Errorf("Expected 0 failed containers, got %d", stats.failedContainers)
	}

	if stats.totalLogs != 3 {
		t.Errorf("Expected 3 total logs, got %d", stats.totalLogs)
	}

	want := time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)
	for _, c := range containers {
		if got, _ := st.GetLastScan(c.ID); !got.Equal(want) {
			t.Errorf("Cursor for %s = %v, want %v", c.Name, got, want)
		}
	}
}

// TestUpdateGlobalSummary_VerboseMode tests verbose mode
func TestUpdateGlobalSummary_VerboseMode(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false
	scanCfg.verbose = true

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Output: config.OutputConfig{
			KnowledgeBaseDir: tmpDir + "/kb",
		},
	}

	// Create KB directory
	_ = os.MkdirAll(cfg.Output.KnowledgeBaseDir, 0750)

	results := map[string]*chunking.AnalyzeResult{
		"container1": {Analysis: "Test analysis"},
	}

	err := updateGlobalSummary(results, cfg, scanCfg)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

// TestHandleExecutiveSummaryAndNotifications_VerboseMode tests verbose mode
func TestHandleExecutiveSummaryAndNotifications_VerboseMode(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false
	scanCfg.verbose = true

	ctx := context.Background()
	results := map[string]*chunking.AnalyzeResult{
		"container1": {Analysis: "Test"},
	}

	cfg := &config.Config{
		LLM: config.LLMConfig{
			APIKey: "", // No API key to trigger early error
		},
	}

	err := handleExecutiveSummaryAndNotifications(ctx, results, cfg, scanCfg)

	if err == nil {
		t.Error("Expected error when LLM init fails")
	}
}

// TestSendNotificationIfNeeded_VerboseMode tests verbose mode
func TestSendNotificationIfNeeded_VerboseMode(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.verbose = true

	cfg := &config.Config{
		Notification: config.NotificationConfig{
			Enabled: false,
		},
	}

	containerAnalyses := map[string]string{
		"container1": "Test",
	}

	err := sendNotificationIfNeeded("summary", 1, containerAnalyses, cfg, scanCfg)

	if err != nil {
		t.Errorf("Expected no error when notifications disabled, got: %v", err)
	}
}

const retryContainerID = "abc123def456abc123def456abc123def456abc123def456abc123def456abc9"

// retryTestEnv builds a state with a cursor, a mock docker with fixed logs and a config
// pointing at temp dirs, for the cursor-on-success tests.
func retryTestEnv(t *testing.T, cursor time.Time) (*state.State, *MockDockerClient, []docker.Container, *config.Config) {
	t.Helper()
	tmpDir := t.TempDir()

	st, err := state.Load(filepath.Join(tmpDir, "state.json"))
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}
	containers := []docker.Container{{ID: retryContainerID, Name: "retry-svc", State: "running"}}
	st.UpdateContainer(retryContainerID, "retry-svc", cursor, "")

	mockDocker := &MockDockerClient{
		containers: containers,
		logs: map[string][]docker.LogEntry{
			retryContainerID: {
				{Timestamp: "2026-01-01T10:00:00Z", Stream: "stdout", Message: "one"},
				{Timestamp: "2026-01-01T10:00:05Z", Stream: "stdout", Message: "two"},
			},
		},
	}
	c := &config.Config{
		LLM: config.LLMConfig{APIKey: "k", Model: "m", BaseURL: "http://test", ContextWindow: 4000},
		Output: config.OutputConfig{
			ReportsDir:       filepath.Join(tmpDir, "reports"),
			KnowledgeBaseDir: filepath.Join(tmpDir, "kb"),
		},
		Scan: config.ScanConfig{MaxWindow: 24 * time.Hour},
	}
	_ = os.MkdirAll(c.Output.ReportsDir, 0o750)
	_ = os.MkdirAll(filepath.Join(c.Output.KnowledgeBaseDir, "services"), 0o750)
	return st, mockDocker, containers, c
}

func TestProcessContainers_FailedAnalysisKeepsCursor(t *testing.T) {
	withScanLLMMock(t, &fakeScanLLM{analysis: "x", failAll: true})
	t0 := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	st, mockDocker, containers, c := retryTestEnv(t, t0)
	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false

	results, stats := processContainers(context.Background(), mockDocker, st, containers, c, scanCfg, 0)

	if got, _ := st.GetLastScan(retryContainerID); !got.Equal(t0) {
		t.Errorf("Cursor moved to %v, want %v", got, t0)
	}
	if stats.failedContainers != 1 {
		t.Errorf("Expected 1 failed container, got %d", stats.failedContainers)
	}
	if _, ok := results["retry-svc"]; ok {
		t.Error("Failed result must not be in globalResults")
	}
}

func TestProcessContainers_SuccessAdvancesCursorAndAddsGapNote(t *testing.T) {
	withScanLLMMock(t, &fakeScanLLM{analysis: "all good"})
	oldCursor := time.Now().Add(-48 * time.Hour)
	st, mockDocker, containers, c := retryTestEnv(t, oldCursor)
	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false
	read := captureStdout(t)

	results, stats := processContainers(context.Background(), mockDocker, st, containers, c, scanCfg, 0)
	out := read()

	want := time.Date(2026, 1, 1, 10, 0, 5, 0, time.UTC)
	if got, _ := st.GetLastScan(retryContainerID); !got.Equal(want) {
		t.Errorf("Cursor = %v, want %v", got, want)
	}
	if stats.failedContainers != 0 {
		t.Errorf("Expected no failures, got %d", stats.failedContainers)
	}
	res := results["retry-svc"]
	if res == nil {
		t.Fatal("Expected result in globalResults")
	}
	gapNotes := 0
	for _, n := range res.CoverageNotes {
		if strings.HasPrefix(n, "Skipped log gap ") && strings.Contains(n, "scan.max_window=24h0m0s") {
			gapNotes++
		}
	}
	if gapNotes != 1 {
		t.Errorf("Expected exactly one gap note, got %d in %v", gapNotes, res.CoverageNotes)
	}
	if n := strings.Count(out, "Skipped log gap"); n != 1 {
		t.Errorf("Expected gap printed once, got %d:\n%s", n, out)
	}

	found := false
	_ = filepath.WalkDir(c.Output.ReportsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // best-effort walk
		}
		if data, rerr := os.ReadFile(path); rerr == nil && strings.Contains(string(data), "## Coverage") {
			found = true
		}
		return nil
	})
	if !found {
		t.Error("Expected report with '## Coverage' section")
	}
}
