// Package policy provides bash command security checking.
package policy

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// BannedCommands is the hardcoded list of commands that are always blocked.
// These are dangerous for system security and should never be executed by an agent.
// Based on Charmbracelet Crush's approach.
var BannedCommands = []string{
	// Network/Download tools - prevent data exfiltration and arbitrary downloads
	"alias",
	"aria2c",
	"axel",
	"chrome",
	"curl",
	"curlie",
	"firefox",
	"http-prompt",
	"httpie",
	"links",
	"lynx",
	"nc",
	"netcat",
	"ncat",
	"safari",
	"scp",
	"sftp",
	"ssh",
	"telnet",
	"w3m",
	"wget",
	"xh",

	// System administration - prevent privilege escalation
	"doas",
	"su",
	"sudo",
	"pkexec",
	"gksudo",
	"kdesudo",

	// Package managers - prevent system modification
	"apk",
	"apt",
	"apt-cache",
	"apt-get",
	"dnf",
	"dpkg",
	"emerge",
	"home-manager",
	"makepkg",
	"opkg",
	"pacman",
	"paru",
	"pkg",
	"pkg_add",
	"pkg_delete",
	"portage",
	"rpm",
	"yay",
	"yum",
	"zypper",
	"snap",
	"flatpak",
	"nix-env",

	// System modification - prevent system changes
	"at",
	"batch",
	"chkconfig",
	"crontab",
	"fdisk",
	"mkfs",
	"mount",
	"parted",
	"service",
	"systemctl",
	"umount",
	"shutdown",
	"reboot",
	"poweroff",
	"init",
	"telinit",

	// Network configuration - prevent network changes
	"firewall-cmd",
	"ifconfig",
	"ip",
	"iptables",
	"ip6tables",
	"nft",
	"netstat",
	"pfctl",
	"route",
	"ufw",
	"nmcli",
	"networkctl",

	// User/group management - prevent identity changes
	"useradd",
	"userdel",
	"usermod",
	"groupadd",
	"groupdel",
	"groupmod",
	"passwd",
	"chpasswd",
	"adduser",
	"deluser",

	// Dangerous file operations
	"shred",
	"wipe",
	"dd",
	"losetup",

	// Process/capability manipulation
	"setcap",
	"getcap",
	"chroot",
	"unshare",
	"nsenter",
}

// BannedSubcommands defines specific subcommand patterns to block.
// Format: command, subcommand args, flags (any of which triggers block)
type BannedSubcommand struct {
	Command string
	Args    []string // Subcommand arguments (e.g., ["install"])
	Flags   []string // Flags that trigger block (e.g., ["--global", "-g"])
}

// BannedSubcommandPatterns blocks specific subcommand patterns even if the base command is allowed.
var BannedSubcommandPatterns = []BannedSubcommand{
	// System package managers (redundant but explicit)
	{Command: "apk", Args: []string{"add"}},
	{Command: "apt", Args: []string{"install"}},
	{Command: "apt-get", Args: []string{"install"}},
	{Command: "dnf", Args: []string{"install"}},
	{Command: "pacman", Flags: []string{"-S"}},
	{Command: "pkg", Args: []string{"install"}},
	{Command: "yum", Args: []string{"install"}},
	{Command: "zypper", Args: []string{"install"}},

	// Language-specific package managers - block global installs only
	{Command: "brew", Args: []string{"install"}},
	{Command: "cargo", Args: []string{"install"}},
	{Command: "gem", Args: []string{"install"}},
	{Command: "go", Args: []string{"install"}},
	{Command: "npm", Args: []string{"install"}, Flags: []string{"--global", "-g"}},
	{Command: "pip", Args: []string{"install"}, Flags: []string{"--user", "--system"}},
	{Command: "pip3", Args: []string{"install"}, Flags: []string{"--user", "--system"}},
	{Command: "pnpm", Args: []string{"add"}, Flags: []string{"--global", "-g"}},
	{Command: "yarn", Args: []string{"global", "add"}},

	// Dangerous go test usage (arbitrary command execution)
	{Command: "go", Args: []string{"test"}, Flags: []string{"-exec"}},

	// Git config manipulation (could change hooks, credentials)
	{Command: "git", Args: []string{"config", "--global"}},
	{Command: "git", Args: []string{"config", "--system"}},

	// Docker/container escape attempts
	{Command: "docker", Args: []string{"run"}, Flags: []string{"--privileged"}},
	{Command: "docker", Args: []string{"run"}, Flags: []string{"-v", "/:/"}},
	{Command: "podman", Args: []string{"run"}, Flags: []string{"--privileged"}},
}

