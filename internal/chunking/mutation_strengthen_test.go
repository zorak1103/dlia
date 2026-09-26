package chunking

// Targeted assertion-strengthening tests. Each test pins exact observable
// behavior (boundaries, token arithmetic, message text) so that mutation
// testing can no longer survive on "runs without error" alone.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/llm"
	"github.com/zorak1103/dlia/internal/prompts"
)

// recordingClient captures every call so tests can assert exact call counts,
// prompt contents, and computed token totals.
type recordingClient struct {
	analyzeUserPrompts []string
	summarizePrompts   []string

	analyzeResponse string
	analyzeUsage    *llm.TokenUsage
	analyzeErr      error

	summarizeResponse string
	summarizeErrOn    map[int]error // by call index (0-based)
}

func newRecordingClient() *recordingClient {
	return &recordingClient{
		analyzeResponse:   "final-analysis",
		analyzeUsage:      &llm.TokenUsage{TotalTokens: 150},
		summarizeResponse: "summary",
	}
}

func (c *recordingClient) Analyze(_ context.Context, _, _, userPrompt string) (string, *llm.TokenUsage, error) {
	c.analyzeUserPrompts = append(c.analyzeUserPrompts, userPrompt)
	if c.analyzeErr != nil {
		return "", nil, c.analyzeErr
	}
	return c.analyzeResponse, c.analyzeUsage, nil
}

func (c *recordingClient) SummarizeChunk(_ context.Context, _, _, chunkPrompt string) (string, error) {
	i := len(c.summarizePrompts)
	c.summarizePrompts = append(c.summarizePrompts, chunkPrompt)
	if err, ok := c.summarizeErrOn[i]; ok {
		return "", err
	}
	return c.summarizeResponse, nil
}

func newTestPipeline(t *testing.T, tok TokenizerInterface, client AnalysisClient, maxTokens int) *Pipeline {
	t.Helper()
	return &Pipeline{
		tokenizer:    tok,
		client:       client,
		maxTokens:    maxTokens,
		promptLoader: prompts.NewPromptLoader(&config.Config{}),
	}
}

func threeLogs() []docker.LogEntry {
	return []docker.LogEntry{
		{Timestamp: "2023-01-01T00:00:00Z", Message: "msg one"},
		{Timestamp: "2023-01-01T00:00:01Z", Message: "msg two"},
		{Timestamp: "2023-01-01T00:00:02Z", Message: "msg three"},
	}
}

// --- ChunkLogs: exact split boundaries and arithmetic ---

func TestChunkLogs_ExactBoundaryStaysInOneChunk(t *testing.T) {
	tok := NewMockTokenizer(1)
	logs := threeLogs()
	total := tok.CountTokens(FormatLogs(logs))

	chunks := ChunkLogs(logs, total, tok)
	require.Len(t, chunks, 1, "logs exactly fitting maxTokens must NOT be split (> not >=)")
	assert.Equal(t, total, chunks[0].TokenCount)
	assert.Equal(t, 0, chunks[0].Index)
	assert.Equal(t, 1, chunks[0].Total)
}

func TestChunkLogs_OneOverBoundarySplits(t *testing.T) {
	tok := NewMockTokenizer(1)
	logs := threeLogs()
	t1 := tok.CountTokens(FormatLogs(logs[:1]))
	t2 := tok.CountTokens(FormatLogs(logs[1:2]))
	t3 := tok.CountTokens(FormatLogs(logs[2:]))
	total := tok.CountTokens(FormatLogs(logs))

	chunks := ChunkLogs(logs, total-1, tok)
	require.Len(t, chunks, 2, "one token over the limit must force a split")
	assert.Equal(t, t1+t2, chunks[0].TokenCount)
	assert.Len(t, chunks[0].Logs, 2)
	assert.Equal(t, t3, chunks[1].TokenCount)
	assert.Len(t, chunks[1].Logs, 1)
	assert.Equal(t, []int{0, 1}, []int{chunks[0].Index, chunks[1].Index})
	assert.Equal(t, []int{2, 2}, []int{chunks[0].Total, chunks[1].Total})
}

func TestChunkLogs_EveryLogOversizedGetsOwnChunk(t *testing.T) {
	tok := NewMockTokenizer(1)
	logs := threeLogs()
	t1 := tok.CountTokens(FormatLogs(logs[:1]))

	// Max below one log: each log overflows the current chunk, so each must
	// land in its own chunk, kept whole (never split).
	chunks := ChunkLogs(logs, t1-1, tok)
	require.Len(t, chunks, 3)
	for i, c := range chunks {
		assert.Len(t, c.Logs, 1, "oversized log must stay whole")
		assert.Equal(t, tok.CountTokens(FormatLogs(c.Logs)), c.TokenCount)
		assert.Equal(t, i, c.Index)
		assert.Equal(t, 3, c.Total)
	}
}

