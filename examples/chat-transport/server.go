// Package main demonstrates a WebSocket JSON-RPC chat server using agentkit transport.
//
// This server accepts WebSocket connections and handles chat messages using
// the JSON-RPC 2.0 protocol over agentkit's WebSocket transport.
//
// Run: go run server.go
// Then connect with: go run client.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vinayprograms/agentkit/transport"
)

// ChatServer manages connected clients and broadcasts messages.
type ChatServer struct {
	mu      sync.RWMutex
	clients map[string]*Client
	nextID  int
}

// Client represents a connected chat client.
type Client struct {
	id        string
	username  string
	transport *transport.WebSocketTransport
	cancel    context.CancelFunc
}

// ChatMessage is the payload for chat messages.
type ChatMessage struct {
	From    string `json:"from"`
	Message string `json:"message"`
	Time    string `json:"time"`
}

// JoinParams for the "join" RPC method.
type JoinParams struct {
	Username string `json:"username"`
}

// SendParams for the "send" RPC method.
type SendParams struct {
	Message string `json:"message"`
}

func main() {
	server := &ChatServer{
		clients: make(map[string]*Client),
	}

	upgrader := transport.NewWebSocketUpgrader()

	http.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		server.handleConnection(w, r, upgrader)
	})

	addr := ":8080"
	log.Printf("Chat server starting on ws://localhost%s/chat", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// handleConnection upgrades HTTP to WebSocket and manages the client session.
func (s *ChatServer) handleConnection(w http.ResponseWriter, r *http.Request, upgrader *websocket.Upgrader) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	// Create WebSocket transport with default config
	cfg := transport.DefaultWebSocketConfig()
	cfg.PingInterval = 10 * time.Second
	wt := transport.NewWebSocketTransport(conn, cfg)

	// Create client with unique ID
	s.mu.Lock()
	s.nextID++
	clientID := fmt.Sprintf("client-%d", s.nextID)
	s.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	client := &Client{
		id:        clientID,
		transport: wt,
		cancel:    cancel,
	}

	// Register client
	s.mu.Lock()
	s.clients[clientID] = client
	s.mu.Unlock()

	log.Printf("Client %s connected", clientID)

	// Start message handler in goroutine
	go s.handleMessages(ctx, client)

	// Run transport (blocks until shutdown)
	if err := wt.Run(ctx); err != nil && err != context.Canceled {
		log.Printf("Transport error for %s: %v", clientID, err)
	}

	// Cleanup
	s.mu.Lock()
	delete(s.clients, clientID)
	s.mu.Unlock()

	if client.username != "" {
		s.broadcast(fmt.Sprintf("%s left the chat", client.username))
	}
	log.Printf("Client %s disconnected", clientID)
}

// handleMessages processes incoming JSON-RPC requests from a client.
func (s *ChatServer) handleMessages(ctx context.Context, client *Client) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-client.transport.Recv():
			if !ok {
				return
			}
			s.processMessage(client, msg)
		}
	}
}

// processMessage handles a single incoming message.
func (s *ChatServer) processMessage(client *Client, msg *transport.InboundMessage) {
	if msg.Request == nil {
		return // Ignore notifications for now
	}

	req := msg.Request
	var result interface{}
	var rpcErr *transport.Error

	switch req.Method {
	case "join":
		result, rpcErr = s.handleJoin(client, req.Params)
	case "send":
		result, rpcErr = s.handleSend(client, req.Params)
	case "list":
		result, rpcErr = s.handleList()
	default:
		rpcErr = &transport.Error{
			Code:    transport.MethodNotFound,
			Message: "Method not found",
			Data:    req.Method,
		}
	}

	// Send response
	resp := &transport.OutboundMessage{
		Response: &transport.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
		},
	}
	if rpcErr != nil {
		resp.Response.Error = rpcErr
	} else {
		resp.Response.Result = result
	}
	client.transport.Send(resp)
}

// handleJoin processes a client joining with a username.
func (s *ChatServer) handleJoin(client *Client, params json.RawMessage) (interface{}, *transport.Error) {
	var p JoinParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &transport.Error{Code: transport.InvalidParams, Message: "Invalid params"}
	}
	if p.Username == "" {
		return nil, &transport.Error{Code: transport.InvalidParams, Message: "Username required"}
	}

	client.username = p.Username
	log.Printf("Client %s joined as %s", client.id, p.Username)

	// Broadcast join notification
	s.broadcast(fmt.Sprintf("%s joined the chat", p.Username))

	return map[string]string{"status": "joined", "username": p.Username}, nil
}

// handleSend processes a chat message.
func (s *ChatServer) handleSend(client *Client, params json.RawMessage) (interface{}, *transport.Error) {
	if client.username == "" {
		return nil, &transport.Error{Code: transport.InvalidRequest, Message: "Must join first"}
	}

	var p SendParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &transport.Error{Code: transport.InvalidParams, Message: "Invalid params"}
	}
	if p.Message == "" {
		return nil, &transport.Error{Code: transport.InvalidParams, Message: "Message required"}
	}

	// Broadcast the message to all clients
	chatMsg := ChatMessage{
		From:    client.username,
		Message: p.Message,
		Time:    time.Now().Format("15:04:05"),
	}

	s.broadcastChat(chatMsg, client.id) // Exclude sender

	return map[string]string{"status": "sent"}, nil
}

// handleList returns the list of connected users.
func (s *ChatServer) handleList() (interface{}, *transport.Error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]string, 0)
	for _, c := range s.clients {
		if c.username != "" {
			users = append(users, c.username)
		}
	}
	return map[string][]string{"users": users}, nil
}

// broadcast sends a system notification to all clients.
func (s *ChatServer) broadcast(message string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, c := range s.clients {
		c.transport.Send(&transport.OutboundMessage{
			Notification: &transport.Notification{
				JSONRPC: "2.0",
				Method:  "system",
				Params:  map[string]string{"message": message},
			},
		})
	}
}

// broadcastChat sends a chat message to all clients except the sender.
func (s *ChatServer) broadcastChat(msg ChatMessage, excludeID string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for id, c := range s.clients {
		if id == excludeID {
			continue // Don't echo back to sender
		}
		c.transport.Send(&transport.OutboundMessage{
			Notification: &transport.Notification{
				JSONRPC: "2.0",
				Method:  "chat",
				Params:  msg,
			},
		})
	}
}
