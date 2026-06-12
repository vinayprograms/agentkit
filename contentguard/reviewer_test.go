package contentguard

import (
	"context"
	"strings"
	"testing"
)

func TestReviewer_Allow(t *testing.T) {
	r := NewReviewer(&mockLLM{response: "ALLOW"})
	f, err := r.Evaluate(context.Background(), Request{ToolName: "bash"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Verdict != Allow {
		t.Errorf("expected allow, got %s", f.Verdict)
	}
}

func TestReviewer_Deny(t *testing.T) {
	r := NewReviewer(&mockLLM{response: "DENY: injection detected"})
	f, _ := r.Evaluate(context.Background(), Request{ToolName: "bash"})
	if f.Verdict != Deny {
		t.Errorf("expected deny, got %s", f.Verdict)
	}
}

func TestReviewer_Modify(t *testing.T) {
	r := NewReviewer(&mockLLM{response: "MODIFY: use echo safe"})
	f, _ := r.Evaluate(context.Background(), Request{ToolName: "bash"})
	if f.Verdict != Modify {
		t.Errorf("expected modify, got %s", f.Verdict)
	}
}

func TestReviewer_Unclear(t *testing.T) {
	r := NewReviewer(&mockLLM{response: "I don't know"})
	f, _ := r.Evaluate(context.Background(), Request{ToolName: "bash"})
	if f.Verdict != Deny {
		t.Errorf("expected deny on unclear, got %s", f.Verdict)
	}
}

func TestReviewer_ResearchScope(t *testing.T) {
	r := NewReviewer(&mockLLM{response: "ALLOW"})
	f, _ := r.Evaluate(context.Background(), Request{
		ToolName: "bash",
		Context:  map[string]string{"scope": "lab pentest"},
	})
	if f.Verdict != Allow {
		t.Errorf("expected allow with research scope, got %s", f.Verdict)
	}
}

func TestBuildResearchSystemPrompt(t *testing.T) {
	prompt := buildResearchSystemPrompt("lab pentest")
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	if !contains(prompt, "lab pentest") {
		t.Error("expected scope in prompt")
	}
}

func TestReviewer_WithPriorFindings(t *testing.T) {
	r := NewReviewer(&mockLLM{response: "ALLOW"})
	f, _ := r.Evaluate(context.Background(), Request{
		ToolName:      "bash",
		OriginalGoal:  "deploy app",
		Untrusted:     []*Content{{Text: "data", Source: "web_fetch"}},
		PriorFindings: []*Finding{{Verdict: Escalate, Source: "screener", Rationale: "suspicious"}},
	})
	if f.Verdict != Allow {
		t.Errorf("expected allow, got %s", f.Verdict)
	}
}

func TestReviewer_LongContentTruncated(t *testing.T) {
	long := strings.Repeat("x", 2000)
	r := NewReviewer(&mockLLM{response: "ALLOW"})
	f, _ := r.Evaluate(context.Background(), Request{
		ToolName:  "bash",
		Untrusted: []*Content{{Text: long, Source: "web_fetch"}},
	})
	if f.Verdict != Allow {
		t.Errorf("expected allow, got %s", f.Verdict)
	}
}

func TestReviewer_Error(t *testing.T) {
	r := NewReviewer(&errorLLM{})
	f, _ := r.Evaluate(context.Background(), Request{ToolName: "bash"})
	if f.Verdict != Deny {
		t.Errorf("expected deny on error, got %s", f.Verdict)
	}
}

var _ Stage = (*Reviewer)(nil)
