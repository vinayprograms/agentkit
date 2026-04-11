package acp

// ReadFileParams is sent by the agent to read a file via the host.
// The host returns editor buffer content, not just disk state.
type ReadFileParams struct {
	Path  string `json:"path"`            // absolute path
	Line  int    `json:"line,omitempty"`  // 1-based start line
	Limit int    `json:"limit,omitempty"` // max lines to return
	Meta  Meta   `json:"_meta,omitempty"`
}

// ReadFileResult contains the file content.
type ReadFileResult struct {
	Content string `json:"content"`
	Meta    Meta   `json:"_meta,omitempty"`
}

// WriteFileParams is sent by the agent to write a file via the host.
// Creates the file if it does not exist.
type WriteFileParams struct {
	Path    string `json:"path"` // absolute path
	Content string `json:"content"`
	Meta    Meta   `json:"_meta,omitempty"`
}

// WriteFileResult is returned on success (no content).
type WriteFileResult struct {
	Meta Meta `json:"_meta,omitempty"`
}
