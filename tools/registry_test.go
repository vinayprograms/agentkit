package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistry_Register(t *testing.T) {
	reg := NewRegistry()
	err := reg.Register(New(Pwd()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reg.Has("pwd") {
		t.Error("expected pwd to be registered")
	}
}

func TestRegistry_DuplicateRegister(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Pwd()))
	err := reg.Register(New(Pwd()))
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestRegistry_Get(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Pwd()))

	if reg.Get("pwd") == nil {
		t.Error("expected to find pwd")
	}
	if reg.Get("nonexistent") != nil {
		t.Error("expected nil for nonexistent")
	}
}

func TestRegistry_Definitions(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Pwd()))
	reg.Register(New(Hostname()))

	defs := reg.Definitions()
	if len(defs) != 2 {
		t.Errorf("expected 2 definitions, got %d", len(defs))
	}

	found := false
	for _, d := range defs {
		if d.Name == "pwd" {
			found = true
			if d.Description == "" {
				t.Error("expected non-empty description")
			}
		}
	}
	if !found {
		t.Error("pwd not in definitions")
	}
}

func TestRegistry_Subset(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Pwd()))
	reg.Register(New(Hostname()))

	sub, err := reg.Subset([]string{"pwd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sub.Has("pwd") {
		t.Error("expected pwd in subset")
	}
	if sub.Has("hostname") {
		t.Error("hostname must not be in subset")
	}
	if len(sub.Definitions()) != 1 {
		t.Errorf("expected 1 definition, got %d", len(sub.Definitions()))
	}
}

func TestRegistry_SubsetEmpty(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Pwd()))

	sub, err := reg.Subset(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sub.Definitions()) != 0 {
		t.Errorf("expected empty subset, got %d entries", len(sub.Definitions()))
	}
}

func TestRegistry_SubsetAll(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Pwd()))
	reg.Register(New(Hostname()))

	sub, err := reg.Subset([]string{"pwd", "hostname"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sub.Definitions()) != 2 {
		t.Errorf("expected 2 entries, got %d", len(sub.Definitions()))
	}
}

func TestRegistry_SubsetUnknown(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Pwd()))

	sub, err := reg.Subset([]string{"pwd", "nonexistent"})
	if err == nil {
		t.Error("expected error for unknown tool")
	}
	if sub != nil {
		t.Error("expected nil registry on error")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("expected error to name missing tool, got: %v", err)
	}
}

func TestRegistry_SubsetSharesEntries(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Pwd()))

	sub, err := reg.Subset([]string{"pwd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result, err := sub.Execute(context.Background(), "pwd", nil)
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result from subset registry")
	}
}

