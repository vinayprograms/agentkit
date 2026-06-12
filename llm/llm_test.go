// Package llm provides LLM provider interfaces and implementations.
package llm

import (
	"context"
	"strings"
	"testing"
)

// R4.1.1: Define Chat method
func TestProvider_ChatMethod(t *testing.T) {
	provider := newMock()
	provider.SetResponse("Hello from LLM")

	resp, err := provider.Chat(context.Background(), ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}

	if resp.Content != "Hello from LLM" {
		t.Errorf("expected 'Hello from LLM', got %s", resp.Content)
	}
}

// R4.1.3: Support tool definitions
func TestProvider_ToolDefinitions(t *testing.T) {
	provider := newMock()
	provider.SetToolCall("read", map[string]interface{}{"path": "/test.txt"})

	resp, err := provider.Chat(context.Background(), ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "Read the test file"},
		},
		Tools: []ToolDef{
			{
				Name:        "read",
				Description: "Read a file",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type": "string",
						},
					},
				},
			},
		},
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
}

// R4.1.4: Parse tool calls from response
func TestProvider_ParseToolCalls(t *testing.T) {
	provider := newMock()
	provider.SetToolCall("write", map[string]interface{}{
		"path":    "/output.txt",
		"content": "hello",
	})

	resp, _ := provider.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "Write a file"}},
		Tools:    []ToolDef{{Name: "write"}},
	})

	tc := resp.ToolCalls[0]
	if tc.Args["path"] != "/output.txt" {
		t.Errorf("expected path '/output.txt', got %v", tc.Args["path"])
	}
	if tc.Args["content"] != "hello" {
		t.Errorf("expected content 'hello', got %v", tc.Args["content"])
	}
}

// Test multiple tool calls
func TestProvider_MultipleToolCalls(t *testing.T) {
	provider := newMock()
	provider.SetToolCalls([]ToolCallResponse{
		{ID: "1", Name: "read", Args: map[string]interface{}{"path": "/a.txt"}},
		{ID: "2", Name: "read", Args: map[string]interface{}{"path": "/b.txt"}},
	})

	resp, _ := provider.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "Read two files"}},
		Tools:    []ToolDef{{Name: "read"}},
	})

	if len(resp.ToolCalls) != 2 {
		t.Errorf("expected 2 tool calls, got %d", len(resp.ToolCalls))
	}
}

// Test tool result message
func TestProvider_ToolResultMessage(t *testing.T) {
	provider := newMock()
	provider.SetResponse("File contains: hello world")

	resp, err := provider.Chat(context.Background(), ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "Read the file"},
			{Role: "assistant", Content: "", ToolCalls: []ToolCallResponse{
				{ID: "tc1", Name: "read", Args: map[string]interface{}{"path": "/test.txt"}},
			}},
			{Role: "tool", ToolCallID: "tc1", Content: "hello world"},
		},
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}

	if !strings.Contains(resp.Content, "hello world") {
		t.Errorf("response should reference tool result: %s", resp.Content)
	}
}

// Test token counting
func TestProvider_TokenCounts(t *testing.T) {
	provider := newMock()
	provider.SetResponse("Response")
	provider.SetTokenCounts(100, 50)

	resp, _ := provider.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})

	if resp.InputTokens != 100 {
		t.Errorf("expected input tokens 100, got %d", resp.InputTokens)
	}
	if resp.OutputTokens != 50 {
		t.Errorf("expected output tokens 50, got %d", resp.OutputTokens)
	}
}

// Test stop reason
func TestProvider_StopReason(t *testing.T) {
	provider := newMock()
	provider.SetResponse("Done")
	provider.SetStopReason("end_turn")

	resp, _ := provider.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})

	if resp.StopReason != "end_turn" {
		t.Errorf("expected stop reason 'end_turn', got %s", resp.StopReason)
	}
}

