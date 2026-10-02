package chunking

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/prompts"
)

func findMarker(t *testing.T, s, kind string) string {
	t.Helper()
	got := regexp.MustCompile(`<` + kind + `-[0-9a-f]{32}>`).FindString(s)
	require.NotEmpty(t, got, "no %s marker found in %q", kind, s)
	return got
}

// sharedMarker asserts that system and user prompt carry the same marker and returns it.
func sharedMarker(t *testing.T, system, user, kind string) string {
	t.Helper()
	m := findMarker(t, user, kind)
	assert.Contains(t, system, m, "system prompt must name the user prompt's marker")
	assert.Contains(t, user, "</"+m[1:], "user prompt must contain the closing marker")
	return m
}

func customLoader(t *testing.T, cfg config.PromptsConfig) *prompts.PromptLoader {
	t.Helper()
	return prompts.NewPromptLoader(&config.Config{Prompts: cfg})
}

func writeTemplate(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	return path
}

func TestAnalyzeLogs_Direct_MarkerInSystemAndUser(t *testing.T) {
	client := newRecordingClient()
	p := newTestPipeline(t, NewMockTokenizer(0.1), client, 1_000_000)

	_, err := p.AnalyzeLogs(context.Background(), "c", oneLogs())
	require.NoError(t, err)

	require.Len(t, client.analyzeUserPrompts, 1)
	system, user := client.analyzeSystemPrompts[0], client.analyzeUserPrompts[0]
	m := sharedMarker(t, system, user, "logs")

	open := strings.Index(user, m)
	closeIdx := strings.Index(user, "</"+m[1:])
	require.Greater(t, closeIdx, open)
	assert.Contains(t, user[open:closeIdx], "hello", "log message must sit between the markers")
}

func TestAnalyzeLogs_Chunked_EachCallOwnMarker(t *testing.T) {
	client := newRecordingClient()
	p, _ := newForcedChunkedPipeline(t, NewMockTokenizer(1), client)

	p.ignoreDir = t.TempDir()
	const ignoreText = "IGNORE-ME-PLEASE"
	require.NoError(t, os.WriteFile(filepath.Join(p.ignoreDir, "c.md"), []byte(ignoreText), 0600))

	_, err := p.AnalyzeLogs(context.Background(), "c", threeLogs())
	require.NoError(t, err)

	// The ignore text enlarges the system prompt, so the chunk count may exceed 2.
	require.GreaterOrEqual(t, len(client.summarizePrompts), 2)
	require.Len(t, client.analyzeUserPrompts, 1)

	markers := make([]string, 0, len(client.summarizePrompts)+1)
	for i, user := range client.summarizePrompts {
		markers = append(markers, sharedMarker(t, client.summarizeSystemPrompts[i], user, "logs"))
	}
	markers = append(markers, sharedMarker(t, client.analyzeSystemPrompts[0], client.analyzeUserPrompts[0], "summaries"))

	seen := map[string]bool{}
	for _, m := range markers {
		assert.False(t, seen[m], "marker %s reused across calls", m)
		seen[m] = true
	}

	systems := append(append([]string{}, client.summarizeSystemPrompts...), client.analyzeSystemPrompts...)
	users := append(append([]string{}, client.summarizePrompts...), client.analyzeUserPrompts...)
	for _, s := range systems {
		assert.Contains(t, s, ignoreText, "ignore instructions belong in every system prompt")
	}
	for _, u := range users {
		assert.NotContains(t, u, ignoreText, "ignore instructions must never reach a user prompt")
	}
}

func TestAnalyzeLogs_PromptErrorFails(t *testing.T) {
	bad := writeTemplate(t, "bad.md", "{{.Nope}}")

	t.Run("analysis", func(t *testing.T) {
		client := newRecordingClient()
		p := newTestPipeline(t, NewMockTokenizer(0.1), client, 1_000_000)
		p.promptLoader = customLoader(t, config.PromptsConfig{AnalysisPrompt: bad})

		_, err := p.AnalyzeLogs(context.Background(), "c", oneLogs())
		require.Error(t, err)
		assert.Empty(t, client.analyzeUserPrompts)
	})

	t.Run("chunk", func(t *testing.T) {
		client := newRecordingClient()
		p, _ := newForcedChunkedPipeline(t, NewMockTokenizer(1), client)
		p.promptLoader = customLoader(t, config.PromptsConfig{ChunkSummaryPrompt: bad})

		_, err := p.AnalyzeLogs(context.Background(), "c", threeLogs())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "chunk summary prompt")
		assert.Empty(t, client.summarizePrompts)
	})

	t.Run("synthesis", func(t *testing.T) {
		client := newRecordingClient()
		p, _ := newForcedChunkedPipeline(t, NewMockTokenizer(1), client)
		p.promptLoader = customLoader(t, config.PromptsConfig{SynthesisPrompt: bad})

		_, err := p.AnalyzeLogs(context.Background(), "c", threeLogs())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "synthesis prompt")
		assert.Empty(t, client.analyzeUserPrompts)
	})
}
