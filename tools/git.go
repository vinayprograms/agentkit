package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Safe git subcommands - read-only or standard workflow operations.
var safeGitSubcommands = map[string]bool{
	"status":    true,
	"diff":      true,
	"log":       true,
	"show":      true,
	"add":       true,
	"commit":    true,
	"push":      true,
	"pull":      true,
	"fetch":     true,
	"branch":    true,
	"checkout":  true,
	"switch":    true,
	"stash":     true,
	"tag":       true,
	"remote":    true,
	"rev-parse": true,
	"shortlog":  true,
	"blame":     true,
	"ls-files":  true,
	"ls-tree":   true,
}

// Dangerous git flags that are always blocked.
var dangerousGitFlags = []string{
	"--force",
	"-f",     // force push
	"--hard", // reset --hard
	"--mirror",
	"--no-verify", // skip hooks
	"filter-branch",
	"reflog",
	"gc",
	"prune",
	"fsck",
	"rebase",      // can rewrite history
	"cherry-pick", // can be safe but complex
	"reset",       // dangerous without --soft
	"clean",       // deletes untracked files
	"rm",          // use the rm tool instead
}

type gitTool struct {
	workspace string
}

// Git creates a git tool that runs safe git commands.
// If workspace is non-empty, it is used as the default working directory.
func Git(workspace string) Tool {
	return &gitTool{workspace: workspace}
}

func (t *gitTool) Name() string { return "git" }

func (t *gitTool) Description() string {
	return "Run safe git commands. Allowed subcommands: status, diff, log, show, add, commit, push, pull, fetch, branch, checkout, switch, stash, tag, remote, rev-parse, shortlog, blame, ls-files, ls-tree. Dangerous flags like --force and --hard are blocked."
}

func (t *gitTool) Parameters() map[string]Param {
	return map[string]Param{
		"args": {
			Type:        StringParam,
			Description: `Git arguments (e.g., 'status', 'diff --stat', 'commit -m "message"', 'log --oneline -10')`,
			Required:    true,
		},
		"cwd": {
			Type:        StringParam,
			Description: "Working directory for the git command (optional, defaults to workspace)",
		},
	}
}

func (t *gitTool) Execute(ctx context.Context, args Args) (string, error) {
	gitArgs, err := args.String("args")
	if err != nil {
		return "", err
	}

	cwd := args.StringOr("cwd", t.workspace)

	// Parse and validate the git command
	parts := parseGitArgs(gitArgs)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty git command")
	}

	subcommand := parts[0]

	// Check subcommand allowlist
	if !safeGitSubcommands[subcommand] {
		return "", fmt.Errorf("git subcommand %q is not allowed. Safe subcommands: %s",
			subcommand, safeSubcommandList())
	}

	// Check for dangerous flags
	for _, part := range parts {
		for _, dangerous := range dangerousGitFlags {
			if part == dangerous {
				return "", fmt.Errorf("git flag %q is blocked for safety", part)
			}
		}
	}

	// Execute
	cmd := exec.CommandContext(ctx, "git", parts...)
	if cwd != "" {
		cmd.Dir = cwd
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %s\n%s", subcommand, err, string(output))
	}

	result := strings.TrimSpace(string(output))
	if result == "" {
		return fmt.Sprintf("git %s completed (no output)", subcommand), nil
	}

	return result, nil
}

// parseGitArgs splits git arguments respecting quotes.
func parseGitArgs(s string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			if c == quoteChar {
				inQuote = false
			} else {
				current.WriteByte(c)
			}
		} else if c == '"' || c == '\'' {
			inQuote = true
			quoteChar = c
		} else if c == ' ' || c == '\t' {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		} else {
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

func safeSubcommandList() string {
	var cmds []string
	for cmd := range safeGitSubcommands {
		cmds = append(cmds, cmd)
	}
	return strings.Join(cmds, ", ")
}
