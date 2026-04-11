package acp

// Command is a slash command advertised by the agent.
type Command struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputHint   string `json:"inputHint,omitempty"`
	Input       string `json:"input,omitempty"`
}
