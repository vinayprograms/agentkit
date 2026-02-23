# AgentKit

Reusable Go packages for building AI agents.

## Packages

| Package | Description |
|---------|-------------|
| `llm` | LLM provider adapters (OpenAI, Anthropic, Google, Mistral, Groq, Ollama) |
| `memory` | Semantic memory (FIL model) + scratchpad (KV) with BM25 search |
| `tools` | Tool registry, built-in tools, MCP integration |
| `mcp` | Model Context Protocol client |
| `acp` | Agent Communication Protocol (agent-to-agent messaging) |
| `security` | Taint tracking, content verification, audit trail |
| `credentials` | Credential management (TOML-based) |
| `transport` | Pluggable transports (stdio, WebSocket, SSE) with JSON-RPC 2.0 |
| `bus` | Message bus clients (NATS, in-memory) for pub/sub and request/reply |
| `heartbeat` | Agent liveness detection and death detection for swarms |
| `state` | Shared state (KV store, distributed locks) for agent coordination |
| `registry` | Agent registration and discovery for swarm coordination |
| `ratelimit` | Rate limit coordination across agents in a swarm |
| `results` | Task result publication and retrieval for agent coordination |
| `tasks` | Idempotent task handling with deduplication for safe retries |
| `errors` | Structured error taxonomy with codes, categories, and retry semantics |
| `shutdown` | Graceful shutdown coordination with signal handling |
| `policy` | Security policy enforcement |
| `logging` | Structured logging |
| `telemetry` | Observability (OpenTelemetry) |

## Installation

```bash
go get github.com/vinayprograms/agentkit
```

## Usage

```go
import (
    "github.com/vinayprograms/agentkit/llm"
    "github.com/vinayprograms/agentkit/memory"
    "github.com/vinayprograms/agentkit/tools"
)

// Create an LLM provider
provider, _ := llm.NewOpenAIProvider(llm.OpenAIConfig{
    APIKey: os.Getenv("OPENAI_API_KEY"),
    Model:  "gpt-4o",
})

// Create memory store
embedder, _ := memory.NewOpenAIEmbedder(apiKey, "text-embedding-3-small")
store, _ := memory.NewBleveStore(memory.BleveStoreConfig{
    BasePath: "./memory",
    Embedder: embedder,
})

// Create tool registry
registry := tools.NewRegistry()
registry.SetScratchpad(tools.NewInMemoryStore(), false)
registry.SetSemanticMemory(memory.NewToolsAdapter(store))
```

## Transport Layer

The transport package provides pluggable transports for JSON-RPC 2.0 communication:

```go
import "github.com/vinayprograms/agentkit/transport"

// Stdio transport (for CLI tools)
t := transport.NewStdioTransport(os.Stdin, os.Stdout, transport.DefaultConfig())

// WebSocket transport (for real-time UIs)
conn, _ := upgrader.Upgrade(w, r, nil)
t := transport.NewWebSocketTransport(conn, transport.DefaultWebSocketConfig())

// SSE transport (for web clients)
t := transport.NewSSETransport(transport.DefaultSSEConfig())
http.HandleFunc("/events", t.HandleSSE)  // Server→Client
http.HandleFunc("/rpc", t.HandlePost)    // Client→Server

// Common usage pattern
go t.Run(ctx)
for msg := range t.Recv() {
    // Handle request
    t.Send(&transport.OutboundMessage{Response: resp})
}
```

All transports use channel-based APIs for Go-idiomatic concurrent use.

## Message Bus

The bus package provides pub/sub and request/reply messaging for agent communication:

```go
import "github.com/vinayprograms/agentkit/bus"

// NATS bus (production)
nbus, _ := bus.NewNATSBus(bus.NATSConfig{URL: "nats://localhost:4222"})

// Memory bus (testing)
mbus := bus.NewMemoryBus(bus.DefaultConfig())

// Pub/Sub
sub, _ := nbus.Subscribe("events.user")
for msg := range sub.Messages() {
    fmt.Printf("Received: %s\n", msg.Data)
}

// Queue groups (load balanced)
sub, _ := nbus.QueueSubscribe("tasks", "workers")

// Request/Reply
reply, _ := nbus.Request("service.echo", []byte("ping"), 5*time.Second)
```

Queue groups enable natural scaling: messages are load-balanced across agents subscribed to the same queue.

## Agent Registry

The registry package provides agent registration and discovery for swarm coordination:

```go
import "github.com/vinayprograms/agentkit/registry"

// In-memory registry (testing/single-node)
reg := registry.NewMemoryRegistry(registry.MemoryConfig{TTL: 30 * time.Second})

// NATS registry (distributed)
nbus, _ := bus.NewNATSBus(bus.NATSConfig{URL: "nats://localhost:4222"})
reg, _ := registry.NewNATSRegistry(nbus.Conn(), registry.NATSRegistryConfig{
    BucketName: "my-swarm",
    TTL:        30 * time.Second,
})

// Register an agent
reg.Register(registry.AgentInfo{
    ID:           "agent-1",
    Name:         "Code Review Agent",
    Capabilities: []string{"code-review", "testing"},
    Status:       registry.StatusIdle,
    Load:         0.3,
})

// Discover agents by capability (sorted by load)
agents, _ := reg.FindByCapability("code-review")
target := agents[0] // Pick the least loaded agent

// Watch for changes
events, _ := reg.Watch()
for event := range events {
    fmt.Printf("%s: %s\n", event.Type, event.Agent.ID)
}
```

