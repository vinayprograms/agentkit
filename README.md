# Agentkit

A Go toolkit for building AI agents. Single-agent building blocks — LLM providers, tool execution, memory, content/shell guards, and protocol support. No opinions about agent architecture; use what you need.

## Install

```bash
go get github.com/vinayprograms/agentkit
```

## Quick Start

The simplest agent: create an LLM, define tools, and run an agentic loop.

```go
package main

import (
    "context"
    "fmt"
    "os"
    "time"

    "github.com/vinayprograms/agentkit/llm"
)

func main() {
    model, _ := llm.New(llm.Config{
        Service:   "anthropic",
        Model:     "claude-sonnet-4-20250514",
        APIKey:    os.Getenv("ANTHROPIC_API_KEY"),
        MaxTokens: 4096,
    })

    tools := []llm.ToolDef{{
        Name:        "current_time",
        Description: "Returns the current UTC time.",
        Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
    }}

    messages := []llm.Message{{Role: "user", Content: "What time is it?"}}

    for {
        resp, _ := model.Chat(context.Background(), llm.ChatRequest{
            Messages: messages, Tools: tools, MaxTokens: 4096,
        })

        if len(resp.ToolCalls) == 0 {
            fmt.Println(resp.Content)
            break
        }

        messages = append(messages, llm.Message{
            Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls,
        })
        for _, tc := range resp.ToolCalls {
            result := time.Now().UTC().Format(time.RFC3339)
            messages = append(messages, llm.Message{
                Role: "tool", Content: result, ToolCallID: tc.ID,
            })
        }
    }
}
```

See [examples/simple-llm-agent](examples/simple-llm-agent/) for the runnable version.

## Packages

### Core

| Package | What it does | Doc |
|---|---|---|
| **llm** | LLM provider abstraction (Anthropic, OpenAI, Google, Groq, Mistral, Ollama, etc.) | [llm-design.md](docs/llm-design.md) |
| **tools** | Tool definition, validation, and execution with optional guards | — |
| **memory** | Semantic memory with BM25 search (in-memory + Bleve backends) | [memory-design.md](docs/memory-design.md) |
| **embedding** | Text-to-vector embeddings for semantic search | — |
| **errors** | Structured error taxonomy with categories and retry semantics | [errors-design.md](docs/errors-design.md) |

### Security

| Package | What it does |
|---|---|
| **shellguard** | Shell command gating — deterministic deny list + optional LLM analysis |
| **contentguard** | Content verification pipeline with stage-based workflows |

### Runtime

| Package | What it does | Doc |
|---|---|---|
| **credentials** | Credential lookup (file, env, merged) |
| **policy** | TOML-based policy configuration |
| **mcp** | Model Context Protocol client (stdio, HTTP) | [mcp-design.md](docs/mcp-design.md) |
| **shutdown** | Graceful shutdown with phased execution | [shutdown-design.md](docs/shutdown-design.md) |

### Protocol

| Package | What it does | Doc |
|---|---|---|
| **acp/** | Agent Client Protocol — editor/IDE integration (separate module) | [acp/README.md](acp/README.md) |

`acp` is a separate Go module so consumers can import ACP without pulling in the rest of agentkit. See [acp/README.md](acp/README.md).

## Observability

Every package that performs I/O emits OpenTelemetry spans. Consumers initialize OTel in their `main` and agentkit emits to whatever exporter is configured:

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

exp, _ := otlptracegrpc.New(ctx)
tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp))
otel.SetTracerProvider(tp)
defer tp.Shutdown(ctx)
```

Without initialization, OTel defaults to a no-op — spans silently vanish and agentkit has zero observability overhead.

## Examples

| Example | What it shows |
|---|---|
| [simple-llm-agent](examples/simple-llm-agent/) | LLM provider + custom tools + agentic loop |
| [memory-agent](examples/memory-agent/) | Persistent BM25 memory with remember/recall |
| [structured-errors](examples/structured-errors/) | Error handling patterns |
| [graceful-shutdown](examples/graceful-shutdown/) | Multi-phase shutdown |

## Design Philosophy

- **Composition over frameworks** — use what you need, ignore the rest
- **Consumer defines interfaces** — kit packages expose concrete behavior; interfaces live where they're used
- **Ready-to-use construction** — no `Init()` calls, no `Set*` methods; one constructor returns a working object
- **Backend-agnostic** — in-memory implementations for tests, real backends for production
- **Go idiomatic** — channels, interfaces, context propagation, no magic

## License

Apache-2.0
