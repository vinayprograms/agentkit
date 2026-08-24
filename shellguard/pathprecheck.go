package shellguard

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// safeBaseCommands is the small, explicit, read-only-or-in-bounds safelist
// of base commands the path pre-check may ever skip the LLM for. This list
// must stay small: adding to it widens what can bypass LLM review, so any
// addition needs the same scrutiny as the pre-check design itself.
var safeBaseCommands = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true, "wc": true,
	"grep": true, "find": true, "pwd": true, "echo": true,
	"mkdir": true, "touch": true,
	"git": true, "go": true,
}

// safeGitSubcommands / safeGoSubcommands further restrict git/go to
// read-only-or-in-bounds subcommands. Any other subcommand falls through to
// the LLM.
var safeGitSubcommands = map[string]bool{"status": true, "log": true, "diff": true, "show": true}
var safeGoSubcommands = map[string]bool{"build": true, "test": true, "vet": true}

// dangerousFindFlags can make `find` execute or modify arbitrary paths, so
// their presence disqualifies the command from the pre-check regardless of
// how innocuous the rest of the invocation looks.
var dangerousFindFlags = []string{"-exec", "-execdir", "-ok", "-okdir", "-delete", "-fprint", "-fprintf", "-fls"}

// bareVarRe matches an unbraced shell variable reference ($VAR). "$(" and
// "${" (command substitution / braced expansion) are already caught by
// Shell.HasChainedCommands, which the pre-check calls first.
var bareVarRe = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*`)

// credentialPathRe matches path text that must never be pre-check-allowed,
// mirroring the LLM prompt's credential rule (llm.go rule 2). A command
// touching one of these always falls through to the LLM (which blocks it)
// rather than being pre-check-skipped.
var credentialPathRe = regexp.MustCompile(`(?i)(^|[/.])\.ssh(/|$)|(^|[/.])\.aws(/|$)|credentials|secrets?|\.pem$|\.key$|(^|/)\.env(\.|$)|id_rsa|id_ed25519`)

// pathPrecheck is a conservative deterministic pre-check for path
// boundaries. It runs after the deterministic denylist stage (checkDeterministic)
// and before the LLM review. It can ONLY EVER conclude "provably in bounds,
// skip the LLM" — it never blocks anything itself and never overrides a
// block. Any doubt at all — an unparseable argument, a path outside
// allowedDirs/tmp, a base command or subcommand outside the safelist, any
// dynamic construct that could hide or construct a path at runtime — falls
// through to the LLM exactly as before this pre-check existed.
func (g *Gate) pathPrecheck(command string) (skip bool, reason string) {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return false, ""
	}

	// Any chaining, piping, or substitution metacharacter disqualifies the
	// whole command: it can hide or construct a path (or run a second,
	// unreviewed command) at runtime. This also catches backticks, $(...)
	// and ${...}.
	if g.shell.HasChainedCommands(cmd) {
		return false, "command contains chaining/piping/substitution metacharacters"
	}
	if strings.ContainsAny(cmd, "*?[]<>") {
		return false, "command contains a glob or redirection metacharacter"
	}
	if bareVarRe.MatchString(cmd) {
		return false, "command contains a variable expansion"
	}
	if containsWord(cmd, "eval") {
		return false, "command contains eval"
	}

	words := strings.Fields(cmd)
	if len(words) == 0 {
		return false, ""
	}

	base := g.shell.ExtractCommand(cmd)
	if !safeBaseCommands[base] {
		return false, fmt.Sprintf("base command %q is not on the pre-check safelist", base)
	}

	argStart := 1
	switch base {
	case "git":
		if len(words) < 2 || !safeGitSubcommands[words[1]] {
			return false, "git subcommand not on the pre-check safelist"
		}
		argStart = 2
	case "go":
		if len(words) < 2 || !safeGoSubcommands[words[1]] {
			return false, "go subcommand not on the pre-check safelist"
		}
		argStart = 2
	case "find":
		for _, w := range words[1:] {
			for _, f := range dangerousFindFlags {
				if w == f {
					return false, "find with an action flag can execute or modify arbitrary paths"
				}
			}
		}
	}

	for _, w := range words[argStart:] {
		if strings.HasPrefix(w, "-") {
			continue // flag, not a path
		}
		if credentialPathRe.MatchString(w) {
			return false, "argument looks like a credential path"
		}

		arg := strings.TrimSuffix(w, "/...") // go's "./..." package pattern
		if arg == "" {
			arg = "."
		}

		resolved, err := g.resolvePathArg(arg)
		if err != nil {
			return false, "could not resolve argument as a path"
		}
		if credentialPathRe.MatchString(resolved) {
			return false, "resolved path looks like a credential path"
		}
		if !g.pathInBounds(resolved) {
			return false, fmt.Sprintf("path %q is outside allowed_dirs/tmp", resolved)
		}
	}

	return true, "deterministic path pre-check: safelisted command, all path arguments resolve inside allowed_dirs/tmp, no dynamic constructs"
}

// resolvePathArg expands ~, then resolves a relative path against the
// workspace, then canonicalizes (cleans) the result. It does not touch the
// filesystem — this is purely lexical resolution, matching the LLM prompt's
// own resolution rules (llm.go rules 5 and 9), which is deliberate: a
// symlink-following resolution would need a live filesystem and could
// itself be racy, whereas lexical resolution is exactly what path
// traversal (`..`) needs and is available.
func (g *Gate) resolvePathArg(arg string) (string, error) {
	p := arg
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			p = home
		} else {
			p = filepath.Join(home, p[2:])
		}
	} else if !filepath.IsAbs(p) {
		p = filepath.Join(g.workspace, p)
	}
	return filepath.Clean(p), nil
}

// pathInBounds reports whether resolved is inside /tmp or one of
// allowedDirs.
func (g *Gate) pathInBounds(resolved string) bool {
	if isWithinDir(resolved, "/tmp") {
		return true
	}
	for _, d := range g.allowedDirs {
		if isWithinDir(resolved, d) {
			return true
		}
	}
	return false
}

func isWithinDir(p, dir string) bool {
	dir = filepath.Clean(dir)
	if p == dir {
		return true
	}
	return strings.HasPrefix(p, dir+string(filepath.Separator))
}

func containsWord(cmd, word string) bool {
	for _, w := range strings.Fields(cmd) {
		if w == word {
			return true
		}
	}
	return false
}

// CheckPathPrecheck exposes the path pre-check for testing/preview,
// mirroring CheckDeterministic.
func (g *Gate) CheckPathPrecheck(command string) (bool, string) {
	return g.pathPrecheck(command)
}
