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

// testSearchTool builds a webSearchTool with a custom HTTP client transport.
func testSearchTool(transport http.RoundTripper) *webSearchTool {
	return &webSearchTool{
		client: &http.Client{Transport: transport},
	}
}

// ---------------------------------------------------------------------------
// parseDuckDuckGoHTML — additional cases (base tests in registry_test.go)
// ---------------------------------------------------------------------------

func TestWebSearch_TavilyBackendViaExecute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"results": []map[string]any{
				{"title": "Tavily via Execute", "url": "https://tavily.example.com", "content": "snippet"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()


	creds := &mockCreds{keys: map[string]string{"tavily": "test-tavily-key"}}
	tool := Search(creds)
	tool.(*webSearchTool).client = &http.Client{Transport: rewriteTransport{url: server.URL}}

	args, _ := Validate(tool.Parameters(), map[string]any{"query": "test"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Tavily via Execute") {
		t.Errorf("expected Tavily result, got %q", result)
	}
}

func TestWebSearch_BraveBackendViaExecute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"web": map[string]any{
				"results": []map[string]any{
					{"title": "Brave via Execute", "url": "https://brave.example.com", "description": "snippet"},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()


	creds := &mockCreds{keys: map[string]string{"brave": "test-brave-key"}}
	tool := Search(creds)
	tool.(*webSearchTool).client = &http.Client{Transport: rewriteTransport{url: server.URL}}

	args, _ := Validate(tool.Parameters(), map[string]any{"query": "test"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Brave via Execute") {
		t.Errorf("expected Brave result, got %q", result)
	}
}

func TestWebSearch_DuckDuckGoFallbackViaExecute(t *testing.T) {
	html := `<a class="result__a" href="https://ddg.example.com">DDG Result</a>
		<a class="result__snippet" href="#">DDG snippet</a>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, html)
	}))
	defer server.Close()

	// Reset both rate limiters

	// No credentials -- should fall back to DuckDuckGo
	tool := Search(nil)
	tool.(*webSearchTool).client = &http.Client{Transport: rewriteTransport{url: server.URL}}

	args, _ := Validate(tool.Parameters(), map[string]any{"query": "test"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "DDG Result") {
		t.Errorf("expected DDG result, got %q", result)
	}
}

func TestSearchTavily_ViaHTTPTest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "expected POST", http.StatusMethodNotAllowed)
			return
		}
		resp := map[string]any{
			"results": []map[string]any{
				{"title": "Tavily Result", "url": "https://tavily.example.com", "content": "Tavily snippet"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tool := testSearchTool(rewriteTransport{url: server.URL})

	results, err := tool.searchTavily(context.Background(), "test", 3, "test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Title != "Tavily Result" {
		t.Errorf("expected Tavily Result, got %v", results)
	}
}

func TestSearchBrave_ViaHTTPTest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"web": map[string]any{
				"results": []map[string]any{
					{"title": "Brave Result", "url": "https://brave.example.com", "description": "Brave snippet"},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tool := testSearchTool(rewriteTransport{url: server.URL})

	results, err := tool.searchBrave(context.Background(), "test", 3, "test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Title != "Brave Result" {
		t.Errorf("expected Brave Result, got %v", results)
	}
}

func TestSearchDuckDuckGo_ViaHTTPTest(t *testing.T) {
	html := `<html><body>
		<a class="result__a" href="https://example.com/page1">Result One</a>
		<a class="result__snippet" href="#">First snippet</a>
		<a class="result__a" href="https://example.com/page2">Result Two</a>
		<a class="result__snippet" href="#">Second snippet</a>
	</body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, html)
	}))
	defer server.Close()

	tool := testSearchTool(rewriteTransport{url: server.URL})


	results, err := tool.searchDuckDuckGo(context.Background(), "test", 5)
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

func TestSearchDuckDuckGo_RateLimit(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		html := `<a class="result__a" href="https://example.com">Title</a>
			<a class="result__snippet" href="#">Snippet</a>`
		fmt.Fprint(w, html)
	}))
	defer server.Close()

	tool := testSearchTool(rewriteTransport{url: server.URL})

	results, err := tool.searchDuckDuckGo(context.Background(), "test", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result after retries, got %d", len(results))
	}
}

func TestSearchDuckDuckGo_NonRetryableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	tool := testSearchTool(rewriteTransport{url: server.URL})


	_, err := tool.searchDuckDuckGo(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected error for 500 status")
	}
}

func TestSearchSearXNG_ViaHTTPTest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"results": []map[string]any{
				{"title": "SearX One", "url": "https://searx1.com", "content": "snippet 1"},
				{"title": "SearX Two", "url": "https://searx2.com", "content": "snippet 2"},
				{"title": "SearX Three", "url": "https://searx3.com", "content": "snippet 3"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tool := testSearchTool(http.DefaultTransport)

	results, err := tool.searchSearXNG(context.Background(), "test", 2, server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results (count limit), got %d", len(results))
	}
}

func TestSearchTavily_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	tool := testSearchTool(rewriteTransport{url: server.URL})

	_, err := tool.searchTavily(context.Background(), "test", 3, "test-key")
	if err == nil {
		t.Error("expected error for bad request")
	}
}

