// Package host provides the host-side (editor/IDE) ACP implementation.
//
// Create a host with New, then call Start to connect to an agent:
//
//	h := host.New(host.Config{
//	    Info:       acp.Info{Name: "my-editor", Version: "2.0"},
//	    Permission: myPermHandler,
//	    FS:         myFSHandler,
//	    OnUpdate:   func(ctx context.Context, u update.Update) { ... },
//	})
//	h.Start(ctx, agentStdin, agentStdout)
//	sess, _ := h.NewSession(ctx, session.Params{Cwd: "/project"})
//	result, _ := h.Prompt(ctx, sess.ID, []content.Block{{Type: content.Text, Text: "hello"}})
package host

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

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
)

// FSHandler implements host-mediated file system access.
// Non-nil on Config → ReadTextFile and WriteTextFile advertised.
type FSHandler interface {
	ReadFile(ctx context.Context, sessionID string, p fs.ReadParams) (fs.ReadResult, error)
	WriteFile(ctx context.Context, sessionID string, p fs.WriteParams) (fs.WriteResult, error)
}

// TerminalHandler implements host-mediated terminal management.
// Non-nil on Config → Terminal capability advertised.
type TerminalHandler interface {
	Create(ctx context.Context, sessionID string, p terminal.Create) (string, error)
	Output(ctx context.Context, sessionID, terminalID string) (terminal.Result, error)
	Wait(ctx context.Context, sessionID, terminalID string) (terminal.Result, error)
	Kill(ctx context.Context, sessionID, terminalID string) error
	Release(ctx context.Context, sessionID, terminalID string) error
}

// PermissionHandler handles tool permission requests from the agent.
type PermissionHandler interface {
	Request(ctx context.Context, p tool.Permission) (tool.Approval, error)
}

// Config specifies all host behavior.
type Config struct {
	Info acp.Info

	// Token is sent in the authenticate request. Empty skips authentication.
	Token string

	// Permission handles permission requests from the agent.
	Permission PermissionHandler

	// FS handles file system requests. nil = not advertised.
	FS FSHandler

	// Terminal handles terminal requests. nil = not advertised.
	Terminal TerminalHandler

	// OnUpdate is called for each session/update notification.
	// Called from the dispatch goroutine — must not block.
	// nil drops updates silently.
	OnUpdate func(ctx context.Context, u update.Update)
}

// Session is a host-side handle to an active agent session.
type Session struct {
	ID       string
	Metadata map[string]string
}

// Host is the host-side ACP orchestrator.
type Host struct {
	cfg  Config
	conn *rpc.Conn
}

// New creates a Host from the given config.
func New(cfg Config) *Host {
	return &Host{cfg: cfg}
}

// Start connects to the agent over r/w, performs the initialize/authenticate
// handshake, and returns. The connection runs in the background until ctx
// is cancelled. All other Host methods may be called after Start returns.
func (h *Host) Start(ctx context.Context, r io.Reader, w io.Writer) error {
	h.conn = rpc.NewConn(r, w)
	h.register()

	// Start the read loop in the background.
	go h.conn.Run(ctx)

	// Perform the initialize handshake.
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

	// Authenticate if a token is configured.
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
func (h *Host) NewSession(ctx context.Context, p session.Params) (Session, error) {
	r, err := call[session.Result](ctx, h.conn, rpc.MethodSessionNew, p)
	if err != nil {
		return Session{}, err
	}
	return Session{ID: r.Session.ID, Metadata: r.Session.Metadata}, nil
}

// LoadSession restores a previous session.
func (h *Host) LoadSession(ctx context.Context, p session.LoadParams) (Session, error) {
	r, err := call[session.LoadResult](ctx, h.conn, rpc.MethodSessionLoad, p)
	if err != nil {
		return Session{}, err
	}
	return Session{ID: r.Session.ID, Metadata: r.Session.Metadata}, nil
}

// Prompt sends content to the agent and blocks until the turn completes.
// Updates arrive via Config.OnUpdate during this call.
func (h *Host) Prompt(ctx context.Context, sessionID string, blocks []content.Block) (prompt.Result, error) {
	return call[prompt.Result](ctx, h.conn, rpc.MethodSessionPrompt, prompt.Params{
		SessionID: sessionID,
		Content:   blocks,
	})
}

// PromptCommand sends a slash command prompt turn.
func (h *Host) PromptCommand(ctx context.Context, sessionID string, cmd config.Command) (prompt.Result, error) {
	return call[prompt.Result](ctx, h.conn, rpc.MethodSessionPrompt, prompt.Params{
		SessionID: sessionID,
		Command:   &cmd,
	})
}

// Cancel sends a cancellation notification for an in-progress prompt.
func (h *Host) Cancel(ctx context.Context, sessionID string) error {
	return h.conn.Notify(ctx, rpc.MethodSessionCancel, session.Cancel{SessionID: sessionID})
}

// SetMode changes the session mode (deprecated).
func (h *Host) SetMode(ctx context.Context, p config.ModeParams) (config.ModeResult, error) {
	return call[config.ModeResult](ctx, h.conn, rpc.MethodSetMode, p)
}

// SetOption changes a config option value.
func (h *Host) SetOption(ctx context.Context, p config.SetParams) (config.SetResult, error) {
	return call[config.SetResult](ctx, h.conn, rpc.MethodSetConfig, p)
}

// --- handler registration ---

func (h *Host) register() {
	// Agent → Host requests.
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

	// Agent → Host notifications.
	if h.cfg.OnUpdate != nil {
		h.conn.HandleNotify(rpc.MethodSessionUpdate, h.handleUpdate)
	}
}

// --- internal handshake types ---

type initParams struct {
	ProtocolVersion int            `json:"protocolVersion"`
	Info            acp.Info       `json:"clientInfo"`
	Capabilities    map[string]any `json:"capabilities"`
	Meta            map[string]any `json:"_meta,omitempty"`
}

type authParams struct {
	Method      string         `json:"method"`
	Credentials any            `json:"credentials,omitempty"`
	Meta        map[string]any `json:"_meta,omitempty"`
}

// --- request handlers ---

func (h *Host) handlePermission(ctx context.Context, req *rpc.Request) (any, error) {
	var p tool.Permission
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid permission params"}
	}
	return h.cfg.Permission.Request(ctx, p)
}

