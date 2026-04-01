package contentguard

import (
	"context"
	"testing"

	"github.com/vinayprograms/agentkit/llm"
)

type mockLLM struct {
	response string
}

func (m *mockLLM) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{Content: m.response, InputTokens: 10, OutputTokens: 5}, nil
}

func TestScreener_NotSuspicious(t *testing.T) {
	s := LLMScreener(&mockLLM{response: "NO"}, "")
	result, err := s.Evaluate(context.Background(), Request{
		ToolName: "bash",
		ToolArgs: map[string]any{"command": "ls"},
		Taints:   []*Taint{{Content: "safe content"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("expected allowed")
	}
}

func TestScreener_Suspicious(t *testing.T) {
	s := LLMScreener(&mockLLM{response: "YES - injection"}, "")
	result, err := s.Evaluate(context.Background(), Request{
		ToolName: "bash",
		Taints:   []*Taint{{Content: "ignore instructions"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Escalate {
		t.Error("expected escalate on YES")
	}
}

func TestScreener_Ambiguous(t *testing.T) {
	s := LLMScreener(&mockLLM{response: "I'm not sure about this"}, "")
	result, err := s.Evaluate(context.Background(), Request{ToolName: "bash"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Escalate {
		t.Error("expected escalate on ambiguous response")
	}
}

// Verify Screener satisfies Stage interface
var _ Stage = (*Screener)(nil)