// DangerousPipePatterns detects dangerous command chaining.
var DangerousPipePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)curl\s+.+\|\s*(ba)?sh`),
	regexp.MustCompile(`(?i)wget\s+.+\|\s*(ba)?sh`),
	regexp.MustCompile(`(?i)curl\s+.+\|\s*python`),
	regexp.MustCompile(`(?i)wget\s+.+\|\s*python`),
	regexp.MustCompile(`(?i)\|\s*sudo\b`),
	regexp.MustCompile(`(?i)\|\s*su\b`),
	regexp.MustCompile(`(?i)\|\s*base64\s+-d\s*\|\s*(ba)?sh`), // Encoded payload execution
}

// BashChecker provides two-step bash command security checking.
type BashChecker struct {
	// AllowedDirs from agent.toml - directories the agent can access
	AllowedDirs []string

	// UserDeniedCommands from agent.toml - additional commands to block
	UserDeniedCommands []string

	// LLMChecker is called for step 2 (semantic check) if step 1 passes
	// Takes: command string, allowed_dirs []string
	// Returns: (allowed bool, reason string, error)
	LLMChecker func(ctx context.Context, command string, allowedDirs []string) (bool, string, error)

	// Workspace is the base working directory
	Workspace string

	// OnDecision is called after each security decision for logging/auditing.
	// step: "deterministic" or "llm"
	// allowed: whether the command was allowed
	// reason: explanation (especially for blocks)
	OnDecision func(command, step string, allowed bool, reason string)
}

// NewBashChecker creates a new bash security checker.
func NewBashChecker(workspace string, allowedDirs, userDeniedCommands []string) *BashChecker {
	return &BashChecker{
		AllowedDirs:        allowedDirs,
		UserDeniedCommands: userDeniedCommands,
		Workspace:          workspace,
	}
}

// LLMPolicyChecker is an interface for LLM-based policy checking.
type LLMPolicyChecker interface {
	// CheckBashCommand asks the LLM if a bash command violates directory policy.
	// Returns (allowed, reason, error)
	CheckBashCommand(ctx context.Context, command string, allowedDirs []string) (bool, string, error)
}

// SetLLMChecker sets the LLM checker for directory policy verification.
func (c *BashChecker) SetLLMChecker(checker LLMPolicyChecker) {
	c.LLMChecker = func(ctx context.Context, command string, allowedDirs []string) (bool, string, error) {
		return checker.CheckBashCommand(ctx, command, allowedDirs)
	}
}

// Check performs two-step security checking on a bash command.
// Step 1: Deterministic denylist check (fast, zero LLM cost)
// Step 2: LLM policy check (only if step 1 passes and LLMChecker is configured)
func (c *BashChecker) Check(ctx context.Context, command string) (bool, string, error) {
	// Step 1: Deterministic checks
	allowed, reason := c.checkDeterministic(command)
	if !allowed {
		if c.OnDecision != nil {
			c.OnDecision(command, "deterministic", false, reason)
		}
		return false, reason, nil
	}
	if c.OnDecision != nil {
		c.OnDecision(command, "deterministic", true, "")
	}

	// Step 2: LLM policy check (if configured)
	if c.LLMChecker != nil && len(c.AllowedDirs) > 0 {
		allowed, reason, err := c.LLMChecker(ctx, command, c.AllowedDirs)
		if err != nil {
			if c.OnDecision != nil {
				c.OnDecision(command, "llm", false, fmt.Sprintf("error: %v", err))
			}
			return false, fmt.Sprintf("LLM policy check failed: %v", err), err
		}
		if c.OnDecision != nil {
			c.OnDecision(command, "llm", allowed, reason)
		}
		if !allowed {
			return false, reason, nil
		}
	}

	return true, "", nil
}

// CheckDeterministic performs only the fast deterministic checks (for testing/preview).
func (c *BashChecker) CheckDeterministic(command string) (bool, string) {
	return c.checkDeterministic(command)
}

func (c *BashChecker) checkDeterministic(command string) (bool, string) {
	// Normalize command
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return false, "empty command"
	}

	// Check dangerous pipe patterns first
	for _, pattern := range DangerousPipePatterns {
		if pattern.MatchString(cmd) {
			return false, fmt.Sprintf("dangerous pipe pattern detected: %s", pattern.String())
		}
	}

	// Check if command has shell metacharacters (pipes, chains, etc.)
	// If so, we need to check ALL segments, not just the first command
	if containsUnquotedMetachars(cmd) {
		// Check each segment of a piped/chained command
		segments := splitCommandSegments(cmd)
		for _, seg := range segments {
			segBase := extractBaseCommand(seg)
			for _, banned := range BannedCommands {
				if segBase == banned {
					return false, fmt.Sprintf("command '%s' is blocked for security", banned)
				}
			}
			for _, denied := range c.UserDeniedCommands {
				if segBase == denied {
					return false, fmt.Sprintf("command '%s' is blocked by policy", denied)
				}
			}
			// Check subcommand patterns for each segment
			if blocked, reason := c.checkSubcommandPatterns(seg); blocked {
				return false, reason
			}
		}
		return true, ""
	}

	// Simple command (no metacharacters) - just check the base command
	baseCmd := extractBaseCommand(cmd)

	// Check against banned commands
	for _, banned := range BannedCommands {
		if baseCmd == banned {
			return false, fmt.Sprintf("command '%s' is blocked for security", banned)
		}
	}

	// Check against user-defined denied commands
	for _, denied := range c.UserDeniedCommands {
		if baseCmd == denied {
			return false, fmt.Sprintf("command '%s' is blocked by policy", denied)
		}
	}

	// Check banned subcommand patterns
	if blocked, reason := c.checkSubcommandPatterns(cmd); blocked {
		return false, reason
	}

	return true, ""
}

// checkSubcommandPatterns checks if command matches any banned subcommand pattern.
func (c *BashChecker) checkSubcommandPatterns(cmd string) (blocked bool, reason string) {
	words := strings.Fields(cmd)
	if len(words) == 0 {
		return false, ""
	}

	baseCmd := words[0]

	for _, pattern := range BannedSubcommandPatterns {
		if baseCmd != pattern.Command {
			continue
		}

		// Check if all required args are present
		if len(pattern.Args) > 0 {
			argsMatched := 0
			for _, arg := range pattern.Args {
				for _, word := range words[1:] {
					if word == arg {
						argsMatched++
						break
					}
				}
			}
			if argsMatched < len(pattern.Args) {
				continue // Not all required args present
			}
		}

		// If no flags required, block now
		if len(pattern.Flags) == 0 {
			return true, fmt.Sprintf("command pattern '%s %s' is blocked", pattern.Command, strings.Join(pattern.Args, " "))
		}

		// Check if any of the blocking flags are present
		for _, flag := range pattern.Flags {
			for _, word := range words[1:] {
				if word == flag || strings.HasPrefix(word, flag+"=") {
					return true, fmt.Sprintf("command '%s' with '%s' is blocked", pattern.Command, flag)
				}
			}
		}
	}

	return false, ""
}

// extractBaseCommand gets the first command from a potentially piped/chained command.
func extractBaseCommand(cmd string) string {
	// Handle leading whitespace
	cmd = strings.TrimSpace(cmd)

	// Handle env command prefix (e.g., "env VAR=val command")
	if strings.HasPrefix(cmd, "env ") {
		words := strings.Fields(cmd)
		for i, w := range words[1:] {
			if !strings.Contains(w, "=") {
				return words[i+1]
			}
		}
	}

	// Get first word
	words := strings.Fields(cmd)
	if len(words) == 0 {
		return ""
	}

	// Strip path prefix (e.g., /usr/bin/curl -> curl)
	base := words[0]
	if idx := strings.LastIndex(base, "/"); idx != -1 {
		base = base[idx+1:]
	}

	return base
}

// containsUnquotedMetachars checks for shell metacharacters outside of quotes.
func containsUnquotedMetachars(cmd string) bool {
	metachars := []string{"|", "&&", "||", ";", "`", "$(", "${"}
	inSingle := false
	inDouble := false

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if c == '\'' && !inDouble {
			inSingle = !inSingle
		} else if c == '"' && !inSingle {
			inDouble = !inDouble
		}

		if !inSingle && !inDouble {
			remaining := cmd[i:]
			for _, meta := range metachars {
				if strings.HasPrefix(remaining, meta) {
					return true
				}
			}
		}
	}

	return false
}

