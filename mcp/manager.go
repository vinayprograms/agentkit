package mcp

import (
	"context"
	"fmt"
	"sync"

	"github.com/vinayprograms/agentkit/telemetry"
)

// Manager manages multiple MCP server connections.
type Manager struct {
	clients     map[string]Client
	deniedTools map[string]map[string]bool // server -> tool -> denied
	mu          sync.RWMutex
}

// NewManager creates a new MCP manager.
func NewManager() *Manager {
	return &Manager{
		clients:     make(map[string]Client),
		deniedTools: make(map[string]map[string]bool),
	}
}

// Connect connects to a local MCP server via stdio.
func (m *Manager) Connect(ctx context.Context, name string, config ServerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.clients[name]; exists {
		return fmt.Errorf("server %q already connected", name)
	}

	client, err := Stdio(config)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	if err := client.Initialize(ctx); err != nil {
		client.Close()
		return fmt.Errorf("failed to initialize: %w", err)
	}

	if _, err := client.ListTools(ctx); err != nil {
		client.Close()
		return fmt.Errorf("failed to list tools: %w", err)
	}

	m.clients[name] = client
	return nil
}

// Add registers a pre-created Client under the given name.
// Use this for remote MCP servers or custom client implementations.
func (m *Manager) Add(name string, client Client) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.clients[name]; exists {
		return fmt.Errorf("server %q already connected", name)
	}

	m.clients[name] = client
	return nil
}

// SetDeniedTools sets tools to exclude from a server's tool list.
func (m *Manager) SetDeniedTools(server string, tools []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	denied := make(map[string]bool)
	for _, t := range tools {
		denied[t] = true
	}
	m.deniedTools[server] = denied
}

// Disconnect disconnects from an MCP server.
func (m *Manager) Disconnect(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	client, ok := m.clients[name]
	if !ok {
		return fmt.Errorf("server %q not connected", name)
	}

	delete(m.clients, name)
	return client.Close()
}

// ToolWithServer pairs a tool with its server name.
type ToolWithServer struct {
	Server string
	Tool   Tool
}

// AllTools returns all tools from all connected servers, excluding denied tools.
func (m *Manager) AllTools() []ToolWithServer {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var tools []ToolWithServer
	for server, client := range m.clients {
		denied := m.deniedTools[server]
		for _, tool := range client.Tools() {
			if denied != nil && denied[tool.Name] {
				continue
			}
			tools = append(tools, ToolWithServer{
				Server: server,
				Tool:   tool,
			})
		}
	}
	return tools
}

// CallTool calls a tool on a specific server.
func (m *Manager) CallTool(ctx context.Context, server, tool string, args map[string]any) (*Result, error) {
	m.mu.RLock()
	client, ok := m.clients[server]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("server %q not connected", server)
	}

	tracer := telemetry.GetTracer()
	ctx, span := tracer.StartMCPSpan(ctx, server, tool)

	result, err := client.CallTool(ctx, tool, args)

	var resultStr string
	if result != nil && len(result.Content) > 0 {
		resultStr = result.Content[0].Text
	}

	tracer.EndMCPSpan(span, telemetry.MCPSpanOptions{
		Server: server,
		Tool:   tool,
		Args:   args,
		Result: resultStr,
	}, err)

	return result, err
}

// FindTool finds which server has a tool, excluding denied tools.
func (m *Manager) FindTool(name string) (server string, found bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for srv, client := range m.clients {
		denied := m.deniedTools[srv]
		for _, tool := range client.Tools() {
			if tool.Name == name {
				if denied != nil && denied[name] {
					continue
				}
				return srv, true
			}
		}
	}
	return "", false
}

// Close disconnects all servers.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error
	for name, client := range m.clients {
		if err := client.Close(); err != nil {
			lastErr = err
		}
		delete(m.clients, name)
	}
	return lastErr
}

// ServerCount returns the number of connected servers.
func (m *Manager) ServerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients)
}

// Servers returns the names of connected servers.
func (m *Manager) Servers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}
	return names
}
