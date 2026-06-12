package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type headTool struct {
	workspace  string
	extraRoots []string
}

// Head creates a tool that reads the first N lines of a file within the given workspace.
func Head(workspace string, extraRoots ...string) Tool {
	return &headTool{workspace: workspace, extraRoots: extraRoots}
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
	return confine(path, t.workspace, t.extraRoots)
}
