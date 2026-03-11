package policy

import (
	"context"
	"fmt"
	"strings"
)

// GenerateResult contains the LLM response with token counts.
type GenerateResult struct {
	Content      string
	InputTokens  int
	OutputTokens int
}

// LLMProvider is the minimal interface needed for policy checking.
type LLMProvider interface {
	// Generate returns the LLM's response to a prompt with token counts.
	Generate(ctx context.Context, prompt string) (*GenerateResult, error)
}

// SmallLLMChecker implements LLMPolicyChecker using a fast/cheap LLM.
type SmallLLMChecker struct {
	provider      LLMProvider
	securityScope string
}

// BashCheckResult contains the result of a bash command check.
type BashCheckResult struct {
	Allowed      bool
	Reason       string
	InputTokens  int
	OutputTokens int
}

// NewSmallLLMChecker creates a new LLM-based policy checker.
func NewSmallLLMChecker(provider LLMProvider) *SmallLLMChecker {
	return &SmallLLMChecker{provider: provider}
}

// SetSecurityScope sets the security research scope for exception handling.
// When set, the LLM is told about authorized security research activities.
func (c *SmallLLMChecker) SetSecurityScope(scope string) {
	c.securityScope = scope
}

// parseVerdict extracts the verdict from an LLM response.
// Small models often dump reasoning before the verdict, so we scan all lines
// and take the LAST ALLOW/BLOCK found (the model's "final answer").
// Also extracts reason text following the verdict line.
func parseVerdict(content string) (verdict, reason string) {
	lines := strings.Split(content, "\n")
	lastVerdict := ""
	lastVerdictIdx := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)
		// Check for standalone ALLOW/BLOCK or **ALLOW**/**BLOCK** (markdown bold)
		cleaned := strings.Trim(upper, "*_ ")
		if cleaned == "ALLOW" || cleaned == "BLOCK" {
			lastVerdict = cleaned
			lastVerdictIdx = i
		}
	}

	if lastVerdict == "" {
		return "", content
	}

	// Extract reason from the line after the verdict
	if lastVerdict == "BLOCK" && lastVerdictIdx+1 < len(lines) {
		reasonLine := strings.TrimSpace(lines[lastVerdictIdx+1])
		if strings.HasPrefix(strings.ToLower(reasonLine), "reason:") {
			reason = strings.TrimSpace(reasonLine[7:])
		} else if reasonLine != "" {
			reason = reasonLine
		}
	}

	return lastVerdict, reason
}

// CheckBashCommand asks the LLM if a bash command violates directory policy.
// workingDir is the cwd where the command executes (for resolving relative paths).
// Returns a BashCheckResult with the decision and token usage.
func (c *SmallLLMChecker) CheckBashCommand(ctx context.Context, command string, allowedDirs []string, workingDir string) (*BashCheckResult, error) {
	if c.provider == nil {
		return &BashCheckResult{Allowed: true}, nil // No LLM configured, allow
	}

	// Build the prompt
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
3. Relative paths resolve from WORKING DIRECTORY — check if the resolved path is inside a writable directory
4. /tmp is always writable (temporary files and build outputs)
5. /dev/null, /dev/zero, /dev/urandom are always writable (system devices)
6. Writing ANYWHERE ELSE is BLOCKED — including /workdir, /opt, /etc, /var, /root (unless listed above), /home, or any other path not in the writable list
7. If a security research context is provided, commands within that scope are OK

ANSWER FORMAT: Reply with ONLY "ALLOW" or "BLOCK" on its own line.
If BLOCK, add a brief reason on the next line.
Do NOT explain your reasoning. Do NOT hedge. Just the verdict.

ALLOW means: the command only reads/executes, OR writes inside writable directories (including subdirectories).
BLOCK means: the command writes to a path that is NOT inside any writable directory.

CRITICAL: Subdirectories of writable directories ARE writable. If /workspace is writable, then /workspace/src/main.go is ALSO writable.

Your answer:`,
		securityContext,
		workingDir,
		strings.Join(allowedDirs, "\n"),
		command,
	)

	result, err := c.provider.Generate(ctx, prompt)
	if err != nil {
		return &BashCheckResult{
			Allowed: false,
			Reason:  fmt.Sprintf("LLM check failed: %v", err),
		}, err
	}

	// Parse response — extract verdict robustly.
	// Small models often ignore "first word" instructions and dump reasoning
	// before the verdict. We scan all lines for ALLOW/BLOCK and take the LAST
	// occurrence (the model's "final answer" after reasoning).
	content := strings.TrimSpace(result.Content)
	if content == "" {
		return &BashCheckResult{
			Allowed:      false,
			Reason:       "LLM returned empty response",
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
		}, nil
	}

	verdict, reason := parseVerdict(content)

	switch verdict {
	case "ALLOW":
		return &BashCheckResult{
			Allowed:      true,
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
		}, nil
	case "BLOCK":
		if reason == "" {
			reason = "blocked by LLM policy check"
		}
		return &BashCheckResult{
			Allowed:      false,
			Reason:       reason,
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
		}, nil
	default:
		return &BashCheckResult{
			Allowed:      false,
			Reason:       content,
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
		}, nil
	}
}
