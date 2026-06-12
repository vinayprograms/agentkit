package memory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vinayprograms/agentkit/llm"
)

// mockModel implements llm.Model for testing the Extractor.
type mockModel struct {
	response string
	err      error
	lastReq  llm.ChatRequest
}

func (m *mockModel) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	m.lastReq = req
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

// ---------------------------------------------------------------------------
// InMemoryStore.Get — missing key returns empty string
// ---------------------------------------------------------------------------

func TestInMemoryStore_Get_MissingKey(t *testing.T) {
	store := NewInMemoryStore()

	val, err := store.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get should not error for missing key: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for missing key, got '%s'", val)
	}
}

// ---------------------------------------------------------------------------
// InMemoryStore.RecallByCategory — limit=0 default and empty query
// ---------------------------------------------------------------------------

func TestInMemoryStore_RecallByCategory_DefaultLimit(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	// Store more than 5 findings containing the word "database"
	for i := 0; i < 8; i++ {
		store.RememberObservation(ctx, "database observation number "+string(rune('A'+i)), "finding", "test")
	}

	// limit=0 should default to 5
	results, err := store.RecallByCategory(ctx, "database", "finding", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) > 5 {
		t.Errorf("expected at most 5 results with default limit, got %d", len(results))
	}
}

func TestInMemoryStore_RecallByCategory_EmptyQuery(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.RememberObservation(ctx, "some finding", "finding", "test")

	// Empty query has no terms, so nothing matches
	results, err := store.RecallByCategory(ctx, "", "finding", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(results))
	}
}

func TestInMemoryStore_RecallByCategory_EmptyStore(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	results, err := store.RecallByCategory(ctx, "anything", "finding", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil for empty store, got %v", results)
	}
}

// ---------------------------------------------------------------------------
// InMemoryStore.RecallFIL — error path coverage (no real error from InMemory,
// but we exercise the default limitPerCategory branch)
// ---------------------------------------------------------------------------

func TestInMemoryStore_RecallFIL_DefaultLimit(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.RememberObservation(ctx, "finding about databases", "finding", "test")
	store.RememberObservation(ctx, "insight about databases", "insight", "test")
	store.RememberObservation(ctx, "lesson about databases", "lesson", "test")

	// limitPerCategory=0 should default to 5
	fil, err := store.RecallFIL(ctx, "databases", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fil == nil {
		t.Fatal("expected non-nil FIL result")
	}
	if len(fil.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(fil.Findings))
	}
}

// ---------------------------------------------------------------------------
// InMemoryStore.Recall — TimeRange filter, MinScore, default limit
// ---------------------------------------------------------------------------

