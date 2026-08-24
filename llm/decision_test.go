package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
)

var testTool = ToolDef{
	Name:        "verdict",
	Description: "Report the verdict.",
	Parameters: map[string]any{
		"properties": map[string]any{
			"allow": map[string]any{"type": "boolean"},
		},
	},
}

func TestAsk_ToolCalled(t *testing.T) {
	m := newMock()
	m.SetToolCall("verdict", map[string]interface{}{"allow": true})

	d, err := Ask(context.Background(), m, "check this", testTool, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d == nil {
		t.Fatal("expected a decision, got nil")
	}
	if !d.ToolCalled {
		t.Error("expected ToolCalled=true")
	}
	if d.Args["allow"] != true {
		t.Errorf("expected allow=true, got %v", d.Args["allow"])
	}

	// Request must pin ToolChoice to the tool and force thinking off.
	req := m.LastRequest()
	if name, ok := req.ToolChoice.ToolName(); !ok || name != "verdict" {
		t.Errorf("expected ToolChoice pinned to 'verdict', got %+v", req.ToolChoice)
	}
	if req.Thinking != ThinkingOff {
		t.Errorf("expected Thinking off, got %v", req.Thinking)
	}
}

func TestAsk_ProseFallback(t *testing.T) {
	m := newMock()
	m.SetResponse("some preamble\nALLOW: looks fine")

	parse := func(content string) (map[string]any, bool) {
		if strings.Contains(content, "ALLOW") {
			return map[string]any{"allow": true}, true
		}
		return nil, false
	}

	d, err := Ask(context.Background(), m, "check this", testTool, parse)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d == nil {
		t.Fatal("expected a decision, got nil")
	}
	if d.ToolCalled {
		t.Error("expected ToolCalled=false (model answered in prose)")
	}
	if d.Args["allow"] != true {
		t.Errorf("expected fallback parser's allow=true, got %v", d.Args["allow"])
	}
}

func TestAsk_ProseFallback_Unrecoverable(t *testing.T) {
	m := newMock()
	m.SetResponse("I don't know what to say")

	parse := func(content string) (map[string]any, bool) { return nil, false }

	d, err := Ask(context.Background(), m, "check this", testTool, parse)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d == nil {
		t.Fatal("expected a decision, got nil")
	}
	if d.ToolCalled {
		t.Error("expected ToolCalled=false")
	}
	if d.Args != nil {
		t.Errorf("expected nil Args when the fallback parser can't recover a decision, got %v", d.Args)
	}
	if d.Content != "I don't know what to say" {
		t.Errorf("expected raw content preserved, got %q", d.Content)
	}
}

// TestAsk_EmptyThenSuccess reproduces the reasoning-model failure mode
// (StopReason=="length", all tokens in hidden thinking, empty content) that
// shellguard's llmCheck already retries once for. Ask must do the same: one
// retry, then use whatever the retry returns.
func TestAsk_EmptyThenSuccess(t *testing.T) {
	m := newMock()
	calls := 0
	m.ChatFunc = func(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
		calls++
		if calls == 1 {
			return &ChatResponse{Content: "", StopReason: "length"}, nil
		}
		return &ChatResponse{
			ToolCalls: []ToolCallResponse{{Name: "verdict", Args: map[string]any{"allow": false}}},
		}, nil
	}

	d, err := Ask(context.Background(), m, "check this", testTool, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected exactly one retry (2 calls), got %d", calls)
	}
	if d == nil || !d.ToolCalled || d.Args["allow"] != false {
		t.Fatalf("expected the retry's tool call to win, got %+v", d)
	}
}

// TestAsk_EmptyTwice documents that Ask gives up after one retry: with both
// attempts empty, callers get a Decision with no tool call, no args, and no
// content (but still the summed token counts) and must supply their own
// fallback (e.g. shellguard's deterministic ALLOW).
func TestAsk_EmptyTwice(t *testing.T) {
	m := newMock()
	calls := 0
	m.ChatFunc = func(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
		calls++
		return &ChatResponse{Content: "", StopReason: "length", InputTokens: 10, OutputTokens: 1}, nil
	}

	d, err := Ask(context.Background(), m, "check this", testTool, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d == nil {
		t.Fatal("expected a non-nil Decision after two empty responses")
	}
	if d.ToolCalled || d.Args != nil || d.Content != "" {
		t.Errorf("expected an empty Decision, got %+v", d)
	}
	if d.InputTokens != 20 || d.OutputTokens != 2 {
		t.Errorf("expected summed token counts (20, 2), got (%d, %d)", d.InputTokens, d.OutputTokens)
	}
	if calls != 2 {
		t.Fatalf("expected exactly 2 calls (1 retry), got %d", calls)
	}
}

func TestAsk_ErrorPropagates(t *testing.T) {
	m := newMock()
	wantErr := errors.New("boom")
	m.SetError(wantErr)

	_, err := Ask(context.Background(), m, "check this", testTool, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error to propagate, got %v", err)
	}
}
