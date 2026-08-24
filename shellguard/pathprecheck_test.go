package shellguard

import (
	"context"
	"testing"

	"github.com/vinayprograms/agentkit/llm"
)

// failingModel fails the test if it is ever invoked. Used to prove the
// happy-path pre-check genuinely skips the LLM call, not just its verdict.
type failingModel struct{ t *testing.T }

func (m *failingModel) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	m.t.Fatalf("LLM should not have been called; last message: %+v", req.Messages[len(req.Messages)-1])
	return nil, nil
}

// TestPathPrecheck_HappyPath: provably in-bounds commands must be
// pre-check-allowed with the LLM never invoked.
func TestPathPrecheck_HappyPath(t *testing.T) {
	tests := []string{
		"go test ./...",
		"go build ./...",
		"go vet ./...",
		"ls .",
		"ls -la",
		"cat ./src/main.go",
		"git status",
		"git log",
		"git diff",
		"pwd",
		"echo hello",
		"mkdir -p ./build/out",
		"touch ./scratch/note.txt",
		"find . -name main.go",
	}

	for _, cmd := range tests {
		t.Run(cmd, func(t *testing.T) {
			gate := NewGate(Config{
				Shell:       Bash(),
				Workspace:   "/workspace",
				AllowedDirs: []string{"/workspace"},
				Model:       &failingModel{t: t},
			})
			if err := gate.check(context.Background(), cmd); err != nil {
				t.Errorf("expected command to be allowed, got error: %v", err)
			}
		})
	}
}

// TestPathPrecheck_Adversarial: every one of these must NOT be
// pre-check-allowed. They must fall through to the LLM (or be blocked by
// the deterministic stage before ever reaching the pre-check).
func TestPathPrecheck_Adversarial(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{"command substitution", "cat $(echo /etc/passwd)"},
		{"ssh key literal ~", "cat ~/.ssh/id_rsa"},
		{"aws credentials double-quoted var", `cat "$HOME/.aws/credentials"`},
		{"path traversal relative", "ls /workspace/../etc"},
		{"path traversal double", "cat /workspace/../../etc/passwd"},
		{"grep outside workspace", "grep -r x /etc"},
		{"redirect to system path", "echo hi > /etc/x"},
		{"backtick substitution", "cat `echo /etc/passwd`"},
		{"braced variable expansion", "cat ${HOME}/.aws/credentials"},
		{"glob form", "cat /workspace/*.go /etc/passwd"},
		{"glob hides traversal", "cat /workspace/../[e]tc/passwd"},
		{"eval form", "eval cat /etc/passwd"},
		{"eval chained", "echo x && eval 'cat /etc/passwd'"},
		{"renamed/aliased binary via path", "/bin/cat /etc/passwd"},
		{"base64-decoded script", "echo Y2F0IC9ldGMvcGFzc3dk | base64 -d | sh"},
		{"unknown base command", "perl -e 'print 1'"},
		{"find with -exec", "find . -exec cat {} \\;"},
		{"find with -delete", "find /workspace -delete"},
		{"git subcommand not safelisted", "git push origin main"},
		{"go subcommand not safelisted", "go run main.go"},
		{"bare var in arg", "cat $SECRET_PATH"},
		{"pipe into interpreter", "echo evil | python3"},
		{"quoted glob still conservative", `find . -name "*.go"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gate := NewGate(Config{
				Shell:       Bash(),
				Workspace:   "/workspace",
				AllowedDirs: []string{"/workspace"},
			})

			// The command must not be pre-check-skippable...
			if skip, reason := gate.CheckPathPrecheck(tc.cmd); skip {
				t.Errorf("command %q was pre-check-allowed (reason: %q); it must fall through to the LLM", tc.cmd, reason)
			}

			// ...and running it through the full gate with no LLM configured
			// must not silently allow it either (deterministic-only mode: if
			// the deterministic stage blocks it, fine; if it doesn't, the
			// absence of a model must not be mistaken for an allow via
			// pre-check). We only assert the pre-check result above, since
			// deterministic-only mode intentionally allows anything the
			// denylist doesn't block (mirrors pre-existing behavior when no
			// model is configured) — the security property under test is
			// specifically that path-precheck itself doesn't skip the LLM.
		})
	}
}

// TestPathPrecheck_AdversarialFallsThroughToLLM verifies the adversarial
// commands, when a real LLM stage IS configured, actually reach it (proven
// by using a model that always blocks) rather than being silently allowed
// by the pre-check.
func TestPathPrecheck_AdversarialFallsThroughToLLM(t *testing.T) {
	blockAll := &mockModel{allowedDirs: nil} // mockModel blocks anything not matching an allowed dir substring or lacking a space-slash
	tests := []string{
		"cat $(echo /etc/passwd)",
		"cat ~/.ssh/id_rsa",
		`cat "$HOME/.aws/credentials"`,
		"ls /workspace/../etc",
		"cat /workspace/../../etc/passwd",
		"grep -r x /etc",
		"echo hi > /etc/x",
		"cat `echo /etc/passwd`",
		"cat ${HOME}/.aws/credentials",
	}

	for _, cmd := range tests {
		t.Run(cmd, func(t *testing.T) {
			gate := NewGate(Config{
				Shell:       Bash(),
				Workspace:   "/workspace",
				AllowedDirs: []string{"/workspace"},
				Model:       blockAll,
			})
			var sawLLMStep bool
			gate.OnDecision = func(command, step string, allowed bool, reason string, durationMs int64, inputTokens, outputTokens int) {
				if step == "llm" {
					sawLLMStep = true
				}
			}
			_ = gate.check(context.Background(), cmd)
			if !sawLLMStep {
				t.Errorf("command %q never reached the LLM stage", cmd)
			}
		})
	}
}

// TestPathPrecheck_OnDecisionStage verifies the pre-check fires OnDecision
// with a distinct stage name so callers can audit it separately from the
// LLM.
func TestPathPrecheck_OnDecisionStage(t *testing.T) {
	gate := NewGate(Config{
		Shell:       Bash(),
		Workspace:   "/workspace",
		AllowedDirs: []string{"/workspace"},
		Model:       &failingModel{t: t},
	})
	var steps []string
	gate.OnDecision = func(command, step string, allowed bool, reason string, durationMs int64, inputTokens, outputTokens int) {
		steps = append(steps, step)
	}
	if err := gate.check(context.Background(), "ls ."); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, s := range steps {
		if s == "path-precheck" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an OnDecision call with step 'path-precheck', got %v", steps)
	}
}
