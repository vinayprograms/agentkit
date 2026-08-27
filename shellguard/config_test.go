package shellguard

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vinayprograms/agentkit/llm"
)

// promptCapturingModel records the prompt (first message content) it was asked
// and always answers ALLOW, so tests can inspect what llmCheck sent without
// caring about the verdict.
type promptCapturingModel struct {
	lastPrompt string
}

func (m *promptCapturingModel) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if len(req.Messages) > 0 {
		m.lastPrompt = req.Messages[0].Content
	}
	return &llm.ChatResponse{Content: `{"verdict":"ALLOW"}`}, nil
}

// TestLLMPrompt_ContainsCoreRules asserts the rewritten prompt still covers
// every rule the old one did (write confinement, subdirectories,
// path-traversal, /tmp, /dev, and the security-research escape hatch) plus
// the new data-read vs toolchain-read distinction with worked examples.
func TestLLMPrompt_ContainsCoreRules(t *testing.T) {
	m := &promptCapturingModel{}
	_, err := llmCheck(context.Background(), m, "cat /etc/passwd", []string{"/workspace"}, nil, nil, "/workspace", "", llm.ThinkingOff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := m.lastPrompt

	mustContain := []string{
		// data-read vs toolchain-read distinction, the whole point of the change
		"DATA READ",
		"TOOLCHAIN ACCESS",
		"GOCACHE",
		"GOROOT",
		"go build ./...", // worked example
		"site-packages",
		"node_modules",
		// credential paths always blocked
		"~/.ssh",
		"~/.aws",
		".pem",
		".key",
		"id_rsa",
		// preserved from the old prompt
		"WRITE operations",
		"SUBDIRECTORIES",
		"/tmp",
		"/dev/null",
		"path traversal",
		"/../",
	}
	for _, s := range mustContain {
		if !strings.Contains(p, s) {
			t.Errorf("prompt missing expected content %q\n---\n%s", s, p)
		}
	}
}

func TestLLMPrompt_SecurityScopeEscapeHatchPreserved(t *testing.T) {
	m := &promptCapturingModel{}
	_, err := llmCheck(context.Background(), m, "nmap localhost", []string{"/workspace"}, nil, nil, "/workspace", "penetration testing of internal network", llm.ThinkingOff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(m.lastPrompt, "SECURITY RESEARCH CONTEXT") {
		t.Error("security research context escape hatch missing from prompt")
	}
	if !strings.Contains(m.lastPrompt, "penetration testing of internal network") {
		t.Error("security scope text not interpolated into prompt")
	}
}

// TestLLMPrompt_DeniedCommandsInterpolated covers task 2: policy-denied
// commands must be listed in the LLM prompt with "same effect, another
// route" guidance, not just enforced by the deterministic exact-match stage.
func TestLLMPrompt_DeniedCommandsInterpolated(t *testing.T) {
	m := &promptCapturingModel{}
	_, err := llmCheck(context.Background(), m, "ls", []string{"/workspace"}, []string{"curl", "wget"}, nil, "/workspace", "", llm.ThinkingOff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := m.lastPrompt
	if !strings.Contains(p, "curl") || !strings.Contains(p, "wget") {
		t.Errorf("denied commands not interpolated into prompt:\n%s", p)
	}
	if !strings.Contains(p, "SAME EFFECT") {
		t.Errorf("prompt missing same-effect-another-route guidance:\n%s", p)
	}
}

// TestLLMPrompt_NoDeniedCommands_NoDeadSection: when there are no
// policy-denied commands, the prompt should not carry an empty/dangling
// "COMMANDS DENIED BY POLICY" section (keeps prompts clean and cheap).
func TestLLMPrompt_NoDeniedCommands_NoDeadSection(t *testing.T) {
	m := &promptCapturingModel{}
	_, err := llmCheck(context.Background(), m, "ls", []string{"/workspace"}, nil, nil, "/workspace", "", llm.ThinkingOff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(m.lastPrompt, "these base commands are blocked outright") {
		t.Errorf("expected no denied-commands section when list is empty:\n%s", m.lastPrompt)
	}
}

// TestLLMPrompt_DisabledToolsInterpolated covers task 3's prompt half: the
// disabled tool names must appear along with the "no side door" guidance
// for write and read.
func TestLLMPrompt_DisabledToolsInterpolated(t *testing.T) {
	m := &promptCapturingModel{}
	_, err := llmCheck(context.Background(), m, "ls", []string{"/workspace"}, nil, []string{"write", "read"}, "/workspace", "", llm.ThinkingOff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := m.lastPrompt
	if !strings.Contains(p, "AGENT TOOLS DISABLED BY POLICY") {
		t.Errorf("prompt missing disabled-tools section:\n%s", p)
	}
	if !strings.Contains(p, "write") || !strings.Contains(p, "read") {
		t.Errorf("disabled tool names not interpolated:\n%s", p)
	}
	if !strings.Contains(p, "side door") {
		t.Errorf("prompt missing side-door guidance:\n%s", p)
	}
}

func TestLLMPrompt_NoDisabledTools_NoDeadSection(t *testing.T) {
	m := &promptCapturingModel{}
	_, err := llmCheck(context.Background(), m, "ls", []string{"/workspace"}, nil, nil, "/workspace", "", llm.ThinkingOff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(m.lastPrompt, "AGENT TOOLS DISABLED BY POLICY") {
		t.Errorf("expected no disabled-tools section when list is empty:\n%s", m.lastPrompt)
	}
}

// TestNew_DelegatesToNewGate: New must behave identically to before —
// same fields set, same deterministic + LLM behavior — now that it
// delegates to NewGate. DisabledTools should be left unset (nil) since New
// has no parameter for it.
func TestNew_DelegatesToNewGate(t *testing.T) {
	m := &promptCapturingModel{}
	g := New(Bash(), "/workspace", []string{"/workspace"}, []string{"curl"}, m, "research scope")

	if g.workspace != "/workspace" {
		t.Errorf("workspace = %q, want /workspace", g.workspace)
	}
	if len(g.allowedDirs) != 1 || g.allowedDirs[0] != "/workspace" {
		t.Errorf("allowedDirs = %v", g.allowedDirs)
	}
	if len(g.userDeniedCommands) != 1 || g.userDeniedCommands[0] != "curl" {
		t.Errorf("userDeniedCommands = %v", g.userDeniedCommands)
	}
	if g.securityScope != "research scope" {
		t.Errorf("securityScope = %q", g.securityScope)
	}
	if g.disabledTools != nil {
		t.Errorf("disabledTools = %v, want nil (New has no way to set it)", g.disabledTools)
	}
	if g.model != m {
		t.Error("model not wired through")
	}

	// Deterministic behavior unchanged: curl is both a hardcoded banned
	// command and now a user-denied one — either way it's blocked before
	// the LLM ever runs.
	allowed, reason := g.CheckDeterministic("curl http://example.com")
	if allowed {
		t.Error("expected curl to be blocked deterministically")
	}
	if reason == "" {
		t.Error("expected a reason for the block")
	}
}

// TestNewGate_SetsAllFields covers task 4's Config-constructor half: every
// Config field, including the new DisabledTools, must land on the Gate.
func TestNewGate_SetsAllFields(t *testing.T) {
	m := &promptCapturingModel{}
	g := NewGate(Config{
		Shell:              Bash(),
		Workspace:          "/ws",
		AllowedDirs:        []string{"/ws", "/tmp/x"},
		UserDeniedCommands: []string{"nc"},
		DisabledTools:      []string{"write"},
		Model:              m,
		SecurityScope:      "scope",
	})

	if g.workspace != "/ws" {
		t.Errorf("workspace = %q", g.workspace)
	}
	if len(g.allowedDirs) != 2 {
		t.Errorf("allowedDirs = %v", g.allowedDirs)
	}
	if len(g.userDeniedCommands) != 1 || g.userDeniedCommands[0] != "nc" {
		t.Errorf("userDeniedCommands = %v", g.userDeniedCommands)
	}
	if len(g.disabledTools) != 1 || g.disabledTools[0] != "write" {
		t.Errorf("disabledTools = %v", g.disabledTools)
	}
	if g.securityScope != "scope" {
		t.Errorf("securityScope = %q", g.securityScope)
	}
	if g.model != m {
		t.Error("model not wired through")
	}

	// End-to-end: disabledTools reaches the LLM prompt through Check.
	_, err := llmCheck(context.Background(), m, "ls", g.allowedDirs, g.userDeniedCommands, g.disabledTools, g.workspace, g.securityScope, llm.ThinkingOff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(m.lastPrompt, "write") {
		t.Error("DisabledTools from Config did not reach the LLM prompt")
	}
}

// thinkingCapturingModel records the thinking level it was asked with and
// always answers ALLOW.
type thinkingCapturingModel struct {
	levels []llm.ThinkingLevel
}

func (m *thinkingCapturingModel) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	m.levels = append(m.levels, req.Thinking)
	return &llm.ChatResponse{Content: `{"verdict":"ALLOW"}`}, nil
}

// TestGate_Thinking_DefaultsOff pins the default: a Config that says nothing
// about thinking gets none. Most callers send short, shaped commands, and a
// snap verdict on those is the right trade.
func TestGate_Thinking_DefaultsOff(t *testing.T) {
	m := &thinkingCapturingModel{}
	gate := NewGate(Config{Shell: Bash(), Workspace: "/workspace", AllowedDirs: []string{"/workspace"}, Model: m})

	// A command the path pre-check cannot prove safe, so the LLM stage runs.
	if err := gate.check(context.Background(), "cat ../outside.txt"); err != nil {
		t.Fatalf("unexpected block: %v", err)
	}
	if len(m.levels) == 0 {
		t.Fatal("LLM stage never ran")
	}
	if m.levels[0] != llm.ThinkingOff {
		t.Errorf("default thinking = %q, want %q", m.levels[0], llm.ThinkingOff)
	}
}

// TestGate_Thinking_Enabled asserts the Config bool actually reaches the
// model. Judging a long compound command is a reasoning question, and a
// policy that asks for reasoning must get it.
func TestGate_Thinking_Enabled(t *testing.T) {
	m := &thinkingCapturingModel{}
	gate := NewGate(Config{Shell: Bash(), Workspace: "/workspace", AllowedDirs: []string{"/workspace"}, Model: m, Thinking: true})

	if err := gate.check(context.Background(), "cat ../outside.txt"); err != nil {
		t.Fatalf("unexpected block: %v", err)
	}
	if len(m.levels) == 0 {
		t.Fatal("LLM stage never ran")
	}
	if m.levels[0] == llm.ThinkingOff {
		t.Error("Thinking:true must not send ThinkingOff")
	}
}

// slowModel blocks until ctx is done, standing in for a reviewer too slow to
// answer within the gate's deadline.
type slowModel struct{}

func (m *slowModel) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestGate_LLMTimeout_FallsBackToDeterministic is the availability contract:
// a reviewer that cannot answer in time must not be able to stall or fail
// the caller. The LLM stage runs only on commands the deterministic stage
// already allowed, so the gate falls back to that verdict and records the
// decision as degraded under a step name that says the check could not run.
func TestGate_LLMTimeout_FallsBackToDeterministic(t *testing.T) {
	gate := NewGate(Config{
		Shell:       Bash(),
		Workspace:   "/workspace",
		AllowedDirs: []string{"/workspace"},
		Model:       &slowModel{},
		Timeout:     20 * time.Millisecond,
	})

	var lastStep, lastReason string
	var lastAllowed bool
	gate.OnDecision = func(command, step string, allowed bool, reason string, durationMs int64, in, out int) {
		lastStep, lastAllowed, lastReason = step, allowed, reason
	}

	start := time.Now()
	err := gate.check(context.Background(), "cat ../outside.txt")
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("timeout should fall back to deterministic ALLOW, got: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("gate did not honor its deadline: took %v", elapsed)
	}
	if lastStep != "llm-timeout" {
		t.Errorf("step = %q, want %q — a timeout must be distinguishable from a denial", lastStep, "llm-timeout")
	}
	if !lastAllowed {
		t.Error("degraded decision should be recorded as allowed")
	}
	if !strings.Contains(lastReason, "falling back to deterministic ALLOW") {
		t.Errorf("reason should record the degradation, got: %q", lastReason)
	}
}

// TestGate_NoTimeout_NoDeadline asserts Timeout:0 leaves the caller's context
// untouched rather than inventing a deadline of its own.
func TestGate_NoTimeout_NoDeadline(t *testing.T) {
	m := &deadlineCapturingModel{}
	gate := NewGate(Config{Shell: Bash(), Workspace: "/workspace", AllowedDirs: []string{"/workspace"}, Model: m})

	if err := gate.check(context.Background(), "cat ../outside.txt"); err != nil {
		t.Fatalf("unexpected block: %v", err)
	}
	if m.hadDeadline {
		t.Error("Timeout:0 must not impose a deadline")
	}
}

type deadlineCapturingModel struct{ hadDeadline bool }

func (m *deadlineCapturingModel) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	_, m.hadDeadline = ctx.Deadline()
	return &llm.ChatResponse{Content: `{"verdict":"ALLOW"}`}, nil
}
