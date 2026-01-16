package chunking

import (
	"fmt"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

// TokenizerInterface defines the interface for token counting
type TokenizerInterface interface {
	CountTokens(text string) int
	EstimateSystemPromptTokens(systemPrompt string) int
	EstimateUserPromptTokens(userPrompt string) int
	WillFitInContext(content string, maxTokens int) bool
}

// Tokenizer wraps tiktoken for counting tokens
type Tokenizer struct {
	encoding *tiktoken.Tiktoken
}

// NewTokenizer creates a new tokenizer for the specified model
func NewTokenizer(model string) (*Tokenizer, error) {
	// Get encoding for model
	encoding, err := tiktoken.EncodingForModel(model)
	if err != nil {
		// Fallback to cl100k_base (used by gpt-4, gpt-3.5-turbo)
		encoding, err = tiktoken.GetEncoding("cl100k_base")
		if err != nil {
			return nil, fmt.Errorf("failed to get tokenizer encoding for model %s (fallback to cl100k_base also failed): %w", model, err)
		}
	}

	return &Tokenizer{encoding: encoding}, nil
}

// CountTokens counts the number of tokens in a text
func (t *Tokenizer) CountTokens(text string) int {
	tokens := t.encoding.Encode(text, nil, nil)
	return len(tokens)
}

// EstimateSystemPromptTokens estimates tokens for system prompt
func (t *Tokenizer) EstimateSystemPromptTokens(systemPrompt string) int {
	// System messages have slight overhead
	return t.CountTokens(systemPrompt) + 4
}

// EstimateUserPromptTokens estimates tokens for user prompt
func (t *Tokenizer) EstimateUserPromptTokens(userPrompt string) int {
	// User messages have slight overhead
	return t.CountTokens(userPrompt) + 4
}

// WillFitInContext checks if content fits within token budget
func (t *Tokenizer) WillFitInContext(content string, maxTokens int) bool {
	return t.CountTokens(content) <= maxTokens
}
