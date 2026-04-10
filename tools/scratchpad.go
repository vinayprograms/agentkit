package tools

import (
	"context"
	"fmt"
	"strings"
)

// Scratchpad is the interface for key-value storage used by scratchpad tools.
// memory.InMemoryStore and memory.BleveStore both satisfy this interface.
type Scratchpad interface {
	Get(key string) (string, error)
	Set(key, value string) error
	List(prefix string) ([]string, error)
	Search(query string) (map[string]string, error)
}

// --- ScratchpadRead ---

type scratchpadReadTool struct {
	store Scratchpad
}

func ScratchpadRead(store Scratchpad) Tool {
	return &scratchpadReadTool{store: store}
}

func (t *scratchpadReadTool) Name() string { return "scratchpad_read" }

func (t *scratchpadReadTool) Description() string {
	return "Read a value by exact key from the scratchpad."
}

func (t *scratchpadReadTool) Parameters() map[string]Param {
	return map[string]Param{
		"key": {
			Type:        StringParam,
			Description: "The exact key to read",
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
	if value == "" {
		return fmt.Sprintf("Key %q not found.", key), nil
	}
	return value, nil
}

// --- ScratchpadWrite ---

type scratchpadWriteTool struct {
	store Scratchpad
}

func ScratchpadWrite(store Scratchpad) Tool {
	return &scratchpadWriteTool{store: store}
}

func (t *scratchpadWriteTool) Name() string { return "scratchpad_write" }

func (t *scratchpadWriteTool) Description() string {
	return "Store a key-value pair in the scratchpad."
}

func (t *scratchpadWriteTool) Parameters() map[string]Param {
	return map[string]Param{
		"key": {
			Type:        StringParam,
			Description: "The key to store under",
			Required:    true,
		},
		"value": {
			Type:        StringParam,
			Description: "The value to store",
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
	return "ok", nil
}

// --- ScratchpadList ---

type scratchpadListTool struct {
	store Scratchpad
}

func ScratchpadList(store Scratchpad) Tool {
	return &scratchpadListTool{store: store}
}

func (t *scratchpadListTool) Name() string { return "scratchpad_list" }

func (t *scratchpadListTool) Description() string {
	return "List keys in the scratchpad, optionally filtered by prefix."
}

func (t *scratchpadListTool) Parameters() map[string]Param {
	return map[string]Param{
		"prefix": {
			Type:        StringParam,
			Description: "Optional prefix to filter keys",
		},
	}
}

func (t *scratchpadListTool) Execute(ctx context.Context, args Args) (string, error) {
	prefix := args.StringOr("prefix", "")
	keys, err := t.store.List(prefix)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "No keys found.", nil
	}
	return strings.Join(keys, "\n"), nil
}

// --- ScratchpadSearch ---

type scratchpadSearchTool struct {
	store Scratchpad
}

func ScratchpadSearch(store Scratchpad) Tool {
	return &scratchpadSearchTool{store: store}
}

func (t *scratchpadSearchTool) Name() string { return "scratchpad_search" }

func (t *scratchpadSearchTool) Description() string {
	return "Search scratchpad keys and values by query."
}

func (t *scratchpadSearchTool) Parameters() map[string]Param {
	return map[string]Param{
		"query": {
			Type:        StringParam,
			Description: "Search query",
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
		return "No matches found.", nil
	}
	var sb strings.Builder
	i := 0
	for k, v := range results {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "%s: %s", k, v)
		i++
	}
	return sb.String(), nil
}
