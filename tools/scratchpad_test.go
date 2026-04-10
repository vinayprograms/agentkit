package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// mockScratchpad implements Scratchpad for testing.
type mockScratchpad struct {
	data map[string]string
}

func newMockScratchpad() *mockScratchpad {
	return &mockScratchpad{data: make(map[string]string)}
}

func (m *mockScratchpad) Get(key string) (string, error) {
	v, ok := m.data[key]
	if !ok {
		return "", fmt.Errorf("key not found: %s", key)
	}
	return v, nil
}

func (m *mockScratchpad) Set(key, value string) error {
	m.data[key] = value
	return nil
}

func (m *mockScratchpad) List(prefix string) ([]string, error) {
	var keys []string
	for k := range m.data {
		if prefix == "" || strings.Contains(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (m *mockScratchpad) Search(query string) (map[string]string, error) {
	results := make(map[string]string)
	q := strings.ToLower(query)
	for k, v := range m.data {
		if strings.Contains(strings.ToLower(k), q) || strings.Contains(strings.ToLower(v), q) {
			results[k] = v
		}
	}
	return results, nil
}

func TestScratchpadWrite(t *testing.T) {
	store := newMockScratchpad()
	tool := ScratchpadWrite(store)

	args, err := Validate(tool.Parameters(), map[string]any{"key": "api_url", "value": "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result != "ok" {
		t.Errorf("expected 'ok', got %q", result)
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
	store := newMockScratchpad()
	store.Set("color", "blue")

	tool := ScratchpadRead(store)
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
	store := newMockScratchpad()
	tool := ScratchpadRead(store)

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
	store := newMockScratchpad()
	store.Set("api_url", "https://example.com")
	store.Set("api_key", "secret123")
	store.Set("db_host", "localhost")

	tool := ScratchpadList(store)
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

func TestScratchpadList_WithPrefix(t *testing.T) {
	store := newMockScratchpad()
	store.Set("api_url", "https://example.com")
	store.Set("api_key", "secret123")
	store.Set("db_host", "localhost")

	tool := ScratchpadList(store)
	args, err := Validate(tool.Parameters(), map[string]any{"prefix": "api"})
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
	store := newMockScratchpad()
	tool := ScratchpadList(store)

	args, err := Validate(tool.Parameters(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result != "No keys found." {
		t.Errorf("expected 'No keys found.', got %q", result)
	}
}

func TestScratchpadSearch(t *testing.T) {
	store := newMockScratchpad()
	store.Set("api_url", "https://example.com/api")
	store.Set("db_host", "localhost:5432")
	store.Set("note", "the api is rate-limited")

	tool := ScratchpadSearch(store)
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
	store := newMockScratchpad()
	store.Set("key1", "value1")

	tool := ScratchpadSearch(store)
	args, err := Validate(tool.Parameters(), map[string]any{"query": "zzz"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result != "No matches found." {
		t.Errorf("expected 'No matches found.', got %q", result)
	}
}

// Test scratchpad read/write/search with bad args (missing required params)
func TestScratchpadRead_MissingKeyParam(t *testing.T) {
	store := newMockScratchpad()
	tool := ScratchpadRead(store)

	// Execute with empty args -- should error on missing "key"
	args := Args{values: map[string]any{}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing 'key' param")
	}
}

func TestScratchpadWrite_MissingParams(t *testing.T) {
	store := newMockScratchpad()
	tool := ScratchpadWrite(store)

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
	store := newMockScratchpad()
	tool := ScratchpadSearch(store)

	args := Args{values: map[string]any{}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing 'query' param")
	}
}
