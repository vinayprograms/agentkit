// Package transport provides pluggable transports for JSON-RPC 2.0 communication.
//
// # Overview
//
// The transport package implements bidirectional message passing over various
// backends while maintaining JSON-RPC 2.0 as the protocol. All transports
// implement the Transport interface with channel-based APIs for Go-idiomatic
// concurrent use.
//
// # Available Transports
//
//   - StdioTransport: Communication over stdin/stdout (for CLI tools)
//   - WebSocketTransport: Bidirectional over WebSocket (for real-time UIs)
//   - SSETransport: Server-Sent Events + HTTP POST (for web clients)
//
// # Usage
//
// All transports follow the same pattern:
//
//	t := transport.NewStdioTransport(os.Stdin, os.Stdout, transport.DefaultConfig())
//	go t.Run(ctx)
//
//	for msg := range t.Recv() {
//	    if msg.Request != nil {
//	        // Handle request, send response
//	        t.Send(&transport.OutboundMessage{
//	            Response: &transport.Response{
//	                JSONRPC: "2.0",
//	                ID:      msg.Request.ID,
//	                Result:  result,
//	            },
//	        })
//	    }
//	}
//
// # Design Decisions
//
//   - Channel-based API: Go-idiomatic for concurrent use
//   - JSON-RPC 2.0: Protocol for all transports
//   - Reconnection: Handled by agent implementations, not agentkit
//
// # Thread Safety
//
// All transport methods are safe for concurrent use. The Recv() channel
// is closed when the transport shuts down.
package transport
