package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const defaultHTTPTimeout = 2 * time.Minute

// WebOption configures web tools (Fetch, Search).
type WebOption func(*webConfig)

type webConfig struct {
	timeout time.Duration
}

// WithHTTPTimeout sets the HTTP client timeout for web tools.
// A non-positive value leaves the default (2 minutes) in place.
func WithHTTPTimeout(d time.Duration) WebOption {
	return func(c *webConfig) {
		if d > 0 {
			c.timeout = d
		}
	}
}

func webConfigFrom(opts []WebOption) webConfig {
	c := webConfig{timeout: defaultHTTPTimeout}
	for _, opt := range opts {
		opt(&c)
	}
	return c
}

type webFetchTool struct {
	summarizer Summarizer
	client     *http.Client
}

// Fetch returns a tool that fetches web page content.
// summarizer may be nil (returns full extracted text).
func Fetch(summarizer Summarizer, opts ...WebOption) Tool {
	cfg := webConfigFrom(opts)
	return &webFetchTool{
		summarizer: summarizer,
		client:     &http.Client{Timeout: cfg.timeout},
	}
}

func (t *webFetchTool) Name() string { return "web_fetch" }

func (t *webFetchTool) Description() string {
	return "Fetch and summarize content from a URL. Requires a question/prompt — the tool returns a concise answer based on the page content, not the raw page. Use after web_search to get specific information from promising results."
}

func (t *webFetchTool) Parameters() map[string]Param {
	return map[string]Param{
		"url": {
			Type:        StringParam,
			Description: "URL to fetch (typically from web_search results)",
			Required:    true,
		},
		"question": {
			Type:        StringParam,
			Description: "What information to extract from the page",
			Required:    true,
		},
	}
}

func (t *webFetchTool) Execute(ctx context.Context, args Args) (string, error) {
	url, err := args.String("url")
	if err != nil {
		return "", err
	}

	question, err := args.String("question")
	if err != nil {
		return "", err
	}

	// Fetch the page.
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AIAgent/1.0)")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	// Read body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// HTTP errors with a body are results, not errors.
	if resp.StatusCode >= 400 {
		return fmt.Sprintf("HTTP %d %s\n\n%s", resp.StatusCode, resp.Status, string(body)), nil
	}

	// Extract readable text from HTML.
	content := extractReadableText(string(body))

	// Without a summarizer, return full text.
	if t.summarizer == nil {
		return content, nil
	}

	// Use summarizer to answer the question.
	answer, err := t.summarizer.Summarize(ctx, content, question)
	if err != nil {
		return "", fmt.Errorf("summarization failed: %w", err)
	}
	return answer, nil
}

// extractReadableText removes HTML tags and extracts readable content.
func extractReadableText(html string) string {
	// Remove script and style blocks.
	reScript := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	html = reScript.ReplaceAllString(html, "")
	reStyle := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	html = reStyle.ReplaceAllString(html, "")
	reHead := regexp.MustCompile(`(?is)<head[^>]*>.*?</head>`)
	html = reHead.ReplaceAllString(html, "")
	reNav := regexp.MustCompile(`(?is)<nav[^>]*>.*?</nav>`)
	html = reNav.ReplaceAllString(html, "")
	reFooter := regexp.MustCompile(`(?is)<footer[^>]*>.*?</footer>`)
	html = reFooter.ReplaceAllString(html, "")

	// Remove HTML comments.
	reComments := regexp.MustCompile(`(?s)<!--.*?-->`)
	html = reComments.ReplaceAllString(html, "")

	// Add newlines before block elements.
	reBlock := regexp.MustCompile(`<(p|div|br|h[1-6]|li|tr)[^>]*>`)
	html = reBlock.ReplaceAllString(html, "\n")

	// Remove all remaining HTML tags.
	reTags := regexp.MustCompile(`<[^>]+>`)
	text := reTags.ReplaceAllString(html, "")

	// Only decode non-printing entities. Keep &amp; &lt; &gt; etc. escaped
	// to avoid reintroducing characters that could be injection payloads.
	text = strings.ReplaceAll(text, "&nbsp;", " ")

	// Clean up whitespace.
	reMultiSpace := regexp.MustCompile(`[ \t]+`)
	text = reMultiSpace.ReplaceAllString(text, " ")
	reMultiNewline := regexp.MustCompile(`\n{3,}`)
	text = reMultiNewline.ReplaceAllString(text, "\n\n")

	return strings.TrimSpace(text)
}
