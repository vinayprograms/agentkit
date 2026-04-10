package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vinayprograms/agentkit/credentials"
)

// --- Engine tests via httptest ---

func TestSearXNG_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "SearXNG Result", "url": "https://searxng.example.com", "content": "snippet"},
			},
		})
	}))
	defer server.Close()

	engine := &searxngEngine{baseURL: server.URL, client: server.Client()}
	results, err := engine.Search(context.Background(), "test", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Title != "SearXNG Result" {
		t.Errorf("unexpected results: %v", results)
	}
}

func TestSearXNG_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	engine := &searxngEngine{baseURL: server.URL, client: server.Client()}
	_, err := engine.Search(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected error")
	}
}

func TestSearXNG_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	engine := &searxngEngine{baseURL: server.URL, client: server.Client()}
	_, err := engine.Search(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestBrave_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"web": map[string]any{
				"results": []map[string]any{
					{"title": "Brave Result", "url": "https://brave.example.com", "description": "snippet"},
				},
			},
		})
	}))
	defer server.Close()

	engine := &braveEngine{apiKey: "test-key", client: &http.Client{Transport: rewriteTransport{url: server.URL}}}
	results, err := engine.Search(context.Background(), "test", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Title != "Brave Result" {
		t.Errorf("unexpected results: %v", results)
	}
}

func TestBrave_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	engine := &braveEngine{apiKey: "test-key", client: &http.Client{Transport: rewriteTransport{url: server.URL}}}
	_, err := engine.Search(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestTavily_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Tavily Result", "url": "https://tavily.example.com", "content": "snippet"},
			},
		})
	}))
	defer server.Close()

	engine := &tavilyEngine{apiKey: "test-key", client: &http.Client{Transport: rewriteTransport{url: server.URL}}}
	results, err := engine.Search(context.Background(), "test", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Title != "Tavily Result" {
		t.Errorf("unexpected results: %v", results)
	}
}

