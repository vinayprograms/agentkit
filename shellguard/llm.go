package shellguard

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vinayprograms/agentkit/llm"
)

// llmCheck asks an LLM whether a bash command violates directory write policy.
func llmCheck(ctx context.Context, model llm.Model, command string, allowedDirs []string, workingDir, securityScope string) (*Result, error) {
	var securityContext string
	if securityScope != "" {
		securityContext = fmt.Sprintf(`
SECURITY RESEARCH CONTEXT:
This agent is conducting authorized security research within scope:
"%s"

Commands that fall within this research scope should be ALLOWED even if they
access paths outside the normal allowed directories. Use judgment to determine
if the command is part of legitimate security research.

`, securityScope)
	}

	prompt := fmt.Sprintf(`Analyze this bash command for write access violations.
%s
WORKING DIRECTORY (cwd where command executes):
%s

WRITABLE DIRECTORIES (agent can ONLY write here):
%s

COMMAND:
%s

RULES:
1. READ and EXECUTE from anywhere is OK — running compilers, interpreters, build tools, reading system libraries, and accessing toolchain paths is normal
2. WRITE operations (create, modify, delete, mkdir, touch, mv, cp, >, >>) are ONLY allowed inside the WRITABLE DIRECTORIES listed above
3. A writable directory means the directory AND ALL ITS SUBDIRECTORIES at any depth are writable. Example: if /workspace is writable, then /workspace/src/main.go, /workspace/internal/auth/handler.go, and /workspace/a/b/c/d.txt are ALL writable. This is non-negotiable.
4. Relative paths resolve from WORKING DIRECTORY — check if the resolved path is inside a writable directory
5. /tmp is always writable (temporary files and build outputs)
6. /dev/null, /dev/zero, /dev/urandom are always writable (system devices)
7. Writing ANYWHERE ELSE is BLOCKED — including /workdir, /opt, /etc, /var, /root (unless listed above), /home, or any other path not in the writable list
8. SECURITY: Watch for path traversal attacks. Paths containing /../ or /../../ that escape a writable directory MUST be resolved to their canonical form first. Example: /workspace/../etc/passwd resolves to /etc/passwd which is NOT inside /workspace — BLOCK it.
9. If a security research context is provided, commands within that scope are OK

DECISION LOGIC:
For each write path in the command:
  a. Resolve the full absolute path (expand relative paths from cwd, resolve all .. components)
  b. Check: does the resolved path start with any WRITABLE DIRECTORY prefix?
  c. If YES for all write paths → ALLOW
  d. If NO for any write path → BLOCK

Respond with ONLY a JSON object, nothing else:
{"verdict":"ALLOW"} or {"verdict":"BLOCK","reason":"brief explanation"}`,
		securityContext,
		workingDir,
		strings.Join(allowedDirs, "\n"),
		command,
	)

	// Take the decision through a structured tool call rather than parsing
	// prose: llm.Ask pins ToolChoice to verdictTool, forces thinking off (a
	// bounded classification isn't a task that benefits from deliberation),
	// and — for providers/models that can't honor ToolChoice, or that
	// answer in prose anyway — falls back to parseVerdict. It also absorbs
	// the empty-content/StopReason=="length" retry-once behavior this
	// function used to implement inline.
	d, err := llm.Ask(ctx, model, prompt, verdictTool, parseVerdictFallback)
	if err != nil {
		return &Result{Allowed: false, Reason: fmt.Sprintf("LLM check failed: %v", err)}, err
	}

	if d.Content == "" && d.Args == nil {
		// Empty response twice: a reviewer failure is not a denial.
		// llmCheck only runs once the deterministic stage has already
		// allowed this command (see Gate.check), so fall back to that
		// verdict — fail-closed DENY here would turn an unrelated model
		// hiccup into a hard block on a command the deterministic rules
		// already cleared (see shellguard llm.go / P0 8c).
		return &Result{
			Allowed:      true,
			Reason:       "LLM reviewer returned empty response twice; falling back to deterministic ALLOW",
			InputTokens:  d.InputTokens,
			OutputTokens: d.OutputTokens,
		}, nil
	}

	if d.Args == nil {
		// Model answered in prose and parseVerdict couldn't recover a
		// verdict from it either.
		return &Result{Allowed: false, Reason: truncateReason(d.Content), InputTokens: d.InputTokens, OutputTokens: d.OutputTokens}, nil
	}

	allow, _ := d.Args["allow"].(bool)
	reason, _ := d.Args["reason"].(string)
	reason = truncateReason(reason)

	if allow {
		return &Result{Allowed: true, InputTokens: d.InputTokens, OutputTokens: d.OutputTokens}, nil
	}
	if reason == "" {
		reason = "blocked by LLM check"
	}
	return &Result{Allowed: false, Reason: reason, InputTokens: d.InputTokens, OutputTokens: d.OutputTokens}, nil
}

