# mcp

MCP (Model Context Protocol) client support. Connect to local or remote MCP tool servers.

## Usage

```go
// Local MCP server via stdio
client, err := mcp.Stdio(mcp.ServerConfig{
    Command: "npx",
    Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
})

// Remote MCP server via HTTP
client, err := mcp.HTTP("https://mcp.example.com/rpc")

// Same interface for both
defer client.Close()
client.Initialize(ctx)
tools, _ := client.ListTools(ctx)
result, _ := client.CallTool(ctx, "read_file", map[string]any{"path": "/tmp/test.txt"})
```

## Client Interface

```go
type Client interface {
    Initialize(ctx context.Context) error
    ListTools(ctx context.Context) ([]Tool, error)
    CallTool(ctx context.Context, name string, args map[string]any) (*Result, error)
    Tools() []Tool
    Close() error
}
```

Two implementations:
- `Stdio(config)` — spawns a local process, communicates via stdin/stdout
- `HTTP(endpoint)` — connects to a remote server via HTTP POST (JSON-RPC)

## Manager

Manage multiple MCP server connections:

```go
mgr := mcp.NewManager()

// Local server
mgr.Connect(ctx, "filesystem", mcp.ServerConfig{
    Command: "npx",
    Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
})

// Remote server
remote, _ := mcp.HTTP("https://mcp.atlassian.com/rpc")
remote.Initialize(ctx)
remote.ListTools(ctx)
mgr.Add("atlassian", remote)

// Deny specific tools
mgr.SetDeniedTools("filesystem", []string{"delete_file"})

// Discover and call tools
tools := mgr.AllTools()
server, found := mgr.FindTool("read_file")
result, err := mgr.CallTool(ctx, server, "read_file", args)

defer mgr.Close()
```
