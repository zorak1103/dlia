package cmd

// Tests in this file strengthen assertions for display functions that print
// directly to os.Stdout. They capture stdout via captureStdout and therefore
// must NOT run in parallel (os.Stdout is process-global).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zorak1103/dlia/internal/chunking"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/prompts"
	"github.com/zorak1103/dlia/internal/state"
)

func TestDisplayVerboseHeader_FilterAndLookbackLines(t *testing.T) {
	cfg := &config.Config{
		LLM:    config.LLMConfig{Model: "test-model"},
		Docker: config.DockerConfig{SocketPath: "unix:///var/run/docker.sock"},
		Output: config.OutputConfig{StateFile: filepath.Join("tmp", "state.json")},
	}

	t.Run("prints filter and lookback lines when set", func(t *testing.T) {
		scanCfg := newTestScanConfig()
		scanCfg.verbose = true
		scanCfg.filter = "web.*"

		read := captureStdout(t)
		displayVerboseHeader(cfg, scanCfg, 2*time.Hour)
		out := read()

		assert.Contains(t, out, "Container Filter: web.*")
		assert.Contains(t, out, "Lookback Duration: 2h0m0s")
	})

	t.Run("omits filter and lookback lines when unset", func(t *testing.T) {
		scanCfg := newTestScanConfig()
		scanCfg.verbose = true

		read := captureStdout(t)
		displayVerboseHeader(cfg, scanCfg, 0)
		out := read()

		assert.NotContains(t, out, "Container Filter:")
		assert.NotContains(t, out, "Lookback Duration:")
	})
}

func TestDisplayPromptConfiguration_LoaderStates(t *testing.T) {
	original := getDefaultPromptLoader
	t.Cleanup(func() { getDefaultPromptLoader = original })

	t.Run("nil loader prints header without panic", func(t *testing.T) {
		getDefaultPromptLoader = func() *prompts.PromptLoader { return nil }

		read := captureStdout(t)
		displayPromptConfiguration() // must not panic
		out := read()

		assert.Contains(t, out, "Prompt Configuration:")
		assert.NotContains(t, out, "Using built-in defaults")
		assert.NotContains(t, out, "EXTERNAL")
		assert.NotContains(t, out, "INTERNAL DEFAULT")
	})

	t.Run("empty sources prints built-in defaults note", func(t *testing.T) {
		getDefaultPromptLoader = func() *prompts.PromptLoader {
			return prompts.NewPromptLoader(&config.Config{})
		}

		read := captureStdout(t)
		displayPromptConfiguration()
		out := read()

		assert.Contains(t, out, "Using built-in defaults (will be loaded on first use)")
		assert.NotContains(t, out, "EXTERNAL")
	})

	t.Run("loaded sources are listed instead of the defaults note", func(t *testing.T) {
		loader := prompts.NewPromptLoader(&config.Config{})
		_, err := loader.SystemPrompt("")
		require.NoError(t, err)
		getDefaultPromptLoader = func() *prompts.PromptLoader { return loader }

		read := captureStdout(t)
		displayPromptConfiguration()
		out := read()

		assert.Contains(t, out, "system_prompt: INTERNAL DEFAULT")
		assert.NotContains(t, out, "Using built-in defaults")
	})
}

func TestDisplayNoContainersFound_FilterOutput(t *testing.T) {
	t.Run("with filter shows the filter", func(t *testing.T) {
		scanCfg := newTestScanConfig()
		scanCfg.filter = "nginx.*"

		read := captureStdout(t)
		displayNoContainersFound(scanCfg)
		out := read()

		assert.Contains(t, out, "No containers found")
		assert.Contains(t, out, "(with filter: nginx.*)")
	})

	t.Run("without filter shows no filter note", func(t *testing.T) {
		scanCfg := newTestScanConfig()

		read := captureStdout(t)
		displayNoContainersFound(scanCfg)
		out := read()

		assert.Contains(t, out, "No containers found")
		assert.NotContains(t, out, "with filter")
	})
}