// verdictTool is the structured-decision tool llmCheck asks the model to
// call: an explicit allow/reason pair instead of a verdict scraped from
// prose.
var verdictTool = llm.ToolDef{
	Name:        "verdict",
	Description: "Report the write-access-violation verdict for the analyzed command.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"allow":  map[string]any{"type": "boolean", "description": "true to ALLOW the command, false to BLOCK it"},
			"reason": map[string]any{"type": "string", "description": "brief explanation, required when allow is false"},
		},
		"required": []string{"allow"},
	},
}

// parseVerdictFallback adapts parseVerdict to llm.ParseFallback: it's the
// prose fallback llm.Ask uses when the model answers in text instead of
// calling verdictTool.
func parseVerdictFallback(content string) (map[string]any, bool) {
	verdict, reason := parseVerdict(content)
	if verdict == "" {
		return nil, false
	}
	return map[string]any{"allow": verdict == "ALLOW", "reason": reason}, true
}

type verdictResponse struct {
	Verdict string `json:"verdict"`
	Reason  string `json:"reason,omitempty"`
}

// maxReasonLen bounds the verdict reason text surfaced to the caller (and
// from there, often back into an agent's context or a session log). A
// reasoning model's chain-of-thought leaking into resp.Content can run to
// 900+ chars; nothing downstream needs more than a brief explanation.
const maxReasonLen = 200

func truncateReason(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxReasonLen {
		return s
	}
	return s[:maxReasonLen] + "... (truncated)"
}

// balancedJSONObjects returns every top-level balanced {...} substring of
// s, in the order they appear. A reasoning model's reply can carry
// chain-of-thought prose before the verdict JSON, or even two JSON objects
// (e.g. one embedded in an explanation, one as the actual answer) — this
// lets parseVerdict try candidates from the end, where the actual verdict
// almost always lands.
func balancedJSONObjects(s string) []string {
	var objs []string
	depth := 0
	start := -1
	for i, r := range s {
		switch r {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					objs = append(objs, s[start:i+1])
					start = -1
				}
			}
		}
	}
	return objs
}

func parseVerdict(content string) (verdict, reason string) {
	var resp verdictResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &resp); err == nil && resp.Verdict != "" {
		return strings.ToUpper(resp.Verdict), resp.Reason
	}

	// Try the LAST balanced JSON object that actually parses as a verdict
	// first: reasoning text precedes the verdict far more often than it
	// follows it, and when two objects are present the later one is the
	// answer.
	objs := balancedJSONObjects(content)
	for i := len(objs) - 1; i >= 0; i-- {
		var r verdictResponse
		if err := json.Unmarshal([]byte(objs[i]), &r); err == nil && r.Verdict != "" {
			return strings.ToUpper(r.Verdict), r.Reason
		}
	}

	lines := strings.Split(content, "\n")
	lastVerdict := ""
	for _, line := range lines {
		cleaned := strings.ToUpper(strings.Trim(strings.TrimSpace(line), "*_ "))
		if cleaned == "ALLOW" || cleaned == "BLOCK" {
			lastVerdict = cleaned
		}
	}
	if lastVerdict != "" {
		return lastVerdict, ""
	}

	return "", content
}
