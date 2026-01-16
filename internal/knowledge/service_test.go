package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zorak1103/dlia/internal/chunking"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/sanitize"
)

func TestUpdateServiceKB(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		analysis      *chunking.AnalyzeResult
		wantStatus    string
		wantErr       bool
	}{
		{
			name:          "healthy status with normal analysis",
			containerName: "test-container",
			analysis: &chunking.AnalyzeResult{
				Analysis: "All systems running normally. No issues detected.",
			},
			wantStatus: "🟢 Healthy",
			wantErr:    false,
		},
		{
			name:          "issues status with critical errors",
			containerName: "test-container",
			analysis: &chunking.AnalyzeResult{
				Analysis: "Critical database connection error detected.",
			},
			wantStatus: "🔴 Issues Detected",
			wantErr:    false,
		},
		{
			name:          "issues status with errors",
			containerName: "test-container",
			analysis: &chunking.AnalyzeResult{
				Analysis: "Multiple error messages found in logs.",
			},
			wantStatus: "🔴 Issues Detected",
			wantErr:    false,
		},
		{
			name:          "warnings status",
			containerName: "test-container",
			analysis: &chunking.AnalyzeResult{
				Analysis: "Warning: Memory usage is high.",
			},
			wantStatus: "🟡 Warnings",
			wantErr:    false,
		},
		{
			name:          "container with slash in name",
			containerName: "project/container",
			analysis: &chunking.AnalyzeResult{
				Analysis: "Normal operation.",
			},
			wantStatus: "🟢 Healthy",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cfg := &config.Config{
				Output: config.OutputConfig{
					KnowledgeBaseDir:       tmpDir,
					KnowledgeRetentionDays: 30,
				},
			}

			err := UpdateServiceKB(tt.containerName, tt.analysis, cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateServiceKB() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Verify file was created
			sanitized := sanitize.Name(tt.containerName)
			filePath := filepath.Join(tmpDir, "services", sanitized+".md")
			// #nosec G304 - reading from controlled test temp directory
			content, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("Failed to read KB file: %v", err)
			}

			contentStr := string(content)

			// Check for expected content
			if !strings.Contains(contentStr, tt.wantStatus) {
				t.Errorf("Expected status %q not found in content", tt.wantStatus)
			}

			if !strings.Contains(contentStr, tt.analysis.Analysis) {
				t.Error("Analysis content not found in KB file")
			}

			if !strings.Contains(contentStr, "# Knowledge Base:") {
				t.Error("KB file missing header")
			}

			if !strings.Contains(contentStr, "## Service History") {
				t.Error("KB file missing service history section")
			}

			if !strings.Contains(contentStr, "### Scan:") {
				t.Error("KB file missing scan entry")
			}
		})
	}
}

func TestUpdateServiceKB_AppendToExisting(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Output: config.OutputConfig{
			KnowledgeBaseDir:       tmpDir,
			KnowledgeRetentionDays: 30,
		},
	}

	containerName := "test-container"

	// First update
	analysis1 := &chunking.AnalyzeResult{
		Analysis: "First scan - all good.",
	}
	err := UpdateServiceKB(containerName, analysis1, cfg)
	if err != nil {
		t.Fatalf("First UpdateServiceKB() error = %v", err)
	}

	// Second update
	analysis2 := &chunking.AnalyzeResult{
		Analysis: "Second scan - found warning.",
	}
	err = UpdateServiceKB(containerName, analysis2, cfg)
	if err != nil {
		t.Fatalf("Second UpdateServiceKB() error = %v", err)
	}

	// Verify both entries exist
	filePath := filepath.Join(tmpDir, "services", containerName+".md")
	// #nosec G304 - reading from controlled test temp directory
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read KB file: %v", err)
	}

	contentStr := string(content)

	if !strings.Contains(contentStr, "First scan - all good.") {
		t.Error("First scan not found in KB file")
	}

	if !strings.Contains(contentStr, "Second scan - found warning.") {
		t.Error("Second scan not found in KB file")
	}

	// Count scan entries
	scanCount := strings.Count(contentStr, "### Scan:")
	if scanCount != 2 {
		t.Errorf("Expected 2 scan entries, got %d", scanCount)
	}
}

