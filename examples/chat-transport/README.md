# Chat Transport Example

A simple real-time chat application demonstrating agentkit's WebSocket transport with JSON-RPC 2.0 messaging.

## Overview

This example shows how to build a multi-user chat server using:
- **WebSocket transport** for real-time bidirectional communication
- **JSON-RPC 2.0** for structured request/response and notifications
- **Server-side broadcast** for distributing messages to all clients

## Architecture

```
┌─────────────┐     WebSocket/JSON-RPC     ┌─────────────┐
│   Client    │◄──────────────────────────►│   Server    │
│  (client.go)│                            │ (server.go) │
└─────────────┘                            │             │
                                           │  Broadcasts │
┌─────────────┐     WebSocket/JSON-RPC     │  messages   │
│   Client    │◄──────────────────────────►│  to all     │
│  (client.go)│                            │  clients    │
└─────────────┘                            └─────────────┘
```

## JSON-RPC Methods

### Requests (Client → Server)

| Method | Params | Description |
|--------|--------|-------------|
| `join` | `{username: string}` | Join the chat with a username |
| `send` | `{message: string}` | Send a chat message |
| `list` | none | List connected users |

### Notifications (Server → Client)

| Method | Params | Description |
|--------|--------|-------------|
| `chat` | `{from, message, time}` | Chat message from another user |
| `system` | `{message}` | System notification (join/leave) |

## How to Run

### Prerequisites

```bash
# In the agentkit directory
go mod tidy
```

### Start the Server

```bash
cd examples/chat-transport
go run server.go
```

The server starts on `ws://localhost:8080/chat`.

### Connect Clients

In separate terminals, run clients with different usernames:

```bash
# Terminal 2
go run client.go Alice

# Terminal 3
go run client.go Bob
```

### Chat!

Type messages and press Enter to send. Use commands:
- `/list` - Show connected users
- `/quit` - Exit the chat

## Example Session

**Server output:**
```
Chat server starting on ws://localhost:8080/chat
Client client-1 connected
Client client-1 joined as Alice
Client client-2 connected
Client client-2 joined as Bob
```

**Alice's terminal:**
```
Connecting to ws://localhost:8080/chat...
Joined as 'Alice'. Type messages and press Enter to send.
Commands: /list (show users), /quit (exit)

*** Bob joined the chat ***
[14:32:15] Bob: Hello Alice!
Hi Bob!
```

**Bob's terminal:**
```
Connecting to ws://localhost:8080/chat...
Joined as 'Bob'. Type messages and press Enter to send.
Commands: /list (show users), /quit (exit)

Hello Alice!
[14:32:18] Alice: Hi Bob!
```

## Key Concepts Demonstrated

1. **WebSocketTransport** - Wrapping gorilla/websocket with agentkit's transport interface
2. **JSON-RPC parsing** - Using `transport.ParseInbound` for message handling
3. **Request/Response** - Handling methods with proper error codes
4. **Notifications** - Broadcasting events without expecting responses
5. **Connection management** - Tracking clients and handling disconnects

## Notes

- The server uses agentkit's `WebSocketTransport` for handling connections
- The client uses raw gorilla/websocket for request correlation
- In production, add authentication, rate limiting, and proper error handling
