package llm

import (
	"testing"
)

// Test Prompt helper

func TestPrompt_Simple(t *testing.T) {
	req := Prompt("hello")
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "user" || req.Messages[0].Content != "hello" {
		t.Errorf("unexpected message: %+v", req.Messages[0])
	}
}

func TestPrompt_WithSystemPrompt(t *testing.T) {
	req := Prompt("hello", SystemPrompt("Be concise."))
	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Content != "Be concise." {
		t.Errorf("unexpected system message: %+v", req.Messages[0])
	}
	if req.Messages[1].Role != "user" || req.Messages[1].Content != "hello" {
		t.Errorf("unexpected user message: %+v", req.Messages[1])
	}
}

func TestPrompt_WithMaxTokens(t *testing.T) {
	req := Prompt("hello", MaxTokens(500))
	if req.MaxTokens != 500 {
		t.Errorf("expected max tokens 500, got %d", req.MaxTokens)
	}
}

func TestPrompt_AllOptions(t *testing.T) {
	req := Prompt("hello", SystemPrompt("Be helpful."), MaxTokens(100))
	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}
	if req.MaxTokens != 100 {
		t.Errorf("expected max tokens 100, got %d", req.MaxTokens)
	}
}
