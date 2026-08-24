// Package shellguard provides shell command security checking with a two-step pipeline:
// deterministic denylist checks followed by optional LLM-based analysis.
package shellguard

import (
	"context"
	"fmt"
	"time"

	"github.com/vinayprograms/agentkit/llm"
	"github.com/vinayprograms/agentkit/tools"
	"go.opentelemetry.io/otel/attribute"
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
	disabledTools      []string
	workspace          string
	model              llm.Model
	securityScope      string

	// OnDecision is called after each security decision for logging/auditing.
	OnDecision func(command, step string, allowed bool, reason string, durationMs int64, inputTokens, outputTokens int)
}

// Config is the input to NewGate. It is additive: every New parameter has a
// same-named field, plus DisabledTools, which New has no way to set.
type Config struct {
	// Shell determines how commands are parsed (use BashShell{}, FishShell{}, or PosixShell{}).
	Shell Shell
	// Workspace is the cwd commands execute in.
	Workspace string
	// AllowedDirs are the directories the LLM stage treats as writable, and
	// as the boundary for data reads (see llm.go's data-read vs
	// toolchain-read distinction).
	AllowedDirs []string
	// UserDeniedCommands are policy-denied base command names. They are
	// enforced deterministically (exact base-name match) and also passed to
	// the LLM stage so it can catch a command that achieves the same effect
	// by another route.
	UserDeniedCommands []string
	// DisabledTools are agent tool names (e.g. "write", "read") disabled by
	// policy. The LLM stage uses this so bash can't be used as a side door
	// around a disabled tool: a disabled "write"/"edit" blocks all bash
	// writes, a disabled "read" blocks all bash data reads (even inside
	// AllowedDirs).
	DisabledTools []string
	// Model is optional (nil for deterministic-only checks).
	Model llm.Model
	// SecurityScope is optional ("" for normal mode).
	SecurityScope string
}

// NewGate creates a new shell command security gate from a Config. This is
// the preferred constructor — see New's deprecation note.
func NewGate(cfg Config) *Gate {
	return &Gate{
		shell:              cfg.Shell,
		allowedDirs:        cfg.AllowedDirs,
		userDeniedCommands: cfg.UserDeniedCommands,
		disabledTools:      cfg.DisabledTools,
		workspace:          cfg.Workspace,
		model:              cfg.Model,
		securityScope:      cfg.SecurityScope,
	}
}

// New creates a new shell command security gate.
// shell determines how commands are parsed (use BashShell{}, FishShell{}, or PosixShell{}).
// model is optional (nil for deterministic-only checks).
// securityScope is optional ("" for normal mode).
//
// Deprecated: use NewGate, which also accepts DisabledTools. New delegates
// to it unchanged and is kept for existing callers.
func New(shell Shell, workspace string, allowedDirs, userDeniedCommands []string, model llm.Model, securityScope string) *Gate {
	return NewGate(Config{
		Shell:              shell,
		Workspace:          workspace,
		AllowedDirs:        allowedDirs,
		UserDeniedCommands: userDeniedCommands,
		Model:              model,
		SecurityScope:      securityScope,
	})
}

// Check implements tools.Guard. It extracts "command" from args and runs the security pipeline.
func (g *Gate) Check(ctx context.Context, args tools.Args) (err error) {
	ctx, end := trace(ctx, "check")
	defer end(&err)

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
		event(ctx, "deterministic.blocked", attribute.String("reason", reason))
		return fmt.Errorf("blocked: %s", reason)
	}
	event(ctx, "deterministic.passed")

	// Step 2: LLM analysis (if model configured).
	// allowedDirs is context for the LLM prompt, not a gate for running it.
	if g.model == nil {
		event(ctx, "llm.skipped")
		return nil
	}

	event(ctx, "llm.started")
	start := time.Now()
	result, err := llmCheck(ctx, g.model, command, g.allowedDirs, g.userDeniedCommands, g.disabledTools, g.workspace, g.securityScope)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		g.logDecision(command, "llm", false, fmt.Sprintf("error: %v", err), durationMs, 0, 0)
		event(ctx, "llm.error", attribute.String("error", err.Error()))
		return fmt.Errorf("LLM check failed: %v", err)
	}
	g.logDecision(command, "llm", result.Allowed, result.Reason, durationMs, result.InputTokens, result.OutputTokens)
	if !result.Allowed {
		event(ctx, "llm.blocked", attribute.String("reason", result.Reason))
		return fmt.Errorf("blocked: %s", result.Reason)
	}
	event(ctx, "llm.allowed")
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
