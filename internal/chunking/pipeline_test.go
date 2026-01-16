package chunking

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
	"github.com/zorak1103/dlia/internal/llm"
	"github.com/zorak1103/dlia/internal/prompts"
)

const (
	testMockAnalysisResponse = "Mock analysis response"
)

// MockTokenizer implements a mock tokenizer for testing
type MockTokenizer struct {
	tokensPerChar float64
}

func NewMockTokenizer(tokensPerChar float64) *MockTokenizer {
	return &MockTokenizer{tokensPerChar: tokensPerChar}
}

func (m *MockTokenizer) CountTokens(text string) int {
	return int(float64(len(text)) * m.tokensPerChar)
}

func (m *MockTokenizer) EstimateSystemPromptTokens(systemPrompt string) int {
	return m.CountTokens(systemPrompt) + 4
}

func (m *MockTokenizer) EstimateUserPromptTokens(userPrompt string) int {
	return m.CountTokens(userPrompt) + 4
}

func (m *MockTokenizer) WillFitInContext(content string, maxTokens int) bool {
	return m.CountTokens(content) <= maxTokens
}

// MockLLMClient implements a mock LLM client for testing
type MockLLMClient struct {
	analyzeResponse   string
	analyzeUsage      *llm.TokenUsage
	analyzeError      error
	summarizeResponse string
	summarizeError    error
}

func NewMockLLMClient() *MockLLMClient {
	return &MockLLMClient{
		analyzeResponse: "Mock analysis response",
		analyzeUsage: &llm.TokenUsage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
		summarizeResponse: "Mock summary response",
	}
}

func (m *MockLLMClient) Analyze(_ context.Context, _, _, _ string) (string, *llm.TokenUsage, error) {
	return m.analyzeResponse, m.analyzeUsage, m.analyzeError
}

func (m *MockLLMClient) SummarizeChunk(_ context.Context, _, _, _ string) (string, error) {
	return m.summarizeResponse, m.summarizeError
}

func (m *MockLLMClient) ChatCompletion(_ context.Context, _ []llm.ChatMessage, _ float64, _ int) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		Choices: []llm.Choice{
			{
				Message: llm.ChatMessage{
					Role:    "assistant",
					Content: m.analyzeResponse,
				},
			},
		},
		Usage: *m.analyzeUsage,
	}, m.analyzeError
}

func TestNewPipeline(t *testing.T) {
	tests := []struct {
		name           string
		model          string
		maxTokens      int
		wantErr        bool
		checkTokenizer bool
		checkClient    bool
	}{
		{
			name:           "valid pipeline creation",
			model:          "gpt-4",
			maxTokens:      8000,
			wantErr:        false,
			checkTokenizer: true,
			checkClient:    true,
		},
		{
			name:           "empty model",
			model:          "",
			maxTokens:      8000,
			wantErr:        false, // May or may not error depending on tokenizer implementation
			checkTokenizer: false,
			checkClient:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llmClient := NewMockLLMClient()
			testCfg := &config.Config{
				Prompts: config.PromptsConfig{},
			}
			promptLoader := prompts.NewPromptLoader(testCfg)
			pipeline, err := NewPipeline(tt.model, tt.maxTokens, llmClient, promptLoader, nil)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			if err != nil {
				// Empty model may error - log and continue
				t.Logf("NewPipeline returned error (may be expected): %v", err)
				return
			}

			require.NotNil(t, pipeline)

			if tt.checkTokenizer {
				assert.NotNil(t, pipeline.tokenizer, "Expected tokenizer to be set")
			}

			if tt.checkClient {
				assert.NotNil(t, pipeline.client, "Expected client to be set")
			}

			assert.Equal(t, tt.maxTokens, pipeline.maxTokens)
		})
	}
}

