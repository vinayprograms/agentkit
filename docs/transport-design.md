# Transport Abstraction Design

## Overview

Pluggable transport layer for agentkit supporting multiple backends while maintaining JSON-RPC 2.0 as the protocol.

## Design Decisions

| Area | Decision |
|------|----------|
| Concurrency | Channel-based API (Go idiomatic) |
| Protocol | JSON-RPC 2.0 (all transports) |
| Reconnection | Transport impl responsibility (in agent, not agentkit) |
| Architecture | One agent instance per user |

## Transports

### 1. stdio (current)
- Bidirectional over stdin/stdout
- Line-delimited JSON-RPC

### 2. WebSocket
- Bidirectional JSON-RPC over persistent connection
- Native fit for JSON-RPC

### 3. SSE + HTTP
- Server→Client: SSE stream (JSON-RPC notifications/responses)
- Client→Server: HTTP POST (JSON-RPC requests)
- Same JSON-RPC format, split transport

## Interface

```go
// Transport provides bidirectional message passing.
type Transport interface {
    // Recv returns channel for incoming messages (requests from client)
    Recv() <-chan *InboundMessage
    
    // Send queues a message for delivery (responses/notifications to client)
    Send(msg *OutboundMessage) error
    
    // Run starts the transport, blocks until ctx cancelled or error
    Run(ctx context.Context) error
    
    // Close shuts down the transport gracefully
    Close() error
}

// InboundMessage wraps an incoming JSON-RPC message.
type InboundMessage struct {
    Request      *Request      // Non-nil if this is a request
    Notification *Notification // Non-nil if this is a notification (no ID)
    Raw          []byte        // Original bytes for passthrough
}

// OutboundMessage wraps an outgoing JSON-RPC message.
type OutboundMessage struct {
    Response     *Response     // Non-nil if this is a response
    Notification *Notification // Non-nil if this is a notification
}
```

## Package Structure

```
transport/
├── transport.go      # Interface + types
├── jsonrpc.go        # JSON-RPC types (existing, cleaned up)
├── stdio.go          # StdioTransport
├── stdio_test.go
├── websocket.go      # WebSocketTransport
├── websocket_test.go
├── sse.go            # SSETransport (SSE + HTTP POST)
├── sse_test.go
└── doc.go            # Package documentation
```

## Implementation Notes

### Channel Sizing
- Recv channel: buffered (e.g., 100) to prevent blocking on slow handlers
- Internal send channel: buffered to allow non-blocking Send()

### Backpressure
- If recv channel fills, transport may drop messages or block (configurable)
- Send() returns error if transport is closed

### Error Handling
- Transport errors surface via Run() return
- Per-message errors use JSON-RPC error responses

### Graceful Shutdown
1. Close() signals shutdown
2. Transport drains pending sends
3. Run() returns nil or context error

## Migration Path

1. Keep existing Server for backward compatibility
2. Add Transport interface implementations
3. New code uses Transport directly
4. Eventually deprecate Server wrapper

## Testing Strategy

| Level | Focus |
|-------|-------|
| Unit | Message parsing, channel mechanics |
| Integration | Transport + handler round-trip |
| System | Full agent workflow over transport |
| Performance | Throughput, latency, memory |
| Failure | Disconnects, malformed input, timeouts |
| Security | Size limits, injection, DoS |
