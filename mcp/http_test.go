package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClient_Initialize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)

		json.NewEncoder(w).Encode(rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"protocolVersion":"2024-11-05"}`),
		})
	}))
	defer server.Close()

	client, err := HTTP(server.URL)
	if err != nil {
		t.Fatalf("HTTP: %v", err)
	}
	defer client.Close()

	err = client.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
}

func TestHTTPClient_ListTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)

		var result any
		switch req.Method {
		case "initialize", "notifications/initialized":
			result = map[string]any{"protocolVersion": "2024-11-05"}
		case "tools/list":
			result = toolsListResult{
				Tools: []Tool{
					{Name: "read", Description: "Read a file"},
					{Name: "write", Description: "Write a file"},
				},
			}
		}

		json.NewEncoder(w).Encode(rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  mustMarshal(result),
		})
	}))
	defer server.Close()

	client, err := HTTP(server.URL)
	if err != nil {
		t.Fatalf("HTTP: %v", err)
	}
	defer client.Close()

	client.Initialize(context.Background())

	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}

	// Tools() returns cached list
	cached := client.Tools()
	if len(cached) != 2 {
		t.Errorf("expected 2 cached tools, got %d", len(cached))
	}
}

func TestHTTPClient_CallTool(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)

		var result any
		switch req.Method {
		case "initialize", "notifications/initialized":
			result = map[string]any{"protocolVersion": "2024-11-05"}
		case "tools/call":
			result = Result{
				Content: []Content{{Type: "text", Text: "file contents"}},
			}
		}

		json.NewEncoder(w).Encode(rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  mustMarshal(result),
		})
	}))
	defer server.Close()

	client, err := HTTP(server.URL)
	if err != nil {
		t.Fatalf("HTTP: %v", err)
	}
	defer client.Close()

	client.Initialize(context.Background())

	result, err := client.CallTool(context.Background(), "read", map[string]any{"path": "/test"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.Content[0].Text != "file contents" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestHTTPClient_NotInitialized(t *testing.T) {
	client, _ := HTTP("http://localhost:9999")

	_, err := client.ListTools(context.Background())
	if err == nil {
		t.Error("expected error when not initialized")
	}

	_, err = client.CallTool(context.Background(), "read", nil)
	if err == nil {
		t.Error("expected error when not initialized")
	}
}

func TestHTTPClient_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client, _ := HTTP(server.URL)

	err := client.Initialize(context.Background())
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

	client, _ := HTTP(server.URL)

	err := client.Initialize(context.Background())
	if err == nil {
		t.Error("expected RPC error")
	}
}

func TestHTTPClient_ConnectionError(t *testing.T) {
	client, _ := HTTP("http://localhost:1") // nothing listening

	err := client.Initialize(context.Background())
	if err == nil {
		t.Error("expected connection error")
	}
}

func mustMarshal(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
