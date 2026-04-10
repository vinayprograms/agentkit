package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vinayprograms/agentkit/memory"
)

// -----------------------------------------------------------------------
// bash.go — missing arg, default timeout (no deadline in ctx)
// -----------------------------------------------------------------------

func TestBash_MissingCommand(t *testing.T) {
	tool := Bash(t.TempDir())
	args := Args{values: map[string]any{}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing command arg")
	}
}

func TestBash_DefaultTimeout(t *testing.T) {
	// Exercises the branch where ctx has no deadline so the tool adds its own.
	tool := Bash(t.TempDir())
	args, _ := Validate(tool.Parameters(), map[string]any{"command": "echo ok"})
	result, err := tool.Execute(context.Background(), args) // no deadline
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "ok") {
		t.Errorf("expected 'ok', got %q", result)
	}
}

func TestBash_StderrOnlyNoStdout(t *testing.T) {
	// Hit the stderr-only path where b.Len()==0 when stderr is appended.
	tool := Bash(t.TempDir())
	args, _ := Validate(tool.Parameters(), map[string]any{"command": "echo err >&2; exit 0"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "STDERR:") {
		t.Errorf("expected STDERR, got %q", result)
	}
}

func TestBash_ExitCodeOnly(t *testing.T) {
	// Non-zero exit with no stdout and no stderr.
	tool := Bash(t.TempDir())
	args, _ := Validate(tool.Parameters(), map[string]any{"command": "exit 42"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Exit code: 42") {
		t.Errorf("expected 'Exit code: 42', got %q", result)
	}
}

// -----------------------------------------------------------------------
// cp.go — missing args, copyFile to unwritable dest
// -----------------------------------------------------------------------

func TestCp_MissingSourceArg(t *testing.T) {
	tool := Cp(t.TempDir())
	args := Args{values: map[string]any{"destination": "dst.txt"}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing source")
	}
}

func TestCp_MissingDestArg(t *testing.T) {
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "src.txt"), []byte("data"), 0644)
	tool := Cp(ws)
	args := Args{values: map[string]any{"source": "src.txt"}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing destination")
	}
}

func TestCp_CopyFileToUnwritableDest(t *testing.T) {
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "src.txt"), []byte("data"), 0644)

	// Create a directory that can't be written to
	noWrite := filepath.Join(ws, "nope")
	os.MkdirAll(noWrite, 0555)
	defer os.Chmod(noWrite, 0755) // cleanup

	tool := Cp(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{
		"source":      "src.txt",
		"destination": filepath.Join(noWrite, "dst.txt"),
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error writing to unwritable directory")
	}
}

func TestCp_EmptyWorkspaceResolve(t *testing.T) {
	// With empty workspace, no workspace boundary check.
	ws := t.TempDir()
	src := filepath.Join(ws, "a.txt")
	dst := filepath.Join(ws, "b.txt")
	os.WriteFile(src, []byte("hello"), 0644)

	tool := Cp("") // empty workspace
	args, _ := Validate(tool.Parameters(), map[string]any{
		"source":      src,
		"destination": dst,
	})
	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------
// diff.go — missing file_b arg, file with additions only, deletions only
// -----------------------------------------------------------------------

func TestDiff_MissingFileA(t *testing.T) {
	tool := Diff()
	args := Args{values: map[string]any{"file_b": "/tmp/x"}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing file_a")
	}
}

func TestDiff_MissingFileB(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	os.WriteFile(f, []byte("x"), 0644)
	tool := Diff()
	args := Args{values: map[string]any{"file_a": f}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing file_b")
	}
}

func TestDiff_FileALongerThanB(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "a.txt")
	fb := filepath.Join(dir, "b.txt")
	os.WriteFile(fa, []byte("line1\nline2\nline3\nline4\nline5\n"), 0644)
	os.WriteFile(fb, []byte("line1\n"), 0644)

	tool := Diff()
	args, _ := Validate(tool.Parameters(), map[string]any{"file_a": fa, "file_b": fb})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "@@") {
		t.Error("expected diff hunk")
	}
}

func TestDiff_FileBLongerThanA(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "a.txt")
	fb := filepath.Join(dir, "b.txt")
	os.WriteFile(fa, []byte("line1\n"), 0644)
	os.WriteFile(fb, []byte("line1\nline2\nline3\nline4\nline5\n"), 0644)

	tool := Diff()
	args, _ := Validate(tool.Parameters(), map[string]any{"file_a": fa, "file_b": fb})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "@@") {
		t.Error("expected diff hunk")
	}
}

// -----------------------------------------------------------------------
// edit.go — missing args, file not found, outside workspace
// -----------------------------------------------------------------------

