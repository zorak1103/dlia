package cmd

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/zorak1103/dlia/internal/chunking"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/state"
)

// TestProcessContainers_Success tests successful container processing
func TestProcessContainers_Success(t *testing.T) {
	t.Parallel()

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
			APIKey:    "test-key",
			Model:     "test-model",
			BaseURL:   "http://test",
			MaxTokens: 4000,
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

	if len(results) != 0 {
		t.Errorf("Expected 0 results without LLM, got %d", len(results))
	}

	if stats.scannedContainers != 1 {
		t.Errorf("Expected 1 scanned container, got %d", stats.scannedContainers)
	}

	if stats.totalLogs != 1 {
		t.Errorf("Expected 1 total logs, got %d", stats.totalLogs)
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
			APIKey:    "test-key",
			Model:     "test-model",
			BaseURL:   "http://test",
			MaxTokens: 4000,
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
			APIKey:    "test-key",
			Model:     "test-model",
			BaseURL:   "http://test",
			MaxTokens: 4000,
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
	t.Parallel()

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
			APIKey:    "test-key",
			Model:     "test-model",
			BaseURL:   "http://test",
			MaxTokens: 4000,
		},
		Output: config.OutputConfig{
			ReportsDir:       tmpDir + "/reports",
			KnowledgeBaseDir: tmpDir + "/kb",
		},
	}

	// Create directories
	_ = os.MkdirAll(cfg.Output.ReportsDir, 0750)
	_ = os.MkdirAll(cfg.Output.KnowledgeBaseDir+"/services", 0750)

	_, stats := processContainers(ctx, mockDocker, st, containers, cfg, scanCfg, 0)

	if stats.scannedContainers != 3 {
		t.Errorf("Expected 3 scanned containers, got %d", stats.scannedContainers)
	}

	if stats.totalLogs != 3 {
		t.Errorf("Expected 3 total logs, got %d", stats.totalLogs)
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
