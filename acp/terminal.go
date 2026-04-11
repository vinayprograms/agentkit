package acp

// CreateTerminalParams is sent by the agent to launch a terminal via the host.
type CreateTerminalParams struct {
	Command     string            `json:"command"`
	Args        []string          `json:"args,omitempty"`
	Cwd         string            `json:"cwd,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	OutputLimit int               `json:"outputLimit,omitempty"`
	Meta        Meta              `json:"_meta,omitempty"`
}

// CreateTerminalResult returns the terminal identifier.
type CreateTerminalResult struct {
	TerminalID string `json:"terminalId"`
	Meta       Meta   `json:"_meta,omitempty"`
}

// TerminalOutputParams requests output from a terminal (non-blocking).
type TerminalOutputParams struct {
	TerminalID string `json:"terminalId"`
	Meta       Meta   `json:"_meta,omitempty"`
}

// TerminalOutputResult contains available terminal output.
type TerminalOutputResult struct {
	Output string `json:"output"`
	Meta   Meta   `json:"_meta,omitempty"`
}

// TerminalWaitParams blocks until the terminal process exits.
type TerminalWaitParams struct {
	TerminalID string `json:"terminalId"`
	Meta       Meta   `json:"_meta,omitempty"`
}

// TerminalWaitResult returns the exit code and final output.
type TerminalWaitResult struct {
	ExitCode int    `json:"exitCode"`
	Output   string `json:"output,omitempty"`
	Meta     Meta   `json:"_meta,omitempty"`
}

// TerminalKillParams stops the terminal process but preserves the terminal.
type TerminalKillParams struct {
	TerminalID string `json:"terminalId"`
	Meta       Meta   `json:"_meta,omitempty"`
}

// TerminalKillResult is returned on success.
type TerminalKillResult struct {
	Meta Meta `json:"_meta,omitempty"`
}

// TerminalReleaseParams releases all terminal resources.
type TerminalReleaseParams struct {
	TerminalID string `json:"terminalId"`
	Meta       Meta   `json:"_meta,omitempty"`
}

// TerminalReleaseResult is returned on success.
type TerminalReleaseResult struct {
	Meta Meta `json:"_meta,omitempty"`
}
