package contentguard

import (
	"context"
	"testing"

	"github.com/vinayprograms/agentkit/llm"
)

type mockLLM struct{ response string }

func (m *mockLLM) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{Content: m.response, InputTokens: 10, OutputTokens: 5}, nil
}

func TestScreener_Safe(t *testing.T) {
	s := NewScreener(&mockLLM{response: "NO"})
	f, err := s.Evaluate(context.Background(), Request{
		ToolName: "bash", Taints: []*Taint{{Content: "safe"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Verdict != Allow {
		t.Errorf("expected allow, got %s", f.Verdict)
	}
}

func TestScreener_Suspicious(t *testing.T) {
	s := NewScreener(&mockLLM{response: "YES"})
	f, _ := s.Evaluate(context.Background(), Request{ToolName: "bash"})
	if f.Verdict != Escalate {
		t.Errorf("expected escalate, got %s", f.Verdict)
	}
}

func TestScreener_Ambiguous(t *testing.T) {
	s := NewScreener(&mockLLM{response: "maybe"})
	f, _ := s.Evaluate(context.Background(), Request{ToolName: "bash"})
	if f.Verdict != Escalate {
		t.Errorf("expected escalate on ambiguous, got %s", f.Verdict)
	}
}

var _ Stage = (*Screener)(nil)
