package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vinayprograms/agentkit/llm"
)

// mockLLM implements llm.Model for testing.
type mockLLM struct {
	response string
	err      error
}

func (m *mockLLM) Chat(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &llm.ChatResponse{Content: m.response}, nil
}

// ---------------------------------------------------------------------------
// Successful fetch
// ---------------------------------------------------------------------------

func TestWebFetch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("<html><body><p>Hello World</p></body></html>"))
	}))
	defer srv.Close()

	tool := Fetch(nil, nil)
	args, err := Validate(tool.Parameters(), map[string]any{
		"url":      srv.URL,
		"question": "What does the page say?",
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(result, "Hello World") {
		t.Errorf("expected result to contain 'Hello World', got: %s", result)
	}
}

// ---------------------------------------------------------------------------
// Fetch with summarizer (mock LLM)
// ---------------------------------------------------------------------------

func TestWebFetch_WithSummarizer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("<html><body><p>Go is an open-source programming language.</p></body></html>"))
	}))
	defer srv.Close()

	mock := &mockLLM{response: "Go is an open-source language."}
	summarizer := NewSummarizer(mock)

	tool := Fetch(summarizer, nil)
	args, err := Validate(tool.Parameters(), map[string]any{
		"url":      srv.URL,
		"question": "What is Go?",
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if result != "Go is an open-source language." {
		t.Errorf("expected summarized response, got: %s", result)
	}
}

// ---------------------------------------------------------------------------
// HTTP 404 — returns result, not error
// ---------------------------------------------------------------------------

func TestWebFetch_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte("page not found"))
	}))
	defer srv.Close()

	tool := Fetch(nil, nil)
	args, _ := Validate(tool.Parameters(), map[string]any{
		"url":      srv.URL,
		"question": "anything",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("expected no error for 404, got: %v", err)
	}
	if !strings.Contains(result, "404") {
		t.Errorf("expected result to contain '404', got: %s", result)
	}
	if !strings.Contains(result, "page not found") {
		t.Errorf("expected result to contain body, got: %s", result)
	}
}

// ---------------------------------------------------------------------------
// HTTP 500 — returns result, not error
// ---------------------------------------------------------------------------

func TestWebFetch_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	tool := Fetch(nil, nil)
	args, _ := Validate(tool.Parameters(), map[string]any{
		"url":      srv.URL,
		"question": "anything",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("expected no error for 500, got: %v", err)
	}
	if !strings.Contains(result, "500") {
		t.Errorf("expected result to contain '500', got: %s", result)
	}
	if !strings.Contains(result, "internal server error") {
		t.Errorf("expected result to contain body, got: %s", result)
	}
}

// ---------------------------------------------------------------------------
// Invalid URL — should return error
// ---------------------------------------------------------------------------

func TestWebFetch_InvalidURL(t *testing.T) {
	tool := Fetch(nil, nil)
	args, _ := Validate(tool.Parameters(), map[string]any{
		"url":      "://not-a-url",
		"question": "anything",
	})

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

// ---------------------------------------------------------------------------
// extractReadableText helper
// ---------------------------------------------------------------------------

func TestExtractReadableText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
		excludes []string
	}{
		{
			name:     "strips tags and extracts text",
			input:    "<html><body><p>Hello</p><p>World</p></body></html>",
			contains: []string{"Hello", "World"},
			excludes: []string{"<html>", "<body>", "<p>", "</p>"},
		},
		{
			name:     "removes script blocks",
			input:    `<p>Before</p><script>alert("xss")</script><p>After</p>`,
			contains: []string{"Before", "After"},
			excludes: []string{"alert", "script"},
		},
		{
			name:     "removes style blocks",
			input:    `<p>Content</p><style>body{color:red}</style>`,
			contains: []string{"Content"},
			excludes: []string{"color", "style"},
		},
		{
			name:     "removes nav and footer",
			input:    `<nav>Menu</nav><p>Main</p><footer>Copyright</footer>`,
			contains: []string{"Main"},
			excludes: []string{"Menu", "Copyright"},
		},
		{
			name:     "decodes HTML entities",
			input:    `<p>A &amp; B &lt; C &gt; D &quot;E&quot; &#39;F&#39;</p>`,
			contains: []string{"A & B < C > D \"E\" 'F'"},
		},
		{
			name:     "removes head block",
			input:    `<head><title>Title</title><meta charset="utf-8"></head><body>Body</body>`,
			contains: []string{"Body"},
			excludes: []string{"Title", "meta"},
		},
		{
			name:     "removes HTML comments",
			input:    `<p>Visible</p><!-- hidden comment --><p>Also visible</p>`,
			contains: []string{"Visible", "Also visible"},
			excludes: []string{"hidden comment"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractReadableText(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(result, s) {
					t.Errorf("expected result to contain %q, got: %q", s, result)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(result, s) {
					t.Errorf("expected result NOT to contain %q, got: %q", s, result)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Missing required parameters
// ---------------------------------------------------------------------------

func TestWebFetch_MissingURL(t *testing.T) {
	tool := Fetch(nil, nil)
	_, err := Validate(tool.Parameters(), map[string]any{
		"question": "what?",
	})
	if err == nil {
		t.Fatal("expected validation error for missing url")
	}
}

func TestWebFetch_MissingQuestion(t *testing.T) {
	tool := Fetch(nil, nil)
	_, err := Validate(tool.Parameters(), map[string]any{
		"url": "http://example.com",
	})
	if err == nil {
		t.Fatal("expected validation error for missing question")
	}
}

// ---------------------------------------------------------------------------
// Summarizer returns error — should propagate
// ---------------------------------------------------------------------------

func TestWebFetch_LargeContentReturnsFullText(t *testing.T) {
	bigContent := strings.Repeat("word ", 5000) // 25000 chars
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprintf(w, "<html><body><p>%s</p></body></html>", bigContent)
	}))
	defer srv.Close()

	tool := Fetch(nil, nil) // no summarizer
	args, _ := Validate(tool.Parameters(), map[string]any{
		"url":      srv.URL,
		"question": "anything",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result, "truncated") {
		t.Error("content should not be truncated")
	}
	if len(result) < 20000 {
		t.Errorf("expected full content, got length %d", len(result))
	}
}

func TestWebFetch_ConnectionError(t *testing.T) {
	tool := Fetch(nil, nil)
	args, _ := Validate(tool.Parameters(), map[string]any{
		"url":      "http://localhost:1", // port 1 should refuse connection
		"question": "anything",
	})

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for connection failure")
	}
}

func TestWebFetch_SummarizerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("<html><body><p>Content</p></body></html>"))
	}))
	defer srv.Close()

	mock := &mockLLM{err: fmt.Errorf("LLM unavailable")}
	summarizer := NewSummarizer(mock)

	tool := Fetch(summarizer, nil)
	args, _ := Validate(tool.Parameters(), map[string]any{
		"url":      srv.URL,
		"question": "anything",
	})

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error when summarizer fails")
	}
	if !strings.Contains(err.Error(), "LLM unavailable") {
		t.Errorf("expected error to mention LLM, got: %v", err)
	}
}
