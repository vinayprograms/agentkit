package mcp

import (
	"context"
	"testing"
)

func TestToolDefinition(t *testing.T) {
	tool := Tool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"arg1": map[string]any{
					"type":        "string",
					"description": "First argument",
				},
			},
			"required": []string{"arg1"},
		},
	}

	if tool.Name != "test_tool" {
		t.Errorf("expected name 'test_tool', got %q", tool.Name)
	}
}

func TestServerConfig(t *testing.T) {
	config := ServerConfig{
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
		Env: map[string]string{
			"DEBUG": "true",
		},
	}

	if config.Command != "npx" {
		t.Errorf("expected command 'npx', got %q", config.Command)
	}
	if len(config.Args) != 3 {
		t.Errorf("expected 3 args, got %d", len(config.Args))
	}
}

func TestManager_Empty(t *testing.T) {
	m := NewManager()

	if m.ServerCount() != 0 {
		t.Errorf("expected 0 servers, got %d", m.ServerCount())
	}

	tools := m.AllTools()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestManager_FindTool_NotFound(t *testing.T) {
	m := NewManager()

	server, found := m.FindTool("nonexistent")
	if found {
		t.Errorf("expected tool not found, got server %q", server)
	}
}

func TestManager_Add(t *testing.T) {
	m := NewManager()

	mock := &mockClient{
		tools: []Tool{
			{Name: "read", Description: "Read a file"},
			{Name: "write", Description: "Write a file"},
		},
	}

	err := m.Add("test-server", mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.ServerCount() != 1 {
		t.Errorf("expected 1 server, got %d", m.ServerCount())
	}

	tools := m.AllTools()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
}

func TestManager_Add_Duplicate(t *testing.T) {
	m := NewManager()
	mock := &mockClient{}

	m.Add("srv", mock)
	err := m.Add("srv", mock)
	if err == nil {
		t.Error("expected error for duplicate server")
	}
}

func TestManager_DeniedTools(t *testing.T) {
	m := NewManager()

	mock := &mockClient{
		tools: []Tool{
			{Name: "read", Description: "Read"},
			{Name: "write", Description: "Write"},
			{Name: "delete", Description: "Delete"},
		},
	}

	m.Add("srv", mock)
	m.SetDeniedTools("srv", []string{"delete"})

	tools := m.AllTools()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools (delete denied), got %d", len(tools))
	}

	for _, tw := range tools {
		if tw.Tool.Name == "delete" {
			t.Error("denied tool 'delete' should not appear")
		}
	}
}

func TestManager_FindTool(t *testing.T) {
	m := NewManager()
	mock := &mockClient{
		tools: []Tool{{Name: "search", Description: "Search"}},
	}
	m.Add("srv", mock)

	server, found := m.FindTool("search")
	if !found {
		t.Error("expected to find tool")
	}
	if server != "srv" {
		t.Errorf("expected server 'srv', got %q", server)
	}
}

func TestManager_FindTool_Denied(t *testing.T) {
	m := NewManager()
	mock := &mockClient{
		tools: []Tool{{Name: "search", Description: "Search"}},
	}
	m.Add("srv", mock)
	m.SetDeniedTools("srv", []string{"search"})

	_, found := m.FindTool("search")
	if found {
		t.Error("denied tool should not be found")
	}
}

func TestManager_CallTool(t *testing.T) {
	m := NewManager()
	mock := &mockClient{
		tools:      []Tool{{Name: "read", Description: "Read"}},
		callResult: &Result{Content: []Content{{Type: "text", Text: "file contents"}}},
	}
	m.Add("srv", mock)

	result, err := m.CallTool(context.Background(), "srv", "read", map[string]any{"path": "/test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content[0].Text != "file contents" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestManager_CallTool_NotConnected(t *testing.T) {
	m := NewManager()

	_, err := m.CallTool(context.Background(), "missing", "read", nil)
	if err == nil {
		t.Error("expected error for missing server")
	}
}

func TestManager_Disconnect(t *testing.T) {
	m := NewManager()
	mock := &mockClient{}
	m.Add("srv", mock)

	err := m.Disconnect("srv")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ServerCount() != 0 {
		t.Error("expected 0 servers after disconnect")
	}
	if !mock.closed {
		t.Error("expected Close to be called")
	}
}

func TestManager_Disconnect_NotFound(t *testing.T) {
	m := NewManager()
	err := m.Disconnect("missing")
	if err == nil {
		t.Error("expected error for missing server")
	}
}

func TestManager_Servers(t *testing.T) {
	m := NewManager()
	m.Add("a", &mockClient{})
	m.Add("b", &mockClient{})

	servers := m.Servers()
	if len(servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(servers))
	}
}

func TestManager_Close(t *testing.T) {
	m := NewManager()
	mock1 := &mockClient{}
	mock2 := &mockClient{}
	m.Add("a", mock1)
	m.Add("b", mock2)

	m.Close()

	if !mock1.closed || !mock2.closed {
		t.Error("expected all clients closed")
	}
	if m.ServerCount() != 0 {
		t.Error("expected 0 servers after close")
	}
}

func TestResult(t *testing.T) {
	result := Result{
		Content: []Content{
			{Type: "text", Text: "Hello, world!"},
		},
		IsError: false,
	}

	if len(result.Content) != 1 {
		t.Errorf("expected 1 content item, got %d", len(result.Content))
	}
	if result.Content[0].Text != "Hello, world!" {
		t.Errorf("unexpected content text")
	}
}

func TestRPCError(t *testing.T) {
	err := &rpcError{
		Code:    -32601,
		Message: "Method not found",
	}

	errStr := err.Error()
	if errStr != "RPC error -32601: Method not found" {
		t.Errorf("unexpected error string: %s", errStr)
	}
}

// Integration test — skipped without actual MCP server
func TestStdioClient_Integration(t *testing.T) {
	t.Skip("requires actual MCP server")

	client, err := Stdio(ServerConfig{
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-memory"},
	})
	if err != nil {
		t.Fatalf("Stdio: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	t.Logf("Found %d tools", len(tools))
	for _, tool := range tools {
		t.Logf("  - %s: %s", tool.Name, tool.Description)
	}
}

// --- mock client ---

type mockClient struct {
	tools      []Tool
	callResult *Result
	callErr    error
	closed     bool
}

func (m *mockClient) Initialize(ctx context.Context) error { return nil }

func (m *mockClient) ListTools(ctx context.Context) ([]Tool, error) {
	return m.tools, nil
}

func (m *mockClient) CallTool(ctx context.Context, name string, args map[string]any) (*Result, error) {
	return m.callResult, m.callErr
}

func (m *mockClient) Tools() []Tool { return m.tools }

func (m *mockClient) Close() error {
	m.closed = true
	return nil
}
