# Shared State Design

## Overview

The state package provides distributed key-value storage with locking for shared agent state. It supports TTL-based expiry, change watching, and distributed locks for coordination.

## Goals

| Goal | Description |
|------|-------------|
| Shared state | Key-value storage accessible by all agents |
| Distributed locking | Prevent concurrent access to resources |
| TTL support | Auto-expire transient data |
| Watch patterns | React to state changes in real-time |
| Backend agnostic | Memory for testing, NATS JetStream for production |

## Non-Goals

| Non-Goal | Reason |
|----------|--------|
| Full database | Not for large datasets or complex queries |
| Transactions | Single-key operations only |
| Strong consistency | Eventual consistency is acceptable |
| Schema enforcement | Opaque byte values |

## Core Types

### KeyValue

```go
type KeyValue struct {
    Key       string
    Value     []byte
    Revision  uint64     // Monotonic version number
    Operation Operation  // OpPut or OpDelete
    Created   time.Time
    Modified  time.Time
}

type Operation int
const (
    OpPut    Operation = iota  // Created or updated
    OpDelete                    // Deleted
)
```

### StateStore Interface

```go
type StateStore interface {
    // Get retrieves a value by key.
    Get(key string) ([]byte, error)

    // GetKeyValue retrieves full entry with metadata.
    GetKeyValue(key string) (*KeyValue, error)

    // Put stores a value with optional TTL.
    // If ttl is 0, the key never expires.
    Put(key string, value []byte, ttl time.Duration) error

    // Delete removes a key.
    Delete(key string) error

    // Keys returns all keys matching a pattern.
    // Pattern supports * wildcard at end (e.g., "config.*").
    Keys(pattern string) ([]string, error)

    // Watch watches for changes to keys matching pattern.
    Watch(pattern string) (<-chan *KeyValue, error)

    // Lock acquires a distributed lock with TTL.
    Lock(key string, ttl time.Duration) (Lock, error)

    // Close shuts down the store.
    Close() error
}
```

### Lock Interface

```go
type Lock interface {
    // Unlock releases the lock.
    Unlock() error

    // Refresh extends the lock TTL.
    Refresh() error

    // Key returns the lock key.
    Key() string
}
```

## Architecture

![StateStore Architecture](images/state-architecture.png)

## Implementations

### MemoryStore

In-memory implementation for testing and single-process use.

| Feature | Implementation |
|---------|----------------|
| Storage | `map[string]*entry` with RWMutex |
| TTL | Cleanup goroutine runs every second |
| Locks | `map[string]*memoryLock` with expiry checks |
| Watch | Slice of watchers with pattern matching |
| Revision | Monotonic counter incremented on each change |

### NATSStore

Production implementation using NATS JetStream KV.

| Feature | Implementation |
|---------|----------------|
| Storage | JetStream KV bucket |
| TTL | Bucket-level TTL configuration |
| Locks | KV entries with TTL-encoded values |
| Watch | Native KV watcher with pattern translation |
| Replication | JetStream replication for HA |

**Configuration:**
- `Conn` - NATS connection
- `Bucket` - KV bucket name (default: "agent-state")
- `TTL` - Default entry TTL
- `History` - Revisions to keep per key
- `MaxValueSize` - Maximum value size (default: 1MB)

## Package Structure

![Package Structure](images/state-package-structure.png)

## Distributed Locking Design

Locks prevent concurrent access to resources across agents.

### Lock Acquisition

![Lock Acquisition Flow](images/state-lock-acquisition.png)

### Lock Contention

![Lock Contention Scenario](images/state-lock-contention.png)

### Lock Best Practices

1. **Always set TTL** - Prevents deadlocks from crashed agents
2. **Refresh for long operations** - Call `Refresh()` periodically
3. **Use defer Unlock** - Ensure release on all code paths
4. **Handle ErrLockHeld** - Retry with backoff or fail gracefully

