package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type mkdirTool struct {
	workspace string
}

// Mkdir creates a tool that makes directories within the given workspace.
func Mkdir(workspace string) Tool {
	return &mkdirTool{workspace: workspace}
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
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.workspace, path)
	}
	path = filepath.Clean(path)

	if t.workspace != "" && !strings.HasPrefix(path, filepath.Clean(t.workspace)+string(filepath.Separator)) && path != filepath.Clean(t.workspace) {
		return "", fmt.Errorf("path %s is outside workspace %s", path, t.workspace)
	}
	return path, nil
}
