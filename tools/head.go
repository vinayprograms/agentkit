package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type headTool struct {
	workspace string
}

// Head creates a tool that reads the first N lines of a file within the given workspace.
func Head(workspace string) Tool {
	return &headTool{workspace: workspace}
}

func (t *headTool) Name() string { return "head" }

func (t *headTool) Description() string {
	return "Read the first N lines of a file (default 10)."
}

func (t *headTool) Parameters() map[string]Param {
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

func (t *headTool) Execute(ctx context.Context, args Args) (string, error) {
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

	lines := strings.SplitN(string(content), "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}

	return strings.Join(lines, "\n"), nil
}

func (t *headTool) resolve(path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.workspace, path)
	}
	path = filepath.Clean(path)

	if t.workspace != "" && !strings.HasPrefix(path, filepath.Clean(t.workspace)+string(filepath.Separator)) && path != filepath.Clean(t.workspace) {
		return "", fmt.Errorf("path %s is outside workspace %s", path, t.workspace)
	}
	return path, nil
}
