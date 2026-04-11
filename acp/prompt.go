package acp

// StopReason indicates why a prompt turn ended.
type StopReason string

const (
	StopEnd       StopReason = "end_turn"
	StopTokens    StopReason = "max_tokens"
	StopTurns     StopReason = "max_turn_requests"
	StopRefusal   StopReason = "refusal"
	StopCancelled StopReason = "cancelled"
)

// PromptParams is sent by the host to begin a prompt turn.
type PromptParams struct {
	SessionID string    `json:"sessionId"`
	Content   []Content `json:"content"`
	Command   *Command  `json:"command,omitempty"`
	Meta      Meta      `json:"_meta,omitempty"`
}

// PromptResult is returned by the agent when the turn completes.
type PromptResult struct {
	StopReason StopReason `json:"stopReason"`
	Meta       Meta       `json:"_meta,omitempty"`
}
