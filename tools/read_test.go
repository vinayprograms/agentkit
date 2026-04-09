package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRead_NonExistentFile(t *testing.T) {
	dir := t.TempDir()
	tool := Read(dir)

	args, err := Validate(tool.Parameters(), map[string]any{"path": filepath.Join(dir, "missing.txt")})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestRead_RelativePath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0644)

	tool := Read(dir)
	args, err := Validate(tool.Parameters(), map[string]any{"path": "hello.txt"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestRead_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "abs.txt")
	os.WriteFile(path, []byte("absolute"), 0644)

	tool := Read(dir)
	args, err := Validate(tool.Parameters(), map[string]any{"path": path})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "absolute" {
		t.Errorf("expected 'absolute', got %q", result)
	}
}

func TestRead_OutsideWorkspace(t *testing.T) {
	dir := t.TempDir()
	tool := Read(dir)

	args, err := Validate(tool.Parameters(), map[string]any{"path": "/etc/passwd"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for path outside workspace")
	}
}

func TestRead_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	tool := Read(dir)

	args, err := Validate(tool.Parameters(), map[string]any{"path": "../../../etc/passwd"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestRead_NameAndDescription(t *testing.T) {
	tool := Read(t.TempDir())
	if tool.Name() != "read" {
		t.Errorf("expected name 'read', got %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}
