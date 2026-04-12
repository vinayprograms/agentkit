package host

import (
	"context"
	"encoding/json"

	"github.com/vinayprograms/agentkit/acp/internal/rpc"
	"github.com/vinayprograms/agentkit/acp/proto/terminal"
)

// terminals groups terminal request handlers.
type terminals struct{ h *Host }

func (t *terminals) create(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := trace(ctx, server, "terminal.create")
	defer end(&err)

	var p terminal.Create
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid terminal params"}
	}
	id, err := t.h.cfg.Terminal.Create(ctx, t.h.activeSession(), p)
	if err != nil {
		return nil, err
	}
	return terminal.Created{TerminalID: id}, nil
}

func (t *terminals) output(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := trace(ctx, server, "terminal.output")
	defer end(&err)

	var p terminal.Ref
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid terminal params"}
	}
	return t.h.cfg.Terminal.Output(ctx, t.h.activeSession(), p.TerminalID)
}

func (t *terminals) wait(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := trace(ctx, server, "terminal.wait")
	defer end(&err)

	var p terminal.Ref
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid terminal params"}
	}
	return t.h.cfg.Terminal.Wait(ctx, t.h.activeSession(), p.TerminalID)
}

func (t *terminals) kill(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := trace(ctx, server, "terminal.kill")
	defer end(&err)

	var p terminal.Ref
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid terminal params"}
	}
	if err := t.h.cfg.Terminal.Kill(ctx, t.h.activeSession(), p.TerminalID); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

func (t *terminals) release(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := trace(ctx, server, "terminal.release")
	defer end(&err)

	var p terminal.Ref
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid terminal params"}
	}
	if err := t.h.cfg.Terminal.Release(ctx, t.h.activeSession(), p.TerminalID); err != nil {
		return nil, err
	}
	return struct{}{}, nil
}
