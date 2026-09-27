package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zorak1103/dlia/internal/llmlogger"
)

func TestNewClient(t *testing.T) {
	baseURL := "https://api.openai.com/v1"
	apiKey := "test-key"
	model := "gpt-4"

	client := NewClient(baseURL, apiKey, model)

	// Type assert to access implementation fields for testing
	impl, ok := client.(*clientImpl)
	if !ok {
		t.Fatal("Expected client to be *clientImpl")
	}

	if impl.baseURL != baseURL {
		t.Errorf("Expected baseURL %s, got %s", baseURL, impl.baseURL)
	}

	if impl.apiKey != apiKey {
		t.Errorf("Expected apiKey %s, got %s", apiKey, impl.apiKey)
	}

	if impl.model != model {
		t.Errorf("Expected model %s, got %s", model, impl.model)
	}

	if impl.httpClient.Timeout != 120*time.Second {
		t.Errorf("Expected timeout 120s, got %v", impl.httpClient.Timeout)
	}
}

func TestClient_Analyze(t *testing.T) {
	tests := []struct {
		name          string
		systemPrompt  string
		userPrompt    string
		response      ChatResponse
		responseCode  int
		expectError   bool
		expectContent string
		expectTokens  int
	}{
		{
			name:         "successful analysis",
			systemPrompt: "You are a helpful assistant.",
			userPrompt:   "Analyze these logs.",
			response: ChatResponse{
				Choices: []Choice{
					{
						Message: ChatMessage{
							Role:    "assistant",
							Content: "Analysis complete.",
						},
					},
				},
				Usage: TokenUsage{
					PromptTokens:     10,
					CompletionTokens: 5,
					TotalTokens:      15,
				},
			},
			responseCode:  http.StatusOK,
			expectError:   false,
			expectContent: "Analysis complete.",
			expectTokens:  15,
		},
		{
			name:         "no choices in response",
			systemPrompt: "You are a helpful assistant.",
			userPrompt:   "Analyze these logs.",
			response: ChatResponse{
				Choices: []Choice{},
				Usage:   TokenUsage{TotalTokens: 10},
			},
			responseCode: http.StatusOK,
			expectError:  true,
		},
		{
			name:         "API error response",
			systemPrompt: "You are a helpful assistant.",
			userPrompt:   "Analyze these logs.",
			response: ChatResponse{
				Error: &APIError{
					Message: "Invalid request",
					Type:    "invalid_request_error",
					Code:    "invalid_api_key",
				},
			},
			responseCode: http.StatusUnauthorized,
			expectError:  true,
		},
		{
			name:         "HTTP error",
			systemPrompt: "You are a helpful assistant.",
			userPrompt:   "Analyze these logs.",
			response:     ChatResponse{},
			responseCode: http.StatusInternalServerError,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and path
				if r.Method != "POST" {
					t.Errorf("Expected POST request, got %s", r.Method)
				}
				if r.URL.Path != "/chat/completions" {
					t.Errorf("Expected path /chat/completions, got %s", r.URL.Path)
				}

				// Verify headers
				if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
					t.Errorf("Expected Content-Type application/json, got %s", contentType)
				}

				// Verify authorization header
				if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
					t.Errorf("Expected Authorization Bearer test-key, got %s", auth)
				}

				// Read and verify request body
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("Failed to read request body: %v", err)
					return
				}

				var req ChatRequest
				if err := json.Unmarshal(body, &req); err != nil {
					t.Errorf("Failed to unmarshal request: %v", err)
					return
				}

				if req.Model != "test-model" {
					t.Errorf("Expected model test-model, got %s", req.Model)
				}

				if len(req.Messages) != 2 {
					t.Errorf("Expected 2 messages, got %d", len(req.Messages))
				}

				if req.Messages[0].Content != tt.systemPrompt {
					t.Errorf("Expected system prompt %s, got %s", tt.systemPrompt, req.Messages[0].Content)
				}

				if req.Messages[1].Content != tt.userPrompt {
					t.Errorf("Expected user prompt %s, got %s", tt.userPrompt, req.Messages[1].Content)
				}

				// Send response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.responseCode)
				json.NewEncoder(w).Encode(tt.response) // nolint:errcheck,gosec
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-key", "test-model")
			ctx := context.Background()

			content, usage, err := client.Analyze(ctx, "", tt.systemPrompt, tt.userPrompt)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			if !tt.expectError {
				if content != tt.expectContent {
					t.Errorf("Expected content %s, got %s", tt.expectContent, content)
				}

				if usage == nil {
					t.Error("Expected usage to be non-nil")
				} else if usage.TotalTokens != tt.expectTokens {
					t.Errorf("Expected %d tokens, got %d", tt.expectTokens, usage.TotalTokens)
				}
			}
		})
	}
}

