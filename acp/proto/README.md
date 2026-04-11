# ACP Protocol Types Guide

This guide shows how to use each protocol type package with practical examples. Types are organized by use case, not by package.

For the full ACP specification, see https://agentclientprotocol.com.

---

## Content Blocks — [spec](https://agentclientprotocol.com/protocol/content)

Content blocks represent displayable information in prompts, tool outputs, and responses. Import `acp/proto/content`.

### Sending text

```go
content.Block{Type: content.Text, Text: "Explain this function"}
```

### Sending images

The host can include images in prompts if the agent declares `Image: true` in its capabilities:

```go
content.Block{
    Type:     content.Image,
    Data:     base64.StdEncoding.EncodeToString(pngBytes),
    MimeType: "image/png",
}
```

### Sending audio

Requires `Audio: true` in agent capabilities:

```go
content.Block{
    Type:     content.Audio,
    Data:     base64.StdEncoding.EncodeToString(wavBytes),
    MimeType: "audio/wav",
}
```

### Embedding file content (@-mentions)

When the host includes a file the agent can't access directly, it embeds the full content:

```go
content.Block{
    Type: content.Resource,
    Embedded: &content.Embedded{
        URI:      "file:///project/main.go",
        MimeType: "text/x-go",
        Text:     fileContent,
    },
}
```

### Resource links

References to resources the agent can fetch itself:

```go
content.Block{
    Type:        content.Link,
    URI:         "https://docs.example.com/api",
    Name:        "API Documentation",
    Description: "REST API reference for the project",
}
```

### Combining content in a prompt

A single prompt can mix content types:

```go
h.Prompt(ctx, sess.ID, []content.Block{
    {Type: content.Text, Text: "What does this screenshot show?"},
    {Type: content.Image, Data: screenshotBase64, MimeType: "image/png"},
    {Type: content.Resource, Embedded: &content.Embedded{
        URI: "file:///project/config.yaml", Text: configYAML,
    }},
})
```

---

## Tool Call Lifecycle — [spec](https://agentclientprotocol.com/protocol/tool-calls)

Tool calls track what the agent is doing. Import `acp/proto/tool`.

### Reporting a tool call

Inside an agent's `Prompt` handler, report tool progress via updates:

```go
call := tool.Call{
    ID:     "read-1",
    Title:  "Reading main.go",
    Kind:   tool.Read,
    Status: tool.Pending,
    Location: &tool.Location{
        Path: "/project/main.go",
        Line: 42,
    },
}

// Report start.
turn.SendUpdate(ctx, update.Update{
    Type:     update.ToolCall,
    ToolCall: &call,
})

// Do the work...
src, _ := turn.ReadFile(ctx, fs.ReadParams{Path: "/project/main.go"})

// Report completion with output.
call.Status = tool.Done
call.Output = []content.Block{{Type: content.Text, Text: src.Content}}
turn.SendUpdate(ctx, update.Update{
    Type:     update.ToolCall,
    ToolCall: &call,
})
```

### Tool kinds

Categorize tool calls so the host can render appropriate UX:

| Kind | Use for |
|---|---|
| `tool.Read` | Reading files, fetching data |
| `tool.Edit` | Modifying files |
| `tool.Delete` | Removing files or resources |
| `tool.Move` | Renaming or moving files |
| `tool.Search` | Searching codebases, grep |
| `tool.Execute` | Running commands, scripts |
| `tool.Think` | Internal reasoning steps |
| `tool.Fetch` | HTTP requests, API calls |
| `tool.Other` | Anything else |

### Reporting diffs

When a tool modifies a file, include the structured diff:

```go
call.Diff = &tool.Diff{
    OldText: "func main() {\n\tlog.Println(\"hello\")\n}",
    NewText: "func main() {\n\tslog.Info(\"hello\")\n}",
}
```

---

## Permissions — [spec](https://agentclientprotocol.com/protocol/tool-calls)

Before executing destructive operations, ask the user. Import `acp/proto/tool`.

### Requesting permission (agent side)

