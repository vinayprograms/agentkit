package contentguard

import (
	"context"
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
		ToolName:   "bash",
		Exceptions: map[string]string{"scope": "lab pentest"},
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

var _ Stage = (*Reviewer)(nil)