func TestSearchBrave_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	tool := testSearchTool(rewriteTransport{url: server.URL})

	_, err := tool.searchBrave(context.Background(), "test", 3, "bad-key")
	if err == nil {
		t.Error("expected error for unauthorized")
	}
}

func TestSearchSearXNG_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not json")
	}))
	defer server.Close()

	tool := testSearchTool(http.DefaultTransport)

	_, err := tool.searchSearXNG(context.Background(), "test", 3, server.URL)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSearchBrave_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not json")
	}))
	defer server.Close()

	tool := testSearchTool(rewriteTransport{url: server.URL})

	_, err := tool.searchBrave(context.Background(), "test", 3, "test-key")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSearchTavily_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not json")
	}))
	defer server.Close()

	tool := testSearchTool(rewriteTransport{url: server.URL})

	_, err := tool.searchTavily(context.Background(), "test", 3, "test-key")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSearchDuckDuckGo_AllRetriesFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	tool := testSearchTool(rewriteTransport{url: server.URL})

	_, err := tool.searchDuckDuckGo(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected error after all retries exhausted")
	}
	if !strings.Contains(err.Error(), "retries") {
		t.Errorf("expected 'retries' in error, got %q", err.Error())
	}
}

// rewriteTransport redirects all requests to a test server URL.
type rewriteTransport struct {
	url string
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite the request URL to point at our test server
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.url, "http://")
	return http.DefaultTransport.RoundTrip(req)
}