func TestClient_SummarizeChunk(t *testing.T) {
	tests := []struct {
		name          string
		systemPrompt  string
		chunkPrompt   string
		response      ChatResponse
		responseCode  int
		expectError   bool
		expectContent string
	}{
		{
			name:         "successful summarization",
			systemPrompt: "Summarize the following logs.",
			chunkPrompt:  "Log content here...",
			response: ChatResponse{
				Choices: []Choice{
					{
						Message: ChatMessage{
							Role:    "assistant",
							Content: "Summary of logs.",
						},
					},
				},
			},
			responseCode:  http.StatusOK,
			expectError:   false,
			expectContent: "Summary of logs.",
		},
		{
			name:         "no choices in response",
			systemPrompt: "Summarize the following logs.",
			chunkPrompt:  "Log content here...",
			response: ChatResponse{
				Choices: []Choice{},
			},
			responseCode: http.StatusOK,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				if r.Method != "POST" {
					t.Errorf("Expected POST request, got %s", r.Method)
				}

				// Read request body
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("Failed to read request body: %v", err)
					return
				}

				var req ChatRequest
				if err := json.Unmarshal(body, &req); err != nil {
					t.Errorf("Failed to unmarshal request: %v", err)
					return
				}

				if req.Temperature != 0.3 {
					t.Errorf("Expected temperature 0.3, got %f", req.Temperature)
				}

				if req.MaxTokens != 2000 {
					t.Errorf("Expected max_tokens 2000, got %d", req.MaxTokens)
				}

				// Send response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.responseCode)
				json.NewEncoder(w).Encode(tt.response) // nolint:errcheck,gosec
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-key", "test-model")
			ctx := context.Background()

			content, err := client.SummarizeChunk(ctx, "", tt.systemPrompt, tt.chunkPrompt)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			if !tt.expectError && content != tt.expectContent {
				t.Errorf("Expected content %s, got %s", tt.expectContent, content)
			}
		})
	}
}

func TestClient_RetryLogic(t *testing.T) {
	attemptCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attemptCount++

		// Fail first 2 attempts, succeed on 3rd
		if attemptCount <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error": "service unavailable"}`)) // nolint:errcheck,gosec
			return
		}

		response := ChatResponse{
			Choices: []Choice{
				{
					Message: ChatMessage{
						Role:    "assistant",
						Content: "Success after retries",
					},
				},
			},
			Usage: TokenUsage{TotalTokens: 10},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response) // nolint:errcheck,gosec
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-model")
	ctx := context.Background()

	content, usage, err := client.Analyze(ctx, "", "system", "user")

	if err != nil {
		t.Errorf("Expected no error after retries, got: %v", err)
	}

	if content != "Success after retries" {
		t.Errorf("Expected 'Success after retries', got %s", content)
	}

	if usage.TotalTokens != 10 {
		t.Errorf("Expected 10 tokens, got %d", usage.TotalTokens)
	}

	if attemptCount != 3 {
		t.Errorf("Expected 3 attempts, got %d", attemptCount)
	}
}

func TestClient_RetryLogic_ContextCanceledDuringBackoff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Always fail so the retry loop enters its backoff sleep (1s for the first retry).
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error": "service unavailable"}`)) // nolint:errcheck,gosec
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-model")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, err := client.Analyze(ctx, "", "system", "user")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Expected error when context is canceled during retry backoff")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected error to wrap context.DeadlineExceeded, got: %v", err)
	}

	// The first backoff sleep is 1s; canceling should abort it immediately
	// instead of waiting it out.
	if elapsed >= 1*time.Second {
		t.Errorf("Expected retry to abort promptly on context cancellation, took %v", elapsed)
	}
}

func TestClient_MaxRetriesExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-model")
	ctx := context.Background()

	start := time.Now()
	_, _, err := client.Analyze(ctx, "", "system", "user")
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected error after max retries")
	}

	// Should take approximately 3 seconds (1s + 2s delays)
	if duration < 2*time.Second || duration > 4*time.Second {
		t.Errorf("Expected duration around 3 seconds, got %v", duration)
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Simulate slow response
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ChatResponse{ // nolint:errcheck,gosec
			Choices: []Choice{
				{
					Message: ChatMessage{Content: "Response"},
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-model")

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, _, err := client.Analyze(ctx, "", "system", "user")

	if err == nil {
		t.Error("Expected context cancellation error")
	}

	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context error, got: %v", err)
	}
}

func TestClient_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{ invalid json")) // nolint:errcheck,gosec
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-model")
	ctx := context.Background()

	_, _, err := client.Analyze(ctx, "", "system", "user")

	if err == nil {
		t.Error("Expected JSON parsing error")
	}

	if !strings.Contains(err.Error(), "failed to parse response") {
		t.Errorf("Expected JSON parsing error, got: %v", err)
	}
}

func TestClient_EmptyAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check that no Authorization header is set when API key is empty
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("Expected no Authorization header, got %s", auth)
		}

		response := ChatResponse{
			Choices: []Choice{
				{
					Message: ChatMessage{Content: "Response"},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response) // nolint:errcheck,gosec
	}))
	defer server.Close()

	client := NewClient(server.URL, "", "test-model")
	ctx := context.Background()

	content, _, err := client.Analyze(ctx, "", "system", "user")

	if err != nil {
		t.Errorf("Expected no error with empty API key, got: %v", err)
	}

	if content != "Response" {
		t.Errorf("Expected 'Response', got %s", content)
	}
}

