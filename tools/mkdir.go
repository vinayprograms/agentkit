package tools

import (
	"context"
	"fmt"
	"os"
)

type mkdirTool struct {
	workspace  string
	extraRoots []string
}

// Mkdir creates a tool that makes directories within the given workspace.
func Mkdir(workspace string, extraRoots ...string) Tool {
	return &mkdirTool{workspace: workspace, extraRoots: extraRoots}
}

func (t *mkdirTool) Name() string { return "mkdir" }

func (t *mkdirTool) Description() string {
	return "Create a directory (and parent directories if needed)."
}

func (t *mkdirTool) Parameters() map[string]Param {
	return map[string]Param{
		"path": {
			Type:        StringParam,
			Description: "Directory path to create",
			Required:    true,
		},
	}
}

func (t *mkdirTool) Execute(ctx context.Context, args Args) (string, error) {
	path, err := args.String("path")
	if err != nil {
		return "", err
	}

	path, err = t.resolve(path)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	return fmt.Sprintf("Created directory: %s", path), nil
}

// resolve makes the path absolute relative to workspace and validates it stays inside.
func (t *mkdirTool) resolve(path string) (string, error) {
	return confine(path, t.workspace, t.extraRoots)
}
