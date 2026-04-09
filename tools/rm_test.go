package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRmRemovesFile(t *testing.T) {
	ws := t.TempDir()
	tool := Rm(ws)

	f := filepath.Join(ws, "deleteme.txt")
	os.WriteFile(f, []byte("bye"), 0644)

	args, err := Validate(tool.Parameters(), map[string]any{"path": "deleteme.txt"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Error("file should have been deleted")
	}
}

func TestRmRemovesDirRecursive(t *testing.T) {
	ws := t.TempDir()
	tool := Rm(ws)

	dir := filepath.Join(ws, "mydir")
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "file.txt"), []byte("x"), 0644)

	args, err := Validate(tool.Parameters(), map[string]any{
		"path":      "mydir",
		"recursive": true,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("directory should have been deleted")
	}
}

func TestRmNonEmptyDirWithoutRecursiveFails(t *testing.T) {
	ws := t.TempDir()
	tool := Rm(ws)

	dir := filepath.Join(ws, "nonempty")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0644)

	args, err := Validate(tool.Parameters(), map[string]any{"path": "nonempty"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error when removing non-empty dir without recursive")
	}
}