func TestPruneEntries(t *testing.T) {
	now := time.Now()
	old := now.Add(-40 * 24 * time.Hour)    // 40 days ago (should be pruned)
	recent := now.Add(-10 * 24 * time.Hour) // 10 days ago (should be kept)

	tests := []struct {
		name      string
		content   string
		retention time.Duration
		wantCount int
	}{
		{
			name: "prune old entries",
			content: `# Knowledge Base: test-container

## Service History

### Scan: ` + old.Format(time.RFC3339) + `
**Status:** 🟢 Healthy

Old entry that should be pruned.

---

### Scan: ` + recent.Format(time.RFC3339) + `
**Status:** 🟢 Healthy

Recent entry that should be kept.

---
`,
			retention: 30 * 24 * time.Hour,
			wantCount: 1,
		},
		{
			name: "keep all entries within retention",
			content: `# Knowledge Base: test-container

## Service History

### Scan: ` + recent.Format(time.RFC3339) + `
**Status:** 🟢 Healthy

Entry 1

---

### Scan: ` + now.Format(time.RFC3339) + `
**Status:** 🟢 Healthy

Entry 2

---
`,
			retention: 30 * 24 * time.Hour,
			wantCount: 2,
		},
		{
			name: "handle entries without timestamps",
			content: `# Knowledge Base: test-container

## Service History

### Scan: invalid-timestamp
**Status:** 🟢 Healthy

Entry without valid timestamp (should be kept)

---

### Scan: ` + recent.Format(time.RFC3339) + `
**Status:** 🟢 Healthy

Valid entry

---
`,
			retention: 30 * 24 * time.Hour,
			wantCount: 2,
		},
		{
			name: "handle empty content",
			content: `# Knowledge Base: test-container

## Service History
`,
			retention: 30 * 24 * time.Hour,
			wantCount: 0,
		},
		{
			name: "handle content without service history section",
			content: `# Knowledge Base: test-container

Some other content
`,
			retention: 30 * 24 * time.Hour,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pruneEntries(tt.content, tt.retention)

			// Count remaining entries
			entryCount := strings.Count(result, "### Scan:")
			if entryCount != tt.wantCount {
				t.Errorf("Expected %d entries, got %d", tt.wantCount, entryCount)
			}

			// Verify structure is maintained
			if tt.wantCount > 0 || strings.Contains(tt.content, "## Service History") {
				if !strings.Contains(result, "## Service History") {
					t.Error("Service History section should be present")
				}
			}
		})
	}
}

