// Package mcp provides MCP (Model Context Protocol) client support.
// MCP allows connecting to external tool servers over stdio or HTTP.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// Client is the interface for communicating with an MCP server.
type Client interface {
	// Initialize performs the MCP initialization handshake.
	Initialize(ctx context.Context) error

	// ListTools fetches available tools from the server.
	ListTools(ctx context.Context) ([]Tool, error)

	// CallTool invokes a tool on the server.
	CallTool(ctx context.Context, name string, args map[string]any) (*Result, error)

	// Tools returns the cached tool list from the last ListTools call.
	Tools() []Tool

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

// ServerConfig configures a local MCP server connection (stdio).
type ServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
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
