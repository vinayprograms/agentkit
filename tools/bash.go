package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// defaultBashTimeout is the default command timeout.
const defaultBashTimeout = 120 * time.Second

type bashTool struct {
	workspace string
	timeout   time.Duration
}

// Bash returns a tool that executes shell commands in the given workspace.
// Security guards (e.g. shellguard) are layered on externally via
// registry.Register(tools.New(tools.Bash(workspace)).With(gate)).
func Bash(workspace string) Tool {
	return &bashTool{
		workspace: workspace,
		timeout:   defaultBashTimeout,
	}
}

func (t *bashTool) Name() string { return "bash" }

func (t *bashTool) Description() string {
	timeout := int(t.timeout.Seconds())
	return fmt.Sprintf(
		"Execute a shell command. Commands are killed after %d seconds. "+
			"Do NOT start long-running servers or processes that block indefinitely "+
			"— instead, start them in the background and test with a timeout. "+
			"Use as last resort — prefer dedicated tools (read, write, edit, grep, glob, tree, git) "+
			"when they cover the operation. Bash is best for: build commands, running tests, "+
			"piping multiple commands, or operations no built-in tool handles.",
		timeout,
	)
}

func (t *bashTool) Parameters() map[string]Param {
	return map[string]Param{
		"command": {
			Type:        StringParam,
			Description: "Shell command to execute",
			Required:    true,
		},
	}
}

func (t *bashTool) Execute(ctx context.Context, args Args) (string, error) {
	command, err := args.String("command")
	if err != nil {
		return "", err
	}

	// Apply timeout if the context doesn't already have a deadline.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = t.workspace

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	exitCode := 0
	if err != nil {
		// Timeout is a real error.
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("command timed out after %s", t.timeout)
		}
		// Non-zero exit is a result, not an error.
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Actual failure (e.g. command not found, exec error).
			return "", fmt.Errorf("failed to execute command: %w", err)
		}
	}

	// Format stdout/stderr/exit as a single result string.
	var b strings.Builder
	if out := stdout.String(); out != "" {
		b.WriteString(out)
	}
	if se := stderr.String(); se != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("STDERR:\n")
		b.WriteString(se)
	}
	if exitCode != 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "Exit code: %d", exitCode)
	}
	return b.String(), nil
}
