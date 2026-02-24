# Task Queue Example

Demonstrates work distribution using the message bus queue groups.

## Components

- **coordinator/** - Submits tasks and collects results
- **worker/** - Processes tasks from the queue

## Running

Start one or more workers:
```bash
go run ./examples/task-queue/worker/
go run ./examples/task-queue/worker/  # second worker
```

In another terminal, start the coordinator:
```bash
go run ./examples/task-queue/coordinator/
```

## What This Shows

- Queue groups for load balancing
- Work distribution across multiple workers
- Result collection
