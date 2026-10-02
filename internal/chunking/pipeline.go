// Package chunking implements the log chunking and processing pipeline.
package chunking

import (
	"context"
	"fmt"

	"github.com/zorak1103/dlia/internal/anonymize"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/llm"
	"github.com/zorak1103/dlia/internal/prompts"
	"github.com/zorak1103/dlia/internal/severity"
)

const (
	// DefaultResponseReserveTokens is the response reserve used when no llm.max_answer_tokens
	// is configured. The pipeline reserves llm.max_answer_tokens so the model has adequate
	// space for complete responses; insufficient reserve may cause truncated outputs.
	DefaultResponseReserveTokens = 4000

	// SystemPromptReserveTokens accounts for the system prompt overhead in token calculations.
	// This estimate is based on typical prompt templates and may need adjustment for custom prompts.
	// It also has to cover the untrusted-data rule appended to every system prompt (~170 tokens),
	// which custom prompts do not include themselves.
	SystemPromptReserveTokens = 500

	// ChunkSizeDivisor controls how conservatively we size chunks relative to available tokens.
	// A divisor of 2 means each chunk uses at most 50% of available tokens, leaving headroom
	// for token estimation variance and ensuring model responses aren't truncated.
	ChunkSizeDivisor = 2

	// EstimateBudgetPercent is the share of the context window used as token budget
	// when the tokenizer is only an estimate (unknown model, cl100k_base fallback).
	EstimateBudgetPercent = 80
)

// Pipeline orchestrates the log processing pipeline
type Pipeline struct {
	tokenizer                  TokenizerInterface
	client                     AnalysisClient
	maxTokens                  int
	ignoreDir                  string
	config                     *config.Config
	compiledRegexpsByContainer map[string]*RegexpFilter
	promptLoader               *prompts.PromptLoader
	tokenCountIsEstimate       bool
	maxChunks                  int
	responseReserve            int
}

// TokenCountIsEstimate reports whether token counts are estimates because the
// model is unknown to the tokenizer.
func (p *Pipeline) TokenCountIsEstimate() bool {
	return p.tokenCountIsEstimate
}

// AnalysisClient is the subset of llm.Client the pipeline depends on for
// analyzing and summarizing logs. Declared here, at the consumer, rather
// than in the llm package, so the dependency stays as narrow as what
// Pipeline actually calls.
type AnalysisClient interface {
	Analyze(ctx context.Context, containerName, systemPrompt, userPrompt string) (string, *llm.TokenUsage, error)
	SummarizeChunk(ctx context.Context, containerName, systemPrompt, chunkPrompt string) (string, error)
}

// NewPipeline creates a new processing pipeline with default configuration.
// The pipeline handles log deduplication, optional regexp filtering, token counting,
// and LLM-based analysis with automatic chunking for large log batches.
func NewPipeline(model string, contextWindow int, client AnalysisClient, promptLoader *prompts.PromptLoader, cfg *config.Config) (*Pipeline, error) {
	return NewPipelineWithConfig(model, contextWindow, client, promptLoader, "", cfg)
}

// NewPipelineWithConfig creates a new processing pipeline with custom ignore directory.
// Use this when you need to specify a non-default location for container-specific ignore patterns.
func NewPipelineWithConfig(model string, contextWindow int, client AnalysisClient, promptLoader *prompts.PromptLoader, ignoreDir string, cfg *config.Config) (*Pipeline, error) {
	tokenizer, err := NewTokenizer(model)
	if err != nil {
		return nil, fmt.Errorf("failed to create tokenizer for model %s: %w", model, err)
	}

	if ignoreDir == "" {
		ignoreDir = config.DefaultIgnoreDir
	}

	// Pre-allocate map capacity: typical deployments use 3-5 filter patterns
	regexpFilters := make(map[string]*RegexpFilter, 5)
	if cfg != nil {
		for containerName, filterCfg := range cfg.RegexpFilters {
			if filterCfg.Enabled && len(filterCfg.Patterns) > 0 {
				filter, err := NewRegexpFilter(filterCfg.Patterns)
				if err != nil {
					return nil, fmt.Errorf("failed to create regexp filter for container %s: %w", containerName, err)
				}
				regexpFilters[containerName] = filter
			}
		}
	}

	maxTokens := contextWindow
	if tokenizer.IsEstimate() {
		maxTokens = contextWindow * EstimateBudgetPercent / 100
	}

	maxChunks := 0
	responseReserve := DefaultResponseReserveTokens
	if cfg != nil {
		maxChunks = cfg.LLM.MaxChunksPerContainer
		if cfg.LLM.MaxAnswerTokens > 0 {
			responseReserve = cfg.LLM.MaxAnswerTokens
		}
	}

	return &Pipeline{
		tokenizer:                  tokenizer,
		client:                     client,
		maxTokens:                  maxTokens,
		tokenCountIsEstimate:       tokenizer.IsEstimate(),
		ignoreDir:                  ignoreDir,
		config:                     cfg,
		compiledRegexpsByContainer: regexpFilters,
		promptLoader:               promptLoader,
		maxChunks:                  maxChunks,
		responseReserve:            responseReserve,
	}, nil
}