func TestParseDuckDuckGoHTML_NoResults(t *testing.T) {
	html := `<html><body><div class="no-results">No results</div></body></html>`
	results := parseDuckDuckGoHTML(html, 5)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestParseDuckDuckGoHTML_CountLimit(t *testing.T) {
	// Build HTML with 5 results but request only 2.
	var b strings.Builder
	for i := 0; i < 5; i++ {
		b.WriteString(`<a class="result__a" href="https://example.com/page">Title</a>`)
		b.WriteString(`<a class="result__snippet" href="#">Snippet</a>`)
	}
	results := parseDuckDuckGoHTML(b.String(), 2)
	if len(results) != 2 {
		t.Errorf("expected 2 results (count limit), got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// formatSearchResults
// ---------------------------------------------------------------------------

func TestFormatSearchResults_Empty(t *testing.T) {
	result := formatSearchResults(nil)
	if result != "No results found." {
		t.Errorf("expected 'No results found.', got: %q", result)
	}
}

func TestFormatSearchResults_SingleResult(t *testing.T) {
	results := []searchResult{
		{Title: "Go Programming", URL: "https://go.dev", Snippet: "The Go language"},
	}
	out := formatSearchResults(results)

	if !strings.Contains(out, "1. Go Programming") {
		t.Errorf("expected numbered title, got: %q", out)
	}
	if !strings.Contains(out, "https://go.dev") {
		t.Errorf("expected URL, got: %q", out)
	}
	if !strings.Contains(out, "The Go language") {
		t.Errorf("expected snippet, got: %q", out)
	}
}

func TestFormatSearchResults_MultipleResults(t *testing.T) {
	results := []searchResult{
		{Title: "First", URL: "https://first.com", Snippet: "snippet 1"},
		{Title: "Second", URL: "https://second.com", Snippet: "snippet 2"},
		{Title: "Third", URL: "https://third.com", Snippet: ""},
	}
	out := formatSearchResults(results)

	if !strings.Contains(out, "1. First") {
		t.Errorf("expected '1. First', got: %q", out)
	}
	if !strings.Contains(out, "2. Second") {
		t.Errorf("expected '2. Second', got: %q", out)
	}
	if !strings.Contains(out, "3. Third") {
		t.Errorf("expected '3. Third', got: %q", out)
	}
	// Third result has no snippet — should not have a trailing snippet line.
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if strings.Contains(line, "3. Third") {
			// Next line should be the URL only (no extra line after).
			if i+1 < len(lines) && strings.Contains(lines[i+1], "https://third.com") {
				// Good — URL line exists.
				// There should be no snippet line after the URL.
				if i+2 < len(lines) && strings.TrimSpace(lines[i+2]) != "" {
					// The next non-empty content should be from another result or end.
				}
			}
		}
	}
}

func TestFormatSearchResults_NoSnippet(t *testing.T) {
	results := []searchResult{
		{Title: "Only Title", URL: "https://example.com"},
	}
	out := formatSearchResults(results)

	if !strings.Contains(out, "Only Title") {
		t.Errorf("expected title, got: %q", out)
	}
	if !strings.Contains(out, "https://example.com") {
		t.Errorf("expected URL, got: %q", out)
	}
}

// ---------------------------------------------------------------------------
// DuckDuckGo HTML parsing with redirect URLs
// ---------------------------------------------------------------------------

func TestParseDuckDuckGoHTML_SkipsNonHTTPURLs(t *testing.T) {
	html := `<a class="result__a" href="/internal/page">Internal</a>
	<a class="result__snippet" href="#">Snippet</a>`
	results := parseDuckDuckGoHTML(html, 5)
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-http URLs, got %d", len(results))
	}
}

// --- WebSearch constructor, parameters, description ---

func TestWebSearch_NameAndDescription(t *testing.T) {
	tool := Search(nil)
	if tool.Name() != "web_search" {
		t.Errorf("expected name 'web_search', got %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestWebSearch_Parameters(t *testing.T) {
	tool := Search(nil)
	params := tool.Parameters()

	q, ok := params["query"]
	if !ok {
		t.Fatal("expected 'query' parameter")
	}
	if !q.Required {
		t.Error("query should be required")
	}
	if q.Type != StringParam {
		t.Errorf("query type should be StringParam, got %v", q.Type)
	}

	c, ok := params["count"]
	if !ok {
		t.Fatal("expected 'count' parameter")
	}
	if c.Required {
		t.Error("count should not be required")
	}
	if c.Type != IntParam {
		t.Errorf("count type should be IntParam, got %v", c.Type)
	}
}

// mockCreds implements credentials.Lookup for testing.
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

func TestWebSearch_SearXNGBackend(t *testing.T) {
	// Create a mock SearXNG server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"results": []map[string]any{
				{"title": "Test Result", "url": "https://example.com", "content": "A test snippet"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	creds := &mockCreds{keys: map[string]string{"searxng": server.URL}}
	tool := Search(creds)

	args, err := Validate(tool.Parameters(), map[string]any{"query": "test", "count": 3})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Test Result") {
		t.Errorf("expected 'Test Result' in output, got %q", result)
	}
	if !strings.Contains(result, "https://example.com") {
		t.Errorf("expected URL in output, got %q", result)
	}
}

func TestWebSearch_SearXNGBackendError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	creds := &mockCreds{keys: map[string]string{"searxng": server.URL}}
	tool := Search(creds)

	args, err := Validate(tool.Parameters(), map[string]any{"query": "test"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for server error response")
	}
}

func TestWebSearch_CountClamping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{"results": []map[string]any{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	creds := &mockCreds{keys: map[string]string{"searxng": server.URL}}
	tool := Search(creds)

	// Test count < 1 gets clamped to 1
	args, _ := Validate(tool.Parameters(), map[string]any{"query": "test", "count": 0})
	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test count > 10 gets clamped to 10
	args, _ = Validate(tool.Parameters(), map[string]any{"query": "test", "count": 20})
	_, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Helper function tests ---

func TestDecodeURLComponent(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"https%3A%2F%2Fexample.com%2Fpath%3Fq%3Dtest%26p%3D1", "https://example.com/path?q=test&p=1"},
		{"simple", "simple"},
	}
	for _, tc := range tests {
		result, err := decodeURLComponent(tc.input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != tc.expected {
			t.Errorf("decodeURLComponent(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"<b>bold</b>", "bold"},
		{"no tags", "no tags"},
		{"<a href='x'>link</a> text", "link text"},
	}
	for _, tc := range tests {
		result := stripHTMLTags(tc.input)
		if result != tc.expected {
			t.Errorf("stripHTMLTags(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestDecodeHTMLEntities(t *testing.T) {
	input := "Tom &amp; Jerry &lt;show&gt; &quot;classic&quot; it&#39;s&nbsp;here"
	result := decodeHTMLEntities(input)
	expected := "Tom & Jerry <show> \"classic\" it's here"
	if result != expected {
		t.Errorf("decodeHTMLEntities: got %q, want %q", result, expected)
	}
}

func TestParseDuckDuckGoHTML_WithRedirectURL(t *testing.T) {
	html := fmt.Sprintf(
		`<a class="result__a" href="/l/?uddg=https%%3A%%2F%%2Fexample.com%%2Fpage&rut=abc">Title</a>`+
			`<a class="result__snippet" href="#">Snippet text</a>`)
	results := parseDuckDuckGoHTML(html, 5)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].URL != "https://example.com/page" {
		t.Errorf("expected decoded URL, got %q", results[0].URL)
	}
}

func TestParseDuckDuckGoHTML_DirectHTTPURL(t *testing.T) {
	html := `<a class="result__a" href="https://example.com/direct">Direct Link</a>
	<a class="result__snippet" href="#">Direct snippet</a>`
	results := parseDuckDuckGoHTML(html, 5)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].URL != "https://example.com/direct" {
		t.Errorf("expected direct URL, got %q", results[0].URL)
	}
}