```go
approval, err := turn.RequestPermission(ctx, tool.Permission{
    ToolCall: tool.Call{
        ID:    "exec-1",
        Kind:  tool.Execute,
        Title: "Run deployment script",
        Input: "deploy.sh --prod",
    },
})

switch approval.Decision {
case tool.AllowOnce:
    // Proceed this time only.
case tool.AllowAlways:
    // Proceed and remember — don't ask again for this tool.
case tool.RejectOnce:
    // Skip this time.
case tool.RejectAlways:
    // Never run this tool in this session.
}
```

### Handling permission requests (host side)

```go
host.Config{
    Permission: func(ctx context.Context, p tool.Permission) (tool.Approval, error) {
        // Show the user what the agent wants to do.
        fmt.Printf("Agent wants to: %s\n", p.ToolCall.Title)
        fmt.Printf("  Kind: %s\n", p.ToolCall.Kind)
        fmt.Printf("  Input: %s\n", p.ToolCall.Input)

        decision := promptUser("Allow? [y/n/Y/N]: ")
        return tool.Approval{Decision: decision}, nil
    },
}
```

---

## Streaming Updates — [spec](https://agentclientprotocol.com/protocol/prompt-turn)

Agents push progress to the host during a prompt turn. Import `acp/proto/update`.

### Message chunks

Stream text back to the host incrementally:

```go
turn.SendUpdate(ctx, update.Update{
    Type:  update.Message,
    Role:  "assistant",
    Chunk: "Here's what I found:\n",
})

// Send more chunks as you generate them.
for _, line := range lines {
    turn.SendUpdate(ctx, update.Update{
        Type: update.Message, Role: "assistant",
        Chunk: line + "\n",
    })
}
```

### Receiving updates (host side)

```go
host.Config{
    OnUpdate: func(ctx context.Context, u update.Update) {
        switch u.Type {
        case update.Message:
            fmt.Print(u.Chunk) // stream to terminal

        case update.ToolCall:
            fmt.Printf("[%s] %s: %s\n",
                u.ToolCall.Kind, u.ToolCall.Title, u.ToolCall.Status)

        case update.Plan:
            for _, step := range u.Plan {
                fmt.Printf("  [%s] %s\n", step.Status, step.Content)
            }

        case update.Config:
            fmt.Printf("Setting changed: %s = %s\n",
                u.Setting.Name, u.Setting.Value)

        case update.Commands:
            for _, cmd := range u.Commands {
                fmt.Printf("  /%s — %s\n", cmd.Name, cmd.Description)
            }
        }
    },
}
```

---

## Execution Plans — [spec](https://agentclientprotocol.com/protocol/agent-plan)

Agents communicate multi-step strategies to the host. Import `acp/proto/plan`.

### Sending a plan

Each update sends the **complete** plan (full replacement, not incremental):

```go
turn.SendUpdate(ctx, update.Update{
    Type: update.Plan,
    Plan: []plan.Step{
        {Content: "Analyze existing code", Status: plan.Done, Priority: plan.High},
        {Content: "Write unit tests", Status: plan.Running, Priority: plan.High},
        {Content: "Refactor to use slog", Status: plan.Pending, Priority: plan.Medium},
        {Content: "Update documentation", Status: plan.Pending, Priority: plan.Low},
    },
})
```

### Updating the plan as work progresses

```go
// After finishing tests, update the plan.
turn.SendUpdate(ctx, update.Update{
    Type: update.Plan,
    Plan: []plan.Step{
        {Content: "Analyze existing code", Status: plan.Done, Priority: plan.High},
        {Content: "Write unit tests", Status: plan.Done, Priority: plan.High},
        {Content: "Refactor to use slog", Status: plan.Running, Priority: plan.Medium},
        {Content: "Update documentation", Status: plan.Pending, Priority: plan.Low},
    },
})
```

---

## File System — [spec](https://agentclientprotocol.com/protocol/file-system)

Agents read and write files through the host, not the filesystem. This gives access to unsaved editor buffers. Import `acp/proto/fs`.

### Reading a file (agent side)

```go
// Read the entire file.
result, err := turn.ReadFile(ctx, fs.ReadParams{Path: "/project/main.go"})
fmt.Println(result.Content)

// Read a specific range (1-based line numbers).
result, err := turn.ReadFile(ctx, fs.ReadParams{
    Path:  "/project/main.go",
    Line:  10,  // start at line 10
    Limit: 20,  // read 20 lines
})
```

### Writing a file (agent side)

