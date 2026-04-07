package contentguard

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/vinayprograms/agentkit/llm"
)

type mockLLM struct{ response string }

func (m *mockLLM) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{Content: m.response, InputTokens: 10, OutputTokens: 5}, nil
}

type errorLLM struct{}

func (e *errorLLM) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, fmt.Errorf("LLM unavailable")
}

func TestScreener_Safe(t *testing.T) {
	s := NewScreener(&mockLLM{response: "NO"})
	f, err := s.Evaluate(context.Background(), Request{
		ToolName: "bash", Untrusted: []*Content{{Text: "safe"}},
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

func TestScreener_WithScope(t *testing.T) {
	s := NewScreener(&mockLLM{response: "NO"})
	f, _ := s.Evaluate(context.Background(), Request{
		ToolName:  "bash",
		Untrusted: []*Content{{Text: "data"}},
		Context:   map[string]string{"scope": "lab pentest"},
	})
	if f.Verdict != Allow {
		t.Errorf("expected allow, got %s", f.Verdict)
	}
}

func TestScreener_WithPriorFindings(t *testing.T) {
	s := NewScreener(&mockLLM{response: "YES"})
	f, _ := s.Evaluate(context.Background(), Request{
		ToolName:      "bash",
		PriorFindings: []*Finding{{Verdict: Escalate, Source: "deterministic", Rationale: "high_risk_tool:bash"}},
	})
	if f.Verdict != Escalate {
		t.Errorf("expected escalate, got %s", f.Verdict)
	}
}

func TestScreener_LongContentTruncated(t *testing.T) {
	long := strings.Repeat("x", 3000)
	s := NewScreener(&mockLLM{response: "NO"})
	f, _ := s.Evaluate(context.Background(), Request{
		ToolName:  "bash",
		Untrusted: []*Content{{Text: long}},
	})
	if f.Verdict != Allow {
		t.Errorf("expected allow, got %s", f.Verdict)
	}
}

func TestScreener_Error(t *testing.T) {
	s := NewScreener(&errorLLM{})
	f, _ := s.Evaluate(context.Background(), Request{ToolName: "bash"})
	if f.Verdict != Escalate {
		t.Errorf("expected escalate on error, got %s", f.Verdict)
	}
}

var _ Stage = (*Screener)(nil)
