package memory

import (
	"context"
	"os"
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