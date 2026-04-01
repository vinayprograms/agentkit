package contentguard

import (
	"context"
	"fmt"
	"strings"
	"sync"

	
	"github.com/vinayprograms/agentkit/logging"
)

// Mode represents the security operation mode.
type Mode string

const (
	Default  Mode = "default"
	Paranoid Mode = "paranoid"
	Research Mode = "research"
)

// Config configures the content guard.
type Config struct {
	Mode          Mode
	ResearchScope string     // required when Mode is Research
	UserTrust     TrustLevel

	// Tiers is the ordered verification pipeline after Tier 1 (deterministic).
	// Each tier can allow, deny, or escalate to the next.
	// If empty, flagged tool calls are denied (fail-safe).
	Stages []Stage

	Logger *logging.Logger
}

// Guard implements the tiered content security pipeline.
type Guard struct {
	mode          Mode
	researchScope string
	userTrust     TrustLevel
	stages        []Stage
	audit         *AuditTrail
	logger        *logging.Logger

	taints        []*Taint
	taintsMu      sync.RWMutex
	taintCounter  int
	contentHashes map[string]string
}

// New creates a new security verifier.
func New(cfg Config, sessionID string) (*Guard, error) {
	audit, err := NewAuditTrail(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to create audit trail: %w", err)
	}

	logger := cfg.Logger
	if logger == nil {
		logger = logging.New().WithComponent("security")
	}

	g := &Guard{
		mode:          cfg.Mode,
		researchScope: cfg.ResearchScope,
		userTrust:     cfg.UserTrust,
		stages:        cfg.Stages,
		audit:         audit,
		logger:        logger,
		taints:        make([]*Taint, 0),
		contentHashes: make(map[string]string),
	}

	logger.Info("security verifier initialized", map[string]interface{}{
		"mode":           string(cfg.Mode),
		"research_scope": cfg.ResearchScope,
		"user_trust":     string(cfg.UserTrust),
		"session_id":     sessionID,
	})

	return g, nil
}

// HighRiskTools is the set of tools that require extra scrutiny.
var HighRiskTools = map[string]bool{
	"bash":        true,
	"write":       true,
	"web_fetch":   true,
	"spawn_agent": true,
}

// Check runs the tiered verification pipeline for a tool call.
// agentContext filters taints to only those from the same agent (empty = all taints).
func (g *Guard) Check(ctx context.Context, toolName string, args map[string]interface{}, originalGoal, agentContext string) (*Result, error) {
	result := &Result{
		Allowed:  false,
		ToolName: toolName,
	}

	// Tier 1: Deterministic checks (always runs, built-in)
	tier1Result := g.tier1Check(toolName, args, agentContext)
	result.Tier1 = tier1Result

	if len(tier1Result.RelatedBlocks) > 0 {
		result.TaintLineage = g.TaintLineageFor(tier1Result.RelatedBlocks)
	}

	if tier1Result.Pass {
		result.Allowed = true
		g.recordDecision(tier1Result.Taint, "pass", "skipped", "skipped")
		return result, nil
	}

	// Run configurable tiers in order
	if len(g.stages) == 0 {
		// No tiers configured — fail-safe deny
		result.DenyReason = "no verification tiers configured, denying high-risk action"
		g.recordDecision(tier1Result.Taint, "escalate", "denied:no_stages", "")
		return result, nil
	}

	req := Request{
		ToolName:     toolName,
		ToolArgs:     args,
		Taints:       g.getUntrustedTaints(),
		OriginalGoal: originalGoal,
		PriorReasons: tier1Result.Reasons,
	}

	for i, stage := range g.stages {
		stageResult, err := stage.Evaluate(ctx, req)
		if err != nil {
			result.DenyReason = fmt.Sprintf("stage %d error: %v", i+2, err)
			g.recordDecision(tier1Result.Taint, "escalate", fmt.Sprintf("stage%d:error", i+2), "")
			return result, nil
		}

		result.Responses = append(result.Responses, stageResult)

		if g.mode == Paranoid {
			// Paranoid: run ALL stages regardless. Deny if ANY stage denies.
			if !stageResult.Allowed && !stageResult.Escalate {
				result.DenyReason = stageResult.Reason
				result.Modification = stageResult.Correction
				g.recordDecision(tier1Result.Taint, "paranoid", fmt.Sprintf("stage%d:%s", i+2, stageResult.Verdict), "")
				return result, nil
			}
			req.PriorReasons = append(req.PriorReasons, stageResult.Reason)
			continue
		}

		// Default: stop on allow or deny; continue only on escalate.
		if stageResult.Allowed {
			result.Allowed = true
			g.recordDecision(tier1Result.Taint, "escalate", fmt.Sprintf("stage%d:allow", i+2), "")
			return result, nil
		}

		if !stageResult.Escalate {
			result.DenyReason = stageResult.Reason
			result.Modification = stageResult.Correction
			g.recordDecision(tier1Result.Taint, "escalate", fmt.Sprintf("stage%d:%s", i+2, stageResult.Verdict), "")
			return result, nil
		}

		req.PriorReasons = append(req.PriorReasons, stageResult.Reason)
	}

	if g.mode == Paranoid {
		// All stages passed in paranoid mode — allow
		result.Allowed = true
		g.recordDecision(tier1Result.Taint, "paranoid", "all_stages_passed", "")
		return result, nil
	}

	// Default mode: all stages escalated with no final decision — fail-safe deny
	result.DenyReason = "all stages escalated without verdict, denying"
	g.recordDecision(tier1Result.Taint, "escalate", "exhausted", "")
	return result, nil
}

