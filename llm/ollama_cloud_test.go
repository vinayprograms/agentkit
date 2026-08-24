package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaCloudProvider_Config(t *testing.T) {
	// Test missing API key
	_, err := newOllamaCloud(ollamaCloudConfig{
		Model: "gpt-oss:120b",
	})
	if err == nil {
		t.Error("expected error for missing API key")
	}

	// Test missing model
	_, err = newOllamaCloud(ollamaCloudConfig{
		APIKey: "test-key",
	})
	if err == nil {
		t.Error("expected error for missing model")
	}

	// Test valid config
	p, err := newOllamaCloud(ollamaCloudConfig{
		APIKey: "test-key",
		Model:  "gpt-oss:120b",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.baseURL != "https://ollama.com" {
		t.Errorf("expected default baseURL, got %s", p.baseURL)
	}
	if p.maxTokens != 4096 {
		t.Errorf("expected default maxTokens 4096, got %d", p.maxTokens)
	}
}

func TestOllamaCloudProvider_Chat(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected path /api/chat, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer auth, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected JSON content type, got %s", r.Header.Get("Content-Type"))
		}

		// Parse request
		var req ollamaChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if req.Model != "gpt-oss:120b" {
			t.Errorf("expected model gpt-oss:120b, got %s", req.Model)
		}
		if len(req.Messages) != 1 {
			t.Errorf("expected 1 message, got %d", len(req.Messages))
		}

		// Send response
		resp := ollamaChatResponse{
			Model: "gpt-oss:120b",
			Message: ollamaMessage{
				Role:    "assistant",
				Content: "The sky is blue due to Rayleigh scattering.",
			},
			Done:            true,
			DoneReason:      "stop",
			PromptEvalCount: 10,
			EvalCount:       15,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create provider with mock server
	p, err := newOllamaCloud(ollamaCloudConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-oss:120b",
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	// Make request
	resp, err := p.Chat(context.Background(), ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "Why is the sky blue?"},
		},
	})
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}

	if resp.Content != "The sky is blue due to Rayleigh scattering." {
		t.Errorf("unexpected content: %s", resp.Content)
	}
	if resp.InputTokens != 10 {
		t.Errorf("expected 10 input tokens, got %d", resp.InputTokens)
	}
	if resp.OutputTokens != 15 {
		t.Errorf("expected 15 output tokens, got %d", resp.OutputTokens)
	}
}

func TestOllamaCloudProvider_ToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaChatResponse{
			Model: "gpt-oss:120b",
			Message: ollamaMessage{
				Role: "assistant",
				ToolCalls: []ollamaToolCall{
					{
						Function: ollamaFunction{
							Name:      "get_weather",
							Arguments: map[string]interface{}{"location": "NYC"},
						},
					},
				},
			},
			Done:       true,
			DoneReason: "tool_calls",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, _ := newOllamaCloud(ollamaCloudConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-oss:120b",
	})

	resp, err := p.Chat(context.Background(), ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "What's the weather in NYC?"},
		},
		Tools: []ToolDef{
			{Name: "get_weather", Description: "Get weather", Parameters: map[string]interface{}{}},
		},
	})
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "get_weather" {
		t.Errorf("expected tool name get_weather, got %s", resp.ToolCalls[0].Name)
	}
}

// Test Ollama conversion helpers

func TestToOllamaMessages(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "Be helpful."},
		{Role: "user", Content: "Hi"},
	}
	result := toOllamaMessages(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Role != "system" {
		t.Errorf("expected system role, got %q", result[0].Role)
	}
}

func TestToOllamaTools(t *testing.T) {
	tools := []ToolDef{
		{Name: "ls", Description: "List files", Parameters: map[string]any{"type": "object"}},
	}
	result := toOllamaTools(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Function.Name != "ls" {
		t.Errorf("expected tool name 'ls', got %q", result[0].Function.Name)
	}
}

func TestFromOllamaResponse(t *testing.T) {
	resp := &ollamaChatResponse{
		Model: "llama3",
		Message: ollamaMessage{
			Content: "Hello!",
		},
		DoneReason:      "stop",
		PromptEvalCount: 10,
		EvalCount:       5,
	}
	result := fromOllamaResponse(resp)
	if result.Content != "Hello!" {
		t.Errorf("expected content 'Hello!', got %q", result.Content)
	}
	if result.InputTokens != 10 || result.OutputTokens != 5 {
		t.Errorf("unexpected tokens: in=%d out=%d", result.InputTokens, result.OutputTokens)
	}
}

func TestFromOllamaResponse_ToolCalls(t *testing.T) {
	resp := &ollamaChatResponse{
		Model: "llama3",
		Message: ollamaMessage{
			ToolCalls: []ollamaToolCall{
				{Function: ollamaFunction{Name: "search", Arguments: map[string]any{"q": "go"}}},
			},
		},
	}
	result := fromOllamaResponse(resp)
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Name != "search" {
		t.Errorf("expected tool name 'search', got %q", result.ToolCalls[0].Name)
	}
}

// TestOllamaCloudProvider_ToolChoiceDegradesToAuto documents that Ollama's
// native /api/chat has no documented tool_choice / forced-tool-call field
// (checked against https://docs.ollama.com/capabilities/tool-calling,
// 2026-08-23). Setting req.ToolChoice must not error and must not add any
// unrecognized field to the wire request — callers of this provider must
// keep a prose fallback.
func TestOllamaCloudProvider_ToolChoiceDegradesToAuto(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		resp := ollamaChatResponse{
			Model:   "gpt-oss:120b",
			Message: ollamaMessage{Role: "assistant", Content: "ok"},
			Done:    true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, err := newOllamaCloud(ollamaCloudConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-oss:120b",
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	_, err = p.Chat(context.Background(), ChatRequest{
		Messages:   []Message{{Role: "user", Content: "hi"}},
		ToolChoice: ToolChoiceTool("verdict"),
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(gotBody, &raw); err != nil {
		t.Fatalf("unmarshal sent body: %v", err)
	}
	if _, present := raw["tool_choice"]; present {
		t.Errorf("expected no tool_choice field in Ollama request, got %v", raw["tool_choice"])
	}
}
