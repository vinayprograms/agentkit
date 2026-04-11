package acp

// Content type discriminators.
const (
	TypeText     = "text"
	TypeImage    = "image"
	TypeAudio    = "audio"
	TypeResource = "resource"
	TypeLink     = "resource_link"
)

// Content is a displayable block in prompts, outputs, and tool results.
// The Type field determines which other fields are populated.
type Content struct {
	Type string `json:"type"`

	// Text content.
	Text string `json:"text,omitempty"`

	// Image or audio (base64-encoded).
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`

	// Embedded resource (full file/resource content for @-mentions).
	Resource *Resource `json:"resource,omitempty"`

	// Resource link (reference the agent can fetch itself).
	URI         string `json:"uri,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`

	Annotations map[string]any `json:"annotations,omitempty"`
	Meta        Meta           `json:"_meta,omitempty"`
}

// Resource is embedded file or resource content.
type Resource struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"` // base64 for binary
}
