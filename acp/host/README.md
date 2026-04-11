# Building an ACP Host

This package provides the host-side (editor/IDE) ACP implementation. A host launches an agent as a subprocess, drives the protocol lifecycle, and provides capabilities the agent can use (file access, terminals, permissions).

**Spec references:** [protocol overview](https://agentclientprotocol.com/protocol/overview), [initialization](https://agentclientprotocol.com/protocol/initialization), [session setup](https://agentclientprotocol.com/protocol/session-setup)

## Minimal Host

```go
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
h.Start(ctx, stdout, stdin) // handshake, then returns

sess, _ := h.NewSession(ctx, session.Params{Cwd: "/project"})
result, _ := h.Prompt(ctx, sess.ID, []content.Block{
    {Type: content.Text, Text: "Explain this codebase"},
})
fmt.Println("Done:", result.Reason)
```

## Lifecycle

```go
// 1. Connect and handshake (blocking until complete).
err := h.Start(ctx, agentStdout, agentStdin)

// 2. Create a session.
sess, err := h.NewSession(ctx, session.Params{
    Cwd: "/project",
    MCP: []session.MCPServer{{
        Name:      "filesystem",
        Transport: session.MCPTransport{Type: "stdio", Command: "mcp-fs"},
    }},
})

// 3. Send prompts (blocks until the agent completes the turn).
result, err := h.Prompt(ctx, sess.ID, []content.Block{
    {Type: content.Text, Text: "Refactor main.go to use slog"},
})

// 4. Cancel an in-progress prompt from another goroutine.
h.Cancel(ctx, sess.ID)
```

`Start` performs the initialize/authenticate handshake then returns. The connection runs in the background until the context is cancelled. All other methods may be called after `Start` returns.

## Receiving Updates

The agent streams progress via `session/update` notifications during a prompt turn. Handle them with `Config.OnUpdate`:

```go
host.Config{
    OnUpdate: func(ctx context.Context, u update.Update) {
        switch u.Type {
        case update.Message:
            ui.AppendChunk(u.SessionID, u.Role, u.Chunk)

        case update.ToolCall:
            ui.ShowToolCall(u.SessionID, u.ToolCall)

        case update.Plan:
            ui.ShowPlan(u.SessionID, u.Plan)

        case update.Config:
            ui.UpdateSetting(u.SessionID, u.Setting)

        case update.Commands:
            ui.SetCommands(u.SessionID, u.Commands)
        }
    },
}
```

`OnUpdate` is called from the RPC dispatch goroutine. It must not block — offload heavy work to a channel or goroutine.

If `OnUpdate` is nil, notifications are silently dropped.

## Providing Capabilities

Capabilities are advertised to the agent during the handshake. A non-nil handler means the capability is available.

### File system access

```go
type myFS struct {
    editors map[string]*editor.Buffer
}

func (f *myFS) ReadFile(ctx context.Context, sessionID string, p fs.ReadParams) (fs.ReadResult, error) {
    buf := f.editors[p.Path]
    if buf == nil {
        // Fall back to disk.
        data, err := os.ReadFile(p.Path)
        return fs.ReadResult{Content: string(data)}, err
    }
    // Return unsaved editor content.
    return fs.ReadResult{Content: buf.Text(p.Line, p.Limit)}, nil
}

func (f *myFS) WriteFile(ctx context.Context, sessionID string, p fs.WriteParams) (fs.WriteResult, error) {
    return fs.WriteResult{}, os.WriteFile(p.Path, []byte(p.Content), 0644)
}
```

Set `Config.FS` to advertise `ReadTextFile` and `WriteTextFile`.

### Terminal management

```go
type myTerminal struct {
    procs map[string]*os.Process
}

func (t *myTerminal) Create(ctx context.Context, sessionID string, p terminal.Create) (string, error) {
    cmd := exec.CommandContext(ctx, p.Command, p.Args...)
    cmd.Dir = p.Cwd
    // ... start process, store reference ...
    return terminalID, nil
}

func (t *myTerminal) Output(ctx context.Context, sessionID, terminalID string) (terminal.Result, error) {
    // Return buffered stdout/stderr since last read.
    return terminal.Result{Output: readBuffer(terminalID)}, nil
}

func (t *myTerminal) Wait(ctx context.Context, sessionID, terminalID string) (terminal.Result, error) {
    // Block until process exits.
    exitCode := waitProcess(terminalID)
    return terminal.Result{ExitCode: exitCode, Output: finalOutput(terminalID)}, nil
}

func (t *myTerminal) Kill(ctx context.Context, sessionID, terminalID string) error {
    return killProcess(terminalID)
}

func (t *myTerminal) Release(ctx context.Context, sessionID, terminalID string) error {
    return cleanup(terminalID)
}
```

Set `Config.Terminal` to advertise the terminal capability.

### Permission handling

```go
host.Config{
    Permission: func(ctx context.Context, perm tool.Permission) (tool.Approval, error) {
        decision := ui.ShowPermissionDialog(perm.ToolCall)
        return tool.Approval{Decision: decision}, nil
    },
}
```

Set `Config.Permission` to handle `session/request_permission` from the agent.

## Authentication

Send a token during the handshake:

```go
host.Config{
    Token: os.Getenv("AGENT_TOKEN"),
}
```

If `Token` is empty, the authenticate step is skipped.

## Slash Commands

Send a slash command instead of regular content:

```go
result, err := h.PromptCommand(ctx, sess.ID, config.Command{
    Name:  "search",
    Input: "TODO comments",
})
```

## Config Options and Modes

```go
// Change a config option.
_, err := h.SetOption(ctx, config.SetParams{
    SessionID: sess.ID,
    OptionID:  "model",
    Value:     "claude-sonnet-4-20250514",
})

// Change the session mode (deprecated — use SetOption).
_, err := h.SetMode(ctx, config.ModeParams{
    SessionID: sess.ID,
    Mode:      "plan",
})
```

## Session Restoration

If the agent supports session loading:

```go
sess, err := h.LoadSession(ctx, session.LoadParams{
    SessionID: previousSessionID,
})
// The agent replays history via OnUpdate before returning.
```

## Handler Interfaces

| Interface | Methods | Capability advertised |
|---|---|---|
| `FSHandler` | `ReadFile`, `WriteFile` | `ReadTextFile`, `WriteTextFile` |
| `TerminalHandler` | `Create`, `Output`, `Wait`, `Kill`, `Release` | `Terminal` |
| `Config.Permission` (func) | single function | (required by protocol) |

All handler methods receive a `sessionID` parameter so implementations can scope resources per session (e.g., show terminals in the correct editor tab).

## Further Reading

- [ACP Specification](https://agentclientprotocol.com) — the full protocol standard
- [Protocol Overview](https://agentclientprotocol.com/protocol/overview) — message flow and method reference
- [Initialization](https://agentclientprotocol.com/protocol/initialization) — handshake and capability negotiation
- [File System Operations](https://agentclientprotocol.com/protocol/file-system) — host-mediated file access
- [Terminal Management](https://agentclientprotocol.com/protocol/terminal) — terminal lifecycle
- [Tool Calls and Permissions](https://agentclientprotocol.com/protocol/tool-calls) — permission model
- [Client Ecosystem](https://agentclientprotocol.com/ecosystem/clients) — other ACP hosts for reference