```go
lock, err := store.Lock("resource.db", 30*time.Second)
if err == ErrLockHeld {
    // Retry or fail
    return err
}
if err != nil {
    return err
}
defer lock.Unlock()

// For long operations, refresh periodically
go func() {
    ticker := time.NewTicker(10 * time.Second)
    for range ticker.C {
        if err := lock.Refresh(); err != nil {
            break
        }
    }
}()

// Do protected work
```

## TTL and Expiry

TTL enables automatic cleanup of transient data.

```go
// Store with 5-minute TTL
store.Put("cache.result.123", data, 5*time.Minute)

// Store without expiry
store.Put("config.permanent", data, 0)
```

**Expiry behavior:**
- MemoryStore: Cleanup goroutine removes expired entries every second
- NATSStore: JetStream handles expiry at bucket level
- Expired keys return `ErrNotFound` on Get
- Watch receives `OpDelete` event on expiry

## Watch Patterns

Watch enables reactive state handling:

```go
// Watch all config changes
changes, err := store.Watch("config.*")
if err != nil {
    return err
}

for kv := range changes {
    switch kv.Operation {
    case OpPut:
        log.Printf("Config updated: %s", kv.Key)
        reloadConfig(kv.Key, kv.Value)
    case OpDelete:
        log.Printf("Config deleted: %s", kv.Key)
        removeConfig(kv.Key)
    }
}
```

**Pattern syntax:**
- `*` - Match all keys
- `prefix.*` - Match keys starting with prefix

**Channel behavior:**
- Buffer size: 64 events
- Non-blocking sends (drops if full)
- Closed when store closes

## Usage Patterns

### Shared Configuration

```go
// Writer
config := map[string]string{"timeout": "30s"}
data, _ := json.Marshal(config)
store.Put("config.agent-pool", data, 0)

// Reader
data, err := store.Get("config.agent-pool")
var config map[string]string
json.Unmarshal(data, &config)
```

### Task Queue with Locking

```go
// Worker claims task
lock, err := store.Lock("task."+taskID, 60*time.Second)
if err == ErrLockHeld {
    return // Another worker got it
}
defer lock.Unlock()

// Process task (protected)
taskData, _ := store.Get("tasks." + taskID)
result := process(taskData)

// Store result and clean up
store.Put("results."+taskID, result, 24*time.Hour)
store.Delete("tasks." + taskID)
```

### Caching with TTL

```go
// Check cache
result, err := store.Get("cache.expensive." + key)
if err == nil {
    return result, nil
}

// Cache miss - compute and store
result = expensiveComputation(key)
store.Put("cache.expensive."+key, result, 10*time.Minute)
return result, nil
```

### Leader Election (Simple)

```go
func tryBecomeLeader(store StateStore, agentID string) bool {
    lock, err := store.Lock("leader", 30*time.Second)
    if err != nil {
        return false
    }
    
    // We're the leader - start refresh loop
    go func() {
        ticker := time.NewTicker(10 * time.Second)
        for range ticker.C {
            if err := lock.Refresh(); err != nil {
                // Lost leadership
                return
            }
        }
    }()
    
    return true
}
```

## Error Handling

| Error | Meaning | Recovery |
|-------|---------|----------|
| `ErrNotFound` | Key does not exist | Expected for queries |
| `ErrClosed` | Store has been closed | Reconnect or exit |
| `ErrLockHeld` | Lock held by another | Retry with backoff |
| `ErrLockNotHeld` | Unlock without lock | Fix logic error |
| `ErrLockExpired` | Lock TTL elapsed | Re-acquire if needed |
| `ErrInvalidKey` | Empty or malformed key | Fix key format |
| `ErrInvalidTTL` | Negative TTL | Use 0 for no expiry |

## Key Validation

Keys must:
- Be non-empty
- Not contain spaces
- Not start or end with `.`
- Be at most 1024 characters

## Testing Strategy

| Level | Focus |
|-------|-------|
| Unit | CRUD operations, pattern matching |
| Integration | Multi-agent state sharing |
| Locking | Contention, TTL expiry, deadlock prevention |
| TTL | Expiry timing, cleanup behavior |
| Watch | Event delivery, pattern filtering |
| Concurrency | Race conditions, lock contention |