func TestChunkLogs_FinalFlushRequired(t *testing.T) {
	tok := NewMockTokenizer(1)
	logs := threeLogs()

	chunks := ChunkLogs(logs, 1_000_000, tok)
	require.Len(t, chunks, 1, "final flush must emit the accumulated chunk")
	assert.Len(t, chunks[0].Logs, 3)
}

// --- Deduplicate: threshold boundary and sequence accounting ---

func TestDeduplicate_ExactlyAtThresholdCollapses(t *testing.T) {
	got := Deduplicate([]docker.LogEntry{{Message: "a"}, {Message: "a"}, {Message: "a"}})
	require.Len(t, got, 1)
	assert.Equal(t, "[REPEAT x3] a", got[0].Message)
}

func TestDeduplicate_JustBelowThresholdKeptAsIs(t *testing.T) {
	got := Deduplicate([]docker.LogEntry{{Message: "a"}, {Message: "a"}})
	require.Len(t, got, 2)
	assert.Equal(t, "a", got[0].Message)
	assert.Equal(t, "a", got[1].Message)
}

func TestDeduplicate_SequenceLengthCountedFromSeqStart(t *testing.T) {
	// Sequence does not start at index 0, so endIdx-seqStart must be 3.
	got := Deduplicate([]docker.LogEntry{
		{Message: "x"}, {Message: "a"}, {Message: "a"}, {Message: "a"},
	})
	require.Len(t, got, 2)
	assert.Equal(t, "x", got[0].Message)
	assert.Equal(t, "[REPEAT x3] a", got[1].Message)
}

func TestDeduplicate_BelowThresholdFlushStopsAtSequenceEnd(t *testing.T) {
	got := Deduplicate([]docker.LogEntry{
		{Message: "x"}, {Message: "a"}, {Message: "a"}, {Message: "b"},
	})
	require.Len(t, got, 4)
	assert.Equal(t, []string{"x", "a", "a", "b"}, []string{
		got[0].Message, got[1].Message, got[2].Message, got[3].Message,
	})
}

func TestFormatLogs_ExactMixedOutput(t *testing.T) {
	got := FormatLogs([]docker.LogEntry{
		{Timestamp: "t1", Message: "with ts"},
		{Message: "no ts"},
	})
	assert.Equal(t, "[t1] with ts\nno ts\n", got)
}

// --- Tokenizer: reserve arithmetic and fit boundary ---

func TestTokenizer_EstimateAddsExactOverhead(t *testing.T) {
	tok, err := NewTokenizer("gpt-4")
	require.NoError(t, err)
	text := "some prompt text"

	assert.Equal(t, tok.CountTokens(text)+4, tok.EstimateSystemPromptTokens(text))
	assert.Equal(t, tok.CountTokens(text)+4, tok.EstimateUserPromptTokens(text))
}

func TestTokenizer_WillFitInContext_InclusiveBoundary(t *testing.T) {
	tok, err := NewTokenizer("gpt-4")
	require.NoError(t, err)
	text := "boundary text"
	n := tok.CountTokens(text)

	assert.True(t, tok.WillFitInContext(text, n), "exactly fitting content must fit (<=)")
	assert.False(t, tok.WillFitInContext(text, n-1), "one token short must not fit")
}

func TestTokenizer_EmptyStringHasZeroTokens(t *testing.T) {
	tok, err := NewTokenizer("gpt-4")
	require.NoError(t, err)
	assert.Equal(t, 0, tok.CountTokens(""))
}

// --- RegexpFilter: no-pattern passthrough and exact stats ---

func TestRegexpFilter_NoPatternsPassesThroughWithZeroStats(t *testing.T) {
	rf, err := NewRegexpFilter(nil)
	require.NoError(t, err)

	logs := []string{"keep 1", "keep 2", "keep 3"}
	filtered, stats := rf.Filter(logs)
	assert.Equal(t, logs, filtered)
	assert.Equal(t, FilterStats{LinesTotal: 3, LinesFiltered: 0, LinesKept: 3}, stats)

	assert.False(t, rf.MatchesAny("keep 1"))
}

func TestRegexpFilter_ExactStats(t *testing.T) {
	rf, err := NewRegexpFilter([]string{"DEBUG"})
	require.NoError(t, err)

	logs := []string{"DEBUG: noisy", "INFO: fine", "debug lower", "ERROR: fine too"}
	filtered, stats := rf.Filter(logs)
	assert.Equal(t, []string{"INFO: fine", "debug lower", "ERROR: fine too"}, filtered)
	assert.Equal(t, FilterStats{LinesTotal: 4, LinesFiltered: 1, LinesKept: 3}, stats)
}

