package llm

import (
	"context"
	"fmt"
)

// StreamEventType identifies the kind of delta carried by a StreamEvent.
type StreamEventType string

const (
	// StreamContent marks a delta of the assistant's visible reply text.
	StreamContent StreamEventType = "content"
	// StreamThinking marks a delta of the assistant's extended-thinking text.
	StreamThinking StreamEventType = "thinking"
)

// StreamEvent is one delta delivered during a streaming chat.
type StreamEvent struct {
	Type StreamEventType
	Text string
}

// Streamer is an optional capability interface a Model may implement to
// deliver token-by-token deltas instead of only a complete response.
//
// Stream must return the same aggregated *ChatResponse that Chat would for
// the same exchange. The callback is invoked synchronously, in arrival
// order, and only with non-empty Text; tool-call deltas are never surfaced
// as events, they are buffered internally and delivered only in the final
// ChatResponse. A non-nil error returned by the callback aborts the stream.
type Streamer interface {
	Stream(ctx context.Context, req ChatRequest, on func(StreamEvent) error) (*ChatResponse, error)
}

// errStreamCallback wraps an error returned by a Stream callback so that
// errors.Is can find the original callback error through the provider's
// wrapping.
func errStreamCallback(err error) error {
	return fmt.Errorf("stream callback: %w", err)
}

// Stream works with ANY Model: it delegates to real streaming when m
// implements Streamer, and falls back to a single Chat call otherwise,
// synthesizing deltas from the aggregated response. A caller that ignores
// every delta gets Chat-equivalent results either way.
func Stream(ctx context.Context, m Model, req ChatRequest, on func(StreamEvent) error) (*ChatResponse, error) {
	if s, ok := m.(Streamer); ok {
		return s.Stream(ctx, req, on)
	}

	resp, err := m.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.Thinking != "" {
		if err := on(StreamEvent{Type: StreamThinking, Text: resp.Thinking}); err != nil {
			return nil, errStreamCallback(err)
		}
	}
	if resp.Content != "" {
		if err := on(StreamEvent{Type: StreamContent, Text: resp.Content}); err != nil {
			return nil, errStreamCallback(err)
		}
	}

	return resp, nil
}

// deliveryTracker wraps a caller-supplied callback so providers can tell,
// after the fact, whether any delta was successfully delivered. This is the
// signal withStreamRetry uses to decide whether a failure may still be
// retried: once the callback has received data, a retry would double-deliver
// it, so it must not be attempted.
func deliveryTracker(on func(StreamEvent) error) (wrapped func(StreamEvent) error, delivered *bool) {
	d := false
	return func(ev StreamEvent) error {
		if err := on(ev); err != nil {
			return err
		}
		d = true
		return nil
	}, &d
}
