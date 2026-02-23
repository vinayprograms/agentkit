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

## Used By

- [headless-agent](https://github.com/vinayprograms/agent) — Goal-oriented headless agent
- coordinator (coming soon) — User-facing agent orchestrator

## License

Apache-2.0
