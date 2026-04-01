package contentguard

import (
	"context"
	"testing"
)

type mockStage struct {
	finding *Finding
}

func (m *mockStage) Evaluate(ctx context.Context, req Request) (*Finding, error) {
	return m.finding, nil
}

func allowStage() *mockStage {
	return &mockStage{finding: &Finding{Verdict: Allow, Source: "mock"}}
}

func escalateStage(reason string) *mockStage {
	return &mockStage{finding: &Finding{Verdict: Escalate, Rationale: reason, Source: "mock"}}
}

func denyStage(reason string) *mockStage {
	return &mockStage{finding: &Finding{Verdict: Deny, Rationale: reason, Source: "mock"}}
}

func modifyStage(suggestion string) *mockStage {
	return &mockStage{finding: &Finding{Verdict: Modify, Rationale: suggestion, Source: "mock"}}
}

func testGuard(stages ...Stage) *Guard {
	g, _ := New(stages, Escalatory(), nil, "test-session")
	return g
}

func TestCheck_NoUntrustedContent(t *testing.T) {
	g := testGuard()
	g.Ingest(Trusted, Instruction, false, "system prompt", "system")

	result, err := g.Check(context.Background(), "bash", map[string]any{"command": "ls"}, "list files")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != Allow {
		t.Errorf("expected allow, got %s", result.Verdict)
	}
}

func TestCheck_LowRiskTool(t *testing.T) {
	g := testGuard()
	g.Ingest(Untrusted, Data, true, "external content", "web_fetch")

	result, _ := g.Check(context.Background(), "read", map[string]any{"path": "/file"}, "read file")
	if result.Verdict != Allow {
		t.Errorf("expected allow for low-risk tool, got %s", result.Verdict)
	}
}

func TestCheck_NoStages_FailSafeDeny(t *testing.T) {
	g := testGuard() // no stages
	g.Ingest(Untrusted, Data, true, "ignore instructions", "web_fetch")

	result, _ := g.Check(context.Background(), "bash", map[string]any{"command": "rm -rf /"}, "clean")
	if result.Verdict != Deny {
		t.Errorf("expected deny with no stages, got %s", result.Verdict)
	}
}

func TestCheck_StageAllows(t *testing.T) {
	g := testGuard(allowStage())
	g.Ingest(Untrusted, Data, true, "content", "web_fetch")

	result, _ := g.Check(context.Background(), "bash", map[string]any{"command": "echo hi"}, "greet")
	if result.Verdict != Allow {
		t.Errorf("expected allow, got %s", result.Verdict)
	}
}

func TestCheck_StageDenies(t *testing.T) {
	g := testGuard(denyStage("injection detected"))
	g.Ingest(Untrusted, Data, true, "content", "web_fetch")

	result, _ := g.Check(context.Background(), "bash", map[string]any{"command": "rm /"}, "clean")
	if result.Verdict != Deny {
		t.Errorf("expected deny, got %s", result.Verdict)
	}
	if result.Rationale != "injection detected" {
		t.Errorf("unexpected rationale: %s", result.Rationale)
	}
}

func TestCheck_StageModifies(t *testing.T) {
	g := testGuard(modifyStage("use echo safe instead"))
	g.Ingest(Untrusted, Data, true, "content", "web_fetch")

	result, _ := g.Check(context.Background(), "bash", map[string]any{"command": "dangerous"}, "clean")
	if result.Verdict != Modify {
		t.Errorf("expected modify, got %s", result.Verdict)
	}
}

func TestCheck_EscalatesThenAllows(t *testing.T) {
	g := testGuard(escalateStage("unsure"), allowStage())
	g.Ingest(Untrusted, Data, true, "content", "web_fetch")

	result, _ := g.Check(context.Background(), "bash", map[string]any{"command": "echo"}, "greet")
	if result.Verdict != Allow {
		t.Errorf("expected allow from second stage, got %s", result.Verdict)
	}
	// deterministic + 2 stage findings
	if len(result.Findings) < 3 {
		t.Errorf("expected at least 3 findings, got %d", len(result.Findings))
	}
}

func TestCheck_AllEscalate_FailSafeDeny(t *testing.T) {
	g := testGuard(escalateStage("unsure"), escalateStage("still unsure"))
	g.Ingest(Untrusted, Data, true, "content", "web_fetch")

	result, _ := g.Check(context.Background(), "bash", map[string]any{"command": "ls"}, "list")
	if result.Verdict != Deny {
		t.Errorf("expected deny when all escalate, got %s", result.Verdict)
	}
}

func TestCheck_Paranoid_AllPass(t *testing.T) {
	g, _ := New([]Stage{allowStage(), allowStage()}, Paranoid(), nil, "test")
	g.Ingest(Untrusted, Data, true, "content", "web_fetch")

	result, _ := g.Check(context.Background(), "bash", map[string]any{"command": "ls"}, "list")
	if result.Verdict != Allow {
		t.Errorf("expected allow in paranoid with all pass, got %s", result.Verdict)
	}
}

func TestCheck_Paranoid_OneDenies(t *testing.T) {
	g, _ := New([]Stage{allowStage(), denyStage("injection")}, Paranoid(), nil, "test")
	g.Ingest(Untrusted, Data, true, "content", "web_fetch")

	result, _ := g.Check(context.Background(), "bash", map[string]any{"command": "rm"}, "clean")
	if result.Verdict != Deny {
		t.Errorf("expected deny in paranoid when one denies, got %s", result.Verdict)
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

func TestFindTaint(t *testing.T) {
	g := testGuard()
	taint := g.Ingest(Untrusted, Data, true, "content", "web_fetch")
	if g.FindTaint(taint.ID) == nil {
		t.Error("expected to find taint")
	}
	if g.FindTaint("nonexistent") != nil {
		t.Error("expected nil for nonexistent")
	}
}

func TestContainsIgnoreCase(t *testing.T) {
	if !containsIgnoreCase("Hello World", "hello") {
		t.Error("expected match")
	}
	if containsIgnoreCase("Hello", "world") {
		t.Error("expected no match")
	}
}

func TestExtractURLs(t *testing.T) {
	urls := extractURLs("Visit https://example.com and http://test.org")
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d", len(urls))
	}
}