func TestInMemoryStore_Recall_TimeRangeFilter(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.RememberObservation(ctx, "database observation early", "finding", "test")

	// Use a time range that excludes everything (far in the past)
	past := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	pastEnd := time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC)

	results, err := store.Recall(ctx, "database", RecallOpts{
		Limit:     10,
		TimeRange: &TimeRange{Start: past, End: pastEnd},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results with past time range, got %d", len(results))
	}

	// Use a time range that includes everything
	now := time.Now()
	results, err = store.Recall(ctx, "database", RecallOpts{
		Limit:     10,
		TimeRange: &TimeRange{Start: now.Add(-1 * time.Hour), End: now.Add(1 * time.Hour)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected results with inclusive time range")
	}
}

func TestInMemoryStore_Recall_MinScoreFilter(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	store.RememberObservation(ctx, "database is great", "finding", "test")

	// High MinScore should filter out low-scoring results
	results, err := store.Recall(ctx, "database", RecallOpts{
		Limit:    10,
		MinScore: 0.99,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Score for single term match is 0.1, so everything is filtered
	if len(results) != 0 {
		t.Errorf("expected 0 results with high min score, got %d", len(results))
	}
}

func TestInMemoryStore_Recall_DefaultLimit(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	// Store 15 items all containing "database"
	for i := 0; i < 15; i++ {
		store.RememberObservation(ctx, "database item", "finding", "test")
	}

	// Limit=0 should default to 10
	results, err := store.Recall(ctx, "database", RecallOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) > 10 {
		t.Errorf("expected at most 10 results with default limit, got %d", len(results))
	}
}

func TestInMemoryStore_Recall_EmptyStore(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	results, err := store.Recall(ctx, "anything", RecallOpts{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil for empty store, got %v", results)
	}
}

// ---------------------------------------------------------------------------
// InMemoryStore.RememberFIL — exercise all three category loops
// ---------------------------------------------------------------------------

func TestInMemoryStore_RememberFIL_EmptySlices(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	ids, err := store.RememberFIL(ctx, nil, nil, nil, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 IDs for empty slices, got %d", len(ids))
	}
}

// ---------------------------------------------------------------------------
// parseFIL — uncovered branch: malformed JSON (end <= start)
// ---------------------------------------------------------------------------

func TestParseFIL_NoBraces(t *testing.T) {
	f, i, l := parseFIL("no braces here")
	if f != nil || i != nil || l != nil {
		t.Errorf("expected nil slices, got f=%v i=%v l=%v", f, i, l)
	}
}

func TestParseFIL_MalformedBraces(t *testing.T) {
	// end brace before start brace
	f, i, l := parseFIL("} something {")
	if f != nil || i != nil || l != nil {
		t.Errorf("expected nil slices, got f=%v i=%v l=%v", f, i, l)
	}
}

func TestParseFIL_MarkdownCodeBlockEmpty(t *testing.T) {
	// Code block with no JSON content
	f, i, l := parseFIL("```\n```")
	if f != nil || i != nil || l != nil {
		t.Errorf("expected nil slices for empty code block, got f=%v i=%v l=%v", f, i, l)
	}
}

func TestParseFIL_OnlyOpenBrace(t *testing.T) {
	f, i, l := parseFIL("{")
	if f != nil || i != nil || l != nil {
		t.Errorf("expected nil slices, got f=%v i=%v l=%v", f, i, l)
	}
}

func TestParseFIL_ValidBracesInvalidJSON(t *testing.T) {
	// Has { and } but content is not valid JSON
	f, i, l := parseFIL("{not valid json}")
	if f != nil || i != nil || l != nil {
		t.Errorf("expected nil slices, got f=%v i=%v l=%v", f, i, l)
	}
}

func TestParseFIL_MarkdownBlockWithPreamble(t *testing.T) {
	// Markdown code block with preamble text before the block - starts with ```
	// but has preamble on first line before ```
	input := "```json\n{\"findings\":[\"f1\"],\"insights\":[],\"lessons\":[]}\n```"
	f, _, _ := parseFIL(input)
	if len(f) != 1 || f[0] != "f1" {
		t.Errorf("findings = %v, want [f1]", f)
	}
}

func TestExtractor_Extract_WithSource(t *testing.T) {
	m := &mockModel{response: `{"findings":["f"]}`}
	ext := NewExtractor(m)

	_, _, _, err := ext.Extract(context.Background(),
		"This is a sufficiently long text that exceeds fifty characters easily",
		WithSource("research-step"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	user := m.lastReq.Messages[len(m.lastReq.Messages)-1].Content
	if !strings.Contains(user, "Source: research-step") {
		t.Errorf("expected source label in prompt, got %q", user)
	}
}

func TestExtractor_Extract_Truncation(t *testing.T) {
	m := &mockModel{response: `{"findings":["f"]}`}
	ext := NewExtractor(m)

	long := strings.Repeat("x", 5000)
	_, _, _, err := ext.Extract(context.Background(), long, WithMaxInputChars(100))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	user := m.lastReq.Messages[len(m.lastReq.Messages)-1].Content
	if !strings.Contains(user, "[truncated]") {
		t.Errorf("expected truncation marker, got len %d", len(user))
	}
	if len(user) > 200 {
		t.Errorf("expected truncated input, got %d chars", len(user))
	}
}

func TestExtractor_Extract_DefaultTruncation(t *testing.T) {
	m := &mockModel{response: `{"findings":["f"]}`}
	ext := NewExtractor(m)

	long := strings.Repeat("y", 9000)
	if _, _, _, err := ext.Extract(context.Background(), long); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	user := m.lastReq.Messages[len(m.lastReq.Messages)-1].Content
	if !strings.Contains(user, "[truncated]") {
		t.Error("expected default truncation at 4000 chars")
	}
}