Agents self-register with capabilities, status, and load. The registry enables capability-based routing with load-aware selection.

## Heartbeat System

The heartbeat package provides agent liveness detection for distributed swarms:

```go
import "github.com/vinayprograms/agentkit/heartbeat"

// Create sender (from agent)
sender, _ := heartbeat.NewBusSender(heartbeat.SenderConfig{
    Bus:      nbus,
    AgentID:  "agent-1",
    Interval: 5 * time.Second,
})
sender.SetStatus("busy")
sender.SetLoad(0.75)
sender.SetMetadata("version", "1.0.0")
sender.Start(ctx)

// Create monitor (from coordinator)
monitor, _ := heartbeat.NewBusMonitor(heartbeat.MonitorConfig{
    Bus:     nbus,
    Timeout: 15 * time.Second, // 3 missed heartbeats
})
monitor.OnDead(func(agentID string) {
    log.Printf("Agent %s presumed dead, triggering failover", agentID)
})
ch, _ := monitor.WatchAll()
for hb := range ch {
    fmt.Printf("Heartbeat from %s: status=%s load=%.2f\n", hb.AgentID, hb.Status, hb.Load)
}

// Check if specific agent is alive
if monitor.IsAlive("agent-1", 15*time.Second) {
    // Agent is responsive
}
```

Heartbeats enable swarm coordinators to detect dead agents and trigger failover automatically.

## Shared State

The state package provides distributed key-value storage with locking for agent coordination:

```go
import "github.com/vinayprograms/agentkit/state"

// In-memory store (testing)
store := state.NewMemoryStore()

// NATS JetStream KV (production)
nbus, _ := bus.NewNATSBus(bus.NATSConfig{URL: "nats://localhost:4222"})
store, _ := state.NewNATSStore(state.NATSStoreConfig{
    Conn:   nbus.Conn(),
    Bucket: "agent-state",
})

// Key-value operations
store.Put("config.timeout", []byte("30s"), time.Hour) // with TTL
val, _ := store.Get("config.timeout")

// Watch for changes
ch, _ := store.Watch("config.*")
for kv := range ch {
    fmt.Printf("Key %s changed: %s\n", kv.Key, kv.Value)
}

// Distributed locking
lock, _ := store.Lock("resource.mutex", 30*time.Second)
defer lock.Unlock()
// ... critical section ...
lock.Refresh() // Extend TTL if needed
```

Supports TTL-based expiry, pattern-based key listing and watch, and distributed locks with automatic expiry.

## Rate Limiting

The ratelimit package provides coordinated rate limiting across agents in a swarm:

```go
import "github.com/vinayprograms/agentkit/ratelimit"

// In-memory limiter (single process)
limiter := ratelimit.NewMemoryLimiter()
limiter.SetCapacity("openai-api", 60, time.Minute) // 60 requests per minute

// Distributed limiter (swarm coordination)
nbus, _ := bus.NewNATSBus(bus.NATSConfig{URL: "nats://localhost:4222"})
limiter, _ := ratelimit.NewDistributedLimiter(ratelimit.DistributedConfig{
    Bus:     nbus,
    AgentID: "agent-1",
})
limiter.SetCapacity("shared-api", 100, time.Minute)

// Acquire a token (blocks until available)
if err := limiter.Acquire(ctx, "openai-api"); err != nil {
    return err // context cancelled
}
defer limiter.Release("openai-api")

// Non-blocking attempt
if limiter.TryAcquire("openai-api") {
    defer limiter.Release("openai-api")
    // Make request
} else {
    // Handle rate limit (queue, retry later, etc.)
}

// Announce reduced capacity (after 429 response)
limiter.AnnounceReduced("shared-api", "received 429 from API")

// Check current capacity
cap := limiter.GetCapacity("shared-api")
fmt.Printf("Available: %d/%d\n", cap.Available, cap.Total)
```

The distributed limiter coordinates across agents via the message bus. When any agent announces reduced capacity (e.g., after receiving a 429 response), all agents automatically adjust their limits. Capacity gradually recovers over time.

## Result Publication

The results package provides task result publication and retrieval for agent coordination:

