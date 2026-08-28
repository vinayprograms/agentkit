package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ndjsonLine writes one NDJSON chunk followed by a newline, flushing so the
// client observes it as a discrete streaming delivery.
func ndjsonLine(w http.ResponseWriter, flusher http.Flusher, chunk ollamaChatResponse) {
	b, _ := json.Marshal(chunk)
	w.Write(b)
	w.Write([]byte("\n"))
	flusher.Flush()
}

func TestOllamaCloudProvider_Stream_DeltaSequenceAndAggregation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected /api/chat, got %s", r.URL.Path)
		}
		flusher := w.(http.Flusher)
		ndjsonLine(w, flusher, ollamaChatResponse{Model: "gpt-oss:120b", Message: ollamaMessage{Role: "assistant", Thinking: "thinking-1"}})
		ndjsonLine(w, flusher, ollamaChatResponse{Model: "gpt-oss:120b", Message: ollamaMessage{Role: "assistant", Content: "Hello "}})
		ndjsonLine(w, flusher, ollamaChatResponse{Model: "gpt-oss:120b", Message: ollamaMessage{Role: "assistant", Content: "world"}})
		ndjsonLine(w, flusher, ollamaChatResponse{
			Model: "gpt-oss:120b", Done: true, DoneReason: "stop",
			PromptEvalCount: 7, EvalCount: 9,
		})
	}))
	defer server.Close()

	p, err := newOllamaCloud(ollamaCloudConfig{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-oss:120b"})
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
	if events[1].Text != "Hello " || events[2].Text != "world" {
		t.Errorf("unexpected content deltas: %+v", events[1:])
	}

	if resp.Content != "Hello world" {
		t.Errorf("expected aggregated content 'Hello world', got %q", resp.Content)
	}
	if resp.Thinking != "thinking-1" {
		t.Errorf("expected aggregated thinking 'thinking-1', got %q", resp.Thinking)
	}
	if resp.StopReason != "stop" || resp.InputTokens != 7 || resp.OutputTokens != 9 {
		t.Errorf("unexpected final fields: %+v", resp)
	}
}

// TestOllamaCloudProvider_Stream_MatchesChat verifies that streaming and
// non-streaming requests against equivalent fixtures produce identical
// aggregated ChatResponse values.
func TestOllamaCloudProvider_Stream_MatchesChat(t *testing.T) {
	nonStreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaChatResponse{
			Model:           "gpt-oss:120b",
			Message:         ollamaMessage{Role: "assistant", Content: "Hello world", Thinking: "thinking-1"},
			Done:            true,
			DoneReason:      "stop",
			PromptEvalCount: 7,
			EvalCount:       9,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer nonStreamServer.Close()

	streamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		ndjsonLine(w, flusher, ollamaChatResponse{Model: "gpt-oss:120b", Message: ollamaMessage{Role: "assistant", Thinking: "thinking-1"}})
		ndjsonLine(w, flusher, ollamaChatResponse{Model: "gpt-oss:120b", Message: ollamaMessage{Role: "assistant", Content: "Hello world"}})
		ndjsonLine(w, flusher, ollamaChatResponse{Model: "gpt-oss:120b", Done: true, DoneReason: "stop", PromptEvalCount: 7, EvalCount: 9})
	}))
	defer streamServer.Close()

	chatP, _ := newOllamaCloud(ollamaCloudConfig{APIKey: "k", BaseURL: nonStreamServer.URL, Model: "gpt-oss:120b"})
	streamP, _ := newOllamaCloud(ollamaCloudConfig{APIKey: "k", BaseURL: streamServer.URL, Model: "gpt-oss:120b"})

	req := ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}
	chatResp, err := chatP.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	streamResp, err := streamP.Stream(context.Background(), req, func(StreamEvent) error { return nil })
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	if chatResp.Content != streamResp.Content || chatResp.Thinking != streamResp.Thinking ||
		chatResp.StopReason != streamResp.StopReason || chatResp.InputTokens != streamResp.InputTokens ||
		chatResp.OutputTokens != streamResp.OutputTokens || chatResp.Model != streamResp.Model {
		t.Errorf("Chat and Stream diverged:\nchat:   %+v\nstream: %+v", chatResp, streamResp)
	}
}

func TestOllamaCloudProvider_Stream_CallbackErrorAbortsNoFurtherDeliveries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		ndjsonLine(w, flusher, ollamaChatResponse{Message: ollamaMessage{Content: "one"}})
		ndjsonLine(w, flusher, ollamaChatResponse{Message: ollamaMessage{Content: "two"}})
		ndjsonLine(w, flusher, ollamaChatResponse{Done: true, DoneReason: "stop"})
	}))
	defer server.Close()

	p, _ := newOllamaCloud(ollamaCloudConfig{APIKey: "k", BaseURL: server.URL, Model: "m"})

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

func TestOllamaCloudProvider_Stream_RetryBeforeFirstDelta(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"boom"}`))
			return
		}
		flusher := w.(http.Flusher)
		ndjsonLine(w, flusher, ollamaChatResponse{Message: ollamaMessage{Content: "hi there"}})
		ndjsonLine(w, flusher, ollamaChatResponse{Done: true, DoneReason: "stop"})
	}))
	defer server.Close()

	p, _ := newOllamaCloud(ollamaCloudConfig{
		APIKey: "k", BaseURL: server.URL, Model: "m",
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

func TestOllamaCloudProvider_Stream_MidStreamDisconnectAfterDeltaNoRetry(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		flusher := w.(http.Flusher)
		ndjsonLine(w, flusher, ollamaChatResponse{Message: ollamaMessage{Content: "partial"}})
		// Truncate the connection mid-stream: write invalid NDJSON then
		// close, forcing a body parse error after a delta was delivered.
		w.Write([]byte("{not valid json"))
		flusher.Flush()
	}))
	defer server.Close()

	p, _ := newOllamaCloud(ollamaCloudConfig{
		APIKey: "k", BaseURL: server.URL, Model: "m",
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
