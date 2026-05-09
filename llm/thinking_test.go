package llm

import (
	"testing"
)

func TestInferThinkingLevel(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		tools    []ToolDef
		expected ThinkingLevel
	}{
		{
			name: "simple greeting",
			messages: []Message{
				{Role: "user", Content: "Hello, how are you?"},
			},
			expected: ThinkingOff,
		},
		{
			name: "math problem",
			messages: []Message{
				{Role: "user", Content: "Calculate 2^10 + 15/3"},
			},
			expected: ThinkingHigh,
		},
		{
			name: "prove keyword",
			messages: []Message{
				{Role: "user", Content: "Prove that the sum of angles in a triangle is 180 degrees"},
			},
			expected: ThinkingHigh,
		},
		{
			name: "architecture design",
			messages: []Message{
				{Role: "user", Content: "Design system for a real-time chat application with 1M users"},
			},
			expected: ThinkingHigh,
		},
		{
			name: "security analysis",
			messages: []Message{
				{Role: "user", Content: "Do a security analysis of this authentication flow"},
			},
			expected: ThinkingHigh,
		},
		{
			name: "debug request",
			messages: []Message{
				{Role: "user", Content: "Why is this function returning null?"},
			},
			expected: ThinkingHigh,
		},
		{
			name: "code implementation",
			messages: []Message{
				{Role: "user", Content: "Implement a function to sort an array"},
			},
			expected: ThinkingMedium,
		},
		{
			name: "step by step",
			messages: []Message{
				{Role: "user", Content: "Explain step by step how to deploy this"},
			},
			expected: ThinkingMedium,
		},
		{
			name: "refactor request",
			messages: []Message{
				{Role: "user", Content: "Refactor this code to be more maintainable"},
			},
			expected: ThinkingMedium,
		},
		{
			name: "many tools",
			messages: []Message{
				{Role: "user", Content: "Do something"},
			},
			tools:    make([]ToolDef, 12),
			expected: ThinkingHigh,
		},
		{
			name: "moderate tools",
			messages: []Message{
				{Role: "user", Content: "Do something"},
			},
			tools:    make([]ToolDef, 7),
			expected: ThinkingMedium,
		},
		{
			name: "how to question",
			messages: []Message{
				{Role: "user", Content: "How to install Docker on Ubuntu?"},
			},
			expected: ThinkingLow,
		},
		{
			name: "recommendation request",
			messages: []Message{
				{Role: "user", Content: "What is the best database for this use case?"},
			},
			expected: ThinkingLow,
		},
		{
			name: "long context",
			messages: []Message{
				{Role: "user", Content: string(make([]byte, 3500))},
			},
			expected: ThinkingHigh,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InferThinkingLevel(tt.messages, tt.tools)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestResolveThinkingLevel(t *testing.T) {
	simple := ChatRequest{Messages: []Message{{Role: "user", Content: "Simple hello"}}}

	// Auto mode uses classifier
	if got := ResolveThinkingLevel(ThinkingConfig{Level: ThinkingAuto}, simple); got != ThinkingOff {
		t.Errorf("auto mode should return Off for simple message, got %s", got)
	}

	// Fixed mode ignores classifier
	if got := ResolveThinkingLevel(ThinkingConfig{Level: ThinkingHigh}, simple); got != ThinkingHigh {
		t.Errorf("fixed mode should return High, got %s", got)
	}

	// Empty level defaults to auto
	if got := ResolveThinkingLevel(ThinkingConfig{Level: ""}, simple); got != ThinkingOff {
		t.Errorf("empty level should default to auto, got %s", got)
	}
}

func TestResolveThinkingLevelRequestOverride(t *testing.T) {
	simple := []Message{{Role: "user", Content: "Simple hello"}}

	// Override beats provider default (force-on a provider configured Off).
	req := ChatRequest{Messages: simple, Thinking: ThinkingHigh}
	if got := ResolveThinkingLevel(ThinkingConfig{Level: ThinkingOff}, req); got != ThinkingHigh {
		t.Errorf("request override should win over provider Off, got %s", got)
	}

	// Override beats provider default (force-off a provider configured High).
	req = ChatRequest{Messages: simple, Thinking: ThinkingOff}
	if got := ResolveThinkingLevel(ThinkingConfig{Level: ThinkingHigh}, req); got != ThinkingOff {
		t.Errorf("request override should win over provider High, got %s", got)
	}

	// No override falls through to provider config.
	req = ChatRequest{Messages: simple}
	if got := ResolveThinkingLevel(ThinkingConfig{Level: ThinkingMedium}, req); got != ThinkingMedium {
		t.Errorf("nil override should yield provider Medium, got %s", got)
	}

	// Auto override runs the heuristic.
	req = ChatRequest{Messages: simple, Thinking: ThinkingAuto}
	if got := ResolveThinkingLevel(ThinkingConfig{Level: ThinkingHigh}, req); got != ThinkingOff {
		t.Errorf("auto override on simple message should yield Off (heuristic), got %s", got)
	}

	// Tools flow through to the heuristic via req.Tools (regression guard for
	// the signature change that folded messages/tools into ChatRequest).
	manyTools := make([]ToolDef, 11) // > 10 trips detectHighComplexity
	req = ChatRequest{Messages: simple, Tools: manyTools, Thinking: ThinkingAuto}
	if got := ResolveThinkingLevel(ThinkingConfig{}, req); got != ThinkingHigh {
		t.Errorf("auto with >10 tools should yield High via heuristic, got %s", got)
	}
}


func TestContainsMathExpression(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"Calculate 2+3", true},
		{"What is 10/2?", true},
		{"Compute 2^10", true},
		{"Hello world", false},
		{"x > y comparison", false},
		{"1/2 fraction", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := containsMathExpression(tt.input)
			if result != tt.expected {
				t.Errorf("containsMathExpression(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDefaultThinkingIsAuto(t *testing.T) {
	// When thinking level is empty string, ResolveThinkingLevel should use heuristic
	config := ThinkingConfig{Level: ""}
	
	// Simple message should get Off (heuristic decides)
	simple := ChatRequest{Messages: []Message{{Role: "user", Content: "Hello"}}}
	if got := ResolveThinkingLevel(config, simple); got != ThinkingOff {
		t.Errorf("expected Off for simple hello with empty config, got %s", got)
	}

	// Complex message should get High (heuristic decides)
	complex := ChatRequest{Messages: []Message{{Role: "user", Content: "Prove that P = NP"}}}
	if got := ResolveThinkingLevel(config, complex); got != ThinkingHigh {
		t.Errorf("expected High for proof request with empty config, got %s", got)
	}
}
