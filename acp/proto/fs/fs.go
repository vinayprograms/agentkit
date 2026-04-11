// Package fs defines file system operations mediated by the host.
// The host returns editor buffer content, not just disk state.
package fs

// ReadParams is sent by the agent to read a file via the host.
type ReadParams struct {
	Path  string         `json:"path"`            // absolute path
	Line  int            `json:"line,omitempty"`   // 1-based start line
	Limit int            `json:"limit,omitempty"`  // max lines to return
	Meta  map[string]any `json:"_meta,omitempty"`
}

// ReadResult contains the file content.
type ReadResult struct {
	Content string         `json:"content"`
	Meta    map[string]any `json:"_meta,omitempty"`
}

// WriteParams is sent by the agent to write a file via the host.
// Creates the file if it does not exist.
type WriteParams struct {
	Path    string         `json:"path"` // absolute path
	Content string         `json:"content"`
	Meta    map[string]any `json:"_meta,omitempty"`
}

// WriteResult is returned on success.
type WriteResult struct {
	Meta map[string]any `json:"_meta,omitempty"`
}
