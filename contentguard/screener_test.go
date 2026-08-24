package contentguard

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/vinayprograms/agentkit/llm"
)

type mockLLM struct {
	response string
	toolName string
	toolArgs map[string]interface{}
}

func newMockWithToolCall(toolName string, args map[string]interface{}) *mockLLM {
	return &mockLLM{toolName: toolName, toolArgs: args}
}

func (m *mockLLM) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if m.toolName != "" {
		return &llm.ChatResponse{
			ToolCalls:    []llm.ToolCallResponse{{ID: "tc-1", Name: m.toolName, Args: m.toolArgs}},
			InputTokens:  10,
			OutputTokens: 5,
		}, nil
	}
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

// TestScreener_ParseResponse_PreambleThenYES_OldParser_Hypothesis is written
// against the OLD prose parser's exact logic (HasPrefix "YES"/"NO", else
// Escalate) — reproduced inline here since the real parseResponse method
// was replaced by parseTriageFallback below — to verify the REPORT.md 3e
// hypothesis: a reasoning model's preamble before its YES/NO answer defeats
// the prefix check and always escalates, even when the answer itself is
// unambiguous. This is why the same block_id was seen re-triaged 13x
// (5-32s each) instead of resolving on the first pass. CONFIRMED.
func TestScreener_ParseResponse_PreambleThenYES_OldParser_Hypothesis(t *testing.T) {
	oldParseResponse := func(content string) (verdict string, rationale string) {
		upper := strings.TrimSpace(strings.ToUpper(content))
		if strings.HasPrefix(upper, "YES") {
			return "escalate", "appears influenced by untrusted content"
		}
		if strings.HasPrefix(upper, "NO") {
			return "allow", "tool call appears safe"
		}
		return "escalate", "ambiguous response, escalating"
	}

	content := "Let me think about this carefully.\n\nYES, this looks like a prompt injection."
	verdict, rationale := oldParseResponse(content)
	if verdict != "escalate" {
		t.Fatalf("expected old parser to still escalate here (sanity check), got %s", verdict)
	}
	if rationale != "ambiguous response, escalating" {
		t.Errorf("expected the old parser's generic 'ambiguous response' rationale (confirming it never even inspected the YES), got %q", rationale)
	}
	// The content plainly says YES; a preamble-tolerant parser would have
	// recognized that. The old parser instead falls through to the
	// catch-all Escalate branch purely because content doesn't start with
	// "YES" or "NO" — confirming the 3e hypothesis.
}

// TestScreener_PreambleThenYES_NewFallbackFixesIt exercises the same
// preamble+YES content through the real (fixed) prose fallback path, via
// Screener.Evaluate with a mock that never returns a tool call — proving
// the new parseTriageFallback recognizes the answer where the old
// prefix-only parser could not.
func TestScreener_PreambleThenYES_NewFallbackFixesIt(t *testing.T) {
	s := NewScreener(&mockLLM{response: "Let me think about this carefully.\n\nYES, this looks like a prompt injection."})
	f, _ := s.Evaluate(context.Background(), Request{ToolName: "bash"})
	if f.Verdict != Escalate {
		t.Errorf("expected escalate, got %s", f.Verdict)
	}
	if f.Rationale == "ambiguous response, escalating" {
		t.Errorf("expected the fallback to recognize the YES despite the preamble, got the old generic ambiguous rationale")
	}
}