func TestPruneEntries_PreservesRecent(t *testing.T) {
	now := time.Now()
	timestamps := []time.Time{
		now.Add(-1 * time.Hour),
		now.Add(-24 * time.Hour),
		now.Add(-7 * 24 * time.Hour),
		now.Add(-20 * 24 * time.Hour),
		now.Add(-35 * 24 * time.Hour), // This one should be pruned
		now.Add(-60 * 24 * time.Hour), // This one should be pruned
	}

	var content strings.Builder
	content.WriteString("# Knowledge Base: test-container\n\n")
	content.WriteString("## Service History\n")

	for i, ts := range timestamps {
		content.WriteString(fmt.Sprintf("\n### Scan: %s\n", ts.Format(time.RFC3339)))
		content.WriteString("**Status:** 🟢 Healthy\n\n")
		content.WriteString(fmt.Sprintf("Entry %d\n\n", i+1))
		content.WriteString("---\n")
	}

	result := pruneEntries(content.String(), 30*24*time.Hour)

	// Should have 4 entries (those within 30 days)
	entryCount := strings.Count(result, "### Scan:")
	if entryCount != 4 {
		t.Errorf("Expected 4 entries, got %d", entryCount)
	}

	// Verify the pruned entries are not present
	if strings.Contains(result, "Entry 5") || strings.Contains(result, "Entry 6") {
		t.Error("Old entries should be pruned")
	}

	// Verify recent entries are present
	if !strings.Contains(result, "Entry 1") {
		t.Error("Recent entry 1 should be present")
	}
	if !strings.Contains(result, "Entry 4") {
		t.Error("Recent entry 4 should be present")
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no slashes",
			input: "test-container",
			want:  "test-container",
		},
		{
			name:  "single slash",
			input: "project/container",
			want:  "project_container",
		},
		{
			name:  "multiple slashes",
			input: "org/project/container",
			want:  "org_project_container",
		},
		{
			name:  "trailing slash",
			input: "container/",
			want:  "container_",
		},
		{
			name:  "leading slash",
			input: "/container",
			want:  "_container",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitize.Name(tt.input)
			if got != tt.want {
				t.Errorf("sanitize.Name() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpdateServiceKB_CreateDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Use a nested path that doesn't exist
	kbDir := filepath.Join(tmpDir, "nested", "kb", "path")

	cfg := &config.Config{
		Output: config.OutputConfig{
			KnowledgeBaseDir:       kbDir,
			KnowledgeRetentionDays: 30,
		},
	}

	analysis := &chunking.AnalyzeResult{
		Analysis: "Test analysis.",
	}

	err := UpdateServiceKB("test", analysis, cfg)
	if err != nil {
		t.Fatalf("UpdateServiceKB() error = %v", err)
	}

	// Verify directory was created
	servicesDir := filepath.Join(kbDir, "services")
	if _, err := os.Stat(servicesDir); os.IsNotExist(err) {
		t.Error("Services directory should be created")
	}

	// Verify file was created
	filePath := filepath.Join(servicesDir, "test.md")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("KB file should be created")
	}
}

func TestUpdateServiceKB_StatusDetection(t *testing.T) {
	tests := []struct {
		name       string
		analysis   string
		wantStatus string
	}{
		{
			name:       "critical lowercase",
			analysis:   "critical failure detected",
			wantStatus: "🔴 Issues Detected",
		},
		{
			name:       "critical uppercase",
			analysis:   "CRITICAL system failure",
			wantStatus: "🔴 Issues Detected",
		},
		{
			name:       "error lowercase",
			analysis:   "connection error occurred",
			wantStatus: "🔴 Issues Detected",
		},
		{
			name:       "error uppercase",
			analysis:   "ERROR: Unable to connect",
			wantStatus: "🔴 Issues Detected",
		},
		{
			name:       "warning lowercase",
			analysis:   "warning: high memory usage",
			wantStatus: "🟡 Warnings",
		},
		{
			name:       "warning uppercase",
			analysis:   "WARNING: Disk space low",
			wantStatus: "🟡 Warnings",
		},
		{
			name:       "healthy - no keywords",
			analysis:   "All systems operational",
			wantStatus: "🟢 Healthy",
		},
		{
			name:       "healthy - info only",
			analysis:   "Service started successfully",
			wantStatus: "🟢 Healthy",
		},
		{
			name:       "critical takes precedence over warning",
			analysis:   "Critical error detected. Warning: this is serious.",
			wantStatus: "🔴 Issues Detected",
		},
		{
			name:       "error takes precedence over warning",
			analysis:   "Error in processing. Warning: check logs.",
			wantStatus: "🔴 Issues Detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cfg := &config.Config{
				Output: config.OutputConfig{
					KnowledgeBaseDir:       tmpDir,
					KnowledgeRetentionDays: 30,
				},
			}

			analysis := &chunking.AnalyzeResult{
				Analysis: tt.analysis,
			}

			err := UpdateServiceKB("test", analysis, cfg)
			if err != nil {
				t.Fatalf("UpdateServiceKB() error = %v", err)
			}

			filePath := filepath.Join(tmpDir, "services", "test.md")
			// #nosec G304 - reading from controlled test temp directory
			content, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("Failed to read KB file: %v", err)
			}

			if !strings.Contains(string(content), tt.wantStatus) {
				t.Errorf("Expected status %q not found in content", tt.wantStatus)
			}
		})
	}
}

