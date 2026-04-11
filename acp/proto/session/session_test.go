package session

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSessionRoundtrip(t *testing.T) {
	s := Session{ID: "s-1", Metadata: map[string]string{"cwd": "/project"}}
	data, _ := json.Marshal(s)

	var got Session
	json.Unmarshal(data, &got)
	if got.ID != "s-1" || got.Metadata["cwd"] != "/project" {
		t.Errorf("roundtrip: %+v", got)
	}
}

func TestParamsRoundtrip(t *testing.T) {
	p := Params{
		Cwd:      "/project",
		Metadata: map[string]string{"k": "v"},
		MCP: []MCPServer{{
			Name: "fs",
			Transport: MCPTransport{
				Type: "stdio", Command: "mcp-fs", Args: []string{"/"},
			},
		}},
	}
	data, _ := json.Marshal(p)

	var got Params
	json.Unmarshal(data, &got)
	if got.Cwd != "/project" || len(got.MCP) != 1 || got.MCP[0].Name != "fs" {
		t.Errorf("roundtrip: %+v", got)
	}
}

func TestParamsOmitempty(t *testing.T) {
	p := Params{Cwd: "/"}
	data, _ := json.Marshal(p)
	raw := string(data)
	for _, key := range []string{`"metadata":`, `"mcpServers":`, `"_meta":`} {
		if strings.Contains(raw, key) {
			t.Errorf("should omit %s, got %s", key, raw)
		}
	}
}

func TestCancelRoundtrip(t *testing.T) {
	c := Cancel{SessionID: "s-1"}
	data, _ := json.Marshal(c)

	if !strings.Contains(string(data), `"sessionId":"s-1"`) {
		t.Errorf("expected sessionId: %s", data)
	}
}

func TestMCPTransportHTTP(t *testing.T) {
	tr := MCPTransport{
		Type:    "http",
		URL:     "https://mcp.example.com",
		Headers: map[string]string{"Authorization": "Bearer tok"},
	}
	data, _ := json.Marshal(tr)
	raw := string(data)

	if !strings.Contains(raw, `"url":"https://mcp.example.com"`) {
		t.Errorf("expected URL: %s", raw)
	}
	if !strings.Contains(raw, `"Authorization"`) {
		t.Errorf("expected headers: %s", raw)
	}
}
