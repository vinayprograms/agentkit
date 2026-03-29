package bashsec

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CheckResult contains the result of a bash command check.
type CheckResult struct {
	Allowed      bool
	Reason       string
	InputTokens  int
	OutputTokens int
}

// LLMProvider is the minimal interface needed for policy checking.
type LLMProvider interface {
	Generate(ctx context.Context, prompt string) (*GenerateResult, error)
}

// GenerateResult contains the LLM response with token counts.
type GenerateResult struct {
	Content      string
	InputTokens  int
	OutputTokens int
}

// SmallLLMChecker implements LLMPolicyChecker using a fast/cheap LLM.
type SmallLLMChecker struct {
	provider      LLMProvider
	securityScope string
}

// NewSmallLLMChecker creates a new LLM-based policy checker.
func NewSmallLLMChecker(provider LLMProvider) *SmallLLMChecker {
	return &SmallLLMChecker{provider: provider}
}

// SetSecurityScope sets the security research scope for exception handling.
func (c *SmallLLMChecker) SetSecurityScope(scope string) {
	c.securityScope = scope
}

type verdictResponse struct {
	Verdict string `json:"verdict"`
	Reason  string `json:"reason,omitempty"`
}

func parseVerdict(content string) (verdict, reason string) {
	var resp verdictResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &resp); err == nil {
		return strings.ToUpper(resp.Verdict), resp.Reason
	}

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(content[start:end+1]), &resp); err == nil {
			return strings.ToUpper(resp.Verdict), resp.Reason
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

// CheckBashCommand asks the LLM if a bash command violates directory policy.
func (c *SmallLLMChecker) CheckBashCommand(ctx context.Context, command string, allowedDirs []string, workingDir string) (*CheckResult, error) {
	if c.provider == nil {
		return &CheckResult{Allowed: true}, nil
	}

	var securityContext string
	if c.securityScope != "" {
		securityContext = fmt.Sprintf(`
SECURITY RESEARCH CONTEXT:
This agent is conducting authorized security research within scope:
"%s"

Commands that fall within this research scope should be ALLOWED even if they
access paths outside the normal allowed directories. Use judgment to determine
if the command is part of legitimate security research.

`, c.securityScope)
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

	result, err := c.provider.Generate(ctx, prompt)
	if err != nil {
		return &CheckResult{
			Allowed: false,
			Reason:  fmt.Sprintf("LLM check failed: %v", err),
		}, err
	}

	content := strings.TrimSpace(result.Content)
	if content == "" {
		return &CheckResult{
			Allowed:      false,
			Reason:       "LLM returned empty response",
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
		}, nil
	}

	verdict, reason := parseVerdict(content)

	switch verdict {
	case "ALLOW":
		return &CheckResult{
			Allowed:      true,
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
		}, nil
	case "BLOCK":
		if reason == "" {
			reason = "blocked by LLM policy check"
		}
		return &CheckResult{
			Allowed:      false,
			Reason:       reason,
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
		}, nil
	default:
		return &CheckResult{
			Allowed:      false,
			Reason:       content,
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
		}, nil
	}
}
