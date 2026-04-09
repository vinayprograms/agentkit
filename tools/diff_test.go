package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiff_DifferentFiles(t *testing.T) {
	dir := t.TempDir()

	file1 := filepath.Join(dir, "a.txt")
	file2 := filepath.Join(dir, "b.txt")

	os.WriteFile(file1, []byte("line1\nline2\nline3\n"), 0644)
	os.WriteFile(file2, []byte("line1\nchanged\nline3\n"), 0644)

	tool := Diff()
	args, err := Validate(tool.Parameters(), map[string]any{
		"file_a": file1,
		"file_b": file2,
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(result, "---") {
		t.Error("expected diff to contain --- marker")
	}
	if !strings.Contains(result, "+++") {
		t.Error("expected diff to contain +++ marker")
	}
	if !strings.Contains(result, "@@") {
		t.Error("expected diff to contain @@ hunk header")
	}
	if !strings.Contains(result, "-line2") && !strings.Contains(result, "-changed") {
		t.Error("expected diff to contain removed line marker")
	}
}

func TestDiff_IdenticalFiles(t *testing.T) {
	dir := t.TempDir()

	file1 := filepath.Join(dir, "a.txt")
	file2 := filepath.Join(dir, "b.txt")

	content := []byte("same\ncontent\nhere\n")
	os.WriteFile(file1, content, 0644)
	os.WriteFile(file2, content, 0644)

	tool := Diff()
	args, err := Validate(tool.Parameters(), map[string]any{
		"file_a": file1,
		"file_b": file2,
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	// The diff tool may return just headers with no hunks for identical files,
	// or "Files are identical". Either way, there should be no @@ hunk markers.
	if strings.Contains(result, "@@") {
		t.Errorf("expected no diff hunks for identical files, got %q", result)
	}
}

func TestDiff_MissingFile(t *testing.T) {
	dir := t.TempDir()

	file1 := filepath.Join(dir, "exists.txt")
	file2 := filepath.Join(dir, "missing.txt")

	os.WriteFile(file1, []byte("hello\n"), 0644)

	tool := Diff()
	args, err := Validate(tool.Parameters(), map[string]any{
		"file_a": file1,
		"file_b": file2,
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "missing.txt") {
		t.Errorf("error should mention missing file, got: %v", err)
	}
}

func TestDiff_MissingFirstFile(t *testing.T) {
	dir := t.TempDir()

	file1 := filepath.Join(dir, "missing.txt")
	file2 := filepath.Join(dir, "exists.txt")

	os.WriteFile(file2, []byte("hello\n"), 0644)

	tool := Diff()
	args, err := Validate(tool.Parameters(), map[string]any{
		"file_a": file1,
		"file_b": file2,
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "missing.txt") {
		t.Errorf("error should mention missing file, got: %v", err)
	}
}
