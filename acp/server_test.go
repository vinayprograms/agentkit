package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestInfo(t *testing.T) {
	info := Info{Name: "test-agent", Version: "1.0.0"}
	if info.Name != "test-agent" {
		t.Errorf("expected name 'test-agent', got %q", info.Name)
	}
}

func TestCapabilities(t *testing.T) {
	caps := Capabilities{
		LoadSession: true,
		PromptCapabilities: PromptCapabilities{
			Image: true,
			Audio: false,
		},
	}
	if !caps.LoadSession {
		t.Error("expected LoadSession to be true")
	}
	if !caps.PromptCapabilities.Image {
		t.Error("expected Image to be true")
	}
}

func TestSession(t *testing.T) {
	s := Session{
		ID:       "session-123",
		Metadata: map[string]string{"workDir": "/project"},
	}
	if s.ID != "session-123" {
		t.Errorf("expected ID 'session-123', got %q", s.ID)
	}
}

func TestPromptRequest(t *testing.T) {
	req := PromptRequest{
		SessionID: "session-1",
		Prompt:    []PromptPart{{Type: "text", Text: "Hello, agent!"}},
	}
	if len(req.Prompt) != 1 || req.Prompt[0].Text != "Hello, agent!" {
		t.Errorf("unexpected prompt: %+v", req.Prompt)
	}
}

func TestPromptRequestWithCommand(t *testing.T) {
	req := PromptRequest{
		SessionID: "session-1",
		Command:   &CommandInput{Name: "search", Input: "find all TODOs"},
	}
	if req.Command == nil || req.Command.Name != "search" {
		t.Errorf("expected command 'search', got %+v", req.Command)
	}
}

func TestSessionUpdate(t *testing.T) {
	update := SessionUpdate{Type: "messageChunk", Role: "assistant", Chunk: "Hello"}
	if update.Type != "messageChunk" {
		t.Errorf("expected type 'messageChunk', got %q", update.Type)
	}
}

func TestToolCallUpdate(t *testing.T) {
	update := ToolCallUpdate{ID: "call-1", Name: "read_file", Status: "running", Input: "/path"}
	if update.Status != "running" {
		t.Errorf("expected status 'running', got %q", update.Status)
	}
}

func TestErrorCodes(t *testing.T) {
	e := Error{Code: -32601, Message: "Method not found"}
	if e.Code != -32601 {
		t.Errorf("expected code -32601, got %d", e.Code)
	}
}

func TestInitializeRequest(t *testing.T) {
	req := InitializeRequest{
		ProtocolVersion: "2025-01-01",
		ClientInfo:      ClientInfo{Name: "vscode", Version: "1.85.0"},
		Capabilities:    ClientCapabilities{Terminal: true, ReadTextFile: true, WriteTextFile: true},
	}
	if req.ProtocolVersion != "2025-01-01" {
		t.Errorf("unexpected protocol version")
	}
	if !req.Capabilities.Terminal {
		t.Error("expected terminal capability")
	}
}

func TestNotificationMarshal(t *testing.T) {
	notif := Notification{
		JSONRPC: "2.0",
		Method:  "session/update",
		Params: SessionNotification{
			SessionID: "session-1",
			Update:    SessionUpdate{Type: "messageChunk", Chunk: "Hello"},
		},
	}
	data, err := json.Marshal(notif)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "session/update") {
		t.Error("expected method in JSON")
	}
	if !strings.Contains(s, "messageChunk") {
		t.Error("expected update type in JSON")
	}
}

// newTestServer creates a server wired to the given input/output for testing.
func newTestServer(input string, output *bytes.Buffer, prompt func(context.Context, *PromptRequest) (*PromptResponse, error)) *Server {
	return &Server{
		stdin:  strings.NewReader(input),
		stdout: output,
		info:   Info{Name: "test-agent", Version: "1.0.0"},
		caps:   Capabilities{},
		prompt: prompt,
	}
}

