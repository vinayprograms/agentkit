package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"


)

// Sentinel errors.
var (
	ErrClosed  = errors.New("rpc: connection closed")
	ErrStarted = errors.New("rpc: cannot register after Run")
)

// RequestHandler handles an inbound JSON-RPC request.
// Return a result to send a success response. Return an *Error for a
// protocol error, or any other error for an internal error response.
type RequestHandler func(ctx context.Context, req *Request) (any, error)

// NotifyHandler handles an inbound JSON-RPC notification.
// No response is sent. Errors are the handler's responsibility.
type NotifyHandler func(ctx context.Context, n *Notification)

// Conn is a bidirectional JSON-RPC 2.0 connection.
// Both sides of ACP (agent and host) use a Conn to send and receive
// requests, responses, and notifications concurrently.
//
// Register handlers before calling Run. Call and Notify may be called
// concurrently from any goroutine after Run starts.
type Conn struct {
	r io.Reader
	w io.Writer

	mu     sync.Mutex // serializes writes
	nextID atomic.Int64

	handlers map[string]RequestHandler
	notif    map[string]NotifyHandler

	pending sync.Map // id string → chan *Response
	started atomic.Bool
	done    chan struct{}
}

// NewConn creates a connection over the given reader and writer.
func NewConn(r io.Reader, w io.Writer) *Conn {
	return &Conn{
		r:        r,
		w:        w,
		handlers: make(map[string]RequestHandler),
		notif:    make(map[string]NotifyHandler),
		done:     make(chan struct{}),
	}
}

// Handle registers a handler for inbound requests with the given method.
// Must be called before Run.
func (c *Conn) Handle(method string, h RequestHandler) error {
	if c.started.Load() {
		return ErrStarted
	}
	c.handlers[method] = h
	return nil
}

// HandleNotify registers a handler for inbound notifications with the given method.
// Must be called before Run.
func (c *Conn) HandleNotify(method string, h NotifyHandler) error {
	if c.started.Load() {
		return ErrStarted
	}
	c.notif[method] = h
	return nil
}

// Call sends a request and blocks until a response is received, the context
// is cancelled, or the connection closes. The caller inspects resp.Error
// for protocol-level errors.
func (c *Conn) Call(ctx context.Context, method string, params any) (*Response, error) {
	select {
	case <-c.done:
		return nil, ErrClosed
	default:
	}

	id := fmt.Sprintf("%d", c.nextID.Add(1))

	raw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("rpc: marshal params: %w", err)
	}

	ch := make(chan *Response, 1)
	c.pending.Store(id, ch)
	defer c.pending.Delete(id)

	if err := c.send(Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  raw,
	}); err != nil {
		return nil, err
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-c.done:
		return nil, ErrClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Notify sends a notification (fire-and-forget). Returns an error only
// if serialization or the write fails.
func (c *Conn) Notify(ctx context.Context, method string, params any) error {
	select {
	case <-c.done:
		return ErrClosed
	default:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return c.send(Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}

// Run starts the read loop. It blocks until the reader reaches EOF,
// the context is cancelled, or an I/O error occurs. All registered
// request handlers run in their own goroutines so the read loop is
// never blocked by slow handlers (and handlers can Call back without
// deadlocking).
//
// If the reader implements io.Closer, it is closed when the context
// is cancelled to unblock the scanner.
func (c *Conn) Run(ctx context.Context) error {
	c.started.Store(true)
	defer close(c.done)

	// Unblock the scanner on context cancellation.
	if rc, ok := c.r.(io.Closer); ok {
		go func() {
			select {
			case <-ctx.Done():
				rc.Close()
			case <-c.done:
			}
		}()
	}

	scanner := bufio.NewScanner(c.r)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		c.dispatch(ctx, line)
	}

	// Distinguish context cancellation from I/O errors.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return scanner.Err()
}

// dispatch determines the message type and routes it.
func (c *Conn) dispatch(ctx context.Context, data []byte) {
	var probe struct {
		ID     json.RawMessage `json:"id,omitempty"`
		Method string          `json:"method,omitempty"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		c.respondError(nil, ErrParse, "parse error")
		return
	}

	hasID := len(probe.ID) > 0 && string(probe.ID) != "null"
	hasMethod := probe.Method != ""

	switch {
	case hasID && !hasMethod:
		// Response — correlate with pending Call.
		var resp Response
		if err := json.Unmarshal(data, &resp); err != nil {
			return
		}
		idStr := fmt.Sprintf("%v", resp.ID)
		if val, ok := c.pending.Load(idStr); ok {
			ch := val.(chan *Response)
			select {
			case ch <- &resp:
			default:
			}
		}

	case hasID && hasMethod:
		// Request — dispatch to handler in a goroutine.
		var req Request
		if err := json.Unmarshal(data, &req); err != nil {
			c.respondError(nil, ErrParse, "parse error")
			return
		}
		go c.handleRequest(ctx, &req)

	case !hasID && hasMethod:
		// Notification — dispatch to handler in a goroutine.
		var n Notification
		if err := json.Unmarshal(data, &n); err != nil {
			return // notifications have no error response
		}
		go c.handleNotify(ctx, &n)
	}
}

func (c *Conn) handleRequest(ctx context.Context, req *Request) {
	h, ok := c.handlers[req.Method]
	if !ok {
		c.respondError(req.ID, ErrNoMethod, "method not found")
		return
	}

	result, err := h(ctx, req)
	if err != nil {
		var acpErr *Error
		if errors.As(err, &acpErr) {
			c.respondError(req.ID, acpErr.Code, acpErr.Message)
		} else {
			c.respondError(req.ID, ErrInternal, err.Error())
		}
		return
	}

	c.send(Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	})
}

func (c *Conn) handleNotify(ctx context.Context, n *Notification) {
	h, ok := c.notif[n.Method]
	if !ok {
		return // silently drop per JSON-RPC spec
	}
	h(ctx, n)
}

func (c *Conn) respondError(id any, code int, message string) {
	c.send(Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &Error{Code: code, Message: message},
	})
}

// Invoke sends a typed JSON-RPC request and unmarshals the result into T.
// Combines Call + error check + unmarshal in one step.
func Invoke[T any](ctx context.Context, c *Conn, method string, params any) (T, error) {
	var zero T
	resp, err := c.Call(ctx, method, params)
	if err != nil {
		return zero, err
	}
	if resp.Error != nil {
		return zero, resp.Error
	}
	raw, err := json.Marshal(resp.Result)
	if err != nil {
		return zero, fmt.Errorf("rpc: marshal result: %w", err)
	}
	var result T
	if err := json.Unmarshal(raw, &result); err != nil {
		return zero, fmt.Errorf("rpc: unmarshal result: %w", err)
	}
	return result, nil
}

func (c *Conn) send(msg any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("rpc: marshal: %w", err)
	}

	_, err = fmt.Fprintf(c.w, "%s\n", data)
	return err
}
