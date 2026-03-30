package llm

// Option configures a Prompt-built ChatRequest.
type Option func(*promptConfig)

type promptConfig struct {
	systemPrompt string
	maxTokens    int
}

// SystemPrompt adds a system message before the user prompt.
func SystemPrompt(s string) Option {
	return func(c *promptConfig) { c.systemPrompt = s }
}

// MaxTokens sets the max response tokens.
func MaxTokens(n int) Option {
	return func(c *promptConfig) { c.maxTokens = n }
}

// Prompt builds a ChatRequest from a simple text prompt.
// Examples:
//
//	resp, err := provider.Chat(ctx, llm.Prompt("summarize this"))
//	resp, err := provider.Chat(ctx, llm.Prompt("explain", llm.SystemPrompt("Be concise.")))
func Prompt(text string, opts ...Option) ChatRequest {
	cfg := &promptConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	var messages []Message
	if cfg.systemPrompt != "" {
		messages = append(messages, Message{Role: "system", Content: cfg.systemPrompt})
	}
	messages = append(messages, Message{Role: "user", Content: text})

	req := ChatRequest{Messages: messages}
	if cfg.maxTokens > 0 {
		req.MaxTokens = cfg.maxTokens
	}
	return req
}
