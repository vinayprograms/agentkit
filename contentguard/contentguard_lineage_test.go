package contentguard

import (
	"testing"
)

func TestGetTaintLineage_Simple(t *testing.T) {
	g, err := New(Config{Mode: Default}, "test-session")
	if err != nil {
		t.Fatalf("failed to create guard: %v", err)
	}
	defer g.Close()

	// Add a simple untrusted taint
	taint := g.IngestWithLineage(Untrusted, Data, true, "malicious content", "tool:web_fetch", "", 42, nil)

	lineage := g.TaintLineage(taint.ID)
	if lineage == nil {
		t.Fatal("expected non-nil lineage")
	}

	if lineage.TaintID != taint.ID {
		t.Errorf("expected taint ID %s, got %s", taint.ID, lineage.TaintID)
	}
	if lineage.Trust != Untrusted {
		t.Errorf("expected trust untrusted, got %s", lineage.Trust)
	}
	if lineage.Source != "tool:web_fetch" {
		t.Errorf("expected source tool:web_fetch, got %s", lineage.Source)
	}
	if lineage.EventSeq != 42 {
		t.Errorf("expected event seq 42, got %d", lineage.EventSeq)
	}
	if len(lineage.TaintedBy) != 0 {
		t.Errorf("expected no parents, got %d", len(lineage.TaintedBy))
	}
}

func TestGetTaintLineage_WithParents(t *testing.T) {
	g, err := New(Config{Mode: Default}, "test-session")
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}
	defer g.Close()

	// Add parent taints (root sources)
	parent1 := g.IngestWithLineage(Untrusted, Data, true, "content from web", "tool:web_fetch", "", 10, nil)
	parent2 := g.IngestWithLineage(Untrusted, Data, true, "content from file", "tool:read", "", 15, nil)

	// Add a child taint that was influenced by both parents
	child := g.IngestWithLineage(Vetted, Instruction, false, "LLM response", "llm:response", "", 20, []string{parent1.ID, parent2.ID})

	lineage := g.TaintLineage(child.ID)
	if lineage == nil {
		t.Fatal("expected non-nil lineage")
	}

	if lineage.TaintID != child.ID {
		t.Errorf("expected taint ID %s, got %s", child.ID, lineage.TaintID)
	}
	if len(lineage.TaintedBy) != 2 {
		t.Fatalf("expected 2 parents, got %d", len(lineage.TaintedBy))
	}

	// Check parents
	foundParent1, foundParent2 := false, false
	for _, p := range lineage.TaintedBy {
		if p.TaintID == parent1.ID {
			foundParent1 = true
			if p.Source != "tool:web_fetch" {
				t.Errorf("parent1 source mismatch: %s", p.Source)
			}
		}
		if p.TaintID == parent2.ID {
			foundParent2 = true
			if p.Source != "tool:read" {
				t.Errorf("parent2 source mismatch: %s", p.Source)
			}
		}
	}
	if !foundParent1 {
		t.Error("parent1 not found in lineage")
	}
	if !foundParent2 {
		t.Error("parent2 not found in lineage")
	}
}

func TestGetTaintLineage_DeepChain(t *testing.T) {
	g, err := New(Config{Mode: Default}, "test-session")
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}
	defer g.Close()

	// Create a chain: grandparent -> parent -> child
	grandparent := g.IngestWithLineage(Untrusted, Data, true, "external data", "tool:web_fetch", "", 10, nil)
	parent := g.IngestWithLineage(Untrusted, Data, true, "processed data", "llm:response", "", 20, []string{grandparent.ID})
	child := g.IngestWithLineage(Untrusted, Data, true, "final output", "llm:response", "", 30, []string{parent.ID})

	lineage := g.TaintLineage(child.ID)
	if lineage == nil {
		t.Fatal("expected non-nil lineage")
	}

	// Check depth
	if lineage.Depth != 0 {
		t.Errorf("expected depth 0 for child, got %d", lineage.Depth)
	}
	if len(lineage.TaintedBy) != 1 {
		t.Fatalf("expected 1 parent for child, got %d", len(lineage.TaintedBy))
	}

	parentNode := lineage.TaintedBy[0]
	if parentNode.Depth != 1 {
		t.Errorf("expected depth 1 for parent, got %d", parentNode.Depth)
	}
	if len(parentNode.TaintedBy) != 1 {
		t.Fatalf("expected 1 grandparent, got %d", len(parentNode.TaintedBy))
	}

	grandparentNode := parentNode.TaintedBy[0]
	if grandparentNode.Depth != 2 {
		t.Errorf("expected depth 2 for grandparent, got %d", grandparentNode.Depth)
	}
	if grandparentNode.TaintID != grandparent.ID {
		t.Errorf("expected grandparent ID %s, got %s", grandparent.ID, grandparentNode.TaintID)
	}
}

func TestGetTaintLineage_NotFound(t *testing.T) {
	g, err := New(Config{Mode: Default}, "test-session")
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}
	defer g.Close()

	lineage := g.TaintLineage("nonexistent")
	if lineage != nil {
		t.Error("expected nil lineage for nonexistent taint")
	}
}

func TestGetCurrentUntrustedBlockIDs(t *testing.T) {
	g, err := New(Config{Mode: Default}, "test-session")
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}
	defer g.Close()

	// Add mixed taints
	g.Ingest(Trusted, Instruction, false, "system prompt", "system")
	g.Ingest(Untrusted, Data, true, "external 1", "tool:web")
	g.Ingest(Vetted, Instruction, false, "user prompt", "user")
	g.Ingest(Untrusted, Data, true, "external 2", "tool:read")

	ids := g.UntrustedIDs()
	if len(ids) != 2 {
		t.Errorf("expected 2 untrusted taints, got %d", len(ids))
	}
}
