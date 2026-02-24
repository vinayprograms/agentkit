# Chat Transport Example

Demonstrates WebSocket JSON-RPC communication using agentkit's transport package.

## Components

- **server/** - WebSocket server that handles chat messages
- **client/** - Interactive client that connects and sends messages

## Running

Start the server:
```bash
go run ./examples/chat-transport/server/
```

In another terminal, start the client:
```bash
go run ./examples/chat-transport/client/
```

## What This Shows

- Setting up WebSocket transport
- JSON-RPC message handling
- Bidirectional communication
