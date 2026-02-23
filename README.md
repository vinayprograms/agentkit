# AgentKit

Reusable Go packages for building AI agent swarms.

## What Is This?

AgentKit provides the building blocks for distributed AI agents:

- **LLM integration** — Unified interface for Claude, GPT, Gemini, and more
- **Agent coordination** — Registry, heartbeat, message bus for swarm behavior
- **Distributed state** — Shared key-value storage with locking
- **Task management** — Idempotent tasks with retry semantics
- **Observability** — Structured logging and OpenTelemetry tracing

## Documentation

📚 **[Design Docs](docs/)** — Understand how each package works before using it.

| Package | Purpose | Design Doc |
|---------|---------|------------|
| [llm](llm/) | LLM provider abstraction | [llm-design.md](docs/llm-design.md) |
| [bus](bus/) | Message bus (pub/sub, request/reply) | [bus-design.md](docs/bus-design.md) |
| [registry](registry/) | Agent registration and discovery | [registry-design.md](docs/registry-design.md) |
| [heartbeat](heartbeat/) | Liveness detection | [heartbeat-design.md](docs/heartbeat-design.md) |
| [state](state/) | Distributed key-value store | [state-design.md](docs/state-design.md) |
| [tasks](tasks/) | Idempotent task handling | [tasks-design.md](docs/tasks-design.md) |
| [results](results/) | Result publication | [results-design.md](docs/results-design.md) |
| [ratelimit](ratelimit/) | Coordinated rate limiting | [ratelimit-design.md](docs/ratelimit-design.md) |
| [shutdown](shutdown/) | Graceful shutdown | [shutdown-design.md](docs/shutdown-design.md) |
| [errors](errors/) | Structured error taxonomy | [errors-design.md](docs/errors-design.md) |
| [transport](transport/) | JSON-RPC transports | [transport-design.md](docs/transport-design.md) |
| [mcp](mcp/) | MCP client for external tools | [mcp-design.md](docs/mcp-design.md) |
| [acp](acp/) | Editor integration protocol | [acp-design.md](docs/acp-design.md) |
| [memory](memory/) | Semantic memory (FIL model) | [memory-design.md](docs/memory-design.md) |
| [logging](logging/) | Structured logging | [logging-design.md](docs/logging-design.md) |
| [telemetry](telemetry/) | OpenTelemetry tracing | [telemetry-design.md](docs/telemetry-design.md) |

📂 **[Examples](examples/)** — Working code you can run.

| Example | What It Shows |
|---------|---------------|
| [chat-transport](examples/chat-transport/) | Basic transport setup |
| [task-queue](examples/task-queue/) | Work distribution via bus |
| [swarm-heartbeat](examples/swarm-heartbeat/) | Agent liveness detection |
| [graceful-shutdown](examples/graceful-shutdown/) | Multi-phase shutdown |
| [idempotent-tasks](examples/idempotent-tasks/) | Safe task retries |
| [rate-limiting](examples/rate-limiting/) | Coordinated rate limits |
| [result-publication](examples/result-publication/) | Pub/sub for results |
| [structured-errors](examples/structured-errors/) | Error handling patterns |

## Getting Started

### Installation

```bash
go get github.com/vinayprograms/agentkit
```

### Learning Path

**New to AgentKit?** Read the docs in this order:

1. **[bus-design.md](docs/bus-design.md)** — Understand how agents communicate
2. **[registry-design.md](docs/registry-design.md)** — How agents find each other
3. **[heartbeat-design.md](docs/heartbeat-design.md)** — Detecting dead agents
4. **[state-design.md](docs/state-design.md)** — Sharing data between agents
5. **[llm-design.md](docs/llm-design.md)** — Calling LLMs

Then explore based on what you're building:

- **Building a swarm?** → tasks, results, ratelimit, shutdown
- **Editor integration?** → acp, transport
- **External tools?** → mcp
- **Observability?** → logging, telemetry, errors

### Quick Example

Here's a minimal agent that registers itself, sends heartbeats, and responds to work:

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/vinayprograms/agentkit/bus"
    "github.com/vinayprograms/agentkit/heartbeat"
    "github.com/vinayprograms/agentkit/registry"
)

func main() {
    ctx := context.Background()

    // 1. Connect to message bus
    msgBus, err := bus.NewNATSBus(bus.NATSConfig{
        URL: "nats://localhost:4222",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer msgBus.Close()

    // 2. Create registry (for agent discovery)
    reg, err := registry.NewNATSRegistry(msgBus.Conn(), registry.NATSRegistryConfig{
        BucketName: "my-swarm",
        TTL:        30 * time.Second,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer reg.Close()

    // 3. Register this agent
    agentID := "worker-1"
    err = reg.Register(registry.AgentInfo{
        ID:           agentID,
        Name:         "Example Worker",
        Capabilities: []string{"process-tasks"},
        Status:       registry.StatusIdle,
    })
    if err != nil {
        log.Fatal(err)
    }

    // 4. Start heartbeat (so others know we're alive)
    sender, err := heartbeat.NewBusSender(heartbeat.SenderConfig{
        Bus:      msgBus,
        AgentID:  agentID,
        Interval: 5 * time.Second,
    })
    if err != nil {
        log.Fatal(err)
    }
    sender.Start(ctx)
    defer sender.Stop()

    // 5. Subscribe to work
    sub, err := msgBus.QueueSubscribe("tasks.process", "workers")
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Agent %s ready, waiting for tasks...", agentID)

    for msg := range sub.Messages() {
        sender.SetStatus("busy")
        log.Printf("Processing: %s", msg.Data)
        
        // Do work here...
        
        sender.SetStatus("idle")
    }
}
```

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         Your Agent                               │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐            │
│  │   LLM   │  │  Tools  │  │ Memory  │  │   MCP   │            │
│  │(Claude, │  │(builtin │  │ (FIL +  │  │(external│            │
│  │ GPT...) │  │ + reg)  │  │ search) │  │ tools)  │            │
│  └─────────┘  └─────────┘  └─────────┘  └─────────┘            │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────────────┐│
│  │              Swarm Coordination Layer                        ││
│  │  ┌────────┐ ┌─────────┐ ┌───────┐ ┌─────────┐ ┌──────────┐ ││
│  │  │Registry│ │Heartbeat│ │ State │ │  Tasks  │ │ Results  │ ││
│  │  └────────┘ └─────────┘ └───────┘ └─────────┘ └──────────┘ ││
│  └─────────────────────────────────────────────────────────────┘│
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────────────┐│
│  │                      Message Bus                             ││
│  │            (pub/sub, queue groups, request/reply)            ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

**Message Bus** — Foundation for all agent communication. Supports pub/sub (broadcast), queue groups (load balancing), and request/reply (RPC).

**Swarm Coordination** — Registry (who exists), heartbeat (who's alive), state (shared data), tasks (work items), results (outputs).

**Agent Logic** — Your code. Uses LLM for reasoning, tools for actions, memory for learning.

## Design Philosophy

- **Composition over frameworks** — Use what you need, ignore the rest
- **Backend agnostic** — Memory implementations for testing, NATS for production
- **Go idiomatic** — Channels, interfaces, context propagation
- **Explicit over magic** — No auto-discovery, no hidden state

## Used By

- [headless-agent](https://github.com/vinayprograms/agent) — Goal-oriented headless agent

## License

Apache-2.0
