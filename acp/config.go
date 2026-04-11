package acp

// ConfigCategory classifies a config option for client UX.
// Reserved categories: "mode", "model", "thought_level".
// Custom categories use underscore prefix (e.g., "_custom").
type ConfigCategory string

const (
	CategoryMode    ConfigCategory = "mode"
	CategoryModel   ConfigCategory = "model"
	CategoryThought ConfigCategory = "thought_level"
)

// ConfigOption is a runtime-adjustable setting exposed by the agent.
type ConfigOption struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Category ConfigCategory `json:"category,omitempty"`
	Type     string         `json:"type"` // currently only "select"
	Value    string         `json:"value"`
	Choices  []ConfigChoice `json:"choices,omitempty"`
	Meta     Meta           `json:"_meta,omitempty"`
}

// ConfigChoice is one selectable value for a config option.
type ConfigChoice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// SetConfigParams is sent by the host to change a config option.
type SetConfigParams struct {
	SessionID string `json:"sessionId"`
	OptionID  string `json:"optionId"`
	Value     string `json:"value"`
	Meta      Meta   `json:"_meta,omitempty"`
}

// SetConfigResult is returned by the agent.
type SetConfigResult struct {
	Meta Meta `json:"_meta,omitempty"`
}

// SetModeParams is sent by the host to switch session mode (deprecated).
type SetModeParams struct {
	SessionID string `json:"sessionId"`
	Mode      string `json:"mode"`
	Meta      Meta   `json:"_meta,omitempty"`
}

// SetModeResult is returned by the agent.
type SetModeResult struct {
	Meta Meta `json:"_meta,omitempty"`
}
