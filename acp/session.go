package acp

// Session represents an active agent session.
type Session struct {
	ID       string            `json:"id"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// NewSessionParams is sent by the host to create a session.
type NewSessionParams struct {
	Cwd      string            `json:"cwd"`
	Metadata map[string]string `json:"metadata,omitempty"`
	MCP      []MCPServer       `json:"mcpServers,omitempty"`
	Meta     Meta              `json:"_meta,omitempty"`
}

// NewSessionResult is returned by the agent.
type NewSessionResult struct {
	Session Session `json:"session"`
	Meta    Meta    `json:"_meta,omitempty"`
}

// LoadSessionParams is sent by the host to restore a previous session.
type LoadSessionParams struct {
	SessionID string `json:"sessionId"`
	Meta      Meta   `json:"_meta,omitempty"`
}

// LoadSessionResult is returned after the agent replays session history.
type LoadSessionResult struct {
	Session Session `json:"session"`
	Meta    Meta    `json:"_meta,omitempty"`
}

// CancelParams is sent by the host to cancel the current prompt turn.
// This is a notification (no response expected).
type CancelParams struct {
	SessionID string `json:"sessionId"`
	Meta      Meta   `json:"_meta,omitempty"`
}

// MCPServer describes an MCP server the agent should connect to.
type MCPServer struct {
	Name      string    `json:"name"`
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
