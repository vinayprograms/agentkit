// Package config defines runtime settings, modes, and slash commands.
package config

// Category classifies an option for client UX.
// Reserved categories: Mode, Model, Thought.
// Custom categories use underscore prefix (e.g., "_custom").
type Category string

const (
	Mode    Category = "mode"
	Model   Category = "model"
	Thought Category = "thought_level"
)

// Option is a runtime-adjustable setting exposed by the agent.
type Option struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Category Category       `json:"category,omitempty"`
	Type     string         `json:"type"` // currently only "select"
	Value    string         `json:"value"`
	Choices  []Choice       `json:"choices,omitempty"`
	Meta     map[string]any `json:"_meta,omitempty"`
}

// Choice is one selectable value for an option.
type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// SetParams is sent by the host to change an option.
type SetParams struct {
	SessionID string         `json:"sessionId"`
	OptionID  string         `json:"optionId"`
	Value     string         `json:"value"`
	Meta      map[string]any `json:"_meta,omitempty"`
}

// SetResult is returned by the agent.
type SetResult struct {
	Meta map[string]any `json:"_meta,omitempty"`
}

// ModeParams is sent by the host to switch session mode (deprecated).
type ModeParams struct {
	SessionID string         `json:"sessionId"`
	Mode      string         `json:"mode"`
	Meta      map[string]any `json:"_meta,omitempty"`
}

// ModeResult is returned by the agent.
type ModeResult struct {
	Meta map[string]any `json:"_meta,omitempty"`
}

// Command is a slash command advertised by the agent.
type Command struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputHint   string `json:"inputHint,omitempty"`
	Input       string `json:"input,omitempty"`
}
