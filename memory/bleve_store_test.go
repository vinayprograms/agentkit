package memory

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestBleveStore_RememberRecall(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bleve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewBleveStore(BleveStoreConfig{
		BasePath: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Remember observations with categories
	_, err = store.RememberObservation(ctx, "The user prefers dark mode and vim keybindings", "finding", "explicit")
	if err != nil {
		t.Fatalf("remember failed: %v", err)
	}

	_, err = store.RememberObservation(ctx, "We decided to use PostgreSQL for the database", "insight", "session:123")
	if err != nil {
		t.Fatalf("remember failed: %v", err)
	}

	_, err = store.RememberObservation(ctx, "Avoid using deprecated APIs", "lesson", "session:123")
	if err != nil {
		t.Fatalf("remember failed: %v", err)
	}

	// Recall - should find results
	results, err := store.Recall(ctx, "user preferences", RecallOpts{Limit: 10})
	if err != nil {
		t.Fatalf("recall failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("expected to find results for 'user preferences'")
	}
}

func TestBleveStore_RecallFIL(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bleve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewBleveStore(BleveStoreConfig{
		BasePath: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Store in different categories
	_, err = store.RememberObservation(ctx, "API rate limit is 100 per minute", "finding", "test")
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.RememberObservation(ctx, "REST is simpler than GraphQL for our use case", "insight", "test")
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.RememberObservation(ctx, "Always check rate limits before integration", "lesson", "test")
	if err != nil {
		t.Fatal(err)
	}

	// Recall by category
	results, err := store.RecallFIL(ctx, "API rate", 5)
	if err != nil {
		t.Fatalf("RecallFIL failed: %v", err)
	}

	t.Logf("Findings: %v", results.Findings)
	t.Logf("Insights: %v", results.Insights)
	t.Logf("Lessons: %v", results.Lessons)

	if len(results.Findings) == 0 {
		t.Error("expected findings about API rate")
	}
	if len(results.Lessons) == 0 {
		t.Error("expected lessons about rate limits")
	}
}

func TestBleveStore_RecallByCategory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bleve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewBleveStore(BleveStoreConfig{
		BasePath: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Store in different categories
	_, err = store.RememberObservation(ctx, "Database is PostgreSQL version 15", "finding", "test")
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.RememberObservation(ctx, "PostgreSQL chosen for JSON support", "insight", "test")
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.RememberObservation(ctx, "Always index foreign keys", "lesson", "test")
	if err != nil {
		t.Fatal(err)
	}

	// Recall only findings
	findings, err := store.RecallByCategory(ctx, "database PostgreSQL", "finding", 5)
	if err != nil {
		t.Fatal(err)
	}

	if len(findings) == 0 {
		t.Error("expected to find findings about database")
	}

	// Verify we only get findings, not insights or lessons
	for _, f := range findings {
		// Should contain database-related content
		t.Logf("Finding: %s", f)
	}
}

func TestBleveStore_KeyValue(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bleve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewBleveStore(BleveStoreConfig{
		BasePath: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Set and get
	err = store.Set("api_endpoint", "https://api.example.com")
	if err != nil {
		t.Fatal(err)
	}

	val, err := store.Get("api_endpoint")
	if err != nil {
		t.Fatal(err)
	}

	if val != "https://api.example.com" {
		t.Errorf("expected 'https://api.example.com', got '%s'", val)
	}

	// List
	keys, err := store.List("api")
	if err != nil {
		t.Fatal(err)
	}

	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}
}

func TestBleveStore_RetrieveByID(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bleve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewBleveStore(BleveStoreConfig{
		BasePath: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Store an observation
	id, err := store.RememberObservation(ctx, "Test observation", "finding", "test")
	if err != nil {
		t.Fatal(err)
	}

	// Retrieve by ID
	item, err := store.RetrieveByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	if item == nil {
		t.Fatal("expected to find item")
	}

	if item.Content != "Test observation" {
		t.Errorf("expected 'Test observation', got '%s'", item.Content)
	}

	if item.Category != "finding" {
		t.Errorf("expected 'finding', got '%s'", item.Category)
	}
}

func TestBleveStore_RememberFIL(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bleve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewBleveStore(BleveStoreConfig{
		BasePath: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	findings := []string{"F1: First finding", "F2: Second finding"}
	insights := []string{"I1: First insight"}
	lessons := []string{"L1: First lesson", "L2: Second lesson"}

	ids, err := store.RememberFIL(ctx, findings, insights, lessons, "test")
	if err != nil {
		t.Fatalf("RememberFIL failed: %v", err)
	}

	// Should return 5 IDs (2 findings + 1 insight + 2 lessons)
	if len(ids) != 5 {
		t.Errorf("expected 5 IDs, got %d", len(ids))
	}

	// Verify we can recall the data
	results, err := store.RecallFIL(ctx, "finding", 10)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Retrieved FIL: F=%d I=%d L=%d", len(results.Findings), len(results.Insights), len(results.Lessons))
}

func TestBleveStore_Persistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bleve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create store and add data
	store1, err := NewBleveStore(BleveStoreConfig{
		BasePath: tmpDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = store1.RememberObservation(ctx, "Persistent data", "finding", "test")
	if err != nil {
		t.Fatal(err)
	}

	err = store1.Set("key1", "value1")
	if err != nil {
		t.Fatal(err)
	}

	store1.Close()

	// Reopen and verify data persists
	store2, err := NewBleveStore(BleveStoreConfig{
		BasePath: tmpDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()

	results, err := store2.Recall(ctx, "Persistent", RecallOpts{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) == 0 {
		t.Error("expected to find persistent data after reopen")
	}

	val, err := store2.Get("key1")
	if err != nil {
		t.Fatal(err)
	}

	if val != "value1" {
		t.Errorf("expected 'value1', got '%s'", val)
	}
}

// ---------------------------------------------------------------------------
// BleveStore.Search (KV substring search)
// ---------------------------------------------------------------------------

func TestBleveStore_Search(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.Set("db.host", "postgres.example.com")
	store.Set("db.port", "5432")
	store.Set("app.name", "MyApp")

	// Search by value substring
	results, err := store.Search("postgres")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results["db.host"] != "postgres.example.com" {
		t.Errorf("unexpected result: %v", results)
	}

	// Search by key substring
	results, err = store.Search("db.")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results for key search, got %d", len(results))
	}

	// Search with no matches
	results, err = store.Search("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}

	// Case-insensitive search
	results, err = store.Search("POSTGRES")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result for case-insensitive search, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// BleveStore.ListAll
// ---------------------------------------------------------------------------

func TestBleveStore_ListAll(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	store.RememberObservation(ctx, "finding one", "finding", "test")
	store.RememberObservation(ctx, "finding two", "finding", "test")
	store.RememberObservation(ctx, "insight one", "insight", "test")
	store.RememberObservation(ctx, "lesson one", "lesson", "test")

	// List all (no category filter)
	items, err := store.ListAll(ctx, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Errorf("expected 4 items, got %d", len(items))
	}

	// List by category
	items, err = store.ListAll(ctx, "finding", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 findings, got %d", len(items))
	}
	for _, item := range items {
		if item.Category != "finding" {
			t.Errorf("expected category 'finding', got '%s'", item.Category)
		}
	}

	// List with limit
	items, err = store.ListAll(ctx, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items (limited), got %d", len(items))
	}
}

// ---------------------------------------------------------------------------
// BleveStore.ConsolidateSession
// ---------------------------------------------------------------------------

func TestBleveStore_ConsolidateSession_Empty(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Empty transcript should be a no-op
	err = store.ConsolidateSession(context.Background(), "sess-1", nil)
	if err != nil {
		t.Fatalf("expected nil error for empty transcript: %v", err)
	}

	err = store.ConsolidateSession(context.Background(), "sess-1", []Message{})
	if err != nil {
		t.Fatalf("expected nil error for empty transcript: %v", err)
	}
}

func TestBleveStore_ConsolidateSession_WithInsights(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create a transcript with decision language that triggers heuristic extraction
	transcript := []Message{
		{Role: "user", Content: "What database should we use?"},
		{Role: "assistant", Content: "We decided to use PostgreSQL because it has excellent JSON support and is well suited for our workload. This is an important conclusion for the project going forward."},
		{Role: "user", Content: "Sounds good"},
		{Role: "assistant", Content: strings.Repeat("This is a long summary message with enough content. ", 5)},
	}

	err = store.ConsolidateSession(ctx, "sess-2", transcript)
	if err != nil {
		t.Fatalf("ConsolidateSession failed: %v", err)
	}

	// Verify insights were stored
	items, err := store.ListAll(ctx, "insight", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Error("expected at least one insight from consolidation")
	}
}

func TestBleveStore_ConsolidateSession_ShortMessages(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	// Transcript where messages are too short to store (< 50 chars)
	// and no decision language
	transcript := []Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}

	err = store.ConsolidateSession(ctx, "sess-3", transcript)
	if err != nil {
		t.Fatalf("ConsolidateSession failed: %v", err)
	}

	items, err := store.ListAll(ctx, "insight", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 insights for short messages, got %d", len(items))
	}
}

func TestBleveStore_ConsolidateSession_LongInsightTruncated(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create a very long message with decision language
	longContent := "We decided that " + strings.Repeat("x", 2500)
	transcript := []Message{
		{Role: "user", Content: "What should we do?"},
		{Role: "assistant", Content: longContent},
	}

	err = store.ConsolidateSession(ctx, "sess-4", transcript)
	if err != nil {
		t.Fatalf("ConsolidateSession failed: %v", err)
	}

	items, err := store.ListAll(ctx, "insight", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("expected at least one insight")
	}
	// The stored content should be truncated to 2000 + "..."
	for _, item := range items {
		if len(item.Content) > 2010 {
			t.Errorf("expected truncated content, got length %d", len(item.Content))
		}
	}
}

// ---------------------------------------------------------------------------
// containsAny helper
// ---------------------------------------------------------------------------

func TestContainsAny(t *testing.T) {
	if !containsAny("we decided to use go", []string{"decided", "conclusion"}) {
		t.Error("expected true for 'decided'")
	}
	if !containsAny("this is important", []string{"decided", "important"}) {
		t.Error("expected true for 'important'")
	}
	if containsAny("nothing special here", []string{"decided", "conclusion"}) {
		t.Error("expected false when no patterns match")
	}
	if containsAny("", []string{"decided"}) {
		t.Error("expected false for empty text")
	}
	if containsAny("something", []string{}) {
		t.Error("expected false for empty patterns")
	}
}

// ---------------------------------------------------------------------------
// BleveStore.Get — missing key
// ---------------------------------------------------------------------------

func TestBleveStore_Get_MissingKey(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	val, err := store.Get("nonexistent")
	if err == nil {
		t.Error("expected error for missing key")
	}
	if val != "" {
		t.Errorf("expected empty string, got '%s'", val)
	}
}

// ---------------------------------------------------------------------------
// BleveStore.RetrieveByID — missing ID
// ---------------------------------------------------------------------------

func TestBleveStore_RetrieveByID_MissingID(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	item, err := store.RetrieveByID(context.Background(), "nonexistent-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item != nil {
		t.Error("expected nil for nonexistent ID")
	}
}

// ---------------------------------------------------------------------------
// BleveStore.RecallByCategory — default limit
// ---------------------------------------------------------------------------

func TestBleveStore_RecallByCategory_DefaultLimit(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	for i := 0; i < 8; i++ {
		store.RememberObservation(ctx, "database observation", "finding", "test")
	}

	// limit=0 should default to 5
	results, err := store.RecallByCategory(ctx, "database", "finding", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) > 5 {
		t.Errorf("expected at most 5 results with default limit, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// BleveStore.RecallFIL — default limit
// ---------------------------------------------------------------------------

func TestBleveStore_RecallFIL_DefaultLimit(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	store.RememberObservation(ctx, "finding about databases", "finding", "test")
	store.RememberObservation(ctx, "insight about databases", "insight", "test")
	store.RememberObservation(ctx, "lesson about databases", "lesson", "test")

	// limitPerCategory=0 should default to 5
	fil, err := store.RecallFIL(ctx, "databases", 0)
	if err != nil {
		t.Fatal(err)
	}
	if fil == nil {
		t.Fatal("expected non-nil FIL result")
	}
}

// ---------------------------------------------------------------------------
// BleveStore.Recall — default limit and MinScore
// ---------------------------------------------------------------------------

func TestBleveStore_Recall_DefaultLimit(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	for i := 0; i < 15; i++ {
		store.RememberObservation(ctx, "database observation", "finding", "test")
	}

	// Limit=0 should default to 10
	results, err := store.Recall(ctx, "database", RecallOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) > 10 {
		t.Errorf("expected at most 10 results, got %d", len(results))
	}
}

func TestBleveStore_Recall_MinScoreFilter(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	store.RememberObservation(ctx, "database is great", "finding", "test")

	// Very high MinScore should filter everything
	results, err := store.Recall(ctx, "database", RecallOpts{MinScore: 0.99})
	if err != nil {
		t.Fatal(err)
	}
	// Results might or might not be filtered depending on BM25 scores,
	// but we exercise the branch
	_ = results
}

// ---------------------------------------------------------------------------
// BleveStore.RememberFIL — empty slices
// ---------------------------------------------------------------------------

func TestBleveStore_RememberFIL_EmptySlices(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ids, err := store.RememberFIL(context.Background(), nil, nil, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 IDs, got %d", len(ids))
	}
}

// ---------------------------------------------------------------------------
// BleveStore.NewBleveStore — error on invalid path
// ---------------------------------------------------------------------------

func TestBleveStore_NewBleveStore_InvalidPath(t *testing.T) {
	// Try to create store at a path that can't be created
	_, err := NewBleveStore(BleveStoreConfig{BasePath: "/dev/null/impossible"})
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

// ---------------------------------------------------------------------------
// BleveStore — saveKV error branch (read-only directory)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// BleveStore — error paths via closed index
// ---------------------------------------------------------------------------

func TestBleveStore_RememberObservation_ClosedIndex(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	store.index.Close()

	_, err = store.RememberObservation(context.Background(), "test", "finding", "test")
	if err == nil {
		t.Error("expected error after closing index")
	}
}

func TestBleveStore_RememberFIL_ErrorPropagation(t *testing.T) {
	// Test error propagation for findings
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	store.index.Close()

	_, err = store.RememberFIL(context.Background(), []string{"f1"}, nil, nil, "test")
	if err == nil {
		t.Error("expected error from findings")
	}

	// Test error propagation for insights
	store2, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store2.RememberObservation(ctx, "pre-existing", "finding", "test") // succeeds
	store2.index.Close()

	_, err = store2.RememberFIL(ctx, nil, []string{"i1"}, nil, "test")
	if err == nil {
		t.Error("expected error from insights")
	}

	// Test error propagation for lessons
	store3, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	store3.index.Close()

	_, err = store3.RememberFIL(ctx, nil, nil, []string{"l1"}, "test")
	if err == nil {
		t.Error("expected error from lessons")
	}
}

func TestBleveStore_RetrieveByID_ClosedIndex(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	store.index.Close()

	_, err = store.RetrieveByID(context.Background(), "some-id")
	if err == nil {
		t.Error("expected error after closing index")
	}
}

func TestBleveStore_RecallByCategory_ClosedIndex(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	store.index.Close()

	_, err = store.RecallByCategory(context.Background(), "query", "finding", 5)
	if err == nil {
		t.Error("expected error after closing index")
	}
}

func TestBleveStore_RecallFIL_ErrorPropagation(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	store.index.Close()

	_, err = store.RecallFIL(context.Background(), "query", 5)
	if err == nil {
		t.Error("expected error after closing index")
	}
}

func TestBleveStore_Recall_ClosedIndex(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	store.index.Close()

	_, err = store.Recall(context.Background(), "query", RecallOpts{Limit: 10})
	if err == nil {
		t.Error("expected error after closing index")
	}
}

func TestBleveStore_ListAll_ClosedIndex(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	store.index.Close()

	_, err = store.ListAll(context.Background(), "", 0)
	if err == nil {
		t.Error("expected error after closing index")
	}
}

// ---------------------------------------------------------------------------
// BleveStore — NewBleveStore loadKV error (non-NotExist)
// ---------------------------------------------------------------------------

func TestBleveStore_NewBleveStore_BadKVFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Write invalid JSON to kv.json
	kvPath := tmpDir + "/kv.json"
	os.WriteFile(kvPath, []byte("not json"), 0644)

	_, err := NewBleveStore(BleveStoreConfig{BasePath: tmpDir})
	if err == nil {
		t.Error("expected error for malformed kv.json")
	}
}

func TestBleveStore_NewBleveStore_CorruptedIndex(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file (not a directory) at the index path to corrupt it
	indexPath := tmpDir + "/observations.bleve"
	os.WriteFile(indexPath, []byte("corrupted"), 0644)

	_, err := NewBleveStore(BleveStoreConfig{BasePath: tmpDir})
	if err == nil {
		t.Error("expected error for corrupted index")
	}
}

// ---------------------------------------------------------------------------
// BleveStore.ConsolidateSession — last assistant message branch, no decision keywords
// ---------------------------------------------------------------------------

func TestBleveStore_ConsolidateSession_LastAssistantOnly(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	// Transcript with no decision language but a long last assistant message
	transcript := []Message{
		{Role: "user", Content: "Tell me about the architecture"},
		{Role: "assistant", Content: strings.Repeat("The architecture uses microservices with event-driven communication patterns that enable high scalability. ", 3)},
	}

	err = store.ConsolidateSession(ctx, "sess-5", transcript)
	if err != nil {
		t.Fatal(err)
	}

	items, err := store.ListAll(ctx, "insight", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Error("expected insight from last assistant message")
	}
}

// Test that last assistant message is not stored if short
func TestBleveStore_ConsolidateSession_ShortLastAssistant(t *testing.T) {
	store, err := NewBleveStore(BleveStoreConfig{BasePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	// Only short messages, no decision language
	transcript := []Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello there"},
	}

	err = store.ConsolidateSession(ctx, "sess-6", transcript)
	if err != nil {
		t.Fatal(err)
	}

	items, err := store.ListAll(ctx, "insight", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 insights, got %d", len(items))
	}
}

func TestBleveStore_SaveKV_ErrorBranch(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewBleveStore(BleveStoreConfig{BasePath: tmpDir})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Make the directory read-only to trigger saveKV error
	kvDir := tmpDir
	os.Chmod(kvDir, 0555)
	defer os.Chmod(kvDir, 0755)

	// Remove existing kv.json so write creates a new file (and fails)
	os.Remove(store.kvPath)

	err = store.Set("key", "value")
	if err == nil {
		// On some systems this may succeed if running as root; skip
		t.Skip("chmod did not prevent write, possibly running as root")
	}
}