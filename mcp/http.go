package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
)

// httpClient connects to a remote MCP server via HTTP.
// Uses JSON-RPC over HTTP POST (Streamable HTTP transport per MCP spec).
type httpClient struct {
	endpoint string
	client   *http.Client
	id       atomic.Int64
	tools    []Tool
	ready    bool
	mu       sync.Mutex
}

// HTTP creates a ready-to-use Client that connects to a remote MCP server via
// the Streamable HTTP transport. It performs the MCP handshake and discovers
// available tools. Call Close when done.
func HTTP(ctx context.Context, cfg HTTPConfig) (Client, error) {
	c := &httpClient{
		endpoint: cfg.Endpoint,
		client:   &http.Client{},
	}

	if err := c.initialize(ctx); err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}

	if err := c.refreshTools(ctx); err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}

	return c, nil
}

func (c *httpClient) initialize(ctx context.Context) error {
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

	// Send initialized notification
	c.call(ctx, "notifications/initialized", nil)

	c.mu.Lock()
	c.ready = true
	c.mu.Unlock()
	return nil
}

// refreshTools loads the server's tool list into the client's cache.
func (c *httpClient) refreshTools(ctx context.Context) error {
	result, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return err
	}

	var list toolsListResult
	if err := json.Unmarshal(result, &list); err != nil {
		return fmt.Errorf("failed to parse tools list: %w", err)
	}

	c.mu.Lock()
	c.tools = list.Tools
	c.mu.Unlock()
	return nil
}

func (c *httpClient) CallTool(ctx context.Context, name string, args map[string]any) (*Result, error) {

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

func (c *httpClient) Tools() []Tool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tools
}

func (c *httpClient) Close() error {
	return nil
}

func (c *httpClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.id.Add(1)

	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, rpcResp.Error
	}

	return rpcResp.Result, nil
}
