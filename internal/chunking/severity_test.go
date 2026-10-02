package chunking

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/prompts"
	"github.com/zorak1103/dlia/internal/severity"
)

// newCustomPromptLoader creates a PromptLoader that uses a temp file as the
// analysis prompt, containing the given template content.
func newCustomPromptLoader(t *testing.T, analysisTemplate string) *prompts.PromptLoader {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "analysis_prompt.md")
	require.NoError(t, os.WriteFile(path, []byte(analysisTemplate), 0600))
	cfg := &config.Config{
		Prompts: config.PromptsConfig{AnalysisPrompt: path},
	}
	return prompts.NewPromptLoader(cfg)
}

// oneLogs returns a single log entry for direct (non-chunked) pipeline tests.
func oneLogs() []docker.LogEntry {
	return []docker.LogEntry{
		{Timestamp: "2023-01-01T00:00:00Z", Message: "hello"},
	}
}

// TestAnalyzeLogs_Direct_AppendsInstructionAndParses verifies that the direct
// path appends the SEVERITY instruction to the user prompt and parses the
// returned severity line from the LLM response.
func TestAnalyzeLogs_Direct_AppendsInstructionAndParses(t *testing.T) {
	client := newRecordingClient()
	client.analyzeResponse = "Fine.\nSEVERITY: warning"

	p := newTestPipeline(t, NewMockTokenizer(0.1), client, 1_000_000)

	res, err := p.AnalyzeLogs(context.Background(), "c", oneLogs())
	require.NoError(t, err)

	require.Len(t, client.analyzeUserPrompts, 1)
	assert.True(t,
		strings.HasSuffix(client.analyzeUserPrompts[0], "\n\n"+severity.Instruction),
		"user prompt must end with severity instruction, got:\n%s",
		client.analyzeUserPrompts[0],
	)
	assert.Equal(t, severity.Warning, res.Severity)
	assert.Equal(t, "Fine.", res.Analysis)
}

// TestAnalyzeLogs_Chunked_InstructionOnlyInSynthesis verifies that chunk
// summary prompts do NOT contain the SEVERITY instruction, while the synthesis
// prompt does, and the parsed severity is set on the result.
func TestAnalyzeLogs_Chunked_InstructionOnlyInSynthesis(t *testing.T) {
	tok := NewMockTokenizer(1)
	client := newRecordingClient()
	client.analyzeResponse = "x\nSEVERITY: critical"
	client.summarizeResponse = "summary"

	p, _ := newForcedChunkedPipeline(t, tok, client)

	res, err := p.AnalyzeLogs(context.Background(), "c", threeLogs())
	require.NoError(t, err)

	// All chunk summary prompts must NOT contain SEVERITY
	for i, sp := range client.summarizePrompts {
		assert.NotContains(t, sp, "SEVERITY:",
			"chunk summary prompt %d must not contain SEVERITY instruction", i)
	}

	// The single synthesis prompt must end with the instruction
	require.Len(t, client.analyzeUserPrompts, 1)
	assert.True(t,
		strings.HasSuffix(client.analyzeUserPrompts[0], "\n\n"+severity.Instruction),
		"synthesis prompt must end with severity instruction",
	)

	assert.Equal(t, severity.Critical, res.Severity)
	assert.Equal(t, "x", res.Analysis)
}

// TestAnalyzeLogs_CustomTemplate_GetsInstruction ensures that even when a
// custom analysis prompt template is configured, the SEVERITY instruction is
// still appended in code.
func TestAnalyzeLogs_CustomTemplate_GetsInstruction(t *testing.T) {
	// Custom template without any SEVERITY mention
	const customTemplate = "Analyze logs for {{.ContainerName}}:\n{{.Logs}}"
	loader := newCustomPromptLoader(t, customTemplate)

	client := newRecordingClient()
	client.analyzeResponse = "all good\nSEVERITY: ok"

	tok := NewMockTokenizer(0.1)
	p := &Pipeline{
		tokenizer:       tok,
		client:          client,
		maxTokens:       1_000_000,
		responseReserve: DefaultResponseReserveTokens,
		promptLoader:    loader,
	}

	res, err := p.AnalyzeLogs(context.Background(), "c", oneLogs())
	require.NoError(t, err)

	require.Len(t, client.analyzeUserPrompts, 1)
	assert.True(t,
		strings.HasSuffix(client.analyzeUserPrompts[0], "\n\n"+severity.Instruction),
		"custom-template prompt must still end with severity instruction",
	)
	assert.Equal(t, severity.OK, res.Severity)
	assert.Equal(t, "all good", res.Analysis)
}

