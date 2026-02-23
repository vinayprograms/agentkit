# Task Queue Example

A distributed task queue demonstrating agentkit's message bus and registry for coordinated work distribution.

## Overview

This example shows how to build a task distribution system using:
- **Message Bus** for publishing tasks and receiving results
- **Queue Subscriptions** for load-balanced task distribution
- **Registry** for worker discovery and status tracking

## Architecture

```
                    ┌─────────────────┐
                    │   Coordinator   │
                    │ (coordinator.go)│
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              │ tasks.queue  │              │ tasks.complete
              ▼              │              │ tasks.failed
        ┌─────────┐          │         ┌────▲────┐
        │ Worker  │──────────┼─────────│ Worker  │
        │    1    │          │         │    2    │
        └─────────┘          │         └─────────┘
                    (Queue Group: "workers")
```

## Components

### Coordinator (`coordinator.go`)
- Publishes tasks to `tasks.queue`
- Subscribes to `tasks.complete` and `tasks.failed` for results
- Tracks pending tasks and completion statistics
- Monitors worker registrations via registry Watch

### Worker (`worker.go`)
- Registers with capabilities in the registry
- Queue subscribes to `tasks.queue` (load-balanced across workers)
- Processes tasks based on type (compute, fetch, process)
- Reports results to completion/failure subjects
- Sends heartbeats to keep registry entry fresh

## Task Types

| Type | Description | Simulated Work |
|------|-------------|----------------|
| `compute` | CPU-bound operations | fibonacci, factorial, prime check |
| `fetch` | Network requests | Simulated HTTP fetch with latency |
| `process` | Data transformation | String processing |

## How to Run

### Quick Demo (Embedded Workers)

The coordinator includes embedded workers for easy testing:

```bash
cd examples/task-queue
go run coordinator.go
```

This runs a coordinator with 2 embedded workers that process sample tasks automatically.

### Distributed Mode

For proper distributed operation, use a shared message bus (NATS):

**Terminal 1 - Start NATS:**
```bash
docker run -p 4222:4222 nats:latest
```

**Terminal 2 - Coordinator:**
```bash
# Modify coordinator.go to use NATS bus instead of memory bus
go run coordinator.go
```

**Terminals 3, 4, ... - Workers:**
```bash
go run worker.go worker-1
go run worker.go worker-2
```

## Example Output

**Coordinator:**
```
Task Coordinator started
Waiting for workers to connect...
Worker joined: worker-1 (capabilities: [compute fetch process])
Worker joined: worker-2 (capabilities: [compute fetch process])

--- Submitting sample tasks ---
Submitted: task-1 (compute)
Submitted: task-2 (compute)
Submitted: task-3 (fetch)
Submitted: task-4 (process)
Submitted: task-5 (compute)
✓ Task task-1 completed by worker-1 (156ms)
✓ Task task-2 completed by worker-2 (234ms)
✓ Task task-3 completed by worker-1 (312ms)
✓ Task task-4 completed by worker-2 (178ms)
✓ Task task-5 completed by worker-1 (145ms)
Status: 0 pending, 5 completed, 2 workers active

^C
Shutting down...

=== Final Statistics ===
Tasks submitted: 5
Tasks completed: 5
Tasks pending:   0
Success rate:    100.0%
```

## Key Concepts Demonstrated

1. **QueueSubscribe** - Multiple workers share a queue group for automatic load balancing
2. **Registry Watch** - Real-time notifications when workers join/leave
3. **Status Updates** - Workers update registry status (idle/busy) and load
4. **Heartbeat** - Workers refresh registry entries to prevent TTL expiry
5. **Pub/Sub for Results** - Decoupled result reporting via dedicated subjects

## Message Flow

1. Coordinator publishes task to `tasks.queue`
2. One worker (via queue group) receives the task
3. Worker sets status to "busy", processes task
4. Worker publishes result to `tasks.complete` or `tasks.failed`
5. Worker sets status to "idle"
6. Coordinator receives result, updates statistics

## Notes

- The in-memory bus only works within a single process
- For multi-process distribution, switch to NATS backend
- Workers automatically re-register if their entry expires
- Task processing has simulated random failures (10-15%)
