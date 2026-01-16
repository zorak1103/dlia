package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zorak1103/dlia/internal/chunking"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/state"
)

const testContainerID = "test-container"

func TestParseLookbackDuration_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{"one hour", "1h", time.Hour},
		{"24 hours", "24h", 24 * time.Hour},
		{"30 minutes", "30m", 30 * time.Minute},
		{"1 hour 30 min", "1h30m", 90 * time.Minute},
		{"2 days", "48h", 48 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scanCfg := newTestScanConfig()
			scanCfg.lookback = tt.input
			duration, err := parseLookbackDuration(scanCfg)

			if err != nil {
				t.Errorf("Expected no error for valid duration %s, got: %v", tt.input, err)
			}

			if duration != tt.expected {
				t.Errorf("Expected duration %v, got %v", tt.expected, duration)
			}
		})
	}
}

func TestParseLookbackDuration_Invalid(t *testing.T) {
	t.Parallel()

	tests := []string{
		"invalid",
		"1x",
		"abc",
		"",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			scanCfg := newTestScanConfig()
			scanCfg.lookback = input

			if input == "" {
				duration, err := parseLookbackDuration(scanCfg)
				if err != nil {
					t.Errorf("Expected no error for empty duration, got: %v", err)
				}
				if duration != 0 {
					t.Errorf("Expected 0 duration for empty string, got: %v", duration)
				}
				return
			}

			_, err := parseLookbackDuration(scanCfg)

			if err == nil {
				t.Errorf("Expected error for invalid duration %s", input)
			}

			if !strings.Contains(err.Error(), "invalid lookback duration") {
				t.Errorf("Expected 'invalid lookback duration' in error, got: %v", err)
			}
		})
	}
}

func TestDetermineLogStartTime_WithLookback(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.lookback = "2h"

	st := &state.State{}
	lookbackDuration := 2 * time.Hour

	since := determineLogStartTime(st, testContainerID, scanCfg, lookbackDuration)

	// Should be approximately 2 hours ago
	expected := time.Now().Add(-lookbackDuration)
	diff := since.Sub(expected)
	if diff > time.Second || diff < -time.Second {
		t.Errorf("Expected time around %v, got %v (diff: %v)", expected, since, diff)
	}
}

func TestDetermineLogStartTime_WithState(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()

	// Create a temporary state file
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/state.json"

	st, err := state.Load(stateFile)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	lastScan := time.Now().Add(-30 * time.Minute)

	st.UpdateContainer(testContainerID, "test", lastScan, "")

	since := determineLogStartTime(st, testContainerID, scanCfg, 0)

	if !since.Equal(lastScan) {
		t.Errorf("Expected time %v, got %v", lastScan, since)
	}
}

func TestDetermineLogStartTime_FirstScan(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()

	// Create a temporary state file
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/state.json"

	st, err := state.Load(stateFile)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	since := determineLogStartTime(st, testContainerID, scanCfg, 0)

	// Should be approximately 1 hour ago for first scan
	expected := time.Now().Add(-1 * time.Hour)
	diff := since.Sub(expected)
	if diff > time.Second || diff < -time.Second {
		t.Errorf("Expected time around %v (1 hour ago), got %v (diff: %v)", expected, since, diff)
	}
}

func TestDisplayLogsPreview_VerboseMode(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.verbose = true

	logs := []docker.LogEntry{
		{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Line 1"},
		{Timestamp: "2023-01-01T10:00:01Z", Stream: "stdout", Message: "Line 2"},
		{Timestamp: "2023-01-01T10:00:02Z", Stream: "stdout", Message: "Line 3"},
	}

	// This function prints to stdout, so we can't easily capture it
	// But we can at least verify it doesn't panic
	displayLogsPreview(logs, scanCfg)
}

func TestDisplayLogsPreview_ManyLogs(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.verbose = true

	// Create more than 10 logs
	logs := make([]docker.LogEntry, 15)
	for i := range logs {
		logs[i] = docker.LogEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Stream:    "stdout",
			Message:   "Log message",
		}
	}

	// Should truncate to 10 and show "... (5 more lines)"
	displayLogsPreview(logs, scanCfg)
}