// Test max tokens configuration
func TestProvider_MaxTokens(t *testing.T) {
	provider := newMock()
	provider.SetResponse("Response")

	_, err := provider.Chat(context.Background(), ChatRequest{
		Messages:  []Message{{Role: "user", Content: "Hello"}},
		MaxTokens: 4096,
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}

	if provider.LastRequest().MaxTokens != 4096 {
		t.Errorf("expected max tokens 4096, got %d", provider.LastRequest().MaxTokens)
	}
}

// R4.2.1-R4.2.6: Provider adapter tests
// Note: These test the adapter interface, actual Provider integration is tested via integration tests

func TestProviderAdapter_ConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid anthropic config",
			config: Config{
				Service:   "anthropic",
				Model:     "claude-3-5-sonnet-20241022",
				APIKey:    "test-key",
				MaxTokens: 4096,
			},
			wantErr: false,
		},
		{
			name: "valid openai config",
			config: Config{
				Service:   "openai",
				Model:     "gpt-4o",
				APIKey:    "test-key",
				MaxTokens: 4096,
			},
			wantErr: false,
		},
		{
			name: "valid google config",
			config: Config{
				Service:   "google",
				Model:     "gemini-1.5-pro",
				APIKey:    "test-key",
				MaxTokens: 4096,
			},
			wantErr: false,
		},
		{
			name: "valid groq config",
			config: Config{
				Service:   "groq",
				Model:     "llama-3.1-70b-versatile",
				APIKey:    "test-key",
				MaxTokens: 4096,
			},
			wantErr: false,
		},
		{
			name: "valid mistral config",
			config: Config{
				Service:   "mistral",
				Model:     "mistral-large-latest",
				APIKey:    "test-key",
				MaxTokens: 4096,
			},
			wantErr: false,
		},
		{
			name: "missing provider",
			config: Config{
				Model:  "claude-3-5-sonnet-20241022",
				APIKey: "test-key",
			},
			wantErr: true,
		},
		{
			name: "missing model",
			config: Config{
				Service:   "anthropic",
				APIKey:    "test-key",
				MaxTokens: 4096,
			},
			wantErr: true,
		},
		{
			name: "missing api key",
			config: Config{
				Service: "anthropic",
				Model:   "claude-3-5-sonnet-20241022",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestNew_AllProviders tests that all supported providers can be instantiated
func TestNew_AllProviders(t *testing.T) {
	providers := []struct {
		name  string
		model string
	}{
		{"anthropic", "claude-3-5-sonnet-20241022"},
		{"openai", "gpt-4o"},
		{"google", "gemini-1.5-pro"},
		{"groq", "llama-3.1-70b-versatile"},
		{"mistral", "mistral-large-latest"},
		{"xai", "grok-2"},
		{"openrouter", "anthropic/claude-3-opus"},
		{"ollama-local", "llama3"},
		{"lmstudio", "local-model"},
	}

	for _, p := range providers {
		t.Run(p.name, func(t *testing.T) {
			cfg := Config{
				Service:   p.name,
				Model:     p.model,
				APIKey:    "test-key-for-" + p.name,
				MaxTokens: 4096,
			}

			provider, err := New(cfg)
			if err != nil {
				t.Fatalf("New(%s) failed: %v", p.name, err)
			}
			if provider == nil {
				t.Errorf("New(%s) returned nil provider", p.name)
			}
		})
	}
}

// TestNew_UnsupportedProvider tests that unsupported providers return an error
func TestNew_UnsupportedProvider(t *testing.T) {
	cfg := Config{
		Service:   "unsupported-provider",
		Model:     "some-model",
		APIKey:    "test-key",
		MaxTokens: 4096,
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("expected error for unsupported provider")
	}
}

// TestInferService tests model name to provider inference
func TestInferService(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		// Anthropic
		{"claude-3-5-sonnet-20241022", "anthropic"},
		{"claude-3-opus-20240229", "anthropic"},
		{"claude-3-haiku-20240307", "anthropic"},
		{"Claude-3-Sonnet", "anthropic"}, // case insensitive

		// OpenAI
		{"gpt-4o", "openai"},
		{"gpt-4-turbo", "openai"},
		{"gpt-3.5-turbo", "openai"},
		{"o1-preview", "openai"},
		{"o1-mini", "openai"},
		{"o3-mini", "openai"},
		{"chatgpt-4o-latest", "openai"},

		// Google
		{"gemini-1.5-pro", "google"},
		{"gemini-1.5-flash", "google"},
		{"gemini-2.0-flash", "google"},
		{"gemma-2-9b", "google"},

		// Mistral
		{"mistral-large-latest", "mistral"},
		{"mistral-small-latest", "mistral"},
		{"mixtral-8x7b-instruct", "mistral"},
		{"codestral-latest", "mistral"},
		{"pixtral-12b", "mistral"},

		// xAI
		{"grok-2", "xai"},
		{"grok-beta", "xai"},

		// Unknown
		{"unknown-model", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := InferService(tt.model)
			if result != tt.expected {
				t.Errorf("InferService(%q) = %q, want %q", tt.model, result, tt.expected)
			}
		})
	}
}

