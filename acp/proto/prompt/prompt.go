// Package prompt defines the prompt turn request and response types.
package prompt

import (
	"github.com/vinayprograms/agentkit/acp/proto/config"
	"github.com/vinayprograms/agentkit/acp/proto/content"
)

// Reason indicates why a prompt turn ended.
type Reason string

const (
	EndTurn   Reason = "end_turn"
	MaxTokens Reason = "max_tokens"
	MaxTurns  Reason = "max_turn_requests"
	Refusal   Reason = "refusal"
	Cancelled Reason = "cancelled"
)

// Params is sent by the host to begin a prompt turn.
type Params struct {
	SessionID string          `json:"sessionId"`
	Content   []content.Block `json:"content"`
	Command   *config.Command `json:"command,omitempty"`
	Meta      map[string]any  `json:"_meta,omitempty"`
}

// Result is returned by the agent when the turn completes.
type Result struct {
	Reason Reason         `json:"stopReason"`
	Meta   map[string]any `json:"_meta,omitempty"`
}