func TestDisplayLogsPreview_OutputBoundaries(t *testing.T) {
	newLog := func(i int) docker.LogEntry {
		return docker.LogEntry{
			Timestamp: fmt.Sprintf("2023-01-01T10:00:%02dZ", i),
			Stream:    "stdout",
			Message:   fmt.Sprintf("log %d", i),
		}
	}

	t.Run("empty logs prints nothing", func(t *testing.T) {
		scanCfg := newTestScanConfig()
		scanCfg.verbose = true

		read := captureStdout(t)
		displayLogsPreview(nil, scanCfg)
		out := read()

		assert.Empty(t, out)
	})

	t.Run("exactly ten logs shows no more-lines line", func(t *testing.T) {
		scanCfg := newTestScanConfig()
		scanCfg.verbose = true

		logs := make([]docker.LogEntry, 10)
		for i := range logs {
			logs[i] = newLog(i)
		}

		read := captureStdout(t)
		displayLogsPreview(logs, scanCfg)
		out := read()

		assert.Contains(t, out, "log 9")
		assert.NotContains(t, out, "more lines")
	})

	t.Run("eleven logs shows one more line", func(t *testing.T) {
		scanCfg := newTestScanConfig()
		scanCfg.verbose = true

		logs := make([]docker.LogEntry, 11)
		for i := range logs {
			logs[i] = newLog(i)
		}

		read := captureStdout(t)
		displayLogsPreview(logs, scanCfg)
		out := read()

		assert.Contains(t, out, "... (1 more lines)")
	})

	t.Run("fifteen logs shows five more lines", func(t *testing.T) {
		scanCfg := newTestScanConfig()
		scanCfg.verbose = true

		logs := make([]docker.LogEntry, 15)
		for i := range logs {
			logs[i] = newLog(i)
		}

		read := captureStdout(t)
		displayLogsPreview(logs, scanCfg)
		out := read()

		assert.Contains(t, out, "... (5 more lines)")
	})
}

func TestDisplayAnalysisResults_OutputAssertions(t *testing.T) {
	t.Run("renders analysis lines and skips blank ones", func(t *testing.T) {
		scanCfg := newTestScanConfig()
		scanCfg.verbose = true

		result := &chunking.AnalyzeResult{
			Analysis:   "first line\n\nsecond line",
			TokensUsed: 100,
			ChunksUsed: 1,
		}

		read := captureStdout(t)
		displayAnalysisResults(result, scanCfg)
		out := read()

		assert.Contains(t, out, "│ first line")
		assert.Contains(t, out, "│ second line")
		assert.NotContains(t, out, "│ \n")
		assert.Contains(t, out, "📊 Tokens used: 100")
		assert.NotContains(t, out, "(chunked analysis)")
	})

	t.Run("multiple chunks note with chunked analysis", func(t *testing.T) {
		scanCfg := newTestScanConfig()
		scanCfg.verbose = true

		result := &chunking.AnalyzeResult{
			Analysis:   "Chunked analysis",
			TokensUsed: 500,
			ChunksUsed: 3,
		}

		read := captureStdout(t)
		displayAnalysisResults(result, scanCfg)
		out := read()

		assert.Contains(t, out, "(chunked analysis)")
	})

	t.Run("filter stats render the exact percentage", func(t *testing.T) {
		scanCfg := newTestScanConfig()
		scanCfg.filterStats = true

		result := &chunking.AnalyzeResult{
			Analysis: "Analysis with filtering",
			FilterStats: chunking.FilterStats{
				LinesTotal:    1000,
				LinesFiltered: 250,
			},
		}

		read := captureStdout(t)
		displayAnalysisResults(result, scanCfg)
		out := read()

		assert.Contains(t, out, "🔍 Regexp Filter: Filtered 250/1000 log lines (25.0%)")
	})

	t.Run("zero total lines shows no filter line", func(t *testing.T) {
		scanCfg := newTestScanConfig()
		scanCfg.filterStats = true

		result := &chunking.AnalyzeResult{
			Analysis:    "Nothing filtered",
			FilterStats: chunking.FilterStats{},
		}

		read := captureStdout(t)
		displayAnalysisResults(result, scanCfg)
		out := read()

		assert.NotContains(t, out, "Regexp Filter")
	})
}

