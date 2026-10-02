// Package llm provides a client for interacting with LLM APIs.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zorak1103/dlia/internal/llmlogger"
)

// Client defines the interface for LLM client operations.
// Implementations provide chat completion, analysis, and summarization capabilities.
type Client interface {
	// ChatCompletion sends a chat completion request to the LLM API.
	// Returns the completion response or error if request fails.
	//
	// Example usage:
	//
	//	client := llm.NewClient(llm.Options{BaseURL: "https://api.openai.com/v1", APIKey: "sk-...", Model: "gpt-4"})
	//	messages := []llm.ChatMessage{
	//	    {Role: "system", Content: "You are a helpful assistant analyzing Docker logs."},
	//	    {Role: "user", Content: "Analyze these error logs: [ERROR] Connection failed"},
	//	}
	//	resp, err := client.ChatCompletion(ctx, messages, 0.3, 2000)
	//	if err != nil {
	//	    log.Fatal(err)
	//	}
	//	fmt.Printf("Analysis: %s\nTokens used: %d\n",
	//	    resp.Choices[0].Message.Content, resp.Usage.TotalTokens)
	ChatCompletion(ctx context.Context, messages []ChatMessage, temperature float64, maxTokens int) (*ChatResponse, error)

	// Analyze performs semantic analysis on container logs.
	// Returns analysis result, token usage statistics, or error if analysis fails.
	//
	// Example usage:
	//
	//	client := llm.NewClient(llm.Options{BaseURL: "https://api.openai.com/v1", APIKey: "sk-...", Model: "gpt-4"})
	//	systemPrompt := "Analyze Docker container logs for errors and patterns."
	//	userPrompt := "Container: nginx-web\nLogs:\n[ERROR] 502 Bad Gateway\n[WARN] Upstream timeout"
	//	analysis, usage, err := client.Analyze(ctx, "nginx-web", systemPrompt, userPrompt)
	//	if err != nil {
	//	    log.Fatal(err)
	//	}
	//	fmt.Printf("Analysis: %s\nTokens: %d input, %d output\n",
	//	    analysis, usage.PromptTokens, usage.CompletionTokens)
	Analyze(ctx context.Context, containerName, systemPrompt, userPrompt string) (string, *TokenUsage, error)

	// SummarizeChunk generates a summary of a log chunk for incremental processing.
	// Used for chunked analysis of large log volumes.
	//
	// Example usage:
	//
	//	client := llm.NewClient(llm.Options{BaseURL: "https://api.openai.com/v1", APIKey: "sk-...", Model: "gpt-4"})
	//	systemPrompt := "Summarize Docker log chunks concisely, preserving critical errors."
	//	chunkPrompt := "Chunk 1/5:\n[INFO] Service started\n[ERROR] Database connection timeout"
	//	summary, err := client.SummarizeChunk(ctx, "postgres-db", systemPrompt, chunkPrompt)
	//	if err != nil {
	//	    log.Fatal(err)
	//	}
	//	fmt.Printf("Summary: %s\n", summary)
	SummarizeChunk(ctx context.Context, containerName, systemPrompt, chunkPrompt string) (string, error)

	// SetLogger configures the LLM logger for capturing request/response pairs.
	SetLogger(logger *llmlogger.Logger)
}

// Default answer limits applied when Options leaves them unset.
const (
	defaultMaxAnswerTokens       = 4000
	defaultMaxChunkSummaryTokens = 2000
	analysisTemperature          = 0.3
)

// Options configures an LLM client.
type Options struct {
	BaseURL, APIKey, Model string
	// MaxAnswerTokens caps Analyze answers (default 4000).
	MaxAnswerTokens int
	// MaxChunkSummaryTokens caps SummarizeChunk answers (default 2000).
	MaxChunkSummaryTokens int
	// ExtraBody holds additional top-level request fields merged into every request.
	ExtraBody map[string]any
}

// clientImpl represents an LLM API client implementation
type clientImpl struct {
	baseURL               string
	apiKey                string
	model                 string
	maxAnswerTokens       int
	maxChunkSummaryTokens int
	extraBody             map[string]any
	httpClient            *http.Client
	logger                *llmlogger.Logger
}

// Compile-time verification that clientImpl implements Client
var _ Client = (*clientImpl)(nil)

// NewClient connects to an OpenAI-compatible API described by opts.
// Non-positive answer limits fall back to their defaults.
func NewClient(opts Options) Client {
	if opts.MaxAnswerTokens <= 0 {
		opts.MaxAnswerTokens = defaultMaxAnswerTokens
	}
	if opts.MaxChunkSummaryTokens <= 0 {
		opts.MaxChunkSummaryTokens = defaultMaxChunkSummaryTokens
	}
	return &clientImpl{
		baseURL:               opts.BaseURL,
		apiKey:                opts.APIKey,
		model:                 opts.Model,
		maxAnswerTokens:       opts.MaxAnswerTokens,
		maxChunkSummaryTokens: opts.MaxChunkSummaryTokens,
		extraBody:             opts.ExtraBody,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // 2 minutes for long responses
		},
	}
}

