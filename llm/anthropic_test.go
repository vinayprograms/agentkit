package llm

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func anthropicMessageNewParamsForTest() anthropic.MessageNewParams {
	return anthropic.MessageNewParams{
		Model:     "claude-test",
		MaxTokens: 4096,
	}
}

// =============================================================================
// Anthropic Provider Tests
// =============================================================================

func TestAnthropicProvider_Creation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     anthropicConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: anthropicConfig{
				APIKey:    "test-key",
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 4096,
			},
			wantErr: false,
		},
		{
			name: "missing api key",
			cfg: anthropicConfig{
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 4096,
			},
			wantErr: true,
		},
		{
			name: "missing model",
			cfg: anthropicConfig{
				APIKey:    "test-key",
				MaxTokens: 4096,
			},
			wantErr: true,
		},
		{
			name: "missing max_tokens",
			cfg: anthropicConfig{
				APIKey: "test-key",
				Model:  "claude-3-5-sonnet-20241022",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newAnthropic(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("newAnthropic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Test Anthropic message conversion helpers

func TestToAnthropicMessages_SystemExtracted(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hello"},
	}
	system, messages := toAnthropicMessages(msgs)
	if system != "You are helpful." {
		t.Errorf("expected system prompt extracted, got %q", system)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message (user only), got %d", len(messages))
	}
}

func TestToAnthropicMessages_ToolCalls(t *testing.T) {
	msgs := []Message{
		{Role: "assistant", Content: "Let me check.", ToolCalls: []ToolCallResponse{
			{ID: "tc-1", Name: "search", Args: map[string]any{"q": "go"}},
		}},
		{Role: "tool", ToolCallID: "tc-1", Content: `{"results": []}`},
	}
	_, messages := toAnthropicMessages(msgs)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
}

func TestToAnthropicMessages_NoSystem(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "Hello"},
	}
	system, messages := toAnthropicMessages(msgs)
	if system != "" {
		t.Errorf("expected no system prompt, got %q", system)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
}

func TestToAnthropicTools(t *testing.T) {
	tools := []ToolDef{
		{Name: "search", Description: "Search the web", Parameters: map[string]any{
			"properties": map[string]any{"q": map[string]any{"type": "string"}},
		}},
	}
	result := toAnthropicTools(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].OfTool.Name != "search" {
		t.Errorf("expected tool name 'search', got %q", result[0].OfTool.Name)
	}
}

func TestToAnthropicTools_Empty(t *testing.T) {
	result := toAnthropicTools(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 tools, got %d", len(result))
	}
}

func TestThinkingLevelToAnthropicBudget(t *testing.T) {
	// Config budget takes precedence
	budget := thinkingLevelToAnthropicBudget(ThinkingHigh, 20000)
	if budget != 20000 {
		t.Errorf("expected config budget 20000, got %d", budget)
	}

	// Default budgets
	budget = thinkingLevelToAnthropicBudget(ThinkingHigh, 0)
	if budget != 16000 {
		t.Errorf("expected high budget 16000, got %d", budget)
	}

	budget = thinkingLevelToAnthropicBudget(ThinkingMedium, 0)
	if budget != 8000 {
		t.Errorf("expected medium budget 8000, got %d", budget)
	}

	budget = thinkingLevelToAnthropicBudget(ThinkingLow, 0)
	if budget != 4000 {
		t.Errorf("expected low budget 4000, got %d", budget)
	}

	budget = thinkingLevelToAnthropicBudget(ThinkingOff, 0)
	if budget != 0 {
		t.Errorf("expected off budget 0, got %d", budget)
	}
}

func TestAnthropicAuthOptions_APIKey(t *testing.T) {
	cfg := anthropicConfig{APIKey: "sk-test"}
	opts := anthropicAuthOptions(cfg)
	if len(opts) != 1 {
		t.Errorf("expected 1 option for API key auth, got %d", len(opts))
	}
}

func TestAnthropicAuthOptions_OAuth(t *testing.T) {
	cfg := anthropicConfig{APIKey: "oauth-token", IsOAuthToken: true}
	opts := anthropicAuthOptions(cfg)
	if len(opts) != 2 {
		t.Errorf("expected 2 options for OAuth auth (bearer + beta header), got %d", len(opts))
	}
}

func TestApplyAnthropicThinking_Off(t *testing.T) {
	cfg := ThinkingConfig{Level: ThinkingOff}
	req := ChatRequest{Messages: []Message{{Role: "user", Content: "hello"}}}
	params := anthropicMessageNewParamsForTest()
	maxTokens := int64(4096)

	applyAnthropicThinking(cfg, req, &params, &maxTokens)

	// MaxTokens should be unchanged
	if maxTokens != 4096 {
		t.Errorf("expected maxTokens unchanged at 4096, got %d", maxTokens)
	}
}

func TestApplyAnthropicThinking_High(t *testing.T) {
	cfg := ThinkingConfig{Level: ThinkingHigh}
	req := ChatRequest{Messages: []Message{{Role: "user", Content: "prove P=NP"}}}
	params := anthropicMessageNewParamsForTest()
	maxTokens := int64(4096)

	applyAnthropicThinking(cfg, req, &params, &maxTokens)

	// MaxTokens should be bumped to accommodate thinking budget
	if maxTokens <= 4096 {
		t.Errorf("expected maxTokens increased for thinking, got %d", maxTokens)
	}
}