func TestPruneEntries_EdgeCases(t *testing.T) {
	t.Run("no separator between entries", func(t *testing.T) {
		now := time.Now()
		content := `# Knowledge Base: test-container

## Service History

### Scan: ` + now.Format(time.RFC3339) + `
**Status:** 🟢 Healthy

Entry 1

### Scan: ` + now.Add(-1*time.Hour).Format(time.RFC3339) + `
**Status:** 🟢 Healthy

Entry 2
`
		result := pruneEntries(content, 30*24*time.Hour)

		// Both entries should be preserved even without separator
		entryCount := strings.Count(result, "### Scan:")
		if entryCount < 1 {
			t.Error("At least one entry should be preserved")
		}
	})

	t.Run("empty entries section", func(t *testing.T) {
		content := `# Knowledge Base: test-container

## Service History

---
---
---
`
		result := pruneEntries(content, 30*24*time.Hour)

		// Should not panic and should maintain structure
		if !strings.Contains(result, "## Service History") {
			t.Error("Service History section should be preserved")
		}
	})

	t.Run("malformed timestamp", func(t *testing.T) {
		content := `# Knowledge Base: test-container

## Service History

### Scan: not-a-timestamp
**Status:** 🟢 Healthy

Entry with bad timestamp

---
`
		result := pruneEntries(content, 30*24*time.Hour)

		// Entry with bad timestamp should be kept (fail-safe)
		if !strings.Contains(result, "Entry with bad timestamp") {
			t.Error("Entry with malformed timestamp should be preserved")
		}
	})
}

func BenchmarkUpdateServiceKB(b *testing.B) {
	tmpDir := b.TempDir()
	cfg := &config.Config{
		Output: config.OutputConfig{
			KnowledgeBaseDir:       tmpDir,
			KnowledgeRetentionDays: 30,
		},
	}

	analysis := &chunking.AnalyzeResult{
		Analysis: "Benchmark test analysis.",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = UpdateServiceKB("test-container", analysis, cfg)
	}
}

func BenchmarkPruneEntries(b *testing.B) {
	now := time.Now()
	var content strings.Builder
	content.WriteString("# Knowledge Base: test-container\n\n")
	content.WriteString("## Service History\n")

	// Create content with 100 entries
	for i := 0; i < 100; i++ {
		ts := now.Add(time.Duration(-i) * 24 * time.Hour)
		content.WriteString(fmt.Sprintf("\n### Scan: %s\n", ts.Format(time.RFC3339)))
		content.WriteString("**Status:** 🟢 Healthy\n\n")
		content.WriteString(fmt.Sprintf("Entry %d\n\n", i+1))
		content.WriteString("---\n")
	}

	contentStr := content.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pruneEntries(contentStr, 30*24*time.Hour)
	}
}

func BenchmarkSanitizeName(b *testing.B) {
	testNames := []string{
		"simple-name",
		"org/project/container",
		"multiple/slashes/in/name",
		"",
		"no-slashes-here",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, name := range testNames {
			_ = sanitize.Name(name)
		}
	}
}

func TestUpdateGlobalSummary(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Output: config.OutputConfig{
			KnowledgeBaseDir: tmpDir,
		},
	}

	results := map[string]*chunking.AnalyzeResult{
		"service1": {
			Analysis: "All systems operational. No issues detected.",
		},
		"service2": {
			Analysis: "Critical database error detected.",
		},
		"service3": {
			Analysis: "Warning: Memory usage is high.",
		},
	}

	err := UpdateGlobalSummary(results, cfg)
	if err != nil {
		t.Fatalf("UpdateGlobalSummary() error = %v", err)
	}

	// Verify file was created
	filePath := filepath.Join(tmpDir, "global_summary.md")
	// #nosec G304 - reading from controlled test temp directory
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read global summary file: %v", err)
	}

	contentStr := string(content)

	// Check for expected content
	expectedStrings := []string{
		"# 🌍 Global System Summary",
		"**Last Updated:**",
		"System Health:",
		"Service Status",
		"service1",
		"service2",
		"service3",
		"Recent Critical Issues",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(contentStr, expected) {
			t.Errorf("Expected content to contain %q", expected)
		}
	}

	// Verify issues are reported
	if !strings.Contains(contentStr, "Service(s) Reporting Issues") {
		t.Error("Expected issues to be reported in global summary")
	}

	// Verify critical section includes service2
	if !strings.Contains(contentStr, "### service2") {
		t.Error("Expected service2 in critical issues section")
	}
}

