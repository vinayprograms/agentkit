package contentguard

import (
	"testing"
)

func TestLineage_Simple(t *testing.T) {
	g, _ := New(nil, Escalatory(), Defaults())
	defer g.Close()

	c := g.IngestWithLineage(Untrusted, Data, true, "malicious content", "tool:web_fetch", nil)

	if len(c.Origins) != 0 {
		t.Errorf("expected no parents, got %d", len(c.Origins))
	}
}

func TestLineage_WithParents(t *testing.T) {
	g, _ := New(nil, Escalatory(), Defaults())
	defer g.Close()

	parent1 := g.IngestWithLineage(Untrusted, Data, true, "content from web", "tool:web_fetch", nil)
	parent2 := g.IngestWithLineage(Untrusted, Data, true, "content from file", "tool:read", nil)

	child := g.IngestWithLineage(Vetted, Instruction, false, "LLM response", "llm:response", []string{parent1.ID, parent2.ID})

	if len(child.Origins) != 2 {
		t.Fatalf("expected 2 parents, got %d", len(child.Origins))
	}

	foundParent1, foundParent2 := false, false
	for _, p := range child.Origins {
		if p == parent1 {
			foundParent1 = true
			if p.Source != "tool:web_fetch" {
				t.Errorf("parent1 source mismatch: %s", p.Source)
			}
		}
		if p == parent2 {
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

func TestLineage_DeepChain(t *testing.T) {
	g, _ := New(nil, Escalatory(), Defaults())
	defer g.Close()

	grandparent := g.IngestWithLineage(Untrusted, Data, true, "external data", "tool:web_fetch", nil)
	parent := g.IngestWithLineage(Untrusted, Data, true, "processed data", "llm:response", []string{grandparent.ID})
	child := g.IngestWithLineage(Untrusted, Data, true, "final output", "llm:response", []string{parent.ID})

	// child → parent → grandparent
	if len(child.Origins) != 1 {
		t.Fatalf("expected 1 parent, got %d", len(child.Origins))
	}
	if child.Origins[0] != parent {
		t.Error("expected child's parent to be parent")
	}

	if len(parent.Origins) != 1 {
		t.Fatalf("expected 1 grandparent, got %d", len(parent.Origins))
	}
	if parent.Origins[0] != grandparent {
		t.Error("expected parent's parent to be grandparent")
	}

	if len(grandparent.Origins) != 0 {
		t.Errorf("expected no parents for grandparent, got %d", len(grandparent.Origins))
	}
}

func TestLineage_DAG(t *testing.T) {
	g, _ := New(nil, Escalatory(), Defaults())
	defer g.Close()

	// A is ancestor of both B and C; D is child of both B and C
	// A appears via two paths but is the same pointer
	a := g.Ingest(Untrusted, Data, true, "root content", "tool:web_fetch")
	b := g.IngestWithLineage(Untrusted, Data, true, "derived B", "llm:response", []string{a.ID})
	c := g.IngestWithLineage(Untrusted, Data, true, "derived C", "tool:read", []string{a.ID})
	d := g.IngestWithLineage(Untrusted, Data, true, "final", "llm:response", []string{b.ID, c.ID})

	if len(d.Origins) != 2 {
		t.Fatalf("expected 2 parents, got %d", len(d.Origins))
	}

	// Both paths lead to the same pointer
	if b.Origins[0] != a {
		t.Error("expected B's parent to be A")
	}
	if c.Origins[0] != a {
		t.Error("expected C's parent to be A")
	}
	if b.Origins[0] != c.Origins[0] {
		t.Error("expected same pointer for shared ancestor A")
	}
}

func TestFind_AfterResolvedLineage(t *testing.T) {
	g, _ := New(nil, Escalatory(), Defaults())
	defer g.Close()

	parent := g.Ingest(Untrusted, Data, true, "content", "web_fetch")
	child := g.IngestWithLineage(Untrusted, Data, true, "derived", "llm", []string{parent.ID})

	found := g.Find(child.ID)
	if found == nil {
		t.Fatal("expected to find child")
	}
	if len(found.Origins) != 1 || found.Origins[0] != parent {
		t.Error("expected resolved parent pointer on found content")
	}
}

func TestUntrustedIDs(t *testing.T) {
	g, _ := New(nil, Escalatory(), Defaults())
	defer g.Close()

	g.Ingest(Trusted, Instruction, false, "system prompt", "system")
	g.Ingest(Untrusted, Data, true, "external 1", "tool:web")
	g.Ingest(Vetted, Instruction, false, "user prompt", "user")
	g.Ingest(Untrusted, Data, true, "external 2", "tool:read")

	ids := g.UntrustedIDs()
	if len(ids) != 2 {
		t.Errorf("expected 2 untrusted entries, got %d", len(ids))
	}
}
