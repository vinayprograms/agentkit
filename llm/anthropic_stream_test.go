package llm

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// mustAnthropicEvent unmarshals raw JSON into a MessageStreamEventUnion the
// same way the SDK's SSE decoder does, so JSON.raw (and hence the As*
// accessors) are populated exactly as they would be for a real event.
func mustAnthropicEvent(t *testing.T, raw string) anthropic.MessageStreamEventUnion {
	t.Helper()
	var ev anthropic.MessageStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		t.Fatalf("unmarshal synthetic event: %v", err)
	}
	return ev
}

func TestProcessAnthropicStreamEvent_TextDelta(t *testing.T) {
	blocks := map[int64]*anthropicBlockState{}
	result := &ChatResponse{}

	start := mustAnthropicEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	if err := processAnthropicStreamEvent(start, blocks, result, nil); err != nil {
		t.Fatalf("content_block_start: %v", err)
	}

	var events []StreamEvent
	on := func(ev StreamEvent) error {
		events = append(events, ev)
		return nil
	}

	delta1 := mustAnthropicEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello "}}`)
	if err := processAnthropicStreamEvent(delta1, blocks, result, on); err != nil {
		t.Fatalf("delta1: %v", err)
	}
	delta2 := mustAnthropicEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"world"}}`)
	if err := processAnthropicStreamEvent(delta2, blocks, result, on); err != nil {
		t.Fatalf("delta2: %v", err)
	}

	stop := mustAnthropicEvent(t, `{"type":"content_block_stop","index":0}`)
	if err := processAnthropicStreamEvent(stop, blocks, result, on); err != nil {
		t.Fatalf("content_block_stop: %v", err)
	}

	if len(events) != 2 || events[0].Text != "Hello " || events[1].Text != "world" {
		t.Fatalf("unexpected events: %+v", events)
	}
	for _, ev := range events {
		if ev.Type != StreamContent {
			t.Errorf("expected StreamContent, got %s", ev.Type)
		}
	}
	if result.Content != "Hello world" {
		t.Errorf("expected aggregated content 'Hello world', got %q", result.Content)
	}
}

func TestProcessAnthropicStreamEvent_ThinkingDelta(t *testing.T) {
	blocks := map[int64]*anthropicBlockState{}
	result := &ChatResponse{}

	start := mustAnthropicEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`)
	processAnthropicStreamEvent(start, blocks, result, nil)

	var events []StreamEvent
	on := func(ev StreamEvent) error {
		events = append(events, ev)
		return nil
	}
	delta := mustAnthropicEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"pondering..."}}`)
	if err := processAnthropicStreamEvent(delta, blocks, result, on); err != nil {
		t.Fatalf("delta: %v", err)
	}
	stop := mustAnthropicEvent(t, `{"type":"content_block_stop","index":0}`)
	processAnthropicStreamEvent(stop, blocks, result, on)

	if len(events) != 1 || events[0].Type != StreamThinking || events[0].Text != "pondering..." {
		t.Fatalf("unexpected events: %+v", events)
	}
	if result.Thinking != "pondering..." {
		t.Errorf("expected aggregated thinking, got %q", result.Thinking)
	}
}

func TestProcessAnthropicStreamEvent_ToolUseBufferedNotSurfaced(t *testing.T) {
	blocks := map[int64]*anthropicBlockState{}
	result := &ChatResponse{}

	start := mustAnthropicEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tc-1","name":"search"}}`)
	if err := processAnthropicStreamEvent(start, blocks, result, nil); err != nil {
		t.Fatalf("start: %v", err)
	}

	var events []StreamEvent
	on := func(ev StreamEvent) error {
		events = append(events, ev)
		return nil
	}

	d1 := mustAnthropicEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"q\":"}}`)
	if err := processAnthropicStreamEvent(d1, blocks, result, on); err != nil {
		t.Fatalf("d1: %v", err)
	}
	d2 := mustAnthropicEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"go\"}"}}`)
	if err := processAnthropicStreamEvent(d2, blocks, result, on); err != nil {
		t.Fatalf("d2: %v", err)
	}

	stop := mustAnthropicEvent(t, `{"type":"content_block_stop","index":0}`)
	if err := processAnthropicStreamEvent(stop, blocks, result, on); err != nil {
		t.Fatalf("stop: %v", err)
	}

	if len(events) != 0 {
		t.Fatalf("expected no stream events for tool-use deltas, got %+v", events)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 buffered tool call, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].ID != "tc-1" || result.ToolCalls[0].Name != "search" {
		t.Errorf("unexpected tool call: %+v", result.ToolCalls[0])
	}
	if result.ToolCalls[0].Args["q"] != "go" {
		t.Errorf("expected assembled args q=go, got %v", result.ToolCalls[0].Args)
	}
}

func TestProcessAnthropicStreamEvent_MessageStartAndDelta(t *testing.T) {
	blocks := map[int64]*anthropicBlockState{}
	result := &ChatResponse{}

	start := mustAnthropicEvent(t, `{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","content":[],"model":"claude-test","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":11,"output_tokens":0,"cache_creation_input_tokens":2,"cache_read_input_tokens":3}}}`)
	if err := processAnthropicStreamEvent(start, blocks, result, nil); err != nil {
		t.Fatalf("message_start: %v", err)
	}
	if result.Model != "claude-test" || result.InputTokens != 11 || result.CacheCreationInputTokens != 2 || result.CacheReadInputTokens != 3 {
		t.Fatalf("unexpected result after message_start: %+v", result)
	}

	delta := mustAnthropicEvent(t, `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":42}}`)
	if err := processAnthropicStreamEvent(delta, blocks, result, nil); err != nil {
		t.Fatalf("message_delta: %v", err)
	}
	if result.StopReason != "end_turn" || result.OutputTokens != 42 {
		t.Fatalf("unexpected result after message_delta: %+v", result)
	}
}

func TestProcessAnthropicStreamEvent_CallbackErrorAbortsAndWraps(t *testing.T) {
	blocks := map[int64]*anthropicBlockState{}
	result := &ChatResponse{}

	start := mustAnthropicEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	processAnthropicStreamEvent(start, blocks, result, nil)

	sentinel := errors.New("boom")
	on := func(ev StreamEvent) error { return sentinel }

	delta := mustAnthropicEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`)
	err := processAnthropicStreamEvent(delta, blocks, result, on)
	if err == nil {
		t.Fatal("expected error from callback")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected errors.Is to find sentinel, got %v", err)
	}
}

func TestProcessAnthropicStreamEvent_ToolArgsMalformedJSON(t *testing.T) {
	blocks := map[int64]*anthropicBlockState{}
	result := &ChatResponse{}

	start := mustAnthropicEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tc-1","name":"search"}}`)
	processAnthropicStreamEvent(start, blocks, result, nil)

	d := mustAnthropicEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{not json"}}`)
	processAnthropicStreamEvent(d, blocks, result, nil)

	stop := mustAnthropicEvent(t, `{"type":"content_block_stop","index":0}`)
	err := processAnthropicStreamEvent(stop, blocks, result, nil)
	if err == nil {
		t.Fatal("expected error for malformed tool call arguments")
	}
}