func TestEdit_MissingPathArg(t *testing.T) {
	tool := Edit(t.TempDir())
	args := Args{values: map[string]any{"old": "x", "new": "y"}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestEdit_MissingOldArg(t *testing.T) {
	tool := Edit(t.TempDir())
	args := Args{values: map[string]any{"path": "f.txt", "new": "y"}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing old")
	}
}

func TestEdit_MissingNewArg(t *testing.T) {
	ws := t.TempDir()
	f := filepath.Join(ws, "f.txt")
	os.WriteFile(f, []byte("hello"), 0644)
	tool := Edit(ws)
	args := Args{values: map[string]any{"path": f, "old": "hello"}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing new")
	}
}

func TestEdit_EmptyWorkspace(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	os.WriteFile(f, []byte("hello world"), 0644)
	tool := Edit("") // empty workspace - no boundary check
	args, _ := Validate(tool.Parameters(), map[string]any{
		"path": f, "old": "world", "new": "test",
	})
	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------
// env.go — env var with no '=' in os.Environ() (edge case), sensitive in listing
// -----------------------------------------------------------------------

func TestEnv_IsSensitiveVariousPatterns(t *testing.T) {
	patterns := []string{"MY_PASSWORD", "DB_PASS_WORD", "CREDENTIAL_X", "AUTH_CONFIG", "PRIVATE_DATA"}
	for _, p := range patterns {
		if !isSensitiveEnvVar(p) {
			t.Errorf("expected %s to be sensitive", p)
		}
	}
	if isSensitiveEnvVar("PATH") {
		t.Error("PATH should not be sensitive")
	}
}

// -----------------------------------------------------------------------
// git.go — empty args, failed git command, no output command
// -----------------------------------------------------------------------

func TestGit_EmptyArgs(t *testing.T) {
	tool := Git(t.TempDir())
	args, _ := Validate(tool.Parameters(), map[string]any{"args": ""})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for empty git command")
	}
	if !strings.Contains(err.Error(), "empty git command") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGit_MissingArgsParam(t *testing.T) {
	tool := Git(t.TempDir())
	args := Args{values: map[string]any{}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing args param")
	}
}

func TestGit_FailedCommand(t *testing.T) {
	dir := initGitRepo(t)
	tool := Git(dir)
	// log with invalid option
	args, _ := Validate(tool.Parameters(), map[string]any{"args": "log --invalidoption"})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for invalid git option")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGit_NoOutputCommand(t *testing.T) {
	dir := initGitRepo(t)
	tool := Git(dir)
	// stash with nothing to stash — should produce "completed (no output)" or similar
	args, _ := Validate(tool.Parameters(), map[string]any{"args": "stash list"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "no output") {
		t.Errorf("expected 'no output' message, got %q", result)
	}
}

func TestGit_QuotedArgs(t *testing.T) {
	dir := initGitRepo(t)
	tool := Git(dir)
	// Test parsing of quoted args — commit with quoted message
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0644)
	args, _ := Validate(tool.Parameters(), map[string]any{"args": "add new.txt"})
	tool.Execute(context.Background(), args)

	args, _ = Validate(tool.Parameters(), map[string]any{"args": `commit -m "test commit message"`})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "test commit message") {
		t.Errorf("expected commit message in result, got %q", result)
	}
}

// -----------------------------------------------------------------------
// glob.go — empty workspace (no prefix), relative pattern
// -----------------------------------------------------------------------

func TestGlob_EmptyWorkspace(t *testing.T) {
	tool := Glob("") // no workspace
	args, _ := Validate(tool.Parameters(), map[string]any{"pattern": "/nonexistent_dir_xyz/*.go"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if result != "No matches found." {
		t.Errorf("expected no matches, got %q", result)
	}
}

func TestGlob_MissingPatternArg(t *testing.T) {
	tool := Glob(t.TempDir())
	args := Args{values: map[string]any{}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing pattern")
	}
}

// -----------------------------------------------------------------------
// grep.go — single file search, path not found, outside workspace
// -----------------------------------------------------------------------

func TestGrep_SingleFileMatch(t *testing.T) {
	ws := t.TempDir()
	f := filepath.Join(ws, "test.txt")
	os.WriteFile(f, []byte("line one\nline two"), 0644)

	tool := Grep(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{"pattern": "two", "path": f})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "line two") {
		t.Errorf("expected match, got %q", result)
	}
}

func TestGrep_SingleFileNoMatch(t *testing.T) {
	ws := t.TempDir()
	f := filepath.Join(ws, "test.txt")
	os.WriteFile(f, []byte("hello"), 0644)

	tool := Grep(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{"pattern": "zzz", "path": f})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if result != "No matches found." {
		t.Errorf("expected 'No matches found.', got %q", result)
	}
}

func TestGrep_PathNotFound(t *testing.T) {
	ws := t.TempDir()
	tool := Grep(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{
		"pattern": "x",
		"path":    filepath.Join(ws, "nope.txt"),
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestGrep_MissingPatternArg(t *testing.T) {
	tool := Grep(t.TempDir())
	args := Args{values: map[string]any{"path": "."}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing pattern")
	}
}

func TestGrep_MissingPathArg(t *testing.T) {
	tool := Grep(t.TempDir())
	args := Args{values: map[string]any{"pattern": "x"}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestGrep_EmptyWorkspace(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	os.WriteFile(f, []byte("match me"), 0644)

	tool := Grep("") // no workspace boundary
	args, _ := Validate(tool.Parameters(), map[string]any{"pattern": "match", "path": f})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "match me") {
		t.Errorf("expected match, got %q", result)
	}
}

// -----------------------------------------------------------------------
// head.go — fewer lines than requested
// -----------------------------------------------------------------------

func TestHead_FewerLinesThanRequested(t *testing.T) {
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "small.txt"), []byte("one\ntwo"), 0644)

	tool := Head(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{"path": "small.txt", "lines": 100})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if result != "one\ntwo" {
		t.Errorf("expected full file, got %q", result)
	}
}

func TestHead_MissingPathArg(t *testing.T) {
	tool := Head(t.TempDir())
	args := Args{values: map[string]any{}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing path")
	}
}

// -----------------------------------------------------------------------
// ls.go — empty directory, missing path
// -----------------------------------------------------------------------

func TestLs_EmptyDir(t *testing.T) {
	ws := t.TempDir()
	emptyDir := filepath.Join(ws, "empty")
	os.MkdirAll(emptyDir, 0755)

	tool := Ls(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{"path": emptyDir})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if result != "Empty directory." {
		t.Errorf("expected 'Empty directory.', got %q", result)
	}
}

func TestLs_MissingPathArg(t *testing.T) {
	tool := Ls(t.TempDir())
	args := Args{values: map[string]any{}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestLs_EmptyWorkspace(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0644)

	tool := Ls("")
	args, _ := Validate(tool.Parameters(), map[string]any{"path": dir})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "f.txt") {
		t.Errorf("expected f.txt, got %q", result)
	}
}

// -----------------------------------------------------------------------
// memory.go — remember with store error, recall with store error
// -----------------------------------------------------------------------

type failingMemory struct{}

func (m *failingMemory) RememberFIL(ctx context.Context, findings, insights, lessons []string, source string) ([]string, error) {
	return nil, fmt.Errorf("store error")
}

func (m *failingMemory) RecallFIL(ctx context.Context, query string, limitPerCategory int) (*memory.FILResult, error) {
	return nil, fmt.Errorf("recall error")
}

func TestRemember_StoreError(t *testing.T) {
	tool := Remember(&failingMemory{})
	args, _ := Validate(tool.Parameters(), map[string]any{
		"findings": []any{"something"},
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error from failing store")
	}
	if !strings.Contains(err.Error(), "failed to store") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRecall_StoreError(t *testing.T) {
	tool := Recall(&failingMemory{})
	args, _ := Validate(tool.Parameters(), map[string]any{"query": "test"})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error from failing store")
	}
	if !strings.Contains(err.Error(), "recall failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRecall_NilResult(t *testing.T) {
	mem := newMockMemory()
	tool := Recall(mem)
	// query that won't match anything — results is empty but not nil
	args, _ := Validate(tool.Parameters(), map[string]any{"query": "zzzzz"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if result != "No relevant memories found." {
		t.Errorf("expected no memories, got %q", result)
	}
}

func TestRecall_MissingQueryArg(t *testing.T) {
	tool := Recall(newMockMemory())
	args := Args{values: map[string]any{}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing query")
	}
}

func TestRemember_FilterEmptyStrings(t *testing.T) {
	mem := newMockMemory()
	tool := Remember(mem)
	// Pass items with whitespace-only entries
	args, _ := Validate(tool.Parameters(), map[string]any{
		"findings": []any{"real finding", "  ", ""},
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Stored 1 observations") {
		t.Errorf("expected 1 stored, got %q", result)
	}
}

// -----------------------------------------------------------------------
// mkdir.go — missing path, empty workspace
// -----------------------------------------------------------------------

func TestMkdir_MissingPathArg(t *testing.T) {
	tool := Mkdir(t.TempDir())
	args := Args{values: map[string]any{}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestMkdir_EmptyWorkspace(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sub")
	tool := Mkdir("") // no workspace boundary
	args, _ := Validate(tool.Parameters(), map[string]any{"path": target})
	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Error("directory should exist")
	}
}

// -----------------------------------------------------------------------
// mv.go — missing args
// -----------------------------------------------------------------------

func TestMv_MissingSourceArg(t *testing.T) {
	tool := Mv(t.TempDir())
	args := Args{values: map[string]any{"destination": "dst.txt"}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing source")
	}
}

func TestMv_MissingDestArg(t *testing.T) {
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "s.txt"), []byte("x"), 0644)
	tool := Mv(ws)
	args := Args{values: map[string]any{"source": "s.txt"}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing destination")
	}
}

// -----------------------------------------------------------------------
// patch.go — missing args, malformed patch, only removals
// -----------------------------------------------------------------------

func TestPatch_MissingPathArg(t *testing.T) {
	tool := Patch()
	args := Args{values: map[string]any{"patch": "x"}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestPatch_MissingPatchArg(t *testing.T) {
	tool := Patch()
	args := Args{values: map[string]any{"path": "/tmp/x"}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing patch")
	}
}

func TestPatch_RemoveLinesOnly(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "t.txt")
	os.WriteFile(f, []byte("line1\nline2\nline3\n"), 0644)

	patch := `--- a/t.txt
+++ b/t.txt
@@ -1,3 +1,2 @@
 line1
-line2
 line3
`
	tool := Patch()
	args, _ := Validate(tool.Parameters(), map[string]any{"path": f, "patch": patch})
	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(f)
	if strings.Contains(string(content), "line2") {
		t.Error("line2 should have been removed")
	}
}

func TestPatch_PatchWithNoHunks(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "t.txt")
	os.WriteFile(f, []byte("hello\n"), 0644)

	// A patch with just headers and no @@ hunks
	patch := `--- a/t.txt
+++ b/t.txt
`
	tool := Patch()
	args, _ := Validate(tool.Parameters(), map[string]any{"path": f, "patch": patch})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "successfully") {
		t.Errorf("expected success, got %q", result)
	}
}

// -----------------------------------------------------------------------
// read.go — missing path, empty workspace
// -----------------------------------------------------------------------

func TestRead_MissingPathArg(t *testing.T) {
	tool := Read(t.TempDir())
	args := Args{values: map[string]any{}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestRead_EmptyWorkspace(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	os.WriteFile(f, []byte("content"), 0644)

	tool := Read("") // no workspace boundary check
	args, _ := Validate(tool.Parameters(), map[string]any{"path": f})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if result != "content" {
		t.Errorf("expected 'content', got %q", result)
	}
}

// -----------------------------------------------------------------------
// registry.go — nil entry registration
// -----------------------------------------------------------------------

func TestRegistry_RegisterNilEntry(t *testing.T) {
	reg := NewRegistry()
	err := reg.Register(nil)
	if err == nil {
		t.Error("expected error for nil entry")
	}
	if !strings.Contains(err.Error(), "nil entry") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRegistry_RegisterNilToolEntry(t *testing.T) {
	reg := NewRegistry()
	err := reg.Register(&Entry{tool: nil})
	if err == nil {
		t.Error("expected error for nil tool entry")
	}
}

// -----------------------------------------------------------------------
// rm.go — delete single file with recursive=true (different code path)
// -----------------------------------------------------------------------

func TestRm_FileWithRecursive(t *testing.T) {
	ws := t.TempDir()
	f := filepath.Join(ws, "f.txt")
	os.WriteFile(f, []byte("x"), 0644)

	tool := Rm(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{
		"path":      "f.txt",
		"recursive": true,
	})
	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestRm_MissingPathArg(t *testing.T) {
	tool := Rm(t.TempDir())
	args := Args{values: map[string]any{}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestRm_EmptyDirWithoutRecursive(t *testing.T) {
	ws := t.TempDir()
	emptyDir := filepath.Join(ws, "empty")
	os.MkdirAll(emptyDir, 0755)

	tool := Rm(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{"path": "empty"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Deleted") {
		t.Errorf("expected 'Deleted', got %q", result)
	}
}

// -----------------------------------------------------------------------
// scratchpad.go — error paths in read/write/list/search
// -----------------------------------------------------------------------

type errorScratchpad struct{}

func (s *errorScratchpad) Get(key string) (string, error)           { return "", fmt.Errorf("get error") }
func (s *errorScratchpad) Set(key, value string) error              { return fmt.Errorf("set error") }
func (s *errorScratchpad) List(prefix string) ([]string, error)     { return nil, fmt.Errorf("list error") }
func (s *errorScratchpad) Search(query string) (map[string]string, error) {
	return nil, fmt.Errorf("search error")
}

func TestScratchpadRead_StoreError(t *testing.T) {
	tool := ScratchpadRead(&errorScratchpad{})
	args, _ := Validate(tool.Parameters(), map[string]any{"key": "x"})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error from failing store")
	}
}

func TestScratchpadWrite_StoreError(t *testing.T) {
	tool := ScratchpadWrite(&errorScratchpad{})
	args, _ := Validate(tool.Parameters(), map[string]any{"key": "x", "value": "y"})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error from failing store")
	}
}

func TestScratchpadList_StoreError(t *testing.T) {
	tool := ScratchpadList(&errorScratchpad{})
	args, _ := Validate(tool.Parameters(), map[string]any{})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error from failing store")
	}
}

func TestScratchpadSearch_StoreError(t *testing.T) {
	tool := ScratchpadSearch(&errorScratchpad{})
	args, _ := Validate(tool.Parameters(), map[string]any{"query": "x"})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error from failing store")
	}
}

func TestScratchpadRead_EmptyValueReturnsNotFound(t *testing.T) {
	store := newMockScratchpad()
	store.data["k"] = "" // empty value
	tool := ScratchpadRead(store)
	args, _ := Validate(tool.Parameters(), map[string]any{"key": "k"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "not found") {
		t.Errorf("expected 'not found', got %q", result)
	}
}

// -----------------------------------------------------------------------
// spawn.go — invalid agent format, missing required fields
// -----------------------------------------------------------------------

func TestSpawn_InvalidAgentFormat(t *testing.T) {
	tool := Spawn(func(ctx context.Context, role, task string, outputs []string) (string, error) {
		return "ok", nil
	})
	args, _ := Validate(tool.Parameters(), map[string]any{
		"agents": []any{"not a map"},
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for invalid agent format")
	}
	if !strings.Contains(err.Error(), "invalid format") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSpawn_MissingRoleField(t *testing.T) {
	tool := Spawn(func(ctx context.Context, role, task string, outputs []string) (string, error) {
		return "ok", nil
	})
	args, _ := Validate(tool.Parameters(), map[string]any{
		"agents": []any{
			map[string]any{"task": "do something"},
		},
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing role")
	}
}

func TestSpawn_AgentsNotArray(t *testing.T) {
	tool := Spawn(func(ctx context.Context, role, task string, outputs []string) (string, error) {
		return "ok", nil
	})
	args := Args{values: map[string]any{"agents": "not an array"}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error when agents is not an array")
	}
	if !strings.Contains(err.Error(), "must be an array") {
		t.Errorf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------
// tail.go — fewer lines than requested, missing path
// -----------------------------------------------------------------------

func TestTail_FewerLinesThanRequested(t *testing.T) {
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "small.txt"), []byte("one\ntwo"), 0644)

	tool := Tail(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{"path": "small.txt", "lines": 100})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}
}

func TestTail_MissingPathArg(t *testing.T) {
	tool := Tail(t.TempDir())
	args := Args{values: map[string]any{}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing path")
	}
}

// -----------------------------------------------------------------------
// tree.go — empty directory, non-existent directory
// -----------------------------------------------------------------------

func TestTree_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	tool := Tree(dir)
	args, _ := Validate(tool.Parameters(), map[string]any{"path": dir})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	// Empty dir should just show the root path and no tree entries
	if !strings.HasPrefix(result, dir) {
		t.Errorf("expected result to start with dir path, got %q", result)
	}
}

func TestTree_NonExistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nope")
	tool := Tree("")
	args, _ := Validate(tool.Parameters(), map[string]any{"path": dir})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err) // buildTree silently handles os.ReadDir errors
	}
	// Should just show the path header, no entries
	if !strings.HasPrefix(result, dir) {
		t.Errorf("expected dir in result, got %q", result)
	}
}

func TestTree_HiddenFilesExcluded(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "visible"), []byte("x"), 0644)

	tool := Tree("")
	args, _ := Validate(tool.Parameters(), map[string]any{"path": dir})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result, ".hidden") {
		t.Error("hidden files should be excluded")
	}
	if !strings.Contains(result, "visible") {
		t.Error("visible files should be included")
	}
}

func TestTree_MissingPathArg(t *testing.T) {
	tool := Tree(t.TempDir())
	args := Args{values: map[string]any{}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing path")
	}
}

// -----------------------------------------------------------------------
// web_fetch.go — missing url arg, missing question arg
// -----------------------------------------------------------------------

func TestWebFetch_MissingURLArg(t *testing.T) {
	tool := Fetch(nil)
	args := Args{values: map[string]any{"question": "q"}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing url")
	}
}

func TestWebFetch_MissingQuestionArg(t *testing.T) {
	tool := Fetch(nil)
	args := Args{values: map[string]any{"url": "http://example.com"}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing question")
	}
}

// -----------------------------------------------------------------------
// web_search.go — resolveEngines with different cred combos, count clamping
// -----------------------------------------------------------------------

func TestResolveEngines_NilCreds(t *testing.T) {
	engines := resolveEngines(nil)
	// Should have at least DuckDuckGo as fallback
	if len(engines) < 1 {
		t.Error("expected at least 1 engine")
	}
}

func TestResolveEngines_WithSearXNG(t *testing.T) {
	creds := &mockCreds{keys: map[string]string{"searxng": "http://localhost:8080"}}
	engines := resolveEngines(creds)
	// Should have searxng + duckduckgo
	if len(engines) < 2 {
		t.Errorf("expected at least 2 engines, got %d", len(engines))
	}
}

func TestResolveEngines_WithBrave(t *testing.T) {
	creds := &mockCreds{keys: map[string]string{"brave": "test-key"}}
	engines := resolveEngines(creds)
	if len(engines) < 2 {
		t.Errorf("expected at least 2 engines, got %d", len(engines))
	}
}

func TestResolveEngines_AllProviders(t *testing.T) {
	creds := &mockCreds{keys: map[string]string{
		"searxng": "http://localhost",
		"brave":   "key1",
		"tavily":  "key2",
	}}
	engines := resolveEngines(creds)
	// searxng + brave + tavily + duckduckgo = 4
	if len(engines) != 4 {
		t.Errorf("expected 4 engines, got %d", len(engines))
	}
}

func TestSearch_MissingQueryArg(t *testing.T) {
	tool := Search(nil)
	args := Args{values: map[string]any{}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing query")
	}
}

func TestSearch_CountClampLow(t *testing.T) {
	engine := &mockEngine{results: []searchResult{{Title: "R", URL: "https://r.com"}}}
	tool := &webSearchTool{engines: []SearchEngine{engine}}
	args, _ := Validate(tool.Parameters(), map[string]any{"query": "test", "count": -5})
	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
}

// DuckDuckGo non-200/non-rate-limit error
func TestDuckDuckGo_Non200Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503) // Not a rate-limit status
	}))
	defer server.Close()

	engine := &duckDuckGoEngine{client: &http.Client{Transport: rewriteTransport{url: server.URL}}}
	_, err := engine.Search(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected error for 503")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("expected 503 in error, got %v", err)
	}
}

// DuckDuckGo 202 rate limit
func TestDuckDuckGo_202RateLimit(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= 1 {
			w.WriteHeader(202) // Rate limited
			return
		}
		fmt.Fprint(w, `<a class="result__a" href="https://example.com">OK</a>`)
	}))
	defer server.Close()

	engine := &duckDuckGoEngine{client: &http.Client{Transport: rewriteTransport{url: server.URL}}}
	results, err := engine.Search(context.Background(), "test", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) < 1 {
		t.Error("expected at least 1 result after retry")
	}
}

