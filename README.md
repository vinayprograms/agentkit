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

## Used By

- [headless-agent](https://github.com/vinayprograms/agent) — Goal-oriented headless agent
- coordinator (coming soon) — User-facing agent orchestrator

## License

Apache-2.0