// Tier1Result holds the result of deterministic checks.
type Tier1Result struct {
	Pass          bool
	Reasons       []string
	SkipReason    string   // Why escalation was skipped (for forensic clarity)
	Taint         *Taint   // The primary untrusted taint that triggered escalation
	RelatedBlocks []*Taint // All taints whose content is used in this tool call
}

func (g *Guard) tier1Check(toolName string, args map[string]interface{}, agentContext string) *Tier1Result {
	result := &Tier1Result{Pass: true}

	// Check 1: Any untrusted content in context?
	// Filter by agent context if specified
	untrustedBlocks := g.getUntrustedTaintsForContext(agentContext)
	if len(untrustedBlocks) == 0 {
		result.SkipReason = "no_untrusted_content"
		return result // No untrusted content - pass
	}

	// Check 2: Is this a high-risk tool?
	isHighRisk := HighRiskTools[toolName]
	if !isHighRisk {
		result.SkipReason = fmt.Sprintf("low_risk_tool:%s", toolName)
		return result // Low-risk tool - pass
	}

	result.Pass = false
	result.Reasons = append(result.Reasons, "high_risk_tool:"+toolName)

	// Serialize args for pattern matching
	argsStr := fmt.Sprintf("%v", args)

	// Check 3: Find ALL taints whose content is being used in this tool call
	// This is simple taint tracking - check if args contain data from any taint
	var relevantBlocks []*Taint
	for _, taint := range untrustedBlocks {
		if g.argsContainBlockData(argsStr, taint) {
			relevantBlocks = append(relevantBlocks, taint)
		}
	}

	// Check 4: Suspicious patterns in relevant taints (or all if none found)
	blocksToCheck := untrustedBlocks
	if len(relevantBlocks) > 0 {
		blocksToCheck = relevantBlocks
		result.Taint = relevantBlocks[0] // Primary taint for reporting
		result.RelatedBlocks = relevantBlocks // All contributing taints
	}

	// Collect pattern findings but don't let patterns override taint selection
	// The taint should be selected based on content correlation, not pattern presence
	// Use a set to deduplicate reasons across multiple taints
	seenReasons := make(map[string]bool)
	
	for _, taint := range blocksToCheck {
		// Check for injection patterns (regex-based)
		patterns := DetectSuspiciousPatterns(taint.Content)
		for _, p := range patterns {
			reason := "pattern:" + p.Name
			if !seenReasons[reason] {
				seenReasons[reason] = true
				result.Reasons = append(result.Reasons, reason)
			}
		}

		// Check for sensitive keywords (simple word match)
		keywords := DetectSensitiveKeywords(taint.Content)
		for _, kw := range keywords {
			reason := "keyword:" + kw.Keyword
			if !seenReasons[reason] {
				seenReasons[reason] = true
				result.Reasons = append(result.Reasons, reason)
			}
		}

		// Check 5: Encoded content
		if HasEncodedContent(taint.Content) {
			if !seenReasons["encoded_content"] {
				seenReasons["encoded_content"] = true
				result.Reasons = append(result.Reasons, "encoded_content")
			}
		}
	}

	// Check args for suspicious patterns
	if HasSuspiciousPatterns(argsStr) {
		if !seenReasons["suspicious_args"] {
			seenReasons["suspicious_args"] = true
			result.Reasons = append(result.Reasons, "suspicious_args")
		}
	}

	// If still no taint identified, use the most recent one (not the first)
	if result.Taint == nil && len(untrustedBlocks) > 0 {
		result.Taint = untrustedBlocks[len(untrustedBlocks)-1]
	}

	return result
}

