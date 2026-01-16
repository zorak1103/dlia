// Package reporting generates reports from analysis results.
package reporting

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zorak1103/dlia/internal/chunking"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/sanitize"
)

func TestGenerateScanReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		containerName string
		analysis      *chunking.AnalyzeResult
		logs          []docker.LogEntry
		wantContains  []string
	}{
		{
			name:          "basic report generation",
			containerName: "test-container",
			analysis: &chunking.AnalyzeResult{
				Analysis:       "Test analysis content",
				TokensUsed:     1000,
				ChunksUsed:     2,
				Deduplicated:   false,
				OriginalCount:  100,
				ProcessedCount: 100,
			},
			logs: []docker.LogEntry{},
			wantContains: []string{
				"# Scan Report: test-container",
				"**Container:** `test-container`",
				"**Log Entries:** 100",
				"**Tokens Used:** 1000",
				"## 🤖 AI Analysis",
				"Test analysis content",
				"## 📊 Statistics",
				"| Original Logs | 100 |",
				"| Processed Logs | 100 |",
				"| Tokens | 1000 |",
				"| Chunks | 2 |",
			},
		},
		{
			name:          "report with deduplication",
			containerName: "dedupe-container",
			analysis: &chunking.AnalyzeResult{
				Analysis:       "Deduplicated analysis",
				TokensUsed:     500,
				ChunksUsed:     1,
				Deduplicated:   true,
				OriginalCount:  200,
				ProcessedCount: 100,
			},
			logs: []docker.LogEntry{},
			wantContains: []string{
				"# Scan Report: dedupe-container",
				"**Log Entries:** 200",
				"| Deduplication | 50.0% |",
				"| Original Logs | 200 |",
				"| Processed Logs | 100 |",
			},
		},
		{
			name:          "report with special characters in container name",
			containerName: "my/special-container",
			analysis: &chunking.AnalyzeResult{
				Analysis:       "Analysis for special container",
				TokensUsed:     250,
				ChunksUsed:     1,
				Deduplicated:   false,
				OriginalCount:  50,
				ProcessedCount: 50,
			},
			logs: []docker.LogEntry{},
			wantContains: []string{
				"# Scan Report: my/special-container",
				"**Container:** `my/special-container`",
			},
		},
		{
			name:          "report with zero logs",
			containerName: "empty-container",
			analysis: &chunking.AnalyzeResult{
				Analysis:       "No logs to analyze",
				TokensUsed:     0,
				ChunksUsed:     0,
				Deduplicated:   false,
				OriginalCount:  0,
				ProcessedCount: 0,
			},
			logs: []docker.LogEntry{},
			wantContains: []string{
				"# Scan Report: empty-container",
				"**Log Entries:** 0",
				"| Original Logs | 0 |",
				"| Processed Logs | 0 |",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := GenerateScanReport(tt.containerName, tt.analysis, tt.logs)

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("GenerateScanReport() missing expected content: %q\nGot:\n%s", want, result)
				}
			}
		})
	}
}

func TestGenerateScanReport_HasDateHeader(t *testing.T) {
	t.Parallel()

	analysis := &chunking.AnalyzeResult{
		Analysis:       "Test",
		OriginalCount:  10,
		ProcessedCount: 10,
	}

	result := GenerateScanReport("test", analysis, nil)

	if !strings.Contains(result, "**Date:**") {
		t.Error("GenerateScanReport() missing date header")
	}
}

func TestSaveReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		containerName string
		content       string
		wantErr       bool
	}{
		{
			name:          "save basic report",
			containerName: "test-container",
			content:       "# Test Report\n\nThis is a test report.",
			wantErr:       false,
		},
		{
			name:          "save report with slash in name",
			containerName: "namespace/container",
			content:       "# Namespaced Report\n\nContent here.",
			wantErr:       false,
		},
		{
			name:          "save empty report",
			containerName: "empty-report",
			content:       "",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a temporary directory for reports
			tmpDir := t.TempDir()

			cfg := &config.Config{
				Output: config.OutputConfig{
					ReportsDir: tmpDir,
				},
			}

			filePath, err := SaveReport(tt.containerName, tt.content, cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("SaveReport() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Verify file exists
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				t.Errorf("SaveReport() file not created at %s", filePath)
				return
			}

			// Verify file content
			content, err := os.ReadFile(filePath) //nolint:gosec // Test code reading file created by the test
			if err != nil {
				t.Errorf("SaveReport() failed to read created file: %v", err)
				return
			}

			if string(content) != tt.content {
				t.Errorf("SaveReport() content mismatch\nGot: %s\nWant: %s", string(content), tt.content)
			}

			// Verify file path structure
			expectedDir := filepath.Join(tmpDir, sanitize.Name(tt.containerName))
			if !strings.HasPrefix(filePath, expectedDir) {
				t.Errorf("SaveReport() unexpected directory structure\nGot: %s\nExpected prefix: %s", filePath, expectedDir)
			}

			// Verify filename format (YYYY-MM-DD_HH-MM-SS.md)
			filename := filepath.Base(filePath)
			if !strings.HasSuffix(filename, ".md") {
				t.Errorf("SaveReport() filename should end with .md, got: %s", filename)
			}
		})
	}
}

