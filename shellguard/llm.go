package shellguard

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vinayprograms/agentkit/llm"
)

// llmCheck asks an LLM whether a bash command violates policy: write access,
// data-read access, or the deny list. deniedCommands and disabledTools are
// policy context, not deterministic gates — the deterministic denylist.go
// stage already blocked exact userDeniedCommands base-name matches before
// llmCheck ever runs; here they're repeated so the model can also catch a
// command that ACHIEVES THE SAME EFFECT by another route.
func llmCheck(ctx context.Context, model llm.Model, command string, allowedDirs, deniedCommands, disabledTools []string, workingDir, securityScope string) (*Result, error) {
	var securityContext string
	if securityScope != "" {
		securityContext = fmt.Sprintf(`
SECURITY RESEARCH CONTEXT:
This agent is conducting authorized security research within scope:
"%s"

Commands that fall within this research scope should be ALLOWED even if they
access paths outside the normal allowed directories. Use judgment to determine
if the command is part of legitimate security research.

`, securityScope)
	}

	var denyContext string
	if len(deniedCommands) > 0 {
		denyContext = fmt.Sprintf(`
COMMANDS DENIED BY POLICY (these base commands are blocked outright):
%s

These are already blocked by an exact base-command-name match before this
review runs. Your job here is the gap that check can't cover: block any
command that ACHIEVES THE SAME EFFECT as a denied command by another route —
a python/perl/node/ruby one-liner that shells out or does the same thing
in-process, a shell builtin or alias, a symlink or renamed copy of the
binary, or a base64/hex-decoded script that reconstructs the denied command.
Judge by effect, not by literal command name.

`, strings.Join(deniedCommands, "\n"))
	}

	var toolContext string
	if len(disabledTools) > 0 {
		toolContext = fmt.Sprintf(`
AGENT TOOLS DISABLED BY POLICY:
%s

Bash must not be used as a side door around a disabled tool. Concretely:
- if "write" or "edit" is disabled, BLOCK any bash command that creates,
  modifies, deletes, or appends to a file (see WRITE rules below) — treat
  ALL bash write operations as blocked, not just ones outside allowed_dirs.
- if "read" is disabled, BLOCK any bash DATA READ (see the data-read vs
  toolchain-read distinction below), even inside allowed_dirs — reading
  file contents into the agent's context is exactly what the read tool
  does, and disabling it means disabling it everywhere.

`, strings.Join(disabledTools, "\n"))
	}

	prompt := fmt.Sprintf(`Analyze this bash command for policy violations: writes, data reads, and denied commands.
%s%s%s
WORKING DIRECTORY (cwd where command executes):
%s

ALLOWED DIRECTORIES (agent can write here, and this is also where its data reads are confined):
%s

COMMAND:
%s

RULES:

1. DATA READS vs TOOLCHAIN ACCESS — this is the key judgment call.
   A DATA READ is any command whose PURPOSE is to pull a file's contents
   into the agent's own context or output: cat, head, tail, less, more,
   grep/rg/ag on a specific file, sed -n 'p', awk printing a file, xxd,
   od, base64 of a file, python -c "open(path).read()", node -e
   "fs.readFileSync(...)", diff of two files. Data reads are CONFINED to
   the ALLOWED DIRECTORIES exactly like writes — BLOCK a data read whose
   resolved path is outside them, unless it is toolchain access (below),
   /tmp, or a credential path (also below, which is blocked regardless).
     Example (BLOCK): "cat ~/.bash_history" — purpose is to pull file
     contents into context; ~/.bash_history is outside allowed_dirs.
     Example (BLOCK): "grep -r password /etc" — data read outside allowed_dirs.
   TOOLCHAIN ACCESS is any read OR write that happens INCIDENTALLY while
   EXECUTING a program, not because the agent asked for that file's
   contents or asked to create a file there: a compiler reading its own
   standard library, a package manager reading or populating its module
   cache, an interpreter reading its own stdlib, a linker reading system
   libraries, a build tool reading AND WRITING its build cache. This
   includes writes, not just reads — 'go build'/'go test' compile
   packages into GOCACHE, 'pip install' populates site-packages, 'npm
   install' populates node_modules and its cache: these are toolchain
   writes, and they are ALWAYS OK too, for the same reason toolchain
   reads are. Toolchain access (read and write) is ALWAYS OK, anywhere,
   including: GOROOT, GOCACHE, GOPATH, ~/go/pkg/mod, ~/.cache/go-build,
   Python site-packages and venvs, ~/.cache/pip, __pycache__,
   .pytest_cache, node_modules, npm/yarn/pnpm caches (~/.npm, ~/.cache/yarn),
   ~/.cargo, /usr/lib, /usr/include, /usr/local, Homebrew paths
   (/opt/homebrew, /usr/local/Cellar), Xcode toolchain paths, and any
   similar interpreter/library/module-cache/build-cache path.
     Example (ALLOW): "go build ./..." — reads GOROOT, and writes compiled
     package objects to GOCACHE. Both are toolchain access, even though
     neither is in allowed_dirs.
     Example (ALLOW): "go test ./..." — same as go build: compiles into
     GOCACHE (write) and reads GOROOT/the module cache (read). This is
     the single most common false block to avoid — GOCACHE being outside
     allowed_dirs does NOT make 'go test'/'go build' a write violation.
     Example (ALLOW): "go vet ./...", "python script.py" (reads
     site-packages), "npm test" (reads node_modules), "npm install" or
     "pip install -r requirements.txt" run inside allowed_dirs (writes
     node_modules/site-packages under the project, and populates the
     global npm/pip cache — both toolchain writes).
   The test is INTENT: is the command reading or writing this path
   because it's data the agent wants to inspect or a deliverable it wants
   to produce, or because the tool needs it to run/build at all? When
   genuinely ambiguous, prefer ALLOW for toolchain-shaped paths (module
   caches, stdlib, headers, build caches) and BLOCK for anything that
   looks like deliberately dumping or planting arbitrary file contents
   outside the project.

2. CREDENTIAL / SECRET PATHS — ALWAYS BLOCK, regardless of location, even
   inside allowed_dirs, even as "toolchain access": ~/.ssh, ~/.aws,
   ~/.config/**/credentials*, *.pem, *.key, .env, .env.*, id_rsa*,
   id_ed25519*, any path containing "credentials", "secrets", or a private
   key extension. A build tool has no legitimate reason to need these.

3. WRITE operations (create, modify, delete, mkdir, touch, mv, cp, >, >>)
   are allowed inside the ALLOWED DIRECTORIES listed above, in /tmp, and
   in toolchain cache/module paths (see rule 1) as a byproduct of running
   a compiler/interpreter/package manager. A write ANYWHERE ELSE is
   BLOCKED — an agent deliberately writing a new file to, say, /opt or
   /etc is not a toolchain write and must still be blocked.
4. An allowed directory means the directory AND ALL ITS SUBDIRECTORIES at
   any depth are allowed. Example: if /workspace is allowed, then
   /workspace/src/main.go, /workspace/internal/auth/handler.go, and
   /workspace/a/b/c/d.txt are ALL allowed for read and write. Non-negotiable.
5. Relative paths resolve from WORKING DIRECTORY — check if the resolved
   path is inside an allowed directory.
6. /tmp is always writable and always readable (temporary files and build
   outputs).
7. /dev/null, /dev/zero, /dev/urandom are always writable (system devices).
8. Writing ANYWHERE ELSE is BLOCKED — including /workdir, /opt, /etc,
   /var, /root (unless listed above), /home, or any other path not in the
   allowed list.
9. SECURITY: Watch for path traversal attacks. Paths containing /../ or
   /../../ that escape an allowed directory MUST be resolved to their
   canonical form first. Example: /workspace/../etc/passwd resolves to
   /etc/passwd which is NOT inside /workspace — BLOCK it.
10. If a security research context is provided, commands within that scope
    are OK.

DECISION LOGIC:
For each write path and each read path in the command:
  a. Resolve the full absolute path (expand relative paths from cwd,
     resolve all .. components).
  b. If it's a credential/secret path → BLOCK, no exceptions.
  c. Is it toolchain access (rule 1) — a compiler/interpreter/package
     manager reading or writing its own libraries, stdlib, module cache,
     or build cache? → ALLOW, whether it's a read or a write.
  d. Otherwise, if it's a write → does the resolved path start with any
     ALLOWED DIRECTORY prefix (or /tmp)? If not → BLOCK.
  e. Otherwise, if it's a data read → does the resolved path start with
     any ALLOWED DIRECTORY prefix (or /tmp)? If not → BLOCK.
  f. Also check: does the command achieve the same effect as a denied
     command (see COMMANDS DENIED BY POLICY above, if any)? If so → BLOCK.
If none of the above trigger a BLOCK → ALLOW.

Respond with ONLY a JSON object, nothing else:
{"verdict":"ALLOW"} or {"verdict":"BLOCK","reason":"brief explanation"}`,
		securityContext,
		denyContext,
		toolContext,
		workingDir,
		strings.Join(allowedDirs, "\n"),
		command,
	)

	// Take the decision through a structured tool call rather than parsing
	// prose: llm.Ask pins ToolChoice to verdictTool, forces thinking off (a
	// bounded classification isn't a task that benefits from deliberation),
	// and — for providers/models that can't honor ToolChoice, or that
	// answer in prose anyway — falls back to parseVerdict. It also absorbs
	// the empty-content/StopReason=="length" retry-once behavior this
	// function used to implement inline.
	d, err := llm.Ask(ctx, model, prompt, verdictTool, parseVerdictFallback)
	if err != nil {
		return &Result{Allowed: false, Reason: fmt.Sprintf("LLM check failed: %v", err)}, err
	}

	if d.Content == "" && d.Args == nil {
		// Empty response twice: a reviewer failure is not a denial.
		// llmCheck only runs once the deterministic stage has already
		// allowed this command (see Gate.check), so fall back to that
		// verdict — fail-closed DENY here would turn an unrelated model
		// hiccup into a hard block on a command the deterministic rules
		// already cleared (see shellguard llm.go / P0 8c).
		return &Result{
			Allowed:      true,
			Reason:       "LLM reviewer returned empty response twice; falling back to deterministic ALLOW",
			InputTokens:  d.InputTokens,
			OutputTokens: d.OutputTokens,
		}, nil
	}

	if d.Args == nil {
		// Model answered in prose and parseVerdict couldn't recover a
		// verdict from it either.
		return &Result{Allowed: false, Reason: truncateReason(d.Content), InputTokens: d.InputTokens, OutputTokens: d.OutputTokens}, nil
	}

	allow, _ := d.Args["allow"].(bool)
	reason, _ := d.Args["reason"].(string)
	reason = truncateReason(reason)

	if allow {
		return &Result{Allowed: true, InputTokens: d.InputTokens, OutputTokens: d.OutputTokens}, nil
	}
	if reason == "" {
		reason = "blocked by LLM check"
	}
	return &Result{Allowed: false, Reason: reason, InputTokens: d.InputTokens, OutputTokens: d.OutputTokens}, nil
}

