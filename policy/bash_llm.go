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

Answer with exactly one word on the first line:
- ALLOW - if the command only reads/executes OR writes inside writable directories
- BLOCK - if the command writes outside writable directories

If BLOCK, add a brief reason on the second line.

Example 1:
Command: go build -o ./app ./cmd/server
Answer: ALLOW
(executes go toolchain, writes to working directory)

Example 2:
Command: mkdir -p /workdir/src
Writable: /home/user/project
Answer: BLOCK
Reason: creates directory /workdir which is outside writable directories

Example 3:
Command: cat /etc/os-release
Answer: ALLOW
(read-only access)

Example 4:
Command: echo "hello" > /opt/output.txt
Writable: /home/user/project
Answer: BLOCK
Reason: writes to /opt which is outside writable directories

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
