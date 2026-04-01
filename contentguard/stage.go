package contentguard

import "context"

// Tier is a stage in the content guard verification pipeline.
// Each tier evaluates a tool call and decides to allow, deny, or escalate.
type Stage interface {
	Evaluate(ctx context.Context, req Request) (*Response, error)
}

// Request carries all information a tier needs to make a decision.
type Request struct {
	ToolName     string
	ToolArgs     map[string]any
	Taints       []*Taint // all untrusted taints in context
	OriginalGoal string
	PriorReasons []string // accumulated reasons from tier1 + earlier tiers
}

// Response is the outcome of a tier evaluation.
type Response struct {
	Allowed    bool    // tool call is safe — stop pipeline
	Escalate   bool    // pass to next tier for deeper analysis
	Reason     string  // explanation (for deny/escalate)
	Verdict    Verdict // for audit trail
	Correction string  // suggested safer alternative (for modify verdicts)

	// Token usage (for LLM-backed tiers)
	InputTokens  int
	OutputTokens int
	LatencyMs    int64
}
