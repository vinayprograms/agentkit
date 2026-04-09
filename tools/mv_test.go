package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMvMovesFile(t *testing.T) {
	ws := t.TempDir()
	tool := Mv(ws)

	src := filepath.Join(ws, "original.txt")
	os.WriteFile(src, []byte("content"), 0644)

	args, err := Validate(tool.Parameters(), map[string]any{
		"source":      "original.txt",
		"destination": "moved.txt",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source file should no longer exist")
	}

	data, err := os.ReadFile(filepath.Join(ws, "moved.txt"))
	if err != nil {
		t.Fatalf("destination file not found: %v", err)
	}
	if string(data) != "content" {
		t.Errorf("got %q, want %q", string(data), "content")
	}
}

func TestMvRenamesFile(t *testing.T) {
	ws := t.TempDir()
	tool := Mv(ws)

	os.WriteFile(filepath.Join(ws, "old.txt"), []byte("hello"), 0644)

	args, err := Validate(tool.Parameters(), map[string]any{
		"source":      "old.txt",
		"destination": "new.txt",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result == "" {
		t.Error("expected non-empty result")
	}

	if _, err := os.Stat(filepath.Join(ws, "new.txt")); err != nil {
		t.Error("renamed file should exist")
	}
}

func TestMv_SourceOutsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	tool := Mv(ws)

	args, _ := Validate(tool.Parameters(), map[string]any{
		"source":      "/etc/passwd",
		"destination": "dst.txt",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for source outside workspace")
	}
}

func TestMv_DestOutsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	tool := Mv(ws)

	os.WriteFile(filepath.Join(ws, "src.txt"), []byte("data"), 0644)

	args, _ := Validate(tool.Parameters(), map[string]any{
		"source":      "src.txt",
		"destination": "/tmp/evil.txt",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for destination outside workspace")
	}
}

func TestMv_SourceNotFound(t *testing.T) {
	ws := t.TempDir()
	tool := Mv(ws)

	args, _ := Validate(tool.Parameters(), map[string]any{
		"source":      "nonexistent.txt",
		"destination": "dst.txt",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for nonexistent source")
	}
}
