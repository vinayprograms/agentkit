package acp

// Decision is the user's response to a permission request.
type Decision string

const (
	AllowOnce    Decision = "allow_once"
	AllowAlways  Decision = "allow_always"
	RejectOnce   Decision = "reject_once"
	RejectAlways Decision = "reject_always"
)

// PermissionParams is sent by the agent to request tool execution approval.
type PermissionParams struct {
	SessionID string   `json:"sessionId"`
	ToolCall  ToolCall `json:"toolCall"`
	Meta      Meta     `json:"_meta,omitempty"`
}

// PermissionResult is the host's decision.
type PermissionResult struct {
	Decision Decision `json:"decision"`
	Meta     Meta     `json:"_meta,omitempty"`
}
