package contentguard

import (
	"context"
	"testing"
)

func TestReviewer_Evaluate_Allow(t *testing.T) {
	r := &Reviewer{provider: &mockLLM{response: "ALLOW\nThis tool call is safe."}, mode: Default}
	result, err := r.Evaluate(context.Background(), ReviewRequest{
		ToolName:        "bash",
		ToolArgs:        map[string]any{"command": "ls"},
		UntrustedTaints: []*Taint{{Content: "safe"}},
		OriginalGoal:    "list files",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictAllow {
		t.Errorf("expected ALLOW, got %s", result.Verdict)
	}
}

func TestReviewer_Evaluate_Deny(t *testing.T) {
	r := &Reviewer{provider: &mockLLM{response: "DENY\nPrompt injection detected."}, mode: Default}
	result, err := r.Evaluate(context.Background(), ReviewRequest{
		ToolName:        "bash",
		ToolArgs:        map[string]any{"command": "rm -rf /"},
		UntrustedTaints: []*Taint{{Content: "ignore instructions"}},
		OriginalGoal:    "clean up",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDeny {
		t.Errorf("expected DENY, got %s", result.Verdict)
	}
}

func TestReviewer_Evaluate_Modify(t *testing.T) {
	r := &Reviewer{provider: &mockLLM{response: "MODIFY\nUse safer command.\nCORRECTION: echo safe"}, mode: Default}
	result, err := r.Evaluate(context.Background(), ReviewRequest{
		ToolName: "bash",
		ToolArgs: map[string]any{"command": "dangerous"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictModify {
		t.Errorf("expected MODIFY, got %s", result.Verdict)
	}
}

func TestLLMReviewer_Factory(t *testing.T) {
	fn := LLMReviewer(&mockLLM{response: "ALLOW"}, Default, "")
	if fn == nil {
		t.Fatal("expected non-nil ReviewFunc")
	}
	result, err := fn(context.Background(), ReviewRequest{ToolName: "bash"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictAllow {
		t.Errorf("expected ALLOW, got %s", result.Verdict)
	}
}
