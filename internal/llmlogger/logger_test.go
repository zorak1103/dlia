package llmlogger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewLogger(t *testing.T) {
	t.Run("creates logger with correct fields", func(t *testing.T) {
		logger := NewLogger("/tmp/logs", true)
		if logger.baseDir != "/tmp/logs" {
			t.Errorf("expected baseDir '/tmp/logs', got '%s'", logger.baseDir)
		}
		if !logger.enabled {
			t.Error("expected enabled to be true")
		}
	})

	t.Run("creates disabled logger", func(t *testing.T) {
		logger := NewLogger("/tmp/logs", false)
		if logger.enabled {
			t.Error("expected enabled to be false")
		}
	})
}

func TestIsEnabled(t *testing.T) {
	t.Run("returns true when enabled", func(t *testing.T) {
		logger := NewLogger("/tmp/logs", true)
		if !logger.IsEnabled() {
			t.Error("expected IsEnabled to return true")
		}
	})

	t.Run("returns false when disabled", func(t *testing.T) {
		logger := NewLogger("/tmp/logs", false)
		if logger.IsEnabled() {
			t.Error("expected IsEnabled to return false")
		}
	})

	t.Run("returns false for nil logger", func(t *testing.T) {
		var logger *Logger
		if logger.IsEnabled() {
			t.Error("expected IsEnabled to return false for nil logger")
		}
	})
}

func TestLogInteraction(t *testing.T) {
	t.Run("disabled logger returns nil without creating files", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := NewLogger(tmpDir, false)

		err := logger.LogInteraction("test-container", "input", map[string]string{"key": "value"}, map[string]string{"result": "ok"})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}

		// Verify no files were created
		entries, _ := os.ReadDir(tmpDir)
		if len(entries) != 0 {
			t.Errorf("expected no files created, found %d entries", len(entries))
		}
	})

	t.Run("nil logger returns nil without panic", func(t *testing.T) {
		var logger *Logger
		err := logger.LogInteraction("test-container", "input", nil, nil)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("enabled logger creates file with correct content", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := NewLogger(tmpDir, true)

		request := map[string]interface{}{
			"model":    "gpt-4",
			"messages": []string{"Hello"},
		}
		response := map[string]interface{}{
			"id":      "123",
			"choices": []string{"Hi there"},
		}

		err := logger.LogInteraction("my-container", "original log content", request, response)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify container directory was created
		containerDir := filepath.Join(tmpDir, "my-container")
		info, err := os.Stat(containerDir)
		if err != nil {
			t.Fatalf("container directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Error("expected container directory to be a directory")
		}

		// Verify file was created
		entries, err := os.ReadDir(containerDir)
		if err != nil {
			t.Fatalf("failed to read container directory: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 file, found %d", len(entries))
		}

		// Verify file content
		content, err := os.ReadFile(filepath.Join(containerDir, entries[0].Name())) //nolint:gosec // Test code reading known test files
		if err != nil {
			t.Fatalf("failed to read log file: %v", err)
		}

		contentStr := string(content)
		if !strings.Contains(contentStr, "# LLM Interaction Log") {
			t.Error("missing header in log file")
		}
		if !strings.Contains(contentStr, "**Container**: my-container") {
			t.Error("missing container name in log file")
		}
		if !strings.Contains(contentStr, "## Original Input") {
			t.Error("missing original input section")
		}
		if !strings.Contains(contentStr, "original log content") {
			t.Error("missing original input content")
		}
		if !strings.Contains(contentStr, "## Request Sent to LLM") {
			t.Error("missing request section")
		}
		if !strings.Contains(contentStr, `"model": "gpt-4"`) {
			t.Error("missing request content")
		}
		if !strings.Contains(contentStr, "## LLM Response") {
			t.Error("missing response section")
		}
		if !strings.Contains(contentStr, `"id": "123"`) {
			t.Error("missing response content")
		}
	})

	t.Run("auto-creates nested directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		nestedDir := filepath.Join(tmpDir, "nested", "path", "logs")
		logger := NewLogger(nestedDir, true)

		err := logger.LogInteraction("container", "input", nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		containerDir := filepath.Join(nestedDir, "container")
		if _, err := os.Stat(containerDir); os.IsNotExist(err) {
			t.Error("nested directories were not created")
		}
	})
}

func TestLogInteraction_SanitizesContainerDirName(t *testing.T) {
	t.Run("flattens slashes in container name to a single directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := NewLogger(tmpDir, true)

		if err := logger.LogInteraction("my/container", "input", nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, err := os.Stat(filepath.Join(tmpDir, "my_container")); err != nil {
			t.Errorf("expected sanitized container directory 'my_container': %v", err)
		}
	})

	t.Run("defuses Windows-reserved container names", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := NewLogger(tmpDir, true)

		if err := logger.LogInteraction("con", "input", nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, err := os.Stat(filepath.Join(tmpDir, "CON_")); err != nil {
			t.Errorf("expected defused container directory 'CON_': %v", err)
		}
	})
}

func TestFormatMarkdown(t *testing.T) {
	t.Run("formats markdown correctly", func(t *testing.T) {
		// Use a fixed time for testing
		testTime := mustParseTime("2024-01-15T10:30:00Z")

		content := formatMarkdown(
			"test-container",
			testTime,
			"sample log content",
			[]byte(`{"key": "value"}`),
			[]byte(`{"result": "success"}`),
		)

		expectedParts := []string{
			"# LLM Interaction Log",
			"**Container**: test-container",
			"**Timestamp**: 2024-01-15T10:30:00Z",
			"## Original Input",
			"sample log content",
			"## Request Sent to LLM",
			`{"key": "value"}`,
			"## LLM Response",
			`{"result": "success"}`,
		}

		for _, part := range expectedParts {
			if !strings.Contains(content, part) {
				t.Errorf("missing expected content: %q", part)
			}
		}
	})
}

func mustParseTime(s string) (t time.Time) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