func TestNewRegexpFilter_InvalidPatternNamesIndex(t *testing.T) {
	_, err := NewRegexpFilter([]string{"ok", "["})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "index 1")
	assert.Contains(t, err.Error(), `"["`)
}

// --- Pipeline: direct vs chunked decision, budget arithmetic, prompts ---

func TestPipeline_DirectPathWhenBudgetAllows(t *testing.T) {
	tok := NewMockTokenizer(1)
	client := newRecordingClient()

	logs := threeLogs()
	loader := prompts.NewPromptLoader(&config.Config{})
	systemPrompt, err := loader.SystemPrompt("")
	require.NoError(t, err)
	basePrompt, err := loader.AnalysisPrompt("c", "", 3)
	require.NoError(t, err)
	total := tok.EstimateSystemPromptTokens(systemPrompt) + tok.CountTokens(basePrompt) +
		tok.CountTokens(FormatLogs(logs))

	p := newTestPipeline(t, tok, client, total+ResponseReserveTokens)
	res, err := p.AnalyzeLogs(context.Background(), "c", logs)
	require.NoError(t, err)

	assert.Empty(t, client.summarizePrompts, "budget allows direct analysis: no chunk summaries")
	require.Len(t, client.analyzeUserPrompts, 1)
	assert.Equal(t, "final-analysis", res.Analysis)
	assert.Equal(t, 1, res.ChunksUsed)
	assert.Equal(t, 150, res.TokensUsed)
	assert.False(t, res.Deduplicated)
	assert.Equal(t, 3, res.OriginalCount)
	assert.Equal(t, 3, res.ProcessedCount)
}

// newForcedChunkedPipeline builds a pipeline whose budget only allows two
// small chunks (independent of the embedded prompt sizes): available tokens
// 140, chunk size 70, so threeLogs() splits 2+1.
func newForcedChunkedPipeline(t *testing.T, tok TokenizerInterface, client AnalysisClient) (*Pipeline, int) {
	t.Helper()
	loader := prompts.NewPromptLoader(&config.Config{})
	sysPrompt, err := loader.SystemPrompt("")
	require.NoError(t, err)
	sysTok := tok.EstimateSystemPromptTokens(sysPrompt)
	maxTokens := ResponseReserveTokens + sysTok + 140
	p := &Pipeline{
		tokenizer:    tok,
		client:       client,
		maxTokens:    maxTokens,
		promptLoader: loader,
	}
	return p, 70 // chunk budget used by the pipeline (availableTokens / ChunkSizeDivisor)
}

func TestPipeline_OneTokenShortForcesChunking(t *testing.T) {
	tok := NewMockTokenizer(1)
	client := newRecordingClient()

	logs := threeLogs()
	p, chunkBudget := newForcedChunkedPipeline(t, tok, client)
	expectedChunks := ChunkLogs(logs, chunkBudget, tok)
	require.Greater(t, len(expectedChunks), 1, "test setup must force multiple chunks")

	res, err := p.AnalyzeLogs(context.Background(), "c", logs)
	require.NoError(t, err)

	require.Len(t, client.summarizePrompts, len(expectedChunks),
		"chunk size is availableTokens/%d", ChunkSizeDivisor)
	require.Len(t, client.analyzeUserPrompts, 1, "exactly one synthesis analysis call")
	assert.Equal(t, len(expectedChunks), res.ChunksUsed)
	assert.Equal(t, "final-analysis", res.Analysis)

	// Exact token accounting: per-chunk prompt + summary tokens, plus synthesis usage.
	var expectedTokens int
	for i, chunk := range expectedChunks {
		expectedTokens += tok.CountTokens(FormatChunk(chunk)) + tok.CountTokens("summary")
		_ = i
	}
	expectedTokens += 150
	assert.Equal(t, expectedTokens, res.TokensUsed)
}

func TestPipeline_ChunkPromptsAreNumberedOneBased(t *testing.T) {
	tok := NewMockTokenizer(1)
	client := newRecordingClient()

	p, _ := newForcedChunkedPipeline(t, tok, client)
	_, err := p.AnalyzeLogs(context.Background(), "c", threeLogs())
	require.NoError(t, err)

	require.Len(t, client.summarizePrompts, 2)
	assert.Contains(t, client.summarizePrompts[0], "chunk 1 of 2")
	assert.Contains(t, client.summarizePrompts[1], "chunk 2 of 2")
}

func TestPipeline_SummarizeErrorMessageCarriesChunkNumber(t *testing.T) {
	tok := NewMockTokenizer(1)
	client := newRecordingClient()
	client.summarizeErrOn = map[int]error{1: fmt.Errorf("boom")}

	p, _ := newForcedChunkedPipeline(t, tok, client)
	_, err := p.AnalyzeLogs(context.Background(), "c", threeLogs())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chunk 2/2", "second chunk failure must be reported as 2/2")
	assert.Contains(t, err.Error(), "boom")
}

