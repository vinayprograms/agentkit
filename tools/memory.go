package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/vinayprograms/agentkit/types"
)

// Memory is the interface for observation-based memory used by remember/recall tools.
// memory.InMemoryStore and memory.BleveStore both satisfy this interface.
type Memory interface {
	RememberFIL(ctx context.Context, findings, insights, lessons []string, source string) ([]string, error)
	RecallFIL(ctx context.Context, query string, limitPerCategory int) (*types.FILResult, error)
}

// --- Remember Tool ---

type rememberTool struct {
	memory Memory
}

// Remember creates a tool that stores observations in persistent memory.
func Remember(mem Memory) Tool {
	return &rememberTool{memory: mem}
}

func (t *rememberTool) Name() string { return "remember" }

func (t *rememberTool) Description() string {
	return `Store important discoveries in persistent knowledge base (survives across sessions).

Categories:
- findings: Facts discovered (e.g., "API rate limit is 100/min")
- insights: Conclusions/decisions (e.g., "Chose PostgreSQL for JSON support")
- lessons: Rules for future (e.g., "Always check rate limits first")

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
	sb.WriteString("Use recall() with relevant keywords to find later.")
	return sb.String(), nil
}

// --- Recall Tool ---

type recallTool struct {
	memory Memory
}

// Recall creates a tool that searches memory for relevant observations.
func Recall(mem Memory) Tool {
	return &recallTool{memory: mem}
}

func (t *recallTool) Name() string { return "recall" }

func (t *recallTool) Description() string {
	return `Search your persistent knowledge base — use BEFORE external searches!

Uses keyword-based search. Use distinctive keywords, not sentences:
- "PostgreSQL JSON" finds "Chose PostgreSQL for JSON support"
- "OAuth refresh tokens" finds auth-related decisions

Returns categorized results: findings, insights, and lessons.`
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
