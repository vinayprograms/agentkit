package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// MemoryStore is the interface for key-value storage (scratchpad).
type MemoryStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
	List(prefix string) ([]string, error)
	Search(query string) ([]MemorySearchResult, error)
}

// MemorySearchResult represents a search hit in scratchpad.
type MemorySearchResult struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// --- Scratchpad Tools ---

type scratchpadReadTool struct {
	store     MemoryStore
	persisted bool
}

// ScratchpadRead creates a tool that reads a value by exact key from the scratchpad.
// When persistent is true, the description tells the LLM that data survives across runs.
func ScratchpadRead(store MemoryStore, persistent bool) Tool {
	return &scratchpadReadTool{store: store, persisted: persistent}
}

func (t *scratchpadReadTool) Name() string { return "scratchpad_read" }

func (t *scratchpadReadTool) Description() string {
	if t.persisted {
		return `Read a value by exact key from persistent scratchpad.

⚠️ PERSISTENT: Data survives across agent runs.

Use for: intermediate results, temporary state, values to reference later by exact key.
Examples: "api_endpoint", "selected_model", "step3_output"

NOT for: insights or learnings. Use remember for semantic storage.`
	}
	return `Read a value by exact key from session scratchpad.

⚠️ EPHEMERAL: Data is lost when this agent run ends.

Use for: intermediate results, temporary state, values to reference later by exact key.
Examples: "api_endpoint", "selected_model", "step3_output"

NOT for: insights or learnings. Use remember for semantic storage.`
}

func (t *scratchpadReadTool) Parameters() map[string]Param {
	return map[string]Param{
		"key": {
			Type:        StringParam,
			Description: "Key to read",
			Required:    true,
		},
	}
}

func (t *scratchpadReadTool) Execute(ctx context.Context, args Args) (string, error) {
	key, err := args.String("key")
	if err != nil {
		return "", err
	}

	value, err := t.store.Get(key)
	if err != nil {
		return "", err
	}
	return value, nil
}

// ---

type scratchpadWriteTool struct {
	store     MemoryStore
	persisted bool
}

// ScratchpadWrite creates a tool that stores a key-value pair in the scratchpad.
// When persistent is true, the description tells the LLM that data survives across runs.
func ScratchpadWrite(store MemoryStore, persistent bool) Tool {
	return &scratchpadWriteTool{store: store, persisted: persistent}
}

func (t *scratchpadWriteTool) Name() string { return "scratchpad_write" }

func (t *scratchpadWriteTool) Description() string {
	if t.persisted {
		return `Store a key-value pair in persistent scratchpad.

⚠️ PERSISTENT: Data survives across agent runs.

Use for: intermediate results, temporary state, values you'll retrieve by EXACT key.
Examples: scratchpad_write("api_endpoint", "https://api.example.com")

NOT for: insights, decisions, learnings. Use remember for semantic storage.`
	}
	return `Store a key-value pair in session scratchpad.

⚠️ EPHEMERAL: Data is lost when this agent run ends.

Use for: intermediate results, temporary state, values you'll retrieve by EXACT key.
Examples: scratchpad_write("api_endpoint", "https://api.example.com")

NOT for: insights, decisions, learnings. Use remember for semantic storage.`
}

func (t *scratchpadWriteTool) Parameters() map[string]Param {
	return map[string]Param{
		"key": {
			Type:        StringParam,
			Description: "Key to write",
			Required:    true,
		},
		"value": {
			Type:        StringParam,
			Description: "Value to store",
			Required:    true,
		},
	}
}

func (t *scratchpadWriteTool) Execute(ctx context.Context, args Args) (string, error) {
	key, err := args.String("key")
	if err != nil {
		return "", err
	}
	value, err := args.String("value")
	if err != nil {
		return "", err
	}

	if err := t.store.Set(key, value); err != nil {
		return "", err
	}
	return fmt.Sprintf("Stored in scratchpad under key %q. Use scratchpad_read(%q) to retrieve.", key, key), nil
}

// ---

type scratchpadListTool struct {
	store     MemoryStore
	persisted bool
}

// ScratchpadList creates a tool that lists keys in the scratchpad.
// When persistent is true, the description tells the LLM that data survives across runs.
func ScratchpadList(store MemoryStore, persistent bool) Tool {
	return &scratchpadListTool{store: store, persisted: persistent}
}

func (t *scratchpadListTool) Name() string { return "scratchpad_list" }

func (t *scratchpadListTool) Description() string {
	persistence := "EPHEMERAL (session only)"
	if t.persisted {
		persistence = "PERSISTENT (survives runs)"
	}
	return fmt.Sprintf(`List keys in scratchpad, optionally filtered by substring.

⚠️ %s

Use for: discovering what's stored in scratchpad.
Example: scratchpad_list("") → all keys
Example: scratchpad_list("api") → ["api_endpoint", "user_api_key"]`, persistence)
}

