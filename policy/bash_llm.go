package policy

import (
	"context"
	"fmt"
	"strings"
)

// LLMProvider is the minimal interface needed for policy checking.
type LLMProvider interface {
	// Generate returns the LLM's response to a prompt.
	Generate(ctx context.Context, prompt string) (string, error)
}

// SmallLLMChecker implements LLMPolicyChecker using a fast/cheap LLM.
type SmallLLMChecker struct {
	provider LLMProvider
}

// NewSmallLLMChecker creates a new LLM-based policy checker.
func NewSmallLLMChecker(provider LLMProvider) *SmallLLMChecker {
	return &SmallLLMChecker{provider: provider}
}

// CheckBashCommand asks the LLM if a bash command violates directory policy.
func (c *SmallLLMChecker) CheckBashCommand(ctx context.Context, command string, allowedDirs []string) (bool, string, error) {
	if c.provider == nil {
		return true, "", nil // No LLM configured, allow
	}

	// Build the prompt
	prompt := fmt.Sprintf(`Analyze this bash command for directory access violations.

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

Answer with exactly one word on the first line:
- ALLOW - if the command only accesses allowed directories
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
		strings.Join(allowedDirs, "\n"),
		command,
	)

	response, err := c.provider.Generate(ctx, prompt)
	if err != nil {
		return false, fmt.Sprintf("LLM check failed: %v", err), err
	}

	// Parse response
	response = strings.TrimSpace(response)
	lines := strings.SplitN(response, "\n", 2)
	if len(lines) == 0 {
		return false, "LLM returned empty response", nil
	}

	verdict := strings.ToUpper(strings.TrimSpace(lines[0]))

	switch verdict {
	case "ALLOW":
		return true, "", nil
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
		return false, reason, nil
	default:
		// Unclear response - fail safe (block)
		return false, fmt.Sprintf("unclear LLM response: %s", lines[0]), nil
	}
}
