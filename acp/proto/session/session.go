// Package session defines the session lifecycle types.
package session

// Session represents an active agent session.
type Session struct {
	ID       string            `json:"id"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Params is sent by the host to create a new session.
type Params struct {
	Cwd      string            `json:"cwd"`
	Metadata map[string]string `json:"metadata,omitempty"`
	MCP      []MCPServer       `json:"mcpServers,omitempty"`
	Meta     map[string]any    `json:"_meta,omitempty"`
}

// Result is returned by the agent after session creation.
type Result struct {
	Session Session        `json:"session"`
	Meta    map[string]any `json:"_meta,omitempty"`
}

// LoadParams is sent by the host to restore a previous session.
type LoadParams struct {
	SessionID string         `json:"sessionId"`
	Meta      map[string]any `json:"_meta,omitempty"`
}

// LoadResult is returned after the agent replays session history.
type LoadResult struct {
	Session Session        `json:"session"`
	Meta    map[string]any `json:"_meta,omitempty"`
}

// Cancel is sent by the host to cancel the current prompt turn.
// This is a notification (no response expected).
type Cancel struct {
	SessionID string         `json:"sessionId"`
	Meta      map[string]any `json:"_meta,omitempty"`
}

// MCPServer describes an MCP server the agent should connect to.
type MCPServer struct {
	Name      string       `json:"name"`
	Transport MCPTransport `json:"transport"`
}

// MCPTransport describes how to reach an MCP server.
type MCPTransport struct {
	Type    string            `json:"type"` // "stdio", "http", "sse"
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}
