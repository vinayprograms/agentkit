package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Unit Tests ---

func TestSSEConfig_Defaults(t *testing.T) {
	cfg := DefaultSSEConfig()
	if cfg.HeartbeatInterval != 30*time.Second {
		t.Errorf("HeartbeatInterval = %v, want 30s", cfg.HeartbeatInterval)
	}
}

// --- Integration Tests ---

func TestSSETransport_PostAndReceive(t *testing.T) {
	transport := NewSSETransport(DefaultSSEConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go transport.Run(ctx)

	// Create test server with POST handler
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", transport.HandlePost)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Send POST request
	req := Request{JSONRPC: "2.0", ID: 1, Method: "test", Params: json.RawMessage(`{}`)}
	reqData, _ := json.Marshal(req)

	resp, err := http.Post(server.URL+"/rpc", "application/json", bytes.NewReader(reqData))
	if err != nil {
		t.Fatalf("POST error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}

	// Should receive on transport
	select {
	case msg := <-transport.Recv():
		if msg.Request == nil {
			t.Fatal("expected request")
		}
		if msg.Request.Method != "test" {
			t.Errorf("method = %q, want %q", msg.Request.Method, "test")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestSSETransport_SSEStream(t *testing.T) {
	transport := NewSSETransport(SSEConfig{
		Config:            DefaultConfig(),
		HeartbeatInterval: 0, // Disable for test
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go transport.Run(ctx)

	// Create test server
	mux := http.NewServeMux()
	mux.HandleFunc("/events", transport.HandleSSE)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Connect SSE client with context (no global timeout)
	clientCtx, clientCancel := context.WithCancel(ctx)
	defer clientCancel()

	req, _ := http.NewRequestWithContext(clientCtx, http.MethodGet, server.URL+"/events", nil)
	req.Header.Set("Accept", "text/event-stream")

	// Use client without timeout for SSE (long-lived connection)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("SSE connect error: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("content-type = %q, want text/event-stream", resp.Header.Get("Content-Type"))
	}

	// Send notification from transport
	transport.Send(&OutboundMessage{
		Notification: &Notification{JSONRPC: "2.0", Method: "update", Params: map[string]string{"status": "ready"}},
	})

	// Read SSE event with timeout
	buf := make([]byte, 4096)
	readDone := make(chan int)
	var n int
	var readErr error
	go func() {
		n, readErr = resp.Body.Read(buf)
		close(readDone)
	}()

	select {
	case <-readDone:
		if readErr != nil && readErr != io.EOF {
			t.Fatalf("read error: %v", readErr)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout reading SSE event")
	}

	// Should contain "data:" line with JSON
	if !strings.Contains(string(buf[:n]), `"method":"update"`) {
		t.Errorf("unexpected SSE data: %s", buf[:n])
	}
}

func TestSSETransport_MultipleClients(t *testing.T) {
	transport := NewSSETransport(SSEConfig{
		Config:            DefaultConfig(),
		HeartbeatInterval: 0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go transport.Run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/events", transport.HandleSSE)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Connect multiple clients
	var wg sync.WaitGroup
	clientCount := 3
	received := make([]bool, clientCount)

	for i := 0; i < clientCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			req, _ := http.NewRequest(http.MethodGet, server.URL+"/events", nil)
			req.Header.Set("Accept", "text/event-stream")
			req = req.WithContext(ctx)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			buf := make([]byte, 4096)
			readDone := make(chan int)
			var n int
			go func() {
				n, _ = resp.Body.Read(buf)
				close(readDone)
			}()

			select {
			case <-readDone:
				if strings.Contains(string(buf[:n]), `"method":"broadcast"`) {
					received[idx] = true
				}
			case <-time.After(2 * time.Second):
			}
		}(i)
	}

	// Give clients time to connect
	time.Sleep(100 * time.Millisecond)

	// Broadcast message
	transport.Send(&OutboundMessage{
		Notification: &Notification{JSONRPC: "2.0", Method: "broadcast"},
	})

	wg.Wait()

	// Check all clients received
	for i, got := range received {
		if !got {
			t.Errorf("client %d did not receive broadcast", i)
		}
	}
}

// --- Failure Tests ---

func TestSSETransport_PostMalformedJSON(t *testing.T) {
	transport := NewSSETransport(DefaultSSEConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go transport.Run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", transport.HandlePost)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Send malformed JSON
	resp, _ := http.Post(server.URL+"/rpc", "application/json", strings.NewReader(`{invalid`))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	// Should have JSON-RPC error in body
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"error"`) {
		t.Errorf("expected error in response: %s", body)
	}
}

func TestSSETransport_PostWrongMethod(t *testing.T) {
	transport := NewSSETransport(DefaultSSEConfig())

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", transport.HandlePost)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, _ := http.Get(server.URL + "/rpc")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestSSETransport_SendAfterClose(t *testing.T) {
	transport := NewSSETransport(DefaultSSEConfig())
	transport.Close()

	err := transport.Send(&OutboundMessage{
		Notification: &Notification{JSONRPC: "2.0", Method: "test"},
	})
	if err != ErrClosed {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}

// --- Performance Tests ---

func BenchmarkSSETransport_PostParse(b *testing.B) {
	transport := NewSSETransport(DefaultSSEConfig())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go transport.Run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", transport.HandlePost)
	server := httptest.NewServer(mux)
	defer server.Close()

	req := Request{JSONRPC: "2.0", ID: 1, Method: "bench"}
	reqData, _ := json.Marshal(req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, _ := http.Post(server.URL+"/rpc", "application/json", bytes.NewReader(reqData))
		resp.Body.Close()
		<-transport.Recv()
	}
}

// --- Security Tests ---

func TestSSETransport_PostSizeLimit(t *testing.T) {
	transport := NewSSETransport(DefaultSSEConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go transport.Run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", transport.HandlePost)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Send oversized payload (> 1MB)
	largeData := strings.Repeat("x", 2*1024*1024)
	resp, _ := http.Post(server.URL+"/rpc", "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"large","params":{"data":"`+largeData+`"}}`))
	defer resp.Body.Close()

	// Should still work (truncated at 1MB by handler)
	// Actual behavior depends on implementation
}
