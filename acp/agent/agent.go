// Package agent provides the agent-side ACP implementation.
//
// Create an agent with New, then call Run to serve the host:
//
//	srv := agent.New(agent.Config{
//	    Info:         acp.Info{Name: "my-agent", Version: "1.0"},
//	    Capabilities: agent.Capabilities{Image: true},
//	    Prompt: func(ctx context.Context, turn *agent.Turn) (prompt.Result, error) {
//	        src, _ := turn.ReadFile(ctx, fs.ReadParams{Path: "/main.go"})
//	        turn.SendUpdate(ctx, update.Update{Type: update.Message, Chunk: src.Content})
//	        return prompt.Result{Reason: prompt.EndTurn}, nil
//	    },
//	})
//	srv.Run(ctx, os.Stdin, os.Stdout)
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/vinayprograms/agentkit/acp"
	"github.com/vinayprograms/agentkit/acp/internal/rpc"
	"github.com/vinayprograms/agentkit/acp/proto/config"
	"github.com/vinayprograms/agentkit/acp/proto/fs"
	"github.com/vinayprograms/agentkit/acp/proto/prompt"
	"github.com/vinayprograms/agentkit/acp/proto/session"
	"github.com/vinayprograms/agentkit/acp/proto/terminal"
	"github.com/vinayprograms/agentkit/acp/proto/tool"
	"github.com/vinayprograms/agentkit/acp/proto/update"
)

// Capabilities declares which content types the agent can process.
// Advertised during the initialize handshake.
type Capabilities struct {
	Image bool
	Audio bool
}

// Config specifies all agent behavior. Prompt is required; all others are optional.
type Config struct {
	Info         acp.Info
	Capabilities Capabilities

	// Auth validates the host's authentication token.
	// nil accepts all tokens.
	Auth func(ctx context.Context, token string) error

	// NewSession handles session/new.
	// nil accepts all sessions with an auto-generated ID.
	NewSession func(ctx context.Context, p session.Params) (session.Result, error)

	// LoadSession handles session/load.
	// nil rejects all load requests.
	LoadSession func(ctx context.Context, p session.LoadParams) (session.LoadResult, error)

	// Prompt handles session/prompt. Required.
	Prompt func(ctx context.Context, turn *Turn) (prompt.Result, error)

	// Cancel handles the session/cancel notification. Optional.
	Cancel func(ctx context.Context, sessionID string)

	// SetMode handles session/set_mode. nil rejects mode changes.
	SetMode func(ctx context.Context, p config.ModeParams) (config.ModeResult, error)

	// SetOption handles session/set_config_option. nil rejects changes.
	SetOption func(ctx context.Context, p config.SetParams) (config.SetResult, error)
}

// Turn represents one active prompt turn. Passed to Config.Prompt.
// All methods call the host over the active connection.
type Turn struct {
	Params  prompt.Params
	Session session.Session
	conn    *rpc.Conn
}

// ReadFile asks the host to read a file.
func (t *Turn) ReadFile(ctx context.Context, p fs.ReadParams) (fs.ReadResult, error) {
	return call[fs.ReadResult](ctx, t.conn, rpc.MethodReadFile, p)
}

// WriteFile asks the host to write a file.
func (t *Turn) WriteFile(ctx context.Context, p fs.WriteParams) (fs.WriteResult, error) {
	return call[fs.WriteResult](ctx, t.conn, rpc.MethodWriteFile, p)
}

// CreateTerminal asks the host to launch a terminal process.
// Returns the terminal ID.
func (t *Turn) CreateTerminal(ctx context.Context, p terminal.Create) (string, error) {
	r, err := call[terminal.Created](ctx, t.conn, rpc.MethodTerminalCreate, p)
	if err != nil {
		return "", err
	}
	return r.TerminalID, nil
}

// TerminalOutput reads buffered output from a terminal.
func (t *Turn) TerminalOutput(ctx context.Context, terminalID string) (terminal.Result, error) {
	return call[terminal.Result](ctx, t.conn, rpc.MethodTerminalOutput, terminal.Ref{TerminalID: terminalID})
}

// TerminalWait blocks until the terminal process exits.
func (t *Turn) TerminalWait(ctx context.Context, terminalID string) (terminal.Result, error) {
	return call[terminal.Result](ctx, t.conn, rpc.MethodTerminalWait, terminal.Ref{TerminalID: terminalID})
}

// TerminalKill forcibly terminates a terminal process.
func (t *Turn) TerminalKill(ctx context.Context, terminalID string) error {
	_, err := call[terminal.Result](ctx, t.conn, rpc.MethodTerminalKill, terminal.Ref{TerminalID: terminalID})
	return err
}

// TerminalRelease releases the host's terminal handle.
func (t *Turn) TerminalRelease(ctx context.Context, terminalID string) error {
	_, err := call[terminal.Result](ctx, t.conn, rpc.MethodTerminalRelease, terminal.Ref{TerminalID: terminalID})
	return err
}

// RequestPermission asks the host to display a permission dialog.
func (t *Turn) RequestPermission(ctx context.Context, p tool.Permission) (tool.Approval, error) {
	return call[tool.Approval](ctx, t.conn, rpc.MethodRequestPermission, p)
}

// SendUpdate sends a session/update notification to the host.
func (t *Turn) SendUpdate(ctx context.Context, u update.Update) error {
	if u.SessionID == "" {
		u.SessionID = t.Session.ID
	}
	return t.conn.Notify(ctx, rpc.MethodSessionUpdate, u)
}