func (h *Host) handleReadFile(ctx context.Context, req *rpc.Request) (any, error) {
	var p fs.ReadParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid read params"}
	}
	sessionID := extractSessionID(req)
	return h.cfg.FS.ReadFile(ctx, sessionID, p)
}

func (h *Host) handleWriteFile(ctx context.Context, req *rpc.Request) (any, error) {
	var p fs.WriteParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid write params"}
	}
	sessionID := extractSessionID(req)
	return h.cfg.FS.WriteFile(ctx, sessionID, p)
}

func (h *Host) handleTerminalCreate(ctx context.Context, req *rpc.Request) (any, error) {
	var p terminal.Create
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid terminal create params"}
	}
	sessionID := extractSessionID(req)
	id, err := h.cfg.Terminal.Create(ctx, sessionID, p)
	if err != nil {
		return nil, err
	}
	return terminal.Created{TerminalID: id}, nil
}

func (h *Host) handleTerminalOutput(ctx context.Context, req *rpc.Request) (any, error) {
	var p terminal.Ref
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid terminal params"}
	}
	return h.cfg.Terminal.Output(ctx, extractSessionID(req), p.TerminalID)
}

func (h *Host) handleTerminalWait(ctx context.Context, req *rpc.Request) (any, error) {
	var p terminal.Ref
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid terminal params"}
	}
	return h.cfg.Terminal.Wait(ctx, extractSessionID(req), p.TerminalID)
}

func (h *Host) handleTerminalKill(ctx context.Context, req *rpc.Request) (any, error) {
	var p terminal.Ref
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid terminal params"}
	}
	if err := h.cfg.Terminal.Kill(ctx, extractSessionID(req), p.TerminalID); err != nil {
		return nil, err
	}
	return terminal.Result{}, nil
}

func (h *Host) handleTerminalRelease(ctx context.Context, req *rpc.Request) (any, error) {
	var p terminal.Ref
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid terminal params"}
	}
	if err := h.cfg.Terminal.Release(ctx, extractSessionID(req), p.TerminalID); err != nil {
		return nil, err
	}
	return terminal.Result{}, nil
}

func (h *Host) handleUpdate(ctx context.Context, n *rpc.Notification) {
	raw, _ := json.Marshal(n.Params)
	var u update.Update
	if err := json.Unmarshal(raw, &u); err != nil {
		return
	}
	h.cfg.OnUpdate(ctx, u)
}

// --- helpers ---

// extractSessionID pulls the sessionId from raw request params.
// Returns empty string if not present (best-effort).
func extractSessionID(req *rpc.Request) string {
	var m map[string]json.RawMessage
	json.Unmarshal(req.Params, &m)
	var id string
	if raw, ok := m["sessionId"]; ok {
		json.Unmarshal(raw, &id)
	}
	return id
}

// call sends a JSON-RPC request and unmarshals the result into T.
func call[T any](ctx context.Context, conn *rpc.Conn, method string, params any) (T, error) {
	var zero T
	resp, err := conn.Call(ctx, method, params)
	if err != nil {
		return zero, err
	}
	if resp.Error != nil {
		return zero, resp.Error
	}

	raw, err := json.Marshal(resp.Result)
	if err != nil {
		return zero, fmt.Errorf("host: marshal result: %w", err)
	}

	var result T
	if err := json.Unmarshal(raw, &result); err != nil {
		return zero, fmt.Errorf("host: unmarshal result: %w", err)
	}
	return result, nil
}
