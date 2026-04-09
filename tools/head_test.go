package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHeadDefault10Lines(t *testing.T) {
	ws := t.TempDir()
	tool := Head(ws)

	// Create a file with 20 lines
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, fmt.Sprintf("line%d", i))
	}
	os.WriteFile(filepath.Join(ws, "test.txt"), []byte(strings.Join(lines, "\n")), 0644)

	args, err := Validate(tool.Parameters(), map[string]any{"path": "test.txt"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.Split(result, "\n")
	if len(got) != 10 {
		t.Errorf("expected 10 lines, got %d", len(got))
	}
	if got[0] != "line1" {
		t.Errorf("first line: got %q, want %q", got[0], "line1")
	}
	if got[9] != "line10" {
		t.Errorf("last line: got %q, want %q", got[9], "line10")
	}
}

func TestHeadCustomLines(t *testing.T) {
	ws := t.TempDir()
	tool := Head(ws)

	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, fmt.Sprintf("line%d", i))
	}
	os.WriteFile(filepath.Join(ws, "test.txt"), []byte(strings.Join(lines, "\n")), 0644)

	args, err := Validate(tool.Parameters(), map[string]any{"path": "test.txt", "lines": 3})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.Split(result, "\n")
	if len(got) != 3 {
		t.Errorf("expected 3 lines, got %d", len(got))
	}
	if got[0] != "line1" {
		t.Errorf("first line: got %q, want %q", got[0], "line1")
	}
}
