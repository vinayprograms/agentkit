package shellguard

import (
	"context"
	"testing"

	"github.com/vinayprograms/agentkit/llm"
)

func TestParseVerdict(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		expectVerdict string
		expectReason  string
	}{
		{"json ALLOW", `{"verdict":"ALLOW"}`, "ALLOW", ""},
		{"json BLOCK", `{"verdict":"BLOCK","reason":"writes to /etc"}`, "BLOCK", "writes to /etc"},
		{"json lowercase", `{"verdict":"allow"}`, "ALLOW", ""},
		{"json with whitespace", `  {"verdict": "BLOCK", "reason": "bad path"}  `, "BLOCK", "bad path"},
		{"json embedded in text", `Here is my analysis:\n{"verdict":"ALLOW"}\nDone.`, "ALLOW", ""},
		{"json after reasoning", "Some reasoning...\n{\"verdict\":\"BLOCK\",\"reason\":\"writes to /opt\"}", "BLOCK", "writes to /opt"},
		{"plain ALLOW fallback", "ALLOW", "ALLOW", ""},
		{"plain BLOCK fallback", "BLOCK", "BLOCK", ""},
		{"bold ALLOW fallback", "**ALLOW**", "ALLOW", ""},
		{"rambling then ALLOW", "This seems safe\n\nALLOW", "ALLOW", ""},
		{"no verdict", "I'm not sure about this command", "", "I'm not sure about this command"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict, reason := parseVerdict(tt.content)
			if verdict != tt.expectVerdict {
				t.Errorf("parseVerdict(%q) verdict = %q, want %q", tt.content, verdict, tt.expectVerdict)
			}
			if tt.expectReason != "" && reason != tt.expectReason {
				t.Errorf("parseVerdict(%q) reason = %q, want %q", tt.content, reason, tt.expectReason)
			}
		})
	}
}

func TestLLMCheck_EmptyResponse(t *testing.T) {
	mock := &emptyResponseModel{}
	result, err := llmCheck(context.Background(), mock, "ls", []string{"/workspace"}, "/workspace", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Allowed {
		t.Error("empty LLM response should block")
	}
	if result.Reason != "LLM returned empty response" {
		t.Errorf("unexpected reason: %s", result.Reason)
	}
}

type emptyResponseModel struct{}

func (m *emptyResponseModel) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{Content: ""}, nil
}

func TestLLMCheck_BlockWithNoReason(t *testing.T) {
	mock := &fixedResponseModel{response: `{"verdict":"BLOCK"}`}
	result, err := llmCheck(context.Background(), mock, "rm -rf /", []string{"/workspace"}, "/workspace", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Allowed {
		t.Error("BLOCK verdict should block")
	}
	if result.Reason != "blocked by LLM check" {
		t.Errorf("expected default block reason, got: %s", result.Reason)
	}
}

func TestLLMCheck_UnknownVerdict(t *testing.T) {
	mock := &fixedResponseModel{response: "I cannot determine if this is safe"}
	result, err := llmCheck(context.Background(), mock, "something", []string{"/workspace"}, "/workspace", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Allowed {
		t.Error("unknown verdict should block")
	}
}

func TestLLMCheck_WithSecurityScope(t *testing.T) {
	// Verify security scope is included in the prompt
	var capturedPrompt string
	mock := &capturingModel{capturedPrompt: &capturedPrompt}
	llmCheck(context.Background(), mock, "nmap localhost", []string{"/workspace"}, "/workspace", "penetration testing")
	if capturedPrompt == "" {
		t.Fatal("prompt not captured")
	}
	if !contains(capturedPrompt, "penetration testing") {
		t.Error("security scope should be included in prompt")
	}
}

func TestLLMCheck_MalformedJSON(t *testing.T) {
	mock := &fixedResponseModel{response: `{"verdict": "ALLOW", broken`}
	result, err := llmCheck(context.Background(), mock, "ls", []string{"/workspace"}, "/workspace", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Malformed JSON with no fallback verdict keyword → block
	if result.Allowed {
		t.Error("malformed JSON should block (fail closed)")
	}
}

func TestLLMCheck_WrongJSONStructure(t *testing.T) {
	mock := &fixedResponseModel{response: `{"answer": "yes", "safe": true}`}
	result, err := llmCheck(context.Background(), mock, "ls", []string{"/workspace"}, "/workspace", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Allowed {
		t.Error("wrong JSON structure should block (fail closed)")
	}
}

func TestLLMCheck_HTMLResponse(t *testing.T) {
	mock := &fixedResponseModel{response: `<html><body>Error 503</body></html>`}
	result, err := llmCheck(context.Background(), mock, "ls", []string{"/workspace"}, "/workspace", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Allowed {
		t.Error("HTML response should block (fail closed)")
	}
}

func TestLLMCheck_LLMRefusal(t *testing.T) {
	mock := &fixedResponseModel{response: "I'm sorry, I cannot evaluate shell commands for security purposes."}
	result, err := llmCheck(context.Background(), mock, "rm -rf /", []string{"/workspace"}, "/workspace", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Allowed {
		t.Error("LLM refusal should block (fail closed)")
	}
}

func TestLLMCheck_Allow(t *testing.T) {
	mock := &fixedResponseModel{response: `{"verdict":"ALLOW"}`}
	result, err := llmCheck(context.Background(), mock, "ls /workspace", []string{"/workspace"}, "/workspace", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("ALLOW verdict should allow")
	}
}

type fixedResponseModel struct {
	response string
}

func (m *fixedResponseModel) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{Content: m.response, InputTokens: 10, OutputTokens: 5}, nil
}

type capturingModel struct {
	capturedPrompt *string
}

func (m *capturingModel) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if len(req.Messages) > 0 {
		*m.capturedPrompt = req.Messages[len(req.Messages)-1].Content
	}
	return &llm.ChatResponse{Content: `{"verdict":"ALLOW"}`}, nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