func TestPipeline_AnalyzeLogs(t *testing.T) {
	tests := []struct {
		name                string
		logs                []docker.LogEntry
		maxTokens           int
		tokensPerChar       float64
		setupMock           func(*MockLLMClient)
		regexpFilters       map[string]*RegexpFilter
		containerID         string
		wantErr             bool
		checkAnalysis       string
		checkOriginalCount  int
		checkProcessedCount int
		checkChunksUsed     int
		minChunksUsed       int // Use this when exact chunk count is unpredictable
		checkTokensUsed     int
		checkDeduplicated   bool
		checkFilterStats    bool
		wantLinesTotal      int
		wantLinesFiltered   int
		wantLinesKept       int
	}{
		{
			name:               "empty logs",
			logs:               []docker.LogEntry{},
			maxTokens:          8000,
			tokensPerChar:      0.1,
			containerID:        "test-container",
			wantErr:            false,
			checkAnalysis:      "No logs to analyze",
			checkOriginalCount: 0,
		},
		{
			name: "single chunk",
			logs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Log message 1"},
				{Timestamp: "2023-01-01T10:00:01Z", Stream: "stdout", Message: "Log message 2"},
				{Timestamp: "2023-01-01T10:00:02Z", Stream: "stdout", Message: "Log message 3"},
			},
			maxTokens:           8000,
			tokensPerChar:       0.1,
			containerID:         "test-container",
			wantErr:             false,
			checkAnalysis:       testMockAnalysisResponse,
			checkOriginalCount:  3,
			checkProcessedCount: 3,
			checkChunksUsed:     1,
			checkTokensUsed:     150,
		},
		{
			name: "with deduplication",
			logs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Repeated message"},
				{Timestamp: "2023-01-01T10:00:01Z", Stream: "stdout", Message: "Repeated message"},
				{Timestamp: "2023-01-01T10:00:02Z", Stream: "stdout", Message: "Repeated message"},
				{Timestamp: "2023-01-01T10:00:03Z", Stream: "stdout", Message: "Repeated message"},
				{Timestamp: "2023-01-01T10:00:04Z", Stream: "stdout", Message: "Unique message"},
			},
			maxTokens:           8000,
			tokensPerChar:       0.1,
			containerID:         "test-container",
			wantErr:             false,
			checkOriginalCount:  5,
			checkProcessedCount: 2, // [REPEAT x4] and unique message
			checkDeduplicated:   true,
		},
		{
			name: "LLM error",
			logs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Test message"},
			},
			maxTokens:     8000,
			tokensPerChar: 0.1,
			setupMock: func(m *MockLLMClient) {
				m.analyzeError = &llm.APIError{Message: "API Error", Type: "api_error", Code: "rate_limit_exceeded"}
			},
			containerID: "test-container",
			wantErr:     true,
		},
		{
			name: "with chunking",
			logs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "This is a longer log message that contains multiple words and should take up more tokens"},
				{Timestamp: "2023-01-01T10:00:01Z", Stream: "stdout", Message: "Another long log message with different content to ensure we test chunking properly"},
				{Timestamp: "2023-01-01T10:00:02Z", Stream: "stdout", Message: "Yet another message to add more content and force multiple chunks in our test"},
				{Timestamp: "2023-01-01T10:00:03Z", Stream: "stdout", Message: "Final message for comprehensive chunking test coverage verification"},
			},
			maxTokens:     500, // Small max to force chunking
			tokensPerChar: 1.0, // High token count to force chunking
			containerID:   "test-container",
			wantErr:       false,
			checkAnalysis: testMockAnalysisResponse,
			minChunksUsed: 2, // Chunking should produce at least 2 chunks
		},
		{
			name: "with regexp filtering",
			logs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "DEBUG: Verbose debugging info"},
				{Timestamp: "2023-01-01T10:00:01Z", Stream: "stdout", Message: "INFO: Application started"},
				{Timestamp: "2023-01-01T10:00:02Z", Stream: "stdout", Message: "DEBUG: Connection established"},
				{Timestamp: "2023-01-01T10:00:03Z", Stream: "stdout", Message: "ERROR: Something went wrong"},
				{Timestamp: "2023-01-01T10:00:04Z", Stream: "stdout", Message: "DEBUG: Processing request"},
				{Timestamp: "2023-01-01T10:00:05Z", Stream: "stdout", Message: "WARN: High memory usage"},
			},
			maxTokens:     8000,
			tokensPerChar: 0.1,
			regexpFilters: func() map[string]*RegexpFilter {
				filter, _ := NewRegexpFilter([]string{"^DEBUG:"})
				return map[string]*RegexpFilter{"test-container": filter}
			}(),
			containerID:         "test-container",
			wantErr:             false,
			checkOriginalCount:  6,
			checkProcessedCount: 3, // Only non-DEBUG lines
			checkFilterStats:    true,
			wantLinesTotal:      6,
			wantLinesFiltered:   3, // 3 DEBUG lines filtered
			wantLinesKept:       3, // 3 non-DEBUG lines kept
		},
		{
			name: "without regexp filtering",
			logs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "DEBUG: This should not be filtered"},
				{Timestamp: "2023-01-01T10:00:01Z", Stream: "stdout", Message: "INFO: Regular log"},
			},
			maxTokens:           8000,
			tokensPerChar:       0.1,
			regexpFilters:       nil, // No filters
			containerID:         "test-container",
			wantErr:             false,
			checkProcessedCount: 2,
			checkFilterStats:    true,
			wantLinesTotal:      2,
			wantLinesFiltered:   0, // No filtering
			wantLinesKept:       2, // All kept
		},
		{
			name: "regexp filter for different container",
			logs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "DEBUG: Should not be filtered"},
				{Timestamp: "2023-01-01T10:00:01Z", Stream: "stdout", Message: "INFO: Regular log"},
			},
			maxTokens:     8000,
			tokensPerChar: 0.1,
			regexpFilters: func() map[string]*RegexpFilter {
				filter, _ := NewRegexpFilter([]string{"^DEBUG:"})
				return map[string]*RegexpFilter{"other-container": filter} // Filter for different container
			}(),
			containerID:       "test-container",
			wantErr:           false,
			checkFilterStats:  true,
			wantLinesTotal:    2, // Total lines before filtering
			wantLinesFiltered: 0, // Filter should not apply
			wantLinesKept:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llmClient := NewMockLLMClient()
			if tt.setupMock != nil {
				tt.setupMock(llmClient)
			}

			tokenizer := NewMockTokenizer(tt.tokensPerChar)
			// Create a minimal config for the promptLoader
			testCfg := &config.Config{
				Prompts: config.PromptsConfig{},
			}
			promptLoader := prompts.NewPromptLoader(testCfg)

			pipeline := &Pipeline{
				client:                     llmClient,
				maxTokens:                  tt.maxTokens,
				tokenizer:                  tokenizer,
				compiledRegexpsByContainer: tt.regexpFilters,
				promptLoader:               promptLoader,
			}

			ctx := context.Background()
			result, err := pipeline.AnalyzeLogs(ctx, tt.containerID, tt.logs)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			if tt.checkAnalysis != "" {
				assert.Equal(t, tt.checkAnalysis, result.Analysis)
			}

			if tt.checkOriginalCount > 0 || len(tt.logs) == 0 {
				assert.Equal(t, tt.checkOriginalCount, result.OriginalCount)
			}

			if tt.checkProcessedCount > 0 {
				assert.Equal(t, tt.checkProcessedCount, result.ProcessedCount)
			}

			if tt.checkChunksUsed != 0 {
				assert.Equal(t, tt.checkChunksUsed, result.ChunksUsed)
			}

			if tt.minChunksUsed > 0 {
				assert.GreaterOrEqual(t, result.ChunksUsed, tt.minChunksUsed,
					"expected at least %d chunks", tt.minChunksUsed)
			}

			if tt.checkTokensUsed > 0 {
				assert.Equal(t, tt.checkTokensUsed, result.TokensUsed)
			}

			if tt.checkDeduplicated {
				assert.True(t, result.Deduplicated, "Expected deduplicated to be true")
			}

			if tt.checkFilterStats {
				assert.Equal(t, tt.wantLinesTotal, result.FilterStats.LinesTotal)
				assert.Equal(t, tt.wantLinesFiltered, result.FilterStats.LinesFiltered)
				assert.Equal(t, tt.wantLinesKept, result.FilterStats.LinesKept)
				// Verify consistency: Total = Filtered + Kept
				assert.Equal(t, result.FilterStats.LinesTotal,
					result.FilterStats.LinesFiltered+result.FilterStats.LinesKept,
					"FilterStats inconsistent")
			}
		})
	}
}