```go
import "github.com/vinayprograms/agentkit/results"

// In-memory publisher (testing)
pub := results.NewMemoryPublisher()

// Bus-backed publisher (distributed)
nbus, _ := bus.NewNATSBus(bus.NATSConfig{URL: "nats://localhost:4222"})
pub := results.NewBusPublisher(nbus, results.BusPublisherConfig{
    SubjectPrefix: "results",
})

// Publish a result
err := pub.Publish(ctx, "task-123", results.Result{
    TaskID:   "task-123",
    Status:   results.StatusSuccess,
    Output:   []byte(`{"answer": 42}`),
    Metadata: map[string]string{"model": "gpt-4"},
})

// Retrieve a result
result, err := pub.Get(ctx, "task-123")

// Subscribe to result updates (non-blocking)
ch, _ := pub.Subscribe("task-456")
for result := range ch {
    fmt.Printf("Result update: %s -> %s\n", result.TaskID, result.Status)
    if result.Status.IsTerminal() {
        break // Channel closes on terminal state
    }
}

// List results with filters
pending, _ := pub.List(results.ResultFilter{
    Status:       results.StatusPending,
    TaskIDPrefix: "batch-",
    Limit:        10,
})
```

The bus-backed publisher enables distributed subscribers to receive real-time result notifications without polling.

## Idempotent Task Handling

The tasks package provides safe task retries with deduplication:

```go
import "github.com/vinayprograms/agentkit/tasks"

// Create task manager backed by state store
store := state.NewMemoryStore()
mgr := tasks.NewManager(store)

// Submit a task with an idempotency key
task := tasks.Task{
    IdempotencyKey: "order-123-process",
    Payload:        []byte(`{"order_id": "123"}`),
    MaxAttempts:    3,
}
taskID, _ := mgr.Submit(ctx, task)

// Duplicate submissions return the same task ID (idempotent)
taskID2, _ := mgr.Submit(ctx, tasks.Task{
    IdempotencyKey: "order-123-process",
    Payload:        []byte(`{"order_id": "123"}`),
})
// taskID == taskID2

// Worker claims and processes the task
err := mgr.Claim(ctx, taskID, "worker-1")
// ... do work ...

// Complete on success
err = mgr.Complete(ctx, taskID, []byte(`{"status": "processed"}`))

// Or fail with automatic retry
err = mgr.Fail(ctx, taskID, errors.New("temporary failure"))
// Task moves back to pending if attempts < MaxAttempts

// List pending tasks for processing
pending, _ := mgr.List(ctx, tasks.StatusPending)
```

The idempotency key ensures duplicate task submissions (e.g., from retried network requests) don't create duplicate work. Tasks support configurable retry limits with automatic state transitions.

## Graceful Shutdown

The shutdown package provides graceful shutdown coordination with signal handling:

```go
import "github.com/vinayprograms/agentkit/shutdown"

// Create coordinator
coord := shutdown.NewCoordinator(shutdown.DefaultConfig())

// Handle OS signals (SIGTERM, SIGINT)
coord.HandleSignals()

// Register shutdown handlers with phases (lower = earlier)
coord.RegisterWithPhase("http-server", httpServer, 10)   // Stop accepting requests
coord.RegisterWithPhase("workers", workerPool, 20)       // Drain work queues
coord.RegisterWithPhase("database", dbConn, 30)          // Close connections

// Or use a function directly
coord.RegisterFunc("cleanup", func(ctx context.Context) error {
    // Finish current task
    // Re-queue pending work
    // Release resources
    return nil
})

// Wait for shutdown to complete
<-coord.Done()
if err := coord.Err(); err != nil {
    log.Printf("Shutdown error: %v", err)
}

// Or trigger manually with timeout
if err := coord.ShutdownWithTimeout(30 * time.Second); err != nil {
    log.Printf("Shutdown incomplete: %v", err)
}
```

Handlers in the same phase are shut down concurrently. Lower phase numbers shut down first, ensuring dependencies are respected (e.g., stop accepting requests before draining workers, close database last).

## Structured Errors

The errors package provides a comprehensive error taxonomy for swarm coordination:

```go
import "github.com/vinayprograms/agentkit/errors"

// Create typed errors
err := errors.New(errors.ErrCodeTimeout, "operation timed out")
err := errors.NotFound("agent agent-1 not found")
err := errors.RateLimited("API quota exceeded", errors.WithMetadata("retry_after", "60s"))

// Wrap existing errors with context
if err := doSomething(); err != nil {
    return errors.Wrap(err, "processing request")
}

// Check error properties for retry decisions
if agentErr := errors.AsAgentError(err); agentErr != nil {
    if agentErr.Retryable() {
        // Safe to retry
    }
    fmt.Printf("Code: %s, Category: %s\n", agentErr.Code(), agentErr.Category())
}

// Convenience checks
if errors.IsRetryable(err) { /* retry */ }
if errors.Is(err, errors.ErrCodeRateLimit) { /* backoff */ }
if errors.IsTransient(err) { /* temporary failure */ }

// JSON serialization for cross-agent communication
data, _ := json.Marshal(err)
var received errors.Error
json.Unmarshal(data, &received)
```

Error categories enable automatic retry decisions:
- **Transient**: Temporary failures (timeout, network) — retry may succeed
- **Permanent**: Non-recoverable (not found, invalid input) — don't retry
- **Resource**: Quota/limit issues (rate limit, capacity) — retry with backoff
- **Internal**: Bugs or unexpected errors — investigate

## Used By

- [headless-agent](https://github.com/vinayprograms/agent) — Goal-oriented headless agent
- coordinator (coming soon) — User-facing agent orchestrator

## License

Apache-2.0