// buildBody assembles the request body: the standard fields plus ExtraBody.
// temperature 0 and maxTokens <= 0 are omitted.
func (c *clientImpl) buildBody(messages []ChatMessage, temperature float64, maxTokens int) map[string]any {
	body := map[string]any{
		"model":    c.model,
		"messages": messages,
	}
	if temperature != 0 {
		body["temperature"] = temperature
	}
	if maxTokens > 0 {
		body["max_tokens"] = maxTokens
	}
	for k, v := range c.extraBody {
		body[k] = v
	}
	return body
}

func (c *clientImpl) SetLogger(logger *llmlogger.Logger) {
	c.logger = logger
}

// retryResult holds the result of a single retry attempt.
type retryResult struct {
	body       []byte
	statusCode int
	err        error
}

// executeWithRetry performs an HTTP request with retry logic for transient errors.
func (c *clientImpl) executeWithRetry(httpReq *http.Request, maxRetries int) (body []byte, statusCode int, err error) {
	ctx := httpReq.Context()
	for attempt := range maxRetries {
		result := c.executeRequest(httpReq)
		if result.err == nil && result.statusCode == http.StatusOK {
			return result.body, result.statusCode, nil
		}

		// Don't retry on last attempt
		if attempt >= maxRetries-1 {
			if result.err != nil {
				return nil, 0, fmt.Errorf("failed after %d attempts: %w", maxRetries, result.err)
			}
			return result.body, result.statusCode, nil
		}

		// Retry on network errors or 5xx status codes
		if result.err != nil || result.statusCode >= 500 {
			select {
			case <-ctx.Done():
				return nil, 0, fmt.Errorf("retry backoff canceled: %w", ctx.Err())
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
			continue
		}

		// Non-retryable error (4xx)
		return result.body, result.statusCode, nil
	}
	return nil, 0, fmt.Errorf("no attempts made: maxRetries must be positive, got %d", maxRetries)
}

// executeRequest performs a single HTTP request and returns the result.
func (c *clientImpl) executeRequest(httpReq *http.Request) retryResult {
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return retryResult{err: err}
	}

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close() // Close response body; error not actionable as body is already read
	if err != nil {
		return retryResult{err: err}
	}

	return retryResult{body: body, statusCode: resp.StatusCode}
}

func (c *clientImpl) ChatCompletion(ctx context.Context, messages []ChatMessage, temperature float64, maxTokens int) (*ChatResponse, error) {
	body, err := json.Marshal(c.buildBody(messages, temperature, maxTokens))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chat completion request for model %s: %w", c.model, err)
	}

	endpoint := c.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request to %s for model %s: %w", endpoint, c.model, err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	respBody, statusCode, err := c.executeWithRetry(httpReq, 3)
	if err != nil {
		return nil, fmt.Errorf("request to %s for model %s failed: %w", endpoint, c.model, err)
	}

	if statusCode != http.StatusOK {
		var apiResp ChatResponse
		if unmarshalErr := json.Unmarshal(respBody, &apiResp); unmarshalErr == nil && apiResp.Error != nil {
			return nil, apiResp.Error
		}
		return nil, fmt.Errorf("API %s returned status %d for model %s: %s", endpoint, statusCode, c.model, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse response from %s for model %s: %w", endpoint, c.model, err)
	}

	if chatResp.Error != nil {
		return nil, chatResp.Error
	}

	return &chatResp, nil
}

func (c *clientImpl) Analyze(ctx context.Context, containerName, systemPrompt, userPrompt string) (string, *TokenUsage, error) {
	return c.complete(ctx, containerName, systemPrompt, userPrompt, c.maxAnswerTokens, "llm.max_answer_tokens")
}

func (c *clientImpl) SummarizeChunk(ctx context.Context, containerName, systemPrompt, chunkPrompt string) (string, error) {
	content, _, err := c.complete(ctx, containerName, systemPrompt, chunkPrompt, c.maxChunkSummaryTokens, "llm.max_chunk_summary_tokens")
	return content, err
}

// complete runs one system+user chat completion, logs the interaction (also
// for incomplete answers) and rejects truncated or empty answers.
func (c *clientImpl) complete(ctx context.Context, container, system, user string, limit int, limitKey string) (string, *TokenUsage, error) {
	messages := []ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}

	resp, err := c.ChatCompletion(ctx, messages, analysisTemperature, limit)
	if err != nil {
		return "", nil, err
	}

	if len(resp.Choices) == 0 {
		return "", nil, fmt.Errorf("no choices in response for container %s from model %s", container, c.model)
	}

	if c.logger != nil {
		if logErr := c.logger.LogInteraction(container, user, c.buildBody(messages, analysisTemperature, limit), resp); logErr != nil {
			// Log error but don't fail the call
			fmt.Printf("Warning: failed to log LLM interaction: %v\n", logErr)
		}
	}

	choice := resp.Choices[0]
	reason := ""
	switch {
	case choice.FinishReason == "length":
		reason = "finish_reason=length"
	case strings.TrimSpace(choice.Message.Content) == "":
		reason = "empty answer"
	}
	if reason != "" {
		return "", nil, &IncompleteAnswerError{Container: container, Reason: reason, Limit: limit, LimitKey: limitKey}
	}

	return choice.Message.Content, &resp.Usage, nil
}