func TestMockTokenizer(t *testing.T) {
	tests := []struct {
		name          string
		tokensPerChar float64
		text          string
		maxTokens     int
		testType      string // "count", "system", "user", "willfit"
		wantTokens    int
		wantFit       bool
	}{
		{
			name:          "count tokens",
			tokensPerChar: 0.1,
			text:          "This is a test message", // 22 chars
			testType:      "count",
			wantTokens:    2, // 22 * 0.1 = 2.2 -> 2 (integer division)
		},
		{
			name:          "estimate system prompt tokens",
			tokensPerChar: 0.1,
			text:          "System prompt", // 13 chars
			testType:      "system",
			wantTokens:    5, // (13 * 0.1) + 4 = 1 + 4 = 5
		},
		{
			name:          "estimate user prompt tokens",
			tokensPerChar: 0.1,
			text:          "Test user prompt", // 16 chars
			testType:      "user",
			wantTokens:    5, // (16 * 0.1) + 4 = 1 + 4 = 5
		},
		{
			name:          "will fit in context - short text",
			tokensPerChar: 0.1,
			text:          "short text", // 10 chars = 1 token
			maxTokens:     100,
			testType:      "willfit",
			wantFit:       true,
		},
		{
			name:          "will not fit in context - long text",
			tokensPerChar: 0.1,
			text: func() string {
				s := ""
				for i := 0; i < 2000; i++ {
					s += "word "
				}
				return s
			}(),
			maxTokens: 100,
			testType:  "willfit",
			wantFit:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenizer := NewMockTokenizer(tt.tokensPerChar)

			switch tt.testType {
			case "count":
				result := tokenizer.CountTokens(tt.text)
				assert.Equal(t, tt.wantTokens, result)

			case "system":
				result := tokenizer.EstimateSystemPromptTokens(tt.text)
				assert.Equal(t, tt.wantTokens, result)

			case "user":
				result := tokenizer.EstimateUserPromptTokens(tt.text)
				assert.Equal(t, tt.wantTokens, result)

			case "willfit":
				result := tokenizer.WillFitInContext(tt.text, tt.maxTokens)
				assert.Equal(t, tt.wantFit, result)
			}
		})
	}
}

