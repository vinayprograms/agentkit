# Results Design

## What This Package Does

The `results` package lets agents publish task results and other agents subscribe to them. When a task completes, interested parties receive a notification without polling.

## Why It Exists

In distributed workflows, one agent often needs to know when another agent finishes work:

- **Pipeline coordination** — Agent B waits for Agent A's result before starting
- **Progress tracking** — A coordinator monitors task completion across workers
- **Async responses** — Request something, do other work, get notified when it's ready

Without a results system, agents would need to poll repeatedly ("is it done yet?"). That wastes resources and adds latency. Push-based notifications are more efficient.

## When to Use It

**Use results for:**
- Publishing task outputs for other agents
- Subscribing to task completion notifications
- Tracking task status (pending → success/failed)
- Coordinating multi-stage pipelines

**Don't use results for:**
- Persistent storage (use `state` — results may live only in memory)
- Streaming data (use `bus` — results are single values, not streams)
- Complex queries (this is simple filtering, not a database)

## Core Concepts

### Result Lifecycle

A result moves through three states:

1. **Pending** — Task is in progress
2. **Success** — Task completed successfully (with output)
3. **Failed** — Task failed (with error message)

Success and failed are **terminal states**. Once a result reaches them, subscriptions auto-close.


### Publishing

When a task makes progress or completes, the worker publishes an update:
- First publish creates the result (pending status)
- Subsequent publishes update the result
- Final publish sets status to success or failed

Publishing is idempotent — you can publish the same result multiple times safely.

### Subscribing

Agents can subscribe to a task ID to receive updates:
- If the result already exists, it's delivered immediately
- As the result updates, subscribers receive each change
- When the result reaches a terminal state, the subscription closes

Subscriptions are push-based — you receive a channel and read from it. No polling.


### Filtering

You can list results matching criteria:
- By status (only pending, only failed)
- By task ID prefix (all tasks starting with "batch-123-")
- By time range (created in the last hour)
- By metadata (all tasks with `type=email`)

## Architecture


Two implementations:

**MemoryPublisher** — In-memory storage and notifications. Results live only in this process. Good for testing and single-process workflows.

**BusPublisher** — Uses the message bus for notifications. Each process has its own result cache, but publishes/subscribes go through the bus. Good for distributed systems where multiple agents need to coordinate.

## Common Patterns

### Wait for Completion

Subscribe to a task, then block until you receive a terminal result:

1. Subscribe to task ID
2. Read from channel
3. Check if status is terminal
4. If terminal, you have the final result
5. If not, keep reading

### Fire and Forget with Callback

Start a task, register a subscription, then continue with other work. When the result arrives on the channel, handle it (or use a goroutine to wait).

### Progress Updates

Publish intermediate results with metadata (e.g., "50% complete"). Subscribers see each update. Final publish sets terminal status.

### Batch Monitoring

Subscribe to multiple task IDs. As each completes, update your tracking. Useful for coordinators managing many parallel tasks.

## Error Scenarios

| Situation | What Happens | What to Do |
|-----------|--------------|------------|
| Task doesn't exist | Subscribe waits indefinitely | Set a timeout, or check if task was submitted |
| Publisher closed | All subscriptions close | Expected during shutdown |
| Result not found | Get/Delete return error | Task may not have published yet |
| Network partition (BusPublisher) | Notifications delayed | Results still work locally |

## Design Decisions

### Why auto-close on terminal?

Once a result is final, there are no more updates to send. Keeping subscriptions open would leak resources. Auto-close is the clean pattern.

### Why not persist results?

Results serve coordination, not storage. If you need durability, publish results AND store them in the state package. This keeps the results package simple.

### Why local cache in BusPublisher?

Performance. Reads come from local cache (fast). Writes go to the bus (distributed). The cache is eventually consistent — writes propagate via bus notifications.

## Integration Notes

### With Tasks Package

Tasks track work items with claims and retries. Results track outputs. A typical flow:
1. Coordinator submits task (tasks package)
2. Worker claims and processes task
3. Worker publishes result (results package)
4. Coordinator receives notification

### With Bus Package

BusPublisher uses the bus for notifications. Subject format: `results.{taskID}`. Subscribers listen for updates on these subjects.

### With Shutdown

On shutdown, call `Close()`. This closes all subscription channels and releases resources. Subscribers receive channel close and can exit cleanly.

## Testing Strategy

| Level | Focus |
|-------|-------|
| Unit | Publish/get/subscribe/delete operations |
| Lifecycle | Pending → success/failed transitions |
| Subscription | Immediate delivery, updates, auto-close |
| Concurrency | Multiple subscribers, concurrent publishes |
| Filter | Status, prefix, time range, metadata |