// TestAnalyzeLogs_MissingSeverity_Unknown verifies that when the LLM response
// contains no SEVERITY line, the result has Unknown severity and the analysis
// text is returned unchanged.
func TestAnalyzeLogs_MissingSeverity_Unknown(t *testing.T) {
	client := newRecordingClient()
	client.analyzeResponse = "no severity line here"

	p := newTestPipeline(t, NewMockTokenizer(0.1), client, 1_000_000)

	res, err := p.AnalyzeLogs(context.Background(), "c", oneLogs())
	require.NoError(t, err)

	assert.Equal(t, severity.Unknown, res.Severity)
	assert.Equal(t, "no severity line here", res.Analysis)
}

// TestAnalyzeLogs_NoLogs_OK verifies that the empty-log early return yields
// Severity == OK (zero value, no LLM call).
func TestAnalyzeLogs_NoLogs_OK(t *testing.T) {
	client := newRecordingClient()
	p := newTestPipeline(t, NewMockTokenizer(0.1), client, 1_000_000)

	res, err := p.AnalyzeLogs(context.Background(), "c", []docker.LogEntry{})
	require.NoError(t, err)

	assert.Equal(t, severity.OK, res.Severity)
	assert.Empty(t, client.analyzeUserPrompts)
}

// TestAnalyzeLogs_SeverityOnlyAnswer_Direct_Error verifies that an LLM answer
// containing only a SEVERITY line is rejected with ErrSeverityOnlyAnswer on the
// direct path: the report would otherwise render an empty analysis box.
func TestAnalyzeLogs_SeverityOnlyAnswer_Direct_Error(t *testing.T) {
	client := newRecordingClient()
	client.analyzeResponse = "SEVERITY: ok"

	p := newTestPipeline(t, NewMockTokenizer(0.1), client, 1_000_000)

	_, err := p.AnalyzeLogs(context.Background(), "c", oneLogs())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSeverityOnlyAnswer)
	assert.Contains(t, err.Error(),
		"analysis for container c empty: model returned only a severity line")
}

// TestAnalyzeLogs_SeverityOnlyAnswer_Chunked_Error verifies that the chunked
// path rejects a severity-only synthesis answer with the same error.
func TestAnalyzeLogs_SeverityOnlyAnswer_Chunked_Error(t *testing.T) {
	tok := NewMockTokenizer(1)
	client := newRecordingClient()
	client.analyzeResponse = "SEVERITY: ok"

	p, _ := newForcedChunkedPipeline(t, tok, client)

	_, err := p.AnalyzeLogs(context.Background(), "c", threeLogs())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSeverityOnlyAnswer)
	assert.Contains(t, err.Error(),
		"analysis for container c empty: model returned only a severity line")
}

// TestAnalyzeLogs_SeverityOnlyAnswer_UnrecognizedValue_Error verifies that a
// severity-only answer with an unrecognized value fails too: the line is
// removed, so nothing usable remains.
func TestAnalyzeLogs_SeverityOnlyAnswer_UnrecognizedValue_Error(t *testing.T) {
	client := newRecordingClient()
	client.analyzeResponse = "SEVERITY: banana"

	p := newTestPipeline(t, NewMockTokenizer(0.1), client, 1_000_000)

	_, err := p.AnalyzeLogs(context.Background(), "c", oneLogs())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSeverityOnlyAnswer)
}

// TestAnalyzeLogs_SeverityOnlyAnswer_TrailingWhitespace_Error verifies that a
// whitespace-only remainder after removing the severity line counts as an
// empty analysis.
func TestAnalyzeLogs_SeverityOnlyAnswer_TrailingWhitespace_Error(t *testing.T) {
	client := newRecordingClient()
	client.analyzeResponse = "SEVERITY: ok\n  \n"

	p := newTestPipeline(t, NewMockTokenizer(0.1), client, 1_000_000)

	_, err := p.AnalyzeLogs(context.Background(), "c", oneLogs())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSeverityOnlyAnswer)
}

// TestAnalyzeLogs_SeverityOnlyAnswer_Fenced_Error documents the interplay
// between the empty-fence removal and the severity-only check: a fenced
// severity-only answer cleans to "" and must fail as well.
func TestAnalyzeLogs_SeverityOnlyAnswer_Fenced_Error(t *testing.T) {
	client := newRecordingClient()
	client.analyzeResponse = "```\nSEVERITY: ok\n```"

	p := newTestPipeline(t, NewMockTokenizer(0.1), client, 1_000_000)

	_, err := p.AnalyzeLogs(context.Background(), "c", oneLogs())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSeverityOnlyAnswer)
}

// TestAnalyzeLogs_SeverityWithText_NoError guards against over-eagerness: text
// after the severity line means the analysis is not empty and must succeed.
func TestAnalyzeLogs_SeverityWithText_NoError(t *testing.T) {
	client := newRecordingClient()
	client.analyzeResponse = "SEVERITY: ok\n\nLet me know"

	p := newTestPipeline(t, NewMockTokenizer(0.1), client, 1_000_000)

	res, err := p.AnalyzeLogs(context.Background(), "c", oneLogs())
	require.NoError(t, err)
	assert.Equal(t, severity.OK, res.Severity)
}
