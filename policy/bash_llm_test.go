package policy

import "testing"

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
