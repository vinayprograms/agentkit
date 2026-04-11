// Package acp defines the Agent Client Protocol wire types.
//
// ACP standardizes communication between code editors (hosts) and AI coding
// agents. This package contains the shared protocol types used by both sides.
// For implementations, see acp/agent (agent-side) and acp/host (host-side).
//
// The protocol is built on JSON-RPC 2.0 with bidirectional communication:
// both host and agent can send requests and notifications to each other.
//
// # Message Flow
//
//	Host                          Agent
//	  │                             │
//	  │──── initialize ────────────▶│
//	  │◀─── result ────────────────│
//	  │──── authenticate ──────────▶│
//	  │◀─── result ────────────────│
//	  │──── session/new ───────────▶│
//	  │◀─── result ────────────────│
//	  │──── session/prompt ────────▶│
//	  │◀─── session/update ────────│ (notifications: chunks, tool calls, plan)
//	  │◀─── request_permission ────│ (agent asks host)
//	  │──── result ────────────────▶│
//	  │◀─── fs/read_text_file ─────│ (agent asks host)
//	  │──── result ────────────────▶│
//	  │◀─── result ────────────────│ (prompt complete)
//
// # Extensibility
//
// Every protocol type supports a Meta field (serialized as "_meta") for custom
// data without breaking the core protocol. Reserved meta keys: "traceparent",
// "tracestate", "baggage" (W3C trace context).
package acp
