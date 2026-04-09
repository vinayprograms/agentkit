package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestScratchpadWrite(t *testing.T) {
	store := NewInMemoryStore()
	tool := ScratchpadWrite(store, false)

	args, err := Validate(tool.Parameters(), map[string]any{"key": "api_url", "value": "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "api_url") {
		t.Errorf("expected result to mention the key, got %q", result)
	}

	// Verify value was stored
	val, err := store.Get("api_url")
	if err != nil {
		t.Fatal(err)
	}
	if val != "https://example.com" {
		t.Errorf("expected stored value 'https://example.com', got %q", val)
	}
}

func TestScratchpadRead_ExistingKey(t *testing.T) {
	store := NewInMemoryStore()
	store.Set("color", "blue")

	tool := ScratchpadRead(store, false)
	args, err := Validate(tool.Parameters(), map[string]any{"key": "color"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result != "blue" {
		t.Errorf("expected 'blue', got %q", result)
	}
}

func TestScratchpadRead_MissingKey(t *testing.T) {
	store := NewInMemoryStore()
	tool := ScratchpadRead(store, false)

	args, err := Validate(tool.Parameters(), map[string]any{"key": "missing"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing key")
	}
}

func TestScratchpadList(t *testing.T) {
	store := NewInMemoryStore()
	store.Set("api_url", "https://example.com")
	store.Set("api_key", "secret123")
	store.Set("db_host", "localhost")

	tool := ScratchpadList(store, false)
	args, err := Validate(tool.Parameters(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "api_url") || !strings.Contains(result, "api_key") || !strings.Contains(result, "db_host") {
		t.Errorf("expected all keys in result, got %q", result)
	}
}

func TestScratchpadList_WithFilter(t *testing.T) {
	store := NewInMemoryStore()
	store.Set("api_url", "https://example.com")
	store.Set("api_key", "secret123")
	store.Set("db_host", "localhost")

	tool := ScratchpadList(store, false)
	args, err := Validate(tool.Parameters(), map[string]any{"filter": "api"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "api_url") || !strings.Contains(result, "api_key") {
		t.Errorf("expected api keys in result, got %q", result)
	}
	if strings.Contains(result, "db_host") {
		t.Errorf("did not expect db_host in filtered result, got %q", result)
	}
}

func TestScratchpadList_Empty(t *testing.T) {
	store := NewInMemoryStore()
	tool := ScratchpadList(store, false)

	args, err := Validate(tool.Parameters(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result != "no keys found" {
		t.Errorf("expected 'no keys found', got %q", result)
	}
}

func TestScratchpadSearch(t *testing.T) {
	store := NewInMemoryStore()
	store.Set("api_url", "https://example.com/api")
	store.Set("db_host", "localhost:5432")
	store.Set("note", "the api is rate-limited")

	tool := ScratchpadSearch(store, false)
	args, err := Validate(tool.Parameters(), map[string]any{"query": "api"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	// Should match api_url (key match) and note (value match)
	if !strings.Contains(result, "api_url") {
		t.Errorf("expected api_url in search results, got %q", result)
	}
	if !strings.Contains(result, "note") {
		t.Errorf("expected note in search results (value contains 'api'), got %q", result)
	}
}

func TestScratchpadSearch_NoResults(t *testing.T) {
	store := NewInMemoryStore()
	store.Set("key1", "value1")

	tool := ScratchpadSearch(store, false)
	args, err := Validate(tool.Parameters(), map[string]any{"query": "zzz"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result != "no matching entries found" {
		t.Errorf("expected 'no matching entries found', got %q", result)
	}
}

// --- FileMemoryStore tests ---

func TestFileMemoryStore_NewCreatesStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")
	store := NewFileMemoryStore(path)
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestFileMemoryStore_GetSetListSearch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")
	store := NewFileMemoryStore(path)

	// Set values
	if err := store.Set("api_url", "https://example.com"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("api_key", "secret123"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("db_host", "localhost"); err != nil {
		t.Fatal(err)
	}

	// Get existing key
	val, err := store.Get("api_url")
	if err != nil {
		t.Fatal(err)
	}
	if val != "https://example.com" {
		t.Errorf("expected 'https://example.com', got %q", val)
	}

	// Get missing key
	_, err = store.Get("missing")
	if err == nil {
		t.Error("expected error for missing key")
	}

	// List all
	keys, err := store.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}

	// List with filter
	keys, err = store.List("api")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys matching 'api', got %d", len(keys))
	}

	// Search by value
	results, err := store.Search("example")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Key != "api_url" {
		t.Errorf("expected search to find api_url, got %v", results)
	}

	// Search by key
	results, err = store.Search("db")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Key != "db_host" {
		t.Errorf("expected search to find db_host, got %v", results)
	}

	// Search no match
	results, err = store.Search("zzz")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestFileMemoryStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")

	// Write data with first store instance
	store1 := NewFileMemoryStore(path)
	store1.Set("key1", "value1")
	store1.Set("key2", "value2")

	// Create new store instance on same file
	store2 := NewFileMemoryStore(path)

	val, err := store2.Get("key1")
	if err != nil {
		t.Fatalf("expected persisted key1, got error: %v", err)
	}
	if val != "value1" {
		t.Errorf("expected 'value1', got %q", val)
	}

	val, err = store2.Get("key2")
	if err != nil {
		t.Fatalf("expected persisted key2, got error: %v", err)
	}
	if val != "value2" {
		t.Errorf("expected 'value2', got %q", val)
	}
}

// Test scratchpad list with "prefix" backward-compatibility parameter
func TestScratchpadList_WithPrefix(t *testing.T) {
	store := NewInMemoryStore()
	store.Set("api_url", "https://example.com")
	store.Set("api_key", "secret123")
	store.Set("db_host", "localhost")

	tool := ScratchpadList(store, false)
	// Use "prefix" instead of "filter" for backward compatibility
	args := Args{values: map[string]any{"prefix": "api"}}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "api_url") || !strings.Contains(result, "api_key") {
		t.Errorf("expected api keys in result, got %q", result)
	}
	if strings.Contains(result, "db_host") {
		t.Errorf("did not expect db_host in filtered result, got %q", result)
	}
}

// Test scratchpad read/write/search with bad args (missing required params)
func TestScratchpadRead_MissingKeyParam(t *testing.T) {
	store := NewInMemoryStore()
	tool := ScratchpadRead(store, false)

	// Execute with empty args -- should error on missing "key"
	args := Args{values: map[string]any{}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing 'key' param")
	}
}

func TestScratchpadWrite_MissingParams(t *testing.T) {
	store := NewInMemoryStore()
	tool := ScratchpadWrite(store, false)

	// Missing "key"
	args := Args{values: map[string]any{"value": "val"}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing 'key' param")
	}

	// Missing "value"
	args = Args{values: map[string]any{"key": "k"}}
	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing 'value' param")
	}
}

func TestScratchpadSearch_MissingQuery(t *testing.T) {
	store := NewInMemoryStore()
	tool := ScratchpadSearch(store, false)

	args := Args{values: map[string]any{}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing 'query' param")
	}
}

func TestScratchpad_PersistentDescription(t *testing.T) {
	store := NewInMemoryStore()

	persistent := ScratchpadWrite(store, true)
	ephemeral := ScratchpadWrite(store, false)

	if !strings.Contains(persistent.Description(), "PERSISTENT") {
		t.Error("persistent tool should mention PERSISTENT in description")
	}
	if !strings.Contains(ephemeral.Description(), "EPHEMERAL") {
		t.Error("ephemeral tool should mention EPHEMERAL in description")
	}

	// Check read tool descriptions too
	persistentRead := ScratchpadRead(store, true)
	ephemeralRead := ScratchpadRead(store, false)

	if !strings.Contains(persistentRead.Description(), "PERSISTENT") {
		t.Error("persistent read tool should mention PERSISTENT")
	}
	if !strings.Contains(ephemeralRead.Description(), "EPHEMERAL") {
		t.Error("ephemeral read tool should mention EPHEMERAL")
	}

	// Check list tool descriptions
	persistentList := ScratchpadList(store, true)
	ephemeralList := ScratchpadList(store, false)
	if !strings.Contains(persistentList.Description(), "PERSISTENT") {
		t.Error("persistent list tool should mention PERSISTENT")
	}
	if !strings.Contains(ephemeralList.Description(), "EPHEMERAL") {
		t.Error("ephemeral list tool should mention EPHEMERAL")
	}

	// Check search tool descriptions
	persistentSearch := ScratchpadSearch(store, true)
	ephemeralSearch := ScratchpadSearch(store, false)
	if !strings.Contains(persistentSearch.Description(), "PERSISTENT") {
		t.Error("persistent search tool should mention PERSISTENT")
	}
	if !strings.Contains(ephemeralSearch.Description(), "EPHEMERAL") {
		t.Error("ephemeral search tool should mention EPHEMERAL")
	}
}
