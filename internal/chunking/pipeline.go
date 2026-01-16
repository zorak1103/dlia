// Package chunking implements the log chunking and processing pipeline.
package chunking

import (
	"context"
	"fmt"

	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/llm"
	"github.com/zorak1103/dlia/internal/prompts"
)

const (
	// ResponseReserveTokens ensures the model has adequate space for complete responses
	// while processing log analysis requests. Insufficient reserve may cause truncated outputs.
	ResponseReserveTokens = 4000

	// SystemPromptReserveTokens accounts for the system prompt overhead in token calculations.
	// This estimate is based on typical prompt templates and may need adjustment for custom prompts.
	SystemPromptReserveTokens = 500

	// ChunkSizeDivisor controls how conservatively we size chunks relative to available tokens.
	// A divisor of 2 means each chunk uses at most 50% of available tokens, leaving headroom
	// for token estimation variance and ensuring model responses aren't truncated.
	ChunkSizeDivisor = 2
)

// Pipeline orchestrates the log processing pipeline
type Pipeline struct {
	tokenizer                  TokenizerInterface
	client                     llm.ClientInterface
	maxTokens                  int
	ignoreDir                  string
	config                     *config.Config
	compiledRegexpsByContainer map[string]*RegexpFilter
	promptLoader               *prompts.PromptLoader
}

// NewPipeline creates a new processing pipeline with default configuration.
// The pipeline handles log deduplication, optional regexp filtering, token counting,
// and LLM-based analysis with automatic chunking for large log batches.
func NewPipeline(model string, maxTokens int, client llm.ClientInterface, promptLoader *prompts.PromptLoader, cfg *config.Config) (*Pipeline, error) {
	return NewPipelineWithConfig(model, maxTokens, client, promptLoader, "", cfg)
}

// NewPipelineWithConfig creates a new processing pipeline with custom ignore directory.
// Use this when you need to specify a non-default location for container-specific ignore patterns.
func NewPipelineWithConfig(model string, maxTokens int, client llm.ClientInterface, promptLoader *prompts.PromptLoader, ignoreDir string, cfg *config.Config) (*Pipeline, error) {
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

	return &Pipeline{
		tokenizer:                  tokenizer,
		client:                     client,
		maxTokens:                  maxTokens,
		ignoreDir:                  ignoreDir,
		config:                     cfg,
		compiledRegexpsByContainer: regexpFilters,
		promptLoader:               promptLoader,
	}, nil
}

// AnalyzeResult contains the analysis result
type AnalyzeResult struct {
	Analysis       string
	TokensUsed     int
	ChunksUsed     int
	Deduplicated   bool
	OriginalCount  int
	ProcessedCount int
	FilterStats    FilterStats
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

	// Step 2: Format logs
	logsText := FormatLogs(processedLogs)

	// Step 3: Load container-specific ignore patterns (error returns empty string, which is valid)
	ignoreInstructions, _ := config.GetIgnoreInstructions(containerName, p.ignoreDir) //nolint:errcheck // Error returns empty string, which is valid

	systemPrompt, err := p.promptLoader.SystemPrompt(ignoreInstructions)
	if err != nil {
		return nil, fmt.Errorf("failed to load system prompt: %w", err)
	}
	userPromptBase, err := p.promptLoader.AnalysisPrompt(containerName, "", len(processedLogs))
	if err != nil {
		return nil, fmt.Errorf("failed to load analysis prompt: %w", err)
	}

	// Calculate token budget: system prompt + base user prompt + actual log content.
	// Available tokens for logs = model limit - response reserve - system overhead.
	// This ensures the model can generate a complete response without truncation.
	systemTokens := p.tokenizer.EstimateSystemPromptTokens(systemPrompt)
	baseUserTokens := p.tokenizer.CountTokens(userPromptBase)
	logsTokens := p.tokenizer.CountTokens(logsText)

	totalTokens := systemTokens + baseUserTokens + logsTokens
	availableTokens := p.maxTokens - ResponseReserveTokens - systemTokens

	// Step 4: Choose analysis strategy based on token budget
	if totalTokens+ResponseReserveTokens <= p.maxTokens {
		analysis, usage, err := p.analyzeDirectly(ctx, containerName, processedLogs, systemPrompt, logsText)
		if err != nil {
			return nil, err
		}
		result.Analysis = analysis
		result.TokensUsed = usage.TotalTokens
		result.ChunksUsed = 1
	} else {
		analysis, tokensUsed, chunksUsed, err := p.analyzeWithChunking(ctx, containerName, processedLogs, systemPrompt, availableTokens)
		if err != nil {
			return nil, err
		}
		result.Analysis = analysis
		result.TokensUsed = tokensUsed
		result.ChunksUsed = chunksUsed
	}

	return result, nil
}

func (p *Pipeline) analyzeDirectly(ctx context.Context, containerName string, logs []docker.LogEntry, systemPrompt, logsText string) (string, *llm.TokenUsage, error) {
	userPrompt, err := p.promptLoader.AnalysisPrompt(containerName, logsText, len(logs))
	if err != nil {
		return "", nil, fmt.Errorf("failed to load analysis prompt: %w", err)
	}
	return p.client.Analyze(ctx, containerName, systemPrompt, userPrompt)
}

func (p *Pipeline) analyzeWithChunking(ctx context.Context, containerName string, logs []docker.LogEntry, systemPrompt string, availableTokens int) (analysis string, totalTokens, chunksUsed int, err error) {
	chunks := ChunkLogs(logs, availableTokens/ChunkSizeDivisor, p.tokenizer)

	if len(chunks) == 0 {
		return "No logs could be processed within token limits", 0, 0, nil
	}

	summaries := make([]string, len(chunks))
	chunksUsed = len(chunks)
	totalTokens = 0

	for i, chunk := range chunks {
		chunkText := FormatChunk(chunk)
		chunkPrompt, promptErr := p.promptLoader.ChunkSummaryPrompt(containerName, i+1, len(chunks), chunkText)
		if promptErr != nil {
			return "", totalTokens, chunksUsed, fmt.Errorf("failed to load chunk summary prompt: %w", promptErr)
		}

		summary, summarizeErr := p.client.SummarizeChunk(ctx, containerName, systemPrompt, chunkPrompt)
		if summarizeErr != nil {
			return "", totalTokens, chunksUsed, fmt.Errorf("failed to summarize chunk %d/%d (length: %d logs, %d tokens) for container %s: %w",
				i+1, len(chunks), len(chunk.Logs), chunk.TokenCount, containerName, summarizeErr)
		}

		summaries[i] = summary
		// Estimate token usage since SummarizeChunk doesn't return usage metrics
		totalTokens += p.tokenizer.CountTokens(chunkText) + p.tokenizer.CountTokens(summary)
	}

	synthesisPrompt, synthesisErr := p.promptLoader.SynthesisPrompt(containerName, summaries)
	if synthesisErr != nil {
		return "", totalTokens, chunksUsed, fmt.Errorf("failed to load synthesis prompt: %w", synthesisErr)
	}
	finalAnalysis, usage, analyzeErr := p.client.Analyze(ctx, containerName, systemPrompt, synthesisPrompt)
	if analyzeErr != nil {
		return "", totalTokens, chunksUsed, fmt.Errorf("failed to synthesize %d chunk summaries for container %s: %w",
			len(summaries), containerName, analyzeErr)
	}

	totalTokens += usage.TotalTokens

	return finalAnalysis, totalTokens, chunksUsed, nil
}