// splitCommandSegments splits a command by pipes, semicolons, and logical operators.
func splitCommandSegments(cmd string) []string {
	var segments []string
	current := ""
	inSingle := false
	inDouble := false

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			current += string(c)
		} else if c == '"' && !inSingle {
			inDouble = !inDouble
			current += string(c)
		} else if !inSingle && !inDouble {
			remaining := cmd[i:]
			// Check for multi-character operators first
			if strings.HasPrefix(remaining, "&&") || strings.HasPrefix(remaining, "||") {
				if strings.TrimSpace(current) != "" {
					segments = append(segments, strings.TrimSpace(current))
				}
				current = ""
				i++ // Skip the second character of && or ||
				continue
			}
			// Single character separators
			if c == '|' || c == ';' {
				if strings.TrimSpace(current) != "" {
					segments = append(segments, strings.TrimSpace(current))
				}
				current = ""
				continue
			}
			current += string(c)
		} else {
			current += string(c)
		}
	}

	if strings.TrimSpace(current) != "" {
		segments = append(segments, strings.TrimSpace(current))
	}

	return segments
}

// MergeUserDenylist merges user-defined denied commands with the built-in list,
// returning the combined set (with duplicates removed).
func MergeUserDenylist(userDenied []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(BannedCommands)+len(userDenied))

	for _, cmd := range BannedCommands {
		if !seen[cmd] {
			seen[cmd] = true
			result = append(result, cmd)
		}
	}

	for _, cmd := range userDenied {
		if !seen[cmd] {
			seen[cmd] = true
			result = append(result, cmd)
		}
	}

	return result
}
