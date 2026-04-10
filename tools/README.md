# tools

Tool registry for LLM agents. Provides the `Tool` interface, typed parameter schemas, argument validation, guard integration, and 30+ built-in tools.

## Usage

```go
// Create registry and register tools
registry := tools.NewRegistry()
registry.Register(tools.New(tools.Pwd()))
registry.Register(tools.New(tools.Read(workspace)))
registry.Register(tools.New(tools.Bash(workspace)).With(gate))
registry.Register(tools.New(tools.Fetch(summarizer)))

// Get definitions for LLM
defs := registry.Definitions()

// Execute a tool call from LLM
result, err := registry.Execute(ctx, "bash", map[string]any{"command": "ls -la"})
```

## Tool Interface

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]Param
    Execute(ctx context.Context, args Args) (string, error)
}
```

Tools return `(string, error)`. The string is the result for the LLM. Errors are infrastructure failures (timeout, permission denied). Tool-level results (non-zero exit, HTTP 404 with body) are returned as strings, not errors.

## Writing Custom Tools

```go
type myTool struct{}

func MyTool() tools.Tool { return &myTool{} }

func (t *myTool) Name() string        { return "my_tool" }
func (t *myTool) Description() string { return "Does something useful." }

func (t *myTool) Parameters() map[string]tools.Param {
    return map[string]tools.Param{
        "input": {Type: tools.StringParam, Description: "Input text", Required: true},
        "count": {Type: tools.IntParam, Description: "Number of items"},
    }
}

func (t *myTool) Execute(ctx context.Context, args tools.Args) (string, error) {
    input, _ := args.String("input")
    count := args.IntOr("count", 10)
    // ... do work ...
    return result, nil
}
```

Register it:

```go
registry.Register(tools.New(MyTool()))
```

## Guards

Guards check tool calls before execution. Implement `tools.Guard`:

```go
type Guard interface {
    Check(ctx context.Context, args Args) error
}
```

Attach guards at registration:

```go
registry.Register(tools.New(tools.Bash(workspace)).With(gate))
registry.Register(tools.New(tools.Bash(workspace)).With(gate1).With(gate2))
```

Guards run in order. If any returns an error, execution stops.

## Parameters

```go
type ParamType string

const (
    StringParam  ParamType = "string"
    IntParam     ParamType = "integer"
    BoolParam    ParamType = "boolean"
    ArrayParam   ParamType = "array"
)

type Param struct {
    Type        ParamType
    Description string
    Enum        []string  // optional, constrains values
    Required    bool
}
```

The registry validates raw args against parameters before calling `Execute`. Tools receive pre-validated `Args` with typed accessors.

## Built-in Tools

| Tool | Constructor | Description |
|---|---|---|
| bash | `Bash(workspace)` | Execute shell commands |
| read | `Read(workspace)` | Read file contents |
| write | `Write(workspace)` | Write content to file |
| edit | `Edit(workspace)` | Find-and-replace in file |
| glob | `Glob(workspace)` | Pattern-based file search |
| grep | `Grep(workspace)` | Search file contents |
| ls | `Ls(workspace)` | List directory |
| mkdir | `Mkdir(workspace)` | Create directory |
| mv | `Mv(workspace)` | Move/rename file |
| cp | `Cp(workspace)` | Copy file or directory |
| rm | `Rm(workspace)` | Delete file or directory |
| head | `Head(workspace)` | Read first N lines |
| tail | `Tail(workspace)` | Read last N lines |
| diff | `Diff()` | Compare two files |
| tree | `Tree(workspace)` | Directory tree view |
| patch | `Patch()` | Apply unified diff |
| git | `Git(workspace)` | Run git commands (safe subset) |
| pwd | `Pwd()` | Current directory |
| hostname | `Hostname()` | System hostname |
| whoami | `Whoami()` | Current user |
| env | `Env()` | Environment variables |
| which | `Which()` | Command lookup |
| sysinfo | `Sysinfo()` | System information |
| datetime | `Datetime()` | Current date/time |
| web_fetch | `Fetch(summarizer)` | Fetch web page content |
| web_search | `Search(creds)` | Search the web |
| scratchpad | `ScratchpadRead/Write/List/Search(store, persistent)` | Key-value memory |
| remember | `Remember(mem)` | Store observations |
| recall | `Recall(mem)` | Search observations |
| spawn_agent | `SpawnAgent(spawner)` | Spawn sub-agent |
| spawn_agents | `SpawnAgents(spawner)` | Spawn multiple sub-agents |
