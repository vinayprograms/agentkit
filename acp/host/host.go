// Package host provides the host-side (editor/IDE) ACP implementation.
//
// Create a host with New, then call Start to connect to an agent:
//
//	h := host.New(host.Config{
//	    Info:     acp.Info{Name: "my-editor", Version: "2.0"},
//	    FS:       myFS,
//	    OnUpdate: func(ctx context.Context, u update.Update) { ... },
//	})
//	h.Start(ctx, agentStdout, agentStdin)
//	sess, _ := h.NewSession(ctx, session.Params{Cwd: "/project"})
//	result, _ := h.Prompt(ctx, sess.ID, []content.Block{{Type: content.Text, Text: "hello"}})
package host

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/vinayprograms/agentkit/acp"
	"github.com/vinayprograms/agentkit/acp/internal/rpc"
	"github.com/vinayprograms/agentkit/acp/proto/config"
	"github.com/vinayprograms/agentkit/acp/proto/content"
	"github.com/vinayprograms/agentkit/acp/proto/fs"
	"github.com/vinayprograms/agentkit/acp/proto/prompt"
	"github.com/vinayprograms/agentkit/acp/proto/session"
	"github.com/vinayprograms/agentkit/acp/proto/terminal"
	"github.com/vinayprograms/agentkit/acp/proto/tool"
	"github.com/vinayprograms/agentkit/acp/proto/update"
	"go.opentelemetry.io/otel/attribute"
)

// FS implements host-mediated file system access.
// Non-nil on Config → ReadTextFile and WriteTextFile advertised.
type FS interface {
	ReadFile(ctx context.Context, sessionID string, p fs.ReadParams) (fs.ReadResult, error)
	WriteFile(ctx context.Context, sessionID string, p fs.WriteParams) (fs.WriteResult, error)
}

// Terminal implements host-mediated terminal management.
// Non-nil on Config → Terminal capability advertised.
type Terminal interface {
	Create(ctx context.Context, sessionID string, p terminal.Create) (string, error)
	Output(ctx context.Context, sessionID, terminalID string) (terminal.Result, error)
	Wait(ctx context.Context, sessionID, terminalID string) (terminal.Result, error)
	Kill(ctx context.Context, sessionID, terminalID string) error
	Release(ctx context.Context, sessionID, terminalID string) error
}

// Config specifies all host behavior.
type Config struct {
	Info  acp.Info
	Token string // sent in authenticate; empty skips auth

	// Permission handles tool permission requests from the agent.
	// nil auto-rejects all permission requests.
	Permission func(ctx context.Context, p tool.Permission) (tool.Approval, error)

	FS       FS       // nil = not advertised
	Terminal Terminal // nil = not advertised

	// OnUpdate is called for each session/update notification.
	// Must not block. nil drops updates silently.
	OnUpdate func(ctx context.Context, u update.Update)
}

// Host is the host-side ACP orchestrator.
type Host struct {
	cfg     Config
	conn    *rpc.Conn
	mu      sync.Mutex
	session string // active session ID
}

// New creates a Host.
func New(cfg Config) *Host {
	return &Host{cfg: cfg}
}

