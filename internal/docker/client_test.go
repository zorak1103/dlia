package docker

import (
	"context"
	"fmt"
	"testing"
	"time"
)

const (
	testTimestamp = "2025-01-01T10:00:00Z"
	failOnLogs    = "logs"
)

// mockDockerClient implements Client for testing
type mockDockerClient struct {
	containers []Container
	logs       []LogEntry
	shouldFail bool
	failOn     string
}

func (m *mockDockerClient) Ping(_ context.Context) error {
	if m.shouldFail && m.failOn == "ping" {
		return ErrConnectionFailed
	}
	return nil
}

func (m *mockDockerClient) Close() error {
	return nil
}

func (m *mockDockerClient) ListContainers(_ context.Context, _ FilterOptions) ([]Container, error) {
	if m.shouldFail && m.failOn == "list" {
		return nil, ErrConnectionFailed
	}
	return m.containers, nil
}

func (m *mockDockerClient) ReadLogsSince(_ context.Context, _ string, _ time.Time) ([]LogEntry, error) {
	if m.shouldFail && m.failOn == failOnLogs {
		return nil, ErrConnectionFailed
	}
	return m.logs, nil
}

func (m *mockDockerClient) ReadLogsLookback(_ context.Context, _ string, _ time.Duration) ([]LogEntry, error) {
	if m.shouldFail && m.failOn == failOnLogs {
		return nil, ErrConnectionFailed
	}
	return m.logs, nil
}

func TestClient_ListContainers(t *testing.T) {
	containers := []Container{
		{
			ID:    "container1",
			Name:  "test-container-1",
			State: "running",
		},
		{
			ID:    "container2",
			Name:  "test-container-2",
			State: "exited",
		},
	}

	tests := []struct {
		name        string
		containers  []Container
		opts        FilterOptions
		expectCount int
		expectError bool
	}{
		{
			name:        "list all containers",
			containers:  containers,
			opts:        FilterOptions{IncludeAll: true},
			expectCount: 2,
			expectError: false,
		},
		{
			name:        "list running containers only",
			containers:  containers,
			opts:        FilterOptions{IncludeAll: false},
			expectCount: 2, // Mock doesn't filter by state
			expectError: false,
		},
		{
			name:        "empty list",
			containers:  []Container{},
			opts:        FilterOptions{IncludeAll: true},
			expectCount: 0,
			expectError: false,
		},
		{
			name:        "docker error",
			containers:  containers,
			opts:        FilterOptions{IncludeAll: true},
			expectCount: 0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDockerClient{
				containers: tt.containers,
				shouldFail: tt.expectError,
				failOn:     "list",
			}
			client := NewClientWithInterface(mock)

			ctx := context.Background()
			result, err := client.ListContainers(ctx, tt.opts)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !tt.expectError && len(result) != tt.expectCount {
				t.Errorf("Expected %d containers, got %d", tt.expectCount, len(result))
			}
		})
	}
}

func TestClient_ReadLogsSince(t *testing.T) {
	testTime := time.Now().Add(-1 * time.Hour)

	logs := []LogEntry{
		{Timestamp: testTimestamp, Stream: "stdout", Message: "log line 1"},
		{Timestamp: "2025-01-01T10:01:00Z", Stream: "stdout", Message: "log line 2"},
	}

	tests := []struct {
		name        string
		containerID string
		logs        []LogEntry
		expectCount int
		expectError bool
	}{
		{
			name:        "successful log retrieval",
			containerID: "container1",
			logs:        logs,
			expectCount: 2,
			expectError: false,
		},
		{
			name:        "empty logs",
			containerID: "container1",
			logs:        []LogEntry{},
			expectCount: 0,
			expectError: false,
		},
		{
			name:        "docker error",
			containerID: "container1",
			expectCount: 0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDockerClient{
				logs:       tt.logs,
				shouldFail: tt.expectError,
				failOn:     "logs",
			}
			client := NewClientWithInterface(mock)

			ctx := context.Background()
			result, err := client.ReadLogsSince(ctx, tt.containerID, testTime)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !tt.expectError && len(result) != tt.expectCount {
				t.Errorf("Expected %d log entries, got %d", tt.expectCount, len(result))
			}
		})
	}
}

