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
	ctx, end := startClientSpan(ctx, "acp.host.session.new")
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
	ctx, end := startClientSpan(ctx, "acp.host.session.load")
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
	ctx, end := startClientSpan(ctx, "acp.host.prompt", attribute.String("acp.session_id", sessionID))
	defer end(&err)

	return rpc.Invoke[prompt.Result](ctx, h.conn, rpc.MethodSessionPrompt, prompt.Params{
		SessionID: sessionID,
		Content:   blocks,
	})
}

// PromptCommand sends a slash command prompt turn.
func (h *Host) PromptCommand(ctx context.Context, sessionID string, cmd config.Command) (res prompt.Result, err error) {
	ctx, end := startClientSpan(ctx, "acp.host.prompt_command", attribute.String("acp.session_id", sessionID))
	defer end(&err)

	return rpc.Invoke[prompt.Result](ctx, h.conn, rpc.MethodSessionPrompt, prompt.Params{
		SessionID: sessionID,
		Command:   &cmd,
	})
}

// Cancel sends a cancellation notification for an in-progress prompt.
func (h *Host) Cancel(ctx context.Context, sessionID string) (err error) {
	ctx, end := startClientSpan(ctx, "acp.host.cancel", attribute.String("acp.session_id", sessionID))
	defer end(&err)

	return h.conn.Notify(ctx, rpc.MethodSessionCancel, session.Cancel{SessionID: sessionID})
}

// SetMode changes the session mode (deprecated).
func (h *Host) SetMode(ctx context.Context, p config.ModeParams) (res config.ModeResult, err error) {
	ctx, end := startClientSpan(ctx, "acp.host.set_mode")
	defer end(&err)

	return rpc.Invoke[config.ModeResult](ctx, h.conn, rpc.MethodSetMode, p)
}

// SetOption changes a config option value.
func (h *Host) SetOption(ctx context.Context, p config.SetParams) (res config.SetResult, err error) {
	ctx, end := startClientSpan(ctx, "acp.host.set_option")
	defer end(&err)

	return rpc.Invoke[config.SetResult](ctx, h.conn, rpc.MethodSetConfig, p)
}

// --- handler registration ---

func (h *Host) register() {
	if h.cfg.Permission != nil {
		h.conn.Handle(rpc.MethodRequestPermission, h.handlePermission)
	}
	if h.cfg.FS != nil {
		h.conn.Handle(rpc.MethodReadFile, h.handleReadFile)
		h.conn.Handle(rpc.MethodWriteFile, h.handleWriteFile)
	}
	if h.cfg.Terminal != nil {
		h.conn.Handle(rpc.MethodTerminalCreate, h.handleTerminalCreate)
		h.conn.Handle(rpc.MethodTerminalOutput, h.handleTerminalOutput)
		h.conn.Handle(rpc.MethodTerminalWait, h.handleTerminalWait)
		h.conn.Handle(rpc.MethodTerminalKill, h.handleTerminalKill)
		h.conn.Handle(rpc.MethodTerminalRelease, h.handleTerminalRelease)
	}
	if h.cfg.OnUpdate != nil {
		h.conn.HandleNotify(rpc.MethodSessionUpdate, h.handleUpdate)
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

func (h *Host) handlePermission(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := startServerSpan(ctx, "acp.host.permission")
	defer end(&err)

	var p tool.Permission
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid permission params"}
	}
	return h.cfg.Permission(ctx, p)
}

func (h *Host) handleReadFile(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := startServerSpan(ctx, "acp.host.fs.read")
	defer end(&err)

	var p fs.ReadParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid read params"}
	}
	return h.cfg.FS.ReadFile(ctx, h.activeSession(), p)
}

func (h *Host) handleWriteFile(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := startServerSpan(ctx, "acp.host.fs.write")
	defer end(&err)

	var p fs.WriteParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid write params"}
	}
	return h.cfg.FS.WriteFile(ctx, h.activeSession(), p)
}

func (h *Host) handleTerminalCreate(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := startServerSpan(ctx, "acp.host.terminal.create")
	defer end(&err)

	var p terminal.Create
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid terminal params"}
	}
	id, err := h.cfg.Terminal.Create(ctx, h.activeSession(), p)
	if err != nil {
		return nil, err
	}
	return terminal.Created{TerminalID: id}, nil
}

func (h *Host) handleTerminalOutput(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := startServerSpan(ctx, "acp.host.terminal.output")
	defer end(&err)

	var p terminal.Ref
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid terminal params"}
	}
	return h.cfg.Terminal.Output(ctx, h.activeSession(), p.TerminalID)
}

func (h *Host) handleTerminalWait(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := startServerSpan(ctx, "acp.host.terminal.wait")
	defer end(&err)

	var p terminal.Ref
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid terminal params"}
	}
	return h.cfg.Terminal.Wait(ctx, h.activeSession(), p.TerminalID)
}

func (h *Host) handleTerminalKill(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := startServerSpan(ctx, "acp.host.terminal.kill")
	defer end(&err)

	var p terminal.Ref
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid terminal params"}
	}
	if err := h.cfg.Terminal.Kill(ctx, h.activeSession(), p.TerminalID); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

func (h *Host) handleTerminalRelease(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := startServerSpan(ctx, "acp.host.terminal.release")
	defer end(&err)

	var p terminal.Ref
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid terminal params"}
	}
	if err := h.cfg.Terminal.Release(ctx, h.activeSession(), p.TerminalID); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

func (h *Host) handleUpdate(ctx context.Context, n *rpc.Notification) {
	raw, _ := json.Marshal(n.Params)
	var u update.Update
	if err := json.Unmarshal(raw, &u); err != nil {
		return
	}
	h.cfg.OnUpdate(ctx, u)
}