func TestDisplayLogsPreview_NonVerbose(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.verbose = false

	logs := []docker.LogEntry{
		{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Line 1"},
	}

	// Should not print anything
	displayLogsPreview(logs, scanCfg)
}

func TestDisplayAnalysisResults_WithResult(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.verbose = true

	result := &chunking.AnalyzeResult{
		Analysis:       "Test analysis\nMultiple lines\nOf output",
		TokensUsed:     100,
		ChunksUsed:     1,
		Deduplicated:   true,
		OriginalCount:  50,
		ProcessedCount: 25,
	}

	// This function prints to stdout
	displayAnalysisResults(result, scanCfg)
}

func TestDisplayAnalysisResults_MultipleChunks(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.verbose = true

	result := &chunking.AnalyzeResult{
		Analysis:   "Chunked analysis",
		TokensUsed: 500,
		ChunksUsed: 3,
	}

	displayAnalysisResults(result, scanCfg)
}

func TestDisplayAnalysisResults_WithFilterStats(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.filterStats = true
	scanCfg.verbose = true

	result := &chunking.AnalyzeResult{
		Analysis:       "Analysis with filtering",
		TokensUsed:     200,
		ChunksUsed:     1,
		Deduplicated:   false,
		OriginalCount:  100,
		ProcessedCount: 100,
		FilterStats: chunking.FilterStats{
			LinesTotal:    1000,
			LinesFiltered: 250,
		},
	}

	// This function prints to stdout including filter statistics
	displayAnalysisResults(result, scanCfg)

	// Verify the filterStats path was executed (function doesn't panic)
	// The actual output contains: "🔍 Regexp Filter: Filtered 250/1000 log lines (25.0%)"
}

func TestUpdateContainerState_WithLogs(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.verbose = false
	scanCfg.dryRun = false

	// Create a temporary state file
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/state.json"

	st, err := state.Load(stateFile)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	container := docker.Container{
		ID:   "test123",
		Name: "test-container",
	}

	logs := []docker.LogEntry{
		{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "First"},
		{Timestamp: "2023-01-01T10:00:05Z", Stream: "stdout", Message: "Last"},
	}

	updateContainerState(st, container, logs, scanCfg, 0)

	// Verify state was updated
	lastScan, exists := st.GetLastScan(container.ID)
	if !exists {
		t.Error("Expected container to be in state")
	}

	// The latest timestamp should be parsed and stored
	expected, _ := time.Parse(time.RFC3339, "2023-01-01T10:00:05Z")
	if !lastScan.Equal(expected) {
		t.Errorf("Expected last scan %v, got %v", expected, lastScan)
	}
}

func TestUpdateContainerState_NoLogs(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()

	// Create a temporary state file
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/state.json"

	st, err := state.Load(stateFile)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	container := docker.Container{
		ID:   "test123",
		Name: "test-container",
	}

	var logs []docker.LogEntry

	updateContainerState(st, container, logs, scanCfg, 0)

	// State should not be updated when there are no logs
	_, exists := st.GetLastScan(container.ID)
	if exists {
		t.Error("Expected container not to be in state when there are no logs")
	}
}

func TestUpdateContainerState_DryRun(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.verbose = false
	scanCfg.dryRun = true

	// Create a temporary state file
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/state.json"

	st, err := state.Load(stateFile)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	container := docker.Container{
		ID:   "test123",
		Name: "test-container",
	}

	logs := []docker.LogEntry{
		{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Test"},
	}

	updateContainerState(st, container, logs, scanCfg, 0)

	// State should not be updated in dry run mode
	_, exists := st.GetLastScan(container.ID)
	if exists {
		t.Error("Expected container not to be in state during dry run")
	}
}

func TestUpdateContainerState_LookbackMode(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.verbose = false
	scanCfg.dryRun = false

	// Create a temporary state file
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/state.json"

	st, err := state.Load(stateFile)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	container := docker.Container{
		ID:   "test123",
		Name: "test-container",
	}

	logs := []docker.LogEntry{
		{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Test"},
	}

	lookbackDuration := 1 * time.Hour

	updateContainerState(st, container, logs, scanCfg, lookbackDuration)

	// State should not be updated in lookback mode
	_, exists := st.GetLastScan(container.ID)
	if exists {
		t.Error("Expected container not to be in state during lookback mode")
	}
}

func TestDetectIssues_WithIssues(t *testing.T) {
	tests := []struct {
		name     string
		analyses map[string]string
		expected bool
	}{
		{
			name: "contains error",
			analyses: map[string]string{
				"container1": "Everything is fine but there's an error here",
			},
			expected: true,
		},
		{
			name: "contains warning",
			analyses: map[string]string{
				"container1": "Warning: something happened",
			},
			expected: true,
		},
		{
			name: "contains exception",
			analyses: map[string]string{
				"container1": "Exception thrown during processing",
			},
			expected: true,
		},
		{
			name: "contains failed",
			analyses: map[string]string{
				"container1": "Operation failed successfully",
			},
			expected: true,
		},
		{
			name: "contains critical",
			analyses: map[string]string{
				"container1": "Critical issue detected",
			},
			expected: true,
		},
		{
			name: "no issues",
			analyses: map[string]string{
				"container1": "Everything is running smoothly",
				"container2": "All systems operational",
			},
			expected: false,
		},
		{
			name: "case insensitive",
			analyses: map[string]string{
				"container1": "ERROR in line 42",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectIssues(tt.analyses)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestDisplayNoContainersFound(t *testing.T) {
	t.Parallel()

	// Test with no filter
	scanCfg := newTestScanConfig()
	displayNoContainersFound(scanCfg)

	// Test with filter
	scanCfg.filter = "nginx.*"
	displayNoContainersFound(scanCfg)
}

func TestProcessLLMAnalysis_DryRun(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.dryRun = true

	ctx := context.Background()
	containerName := testContainerID
	logs := []docker.LogEntry{
		{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Test"},
	}
	cfg := &config.Config{
		LLM: config.LLMConfig{
			APIKey:    "test-key",
			Model:     "test-model",
			BaseURL:   "http://test",
			MaxTokens: 4000,
		},
	}
	var pipeline *chunking.Pipeline

	result := processLLMAnalysis(ctx, containerName, logs, cfg, scanCfg, &pipeline)

	if result != nil {
		t.Error("Expected nil result in dry run mode")
	}

	if pipeline != nil {
		t.Error("Expected pipeline not to be initialized in dry run mode")
	}
}

func TestProcessLLMAnalysis_PipelineInitializationFails(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false

	ctx := context.Background()
	containerName := "test-container"
	logs := []docker.LogEntry{
		{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Test"},
	}

	// Config without API key should cause initialization to fail
	cfg := &config.Config{
		LLM: config.LLMConfig{
			APIKey:    "", // Missing API key
			Model:     "test-model",
			BaseURL:   "http://test",
			MaxTokens: 4000,
		},
	}

	var pipeline *chunking.Pipeline
	result := processLLMAnalysis(ctx, containerName, logs, cfg, scanCfg, &pipeline)

	// Should return nil when pipeline initialization fails
	if result != nil {
		t.Error("Expected nil result when pipeline initialization fails")
	}

	// Pipeline should remain nil
	if pipeline != nil {
		t.Error("Expected pipeline to remain nil after initialization failure")
	}

	// After initialization failure, scanCfg.dryRun should be set to true
	if !scanCfg.dryRun {
		t.Error("Expected scanCfg.dryRun to be set to true after pipeline initialization failure")
	}
}

func TestInitializeLLMPipeline_NoAPIKey(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()

	cfg := &config.Config{
		LLM: config.LLMConfig{
			APIKey:    "",
			Model:     "test-model",
			BaseURL:   "http://test",
			MaxTokens: 4000,
		},
	}

	_, err := initializeLLMPipeline(cfg, scanCfg)

	if err == nil {
		t.Error("Expected error when API key is not configured")
	}

	if !strings.Contains(err.Error(), "API key not configured") {
		t.Errorf("Expected 'API key not configured' error, got: %v", err)
	}
}

func TestInitializeLLMPipeline_WithAPIKey(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()

	cfg := &config.Config{
		LLM: config.LLMConfig{
			APIKey:    "test-key",
			Model:     "test-model",
			BaseURL:   "http://test",
			MaxTokens: 4000,
		},
	}

	pipeline, err := initializeLLMPipeline(cfg, scanCfg)

	if err != nil {
		t.Errorf("Expected no error with valid config, got: %v", err)
	}

	if pipeline == nil {
		t.Error("Expected pipeline to be created")
	}
}

func TestInitializeLLMPipeline_WithLLMLogging(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.llmLog = true
	scanCfg.verbose = true

	// Create temporary directory for LLM logs
	tmpDir := t.TempDir()

	cfg := &config.Config{
		LLM: config.LLMConfig{
			APIKey:    "test-key",
			Model:     "test-model",
			BaseURL:   "http://test",
			MaxTokens: 4000,
		},
		Output: config.OutputConfig{
			LLMLogDir:     tmpDir,
			LLMLogEnabled: false, // Flag takes precedence
		},
	}

	pipeline, err := initializeLLMPipeline(cfg, scanCfg)

	if err != nil {
		t.Errorf("Expected no error with valid config and LLM logging, got: %v", err)
	}

	if pipeline == nil {
		t.Error("Expected pipeline to be created with LLM logging enabled")
	}

	// Verify the LLM logging path was executed (function doesn't panic)
	// The actual code initializes an llmlogger when llmLog flag is true
}

func TestSaveStateIfNeeded_DryRun(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.dryRun = true

	// Create a temporary state file
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/state.json"

	st, err := state.Load(stateFile)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	lookbackDuration := time.Duration(0)

	err = saveStateIfNeeded(st, scanCfg, lookbackDuration)

	if err != nil {
		t.Errorf("Expected no error in dry run, got: %v", err)
	}
}

func TestSaveStateIfNeeded_LookbackMode(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.dryRun = false

	// Create a temporary state file
	tmpDir := t.TempDir()
	stateFile := tmpDir + "/state.json"

	st, err := state.Load(stateFile)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	lookbackDuration := 1 * time.Hour

	err = saveStateIfNeeded(st, scanCfg, lookbackDuration)

	if err != nil {
		t.Errorf("Expected no error in lookback mode, got: %v", err)
	}
}

func TestValidateAndFilterContainers_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		namePattern    string
		mockContainers []docker.Container
		expectedCount  int
		expectedNames  []string
	}{
		{
			name:        "with matching containers",
			namePattern: "nginx.*",
			mockContainers: []docker.Container{
				{ID: "c1", Name: "nginx-web", State: "running"},
				{ID: "c2", Name: "nginx-api", State: "running"},
				{ID: "c3", Name: "app-server", State: "running"},
			},
			expectedCount: 3,
			expectedNames: []string{"nginx-web", "nginx-api", "app-server"},
		},
		{
			name:           "with no matching containers - empty result",
			namePattern:    "test.*",
			mockContainers: []docker.Container{},
			expectedCount:  0,
			expectedNames:  []string{},
		},
		{
			name:        "with all containers matching",
			namePattern: "",
			mockContainers: []docker.Container{
				{ID: "c1", Name: testContainerID, State: "running"},
				{ID: "c2", Name: "container2", State: "exited"},
				{ID: "c3", Name: "container3", State: "running"},
			},
			expectedCount: 3,
			expectedNames: []string{testContainerID, "container2", "container3"},
		},
		{
			name:        "with specific pattern filter",
			namePattern: "app-.*",
			mockContainers: []docker.Container{
				{ID: "c1", Name: "app-web", State: "running"},
				{ID: "c2", Name: "db-postgres", State: "running"},
				{ID: "c3", Name: "app-api", State: "running"},
			},
			expectedCount: 3,
			expectedNames: []string{"app-web", "db-postgres", "app-api"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			mockDocker := &MockDockerClient{
				containers: tt.mockContainers,
			}

			containers, err := validateAndFilterContainers(ctx, mockDocker, tt.namePattern)

			if err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}

			if len(containers) != tt.expectedCount {
				t.Errorf("Expected %d containers, got %d", tt.expectedCount, len(containers))
			}

			// Verify container names match expected
			for i, container := range containers {
				if i < len(tt.expectedNames) && container.Name != tt.expectedNames[i] {
					t.Errorf("Expected container name %s at index %d, got %s", tt.expectedNames[i], i, container.Name)
				}
			}
		})
	}
}

func TestValidateAndFilterContainers_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		namePattern string
		dockerError error
		expectedErr string
	}{
		{
			name:        "docker connection failed",
			namePattern: "",
			dockerError: docker.ErrConnectionFailed,
			expectedErr: "failed to list containers",
		},
		{
			name:        "docker permission denied",
			namePattern: "test.*",
			dockerError: fmt.Errorf("permission denied"),
			expectedErr: "failed to list containers",
		},
		{
			name:        "docker timeout",
			namePattern: "app-.*",
			dockerError: context.DeadlineExceeded,
			expectedErr: "failed to list containers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			mockDocker := &MockDockerClient{
				listErr: tt.dockerError,
			}

			containers, err := validateAndFilterContainers(ctx, mockDocker, tt.namePattern)

			if err == nil {
				t.Error("Expected error from Docker client")
			}

			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Errorf("Expected error containing '%s', got: %v", tt.expectedErr, err)
			}

			if containers != nil {
				t.Errorf("Expected nil containers on error, got: %v", containers)
			}
		})
	}
}

func TestGetContainersToScan_Success(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()

	ctx := context.Background()
	mockDocker := &MockDockerClient{
		containers: []docker.Container{
			{ID: "c1", Name: testContainerID, State: "running"},
			{ID: "c2", Name: "container2", State: "running"},
		},
	}

	containers, err := getContainersToScan(ctx, mockDocker, scanCfg)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if len(containers) != 2 {
		t.Errorf("Expected 2 containers, got %d", len(containers))
	}
}

func TestProcessContainerLogs_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		containerID   string
		mockLogs      []docker.LogEntry
		since         time.Time
		expectedCount int
	}{
		{
			name:        "with multiple log entries",
			containerID: "abc123456789",
			mockLogs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Log line 1"},
				{Timestamp: "2023-01-01T10:00:01Z", Stream: "stdout", Message: "Log line 2"},
				{Timestamp: "2023-01-01T10:00:02Z", Stream: "stderr", Message: "Error line 1"},
			},
			since:         time.Now().Add(-1 * time.Hour),
			expectedCount: 3,
		},
		{
			name:          "with no log entries",
			containerID:   "def456789012",
			mockLogs:      []docker.LogEntry{},
			since:         time.Now().Add(-30 * time.Minute),
			expectedCount: 0,
		},
		{
			name:        "with single log entry",
			containerID: "ghi789012345",
			mockLogs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Single log"},
			},
			since:         time.Now().Add(-2 * time.Hour),
			expectedCount: 1,
		},
		{
			name:        "with mixed streams stdout and stderr",
			containerID: "jkl012345678",
			mockLogs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Info message"},
				{Timestamp: "2023-01-01T10:00:01Z", Stream: "stderr", Message: "Error message"},
				{Timestamp: "2023-01-01T10:00:02Z", Stream: "stdout", Message: "Another info"},
			},
			since:         time.Now().Add(-15 * time.Minute),
			expectedCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			mockDocker := &MockDockerClient{
				logs: map[string][]docker.LogEntry{
					tt.containerID: tt.mockLogs,
				},
			}

			logs, err := processContainerLogs(ctx, mockDocker, tt.containerID, tt.since)

			if err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}

			if len(logs) != tt.expectedCount {
				t.Errorf("Expected %d log entries, got %d", tt.expectedCount, len(logs))
			}

			// Verify logs match expected
			for i, log := range logs {
				if i < len(tt.mockLogs) {
					if log.Message != tt.mockLogs[i].Message {
						t.Errorf("Expected log message '%s' at index %d, got '%s'", tt.mockLogs[i].Message, i, log.Message)
					}
					if log.Stream != tt.mockLogs[i].Stream {
						t.Errorf("Expected stream '%s' at index %d, got '%s'", tt.mockLogs[i].Stream, i, log.Stream)
					}
				}
			}
		})
	}
}

