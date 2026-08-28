package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// sseWrite writes one "data: ..." line followed by the SSE blank-line
// terminator, flushing so the client observes it as a discrete delivery.
func sseWrite(w http.ResponseWriter, flusher http.Flusher, payload string) {
	fmt.Fprintf(w, "data: %s\n\n", payload)
	flusher.Flush()
}

func sseChunkJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal chunk: %v", err)
	}
	return string(b)
}

func TestOpenAICompatProvider_Stream_DeltaSequenceAndAggregation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}
		var req oaiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if !req.Stream {
			t.Errorf("expected stream:true in request")
		}
		if req.StreamOptions == nil || !req.StreamOptions.IncludeUsage {
			t.Errorf("expected stream_options.include_usage:true in request")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		sseWrite(w, flusher, sseChunkJSON(t, map[string]any{
			"model":   "test-model",
			"choices": []map[string]any{{"delta": map[string]any{"reasoning_content": "pondering"}}},
		}))
		sseWrite(w, flusher, sseChunkJSON(t, map[string]any{
			"model":   "test-model",
			"choices": []map[string]any{{"delta": map[string]any{"content": "Hello "}}},
		}))
		sseWrite(w, flusher, sseChunkJSON(t, map[string]any{
			"model":   "test-model",
			"choices": []map[string]any{{"delta": map[string]any{"content": "world"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 4, "completion_tokens": 6},
		}))
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p, err := newOpenAICompat("test", openAICompatConfig{BaseURL: server.URL, Model: "test-model", MaxTokens: 4096})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	var events []StreamEvent
	resp, err := p.Stream(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(ev StreamEvent) error {
		events = append(events, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	wantTypes := []StreamEventType{StreamThinking, StreamContent, StreamContent}
	if len(events) != len(wantTypes) {
		t.Fatalf("expected %d events, got %d: %+v", len(wantTypes), len(events), events)
	}
	for i, want := range wantTypes {
		if events[i].Type != want {
			t.Errorf("event %d: expected type %s, got %s", i, want, events[i].Type)
		}
	}

	if resp.Content != "Hello world" {
		t.Errorf("expected aggregated content 'Hello world', got %q", resp.Content)
	}
	if resp.Thinking != "pondering" {
		t.Errorf("expected aggregated thinking 'pondering', got %q", resp.Thinking)
	}
	if resp.StopReason != "stop" || resp.InputTokens != 4 || resp.OutputTokens != 6 {
		t.Errorf("unexpected final fields: %+v", resp)
	}
}

func TestOpenAICompatProvider_Stream_ToolCallsBufferedNotSurfaced(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		sseWrite(w, flusher, sseChunkJSON(t, map[string]any{
			"choices": []map[string]any{{"delta": map[string]any{
				"tool_calls": []map[string]any{{"index": 0, "id": "call_1", "function": map[string]any{"name": "search", "arguments": `{"q":`}}},
			}}},
		}))
		sseWrite(w, flusher, sseChunkJSON(t, map[string]any{
			"choices": []map[string]any{{"delta": map[string]any{
				"tool_calls": []map[string]any{{"index": 0, "function": map[string]any{"arguments": `"go"}`}}},
			}, "finish_reason": "tool_calls"}},
		}))
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p, _ := newOpenAICompat("test", openAICompatConfig{BaseURL: server.URL, Model: "test-model", MaxTokens: 4096})

	var events []StreamEvent
	resp, err := p.Stream(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}, func(ev StreamEvent) error {
		events = append(events, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected no stream events for tool-call deltas, got %+v", events)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 buffered tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "search" || resp.ToolCalls[0].ID != "call_1" {
		t.Errorf("unexpected tool call: %+v", resp.ToolCalls[0])
	}
	if resp.ToolCalls[0].Args["q"] != "go" {
		t.Errorf("expected assembled args q=go, got %v", resp.ToolCalls[0].Args)
	}
}

// TestOpenAICompatProvider_Stream_MatchesChat verifies streaming and
// non-streaming requests against equivalent fixtures produce identical
// aggregated ChatResponse content.
func TestOpenAICompatProvider_Stream_MatchesChat(t *testing.T) {
	nonStreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id": "x", "model": "test-model",
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": "Hello world"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 4, "completion_tokens": 6},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer nonStreamServer.Close()

	streamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		sseWrite(w, flusher, sseChunkJSON(t, map[string]any{
			"model":   "test-model",
			"choices": []map[string]any{{"delta": map[string]any{"content": "Hello world"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 4, "completion_tokens": 6},
		}))
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer streamServer.Close()

	chatP, _ := newOpenAICompat("test", openAICompatConfig{BaseURL: nonStreamServer.URL, Model: "test-model", MaxTokens: 4096})
	streamP, _ := newOpenAICompat("test", openAICompatConfig{BaseURL: streamServer.URL, Model: "test-model", MaxTokens: 4096})

	req := ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}
	chatResp, err := chatP.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	streamResp, err := streamP.Stream(context.Background(), req, func(StreamEvent) error { return nil })
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	if chatResp.Content != streamResp.Content || chatResp.StopReason != streamResp.StopReason ||
		chatResp.InputTokens != streamResp.InputTokens || chatResp.OutputTokens != streamResp.OutputTokens ||
		chatResp.Model != streamResp.Model {
		t.Errorf("Chat and Stream diverged:\nchat:   %+v\nstream: %+v", chatResp, streamResp)
	}
}

func TestOpenAICompatProvider_Stream_CallbackErrorAbortsNoFurtherDeliveries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		sseWrite(w, flusher, sseChunkJSON(t, map[string]any{"choices": []map[string]any{{"delta": map[string]any{"content": "one"}}}}))
		sseWrite(w, flusher, sseChunkJSON(t, map[string]any{"choices": []map[string]any{{"delta": map[string]any{"content": "two"}}}}))
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p, _ := newOpenAICompat("test", openAICompatConfig{BaseURL: server.URL, Model: "test-model", MaxTokens: 4096})

	sentinel := errors.New("stop here")
	var got []string
	_, err := p.Stream(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}, func(ev StreamEvent) error {
		got = append(got, ev.Text)
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected errors.Is to find sentinel, got %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 delivery before abort, got %d: %v", len(got), got)
	}
}

func TestOpenAICompatProvider_Stream_RetryBeforeFirstDelta(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		sseWrite(w, flusher, sseChunkJSON(t, map[string]any{"choices": []map[string]any{{"delta": map[string]any{"content": "hi there"}, "finish_reason": "stop"}}}))
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p, _ := newOpenAICompat("test", openAICompatConfig{
		BaseURL: server.URL, Model: "test-model", MaxTokens: 4096,
		Retry: RetryConfig{MaxRetries: 3, InitBackoff: 5 * time.Millisecond, MaxBackoff: 20 * time.Millisecond},
	})

	var events []string
	resp, err := p.Stream(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}, func(ev StreamEvent) error {
		events = append(events, ev.Text)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls (1 retry), got %d", calls)
	}
	if len(events) != 1 || events[0] != "hi there" {
		t.Errorf("expected exactly one delivery of the delta, got %v", events)
	}
	if resp.Content != "hi there" {
		t.Errorf("unexpected content: %q", resp.Content)
	}
}

func TestOpenAICompatProvider_Stream_MidStreamDisconnectAfterDeltaNoRetry(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		sseWrite(w, flusher, sseChunkJSON(t, map[string]any{"choices": []map[string]any{{"delta": map[string]any{"content": "partial"}}}}))
		// Malformed trailing line forces a parse error after a delta was
		// already delivered.
		fmt.Fprint(w, "data: {not valid json\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p, _ := newOpenAICompat("test", openAICompatConfig{
		BaseURL: server.URL, Model: "test-model", MaxTokens: 4096,
		Retry: RetryConfig{MaxRetries: 3, InitBackoff: 5 * time.Millisecond, MaxBackoff: 20 * time.Millisecond},
	})

	delivered := 0
	_, err := p.Stream(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}, func(ev StreamEvent) error {
		delivered++
		return nil
	})
	if err == nil {
		t.Fatal("expected error from malformed trailing chunk")
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 request (no retry after a delta was delivered), got %d", calls)
	}
	if delivered != 1 {
		t.Errorf("expected exactly 1 delivery before the failure, got %d", delivered)
	}
}