// verdictTool is the structured-decision tool llmCheck asks the model to
// call: an explicit allow/reason pair instead of a verdict scraped from
// prose.
var verdictTool = llm.ToolDef{
	Name:        "verdict",
	Description: "Report the write-access-violation verdict for the analyzed command.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"allow":  map[string]any{"type": "boolean", "description": "true to ALLOW the command, false to BLOCK it"},
			"reason": map[string]any{"type": "string", "description": "brief explanation, required when allow is false"},
		},
		"required": []string{"allow"},
	},
}

// parseVerdictFallback adapts parseVerdict to llm.ParseFallback: it's the
// prose fallback llm.Ask uses when the model answers in text instead of
// calling verdictTool.
func parseVerdictFallback(content string) (map[string]any, bool) {
	verdict, reason := parseVerdict(content)
	if verdict == "" {
		return nil, false
	}
	return map[string]any{"allow": verdict == "ALLOW", "reason": reason}, true
}

type verdictResponse struct {
	Verdict string `json:"verdict"`
	Reason  string `json:"reason,omitempty"`
}

// maxReasonLen bounds the verdict reason text surfaced to the caller (and
// from there, often back into an agent's context or a session log). A
// reasoning model's chain-of-thought leaking into resp.Content can run to
// 900+ chars; nothing downstream needs more than a brief explanation.
const maxReasonLen = 200