func TestProcessContainerLogs_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		containerID string
		dockerError error
		expectedErr string
	}{
		{
			name:        "docker read logs failed",
			containerID: "abc123456789",
			dockerError: fmt.Errorf("connection timeout"),
			expectedErr: "failed to read logs for container abc123456789",
		},
		{
			name:        "container not found",
			containerID: "nonexistent123",
			dockerError: fmt.Errorf("no such container"),
			expectedErr: "failed to read logs for container nonexistent",
		},
		{
			name:        "permission denied",
			containerID: "restricted456",
			dockerError: fmt.Errorf("access denied"),
			expectedErr: "failed to read logs for container restricted4",
		},
		{
			name:        "context canceled",
			containerID: "canceled789012",
			dockerError: context.Canceled,
			expectedErr: "failed to read logs for container canceled789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			mockDocker := &MockDockerClient{
				logsErr: tt.dockerError,
			}

			logs, err := processContainerLogs(ctx, mockDocker, tt.containerID, time.Now())

			if err == nil {
				t.Error("Expected error from Docker client")
			}

			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Errorf("Expected error containing '%s', got: %v", tt.expectedErr, err)
			}

			if logs != nil {
				t.Errorf("Expected nil logs on error, got: %v", logs)
			}
		})
	}
}

func TestGetContainersToScan_Error(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()

	ctx := context.Background()
	mockDocker := &MockDockerClient{
		listErr: docker.ErrConnectionFailed,
	}

	_, err := getContainersToScan(ctx, mockDocker, scanCfg)

	if err == nil {
		t.Error("Expected error from Docker client")
	}

	if !strings.Contains(err.Error(), "failed to list containers") {
		t.Errorf("Expected 'failed to list containers' error, got: %v", err)
	}
}

