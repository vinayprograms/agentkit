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

	// InputTokens and OutputTokens are summed across every Chat call Ask
	// made to reach this Decision (including the empty-response retry, if
	// one happened), so callers can log/attribute cost without re-deriving
	// the retry accounting themselves.
	InputTokens  int
	OutputTokens int
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
// giving up. If both attempts come back empty, Ask returns a Decision with
// ToolCalled=false, Args==nil, and Content=="" (still carrying the summed
// token counts) — callers must supply their own fallback for "the model
// said nothing at all" the way shellguard's llmCheck falls back to
// deterministic ALLOW. That empty-twice case is indistinguishable from an
// unrecoverable-prose Decision except by Content=="", so check that when it
// matters.
func Ask(ctx context.Context, model Model, prompt string, tool ToolDef, parse ParseFallback) (*Decision, error) {
	return AskThinking(ctx, model, prompt, tool, parse, ThinkingOff)
}

// AskThinking is Ask with the thinking level left to the caller.
//
// Ask forces ThinkingOff because a bounded classification does not benefit
// from deliberation. That holds for short, shaped inputs; it does not hold
// everywhere. A caller judging a long compound shell command — pipes,
// chained operators, subshells — is asking a genuine reasoning question,
// and the reviewer's answer is only as good as the reasoning behind it.
// Such a caller passes its own level here.
//
// The empty-twice retry still forces thinking off regardless of level: that
// retry exists precisely because a reasoning model can burn its whole
// max_tokens budget on hidden thinking, so retrying with the same level
// would repeat the failure it is meant to escape.
func AskThinking(ctx context.Context, model Model, prompt string, tool ToolDef, parse ParseFallback, level ThinkingLevel) (*Decision, error) {
	req := ChatRequest{
		Messages:   []Message{{Role: "user", Content: prompt}},
		Tools:      []ToolDef{tool},
		ToolChoice: ToolChoiceTool(tool.Name),
		Thinking:   level,
	}

	resp, tokensIn, tokensOut, err := chatRetryEmpty(ctx, model, req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		// Both attempts came back empty; caller supplies its own fallback.
		return &Decision{InputTokens: tokensIn, OutputTokens: tokensOut}, nil
	}

	if tc, ok := findToolCall(resp.ToolCalls, tool.Name); ok {
		return &Decision{Args: tc.Args, ToolCalled: true, Content: resp.Content, InputTokens: tokensIn, OutputTokens: tokensOut}, nil
	}

	content := strings.TrimSpace(resp.Content)
	if parse != nil {
		if args, ok := parse(content); ok {
			return &Decision{Args: args, ToolCalled: false, Content: content, InputTokens: tokensIn, OutputTokens: tokensOut}, nil
		}
	}
	return &Decision{Args: nil, ToolCalled: false, Content: content, InputTokens: tokensIn, OutputTokens: tokensOut}, nil
}

// chatRetryEmpty calls model.Chat(ctx, req) and, if the response comes back
// with empty content and no tool calls, retries once. Returns (nil, 0, 0,
// nil) if both attempts are empty; the returned token counts are always the
// sum across every attempt made. This is the empty-response handling
// shellguard's llmCheck implements inline; it lives here so every
// structured-decision call site shares it instead of re-deriving it.
func chatRetryEmpty(ctx context.Context, model Model, req ChatRequest) (*ChatResponse, int, int, error) {
	resp, err := model.Chat(ctx, req)
	if err != nil {
		return nil, 0, 0, err
	}
	tokensIn, tokensOut := resp.InputTokens, resp.OutputTokens
	if !isEmptyResponse(resp) {
		return resp, tokensIn, tokensOut, nil
	}

	// Retry with thinking forced off: this retry exists because a reasoning
	// model can spend its whole max_tokens budget on hidden thinking and
	// return empty content, so repeating the request at the caller's level
	// would just repeat that failure.
	retry := req
	retry.Thinking = ThinkingOff
	resp2, err := model.Chat(ctx, retry)
	if err != nil {
		// The first attempt already came back empty; a failing retry is no
		// worse than an empty one, so treat it the same way rather than
		// surfacing an error for what the caller would handle identically
		// either way (matches shellguard's original llmCheck behavior).
		return nil, tokensIn, tokensOut, nil
	}
	tokensIn += resp2.InputTokens
	tokensOut += resp2.OutputTokens
	if isEmptyResponse(resp2) {
		return nil, tokensIn, tokensOut, nil
	}
	return resp2, tokensIn, tokensOut, nil
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
