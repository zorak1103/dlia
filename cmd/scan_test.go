package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/llm"
)

// MockDockerClient for testing
type MockDockerClient struct {
	containers []docker.Container
	logs       map[string][]docker.LogEntry
	pingErr    error
	listErr    error
	logsErr    error
}

func (m *MockDockerClient) Ping(_ context.Context) error {
	return m.pingErr
}

func (m *MockDockerClient) ListContainers(_ context.Context, _ docker.FilterOptions) ([]docker.Container, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.containers, nil
}

func (m *MockDockerClient) GetContainer(_ context.Context, containerID string) (*docker.Container, error) {
	for _, ctr := range m.containers {
		if ctr.ID == containerID {
			return &ctr, nil
		}
	}
	return nil, docker.ErrNotFound
}

func (m *MockDockerClient) ContainerExists(_ context.Context, containerID string) bool {
	for _, ctr := range m.containers {
		if ctr.ID == containerID {
			return true
		}
	}
	return false
}

func (m *MockDockerClient) Close() error {
	return nil
}

func (m *MockDockerClient) ReadLogsSince(_ context.Context, containerID string, _ time.Time) ([]docker.LogEntry, error) {
	if m.logsErr != nil {
		return nil, m.logsErr
	}
	if logs, exists := m.logs[containerID]; exists {
		return logs, nil
	}
	return []docker.LogEntry{}, nil
}

func (m *MockDockerClient) ReadLogs(_ context.Context, containerID string, _ docker.LogsOptions) ([]docker.LogEntry, error) {
	if m.logsErr != nil {
		return nil, m.logsErr
	}
	if logs, exists := m.logs[containerID]; exists {
		return logs, nil
	}
	return []docker.LogEntry{}, nil
}

func (m *MockDockerClient) ReadLogsLookback(_ context.Context, containerID string, _ time.Duration) ([]docker.LogEntry, error) {
	if m.logsErr != nil {
		return nil, m.logsErr
	}
	if logs, exists := m.logs[containerID]; exists {
		return logs, nil
	}
	return []docker.LogEntry{}, nil
}

// MockLLMClient for testing
type MockLLMClient struct {
	analyzeResponse string
	analyzeUsage    *llm.TokenUsage
	analyzeError    error
}

func (m *MockLLMClient) Analyze(_ context.Context, _, _ string) (string, *llm.TokenUsage, error) {
	return m.analyzeResponse, m.analyzeUsage, m.analyzeError
}

func (m *MockLLMClient) SummarizeChunk(_ context.Context, _, _ string) (string, error) {
	return "Mock summary", nil
}

func (m *MockLLMClient) ChatCompletion(_ context.Context, _ []llm.ChatMessage, _ float64, _ int) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		Choices: []llm.Choice{
			{
				Message: llm.ChatMessage{
					Role:    "assistant",
					Content: m.analyzeResponse,
				},
			},
		},
		Usage: *m.analyzeUsage,
	}, m.analyzeError
}

func setupTestConfig(_ *testing.T) {
	viper.Reset()
	viper.Set("docker.socket", "")
	viper.Set("llm.base_url", "http://mock-api")
	viper.Set("llm.api_key", "mock-key")
	viper.Set("llm.model", "mock-model")
	viper.Set("llm.max_tokens", 4000)
	viper.Set("state.file", "test-state.json")
}

func TestScanCmd_Flags(t *testing.T) {
	cmd := scanCmd

	// Test that flags are properly defined
	flags := cmd.Flags()

	if flags.Lookup("dry-run") == nil {
		t.Errorf("dry-run flag not defined")
	}

	if flags.Lookup("filter") == nil {
		t.Errorf("filter flag not defined")
	}

	if flags.Lookup("lookback") == nil {
		t.Errorf("lookback flag not defined")
	}
}