func TestDisplayScanSummary(t *testing.T) {
	tests := []struct {
		name             string
		dryRun           bool
		lookbackDuration time.Duration
	}{
		{"normal mode", false, 0},
		{"dry run mode", true, 0},
		{"lookback mode", false, 1 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			scanCfg := newTestScanConfig()
			scanCfg.dryRun = tt.dryRun

			stats := scanStats{
				totalLogs:         100,
				scannedContainers: 5,
			}

			// This function prints to stdout
			displayScanSummary(stats, scanCfg, tt.lookbackDuration)
		})
	}
}

func TestDisplayScanHeader(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			Model: "test-model",
		},
		Docker: config.DockerConfig{
			SocketPath: "unix:///var/run/docker.sock",
		},
		Output: config.OutputConfig{
			StateFile: "state.json",
		},
	}

	tests := []struct {
		name             string
		verbose          bool
		dryRun           bool
		filter           string
		lookbackDuration time.Duration
	}{
		{"verbose with filter and lookback", true, false, "nginx.*", 1 * time.Hour},
		{"non-verbose", false, false, "", 0},
		{"dry run", false, true, "", 0},
		{"verbose dry run", true, true, "app-.*", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			scanCfg := newTestScanConfig()
			scanCfg.verbose = tt.verbose
			scanCfg.dryRun = tt.dryRun
			scanCfg.filter = tt.filter

			// This function prints to stdout
			displayScanHeader(cfg, scanCfg, tt.lookbackDuration)
		})
	}
}