func TestProcessContainers_PrintsTruncatedID(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := state.Load(filepath.Join(tmpDir, "state.json"))
	require.NoError(t, err)

	container := docker.Container{ID: "abc123def456abc123def456", Name: "web-1", State: "running"}
	mockDocker := &MockDockerClient{
		containers: []docker.Container{container},
		logs: map[string][]docker.LogEntry{
			container.ID: scanLogs,
		},
	}

	cfg := &config.Config{
		Output: config.OutputConfig{
			ReportsDir:       filepath.Join(tmpDir, "reports"),
			KnowledgeBaseDir: filepath.Join(tmpDir, "kb"),
		},
	}

	scanCfg := newTestScanConfig()
	scanCfg.dryRun = true // no LLM calls needed for the print assertion

	read := captureStdout(t)
	_, _ = processContainers(context.Background(), mockDocker, st, []docker.Container{container}, cfg, scanCfg, 0)
	out := read()

	assert.Contains(t, out, "[1/1] Processing: web-1 (ID: abc123def456)")
}

// TestEnrichObsoleteWithStorageFlags pins the name guard of
// enrichObsoleteWithStorageFlags: empty-name entries must not be enriched,
// named entries get their storage flags set.
func TestEnrichObsoleteWithStorageFlags(t *testing.T) {
	storage := &storageMaps{
		kbMap:      map[string]bool{"web-app": true, "": true},
		reportsMap: map[string]bool{"web-app": true},
		llmLogsMap: map[string]bool{},
	}
	obsoleteMap := map[string]*ObsoleteContainer{
		"id1": {ID: "id1", Name: "web-app", InState: true},
		"id2": {ID: "id2", Name: ""},
	}

	enrichObsoleteWithStorageFlags(obsoleteMap, storage)

	assert.True(t, obsoleteMap["id1"].InKB, "named entry with KB hit should set InKB")
	assert.True(t, obsoleteMap["id1"].InReports, "named entry with reports hit should set InReports")
	assert.False(t, obsoleteMap["id1"].InLLMLogs, "no LLM logs hit should keep InLLMLogs false")
	// Empty-name entries must not be enriched even if the maps have an
	// entry for the empty string.
	assert.False(t, obsoleteMap["id2"].InKB)
	assert.False(t, obsoleteMap["id2"].InReports)
	assert.False(t, obsoleteMap["id2"].InLLMLogs)
}

func TestHandleReportingAndKnowledge_VerboseCaptured(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Output: config.OutputConfig{
			ReportsDir:       filepath.Join(tmpDir, "reports"),
			KnowledgeBaseDir: filepath.Join(tmpDir, "kb"),
		},
	}

	result := &chunking.AnalyzeResult{Analysis: "Test analysis", TokensUsed: 100}
	logs := []docker.LogEntry{
		{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Test"},
	}

	scanCfg := newTestScanConfig()
	scanCfg.verbose = true

	read := captureStdout(t)
	handleReportingAndKnowledge("test-container", result, logs, cfg, scanCfg)
	out := read()

	assert.Contains(t, out, "Knowledge base updated")
	assert.NotContains(t, out, "Failed to update knowledge base")
	// The KB entry must actually have been written.
	_, statErr := os.Stat(filepath.Join(cfg.Output.KnowledgeBaseDir, "services", "test-container.md"))
	assert.NoError(t, statErr, "KB file should be written on success")
}
