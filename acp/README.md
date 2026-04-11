# ACP — Agent Client Protocol

Go implementation of the [Agent Client Protocol](https://agentclientprotocol.com), a standard for communication between code editors (hosts) and AI coding agents.

ACP does for agents what LSP did for language servers — one protocol, universal interop.

**Spec reference:** https://agentclientprotocol.com

## Install

```
go get github.com/vinayprograms/agentkit/acp@latest
```

## Quick Start

### Build an agent

```go
package main

import (
    "context"
    "os"

    "github.com/vinayprograms/agentkit/acp"
    "github.com/vinayprograms/agentkit/acp/agent"
    "github.com/vinayprograms/agentkit/acp/proto/prompt"
    "github.com/vinayprograms/agentkit/acp/proto/update"
)

type myAgent struct{}

func (a *myAgent) Prompt(ctx context.Context, turn *agent.Turn) (prompt.Result, error) {
    turn.SendUpdate(ctx, update.Update{
        Type: update.Message, Role: "assistant",
        Chunk: "Hello from the agent!",
    })
    return prompt.Result{Reason: prompt.EndTurn}, nil
}

func main() {
    srv := agent.New(agent.Config{
        Info:    acp.Info{Name: "my-agent", Version: "1.0"},
        Handler: &myAgent{},
    })
    srv.Run(context.Background(), os.Stdin, os.Stdout)
}
```

### Build a host

```go
package main

import (
    "context"
    "fmt"
    "os/exec"

    "github.com/vinayprograms/agentkit/acp"
    "github.com/vinayprograms/agentkit/acp/host"
    "github.com/vinayprograms/agentkit/acp/proto/content"
    "github.com/vinayprograms/agentkit/acp/proto/session"
    "github.com/vinayprograms/agentkit/acp/proto/update"
)

func main() {
    cmd := exec.Command("my-agent")
    stdin, _ := cmd.StdinPipe()
    stdout, _ := cmd.StdoutPipe()
    cmd.Start()

    h := host.New(host.Config{
        Info: acp.Info{Name: "my-editor", Version: "2.0"},
        OnUpdate: func(ctx context.Context, u update.Update) {
            if u.Type == update.Message {
                fmt.Print(u.Chunk)
            }
        },
    })

    ctx := context.Background()
    h.Start(ctx, stdout, stdin)

    sess, _ := h.NewSession(ctx, session.Params{Cwd: "/project"})
    h.Prompt(ctx, sess.ID, []content.Block{
        {Type: content.Text, Text: "Explain main.go"},
    })
}
```

## Architecture

```
Host (editor/IDE)                    Agent (AI)
      │                                │
      │──── initialize ───────────────▶│
      │◀─── result ──────────────────-─│
      │──── authenticate ─────────────▶│  (optional)
      │◀─── result ─────────────────-──│
      │──── session/new ──────────────▶│
      │◀─── result ───────-────────────│
      │──── session/prompt ───────────▶│
      │                                │
      │◀─── session/update -───────────│  (streaming: chunks, tool calls, plan)
      │◀─── request_permission ──-─────│  (agent asks host)
      │──── result ───────────────────▶│
      │◀─── fs/read_text_file ────-────│  (agent asks host)
      │──── result ───────────────────▶│
      │◀─── terminal/create ───────-───│  (agent asks host)
      │──── result ───────────────────▶│
      │                                │
      │◀─── result ─────────────────-──│  (prompt complete)
```

The protocol is **bidirectional**: both sides send requests and handle incoming requests concurrently over a single JSON-RPC 2.0 connection.

## Package Structure

```
acp/                    Entry point: Info, Meta
├── agent/              Build an agent (see agent/README.md)
├── host/               Build a host (see host/README.md)
├── proto/              Protocol types
│   ├── content/        Content blocks (text, image, audio, resources)
│   ├── tool/           Tool calls, permissions, lifecycle
│   ├── prompt/         Prompt turns and stop reasons
│   ├── plan/           Execution plan steps
│   ├── config/         Runtime settings and slash commands
│   ├── update/         Session update notifications
│   ├── terminal/       Terminal lifecycle management
│   ├── fs/             File system operations
│   └── session/        Session lifecycle
└── internal/rpc/       JSON-RPC transport (not importable)
```

## Protocol Types Reference

For detailed usage examples of every type, see [proto/README.md](proto/README.md).

### Content (`proto/content`) — [spec](https://agentclientprotocol.com/protocol/content)

| Type | Description |
|---|---|
| `Block` | Displayable unit — text, image, audio, or resource |
| `Embedded` | Inline file/resource content (for @-mentions) |

Constants: `Text`, `Image`, `Audio`, `Resource`, `Link`

### Tool Calls (`proto/tool`) — [spec](https://agentclientprotocol.com/protocol/tool-calls)

| Type | Description |
|---|---|
| `Call` | Tool invocation with lifecycle (pending → running → done/failed) |
| `Location` | File path + line being accessed |
| `Diff` | Structured text change (oldText/newText) |
| `Permission` | Agent's request for tool execution approval |
| `Approval` | Host's decision (allow/reject, once/always) |

Kinds: `Read`, `Edit`, `Delete`, `Move`, `Search`, `Execute`, `Think`, `Fetch`, `Other`

Statuses: `Pending`, `Running`, `Done`, `Failed`

Decisions: `AllowOnce`, `AllowAlways`, `RejectOnce`, `RejectAlways`

### Prompt (`proto/prompt`) — [spec](https://agentclientprotocol.com/protocol/prompt-turn)

| Type | Description |
|---|---|
| `Params` | Prompt turn request (content + optional command) |
| `Result` | Turn completion with stop reason |

Reasons: `EndTurn`, `MaxTokens`, `MaxTurns`, `Refusal`, `Cancelled`

### Plan (`proto/plan`) — [spec](https://agentclientprotocol.com/protocol/agent-plan)

| Type | Description |
|---|---|
| `Step` | One entry in the execution plan |

Statuses: `Pending`, `Running`, `Done`

Priorities: `High`, `Medium`, `Low`

### Config (`proto/config`) — [spec: options](https://agentclientprotocol.com/protocol/session-config-options), [spec: commands](https://agentclientprotocol.com/protocol/slash-commands)

| Type | Description |
|---|---|
| `Option` | Runtime-adjustable setting |
| `Choice` | One selectable value for an option |
| `Command` | Slash command definition |

Categories: `Mode`, `Model`, `Thought`

### Update (`proto/update`) — [spec](https://agentclientprotocol.com/protocol/prompt-turn)

| Type | Description |
|---|---|
| `Update` | Session notification payload (discriminated union) |

Types: `Message`, `ToolCall`, `Plan`, `Config`, `Commands`

### Terminal (`proto/terminal`) — [spec](https://agentclientprotocol.com/protocol/terminal)

| Type | Description |
|---|---|
| `Create` | Launch params (command, args, cwd, env) |
| `Created` | Result of create (terminal ID) |
| `Ref` | Terminal reference for subsequent operations |
| `Result` | Output/exit result |

### File System (`proto/fs`) — [spec](https://agentclientprotocol.com/protocol/file-system)

| Type | Description |
|---|---|
| `ReadParams` / `ReadResult` | Read file content (supports partial reads) |
| `WriteParams` / `WriteResult` | Write file content (creates if absent) |

### Session (`proto/session`) — [spec](https://agentclientprotocol.com/protocol/session-setup)

| Type | Description |
|---|---|
| `Session` | Active session (ID + metadata) |
| `Params` / `Result` | Create a new session |
| `LoadParams` / `LoadResult` | Restore a previous session |
| `Cancel` | Cancel an in-progress prompt turn |
| `MCPServer` / `MCPTransport` | MCP server configuration |

## Extensibility — [spec](https://agentclientprotocol.com/protocol/extensibility)

Every protocol type supports a `Meta` field (serialized as `_meta`) for custom data. Reserved keys for W3C distributed tracing: `traceparent`, `tracestate`, `baggage`.

## Further Reading

- [ACP Specification](https://agentclientprotocol.com) — the full protocol standard
- [Protocol Overview](https://agentclientprotocol.com/protocol/overview) — message flow and method reference
- [Initialization](https://agentclientprotocol.com/protocol/initialization) — handshake and capability negotiation
- [Agent Registry](https://agentclientprotocol.com/registry) — catalog of ACP-compatible agents and clients
