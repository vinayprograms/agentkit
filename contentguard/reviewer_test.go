package contentguard

import (
	"context"
	"testing"
)

func TestReviewer_Allow(t *testing.T) {
	r := LLMReviewer(&mockLLM{response: "ALLOW"}, Default, "")
	result, err := r.Evaluate(context.Background(), Request{
		ToolName: "bash",
		Taints:   []*Taint{{Content: "safe"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("expected allowed")
	}
}

func TestReviewer_Deny(t *testing.T) {
	r := LLMReviewer(&mockLLM{response: "DENY: injection detected"}, Default, "")
	result, err := r.Evaluate(context.Background(), Request{ToolName: "bash"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Allowed || result.Escalate {
		t.Error("expected deny")
	}
	if result.Verdict != VerdictDeny {
		t.Errorf("expected DENY verdict, got %s", result.Verdict)
	}
}

func TestReviewer_Modify(t *testing.T) {
	r := LLMReviewer(&mockLLM{response: "MODIFY: use echo safe"}, Default, "")
	result, err := r.Evaluate(context.Background(), Request{ToolName: "bash"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictModify {
		t.Errorf("expected MODIFY verdict, got %s", result.Verdict)
	}
	if result.Correction == "" {
		t.Error("expected correction")
	}
}

func TestReviewer_UnclearResponse(t *testing.T) {
	r := LLMReviewer(&mockLLM{response: "I cannot determine safety"}, Default, "")
	result, err := r.Evaluate(context.Background(), Request{ToolName: "bash"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDeny {
		t.Error("expected deny on unclear response")
	}
}

// Verify Reviewer satisfies Stage interface
var _ Stage = (*Reviewer)(nil)