// argsContainBlockData checks if tool arguments contain data from a taint.
// This is a simple substring match - full taint tracking would be more sophisticated.
func (g *Guard) argsContainBlockData(argsStr string, taint *Taint) bool {
	// Extract meaningful substrings from taint content to check
	// For URLs, check if any URL from the taint appears in args
	urls := extractURLs(taint.Content)
	for _, url := range urls {
		if len(url) > 20 && containsIgnoreCase(argsStr, url) {
			return true
		}
	}

	// For other content, check if significant portions appear
	// (Skip very short content to avoid false positives)
	if len(taint.Content) > 100 {
		// Check if a meaningful chunk of taint content appears in args
		// Use a sliding window of 50 chars
		for i := 0; i+50 <= len(taint.Content) && i < 500; i += 25 {
			chunk := taint.Content[i : i+50]
			if containsIgnoreCase(argsStr, chunk) {
				return true
			}
		}
	}

	return false
}

// extractURLs extracts URLs from content.
func extractURLs(content string) []string {
	var urls []string
	// Simple URL extraction - look for http:// or https://
	words := strings.Fields(content)
	for _, word := range words {
		// Clean up common JSON/markdown artifacts
		word = strings.Trim(word, `"',[]{}()`)
		if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
			// Truncate at common terminators
			for _, term := range []string{`"`, `'`, `\u003c`, `>`, ` `, `\n`} {
				if idx := strings.Index(word, term); idx > 0 {
					word = word[:idx]
				}
			}
			urls = append(urls, word)
		}
	}
	return urls
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}


func (g *Guard) recordDecision(taint *Taint, tier1, tier2, tier3 string) {
	if taint == nil {
		return
	}
	g.audit.RecordDecision(taint, tier1, tier2, tier3)
}

// Result holds the complete verification result.
type Result struct {
	Allowed      bool
	ToolName     string
	DenyReason   string
	Modification string
	Tier1        *Tier1Result
	Responses  []*Response       // results from configurable tiers (in order)
	TaintLineage []*TaintLineageNode // taint dependency tree for related taints
}

// AuditTrail returns the audit trail for export.
func (g *Guard) AuditTrail() *AuditTrail {
	return g.audit
}

// Destroy cleans up resources, including zeroing the private key.
func (g *Guard) Close() {
	if g.audit != nil {
		g.audit.Close()
	}
}

// ClearContext removes all taints from the context.
func (g *Guard) ClearContext() {
	g.taintsMu.Lock()
	defer g.taintsMu.Unlock()
	g.taints = make([]*Taint, 0)
}

