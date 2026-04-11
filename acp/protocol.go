package acp

import "encoding/json"

// Info describes the agent.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Capabilities advertises agent features.
type Capabilities struct {
	LoadSession        bool               `json:"loadSession,omitempty"`
	PromptCapabilities PromptCapabilities `json:"promptCapabilities,omitempty"`
}

// PromptCapabilities describes what prompts can contain.
type PromptCapabilities struct {
	Image           bool `json:"image,omitempty"`
	Audio           bool `json:"audio,omitempty"`
	EmbeddedContext bool `json:"embeddedContext,omitempty"`
}

// Session represents an agent session.
type Session struct {
	ID       string            `json:"id"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Request is a JSON-RPC request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC response.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      any `json:"id"`
	Result  any `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
}

// Notification is a JSON-RPC notification.
type Notification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  any `json:"params,omitempty"`
}

// Error is a JSON-RPC error.
type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    any `json:"data,omitempty"`
}

// InitializeRequest is the initialize request params.
type InitializeRequest struct {
	ProtocolVersion string             `json:"protocolVersion"`
	ClientInfo      ClientInfo         `json:"clientInfo"`
	Capabilities    ClientCapabilities `json:"capabilities"`
}

// ClientInfo describes the client.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientCapabilities describes client features.
type ClientCapabilities struct {
	Terminal      bool `json:"terminal,omitempty"`
	ReadTextFile  bool `json:"fs.readTextFile,omitempty"`
	WriteTextFile bool `json:"fs.writeTextFile,omitempty"`
}

// InitializeResponse is the initialize response.
type InitializeResponse struct {
	ProtocolVersion string       `json:"protocolVersion"`
	AgentInfo       Info         `json:"agentInfo"`
	Capabilities    Capabilities `json:"capabilities"`
}

// NewSessionRequest creates a new session.
type NewSessionRequest struct {
	Metadata map[string]string `json:"metadata,omitempty"`
}

// NewSessionResponse returns the session.
type NewSessionResponse struct {
	Session Session `json:"session"`
}

// PromptRequest is a prompt turn request.
type PromptRequest struct {
	SessionID string        `json:"sessionId"`
	Prompt    []PromptPart  `json:"prompt"`
	Command   *CommandInput `json:"command,omitempty"`
}

// PromptPart is a part of a prompt.
type PromptPart struct {
	Type string `json:"type"` // "text", "image", "audio"
	Text string `json:"text,omitempty"`
	Data string `json:"data,omitempty"` // base64
	Mime string `json:"mimeType,omitempty"`
}

// CommandInput is a slash command.
type CommandInput struct {
	Name  string `json:"name"`
	Input string `json:"input,omitempty"`
}

// PromptResponse is the response to a prompt.
type PromptResponse struct {
	StopReason string `json:"stopReason"` // "endTurn", "cancelled", "error"
}

// SessionNotification notifies about session updates.
type SessionNotification struct {
	SessionID string        `json:"sessionId"`
	Update    SessionUpdate `json:"update"`
}

// SessionUpdate is a session state update.
type SessionUpdate struct {
	Type string `json:"type"` // "messageChunk", "toolCall", "planUpdate"
	// For messageChunk
	Role  string `json:"role,omitempty"`
	Chunk string `json:"chunk,omitempty"`
	// For toolCall
	ToolCall *ToolCallUpdate `json:"toolCall,omitempty"`
}

// ToolCallUpdate is a tool call notification.
type ToolCallUpdate struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"` // "running", "completed", "error"
	Input  string `json:"input,omitempty"`
	Output string `json:"output,omitempty"`
}
