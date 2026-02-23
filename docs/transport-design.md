# Transport Abstraction Design

## What This Package Does

The `transport` package handles how agents communicate with the outside world. It provides a pluggable layer that can use different connection types (stdin/stdout, WebSocket, HTTP) while keeping the same message format (JSON-RPC 2.0).

## Why It Exists

Agents need to receive commands and send responses. Where those commands come from varies:

- **CLI tools** — Use stdin/stdout (simple pipes)
- **Web clients** — Use WebSocket or HTTP
- **Other services** — Might use any of the above

Without a transport abstraction, you'd need different agent implementations for each connection type. The transport layer lets you write one agent that works over any connection.

## When to Use It

**Use transport for:**
- Building agents that need to communicate with external clients
- Supporting multiple connection types from the same agent code
- Implementing custom communication protocols

**Don't use transport for:**
- Agent-to-agent communication within a swarm (use `bus`)
- Storing data (use `state`)
- Internal function calls

## Core Concepts

### JSON-RPC 2.0

All transports use JSON-RPC 2.0 as the message protocol. This is a simple format where:
- **Requests** have an ID, method name, and parameters
- **Responses** have the same ID, plus result or error
- **Notifications** are requests without an ID (no response expected)

Using a standard protocol means clients can be written in any language.

### Inbound and Outbound

The transport handles bidirectional messaging:
- **Inbound** — Messages coming into the agent (requests, notifications)
- **Outbound** — Messages going out (responses, notifications)

Each direction uses a channel. The agent reads from the inbound channel, processes messages, and writes to the outbound channel.

### Transport Lifecycle

1. **Create** — Instantiate the transport with configuration
2. **Run** — Start the transport (blocks until shutdown)
3. **Process** — Read inbound, write outbound in your handler
4. **Close** — Gracefully shut down

## Available Transports

### stdio

Communicates over standard input/output. Each message is a line of JSON.

**Best for:** CLI tools, piped commands, simple integrations

**How it works:** Reads lines from stdin, writes lines to stdout. No network involved.

### WebSocket

Bidirectional communication over a persistent WebSocket connection.

**Best for:** Web clients, real-time UIs, long-running connections

**How it works:** Client connects via WebSocket. Messages flow both directions over the same connection.

### SSE + HTTP

Server-Sent Events for server-to-client, HTTP POST for client-to-server.

**Best for:** Environments where WebSocket isn't available, one-way streaming with occasional requests

**How it works:** Client opens SSE connection to receive updates. Client sends requests via HTTP POST to a separate endpoint.

## Design Decisions

### Why JSON-RPC?

JSON-RPC is simple, well-specified, and language-agnostic. It's easy to debug (human-readable JSON) and has client libraries in every language.

### Why channels?

Channels are idiomatic Go. They naturally handle concurrent message processing and provide backpressure when handlers are slow.

### Why one agent per user?

The transport assumes one agent instance per connection. This simplifies state management — each agent handles one user's session.

## Message Flow

1. Client sends JSON-RPC request over transport
2. Transport parses and puts on inbound channel
3. Agent handler reads from channel, processes request
4. Handler creates response, puts on outbound channel
5. Transport serializes and sends to client

For notifications (no response expected), steps 4-5 are skipped.

## Error Handling

| Situation | What Happens |
|-----------|--------------|
| Malformed JSON | Transport returns JSON-RPC parse error |
| Invalid request | Handler returns JSON-RPC invalid request error |
| Handler error | Handler returns JSON-RPC application error |
| Connection lost | Transport's Run() returns error |
| Channel full | Configurable: drop message or block |

## Backpressure

If the agent can't process messages fast enough, channels fill up. The transport can either:
- **Block** — Stop accepting new messages until space opens
- **Drop** — Discard oldest messages to make room

The right choice depends on your application. Blocking preserves all messages but may cause client timeouts. Dropping keeps things responsive but loses data.

## Testing Strategy

| Level | Focus |
|-------|-------|
| Unit | Message parsing, channel mechanics |
| Integration | Transport + handler round-trip |
| System | Full agent workflow over transport |
| Performance | Throughput, latency, memory usage |
| Failure | Disconnects, malformed input, timeouts |
| Security | Size limits, injection prevention |
