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
	thinking           llm.ThinkingLevel
	llmTimeout         time.Duration

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
	// Thinking enables reasoning in the LLM stage. Default (false) keeps
	// thinking off, which is right for the short, shaped commands most
	// callers send. Enable it when the commands under review are long and
	// compound — pipes, chained operators, subshells — where spotting a
	// side door is a reasoning problem and a snap verdict is a weak one.
	// Reasoning costs latency, so pair it with a Timeout you have measured.
	Thinking bool
	// Timeout bounds the LLM stage. Zero means no deadline.
	//
	// The LLM stage is an enhancement over the deterministic stage, not a
	// gate in front of it: it only runs on commands the deterministic rules
	// have already allowed. So a stage that cannot answer in time must not
	// be able to stall or fail the caller — on timeout the gate falls back
	// to the deterministic verdict it already has, and records the decision
	// as degraded. Without a deadline a slow model blocks the caller for as
	// long as it takes, which is an availability failure in a component
	// whose whole job is to be a fast pre-flight check.
	Timeout time.Duration
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
		thinking:           thinkingLevel(cfg.Thinking),
		llmTimeout:         cfg.Timeout,
	}
}

// thinkingLevel maps the Config bool onto an llm.ThinkingLevel. The bool is
// the API because "should the reviewer reason about this" is the question a
// policy author is answering; the levels below it are a model concern.
func thinkingLevel(on bool) llm.ThinkingLevel {
	if on {
		return llm.ThinkingMedium
	}
	return llm.ThinkingOff
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

	// Step 1.5: deterministic path pre-check. This can ONLY conclude
	// "provably in bounds, skip the LLM" — it never blocks. Anything not
	// provably safe falls through to the LLM exactly as before this
	// existed. See pathprecheck.go for the full boundary this enforces.
	if skip, reason := g.pathPrecheck(command); skip {
		g.logDecision(command, "path-precheck", true, reason, 0, 0, 0)
		event(ctx, "path-precheck.skipped-llm")
		return nil
	}

	// Step 2: LLM analysis (if model configured).
	// allowedDirs is context for the LLM prompt, not a gate for running it.
	if g.model == nil {
		event(ctx, "llm.skipped")
		return nil
	}

	event(ctx, "llm.started")
	llmCtx := ctx
	if g.llmTimeout > 0 {
		var cancel context.CancelFunc
		llmCtx, cancel = context.WithTimeout(ctx, g.llmTimeout)
		defer cancel()
	}
	start := time.Now()
	result, err := llmCheck(llmCtx, g.model, command, g.allowedDirs, g.userDeniedCommands, g.disabledTools, g.workspace, g.securityScope, g.thinking)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		// The reviewer failed to produce a verdict. It is not a verdict.
		//
		// Fail-closed here would convert a model hiccup, a provider outage
		// or a slow reviewer into a hard block on a command the
		// deterministic stage has already allowed — the LLM stage only runs
		// after that stage passes. So fall back to the verdict already in
		// hand and record the decision as degraded, under a step name that
		// distinguishes "the check could not run" from "the check ran and
		// denied". Matches llmCheck's existing empty-response fallback.
		step := "llm-error"
		if llmCtx.Err() != nil && ctx.Err() == nil {
			// Our deadline, not the caller's cancellation.
			step = "llm-timeout"
		}
		g.logDecision(command, step, true, fmt.Sprintf("LLM stage unavailable (%v); falling back to deterministic ALLOW", err), durationMs, 0, 0)
		event(ctx, "llm.degraded", attribute.String("step", step), attribute.String("error", err.Error()))
		return nil
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

// LLMSettings reports the LLM stage's configured thinking level and
// deadline. Exported so a consumer can assert its policy actually reached
// the gate: both settings are silent at runtime — thinking changes only
// latency and verdict quality, and a deadline that never fires is
// indistinguishable from one that was never set — so a mis-wiring would
// otherwise surface as nothing at all.
func (g *Gate) LLMSettings() (llm.ThinkingLevel, time.Duration) {
	return g.thinking, g.llmTimeout
}

// CheckDeterministic performs only the fast deterministic checks (for testing/preview).
func (g *Gate) CheckDeterministic(command string) (bool, string) {
	return g.checkDeterministic(command)
}
