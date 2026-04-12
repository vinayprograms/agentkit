// Package agent provides the agent-side ACP implementation.
//
// Create an agent with New, then call Run to serve the host:
//
//	srv := agent.New(agent.Config{
//	    Info:    acp.Info{Name: "my-agent", Version: "1.0"},
//	    Handler: &myAgent{},
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
	"go.opentelemetry.io/otel/attribute"
)

// Handler is the core agent behavior. Implement this to handle prompts.
type Handler interface {
	Prompt(ctx context.Context, turn *Turn) (prompt.Result, error)
}

// Capabilities declares which content types the agent can process.
type Capabilities struct {
	Image bool
	Audio bool
}

// Config specifies all agent behavior. Handler is required.
type Config struct {
	Info         acp.Info
	Capabilities Capabilities
	Handler      Handler

	// Optional hooks (nil = default behavior).
	Auth        func(ctx context.Context, token string) error
	NewSession  func(ctx context.Context, p session.Params) (session.Result, error)
	LoadSession func(ctx context.Context, p session.LoadParams) (session.LoadResult, error)
	Cancel      func(ctx context.Context, sessionID string)
	SetMode     func(ctx context.Context, p config.ModeParams) (config.ModeResult, error)
	SetOption   func(ctx context.Context, p config.SetParams) (config.SetResult, error)
}

// Turn represents one active prompt turn. Passed to Handler.Prompt.
// All methods call the host over the active connection.
type Turn struct {
	Params  prompt.Params
	Session session.Session
	conn    *rpc.Conn
}

// ReadFile asks the host to read a file.
func (t *Turn) ReadFile(ctx context.Context, p fs.ReadParams) (fs.ReadResult, error) {
	return rpc.Invoke[fs.ReadResult](ctx, t.conn, rpc.MethodReadFile, p)
}

// WriteFile asks the host to write a file.
func (t *Turn) WriteFile(ctx context.Context, p fs.WriteParams) (fs.WriteResult, error) {
	return rpc.Invoke[fs.WriteResult](ctx, t.conn, rpc.MethodWriteFile, p)
}

// CreateTerminal asks the host to launch a terminal process. Returns the terminal ID.
func (t *Turn) CreateTerminal(ctx context.Context, p terminal.Create) (string, error) {
	r, err := rpc.Invoke[terminal.Created](ctx, t.conn, rpc.MethodTerminalCreate, p)
	if err != nil {
		return "", err
	}
	return r.TerminalID, nil
}

// TerminalOutput reads buffered output from a terminal.
func (t *Turn) TerminalOutput(ctx context.Context, terminalID string) (terminal.Result, error) {
	return rpc.Invoke[terminal.Result](ctx, t.conn, rpc.MethodTerminalOutput, terminal.Ref{TerminalID: terminalID})
}

// TerminalWait blocks until the terminal process exits.
func (t *Turn) TerminalWait(ctx context.Context, terminalID string) (terminal.Result, error) {
	return rpc.Invoke[terminal.Result](ctx, t.conn, rpc.MethodTerminalWait, terminal.Ref{TerminalID: terminalID})
}

// TerminalKill forcibly terminates a terminal process.
func (t *Turn) TerminalKill(ctx context.Context, terminalID string) error {
	_, err := rpc.Invoke[terminal.Result](ctx, t.conn, rpc.MethodTerminalKill, terminal.Ref{TerminalID: terminalID})
	return err
}

// TerminalRelease releases the host's terminal handle.
func (t *Turn) TerminalRelease(ctx context.Context, terminalID string) error {
	_, err := rpc.Invoke[terminal.Result](ctx, t.conn, rpc.MethodTerminalRelease, terminal.Ref{TerminalID: terminalID})
	return err
}

// RequestPermission asks the host to display a permission dialog.
func (t *Turn) RequestPermission(ctx context.Context, p tool.Permission) (tool.Approval, error) {
	return rpc.Invoke[tool.Approval](ctx, t.conn, rpc.MethodRequestPermission, p)
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

// New creates an Agent. Panics if Config.Handler is nil.
func New(cfg Config) *Agent {
	if cfg.Handler == nil {
		panic("agent: Config.Handler is required")
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

// --- internal handshake types ---

type initResult struct {
	ProtocolVersion int            `json:"protocolVersion"`
	Info            acp.Info       `json:"agentInfo"`
	Capabilities    map[string]any `json:"capabilities"`
}

type authParams struct {
	Credentials any `json:"credentials,omitempty"`
}

// --- handlers ---

func (a *Agent) handleInit(_ context.Context, _ *rpc.Request) (any, error) {
	return initResult{
		ProtocolVersion: 1,
		Info:            a.cfg.Info,
		Capabilities: map[string]any{
			"promptCapabilities": map[string]any{
				"image": a.cfg.Capabilities.Image,
				"audio": a.cfg.Capabilities.Audio,
			},
		},
	}, nil
}

func (a *Agent) handleAuth(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := startServerSpan(ctx, "acp.agent.auth")
	defer end(&err)

	if a.cfg.Auth == nil {
		return struct{}{}, nil
	}
	var p authParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid auth params"}
	}
	token, _ := p.Credentials.(string)
	if err := a.cfg.Auth(ctx, token); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrInternal, Message: err.Error()}
	}
	return struct{}{}, nil
}

func (a *Agent) handleNewSession(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := startServerSpan(ctx, "acp.agent.session.new")
	defer end(&err)

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
	a.nextID++
	a.session = session.Session{
		ID:       fmt.Sprintf("session-%d", a.nextID),
		Metadata: p.Metadata,
	}
	return session.Result{Session: a.session}, nil
}

func (a *Agent) handleLoadSession(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := startServerSpan(ctx, "acp.agent.session.load")
	defer end(&err)

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

func (a *Agent) handlePrompt(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := startServerSpan(ctx, "acp.agent.prompt",
		attribute.String("acp.session_id", a.session.ID),
	)
	defer end(&err)

	var p prompt.Params
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid prompt params"}
	}
	return a.cfg.Handler.Prompt(ctx, &Turn{
		Params:  p,
		Session: a.session,
		conn:    a.conn,
	})
}

func (a *Agent) handleCancel(ctx context.Context, n *rpc.Notification) {
	var err error
	ctx, end := startServerSpan(ctx, "acp.agent.cancel")
	defer end(&err)

	if a.cfg.Cancel == nil {
		return
	}
	var p session.Cancel
	raw, _ := json.Marshal(n.Params)
	json.Unmarshal(raw, &p)
	a.cfg.Cancel(ctx, p.SessionID)
}

func (a *Agent) handleSetMode(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := startServerSpan(ctx, "acp.agent.set_mode")
	defer end(&err)

	var p config.ModeParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid mode params"}
	}
	return a.cfg.SetMode(ctx, p)
}

func (a *Agent) handleSetOption(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := startServerSpan(ctx, "acp.agent.set_option")
	defer end(&err)

	var p config.SetParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid config params"}
	}
	return a.cfg.SetOption(ctx, p)
}
