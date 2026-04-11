// Package acp provides the Agent Client Protocol — a standard for communication
// between code editors (hosts) and AI coding agents.
//
// # Getting started
//
// To build an agent, use [github.com/vinayprograms/agentkit/acp/agent].
// To build a host (editor/IDE), use [github.com/vinayprograms/agentkit/acp/host].
//
// Protocol types live under acp/proto/:
//
//	acp/proto/content    — content blocks (text, image, audio, resources)
//	acp/proto/tool       — tool calls, permissions, lifecycle
//	acp/proto/prompt     — prompt turns and stop reasons
//	acp/proto/plan       — execution plan steps
//	acp/proto/config     — runtime settings and slash commands
//	acp/proto/update     — session update notifications
//	acp/proto/terminal   — terminal lifecycle management
//	acp/proto/fs         — file system operations
//	acp/proto/session    — session lifecycle
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
package acp
