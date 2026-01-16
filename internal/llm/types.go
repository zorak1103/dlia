package llm

import "context"

// ClientInterface defines the interface for LLM client operations
type ClientInterface interface {
	Analyze(ctx context.Context, containerName, systemPrompt, userPrompt string) (string, *TokenUsage, error)
	SummarizeChunk(ctx context.Context, containerName, systemPrompt, chunkPrompt string) (string, error)
	ChatCompletion(ctx context.Context, messages []ChatMessage, temperature float64, maxTokens int) (*ChatResponse, error)
}

// ChatMessage represents a single message in a conversation
type ChatMessage struct {
	Role    string `json:"role"`    // system, user, assistant
	Content string `json:"content"` // message content
}

// ChatRequest represents a request to the chat completion API
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	TopP        float64       `json:"top_p,omitempty"`
}

// ChatResponse represents the API response
type ChatResponse struct {
	ID      string     `json:"id"`
	Object  string     `json:"object"`
	Created int64      `json:"created"`
	Model   string     `json:"model"`
	Choices []Choice   `json:"choices"`
	Usage   TokenUsage `json:"usage"`
	Error   *APIError  `json:"error,omitempty"`
}

// Choice represents a single completion choice
type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// TokenUsage represents token usage statistics
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// APIError represents an error from the API
type APIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// Error implements the error interface
func (e *APIError) Error() string {
	if e.Code != "" {
		return e.Code + ": " + e.Message
	}
	return e.Message
}