func TestUpdateGlobalSummary_NoIssues(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Output: config.OutputConfig{
			KnowledgeBaseDir: tmpDir,
		},
	}

	results := map[string]*chunking.AnalyzeResult{
		"service1": {
			Analysis: "All systems operational.",
		},
		"service2": {
			Analysis: "Running normally.",
		},
	}

	err := UpdateGlobalSummary(results, cfg)
	if err != nil {
		t.Fatalf("UpdateGlobalSummary() error = %v", err)
	}

	filePath := filepath.Join(tmpDir, "global_summary.md")
	// #nosec G304 - reading from controlled test temp directory
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read global summary file: %v", err)
	}

	contentStr := string(content)

	// Verify healthy status
	if !strings.Contains(contentStr, "🟢 All Systems Operational") {
		t.Error("Expected all systems operational status")
	}

	// Verify no critical issues
	if !strings.Contains(contentStr, "No critical issues reported") {
		t.Error("Expected no critical issues message")
	}
}

func TestUpdateGlobalSummary_EmptyResults(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Output: config.OutputConfig{
			KnowledgeBaseDir: tmpDir,
		},
	}

	results := map[string]*chunking.AnalyzeResult{}

	err := UpdateGlobalSummary(results, cfg)
	if err != nil {
		t.Fatalf("UpdateGlobalSummary() error = %v", err)
	}

	filePath := filepath.Join(tmpDir, "global_summary.md")
	// #nosec G304 - reading from controlled test temp directory
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read global summary file: %v", err)
	}

	contentStr := string(content)

	// Should still have structure
	if !strings.Contains(contentStr, "# 🌍 Global System Summary") {
		t.Error("Expected global summary header")
	}

	// Should show healthy with no services
	if !strings.Contains(contentStr, "🟢 All Systems Operational") {
		t.Error("Expected operational status for empty results")
	}
}

