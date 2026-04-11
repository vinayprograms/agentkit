// Package acp provides Agent Client Protocol support.
// ACP standardizes communication between code editors and coding agents.
package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
)

// Config configures an ACP server.
type Config struct {
	Info         Info
	Capabilities Capabilities

	// Prompt handles incoming prompt requests.
	// Required — Run returns an error for prompts if nil.
	Prompt func(ctx context.Context, req *PromptRequest) (*PromptResponse, error)
}

// Server is an ACP agent server that communicates via JSON-RPC over stdin/stdout.
type Server struct {
	stdin   io.Reader
	stdout  io.Writer
	scanner *bufio.Scanner
	mu      sync.Mutex
	id      atomic.Int64

	prompt      func(ctx context.Context, req *PromptRequest) (*PromptResponse, error)
	session     *Session
	initialized bool
	info        Info
	caps        Capabilities
}

// New creates an ACP server. By default it reads from stdin and writes to stdout.
func New(cfg Config) *Server {
	return &Server{
		stdin:  os.Stdin,
		stdout: os.Stdout,
		info:   cfg.Info,
		caps:   cfg.Capabilities,
		prompt: cfg.Prompt,
	}
}

// Run starts the server loop, reading JSON-RPC messages until EOF or context cancellation.
func (s *Server) Run(ctx context.Context) error {
	s.scanner = bufio.NewScanner(s.stdin)

	for s.scanner.Scan() {
		line := s.scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, -32700, "Parse error", nil)
			continue
		}

		if err := s.handleRequest(ctx, &req); err != nil {
			s.sendError(req.ID, -32603, err.Error(), nil)
		}
	}

	return s.scanner.Err()
}

func (s *Server) handleRequest(ctx context.Context, req *Request) error {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "session/new":
		return s.handleNewSession(req)
	case "session/prompt":
		return s.handlePrompt(ctx, req)
	case "session/cancel":
		return s.sendResult(req.ID, map[string]any{})
	default:
		return s.sendError(req.ID, -32601, "Method not found", nil)
	}
}

func (s *Server) handleInitialize(req *Request) error {
	var params InitializeRequest
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return s.sendError(req.ID, -32602, "Invalid params", nil)
		}
	}

	s.initialized = true

	return s.sendResult(req.ID, InitializeResponse{
		ProtocolVersion: "2025-01-01",
		AgentInfo:       s.info,
		Capabilities:    s.caps,
	})
}

func (s *Server) handleNewSession(req *Request) error {
	var params NewSessionRequest
	if req.Params != nil {
		json.Unmarshal(req.Params, &params)
	}

	s.session = &Session{
		ID:       fmt.Sprintf("session-%d", s.id.Add(1)),
		Metadata: params.Metadata,
	}

	return s.sendResult(req.ID, NewSessionResponse{
		Session: *s.session,
	})
}

func (s *Server) handlePrompt(ctx context.Context, req *Request) error {
	if s.prompt == nil {
		return s.sendError(req.ID, -32603, "no prompt handler", nil)
	}

	var params PromptRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.sendError(req.ID, -32602, "Invalid params", nil)
	}

	resp, err := s.prompt(ctx, &params)
	if err != nil {
		return s.sendError(req.ID, -32603, err.Error(), nil)
	}

	return s.sendResult(req.ID, resp)
}

// SendMessageChunk sends a message chunk notification to the client.
func (s *Server) SendMessageChunk(sessionID, role, chunk string) error {
	return s.sendNotification("session/update", SessionNotification{
		SessionID: sessionID,
		Update: SessionUpdate{
			Type:  "messageChunk",
			Role:  role,
			Chunk: chunk,
		},
	})
}

// SendToolCall sends a tool call update notification to the client.
func (s *Server) SendToolCall(sessionID string, call *ToolCallUpdate) error {
	return s.sendNotification("session/update", SessionNotification{
		SessionID: sessionID,
		Update: SessionUpdate{
			Type:     "toolCall",
			ToolCall: call,
		},
	})
}

func (s *Server) sendResult(id any, result any) error {
	return s.send(Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *Server) sendError(id any, code int, message string, data any) error {
	return s.send(Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &Error{Code: code, Message: message, Data: data},
	})
}

func (s *Server) sendNotification(method string, params any) error {
	return s.send(Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}

func (s *Server) send(msg any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(s.stdout, "%s\n", data)
	return err
}
