# mcp

MCP (Model Context Protocol) client support. Connect to local or remote MCP tool servers.

## Usage

```go
ctx := context.Background()

// Local MCP server via stdio
client, err := mcp.Stdio(ctx, mcp.ServerConfig{
    Command: "npx",
    Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
})

// Remote MCP server via HTTP
client, err := mcp.HTTP(ctx, "https://mcp.example.com/rpc")

// Both return ready-to-use clients
tools := client.Tools()
result, _ := client.CallTool(ctx, "read_file", map[string]any{"path": "/tmp/test.txt"})
defer client.Close()
```

## Client Interface

```go
type Client interface {
    ListTools(ctx context.Context) ([]Tool, error)
    CallTool(ctx context.Context, name string, args map[string]any) (*Result, error)
    Tools() []Tool
    Close() error
}
```

Clients are ready after creation — `Stdio()` and `HTTP()` handle the MCP handshake and tool discovery internally.

## Manager

Manage multiple MCP server connections:

```go
mgr := mcp.NewManager()

// Register servers
client, _ := mcp.Stdio(ctx, mcp.ServerConfig{Command: "npx", Args: []string{...}})
mgr.Register("filesystem", client)

remote, _ := mcp.HTTP(ctx, "https://mcp.atlassian.com/rpc")
mgr.Register("atlassian", remote)

// Deny specific tools
mgr.Deny("filesystem", []string{"delete_file"})

// Discover and call tools
tools := mgr.AllTools()
server, found := mgr.FindTool("read_file")
result, err := mgr.CallTool(ctx, server, "read_file", args)

defer mgr.Close()
```