func TestClient_RequestSerialization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read request body: %v", err)
			return
		}

		var req ChatRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("Failed to unmarshal request: %v", err)
			return
		}

		// Verify all fields are properly set
		if req.Model != "test-model" {
			t.Errorf("Expected model test-model, got %s", req.Model)
		}

		if req.Temperature != 0.7 {
			t.Errorf("Expected temperature 0.7, got %f", req.Temperature)
		}

		if req.MaxTokens != 1000 {
			t.Errorf("Expected max_tokens 1000, got %d", req.MaxTokens)
		}

		if len(req.Messages) != 2 {
			t.Errorf("Expected 2 messages, got %d", len(req.Messages))
		}

		// Send response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ChatResponse{ // nolint:errcheck,gosec
			Choices: []Choice{
				{
					Message: ChatMessage{Content: "OK"},
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-model")
	ctx := context.Background()

	messages := []ChatMessage{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "User prompt"},
	}

	_, err := client.ChatCompletion(ctx, messages, 0.7, 1000)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

// captureStdout redirects os.Stdout into a buffer; the returned func drains it.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	return func() string {
		os.Stdout = orig
		w.Close() // nolint:errcheck
		buf := &strings.Builder{}
		_, _ = io.Copy(buf, r)
		r.Close() // nolint:errcheck
		return buf.String()
	}
}

func newAnalyzeTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ChatResponse{ // nolint:errcheck,gosec
			Choices: []Choice{
				{Message: ChatMessage{Role: "assistant", Content: "analysis result"}},
			},
			Usage: TokenUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
		})
	}))
	t.Cleanup(server.Close)
	return server
}

// ponytail: covers the logger guard branches in Analyze (client.go:231) and
// SummarizeChunk (client.go:264) — both the successful-log path and the
// failed-log warning path — using stdout capture to pin the warning text.
func TestAnalyze_LoggerGuards(t *testing.T) {
	successResp := "analysis result"

	t.Run("successful log write is silent", func(t *testing.T) {
		server := newAnalyzeTestServer(t)
		client := NewClient(server.URL, "test-key", "test-model")
		client.SetLogger(llmlogger.NewLogger(t.TempDir(), true))

		read := captureStdout(t)
		content, usage, err := client.Analyze(context.Background(), "web-1", "system", "user")
		out := read()

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if content != successResp {
			t.Errorf("Expected content %q, got %q", successResp, content)
		}
		if usage == nil {
			t.Error("Expected usage to be non-nil")
		}
		if strings.Contains(out, "failed to log LLM interaction") {
			t.Errorf("Did not expect log warning, got stdout: %q", out)
		}
	})

	t.Run("failed log write prints warning but does not fail", func(t *testing.T) {
		server := newAnalyzeTestServer(t)
		// baseDir is a file, so MkdirAll inside the logger fails deterministically
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatalf("could not create blocker file: %v", err)
		}
		client := NewClient(server.URL, "test-key", "test-model")
		client.SetLogger(llmlogger.NewLogger(blocker, true))

		read := captureStdout(t)
		content, _, err := client.Analyze(context.Background(), "web-1", "system", "user")
		out := read()

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if content != successResp {
			t.Errorf("Expected content %q, got %q", successResp, content)
		}
		if !strings.Contains(out, "Warning: failed to log LLM interaction") {
			t.Errorf("Expected log warning on stdout, got: %q", out)
		}
	})
}

