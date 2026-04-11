package host_test

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/vinayprograms/agentkit/acp"
	"github.com/vinayprograms/agentkit/acp/agent"
	"github.com/vinayprograms/agentkit/acp/host"
	"github.com/vinayprograms/agentkit/acp/proto/config"
	"github.com/vinayprograms/agentkit/acp/proto/content"
	"github.com/vinayprograms/agentkit/acp/proto/prompt"
	"github.com/vinayprograms/agentkit/acp/proto/session"
	"github.com/vinayprograms/agentkit/acp/proto/tool"
	"github.com/vinayprograms/agentkit/acp/proto/update"
)

func pipe() (agentR io.ReadCloser, agentW io.WriteCloser, hostR io.ReadCloser, hostW io.WriteCloser) {
	ar, hw := io.Pipe()
	hr, aw := io.Pipe()
	return ar, aw, hr, hw
}

type nopAgent struct{}

func (a *nopAgent) Prompt(_ context.Context, _ *agent.Turn) (prompt.Result, error) {
	return prompt.Result{Reason: prompt.EndTurn}, nil
}

func startPair(t *testing.T, ctx context.Context, acfg agent.Config, hcfg host.Config) (*host.Host, func()) {
	t.Helper()
	ar, aw, hr, hw := pipe()

	srv := agent.New(acfg)
	h := host.New(hcfg)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); srv.Run(ctx, ar, aw) }()

	if err := h.Start(ctx, hr, hw); err != nil {
		t.Fatalf("Start: %v", err)
	}

	return h, func() { wg.Wait() }
}

func TestNewSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, wait := startPair(t, ctx, agent.Config{
		Info:    acp.Info{Name: "agent", Version: "1"},
		Handler: &nopAgent{},
	}, host.Config{
		Info: acp.Info{Name: "host", Version: "1"},
	})

	sess, err := h.NewSession(ctx, session.Params{Cwd: "/project"})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}

	cancel()
	wait()
}

func TestPromptEndTurn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, wait := startPair(t, ctx, agent.Config{
		Info:    acp.Info{Name: "agent", Version: "1"},
		Handler: &nopAgent{},
	}, host.Config{
		Info: acp.Info{Name: "host", Version: "1"},
	})

	sess, _ := h.NewSession(ctx, session.Params{Cwd: "/"})
	result, err := h.Prompt(ctx, sess.ID, []content.Block{
		{Type: content.Text, Text: "hello"},
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if result.Reason != prompt.EndTurn {
		t.Fatalf("expected EndTurn, got %s", result.Reason)
	}

	cancel()
	wait()
}

func TestPromptCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, wait := startPair(t, ctx, agent.Config{
		Info:    acp.Info{Name: "agent", Version: "1"},
		Handler: &nopAgent{},
	}, host.Config{
		Info: acp.Info{Name: "host", Version: "1"},
	})

	sess, _ := h.NewSession(ctx, session.Params{Cwd: "/"})
	result, err := h.PromptCommand(ctx, sess.ID, config.Command{
		Name: "search", Input: "TODO",
	})
	if err != nil {
		t.Fatalf("PromptCommand: %v", err)
	}
	if result.Reason != prompt.EndTurn {
		t.Fatalf("expected EndTurn, got %s", result.Reason)
	}

	cancel()
	wait()
}

func TestCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cancelled := make(chan string, 1)

	h, wait := startPair(t, ctx, agent.Config{
		Info:    acp.Info{Name: "agent", Version: "1"},
		Handler: &nopAgent{},
		Cancel: func(_ context.Context, sessionID string) {
			cancelled <- sessionID
		},
	}, host.Config{
		Info: acp.Info{Name: "host", Version: "1"},
	})

	sess, _ := h.NewSession(ctx, session.Params{Cwd: "/"})
	if err := h.Cancel(ctx, sess.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	select {
	case id := <-cancelled:
		if id != sess.ID {
			t.Fatalf("expected session %s, got %s", sess.ID, id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel not received")
	}

	cancel()
	wait()
}

func TestOnUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type chunkAgent struct{ nopAgent }
	var ca chunkAgent

	var received []update.Update
	var mu sync.Mutex

	h, wait := startPair(t, ctx, agent.Config{
		Info: acp.Info{Name: "agent", Version: "1"},
		Handler: agent.Handler(handlerFunc(func(ctx context.Context, turn *agent.Turn) (prompt.Result, error) {
			turn.SendUpdate(ctx, update.Update{
				Type: update.Message, Role: "assistant", Chunk: "hello",
			})
			return prompt.Result{Reason: prompt.EndTurn}, nil
		})),
	}, host.Config{
		Info: acp.Info{Name: "host", Version: "1"},
		OnUpdate: func(_ context.Context, u update.Update) {
			mu.Lock()
			received = append(received, u)
			mu.Unlock()
		},
	})
	_ = ca

	sess, _ := h.NewSession(ctx, session.Params{Cwd: "/"})
	h.Prompt(ctx, sess.ID, []content.Block{{Type: content.Text, Text: "go"}})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) == 0 {
		t.Fatal("expected updates")
	}
	if received[0].Chunk != "hello" {
		t.Fatalf("expected 'hello', got %q", received[0].Chunk)
	}

	cancel()
	wait()
}

func TestPermissionFunc(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, wait := startPair(t, ctx, agent.Config{
		Info: acp.Info{Name: "agent", Version: "1"},
		Handler: handlerFunc(func(ctx context.Context, turn *agent.Turn) (prompt.Result, error) {
			approval, err := turn.RequestPermission(ctx, tool.Permission{
				ToolCall: tool.Call{ID: "1", Kind: tool.Execute},
			})
			if err != nil {
				return prompt.Result{}, err
			}
			if approval.Decision != tool.AllowAlways {
				t.Errorf("expected AllowAlways, got %s", approval.Decision)
			}
			return prompt.Result{Reason: prompt.EndTurn}, nil
		}),
	}, host.Config{
		Info: acp.Info{Name: "host", Version: "1"},
		Permission: func(_ context.Context, _ tool.Permission) (tool.Approval, error) {
			return tool.Approval{Decision: tool.AllowAlways}, nil
		},
	})

	sess, _ := h.NewSession(ctx, session.Params{Cwd: "/"})
	h.Prompt(ctx, sess.ID, []content.Block{{Type: content.Text, Text: "go"}})

	cancel()
	wait()
}

func TestAuthSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h, wait := startPair(t, ctx, agent.Config{
		Info:    acp.Info{Name: "agent", Version: "1"},
		Handler: &nopAgent{},
		Auth: func(_ context.Context, token string) error {
			if token != "valid" {
				return &rpcErr{msg: "bad token"}
			}
			return nil
		},
	}, host.Config{
		Info:  acp.Info{Name: "host", Version: "1"},
		Token: "valid",
	})
	_ = h

	cancel()
	wait()
}

func TestNoUpdateHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// OnUpdate nil — updates should be silently dropped, no crash.
	h, wait := startPair(t, ctx, agent.Config{
		Info: acp.Info{Name: "agent", Version: "1"},
		Handler: handlerFunc(func(ctx context.Context, turn *agent.Turn) (prompt.Result, error) {
			turn.SendUpdate(ctx, update.Update{Type: update.Message, Chunk: "drop me"})
			return prompt.Result{Reason: prompt.EndTurn}, nil
		}),
	}, host.Config{
		Info: acp.Info{Name: "host", Version: "1"},
		// OnUpdate deliberately nil
	})

	sess, _ := h.NewSession(ctx, session.Params{Cwd: "/"})
	_, err := h.Prompt(ctx, sess.ID, []content.Block{{Type: content.Text, Text: "go"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	cancel()
	wait()
}

// --- helpers ---

// handlerFunc adapts a function to the agent.Handler interface.
type handlerFunc func(context.Context, *agent.Turn) (prompt.Result, error)

func (f handlerFunc) Prompt(ctx context.Context, turn *agent.Turn) (prompt.Result, error) {
	return f(ctx, turn)
}

type rpcErr struct{ msg string }

func (e *rpcErr) Error() string { return e.msg }
