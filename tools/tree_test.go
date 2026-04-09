package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTree_ShowsStructure(t *testing.T) {
	dir := t.TempDir()

	// Create nested structure
	os.MkdirAll(filepath.Join(dir, "src", "pkg"), 0755)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("readme"), 0644)
	os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "src", "pkg", "lib.go"), []byte("package pkg"), 0644)

	tool := Tree(dir)
	args, err := Validate(tool.Parameters(), map[string]any{
		"path": dir,
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(result, "README.md") {
		t.Error("expected tree to contain README.md")
	}
	if !strings.Contains(result, "src/") {
		t.Error("expected tree to contain src/")
	}
	if !strings.Contains(result, "main.go") {
		t.Error("expected tree to contain main.go")
	}
	if !strings.Contains(result, "lib.go") {
		t.Error("expected tree to contain lib.go")
	}
	// Check tree connectors are present
	if !strings.Contains(result, "├── ") && !strings.Contains(result, "└── ") {
		t.Error("expected tree connectors in output")
	}
}

func TestTree_RespectsDepth(t *testing.T) {
	dir := t.TempDir()

	// Create 3-level structure
	os.MkdirAll(filepath.Join(dir, "a", "b", "c"), 0755)
	os.WriteFile(filepath.Join(dir, "a", "b", "c", "deep.txt"), []byte("deep"), 0644)
	os.WriteFile(filepath.Join(dir, "a", "top.txt"), []byte("top"), 0644)

	tool := Tree(dir)
	args, err := Validate(tool.Parameters(), map[string]any{
		"path":  dir,
		"depth": 1,
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	// Depth 1 should show "a/" but not contents inside a/
	if !strings.Contains(result, "a/") {
		t.Error("expected depth-1 tree to contain a/")
	}
	if strings.Contains(result, "top.txt") {
		t.Error("depth 1 should not show files inside a/")
	}
	if strings.Contains(result, "deep.txt") {
		t.Error("depth 1 should not show deeply nested files")
	}
}

func TestTree_DefaultDepth(t *testing.T) {
	dir := t.TempDir()

	// Create 4-level structure
	os.MkdirAll(filepath.Join(dir, "a", "b", "c", "d"), 0755)
	os.WriteFile(filepath.Join(dir, "a", "b", "c", "d", "verydeep.txt"), []byte("deep"), 0644)
	os.WriteFile(filepath.Join(dir, "a", "b", "level2.txt"), []byte("l2"), 0644)

	tool := Tree(dir)
	args, err := Validate(tool.Parameters(), map[string]any{
		"path": dir,
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	// Default depth is 3; level2.txt is at depth 2, should be visible
	if !strings.Contains(result, "level2.txt") {
		t.Error("expected default depth to show files at depth 2")
	}
	// verydeep.txt is at depth 4, should not be visible with default depth 3
	if strings.Contains(result, "verydeep.txt") {
		t.Error("default depth 3 should not show files at depth 4")
	}
}
