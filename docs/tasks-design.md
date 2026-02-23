# Tasks Design

## Overview

The tasks package provides idempotent task handling with deduplication for distributed agent workloads. It enables safe retry semantics through idempotency keys, claim-based ownership, and configurable retry limits.

## Goals

| Goal | Description |
|------|-------------|
| Idempotent submission | Duplicate task submissions return existing task |
| Claim ownership | Workers claim tasks with exclusive access |
| Retry semantics | Failed tasks automatically retry with limits |
| State persistence | Tasks survive restarts via state backend |
| Deduplication | Idempotency keys prevent duplicate work |

## Non-Goals

| Non-Goal | Reason |
|----------|--------|
| Scheduling | No time-based or priority scheduling |
| Fan-out | Single worker per task, no parallel execution |
| Dead letter queue | Failed tasks stay in state, no separate queue |
| Rate limiting | Workers control their own claim rate |

## Core Types

### Task

```go
type Task struct {
    // ID is the unique identifier for the task.
    ID string

    // IdempotencyKey is used for deduplication.
    IdempotencyKey string

    // Payload is the task data to be processed.
    Payload []byte

    // Status is the current state of the task.
    Status TaskStatus

    // Attempts is the number of times this task has been attempted.
    Attempts int

    // MaxAttempts is the maximum number of attempts allowed.
    // Zero means unlimited attempts.
    MaxAttempts int

    // CreatedAt is when the task was created.
    CreatedAt time.Time

    // ClaimedAt is when the task was last claimed.
    ClaimedAt *time.Time

    // ClaimedBy is the worker ID that currently holds the task.
    ClaimedBy string

    // CompletedAt is when the task was completed or failed.
    CompletedAt *time.Time

    // Result is the output from task completion.
    Result []byte

    // Error is the error message if the task failed.
    Error string
}
```

### TaskStatus

```go
type TaskStatus string

const (
    StatusPending   TaskStatus = "pending"    // Waiting to be claimed
    StatusClaimed   TaskStatus = "claimed"    // Being worked on
    StatusCompleted TaskStatus = "completed"  // Successfully finished
    StatusFailed    TaskStatus = "failed"     // Permanently failed
)
```

### TaskManager Interface

```go
type TaskManager interface {
    // Submit creates a new task or returns existing ID if
    // a task with the same IdempotencyKey already exists.
    Submit(ctx context.Context, task Task) (string, error)

    // Claim marks a task as being worked on by a worker.
    Claim(ctx context.Context, taskID string, workerID string) error

    // Complete marks a task as successfully completed.
    Complete(ctx context.Context, taskID string, result []byte) error

    // Fail marks a task as failed with the given error.
    Fail(ctx context.Context, taskID string, err error) error

    // Retry moves a failed/claimed task back to pending.
    Retry(ctx context.Context, taskID string) error

    // Get retrieves a task by ID.
    Get(ctx context.Context, taskID string) (*Task, error)

    // GetByIdempotencyKey retrieves a task by its idempotency key.
    GetByIdempotencyKey(ctx context.Context, key string) (*Task, error)

    // List returns all tasks matching the status filter.
    List(ctx context.Context, status TaskStatus) ([]*Task, error)

    // Delete removes a terminal task by ID.
    Delete(ctx context.Context, taskID string) error

    // Close releases resources.
    Close() error
}
```

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                         TaskManager                               │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                    Task Storage                              │ │
│  │                                                              │ │
│  │   tasks.task.abc123    ──▶ {id, status, payload...}         │ │
│  │   tasks.task.def456    ──▶ {id, status, payload...}         │ │
│  │   tasks.task.ghi789    ──▶ {id, status, payload...}         │ │
│  │                                                              │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                 Idempotency Index                            │ │
│  │                                                              │ │
│  │   tasks.idem.order-123-process  ──▶ "abc123"                │ │
│  │   tasks.idem.email-456-send     ──▶ "def456"                │ │
│  │                                                              │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                    State Backend                             │ │
│  │                                                              │ │
│  │   MemoryStore (testing) │ NATSStore (production)            │ │
│  │                                                              │ │
│  └─────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘
```

## Task Lifecycle

Tasks move through four states:

```
                    ┌──────────────────────────────────────────┐
                    │                                          │
                    ▼                                          │
┌─────────┐    ┌─────────┐    ┌───────────┐                   │
│ Submit  │───▶│ Pending │───▶│  Claimed  │                   │
└─────────┘    └─────────┘    └───────────┘                   │
                    ▲              │    │                      │
                    │              │    │                      │
               (retry)         Complete()  Fail()              │
                    │              │    │                      │
                    │              ▼    ▼                      │
                    │      ┌───────────────────┐              │
                    │      │                   │              │
                    │      ▼                   ▼              │
                    │  ┌───────────┐    ┌──────────┐          │
                    │  │ Completed │    │  Failed  │──────────┘
                    │  └───────────┘    └──────────┘  (if retries
                    │       ▲                ▲         available)
                    │       │                │
                    └───────┴────────────────┘
                         (Terminal States)
