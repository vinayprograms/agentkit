# Message Bus Design

## What This Package Does

The `bus` package lets agents send messages to each other. It's the communication backbone of a swarm — agents publish messages, subscribe to topics, and make requests that expect responses.

## Why It Exists

Agents in a swarm need to communicate:

- **Broadcasting events** — "I finished task X" or "Config changed"
- **Distributing work** — Send tasks to a pool of workers
- **Requesting information** — "What's the status of task Y?"

Without a message bus, agents would need direct connections to each other, creating a tangled web that doesn't scale. The bus provides a central communication layer where agents publish to topics and subscribe to what they care about.

## When to Use It

**Use the bus for:**
- Event notifications between agents
- Work distribution across worker pools
- Request/reply interactions (RPC-style)
- Heartbeats and status broadcasts

**Don't use the bus for:**
- Storing data (use `state` package)
- Large file transfers (too much overhead)
- Guaranteed delivery (messages can be dropped if buffers fill)

## Core Concepts

### Subjects

Messages are sent to **subjects** — string identifiers like `events.task.completed` or `heartbeat.agent-a`. Agents subscribe to subjects they care about. Dots are conventional separators but have no special meaning.

### Publish/Subscribe (Fan-out)

When you publish to a subject, **all subscribers receive the message**. This is broadcast semantics — useful for events where everyone needs to know.

![Publish/Subscribe pattern - all subscribers receive every message](images/bus-pubsub.png)

### Queue Groups (Load Balancing)

Sometimes you want only **one subscriber** to handle each message (work distribution). Queue groups solve this — multiple subscribers join a named group, and messages are distributed round-robin among them.

![Queue Groups pattern - messages load-balanced across workers](images/bus-queue.png)

This enables horizontal scaling: add more workers to the queue group, and work automatically distributes across them.

### Request/Reply (RPC)

For synchronous interactions, the bus supports request/reply. You send a request and wait for a single response. Under the hood, this uses a temporary reply subject.

![Request/Reply pattern - synchronous RPC with reply](images/bus-reqreply.png)

This is useful when an agent needs information from another agent and can't proceed without it.

## Architecture

The `MessageBus` interface has two implementations:

**MemoryBus** — In-memory implementation for testing and single-process scenarios. All subscriptions and routing happen in Go maps protected by mutexes. No network involved.

**NATSBus** — Production implementation using NATS messaging server. Provides clustering, persistence options, and battle-tested reliability. This is what you'd use in a real deployment.

Both implement the same interface, so code works unchanged whether you're testing locally or running in production.

## Message Flow

1. **Publisher** calls `Publish(subject, data)`
2. **Bus** routes message to all matching subscribers
3. **Subscribers** receive message on their channel
4. For queue groups, only one member receives each message

Messages are fire-and-forget from the publisher's perspective. The bus handles routing, but doesn't guarantee delivery if subscribers are slow or offline.

## Common Patterns

### Event Broadcasting

One agent publishes events; many agents subscribe to react. Example: an agent completes a task and broadcasts the result. Other agents (UI, logging, dependent tasks) all receive the notification.

### Work Queue

A coordinator publishes tasks to a subject. Multiple workers subscribe via a queue group. Each task goes to exactly one worker. As workers finish, they're automatically given more work.

### Service Discovery

Agents periodically publish heartbeats to a known subject. A monitor subscribes and tracks which agents are alive. If heartbeats stop, the monitor knows the agent died.

### Scatter/Gather

Publisher sends a request to multiple agents (via pub/sub). Each responds to the reply subject. Publisher collects responses until timeout. Useful for "ask everyone and aggregate results."

## Error Scenarios

| Situation | What Happens | What to Do |
|-----------|--------------|------------|
| No subscribers | Message is dropped | Expected for some subjects |
| Subscriber buffer full | Message dropped for that subscriber | Increase buffer or process faster |
| Request timeout | No response received in time | Retry or fail gracefully |
| Bus closed | All operations fail | Reconnect or shut down |
| NATS disconnected | NATSBus auto-reconnects | Configure reconnect settings |

## Delivery Guarantees

The bus provides **at-most-once delivery**:
- Messages might not arrive (network issues, full buffers)
- Messages never arrive twice
- No ordering guarantees across subjects

If you need guaranteed delivery, use the `state` package for persistence or implement acknowledgment on top of the bus.

## Choosing Between Patterns

| Need | Pattern | Why |
|------|---------|-----|
| Everyone should know | Pub/Sub | Fan-out to all subscribers |
| One handler per message | Queue Group | Load balancing |
| Need a response | Request/Reply | Synchronous RPC |
| Broadcast + one handler | Both | Pub/sub for events, queue for work |

## Testing Strategy

| Level | Focus |
|-------|-------|
| Unit | Message routing, queue group distribution |
| Integration | Multi-agent communication flows |
| Concurrency | Race conditions, deadlocks |
| Failure | Connection drops, timeouts, buffer overflow |
