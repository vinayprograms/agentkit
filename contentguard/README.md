# contentguard

Content trust verification for agent tool calls. Protects against prompt injection by tracking untrusted content and verifying tool calls through a staged pipeline.

## Usage

```go
guard, err := contentguard.New(
    []contentguard.Stage{
        contentguard.NewScreener(cheapModel),
        contentguard.NewReviewer(capableModel),
    },
    contentguard.Escalatory(),
    map[string]string{"scope": "authorized pentest of lab network"},
    sessionID,
)
defer guard.Close()

// Track content as it enters the system
guard.Ingest(contentguard.Untrusted, contentguard.Data, true, html, "web_fetch")

// Verify a tool call
result, err := guard.Check(ctx, "bash", args, originalGoal)
switch result.Verdict {
case contentguard.Allow:  // proceed
case contentguard.Deny:   // blocked — result.Rationale explains why
case contentguard.Modify: // blocked — result.Rationale has the suggested alternative
}
```

## How It Works

1. **Deterministic check** (built-in, always runs) — detects untrusted content, pattern matches, keyword scanning
2. **Configurable stages** — run through the pipeline per the chosen workflow

## Workflows

| Workflow | Behavior |
|---|---|
| `Escalatory()` | Stop on first allow/deny/modify. Only escalate passes to next stage. |
| `Paranoid()` | ALL stages must run. Deny if ANY denies. Allow only if all pass. |

## Stages

Stages implement the `Stage` interface:

```go
type Stage interface {
    Evaluate(ctx context.Context, req Request) (*Finding, error)
}
```

Built-in stages:
- `NewScreener(model)` — quick LLM triage (YES/NO)
- `NewReviewer(model)` — full LLM review (ALLOW/DENY/MODIFY)

Custom stages (rule engine, human approval, etc.) implement the same interface.

## Exceptions

Exceptions flow from the guard into every stage's `Request.Exceptions`:

```go
contentguard.New(stages, workflow,
    map[string]string{"scope": "authorized pentest"},
    sessionID,
)
```

Stages read exceptions to adjust behavior (e.g., research scope modifies LLM prompts).

## Verdicts

| Verdict | Meaning |
|---|---|
| `Allow` | Tool call is safe |
| `Deny` | Tool call is blocked |
| `Modify` | Tool call needs changes (rationale has the suggestion) |
| `Escalate` | Stage can't decide, pass to next (only in findings, never in final result) |

## Trust Levels

| Level | Meaning |
|---|---|
| `Trusted` | Framework-generated (system prompts) |
| `Vetted` | Human-authored (goals) |
| `Untrusted` | External content (web fetches, tool results) |