func truncateReason(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxReasonLen {
		return s
	}
	return s[:maxReasonLen] + "... (truncated)"
}

// balancedJSONObjects returns every top-level balanced {...} substring of
// s, in the order they appear. A reasoning model's reply can carry
// chain-of-thought prose before the verdict JSON, or even two JSON objects
// (e.g. one embedded in an explanation, one as the actual answer) — this
// lets parseVerdict try candidates from the end, where the actual verdict
// almost always lands.
func balancedJSONObjects(s string) []string {
	var objs []string
	depth := 0
	start := -1
	for i, r := range s {
		switch r {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					objs = append(objs, s[start:i+1])
					start = -1
				}
			}
		}
	}
	return objs
}

func parseVerdict(content string) (verdict, reason string) {
	var resp verdictResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &resp); err == nil && resp.Verdict != "" {
		return strings.ToUpper(resp.Verdict), resp.Reason
	}

	// Try the LAST balanced JSON object that actually parses as a verdict
	// first: reasoning text precedes the verdict far more often than it
	// follows it, and when two objects are present the later one is the
	// answer.
	objs := balancedJSONObjects(content)
	for i := len(objs) - 1; i >= 0; i-- {
		var r verdictResponse
		if err := json.Unmarshal([]byte(objs[i]), &r); err == nil && r.Verdict != "" {
			return strings.ToUpper(r.Verdict), r.Reason
		}
	}

	lines := strings.Split(content, "\n")
	lastVerdict := ""
	for _, line := range lines {
		cleaned := strings.ToUpper(strings.Trim(strings.TrimSpace(line), "*_ "))
		if cleaned == "ALLOW" || cleaned == "BLOCK" {
			lastVerdict = cleaned
		}
	}
	if lastVerdict != "" {
		return lastVerdict, ""
	}

	return "", content
}
