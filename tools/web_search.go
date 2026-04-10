package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/vinayprograms/agentkit/credentials"
)

// SearchEngine performs web searches.
type SearchEngine interface {
	Search(ctx context.Context, query string, count int) ([]searchResult, error)
}

// searchResult represents a single search result.
type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

type webSearchTool struct {
	engines []SearchEngine
}

// Search returns a tool that searches the web using available backends.
// Engines are resolved from credentials and environment variables.
// DuckDuckGo is always included as the final fallback.
// On execution, engines are tried in order — if one fails, the next is tried.
func Search(creds credentials.Lookup) Tool {
	return &webSearchTool{
		engines: resolveEngines(creds),
	}
}

func resolveEngines(creds credentials.Lookup) []SearchEngine {
	var engines []SearchEngine
	client := &http.Client{Timeout: defaultHTTPTimeout}

	if creds != nil {
		if url := string(creds.Get("searxng")); url != "" {
			engines = append(engines, &searxngEngine{baseURL: url, client: client})
		}
		if key := string(creds.Get("brave")); key != "" {
			engines = append(engines, &braveEngine{apiKey: key, client: client})
		}
		if key := string(creds.Get("tavily")); key != "" {
			engines = append(engines, &tavilyEngine{apiKey: key, client: client})
		}
	}

	// DuckDuckGo is always the final fallback.
	engines = append(engines, &duckDuckGoEngine{client: client})

	return engines
}

func (t *webSearchTool) Name() string { return "web_search" }

func (t *webSearchTool) Description() string {
	return "Search the web. Returns titles, URLs, and short snippets. " +
		"IMPORTANT: Snippets are brief previews only — use web_fetch on relevant URLs " +
		"to get the full content needed for research. The standard flow is: web_search " +
		"to discover sources, then web_fetch on 2-4 most relevant URLs."
}

func (t *webSearchTool) Parameters() map[string]Param {
	return map[string]Param{
		"query": {
			Type:        StringParam,
			Description: "Search query",
			Required:    true,
		},
		"count": {
			Type:        IntParam,
			Description: "Number of results (1-10, default 5)",
		},
	}
}

func (t *webSearchTool) Execute(ctx context.Context, args Args) (string, error) {
	query, err := args.String("query")
	if err != nil {
		return "", err
	}

	count := args.IntOr("count", 5)
	if count < 1 {
		count = 1
	} else if count > 10 {
		count = 10
	}

	// Try engines in order. If one fails, fall back to the next.
	var lastErr error
	for _, engine := range t.engines {
		results, err := engine.Search(ctx, query, count)
		if err != nil {
			lastErr = err
			continue
		}
		return formatSearchResults(results), nil
	}

	return "", fmt.Errorf("all search engines failed: %w", lastErr)
}

// formatSearchResults turns results into human-readable text.
func formatSearchResults(results []searchResult) string {
	if len(results) == 0 {
		return "No results found."
	}
	var b strings.Builder
	for i, r := range results {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "%d. %s\n   %s", i+1, r.Title, r.URL)
		if r.Snippet != "" {
			fmt.Fprintf(&b, "\n   %s", r.Snippet)
		}
	}
	return b.String()
}

// --- SearXNG ---

type searxngEngine struct {
	baseURL string
	client  *http.Client
}

func (e *searxngEngine) Search(ctx context.Context, query string, count int) ([]searchResult, error) {
	url := fmt.Sprintf("%s/search?q=%s&format=json&categories=general",
		strings.TrimSuffix(e.baseURL, "/"), strings.ReplaceAll(query, " ", "+"))

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "HeadlessAgent/1.0")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searxng: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("searxng error (%d): %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("searxng: invalid response: %w", err)
	}

	results := make([]searchResult, 0, count)
	for i, r := range parsed.Results {
		if i >= count {
			break
		}
		results = append(results, searchResult{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return results, nil
}

// --- Brave ---

type braveEngine struct {
	apiKey string
	client *http.Client
}

func (e *braveEngine) Search(ctx context.Context, query string, count int) ([]searchResult, error) {
	url := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		strings.ReplaceAll(query, " ", "+"), count)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Subscription-Token", e.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("brave error (%d): %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("brave: invalid response: %w", err)
	}

	results := make([]searchResult, 0, len(parsed.Web.Results))
	for _, r := range parsed.Web.Results {
		results = append(results, searchResult{Title: r.Title, URL: r.URL, Snippet: r.Description})
	}
	return results, nil
}