```

### State Transitions

| From | To | Trigger | Condition |
|------|-----|---------|-----------|
| – | Pending | `Submit()` | New task or new idempotency key |
| Pending | Claimed | `Claim()` | Task not already claimed |
| Claimed | Completed | `Complete()` | Same worker completes |
| Claimed | Failed | `Fail()` | MaxAttempts reached |
| Claimed | Pending | `Fail()` | Retries available |
| Claimed | Pending | `Retry()` | Retries available |
| Failed | Pending | `Retry()` | Retries available |

### Terminal States

Tasks in `StatusCompleted` or `StatusFailed` are terminal:
- No further state transitions (except `Retry()` for failed with retries)
- Can be deleted via `Delete()`
- Idempotency key lookup still returns the task

## Idempotency Key Design

Idempotency keys enable safe retry semantics and deduplication.

### How It Works

```
Client                        TaskManager                    State Store
   │                               │                              │
   │──Submit(key="order-123")─────▶│                              │
   │                               │──Get("tasks.idem.order-123")─▶│
   │                               │◀──────(not found)────────────│
   │                               │                              │
   │                               │──Put(task data)─────────────▶│
   │                               │──Put(idem mapping)──────────▶│
   │◀──────taskID="abc123"─────────│                              │
   │                               │                              │
   │  (network timeout, retry)     │                              │
   │                               │                              │
   │──Submit(key="order-123")─────▶│                              │
   │                               │──Get("tasks.idem.order-123")─▶│
   │                               │◀──────"abc123"───────────────│
   │◀──────taskID="abc123"─────────│  (existing task returned)    │
```

### Key Design Guidelines

| Guideline | Explanation |
|-----------|-------------|
| Include entity ID | `order-123-process` not just `process` |
| Include action | `order-123-send-email` not just `order-123` |
| Be deterministic | Same input → same key every time |
| Avoid timestamps | Retries generate different keys |

### Storage Layout

```
State Store Keys:
  tasks.idem.{idempotency-key}  →  task-id (string)
  tasks.task.{task-id}          →  task JSON (bytes)
```

The idempotency key is checked atomically during submission, preventing race conditions where duplicate tasks could be created.

## Retry Logic and Max Attempts

### Attempt Counting

- `Attempts` increments on each `Claim()` call
- First claim: `Attempts = 1`
- `MaxAttempts = 0` means unlimited retries
- `MaxAttempts = 3` allows 3 total attempts

### Fail Behavior

```go
// When Fail() is called:
if task.MaxAttempts > 0 && task.Attempts >= task.MaxAttempts {
    // Permanently failed - no more retries
    task.Status = StatusFailed
    task.CompletedAt = now
} else {
    // Move back to pending for retry
    task.Status = StatusPending
    task.ClaimedBy = ""
}
```

### Retry Flow

```
Worker A                       TaskManager                      Worker B
   │                               │                                │
   │──Claim(task, "worker-a")─────▶│                                │
   │◀──────(OK, Attempts=1)────────│                                │
   │                               │                                │
   │  (work fails)                 │                                │
   │                               │                                │
   │──Fail(task, err)─────────────▶│                                │
   │◀──────(OK, back to Pending)───│                                │
   │                               │                                │
   │                               │◀──Claim(task, "worker-b")──────│
   │                               │──────(OK, Attempts=2)─────────▶│
   │                               │                                │
   │                               │  (work succeeds)               │
   │                               │                                │
   │                               │◀──Complete(task, result)───────│
```

## State Backend Integration

The task manager delegates storage to a `state.StateStore` implementation.

### Key Prefixes

| Prefix | Purpose |
|--------|---------|
| `tasks.task.` | Task data (JSON-encoded) |
| `tasks.idem.` | Idempotency key → task ID mapping |

### Operations Mapping

| TaskManager | StateStore |
|-------------|------------|
| `Submit()` | `Get()` + `Put()` (check then create) |
| `Claim()` | `Get()` + `Put()` (read-modify-write) |
| `Complete()` | `Get()` + `Put()` |
| `Fail()` | `Get()` + `Put()` |
| `Get()` | `Get()` |
| `List()` | `Keys()` + multiple `Get()` |
| `Delete()` | `Delete()` × 2 (task + idem key) |

### Backend Requirements

The state backend must support:
- Key-value Get/Put/Delete
- Pattern-based key listing (`Keys("tasks.task.*")`)
- No TTL required (tasks persist indefinitely)

## Claim Ownership and Conflict Handling

### Worker Identification

Each worker must provide a unique `workerID` when claiming tasks. This enables:
- Conflict detection between workers
- Idempotent re-claims by the same worker
- Audit trail of task processing

### Claim Conflicts

```
Worker A                       TaskManager                      Worker B
   │                               │                                │
   │──Claim(task, "worker-a")─────▶│                                │
   │◀──────(OK)────────────────────│                                │
   │                               │                                │
   │                               │◀──Claim(task, "worker-b")──────│
   │                               │──────ErrTaskAlreadyClaimed────▶│
   │                               │                                │
   │──Claim(task, "worker-a")─────▶│  (re-claim same worker)        │
   │◀──────(OK, idempotent)────────│                                │
