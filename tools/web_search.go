package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/vinayprograms/agentkit/credentials"
)

type webSearchTool struct {
	creds  credentials.Lookup
	client *http.Client
}

// Search returns a tool that searches the web using available backends.
// creds may be nil (falls back to environment variables and DuckDuckGo).
func Search(creds credentials.Lookup) Tool {
	return &webSearchTool{
		creds:  creds,
		client: &http.Client{Timeout: defaultHTTPTimeout},
	}
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

// Global rate limiter for web search.
var (
	searchMutex    sync.Mutex
	lastSearchTime time.Time
	searchCooldown = 500 * time.Millisecond
)

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

	// Rate limiting.
	searchMutex.Lock()
	elapsed := time.Since(lastSearchTime)
	if elapsed < searchCooldown {
		wait := searchCooldown - elapsed
		searchMutex.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(wait):
		}
		searchMutex.Lock()
	}
	lastSearchTime = time.Now()
	searchMutex.Unlock()

	// Resolve API keys: credentials first, then env vars.
	var searxngURL, braveKey, tavilyKey string
	if t.creds != nil {
		searxngURL = string(t.creds.Get("searxng"))
		braveKey = string(t.creds.Get("brave"))
		tavilyKey = string(t.creds.Get("tavily"))
	}
	if searxngURL == "" {
		searxngURL = os.Getenv("SEARXNG_URL")
	}
	if braveKey == "" {
		braveKey = os.Getenv("BRAVE_API_KEY")
	}
	if tavilyKey == "" {
		tavilyKey = os.Getenv("TAVILY_API_KEY")
	}

	var results []searchResult
	switch {
	case searxngURL != "":
		results, err = t.searchSearXNG(ctx, query, count, searxngURL)
	case braveKey != "":
		results, err = t.searchBrave(ctx, query, count, braveKey)
	case tavilyKey != "":
		results, err = t.searchTavily(ctx, query, count, tavilyKey)
	default:
		results, err = t.searchDuckDuckGo(ctx, query, count)
	}
	if err != nil {
		return "", err
	}

	return formatSearchResults(results), nil
}

// searchResult represents a single search result (unexported).
type searchResult struct {
	Title   string
	URL     string
	Snippet string
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

// --- Search backends ---

func (t *webSearchTool) searchSearXNG(ctx context.Context, query string, count int, baseURL string) ([]searchResult, error) {
	baseURL = strings.TrimSuffix(baseURL, "/")
	searchURL := fmt.Sprintf("%s/search?q=%s&format=json&categories=general",
		baseURL, strings.ReplaceAll(query, " ", "+"))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "HeadlessAgent/1.0 (+https://github.com/vinayprograms/agent)")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searxng search failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("searxng search error (%d): %s", resp.StatusCode, string(body))
	}

	var searxResp struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&searxResp); err != nil {
		return nil, fmt.Errorf("failed to parse searxng response: %w", err)
	}

	results := make([]searchResult, 0, count)
	for i, r := range searxResp.Results {
		if i >= count {
			break
		}
		results = append(results, searchResult{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return results, nil
}

func (t *webSearchTool) searchBrave(ctx context.Context, query string, count int, apiKey string) ([]searchResult, error) {
	url := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		strings.ReplaceAll(query, " ", "+"), count)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Subscription-Token", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave search failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("brave search error (%d): %s", resp.StatusCode, string(body))
	}

	var braveResp struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&braveResp); err != nil {
		return nil, fmt.Errorf("failed to parse brave response: %w", err)
	}

	results := make([]searchResult, 0, len(braveResp.Web.Results))
	for _, r := range braveResp.Web.Results {
		results = append(results, searchResult{Title: r.Title, URL: r.URL, Snippet: r.Description})
	}
	return results, nil
}

func (t *webSearchTool) searchTavily(ctx context.Context, query string, count int, apiKey string) ([]searchResult, error) {
	reqBody := map[string]interface{}{
		"api_key":     apiKey,
		"query":       query,
		"max_results": count,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.tavily.com/search", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily search failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tavily search error (%d): %s", resp.StatusCode, string(body))
	}

	var tavilyResp struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tavilyResp); err != nil {
		return nil, fmt.Errorf("failed to parse tavily response: %w", err)
	}

	results := make([]searchResult, 0, len(tavilyResp.Results))
	for _, r := range tavilyResp.Results {
		results = append(results, searchResult{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return results, nil
}

// DDG-specific rate limiting.
var (
	ddgMutex      sync.Mutex
	ddgLastSearch time.Time
	ddgCooldown   = 2 * time.Second
	ddgBackoff    = 2 * time.Second
	ddgMaxBackoff = 5 * time.Second
	ddgMaxRetries = 3
)

func (t *webSearchTool) searchDuckDuckGo(ctx context.Context, query string, count int) ([]searchResult, error) {
	ddgMutex.Lock()
	elapsed := time.Since(ddgLastSearch)
	if elapsed < ddgCooldown {
		wait := ddgCooldown - elapsed
		ddgMutex.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		ddgMutex.Lock()
	}
	ddgLastSearch = time.Now()
	ddgMutex.Unlock()

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
		req.Header.Set("User-Agent", "HeadlessAgent/1.0 (+https://github.com/vinayprograms/agent)")
		req.Header.Set("Accept", "text/html")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")

		resp, err := t.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("duckduckgo search failed: %w", err)
			continue
		}

		if resp.StatusCode == 202 || resp.StatusCode == 403 || resp.StatusCode == 429 {
			resp.Body.Close()
			lastErr = fmt.Errorf("duckduckgo rate limited (status %d), retrying", resp.StatusCode)
			continue
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			return nil, fmt.Errorf("duckduckgo search error: status %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read duckduckgo response: %w", err)
		}

		return parseDuckDuckGoHTML(string(body), count), nil
	}

	return nil, fmt.Errorf("duckduckgo search failed after %d retries: %w", ddgMaxRetries, lastErr)
}

func parseDuckDuckGoHTML(html string, count int) []searchResult {
	var results []searchResult

	linkRe := regexp.MustCompile(`<a[^>]+class="result__a"[^>]+href="([^"]+)"[^>]*>([^<]+)</a>`)
	snippetRe := regexp.MustCompile(`<a[^>]+class="result__snippet"[^>]*>([^<]+(?:<[^>]+>[^<]*</[^>]+>)*[^<]*)</a>`)

	links := linkRe.FindAllStringSubmatch(html, count*2)
	snippets := snippetRe.FindAllStringSubmatch(html, count*2)

	for i := 0; i < len(links) && len(results) < count; i++ {
		url := links[i][1]
		title := strings.TrimSpace(links[i][2])

		// DuckDuckGo wraps URLs in a redirect — extract the actual URL.
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
