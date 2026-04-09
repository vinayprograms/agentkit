package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMkdirCreatesDir(t *testing.T) {
	ws := t.TempDir()
	tool := Mkdir(ws)

	args, err := Validate(tool.Parameters(), map[string]any{"path": "testdir"})
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

	info, err := os.Stat(filepath.Join(ws, "testdir"))
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected a directory")
	}
}

func TestMkdirNestedDirs(t *testing.T) {
	ws := t.TempDir()
	tool := Mkdir(ws)

	args, err := Validate(tool.Parameters(), map[string]any{"path": "a/b/c"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(ws, "a", "b", "c")); err != nil {
		t.Fatalf("nested directories not created: %v", err)
	}
}

func TestMkdirOutsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	tool := Mkdir(ws)

	args, err := Validate(tool.Parameters(), map[string]any{"path": "/tmp/outsideworkspace"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for path outside workspace")
	}
}
