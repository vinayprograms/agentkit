// Package mcp provides MCP (Model Context Protocol) client support.
// MCP allows connecting to external tool servers over stdio or HTTP.
//
// # Connect → Register → Deny lifecycle
//
// The intended pattern is to connect a client, register it with a Manager, then
// narrow its exposed tools with Deny. Deny is a denylist applied after the
// server advertises its tools, so it composes naturally with a policy probe:
// list the server's tools, decide which to exclude, and Deny the rest.
//
//	client, err := mcp.Stdio(ctx, cfg) // or mcp.HTTP(ctx, cfg)
//	if err != nil {
//		return err
//	}
//	if err := mgr.Register("filesystem", client); err != nil {
//		return err
//	}
//
//	// Probe the advertised tools and exclude everything the policy disallows.
//	var deny []string
//	for _, t := range client.Tools() {
//		if !pol.IsToolEnabled("filesystem:" + t.Name) {
//			deny = append(deny, t.Name)
//		}
//	}
//	mgr.Deny("filesystem", deny)
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// Client is the interface for communicating with an MCP server.
// Clients are ready to use after creation — Stdio() and HTTP() handle
// initialization, including loading the server's tool list.
type Client interface {
	// Tools returns the server's tools. The client loads them at connection
	// and serves them from there; whether a given call hits the network or a
	// cache is the client's own concern, not the caller's.
	Tools() []Tool

	// CallTool invokes a tool on the server.
	CallTool(ctx context.Context, name string, args map[string]any) (*Result, error)

	// Close shuts down the connection.
	Close() error
}

// Tool represents an MCP tool definition.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Result is the outcome of a tool call.
type Result struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError"`
}

// Content represents content in a tool result.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Data string `json:"data,omitempty"` // base64 for images
}

// ServerConfig configures a local MCP server connection (stdio transport).
type ServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// HTTPConfig configures a remote MCP server connection (Streamable HTTP transport).
type HTTPConfig struct {
	Endpoint string `json:"endpoint"`
}

// --- JSON-RPC types (internal to MCP) ---

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

type toolsListResult struct {
	Tools []Tool `json:"tools"`
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}
