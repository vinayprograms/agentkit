package contentguard

import (
	"context"
	"fmt"
)

// Workflow defines how stages are executed in the verification pipeline.
type Workflow interface {
	Execute(ctx context.Context, stages []Stage, req Request) *Result
}

// Escalatory returns a Workflow that stops on the first allow/deny/modify verdict.
// Only escalate passes to the next stage. If all stages escalate, fail-safe deny.
func Escalatory() Workflow { return escalatory{} }

// Paranoid returns a Workflow that runs ALL stages regardless of individual verdicts.
// Deny if ANY stage denies. Allow only if ALL stages allow or escalate.
func Paranoid() Workflow { return paranoid{} }

type escalatory struct{}

func (escalatory) Execute(ctx context.Context, stages []Stage, req Request) *Result {
	result := &Result{Verdict: Deny, ToolName: req.ToolName}

	for _, stage := range stages {
		finding, err := stage.Evaluate(ctx, req)
		if err != nil {
			result.Rationale = fmt.Sprintf("stage error: %v", err)
			result.Findings = append(result.Findings, &Finding{
				Verdict:   Deny,
				Rationale: result.Rationale,
				Source:    "error",
			})
			return result
		}

		result.Findings = append(result.Findings, finding)

		switch finding.Verdict {
		case Allow:
			result.Verdict = Allow
			result.Rationale = finding.Rationale
			return result
		case Deny:
			result.Verdict = Deny
			result.Rationale = finding.Rationale
			return result
		case Modify:
			result.Verdict = Modify
			result.Rationale = finding.Rationale
			return result
		case Escalate:
			req.PriorFindings = append(req.PriorFindings, finding)
			continue
		}
	}

	// All stages escalated — fail-safe deny
	result.Rationale = "all stages escalated without verdict"
	return result
}

type paranoid struct{}

func (paranoid) Execute(ctx context.Context, stages []Stage, req Request) *Result {
	result := &Result{Verdict: Allow, ToolName: req.ToolName}

	for _, stage := range stages {
		finding, err := stage.Evaluate(ctx, req)
		if err != nil {
			result.Verdict = Deny
			result.Rationale = fmt.Sprintf("stage error: %v", err)
			result.Findings = append(result.Findings, &Finding{
				Verdict:   Deny,
				Rationale: result.Rationale,
				Source:    "error",
			})
			return result
		}

		result.Findings = append(result.Findings, finding)

		// Deny or Modify immediately stops — paranoid fails fast on any rejection
		if finding.Verdict == Deny || finding.Verdict == Modify {
			result.Verdict = finding.Verdict
			result.Rationale = finding.Rationale
			return result
		}

		// Allow and Escalate both continue — all stages must run
		req.PriorFindings = append(req.PriorFindings, finding)
	}

	// All stages passed — allow
	result.Verdict = Allow
	result.Rationale = "all stages passed"
	return result
}
