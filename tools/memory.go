package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/vinayprograms/agentkit/types"
)

// SemanticMemory provides semantic memory operations.
type SemanticMemory interface {
	// RememberFIL stores multiple observations at once and returns their IDs.
	RememberFIL(ctx context.Context, findings, insights, lessons []string, source string) ([]string, error)
	// RetrieveByID gets a single observation by ID.
	RetrieveByID(ctx context.Context, id string) (*types.ObservationItem, error)
	// RecallFIL searches and returns categorized results.
	RecallFIL(ctx context.Context, query string, limitPerCategory int) (*types.FILResult, error)
	// Recall searches and returns flat results with scores.
	Recall(ctx context.Context, query string, limit int) ([]types.SemanticMemoryResult, error)
}

// --- Remember Tool ---

type rememberTool struct {
	memory SemanticMemory
}

// Remember creates a tool that stores observations in semantic memory.
func Remember(mem SemanticMemory) Tool {
	return &rememberTool{memory: mem}
}

func (t *rememberTool) Name() string { return "remember" }

func (t *rememberTool) Description() string {
	return `Store important discoveries in persistent knowledge base (survives across sessions).

Categories:
- findings: Facts discovered (e.g., "API rate limit is 100/min")
- insights: Conclusions/decisions (e.g., "Chose PostgreSQL for JSON support")
- lessons: Rules for future (e.g., "Always check rate limits first")

Example:
  remember({
    "findings": ["Database uses PostgreSQL", "API has 100 req/min limit"],
    "insights": ["PostgreSQL chosen for JSON support"],
    "lessons": ["Always check rate limits before integration"]
  })

Returns the count and IDs of stored observations.`
}

func (t *rememberTool) Parameters() map[string]Param {
	return map[string]Param{
		"findings": {
			Type:        ArrayParam,
			Description: "Facts discovered (raw observations)",
		},
		"insights": {
			Type:        ArrayParam,
			Description: "Conclusions drawn from findings",
		},
		"lessons": {
			Type:        ArrayParam,
			Description: "Actionable rules for future",
		},
	}
}

func (t *rememberTool) Execute(ctx context.Context, args Args) (string, error) {
	findings := args.StringSliceOr("findings", nil)
	insights := args.StringSliceOr("insights", nil)
	lessons := args.StringSliceOr("lessons", nil)

	// Filter out empty strings
	findings = filterNonEmpty(findings)
	insights = filterNonEmpty(insights)
	lessons = filterNonEmpty(lessons)

	if len(findings) == 0 && len(insights) == 0 && len(lessons) == 0 {
		return "", fmt.Errorf("at least one finding, insight, or lesson is required")
	}

	ids, err := t.memory.RememberFIL(ctx, findings, insights, lessons, "explicit")
	if err != nil {
		return "", fmt.Errorf("failed to store memories: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Stored %d observations.\n", len(ids))
	fmt.Fprintf(&sb, "IDs: %s\n", strings.Join(ids, ", "))
	sb.WriteString("Stored in persistent memory. Use recall() with relevant keywords to find later.")
	return sb.String(), nil
}

// --- Recall Tool ---

type recallTool struct {
	memory SemanticMemory
}

// Recall creates a tool that searches semantic memory for relevant observations.
func Recall(mem SemanticMemory) Tool {
	return &recallTool{memory: mem}
}

func (t *recallTool) Name() string { return "recall" }

func (t *recallTool) Description() string {
	return `Search your persistent knowledge base -- use BEFORE external searches!

This searches your accumulated knowledge from ALL past sessions.
Check here FIRST before web search, file reading, or MCP calls.

Uses keyword-based search (BM25). Use DISTINCTIVE KEYWORDS, not sentences:
- "PostgreSQL JSON" finds "Chose PostgreSQL for JSON support"
- "OAuth refresh tokens" finds auth-related decisions
- "What database did we choose?" is too vague, may miss results

Tips for better results:
- Use 2-4 key terms that appear in the original content
- Include specific names: tools, libraries, formats, concepts
- Try multiple searches with different keyword combinations

Returns categorized results: findings, insights, and lessons.

Parameters:
  - query (required): Keywords to search for
  - limit (optional): Results per category (default 5)`
}

func (t *recallTool) Parameters() map[string]Param {
	return map[string]Param{
		"query": {
			Type:        StringParam,
			Description: "Keywords to search for",
			Required:    true,
		},
		"limit": {
			Type:        IntParam,
			Description: "Results per category (default 5)",
		},
	}
}

func (t *recallTool) Execute(ctx context.Context, args Args) (string, error) {
	query, err := args.String("query")
	if err != nil {
		return "", err
	}

	limit := args.IntOr("limit", 5)

	results, err := t.memory.RecallFIL(ctx, query, limit)
	if err != nil {
		return "", fmt.Errorf("recall failed: %w", err)
	}

	if results == nil || (len(results.Findings) == 0 && len(results.Insights) == 0 && len(results.Lessons) == 0) {
		return "No relevant memories found.", nil
	}

	var sb strings.Builder
	if len(results.Findings) > 0 {
		sb.WriteString("Findings:\n")
		for _, f := range results.Findings {
			fmt.Fprintf(&sb, "  - %s\n", f)
		}
	}
	if len(results.Insights) > 0 {
		sb.WriteString("Insights:\n")
		for _, i := range results.Insights {
			fmt.Fprintf(&sb, "  - %s\n", i)
		}
	}
	if len(results.Lessons) > 0 {
		sb.WriteString("Lessons:\n")
		for _, l := range results.Lessons {
			fmt.Fprintf(&sb, "  - %s\n", l)
		}
	}
	return sb.String(), nil
}

// filterNonEmpty returns a new slice with empty strings removed.
func filterNonEmpty(items []string) []string {
	if items == nil {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, s := range items {
		if strings.TrimSpace(s) != "" {
			result = append(result, s)
		}
	}
	return result
}