// Start connects to the agent over r/w, performs the handshake,
// and returns. The connection runs until ctx is cancelled.
func (h *Host) Start(ctx context.Context, r io.Reader, w io.Writer) error {
	h.conn = rpc.NewConn(r, w)
	h.register()
	go h.conn.Run(ctx)

	// Initialize.
	caps := map[string]any{}
	if h.cfg.FS != nil {
		caps["fs.readTextFile"] = true
		caps["fs.writeTextFile"] = true
	}
	if h.cfg.Terminal != nil {
		caps["terminal"] = true
	}

	resp, err := h.conn.Call(ctx, rpc.MethodInitialize, initParams{
		ProtocolVersion: 1,
		Info:            h.cfg.Info,
		Capabilities:    caps,
	})
	if err != nil {
		return fmt.Errorf("host: initialize: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("host: initialize: %s", resp.Error.Message)
	}

	// Authenticate.
	if h.cfg.Token != "" {
		resp, err := h.conn.Call(ctx, rpc.MethodAuthenticate, authParams{
			Method:      "token",
			Credentials: h.cfg.Token,
		})
		if err != nil {
			return fmt.Errorf("host: authenticate: %w", err)
		}
		if resp.Error != nil {
			return fmt.Errorf("host: authenticate: %s", resp.Error.Message)
		}
	}

	return nil
}

// NewSession creates a new session with the agent.
func (h *Host) NewSession(ctx context.Context, p session.Params) (sess session.Session, err error) {
	ctx, end := trace(ctx, client, "session.new")
	defer end(&err)

	r, err := rpc.Invoke[session.Result](ctx, h.conn, rpc.MethodSessionNew, p)
	if err != nil {
		return session.Session{}, err
	}
	h.mu.Lock()
	h.session = r.Session.ID
	h.mu.Unlock()
	return r.Session, nil
}

// LoadSession restores a previous session.
func (h *Host) LoadSession(ctx context.Context, p session.LoadParams) (sess session.Session, err error) {
	ctx, end := trace(ctx, client, "session.load")
	defer end(&err)

	r, err := rpc.Invoke[session.LoadResult](ctx, h.conn, rpc.MethodSessionLoad, p)
	if err != nil {
		return session.Session{}, err
	}
	h.mu.Lock()
	h.session = r.Session.ID
	h.mu.Unlock()
	return r.Session, nil
}

// Prompt sends content to the agent and blocks until the turn completes.
func (h *Host) Prompt(ctx context.Context, sessionID string, blocks []content.Block) (res prompt.Result, err error) {
	ctx, end := trace(ctx, client, "prompt", attribute.String("acp.session_id", sessionID))
	defer end(&err)

	return rpc.Invoke[prompt.Result](ctx, h.conn, rpc.MethodSessionPrompt, prompt.Params{
		SessionID: sessionID,
		Content:   blocks,
	})
}

// PromptCommand sends a slash command prompt turn.
func (h *Host) PromptCommand(ctx context.Context, sessionID string, cmd config.Command) (res prompt.Result, err error) {
	ctx, end := trace(ctx, client, "prompt_command", attribute.String("acp.session_id", sessionID))
	defer end(&err)

	return rpc.Invoke[prompt.Result](ctx, h.conn, rpc.MethodSessionPrompt, prompt.Params{
		SessionID: sessionID,
		Command:   &cmd,
	})
}

// Cancel sends a cancellation notification for an in-progress prompt.
func (h *Host) Cancel(ctx context.Context, sessionID string) (err error) {
	ctx, end := trace(ctx, client, "cancel", attribute.String("acp.session_id", sessionID))
	defer end(&err)

	return h.conn.Notify(ctx, rpc.MethodSessionCancel, session.Cancel{SessionID: sessionID})
}

// SetMode changes the session mode (deprecated).
func (h *Host) SetMode(ctx context.Context, p config.ModeParams) (res config.ModeResult, err error) {
	ctx, end := trace(ctx, client, "set_mode")
	defer end(&err)

	return rpc.Invoke[config.ModeResult](ctx, h.conn, rpc.MethodSetMode, p)
}

// SetOption changes a config option value.
func (h *Host) SetOption(ctx context.Context, p config.SetParams) (res config.SetResult, err error) {
	ctx, end := trace(ctx, client, "set_option")
	defer end(&err)

	return rpc.Invoke[config.SetResult](ctx, h.conn, rpc.MethodSetConfig, p)
}

// --- handler registration ---

func (h *Host) register() {
	if h.cfg.Permission != nil {
		h.conn.Handle(rpc.MethodRequestPermission, h.permission)
	}
	if h.cfg.FS != nil {
		f := &files{h: h}
		h.conn.Handle(rpc.MethodReadFile, f.read)
		h.conn.Handle(rpc.MethodWriteFile, f.write)
	}
	if h.cfg.Terminal != nil {
		t := &terminals{h: h}
		h.conn.Handle(rpc.MethodTerminalCreate, t.create)
		h.conn.Handle(rpc.MethodTerminalOutput, t.output)
		h.conn.Handle(rpc.MethodTerminalWait, t.wait)
		h.conn.Handle(rpc.MethodTerminalKill, t.kill)
		h.conn.Handle(rpc.MethodTerminalRelease, t.release)
	}
	if h.cfg.OnUpdate != nil {
		h.conn.HandleNotify(rpc.MethodSessionUpdate, h.update)
	}
}

// --- internal handshake types ---

type initParams struct {
	ProtocolVersion int            `json:"protocolVersion"`
	Info            acp.Info       `json:"clientInfo"`
	Capabilities    map[string]any `json:"capabilities"`
}

type authParams struct {
	Method      string `json:"method"`
	Credentials any    `json:"credentials,omitempty"`
}

// --- request handlers ---

func (h *Host) activeSession() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.session
}

func (h *Host) permission(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := trace(ctx, server, "permission")
	defer end(&err)

	var p tool.Permission
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid permission params"}
	}
	return h.cfg.Permission(ctx, p)
}

func (h *Host) update(ctx context.Context, n *rpc.Notification) {
	raw, _ := json.Marshal(n.Params)
	var u update.Update
	if err := json.Unmarshal(raw, &u); err != nil {
		return
	}
	h.cfg.OnUpdate(ctx, u)
}
