// Package tool defines tool call types, lifecycle, and permission model.
package tool

import "github.com/vinayprograms/agentkit/acp/proto/content"

// Kind categorizes a tool call for client UX.
type Kind string

const (
	Read    Kind = "read"
	Edit    Kind = "edit"
	Delete  Kind = "delete"
	Move    Kind = "move"
	Search  Kind = "search"
	Execute Kind = "execute"
	Think   Kind = "think"
	Fetch   Kind = "fetch"
	Other   Kind = "other"
)

// Status tracks tool call lifecycle.
type Status string

const (
	Pending Status = "pending"
	Running Status = "in_progress"
	Done    Status = "completed"
	Failed  Status = "failed"
)

// Call represents a tool invocation with its full lifecycle.
type Call struct {
	ID         string          `json:"id"`
	Title      string          `json:"title,omitempty"`
	Kind       Kind            `json:"kind,omitempty"`
	Status     Status          `json:"status"`
	Input      string          `json:"input,omitempty"`
	Output     []content.Block `json:"output,omitempty"`
	Location   *Location       `json:"location,omitempty"`
	TerminalID string          `json:"terminalId,omitempty"`
	Diff       *Diff           `json:"diff,omitempty"`
	Meta       map[string]any  `json:"_meta,omitempty"`
}

// Location identifies a file position being accessed.
type Location struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
}

// Diff represents a structured text change in tool output.
type Diff struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

// Decision is the user's response to a permission request.
type Decision string

const (
	AllowOnce    Decision = "allow_once"
	AllowAlways  Decision = "allow_always"
	RejectOnce   Decision = "reject_once"
	RejectAlways Decision = "reject_always"
)

// Permission is sent by the agent to request tool execution approval.
type Permission struct {
	SessionID string         `json:"sessionId"`
	ToolCall  Call           `json:"toolCall"`
	Meta      map[string]any `json:"_meta,omitempty"`
}

// Approval is the host's decision on a permission request.
type Approval struct {
	Decision Decision       `json:"decision"`
	Meta     map[string]any `json:"_meta,omitempty"`
}
