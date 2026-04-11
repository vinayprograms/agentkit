package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"


)

// pipe creates a bidirectional pair of Conns connected via io.Pipe.
func pipe() (a, b *Conn) {
	ar, bw := io.Pipe()
	br, aw := io.Pipe()
	return NewConn(ar, aw), NewConn(br, bw)
}

func TestCallResponse(t *testing.T) {
	a, b := pipe()

	b.Handle("echo", func(ctx context.Context, req *Request) (any, error) {
		var params map[string]string
		json.Unmarshal(req.Params, &params)
		return params, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); b.Run(ctx) }()
	go func() { defer wg.Done(); a.Run(ctx) }()

	resp, err := a.Call(ctx, "echo", map[string]string{"msg": "hello"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected protocol error: %+v", resp.Error)
	}

	raw, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(raw), "hello") {
		t.Fatalf("expected 'hello' in result, got %s", raw)
	}

	cancel()
	wg.Wait()
}

func TestNotification(t *testing.T) {
	a, b := pipe()

	received := make(chan string, 1)
	b.HandleNotify("ping", func(ctx context.Context, n *Notification) {
		raw, _ := json.Marshal(n.Params)
		received <- string(raw)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); b.Run(ctx) }()
	go func() { defer wg.Done(); a.Run(ctx) }()

	if err := a.Notify(ctx, "ping", map[string]string{"v": "1"}); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	select {
	case msg := <-received:
		if !strings.Contains(msg, "1") {
			t.Fatalf("unexpected notification payload: %s", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("notification not received")
	}

	cancel()
	wg.Wait()
}

func TestMethodNotFound(t *testing.T) {
	a, b := pipe()

	// b has no handlers registered

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); b.Run(ctx) }()
	go func() { defer wg.Done(); a.Run(ctx) }()

	resp, err := a.Call(ctx, "nonexistent", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != ErrNoMethod {
		t.Fatalf("expected ErrNoMethod, got %d", resp.Error.Code)
	}

	cancel()
	wg.Wait()
}

func TestHandlerReturnsACPError(t *testing.T) {
	a, b := pipe()

	b.Handle("fail", func(ctx context.Context, req *Request) (any, error) {
		return nil, &Error{Code: ErrBadParams, Message: "bad input"}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); b.Run(ctx) }()
	go func() { defer wg.Done(); a.Run(ctx) }()

	resp, err := a.Call(ctx, "fail", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != ErrBadParams {
		t.Fatalf("expected ErrBadParams, got %+v", resp.Error)
	}

	cancel()
	wg.Wait()
}

func TestHandlerReturnsGoError(t *testing.T) {
	a, b := pipe()

	b.Handle("boom", func(ctx context.Context, req *Request) (any, error) {
		return nil, errors.New("something broke")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); b.Run(ctx) }()
	go func() { defer wg.Done(); a.Run(ctx) }()

	resp, err := a.Call(ctx, "boom", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != ErrInternal {
		t.Fatalf("expected ErrInternal, got %+v", resp.Error)
	}
	if resp.Error.Message != "something broke" {
		t.Fatalf("expected error message preserved, got %q", resp.Error.Message)
	}

	cancel()
	wg.Wait()
}

func TestUnknownNotificationDropped(t *testing.T) {
	a, b := pipe()

	// b has no notification handlers — should silently drop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); b.Run(ctx) }()
	go func() { defer wg.Done(); a.Run(ctx) }()

	// Should not error — notification is fire-and-forget
	if err := a.Notify(ctx, "unknown", nil); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	// Give a moment for processing, then verify no crash
	time.Sleep(50 * time.Millisecond)

	cancel()
	wg.Wait()
}

func TestContextCancelsCall(t *testing.T) {
	a, b := pipe()

	// Handler that blocks forever
	b.Handle("slow", func(ctx context.Context, req *Request) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	runCtx, runCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer runCancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); b.Run(runCtx) }()
	go func() { defer wg.Done(); a.Run(runCtx) }()

	callCtx, callCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer callCancel()

	_, err := a.Call(callCtx, "slow", nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}

	runCancel()
	wg.Wait()
}

func TestCallAfterClose(t *testing.T) {
	r := strings.NewReader("") // EOF immediately
	conn := NewConn(r, io.Discard)

	ctx := context.Background()
	conn.Run(ctx) // returns immediately on EOF

	_, err := conn.Call(ctx, "test", nil)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestNotifyAfterClose(t *testing.T) {
	r := strings.NewReader("")
	conn := NewConn(r, io.Discard)

	ctx := context.Background()
	conn.Run(ctx)

	err := conn.Notify(ctx, "test", nil)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestHandleAfterRun(t *testing.T) {
	r := strings.NewReader("")
	conn := NewConn(r, io.Discard)

	conn.Run(context.Background())

	if err := conn.Handle("test", nil); !errors.Is(err, ErrStarted) {
		t.Fatalf("expected ErrStarted, got %v", err)
	}
	if err := conn.HandleNotify("test", nil); !errors.Is(err, ErrStarted) {
		t.Fatalf("expected ErrStarted, got %v", err)
	}
}

func TestBidirectionalCalls(t *testing.T) {
	a, b := pipe()

	// a handles "ping" from b
	a.Handle("ping", func(ctx context.Context, req *Request) (any, error) {
		return map[string]string{"pong": "from-a"}, nil
	})

	// b handles "ping" from a
	b.Handle("ping", func(ctx context.Context, req *Request) (any, error) {
		return map[string]string{"pong": "from-b"}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a.Run(ctx) }()
	go func() { defer wg.Done(); b.Run(ctx) }()

	// a calls b
	resp1, err := a.Call(ctx, "ping", nil)
	if err != nil {
		t.Fatalf("a→b Call: %v", err)
	}
	raw1, _ := json.Marshal(resp1.Result)
	if !strings.Contains(string(raw1), "from-b") {
		t.Fatalf("expected from-b, got %s", raw1)
	}

	// b calls a
	resp2, err := b.Call(ctx, "ping", nil)
	if err != nil {
		t.Fatalf("b→a Call: %v", err)
	}
	raw2, _ := json.Marshal(resp2.Result)
	if !strings.Contains(string(raw2), "from-a") {
		t.Fatalf("expected from-a, got %s", raw2)
	}

	cancel()
	wg.Wait()
}

func TestConcurrentCalls(t *testing.T) {
	a, b := pipe()

	b.Handle("echo", func(ctx context.Context, req *Request) (any, error) {
		var params map[string]string
		json.Unmarshal(req.Params, &params)
		return params, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a.Run(ctx) }()
	go func() { defer wg.Done(); b.Run(ctx) }()

	// Fire 10 concurrent calls
	var callWg sync.WaitGroup
	errs := make(chan error, 10)
	for i := range 10 {
		callWg.Add(1)
		go func(n int) {
			defer callWg.Done()
			resp, err := a.Call(ctx, "echo", map[string]string{"n": fmt.Sprintf("%d", n)})
			if err != nil {
				errs <- fmt.Errorf("call %d: %w", n, err)
				return
			}
			if resp.Error != nil {
				errs <- fmt.Errorf("call %d: protocol error %+v", n, resp.Error)
			}
		}(i)
	}

	callWg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}

	cancel()
	wg.Wait()
}

func TestCallbackFromHandler(t *testing.T) {
	a, b := pipe()

	// a handles permission requests from b
	a.Handle("permission", func(ctx context.Context, req *Request) (any, error) {
		return map[string]string{"decision": "allow"}, nil
	})

	// b's prompt handler calls back to a for permission
	b.Handle("prompt", func(ctx context.Context, req *Request) (any, error) {
		// This Call goes from b → a while b is handling a request from a.
		// Tests that goroutine-per-handler prevents deadlock.
		resp, err := b.Call(ctx, "permission", map[string]string{"tool": "read_file"})
		if err != nil {
			return nil, err
		}
		return resp.Result, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a.Run(ctx) }()
	go func() { defer wg.Done(); b.Run(ctx) }()

	resp, err := a.Call(ctx, "prompt", map[string]string{"text": "hello"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	raw, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(raw), "allow") {
		t.Fatalf("expected 'allow' in result, got %s", raw)
	}

	cancel()
	wg.Wait()
}

func TestRunReturnsOnEOF(t *testing.T) {
	r := strings.NewReader("") // immediate EOF
	conn := NewConn(r, io.Discard)

	err := conn.Run(context.Background())
	if err != nil {
		t.Fatalf("expected nil on EOF, got %v", err)
	}
}

func TestRunSkipsEmptyLines(t *testing.T) {
	req, _ := json.Marshal(Request{JSONRPC: "2.0", ID: "1", Method: "test"})
	input := "\n\n" + string(req) + "\n\n"

	var called bool
	conn := NewConn(strings.NewReader(input), io.Discard)
	conn.Handle("test", func(ctx context.Context, req *Request) (any, error) {
		called = true
		return "ok", nil
	})

	conn.Run(context.Background())

	// Give handler goroutine time to execute
	time.Sleep(50 * time.Millisecond)

	if !called {
		t.Fatal("expected handler to be called despite empty lines")
	}
}

func TestParseError(t *testing.T) {
	input := "not json at all\n"

	var out strings.Builder
	conn := NewConn(strings.NewReader(input), &out)
	conn.Run(context.Background())

	if !strings.Contains(out.String(), "parse error") {
		t.Fatalf("expected parse error response, got %q", out.String())
	}
}
