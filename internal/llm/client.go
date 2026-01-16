// Package llm provides a client for interacting with LLM APIs.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	//	client := llm.NewClient("https://api.openai.com/v1", "sk-...", "gpt-4")
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
	//	client := llm.NewClient("https://api.openai.com/v1", "sk-...", "gpt-4")
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
	//	client := llm.NewClient("https://api.openai.com/v1", "sk-...", "gpt-4")
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

// clientImpl represents an LLM API client implementation
type clientImpl struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
	logger     *llmlogger.Logger
}

// Compile-time verification that clientImpl implements Client
var _ Client = (*clientImpl)(nil)

// NewClient connects to an OpenAI-compatible API at baseURL using the specified model.
func NewClient(baseURL, apiKey, model string) Client {
	return &clientImpl{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // 2 minutes for long responses
		},
	}
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
func (c *clientImpl) executeWithRetry(httpReq *http.Request, maxRetries int) ([]byte, int, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
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
			lastErr = result.err
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}

		// Non-retryable error (4xx)
		return result.body, result.statusCode, nil
	}
	return nil, 0, lastErr
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
	req := ChatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	body, err := json.Marshal(req)
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
	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	req := ChatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: 0.3,
		MaxTokens:   4000,
	}

	resp, err := c.ChatCompletion(ctx, req.Messages, req.Temperature, req.MaxTokens)
	if err != nil {
		return "", nil, err
	}

	if len(resp.Choices) == 0 {
		return "", nil, fmt.Errorf("no choices in response for container %s from model %s", containerName, c.model)
	}

	// Log the interaction if logger is configured
	if c.logger != nil {
		if logErr := c.logger.LogInteraction(containerName, userPrompt, req, resp); logErr != nil {
			// Log error but don't fail the analysis
			fmt.Printf("Warning: failed to log LLM interaction: %v\n", logErr)
		}
	}

	return resp.Choices[0].Message.Content, &resp.Usage, nil
}

func (c *clientImpl) SummarizeChunk(ctx context.Context, containerName, systemPrompt, chunkPrompt string) (string, error) {
	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: chunkPrompt},
	}

	req := ChatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: 0.3,
		MaxTokens:   2000,
	}

	resp, err := c.ChatCompletion(ctx, req.Messages, req.Temperature, req.MaxTokens)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response for container %s from model %s", containerName, c.model)
	}

	// Log the interaction if logger is configured
	if c.logger != nil {
		if logErr := c.logger.LogInteraction(containerName, chunkPrompt, req, resp); logErr != nil {
			// Log error but don't fail the summarization
			fmt.Printf("Warning: failed to log LLM interaction: %v\n", logErr)
		}
	}

	return resp.Choices[0].Message.Content, nil
}
