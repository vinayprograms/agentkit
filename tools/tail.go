package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type tailTool struct {
	workspace string
}

// Tail creates a tool that reads the last N lines of a file within the given workspace.
func Tail(workspace string) Tool {
	return &tailTool{workspace: workspace}
}

func (t *tailTool) Name() string { return "tail" }

func (t *tailTool) Description() string {
	return "Read the last N lines of a file (default 10)."
}

func (t *tailTool) Parameters() map[string]Param {
	return map[string]Param{
		"path": {
			Type:        StringParam,
			Description: "Path to the file",
			Required:    true,
		},
		"lines": {
			Type:        IntParam,
			Description: "Number of lines to read (default 10)",
		},
	}
}

func (t *tailTool) Execute(ctx context.Context, args Args) (string, error) {
	path, err := args.String("path")
	if err != nil {
		return "", err
	}

	path, err = t.resolve(path)
	if err != nil {
		return "", err
	}

	n := args.IntOr("lines", 10)

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	allLines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
	start := len(allLines) - n
	if start < 0 {
		start = 0
	}

	return strings.Join(allLines[start:], "\n"), nil
}

func (t *tailTool) resolve(path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.workspace, path)
	}
	path = filepath.Clean(path)

	if t.workspace != "" && !strings.HasPrefix(path, filepath.Clean(t.workspace)+string(filepath.Separator)) && path != filepath.Clean(t.workspace) {
		return "", fmt.Errorf("path %s is outside workspace %s", path, t.workspace)
	}
	return path, nil
}
