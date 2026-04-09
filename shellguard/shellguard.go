// Package shellguard provides shell command security checking with a two-step pipeline:
// deterministic denylist checks followed by optional LLM-based analysis.
package shellguard

import (
	"context"
	"fmt"
	"time"

	"github.com/vinayprograms/agentkit/llm"
	"github.com/vinayprograms/agentkit/tools"
)

// Result contains the outcome of a command security check.
type Result struct {
	Allowed      bool
	Reason       string
	InputTokens  int
	OutputTokens int
}

// Gate is the security gate for shell commands. Commands pass through
// deterministic checks (banned commands, patterns, pipes) and optionally
// LLM-based analysis before being allowed to execute.
type Gate struct {
	shell              Shell
	allowedDirs        []string
	userDeniedCommands []string
	workspace          string
	model              llm.Model
	securityScope      string

	// OnDecision is called after each security decision for logging/auditing.
	OnDecision func(command, step string, allowed bool, reason string, durationMs int64, inputTokens, outputTokens int)
}

// New creates a new shell command security gate.
// shell determines how commands are parsed (use BashShell{}, FishShell{}, or PosixShell{}).
// model is optional (nil for deterministic-only checks).
// securityScope is optional ("" for normal mode).
func New(shell Shell, workspace string, allowedDirs, userDeniedCommands []string, model llm.Model, securityScope string) *Gate {
	return &Gate{
		shell:              shell,
		allowedDirs:        allowedDirs,
		userDeniedCommands: userDeniedCommands,
		workspace:          workspace,
		model:              model,
		securityScope:      securityScope,
	}
}

// Check implements tools.Guard. It extracts "command" from args and runs the security pipeline.
func (g *Gate) Check(ctx context.Context, args tools.Args) error {
	command, err := args.String("command")
	if err != nil {
		return fmt.Errorf("shellguard: %w", err)
	}
	return g.check(ctx, command)
}

// check runs the security pipeline: deterministic checks, then LLM analysis if configured.
func (g *Gate) check(ctx context.Context, command string) error {
	// Step 1: deterministic checks
	allowed, reason := g.checkDeterministic(command)
	g.logDecision(command, "deterministic", allowed, reason, 0, 0, 0)
	if !allowed {
		return fmt.Errorf("blocked: %s", reason)
	}

	// Step 2: LLM analysis (if model configured and allowed dirs set)
	if g.model != nil && len(g.allowedDirs) > 0 {
		start := time.Now()
		result, err := llmCheck(ctx, g.model, command, g.allowedDirs, g.workspace, g.securityScope)
		durationMs := time.Since(start).Milliseconds()
		if err != nil {
			g.logDecision(command, "llm", false, fmt.Sprintf("error: %v", err), durationMs, 0, 0)
			return fmt.Errorf("LLM check failed: %v", err)
		}
		g.logDecision(command, "llm", result.Allowed, result.Reason, durationMs, result.InputTokens, result.OutputTokens)
		if !result.Allowed {
			return fmt.Errorf("blocked: %s", result.Reason)
		}
	}

	return nil
}

func (g *Gate) logDecision(command, step string, allowed bool, reason string, durationMs int64, inputTokens, outputTokens int) {
	if g.OnDecision != nil {
		g.OnDecision(command, step, allowed, reason, durationMs, inputTokens, outputTokens)
	}
}

// CheckDeterministic performs only the fast deterministic checks (for testing/preview).
func (g *Gate) CheckDeterministic(command string) (bool, string) {
	return g.checkDeterministic(command)
}
