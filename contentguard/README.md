# contentguard

Content trust verification for agent tool calls. Protects against prompt injection by tracking content with trust metadata and verifying tool calls through a staged pipeline.

## Usage

```go
guard, err := contentguard.New(
    []contentguard.Stage{
        contentguard.NewScreener(cheapModel),
        contentguard.NewReviewer(capableModel),
    },
    contentguard.Escalatory(),
    contentguard.Config{
        Context:  map[string]string{"scope": "authorized pentest of lab network"},
        Patterns: []string{"exfil:send.*external"},
        Keywords: []string{"custom_secret"},
        Skip:     []string{"read", "list_files"},
    },
)
defer guard.Close()

// Track content as it enters the system
guard.Ingest(contentguard.Untrusted, contentguard.Data, true, html, "web_fetch")

// Track derived content with lineage
guard.IngestWithLineage(contentguard.Untrusted, contentguard.Data, true, derived, "llm:response", []string{parentID})

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

## Context

Context flows from the guard into every stage's `Request.Context`:

```go
contentguard.New(stages, workflow, contentguard.Config{
    Context: map[string]string{"scope": "authorized pentest"},
})
```

Stages read context to adjust behavior (e.g., research scope modifies LLM prompts).

## Verdicts

| Verdict | Meaning |
|---|---|
| `Allow` | Tool call is safe |
| `Deny` | Tool call is blocked |
| `Modify` | Tool call needs changes (rationale has the suggestion) |
| `Escalate` | Stage can't decide, pass to next (only in findings, never in final result) |

## Trust

| Level | Meaning |
|---|---|
| `Trusted` | Framework-generated (system prompts) |
| `Vetted` | Human-authored (goals) |
| `Untrusted` | External content (web fetches, tool results) |
