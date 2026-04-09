package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrep_BasicMatch(t *testing.T) {
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "hello.txt"), []byte("hello world\ngoodbye world\nhello again"), 0644)

	tool := Grep(ws)
	args, err := Validate(tool.Parameters(), map[string]any{"pattern": "hello", "path": ws})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 matches, got %d: %s", len(lines), result)
	}
	// Verify format: file:line: content
	if !strings.Contains(lines[0], "hello.txt:1: hello world") {
		t.Errorf("unexpected first match: %s", lines[0])
	}
	if !strings.Contains(lines[1], "hello.txt:3: hello again") {
		t.Errorf("unexpected second match: %s", lines[1])
	}
}

func TestGrep_IgnoreCase(t *testing.T) {
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "mixed.txt"), []byte("Hello World\nhello world\nHELLO WORLD"), 0644)

	tool := Grep(ws)
	// Use regex (?i) for case-insensitive matching
	args, err := Validate(tool.Parameters(), map[string]any{"pattern": "(?i)hello", "path": ws})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(result, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 matches with case-insensitive regex, got %d: %s", len(lines), result)
	}
}

func TestGrep_PathScoping(t *testing.T) {
	ws := t.TempDir()
	sub := filepath.Join(ws, "subdir")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(ws, "root.txt"), []byte("match here"), 0644)
	os.WriteFile(filepath.Join(sub, "child.txt"), []byte("match here too"), 0644)

	tool := Grep(ws)

	// Search only in subdir
	args, err := Validate(tool.Parameters(), map[string]any{"pattern": "match", "path": sub})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(result, "root.txt") {
		t.Error("result should not include root.txt when scoped to subdir")
	}
	if !strings.Contains(result, "child.txt") {
		t.Error("result should include child.txt from subdir")
	}
}

func TestGrep_NoMatches(t *testing.T) {
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "data.txt"), []byte("some content"), 0644)

	tool := Grep(ws)
	args, err := Validate(tool.Parameters(), map[string]any{"pattern": "nonexistent", "path": ws})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result != "No matches found." {
		t.Errorf("expected 'No matches found.', got %q", result)
	}
}

func TestGrep_RelativePath(t *testing.T) {
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "rel.txt"), []byte("find me"), 0644)

	tool := Grep(ws)
	args, err := Validate(tool.Parameters(), map[string]any{"pattern": "find", "path": "rel.txt"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "find me") {
		t.Errorf("expected match for relative path, got %q", result)
	}
}

func TestGrep_InvalidRegex(t *testing.T) {
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "f.txt"), []byte("data"), 0644)

	tool := Grep(ws)
	args, err := Validate(tool.Parameters(), map[string]any{"pattern": "[invalid", "path": ws})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestGrep_OutsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	tool := Grep(ws)

	args, err := Validate(tool.Parameters(), map[string]any{"pattern": "x", "path": "/etc/passwd"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for path outside workspace")
	}
}