func TestGenerateAndSaveReport_Success(t *testing.T) {
	// Create temporary directory for reports
	tmpDir := t.TempDir()

	// Save and restore original verbose flag
	originalVerbose := verbose
	t.Cleanup(func() { verbose = originalVerbose })

	tests := []struct {
		name          string
		containerName string
		verbose       bool
		result        *chunking.AnalyzeResult
		logs          []docker.LogEntry
	}{
		{
			name:          "basic report with verbose mode",
			containerName: "test-container",
			verbose:       true,
			result: &chunking.AnalyzeResult{
				Analysis:       "Test analysis output",
				TokensUsed:     150,
				ChunksUsed:     1,
				Deduplicated:   false,
				OriginalCount:  10,
				ProcessedCount: 10,
			},
			logs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Log line 1"},
			},
		},
		{
			name:          "report with deduplication stats",
			containerName: "dedupe-container",
			verbose:       false,
			result: &chunking.AnalyzeResult{
				Analysis:       "Deduplicated analysis",
				TokensUsed:     200,
				ChunksUsed:     2,
				Deduplicated:   true,
				OriginalCount:  100,
				ProcessedCount: 50,
			},
			logs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Repeated log"},
				{Timestamp: "2023-01-01T10:00:01Z", Stream: "stdout", Message: "Repeated log"},
			},
		},
		{
			name:          "container with special characters",
			containerName: "test/container-name",
			verbose:       true,
			result: &chunking.AnalyzeResult{
				Analysis:       "Special character test",
				TokensUsed:     100,
				ChunksUsed:     1,
				OriginalCount:  5,
				ProcessedCount: 5,
			},
			logs: []docker.LogEntry{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanCfg := newTestScanConfig()
			scanCfg.verbose = tt.verbose

			cfg := &config.Config{
				Output: config.OutputConfig{
					ReportsDir: tmpDir,
				},
			}

			reportPath, err := generateAndSaveReport(tt.containerName, tt.result, tt.logs, cfg, scanCfg)

			if err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}

			if reportPath == "" {
				t.Error("Expected non-empty report path")
			}

			// Verify file was created
			if _, err := os.Stat(reportPath); os.IsNotExist(err) {
				t.Errorf("Report file was not created at: %s", reportPath)
			}

			// Verify file contains expected content
			// #nosec G304 - reportPath is generated by the test and safe for testing purposes
			content, err := os.ReadFile(reportPath)
			if err != nil {
				t.Errorf("Failed to read report file: %v", err)
			}

			contentStr := string(content)

			// Check for basic report elements
			if !strings.Contains(contentStr, tt.containerName) {
				t.Errorf("Report does not contain container name: %s", tt.containerName)
			}

			if !strings.Contains(contentStr, tt.result.Analysis) {
				t.Error("Report does not contain analysis output")
			}

			if !strings.Contains(contentStr, fmt.Sprintf("**Tokens Used:** %d", tt.result.TokensUsed)) {
				t.Error("Report does not contain token usage")
			}
		})
	}
}

