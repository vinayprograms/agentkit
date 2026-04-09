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
