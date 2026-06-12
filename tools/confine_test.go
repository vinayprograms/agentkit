package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfine_WorkspaceOnly(t *testing.T) {
	ws := "/work"
	got, err := confine("file.txt", ws, nil)
	if err != nil || got != filepath.Clean("/work/file.txt") {
		t.Fatalf("got %q err %v", got, err)
	}

	if _, err := confine("/etc/passwd", ws, nil); err == nil {
		t.Fatal("expected outside-workspace error")
	}
}

func TestConfine_ExtraRootAllowsOutsidePath(t *testing.T) {
	ws := "/work"
	extra := []string{"/data"}

	got, err := confine("/data/sub/file.txt", ws, extra)
	if err != nil || got != "/data/sub/file.txt" {
		t.Fatalf("extra root should allow path: got %q err %v", got, err)
	}

	// Root itself is allowed.
	if _, err := confine("/data", ws, extra); err != nil {
		t.Fatalf("root path should be allowed: %v", err)
	}

	// Still denies a path under neither root.
	if _, err := confine("/other/file", ws, extra); err == nil {
		t.Fatal("expected denial for path outside all roots")
	}
}

func TestConfine_EmptyWorkspaceDisablesConfinement(t *testing.T) {
	got, err := confine("/anywhere/file", "", nil)
	if err != nil || got != "/anywhere/file" {
		t.Fatalf("empty workspace should allow any path: got %q err %v", got, err)
	}
}

func TestReadTool_ExtraRoots(t *testing.T) {
	ws := t.TempDir()
	extra := t.TempDir()
	target := filepath.Join(extra, "note.txt")
	if err := os.WriteFile(target, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}

	tool := Read(ws, extra)
	out, err := tool.Execute(context.Background(), mustArgs(t, tool, map[string]any{"path": target}))
	if err != nil {
		t.Fatalf("read within extra root failed: %v", err)
	}
	if out != "hello" {
		t.Fatalf("got %q", out)
	}

	// Without the extra root, the same path is rejected.
	tool2 := Read(ws)
	if _, err := tool2.Execute(context.Background(), mustArgs(t, tool2, map[string]any{"path": target})); err == nil {
		t.Fatal("expected rejection without extra root")
	}
}

func mustArgs(t *testing.T, tool Tool, raw map[string]any) Args {
	t.Helper()
	args, err := Validate(tool.Parameters(), raw)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	return args
}

func TestWithHTTPTimeout(t *testing.T) {
	cfg := webConfigFrom([]WebOption{WithHTTPTimeout(7 * time.Second)})
	if cfg.timeout != 7*time.Second {
		t.Fatalf("got %v", cfg.timeout)
	}
	// Non-positive leaves default.
	cfg = webConfigFrom([]WebOption{WithHTTPTimeout(0)})
	if cfg.timeout != defaultHTTPTimeout {
		t.Fatalf("expected default, got %v", cfg.timeout)
	}
}
