# memory

Persistent knowledge storage with key-value operations, observation management, and text search.

## Stores

Two implementations of `Store`, fully interchangeable:

- **`InMemoryStore`** — ephemeral, all data lost on exit. Good for scratchpad, testing, or long-running agents.
- **`BleveStore`** — persistent, disk-backed with BM25 text search. Good for cross-session memory.

```go
// Ephemeral
store := memory.NewInMemoryStore()

// Persistent
store, err := memory.NewBleveStore(memory.BleveStoreConfig{
    BasePath: "/var/data/agent/memory",
})
defer store.Close()
```

## Key-Value Operations

```go
store.Set("api.endpoint", "https://api.example.com")
value, err := store.Get("api.endpoint")
keys, err := store.List("api.")
results, err := store.Search("example")  // returns map[string]string
```

## Observations (Findings, Insights, Lessons)

Store and retrieve categorized observations:

```go
// Store
ids, err := store.RememberFIL(ctx,
    []string{"API rate limit is 100/min"},        // findings
    []string{"REST is simpler than GraphQL"},      // insights
    []string{"Always check rate limits first"},    // lessons
    "research-step",
)

// Retrieve by category
results, err := store.RecallFIL(ctx, "rate limit", 5)
// results.Findings, results.Insights, results.Lessons

// List all
items, err := store.ListAll(ctx, "finding", 100)
```

## Extractor

Extract findings, insights, and lessons from text using an LLM:

```go
extractor := memory.NewExtractor(llmModel)
findings, insights, lessons, err := extractor.Extract(ctx, stepOutput)

// Store the extracted observations
store.RememberFIL(ctx, findings, insights, lessons, "step:research")
```

The extractor is decoupled from storage — it returns raw slices. The consumer decides what to store, filter, or discard.

## Tools Integration

The tools package defines `Scratchpad` and `Memory` interfaces that `Store` satisfies implicitly:

```go
// Scratchpad tools use KV operations
registry.Register(tools.New(tools.ScratchpadRead(store)))

// Memory tools use observation operations
registry.Register(tools.New(tools.Remember(store)))
registry.Register(tools.New(tools.Recall(store)))
```

Use different stores for different concerns:

```go
scratchpad := memory.NewInMemoryStore()           // ephemeral working memory
longTerm, _ := memory.NewBleveStore(config)       // persistent knowledge

registry.Register(tools.New(tools.ScratchpadRead(scratchpad)))
registry.Register(tools.New(tools.Remember(longTerm)))
```