// AnalyzeResult contains the analysis result
type AnalyzeResult struct {
	Analysis       string
	Severity       severity.Level
	TokensUsed     int
	ChunksUsed     int
	Deduplicated   bool
	OriginalCount  int
	ProcessedCount int
	FilterStats    FilterStats
	// CoverageNotes lists parts of the logs that were not analyzed (rendered in the report).
	CoverageNotes []string
}

// applyRegexpFilter applies container-specific regexp filtering to logs.
// Returns filtered logs and filter statistics. Logs that match any pattern are excluded.
func (p *Pipeline) applyRegexpFilter(containerName string, logs []docker.LogEntry) ([]docker.LogEntry, FilterStats) {
	filter, exists := p.compiledRegexpsByContainer[containerName]
	if !exists {
		return logs, FilterStats{
			LinesTotal:    len(logs),
			LinesFiltered: 0,
			LinesKept:     len(logs),
		}
	}

	stats := FilterStats{
		LinesTotal: len(logs),
	}

	filteredLogs := make([]docker.LogEntry, 0, len(logs))
	for _, entry := range logs {
		if filter.MatchesAny(entry.Message) {
			stats.LinesFiltered++
		} else {
			filteredLogs = append(filteredLogs, entry)
		}
	}
	stats.LinesKept = len(filteredLogs)

	return filteredLogs, stats
}

// anonymizeLogs masks IPs and secrets according to the privacy config using one
// session per call, so placeholders stay consistent across all chunks. It returns
// a masked copy and never mutates the input.
func (p *Pipeline) anonymizeLogs(logs []docker.LogEntry) []docker.LogEntry {
	if p.config == nil || (!p.config.Privacy.AnonymizeIPs && !p.config.Privacy.AnonymizeSecrets) {
		return logs
	}

	session := anonymize.NewSession(anonymize.Options{
		IPs:     p.config.Privacy.AnonymizeIPs,
		Secrets: p.config.Privacy.AnonymizeSecrets,
	})
	masked := make([]docker.LogEntry, len(logs))
	for i, entry := range logs {
		entry.Message = session.Apply(entry.Message)
		masked[i] = entry
	}
	return masked
}

// AnalyzeLogs processes container logs through the complete pipeline: deduplication,
// optional regexp filtering, and LLM-based analysis. Automatically handles chunking
// and recursive summarization when logs exceed the model's context window.
func (p *Pipeline) AnalyzeLogs(ctx context.Context, containerName string, logs []docker.LogEntry) (*AnalyzeResult, error) {
	if len(logs) == 0 {
		return &AnalyzeResult{
			Analysis: "No logs to analyze",
		}, nil
	}

	result := &AnalyzeResult{
		OriginalCount: len(logs),
	}

	// Step 1: Deduplicate
	dedupLogs := Deduplicate(logs)
	if len(dedupLogs) < len(logs) {
		result.Deduplicated = true
		result.ProcessedCount = len(dedupLogs)
	} else {
		result.ProcessedCount = len(logs)
	}

	// Step 1.5: Apply regexp filtering if configured for this container
	processedLogs, filterStats := p.applyRegexpFilter(containerName, dedupLogs)
	result.FilterStats = filterStats
	result.ProcessedCount = len(processedLogs)

	// Step 1.6: Mask IPs and secrets before anything leaves the process
	processedLogs = p.anonymizeLogs(processedLogs)

	// Step 2: Format logs
	logsText := FormatLogs(processedLogs)

	// Step 3: Load container-specific ignore patterns (error returns empty string, which is valid)
	ignoreInstructions, _ := config.GetIgnoreInstructions(containerName, p.ignoreDir) //nolint:errcheck // Error returns empty string, which is valid

	// Prompt with empty logs: only used to size the token budget. The marker has a fixed
	// length, so this estimate matches the real call within a few tokens (the random hex
	// tokenizes variably).
	base, err := p.promptLoader.AnalysisMessages(containerName, ignoreInstructions, "", len(processedLogs))
	if err != nil {
		return nil, fmt.Errorf("failed to load analysis prompt: %w", err)
	}

	// Step 4: Choose analysis strategy and parse severity from the final answer.
	return p.analyzeByBudget(ctx, containerName, processedLogs, base, ignoreInstructions, logsText, result)
}

