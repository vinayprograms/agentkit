package contentguard

import (
	"testing"
)

func TestSecurityModes(t *testing.T) {
	tests := []struct {
		name     string
		mode     Mode
		expected string
	}{
		{"default mode", Default, "default"},
		{"paranoid mode", Paranoid, "paranoid"},
		{"research mode", Research, "research"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.mode) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(tt.mode))
			}
		})
	}
}

func TestReviewer_ResearchPrompt(t *testing.T) {
	// Verify research mode builds prompt with scope
	scope := "authorized pentest of internal lab network"
	r := &Reviewer{mode: Research, researchScope: scope}
	prompt := r.buildResearchSystemPrompt()

	if prompt == "" {
		t.Error("expected non-empty research system prompt")
	}
	if !contains(prompt, scope) {
		t.Error("expected research prompt to contain scope")
	}
	if !contains(prompt, "AUTHORIZED SECURITY RESEARCH") {
		t.Error("expected research prompt to indicate authorized research")
	}
}

func TestReviewer_DefaultMode(t *testing.T) {
	r := &Reviewer{mode: Default}
	if r.mode != Default {
		t.Errorf("expected mode=default, got %s", r.mode)
	}
	if r.researchScope != "" {
		t.Errorf("expected empty scope, got %q", r.researchScope)
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