func (t *scratchpadListTool) Parameters() map[string]Param {
	return map[string]Param{
		"filter": {
			Type:        StringParam,
			Description: "Optional substring to filter keys (case-insensitive)",
		},
	}
}

func (t *scratchpadListTool) Execute(ctx context.Context, args Args) (string, error) {
	filter := args.StringOr("filter", "")
	// Also accept "prefix" for backward compatibility
	if filter == "" {
		filter = args.StringOr("prefix", "")
	}

	keys, err := t.store.List(filter)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "no keys found", nil
	}
	return strings.Join(keys, "\n"), nil
}

// ---

type scratchpadSearchTool struct {
	store     MemoryStore
	persisted bool
}

// ScratchpadSearch creates a tool that performs substring search in scratchpad keys and values.
// When persistent is true, the description tells the LLM that data survives across runs.
func ScratchpadSearch(store MemoryStore, persistent bool) Tool {
	return &scratchpadSearchTool{store: store, persisted: persistent}
}

func (t *scratchpadSearchTool) Name() string { return "scratchpad_search" }

func (t *scratchpadSearchTool) Description() string {
	persistence := "EPHEMERAL (session only)"
	if t.persisted {
		persistence = "PERSISTENT (survives runs)"
	}
	return fmt.Sprintf(`Substring search in scratchpad keys and values.

⚠️ %s

Use for: finding a value when you know part of the key or value.
Example: scratchpad_search("endpoint") → finds "api_endpoint": "https://..."`, persistence)
}

func (t *scratchpadSearchTool) Parameters() map[string]Param {
	return map[string]Param{
		"query": {
			Type:        StringParam,
			Description: "Search term to find in scratchpad keys/values",
			Required:    true,
		},
	}
}

func (t *scratchpadSearchTool) Execute(ctx context.Context, args Args) (string, error) {
	query, err := args.String("query")
	if err != nil {
		return "", err
	}

	results, err := t.store.Search(query)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "no matching entries found", nil
	}

	var sb strings.Builder
	for i, r := range results {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("%s: %s", r.Key, r.Value))
	}
	return sb.String(), nil
}

// --- MemoryStore Implementations ---

// FileMemoryStore stores memory in a JSON file.
type FileMemoryStore struct {
	path string
	data map[string]string
}

// NewFileMemoryStore creates a new file-based memory store.
func NewFileMemoryStore(path string) *FileMemoryStore {
	store := &FileMemoryStore{
		path: path,
		data: make(map[string]string),
	}
	// Load existing data
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &store.data)
	}
	return store
}

func (s *FileMemoryStore) Get(key string) (string, error) {
	if val, ok := s.data[key]; ok {
		return val, nil
	}
	return "", fmt.Errorf("key not found: %s", key)
}

func (s *FileMemoryStore) Set(key, value string) error {
	s.data[key] = value
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func (s *FileMemoryStore) List(filter string) ([]string, error) {
	var keys []string
	filterLower := strings.ToLower(filter)
	for k := range s.data {
		if filter == "" || strings.Contains(strings.ToLower(k), filterLower) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (s *FileMemoryStore) Search(query string) ([]MemorySearchResult, error) {
	query = strings.ToLower(query)
	var results []MemorySearchResult
	for k, v := range s.data {
		if strings.Contains(strings.ToLower(v), query) ||
			strings.Contains(strings.ToLower(k), query) {
			results = append(results, MemorySearchResult{Key: k, Value: v})
		}
	}
	return results, nil
}

// InMemoryStore stores memory in-memory only (lost after run).
type InMemoryStore struct {
	data map[string]string
}

// NewInMemoryStore creates a new in-memory store (scratchpad mode).
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		data: make(map[string]string),
	}
}

func (s *InMemoryStore) Get(key string) (string, error) {
	if val, ok := s.data[key]; ok {
		return val, nil
	}
	return "", fmt.Errorf("key not found: %s", key)
}

func (s *InMemoryStore) Set(key, value string) error {
	s.data[key] = value
	return nil
}

func (s *InMemoryStore) List(filter string) ([]string, error) {
	var keys []string
	filterLower := strings.ToLower(filter)
	for k := range s.data {
		if filter == "" || strings.Contains(strings.ToLower(k), filterLower) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (s *InMemoryStore) Search(query string) ([]MemorySearchResult, error) {
	query = strings.ToLower(query)
	var results []MemorySearchResult
	for k, v := range s.data {
		if strings.Contains(strings.ToLower(v), query) ||
			strings.Contains(strings.ToLower(k), query) {
			results = append(results, MemorySearchResult{Key: k, Value: v})
		}
	}
	return results, nil
}
