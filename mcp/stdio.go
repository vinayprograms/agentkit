package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
)

// stdioClient connects to a local MCP server via stdin/stdout.
type stdioClient struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	scanner *bufio.Scanner
	mu      sync.Mutex
	id      atomic.Int64
	pending map[int64]chan *rpcResponse
	pendMu  sync.Mutex
	tools   []Tool
	ready   bool
}

// Stdio creates a ready-to-use Client that connects to a local MCP server via stdio.
// It spawns the process, performs the MCP handshake, and discovers available tools.
func Stdio(ctx context.Context, config ServerConfig) (Client, error) {
	cmd := exec.Command(config.Command, config.Args...)

	cmd.Env = os.Environ()
	for k, v := range config.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout: %w", err)
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start server: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	c := &stdioClient{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		scanner: scanner,
		pending: make(map[int64]chan *rpcResponse),
	}

	go c.readResponses()

	if err := c.initialize(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("initialize: %w", err)
	}

	if err := c.refreshTools(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("list tools: %w", err)
	}

	return c, nil
}

func (c *stdioClient) initialize(ctx context.Context) error {
	result, err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "agentkit",
			"version": "1.0.0",
		},
	})
	if err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}
	_ = result

	c.notify("notifications/initialized", nil)
	c.ready = true
	return nil
}

// refreshTools loads the server's tool list into the client's cache.
func (c *stdioClient) refreshTools(ctx context.Context) error {
	result, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return err
	}

	var list toolsListResult
	if err := json.Unmarshal(result, &list); err != nil {
		return fmt.Errorf("failed to parse tools list: %w", err)
	}

	c.tools = list.Tools
	return nil
}

func (c *stdioClient) CallTool(ctx context.Context, name string, args map[string]any) (*Result, error) {

	result, err := c.call(ctx, "tools/call", toolCallParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return nil, err
	}

	var callResult Result
	if err := json.Unmarshal(result, &callResult); err != nil {
		return nil, fmt.Errorf("failed to parse tool result: %w", err)
	}

	return &callResult, nil
}

func (c *stdioClient) Tools() []Tool {
	return c.tools
}

func (c *stdioClient) Close() error {
	c.stdin.Close()
	return c.cmd.Wait()
}

func (c *stdioClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.id.Add(1)

	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	respCh := make(chan *rpcResponse, 1)
	c.pendMu.Lock()
	c.pending[id] = respCh
	c.pendMu.Unlock()

	defer func() {
		c.pendMu.Lock()
		delete(c.pending, id)
		c.pendMu.Unlock()
	}()

	if err := c.send(req); err != nil {
		return nil, err
	}

	select {
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *stdioClient) notify(method string, params any) error {
	req := struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params,omitempty"`
	}{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return c.send(req)
}

func (c *stdioClient) send(msg any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(c.stdin, "%s\n", data)
	return err
}

func (c *stdioClient) readResponses() {
	for c.scanner.Scan() {
		line := c.scanner.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}

		var resp rpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}

		if resp.ID == 0 && resp.Result == nil && resp.Error == nil {
			continue
		}

		c.pendMu.Lock()
		ch, ok := c.pending[resp.ID]
		c.pendMu.Unlock()

		if ok {
			ch <- &resp
		}
	}
}