func TestSaveReport_DirectoryCreation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "nested", "reports")

	cfg := &config.Config{
		Output: config.OutputConfig{
			ReportsDir: nestedDir,
		},
	}

	filePath, err := SaveReport("test-container", "test content", cfg)
	if err != nil {
		t.Errorf("SaveReport() failed to create nested directories: %v", err)
		return
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("SaveReport() file not created in nested directory: %s", filePath)
	}
}

func TestSaveReport_FilePermissions(t *testing.T) {
	t.Parallel()

	// Skip on Windows as it doesn't support Unix-style file permissions
	if os.PathSeparator == '\\' {
		t.Skip("Skipping file permissions test on Windows")
	}

	tmpDir := t.TempDir()

	cfg := &config.Config{
		Output: config.OutputConfig{
			ReportsDir: tmpDir,
		},
	}

	filePath, err := SaveReport("test-container", "test content", cfg)
	if err != nil {
		t.Fatalf("SaveReport() failed: %v", err)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	// Check file is not world-readable (0600 permission)
	mode := info.Mode().Perm()
	if mode&0077 != 0 {
		t.Errorf("SaveReport() file has insecure permissions: %o, expected 0600", mode)
	}
}

func TestCalculateSavings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		original  int
		processed int
		want      float64
	}{
		{
			name:      "50% savings",
			original:  100,
			processed: 50,
			want:      50.0,
		},
		{
			name:      "no savings",
			original:  100,
			processed: 100,
			want:      0.0,
		},
		{
			name:      "100% savings",
			original:  100,
			processed: 0,
			want:      100.0,
		},
		{
			name:      "zero original",
			original:  0,
			processed: 0,
			want:      0.0,
		},
		{
			name:      "25% savings",
			original:  200,
			processed: 150,
			want:      25.0,
		},
		{
			name:      "small savings",
			original:  1000,
			processed: 999,
			want:      0.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := calculateSavings(tt.original, tt.processed)
			if got != tt.want {
				t.Errorf("calculateSavings(%d, %d) = %v, want %v", tt.original, tt.processed, got, tt.want)
			}
		})
	}
}

func TestSanitizeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple name",
			input: "container",
			want:  "container",
		},
		{
			name:  "name with slash",
			input: "namespace/container",
			want:  "namespace_container",
		},
		{
			name:  "multiple slashes",
			input: "a/b/c/d",
			want:  "a_b_c_d",
		},
		{
			name:  "no slashes",
			input: "simple-container-name",
			want:  "simple-container-name",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only slashes",
			input: "///",
			want:  "___",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := sanitize.Name(tt.input)
			if got != tt.want {
				t.Errorf("sanitize.Name(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerateScanReport_DeduplicationPercentage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		original       int
		processed      int
		wantPercentage string
	}{
		{
			name:           "high deduplication",
			original:       1000,
			processed:      100,
			wantPercentage: "90.0%",
		},
		{
			name:           "low deduplication",
			original:       100,
			processed:      95,
			wantPercentage: "5.0%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analysis := &chunking.AnalyzeResult{
				Analysis:       "Test",
				Deduplicated:   true,
				OriginalCount:  tt.original,
				ProcessedCount: tt.processed,
			}

			result := GenerateScanReport("test", analysis, nil)

			if !strings.Contains(result, tt.wantPercentage) {
				t.Errorf("GenerateScanReport() deduplication percentage not found\nWant: %s\nGot:\n%s", tt.wantPercentage, result)
			}
		})
	}
}

func TestGenerateScanReport_NoDeduplicationRow(t *testing.T) {
	t.Parallel()

	analysis := &chunking.AnalyzeResult{
		Analysis:       "Test",
		Deduplicated:   false,
		OriginalCount:  100,
		ProcessedCount: 100,
	}

	result := GenerateScanReport("test", analysis, nil)

	if strings.Contains(result, "| Deduplication |") {
		t.Error("GenerateScanReport() should not include deduplication row when Deduplicated is false")
	}
}
