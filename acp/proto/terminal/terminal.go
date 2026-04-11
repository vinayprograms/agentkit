// Package terminal defines terminal lifecycle types for host-mediated
// command execution.
package terminal

// Create is sent by the agent to launch a terminal via the host.
type Create struct {
	Command     string            `json:"command"`
	Args        []string          `json:"args,omitempty"`
	Cwd         string            `json:"cwd,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	OutputLimit int               `json:"outputLimit,omitempty"`
	Meta        map[string]any    `json:"_meta,omitempty"`
}

// Created is returned by terminal/create with the new terminal's ID.
type Created struct {
	TerminalID string         `json:"terminalId"`
	Meta       map[string]any `json:"_meta,omitempty"`
}

// Ref identifies an existing terminal for subsequent operations
// (output, wait, kill, release).
type Ref struct {
	TerminalID string         `json:"terminalId"`
	Meta       map[string]any `json:"_meta,omitempty"`
}

// Result is returned by output and wait operations.
// Output populates Output; wait populates both ExitCode and Output.
type Result struct {
	ExitCode int            `json:"exitCode,omitempty"`
	Output   string         `json:"output,omitempty"`
	Meta     map[string]any `json:"_meta,omitempty"`
}