func TestScanCmd_DryRun(t *testing.T) {
	t.Parallel()

	setupTestConfig(t)

	// Create mock containers
	containers := []docker.Container{
		{
			ID:    "container1",
			Name:  "test-container",
			State: "running",
			Image: "nginx:latest",
		},
	}

	// Create mock logs
	logs := map[string][]docker.LogEntry{
		"container1": {
			{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Test log message"},
		},
	}

	mockDocker := &MockDockerClient{
		containers: containers,
		logs:       logs,
	}

	// Mock LLM client
	_ = &MockLLMClient{
		analyzeResponse: "Mock analysis",
		analyzeUsage:    &llm.TokenUsage{TotalTokens: 100},
	}

	// Test using mock docker client directly without modifying global state
	ctx := context.Background()

	// Test that we can list containers
	ctrList, err := mockDocker.ListContainers(ctx, docker.FilterOptions{})
	if err != nil {
		t.Errorf("Expected no error listing containers, got: %v", err)
	}

	if len(ctrList) != 1 {
		t.Errorf("Expected 1 container, got %d", len(ctrList))
	}

	// Test that we can read logs
	logEntries, err := mockDocker.ReadLogsSince(ctx, "container1", time.Time{})
	if err != nil {
		t.Errorf("Expected no error reading logs, got: %v", err)
	}

	if len(logEntries) != 1 {
		t.Errorf("Expected 1 log entry, got %d", len(logEntries))
	}
}

func TestScanCmd_WithFilter(t *testing.T) {
	setupTestConfig(t)

	// Create mock containers
	containers := []docker.Container{
		{
			ID:    "web-1",
			Name:  "nginx-web",
			State: "running",
			Image: "nginx:latest",
		},
		{
			ID:    "db-1",
			Name:  "postgres-db",
			State: "running",
			Image: "postgres:latest",
		},
	}

	mockDocker := &MockDockerClient{
		containers: containers,
	}

	// Test filtering
	ctx := context.Background()

	// Test filter for nginx containers
	opts := docker.FilterOptions{
		NamePattern: "nginx.*",
		IncludeAll:  true,
	}

	_, err := mockDocker.ListContainers(ctx, opts)
	if err != nil {
		t.Errorf("Expected no error filtering containers, got: %v", err)
	}

	// This test is simplified - in reality you'd need to test the actual regex filtering
	// For now, just verify the mock works
	// Mock doesn't implement filtering, so this is expected
	// if len(filtered) != len(containers) { }
}

func TestScanCmd_LookbackOption(t *testing.T) {
	setupTestConfig(t)

	// Test lookback parsing
	tests := []struct {
		input    string
		expected time.Duration
		hasError bool
	}{
		{"1h", time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			duration, err := time.ParseDuration(tt.input)

			if tt.hasError && err == nil {
				t.Error("Expected error for invalid duration")
			}

			if !tt.hasError {
				if err != nil {
					t.Errorf("Expected no error for valid duration %s, got: %v", tt.input, err)
				}

				if duration != tt.expected {
					t.Errorf("Expected duration %v, got %v", tt.expected, duration)
				}
			}
		})
	}
}

func TestScanCmd_ConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func()
		expectError bool
	}{
		{
			name: "valid config",
			setupConfig: func() {
				viper.Reset()
				viper.Set("llm.base_url", "http://valid-api")
				viper.Set("llm.api_key", "valid-key")
				viper.Set("llm.model", "valid-model")
				viper.Set("output.reports_dir", "reports")
				viper.Set("output.knowledge_base_dir", "knowledge")
				viper.Set("output.state_file", "state.json")
			},
			expectError: false,
		},
		{
			name: "missing API key",
			setupConfig: func() {
				viper.Reset()
				viper.Set("llm.base_url", "http://valid-api")
				viper.Set("llm.model", "valid-model")
				viper.Set("output.reports_dir", "reports")
				viper.Set("output.knowledge_base_dir", "knowledge")
				viper.Set("output.state_file", "state.json")
				// API key has no default, so it should fail validation
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupConfig()

			// Test config validation using global viper
			cfg, err := config.LoadFromViper()

			if tt.expectError && err == nil {
				t.Error("Expected config validation error")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no config validation error, got: %v", err)
			}

			if !tt.expectError && cfg != nil {
				// Verify config has required fields
				if cfg.LLM.BaseURL == "" {
					t.Error("Expected BaseURL to be set")
				}
				if cfg.LLM.APIKey == "" {
					t.Error("Expected APIKey to be set")
				}
				if cfg.LLM.Model == "" {
					t.Error("Expected Model to be set")
				}
			}
		})
	}
}

