package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCpCopiesFile(t *testing.T) {
	ws := t.TempDir()
	tool := Cp(ws)

	os.WriteFile(filepath.Join(ws, "src.txt"), []byte("data"), 0644)

	args, err := Validate(tool.Parameters(), map[string]any{
		"source":      "src.txt",
		"destination": "dst.txt",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	// Source should still exist
	if _, err := os.Stat(filepath.Join(ws, "src.txt")); err != nil {
		t.Error("source should still exist after copy")
	}

	data, err := os.ReadFile(filepath.Join(ws, "dst.txt"))
	if err != nil {
		t.Fatalf("destination not found: %v", err)
	}
	if string(data) != "data" {
		t.Errorf("got %q, want %q", string(data), "data")
	}
}

func TestCpCopiesDirRecursively(t *testing.T) {
	ws := t.TempDir()
	tool := Cp(ws)

	// Create source dir with nested file
	os.MkdirAll(filepath.Join(ws, "srcdir", "sub"), 0755)
	os.WriteFile(filepath.Join(ws, "srcdir", "a.txt"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(ws, "srcdir", "sub", "b.txt"), []byte("bbb"), 0644)

	args, err := Validate(tool.Parameters(), map[string]any{
		"source":      "srcdir",
		"destination": "dstdir",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(ws, "dstdir", "a.txt"))
	if err != nil {
		t.Fatalf("copied file not found: %v", err)
	}
	if string(data) != "aaa" {
		t.Errorf("got %q, want %q", string(data), "aaa")
	}

	data, err = os.ReadFile(filepath.Join(ws, "dstdir", "sub", "b.txt"))
	if err != nil {
		t.Fatalf("nested copied file not found: %v", err)
	}
	if string(data) != "bbb" {
		t.Errorf("got %q, want %q", string(data), "bbb")
	}
}

func TestCp_SourceNotFound(t *testing.T) {
	ws := t.TempDir()
	tool := Cp(ws)

	args, _ := Validate(tool.Parameters(), map[string]any{
		"source":      "nonexistent.txt",
		"destination": "dst.txt",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for nonexistent source")
	}
}

func TestCp_SourceOutsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	tool := Cp(ws)

	args, _ := Validate(tool.Parameters(), map[string]any{
		"source":      "/etc/passwd",
		"destination": "dst.txt",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for source outside workspace")
	}
	if !strings.Contains(err.Error(), "outside workspace") {
		t.Errorf("expected 'outside workspace' in error, got %q", err.Error())
	}
}

func TestCp_DestOutsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	tool := Cp(ws)

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
