package acp

// ToolKind categorizes a tool call for client UX.
type ToolKind string

const (
	KindRead    ToolKind = "read"
	KindEdit    ToolKind = "edit"
	KindDelete  ToolKind = "delete"
	KindMove    ToolKind = "move"
	KindSearch  ToolKind = "search"
	KindExecute ToolKind = "execute"
	KindThink   ToolKind = "think"
	KindFetch   ToolKind = "fetch"
	KindOther   ToolKind = "other"
)

// ToolStatus tracks tool call lifecycle.
type ToolStatus string

const (
	ToolPending ToolStatus = "pending"
	ToolRunning ToolStatus = "in_progress"
	ToolDone    ToolStatus = "completed"
	ToolFailed  ToolStatus = "failed"
)

// ToolCall represents a tool invocation with its full lifecycle.
type ToolCall struct {
	ID         string     `json:"id"`
	Title      string     `json:"title,omitempty"`
	Kind       ToolKind   `json:"kind,omitempty"`
	Status     ToolStatus `json:"status"`
	Input      string     `json:"input,omitempty"`
	Output     []Content  `json:"output,omitempty"`
	Location   *Location  `json:"location,omitempty"`
	TerminalID string     `json:"terminalId,omitempty"`
	Diff       *Diff      `json:"diff,omitempty"`
	Meta       Meta       `json:"_meta,omitempty"`
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
