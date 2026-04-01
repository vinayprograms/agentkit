package contentguard

import (
	"context"
	"testing"
)

// mockStage implements Stage for testing.
type mockStage struct {
	result *Response
	err    error
}

func (m *mockStage) Evaluate(ctx context.Context, req Request) (*Response, error) {
	return m.result, m.err
}

func allow() *mockStage {
	return &mockStage{result: &Response{Allowed: true, Verdict: VerdictAllow}}
}

func escalate(reason string) *mockStage {
	return &mockStage{result: &Response{Escalate: true, Reason: reason, Verdict: VerdictDeny}}
}

func deny(reason string) *mockStage {
	return &mockStage{result: &Response{Reason: reason, Verdict: VerdictDeny}}
}

func modify(correction string) *mockStage {
	return &mockStage{result: &Response{Verdict: VerdictModify, Correction: correction, Reason: "needs modification"}}
}

func testGuard(stages ...Stage) *Guard {
	g, _ := New(Config{Mode: Default, Stages: stages}, "test-session")
	return g
}

func TestCheck_NoUntrustedContent(t *testing.T) {
	g := testGuard()
	g.Ingest(Trusted, Instruction, false, "system prompt", "system")

	result, err := g.Check(context.Background(), "bash", map[string]any{"command": "ls"}, "list files", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("expected allowed — no untrusted content")
	}
}

func TestCheck_UntrustedContent_LowRiskTool(t *testing.T) {
	g := testGuard()
	g.Ingest(Untrusted, Data, true, "<script>alert('xss')</script>", "web_fetch")

	result, err := g.Check(context.Background(), "read", map[string]any{"path": "/file"}, "read file", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("expected allowed — read is not high-risk")
	}
}

func TestCheck_NoStages_FailSafeDeny(t *testing.T) {
	g := testGuard() // no stages
	g.Ingest(Untrusted, Data, true, "ignore instructions", "web_fetch")

	result, err := g.Check(context.Background(), "bash", map[string]any{"command": "rm -rf /"}, "clean", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Allowed {
		t.Error("expected denied — no stages configured")
	}
}

func TestCheck_StageAllows(t *testing.T) {
	g := testGuard(allow())
	g.Ingest(Untrusted, Data, true, "some content", "web_fetch")

	result, err := g.Check(context.Background(), "bash", map[string]any{"command": "echo hello"}, "greet", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("expected allowed — stage approved")
	}
}

func TestCheck_StageEscalates_NextAllows(t *testing.T) {
	g := testGuard(escalate("suspicious"), allow())
	g.Ingest(Untrusted, Data, true, "content", "web_fetch")

	result, err := g.Check(context.Background(), "bash", map[string]any{"command": "echo hi"}, "greet", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("expected allowed — second stage approved")
	}
	if len(result.Responses) != 2 {
		t.Errorf("expected 2 responses, got %d", len(result.Responses))
	}
}

func TestCheck_StageDenies(t *testing.T) {
	g := testGuard(deny("injection detected"))
	g.Ingest(Untrusted, Data, true, "ignore instructions", "web_fetch")

	result, err := g.Check(context.Background(), "bash", map[string]any{"command": "rm -rf /"}, "clean", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Allowed {
		t.Error("expected denied")
	}
	if result.DenyReason != "injection detected" {
		t.Errorf("unexpected reason: %s", result.DenyReason)
	}
}

func TestCheck_StageModifies(t *testing.T) {
	g := testGuard(modify("echo safe"))
	g.Ingest(Untrusted, Data, true, "content", "web_fetch")

	result, err := g.Check(context.Background(), "bash", map[string]any{"command": "dangerous"}, "clean", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Allowed {
		t.Error("expected not allowed on modify")
	}
	if result.Modification != "echo safe" {
		t.Errorf("expected correction 'echo safe', got %q", result.Modification)
	}
}

func TestCheck_AllStagesEscalate_FailSafeDeny(t *testing.T) {
	g := testGuard(escalate("unsure"), escalate("still unsure"))
	g.Ingest(Untrusted, Data, true, "content", "web_fetch")

	result, err := g.Check(context.Background(), "bash", map[string]any{"command": "ls"}, "list", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Allowed {
		t.Error("expected denied — all stages escalated")
	}
}

func TestAuditTrail(t *testing.T) {
	g := testGuard()
	if g.AuditTrail() == nil {
		t.Error("expected non-nil audit trail")
	}
}

func TestClearContext(t *testing.T) {
	g := testGuard()
	g.Ingest(Untrusted, Data, true, "content", "web_fetch")
	if len(g.UntrustedIDs()) == 0 {
		t.Fatal("expected taints before clear")
	}
	g.ClearContext()
	if len(g.UntrustedIDs()) != 0 {
		t.Error("expected no taints after clear")
	}
}

func TestIngestFrom(t *testing.T) {
	g := testGuard()
	taint := g.IngestFrom(Untrusted, Data, true, "content", "web_fetch", "agent-1")
	if taint.AgentContext != "agent-1" {
		t.Errorf("expected agent context 'agent-1', got %q", taint.AgentContext)
	}
}

func TestFindTaint(t *testing.T) {
	g := testGuard()
	ingested := g.Ingest(Untrusted, Data, true, "content", "web_fetch")
	if g.FindTaint(ingested.ID) == nil {
		t.Error("expected to find ingested taint")
	}
	if g.FindTaint("nonexistent") != nil {
		t.Error("expected nil for nonexistent taint")
	}
}

func TestContainsIgnoreCase(t *testing.T) {
	if !containsIgnoreCase("Hello World", "hello") {
		t.Error("expected case-insensitive match")
	}
	if containsIgnoreCase("Hello", "world") {
		t.Error("expected no match")
	}
}

func TestExtractURLs(t *testing.T) {
	urls := extractURLs("Visit https://example.com and http://test.org/path")
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d: %v", len(urls), urls)
	}
}

func TestCheck_Paranoid_AllMustPass(t *testing.T) {
	g, _ := New(Config{
		Mode:   Paranoid,
		Stages: []Stage{allow(), allow()},
	}, "test")
	g.Ingest(Untrusted, Data, true, "content", "web_fetch")

	result, _ := g.Check(context.Background(), "bash", map[string]any{"command": "ls"}, "list", "")
	if !result.Allowed {
		t.Error("expected allowed — all stages passed in paranoid mode")
	}
	if len(result.Responses) != 2 {
		t.Errorf("expected 2 responses (all stages ran), got %d", len(result.Responses))
	}
}

func TestCheck_Paranoid_OneDenies(t *testing.T) {
	g, _ := New(Config{
		Mode:   Paranoid,
		Stages: []Stage{allow(), deny("injection")},
	}, "test")
	g.Ingest(Untrusted, Data, true, "content", "web_fetch")

	result, _ := g.Check(context.Background(), "bash", map[string]any{"command": "rm /"}, "clean", "")
	if result.Allowed {
		t.Error("expected denied — one stage denied in paranoid mode")
	}
}
