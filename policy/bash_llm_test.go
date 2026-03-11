package policy

import "testing"

func TestParseVerdict(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		expectVerdict  string
		expectReason   string
	}{
		{"clean ALLOW", "ALLOW", "ALLOW", ""},
		{"clean BLOCK", "BLOCK\nReason: writes to /etc", "BLOCK", "writes to /etc"},
		{"bold ALLOW", "**ALLOW**", "ALLOW", ""},
		{"bold BLOCK", "**BLOCK**\nwrites outside", "BLOCK", "writes outside"},
		{"rambling then ALLOW", "This command reads files...\nit seems safe\n\nALLOW", "ALLOW", ""},
		{"rambling then BLOCK", "Analysis:\npath /etc is bad\n\nBLOCK\nwrites to /etc", "BLOCK", "writes to /etc"},
		{"says BLOCK then corrects to ALLOW", "BLOCK but wait...\ncorrection:\nALLOW", "ALLOW", ""},
		{"final answer ALLOW", "Some reasoning...\n**Final Answer:**\n**ALLOW**", "ALLOW", ""},
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