func TestPipeline_SynthesisErrorCarriesChunkCount(t *testing.T) {
	tok := NewMockTokenizer(1)
	client := newRecordingClient()
	client.analyzeErr = fmt.Errorf("synth down")

	p, _ := newForcedChunkedPipeline(t, tok, client)
	_, err := p.AnalyzeLogs(context.Background(), "c", threeLogs())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "synthesize 2 chunk summaries")
	assert.Contains(t, err.Error(), "synth down")
}

func TestPipeline_DeduplicationFlagBothDirections(t *testing.T) {
	tok := NewMockTokenizer(1)
	client := newRecordingClient()
	p := newTestPipeline(t, tok, client, 1_000_000)

	dup := []docker.LogEntry{
		{Message: "a"}, {Message: "a"}, {Message: "a"},
		{Message: "b"},
	}
	res, err := p.AnalyzeLogs(context.Background(), "c", dup)
	require.NoError(t, err)
	assert.True(t, res.Deduplicated, "4 lines collapsed to 2 must be flagged")
	assert.Equal(t, 4, res.OriginalCount)
	assert.Equal(t, 2, res.ProcessedCount)

	unique := []docker.LogEntry{{Message: "a"}, {Message: "b"}}
	res2, err := p.AnalyzeLogs(context.Background(), "c", unique)
	require.NoError(t, err)
	assert.False(t, res2.Deduplicated, "no duplicates must not be flagged")
	assert.Equal(t, 2, res2.OriginalCount)
	assert.Equal(t, 2, res2.ProcessedCount)
}

func TestPipeline_FilteredStatsAndCount(t *testing.T) {
	tok := NewMockTokenizer(1)
	client := newRecordingClient()
	rf, err := NewRegexpFilter([]string{"^DROP"})
	require.NoError(t, err)

	p := newTestPipeline(t, tok, client, 1_000_000)
	p.compiledRegexpsByContainer = map[string]*RegexpFilter{"c": rf}

	logs := []docker.LogEntry{
		{Message: "DROP me"},
		{Message: "keep me"},
		{Message: "keep me too"},
	}
	res, err := p.AnalyzeLogs(context.Background(), "c", logs)
	require.NoError(t, err)
	assert.Equal(t, FilterStats{LinesTotal: 3, LinesFiltered: 1, LinesKept: 2}, res.FilterStats)
	assert.Equal(t, 2, res.ProcessedCount, "processed count must reflect filtering")
	require.NotEmpty(t, client.analyzeUserPrompts)
	assert.Contains(t, client.analyzeUserPrompts[0], "keep me")
	assert.NotContains(t, client.analyzeUserPrompts[0], "DROP me")
}

func TestPipeline_UnfilteredContainerHasIdentityStats(t *testing.T) {
	tok := NewMockTokenizer(1)
	client := newRecordingClient()
	rf, err := NewRegexpFilter([]string{"^DROP"})
	require.NoError(t, err)

	p := newTestPipeline(t, tok, client, 1_000_000)
	p.compiledRegexpsByContainer = map[string]*RegexpFilter{"other": rf}

	res, err := p.AnalyzeLogs(context.Background(), "c", threeLogs())
	require.NoError(t, err)
	assert.Equal(t, FilterStats{LinesTotal: 3, LinesFiltered: 0, LinesKept: 3}, res.FilterStats)
	assert.Equal(t, 3, res.ProcessedCount)
}

func TestPipeline_EmptyLogsShortCircuit(t *testing.T) {
	tok := NewMockTokenizer(1)
	client := newRecordingClient()
	p := newTestPipeline(t, tok, client, 1_000_000)

	res, err := p.AnalyzeLogs(context.Background(), "c", nil)
	require.NoError(t, err)
	assert.Equal(t, "No logs to analyze", res.Analysis)
	assert.Empty(t, client.analyzeUserPrompts)
	assert.Empty(t, client.summarizePrompts)
}

func TestPipeline_NegativeBudgetStillChunksSingle(t *testing.T) {
	tok := NewMockTokenizer(1)
	client := newRecordingClient()

	// ponytail-documented edge: availableTokens below zero makes chunk size
	// negative, yet ChunkLogs still emits one oversized chunk instead of
	// refusing. Pinned as current behavior; fix separately if it matters.
	huge := docker.LogEntry{Message: strings.Repeat("x", 10_000)}
	p := newTestPipeline(t, tok, client, ResponseReserveTokens+1)

	res, err := p.AnalyzeLogs(context.Background(), "c", []docker.LogEntry{huge})
	require.NoError(t, err)
	assert.Equal(t, 1, res.ChunksUsed)
	require.Len(t, client.summarizePrompts, 1)
	require.Len(t, client.analyzeUserPrompts, 1)
}
