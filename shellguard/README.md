# shellguard

Shell command security gate. Implements `tools.Guard` so it drops directly into a tool registry entry via `.With(gate)`. Commands pass through two stages: fast deterministic checks (banned commands, dangerous patterns, pipe chains) followed by optional LLM-based analysis for deeper intent evaluation.

## Usage

```go
gate := shellguard.New(
    shellguard.BashShell{},
    "/workspace",                          // workspace root
    []string{"/workspace", "/tmp"},        // allowed directories
    []string{"rm -rf", "shutdown"},        // additional denied commands
    model,                                 // nil = deterministic only
    "",                                    // security scope ("" = normal mode)
)

reg := tools.NewRegistry()
reg.Register(tools.New(tools.Bash("/workspace")).With(gate))
```

## Deterministic checks

Applied on every command regardless of whether an LLM model is configured:
- Banned command list (hardcoded dangerous commands + `userDeniedCommands`)
- Path traversal outside `allowedDirs`
- Pipe chains that include blocked commands
- Shell metacharacter injection patterns

## LLM analysis

Runs when `model != nil`. The model evaluates intent and context for commands that pass deterministic checks. `allowedDirs` is fed into the prompt as context — an empty list means the LLM reasons about the command without directory constraints, not that the check is skipped. Skip this tier by passing `nil` for `model` in trusted environments where deterministic checks are sufficient.

## Security scope

Pass a non-empty `securityScope` to enable research/pentest mode: the LLM reviewer receives the scope string and permits operations within the declared boundary that would otherwise be flagged.

## Auditing

```go
gate.OnDecision = func(command, step string, allowed bool, reason string,
    durationMs int64, inputTokens, outputTokens int) {
    log.Printf("shellguard: %s → %v (%s)", command, allowed, reason)
}
```