// --- Tavily ---

type tavilyEngine struct {
	apiKey string
	client *http.Client
}

func (e *tavilyEngine) Search(ctx context.Context, query string, count int) ([]searchResult, error) {
	reqBody := map[string]any{
		"api_key":     e.apiKey,
		"query":       query,
		"max_results": count,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.tavily.com/search", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tavily error (%d): %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("tavily: invalid response: %w", err)
	}

	results := make([]searchResult, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		results = append(results, searchResult{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return results, nil
}

// --- DuckDuckGo (fallback) ---

type duckDuckGoEngine struct {
	client     *http.Client
	mu         sync.Mutex
	lastSearch time.Time
}

const (
	ddgCooldown   = 2 * time.Second
	ddgBackoff    = 2 * time.Second
	ddgMaxBackoff = 5 * time.Second
	ddgMaxRetries = 3
)

func (e *duckDuckGoEngine) Search(ctx context.Context, query string, count int) ([]searchResult, error) {
	e.mu.Lock()
	elapsed := time.Since(e.lastSearch)
	if elapsed < ddgCooldown {
		wait := ddgCooldown - elapsed
		e.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		e.mu.Lock()
	}
	e.lastSearch = time.Now()
	e.mu.Unlock()

	searchURL := fmt.Sprintf("https://duckduckgo.com/html/?q=%s",
		strings.ReplaceAll(strings.ReplaceAll(query, " ", "+"), "&", "%26"))

	backoff := ddgBackoff
	var lastErr error

	for attempt := 0; attempt <= ddgMaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > ddgMaxBackoff {
				backoff = ddgMaxBackoff
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "HeadlessAgent/1.0")
		req.Header.Set("Accept", "text/html")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")

		resp, err := e.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("duckduckgo: %w", err)
			continue
		}

		if resp.StatusCode == 202 || resp.StatusCode == 403 || resp.StatusCode == 429 {
			resp.Body.Close()
			lastErr = fmt.Errorf("duckduckgo: rate limited (status %d)", resp.StatusCode)
			continue
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			return nil, fmt.Errorf("duckduckgo error: status %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("duckduckgo: failed to read response: %w", err)
		}

		return parseDuckDuckGoHTML(string(body), count), nil
	}

	return nil, fmt.Errorf("duckduckgo: failed after %d retries: %w", ddgMaxRetries, lastErr)
}

// --- HTML parsing helpers ---

func parseDuckDuckGoHTML(html string, count int) []searchResult {
	var results []searchResult

	linkRe := regexp.MustCompile(`<a[^>]+class="result__a"[^>]+href="([^"]+)"[^>]*>([^<]+)</a>`)
	snippetRe := regexp.MustCompile(`<a[^>]+class="result__snippet"[^>]*>([^<]+(?:<[^>]+>[^<]*</[^>]+>)*[^<]*)</a>`)

	links := linkRe.FindAllStringSubmatch(html, count*2)
	snippets := snippetRe.FindAllStringSubmatch(html, count*2)

	for i := 0; i < len(links) && len(results) < count; i++ {
		url := links[i][1]
		title := strings.TrimSpace(links[i][2])

		if strings.Contains(url, "uddg=") {
			if parts := strings.Split(url, "uddg="); len(parts) > 1 {
				decoded, err := decodeURLComponent(parts[1])
				if err == nil {
					if idx := strings.Index(decoded, "&"); idx != -1 {
						decoded = decoded[:idx]
					}
					url = decoded
				}
			}
		}

		if !strings.HasPrefix(url, "http") {
			continue
		}

		snippet := ""
		if i < len(snippets) {
			snippet = stripHTMLTags(snippets[i][1])
		}

		results = append(results, searchResult{
			Title:   decodeHTMLEntities(title),
			URL:     url,
			Snippet: decodeHTMLEntities(snippet),
		})
	}

	return results
}

func decodeURLComponent(s string) (string, error) {
	s = strings.ReplaceAll(s, "%3A", ":")
	s = strings.ReplaceAll(s, "%2F", "/")
	s = strings.ReplaceAll(s, "%3F", "?")
	s = strings.ReplaceAll(s, "%3D", "=")
	s = strings.ReplaceAll(s, "%26", "&")
	s = strings.ReplaceAll(s, "%25", "%")
	return s, nil
}

func stripHTMLTags(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return strings.TrimSpace(re.ReplaceAllString(s, ""))
}

func decodeHTMLEntities(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return s
}
