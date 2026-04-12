package host

import (
	"context"
	"encoding/json"

	"github.com/vinayprograms/agentkit/acp/internal/rpc"
	"github.com/vinayprograms/agentkit/acp/proto/fs"
)

// files groups file system request handlers.
type files struct{ h *Host }

func (f *files) read(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := trace(ctx, server, "fs.read")
	defer end(&err)

	var p fs.ReadParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid read params"}
	}
	return f.h.cfg.FS.ReadFile(ctx, f.h.activeSession(), p)
}

func (f *files) write(ctx context.Context, req *rpc.Request) (res any, err error) {
	ctx, end := trace(ctx, server, "fs.write")
	defer end(&err)

	var p fs.WriteParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &rpc.Error{Code: rpc.ErrBadParams, Message: "invalid write params"}
	}
	return f.h.cfg.FS.WriteFile(ctx, f.h.activeSession(), p)
}
