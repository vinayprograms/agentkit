package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type writeTool struct {
	workspace  string
	extraRoots []string
}

// Write creates a tool that writes content to files within the given workspace.
func Write(workspace string, extraRoots ...string) Tool {
	return &writeTool{workspace: workspace, extraRoots: extraRoots}
}

func (t *writeTool) Name() string { return "write" }

func (t *writeTool) Description() string {
	return "Write content to a file at the given path. Creates parent directories if needed."
}

func (t *writeTool) Parameters() map[string]Param {
	return map[string]Param{
		"path": {
			Type:        StringParam,
			Description: "Path to the file to write",
			Required:    true,
		},
		"content": {
			Type:        StringParam,
			Description: "Content to write to the file",
			Required:    true,
		},
	}
}

func (t *writeTool) Execute(ctx context.Context, args Args) (string, error) {
	path, err := args.String("path")
	if err != nil {
		return "", err
	}
	content, err := args.String("content")
	if err != nil {
		return "", err
	}

	safePath, err := t.resolve(path)
	if err != nil {
		return "", err
	}

	// Create parent directories
	dir := filepath.Dir(safePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directories: %w", err)
	}

	if err := os.WriteFile(safePath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return "ok", nil
}

func (t *writeTool) resolve(path string) (string, error) {
	return confine(path, t.workspace, t.extraRoots)
}