```go
_, err := turn.WriteFile(ctx, fs.WriteParams{
    Path:    "/project/main.go",
    Content: newContent,
})
// Creates the file if it doesn't exist.
```

### Implementing file access (host side)

```go
type myFS struct {
    buffers map[string]string // unsaved editor content
}

func (f *myFS) ReadFile(ctx context.Context, sessionID string, p fs.ReadParams) (fs.ReadResult, error) {
    // Prefer unsaved editor content over disk.
    if content, ok := f.buffers[p.Path]; ok {
        return fs.ReadResult{Content: applyRange(content, p.Line, p.Limit)}, nil
    }
    data, err := os.ReadFile(p.Path)
    return fs.ReadResult{Content: string(data)}, err
}

func (f *myFS) WriteFile(ctx context.Context, sessionID string, p fs.WriteParams) (fs.WriteResult, error) {
    return fs.WriteResult{}, os.WriteFile(p.Path, []byte(p.Content), 0644)
}
```

---

## Terminal Management — [spec](https://agentclientprotocol.com/protocol/terminal)

Agents run commands through the host's terminal. Import `acp/proto/terminal`.

### Running a command (agent side)

```go
// Launch the process.
termID, err := turn.CreateTerminal(ctx, terminal.Create{
    Command: "go",
    Args:    []string{"test", "-v", "./..."},
    Cwd:     "/project",
    Env:     map[string]string{"GOFLAGS": "-count=1"},
})

// Wait for completion.
result, err := turn.TerminalWait(ctx, termID)
fmt.Printf("Exit: %d\nOutput:\n%s\n", result.ExitCode, result.Output)

// Always release when done.
turn.TerminalRelease(ctx, termID)
```

### Streaming output from a long-running process

```go
termID, _ := turn.CreateTerminal(ctx, terminal.Create{
    Command: "npm", Args: []string{"run", "dev"},
})

// Poll for output.
for {
    result, _ := turn.TerminalOutput(ctx, termID)
    if result.Output != "" {
        turn.SendUpdate(ctx, update.Update{
            Type: update.Message, Chunk: result.Output,
        })
    }
    time.Sleep(500 * time.Millisecond)
    // Break when done...
}

turn.TerminalKill(ctx, termID)
turn.TerminalRelease(ctx, termID)
```

### Implementing terminal support (host side)

```go
type myTerminal struct {
    procs map[string]*exec.Cmd
    bufs  map[string]*bytes.Buffer
}

func (t *myTerminal) Create(ctx context.Context, sessionID string, p terminal.Create) (string, error) {
    id := uuid.New().String()
    cmd := exec.CommandContext(ctx, p.Command, p.Args...)
    cmd.Dir = p.Cwd
    for k, v := range p.Env {
        cmd.Env = append(cmd.Env, k+"="+v)
    }
    buf := &bytes.Buffer{}
    cmd.Stdout = buf
    cmd.Stderr = buf
    if err := cmd.Start(); err != nil {
        return "", err
    }
    t.procs[id] = cmd
    t.bufs[id] = buf
    return id, nil
}

func (t *myTerminal) Wait(ctx context.Context, sessionID, terminalID string) (terminal.Result, error) {
    cmd := t.procs[terminalID]
    err := cmd.Wait()
    code := cmd.ProcessState.ExitCode()
    return terminal.Result{ExitCode: code, Output: t.bufs[terminalID].String()}, err
}

// Output, Kill, Release follow the same pattern.
```

---

## Sessions — [spec](https://agentclientprotocol.com/protocol/session-setup)

Sessions are isolated conversations with independent context. Import `acp/proto/session`.

### Creating a session (host side)

```go
sess, err := h.NewSession(ctx, session.Params{
    Cwd: "/project",
    Metadata: map[string]string{
        "editor": "vscode",
        "workspace": "my-project",
    },
})
fmt.Println("Session:", sess.ID)
```

### Passing MCP servers at session creation

Tell the agent which MCP servers to connect to:

```go
sess, err := h.NewSession(ctx, session.Params{
    Cwd: "/project",
    MCP: []session.MCPServer{
        {
            Name: "filesystem",
            Transport: session.MCPTransport{
                Type:    "stdio",
                Command: "mcp-filesystem",
                Args:    []string{"/project"},
            },
        },
        {
            Name: "github",
            Transport: session.MCPTransport{
                Type:    "http",
                URL:     "https://mcp.github.com",
                Headers: map[string]string{"Authorization": "Bearer " + token},
            },
        },
    },
})
```

