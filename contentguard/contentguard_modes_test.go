package contentguard

import (
	"testing"
)

func TestBuildResearchPrompt(t *testing.T) {
	prompt := buildResearchSystemPrompt("authorized pentest of lab network")
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	if !contains(prompt, "authorized pentest") {
		t.Error("expected scope in prompt")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
