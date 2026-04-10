package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/vinayprograms/agentkit/llm"
)

// mockModel implements llm.Model for testing the Extractor.
type mockModel struct {
	response string
	err      error
}

func (m *mockModel) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &llm.ChatResponse{Content: m.response}, nil
}

// ---------------------------------------------------------------------------
// Extractor.Extract tests
// ---------------------------------------------------------------------------

func TestExtractor_Extract_ValidJSON(t *testing.T) {
	m := &mockModel{response: `{"findings":["fact1"],"insights":["ins1"],"lessons":["les1"]}`}
	ext := NewExtractor(m)

	f, i, l, err := ext.Extract(context.Background(), "This is a sufficiently long text that exceeds fifty characters easily")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f) != 1 || f[0] != "fact1" {
		t.Errorf("findings = %v, want [fact1]", f)
	}
	if len(i) != 1 || i[0] != "ins1" {
		t.Errorf("insights = %v, want [ins1]", i)
	}
	if len(l) != 1 || l[0] != "les1" {
		t.Errorf("lessons = %v, want [les1]", l)
	}
}

func TestExtractor_Extract_MarkdownCodeBlock(t *testing.T) {
	resp := "```json\n{\"findings\":[\"f1\"],\"insights\":[],\"lessons\":[\"l1\"]}\n```"
	m := &mockModel{response: resp}
	ext := NewExtractor(m)

	f, _, l, err := ext.Extract(context.Background(), "This is a sufficiently long text that exceeds fifty characters easily")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f) != 1 || f[0] != "f1" {
		t.Errorf("findings = %v, want [f1]", f)
	}
	if len(l) != 1 || l[0] != "l1" {
		t.Errorf("lessons = %v, want [l1]", l)
	}
}

func TestExtractor_Extract_NilModel(t *testing.T) {
	ext := NewExtractor(nil)

	f, i, l, err := ext.Extract(context.Background(), "This is a sufficiently long text that exceeds fifty characters easily")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != nil || i != nil || l != nil {
		t.Errorf("expected nil slices, got f=%v i=%v l=%v", f, i, l)
	}
}

func TestExtractor_Extract_ShortText(t *testing.T) {
	m := &mockModel{response: `{"findings":["x"]}`}
	ext := NewExtractor(m)

	f, i, l, err := ext.Extract(context.Background(), "short")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != nil || i != nil || l != nil {
		t.Errorf("expected nil slices for short text, got f=%v i=%v l=%v", f, i, l)
	}
}

func TestExtractor_Extract_LLMError(t *testing.T) {
	m := &mockModel{err: errors.New("llm down")}
	ext := NewExtractor(m)

	f, i, l, err := ext.Extract(context.Background(), "This is a sufficiently long text that exceeds fifty characters easily")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if f != nil || i != nil || l != nil {
		t.Errorf("expected nil slices on LLM error, got f=%v i=%v l=%v", f, i, l)
	}
}

func TestExtractor_Extract_InvalidJSON(t *testing.T) {
	m := &mockModel{response: "not json at all"}
	ext := NewExtractor(m)

	f, i, l, err := ext.Extract(context.Background(), "This is a sufficiently long text that exceeds fifty characters easily")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != nil || i != nil || l != nil {
		t.Errorf("expected nil slices for invalid JSON, got f=%v i=%v l=%v", f, i, l)
	}
}

// ---------------------------------------------------------------------------
// parseFIL tests
// ---------------------------------------------------------------------------

func TestParseFIL_ValidJSON(t *testing.T) {
	f, i, l := parseFIL(`{"findings":["a","b"],"insights":["c"],"lessons":[]}`)
	if len(f) != 2 {
		t.Errorf("findings len = %d, want 2", len(f))
	}
	if len(i) != 1 {
		t.Errorf("insights len = %d, want 1", len(i))
	}
	if len(l) != 0 {
		t.Errorf("lessons len = %d, want 0", len(l))
	}
}