func TestSummarizeChunk_LoggerGuards(t *testing.T) {
	successResp := "analysis result"

	t.Run("successful log write is silent", func(t *testing.T) {
		server := newAnalyzeTestServer(t)
		client := NewClient(server.URL, "test-key", "test-model")
		client.SetLogger(llmlogger.NewLogger(t.TempDir(), true))

		read := captureStdout(t)
		content, err := client.SummarizeChunk(context.Background(), "web-1", "system", "chunk")
		out := read()

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if content != successResp {
			t.Errorf("Expected content %q, got %q", successResp, content)
		}
		if strings.Contains(out, "failed to log LLM interaction") {
			t.Errorf("Did not expect log warning, got stdout: %q", out)
		}
	})

	t.Run("failed log write prints warning but does not fail", func(t *testing.T) {
		server := newAnalyzeTestServer(t)
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatalf("could not create blocker file: %v", err)
		}
		client := NewClient(server.URL, "test-key", "test-model")
		client.SetLogger(llmlogger.NewLogger(blocker, true))

		read := captureStdout(t)
		content, err := client.SummarizeChunk(context.Background(), "web-1", "system", "chunk")
		out := read()

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if content != successResp {
			t.Errorf("Expected content %q, got %q", successResp, content)
		}
		if !strings.Contains(out, "Warning: failed to log LLM interaction") {
			t.Errorf("Expected log warning on stdout, got: %q", out)
		}
	})
}

func TestClient_RetryNetworkErrorExhausted(t *testing.T) {
	// Use the URL of a server that has already been closed so every attempt
	// fails at the network (transport) level, not with a retryable 5xx status.
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	baseURL := server.URL
	server.Close()

	client := NewClient(baseURL, "test-key", "test-model")

	_, err := client.ChatCompletion(
		context.Background(),
		[]ChatMessage{{Role: "user", Content: "hi"}},
		0.5, 100,
	)
	if err == nil {
		t.Fatal("Expected error when all attempts fail at the network level")
	}

	if !strings.Contains(err.Error(), "failed after 3 attempts") {
		t.Errorf("Expected 'failed after 3 attempts' error, got: %v", err)
	}
}

func TestClient_ReadResponseBodyError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Advertise more bytes than are sent so io.ReadAll fails with an
		// unexpected EOF while draining the response body.
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte(`{"partial`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-model")

	_, err := client.ChatCompletion(
		context.Background(),
		[]ChatMessage{{Role: "user", Content: "hi"}},
		0.5, 100,
	)
	if err == nil {
		t.Fatal("Expected error when response body cannot be read")
	}

	if !strings.Contains(err.Error(), "failed after 3 attempts") {
		t.Errorf("Expected read error to be retried and surfaced, got: %v", err)
	}
}

func TestClient_MarshalRequestError(t *testing.T) {
	client := NewClient("http://api.example.invalid/v1", "test-key", "test-model")

	// NaN cannot be represented in JSON, so marshaling the request fails
	// before any HTTP traffic happens.
	_, err := client.ChatCompletion(
		context.Background(),
		[]ChatMessage{{Role: "user", Content: "hi"}},
		math.NaN(), 100,
	)
	if err == nil {
		t.Fatal("Expected error when request cannot be marshaled")
	}

	if !strings.Contains(err.Error(), "failed to marshal chat completion request") {
		t.Errorf("Expected marshal error, got: %v", err)
	}
}

func TestClient_NewRequestError(t *testing.T) {
	// A base URL without a scheme makes http.NewRequestWithContext fail.
	client := NewClient("://invalid-base-url", "test-key", "test-model")

	_, err := client.ChatCompletion(
		context.Background(),
		[]ChatMessage{{Role: "user", Content: "hi"}},
		0.5, 100,
	)
	if err == nil {
		t.Fatal("Expected error when HTTP request cannot be created")
	}

	if !strings.Contains(err.Error(), "failed to create HTTP request") {
		t.Errorf("Expected request creation error, got: %v", err)
	}
}

func TestClient_OKResponseWithAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":{"code":"insufficient_quota","message":"quota exceeded","type":"quota_error"}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-model")

	_, err := client.ChatCompletion(
		context.Background(),
		[]ChatMessage{{Role: "user", Content: "hi"}},
		0.5, 100,
	)
	if err == nil {
		t.Fatal("Expected API error from a 200 response carrying an error object")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Expected *APIError, got: %v", err)
	}

	if apiErr.Code != "insufficient_quota" {
		t.Errorf("Expected code 'insufficient_quota', got %q", apiErr.Code)
	}
}

func TestClient_SummarizeChunk_PropagatesCompletionError(t *testing.T) {
	// A 4xx status is not retried, so this fails fast on the first attempt.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`bad request`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-model")

	_, err := client.SummarizeChunk(context.Background(), "test-container", "system", "chunk")
	if err == nil {
		t.Fatal("Expected SummarizeChunk to propagate the completion error")
	}

	if !strings.Contains(err.Error(), "returned status 400") {
		t.Errorf("Expected underlying API status error, got: %v", err)
	}
}

// Helper function to check if string contains substring