func TestPipeline_AnalyzeDirectly(t *testing.T) {
	llmClient := NewMockLLMClient()
	tokenizer := NewMockTokenizer(0.1)
	testCfg := &config.Config{
		Prompts: config.PromptsConfig{},
	}
	promptLoader := prompts.NewPromptLoader(testCfg)

	pipeline := &Pipeline{
		client:       llmClient,
		maxTokens:    8000,
		tokenizer:    tokenizer,
		promptLoader: promptLoader,
	}

	logs := []docker.LogEntry{
		{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Direct analysis test"},
	}

	logsText := FormatLogs(logs)
	ctx := context.Background()
	analysis, usage, err := pipeline.analyzeDirectly(ctx, "test-container", logs, "system prompt", logsText)

	require.NoError(t, err)
	assert.Equal(t, testMockAnalysisResponse, analysis)
	assert.Equal(t, 150, usage.TotalTokens)
}

func TestPipeline_AnalyzeWithChunking(t *testing.T) {
	tests := []struct {
		name            string
		logs            []docker.LogEntry
		setupMock       func(*MockLLMClient)
		availableTokens int
		wantErr         bool
		checkAnalysis   string
		checkTokens     int
	}{
		{
			name: "summarization error",
			logs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Long message to force chunking and error"},
				{Timestamp: "2023-01-01T10:00:01Z", Stream: "stdout", Message: "Another long message for error testing"},
			},
			setupMock: func(m *MockLLMClient) {
				m.summarizeError = &llm.APIError{Message: "Summarize Error", Type: "api_error", Code: "error"}
			},
			availableTokens: 200,
			wantErr:         true,
		},
		{
			name:            "no chunks - empty logs",
			logs:            []docker.LogEntry{},
			availableTokens: 1,
			wantErr:         false,
			checkAnalysis:   "No logs could be processed within token limits",
			checkTokens:     0,
		},
		{
			name: "synthesis error",
			logs: []docker.LogEntry{
				{Timestamp: "2023-01-01T10:00:00Z", Stream: "stdout", Message: "Long message to force chunking"},
				{Timestamp: "2023-01-01T10:00:01Z", Stream: "stdout", Message: "Another long message"},
			},
			setupMock: func(m *MockLLMClient) {
				// First let summarize succeed, then make analyze fail
				m.analyzeError = &llm.APIError{Message: "Synthesis Error", Type: "api_error", Code: "error"}
			},
			availableTokens: 200,
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llmClient := NewMockLLMClient()
			if tt.setupMock != nil {
				tt.setupMock(llmClient)
			}

			tokenizer := NewMockTokenizer(1.0)
			testCfg := &config.Config{
				Prompts: config.PromptsConfig{},
			}
			promptLoader := prompts.NewPromptLoader(testCfg)

			pipeline := &Pipeline{
				client:       llmClient,
				maxTokens:    500,
				tokenizer:    tokenizer,
				promptLoader: promptLoader,
			}

			ctx := context.Background()
			analysis, tokens, chunksUsed, err := pipeline.analyzeWithChunking(ctx, "test-container", tt.logs, "system prompt", tt.availableTokens)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.checkAnalysis != "" {
				assert.Equal(t, tt.checkAnalysis, analysis)
			}

			if tt.checkTokens >= 0 {
				assert.Equal(t, tt.checkTokens, tokens)
			}

			// Verify chunksUsed is non-negative (0 for empty, positive for actual chunks)
			assert.GreaterOrEqual(t, chunksUsed, 0)
		})
	}
}

func TestMockLLMClient_ChatCompletion(t *testing.T) {
	client := NewMockLLMClient()
	ctx := context.Background()

	response, err := client.ChatCompletion(ctx, []llm.ChatMessage{
		{Role: "user", Content: "test"},
	}, 0.7, 1000)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Len(t, response.Choices, 1)
	assert.Equal(t, "Mock analysis response", response.Choices[0].Message.Content)
	assert.Equal(t, 150, response.Usage.TotalTokens)
}

func TestPipeline_Constants(t *testing.T) {
	assert.Equal(t, 4000, ResponseReserveTokens, "Expected ResponseReserveTokens to be 4000")
	assert.Equal(t, 500, SystemPromptReserveTokens, "Expected SystemPromptReserveTokens to be 500")
	assert.Equal(t, 2, ChunkSizeDivisor, "Expected ChunkSizeDivisor to be 2")
}