func TestNew(t *testing.T) {
	srv := New(Config{
		Info:         Info{Name: "test", Version: "1.0.0"},
		Capabilities: Capabilities{LoadSession: true},
		Prompt:       func(ctx context.Context, req *PromptRequest) (*PromptResponse, error) { return nil, nil },
	})
	if srv == nil {
		t.Fatal("expected server to be created")
	}
}

func TestInitialize(t *testing.T) {
	var out bytes.Buffer
	srv := newTestServer("", &out, nil)

	req := &Request{JSONRPC: "2.0", ID: 1, Method: "initialize"}
	srv.handleRequest(context.Background(), req)

	var resp Response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		lines := strings.Split(out.String(), "\n")
		json.Unmarshal([]byte(lines[0]), &resp)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
	if !srv.initialized {
		t.Error("expected initialized to be true")
	}
}

func TestNewSession(t *testing.T) {
	var out bytes.Buffer
	srv := newTestServer("", &out, nil)
	srv.initialized = true

	req := &Request{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "session/new",
		Params:  json.RawMessage(`{"metadata": {"workDir": "/test"}}`),
	}
	srv.handleRequest(context.Background(), req)

	if srv.session == nil {
		t.Fatal("expected session to be created")
	}
	if srv.session.Metadata["workDir"] != "/test" {
		t.Error("expected workDir metadata to be set")
	}
}

func TestPromptHandler(t *testing.T) {
	var out bytes.Buffer
	handler := func(ctx context.Context, req *PromptRequest) (*PromptResponse, error) {
		return &PromptResponse{StopReason: "endTurn"}, nil
	}
	srv := newTestServer("", &out, handler)

	params, _ := json.Marshal(PromptRequest{
		SessionID: "session-1",
		Prompt:    []PromptPart{{Type: "text", Text: "hello"}},
	})
	req := &Request{JSONRPC: "2.0", ID: 3, Method: "session/prompt", Params: params}
	srv.handleRequest(context.Background(), req)

	var resp Response
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

func TestPromptNoHandler(t *testing.T) {
	var out bytes.Buffer
	srv := newTestServer("", &out, nil)

	params, _ := json.Marshal(PromptRequest{SessionID: "s1", Prompt: []PromptPart{{Type: "text", Text: "hi"}}})
	req := &Request{JSONRPC: "2.0", ID: 4, Method: "session/prompt", Params: params}
	srv.handleRequest(context.Background(), req)

	var resp Response
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error == nil {
		t.Fatal("expected error when no prompt handler")
	}
	if resp.Error.Code != -32603 {
		t.Errorf("expected code -32603, got %d", resp.Error.Code)
	}
}

func TestMethodNotFound(t *testing.T) {
	var out bytes.Buffer
	srv := newTestServer("", &out, nil)

	req := &Request{JSONRPC: "2.0", ID: 5, Method: "unknown/method"}
	srv.handleRequest(context.Background(), req)

	var resp Response
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("expected method not found error, got %+v", resp.Error)
	}
}