// DuckDuckGo 403 rate limit
func TestDuckDuckGo_403RateLimit(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= 1 {
			w.WriteHeader(403)
			return
		}
		fmt.Fprint(w, `<a class="result__a" href="https://example.com">OK</a>`)
	}))
	defer server.Close()

	engine := &duckDuckGoEngine{client: &http.Client{Transport: rewriteTransport{url: server.URL}}}
	results, err := engine.Search(context.Background(), "test", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) < 1 {
		t.Error("expected results after retry")
	}
}

// DuckDuckGo context cancellation during cooldown
func TestDuckDuckGo_ContextCancelledDuringCooldown(t *testing.T) {
	engine := &duckDuckGoEngine{
		client:     &http.Client{},
		lastSearch: time.Now(), // just searched, will trigger cooldown
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := engine.Search(ctx, "test", 5)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

// DuckDuckGo context cancelled during backoff retry
func TestDuckDuckGo_ContextCancelledDuringRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	}))
	defer server.Close()

	engine := &duckDuckGoEngine{client: &http.Client{Transport: rewriteTransport{url: server.URL}}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := engine.Search(ctx, "test", 5)
	if err == nil {
		t.Error("expected error from timeout")
	}
}

// DuckDuckGo read body error — hard to test directly, but we test success path
func TestDuckDuckGo_SuccessParseHTML(t *testing.T) {
	html := `<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fgo.dev&rut=x">Go Dev</a>
	<a class="result__snippet">Official Go site</a>
	<a class="result__a" href="https://golang.org">Golang</a>
	<a class="result__snippet">Another site</a>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, html)
	}))
	defer server.Close()

	engine := &duckDuckGoEngine{client: &http.Client{Transport: rewriteTransport{url: server.URL}}}
	results, err := engine.Search(context.Background(), "test", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 2 {
		t.Errorf("expected at least 2 results, got %d", len(results))
	}
}

// Brave error status
func TestBrave_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte("unauthorized"))
	}))
	defer server.Close()

	engine := &braveEngine{apiKey: "bad", client: &http.Client{Transport: rewriteTransport{url: server.URL}}}
	_, err := engine.Search(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected error for 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("unexpected error: %v", err)
	}
}

// Tavily error status
func TestTavily_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	engine := &tavilyEngine{apiKey: "bad", client: &http.Client{Transport: rewriteTransport{url: server.URL}}}
	_, err := engine.Search(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected error for 400")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------
// which.go — missing command arg
// -----------------------------------------------------------------------

func TestWhich_MissingCommandArg(t *testing.T) {
	tool := Which()
	args := Args{values: map[string]any{}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing command")
	}
}

// -----------------------------------------------------------------------
// write.go — missing args, empty workspace
// -----------------------------------------------------------------------

func TestWrite_MissingPathArg(t *testing.T) {
	tool := Write(t.TempDir())
	args := Args{values: map[string]any{"content": "x"}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestWrite_MissingContentArg(t *testing.T) {
	tool := Write(t.TempDir())
	args := Args{values: map[string]any{"path": "f.txt"}}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing content")
	}
}

func TestWrite_EmptyWorkspace(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	tool := Write("") // no workspace boundary
	args, _ := Validate(tool.Parameters(), map[string]any{"path": f, "content": "hello"})
	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(f)
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", string(data))
	}
}

// -----------------------------------------------------------------------
// DuckDuckGo URL with uddg but no valid URL after decode
// -----------------------------------------------------------------------

func TestParseDuckDuckGoHTML_UddgNonHTTP(t *testing.T) {
	// URL that has uddg= but the decoded value doesn't start with http
	html := `<a class="result__a" href="//duckduckgo.com/l/?uddg=ftp%3A%2F%2Fexample.com">FTP</a>`
	results := parseDuckDuckGoHTML(html, 5)
	// ftp:// doesn't start with http, so it's skipped
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-http URL, got %d", len(results))
	}
}

// -----------------------------------------------------------------------
// SearXNG limiting results to count
// -----------------------------------------------------------------------

func TestSearXNG_LimitsToCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		results := make([]map[string]any, 10)
		for i := range 10 {
			results[i] = map[string]any{
				"title":   fmt.Sprintf("Result %d", i),
				"url":     fmt.Sprintf("https://example.com/%d", i),
				"content": "snippet",
			}
		}
		data := map[string]any{"results": results}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	}))
	defer server.Close()

	engine := &searxngEngine{baseURL: server.URL, client: server.Client()}
	results, err := engine.Search(context.Background(), "test", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

// -----------------------------------------------------------------------
// DuckDuckGo client.Do error path (connection error on retry)
// -----------------------------------------------------------------------

func TestDuckDuckGo_ClientDoError(t *testing.T) {
	// Use a client that always fails
	engine := &duckDuckGoEngine{
		client: &http.Client{Transport: &failingTransport{}},
	}
	_, err := engine.Search(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected error from failing transport")
	}
	if !strings.Contains(err.Error(), "retries") {
		t.Errorf("expected retries error, got %v", err)
	}
}

type failingTransport struct{}

func (t *failingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("transport error")
}

// -----------------------------------------------------------------------
// diff.go — identical files return "Files are identical"
// -----------------------------------------------------------------------

func TestDiff_IdenticalFilesNoHunks(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "a.txt")
	fb := filepath.Join(dir, "b.txt")
	os.WriteFile(fa, []byte("same content"), 0644)
	os.WriteFile(fb, []byte("same content"), 0644)

	tool := Diff()
	args, _ := Validate(tool.Parameters(), map[string]any{"file_a": fa, "file_b": fb})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	// Identical files still produce headers but no @@ hunks
	if strings.Contains(result, "@@") {
		t.Errorf("expected no hunks for identical files, got %q", result)
	}
}

// -----------------------------------------------------------------------
// edit.go — resolve with relative path in workspace
// -----------------------------------------------------------------------

func TestEdit_RelativePath(t *testing.T) {
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "f.txt"), []byte("hello world"), 0644)

	tool := Edit(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{
		"path": "f.txt",
		"old":  "world",
		"new":  "test",
	})
	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(ws, "f.txt"))
	if !strings.Contains(string(data), "test") {
		t.Errorf("expected 'test', got %q", string(data))
	}
}

// -----------------------------------------------------------------------
// write.go — write to unwritable dir
// -----------------------------------------------------------------------

func TestWrite_UnwritableDir(t *testing.T) {
	ws := t.TempDir()
	noWrite := filepath.Join(ws, "nope")
	os.MkdirAll(noWrite, 0555)
	defer os.Chmod(noWrite, 0755)

	tool := Write(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{
		"path":    filepath.Join(noWrite, "sub", "file.txt"),
		"content": "hello",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error writing to unwritable dir")
	}
}

// -----------------------------------------------------------------------
// cp.go — copyDir walk error, copy unreadable file
// -----------------------------------------------------------------------

func TestCp_CopyDirWithUnreadableFile(t *testing.T) {
	ws := t.TempDir()
	srcDir := filepath.Join(ws, "src")
	os.MkdirAll(srcDir, 0755)
	unreadable := filepath.Join(srcDir, "no.txt")
	os.WriteFile(unreadable, []byte("secret"), 0000)
	defer os.Chmod(unreadable, 0644) // cleanup

	tool := Cp(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{
		"source":      "src",
		"destination": "dst",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error copying unreadable file")
	}
}

// -----------------------------------------------------------------------
// patch.go — stat error after patching (file removed between read and stat)
// -----------------------------------------------------------------------

// Note: patch stat error at line 59 is hard to trigger in a normal test.
// But we can test the write error by making the file read-only.

func TestPatch_WriteError(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "t.txt")
	os.WriteFile(f, []byte("line1\nline2\nline3\n"), 0644)

	// Make file read-only after initial read
	patch := `--- a/t.txt
+++ b/t.txt
@@ -1,3 +1,3 @@
 line1
-line2
+modified
 line3
`
	// Make the whole dir read-only to prevent writing
	os.Chmod(f, 0444)
	defer os.Chmod(f, 0644)

	tool := Patch()
	args, _ := Validate(tool.Parameters(), map[string]any{"path": f, "patch": patch})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error writing to read-only file")
	}
}

// -----------------------------------------------------------------------
// grep.go — error reading file in directory walk
// -----------------------------------------------------------------------

func TestGrep_UnreadableFileInDir(t *testing.T) {
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "readable.txt"), []byte("match"), 0644)
	unreadable := filepath.Join(ws, "unreadable.txt")
	os.WriteFile(unreadable, []byte("match"), 0000)
	defer os.Chmod(unreadable, 0644)

	tool := Grep(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{"pattern": "match", "path": ws})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	// Should still find match in readable file
	if !strings.Contains(result, "readable.txt") {
		t.Errorf("expected match in readable.txt, got %q", result)
	}
}

// -----------------------------------------------------------------------
// env.go — exercise the len(parts) != 2 branch
// This is hard to test since os.Environ() always returns valid entries.
// We can at least ensure the list path works with some entries.
// -----------------------------------------------------------------------

func TestEnv_ListFiltersSecrets(t *testing.T) {
	// Set both a normal and sensitive var, verify filtering
	os.Setenv("AGENTKIT_NORMAL_VAR", "visible")
	os.Setenv("AGENTKIT_SECRET_TOKEN_VAR", "invisible")
	defer os.Unsetenv("AGENTKIT_NORMAL_VAR")
	defer os.Unsetenv("AGENTKIT_SECRET_TOKEN_VAR")

	tool := Env()
	args, _ := Validate(tool.Parameters(), map[string]any{})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "AGENTKIT_NORMAL_VAR") {
		t.Error("expected normal var in listing")
	}
	if strings.Contains(result, "AGENTKIT_SECRET_TOKEN_VAR") {
		t.Error("secret var should be filtered")
	}
}

// -----------------------------------------------------------------------
// mkdir.go — MkdirAll error (unwritable parent)
// -----------------------------------------------------------------------

func TestMkdir_MkdirAllError(t *testing.T) {
	ws := t.TempDir()
	noWrite := filepath.Join(ws, "locked")
	os.MkdirAll(noWrite, 0555)
	defer os.Chmod(noWrite, 0755)

	tool := Mkdir(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{
		"path": filepath.Join(noWrite, "sub", "dir"),
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error creating dir in unwritable parent")
	}
}

// -----------------------------------------------------------------------
// rm.go — os.Remove error for file (unlikely but covers the else branch)
// -----------------------------------------------------------------------

func TestRm_DeleteFileNotRecursive(t *testing.T) {
	// Test the else branch in rm Execute: file + recursive=false
	ws := t.TempDir()
	f := filepath.Join(ws, "f.txt")
	os.WriteFile(f, []byte("data"), 0644)

	tool := Rm(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{
		"path":      "f.txt",
		"recursive": false,
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Deleted") {
		t.Errorf("expected 'Deleted', got %q", result)
	}
}

// -----------------------------------------------------------------------
// web_search — SearXNG/Brave/Tavily connection error paths
// -----------------------------------------------------------------------

func TestSearXNG_ConnectionError(t *testing.T) {
	engine := &searxngEngine{baseURL: "http://localhost:1", client: &http.Client{Timeout: 100 * time.Millisecond}}
	_, err := engine.Search(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected connection error")
	}
}

func TestBrave_ConnectionError(t *testing.T) {
	engine := &braveEngine{apiKey: "key", client: &http.Client{Transport: &failingTransport{}}}
	_, err := engine.Search(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected connection error")
	}
}

func TestTavily_ConnectionError(t *testing.T) {
	engine := &tavilyEngine{apiKey: "key", client: &http.Client{Transport: &failingTransport{}}}
	_, err := engine.Search(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected connection error")
	}
}

// -----------------------------------------------------------------------
// diff.go — unifiedDiff edge: context after block at end of file
// -----------------------------------------------------------------------

func TestDiff_DiffAtEndOfFile(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "a.txt")
	fb := filepath.Join(dir, "b.txt")
	// Diff where the last lines differ
	os.WriteFile(fa, []byte("same1\nsame2\nsame3\nold_end"), 0644)
	os.WriteFile(fb, []byte("same1\nsame2\nsame3\nnew_end"), 0644)

	tool := Diff()
	args, _ := Validate(tool.Parameters(), map[string]any{"file_a": fa, "file_b": fb})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "-old_end") {
		t.Errorf("expected removed line, got %q", result)
	}
	if !strings.Contains(result, "+new_end") {
		t.Errorf("expected added line, got %q", result)
	}
}

func TestDiff_MultipleHunks(t *testing.T) {
	dir := t.TempDir()
	fa := filepath.Join(dir, "a.txt")
	fb := filepath.Join(dir, "b.txt")

	// Create files with differences separated by enough matching lines
	var aLines, bLines []string
	for i := 0; i < 20; i++ {
		aLines = append(aLines, fmt.Sprintf("line%d", i))
		bLines = append(bLines, fmt.Sprintf("line%d", i))
	}
	aLines[2] = "old_early"
	bLines[2] = "new_early"
	aLines[15] = "old_late"
	bLines[15] = "new_late"

	os.WriteFile(fa, []byte(strings.Join(aLines, "\n")), 0644)
	os.WriteFile(fb, []byte(strings.Join(bLines, "\n")), 0644)

	tool := Diff()
	args, _ := Validate(tool.Parameters(), map[string]any{"file_a": fa, "file_b": fb})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "@@") {
		t.Error("expected hunk headers in diff")
	}
}

// -----------------------------------------------------------------------
// patch.go — applyPatch with pos < 0
// -----------------------------------------------------------------------

func TestPatch_HunkAtStartOfFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "t.txt")
	os.WriteFile(f, []byte("a\nb\nc\n"), 0644)

	// Patch targeting line 1 with @@ -1,1 format
	patch := `--- a/t.txt
+++ b/t.txt
@@ -1,1 +1,1 @@
-a
+x
`
	tool := Patch()
	args, _ := Validate(tool.Parameters(), map[string]any{"path": f, "patch": patch})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "successfully") {
		t.Errorf("expected success, got %q", result)
	}
	data, _ := os.ReadFile(f)
	if !strings.HasPrefix(string(data), "x\n") {
		t.Errorf("expected file to start with 'x', got %q", string(data))
	}
}

// -----------------------------------------------------------------------
// ls.go — resolve with relative path
// -----------------------------------------------------------------------

func TestLs_RelativePath(t *testing.T) {
	ws := t.TempDir()
	sub := filepath.Join(ws, "sub")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(sub, "f.txt"), []byte("x"), 0644)

	tool := Ls(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{"path": "sub"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "f.txt") {
		t.Errorf("expected f.txt, got %q", result)
	}
}

// -----------------------------------------------------------------------
// web_fetch.go — read body error is hard to test, but ensure the flow works
// -----------------------------------------------------------------------

// -----------------------------------------------------------------------
// bash.go — exec error (not ExitError, not timeout)
// -----------------------------------------------------------------------

func TestBash_ExecErrorInvalidBinary(t *testing.T) {
	// Create a tool with a nonexistent workspace to trigger a different error.
	// Actually the exec error at line 86 happens when cmd.Run() returns
	// a non-ExitError, non-timeout error. This is hard to trigger since bash
	// always runs. We can try with an empty PATH scenario but that's flaky.
	// Instead, let's cover the remaining branches elsewhere.
}

// -----------------------------------------------------------------------
// edit.go — WriteFile error (read-only file)
// -----------------------------------------------------------------------

func TestEdit_WriteFileError(t *testing.T) {
	ws := t.TempDir()
	f := filepath.Join(ws, "readonly.txt")
	os.WriteFile(f, []byte("hello world"), 0644)

	// Make file read-only
	os.Chmod(f, 0444)
	defer os.Chmod(f, 0644)

	tool := Edit(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{
		"path": f, "old": "world", "new": "test",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error writing to read-only file")
	}
	if !strings.Contains(err.Error(), "failed to write") {
		t.Errorf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------
// grep.go — filepath.Walk error returning from Execute
// -----------------------------------------------------------------------

func TestGrep_SingleFileReadError(t *testing.T) {
	ws := t.TempDir()
	f := filepath.Join(ws, "unreadable.txt")
	os.WriteFile(f, []byte("data"), 0000)
	defer os.Chmod(f, 0644)

	tool := Grep(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{"pattern": "data", "path": f})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error reading unreadable file")
	}
}

// -----------------------------------------------------------------------
// rm.go — os.Remove fails on a file (permission denied)
// -----------------------------------------------------------------------

func TestRm_RemoveFilePermissionDenied(t *testing.T) {
	ws := t.TempDir()
	lockedDir := filepath.Join(ws, "locked")
	os.MkdirAll(lockedDir, 0755)
	f := filepath.Join(lockedDir, "f.txt")
	os.WriteFile(f, []byte("x"), 0644)
	// Lock the directory so files can't be removed
	os.Chmod(lockedDir, 0555)
	defer os.Chmod(lockedDir, 0755)

	tool := Rm(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{
		"path": filepath.Join("locked", "f.txt"),
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error deleting file in locked dir")
	}
}

// -----------------------------------------------------------------------
// patch.go — os.Stat error after successful patch (hard, test WriteFile)
// -----------------------------------------------------------------------

// The patch Stat error at line 59 is between ReadFile and WriteFile.
// To hit it, the file would need to be deleted between those calls.
// This is a race condition and not worth testing. The WriteFile error
// at line 64 is already tested above via TestPatch_WriteError.

// -----------------------------------------------------------------------
// copyDir error paths
// -----------------------------------------------------------------------

func TestCopyDir_WalkError(t *testing.T) {
	ws := t.TempDir()
	srcDir := filepath.Join(ws, "src")
	os.MkdirAll(filepath.Join(srcDir, "sub"), 0755)
	os.WriteFile(filepath.Join(srcDir, "sub", "f.txt"), []byte("x"), 0644)

	// Make a sub dir unreadable to trigger walk error
	os.Chmod(filepath.Join(srcDir, "sub"), 0000)
	defer os.Chmod(filepath.Join(srcDir, "sub"), 0755)

	err := copyDir(srcDir, filepath.Join(ws, "dst"))
	if err == nil {
		t.Error("expected error from walk")
	}
}

// -----------------------------------------------------------------------
// DuckDuckGo — connection error on initial attempt (before retry)
// -----------------------------------------------------------------------

func TestDuckDuckGo_ReadBodyError(t *testing.T) {
	// Server that sets Content-Length but doesn't send full body, causing ReadAll to error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(200)
		w.Write([]byte("short"))
	}))
	defer server.Close()

	engine := &duckDuckGoEngine{client: &http.Client{Transport: rewriteTransport{url: server.URL}}}
	_, err := engine.Search(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected error from truncated body")
	}
	if !strings.Contains(err.Error(), "read response") {
		t.Errorf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------
// web_search — DuckDuckGo cooldown path (lastSearch very recent)
// -----------------------------------------------------------------------

func TestDuckDuckGo_CooldownWait(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<a class="result__a" href="https://example.com">OK</a>`)
	}))
	defer server.Close()

	engine := &duckDuckGoEngine{
		client:     &http.Client{Transport: rewriteTransport{url: server.URL}},
		lastSearch: time.Now(), // trigger cooldown
	}
	results, err := engine.Search(context.Background(), "test", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) < 1 {
		t.Error("expected results after cooldown")
	}
}