### Handling session creation (agent side)

```go
agent.Config{
    NewSession: func(ctx context.Context, p session.Params) (session.Result, error) {
        id := uuid.New().String()

        // Connect to MCP servers the host provided.
        for _, mcp := range p.MCP {
            connectMCP(mcp)
        }

        return session.Result{
            Session: session.Session{ID: id, Metadata: p.Metadata},
        }, nil
    },
}
```

### Restoring a previous session

```go
// Host side.
sess, err := h.LoadSession(ctx, session.LoadParams{SessionID: previousID})
// The agent replays history via OnUpdate before returning.

// Agent side.
agent.Config{
    LoadSession: func(ctx context.Context, p session.LoadParams) (session.LoadResult, error) {
        history, err := store.Load(p.SessionID)
        if err != nil {
            return session.LoadResult{}, err
        }
        return session.LoadResult{Session: history.Session}, nil
    },
}
```

---

## Config Options — [spec](https://agentclientprotocol.com/protocol/session-config-options)

Runtime-adjustable settings exposed by the agent. Import `acp/proto/config`.

### Advertising available options (agent side)

Send available options as a session update after session creation:

```go
turn.SendUpdate(ctx, update.Update{
    Type: update.Config,
    Setting: &config.Option{
        ID:       "model",
        Name:     "AI Model",
        Category: config.Model,
        Type:     "select",
        Value:    "claude-sonnet-4-20250514",
        Choices: []config.Choice{
            {Value: "claude-sonnet-4-20250514", Label: "Claude Sonnet"},
            {Value: "claude-opus-4-20250514", Label: "Claude Opus"},
        },
    },
})
```

### Changing an option (host side)

```go
_, err := h.SetOption(ctx, config.SetParams{
    SessionID: sess.ID,
    OptionID:  "model",
    Value:     "claude-opus-4-20250514",
})
```

### Handling option changes (agent side)

```go
agent.Config{
    SetOption: func(ctx context.Context, p config.SetParams) (config.SetResult, error) {
        switch p.OptionID {
        case "model":
            agent.SwitchModel(p.Value)
        default:
            return config.SetResult{}, fmt.Errorf("unknown option: %s", p.OptionID)
        }
        return config.SetResult{}, nil
    },
}
```

---

## Slash Commands — [spec](https://agentclientprotocol.com/protocol/slash-commands)

Agents advertise commands for quick user access. Import `acp/proto/config`.

### Advertising commands (agent side)

```go
turn.SendUpdate(ctx, update.Update{
    Type: update.Commands,
    Commands: []config.Command{
        {Name: "search", Description: "Search the codebase", InputHint: "query"},
        {Name: "test", Description: "Run test suite"},
        {Name: "deploy", Description: "Deploy to staging", InputHint: "environment"},
    },
})
```

### Sending a slash command (host side)

```go
result, err := h.PromptCommand(ctx, sess.ID, config.Command{
    Name:  "search",
    Input: "TODO comments",
})
```

---

## Prompt Turn — [spec](https://agentclientprotocol.com/protocol/prompt-turn)

The prompt turn is the core interaction cycle. Import `acp/proto/prompt`.

### Stop reasons

Every prompt turn ends with a reason:

| Reason | Meaning | Host action |
|---|---|---|
| `prompt.EndTurn` | Agent finished naturally | Display final result |
| `prompt.MaxTokens` | Output was truncated | Warn user, offer to continue |
| `prompt.MaxTurns` | Too many LLM roundtrips | Warn user about complexity |
| `prompt.Refusal` | Agent declined the request | Display refusal message |
| `prompt.Cancelled` | User cancelled via `Cancel` | Clean up UI state |

### Handling stop reasons (host side)

```go
result, err := h.Prompt(ctx, sess.ID, blocks)
if err != nil {
    log.Fatal(err)
}

switch result.Reason {
case prompt.EndTurn:
    // Normal completion.
case prompt.MaxTokens:
    fmt.Println("(response was truncated)")
case prompt.Cancelled:
    fmt.Println("(cancelled by user)")
case prompt.Refusal:
    fmt.Println("(agent declined)")
}
```