```

### Conflict Resolution

| Scenario | Behavior |
|----------|----------|
| Task pending, worker claims | Success, task now claimed |
| Task claimed by same worker | Success (idempotent) |
| Task claimed by different worker | `ErrTaskAlreadyClaimed` |
| Task completed | `ErrTaskCompleted` |
| Task failed | `ErrTaskFailed` |

### Stale Claim Handling

The current implementation does not automatically expire stale claims. Options:
1. Worker calls `Fail()` before shutting down
2. External process calls `Retry()` for stuck tasks
3. Future: claim TTL with automatic release

## Package Structure

```
tasks/
├── doc.go         # Package documentation
├── tasks.go       # Types, errors, TaskManager interface
├── manager.go     # Manager implementation
└── tasks_test.go  # Comprehensive tests
```

## Usage Patterns

### Basic Task Processing

```go
store := state.NewMemoryStore()
mgr := tasks.NewManager(store)

// Submit a task
task := tasks.Task{
    IdempotencyKey: "order-123-process",
    Payload:        []byte(`{"order_id": "123"}`),
    MaxAttempts:    3,
}
taskID, err := mgr.Submit(ctx, task)

// Worker loop
for {
    tasks, _ := mgr.List(ctx, tasks.StatusPending)
    for _, t := range tasks {
        if err := mgr.Claim(ctx, t.ID, "worker-1"); err != nil {
            continue // Another worker got it
        }
        
        result, err := processTask(t.Payload)
        if err != nil {
            mgr.Fail(ctx, t.ID, err)
        } else {
            mgr.Complete(ctx, t.ID, result)
        }
    }
}
```

### Idempotent Submission

```go
// Safe to retry - same task returned
id1, _ := mgr.Submit(ctx, Task{
    IdempotencyKey: "email-456-send",
    Payload:        []byte("email data"),
})

id2, _ := mgr.Submit(ctx, Task{
    IdempotencyKey: "email-456-send",
    Payload:        []byte("email data"),
})
// id1 == id2, no duplicate task
```

### Checking Task Status

```go
// By ID
task, err := mgr.Get(ctx, taskID)
if task.Status == tasks.StatusCompleted {
    fmt.Println("Result:", string(task.Result))
}

// By idempotency key
task, err := mgr.GetByIdempotencyKey(ctx, "order-123-process")
if err == tasks.ErrTaskNotFound {
    // Task was never submitted
}
```

### Cleanup Old Tasks

```go
// List and delete completed tasks
completed, _ := mgr.List(ctx, tasks.StatusCompleted)
for _, t := range completed {
    if time.Since(t.CompletedAt) > 24*time.Hour {
        mgr.Delete(ctx, t.ID)
    }
}
```

## Error Handling

| Error | Meaning | Recovery |
|-------|---------|----------|
| `ErrTaskNotFound` | Task does not exist | Check task ID |
| `ErrTaskAlreadyClaimed` | Another worker has it | Skip or wait |
| `ErrTaskNotClaimed` | Must claim before complete/fail | Call Claim first |
| `ErrTaskCompleted` | Task already done | No action needed |
| `ErrTaskFailed` | Task permanently failed | Check Error field |
| `ErrMaxAttemptsReached` | No more retries allowed | Manual intervention |
| `ErrInvalidTask` | Missing required fields | Include Payload |
| `ErrInvalidWorkerID` | Empty worker ID | Provide unique ID |
| `ErrWrongWorker` | Not the claiming worker | Only claimer can complete |
| `ErrStoreClosed` | Manager was closed | Create new manager |

## Testing Strategy

| Level | Focus |
|-------|-------|
| Unit | CRUD operations, state transitions |
| Idempotency | Duplicate submission handling |
| Concurrency | Multiple workers claiming same task |
| Retry | Attempt counting, max attempts |
| Lifecycle | Full pending→claimed→completed flow |
| Edge cases | Re-claim, double complete, empty payloads |