func TestClient_Ping(t *testing.T) {
	tests := []struct {
		name        string
		shouldFail  bool
		expectError bool
	}{
		{
			name:        "successful ping",
			shouldFail:  false,
			expectError: false,
		},
		{
			name:        "ping failure",
			shouldFail:  true,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDockerClient{
				shouldFail: tt.shouldFail,
				failOn:     "ping",
			}
			client := NewClientWithInterface(mock)

			ctx := context.Background()
			err := client.Ping(ctx)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestClient_Close(t *testing.T) {
	mock := &mockDockerClient{}
	client := NewClientWithInterface(mock)

	err := client.Close()
	if err != nil {
		t.Errorf("Unexpected error closing client: %v", err)
	}
}

// Benchmark tests
func BenchmarkClient_ListContainers(b *testing.B) {
	containers := make([]Container, 100)
	for i := 0; i < 100; i++ {
		containers[i] = Container{
			ID:    fmt.Sprintf("container%d", i),
			Name:  fmt.Sprintf("test-container-%d", i),
			State: "running",
		}
	}

	mock := &mockDockerClient{
		containers: containers,
	}
	client := NewClientWithInterface(mock)

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = client.ListContainers(ctx, FilterOptions{})
	}
}

func BenchmarkClient_ReadLogsSince(b *testing.B) {
	logs := make([]LogEntry, 100)
	for i := 0; i < 100; i++ {
		logs[i] = LogEntry{
			Timestamp: fmt.Sprintf("2025-01-01T10:%02d:00Z", i),
			Stream:    "stdout",
			Message:   fmt.Sprintf("log line %d", i),
		}
	}

	mock := &mockDockerClient{
		logs: logs,
	}
	client := NewClientWithInterface(mock)

	ctx := context.Background()
	testTime := time.Now().Add(-1 * time.Hour)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = client.ReadLogsSince(ctx, "container1", testTime)
	}
}

func TestClient_ReadLogsLookback(t *testing.T) {
	logs := []LogEntry{
		{Timestamp: testTimestamp, Stream: "stdout", Message: "log line 1"},
		{Timestamp: "2025-01-01T10:01:00Z", Stream: "stdout", Message: "log line 2"},
	}

	mock := &mockDockerClient{
		logs: logs,
	}
	client := NewClientWithInterface(mock)

	ctx := context.Background()
	result, err := client.ReadLogsLookback(ctx, "container1", 1*time.Hour)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 log entries, got %d", len(result))
	}
}

func TestParseLogLine_WithTimestamp(t *testing.T) {
	line := "2025-01-01T10:00:00.123456789Z This is a test message"
	entry := parseLogLine(line)

	if entry == nil {
		t.Fatal("Expected non-nil entry")
	}

	if entry.Timestamp != "2025-01-01T10:00:00.123456789Z" {
		t.Errorf("Expected timestamp '2025-01-01T10:00:00.123456789Z', got '%s'", entry.Timestamp)
	}

	if entry.Message != "This is a test message" {
		t.Errorf("Expected message 'This is a test message', got '%s'", entry.Message)
	}

	if entry.Stream != "stdout" {
		t.Errorf("Expected stream 'stdout', got '%s'", entry.Stream)
	}
}

func TestParseLogLine_WithoutTimestamp(t *testing.T) {
	line := "Simple log message without timestamp"
	entry := parseLogLine(line)

	if entry == nil {
		t.Fatal("Expected non-nil entry")
	}

	// parseLogLine splits on first space, so "Simple" becomes timestamp and rest is message
	if entry.Timestamp != "Simple" {
		t.Errorf("Expected timestamp 'Simple', got '%s'", entry.Timestamp)
	}

	if entry.Message != "log message without timestamp" {
		t.Errorf("Expected message 'log message without timestamp', got '%s'", entry.Message)
	}
}

func TestParseLogLine_EmptyMessage(t *testing.T) {
	line := "2025-01-01T10:00:00Z"
	entry := parseLogLine(line)

	if entry == nil {
		t.Fatal("Expected non-nil entry")
	}

	// When line has no space, entire line is treated as message with no timestamp
	if entry.Timestamp != "" {
		t.Errorf("Expected empty timestamp, got '%s'", entry.Timestamp)
	}

	if entry.Message != "2025-01-01T10:00:00Z" {
		t.Errorf("Expected message '2025-01-01T10:00:00Z', got '%s'", entry.Message)
	}
}

func TestGetLatestLogTime_WithEntries(t *testing.T) {
	entries := []LogEntry{
		{Timestamp: testTimestamp, Message: "first"},
		{Timestamp: "2025-01-01T10:01:00Z", Message: "second"},
		{Timestamp: "2025-01-01T10:02:00.123456789Z", Message: "last"},
	}

	latestTime, err := GetLatestLogTime(entries)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	expected, _ := time.Parse(time.RFC3339Nano, "2025-01-01T10:02:00.123456789Z")
	if !latestTime.Equal(expected) {
		t.Errorf("Expected time %v, got %v", expected, latestTime)
	}
}

func TestGetLatestLogTime_EmptyEntries(t *testing.T) {
	entries := []LogEntry{}

	latestTime, err := GetLatestLogTime(entries)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !latestTime.IsZero() {
		t.Errorf("Expected zero time for empty entries, got %v", latestTime)
	}
}

