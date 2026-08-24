package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// =============================================================================
// OpenAI-Compatible Provider Tests
// =============================================================================

func TestOpenAICompatProvider_Creation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     openAICompatConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: openAICompatConfig{
				APIKey:    "test-key",
				BaseURL:   "https://api.example.com/v1",
				Model:     "model-1",
				MaxTokens: 4096,
			},
			wantErr: false,
		},
		{
			name: "missing base url",
			cfg: openAICompatConfig{
				APIKey:    "test-key",
				Model:     "model-1",
				MaxTokens: 4096,
			},
			wantErr: true,
		},
		{
			name: "missing model",
			cfg: openAICompatConfig{
				BaseURL:   "https://api.example.com/v1",
				MaxTokens: 4096,
			},
			wantErr: true,
		},
		{
			name: "missing max_tokens",
			cfg: openAICompatConfig{
				BaseURL: "https://api.example.com/v1",
				Model:   "model-1",
			},
			wantErr: true,
		},
		{
			name: "api key optional for local",
			cfg: openAICompatConfig{
				BaseURL:   "http://localhost:11434/v1",
				Model:     "llama3",
				MaxTokens: 4096,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newOpenAICompat("test", tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("newOpenAICompat() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOpenAICompatProvider_MockServer(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}

		// Return mock response
		resp := map[string]interface{}{
			"id":    "test-id",
			"model": "test-model",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Hello from mock server!",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, err := newOpenAICompat("test", openAICompatConfig{
		BaseURL:   server.URL,
		Model:     "test-model",
		MaxTokens: 4096,
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	resp, err := provider.Chat(context.Background(), ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}

	if resp.Content != "Hello from mock server!" {
		t.Errorf("expected 'Hello from mock server!', got %s", resp.Content)
	}
	if resp.InputTokens != 10 {
		t.Errorf("expected 10 input tokens, got %d", resp.InputTokens)
	}
	if resp.OutputTokens != 5 {
		t.Errorf("expected 5 output tokens, got %d", resp.OutputTokens)
	}
}

func TestOpenAICompatProvider_ToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"id":    "test-id",
			"model": "test-model",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_123",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "read",
									"arguments": `{"path": "/test.txt"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     10,
				"completion_tokens": 5,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := newOpenAICompat("test", openAICompatConfig{
		BaseURL:   server.URL,
		Model:     "test-model",
		MaxTokens: 4096,
	})

	resp, err := provider.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "Read the file"}},
		Tools: []ToolDef{{
			Name:        "read",
			Description: "Read a file",
			Parameters:  map[string]interface{}{"type": "object"},
		}},
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "read" {
		t.Errorf("expected tool name 'read', got %s", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Args["path"] != "/test.txt" {
		t.Errorf("expected path '/test.txt', got %v", resp.ToolCalls[0].Args["path"])
	}
}

func TestOpenAICompatProvider_RateLimitRetry(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(429)
			w.Write([]byte(`{"error": "rate limit exceeded"}`))
			return
		}
		// Success on third call
		resp := map[string]interface{}{
			"id":    "test-id",
			"model": "test-model",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Success after retry!",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     10,
				"completion_tokens": 5,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := newOpenAICompat("test", openAICompatConfig{
		BaseURL:   server.URL,
		Model:     "test-model",
		MaxTokens: 4096,
		Retry: RetryConfig{
			MaxRetries:  5,
			InitBackoff: 10 * time.Millisecond,
			MaxBackoff:  100 * time.Millisecond,
		},
	})

	resp, err := provider.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}

	if calls != 3 {
		t.Errorf("expected 3 calls (2 retries), got %d", calls)
	}
	if resp.Content != "Success after retry!" {
		t.Errorf("expected 'Success after retry!', got %s", resp.Content)
	}
}

func TestGroqProvider_Creation(t *testing.T) {
	cfg := openAICompatConfig{
		APIKey:    "test-key",
		Model:     "llama-3.1-70b-versatile",
		MaxTokens: 4096,
	}

	provider, err := newOpenAICompat("groq", cfg)
	if err != nil {
		t.Fatalf("newGroqProvider() error: %v", err)
	}
	if provider == nil {
		t.Error("expected non-nil provider")
	}
	// Check that BaseURL was set to Groq's default
	if provider.baseURL != GroqBaseURL {
		t.Errorf("expected base URL %s, got %s", GroqBaseURL, provider.baseURL)
	}
}

func TestMistralProvider_Creation(t *testing.T) {
	cfg := openAICompatConfig{
		APIKey:    "test-key",
		Model:     "mistral-large-latest",
		MaxTokens: 4096,
	}

	provider, err := newOpenAICompat("mistral", cfg)
	if err != nil {
		t.Fatalf("newMistralProvider() error: %v", err)
	}
	if provider == nil {
		t.Error("expected non-nil provider")
	}
	// Check that BaseURL was set to Mistral's default
	if provider.baseURL != MistralBaseURL {
		t.Errorf("expected base URL %s, got %s", MistralBaseURL, provider.baseURL)
	}
}

func TestXAIProvider_Creation(t *testing.T) {
	cfg := openAICompatConfig{
		APIKey:    "test-key",
		Model:     "grok-2",
		MaxTokens: 4096,
	}

	provider, err := newOpenAICompat("xai", cfg)
	if err != nil {
		t.Fatalf("newXAIProvider() error: %v", err)
	}
	if provider == nil {
		t.Error("expected non-nil provider")
	}
	if provider.baseURL != XAIBaseURL {
		t.Errorf("expected base URL %s, got %s", XAIBaseURL, provider.baseURL)
	}
}

func TestOpenRouterProvider_Creation(t *testing.T) {
	cfg := openAICompatConfig{
		APIKey:    "test-key",
		Model:     "anthropic/claude-3-opus",
		MaxTokens: 4096,
	}

	provider, err := newOpenAICompat("openrouter", cfg)
	if err != nil {
		t.Fatalf("newOpenRouterProvider() error: %v", err)
	}
	if provider == nil {
		t.Error("expected non-nil provider")
	}
	if provider.baseURL != OpenRouterBaseURL {
		t.Errorf("expected base URL %s, got %s", OpenRouterBaseURL, provider.baseURL)
	}
}

func TestOllamaLocalProvider_Creation(t *testing.T) {
	cfg := openAICompatConfig{
		Model:     "llama3",
		MaxTokens: 4096,
		// No API key required
	}

	provider, err := newOpenAICompat("ollama-local", cfg)
	if err != nil {
		t.Fatalf("newOllamaLocalProvider() error: %v", err)
	}
	if provider == nil {
		t.Error("expected non-nil provider")
	}
	if provider.baseURL != OllamaLocalURL {
		t.Errorf("expected base URL %s, got %s", OllamaLocalURL, provider.baseURL)
	}
}

func TestLMStudioProvider_Creation(t *testing.T) {
	cfg := openAICompatConfig{
		Model:     "local-model",
		MaxTokens: 4096,
		// No API key required
	}

	provider, err := newOpenAICompat("lmstudio", cfg)
	if err != nil {
		t.Fatalf("newLMStudioProvider() error: %v", err)
	}
	if provider == nil {
		t.Error("expected non-nil provider")
	}
	if provider.baseURL != LMStudioLocalURL {
		t.Errorf("expected base URL %s, got %s", LMStudioLocalURL, provider.baseURL)
	}
}

// Test OAI compat conversion helpers

func TestToOAICompatMessages(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "Be helpful."},
		{Role: "user", Content: "Hi"},
		{Role: "assistant", Content: "Hello!"},
	}
	result := toOAICompatMessages(msgs)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	if result[0].Role != "system" || result[0].Content != "Be helpful." {
		t.Errorf("unexpected system message: %+v", result[0])
	}
}

func TestToOAICompatMessages_ToolCalls(t *testing.T) {
	msgs := []Message{
		{Role: "assistant", Content: "", ToolCalls: []ToolCallResponse{
			{ID: "tc-1", Name: "ls", Args: map[string]any{"path": "/"}},
		}},
		{Role: "tool", ToolCallID: "tc-1", Content: "file1.txt"},
	}
	result := toOAICompatMessages(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if len(result[0].ToolCalls) != 1 {
		t.Errorf("expected 1 tool call on assistant message, got %d", len(result[0].ToolCalls))
	}
	if result[1].ToolCallID != "tc-1" {
		t.Errorf("expected tool call ID tc-1, got %q", result[1].ToolCallID)
	}
}

func TestToOAICompatTools(t *testing.T) {
	tools := []ToolDef{
		{Name: "read", Description: "Read a file", Parameters: map[string]any{"type": "object"}},
		{Name: "write", Description: "Write a file", Parameters: map[string]any{"type": "object"}},
	}
	result := toOAICompatTools(tools)
	if len(result) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result))
	}
	if result[0].Function.Name != "read" {
		t.Errorf("expected tool name 'read', got %q", result[0].Function.Name)
	}
}

func TestFromOAICompatResponse_Basic(t *testing.T) {
	resp := &oaiResponse{
		Model: "gpt-4",
		Choices: []struct {
			Index        int        `json:"index"`
			Message      oaiMessage `json:"message"`
			FinishReason string     `json:"finish_reason"`
		}{
			{
				Message:      oaiMessage{Content: "Hello!"},
				FinishReason: "stop",
			},
		},
		Usage: struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		}{
			PromptTokens:     10,
			CompletionTokens: 5,
		},
	}

	result, err := fromOAICompatResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "Hello!" {
		t.Errorf("expected content 'Hello!', got %q", result.Content)
	}
	if result.Model != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got %q", result.Model)
	}
	if result.InputTokens != 10 || result.OutputTokens != 5 {
		t.Errorf("unexpected tokens: in=%d out=%d", result.InputTokens, result.OutputTokens)
	}
}

func TestFromOAICompatResponse_ToolCalls(t *testing.T) {
	resp := &oaiResponse{
		Model: "gpt-4",
		Choices: []struct {
			Index        int        `json:"index"`
			Message      oaiMessage `json:"message"`
			FinishReason string     `json:"finish_reason"`
		}{
			{
				Message: oaiMessage{
					ToolCalls: []oaiToolCall{
						{ID: "tc-1", Type: "function", Function: oaiFunction{Name: "search", Arguments: `{"q":"go"}`}},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}

	result, err := fromOAICompatResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Name != "search" {
		t.Errorf("expected tool name 'search', got %q", result.ToolCalls[0].Name)
	}
	if result.ToolCalls[0].Args["q"] != "go" {
		t.Errorf("expected arg q='go', got %v", result.ToolCalls[0].Args["q"])
	}
}

func TestFromOAICompatResponse_MalformedToolArgs(t *testing.T) {
	resp := &oaiResponse{
		Model: "gpt-4",
		Choices: []struct {
			Index        int        `json:"index"`
			Message      oaiMessage `json:"message"`
			FinishReason string     `json:"finish_reason"`
		}{
			{
				Message: oaiMessage{
					ToolCalls: []oaiToolCall{
						{ID: "tc-1", Function: oaiFunction{Name: "bad", Arguments: `{not json`}},
					},
				},
			},
		},
	}

	_, err := fromOAICompatResponse(resp)
	if err == nil {
		t.Error("expected error for malformed tool arguments")
	}
}

// =============================================================================
// ToolChoice Tests
// =============================================================================

func TestToOAICompatToolChoice(t *testing.T) {
	tests := []struct {
		name   string
		choice ToolChoice
		want   any
	}{
		{"auto", ToolChoiceAuto, nil},
		{"required", ToolChoiceRequired, "required"},
		{"named tool", ToolChoiceTool("verdict"), oaiNamedToolChoice{Type: "function", Function: oaiNamedToolChoiceFunction{Name: "verdict"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toOAICompatToolChoice(tt.choice)
			if got != tt.want {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

// TestOpenAICompatProvider_ToolChoiceInRequest asserts the emitted request
// JSON carries tool_choice — arbitrary OpenAI-compatible servers should be
// able to honor it, but a server that ignores it must not break the call
// (see TestOpenAICompatProvider_MockServer, which sends no ToolChoice at all).
func TestOpenAICompatProvider_ToolChoiceInRequest(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		resp := map[string]interface{}{
			"id":    "test-id",
			"model": "test-model",
			"choices": []map[string]interface{}{
				{"index": 0, "message": map[string]interface{}{"role": "assistant", "content": "ok"}, "finish_reason": "stop"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, err := newOpenAICompat("test", openAICompatConfig{
		BaseURL:   server.URL,
		Model:     "test-model",
		MaxTokens: 4096,
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	_, err = provider.Chat(context.Background(), ChatRequest{
		Messages:   []Message{{Role: "user", Content: "hi"}},
		ToolChoice: ToolChoiceTool("verdict"),
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}

	var sent oaiRequest
	if err := json.Unmarshal(gotBody, &sent); err != nil {
		t.Fatalf("unmarshal sent body: %v", err)
	}
	tc, ok := sent.ToolChoice.(map[string]interface{})
	if !ok {
		t.Fatalf("expected tool_choice object in request, got %#v", sent.ToolChoice)
	}
	if tc["type"] != "function" {
		t.Errorf("expected tool_choice.type=function, got %v", tc["type"])
	}
}
