package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatch_ApplyValidDiff(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "target.txt")

	original := "line1\nline2\nline3\n"
	os.WriteFile(file, []byte(original), 0644)

	patch := `--- a/target.txt
+++ b/target.txt
@@ -1,3 +1,3 @@
 line1
-line2
+modified
 line3
`

	tool := Patch()
	args, err := Validate(tool.Parameters(), map[string]any{
		"path":  file,
		"patch": patch,
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(result, "successfully") {
		t.Errorf("expected success message, got %q", result)
	}

	// Verify the file was modified
	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}

	if !strings.Contains(string(content), "modified") {
		t.Errorf("patched file should contain 'modified', got %q", string(content))
	}
	if strings.Contains(string(content), "line2") {
		t.Errorf("patched file should not contain 'line2', got %q", string(content))
	}
}

func TestPatch_MissingFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "nonexistent.txt")

	patch := `--- a/nonexistent.txt
+++ b/nonexistent.txt
@@ -1,1 +1,1 @@
-old
+new
`

	tool := Patch()
	args, err := Validate(tool.Parameters(), map[string]any{
		"path":  file,
		"patch": patch,
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestPatch_AddLines(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "target.txt")

	original := "line1\nline2\nline3\n"
	os.WriteFile(file, []byte(original), 0644)

	patch := `--- a/target.txt
+++ b/target.txt
@@ -1,3 +1,4 @@
 line1
+inserted
 line2
 line3
`

	tool := Patch()
	args, err := Validate(tool.Parameters(), map[string]any{
		"path":  file,
		"patch": patch,
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}

	if !strings.Contains(string(content), "inserted") {
		t.Errorf("patched file should contain 'inserted', got %q", string(content))
	}
}
