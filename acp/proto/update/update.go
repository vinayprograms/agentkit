// Package update defines the session/update notification payload.
package update

import (
	"github.com/vinayprograms/agentkit/acp/proto/config"
	"github.com/vinayprograms/agentkit/acp/proto/plan"
	"github.com/vinayprograms/agentkit/acp/proto/tool"
)

// Type discriminators for session updates.
const (
	Message  = "messageChunk"
	ToolCall = "toolCall"
	Plan     = "planUpdate"
	Config   = "configOptionUpdate"
	Commands = "availableCommandsUpdate"
)

// Update is the payload of a session/update notification.
// The Type field determines which other fields are populated.
type Update struct {
	SessionID string `json:"sessionId"`
	Type      string `json:"type"`

	// Message chunk.
	Role  string `json:"role,omitempty"`
	Chunk string `json:"chunk,omitempty"`

	// Tool call.
	ToolCall *tool.Call `json:"toolCall,omitempty"`

	// Plan (full replacement).
	Plan []plan.Step `json:"plan,omitempty"`

	// Config option change.
	Setting *config.Option `json:"configOption,omitempty"`

	// Available slash commands.
	Commands []config.Command `json:"commands,omitempty"`

	Meta map[string]any `json:"_meta,omitempty"`
}
