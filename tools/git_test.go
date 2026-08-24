package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initGitRepo creates a temp dir with an initialized git repo containing one commit.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, c := range cmds {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", c, err, out)
		}
	}

	// Create and commit a file
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world\n"), 0644)
	for _, c := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial commit"},
	} {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", c, err, out)
		}
	}

	return dir
}

func TestGit_Status(t *testing.T) {
	dir := initGitRepo(t)

	tool := Git(dir)
	args, err := Validate(tool.Parameters(), map[string]any{
		"args": "status",
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(result, "nothing to commit") && !strings.Contains(result, "clean") {
		t.Errorf("expected clean status, got %q", result)
	}
}

func TestGit_Log(t *testing.T) {
	dir := initGitRepo(t)

	tool := Git(dir)
	args, err := Validate(tool.Parameters(), map[string]any{
		"args": "log --oneline",
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(result, "initial commit") {
		t.Errorf("expected log to contain 'initial commit', got %q", result)
	}
}

func TestGit_BlocksDangerousForce(t *testing.T) {
	dir := initGitRepo(t)

	tool := Git(dir)
	args, err := Validate(tool.Parameters(), map[string]any{
		"args": "push --force",
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for --force flag")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("error should mention blocked, got: %v", err)
	}
}

func TestGit_BlocksDangerousReset(t *testing.T) {
	dir := initGitRepo(t)

	tool := Git(dir)
	args, err := Validate(tool.Parameters(), map[string]any{
		"args": "reset --hard HEAD~1",
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for reset command")
	}
	// "reset" is in the dangerous flags list, so it's blocked either as
	// a subcommand (not allowed) or as a flag (blocked for safety).
	if !strings.Contains(err.Error(), "blocked") && !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("error should mention blocked or not allowed, got: %v", err)
	}
}

func TestGit_BlocksUnsafeSubcommand(t *testing.T) {
	dir := initGitRepo(t)

	tool := Git(dir)
	args, err := Validate(tool.Parameters(), map[string]any{
		"args": "filter-branch",
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for filter-branch")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("error should mention not allowed, got: %v", err)
	}
}

func TestGit_BlocksNoVerify(t *testing.T) {
	dir := initGitRepo(t)

	tool := Git(dir)
	args, err := Validate(tool.Parameters(), map[string]any{
		"args": "commit --no-verify -m 'skip hooks'",
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for --no-verify flag")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("error should mention blocked, got: %v", err)
	}
}

func TestGit_DiffStat(t *testing.T) {
	dir := initGitRepo(t)

	// Modify a file to produce a diff
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello changed\n"), 0644)

	tool := Git(dir)
	args, err := Validate(tool.Parameters(), map[string]any{
		"args": "diff --stat",
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(result, "hello.txt") {
		t.Errorf("expected diff stat to mention hello.txt, got %q", result)
	}
}

func TestGit_CustomCwd(t *testing.T) {
	dir := initGitRepo(t)

	// Use empty workspace, pass cwd explicitly
	tool := Git("")
	args, err := Validate(tool.Parameters(), map[string]any{
		"args": "status",
		"cwd":  dir,
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(result, "nothing to commit") && !strings.Contains(result, "clean") {
		t.Errorf("expected clean status via cwd, got %q", result)
	}
}

func TestGit_ShortlogDefaultsToHEAD(t *testing.T) {
	dir := initGitRepo(t)

	tool := Git(dir)
	args, err := Validate(tool.Parameters(), map[string]any{
		"args": "shortlog -sn",
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(result, "Test") {
		t.Errorf("expected shortlog to return author data on the first call without an explicit revision, got %q", result)
	}
}

func TestGit_ShortlogExplicitRevisionUnaffected(t *testing.T) {
	dir := initGitRepo(t)

	tool := Git(dir)
	args, err := Validate(tool.Parameters(), map[string]any{
		"args": "shortlog -sn HEAD",
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(result, "Test") {
		t.Errorf("expected shortlog with explicit HEAD to still work, got %q", result)
	}
}

func TestGit_AllowsReadOnlyPlumbing(t *testing.T) {
	dir := initGitRepo(t)
	tool := Git(dir)

	for _, cmd := range []string{"rev-list HEAD", "rev-parse HEAD", "describe --always", "for-each-ref"} {
		args, err := Validate(tool.Parameters(), map[string]any{"args": cmd})
		if err != nil {
			t.Fatalf("validate %q: %v", cmd, err)
		}
		if _, err := tool.Execute(context.Background(), args); err != nil {
			t.Errorf("expected %q to be allowed and succeed, got error: %v", cmd, err)
		}
	}
}

func TestGit_TruncatesLongErrorOutput(t *testing.T) {
	dir := initGitRepo(t)
	tool := Git(dir)

	// `git diff --no-index` on two missing paths prints a full usage page
	// to stderr and exits 129 — this should come back truncated, not as a
	// multi-KB dump.
	args, err := Validate(tool.Parameters(), map[string]any{
		"args": "diff --no-index --bogus-flag-xyz",
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected an error from an invalid diff invocation")
	}
	if len(err.Error()) > gitErrorMaxBytes+200 {
		t.Errorf("expected error output to be truncated to ~%d bytes, got %d bytes: %q", gitErrorMaxBytes, len(err.Error()), err.Error())
	}
}

func TestTruncateGitError_ShortOutputUnchanged(t *testing.T) {
	short := "fatal: not a git repository"
	if got := truncateGitError(short); got != short {
		t.Errorf("expected short output unchanged, got %q", got)
	}
}

func TestTruncateGitError_LongOutputTruncated(t *testing.T) {
	long := "fatal: usage error\n" + strings.Repeat("x", 5000)
	got := truncateGitError(long)
	if len(got) > gitErrorMaxBytes+50 {
		t.Errorf("expected truncated output near %d bytes, got %d", gitErrorMaxBytes, len(got))
	}
	if !strings.HasPrefix(got, "fatal: usage error") {
		t.Errorf("expected first line preserved, got %q", got[:min(len(got), 50)])
	}
	if !strings.Contains(got, "...[truncated]") {
		t.Errorf("expected truncation marker, got %q", got)
	}
}
