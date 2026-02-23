# Result Publication Example

Demonstrates task result publication using agentkit's `results` package.

## Overview

The `results` package provides a way to store, retrieve, and subscribe to task results. This decouples task execution from result consumption, enabling async workflows and distributed coordination.

## Key Concepts

### ResultPublisher Interface

The core interface for storing and retrieving results:

```go
type ResultPublisher interface {
    Publish(ctx context.Context, taskID string, result Result) error
    Get(ctx context.Context, taskID string) (*Result, error)
    Subscribe(taskID string) (<-chan *Result, error)
    List(filter ResultFilter) ([]*Result, error)
    Delete(ctx context.Context, taskID string) error
    Close() error
}
```

### Result Statuses

- `StatusPending` - Task is in progress
- `StatusSuccess` - Task completed successfully
- `StatusFailed` - Task failed

### Publisher Implementations

1. **MemoryPublisher** - In-memory storage, ideal for testing and single-process
2. **BusPublisher** - Uses message bus for distributed notifications

## Running the Example

```bash
cd examples/result-publication
go run main.go
```

## Code Walkthrough

### Creating Publishers

```go
// In-memory publisher (testing, single-process)
pub := results.NewMemoryPublisher()
defer pub.Close()

// Bus-backed publisher (distributed systems)
memBus := bus.NewMemoryBus(bus.DefaultConfig())
pub := results.NewBusPublisher(memBus, results.BusPublisherConfig{
    SubjectPrefix: "results",
    BufferSize:    16,
})
```

### Publishing Results

```go
// Publish pending (task started)
pub.Publish(ctx, "task-001", results.Result{
    TaskID: "task-001",
    Status: results.StatusPending,
    Metadata: map[string]string{
        "worker": "agent-1",
    },
})

// Update to success (task completed)
pub.Publish(ctx, "task-001", results.Result{
    TaskID: "task-001",
    Status: results.StatusSuccess,
    Output: []byte(`{"answer": 42}`),
})

// Or publish failure
pub.Publish(ctx, "task-002", results.Result{
    TaskID: "task-002",
    Status: results.StatusFailed,
    Error:  "connection timeout",
})
```

### Subscribing to Updates

```go
ch, err := pub.Subscribe("task-001")
if err != nil {
    log.Fatal(err)
}

for result := range ch {
    fmt.Printf("Status: %s\n", result.Status)
    if result.Status.IsTerminal() {
        // Task finished (success or failed)
        break
    }
}
```

Subscriptions auto-close when:
- The task reaches a terminal state (success/failed)
- The publisher is closed

### Filtering Results

```go
// By status
successful, _ := pub.List(results.ResultFilter{
    Status: results.StatusSuccess,
})

// By task ID prefix
batch, _ := pub.List(results.ResultFilter{
    TaskIDPrefix: "batch-",
    Limit:        10,
})

// By metadata
computeTasks, _ := pub.List(results.ResultFilter{
    Metadata: map[string]string{
        "type": "compute",
    },
})

// Combined filters
pub.List(results.ResultFilter{
    Status:       results.StatusSuccess,
    TaskIDPrefix: "batch-",
    Metadata: map[string]string{
        "priority": "high",
    },
    Limit: 5,
})
```

## Production Usage

For distributed systems, use NATS as the message bus:

```go
natsBus, _ := bus.NewNATSBus(bus.NATSConfig{
    URL: "nats://localhost:4222",
})
defer natsBus.Close()

pub := results.NewBusPublisher(natsBus, results.DefaultBusPublisherConfig())
```

This enables:
- Multiple processes to publish/subscribe to results
- Notifications across network boundaries
- Decoupled producers and consumers

## Typical Workflow

1. **Coordinator** creates a task and subscribes to its result
2. **Worker** receives the task, publishes `StatusPending`
3. **Worker** processes the task
4. **Worker** publishes `StatusSuccess` or `StatusFailed`
5. **Coordinator** receives the update and continues

```
Coordinator                    Worker
    |                            |
    |-- Create task, Subscribe --|
    |                            |
    |                            |-- Publish(Pending)
    |<--- Receive Pending -------|
    |                            |
    |                            |-- [Process task]
    |                            |
    |                            |-- Publish(Success)
    |<--- Receive Success -------|
    |                            |
    |-- Act on result ---------->|
```

## See Also

- `results/doc.go` - Package documentation
- `results/results.go` - Core types and interfaces
- `results/memory.go` - MemoryPublisher implementation
- `results/bus.go` - BusPublisher implementation