// analyzeByBudget calculates the token budget, picks direct or chunked analysis,
// then strips the SEVERITY line from the final answer and stores it in result.
func (p *Pipeline) analyzeByBudget(ctx context.Context, containerName string, processedLogs []docker.LogEntry, base prompts.Messages, ignoreInstructions, logsText string, result *AnalyzeResult) (*AnalyzeResult, error) {
	// Calculate token budget: system prompt + base user prompt (with severity instruction)
	// + actual log content. Available tokens for logs = model limit - response reserve - system overhead.
	systemTokens := p.tokenizer.EstimateSystemPromptTokens(base.System)
	baseUserTokens := p.tokenizer.CountTokens(base.User)
	logsTokens := p.tokenizer.CountTokens(logsText)

	totalTokens := systemTokens + baseUserTokens + logsTokens
	availableTokens := p.maxTokens - p.responseReserve - systemTokens

	var err error
	if totalTokens+p.responseReserve <= p.maxTokens {
		var usage *llm.TokenUsage
		result.Analysis, usage, err = p.analyzeDirectly(ctx, containerName, ignoreInstructions, processedLogs, logsText)
		if err != nil {
			return nil, err
		}
		result.TokensUsed = usage.TotalTokens
		result.ChunksUsed = 1
	} else {
		result.Analysis, result.TokensUsed, result.ChunksUsed, result.CoverageNotes, err = p.analyzeWithChunking(ctx, containerName, ignoreInstructions, processedLogs, availableTokens)
		if err != nil {
			return nil, err
		}
	}

	if result.ChunksUsed > 0 {
		result.Severity, result.Analysis = severity.Parse(result.Analysis)
	}

	return result, nil
}

func (p *Pipeline) analyzeDirectly(ctx context.Context, containerName, ignoreInstructions string, logs []docker.LogEntry, logsText string) (string, *llm.TokenUsage, error) {
	m, err := p.promptLoader.AnalysisMessages(containerName, ignoreInstructions, logsText, len(logs))
	if err != nil {
		return "", nil, fmt.Errorf("failed to load analysis prompt: %w", err)
	}
	return p.client.Analyze(ctx, containerName, m.System, m.User)
}

// limitChunks keeps only the newest maxChunks chunks (renumbered) and returns a
// coverage note describing the skipped ones. maxChunks <= 0 means unlimited.
func limitChunks(chunks []Chunk, maxChunks int) (kept []Chunk, note string) {
	if maxChunks <= 0 || len(chunks) <= maxChunks {
		return chunks, ""
	}

	skipped := chunks[:len(chunks)-maxChunks]
	kept = append([]Chunk(nil), chunks[len(chunks)-maxChunks:]...)
	for i := range kept {
		kept[i].Index = i
		kept[i].Total = len(kept)
	}

	lines := 0
	for _, c := range skipped {
		lines += len(c.Logs)
	}
	period := ""
	first := skipped[0].Logs[0].Timestamp
	lastLogs := skipped[len(skipped)-1].Logs
	last := lastLogs[len(lastLogs)-1].Timestamp
	if first != "" && last != "" {
		period = fmt.Sprintf(", %s \u2013 %s", first, last)
	}

	note = fmt.Sprintf("Skipped %d of %d chunks (%d log entries%s) because of llm.max_chunks_per_container=%d",
		len(skipped), len(chunks), lines, period, maxChunks)
	return kept, note
}

func (p *Pipeline) analyzeWithChunking(ctx context.Context, containerName, ignoreInstructions string, logs []docker.LogEntry, availableTokens int) (analysis string, totalTokens, chunksUsed int, notes []string, err error) {
	chunks := ChunkLogs(logs, availableTokens/ChunkSizeDivisor, p.tokenizer)

	if len(chunks) == 0 {
		return "No logs could be processed within token limits", 0, 0, nil, nil
	}

	chunks, note := limitChunks(chunks, p.maxChunks)
	if note != "" {
		notes = append(notes, note)
	}

	summaries := make([]string, len(chunks))
	chunksUsed = len(chunks)
	totalTokens = 0

	for i, chunk := range chunks {
		chunkText := FormatChunk(chunk)
		m, promptErr := p.promptLoader.ChunkMessages(containerName, ignoreInstructions, i+1, len(chunks), chunkText)
		if promptErr != nil {
			return "", totalTokens, chunksUsed, notes, fmt.Errorf("failed to load chunk summary prompt: %w", promptErr)
		}

		summary, summarizeErr := p.client.SummarizeChunk(ctx, containerName, m.System, m.User)
		if summarizeErr != nil {
			return "", totalTokens, chunksUsed, notes, fmt.Errorf("failed to summarize chunk %d/%d (length: %d logs, %d tokens) for container %s: %w",
				i+1, len(chunks), len(chunk.Logs), chunk.TokenCount, containerName, summarizeErr)
		}

		summaries[i] = summary
		// Estimate token usage since SummarizeChunk doesn't return usage metrics
		totalTokens += p.tokenizer.CountTokens(chunkText) + p.tokenizer.CountTokens(summary)
	}

	synth, synthesisErr := p.promptLoader.SynthesisMessages(containerName, ignoreInstructions, summaries)
	if synthesisErr != nil {
		return "", totalTokens, chunksUsed, notes, fmt.Errorf("failed to load synthesis prompt: %w", synthesisErr)
	}
	finalAnalysis, usage, analyzeErr := p.client.Analyze(ctx, containerName, synth.System, synth.User)
	if analyzeErr != nil {
		return "", totalTokens, chunksUsed, notes, fmt.Errorf("failed to synthesize %d chunk summaries for container %s: %w",
			len(summaries), containerName, analyzeErr)
	}

	totalTokens += usage.TotalTokens

	return finalAnalysis, totalTokens, chunksUsed, notes, nil
}