func TestParseFIL_ExtraWhitespace(t *testing.T) {
	f, i, l := parseFIL(`
	  {"findings": ["x"], "insights": [], "lessons": ["y"]}
	`)
	if len(f) != 1 || f[0] != "x" {
		t.Errorf("findings = %v, want [x]", f)
	}
	if len(i) != 0 {
		t.Errorf("insights = %v, want []", i)
	}
	if len(l) != 1 || l[0] != "y" {
		t.Errorf("lessons = %v, want [y]", l)
	}
}

func TestParseFIL_MarkdownCodeBlock(t *testing.T) {
	input := "```json\n{\"findings\":[\"f\"],\"insights\":[\"i\"],\"lessons\":[\"l\"]}\n```"
	f, i, l := parseFIL(input)
	if len(f) != 1 || f[0] != "f" {
		t.Errorf("findings = %v, want [f]", f)
	}
	if len(i) != 1 || i[0] != "i" {
		t.Errorf("insights = %v, want [i]", i)
	}
	if len(l) != 1 || l[0] != "l" {
		t.Errorf("lessons = %v, want [l]", l)
	}
}

func TestParseFIL_InvalidJSON(t *testing.T) {
	f, i, l := parseFIL("this is not json")
	if f != nil || i != nil || l != nil {
		t.Errorf("expected nil slices for invalid JSON, got f=%v i=%v l=%v", f, i, l)
	}
}

func TestParseFIL_EmptyObject(t *testing.T) {
	f, i, l := parseFIL(`{"findings":[],"insights":[],"lessons":[]}`)
	if f == nil || i == nil || l == nil {
		t.Errorf("expected non-nil empty slices, got f=%v i=%v l=%v", f, i, l)
	}
	if len(f) != 0 || len(i) != 0 || len(l) != 0 {
		t.Errorf("expected empty slices, got f=%v i=%v l=%v", f, i, l)
	}
}

// ---------------------------------------------------------------------------
// InMemoryStore.ListAll tests
// ---------------------------------------------------------------------------

func TestInMemoryStore_ListAll_Empty(t *testing.T) {
	store := NewInMemoryStore()
	items, err := store.ListAll(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestInMemoryStore_ListAll_AllCategories(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.RememberObservation(ctx, "finding1", "finding", "test")
	store.RememberObservation(ctx, "insight1", "insight", "test")
	store.RememberObservation(ctx, "lesson1", "lesson", "test")

	items, err := store.ListAll(ctx, "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

func TestInMemoryStore_ListAll_FilterByCategory(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.RememberObservation(ctx, "finding1", "finding", "test")
	store.RememberObservation(ctx, "finding2", "finding", "test")
	store.RememberObservation(ctx, "insight1", "insight", "test")

	items, err := store.ListAll(ctx, "finding", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 findings, got %d", len(items))
	}
	for _, item := range items {
		if item.Category != "finding" {
			t.Errorf("expected category 'finding', got '%s'", item.Category)
		}
	}
}

func TestInMemoryStore_ListAll_WithLimit(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	for j := 0; j < 5; j++ {
		store.RememberObservation(ctx, "item", "finding", "test")
	}

	items, err := store.ListAll(ctx, "", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items (limit), got %d", len(items))
	}
}

// ---------------------------------------------------------------------------
// InMemoryStore no-op methods
// ---------------------------------------------------------------------------

func TestInMemoryStore_ConsolidateSession_NoOp(t *testing.T) {
	store := NewInMemoryStore()
	err := store.ConsolidateSession(context.Background(), "session-1", []Message{
		{Role: "user", Content: "hello"},
	})
	if err != nil {
		t.Errorf("ConsolidateSession should return nil, got: %v", err)
	}
}

func TestInMemoryStore_Close_NoOp(t *testing.T) {
	store := NewInMemoryStore()
	err := store.Close()
	if err != nil {
		t.Errorf("Close should return nil, got: %v", err)
	}
}

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