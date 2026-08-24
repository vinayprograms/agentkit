package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// safeGitSubcommands is the supported surface for this tool, not a safety
// boundary — actual safety comes from dangerousGitFlags below, which blocks
// specific destructive flags regardless of subcommand. This list exists to
// keep the tool from attempting subcommands it doesn't know how to run
// sensibly (e.g. interactive ones), and to give a useful error/description.
// It intentionally includes write subcommands (add, commit, push, ...) that
// policy is expected to gate separately; read-only plumbing commands are
// included alongside the porcelain ones so read-only investigation (log
// analysis, diffing refs, etc.) doesn't get rejected while writes are
// allowed.
var safeGitSubcommands = map[string]bool{
	"status":       true,
	"diff":         true,
	"log":          true,
	"show":         true,
	"add":          true,
	"commit":       true,
	"push":         true,
	"pull":         true,
	"fetch":        true,
	"branch":       true,
	"checkout":     true,
	"switch":       true,
	"stash":        true,
	"tag":          true,
	"remote":       true,
	"rev-parse":    true,
	"rev-list":     true,
	"cat-file":     true,
	"describe":     true,
	"for-each-ref": true,
	"shortlog":     true,
	"blame":        true,
	"ls-files":     true,
	"ls-tree":      true,
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
	return "Run safe git commands. Allowed subcommands: status, diff, log, show, add, commit, push, pull, fetch, branch, checkout, switch, stash, tag, remote, rev-parse, rev-list, cat-file, describe, for-each-ref, shortlog, blame, ls-files, ls-tree. Dangerous flags like --force and --hard are blocked. shortlog defaults to HEAD when no revision is given."
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

	// `git shortlog` reads the revision list from stdin when none is given
	// on the command line and no TTY is attached (as is always the case
	// here) — the result is silent, empty output rather than an error.
	// Default to HEAD so the tool behaves as a caller asking for "shortlog"
	// would expect.
	if subcommand == "shortlog" && !hasRevisionArg(parts[1:]) {
		parts = append(parts, "HEAD")
	}

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
		return "", fmt.Errorf("git %s failed: %s\n%s", subcommand, err, truncateGitError(string(output)))
	}

	result := strings.TrimSpace(string(output))
	if result == "" {
		return fmt.Sprintf("git %s completed (no output)", subcommand), nil
	}

	return result, nil
}

// hasRevisionArg reports whether args (the subcommand's arguments) already
// contain something that looks like a revision or path — i.e. any token
// that isn't a flag. Used to decide whether shortlog needs a default HEAD
// appended.
func hasRevisionArg(args []string) bool {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return true
		}
	}
	return false
}

// gitErrorMaxBytes caps how much of a failed git command's combined output
// is fed back into model context. Some failures (e.g. `git diff --no-index`
// exiting 129) print a full usage page — several KB of text the model
// doesn't need and that just burns context.
const gitErrorMaxBytes = 500

// truncateGitError keeps the first line (the actual error message in most
// git failures) plus a capped amount of the remaining output, so long usage
// dumps don't get fed verbatim back into model context.
func truncateGitError(output string) string {
	output = strings.TrimSpace(output)
	if len(output) <= gitErrorMaxBytes {
		return output
	}
	firstLine, rest := output, ""
	if idx := strings.IndexByte(output, '\n'); idx != -1 {
		firstLine, rest = output[:idx], output[idx+1:]
	}
	budget := gitErrorMaxBytes - len(firstLine)
	if budget <= 0 {
		return firstLine[:min(len(firstLine), gitErrorMaxBytes)] + " ...[truncated]"
	}
	rest = strings.TrimSpace(rest)
	if len(rest) > budget {
		rest = rest[:budget] + " ...[truncated]"
	}
	if rest == "" {
		return firstLine
	}
	return firstLine + "\n" + rest
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
