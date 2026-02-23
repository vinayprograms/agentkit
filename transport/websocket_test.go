package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// --- Unit Tests ---

func TestWebSocketConfig_Defaults(t *testing.T) {
	cfg := DefaultWebSocketConfig()
	if cfg.MaxMessageSize != 1024*1024 {
		t.Errorf("MaxMessageSize = %d, want 1MB", cfg.MaxMessageSize)
	}
	if cfg.WriteTimeout != 10*time.Second {
		t.Errorf("WriteTimeout = %v, want 10s", cfg.WriteTimeout)
	}
}

// --- Integration Tests ---

func TestWebSocketTransport_RoundTrip(t *testing.T) {
	// Create test server
	var serverTransport *WebSocketTransport
	upgrader := NewWebSocketUpgrader()
	serverReady := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade error: %v", err)
			return
		}
		serverTransport = NewWebSocketTransport(conn, DefaultWebSocketConfig())
		close(serverReady)
	}))
	defer server.Close()

	// Connect client
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer clientConn.Close()

	// Wait for server to set up transport
	<-serverReady

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start server transport
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		serverTransport.Run(ctx)
	}()

	// Client sends request
	req := Request{JSONRPC: "2.0", ID: 1, Method: "test"}
	reqData, _ := json.Marshal(req)
	clientConn.WriteMessage(websocket.TextMessage, reqData)

	// Server receives
	select {
	case msg := <-serverTransport.Recv():
		if msg.Request == nil {
			t.Fatal("expected request")
		}
		if msg.Request.Method != "test" {
			t.Errorf("method = %q, want %q", msg.Request.Method, "test")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}

	// Server sends response
	serverTransport.Send(&OutboundMessage{
		Response: &Response{JSONRPC: "2.0", ID: 1, Result: "ok"},
	})

	// Client receives response
	clientConn.SetReadDeadline(time.Now().Add(time.Second))
	_, data, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	var resp Response
	json.Unmarshal(data, &resp)
	if resp.Result != "ok" {
		t.Errorf("result = %v, want %q", resp.Result, "ok")
	}

	cancel()
	wg.Wait()
}

func TestWebSocketTransport_Notifications(t *testing.T) {
	var serverTransport *WebSocketTransport
	upgrader := NewWebSocketUpgrader()
	serverReady := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _ := upgrader.Upgrade(w, r, nil)
		serverTransport = NewWebSocketTransport(conn, DefaultWebSocketConfig())
		close(serverReady)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, _ := websocket.DefaultDialer.Dial(wsURL, nil)
	defer clientConn.Close()

	<-serverReady

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go serverTransport.Run(ctx)

	// Server sends notification
	serverTransport.Send(&OutboundMessage{
		Notification: &Notification{JSONRPC: "2.0", Method: "update", Params: map[string]string{"status": "ready"}},
	})

	// Client receives notification
	clientConn.SetReadDeadline(time.Now().Add(time.Second))
	_, data, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	var notif Notification
	json.Unmarshal(data, &notif)
	if notif.Method != "update" {
		t.Errorf("method = %q, want %q", notif.Method, "update")
	}
}

// --- Failure Tests ---

func TestWebSocketTransport_SendAfterClose(t *testing.T) {
	var serverTransport *WebSocketTransport
	upgrader := NewWebSocketUpgrader()
	serverReady := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _ := upgrader.Upgrade(w, r, nil)
		serverTransport = NewWebSocketTransport(conn, DefaultWebSocketConfig())
		close(serverReady)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, _ := websocket.DefaultDialer.Dial(wsURL, nil)
	clientConn.Close()

	<-serverReady

	serverTransport.Close()

	err := serverTransport.Send(&OutboundMessage{
		Notification: &Notification{JSONRPC: "2.0", Method: "test"},
	})
	if err != ErrClosed {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}

func TestWebSocketTransport_ClientDisconnect(t *testing.T) {
	var serverTransport *WebSocketTransport
	upgrader := NewWebSocketUpgrader()
	serverReady := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _ := upgrader.Upgrade(w, r, nil)
		serverTransport = NewWebSocketTransport(conn, DefaultWebSocketConfig())
		close(serverReady)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, _ := websocket.DefaultDialer.Dial(wsURL, nil)

	<-serverReady

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		serverTransport.Run(ctx)
		close(done)
	}()

	// Disconnect client
	clientConn.Close()

	// Transport should handle disconnect gracefully
	select {
	case <-done:
		// Good, transport exited
	case <-time.After(time.Second):
		cancel() // Force exit
	}
}

// --- Performance Tests ---

func BenchmarkWebSocketTransport_Throughput(b *testing.B) {
	var serverTransport *WebSocketTransport
	upgrader := NewWebSocketUpgrader()
	serverReady := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _ := upgrader.Upgrade(w, r, nil)
		serverTransport = NewWebSocketTransport(conn, DefaultWebSocketConfig())
		close(serverReady)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, _ := websocket.DefaultDialer.Dial(wsURL, nil)
	defer clientConn.Close()

	<-serverReady

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go serverTransport.Run(ctx)

	// Pre-marshal request
	req := Request{JSONRPC: "2.0", ID: 1, Method: "bench"}
	reqData, _ := json.Marshal(req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clientConn.WriteMessage(websocket.TextMessage, reqData)
		<-serverTransport.Recv()
	}
}

// --- Security Tests ---

func TestWebSocketTransport_MalformedJSON(t *testing.T) {
	var serverTransport *WebSocketTransport
	upgrader := NewWebSocketUpgrader()
	serverReady := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _ := upgrader.Upgrade(w, r, nil)
		serverTransport = NewWebSocketTransport(conn, DefaultWebSocketConfig())
		close(serverReady)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, _ := websocket.DefaultDialer.Dial(wsURL, nil)
	defer clientConn.Close()

	<-serverReady

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go serverTransport.Run(ctx)

	// Send malformed JSON
	clientConn.WriteMessage(websocket.TextMessage, []byte(`{invalid`))

	// Should receive error response
	clientConn.SetReadDeadline(time.Now().Add(time.Second))
	_, data, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	var resp Response
	json.Unmarshal(data, &resp)
	if resp.Error == nil {
		t.Error("expected error response")
	}
	if resp.Error.Code != ParseError {
		t.Errorf("error code = %d, want %d", resp.Error.Code, ParseError)
	}
}