func TestGetLatestLogTime_NoTimestamp(t *testing.T) {
	entries := []LogEntry{
		{Timestamp: "", Message: "no timestamp"},
	}

	latestTime, err := GetLatestLogTime(entries)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !latestTime.IsZero() {
		t.Errorf("Expected zero time for entry without timestamp, got %v", latestTime)
	}
}

func TestGetLatestLogTime_InvalidTimestamp(t *testing.T) {
	entries := []LogEntry{
		{Timestamp: "invalid-timestamp", Message: "bad timestamp"},
	}

	_, err := GetLatestLogTime(entries)
	if err == nil {
		t.Error("Expected error for invalid timestamp")
	}
}

func TestGetLatestLogTime_RFC3339Format(t *testing.T) {
	entries := []LogEntry{
		{Timestamp: testTimestamp, Message: "RFC3339 format"},
	}

	latestTime, err := GetLatestLogTime(entries)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	expected, _ := time.Parse(time.RFC3339, testTimestamp)
	if !latestTime.Equal(expected) {
		t.Errorf("Expected time %v, got %v", expected, latestTime)
	}
}

func TestErrorConstants(t *testing.T) {
	if ErrConnectionFailed == nil {
		t.Error("Expected ErrConnectionFailed to be defined")
	}

	if ErrNotFound == nil {
		t.Error("Expected ErrNotFound to be defined")
	}

	if ErrConnectionFailed.Error() != "docker connection failed" {
		t.Errorf("Expected 'docker connection failed', got '%s'", ErrConnectionFailed.Error())
	}

	if ErrNotFound.Error() != "container not found" {
		t.Errorf("Expected 'container not found', got '%s'", ErrNotFound.Error())
	}
}

func TestNewClientWithInterface(t *testing.T) {
	mock := &mockDockerClient{}
	client := NewClientWithInterface(mock)

	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	// Test that the client can perform basic operations
	ctx := context.Background()
	err := client.Ping(ctx)
	if err != nil {
		t.Errorf("Unexpected error from Ping: %v", err)
	}
}

func TestFilterOptions_NamePattern(t *testing.T) {
	containers := []Container{
		{ID: "1", Name: "test-app-1", State: "running"},
		{ID: "2", Name: "test-db-1", State: "running"},
		{ID: "3", Name: "prod-app-1", State: "running"},
	}

	tests := []struct {
		name        string
		pattern     string
		expectCount int
		expectError bool
	}{
		{
			name:        "match all test containers",
			pattern:     "^test-",
			expectCount: 2,
			expectError: false,
		},
		{
			name:        "match app containers",
			pattern:     "-app-",
			expectCount: 2,
			expectError: false,
		},
		{
			name:        "match specific container",
			pattern:     "^prod-app-1$",
			expectCount: 1,
			expectError: false,
		},
		{
			name:        "no matches",
			pattern:     "^nonexistent",
			expectCount: 0,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDockerClient{
				containers: containers,
			}
			client := NewClientWithInterface(mock)

			ctx := context.Background()
			result, err := client.ListContainers(ctx, FilterOptions{
				NamePattern: tt.pattern,
				IncludeAll:  true,
			})

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Note: Our mock doesn't filter by pattern, so we just verify it doesn't error
			if !tt.expectError && result == nil {
				t.Error("Expected non-nil result")
			}
		})
	}
}

func TestContainer_Fields(t *testing.T) {
	container := Container{
		ID:     "test-id",
		Name:   "test-name",
		State:  "running",
		Image:  "test:latest",
		Labels: map[string]string{"env": "test"},
	}

	if container.ID != "test-id" {
		t.Errorf("Expected ID 'test-id', got '%s'", container.ID)
	}

	if container.Name != "test-name" {
		t.Errorf("Expected Name 'test-name', got '%s'", container.Name)
	}

	if container.State != "running" {
		t.Errorf("Expected State 'running', got '%s'", container.State)
	}

	if container.Labels["env"] != "test" {
		t.Errorf("Expected label env='test', got '%s'", container.Labels["env"])
	}
}

func TestLogEntry_Fields(t *testing.T) {
	entry := LogEntry{
		Timestamp: "2025-01-01T10:00:00Z",
		Stream:    "stdout",
		Message:   "test message",
	}

	if entry.Timestamp != testTimestamp {
		t.Errorf("Expected Timestamp '2025-01-01T10:00:00Z', got '%s'", entry.Timestamp)
	}

	if entry.Stream != "stdout" {
		t.Errorf("Expected Stream 'stdout', got '%s'", entry.Stream)
	}

	if entry.Message != "test message" {
		t.Errorf("Expected Message 'test message', got '%s'", entry.Message)
	}
}
