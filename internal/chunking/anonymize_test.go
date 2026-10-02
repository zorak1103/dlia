package chunking

import (
	"context"
	"errors"
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

func privacyConfig(ips, secrets bool) *config.Config {
	return &config.Config{Privacy: config.PrivacyConfig{AnonymizeIPs: ips, AnonymizeSecrets: secrets}}
}

func TestAnalyzeLogs_MasksBeforeLLM_Direct(t *testing.T) {
	client := newRecordingClient()
	p := newTestPipeline(t, NewMockTokenizer(1), client, 1_000_000)
	p.config = privacyConfig(true, true)

	logs := []docker.LogEntry{{Timestamp: "2023-01-01T00:00:00Z", Message: "login from 203.0.113.7 password=hunter2"}}
	_, err := p.AnalyzeLogs(context.Background(), "c", logs)
	require.NoError(t, err)

	require.Len(t, client.analyzeUserPrompts, 1)
	prompt := client.analyzeUserPrompts[0]
	assert.Contains(t, prompt, "<IP-1>")
	assert.Contains(t, prompt, "password=<SECRET>")
	assert.NotContains(t, prompt, "203.0.113.7")
	assert.NotContains(t, prompt, "hunter2")
}

func TestAnalyzeLogs_MasksBeforeLLM_Chunked(t *testing.T) {
	client := newRecordingClient()
	p, _ := newForcedChunkedPipeline(t, NewMockTokenizer(1), client)
	p.config = privacyConfig(true, true)

	var logs []docker.LogEntry
	for i := 1; i <= 20; i++ {
		logs = append(logs, docker.LogEntry{
			Timestamp: fmt.Sprintf("2023-01-01T00:00:%02dZ", i),
			Message:   fmt.Sprintf("conn from 203.0.113.%d token=secret%d", i, i),
		})
	}

	res, err := p.AnalyzeLogs(context.Background(), "c", logs)
	require.NoError(t, err)
	require.Greater(t, res.ChunksUsed, 1, "test setup must force chunking")
	require.Len(t, client.summarizePrompts, res.ChunksUsed)
	require.Len(t, client.analyzeUserPrompts, 1)

	all := append(append([]string{}, client.summarizePrompts...), client.analyzeUserPrompts...)
	for _, prompt := range all {
		assert.NotContains(t, prompt, "203.0.113.")
		assert.NotContains(t, prompt, "secret1")
	}
	assert.Contains(t, strings.Join(client.summarizePrompts, "\n"), "<IP-")
	assert.Contains(t, strings.Join(client.summarizePrompts, "\n"), "token=<SECRET>")
}

// TestAnalyzeLogs_PlaceholderConsistentAcrossChunks pins that one session masks
// the whole batch: an IP repeated in a later chunk keeps its placeholder number.
// The layout forces the second IP to appear first in chunk 1 (as <IP-2> there)
// and alone in chunk 2 — a fresh session per chunk would renumber it to <IP-1>.
func TestAnalyzeLogs_PlaceholderConsistentAcrossChunks(t *testing.T) {
	client := newRecordingClient()
	tok := NewMockTokenizer(1)
	loader := prompts.NewPromptLoader(&config.Config{})
	base, err := loader.AnalysisMessages("c", "", "", 3)
	require.NoError(t, err)
	maxTokens := DefaultResponseReserveTokens + tok.EstimateSystemPromptTokens(base.System) + 240 // chunk budget 120
	p := &Pipeline{
		tokenizer:       tok,
		client:          client,
		maxTokens:       maxTokens,
		responseReserve: DefaultResponseReserveTokens,
		promptLoader:    loader,
	}
	p.config = privacyConfig(true, true)

	logs := []docker.LogEntry{
		{Timestamp: "2023-01-01T00:00:00Z", Message: "from 198.51.100.9 to 203.0.113.7"},
		{Timestamp: "2023-01-01T00:00:01Z", Message: "from 198.51.100.9 again"},
		{Timestamp: "2023-01-01T00:00:02Z", Message: "from 203.0.113.7 again"},
	}

	_, err = p.AnalyzeLogs(context.Background(), "c", logs)
	require.NoError(t, err)
	require.Greater(t, len(client.summarizePrompts), 1, "test setup must force chunking")

	first := client.summarizePrompts[0]
	assert.Contains(t, first, "<IP-1>")
	assert.Contains(t, first, "<IP-2>")

	last := client.summarizePrompts[len(client.summarizePrompts)-1]
	assert.Contains(t, last, "<IP-2>", "the repeated IP keeps its placeholder in the next chunk")
	assert.NotContains(t, last, "<IP-1>", "a fresh session per chunk would renumber the repeated IP to <IP-1>")

	for _, prompt := range client.summarizePrompts {
		assert.NotContains(t, prompt, "198.51.100.")
		assert.NotContains(t, prompt, "203.0.113.7")
	}
}

func TestAnalyzeLogs_DoesNotMutateInput(t *testing.T) {
	client := newRecordingClient()
	p := newTestPipeline(t, NewMockTokenizer(1), client, 1_000_000)
	p.config = privacyConfig(true, true)

	logs := []docker.LogEntry{
		{Timestamp: "2023-01-01T00:00:00Z", Message: "from 203.0.113.7 password=hunter2"},
		{Timestamp: "2023-01-01T00:00:01Z", Message: "from 198.51.100.9"},
	}
	want := append([]docker.LogEntry(nil), logs...)

	_, err := p.AnalyzeLogs(context.Background(), "c", logs)
	require.NoError(t, err)
	assert.Equal(t, want, logs)
}

func TestAnalyzeLogs_PrivacyOff(t *testing.T) {
	client := newRecordingClient()
	p := newTestPipeline(t, NewMockTokenizer(1), client, 1_000_000)
	p.config = privacyConfig(false, false)

	logs := []docker.LogEntry{{Message: "login from 203.0.113.7 password=hunter2"}}
	_, err := p.AnalyzeLogs(context.Background(), "c", logs)
	require.NoError(t, err)

	require.Len(t, client.analyzeUserPrompts, 1)
	assert.Contains(t, client.analyzeUserPrompts[0], "203.0.113.7")
	assert.Contains(t, client.analyzeUserPrompts[0], "hunter2")
}

func TestAnalyzeLogs_NilConfigNoMasking(t *testing.T) {
	client := newRecordingClient()
	p := newTestPipeline(t, NewMockTokenizer(1), client, 1_000_000)
	p.config = nil

	logs := []docker.LogEntry{{Message: "login from 203.0.113.7"}}
	require.NotPanics(t, func() {
		_, err := p.AnalyzeLogs(context.Background(), "c", logs)
		require.NoError(t, err)
	})

	require.Len(t, client.analyzeUserPrompts, 1)
	assert.Contains(t, client.analyzeUserPrompts[0], "203.0.113.7")
}

func TestAnalyzeLogs_MaskingKeepsCounts(t *testing.T) {
	client := newRecordingClient()
	p := newTestPipeline(t, NewMockTokenizer(1), client, 1_000_000)
	p.config = privacyConfig(true, true)

	res, err := p.AnalyzeLogs(context.Background(), "c", []docker.LogEntry{
		{Message: "from 203.0.113.7"}, {Message: "from 203.0.113.8"},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, res.OriginalCount)
	assert.Equal(t, 2, res.ProcessedCount)
}

func TestPipeline_ResponseReserveFollowsConfig(t *testing.T) {
	loader := prompts.NewPromptLoader(&config.Config{})
	logs := threeLogs()

	probe, err := NewPipelineWithConfig("gpt-4", 100000, newRecordingClient(), loader, "", &config.Config{
		LLM: config.LLMConfig{MaxAnswerTokens: 4000},
	})
	require.NoError(t, err)
	base, err := loader.AnalysisMessages("c", "", "", len(logs))
	require.NoError(t, err)
	total := probe.tokenizer.EstimateSystemPromptTokens(base.System) +
		probe.tokenizer.CountTokens(base.User) +
		probe.tokenizer.CountTokens(FormatLogs(logs))
	// slack: the random marker hex tokenizes variably between calls
	const slack = 50
	window := total + 4000 + slack

	run := func(maxAnswer int) (*AnalyzeResult, *recordingClient) {
		client := newRecordingClient()
		p, err := NewPipelineWithConfig("gpt-4", window, client, loader, "", &config.Config{
			LLM: config.LLMConfig{MaxAnswerTokens: maxAnswer},
		})
		require.NoError(t, err)
		require.Equal(t, maxAnswer, p.responseReserve)
		res, err := p.AnalyzeLogs(context.Background(), "c", logs)
		require.NoError(t, err)
		return res, client
	}

	res, client := run(4000)
	assert.Equal(t, 1, res.ChunksUsed)
	assert.Empty(t, client.summarizePrompts, "fits with reserve 4000")

	res, client = run(4000 + 2*slack)
	assert.NotEmpty(t, client.summarizePrompts, "larger max_answer_tokens must force chunking")
	assert.GreaterOrEqual(t, res.ChunksUsed, 1)
}

func TestPipeline_ResponseReserveDefault(t *testing.T) {
	loader := prompts.NewPromptLoader(&config.Config{})

	p, err := NewPipelineWithConfig("gpt-4", 8000, newRecordingClient(), loader, "", nil)
	require.NoError(t, err)
	assert.Equal(t, DefaultResponseReserveTokens, p.responseReserve)

	p, err = NewPipelineWithConfig("gpt-4", 8000, newRecordingClient(), loader, "", &config.Config{})
	require.NoError(t, err)
	assert.Equal(t, DefaultResponseReserveTokens, p.responseReserve)
}

func TestAnalyzeLogs_IncompleteAnswerPropagates(t *testing.T) {
	incomplete := &llm.IncompleteAnswerError{Reason: "empty answer"}

	t.Run("direct", func(t *testing.T) {
		client := newRecordingClient()
		client.analyzeErr = incomplete
		p := newTestPipeline(t, NewMockTokenizer(1), client, 1_000_000)

		_, err := p.AnalyzeLogs(context.Background(), "c", threeLogs())
		require.Error(t, err)
		assert.True(t, errors.Is(err, llm.ErrIncompleteAnswer))
	})

	t.Run("synthesis", func(t *testing.T) {
		client := newRecordingClient()
		client.analyzeErr = incomplete
		p, _ := newForcedChunkedPipeline(t, NewMockTokenizer(1), client)

		_, err := p.AnalyzeLogs(context.Background(), "c", threeLogs())
		require.Error(t, err)
		assert.NotEmpty(t, client.summarizePrompts, "must reach the synthesis step")
		assert.True(t, errors.Is(err, llm.ErrIncompleteAnswer))
	})
}
