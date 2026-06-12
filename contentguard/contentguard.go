package contentguard

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	otrace "go.opentelemetry.io/otel/trace"
)

// Config holds optional configuration for the guard.
// Use Defaults() for zero-value config.
type Config struct {
	Context  map[string]string // flows to stages (e.g., research scope)
	Patterns []string          // custom "name:regex" injection patterns
	Keywords []string          // custom sensitive keywords
	Skip     []string          // tools that skip verification
}

// Defaults returns a zero-value Config.
func Defaults() Config { return Config{} }

// Guard verifies tool calls against ingested content through a staged pipeline.
type Guard struct {
	stages   []Stage
	workflow Workflow
	context  map[string]string

	patterns []namedPattern
	keywords []string
	skip     map[string]bool

	tracked       []*Content
	contentByID   map[string]*Content
	mu            sync.RWMutex
	contentCount  int
	contentHashes map[string]string
}

// New creates a content guard.
func New(stages []Stage, workflow Workflow, cfg Config) (*Guard, error) {
	allPatterns, err := buildPatterns(cfg.Patterns)
	if err != nil {
		return nil, fmt.Errorf("contentguard: %w", err)
	}

	skip := make(map[string]bool, len(cfg.Skip))
	for _, t := range cfg.Skip {
		skip[t] = true
	}

	return &Guard{
		stages:        stages,
		workflow:      workflow,
		context:       cfg.Context,
		patterns:      allPatterns,
		keywords:      buildKeywords(cfg.Keywords),
		skip:          skip,
		tracked:       make([]*Content, 0),
		contentByID:   make(map[string]*Content),
		contentHashes: make(map[string]string),
	}, nil
}

// Check runs the verification pipeline for a tool call.
func (g *Guard) Check(ctx context.Context, toolName string, args map[string]any, originalGoal string) (res *Result, err error) {
	ctx, end := trace(ctx, "check", attribute.String("tool.name", toolName))
	defer func() {
		if res != nil {
			otrace.SpanFromContext(ctx).SetAttributes(attribute.String("contentguard.verdict", string(res.Verdict)))
		}
		end(&err)
	}()

	// Untrusted content in scope for this call — reported on every Result so
	// consumers can propagate taint into the resulting tool-result block.
	untrusted := g.getUntrusted()
	related := relatedContent(untrusted)

	// Step 1: deterministic checks (built-in, non-optional)
	deterministicFinding := g.deterministicCheck(toolName, args)
	event(ctx, "deterministic.evaluated", attribute.String("verdict", string(deterministicFinding.Verdict)))

	if deterministicFinding.Verdict == Allow {
		event(ctx, "deterministic.short_circuit")
		return &Result{
			Verdict:  Allow,
			ToolName: toolName,
			Findings: []*Finding{deterministicFinding},
			Related:  related,
		}, nil
	}

	// Step 2: delegate to workflow
	if len(g.stages) == 0 {
		event(ctx, "workflow.no_stages")
		return &Result{
			Verdict:   Deny,
			Rationale: "no verification stages configured",
			ToolName:  toolName,
			Findings:  []*Finding{deterministicFinding},
			Related:   related,
		}, nil
	}

	req := Request{
		ToolName:      toolName,
		ToolArgs:      args,
		Untrusted:     untrusted,
		OriginalGoal:  originalGoal,
		PriorFindings: []*Finding{deterministicFinding},
		Context:       g.context,
	}

	event(ctx, "workflow.started", attribute.Int("stages.count", len(g.stages)))
	result := g.workflow.Execute(ctx, g.stages, req)
	event(ctx, "workflow.completed", attribute.String("verdict", string(result.Verdict)))

	// Prepend deterministic finding
	result.Findings = append([]*Finding{deterministicFinding}, result.Findings...)
	result.ToolName = toolName
	result.Related = related

	return result, nil
}

// relatedContent maps tracked content to the public RelatedContent view.
func relatedContent(content []*Content) []RelatedContent {
	if len(content) == 0 {
		return nil
	}
	related := make([]RelatedContent, len(content))
	for i, c := range content {
		related[i] = RelatedContent{ID: c.ID, Trust: c.Trust}
	}
	return related
}

// deterministicCheck performs fast pattern-based checks.
func (g *Guard) deterministicCheck(toolName string, args map[string]any) *Finding {
	untrusted := g.getUntrusted()
	if len(untrusted) == 0 {
		return &Finding{Verdict: Allow, Rationale: "no untrusted content", Source: "deterministic"}
	}

	if g.skip[toolName] {
		return &Finding{Verdict: Allow, Rationale: fmt.Sprintf("skipped tool: %s", toolName), Source: "deterministic"}
	}

	// Untrusted content present → check patterns
	var reasons []string
	reasons = append(reasons, "tool:"+toolName)

	argsStr := fmt.Sprintf("%v", args)

	for _, c := range untrusted {
		for _, name := range g.detectSuspiciousPatterns(c.Text) {
			reasons = append(reasons, "pattern:"+name)
		}
		for _, kw := range g.detectSensitiveKeywords(c.Text) {
			reasons = append(reasons, "keyword:"+kw)
		}
		if hasEncodedContent(c.Text) {
			reasons = append(reasons, "encoded_content")
		}
	}

	if len(g.detectSuspiciousPatterns(argsStr)) > 0 {
		reasons = append(reasons, "suspicious_args")
	}

	return &Finding{
		Verdict:   Escalate,
		Rationale: strings.Join(reasons, ", "),
		Source:    "deterministic",
	}
}

// Close cleans up resources.
func (g *Guard) Close() {
}

// ClearContext removes all tracked content.
func (g *Guard) ClearContext() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tracked = make([]*Content, 0)
	g.contentByID = make(map[string]*Content)
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