func TestSessionCancel(t *testing.T) {
	var out bytes.Buffer
	srv := newTestServer("", &out, nil)

	req := &Request{JSONRPC: "2.0", ID: 6, Method: "session/cancel"}
	srv.handleRequest(context.Background(), req)

	var resp Response
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

func TestSendMessageChunk(t *testing.T) {
	var out bytes.Buffer
	srv := newTestServer("", &out, nil)

	if err := srv.SendMessageChunk("session-1", "assistant", "Hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var notif Notification
	json.Unmarshal(out.Bytes(), &notif)
	if notif.Method != "session/update" {
		t.Errorf("expected method 'session/update', got %q", notif.Method)
	}
}

func TestSendToolCall(t *testing.T) {
	var out bytes.Buffer
	srv := newTestServer("", &out, nil)

	call := &ToolCallUpdate{ID: "call-1", Name: "read_file", Status: "running"}
	if err := srv.SendToolCall("session-1", call); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s := out.String()
	if !strings.Contains(s, "toolCall") {
		t.Error("expected toolCall in output")
	}
}

func TestRun(t *testing.T) {
	initReq, _ := json.Marshal(Request{JSONRPC: "2.0", ID: 1, Method: "initialize"})
	input := string(initReq) + "\n"

	var out bytes.Buffer
	srv := &Server{
		stdin:  strings.NewReader(input),
		stdout: &out,
		info:   Info{Name: "test", Version: "1.0"},
		caps:   Capabilities{},
	}

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !srv.initialized {
		t.Error("expected server to be initialized after Run")
	}
}

func TestRunParseError(t *testing.T) {
	input := "not json\n"
	var out bytes.Buffer
	srv := &Server{
		stdin:  strings.NewReader(input),
		stdout: &out,
		info:   Info{Name: "test", Version: "1.0"},
	}

	srv.Run(context.Background())

	var resp Response
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != -32700 {
		t.Fatalf("expected parse error, got %+v", resp.Error)
	}
}

func TestRunSkipsEmptyLines(t *testing.T) {
	initReq, _ := json.Marshal(Request{JSONRPC: "2.0", ID: 1, Method: "initialize"})
	input := "\n\n" + string(initReq) + "\n\n"

	var out bytes.Buffer
	srv := &Server{
		stdin:  strings.NewReader(input),
		stdout: &out,
		info:   Info{Name: "test", Version: "1.0"},
	}

	srv.Run(context.Background())
	if !srv.initialized {
		t.Error("expected initialization despite empty lines")
	}
}

func TestInitializeWithParams(t *testing.T) {
	var out bytes.Buffer
	srv := newTestServer("", &out, nil)

	params, _ := json.Marshal(InitializeRequest{
		ProtocolVersion: "2025-01-01",
		ClientInfo:      ClientInfo{Name: "vscode", Version: "1.85.0"},
	})
	req := &Request{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: params}
	srv.handleRequest(context.Background(), req)

	var resp Response
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

func TestInitializeInvalidParams(t *testing.T) {
	var out bytes.Buffer
	srv := newTestServer("", &out, nil)

	req := &Request{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: json.RawMessage(`{invalid}`)}
	srv.handleRequest(context.Background(), req)

	var resp Response
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Fatalf("expected invalid params error, got %+v", resp.Error)
	}
}

func TestPromptHandlerError(t *testing.T) {
	var out bytes.Buffer
	srv := newTestServer("", &out, func(ctx context.Context, req *PromptRequest) (*PromptResponse, error) {
		return nil, fmt.Errorf("something went wrong")
	})

	params, _ := json.Marshal(PromptRequest{SessionID: "s1", Prompt: []PromptPart{{Type: "text", Text: "hi"}}})
	req := &Request{JSONRPC: "2.0", ID: 1, Method: "session/prompt", Params: params}
	srv.handleRequest(context.Background(), req)

	var resp Response
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != -32603 {
		t.Fatalf("expected handler error, got %+v", resp.Error)
	}
}

func TestRunHandleRequestError(t *testing.T) {
	// A valid JSON-RPC request with an unknown method triggers handleRequest returning error
	// which causes Run to call sendError with the request ID
	unknownReq, _ := json.Marshal(Request{JSONRPC: "2.0", ID: 99, Method: "bogus/method"})
	input := string(unknownReq) + "\n"

	var out bytes.Buffer
	srv := &Server{
		stdin:  strings.NewReader(input),
		stdout: &out,
		info:   Info{Name: "test", Version: "1.0"},
	}

	srv.Run(context.Background())

	// sendError inside handleRequest already writes the response (method not found),
	// and then returns nil — so Run's own sendError path doesn't fire.
	// But we verify the method-not-found error was written.
	var resp Response
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("expected method not found error, got %+v", resp.Error)
	}
}

func TestPromptInvalidParams(t *testing.T) {
	var out bytes.Buffer
	srv := newTestServer("", &out, func(ctx context.Context, req *PromptRequest) (*PromptResponse, error) {
		return nil, nil
	})

	req := &Request{JSONRPC: "2.0", ID: 1, Method: "session/prompt", Params: json.RawMessage(`{invalid}`)}
	srv.handleRequest(context.Background(), req)

	var resp Response
	json.Unmarshal(out.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Fatalf("expected invalid params error, got %+v", resp.Error)
	}
}
