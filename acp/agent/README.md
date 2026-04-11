# Building an ACP Agent

This package provides the agent-side ACP implementation. An agent is an AI coding assistant that communicates with a host (editor/IDE) over stdin/stdout using JSON-RPC.

**Spec references:** [prompt turn lifecycle](https://agentclientprotocol.com/protocol/prompt-turn), [initialization](https://agentclientprotocol.com/protocol/initialization), [session setup](https://agentclientprotocol.com/protocol/session-setup)

## Minimal Agent

```go
srv := agent.New(agent.Config{
    Info: acp.Info{Name: "my-agent", Version: "1.0"},
    Prompt: func(ctx context.Context, turn *agent.Turn) (prompt.Result, error) {
        // Process the prompt and respond.
        return prompt.Result{Reason: prompt.EndTurn}, nil
    },
})
srv.Run(ctx, os.Stdin, os.Stdout)
```

`Config.Prompt` is the only required field. Everything else has sensible defaults.

## The Turn Object

When a prompt arrives, your handler receives a `*Turn` — a session-scoped handle with methods to call back to the host:

```go
Prompt: func(ctx context.Context, turn *agent.Turn) (prompt.Result, error) {
    // The incoming prompt content.
    for _, block := range turn.Params.Content {
        fmt.Println(block.Text)
    }

    // The active session.
    fmt.Println("Session:", turn.Session.ID)

    // Stream a response back to the host.
    turn.SendUpdate(ctx, update.Update{
        Type: update.Message, Role: "assistant",
        Chunk: "Working on it...",
    })

    return prompt.Result{Reason: prompt.EndTurn}, nil
}
```

### Reading and writing files — [spec](https://agentclientprotocol.com/protocol/file-system)

The agent reads files through the host, not the filesystem directly. This gives access to unsaved editor buffers:

```go
result, err := turn.ReadFile(ctx, fs.ReadParams{
    Path:  "/project/main.go",
    Line:  10,    // 1-based, optional
    Limit: 20,    // max lines, optional
})
fmt.Println(result.Content)

_, err = turn.WriteFile(ctx, fs.WriteParams{
    Path:    "/project/main.go",
    Content: newContent,
})
```

### Running terminal commands — [spec](https://agentclientprotocol.com/protocol/terminal)

```go
// Launch a process.
termID, err := turn.CreateTerminal(ctx, terminal.Create{
    Command: "go",
    Args:    []string{"test", "./..."},
    Cwd:     "/project",
})

// Wait for it to finish.
result, err := turn.TerminalWait(ctx, termID)
fmt.Printf("Exit code: %d\nOutput: %s\n", result.ExitCode, result.Output)

// Always release when done.
turn.TerminalRelease(ctx, termID)
```

For long-running processes, read output incrementally:

```go
termID, _ := turn.CreateTerminal(ctx, terminal.Create{Command: "tail", Args: []string{"-f", "log"}})
for {
    result, _ := turn.TerminalOutput(ctx, termID)
    if result.Output != "" {
        process(result.Output)
    }
    // ... break condition ...
}
turn.TerminalKill(ctx, termID)
turn.TerminalRelease(ctx, termID)
```

### Requesting permission — [spec](https://agentclientprotocol.com/protocol/tool-calls)

Before executing destructive operations, ask the user:

```go
approval, err := turn.RequestPermission(ctx, tool.Permission{
    ToolCall: tool.Call{
        ID:    "1",
        Kind:  tool.Execute,
        Title: "Run deployment script",
    },
})

switch approval.Decision {
case tool.AllowOnce:
    // Proceed this time.
case tool.AllowAlways:
    // Proceed and remember for future calls.
case tool.RejectOnce, tool.RejectAlways:
    // User declined.
}
```

### Streaming updates

Send progress to the host during a prompt turn:

```go
// Text chunks (streamed to the user).
turn.SendUpdate(ctx, update.Update{
    Type: update.Message, Role: "assistant",
    Chunk: "Here's what I found...",
})

// Tool call progress.
turn.SendUpdate(ctx, update.Update{
    Type: update.ToolCall,
    ToolCall: &tool.Call{
        ID: "1", Kind: tool.Read, Status: tool.Running,
        Title: "Reading main.go",
        Location: &tool.Location{Path: "/project/main.go", Line: 42},
    },
})

// Execution plan.
turn.SendUpdate(ctx, update.Update{
    Type: update.Plan,
    Plan: []plan.Step{
        {Content: "Analyze code", Status: plan.Done, Priority: plan.High},
        {Content: "Write tests", Status: plan.Running, Priority: plan.High},
        {Content: "Refactor", Status: plan.Pending, Priority: plan.Medium},
    },
})
```

## Optional Handlers

### Authentication

Validate the host's token before accepting requests:

```go
agent.Config{
    Auth: func(ctx context.Context, token string) error {
        if token != os.Getenv("AGENT_SECRET") {
            return errors.New("unauthorized")
        }
        return nil
    },
}
```

If `Auth` is nil, all tokens are accepted.

### Session management

```go
agent.Config{
    NewSession: func(ctx context.Context, p session.Params) (session.Result, error) {
        id := uuid.New().String()
        store.Create(id, p.Cwd, p.Metadata)
        return session.Result{
            Session: session.Session{ID: id, Metadata: p.Metadata},
        }, nil
    },

    LoadSession: func(ctx context.Context, p session.LoadParams) (session.LoadResult, error) {
        s, err := store.Load(p.SessionID)
        if err != nil {
            return session.LoadResult{}, err
        }
        return session.LoadResult{Session: s}, nil
    },

    Cancel: func(ctx context.Context, sessionID string) {
        store.Cancel(sessionID)
    },
}
```

If `NewSession` is nil, sessions are auto-created with generated IDs. If `LoadSession` is nil, load requests are rejected.

### Config options and modes

```go
agent.Config{
    SetMode: func(ctx context.Context, p config.ModeParams) (config.ModeResult, error) {
        applyMode(p.SessionID, p.Mode)
        return config.ModeResult{}, nil
    },

    SetOption: func(ctx context.Context, p config.SetParams) (config.SetResult, error) {
        applyOption(p.SessionID, p.OptionID, p.Value)
        return config.SetResult{}, nil
    },
}
```

## Capabilities

Declare what content types your agent can process:

```go
agent.Config{
    Capabilities: agent.Capabilities{
        Image: true,  // Can process image content in prompts
        Audio: true,  // Can process audio content in prompts
    },
}
```

These are advertised to the host during the initialize handshake.

## Lifecycle

```
Host calls initialize  →  Agent responds with capabilities
Host calls authenticate →  Agent validates token (if Auth is set)
Host calls session/new  →  Agent creates session
Host calls session/prompt → Agent's Prompt handler runs
                            ├── Agent streams updates back
                            ├── Agent calls host for files/terminals/permissions
                            └── Agent returns stop reason
Host calls session/cancel → Agent's Cancel handler runs (notification)
```

`Agent.Run` blocks until the context is cancelled or the connection closes.

## Further Reading

- [ACP Specification](https://agentclientprotocol.com) — the full protocol standard
- [Prompt Turn Lifecycle](https://agentclientprotocol.com/protocol/prompt-turn) — the core agentic loop
- [Tool Calls and Permissions](https://agentclientprotocol.com/protocol/tool-calls) — permission model
- [Agent Plan](https://agentclientprotocol.com/protocol/agent-plan) — execution plan notifications
- [Session Config Options](https://agentclientprotocol.com/protocol/session-config-options) — runtime settings
- [Slash Commands](https://agentclientprotocol.com/protocol/slash-commands) — command advertisement
