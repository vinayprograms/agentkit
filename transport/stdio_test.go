package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Unit Tests ---

func TestParseInbound_Request(t *testing.T) {
	data := []byte(`{"jsonrpc":"2.0","id":1,"method":"test","params":{"foo":"bar"}}`)
	msg, err := ParseInbound(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Request == nil {
		t.Fatal("expected request, got nil")
	}
	if msg.Request.Method != "test" {
		t.Errorf("method = %q, want %q", msg.Request.Method, "test")
	}
	if msg.Request.ID != float64(1) {
		t.Errorf("id = %v, want 1", msg.Request.ID)
	}
}

func TestParseInbound_Notification(t *testing.T) {
	data := []byte(`{"jsonrpc":"2.0","method":"notify","params":{}}`)
	msg, err := ParseInbound(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Notification == nil {
		t.Fatal("expected notification, got nil")
	}
	if msg.Notification.Method != "notify" {
		t.Errorf("method = %q, want %q", msg.Notification.Method, "notify")
	}
}

func TestParseInbound_InvalidJSON(t *testing.T) {
	data := []byte(`{invalid json}`)
	_, err := ParseInbound(data)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	rpcErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if rpcErr.Code != ParseError {
		t.Errorf("code = %d, want %d", rpcErr.Code, ParseError)
	}
}

func TestParseInbound_WrongVersion(t *testing.T) {
	data := []byte(`{"jsonrpc":"1.0","id":1,"method":"test"}`)
	_, err := ParseInbound(data)
	if err == nil {
		t.Fatal("expected error for wrong version")
	}
	rpcErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if rpcErr.Code != InvalidRequest {
		t.Errorf("code = %d, want %d", rpcErr.Code, InvalidRequest)
	}
}

func TestMarshalOutbound_Response(t *testing.T) {
	msg := &OutboundMessage{
		Response: &Response{
			JSONRPC: "2.0",
			ID:      1,
			Result:  map[string]string{"status": "ok"},
		},
	}
	data, err := MarshalOutbound(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(data, []byte(`"result"`)) {
		t.Errorf("expected result in output: %s", data)
	}
}

func TestMarshalOutbound_Notification(t *testing.T) {
	msg := &OutboundMessage{
		Notification: &Notification{
			JSONRPC: "2.0",
			Method:  "event",
			Params:  map[string]string{"type": "update"},
		},
	}
	data, err := MarshalOutbound(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(data, []byte(`"method":"event"`)) {
		t.Errorf("expected method in output: %s", data)
	}
}

// --- Integration Tests ---

func TestStdioTransport_RoundTrip(t *testing.T) {
	// Create pipes for bidirectional communication
	clientRead, serverWrite := io.Pipe()
	serverRead, clientWrite := io.Pipe()

	transport := NewStdioTransport(serverRead, serverWrite, DefaultConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start transport in background
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		transport.Run(ctx)
	}()

	// Send a request from "client"
	req := Request{JSONRPC: "2.0", ID: 1, Method: "echo", Params: json.RawMessage(`{"msg":"hello"}`)}
	reqData, _ := json.Marshal(req)
	clientWrite.Write(append(reqData, '\n'))

	// Receive on transport
	select {
	case msg := <-transport.Recv():
		if msg.Request == nil {
			t.Fatal("expected request")
		}
		if msg.Request.Method != "echo" {
			t.Errorf("method = %q, want %q", msg.Request.Method, "echo")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}

	// Send response
	transport.Send(&OutboundMessage{
		Response: &Response{
			JSONRPC: "2.0",
			ID:      1,
			Result:  "world",
		},
	})

	// Read response on "client"
	buf := make([]byte, 4096)
	n, _ := clientRead.Read(buf)
	if !bytes.Contains(buf[:n], []byte(`"result":"world"`)) {
		t.Errorf("unexpected response: %s", buf[:n])
	}

	// Cleanup
	cancel()
	clientWrite.Close()
	clientRead.Close()
	wg.Wait()
}

func TestStdioTransport_MultipleMessages(t *testing.T) {
	input := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"m1"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"m2"}` + "\n" +
			`{"jsonrpc":"2.0","id":3,"method":"m3"}` + "\n",
	)
	output := &bytes.Buffer{}

	transport := NewStdioTransport(input, output, DefaultConfig())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	go transport.Run(ctx)

	received := 0
	for received < 3 {
		select {
		case msg, ok := <-transport.Recv():
			if !ok {
				cancel()
				if received < 3 {
					t.Fatalf("channel closed early, received %d messages", received)
				}
				return
			}
			if msg.Request == nil {
				t.Error("expected request")
			}
			received++
		case <-time.After(time.Second):
			t.Fatalf("timeout, received %d messages", received)
		}
	}
}

// --- Failure Tests ---

func TestStdioTransport_SendAfterClose(t *testing.T) {
	transport := NewStdioTransport(
		strings.NewReader(""),
		io.Discard,
		DefaultConfig(),
	)

	transport.Close()

	err := transport.Send(&OutboundMessage{
		Notification: &Notification{JSONRPC: "2.0", Method: "test"},
	})
	if err != ErrClosed {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}

func TestStdioTransport_MalformedInput(t *testing.T) {
	input := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"good"}` + "\n" +
			`{bad json}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"also_good"}` + "\n",
	)
	output := &bytes.Buffer{}

	transport := NewStdioTransport(input, output, DefaultConfig())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	go transport.Run(ctx)

	// Should receive first message
	msg := <-transport.Recv()
	if msg.Request == nil || msg.Request.Method != "good" {
		t.Error("expected first good message")
	}

	// Should receive third message (second was malformed)
	msg = <-transport.Recv()
	if msg.Request == nil || msg.Request.Method != "also_good" {
		t.Error("expected third good message")
	}

	// Check error response was written for malformed message
	if !bytes.Contains(output.Bytes(), []byte(`"error"`)) {
		t.Error("expected error response for malformed input")
	}
}

// --- Performance Tests ---

func BenchmarkParseInbound(b *testing.B) {
	data := []byte(`{"jsonrpc":"2.0","id":1,"method":"benchmark","params":{"data":"test"}}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseInbound(data)
	}
}

func BenchmarkMarshalOutbound(b *testing.B) {
	msg := &OutboundMessage{
		Response: &Response{
			JSONRPC: "2.0",
			ID:      1,
			Result:  map[string]interface{}{"status": "ok", "count": 42},
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MarshalOutbound(msg)
	}
}

func BenchmarkStdioTransport_Throughput(b *testing.B) {
	// Generate test messages
	var msgs bytes.Buffer
	for i := 0; i < b.N; i++ {
		req := Request{JSONRPC: "2.0", ID: i, Method: "bench"}
		data, _ := json.Marshal(req)
		msgs.Write(append(data, '\n'))
	}

	transport := NewStdioTransport(
		bytes.NewReader(msgs.Bytes()),
		io.Discard,
		Config{RecvBufferSize: 1000, SendBufferSize: 1000},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go transport.Run(ctx)

	b.ResetTimer()
	received := 0
	for received < b.N {
		_, ok := <-transport.Recv()
		if !ok {
			break
		}
		received++
	}
}

// --- Security Tests ---

func TestStdioTransport_LargeMessage(t *testing.T) {
	// Create a message near the 1MB limit
	largeData := strings.Repeat("x", 900*1024)
	input := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"large","params":{"data":"` + largeData + `"}}` + "\n",
	)
	output := &bytes.Buffer{}

	transport := NewStdioTransport(input, output, DefaultConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go transport.Run(ctx)

	select {
	case msg := <-transport.Recv():
		if msg.Request == nil {
			t.Fatal("expected request")
		}
		if msg.Request.Method != "large" {
			t.Errorf("method = %q, want %q", msg.Request.Method, "large")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for large message")
	}
}

func TestStdioTransport_EmptyLines(t *testing.T) {
	input := strings.NewReader(
		"\n" +
			`{"jsonrpc":"2.0","id":1,"method":"test"}` + "\n" +
			"\n\n" +
			`{"jsonrpc":"2.0","id":2,"method":"test2"}` + "\n",
	)
	output := &bytes.Buffer{}

	transport := NewStdioTransport(input, output, DefaultConfig())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	go transport.Run(ctx)

	// Should receive both messages, ignoring empty lines
	count := 0
	for count < 2 {
		select {
		case msg, ok := <-transport.Recv():
			if !ok {
				t.Fatalf("channel closed, received %d", count)
			}
			if msg.Request != nil {
				count++
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout, received %d", count)
		}
	}
}
