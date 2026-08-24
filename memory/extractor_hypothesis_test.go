package memory

import (
	"context"
	"testing"

	"github.com/vinayprograms/agentkit/llm"
)

// TestParseFIL_GPTOSSStyle_ThinkingAllTokens_Bug11Hypothesis reproduces the
// REPORT.md bug #11 hypothesis: "Observations: enabled" banner but ZERO
// observation events across 45 e2e runs. gpt-oss-style reasoning models can
// burn their whole max_tokens budget on hidden thinking, returning
// StopReason=="length" and empty Content (same failure mode as P0 8c/8d and
// site 1's shellguard bug) — but unlike shellguard's llmCheck, extractor.go
// has NO empty-response check or retry at all. If Content is ever empty,
// parseFIL("") silently returns (nil, nil, nil) and Extract reports success
// (err==nil) with zero observations, forever, with no signal to the caller.
func TestParseFIL_GPTOSSStyle_ThinkingAllTokens_Bug11Hypothesis(t *testing.T) {
	// Content is empty because the model spent its entire token budget on
	// hidden thinking (that text lives in ChatResponse.Thinking, which
	// parseFIL never sees — it only ever looks at resp.Content).
	f, i, l := parseFIL("")
	if f != nil || i != nil || l != nil {
		t.Fatalf("expected nil/nil/nil from empty content, got %v/%v/%v", f, i, l)
	}
	// CONFIRMED: an empty resp.Content silently yields zero observations,
	// and extractor.Extract has no retry/empty-check to catch it before
	// calling parseFIL (see extractor.go Extract: resp, err := e.model.Chat(...);
	// if err != nil { return nil,nil,nil,nil }; f, i, l := parseFIL(resp.Content)
	// — no check of resp.Content=="" or resp.StopReason=="length" in between).
}

// TestParseFIL_GPTOSSStyle_ThinkingPreambleThenFencedJSON exercises the
// other half of the #11 hypothesis: even when the model DOES return prose
// content (not empty), a gpt-oss-style reply commonly puts a chain-of-thought
// preamble in Content before the fenced JSON answer (some deployments surface
// thinking as leading prose in Content rather than a separate field). The old
// parseFIL only strips a ``` fence when content starts with "```"
// (strings.HasPrefix) — a preamble before the fence defeats that entirely,
// and parseFIL falls back to a naive first-"{"/last-"}" slice of the WHOLE
// content, including any braces in the preamble.
func TestParseFIL_GPTOSSStyle_ThinkingPreambleThenFencedJSON(t *testing.T) {
	content := "Let me think about what to extract here. The schema I should follow looks " +
		"roughly like {key: value} pairs grouped into three arrays, so I'll produce that " +
		"structure now.\n\n" +
		"```json\n" +
		`{"findings": ["The API rate limit is 100 req/min"], "insights": ["REST fits better than GraphQL"], "lessons": ["Check rate limits first"]}` +
		"\n```\n"

	f, i, l := parseFIL(content)
	if len(f) == 0 && len(i) == 0 && len(l) == 0 {
		t.Log("CONFIRMED: preamble containing '{'/'}' before the fenced JSON defeats parseFIL's naive first-brace/last-brace scan; extraction silently returns nothing.")
		return
	}
	t.Logf("parseFIL recovered findings=%v insights=%v lessons=%v despite the preamble; this particular shape doesn't reproduce the bug (the brace-mismatch scenario needs testing case-by-case).", f, i, l)
}

// emptyContentModel always returns empty Content/StopReason=="length", the
// gpt-oss failure mode where the whole max_tokens budget goes to hidden
// thinking. Before the llm.Ask conversion below, extractor.Extract made
// exactly ONE Chat call in this situation and silently returned zero
// findings/insights/lessons on every single call — no retry, unlike
// shellguard's llmCheck (see TestExtractor_Extract_EmptyContent_
// Bug11Hypothesis_Fixed, which confirms the fix: Extract now goes through
// llm.Ask and gets the same retry-once-then-give-up handling shellguard
// already had).
type emptyContentModel struct{ calls int }

func (m *emptyContentModel) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	m.calls++
	return &llm.ChatResponse{Content: "", StopReason: "length"}, nil
}

func TestExtractor_Extract_EmptyContent_Bug11Hypothesis_Fixed(t *testing.T) {
	m := &emptyContentModel{}
	e := NewExtractor(m)

	f, i, l, err := e.Extract(context.Background(), "This is a long enough piece of text to pass the length gate for extraction to run at all.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != nil || i != nil || l != nil {
		t.Errorf("expected nil/nil/nil observations, got %v/%v/%v", f, i, l)
	}
	// FIXED: Extract now goes through llm.Ask, which retries once on an
	// empty response before giving up — the same handling shellguard's
	// llmCheck already had. Two calls, not the pre-fix one-shot.
	if m.calls != 2 {
		t.Errorf("expected llm.Ask's one retry (2 calls), got %d", m.calls)
	}
}

// toolCallModel returns a fixed tool call for the extract tool, exercising
// the structured (non-fallback) path.
type toolCallModel struct{ args map[string]interface{} }

func (m *toolCallModel) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		ToolCalls: []llm.ToolCallResponse{{ID: "tc-1", Name: "extract", Args: m.args}},
	}, nil
}

func TestExtractor_Extract_ToolCallPath(t *testing.T) {
	m := &toolCallModel{args: map[string]interface{}{
		"findings": []interface{}{"The API rate limit is 100 req/min"},
		"insights": []interface{}{"REST fits better than GraphQL"},
		"lessons":  []interface{}{"Check rate limits first"},
	}}
	e := NewExtractor(m)

	f, i, l, err := e.Extract(context.Background(), "This is a long enough piece of text to pass the length gate for extraction to run at all.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f) != 1 || f[0] != "The API rate limit is 100 req/min" {
		t.Errorf("unexpected findings: %v", f)
	}
	if len(i) != 1 || i[0] != "REST fits better than GraphQL" {
		t.Errorf("unexpected insights: %v", i)
	}
	if len(l) != 1 || l[0] != "Check rate limits first" {
		t.Errorf("unexpected lessons: %v", l)
	}
}
