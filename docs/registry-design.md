# Agent Registry Design

## What This Package Does

The `registry` package keeps track of which agents exist and what they can do. Agents register themselves ("I can do code review"), and other agents query the registry to find help ("Who can review code?").

## Why It Exists

In a swarm, agents need to find each other:

- **Task routing** — A coordinator receives a "review this code" task. Who should handle it? The registry answers: "Agent X can do code-review and has low load."
- **Load balancing** — Multiple agents might handle the same capability. The registry tracks load so work goes to the least-busy agent.
- **Dynamic membership** — Agents come and go. The registry provides a live view of who's currently available.

Without a registry, you'd need static configuration listing every agent and its capabilities. That doesn't scale and breaks when agents crash.

## When to Use It

**Use the registry for:**
- Finding agents by capability ("who can do X?")
- Load-aware task routing
- Tracking which agents are currently online
- Reacting when agents join or leave

**Don't use the registry for:**
- Health checks (use `heartbeat` for liveness)
- Storing agent state (use `state` package)
- Message routing (use `bus` package)

## Core Concepts

### Agent Information

Each registered agent has:
- **ID** — Unique identifier (typically a UUID)
- **Name** — Human-readable label
- **Capabilities** — List of things it can do (e.g., "code-review", "test", "deploy")
- **Status** — Current state ("idle", "busy", "stopping")
- **Load** — How busy it is (0.0 to 1.0)
- **Metadata** — Optional key-value pairs for extra info

### Registration

Agents register themselves when they start and update their entry periodically. Registration is upsert — if the agent exists, it updates; if not, it creates.

Registration typically happens alongside heartbeats: every few seconds, the agent re-registers with fresh load metrics.

### TTL and Expiry

Registrations can have a TTL (time-to-live). If an agent stops updating, its entry automatically expires. This prevents stale entries from dead agents cluttering the registry.

### Capabilities

Capabilities are simple strings. There's no hierarchy or wildcard matching — an agent either has a capability or it doesn't. Keep capability names consistent across your swarm.

Examples: "code-review", "image-generation", "web-search", "python-execution"

## Architecture


The registry has two implementations:

**MemoryRegistry** — In-memory storage for testing and single-node deployments. All data lives in Go maps.

**NATSRegistry** — Distributed storage using NATS JetStream KV. Supports clustering and replication. This is what you'd use in production.

Both implement the same interface, so code works unchanged across environments.

## Finding Agents

### By Capability

Query for agents that can handle a specific task. Results come back sorted by load (lowest first), making it easy to pick the least-busy agent.

### By Filter

More complex queries can filter by:
- Status (only "idle" agents)
- Maximum load (only agents below 50% load)
- Capability (combined with above)

### Load-Aware Selection

The registry doesn't make routing decisions — it provides data. Typical patterns:

- **Least loaded** — Always pick the first result (lowest load)
- **Random from top N** — Pick randomly from the 3 least-loaded agents
- **Threshold** — Only consider agents below a certain load

## Watching for Changes

You can subscribe to registry events to react when agents come and go:

- **Added** — New agent registered
- **Updated** — Existing agent changed (load, status)
- **Removed** — Agent deregistered or expired

This enables patterns like:
- Rescheduling work when an agent disappears
- Logging swarm membership changes
- Updating dashboards in real-time

## Integration with Heartbeat

Registry and heartbeat often work together:


1. Agent sends heartbeat (proves it's alive)
2. Heartbeat sender also updates registry (shares load/status)
3. If heartbeats stop, monitor fires callback
4. Callback deregisters the dead agent

This creates a complete lifecycle: agents automatically appear when healthy and disappear when dead.

## Common Patterns

### Self-Registration

Agent registers on startup with its capabilities. Periodically re-registers with updated load. Deregisters on graceful shutdown.

### Capability-Based Routing

Coordinator receives task, queries registry for capable agents, picks one (usually least loaded), assigns work.

### Supervisor Watching

A supervisor watches for removed events. When an agent disappears, the supervisor reassigns any pending work.

### Discovery Cache

For performance, cache registry results briefly. The registry is eventually consistent anyway — a few seconds of staleness is usually fine.

## Error Scenarios

| Situation | What Happens | What to Do |
|-----------|--------------|------------|
| Agent not found | Query returns empty | Expected — agent may have died |
| Registry closed | Operations fail | Reconnect or shut down |
| TTL expiry | Agent removed automatically | Normal cleanup behavior |
| Network partition | Registry may be stale | Accept eventual consistency |

## Design Decisions

### Why capabilities instead of roles?

Capabilities are more flexible. An agent can have multiple capabilities, and capability names are arbitrary. Roles suggest hierarchy; capabilities suggest composition.

### Why client-side load selection?

The registry returns data; the caller decides policy. Different tasks might want different selection strategies. Keeping selection logic out of the registry makes it more flexible.

### Why TTL-based cleanup?

TTL handles the common case (crashed agents) without requiring explicit deregistration. Combined with heartbeat integration, it creates robust self-healing behavior.

## Testing Strategy

| Level | Focus |
|-------|-------|
| Unit | CRUD operations, filtering logic, event delivery |
| Integration | Multi-agent registration and discovery |
| Concurrency | Simultaneous register/deregister calls |
| TTL | Expiry timing and cleanup |
| Watch | Event ordering, channel behavior |
