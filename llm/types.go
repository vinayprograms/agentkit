package llm

// Message represents an LLM message.
type Message struct {
	Role       string             `json:"role"` // user, assistant, tool, system
	Content    string             `json:"content"`
	ToolCalls  []ToolCallResponse `json:"tool_calls,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"` // For tool result messages
}

// ToolDef represents a tool definition for the LLM.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCallResponse represents a tool call from the LLM.
type ToolCallResponse struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// ChatRequest represents a chat request to the LLM.
type ChatRequest struct {
	Messages   []Message     `json:"messages"`
	Tools      []ToolDef     `json:"tools,omitempty"`
	MaxTokens  int           `json:"max_tokens,omitempty"`
	Thinking   ThinkingLevel `json:"thinking,omitempty"`    // Per-call override; empty = use provider default. Ignored by providers/models that don't support thinking.
	ToolChoice ToolChoice    `json:"tool_choice,omitempty"` // Per-call override; zero value = ToolChoiceAuto. Providers/models that can't honor it fall back to auto rather than erroring.
}

// ChatResponse represents a chat response from the LLM.
type ChatResponse struct {
	Content      string             `json:"content"`
	Thinking     string             `json:"thinking,omitempty"`
	ToolCalls    []ToolCallResponse `json:"tool_calls,omitempty"`
	StopReason   string             `json:"stop_reason"`
	InputTokens  int                `json:"input_tokens"`
	OutputTokens int                `json:"output_tokens"`
	Model        string             `json:"model"`
	// Provider-specific metrics (populated only by providers that support them).
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}
