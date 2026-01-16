package chunking

import (
	"testing"
)

func TestNewTokenizer(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		expectErr bool
	}{
		{
			name:      "valid GPT-4 model",
			model:     "gpt-4",
			expectErr: false,
		},
		{
			name:      "valid GPT-3.5-turbo model",
			model:     "gpt-3.5-turbo",
			expectErr: false,
		},
		{
			name:      "unknown model falls back to cl100k_base",
			model:     "unknown-model",
			expectErr: false,
		},
		{
			name:      "empty model falls back to cl100k_base",
			model:     "",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenizer, err := NewTokenizer(tt.model)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tokenizer == nil {
					t.Error("expected tokenizer but got nil")
				}
				if tokenizer != nil && tokenizer.encoding == nil {
					t.Error("expected encoding but got nil")
				}
			}
		})
	}
}

func TestCountTokens(t *testing.T) {
	tokenizer, err := NewTokenizer("gpt-4")
	if err != nil {
		t.Fatalf("failed to create tokenizer: %v", err)
	}

	tests := []struct {
		name     string
		text     string
		minCount int // Minimum expected token count
	}{
		{
			name:     "empty string",
			text:     "",
			minCount: 0,
		},
		{
			name:     "single word",
			text:     "hello",
			minCount: 1,
		},
		{
			name:     "simple sentence",
			text:     "Hello, world!",
			minCount: 1,
		},
		{
			name:     "multiple sentences",
			text:     "This is a test. This is another test.",
			minCount: 5,
		},
		{
			name:     "with newlines",
			text:     "Line 1\nLine 2\nLine 3",
			minCount: 3,
		},
		{
			name:     "with special characters",
			text:     "Hello @user! How are you? #golang",
			minCount: 5,
		},
		{
			name:     "long text",
			text:     "The quick brown fox jumps over the lazy dog. This is a longer piece of text to test token counting with multiple sentences and various words.",
			minCount: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := tokenizer.CountTokens(tt.text)
			if count < tt.minCount {
				t.Errorf("expected at least %d tokens, got %d", tt.minCount, count)
			}
			// Verify consistency - calling twice should give same result
			count2 := tokenizer.CountTokens(tt.text)
			if count != count2 {
				t.Errorf("inconsistent token count: first call=%d, second call=%d", count, count2)
			}
		})
	}
}

func TestEstimateSystemPromptTokens(t *testing.T) {
	tokenizer, err := NewTokenizer("gpt-4")
	if err != nil {
		t.Fatalf("failed to create tokenizer: %v", err)
	}

	tests := []struct {
		name         string
		systemPrompt string
	}{
		{
			name:         "empty system prompt",
			systemPrompt: "",
		},
		{
			name:         "simple system prompt",
			systemPrompt: "You are a helpful assistant.",
		},
		{
			name:         "detailed system prompt",
			systemPrompt: "You are a helpful assistant that analyzes Docker logs and provides insights. Be concise and clear.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseCount := tokenizer.CountTokens(tt.systemPrompt)
			estimatedCount := tokenizer.EstimateSystemPromptTokens(tt.systemPrompt)

			// Should add 4 tokens for overhead
			expectedCount := baseCount + 4
			if estimatedCount != expectedCount {
				t.Errorf("expected %d tokens (base %d + 4 overhead), got %d", expectedCount, baseCount, estimatedCount)
			}
		})
	}
}

func TestEstimateUserPromptTokens(t *testing.T) {
	tokenizer, err := NewTokenizer("gpt-4")
	if err != nil {
		t.Fatalf("failed to create tokenizer: %v", err)
	}

	tests := []struct {
		name       string
		userPrompt string
	}{
		{
			name:       "empty user prompt",
			userPrompt: "",
		},
		{
			name:       "simple user prompt",
			userPrompt: "Analyze these logs.",
		},
		{
			name:       "detailed user prompt",
			userPrompt: "Please analyze the following Docker container logs and provide a summary of any errors or warnings.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseCount := tokenizer.CountTokens(tt.userPrompt)
			estimatedCount := tokenizer.EstimateUserPromptTokens(tt.userPrompt)

			// Should add 4 tokens for overhead
			expectedCount := baseCount + 4
			if estimatedCount != expectedCount {
				t.Errorf("expected %d tokens (base %d + 4 overhead), got %d", expectedCount, baseCount, estimatedCount)
			}
		})
	}
}

