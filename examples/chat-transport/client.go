// Package main demonstrates a WebSocket JSON-RPC chat client.
//
// This client connects to the chat server and allows sending/receiving messages
// using the JSON-RPC 2.0 protocol over WebSocket.
//
// Run: go run client.go [username]
// Requires the server to be running: go run server.go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/gorilla/websocket"
)

// JSONRPCRequest represents an outgoing request.
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCResponse represents an incoming response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// JSONRPCNotification represents an incoming notification (no ID).
type JSONRPCNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// ChatMessage matches the server's broadcast format.
type ChatMessage struct {
	From    string `json:"from"`
	Message string `json:"message"`
	Time    string `json:"time"`
}

// SystemMessage for system notifications.
type SystemMessage struct {
	Message string `json:"message"`
}

// ChatClient manages the connection to the chat server.
type ChatClient struct {
	conn     *websocket.Conn
	username string
	reqID    atomic.Int64
	mu       sync.Mutex

	// Pending requests waiting for responses
	pending   map[int64]chan *JSONRPCResponse
	pendingMu sync.Mutex
}

func main() {
	// Get username from args or default
	username := "guest"
	if len(os.Args) > 1 {
		username = os.Args[1]
	}

	// Connect to server
	serverURL := "ws://localhost:8080/chat"
	fmt.Printf("Connecting to %s...\n", serverURL)

	conn, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := &ChatClient{
		conn:     conn,
		username: username,
		pending:  make(map[int64]chan *JSONRPCResponse),
	}

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan struct{})

	// Handle incoming messages
	go client.readLoop(done)

	// Join the chat
	if err := client.join(); err != nil {
		log.Fatalf("Failed to join: %v", err)
	}
	fmt.Printf("\nJoined as '%s'. Type messages and press Enter to send.\n", username)
	fmt.Println("Commands: /list (show users), /quit (exit)\n")

	// Read user input
	inputCh := make(chan string)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			inputCh <- scanner.Text()
		}
		close(inputCh)
	}()

	// Main loop
	for {
		select {
		case <-sigCh:
			fmt.Println("\nDisconnecting...")
			return
		case <-done:
			fmt.Println("Connection closed by server")
			return
		case input, ok := <-inputCh:
			if !ok {
				return
			}
			if quit := client.handleInput(input); quit {
				return
			}
		}
	}
}

// handleInput processes user keyboard input.
func (c *ChatClient) handleInput(input string) bool {
	if input == "" {
		return false
	}

	switch input {
	case "/quit":
		fmt.Println("Goodbye!")
		return true
	case "/list":
		c.listUsers()
	default:
		c.sendMessage(input)
	}
	return false
}

// readLoop reads messages from the WebSocket connection.
func (c *ChatClient) readLoop(done chan struct{}) {
	defer close(done)

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			}
			log.Printf("Read error: %v", err)
			return
		}

		// Try to parse as a response (has id)
		var msg struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		if msg.ID != nil {
			// It's a response to one of our requests
			c.pendingMu.Lock()
			if ch, ok := c.pending[*msg.ID]; ok {
				var resp JSONRPCResponse
				json.Unmarshal(data, &resp)
				ch <- &resp
				delete(c.pending, *msg.ID)
			}
			c.pendingMu.Unlock()
		} else if msg.Method != "" {
			// It's a notification
			c.handleNotification(msg.Method, msg.Params)
		}
	}
}

// handleNotification processes incoming server notifications.
func (c *ChatClient) handleNotification(method string, params json.RawMessage) {
	switch method {
	case "chat":
		var msg ChatMessage
		if err := json.Unmarshal(params, &msg); err == nil {
			fmt.Printf("[%s] %s: %s\n", msg.Time, msg.From, msg.Message)
		}
	case "system":
		var msg SystemMessage
		if err := json.Unmarshal(params, &msg); err == nil {
			fmt.Printf("*** %s ***\n", msg.Message)
		}
	}
}

// request sends a JSON-RPC request and waits for response.
func (c *ChatClient) request(method string, params interface{}) (*JSONRPCResponse, error) {
	id := c.reqID.Add(1)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	// Create response channel
	respCh := make(chan *JSONRPCResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = respCh
	c.pendingMu.Unlock()

	// Send request
	c.mu.Lock()
	err := c.conn.WriteJSON(req)
	c.mu.Unlock()
	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("write error: %w", err)
	}

	// Wait for response (with timeout in production)
	resp := <-respCh
	return resp, nil
}

// join sends a join request to the server.
func (c *ChatClient) join() error {
	resp, err := c.request("join", map[string]string{"username": c.username})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("join failed: %s", resp.Error.Message)
	}
	return nil
}

// sendMessage sends a chat message to the server.
func (c *ChatClient) sendMessage(message string) {
	resp, err := c.request("send", map[string]string{"message": message})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	if resp.Error != nil {
		fmt.Printf("Error: %s\n", resp.Error.Message)
	}
	// Message sent successfully - no echo since server broadcasts to others
}

// listUsers requests the list of connected users.
func (c *ChatClient) listUsers() {
	resp, err := c.request("list", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	if resp.Error != nil {
		fmt.Printf("Error: %s\n", resp.Error.Message)
		return
	}

	var result struct {
		Users []string `json:"users"`
	}
	if err := json.Unmarshal(resp.Result, &result); err == nil {
		fmt.Println("Connected users:")
		for _, u := range result.Users {
			fmt.Printf("  • %s\n", u)
		}
	}
}
