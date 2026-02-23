# Idempotent Tasks Example

This example demonstrates the `tasks` package for **stateful, idempotent task processing** with proper lifecycle management.

## Overview

The `tasks` package provides:

- **Idempotency**: Tasks with the same `IdempotencyKey` are deduplicated
- **State Tracking**: Persistent task state across restarts (pending/claimed/completed/failed)
- **Claim Semantics**: Workers explicitly claim tasks, preventing double-processing
- **Retry Support**: Failed tasks automatically retry up to `MaxAttempts`
- **Result Storage**: Completed task results are stored for later retrieval

## Comparison: bus-based vs tasks-based

| Feature | task-queue (bus) | idempotent-tasks |
|---------|------------------|------------------|
| Message delivery | Fire-and-forget pub/sub | Persistent state store |
| Deduplication | None | Via IdempotencyKey |
| Task state | Not tracked | Full lifecycle tracking |
| Retries | Manual implementation | Built-in with MaxAttempts |
| Worker claim | Implicit (queue subscribe) | Explicit Claim/Complete/Fail |
| Failure handling | Lost or manual retry | Automatic retry to pending |
| Result retrieval | Not available | Stored on completion |
| Audit trail | None | Full task history |

### When to use each

**Use `task-queue` (bus-based) when:**
- Tasks are ephemeral (ok to lose on crash)
- High throughput, low latency is critical
- Simple fire-and-forget semantics are sufficient
- No need for retry or result tracking

**Use `idempotent-tasks` when:**
- Tasks must not be duplicated (payments, orders)
- Need to survive restarts
- Retry logic is required
- Need to query task status/results
- Audit trail is important

## Files

- **producer.go** - Submits tasks with idempotency keys, demonstrates deduplication
- **worker.go** - Claims, processes, and completes/fails tasks with retry

## Running the Example

### Build

```bash
cd agentkit/examples/idempotent-tasks
go build producer.go
go build worker.go
```

### Run Producer (Submit Tasks)

```bash
./producer
```

Output:
```
=== Idempotent Task Producer ===

--- Demo 1: Basic Task Submission ---
✓ Submitted task: 20240223150405.123456789 (key: order-12345)

--- Demo 2: Idempotent Deduplication ---
Attempting to submit same order again (should dedupe)...
✓ Deduplicated! Returned existing task: 20240223150405.123456789

--- Demo 3: Submitting Multiple Unique Tasks ---
✓ Submitted: 20240223150405.234567890 (order-12346, $49.99)
✓ Submitted: 20240223150405.345678901 (order-12347, $199.99)
✓ Submitted: 20240223150405.456789012 (order-12348, $29.99)
...
```

### Run Worker (Process Tasks)

```bash
./worker worker-1
```

Output:
```
[worker-1] Task Worker started
Processing loop active - waiting for tasks...
[worker-1] Processing: 20240223150405.123456789 (process_order) - attempt 1/3
[worker-1] ✓ Completed: 20240223150405.123456789 - Order 12345 processed, charged $99.99
[worker-1] Processing: 20240223150405.234567890 (process_order) - attempt 1/3
[worker-1] ↻ Retry queued: 20240223150405.234567890 - processing failed: temporary error (attempt 1/3)
...
```

### Run Multiple Workers

```bash
# Terminal 1
./worker worker-1

# Terminal 2  
./worker worker-2
```

Workers will compete to claim tasks. Once claimed, only that worker can complete/fail the task.

## Key Concepts

### Idempotency Keys

```go
// Same key = same task (deduplicated)
id1, _ := manager.Submit(ctx, tasks.Task{
    IdempotencyKey: "order-12345",
    Payload:        payload,
})

id2, _ := manager.Submit(ctx, tasks.Task{
    IdempotencyKey: "order-12345",  // Same key
    Payload:        payload,
})

// id1 == id2 (deduplication!)
```

### Task Lifecycle

```
              ┌─────────┐
              │ PENDING │ ←──────────────────┐
              └────┬────┘                    │
                   │                         │
              Claim(taskID, workerID)        │
                   │                         │
                   ▼                         │
              ┌─────────┐              Fail() with
              │ CLAIMED │              retries left
              └────┬────┘                    │
                   │                         │
         ┌─────────┴─────────┐              │
         │                   │              │
    Complete()           Fail()─────────────┘
         │                   │
         ▼                   ▼
   ┌───────────┐      ┌────────┐
   │ COMPLETED │      │ FAILED │ (max attempts)
   └───────────┘      └────────┘
```

### Retry Handling

```go
task := tasks.Task{
    Payload:     payload,
    MaxAttempts: 3,  // Will retry up to 3 times
}

// On failure, task moves back to PENDING for retry
// After 3 attempts, status becomes FAILED (permanent)
```

### Looking Up Tasks

```go
// By ID
task, _ := manager.Get(ctx, taskID)

// By idempotency key
task, _ := manager.GetByIdempotencyKey(ctx, "order-12345")

// List by status
pending, _ := manager.List(ctx, tasks.StatusPending)
completed, _ := manager.List(ctx, tasks.StatusCompleted)
```

## Production Notes

### Distributed State

For distributed workers, use NATS KV as the state backend:

```go
nc, _ := nats.Connect("nats://localhost:4222")
store, _ := state.NewNATSStore(nc, state.NATSConfig{
    Bucket: "tasks",
})
manager := tasks.NewManager(store)
```

### Stale Claim Recovery

In production, implement stale claim detection:

```go
// Check for tasks claimed too long ago (worker might have crashed)
tasks, _ := manager.List(ctx, tasks.StatusClaimed)
for _, t := range tasks {
    if t.ClaimedAt != nil && time.Since(*t.ClaimedAt) > 5*time.Minute {
        manager.Retry(ctx, t.ID)  // Release back to pending
    }
}
```

### Task Cleanup

Clean up old completed/failed tasks:

```go
tasks, _ := manager.List(ctx, tasks.StatusCompleted)
for _, t := range tasks {
    if time.Since(*t.CompletedAt) > 24*time.Hour {
        manager.Delete(ctx, t.ID)
    }
}
```