func TestRegistry_Execute(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Pwd()))

	result, err := reg.Execute(context.Background(), "pwd", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestRegistry_ExecuteUnknown(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Execute(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestRegistry_ExecuteValidationError(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Read(t.TempDir())))

	// read requires "path" parameter
	_, err := reg.Execute(context.Background(), "read", map[string]any{})
	if err == nil {
		t.Error("expected validation error for missing required param")
	}
}

func TestRegistry_Guard(t *testing.T) {
	reg := NewRegistry()

	guard := &blockingGuard{}
	reg.Register(New(Pwd()).With(guard))

	_, err := reg.Execute(context.Background(), "pwd", nil)
	if err == nil {
		t.Error("expected guard to block execution")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("expected blocked error, got: %v", err)
	}
}

func TestRegistry_MultipleGuards(t *testing.T) {
	reg := NewRegistry()

	counter := &countingGuard{}
	reg.Register(New(Pwd()).With(counter).With(counter))

	reg.Execute(context.Background(), "pwd", nil)
	if counter.count != 2 {
		t.Errorf("expected 2 guard checks, got %d", counter.count)
	}
}

// Tool tests

func TestTool_Read(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("hello world"), 0644)

	reg := NewRegistry()
	reg.Register(New(Read(tmpDir)))

	result, err := reg.Execute(context.Background(), "read", map[string]any{
		"path": testFile,
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if !strings.Contains(result, "hello world") {
		t.Errorf("expected file content, got %s", result)
	}
}

func TestTool_Write(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "output.txt")

	reg := NewRegistry()
	reg.Register(New(Write(tmpDir)))

	_, err := reg.Execute(context.Background(), "write", map[string]any{
		"path":    testFile,
		"content": "new content",
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	content, _ := os.ReadFile(testFile)
	if string(content) != "new content" {
		t.Errorf("expected 'new content', got %s", content)
	}
}

func TestTool_Write_CreateDirs(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "a", "b", "c", "file.txt")

	reg := NewRegistry()
	reg.Register(New(Write(tmpDir)))

	_, err := reg.Execute(context.Background(), "write", map[string]any{
		"path":    testFile,
		"content": "nested",
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("file should exist")
	}
}

func TestTool_Write_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	reg := NewRegistry()
	reg.Register(New(Write(tmpDir)))

	attacks := []string{
		"../../../etc/passwd",
		"foo/../../../etc/passwd",
		"./foo/../../bar/../../../etc/passwd",
	}

	for _, path := range attacks {
		_, err := reg.Execute(context.Background(), "write", map[string]any{
			"path":    path,
			"content": "malicious",
		})
		if err == nil {
			t.Errorf("path traversal should be blocked: %q", path)
		}
	}
}

func TestTool_Edit(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("hello world"), 0644)

	reg := NewRegistry()
	reg.Register(New(Edit(tmpDir)))

	_, err := reg.Execute(context.Background(), "edit", map[string]any{
		"path": testFile,
		"old":  "world",
		"new":  "universe",
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	content, _ := os.ReadFile(testFile)
	if string(content) != "hello universe" {
		t.Errorf("expected 'hello universe', got %s", content)
	}
}

func TestTool_Edit_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("hello world"), 0644)

	reg := NewRegistry()
	reg.Register(New(Edit(tmpDir)))

	_, err := reg.Execute(context.Background(), "edit", map[string]any{
		"path": testFile,
		"old":  "nonexistent",
		"new":  "replacement",
	})
	if err == nil {
		t.Error("expected error for pattern not found")
	}
}

func TestTool_Glob(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "a.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(tmpDir, "b.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(tmpDir, "c.txt"), []byte(""), 0644)

	reg := NewRegistry()
	reg.Register(New(Glob(tmpDir)))

	result, err := reg.Execute(context.Background(), "glob", map[string]any{
		"pattern": filepath.Join(tmpDir, "*.go"),
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 .go files, got %d: %v", len(lines), lines)
	}
}

func TestTool_Ls(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte(""), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte(""), 0644)
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)

	reg := NewRegistry()
	reg.Register(New(Ls(tmpDir)))

	result, err := reg.Execute(context.Background(), "ls", map[string]any{
		"path": tmpDir,
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 entries, got %d: %v", len(lines), lines)
	}
}

func TestTool_Bash(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Bash(t.TempDir())))

	result, err := reg.Execute(context.Background(), "bash", map[string]any{
		"command": "echo hello",
	})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if !strings.Contains(result, "hello") {
		t.Errorf("expected 'hello' in output, got %s", result)
	}
}

func TestTool_Read_OutsideWorkspace(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Read(t.TempDir())))

	_, err := reg.Execute(context.Background(), "read", map[string]any{
		"path": "/etc/passwd",
	})
	if err == nil {
		t.Error("expected error for path outside workspace")
	}
}

// --- Additional tool error-path tests ---

func TestTool_Edit_OutsideWorkspace(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Edit(t.TempDir())))

	_, err := reg.Execute(context.Background(), "edit", map[string]any{
		"path": "/etc/passwd",
		"old":  "root",
		"new":  "hacked",
	})
	if err == nil {
		t.Error("expected error for path outside workspace")
	}
}