// TestNew_InferredProvider tests that provider can be inferred from model name
func TestNew_InferredProvider(t *testing.T) {
	tests := []struct {
		model   string
		wantErr bool
	}{
		{"claude-3-5-sonnet-20241022", false},
		{"gpt-4o", false},
		{"gemini-1.5-pro", false},
		{"mistral-large-latest", false},
		{"unknown-model-xyz", true}, // cannot infer
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			cfg := Config{
				// Provider not set - should be inferred
				Model:     tt.model,
				APIKey:    "test-key",
				MaxTokens: 4096,
			}

			_, err := New(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_MaxTokensMandatory(t *testing.T) {
	cfg := Config{
		Service: "anthropic",
		Model:   "claude-3-5-sonnet-20241022",
		APIKey:  "test-key",
		// MaxTokens not set - should fail validation
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for missing max_tokens")
	}

	// Now set it and should pass
	cfg.MaxTokens = 4096
	err = cfg.Validate()
	if err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

// mockModel is a mock LLM provider for testing.
type mockModel struct {
	response     string
	toolCalls    []ToolCallResponse
	stopReason   string
	inputTokens  int
	outputTokens int
	lastRequest  *ChatRequest
	err          error
	callCount    int
	ChatFunc     func(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}

func newMock() *mockModel {
	return &mockModel{stopReason: "end_turn"}
}

func (p *mockModel) SetResponse(content string) { p.response = content }
func (p *mockModel) SetToolCall(name string, args map[string]interface{}) {
	p.toolCalls = []ToolCallResponse{{ID: "tc-1", Name: name, Args: args}}
}
func (p *mockModel) SetToolCalls(calls []ToolCallResponse) { p.toolCalls = calls }
func (p *mockModel) SetTokenCounts(input, output int)      { p.inputTokens = input; p.outputTokens = output }
func (p *mockModel) SetStopReason(reason string)           { p.stopReason = reason }
func (p *mockModel) SetError(err error)                    { p.err = err }
func (p *mockModel) LastRequest() *ChatRequest             { return p.lastRequest }
func (p *mockModel) CallCount() int                        { return p.callCount }

func (p *mockModel) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	p.callCount++
	p.lastRequest = &req

	if p.ChatFunc != nil {
		return p.ChatFunc(ctx, req)
	}
	if p.err != nil {
		return nil, p.err
	}

	hasToolResult := false
	for _, msg := range req.Messages {
		if msg.Role == "tool" {
			hasToolResult = true
			break
		}
	}

	if hasToolResult {
		return &ChatResponse{
			Content:      p.response,
			StopReason:   p.stopReason,
			InputTokens:  p.inputTokens,
			OutputTokens: p.outputTokens,
		}, nil
	}

	return &ChatResponse{
		Content:      p.response,
		ToolCalls:    p.toolCalls,
		StopReason:   p.stopReason,
		InputTokens:  p.inputTokens,
		OutputTokens: p.outputTokens,
	}, nil
}
