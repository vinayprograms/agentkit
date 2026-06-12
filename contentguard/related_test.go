package contentguard

import (
	"context"
	"testing"
	"time"
)

func TestCheck_ReportsRelatedContent(t *testing.T) {
	g := testGuard(denyStage("injection"))
	c1 := g.Ingest(Untrusted, Data, true, "external one", "web_fetch")
	c2 := g.Ingest(Untrusted, Data, true, "external two", "web_fetch")

	result, _ := g.Check(context.Background(), "bash", map[string]any{"command": "ls"}, "list")

	ids := map[string]Trust{}
	for _, r := range result.Related {
		ids[r.ID] = r.Trust
	}
	if ids[c1.ID] != Untrusted || ids[c2.ID] != Untrusted {
		t.Fatalf("expected both untrusted blocks in Related, got %+v", result.Related)
	}
}

func TestCheck_NoRelatedWhenAllTrusted(t *testing.T) {
	g := testGuard(allowStage())
	g.Ingest(Trusted, Instruction, false, "system", "system")

	result, _ := g.Check(context.Background(), "bash", map[string]any{"command": "ls"}, "list")
	if len(result.Related) != 0 {
		t.Fatalf("expected no related content, got %+v", result.Related)
	}
}

type latencyStage struct{ d time.Duration }

func (s *latencyStage) Evaluate(ctx context.Context, req Request) (*Finding, error) {
	return &Finding{Verdict: Deny, Source: "mock", Latency: s.d}, nil
}

func TestFinding_LatencyPropagates(t *testing.T) {
	g := testGuard(&latencyStage{d: 42 * time.Millisecond})
	g.Ingest(Untrusted, Data, true, "content", "web_fetch")

	result, _ := g.Check(context.Background(), "bash", map[string]any{"command": "ls"}, "list")

	var found bool
	for _, f := range result.Findings {
		if f.Source == "mock" {
			found = true
			if f.Latency != 42*time.Millisecond {
				t.Fatalf("expected 42ms latency, got %v", f.Latency)
			}
		}
	}
	if !found {
		t.Fatal("mock finding not present in result")
	}
}

func TestExportedShannonEntropy(t *testing.T) {
	if e := ShannonEntropy(""); e != 0 {
		t.Fatalf("empty string should have zero entropy, got %v", e)
	}
	low := ShannonEntropy("aaaaaaaaaa")
	high := ShannonEntropy("Kx9vLmQpR2hYnT5wZ3jBcF8aS1dE0uOyI4bNqCrVfM7eWxPgJk2iU6")
	if !(low < high) {
		t.Fatalf("expected repetitive text below base64, got low=%v high=%v", low, high)
	}
}

func TestExportedIsHighEntropy(t *testing.T) {
	if IsHighEntropy("the quick brown fox jumps") {
		t.Fatal("plain english should not be high entropy")
	}
	if !IsHighEntropy("Kx9vLmQpR2hYnT5wZ3jBcF8aS1dE0uOyI4bNqCrVfM7eWxPgJk2iU6") {
		t.Fatal("base64 blob should be high entropy")
	}
}