func TestTavily_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	engine := &tavilyEngine{apiKey: "test-key", client: &http.Client{Transport: rewriteTransport{url: server.URL}}}
	_, err := engine.Search(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDuckDuckGo_Search(t *testing.T) {
	html := `<html><body>
		<a class="result__a" href="https://example.com/page1">Result One</a>
		<a class="result__snippet" href="#">First snippet</a>
	</body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, html)
	}))
	defer server.Close()

	engine := &duckDuckGoEngine{client: &http.Client{Transport: rewriteTransport{url: server.URL}}}
	results, err := engine.Search(context.Background(), "test", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) < 1 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Title != "Result One" {
		t.Errorf("expected 'Result One', got %q", results[0].Title)
	}
}

func TestDuckDuckGo_RateLimit_Retries(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= 1 {
			w.WriteHeader(429)
			return
		}
		fmt.Fprint(w, `<a class="result__a" href="https://example.com">OK</a>`)
	}))
	defer server.Close()

	engine := &duckDuckGoEngine{client: &http.Client{Transport: rewriteTransport{url: server.URL}}}
	results, err := engine.Search(context.Background(), "test", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) < 1 {
		t.Error("expected results after retry")
	}
}

func TestDuckDuckGo_AllRetriesFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	}))
	defer server.Close()

	engine := &duckDuckGoEngine{client: &http.Client{Transport: rewriteTransport{url: server.URL}}}
	_, err := engine.Search(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected error after all retries")
	}
	if !strings.Contains(err.Error(), "retries") {
		t.Errorf("expected 'retries' in error, got %q", err.Error())
	}
}

// --- Fallback behavior ---

func TestSearch_Fallback(t *testing.T) {
	// First engine fails, second succeeds
	failing := &mockEngine{err: fmt.Errorf("down")}
	succeeding := &mockEngine{results: []searchResult{{Title: "Fallback", URL: "https://fallback.com"}}}

	tool := &webSearchTool{engines: []SearchEngine{failing, succeeding}}
	args, _ := Validate(tool.Parameters(), map[string]any{"query": "test"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Fallback") {
		t.Errorf("expected fallback result, got %q", result)
	}
}

func TestSearch_AllFail(t *testing.T) {
	tool := &webSearchTool{engines: []SearchEngine{
		&mockEngine{err: fmt.Errorf("engine1 down")},
		&mockEngine{err: fmt.Errorf("engine2 down")},
	}}
	args, _ := Validate(tool.Parameters(), map[string]any{"query": "test"})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error when all engines fail")
	}
	if !strings.Contains(err.Error(), "all search engines failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Tool-level tests ---

func TestSearch_ViaExecute_WithCreds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Tavily Result", "url": "https://tavily.example.com", "content": "snippet"},
			},
		})
	}))
	defer server.Close()

	creds := &mockCreds{keys: map[string]string{"tavily": "test-key"}}
	tool := Search(creds)
	// Override the client on the first engine (tavily) to redirect to test server
	tool.(*webSearchTool).engines[0].(*tavilyEngine).client = &http.Client{Transport: rewriteTransport{url: server.URL}}

	args, _ := Validate(tool.Parameters(), map[string]any{"query": "test"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Tavily Result") {
		t.Errorf("expected Tavily result, got %q", result)
	}
}

func TestSearch_NameAndDescription(t *testing.T) {
	tool := Search(nil)
	if tool.Name() != "web_search" {
		t.Errorf("expected 'web_search', got %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestSearch_Parameters(t *testing.T) {
	tool := Search(nil)
	params := tool.Parameters()
	if _, ok := params["query"]; !ok {
		t.Error("expected 'query' parameter")
	}
	if _, ok := params["count"]; !ok {
		t.Error("expected 'count' parameter")
	}
}

func TestSearch_CountClamping(t *testing.T) {
	engine := &mockEngine{results: []searchResult{{Title: "R", URL: "https://r.com"}}}
	tool := &webSearchTool{engines: []SearchEngine{engine}}

	// count > 10 clamped to 10
	args, _ := Validate(tool.Parameters(), map[string]any{"query": "test", "count": 20})
	tool.Execute(context.Background(), args)

	// count < 1 clamped to 1
	args, _ = Validate(tool.Parameters(), map[string]any{"query": "test", "count": 0})
	tool.Execute(context.Background(), args)
}

// --- HTML parsing ---

func TestParseDuckDuckGoHTML(t *testing.T) {
	html := `
	<div class="result">
		<a rel="nofollow" class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fpage&rut=abc">Example Title</a>
		<a class="result__snippet">This is a snippet.</a>
	</div>
	<div class="result">
		<a rel="nofollow" class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fgolang.org%2Fdoc&rut=def">Go Documentation</a>
		<a class="result__snippet">Official Go docs.</a>
	</div>
	`
	results := parseDuckDuckGoHTML(html, 5)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "Example Title" {
		t.Errorf("expected 'Example Title', got %q", results[0].Title)
	}
	if results[0].URL != "https://example.com/page" {
		t.Errorf("expected URL, got %q", results[0].URL)
	}
}

func TestParseDuckDuckGoHTML_NoResults(t *testing.T) {
	results := parseDuckDuckGoHTML("<html><body></body></html>", 5)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestParseDuckDuckGoHTML_CountLimit(t *testing.T) {
	var html string
	for i := range 10 {
		html += fmt.Sprintf(`<a class="result__a" href="https://example.com/%d">Title %d</a>`, i, i)
	}
	results := parseDuckDuckGoHTML(html, 3)
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestParseDuckDuckGoHTML_SkipsNonHTTP(t *testing.T) {
	html := `<a class="result__a" href="javascript:void(0)">Bad</a>
	          <a class="result__a" href="https://good.com">Good</a>`
	results := parseDuckDuckGoHTML(html, 5)
	if len(results) != 1 || results[0].URL != "https://good.com" {
		t.Errorf("expected only https result, got %v", results)
	}
}

func TestFormatSearchResults_Empty(t *testing.T) {
	result := formatSearchResults(nil)
	if result != "No results found." {
		t.Errorf("expected 'No results found.', got %q", result)
	}
}

func TestFormatSearchResults(t *testing.T) {
	results := []searchResult{
		{Title: "First", URL: "https://first.com", Snippet: "snippet1"},
		{Title: "Second", URL: "https://second.com"},
	}
	formatted := formatSearchResults(results)
	if !strings.Contains(formatted, "1. First") {
		t.Error("expected numbered first result")
	}
	if !strings.Contains(formatted, "2. Second") {
		t.Error("expected numbered second result")
	}
	if !strings.Contains(formatted, "snippet1") {
		t.Error("expected snippet")
	}
}

func TestDecodeURLComponent(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"https%3A%2F%2Fexample.com", "https://example.com"},
		{"path%3Fquery%3Dvalue", "path?query=value"},
	}
	for _, tt := range tests {
		result, _ := decodeURLComponent(tt.input)
		if result != tt.expected {
			t.Errorf("decodeURLComponent(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestDecodeHTMLEntities(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"Hello &amp; World", "Hello & World"},
		{"&lt;tag&gt;", "<tag>"},
		{"It&#39;s a test", "It's a test"},
	}
	for _, tt := range tests {
		result := decodeHTMLEntities(tt.input)
		if result != tt.expected {
			t.Errorf("decodeHTMLEntities(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestStripHTMLTags(t *testing.T) {
	input := "Hello <b>world</b> and <a href='x'>link</a>!"
	expected := "Hello world and link!"
	result := stripHTMLTags(input)
	if result != expected {
		t.Errorf("stripHTMLTags(%q) = %q, want %q", input, result, expected)
	}
}

// --- Test helpers ---

type mockEngine struct {
	results []searchResult
	err     error
}

func (m *mockEngine) Search(ctx context.Context, query string, count int) ([]searchResult, error) {
	return m.results, m.err
}

type mockCreds struct {
	keys map[string]string
}

func (m *mockCreds) Get(provider string) credentials.Credential {
	return credentials.Credential(m.keys[provider])
}

func (m *mockCreds) Providers() []string {
	var p []string
	for k := range m.keys {
		p = append(p, k)
	}
	return p
}

type rewriteTransport struct {
	url string
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.url, "http://")
	return http.DefaultTransport.RoundTrip(req)
}
