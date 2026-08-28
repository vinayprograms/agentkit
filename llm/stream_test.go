package llm

import (
	"context"
	"errors"
	"testing"
)

// =============================================================================
// llm.Stream fallback (non-Streamer Model)
// =============================================================================

func TestStream_FallbackOrderAndAggregation(t *testing.T) {
	m := newMock()
	m.SetResponse("hello world")
	m.stopReason = "end_turn"
	m.ChatFunc = func(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
		return &ChatResponse{
			Content:      "hello world",
			Thinking:     "let me think",
			StopReason:   "end_turn",
			InputTokens:  3,
			OutputTokens: 5,
			Model:        "mock",
		}, nil
	}

	var events []StreamEvent
	resp, err := Stream(context.Background(), m, ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(ev StreamEvent) error {
		events = append(events, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %+v", len(events), events)
	}
	if events[0].Type != StreamThinking || events[0].Text != "let me think" {
		t.Errorf("expected thinking event first, got %+v", events[0])
	}
	if events[1].Type != StreamContent || events[1].Text != "hello world" {
		t.Errorf("expected content event second, got %+v", events[1])
	}

	if resp.Content != "hello world" || resp.Thinking != "let me think" {
		t.Errorf("unexpected aggregated response: %+v", resp)
	}
	if resp.InputTokens != 3 || resp.OutputTokens != 5 {
		t.Errorf("unexpected token counts: %+v", resp)
	}
}

func TestStream_FallbackSkipsEmptyFields(t *testing.T) {
	m := newMock()
	m.ChatFunc = func(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
		return &ChatResponse{Content: "only content"}, nil
	}

	var events []StreamEvent
	_, err := Stream(context.Background(), m, ChatRequest{}, func(ev StreamEvent) error {
		events = append(events, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (no thinking), got %d: %+v", len(events), events)
	}
	if events[0].Type != StreamContent {
		t.Errorf("expected content event, got %+v", events[0])
	}
}

func TestStream_FallbackCallbackErrorAbortsAndWraps(t *testing.T) {
	m := newMock()
	m.ChatFunc = func(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
		return &ChatResponse{Content: "hello", Thinking: "thinking"}, nil
	}

	sentinel := errors.New("boom")
	calls := 0
	_, err := Stream(context.Background(), m, ChatRequest{}, func(ev StreamEvent) error {
		calls++
		return sentinel
	})
	if err == nil {
		t.Fatal("expected error from Stream()")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected errors.Is to find sentinel, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected callback invoked exactly once (thinking, then abort), got %d", calls)
	}
}

func TestStream_ChatErrorPropagates(t *testing.T) {
	m := newMock()
	sentinel := errors.New("chat failed")
	m.SetError(sentinel)

	_, err := Stream(context.Background(), m, ChatRequest{}, func(ev StreamEvent) error {
		t.Fatal("callback should not be invoked when Chat fails")
		return nil
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("expected errors.Is to find sentinel, got %v", err)
	}
}

// streamerMock is a Model that also implements Streamer, used to verify
// delegation (llm.Stream and tracedModel.Stream should call Stream, not
// synthesize deltas from Chat).
type streamerMock struct {
	*mockModel
	streamCalled bool
	streamFunc   func(ctx context.Context, req ChatRequest, on func(StreamEvent) error) (*ChatResponse, error)
}

func (s *streamerMock) Stream(ctx context.Context, req ChatRequest, on func(StreamEvent) error) (*ChatResponse, error) {
	s.streamCalled = true
	if s.streamFunc != nil {
		return s.streamFunc(ctx, req, on)
	}
	return &ChatResponse{Content: "native"}, nil
}

func TestStream_DelegatesToStreamer(t *testing.T) {
	sm := &streamerMock{mockModel: newMock()}
	resp, err := Stream(context.Background(), sm, ChatRequest{}, func(ev StreamEvent) error { return nil })
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	if !sm.streamCalled {
		t.Error("expected Stream() to delegate to the Streamer's Stream method")
	}
	if sm.mockModel.CallCount() != 0 {
		t.Error("expected Chat not to be called when Streamer is available")
	}
	if resp.Content != "native" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

// =============================================================================
// tracedModel.Stream forwarding
// =============================================================================

func TestTracedModel_StreamForwardsToStreamerInner(t *testing.T) {
	sm := &streamerMock{mockModel: newMock()}
	traced := instrument(sm, "mock-provider", "mock-model")

	streamer, ok := traced.(Streamer)
	if !ok {
		t.Fatal("expected tracedModel to implement Streamer")
	}

	resp, err := streamer.Stream(context.Background(), ChatRequest{}, func(ev StreamEvent) error { return nil })
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	if !sm.streamCalled {
		t.Error("expected tracedModel.Stream to forward to inner.Stream")
	}
	if resp.Content != "native" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestTracedModel_StreamFallsBackForNonStreamerInner(t *testing.T) {
	m := newMock()
	m.ChatFunc = func(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
		return &ChatResponse{Content: "fallback content"}, nil
	}
	traced := instrument(m, "mock-provider", "mock-model")

	streamer, ok := traced.(Streamer)
	if !ok {
		t.Fatal("expected tracedModel to implement Streamer even when inner does not")
	}

	var events []StreamEvent
	resp, err := streamer.Stream(context.Background(), ChatRequest{}, func(ev StreamEvent) error {
		events = append(events, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	if m.CallCount() != 1 {
		t.Errorf("expected Chat called once as fallback, got %d", m.CallCount())
	}
	if len(events) != 1 || events[0].Text != "fallback content" {
		t.Errorf("unexpected synthesized events: %+v", events)
	}
	if resp.Content != "fallback content" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestTracedModel_StreamCallbackErrorWraps(t *testing.T) {
	sentinel := errors.New("cb error")
	sm := &streamerMock{mockModel: newMock(), streamFunc: func(ctx context.Context, req ChatRequest, on func(StreamEvent) error) (*ChatResponse, error) {
		return nil, errStreamCallback(sentinel)
	}}
	traced := instrument(sm, "p", "m")
	streamer := traced.(Streamer)

	_, err := streamer.Stream(context.Background(), ChatRequest{}, func(ev StreamEvent) error { return nil })
	if !errors.Is(err, sentinel) {
		t.Errorf("expected errors.Is to find sentinel through tracedModel, got %v", err)
	}
}
