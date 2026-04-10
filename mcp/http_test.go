package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mcpServer creates a test server that handles MCP JSON-RPC requests.
func mcpServer(tools []Tool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)

		var result any
		switch req.Method {
		case "initialize", "notifications/initialized":
			result = map[string]any{"protocolVersion": "2024-11-05"}
		case "tools/list":
			result = toolsListResult{Tools: tools}
		case "tools/call":
			result = Result{Content: []Content{{Type: "text", Text: "tool output"}}}
		default:
			json.NewEncoder(w).Encode(rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &rpcError{Code: -32601, Message: "Method not found"},
			})
			return
		}

		json.NewEncoder(w).Encode(rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  mustMarshal(result),
		})
	}))
}

func TestHTTPClient_CreateAndListTools(t *testing.T) {
	server := mcpServer([]Tool{
		{Name: "read", Description: "Read a file"},
		{Name: "write", Description: "Write a file"},
	})
	defer server.Close()

	client, err := HTTP(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("HTTP: %v", err)
	}
	defer client.Close()

	tools := client.Tools()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
}

func TestHTTPClient_CallTool(t *testing.T) {
	server := mcpServer([]Tool{{Name: "read", Description: "Read"}})
	defer server.Close()

	client, err := HTTP(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("HTTP: %v", err)
	}
	defer client.Close()

	result, err := client.CallTool(context.Background(), "read", map[string]any{"path": "/test"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.Content[0].Text != "tool output" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestHTTPClient_RefreshTools(t *testing.T) {
	server := mcpServer([]Tool{{Name: "read", Description: "Read"}})
	defer server.Close()

	client, err := HTTP(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("HTTP: %v", err)
	}
	defer client.Close()

	// ListTools refreshes the cache
	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}
}

func TestHTTPClient_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	_, err := HTTP(context.Background(), server.URL)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestHTTPClient_RPCError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)
		json.NewEncoder(w).Encode(rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: "Method not found"},
		})
	}))
	defer server.Close()

	_, err := HTTP(context.Background(), server.URL)
	if err == nil {
		t.Error("expected RPC error")
	}
}

func TestHTTPClient_ConnectionError(t *testing.T) {
	_, err := HTTP(context.Background(), "http://localhost:1")
	if err == nil {
		t.Error("expected connection error")
	}
}

func mustMarshal(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
