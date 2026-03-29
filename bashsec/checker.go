// Package bashsec provides bash command security checking with a two-step pipeline:
// deterministic denylist checks followed by optional LLM-based path analysis.
package bashsec

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// BannedCommands is the hardcoded list of commands that are always blocked.
// These are dangerous for system security and should never be executed by an agent.
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

// BannedSubcommand defines specific subcommand patterns to block.
type BannedSubcommand struct {
	Command string
	Args    []string // Subcommand arguments (e.g., ["install"])
	Flags   []string // Flags that trigger block (e.g., ["--global", "-g"])
}

// BannedSubcommandPatterns blocks specific subcommand patterns even if the base command is allowed.
// Commands already in BannedCommands are not listed here — they're caught earlier.
var BannedSubcommandPatterns = []BannedSubcommand{
	// Language-specific package managers - block global/system installs
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
	regexp.MustCompile(`(?i)\|\s*base64\s+-d\s*\|\s*(ba)?sh`),
}

// Checker provides two-step bash command security checking.
type Checker struct {
	// AllowedDirs returns the directories the agent may write to.
	// Called during LLM check (step 2). Nil means no LLM path checking.
	AllowedDirs []string

	// UserDeniedCommands are additional commands to block (from policy.toml).
	UserDeniedCommands []string

	// LLMChecker is called for step 2 (semantic check) if step 1 passes.
	LLMChecker func(ctx context.Context, command string, allowedDirs []string) (*CheckResult, error)

	// Workspace is the base working directory.
	Workspace string

	// OnDecision is called after each security decision for logging/auditing.
	OnDecision func(command, step string, allowed bool, reason string, durationMs int64, inputTokens, outputTokens int)
}

// NewChecker creates a new bash security checker.
func NewChecker(workspace string, allowedDirs, userDeniedCommands []string) *Checker {
	return &Checker{
		AllowedDirs:        allowedDirs,
		UserDeniedCommands: userDeniedCommands,
		Workspace:          workspace,
	}
}

// LLMPolicyChecker is an interface for LLM-based policy checking.
type LLMPolicyChecker interface {
	CheckBashCommand(ctx context.Context, command string, allowedDirs []string, workingDir string) (*CheckResult, error)
}

// SetLLMChecker sets the LLM checker for directory policy verification.
func (c *Checker) SetLLMChecker(checker LLMPolicyChecker) {
	c.LLMChecker = func(ctx context.Context, command string, allowedDirs []string) (*CheckResult, error) {
		return checker.CheckBashCommand(ctx, command, allowedDirs, c.Workspace)
	}
}

// Check runs the two-tier security pipeline: deterministic then LLM.
func (c *Checker) Check(ctx context.Context, command string) (bool, string, error) {
	allowed, reason := c.checkDeterministic(command)
	if !allowed {
		if c.OnDecision != nil {
			c.OnDecision(command, "deterministic", false, reason, 0, 0, 0)
		}
		return false, reason, nil
	}
	if c.OnDecision != nil {
		c.OnDecision(command, "deterministic", true, "", 0, 0, 0)
	}

	if c.LLMChecker != nil && len(c.AllowedDirs) > 0 {
		start := time.Now()
		result, err := c.LLMChecker(ctx, command, c.AllowedDirs)
		durationMs := time.Since(start).Milliseconds()
		if err != nil {
			if c.OnDecision != nil {
				c.OnDecision(command, "llm", false, fmt.Sprintf("error: %v", err), durationMs, 0, 0)
			}
			return false, fmt.Sprintf("LLM policy check failed: %v", err), err
		}
		if c.OnDecision != nil {
			c.OnDecision(command, "llm", result.Allowed, result.Reason, durationMs, result.InputTokens, result.OutputTokens)
		}
		if !result.Allowed {
			return false, result.Reason, nil
		}
	}

	return true, "", nil
}

// CheckDeterministic performs only the fast deterministic checks (for testing/preview).
func (c *Checker) CheckDeterministic(command string) (bool, string) {
	return c.checkDeterministic(command)
}

func (c *Checker) checkDeterministic(command string) (bool, string) {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return false, "empty command"
	}

	for _, pattern := range DangerousPipePatterns {
		if pattern.MatchString(cmd) {
			return false, fmt.Sprintf("dangerous pipe pattern detected: %s", pattern.String())
		}
	}

	segments := []string{cmd}
	if containsUnquotedMetachars(cmd) {
		segments = splitCommandSegments(cmd)
	}

	for _, seg := range segments {
		if blocked, reason := c.checkSegment(seg); blocked {
			return false, reason
		}
	}

	return true, ""
}

func (c *Checker) checkSegment(seg string) (bool, string) {
	base := extractBaseCommand(seg)

	for _, banned := range BannedCommands {
		if base == banned {
			return true, fmt.Sprintf("command '%s' is blocked for security", banned)
		}
	}
	for _, denied := range c.UserDeniedCommands {
		if base == denied {
			return true, fmt.Sprintf("command '%s' is blocked by policy", denied)
		}
	}

	return c.checkSubcommandPatterns(seg)
}

func (c *Checker) checkSubcommandPatterns(cmd string) (blocked bool, reason string) {
	words := strings.Fields(cmd)
	if len(words) == 0 {
		return false, ""
	}

	baseCmd := words[0]

	for _, pattern := range BannedSubcommandPatterns {
		if baseCmd != pattern.Command {
			continue
		}

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
				continue
			}
		}

		if len(pattern.Flags) == 0 {
			return true, fmt.Sprintf("command pattern '%s %s' is blocked", pattern.Command, strings.Join(pattern.Args, " "))
		}

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

func extractBaseCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)

	if strings.HasPrefix(cmd, "env ") {
		words := strings.Fields(cmd)
		for i, w := range words[1:] {
			if !strings.Contains(w, "=") {
				return words[i+1]
			}
		}
	}

	words := strings.Fields(cmd)
	if len(words) == 0 {
		return ""
	}

	base := words[0]
	if idx := strings.LastIndex(base, "/"); idx != -1 {
		base = base[idx+1:]
	}

	return base
}

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
			if strings.HasPrefix(remaining, "&&") || strings.HasPrefix(remaining, "||") {
				if strings.TrimSpace(current) != "" {
					segments = append(segments, strings.TrimSpace(current))
				}
				current = ""
				i++
				continue
			}
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
