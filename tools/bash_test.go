package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBash_NonZeroExitCode(t *testing.T) {
	tool := Bash(t.TempDir())
	args, err := Validate(tool.Parameters(), map[string]any{"command": "exit 1"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("non-zero exit should not return error, got: %v", err)
	}
	if !strings.Contains(result, "Exit code: 1") {
		t.Errorf("expected 'Exit code: 1' in result, got %q", result)
	}
}

func TestBash_StderrOutput(t *testing.T) {
	tool := Bash(t.TempDir())
	args, err := Validate(tool.Parameters(), map[string]any{"command": "echo err >&2"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "STDERR:") {
		t.Errorf("expected STDERR in result, got %q", result)
	}
	if !strings.Contains(result, "err") {
		t.Errorf("expected stderr content 'err' in result, got %q", result)
	}
}

func TestBash_Timeout(t *testing.T) {
	tool := Bash(t.TempDir())
	args, err := Validate(tool.Parameters(), map[string]any{"command": "sleep 10"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err = tool.Execute(ctx, args)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error, got %q", err.Error())
	}
}

func TestBash_CommandNotFound(t *testing.T) {
	tool := Bash(t.TempDir())
	args, err := Validate(tool.Parameters(), map[string]any{"command": "nonexistent_command_xyz_123"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	// bash -c with a nonexistent command returns exit code 127 with stderr
	// It's not an exec error since bash itself runs fine.
	if err != nil {
		// If it's an actual exec error, that's also acceptable
		return
	}
	// Otherwise it should show exit code and stderr about command not found
	if !strings.Contains(result, "Exit code:") && !strings.Contains(result, "not found") {
		t.Errorf("expected exit code or 'not found' in result, got %q", result)
	}
}

func TestBash_StdoutAndStderr(t *testing.T) {
	tool := Bash(t.TempDir())
	args, err := Validate(tool.Parameters(), map[string]any{"command": "echo out; echo err >&2"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "out") {
		t.Errorf("expected stdout 'out' in result, got %q", result)
	}
	if !strings.Contains(result, "STDERR:") {
		t.Errorf("expected STDERR in result, got %q", result)
	}
}

func TestBash_NameAndDescription(t *testing.T) {
	tool := Bash(t.TempDir())
	if tool.Name() != "bash" {
		t.Errorf("expected name 'bash', got %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
	if !strings.Contains(tool.Description(), "120") {
		t.Error("expected description to mention default timeout")
	}
}
