package agent_test

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vinayprograms/agentkit/acp"
	"github.com/vinayprograms/agentkit/acp/agent"
	"github.com/vinayprograms/agentkit/acp/host"
	"github.com/vinayprograms/agentkit/acp/proto/content"
	"github.com/vinayprograms/agentkit/acp/proto/fs"
	"github.com/vinayprograms/agentkit/acp/proto/prompt"
	"github.com/vinayprograms/agentkit/acp/proto/session"
	"github.com/vinayprograms/agentkit/acp/proto/terminal"
	"github.com/vinayprograms/agentkit/acp/proto/tool"
	"github.com/vinayprograms/agentkit/acp/proto/update"
)

// pipe creates a bidirectional connection between agent and host.
func pipe() (agentR io.ReadCloser, agentW io.WriteCloser, hostR io.ReadCloser, hostW io.WriteCloser) {
	ar, hw := io.Pipe()
	hr, aw := io.Pipe()
	return ar, aw, hr, hw
}

func TestHandshake(t *testing.T) {
	ar, aw, hr, hw := pipe()

	srv := agent.New(agent.Config{
		Info:         acp.Info{Name: "test-agent", Version: "1.0"},
		Capabilities: agent.Capabilities{Image: true},
		Prompt: func(ctx context.Context, turn *agent.Turn) (prompt.Result, error) {
			return prompt.Result{Reason: prompt.EndTurn}, nil
		},
	})

	h := host.New(host.Config{
		Info: acp.Info{Name: "test-host", Version: "1.0"},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		srv.Run(ctx, ar, aw)
	}()

	if err := h.Start(ctx, hr, hw); err != nil {
		t.Fatalf("Start: %v", err)
	}

	cancel()
	wg.Wait()
}

func TestPromptRoundtrip(t *testing.T) {
	ar, aw, hr, hw := pipe()

	srv := agent.New(agent.Config{
		Info: acp.Info{Name: "test-agent", Version: "1.0"},
		Prompt: func(ctx context.Context, turn *agent.Turn) (prompt.Result, error) {
			if len(turn.Params.Content) == 0 {
				return prompt.Result{}, nil
			}
			text := turn.Params.Content[0].Text
			turn.SendUpdate(ctx, update.Update{
				Type:  update.Message,
				Role:  "assistant",
				Chunk: "Echo: " + text,
			})
			return prompt.Result{Reason: prompt.EndTurn}, nil
		},
	})

	var received []update.Update
	var mu sync.Mutex

	h := host.New(host.Config{
		Info: acp.Info{Name: "test-host", Version: "1.0"},
		OnUpdate: func(ctx context.Context, u update.Update) {
			mu.Lock()
			received = append(received, u)
			mu.Unlock()
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); srv.Run(ctx, ar, aw) }()

	if err := h.Start(ctx, hr, hw); err != nil {
		t.Fatalf("Start: %v", err)
	}

	sess, err := h.NewSession(ctx, session.Params{Cwd: "/project"})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}

	result, err := h.Prompt(ctx, sess.ID, []content.Block{
		{Type: content.Text, Text: "Hello agent"},
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if result.Reason != prompt.EndTurn {
		t.Fatalf("expected EndTurn, got %s", result.Reason)
	}

	// Give notification time to arrive.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) == 0 {
		t.Fatal("expected at least one update")
	}
	if !strings.Contains(received[0].Chunk, "Echo: Hello agent") {
		t.Fatalf("unexpected chunk: %q", received[0].Chunk)
	}

	cancel()
	wg.Wait()
}

func TestReadFileCallback(t *testing.T) {
	ar, aw, hr, hw := pipe()

	srv := agent.New(agent.Config{
		Info: acp.Info{Name: "test-agent", Version: "1.0"},
		Prompt: func(ctx context.Context, turn *agent.Turn) (prompt.Result, error) {
			result, err := turn.ReadFile(ctx, fs.ReadParams{Path: "/hello.txt"})
			if err != nil {
				return prompt.Result{}, err
			}
			turn.SendUpdate(ctx, update.Update{
				Type: update.Message, Chunk: result.Content,
			})
			return prompt.Result{Reason: prompt.EndTurn}, nil
		},
	})

	h := host.New(host.Config{
		Info: acp.Info{Name: "test-host", Version: "1.0"},
		FS: &mockFS{content: "file content here"},
		OnUpdate: func(ctx context.Context, u update.Update) {},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); srv.Run(ctx, ar, aw) }()

	h.Start(ctx, hr, hw)
	sess, _ := h.NewSession(ctx, session.Params{Cwd: "/"})

	result, err := h.Prompt(ctx, sess.ID, []content.Block{{Type: content.Text, Text: "read"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if result.Reason != prompt.EndTurn {
		t.Fatalf("expected EndTurn, got %s", result.Reason)
	}

	cancel()
	wg.Wait()
}

func TestTerminalCallback(t *testing.T) {
	ar, aw, hr, hw := pipe()

	srv := agent.New(agent.Config{
		Info: acp.Info{Name: "test-agent", Version: "1.0"},
		Prompt: func(ctx context.Context, turn *agent.Turn) (prompt.Result, error) {
			id, err := turn.CreateTerminal(ctx, terminal.Create{Command: "echo", Args: []string{"hello"}})
			if err != nil {
				return prompt.Result{}, err
			}
			result, err := turn.TerminalWait(ctx, id)
			if err != nil {
				return prompt.Result{}, err
			}
			turn.SendUpdate(ctx, update.Update{
				Type: update.Message, Chunk: result.Output,
			})
			_ = turn.TerminalRelease(ctx, id)
			return prompt.Result{Reason: prompt.EndTurn}, nil
		},
	})

	h := host.New(host.Config{
		Info:     acp.Info{Name: "test-host", Version: "1.0"},
		Terminal: &mockTerminal{},
		OnUpdate: func(ctx context.Context, u update.Update) {},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); srv.Run(ctx, ar, aw) }()

	h.Start(ctx, hr, hw)
	sess, _ := h.NewSession(ctx, session.Params{Cwd: "/"})

	result, err := h.Prompt(ctx, sess.ID, []content.Block{{Type: content.Text, Text: "run"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if result.Reason != prompt.EndTurn {
		t.Fatalf("expected EndTurn, got %s", result.Reason)
	}

	cancel()
	wg.Wait()
}

func TestPermissionCallback(t *testing.T) {
	ar, aw, hr, hw := pipe()

	srv := agent.New(agent.Config{
		Info: acp.Info{Name: "test-agent", Version: "1.0"},
		Prompt: func(ctx context.Context, turn *agent.Turn) (prompt.Result, error) {
			approval, err := turn.RequestPermission(ctx, tool.Permission{
				SessionID: turn.Session.ID,
				ToolCall:  tool.Call{ID: "1", Kind: tool.Execute, Status: tool.Pending},
			})
			if err != nil {
				return prompt.Result{}, err
			}
			turn.SendUpdate(ctx, update.Update{
				Type: update.Message, Chunk: string(approval.Decision),
			})
			return prompt.Result{Reason: prompt.EndTurn}, nil
		},
	})

	h := host.New(host.Config{
		Info:       acp.Info{Name: "test-host", Version: "1.0"},
		Permission: &mockPermission{decision: tool.AllowOnce},
		OnUpdate:   func(ctx context.Context, u update.Update) {},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); srv.Run(ctx, ar, aw) }()

	h.Start(ctx, hr, hw)
	sess, _ := h.NewSession(ctx, session.Params{Cwd: "/"})

	_, err := h.Prompt(ctx, sess.ID, []content.Block{{Type: content.Text, Text: "do it"}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	cancel()
	wg.Wait()
}

func TestAuth(t *testing.T) {
	ar, aw, hr, hw := pipe()

	srv := agent.New(agent.Config{
		Info: acp.Info{Name: "test-agent", Version: "1.0"},
		Auth: func(ctx context.Context, token string) error {
			if token != "secret" {
				return &struct{ error }{error: nil} // just return generic error
			}
			return nil
		},
		Prompt: func(ctx context.Context, turn *agent.Turn) (prompt.Result, error) {
			return prompt.Result{Reason: prompt.EndTurn}, nil
		},
	})

	h := host.New(host.Config{
		Info:  acp.Info{Name: "test-host", Version: "1.0"},
		Token: "secret",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); srv.Run(ctx, ar, aw) }()

	if err := h.Start(ctx, hr, hw); err != nil {
		t.Fatalf("Start with correct token: %v", err)
	}

	cancel()
	wg.Wait()
}

// --- mock handlers ---

type mockFS struct {
	content string
}

func (m *mockFS) ReadFile(ctx context.Context, sessionID string, p fs.ReadParams) (fs.ReadResult, error) {
	return fs.ReadResult{Content: m.content}, nil
}

func (m *mockFS) WriteFile(ctx context.Context, sessionID string, p fs.WriteParams) (fs.WriteResult, error) {
	m.content = p.Content
	return fs.WriteResult{}, nil
}

type mockTerminal struct {
	nextID int
}

func (m *mockTerminal) Create(ctx context.Context, sessionID string, p terminal.Create) (string, error) {
	m.nextID++
	return "term-1", nil
}

func (m *mockTerminal) Output(ctx context.Context, sessionID, terminalID string) (terminal.Result, error) {
	return terminal.Result{Output: "mock output"}, nil
}

func (m *mockTerminal) Wait(ctx context.Context, sessionID, terminalID string) (terminal.Result, error) {
	return terminal.Result{ExitCode: 0, Output: "done"}, nil
}

func (m *mockTerminal) Kill(ctx context.Context, sessionID, terminalID string) error {
	return nil
}

func (m *mockTerminal) Release(ctx context.Context, sessionID, terminalID string) error {
	return nil
}

type mockPermission struct {
	decision tool.Decision
}

func (m *mockPermission) Request(ctx context.Context, p tool.Permission) (tool.Approval, error) {
	return tool.Approval{Decision: m.decision}, nil
}
