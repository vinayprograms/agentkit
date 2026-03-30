# contentguard

Content trust verification for agent tool calls. Protects against prompt injection by tracking untrusted content and verifying tool calls through a tiered security pipeline.

## Usage

```go
guard, err := contentguard.New(contentguard.Config{
    Mode:     contentguard.Default,
    Screener: contentguard.LLMScreener(cheapModel, ""),
    Reviewer: contentguard.LLMReviewer(capableModel, contentguard.Default, ""),
}, sessionID)
defer guard.Close()

// Track content as it enters the system
guard.Ingest(contentguard.Untrusted, contentguard.Data, true, html, "web_fetch")

// Before executing a tool call, check if it's safe
result, err := guard.Check(ctx, "bash", args, originalGoal)
if !result.Allowed {
    fmt.Println("Blocked:", result.DenyReason)
}
```

## How It Works

The guard runs a 3-tier pipeline on every high-risk tool call:

1. **Tier 1 (deterministic)** — checks if untrusted content exists in context + pattern/keyword/encoding detection
2. **Tier 2 (screener)** — quick LLM check: "is this tool call influenced by untrusted content?" Skipped in paranoid mode.
3. **Tier 3 (reviewer)** — full LLM review with ALLOW/DENY/MODIFY verdict

Low-risk tools skip the pipeline. If no untrusted content is tracked, all tools pass.

## Modes

| Mode | Behavior |
|---|---|
| `Default` | Full 3-tier pipeline |
| `Paranoid` | Skips Tier 2, goes straight to Tier 3 |
| `Research` | Adds security research scope context to LLM prompts |

## Pluggable Screener and Reviewer

`ScreenFunc` and `ReviewFunc` are function types — swap in any implementation:

```go
// LLM-backed (default)
contentguard.LLMScreener(model, scope)
contentguard.LLMReviewer(model, mode, scope)

// Custom (rule engine, human approval, test stub)
customScreener := func(ctx context.Context, req contentguard.ScreenRequest) (*contentguard.ScreenResult, error) {
    return &contentguard.ScreenResult{Suspicious: false}, nil
}
```

## Trust Levels

| Level | Meaning |
|---|---|
| `Trusted` | Framework-generated (system prompts) |
| `Vetted` | Human-authored (goals, signed packages) |
| `Untrusted` | External content (tool results, web fetches) |

## Content Kinds

| Kind | Meaning |
|---|---|
| `Instruction` | Executable instructions |
| `Data` | Data content (untrusted content is always Data) |
