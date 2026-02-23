# MCP Client Design

## What This Package Does

The `mcp` package connects your agent to external tool servers using the Model Context Protocol (MCP). MCP servers provide tools (filesystem access, web search, custom APIs) that your agent can discover and invoke.

## Why It Exists

Agents need tools to interact with the world:
- Read/write files
- Search the web
- Access databases
- Call external APIs

You could build these tools into your agent, but that's limiting:
- Every agent would need to implement the same tools
- Adding new tools requires agent changes
- Tools can't be shared across different agent frameworks

MCP solves this by standardizing tool communication. Your agent connects to MCP servers (separate processes) that provide tools. The agent discovers available tools and calls them as needed.

## When to Use It

**Use MCP for:**
- Connecting to existing MCP tool servers
- Accessing tools without building them yourself
- Sharing tools across multiple agents
- Using community-provided MCP servers

**Don't use MCP for:**
- Tools that must be built into your agent
- Very high-frequency tool calls (process IPC has overhead)
- Custom protocols (MCP is JSON-RPC specific)

## Core Concepts

### MCP Servers

An MCP server is an external process that provides tools. You configure it with:
- **Command** — The executable to run
- **Args** — Command-line arguments
- **Env** — Environment variables

The agent spawns the server process and communicates via stdin/stdout using JSON-RPC.

### Tool Discovery

When the agent connects to an MCP server, it asks "what tools do you have?" The server responds with a list of tools, each with:
- **Name** — Identifier for calling the tool
- **Description** — What the tool does (shown to LLMs)
- **Input schema** — JSON schema for the tool's parameters

The agent caches this list and exposes tools to the LLM.

### Tool Invocation

When the LLM decides to use a tool:
1. Agent sends `tools/call` request to the MCP server
2. Server executes the tool
3. Server returns result (text, images, or error)
4. Agent passes result back to LLM

The agent is the bridge between LLM and MCP servers.

## Architecture


Two main components:

**Client** — Handles one MCP server connection. Manages the subprocess, sends requests, receives responses.

**Manager** — Coordinates multiple clients. Aggregates tools from all connected servers. Routes tool calls to the right server.

### Multi-Server Support

An agent can connect to multiple MCP servers simultaneously:
- Filesystem server (file operations)
- Memory server (knowledge storage)
- Custom server (your application's API)

The manager exposes all tools as if they came from one source.

## Connection Lifecycle

### Startup Flow


1. Agent spawns MCP server process
2. Agent sends `initialize` request
3. Server responds with capabilities
4. Agent sends `initialized` notification
5. Agent sends `tools/list` request
6. Server responds with available tools
7. Connection is ready

### State Machine


Clients move through states:
- **Created** — Process spawned but not initialized
- **Ready** — Handshake complete, can call tools
- **Closed** — Connection terminated

### Shutdown

1. Agent closes stdin to server
2. Server detects EOF and exits
3. Agent waits for process to terminate

## Tool Aggregation

When you have multiple MCP servers, the manager combines their tools:


You can also filter tools:
- **Deny list** — Block specific tools from being exposed
- **Per-server filtering** — Different rules for different servers

This prevents accidentally exposing dangerous tools to the LLM.

## Common Patterns

### Basic Tool Server

Configure a server, connect, discover tools, use them:
1. Create client with server config
2. Initialize connection
3. List tools
4. Call tools as needed
5. Close when done

### Multiple Servers

Use the Manager to coordinate:
1. Add multiple server configs
2. Connect all
3. Get aggregated tool list
4. Manager routes calls automatically

### Tool Filtering

Some tools are dangerous (arbitrary code execution, system access). Use deny lists to prevent the LLM from seeing or calling them.

## Error Handling

| Situation | What Happens | What to Do |
|-----------|--------------|------------|
| Server won't start | Client creation fails | Check command path and permissions |
| Initialize timeout | Connection fails | Server may be slow or stuck |
| Tool not found | Call returns error | Check tool name spelling |
| Tool execution error | IsError=true in result | Check tool arguments |
| Server crashed | Subsequent calls fail | Reconnect or exit |

## Protocol Details

MCP uses JSON-RPC 2.0:
- **Requests** have ID, method, params
- **Responses** have ID and result/error
- **Notifications** have no ID, no response expected

Messages are newline-delimited JSON over stdio.

### Key Methods

| Method | Purpose |
|--------|---------|
| `initialize` | Start protocol, negotiate capabilities |
| `initialized` | Acknowledge initialization complete |
| `tools/list` | Get available tools |
| `tools/call` | Invoke a tool |

## Integration Notes

### With LLM Package

The LLM receives tool definitions (from MCP tool list) and may request tool calls. The agent executes those calls via MCP and feeds results back to the LLM.

### With Telemetry

Tool calls can be traced via the telemetry package. Each MCP call becomes a span with tool name, arguments, and result.

### With Shutdown

On agent shutdown, close all MCP clients. This terminates server processes cleanly.

## Design Decisions

### Why stdio?

Simple, universal, no network configuration. The agent spawns the server — stdin/stdout is the natural communication channel.

### Why separate servers?

Isolation. A buggy tool server can't crash the agent. Tools can be developed independently and shared.

### Why tool filtering?

Safety. LLMs might try to use tools inappropriately. Filtering is the last line of defense before execution.

## Testing Strategy

| Level | Focus |
|-------|-------|
| Unit | Message parsing, state transitions |
| Protocol | Initialize sequence, tool listing |
| Tool calls | Successful calls, error handling |
| Multi-server | Manager aggregation, routing |
| Failure | Server crashes, timeouts |
