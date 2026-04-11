// Package content defines displayable blocks in prompts, outputs, and tool results.
package content

// Block type discriminators.
const (
	Text     = "text"
	Image    = "image"
	Audio    = "audio"
	Resource = "resource"
	Link     = "resource_link"
)

// Block is a single displayable unit. The Type field determines
// which other fields are populated.
type Block struct {
	Type string `json:"type"`

	// Text content.
	Text string `json:"text,omitempty"`

	// Image or audio (base64-encoded).
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`

	// Embedded resource (full file/resource content for @-mentions).
	Embedded *Embedded `json:"resource,omitempty"`

	// Resource link (reference the agent can fetch itself).
	URI         string `json:"uri,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`

	Annotations map[string]any `json:"annotations,omitempty"`
	Meta        map[string]any `json:"_meta,omitempty"`
}

// Embedded is file or resource content included inline.
type Embedded struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"` // base64 for binary
}