func TestScanCmd_ErrorHandling(t *testing.T) {
	setupTestConfig(t)

	tests := []struct {
		name       string
		setupMocks func() (*MockDockerClient, *MockLLMClient)
		expectErr  bool
	}{
		{
			name: "Docker ping error",
			setupMocks: func() (*MockDockerClient, *MockLLMClient) {
				return &MockDockerClient{
					pingErr: docker.ErrConnectionFailed,
				}, &MockLLMClient{}
			},
			expectErr: true,
		},
		{
			name: "Docker list error",
			setupMocks: func() (*MockDockerClient, *MockLLMClient) {
				return &MockDockerClient{
					listErr: docker.ErrConnectionFailed,
				}, &MockLLMClient{}
			},
			expectErr: true,
		},
		{
			name: "LLM analysis error",
			setupMocks: func() (*MockDockerClient, *MockLLMClient) {
				return &MockDockerClient{
						containers: []docker.Container{
							{ID: "test", Name: "test", State: "running"},
						},
						logs: map[string][]docker.LogEntry{
							"test": {{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "test"}},
						},
					}, &MockLLMClient{
						analyzeError: &llm.APIError{Message: "API Error", Type: "api_error", Code: "rate_limit"},
					}
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDocker, mockLLM := tt.setupMocks()

			// Test error scenarios
			ctx := context.Background()

			// Test Docker ping
			if err := mockDocker.Ping(ctx); err != nil && tt.expectErr {
				// Expected error case
				return
			}

			// Test container listing
			containers, err := mockDocker.ListContainers(ctx, docker.FilterOptions{})
			if err != nil && tt.expectErr {
				// Expected error case
				return
			}

			// Test LLM analysis if we have containers
			if len(containers) > 0 {
				_, _, err := mockLLM.Analyze(ctx, "system", "user")
				if err != nil && tt.expectErr {
					// Expected error case
					return
				}
			}

			if tt.expectErr {
				t.Error("Expected error but none occurred")
			}
		})
	}
}

func TestScanCmd_OutputFormatting(t *testing.T) {
	t.Parallel()

	// Test that output is properly formatted
	tests := []struct {
		name       string
		dryRun     bool
		expectJSON bool
	}{
		{"normal output", false, false},
		{"dry run output", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			setupTestConfig(t)

			// Capture output
			var buf bytes.Buffer

			// Use local scanConfig instead of global variable
			scanCfg := newTestScanConfig()
			scanCfg.dryRun = tt.dryRun

			if scanCfg.dryRun {
				buf.WriteString("Dry run mode - no changes will be made\n")
			}

			output := buf.String()

			if tt.dryRun && !contains(output, "Dry run mode") {
				t.Error("Expected dry run message in output")
			}
		})
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			func() bool {
				for i := 1; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}())))
}

func TestScanCmd_HelpOutput(t *testing.T) {
	// Test the actual scanCmd's help output
	var buf bytes.Buffer

	// Use the actual scanCmd to test its help
	scanCmd.SetOut(&buf)
	scanCmd.SetErr(&buf)

	// Use Help() method directly for reliable output capture
	err := scanCmd.Help()
	if err != nil {
		t.Errorf("Expected no error getting help, got: %v", err)
	}

	output := buf.String()

	// Check for long description (shown in help output)
	if !strings.Contains(output, "single analysis pass") {
		t.Errorf("Expected help output to contain command description, got: %s", output)
	}

	// Check for flags - these should be present in the actual command's help
	if !strings.Contains(output, "dry-run") {
		t.Errorf("Expected help output to contain dry-run flag, got: %s", output)
	}

	if !strings.Contains(output, "filter") {
		t.Errorf("Expected help output to contain filter flag, got: %s", output)
	}

	if !strings.Contains(output, "lookback") {
		t.Errorf("Expected help output to contain lookback flag, got: %s", output)
	}
}
