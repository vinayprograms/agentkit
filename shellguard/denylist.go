package shellguard

import (
	"fmt"
	"regexp"
	"strings"
)

// BannedCommands is the hardcoded list of commands that are always blocked.
var BannedCommands = []string{
	// Network/Download tools - prevent data exfiltration and arbitrary downloads
	"alias", "aria2c", "axel", "chrome", "curl", "curlie", "firefox",
	"http-prompt", "httpie", "links", "lynx", "nc", "netcat", "ncat",
	"safari", "scp", "sftp", "ssh", "telnet", "w3m", "wget", "xh",

	// System administration - prevent privilege escalation
	"doas", "su", "sudo", "pkexec", "gksudo", "kdesudo",

	// Package managers - prevent system modification
	"apk", "apt", "apt-cache", "apt-get", "dnf", "dpkg", "emerge",
	"home-manager", "makepkg", "opkg", "pacman", "paru", "pkg",
	"pkg_add", "pkg_delete", "portage", "rpm", "yay", "yum", "zypper",
	"snap", "flatpak", "nix-env",

	// System modification - prevent system changes
	"at", "batch", "chkconfig", "crontab", "fdisk", "mkfs", "mount",
	"parted", "service", "systemctl", "umount", "shutdown", "reboot",
	"poweroff", "init", "telinit",

	// Network configuration - prevent network changes
	"firewall-cmd", "ifconfig", "ip", "iptables", "ip6tables", "nft",
	"netstat", "pfctl", "route", "ufw", "nmcli", "networkctl",

	// User/group management - prevent identity changes
	"useradd", "userdel", "usermod", "groupadd", "groupdel", "groupmod",
	"passwd", "chpasswd", "adduser", "deluser",

	// Dangerous file operations
	"shred", "wipe", "dd", "losetup",

	// Process/capability manipulation
	"setcap", "getcap", "chroot", "unshare", "nsenter",
}

// BannedSubcommand defines specific subcommand patterns to block.
type BannedSubcommand struct {
	Command string
	Args    []string
	Flags   []string
}

// BannedSubcommandPatterns blocks specific subcommand patterns even if the base command is allowed.
var BannedSubcommandPatterns = []BannedSubcommand{
	{Command: "brew", Args: []string{"install"}},
	{Command: "cargo", Args: []string{"install"}},
	{Command: "gem", Args: []string{"install"}},
	{Command: "go", Args: []string{"install"}},
	{Command: "npm", Args: []string{"install"}, Flags: []string{"--global", "-g"}},
	{Command: "pip", Args: []string{"install"}, Flags: []string{"--user", "--system"}},
	{Command: "pip3", Args: []string{"install"}, Flags: []string{"--user", "--system"}},
	{Command: "pnpm", Args: []string{"add"}, Flags: []string{"--global", "-g"}},
	{Command: "yarn", Args: []string{"global", "add"}},
	{Command: "go", Args: []string{"test"}, Flags: []string{"-exec"}},
	{Command: "git", Args: []string{"config", "--global"}},
	{Command: "git", Args: []string{"config", "--system"}},
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

// checkDeterministic runs fast, zero-cost security checks.
func (g *Gate) checkDeterministic(command string) (bool, string) {
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
	if g.shell.HasChainedCommands(cmd) {
		segments = g.shell.SplitSegments(cmd)
	}

	for _, seg := range segments {
		if blocked, reason := g.checkSegment(seg); blocked {
			return false, reason
		}
	}

	return true, ""
}

func (g *Gate) checkSegment(seg string) (bool, string) {
	base := g.shell.ExtractCommand(seg)

	for _, banned := range BannedCommands {
		if base == banned {
			return true, fmt.Sprintf("command '%s' is blocked for security", banned)
		}
	}
	for _, denied := range g.userDeniedCommands {
		if base == denied {
			return true, fmt.Sprintf("command '%s' is blocked by policy", denied)
		}
	}

	return g.checkSubcommandPatterns(seg)
}

func (g *Gate) checkSubcommandPatterns(cmd string) (blocked bool, reason string) {
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

