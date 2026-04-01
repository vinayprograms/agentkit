package contentguard

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/vinayprograms/agentkit/logging"
)

// Guard verifies tool calls against ingested content through a staged pipeline.
type Guard struct {
	stages     []Stage
	workflow   Workflow
	exceptions map[string]string
	audit      *AuditTrail
	logger     *logging.Logger

	taints        []*Taint
	taintsMu      sync.RWMutex
	taintCounter  int
	contentHashes map[string]string
}

// New creates a content guard.
func New(stages []Stage, workflow Workflow, exceptions map[string]string, sessionID string) (*Guard, error) {
	audit, err := NewAuditTrail(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to create audit trail: %w", err)
	}

	logger := logging.New().WithComponent("contentguard")

	logger.Info("content guard initialized", map[string]interface{}{
		"stages":     len(stages),
		"exceptions": len(exceptions),
		"session_id": sessionID,
	})

	return &Guard{
		stages:        stages,
		workflow:      workflow,
		exceptions:    exceptions,
		audit:         audit,
		logger:        logger,
		taints:        make([]*Taint, 0),
		contentHashes: make(map[string]string),
	}, nil
}

// HighRiskTools is the set of tools that require verification.
var HighRiskTools = map[string]bool{
	"bash":        true,
	"write":       true,
	"web_fetch":   true,
	"spawn_agent": true,
}

// Check runs the verification pipeline for a tool call.
func (g *Guard) Check(ctx context.Context, toolName string, args map[string]any, originalGoal string) (*Result, error) {
	// Step 1: deterministic checks (built-in, non-optional)
	deterministicFinding := g.deterministicCheck(toolName, args)

	if deterministicFinding.Verdict == Allow {
		return &Result{
			Verdict:  Allow,
			ToolName: toolName,
			Findings: []*Finding{deterministicFinding},
		}, nil
	}

	// Step 2: delegate to workflow
	if len(g.stages) == 0 {
		return &Result{
			Verdict:   Deny,
			Rationale: "no verification stages configured",
			ToolName:  toolName,
			Findings:  []*Finding{deterministicFinding},
		}, nil
	}

	req := Request{
		ToolName:      toolName,
		ToolArgs:      args,
		Taints:        g.getUntrustedTaints(),
		OriginalGoal:  originalGoal,
		PriorFindings: []*Finding{deterministicFinding},
	}

	result := g.workflow.Execute(ctx, g.stages, req, g.exceptions)

	// Prepend deterministic finding
	result.Findings = append([]*Finding{deterministicFinding}, result.Findings...)
	result.ToolName = toolName

	// Build taint lineage
	untrusted := g.getUntrustedTaints()
	if len(untrusted) > 0 {
		result.TaintLineage = g.TaintLineageFor(untrusted)
	}

	return result, nil
}

// deterministicCheck performs fast pattern-based checks.
func (g *Guard) deterministicCheck(toolName string, args map[string]any) *Finding {
	untrusted := g.getUntrustedTaints()
	if len(untrusted) == 0 {
		return &Finding{Verdict: Allow, Rationale: "no untrusted content", Source: "deterministic"}
	}

	if !HighRiskTools[toolName] {
		return &Finding{Verdict: Allow, Rationale: fmt.Sprintf("low-risk tool: %s", toolName), Source: "deterministic"}
	}

	// High-risk tool + untrusted content → check patterns
	var reasons []string
	reasons = append(reasons, "high_risk_tool:"+toolName)

	argsStr := fmt.Sprintf("%v", args)

	for _, taint := range untrusted {
		for _, p := range DetectSuspiciousPatterns(taint.Content) {
			reasons = append(reasons, "pattern:"+p.Name)
		}
		for _, kw := range DetectSensitiveKeywords(taint.Content) {
			reasons = append(reasons, "keyword:"+kw.Keyword)
		}
		if HasEncodedContent(taint.Content) {
			reasons = append(reasons, "encoded_content")
		}
	}

	if HasSuspiciousPatterns(argsStr) {
		reasons = append(reasons, "suspicious_args")
	}

	return &Finding{
		Verdict:   Escalate,
		Rationale: strings.Join(reasons, ", "),
		Source:    "deterministic",
	}
}

// AuditTrail returns the audit trail for export.
func (g *Guard) AuditTrail() *AuditTrail {
	return g.audit
}

// Close cleans up resources.
func (g *Guard) Close() {
	if g.audit != nil {
		g.audit.Close()
	}
}

// ClearContext removes all taints.
func (g *Guard) ClearContext() {
	g.taintsMu.Lock()
	defer g.taintsMu.Unlock()
	g.taints = make([]*Taint, 0)
	g.contentHashes = make(map[string]string)
}

// Utility functions

func extractURLs(content string) []string {
	var urls []string
	words := strings.Fields(content)
	for _, word := range words {
		word = strings.Trim(word, `"',[]{}()`)
		if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
			for _, term := range []string{`"`, `'`, `>`, ` `, `\n`} {
				if idx := strings.Index(word, term); idx > 0 {
					word = word[:idx]
				}
			}
			urls = append(urls, word)
		}
	}
	return urls
}

func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