// Agent is the agent-side ACP orchestrator.
type Agent struct {
	cfg     Config
	conn    *rpc.Conn
	session session.Session
	nextID  int
}

// New creates an Agent. Panics if Config.Prompt is nil.
func New(cfg Config) *Agent {
	if cfg.Prompt == nil {
		panic("agent: Config.Prompt is required")
	}
	return &Agent{cfg: cfg}
}

// Run connects over r/w, performs the handshake, and serves requests
// until the context is cancelled or the connection closes.
func (a *Agent) Run(ctx context.Context, r io.Reader, w io.Writer) error {
	a.conn = rpc.NewConn(r, w)
	a.register()
	return a.conn.Run(ctx)
}

func (a *Agent) register() {
	a.conn.Handle(rpc.MethodInitialize, a.handleInit)
	a.conn.Handle(rpc.MethodAuthenticate, a.handleAuth)
	a.conn.Handle(rpc.MethodSessionNew, a.handleNewSession)
	a.conn.Handle(rpc.MethodSessionLoad, a.handleLoadSession)
	a.conn.Handle(rpc.MethodSessionPrompt, a.handlePrompt)
	a.conn.HandleNotify(rpc.MethodSessionCancel, a.handleCancel)

	if a.cfg.SetMode != nil {
		a.conn.Handle(rpc.MethodSetMode, a.handleSetMode)
	}
	if a.cfg.SetOption != nil {
		a.conn.Handle(rpc.MethodSetConfig, a.handleSetOption)
	}
}

// --- internal handshake types (never exported) ---

type initParams struct {
	ProtocolVersion int            `json:"protocolVersion"`
	Info            acp.Info       `json:"clientInfo"`
	Capabilities    map[string]any `json:"capabilities"`
	Meta            map[string]any `json:"_meta,omitempty"`
}

type initResult struct {
	ProtocolVersion int            `json:"protocolVersion"`
	Info            acp.Info       `json:"agentInfo"`
	Capabilities    map[string]any `json:"capabilities"`
	Meta            map[string]any `json:"_meta,omitempty"`
}

type authParams struct {
	Method      string         `json:"method"`
	Credentials any            `json:"credentials,omitempty"`
	Meta        map[string]any `json:"_meta,omitempty"`
}

type authResult struct {
	Meta map[string]any `json:"_meta,omitempty"`
}

// --- handlers ---

func (a *Agent) handleInit(ctx context.Context, req *rpc.Request) (any, error) {
	caps := map[string]any{
		"promptCapabilities": map[string]any{
			"image": a.cfg.Capabilities.Image,
			"audio": a.cfg.Capabilities.Audio,
		},
	}

	return initResult{
		ProtocolVersion: 1,
		Info:            a.cfg.Info,
		Capabilities:    caps,
	}, nil
}

func (a *Agent) handleAuth(ctx context.Context, req *rpc.Request) (any, error) {
	if a.cfg.Auth == nil {
		return authResult{}, nil
	}

	var p authParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid auth params"}
	}

	// Extract token from credentials (string or nested)
	token, _ := p.Credentials.(string)
	if err := a.cfg.Auth(ctx, token); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrInternal, Message: err.Error()}
	}

	return authResult{}, nil
}

func (a *Agent) handleNewSession(ctx context.Context, req *rpc.Request) (any, error) {
	var p session.Params
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid session params"}
	}

	if a.cfg.NewSession != nil {
		result, err := a.cfg.NewSession(ctx, p)
		if err != nil {
			return nil, err
		}
		a.session = result.Session
		return result, nil
	}

	// Default: auto-generate session
	a.nextID++
	a.session = session.Session{
		ID:       fmt.Sprintf("session-%d", a.nextID),
		Metadata: p.Metadata,
	}
	return session.Result{Session: a.session}, nil
}

func (a *Agent) handleLoadSession(ctx context.Context, req *rpc.Request) (any, error) {
	if a.cfg.LoadSession == nil {
		return nil, &rpc.Error{Code: rpc.ErrInternal, Message: "session loading not supported"}
	}

	var p session.LoadParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid load params"}
	}

	result, err := a.cfg.LoadSession(ctx, p)
	if err != nil {
		return nil, err
	}
	a.session = result.Session
	return result, nil
}

func (a *Agent) handlePrompt(ctx context.Context, req *rpc.Request) (any, error) {
	var p prompt.Params
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid prompt params"}
	}

	turn := &Turn{
		Params:  p,
		Session: a.session,
		conn:    a.conn,
	}

	return a.cfg.Prompt(ctx, turn)
}

func (a *Agent) handleCancel(ctx context.Context, n *rpc.Notification) {
	if a.cfg.Cancel == nil {
		return
	}

	var p session.Cancel
	raw, _ := json.Marshal(n.Params)
	json.Unmarshal(raw, &p)
	a.cfg.Cancel(ctx, p.SessionID)
}

func (a *Agent) handleSetMode(ctx context.Context, req *rpc.Request) (any, error) {
	var p config.ModeParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid mode params"}
	}
	return a.cfg.SetMode(ctx, p)
}

func (a *Agent) handleSetOption(ctx context.Context, req *rpc.Request) (any, error) {
	var p config.SetParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid config params"}
	}
	return a.cfg.SetOption(ctx, p)
}

// --- generic caller helper ---

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
		return zero, fmt.Errorf("agent: marshal result: %w", err)
	}

	var result T
	if err := json.Unmarshal(raw, &result); err != nil {
		return zero, fmt.Errorf("agent: unmarshal result: %w", err)
	}
	return result, nil
}