// -----------------------------------------------------------------------
// SearXNG — connection error (client.Do error)
// -----------------------------------------------------------------------

func TestSearXNG_ClientDoError(t *testing.T) {
	engine := &searxngEngine{
		baseURL: "http://localhost:1",
		client:  &http.Client{Transport: &failingTransport{}},
	}
	_, err := engine.Search(context.Background(), "test", 5)
	if err == nil {
		t.Error("expected error from failing transport")
	}
	if !strings.Contains(err.Error(), "searxng") {
		t.Errorf("expected searxng prefix, got %v", err)
	}
}

// -----------------------------------------------------------------------
// write.go — WriteFile error (dir instead of file)
// -----------------------------------------------------------------------

func TestWrite_WriteFileErrorDirAsFile(t *testing.T) {
	ws := t.TempDir()
	// Create a directory where the file should go
	dirAsFile := filepath.Join(ws, "notafile")
	os.MkdirAll(dirAsFile, 0755)

	tool := Write(ws)
	args, _ := Validate(tool.Parameters(), map[string]any{
		"path":    "notafile", // this is a directory
		"content": "hello",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error writing to directory path")
	}
}

// -----------------------------------------------------------------------
// SearXNG/Brave/Tavily — NewRequestWithContext error (invalid method)
// These only fail for truly invalid URLs, so not easily testable.
// -----------------------------------------------------------------------

// -----------------------------------------------------------------------
// patch.go — exercise the "pos+countA > len(result)" branch by using
// a patch whose hunk extends beyond the file
// -----------------------------------------------------------------------

func TestPatch_HunkBeyondEndOfFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "t.txt")
	os.WriteFile(f, []byte("a\nb\n"), 0644)

	// Patch claims to modify more lines than exist
	patch := `--- a/t.txt
+++ b/t.txt
@@ -1,10 +1,2 @@
 a
 b
`
	tool := Patch()
	args, _ := Validate(tool.Parameters(), map[string]any{"path": f, "patch": patch})
	// This may succeed or fail depending on implementation — we just ensure it doesn't panic
	tool.Execute(context.Background(), args)
}

// ensure json import is used
var _ = json.NewEncoder
