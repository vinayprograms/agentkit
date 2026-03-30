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

// Config holds security verifier configuration.
// ScreenFunc performs quick triage on a flagged tool call (Tier 2).
// Return true if suspicious (should escalate to review).
type ScreenFunc func(ctx context.Context, req ScreenRequest) (*ScreenResult, error)

// ReviewFunc performs full review on an escalated tool call (Tier 3).
type ReviewFunc func(ctx context.Context, req ReviewRequest) (*ReviewResult, error)

// Config configures the content guard.
type Config struct {
	Mode          Mode
	ResearchScope string     // required when Mode is Research
	UserTrust     TrustLevel

	// Screener is the Tier 2 quick check. Nil disables Tier 2.
	Screener ScreenFunc

	// Reviewer is the Tier 3 full verdict. Nil means escalated calls are denied.
	Reviewer ReviewFunc

	Logger *logging.Logger
}

// Guard implements the tiered content security pipeline.
type Guard struct {
	mode          Mode
	researchScope string
	userTrust     TrustLevel
	screener      ScreenFunc
	reviewer      ReviewFunc
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
		screener:      cfg.Screener,
		reviewer:      cfg.Reviewer,
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

// AddBlock adds a content taint to the context.
func (g *Guard) AddBlock(trust TrustLevel, typ ContentKind, mutable bool, content, source string) *Taint {
	return g.addBlockInternal(trust, typ, mutable, content, source, "", 0, nil)
}

// AddBlockWithContext adds a content taint with an agent context identifier.
func (g *Guard) AddBlockWithContext(trust TrustLevel, typ ContentKind, mutable bool, content, source, agentContext string) *Taint {
	return g.addBlockInternal(trust, typ, mutable, content, source, agentContext, 0, nil)
}

// AddBlockWithTaint adds a content taint with explicit taint lineage.
// eventSeq is the session event sequence when this taint was created.
// taintedBy lists IDs of taints that influenced this taint.
func (g *Guard) AddBlockWithTaint(trust TrustLevel, typ ContentKind, mutable bool, content, source, agentContext string, eventSeq uint64, taintedBy []string) *Taint {
	return g.addBlockInternal(trust, typ, mutable, content, source, agentContext, eventSeq, taintedBy)
}

func (g *Guard) addBlockInternal(trust TrustLevel, typ ContentKind, mutable bool, content, source, agentContext string, eventSeq uint64, taintedBy []string) *Taint {
	g.taintsMu.Lock()
	defer g.taintsMu.Unlock()

	// Compute content hash for de-duplication
	contentHash := computeHash(content)

	// Check if we've seen this exact content before (de-duplication)
	// Only de-dupe untrusted content (trusted/vetted content doesn't need security checks)
	if trust == Untrusted {
		if existingID, exists := g.contentHashes[contentHash]; exists {
			// Find the existing taint
			for _, b := range g.taints {
				if b.ID == existingID {
					// Log de-duplication hit
					if g.logger != nil {
						g.logger.Debug("content deduplicated", map[string]interface{}{
							"hash":         contentHash[:16] + "...",
							"existing_id":  existingID,
							"source":       source,
						})
					}
					// Return a new taint that references the existing content
					// but with the new source/taint info (for accurate taint tracking)
					g.taintCounter++
					id := fmt.Sprintf("b%04d", g.taintCounter)
					taint := newTaint(id, trust, typ, mutable, content, source)
					taint.AgentContext = agentContext
					taint.CreatedAtSeq = eventSeq
					taint.TaintedBy = taintedBy
					taint.DedupeHit = true
					// Link to original taint for verification skip
					taint.TaintedBy = append(taint.TaintedBy, existingID)
					g.taints = append(g.taints, taint)
					return taint
				}
			}
		}
	}

	// New content - create taint and register hash
	g.taintCounter++
	id := fmt.Sprintf("b%04d", g.taintCounter)

	taint := newTaint(id, trust, typ, mutable, content, source)
	taint.AgentContext = agentContext
	taint.CreatedAtSeq = eventSeq
	taint.TaintedBy = taintedBy
	g.taints = append(g.taints, taint)

	// Register content hash for untrusted content
	if trust == Untrusted {
		g.contentHashes[contentHash] = id
	}

	return taint
}

// GetCurrentUntrustedBlockIDs returns IDs of all untrusted taints in context.
// Used to mark LLM responses as tainted by these taints.
func (g *Guard) GetCurrentUntrustedBlockIDs() []string {
	g.taintsMu.RLock()
	defer g.taintsMu.RUnlock()

	var ids []string
	for _, b := range g.taints {
		if b.Trust == Untrusted {
			ids = append(ids, b.ID)
		}
	}
	return ids
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

	// Tier 1: Deterministic checks
	tier1Result := g.tier1Check(toolName, args, agentContext)
	result.Tier1 = tier1Result

	// Build taint lineage for all related taints
	if len(tier1Result.RelatedBlocks) > 0 {
		result.TaintLineage = g.GetTaintLineageForBlocks(tier1Result.RelatedBlocks)
	}

	if tier1Result.Pass {
		// No untrusted content or low-risk tool - allow
		result.Allowed = true
		g.recordDecision(tier1Result.Taint, "pass", "skipped", "skipped")
		return result, nil
	}

	// Tier 2: Cheap model triage (skip in paranoid mode - go straight to T3)
	if g.mode != Paranoid && g.screener != nil {
		tier2Result, err := g.tier2Check(ctx, toolName, args, tier1Result.Taint)
		if err == nil {
			result.Tier2 = tier2Result

			if !tier2Result.Suspicious {
				// Triage cleared
				result.Allowed = true
				g.recordDecision(tier1Result.Taint, "escalate", "pass", "skipped")
				return result, nil
			}
		}
		// Continue to tier 3 on error
	}

	// Tier 3: Full supervisor
	if g.reviewer == nil {
		// No supervisor configured - fail-safe deny
		result.Allowed = false
		result.DenyReason = "no security supervisor configured, denying high-risk action"
		g.recordDecision(tier1Result.Taint, "escalate", "escalate", "denied:no_supervisor")
		return result, nil
	}

	tier3Result, err := g.tier3Check(ctx, toolName, args, originalGoal, tier1Result)
	if err != nil {
		result.Allowed = false
		result.DenyReason = fmt.Sprintf("tier 3 error: %v", err)
		g.recordDecision(tier1Result.Taint, "escalate", "escalate", "error")
		return result, nil
	}

	result.Tier3 = tier3Result
	tier3Log := string(tier3Result.Verdict)

	switch tier3Result.Verdict {
	case VerdictAllow:
		result.Allowed = true
	case VerdictDeny:
		result.Allowed = false
		result.DenyReason = tier3Result.Reason
	case VerdictModify:
		result.Allowed = false
		result.DenyReason = tier3Result.Reason
		result.Modification = tier3Result.Correction
	}

	g.recordDecision(tier1Result.Taint, "escalate", "escalate", tier3Log)

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

func (g *Guard) tier2Check(ctx context.Context, toolName string, args map[string]interface{}, taint *Taint) (*ScreenResult, error) {
	return g.screener(ctx, ScreenRequest{
		ToolName:       toolName,
		ToolArgs:       args,
		UntrustedBlock: taint,
	})
}

func (g *Guard) tier3Check(ctx context.Context, toolName string, args map[string]interface{}, originalGoal string, tier1 *Tier1Result) (*ReviewResult, error) {
	tier2Reason := "skipped"
	if g.mode == Paranoid {
		tier2Reason = "paranoid mode"
	}

	return g.reviewer(ctx, ReviewRequest{
		ToolName:        toolName,
		ToolArgs:        args,
		UntrustedTaints: g.getUntrustedTaints(),
		OriginalGoal:    originalGoal,
		Tier1Flags:      tier1.Reasons,
		Tier2Reason:     tier2Reason,
	})
}

func (g *Guard) getUntrustedTaints() []*Taint {
	return g.getUntrustedTaintsForContext("")
}

// getUntrustedTaintsForContext returns untrusted taints filtered by agent context.
// If agentContext is empty, returns all untrusted taints.
// If agentContext is set, returns taints matching that context OR taints with no context (shared taints).
func (g *Guard) getUntrustedTaintsForContext(agentContext string) []*Taint {
	g.taintsMu.RLock()
	defer g.taintsMu.RUnlock()

	var untrusted []*Taint
	for _, b := range g.taints {
		if b.Trust != Untrusted {
			continue
		}
		// If no filter, include all
		if agentContext == "" {
			untrusted = append(untrusted, b)
			continue
		}
		// Include if taint matches context OR has no context (shared)
		if b.AgentContext == agentContext || b.AgentContext == "" {
			untrusted = append(untrusted, b)
		}
	}
	return untrusted
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
	Tier2        *ScreenResult
	Tier3        *ReviewResult
	TaintLineage []*TaintLineageNode // Taint dependency tree for related taints
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

// TaintLineageNode represents a node in the taint dependency tree.
// Exported for use by session package.
type TaintLineageNode struct {
	TaintID   string              `json:"taint_id"`
	Trust     TrustLevel          `json:"trust"`
	Source    string              `json:"source"`
	EventSeq  uint64              `json:"event_seq,omitempty"`
	Depth     int                 `json:"depth,omitempty"`
	TaintedBy []*TaintLineageNode `json:"tainted_by,omitempty"`
}

// GetTaintLineage builds the full taint dependency tree for a taint.
// Returns nil if the taint is not found.
func (g *Guard) GetTaintLineage(blockID string) *TaintLineageNode {
	g.taintsMu.RLock()
	defer g.taintsMu.RUnlock()

	taint := g.findBlockByID(blockID)
	if taint == nil {
		return nil
	}

	return g.buildLineageTree(taint, 0, make(map[string]bool))
}

// GetTaintLineageForBlocks builds lineage trees for multiple taints.
func (g *Guard) GetTaintLineageForBlocks(taints []*Taint) []*TaintLineageNode {
	g.taintsMu.RLock()
	defer g.taintsMu.RUnlock()

	var lineages []*TaintLineageNode
	for _, taint := range taints {
		lineage := g.buildLineageTree(taint, 0, make(map[string]bool))
		if lineage != nil {
			lineages = append(lineages, lineage)
		}
	}
	return lineages
}

// buildLineageTree recursively builds the taint tree for a taint.
// visited prevents infinite loops from circular taint references.
func (g *Guard) buildLineageTree(taint *Taint, depth int, visited map[string]bool) *TaintLineageNode {
	if taint == nil || visited[taint.ID] {
		return nil
	}
	visited[taint.ID] = true

	node := &TaintLineageNode{
		TaintID:  taint.ID,
		Trust:    taint.Trust,
		Source:   taint.Source,
		EventSeq: taint.CreatedAtSeq,
		Depth:    depth,
	}

	// Recursively build parent lineage
	for _, parentID := range taint.TaintedBy {
		parent := g.findBlockByID(parentID)
		if parent != nil {
			parentNode := g.buildLineageTree(parent, depth+1, visited)
			if parentNode != nil {
				node.TaintedBy = append(node.TaintedBy, parentNode)
			}
		}
	}

	return node
}

// findBlockByID returns a taint by ID (caller must hold lock).
func (g *Guard) findBlockByID(id string) *Taint {
	for _, b := range g.taints {
		if b.ID == id {
			return b
		}
	}
	return nil
}

// GetBlock returns a taint by ID (thread-safe).
func (g *Guard) GetBlock(id string) *Taint {
	g.taintsMu.RLock()
	defer g.taintsMu.RUnlock()
	return g.findBlockByID(id)
}
