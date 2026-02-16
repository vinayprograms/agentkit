package memory

import (
	"context"
	"testing"
)

func TestInMemoryStore_RememberObservation(t *testing.T) {
	store := NewInMemoryStore()

	ctx := context.Background()

	// Remember observations - now returns ID
	id, err := store.RememberObservation(ctx, "The user prefers dark mode", "finding", "explicit")
	if err != nil {
		t.Fatalf("remember failed: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty ID")
	}

	_, err = store.RememberObservation(ctx, "PostgreSQL is best for JSON", "insight", "session:123")
	if err != nil {
		t.Fatalf("remember failed: %v", err)
	}

	_, err = store.RememberObservation(ctx, "Always validate input", "lesson", "session:123")
	if err != nil {
		t.Fatalf("remember failed: %v", err)
	}

	// Recall - should find results
	results, err := store.Recall(ctx, "user preferences", RecallOpts{Limit: 10})
	if err != nil {
		t.Fatalf("recall failed: %v", err)
	}

	if len(results) < 1 {
		t.Error("expected at least 1 result")
	}

	// Verify the results have required fields
	for _, r := range results {
		if r.ID == "" {
			t.Error("result should have ID")
		}
		if r.Content == "" {
			t.Error("result should have content")
		}
		if r.Category == "" {
			t.Error("result should have category")
		}
		if r.Score < 0 || r.Score > 1 {
			t.Errorf("score should be 0-1, got %f", r.Score)
		}
	}
}

func TestInMemoryStore_RememberFIL(t *testing.T) {
	store := NewInMemoryStore()

	ctx := context.Background()

	// Store using RememberFIL
	ids, err := store.RememberFIL(ctx,
		[]string{"API rate limit is 100 per minute", "Database is PostgreSQL"},
		[]string{"REST is simpler than GraphQL"},
		[]string{"Always check rate limits"},
		"test",
	)
	if err != nil {
		t.Fatalf("remember FIL failed: %v", err)
	}

	if len(ids) != 4 {
		t.Errorf("expected 4 IDs, got %d", len(ids))
	}

	// Recall as FIL
	fil, err := store.RecallFIL(ctx, "API rate", 5)
	if err != nil {
		t.Fatalf("recall FIL failed: %v", err)
	}

	if fil == nil {
		t.Fatal("expected FIL result")
	}

	// Should have findings about API
	if len(fil.Findings) == 0 {
		t.Error("expected at least 1 finding")
	}

	t.Logf("Findings: %v", fil.Findings)
	t.Logf("Insights: %v", fil.Insights)
	t.Logf("Lessons: %v", fil.Lessons)
}

func TestInMemoryStore_RetrieveByID(t *testing.T) {
	store := NewInMemoryStore()

	ctx := context.Background()

	// Remember something
	id, err := store.RememberObservation(ctx, "Database uses PostgreSQL", "finding", "test")
	if err != nil {
		t.Fatalf("remember failed: %v", err)
	}

	// Retrieve by ID
	item, err := store.RetrieveByID(ctx, id)
	if err != nil {
		t.Fatalf("retrieve failed: %v", err)
	}
	if item == nil {
		t.Fatal("expected item")
	}

	if item.ID != id {
		t.Errorf("ID mismatch: got %s, want %s", item.ID, id)
	}
	if item.Content != "Database uses PostgreSQL" {
		t.Errorf("content mismatch: got %s", item.Content)
	}
	if item.Category != "finding" {
		t.Errorf("category mismatch: got %s", item.Category)
	}

	// Retrieve non-existent
	item, err = store.RetrieveByID(ctx, "non-existent")
	if err != nil {
		t.Fatalf("retrieve should not error for missing: %v", err)
	}
	if item != nil {
		t.Error("expected nil for non-existent ID")
	}
}

func TestInMemoryStore_RecallByCategory(t *testing.T) {
	store := NewInMemoryStore()

	ctx := context.Background()

	// Store mixed observations
	store.RememberObservation(ctx, "Database uses PostgreSQL", "finding", "test")
	store.RememberObservation(ctx, "Database should be indexed", "lesson", "test")
	store.RememberObservation(ctx, "Database performance is good", "insight", "test")

	// Recall only findings
	findings, err := store.RecallByCategory(ctx, "database", "finding", 5)
	if err != nil {
		t.Fatalf("recall by category failed: %v", err)
	}

	if len(findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(findings))
	}

	// Recall only lessons
	lessons, err := store.RecallByCategory(ctx, "database", "lesson", 5)
	if err != nil {
		t.Fatalf("recall by category failed: %v", err)
	}

	if len(lessons) != 1 {
		t.Errorf("expected 1 lesson, got %d", len(lessons))
	}
}

func TestInMemoryStore_KeyValue(t *testing.T) {
	store := NewInMemoryStore()

	// Set and get
	err := store.Set("user.name", "Alice")
	if err != nil {
		t.Fatalf("set failed: %v", err)
	}

	value, err := store.Get("user.name")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if value != "Alice" {
		t.Errorf("expected 'Alice', got '%s'", value)
	}

	// List
	store.Set("user.email", "alice@example.com")
	store.Set("project.name", "MyProject")

	keys, err := store.List("user.")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}

	// Search
	results, err := store.Search("example.com")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}