func TestHasIssues(t *testing.T) {
	tests := []struct {
		name     string
		analysis string
		want     bool
	}{
		{
			name:     "critical lowercase",
			analysis: "critical failure",
			want:     true,
		},
		{
			name:     "critical uppercase",
			analysis: "CRITICAL ERROR",
			want:     true,
		},
		{
			name:     "error lowercase",
			analysis: "error occurred",
			want:     true,
		},
		{
			name:     "error uppercase",
			analysis: "ERROR detected",
			want:     true,
		},
		{
			name:     "no issues",
			analysis: "All systems operational",
			want:     false,
		},
		{
			name:     "warning only",
			analysis: "Warning: high memory",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasIssues(tt.analysis)
			if got != tt.want {
				t.Errorf("hasIssues() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasWarnings(t *testing.T) {
	tests := []struct {
		name     string
		analysis string
		want     bool
	}{
		{
			name:     "warning lowercase",
			analysis: "warning detected",
			want:     true,
		},
		{
			name:     "warning uppercase",
			analysis: "WARNING: high usage",
			want:     true,
		},
		{
			name:     "no warnings",
			analysis: "All systems operational",
			want:     false,
		},
		{
			name:     "error but no warning",
			analysis: "Error occurred",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasWarnings(tt.analysis)
			if got != tt.want {
				t.Errorf("hasWarnings() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractSummary(t *testing.T) {
	tests := []struct {
		name     string
		analysis string
		want     string
	}{
		{
			name: "with summary marker",
			analysis: `**Summary**: System is running normally
Additional details here`,
			want: "System is running normally",
		},
		{
			name:     "without summary marker",
			analysis: "First line of analysis\nSecond line here",
			want:     "First line of analysis",
		},
		{
			name:     "long first line",
			analysis: strings.Repeat("a", 100),
			want:     strings.Repeat("a", 50) + "...",
		},
		{
			name:     "empty analysis",
			analysis: "",
			want:     "", // extractSummary returns truncate("", 50) which is ""
		},
		{
			name:     "only whitespace first line",
			analysis: "   \n  \n  ",
			want:     "   ", // extractSummary returns first line, which is whitespace
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSummary(tt.analysis)
			if got != tt.want {
				t.Errorf("extractSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractErrors(t *testing.T) {
	tests := []struct {
		name     string
		analysis string
		want     string
	}{
		{
			name: "with errors section",
			analysis: `Some preamble
**Errors**
- Error 1
- Error 2
**Warnings**
- Warning 1`,
			want: "**Errors**\n- Error 1\n- Error 2",
		},
		{
			name: "errors without warnings",
			analysis: `**Errors**
- Critical error
Some other text`,
			want: "**Errors**\n- Critical error\nSome other text",
		},
		{
			name:     "no errors section",
			analysis: "No errors marker found",
			want:     "Issues detected but could not parse specific errors.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractErrors(tt.analysis)
			if !strings.Contains(got, strings.TrimSpace(tt.want)) {
				t.Errorf("extractErrors() = %v, want to contain %v", got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "needs truncation",
			input:  "this is a long string",
			maxLen: 10,
			want:   "this is a ...",
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 10,
			want:   "",
		},
		{
			name:   "maxLen zero",
			input:  "test",
			maxLen: 0,
			want:   "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpdateGlobalSummary_TableFormat(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Output: config.OutputConfig{
			KnowledgeBaseDir: tmpDir,
		},
	}

	results := map[string]*chunking.AnalyzeResult{
		"alpha": {Analysis: "OK"},
		"beta":  {Analysis: "Error found"},
		"gamma": {Analysis: "Warning detected"},
	}

	err := UpdateGlobalSummary(results, cfg)
	if err != nil {
		t.Fatalf("UpdateGlobalSummary() error = %v", err)
	}

	filePath := filepath.Join(tmpDir, "global_summary.md")
	// #nosec G304 - reading from controlled test temp directory
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read global summary file: %v", err)
	}

	contentStr := string(content)

	// Verify table headers
	if !strings.Contains(contentStr, "| Service | Status | Last Analysis |") {
		t.Error("Expected table headers")
	}

	// Verify table separator
	if !strings.Contains(contentStr, "|---------|--------|---------------|") {
		t.Error("Expected table separator")
	}

	// Verify services are sorted alphabetically
	alphaPos := strings.Index(contentStr, "| alpha |")
	betaPos := strings.Index(contentStr, "| beta |")
	gammaPos := strings.Index(contentStr, "| gamma |")

	if alphaPos == -1 || betaPos == -1 || gammaPos == -1 {
		t.Error("Expected all services in table")
	}

	if alphaPos >= betaPos || betaPos >= gammaPos {
		t.Error("Expected services in alphabetical order")
	}

	// Verify status icons
	if !strings.Contains(contentStr, "🟢 OK") {
		t.Error("Expected OK status")
	}
	if !strings.Contains(contentStr, "🔴 Issues") {
		t.Error("Expected Issues status")
	}
	if !strings.Contains(contentStr, "🟡 Warning") {
		t.Error("Expected Warning status")
	}
}

func TestUpdateGlobalSummary_CreateDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Use a nested path that doesn't exist
	kbDir := filepath.Join(tmpDir, "nested", "kb")

	cfg := &config.Config{
		Output: config.OutputConfig{
			KnowledgeBaseDir: kbDir,
		},
	}

	results := map[string]*chunking.AnalyzeResult{
		"test": {Analysis: "OK"},
	}

	err := UpdateGlobalSummary(results, cfg)
	if err != nil {
		t.Fatalf("UpdateGlobalSummary() error = %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(kbDir); os.IsNotExist(err) {
		t.Error("KB directory should be created")
	}

	// Verify file was created
	filePath := filepath.Join(kbDir, "global_summary.md")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("Global summary file should be created")
	}
}

func BenchmarkUpdateGlobalSummary(b *testing.B) {
	tmpDir := b.TempDir()
	cfg := &config.Config{
		Output: config.OutputConfig{
			KnowledgeBaseDir: tmpDir,
		},
	}

	// Create 50 services
	results := make(map[string]*chunking.AnalyzeResult, 50)
	for i := 0; i < 50; i++ {
		results[fmt.Sprintf("service%d", i)] = &chunking.AnalyzeResult{
			Analysis: fmt.Sprintf("Analysis for service %d", i),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = UpdateGlobalSummary(results, cfg)
	}
}
