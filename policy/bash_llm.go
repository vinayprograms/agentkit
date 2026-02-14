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

// CheckBashCommand asks the LLM if a bash command violates directory policy.
// Returns a BashCheckResult with the decision and token usage.
func (c *SmallLLMChecker) CheckBashCommand(ctx context.Context, command string, allowedDirs []string) (*BashCheckResult, error) {
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

	prompt := fmt.Sprintf(`Analyze this bash command for directory access violations.
%s
ALLOWED DIRECTORIES (agent can access these):
%s

COMMAND:
%s

RULES:
1. Commands accessing paths INSIDE allowed directories are OK
2. Commands accessing paths OUTSIDE allowed directories are BLOCKED  
3. Relative paths (./foo, ../bar) starting from allowed dirs are OK
4. Commands with no file paths are OK
5. Reading from /dev/null, /proc, /sys is OK
6. Commands that could write, delete, or modify files outside allowed dirs are BLOCKED
7. If a security research context is provided, commands within that scope are OK

Answer with exactly one word on the first line:
- ALLOW - if the command only accesses allowed directories (or is within security research scope)
- BLOCK - if the command accesses paths outside allowed directories

If BLOCK, add a brief reason on the second line.

Example 1:
Command: cat /etc/passwd
Answer: BLOCK
Reason: Reads /etc/passwd which is outside allowed directories

Example 2:
Command: ls -la
Answer: ALLOW

Example 3:
Command: rm -rf /
Answer: BLOCK
Reason: Attempts to delete files in root directory

Your answer:`,
		securityContext,
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

	// Parse response
	content := strings.TrimSpace(result.Content)
	lines := strings.SplitN(content, "\n", 2)
	if len(lines) == 0 {
		return &BashCheckResult{
			Allowed:      false,
			Reason:       "LLM returned empty response",
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
		}, nil
	}

	verdict := strings.ToUpper(strings.TrimSpace(lines[0]))

	switch verdict {
	case "ALLOW":
		return &BashCheckResult{
			Allowed:      true,
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
		}, nil
	case "BLOCK":
		reason := "blocked by LLM policy check"
		if len(lines) > 1 {
			// Extract reason from second line
			reasonLine := strings.TrimSpace(lines[1])
			if strings.HasPrefix(strings.ToLower(reasonLine), "reason:") {
				reason = strings.TrimSpace(reasonLine[7:])
			} else {
				reason = reasonLine
			}
		}
		return &BashCheckResult{
			Allowed:      false,
			Reason:       reason,
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
		}, nil
	default:
		// Unclear response - fail safe (block)
		return &BashCheckResult{
			Allowed:      false,
			Reason:       fmt.Sprintf("unclear LLM response: %s", lines[0]),
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
		}, nil
	}
}