func TestGenerateAndSaveReport_DirectoryCreationError(t *testing.T) {
	t.Parallel()

	scanCfg := newTestScanConfig()
	scanCfg.verbose = false

	containerName := "test-container"
	result := &chunking.AnalyzeResult{
		Analysis:       "Test analysis",
		TokensUsed:     100,
		ChunksUsed:     1,
		OriginalCount:  5,
		ProcessedCount: 5,
	}
	logs := []docker.LogEntry{}

	// Use an invalid path that will fail directory creation
	// On Unix systems, /dev/null/reports should fail
	// On Windows, we use a path with invalid characters
	invalidPath := "/dev/null/reports"
	if os.PathSeparator == '\\' {
		// Windows - use invalid characters
		invalidPath = "C:\\invalid<>path\\reports"
	}

	cfg := &config.Config{
		Output: config.OutputConfig{
			ReportsDir: invalidPath,
		},
	}

	reportPath, err := generateAndSaveReport(containerName, result, logs, cfg, scanCfg)

	if err == nil {
		t.Error("Expected error when directory creation fails")
	}

	if reportPath != "" {
		t.Errorf("Expected empty report path on error, got: %s", reportPath)
	}

	if !strings.Contains(err.Error(), "failed to save report") {
		t.Errorf("Expected 'failed to save report' in error, got: %v", err)
	}

	if !strings.Contains(err.Error(), containerName) {
		t.Errorf("Expected container name in error, got: %v", err)
	}
}

func TestGenerateAndSaveReport_VerboseOutput(t *testing.T) {
	// Create temporary directory for reports
	tmpDir := t.TempDir()

	// Save and restore original verbose flag
	originalVerbose := verbose
	defer func() { verbose = originalVerbose }()

	tests := []struct {
		name    string
		verbose bool
	}{
		{"verbose mode enabled", true},
		{"verbose mode disabled", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanCfg := newTestScanConfig()
			scanCfg.verbose = tt.verbose

			containerName := "verbose-test-container"
			result := &chunking.AnalyzeResult{
				Analysis:       "Verbose test",
				TokensUsed:     50,
				ChunksUsed:     1,
				OriginalCount:  3,
				ProcessedCount: 3,
			}
			logs := []docker.LogEntry{}

			cfg := &config.Config{
				Output: config.OutputConfig{
					ReportsDir: tmpDir,
				},
			}

			reportPath, err := generateAndSaveReport(containerName, result, logs, cfg, scanCfg)

			if err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}

			if reportPath == "" {
				t.Error("Expected non-empty report path")
			}

			// Note: We can't easily capture stdout to verify verbose output
			// but we can verify the function completes successfully
		})
	}
}
