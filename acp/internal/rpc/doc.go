// Package rpc provides a bidirectional JSON-RPC 2.0 connection over
// newline-delimited JSON on io.Reader/io.Writer.
//
// Both acp/agent and acp/host use this package for wire communication.
// It is internal — consumers use the agent or host packages, not this one.
//
// Usage contract: register all handlers before calling Run.
//
//	conn := rpc.NewConn(reader, writer)
//	conn.Handle("session/prompt", handlePrompt)
//	conn.HandleNotification("session/update", handleUpdate)
//	err := conn.Run(ctx) // blocks until EOF or context cancellation
package rpc
