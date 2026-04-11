package acp

// Session update type discriminators.
const (
	UpdateMessage  = "messageChunk"
	UpdateToolCall = "toolCall"
	UpdatePlan     = "planUpdate"
	UpdateConfig   = "configOptionUpdate"
	UpdateCommands = "availableCommandsUpdate"
)

// Update is the payload of a session/update notification.
// The Type field determines which other fields are populated.
type Update struct {
	SessionID string `json:"sessionId"`
	Type      string `json:"type"`

	// messageChunk
	Role  string `json:"role,omitempty"`
	Chunk string `json:"chunk,omitempty"`

	// toolCall
	ToolCall *ToolCall `json:"toolCall,omitempty"`

	// planUpdate (full replacement of the plan)
	Plan []PlanEntry `json:"plan,omitempty"`

	// configOptionUpdate
	ConfigOption *ConfigOption `json:"configOption,omitempty"`

	// availableCommandsUpdate
	Commands []Command `json:"commands,omitempty"`

	Meta Meta `json:"_meta,omitempty"`
}