func TestWillFitInContext(t *testing.T) {
	tokenizer, err := NewTokenizer("gpt-4")
	if err != nil {
		t.Fatalf("failed to create tokenizer: %v", err)
	}

	tests := []struct {
		name      string
		content   string
		maxTokens int
		expected  bool
	}{
		{
			name:      "empty content always fits",
			content:   "",
			maxTokens: 10,
			expected:  true,
		},
		{
			name:      "small content fits",
			content:   "Hello world",
			maxTokens: 100,
			expected:  true,
		},
		{
			name:      "content exceeds limit",
			content:   "This is a longer piece of text that will definitely exceed a very small token limit.",
			maxTokens: 5,
			expected:  false,
		},
		{
			name:      "content at exact boundary",
			content:   "test",
			maxTokens: 1, // "test" is typically 1 token
			expected:  true,
		},
		{
			name:      "zero max tokens",
			content:   "any text",
			maxTokens: 0,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenizer.WillFitInContext(tt.content, tt.maxTokens)
			if result != tt.expected {
				tokenCount := tokenizer.CountTokens(tt.content)
				t.Errorf("expected %v, got %v (content has %d tokens, max is %d)", tt.expected, result, tokenCount, tt.maxTokens)
			}
		})
	}
}

func TestTokenizerInterface(t *testing.T) {
	// Verify that Tokenizer implements TokenizerInterface
	var _ TokenizerInterface = (*Tokenizer)(nil)

	tokenizer, err := NewTokenizer("gpt-4")
	if err != nil {
		t.Fatalf("failed to create tokenizer: %v", err)
	}

	// Test that all interface methods work
	t.Run("interface methods", func(t *testing.T) {
		text := "Test text for interface"

		// CountTokens
		count := tokenizer.CountTokens(text)
		if count <= 0 {
			t.Errorf("CountTokens should return positive count, got %d", count)
		}

		// EstimateSystemPromptTokens
		sysCount := tokenizer.EstimateSystemPromptTokens(text)
		if sysCount <= count {
			t.Errorf("EstimateSystemPromptTokens should add overhead, got %d vs base %d", sysCount, count)
		}

		// EstimateUserPromptTokens
		userCount := tokenizer.EstimateUserPromptTokens(text)
		if userCount <= count {
			t.Errorf("EstimateUserPromptTokens should add overhead, got %d vs base %d", userCount, count)
		}

		// WillFitInContext
		if !tokenizer.WillFitInContext(text, 1000) {
			t.Error("short text should fit in large context")
		}
		if tokenizer.WillFitInContext(text, 1) {
			t.Error("text should not fit in tiny context")
		}
	})
}

func TestTokenizerConsistency(t *testing.T) {
	// Test that different tokenizer instances produce consistent results
	tokenizer1, err := NewTokenizer("gpt-4")
	if err != nil {
		t.Fatalf("failed to create tokenizer1: %v", err)
	}

	tokenizer2, err := NewTokenizer("gpt-4")
	if err != nil {
		t.Fatalf("failed to create tokenizer2: %v", err)
	}

	testTexts := []string{
		"Hello, world!",
		"This is a longer text with multiple sentences. It should produce consistent token counts.",
		"Special characters: @#$%^&*()",
		"",
	}

	for _, text := range testTexts {
		count1 := tokenizer1.CountTokens(text)
		count2 := tokenizer2.CountTokens(text)
		if count1 != count2 {
			t.Errorf("inconsistent counts for '%s': tokenizer1=%d, tokenizer2=%d", text, count1, count2)
		}
	}
}
