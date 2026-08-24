package llm

import (
	"context"
	"strings"
)

// Decision is the parsed result of a single structured-decision call: either
// the arguments of the tool the model was asked to call, or — when the model
// answered in prose instead — whatever a caller-supplied lenient parser could
// recover from that prose.
type Decision struct {
	// Args holds the decision's fields. When ToolCalled is true these came
	// from the model's tool-call arguments; otherwise they came from
	// ParseFallback run over Content.
	Args map[string]any

	// ToolCalled reports whether the model actually called the requested
	// tool (Args came from structured tool-call arguments) as opposed to
	// prose that ParseFallback had to recover a decision from.
	ToolCalled bool

	// Content is the model's raw text content. It is set whenever the model
	// produced (or fell back to) prose; it's empty when the model called
	// the tool with no accompanying text.
	Content string
}

// ParseFallback recovers decision arguments from prose content when the
// model didn't make the requested tool call. ok is false when the content
// carries no recoverable decision.
type ParseFallback func(content string) (args map[string]any, ok bool)

// Ask asks model to make one structured decision: it sends prompt with
// ToolChoice pinned to tool, and returns the tool call's arguments as a
// Decision.
//
// If the model answers in prose instead of calling the tool — including
// providers that can't honor ToolChoice at all (see ChatRequest.ToolChoice)
// — Ask falls back to parse, a caller-supplied lenient text parser, and
// reports ToolCalled=false.
//
// Ask also absorbs the empty-content / StopReason=="length" failure mode
// already handled in shellguard: a reasoning model can burn its whole
// max_tokens budget on hidden thinking and return empty content. Ask asks
// again once, with Thinking forced off (a decision is a bounded
// classification, not a task that benefits from deliberation), before
// giving up. If both attempts come back empty, Ask returns a nil Decision
// and a nil error — callers must supply their own fallback for "the model
// said nothing at all" the way shellguard's llmCheck falls back to
// deterministic ALLOW.
func Ask(ctx context.Context, model Model, prompt string, tool ToolDef, parse ParseFallback) (*Decision, error) {
	req := ChatRequest{
		Messages:   []Message{{Role: "user", Content: prompt}},
		Tools:      []ToolDef{tool},
		ToolChoice: ToolChoiceTool(tool.Name),
		Thinking:   ThinkingOff,
	}

	resp, err := chatRetryEmpty(ctx, model, req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		// Both attempts came back empty; caller supplies its own fallback.
		return nil, nil
	}

	if tc, ok := findToolCall(resp.ToolCalls, tool.Name); ok {
		return &Decision{Args: tc.Args, ToolCalled: true, Content: resp.Content}, nil
	}

	content := strings.TrimSpace(resp.Content)
	if parse != nil {
		if args, ok := parse(content); ok {
			return &Decision{Args: args, ToolCalled: false, Content: content}, nil
		}
	}
	return &Decision{Args: nil, ToolCalled: false, Content: content}, nil
}

// chatRetryEmpty calls model.Chat(ctx, req) and, if the response comes back
// with empty content and no tool calls, retries once. Returns (nil, nil) if
// both attempts are empty. This is the empty-response handling shellguard's
// llmCheck implements inline; it lives here so every structured-decision
// call site shares it instead of re-deriving it.
func chatRetryEmpty(ctx context.Context, model Model, req ChatRequest) (*ChatResponse, error) {
	resp, err := model.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	if !isEmptyResponse(resp) {
		return resp, nil
	}

	resp2, err := model.Chat(ctx, req)
	if err != nil {
		// The first attempt at least succeeded (empty); surface the retry's
		// error rather than silently discarding it, but let the caller
		// decide whether "empty" or "errored on retry" matters — callers
		// that only care about "did we get a decision" can treat either the
		// same as chatRetryEmpty returning (nil, nil) with no fallback.
		return nil, err
	}
	if isEmptyResponse(resp2) {
		return nil, nil
	}
	return resp2, nil
}

// isEmptyResponse reports whether resp carries no usable content: no tool
// calls and no non-whitespace text. A response with StopReason=="length"
// and empty content is the classic reasoning-model failure mode (all of
// max_tokens spent on hidden thinking); it's covered by the same check
// since it also has empty content and no tool calls.
func isEmptyResponse(resp *ChatResponse) bool {
	if resp == nil {
		return true
	}
	if len(resp.ToolCalls) > 0 {
		return false
	}
	return strings.TrimSpace(resp.Content) == ""
}

// findToolCall returns the first call to the named tool, if any.
func findToolCall(calls []ToolCallResponse, name string) (ToolCallResponse, bool) {
	for _, c := range calls {
		if c.Name == name {
			return c, true
		}
	}
	return ToolCallResponse{}, false
}