func TestTool_Edit_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	reg.Register(New(Edit(dir)))

	_, err := reg.Execute(context.Background(), "edit", map[string]any{
		"path": filepath.Join(dir, "nope.txt"),
		"old":  "x",
		"new":  "y",
	})
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestTool_Glob_NoMatches(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	reg.Register(New(Glob(dir)))

	result, err := reg.Execute(context.Background(), "glob", map[string]any{
		"pattern": filepath.Join(dir, "*.xyz"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "No matches found." {
		t.Errorf("expected 'No matches found.', got %q", result)
	}
}

func TestTool_Head_ReadFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "lines.txt")
	var content string
	for i := 1; i <= 20; i++ {
		content += fmt.Sprintf("line %d\n", i)
	}
	os.WriteFile(f, []byte(content), 0644)

	reg := NewRegistry()
	reg.Register(New(Head(dir)))

	result, err := reg.Execute(context.Background(), "head", map[string]any{
		"path":  f,
		"lines": 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines, got %d", len(lines))
	}
}

func TestTool_Tail_ReadFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "lines.txt")
	var content string
	for i := 1; i <= 20; i++ {
		content += fmt.Sprintf("line %d\n", i)
	}
	os.WriteFile(f, []byte(content), 0644)

	reg := NewRegistry()
	reg.Register(New(Tail(dir)))

	result, err := reg.Execute(context.Background(), "tail", map[string]any{
		"path":  f,
		"lines": 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines, got %d", len(lines))
	}
}

func TestTool_Head_OutsideWorkspace(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Head(t.TempDir())))

	_, err := reg.Execute(context.Background(), "head", map[string]any{
		"path": "/etc/passwd",
	})
	if err == nil {
		t.Error("expected error for path outside workspace")
	}
}

func TestTool_Tail_OutsideWorkspace(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Tail(t.TempDir())))

	_, err := reg.Execute(context.Background(), "tail", map[string]any{
		"path": "/etc/passwd",
	})
	if err == nil {
		t.Error("expected error for path outside workspace")
	}
}

func TestTool_Head_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	reg.Register(New(Head(dir)))

	_, err := reg.Execute(context.Background(), "head", map[string]any{
		"path": filepath.Join(dir, "nope.txt"),
	})
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestTool_Tail_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	reg.Register(New(Tail(dir)))

	_, err := reg.Execute(context.Background(), "tail", map[string]any{
		"path": filepath.Join(dir, "nope.txt"),
	})
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestTool_Write_RelativePath(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	reg.Register(New(Write(dir)))

	_, err := reg.Execute(context.Background(), "write", map[string]any{
		"path":    "relative.txt",
		"content": "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "relative.txt"))
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", string(data))
	}
}

func TestTool_Write_OutsideWorkspace(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Write(t.TempDir())))

	_, err := reg.Execute(context.Background(), "write", map[string]any{
		"path":    "/etc/evil.txt",
		"content": "bad",
	})
	if err == nil {
		t.Error("expected error for path outside workspace")
	}
}

func TestTool_Rm_RecursiveDir(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	os.MkdirAll(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "file.txt"), []byte("x"), 0644)

	reg := NewRegistry()
	reg.Register(New(Rm(dir)))

	_, err := reg.Execute(context.Background(), "rm", map[string]any{
		"path":      subdir,
		"recursive": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(subdir); !os.IsNotExist(err) {
		t.Error("directory should have been removed")
	}
}

func TestTool_Rm_NonEmptyDirWithoutRecursive(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	os.MkdirAll(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "file.txt"), []byte("x"), 0644)

	reg := NewRegistry()
	reg.Register(New(Rm(dir)))

	_, err := reg.Execute(context.Background(), "rm", map[string]any{
		"path": subdir,
	})
	if err == nil {
		t.Error("expected error for non-empty dir without recursive")
	}
}

func TestTool_Rm_OutsideWorkspace(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Rm(t.TempDir())))

	_, err := reg.Execute(context.Background(), "rm", map[string]any{
		"path": "/etc/passwd",
	})
	if err == nil {
		t.Error("expected error for path outside workspace")
	}
}

func TestTool_Rm_NonExistent(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	reg.Register(New(Rm(dir)))

	_, err := reg.Execute(context.Background(), "rm", map[string]any{
		"path": filepath.Join(dir, "nope.txt"),
	})
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestTool_Rm_File(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "to_remove.txt")
	os.WriteFile(f, []byte("bye"), 0644)

	reg := NewRegistry()
	reg.Register(New(Rm(dir)))

	_, err := reg.Execute(context.Background(), "rm", map[string]any{"path": f})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Error("file should have been removed")
	}
}

