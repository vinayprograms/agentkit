package contentguard

import "context"

// Verdict is the outcome of a stage evaluation or the guard's final decision.
type Verdict string

const (
	Allow    Verdict = "allow"
	Deny     Verdict = "deny"
	Modify   Verdict = "modify"
	Escalate Verdict = "escalate" // only in Finding, never in Result
)

// Stage is one step in the verification pipeline.
type Stage interface {
	Evaluate(ctx context.Context, req Request) (*Finding, error)
}

// Finding is what one stage concluded about a tool call.
type Finding struct {
	Verdict   Verdict
	Rationale string // why (deny), what instead (modify), why unsure (escalate)
	Source    string // which stage produced this
}

// Request carries all information stages need to make a decision.
type Request struct {
	ToolName      string
	ToolArgs      map[string]any
	Untrusted     []*Content
	OriginalGoal  string
	PriorFindings []*Finding        // what earlier stages found
	Context       map[string]string // guard-level context (e.g., research scope)
}

// Result is the guard's final answer on a tool call.
type Result struct {
	Verdict      Verdict
	Rationale    string
	ToolName     string
	Findings []*Finding // all findings, deterministic first
}