func TestTool_Mkdir_Success(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	reg.Register(New(Mkdir(dir)))

	newDir := filepath.Join(dir, "newdir", "sub")
	_, err := reg.Execute(context.Background(), "mkdir", map[string]any{"path": newDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, err := os.Stat(newDir)
	if err != nil {
		t.Fatal("directory should exist")
	}
	if !info.IsDir() {
		t.Error("should be a directory")
	}
}

func TestTool_Mkdir_OutsideWorkspace(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Mkdir(t.TempDir())))

	_, err := reg.Execute(context.Background(), "mkdir", map[string]any{
		"path": "/tmp/evil_dir",
	})
	if err == nil {
		t.Error("expected error for path outside workspace")
	}
}

func TestTool_Ls_OutsideWorkspace(t *testing.T) {
	reg := NewRegistry()
	reg.Register(New(Ls(t.TempDir())))

	_, err := reg.Execute(context.Background(), "ls", map[string]any{
		"path": "/etc",
	})
	if err == nil {
		t.Error("expected error for path outside workspace")
	}
}

func TestTool_Ls_NonExistent(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	reg.Register(New(Ls(dir)))

	_, err := reg.Execute(context.Background(), "ls", map[string]any{
		"path": filepath.Join(dir, "nope"),
	})
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestTool_Glob_InvalidPattern(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	reg.Register(New(Glob(dir)))

	_, err := reg.Execute(context.Background(), "glob", map[string]any{
		"pattern": "[invalid",
	})
	if err == nil {
		t.Error("expected error for invalid glob pattern")
	}
}

func TestTool_Grep_WithPattern(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello world\nfoo bar"), 0644)

	reg := NewRegistry()
	reg.Register(New(Grep(dir)))

	result, err := reg.Execute(context.Background(), "grep", map[string]any{
		"pattern": "hello",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "hello") {
		t.Errorf("expected 'hello' in result, got %q", result)
	}
}

func TestTool_Grep_NoMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello world"), 0644)

	reg := NewRegistry()
	reg.Register(New(Grep(dir)))

	result, err := reg.Execute(context.Background(), "grep", map[string]any{
		"pattern": "zzz_no_match",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No matches") {
		t.Errorf("expected 'No matches' in result, got %q", result)
	}
}

func TestTool_Tree_WithDepth(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a", "b", "c"), 0755)
	os.WriteFile(filepath.Join(dir, "a", "b", "c", "deep.txt"), []byte(""), 0644)

	reg := NewRegistry()
	reg.Register(New(Tree(dir)))

	result, err := reg.Execute(context.Background(), "tree", map[string]any{
		"path":  dir,
		"depth": 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result, "deep.txt") {
		t.Error("deep.txt should not appear with depth=1")
	}
}

func TestTool_Git_Status(t *testing.T) {
	dir := t.TempDir()
	// Init a git repo
	reg := NewRegistry()
	reg.Register(New(Bash(dir)))
	reg.Execute(context.Background(), "bash", map[string]any{"command": "git init && git config user.email 'test@test.com' && git config user.name 'Test'"})

	reg2 := NewRegistry()
	reg2.Register(New(Git(dir)))

	result, err := reg2.Execute(context.Background(), "git", map[string]any{
		"args": "status",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty git status result")
	}
}

func TestTool_Git_DangerousFlag(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	reg.Register(New(Bash(dir)))
	reg.Execute(context.Background(), "bash", map[string]any{"command": "git init"})

	reg2 := NewRegistry()
	reg2.Register(New(Git(dir)))

	_, err := reg2.Execute(context.Background(), "git", map[string]any{
		"args": "push --force",
	})
	if err == nil {
		t.Error("expected error for dangerous flag")
	}
}

func TestTool_Git_DisallowedSubcommand(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	reg.Register(New(Git(dir)))

	_, err := reg.Execute(context.Background(), "git", map[string]any{
		"args": "clean -fd",
	})
	if err == nil {
		t.Error("expected error for disallowed subcommand")
	}
}

// Test helpers

type blockingGuard struct{}

func (g *blockingGuard) Check(ctx context.Context, args Args) error {
	return fmt.Errorf("blocked by guard")
}

type countingGuard struct{ count int }

func (g *countingGuard) Check(ctx context.Context, args Args) error {
	g.count++
	return nil
}
